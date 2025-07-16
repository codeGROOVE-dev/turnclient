// Package turn provides a client for the Turn API service.
package turn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	maxURLLength   = 2048
	userAgent      = "turnclient/1.0"
)

// Client communicates with the Turn API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	authToken  string
	logger     *log.Logger
}

// NewClient creates a new Turn API client.
// The baseURL should be a valid HTTP(S) URL without trailing slash.
func NewClient(baseURL string) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("base URL cannot be empty")
	}
	
	if len(baseURL) > maxURLLength {
		return nil, fmt.Errorf("base URL too long (max %d characters)", maxURLLength)
	}
	
	// Validate and normalize the base URL
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("base URL must use http or https scheme")
	}
	
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("base URL must include host")
	}
	
	// Remove trailing slash for consistency
	baseURL = strings.TrimRight(baseURL, "/")
	
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				DisableCompression:  true,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		logger: log.New(io.Discard, "[turn-client] ", log.LstdFlags|log.Lshortfile),
	}, nil
}

// SetAuthToken sets the GitHub authentication token.
func (c *Client) SetAuthToken(token string) {
	if token == "" {
		c.logger.Println("warning: setting empty auth token")
	}
	c.authToken = token
	c.logger.Println("auth token updated")
}

// SetLogger sets a custom logger for the client.
func (c *Client) SetLogger(logger *log.Logger) {
	if logger != nil {
		c.logger = logger
	}
}

// Check performs a PR check with a known PR update timestamp for caching.
func (c *Client) Check(ctx context.Context, prURL string, updatedAt time.Time) (*CheckResponse, error) {
	// Validate inputs
	if prURL == "" {
		return nil, fmt.Errorf("PR URL cannot be empty")
	}
	if updatedAt.IsZero() {
		return nil, fmt.Errorf("updated_at timestamp cannot be zero")
	}
	
	// Log sanitized values to prevent log injection
	safePR := sanitizeForLog(prURL)
	c.logger.Printf("checking PR %s (updated: %s)", safePR, updatedAt.Format(time.RFC3339))
	
	req := CheckRequest{
		URL:       prURL,
		UpdatedAt: updatedAt,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := c.baseURL + "/v1/validate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", userAgent)
	httpReq.Header.Set("Accept", "application/json")
	if c.authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	c.logger.Printf("sending request to %s", endpoint)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Limit response body to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, 1024*1024) // 1MB limit
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	c.logger.Printf("received response: status=%d, size=%d bytes", resp.StatusCode, len(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var result CheckResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	c.logger.Printf("check complete: %d actions assigned", len(result.NextAction))
	return &result, nil
}

// CurrentUser gets the current authenticated GitHub user.
// This is a package-level function as it doesn't require a Turn API client.
func CurrentUser(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("token cannot be empty")
	}
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		limitedReader := io.LimitReader(resp.Body, 1024*1024) // 1MB limit
		body, _ := io.ReadAll(limitedReader)
		return "", fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if user.Login == "" {
		return "", fmt.Errorf("empty username in GitHub response")
	}

	return user.Login, nil
}

// sanitizeForLog removes newlines and control characters to prevent log injection.
func sanitizeForLog(input string) string {
	// Replace newlines and carriage returns
	sanitized := strings.ReplaceAll(input, "\n", "\\n")
	sanitized = strings.ReplaceAll(sanitized, "\r", "\\r")
	
	// Remove other control characters
	var result strings.Builder
	for _, r := range sanitized {
		if r >= 32 && r != 127 {
			result.WriteRune(r)
		}
	}
	
	// Limit length to prevent log flooding
	output := result.String()
	const maxLen = 100
	if len(output) > maxLen {
		output = output[:maxLen-3] + "..."
	}
	
	return output
}

