package alipan

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"MrRSS/internal/alipan"
	"MrRSS/internal/handlers/core"
	"MrRSS/internal/models"
	opmlpkg "MrRSS/internal/opml"
)

type config struct {
	ClientID       string
	ClientSecret   string
	RefreshToken   string
	BackupFolder   string
	BackupName     string
	RSSHubEndpoint string
}

type oauthPendingEntry struct {
	RedirectURI string
	CreatedAt   time.Time
}

var (
	oauthPendingMu sync.Mutex
	oauthPending   = map[string]oauthPendingEntry{}
)

const (
	builtinClientIDEnv     = "MRRSS_ALIPAN_CLIENT_ID"
	builtinClientSecretEnv = "MRRSS_ALIPAN_CLIENT_SECRET"
)

func HandleOAuthStatus(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hasBuiltin := strings.TrimSpace(os.Getenv(builtinClientIDEnv)) != "" && strings.TrimSpace(os.Getenv(builtinClientSecretEnv)) != ""
	refreshToken, _ := h.DB.GetEncryptedSetting("alipan_refresh_token")

	writeJSON(w, map[string]interface{}{
		"connected":   strings.TrimSpace(refreshToken) != "",
		"has_builtin": hasBuiltin,
		"enabled":     true,
		"credential_via": func() string {
			if hasBuiltin {
				return "builtin"
			}
			return "manual"
		}(),
	})
}

func HandleOAuthStart(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientID := strings.TrimSpace(os.Getenv(builtinClientIDEnv))
	if clientID == "" {
		http.Error(w, "builtin alipan app is not configured", http.StatusBadRequest)
		return
	}

	state, err := randomState(24)
	if err != nil {
		http.Error(w, "failed to create oauth state", http.StatusInternalServerError)
		return
	}

	redirectURI := buildRedirectURI(r)

	oauthPendingMu.Lock()
	cleanupPendingLocked()
	oauthPending[state] = oauthPendingEntry{RedirectURI: redirectURI, CreatedAt: time.Now()}
	oauthPendingMu.Unlock()

	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("state", state)
	authorizeURL := "https://openapi.alipan.com/oauth/authorize?" + q.Encode()

	writeJSON(w, map[string]string{
		"authorize_url": authorizeURL,
	})
}

