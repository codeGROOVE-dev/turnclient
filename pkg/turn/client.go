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
	userAgent       = "turnclient/1.0"
	maxResponseSize = 1024 * 1024 // 1MB
	clientTimeout   = 30 * time.Second
	retryAttempts   = 4 // 1 initial + 3 retries
	logMaxLength    = 100
	errorMaxLength  = 500
	asciiDEL        = 127 // ASCII DEL character
)

// Client communicates with the Turn API.
// All methods are safe for concurrent use.
type Client struct {
	httpClient *http.Client
	logger     *log.Logger
	baseURL    string
	authToken  string
	noCache    bool
}

// NewClient creates a new Turn API client.
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

	// Normalize URL by removing trailing slash
	baseURL = strings.TrimRight(baseURL, "/")

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: clientTimeout,
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

// SetNoCache enables or disables caching for this client.
func (c *Client) SetNoCache(noCache bool) {
	c.noCache = noCache
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
	if c.noCache {
		httpReq.Header.Set("Cache-Control", "no-cache")
	}

	c.logger.Printf("sending request to %s", endpoint)

	// Use retry for exponential backoff with jitter
	var resp *http.Response

	err = retry.Do(
		func() error {
			var err error
			// Close previous response body if it exists
			if resp != nil && resp.Body != nil {
				if err := resp.Body.Close(); err != nil {
					c.logger.Printf("failed to close previous response body: %v", err)
				}
			}
			resp, err = c.httpClient.Do(httpReq) //nolint:bodyclose // closed in defer after retry loop
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
		retry.RetryIf(func(_ error) bool {
			// Always retry on network errors or specific status codes
			return true
		}),
	)
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
		errorBody := string(body)
		if len(errorBody) > errorMaxLength {
			// Truncate at rune boundary to avoid splitting UTF-8 characters
			runes := []rune(errorBody)
			if len(runes) > errorMaxLength {
				errorBody = string(runes[:errorMaxLength]) + "... (truncated)"
			}
		}
		return nil, fmt.Errorf("api request failed with status %d: %s", resp.StatusCode, errorBody)
	}

	var result CheckResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	c.logger.Printf("check complete: %d actions assigned", len(result.PRState.UnblockAction))
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

	// Use retry for exponential backoff with jitter
	var resp *http.Response

	err = retry.Do(
		func() error {
			var err error
			// Close previous response body if it exists
			if resp != nil && resp.Body != nil {
				if err := resp.Body.Close(); err != nil {
					c.logger.Printf("failed to close previous response body: %v", err)
				}
			}
			resp, err = c.httpClient.Do(req) //nolint:bodyclose // closed in defer after retry loop
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
				return fmt.Errorf("github API returned status %d", resp.StatusCode)
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
			c.logger.Printf("retrying GitHub API request (attempt %d): %v", n+1, err)
		}),
		retry.RetryIf(func(_ error) bool {
			// Always retry on network errors or specific status codes
			return true
		}),
	)
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

// sanitizeForLog removes control characters to prevent log injection.
func sanitizeForLog(s string) string {
	// Truncate early to avoid processing unnecessary characters
	truncated := false
	if len(s) > logMaxLength {
		// Find proper rune boundary
		runes := []rune(s)
		if len(runes) > logMaxLength {
			s = string(runes[:logMaxLength])
			truncated = true
		}
	}

	// Single pass for efficiency
	var result strings.Builder
	result.Grow(len(s) + 3) // Pre-allocate with space for "..."

	for _, r := range s {
		switch r {
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		default:
			if r >= ' ' && r != asciiDEL {
				result.WriteRune(r)
			}
			// Control characters are skipped
		}
	}

	if truncated {
		result.WriteString("...")
	}

	return result.String()
}
