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
		PRState: PRState{
			UnblockAction: map[string]Action{
				"user1": {Kind: "REVIEW", Critical: true, Reason: "needs to review"},
				"user2": {Kind: "APPROVE", Critical: false, Reason: "needs to approve"},
			},
			UpdatedAt: now,
			LastActivity: LastActivity{
				Kind:      "comment",
				Author:    "testuser",
				Message:   "Please review",
				Timestamp: now,
			},
			Checks:             Checks{Failing: 2},
			UnresolvedComments: 1,
			Draft:              false,
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
	if len(decoded.PRState.UnblockAction) != len(resp.PRState.UnblockAction) {
		t.Errorf("UnblockAction length = %d, want %d", len(decoded.PRState.UnblockAction), len(resp.PRState.UnblockAction))
	}
	for user, action := range resp.PRState.UnblockAction {
		decodedAction := decoded.PRState.UnblockAction[user]
		if decodedAction.Kind != action.Kind {
			t.Errorf("UnblockAction[%s].Kind = %s, want %s", user, decodedAction.Kind, action.Kind)
		}
		if decodedAction.Critical != action.Critical {
			t.Errorf("UnblockAction[%s].Critical = %v, want %v", user, decodedAction.Critical, action.Critical)
		}
	}

	// Verify recent activity
	if decoded.PRState.LastActivity.Kind != resp.PRState.LastActivity.Kind {
		t.Errorf("LastActivity.Kind = %s, want %s", decoded.PRState.LastActivity.Kind, resp.PRState.LastActivity.Kind)
	}

	// Verify debugging info
	if decoded.PRState.Checks.Failing != resp.PRState.Checks.Failing {
		t.Errorf("Checks.Failing = %d, want %d", decoded.PRState.Checks.Failing, resp.PRState.Checks.Failing)
	}
	if decoded.PRState.UnresolvedComments != resp.PRState.UnresolvedComments {
		t.Errorf("UnresolvedComments = %d, want %d", decoded.PRState.UnresolvedComments, resp.PRState.UnresolvedComments)
	}

	// Verify flags
	if decoded.PRState.ReadyToMerge != resp.PRState.ReadyToMerge {
		t.Errorf("ReadyToMerge = %v, want %v", decoded.PRState.ReadyToMerge, resp.PRState.ReadyToMerge)
	}
	if decoded.PRState.Draft != resp.PRState.Draft {
		t.Errorf("Draft = %v, want %v", decoded.PRState.Draft, resp.PRState.Draft)
	}

	// Verify server info
	if decoded.Timestamp.Unix() != resp.Timestamp.Unix() {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, resp.Timestamp)
	}
	if decoded.Commit != resp.Commit {
		t.Errorf("Commit = %s, want %s", decoded.Commit, resp.Commit)
	}
}
