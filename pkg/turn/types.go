package turn

import "time"

// CheckRequest represents a request to check if a PR is blocked by a user.
type CheckRequest struct {
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"` // Last known update time of the PR (required)
	User      string    `json:"user"`
}

// Action represents an expected action from a specific user.
type Action struct {
	Kind          string `json:"kind"`
	Critical      bool   `json:"critical"`
	Reason        string `json:"reason"`
	ReadyToNotify bool   `json:"ready_to_notify"`
}

// LastActivity represents the most recent activity on a PR.
type LastActivity struct {
	Kind      string    `json:"kind"`      // "commit", "comment", "review", "review_comment"
	Author    string    `json:"author"`    // Username who performed the action
	Message   string    `json:"message"`   // Commit message or comment excerpt
	Timestamp time.Time `json:"timestamp"` // When it happened
}

type Checks struct {
	Total   int `json:"total"`   // number of checks associated to this PR
	Failing int `json:"failing"` // number of checks that failed
	Waiting int `json:"waiting"` // waiting for a deployment protection rule to be satisfied.
	Pending int `json:"pending"` // Pending execution (effectively: total - failing - waiting - passing)
	Passing int `json:"passing"` // Number of checks that passed
	Ignored int `json:"ignored"` // Number of failing tests we ignored
}

// PRState represents the current state of a PR.
type PRState struct {
	UnblockAction map[string]Action `json:"unblock_action"` // Next action expected from each user
	UpdatedAt     time.Time         `json:"updated_at"`     // Last time the PR was updated
	LastActivity  LastActivity      `json:"last_activity"`  // Most recent activity

	Checks             Checks `json:"checks"` // Check states
	UnresolvedComments int    `json:"unresolved_comments"`

	Size string `json:"size"` // XXS, XS, S, M, L, XL, XXL, INSANE

	// Status
	Draft         bool `json:"draft"`
	ReadyToMerge  bool `json:"ready_to_merge"`
	MergeConflict bool `json:"merge_conflict"`
	Approved      bool `json:"approved"`

	Tags []string `json:"tags"` // e.g., ["draft", "merge_conflict", "approved"]

	// State duration tracking
	StateDurations map[string]int `json:"state_durations,omitempty"` // Cumulative seconds spent in each state
	CurrentState   string         `json:"current_state,omitempty"`   // Current state the PR is in
}

// CheckResponse represents the response from a PR check.
type CheckResponse struct {
	PRState   PRState   `json:"pr_state"`
	Timestamp time.Time `json:"timestamp"` // Server generation time
	Commit    string    `json:"commit"`    // Server version
}
