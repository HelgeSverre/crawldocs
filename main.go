package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/allegro/bigcache/v3"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/queue"
)

const (
	defaultMaxPages    = 5000
	defaultParallelism = 10  // Increased from 2
	defaultRateLimit   = 10  // requests per second
	defaultTimeout     = 30  // seconds
	minContentLength   = 100 // minimum content length to save
	ManifestVersion    = "1.1.0"
)

// Crawler represents the enhanced web crawler with manifest support
type Crawler struct {
	baseURL      string
	domain       string
	outputDir    string
	maxPages     int
	pageCount    int32 // Use atomic for thread safety
	parallelism  int
	manifest     *CrawlManifest
	contentCache *bigcache.BigCache // High-performance cache for duplicate detection
	urlBloom     *bloom.BloomFilter // Memory-efficient URL tracking
	urlQueue     *queue.Queue
	collector    *colly.Collector
	verbose      bool

	// Performance metrics
	startTime    time.Time
	bytesWritten int64

	// Worker pool for async file writes
	writeQueue chan writeTask
	writeWg    sync.WaitGroup

	// Pre-compiled regex patterns
	regexOnce     sync.Once
	cleanPatterns *cleaningPatterns
}

// writeTask represents an async file write operation
type writeTask struct {
	filePath string
	content  []byte
	pageInfo *PageInfo
}

// cleaningPatterns holds pre-compiled regex patterns
type cleaningPatterns struct {
	fontFaceRe    *regexp.Regexp
	cssRuleRe     *regexp.Regexp
	cssPropertyRe *regexp.Regexp
	urlRe         *regexp.Regexp
	formatRe      *regexp.Regexp
	cssCommentRe  *regexp.Regexp
	wsRe          *regexp.Regexp
}

