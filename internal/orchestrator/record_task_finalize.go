package orchestrator

import (
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// record_task_finalize.go — the finalize-record concern: copy the
// finalizer's answer onto the task result, log it for post-run audit,
// run the observation-only answer-style advisory, persist the
// transcript dump, and emit the objective-done event. Split out of
// orchestrator.go (STYLE-1, §29.96.3) when the advisory pushed that
// file over the IR-delivery line ratchet.

// recordTaskFinalize copies the finalizer's FinalAnswer into
// Mutable.result and emits the objective-done event. Empty answers
// are still recorded — callers downstream (render layer) treat an
// empty result as "no answer" and display the fail state instead.
func (o *Orchestrator) recordTaskFinalize(out *agent.StageOutput) {
	answer := ""
	if out != nil {
		answer = out.FinalAnswer
	}
	o.busCtx.Mutable.SetResult(answer)
	// INFO (post-2026-04-30): default log level captures the agent-
	// emitted raw markdown so post-run audit can find the answer
	// without enabling DEBUG. Pre-fix this was DEBUG, which meant
	// the only INFO-level record of the final answer was the REPL/
	// single-shot dispatch's own log line; if those changed shape
	// or got truncated, the orchestrator-level record was silently
	// invisible. Promotion is cheap (final answer is one log entry
	// per Run, ≤ 30 KB typical, no rotation impact).
	logging.Info("[orchestrator] final answer (len=%d):\n%s\n---", len(answer), answer)

	// STYLE-1 advisory lint (§29.96.3): count Chinese AI-register
	// filler phrases in the final answer. OBSERVATION ONLY — a noisy
	// signal per the precise-signals red line, so it is a WARN log
	// line for humans (and an eval observation column derived from
	// this line). It never gates emits, never enters a verdict,
	// never blocks the answer, and is never fed back into any LLM
	// prompt or retry hint. Zero hits log nothing (no noise).
	if hits, breakdown := skill.CountAnswerStyleFillerHits(answer); hits > 0 {
		logging.Warning("[orchestrator] answer style advisory (observation-only, never gates): ai_style_hits=%d %s", hits, breakdown)
	}

	// Final-answer transcript dump. Persist every non-empty final
	// answer, including best-effort fallback answers that never landed
	// an AnswerDocumentV2. The dump is the raw markdown answer body
	// that feeds REPL rendering, not ANSI/border terminal chrome.
	// Best-effort: the helper logs and swallows every IO error so the
	// dump never affects the rest of the pipeline.
	if o.outputDumpDir != "" && strings.TrimSpace(answer) != "" {
		dumpRequest := o.outputTranscriptRequestForDump()
		rootCauses := o.busCtx.Mutable.TraceRootCauseReport()
		if result := writeFinalOutputDumpResult(dumpFinalOutputArgs{
			dir:      o.outputDumpDir,
			max:      o.outputDumpMax,
			language: o.language,
			request:  dumpRequest,
			answer:   answer,
			hasLog:   o.attachedLog != "",
			logBytes: len(o.attachedLog),
			hasTrace: o.attachedHitrace != "",
			traceB:   len(o.attachedHitrace),
			artifacts: outputdump.MergeRuntimeArtifacts(
				outputdump.RuntimeArtifactsFromRequest(dumpRequest),
				outputdump.RuntimeArtifactsFromAttachment("log", o.attachedLog),
				outputdump.RuntimeArtifactsFromAttachment("trace", o.attachedHitrace),
			),
			rootCauses:                 rootCauses,
			rootCauseUnavailableReason: traceRootCauseUnavailableReason(o.busCtx.Mutable, rootCauses),
			now:                        time.Now(),
			pid:                        os.Getpid(),
		}); result.MarkdownPath != "" || result.HTMLPath != "" || result.RootCauseJSONPath != "" {
			o.busCtx.Mutable.SetFinalAnswerArtifactPaths(result.MarkdownPath, result.HTMLPath, result.RootCauseJSONPath)
		}
	}

	o.emit(render.Event{
		Kind:      render.EventObjectiveDone,
		Timestamp: time.Now(),
		Objective: o.busCtx.Mutable.Objective(),
	})
}

func traceRootCauseUnavailableReason(mutable *types.MutableState, report *types.TraceRootCauseReportV2) string {
	if report != nil {
		return ""
	}
	if mutable == nil {
		return "trace_root_cause_runtime_unavailable"
	}
	contract := mutable.TraceFindingContract()
	if contract == nil {
		return "trace_root_cause_contract_not_active"
	}
	if !contract.RootCauseReportEnabled {
		return "no_selectable_typed_on_chain_candidates"
	}
	return "valid_model_root_cause_selection_unavailable"
}

func (o *Orchestrator) outputTranscriptRequestForDump() string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		if o != nil && strings.TrimSpace(o.outputTranscriptRequest) != "" {
			return o.outputTranscriptRequest
		}
		return ""
	}
	if request := strings.TrimSpace(o.busCtx.Mutable.OutputTranscriptRequest()); request != "" {
		return request
	}
	return types.StripConversationPrefix(o.busCtx.Mutable.Objective())
}
