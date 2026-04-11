package discovery

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"sync"
)

type WebsiteCandidate struct {
	URL    string `json:"url"`
	Score  int    `json:"score,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// DiscoverFromWebsitesWithProgress validates AI-generated website candidates into subscribeable feeds.
func (s *Service) DiscoverFromWebsitesWithProgress(ctx context.Context, candidates []WebsiteCandidate, progressCb ProgressCallback) ([]DiscoveredBlog, error) {
	feeds, _, err := s.DiscoverFromWebsitesWithProgressDetailed(ctx, candidates, progressCb)
	return feeds, err
}

// DiscoverFromWebsitesWithProgressDetailed validates AI-generated website candidates and records failures.
func (s *Service) DiscoverFromWebsitesWithProgressDetailed(ctx context.Context, candidates []WebsiteCandidate, progressCb ProgressCallback) ([]DiscoveredBlog, []FailedCandidate, error) {
	normalized := NormalizeWebsiteCandidatesForRecommendation(candidates)
	if len(normalized) == 0 {
		return []DiscoveredBlog{}, []FailedCandidate{}, nil
	}

	results := make([]DiscoveredBlog, len(normalized))
	failed := make([]FailedCandidate, len(normalized))
	resultMu := sync.Mutex{}
	var wg sync.WaitGroup
	sem := make(chan struct{}, MaxConcurrentRSSChecks)

	for i, candidate := range normalized {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil, nil, ctx.Err()
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(index int, website WebsiteCandidate) {
			defer wg.Done()
			defer func() { <-sem }()

			if progressCb != nil {
				resultMu.Lock()
				currentFound := countDiscovered(results)
				resultMu.Unlock()
				progressCb(Progress{
					Stage:      "validating_candidates",
					Message:    "Validating website candidates",
					Detail:     website.URL,
					Current:    index + 1,
					Total:      len(normalized),
					FoundCount: currentFound,
				})
			}

			blog, err := s.discoverBlogRSS(ctx, website.URL)
			if err != nil {
				resultMu.Lock()
				failed[index] = FailedCandidate{URL: website.URL, Stage: "validating_candidates", Reason: err.Error()}
				resultMu.Unlock()
				return
			}

			resultMu.Lock()
			results[index] = blog
			resultMu.Unlock()
		}(i, candidate)
	}

	wg.Wait()

	filtered := make([]DiscoveredBlog, 0, len(results))
	seenRSS := make(map[string]bool, len(results))
	for _, result := range results {
		if result.RSSFeed == "" || seenRSS[result.RSSFeed] {
			continue
		}
		seenRSS[result.RSSFeed] = true
		filtered = append(filtered, result)
	}
	filteredFailed := make([]FailedCandidate, 0, len(failed))
	for _, failure := range failed {
		if failure.URL == "" {
			continue
		}
		filteredFailed = append(filteredFailed, failure)
	}

	return filtered, filteredFailed, nil
}

func NormalizeWebsiteCandidatesForRecommendation(candidates []WebsiteCandidate) []WebsiteCandidate {
	normalized := make([]WebsiteCandidate, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))

	for _, candidate := range candidates {
		normalizedURL := normalizeWebsiteURL(candidate.URL)
		if normalizedURL == "" || seen[normalizedURL] {
			continue
		}
		seen[normalizedURL] = true
		candidate.URL = normalizedURL
		normalized = append(normalized, candidate)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].Score == normalized[j].Score {
			return normalized[i].URL < normalized[j].URL
		}
		return normalized[i].Score > normalized[j].Score
	})

	return normalized
}

func normalizeWebsiteURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" && parsed.Host == "" {
		parsed, err = url.Parse("https://" + trimmed)
		if err != nil {
			return ""
		}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	if parsed.Host == "" {
		return ""
	}

	parsed.Fragment = ""
	parsed.RawQuery = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}

	return strings.TrimRight(parsed.String(), "/")
}

func countDiscovered(results []DiscoveredBlog) int {
	count := 0
	for _, result := range results {
		if result.RSSFeed != "" {
			count++
		}
	}
	return count
}
