package tool

// answer_document_projection_boundary_fold_test.go — ANSWERFACE-1 件2
// (§29.140 G6, 2026-07-19) display pins: the four-state partition line
// disclosures its window-boundary extrapolated component with the in-term
// 「,含未覆盖段 X.XXXms 折入」 clause (词面单点 helper), driven ONLY by the
// typed head_carry_*/tail_open_* account fields (精确信号). The value is
// already inside the term — the partition Σ, the percentages and every other
// face stay byte-identical; a fold claiming more than its own term renders
// nothing (拒标不造数). The legend's 全窗四态 entry explains the clause.
//
// Witness shape (a3/c2 形): donghu_tieba_frame whole-window account publishes
// tail_open running 1.016ms (switch-in with no closing event); the engine-side
// values are pinned in internal/tracequery/target_window_state_boundary_fold_test.go.

import (
	"strings"
	"testing"
)

// boundaryFoldPartitionLine extracts the 关注线程全窗四态 partition row (the
// legend's 全窗四态 entry legitimately mentions the clause vocabulary whenever
// the account renders — entry-level like every other caliber legend — so the
// no-clause assertions scope to the row itself).
func boundaryFoldPartitionLine(t *testing.T, lead string) string {
	t.Helper()
	for _, line := range strings.Split(lead, "\n") {
		if strings.Contains(line, "关注线程全窗四态") || strings.Contains(line, "Focused-thread full-window states") {
			return line
		}
	}
	t.Fatalf("partition line missing:\n%s", lead)
	return ""
}

func TestBoundaryFoldClauseRendersOnRunningTerm(t *testing.T) {
	projection := cov4RunningDominantProjection()
	projection.TargetStateAccount.TailOpenMS = 1.016
	projection.TargetStateAccount.TailOpenState = "running"
	_, lead := cov4LeadText(t, projection, true)
	if !strings.Contains(lead, "- 关注线程全窗四态: running 109.270ms(72%,含未覆盖段 1.016ms 折入) + runnable 8.226ms(5%) + sleep 28.507ms(19%,其中 IO等待 20.000ms) + D-state 5.379ms(4%,其中 IO等待 1.340ms) = 151.382ms(四态合计=分析窗)。") {
		t.Fatalf("the running term must carry the boundary-fold disclosure clause:\n%s", lead)
	}
	if !strings.Contains(lead, "「含未覆盖段 X 折入」 = 该态含窗界外推段") {
		t.Fatalf("the 全窗四态 legend must explain the fold clause:\n%s", lead)
	}
}

func TestBoundaryFoldClauseRendersOnRunningTermEN(t *testing.T) {
	projection := cov4RunningDominantProjection()
	projection.TargetStateAccount.TailOpenMS = 1.016
	projection.TargetStateAccount.TailOpenState = "running"
	_, lead := cov4LeadText(t, projection, false)
	if !strings.Contains(lead, "running 109.270ms (72%, incl. uncovered segment 1.016ms folded in)") {
		t.Fatalf("the EN running term must carry the boundary-fold clause:\n%s", lead)
	}
	if !strings.Contains(lead, "incl. uncovered segment X folded in = the state contains a window-boundary extrapolated segment") {
		t.Fatalf("the EN legend must explain the fold clause:\n%s", lead)
	}
}

func TestBoundaryFoldHeadAndTailSameLaneSum(t *testing.T) {
	projection := cov4RunningDominantProjection()
	projection.TargetStateAccount.HeadCarryMS = 2.000
	projection.TargetStateAccount.HeadCarryState = "sleep"
	projection.TargetStateAccount.TailOpenMS = 3.000
	projection.TargetStateAccount.TailOpenState = "sleep"
	_, lead := cov4LeadText(t, projection, true)
	if !strings.Contains(lead, "sleep 28.507ms(19%,其中 IO等待 20.000ms,含未覆盖段 5.000ms 折入)") {
		t.Fatalf("same-lane head+tail folds must sum inside the one term clause:\n%s", lead)
	}
}

func TestBoundaryFoldIOWaitLaneBelongsToDStateTerm(t *testing.T) {
	projection := cov4RunningDominantProjection()
	projection.TargetStateAccount.HeadCarryMS = 0.500
	projection.TargetStateAccount.HeadCarryState = "io_wait"
	_, lead := cov4LeadText(t, projection, true)
	if !strings.Contains(lead, "D-state 5.379ms(4%,其中 IO等待 1.340ms,含未覆盖段 0.500ms 折入)") {
		t.Fatalf("an io_wait-lane fold belongs to the D-state display term:\n%s", lead)
	}
}

func TestBoundaryFoldOverclaimRendersNothing(t *testing.T) {
	projection := cov4RunningDominantProjection()
	// A fold larger than its own term is unprovable — refuse the clause,
	// keep the partition line (拒标不造数).
	projection.TargetStateAccount.TailOpenMS = 9.000
	projection.TargetStateAccount.TailOpenState = "runnable" // term is 8.226
	_, lead := cov4LeadText(t, projection, true)
	line := boundaryFoldPartitionLine(t, lead)
	if strings.Contains(line, "含未覆盖段") {
		t.Fatalf("an over-claiming fold must render no clause:\n%s", line)
	}
	if !strings.Contains(line, "- 关注线程全窗四态: running 109.270ms(72%) + runnable 8.226ms(5%)") {
		t.Fatalf("the partition line itself must survive the refusal:\n%s", line)
	}
}

func TestBoundaryFoldAbsentAccountUnchanged(t *testing.T) {
	projection := cov4RunningDominantProjection()
	_, lead := cov4LeadText(t, projection, true)
	if line := boundaryFoldPartitionLine(t, lead); strings.Contains(line, "含未覆盖段") {
		t.Fatalf("a fold-free account must render no clause:\n%s", line)
	}
}
