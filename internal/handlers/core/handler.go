// Package core contains the main Handler struct and core HTTP handlers for the application.
// It defines the Handler struct which holds dependencies like the database and fetcher.
package core

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"MrRSS/internal/aiusage"
	"MrRSS/internal/cache"
	"MrRSS/internal/database"
	"MrRSS/internal/discovery"
	"MrRSS/internal/feed"
	"MrRSS/internal/models"
	"MrRSS/internal/statistics"
	"MrRSS/internal/translation"
	"MrRSS/internal/utils"

	"codeberg.org/readeck/go-readability/v2"

	"github.com/mmcdole/gofeed"
)

// Discovery timeout constants
const (
	// SingleFeedDiscoveryTimeout is the timeout for discovering feeds from a single source
	SingleFeedDiscoveryTimeout = 90 * time.Second
	// BatchDiscoveryTimeout is the timeout for discovering feeds from all sources
	BatchDiscoveryTimeout = 5 * time.Minute
)

// DiscoveryState represents the current state of a discovery operation
type DiscoveryState struct {
	IsRunning        bool                        `json:"is_running"`
	Progress         discovery.Progress          `json:"progress"`
	Feeds            []discovery.DiscoveredBlog  `json:"feeds,omitempty"`
	FailedCandidates []discovery.FailedCandidate `json:"failed_candidates,omitempty"`
	Error            string                      `json:"error,omitempty"`
	IsComplete       bool                        `json:"is_complete"`
}

// Handler holds all dependencies for HTTP handlers.
type Handler struct {
	DB               *database.DB
	Fetcher          *feed.Fetcher
	Translator       translation.Translator
	AITracker        *aiusage.Tracker
	DiscoveryService *discovery.Service
	App              interface{}         // Wails app instance for browser integration (interface{} to avoid import in server mode)
	ContentCache     *cache.ContentCache // Cache for article content
	Stats            *statistics.Service // Statistics tracking service

	// Discovery state tracking for polling-based progress
	DiscoveryMu          sync.RWMutex
	SingleDiscoveryState *DiscoveryState
	RecommendState       *DiscoveryState
	BatchDiscoveryState  *DiscoveryState
}

// NewHandler creates a new Handler with the given dependencies.
func NewHandler(db *database.DB, fetcher *feed.Fetcher, translator translation.Translator) *Handler {
	h := &Handler{
		DB:               db,
		Fetcher:          fetcher,
		Translator:       translator,
		AITracker:        aiusage.NewTracker(db),
		DiscoveryService: discovery.NewService(),
		ContentCache:     cache.NewContentCache(100, 30*time.Minute), // Cache up to 100 articles for 30 minutes
		Stats:            statistics.NewService(db),
	}

	return h
}

// CallAppMethod calls a method on the Wails app instance if available
func (h *Handler) CallAppMethod(method string, args ...interface{}) error {
	if h.App == nil {
		return fmt.Errorf("app instance not set")
	}

	// Use reflection or type assertion to call the method
	// This is a simplified version - you may need to adjust based on your actual Wails app structure
	// For now, just log that we want to call this method
	log.Printf("Would call app method: %s with args: %v", method, args)
	return nil
}

// SetApp sets the Wails application instance for browser integration.
// This is called after app initialization in main.go.
func (h *Handler) SetApp(app interface{}) {
	h.App = app
}

// Statistics returns the statistics service
func (h *Handler) Statistics() *statistics.Service {
	return h.Stats
}