// NewCrawler creates a new enhanced crawler instance
func NewCrawler(targetURL, outputDir string, maxPages, rateLimit, parallelism int, verbose bool) (*Crawler, error) {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if outputDir == "" {
		outputDir = strings.ReplaceAll(parsedURL.Host, ".", "_")
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Initialize crawler configuration
	config := CrawlConfig{
		MaxPages:    maxPages,
		Parallelism: parallelism,
		Verbose:     verbose,
		UserAgent:   "CrawlDocs/2.0",
		RateLimit:   rateLimit,
		Timeout:     defaultTimeout,
	}

	// Create or load manifest
	manifest := NewManifest(targetURL, parsedURL.Host, outputDir, config)

	// Check for existing manifest (resume capability)
	if existingManifest, err := LoadManifest(outputDir); err == nil {
		if existingManifest.Metadata.Status == "running" {
			// Resume from previous crawl
			manifest = existingManifest
			log.Println("Resuming previous crawl session:", manifest.Metadata.SessionID)
		}
	}

	// Initialize BigCache with optimized settings
	cacheConfig := bigcache.Config{
		Shards:             1024,
		LifeWindow:         10 * time.Minute,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000 * 10 * 60,
		MaxEntrySize:       500,
		Verbose:            false,
		HardMaxCacheSize:   512, // 512 MB
		OnRemove:           nil,
		OnRemoveWithReason: nil,
	}

	contentCache, err := bigcache.New(context.Background(), cacheConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	// Initialize bloom filter for URL tracking
	// Estimated for 1M URLs with 0.01% false positive rate
	urlBloom := bloom.NewWithEstimates(1000000, 0.0001)

	crawler := &Crawler{
		baseURL:      targetURL,
		domain:       parsedURL.Host,
		outputDir:    outputDir,
		maxPages:     maxPages,
		parallelism:  parallelism,
		manifest:     manifest,
		contentCache: contentCache,
		urlBloom:     urlBloom,
		verbose:      verbose,
		startTime:    time.Now(),
		writeQueue:   make(chan writeTask, parallelism*2),
	}

	// Create optimized HTTP transport with connection pooling
	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       15,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
		ForceAttemptHTTP2:     true,
		DisableKeepAlives:     false,
		ResponseHeaderTimeout: time.Duration(config.Timeout) * time.Second,
	}

	// Create custom HTTP client with the optimized transport
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(config.Timeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Initialize colly with optimized settings
	crawler.collector = colly.NewCollector(
		colly.AllowedDomains(crawler.domain),
		colly.Async(true),
		colly.UserAgent(config.UserAgent),
		colly.MaxDepth(10),
	)

	// Set the custom HTTP client
	crawler.collector.SetClient(httpClient)

	// Set up rate limiting
	limitRule := &colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: crawler.parallelism,
	}

	// Only set delay if rate limit is not 0 (0 means unlimited)
	if rateLimit > 0 {
		limitRule.Delay = time.Second / time.Duration(rateLimit)
	} else {
		// 0 means no delay (unlimited speed)
		limitRule.Delay = 0
	}

	if err := crawler.collector.Limit(limitRule); err != nil {
		return nil, fmt.Errorf("failed to set rate limit: %w", err)
	}

	// Initialize URL queue with priority support
	q, err := queue.New(
		crawler.parallelism*2,
		&queue.InMemoryQueueStorage{MaxSize: 100000},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create queue: %w", err)
	}
	crawler.urlQueue = q

	// Start async write workers
	for i := 0; i < crawler.parallelism/2; i++ {
		go crawler.fileWriteWorker()
	}

	return crawler, nil
}

func main() {
	var (
		targetURL      = flag.String("url", "", "Target URL to crawl (required)")
		targetURLShort = flag.String("u", "", "Target URL to crawl (shorthand for --url)")
		outputDir      = flag.String("output", "", "Output directory name (defaults to domain name)")
		outputDirShort = flag.String("o", "", "Output directory name (shorthand for --output)")
		maxPages       = flag.Int("max-pages", defaultMaxPages, "Maximum number of pages to crawl")
		maxPagesShort  = flag.Int("p", defaultMaxPages, "Maximum number of pages to crawl (shorthand for --max-pages)")
		rateLimit      = flag.Int("rate-limit", defaultRateLimit, "Maximum pages per second")
		rateLimitShort = flag.Int("r", defaultRateLimit, "Maximum pages per second (shorthand for --rate-limit)")
		workers        = flag.Int("workers", defaultParallelism, "Number of concurrent workers")
		workersShort   = flag.Int("w", defaultParallelism, "Number of concurrent workers (shorthand for --workers)")
		verbose        = flag.Bool("verbose", false, "Verbose logging")
		verboseShort   = flag.Bool("v", false, "Verbose logging (shorthand for --verbose)")
		resume         = flag.Bool("resume", false, "Resume a previous crawl session")
		report         = flag.Bool("report", false, "Generate a report from manifest")
		version        = flag.Bool("version", false, "Display version information")
	)
	flag.Parse()

	// Handle version flag
	if *version {
		fmt.Printf("CrawlDocs v%s - Enhanced Documentation Crawler\n", ManifestVersion)
		fmt.Println("https://github.com/HelgeSverre/crawldocs")
		os.Exit(0)
	}

	// Merge short and long flag values (short takes precedence if both provided)
	if *targetURLShort != "" {
		*targetURL = *targetURLShort
	}
	if *outputDirShort != "" {
		*outputDir = *outputDirShort
	}
	if *maxPagesShort != defaultMaxPages {
		*maxPages = *maxPagesShort
	}
	if *rateLimitShort != defaultRateLimit {
		*rateLimit = *rateLimitShort
	}
	if *workersShort != defaultParallelism {
		*workers = *workersShort
	}
	if *verboseShort {
		*verbose = *verboseShort
	}

	// Handle report generation
	if *report {
		if *outputDir == "" {
			fmt.Println("Error: --output/-o flag is required for report generation")
			os.Exit(1)
		}
		if err := generateReport(*outputDir); err != nil {
			log.Fatal("Failed to generate report:", err)
		}
		return
	}

	// Validate required flags for crawling
	if *targetURL == "" && !*resume {
		fmt.Printf("CrawlDocs v%s - Enhanced Documentation Crawler\n", ManifestVersion)
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  crawldocs --url <URL> [--output <dir>] [--max-pages <num>] [--rate-limit <num>] [--workers <num>] [--verbose]")
		fmt.Println("  crawldocs --resume --output <dir> [--verbose]")
		fmt.Println("  crawldocs --report --output <dir>")
		fmt.Println("  crawldocs --version")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  --url, -u         Target URL to crawl (required)")
		fmt.Println("  --output, -o      Output directory (defaults to domain name)")
		fmt.Println("  --max-pages, -p   Maximum pages to crawl (default: 5000)")
		fmt.Println("  --rate-limit, -r  Maximum pages per second (default: 10, 0 = unlimited)")
		fmt.Println("  --workers, -w     Number of concurrent workers (default: 10)")
		fmt.Println("  --verbose, -v     Verbose output")
		fmt.Println("  --resume          Resume a previous crawl")
		fmt.Println("  --report          Generate report from manifest")
		fmt.Println("  --version         Display version information")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  crawldocs --url https://docs.python.org --max-pages 1000")
		fmt.Println("  crawldocs -u https://example.com -r 5 -w 5 -v")
		fmt.Println("  crawldocs --resume --output docs_python_org")
		fmt.Println("  crawldocs --report -o docs_python_org")
		os.Exit(1)
	}

	// Handle resume
	if *resume {
		if *outputDir == "" {
			fmt.Println("Error: --output/-o flag is required for resume")
			os.Exit(1)
		}

		manifest, err := LoadManifest(*outputDir)
		if err != nil {
			log.Fatal("Failed to load manifest for resume:", err)
		}

		*targetURL = manifest.Metadata.BaseURL
		*maxPages = manifest.Config.MaxPages
		logInfo("Resuming crawl of %s", *targetURL)
		logProgress(manifest.Statistics.TotalPages, *maxPages, float64(manifest.Statistics.TotalPages)/float64(*maxPages)*100)
	}

	// Create enhanced crawler
	crawler, err := NewCrawler(*targetURL, *outputDir, *maxPages, *rateLimit, *workers, *verbose)
	if err != nil {
		log.Fatal(err)
	}

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println()
		logInfo("Gracefully shutting down...")

		// Save manifest before exit
		if err := crawler.manifest.Save(crawler.outputDir); err != nil {
			logError("Failed to save manifest on shutdown: %v", err)
		}

		os.Exit(0)
	}()

	// Start crawling
	logInfo("Starting crawl of %s", *targetURL)
	logDim("Output directory: %s", crawler.outputDir)
	logDim("Max pages: %d", *maxPages)
	if *rateLimit == 0 {
		logDim("Rate limit: unlimited")
	} else {
		logDim("Rate limit: %d pages/sec", *rateLimit)
	}
	logDim("Workers: %d", crawler.parallelism)
	fmt.Println()

	if err := crawler.Start(); err != nil {
		log.Fatal("Crawling failed:", err)
	}

	fmt.Println()

	// Calculate performance metrics
	duration := time.Since(crawler.startTime)
	pagesPerSecond := float64(atomic.LoadInt32(&crawler.pageCount)) / duration.Seconds()
	mbWritten := float64(atomic.LoadInt64(&crawler.bytesWritten)) / (1024 * 1024)

	logSuccess("Crawling completed! %d pages saved to %s/",
		atomic.LoadInt32(&crawler.pageCount), crawler.outputDir)
	logInfo("Performance: %.2f pages/sec, %.2f MB written", pagesPerSecond, mbWritten)
	logInfo("View the manifest at: %s/crawl-manifest.json", crawler.outputDir)
}

