package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/gocolly/colly/v2"
)

const (
	defaultMaxPages = 5000
	parallelism     = 2
)

// Crawler represents the web crawler with its configuration
type Crawler struct {
	baseURL   string
	domain    string
	outputDir string
	maxPages  int
	pageCount int
	mutex     sync.Mutex
	visited   map[string]bool
	verbose   bool
	useSlug   bool
}

// NewCrawler creates a new crawler instance
func NewCrawler(targetURL, outputDir string, maxPages int, verbose, useSlug bool) (*Crawler, error) {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if outputDir == "" {
		outputDir = strings.ReplaceAll(parsedURL.Host, ".", "_")
	}

	return &Crawler{
		baseURL:   targetURL,
		domain:    parsedURL.Host,
		outputDir: outputDir,
		maxPages:  maxPages,
		visited:   make(map[string]bool),
		verbose:   verbose,
		useSlug:   useSlug,
	}, nil
}

func main() {
	var (
		targetURL = flag.String("url", "", "Target URL to crawl (required)")
		outputDir = flag.String("o", "", "Output directory name (defaults to domain name)")
		maxPages  = flag.Int("max", defaultMaxPages, "Maximum number of pages to crawl")
		verbose   = flag.Bool("v", false, "Verbose logging")
		useSlug   = flag.Bool("slug", false, "Use URL-based filenames instead of numeric sequences")
	)
	flag.Parse()

	if *targetURL == "" {
		fmt.Println("Usage: crawldocs -url=<URL> [-o=<output_dir>] [-max=<max_pages>] [-v] [-slug]")
		fmt.Println("Example: crawldocs -url=https://example.com/docs")
		os.Exit(1)
	}

	crawler, err := NewCrawler(*targetURL, *outputDir, *maxPages, *verbose, *useSlug)
	if err != nil {
		log.Fatal(err)
	}

	if err := crawler.Start(); err != nil {
		log.Fatal("Crawling failed:", err)
	}

	fmt.Printf("Crawling completed! %d pages saved to %s/\n", crawler.pageCount, crawler.outputDir)
}

// Start begins the crawling process
func (c *Crawler) Start() error {
	// Create output directory
	if err := os.MkdirAll(c.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Initialize colly
	collector := colly.NewCollector(
		colly.AllowedDomains(c.domain),
		colly.Async(true),
	)

	// Set up rate limiting
	if err := collector.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: parallelism,
	}); err != nil {
		return fmt.Errorf("failed to set rate limit: %w", err)
	}

	// Set up callbacks
	c.setupCallbacks(collector)

	// Start crawling
	if err := collector.Visit(c.baseURL); err != nil {
		return fmt.Errorf("failed to start crawling: %w", err)
	}

	// Wait for completion
	collector.Wait()

	return nil
}

// setupCallbacks configures the collector callbacks
func (c *Crawler) setupCallbacks(collector *colly.Collector) {
	// Handle HTML pages
	collector.OnHTML("html", func(e *colly.HTMLElement) {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		if c.maxPages > 0 && c.pageCount >= c.maxPages {
			return
		}

		currentURL := e.Request.URL.String()
		if c.visited[currentURL] {
			return
		}

		c.visited[currentURL] = true
		c.pageCount++

		// Extract and save content
		if err := c.savePage(e, currentURL); err != nil {
			log.Printf("Failed to save page %s: %v", currentURL, err)
		}
	})

	// Find and follow links
	collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		c.mutex.Lock()
		shouldContinue := c.maxPages == 0 || c.pageCount < c.maxPages
		c.mutex.Unlock()

		if !shouldContinue {
			return
		}

		link := e.Attr("href")
		absoluteURL := e.Request.AbsoluteURL(link)

		// Only follow links within the same domain
		if c.isValidURL(absoluteURL) {
			_ = e.Request.Visit(absoluteURL)
		}
	})

	// Log requests if verbose
	if c.verbose {
		collector.OnRequest(func(r *colly.Request) {
			fmt.Printf("Visiting: %s\n", r.URL.String())
		})
	}

	// Handle errors
	collector.OnError(func(r *colly.Response, err error) {
		log.Printf("Error visiting %s: %v", r.Request.URL, err)
	})
}

// savePage extracts and saves the page content
func (c *Crawler) savePage(e *colly.HTMLElement, currentURL string) error {
	// Extract text content and clean it up
	content := cleanHTMLSimple(e.DOM.Text())

	// Create filename
	var filename string
	if c.useSlug {
		slug := slugify(currentURL)
		filename = fmt.Sprintf("%s.md", slug)
		// Check for duplicates and append counter if needed
		basePath := filepath.Join(c.outputDir, filename)
		counter := 1
		for {
			if _, err := os.Stat(basePath); os.IsNotExist(err) {
				break
			}
			filename = fmt.Sprintf("%s-%d.md", slug, counter)
			basePath = filepath.Join(c.outputDir, filename)
			counter++
		}
	} else {
		filename = fmt.Sprintf("%04d.md", c.pageCount)
	}
	filePath := filepath.Join(c.outputDir, filename)

	// Get page title
	title := strings.TrimSpace(e.DOM.Find("title").Text())
	if title == "" {
		title = "Untitled"
	}

	// Prepare content with metadata
	finalContent := fmt.Sprintf("# %s\n\nSource: %s\n\n---\n\n%s",
		title,
		currentURL,
		content,
	)

	// Write to file
	if err := os.WriteFile(filePath, []byte(finalContent), 0644); err != nil {
		return err
	}

	if c.verbose {
		fmt.Printf("Saved: %s -> %s\n", currentURL, filename)
	}

	return nil
}

// isValidURL checks if the URL should be crawled
func (c *Crawler) isValidURL(absoluteURL string) bool {
	parsedLink, err := url.Parse(absoluteURL)
	if err != nil {
		return false
	}
	return parsedLink.Host == c.domain
}

// slugify converts a URL to a safe filename
func slugify(urlStr string) string {
	// Parse the URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "index"
	}

	// Get the path and clean it
	path := parsedURL.Path
	if path == "" || path == "/" {
		return "index"
	}

	// Remove leading/trailing slashes
	path = strings.Trim(path, "/")

	// Replace slashes with hyphens
	slug := strings.ReplaceAll(path, "/", "-")

	// Remove or replace unsafe characters
	reg := regexp.MustCompile(`[^a-zA-Z0-9\-_]`)
	slug = reg.ReplaceAllString(slug, "-")

	// Replace multiple hyphens with single hyphen
	reg = regexp.MustCompile(`-+`)
	slug = reg.ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// If empty after cleaning, use index
	if slug == "" {
		return "index"
	}

	// Truncate if too long
	if len(slug) > 200 {
		slug = slug[:200]
	}

	return slug
}
