package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/Hyaxia/blogwatcher/internal/controller"
	"github.com/Hyaxia/blogwatcher/internal/model"
	"github.com/Hyaxia/blogwatcher/internal/scanner"
	"github.com/Hyaxia/blogwatcher/internal/storage"
)

func newAddCommand() *cobra.Command {
	var feedURL string
	var scrapeSelector string
	var group string

	cmd := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add a new blog to track.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			url := args[1]
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()
			_, err = controller.AddBlog(db, name, url, feedURL, scrapeSelector, group)
			if err != nil {
				printError(err)
				return markError(err)
			}
			color.New(color.FgGreen).Printf("Added blog '%s'\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&feedURL, "feed-url", "", "RSS/Atom feed URL (auto-discovered if not provided)")
	cmd.Flags().StringVar(&scrapeSelector, "scrape-selector", "", "CSS selector for HTML scraping fallback")
	cmd.Flags().StringVar(&group, "group", "", "Group name for organizing blogs")
	return cmd
}

func newRemoveCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a blog from tracking.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !yes {
				confirmed, err := confirm(fmt.Sprintf("Remove blog '%s' and all its articles?", name))
				if err != nil {
					return err
				}
				if !confirmed {
					return nil
				}
			}
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()
			if err := controller.RemoveBlog(db, name); err != nil {
				printError(err)
				return markError(err)
			}
			color.New(color.FgGreen).Printf("Removed blog '%s'\n", name)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func newBlogsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blogs",
		Short: "List all tracked blogs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()
			blogs, err := db.ListBlogs()
			if err != nil {
				return err
			}
			if len(blogs) == 0 {
				fmt.Println("No blogs tracked yet. Use 'blogwatcher add' to add one.")
				return nil
			}
			color.New(color.FgCyan, color.Bold).Printf("Tracked blogs (%d):\n\n", len(blogs))
			for _, blog := range blogs {
				color.New(color.FgWhite, color.Bold).Printf("  %s\n", blog.Name)
				fmt.Printf("    URL: %s\n", blog.URL)
				if blog.Group != "" {
					fmt.Printf("    Group: %s\n", blog.Group)
				}
				if blog.FeedURL != "" {
					fmt.Printf("    Feed: %s\n", blog.FeedURL)
				}
				if blog.ScrapeSelector != "" {
					fmt.Printf("    Selector: %s\n", blog.ScrapeSelector)
				}
				if blog.LastScanned != nil {
					fmt.Printf("    Last scanned: %s\n", blog.LastScanned.Format("2006-01-02 15:04"))
				}
				fmt.Println()
			}
			return nil
		},
	}
	return cmd
}

