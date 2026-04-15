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

// BuildInitialPrompt is intentionally empty. All static analyzer-
// contract text (field enums, descriptions, hard rules) lives in the
// single-source-of-truth tables in internal/skill/analysis_contract.go
// and flows into the system prompt through the skill rendering path
// in context/builder.go (Skill Goal / Workflow / Output Format /
// Prohibitions). The analyzer has no per-dispatch dynamic content
// that is not already injected elsewhere:
//
//   - the user request is rendered as the "User Request" user section;
//   - language preference flows through Preferences → "User
//     Preferences";
//   - retry hints flow through RetryHint → "Retry Directive".
//
// BaseAgent.buildInitialMessages skips an empty evaluator instruction,
// so returning "" here means the LLM sees the skill's system block
// exactly once instead of once from the skill path plus a second copy
// from this method.
func (e *analyzerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	_ = ctx
	_ = sk
	return ""
}

func (e *analyzerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	_ = iteration
	return len(resp.ToolCalls) == 0
}

func (e *analyzerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	// Extract the last assistant message as the human-readable summary.
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	// Build the Analyzer v3 IR deterministically from the RequestModel
	// the LLM emitted via emit_analysis. Missing LLM output is not
	// fatal — buildAnalysisIR synthesises a minimal zero-value
	// RequestModel from the raw objective so downstream stages always
	// have a valid IR to read.
	ir := buildAnalysisIR(ctx)

	data := map[string]any{
		"result": lastContent,
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

	return out, nil
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
