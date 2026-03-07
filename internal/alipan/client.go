package alipan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultOAuthBase = "https://openapi.alipan.com"
	defaultAPIBase   = "https://api.alipan.com"
)

type Client struct {
	httpClient *http.Client
	oauthBase  string
	apiBase    string
}

type Credentials struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
}

type AuthCodeCredentials struct {
	ClientID     string
	ClientSecret string
	Code         string
	RedirectURI  string
}

type DriveInfo struct {
	DefaultDriveID string
}

type TokenResult struct {
	AccessToken  string
	RefreshToken string
}

type FileEntry struct {
	FileID string `json:"file_id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
}

type apiErrorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	ErrorCode string `json:"error"`
	ErrorMsg  string `json:"error_description"`
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		oauthBase:  defaultOAuthBase,
		apiBase:    defaultAPIBase,
	}
}

func (c *Client) RefreshAccessToken(ctx context.Context, creds Credentials) (*TokenResult, error) {
	if strings.TrimSpace(creds.ClientID) == "" || strings.TrimSpace(creds.ClientSecret) == "" || strings.TrimSpace(creds.RefreshToken) == "" {
		return nil, fmt.Errorf("missing alipan credentials")
	}

	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": strings.TrimSpace(creds.RefreshToken),
		"client_id":     strings.TrimSpace(creds.ClientID),
		"client_secret": strings.TrimSpace(creds.ClientSecret),
	}

	type tokenResponse struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}

	paths := []string{"/oauth/access_token", "/v2/oauth/token"}
	var lastErr error
	for _, path := range paths {
		var resp tokenResponse
		err := c.doJSON(ctx, http.MethodPost, c.oauthBase+path, "", body, &resp)
		if err == nil {
			if resp.AccessToken == "" {
				return nil, fmt.Errorf("alipan returned empty access token")
			}
			if resp.RefreshToken == "" {
				resp.RefreshToken = creds.RefreshToken
			}
			return &TokenResult{AccessToken: resp.AccessToken, RefreshToken: resp.RefreshToken}, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("refresh alipan token failed: %w", lastErr)
}

func (c *Client) ExchangeAuthorizationCode(ctx context.Context, creds AuthCodeCredentials) (*TokenResult, error) {
	if strings.TrimSpace(creds.ClientID) == "" || strings.TrimSpace(creds.ClientSecret) == "" || strings.TrimSpace(creds.Code) == "" || strings.TrimSpace(creds.RedirectURI) == "" {
		return nil, fmt.Errorf("missing alipan authorization code credentials")
	}

	body := map[string]string{
		"grant_type":    "authorization_code",
		"code":          strings.TrimSpace(creds.Code),
		"redirect_uri":  strings.TrimSpace(creds.RedirectURI),
		"client_id":     strings.TrimSpace(creds.ClientID),
		"client_secret": strings.TrimSpace(creds.ClientSecret),
	}

	type tokenResponse struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}

	paths := []string{"/oauth/access_token", "/v2/oauth/token"}
	var lastErr error
	for _, path := range paths {
		var resp tokenResponse
		err := c.doJSON(ctx, http.MethodPost, c.oauthBase+path, "", body, &resp)
		if err == nil {
			if strings.TrimSpace(resp.AccessToken) == "" || strings.TrimSpace(resp.RefreshToken) == "" {
				return nil, fmt.Errorf("alipan returned incomplete token response")
			}
			return &TokenResult{AccessToken: resp.AccessToken, RefreshToken: resp.RefreshToken}, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("exchange alipan authorization code failed: %w", lastErr)
}

func (c *Client) GetDriveInfo(ctx context.Context, accessToken string) (*DriveInfo, error) {
	type driveInfoResponse struct {
		DefaultDriveID  string `json:"default_drive_id"`
		ResourceDriveID string `json:"resource_drive_id"`
	}

	var resp driveInfoResponse
	err := c.doJSON(ctx, http.MethodPost, c.apiBase+"/adrive/v1.0/user/getDriveInfo", accessToken, map[string]string{}, &resp)
	if err != nil {
		return nil, err
	}

	driveID := strings.TrimSpace(resp.DefaultDriveID)
	if driveID == "" {
		driveID = strings.TrimSpace(resp.ResourceDriveID)
	}
	if driveID == "" {
		return nil, fmt.Errorf("failed to resolve alipan drive id")
	}

	return &DriveInfo{DefaultDriveID: driveID}, nil
}

func (c *Client) EnsureFolder(ctx context.Context, accessToken, driveID, parentFileID, folderName string) (string, error) {
	folderName = strings.TrimSpace(folderName)
	if folderName == "" {
		return parentFileID, nil
	}

	children, err := c.ListFiles(ctx, accessToken, driveID, parentFileID)
	if err != nil {
		return "", err
	}
	for _, child := range children {
		if child.Type == "folder" && child.Name == folderName {
			return child.FileID, nil
		}
	}

	type createResponse struct {
		FileID string `json:"file_id"`
	}
	var created createResponse
	err = c.doJSON(ctx, http.MethodPost, c.apiBase+"/adrive/v2/file/createWithFolders", accessToken, map[string]interface{}{
		"drive_id":        driveID,
		"parent_file_id":  parentFileID,
		"name":            folderName,
		"type":            "folder",
		"check_name_mode": "refuse",
	}, &created)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(created.FileID) == "" {
		return "", fmt.Errorf("failed to create alipan folder")
	}

	return created.FileID, nil
}

func (c *Client) ListFiles(ctx context.Context, accessToken, driveID, parentFileID string) ([]FileEntry, error) {
	type listResponse struct {
		Items []FileEntry `json:"items"`
	}

	var resp listResponse
	err := c.doJSON(ctx, http.MethodPost, c.apiBase+"/adrive/v3/file/list", accessToken, map[string]interface{}{
		"drive_id":       driveID,
		"parent_file_id": parentFileID,
		"limit":          200,
	}, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) UploadSmallFile(ctx context.Context, accessToken, driveID, parentFileID, fileName string, data []byte) error {
	type partInfo struct {
		UploadURL string `json:"upload_url"`
	}
	type createResponse struct {
		DriveID      string     `json:"drive_id"`
		FileID       string     `json:"file_id"`
		UploadID     string     `json:"upload_id"`
		RapidUpload  bool       `json:"rapid_upload"`
		PartInfoList []partInfo `json:"part_info_list"`
	}

	var created createResponse
	err := c.doJSON(ctx, http.MethodPost, c.apiBase+"/adrive/v2/file/createWithFolders", accessToken, map[string]interface{}{
		"drive_id":        driveID,
		"parent_file_id":  parentFileID,
		"name":            fileName,
		"type":            "file",
		"size":            len(data),
		"check_name_mode": "overwrite",
		"part_info_list": []map[string]int{
			{"part_number": 1},
		},
	}, &created)
	if err != nil {
		return err
	}

	if !created.RapidUpload {
		if len(created.PartInfoList) == 0 || strings.TrimSpace(created.PartInfoList[0].UploadURL) == "" {
			return fmt.Errorf("alipan did not return upload url")
		}
		if err := c.putUpload(ctx, created.PartInfoList[0].UploadURL, data); err != nil {
			return err
		}
	}

	completeDriveID := strings.TrimSpace(created.DriveID)
	if completeDriveID == "" {
		completeDriveID = driveID
	}

	return c.doJSON(ctx, http.MethodPost, c.apiBase+"/v2/file/complete", accessToken, map[string]string{
		"drive_id":  completeDriveID,
		"file_id":   created.FileID,
		"upload_id": created.UploadID,
	}, nil)
}

func (c *Client) DownloadFileByName(ctx context.Context, accessToken, driveID, parentFileID, fileName string) ([]byte, error) {
	entries, err := c.ListFiles(ctx, accessToken, driveID, parentFileID)
	if err != nil {
		return nil, err
	}

	var targetFileID string
	for _, entry := range entries {
		if entry.Type == "file" && entry.Name == fileName {
			targetFileID = entry.FileID
			break
		}
	}
	if targetFileID == "" {
		return nil, fmt.Errorf("backup file %q not found", fileName)
	}

	type urlResponse struct {
		URL string `json:"url"`
	}
	var download urlResponse
	err = c.doJSON(ctx, http.MethodPost, c.apiBase+"/v2/file/get_download_url", accessToken, map[string]string{
		"drive_id": driveID,
		"file_id":  targetFileID,
	}, &download)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(download.URL) == "" {
		return nil, fmt.Errorf("alipan returned empty download url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, download.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("download backup failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) putUpload(ctx context.Context, uploadURL string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("upload part failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, reqURL, accessToken string, payload interface{}, out interface{}) error {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(accessToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		msg := extractAPIError(body)
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("alipan api error (%s): %s", resp.Status, msg)
	}

	if out != nil {
		if len(body) == 0 {
			return nil
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("parse alipan response failed: %w", err)
		}
	}

	return nil
}

func extractAPIError(body []byte) string {
	var apiErr apiErrorEnvelope
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if strings.TrimSpace(apiErr.Code) != "" {
		parts = append(parts, apiErr.Code)
	} else if strings.TrimSpace(apiErr.ErrorCode) != "" {
		parts = append(parts, apiErr.ErrorCode)
	}
	if strings.TrimSpace(apiErr.Message) != "" {
		parts = append(parts, apiErr.Message)
	} else if strings.TrimSpace(apiErr.ErrorMsg) != "" {
		parts = append(parts, apiErr.ErrorMsg)
	}
	return strings.TrimSpace(strings.Join(parts, ": "))
}

func NormalizeFileName(name, fallback string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = fallback
	}
	trimmed = strings.ReplaceAll(trimmed, "\\", "_")
	trimmed = strings.ReplaceAll(trimmed, "/", "_")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		trimmed = fallback
	}
	return trimmed
}

func NormalizeFolderName(name string) string {
	trimmed := strings.TrimSpace(name)
	trimmed = strings.Trim(trimmed, "/")
	return strings.ReplaceAll(trimmed, "\\", "/")
}

func SplitFolderSegments(folder string) []string {
	folder = NormalizeFolderName(folder)
	if folder == "" {
		return nil
	}
	raw := strings.Split(folder, "/")
	segments := make([]string, 0, len(raw))
	for _, seg := range raw {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		segments = append(segments, seg)
	}
	return segments
}

func IsURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
