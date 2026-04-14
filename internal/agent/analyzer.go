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
// RunPolicy, QualityGate) is derived deterministically after the
// ReAct loop exits.
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
//
// The LLM never sees the legacy AnalyzerClassification carrier, the
// todo_write tool, or any translation layer from legacy question_kind
// strings to v3 Intent values — that path was deleted when
// emit_analysis shipped.
type analyzerEvaluator struct{}

func (e *analyzerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	_ = ctx
	_ = sk
	// The analyzer is the first LLM in the pipeline to see the raw
	// user request. Its only job is to call emit_analysis ONCE with
	// the classification fields below. The deterministic v3 pipeline
	// (normalizer → compiler → risk → hdp → counterfactual → gate)
	// runs in ParseOutput and does not need any free-form rationale.
	return `You are the analyzer. Classify the user's request by calling the emit_analysis tool EXACTLY ONCE. Every field below feeds a deterministic downstream stage — vague input here cascades into wasted search and hallucinated answers.

# Required emit_analysis fields

intent        — the v3 task intent. Pick one:
   explain         — user wants to understand how something works
   root_cause      — user is debugging, asks "why does X fail"
   trace           — follow a data flow or call chain end to end
   enumerate       — list every X, count Xs (also for "which agents call Y")
   config_query    — look up what a config key controls
   return_value    — asks what a specific function returns or its literal name
   refactor        — design or implement a code change
   bugfix          — fix a defect
   security_audit  — review for vulnerabilities
   unknown         — genuinely ambiguous (ERM will fall back to keyword inference)

scenario      — which scenario template drives the investigation plan:
   architecture_explain    — explain mechanism / code / flow
   root_cause              — debug a failure
   security_audit          — vulnerability review
   refactor_design         — plan a refactor
   config_trace            — trace config → behaviour
   performance_bottleneck  — find a perf hotspot
   generic                 — none of the above (safe fallback)

complexity    — "simple" (single lookup/count, 1-2 files), "moderate" (single component, 3-5 files), "complex" (cross-component, 6+ files).

writing       — true only if the task may MUTATE files; false for read/explain/audit.
high_risk     — true only when writing=true AND the change needs design/code review (security, schema, irreversible). Always false for read-only work.

keywords      — ≥8 grep search terms. Include every CamelCase / snake_case identifier the user wrote PLUS conceptual synonyms. For Chinese questions include BOTH Chinese and English forms (the codebase is English).
entities      — CamelCase / snake_case symbol names copied VERBATIM from the user's wording. Do NOT translate, re-case, pluralise, or paraphrase. Generic nouns (count, function, thing, agent, handler, module) MUST NOT appear here — they poison ERM ranking. Leave empty only when the question has no identifier-looking tokens.
question_kind — ERM predicate selector. Pick one:
   registration   — "which/how many X register/bind Y", "X 是在哪注册的"
   mechanism      — "how does X work", "explain the process of X", "X 怎么实现"
   return_value   — "what does X return", "X.Name() 是什么"
   conditional    — "when does X fire", "under what condition", "什么时候"
   config_mapping — "what does config key K control"
   enumeration    — "list all X", "count of X"
   call_chain     — "which X calls Y", "从 A 到 B 怎么调用的"
   unknown        — genuinely ambiguous
answer_shape  — the finalizer's anti-hallucination selector:
   list_of_symbols — answer is a set of identifier names (forbids symbols not in Ground Truth evidence)
   step_list       — ordered steps of a mechanism
   value           — a single literal / returned value
   boolean         — yes/no
   config_value    — a resolved config key value
   explanation     — long prose explanation
   none            — no structured shape applies

# Hard rules

1. Every field in emit_analysis is REQUIRED (keywords and entities may be empty arrays). Missing required fields rejects the call.
2. entities come from the user's ORIGINAL text only. "ContinuationPrompt" stays as "ContinuationPrompt" — not "continuation prompt", not "continuation_prompt", not "prompt".
3. Do not invent an intent by stretching a category. If two fit equally, choose the one that directly matches the user's verb. If none fit, use "unknown".
4. answer_shape=list_of_symbols ONLY when the user is asking for a SET of names they want to see listed. "How many agents call X" is list_of_symbols (they want the names even if phrased as a count). "Is X registered" is boolean. "Explain X" is step_list or explanation.
5. Do NOT call any other tool. Do NOT write free-form prose before the tool call. emit_analysis is the only output channel for the analyze stage.
6. After emit_analysis succeeds, you may add a one-paragraph rationale for the trace log. This text is captured but does not drive any agent — the structured fields are what matter.`
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

	// v3: ensure the single-task fallback list exists so downstream
	// stages always have a task to route on. The analyzer no longer
	// decomposes the request into sibling tasks — v3 treats every
	// request as one analysis task whose decomposition happens inside
	// the TaskGraph, not inside TaskList.
	if ctx.Mutable != nil {
		if tl := ctx.Mutable.TaskList(); len(tl.Tasks) == 0 {
			tl.Tasks = []types.TaskItem{{
				ID:     "task-1",
				Title:  tl.Objective,
				Status: types.TaskPending,
			}}
			tl.CurrentTaskID = "task-1"
			ctx.Mutable.SetTaskList(tl)
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
//	risk.DerivePolicy      — frozen RunPolicy
//	hdp.Plan + hdp.Bind    — hypotheses + binding
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

	// Risk matrix + run policy. Pre-normalisation risk evidence is
	// empty — risk.Evaluate decorates it from the term graph.
	rm.RiskMatrix = risk.Evaluate(rm, rm.RiskMatrix)
	runPolicy := risk.DerivePolicy(rm.RiskMatrix, rm.Writing)
	if rm.HighRisk {
		runPolicy.RequireDesignReview = true
		runPolicy.RequireCodeReview = true
	}

	// Hypothesis planning and binding.
	hypotheses := hdp.Plan(rm)
	hdp.Bind(&out.TaskGraph, hypotheses)

	// Optional counterfactual expansion (default off; only fires on
	// complex + ambiguous explain/root_cause/security_audit).
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
		RunPolicy:      runPolicy,
		TraceID:        ctx.CurrentTaskID,
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
