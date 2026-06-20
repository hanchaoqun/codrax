package orchestrator

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

const transientRetryObservationLedgerInputLimit = 24

const exploreFactRetryCheckpointPrefix = "Explorer continuation checkpoint:"

// retryReadStageDispatchError converts transient read-mode stage
// dispatch errors into a normal scheduler retry instead of forcing the
// partially-completed pipeline straight into extract/finalize.
//
// Two decoupled budgets:
//   - transientRetryBudget (orchestrator-owned) caps THIS path so a
//     network blip during explore / finalize does not drain the
//     content-retry budget that contract-violation retries depend on
//     OR the pipeline step budget that should count productive stage
//     dispatches.
//   - retryBudget (graph-owned) is reserved for content retries
//     (contract violations, validation feedback, etc.).
//
// Stall plateau detection: when the previous transient-failed attempt
// produced an identical non-empty tool-call signature under a typed
// stream-stall error, further retry would just re-hit the same wall
// (e.g. the LLM is structurally confused by the request shape).
// Ambiguous "<no-tools>" and pure transport faults stay governed by
// the transient budget instead of being promoted to a structural stop.
func (o *Orchestrator) retryReadStageDispatchError(
	state *graphState,
	stage types.PipelineStage,
	window []*types.TaskNode,
	fin *types.TaskNode,
	err error,
) bool {
	if o == nil || state == nil || o.busCtx == nil || err == nil {
		return false
	}
	// L4 only retries on stream-level errors (EOF / stream stalled /
	// first-byte timeout / network). HTTP 429 / 5xx are L1's domain
	// — by the time we see one here, L1 already exhausted its
	// 6-attempt × 62-second budget; retrying at L4 would burn 2×
	// more wall-clock for the same persistent upstream condition.
	// The IsStreamLevelRetryable subset filters this.
	if o.busCtx.Mode != types.ModeRead || !llm.IsStreamLevelRetryable(err) {
		return false
	}

	if stage == types.StageExplore && o.completeExploreWindowAfterTransientProgress(state, window, err) {
		return true
	}

	// Transient retry uses its own dedicated budget so stream stalls
	// don't drain the content-retry budget.
	if state.transientRetryUsed >= o.transientRetryBudget {
		return false
	}

	// Stall plateau detection. Compute a signature for the just-failed
	// dispatch and compare against the previous transient-retry's
	// signature. Identical pair (with no terminal emit between) means
	// "stuck on the same wall"; surface terminal failure rather than
	// burn another retry. Use the first window node's ID as the key
	// for explore (any node in the failing window suffices since the
	// whole window stalls on the same dispatch error). Finalize uses
	// the finalize node's ID.
	var keyID string
	switch stage {
	case types.StageExplore:
		if len(window) == 0 {
			return false
		}
		keyID = window[0].ID
	case types.StageFinalize:
		if fin == nil {
			return false
		}
		keyID = fin.ID
	default:
		return false
	}
	sig := computeStallSignature(o.busCtx.Mutable.DispatchToolResults(), nil, stage)
	if transientHardPlateauEligible(err, sig) && state.transientStallPlateau(keyID, sig) {
		friendly := stallPlateauMessage(o.busCtx, stage, friendlyDispatchErr(err), o.autoInitRepo, o.scaffoldEnabled)
		logging.Warning("[orchestrator] %s read-mode transient stall plateau (sig=%q) — suppressing retry: %s",
			stage, sig, friendly)
		o.busCtx.TaskState.LastError = friendly
		return false
	}
	state.rememberTransientSignature(keyID, sig)

	checkpointHint := ""
	if stage == types.StageExplore {
		checkpointHint = o.buildExploreTransientRetryCheckpointHint()
		if checkpointHint != "" {
			state.setTransientRetryHint(checkpointHint)
			logging.Debug("[diag orchestrator] phase=transient_retry_checkpoint stage=%s installed=true len=%d",
				stage, len(checkpointHint))
		}
	}

	switch stage {
	case types.StageExplore:
		for _, n := range window {
			state.requeue(n.ID)
		}
	case types.StageFinalize:
		state.requeue(fin.ID)
	}

	state.recordTransientRetry()
	logging.Warning("[orchestrator] retrying %s after transient dispatch error (%d/%d transient budget; pipeline step budget unchanged): %v",
		stage, state.transientRetryUsed, o.transientRetryBudget, err)
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeRetry,
		Reasoning:  o.softTransientRetryNotice(stage, checkpointHint != ""),
	})
	return true
}

