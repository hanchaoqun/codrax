package agent

// answer_document_evaluator.go — finalizer evaluator that emits a
// structured AnswerDocument via the emit_answer_document tool and
// renders it to user prose through internal/render/answerdoc.go.
//
// Design contract: the LLM emits AnswerDocument via one batched
// tool call, the evaluator runs shape-level structural validation
// plus the cardinality cross-check for list_of_symbols/complete
// slates, and the renderer produces deterministic prose. The four
// fake-green patterns are structurally impossible on this path.
//
// Cardinality cross-check: the emit_answer_document tool already
// validates per-item grounding (line > 0, file not in WorkDir,
// citation_ref in range). The baseline (TerminalEvidenceCount +
// AnalysisIR.AnswerContract.MustInclude) is not part of the tool
// schema, so the evaluator applies validateCompletenessClaim here
// at ParseOutput time — the same function extractorEvaluator uses
// for emit_answer_symbol, so one audited implementation covers
// both stages.

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// answerDocumentEvaluator is the Evaluator implementation for the
// finalize stage.
type answerDocumentEvaluator struct {
	// maxRetries caps correction rounds. Set from
	// AgentSettings.FinalizerMaxCorrectionRetries at construction.
	maxRetries int
	// language is captured at BuildInitialInstruction time so ParseOutput
	// can pick the renderer locale without re-deriving it.
	language string

	// mu is captured at BuildInitialInstruction so ContinuationPrompt can
	// check whether the AnswerDocument tool call has landed before
	// issuing a correction retry. Without this, a content-only turn
	// that followed a successful tool call would burn a retry and
	// re-prompt the LLM to emit the tool call a second time,
	// clobbering the first document. The evaluator Evaluator
	// interface does not pass ctx to ContinuationPrompt, so the
	// pointer has to be stashed on the struct.
	mu *types.MutableState

	// retriesUsed tracks correction rounds across the ReAct loop,
	// bounded by e.maxRetries.
	retriesUsed int

	// rejectHintsUsed tracks targeted mid-loop repair hints after a
	// failed emit_answer_document tool call. Kept separate from the
	// soft-stop retry budget so a tool-level schema rejection can be
	// corrected immediately without burning the "missing document"
	// fallback path.
	rejectHintsUsed int

	// Shrinkage-salvage knobs, populated from AgentSettings at
	// construction. See salvagePriorDraftIntoSummary for the
	// triggering conditions and the salvage contract.
	//
	// preservePriorProse is the master switch; nil means "use the
	// code default (enabled)", *false disables the salvage outright.
	preservePriorProse *bool
	// shrinkageMinProseLen is the prior-draft length floor in bytes.
	// Zero falls back to the package-level default constant.
	shrinkageMinProseLen int
	// shrinkageRatio is the len(summary) / len(prior) ceiling below
	// which the salvage fires. Zero falls back to the default.
	shrinkageRatio float64

	// diagramRequired captures the resolved DiagramContract so retry
	// hints can preserve required diagrams instead of suggesting they
	// be deleted during summary-cap repair.
	diagramRequired    bool
	diagramMinimum     int
	diagramKinds       []types.DiagramKind
	configTraceDiagram bool
}

func (e *answerDocumentEvaluator) rejectHintBudget() int {
	if e == nil {
		return 0
	}
	if e.maxRetries <= 0 {
		return 8
	}
	budget := e.maxRetries * 4
	if budget < 8 {
		budget = 8
	}
	return budget
}

