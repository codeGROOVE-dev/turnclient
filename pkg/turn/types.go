package turn

import (
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx"
)

// ActionKind represents the type of action required from a user.
type ActionKind string

// Action constants.
const (
	ActionResolveComments  ActionKind = "resolve_comments"
	ActionPublishDraft     ActionKind = "publish_draft"
	ActionRequestReviewers ActionKind = "request_reviewers"
	ActionReview           ActionKind = "review"
	ActionReReview         ActionKind = "re_review"
	ActionReviewDiscussion ActionKind = "review_discussion" // Respond to discussion/questions without code changes
	ActionApprove          ActionKind = "approve"           // Formally approve PR (for LGTM scenarios)
	ActionFixTests         ActionKind = "fix_tests"
	ActionTestsPending     ActionKind = "tests_pending"
	ActionRerunTests       ActionKind = "rerun_tests"
	ActionRespond          ActionKind = "respond"
	ActionFixConflict      ActionKind = "fix_conflict"
	ActionMerge            ActionKind = "merge"
)

// WorkflowState represents the current state of a PR in the workflow.
type WorkflowState string

// Workflow state constants for tracking time spent in each state.
const (
	StateNewlyPublished             WorkflowState = "NEWLY_PUBLISHED"
	StateInDraft                    WorkflowState = "IN_DRAFT"
	StatePublishedWaitingForTests   WorkflowState = "PUBLISHED_WAITING_FOR_TESTS"
	StateTestedWaitingForFixes      WorkflowState = "TESTED_WAITING_FOR_FIXES"
	StateTestedWaitingForAssignment WorkflowState = "TESTED_WAITING_FOR_ASSIGNMENT"
	StateAssignedWaitingForReview   WorkflowState = "ASSIGNED_WAITING_FOR_REVIEW"
	StateReviewedNeedsRefinement    WorkflowState = "REVIEWED_NEEDS_REFINEMENT"
	StateRefinedWaitingForApproval  WorkflowState = "REFINED_WAITING_FOR_APPROVAL"
	StateApprovedWaitingForMerge    WorkflowState = "APPROVED_WAITING_FOR_MERGE"
)

// CheckRequest represents a request to check if a PR is blocked by a user.
type CheckRequest struct {
	URL           string    `json:"url"`
	UpdatedAt     time.Time `json:"updated_at"` // Last known update time of the PR (required)
	User          string    `json:"user"`
	IncludeEvents bool      `json:"include_events,omitempty"` // Include full event list from prx (defaults to false)
}

// Action represents an expected action from a specific user.
type Action struct {
	Since    time.Time  `json:"since"`
	Kind     ActionKind `json:"kind"`
	Reason   string     `json:"reason"`
	Critical bool       `json:"critical"`
}

// LastActivity represents the most recent activity on a PR.
type LastActivity struct {
	Timestamp time.Time `json:"timestamp"`
	Kind      string    `json:"kind"`
	Actor     string    `json:"actor"`
	Message   string    `json:"message"`
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

// Analysis represents the computed analysis of a PR.
type Analysis struct {
	LastActivity       LastActivity      `json:"last_activity"`
	NextAction         map[string]Action `json:"next_action"`
	SecondsInState     map[string]int    `json:"seconds_in_state,omitempty"`
	Size               string            `json:"size"`
	WorkflowState      string            `json:"workflow_state,omitempty"`
	Tags               []string          `json:"tags"`
	StateTransitions   []StateTransition `json:"state_transitions,omitempty"`
	Checks             Checks            `json:"checks"`
	UnresolvedComments int               `json:"unresolved_comments"`
	ReadyToMerge       bool              `json:"ready_to_merge"`
	MergeConflict      bool              `json:"merge_conflict"`
	Approved           bool              `json:"approved"`
}

// CheckResponse represents the response from a PR check.
type CheckResponse struct {
	Timestamp   time.Time       `json:"timestamp"`
	Commit      string          `json:"commit"`
	PullRequest prx.PullRequest `json:"pull_request"`
	Analysis    Analysis        `json:"analysis"`
	Events      []prx.Event     `json:"events,omitempty"` // Full event list from prx (only included if requested)
}
