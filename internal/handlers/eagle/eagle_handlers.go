package eagle

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"MrRSS/internal/handlers/core"
)

const defaultEagleAPIURL = "http://localhost:41595"

type eagleConfig struct {
	Enabled        bool
	APIURL         string
	FolderID       string
	Tags           []string
	IncludeFeedTag bool
	NameTemplate   string
	Timeout        time.Duration
	MaxBatchSize   int
}

type saveImagesRequest struct {
	Images       []eagleImageRequest `json:"images"`
	ArticleTitle string              `json:"article_title"`
	ArticleURL   string              `json:"article_url"`
	FeedTitle    string              `json:"feed_title"`
	FeedURL      string              `json:"feed_url"`
}

type eagleImageRequest struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

type saveImagesResponse struct {
	Success int      `json:"success"`
	Failed  int      `json:"failed"`
	Total   int      `json:"total"`
	Errors  []string `json:"errors,omitempty"`
}

type eagleAddRequest struct {
	URL              string            `json:"url"`
	Name             string            `json:"name"`
	Website          string            `json:"website,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	Annotation       string            `json:"annotation,omitempty"`
	ModificationTime int64             `json:"modificationTime,omitempty"`
	FolderID         string            `json:"folderId,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
}

type eagleAPIResponse struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type eagleFolder struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Children []eagleFolder `json:"children"`
}

type eagleFolderOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Depth int    `json:"depth"`
}

// HandleSaveImages saves one or more images to Eagle by downloading them in MrRSS first.
func HandleSaveImages(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req saveImagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Images) == 0 {
		http.Error(w, "At least one image is required", http.StatusBadRequest)
		return
	}

	config := loadEagleConfig(h)
	if !config.Enabled {
		http.Error(w, "Eagle integration is disabled", http.StatusForbidden)
		return
	}
	if config.MaxBatchSize > 0 && len(req.Images) > config.MaxBatchSize {
		http.Error(w, fmt.Sprintf("Too many images: maximum batch size is %d", config.MaxBatchSize), http.StatusBadRequest)
		return
	}

	client := &http.Client{Timeout: config.Timeout}
	result := saveImagesResponse{Total: len(req.Images)}

	for i, image := range req.Images {
		if strings.TrimSpace(image.URL) == "" {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("image %d: missing URL", i+1))
			continue
		}

		if err := saveImageToEagle(r.Context(), client, config, req, image, i); err != nil {
			log.Printf("Failed to save image to Eagle: %v", err)
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("image %d: %v", i+1, err))
			continue
		}

		result.Success++
	}

	w.Header().Set("Content-Type", "application/json")
	status := http.StatusOK
	if result.Success == 0 {
		status = http.StatusBadGateway
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(result)
}

// HandleListFolders returns a flattened list of Eagle folders for visual selection.
func HandleListFolders(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config := loadEagleConfig(h)
	client := &http.Client{Timeout: config.Timeout}

	body, err := callEagle(r.Context(), client, config.APIURL, "/api/folder/list")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	var result eagleAPIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		http.Error(w, "Failed to parse Eagle folder response", http.StatusBadGateway)
		return
	}
	if result.Status != "success" {
		http.Error(w, eagleStatusError(result).Error(), http.StatusBadGateway)
		return
	}

	var folders []eagleFolder
	if err := json.Unmarshal(result.Data, &folders); err != nil {
		http.Error(w, "Failed to parse Eagle folders", http.StatusBadGateway)
		return
	}

	options := flattenFolders(folders, "", 0)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"folders": options})
}