func newScanCommand() *cobra.Command {
	var silent bool
	var workers int
	var group string

	cmd := &cobra.Command{
		Use:   "scan [blog_name]",
		Short: "Scan blogs for new articles.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()

			if len(args) == 1 {
				result, err := scanner.ScanBlogByName(db, args[0])
				if err != nil {
					return err
				}
				if result == nil {
					err := fmt.Errorf("Blog '%s' not found", args[0])
					printError(err)
					return markError(err)
				}
				if !silent {
					printScanResult(*result)
				}
			} else if group != "" {
				blogs, err := db.ListBlogsByGroup(group)
				if err != nil {
					return err
				}
				if len(blogs) == 0 {
					fmt.Printf("No blogs found in group '%s'.\n", group)
					return nil
				}
				if !silent {
					color.New(color.FgCyan).Printf("Scanning %d blog(s) in group '%s'...\n\n", len(blogs), group)
				}
				results, err := scanner.ScanBlogsByGroup(db, group, workers)
				if err != nil {
					return err
				}
				totalNew := 0
				for _, result := range results {
					if !silent {
						printScanResult(result)
					}
					totalNew += result.NewArticles
				}
				if !silent {
					fmt.Println()
					if totalNew > 0 {
						color.New(color.FgGreen, color.Bold).Printf("Found %d new article(s) total!\n", totalNew)
					} else {
						color.New(color.FgYellow).Println("No new articles found.")
					}
				}
			} else {
				blogs, err := db.ListBlogs()
				if err != nil {
					return err
				}
				if len(blogs) == 0 {
					fmt.Println("No blogs tracked yet. Use 'blogwatcher add' to add one.")
					return nil
				}
				if !silent {
					color.New(color.FgCyan).Printf("Scanning %d blog(s)...\n\n", len(blogs))
				}
				results, err := scanner.ScanAllBlogs(db, workers)
				if err != nil {
					return err
				}
				totalNew := 0
				for _, result := range results {
					if !silent {
						printScanResult(result)
					}
					totalNew += result.NewArticles
				}
				if !silent {
					fmt.Println()
					if totalNew > 0 {
						color.New(color.FgGreen, color.Bold).Printf("Found %d new article(s) total!\n", totalNew)
					} else {
						color.New(color.FgYellow).Println("No new articles found.")
					}
				}
			}

			if silent {
				fmt.Println("scan done")
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&silent, "silent", "s", false, "Only output 'scan done' when complete")
	cmd.Flags().IntVarP(&workers, "workers", "w", 8, "Number of concurrent workers when scanning all blogs")
	cmd.Flags().StringVarP(&group, "group", "g", "", "Only scan blogs in this group")
	return cmd
}

func newArticlesCommand() *cobra.Command {
	var showAll bool
	var blogName string
	var group string

	cmd := &cobra.Command{
		Use:   "articles",
		Short: "List articles.",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()
			articles, blogNames, blogGroups, err := controller.GetArticles(db, showAll, blogName, group)
			if err != nil {
				printError(err)
				return markError(err)
			}
			if len(articles) == 0 {
				if showAll {
					fmt.Println("No articles found.")
				} else {
					color.New(color.FgGreen).Println("No unread articles!")
				}
				return nil
			}

			label := "Unread articles"
			if showAll {
				label = "All articles"
			}
			color.New(color.FgCyan, color.Bold).Printf("%s (%d):\n\n", label, len(articles))
			for _, article := range articles {
				printArticle(article, blogNames[article.BlogID], blogGroups[article.BlogID])
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "Show all articles (including read)")
	cmd.Flags().StringVarP(&blogName, "blog", "b", "", "Filter by blog name")
	cmd.Flags().StringVarP(&group, "group", "g", "", "Filter by blog group")
	return cmd
}

func newReadCommand() *cobra.Command {
	var markAll bool
	var blogName string
	var yes bool

	cmd := &cobra.Command{
		Use:   "read [article_id]",
		Short: "Mark an article (or all unread articles) as read.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if markAll {
				db, err := storage.OpenDatabase("")
				if err != nil {
					return err
				}
				defer db.Close()

				articles, blogNames, _, err := controller.GetArticles(db, false, blogName, "")
				if err != nil {
					printError(err)
					return markError(err)
				}
				if len(articles) == 0 {
					color.New(color.FgGreen).Println("No unread articles to mark as read.")
					return nil
				}
				if !yes {
					scope := "all blogs"
					if blogName != "" {
						scope = fmt.Sprintf("from '%s'", blogName)
					}
					confirmed, err := confirm(fmt.Sprintf("Mark %d article(s) %s as read?", len(articles), scope))
					if err != nil {
						return err
					}
					if !confirmed {
						return nil
					}
				}
				_ = blogNames
				marked, err := controller.MarkAllArticlesRead(db, blogName)
				if err != nil {
					printError(err)
					return markError(err)
				}
				color.New(color.FgGreen).Printf("Marked %d article(s) as read\n", len(marked))
				return nil
			}

			if len(args) == 0 {
				return fmt.Errorf("article_id is required unless --all is specified")
			}
			articleID, err := parseID(args[0])
			if err != nil {
				return err
			}
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()
			article, err := controller.MarkArticleRead(db, articleID)
			if err != nil {
				printError(err)
				return markError(err)
			}
			if article.IsRead {
				fmt.Printf("Article %d is already marked as read.\n", articleID)
			} else {
				color.New(color.FgGreen).Printf("Marked article %d as read\n", articleID)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&markAll, "all", "a", false, "Mark all unread articles as read")
	cmd.Flags().StringVarP(&blogName, "blog", "b", "", "Only mark articles from this blog (used with --all)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt (used with --all)")
	return cmd
}

func newReadAllCommand() *cobra.Command {
	var blogName string
	var yes bool

	cmd := &cobra.Command{
		Use:   "read-all",
		Short: "Mark all unread articles as read.",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()

			articles, blogNames, _, err := controller.GetArticles(db, false, blogName, "")
			if err != nil {
				printError(err)
				return markError(err)
			}
			if len(articles) == 0 {
				color.New(color.FgGreen).Println("No unread articles to mark as read.")
				return nil
			}

			if !yes {
				scope := "all blogs"
				if blogName != "" {
					scope = fmt.Sprintf("from '%s'", blogName)
				}
				confirmed, err := confirm(fmt.Sprintf("Mark %d article(s) %s as read?", len(articles), scope))
				if err != nil {
					return err
				}
				if !confirmed {
					return nil
				}
			}

			marked, err := controller.MarkAllArticlesRead(db, blogName)
			if err != nil {
				printError(err)
				return markError(err)
			}

			_ = blogNames
			color.New(color.FgGreen).Printf("Marked %d article(s) as read\n", len(marked))
			return nil
		},
	}

	cmd.Flags().StringVarP(&blogName, "blog", "b", "", "Only mark articles from this blog")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func newUnreadCommand() *cobra.Command {
	var markAll bool
	var blogName string
	var group string
	var yes bool

	cmd := &cobra.Command{
		Use:   "unread [article_id]",
		Short: "Mark an article (or all read articles) as unread.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if markAll {
				db, err := storage.OpenDatabase("")
				if err != nil {
					return err
				}
				defer db.Close()

				if !yes {
					scope := "all blogs"
					if group != "" {
						scope = fmt.Sprintf("in group '%s'", group)
					} else if blogName != "" {
						scope = fmt.Sprintf("from '%s'", blogName)
					}
					confirmed, err := confirm(fmt.Sprintf("Mark all read articles %s as unread?", scope))
					if err != nil {
						return err
					}
					if !confirmed {
						return nil
					}
				}

				marked, err := controller.MarkAllArticlesUnread(db, blogName, group)
				if err != nil {
					printError(err)
					return markError(err)
				}
				color.New(color.FgGreen).Printf("Marked %d article(s) as unread\n", len(marked))
				return nil
			}

			if len(args) == 0 {
				return fmt.Errorf("article_id is required unless --all is specified")
			}
			articleID, err := parseID(args[0])
			if err != nil {
				return err
			}
			db, err := storage.OpenDatabase("")
			if err != nil {
				return err
			}
			defer db.Close()
			article, err := controller.MarkArticleUnread(db, articleID)
			if err != nil {
				printError(err)
				return markError(err)
			}
			if !article.IsRead {
				fmt.Printf("Article %d is already marked as unread.\n", articleID)
			} else {
				color.New(color.FgGreen).Printf("Marked article %d as unread\n", articleID)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&markAll, "all", "a", false, "Mark all read articles as unread")
	cmd.Flags().StringVarP(&blogName, "blog", "b", "", "Only mark articles from this blog (used with --all)")
	cmd.Flags().StringVarP(&group, "group", "g", "", "Only mark articles in this group (used with --all)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt (used with --all)")
	return cmd
}

func printScanResult(result scanner.ScanResult) {
	statusColor := color.FgWhite
	if result.NewArticles > 0 {
		statusColor = color.FgGreen
	}
	color.New(color.FgWhite, color.Bold).Printf("  %s\n", result.BlogName)
	if result.Error != "" {
		color.New(color.FgRed).Printf("    Error: %s\n", result.Error)
		return
	}
	if result.Source == "none" {
		color.New(color.FgYellow).Println("    No feed or scraper configured")
		return
	}
	sourceLabel := "HTML"
	if result.Source == "rss" {
		sourceLabel = "RSS"
	}
	fmt.Printf("    Source: %s | Found: %d | ", sourceLabel, result.TotalFound)
	color.New(statusColor).Printf("New: %d\n", result.NewArticles)
}

func printArticle(article model.Article, blogName string, groupName string) {
	status := color.New(color.FgYellow).Sprint("[new]")
	if article.IsRead {
		status = color.New(color.FgHiBlack).Sprint("[read]")
	}
	idStr := color.New(color.FgCyan).Sprintf("[%d]", article.ID)
	fmt.Printf("  %s %s %s\n", idStr, status, article.Title)
	fmt.Printf("       Blog: %s\n", blogName)
	if groupName != "" {
		fmt.Printf("       Group: %s\n", groupName)
	}
	fmt.Printf("       URL: %s\n", article.URL)
	if article.PublishedDate != nil {
		fmt.Printf("       Published: %s\n", article.PublishedDate.Format("2006-01-02"))
	}
	fmt.Println()
}

func printError(err error) {
	color.New(color.FgRed).Printf("Error: %s\n", err.Error())
}

func parseID(value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid article id: %s", value)
	}
	return parsed, nil
}

func confirm(prompt string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/N]: ", prompt)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}

func init() {
	cobra.EnableCommandSorting = false
	cobra.AddTemplateFunc("now", func() string { return time.Now().Format(time.RFC3339) })
}
