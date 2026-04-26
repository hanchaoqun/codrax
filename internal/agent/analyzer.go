package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/analysis/binder"
	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/analysis/counterfactual"
	"github.com/hanchaoqun/codrax/internal/analysis/declarative"
	"github.com/hanchaoqun/codrax/internal/analysis/findings_validator"
	"github.com/hanchaoqun/codrax/internal/analysis/gate"
	"github.com/hanchaoqun/codrax/internal/analysis/hdp"
	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
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
		// Session 11 C0' — also reset the classification grep state
		// so cross-dispatch retries start with a clean budget. The
		// trigger flag is flipped back on by the evaluator's Observe
		// path once Round 1 completes and classification is found
		// ambiguous (see maybeTriggerClassificationGrep below).
		ctx.Mutable.ResetClassificationGrep()
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
		overview, graph := buildAnalyzerRepoOverview(ctx.RepoRoot, objective)
		// Publish the graph to Mutable so analyzerGraphForNormalize
		// (post-LLM, buildAnalysisIR) reuses this handle rather than
		// calling BuildOrLoadGraph a second time. Safe to set even
		// when overview is empty — the graph may still be usable.
		if graph != nil && ctx.Mutable != nil {
			ctx.Mutable.SetSearchGraph(graph)
		}
		if overview != "" {
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
// Returns (empty string, nil) on any error — the analyzer proceeds
// without an overview and the LLM calls repo_map itself (graceful
// degrade). Returns the *repomap.Graph alongside the rendered string
// so callers can publish it to Mutable.SearchGraph() and skip a
// second BuildOrLoadGraph round-trip later in the pipeline.
func buildAnalyzerRepoOverview(repoRoot, objective string) (string, *repomap.Graph) {
	if types.IsREPLControlInput(objective) {
		return "", nil
	}
	// Extract code identifiers from the question to use as the graph
	// query. extractQuestionEntities pulls CamelCase/snake_case tokens
	// — exactly the kind of tokens that match file and symbol names.
	entities := extractQuestionEntities(objective)

	query := strings.Join(entities, " ")
	graph, err := repomap.BuildOrLoadGraph(repoRoot, query)
	if err != nil {
		logging.Debug("[analyzer] repo overview unavailable: %v", err)
		return "", nil
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
		return "", graph
	}
	// Cap the overview to keep the initial prompt bounded.
	const maxLen = 4096
	if len(output) > maxLen {
		output = output[:maxLen] + "\n... [truncated]\n"
	}
	logging.Debug("[analyzer] pre-injected %s view (%d bytes, entities=%v)", view, len(output), entities)
	return header + output, graph
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

	// Session 11 C0' — after Round 1's pre-scan completes (and we
	// have not yet emitted the IR), open the ClassificationGrep
	// gate for Round 2. The gate itself does not force the LLM to
	// use line-level grep; it merely admits files_only=false calls
	// when the LLM decides the classification is worth verifying.
	// Budget (3 calls × 8 KB) is the natural throttle.
	//
	// The gate is explicitly conservative: we only open it when
	// Round 1 has surfaced declarative candidates — the pre-scan
	// summary must mention one of the declarative filename patterns
	// (topology, defaults, registry, routes, wire, init, manifest,
	// schema, enum). For plain "how does X work" questions with no
	// declarative files, C0' stays off and the analyzer falls back
	// to grep(files_only=true) only. This keeps the token cost
	// amortized to ambiguous-subject questions.
	if e.prescanRounds >= 1 && ctx != nil && ctx.Mutable != nil {
		if prescanHasDeclarativeCandidate(ctx.Mutable.PrescanSummaryBlob()) {
			ctx.Mutable.SetClassificationGrepTriggered(true)
			logging.Debug("[analyzer] C0' classification_grep triggered: Round %d pre-scan surfaced declarative candidates",
				e.prescanRounds)
		}
	}

	max := e.prescanBudgetOverride
	if max <= 0 {
		max = tool.CurrentAnalysisLimits().MaxPrescanRounds
	}
	if max <= 0 {
		return LoopSignal{}
	}
	// Last-legal-round warning: the LLM just consumed the final
	// prescan slot. Inject a strong must-emit hint so the next
	// response has a chance to call emit_analysis instead of hitting
	// the hard stop with an exhausted counter and nothing emitted.
	//
	// The 5-Q audit (2026-04-19) caught the gap: on unfamiliar repos
	// (glamour, bubbletea, lipgloss) the LLM burns the entire budget
	// verifying entities one per round, never gets a runtime signal
	// that it is at the wall, and the stage fails loud with 0
	// useful work. One grace round with a firm hint gives
	// model-compliant LLMs a chance to course-correct while
	// preserving the fail-loud contract when they do not.
	if e.prescanRounds == max {
		const hintKey = "analyzer.must-emit"
		hint := fmt.Sprintf(
			"Pre-scan budget reached (%d of %d rounds used). Your NEXT response MUST call emit_analysis with the fields you have — any additional prescan tool call (repo_map / grep / list_files) will exhaust the budget and fail the analyze stage. If you still need to verify an entity, batch the grep call in the SAME response as emit_analysis, not before it.",
			e.prescanRounds, max)
		logging.Debug("[analyzer] must-emit hint built key=%q rounds=%d/%d len=%d body=%q",
			hintKey, e.prescanRounds, max, len(hint), logging.Truncate(hint, logging.HintBodyMax))
		return LoopSignal{
			HintRequested: true,
			Progress:      true,
			HintKey:       hintKey,
			Hint:          hint,
		}
	}
	if e.prescanRounds <= max {
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
					Rationale: fmt.Sprintf("%d symbol(s) referenced earlier could not be verified against the repo — grep these to confirm existence or disprove before acting on them", len(symbolTokens)),
					Origin:    "analyzer.unverified_symbols",
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
	// Capture analyzer-authored top-level entities BEFORE deterministic
	// augmentation so provenance stays clean: MentionedEntities answers
	// "did the user say this in RawRequest?" while DerivedEntities later
	// captures log/perf/sub-topic expansions.
	rm.AnalyzerHints.PrimaryEntities = append([]string(nil), rm.AnalyzerHints.Entities...)
	rm.AnalyzerHints.MentionedEntities = types.MentionedEntitiesFromRawRequest(
		rm.RawRequest, rm.AnalyzerHints.PrimaryEntities)

	// Log-triage augmentation. The log_triage pre-stage has already
	// run and written a validated LogBundle onto Mutable.LogTriage
	// (or left it nil if no log was attached, the stage was skipped,
	// or the stage degraded). The analyzer reads it read-only: every
	// consumer below nil-checks and no-ops on absence.
	//
	// Consumers:
	//   - Entities merge: deterministic keyword union into AnalyzerHints.Entities
	//     so the normalizer's dual-gate and explorer's keyword_search can
	//     reach the extracted Func / Pkg / Error.Type tokens.
	//   - reconcileIntent: receives the bundle pointer and applies the
	//     IntentHint=RootCause override when the LLM missed the debugging signal.
	//   - analyzerRequiredFiles: reads bundle.ResolvedFiles as the first-class
	//     file seed, merged with the structural ranker output.
	var logBundle *types.LogBundle
	if ctx.Mutable != nil {
		logBundle = ctx.Mutable.LogTriage()
	}
	if logBundle != nil {
		before := len(rm.AnalyzerHints.Entities)
		if shouldMergeLogTriageEntities(logBundle) {
			rm.AnalyzerHints.Entities = logtriage.MergeEntities(
				rm.AnalyzerHints.Entities, logBundle.Entities)
		}
		logging.Info("[analyzer] log-triage: lang=%s errors=%d resolved=%d entities +%d intent=%q",
			logBundle.Meta.Lang, len(logBundle.Errors),
			len(logBundle.ResolvedFiles),
			len(rm.AnalyzerHints.Entities)-before,
			logBundle.IntentHint)
	}

	// Perf-triage augmentation. Same shape as log-triage: the
	// perf_triage pre-stage has already run and written a PerfBundle
	// onto Mutable.PerfTrace (nil when no --htrace was attached or
	// the stage degraded). Stall symbols + jank trigger spans
	// become entity hints; an IntentHint of "performance" is mirrored
	// onto the analyzer hints so the dual-gate normaliser elevates
	// performance terms to TermSymbol.
	var perfBundle *types.PerfBundle
	if ctx.Mutable != nil {
		perfBundle = ctx.Mutable.PerfTrace()
	}
	if perfBundle != nil && len(perfBundle.Entities) > 0 {
		before := len(rm.AnalyzerHints.Entities)
		rm.AnalyzerHints.Entities = logtriage.MergeEntities(
			rm.AnalyzerHints.Entities, perfBundle.Entities)
		logging.Info("[analyzer] perf-triage: source=%s frames=%d janks=%d stalls=%d entities +%d intent=%q",
			perfBundle.Meta.Source, len(perfBundle.Frames), len(perfBundle.Janks),
			len(perfBundle.Stalls),
			len(rm.AnalyzerHints.Entities)-before,
			perfBundle.IntentHint)
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
	isHistoryLookup := false

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
		resolved, reason := reconcileComplexity(rm.Complexity,
			rm.AnalyzerHints.Entities, rm.AnalyzerHints.Keywords, len(rm.SubTopics),
			rm.AnalyzerHints.Kind, rm.Predicates)
		if resolved != rm.Complexity {
			logComplexityReconcile(rm.Complexity, resolved, reason)
		}
		recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
			"complexity", string(rm.Complexity), string(resolved),
			rm.ComplexityConfidence, reason, rm.Predicates,
		))
		rm.Complexity = resolved
		// Intent sanity rule. Runs AFTER reconcileComplexity so the
		// simple-only gate reads the post-reconcile complexity. Patches
		// the "count / 统计 / how many" → enumerate mis-classification
		// that otherwise locks the pipeline into ShapeListOfSymbols
		// and an unsatisfiable file:line citation floor.
		// See internal/agent/analyzer_intent.go for the rule body.
		intentResolved, intentReason := reconcileIntent(rm.Intent, rm.Predicates, logBundle)
		if intentResolved != rm.Intent {
			logIntentReconcile(rm.Intent, intentResolved, intentReason)
		}
		recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
			"intent", string(rm.Intent), string(intentResolved),
			rm.IntentConfidence, intentReason, rm.Predicates,
		))
		rm.Intent = intentResolved

		// CGEC: AnswerSubject inference. Classifies what kind of
		// source-code literal the answer should be (skill_name,
		// agent_name, config_key, ...). Honours an LLM-supplied
		// AnswerSubject when present; otherwise applies the cue table
		// in analyzer_intent.go::inferAnswerSubject. The chain ranker
		// uses the resolved subject to demote chains whose terminal
		// is the wrong kind, and reconcileShape (post-compile)
		// consults it to swap config_value→value for source-code
		// literals that have no YAML key surface.
		subject, subjReason := inferAnswerSubject(rm)
		if subject.Kind != types.SubjectUnknown {
			logSubjectInferred(subject, subjReason)
			recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
				"subject", string(rm.AnswerSubject.Kind), string(subject.Kind),
				rm.AnswerSubject.Confidence, subjReason, rm.Predicates,
			))
			rm.AnswerSubject = subject
		}
		// PredicateAxis extraction. Orthogonal to AnswerSubject: this
		// captures the question's action verb ("how does X CALL Y" →
		// AxisCall; "how is X REGISTERED" → AxisRegister). The evidence
		// ranker consumes this via internal/analysis/axis.Affinity to
		// bias items whose AnchorKind matches the axis. Zero-value
		// (AxisUnknown) is a clean no-op; reconcilePredicateAxis only
		// FILLS empty and never overrides.
		// See internal/agent/analyzer_predicate.go for the rule body.
		axis, axisReason := reconcilePredicateAxis(rm.PredicateAxis)
		if axis != rm.PredicateAxis {
			logPredicateAxisReconcile(rm.PredicateAxis, axis, axisReason)
			recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
				"axis", string(rm.PredicateAxis), string(axis),
				0, axisReason, rm.Predicates,
			))
			rm.PredicateAxis = axis
		}
		scenarioResolved, scenarioReason := reconcileScenario(rm)
		if scenarioReason != "" {
			logScenarioReconcile(rm.Scenario, scenarioResolved, scenarioReason)
			recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
				"scenario", string(rm.Scenario), string(scenarioResolved),
				0, scenarioReason, rm.Predicates,
			))
			rm.Scenario = scenarioResolved
		}
		// Measurement-scalar signal — captures the reconciled-Intent
		// case (LLM picked enumerate, reconcileIntent downgraded to
		// return_value via IsCountQuestion), the LLM-direct case
		// (IntentReturnValue + IsCountQuestion true on first emit), AND
		// the structural-coherence fallback (shape=value + intent=
		// return_value + answer_subject.kind=numeric co-occur even
		// when IsCountQuestion slipped through as false). Computed
		// after inferAnswerSubject so the fallback sees the inferred
		// subject kind. Every consequence (shape override, 3 citation-
		// gate strips) is applied in one post-compile block below,
		// keyed off this single flag. Keeps "one signal, one response"
		// grep-able.
		isMeasurementScalar = isMeasurementScalarRequest(rm)
		isHistoryLookup = isHistoryLookupRequest(rm)

		// Merge sub-topic entities into main entity list. PrimaryEntities
		// was already captured before deterministic augmentation so the
		// provenance split stays stable here.
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
		logging.Info("[analyzer] multi-topic: %d sub-topics, primary=%v merged=%v",
			len(rm.SubTopics), rm.AnalyzerHints.PrimaryEntities, rm.AnalyzerHints.Entities)
	}
	rm.AnalyzerHints.DerivedEntities = types.DerivedEntitiesFromMentioned(
		rm.AnalyzerHints.Entities, rm.AnalyzerHints.MentionedEntities)

	// Normalizer runs unconditionally on the raw objective. When the
	// repomap graph is available (cache warm from pre-scan or foreground
	// BuildOrLoadGraph below) we pass a repo-grounded SymbolResolver so
	// kindEnWord surfaces promote to TermSymbol only when the LLM named
	// them as entities AND the repo actually defines that identifier.
	// Graph failures degrade silently to the no-resolver path — a
	// regression-free fallback with the same signature as before.
	rm.TermGraph = normalizer.Normalize(
		rm.RawRequest,
		normalizer.Options{
			Resolver: newRepomapSymbolResolver(analyzerGraphForNormalize(ctx, rm)),
			Entities: rm.AnalyzerHints.Entities,
		},
	)

	// Session 11 C0' step 1.5: reconcile classification from any
	// line-level grep observations the analyzer captured in Round 2.
	// Runs AFTER normalizer (so TermGraph is stable) and BEFORE
	// scenario inference + compiler.Compile (so the refined
	// answer_subject.kind + AnalyzerHints.Shape propagate into the
	// deterministic pipeline). When the C0' trigger never fired,
	// ClassificationObservations is empty and reconcileFromObservations
	// is a no-op.
	if ctx != nil && ctx.Mutable != nil {
		obs := ctx.Mutable.ClassificationObservations()
		if len(obs) > 0 {
			logs := reconcileFromObservations(&rm, obs)
			for _, line := range logs {
				logging.Info("[analyzer] %s", line)
			}
		}
	}

	// Session 11 R2 auto-keywords — when the classified question
	// will benefit from declarative-file retrieval (registration,
	// config_mapping, call_chain, or a subject.kind that names a
	// source-code literal), supplement the LLM-provided keywords
	// with declarative filename stems so explorer-side
	// keyword_search (which receives rm.AnalyzerHints.Keywords)
	// tips toward topology/defaults/registry/routes/wire/init/
	// manifest/schema/enum files. Pure-append semantics: existing
	// keywords are never removed.
	if shouldAutoKeywordBoost(&rm) {
		added := appendDeclarativeKeywords(&rm.AnalyzerHints.Keywords)
		if len(added) > 0 {
			logging.Debug("[analyzer] R2 auto-keywords added: %v", added)
		}
	}

	// Scenario default.
	if rm.Scenario == "" || (rm.Scenario == types.ScenarioGeneric && !isScalarSourceLiteralLookup(rm)) {
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
	if isMeasurementScalar || isHistoryLookup {
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
	reconciledShape, shapeReason := reconcileShape(rm, out.AnswerContract.RequiredAnswerShape, rm.AnswerSubject, rm.Predicates)
	if reconciledShape != out.AnswerContract.RequiredAnswerShape {
		before := out.AnswerContract.RequiredAnswerShape
		logShapeReconciled(before, reconciledShape, shapeReason)
		recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
			"shape", string(before), string(reconciledShape),
			rm.ShapeConfidence, shapeReason, rm.Predicates,
		))
		out.AnswerContract.RequiredAnswerShape = reconciledShape
		// Also align the AnalyzerHints surface so downstream readers
		// (ir_accessor.irAnswerShape, answer_document_evaluator,
		// emit_answer_document shape auto-correct) see the
		// reconciled shape. Cover both the config_value→value and the
		// new conditional-enumeration→list_of_symbols swap.
		legacy := mapLegacyAnswerShape(rm.AnalyzerHints.Shape)
		if legacy == types.ShapeConfigValue || legacy == before {
			rm.AnalyzerHints.Shape = string(reconciledShape)
		}
	}

	out.AnswerContract.Diagram = reconcileDiagramContract(rm, out.AnswerContract.RequiredAnswerShape, logBundle)
	out.AnswerContract.ExactResolution = types.BuildExactResolutionContract(rm)

	ir := &types.AnalysisIR{
		Version:        types.AnalysisIRVersion,
		RequestModel:   rm,
		TaskGraph:      out.TaskGraph,
		EvidencePlan:   out.EvidencePlan,
		AnswerContract: out.AnswerContract,
		HypothesisSet:  hypotheses,
	}
	// T3a — populate EvidencePlan.RequiredFiles from the repo_map
	// graph query over the analyzer-extracted entities. The analyzer
	// already runs the same query at BuildInitialInstruction time
	// (buildAnalyzerRepoOverview) to render the task_map view into
	// the prompt, but previously the file list was discarded after
	// rendering. This hook captures the list structurally so the
	// explorer's keyword-search can merge it with its own ranking —
	// closing the gap where the analyzer identified the right files
	// but the signal evaporated before reaching the explorer.
	//
	// Graceful degrade: a nil ctx, empty RepoRoot, missing
	// AnalyzerHints.Entities, or a repomap.BuildOrLoadGraph failure
	// all land on an empty RequiredFiles slice. Downstream consumers
	// already handle the empty case.
	ir.EvidencePlan.RequiredFiles = analyzerRequiredFiles(ctx, rm)
	ir.QualityGate = gate.Run(ir, gate.GlobalThresholds())

	// Writeback the reconciled RequestModel to Mutable so downstream
	// agents reading via ctx.Mutable.RequestModel() see the same
	// reconciled fields the IR carries. Without this step the
	// reconcile* functions above (Complexity / Intent / Scenario /
	// PredicateAxis / AnswerSubject + SubTopics synthesis) write to
	// a local rm copy that only surfaces through ctx.AnalysisIR;
	// tools that still read from Mutable (extractor's axis validator,
	// emit_analysis callers on a re-entry) would see pre-reconcile
	// zero values and silently skip work.
	if ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.SetRequestModel(rm)
	}

	// Schema-v4 observability: render a single [reconcile-shadow]
	// summary line aggregating every reconcile event recorded above.
	// Always emits — even on a quiet run — so the absence of the line
	// is a positive signal that the analyzer never ran (broken Mutable
	// threading) rather than ambiguous silence.
	EmitReconcileSummary(ctxMutable(ctx))

	return ir, nil
}

