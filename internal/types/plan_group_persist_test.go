package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWritePlanGroupToFile_AtomicRename pins the atomic-write
// contract: WritePlanGroupToFile writes to a `.tmp` sibling
// then renames. After a successful write, the .tmp must NOT
// exist on disk (cleaned up by os.Rename), and the .json
// payload must round-trip cleanly.
func TestWritePlanGroupToFile_AtomicRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "group-atomic.json")
	now := time.Now()
	g := &PlanGroup{
		ID:        "group-atomic",
		Goal:      "atomic write test",
		CreatedAt: now,
		Status:    PlanGroupInFlight,
		Decision:  "linear",
		ActiveIdx: 0,
		Phases:    []PhaseRecord{{Index: 0, Goal: "p0", Status: PhasePending}},
	}
	if err := WritePlanGroupToFile(g, path); err != nil {
		t.Fatalf("WritePlanGroupToFile: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file should not survive a successful rename; stat err = %v", err)
	}
	loaded, err := LoadPlanGroupFromFile(path)
	if err != nil {
		t.Fatalf("LoadPlanGroupFromFile: %v", err)
	}
	if loaded == nil || loaded.ID != "group-atomic" {
		t.Errorf("round-trip drift; got %+v", loaded)
	}
}

// TestLoadPlanGroupFromFile_RecoversFromDanglingTmp pins the
// crash-recovery contract: when a previous orchestrator died
// AFTER writing the .tmp but BEFORE the rename completed,
// the .json file either doesn't exist (first write) or
// carries the previous version. The dangling .tmp must NOT
// be silently treated as the active file — Load should read
// the canonical .json (or return (nil, nil) when it's
// missing) and let the caller deal with the orphan .tmp on
// its own schedule (PlanGroupStore.List skips .tmp via the
// ".json.tmp" suffix check).
func TestLoadPlanGroupFromFile_RecoversFromDanglingTmp(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "group-crash.json")
	tmp := canonical + ".tmp"

	// Scenario A: dangling .tmp + missing canonical (first
	// write was killed mid-rename). Load should return
	// (nil, nil) because the canonical doesn't exist.
	if err := os.WriteFile(tmp, []byte(`{"id":"group-crash"}`), 0o644); err != nil {
		t.Fatalf("seed tmp: %v", err)
	}
	loaded, err := LoadPlanGroupFromFile(canonical)
	if err != nil {
		t.Errorf("missing canonical should not error; got %v", err)
	}
	if loaded != nil {
		t.Errorf("missing canonical should return nil; got %+v", loaded)
	}

	// Scenario B: dangling .tmp + canonical exists with the
	// PRIOR version. Load must read the canonical, not the
	// .tmp — operators should see the last consistent state,
	// not whatever partial bytes the killed writer flushed.
	prior := &PlanGroup{
		ID:       "group-crash",
		Decision: "linear",
		Status:   PlanGroupInFlight,
		Phases:   []PhaseRecord{{Index: 0, Goal: "old", Status: PhaseAccepted}},
	}
	if err := WritePlanGroupToFile(prior, canonical); err != nil {
		t.Fatalf("WritePlanGroupToFile prior: %v", err)
	}
	// Re-seed a fresh dangling .tmp (the prior write removed
	// the old one as part of the rename).
	if err := os.WriteFile(tmp, []byte(`{"id":"group-crash","goal":"PARTIAL"}`), 0o644); err != nil {
		t.Fatalf("re-seed tmp: %v", err)
	}
	loaded, err = LoadPlanGroupFromFile(canonical)
	if err != nil {
		t.Fatalf("LoadPlanGroupFromFile: %v", err)
	}
	if loaded == nil {
		t.Fatal("canonical should load; got nil")
	}
	if loaded.Goal == "PARTIAL" {
		t.Errorf("Load must read canonical, not the dangling .tmp; got Goal=%q", loaded.Goal)
	}
	if len(loaded.Phases) == 0 || loaded.Phases[0].Goal != "old" {
		t.Errorf("canonical phase data drift; got %+v", loaded.Phases)
	}
}

// TestLoadPlanGroupFromFile_RefusesUnknownDecision pins the
// forward-compat gate: a file whose Decision is "parallel"
// or any unknown shape must be refused rather than silently
// loaded with garbage Phases ordering.
func TestLoadPlanGroupFromFile_RefusesUnknownDecision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "group-future.json")
	payload := map[string]interface{}{
		"id":         "group-future",
		"decision":   "parallel", // unknown shape
		"status":     string(PlanGroupInFlight),
		"phases":     []interface{}{},
	}
	data, _ := json.Marshal(payload)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := LoadPlanGroupFromFile(path)
	if err == nil {
		t.Fatal("expected error on unknown decision shape")
	}
	if !strings.Contains(err.Error(), "unsupported group decision") {
		t.Errorf("expected 'unsupported group decision' in err; got %v", err)
	}
}
