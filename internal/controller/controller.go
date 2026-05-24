package controller

import (
	"fmt"

	"github.com/Hyaxia/blogwatcher/internal/model"
	"github.com/Hyaxia/blogwatcher/internal/storage"
)

type BlogNotFoundError struct {
	Name string
}

func (e BlogNotFoundError) Error() string {
	return fmt.Sprintf("Blog '%s' not found", e.Name)
}

type BlogAlreadyExistsError struct {
	Field string
	Value string
}

func (e BlogAlreadyExistsError) Error() string {
	return fmt.Sprintf("Blog with %s '%s' already exists", e.Field, e.Value)
}

type ArticleNotFoundError struct {
	ID int64
}

func (e ArticleNotFoundError) Error() string {
	return fmt.Sprintf("Article %d not found", e.ID)
}

func AddBlog(db *storage.Database, name string, url string, feedURL string, scrapeSelector string, group string) (model.Blog, error) {
	if existing, err := db.GetBlogByName(name); err != nil {
		return model.Blog{}, err
	} else if existing != nil {
		return model.Blog{}, BlogAlreadyExistsError{Field: "name", Value: name}
	}
	if existing, err := db.GetBlogByURL(url); err != nil {
		return model.Blog{}, err
	} else if existing != nil {
		return model.Blog{}, BlogAlreadyExistsError{Field: "URL", Value: url}
	}

	blog := model.Blog{
		Name:           name,
		URL:            url,
		FeedURL:        feedURL,
		ScrapeSelector: scrapeSelector,
		Group:          group,
	}
	return db.AddBlog(blog)
}

func RemoveBlog(db *storage.Database, name string) error {
	blog, err := db.GetBlogByName(name)
	if err != nil {
		return err
	}
	if blog == nil {
		return BlogNotFoundError{Name: name}
	}
	_, err = db.RemoveBlog(blog.ID)
	return err
}

func GetArticles(db *storage.Database, showAll bool, blogName string, group string) ([]model.Article, map[int64]string, map[int64]string, error) {
	var blogID *int64
	if blogName != "" {
		blog, err := db.GetBlogByName(blogName)
		if err != nil {
			return nil, nil, nil, err
		}
		if blog == nil {
			return nil, nil, nil, BlogNotFoundError{Name: blogName}
		}
		blogID = &blog.ID
	}

	var articles []model.Article
	if group != "" && blogName == "" {
		// Filter by group: collect articles for all blogs in the group.
		groupBlogs, err := db.ListBlogsByGroup(group)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, b := range groupBlogs {
			bid := b.ID
			arts, err := db.ListArticles(!showAll, &bid)
			if err != nil {
				return nil, nil, nil, err
			}
			articles = append(articles, arts...)
		}
	} else {
		var err error
		articles, err = db.ListArticles(!showAll, blogID)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	blogs, err := db.ListBlogs()
	if err != nil {
		return nil, nil, nil, err
	}
	blogNames := make(map[int64]string)
	blogGroups := make(map[int64]string)
	for _, blog := range blogs {
		blogNames[blog.ID] = blog.Name
		blogGroups[blog.ID] = blog.Group
	}

	return articles, blogNames, blogGroups, nil
}

func MarkArticleRead(db *storage.Database, articleID int64) (model.Article, error) {
	article, err := db.GetArticle(articleID)
	if err != nil {
		return model.Article{}, err
	}
	if article == nil {
		return model.Article{}, ArticleNotFoundError{ID: articleID}
	}
	if !article.IsRead {
		_, err = db.MarkArticleRead(articleID)
		if err != nil {
			return model.Article{}, err
		}
	}
	return *article, nil
}

func MarkAllArticlesRead(db *storage.Database, blogName string) ([]model.Article, error) {
	var blogID *int64
	if blogName != "" {
		blog, err := db.GetBlogByName(blogName)
		if err != nil {
			return nil, err
		}
		if blog == nil {
			return nil, BlogNotFoundError{Name: blogName}
		}
		blogID = &blog.ID
	}

	articles, err := db.ListArticles(true, blogID)
	if err != nil {
		return nil, err
	}

	for _, article := range articles {
		_, err := db.MarkArticleRead(article.ID)
		if err != nil {
			return nil, err
		}
	}

	return articles, nil
}

func MarkArticleUnread(db *storage.Database, articleID int64) (model.Article, error) {
	article, err := db.GetArticle(articleID)
	if err != nil {
		return model.Article{}, err
	}
	if article == nil {
		return model.Article{}, ArticleNotFoundError{ID: articleID}
	}
	if article.IsRead {
		_, err = db.MarkArticleUnread(articleID)
		if err != nil {
			return model.Article{}, err
		}
	}
	return *article, nil
}
