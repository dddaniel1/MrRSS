package utils

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// BuildProxyURL constructs a proxy URL from settings
func BuildProxyURL(proxyType, proxyHost, proxyPort, username, password string) string {
	if proxyHost == "" || proxyPort == "" {
		return ""
	}

	// Build auth string if username is provided
	auth := ""
	if username != "" {
		if password != "" {
			auth = username + ":" + password + "@"
		} else {
			auth = username + "@"
		}
	}

	return fmt.Sprintf("%s://%s%s:%s", proxyType, auth, proxyHost, proxyPort)
}

// CreateHTTPClient creates an HTTP client with optional proxy support
// This is the canonical implementation with proper TLS config and connection pooling
func CreateHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS10,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			},
		},
		MaxIdleConns:        50, // Reduced from 100 to prevent connection exhaustion
		MaxIdleConnsPerHost: 5,  // Reduced from 10 to limit connections per host
		IdleConnTimeout:     90 * time.Second,
		// Enable HTTP/2 for RSS feeds - it helps bypass anti-bot protection (e.g. xlrocket.blog)
		// and matches browser fingerprint better when used with modern User-Agents
		ForceAttemptHTTP2: true,
		// Write buffer size
		WriteBufferSize: 32 * 1024, // 32KB
		// Read buffer size
		ReadBufferSize: 32 * 1024, // 32KB
	}

	// Configure proxy if provided
	if proxyURL != "" {
		parsedProxy, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(parsedProxy)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	return client, nil
}

// RoundTripFunc is an adapter to allow the use of ordinary functions as http.RoundTripper
type RoundTripFunc func(req *http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper
func (rt RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

// UserAgentTransport wraps an http.RoundTripper to add User-Agent headers
type UserAgentTransport struct {
	Original  http.RoundTripper
	userAgent string
}

// RoundTrip implements http.RoundTripper
func (t *UserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Force set User-Agent to match browser (overwrite any existing value)
	req.Header.Set("User-Agent", t.userAgent)

	// Force set Accept header for RSS feeds (overwrite any existing value)
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml, application/atom+xml;q=0.9,*/*;q=0.8")

	// Force set Accept-Language header to mimic browser
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7")

	// Don't manually set Accept-Encoding - let Go's http.Transport handle it automatically
	// This ensures proper gzip decompression is applied
	// Removing: req.Header.Set("Accept-Encoding", "gzip, deflate, br")

	// Force set DNT header
	req.Header.Set("DNT", "1")

	// Note: Don't set Connection header as it's managed by the transport layer
	// Note: Don't set Upgrade-Insecure-Requests as it's only relevant for navigation

	// Add Sec-Fetch headers to mimic modern browsers
	if req.Header.Get("Sec-Fetch-Dest") == "" {
		req.Header.Set("Sec-Fetch-Dest", "document")
	}
	if req.Header.Get("Sec-Fetch-Mode") == "" {
		req.Header.Set("Sec-Fetch-Mode", "navigate")
	}
	if req.Header.Get("Sec-Fetch-Site") == "" {
		req.Header.Set("Sec-Fetch-Site", "none")
	}
	if req.Header.Get("Sec-Fetch-User") == "" {
		req.Header.Set("Sec-Fetch-User", "?1")
	}

	// Cache-Control header
	if req.Header.Get("Cache-Control") == "" {
		req.Header.Set("Cache-Control", "max-age=0")
	}

	return t.Original.RoundTrip(req)
}

// CreateHTTPClientWithUserAgent creates an HTTP client with a custom User-Agent
// This is important because some RSS servers block requests without a proper User-Agent
func CreateHTTPClientWithUserAgent(proxyURL string, timeout time.Duration, userAgent string) (*http.Client, error) {
	baseClient, err := CreateHTTPClient(proxyURL, timeout)
	if err != nil {
		return nil, err
	}

	// Wrap the transport to add User-Agent to all requests
	baseClient.Transport = &UserAgentTransport{
		Original:  baseClient.Transport,
		userAgent: userAgent,
	}

	return baseClient, nil
}