type exploreTransientRetryCheckpoint struct {
	evidenceRows   int
	flowFindings   int
	answerChains   int
	answerSymbols  int
	aggregateFacts int
	readFiles      int
	toolResults    int
	hasClosure     bool
	typedOrigins   string
	proofGuidance  string
}

func (c exploreTransientRetryCheckpoint) hasProgress() bool {
	return c.evidenceRows > 0 ||
		c.flowFindings > 0 ||
		c.answerChains > 0 ||
		c.answerSymbols > 0 ||
		c.aggregateFacts > 0 ||
		c.readFiles > 0 ||
		c.toolResults > 0 ||
		c.hasClosure ||
		c.typedOrigins != "" ||
		c.proofGuidance != ""
}

func (o *Orchestrator) buildExploreTransientRetryCheckpointHint() string {
	if o == nil || o.busCtx == nil {
		return ""
	}
	c := exploreTransientRetryCheckpoint{
		evidenceRows:  len(o.busCtx.EvidenceItems),
		flowFindings:  len(o.busCtx.FlowFindings),
		answerChains:  len(o.busCtx.AnswerChains),
		answerSymbols: len(o.busCtx.AnswerSymbols),
		toolResults:   countSuccessfulToolResults(o.busCtx.ToolResults),
		typedOrigins:  transientRetryTypedObservationSummary(o.busCtx),
	}
	if o.busCtx.Mutable != nil {
		c.evidenceRows += len(o.busCtx.Mutable.EmittedEvidence())
		c.toolResults += countSuccessfulToolResults(o.busCtx.Mutable.DispatchToolResults())
		if artifacts := o.busCtx.Mutable.TurnAArtifacts(); artifacts != nil {
			c.evidenceRows += len(artifacts.EvidenceItems)
			c.flowFindings += len(artifacts.FlowFindings)
			c.aggregateFacts += len(artifacts.AcceptedAggregateFacts)
			c.readFiles += len(artifacts.ReadFiles)
			c.toolResults += countSuccessfulToolResults(artifacts.ToolResults)
			c.hasClosure = strings.TrimSpace(artifacts.AcceptedClosureReason) != "" ||
				strings.TrimSpace(artifacts.AcceptedResultKind) != ""
			c.proofGuidance = readProofGuidanceSummaryFromTurnA(artifacts)
		}
		if len(o.busCtx.Mutable.StableInvestigationAggregateFacts()) > 0 {
			c.aggregateFacts += len(o.busCtx.Mutable.StableInvestigationAggregateFacts())
		}
		if strings.TrimSpace(o.busCtx.Mutable.StableInvestigationCompleteReason()) != "" ||
			strings.TrimSpace(o.busCtx.Mutable.StableInvestigationResultKind()) != "" {
			c.hasClosure = true
		}
	}
	if !c.hasProgress() {
		return ""
	}
	var facts []string
	if c.evidenceRows > 0 {
		facts = append(facts, fmt.Sprintf("structured evidence rows=%d", c.evidenceRows))
	}
	if c.flowFindings > 0 {
		facts = append(facts, fmt.Sprintf("flow findings=%d", c.flowFindings))
	}
	if c.answerChains > 0 {
		facts = append(facts, fmt.Sprintf("answer chains=%d", c.answerChains))
	}
	if c.answerSymbols > 0 {
		facts = append(facts, fmt.Sprintf("answer symbols=%d", c.answerSymbols))
	}
	if c.aggregateFacts > 0 {
		facts = append(facts, fmt.Sprintf("aggregate facts=%d", c.aggregateFacts))
	}
	if c.readFiles > 0 {
		facts = append(facts, fmt.Sprintf("read files=%d", c.readFiles))
	}
	if c.toolResults > 0 {
		facts = append(facts, fmt.Sprintf("successful tool results=%d", c.toolResults))
	}
	if c.typedOrigins != "" {
		facts = append(facts, "typed observation origins="+c.typedOrigins)
	}
	if c.proofGuidance != "" {
		facts = append(facts, c.proofGuidance)
	}
	if c.hasClosure {
		facts = append(facts, "accepted closure state present")
	}
	return strings.Join([]string{
		"A transient model stream error interrupted the previous explore dispatch after durable progress was preserved.",
		"Checkpoint summary (non-authoritative counts): " + strings.Join(facts, "; ") + ".",
		"Continue from this checkpoint; this is a continuation, not a fresh investigation. Reuse the accepted evidence and already-read context visible in the transcript. Avoid repeating broad repository-wide navigation or identical broad searches unless the checkpoint clearly lacks the target needed for the active objective.",
		"Treat any generic DAG/window objectives that follow as remaining-objective context only; they do not reopen broad search once the checkpoint already covers the active objective.",
		"If a specific anchor is still missing, do a narrow follow-up read/search for that anchor. If the checkpoint is enough, call emit_investigation_complete. This checkpoint is advisory only: it is not new evidence and it does not decide sufficiency for you.",
	}, "\n\n")
}