func shouldMergeLogTriageEntities(bundle *types.LogBundle) bool {
	if bundle == nil {
		return false
	}
	return !bundle.IsExternalSource()
}

// analyzerRequiredFiles queries the repo_map graph with the
// analyzer-extracted entities and returns the top-N matching file
// paths. These are the files the analyzer "expects" the explorer to
// consider relevant, based solely on structural matching of entity
// names to symbols in the graph. Non-authoritative — a soft hint,
// not a hard requirement.
//
// Ranking is two-tiered so verbatim user intent dominates diffuse
// doc-comment matches:
//
//  1. Exact anchors first. `exactEntityAnchors` (see
//     internal/agent/keyword_search.go) promotes files whose path
//     or unique symbol definition matches an entity verbatim —
//     e.g. entity "explorer.go" → `internal/agent/explorer.go`
//     (path_exact, rank 2); entity "ValidateFoo" → the file that
//     uniquely defines it (symbol_exact, rank 2); entity "Foo.Bar"
//     → the file defining Bar on receiver Foo (qualified_symbol_
//     exact, rank 3). Uniqueness — not any match — is what keeps
//     the tier from amplifying noisy entities (e.g. "ShouldStop"
//     matches many files, so it never anchors).
//
//  2. Then QueryScore hits. Everything the anchor tier did not
//     catch falls through to the historical score-sorted list,
//     which covers the common case where no entity names a
//     file/symbol verbatim.
//
// The 5-Q audit (2026-04-19) showed the score-only path ranking
// user-named files 6th–10th below doc-comment-heavy unrelated
// files; the anchor tier flips those back to the top without
// affecting the noise-driven cases it can't help with.
//
// Returns nil on any error or missing input; the caller assigns
// directly so a nil slice is a valid zero value.
// analyzerGraphForNormalize returns the repomap graph handle used to
// back the normalizer SymbolResolver during buildAnalysisIR. It first
// tries the already-loaded graph on Mutable (published by pre-scan or
// a prior stage) and falls back to an eager BuildOrLoadGraph keyed by
// the analyzer's entity list. Returns nil on any failure; the caller
// treats nil graph as "resolver unavailable, use concept fallback".
func analyzerGraphForNormalize(ctx *types.AgentContext, rm types.RequestModel) *repomap.Graph {
	if ctx == nil || ctx.RepoRoot == "" {
		return nil
	}
	if ctx.Mutable != nil {
		if g, ok := ctx.Mutable.SearchGraph().(*repomap.Graph); ok && g != nil {
			return g
		}
	}
	entities := rm.AnalyzerHints.Entities
	if len(entities) == 0 {
		entities = extractQuestionEntities(
			types.StripConversationPrefix(rm.RawRequest))
	}
	query := strings.Join(entities, " ")
	g, err := repomap.BuildOrLoadGraph(ctx.RepoRoot, query)
	if err != nil || g == nil {
		return nil
	}
	// Publish so analyzerRequiredFiles + downstream stages skip a
	// second BuildOrLoadGraph round-trip. Safe: SetSearchGraph accepts
	// nil and is written-once-per-run by the main agent.
	if ctx.Mutable != nil {
		ctx.Mutable.SetSearchGraph(g)
	}
	return g
}

