package turn

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCheckRequestJSON(t *testing.T) {
	req := CheckRequest{
		URL:      "https://github.com/owner/repo/pull/123",
		Username: "testuser",
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
	if decoded.Username != req.Username {
		t.Errorf("Username = %s, want %s", decoded.Username, req.Username)
	}
}

func TestCheckResponseJSON(t *testing.T) {
	now := time.Now()
	resp := CheckResponse{
		Status:       1,
		StatusString: "blocked",
		BlockedBy:    []string{"user1", "user2"},
		Reason:       "awaiting review",
		NextAction:   "review required",
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
		TestsPassing:      10,
		TestsPending:      2,
		TestsFailing:      1,
		CommentCount:      5,
		ReviewCount:       3,
		MergeConflict:     false,
		IsDraft:           false,
		HasApproval:       true,
		ChangesRequested:  false,
		AllChecksPassing:  false,
		ReadyToMerge:      false,
		Tags:              []string{"has_approval"},
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
	if decoded.Status != resp.Status {
		t.Errorf("Status = %d, want %d", decoded.Status, resp.Status)
	}
	if decoded.StatusString != resp.StatusString {
		t.Errorf("StatusString = %s, want %s", decoded.StatusString, resp.StatusString)
	}
	if len(decoded.BlockedBy) != len(resp.BlockedBy) {
		t.Errorf("BlockedBy length = %d, want %d", len(decoded.BlockedBy), len(resp.BlockedBy))
	}

	// Verify recent activity
	if decoded.RecentActivity.Type != resp.RecentActivity.Type {
		t.Errorf("RecentActivity.Type = %s, want %s", decoded.RecentActivity.Type, resp.RecentActivity.Type)
	}

	// Verify metrics
	if decoded.TestsPassing != resp.TestsPassing {
		t.Errorf("TestsPassing = %d, want %d", decoded.TestsPassing, resp.TestsPassing)
	}
	if decoded.CommentCount != resp.CommentCount {
		t.Errorf("CommentCount = %d, want %d", decoded.CommentCount, resp.CommentCount)
	}

	// Verify flags
	if decoded.HasApproval != resp.HasApproval {
		t.Errorf("HasApproval = %v, want %v", decoded.HasApproval, resp.HasApproval)
	}
	if decoded.ReadyToMerge != resp.ReadyToMerge {
		t.Errorf("ReadyToMerge = %v, want %v", decoded.ReadyToMerge, resp.ReadyToMerge)
	}
}