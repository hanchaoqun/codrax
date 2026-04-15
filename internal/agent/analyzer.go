package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/analysis/counterfactual"
	"github.com/hanchaoqun/codrax/internal/analysis/gate"
	"github.com/hanchaoqun/codrax/internal/analysis/hdp"
	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/analysis/risk"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzerEvaluator drives the Analyzer v3 analyze stage. The agent
// makes a single LLM call whose only job is to emit a RequestModel
// through the emit_analysis tool; everything else about AnalysisIR
// (TermGraph, TaskGraph, EvidencePlan, AnswerContract, Hypotheses,
// QualityGate) is derived deterministically after the ReAct loop exits.
//
// Pipeline:
//
//  1. BuildInitialPrompt asks the LLM to call emit_analysis with the
//     classified v3 fields (intent, scenario, complexity, writing,
//     keywords, entities, question_kind, answer_shape).
//  2. ShouldStop ends the loop on the first LLM response with no
//     pending tool calls.
//  3. ParseOutput reads the emitted RequestModel from Mutable,
//     normalises the raw objective through the term-graph builder,
//     then runs compiler → risk → hdp → counterfactual → gate to
//     assemble a full AnalysisIR.
//  4. The quality gate's Rejected flag is consulted at the end:
//     structural failures (dag_closure, contract_complete, nil_ir)
//     return a hard error that bubbles up through StageOutput.Error
//     and fails the run fast. Soft-quality failures (coverage,
//     budget_sanity, hypothesis_coverage, risk_consistency) log a
//     warning and the run continues.
type analyzerEvaluator struct{}

// BuildInitialPrompt returns the evaluator's DYNAMIC per-dispatch
// instruction — and for the analyzer, that slot is empty by design.
//
// Skill vs Evaluator contract (see docs/architecture.md §3.3):
//
//   - The skill ("analysis-skill" in internal/skill/analysis_contract.go)
//     owns everything STATIC about the analyze stage: Goal, Workflow,
//     OutputFormat (which holds the enum tables and keyword rules),
//     Prohibitions, and ToolSuggestions. Those fields are rendered into
//     the system prompt by context.BuildPromptContext for every dispatch.
//   - The evaluator's BuildInitialPrompt only contributes content that
//     depends on THIS dispatch's runtime state and cannot be expressed
//     declaratively in the skill. It must NEVER restate the skill's
//     static contract and must NEVER re-emit sections that
//     context.BuildPromptContext already renders (see the canonical
//     section list in internal/context/builder.go:BuildPromptContext).
//
// The analyzer is the minimal case of that contract. Every dynamic
// input the agent needs is already surfaced by the builder:
//
//   - the user request → "User Request" user section;
//   - the language preference → "User Preferences" system section;
//   - retry hints from a failed prior analyze → "Retry Directive" user
//     section.
//
// Post-processing is the analyzer's other responsibility and lives in
// ParseOutput, which runs the deterministic normalizer → compiler →
// risk → hdp → counterfactual → gate pipeline to assemble the full
// AnalysisIR from the single emit_analysis tool call.
//
// BaseAgent.buildInitialMessages drops an empty evaluator instruction,
// so returning "" here is the correct, documented way to say "this
// stage has no per-dispatch supplement beyond what the builder renders".
// The TestAnalyzer_BuildInitialPrompt_IsEmpty guard pins this contract
// so a future commit cannot re-seed static text here by accident.
func (e *analyzerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	_ = ctx
	_ = sk
	return ""
}

func (e *analyzerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	_ = iteration
	return len(resp.ToolCalls) == 0
}

