package discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

// discoverRSSFeeds discovers RSS feeds from a list of blog URLs
func (s *Service) discoverRSSFeeds(ctx context.Context, blogURLs []string) []DiscoveredBlog {
	return s.discoverRSSFeedsWithProgress(ctx, blogURLs, nil)
}

// discoverRSSFeedsWithProgress discovers RSS feeds with progress updates
func (s *Service) discoverRSSFeedsWithProgress(ctx context.Context, blogURLs []string, progressCb ProgressCallback) []DiscoveredBlog {
	discovered, _ := s.discoverRSSFeedsWithProgressDetailed(ctx, blogURLs, progressCb)
	return discovered
}

// discoverRSSFeedsWithProgressDetailed discovers RSS feeds and records per-candidate failures.
func (s *Service) discoverRSSFeedsWithProgressDetailed(ctx context.Context, blogURLs []string, progressCb ProgressCallback) ([]DiscoveredBlog, []FailedCandidate) {
	var wg sync.WaitGroup
	results := make(chan DiscoveredBlog, len(blogURLs))
	failures := make(chan FailedCandidate, len(blogURLs))
	sem := make(chan struct{}, MaxConcurrentRSSChecks)

	// Track progress
	var progressMu sync.Mutex
	processed := 0
	foundCount := 0
	total := len(blogURLs)

OuterLoop:
	for _, blogURL := range blogURLs {
		select {
		case <-ctx.Done():
			break OuterLoop
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(u string) {
			defer wg.Done()
			defer func() { <-sem }()

			// Report progress
			if progressCb != nil {
				progressMu.Lock()
				processed++
				currentProcessed := processed
				currentFound := foundCount
				progressMu.Unlock()

				progressCb(Progress{
					Stage:      "checking_rss",
					Message:    "Checking RSS feed",
					Detail:     u,
					Current:    currentProcessed,
					Total:      total,
					FoundCount: currentFound,
				})
			}

			if blog, err := s.discoverBlogRSS(ctx, u); err == nil {
				progressMu.Lock()
				foundCount++
				progressMu.Unlock()
				results <- blog
			} else {
				// Extract debug info from error message
				candidate := FailedCandidate{URL: u, Stage: "checking_rss", Reason: err.Error()}
				// Try to extract attempted URLs and detected feed from error
				errStr := err.Error()
				if strings.Contains(errStr, "attempted_urls=") {
					idx := strings.Index(errStr, "attempted_urls=")
					if idx >= 0 {
						start := idx + len("attempted_urls=")
						end := strings.Index(errStr[start:], ";")
						if end > 0 {
							candidate.AttemptedURLs = strings.Split(errStr[start:start+end], ",")
						}
					}
				}
				if strings.Contains(errStr, "detected_feed=") {
					idx := strings.Index(errStr, "detected_feed=")
					if idx >= 0 {
						start := idx + len("detected_feed=")
						candidate.DetectedFeedURL = errStr[start:]
						if semIdx := strings.Index(candidate.DetectedFeedURL, ";"); semIdx > 0 {
							candidate.DetectedFeedURL = candidate.DetectedFeedURL[:semIdx]
						}
					}
				}
				failures <- candidate
			}
		}(blogURL)
	}

	go func() {
		wg.Wait()
		close(results)
		close(failures)
	}()

	var discovered []DiscoveredBlog
	for blog := range results {
		discovered = append(discovered, blog)
	}
	var failed []FailedCandidate
	for failure := range failures {
		failed = append(failed, failure)
	}

	return discovered, failed
}

// discoverBlogRSS discovers RSS feed for a single blog
func (s *Service) discoverBlogRSS(ctx context.Context, blogURL string) (DiscoveredBlog, error) {
	// Try to find RSS feed URL
	rssURL, debugInfo, err := s.findRSSFeed(ctx, blogURL)
	if err != nil {
		// Include debug info in error message for richer failure reporting
		return DiscoveredBlog{}, fmt.Errorf("%s; attempted_urls=%s; detected_feed=%s", err.Error(), debugInfo.attemptedURLs, debugInfo.detectedFeed)
	}

	// Parse the RSS feed to get blog info
	feed, err := s.feedParser.ParseURLWithContext(rssURL, ctx)
	if err != nil {
		return DiscoveredBlog{}, fmt.Errorf("parse feed: %w", err)
	}

	// Extract recent articles (max 3)
	var recentArticles []RecentArticle
	for i := 0; i < len(feed.Items) && i < 3; i++ {
		item := feed.Items[i]
		dateStr := ""
		if item.PublishedParsed != nil {
			// Format as relative time or date
			dateStr = item.PublishedParsed.Format("2006-01-02")
		}
		recentArticles = append(recentArticles, RecentArticle{
			Title: item.Title,
			Date:  dateStr,
		})
	}

	// Get favicon
	iconURL := s.getFavicon(blogURL)

	return DiscoveredBlog{
		Name:           feed.Title,
		Homepage:       blogURL,
		RSSFeed:        rssURL,
		IconURL:        iconURL,
		RecentArticles: recentArticles,
	}, nil
}

// rssFeedDebug holds debug information during RSS feed discovery
type rssFeedDebug struct {
	attemptedURLs  []string // All URLs that were tried
	detectedFeed   string   // Feed URL detected from HTML (if any)
	validateErrors []string // Errors encountered during validation
}

// findRSSFeed finds the RSS feed URL for a blog
func (s *Service) findRSSFeed(ctx context.Context, blogURL string) (string, rssFeedDebug, error) {
	debug := rssFeedDebug{
		attemptedURLs: []string{blogURL},
	}

	// First, check if the URL itself is already a valid feed.
	// This handles cases where the user provides a direct feed URL instead of a homepage.
	if s.isValidFeed(ctx, blogURL) {
		return blogURL, debug, nil
	}

	// Common RSS feed paths to try
	u, err := url.Parse(blogURL)
	if err != nil {
		return "", debug, fmt.Errorf("parse URL: %w", err)
	}

	baseURL := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	originalBase := strings.TrimRight(blogURL, "/")

	// First, try to parse HTML and find RSS link in <head> - this is usually the most reliable
	doc, err := s.fetchHTML(ctx, blogURL)
	if err == nil {
		var foundFeed string
		doc.Find("link[type='application/rss+xml'], link[type='application/atom+xml'], link[rel='alternate'][type*='xml']").Each(func(i int, sel *goquery.Selection) {
			if foundFeed != "" {
				return
			}
			if href, exists := sel.Attr("href"); exists {
				foundFeed = s.resolveURL(blogURL, href)
			}
		})

		// Record detected feed URL
		debug.detectedFeed = foundFeed

		if foundFeed != "" && s.isValidFeed(ctx, foundFeed) {
			return foundFeed, debug, nil
		}
		if foundFeed != "" {
			debug.validateErrors = append(debug.validateErrors, fmt.Sprintf("detected feed %s failed validation", foundFeed))
		}
	}

	// Expanded common RSS/Atom feed paths
	commonPaths := []string{
		"/rss.xml",
		"/feed.xml",
		"/atom.xml",
		"/feed",
		"/rss",
		"/feeds/posts/default", // Blogger
		"/index.xml",           // Hugo
		"/feed/",
		"/rss/",
		"/atom/",
		"/blog/feed",
		"/blog/rss",
		"/blog/feed.xml",
		"/blog/rss.xml",
		"/posts/feed",
		"/posts/rss.xml",
		"/?feed=rss2",     // WordPress
		"/feed/?type=rss", // Some WordPress
		"/rss2.xml",
		"/feed.atom",
		"/feed.rss",
	}
	candidatePaths := buildCandidateFeedURLs(baseURL, originalBase, commonPaths)

	// Add candidate paths to attempted URLs
	debug.attemptedURLs = append(debug.attemptedURLs, candidatePaths...)

	// Try common paths concurrently for faster discovery, but choose deterministically.
	resultCh := make(chan string, len(candidatePaths))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, MaxConcurrentPathChecks)

	for _, feedURL := range candidatePaths {
		wg.Add(1)
		go func(fURL string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if s.isValidFeed(ctx, fURL) {
				resultCh <- fURL
			}
		}(feedURL)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	validFeeds := make([]string, 0, len(candidatePaths))
	for result := range resultCh {
		validFeeds = append(validFeeds, result)
	}

	if len(validFeeds) > 0 {
		sort.SliceStable(validFeeds, func(i, j int) bool {
			return feedPriority(validFeeds[i], candidatePaths) < feedPriority(validFeeds[j], candidatePaths)
		})
		return validFeeds[0], debug, nil
	}

	// No valid feed found - add to validate errors
	debug.validateErrors = append(debug.validateErrors, "no valid feed found in any attempted URL")

	return "", debug, errRSSFeedNotFound
}

// isValidFeed checks if a URL is a valid RSS/Atom feed
func (s *Service) isValidFeed(ctx context.Context, feedURL string) bool {
	req, err := http.NewRequestWithContext(ctx, "HEAD", feedURL, nil)
	if err != nil {
		return false
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36")

	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode == http.StatusOK && (strings.Contains(contentType, "xml") || strings.Contains(contentType, "rss") || strings.Contains(contentType, "atom")) {
		return true
	}

	return s.isValidFeedByGET(ctx, feedURL)
}

func (s *Service) isValidFeedByGET(ctx context.Context, feedURL string) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return false
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36")

	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	buf := make([]byte, 1024)
	n, err := io.ReadAtLeast(resp.Body, buf, 1)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false
	}
	if n == 0 {
		return false
	}
	content := strings.ToLower(string(buf[:n]))

	return strings.Contains(content, "<?xml") ||
		strings.Contains(content, "<rss") ||
		strings.Contains(content, "<feed") ||
		strings.Contains(content, "<atom") ||
		strings.Contains(content, "rdf:rdf")
}

func buildCandidateFeedURLs(baseURL, originalBase string, commonPaths []string) []string {
	seen := map[string]bool{}
	urls := make([]string, 0, len(commonPaths)*2)
	for _, root := range []string{originalBase, baseURL} {
		for _, path := range commonPaths {
			candidate := root + path
			if !seen[candidate] {
				seen[candidate] = true
				urls = append(urls, candidate)
			}
		}
	}
	return urls
}

func feedPriority(feedURL string, orderedCandidates []string) int {
	for i, candidate := range orderedCandidates {
		if candidate == feedURL {
			return i
		}
	}
	return len(orderedCandidates)
}

// getFavicon gets the favicon URL for a blog
func (s *Service) getFavicon(blogURL string) string {
	u, err := url.Parse(blogURL)
	if err != nil {
		return ""
	}

	// Use Google's favicon service as fallback
	return fmt.Sprintf("https://www.google.com/s2/favicons?domain=%s", u.Host)
}
