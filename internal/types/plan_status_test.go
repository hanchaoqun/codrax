package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helper: seed a plan JSON on disk for the status tests to mutate.
func seedPlanFile(t *testing.T, dir string, plan *ChangePlan) string {
	t.Helper()
	if plan.ID == "" {
		t.Fatal("seedPlanFile: plan.ID must be non-empty")
	}
	path := filepath.Join(dir, plan.ID+".json")
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// TestUpdatePlanStatusOnDisk_HappyPath verifies the status field
// flips while every other field stays byte-preserved.
func TestUpdatePlanStatusOnDisk_HappyPath(t *testing.T) {
	dir := t.TempDir()
	plan := &ChangePlan{
		ID:            "plan-status-1",
		TriggerTurnID: "turn-abc",
		SessionID:     "sess-xyz",
		Request:       "fix the bug",
		Summary:       "fix",
		Changes:       []FileChange{{Path: "a.go", Kind: "modify", NewContent: "body"}},
		TargetPaths:   []string{"a.go"},
		Status:        PlanStatusPending,
		CreatedAt:     time.Now(),
	}
	path := seedPlanFile(t, dir, plan)

	now := time.Now()
	if err := UpdatePlanStatusOnDisk(path, PlanStatusApplied, &now); err != nil {
		t.Fatalf("UpdatePlanStatusOnDisk: %v", err)
	}

	reloaded, err := LoadChangePlanFromFile(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != PlanStatusApplied {
		t.Errorf("Status = %q, want %q", reloaded.Status, PlanStatusApplied)
	}
	if reloaded.AppliedAt == nil || reloaded.AppliedAt.IsZero() {
		t.Error("AppliedAt should be populated")
	}
	// Byte-preservation checks:
	if reloaded.TriggerTurnID != "turn-abc" {
		t.Errorf("TriggerTurnID lost; got %q", reloaded.TriggerTurnID)
	}
	if reloaded.Request != "fix the bug" {
		t.Errorf("Request lost; got %q", reloaded.Request)
	}
	if len(reloaded.Changes) != 1 || reloaded.Changes[0].Path != "a.go" {
		t.Errorf("Changes lost; got %+v", reloaded.Changes)
	}
}

// TestUpdatePlanStatusOnDisk_NilAppliedAt verifies AppliedAt is
// left untouched when the caller passes nil.
func TestUpdatePlanStatusOnDisk_NilAppliedAt(t *testing.T) {
	dir := t.TempDir()
	plan := &ChangePlan{
		ID:      "plan-nilapp",
		Request: "x",
		Changes: []FileChange{{Path: "a.go", Kind: "modify"}},
		Status:  PlanStatusPending,
	}
	path := seedPlanFile(t, dir, plan)

	if err := UpdatePlanStatusOnDisk(path, PlanStatusVerifyFailed, nil); err != nil {
		t.Fatalf("UpdatePlanStatusOnDisk: %v", err)
	}
	reloaded, err := LoadChangePlanFromFile(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != PlanStatusVerifyFailed {
		t.Errorf("Status = %q, want %q", reloaded.Status, PlanStatusVerifyFailed)
	}
	if reloaded.AppliedAt != nil {
		t.Error("AppliedAt should remain nil when caller passed nil")
	}
}

// TestUpdatePlanStatusOnDisk_EmptyPathRejected locks the defensive
// input guard so misconfigured callers don't hit a confusing
// "read /" error.
func TestUpdatePlanStatusOnDisk_EmptyPathRejected(t *testing.T) {
	if err := UpdatePlanStatusOnDisk("", PlanStatusApplied, nil); err == nil {
		t.Fatal("empty path should error")
	} else if !strings.Contains(err.Error(), "empty path") {
		t.Errorf("error should explain empty path; got %q", err.Error())
	}
}

// TestUpdatePlanStatusOnDisk_MissingFile surfaces a clean error
// (caller typically logs + proceeds).
func TestUpdatePlanStatusOnDisk_MissingFile(t *testing.T) {
	err := UpdatePlanStatusOnDisk(filepath.Join(t.TempDir(), "nonexistent.json"),
		PlanStatusApplied, nil)
	if err == nil {
		t.Fatal("missing file should error")
	}
}

// TestPlanStatusConstants_Uniqueness guards against a future
// refactor accidentally aliasing two states.
func TestPlanStatusConstants_Uniqueness(t *testing.T) {
	all := []string{
		PlanStatusPending,
		PlanStatusApplied,
		PlanStatusApplyFailed,
		PlanStatusVerifyFailed,
		PlanStatusRejected,
	}
	seen := map[string]bool{}
	for _, s := range all {
		if seen[s] {
			t.Errorf("duplicate status constant value: %q", s)
		}
		seen[s] = true
	}
	if len(seen) != len(all) {
		t.Errorf("status constant count mismatch: seen=%d all=%d", len(seen), len(all))
	}
}
