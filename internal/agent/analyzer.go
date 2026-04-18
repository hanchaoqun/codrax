package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/analysis/binder"
	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/analysis/counterfactual"
	"github.com/hanchaoqun/codrax/internal/analysis/findings_validator"
	"github.com/hanchaoqun/codrax/internal/analysis/gate"
	"github.com/hanchaoqun/codrax/internal/analysis/hdp"
	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/analysis/risk"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzerEvaluator drives the Analyzer v3 analyze stage. The agent
// makes a single LLM call whose only job is to emit a RequestModel
// through the emit_analysis tool; everything else about AnalysisIR
// (TermGraph, TaskGraph, EvidencePlan, AnswerContract, Hypotheses,
// QualityGate) is derived deterministically after the ReAct loop exits.
//
// Fail-loud contract: when the LLM fails to call emit_analysis at
// all, or when the deterministic pipeline cannot build a valid IR,
// ParseOutput returns a StageOutput with a populated Error and a nil
// AnalysisIR. The orchestrator's runAnalyzePhase loop retries up to
// MaxRetriesPerStage; after the budget is exhausted the whole Run
// terminates without entering the task phase.
type analyzerEvaluator struct {
	// prescanRounds counts PhaseMidLoop observations whose
	// LastToolResult was a pre-scan navigation tool. Reset at the
	// start of each dispatch.
	prescanRounds int

	// prescanBudgetOverride, when > 0, replaces
	// tool.CurrentAnalysisLimits().MaxPrescanRounds for this dispatch.
	// Set in BuildInitialInstruction when the question text heuristically
	// looks like a multi-topic request (multiple question marks or
	// numbered sub-questions).
	prescanBudgetOverride int

	// agentSettings caches the resolved AgentSettings so the
	// heuristic prescan scaling can read SubTopicPrescanBudgetExtra.
	agentSettings types.AgentSettings
}

func (e *analyzerEvaluator) BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string {
	_ = sk
	e.prescanRounds = 0
	e.prescanBudgetOverride = 0
	if ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.ResetPrescanSummary()
	}

	// Heuristic multi-topic detection: count question marks and
	// numbered list patterns to estimate the number of sub-topics
	// BEFORE emit_analysis is called. This allows prescan budget
	// scaling without waiting for the LLM's SubTopics field.
	if ctx != nil && ctx.Objective != "" {
		estimated := estimateSubTopicCount(ctx.Objective)
		if estimated > 1 {
			base := tool.CurrentAnalysisLimits().MaxPrescanRounds
			if base > 0 {
				extra := (estimated / 2) * e.agentSettings.SubTopicPrescanBudgetExtra
				adjusted := base + extra
				// Cap at base + 2 (max 4 total).
				cap := base + 2
				if cap > 4 {
					cap = 4
				}
				if adjusted > cap {
					adjusted = cap
				}
				if adjusted > base {
					e.prescanBudgetOverride = adjusted
					logging.Debug("[analyzer] multi-topic heuristic: estimated %d sub-topics, prescan budget %d → %d",
						estimated, base, adjusted)
				}
			}
		}
	}

	// Pre-inject a repo_map task_map view so the analyzer starts its
	// first iteration with structural context already visible. Without
	// this, the LLM always burns its first round calling repo_map itself
	// — a predictable, wasted LLM round-trip (~3s). The task_map view
	// uses the user question as the query, so the result is already
	// relevance-filtered. The LLM can still call repo_map/grep/list_files
	// in round 1-2 for additional verification, but now it has a head
	// start.
	if ctx != nil && ctx.RepoRoot != "" && ctx.Objective != "" {
		// In REPL mode, strip the conversation prefix so the query
		// only contains the current question, not prior-turn memory.
		objective := types.StripConversationPrefix(ctx.Objective)
		if overview := buildAnalyzerRepoOverview(ctx.RepoRoot, objective); overview != "" {
			return overview
		}
	}
	return ""
}