func analyzerRequiredFiles(ctx *types.AgentContext, rm types.RequestModel) []string {
	if ctx == nil || ctx.RepoRoot == "" {
		return nil
	}
	if types.IsREPLControlInput(types.StripConversationPrefix(rm.RawRequest)) {
		return nil
	}
	entities := rm.AnalyzerHints.Entities
	if len(entities) == 0 {
		// Fall back to extracting CamelCase/snake_case tokens from
		// the raw request — same strategy as
		// buildAnalyzerRepoOverview uses when no entity list was
		// emitted. Keeps the hint active even when the LLM left
		// entities empty (genuine ambiguity path).
		entities = extractQuestionEntities(
			types.StripConversationPrefix(rm.RawRequest))
	}

	// Log-triage augmentation. The log_triage pre-stage has already
	// resolved every stack-frame path against the repo (os.Stat,
	// Java basename glob, runtime-internal filter) and stored the
	// validated repo-relative list as bundle.ResolvedFiles. Read it
	// read-only and prepend it to the structural ranker output.
	var logFiles []string
	var logAuthoritative bool
	var externalSource bool
	if ctx.Mutable != nil {
		if bundle := ctx.Mutable.LogTriage(); bundle != nil {
			logFiles = bundle.ResolvedFiles
			logAuthoritative = logBundleAuthoritativeFrames(bundle)
			externalSource = bundle.IsExternalSource()
		}
		// Perf-triage ResolvedFiles (PerfStall.File ∪ Frame.File) join
		// the same authoritative seed list. A jank stall whose Symbol
		// resolved to entry/src/main/ets/services/DataLoader.ets is as
		// load-bearing as a panic frame; both should anchor the file
		// ceiling. The two seeds union (de-dup on append).
		if perf := ctx.Mutable.PerfTrace(); perf != nil && len(perf.ResolvedFiles) > 0 {
			seen := map[string]bool{}
			for _, f := range logFiles {
				seen[f] = true
			}
			for _, f := range perf.ResolvedFiles {
				if !seen[f] {
					seen[f] = true
					logFiles = append(logFiles, f)
				}
			}
		}
	}

	// Session-22 fix F1.1 — authoritative log ceiling.
	//
	// When the attached log is a runtime panic / crash (Signals ∩
	// {panic, crash} non-empty) AND it resolved to ≥1 repo file,
	// bundle.ResolvedFiles IS the answer's file set. Every frame in
	// the stack is a concrete file:line; there is no other file the
	// call chain could live in. Running the structural ranker would
	// only drown the authoritative anchor under noise — the ranker's
	// exact-anchor tier matches common method names (e.g. ParseOutput
	// is defined on six evaluators) and promotes unrelated siblings
	// into "Analyzer's Required Files", which the explorer then reads
	// and the finalizer hallucinates into the answer's call-chain
	// diagram. Observed on logtri_go-20260421-112818.
	//
	// For log-triage bundles with non-crash signals (oom / timeout /
	// validation / db / network / permission / logic), the resolved
	// files are usually seeds rather than a complete call chain, so
	// we still run the ranker and merge — those queries benefit from
	// ranker breadth the same way no-log queries do.
	if logAuthoritative && len(logFiles) > 0 {
		return append([]string(nil), logFiles...)
	}
	if externalSource {
		return nil
	}

	if len(entities) == 0 && len(logFiles) == 0 {
		return nil
	}
	var graph *repomap.Graph
	if len(entities) > 0 {
		query := strings.Join(entities, " ")
		if g, err := repomap.BuildOrLoadGraph(ctx.RepoRoot, query); err == nil {
			graph = g
		}
	}

	ranked := rankAnalyzerRequiredFiles(graph, entities)
	if len(logFiles) == 0 {
		return ranked
	}
	return logtriage.MergeResolvedFiles(logFiles, ranked)
}

