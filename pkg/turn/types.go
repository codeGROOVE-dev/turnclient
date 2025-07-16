package turn

import "time"

// CheckRequest represents a request to check if a PR is blocked by a user
type CheckRequest struct {
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"` // Last known update time of the PR (required)
}

type Action struct {
	Kind         string
	CriticalPath bool
}

// CheckResponse represents the response from a PR check
type CheckResponse struct {
	NextAction map[string]Action // represents the next action expected out of a particular user (typically reviewer or author, may be person cited in comment)
	UpdatedAt  time.Time         // Last time the PR was updated

	// Recent activity metadata
	RecentActivity struct {
		Type      string    `json:"type"`      // "commit", "comment", "review", "review_comment"
		Author    string    `json:"author"`    // Username who performed the action
		Message   string    `json:"message"`   // Commit message or comment excerpt
		Timestamp time.Time `json:"timestamp"` // When it happened
	} `json:"recent_activity"`

	// Debugging info
	FailingTests       int `json:"failing_tests"`       // Number of failing tests
	UnresolvedComments int `json:"unresolved_comments"` // Number of unresolved PR comments

	// Status flags
	IsDraft      bool `json:"is_draft"`       // PR is in draft state
	ReadyToMerge bool `json:"ready_to_merge"` // PR can be merged

	// Tags - string representation of status flags for easier filtering/display
	Tags []string `json:"tags"` // e.g., ["draft", "merge_conflict", "has_approval"]
}
