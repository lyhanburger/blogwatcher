package scanner

import (
	"fmt"
	"time"

	"github.com/Hyaxia/blogwatcher/internal/model"
	"github.com/Hyaxia/blogwatcher/internal/rss"
	"github.com/Hyaxia/blogwatcher/internal/scraper"
	"github.com/Hyaxia/blogwatcher/internal/storage"
)

type ScanResult struct {
	BlogName    string
	NewArticles int
	TotalFound  int
	Source      string
	Error       string
}

func ScanBlog(db *storage.Database, blog model.Blog) ScanResult {
	var (
		articles []model.Article
		source   = "none"
		errText  string
	)

	feedURL := blog.FeedURL
	if feedURL == "" {
		if discovered, err := rss.DiscoverFeedURL(blog.URL, 30*time.Second); err == nil && discovered != "" {
			feedURL = discovered
			blog.FeedURL = discovered
			_ = db.UpdateBlog(blog)
		}
	}

	if feedURL != "" {
		feedArticles, err := rss.ParseFeed(feedURL, 30*time.Second)
		if err != nil {
			errText = err.Error()
		} else {
			articles = convertFeedArticles(blog.ID, feedArticles)
			source = "rss"
		}
	}

	if len(articles) == 0 && blog.ScrapeSelector != "" {
		scrapedArticles, err := scraper.ScrapeBlog(blog.URL, blog.ScrapeSelector, 30*time.Second)
		if err != nil {
			if errText != "" {
				errText = fmt.Sprintf("RSS: %s; Scraper: %s", errText, err.Error())
			} else {
				errText = err.Error()
			}
		} else {
			articles = convertScrapedArticles(blog.ID, scrapedArticles)
			source = "scraper"
			errText = ""
		}
	}

	seenURLs := make(map[string]struct{})
	uniqueArticles := make([]model.Article, 0, len(articles))
	for _, article := range articles {
		if _, exists := seenURLs[article.URL]; exists {
			continue
		}
		seenURLs[article.URL] = struct{}{}
		uniqueArticles = append(uniqueArticles, article)
	}

	urlList := make([]string, 0, len(seenURLs))
	for url := range seenURLs {
		urlList = append(urlList, url)
	}

	existing, err := db.GetExistingArticleURLs(urlList)
	if err != nil {
		errText = err.Error()
	}

	discoveredAt := time.Now()
	newArticles := make([]model.Article, 0, len(uniqueArticles))
	for _, article := range uniqueArticles {
		if _, exists := existing[article.URL]; exists {
			continue
		}
		article.DiscoveredDate = &discoveredAt
		newArticles = append(newArticles, article)
	}

	newCount := 0
	if len(newArticles) > 0 {
		count, err := db.AddArticlesBulk(newArticles)
		if err != nil {
			errText = err.Error()
		} else {
			newCount = count
		}
	}

	_ = db.UpdateBlogLastScanned(blog.ID, time.Now())

	return ScanResult{
		BlogName:    blog.Name,
		NewArticles: newCount,
		TotalFound:  len(seenURLs),
		Source:      source,
		Error:       errText,
	}
}

func ScanAllBlogs(db *storage.Database, workers int) ([]ScanResult, error) {
	blogs, err := db.ListBlogs()
	if err != nil {
		return nil, err
	}
	return scanBlogs(db, blogs, workers)
}

func ScanBlogByName(db *storage.Database, name string) (*ScanResult, error) {
	blog, err := db.GetBlogByName(name)
	if err != nil {
		return nil, err
	}
	if blog == nil {
		return nil, nil
	}
	result := ScanBlog(db, *blog)
	return &result, nil
}

func ScanBlogsByGroup(db *storage.Database, group string, workers int) ([]ScanResult, error) {
	blogs, err := db.ListBlogsByGroup(group)
	if err != nil {
		return nil, err
	}
	return scanBlogs(db, blogs, workers)
}

// scanBlogs runs ScanBlog for each blog, using a worker pool when workers > 1.
// Workers are capped at len(blogs) to avoid idle goroutines.
func scanBlogs(db *storage.Database, blogs []model.Blog, workers int) ([]ScanResult, error) {
	if len(blogs) == 0 {
		return []ScanResult{}, nil
	}

	if workers <= 1 {
		results := make([]ScanResult, 0, len(blogs))
		for _, blog := range blogs {
			results = append(results, ScanBlog(db, blog))
		}
		return results, nil
	}

	// Cap workers to avoid spinning up more goroutines than blogs.
	if workers > len(blogs) {
		workers = len(blogs)
	}

	// Open all worker DB connections upfront so a connection failure doesn't
	// deadlock the jobs channel.
	workerDBs := make([]*storage.Database, workers)
	for i := range workerDBs {
		wdb, err := storage.OpenDatabase(db.Path())
		if err != nil {
			for j := 0; j < i; j++ {
				workerDBs[j].Close()
			}
			return nil, err
		}
		workerDBs[i] = wdb
	}
	defer func() {
		for _, wdb := range workerDBs {
			wdb.Close()
		}
	}()

	type job struct {
		Index int
		Blog  model.Blog
	}
	jobs := make(chan job, len(blogs))
	results := make([]ScanResult, len(blogs))
	done := make(chan struct{}, workers)

	for _, wdb := range workerDBs {
		wdb := wdb
		go func() {
			for item := range jobs {
				results[item.Index] = ScanBlog(wdb, item.Blog)
			}
			done <- struct{}{}
		}()
	}

	for index, blog := range blogs {
		jobs <- job{Index: index, Blog: blog}
	}
	close(jobs)

	for i := 0; i < workers; i++ {
		<-done
	}

	return results, nil
}

func convertFeedArticles(blogID int64, articles []rss.FeedArticle) []model.Article {
	result := make([]model.Article, 0, len(articles))
	for _, article := range articles {
		result = append(result, model.Article{
			BlogID:        blogID,
			Title:         article.Title,
			URL:           article.URL,
			PublishedDate: article.PublishedDate,
			IsRead:        false,
		})
	}
	return result
}

func convertScrapedArticles(blogID int64, articles []scraper.ScrapedArticle) []model.Article {
	result := make([]model.Article, 0, len(articles))
	for _, article := range articles {
		result = append(result, model.Article{
			BlogID:        blogID,
			Title:         article.Title,
			URL:           article.URL,
			PublishedDate: article.PublishedDate,
			IsRead:        false,
		})
	}
	return result
}
