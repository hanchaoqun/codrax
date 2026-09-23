package orchestrator

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// drainHypothesisVerdicts is the P2.1 Phase 10 hook invoked after a
// successful StageExtract dispatch. It reads the Turn B verdict
// buffer, applies MarkHypothesis for each entry, and LEAVES the
// buffer populated so the finalizer's prompt builder can render the
// rationale / citation text back to the user.
//
// Error handling policy:
//
//   - Unknown hypothesis id: log a warning and skip. The v3
//     schema-level emit_hypothesis_verdict tool already rejects
//     malformed calls at decode time, so reaching this path means
//     the LLM emitted a verdict for an id not in the hypothesis set
//     (hallucinated id or a typo). We never let a hallucinated id
//     corrupt the IR.
//
//   - Unknown status: same as above. MarkHypothesis validates the
//     enum and returns an error. Skip + warn.
//
//   - Nil AnalysisIR: the extractor dispatched without an analyzer
//     run (REPL bootstrap, unit tests). Skip the drain entirely;
//     the verdicts stay in the buffer but have no IR to write
//     through. This is the same fail-closed policy as Phase 11's
//     nil-Mutable check in the explorer.
//
// The function is a no-op when the buffer is empty, so it is always
// safe to call after any extract dispatch regardless of whether the
// LLM actually used emit_hypothesis_verdict.
func (o *Orchestrator) drainHypothesisVerdicts() {
	if o.busCtx == nil || o.busCtx.Mutable == nil || o.busCtx.AnalysisIR == nil {
		return
	}
	verdicts := o.busCtx.Mutable.EmittedHypothesisVerdicts()
	if len(verdicts) == 0 {
		return
	}
	applied := 0
	for _, v := range verdicts {
		if err := o.busCtx.AnalysisIR.MarkHypothesis(v.HypothesisID, v.Status); err != nil {
			logging.Warning("[orchestrator] hypothesis verdict drain: %v (rationale=%q citation=%q)",
				err, v.Rationale, v.Citation)
			continue
		}
		applied++
	}
	logging.Debug("[orchestrator] applied %d/%d hypothesis verdicts to IR; buffer retained for finalizer rendering",
		applied, len(verdicts))
}

// runAutoVerdicts evaluates criterion-based hypothesis auto-verdicts
// without dispatching the extractor LLM. Falsification conditions
// that are satisfied inject a "rejected" verdict; hypotheses whose
// RequiredEvidence is fully satisfied (but no LLM verdict exists)
// get an "inconclusive" verdict. This is the lightweight post-
// explore-window hook that replaced the per-window extract dispatch
// — the full LLM-backed extract runs once just before finalize.
func (o *Orchestrator) runAutoVerdicts() {
	if o.busCtx == nil || o.busCtx.AnalysisIR == nil || len(o.busCtx.AnalysisIR.HypothesisSet) == 0 {
		return
	}
	mu := o.busCtx.Mutable
	if mu == nil {
		return
	}
	if skipAutoVerdictsForRuntimeSourceAuthority(o.busCtx) {
		logging.Debug("[orchestrator] auto-verdict skipped for runtime artifact without required current-source evidence")
		return
	}
	var taToolResults []types.ToolResult
	if ta := mu.TurnAArtifacts(); ta != nil {
		taToolResults = ta.ToolResults
	}
	env := criterion.Env{
		IR:                     o.busCtx.AnalysisIR,
		ToolDocumentationReady: mu.HasAcceptedToolDocumentationCompletion(&o.busCtx.AnalysisIR.RequestModel),
		Evidence:               o.busCtx.EvidenceItems,
		ToolResults:            taToolResults,
		AnswerSymbols:          o.busCtx.AnswerSymbols,
		AggregateFacts:         mu.StableInvestigationAggregateFacts(),
		PrescanBlob:            mu.PrescanSummaryBlob(),
	}
	existing := mu.EmittedHypothesisVerdicts()
	byID := make(map[string]bool, len(existing))
	for _, v := range existing {
		byID[v.HypothesisID] = true
	}
	var injected []types.HypothesisVerdict
	for _, h := range o.busCtx.AnalysisIR.HypothesisSet {
		fals := criterion.Eval(h.FalsificationCondition, env)
		if fals.Satisfied {
			if byID[h.ID] {
				logging.Warning("[orchestrator] auto-verdict: falsification satisfied for %s: forcing rejected", h.ID)
			}
			injected = append(injected, types.HypothesisVerdict{
				HypothesisID: h.ID,
				Status:       types.HypRejected,
				Rationale:    "falsification condition satisfied: " + fals.Detail,
			})
			continue
		}
		if byID[h.ID] {
			continue
		}
		okReq, _ := criterion.EvalAll(h.RequiredEvidence, env)
		if okReq && len(h.RequiredEvidence) > 0 {
			injected = append(injected, types.HypothesisVerdict{
				HypothesisID: h.ID,
				Status:       types.HypInconclusive,
				Rationale:    "required evidence satisfied but no LLM verdict emitted",
			})
		}
	}
	if len(injected) > 0 {
		mu.AppendEmittedHypothesisVerdicts(injected)
		logging.Info("[orchestrator] injected %d auto-verdict(s) from criterion evaluation", len(injected))
	}
}

// injectInconclusiveForStuckHypotheses is the scheduler's escape hatch
// for hypothesis-validate loops that cannot make progress. It mirrors
// runAutoVerdicts' "inject inconclusive" path but IGNORES
// RequiredEvidence — it applies when the scheduler has detected that
// re-investigation would not advance the env (identical envShape across
// two consecutive SuccessCriteria evaluations) and therefore no amount
// of explore retries will resolve HypUnknown. Applied per validate
// node, so only hypotheses the stuck validate gate's SC references
// are affected; unrelated hypotheses stay untouched.
//
// The rationale is explicit about the give-up — downstream renderers
// surface it as a caveat so the user sees that the answer shipped
// with unresolved hypotheses. The scheduler logs a single
// "[scheduler/stuck]" line at INFO level so operators can grep a
// trace for the exact escape event.
//
// Returns the number of hypotheses newly marked inconclusive.
func (o *Orchestrator) injectInconclusiveForStuckHypotheses(stuckNodeID string) int {
	if o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return 0
	}
	mu := o.busCtx.Mutable
	if mu == nil {
		return 0
	}
	existing := mu.EmittedHypothesisVerdicts()
	byID := make(map[string]bool, len(existing))
	for _, v := range existing {
		byID[v.HypothesisID] = true
	}
	var injected []types.HypothesisVerdict
	for _, h := range o.busCtx.AnalysisIR.HypothesisSet {
		if h.Status != types.HypUnknown && h.Status != "" {
			continue
		}
		if byID[h.ID] {
			continue
		}
		injected = append(injected, types.HypothesisVerdict{
			HypothesisID: h.ID,
			Status:       types.HypInconclusive,
			Rationale: fmt.Sprintf(
				"re-investigation did not advance evidence "+
					"(stuck at stable env shape on validate node %s); "+
					"marking inconclusive to unblock finalize",
				stuckNodeID),
		})
	}
	if len(injected) == 0 {
		return 0
	}
	mu.AppendEmittedHypothesisVerdicts(injected)
	logging.Info("[scheduler/stuck] validate %s: injected %d inconclusive verdict(s) (evidence stable across retries)",
		stuckNodeID, len(injected))
	return len(injected)
}
