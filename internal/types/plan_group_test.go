package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPlanGroup_RoundTrip pins the canonical save → load cycle
// preserves every field.
func TestPlanGroup_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	original := &PlanGroup{
		ID:        "group-test-001",
		Goal:      "schema migration then code update",
		CreatedAt: now,
		Status:    PlanGroupInFlight,
		ActiveIdx: 1,
		Decision:  "linear",
		Phases: []PhaseRecord{
			{
				Index:            0,
				Goal:             "add migration file",
				RoughTargetPaths: []string{"migrations/0001_users.sql"},
				Status:           PhaseAccepted,
				PlanID:           "plan-001",
				AppliedSHA:       "abc123",
				StartedAt:        ptrTime(now),
				FinishedAt:       ptrTime(now.Add(time.Minute)),
				AcceptanceCheck: &AcceptanceCheck{
					Passed:    true,
					Reasoning: "migration applied; schema test passes",
					NextHint:  "ORM layer needs to know about the new users.email column",
				},
			},
			{
				Index:            1,
				Goal:             "update ORM to use new column",
				RoughTargetPaths: []string{"internal/db/user.go"},
				Status:           PhaseInProgress,
				StartedAt:        ptrTime(now.Add(time.Minute)),
			},
		},
	}
	path := filepath.Join(dir, original.ID+".json")
	if err := WritePlanGroupToFile(original, path); err != nil {
		t.Fatalf("Write: %v", err)
	}
	loaded, err := LoadPlanGroupFromFile(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil")
	}
	if loaded.ID != original.ID {
		t.Errorf("ID drift: got %q want %q", loaded.ID, original.ID)
	}
	if loaded.Status != original.Status {
		t.Errorf("Status drift: got %q want %q", loaded.Status, original.Status)
	}
	if loaded.ActiveIdx != original.ActiveIdx {
		t.Errorf("ActiveIdx drift: got %d want %d", loaded.ActiveIdx, original.ActiveIdx)
	}
	if loaded.Decision != "linear" {
		t.Errorf("Decision drift: got %q", loaded.Decision)
	}
	if len(loaded.Phases) != 2 {
		t.Fatalf("Phases length drift: got %d", len(loaded.Phases))
	}
	if loaded.Phases[0].PlanID != "plan-001" {
		t.Errorf("Phase[0].PlanID drift: got %q", loaded.Phases[0].PlanID)
	}
	if loaded.Phases[0].AcceptanceCheck == nil {
		t.Fatal("Phase[0].AcceptanceCheck lost")
	}
	if !loaded.Phases[0].AcceptanceCheck.Passed {
		t.Error("AcceptanceCheck.Passed drift")
	}
	if loaded.Phases[0].AcceptanceCheck.Reasoning != "migration applied; schema test passes" {
		t.Errorf("AcceptanceCheck.Reasoning drift: got %q", loaded.Phases[0].AcceptanceCheck.Reasoning)
	}
	if loaded.Phases[1].Status != PhaseInProgress {
		t.Errorf("Phase[1].Status drift: got %q", loaded.Phases[1].Status)
	}
}

// TestLoadPlanGroup_MissingFile pins the absent-file path:
// returns (nil, nil), not an error. Lets callers do "load on
// startup; nil = no group active" without a stat dance.
func TestLoadPlanGroup_MissingFile(t *testing.T) {
	dir := t.TempDir()
	g, err := LoadPlanGroupFromFile(filepath.Join(dir, "nope.json"))
	if err != nil {
		t.Errorf("missing file should not error; got %v", err)
	}
	if g != nil {
		t.Errorf("missing file should yield nil; got %+v", g)
	}
}

// TestLoadPlanGroup_EmptyPathReturnsNil covers the defensive
// guard against a caller passing "" (e.g. when no
// runtimeAnchor is configured).
func TestLoadPlanGroup_EmptyPathReturnsNil(t *testing.T) {
	g, err := LoadPlanGroupFromFile("")
	if err != nil || g != nil {
		t.Errorf("empty path should yield (nil, nil); got %+v err=%v", g, err)
	}
}