// fileWriteWorker processes async file writes
func (c *Crawler) fileWriteWorker() {
	c.writeWg.Add(1)
	defer c.writeWg.Done()

	for task := range c.writeQueue {
		// Write file
		if err := os.WriteFile(task.filePath, task.content, 0644); err != nil {
			logError("Failed to write file %s: %v", task.filePath, err)
			continue
		}

		// Update manifest
		if task.pageInfo != nil {
			c.manifest.AddPage(task.pageInfo)
		}

		// Update bytes written
		atomic.AddInt64(&c.bytesWritten, int64(len(task.content)))
	}
}

// Start begins the crawling process
func (c *Crawler) Start() error {
	// Set up callbacks
	c.setupCallbacks()

	// Visit the initial URL
	if err := c.collector.Visit(c.baseURL); err != nil {
		return fmt.Errorf("failed to visit initial URL: %w", err)
	}

	// Wait for collector to finish
	c.collector.Wait()

	// Wait for all writes to complete
	close(c.writeQueue)
	c.writeWg.Wait()

	// Update final statistics
	c.manifest.Complete()
	if err := c.manifest.Save(c.outputDir); err != nil {
		logError("Failed to save final manifest: %v", err)
	}

	// Clean up cache
	if err := c.contentCache.Close(); err != nil && c.verbose {
		logError("Failed to close cache: %v", err)
	}

	return nil
}