func (e *analyzerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	// Extract the last assistant message as the human-readable summary.
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	// ------------------------------------------------------------------
	// emit_analysis call-count gate
	// ------------------------------------------------------------------
	//
	// The skill prompt says "call emit_analysis EXACTLY ONCE" but the
	// ReAct loop and the tool itself allow 0 or N calls in practice:
	//
	//   - 0 calls: buildAnalysisIR falls back to a zero-value
	//     RequestModel via readOrSynthesizeRequestModel. Downstream
	//     stages still see a structurally valid IR, so the run
	//     continues — but a silent 0-call path masks a model mismatch
	//     or prompt regression and is the number one reason "analyzer
	//     output looks weird but no error fired".
	//   - N > 1 calls: every call overwrites Mutable.RequestModel, so
	//     the last write wins. This is a best-effort recovery for an
	//     LLM that second-guessed itself, not an approved workflow.
	//
	// This gate counts emit_analysis entries in the dispatch's tool
	// result stream, surfaces the count + the fallback decision through
	// StageOutput.Data as `analysis_emit_calls` and
	// `analysis_fallback_used`, and — per the runtime-tunable
	// tool.AnalysisLimits.RejectMultipleEmit — optionally writes a
	// descriptive message into StageOutput.Error when the LLM called
	// more than once. The default policy is warn-and-continue; the
	// strict policy is for eval harnesses that want a loud signal when
	// a model drifts off contract.
	//
	// The fallback decision is captured BEFORE buildAnalysisIR so the
	// diagnostic reflects whether Mutable.RequestModel was nil at
	// dispatch end, not the post-synthesis state.
	emitCalls := countEmitAnalysisCalls(toolResults)
	fallbackUsed := ctx == nil || ctx.Mutable == nil || ctx.Mutable.RequestModel() == nil
	limits := tool.CurrentAnalysisLimits()
	var emitGateError string

	switch {
	case emitCalls == 0:
		logging.Warning(
			"[analyzer] emit_analysis was NOT called during the analyze dispatch; "+
				"falling back to zero-value RequestModel (objective=%q). This usually means "+
				"the model ignored the skill prompt or the tool schema — fix the prompt or "+
				"the model, not the fallback.",
			truncateForLog(ctxObjective(ctx), 120))
	case emitCalls > 1:
		if limits.RejectMultipleEmit {
			emitGateError = fmt.Sprintf(
				"analyzer emit_analysis called %d times (policy=reject): exactly one call "+
					"is required; only the last write is effective", emitCalls)
			logging.Error("[analyzer] %s", emitGateError)
		} else {
			logging.Warning(
				"[analyzer] emit_analysis called %d times (policy=warn-and-continue); "+
					"only the last write is effective", emitCalls)
		}
	}

	// Build the Analyzer v3 IR deterministically from the RequestModel
	// the LLM emitted via emit_analysis. Missing LLM output is not
	// fatal — buildAnalysisIR synthesises a minimal zero-value
	// RequestModel from the raw objective so downstream stages always
	// have a valid IR to read.
	ir := buildAnalysisIR(ctx)

	data := map[string]any{
		"result":                 lastContent,
		"analysis_emit_calls":    emitCalls,
		"analysis_fallback_used": fallbackUsed,
	}
	if ir != nil {
		hints := ir.RequestModel.AnalyzerHints
		data["question_kind"] = hints.Kind
		data["answer_shape"] = hints.Shape
		data["complexity"] = string(ir.RequestModel.Complexity)
		data["entity_count"] = len(hints.Entities)
		data["keyword_count"] = len(hints.Keywords)
		data["ir_version"] = ir.Version
		data["ir_scenario"] = string(ir.RequestModel.Scenario)
		data["ir_intent"] = string(ir.RequestModel.Intent)
		data["ir_nodes"] = len(ir.TaskGraph.Nodes)
		data["ir_hypotheses"] = len(ir.HypothesisSet)
		data["ir_gate_passed"] = ir.QualityGate.Passed
	}
	raw, err := json.Marshal(data)
	if err != nil {
		raw = json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent))
	}

	out := &StageOutput{
		Data:       raw,
		AnalysisIR: ir,
	}

	// Quality Gate enforcement. Structural failures (nil_ir,
	// dag_closure, contract_complete) are hard errors — downstream
	// stages cannot safely proceed on a malformed IR. Soft-quality
	// failures (coverage, budget_sanity, hypothesis_coverage,
	// risk_consistency) log a warning and the run continues so
	// evaluations can diff the soft failures without a wholesale
	// abort.
	if ir != nil && ir.QualityGate.Rejected {
		if hard, detail := classifyGateFailure(ir.QualityGate); hard {
			logging.Error("[analyzer-v3] quality gate HARD failure: %s", detail)
			out.Error = fmt.Sprintf("analyzer quality gate rejected: %s", detail)
			return out, nil
		}
		logging.Warning("[analyzer-v3] quality gate soft failure (continuing): %+v", ir.QualityGate.Checks)
	}

	// Emit-gate error surfaces AFTER the gate error so the gate's
	// hard-fail path still wins when both trigger, but the repeat-
	// call signal survives when the gate passed. Leaving the IR in
	// place gives downstream stages a "best we have" fallback even
	// when the operator picked the strict policy.
	if emitGateError != "" && out.Error == "" {
		out.Error = emitGateError
	}

	return out, nil
}

