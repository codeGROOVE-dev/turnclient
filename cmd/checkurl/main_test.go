package main

import (
	"flag"
	"testing"
)

func TestConfig(t *testing.T) {
	cfg := config{
		backend:  "https://api.example.com",
		username: "testuser",
		verbose:  true,
		prURL:    "https://github.com/owner/repo/pull/123",
	}

	if cfg.backend == "" {
		t.Error("config backend should not be empty")
	}
	if cfg.username == "" {
		t.Error("config username should not be empty")
	}
	if cfg.prURL == "" {
		t.Error("config prURL should not be empty")
	}
	if !cfg.verbose {
		t.Error("config verbose should be true")
	}
}

func TestPrintUsage(_ *testing.T) {
	// Just verify it doesn't panic
	flag.Usage()
}

func TestValidatePRURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid PR URL",
			url:     "https://github.com/owner/repo/pull/123",
			wantErr: false,
		},
		{
			name:    "valid PR URL with trailing path",
			url:     "https://github.com/owner/repo/pull/123/files",
			wantErr: false,
		},
		{
			name:    "valid PR URL with www",
			url:     "https://www.github.com/owner/repo/pull/123",
			wantErr: false,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "not a GitHub URL",
			url:     "https://gitlab.com/owner/repo/pull/123",
			wantErr: true,
		},
		{
			name:    "not a PR URL",
			url:     "https://github.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "issue URL instead of PR",
			url:     "https://github.com/owner/repo/issues/123",
			wantErr: true,
		},
		{
			name:    "invalid scheme",
			url:     "ftp://github.com/owner/repo/pull/123",
			wantErr: true,
		},
		{
			name:    "no scheme",
			url:     "github.com/owner/repo/pull/123",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePRURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePRURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
