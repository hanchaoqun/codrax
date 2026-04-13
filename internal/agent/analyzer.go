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

// analyzerEvaluator customizes the ReAct loop for the analyzer agent,
// which is responsible only for the analyze stage: classify the user
// request and produce both the legacy task list AND the Analyzer v3
// structured AnalysisIR.
//
// Under Analyzer v3 (batch B5) the analyzer now runs a deterministic
// post-processing pipeline after the LLM has called todo_write:
//
//   1. LLM produces a legacy TaskItem via todo_write (unchanged prompt
//      contract; preserves the 35/35 baseline on df1/df3/t1-t5).
//   2. ParseOutput builds a RequestModel from the TaskItem + the raw
//      user objective. Language is auto-detected from Han characters,
//      intent is mapped from the legacy question_kind, and the term
//      graph comes from running the normalizer over the raw objective.
//   3. compiler.Compile produces the scenario-templated TaskGraph,
//      EvidencePlan, and AnswerContract. The AnswerContract's
//      required_answer_shape is overridden by the legacy answer_shape
//      field when the LLM supplied one, so classifier improvements
//      survive.
//   4. risk.Evaluate + risk.DerivePolicy freeze the RunPolicy.
//   5. hdp.Plan + hdp.Bind attach falsifiable hypotheses to every
//      non-probe node.
//   6. counterfactual.Expand runs only when ShouldExpand says the
//      request is complex, multi-option-ambiguous, and explain/root-
//      cause/security. Default policy keeps it off until batch B9.
//   7. gate.Run reports quality-gate findings. A rejected gate does
//      NOT block the run in B5 — it is logged and surfaced on the IR
//      so downstream analytics can diff failures. B5b will wire this
//      to the retry budget once we have eval coverage.
//
// The downstream consumers (explorer, finalizer, ERM, orchestrator)
// continue to read the legacy TaskItem fields in B5. Batch B5b
// switches them to the IR and deletes the legacy fields.
type analyzerEvaluator struct{}

