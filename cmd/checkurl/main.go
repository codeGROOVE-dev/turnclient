// Command checkurl checks if a GitHub pull request is blocked by a specific user.
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

func main() {
	// Configure logging
	logger := log.New(os.Stderr, "[checkurl] ", log.LstdFlags)
	
	// Define flags
	var (
		backend string
		username string
		verbose bool
	)
	flag.StringVar(&backend, "backend", "http://localhost:8080", "Backend server URL")
	flag.StringVar(&username, "user", "", "GitHub username to check (defaults to current authenticated user)")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()

	// Configure logger verbosity
	if !verbose {
		logger.SetOutput(io.Discard)
	}

	// Get remaining args (should be the PR URL)
	args := flag.Args()
	if len(args) != 1 {
		printUsage()
		os.Exit(1)
	}

	prURL := args[0]
	logger.Printf("checking PR: %s", prURL)

	// Get GitHub auth token
	token := getGitHubToken()
	if token == "" {
		logger.Println("no GitHub token found")
		fmt.Fprintln(os.Stderr, "Warning: No GitHub token found. API requests may be rate limited.")
		fmt.Fprintln(os.Stderr, "To authenticate, run 'gh auth login' or set GITHUB_TOKEN environment variable.")
		fmt.Fprintln(os.Stderr)
	} else {
		logger.Println("GitHub token found")
	}

	// Determine username - use flag value or get from GitHub API
	if username == "" {
		if token == "" {
			fmt.Fprintln(os.Stderr, "Error: No username specified and no GitHub token available.")
			fmt.Fprintln(os.Stderr, "Either specify --user=<username> or authenticate with GitHub.")
			os.Exit(1)
		}
		
		logger.Println("fetching current user from GitHub API")
		
		// Get current user from GitHub API
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		currentUser, err := turn.GetCurrentUser(ctx, token)
		if err != nil {
			logger.Printf("failed to get current user: %v", err)
			fmt.Fprintf(os.Stderr, "Error getting current GitHub user: %v\n", err)
			os.Exit(1)
		}
		username = currentUser
		logger.Printf("using authenticated user: %s", username)
		fmt.Printf("Using authenticated user: %s\n\n", username)
	} else {
		logger.Printf("using specified user: %s", username)
	}

	// Create Turn client
	logger.Printf("creating client for backend: %s", backend)
	client, err := turn.NewClient(backend)
	if err != nil {
		logger.Printf("failed to create client: %v", err)
		fmt.Fprintf(os.Stderr, "Error creating client: %v\n", err)
		os.Exit(1)
	}
	
	if verbose {
		client.SetLogger(logger)
	}
	
	if token != "" {
		client.SetAuthToken(token)
	}

	// Make the check request
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Printf("sending check request")
	result, err := client.Check(ctx, prURL, username)
	if err != nil {
		logger.Printf("check failed: %v", err)
		fmt.Fprintf(os.Stderr, "Error checking PR: %v\n", err)
		os.Exit(1)
	}

	logger.Printf("check successful: status=%d", result.Status)

	// Pretty-print the JSON response
	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		logger.Printf("failed to format response: %v", err)
		fmt.Fprintf(os.Stderr, "Error formatting response: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(prettyJSON))
}

// printUsage prints the usage information to stderr.
func printUsage() {
	progName := os.Args[0]
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <github-pr-url>\n\n", progName)
	fmt.Fprintf(os.Stderr, "Options:\n")
	fmt.Fprintf(os.Stderr, "  --backend=<url>    Backend server URL (default: http://localhost:8080)\n")
	fmt.Fprintf(os.Stderr, "  --user=<username>  GitHub username to check (default: current authenticated user)\n")
	fmt.Fprintf(os.Stderr, "  --verbose          Enable verbose logging\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  %s https://github.com/owner/repo/pull/123\n", progName)
	fmt.Fprintf(os.Stderr, "  %s --backend=https://api.example.com https://github.com/owner/repo/pull/123\n", progName)
	fmt.Fprintf(os.Stderr, "  %s --user=octocat https://github.com/owner/repo/pull/123\n", progName)
	fmt.Fprintf(os.Stderr, "  %s --verbose https://github.com/owner/repo/pull/123\n", progName)
}

// getGitHubToken attempts to retrieve a GitHub authentication token.
// It first checks the GITHUB_TOKEN environment variable, then falls back
// to using the gh CLI tool if available.
func getGitHubToken() string {
	// First, try environment variable
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}

	// Second, try gh auth token command
	cmd := exec.Command("gh", "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}