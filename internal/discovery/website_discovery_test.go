package discovery

import "testing"

func TestNormalizeWebsiteCandidatesForRecommendation(t *testing.T) {
	result := NormalizeWebsiteCandidatesForRecommendation([]WebsiteCandidate{
		{URL: "example.com", Score: 50},
		{URL: "https://example.com/", Score: 90},
		{URL: "https://zzz.com", Score: 90},
		{URL: "ftp://invalid.com", Score: 100},
	})

	if len(result) != 2 {
		t.Fatalf("expected 2 normalized candidates, got %d", len(result))
	}
	if result[0].URL != "https://zzz.com" {
		t.Fatalf("expected highest score first, got %s", result[0].URL)
	}
	if result[1].URL != "https://example.com" {
		t.Fatalf("expected deduped canonical URL, got %s", result[1].URL)
	}
}
