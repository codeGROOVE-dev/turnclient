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
	userAgent       = "turnclient/1.0"
	maxResponseSize = 1024 * 1024 // 1MB
)

// Client communicates with the Turn API.
// All methods are safe for concurrent use.
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
	
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("base URL must use http or https")
	}
	
	// Normalize URL by removing trailing slash
	baseURL = strings.TrimRight(baseURL, "/")
	
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: log.New(io.Discard, "", 0),
	}, nil
}

// SetAuthToken sets the GitHub authentication token.
func (c *Client) SetAuthToken(token string) {
	c.authToken = token
}

// SetLogger sets a custom logger for the client.
func (c *Client) SetLogger(logger *log.Logger) {
	if logger != nil {
		c.logger = logger
	}
}

// Check validates a PR state at the given URL for the specified user.
// The updatedAt timestamp is used for caching.
func (c *Client) Check(ctx context.Context, prURL, user string, updatedAt time.Time) (*CheckResponse, error) {
	if prURL == "" {
		return nil, fmt.Errorf("PR URL cannot be empty")
	}
	if user == "" {
		return nil, fmt.Errorf("user cannot be empty")
	}
	if updatedAt.IsZero() {
		return nil, fmt.Errorf("updated_at timestamp cannot be zero")
	}
	
	c.logger.Printf("checking PR %s for user %s", sanitizeForLog(prURL), sanitizeForLog(user))
	
	req := CheckRequest{
		URL:       prURL,
		UpdatedAt: updatedAt,
		User:      user,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	endpoint := c.baseURL + "/v1/validate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	c.logger.Printf("received response: status=%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result CheckResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	c.logger.Printf("check complete: %d actions assigned", len(result.PRState.UnblockAction))
	return &result, nil
}

// CurrentUser retrieves the current authenticated GitHub user's login.
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return "", fmt.Errorf("GitHub API request failed with status %d: %s", resp.StatusCode, string(body))
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

// sanitizeForLog removes control characters to prevent log injection.
func sanitizeForLog(s string) string {
	if len(s) > 100 {
		s = s[:100] + "..."
	}
	
	// Simple replacement of problematic characters
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	
	return s
}