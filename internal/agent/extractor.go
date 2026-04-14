package agent

// extractor.go — P2.1 Turn B (extractor) evaluator.
//
// Status: Session 1 SKELETON. The four Evaluator methods compile and
// drain the MutableState emit_* buffers so the registry, scheduler,
// and orchestrator dispatch hook can all be wired up end-to-end with
// the flag set. The Session 2 scope adds:
//
//   - BuildInitialPrompt: read TurnAArtifacts.{InvestigationNotes,
//     ToolResults, EvidenceItems, ReadFiles}, bake a digest into the
//     prompt, instruct the LLM to call emit_evidence /
//     emit_answer_symbol / emit_hypothesis_verdict only, forbid
//     read_file / grep / repo_map.
//   - ShouldStop: tighten the one-shot policy with optional one
//     retry on schema-validation failure (matches P0.2's pattern).
//   - ParseOutput: cardinality validator (closes
//     extractAnswerSymbols enumeration completeness gap), drain
//     hypothesis verdicts via AnalysisIR.MarkHypothesis (D7), fail-
//     loud when retry budget is exhausted.
//
// Until then the skeleton runs as a no-op: it consumes one LLM call
// (so the integration test can assert dispatch ordering), reads the
// emit_* buffers, and returns whatever the buffers contained. Today
// those buffers are always empty because no upstream code calls the
// emit_* tools — Session 2 wires Turn A to populate TurnAArtifacts
// and the new extractor prompt teaches the LLM to call the tools.
//
// The skeleton is FLAG-GATED at the orchestrator layer
// (extractStageEnabled in scheduler.go) — it is only dispatched when
// TwoTurnExplorerEnabled() returns true, so leaving the prompt as a
// stub does not affect the legacy single-turn path.

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// extractorEvaluator is the Turn B evaluator. It is a separate type
// from explorerEvaluator so the two turns cannot accidentally share
// state (cross-run leakage was a real problem in pre-P2.1 code, see
// memory/project_repl_equivalence_audit.md).
//
// Session 2 will add cross-run reset fields (lastUserQuestion,
// retriesUsed) to mirror the explorer's pattern. For Session 1 the
// type is intentionally fieldless — adding fields now would invite
// an over-design that Session 2 has to undo.
type extractorEvaluator struct{}

// BuildInitialPrompt implements Evaluator.
//
// Session 1 stub: returns a brief Turn B instruction mentioning the
// constraint set so a debug trace makes it obvious which turn is
// running. Session 2 will replace this body with the real prompt
// builder that reads TurnAArtifacts and bakes a digest into the
// LLM context.
func (e *extractorEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	var b strings.Builder
	b.WriteString("# Extractor (Turn B) — P2.1 SKELETON\n\n")
	b.WriteString("This dispatch is the P2.1 two-turn explorer's extraction phase. ")
	b.WriteString("Turn A (the explorer) has finished its investigation; your job is to ")
	b.WriteString("drain the transcript into structured items via the emit_* tools.\n\n")
	b.WriteString("Allowed tools (call ONLY these — do NOT call read_file, grep, or repo_map):\n")
	b.WriteString("  - emit_evidence\n")
	b.WriteString("  - emit_answer_symbol\n")
	b.WriteString("  - emit_hypothesis_verdict\n\n")
	b.WriteString("Session 1 ship note: this prompt is a SKELETON. Session 2 wiring will ")
	b.WriteString("inject Turn A's investigation transcript, file-read snapshots, and ")
	b.WriteString("hypothesis set into this prompt. Until then the LLM will likely ")
	b.WriteString("produce no useful tool calls — that is expected and the orchestrator ")
	b.WriteString("dispatch hook handles the empty case gracefully.\n")
	if ctx != nil && ctx.CurrentTask != "" {
		b.WriteString("\nUser question: " + ctx.CurrentTask + "\n")
	}
	return b.String()
}

// ShouldStop implements Evaluator.
//
// Session 1 stub: hard-stop after a single LLM iteration. Turn B is
// fundamentally one-shot — it has no investigation phase, no soft-
// stop continuation, no ReAct loop. Even when Session 2 adds the
// "retry once on schema-validation failure" branch, the stop
// condition stays simple: 1 iteration normal-path, 2 iterations
// max with retry, no dynamic continuation.
func (e *extractorEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return iteration >= 1
}

// ParseOutput implements Evaluator.
//
// Session 1 stub: drain the three emit_* buffers from MutableState
// into StageOutput so the orchestrator's applyStageOutput merge sees
// whatever Turn B produced. Today the buffers will always be empty
// because (a) the prompt does not yet teach the LLM to call the
// tools and (b) the Session 1 ship runs default-off. Session 2 will
// add the cardinality validator + hypothesis-verdict drain via
// AnalysisIR.MarkHypothesis.
func (e *extractorEvaluator) ParseOutput(ctx *types.AgentContext, _ []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	out := &StageOutput{
		Data: json.RawMessage(`{}`),
	}
	if ctx == nil || ctx.Mutable == nil {
		return out, nil
	}
	if items := ctx.Mutable.EmittedEvidence(); len(items) > 0 {
		out.EvidenceItems = items
	}
	if syms := ctx.Mutable.EmittedAnswerSymbols(); len(syms) > 0 {
		out.AnswerSymbols = syms
	}
	// HypothesisVerdicts are not on StageOutput today — the Session 2
	// drain hook reads MutableState.EmittedHypothesisVerdicts() and
	// applies each one through AnalysisIR.MarkHypothesis(...). For
	// Session 1 we leave the buffer in place and let Session 2's
	// orchestrator/extractor wiring close the loop.
	return out, nil
}

// DetermineMissingPiece implements Evaluator.
//
// Turn B never triggers a backtrack on its own — when extraction
// fails, the orchestrator's contract checker downstream of finalize
// owns the retry decision. Returning MissingNone keeps the extractor
// out of the orchestrator's "what stage do we route to next" branch.
func (e *extractorEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingNone
}

// NewExtractorAgent constructs the Turn B agent. Mirrors
// NewFinalizerAgent in shape so the registry constructor table looks
// uniform.
func NewExtractorAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentExtractor, deps, &extractorEvaluator{})
}
