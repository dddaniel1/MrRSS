// Package discovery provides blog discovery functionality
package discovery

import (
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"
)

// Discovery configuration constants
const (
	// MaxConcurrentRSSChecks limits the number of concurrent RSS feed checks
	MaxConcurrentRSSChecks = 8
	// MaxConcurrentPathChecks limits the number of concurrent common path checks
	MaxConcurrentPathChecks = 5
	// HTTPClientTimeout is the timeout for HTTP requests
	HTTPClientTimeout = 15 * time.Second
)

// ProgressCallback is called with progress updates during discovery
type ProgressCallback func(progress Progress)

// Progress represents the current discovery progress
type Progress struct {
	Stage      string `json:"stage"`       // Current stage (e.g., "fetching_homepage", "finding_friend_links", "checking_rss")
	Message    string `json:"message"`     // Human-readable message
	Detail     string `json:"detail"`      // Additional detail (e.g., current URL being checked)
	Current    int    `json:"current"`     // Current item index
	Total      int    `json:"total"`       // Total items to process
	FeedName   string `json:"feed_name"`   // Name of the feed being processed (for batch discovery)
	FoundCount int    `json:"found_count"` // Number of feeds found so far
}

// DiscoveredBlog represents a blog found through friend links
type DiscoveredBlog struct {
	Name           string          `json:"name"`
	Homepage       string          `json:"homepage"`
	RSSFeed        string          `json:"rss_feed"`
	IconURL        string          `json:"icon_url"`
	RecentArticles []RecentArticle `json:"recent_articles"`
}

// FailedCandidate represents a candidate website/feed that could not be resolved into a valid subscription.
type FailedCandidate struct {
	URL    string `json:"url"`
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
}

// RecentArticle represents a recent article with title and date
type RecentArticle struct {
	Title string `json:"title"`
	Date  string `json:"date"` // ISO 8601 format or relative time
}

// Service handles blog discovery operations
type Service struct {
	client     *http.Client
	feedParser *gofeed.Parser
}

// NewService creates a new discovery service
func NewService() *Service {
	feedParser := gofeed.NewParser()
	feedParser.Client = &http.Client{
		Timeout: HTTPClientTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errTooManyRedirects
			}
			return nil
		},
	}

	return &Service{
		client: &http.Client{
			Timeout: HTTPClientTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errTooManyRedirects
				}
				return nil
			},
		},
		feedParser: feedParser,
	}
}
