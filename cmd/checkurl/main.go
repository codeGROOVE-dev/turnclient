package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ready-to-review/turnclient/pkg/turn"
)

const (
	defaultBackend  = "https://turn.ready-to-review.dev"
	requestTimeout  = 30 * time.Second
	userAuthTimeout = 10 * time.Second
)

func main() {
	var cfg config
	flag.StringVar(&cfg.backend, "backend", defaultBackend, "Backend server URL")
	flag.StringVar(&cfg.username, "user", "", "GitHub username to check (defaults to current authenticated user)")
	flag.BoolVar(&cfg.verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()

	if flag.NArg() != 1 {
		printUsage()
		os.Exit(1)
	}
	cfg.prURL = flag.Arg(0)

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type config struct {
	backend  string
	username string
	verbose  bool
	prURL    string
}

func run(cfg config) error {
	var logger *log.Logger
	if cfg.verbose {
		logger = log.New(os.Stderr, "[checkurl] ", log.LstdFlags|log.Lshortfile)
	} else {
		logger = log.New(io.Discard, "", 0)
	}

	logger.Printf("checking PR: %s", cfg.prURL)

	token := gitHubToken()
	if token == "" {
		logger.Println("no GitHub token found")
		if cfg.username == "" {
			return fmt.Errorf("no GitHub token found and no username specified\n" +
				"To authenticate, run 'gh auth login' or set GITHUB_TOKEN environment variable.\n" +
				"Alternatively, specify --user=<username> to check a specific user.")
		}
		fmt.Fprintln(os.Stderr, "warning: no GitHub token found, API requests may be rate limited\n")
	} else {
		logger.Println("GitHub token found")
		if cfg.username == "" {
			ctx, cancel := context.WithTimeout(context.Background(), userAuthTimeout)
			defer cancel()
			user, err := turn.CurrentUser(ctx, token)
			if err != nil {
				logger.Printf("failed to auto-detect user: %v", err)
				return fmt.Errorf("failed to auto-detect GitHub user: %v\n"+
					"Please specify --user=<username> to check a specific user.", err)
			}
			cfg.username = user
			logger.Printf("auto-detected user: %s", cfg.username)
		}
	}

	logger.Printf("creating client for backend: %s", cfg.backend)
	client, err := turn.NewClient(cfg.backend)
	if err != nil {
		logger.Printf("failed to create client: %v", err)
		return fmt.Errorf("creating client: %w", err)
	}
	if cfg.verbose {
		client.SetLogger(logger)
	}
	if token != "" {
		client.SetAuthToken(token)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	logger.Println("sending check request")
	result, err := client.Check(ctx, cfg.prURL, cfg.username, time.Now())
	if err != nil {
		logger.Printf("check failed: %v", err)
		return fmt.Errorf("checking PR: %w", err)
	}

	n := len(result.PRState.UnblockAction)
	var critical int
	for _, action := range result.PRState.UnblockAction {
		if action.Critical {
			critical++
		}
	}
	logger.Printf("check successful: %d total actions (%d critical)", n, critical)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		logger.Printf("failed to format response: %v", err)
		return fmt.Errorf("formatting response: %w", err)
	}

	if n > 0 {
		os.Exit(1)
	}
	return nil
}

func printUsage() {
	flag.Usage()
}

// gitHubToken attempts to get a GitHub token from environment or gh CLI.
func gitHubToken() string {
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		return token
	}
	if token := strings.TrimSpace(os.Getenv("GH_TOKEN")); token != "" {
		return token
	}
	
	// Try gh CLI
	cmd := exec.Command("gh", "auth", "token")
	cmd.Stderr = io.Discard
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}