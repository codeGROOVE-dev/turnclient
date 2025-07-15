package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"wriia/internal/server"
	"wriia/pkg/turn"
)

func main() {
	// Define flags
	var backend string
	var username string
	flag.StringVar(&backend, "backend", "", "Backend to use: 'local' for local server, or URL for remote server")
	flag.StringVar(&username, "user", "", "GitHub username to check (defaults to current authenticated user)")
	flag.Parse()

	// Get remaining args (should be the PR URL)
	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [--backend=<local|url>] [--user=<username>] <github-pr-url>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s https://github.com/owner/repo/pull/123\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s --backend=local https://github.com/owner/repo/pull/123\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s --user=octocat https://github.com/owner/repo/pull/123\n", os.Args[0])
		os.Exit(1)
	}

	prURL := args[0]

	// Get GitHub auth token
	token := getGitHubToken()
	if token == "" {
		fmt.Fprintf(os.Stderr, "Warning: No GitHub token found. API requests may be rate limited.\n")
		fmt.Fprintf(os.Stderr, "To authenticate, run 'gh auth login' or set GITHUB_TOKEN environment variable.\n\n")
	}

	// Determine username - use flag value or get from GitHub API
	if username == "" {
		if token == "" {
			fmt.Fprintf(os.Stderr, "Error: No username specified and no GitHub token available.\n")
			fmt.Fprintf(os.Stderr, "Either specify --user=<username> or authenticate with GitHub.\n")
			os.Exit(1)
		}
		
		// Get current user from GitHub API
		ctx := context.Background()
		currentUser, err := turn.GetCurrentUser(ctx, token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting current GitHub user: %v\n", err)
			os.Exit(1)
		}
		username = currentUser
		fmt.Printf("Using authenticated user: %s\n\n", username)
	}

	// Determine server URL
	var serverURL string
	var localServer *turn.LocalServer

	if backend == "local" {
		// Start local server on random port
		srv := server.New()
		ls, err := turn.StartLocalServer(srv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting local server: %v\n", err)
			os.Exit(1)
		}
		localServer = ls
		serverURL = ls.URL()
		defer func() {
			// Shutdown server after request
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			localServer.Shutdown(ctx)
		}()
	} else if backend != "" {
		// Use provided backend URL
		serverURL = backend
	} else if envURL := os.Getenv("WRIIA_SERVER_URL"); envURL != "" {
		// Use environment variable
		serverURL = envURL
	} else {
		// Default to localhost:8080
		serverURL = "http://localhost:8080"
	}

	// Create Turn client
	client := turn.NewClient(serverURL)
	if token != "" {
		client.SetAuthToken(token)
	}

	// Make the check request
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := client.Check(ctx, prURL, username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking PR: %v\n", err)
		os.Exit(1)
	}

	// Pretty-print the JSON response
	prettyJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting response: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(prettyJSON))
}

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