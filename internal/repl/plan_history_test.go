package repl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// writeReportSibling drops a .report.json next to the plan JSON so
// collectPlanHistory can pair them in its scan.
func writeReportSibling(t *testing.T, planPath string, report *types.ChangeReport) {
	t.Helper()
	reportPath := strings.TrimSuffix(planPath, ".json") + ".report.json"
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if err := os.WriteFile(reportPath, data, 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
}

// TestCollectPlanHistory_NoStore verifies the zero-store path short-
// circuits to nil (safe for tests that bypass cmd/root.go wiring).
func TestCollectPlanHistory_NoStore(t *testing.T) {
	r, _ := newScriptedREPL(t, nil)
	if rows := r.collectPlanHistory(); rows != nil {
		t.Errorf("nil store should yield nil rows; got %v", rows)
	}
}

// TestCollectPlanHistory_EmptyDir verifies a store with no plans
// yields nil (caller suppresses the "plans applied" header).
func TestCollectPlanHistory_EmptyDir(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, _ := newScriptedREPL(t, store)
	if rows := r.collectPlanHistory(); rows != nil {
		t.Errorf("empty plan dir should yield nil; got %v", rows)
	}
}

// TestCollectPlanHistory_PlanWithoutReport renders the "no report"
// tail when a plan was saved but never applied / verify never ran.
func TestCollectPlanHistory_PlanWithoutReport(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID: "plan-pending-1", Summary: "x", Status: types.PlanStatusPending,
	}
	if _, err := store.SaveForTest(plan); err != nil {
		t.Fatalf("save: %v", err)
	}

	r, _ := newScriptedREPL(t, store)
	rows := r.collectPlanHistory()
	if len(rows) != 1 {
		t.Fatalf("want 1 row; got %d (%v)", len(rows), rows)
	}
	row := rows[0]
	if !strings.Contains(row, "plan-pending-1") {
		t.Errorf("row should include plan ID; got %q", row)
	}
	if !strings.Contains(row, "status=pending_approval") {
		t.Errorf("row should surface status; got %q", row)
	}
	if !strings.Contains(row, "no report") {
		t.Errorf("row should indicate missing report; got %q", row)
	}
}

// TestCollectPlanHistory_PlanWithPassingReport renders a plan that
// applied + passed verify.
func TestCollectPlanHistory_PlanWithPassingReport(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID: "plan-ok-1", Summary: "x", Status: types.PlanStatusApplied,
	}
	path, err := store.SaveForTest(plan)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	writeReportSibling(t, path, &types.ChangeReport{
		PlanID: "plan-ok-1",
		Passed: true,
		TestResults: []types.TestResult{
			{AssertionID: "TestA", Passed: true},
			{AssertionID: "TestB", Passed: true},
		},
	})

	r, _ := newScriptedREPL(t, store)
	rows := r.collectPlanHistory()
	if len(rows) != 1 {
		t.Fatalf("want 1 row; got %d", len(rows))
	}
	row := rows[0]
	if !strings.Contains(row, "tests=2/2 passed") {
		t.Errorf("row should show 2/2 passing; got %q", row)
	}
	// Passing plan shouldn't list any failing-test names.
	if strings.Contains(row, "TestA") || strings.Contains(row, "TestB") {
		t.Errorf("passing plan should not enumerate test names; got %q", row)
	}
}

// TestCollectPlanHistory_FailingReport renders failing test names
// with the +N more truncation suffix.
func TestCollectPlanHistory_FailingReport(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID: "plan-fail-1", Summary: "x", Status: types.PlanStatusVerifyFailed,
	}
	path, err := store.SaveForTest(plan)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	writeReportSibling(t, path, &types.ChangeReport{
		PlanID:         "plan-fail-1",
		Passed:         false,
		FailureSummary: "lots of failures",
		TestResults: []types.TestResult{
			{AssertionID: "TestA", Passed: false},
			{AssertionID: "TestB", Passed: false},
			{AssertionID: "TestC", Passed: false},
			{AssertionID: "TestD", Passed: false},
			{AssertionID: "TestE", Passed: false},
		},
	})

	r, _ := newScriptedREPL(t, store)
	rows := r.collectPlanHistory()
	if len(rows) != 1 {
		t.Fatalf("want 1 row; got %d", len(rows))
	}
	row := rows[0]
	if !strings.Contains(row, "tests=0/5 passed") {
		t.Errorf("row should show 0/5 passing; got %q", row)
	}
	// First 3 failing names shown; TestE elided.
	for _, name := range []string{"TestA", "TestB", "TestC"} {
		if !strings.Contains(row, name) {
			t.Errorf("row should list failing test %s; got %q", name, row)
		}
	}
	if !strings.Contains(row, "+2 more") {
		t.Errorf("row should show truncation suffix; got %q", row)
	}
}

// TestCollectPlanHistory_UnreadableReport falls back to "(report
// unreadable)" when the .report.json is corrupt.
func TestCollectPlanHistory_UnreadableReport(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID: "plan-corrupt-1", Summary: "x", Status: types.PlanStatusApplyFailed,
	}
	path, err := store.SaveForTest(plan)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	// Write garbage as the report.
	reportPath := strings.TrimSuffix(path, ".json") + ".report.json"
	if err := os.WriteFile(reportPath, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write garbage report: %v", err)
	}

	r, _ := newScriptedREPL(t, store)
	rows := r.collectPlanHistory()
	if len(rows) != 1 {
		t.Fatalf("want 1 row; got %d", len(rows))
	}
	if !strings.Contains(rows[0], "report unreadable") {
		t.Errorf("row should flag unreadable report; got %q", rows[0])
	}
}

// TestHistoryCommand_IncludesPlanSection exercises the /history
// slash-command wiring end-to-end: a scripted REPL with one saved
// plan should render a "plans applied/attempted" section.
func TestHistoryCommand_IncludesPlanSection(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{ID: "plan-hist-1", Summary: "x", Status: types.PlanStatusApplied}
	if _, err := store.SaveForTest(plan); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Need a real memory store because /history reads it, and it's
	// threaded through r.store.Recent / Index.
	r, out := newMemREPL(t, store)
	r.handleSlash("/history")
	rendered := out.String()
	if !strings.Contains(rendered, "plans applied/attempted") {
		t.Errorf("/history should include plan section header; got %q", rendered)
	}
	if !strings.Contains(rendered, "plan-hist-1") {
		t.Errorf("/history should name the plan; got %q", rendered)
	}
	_ = filepath.Base // guard against dead-import deletion if we ever trim
}
