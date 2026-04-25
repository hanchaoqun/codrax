package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// synthesizeBuildFailureReport replaces the legacy "java-build" /
// "hvigor-build" / "cmake-build" / "meson-build" string convention.
// Verify the new structured shape: Kind=BuildError, BuildFailed=true,
// BuildErrors[] populated when stdout is parseable, AssertionID
// derived from first file:line, and renderBuildFailureSummary
// drives FailureSummary.

func TestSynthesizeBuildFailureReport_StructuredShape(t *testing.T) {
	stdout := `[INFO] BUILD FAILURE
[ERROR] /home/ci/src/Foo.java:[42,5] cannot find symbol
[ERROR] /home/ci/src/Foo.java:[99,1] unexpected token
`
	ctx := &types.BusContext{Mutable: types.NewMutableState("")}
	res, err := synthesizeBuildFailureReport(ctx, "run_tests", "java", "Java", stdout)
	if err != nil {
		t.Fatalf("synthesise failed: %v", err)
	}
	if res.Success {
		t.Error("ToolResult.Success should be false on build failure")
	}
	report := ctx.Mutable.ChangeReport()
	if report == nil {
		t.Fatal("ChangeReport must be installed on Mutable")
	}
	if !report.BuildFailed {
		t.Error("BuildFailed flag must be set")
	}
	if report.Passed {
		t.Error("Passed must be false")
	}
	if len(report.TestResults) != 1 {
		t.Fatalf("expected exactly one synthetic row; got %d", len(report.TestResults))
	}
	tr := report.TestResults[0]
	if tr.Kind != types.TestResultKindBuildError {
		t.Errorf("Kind = %q; want %q", tr.Kind, types.TestResultKindBuildError)
	}
	if tr.Suite != "build" {
		t.Errorf("Suite = %q; want build", tr.Suite)
	}
	if tr.AssertionID != "/home/ci/src/Foo.java:42" {
		t.Errorf("AssertionID = %q; want /home/ci/src/Foo.java:42", tr.AssertionID)
	}
	if len(tr.BuildErrors) < 2 {
		t.Errorf("expected ≥2 BuildErrors; got %d (%v)", len(tr.BuildErrors), tr.BuildErrors)
	}
	if !strings.Contains(report.FailureSummary, "compile error(s)") {
		t.Errorf("FailureSummary should mention compile error count; got %q", report.FailureSummary)
	}
	if !strings.Contains(report.FailureSummary, "/home/ci/src/Foo.java:42") {
		t.Errorf("FailureSummary should surface first error file:line; got %q", report.FailureSummary)
	}
}

func TestSynthesizeBuildFailureReport_NoStructuredErrors(t *testing.T) {
	// Build failed but no marker our regexes recognise — should still
	// produce a build-error row, just without BuildErrors[].
	stdout := "Could not resolve dependency foo:bar:1.0 from repository\nFailed to download artifact\n"
	ctx := &types.BusContext{Mutable: types.NewMutableState("")}
	res, err := synthesizeBuildFailureReport(ctx, "run_tests", "java", "Java", stdout)
	if err != nil {
		t.Fatalf("synthesise failed: %v", err)
	}
	if res.Success {
		t.Error("Success should be false")
	}
	report := ctx.Mutable.ChangeReport()
	if !report.BuildFailed {
		t.Error("BuildFailed must be set even without structured errors")
	}
	if len(report.TestResults) != 1 {
		t.Fatalf("expected one synthetic row; got %d", len(report.TestResults))
	}
	tr := report.TestResults[0]
	if tr.Kind != types.TestResultKindBuildError {
		t.Errorf("Kind = %q; want build_error", tr.Kind)
	}
	if len(tr.BuildErrors) != 0 {
		t.Errorf("BuildErrors should be empty when no markers parse; got %v", tr.BuildErrors)
	}
	if tr.AssertionID != "" {
		t.Errorf("AssertionID should be empty (no first BuildError); got %q", tr.AssertionID)
	}
	if tr.FailureDetail == "" {
		t.Errorf("FailureDetail should still carry narrative excerpt; got empty")
	}
}

func TestSynthesizeBuildFailureReport_HvigorTSError(t *testing.T) {
	// HarmonyOS hvigor TypeScript compile error — exercises the TS
	// regex shape via the end-to-end synthesise path.
	stdout := `> hvigor BUILD FAILED
ERROR: src/main/ets/MyAbility.ets(42,5): TS2304: Cannot find name 'BadSymbol'
ERROR: src/main/ets/Util.ets(7,1): TS2322: Type mismatch
`
	ctx := &types.BusContext{Mutable: types.NewMutableState("")}
	_, err := synthesizeBuildFailureReport(ctx, "run_tests", "hvigor", "hvigor", stdout)
	if err != nil {
		t.Fatalf("synthesise failed: %v", err)
	}
	report := ctx.Mutable.ChangeReport()
	tr := report.TestResults[0]
	if len(tr.BuildErrors) < 2 {
		t.Fatalf("expected 2 TS errors; got %d (%v)", len(tr.BuildErrors), tr.BuildErrors)
	}
	if tr.BuildErrors[0].Symbol != "TS2304" {
		t.Errorf("first symbol = %q; want TS2304", tr.BuildErrors[0].Symbol)
	}
	if !strings.Contains(report.FailureSummary, "hvigor build failed") {
		t.Errorf("FailureSummary should name 'hvigor'; got %q", report.FailureSummary)
	}
}

// Plan-aware path: PlanID flows through.
func TestSynthesizeBuildFailureReport_PlanIDPropagated(t *testing.T) {
	stdout := "[ERROR] /x.java:[1,1] err\n"
	ctx := &types.BusContext{Mutable: types.NewMutableState("")}
	ctx.Mutable.SetChangePlan(&types.ChangePlan{ID: "plan-build-fail-test"})
	if _, err := synthesizeBuildFailureReport(ctx, "run_tests", "java", "Java", stdout); err != nil {
		t.Fatalf("synthesise: %v", err)
	}
	if id := ctx.Mutable.ChangeReport().PlanID; id != "plan-build-fail-test" {
		t.Errorf("PlanID not propagated; got %q", id)
	}
}
