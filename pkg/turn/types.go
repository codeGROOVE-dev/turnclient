// Package turn provides types and client for the Turn API
package turn

import "time"

// CheckRequest represents a request to check if a PR is blocked by a user
type CheckRequest struct {
	URL      string `json:"url"`
	Username string `json:"username"`
}

// CheckResponse represents the response from a PR check
type CheckResponse struct {
	// Core status information
	Status       int      `json:"status"`        // 0 = not blocked, 1 = blocked
	StatusString string   `json:"status_string"` // Human-readable status
	BlockedBy    []string `json:"blocked_by"`    // Usernames of who need to take action
	Reason       string   `json:"reason"`        // Why the PR is in this state
	NextAction   string   `json:"next_action"`   // What action is needed
	
	// Recent activity metadata
	RecentActivity struct {
		Type        string    `json:"type"`         // "commit", "comment", "review", "review_comment"
		Author      string    `json:"author"`       // Username who performed the action
		Message     string    `json:"message"`      // Commit message or comment excerpt
		Timestamp   time.Time `json:"timestamp"`    // When it happened
	} `json:"recent_activity"`
	
	// Test status counts
	TestsPassing int `json:"tests_passing"`
	TestsPending int `json:"tests_pending"`
	TestsFailing int `json:"tests_failing"`
	
	// PR metrics
	CommentCount       int `json:"comment_count"`        // Total comments on PR
	ReviewCount        int `json:"review_count"`         // Number of reviews
	CommitCount        int `json:"commit_count"`         // Number of commits
	ChangedFilesCount  int `json:"changed_files_count"`  // Number of files changed
	
	// Status flags
	MergeConflict      bool `json:"merge_conflict"`       // PR has merge conflicts
	IsDraft            bool `json:"is_draft"`             // PR is in draft state
	HasApproval        bool `json:"has_approval"`         // PR has at least one approval
	ChangesRequested   bool `json:"changes_requested"`    // Changes have been requested
	AllChecksPassing   bool `json:"all_checks_passing"`   // All CI checks are passing
	ReadyToMerge       bool `json:"ready_to_merge"`       // PR can be merged
}