// BuildInitialInstruction renders ONLY the dynamic per-dispatch data the
// answer-document-skill system sections cannot carry:
//
//   - Resolved target shape (depends on ctx.AnalysisIR / ctx.AnswerSymbols)
//   - Cardinality baseline for list_of_symbols (MustInclude list)
//   - Prior extraction slate (from explorer / extractor output)
//   - User question echo
//
// All STATIC contract — tool name, shape-field dispatch table,
// citation pool semantics, completeness honesty contract,
// prohibitions — lives in `answer-document-skill` in
// internal/skill/defaults.go and is rendered as system sections
// (Goal / Workflow / OutputFormat / Prohibitions) by
// context/builder.go before this dynamic block runs.
//
// Rationale: matches the pattern used by extractor.go where the
// evaluator stays slim (dynamic data only) and the static contract
// lives in the skill config. Contradicting directives between a
// declarative skill OutputFormat and a Go-string-builder prompt
// are a known footgun, so this evaluator deliberately never
// repeats the skill's contract text.
func (e *answerDocumentEvaluator) BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string {
	e.retriesUsed = 0
	e.rejectHintsUsed = 0
	e.language = extractAnswerDocLang(ctx)
	e.diagramRequired = false
	e.diagramMinimum = 0
	e.diagramKinds = nil
	e.configTraceDiagram = false
	if ctx != nil {
		e.mu = ctx.Mutable
		e.configTraceDiagram = ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.Scenario == types.ScenarioConfigTrace
	}

	var b strings.Builder

	// User question is already rendered by builder.go as "User Request"
	// section — no need to repeat it here.

	shape := resolveAnswerDocShape(ctx)
	fmt.Fprintf(&b, "## Target answer shape\n\n`%s`\n\n", shape)

	if dc := answerDocDiagramContract(ctx); dc != nil && dc.Required {
		e.diagramRequired = true
		e.diagramMinimum = dc.Minimum
		if e.diagramMinimum <= 0 {
			e.diagramMinimum = 1
		}
		e.diagramKinds = append([]types.DiagramKind(nil), dc.PreferredKinds...)
		b.WriteString(renderAnswerDocDiagramContract(dc))
		if seeds := renderAnswerDocDiagramSeeds(ctx, dc); seeds != "" {
			b.WriteString(seeds)
		}
		if skeleton := renderAnswerDocFirstPassDiagramSkeleton(ctx); skeleton != "" {
			b.WriteString(skeleton)
		}
	} else if answerDocDiagramHardRequirementDowngraded(ctx) {
		b.WriteString("## Diagram Preference\n\n")
		b.WriteString("- A diagram would normally help for this question type, but the currently grounded evidence does not yet provide a complete structural seed for a hard-required diagram.\n")
		b.WriteString("- Prefer a grounded prose answer over an invented fence. Only draw a fenced diagram if every node / label can be copied from the existing citations, validated seeds, or Log Triage frames.\n\n")
	}
	if coverage := renderAnswerDocConfigTraceRoleCoverage(ctx, nil); coverage != "" {
		b.WriteString(coverage)
	}
	if exact := renderAnswerDocExactResolutionContract(ctx); exact != "" {
		b.WriteString(exact)
	}
	if checklist := renderAnswerDocSubmissionChecklist(ctx, shape, e.diagramRequired); checklist != "" {
		b.WriteString(checklist)
	}
	if ctx != nil && ctx.AnalysisIR != nil && isScalarSourceLiteralLookup(ctx.AnalysisIR.RequestModel) {
		b.WriteString("## Scalar Lookup Discipline\n\n")
		b.WriteString("- This dispatch asks for one named source-code literal, not for a walkthrough of the surrounding pipeline.\n")
		b.WriteString("- Keep `summary` narrow: identify the literal, give its grounded file:line location, and add only the minimal role sentence needed to justify why it is the answer.\n")
		b.WriteString("- `shape=value` / `shape=config_value` / `shape=boolean` still require a real `summary`. The literal or decision alone is not a complete answer.\n")
		b.WriteString("- Do not expand into adjacent helpers, orchestrated stages, or nearby components unless the user explicitly asked how the mechanism works.\n")
		b.WriteString("- For scalar payloads (`value`, `config_value`, `boolean`), every non-negative `citation_ref` must point at a real entry in `citations[]`. Do not emit `citation_ref: 0` with an empty citations pool.\n")
		b.WriteString("- If you include a secondary citation beyond the defining one, it must still directly name or call/reference the SAME emitted literal. Drop type comments, nearby docstrings, or broad background citations even when they are grounded.\n")
		b.WriteString("- If a related-context evidence item mentions surrounding pipeline pieces, treat that as background noise rather than answer content.\n\n")
		if isScalarRoleLocateLookup(ctx.AnalysisIR.RequestModel) {
			b.WriteString("- This is a role-locate lookup: the question names a clue or output, but the answer is the function / file / symbol that plays that role. Do not promote the clue itself into the exact target lane or the lead sentence.\n")
			b.WriteString("- For this kind of lookup, answer with the located literal and its file:line first. Mention surrounding pipeline stages only if they are strictly necessary to disambiguate the role.\n\n")
		}
	}

	if shape == string(types.ShapeListOfSymbols) {
		must := []string(nil)
		if ctx != nil && ctx.AnalysisIR != nil {
			must = ctx.AnalysisIR.AnswerContract.MustInclude
		}
		b.WriteString("## Expected answer count (symbols_completeness floor)\n\n")
		if len(must) > 0 {
			fmt.Fprintf(&b, "Required-symbol floor: **%d name(s)** — %s\n\n",
				len(must), strings.Join(must, ", "))
			fmt.Fprintf(&b,
				"A `symbols_completeness=complete` claim with fewer than %d items will be "+
					"DOWNGRADED to `lower_bound` automatically with a visible caveat in the "+
					"rendered answer. If you cannot reach the floor, choose `lower_bound` up "+
					"front — it is the honest terminal state.\n\n", len(must))
		} else {
			b.WriteString("Required-symbol floor is empty. No floor is enforced for this dispatch — ")
			b.WriteString("choose `complete` / `lower_bound` / `unknown` based on your own recall confidence.\n\n")
		}

		if ctx != nil && len(ctx.AnswerSymbols) > 0 {
			b.WriteString("## Prior slate from the extraction pipeline\n\n")
			b.WriteString("The upstream deterministic pipeline produced this answer-symbol list. ")
			b.WriteString("Use it as the starting point; adding items requires evidence from the ")
			b.WriteString("Evidence Items section, removing items requires a rationale in the ")
			b.WriteString("symbol's `rationale` field.\n\n")
			for _, s := range ctx.AnswerSymbols {
				if s.File != "" && s.Line > 0 {
					fmt.Fprintf(&b, "- %s (%s:%d)\n", s.Name, s.File, s.Line)
				} else {
					fmt.Fprintf(&b, "- %s\n", s.Name)
				}
			}
			b.WriteString("\n")
		}
	}

	// Multi-topic: guide the finalizer to address each sub-topic.
	if ctx != nil && ctx.AnalysisIR != nil && len(ctx.AnalysisIR.RequestModel.SubTopics) > 1 {
		b.WriteString("## Answer Structure (multi-topic)\n\n")
		b.WriteString("The user asked about multiple topics. " +
			"Your summary MUST address each one with a clearly labeled section:\n\n")
		for i, st := range ctx.AnalysisIR.RequestModel.SubTopics {
			fmt.Fprintf(&b, "%d. %s\n", i+1, st.Summary)
		}
		b.WriteString("\nProvide citations for each section.\n\n")

		// Anchor skeleton: when the extractor produced a per-topic
		// anchor slate (answer-symbols emitted during Turn B with
		// shape=explanation + sub_topics ≥ 1), echo those anchors
		// in the finalizer prompt so the LLM re-emits them as the
		// symbols[] payload. The renderer draws a Key Anchors block
		// beneath the summary when symbols[] is non-empty on
		// explanation shape. This pins the load-bearing identifiers
		// in the rendered output and stops the finalizer from
		// synthesizing prose that drifts from Turn A's evidence.
		if shape == string(types.ShapeExplanation) && len(ctx.AnswerSymbols) > 0 {
			b.WriteString("### Anchor skeleton (emit as symbols[])\n\n")
			b.WriteString("The extractor produced these per-sub-topic anchors. " +
				"Re-emit them verbatim in the `symbols[]` field of emit_answer_document " +
				"so the renderer can show them as a Key Anchors block beneath your prose. " +
				"Each anchor's file:line is authoritative — do not modify.\n\n")
			for _, s := range ctx.AnswerSymbols {
				if s.File != "" && s.Line > 0 {
					fmt.Fprintf(&b, "- %s (%s:%d)", s.Name, s.File, s.Line)
				} else {
					fmt.Fprintf(&b, "- %s", s.Name)
				}
				if r := strings.TrimSpace(s.Rationale); r != "" {
					fmt.Fprintf(&b, " — %s", r)
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func renderAnswerDocSubmissionChecklist(ctx *types.AgentContext, shape string, diagramRequired bool) string {
	var items []string
	switch shape {
	case string(types.ShapeListOfSymbols):
		items = append(items,
			"Fill non-empty `symbols[]` and set `symbols_completeness` honestly (`complete` / `lower_bound` / `unknown`).",
			"Use `summary` to frame what the list enumerates; do not let the list stand alone without context when the question needs explanation.",
		)
	case string(types.ShapeStepList):
		items = append(items,
			"Fill `steps[]` with ordered logical hops. Each step must either cite one grounded line or set `citation_ref=-1` honestly.",
			"Keep the hop-by-hop detail in `steps[]`; `summary` is only the lead-in and any required diagram.",
		)
	case string(types.ShapeValue):
		items = append(items,
			"Fill `value.literal` and `value.citation_ref`.",
			"Fill a real `summary` that names the subject being measured and states how the literal was obtained (lookup / file:line / command / chain).",
		)
	case string(types.ShapeConfigValue):
		items = append(items,
			"Fill `value.key`, `value.literal`, and `value.citation_ref`.",
			"Fill a real `summary` that names the config key / subject and states how the literal was obtained (lookup / file:line / chain).",
		)
	case string(types.ShapeBoolean):
		items = append(items,
			"Fill `boolean{decision,rationale,citation_ref}` with a grounded yes/no result and the mechanism that forces it.",
			"Use `summary` to set up the decision whenever a short lead-in improves readability.",
		)
	case string(types.ShapeExplanation):
		items = append(items,
			"`summary` is REQUIRED and is the main answer body for this dispatch.",
		)
		if ctx == nil || !types.ExplanationAllowsAnchorSkeleton(ctx.AnalysisIR) {
			items = append(items,
				"Leave `symbols[]` empty for this single-topic explanation. Anchor skeletons are only for multi-topic explanation answers whose prompt includes an Anchor skeleton section.",
			)
		}
	}
	if ctx != nil && ctx.AnalysisIR != nil && ctx.AnalysisIR.AnswerContract.ExactResolution != nil {
		items = append(items,
			"Fill `exact_resolution{status,anchor?,context_mode}` to match the current exact-target state; do not leave the status implicit in prose.",
		)
	}
	items = append(items,
		"Keep the answer at the abstraction already grounded by the evidence. A cited struct / function / type name does NOT license an invented field inventory, member count, default table, or exhaustive list unless a cited line or structured evidence item explicitly enumerates those members.",
	)
	if ctx != nil && ctx.LogTriage != nil && len(ctx.LogTriage.Errors) > 0 {
		items = append(items,
			"If this answer explains an attached log / stack trace, name each structured log error type or exception identifier from Log Triage at least once in `summary`. Do not paraphrase the type name away.",
		)
	}
	if diagramRequired {
		items = append(items,
			"`summary` must include at least one grounded triple-backtick diagram for this dispatch.",
			"When a `Diagram Seeds` section is present, treat it as the grounded template for first-pass repair-resistant output: prefer copying its node labels verbatim instead of renaming them into abstract aliases or numbered layers. Put any uncited conceptual layer names in prose outside the fenced diagram.",
			"Every file/path node you keep inside a fenced diagram must also be grounded by `citations[]` or by attached Log Triage frames in this dispatch. If a relationship lacks a grounded node label, explain it in prose instead of inventing a diagram node.",
		)
		if ctx != nil && ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.Scenario == types.ScenarioConfigTrace {
			items = append(items,
				"For config-precedence answers, every fenced-diagram node must have its own grounded citation. If any precedence layer is missing a grounded anchor in this dispatch, leave that layer in prose only instead of inventing a diagram node for it.",
			)
		}
	}
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Submission Checklist\n\n")
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	return b.String()
}

func isScalarRoleLocateLookup(rm types.RequestModel) bool {
	if !isScalarSourceLiteralLookup(rm) {
		return false
	}
	if rm.AnswerSubject.Kind == types.SubjectReturnValue {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Kind), "return_value") {
		return true
	}
	return rm.PredicateAxis == types.AxisReturn
}

// extractAnswerDocLang reads the language from AgentContext.Language,
// which is set by BuildAgentContext from BusContext.Language (the
// -lang CLI flag). Falls back to "en" when empty/off/none.
func extractAnswerDocLang(ctx *types.AgentContext) string {
	if ctx == nil {
		return "en"
	}
	switch ctx.Language {
	case "zh", "zh-CN", "zh-cn", "cn", "chinese":
		return "zh"
	case "", "off", "none":
		return "en"
	}
	return "en"
}

// resolveAnswerDocShape picks the shape string the finalizer prompt
// should target. Preference order:
//
//  1. AnalysisIR.AnswerContract.RequiredAnswerShape — the canonical
//     source wired by P1.3. Typed + non-empty means the analyzer
//     reached a decision.
//  2. irAnswerShape(ctx) — the legacy AnalyzerHints.Shape field,
//     kept as a fallback for pre-P1.3 call paths and REPL turns
//     where the IR is nil.
//  3. Presence of ctx.AnswerSymbols → list_of_symbols (an upstream
//     extraction pipeline found candidates, so the shape is clearly
//     a symbol list).
//  4. Explanation — the safe default.
//
// Returning a string (rather than types.AnswerShape) keeps the callers
// — prompt assembly + shape-specific instructions — uniform against
// both the typed and legacy paths without a per-call coercion.
func resolveAnswerDocShape(ctx *types.AgentContext) string {
	if ctx != nil && ctx.AnalysisIR != nil {
		if shape := types.EffectiveRequiredAnswerShape(ctx.AnalysisIR, ctx.Mutable); shape != "" && shape != types.ShapeNone {
			return string(shape)
		}
	}
	if s := irAnswerShape(ctx); s != "" {
		return s
	}
	if ctx != nil && len(ctx.AnswerSymbols) > 0 {
		return string(types.ShapeListOfSymbols)
	}
	return string(types.ShapeExplanation)
}

func answerSurfacePlan(ctx *types.AgentContext) *types.AnswerSurfacePlan {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return types.BuildAnswerSurfacePlan(
		ctx.AnalysisIR,
		ctx.Mutable,
		ctx.LogTriage,
		ctx.FlowFindings,
		ctx.AnswerChains,
		ctx.EvidenceItems,
	)
}

func baseAnswerDocDiagramContract(ctx *types.AgentContext) *types.DiagramContract {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.AnalysisIR.AnswerContract.Diagram == nil {
		return nil
	}
	return ctx.AnalysisIR.AnswerContract.Diagram
}

func answerDocDiagramContract(ctx *types.AgentContext) *types.DiagramContract {
	if plan := answerSurfacePlan(ctx); plan != nil {
		return plan.Diagram
	}
	return nil
}

func answerDocDiagramHardRequirementDowngraded(ctx *types.AgentContext) bool {
	if plan := answerSurfacePlan(ctx); plan != nil {
		return plan.DiagramHardRequirementDropped
	}
	return false
}

func answerDocExactResolutionContract(ctx *types.AgentContext) *types.ExactResolutionContract {
	if plan := answerSurfacePlan(ctx); plan != nil {
		return plan.ExactResolution
	}
	return nil
}

func renderAnswerDocDiagramContract(dc *types.DiagramContract) string {
	if dc == nil || !dc.Required {
		return ""
	}
	minimum := dc.Minimum
	if minimum <= 0 {
		minimum = 1
	}
	scope := dc.ScopeHint
	if scope == "" {
		scope = types.DiagramScopeOverall
	}

	var b strings.Builder
	b.WriteString("## Diagram Contract\n\n")
	b.WriteString("- Required: yes\n")
	fmt.Fprintf(&b, "- Minimum diagrams: %d\n", minimum)
	if len(dc.PreferredKinds) > 0 {
		kinds := make([]string, 0, len(dc.PreferredKinds))
		for _, kind := range dc.PreferredKinds {
			kinds = append(kinds, string(kind))
		}
		fmt.Fprintf(&b, "- Preferred kinds: %s\n", strings.Join(kinds, ", "))
	}
	fmt.Fprintf(&b, "- Scope: %s\n", scope)
	if len(dc.Reasons) > 0 {
		fmt.Fprintf(&b, "- Reasons: %s\n", strings.Join(dc.Reasons, ", "))
	}
	b.WriteString("- This requirement is independent of answer shape: if it says `Required: yes`, `summary` must contain at least one grounded triple-backtick diagram.\n")
	b.WriteString("- Reuse grounded labels directly inside the fence. Do not rename, normalize, or abstract a cited file / symbol / path literal into a different label unless that alternate label is itself grounded in citations or log frames.\n")
	b.WriteString("- Avoid invented enumeration labels like `Level 1`, `Round 2`, or `Step 3` unless those exact labels appear in grounded evidence.\n\n")
	return b.String()
}

func renderAnswerDocDiagramSeeds(ctx *types.AgentContext, dc *types.DiagramContract) string {
	if ctx == nil || dc == nil || !dc.Required {
		return ""
	}
	var b strings.Builder
	wrote := false
	appendSection := func(title, body string) {
		body = strings.TrimSpace(body)
		if body == "" {
			return
		}
		if !wrote {
			b.WriteString("## Diagram Seeds\n\n")
			b.WriteString("Use these grounded structures as the starting skeleton for the required diagram. Filenames inside the fenced block must come from citations[] or the Log Triage frames.\n\n")
			wrote = true
		}
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", title, body)
	}

	appendSection("Grounded Labeling", renderAnswerDocDiagramLabelSeed())
	appendSection("Diagram Node Allowlist", renderAnswerDocDiagramFileLabelSeed(ctx))
	appendSection("Config Trace Precedence", renderAnswerDocDiagramConfigTraceSeed(ctx))
	appendSection("Log Triage", renderAnswerDocDiagramLogSeed(ctx.LogTriage))
	appendSection("Flow Findings", renderAnswerDocDiagramFlowSeed(ctx.FlowFindings))
	appendSection("Answer Chains", renderAnswerDocDiagramChainSeed(ctx))
	appendSection("Exact Resolution Anchors", renderAnswerDocDiagramExactResolutionSeed(ctx))

	if !wrote {
		return ""
	}
	return b.String()
}

func renderAnswerDocFirstPassDiagramSkeleton(ctx *types.AgentContext) string {
	fence := renderRetryDiagramSeedFenceForRepair(ctx, nil)
	if fence == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## First-Pass Diagram Skeleton\n\n")
	b.WriteString("If you do not already have a richer grounded diagram, copy this fenced skeleton verbatim for the first pass and explain any extra semantics in prose around it. You may delete unused nodes, but do not rename the remaining ones.\n\n")
	b.WriteString(fence)
	b.WriteString("\n\n")
	return b.String()
}

func renderAnswerDocDiagramLabelSeed() string {
	return strings.TrimSpace(
		"Label each node with grounded names you already have: cited repo files, cited symbols, log-frame functions, or path literals that appear in cited line text.\n" +
			"- If the evidence names one spelling, keep that spelling in the diagram instead of renaming it to a nearby alias.\n" +
			"- If you need an alternate label, only use it when that exact label appears in citations or log frames.\n" +
			"- Prefer direct grounded names over abstract buckets such as `Level 1` / `Round 2` / `Step 3`.\n" +
			"- Inside a fenced diagram, keep file/path node labels to the prompt's `Diagram Node Allowlist` unless the same exact label is also grounded by citations[] or Log Triage frames in this tool call.",
	)
}

func renderAnswerDocDiagramFileLabelSeed(ctx *types.AgentContext) string {
	labels := collectAnswerDocDiagramFileLabels(ctx)
	if len(labels) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("If your fenced diagram names file or path nodes, keep them to this exact grounded allowlist unless the same exact alternate label is independently grounded in this dispatch:\n")
	for _, label := range labels {
		fmt.Fprintf(&b, "- `%s`\n", label)
	}
	b.WriteString("Do not shorten, strip suffixes, normalize example/demo filenames into runtime-looking aliases, or rewrite one grounded label into a nearby spelling unless that exact alternate label is independently grounded.")
	return strings.TrimSpace(b.String())
}

func collectAnswerDocDiagramFileLabels(ctx *types.AgentContext) []string {
	if ctx == nil {
		return nil
	}
	seen := make(map[string]bool)
	var labels []string
	appendLabel := func(label string) {
		label = strings.TrimSpace(strings.ReplaceAll(label, `\`, `/`))
		if label == "" || seen[label] {
			return
		}
		seen[label] = true
		labels = append(labels, label)
	}
	if ctx.LogTriage != nil {
		for _, err := range ctx.LogTriage.Errors {
			for _, frame := range err.Frames {
				appendLabel(frame.File)
			}
		}
		for _, file := range ctx.LogTriage.ResolvedFiles {
			appendLabel(file)
		}
	}
	for _, anchor := range collectConfigTraceDiagramAnchors(ctx) {
		label := strings.TrimSpace(anchor.Label)
		if idx := strings.LastIndex(label, ":"); idx > 0 {
			label = label[:idx]
		}
		appendLabel(label)
	}
	for _, chain := range ctx.AnswerChains {
		if types.DiagramEvidenceEligible(chain.Item) {
			appendLabel(chain.Item.Source)
		}
	}
	for _, ev := range ctx.EvidenceItems {
		if ev.DiagramRole == "" {
			continue
		}
		if types.DiagramEvidenceEligible(ev) {
			appendLabel(ev.Source)
		}
	}
	if len(labels) == 0 {
		return nil
	}
	sort.Strings(labels)
	if len(labels) > 10 {
		labels = labels[:10]
	}
	return labels
}

func renderAnswerDocDiagramLogSeed(bundle *types.LogBundle) string {
	if bundle == nil || len(bundle.Errors) == 0 {
		return ""
	}
	resolved := make([]types.LogFrame, 0, 8)
	for _, err := range bundle.Errors {
		for _, frame := range err.Frames {
			if frame.File == "" || frame.Line <= 0 {
				continue
			}
			resolved = append(resolved, frame)
			if len(resolved) >= 8 {
				break
			}
		}
		if len(resolved) >= 8 {
			break
		}
	}
	if len(resolved) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("When you draw a call-chain / sequence diagram, start from these resolved frames:\n\n```\n")
	for i, frame := range resolved {
		name := strings.TrimSpace(frame.Func)
		if name == "" {
			name = "(no symbol)"
		}
		switch {
		case i == 0:
			fmt.Fprintf(&b, "innermost failure: %s:%d in %s\n", frame.File, frame.Line, name)
		case i == len(resolved)-1:
			fmt.Fprintf(&b, "  -> caller (outermost): %s:%d in %s\n", frame.File, frame.Line, name)
		default:
			fmt.Fprintf(&b, "  -> caller:            %s:%d in %s\n", frame.File, frame.Line, name)
		}
	}
	b.WriteString("```")
	return b.String()
}

func renderAnswerDocDiagramFlowSeed(findings []types.FlowFindingDigest) string {
	if len(findings) == 0 {
		return ""
	}
	var b strings.Builder
	limit := len(findings)
	if limit > 3 {
		limit = 3
	}
	for i := 0; i < limit; i++ {
		ff := findings[i]
		parts := make([]string, 0, 3)
		if len(ff.Path) > 0 {
			parts = append(parts, "path="+strings.Join(ff.Path, " -> "))
		}
		if len(ff.Hops) > 0 {
			parts = append(parts, "hops="+strings.Join(ff.Hops, " -> "))
		}
		if len(ff.Conditions) > 0 {
			parts = append(parts, "conditions="+strings.Join(ff.Conditions, "; "))
		}
		if len(parts) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- %s\n", strings.Join(parts, " | "))
	}
	return strings.TrimSpace(b.String())
}

func renderAnswerDocDiagramChainSeed(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	chains := ctx.AnswerChains
	if len(chains) == 0 {
		return ""
	}
	contract := answerDocExactResolutionContract(ctx)
	var b strings.Builder
	limit := len(chains)
	if limit > 3 {
		limit = 3
	}
	for i := 0; i < limit; i++ {
		ev := chains[i].Item
		display := types.EvidencePreferredSurfaceText(ev, contract, false)
		if ev.Source != "" {
			if ev.LineStart > 0 {
				display += fmt.Sprintf(" (%s:%d)", ev.Source, ev.LineStart)
			} else {
				display += fmt.Sprintf(" (%s)", ev.Source)
			}
		}
		fmt.Fprintf(&b, "- %s\n", display)
	}
	return strings.TrimSpace(b.String())
}

func renderAnswerDocDiagramExactResolutionSeed(ctx *types.AgentContext) string {
	contract := answerDocExactResolutionContract(ctx)
	if ctx == nil || contract == nil {
		return ""
	}
	seeds := collectExactResolutionSeeds(ctx, contract)
	if len(seeds) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("For exact-target questions, keep the requested target separate from nearby context unless you have explicit alias proof. If you draw related context, prefer these grounded labels as node names:\n")
	for _, seed := range seeds {
		fmt.Fprintf(&b, "- %s\n", seed.Text)
	}
	return strings.TrimSpace(b.String())
}

type configTraceDiagramAnchor = types.ConfigTraceDiagramAnchor

func renderAnswerDocDiagramConfigTraceSeed(ctx *types.AgentContext) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.AnalysisIR.RequestModel.Scenario != types.ScenarioConfigTrace {
		return ""
	}
	anchors := collectConfigTraceDiagramAnchors(ctx)
	if len(anchors) < 2 {
		return ""
	}
	rolesPresent := make(map[string]bool, len(anchors))
	for _, anchor := range anchors {
		rolesPresent[anchor.Role] = true
	}
	var b strings.Builder
	b.WriteString("For config-precedence questions, use grounded source labels instead of numbered layers. This fenced chain is already ordered from highest precedence at the top to lowest precedence at the bottom using validated `diagram_role_hint` evidence; reuse it verbatim when it matches the evidence, and do NOT rename or reorder its nodes into abstract numbered placeholders:\n\n```\n")
	for i, anchor := range anchors {
		b.WriteString(anchor.Label)
		b.WriteByte('\n')
		if i < len(anchors)-1 {
			b.WriteString("  ->\n")
		}
	}
	b.WriteString("```\n")
	b.WriteString("Precedence semantics live in prose, not in invented node names: `override` = highest-precedence operator / CLI layer, `config` = grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.), `default` = code-default fallback. `runtime` is the binding / merge code path between those layers, not a standalone user-config tier.\n")
	if missing := missingConfigTraceDiagramRoles(rolesPresent); len(missing) > 0 {
		fmt.Fprintf(&b, "Current grounded evidence does NOT include anchor(s) for these precedence role(s): %s. Do not add fenced-diagram nodes for missing roles unless you first cite a real repo anchor for them; if you need to explain those semantics, keep them in prose as general precedence rules rather than grounded nodes in this dispatch.\n", strings.Join(missing, ", "))
	}
	b.WriteString("The safest valid fenced diagram for this dispatch is an exact copy of that chain, or a strict subsequence made only by deleting unused nodes. Do not invent new node names, aliases, buckets, or tier markers.\n")
	b.WriteString("Every node you keep in this diagram must also have a matching citation in `citations[]`. If you cannot cite a node, delete it from the chain instead of renaming it to an abstract bucket name (for example a generic step number, the literal `CLI`, or a tier label). Keep each node label as a plain grounded file/path label; do not prepend ordinal-tier wrappers.\n")
	b.WriteString("If you only need part of the chain, delete unused nodes rather than inventing new abstract labels. Conceptual layer names requested by the user belong in prose headings or bullets unless those exact file/path labels are themselves cited. If you want to explain semantics such as defaults, config-file load, runtime binding, or operator override, keep that explanation in prose outside the fenced diagram and cite it there. If you introduce a different file / symbol / path label, ground it first.")
	return strings.TrimSpace(b.String())
}

func missingConfigTraceDiagramRoles(present map[string]bool) []string {
	if len(present) == 0 {
		return nil
	}
	order := []types.EvidenceDiagramRole{
		types.EvidenceDiagramRoleOverride,
		types.EvidenceDiagramRoleConfig,
		types.EvidenceDiagramRoleRuntime,
		types.EvidenceDiagramRoleDefault,
	}
	var missing []string
	for _, role := range order {
		key := string(role)
		if key == "" || present[key] {
			continue
		}
		missing = append(missing, key)
	}
	return missing
}

func collectConfigTraceDiagramAnchors(ctx *types.AgentContext) []configTraceDiagramAnchor {
	if plan := answerSurfacePlan(ctx); plan != nil {
		out := make([]configTraceDiagramAnchor, 0, len(plan.ConfigTraceDiagramAnchors))
		for _, anchor := range plan.ConfigTraceDiagramAnchors {
			out = append(out, configTraceDiagramAnchor(anchor))
		}
		return out
	}
	return nil
}

func answerDocExactContextRequiredFiles(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	return ctx.Mutable.ExactContextRequiredFiles()
}

func firstNonEmptyString(items ...string) string {
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}
	return ""
}

func renderAnswerDocExactResolutionContract(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	contract := answerDocExactResolutionContract(ctx)
	if contract == nil || len(contract.Targets) == 0 {
		return ""
	}
	label := strings.TrimSpace(contract.TargetLabel)
	if label == "" {
		label = "target"
	}
	pending := types.ExactResolutionPendingTargets(contract, ctx.UnverifiedAnalyzerFindings)
	justification := strings.TrimSpace(sanitizeExactResolutionAbsenceJustification(ctx, contract, ctx.Mutable.StableAbsenceJustification()))
	stateAbsent := contract.AllowAbsence &&
		ctx.Mutable.StableInvestigationResultKind() == "absence" &&
		justification != "" &&
		len(pending) > 0
	plan := answerSurfacePlan(ctx)

	var b strings.Builder
	b.WriteString("## Exact Resolution Contract\n\n")
	fmt.Fprintf(&b, "- Requested exact %s: %s\n", label, backtickJoin(contract.Targets))
	b.WriteString("- Set `exact_resolution.status` explicitly: `exact_match`, `alias_match`, or `absent`.\n")
	b.WriteString("- Resolve the requested exact target directly. Do not answer with a nearby item unless you have explicit alias / synonym / parser-mapping proof.\n")
	b.WriteString("- The system renders the exact-resolution lead deterministically from `exact_resolution`; use `summary` for grounded explanation / nearby context, not for ambiguous substitute wording.\n")
	if contract.AllowAbsence {
		b.WriteString("- Absence-only is acceptable if the investigation shows the exact target is absent. In that case set `exact_resolution.status=\"absent\"`.\n")
	}
	if contract.AliasRequiresProof {
		b.WriteString("- Any alias / equivalent / substitute claim requires `exact_resolution.status=\"alias_match\"` plus `exact_resolution.anchor`, and explicit grounded proof. Never rely on lexical similarity or \"closest match\" reasoning.\n")
	}
	if hint := strings.TrimSpace(contract.RelatedContextScopeHint); hint != "" {
		fmt.Fprintf(&b, "- If you add related context, keep it within the %s, ground it, and set `exact_resolution.context_mode=\"grounded_context_only\"`.\n", hint)
	} else {
		b.WriteString("- If you add related context, keep it grounded and clearly separate it from the exact target resolution by using `exact_resolution.context_mode=\"grounded_context_only\"`.\n")
	}
	b.WriteString("- If you cite nearby grounded context beyond the primary exact-proof sources, you MUST set `exact_resolution.context_mode=\"grounded_context_only\"`. Leave `context_mode=\"none\"` only when the answer stays on the exact target proof itself.\n")
	b.WriteString("- Surface-allowed nearby context is not automatically citation-grade. When the prompt separates citation-grade anchors from prose-only anchors, only the citation-grade set may appear in `citations[]` or fenced diagrams; prose-only anchors must stay uncited in `summary`.\n")
	if stateAbsent {
		b.WriteString("- Investigation state: the exact target is currently absent in the repo / branch under inspection.\n")
		fmt.Fprintf(&b, "- Absence justification: %s\n", justification)
		b.WriteString("- Emit `exact_resolution.status=\"absent\"`; the renderer will insert the exact-absence lead before `summary`.\n")
		b.WriteString("- Locked exact-resolution output: if you keep any nearby grounded context, the safest valid object is `{\"status\":\"absent\",\"context_mode\":\"grounded_context_only\"}`. Do not switch to `exact_match` or `alias_match` unless a newly cited grounded anchor explicitly proves the target or an explicit mapping.\n")
		b.WriteString("- Do not speculate about hypothetical parser / runtime behavior (ignored, warning, error, fallback, etc.) unless a grounded repo anchor explicitly proves that behavior.\n")
		b.WriteString("- When `context_mode=\"grounded_context_only\"`, treat `summary` as the follow-on grounded-context block only. Do not write a second absence paragraph there: the renderer already supplies the exact target lead.\n")
		b.WriteString("- Keep the exact target name in the renderer-generated lead only. `summary` and diagrams should talk about grounded nearby anchors, not keep reusing the absent target as if it had a runtime path.\n")
		b.WriteString("- Keep nearby context at the abstraction already grounded by the evidence. A cited struct / function / type name does NOT license an invented field inventory, member count, default-value table, or exhaustive list unless a cited line or structured evidence item explicitly enumerates those members.\n")
		b.WriteString("- Do not add a separate paragraph about the effect of supplying the absent target unless the user explicitly asked for that behavior and a cited anchor proves it.\n")
		if types.ExactResolutionTargetIsConfigKey(contract) {
			b.WriteString("- Because the exact config key is absent, do NOT force `shape=config_value` with a synthetic literal such as `(missing)` / `(不存在)`. Prefer `shape=explanation` so the answer can lead with the exact absence before any nearby grounded context.\n")
		}
		if ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.Scenario == types.ScenarioConfigTrace {
			b.WriteString("- Because the exact config key is absent, do NOT force `shape=config_value` with a synthetic literal such as `(missing)` / `(不存在)`. Prefer `shape=explanation` so the answer can lead with the exact absence and then explain any grounded same-family precedence chain as related context only.\n")
			b.WriteString("- For config-trace related context, grounded same-scope anchors may appear in `summary` even when they do not carry a validated diagram role. But fenced diagrams and diagram citations are stricter: only anchors with a validated `diagram_role_hint` (`default`, `config`, `runtime`, or `override`) may become diagram nodes.\n")
			b.WriteString("- For config-precedence answers, only create a separate numbered step when that layer has its own grounded repo anchor. If a layer is absent or only inferred from the exact-absence state, keep it in `summary` or set that step's `citation_ref=-1` instead of borrowing a nearby config-file / struct citation.\n")
			b.WriteString("- In `step_list`, any step with `citation_ref >= 0` must mention at least one identifier that appears on the cited line or its nearby corroboration window. If the step summarizes a whole struct/range/absence conclusion rather than one corroborated line, use `citation_ref=-1` and keep the precise line-backed facts in neighboring steps.\n")
			b.WriteString("- A repo-wide search result, aggregate absence conclusion, or test-only proof step usually has no single corroborating production line. In `step_list`, default those steps to `citation_ref=-1` unless one cited line literally states the same claim.\n")
		}
	}
	if plan != nil && plan.PreferredExactResolution != nil {
		fmt.Fprintf(&b, "- Preferred exact_resolution object for this dispatch: `{\"status\":\"%s\",\"context_mode\":\"%s\"}`.\n",
			plan.PreferredExactResolution.Status, plan.PreferredExactResolution.ContextMode)
	}
	if plan != nil && plan.SummarySurfaceMode == types.AnswerSummarySurfaceFollowOnGroundedContext {
		b.WriteString("- Summary surface mode: follow-on grounded context only. The renderer already prints the exact-target absence lead, so `summary` should start directly on the grounded nearby context/mechanism and must not restate the exact target.\n")
	}
	b.WriteString("\n")
	citationGradeRendered := false
	if citationGrade := renderAnswerDocCitationGradeExactContextAnchors(ctx, contract); citationGrade != "" {
		citationGradeRendered = true
		b.WriteString(citationGrade)
	}
	if policy := renderAnswerDocNearbyContextCitationPolicy(ctx, contract); policy != "" {
		b.WriteString(policy)
	}
	if allowed := renderAnswerDocAllowedExactContextAnchors(ctx, contract, citationGradeRendered); allowed != "" {
		b.WriteString(allowed)
	}
	if proseOnly := renderAnswerDocProseOnlyExactContextAnchors(ctx, contract); proseOnly != "" {
		b.WriteString(proseOnly)
	}
	if diagram := renderAnswerDocDiagramGradeExactContextAnchors(ctx, contract); diagram != "" {
		b.WriteString(diagram)
	}
	if candidates := renderAnswerDocRelatedContextCitationCandidates(ctx, contract); candidates != "" {
		b.WriteString(candidates)
	}
	if forbidden := renderAnswerDocForbiddenExactContextAnchors(ctx, contract); forbidden != "" {
		b.WriteString(forbidden)
	}
	if seeds := renderAnswerDocExactResolutionSeeds(ctx, contract); seeds != "" {
		b.WriteString(seeds)
	}
	return b.String()
}

type exactResolutionSeed struct {
	Key   string
	Text  string
	Score int
}

func exactResolutionSurfaceEvidencePool(ctx *types.AgentContext) []types.EvidenceItem {
	if ctx == nil {
		return nil
	}
	var emitted []types.EvidenceItem
	if ctx.Mutable != nil {
		emitted = ctx.Mutable.EmittedEvidence()
	}
	return types.ExactResolutionSurfaceEvidencePool(emitted, ctx.EvidenceItems, ctx.AnswerChains)
}

func renderAnswerDocAllowedExactContextAnchors(ctx *types.AgentContext, contract *types.ExactResolutionContract, citationGradeRendered bool) string {
	anchors := collectAllowedExactContextAnchors(ctx, contract)
	if len(anchors) == 0 {
		return ""
	}
	if citationGradeRendered || answerDocNearbyContextIsProseOnly(ctx, contract) {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Surface-Allowed Grounded Context Anchors\n\n")
	b.WriteString("If you keep nearby grounded context in `summary`, keep it to these already-validated anchors. Anything outside this list is background only unless it is itself a primary exact-proof source.\n\n")
	b.WriteString("When the exact target is absent, start the first sentence of `summary` directly on one of these validated anchors or mechanisms. Do not reopen with the absent target name — the renderer already prints that lead.\n\n")
	for _, anchor := range anchors {
		fmt.Fprintf(&b, "- %s\n", anchor.Text)
	}
	b.WriteString("\n")
	return b.String()
}

func renderAnswerDocNearbyContextCitationPolicy(ctx *types.AgentContext, contract *types.ExactResolutionContract) string {
	if !answerDocNearbyContextIsProseOnly(ctx, contract) {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Nearby Context Citation Policy\n\n")
	b.WriteString("For this dispatch, the validated nearby grounded context is prose-only. Keep it on the user-visible surface only as uncited explanation.\n\n")
	b.WriteString("- Do NOT place nearby grounded context anchors into `citations[]` or fenced diagrams unless a later prompt section explicitly promotes one into the citation-grade set.\n")
	b.WriteString("- Keep `citations[]` on the primary exact-proof / absence-proof anchors only.\n")
	b.WriteString("- If you keep nearby grounded context in `summary`, set `exact_resolution.context_mode=\"grounded_context_only\"` and start directly from the grounded nearby anchor/mechanism rather than repeating the exact target name.\n\n")
	return b.String()
}

func renderAnswerDocCitationGradeExactContextAnchors(ctx *types.AgentContext, contract *types.ExactResolutionContract) string {
	anchors := collectCitationGradeExactContextAnchors(ctx, contract)
	if len(anchors) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Citation-Grade Grounded Context Anchors\n\n")
	b.WriteString("If nearby grounded context appears in `citations[]` or in any fenced diagram, it MUST come from this list (plus any primary exact-proof anchors). Other surface-allowed anchors may still appear in `summary`, but only as uncited prose.\n\n")
	for _, anchor := range anchors {
		fmt.Fprintf(&b, "- %s\n", anchor.Text)
	}
	b.WriteString("\n")
	return b.String()
}

func renderAnswerDocProseOnlyExactContextAnchors(ctx *types.AgentContext, contract *types.ExactResolutionContract) string {
	anchors := collectProseOnlyExactContextAnchors(ctx, contract)
	if len(anchors) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Prose-Only Grounded Context Anchors\n\n")
	b.WriteString("These grounded same-scope anchors may stay in `summary` as nearby context even when they do not yet carry a validated precedence role. Keep them out of fenced diagrams and `citations[]` unless upstream already provided a validated `diagram_role_hint` for them.\n\n")
	for _, anchor := range anchors {
		fmt.Fprintf(&b, "- %s\n", anchor.Text)
	}
	b.WriteString("\n")
	return b.String()
}

func answerDocNearbyContextIsProseOnly(ctx *types.AgentContext, contract *types.ExactResolutionContract) bool {
	plan := answerSurfacePlan(ctx)
	if plan == nil || contract == nil {
		return false
	}
	if len(plan.ProseOnlyExactContextItems) == 0 {
		return false
	}
	for _, item := range plan.CitationGradeExactContextItems {
		if item.ContextRole != types.EvidenceContextRoleAbsenceSupport {
			return false
		}
	}
	return true
}

func renderAnswerDocDiagramGradeExactContextAnchors(ctx *types.AgentContext, contract *types.ExactResolutionContract) string {
	anchors := collectDiagramGradeExactContextAnchors(ctx, contract)
	if len(anchors) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Diagram-Grade Context Anchors\n\n")
	b.WriteString("If you draw a fenced diagram for the nearby grounded context, only use these already-validated precedence anchors (plus any primary exact-proof sources). Grounded same-scope prose anchors that are missing a validated diagram role may stay in `summary`, but they are not diagram nodes.\n\n")
	for _, anchor := range anchors {
		fmt.Fprintf(&b, "- %s\n", anchor.Text)
	}
	b.WriteString("\n")
	return b.String()
}

func collectProseOnlyExactContextAnchors(ctx *types.AgentContext, contract *types.ExactResolutionContract) []exactResolutionSeed {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || contract == nil {
		return nil
	}
	if ctx.AnalysisIR.RequestModel.Scenario != types.ScenarioConfigTrace ||
		contract.TargetKind != types.SubjectConfigKey ||
		ctx.Mutable.StableInvestigationResultKind() != "absence" ||
		strings.TrimSpace(ctx.Mutable.StableAbsenceJustification()) == "" {
		return nil
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		return nil
	}
	return exactResolutionSeedsFromItems(plan.ProseOnlyExactContextItems, func(ev types.EvidenceItem) (string, int) {
		return "grounded same-scope context, prose only", 18
	})
}

type relatedContextCitationCandidate struct {
	Label string
	Role  string
	Score int
}

func renderAnswerDocRelatedContextCitationCandidates(ctx *types.AgentContext, contract *types.ExactResolutionContract) string {
	candidates := collectRelatedContextCitationCandidates(ctx, contract)
	if len(candidates) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Related Context Citation Candidates\n\n")
	b.WriteString("If `summary` or a fenced diagram keeps nearby grounded precedence / lineage context on the user-visible answer surface, cite at least one of these exact file:line anchors in `citations[]`. Treat this list as the file:line form of the citation-grade anchors above. Background-only same-family context may stay uncited in prose only when it does not become the answer's visible lineage explanation or a diagram node.\n\n")
	for _, candidate := range candidates {
		fmt.Fprintf(&b, "- %s [%s]\n", candidate.Label, candidate.Role)
	}
	b.WriteString("\n")
	return b.String()
}

func renderAnswerDocConfigTraceRoleCoverage(ctx *types.AgentContext, contract *types.ExactResolutionContract) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.AnalysisIR.RequestModel.Scenario != types.ScenarioConfigTrace {
		return ""
	}
	anchors := collectConfigTraceDiagramAnchors(ctx)
	if len(anchors) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Precedence Role Coverage\n\n")
	b.WriteString("If you keep multi-layer precedence / lineage context on the user-visible surface, avoid collapsing it to a single mechanism anchor. Prefer keeping at least one validated anchor for each available precedence role below.\n\n")
	for _, anchor := range anchors {
		fmt.Fprintf(&b, "- `%s` → `%s`\n", anchor.Role, anchor.Label)
	}
	b.WriteString("\n")
	return b.String()
}

func collectRelatedContextCitationCandidates(ctx *types.AgentContext, contract *types.ExactResolutionContract) []relatedContextCitationCandidate {
	plan := answerSurfacePlan(ctx)
	if plan == nil || contract == nil {
		return nil
	}
	var out []relatedContextCitationCandidate
	for _, candidate := range plan.RelatedContextCitationCandidates {
		label := fmt.Sprintf("%s:%d", candidate.Source, candidate.Line)
		out = append(out, relatedContextCitationCandidate{
			Label: label,
			Role:  string(candidate.Role) + " precedence anchor",
			Score: 1,
		})
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func renderAnswerDocForbiddenExactContextAnchors(ctx *types.AgentContext, contract *types.ExactResolutionContract) string {
	anchors := collectForbiddenExactContextAnchors(ctx, contract)
	if len(anchors) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Background-Only Anchors\n\n")
	b.WriteString("These nearby same-family / illustrative anchors are NOT answer-grade context for this dispatch. Do not cite them, do not turn them into diagram nodes, and do not surface them in summary prose unless the investigation is explicitly reopened with new grounded proof.\n\n")
	for _, anchor := range anchors {
		fmt.Fprintf(&b, "- %s\n", anchor.Text)
	}
	b.WriteString("\n")
	return b.String()
}

func renderAnswerDocExactResolutionSeeds(ctx *types.AgentContext, contract *types.ExactResolutionContract) string {
	seeds := collectExactResolutionSeeds(ctx, contract)
	if len(seeds) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Exact Resolution Seeds\n\n")
	b.WriteString("These grounded anchors are related to the requested exact target. Use them as related context only unless you can prove an explicit alias / synonym mapping.\n\n")
	for _, seed := range seeds {
		fmt.Fprintf(&b, "- %s\n", seed.Text)
	}
	b.WriteString("\n")
	return b.String()
}

func collectAllowedExactContextAnchors(ctx *types.AgentContext, contract *types.ExactResolutionContract) []exactResolutionSeed {
	plan := answerSurfacePlan(ctx)
	if plan == nil || contract == nil {
		return nil
	}
	return exactResolutionSeedsFromItems(plan.AllowedExactContextItems, func(ev types.EvidenceItem) (string, int) {
		role := "grounded same-scope context"
		score := 18
		switch ev.ContextRole {
		case types.EvidenceContextRoleAbsenceSupport:
			role = "primary absence-proof source"
			score = 40
		default:
			switch ev.DiagramRole {
			case types.EvidenceDiagramRoleOverride:
				role = string(ev.DiagramRole) + " precedence anchor"
				score = 36
			case types.EvidenceDiagramRoleConfig:
				role = string(ev.DiagramRole) + " precedence anchor"
				score = 34
			case types.EvidenceDiagramRoleRuntime:
				role = string(ev.DiagramRole) + " precedence anchor"
				score = 32
			case types.EvidenceDiagramRoleDefault:
				role = string(ev.DiagramRole) + " precedence anchor"
				score = 30
			}
		}
		return role, score
	})
}

func collectCitationGradeExactContextAnchors(ctx *types.AgentContext, contract *types.ExactResolutionContract) []exactResolutionSeed {
	plan := answerSurfacePlan(ctx)
	if plan == nil || contract == nil {
		return nil
	}
	return exactResolutionSeedsFromItems(plan.CitationGradeExactContextItems, func(ev types.EvidenceItem) (string, int) {
		role := "citation-grade grounded context"
		score := 20
		switch ev.ContextRole {
		case types.EvidenceContextRoleAbsenceSupport:
			role = "primary absence-proof source"
			score = 40
		default:
			switch ev.DiagramRole {
			case types.EvidenceDiagramRoleOverride:
				role = string(ev.DiagramRole) + " precedence anchor"
				score = 36
			case types.EvidenceDiagramRoleConfig:
				role = string(ev.DiagramRole) + " precedence anchor"
				score = 34
			case types.EvidenceDiagramRoleRuntime:
				role = string(ev.DiagramRole) + " precedence anchor"
				score = 32
			case types.EvidenceDiagramRoleDefault:
				role = string(ev.DiagramRole) + " precedence anchor"
				score = 30
			}
		}
		return role, score
	})
}

func collectDiagramGradeExactContextAnchors(ctx *types.AgentContext, contract *types.ExactResolutionContract) []exactResolutionSeed {
	plan := answerSurfacePlan(ctx)
	if plan == nil || contract == nil {
		return nil
	}
	return exactResolutionSeedsFromItems(plan.DiagramGradeExactContextItems, func(ev types.EvidenceItem) (string, int) {
		role := "primary absence-proof source"
		if ev.ContextRole != types.EvidenceContextRoleAbsenceSupport && ev.DiagramRole != types.EvidenceDiagramRoleUnknown {
			role = string(ev.DiagramRole) + " precedence anchor"
		}
		return role, scoreForbiddenExactContextAnchor(ev) + 20
	})
}

func collectForbiddenExactContextAnchors(ctx *types.AgentContext, contract *types.ExactResolutionContract) []exactResolutionSeed {
	plan := answerSurfacePlan(ctx)
	if plan == nil || contract == nil {
		return nil
	}
	return exactResolutionSeedsFromItems(plan.ForbiddenExactContextItems, func(ev types.EvidenceItem) (string, int) {
		reason := "background only"
		switch {
		case ev.ContextRole == types.EvidenceContextRoleIllustrativeOnly:
			reason = "illustrative only"
		case types.LooksLikeAuxiliaryEvidencePath(ev.Source):
			reason = "auxiliary path"
		case ctx != nil && ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.Scenario == types.ScenarioConfigTrace && ev.DiagramRole == types.EvidenceDiagramRoleUnknown:
			reason = "no validated precedence role"
		}
		return reason, scoreForbiddenExactContextAnchor(ev)
	})
}

func scoreForbiddenExactContextAnchor(ev types.EvidenceItem) int {
	score := 0
	switch ev.ContextRole {
	case types.EvidenceContextRoleIllustrativeOnly:
		score += 30
	case types.EvidenceContextRoleDefining:
		score += 18
	case types.EvidenceContextRoleRelatedContext:
		score += 16
	case types.EvidenceContextRoleAbsenceSupport:
		score += 12
	}
	if ev.LineStart > 0 {
		score += 4
	}
	if ev.AnchorSymbol != "" {
		score += 2
	}
	return score
}

func exactResolutionSeedsFromItems(items []types.EvidenceItem, classify func(types.EvidenceItem) (string, int)) []exactResolutionSeed {
	if len(items) == 0 {
		return nil
	}
	var anchors []exactResolutionSeed
	seen := make(map[string]bool)
	for _, ev := range items {
		text := formatExactResolutionSurfaceSeed(ev)
		if text == "" {
			continue
		}
		key := fmt.Sprintf("%s:%d:%s:%s", ev.Source, ev.LineStart, ev.AnchorSymbol, ev.Summary)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		role, score := classify(ev)
		if role != "" {
			text = fmt.Sprintf("%s [%s]", text, role)
		}
		anchors = append(anchors, exactResolutionSeed{Key: key, Text: text, Score: score})
	}
	if len(anchors) == 0 {
		return nil
	}
	sort.SliceStable(anchors, func(i, j int) bool {
		if anchors[i].Score != anchors[j].Score {
			return anchors[i].Score > anchors[j].Score
		}
		return anchors[i].Text < anchors[j].Text
	})
	if len(anchors) > 6 {
		anchors = anchors[:6]
	}
	return anchors
}

func formatExactResolutionSurfaceSeed(ev types.EvidenceItem) string {
	parts := make([]string, 0, 2)
	if triple := strings.TrimSpace(strings.Join(exactResolutionSeedParts(ev.Subject, ev.Predicate, ev.Object), " ")); triple != "" {
		parts = append(parts, triple)
	} else if anchor := strings.TrimSpace(ev.AnchorSymbol); anchor != "" {
		parts = append(parts, anchor)
	} else if subject := strings.TrimSpace(ev.Subject); subject != "" {
		parts = append(parts, subject)
	}
	if len(parts) == 0 {
		if summary := strings.TrimSpace(ev.Summary); summary != "" {
			parts = append(parts, summary)
		}
	}
	text := strings.Join(parts, " - ")
	if text == "" {
		text = strings.TrimSpace(ev.Source)
	}
	if ev.Source != "" {
		if ev.LineStart > 0 {
			text += fmt.Sprintf(" (%s:%d)", ev.Source, ev.LineStart)
		} else {
			text += fmt.Sprintf(" (%s)", ev.Source)
		}
	}
	return text
}

func exactResolutionSeedParts(items ...string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func collectExactResolutionSeeds(ctx *types.AgentContext, contract *types.ExactResolutionContract) []exactResolutionSeed {
	if ctx == nil || contract == nil {
		return nil
	}
	contextTerms := types.ExactResolutionContextTerms(contract)
	if len(contextTerms) == 0 && len(contract.Targets) == 0 {
		return nil
	}

	type candidate struct {
		ev    types.EvidenceItem
		score int
	}
	configTraceExactContext := ctx.AnalysisIR != nil &&
		ctx.AnalysisIR.RequestModel.Scenario == types.ScenarioConfigTrace &&
		contract.TargetKind == types.SubjectConfigKey
	stableAbsent := ctx.Mutable != nil &&
		ctx.Mutable.StableInvestigationResultKind() == "absence" &&
		strings.TrimSpace(ctx.Mutable.StableAbsenceJustification()) != ""
	var candidates []candidate
	appendCandidate := func(ev types.EvidenceItem, base int) {
		if ev.Source == "" {
			return
		}
		if configTraceExactContext && stableAbsent &&
			!types.ExactResolutionAnswerContextAnchorAllowedInFiles(contract, ctx.AnalysisIR.RequestModel.Scenario, true, ev, answerDocExactContextRequiredFiles(ctx)) {
			return
		}
		score := base + scoreExactResolutionEvidence(ev, contract, contextTerms, configTraceExactContext)
		if score < 12 {
			return
		}
		candidates = append(candidates, candidate{ev: ev, score: score})
	}
	for _, chain := range ctx.AnswerChains {
		appendCandidate(chain.Item, 6)
	}
	for _, ev := range ctx.EvidenceItems {
		appendCandidate(ev, 0)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].ev.Source != candidates[j].ev.Source {
			return candidates[i].ev.Source < candidates[j].ev.Source
		}
		return candidates[i].ev.LineStart < candidates[j].ev.LineStart
	})

	seen := make(map[string]bool)
	out := make([]exactResolutionSeed, 0, 4)
	for _, cand := range candidates {
		key := fmt.Sprintf("%s:%d:%s:%s:%s:%s", cand.ev.Source, cand.ev.LineStart, cand.ev.Subject, cand.ev.Predicate, cand.ev.Object, cand.ev.Summary)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, exactResolutionSeed{
			Key:   key,
			Text:  types.FormatExactResolutionSeed(cand.ev),
			Score: cand.score,
		})
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func scoreExactResolutionEvidence(ev types.EvidenceItem, contract *types.ExactResolutionContract, contextTerms []string, configTraceExactContext bool) int {
	score := 0
	if contract == nil {
		return score
	}
	if ev.ContextRole == types.EvidenceContextRoleIllustrativeOnly ||
		ev.GroundingStatus == types.GroundingUngrounded ||
		ev.Kind == types.EvidenceUnresolved ||
		ev.Kind == types.EvidenceTruncated {
		return math.MinInt / 8
	}
	text := strings.ToLower(strings.Join([]string{
		ev.Subject, ev.Predicate, ev.Object, ev.AnchorSymbol, ev.Summary, ev.Source,
	}, " "))
	sourceLower := strings.ToLower(ev.Source)
	isTestLike := types.LooksLikeTestFilePath(ev.Source) || strings.Contains(sourceLower, "/testdata/") || strings.Contains(sourceLower, "\\testdata\\")
	exactMention := false
	for _, target := range contract.Targets {
		if types.ExactResolutionTextMentionsTarget(contract, text, target) {
			exactMention = true
			break
		}
	}
	if exactMention {
		if ev.ContextRole == types.EvidenceContextRoleAbsenceSupport {
			score += 1
		} else if isTestLike {
			score += 4
		} else {
			score += 18
		}
	}
	familyScore := types.ExactResolutionSameFamilyMatchScore(contract, strings.Join([]string{
		ev.Subject, ev.AnchorSymbol, ev.Object, ev.Source,
	}, " "))
	if contract.RelatedContextPolicy == types.ExactContextSameFamilyGrounded &&
		contract.TargetKind == types.SubjectConfigKey &&
		!exactMention &&
		ev.ContextRole == types.EvidenceContextRoleRelatedContext &&
		familyScore == 0 {
		return math.MinInt / 8
	}
	if configTraceExactContext &&
		contract.RelatedContextPolicy == types.ExactContextSameFamilyGrounded &&
		!exactMention &&
		ev.ContextRole != types.EvidenceContextRoleAbsenceSupport &&
		ev.DiagramRole == types.EvidenceDiagramRoleUnknown {
		return math.MinInt / 8
	}
	score += familyScore
	contextMatches := 0
	for _, term := range contextTerms {
		if strings.Contains(text, term) {
			contextMatches++
		}
	}
	if contextMatches > 3 {
		contextMatches = 3
	}
	score += contextMatches * 4
	switch ev.Kind {
	case types.EvidenceMechanism, types.EvidenceRelationship, types.EvidenceRegistration:
		score += 6
	case types.EvidenceDirect, types.EvidenceConcrete:
		score += 4
	}
	switch ev.ContextRole {
	case types.EvidenceContextRoleDefining:
		score += 6
	case types.EvidenceContextRoleAbsenceSupport:
		score += 1
	case types.EvidenceContextRoleRelatedContext:
		score += 3
	}
	switch strings.TrimSpace(strings.ToLower(ev.Predicate)) {
	case "maps", "config", "binds", "binds only", "defines", "registers", "wires", "calls", "dispatches", "delegates to", "returns", "constructs":
		score += 4
	}
	if ev.LineStart > 0 {
		score += 2
	}
	if isTestLike {
		score -= 8
	}
	return score
}

func formatExactResolutionSeed(ev types.EvidenceItem) string {
	parts := make([]string, 0, 3)
	if triple := strings.TrimSpace(strings.Join(filterEmptyStrings(ev.Subject, ev.Predicate, ev.Object), " ")); triple != "" {
		parts = append(parts, triple)
	}
	if summary := strings.TrimSpace(ev.Summary); summary != "" {
		if len(parts) == 0 || !strings.Contains(summary, parts[0]) {
			parts = append(parts, summary)
		}
	}
	text := strings.Join(parts, " — ")
	if text == "" {
		text = strings.TrimSpace(ev.Source)
	}
	if ev.Source != "" {
		if ev.LineStart > 0 {
			text += fmt.Sprintf(" (%s:%d)", ev.Source, ev.LineStart)
		} else {
			text += fmt.Sprintf(" (%s)", ev.Source)
		}
	}
	return text
}

func sanitizeExactResolutionAbsenceJustification(ctx *types.AgentContext, contract *types.ExactResolutionContract, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || ctx == nil || contract == nil || !contract.AllowAbsence {
		return raw
	}
	if !hasIllustrativeExactResolutionMention(ctx, contract) {
		return raw
	}
	label := strings.TrimSpace(contract.TargetLabel)
	if label == "" {
		label = "target"
	}
	return fmt.Sprintf(
		"The exact %s was searched across production code/config surfaces and was not found. Any doc/test/example/comment-only mentions are illustrative only and do not define a real %s.",
		label, label,
	)
}

func hasIllustrativeExactResolutionMention(ctx *types.AgentContext, contract *types.ExactResolutionContract) bool {
	if ctx == nil || contract == nil {
		return false
	}
	for _, ev := range ctx.EvidenceItems {
		if ev.ContextRole != types.EvidenceContextRoleIllustrativeOnly {
			continue
		}
		text := strings.Join([]string{
			ev.Subject, ev.Predicate, ev.Object, ev.AnchorSymbol, ev.Summary, ev.Snippet,
		}, "\n")
		for _, target := range contract.Targets {
			if types.ExactResolutionTextMentionsTarget(contract, text, target) {
				return true
			}
		}
	}
	return false
}

func filterEmptyStrings(items ...string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func backtickJoin(items []string) string {
	if len(items) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, "`"+item+"`")
	}
	return strings.Join(quoted, ", ")
}

// ShouldStop always returns false so the BaseAgent's soft-stop path
// delegates the accept/retry decision to ContinuationPrompt.
func (e *answerDocumentEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return false
}

// Observe implements LoopController. The finalizer only has a
// soft-stop branch — the answer-document stage runs a handful of
// one-shot LLM turns, so there is nothing to detect mid-loop.
//
// Soft-stop policy: a populated AnswerDocument in Mutable means the
// emit_answer_document tool call has already landed and ParseOutput
// will render the prose — accept the soft-stop. A missing document
// within the retry budget triggers a correction hint; beyond the
// budget the evaluator stays silent and the stage's ParseOutput
// writes a fail-loud warning from the raw last LLM content.
//
// Evaluator-specific retry budget (retriesUsed vs
// e.maxRetries) is INTENTIONALLY kept here rather
// than delegated to LoopPolicy.MaxContinuations: it is a
// finalizer-specific contract ("try at most N correction prompts
// before fail-loud"), not a generic loop-wide cap. LoopPolicy's
// continuation cap still applies as an outer safety net.
func (e *answerDocumentEvaluator) Observe(ctx *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase == PhaseMidLoop {
		// emit_answer_document is the finalizer's terminal action —
		// once it fires, stop immediately instead of burning one extra
		// LLM round that would just produce a content-only soft-stop.
		if e.mu != nil {
			if doc := e.mu.AnswerDocument(); doc != nil && !doc.IsZero() {
				return LoopSignal{StopRequested: true, StopReason: "emit_answer_document called"}
			}
		}
		if sig := e.unexpectedFinalizerToolSignal(obs); sig.HintRequested {
			return sig
		}
		if sig := e.emitAnswerDocumentRejectSignal(ctx, obs); sig.HintRequested {
			return sig
		}
		return LoopSignal{}
	}
	if obs.Phase != PhaseSoftStop {
		return LoopSignal{}
	}
	if e.mu != nil {
		if doc := e.mu.AnswerDocument(); doc != nil && !doc.IsZero() {
			return LoopSignal{}
		}
	}
	if e.retriesUsed >= e.maxRetries {
		logging.Debug("[finalizer/answer_document] correction retries exhausted (%d); accepting response",
			e.retriesUsed)
		return LoopSignal{}
	}
	e.retriesUsed++
	logging.Debug("[finalizer/answer_document] correction retry #%d: requesting emit_answer_document",
		e.retriesUsed)
	// HintKey embeds the retry counter so the LoopPolicy's dedup
	// window does NOT swallow the second retry. The finalizer's
	// retry budget is an evaluator-owned contract; dedup by a
	// shared key would silently truncate it to the first attempt.
	return LoopSignal{
		HintRequested: true,
		HintKey:       fmt.Sprintf("finalizer.missing_document.%d", e.retriesUsed),
		// Instruct the model to REUSE its prior prose rather than rewrite it.
		// Without this directive, the second pass tends to produce a
		// compressed paraphrase instead of copying the richer draft into
		// the `summary` field — a measurable shrinkage of answer quality.
		Hint: "The answer must be delivered through the `emit_answer_document` tool call — text " +
			"written outside it does not ship. You already drafted the answer in your previous " +
			"message; treat that draft as your final text. Call `emit_answer_document` now and copy " +
			"your previous answer VERBATIM into the `summary` field. Do not trim for length unless " +
			"a prior tool rejection named an active summary cap. Derive the remaining required structured fields (citations[] " +
			"and any shape-specific payload) from the same draft. Do NOT rewrite, compress, or " +
			"paraphrase the content — the richness of the original draft is the answer.",
	}
}

func (e *answerDocumentEvaluator) unexpectedFinalizerToolSignal(obs LoopObservation) LoopSignal {
	if obs.Phase != PhaseMidLoop || len(obs.Response.ToolCalls) == 0 {
		return LoopSignal{}
	}
	var unexpected []string
	for _, tc := range obs.Response.ToolCalls {
		name := strings.TrimSpace(tc.Name)
		if name == "" || name == "emit_answer_document" {
			continue
		}
		unexpected = append(unexpected, name)
	}
	if len(unexpected) == 0 {
		return LoopSignal{}
	}
	if budget := e.rejectHintBudget(); budget > 0 && e.rejectHintsUsed >= budget {
		return LoopSignal{}
	}
	e.rejectHintsUsed++
	toolList := strings.Join(unexpected, ", ")
	return LoopSignal{
		HintRequested: true,
		HintKey:       fmt.Sprintf("finalizer.unexpected_tool.%d.%s", obs.Iteration, toolList),
		Hint:          fmt.Sprintf("This finalizer is a pure synthesizer. Do NOT call `%s` or any other read/search tool. Re-emit `emit_answer_document` using only the already-provided grounded evidence: `citations[]`, Diagram Seeds, Exact Resolution Seeds, and the prompt's Diagram Node Allowlist. If a step cannot honestly cite one grounded line, keep that fact in `summary` or set that step's `citation_ref=-1` instead of reopening files. Do not write free-form prose outside the tool call.", toolList),
	}
}

func (e *answerDocumentEvaluator) emitAnswerDocumentRejectSignal(ctx *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.LastToolResult == nil || obs.LastToolResult.ToolName != "emit_answer_document" || obs.LastToolResult.Success {
		return LoopSignal{}
	}
	if budget := e.rejectHintBudget(); budget > 0 && e.rejectHintsUsed >= budget {
		return LoopSignal{}
	}
	repair := obs.LastToolResult.Repair
	rejectCode, summary := parseAnswerDocRejectEnvelope(strings.TrimSpace(obs.LastToolResult.Summary))
	if rejectCode == "" && repair != nil {
		rejectCode = strings.TrimSpace(repair.Code)
	}
	if summary == "" && (repair == nil || strings.TrimSpace(repair.Hint) == "") {
		return LoopSignal{}
	}

	hint := "Your last `emit_answer_document` call was rejected by the tool. Re-emit `emit_answer_document` now after fixing ONLY the field(s) named in the tool error. Keep the grounded evidence, citations, and answer shape unchanged unless the tool explicitly says to change them. Do not reopen files or call read/search tools. Do not write free-form prose outside the tool call."
	reasonKey := "tool-reject"
	if repair != nil && strings.TrimSpace(repair.Hint) != "" {
		hint = strings.TrimSpace(repair.Hint)
		reasonKey = firstNonEmptyString(rejectCode, "tool-reject")
		if len(repair.Fields) > 0 && !repairHintMentionsFields(hint, repair.Fields) {
			hint = "Fix ONLY these field(s): `" + strings.Join(repair.Fields, "`, `") + "`. " + hint
		}
		if !strings.Contains(hint, "Do not write free-form prose outside the tool call.") {
			hint += " Do not write free-form prose outside the tool call."
		}
	}
	if detail := compactToolRejectSummary(summary); detail != "" && (repair == nil || strings.TrimSpace(repair.Hint) == "") {
		hint = "Your last `emit_answer_document` call was rejected by the tool. Re-emit `emit_answer_document` now after fixing this exact tool error: " + detail + ". Only change the named field(s); keep grounded citations/evidence and do not reopen files. Do not write free-form prose outside the tool call."
	}

	var summaryLen, cap int
	var shape string
	if _, err := fmt.Sscanf(summary, "summary length %d exceeds cap %d for shape=%s", &summaryLen, &cap, &shape); err == nil && cap > 0 {
		reasonKey = "summary-cap"
		if e.diagramRequired {
			hint = fmt.Sprintf("Your last `emit_answer_document` call was rejected because `summary` was too long for shape `%s` (cap %d chars, current %d). Re-emit `emit_answer_document` now with the same grounded answer but shorten `summary` below %d chars. Preserve the required grounded diagram; compress prose, repeated headings, and repeated citation prose first. Keep the facts in the tool fields and `citations[]`; do not write free-form prose outside the tool call.", strings.TrimSpace(shape), cap, summaryLen, cap)
		} else {
			hint = fmt.Sprintf("Your last `emit_answer_document` call was rejected because `summary` was too long for shape `%s` (cap %d chars, current %d). Re-emit `emit_answer_document` now with the same grounded answer but shorten `summary` below %d chars. Cut large diagrams, repeated headings, and repeated citation prose first. Keep the facts in the tool fields and `citations[]`; do not write free-form prose outside the tool call.", strings.TrimSpace(shape), cap, summaryLen, cap)
		}
	}

	if rejectCode == answerDocRejectCodeMissingDiagram || strings.Contains(summary, "diagram required for this dispatch") {
		reasonKey = "missing-diagram"
		hint = "Your last `emit_answer_document` call was rejected because this dispatch REQUIRES a grounded diagram in `summary`. Re-emit `emit_answer_document` now with the same answer shape and payload fields, but add at least one grounded triple-backtick diagram to `summary`. This obligation is independent of answer shape. Keep every filename inside the diagram grounded by citations[] or the Log Triage frames; do not write free-form prose outside the tool call."
		if e.configTraceDiagram {
			hint += " For config-precedence diagrams, do not invent a new box chart or layer aliases on retry; prefer copying the seeded grounded precedence chain verbatim."
		}
		hint = appendRetryDiagramSeedHint(hint, ctx, repair)
	}
	if rejectCode == answerDocRejectCodeDiagramGrounding || strings.Contains(summary, "references file(s) not present in citations[] or attached-log frames") {
		reasonKey = "diagram-grounding"
		hint = "Your last `emit_answer_document` call was rejected by the DIAGRAM-GROUNDING gate: the fenced diagram renamed or introduced file/path labels that are not grounded. Re-emit `emit_answer_document` now with the same answer, but inside the diagram reuse the exact grounded file / symbol / path labels from citations, cited line text, or Log Triage frames. Prefer copying directly from the prompt's `Diagram Node Allowlist` section or from the tool error's allowed-label list. Do NOT normalize one grounded label into a different spelling unless that alternate label is itself grounded. Prefer direct grounded node names over abstract aliases. Do NOT call `read_file`, `grep`, or any other tool to repair this — use the existing citations / seeds only. Do not write free-form prose outside the tool call."
		if repair != nil && repair.Metadata != nil {
			if allowed := strings.TrimSpace(repair.Metadata["allowed_labels"]); allowed != "" {
				hint += " Allowed grounded labels for this dispatch: `" + strings.ReplaceAll(allowed, ", ", "`, `") + "`."
			}
		}
		if e.configTraceDiagram {
			hint += " For config-precedence diagrams, keep the seeded precedence chain's node labels verbatim; do NOT rewrite them into abstract numbered placeholders. If the user asked for conceptual layers, explain those layer names in prose outside the fence unless the exact label is grounded."
		}
		hint = appendRetryDiagramSeedHint(hint, ctx, repair)
	}
	if rejectCode == answerDocRejectCodeDiagramCodename || strings.Contains(summary, "summary introduces codename label(s) not present in any citation's") {
		reasonKey = "diagram-codename"
		hint = "Your last `emit_answer_document` call was rejected by the CODENAME-GROUNDING gate: the summary introduced abstract enumeration labels that are not grounded. Re-emit `emit_answer_document` now with the same answer, but remove invented labels such as `Level 1` / `Round 2` / `Step 3` unless those exact tokens are cited. Label the diagram directly with grounded files, functions, config keys, or other evidenced entities instead. Do NOT call `read_file`, `grep`, or any other tool to repair this — use the existing citations / seeds only. Do not write free-form prose outside the tool call."
		if e.configTraceDiagram {
			hint += " In config-precedence diagrams, grounded file/path labels are the node names; numbered layer aliases are never required. If you need semantics like defaults / config-file load / runtime binding / operator override, move that explanation into prose outside the fenced diagram and keep the fence itself as the seeded chain (or a strict subsequence of it)."
		}
		hint = appendRetryDiagramSeedHint(hint, ctx, repair)
	}
	if rejectCode == answerDocRejectCodeExactContextSurface {
		reasonKey = "exact-context-surface"
		hint = "Your last `emit_answer_document` call was rejected because `summary` leaked exact-target / nearby-context material that does not belong on the user-visible answer surface. Re-emit `emit_answer_document` now with the same `exact_resolution` object and answer shape, but treat `summary` as the follow-on grounded-context block only."
		if repair != nil && repair.Metadata != nil {
			if repeated := strings.TrimSpace(repair.Metadata["repeated_target"]); repeated != "" {
				hint += " Do NOT restate " + repeated + " in `summary`: the renderer already prints the exact-target lead for this dispatch."
			}
			if forbidden := strings.TrimSpace(repair.Metadata["forbidden_anchors"]); forbidden != "" {
				hint += " Drop these background-only anchors from prose, diagrams, and citations: `" + strings.ReplaceAll(forbidden, ", ", "`, `") + "`."
			}
			if allowed := strings.TrimSpace(repair.Metadata["allowed_anchors"]); allowed != "" {
				hint += " Keep any nearby grounded context on this validated anchor set only: `" + strings.ReplaceAll(allowed, ", ", "`, `") + "`."
			}
		}
		hint += " Do not reopen files or switch exact-resolution status; rewrite `summary` around the already-grounded context only."
		if e.diagramRequired {
			hint += " A grounded diagram is still required for this dispatch, so keep one fenced diagram and trim it down to the same allowed anchors instead of deleting it."
			hint = appendRetryDiagramSeedHint(hint, ctx, repair)
		}
	}
	if rejectCode == answerDocRejectCodeExactResolution || strings.Contains(summary, "exact-resolution contract violated:") {
		reasonKey = "exact-resolution"
		hint = "Your last `emit_answer_document` call was rejected by the exact-resolution contract. Re-emit `emit_answer_document` now with a valid `exact_resolution{status,anchor?,context_mode}` object that matches the grounded evidence and current absence state for the requested exact target. Use `exact_match` only when a cited line or grounded evidence explicitly names the exact target, `alias_match` only with explicit grounded mapping proof plus `anchor`, and `absent` only when the investigation closed with `result_kind=\"absence\"` (absence-only is acceptable). Any nearby related context must remain `grounded_context_only`, not an equivalent, alias, or substitute. Do NOT call `read_file`, `grep`, or any other tool to repair this — decide from the current grounded evidence, citations, and seeds. Do not write free-form prose outside the tool call."
	}
	if repair != nil && repair.Code == "exact_resolution" && repair.Metadata != nil {
		if locked := strings.TrimSpace(repair.Metadata["locked_status"]); locked != "" {
			reasonKey = "exact-resolution-locked"
			hint = "Your last `emit_answer_document` call was rejected by the exact-resolution contract because the status is already locked by upstream state. Re-emit `emit_answer_document` now with `exact_resolution.status=\"" + locked + "\"`."
			if mode := strings.TrimSpace(repair.Metadata["preferred_context_mode"]); mode != "" {
				hint += " If you keep any nearby grounded context, set `exact_resolution.context_mode=\"" + mode + "\"`."
			}
			hint += " Do not switch to `exact_match` or `alias_match` unless a newly cited grounded anchor explicitly proves the exact target or an explicit alias mapping. Keep nearby context as background only, not as a substitute. Do NOT call `read_file`, `grep`, or any other tool to repair this — decide from the current grounded evidence, citations, and seeds. Do not write free-form prose outside the tool call."
		}
	}
	if repair != nil && repair.Code == "config_trace_context_citation" {
		reasonKey = "config-trace-context-citation"
		hint = strings.TrimSpace(repair.Hint)
		allowedCitations := ""
		allowedAnchors := ""
		roleCoverage := ""
		proseOnly := ""
		forbidden := ""
		invalid := ""
		if repair.Metadata != nil {
			allowedCitations = strings.TrimSpace(repair.Metadata["allowed_citations"])
			allowedAnchors = strings.TrimSpace(repair.Metadata["allowed_anchors"])
			roleCoverage = strings.TrimSpace(repair.Metadata["precedence_role_anchors"])
			proseOnly = strings.TrimSpace(repair.Metadata["prose_only_anchors"])
			forbidden = strings.TrimSpace(repair.Metadata["forbidden_anchors"])
			invalid = strings.TrimSpace(repair.Metadata["drop_citations"])
			if strings.TrimSpace(repair.Metadata["nearby_context_citation_mode"]) == "prose_only" {
				hint = "Re-emit `emit_answer_document` with the same exact-absence conclusion, but treat the nearby grounded context as prose-only for this dispatch: keep `citations[]` on the primary exact-proof / absence-proof anchors only, keep any nearby context uncited in `summary`, and if that nearby context stays visible set `exact_resolution.context_mode=\"grounded_context_only\"`."
			}
			if allowedCitations != "" {
				hint += " Only these grounded file:line anchors may appear in `citations[]` or fenced diagrams for nearby lineage context: `" + strings.ReplaceAll(allowedCitations, ", ", "`, `") + "`."
			}
			if allowedAnchors != "" {
				hint += " Visible nearby context may only use this validated anchor set: `" + strings.ReplaceAll(allowedAnchors, ", ", "`, `") + "`. Being visible does NOT make every anchor citation-grade; use only the file:line list above inside `citations[]` or fenced diagrams."
			}
			if roleCoverage != "" {
				hint += " If you keep multi-layer precedence on the user-visible surface, preserve it using this validated role coverage when possible: `" + strings.ReplaceAll(roleCoverage, ", ", "`, `") + "`."
			}
			if mode := strings.TrimSpace(repair.Metadata["preferred_context_mode"]); mode != "" && !strings.Contains(hint, "exact_resolution.context_mode") {
				hint += " Keep `exact_resolution.context_mode=\"" + mode + "\"` whenever that nearby context stays on the user-visible surface."
			}
			if proseOnly != "" {
				hint += " These anchors may stay on the user-visible answer surface as uncited prose-only grounded context after you remove their citation(s) and any diagram nodes that used them: `" + strings.ReplaceAll(proseOnly, ", ", "`, `") + "`."
			}
			if forbidden != "" {
				hint += " Drop any prose / diagram node whose only support comes from these background-only anchors: `" + strings.ReplaceAll(forbidden, ", ", "`, `") + "`."
			}
			if invalid != "" {
				hint += " Drop these invalid citation(s) from `citations[]`: `" + strings.ReplaceAll(invalid, ", ", "`, `") + "`."
			}
		}
		if allowedCitations != "" {
			hint += " Choose one valid repair path now: either (a) keep nearby precedence / lineage context, cite at least one allowed file:line lineage anchor, and move any prose-only anchors out of `citations[]` / fenced diagrams, or (b) delete that nearby context from the user-visible answer surface and keep only the exact-absence lead. Do not keep broad same-family background as the cited explanation."
		} else {
			hint += " No citation-grade nearby lineage anchors are available for this dispatch, so do not invent a replacement citation. Either keep only short uncited background that does not become the answer's visible lineage explanation, or delete the nearby context entirely and keep only the exact-absence lead."
		}
		hint = appendRetryDiagramSeedHint(hint, ctx, repair)
	}
	if rejectCode == answerDocRejectCodeAbsentExactConfigValueShape || strings.Contains(summary, "must not use shape=config_value") {
		reasonKey = "absent-config-value-shape"
		hint = "Your last `emit_answer_document` call was rejected because this dispatch is an exact-absent config-trace answer. Do NOT use `shape=config_value` with a synthetic literal like `(missing)` / `(不存在)`. Re-emit now with `shape=explanation`, keep `exact_resolution.status=\"absent\"`, and describe any grounded same-family precedence chain as related context only. Do not write free-form prose outside the tool call."
	}
	if rejectCode == answerDocRejectCodeLogTriageCoverage && repair != nil && strings.TrimSpace(repair.Hint) != "" {
		reasonKey = "log-triage-coverage"
		hint = strings.TrimSpace(repair.Hint)
	}
	if rejectCode == answerDocRejectCodeScalarSummaryRequired && repair != nil && strings.TrimSpace(repair.Hint) != "" {
		reasonKey = "scalar-summary-required"
		hint = strings.TrimSpace(repair.Hint)
	}
	if requiredFieldHint := buildAnswerDocRequiredFieldRetryHint(summary); requiredFieldHint != "" {
		reasonKey = "required-field"
		hint = requiredFieldHint
	}

	// Session-22 special-case: the literal-grounding gate on shape=
	// value / shape=config_value fires when the cited line has zero
	// identifier overlap with value.literal. The tool's error body
	// already names citation_ref=-1 as the escape, but that's
	// buried after diagnostic detail — the LLM sometimes keeps
	// trying fresh fabrications instead of reaching for -1.
	// Pattern-match the error substring here and surface the
	// action-to-take at the top of the mid-loop hint so the LLM
	// self-corrects in one round instead of burning the whole
	// retry budget on more fabrications.
	if rejectCode == answerDocRejectCodeLiteralGrounding || strings.Contains(summary, "not corroborated by citations[") {
		reasonKey = "literal-grounding"
		if repair != nil && strings.TrimSpace(repair.Hint) != "" {
			hint = strings.TrimSpace(repair.Hint)
		} else {
			hint = buildLiteralGroundingRetryHint(summary)
		}
	}
	if repair != nil && len(repair.Fields) > 0 && !repairHintMentionsFields(hint, repair.Fields) {
		hint = "Fix ONLY these field(s): `" + strings.Join(repair.Fields, "`, `") + "`. " + hint
	}
	if !strings.Contains(hint, "Do not write free-form prose outside the tool call.") {
		hint += " Do not write free-form prose outside the tool call."
	}

	e.rejectHintsUsed++
	return LoopSignal{
		HintRequested:  true,
		HintKey:        fmt.Sprintf("finalizer.reject.%s.%d", reasonKey, e.rejectHintsUsed),
		Hint:           hint,
		Progress:       true,
		BypassThrottle: true,
	}
}

func compactToolRejectSummary(summary string) string {
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func repairHintMentionsFields(hint string, fields []string) bool {
	hint = strings.TrimSpace(hint)
	if hint == "" || len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if !strings.Contains(hint, field) {
			return false
		}
	}
	return true
}

const answerDocRejectPrefix = "[answer_doc_reject:"

const (
	answerDocRejectCodeMissingDiagram              = "missing_diagram"
	answerDocRejectCodeDiagramGrounding            = "diagram_grounding"
	answerDocRejectCodeDiagramCodename             = "diagram_codename"
	answerDocRejectCodeExactContextSurface         = "exact_context_surface"
	answerDocRejectCodeExactResolution             = "exact_resolution"
	answerDocRejectCodeAbsentExactConfigValueShape = "absent_exact_config_value_shape"
	answerDocRejectCodeLiteralGrounding            = "literal_grounding"
	answerDocRejectCodeScalarSummaryRequired       = "scalar_summary_required"
	answerDocRejectCodeLogTriageCoverage           = "log_triage_coverage"
)

func parseAnswerDocRejectEnvelope(summary string) (code, detail string) {
	summary = strings.TrimSpace(summary)
	if !strings.HasPrefix(summary, answerDocRejectPrefix) {
		return "", summary
	}
	end := strings.Index(summary, "]")
	if end <= len(answerDocRejectPrefix) {
		return "", summary
	}
	code = strings.TrimSpace(summary[len(answerDocRejectPrefix):end])
	detail = strings.TrimSpace(summary[end+1:])
	if detail == "" {
		detail = summary
	}
	return code, detail
}

func buildAnswerDocRequiredFieldRetryHint(summary string) string {
	switch {
	case strings.Contains(summary, "shape=value requires summary to name the subject"):
		return "Your last `emit_answer_document` call was rejected because `shape=value` is missing the required `summary`. Re-emit the SAME `shape=value` payload, keep the grounded `value.literal` / `value.citation_ref`, and add 1-2 sentences in `summary` that (1) name the measured subject from the question and (2) state how the literal was obtained (lookup / file:line / command / chain). Do NOT reopen files or change the answer shape. Do not write free-form prose outside the tool call."
	case strings.Contains(summary, "shape=config_value requires summary to name the subject"):
		return "Your last `emit_answer_document` call was rejected because `shape=config_value` is missing the required `summary`. Re-emit the SAME `shape=config_value` payload, keep the grounded `value.key` / `value.literal` / `value.citation_ref`, and add 1-2 sentences in `summary` that (1) name the config key or measured subject and (2) state how the literal was obtained (lookup / file:line / chain). Do NOT reopen files or change the answer shape. Do not write free-form prose outside the tool call."
	case strings.Contains(summary, "shape=explanation requires a non-empty summary"):
		return "Your last `emit_answer_document` call was rejected because `shape=explanation` requires a non-empty `summary`. Re-emit the SAME `shape=explanation` answer with the answer body written into `summary`; do not try to move the explanation into other fields or reopen files. Do not write free-form prose outside the tool call."
	}
	return ""
}

func appendRetryDiagramSeedHint(hint string, ctx *types.AgentContext, repair *types.ToolRepair) string {
	seed := renderRetryDiagramSeedFenceForRepair(ctx, repair)
	if seed == "" {
		return hint
	}
	return hint + " If you need the safest grounded repair, copy this seeded fenced diagram verbatim (or delete unused nodes without renaming the remaining ones):\n\n" + seed
}

func renderRetryDiagramSeedFence(ctx *types.AgentContext) string {
	return renderRetryDiagramSeedFenceForRepair(ctx, nil)
}

type retryDiagramSeed struct {
	Fence     string
	MatchKeys []string
}

type retryDiagramSeedFilter struct {
	Strict  bool
	Allowed map[string]bool
}

func renderRetryDiagramSeedFenceForRepair(ctx *types.AgentContext, repair *types.ToolRepair) string {
	if ctx == nil {
		return ""
	}
	filter := buildRetryDiagramSeedFilter(repair)
	if !filter.Strict {
		if plan := answerSurfacePlan(ctx); plan != nil {
			if fence := strings.TrimSpace(plan.CompiledDiagramFence); fence != "" {
				return fence
			}
		}
	}
	for _, kind := range retryDiagramKinds(ctx) {
		if fence := renderRetryDiagramSeedFenceForKind(ctx, kind, filter); fence != "" {
			return fence
		}
	}
	return ""
}

func extractFirstFencedBlock(text string) string {
	start := strings.Index(text, "```")
	if start < 0 {
		return ""
	}
	rest := text[start+3:]
	end := strings.Index(rest, "```")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(text[start : start+3+end+3])
}

func retryDiagramKinds(ctx *types.AgentContext) []types.DiagramKind {
	dc := answerDocDiagramContract(ctx)
	if dc != nil && len(dc.PreferredKinds) > 0 {
		return append([]types.DiagramKind(nil), dc.PreferredKinds...)
	}
	return []types.DiagramKind{
		types.DiagramFlow,
		types.DiagramSequence,
		types.DiagramCallDAG,
		types.DiagramArchitecture,
	}
}

func renderRetryDiagramSeedFenceForKind(ctx *types.AgentContext, kind types.DiagramKind, filter retryDiagramSeedFilter) string {
	if ctx == nil {
		return ""
	}
	var seeds []retryDiagramSeed
	switch kind {
	case types.DiagramFlow:
		if ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.Scenario == types.ScenarioConfigTrace {
			seeds = appendRetryDiagramSeed(seeds, buildRetryConfigTraceDiagramSeed(ctx))
			break
		}
		seeds = appendRetryDiagramSeed(seeds, buildRetryFlowFindingSeed(ctx.FlowFindings))
	case types.DiagramSequence, types.DiagramCallDAG:
		seeds = appendRetryDiagramSeed(seeds, buildRetryLogDiagramSeed(ctx.LogTriage))
		if kind == types.DiagramCallDAG {
			seeds = appendRetryDiagramSeed(seeds, buildRetryFlowFindingSeed(ctx.FlowFindings))
		}
	case types.DiagramArchitecture:
		seeds = appendRetryDiagramSeed(seeds, buildRetryAnswerChainSeed(ctx.AnswerChains))
	}
	for _, seed := range seeds {
		if filter.Allows(seed) {
			return seed.Fence
		}
	}
	return ""
}

func appendRetryDiagramSeed(seeds []retryDiagramSeed, seed retryDiagramSeed) []retryDiagramSeed {
	if strings.TrimSpace(seed.Fence) == "" {
		return seeds
	}
	return append(seeds, seed)
}

func buildRetryConfigTraceDiagramSeed(ctx *types.AgentContext) retryDiagramSeed {
	anchors := collectConfigTraceDiagramAnchors(ctx)
	if len(anchors) < 2 {
		return retryDiagramSeed{}
	}
	keys := make([]string, 0, len(anchors)*2)
	for _, anchor := range anchors {
		label := strings.TrimSpace(anchor.Label)
		if label == "" {
			continue
		}
		keys = append(keys, retryDiagramSeedMatchKeys(label)...)
	}
	return retryDiagramSeed{
		Fence:     types.RenderConfigTraceDiagramFence(anchors),
		MatchKeys: dedupeRetryDiagramNodes(keys, 0),
	}
}

func buildRetryLogDiagramSeed(bundle *types.LogBundle) retryDiagramSeed {
	resolved := collectRetryLogFrames(bundle)
	if len(resolved) < 2 {
		return retryDiagramSeed{}
	}
	var b strings.Builder
	keys := make([]string, 0, len(resolved)*2)
	b.WriteString("```\n")
	for i, frame := range resolved {
		name := strings.TrimSpace(frame.Func)
		if name == "" {
			name = "(no symbol)"
		}
		location := fmt.Sprintf("%s:%d", frame.File, frame.Line)
		keys = append(keys, retryDiagramSeedMatchKeys(location)...)
		switch {
		case i == 0:
			fmt.Fprintf(&b, "innermost failure: %s in %s\n", location, name)
		case i == len(resolved)-1:
			fmt.Fprintf(&b, "  -> caller (outermost): %s in %s\n", location, name)
		default:
			fmt.Fprintf(&b, "  -> caller:            %s in %s\n", location, name)
		}
	}
	b.WriteString("```")
	return retryDiagramSeed{
		Fence:     b.String(),
		MatchKeys: dedupeRetryDiagramNodes(keys, 0),
	}
}

func collectRetryLogFrames(bundle *types.LogBundle) []types.LogFrame {
	if bundle == nil || len(bundle.Errors) == 0 {
		return nil
	}
	resolved := make([]types.LogFrame, 0, 8)
	for _, err := range bundle.Errors {
		for _, frame := range err.Frames {
			if frame.File == "" || frame.Line <= 0 {
				continue
			}
			resolved = append(resolved, frame)
			if len(resolved) >= 8 {
				return resolved
			}
		}
	}
	return resolved
}

func buildRetryFlowFindingSeed(findings []types.FlowFindingDigest) retryDiagramSeed {
	for _, ff := range findings {
		nodes := retryFlowFindingNodes(ff)
		if len(nodes) >= 2 {
			return retryDiagramSeed{
				Fence:     buildRetryDiagramFence(nodes),
				MatchKeys: dedupeRetryDiagramNodes(nodes, 0),
			}
		}
	}
	return retryDiagramSeed{}
}

func retryFlowFindingNodes(ff types.FlowFindingDigest) []string {
	var nodes []string
	nodes = append(nodes, ff.Path...)
	if len(nodes) < 2 {
		for _, hop := range ff.Hops {
			for _, part := range strings.Split(hop, "->") {
				nodes = append(nodes, strings.TrimSpace(part))
			}
		}
	}
	if len(nodes) < 2 {
		nodes = append(nodes, ff.Sources...)
		nodes = append(nodes, ff.Sinks...)
	}
	return dedupeRetryDiagramNodes(nodes, 6)
}

func buildRetryAnswerChainSeed(chains []types.AnswerChain) retryDiagramSeed {
	nodes := make([]string, 0, len(chains))
	keys := make([]string, 0, len(chains)*2)
	for _, chain := range chains {
		item := chain.Item
		label := firstNonEmptyString(
			item.DisplayLocation(true),
			strings.TrimSpace(item.Source),
			strings.TrimSpace(item.Subject),
			strings.TrimSpace(item.AnchorSymbol),
		)
		if label == "" {
			continue
		}
		nodes = append(nodes, label)
		keys = append(keys, retryDiagramSeedMatchKeys(label)...)
	}
	nodes = dedupeRetryDiagramNodes(nodes, 5)
	if len(nodes) < 2 {
		return retryDiagramSeed{}
	}
	return retryDiagramSeed{
		Fence:     buildRetryDiagramFence(nodes),
		MatchKeys: dedupeRetryDiagramNodes(keys, 0),
	}
}

func buildRetryDiagramSeedFilter(repair *types.ToolRepair) retryDiagramSeedFilter {
	filter := retryDiagramSeedFilter{Allowed: map[string]bool{}}
	if repair == nil || repair.Metadata == nil {
		return filter
	}
	appendCSV := func(raw string) {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			filter.Strict = true
			for _, key := range retryDiagramSeedMatchKeys(part) {
				filter.Allowed[key] = true
			}
		}
	}
	appendCSV(repair.Metadata["allowed_citations"])
	appendCSV(repair.Metadata["allowed_labels"])
	appendCSV(repair.Metadata["allowed_diagram_nodes"])
	return filter
}

func (f retryDiagramSeedFilter) Allows(seed retryDiagramSeed) bool {
	if strings.TrimSpace(seed.Fence) == "" {
		return false
	}
	if !f.Strict {
		return true
	}
	if len(seed.MatchKeys) == 0 {
		return false
	}
	for _, key := range seed.MatchKeys {
		if !f.Allowed[strings.TrimSpace(key)] {
			return false
		}
	}
	return true
}

func retryDiagramSeedMatchKeys(label string) []string {
	label = strings.ReplaceAll(strings.TrimSpace(label), `\`, `/`)
	if label == "" {
		return nil
	}
	keys := []string{label}
	if idx := strings.LastIndex(label, ":"); idx > 0 {
		suffix := label[idx+1:]
		if _, err := strconv.Atoi(suffix); err == nil {
			keys = append(keys, label[:idx])
		}
	}
	return dedupeRetryDiagramNodes(keys, 0)
}

func dedupeRetryDiagramNodes(nodes []string, limit int) []string {
	seen := make(map[string]bool, len(nodes))
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		node = strings.TrimSpace(node)
		if node == "" || seen[node] {
			continue
		}
		seen[node] = true
		out = append(out, node)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func buildRetryDiagramFence(nodes []string) string {
	nodes = dedupeRetryDiagramNodes(nodes, 6)
	if len(nodes) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("```\n")
	for i, node := range nodes {
		b.WriteString(node)
		b.WriteByte('\n')
		if i < len(nodes)-1 {
			b.WriteString("  ->\n")
		}
	}
	b.WriteString("```")
	return b.String()
}

func buildLiteralGroundingRetryHint(summary string) string {
	firstLine := strings.TrimSpace(strings.SplitN(summary, "\n", 2)[0])
	refField := "value.citation_ref"
	subject := "`value.literal`"
	context := "the literal is drawn from the attached log / external source (no grounded repo citation)"
	extra := "Do NOT try to find a different file:line — if the literal came from an external trace (panic frame, log function name, etc.), no repo citation exists by definition and -1 is the tool-schema-legal escape."
	if idx, ok := parseLiteralGroundingStepIndex(firstLine); ok {
		refField = fmt.Sprintf("steps[%d].citation_ref", idx)
		subject = fmt.Sprintf("`steps[%d].description`", idx)
		context = "that step summarizes a repo-wide search, aggregate absence, test-only proof, or other claim that has no single corroborating repo line"
		extra = "Do NOT try to borrow a nearby file:line just to satisfy the schema — if the step summarizes an aggregate search result or absence conclusion rather than one corroborated line, `-1` is the tool-schema-legal escape."
	}
	return "Your last `emit_answer_document` call was rejected by the LITERAL-GROUNDING gate: the cited file:line does NOT corroborate " + subject + ". " +
		fmt.Sprintf("The single-action fix: re-emit now with `%s = -1`", refField) +
		" AND add a sentence to `summary` stating that " + context + ". " +
		extra + " Full tool error: " + firstLine
}

var literalGroundingStepRe = regexp.MustCompile(`^steps\[(\d+)\]\.description\b`)

func parseLiteralGroundingStepIndex(firstLine string) (int, bool) {
	m := literalGroundingStepRe.FindStringSubmatch(strings.TrimSpace(firstLine))
	if len(m) != 2 {
		return 0, false
	}
	idx, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return idx, true
}

// answerDocumentStageData is the JSON payload shape written into
// StageOutput.Data. Marshaling via a typed struct (rather than
// fmt.Sprintf with %q) keeps unicode escapes JSON-safe.
type answerDocumentStageData struct {
	FinalAnswer    string                `json:"final_answer"`
	AnswerDocument *types.AnswerDocument `json:"answer_document"`
}

// ParseOutput reads the AnswerDocument from Mutable, runs the
// cardinality cross-check on list_of_symbols + complete claims,
// renders the document to prose, and packages the result into a
// StageOutput. On a zero/missing document, emits a fail-loud warning
// prefixed to the raw last LLM content.
func (e *answerDocumentEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	out := &StageOutput{}

	var doc *types.AnswerDocument
	if ctx != nil && ctx.Mutable != nil {
		doc = ctx.Mutable.AnswerDocument()
	}

	if doc == nil || doc.IsZero() {
		var lastContent string
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "assistant" && messages[i].Content != "" {
				lastContent = messages[i].Content
				break
			}
		}
		warning := "⚠️ answer_document emission missing — the finalizer could not produce a " +
			"structured AnswerDocument. The text below is the raw LLM response; no schema-level " +
			"validation ran on it."
		safeFallback := sanitizePriorDraftForSummary(lastContent)
		if safeFallback == "" {
			safeFallback = strings.TrimSpace(lastContent)
		}
		combined := warning
		if safeFallback != "" {
			combined = warning + "\n\n" + safeFallback
		}
		logging.Warning("[finalizer/answer_document] emit_answer_document missing after retries; falling back to raw content (len=%d)",
			len(lastContent))
		out.Data = marshalStageData(answerDocumentStageData{FinalAnswer: combined})
		out.FinalAnswer = combined
		return out, nil
	}

	if doc.Shape == types.ShapeListOfSymbols && doc.SymbolsCompleteness == types.CompletenessComplete {
		validated := validateCompletenessClaim(ctx, doc.Symbols, doc.SymbolsCompleteness)
		if validated != doc.SymbolsCompleteness {
			logging.Warning("[finalizer/answer_document] symbols_completeness DOWNGRADED %s → %s for list_of_symbols answer",
				doc.SymbolsCompleteness, validated)
			doc.SymbolsCompleteness = validated
			// Caveat surfaces the downgrade in the rendered prose
			// instead of letting it happen silently.
			caveat := "symbols list was marked complete but did not meet the expected answer count; downgraded to lower_bound"
			if e.language == "zh" {
				caveat = "符号列表声称完整但未达到预期数量，已自动降级为 lower_bound"
			}
			doc.Caveats = append(doc.Caveats, caveat)
		}
	}

	e.salvagePriorDraftIntoSummary(doc, messages)

	prose := render.RenderAnswerDocument(doc, e.language)

	out.Data = marshalStageData(answerDocumentStageData{
		FinalAnswer:    prose,
		AnswerDocument: doc,
	})
	out.FinalAnswer = prose

	if len(doc.Symbols) > 0 {
		out.AnswerSymbols = doc.Symbols
		out.AnswerSymbolCompleteness = doc.SymbolsCompleteness
	}

	return out, nil
}

func marshalStageData(payload answerDocumentStageData) json.RawMessage {
	b, err := json.Marshal(payload)
	if err != nil {
		logging.Error("[finalizer/answer_document] marshal stage data failed: %v", err)
		return json.RawMessage(`{}`)
	}
	return b
}

func (e *answerDocumentEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingNone
}

// Shrinkage-salvage defaults. The LLM occasionally writes a rich
// answer as plain prose in its first attempt, fails to call the
// emit_answer_document tool, and then — on the correction retry —
// submits a visibly compressed paraphrase as `summary`. When the
// prior draft is substantial AND the emitted summary is under the
// configured fraction of its length, the richer draft is copied
// into `summary` so the user sees the full answer. These constants
// are the fallback used when AgentSettings does not override them.
const (
	defaultShrinkageMinPriorProseLen = 400
	defaultShrinkageRatio            = 0.5
)

// shrinkageThresholdsForShape returns the (minPriorProseLen, ratio)
// floor for detecting prior-draft shrinkage. The ratio is shape-
// independent (the LLM compresses by the same proportion regardless
// of shape); only the "is the prior draft substantive enough to
// bother salvaging?" floor scales with Summary's role in each shape:
//
//   - Explanation: Summary IS the body → full baseline floor.
//   - StepList / ListOfSymbols: Summary is a 1-3-sentence lead-in
//     above the structured payload → half baseline (prior prose is
//     the merged narrative before the LLM split it into structure).
//   - Boolean: Summary is a lead-in before YES/NO + rationale →
//     3/8 baseline.
//   - Value / ConfigValue: Summary is a 1-sentence lead-in before a
//     scalar literal → quarter baseline (scalar prior drafts
//     rarely exceed a paragraph).
func shrinkageThresholdsForShape(shape types.AnswerShape, baselineMinLen int, baselineRatio float64) (int, float64) {
	switch shape {
	case types.ShapeExplanation:
		return baselineMinLen, baselineRatio
	case types.ShapeStepList, types.ShapeListOfSymbols:
		return baselineMinLen / 2, baselineRatio
	case types.ShapeBoolean:
		return (baselineMinLen * 3) / 8, baselineRatio
	case types.ShapeValue, types.ShapeConfigValue:
		return baselineMinLen / 4, baselineRatio
	default:
		return baselineMinLen, baselineRatio
	}
}

// salvagePriorDraftIntoSummary mutates doc.Summary when the
// finalizer LLM dropped significant content between its pre-tool-call
// draft and its final emit_answer_document.summary. Every shape uses
// Summary as framing text — for explanation it is the body, for the
// other shapes it is a lead-in above the structured payload — so
// losing that framing between drafts visibly degrades the answer on
// every shape. Per-shape thresholds from shrinkageThresholdsForShape
// guard against over-aggressive salvage on scalar shapes whose
// Summary role is smaller.
func (e *answerDocumentEvaluator) salvagePriorDraftIntoSummary(doc *types.AnswerDocument, messages []llm.Message) {
	if e.preservePriorProse != nil && !*e.preservePriorProse {
		return
	}
	if doc == nil {
		return
	}
	baselineMin := e.shrinkageMinProseLen
	if baselineMin <= 0 {
		baselineMin = defaultShrinkageMinPriorProseLen
	}
	baselineRatio := e.shrinkageRatio
	if baselineRatio <= 0 {
		baselineRatio = defaultShrinkageRatio
	}
	minLen, ratio := shrinkageThresholdsForShape(doc.Shape, baselineMin, baselineRatio)
	priorProse := sanitizePriorDraftForSummary(findLastPreToolCallDraft(messages))
	if len(priorProse) < minLen {
		return
	}
	if float64(len(doc.Summary)) >= ratio*float64(len(priorProse)) {
		return
	}
	recovered := priorProse
	itemCount := len(doc.Steps) + len(doc.Symbols)
	if cap := types.SummaryCapFor(doc.Shape, itemCount); len(recovered) > cap {
		// Trim at a UTF-8 rune boundary so CJK prose does not end in
		// a partial multi-byte sequence (which would render as a
		// replacement glyph in the final answer).
		cut := cap
		for cut > 0 && !utf8.RuneStart(recovered[cut]) {
			cut--
		}
		recovered = recovered[:cut]
	}
	logging.Info("[finalizer/shrinkage] recovered prior draft into summary: shape=%s prior=%d summary=%d → %d",
		doc.Shape, len(priorProse), len(doc.Summary), len(recovered))
	doc.Summary = recovered
	caveat := "richer prior draft was preserved in place of a compressed summary"
	if e.language == "zh" {
		caveat = "已保留更丰富的前一轮草稿，替代被压缩的概述"
	}
	doc.Caveats = append(doc.Caveats, caveat)
}

var (
	priorDraftThinkBlockRe = regexp.MustCompile(`(?is)<think>.*?</think>\s*`)
	priorDraftToolCallRe   = regexp.MustCompile(`(?is)<(?:minimax:)?tool_call>.*?</(?:minimax:)?tool_call>\s*`)
	priorDraftInvokeRe     = regexp.MustCompile(`(?is)<invoke\b.*?</invoke>\s*`)
	priorDraftParameterRe  = regexp.MustCompile(`(?is)<parameter\b.*?</parameter>\s*`)
	priorDraftJSONFenceRe  = regexp.MustCompile("(?is)```(?:json)?\\s*\\{.*?\"shape\"\\s*:\\s*\"(?:list_of_symbols|step_list|value|config_value|boolean|explanation)\".*?```\\s*")
	priorDraftParaSplitRe  = regexp.MustCompile(`\n\s*\n+`)
)

// sanitizePriorDraftForSummary strips model-internal reasoning and
// tool-scaffolding from a pre-tool-call draft before salvage reuses it
// as user-visible summary prose. The salvage path is only meant to
// preserve natural-language answer content; letting scratch JSON,
// fake tool-call markup, or prompt-following meta prose leak back into
// Summary degrades answer quality and can expose implementation detail.
func sanitizePriorDraftForSummary(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	s = priorDraftThinkBlockRe.ReplaceAllString(s, "")
	s = priorDraftToolCallRe.ReplaceAllString(s, "")
	s = priorDraftInvokeRe.ReplaceAllString(s, "")
	s = priorDraftParameterRe.ReplaceAllString(s, "")
	s = priorDraftJSONFenceRe.ReplaceAllString(s, "")
	paras := priorDraftParaSplitRe.Split(s, -1)
	kept := make([]string, 0, len(paras))
	for _, p := range paras {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if looksLikeInternalDraftParagraph(p) {
			continue
		}
		kept = append(kept, p)
	}
	return strings.TrimSpace(strings.Join(kept, "\n\n"))
}

func looksLikeInternalDraftParagraph(p string) bool {
	if strings.TrimSpace(p) == "" {
		return true
	}
	lower := strings.ToLower(p)
	switch {
	case strings.Contains(lower, "emit_answer_document"),
		strings.Contains(lower, "citation_ref"),
		strings.Contains(lower, "tool call"),
		strings.Contains(lower, "tool-call"),
		strings.Contains(lower, "citations array"),
		strings.Contains(lower, "target shape"),
		strings.Contains(lower, "response structure"),
		strings.Contains(lower, "\"shape\":"),
		strings.Contains(lower, "\"citations\":"),
		strings.Contains(lower, "\"citation_ref\":"),
		strings.Contains(lower, "<minimax:tool_call>"),
		strings.Contains(lower, "<invoke"),
		strings.Contains(lower, "<parameter"),
		strings.HasPrefix(lower, "translation:"),
		strings.HasPrefix(lower, "let me construct the answer"),
		strings.HasPrefix(lower, "i need to emit"),
		strings.HasPrefix(lower, "i'm finalizing"),
		strings.HasPrefix(lower, "the citations array should contain"),
		strings.HasPrefix(lower, "i'll cite line"):
		return true
	}
	return false
}

// findLastPreToolCallDraft returns the most recent assistant message
// Content that is accompanied by NO tool calls. This uniquely
// identifies a free-text draft the model wrote before it emitted a
// tool call — the assistant message that fires a tool call carries
// a non-empty ToolCalls slice and is skipped.
func findLastPreToolCallDraft(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role != "assistant" {
			continue
		}
		if m.Content == "" {
			continue
		}
		if len(m.ToolCalls) > 0 {
			continue
		}
		return m.Content
	}
	return ""
}
