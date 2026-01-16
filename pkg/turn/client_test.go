package turn

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{
			name:    "valid https URL",
			baseURL: "https://api.example.com",
			wantErr: false,
		},
		{
			name:    "valid http URL",
			baseURL: "http://localhost:8080",
			wantErr: false,
		},
		{
			name:    "URL with trailing slash",
			baseURL: "https://api.example.com/",
			wantErr: false,
		},
		{
			name:    "invalid URL",
			baseURL: "not a url",
			wantErr: true,
		},
		{
			name:    "invalid scheme",
			baseURL: "ftp://example.com",
			wantErr: true,
		},
		{
			name:    "empty URL",
			baseURL: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.baseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewClient() returned nil client without error")
			}
			if !tt.wantErr && client != nil {
				// Verify trailing slash is removed
				if strings.HasSuffix(client.baseURL, "/") {
					t.Errorf("baseURL has trailing slash: %s", client.baseURL)
				}
			}
		})
	}
}

//nolint:gocognit // Test contains comprehensive test cases
func TestClient_Check(t *testing.T) {
	tests := []struct {
		name         string
		prURL        string
		username     string
		response     CheckResponse
		statusCode   int
		wantErr      bool
		errorMessage string
	}{
		{
			name:     "successful check - not blocked",
			prURL:    "https://github.com/owner/repo/pull/123",
			username: "testuser",
			response: CheckResponse{
				Analysis: Analysis{
					NextAction:         map[string]Action{},
					Checks:             Checks{},
					UnresolvedComments: 0,
					ReadyToMerge:       true,
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:     "successful check - blocked",
			prURL:    "https://github.com/owner/repo/pull/456",
			username: "testuser",
			response: CheckResponse{
				Analysis: Analysis{
					NextAction: map[string]Action{
						"testuser": {Kind: "REVIEW", Critical: true, Reason: "needs to review"},
					},
					Checks:             Checks{},
					UnresolvedComments: 0,
					ReadyToMerge:       false,
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:         "empty PR URL",
			prURL:        "",
			username:     "testuser",
			wantErr:      true,
			errorMessage: "PR URL cannot be empty",
		},
		{
			name:         "server error",
			prURL:        "https://github.com/owner/repo/pull/789",
			username:     "testuser",
			statusCode:   http.StatusInternalServerError,
			wantErr:      true,
			errorMessage: "send request:", // Changed due to retry wrapper
		},
		{
			name:         "not found",
			prURL:        "https://github.com/owner/repo/pull/999",
			username:     "testuser",
			statusCode:   http.StatusNotFound,
			wantErr:      true,
			errorMessage: "api request failed with status 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/v1/validate" {
					t.Errorf("expected /v1/validate, got %s", r.URL.Path)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
				}
				if r.Header.Get("User-Agent") != "turnclient/1.1" {
					t.Errorf("expected User-Agent: turnclient/1.1, got %s", r.Header.Get("User-Agent"))
				}

				// Decode request body
				var req CheckRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}

				// Send response
				if tt.statusCode != 0 && tt.statusCode != http.StatusOK {
					w.WriteHeader(tt.statusCode)
					if _, err := w.Write([]byte("error message")); err != nil {
						t.Errorf("failed to write error message: %v", err)
					}
					return
				}

				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(tt.response); err != nil {
					t.Errorf("failed to encode response: %v", err)
				}
			}))
			defer server.Close()

			// Create client
			client, err := NewClient(server.URL)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			client.SetLogger(log.New(io.Discard, "", 0)) // Silence logs during tests

			// Perform check
			ctx := context.Background()
			result, err := client.Check(ctx, tt.prURL, tt.username, time.Now())

			// Verify results
			if (err != nil) != tt.wantErr {
				t.Errorf("Check() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && tt.errorMessage != "" {
				if !strings.Contains(err.Error(), tt.errorMessage) {
					t.Errorf("Check() error = %v, want error containing %q", err, tt.errorMessage)
				}
			}

			if !tt.wantErr && result != nil {
				if len(result.Analysis.NextAction) != len(tt.response.Analysis.NextAction) {
					t.Errorf("NextAction length = %d, want %d", len(result.Analysis.NextAction), len(tt.response.Analysis.NextAction))
				}
				if result.Analysis.ReadyToMerge != tt.response.Analysis.ReadyToMerge {
					t.Errorf("ReadyToMerge = %v, want %v", result.Analysis.ReadyToMerge, tt.response.Analysis.ReadyToMerge)
				}
			}
		})
	}
}