// logBundleAuthoritativeFrames reports whether a log-triage bundle
// should be treated as the file-set ceiling: at least one runtime
// crash signal (panic / crash) AND at least one resolved frame. The
// predicate is shared by analyzerRequiredFiles (F1.1 ceiling) and
// the explorer's Check 6 ranker-coverage gate (F2.1 skip) so both
// sites read the same log-triage authority contract.
func logBundleAuthoritativeFrames(bundle *types.LogBundle) bool {
	if bundle == nil || len(bundle.ResolvedFiles) == 0 {
		return false
	}
	for _, s := range bundle.Meta.Signals {
		if s == types.SignalPanic || s == types.SignalCrash {
			return true
		}
	}
	return false
}

// rankAnalyzerRequiredFiles implements the two-tier ranking
// described on analyzerRequiredFiles. Split into its own function
// so unit tests can exercise the ranker directly with a mock
// *repomap.Graph without going through BuildOrLoadGraph.
func rankAnalyzerRequiredFiles(graph *repomap.Graph, entities []string) []string {
	// Session-22 fix F1.2 — cap lowered from 10 to 3. The original
	// ceiling was set when "Required Files" meant a soft hint; it now
	// feeds the explorer's "Analyzer's Required Files" prompt block
	// and the Check-6 ranker-coverage gate, both of which treat every
	// entry as a first-class read target. Ten files was enough to
	// swamp the prompt with ranker-noise siblings whenever an entity
	// name (e.g. ParseOutput, defined on six evaluators) matched
	// multiple exact anchors. Three matches how many candidates an
	// operator realistically checks in a focused investigation.
	const maxAnalyzerRequiredFiles = 3
	if graph == nil {
		return nil
	}

	// Tier 1: exact anchors. Rank is whatever exactEntityAnchors
	// assigned (qualified_symbol_exact=3, path_exact/symbol_exact=2).
	// Only files with a UNIQUE verbatim hit appear here; that
	// uniqueness is the tier's guardrail against amplifying noisy
	// entities.
	anchors := exactEntityAnchors(graph, entities)

	type scored struct {
		path  string
		tier  int     // >0 for anchored, 0 for pure QueryScore
		rank  int     // anchor.Rank when tier>0, 0 otherwise
		score float64 // QueryScore when tier==0, 0 otherwise
	}
	seen := make(map[string]bool, len(anchors))
	var hits []scored
	for path, a := range anchors {
		hits = append(hits, scored{path: path, tier: 1, rank: a.Rank})
		seen[path] = true
	}

	// Tier 2: QueryScore fallthrough. Skip files already anchored
	// so they are not demoted below their tier-1 slot by a weaker
	// tie-break.
	for path, qs := range graph.QueryScores {
		if qs <= 0 || seen[path] {
			continue
		}
		hits = append(hits, scored{path: path, score: qs})
	}
	if len(hits) == 0 {
		return nil
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].tier != hits[j].tier {
			return hits[i].tier > hits[j].tier
		}
		if hits[i].rank != hits[j].rank {
			return hits[i].rank > hits[j].rank
		}
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		// Deterministic tie-break for identical scores so the
		// prompt text is stable across runs. Important for the
		// "Analyzer's Required Files" rendering to avoid reshuffle
		// noise between otherwise-equivalent runs.
		return hits[i].path < hits[j].path
	})
	if len(hits) > maxAnalyzerRequiredFiles {
		hits = hits[:maxAnalyzerRequiredFiles]
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.path)
	}
	return out
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