func (e *analyzerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	_ = ctx
	_ = sk
	// The analyzer is the first LLM in the pipeline to see the raw
	// user request, so it has strictly more context than any downstream
	// post-hoc extractor. Every field it writes via todo_write feeds a
	// deterministic consumer:
	//
	//   keywords   -> keywordSearch / grep IDF ranking
	//   entities   -> ERM entity set (explorer.go prefers this over regex)
	//   question_kind -> ERM predicate whitelist (skips keyword inference)
	//   answer_shape -> finalizer anti-hallucination constraint
	//
	// Vague output here (generic keywords, missing entities, wrong
	// kind) cascades into wasted breadth scans, spurious ERM requirements,
	// and hallucinated final answers. The prompt is therefore written
	// as an explicit contract.
	return `You decompose the user request into a working task list by calling todo_write.
Your output feeds a deterministic evidence pipeline — downstream stages rely
on the STRUCTURED fields below, not your free-form explanation.

# Required fields on every task

title        — short imperative label in the user's language
writing      — true only if the task may mutate files; leave false for
               any read/explain/audit request
high_risk    — true only when writing=true AND the change needs review
complexity   — "simple" (single lookup/count), "moderate" (single-component
               explanation, the default), "complex" (cross-component flow)
keywords     — ≥8 search terms for grep. MUST include every CamelCase and
               snake_case identifier the user mentioned, PLUS conceptual
               synonyms. For Chinese questions, include BOTH Chinese and
               English forms (the codebase is English).
entities     — CamelCase/snake_case symbol names copied VERBATIM from the
               user's wording. Do NOT translate, pluralise, re-case, or
               paraphrase. Leave empty only if the question has no
               identifier-looking tokens. Generic nouns (count, function,
               thing, agent, handler, module) MUST NOT appear here — they
               poison ERM ranking.
question_kind — pick one:
   registration  — "which/how many X register/bind Y", "X 是在哪注册的"
   mechanism     — "how does X work", "explain the process of X", "X 怎么实现"
   return_value  — "what does X return", "X 的 Name() 是什么"
   conditional   — "when does X fire", "under what condition", "什么时候"
   config_mapping — "what does config key K control", key → behaviour
   enumeration   — "list all X", "count of X", pure set membership
   call_chain    — "which X calls Y", "从 A 到 B 怎么调用的"
   unknown       — genuinely ambiguous; ERM will fall back to keyword inference
answer_shape — pick one:
   list_of_symbols — answer is a set of identifier names (triggers
                     finalizer's anti-hallucination: forbids symbols
                     not present in Ground Truth evidence)
   step_list       — ordered steps of a mechanism
   value           — a single literal / returned value
   boolean         — yes/no
   config_value    — a resolved config key value
   none            — no structured shape applies

# Hard rules

1. entities come from the user's ORIGINAL text only. If the user wrote
   "ContinuationPrompt", put "ContinuationPrompt" — NOT "continuation
   prompt", NOT "continuation_prompt", NOT "prompt".
2. Do not invent a question_kind by stretching one of the categories.
   If two fit equally, choose the one that directly matches the
   user's verb. If none fit, use "unknown".
3. answer_shape=list_of_symbols ONLY when the user is asking for a
   SET of names they want to see listed. "How many agents call X" is
   list_of_symbols (they want the names even if phrased as a count).
   "Is X registered" is boolean. "Explain X" is step_list or none.
4. After calling todo_write, briefly explain your classification choices
   (question_kind and answer_shape) in one paragraph. This text is
   captured for the trace but does not drive any agent — the structured
   fields are what matter.`
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

	// If the LLM never called todo_write, fall back to a single-item
	// analysis-typed task so the orchestrator still has something to
	// route on. The fallback uses the user's original request as the
	// task title and writes through Mutable so we go through the same
	// channel a tool call would.
	if ctx.Mutable != nil {
		if tl := ctx.Mutable.TaskList(); len(tl.Tasks) == 0 {
			// Fail-safe default: the user request as a single
			// non-writing, non-risky task. Picks the analysis policy
			// so the pipeline answers the user instead of guessing
			// code changes.
			tl.Tasks = []types.TaskItem{{
				ID:     "task-1",
				Title:  tl.Objective,
				Status: types.TaskPending,
			}}
			tl.CurrentTaskID = "task-1"
			ctx.Mutable.SetTaskList(tl)
		}
	}

	// Build the Analyzer v3 IR deterministically from the current
	// TaskItem + raw objective. Every sub-component runs even when
	// parts of the TaskItem are empty — the compiler's generic
	// template + a zero risk matrix + a single baseline hypothesis
	// are still a valid starting point for downstream consumers.
	ir := buildAnalysisIR(ctx)

	// Assemble a structured StageOutput.Data so the trace records the
	// analyzer's classification machine-readably (in addition to the
	// natural-language rationale). Downstream consumers and post-run
	// audits can compare the analyzer's declared question_kind against
	// ERM's eventual inference without grepping free text.
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
		if !ir.QualityGate.Passed {
			// Log gate failures at debug so B5 eval can diff them
			// without the trace getting noisy on the happy path.
			logging.Debug("[analyzer-v3] gate rejected: retryable=%v checks=%+v",
				ir.QualityGate.Retryable, ir.QualityGate.Checks)
		}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		// Marshal of a map[string]any with string/int values can only
		// fail on pathological content; fall back to the legacy shape.
		raw = json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent))
	}

	return &StageOutput{
		Data:       raw,
		AnalysisIR: ir,
	}, nil
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

