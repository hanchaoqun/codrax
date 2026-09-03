package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// explore_backtrack_exhaustion.go — §40.43 F-orch 三轮复核 finding Q.
//
// A finalize contract failure routed back_to_explore binds the retry state
// to a new explore-backtrack epoch (the accepted-closure veto, §40.14 V7-2)
// and clears Signals.HasEnoughFacts (§40.43 R2) so the requeued reconcile /
// validate node waits for the explorer's FRESH completion. The backtrack is
// consumed by one of two typed decisions:
//
//   - the explorer's next accepted completion (SetInvestigationComplete
//     advances the completion generation), or
//   - the scheduler's ExploreBacktrackExhausted decision below: the
//     re-opened explore window closed without a fresh accepted completion
//     (the explorer exited without deciding, or its fact-retry lane was
//     refused because the retry budget is spent) and the DAG can make no
//     further progress. The decision advances the same completion
//     generation, so the veto is released through the premise every
//     accepted-closure consumer already reads (accepted_closure_premise.go),
//     and the accepted-closure signal is restored from the RETAINED closure
//     (StableInvestigationCompleteReason) so reconcile proceeds from it and
//     the finalizer re-runs with the contract violations as repair context.
//     Termination then flows through the existing contract-check /
//     accept-with-caveat lanes.
//
// Before this file the exhausted window left the node blocked, the loop
// broke out through the blocked-DAG forced finalize, the forced dispatch
// was skipped because the REJECTED draft was still retained in
// lastFinalize, and the customer received that draft with no caveat.
// applyRetainedRejectedDraftCaveats is the structural backstop for every
// terminal exit that ships a retained rejected draft.

// exploreBacktrackExhaustedReason is the typed lane name recorded on the
// decision. One lane: the window closed with no ready work and blocked nodes.
const exploreBacktrackExhaustedReason = "explore window closed without a fresh accepted completion; DAG blocked"

// releaseExhaustedExploreBacktrack is called by runReadSchedulerLoop at the
// point where no explorer window and no finalize node are ready while nodes
// are blocked on entry conditions. Precise operands only:
//
//  1. a bound explore-contract backtrack is in force for the current
//     epoch / generation (acceptedClosureHasActiveExploreContractBacktrack);
//  2. a retained accepted closure exists to proceed from
//     (StableInvestigationCompleteReason != "").
//
// When both hold it records the typed decision (advancing the completion
// generation — the veto is consumed exactly once, like a fresh completion),
// restores Signals.HasEnoughFacts from the retained closure and returns
// true so the scheduler re-evaluates the DAG. Without a retained closure
// there is nothing to proceed from: it returns false and the caller keeps
// the blocked-DAG exit (the rejected-draft backstop covers the delivery).
func (o *Orchestrator) releaseExhaustedExploreBacktrack(blocked int) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return false
	}
	mut := o.busCtx.Mutable
	if !acceptedClosureHasActiveExploreContractBacktrack(mut) {
		return false
	}
	if strings.TrimSpace(mut.StableInvestigationCompleteReason()) == "" {
		logging.Warning("[orchestrator] explore backtrack exhausted with no retained accepted closure; %d node(s) stay blocked", blocked)
		return false
	}
	d := mut.RecordExploreBacktrackExhausted(exploreBacktrackExhaustedReason)
	o.busCtx.Signals.HasEnoughFacts = true
	logging.Warning("[orchestrator] explore backtrack exhausted: %d node(s) blocked; proceeding from the retained accepted closure (epoch=%d completion_generation=%d→%d)",
		blocked, d.Epoch, d.GenerationBefore, d.GenerationAfter)
	return true
}

// retainedRejectedFinalizeDraft is the typed record of the finalize output
// the contract check REJECTED and a fallback arm requeued instead of
// shipping. The scheduler keeps that output in lastFinalize while the
// retry is in flight; if the loop then terminates without a later finalize
// output (blocked DAG, scheduler stall, step drain, transient dispatch
// failure delivering the previous draft) this record is what proves the
// retained draft is contract-rejected and which violations it carries.
// Identity is the exact StageOutput pointer — a later output supersedes it.
type retainedRejectedFinalizeDraft struct {
	out *agent.StageOutput
	res contract.Result
}

// applyRetainedRejectedDraftCaveats is the structural backstop at the
// terminal exit: when the output about to ship IS the retained rejected
// draft, the contract caveats are applied exactly as the accept-with-caveat
// lanes apply them (violation digest channel decision, materialised user
// caveats, system caveats). The main-exit tail then attaches the
// first-draft reference as for every other caveated delivery. Returns
// whether the backstop fired.
func (o *Orchestrator) applyRetainedRejectedDraftCaveats(shipping *agent.StageOutput, rejected *retainedRejectedFinalizeDraft) bool {
	if o == nil || shipping == nil || rejected == nil || rejected.out == nil || rejected.out != shipping {
		return false
	}
	shipping.FinalAnswer = o.applyContractViolations(shipping.FinalAnswer, rejected.res)
	shipping.FinalAnswer = o.appendUserCaveatsTracked(shipping.FinalAnswer, rejected.res.Violations)
	shipping.FinalAnswer = o.appendSystemCaveatsToAnswer(shipping.FinalAnswer)
	logging.Warning("[orchestrator] terminal exit ships the retained contract-rejected draft; %d violation(s) materialised as user caveat",
		len(rejected.res.Violations))
	return true
}
