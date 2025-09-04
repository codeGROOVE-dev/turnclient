// Package turn provides types and client functionality for the Turn API service.
//
//nolint:revive // API types need to be public
package turn

import (
	"time"

	"github.com/ready-to-review/prx/pkg/prx"
)

// CheckRequest represents a request to check if a PR is blocked by a user.
type CheckRequest struct {
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"` // Last known update time of the PR (required)
	User      string    `json:"user"`
}

// Action represents an expected action from a specific user.
type Action struct {
	Kind        string     `json:"kind"`
	Critical    bool       `json:"critical"`
	Reason      string     `json:"reason"`
	NotifyAfter *time.Time `json:"notify_after,omitempty"`
}

// LastActivity represents the most recent activity on a PR.
type LastActivity struct {
	Kind      string    `json:"kind"`      // "commit", "comment", "review", "review_comment"
	Actor     string    `json:"actor"`     // Username who performed the action
	Message   string    `json:"message"`   // Commit message or comment excerpt
	Timestamp time.Time `json:"timestamp"` // When it happened
}

// Checks represents the status of CI checks for a pull request.
type Checks struct {
	Total   int `json:"total"`   // number of checks associated to this PR
	Failing int `json:"failing"` // number of checks that failed
	Waiting int `json:"waiting"` // waiting for a deployment protection rule to be satisfied.
	Pending int `json:"pending"` // Pending execution (effectively: total - failing - waiting - passing)
	Passing int `json:"passing"` // Number of checks that passed
	Ignored int `json:"ignored"` // Number of failing tests we ignored
}

// StateTransition represents a state change based on an event.
type StateTransition struct {
	FromState     string    `json:"from_state"`
	ToState       string    `json:"to_state"`
	Timestamp     time.Time `json:"timestamp"`
	TriggerEvent  string    `json:"trigger_event"`
	LastEventKind string    `json:"last_event_kind"` // The last event kind seen before this transition
}

// AnalysisState represents the computed analysis state of a PR.
type AnalysisState struct {
	NextAction map[string]Action `json:"next_action"` // Next action for each user to move the PR forward
	LastActivity  LastActivity      `json:"last_activity"`  // Most recent activity

	Checks             Checks `json:"checks"` // Check states
	UnresolvedComments int    `json:"unresolved_comments"`

	Size string `json:"size"` // XXS, XS, S, M, L, XL, XXL, INSANE

	// Status
	ReadyToMerge  bool `json:"ready_to_merge"`
	MergeConflict bool `json:"merge_conflict"`
	Approved      bool `json:"approved"`

	Tags []string `json:"tags"` // e.g., ["draft", "merge_conflict", "approved"]

	// State duration tracking
	StateDurations   map[string]int    `json:"state_durations,omitempty"`   // Cumulative seconds spent in each state
	WorkflowState    string            `json:"workflow_state,omitempty"`    // Current workflow state (e.g. "PUBLISHED_WAITING_FOR_TESTS")
	StateTransitions []StateTransition `json:"state_transitions,omitempty"` // List of state transitions
}

// CheckResponse represents the response from a PR check.
type CheckResponse struct {
	PullRequest prx.PullRequest `json:"pull_request"`
	Analysis    AnalysisState   `json:"analysis"`
	Timestamp   time.Time       `json:"timestamp"` // Server generation time
	Commit      string          `json:"commit"`    // Server version
}