// HandleTestConnection verifies that the configured Eagle API endpoint is reachable.
func HandleTestConnection(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config := loadEagleConfig(h)
	client := &http.Client{Timeout: config.Timeout}
	_, err := callEagle(r.Context(), client, config.APIURL, "/api/folder/list")

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func saveImageToEagle(ctx context.Context, client *http.Client, config eagleConfig, req saveImagesRequest, image eagleImageRequest, index int) error {
	resolvedURL, referer, err := resolveImageURL(image.URL, req.FeedURL, req.ArticleURL)
	if err != nil {
		return err
	}

	dataURL := resolvedURL
	if !strings.HasPrefix(resolvedURL, "data:") {
		dataURL, err = downloadImageAsDataURL(ctx, client, resolvedURL, referer)
		if err != nil {
			return err
		}
	}

	name := strings.TrimSpace(image.Name)
	if name == "" {
		name = buildImageName(config.NameTemplate, req, resolvedURL, index)
	}

	tags := append([]string{}, config.Tags...)
	if config.IncludeFeedTag && strings.TrimSpace(req.FeedTitle) != "" {
		tags = append(tags, strings.TrimSpace(req.FeedTitle))
	}

	body := eagleAddRequest{
		URL:              dataURL,
		Name:             name,
		Website:          req.ArticleURL,
		Tags:             tags,
		Annotation:       req.ArticleTitle,
		ModificationTime: time.Now().UnixMilli(),
		FolderID:         config.FolderID,
	}
	if referer != "" {
		body.Headers = map[string]string{"referer": referer}
	}

	return postToEagle(ctx, client, config.APIURL, body)
}

func resolveImageURL(rawURL, feedURL, articleURL string) (string, string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", "", fmt.Errorf("missing image URL")
	}

	if strings.HasPrefix(trimmed, "data:") {
		return trimmed, "", nil
	}

	if strings.HasPrefix(trimmed, "/api/media/proxy") {
		resolved, referer, err := decodeMediaProxyURL(trimmed)
		if err != nil {
			return "", "", err
		}
		if referer == "" {
			referer = firstNonEmpty(feedURL, articleURL)
		}
		return resolved, referer, nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", "", fmt.Errorf("invalid image URL: %w", err)
	}

	if parsed.Scheme == "" {
		base := firstNonEmpty(feedURL, articleURL)
		if base == "" {
			return "", "", fmt.Errorf("relative image URL has no base URL")
		}
		baseURL, err := url.Parse(base)
		if err != nil {
			return "", "", fmt.Errorf("invalid base URL: %w", err)
		}
		parsed = baseURL.ResolveReference(parsed)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("unsupported image URL scheme: %s", parsed.Scheme)
	}

	return parsed.String(), firstNonEmpty(feedURL, articleURL), nil
}

func decodeMediaProxyURL(proxyURL string) (string, string, error) {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid proxied image URL: %w", err)
	}

	values := parsed.Query()
	mediaURL := values.Get("url")
	if encoded := values.Get("url_b64"); encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", "", fmt.Errorf("invalid proxied image URL encoding: %w", err)
		}
		mediaURL = string(decoded)
	}

	referer := values.Get("referer")
	for _, key := range []string{"referer_b64", "baseurl_b64"} {
		if encoded := values.Get(key); encoded != "" {
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err == nil {
				referer = string(decoded)
				break
			}
		}
	}

	if mediaURL == "" {
		return "", "", fmt.Errorf("proxied image URL is missing source URL")
	}

	return mediaURL, referer, nil
}

func downloadImageAsDataURL(ctx context.Context, client *http.Client, imageURL, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("create image request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download image: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 80<<20))
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	if len(body) == 0 {
		return "", fmt.Errorf("downloaded image is empty")
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	contentType, _, _ = mime.ParseMediaType(contentType)
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("downloaded URL is not an image: %s", contentType)
	}

	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(body), nil
}

func postToEagle(ctx context.Context, client *http.Client, apiURL string, payload eagleAddRequest) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Eagle request: %w", err)
	}

	endpoint, err := buildEagleEndpoint(apiURL, "/api/item/addFromURL")
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("create Eagle request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to Eagle: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Eagle API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result eagleAPIResponse
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &result); err != nil {
			return fmt.Errorf("parse Eagle response: %w", err)
		}
	}
	if result.Status != "" && result.Status != "success" {
		return eagleStatusError(result)
	}

	return nil
}