// buildAnalyzerRepoOverview builds a repo overview for the analyzer's
// initial prompt. Strategy:
//
//  1. Extract CamelCase/snake_case entities from the user question.
//  2. If entities found → build graph with entity query, render task_map
//     (query-relevant files and symbols).
//  3. If no entities → render overview (general repo structure).
//
// Returns empty string on any error — the analyzer proceeds without
// it and the LLM calls repo_map itself (graceful degrade).
func buildAnalyzerRepoOverview(repoRoot, objective string) string {
	// Extract code identifiers from the question to use as the graph
	// query. extractQuestionEntities pulls CamelCase/snake_case tokens
	// — exactly the kind of tokens that match file and symbol names.
	entities := extractQuestionEntities(objective)

	query := strings.Join(entities, " ")
	graph, err := repomap.BuildOrLoadGraph(repoRoot, query)
	if err != nil {
		logging.Debug("[analyzer] repo overview unavailable: %v", err)
		return ""
	}

	var view, output, header string
	if len(entities) > 0 {
		// Query-directed: show files/symbols relevant to the entities.
		view = "task_map"
		output = repomap.GenerateView(graph, view, repomap.ViewParams{
			Query: query,
			TopN:  15,
		})
		header = fmt.Sprintf("## Repository overview (pre-computed for entities: %s)\n\n"+
			"The following task_map shows files and symbols matching the entities from the user's question. "+
			"Use this to inform your entity/keyword choices and pre-scan targets. "+
			"You may still call repo_map, grep, or list_files for additional verification.\n\n",
			strings.Join(entities, ", "))
	} else {
		// No entities extracted — fall back to general overview.
		view = "overview"
		output = repomap.GenerateView(graph, view, repomap.ViewParams{})
		header = "## Repository overview (pre-computed, no tool call needed)\n\n" +
			"The following overview shows the repository structure. " +
			"Use this to orient your entity/keyword choices and pre-scan targets. " +
			"You may still call repo_map, grep, or list_files for additional verification.\n\n"
	}

	if output == "" {
		return ""
	}
	// Cap the overview to keep the initial prompt bounded.
	const maxLen = 4096
	if len(output) > maxLen {
		output = output[:maxLen] + "\n... [truncated]\n"
	}
	logging.Debug("[analyzer] pre-injected %s view (%d bytes, entities=%v)", view, len(output), entities)
	return header + output
}

func (e *analyzerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	// Never hard-stop from ShouldStop. The analyzer relies on:
	// - Observe(PhaseMidLoop) to stop after emit_analysis succeeds
	// - Observe(PhaseMidLoop) to stop after prescan budget exhaustion
	// - MaxIterations as the outer ceiling
	// The old "stop when no tool calls" behavior broke when Think
	// Aloud was added: the LLM writes a plan statement (content-only,
	// no tools) and then would hit the soft-stop path at iter=0,
	// terminating before any tool call. Returning false unconditionally
	// lets the soft-stop path in BaseAgent.Execute handle content-only
	// turns — which will continue the loop since the analyzer does
	// not implement LoopController.PhaseSoftStop.
	return false
}

