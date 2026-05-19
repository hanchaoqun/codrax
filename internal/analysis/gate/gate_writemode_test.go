package gate

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRun_WriteModeSkipsHypothesisCoverage covers the customer
// trigger: "用 python 写一个猜数字游戏" in plan mode generated an IR
// with HypothesisSet entries that all scored below the 30-point
// floor (no codebase to investigate → no high-priority hypotheses).
// Write mode still skips read-mode-only checks entirely so the
// classifier hands off to the write pipeline; read mode now keeps the
// low-priority score as advisory telemetry rather than a hard retry.
func TestRun_WriteModeSkipsHypothesisCoverage(t *testing.T) {
	// Build an IR with the failure shape: one low-priority hypothesis,
	// HypothesisMinPrio=30 default, len > 0.
	ir := &types.AnalysisIR{
		HypothesisSet: []types.Hypothesis{
			{ID: "h1", Priority: 1}, // raw priority below the 30-point floor
		},
	}

	// Read mode: hypothesis_coverage runs.
	readReport := Run(ir, Thresholds{}, "read")
	if findCheck(readReport, "hypothesis_coverage") == nil {
		t.Error("read mode should run hypothesis_coverage check")
	}

	// Write modes: hypothesis_coverage should be SKIPPED entirely,
	// not just passing — its check name shouldn't appear in the
	// report.
	for _, mode := range []string{"plan", "apply", "verify"} {
		report := Run(ir, Thresholds{}, mode)
		if check := findCheck(report, "hypothesis_coverage"); check != nil {
			t.Errorf("mode=%s: hypothesis_coverage should be skipped; got check %+v", mode, *check)
		}
		if check := findCheck(report, "contract_complete"); check != nil {
			t.Errorf("mode=%s: contract_complete should be skipped; got check %+v", mode, *check)
		}
	}
}

// TestRun_WriteModeStillRunsStructuralChecks ensures we ONLY skip
// the read-mode-specific quality checks; the structural checks
// (criterion_resolvable, dag_closure, pending_fields_wellformed,
// coverage, budget_sanity) stay in place because they catch IR-shape
// bugs the analyzer can produce in any mode.
func TestRun_WriteModeStillRunsStructuralChecks(t *testing.T) {
	ir := &types.AnalysisIR{}
	for _, mode := range []string{"plan", "apply", "verify"} {
		report := Run(ir, Thresholds{}, mode)
		for _, name := range []string{
			"coverage", "dag_closure", "budget_sanity",
			"criterion_resolvable", "pending_fields_wellformed",
		} {
			if findCheck(report, name) == nil {
				t.Errorf("mode=%s: structural check %q should still run", mode, name)
			}
		}
	}
}

// TestRun_EmptyModeBehavesAsRead verifies the empty-mode caller path
// (which is what every gate-package unit test passes today) routes
// to read-mode semantics — all 7 checks run.
func TestRun_EmptyModeBehavesAsRead(t *testing.T) {
	ir := &types.AnalysisIR{}
	report := Run(ir, Thresholds{}, "")
	wantChecks := []string{
		"coverage", "dag_closure", "budget_sanity",
		"contract_complete", "hypothesis_coverage",
		"criterion_resolvable", "pending_fields_wellformed",
	}
	for _, name := range wantChecks {
		if findCheck(report, name) == nil {
			t.Errorf("empty mode should run check %q", name)
		}
	}
}