func transientRetryTypedObservationSummary(bus *types.BusContext) string {
	if bus == nil {
		return ""
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, transientRetryObservationLedgerInputLimit))
	if len(ledger.Records) == 0 {
		return ""
	}
	counts := make(map[types.AnswerEvidenceOrigin]int)
	for _, record := range ledger.Records {
		if !types.AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
			continue
		}
		counts[record.Origin]++
	}
	if len(counts) == 0 {
		return ""
	}
	var parts []string
	for _, origin := range types.AllAnswerEvidenceOrigins() {
		if count := counts[origin]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", origin, count))
		}
	}
	return strings.Join(parts, ", ")
}

func (o *Orchestrator) buildExploreFactRetryContinuationHint(output *agent.StageOutput) string {
	if o == nil || o.busCtx == nil {
		return ""
	}
	c := exploreTransientRetryCheckpoint{
		evidenceRows:  len(o.busCtx.EvidenceItems),
		flowFindings:  len(o.busCtx.FlowFindings),
		answerChains:  len(o.busCtx.AnswerChains),
		answerSymbols: len(o.busCtx.AnswerSymbols),
		toolResults:   countSuccessfulToolResults(o.busCtx.ToolResults),
		typedOrigins:  transientRetryTypedObservationSummary(o.busCtx),
	}
	if output != nil {
		c.evidenceRows += len(output.EvidenceItems)
		c.flowFindings += len(output.FlowFindings)
		c.answerChains += len(output.AnswerChains)
		c.answerSymbols += len(output.AnswerSymbols)
		c.toolResults += countSuccessfulToolResults(output.ToolResults)
	}
	var aggregateFacts []types.AnswerAggregateFact
	var toolResults []types.ToolResult
	if o.busCtx.Mutable != nil {
		c.evidenceRows += len(o.busCtx.Mutable.EmittedEvidence())
		dispatchResults := o.busCtx.Mutable.DispatchToolResults()
		c.toolResults += countSuccessfulToolResults(dispatchResults)
		toolResults = append(toolResults, dispatchResults...)
		if artifacts := o.busCtx.Mutable.TurnAArtifacts(); artifacts != nil {
			c.evidenceRows += len(artifacts.EvidenceItems)
			c.flowFindings += len(artifacts.FlowFindings)
			c.aggregateFacts += len(artifacts.AcceptedAggregateFacts)
			c.readFiles += len(artifacts.ReadFiles)
			c.toolResults += countSuccessfulToolResults(artifacts.ToolResults)
			c.hasClosure = strings.TrimSpace(artifacts.AcceptedClosureReason) != "" ||
				strings.TrimSpace(artifacts.AcceptedResultKind) != ""
			c.proofGuidance = readProofGuidanceSummaryFromTurnA(artifacts)
			aggregateFacts = append(aggregateFacts, artifacts.AcceptedAggregateFacts...)
			toolResults = append(toolResults, artifacts.ToolResults...)
		}
		stableFacts := o.busCtx.Mutable.StableInvestigationAggregateFacts()
		if len(stableFacts) > 0 {
			c.aggregateFacts += len(stableFacts)
			aggregateFacts = append(aggregateFacts, stableFacts...)
		}
		if strings.TrimSpace(o.busCtx.Mutable.StableInvestigationCompleteReason()) != "" ||
			strings.TrimSpace(o.busCtx.Mutable.StableInvestigationResultKind()) != "" {
			c.hasClosure = true
		}
	}
	toolResults = append(toolResults, o.busCtx.ToolResults...)
	if output != nil {
		toolResults = append(toolResults, output.ToolResults...)
	}
	if !c.hasProgress() && len(toolResults) == 0 {
		return ""
	}

	var facts []string
	if c.evidenceRows > 0 {
		facts = append(facts, fmt.Sprintf("structured evidence rows=%d", c.evidenceRows))
	}
	if c.flowFindings > 0 {
		facts = append(facts, fmt.Sprintf("flow findings=%d", c.flowFindings))
	}
	if c.answerChains > 0 {
		facts = append(facts, fmt.Sprintf("answer chains=%d", c.answerChains))
	}
	if c.answerSymbols > 0 {
		facts = append(facts, fmt.Sprintf("answer symbols=%d", c.answerSymbols))
	}
	if c.aggregateFacts > 0 {
		facts = append(facts, fmt.Sprintf("aggregate facts=%d", c.aggregateFacts))
	}
	if c.readFiles > 0 {
		facts = append(facts, fmt.Sprintf("read files=%d", c.readFiles))
	}
	if c.toolResults > 0 {
		facts = append(facts, fmt.Sprintf("successful tool results=%d", c.toolResults))
	}
	if c.typedOrigins != "" {
		facts = append(facts, "typed observation origins="+c.typedOrigins)
	}
	if c.proofGuidance != "" {
		facts = append(facts, c.proofGuidance)
	}
	if c.hasClosure {
		facts = append(facts, "accepted closure state present")
	}
	if len(facts) == 0 {
		facts = append(facts, "previous tool results preserved")
	}

	body := []string{
		exploreFactRetryCheckpointPrefix,
		"Previous explore dispatch preserved structured progress but still requested a fact retry. Continue from this checkpoint; this is not a fresh investigation.",
		"Checkpoint summary (non-authoritative counts): " + strings.Join(facts, "; ") + ".",
		"Reuse accepted evidence, aggregate facts, closure reason, already-read files, and tool-result summaries visible in the transcript before doing any broad rediscovery. This checkpoint is advisory only: it is not new evidence and it does not decide sufficiency for you.",
	}
	if exploreRuntimeTraceContinuationLikely(o.busCtx, toolResults) {
		body = append(body, "Runtime/log/trace continuation: continue from already discovered line windows, timestamps, thread ids, event names, and current chain frontier. Prefer `read_file` around returned line numbers or `grep` with `line_start`/`line_end`; use `fixed_string=true` for punctuation-heavy literals. For numeric time-window filtering, a deterministic command is acceptable, then ground the selected vicinity with `read_file` before emitting line-scope evidence.")
	} else if exploreChainContinuationLikely(o.busCtx) {
		body = append(body, "Chain/sequence continuation: keep the current frontier explicit. If one hop remains unresolved, do a narrow follow-up for that hop instead of restarting from all discovered candidates.")
	}
	if agg := aggregateFactContinuationSummary(aggregateFacts, 3); agg != "" {
		body = append(body, "Preserved aggregate facts: "+agg)
	}
	if hints := continuationToolResultHints(toolResults, 8); len(hints) > 0 {
		body = append(body, "Useful preserved tool-result hints:\n- "+strings.Join(hints, "\n- "))
	}
	body = append(body, "If the checkpoint already covers the active objective, call `emit_investigation_complete`. If a concrete anchor is still missing, use one narrow search/read for that anchor, then materialize or close.")
	return strings.Join(body, "\n\n")
}