// Observe enforces the pre-scan budget at runtime. Once the counter
// strictly exceeds AnalysisLimits.MaxPrescanRounds, the next pre-scan
// triggers a force-stop. After the force-stop ParseOutput will see
// Mutable.RequestModel()==nil if the LLM never called emit_analysis,
// and will emit a hard StageOutput.Error — no silent synthesis.
func (e *analyzerEvaluator) Observe(ctx *types.AgentContext, obs LoopObservation) LoopSignal {
	// PhaseSoftStop: when the LLM produces content-only (e.g. Think
	// Aloud plan statement) without calling any tool, inject a
	// continuation hint so the loop doesn't terminate prematurely.
	// The analyzer's contract requires emit_analysis — stopping
	// before that is always wrong.
	if obs.Phase == PhaseSoftStop {
		// Use a unique key per iteration so LoopPolicy dedup doesn't
		// swallow the second continuation — the LLM tends to produce
		// multiple content-only turns before finally issuing tool calls.
		return LoopSignal{
			HintRequested: true,
			HintKey:       fmt.Sprintf("analyzer.continue.%d", obs.Iteration),
			Hint:          "You wrote text but did not call any tool. Do NOT write more analysis — call emit_analysis NOW with the fields you have. If you need to verify entities first, call grep(files_only=true) in the SAME response as your reasoning, not in a separate turn.",
		}
	}
	if obs.Phase != PhaseMidLoop {
		return LoopSignal{}
	}
	if obs.LastToolResult == nil {
		return LoopSignal{}
	}
	// emit_analysis is the analyzer's terminal action — once it fires
	// successfully, there is nothing left for the ReAct loop to do.
	// Stop immediately instead of burning one extra LLM round.
	// When the call FAILED (parameter validation error), do NOT stop:
	// let the LLM see the error message and retry within the same
	// dispatch, which is cheaper than a full runAnalyzePhase retry.
	if obs.LastToolResult.ToolName == "emit_analysis" && obs.LastToolResult.Success {
		return LoopSignal{StopRequested: true, StopReason: "emit_analysis called"}
	}
	if !isPrescanTool(obs.LastToolResult.ToolName) {
		return LoopSignal{}
	}
	e.prescanRounds++

	if ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.AppendPrescanSummary(obs.LastToolResult.Summary)
	}

	max := e.prescanBudgetOverride
	if max <= 0 {
		max = tool.CurrentAnalysisLimits().MaxPrescanRounds
	}
	if max <= 0 || e.prescanRounds <= max {
		return LoopSignal{}
	}
	reason := fmt.Sprintf(
		"pre-scan budget exhausted (%d rounds > max %d); "+
			"analyze stage will fail loud if emit_analysis was not called",
		e.prescanRounds, max)
	logging.Warning("[analyzer] %s", reason)
	return LoopSignal{StopRequested: true, StopReason: reason}
}

