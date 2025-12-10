// Package turn provides a client for the Turn API service that helps track
// pull request review states and blocking actions.
package turn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/retry"
)

const (
	// DefaultBackend is the default backend URL for the Turn API service.
	DefaultBackend = "https://turn.github.codegroove.app"

	userAgent       = "turnclient/1.1"
	maxResponseSize = 1024 * 1024 // 1MB
	clientTimeout   = 30 * time.Second
	retryAttempts   = 4 // 1 initial + 3 retries
	logMaxLength    = 100
	errorMaxLength  = 500
)

// Client communicates with the Turn API.
// Client methods are safe for concurrent use after initialization.
// Set* methods should only be called during setup before concurrent use.
type Client struct {
	httpClient    *http.Client
	logger        *log.Logger
	baseURL       string
	authToken     string
	noCache       bool
	includeEvents bool
}

// NewClient creates a new Turn API client with the specified backend URL.
// The baseURL should be a valid HTTP(S) URL without trailing slash.
func NewClient(baseURL string) (*Client, error) {
	if baseURL == "" {
		return nil, errors.New("base URL cannot be empty")
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("base URL must use http or https")
	}

	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: clientTimeout,
		},
		logger: log.New(io.Discard, "", 0),
	}, nil
}

// NewDefaultClient creates a new Turn API client using the default backend.
func NewDefaultClient() (*Client, error) {
	return NewClient(DefaultBackend)
}

// Option configures a Client.
type Option func(*Client)

// WithBackend sets a custom backend URL.
func WithBackend(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithLogger sets a custom logger.
func WithLogger(logger *log.Logger) Option {
	return func(c *Client) {
		if logger != nil {
			c.logger = logger
		}
	}
}

// WithAuthToken sets the GitHub authentication token.
func WithAuthToken(token string) Option {
	return func(c *Client) {
		c.authToken = token
	}
}

// WithNoCache enables or disables caching.
func WithNoCache(noCache bool) Option {
	return func(c *Client) {
		c.noCache = noCache
	}
}

// New creates a new Turn API client with options.
// If no backend is specified via WithBackend, uses DefaultBackend.
func New(opts ...Option) (*Client, error) {
	c, err := NewClient(DefaultBackend)
	if err != nil {
		return nil, err
	}

	originalURL := c.baseURL
	for _, opt := range opts {
		opt(c)
	}

	// Re-validate if backend was changed via options
	if c.baseURL != originalURL {
		u, err := url.Parse(c.baseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, errors.New("base URL must use http or https")
		}
	}

	return c, nil
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

// SetNoCache enables or disables caching for this client.
func (c *Client) SetNoCache(noCache bool) {
	c.noCache = noCache
}

// IncludeEvents enables including the full event list in check responses.
func (c *Client) IncludeEvents() {
	c.includeEvents = true
}

// Check validates a PR state at the given URL for the specified user.
// The updatedAt timestamp is used for caching.
func (c *Client) Check(ctx context.Context, prURL, user string, updatedAt time.Time) (*CheckResponse, error) {
	if prURL == "" {
		return nil, errors.New("PR URL cannot be empty")
	}
	if user == "" {
		return nil, errors.New("user cannot be empty")
	}
	if updatedAt.IsZero() {
		return nil, errors.New("updated_at timestamp cannot be zero")
	}

	// Truncate and sanitize for logging
	logURL := prURL
	if len(prURL) > logMaxLength {
		rs := []rune(prURL)
		if len(rs) > logMaxLength {
			logURL = string(rs[:logMaxLength]) + "..."
		} else {
			logURL = prURL
		}
	}
	c.logger.Printf("checking PR %s for user %s", logURL, user)

	req := CheckRequest{
		URL:           prURL,
		UpdatedAt:     updatedAt.UTC(),
		User:          user,
		IncludeEvents: c.includeEvents,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	c.logger.Printf("request JSON: %s", buf.String())

	endpoint := c.baseURL + "/v1/validate"
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("User-Agent", userAgent)
	r.Header.Set("Accept", "application/json")
	if c.authToken != "" {
		r.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	if c.noCache {
		r.Header.Set("Cache-Control", "no-cache")
	}

	c.logger.Printf("sending request to %s", endpoint)

	resp, err := c.doWithRetry(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Printf("failed to close response body: %v", err)
		}
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	c.logger.Printf("received response: status=%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		// For error responses, limit the body size in the error message
		msg := string(body)
		if len(body) > errorMaxLength {
			// Truncate at rune boundary to avoid splitting UTF-8 characters
			rs := []rune(msg)
			if len(rs) > errorMaxLength {
				msg = string(rs[:errorMaxLength]) + "... (truncated)"
			}
		}
		return nil, fmt.Errorf("api request failed with status %d: %s", resp.StatusCode, msg)
	}

	var result CheckResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	c.logger.Printf("check complete: %d actions assigned", len(result.Analysis.NextAction))
	return &result, nil
}

// CurrentUser retrieves the current authenticated GitHub user's login.
func (c *Client) CurrentUser(ctx context.Context) (string, error) {
	if c.authToken == "" {
		return "", errors.New("no auth token set")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+c.authToken)

	resp, err := c.doWithRetry(ctx, req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Printf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %w", err)
		}
		return "", fmt.Errorf("github API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if user.Login == "" {
		return "", errors.New("empty username in GitHub response")
	}

	return user.Login, nil
}

// doWithRetry performs an HTTP request with exponential backoff retry.
func (c *Client) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var resp *http.Response

	err := retry.Do(
		func() error {
			var err error
			// Close previous response body if it exists
			if resp != nil && resp.Body != nil {
				if err := resp.Body.Close(); err != nil {
					c.logger.Printf("failed to close previous response body: %v", err)
				}
			}
			resp, err = c.httpClient.Do(req) //nolint:bodyclose // closed by caller
			if err != nil {
				return err
			}

			// Only retry on 5xx errors or 429 (rate limit)
			if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
				// Read and close the error response body
				if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseSize)); err != nil {
					c.logger.Printf("failed to drain response body: %v", err)
				}
				if err := resp.Body.Close(); err != nil {
					c.logger.Printf("failed to close response body: %v", err)
				}
				return fmt.Errorf("server returned status %d", resp.StatusCode)
			}

			return nil
		},
		retry.Context(ctx),
		retry.Attempts(retryAttempts),
		retry.Delay(100*time.Millisecond),
		retry.MaxDelay(5*time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.MaxJitter(300*time.Millisecond),
		retry.OnRetry(func(n uint, err error) {
			c.logger.Printf("retrying request (attempt %d): %v", n+1, err)
		}),
	)

	return resp, err
}