func HandleOAuthCallback(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if state == "" || code == "" {
		writeOAuthHTML(w, false, "Missing oauth state or code")
		return
	}

	oauthPendingMu.Lock()
	pending, ok := oauthPending[state]
	if ok {
		delete(oauthPending, state)
	}
	oauthPendingMu.Unlock()
	if !ok {
		writeOAuthHTML(w, false, "Invalid or expired oauth state")
		return
	}

	clientID := strings.TrimSpace(os.Getenv(builtinClientIDEnv))
	clientSecret := strings.TrimSpace(os.Getenv(builtinClientSecretEnv))
	if clientID == "" || clientSecret == "" {
		writeOAuthHTML(w, false, "Builtin alipan app is not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	client := alipan.NewClient()
	token, err := client.ExchangeAuthorizationCode(ctx, alipan.AuthCodeCredentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Code:         code,
		RedirectURI:  pending.RedirectURI,
	})
	if err != nil {
		writeOAuthHTML(w, false, "Authorization failed: "+err.Error())
		return
	}

	if err := h.DB.SetEncryptedSetting("alipan_refresh_token", token.RefreshToken); err != nil {
		writeOAuthHTML(w, false, "Failed to save account authorization")
		return
	}
	_ = h.DB.SetSetting("alipan_enabled", "true")

	writeOAuthHTML(w, true, "Aliyun Drive connected. You can return to MrRSS.")
}

func HandleOAuthDisconnect(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.DB.SetEncryptedSetting("alipan_refresh_token", ""); err != nil {
		http.Error(w, "failed to clear authorization", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func HandleBackup(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, err := loadConfig(h)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	client := alipan.NewClient()
	token, driveID, folderID, err := authorizeAndResolve(ctx, h, client, cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	feeds, err := h.DB.GetFeeds()
	if err != nil {
		http.Error(w, "failed to read feeds: "+err.Error(), http.StatusInternalServerError)
		return
	}

	localFeeds := make([]models.Feed, 0, len(feeds))
	for _, feed := range feeds {
		if feed.IsFreshRSSSource {
			continue
		}
		localFeeds = append(localFeeds, feed)
	}

	opmlData, err := opmlpkg.GenerateWithOptions(localFeeds, opmlpkg.GenerateOptions{
		UseRSSHubProtocol: false,
		RSSHubEndpoint:    cfg.RSSHubEndpoint,
	})
	if err != nil {
		http.Error(w, "failed to build OPML backup: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := client.UploadSmallFile(ctx, token.AccessToken, driveID, folderID, cfg.BackupName, opmlData); err != nil {
		http.Error(w, "backup upload failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	now := time.Now().Format(time.RFC3339)
	if err := h.DB.SetSetting("alipan_last_backup_time", now); err != nil {
		log.Printf("failed to update alipan_last_backup_time: %v", err)
	}

	writeJSON(w, map[string]interface{}{
		"status":      "ok",
		"format":      "opml",
		"feed_count":  len(localFeeds),
		"backup_time": now,
	})
}

func HandleRestore(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req := struct {
		FullReplace *bool `json:"full_replace"`
	}{}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}

	fullReplace := true
	if req.FullReplace != nil {
		fullReplace = *req.FullReplace
	}

	cfg, err := loadConfig(h)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	client := alipan.NewClient()
	token, driveID, folderID, err := authorizeAndResolve(ctx, h, client, cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	data, err := client.DownloadFileByName(ctx, token.AccessToken, driveID, folderID, cfg.BackupName)
	if err != nil {
		http.Error(w, "download backup failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	incomingFeeds, err := opmlpkg.Parse(bytes.NewReader(data))
	if err != nil {
		http.Error(w, "invalid OPML backup: "+err.Error(), http.StatusBadRequest)
		return
	}

	restored, deleted, err := applyOPMLFeeds(h, incomingFeeds, fullReplace)
	if err != nil {
		http.Error(w, "restore failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	now := time.Now().Format(time.RFC3339)
	if err := h.DB.SetSetting("alipan_last_restore_time", now); err != nil {
		log.Printf("failed to update alipan_last_restore_time: %v", err)
	}

	writeJSON(w, map[string]interface{}{
		"status":         "ok",
		"format":         "opml",
		"restored_count": restored,
		"deleted_count":  deleted,
		"restore_time":   now,
	})
}

func authorizeAndResolve(ctx context.Context, h *core.Handler, client *alipan.Client, cfg *config) (*alipan.TokenResult, string, string, error) {
	token, err := client.RefreshAccessToken(ctx, alipan.Credentials{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RefreshToken: cfg.RefreshToken,
	})
	if err != nil {
		return nil, "", "", err
	}

	if token.RefreshToken != cfg.RefreshToken {
		if err := h.DB.SetEncryptedSetting("alipan_refresh_token", token.RefreshToken); err != nil {
			log.Printf("failed to persist alipan_refresh_token: %v", err)
		}
	}

	driveInfo, err := client.GetDriveInfo(ctx, token.AccessToken)
	if err != nil {
		return nil, "", "", err
	}

	folderID, err := resolveFolder(ctx, client, token.AccessToken, driveInfo.DefaultDriveID, cfg.BackupFolder)
	if err != nil {
		return nil, "", "", err
	}

	return token, driveInfo.DefaultDriveID, folderID, nil
}

func loadConfig(h *core.Handler) (*config, error) {
	enabled, _ := h.DB.GetSetting("alipan_enabled")
	if enabled != "true" {
		return nil, errString("alipan integration is disabled")
	}

	clientID := strings.TrimSpace(os.Getenv(builtinClientIDEnv))
	clientSecret := strings.TrimSpace(os.Getenv(builtinClientSecretEnv))
	if clientID == "" {
		raw, _ := h.DB.GetSetting("alipan_client_id")
		clientID = strings.TrimSpace(raw)
	}
	if clientSecret == "" {
		raw, _ := h.DB.GetEncryptedSetting("alipan_client_secret")
		clientSecret = strings.TrimSpace(raw)
	}
	refreshToken, _ := h.DB.GetEncryptedSetting("alipan_refresh_token")
	backupFolder, _ := h.DB.GetSetting("alipan_backup_folder")
	backupName, _ := h.DB.GetSetting("alipan_backup_filename")
	rsshubEndpoint, _ := h.DB.GetSetting("rsshub_endpoint")

	cfg := &config{
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		RefreshToken:   strings.TrimSpace(refreshToken),
		BackupFolder:   strings.TrimSpace(backupFolder),
		BackupName:     alipan.NormalizeFileName(backupName, "subscriptions.opml"),
		RSSHubEndpoint: strings.TrimSpace(rsshubEndpoint),
	}
	if cfg.BackupFolder == "" {
		cfg.BackupFolder = "MrRSS"
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RefreshToken == "" {
		return nil, errString("alipan credentials are incomplete")
	}

	return cfg, nil
}

func resolveFolder(ctx context.Context, client *alipan.Client, accessToken, driveID, folderPath string) (string, error) {
	current := "root"
	for _, segment := range alipan.SplitFolderSegments(folderPath) {
		nextID, err := client.EnsureFolder(ctx, accessToken, driveID, current, segment)
		if err != nil {
			return "", err
		}
		current = nextID
	}
	return current, nil
}

func applyOPMLFeeds(h *core.Handler, incoming []models.Feed, fullReplace bool) (int, int, error) {
	restored := 0
	deleted := 0
	incomingURLs := make(map[string]struct{}, len(incoming))

	for _, feed := range incoming {
		feedURL := strings.TrimSpace(feed.URL)
		if !alipan.IsURL(feedURL) {
			continue
		}
		incomingURLs[feedURL] = struct{}{}

		item := feed
		item.URL = feedURL
		item.Title = strings.TrimSpace(item.Title)
		if item.Title == "" {
			item.Title = item.URL
		}
		if item.EmailIMAPPort == 0 {
			item.EmailIMAPPort = 993
		}
		if strings.TrimSpace(item.EmailFolder) == "" {
			item.EmailFolder = "INBOX"
		}
		if strings.TrimSpace(item.ArticleViewMode) == "" {
			item.ArticleViewMode = "global"
		}
		if strings.TrimSpace(item.AutoExpandContent) == "" {
			item.AutoExpandContent = "global"
		}

		if _, err := h.DB.AddFeed(&item); err != nil {
			return restored, deleted, err
		}
		restored++
	}

	if fullReplace {
		localFeeds, err := h.DB.GetFeeds()
		if err != nil {
			return restored, deleted, err
		}
		for _, feed := range localFeeds {
			if feed.IsFreshRSSSource {
				continue
			}
			if _, ok := incomingURLs[strings.TrimSpace(feed.URL)]; ok {
				continue
			}
			if err := h.DB.DeleteFeed(feed.ID); err != nil {
				return restored, deleted, err
			}
			deleted++
		}
	}

	return restored, deleted, nil
}

func randomState(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func buildRedirectURI(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}
	host := r.Host
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1:1234"
	}
	return scheme + "://" + host + "/api/alipan/oauth/callback"
}

func cleanupPendingLocked() {
	deadline := time.Now().Add(-10 * time.Minute)
	for state, entry := range oauthPending {
		if entry.CreatedAt.Before(deadline) {
			delete(oauthPending, state)
		}
	}
}

func writeOAuthHTML(w http.ResponseWriter, success bool, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title := "Authorization Failed"
	color := "#c53030"
	if success {
		title = "Authorization Succeeded"
		color = "#2f855a"
	}
	_, _ = w.Write([]byte("<!doctype html><html><head><meta charset=\"utf-8\"><title>MrRSS</title></head><body style=\"font-family: sans-serif; padding: 24px;\"><h2 style=\"color:" + color + "\">" + title + "</h2><p>" + html.EscapeString(message) + "</p><p>You can close this page and return to MrRSS.</p></body></html>"))
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

type errString string

func (e errString) Error() string {
	return string(e)
}
