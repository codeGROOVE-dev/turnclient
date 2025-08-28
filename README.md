# turnclient

A command-line tool to check if a GitHub pull request is blocked by a specific user.

## Installation

```bash
go install github.com/codeGROOVE-dev/turnclient/cmd/checkurl@latest
```

## Usage

```bash
checkurl [options] <github-pr-url>

Options:
  --backend=<url>    Backend server URL (default: http://localhost:8080)
  --user=<username>  GitHub username to check (default: current authenticated user)
  --verbose          Enable verbose logging
```

## Examples

Check if a PR is blocked by the current authenticated user:
```bash
checkurl https://github.com/owner/repo/pull/123
```

Check if a PR is blocked by a specific user:
```bash
checkurl --user=octocat https://github.com/owner/repo/pull/123
```

Use a different backend server:
```bash
checkurl --backend=https://api.example.com https://github.com/owner/repo/pull/123
```

## Authentication

The tool uses GitHub authentication to:
- Automatically detect the current user (when --user is not specified)
- Make authenticated API requests to avoid rate limits

Authentication methods (in order of precedence):
1. `GITHUB_TOKEN` environment variable
2. GitHub CLI (`gh auth token`)

To authenticate:
```bash
# Option 1: Set environment variable
export GITHUB_TOKEN=your_token_here

# Option 2: Use GitHub CLI
gh auth login
```

## Development

### Building

```bash
go build ./cmd/checkurl
```

### Testing

```bash
go test ./...
```

## License

See [LICENSE](LICENSE) file.