func callEagle(ctx context.Context, client *http.Client, apiURL, endpointPath string) ([]byte, error) {
	endpoint, err := buildEagleEndpoint(apiURL, endpointPath)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create Eagle request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to Eagle: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Eagle API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

func loadEagleConfig(h *core.Handler) eagleConfig {
	apiURL := getSetting(h, "eagle_api_url", defaultEagleAPIURL)
	timeoutSeconds := getIntSetting(h, "eagle_timeout_seconds", 45)
	if timeoutSeconds <= 0 {
		timeoutSeconds = 45
	}
	maxBatchSize := getIntSetting(h, "eagle_max_batch_size", 50)
	if maxBatchSize <= 0 {
		maxBatchSize = 50
	}

	rawTags := getSetting(h, "eagle_tags", "MrRSS")
	return eagleConfig{
		Enabled:        getBoolSetting(h, "eagle_enabled", false),
		APIURL:         strings.TrimRight(firstNonEmpty(apiURL, defaultEagleAPIURL), "/"),
		FolderID:       getSetting(h, "eagle_folder_id", ""),
		Tags:           splitTags(rawTags),
		IncludeFeedTag: getBoolSetting(h, "eagle_include_feed_tag", true),
		NameTemplate:   firstNonEmpty(getSetting(h, "eagle_name_template", ""), "{title} {index}"),
		Timeout:        time.Duration(timeoutSeconds) * time.Second,
		MaxBatchSize:   maxBatchSize,
	}
}

func buildEagleEndpoint(apiURL, endpointPath string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(apiURL))
	if err != nil {
		return "", fmt.Errorf("invalid Eagle API URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("Eagle API URL must use HTTP or HTTPS")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("Eagle API URL is missing host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + endpointPath
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func flattenFolders(folders []eagleFolder, parentPath string, depth int) []eagleFolderOption {
	options := make([]eagleFolderOption, 0)
	for _, folder := range folders {
		folderPath := folder.Name
		if parentPath != "" {
			folderPath = parentPath + " / " + folder.Name
		}
		options = append(options, eagleFolderOption{ID: folder.ID, Name: folder.Name, Path: folderPath, Depth: depth})
		options = append(options, flattenFolders(folder.Children, folderPath, depth+1)...)
	}
	return options
}

func buildImageName(template string, req saveImagesRequest, imageURL string, index int) string {
	name := template
	if strings.TrimSpace(name) == "" {
		name = "{title} {timestamp} {index}"
	}
	values := map[string]string{
		"{title}":     req.ArticleTitle,
		"{feed}":      req.FeedTitle,
		"{index}":     fmt.Sprintf("%02d", index+1),
		"{date}":      time.Now().Format("2006-01-02"),
		"{timestamp}": time.Now().Format("20060102-150405"),
	}
	for key, value := range values {
		name = strings.ReplaceAll(name, key, value)
	}

	base := sanitizeName(name)
	if base == "" {
		base = sanitizeName(fileNameFromURL(imageURL))
	}
	if base == "" {
		base = "MrRSS Image"
	}
	return base
}

func fileNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(path.Base(parsed.Path), path.Ext(parsed.Path))
}

func sanitizeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			builder.WriteRune('_')
		default:
			builder.WriteRune(r)
		}
	}
	name := strings.TrimSpace(builder.String())
	if len([]rune(name)) > 120 {
		name = string([]rune(name)[:120])
	}
	return name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func splitTags(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '\n'
	})
	tags := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags
}

func getSetting(h *core.Handler, key, fallback string) string {
	if h == nil || h.DB == nil {
		return fallback
	}
	value, err := h.DB.GetSetting(key)
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func getBoolSetting(h *core.Handler, key string, fallback bool) bool {
	value := strings.ToLower(getSetting(h, key, ""))
	if value == "" {
		return fallback
	}
	return value == "true" || value == "1" || value == "yes"
}

func getIntSetting(h *core.Handler, key string, fallback int) int {
	value := getSetting(h, key, "")
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
		return fallback
	}
	return parsed
}

func eagleStatusError(result eagleAPIResponse) error {
	if result.Message != "" {
		return fmt.Errorf("Eagle API returned %s: %s", result.Status, result.Message)
	}
	return fmt.Errorf("Eagle API returned %s", result.Status)
}