// setupCallbacks configures the collector callbacks
func (c *Crawler) setupCallbacks() {
	// Handle response headers to check content type
	c.collector.OnResponse(func(r *colly.Response) {
		contentType := r.Headers.Get("Content-Type")

		// Check if content type is HTML
		if contentType != "" && !strings.Contains(strings.ToLower(contentType), "text/html") {
			// Skip non-HTML content
			c.manifest.AddPage(&PageInfo{
				URL:          r.Request.URL.String(),
				Status:       "skipped",
				ErrorMessage: fmt.Sprintf("non-HTML content type: %s", contentType),
				ResponseCode: r.StatusCode,
				ContentType:  contentType,
				CrawledAt:    time.Now(),
			})

			if c.verbose {
				logSkip("Non-HTML content (%s): %s", contentType, r.Request.URL)
			}

			// Don't process further
			r.Request.Abort()
			return
		}
	})

	// Handle HTML pages
	c.collector.OnHTML("html", func(e *colly.HTMLElement) {
		startTime := time.Now()
		currentURL := e.Request.URL.String()

		// Check if already visited (using bloom filter first for speed)
		if c.urlBloom.Test([]byte(currentURL)) && c.manifest.IsVisited(currentURL) {
			return
		}

		// Add to bloom filter for fast lookups
		c.urlBloom.Add([]byte(currentURL))

		// Check page limit
		if c.maxPages > 0 && atomic.LoadInt32(&c.pageCount) >= int32(c.maxPages) {
			return
		}

		// Extract and save content
		if err := c.savePage(e, currentURL); err != nil {
			logError("Failed to save page %s: %v", currentURL, err)

			// Add failed page to manifest
			c.manifest.AddPage(&PageInfo{
				URL:            currentURL,
				Status:         "failed",
				ErrorMessage:   err.Error(),
				CrawledAt:      time.Now(),
				ProcessingTime: time.Since(startTime).Milliseconds(),
			})
		}
	})

	// Find and follow links
	c.collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		// Check page limit
		if c.maxPages > 0 && atomic.LoadInt32(&c.pageCount) >= int32(c.maxPages) {
			return
		}

		link := e.Attr("href")
		absoluteURL := e.Request.AbsoluteURL(link)

		// Only follow links within the same domain
		// Check bloom filter first for performance, then manifest
		if c.isValidURL(absoluteURL) && !c.urlBloom.Test([]byte(absoluteURL)) {
			if err := e.Request.Visit(absoluteURL); err != nil {
				if !isAlreadyVisitedError(err) && c.verbose {
					logError("Failed to queue URL %s: %v", absoluteURL, err)
				}
			}
		}
	})

	// Log requests if verbose
	if c.verbose {
		c.collector.OnRequest(func(r *colly.Request) {
			logVisit(r.URL.String())
		})
	}

	// Handle errors
	c.collector.OnError(func(r *colly.Response, err error) {
		// Check if it's an "already visited" error
		if isAlreadyVisitedError(err) {
			if c.verbose {
				logSkip("Already visited: %s", r.Request.URL)
			}
			return
		}

		logError("Failed to visit %s: %v", r.Request.URL, err)

		// Add error to manifest
		c.manifest.AddPage(&PageInfo{
			URL:          r.Request.URL.String(),
			Status:       "failed",
			ErrorMessage: err.Error(),
			ResponseCode: r.StatusCode,
			CrawledAt:    time.Now(),
		})
	})

	// Progress updates
	c.collector.OnScraped(func(r *colly.Response) {
		if atomic.LoadInt32(&c.pageCount)%10 == 0 {
			completed, total, percentage := c.manifest.GetProgress()
			logProgress(completed, total, percentage)
		}
	})
}

