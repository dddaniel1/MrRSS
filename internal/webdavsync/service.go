package webdavsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"MrRSS/internal/database"
	"MrRSS/internal/feed"
	"MrRSS/internal/models"
	"MrRSS/internal/opml"

	"github.com/studio-b12/gowebdav"
)

const defaultRemotePath = "/MrRSS/subscriptions.opml"

type Config struct {
	URL        string
	Username   string
	Password   string
	RemotePath string
}

type Result struct {
	ImportedCount int    `json:"imported_count"`
	ExportedCount int    `json:"exported_count"`
	RemoteExists  bool   `json:"remote_exists"`
	LastSyncTime  string `json:"last_sync_time"`
}

type Service struct {
	db      *database.DB
	fetcher *feed.Fetcher
}

func NewService(db *database.DB, fetcher *feed.Fetcher) *Service {
	return &Service{db: db, fetcher: fetcher}
}

func (s *Service) LoadConfig() (*Config, error) {
	enabled, err := s.db.GetSetting("webdav_enabled")
	if err != nil {
		return nil, fmt.Errorf("read webdav_enabled: %w", err)
	}
	if enabled != "true" {
		return nil, errors.New("WebDAV sync is disabled")
	}

	rawURL, err := s.db.GetSetting("webdav_url")
	if err != nil {
		return nil, fmt.Errorf("read webdav_url: %w", err)
	}
	username, err := s.db.GetSetting("webdav_username")
	if err != nil {
		return nil, fmt.Errorf("read webdav_username: %w", err)
	}
	password, err := s.db.GetEncryptedSetting("webdav_password")
	if err != nil {
		return nil, fmt.Errorf("read webdav_password: %w", err)
	}
	rawPath, err := s.db.GetSetting("webdav_remote_path")
	if err != nil {
		return nil, fmt.Errorf("read webdav_remote_path: %w", err)
	}

	config := &Config{
		URL:        strings.TrimSpace(rawURL),
		Username:   strings.TrimSpace(username),
		Password:   password,
		RemotePath: normalizeRemotePath(rawPath),
	}

	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

func (s *Service) SyncSubscriptions(ctx context.Context) (*Result, error) {
	config, err := s.LoadConfig()
	if err != nil {
		return nil, err
	}

	client := gowebdav.NewClient(config.URL, config.Username, config.Password)
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("connect WebDAV server: %w", err)
	}

	remoteData, remoteExists, err := s.readRemoteSubscriptions(ctx, client, config.RemotePath)
	if err != nil {
		return nil, err
	}

	importedCount, importedIDs, err := s.importRemoteFeeds(remoteData)
	if err != nil {
		return nil, err
	}

	if len(importedIDs) > 0 {
		go s.fetcher.FetchFeedsByIDs(context.Background(), importedIDs)
	}

	exportedData, exportedCount, err := s.exportCurrentSubscriptions()
	if err != nil {
		return nil, err
	}

	if err := s.writeRemoteSubscriptions(ctx, client, config.RemotePath, exportedData); err != nil {
		return nil, err
	}

	lastSyncTime := time.Now().Format(time.RFC3339)
	if err := s.db.SetSetting("webdav_last_sync_time", lastSyncTime); err != nil {
		return nil, fmt.Errorf("save webdav_last_sync_time: %w", err)
	}

	return &Result{
		ImportedCount: importedCount,
		ExportedCount: exportedCount,
		RemoteExists:  remoteExists,
		LastSyncTime:  lastSyncTime,
	}, nil
}

