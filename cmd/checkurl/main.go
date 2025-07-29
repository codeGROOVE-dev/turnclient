// Package main implements the checkurl command-line tool for checking
// GitHub pull request review states using the Turn API.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/ready-to-review/turnclient/pkg/turn"
)

const (
	defaultBackend     = "https://turn.ready-to-review.dev"
	requestTimeout     = 30 * time.Second
	userAuthTimeout    = 10 * time.Second
	serverStartTimeout = 5 * time.Second
)

var (
	// Compile regex once for performance
	prURLPattern = regexp.MustCompile(`^/([^/]+)/([^/]+)/pull/(\d+)(?:/.*)?$`)
)

func main() {
	var cfg config
	flag.StringVar(&cfg.backend, "backend", defaultBackend, "Backend server URL (use 'local' to launch local server)")
	flag.StringVar(&cfg.username, "user", "", "GitHub username to check (defaults to current authenticated user)")
	flag.BoolVar(&cfg.verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&cfg.noCache, "no-cache", false, "Disable caching and fetch fresh data")
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	cfg.prURL = flag.Arg(0)

	// Validate PR URL
	if err := validatePRURL(cfg.prURL); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type config struct {
	backend  string
	username string
	verbose  bool
	noCache  bool
	prURL    string
}

func run(cfg config) error {
	var logger *log.Logger
	if cfg.verbose {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	} else {
		logger = log.New(io.Discard, "", 0)
	}

	// Handle local backend mode
	var serverCmd *exec.Cmd
	var interrupted = make(chan struct{}) // Signal handler notification
	isLocalBackend := cfg.backend == "local"
	if isLocalBackend {
		port, cmd, err := startLocalServer(logger)
		if err != nil {
			return fmt.Errorf("starting local server: %w", err)
		}
		serverCmd = cmd
		cfg.backend = fmt.Sprintf("http://localhost:%d", port)
		logger.Printf("started local server on port %d", port)

		// Also print to stderr in non-verbose mode so user knows what's happening
		if !cfg.verbose {
			fmt.Fprintf(os.Stderr, "Started local server on port %d\n", port)
		}

		// Log when server exits
		go func() {
			err := cmd.Wait()
			if err != nil {
				logger.Printf("server process exited with error: %v", err)
			} else {
				logger.Printf("server process exited normally")
			}
		}()

		// Ensure server is cleaned up on exit
		defer func() {
			if serverCmd != nil && serverCmd.Process != nil {
				logger.Printf("stopping local server")
				serverCmd.Process.Signal(syscall.SIGTERM)
				// Don't wait here as the monitor goroutine is already waiting
			}
		}()

		// Set up signal handling
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			logger.Printf("received interrupt signal")
			close(interrupted) // Notify main goroutine
			if isLocalBackend && serverCmd != nil && serverCmd.Process != nil {
				serverCmd.Process.Signal(syscall.SIGTERM)
			}
		}()
	}

	logger.Printf("starting check for PR: %s, user: %s, backend: %s", cfg.prURL, cfg.username, cfg.backend)

	// Get GitHub token from environment or gh CLI
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		// Try gh CLI
		cmd := exec.Command("gh", "auth", "token")
		cmd.Stderr = io.Discard
		if output, err := cmd.Output(); err == nil {
			token = strings.TrimSpace(string(output))
		}
	}
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
		if cfg.username == "" {
			ctx, cancel := context.WithTimeout(context.Background(), userAuthTimeout)
			defer cancel()
			user, err := client.CurrentUser(ctx)
			if err != nil {
				logger.Printf("failed to auto-detect user: %v", err)
				return fmt.Errorf("failed to auto-detect GitHub user: %v\n"+
					"Please specify --user=<username> to check a specific user.", err)
			}
			cfg.username = user
			logger.Printf("auto-detected user: %s", cfg.username)
		}
	}
	if cfg.noCache {
		client.SetNoCache(true)
	}

	// Create a cancellable context for the request
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	logger.Printf("request timeout set to %v", requestTimeout)

	// Handle interruption
	go func() {
		select {
		case <-interrupted:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Make the request
	logger.Printf("sending check request to backend")
	start := time.Now()
	result, err := client.Check(ctx, cfg.prURL, cfg.username, time.Now())
	elapsed := time.Since(start)
	logger.Printf("check request completed in %v", elapsed)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("request timed out after %v", requestTimeout)
		}
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("interrupted")
		}
		return fmt.Errorf("checking PR: %w", err)
	}

	blockingActions := len(result.PRState.UnblockAction)
	logger.Printf("check completed successfully: %d blocking actions found", blockingActions)
	if blockingActions > 0 {
		for user, action := range result.PRState.UnblockAction {
			logger.Printf("  - %s: %s (critical: %v, reason: %s)", user, action.Kind, action.Critical, action.Reason)
		}
	}

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

// validatePRURL validates that the given URL is a valid GitHub PR URL.
func validatePRURL(prURL string) error {
	if prURL == "" {
		return fmt.Errorf("pr URL cannot be empty")
	}

	u, err := url.Parse(prURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url must use http or https scheme")
	}

	if u.Host != "github.com" && u.Host != "www.github.com" {
		return fmt.Errorf("url must be a GitHub URL")
	}

	if !prURLPattern.MatchString(u.Path) {
		return fmt.Errorf("url must be a GitHub pull request URL (e.g., https://github.com/owner/repo/pull/123)")
	}

	return nil
}


// startLocalServer starts the turnserver as a subprocess on port 0 and returns the actual port
func startLocalServer(logger *log.Logger) (int, *exec.Cmd, error) {
	// Server is expected to be at ../server relative to client
	sourceDir := "../server"
	if _, err := os.Stat(filepath.Join(sourceDir, "cmd/server/main.go")); err != nil {
		return 0, nil, fmt.Errorf("server source not found at %s: %w", sourceDir, err)
	}

	// Get a free port by binding to port 0
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, nil, fmt.Errorf("finding free port: %w", err)
	}
	defer listener.Close() // Close so the server can bind to it

	// Safe type assertion with error checking
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, nil, fmt.Errorf("listener address is not TCP address: %T", listener.Addr())
	}
	port := tcpAddr.Port

	// Start the server using go run to ensure latest code
	logger.Printf("starting server on port %d", port)
	cmd := exec.Command("go", "run", "./cmd/server", fmt.Sprintf("--port=%d", port))
	cmd.Dir = sourceDir

	// Capture server output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close() // Clean up the stdout pipe
		return 0, nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	// Start the server
	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("starting server: %w", err)
	}

	// Forward server output to logger
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			logger.Printf("[server] %s", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Printf("[server] stdout scanner error: %v", err)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logger.Printf("[server] %s", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Printf("[server] stderr scanner error: %v", err)
		}
	}()

	// Wait for server to be ready
	ctx, cancel := context.WithTimeout(context.Background(), serverStartTimeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				if err := cmd.Process.Kill(); err != nil {
					logger.Printf("failed to kill server process: %v", err)
				}
			}
			return 0, nil, fmt.Errorf("server failed to start within %s", serverStartTimeout)
		case <-ticker.C:
			// Try to connect to the server
			conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", port))
			if err == nil {
				conn.Close()
				// Server is ready
				return port, cmd, nil
			}
			// Check if process is still running
			if cmd.ProcessState != nil {
				return 0, nil, fmt.Errorf("server process exited unexpectedly")
			}
		}
	}
}
