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
	// Validate and normalize the base URL
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("base URL must use http or https scheme")
	}
	
	// Remove trailing slash for consistency
	baseURL = strings.TrimRight(baseURL, "/")
	
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: log.New(io.Discard, "[turn-client] ", log.LstdFlags),
	}, nil
}

// SetAuthToken sets the GitHub authentication token.
func (c *Client) SetAuthToken(token string) {
	c.authToken = token
	c.logger.Println("auth token set")
}

// SetLogger sets a custom logger for the client.
func (c *Client) SetLogger(logger *log.Logger) {
	if logger != nil {
		c.logger = logger
	}
}

// Check performs a PR check to see if it's blocked by the specified user.
func (c *Client) Check(ctx context.Context, prURL, username string) (*CheckResponse, error) {
	// Validate inputs
	if prURL == "" {
		return nil, fmt.Errorf("PR URL cannot be empty")
	}
	if username == "" {
		return nil, fmt.Errorf("username cannot be empty")
	}
	
	c.logger.Printf("checking PR %s for user %s", prURL, username)
	
	req := CheckRequest{
		URL:      prURL,
		Username: username,
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
	httpReq.Header.Set("User-Agent", "turnclient/1.0")
	if c.authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	c.logger.Printf("sending request to %s", endpoint)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
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

	c.logger.Printf("check complete: status=%d (%s)", result.Status, result.StatusString)
	return &result, nil
}

// GetCurrentUser gets the current authenticated GitHub user.
// This is a package-level function as it doesn't require a Turn API client.
func GetCurrentUser(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("token cannot be empty")
	}
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "turnclient/1.0")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
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