func (e *analyzerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	emitCalls := countEmitAnalysisCalls(toolResults)
	limits := tool.CurrentAnalysisLimits()
	prescanRounds := e.prescanRounds
	prescanBudgetExhausted := limits.MaxPrescanRounds > 0 && prescanRounds > limits.MaxPrescanRounds

	data := map[string]any{
		"result":                            lastContent,
		"analysis_emit_calls":               emitCalls,
		"analysis_prescan_rounds":           prescanRounds,
		"analysis_prescan_budget_exhausted": prescanBudgetExhausted,
	}

	// Hard fail on 0 emit_analysis calls. The orchestrator's
	// runAnalyzePhase owns retry — this function just surfaces a
	// loud Error.
	if emitCalls == 0 {
		raw, _ := json.Marshal(data)
		return &StageOutput{
			Data:  raw,
			Error: "analyzer: emit_analysis was not called during the analyze dispatch",
		}, nil
	}

	// N > 1: last write wins. Reject policy adds a loud Error at
	// the end but still builds the IR so downstream stages could
	// continue if the operator ignores the signal.
	var emitGateError string
	if emitCalls > 1 {
		if limits.RejectMultipleEmit {
			emitGateError = fmt.Sprintf("analyzer emit_analysis called %d times (policy=reject)", emitCalls)
			logging.Error("[analyzer] %s", emitGateError)
		} else {
			logging.Warning("[analyzer] emit_analysis called %d times; only last write effective", emitCalls)
		}
	}

	ir, buildErr := buildAnalysisIR(ctx)
	if buildErr != nil {
		raw, _ := json.Marshal(data)
		return &StageOutput{
			Data:  raw,
			Error: fmt.Sprintf("analyzer build failure: %v", buildErr),
		}, nil
	}

	// Compute quality probe post-hoc for diagnostics.
	var probeKeywords, probeEntities []string
	if ir != nil {
		probeKeywords = ir.RequestModel.AnalyzerHints.Keywords
		probeEntities = ir.RequestModel.AnalyzerHints.Entities
	}
	qualityProbe := computeQualityProbeFromContext(ctx, prescanRounds, probeKeywords, probeEntities)

	data["analysis_quality_probe"] = qualityProbeToMap(qualityProbe)
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
	out := &StageOutput{Data: raw, AnalysisIR: ir}

	// CGEC P4: validate the analyzer's free-form narrative for
	// hallucinated paths / symbols before it travels downstream as
	// the "Prior Stage Findings" section. Misses get strikethrough +
	// "未验证" / "unverified" annotations so the extractor and
	// finalizer cannot bake hallucinated artefacts into the answer.
	// Verified findings record into MutableState.EvidenceClosure
	// for cross-stage observability.
	if lastContent != "" {
		var graph *repomap.Graph
		if ctx != nil && ctx.Mutable != nil {
			if g, ok := ctx.Mutable.SearchGraph().(*repomap.Graph); ok {
				graph = g
			}
		}
		var repoRoot string
		if ctx != nil {
			repoRoot = ctx.RepoRoot
		}
		validation := findings_validator.Validate(lastContent, repoRoot, graph)
		out.StageReport = validation.Annotated
		if len(validation.Unverified) > 0 && ctx != nil && ctx.Mutable != nil {
			closure := ctx.Mutable.EvidenceClosure()
			var symbolTokens []string
			for _, u := range validation.Unverified {
				closure.AppendUnverifiedFinding(u)
				if u.Kind == "symbol" {
					symbolTokens = append(symbolTokens, u.Token)
				}
			}
			closure.BumpUnverifiedFinds(len(validation.Unverified))
			logging.Warning("[CGEC] I1 findings_validator: unverified_tokens=%d (paths + symbols flagged)", len(validation.Unverified))
			// CGEC C3: the analyzer mentioned entities the validator
			// could not verify against the repo. Emit RepairExpandSearch
			// with the symbol tokens so the next explore round's prompt
			// tells the LLM to grep the real repo for these (or confirm
			// they genuinely don't exist). Path-kind unverified tokens
			// are NOT passed as keywords because path strings aren't
			// grep-able identifiers; they're already rendered as
			// strikethrough in the Annotated StageReport which flows
			// into Prior Stage Findings. Only fires when the analyzer
			// actually named a symbol we couldn't pin down.
			if len(symbolTokens) > 0 {
				closure.AddRepair(types.RepairDirective{
					Kind:      types.RepairExpandSearch,
					Keywords:  symbolTokens,
					Rationale: fmt.Sprintf("analyzer referenced %d symbol(s) that findings_validator could not verify against the repo graph — grep these to confirm existence / disprove before acting on them", len(symbolTokens)),
					Origin:    "findings_validator.unverified_symbols",
				})
				logging.Info("[CGEC] C3 expand_search: origin=findings_validator.unverified_symbols symbols=%d", len(symbolTokens))
			}
		}
	}

	// Quality gate enforcement. All checks except
	// pending_fields_wellformed are classified hard — any hard
	// failure yields an Error that makes runAnalyzePhase retry.
	if ir.QualityGate.Rejected {
		if hard, detail := classifyGateFailure(ir.QualityGate); hard {
			logging.Error("[analyzer-v3] quality gate HARD failure: %s", detail)
			out.Error = fmt.Sprintf("analyzer quality gate rejected: %s", detail)
			return out, nil
		}
		logging.Warning("[analyzer-v3] quality gate soft warning: %+v", ir.QualityGate.Checks)
	}
	// Emit-gate error surfaces after gate so a gate hard-fail still
	// wins when both triggered, but the repeat-call Error survives
	// when the gate passed.
	if emitGateError != "" && out.Error == "" {
		out.Error = emitGateError
	}
	return out, nil
}

// countEmitAnalysisCalls walks the tool-result stream.
func countEmitAnalysisCalls(toolResults []types.ToolResult) int {
	n := 0
	for _, r := range toolResults {
		if r.ToolName == "emit_analysis" {
			n++
		}
	}
	return n
}

// isPrescanTool returns true when name identifies one of the
// analyzer's three evidence-lite pre-scan navigation tools.
func isPrescanTool(name string) bool {
	switch name {
	case "repo_map", "grep", "list_files":
		return true
	}
	return false
}

func (e *analyzerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, _ *StageOutput) types.MissingPiece {
	_ = ctx
	return types.MissingFacts
}