// GetArticleContent fetches article content with caching
// Returns (content, wasCached, error)
func (h *Handler) GetArticleContent(articleID int64) (string, bool, error) {
	// First, check database cache (persistent cache)
	content, found, err := h.DB.GetArticleContent(articleID)
	if err == nil && found {
		// Also populate memory cache for faster subsequent access
		h.ContentCache.Set(articleID, content)
		return content, true, nil
	}

	// Check memory cache (in-memory cache, might be stale but fast)
	if content, found := h.ContentCache.Get(articleID); found {
		return content, true, nil
	}

	// Get the article from database
	article, err := h.DB.GetArticleByID(articleID)
	if err != nil {
		return "", false, err
	}

	// Get the feed
	targetFeed, err := h.DB.GetFeedByID(article.FeedID)
	if err != nil {
		return "", false, err
	}

	if targetFeed == nil {
		return "", false, nil
	}

	// Trigger immediate feed refresh using the new task manager
	// This bypasses the queue and pool limits
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch the feed immediately (article click triggered)
	h.Fetcher.FetchFeedForArticle(ctx, *targetFeed)

	// Parse the feed to get fresh content
	parsedFeed, err := h.Fetcher.ParseFeedWithFeed(ctx, targetFeed, true) // High priority for content fetching
	if err != nil {
		return "", false, err
	}

	// Cache the feed for future use
	h.ContentCache.SetFeed(targetFeed.ID, parsedFeed)

	// Find the article in the feed by multiple criteria for better matching
	matchingItem := h.findMatchingFeedItem(article, parsedFeed.Items)
	if matchingItem != nil {
		content := feed.ExtractContent(matchingItem)
		cleanContent := utils.CleanHTML(content)

		// Cache the content in both memory and database
		h.ContentCache.Set(articleID, cleanContent)
		if err := h.DB.SetArticleContent(articleID, cleanContent); err != nil {
			log.Printf("Error caching content to database: %v", err)
		}

		return cleanContent, false, nil
	}

	return "", false, nil
}

// FetchFullArticleContent fetches the full article content from the original URL using readability.
func (h *Handler) FetchFullArticleContent(articleURL string) (string, error) {
	parsedURL, err := url.Parse(articleURL)
	if err != nil {
		return "", fmt.Errorf("invalid article URL: %w", err)
	}

	proxyURL, err := h.getGlobalProxyURL()
	if err != nil {
		return "", fmt.Errorf("read proxy settings: %w", err)
	}

	httpClient, err := utils.CreateHTTPClientWithUserAgent(
		proxyURL,
		30*time.Second,
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	)
	if err != nil {
		return "", fmt.Errorf("create HTTP client: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request article URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request article URL: HTTP %d", resp.StatusCode)
	}

	article, err := readability.FromReader(resp.Body, parsedURL)
	if err != nil {
		return "", fmt.Errorf("readability parse: %w", err)
	}

	// Render the article content as HTML
	var buf bytes.Buffer
	err = article.RenderHTML(&buf)
	if err != nil {
		return "", fmt.Errorf("render HTML: %w", err)
	}

	return buf.String(), nil
}

func (h *Handler) getGlobalProxyURL() (string, error) {
	proxyEnabled, err := h.DB.GetSetting("proxy_enabled")
	if err != nil || proxyEnabled != "true" {
		return "", nil
	}

	proxyType, _ := h.DB.GetSetting("proxy_type")
	proxyHost, _ := h.DB.GetSetting("proxy_host")
	proxyPort, _ := h.DB.GetSetting("proxy_port")
	proxyUsername, _ := h.DB.GetEncryptedSetting("proxy_username")
	proxyPassword, _ := h.DB.GetEncryptedSetting("proxy_password")

	return utils.BuildProxyURL(proxyType, proxyHost, proxyPort, proxyUsername, proxyPassword), nil
}