// ── Session 11 R2 auto-keywords helper ─────────────────────────────

// shouldAutoKeywordBoost reports whether rm's classification
// qualifies for the R2 declarative-keyword append. The trigger is
// generic: any question_kind the analyzer can emit that "most
// likely resolves to a declarative file" (registration,
// config_mapping, call_chain) OR any AnswerSubject.Kind that
// names a source-code identifier (judged via the subject
// taxonomy's HasJudge, not by enumerating domain-specific kinds).
// This keeps the boost gate language- and domain-neutral: adding
// a new kind to the taxonomy automatically qualifies it; no
// change is needed here for a new codebase to benefit.
func shouldAutoKeywordBoost(rm *types.RequestModel) bool {
	if rm == nil {
		return false
	}
	switch rm.AnalyzerHints.Kind {
	case "registration", "config_mapping", "call_chain":
		return true
	}
	if rm.AnswerSubject.Kind != types.SubjectUnknown &&
		subjectHasIdentifierJudge(rm.AnswerSubject.Kind) {
		return true
	}
	return false
}

// subjectHasIdentifierJudge reports whether a subject.Kind names
// a *source-code identifier* (as opposed to a value, prose, or
// absence answer). Declarative-boost is appropriate when the
// answer is expected to BE an identifier that a declarative file
// typically defines. Keeps the set in one place so new identifier
// kinds automatically flow through both R2 (here) and C5
// (literal-form check).
//
// This is intentionally declared by iteration over the taxonomy's
// behaviour, not by enumerating specific kind values — SkillName,
// AgentName, HandlerRoute etc. are codrax / HTTP-level concepts;
// FunctionName / TypeName / ConfigKey are cross-language. The
// single-level "does this kind name an identifier?" question is
// answered by the universal property "the subject judge awards
// ≥ 0.5 on the representative identifier token for its kind",
// but since that evaluation needs a real token, we use the
// simpler proxy here: any kind whose string label ends in "_name",
// "_key", "_route", or is a documented identifier kind counts.
func subjectHasIdentifierJudge(k types.AnswerSubjectKind) bool {
	s := string(k)
	if strings.HasSuffix(s, "_name") ||
		strings.HasSuffix(s, "_key") ||
		strings.HasSuffix(s, "_route") ||
		strings.HasSuffix(s, "_path") {
		return true
	}
	return false
}

