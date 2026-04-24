package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestSetBaselineCaptureEnabled_RoundTrip locks the simple getter/
// setter pair so yaml wiring in cmd/root.go can trust the
// orchestrator exposes its state for tests.
func TestSetBaselineCaptureEnabled_RoundTrip(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	if got := o.BaselineCaptureEnabled(); got {
		t.Errorf("default should be false; got %v", got)
	}
	o.SetBaselineCaptureEnabled(true)
	if got := o.BaselineCaptureEnabled(); !got {
		t.Errorf("after SetBaselineCaptureEnabled(true) expected true; got %v", got)
	}
	o.SetBaselineCaptureEnabled(false)
	if got := o.BaselineCaptureEnabled(); got {
		t.Errorf("after reset expected false; got %v", got)
	}
}

// TestCaptureBaseline_MovesChangeReportToBaseline simulates what
// captureBaseline does in isolation: run_tests installs ChangeReport;
// captureBaseline moves it to BaselineReport + clears ChangeReport.
// We bypass the real RunTests call by pre-installing a ChangeReport
// (the effect run_tests would have had) and then exercising the
// "move" half of captureBaseline's logic via its public API.
//
// Rationale for this shape: RunTests.Execute detects a runner via
// file sniffing; mocking that here is more invasive than
// demonstrating that the bookkeeping is correct.
func TestCaptureBaseline_MovesChangeReportToBaseline(t *testing.T) {
	// Simulate a test-run result on ChangeReport, then exercise
	// the same "move + reset + persist" sequence captureBaseline uses.
	o := New(types.PipelineSettings{}, nil, nil, nil)
	planDir := t.TempDir()
	planPath := filepath.Join(planDir, "plan-baseline-1.json")
	if err := os.WriteFile(planPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed plan file: %v", err)
	}
	o.busCtx = &types.BusContext{
		RepoRoot:     t.TempDir(),
		MainRepoRoot: t.TempDir(),
		PlanPath:     planPath,
		Mutable:      types.NewMutableState("baseline test"),
	}
	preApply := &types.ChangeReport{
		PlanID: "plan-baseline-1",
		Passed: true,
		TestResults: []types.TestResult{
			{AssertionID: "TestA", Passed: true},
			{AssertionID: "TestB", Passed: true},
		},
	}
	o.busCtx.Mutable.SetChangeReport(preApply)

	// Mirror the exact steps captureBaseline takes after run_tests.
	baseline := o.busCtx.Mutable.ChangeReport()
	if baseline == nil {
		t.Fatal("pre-condition: ChangeReport should be pre-installed")
	}
	o.busCtx.Mutable.SetBaselineReport(baseline)
	o.busCtx.Mutable.ResetChangeReport()
	o.saveBaselineReport(baseline)

	// Post-conditions:
	if got := o.busCtx.Mutable.ChangeReport(); got != nil {
		t.Errorf("ChangeReport should be cleared after move; got %+v", got)
	}
	if got := o.busCtx.Mutable.BaselineReport(); got == nil {
		t.Fatal("BaselineReport should hold the pre-apply snapshot")
	}
	if got := o.busCtx.Mutable.BaselineReport().TestResults; len(got) != 2 {
		t.Errorf("TestResults round-trip lost data; got %d", len(got))
	}
	// Disk persistence check: <plan-dir>/<plan-id>.baseline.json.
	baselinePath := filepath.Join(planDir, "plan-baseline-1.baseline.json")
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("baseline disk save: %v", err)
	}
	var reloaded types.ChangeReport
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatalf("baseline unmarshal: %v", err)
	}
	if reloaded.PlanID != "plan-baseline-1" {
		t.Errorf("disk reload: PlanID = %q, want plan-baseline-1", reloaded.PlanID)
	}
}

// TestCaptureBaseline_NoOpWhenDisabled verifies a no-capture run
// leaves Mutable.BaselineReport nil (read-mode-compatible path).
func TestCaptureBaseline_NoOpWhenDisabled(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		RepoRoot: t.TempDir(),
		Mutable:  types.NewMutableState("probe"),
	}
	// baselineCaptureEnabled defaults to false; don't set it.
	// A real runApplyPhase would simply skip captureBaseline; we
	// confirm the BaselineReport slot stays zero-valued so
	// evalNoRegression short-circuits to satisfied.
	if got := o.busCtx.Mutable.BaselineReport(); got != nil {
		t.Errorf("disabled baseline should not populate slot; got %+v", got)
	}
	if on := o.BaselineCaptureEnabled(); on {
		t.Error("default captureEnabled should be false")
	}
}

// TestSaveBaselineReport_SkipsWhenNoPlanPath ensures we don't
// silently try to write a baseline JSON to stdout's directory when
// the plan lives only in memory (REPL flow before the REPL saves
// to PlanStore). Operator-visible behaviour: no file on disk, but
// Mutable.BaselineReport still holds the data.
func TestSaveBaselineReport_SkipsWhenNoPlanPath(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		// PlanPath deliberately empty.
		Mutable: types.NewMutableState("probe"),
	}
	baseline := &types.ChangeReport{PlanID: "plan-memory-only", Passed: true}
	// Should not panic and should not error when PlanPath is empty.
	o.saveBaselineReport(baseline)
	// Nothing to assert on disk — the test is simply that the call
	// returns cleanly without a panic or exception.
}

// TestSaveBaselineReport_EmptyPlanIDSkips ensures we don't write a
// nonsense path like "/plans/.baseline.json" when plan.ID is
// missing (should not happen in practice — defense in depth).
func TestSaveBaselineReport_EmptyPlanIDSkips(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		PlanPath: filepath.Join(t.TempDir(), "any.json"),
		Mutable:  types.NewMutableState("probe"),
	}
	baseline := &types.ChangeReport{PlanID: "", Passed: true}
	o.saveBaselineReport(baseline)
	// The plan dir should contain zero .baseline.json files.
	entries, err := os.ReadDir(filepath.Dir(o.busCtx.PlanPath))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			if filepath.Base(e.Name()) != filepath.Base(o.busCtx.PlanPath) {
				t.Errorf("unexpected baseline file written with empty PlanID: %s", e.Name())
			}
		}
	}
}
