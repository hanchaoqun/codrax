package types

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteBestPlanReportPair_RoundTrip writes a (plan, report) pair
// next to a synthetic plan file, then reads it back via Load and
// asserts no field drift.
func TestWriteBestPlanReportPair_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	planFile := filepath.Join(dir, "plan-abc.json")
	if err := os.WriteFile(planFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed plan file: %v", err)
	}
	plan := &ChangePlan{
		ID:      "plan-abc",
		Summary: "synthetic plan",
		TargetPaths: []string{"foo.go", "bar.go"},
	}
	report := &ChangeReport{
		PlanID: "plan-abc",
		Passed: true,
		TestResults: []TestResult{{AssertionID: "TestFoo", Passed: true}},
	}
	if err := WriteBestPlanReportPair(plan, report, planFile); err != nil {
		t.Fatalf("WriteBestPlanReportPair: %v", err)
	}
	bestPath := filepath.Join(dir, "plan-abc.best.json")
	if _, err := os.Stat(bestPath); err != nil {
		t.Errorf("best file not created at expected path %q: %v", bestPath, err)
	}
	gotPlan, gotReport, err := LoadBestPlanReportPair(planFile)
	if err != nil {
		t.Fatalf("LoadBestPlanReportPair: %v", err)
	}
	if gotPlan == nil || gotReport == nil {
		t.Fatalf("expected non-nil pair; got plan=%v report=%v", gotPlan, gotReport)
	}
	if gotPlan.ID != plan.ID || gotPlan.Summary != plan.Summary {
		t.Errorf("plan field drift: got %+v want %+v", gotPlan, plan)
	}
	if !gotReport.Passed || gotReport.PlanID != "plan-abc" {
		t.Errorf("report field drift: got %+v", gotReport)
	}
	if len(gotReport.TestResults) != 1 || gotReport.TestResults[0].AssertionID != "TestFoo" {
		t.Errorf("TestResults drift: got %+v", gotReport.TestResults)
	}
}

// TestLoadBestPlanReportPair_MissingFileReturnsNil pins the recovery
// path: a fresh plan with no .best.json sibling returns (nil, nil, nil)
// — not an error. The caller treats this as "no prior latch", same as
// a brand-new Run.
func TestLoadBestPlanReportPair_MissingFileReturnsNil(t *testing.T) {
	dir := t.TempDir()
	planFile := filepath.Join(dir, "fresh.json")
	if err := os.WriteFile(planFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	plan, report, err := LoadBestPlanReportPair(planFile)
	if err != nil {
		t.Errorf("missing best file should not error; got %v", err)
	}
	if plan != nil || report != nil {
		t.Errorf("expected nil pair on missing file; got plan=%v report=%v", plan, report)
	}
}

// TestLoadBestPlanReportPair_EmptyPathReturnsNil pins the defensive
// path: an empty planFilePath is a no-op (caller didn't supply
// --plan-file).
func TestLoadBestPlanReportPair_EmptyPathReturnsNil(t *testing.T) {
	plan, report, err := LoadBestPlanReportPair("")
	if err != nil || plan != nil || report != nil {
		t.Errorf("empty path should yield (nil, nil, nil); got plan=%v report=%v err=%v", plan, report, err)
	}
}

// TestLoadBestPlanReportPair_CorruptFileErrors pins the typed-error
// path: a corrupt .best.json surfaces an error so the orchestrator
// can log + continue without latch (rather than silently using
// garbage).
func TestLoadBestPlanReportPair_CorruptFileErrors(t *testing.T) {
	dir := t.TempDir()
	planFile := filepath.Join(dir, "plan-x.json")
	if err := os.WriteFile(planFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plan-x.best.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("seed corrupt: %v", err)
	}
	_, _, err := LoadBestPlanReportPair(planFile)
	if err == nil {
		t.Error("expected error on corrupt best file; got nil")
	}
}

// TestWriteBestPlanReportPair_NilArgsReject covers defensive guards.
func TestWriteBestPlanReportPair_NilArgsReject(t *testing.T) {
	dir := t.TempDir()
	planFile := filepath.Join(dir, "x.json")
	if err := WriteBestPlanReportPair(nil, &ChangeReport{}, planFile); err == nil {
		t.Error("nil plan should reject")
	}
	if err := WriteBestPlanReportPair(&ChangePlan{ID: "x"}, nil, planFile); err == nil {
		t.Error("nil report should reject")
	}
	if err := WriteBestPlanReportPair(&ChangePlan{}, &ChangeReport{}, planFile); err == nil {
		t.Error("plan with empty ID should reject")
	}
	if err := WriteBestPlanReportPair(&ChangePlan{ID: "x"}, &ChangeReport{}, ""); err == nil {
		t.Error("empty planFilePath should reject")
	}
}

// TestWriteBestPlanReportPair_AtomicRename verifies the .tmp file is
// removed after a successful write — the rename should leave only the
// final .best.json behind.
func TestWriteBestPlanReportPair_AtomicRename(t *testing.T) {
	dir := t.TempDir()
	planFile := filepath.Join(dir, "atomic.json")
	if err := os.WriteFile(planFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	plan := &ChangePlan{ID: "atomic"}
	report := &ChangeReport{PlanID: "atomic"}
	if err := WriteBestPlanReportPair(plan, report, planFile); err != nil {
		t.Fatalf("WriteBestPlanReportPair: %v", err)
	}
	// .tmp must be gone after a successful rename.
	if _, err := os.Stat(filepath.Join(dir, "atomic.best.json.tmp")); !os.IsNotExist(err) {
		t.Errorf(".tmp scratch file should be removed after atomic rename; stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "atomic.best.json")); err != nil {
		t.Errorf("final .best.json should exist; stat err=%v", err)
	}
}