// appendDeclarativeKeywords mutates *kws in place, appending any
// declarative filename stems not already present. Returns the
// slice of newly-added keywords for logging. Dedup is done with a
// linear scan — keyword lists are typically ≤ 15 entries so an O(N)
// append is fine and avoids a map allocation.
func appendDeclarativeKeywords(kws *[]string) []string {
	if kws == nil {
		return nil
	}
	existing := make(map[string]bool, len(*kws))
	for _, k := range *kws {
		existing[strings.ToLower(k)] = true
	}
	var added []string
	for _, cand := range declarative.DefaultFilenamePatterns() {
		if existing[cand] {
			continue
		}
		*kws = append(*kws, cand)
		added = append(added, cand)
	}
	return added
}

// ── Session 11 C0' ClassificationGrep helpers ──────────────────────

// prescanHasDeclarativeCandidate reports whether the analyzer's
// pre-scan blob mentions any declarative-filename pattern. The blob
// is the lowercased concatenation of every pre-scan ToolResult
// Summary (see MutableState.PrescanSummaryBlob), so a substring
// match on declarative.DefaultFilenamePatterns is a cheap and
// lossless proxy for "Round 1 found files that could carry the
// answer". We reuse declarative.DefaultFilenamePatterns instead of
// hard-coding the list here so the trigger word list stays aligned
// with the R1 DeclarativeBoost ranker in G6.
func prescanHasDeclarativeCandidate(blob string) bool {
	if blob == "" {
		return false
	}
	// blob is already lowercased; patterns are ASCII-lowercase by
	// convention in DefaultFilenamePatterns.
	for _, pat := range declarative.DefaultFilenamePatterns() {
		if pat == "" {
			continue
		}
		if strings.Contains(blob, pat) {
			return true
		}
	}
	return false
}