// findMatchingFeedItem finds the best matching feed item for an article using multiple criteria
func (h *Handler) findMatchingFeedItem(article *models.Article, items []*gofeed.Item) *gofeed.Item {
	urlMatches := make([]*gofeed.Item, 0, len(items))
	for _, item := range items {
		if h.itemMatchesArticleURL(item, article.URL) {
			urlMatches = append(urlMatches, item)
		}
	}

	// First pass: URL/GUID + title + published time match (strongest signal)
	strongMatches := make([]*gofeed.Item, 0, len(urlMatches))
	for _, item := range urlMatches {
		if h.titlesMatch(item.Title, article.Title) && h.publishedTimesMatch(item.PublishedParsed, &article.PublishedAt) {
			strongMatches = append(strongMatches, item)
		}
	}
	if len(strongMatches) == 1 {
		return strongMatches[0]
	}
	if len(strongMatches) > 1 {
		if item := h.findClosestByPublishedTime(article, strongMatches); item != nil {
			return item
		}
		log.Printf("Ambiguous strong feed item match for article_id=%d, candidates=%d", article.ID, len(strongMatches))
		return nil
	}

	// Second pass: URL/GUID + title match (must be unique to avoid wrong writes)
	titleURLMatches := make([]*gofeed.Item, 0, len(urlMatches))
	for _, item := range urlMatches {
		if h.titlesMatch(item.Title, article.Title) {
			titleURLMatches = append(titleURLMatches, item)
		}
	}
	if len(titleURLMatches) == 1 {
		return titleURLMatches[0]
	}
	if len(titleURLMatches) > 1 {
		if item := h.findClosestByPublishedTime(article, titleURLMatches); item != nil {
			return item
		}
		log.Printf("Ambiguous URL+title feed item match for article_id=%d, candidates=%d", article.ID, len(titleURLMatches))
		return nil
	}

	// Third pass: URL/GUID + closest published time (must be uniquely closest and time-near)
	if item := h.findClosestByPublishedTime(article, urlMatches); item != nil {
		return item
	}

	// Fourth pass: title + published time match (fallback for when URL/GUID don't match; must be unique)
	titleTimeMatches := make([]*gofeed.Item, 0, len(items))
	for _, item := range items {
		if h.titlesMatch(item.Title, article.Title) && h.publishedTimesMatch(item.PublishedParsed, &article.PublishedAt) {
			titleTimeMatches = append(titleTimeMatches, item)
		}
	}
	if len(titleTimeMatches) == 1 {
		return titleTimeMatches[0]
	}
	if len(titleTimeMatches) > 1 {
		if item := h.findClosestByPublishedTime(article, titleTimeMatches); item != nil {
			return item
		}
		log.Printf("Ambiguous title+time feed item match for article_id=%d, candidates=%d", article.ID, len(titleTimeMatches))
		return nil
	}

	return nil
}

func (h *Handler) itemMatchesArticleURL(item *gofeed.Item, articleURL string) bool {
	if articleURL == "" {
		return false
	}

	if utils.URLsMatch(item.Link, articleURL) {
		return true
	}

	if item.GUID != "" && utils.URLsMatch(item.GUID, articleURL) {
		return true
	}

	return false
}

func (h *Handler) findClosestByPublishedTime(article *models.Article, candidates []*gofeed.Item) *gofeed.Item {
	if len(candidates) == 0 || article.PublishedAt.IsZero() {
		return nil
	}

	var best *gofeed.Item
	var bestDiff time.Duration
	isTie := false

	for _, item := range candidates {
		if item.PublishedParsed == nil {
			continue
		}

		diff := item.PublishedParsed.Sub(article.PublishedAt)
		if diff < 0 {
			diff = -diff
		}

		if best == nil || diff < bestDiff {
			best = item
			bestDiff = diff
			isTie = false
		} else if diff == bestDiff {
			isTie = true
		}
	}

	if best == nil || isTie {
		return nil
	}

	// Reject far-away matches to avoid cross-item contamination on noisy feeds.
	if bestDiff > 30*time.Minute {
		return nil
	}

	return best
}

// titlesMatch checks if two titles match, allowing for minor differences
func (h *Handler) titlesMatch(title1, title2 string) bool {
	if title1 == title2 {
		return true
	}
	// Normalize titles by removing extra whitespace and comparing
	normalized1 := strings.TrimSpace(strings.Join(strings.Fields(title1), " "))
	normalized2 := strings.TrimSpace(strings.Join(strings.Fields(title2), " "))
	return normalized1 == normalized2
}

// publishedTimesMatch checks if two published times match within a reasonable tolerance
func (h *Handler) publishedTimesMatch(time1, time2 *time.Time) bool {
	if time1 == nil || time2 == nil {
		return false
	}
	// Allow for 1 minute difference in published times
	diff := time1.Sub(*time2)
	if diff < 0 {
		diff = -diff
	}
	return diff <= time.Minute
}
