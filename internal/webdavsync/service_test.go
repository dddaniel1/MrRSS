package webdavsync

import "testing"

func TestNormalizeRemotePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "default when empty", in: "", want: "/MrRSS/subscriptions.opml"},
		{name: "leading slash added", in: "sync/subscriptions.opml", want: "/sync/subscriptions.opml"},
		{name: "windows separators normalized", in: `folder\\nested\\subscriptions.opml`, want: "/folder/nested/subscriptions.opml"},
		{name: "directory path gets default file", in: "/folder/nested/", want: "/folder/nested/subscriptions.opml"},
		{name: "root becomes default", in: "/", want: "/MrRSS/subscriptions.opml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRemotePath(tt.in); got != tt.want {
				t.Fatalf("normalizeRemotePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCanonicalFeedURL(t *testing.T) {
	got := canonicalFeedURL("  HTTPS://Example.COM/Feed.xml  ")
	if got != "https://example.com/feed.xml" {
		t.Fatalf("canonicalFeedURL normalized to %q", got)
	}
}