// reconcileFromObservations consumes the C0' ClassificationObs
// sidecar and refines fields on rm. The caller (buildAnalysisIR) is
// expected to have already run the normalizer; this function runs
// BEFORE compiler.Compile so the refined values feed the downstream
// deterministic pipeline.
//
// Reconciliation rules (Session 11 full-design §5.5 table):
//
//   - match literal has `-skill` suffix
//     → AnswerSubject.Kind = skill_name, AnswerShape = value
//   - match literal matches NewXxxAgent(... pattern
//     → AnswerSubject.Kind = agent_name, AnswerShape = value
//   - match literal is in a struct-field table `{Field1: V1, Field2: V2}`
//     → EntityAxes = [Field1 → Field2]
//   - match is `@route(...)` / `app.get(...)` pattern
//     → AnswerSubject.Kind = handler_route
//   - all matches inside `var X = map[...]{...}` body
//     → QuestionKind ∈ {registration, config_mapping}
//
// Each successful reconcile appends a log entry to
// rm.AnalyzerHints.ReconcileLog so the operator can audit why a
// field changed. The reconciler is purely additive — it never
// removes a value, only refines it when the observation is
// unambiguous.
func reconcileFromObservations(rm *types.RequestModel, obs []types.ClassificationObs) []string {
	if rm == nil || len(obs) == 0 {
		return nil
	}

	// Session-22 follow-up B4 — ShapeConfidence floor guard.
	//
	// C0' reconcile is a coarse, single-observation-triggered rule
	// ("I saw a quoted literal, so the answer is probably a literal").
	// It was designed as a safety net for LLMs that hedge or punt on
	// shape classification. When the LLM rated its own shape
	// classification at or above the floor, we should trust it —
	// over-riding a confident classification on a single grep hit is
	// the bug pattern we saw on m1a (ShapeConfidence=0.85 silently
	// downgraded to `value` because Round-2 grep happened to land on
	// a string-shaped match).
	//
	// Mirrors the `kindConfidenceFloorForNarrowing = 0.7` pattern in
	// explorer.go: "zero = LLM declined to rate = treat as low", so
	// the reconcile still fires for truly-unrated cases that C0' was
	// originally designed for.
	if rm.ShapeConfidence > 0 && rm.ShapeConfidence >= shapeConfidenceFloorForC0Reconcile {
		return nil
	}

	var logs []string
	// The analyzer's string shape hint lives in AnalyzerHints.Shape
	// (a raw LLM-extracted label); the typed AnswerContract.
	// RequiredAnswerShape is filled later by compiler.Compile from
	// the hint. Rewriting the hint here lets the downstream compile
	// pick up the refined shape without us touching the compiled
	// AnswerContract directly.
	shapeValueLabel := string(types.ShapeValue)

	// C0' reconciliation is **language- and domain-neutral**: it
	// operates only on observations that confirm the LLM's shape
	// hint is a `value` shape (we saw a quoted literal in a
	// declarative line). The reconciler does NOT hard-code
	// domain-specific kind enums — picking a specific kind like
	// "skill_name" vs "agent_name" is the analyzer-stage intent
	// inference's job (analyzer_intent.go::inferAnswerSubject),
	// which ALREADY consumes AnswerSubject hints and the subject
	// taxonomy. Here we only nudge AnalyzerHints.Shape toward
	// "value" when the observation is structurally literal-like,
	// and let the downstream inference keep its subject decision.
	//
	// This keeps Session 11 generic: adding a new
	// AnswerSubjectKind does not require a new reconciler branch,
	// and the reconciler works identically on any source-code
	// literal regardless of which domain concept it names.
	for _, o := range obs {
		lits := extractQuotedLiterals(o.Text)
		if len(lits) == 0 {
			continue
		}
		if rm.AnalyzerHints.Shape != shapeValueLabel {
			logs = append(logs, fmt.Sprintf(
				"reconcile shape %q → %q (observation %s:%d contained quoted literal %q)",
				rm.AnalyzerHints.Shape, shapeValueLabel, o.Path, o.Line, lits[0]))
			rm.AnalyzerHints.Shape = shapeValueLabel
			break // one reconcile per dispatch
		}
	}
	return logs
}

