package discovery

import (
	"testing"
)

func TestParseWebsiteCandidates_ObjectShape(t *testing.T) {
	input := `{"websites":[{"url":"https://example.com","score":95,"reason":"same topic"}]}`
	result := parseWebsiteCandidates(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(result))
	}
	if result[0].URL != "https://example.com" {
		t.Fatalf("unexpected URL: %s", result[0].URL)
	}
	if result[0].Score != 95 {
		t.Fatalf("unexpected score: %d", result[0].Score)
	}
}

func TestParseWebsiteCandidates_ArrayShape(t *testing.T) {
	input := `[{"homepage":"https://foo.com","score":"88","why":"close topic"}]`
	result := parseWebsiteCandidates(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(result))
	}
	if result[0].Score != 88 {
		t.Fatalf("unexpected score: %d", result[0].Score)
	}
}

func TestParseWebsiteCandidates_FencedJSON(t *testing.T) {
	input := "```json\n{\"sites\":[\"https://one.com\",\"https://two.com\"]}\n```"
	result := parseWebsiteCandidates(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(result))
	}
}

func TestParseWebsiteCandidates_TextFallback(t *testing.T) {
	input := "1. https://one.com\n2. https://two.com/rss\n3. https://three.com"
	result := parseWebsiteCandidates(input)
	if len(result) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(result))
	}
}