// savePage extracts and saves the page content
func (c *Crawler) savePage(e *colly.HTMLElement, currentURL string) error {
	startTime := time.Now()
	statusCode := e.Response.StatusCode

	// Check HTTP status code first
	if statusCode != 200 {
		statusText := getHTTPStatusText(statusCode)
		reason := fmt.Sprintf("%d %s", statusCode, statusText)

		// Add skipped/failed page to manifest
		pageStatus := "skipped"
		if statusCode >= 400 {
			pageStatus = "failed"
		}

		c.manifest.AddPage(&PageInfo{
			URL:            currentURL,
			Status:         pageStatus,
			ErrorMessage:   reason,
			ResponseCode:   statusCode,
			CrawledAt:      time.Now(),
			ProcessingTime: time.Since(startTime).Milliseconds(),
		})

		// Log based on status code
		if c.verbose {
			switch {
			case statusCode == 429:
				logRateLimit("%s: %s", reason, currentURL)
			case statusCode >= 300 && statusCode < 400:
				logSkip("%s: %s", reason, currentURL)
			case statusCode >= 400:
				logError("%s: %s", reason, currentURL)
			}
		}
		return nil
	}

	// Extract text content for 200 responses
	// First try to find main content areas
	var rawContent string
	var contentSource string

	// Try to extract from main content areas first
	mainContent := e.DOM.Find("main, article, [role='main'], .content, #content").First()
	if mainContent.Length() > 0 {
		rawContent = mainContent.Text()
		contentSource = "main content area"
	} else {
		// Fallback to full page text
		rawContent = e.DOM.Text()
		contentSource = "full page"
	}

	// Extract page metadata
	metaDescription := e.DOM.Find("meta[name='description']").AttrOr("content", "")
	ogDescription := e.DOM.Find("meta[property='og:description']").AttrOr("content", "")

	// Check for potential SPA indicators
	reactRoot := e.DOM.Find("#root, #app, [data-react-root], [data-reactroot]").Length() > 0
	vueApp := e.DOM.Find("#app[data-v-], [id^='__nuxt'], [id^='__next']").Length() > 0
	angularApp := e.DOM.Find("[ng-app], [data-ng-app], app-root").Length() > 0

	if (reactRoot || vueApp || angularApp) && c.verbose {
		logWarn("Potential SPA detected - content may be loaded dynamically via JavaScript")
	}

	// Combine content with metadata for better uniqueness
	fullContent := rawContent
	if metaDescription != "" {
		fullContent = metaDescription + "\n\n" + fullContent
	} else if ogDescription != "" {
		fullContent = ogDescription + "\n\n" + fullContent
	}

	// Simple content length validation
	validation := validateContent(fullContent, currentURL)

	if c.verbose {
		logDim("Content source: %s, Raw length: %d, Cleaned length: %d",
			contentSource, len(rawContent), len(validation.CleanedContent))
	}

	if !validation.IsValid {
		// Add skipped page to manifest
		c.manifest.AddPage(&PageInfo{
			URL:            currentURL,
			Status:         "skipped",
			ErrorMessage:   "minimal content",
			ResponseCode:   statusCode,
			CrawledAt:      time.Now(),
			ProcessingTime: time.Since(startTime).Milliseconds(),
		})

		if c.verbose {
			logSkip("Minimal content: %s", currentURL)
		}
		return nil
	}

	// Get page title first
	title := strings.TrimSpace(e.DOM.Find("title").Text())
	if title == "" {
		title = "Untitled"
	}

	// Calculate content hash - include title and URL path to make it more unique
	urlPath := e.Request.URL.Path
	if urlPath == "" {
		urlPath = "/"
	}

	// For very short content, include more context to avoid false duplicates
	var contentForHash string
	if len(validation.CleanedContent) < 500 {
		// Short content - include URL and title to differentiate
		contentForHash = fmt.Sprintf("URL:%s\nTITLE:%s\n%s", urlPath, title, validation.CleanedContent)
	} else {
		// Normal content - just use the content
		contentForHash = validation.CleanedContent
	}

	contentHash := CalculateContentHash(contentForHash)

	// Check for duplicates using BigCache first (faster)
	if cachedURL, err := c.contentCache.Get(contentHash); err == nil {
		// Found in cache, might be a duplicate
		originalPage := c.manifest.GetDuplicatePage(contentHash)

		// Only mark as duplicate if content is substantial (not just template)
		if len(validation.CleanedContent) >= 500 {
			duplicateMsg := "duplicate content"
			fileName := "unknown"

			// originalPage might be nil if it's still in the batch queue
			if originalPage != nil {
				duplicateMsg = fmt.Sprintf("duplicate of %s", originalPage.URL)
				fileName = originalPage.FileName
			} else {
				// Use the cached URL as fallback
				duplicateMsg = fmt.Sprintf("duplicate of %s", string(cachedURL))
			}

			// Add duplicate to manifest
			c.manifest.AddPage(&PageInfo{
				URL:            currentURL,
				Status:         "skipped",
				ErrorMessage:   duplicateMsg,
				ContentHash:    contentHash,
				CrawledAt:      time.Now(),
				ProcessingTime: time.Since(startTime).Milliseconds(),
			})

			c.manifest.Statistics.DuplicatePages++

			if c.verbose {
				logSkip("Duplicate of %s: %s (content length: %d)",
					fileName, currentURL, len(validation.CleanedContent))
			}
			return nil
		} else if c.verbose && originalPage != nil {
			logDim("Similar template content to %s, but too short to be duplicate (%d chars)",
				originalPage.FileName, len(validation.CleanedContent))
		}
	}

	// Add to cache for future duplicate detection
	cacheErr := c.contentCache.Set(contentHash, []byte(currentURL))
	if cacheErr != nil && c.verbose {
		logError("Failed to cache content hash: %v", cacheErr)
	}

	// Extract links
	var linksFound []string
	e.DOM.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			absoluteURL := e.Request.AbsoluteURL(href)
			if c.isValidURL(absoluteURL) {
				linksFound = append(linksFound, absoluteURL)
			}
		}
	})

	// Create filename using slug
	atomic.AddInt32(&c.pageCount, 1)
	slug := slugify(currentURL)
	filename := fmt.Sprintf("%s.md", slug)

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
	filePath := basePath

	// Prepare content with metadata
	finalContent := fmt.Sprintf("# %s\n\nSource: %s\n\n---\n\n%s",
		title,
		currentURL,
		validation.CleanedContent,
	)

	// Create page info
	pageInfo := &PageInfo{
		URL:            currentURL,
		Title:          title,
		ContentHash:    contentHash,
		FileSize:       int64(len(finalContent)),
		FileName:       filename,
		CrawledAt:      time.Now(),
		ResponseCode:   e.Response.StatusCode,
		ContentType:    e.Response.Headers.Get("Content-Type"),
		ProcessingTime: time.Since(startTime).Milliseconds(),
		LinksFound:     linksFound,
		ExtractedLinks: len(linksFound),
		Status:         "completed",
	}

	// Queue async write
	c.writeQueue <- writeTask{
		filePath: filePath,
		content:  []byte(finalContent),
		pageInfo: pageInfo,
	}

	if c.verbose {
		logSuccess("Saved %s -> %s (%d bytes)", currentURL, filename, len(finalContent))
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

// generateReport creates a report from the manifest
func generateReport(outputDir string) error {
	manifest, err := LoadManifest(outputDir)
	if err != nil {
		return err
	}

	fmt.Println("\n=== Crawl Report ===")
	fmt.Printf("Session ID: %s\n", manifest.Metadata.SessionID)
	fmt.Printf("Base URL: %s\n", manifest.Metadata.BaseURL)
	fmt.Printf("Status: %s\n", manifest.Metadata.Status)
	fmt.Printf("Duration: %s\n", manifest.Statistics.CrawlDuration)
	fmt.Println("\n--- Statistics ---")
	fmt.Printf("Total Pages: %d\n", manifest.Statistics.TotalPages)
	fmt.Printf("Successful: %d\n", manifest.Statistics.SuccessfulPages)
	fmt.Printf("Failed: %d\n", manifest.Statistics.FailedPages)
	fmt.Printf("Skipped: %d\n", manifest.Statistics.SkippedPages)
	fmt.Printf("Duplicates: %d\n", manifest.Statistics.DuplicatePages)
	fmt.Printf("Total Size: %.2f MB\n", float64(manifest.Statistics.TotalBytes)/1024/1024)
	fmt.Printf("Avg Page Size: %.2f KB\n", float64(manifest.Statistics.AveragePageSize)/1024)
	fmt.Printf("Pages/Second: %.2f\n", manifest.Statistics.PagesPerSecond)

	if len(manifest.Statistics.ErrorTypes) > 0 {
		fmt.Println("\n--- Error Summary ---")
		for errType, count := range manifest.Statistics.ErrorTypes {
			fmt.Printf("%s: %d\n", errType, count)
		}
	}

	fmt.Println("\n--- Status Codes ---")
	for code, count := range manifest.Statistics.StatusCodes {
		fmt.Printf("%d: %d\n", code, count)
	}

	return nil
}