// estimateSubTopicCount heuristically detects the number of
// independent sub-topics in a question string by counting question
// marks (Chinese and ASCII) and numbered list patterns (e.g. "1. ",
// "2) "). Returns at least 1.
func estimateSubTopicCount(text string) int {
	// Count question marks (? and ？).
	qmarks := 0
	for _, r := range text {
		if r == '?' || r == '？' {
			qmarks++
		}
	}

	// Count numbered list items (e.g. "1. ", "2) ", "3、").
	numbered := 0
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 3 {
			continue
		}
		// Check for patterns like "1. ", "2) ", "3、", "1) "
		if trimmed[0] >= '1' && trimmed[0] <= '9' {
			rest := trimmed[1:]
			if len(rest) > 0 && (rest[0] == '.' || rest[0] == ')') {
				numbered++
			} else if strings.HasPrefix(rest, "、") {
				numbered++
			}
		}
	}

	// Take the larger signal.
	count := qmarks
	if numbered > count {
		count = numbered
	}
	if count < 1 {
		count = 1
	}
	return count
}

// NewAnalyzerAgent creates the analyzer agent.
func NewAnalyzerAgent(deps *Dependencies) Agent {
	eval := &analyzerEvaluator{
		agentSettings: deps.AgentSettings,
	}
	// Override LoopPolicy for the analyzer: MinInjectInterval=1 so
	// soft-stop continuation hints are not throttled. The analyzer
	// runs 3-5 iterations total; the default interval of 3 means the
	// second content-only turn (iter=2) gets throttled and the loop
	// terminates before emit_analysis is called.
	d := *deps
	d.LoopPolicy.MinInjectInterval = 1
	return NewBaseAgent(types.AgentAnalyzer, &d, eval)
}

// ── Analyzer v3 IR builder ───────────────────────────────────────────

