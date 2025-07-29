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
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ready-to-review/turnclient/pkg/turn"
)

const (
	defaultBackend     = "https://turn.ready-to-review.dev"
	requestTimeout     = 30 * time.Second
	localRequestTimeout = 10 * time.Second  // Shorter timeout for local server
	userAuthTimeout    = 10 * time.Second
	serverStartTimeout = 5 * time.Second
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
	var serverCrashed = make(chan error, 1)
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
		
		// Monitor server health
		go func() {
			err := serverCmd.Wait()
			if err != nil {
				logger.Printf("server process exited with error: %v", err)
				serverCrashed <- fmt.Errorf("local server crashed: %w", err)
			} else {
				logger.Printf("server process exited normally")
				serverCrashed <- fmt.Errorf("local server exited unexpectedly")
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
		
		// Also handle signals for clean shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			if serverCmd != nil && serverCmd.Process != nil {
				serverCmd.Process.Signal(syscall.SIGTERM)
			}
			os.Exit(1)
		}()
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
	if cfg.noCache {
		client.SetNoCache(true)
	}

	// Use shorter timeout for local backend
	timeout := requestTimeout
	if isLocalBackend {
		timeout = localRequestTimeout
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create a channel to receive the result
	type checkResult struct {
		resp *turn.CheckResponse
		err  error
	}
	resultChan := make(chan checkResult, 1)

	// Run the check in a goroutine
	go func() {
		resp, err := client.Check(ctx, cfg.prURL, cfg.username, time.Now())
		resultChan <- checkResult{resp, err}
	}()

	// Wait for either the result or server crash
	var result *turn.CheckResponse
	if isLocalBackend {
		select {
		case res := <-resultChan:
			if res.err != nil {
				return fmt.Errorf("checking PR: %w", res.err)
			}
			result = res.resp
		case err := <-serverCrashed:
			cancel() // Cancel the pending request
			return err
		case <-ctx.Done():
			return fmt.Errorf("request timed out after %v", timeout)
		}
	} else {
		select {
		case res := <-resultChan:
			if res.err != nil {
				return fmt.Errorf("checking PR: %w", res.err)
			}
			result = res.resp
		case <-ctx.Done():
			return fmt.Errorf("request timed out after %v", timeout)
		}
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

// startLocalServer starts the turnserver as a subprocess on port 0 and returns the actual port
func startLocalServer(logger *log.Logger) (int, *exec.Cmd, error) {
	// Find the server source directory
	sourceDirs := []string{
		"../server",
		"../../server",
	}
	
	var sourceDir string
	for _, dir := range sourceDirs {
		if _, err := os.Stat(filepath.Join(dir, "cmd/server/main.go")); err == nil {
			sourceDir = dir
			logger.Printf("found server source at %s", sourceDir)
			break
		}
	}
	
	if sourceDir == "" {
		return 0, nil, fmt.Errorf("could not find server source directory")
	}
	
	// Get a free port by binding to port 0
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, nil, fmt.Errorf("finding free port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close() // Close so the server can bind to it
	
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
	}()
	
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logger.Printf("[server] %s", scanner.Text())
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
			cmd.Process.Kill()
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