func readProofGuidanceSummaryFromTurnA(artifacts *types.TurnAArtifacts) string {
	guidance, ok := loopkernel.ReadProofGuidanceFromTurnA(artifacts)
	if !ok || !guidance.Active {
		return ""
	}
	mode := "covered"
	if guidance.HardBlock {
		mode = "hard"
	} else if guidance.Advisory {
		mode = "advisory"
	}
	return fmt.Sprintf("proof authority=%s reason=%s action=%s mode=%s",
		firstNonEmptyRetryString(string(guidance.State), "unknown"),
		firstNonEmptyRetryString(guidance.ReasonCode, "none"),
		firstNonEmptyRetryString(string(guidance.RecommendedAction), "none"),
		mode)
}

func firstNonEmptyRetryString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func exploreRuntimeTraceContinuationLikely(bus *types.BusContext, toolResults []types.ToolResult) bool {
	if bus == nil {
		return false
	}
	if strings.TrimSpace(bus.AttachedLog) != "" || strings.TrimSpace(bus.AttachedHitrace) != "" {
		return true
	}
	if bus.AnalysisIR != nil {
		rm := bus.AnalysisIR.RequestModel
		if rm.HasExternalOnlyRuntimeArtifact() || rm.HasObservationOnlyRuntimeArtifact() {
			return true
		}
	}
	for _, tr := range toolResults {
		s := strings.ToLower(tr.Summary)
		if strings.Contains(s, "runtime/log/trace") ||
			strings.Contains(s, "runtime artifact") ||
			strings.Contains(s, ".systrace") ||
			strings.Contains(s, ".htrace") ||
			strings.Contains(s, ".atrace") ||
			strings.Contains(s, ".perfetto") ||
			strings.Contains(s, ".trace") ||
			strings.Contains(s, ".log") {
			return true
		}
	}
	return false
}

