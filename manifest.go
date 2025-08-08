package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CrawlManifest represents the complete crawl session data
type CrawlManifest struct {
	Version    string               `json:"version"`
	Metadata   CrawlMetadata        `json:"metadata"`
	Pages      map[string]*PageInfo `json:"pages"`
	Queue      []QueueItem          `json:"queue"`
	Statistics CrawlStatistics      `json:"statistics"`
	Config     CrawlConfig          `json:"config"`
	mutex      sync.RWMutex
}

// CrawlMetadata contains session information
type CrawlMetadata struct {
	SessionID string    `json:"session_id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time,omitempty"`
	Status    string    `json:"status"` // "running", "completed", "interrupted"
	BaseURL   string    `json:"base_url"`
	Domain    string    `json:"domain"`
	OutputDir string    `json:"output_dir"`
}

// PageInfo contains detailed information about each crawled page
type PageInfo struct {
	URL            string            `json:"url"`
	Title          string            `json:"title"`
	ContentHash    string            `json:"content_hash"`
	FileSize       int64             `json:"file_size"`
	FileName       string            `json:"file_name"`
	CrawledAt      time.Time         `json:"crawled_at"`
	LastModified   time.Time         `json:"last_modified,omitempty"`
	ResponseCode   int               `json:"response_code"`
	ContentType    string            `json:"content_type"`
	ProcessingTime int64             `json:"processing_time_ms"`
	LinksFound     []string          `json:"links_found"`
	ExtractedLinks int               `json:"extracted_links"`
	ParentURL      string            `json:"parent_url,omitempty"`
	Depth          int               `json:"depth"`
	Status         string            `json:"status"` // "completed", "failed", "skipped"
	ErrorMessage   string            `json:"error_message,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// QueueItem represents a URL waiting to be crawled
type QueueItem struct {
	URL       string    `json:"url"`
	ParentURL string    `json:"parent_url"`
	Depth     int       `json:"depth"`
	Priority  int       `json:"priority"`
	AddedAt   time.Time `json:"added_at"`
}

// CrawlStatistics tracks overall crawl performance
type CrawlStatistics struct {
	TotalPages      int                 `json:"total_pages"`
	SuccessfulPages int                 `json:"successful_pages"`
	FailedPages     int                 `json:"failed_pages"`
	SkippedPages    int                 `json:"skipped_pages"`
	DuplicatePages  int                 `json:"duplicate_pages"`
	TotalBytes      int64               `json:"total_bytes"`
	AveragePageSize int64               `json:"average_page_size"`
	CrawlDuration   string              `json:"crawl_duration"`
	PagesPerSecond  float64             `json:"pages_per_second"`
	BytesPerSecond  float64             `json:"bytes_per_second"`
	ErrorTypes      map[string]int      `json:"error_types"`
	StatusCodes     map[int]int         `json:"status_codes"`
	ContentTypes    map[string]int      `json:"content_types"`
	ProcessingTimes ProcessingTimeStats `json:"processing_times"`
}

// ProcessingTimeStats tracks processing time metrics
type ProcessingTimeStats struct {
	Min     int64   `json:"min_ms"`
	Max     int64   `json:"max_ms"`
	Average float64 `json:"average_ms"`
	Total   int64   `json:"total_ms"`
}

// CrawlConfig stores the configuration used for the crawl
type CrawlConfig struct {
	MaxPages    int    `json:"max_pages"`
	Parallelism int    `json:"parallelism"`
	Verbose     bool   `json:"verbose"`
	UserAgent   string `json:"user_agent"`
	RateLimit   int    `json:"rate_limit"`
	Timeout     int    `json:"timeout_seconds"`
}

// NewManifest creates a new crawl manifest
func NewManifest(baseURL, domain, outputDir string, config CrawlConfig) *CrawlManifest {
	sessionID := fmt.Sprintf("crawl-%d", time.Now().Unix())

	return &CrawlManifest{
		Version: ManifestVersion,
		Metadata: CrawlMetadata{
			SessionID: sessionID,
			StartTime: time.Now(),
			Status:    "running",
			BaseURL:   baseURL,
			Domain:    domain,
			OutputDir: outputDir,
		},
		Pages: make(map[string]*PageInfo),
		Queue: []QueueItem{},
		Statistics: CrawlStatistics{
			ErrorTypes:   make(map[string]int),
			StatusCodes:  make(map[int]int),
			ContentTypes: make(map[string]int),
		},
		Config: config,
	}
}

// AddPage adds a successfully crawled page to the manifest
func (m *CrawlManifest) AddPage(info *PageInfo) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Pages[info.URL] = info
	m.Statistics.TotalPages++

	if info.Status == "completed" {
		m.Statistics.SuccessfulPages++
		m.Statistics.TotalBytes += info.FileSize
		m.Statistics.StatusCodes[info.ResponseCode]++
		m.Statistics.ContentTypes[info.ContentType]++

		// Update processing times
		if m.Statistics.ProcessingTimes.Min == 0 || info.ProcessingTime < m.Statistics.ProcessingTimes.Min {
			m.Statistics.ProcessingTimes.Min = info.ProcessingTime
		}
		if info.ProcessingTime > m.Statistics.ProcessingTimes.Max {
			m.Statistics.ProcessingTimes.Max = info.ProcessingTime
		}
		m.Statistics.ProcessingTimes.Total += info.ProcessingTime
		m.Statistics.ProcessingTimes.Average = float64(m.Statistics.ProcessingTimes.Total) / float64(m.Statistics.SuccessfulPages)
	} else if info.Status == "failed" {
		m.Statistics.FailedPages++
		if info.ErrorMessage != "" {
			m.Statistics.ErrorTypes[info.ErrorMessage]++
		}
	} else if info.Status == "skipped" {
		m.Statistics.SkippedPages++
	}
}

// AddToQueue adds a URL to the crawl queue
func (m *CrawlManifest) AddToQueue(url, parentURL string, depth, priority int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Queue = append(m.Queue, QueueItem{
		URL:       url,
		ParentURL: parentURL,
		Depth:     depth,
		Priority:  priority,
		AddedAt:   time.Now(),
	})
}

// RemoveFromQueue removes a URL from the queue
func (m *CrawlManifest) RemoveFromQueue(url string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	newQueue := []QueueItem{}
	for _, item := range m.Queue {
		if item.URL != url {
			newQueue = append(newQueue, item)
		}
	}
	m.Queue = newQueue
}

// IsDuplicate checks if content hash already exists
func (m *CrawlManifest) IsDuplicate(contentHash string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, page := range m.Pages {
		if page.ContentHash == contentHash && page.Status == "completed" {
			return true
		}
	}
	return false
}

// GetDuplicatePage returns the original page with the same content hash
func (m *CrawlManifest) GetDuplicatePage(contentHash string) *PageInfo {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, page := range m.Pages {
		if page.ContentHash == contentHash && page.Status == "completed" {
			return page
		}
	}
	return nil
}

// IsVisited checks if a URL has been visited
func (m *CrawlManifest) IsVisited(url string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	_, exists := m.Pages[url]
	return exists
}

// UpdateStatistics updates the final statistics
func (m *CrawlManifest) UpdateStatistics() {
	// Don't lock here as this is called from locked methods

	if m.Statistics.SuccessfulPages > 0 {
		m.Statistics.AveragePageSize = m.Statistics.TotalBytes / int64(m.Statistics.SuccessfulPages)
	}

	duration := time.Since(m.Metadata.StartTime)
	m.Statistics.CrawlDuration = duration.String()

	if duration.Seconds() > 0 {
		m.Statistics.PagesPerSecond = float64(m.Statistics.TotalPages) / duration.Seconds()
		m.Statistics.BytesPerSecond = float64(m.Statistics.TotalBytes) / duration.Seconds()
	}
}

// Complete marks the crawl as completed
func (m *CrawlManifest) Complete() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Metadata.EndTime = time.Now()
	m.Metadata.Status = "completed"
	m.UpdateStatistics()
}

// Save writes the manifest to disk
func (m *CrawlManifest) Save(outputDir string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Update statistics before saving
	m.UpdateStatistics()

	manifestPath := filepath.Join(outputDir, "crawl-manifest.json")
	tempPath := manifestPath + ".tmp"

	// Write to temporary file first
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create manifest file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(m); err != nil {
		return fmt.Errorf("failed to encode manifest: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, manifestPath); err != nil {
		return fmt.Errorf("failed to save manifest: %w", err)
	}

	return nil
}

// LoadManifest reads a manifest from disk
func LoadManifest(outputDir string) (*CrawlManifest, error) {
	manifestPath := filepath.Join(outputDir, "crawl-manifest.json")

	file, err := os.Open(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open manifest: %w", err)
	}
	defer file.Close()

	var manifest CrawlManifest
	decoder := json.NewDecoder(file)

	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	return &manifest, nil
}

// CalculateContentHash generates SHA-256 hash of content
func CalculateContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// GetProgress returns current crawl progress
func (m *CrawlManifest) GetProgress() (completed, total int, percentage float64) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	completed = m.Statistics.TotalPages
	total = m.Config.MaxPages
	if total == 0 {
		total = completed + len(m.Queue)
	}

	if total > 0 {
		percentage = float64(completed) / float64(total) * 100
	}

	return completed, total, percentage
}