// shapeConfidenceFloorForC0Reconcile is the minimum LLM-emitted
// ShapeConfidence at which C0' reconcileFromObservations will defer
// to the LLM's shape classification and skip the quoted-literal
// downgrade rule. Below this floor (or at zero — LLM declined to
// rate), the safety net still fires for the ambiguous-subject
// scenarios the rule was originally designed to catch.
//
// 0.7 matches `kindConfidenceFloorForNarrowing` in explorer.go so
// every confidence-gated downstream behaviour speaks the same
// threshold language. The symmetric-direction convention (narrow ONLY
// when high confidence vs. override ONLY when low confidence) does
// not change the chosen threshold — in both cases 0.7 separates
// "confident enough to be trusted" from "hedged enough to be
// overridable".
const shapeConfidenceFloorForC0Reconcile = 0.7

// extractQuotedLiterals walks s and returns every single- or
// double-quoted substring as a slice of unquoted tokens. Used by
// the C0' reconciler to feed candidate literals into the subject
// taxonomy without embedding language-specific parsing here. Empty
// literals are skipped. The helper is language-agnostic: it works
// on Go, Python, YAML, JSON, and any text where the quote form is
// ASCII " or '.
func extractQuotedLiterals(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, q := range []byte{'"', '\''} {
		i := 0
		for i < len(s) {
			start := strings.IndexByte(s[i:], q)
			if start < 0 {
				break
			}
			start += i
			end := strings.IndexByte(s[start+1:], q)
			if end < 0 {
				break
			}
			end += start + 1
			if end > start+1 {
				out = append(out, s[start+1:end])
			}
			i = end + 1
		}
	}
	return out
}

// (Ex-helpers containsSkillLiteral / containsNewAgentLiteral /
// isASCII* removed in the Session 11 over-fitting audit.
// reconcileFromObservations is intentionally minimal: it scans
// observations for any quoted literal via extractQuotedLiterals and
// — on the first hit — downgrades AnalyzerHints.Shape to "value".
// Picking a specific AnswerSubjectKind is inferAnswerSubject's job
// (runs after this reconciler), so the subject.Score taxonomy is
// consulted downstream rather than here.
//
// A prior version of this note claimed this function "delegates to
// subject.Score"; that was an aspirational description of a design
// that was never implemented, not a contract. Reconcile uses the
// quoted-literal heuristic plus two guards: (a) test-file filter in
// scanGrepLinesIntoClassificationObs keeps fixture strings out of
// the obs pool, and (b) the ShapeConfidence >= 0.7 floor above keeps
// confident LLM classifications from being blindly overridden.)
