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
		NextAction: map[string]Action{
			"user1": {Kind: "REVIEW", CriticalPath: true},
			"user2": {Kind: "APPROVE", CriticalPath: false},
		},
		UpdatedAt: now,
		RecentActivity: struct {
			Type      string    `json:"type"`
			Author    string    `json:"author"`
			Message   string    `json:"message"`
			Timestamp time.Time `json:"timestamp"`
		}{
			Type:      "comment",
			Author:    "testuser",
			Message:   "Please review",
			Timestamp: now,
		},
		FailingTests:       2,
		UnresolvedComments: 1,
		IsDraft:            false,
		ReadyToMerge:       false,
		Tags:               []string{"has_approval"},
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
	if len(decoded.NextAction) != len(resp.NextAction) {
		t.Errorf("NextAction length = %d, want %d", len(decoded.NextAction), len(resp.NextAction))
	}
	for user, action := range resp.NextAction {
		decodedAction := decoded.NextAction[user]
		if decodedAction.Kind != action.Kind {
			t.Errorf("NextAction[%s].Kind = %s, want %s", user, decodedAction.Kind, action.Kind)
		}
		if decodedAction.CriticalPath != action.CriticalPath {
			t.Errorf("NextAction[%s].CriticalPath = %v, want %v", user, decodedAction.CriticalPath, action.CriticalPath)
		}
	}

	// Verify recent activity
	if decoded.RecentActivity.Type != resp.RecentActivity.Type {
		t.Errorf("RecentActivity.Type = %s, want %s", decoded.RecentActivity.Type, resp.RecentActivity.Type)
	}

	// Verify debugging info
	if decoded.FailingTests != resp.FailingTests {
		t.Errorf("FailingTests = %d, want %d", decoded.FailingTests, resp.FailingTests)
	}
	if decoded.UnresolvedComments != resp.UnresolvedComments {
		t.Errorf("UnresolvedComments = %d, want %d", decoded.UnresolvedComments, resp.UnresolvedComments)
	}
	
	// Verify flags
	if decoded.ReadyToMerge != resp.ReadyToMerge {
		t.Errorf("ReadyToMerge = %v, want %v", decoded.ReadyToMerge, resp.ReadyToMerge)
	}
	if decoded.IsDraft != resp.IsDraft {
		t.Errorf("IsDraft = %v, want %v", decoded.IsDraft, resp.IsDraft)
	}
}