// countEmitAnalysisCalls walks the tool-result stream collected during
// the analyze dispatch and returns the number of times the LLM invoked
// emit_analysis. Both successful and failed calls are counted: the
// "intent count" is what the call-count gate is testing. A failed
// emit (e.g. rejected by the keyword-floor validator) still means the
// LLM misbehaved on the exactly-once contract.
func countEmitAnalysisCalls(toolResults []types.ToolResult) int {
	n := 0
	for _, r := range toolResults {
		if r.ToolName == "emit_analysis" {
			n++
		}
	}
	return n
}

// ctxObjective pulls the user objective out of an AgentContext for
// diagnostic messages. Returns an empty string when the context or
// its Mutable is nil, so the caller can decide how to format the
// diagnostic without a nil check of its own.
func ctxObjective(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	return ctx.Objective
}

// truncateForLog shortens a long string for a single-line log entry.
// The analyzer's fallback warning embeds the user objective so an
// operator can immediately see which question triggered the missed
// emit_analysis call, but very long objectives would overflow the
// log line.
func truncateForLog(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}

func (e *analyzerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, _ *StageOutput) types.MissingPiece {
	_ = ctx
	return types.MissingFacts
}

// NewAnalyzerAgent creates the analyzer agent (used in the analyze stage).
func NewAnalyzerAgent(deps *Dependencies) Agent {
	eval := &analyzerEvaluator{}
	return NewBaseAgent(types.AgentAnalyzer, deps, eval)
}

// ── Analyzer v3 IR builder ───────────────────────────────────────────

// buildAnalysisIR composes a full AnalysisIR from the emit_analysis-
// produced RequestModel plus the raw objective. It runs the full
// deterministic sub-package pipeline every time:
//
//	normalizer.Normalize   — canonical TermGraph from the raw request
//	compiler.InferScenario — scenario default when LLM omitted one
//	compiler.Compile       — TaskGraph + EvidencePlan + AnswerContract
//	risk.Evaluate          — 6-dim risk matrix
//	hdp.Plan + hdp.Bind    — hypotheses + binding (risk-driven)
//	counterfactual.Expand  — optional branch expansion
//	gate.Run               — deterministic quality gate
//
// When the LLM never called emit_analysis, buildAnalysisIR uses a
// zero-value RequestModel seeded with the raw objective — the
// compiler's generic template and a zero RiskMatrix are a valid
// starting point that still produces a structurally complete IR.
func buildAnalysisIR(ctx *types.AgentContext) *types.AnalysisIR {
	if ctx == nil {
		return nil
	}

	rm := readOrSynthesizeRequestModel(ctx)

	// Normalizer runs unconditionally on the raw objective so
	// downstream Coverage checks have real symbols to work with.
	rm.TermGraph = normalizer.Normalize(rm.RawRequest, normalizer.Options{})

	// Scenario default — LLM may have omitted or the compiler's
	// InferScenario may produce a safer pick given the term graph.
	if rm.Scenario == "" || rm.Scenario == types.ScenarioGeneric {
		if s := compiler.InferScenario(rm); s != "" {
			rm.Scenario = s
		}
	}

	// Scenario compilation — templates produce TaskGraph, EvidencePlan,
	// and a default AnswerContract. The LLM's explicit answer_shape
	// overrides the template default when supplied.
	out := compiler.Compile(rm)
	if rm.AnalyzerHints.Shape != "" {
		if shape := mapLegacyAnswerShape(rm.AnalyzerHints.Shape); shape != "" {
			out.AnswerContract.RequiredAnswerShape = shape
		}
	}
	if out.AnswerContract.Language == "" {
		out.AnswerContract.Language = rm.Language
	}

	// Risk matrix. Pre-normalisation risk evidence is empty —
	// risk.Evaluate decorates it from the term graph. The matrix
	// feeds hdp.Plan for risk-driven hypotheses.
	rm.RiskMatrix = risk.Evaluate(rm, rm.RiskMatrix)

	// Hypothesis planning and binding.
	hypotheses := hdp.Plan(rm)
	hdp.Bind(&out.TaskGraph, hypotheses)

	// Optional counterfactual expansion (default off; only fires on
	// complex + ambiguous explain/root_cause).
	if counterfactual.ShouldExpand(rm) {
		expanded, newIDs := counterfactual.Expand(
			out.TaskGraph, rm,
			counterfactual.Options{Enabled: true, MaxBranches: 1},
		)
		out.TaskGraph = expanded
		if len(newIDs) > 0 {
			hdp.Bind(&out.TaskGraph, hypotheses)
		}
	}

	ir := &types.AnalysisIR{
		Version:        types.AnalysisIRVersion,
		RequestModel:   rm,
		TaskGraph:      out.TaskGraph,
		EvidencePlan:   out.EvidencePlan,
		AnswerContract: out.AnswerContract,
		HypothesisSet:  hypotheses,
	}

	// Quality gate.
	ir.QualityGate = gate.Run(ir, gate.Thresholds{})
	return ir
}

