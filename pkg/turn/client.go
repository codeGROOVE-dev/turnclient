package turn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Client is a client for the Turn API
type Client struct {
	baseURL    string
	httpClient *http.Client
	authToken  string
}

// NewClient creates a new Turn API client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetAuthToken sets the GitHub authentication token
func (c *Client) SetAuthToken(token string) {
	c.authToken = token
}

// Check performs a PR check to see if it's blocked by the specified user
func (c *Client) Check(ctx context.Context, prURL, username string) (*CheckResponse, error) {
	req := CheckRequest{
		URL:      prURL,
		Username: username,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/validate", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
	}

	var result CheckResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return &result, nil
}

// GetCurrentUser gets the current authenticated GitHub user
func GetCurrentUser(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "wriia-turn-client")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error: %d %s", resp.StatusCode, string(body))
	}

	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", err
	}

	if user.Login == "" {
		return "", fmt.Errorf("no username in GitHub response")
	}

	return user.Login, nil
}

// LocalServer provides functionality to start a local server instance
type LocalServer struct {
	listener net.Listener
	server   *http.Server
}

// StartLocalServer starts a local instance of the Turn server on a random port
func StartLocalServer(handler http.Handler) (*LocalServer, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("error starting local server: %w", err)
	}

	server := &http.Server{
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			// Log error but don't crash - server might be shutting down
		}
	}()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	return &LocalServer{
		listener: listener,
		server:   server,
	}, nil
}

// URL returns the URL of the local server
func (ls *LocalServer) URL() string {
	return fmt.Sprintf("http://localhost:%d", ls.listener.Addr().(*net.TCPAddr).Port)
}

// Shutdown gracefully shuts down the local server
func (ls *LocalServer) Shutdown(ctx context.Context) error {
	return ls.server.Shutdown(ctx)
}