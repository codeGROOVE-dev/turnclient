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
		flag.Usage()
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
		logger = log.New(os.Stderr, "", log.LstdFlags)
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
		fmt.Fprint(os.Stderr, "warning: no GitHub token found, API requests may be rate limited\n")
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

	client, err := turn.NewClient(cfg.backend)
	if err != nil {
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

	result, err := client.Check(ctx, cfg.prURL, cfg.username, time.Now())
	if err != nil {
		return fmt.Errorf("checking PR: %w", err)
	}

	blockingActions := len(result.PRState.UnblockAction)
	logger.Printf("found %d blocking actions", blockingActions)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encoding response: %w", err)
	}

	if blockingActions > 0 {
		os.Exit(1)
	}
	return nil
}

// gitHubToken gets a GitHub token from environment or gh CLI.
func gitHubToken() string {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	if token := os.Getenv("GH_TOKEN"); token != "" {
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