func exploreChainContinuationLikely(bus *types.BusContext) bool {
	if bus == nil || bus.AnalysisIR == nil {
		return false
	}
	rm := bus.AnalysisIR.RequestModel
	family := types.ResolveQuestionFamily(rm)
	if family == types.QFRootCauseTrace || family == types.QFCallChain {
		return true
	}
	if rm.DiagramHint != nil {
		switch rm.DiagramHint.Kind {
		case types.DiagramSequence, types.DiagramCallDAG:
			return true
		}
	}
	return false
}

func aggregateFactContinuationSummary(facts []types.AnswerAggregateFact, maxItems int) string {
	if maxItems <= 0 || len(facts) == 0 {
		return ""
	}
	out := make([]string, 0, maxItems)
	seen := map[string]bool{}
	for _, fact := range facts {
		label := strings.TrimSpace(fact.Label)
		value := strings.TrimSpace(fact.Value)
		if label == "" && value == "" {
			continue
		}
		text := label
		if value != "" {
			if text != "" {
				text += "="
			}
			text += value
		}
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, logging.Truncate(text, 160))
		if len(out) >= maxItems {
			break
		}
	}
	return strings.Join(out, "; ")
}

func continuationToolResultHints(results []types.ToolResult, maxItems int) []string {
	if maxItems <= 0 || len(results) == 0 {
		return nil
	}
	prefixes := []string{
		"line_window_hint=",
		"line_windows=",
		"next_shape=",
		"no_match_advisory=",
		"line_window_note=",
		"regex_compatibility_note=",
	}
	seen := map[string]bool{}
	var out []string
	for i := len(results) - 1; i >= 0 && len(out) < maxItems; i-- {
		for _, line := range strings.Split(results[i].Summary, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			for _, prefix := range prefixes {
				if strings.HasPrefix(trimmed, prefix) {
					text := logging.Truncate(trimmed, 220)
					if !seen[text] {
						seen[text] = true
						out = append(out, text)
					}
					break
				}
			}
			if len(out) >= maxItems {
				break
			}
		}
	}
	return out
}