// TestLoadPlanGroup_UnknownDecisionRefused pins the decision-
// shape gate: a future-format file (Decision="tree") fails to
// load on this build, surfacing a clear error so the operator
// upgrades codrax rather than silently getting wrong behaviour.
func TestLoadPlanGroup_UnknownDecisionRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "future.json")
	body := []byte(`{"id":"future","goal":"x","status":"planning","decision":"tree","phases":[],"created_at":"2026-01-01T00:00:00Z","active_idx":0}`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := LoadPlanGroupFromFile(path)
	if err == nil {
		t.Fatal("expected error on unknown decision shape")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected 'unsupported' in error; got %v", err)
	}
}

// TestWritePlanGroup_NilArgsReject pins defensive guards.
func TestWritePlanGroup_NilArgsReject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	if err := WritePlanGroupToFile(nil, path); err == nil {
		t.Error("nil group must reject")
	}
	if err := WritePlanGroupToFile(&PlanGroup{ID: "x"}, ""); err == nil {
		t.Error("empty path must reject")
	}
	if err := WritePlanGroupToFile(&PlanGroup{}, path); err == nil {
		t.Error("empty group.ID must reject")
	}
}

// TestWritePlanGroup_AtomicRename verifies the .tmp scratch file
// is removed after a successful rename.
func TestWritePlanGroup_AtomicRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.json")
	g := &PlanGroup{
		ID:        "atomic",
		Goal:      "x",
		Status:    PlanGroupPlanning,
		Decision:  "linear",
		CreatedAt: time.Now(),
	}
	if err := WritePlanGroupToFile(g, path); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp scratch should be removed; stat err=%v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("final file should exist; stat err=%v", err)
	}
}

// TestPhaseStatus_TerminalDetection pins the terminal-state
// classification used by the scheduler when iterating ActiveIdx
// forward (it skips terminal phases).
func TestPhaseStatus_TerminalDetection(t *testing.T) {
	cases := []struct {
		s        PhaseStatus
		terminal bool
	}{
		{PhasePending, false},
		{PhaseInProgress, false},
		{PhaseApplied, false},
		{PhaseVerified, false},
		{PhaseAccepted, true},
		{PhaseRolledBack, true},
		{PhaseSkipped, true},
		{PhaseAcceptanceUnverified, true},
	}
	for _, c := range cases {
		if got := IsTerminalPhaseStatus(c.s); got != c.terminal {
			t.Errorf("IsTerminalPhaseStatus(%q) = %v; want %v", c.s, got, c.terminal)
		}
	}
}

// TestPlanGroupStatus_TerminalDetection mirrors the phase-level
// terminal check for the group lifecycle.
func TestPlanGroupStatus_TerminalDetection(t *testing.T) {
	cases := []struct {
		s        PlanGroupStatus
		terminal bool
	}{
		{PlanGroupPlanning, false},
		{PlanGroupInFlight, false},
		{PlanGroupCompleted, true},
		{PlanGroupFailed, true},
		{PlanGroupRolledBack, true},
	}
	for _, c := range cases {
		if got := IsTerminalGroupStatus(c.s); got != c.terminal {
			t.Errorf("IsTerminalGroupStatus(%q) = %v; want %v", c.s, got, c.terminal)
		}
	}
}

// TestMaxPhasesPerGroup pins the cap as a stable contract — a
// future bump must update emit_write_analysis schema in
// lockstep, so the constant being touched is a deliberate
// signal.
func TestMaxPhasesPerGroup(t *testing.T) {
	if MaxPhasesPerGroup != 5 {
		t.Errorf("MaxPhasesPerGroup expected 5 (rationale: empirical 2-3 typical, 5 covers tail); got %d", MaxPhasesPerGroup)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
