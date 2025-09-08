package turn

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCheckRequestJSON(t *testing.T) {
	req := CheckRequest{
		URL:       "https://github.com/owner/repo/pull/123",
		UpdatedAt: time.Now(),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal CheckRequest: %v", err)
	}

	var decoded CheckRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal CheckRequest: %v", err)
	}

	if decoded.URL != req.URL {
		t.Errorf("URL = %s, want %s", decoded.URL, req.URL)
	}
	if !decoded.UpdatedAt.Equal(req.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want %v", decoded.UpdatedAt, req.UpdatedAt)
	}
}

func TestCheckResponseJSON(t *testing.T) {
	now := time.Now()
	resp := CheckResponse{
		Analysis: Analysis{
			NextAction: map[string]Action{
				"user1": {Kind: "REVIEW", Critical: true, Reason: "needs to review"},
				"user2": {Kind: "APPROVE", Critical: false, Reason: "needs to approve"},
			},
			LastActivity: LastActivity{
				Kind:      "comment",
				Actor:     "testuser",
				Message:   "Please review",
				Timestamp: now,
			},
			Checks:             Checks{Failing: 2},
			UnresolvedComments: 1,
			ReadyToMerge:       false,
			Tags:               []string{"has_approval"},
		},
		Timestamp: now,
		Commit:    "test-commit",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal CheckResponse: %v", err)
	}

	var decoded CheckResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal CheckResponse: %v", err)
	}

	// Verify core fields
	if len(decoded.Analysis.NextAction) != len(resp.Analysis.NextAction) {
		t.Errorf("NextAction length = %d, want %d", len(decoded.Analysis.NextAction), len(resp.Analysis.NextAction))
	}
	for user, action := range resp.Analysis.NextAction {
		decodedAction := decoded.Analysis.NextAction[user]
		if decodedAction.Kind != action.Kind {
			t.Errorf("NextAction[%s].Kind = %s, want %s", user, decodedAction.Kind, action.Kind)
		}
		if decodedAction.Critical != action.Critical {
			t.Errorf("NextAction[%s].Critical = %v, want %v", user, decodedAction.Critical, action.Critical)
		}
	}

	// Verify recent activity
	if decoded.Analysis.LastActivity.Kind != resp.Analysis.LastActivity.Kind {
		t.Errorf("LastActivity.Kind = %s, want %s", decoded.Analysis.LastActivity.Kind, resp.Analysis.LastActivity.Kind)
	}

	// Verify debugging info
	if decoded.Analysis.Checks.Failing != resp.Analysis.Checks.Failing {
		t.Errorf("Checks.Failing = %d, want %d", decoded.Analysis.Checks.Failing, resp.Analysis.Checks.Failing)
	}
	if decoded.Analysis.UnresolvedComments != resp.Analysis.UnresolvedComments {
		t.Errorf("UnresolvedComments = %d, want %d", decoded.Analysis.UnresolvedComments, resp.Analysis.UnresolvedComments)
	}

	// Verify flags
	if decoded.Analysis.ReadyToMerge != resp.Analysis.ReadyToMerge {
		t.Errorf("ReadyToMerge = %v, want %v", decoded.Analysis.ReadyToMerge, resp.Analysis.ReadyToMerge)
	}

	// Verify server info
	if decoded.Timestamp.Unix() != resp.Timestamp.Unix() {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, resp.Timestamp)
	}
	if decoded.Commit != resp.Commit {
		t.Errorf("Commit = %s, want %s", decoded.Commit, resp.Commit)
	}
}
