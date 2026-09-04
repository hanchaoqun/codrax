package orchestrator

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIRDeliveryHotFileLineRatchet(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path     string
		maxLines int
	}{
		{path: filepath.Join("..", "types", "evidence_closure.go"), maxLines: 2630},
		{path: "scheduler.go", maxLines: 743},
		// Tightened 9395→9260 after WRITEFIX-1 split the apply/verify
		// wording single-point into write_verify_render.go (which gets
		// its own budget below) — the freed budget must not silently
		// grow back into the god-file. Tightened 9260→9135 after
		// COMPLETE-2 (§29.140 GAP-4) split the accepted-closure
		// mixed-origin debt gate into accepted_closure_origin_debt.go
		// (own budget below); same rule: freed budget must not grow back.
		// Tightened 9135→9126 after §40.14 V7-2 moved the finalize
		// acceptance exit (evidence-utilization log + node bookkeeping +
		// retry-chain close) into acceptFinalizeNode in retry_state.go —
		// the four repeated triplets became one call each.
		// Tightened 9126→9018 after F14 (§40.14 V7-2 fold-in) moved the
		// reconcile-node auto-complete arm into accepted_closure_reconcile.go
		// and the shared accepted-closure premise into
		// accepted_closure_premise.go (own budgets below).
		// Tightened 9018→8993 after §40.43 R1/R2 (fold-in round three) moved
		// the P6 finalize repair hard cap trio into
		// finalize_repair_hard_cap.go (own budget below) and folded the
		// three duplicated populateRetryState prologues of the fallback
		// arms into one retryStateAttempt read.
		// Tightened 8993→8827 after the F-run-tests round-three fold-in
		// (§40.36 finding F) moved renderVerifyFailure and its helpers into
		// write_verify_failure_render.go (own budget below) so the failure
		// outcome renders the shared worktree-audit note.
		// Tightened 8827→8814 after the F-orch round-three fold-in (§40.43
		// finding Q) moved the first-draft attachment titles into
		// first_draft_reference.go and put the exhaustion release / rejected-
		// draft backstop into explore_backtrack_exhaustion.go (own budget).
		// Tightened 8814→8813 after §40.47 V4-4 (A7) moved the degraded-IR
		// required-file lanes into copyDegradedRequiredFileLanes (net -1 line;
		// the group's own 9126→9125 record was written against the pre-round
		// base and is superseded by this chain).
		// Tightened 8813→8809 after the F-orch round-four fold-in (§40.43
		// finding V) moved the first-draft capture into the typed
		// firstFinalizeDraftRecord in first_draft_reference.go (own budget).
		// Tightened 8809→8728 after §40.52 (V8-7/V11-3) moved emitCGECSummary
		// into cgec_summary_log.go (own budget below) once the CGEC summary
		// jargon rewording pushed the god-file over the ceiling.
		// Tightened 8809→8554 after §40.27 V7-5 (§40.55) moved the write
		// retry-hint concern (buildRetryHint / buildRetryHintWithBest /
		// buildPlanContentDiff) into write_retry_hint.go (own budget below)
		// and RESTORED the §29.60 pendingCompletionReset lane comments that
		// 4c7a0d0a3 had compressed to stay under the then-ceiling: comment
		// compression is not ratchet compliance (see ratchetComplianceRule
		// in ratchet_compliance_message_census_test.go).
		// Batch-six merge (§40.52 + §40.55 landed together against the same
		// 8809 base): both extractions are present, so the ceiling is the
		// merged post-move count 8473 (zero headroom, as every god-file
		// tightening in this chain), not the lower of the two per-group
		// records — the freed budget of either move must not grow back.
		// Tightened 8473→8451 after the §40.55 合流复核 fold-in (G6-ratchet
		// #0) moved buildRetryHint's 22-line godoc — left behind by the V7-5
		// cut and glued onto buildIterationRecord — to its function in
		// write_retry_hint.go (that row is corrected below in the same
		// change; the pair's combined ceiling falls 8773→8771).
		{path: "orchestrator.go", maxLines: 8451},
		// §40.52: the "[CGEC] summary" operator log (96 lines moved); small
		// round headroom like the sibling concern-file rows.
		{path: "cgec_summary_log.go", maxLines: 100},
		// §40.55 V7-5: the ChangePlan verify→plan retry-hint trio; small
		// headroom over the lines moved so a hint-wording fix does not force
		// a second extraction. CORRECTED 300→320 by the §40.55 合流复核
		// fold-in: the 300 row was minted at 289 actual with buildRetryHint's
		// godoc still sitting in orchestrator.go; the doc (and the header's
		// true-caller note) moved here in the change that tightened
		// orchestrator.go 8473→8451, so this is a transfer bounded by the
		// god-file's tightening, not new budget (318 actual, 2 headroom).
		{path: "write_retry_hint.go", maxLines: 320},
		{path: "first_draft_reference.go", maxLines: 120},
		// §40.43 R1: the P6 hard cap (tightened 110→45 after finding S moved
		// the advisory into finalize_loop_gate_advisory.go, own budget below).
		{path: "finalize_repair_hard_cap.go", maxLines: 45},
		// §40.43 F-orch 三轮复核 finding S: every gate preceding
		// AdvanceRepairExecutionPlan, stated total / conditional per arm.
		{path: "finalize_loop_gate_advisory.go", maxLines: 250},
		{path: "explore_backtrack_exhaustion.go", maxLines: 120},
		{path: "write_verify_failure_render.go", maxLines: 190},
		// F14: the reconcile auto-complete arm and the single accepted-closure
		// premise shared by both auto-complete consumers.
		{path: "accepted_closure_reconcile.go", maxLines: 130},
		{path: "accepted_closure_premise.go", maxLines: 60},
		{path: "write_verify_render.go", maxLines: 420},
		// DELIBERATE 240→280 (§29.146 UPSTREAM-3 件1): the pre-mint
		// withhold half of the current_source waiver double defense
		// (acceptedClosureRequiredOriginLanesBeforeDebtMint +
		// withholdWaivedCurrentSourceOriginLaneBeforeDebtMint) lives in
		// this concern-specific file next to the post-filter backstop it
		// mirrors.
		{path: "accepted_closure_origin_debt.go", maxLines: 280},
		// EVALFIX-2E (CLASS 5): the "[degrade] ledger:" operator
		// aggregate lives in its own concern file so the degradation-
		// ledger surface never grows the god-file.
		{path: "degradation_ledger_log.go", maxLines: 60},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read %s: %v", tc.path, err)
			}
			lines := bytes.Count(data, []byte{'\n'})
			if len(data) > 0 && data[len(data)-1] != '\n' {
				lines++
			}
			if lines > tc.maxLines {
				t.Fatalf("%s has %d lines; IR delivery ratchet allows at most %d. Split concern-specific code or update the delivery ledger before expanding this budget. Comment/blank-line compression and dead-line trimming are NOT ratchet compliance — extract a concern file and lower this ceiling in the same change.", tc.path, lines, tc.maxLines)
			}
		})
	}
}