func countSuccessfulToolResults(results []types.ToolResult) int {
	n := 0
	for _, r := range results {
		if r.Success {
			n++
		}
	}
	return n
}

func (o *Orchestrator) softTransientRetryNotice(stage types.PipelineStage, checkpoint bool) string {
	if o == nil || o.busCtx == nil {
		return softTransportRetryHintForStage("", stage)
	}
	if checkpoint {
		return softTransportCheckpointRetryHintForStage(o.busCtx.Language, stage)
	}
	return softTransportRetryHintForStage(o.busCtx.Language, stage)
}

// completeExploreWindowAfterTransientProgress handles the narrow but
// important case where the explorer already passed the typed completion
// contract, then the transport failed before dispatchStage could return
// cleanly. Retrying the whole explore window in that state throws away
// durable progress and can make the model redo a long investigation from
// round 1. We only take this path when the same accepted-closure predicate
// used by the normal post-dispatch scheduler path says it is safe to move
// forward; otherwise the ordinary transient retry remains in charge.
func (o *Orchestrator) completeExploreWindowAfterTransientProgress(
	state *graphState,
	window []*types.TaskNode,
	err error,
) bool {
	if o == nil || state == nil || o.busCtx == nil || o.busCtx.Mutable == nil || len(window) == 0 {
		return false
	}
	if !o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "") {
		return false
	}
	o.busCtx.Signals.HasEnoughFacts = true
	o.busCtx.TaskState.LastError = ""
	for _, n := range window {
		state.markDone(n.ID)
		o.emitNodeEnd(n.ID, true, "")
	}
	o.runAutoVerdicts()
	o.drainHypothesisVerdicts()
	logging.Warning("[orchestrator] suppressing explore transient retry after accepted investigation closure; proceeding with collected evidence: %v", err)
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeInvestigationReady,
		Reasoning:  softProceedWithCollectedEvidenceMessage(o.busCtx.Language),
	})
	return true
}

// retryReadStandaloneDispatchError is the same transient read-mode
// retry lane for stages that are dispatched as scheduler side-effects
// rather than graph nodes. Today that means the pre-finalize extract
// calls. A dropped stream there used to fall through as "proceeding
// without extract", which let a transport blip erase Turn-B answer
// symbol / verdict work. The retry is capped by transientRetryBudget
// and deliberately does not consume the pipeline step budget.
func (o *Orchestrator) retryReadStandaloneDispatchError(
	state *graphState,
	stage types.PipelineStage,
	err error,
) bool {
	if o == nil || state == nil || o.busCtx == nil || err == nil {
		return false
	}
	if o.busCtx.Mode != types.ModeRead || !llm.IsStreamLevelRetryable(err) {
		return false
	}
	if state.transientRetryUsed >= o.transientRetryBudget {
		return false
	}
	state.recordTransientRetry()
	o.busCtx.TaskState.LastError = ""
	logging.Warning("[orchestrator] retrying standalone %s after transient dispatch error (%d/%d transient budget; pipeline step budget unchanged): %v",
		stage, state.transientRetryUsed, o.transientRetryBudget, err)
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeRetry,
		Reasoning:  softTransportRetryHintForStage(o.busCtx.Language, stage),
	})
	return true
}