// buildAnalysisIR composes a full AnalysisIR from the emit_analysis-
// produced RequestModel. Fail-loud: returns (nil, error) when the
// LLM never called emit_analysis.
func buildAnalysisIR(ctx *types.AgentContext) (*types.AnalysisIR, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, errors.New("analyzer: missing AgentContext.Mutable")
	}
	raw := ctx.Mutable.RequestModel()
	if raw == nil {
		return nil, errors.New("analyzer: emit_analysis was not called")
	}
	rm := *raw
	if rm.RawRequest == "" {
		// Fallback path: strip the REPL conversation prefix so normalizer.Normalize
		// never sees prior-turn memory. emit_analysis already strips at its write
		// site; this guards the rare case where an alternate code path constructs
		// a RequestModel without going through the tool.
		rm.RawRequest = types.StripConversationPrefix(ctx.Objective)
	}
	if rm.Language == "" {
		rm.Language = detectLanguage(rm.RawRequest, ctx.Preferences)
	}

	// Sub-topics post-processing: when the LLM detected multiple
	// independent sub-topics, force explanation shape and merge entities.
	if len(rm.SubTopics) > 5 {
		rm.SubTopics = rm.SubTopics[:5]
		logging.Warning("[analyzer] sub_topics truncated to 5")
	}
	if len(rm.SubTopics) > 1 {
		if rm.AnalyzerHints.Shape != string(types.ShapeExplanation) {
			logging.Warning("[analyzer] sub_topics detected (%d), forcing answer_shape explanation (was %s)",
				len(rm.SubTopics), rm.AnalyzerHints.Shape)
			rm.AnalyzerHints.Shape = string(types.ShapeExplanation)
		}
		if rm.Complexity == types.ComplexitySimple {
			logging.Debug("[analyzer] sub_topics detected, upgrading complexity simple → moderate")
			rm.Complexity = types.ComplexityModerate
		}
	}

	// Set to true when the request is a measurement-scalar question
	// (leading count-verb cue + simple complexity + intent in the
	// enumerate/return_value pair). The answer to such a question is a
	// scalar produced by a tool query (find | wc -l, list_files count,
	// grep count) with no file:line to cite — declared outside the
	// sanity block so the post-compile AnswerContract mutation below
	// can read it. Fires regardless of whether reconcileIntent had to
	// downgrade enumerate→return_value (LLM got it wrong) or the LLM
	// picked return_value directly (LLM got it right) — both
	// populations need the three-citation-gate carve-out.
	isMeasurementScalar := false

	// Post-process complexity sanity check. The sub_topics rule above
	// is one input; reconcileComplexity additionally cross-checks the
	// LLM's pick against entity/keyword counts and question-shape
	// cues in the raw request. Rules fire only on strong signals so
	// the LLM's choice stays the default; see
	// internal/agent/analyzer_complexity.go for the rule catalogue.
	{
		// Raw request stripped of the REPL conversation prefix — otherwise
		// "## Current request\nrepomap的作用" would show simple-lookup
		// cues even for a complex question re-asked in REPL mode.
		rawForComplexity := types.StripConversationPrefix(rm.RawRequest)
		resolved, reason := reconcileComplexity(rm.Complexity, rawForComplexity,
			rm.AnalyzerHints.Entities, rm.AnalyzerHints.Keywords, len(rm.SubTopics))
		if resolved != rm.Complexity {
			logComplexityReconcile(rm.Complexity, resolved, reason)
			rm.Complexity = resolved
		}
		// Intent sanity rule. Runs AFTER reconcileComplexity so the
		// simple-only gate reads the post-reconcile complexity. Patches
		// the "count / 统计 / how many" → enumerate mis-classification
		// that otherwise locks the pipeline into ShapeListOfSymbols
		// and an unsatisfiable file:line citation floor.
		// See internal/agent/analyzer_intent.go for the rule body.
		if intentResolved, intentReason := reconcileIntent(rm.Intent, rawForComplexity, rm.Complexity); intentResolved != rm.Intent {
			logIntentReconcile(rm.Intent, intentResolved, intentReason)
			rm.Intent = intentResolved
		}
		// Broader measurement-scalar signal — captures both the
		// reconciled-downgrade case above AND the case where the LLM
		// picked IntentReturnValue directly on the same count-verb
		// prefix. Every consequence (shape override, 3 citation-gate
		// strips) is applied in one post-compile block below, keyed
		// off this single flag. Keeps "one signal, one response"
		// grep-able.
		isMeasurementScalar = isMeasurementScalarRequest(rm, rawForComplexity)

		// CGEC: AnswerSubject inference. Classifies what kind of
		// source-code literal the answer should be (skill_name,
		// agent_name, config_key, ...). Honours an LLM-supplied
		// AnswerSubject when present; otherwise applies the cue table
		// in analyzer_intent.go::inferAnswerSubject. The chain ranker
		// uses the resolved subject to demote chains whose terminal
		// is the wrong kind, and reconcileShape (post-compile)
		// consults it to swap config_value→value for source-code
		// literals that have no YAML key surface.
		if subject, reason := inferAnswerSubject(rm, rawForComplexity); subject.Kind != types.SubjectUnknown {
			logSubjectInferred(subject, reason)
			rm.AnswerSubject = subject
		}
		// Merge sub-topic entities into main entity list.
		seen := make(map[string]bool, len(rm.AnalyzerHints.Entities))
		for _, e := range rm.AnalyzerHints.Entities {
			seen[e] = true
		}
		for _, st := range rm.SubTopics {
			for _, e := range st.Entities {
				if !seen[e] {
					rm.AnalyzerHints.Entities = append(rm.AnalyzerHints.Entities, e)
					seen[e] = true
				}
			}
		}
		logging.Info("[analyzer] multi-topic: %d sub-topics, merged entities=%v",
			len(rm.SubTopics), rm.AnalyzerHints.Entities)
	}

	// Normalizer runs unconditionally on the raw objective.
	rm.TermGraph = normalizer.Normalize(rm.RawRequest, normalizer.Options{})

	// Scenario default.
	if rm.Scenario == "" || rm.Scenario == types.ScenarioGeneric {
		rm.Scenario = compiler.InferScenario(rm)
	}

	// First-pass compile with an approximate budget signal
	// (hypothesis count unknown yet). Budget gets recomputed after
	// hdp.Plan below.
	sig := budget.BudgetSignals{
		Complexity:      rm.Complexity,
		TermCount:       len(rm.TermGraph.Canonical),
		HypothesisCount: 1,
		PrescanHitRatio: prescanHitRatio(ctx, rm.AnalyzerHints),
	}
	out := compiler.Compile(rm, sig)
	if rm.AnalyzerHints.Shape != "" {
		if shape := mapLegacyAnswerShape(rm.AnalyzerHints.Shape); shape != "" {
			out.AnswerContract.RequiredAnswerShape = shape
		}
	}
	if out.AnswerContract.Language == "" {
		out.AnswerContract.Language = rm.Language
	}

	// Risk matrix and hypothesis planning.
	rm.RiskMatrix = risk.Evaluate(rm, rm.RiskMatrix)
	hypotheses := hdp.Plan(rm)

	// Recompute budget with the real hypothesis count.
	sig.HypothesisCount = len(hypotheses)
	compiler.RecomputeBudget(&out, rm, sig)

	// Relevance-based hypothesis binding.
	if err := binder.BindByRelevance(&out.TaskGraph, hypotheses, binder.Options{}); err != nil {
		return nil, fmt.Errorf("binder: %w", err)
	}

	// Optional counterfactual expansion.
	if counterfactual.ShouldExpand(rm) {
		expanded, newIDs := counterfactual.Expand(
			out.TaskGraph, rm,
			counterfactual.Options{Enabled: true, MaxBranches: 1},
		)
		out.TaskGraph = expanded
		if len(newIDs) > 0 {
			if err := binder.BindByRelevance(&out.TaskGraph, hypotheses, binder.Options{}); err != nil {
				return nil, fmt.Errorf("binder (counterfactual): %w", err)
			}
		}
	}

	// Measurement-scalar carve-out. A count question ("how many X",
	// "统计 …") produces a scalar answer from a tool query
	// (find | wc -l, list_files count, grep count) with no file:line
	// to cite. The isMeasurementScalar signal (computed by
	// isMeasurementScalarRequest in analyzer_intent.go) is a 0-error
	// discriminator: it fires on simple complexity + leading count-
	// verb prefix + intent in {enumerate, return_value}, which is
	// exactly the population that has no file:line to cite. A
	// "what does X return" question — same IntentReturnValue — never
	// fires because there is no count-verb prefix.
	//
	// The signal fires regardless of whether the LLM initially picked
	// enumerate (reconcileIntent downgraded it above) or return_value
	// (already correct). Both populations need the same carve-out —
	// keying off the downgrade event alone missed the "LLM got it
	// right on its own" case and looped the retry budget.
	//
	// Consequences in order:
	//
	//   (a) out.AnswerContract.RequiredAnswerShape → ShapeValue. Forces
	//       the final shape past both compile's intent-switch (already
	//       set to ShapeValue by IntentReturnValue) and the LLM-hint
	//       override at "if rm.AnalyzerHints.Shape != ..." above,
	//       whose stale list_of_symbols hint would otherwise restore
	//       the wrong shape.
	//   (b) rm.AnalyzerHints.Shape → ShapeValue (only when stale
	//       list_of_symbols), so downstream readers of the hint
	//       (ir_accessor.irAnswerShape, answer_document_evaluator)
	//       see the reconciled shape, not the LLM's original emit.
	//   (c-e) Strip the three citation gate surfaces that all consult
	//         CritCitationCountGE independently:
	//
	//         (c) AnswerContract.CitationReq           → contract.checkCitations
	//         (d) AnswerContract.AcceptanceTests       → contract.checkAcceptance
	//         (e) TaskNode.SuccessCriteria (finalize)  → orchestrator.markSuccessCriteriaFailed
	//
	//       Leaving any one enabled loops the retry budget on a
	//       mismatch no amount of re-investigation can fix.
	if isMeasurementScalar {
		out.AnswerContract.RequiredAnswerShape = types.ShapeValue
		if mapLegacyAnswerShape(rm.AnalyzerHints.Shape) == types.ShapeListOfSymbols {
			rm.AnalyzerHints.Shape = string(types.ShapeValue)
		}
		out.AnswerContract.CitationReq.Required = false
		out.AnswerContract.CitationReq.MinCitations = 0
		out.AnswerContract.AcceptanceTests = dropCitationCountGE(out.AnswerContract.AcceptanceTests)
		for i := range out.TaskGraph.Nodes {
			out.TaskGraph.Nodes[i].SuccessCriteria = dropCitationCountGE(out.TaskGraph.Nodes[i].SuccessCriteria)
		}
	}

	// CGEC: shape reconcile. After the measurement-scalar carve-out
	// has had its say, swap ShapeConfigValue → ShapeValue for answer
	// subjects whose literal lives in source code rather than a YAML
	// config (skill names, agent names, function names, type names,
	// handler routes, return values). This catches the bug where the
	// LLM picked config_value for a Go map-literal answer; the
	// finalizer would otherwise be forced to invent a fake key like
	// "explorerAgent.defaultSkill" to satisfy the schema, which the
	// contract checker then rejects.
	//
	// Runs AFTER measurement-scalar so the measurement carve-out
	// (which sets ShapeValue + strips citation gates) is not undone
	// by reconcile picking a different shape; the two rules are
	// disjoint in practice (measurement requires count-verb prefix;
	// reconcile requires source-code-literal subject).
	if reconciled, reason := reconcileShape(out.AnswerContract.RequiredAnswerShape, rm.AnswerSubject); reconciled != out.AnswerContract.RequiredAnswerShape {
		logShapeReconciled(out.AnswerContract.RequiredAnswerShape, reconciled, reason)
		out.AnswerContract.RequiredAnswerShape = reconciled
		// Also align the AnalyzerHints surface so downstream readers
		// (ir_accessor.irAnswerShape, answer_document_evaluator,
		// emit_answer_document shape auto-correct) see the
		// reconciled shape.
		if mapLegacyAnswerShape(rm.AnalyzerHints.Shape) == types.ShapeConfigValue {
			rm.AnalyzerHints.Shape = string(reconciled)
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
	ir.QualityGate = gate.Run(ir, gate.GlobalThresholds())
	return ir, nil
}

// dropCitationCountGE returns a copy of crits with every
// CritCitationCountGE entry removed. Used by the measurement-scalar
// carve-out above to strip the citation_count_ge gate from
// AcceptanceTests and TaskNode.SuccessCriteria in one pass. Returns a
// fresh slice (never the input's backing array) so callers never
// observe an unexpected shared-aliasing mutation.
func dropCitationCountGE(crits []types.Criterion) []types.Criterion {
	if len(crits) == 0 {
		return crits
	}
	out := make([]types.Criterion, 0, len(crits))
	for _, c := range crits {
		if c.Kind != types.CritCitationCountGE {
			out = append(out, c)
		}
	}
	return out
}

// prescanHitRatio pulls the quality probe keyword hit ratio out of
// MutableState if available. Default 1.0 when no probe ran (no
// penalty applied to budget scaling).
func prescanHitRatio(ctx *types.AgentContext, hints types.AnalyzerHints) float64 {
	if ctx == nil || ctx.Mutable == nil {
		return 1.0
	}
	probe := computeQualityProbeFromContext(ctx, 0, hints.Keywords, hints.Entities)
	if probe.KeywordTotal == 0 {
		return 1.0
	}
	return probe.KeywordHitRatio()
}

// classifyGateFailure inspects a GateReport and returns whether any
// failure is HARD. pending_fields_wellformed is the only SOFT check;
// every other failing check is a hard error.
func classifyGateFailure(report types.GateReport) (hard bool, detail string) {
	for _, c := range report.Checks {
		if c.Passed {
			continue
		}
		if c.Name == "pending_fields_wellformed" {
			continue
		}
		return true, fmt.Sprintf("%s: %s", c.Name, c.Detail)
	}
	return false, ""
}

// mapLegacyAnswerShape coerces a free-form answer_shape string into
// the typed AnswerShape enum.
func mapLegacyAnswerShape(s string) types.AnswerShape {
	switch types.AnswerShape(strings.ToLower(strings.TrimSpace(s))) {
	case types.ShapeListOfSymbols:
		return types.ShapeListOfSymbols
	case types.ShapeStepList:
		return types.ShapeStepList
	case types.ShapeValue:
		return types.ShapeValue
	case types.ShapeBoolean:
		return types.ShapeBoolean
	case types.ShapeConfigValue:
		return types.ShapeConfigValue
	case types.ShapeExplanation:
		return types.ShapeExplanation
	case types.ShapeNone:
		return types.ShapeNone
	}
	return ""
}

// detectLanguage picks "zh" when the raw request contains Han
// characters, "en" otherwise.
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