func (s *Service) readRemoteSubscriptions(ctx context.Context, client *gowebdav.Client, remotePath string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	_, err := client.Stat(remotePath)
	if err != nil {
		if isNotFoundError(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("stat remote subscriptions file: %w", err)
	}

	data, err := client.Read(remotePath)
	if err != nil {
		return nil, true, fmt.Errorf("read remote subscriptions file: %w", err)
	}

	return data, true, nil
}

func (s *Service) importRemoteFeeds(remoteData []byte) (int, []int64, error) {
	if len(bytes.TrimSpace(remoteData)) == 0 {
		return 0, nil, nil
	}

	remoteFeeds, err := opml.Parse(bytes.NewReader(remoteData))
	if err != nil {
		return 0, nil, fmt.Errorf("parse remote OPML: %w", err)
	}

	localFeeds, err := s.db.GetFeeds()
	if err != nil {
		return 0, nil, fmt.Errorf("load local feeds: %w", err)
	}

	existingURLs := make(map[string]struct{}, len(localFeeds))
	for _, existing := range localFeeds {
		if existing.IsFreshRSSSource {
			continue
		}
		key := canonicalFeedURL(existing.URL)
		if key != "" {
			existingURLs[key] = struct{}{}
		}
	}

	importedCount := 0
	importedIDs := make([]int64, 0)
	for _, remoteFeed := range remoteFeeds {
		key := canonicalFeedURL(remoteFeed.URL)
		if key == "" {
			continue
		}
		if _, exists := existingURLs[key]; exists {
			continue
		}

		feedModel := remoteFeed
		feedID, err := s.db.AddFeed(&feedModel)
		if err != nil {
			return importedCount, importedIDs, fmt.Errorf("import remote feed %q: %w", remoteFeed.Title, err)
		}

		importedCount++
		importedIDs = append(importedIDs, feedID)
		existingURLs[key] = struct{}{}
	}

	return importedCount, importedIDs, nil
}

func (s *Service) exportCurrentSubscriptions() ([]byte, int, error) {
	feeds, err := s.db.GetFeeds()
	if err != nil {
		return nil, 0, fmt.Errorf("load feeds for export: %w", err)
	}

	localFeeds := make([]models.Feed, 0, len(feeds))
	for _, current := range feeds {
		if current.IsFreshRSSSource {
			continue
		}
		localFeeds = append(localFeeds, current)
	}

	rsshubEndpoint, _ := s.db.GetSetting("rsshub_endpoint")
	if rsshubEndpoint == "" {
		rsshubEndpoint = "https://rsshub.app"
	}

	data, err := opml.GenerateWithOptions(localFeeds, opml.GenerateOptions{
		UseRSSHubProtocol: false,
		RSSHubEndpoint:    rsshubEndpoint,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("generate OPML export: %w", err)
	}

	return data, len(localFeeds), nil
}

func (s *Service) writeRemoteSubscriptions(ctx context.Context, client *gowebdav.Client, remotePath string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	remoteDir := path.Dir(remotePath)
	if remoteDir != "." && remoteDir != "/" {
		if err := client.MkdirAll(remoteDir, 0o755); err != nil {
			return fmt.Errorf("create remote directory %q: %w", remoteDir, err)
		}
	}

	tempPath := remotePath + ".tmp"
	if err := client.Write(tempPath, data, 0o644); err != nil {
		return fmt.Errorf("upload temporary subscriptions file: %w", err)
	}

	if err := client.Rename(tempPath, remotePath, true); err != nil {
		if writeErr := client.Write(remotePath, data, 0o644); writeErr != nil {
			return fmt.Errorf("rename temp subscriptions file: %v; direct upload fallback failed: %w", err, writeErr)
		}
	}

	return nil
}

func validateConfig(config *Config) error {
	if config.URL == "" {
		return errors.New("WebDAV server URL is required")
	}

	parsedURL, err := url.Parse(config.URL)
	if err != nil {
		return fmt.Errorf("invalid WebDAV server URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return errors.New("WebDAV server URL must use http or https")
	}
	if parsedURL.Host == "" {
		return errors.New("WebDAV server URL host is required")
	}

	if strings.TrimSpace(config.Username) == "" {
		return errors.New("WebDAV username is required")
	}
	if strings.TrimSpace(config.Password) == "" {
		return errors.New("WebDAV password is required")
	}

	return nil
}

func normalizeRemotePath(rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return defaultRemotePath
	}

	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}

	cleaned := path.Clean(normalized)
	if cleaned == "/" {
		return defaultRemotePath
	}
	if strings.HasSuffix(normalized, "/") {
		return path.Join(cleaned, "subscriptions.opml")
	}
	return cleaned
}

func canonicalFeedURL(feedURL string) string {
	return strings.TrimSpace(strings.ToLower(feedURL))
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	errText := strings.ToLower(err.Error())
	return strings.Contains(errText, "404") || strings.Contains(errText, "not found")
}