func TestClient_CheckWithAuth(t *testing.T) {
	token := "test-token-123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + token
		if authHeader != expectedAuth {
			t.Errorf("Authorization header = %s, want %s", authHeader, expectedAuth)
		}

		// Send success response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(CheckResponse{
			Analysis: Analysis{
				NextAction:         map[string]Action{},
				Checks:             Checks{},
				UnresolvedComments: 0,
				ReadyToMerge:       true,
			},
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	client.SetLogger(log.New(io.Discard, "", 0))
	client.SetAuthToken(token)

	ctx := context.Background()
	_, err = client.Check(ctx, "https://github.com/owner/repo/pull/123", "testuser", time.Now())
	if err != nil {
		t.Errorf("Check() with auth failed: %v", err)
	}
}

func TestClient_CheckTimeout(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	client.SetLogger(log.New(io.Discard, "", 0))

	// Use a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = client.Check(ctx, "https://github.com/owner/repo/pull/123", "testuser", time.Now())
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

// mockTransport is a custom RoundTripper for testing
type mockTransport struct {
	statusCode int
	response   string
	checkFunc  func(*http.Request)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.checkFunc != nil {
		m.checkFunc(req)
	}

	body := io.NopCloser(strings.NewReader(m.response))
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       body,
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestCurrentUser(t *testing.T) {
	tests := []struct {
		name         string
		hasToken     bool
		statusCode   int
		response     string
		wantLogin    string
		wantErr      bool
		errorMessage string
	}{
		{
			name:         "no auth token",
			hasToken:     false,
			wantErr:      true,
			errorMessage: "no auth token set",
		},
		{
			name:       "successful request",
			hasToken:   true,
			statusCode: http.StatusOK,
			response:   `{"login":"testuser","id":12345}`,
			wantLogin:  "testuser",
			wantErr:    false,
		},
		{
			name:         "unauthorized",
			hasToken:     true,
			statusCode:   http.StatusUnauthorized,
			response:     `{"message":"Bad credentials"}`,
			wantErr:      true,
			errorMessage: "github API request failed with status 401",
		},
		{
			name:         "server error",
			hasToken:     true,
			statusCode:   http.StatusInternalServerError,
			response:     `{"message":"Internal server error"}`,
			wantErr:      true,
			errorMessage: "send request:",
		},
		{
			name:         "empty login in response",
			hasToken:     true,
			statusCode:   http.StatusOK,
			response:     `{"login":"","id":12345}`,
			wantErr:      true,
			errorMessage: "empty username in GitHub response",
		},
		{
			name:         "invalid json response",
			hasToken:     true,
			statusCode:   http.StatusOK,
			response:     `{invalid json}`,
			wantErr:      true,
			errorMessage: "decode response:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient("https://api.example.com")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			client.SetLogger(log.New(io.Discard, "", 0))

			if tt.hasToken {
				client.SetAuthToken("test-token")

				// Set up mock transport for tests that need to make requests
				if tt.statusCode > 0 {
					client.httpClient = &http.Client{
						Transport: &mockTransport{
							statusCode: tt.statusCode,
							response:   tt.response,
							checkFunc: func(req *http.Request) {
								if req.Method != http.MethodGet {
									t.Errorf("expected GET, got %s", req.Method)
								}
								if req.URL.Path != "/user" {
									t.Errorf("expected /user, got %s", req.URL.Path)
								}
								authHeader := req.Header.Get("Authorization")
								if !strings.HasPrefix(authHeader, "Bearer ") {
									t.Errorf("expected Bearer token, got %s", authHeader)
								}
							},
						},
						Timeout: clientTimeout,
					}
				}
			}

			ctx := context.Background()
			login, err := client.CurrentUser(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("CurrentUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && tt.errorMessage != "" {
				if !strings.Contains(err.Error(), tt.errorMessage) {
					t.Errorf("CurrentUser() error = %v, want error containing %q", err, tt.errorMessage)
				}
			}

			if !tt.wantErr && login != tt.wantLogin {
				t.Errorf("CurrentUser() login = %s, want %s", login, tt.wantLogin)
			}
		})
	}
}

func TestSetNoCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Cache-Control header
		cacheControl := r.Header.Get("Cache-Control")
		if cacheControl != "no-cache" {
			t.Errorf("expected Cache-Control: no-cache, got %s", cacheControl)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(CheckResponse{
			Analysis: Analysis{
				NextAction:         map[string]Action{},
				Checks:             Checks{},
				UnresolvedComments: 0,
				ReadyToMerge:       true,
			},
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	client.SetLogger(log.New(io.Discard, "", 0))
	client.SetNoCache(true)

	ctx := context.Background()
	_, err = client.Check(ctx, "https://github.com/owner/repo/pull/123", "testuser", time.Now())
	if err != nil {
		t.Errorf("Check() with noCache failed: %v", err)
	}
}

func TestIncludeEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decode request body to verify IncludeEvents is set
		var req CheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if !req.IncludeEvents {
			t.Error("expected IncludeEvents to be true")
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(CheckResponse{
			Analysis: Analysis{
				NextAction:         map[string]Action{},
				Checks:             Checks{},
				UnresolvedComments: 0,
				ReadyToMerge:       true,
			},
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	client.SetLogger(log.New(io.Discard, "", 0))
	client.IncludeEvents()

	ctx := context.Background()
	_, err = client.Check(ctx, "https://github.com/owner/repo/pull/123", "testuser", time.Now())
	if err != nil {
		t.Errorf("Check() with includeEvents failed: %v", err)
	}
}

func TestCheckValidation(t *testing.T) {
	client, err := NewClient("https://api.example.com")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	client.SetLogger(log.New(io.Discard, "", 0))

	ctx := context.Background()

	t.Run("empty user", func(t *testing.T) {
		_, err := client.Check(ctx, "https://github.com/owner/repo/pull/123", "", time.Now())
		if err == nil || !strings.Contains(err.Error(), "user cannot be empty") {
			t.Errorf("expected 'user cannot be empty' error, got %v", err)
		}
	})

	t.Run("zero timestamp", func(t *testing.T) {
		_, err := client.Check(ctx, "https://github.com/owner/repo/pull/123", "testuser", time.Time{})
		if err == nil || !strings.Contains(err.Error(), "updated_at timestamp cannot be zero") {
			t.Errorf("expected 'updated_at timestamp cannot be zero' error, got %v", err)
		}
	})

	t.Run("very long URL truncation", func(t *testing.T) {
		// Test that very long URLs are properly truncated in logs without causing errors
		longURL := "https://github.com/owner/repo/pull/123?" + strings.Repeat("a", 200)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(CheckResponse{
				Analysis: Analysis{
					NextAction:         map[string]Action{},
					Checks:             Checks{},
					UnresolvedComments: 0,
					ReadyToMerge:       true,
				},
			}); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		client.baseURL = server.URL
		client.httpClient = server.Client()

		_, err := client.Check(ctx, longURL, "testuser", time.Now())
		if err != nil {
			t.Errorf("Check() with long URL failed: %v", err)
		}
	})
}

func TestNewWithOptions(t *testing.T) {
	t.Run("with custom backend", func(t *testing.T) {
		customURL := "https://custom.example.com"
		client, err := New(WithBackend(customURL))
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		if client.baseURL != customURL {
			t.Errorf("baseURL = %s, want %s", client.baseURL, customURL)
		}
	})

	t.Run("with logger", func(t *testing.T) {
		var buf strings.Builder
		logger := log.New(&buf, "", 0)
		client, err := New(WithLogger(logger))
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		if client.logger == nil {
			t.Error("logger was not set")
		}
	})

	t.Run("with nil logger", func(t *testing.T) {
		client, err := New(WithLogger(nil))
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		if client.logger == nil {
			t.Error("logger should not be nil when nil logger provided")
		}
	})

	t.Run("with auth token", func(t *testing.T) {
		token := "test-token-123"
		client, err := New(WithAuthToken(token))
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		if client.authToken != token {
			t.Errorf("authToken = %s, want %s", client.authToken, token)
		}
	})

	t.Run("with no cache", func(t *testing.T) {
		client, err := New(WithNoCache(true))
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		if !client.noCache {
			t.Error("noCache should be true")
		}
	})

	t.Run("with invalid backend via options", func(t *testing.T) {
		_, err := New(WithBackend("ftp://invalid.com"))
		if err == nil {
			t.Error("expected error for invalid backend scheme")
		}
	})

	t.Run("multiple options", func(t *testing.T) {
		customURL := "https://multi.example.com"
		token := "multi-token"
		logger := log.New(io.Discard, "", 0)

		client, err := New(
			WithBackend(customURL),
			WithAuthToken(token),
			WithLogger(logger),
			WithNoCache(true),
		)
		if err != nil {
			t.Fatalf("New() with multiple options failed: %v", err)
		}

		if client.baseURL != customURL {
			t.Errorf("baseURL = %s, want %s", client.baseURL, customURL)
		}
		if client.authToken != token {
			t.Errorf("authToken = %s, want %s", client.authToken, token)
		}
		if !client.noCache {
			t.Error("noCache should be true")
		}
	})
}

func TestNewDefaultClient(t *testing.T) {
	client, err := NewDefaultClient()
	if err != nil {
		t.Fatalf("NewDefaultClient() failed: %v", err)
	}
	if client.baseURL != DefaultBackend {
		t.Errorf("baseURL = %s, want %s", client.baseURL, DefaultBackend)
	}
}
