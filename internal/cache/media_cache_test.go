package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMediaCache_BasicOperations(t *testing.T) {
	dir := t.TempDir()

	mc, err := NewMediaCache(dir)
	if err != nil {
		t.Fatalf("NewMediaCache failed: %v", err)
	}

	url := "https://example.com/image.jpg?query=1"
	path := mc.GetCachedPath(url)
	if filepath.Dir(path) != dir {
		t.Fatalf("cached path in wrong dir: %s", path)
	}

	// Create a cached file to simulate existing cache
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatalf("write cached file: %v", err)
	}

	if !mc.Exists(url) {
		t.Fatalf("expected Exists to be true for cached file")
	}

	data, ctype, err := mc.Get(url, "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(data) != "data" {
		t.Fatalf("unexpected data")
	}
	if ctype != "image/jpeg" {
		t.Fatalf("unexpected content type: %s", ctype)
	}

	// Test CleanupOldFiles (create an old file)
	oldPath := filepath.Join(dir, "old.bin")
	if err := os.WriteFile(oldPath, []byte("x"), 0644); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	// backdate modification time
	oldTime := time.Now().AddDate(0, 0, -10)
	_ = os.Chtimes(oldPath, oldTime, oldTime)

	removed, err := mc.CleanupOldFiles(1)
	if err != nil {
		t.Fatalf("CleanupOldFiles failed: %v", err)
	}
	if removed == 0 {
		t.Fatalf("expected CleanupOldFiles to remove old file")
	}
}

func TestGetExtensionAndContentTypeHelpers(t *testing.T) {
	if ext := getExtensionFromURL("https://x/y.png?v=1"); ext != ".png" {
		t.Fatalf("expected .png got %s", ext)
	}
	if ct := getContentTypeFromPath("file.jpg"); ct != "image/jpeg" {
		t.Fatalf("unexpected content type: %s", ct)
	}
	if ct := getContentTypeFromPath("file.heic"); ct != "image/heic" {
		t.Fatalf("unexpected content type: %s", ct)
	}
	if ext := getExtensionFromContentType("image/png; charset=utf8"); ext != ".png" {
		t.Fatalf("unexpected ext: %s", ext)
	}
	if ext := getExtensionFromContentType("image/heif"); ext != ".heif" {
		t.Fatalf("unexpected ext: %s", ext)
	}
}
