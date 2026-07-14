package orchestrator

// trace_supplement_hook.go — SUPP-CORE (DISPATCH-IND 批1, 2026-07-14): the
// orchestrator-side hook for the post-explore deterministic trace_query
// supplement (internal/tool/trace_query_supplement.go).
//
// Placement contract (§29.60 completion-gate ownership + explore
// zero-displacement): the hook runs at the explore→extract boundary of the
// read scheduler loop — AFTER the model's investigation completion has been
// accepted and every completion gate (Tier-1 floor, structural-empty check,
// completion-obligation lane) has settled, and BEFORE the pre-finalize
// extract dispatch. It never re-opens collection, never requeues, never
// resets the accepted completion, and burns zero model rounds. Missing core
// trace record families are re-collected with typed parameters (fail-open on
// ANY missing typed signal) while the explore dispatch surface stays
// displacement-free and extract/finalize/render all consume the enriched
// ledger (R1 ruling a).
//
// Latch semantics (修复轮 件1 勘正, 2026-07-14): the tool-side latch makes
// revisits of THIS branch within one accepted completion single-shot; it
// does NOT survive an explore REOPEN — when a fallback/violation lane
// re-dispatches explore, the scheduler's explore-entry gate
// (ResetSystemTraceSupplementForExploreReopen) clears the lane AND the
// latch, so the reopened investigation explores displacement-free and the
// supplement re-mints at the NEXT boundary against the updated ledger.

import (
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool"
)

// runTraceQuerySupplement invokes the SUPP-CORE supplement and logs the
// outcome (performance disclosure lives on the tool-side log lines; this is
// the orchestrator breadcrumb).
func (o *Orchestrator) runTraceQuerySupplement() {
	if o == nil || o.busCtx == nil {
		return
	}
	outcome := tool.RunTraceQuerySystemSupplement(o.busCtx)
	if !outcome.Attempted {
		return
	}
	if len(outcome.Executed) == 0 {
		logging.Debug("[orchestrator] trace supplement made no engine call (reason=%s)", outcome.SkipReason)
		return
	}
	logging.Info("[orchestrator] trace supplement executed %d deterministic engine view(s) in %s at the explore→extract boundary",
		len(outcome.Executed), outcome.Elapsed.Round(1000000))
}

// resetTraceSupplementOnExploreReopen is the 件1 (修复轮 2026-07-14)
// explore-entry gate: the read scheduler's ready-window branch is the single
// entry every explore (re)dispatch passes through — sequential AND parallel
// (the parallel dispatcher is invoked inside that same branch). When a
// post-supplement lane reopened explore (FallbackBackToExplore /
// contract-violation requeue), the supplement lane + latch clear HERE so no
// explore-visible consumer face (transcript observation checkpoints, retry
// summaries, the completion authority snapshot, parallel early-convergence,
// the tier1 drill arm) can see supplement records during the reopened
// exploration; the supplement re-mints at the next explore→extract boundary.
func (o *Orchestrator) resetTraceSupplementOnExploreReopen() {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	if o.busCtx.Mutable.ResetSystemTraceSupplementForExploreReopen() {
		logging.Info("[orchestrator] explore reopened after a trace supplement: supplement lane + latch cleared (re-mints at the next explore→extract boundary)")
	}
}
