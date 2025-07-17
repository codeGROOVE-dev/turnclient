package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
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
	backend := flag.String("backend", defaultBackend, "Backend server URL")
	username := flag.String("user", "", "GitHub username to check (defaults to current authenticated user)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	flag.Parse()

	logger := log.New(os.Stderr, "[checkurl] ", log.LstdFlags|log.Lshortfile)
	if !*verbose {
		logger.SetOutput(io.Discard)
	}

	args := flag.Args()
	if len(args) != 1 {
		printUsage()
		os.Exit(1)
	}

	prURL := args[0]
	logger.Printf("checking PR: %s", prURL)

	token := getGitHubToken()
	if token == "" {
		logger.Println("no GitHub token found")
		if *username == "" {
			fmt.Fprintln(os.Stderr, "error: no GitHub token found and no username specified")
			fmt.Fprintln(os.Stderr, "To authenticate, run 'gh auth login' or set GITHUB_TOKEN environment variable.")
			fmt.Fprintln(os.Stderr, "Alternatively, specify --user=<username> to check a specific user.")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "warning: no GitHub token found, API requests may be rate limited")
		fmt.Fprintln(os.Stderr)
	} else {
		logger.Println("GitHub token found")
		if *username == "" {
			user, err := getCurrentUser(token)
			if err != nil {
				logger.Printf("failed to auto-detect user: %v", err)
				fmt.Fprintf(os.Stderr, "error: failed to auto-detect GitHub user: %v\n", err)
				fmt.Fprintln(os.Stderr, "Please specify --user=<username> to check a specific user.")
				os.Exit(1)
			}
			*username = user
			logger.Printf("auto-detected user: %s", *username)
		}
	}

	logger.Printf("creating client for backend: %s", *backend)
	client, err := turn.NewClient(*backend)
	if err != nil {
		logger.Printf("failed to create client: %v", err)
		fmt.Fprintf(os.Stderr, "error creating client: %v\n", err)
		os.Exit(1)
	}
	if *verbose {
		client.SetLogger(logger)
	}
	if token != "" {
		client.SetAuthToken(token)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	logger.Printf("sending check request")
	result, err := client.Check(ctx, prURL, time.Now())
	if err != nil {
		logger.Printf("check failed: %v", err)
		fmt.Fprintf(os.Stderr, "error checking PR: %v\n", err)
		os.Exit(1)
	}

	totalActions := len(result.NextAction)
	criticalActions := 0
	for _, action := range result.NextAction {
		if action.CriticalPath {
			criticalActions++
		}
	}
	logger.Printf("check successful: %d total actions (%d critical)", totalActions, criticalActions)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		logger.Printf("failed to format response: %v", err)
		fmt.Fprintf(os.Stderr, "error formatting response: %v\n", err)
		os.Exit(1)
	}

	if totalActions > 0 {
		os.Exit(1)
	}
}

func printUsage() {
	progName := os.Args[0]
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <github-pr-url>\n\n", progName)
	fmt.Fprintf(os.Stderr, "Options:\n")
	fmt.Fprintf(os.Stderr, "  --backend=<url>    Backend server URL (default: %s)\n", defaultBackend)
	fmt.Fprintf(os.Stderr, "  --user=<username>  GitHub username to check (default: current authenticated user)\n")
	fmt.Fprintf(os.Stderr, "  --verbose          Enable verbose logging\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  %s https://github.com/owner/repo/pull/123\n", progName)
	fmt.Fprintf(os.Stderr, "  %s --backend=https://api.example.com https://github.com/owner/repo/pull/123\n", progName)
	fmt.Fprintf(os.Stderr, "  %s --user=octocat https://github.com/owner/repo/pull/123\n", progName)
	fmt.Fprintf(os.Stderr, "  %s --verbose https://github.com/owner/repo/pull/123\n", progName)
}

func getGitHubToken() string {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return strings.TrimSpace(token)
	}
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return strings.TrimSpace(token)
	}
	cmd := exec.Command("gh", "auth", "token")
	cmd.Stderr = io.Discard
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func getCurrentUser(token string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("no github token available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), userAuthTimeout)
	defer cancel()
	
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github API returned status %d", resp.StatusCode)
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", err
	}
	return user.Login, nil
}