// readOrSynthesizeRequestModel returns the LLM-emitted RequestModel
// from MutableState, or a zero-value fallback seeded with the raw
// objective when the analyzer never called emit_analysis.
func readOrSynthesizeRequestModel(ctx *types.AgentContext) types.RequestModel {
	raw := ctx.Objective
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			if rm.RawRequest == "" {
				rm.RawRequest = raw
			}
			if rm.Language == "" {
				rm.Language = detectLanguage(raw, ctx.Preferences)
			}
			if rm.Complexity == "" {
				rm.Complexity = types.ComplexityModerate
			}
			if rm.Intent == "" {
				rm.Intent = types.IntentUnknown
			}
			return *rm
		}
	}
	// Fallback: analyzer never called emit_analysis. Synthesise a
	// zero-value RequestModel so downstream stages still see a
	// structurally valid IR.
	return types.RequestModel{
		RawRequest: raw,
		Language:   detectLanguage(raw, ctx.Preferences),
		Intent:     types.IntentUnknown,
		Scenario:   types.ScenarioGeneric,
		Complexity: types.ComplexityModerate,
	}
}

// classifyGateFailure inspects a GateReport and returns whether the
// failure is structural (hard fail) along with a short detail
// string. Structural failures: nil_ir, dag_closure, contract_complete.
// Everything else (coverage, budget_sanity, hypothesis_coverage,
// risk_consistency) is soft and logs a warning without aborting.
func classifyGateFailure(report types.GateReport) (hard bool, detail string) {
	for _, c := range report.Checks {
		if c.Passed {
			continue
		}
		switch c.Name {
		case "nil_ir", "dag_closure", "contract_complete":
			return true, fmt.Sprintf("%s: %s", c.Name, c.Detail)
		}
	}
	return false, ""
}

// mapLegacyAnswerShape coerces a free-form answer_shape string into
// the typed AnswerShape enum. Unknown values return "" so the
// compiler's template default stays in place.
func mapLegacyAnswerShape(s string) types.AnswerShape {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "list_of_symbols":
		return types.ShapeListOfSymbols
	case "step_list":
		return types.ShapeStepList
	case "value":
		return types.ShapeValue
	case "boolean":
		return types.ShapeBoolean
	case "config_value":
		return types.ShapeConfigValue
	case "explanation":
		return types.ShapeExplanation
	case "none":
		return types.ShapeNone
	}
	return ""
}

// detectLanguage picks "zh" when the raw request contains Han
// characters, "en" otherwise. Falls back to Preferences-declared
// language when the request is entirely non-alphabetic.
func detectLanguage(raw string, prefs []string) string {
	for _, r := range raw {
		if unicode.Is(unicode.Han, r) {
			return "zh"
		}
	}
	for _, p := range prefs {
		lp := strings.ToLower(p)
		if strings.Contains(lp, "chinese") || strings.Contains(lp, "zh") || strings.Contains(lp, "简体中文") {
			return "zh"
		}
	}
	return "en"
}
