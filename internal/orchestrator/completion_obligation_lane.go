// completion_obligation_lane.go — the read scheduler's bounded
// completion lane (GAP-1, docs/design/correctness_promises_p1p2_20260612.md §1).
//
// Five explore exits can reach extract without an accepted
// emit_investigation_complete (success-criteria pass, stop-condition
// incl. react-iters cap, hard stall, blocked DAG, step-budget drain),
// which structurally bypasses every typed obligation enforced inside
// that tool. Mirroring the write controller's budget-completion verify
// ("completion outranks pacing"), the lane grants ONE emit-only
// explorer dispatch when the loop is about to leave exploration with
// a pending typed obligation, then proceeds regardless of outcome.
//
// All trigger inputs are typed: the analyzer-declared predicate, the
// stable aggregate-fact pool, the retained absence justification, and
// the Mutable completion flag. No prose is inspected.

package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// completionObligationLaneMaxIters bounds the lane's single dispatch:
// the surface is emit-only, so a handful of iterations covers
// "compose member_set → emit completion" plus one repair round.
const completionObligationLaneMaxIters = 4

// pendingCompletionObligations reports the typed completion
// obligations the investigation has not yet satisfied. One entry per
// obligation; extend here when a new typed completion contract lands
// so the lane covers it automatically.
func pendingCompletionObligations(busCtx *types.BusContext) []string {
	if busCtx == nil || busCtx.AnalysisIR == nil || busCtx.Mutable == nil {
		return nil
	}
	var out []string
	rm := busCtx.AnalysisIR.RequestModel
	if rm.Predicates.HasPerMemberTable &&
		!mutableHasMemberSetFact(busCtx.Mutable) &&
		strings.TrimSpace(busCtx.Mutable.StableAbsenceJustification()) == "" {
		out = append(out, "per-member table requires a member_set aggregate fact")
	}
	return out
}

// mutableHasMemberSetFact reports whether the stable investigation
// aggregate-fact pool already carries an exact member set.
func mutableHasMemberSetFact(mut *types.MutableState) bool {
	if mut == nil {
		return false
	}
	for _, f := range mut.StableInvestigationAggregateFacts() {
		if strings.EqualFold(strings.TrimSpace(string(f.Kind)), "member_set") && len(f.Members) > 0 {
			return true
		}
	}
	return false
}

// completionObligationLaneHint is the focused instruction for the
// lane's dispatch. It names the pending obligations and both typed
// exits (the member_set handoff or an absence justification) so the
// model is never cornered without an escape lane.
func completionObligationLaneHint(pending []string) string {
	return fmt.Sprintf(
		"The investigation budget is spent but a typed completion handoff is still pending: %s. "+
			"This turn allows only `emit_evidence` and `emit_investigation_complete`. "+
			"Compose the completion from evidence already gathered: hand over the complete verified member list as an `aggregate_facts` entry with `kind=\"member_set\"` (exact members; `support_refs` per the member contract), then call `emit_investigation_complete`. "+
			"If the set genuinely cannot be enumerated from the gathered evidence, complete with `absence_justification` explaining why instead.",
		strings.Join(pending, "; "))
}

// runCompletionObligationLane fires at the scheduler's convergence
// points (pre-extract floor checks and the step-drain break). It is
// single-shot per Run via the caller-owned laneFired flag; the
// dispatch is billed through the same step counter past the ceiling, exactly
// like the write controller's budget-completion verify.
func (o *Orchestrator) runCompletionObligationLane(laneFired *bool, stepsUsed *int) {
	if o == nil || laneFired == nil || *laneFired {
		return
	}
	if o.busCtx == nil || o.busCtx.Mutable == nil || o.busCtx.Mutable.IsInvestigationComplete() {
		return
	}
	pending := pendingCompletionObligations(o.busCtx)
	if len(pending) == 0 {
		return
	}
	*laneFired = true
	logging.Info("[orchestrator] completion obligation lane: one bounded emit-only dispatch past the ceiling (pending: %s)",
		strings.Join(pending, "; "))

	prevHint := o.busCtx.TaskState.RetryHint
	prevHintStage := o.busCtx.TaskState.RetryHintStage
	prevSurface := o.busCtx.CompletionOnlySurface
	o.busCtx.TaskState.RetryHint = completionObligationLaneHint(pending)
	o.busCtx.TaskState.RetryHintStage = types.StageExplore
	o.busCtx.CompletionOnlySurface = true
	out, err := o.dispatchStage(types.StageExplore)
	o.busCtx.TaskState.RetryHint = prevHint
	o.busCtx.TaskState.RetryHintStage = prevHintStage
	o.busCtx.CompletionOnlySurface = prevSurface
	if stepsUsed != nil {
		*stepsUsed++
	}

	if err != nil {
		logging.Warning("[orchestrator] completion obligation lane dispatch failed: %v", err)
		return
	}
	if out != nil && out.Error != "" {
		logging.Warning("[orchestrator] completion obligation lane stage error: %s", out.Error)
	}
	logging.Info("[orchestrator] completion obligation lane done: investigation_complete=%t",
		o.busCtx.Mutable.IsInvestigationComplete())
}
