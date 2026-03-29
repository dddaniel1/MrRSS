package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"MrRSS/internal/database"
	"MrRSS/internal/feed"
)

func TestNewHandler_ConstructsHandler(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("NewDB failed: %v", err)
	}
	if err := db.Init(); err != nil {
		t.Fatalf("db Init failed: %v", err)
	}

	f := feed.NewFetcher(db)
	h := NewHandler(db, f, nil)

	if h.DB == nil {
		t.Fatal("Handler DB is nil")
	}
	if h.Fetcher == nil {
		t.Fatal("Handler Fetcher is nil")
	}
	if h.DiscoveryService == nil {
		t.Fatal("DiscoveryService should be initialized")
	}
}

func TestGetGlobalProxyURL_DisabledReturnsEmpty(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("NewDB failed: %v", err)
	}
	if err := db.Init(); err != nil {
		t.Fatalf("db Init failed: %v", err)
	}

	f := feed.NewFetcher(db)
	h := NewHandler(db, f, nil)

	if err := db.SetSetting("proxy_enabled", "false"); err != nil {
		t.Fatalf("SetSetting(proxy_enabled) failed: %v", err)
	}

	proxyURL, err := h.getGlobalProxyURL()
	if err != nil {
		t.Fatalf("getGlobalProxyURL returned error: %v", err)
	}
	if proxyURL != "" {
		t.Fatalf("expected empty proxy URL when proxy disabled, got: %s", proxyURL)
	}
}

func TestFetchFullArticleContent_FromLocalServer(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("NewDB failed: %v", err)
	}
	if err := db.Init(); err != nil {
		t.Fatalf("db Init failed: %v", err)
	}

	f := feed.NewFetcher(db)
	h := NewHandler(db, f, nil)

	longParagraph := strings.Repeat("This is a long readable paragraph. ", 80)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><head><title>Test</title></head><body><article><h1>Readable Title</h1><p>" + longParagraph + "</p></article></body></html>"))
	}))
	defer server.Close()

	content, err := h.FetchFullArticleContent(server.URL)
	if err != nil {
		t.Fatalf("FetchFullArticleContent returned error: %v", err)
	}

	if strings.TrimSpace(content) == "" {
		t.Fatal("expected non-empty extracted content")
	}

	if !strings.Contains(strings.ToLower(content), "readable title") {
		t.Fatalf("expected extracted content to include title, got: %s", content)
	}
}
