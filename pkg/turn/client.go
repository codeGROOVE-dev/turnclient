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
	
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("base URL must use http or https scheme")
	}
	
	if u.Host == "" {
		return nil, fmt.Errorf("base URL must include host")
	}
	
	// Normalize URL by removing trailing slash
	baseURL = strings.TrimRight(baseURL, "/")
	
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		logger: log.New(io.Discard, "[turn-client] ", log.LstdFlags|log.Lshortfile),
	}, nil
}

// SetAuthToken sets the GitHub authentication token.
func (c *Client) SetAuthToken(token string) {
	c.authToken = token
	if token == "" {
		c.logger.Println("warning: setting empty auth token")
	} else {
		c.logger.Println("auth token updated")
	}
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
	if updatedAt.IsZero() {
		return nil, fmt.Errorf("updated_at timestamp cannot be zero")
	}
	
	c.logger.Printf("checking PR %s for user %s (updated: %s)", sanitizeForLog(prURL), sanitizeForLog(user), updatedAt.Format(time.RFC3339))
	
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

	// Read response body with size limit
	body, err := readResponseBody(resp, 1024*1024) // 1MB limit
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
		body, _ := readResponseBody(resp, 1024*1024) // 1MB limit
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

// readResponseBody reads response body with a size limit.
func readResponseBody(resp *http.Response, maxSize int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(resp.Body, maxSize))
}

// sanitizeForLog removes newlines and control characters to prevent log injection.
func sanitizeForLog(input string) string {
	const maxLen = 100
	
	// Truncate early if too long
	n := len(input)
	if n > maxLen {
		n = maxLen
		input = input[:n]
	}
	
	var result strings.Builder
	result.Grow(n)
	
	for _, r := range input {
		switch r {
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		default:
			if r >= 32 && r != 127 {
				result.WriteRune(r)
			}
		}
	}
	
	if n == maxLen && result.Len() >= maxLen-3 {
		s := result.String()
		return s[:maxLen-3] + "..."
	}
	return result.String()
}