// buildAnalysisIR composes a full AnalysisIR by running the deterministic
// pipeline (normalizer → compiler → risk → hdp → counterfactual → gate)
// over a RequestModel synthesized from the classification carrier
// (populated by todo_write during the analyze ReAct loop) and the raw
// user objective. It is tolerant of missing fields: every sub-step has
// a sensible zero-value fallback.
//
// Pre-B5b-β this function read its hints from the current TaskItem
// (ctx.Mutable.TaskList().CurrentTask()); B5b-β moved the carrier off
// TaskItem onto MutableState.Classification() so the legacy fields
// could be deleted.
func buildAnalysisIR(ctx *types.AgentContext) *types.AnalysisIR {
	if ctx == nil {
		return nil
	}
	var hints types.AnalyzerClassification
	if ctx.Mutable != nil {
		hints = ctx.Mutable.Classification()
	}

	// 1. RequestModel from classification + Objective.
	rm := buildRequestModel(ctx, hints)

	// 2. Scenario compilation.
	out := compiler.Compile(rm)

	// 3. AnswerShape override from the LLM classification when set —
	// the LLM's explicit shape choice should trump the scenario
	// template's default shape (e.g. the analyzer decided a particular
	// mechanism question is list_of_symbols rather than step_list).
	if hints.AnswerShape != "" {
		if shape := mapAnswerShape(hints.AnswerShape); shape != "" {
			out.AnswerContract.RequiredAnswerShape = shape
		}
	}

	// 4. Risk matrix + run policy.
	rm.RiskMatrix = risk.Evaluate(rm, rm.RiskMatrix)
	runPolicy := risk.DerivePolicy(rm.RiskMatrix, hints.Writing)
	// HighRisk from the LLM classification is an OR of all the review
	// switches — honor it so analyzer-declared risk always escalates.
	if hints.HighRisk {
		runPolicy.RequireDesignReview = true
		runPolicy.RequireCodeReview = true
	}

	// 5. Hypothesis planning and binding.
	hypotheses := hdp.Plan(rm)
	hdp.Bind(&out.TaskGraph, hypotheses)

	// 6. Optional counterfactual expansion.
	if counterfactual.ShouldExpand(rm) {
		expanded, newIDs := counterfactual.Expand(
			out.TaskGraph, rm,
			counterfactual.Options{Enabled: true, MaxBranches: 1},
		)
		out.TaskGraph = expanded
		if len(newIDs) > 0 {
			// Bind hypotheses to any new counterfactual nodes the
			// expander added, so the quality gate's coverage check
			// still passes.
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

	// 7. Quality gate — reported, not enforced in B5.
	ir.QualityGate = gate.Run(ir, gate.Thresholds{})

	return ir
}

// buildRequestModel turns the raw objective + LLM classification
// carrier into a RequestModel. The normalizer produces the canonical
// TermGraph from the raw text (always), and the carrier-supplied
// intent/complexity classification is honored when present.
func buildRequestModel(ctx *types.AgentContext, hints types.AnalyzerClassification) types.RequestModel {
	raw := ctx.Objective
	// Normalize the raw objective to populate the term graph. We run
	// this unconditionally — it's cheap and deterministic.
	termGraph := normalizer.Normalize(raw, normalizer.Options{})

	rm := types.RequestModel{
		RawRequest: raw,
		Language:   detectLanguage(raw, ctx.Preferences),
		TermGraph:  termGraph,
		Complexity: types.ComplexityModerate,
	}

	if hints.Complexity != "" {
		if c := mapComplexity(hints.Complexity); c != "" {
			rm.Complexity = c
		}
	}
	if hints.QuestionKind != "" {
		rm.Intent = mapIntent(hints.QuestionKind)
	} else {
		rm.Intent = types.IntentUnknown
	}
	rm.AnalyzerHints = types.AnalyzerHints{
		Keywords: append([]string(nil), hints.Keywords...),
		Entities: append([]string(nil), hints.Entities...),
		Kind:     hints.QuestionKind,
		Shape:    hints.AnswerShape,
	}

	// Scenario is derived from Intent + TermGraph. The compiler's
	// templates already honor ScenarioGeneric as the fallback so
	// InferScenario's output is always safe.
	rm.Scenario = compiler.InferScenario(rm)

	return rm
}

// mapIntent translates the legacy question_kind enum (analyzer prompt
// contract) into the v3 Intent enum. Unknown maps to IntentUnknown
// rather than an empty string so compiler.InferScenario gets a
// consistent shape.
func mapIntent(qk string) types.Intent {
	switch strings.ToLower(strings.TrimSpace(qk)) {
	case "registration":
		return types.IntentEnumerate // "X registers Y" is a set question
	case "mechanism":
		return types.IntentExplain
	case "return_value":
		return types.IntentReturnValue
	case "conditional":
		return types.IntentTrace
	case "config_mapping":
		return types.IntentConfigQuery
	case "enumeration":
		return types.IntentEnumerate
	case "call_chain":
		return types.IntentTrace
	}
	return types.IntentUnknown
}

// mapComplexity coerces the legacy complexity string into the typed
// Complexity enum. Empty or unrecognized → "" so the caller can apply
// its own default.
func mapComplexity(s string) types.Complexity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "simple":
		return types.ComplexitySimple
	case "moderate", "":
		return types.ComplexityModerate
	case "complex":
		return types.ComplexityComplex
	}
	return ""
}

// mapAnswerShape coerces the legacy answer_shape string into the typed
// AnswerShape enum. Unknown values return "" so the compiler's default
// stays in place.
func mapAnswerShape(s string) types.AnswerShape {
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
	case "none":
		return types.ShapeNone
	}
	return ""
}

// detectLanguage picks "zh" when the raw request contains Han
// characters, "en" otherwise. Falls back to Preferences-declared
// language when the request is entirely non-alphabetic (edge case).
func detectLanguage(raw string, prefs []string) string {
	for _, r := range raw {
		if unicode.Is(unicode.Han, r) {
			return "zh"
		}
	}
	// If the raw request is Latin-only, honor a preference override
	// so -lang=zh still produces an IR with zh language tag.
	for _, p := range prefs {
		lp := strings.ToLower(p)
		if strings.Contains(lp, "chinese") || strings.Contains(lp, "zh") || strings.Contains(lp, "简体中文") {
			return "zh"
		}
	}
	return "en"
}
