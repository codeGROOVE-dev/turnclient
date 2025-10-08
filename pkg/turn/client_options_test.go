package turn

import (
	"log"
	"os"
	"testing"
)

func TestClientCreationOptions(t *testing.T) {
	// Test 1: NewClient with explicit backend
	client1, err := NewClient("https://custom.backend.com")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client1.baseURL != "https://custom.backend.com" {
		t.Errorf("Expected baseURL to be https://custom.backend.com, got %s", client1.baseURL)
	}

	// Test 2: NewDefaultClient uses default backend
	client2, err := NewDefaultClient()
	if err != nil {
		t.Fatalf("NewDefaultClient failed: %v", err)
	}
	if client2.baseURL != DefaultBackend {
		t.Errorf("Expected baseURL to be %s, got %s", DefaultBackend, client2.baseURL)
	}

	// Test 3: New with no options uses default backend
	client3, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if client3.baseURL != DefaultBackend {
		t.Errorf("Expected baseURL to be %s, got %s", DefaultBackend, client3.baseURL)
	}

	// Test 4: New with custom backend option
	client4, err := New(WithBackend("https://another.backend.com"))
	if err != nil {
		t.Fatalf("New with WithBackend failed: %v", err)
	}
	if client4.baseURL != "https://another.backend.com" {
		t.Errorf("Expected baseURL to be https://another.backend.com, got %s", client4.baseURL)
	}

	// Test 5: New with multiple options
	logger := log.New(os.Stderr, "test: ", log.LstdFlags)
	client5, err := New(
		WithBackend("https://test.backend.com"),
		WithLogger(logger),
		WithAuthToken("test-token"),
		WithNoCache(true),
	)
	if err != nil {
		t.Fatalf("New with multiple options failed: %v", err)
	}
	if client5.baseURL != "https://test.backend.com" {
		t.Errorf("Expected baseURL to be https://test.backend.com, got %s", client5.baseURL)
	}
	if client5.logger != logger {
		t.Error("Expected custom logger to be set")
	}
	if client5.authToken != "test-token" {
		t.Error("Expected authToken to be test-token")
	}
	if !client5.noCache {
		t.Error("Expected noCache to be true")
	}

	// Test 6: NewClient with empty string returns error (original behavior preserved)
	_, err = NewClient("")
	if err == nil {
		t.Error("Expected NewClient with empty string to return error")
	}
}
