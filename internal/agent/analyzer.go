package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/analysis/amplifier"
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
	"github.com/hanchaoqun/codrax/internal/analysis/prescan"
	"github.com/hanchaoqun/codrax/internal/analysis/risk"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/retrieve"
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
		estimated := EstimateSubTopicCount(ctx.Objective)
		if estimated > 1 {
			base := tool.CurrentAnalysisLimits().MaxPrescanRounds
			if base > 0 {
				extra := (estimated / 2) * e.agentSettings.SubTopicPrescanBudgetExtra
				adjusted := base + extra
				// Cap at base + 2, bounded by AgentSettings.PrescanRoundsCeil
				// (yaml: agent_prescan_rounds_ceil, default 4).
				cap := base + 2
				ceil := e.agentSettings.PrescanRoundsCeil
				if ceil <= 0 {
					ceil = 4
				}
				if cap > ceil {
					cap = ceil
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
	if ctx != nil && ctx.Mutable != nil {
		limit := e.prescanBudgetOverride
		if limit <= 0 {
			limit = tool.CurrentAnalysisLimits().MaxPrescanRounds
		}
		ctx.Mutable.SetPrescanRoundLimit(limit)
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
		overview, graph := buildAnalyzerRepoOverview(ctx, objective)
		// Publish the graph to Mutable so analyzerGraphForNormalize
		// (post-LLM, buildAnalysisIR) reuses this handle rather than
		// calling BuildOrLoadGraph a second time. Safe to set even
		// when overview is empty — the graph may still be usable.
		if graph != nil && ctx.Mutable != nil {
			ctx.Mutable.SetSearchGraph(graph)
		}
		if overview != "" {
			return prependEmitRetryDirective(ctx, prependAnswerPitfalls(ctx, overview))
		}
	}
	return prependEmitRetryDirective(ctx, prependAnswerPitfalls(ctx, ""))
}

// prependAnswerPitfalls renders the read-mode Answer Taxonomy
// injection (commit 51 Gap 3, mirror of buildActivePitfallsSection
// on the planner). Empty when ctx.ActiveAnswerPitfalls is nil/empty.
//
// Same descriptive framing as the planner's pitfalls section: the
// LLM reads observations from prior Runs, NOT instructions. The
// analyzer is the decider. Past Runs in this repo had to retry
// when classifying these patterns; the new analyze call should be
// alert to whether THIS request might trip the same trap, but it
// is free to classify differently if the data warrants.
func prependAnswerPitfalls(ctx *types.AgentContext, base string) string {
	if ctx == nil || len(ctx.ActiveAnswerPitfalls) == 0 {
		return base
	}
	var b strings.Builder
	b.WriteString("## Known answer pitfalls in this repo\n\n")
	b.WriteString("Past Runs on this repo had to retry classification on requests that matched the patterns below. Each one is annotated with what triggers it. When classifying THIS request, check whether it could match any of these patterns; if so, adjust your classification to avoid the trap. The analyzer is the decider — these are observations from prior Runs, not instructions.\n\n")
	for _, p := range ctx.ActiveAnswerPitfalls {
		fmt.Fprintf(&b, "- **%s** — %s\n", strings.TrimSpace(p.Name), strings.TrimSpace(p.Description))
		if t := strings.TrimSpace(p.Trigger); t != "" {
			fmt.Fprintf(&b, "  - Trigger: %s\n", t)
		}
		if c := strings.TrimSpace(p.Consequence); c != "" {
			fmt.Fprintf(&b, "  - Typical consequence: %s\n", c)
		}
	}
	if base == "" {
		return strings.TrimRight(b.String(), "\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n\n" + base
}

// prependEmitRetryDirective prepends a terminal-forcing directive to
// the analyzer's initial instruction when this dispatch is a retry
// after a "tool_choice=required produced no tool call" failure
// (ctx.EmitStageRetryAttempt > 0). On the happy path (attempt 0) the
// instruction is returned unchanged.
//
// The directive carries:
//
//   - A loud, explicit acknowledgment that the previous attempt
//     produced text instead of a tool call.
//   - The literal JSON shape of the tool call the model must emit,
//     so a model that "knows" emit_analysis is required but skips
//     the syntax sees the exact form.
//
// Combined with the named-function tool_choice that resolveToolChoice
// switches to on retry, this closes the failure mode where a model
// acknowledges the requirement in <think> but produces no tool call.
// The pattern is reusable for any single-emit stage; analyzer is the
// first user because robot-sim-go (Batch K) demonstrated the gap.
func prependEmitRetryDirective(ctx *types.AgentContext, base string) string {
	if ctx == nil || ctx.EmitStageRetryAttempt <= 0 {
		return base
	}
	zh := analyzerPromptLangIsZh(ctx.Language)
	var directive string
	if zh {
		directive = fmt.Sprintf(`## 终止式重试约束 - 第 %d 次重试

上一次尝试没有形成可执行的结构化分析结果。分析阶段必须以一次 `+"`emit_analysis`"+` 工具调用结束。下一次回复必须只包含一个 `+"`emit_analysis`"+` 的 tool_call，并使用你已经判断出的字段。不要输出额外散文，不要调用预扫描工具（repo_map / grep / list_files）。需要的调用形态是：

`+"```json"+`
{"name": "emit_analysis", "arguments": "<json-encoded fields>"}
`+"```"+`

如果调用其它工具，或只输出散文而没有工具调用，本次分析会失败并退出。

`, ctx.EmitStageRetryAttempt)
	} else {
		directive = fmt.Sprintf(`## TERMINAL FORCING — Retry attempt %d

The previous attempt produced text-only output and FAILED. The analyze stage MUST end with a single emit_analysis tool call. Your next response MUST contain exactly one tool_call to emit_analysis with the fields you already have. Do NOT emit additional thinking or prose. Do NOT call any pre-scan tool (repo_map / grep / list_files). The exact wire shape required:

`+"```json"+`
{"name": "emit_analysis", "arguments": "<json-encoded fields>"}
`+"```"+`

If you call any other tool or produce text without a tool call, this dispatch will fail loud and the analyze stage will exit.

`, ctx.EmitStageRetryAttempt)
	}

	// Coherence retry hint: when buildAnalysisIR's previous attempt
	// rejected the IR for a cross-signal coherence violation, the
	// detail strings are pre-staged on Mutable. Render them here so
	// the LLM sees the structural contradiction rather than guessing
	// from "gate rejected".
	if ctx.Mutable != nil {
		if hint := ctx.Mutable.AnalyzerRetryHint(); hint != "" {
			ctx.Mutable.ResetAnalyzerRetryHint()
			if zh {
				directive += "## 上一次 emit_analysis 的结构诊断\n\n" +
					hint + "\n\n" +
					"请在下一次 emit_analysis 中修正这些结构关系。上面的字段是你自己输出的 IR 交叉引用；系统没有改动仓库，只是校验了你声明的关系。\n\n"
			} else {
				directive += "## Structural contradiction in your previous emit_analysis\n\n" +
					hint + "\n\n" +
					"Re-emit emit_analysis with these contradictions resolved. The fields above are LLM-emitted IR cross-references — the system has not changed your repo, only verified the relationships you yourself declared.\n\n"
			}
		}
	}

	if base == "" {
		return directive
	}
	return directive + base
}

func analyzerPromptLangIsZh(lang string) bool {
	return !strings.EqualFold(strings.TrimSpace(lang), "en")
}

// composeCoherenceRetryHint inspects a rejected GateReport and
// renders the failing coherence-check details into a multi-line
// retry hint suitable for the analyzer's next dispatch directive.
// Returns "" when none of the coherence-named checks failed, so the
// generic retry path stays in effect for non-coherence rejections
// (coverage, dag_closure, budget_sanity, etc.).
//
// The Detail field on GateCheck encodes internal rule codes (R1.1,
// R1.2, R1.3, R2.1, R2.2) for log/trace forensics. Those codes are
// internal language — the LLM doesn't know what "R1.2 predicate
// contradiction" means and gets confused. Translate to plain prose
// before injecting into the next-dispatch hint, so the LLM sees a
// concrete description of the structural issue rather than a code
// reference. Internal Detail still flows to logs verbatim via the
// gate's recordReconcileObservation path.
// composeRequiredFileHintsRetryAdvice (2026-05-10 L3-T4) builds an
// optional advisory paragraph appended to the analyzer's retry hint
// when the prior emit_analysis carried RequiredFileHints whose
// confidence sat in the borderline band (0.5–0.79). The downstream
// explorer threshold-bands those entries: ≥0.8 join the primary set
// + pre-read pool; 0.5–0.79 are pre-read only; <0.5 are dropped.
//
// Borderline entries are advisory hints to the LLM that "you can
// either confirm this file with stronger evidence — push the
// confidence up — or drop it — let the deterministic resolver
// decide." This nudges the next dispatch toward a clearer signal
// rather than the same lukewarm recommendation.
//
// Returns empty string when:
//   - ir is nil
//   - no RequiredFileHints were emitted
//   - all hints are already at high confidence (≥0.8) — no advice
//   - all hints are below 0.5 — already dropped, no advice
//
// Cross-language: the returned text uses generic phrasing ("file"
// not "Go file" / "Python module") so it applies to every language
// codrax's repomap supports.
func composeRequiredFileHintsRetryAdvice(ir *types.AnalysisIR) string {
	if ir == nil {
		return ""
	}
	hints := ir.RequestModel.AnalyzerHints.RequiredFileHints
	if len(hints) == 0 {
		return ""
	}
	var borderline []types.RequiredFileHint
	for _, h := range hints {
		if h.Confidence >= 0.5 && h.Confidence < 0.8 {
			borderline = append(borderline, h)
		}
	}
	if len(borderline) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Your prior emit listed these files at borderline confidence (0.5–0.79). On retry, either confirm with a stronger rationale (push confidence ≥ 0.8 so the next stage treats the file as primary) OR drop the entry (let the deterministic file resolver decide):\n")
	for _, h := range borderline {
		fmt.Fprintf(&b, "  - `%s` (confidence %.2f)\n", h.Path, h.Confidence)
	}
	return strings.TrimRight(b.String(), "\n")
}

func composeCoherenceRetryHint(report types.GateReport) string {
	if !report.Rejected {
		return ""
	}
	var b strings.Builder
	for _, c := range report.Checks {
		if c.Passed {
			continue
		}
		switch c.Name {
		case "subtopic_coherence", "shape_subject_coherence":
			fmt.Fprintf(&b, "- %s\n", plainCoherenceDetail(c.Detail))
		}
	}
	// Fix-B (2026-05-10): when R2.2 longform_scalar_subject fires
	// in this report, append targeted remediation guidance pointing
	// at the most common cause (LLM treating an error name in the
	// question as the answer) and the two corrective actions
	// (unset / non-scalar kind). This sits OUTSIDE the per-rule
	// loop so it fires regardless of which checks reported the
	// rule + survives any future check renaming.
	if reportContainsR22(report) {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(r22RetryAdvice())
	}
	return strings.TrimSpace(b.String())
}

// reportContainsR22 reports whether the gate report has a failed
// shape_subject_coherence check whose Detail starts with the R2.2
// rule prefix. Mirrors the orchestrator-side detection in
// r22AutoCorrectShapeSubject.
func reportContainsR22(report types.GateReport) bool {
	for _, c := range report.Checks {
		if c.Passed {
			continue
		}
		if c.Name != "shape_subject_coherence" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(c.Detail), "R2.2 longform_scalar_subject:") {
			return true
		}
	}
	return false
}

// r22RetryAdvice returns the LLM-facing advisory paragraph that
// accompanies an R2.2 retry hint. Phrased in language-/domain-
// neutral structural terms — no internal stage codenames, no
// language-specific examples (the same advice applies to Go panics,
// Python tracebacks, Java exceptions, Rust crashes, JS errors, etc.),
// no per-case curve-fitting (works for root-cause / call-chain /
// architecture / config-precedence / comparison and any other
// explanation-shape question).
//
// Design doc: docs/design/analyzer_failure_remediation.md §3.2.
func r22RetryAdvice() string {
	return "Your prior emit set `answer_subject.kind` to a scalar value, but the question's shape calls for a multi-step explanation (causation, mechanism, sequence, walk-through, comparison) — the answer is the explanation itself, NOT any single name the user happened to mention in the question. To fix on retry: prefer UNSETTING `answer_subject` entirely (the auto-inference from `question_kind` will pick the correct shape), or pick a non-scalar kind only when the question literally asks for ONE named target (e.g. 'which function does X', 'which config key controls Y'). Distinguish the SUBJECT mentioned (an error name, a field, a route, a literal) from the ANSWER WANTED (the explanation of cause / mechanism / behaviour around that subject)."
}

// plainCoherenceDetail (note: per-segment loop below now also passes
// each segment through sanitizeInternalVocab from
// retry_hint_sanitize.go — B5 2026-05-10) strips internal rule
// codes ("R1.1", "R1.2",
// etc.) from the gate's Detail string and returns plain-language
// prose the LLM can act on. Pre-2026-04-30 the raw Detail flowed
// straight to the LLM hint and the codes confused models that had
// no internal documentation context.
//
// B.5 audit followup (2026-05-02): the gate may now emit multi-rule
// details joined by " | " when ≥2 rules fire (e.g. R1.4 + R1.5 on
// the same IR). Strip the prefix from EACH segment independently and
// rejoin so the LLM sees plain prose for every diagnostic.
func plainCoherenceDetail(detail string) string {
	d := strings.TrimSpace(detail)
	// B.5 multi-segment path: split on " | ", strip each piece, rejoin.
	// Single-segment path is byte-identical to the historical loop —
	// strings.Split on a string without " | " returns a 1-element slice.
	//
	// B5 (2026-05-10): each segment now ALSO passes through
	// sanitizeInternalVocab so dotted / Go-style internal token
	// shapes don't leak as code-identifier-shaped prose into the
	// LLM's retry hint (R6 red line direct remediation; forensic
	// anchor: 2026-05-10 chatpp-7d46dee4 log L1316
	// entities=[emit_analysis, is_cross_component]).
	segments := strings.Split(d, " | ")
	for i, seg := range segments {
		stripped := stripCoherencePrefix(strings.TrimSpace(seg))
		segments[i] = sanitizeInternalVocab(stripped)
	}
	return strings.Join(segments, " | ")
}

// stripCoherencePrefix is the per-segment helper for plainCoherenceDetail.
// Kept separate so the multi-segment path doesn't repeat the prefix list
// and a future rule addition (R1.6 / R2.3 / etc.) lands in one place.
func stripCoherencePrefix(d string) string {
	for _, prefix := range []string{
		// R1.1 carries two formats: hard-fail (legacy, retained for safety
		// in case future code re-promotes the rule) and the 2026-05-08
		// soft-advisory form. Strip both so the LLM never sees the
		// internal rule code in its retry hint.
		"R1.1 domain_divergence (advisory): ",
		"R1.1 domain_divergence: ",
		"R1.2 predicate_contradiction: ",
		"R1.3 entity_orphan: ",
		"R1.4 axis_collapse: ",
		"R1.5 entity_unresolvable (advisory): ",
		"R1.5 entity_unresolvable: ",
		"R1.6 completeness_obligation_missing: ",
		"R1.7 bucket_partition_missing: ",
		"R1.8 scope_anchor_distribution (advisory): ",
		"R2.1 scalar_multi_topic: ",
		"R2.2 explanation_scalar_subject: ",
	} {
		if strings.HasPrefix(d, prefix) {
			return strings.TrimPrefix(d, prefix)
		}
	}
	return d
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
func buildAnalyzerRepoOverview(ctx *types.AgentContext, objective string) (string, *repomap.Graph) {
	if types.IsREPLControlInput(objective) {
		return "", nil
	}
	if ctx != nil && ctx.Mutable != nil {
		if overview := renderAnalyzerAuthoritativeLogOverview(ctx.Mutable.LogTriage()); overview != "" {
			return overview, nil
		}
	}
	repoRoot := ""
	if ctx != nil {
		repoRoot = ctx.RepoRoot
	}
	if repoRoot == "" {
		return "", nil
	}
	// B4 (2026-05-10): multi-scope pre-inject. When the question names
	// ≥2 active sub-repo scopes verbatim (cross-sub-repo comparison
	// shape), render per-scope mini task_maps so the analyzer LLM
	// sees every named scope's structural context, not only the
	// pickPrimarySubRepo collapse target. Single-repo / single-scope
	// falls through to the legacy single-graph path below — byte-
	// identical for fixtures without ≥2 scope hits.
	// Design doc: docs/design/multirepo_entity_scope_separation.md §4.4.
	if scopes := detectScopesFromQuestion(ctx, objective); len(scopes) >= 2 {
		if rendered, primary := buildMultiScopeRepoOverview(ctx, objective, scopes); rendered != "" {
			return rendered, primary
		}
		// Multi-scope render returned empty (mg topology lookup failed,
		// EnsureLoaded errored, or every per-scope view was empty);
		// fall through to single-graph path below — caller never sees
		// the partial-fail.
	}
	// Extract code identifiers from the question to use as the graph
	// query. extractQuestionEntities pulls CamelCase/snake_case tokens
	// — exactly the kind of tokens that match file and symbol names.
	entities := extractQuestionEntities(objective)

	query := strings.Join(entities, " ")
	graph, err := repomap.GraphFromAgentContextOrLoad(ctx, repoRoot, query)
	if err != nil {
		logging.Debug("[analyzer] repo overview unavailable: %v", err)
		return "", nil
	}
	// C.1 audit followup (2026-05-02): if strict extraction came up
	// empty (purely-CJK question, all-lowercase short tokens, or
	// concept-only prose), fall back to the repomap-existence-gated
	// tokenizer. This admits any token that resolves to a real
	// Tier-1/2 symbol via NormalizeCodeKey lookup. Without this,
	// "what does this repo do" / "解释一下 explorer 模块的工作流程"
	// degrades to the un-ranked general overview, which forces the
	// LLM to emit concept-word entities downstream.
	if len(entities) == 0 {
		if fallback := extractQuestionEntitiesFallback(objective, graph); len(fallback) > 0 {
			entities = fallback
			query = strings.Join(entities, " ")
		}
	}
	caution := renderAnalyzerOverviewPrescanCaution(graph, objective)

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
		return caution, graph
	}
	// P4-cross-sub-repo (Sc 4, 2026-05-08): when multi-repo posture
	// (≥ 2 sub-repos), prepend a compact cross-sub-repo header so
	// analyzer's prompt sees every sub-repo's manifest + lang summary
	// — answers config-precedence / cross-sub-repo orientation
	// questions naturally without paying the deeper view's budget for
	// every sub-repo. Single-repo posture skips this section.
	if mg := repomap.MultiGraphFromAgentContext(ctx); mg != nil && !mg.IsSingle() {
		if header := renderMultiRepoOverviewHeader(mg); header != "" {
			output = header + "\n" + output
		}
	}
	// Cap the overview to keep the initial prompt bounded.
	const maxLen = 4096
	if len(output) > maxLen {
		output = output[:maxLen] + "\n... [truncated]\n"
	}
	logging.Debug("[analyzer] pre-injected %s view (%d bytes, entities=%v)", view, len(output), entities)
	if caution != "" {
		header += caution + "\n\n"
	}
	return header + output, graph
}

// buildMultiScopeRepoOverview renders one mini task_map view per
// active sub-repo named verbatim in the user's question (B4
// 2026-05-10). The total prompt budget (4096 bytes, matching the
// single-graph path's cap) is split across scopes; each scope gets
// a dedicated section so the analyzer LLM can decompose sub-topics
// per scope without paying the resolver's "primary sub-repo wins"
// asymmetry tax.
//
// Falls back to ("", nil) when:
//   - mg lookup fails (caller falls through to single-graph path)
//   - every per-scope view is empty (no relevant files / symbols)
//   - any scope's RootRel does not resolve to a topology SubRepo
//
// The first scope's *Graph is returned as the "primary" so the
// caller's Mutable.SearchGraph() set still has a valid pointer for
// downstream legacy consumers (analyzerRequiredFiles ranker etc.).
// This is a known partial-correctness mode — see multigraph_facade.go
// design note about caller-wrong-to-ask-for-the-graph in multi-repo
// mode. The multi-graph oracle / resolver paths (B2) DO see every
// active sub-repo's symbols so the gate signals stay correct.
//
// Design doc: docs/design/multirepo_entity_scope_separation.md §4.4.
func buildMultiScopeRepoOverview(ctx *types.AgentContext, objective string, scopes []string) (string, *repomap.Graph) {
	mg := repomap.MultiGraphFromAgentContext(ctx)
	if mg == nil {
		return "", nil
	}
	topo := mg.Topology()
	if topo == nil {
		return "", nil
	}
	const totalBudget = 4096
	perScope := totalBudget / len(scopes)
	if perScope < 800 {
		perScope = 800 // floor — too small and the per-scope view is useless
	}
	topN := 7
	if len(scopes) > 2 {
		topN = 5
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Repository overview (pre-computed for sub-repo scopes: %s)\n\n", strings.Join(scopes, ", ")))
	b.WriteString("The following per-sub-repo task_map shows files and symbols matching the question for each named sub-repo. Use this to inform your sub-topic decomposition and pre-scan targets. You may still call repo_map, grep, or list_files for additional verification.\n\n")
	if header := renderMultiRepoOverviewHeader(mg); header != "" {
		b.WriteString(header)
		b.WriteString("\n")
	}
	var primaryGraph *repomap.Graph
	rendered := 0
	for i, scope := range scopes {
		sr := topo.SubRepoByRootRel(scope)
		if sr == nil {
			continue
		}
		g, err := mg.EnsureLoaded(sr.Slug)
		if err != nil || g == nil {
			logging.Debug("[analyzer] multi-scope: EnsureLoaded(%s) failed: %v", sr.Slug, err)
			continue
		}
		if i == 0 || primaryGraph == nil {
			primaryGraph = g
		}
		view := repomap.GenerateView(g, "task_map", repomap.ViewParams{
			Query: objective,
			TopN:  topN,
		})
		if view == "" {
			continue
		}
		if len(view) > perScope {
			view = view[:perScope] + "\n... [truncated]\n"
		}
		fmt.Fprintf(&b, "\n# Task Map: %s\n\n%s\n", scope, view)
		rendered++
	}
	if rendered == 0 {
		return "", nil
	}
	out := b.String()
	// Outer cap: same +512 slack we let the single-graph path keep
	// for the prepended multi-repo header. Truncation is a last
	// resort — the per-scope per-call cap above usually keeps us
	// well under this ceiling.
	if max := totalBudget + 512; len(out) > max {
		out = out[:max] + "\n... [truncated]\n"
	}
	logging.Debug("[analyzer] pre-injected multi-scope view (%d bytes, scopes=%v rendered=%d)", len(out), scopes, rendered)
	return out, primaryGraph
}

// renderMultiRepoOverviewHeader builds the compact "## Multi-repo
// overview" section that buildAnalyzerRepoOverview prepends when the
// active topology has ≥ 2 sub-repos. Each sub-repo gets one line
// with RootRel, top-3 PrimaryLangs, FileCount, and the count of
// recognised manifest/special files (go.mod / Cargo.toml /
// codrax.yaml / etc.). Sub-repo prefix on SpecialFiles lets the LLM
// see "sub-a/codrax.yaml" vs "sub-b/codrax.yaml" verbatim — answers
// cross-sub-repo config-precedence questions without further drilling.
//
// Returns "" when topology is unavailable, IsSingle, or zero
// sub-repos. Caller falls through to per-graph rendering only.
func renderMultiRepoOverviewHeader(mg *multigraph.MultiGraph) string {
	if mg == nil || mg.IsSingle() {
		return ""
	}
	subs := mg.SubRepos()
	if len(subs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Multi-repo overview (cross-sub-repo orientation)\n\n")
	b.WriteString(fmt.Sprintf("This workspace has %d sub-repos discovered under the parent root. The deeper view below is for the most-relevant sub-repo per the routing fold; cross-sub-repo questions should consult the sub-repo summaries here.\n\n", len(subs)))
	md := mg.Metadata()
	for _, sr := range subs {
		langs := strings.Join(sr.PrimaryLangs, ",")
		if langs == "" {
			langs = "-"
		}
		b.WriteString(fmt.Sprintf("- `%s` — langs=%s files=%d\n", sr.RootRel, langs, sr.FileCount))
	}
	if len(md.SpecialFiles) > 0 {
		b.WriteString("\n**Cross-sub-repo manifest files** (sub-repo prefix preserved for precedence reasoning):\n")
		preview := md.SpecialFiles
		if len(preview) > 16 {
			preview = preview[:16]
		}
		for _, sf := range preview {
			b.WriteString(fmt.Sprintf("- `%s`\n", sf))
		}
		if len(md.SpecialFiles) > len(preview) {
			b.WriteString(fmt.Sprintf("- _… and %d more_\n", len(md.SpecialFiles)-len(preview)))
		}
	}
	if pending := mg.PendingSubRepoNames(); len(pending) > 0 {
		b.WriteString(fmt.Sprintf("\n_Note: routing currently inactive on sub-repos: %s. Use `/repos focus <slug>` to pin._\n", strings.Join(pending, ", ")))
	}
	b.WriteString("\n---\n")
	return b.String()
}

// analyzerOracleFromCtx returns the right SymbolOracle for the
// current Run: cross-sub-repo fan-out via ctx.MultiGraph when
// multi_repo_enabled, else the legacy single-graph oracle wrapping
// `graph`. Single-repo posture (mg.IsSingle()) returns the same
// per-graph oracle as the legacy path — byte-identical.
//
// P4-cross-sub-repo (2026-05-08) — eliminates the false-negative
// where a log/perf-triage entity defined in a non-routed sub-repo
// (e.g., a panic stack frame's Symbol from sub-b while routing
// landed on sub-a) was rejected by MergeEntities' oracle gate.
func analyzerOracleFromCtx(ctx *types.AgentContext, graph *repomap.Graph) types.SymbolOracle {
	if mg := repomap.MultiGraphFromAgentContext(ctx); mg != nil {
		return mg.Oracle()
	}
	return repomap.NewSymbolOracle(graph)
}

func renderAnalyzerAuthoritativeLogOverview(bundle *types.LogBundle) string {
	if !logBundleAuthoritativeFrames(bundle) || bundle == nil || len(bundle.Errors) == 0 {
		return ""
	}
	seenFiles := map[string]bool{}
	seenFrames := map[string]bool{}
	var files []string
	var frames []string
	for _, err := range bundle.Errors {
		for _, frame := range err.Frames {
			file := strings.TrimSpace(strings.ReplaceAll(frame.File, `\`, `/`))
			if file == "" || frame.Line <= 0 {
				continue
			}
			if !seenFiles[file] {
				seenFiles[file] = true
				files = append(files, file)
			}
			label := strings.TrimSpace(frame.Func)
			if label == "" {
				label = file
			} else {
				label = fmt.Sprintf("%s :: %s", file, label)
			}
			if seenFrames[label] {
				continue
			}
			seenFrames[label] = true
			frames = append(frames, label)
		}
	}
	if len(files) == 0 {
		return ""
	}
	sort.Strings(files)
	var b strings.Builder
	b.WriteString("## Authoritative runtime anchors (pre-computed, no broad repo overview needed)\n\n")
	b.WriteString("The attached crash / panic log already resolves to current-repo file+function anchors. Treat these as the file-set ceiling for analysis: do NOT promote same-name symbols from other files into candidate owners unless these anchors themselves fail to ground.\n\n")
	b.WriteString("Resolved files:\n")
	for _, file := range files {
		fmt.Fprintf(&b, "- `%s`\n", file)
	}
	if len(frames) > 0 {
		b.WriteString("\nGrounded frame targets:\n")
		for _, frame := range frames {
			fmt.Fprintf(&b, "- `%s`\n", frame)
		}
	}
	b.WriteString("\nUse the authoritative file/function anchors above first. If you need more structure after that, read those files directly; do not treat repo-wide same-name matches as equivalent evidence.\n")
	return b.String()
}

func renderAnalyzerOverviewPrescanCaution(graph *repomap.Graph, objective string) string {
	if graph == nil {
		return ""
	}
	tokens := extractAnalyzerOverviewCautionTokens(objective)
	if len(tokens) == 0 {
		return ""
	}
	type caution struct {
		Token string
		Kind  string
		Paths []string
	}
	var cautions []caution
	for _, token := range tokens {
		finding := prescan.ClassifyToken(graph, "", token, true)
		switch finding.Status {
		case prescan.TokenStatusPrimary:
			continue
		case prescan.TokenStatusAuxiliaryOnly:
			cautions = append(cautions, caution{Token: token, Kind: "auxiliary_only", Paths: finding.Paths})
			continue
		case prescan.TokenStatusUnresolved:
			if len(finding.Paths) == 0 {
				files := analyzerTopFilesForQuery(graph, token, 4)
				paths := make([]string, 0, len(files))
				for _, fi := range files {
					if fi == nil || strings.TrimSpace(fi.RelPath) == "" {
						continue
					}
					paths = append(paths, fi.RelPath)
				}
				if len(paths) == 0 {
					cautions = append(cautions, caution{Token: token, Kind: "unresolved"})
					continue
				}
				cautions = append(cautions, caution{Token: token, Kind: "unresolved", Paths: paths})
				continue
			}
			cautions = append(cautions, caution{Token: token, Kind: "unresolved", Paths: finding.Paths})
			continue
		default:
			cautions = append(cautions, caution{Token: token, Kind: "unresolved"})
		}
	}
	if len(cautions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Identifier Pre-scan Cautions\n\n")
	b.WriteString("Some identifier-shaped mentions from the request are not yet backed by a production repo anchor. Treat them as search leads, not as proof, until `grep` / `read_file` finds a non-auxiliary repo definition.\n")
	b.WriteString("- Do not let auxiliary-only hits in docs / tests / examples upgrade a token into production proof, a positive exact match, or a substitute answer.\n")
	b.WriteString("- Keep user-mentioned exact targets as unverified leads until a non-auxiliary file grounds them; if that grounding never appears, prefer an honest not-found / absence path over nearby replacements.\n")
	b.WriteString("- Do not promote nearby files or symbols from these cautions into new `exact_targets` unless the user also named them explicitly.\n")
	for _, c := range cautions {
		switch c.Kind {
		case "auxiliary_only":
			paths := c.Paths
			if len(paths) > 3 {
				paths = paths[:3]
			}
			fmt.Fprintf(&b, "- `%s`: current pre-scan hits are auxiliary-only (%s) — keep this token unverified until `grep` / `read_file` finds a non-auxiliary definition.\n", c.Token, strings.Join(paths, ", "))
		case "unresolved":
			fmt.Fprintf(&b, "- `%s`: no current exact production hit in the repo graph — treat it as a search lead, not a proof-bearing anchor.\n", c.Token)
		}
	}
	return strings.TrimSpace(b.String())
}

func extractAnalyzerOverviewCautionTokens(objective string) []string {
	seen := make(map[string]bool)
	var out []string
	scanQuestionTokens(objective, func(tok string, src tokenSource) {
		tok = strings.TrimSpace(strings.Trim(tok, "(){}[]?!.,;:'\""))
		if len(tok) < 4 {
			return
		}
		qualifies := src == tokenBacktick || strings.ContainsAny(tok, "_./\\")
		if !qualifies {
			for _, r := range tok {
				if unicode.IsUpper(r) {
					qualifies = true
					break
				}
			}
		}
		if !qualifies {
			return
		}
		key := strings.ToLower(strings.ReplaceAll(tok, `\`, `/`))
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, tok)
	})
	return out
}

func analyzerTopFilesForQuery(graph *repomap.Graph, query string, topN int) []*repomap.FileInfo {
	if graph == nil || strings.TrimSpace(query) == "" {
		return nil
	}
	ranking := retrieve.RankGraphScores(graph, query)
	files := retrieve.TopFilesByScore(graph, ranking.Scores, topN)
	out := make([]*repomap.FileInfo, 0, len(files))
	for _, fi := range files {
		if fi == nil || strings.TrimSpace(fi.RelPath) == "" {
			continue
		}
		if ranking.Scores[fi.RelPath] <= 0 {
			continue
		}
		out = append(out, fi)
	}
	return out
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

// FilterToolSchemas keeps the analyzer's runtime tool surface aligned
// with the evidence-lite contract, even when MCP tools or future
// skill edits accidentally expose broader read tools. Normal analyze
// turns may see only emit_analysis plus the three navigation pre-scan
// tools. Once the pre-scan budget is closed, the next LLM request is
// physically emit-only instead of relying on prompt prose to stop
// another grep/repo_map/list_files call.
func (e *analyzerEvaluator) FilterToolSchemas(ctx *types.AgentContext, schemas []llm.ToolSchema) []llm.ToolSchema {
	if ctx == nil || ctx.Stage != types.StageAnalyze || len(schemas) == 0 {
		return schemas
	}
	emitOnly := analyzerTerminalEmitOnly(ctx)
	out := make([]llm.ToolSchema, 0, len(schemas))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Name)
		if emitOnly {
			if name == "emit_analysis" {
				out = append(out, schema)
			}
			continue
		}
		if isAnalyzerStageAllowedTool(name) {
			out = append(out, schema)
		}
	}
	return out
}

// Observe enforces the pre-scan budget at runtime. The final legal
// pre-scan round injects a must-emit hint; after that the schema filter
// exposes only emit_analysis. If a provider still returns a blocked
// pre-scan tool call, the structured repair result gets one emit-only
// correction turn instead of immediately failing the whole analyze
// attempt. A genuinely executed over-budget pre-scan is still a hard
// stop, preserving the fail-loud contract for impossible states.
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
	if obs.LastToolResult.Repair != nil {
		switch obs.LastToolResult.Repair.Code {
		case analyzerPrescanBudgetReachedCode, analyzerPrescanTerminalEmitModeCode:
			return LoopSignal{
				HintRequested: true,
				HintKey:       "analyzer.emit-only.after-prescan-reject",
				Hint:          "The pre-scan budget is closed. Do not call repo_map, grep, list_files, read_file, or any other tool. The next response must call emit_analysis exactly once with the best classification you already have; unresolved targets belong in the structured fields for explore to verify.",
			}
		case analyzerToolNotAllowedCode:
			return LoopSignal{
				HintRequested: true,
				HintKey:       "analyzer.tool-boundary",
				Hint:          "That tool is outside the analyze-stage boundary. Analyze is classification-only: use only repo_map, grep(files_only=true), list_files for light location checks, or call emit_analysis now. Do not call read_file or content-reading tools in analyze.",
			}
		}
	}
	if !isPrescanTool(obs.LastToolResult.ToolName) {
		return LoopSignal{}
	}
	e.prescanRounds++

	if ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.AppendPrescanSummary(obs.LastToolResult.Summary)
	}

	// The old ClassificationGrep Round-2 line peek is intentionally not
	// opened here anymore. Analyze remains evidence-lite: repo_map,
	// list_files, and grep(files_only=true) establish existence/location;
	// source-line evidence belongs to explore.

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
			"Pre-scan budget reached (%d of %d rounds used). Your NEXT response MUST call emit_analysis with the fields you have — any additional prescan tool call (repo_map / grep / list_files), even batched with emit_analysis, will exhaust the budget and fail the analyze stage. Put unresolved but relevant targets in the structured fields and let explore gather line-level evidence.",
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
			// R8 (post_shape_residual_audit.md, 2026-05-04): surface
			// the gate failure on the AnalyzerDecision channel so the
			// end-of-Run operator summary catches the silent retry
			// (otherwise only the [analyzer-v3] ERROR log line + the
			// next dispatch's WARN "analyze attempt N/M failed" trail
			// hint at it).
			if mut := ctxMutable(ctx); mut != nil {
				mut.AppendAnalyzerDecision(types.AnalyzerDecisionSignal{
					Kind:   "quality_gate_hard_fail",
					Stage:  string(types.StageAnalyze),
					Reason: detail,
				})
			}
			// Coherence-specific feedback: when the failing checks
			// are the cross-signal coherence gates, render an IR-
			// field-level retry hint so the next dispatch's directive
			// shows the LLM what specific contradiction to fix.
			//
			// Commit 58 Batch D fix (audit #2): for NON-coherence hard
			// failures (coverage / dag_closure / budget_sanity /
			// contract_complete / hypothesis_coverage / etc), thread
			// the full joined gate-failure detail (commit 56's union)
			// into AnalyzerRetryHint so the next dispatch's LLM sees
			// the specific check + Detail strings instead of a generic
			// "rejected" message. Pre-commit-58 these landed only in
			// `out.Error` (log + final exhaustion message) and never
			// reached the prompt — wasted retries until the LLM
			// happened to guess the right axis to widen.
			if ctx != nil && ctx.Mutable != nil {
				baseHint := ""
				if hint := composeCoherenceRetryHint(ir.QualityGate); hint != "" {
					baseHint = hint
				} else {
					baseHint = buildGenericGateRetryHint(detail)
				}
				// L3-T4 (2026-05-10) — append a per-file-confidence
				// nudge when the LLM's prior emit included low- or
				// borderline-confidence required_files. This is
				// purely advisory; the LLM is free to keep its prior
				// recommendations. Only fires when there's
				// something to comment on (no echo for empty or
				// unanimously-confident hint sets).
				if extra := composeRequiredFileHintsRetryAdvice(ir); extra != "" {
					if baseHint != "" {
						baseHint = baseHint + "\n\n" + extra
					} else {
						baseHint = extra
					}
				}
				ctx.Mutable.SetAnalyzerRetryHint(baseHint)
			}
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

// EstimateSubTopicCount heuristically detects the number of
// independent sub-topics in a question string by counting question
// marks (Chinese and ASCII) and numbered list patterns (e.g. "1. ",
// "2) "). Returns at least 1.
func EstimateSubTopicCount(text string) int {
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
	// v3.1 (2026-05-05): the regex-driven enumeration-boundary
	// recovery path was deleted as a keyword-table band-aid (see
	// internal/types/enumeration_boundary.go header). EnumerationBoundary
	// now sources solely from the analyzer LLM's emit_analysis output.
	// When the LLM omits it the question routes through whatever family
	// matches (typically QFGeneric / QFArchitecture / etc.) — honest
	// weakness over dishonest confidence.
	graph := analyzerGraphForNormalize(ctx, rm)
	if resolved, reason := reconcileEnumerationBoundaryScope(rm, graph); reason != "" {
		logging.Debug("[analyzer] enumeration-boundary reconcile: %s", reason)
		recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
			"enumeration_boundary",
			fmt.Sprintf("sub_topics=%d entities=%d", len(rm.SubTopics), len(rm.AnalyzerHints.Entities)),
			fmt.Sprintf("sub_topics=%d entities=%d", len(resolved.SubTopics), len(resolved.AnalyzerHints.Entities)),
			rm.KindConfidence,
			reason,
			rm.Predicates,
		))
		rm = resolved
	}

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
	//   - Structured observations remain on rm.LogTriage for prompt,
	//     answer-surface, and advisory IntentHint consumers; they do
	//     not hard-override user intent.
	//   - analyzerRequiredFiles: reads bundle.ResolvedFiles as the first-class
	//     file seed, merged with the structural ranker output.
	var logBundle *types.LogBundle
	if ctx.Mutable != nil {
		logBundle = ctx.Mutable.LogTriage()
	}
	if logBundle != nil {
		before := len(rm.AnalyzerHints.Entities)
		if shouldMergeLogTriageEntities(logBundle) {
			// Commit 52 P1: oracle gates entity merge against repomap
			// symbol set. nil graph (single-shot CLI without scan) =
			// nil oracle = pre-commit-52 byte-identical behaviour.
			oracle := analyzerOracleFromCtx(ctx, graph)
			rm.AnalyzerHints.Entities = logtriage.MergeEntities(
				rm.AnalyzerHints.Entities, logBundle.Entities, oracle)
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
		// Commit 52 P1: same oracle gate for perf-triage entities.
		oracle := analyzerOracleFromCtx(ctx, graph)
		rm.AnalyzerHints.Entities = logtriage.MergeEntities(
			rm.AnalyzerHints.Entities, perfBundle.Entities, oracle)
		logging.Info("[analyzer] perf-triage: source=%s frames=%d janks=%d stalls=%d entities +%d intent=%q",
			perfBundle.Meta.Source, len(perfBundle.Frames), len(perfBundle.Janks),
			len(perfBundle.Stalls),
			len(rm.AnalyzerHints.Entities)-before,
			perfBundle.IntentHint)
	}

	// 2026-05-02 — attach the validated bundles onto the
	// RequestModel so cross-package deciders (compiler / hdp / gate /
	// criterion / request_traits) can read multi-frame artifact
	// evidence without re-threading the bundle pointers through every
	// signature. nil when the user did not attach --log / --htrace /
	// --atrace. See the LogTriage / PerfTrace fields in
	// types.RequestModel for the contract — these are excluded from
	// JSON serialisation so the LLM-emit wire shape stays unchanged.
	rm.LogTriage = logBundle
	rm.PerfTrace = perfBundle

	// Sub-topics post-processing: when the LLM detected multiple
	// independent sub-topics, lift complexity so downstream budgets
	// expect multi-topic surface area. The pre-shape variant of this
	// block also forced answer_shape=explanation; that override is
	// retired with AnswerShape — the V2 block-only carrier carries
	// multi-topic structure via AnswerSemanticView, not a shape enum.
	if len(rm.SubTopics) > 5 {
		rm.SubTopics = rm.SubTopics[:5]
		logging.Warning("[analyzer] sub_topics truncated to 5")
	}
	if len(rm.SubTopics) > 1 {
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
		predsResolved, predsReason := reconcileSemanticPredicates(rm)
		if predsResolved != rm.Predicates {
			logPredicateReconcile(rm.Predicates, predsResolved, predsReason)
			recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
				"predicates",
				fmt.Sprintf("%t", rm.Predicates.IsCrossComponent),
				fmt.Sprintf("%t", predsResolved.IsCrossComponent),
				0,
				predsReason,
				predsResolved,
			))
			rm.Predicates = predsResolved
		}
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
		// AnswerSubject when present; otherwise applies the typed
		// question_kind fallback in analyzer_intent.go::inferAnswerSubject.
		// The chain ranker uses the resolved subject to demote chains
		// whose terminal is the wrong kind; answer presentation no
		// longer runs a legacy shape override off this field.
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
		if resolved, reason := reconcileDiagnosticQuestionProfile(rm); reason != "" {
			recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
				"diagnostic_profile",
				fmt.Sprintf("intent=%s scenario=%s", rm.Intent, rm.Scenario),
				fmt.Sprintf("intent=%s scenario=%s", resolved.Intent, resolved.Scenario),
				0,
				reason,
				resolved.Predicates,
			))
			logging.Info("[analyzer] diagnostic profile reconciled: %s", reason)
			rm = resolved
		}
		scenarioResolved, scenarioReason := reconcileScenario(rm)
		if scenarioReason != "" {
			logScenarioReconcile(rm.Scenario, scenarioResolved, scenarioReason)
			recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
				"scenario", string(rm.Scenario), string(scenarioResolved),
				0, scenarioReason, rm.Predicates,
			))
			// R8 (post_shape_residual_audit.md, 2026-05-04): also
			// record an end-of-Run operator-facing signal so the
			// scenario flip is visible in the Run summary tooling
			// (not just the [analyzer] WARN log line).
			if mut := ctxMutable(ctx); mut != nil {
				mut.AppendAnalyzerDecision(types.AnalyzerDecisionSignal{
					Kind:   "scenario_reconciled",
					Stage:  string(types.StageAnalyze),
					Before: string(rm.Scenario),
					After:  string(scenarioResolved),
					Reason: scenarioReason,
				})
			}
			rm.Scenario = scenarioResolved
		}
		if resolved, capabilityQuery, reason := reconcileStageToolCapabilitySurface(rm); capabilityQuery != nil {
			recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
				"capability_surface",
				fmt.Sprintf("subject=%s scenario=%s keywords=%d", rm.AnswerSubject.Kind, rm.Scenario, len(rm.AnalyzerHints.Keywords)),
				fmt.Sprintf("subject=%s scenario=%s keywords=%d", resolved.AnswerSubject.Kind, resolved.Scenario, len(resolved.AnalyzerHints.Keywords)),
				resolved.AnswerSubject.Confidence,
				reason,
				resolved.Predicates,
			))
			logging.Debug("[analyzer] capability-surface reconcile: stage=%s agent=%s skill=%s tool=%s",
				capabilityQuery.Binding.Stage, capabilityQuery.Binding.Agent, capabilityQuery.Binding.Skill, capabilityQuery.Tool)
			rm = resolved
		}
		if resolved, reason := reconcileChangeImpactProfile(rm); reason != "" {
			recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
				"change_impact_profile",
				changeImpactClassificationSummary(rm),
				changeImpactClassificationSummary(resolved),
				resolved.KindConfidence,
				reason,
				resolved.Predicates,
			))
			logging.Info("[analyzer] change-impact profile reconciled: %s", reason)
			rm = resolved
		}
		if resolved, reason := reconcileQualifiedCodeSymbolConfigDrift(rm, analyzerGraphForNormalize(ctx, rm)); reason != "" {
			recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
				"qualified_code_symbol_config_drift",
				fmt.Sprintf("intent=%s scenario=%s kind=%s subject=%s axis=%s", rm.Intent, rm.Scenario, rm.AnalyzerHints.Kind, rm.AnswerSubject.Kind, rm.PredicateAxis),
				fmt.Sprintf("intent=%s scenario=%s kind=%s subject=%s axis=%s", resolved.Intent, resolved.Scenario, resolved.AnalyzerHints.Kind, resolved.AnswerSubject.Kind, resolved.PredicateAxis),
				resolved.KindConfidence,
				reason,
				resolved.Predicates,
			))
			logging.Info("[analyzer] qualified code-symbol config drift reconciled: %s", reason)
			rm = resolved
		}
		// Measurement-scalar signal — captures the reconciled-Intent
		// case (LLM picked enumerate, reconcileIntent downgraded to
		// return_value via IsCountQuestion), the LLM-direct case
		// (IntentReturnValue + IsCountQuestion true on first emit), AND
		// the structural-coherence fallback (scalar-answer + intent=
		// return_value + answer_subject.kind=numeric co-occur even
		// when IsCountQuestion slipped through as false). Computed
		// after inferAnswerSubject so the fallback sees the inferred
		// subject kind. Every consequence (citation-gate strips and
		// scalar handling) is applied in one post-compile block below,
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
	//
	// The resolver is hoisted into a local so the same instance flows
	// to gate.RunWith below, where R1.4 (axis_collapse) and R1.5
	// (entity_unresolvable) query it to validate sub-topic entity
	// claims against repo ground truth.
	// B2 (2026-05-10): in multi-repo posture, wrap the multigraph
	// fan-out resolver so R1.4/R1.5 in the coherence gate (and the
	// normalizer's kindEnWord → TermSymbol promotion gate) see every
	// active sub-repo's SymbolDefs, not just the primary collapsed
	// from pickPrimarySubRepo. Single-repo / nil-multigraph keeps the
	// legacy single-graph adapter — byte-identical pre-multi-repo.
	// Design doc: docs/design/multirepo_entity_scope_separation.md §4.2.
	resolver := analyzerSymbolResolver(ctx, rm)
	rm.TermGraph = normalizer.Normalize(
		rm.RawRequest,
		normalizer.Options{
			Resolver: resolver,
			Entities: rm.AnalyzerHints.Entities,
		},
	)

	// Amplifier pre-compile pass — fills LLM-omitted optional typed
	// slots (Predicates.IsCategoryEnumeration / SubTopics) using purely
	// structural signals on rm (TermGraph kinds + confidence shape,
	// Intent, AnswerSubject, Entities). Must run AFTER normalizer.Normalize
	// because R1 reads rm.TermGraph.Canonical, which is empty until
	// normalizer populates it. Runs BEFORE compiler.InferScenario /
	// compiler.Compile so the scenario template selector picks on
	// amplifier-corrected predicates.
	//
	// Tradeoff vs the original design (which placed Amplify before the
	// reconcile chain): R1 fires AFTER reconcileComplexity, so a flipped
	// IsCategoryEnumeration no longer triggers the simple→moderate
	// complexity upgrade in analyzer_complexity.go. The compiler still
	// picks the correct enumeration template via the augmented predicate;
	// the lost upgrade is a nice-to-have, not load-bearing.
	{
		amplified, ampObs := amplifier.Amplify(rm)
		for _, obs := range ampObs {
			recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
				obs.Field, obs.Before, obs.After, 0, obs.Reason, amplified.Predicates,
			))
		}
		rm = amplified
	}

	// L0-B-pre (2026-05-06) — Implementers expansion for enumeration
	// questions whose entity is an interface/trait/protocol declaration.
	//
	// Pre-fix: questions like "list all implementers of LoopController" were
	// classified as IsCategoryEnumeration=true (correctly — the answer is a
	// set), but at analyze time the LLM has not read code yet so it can only
	// name the interface itself, populating entities=[LoopController]. The
	// L0-B gate below then rejects (distinctNamedEntities ≤ 1) and the
	// analyze stage exhausts its retry budget with no answer.
	//
	// Fix: when the analyzer's classification declares enumeration intent
	// AND the named entity is a known interface/trait/protocol Symbol in
	// the repo graph, expand AnalyzerHints.Entities with the typed list of
	// implementer names from Graph.ImplementersOf. The graph primitive is
	// language-agnostic (populateImplementers post-pass handles Go's
	// structural method-set check, Java/Kotlin's `implements`, Rust's
	// `impl Trait`, Python's ABC subclass list, etc.) and is the same
	// signal explorer.implementerFilesFromGraph already uses, so this
	// keeps the typed-signal contract that L0-B requires.
	//
	// Empty / non-interface-shaped entities short-circuit so the
	// expansion is a no-op for any case it cannot ground precisely.
	rm.AnalyzerHints.Entities = expandEntitiesWithImplementers(ctx, rm)
	rm.AnalyzerHints.PrimaryEntities = promoteSubTopicFileAnchorToPrimary(ctx, rm)

	// Multi-repo scope projection — typed lane parallel to PrimaryEntities.
	// COPY semantics: the matched sub-repo names stay in PrimaryEntities
	// (legacy consumers untouched) AND get an additional typed home in
	// PrimaryScopes / SubTopic.Scopes. Empty in single-repo / nil-multigraph
	// posture so this is byte-additive on the JSON wire (omitempty).
	// Design doc: docs/design/multirepo_entity_scope_separation.md §4.1.
	rm.AnalyzerHints.PrimaryScopes = projectPrimaryScopes(ctx, rm.AnalyzerHints.PrimaryEntities)
	projectSubTopicScopes(ctx, rm.SubTopics)

	// L0-B (2026-05-05) — Enumeration cardinality structural sanity.
	// When the analyzer LLM (or amplifier R1 flip) declares the
	// question as IsCategoryEnumeration=true but only emitted ≤1
	// distinct named entity, the emit is structurally self-
	// contradictory: enumerate-one-thing has no answer shape. The
	// likely cause is the LLM picking the enclosing TYPE name
	// (e.g. PipelineStage) instead of the enumerated VALUES (e.g.
	// StageAnalyze / StageExplore / ...). Fail-loud here so the
	// analyzer's MaxRetriesPerStage budget retries with an explicit
	// fix hint; LLM almost always re-emits the enumerated values
	// on the next attempt because the reject message names the
	// fix exactly.
	//
	// Precise-signal rationale: rm.Predicates.IsCategoryEnumeration
	// and len(rm.AnalyzerHints.Entities) are both typed precise
	// slots. The combination cat=true && distinct ≤ 1 is itself a
	// precise structural matrix — not a heuristic — so this hard
	// gate satisfies the "precise signals for hard gates" red line.
	//
	// Empirical: 2026-05-05 qf_arch run-1 (Phase 4.2 eval) emitted
	// entities=[PipelineStage] with cat=false, then later runs in
	// the same case emitted entities=[StageAnalyze, StageExplore,
	// StageExtract, StageFinalize] with cat=true and PASSed. This
	// gate catches the cat=true variant of the entity-1 trap; the
	// cat=false variant (genuine type-name lookup) legitimately
	// passes through.
	//
	// 2026-05-08 — IsRelationalLookup carve-out. The gate above was
	// over-firing on a structurally distinct question shape: "filter
	// set X by a relationship to Y" (the LLM-emitted definition of
	// `is_relational_lookup`). Concrete cases:
	//   - "哪些包 import 了 internal/analysis/criterion?"  (set =
	//     packages, relation = imports, target = criterion)
	//   - "如果删除 internal/tool/ground 哪些文件无法编译?" (set =
	//     files, relation = depends-on, target = ground)
	//   - "criterion 包导出哪些 API?"  (set = symbols, relation =
	//     exported-by, target = criterion)
	// In each case the entity IS the relation target; the values to
	// be enumerated (importing packages / dependent files / exported
	// symbols) are computable only by exploration, not from the
	// LLM's training context. Forcing the LLM to emit values at
	// classification time is structurally impossible — it has not
	// read code yet. Pre-fix the gate exhausted retries on these
	// shapes (3 attempts × same reject message) and the run died
	// with no answer.
	//
	// IsRelationalLookup is an existing typed predicate the LLM
	// already emits (see internal/types/analysis_ir.go:381) — no
	// new signal, no R2' six-spot sync. The carve-out skips the
	// cardinality reject when the LLM affirms the question is
	// "set filtered by relation". The original target of L0-B (LLM
	// confused enclosing TYPE for enumerated VALUES) does NOT carry
	// IsRelationalLookup=true because that error shape names a
	// CATEGORY wrapper, not a relation criterion. So the carve-out
	// preserves L0-B's true-positive coverage while closing the
	// false-positive on relational lookups.
	if rm.Predicates.IsCategoryEnumeration &&
		!rm.Predicates.IsRelationalLookup &&
		distinctNamedEntities(rm.AnalyzerHints.Entities) <= 1 {
		return nil, fmt.Errorf(
			"analyzer: enumeration intent with ≤1 distinct named entity is structurally inconsistent — " +
				"is_category_enumeration=true means the user is asking 'what kinds/types exist', " +
				"so entities must list the actual ENUMERATED MEMBERS (e.g. each implementer name, " +
				"each enum case name, each registered handler name), not the enclosing TYPE name. " +
				"Re-emit emit_analysis ONCE with entities populated with the actual enumerated " +
				"members the user is asking about, OR set is_category_enumeration=false if the " +
				"question is really a type-name / scalar lookup. " +
				"Note: if the question filters a set by a relationship to a named target " +
				"(e.g. 'which packages import X', 'list X's exports', 'X 的子包' / " +
				"'X 下的子目录' / 'list child packages of X' / 'list APIs exported by X' / " +
				"'X 包导出哪些'), set is_relational_lookup=true alongside " +
				"is_category_enumeration=true — the values are looked up after classification, " +
				"not stated by you at this step")
	}

	// Session 11 C0' classification reconcile retired with AnswerShape:
	// the rule existed solely to nudge AnalyzerHints.Shape toward
	// "value" when Round-2 grep observed a quoted literal. With shape
	// retired, the rule has no observable effect — V2 carriers express
	// "this answer is a single resolved literal" via AnswerSubject.Kind
	// + AnswerSemanticView, both of which are derived from the LLM's
	// emit_analysis output, not from a scan of incidental quoted
	// strings.

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

	rm.AnalyzerHints.DerivedEntities = types.DerivedEntitiesFromMentioned(
		rm.AnalyzerHints.Entities, rm.AnalyzerHints.MentionedEntities)

	// Build the typed observation profile after reconciliation and
	// entity expansion so no-attachment diagnostics see the final
	// diagnostic flags, subject inference, and prior-derived entity
	// hints. Earlier log/trace bundle attachment stays read-only;
	// this is the single profile consumed by TaskGraph/finalizer.
	rm.ArtifactObservationProfile = types.BuildArtifactObservationProfileForRequest(rm)

	// Scenario default.
	if rm.Scenario == "" || (rm.Scenario == types.ScenarioGeneric &&
		!isScalarSourceLiteralLookup(rm) &&
		!types.HasCapabilitySurfaceHint(rm)) {
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
	if out.AnswerContract.Language == "" {
		out.AnswerContract.Language = rm.Language
	}

	// Risk matrix and hypothesis planning.
	rm.RiskMatrix = risk.Evaluate(rm, rm.RiskMatrix)
	hypotheses := hdp.Plan(rm)

	// Recompute budget with the real hypothesis count.
	sig.HypothesisCount = len(hypotheses)
	compiler.RecomputeBudget(&out, rm, sig)

	// Amplifier post-compile pass — pins typed identifiers into
	// AnswerContract.MustInclude when the request is an enumeration
	// over named entities. Runs AFTER compiler.Compile populates
	// AnswerContract and BEFORE binder.BindByRelevance so any binder
	// pass over MustInclude sees the augmented set. Phase 1.2 wires
	// the empty rule registry; Phase 4 lands R3.
	for _, obs := range amplifier.AmplifyPostCompile(rm, &out.AnswerContract) {
		recordReconcileObservation(ctxMutable(ctx), reconcileEvent(
			obs.Field, obs.Before, obs.After, 0, obs.Reason, rm.Predicates,
		))
	}

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

	// Citation-free carve-outs. A count question ("how many X",
	// "统计 …") produces a scalar answer from a tool query
	// (find | wc -l, list_files count, grep count) with no file:line
	// to cite. A repository-history lookup answers from VCS metadata,
	// not source lines. An external-only runtime artifact (log/trace
	// decoded successfully but no frame resolved to this checkout)
	// is answer-grade as an observation, but its facts must be
	// rendered with citation_ref=-1 rather than by hunting for
	// unrelated current-repo fixtures.
	//
	// The isMeasurementScalar signal (computed by
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
	// Consequences: strip the three citation gate surfaces that all
	// consult CritCitationCountGE independently — leaving any one
	// enabled loops the retry budget on a mismatch no amount of
	// re-investigation can fix:
	//
	//   (a) AnswerContract.CitationReq           → contract.checkCitations
	//   (b) AnswerContract.AcceptanceTests       → contract.checkAcceptance
	//   (c) TaskNode.SuccessCriteria (finalize)  → orchestrator.markSuccessCriteriaFailed
	//
	// The measurement-scalar / history-lookup carve-out is signalled
	// downstream via Predicates.IsScalarAnswer (read by builder.go's
	// citation-free Raw Tool Outputs gate). External-only runtime
	// artifact disposition is signalled via RequestModel.LogTriage /
	// PerfTrace and AnswerSurfacePlan.RuntimeGroundingDisposition.
	// No answer-body synthesis happens here: this only removes
	// structurally impossible citation floors.
	if isMeasurementScalar || isHistoryLookup || rm.HasObservationOnlyRuntimeArtifact() {
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
	// B8-T1 (block_only_carrier.md §5.8): reconcileShape deleted.
	// The V2 carrier path no longer reads RequiredAnswerShape for
	// answer-rendering decisions — AnswerSemanticView (compiled per
	// QuestionFamily) drives block requirements. The LLM's
	// emit_analysis output is now the authoritative shape decision
	// for the legacy V1 path (which itself is going away in B8-T4);
	// no system-side override.

	out.AnswerContract.Diagram = reconcileDiagramContract(rm, logBundle)
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
	// gate.Run skips read-mode-only quality checks
	// (hypothesis_coverage / contract_complete) when ctx.Mode is plan
	// / apply / verify, where the write pipeline carries its own
	// criterion suite and the planner does not consume HypothesisSet.
	// Without this, a "create from scratch" write request rejects at
	// the gate because the codebase has nothing to hypothesize about.
	mode := ""
	if ctx != nil && ctx.Mode != "" {
		mode = string(ctx.Mode)
	}
	ir.QualityGate = gate.RunWith(ir, gate.GlobalThresholds(), mode, gate.RunOptions{Resolver: resolver})

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

	// CritExternalArtifactDecoded stays out of AcceptanceTests. The
	// current runtime-artifact gate lives in the orchestrator contract
	// layer and consumes typed AnswerDocumentV2 carriers
	// (`observed_artifact_fact` + `external_observation`), not answer
	// prose. Pre-2026-05-02 the analyzer appended this criterion to
	// AcceptanceTests and contract.checkAcceptance treated the kind as
	// unknown, causing unsolvable finalizer retries. Keep the
	// structural trigger outside the analyzer-authored contract.

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
// analyzerSymbolResolver returns the right normalizer.SymbolResolver
// for the current Run. Multi-repo posture (≥2 sub-repos) returns a
// multiRepoSymbolResolver that fans out across every active
// sub-repo via mg.LookupSymbol / mg.IterateSymbolDefs. Single-repo
// / nil-multigraph posture returns the legacy
// newRepomapSymbolResolver(analyzerGraphForNormalize(...)) — byte-
// identical to pre-B2 behaviour.
//
// This closes the matching wiring gap that
// analyzerOracleFromCtx (analyzer.go:478) opened on the SymbolOracle
// path: pre-B2 the oracle path was multi-repo aware but the resolver
// path collapsed to a single graph, producing the asymmetric
// resolution noise that fails coherence R1.5 on cross-sub-repo emit
// shapes (forensic anchor: 2026-05-10 chatpp-7d46dee4 log L2710).
//
// Design doc: docs/design/multirepo_entity_scope_separation.md §4.2.
func analyzerSymbolResolver(ctx *types.AgentContext, rm types.RequestModel) normalizer.SymbolResolver {
	if mg := repomap.MultiGraphFromAgentContext(ctx); mg != nil && !mg.IsSingle() {
		var pending []string
		if ctx != nil {
			pending = ctx.PendingSubRepos
		}
		return newMultiRepoSymbolResolver(mg, pending)
	}
	return newRepomapSymbolResolver(analyzerGraphForNormalize(ctx, rm))
}

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
	g, err := repomap.GraphFromAgentContextOrLoad(ctx, ctx.RepoRoot, query)
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
	if capabilityQuery := detectStageToolCapabilityQuery(rm); capabilityQuery != nil {
		return capabilityAuthorityFiles(capabilityQuery)
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
		if g, err := repomap.GraphFromAgentContextOrLoad(ctx, ctx.RepoRoot, query); err == nil {
			graph = g
		}
	}

	// Tier 1.5 — file-shaped MentionedEntities ∪ ExactTargets.
	// Closes the gap where the user verbatim names a file path the
	// repomap graph cannot rank (no code symbols → QueryScore=0,
	// e.g. codrax.yaml.example / Dockerfile / *.proto). Filesystem
	// is the only signal allowed to decide "this entity names a
	// file" — no extension allowlist, no canonical basename table.
	// See mentioned_file_entities.go.
	//
	// Both AnalyzerHints lanes feed the seeder:
	//   - MentionedEntities: deterministic subset of Entities whose
	//     surface forms appear verbatim in RawRequest (provenance-
	//     bearing).
	//   - ExactTargets: LLM-emitted "config keys / file paths /
	//     symbols / literals", system-validated against RawRequest
	//     provenance before downstream contracts consume it.
	// Real eval s3a-20260502-181424 surfaced the gap: LLM placed
	// "codrax.yaml" in exact_targets but omitted top-level entities,
	// leaving MentionedEntities empty and bypassing the seeder.
	// Non-path exact_targets (function names, config keys without
	// a file form) match no fs layer (Layer A/B stat misses, Layer C
	// basename equality almost never collides with symbol names),
	// so the union adds coverage without false positives on non-
	// path question types.
	mentioned := dedupStringList(
		rm.AnalyzerHints.MentionedEntities,
		rm.AnalyzerHints.ExactTargets,
	)
	fsHits := mentionedFileEntities(ctx.RepoRoot, mentioned, graph)

	ranked := rankAnalyzerRequiredFiles(graph, entities)
	// Tier-split: fs-hits with MentionCount ≥ floor are repo-critical
	// (referenced by many other files) and take cap-budget priority
	// over generic ranker QueryScore hits. Below-floor fs-hits
	// compete with ranker on equal footing — they were verbatim
	// user-named but the repo doesn't treat them as load-bearing.
	floor := CurrentRequiredFileMentionCountFloor()
	var hi, lo []string
	for _, h := range fsHits {
		if h.MentionCount >= floor {
			hi = append(hi, h.Path)
		} else {
			lo = append(lo, h.Path)
		}
	}
	merged := mergeRequiredFilePathLists(hi, append([]string(nil), append(ranked, lo...)...), maxAnalyzerRequiredFilesCap())
	if len(logFiles) == 0 {
		return merged
	}
	return logtriage.MergeResolvedFiles(logFiles, merged)
}

// mergeRequiredFilePathLists unions head + tail, de-dupes by string
// equality preserving head-first order, and clips to cap. Cap=0
// returns the full union (no clip).
func mergeRequiredFilePathLists(head, tail []string, cap int) []string {
	if len(head) == 0 && len(tail) == 0 {
		return nil
	}
	out := make([]string, 0, len(head)+len(tail))
	seen := map[string]bool{}
	for _, p := range head {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if cap > 0 && len(out) >= cap {
			return out
		}
	}
	for _, p := range tail {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if cap > 0 && len(out) >= cap {
			return out
		}
	}
	return out
}

// maxAnalyzerRequiredFilesCap surfaces the historical cap=3 ceiling
// on RequiredFiles (Session-22 fix F1.2). Wrapped as a function so
// future overrides have one site to flip; production stays at 3.
func maxAnalyzerRequiredFilesCap() int { return 3 }

// dedupStringList unions any number of input slices into one
// order-stable, dedup'd result. Empty / whitespace-only entries are
// dropped. First occurrence wins (subsequent duplicates skipped).
//
// Used by analyzerRequiredFiles to build the file-shaped seeder
// input from MentionedEntities ∪ ExactTargets without paying the
// inner Layer-A/B/C cost twice for the same surface form.
func dedupStringList(lists ...[]string) []string {
	total := 0
	for _, l := range lists {
		total += len(l)
	}
	if total == 0 {
		return nil
	}
	seen := make(map[string]bool, total)
	out := make([]string, 0, total)
	for _, l := range lists {
		for _, raw := range l {
			s := strings.TrimSpace(raw)
			if s == "" || seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
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

// buildGenericGateRetryHint converts the joined gate-failure
// detail string (produced by classifyGateFailure for non-coherence
// hard failures) into an IR-field-level retry hint the next
// analyzer dispatch's prependEmitRetryDirective consumes.
//
// The hint is descriptive ("the previous attempt's emit_analysis
// failed these checks: …") not prescriptive — the analyzer LLM
// reads each check's Detail and decides which IR field to widen /
// narrow / re-classify. Without this hint the LLM saw only the
// generic terminal-forcing directive on retry, so a coverage =
// 0.4/0.7 failure would re-emit with the same coverage, then a
// dag_closure miss → another retry, etc.
func buildGenericGateRetryHint(joinedDetail string) string {
	if strings.TrimSpace(joinedDetail) == "" {
		return ""
	}
	return "## Quality gate rejected the previous emit_analysis\n\n" +
		"The structural quality gate found these failing checks (one or more lines):\n\n" +
		"- " + strings.ReplaceAll(joinedDetail, "; ", "\n- ") + "\n\n" +
		"Re-emit emit_analysis with the IR fields the failing check names point at — coverage / dag_closure / hypothesis_coverage / contract_complete / budget_sanity / criterion_resolvable etc. — adjusted to satisfy each. Do NOT just resubmit the same shape; the gate is structural and will reject identical emits."
}

// classifyGateFailure inspects a GateReport and returns whether any
// failure is HARD. pending_fields_wellformed is the only SOFT check;
// every other failing check is a hard error.
//
// Commit 55 Batch B (MEDIUM #3 audit fix): when MULTIPLE checks
// fail in the same dispatch, the detail string lists ALL of them
// rather than just the first one. Pre-fix the single-check return
// caused multi-round retry waste — the LLM fixed coverage on round
// N then got rejected on dag_closure on round N+1, when both were
// surfaceable in one round. The format is "name1: detail1; name2:
// detail2; …" with newlines suppressed inside individual details
// so the analyzer's prepended-directive rendering stays compact.
func classifyGateFailure(report types.GateReport) (hard bool, detail string) {
	var parts []string
	for _, c := range report.Checks {
		if c.Passed {
			continue
		}
		if c.Name == "pending_fields_wellformed" {
			continue
		}
		// Single-line each failure detail so the joined string stays
		// readable when the analyzer renders it into a retry hint.
		safe := strings.ReplaceAll(strings.ReplaceAll(c.Detail, "\n", " "), "\r", " ")
		// Strip internal rule prefix (R1.x / R2.x ...) from the body
		// so the user-facing error reads as plain prose instead of
		// leaking gate-rule codenames. The retry-hint path has its
		// own stripper; this is the user-/log-facing surface. Keep
		// the check name as a category label so operators can see
		// which gate fired.
		safe = plainCoherenceDetail(safe)
		parts = append(parts, fmt.Sprintf("%s: %s", c.Name, safe))
	}
	if len(parts) == 0 {
		return false, ""
	}
	return true, strings.Join(parts, "; ")
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

func prescanHasDeclarativeCandidateResults(history []types.ToolResult, repoRoot, blob string) bool {
	discovered, _, _ := extractFileCoverage(history, repoRoot)
	if len(discovered) == 0 {
		return prescanHasDeclarativeCandidate(strings.ToLower(blob))
	}
	for _, path := range discovered {
		lower := strings.ToLower(strings.TrimSpace(path))
		if lower == "" || types.LooksLikeAuxiliaryEvidencePath(lower) {
			continue
		}
		for _, pat := range declarative.DefaultFilenamePatterns() {
			if pat != "" && strings.Contains(lower, pat) {
				return true
			}
		}
	}
	return false
}

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

// extractQuotedLiterals is consumed by extractor.go for evidence
// scanning. The Session-11 reconcileFromObservations helper that
// originally lived here was retired together with AnswerShape — the
// rule existed solely to nudge AnalyzerHints.Shape toward "value"
// on a quoted-literal hit, and the V2 carrier no longer keys answer
// rendering off shape.

// expandEntitiesWithImplementers is the L0-B-pre helper that turns
// "list all implementers of <interface>" questions into emit-able
// shape for the L0-B cardinality gate. When the analyzer's
// classification says the question IS an enumeration (or
// registration) but the LLM only emitted the interface / trait /
// protocol name (because it has not yet read the implementing
// types), this helper consults the repo graph and expands the
// entity list with the names of every concrete implementer.
//
// Returns the expansion when ALL of the following hold:
//
//  1. rm.Predicates.IsCategoryEnumeration is true (question is an
//     enumeration of values).
//  2. At least one analyzer entity is a known Symbol in the repo graph with
//     Kind ∈ {interface, trait, protocol} — the language-agnostic
//     marker that "this token is the interface side of an
//     implements relation". The entity list may also contain partial
//     candidate implementers the LLM guessed from prior context; those
//     candidates must not prevent typed graph expansion. populateImplementers (graph build
//     post-pass) cross-references this against every concrete type
//     whose method set / `impl Trait` / `extends Class` / Python
//     ABC subclass list satisfies the interface, regardless of
//     source language.
//
// When any condition fails the input slice is returned unchanged
// (no expansion). The helper is a pure read on rm + graph; no
// closure mutation. Empty / nil receiver / unknown entity / no
// implementers all degrade to the same "return as-is" path.
//
// Why here (analyze stage, post-amplifier, pre-L0-B): the L0-B
// gate is the load-bearing structural check that traps the
// interface-only emit. Doing the expansion just before L0-B means
// the gate sees the populated entities slice as if the LLM had
// emitted them, and the rest of the pipeline (compiler /
// scenario template / downstream rendering) consumes the
// authoritative implementer list rather than the wrapper handle.
//
// Cross-language note: the typed signal Symbol.Kind +
// Symbol.Implements (populated by populateImplementers) abstracts
// per-language implementation. Go uses structural method-set
// checking; Java / Kotlin / Swift use the `implements` /
// `Protocol` keyword; Rust uses `impl Trait for T`; Python uses
// ABC subclass registration. The graph normalises all these into
// the same Implements list, so this helper does not need to know
// which language the file is in.
func expandEntitiesWithImplementers(ctx *types.AgentContext, rm types.RequestModel) []string {
	original := rm.AnalyzerHints.Entities
	if !rm.Predicates.IsCategoryEnumeration {
		return original
	}
	if distinctNamedEntities(original) == 0 {
		return original
	}
	graph := analyzerGraphForNormalize(ctx, rm)
	if graph == nil {
		return original
	}
	// Two expansion paths share the same shape:
	//   (a) entity is an interface / trait / protocol → expand with
	//       concrete implementers via Graph.ImplementersOf
	//   (b) entity is a file path → expand with the file's import
	//       paths via graph.FileIndex[path].Imports
	// Both pull from typed graph primitives populated by the
	// per-language extractors, so the expansion is structurally
	// correct + cross-language.

	// Path (a): interface / trait / protocol → implementers.
	// P4-cross-sub-repo (Sc 1, 2026-05-08): when ctx.MultiGraph is
	// present (multi_repo_enabled), pass the carrier so
	// expandImplementersFromGraph fans out via mg.ImplementersOf
	// across every active sub-repo. The interface declaration may
	// live in sub-a while implementers are scattered across sub-b/c —
	// without fan-out the answer would silently drop them.
	if targets := implementerExpansionTargetsFromEntities(graph, original); len(targets) > 0 {
		var implTarget any = graph
		if mg := repomap.MultiGraphFromAgentContext(ctx); mg != nil {
			implTarget = mg
		}
		exp := expandImplementersFromGraph(implTarget, targets)
		if len(exp.Names) > 0 {
			return mergeExpandedEntities(original, exp.Names)
		}
	}

	if distinctNamedEntities(original) != 1 {
		return original
	}
	// Pick the single distinct entity (case-trim). The remaining
	// path/package/directory expansions are single-handle repairs; if
	// the analyzer already emitted multiple entities, expanding a path
	// import set or package API from only the first token would mix
	// unrelated axes.
	var bare string
	for _, e := range original {
		t := strings.TrimSpace(e)
		if t != "" {
			bare = t
			break
		}
	}
	if bare == "" {
		return original
	}

	// Path (b): entity is a file path that resolves in graph.FileIndex
	// AND has at least one tracked import. The natural enumeration
	// for "list X's imports" / "what does X depend on" questions is
	// the file's import paths themselves. Symbol-based lookup did
	// not match (the entity is not an interface symbol), and the
	// FileIndex lookup gives us the typed import slice the
	// per-language extractors populate (Go `import` blocks, Python
	// `import` / `from-import`, Java `import`, JS / TS
	// `import` / `require`, Rust `use`, etc.).
	//
	// Two-step lookup:
	//   1. Direct: graph.FileIndex[bare] (e.g. canonical path
	//      "internal/agent/explorer.go").
	//   2. Suffix-match fallback: when the LLM filled entities with
	//      a basename ("explorer.go") instead of the full path,
	//      scan FileIndex for entries whose key ends with
	//      "/<bare>" or equals "<bare>". Only fires when there is
	//      EXACTLY ONE match — multiple matches mean the basename
	//      is ambiguous and we can't disambiguate at analyze time
	//      (fall through, let the L0-B gate complain so the LLM
	//      retries with a more specific entity).
	if fi := lookupFileInfoWithSuffix(graph, bare); fi != nil && len(fi.Imports) > 0 {
		paths := make([]string, 0, len(fi.Imports))
		for _, imp := range fi.Imports {
			if p := strings.TrimSpace(imp.Path); p != "" {
				paths = append(paths, p)
			}
		}
		if len(paths) == 0 {
			return original
		}
		return mergeExpandedEntities(original, paths)
	}

	// Path (c) — package handle → exported APIs.
	// Phase 2.C (2026-05-09). When the entity resolves to a directory
	// prefix that contains ≥1 .go file in graph.FileIndex AND that
	// directory has ≥1 exported top-level Symbol (function / type /
	// constant / variable), expand entities with the exported symbol
	// names. Catches "list all exports of package X" / "what API does
	// package Y expose" questions that would otherwise hit L0-B
	// (single named entity, no enumerated values).
	//
	// Cross-language portable: Symbol.Exported is a typed boolean
	// the per-language extractors set (Go uppercase first letter,
	// Java/Kotlin public modifier, Python __all__ + non-underscore
	// prefix, Rust pub keyword, etc.). FileIndex prefix scan works
	// regardless of file extension or directory layout convention.
	//
	// Triggers ONLY when distinctNamedEntities=1 (the L0-B-gate
	// precondition this function exists to repair) — same as paths
	// (a) and (b).
	sourceScope := sourceScopeProfileForRequestModel(rm)
	if pkgExports := expandPackageExportsWithSourceScope(graph, bare, sourceScope); len(pkgExports) >= 1 {
		return mergeExpandedEntities(original, pkgExports)
	}

	// Path (d) — parent directory → child packages / sub-modules.
	// Phase 2.C (2026-05-09). When the entity resolves to a directory
	// containing ≥2 child sub-directories (each with ≥1 file indexed
	// in graph.FileIndex), expand entities with the child directory
	// names. Catches "list all sub-packages of internal/X" / "X 的
	// 子包" / "what modules live under Y" questions.
	//
	// Cross-language portable: graph.FileIndex is populated by all
	// per-language repomap extractors (Go / Java / Python / Rust /
	// Cangjie / ArkTS / TS / JS / Swift / Ruby / Lua / Proto /
	// Obj-C / CUDA / etc.) keyed by repo-relative path. Prefix scan
	// works regardless of file extension — the structural notion
	// "directory contains ≥2 indexed sub-directories" is
	// language-neutral. A Python project's `package.__init__.py`,
	// a Rust project's `mod.rs`, a Java project's `pom.xml`-rooted
	// sub-modules, a Proto project's `service/foo/x.proto` all show
	// up identically as path entries under the parent prefix.
	//
	// The ≥2 minimum is structural: a directory with one child is
	// not an enumeration. Empty directory or single-package directory
	// falls through to the existing L0-B reject (which is the
	// correct behavior — no enumeration to do).
	if children := expandChildPackagesWithSourceScope(graph, bare, sourceScope); len(children) >= 2 {
		return mergeExpandedEntities(original, children)
	}

	return original
}

func implementerExpansionTargetsFromEntities(graph *repomap.Graph, entities []string) []string {
	if graph == nil || len(entities) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(entities))
	var out []string
	for _, entity := range entities {
		entity = strings.TrimSpace(entity)
		if entity == "" || seen[strings.ToLower(entity)] {
			continue
		}
		if !entityIsInterfaceLikeInGraph(graph, entity) {
			continue
		}
		seen[strings.ToLower(entity)] = true
		out = append(out, entity)
	}
	return out
}

func entityIsInterfaceLikeInGraph(graph *repomap.Graph, entity string) bool {
	if graph == nil || strings.TrimSpace(entity) == "" {
		return false
	}
	for _, d := range graph.SymbolDefs[entity] {
		if d == nil {
			continue
		}
		switch d.Kind {
		case "interface", "trait", "protocol":
			return true
		}
	}
	return false
}

// expandPackageExports walks graph.FileIndex looking for files
// whose RelPath starts with `<bare>/` and aggregates the names of
// every Symbol.Exported=true top-level (Parent="") definition. Used
// by expandEntitiesWithImplementers Path (c) — see the docstring
// there for the full design rationale.
//
// Source-scope filtering is driven by the analyzer's typed
// SourceScopeProfile. Production scope excludes tests/docs/fixtures/
// examples/prompt-support paths; test/docs/all scopes opt those roles
// back in without scanning the user's prose. Without this filter a
// 9-export package can return 70+ entries (50+ test functions) that
// drown out the real API in the entity hint, while without the typed
// opt-in test/doc questions lose their principal evidence.
//
// Returns nil when the directory has no exported symbols, or when
// `bare` doesn't resolve to a directory prefix OR a package name in
// FileIndex.
//
// Cross-language matching paths (union):
//
//  1. Directory prefix — `bare="internal/analysis/criterion"`
//     matches every file under that dir. The full path form the
//     question text used.
//
//  2. Exact package equality — `bare="criterion"` matches files
//     whose FileInfo.Package equals "criterion". Covers Go module
//     names, Cangjie / Proto / Rust short package decls, and
//     dotted forms when the LLM emitted the full path.
//
//  3. Package last-segment — `bare="criterion"` matches files
//     whose FileInfo.Package's last dotted segment equals
//     "criterion". Covers Java / Kotlin / Python / Scala dotted
//     package paths (e.g. Package="com.foo.criterion") where the
//     LLM naturally emits just the trailing module name in
//     prose ("the criterion package's exports").
//
// All three routes converge on the same exported-symbol harvest.
// The LLM emits whichever form is most natural for the language
// ecosystem; we accept the union so the entity-expansion contract
// is form- and language-independent.
//
// False-positive guard: matching is by FileInfo.Package equality
// (or its last segment), NOT by path basename. A distractor file
// at "third_party/criterion/foo.go" whose extractor-assigned
// Package field is "differentpkg" does NOT match a query for
// "criterion" — the typed Package field is the source of truth.
func expandPackageExports(graph *repomap.Graph, bare string) []string {
	return expandPackageExportsWithSourceScope(graph, bare, nil)
}

func expandPackageExportsWithSourceScope(graph *repomap.Graph, bare string, profile *types.SourceScopeProfile) []string {
	if graph == nil || bare == "" {
		return nil
	}
	prefix := strings.TrimRight(bare, "/") + "/"
	seen := make(map[string]bool)
	var out []string
	for path, fi := range graph.FileIndex {
		// Match any of: directory prefix, exact Package equality,
		// or Package last-segment equality.
		matchesPrefix := strings.HasPrefix(path, prefix)
		matchesPackage := fi != nil && packageMatchesBare(fi.Package, bare)
		if !matchesPrefix && !matchesPackage {
			continue
		}
		if !sourceScopeAllowsExpansionPath(path, profile) {
			continue
		}
		if fi == nil {
			continue
		}
		for _, sym := range fi.Symbols {
			if !sym.Exported {
				continue
			}
			// Top-level only — skip method/field/nested definitions
			// (Parent != "" means this Symbol is scoped under
			// another type/struct/class).
			if sym.Parent != "" {
				continue
			}
			// Filter by Symbol.Kind for multi-language API surface.
			// Cross-language audit (every value here is a Kind the
			// repomap extractors actually emit — see `internal/tool/
			// repomap/...` per-language extractors):
			//   function — Go / Python / JS / TS / Rust / Lua / Ruby
			//   type     — Go (type alias / interface / struct alias)
			//   interface — Go / Java / Kotlin / TS / Cangjie / C#
			//   struct   — Go / Rust / C / C++ / Swift
			//   class    — Java / Kotlin / Python / Ruby / Swift /
			//              JS / TS / C++
			//   trait    — Rust / Cangjie
			//   protocol — Swift / Obj-C
			//   enum     — Go / Java / Rust / Swift / Kotlin /
			//              Cangjie / TS
			//   const    — Go / JS / TS / Rust
			//   var      — Go / JS / TS / Rust mutable
			//   module   — Ruby / Python __init__ / Lua module
			//   message  — Proto message declaration
			//   service  — Proto service declaration
			//   rpc      — Proto rpc method
			//   operator — Cangjie / C++ operator overload (top-level)
			// Exclude "method" / "field" / "ctor" / "package" /
			// relation kinds (call / import / chain / etc).
			switch sym.Kind {
			case "function", "type", "interface", "struct", "class",
				"trait", "protocol", "enum", "const", "var",
				"module", "message", "service", "rpc", "operator":
				if !seen[sym.Name] {
					seen[sym.Name] = true
					out = append(out, sym.Name)
				}
			}
		}
	}
	return out
}

// packageMatchesBare reports whether a FileInfo.Package value
// matches the user-emitted `bare` entity. Cross-language unifier:
//
//   - Empty Package field (C / C++ / CUDA / Obj-C / Lua / static
//     frontend) — never matches; falls back to directory prefix.
//   - Exact equality (`Package=="criterion"`, `bare=="criterion"`)
//     — Go module names, Cangjie / Proto short decls, Rust crates.
//   - Last-segment equality (`Package=="com.foo.criterion"`,
//     `bare=="criterion"`) — Java / Kotlin / Python / Scala dotted
//     package paths where the LLM naturally emits the trailing
//     module name. Splits on '.' (the universal package-segment
//     separator across these ecosystems).
//
// The check is conservative: only EXACT segment-equality counts,
// not substring containment, so "criterion-helpers" does not match
// "criterion".
func packageMatchesBare(pkg, bare string) bool {
	if pkg == "" || bare == "" {
		return false
	}
	if pkg == bare {
		return true
	}
	// Last-dotted-segment match (Java / Kotlin / Python / Scala).
	if dot := strings.LastIndexByte(pkg, '.'); dot >= 0 && dot < len(pkg)-1 {
		if pkg[dot+1:] == bare {
			return true
		}
	}
	return false
}

// expandChildPackages walks graph.FileIndex looking for files whose
// RelPath starts with `<bare>/` and collects distinct immediate
// child directory names. Used by expandEntitiesWithImplementers
// Path (d) — see the docstring there for the full design rationale.
//
// Returns nil when fewer than 2 distinct child directories are
// found (insufficient for an enumeration). Single-package or
// empty directories fall through to the existing L0-B reject —
// the correct behavior because there is no enumeration to perform.
func expandChildPackages(graph *repomap.Graph, bare string) []string {
	return expandChildPackagesWithSourceScope(graph, bare, nil)
}

func expandChildPackagesWithSourceScope(graph *repomap.Graph, bare string, profile *types.SourceScopeProfile) []string {
	if graph == nil || bare == "" {
		return nil
	}
	prefix := strings.TrimRight(bare, "/") + "/"
	seen := make(map[string]bool)
	var out []string
	for path := range graph.FileIndex {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		if !sourceScopeAllowsExpansionPath(path, profile) {
			continue
		}
		// Extract the immediate child directory: the segment
		// between `prefix` and the next "/". Files directly under
		// `<bare>/` (no further nesting) are skipped — they belong
		// to the bare directory itself, not a child package.
		rest := path[len(prefix):]
		slash := strings.IndexByte(rest, '/')
		if slash <= 0 {
			continue
		}
		child := rest[:slash]
		if !seen[child] {
			seen[child] = true
			out = append(out, child)
		}
	}
	return out
}

func sourceScopeAllowsExpansionPath(path string, profile *types.SourceScopeProfile) bool {
	scope := types.SourceScopeProduction
	if profile != nil && profile.RequestedScope.IsValid() {
		scope = profile.RequestedScope
	}
	return types.SourceScopeAllowsPathRole(scope, types.ClassifySourcePathRole(path))
}

// lookupFileInfoWithSuffix resolves an entity that may be a full
// repo-relative path OR a basename. Direct match first; on miss,
// scan FileIndex for any path ending in "/<entity>" or equal to
// "<entity>". Only returns a hit when exactly ONE FileIndex entry
// matches (so the suffix is unambiguous). Returns nil when bare
// is empty, graph is nil, no match, or multiple matches.
func lookupFileInfoWithSuffix(graph *repomap.Graph, bare string) *repomap.FileInfo {
	if graph == nil || bare == "" {
		return nil
	}
	if fi, ok := graph.FileIndex[bare]; ok {
		return fi
	}
	suffix := "/" + bare
	var match *repomap.FileInfo
	matchCount := 0
	for path, fi := range graph.FileIndex {
		if path == bare || strings.HasSuffix(path, suffix) {
			matchCount++
			if matchCount > 1 {
				return nil // ambiguous — multiple files match
			}
			match = fi
		}
	}
	if matchCount == 1 {
		return match
	}
	return nil
}

// mergeExpandedEntities appends `extra` to `original`, dropping
// case-folded duplicates and preserving insertion order. Shared
// helper for the two expansion paths in
// expandEntitiesWithImplementers (interface → implementers, file
// path → imports).
func mergeExpandedEntities(original, extra []string) []string {
	out := append([]string(nil), original...)
	seen := make(map[string]bool, len(extra)+len(out))
	for _, e := range out {
		seen[strings.ToLower(strings.TrimSpace(e))] = true
	}
	for _, name := range extra {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

// promoteSubTopicFileAnchorToPrimary covers the third axis of the
// "list X's Y" enumeration emit shape, the counterpart of paths
// (a) and (b) in expandEntitiesWithImplementers.
//
// Paths (a) and (b) target the count==1 form (LLM emits the
// container handle as the lone primary entity, downstream gates
// ground the values via graph). The skill prompt's "list values
// not container" guidance pushes a more capable LLM toward the
// count>=2 form instead:
//
//	PrimaryEntities = [value1, value2, ...]      (the values lane)
//	SubTopics       = [{entities: [<container>]}] (the anchor lane)
//
// This shape clears the L0-B cardinality gate (≥2 distinct
// entities) but trips R1.3 (entity_orphan in
// internal/analysis/gate/coherence.go) because the sub-topic's
// entity shares no element with PrimaryEntities. Structurally the
// emit is correct — the values are listed, the container is named
// — but the gate cannot reconcile the two lanes on its own.
//
// Repair: when the single sub-topic's lone entity resolves to a
// typed graph anchor (file in graph.FileIndex OR interface /
// trait / protocol in graph.SymbolDefs), promote that anchor into
// PrimaryEntities. R1.3's overlap check is satisfied; downstream
// structural reasoning sees both lanes.
//
// Gates (all must hold):
//
//  1. rm.Predicates.IsCategoryEnumeration is true.
//  2. len(rm.SubTopics) == 1 and the sub-topic has exactly one
//     entity. (≥2 sub-topics is R1.4 axis_collapse territory; 0
//     sub-topics means no orphan check fires.)
//  3. distinctNamedEntities(rm.AnalyzerHints.PrimaryEntities) >= 2.
//     (count<=1 is path (a)/(b)'s domain; coherence.go's R1.3
//     gate only fires when PrimaryEntities count >= 2.)
//  4. The sub-topic's entity resolves to either a file in
//     graph.FileIndex OR an interface / trait / protocol Symbol
//     in graph.SymbolDefs — the same typed graph primitives paths
//     (a) and (b) consume.
//  5. The resolved anchor is not already a case-folded member of
//     PrimaryEntities.
//
// Returns the (possibly augmented) PrimaryEntities slice. Pure
// read on rm + graph; the helper never shrinks the slice.
func promoteSubTopicFileAnchorToPrimary(ctx *types.AgentContext, rm types.RequestModel) []string {
	original := rm.AnalyzerHints.PrimaryEntities
	if !rm.Predicates.IsCategoryEnumeration {
		return original
	}
	if len(rm.SubTopics) != 1 {
		return original
	}
	// Mirror coherence.coherenceMinPrimaryEntitiesForOrphan (literal
	// 2 — duplicating the threshold here avoids a package import
	// cycle into internal/analysis/gate).
	if distinctNamedEntities(original) < 2 {
		return original
	}
	subEnts := rm.SubTopics[0].Entities
	if len(subEnts) != 1 {
		return original
	}
	bare := strings.TrimSpace(subEnts[0])
	if bare == "" {
		return original
	}
	for _, e := range original {
		if strings.EqualFold(strings.TrimSpace(e), bare) {
			return original
		}
	}
	graph := analyzerGraphForNormalize(ctx, rm)
	if graph == nil {
		return original
	}
	if !subTopicAnchorResolvesInGraph(graph, bare) {
		return original
	}
	out := append([]string(nil), original...)
	return append(out, bare)
}

// subTopicAnchorResolvesInGraph is the typed-signal predicate
// promoteSubTopicFileAnchorToPrimary uses to decide whether the
// sub-topic's lone entity is a structural container (file path or
// interface / trait / protocol symbol). Returns true on the first
// matching path.
func subTopicAnchorResolvesInGraph(graph *repomap.Graph, bare string) bool {
	if graph == nil || bare == "" {
		return false
	}
	if fi := lookupFileInfoWithSuffix(graph, bare); fi != nil {
		return true
	}
	if defs, ok := graph.SymbolDefs[bare]; ok {
		for _, d := range defs {
			if d == nil {
				continue
			}
			switch d.Kind {
			case "interface", "trait", "protocol":
				return true
			}
		}
	}
	return false
}

// distinctNamedEntities counts the case-folded distinct non-blank
// entries in entities. Used by the L0-B enumeration cardinality
// gate (see buildAnalysisIR). Mirrors amplifier.distinctEntityCount
// — duplicate logic chosen over importing amplifier here because
// agent already imports amplifier and the helper is a 6-line pure
// function; making it package-private to amplifier preserves that
// package's "no consumers reach in" boundary.
func distinctNamedEntities(entities []string) int {
	if len(entities) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(entities))
	for _, e := range entities {
		key := strings.ToLower(strings.TrimSpace(e))
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
	}
	return len(seen)
}
