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
	"sort"
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
	diagramRequired bool
	diagramMinimum  int
	diagramKinds    []types.DiagramKind
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
	if ctx != nil {
		e.mu = ctx.Mutable
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
	}
	if exact := renderAnswerDocExactResolutionContract(ctx); exact != "" {
		b.WriteString(exact)
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
		if shape := ctx.AnalysisIR.AnswerContract.RequiredAnswerShape; shape != "" && shape != types.ShapeNone {
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

func answerDocDiagramContract(ctx *types.AgentContext) *types.DiagramContract {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.AnalysisIR.AnswerContract.Diagram == nil {
		return nil
	}
	return ctx.AnalysisIR.AnswerContract.Diagram
}

func answerDocExactResolutionContract(ctx *types.AgentContext) *types.ExactResolutionContract {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.AnalysisIR.AnswerContract.ExactResolution == nil {
		return nil
	}
	return ctx.AnalysisIR.AnswerContract.ExactResolution
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
	appendSection("Config Trace Precedence", renderAnswerDocDiagramConfigTraceSeed(ctx))
	appendSection("Log Triage", renderAnswerDocDiagramLogSeed(ctx.LogTriage))
	appendSection("Flow Findings", renderAnswerDocDiagramFlowSeed(ctx.FlowFindings))
	appendSection("Answer Chains", renderAnswerDocDiagramChainSeed(ctx.AnswerChains))
	appendSection("Exact Resolution Anchors", renderAnswerDocDiagramExactResolutionSeed(ctx))

	if !wrote {
		return ""
	}
	return b.String()
}

func renderAnswerDocDiagramLabelSeed() string {
	return strings.TrimSpace(
		"Label each node with grounded names you already have: cited repo files, cited symbols, log-frame functions, or path literals that appear in cited line text.\n" +
			"- If the evidence names one spelling, keep that spelling in the diagram instead of renaming it to a nearby alias.\n" +
			"- If you need an alternate label, only use it when that exact label appears in citations or log frames.\n" +
			"- Prefer direct grounded names over abstract buckets such as `Level 1` / `Round 2` / `Step 3`.",
	)
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

func renderAnswerDocDiagramChainSeed(chains []types.AnswerChain) string {
	if len(chains) == 0 {
		return ""
	}
	var b strings.Builder
	limit := len(chains)
	if limit > 3 {
		limit = 3
	}
	for i := 0; i < limit; i++ {
		ev := chains[i].Item
		display := ev.Summary
		if display == "" {
			display = fmt.Sprintf("[%s] %s %s %s", ev.Kind, ev.Subject, ev.Predicate, ev.Object)
		}
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

type configTraceDiagramAnchor struct {
	Role  string
	Label string
	Score int
}

func renderAnswerDocDiagramConfigTraceSeed(ctx *types.AgentContext) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.AnalysisIR.RequestModel.Scenario != types.ScenarioConfigTrace {
		return ""
	}
	anchors := collectConfigTraceDiagramAnchors(ctx)
	if len(anchors) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("For config-precedence questions, prefer a precedence diagram with grounded source labels instead of numbered layers. Reuse this skeleton when it matches the evidence:\n\n```\n")
	for i, anchor := range anchors {
		b.WriteString(anchor.Label)
		b.WriteByte('\n')
		if i < len(anchors)-1 {
			b.WriteString("  ->\n")
		}
	}
	b.WriteString("```\n")
	b.WriteString("If you only need part of the chain, delete unused nodes rather than inventing new layer labels. If you introduce a different file / symbol / path label, ground it first.")
	return strings.TrimSpace(b.String())
}

func collectConfigTraceDiagramAnchors(ctx *types.AgentContext) []configTraceDiagramAnchor {
	if ctx == nil {
		return nil
	}
	roleOrder := []string{"default", "yaml", "runtime", "override"}
	best := make(map[string]configTraceDiagramAnchor, len(roleOrder))
	appendCandidate := func(ev types.EvidenceItem) {
		role, score := classifyConfigTraceDiagramRole(ev)
		if role == "" || ev.Source == "" {
			return
		}
		label := formatConfigTraceDiagramAnchorLabel(ev)
		if label == "" {
			return
		}
		if cur, ok := best[role]; ok && cur.Score >= score {
			return
		}
		best[role] = configTraceDiagramAnchor{Role: role, Label: label, Score: score}
	}
	for _, chain := range ctx.AnswerChains {
		appendCandidate(chain.Item)
	}
	for _, ev := range ctx.EvidenceItems {
		appendCandidate(ev)
	}
	var out []configTraceDiagramAnchor
	for _, role := range roleOrder {
		if anchor, ok := best[role]; ok {
			out = append(out, anchor)
		}
	}
	return out
}

func classifyConfigTraceDiagramRole(ev types.EvidenceItem) (string, int) {
	text := strings.ToLower(strings.Join(filterEmptyStrings(
		ev.Subject, ev.Predicate, ev.Object, ev.AnchorSymbol, ev.Summary, ev.Source,
	), " "))
	source := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(ev.Source), `\`, `/`))
	if strings.HasSuffix(source, "_test.go") || strings.Contains(source, "/testdata/") {
		return "", 0
	}
	switch {
	case strings.Contains(text, "default") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(ev.Subject)), "default"):
		return "default", 16 + configTraceLineScore(ev)
	case strings.Contains(source, ".yaml") || strings.Contains(source, ".yml") || strings.Contains(text, "yaml"):
		return "yaml", 14 + configTraceLineScore(ev)
	case strings.Contains(source, "/cmd/") || strings.Contains(text, "cli") || strings.Contains(text, "flag") || strings.Contains(text, "override"):
		return "override", 12 + configTraceLineScore(ev)
	case strings.Contains(source, "/config/") || strings.Contains(text, "runtime") || strings.Contains(text, "bind") || strings.Contains(text, "map"):
		return "runtime", 10 + configTraceLineScore(ev)
	default:
		return "", 0
	}
}

func configTraceLineScore(ev types.EvidenceItem) int {
	if ev.LineStart > 0 {
		return 2
	}
	return 0
}

func formatConfigTraceDiagramAnchorLabel(ev types.EvidenceItem) string {
	label := strings.TrimSpace(ev.Source)
	if label == "" {
		return ""
	}
	if ev.LineStart > 0 {
		label = fmt.Sprintf("%s:%d", label, ev.LineStart)
	}
	name := strings.TrimSpace(firstNonEmptyString(ev.AnchorSymbol, ev.Subject))
	if name != "" && !strings.Contains(label, name) {
		label += " " + name
	}
	return label
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
	justification := strings.TrimSpace(ctx.Mutable.AbsenceJustification())
	stateAbsent := contract.AllowAbsence && justification != "" && len(pending) > 0

	var b strings.Builder
	b.WriteString("## Exact Resolution Contract\n\n")
	fmt.Fprintf(&b, "- Requested exact %s: %s\n", label, backtickJoin(contract.Targets))
	b.WriteString("- Resolve the requested exact target directly. Do not answer with a nearby item unless you have explicit alias / synonym / parser-mapping proof.\n")
	if contract.RequireTargetMention {
		b.WriteString("- Name the requested exact target explicitly in `summary`.\n")
	}
	if contract.AllowAbsence {
		b.WriteString("- Absence-only is acceptable if the investigation shows the exact target is absent.\n")
	}
	if contract.AliasRequiresProof {
		b.WriteString("- Any alias / equivalent / substitute claim requires explicit grounded proof, not lexical similarity or \"closest match\" reasoning.\n")
	}
	if hint := strings.TrimSpace(contract.RelatedContextScopeHint); hint != "" {
		fmt.Fprintf(&b, "- If you add related context, keep it within the %s and ground it.\n", hint)
	} else {
		b.WriteString("- If you add related context, keep it grounded and clearly separate it from the exact target resolution.\n")
	}
	if stateAbsent {
		b.WriteString("- Investigation state: the exact target is currently absent in the repo / branch under inspection.\n")
		fmt.Fprintf(&b, "- Absence justification: %s\n", justification)
		b.WriteString("- Lead with the exact absence before any related context.\n")
	}
	b.WriteString("\n")
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

func collectExactResolutionSeeds(ctx *types.AgentContext, contract *types.ExactResolutionContract) []exactResolutionSeed {
	if ctx == nil || contract == nil {
		return nil
	}
	targetTerms := types.ExactResolutionTerms(contract)
	scopeTerms := types.ExactResolutionScopeTerms(contract)
	if len(targetTerms) == 0 && len(scopeTerms) == 0 {
		return nil
	}

	type candidate struct {
		ev    types.EvidenceItem
		score int
	}
	var candidates []candidate
	appendCandidate := func(ev types.EvidenceItem, base int) {
		if ev.Source == "" {
			return
		}
		score := base + scoreExactResolutionEvidence(ev, contract, targetTerms, scopeTerms)
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
			Text:  formatExactResolutionSeed(cand.ev),
			Score: cand.score,
		})
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func scoreExactResolutionEvidence(ev types.EvidenceItem, contract *types.ExactResolutionContract, targetTerms, scopeTerms []string) int {
	score := 0
	if contract == nil {
		return score
	}
	text := strings.ToLower(strings.Join([]string{
		ev.Subject, ev.Predicate, ev.Object, ev.AnchorSymbol, ev.Summary, ev.Source,
	}, " "))
	sourceLower := strings.ToLower(ev.Source)
	isTestLike := strings.HasSuffix(sourceLower, "_test.go") || strings.Contains(sourceLower, "/testdata/") || strings.Contains(sourceLower, "\\testdata\\")
	exactMention := false
	for _, target := range contract.Targets {
		if types.ExactResolutionTextMentionsTarget(contract, text, target) {
			exactMention = true
			break
		}
	}
	if exactMention {
		if isTestLike {
			score += 4
		} else {
			score += 18
		}
	}
	targetMatches := 0
	for _, term := range targetTerms {
		if strings.Contains(text, term) {
			targetMatches++
		}
	}
	if targetMatches > 3 {
		targetMatches = 3
	}
	score += targetMatches * 3
	scopeMatches := 0
	for _, term := range scopeTerms {
		if strings.Contains(text, term) {
			scopeMatches++
		}
	}
	if scopeMatches > 3 {
		scopeMatches = 3
	}
	score += scopeMatches * 4
	switch ev.Kind {
	case types.EvidenceMechanism, types.EvidenceRelationship, types.EvidenceRegistration:
		score += 6
	case types.EvidenceDirect, types.EvidenceConcrete:
		score += 4
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
func (e *answerDocumentEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase == PhaseMidLoop {
		// emit_answer_document is the finalizer's terminal action —
		// once it fires, stop immediately instead of burning one extra
		// LLM round that would just produce a content-only soft-stop.
		if e.mu != nil {
			if doc := e.mu.AnswerDocument(); doc != nil && !doc.IsZero() {
				return LoopSignal{StopRequested: true, StopReason: "emit_answer_document called"}
			}
		}
		if sig := e.emitAnswerDocumentRejectSignal(obs); sig.HintRequested {
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

func (e *answerDocumentEvaluator) emitAnswerDocumentRejectSignal(obs LoopObservation) LoopSignal {
	if obs.LastToolResult == nil || obs.LastToolResult.ToolName != "emit_answer_document" || obs.LastToolResult.Success {
		return LoopSignal{}
	}
	if e.maxRetries > 0 && e.rejectHintsUsed >= e.maxRetries {
		return LoopSignal{}
	}
	summary := strings.TrimSpace(obs.LastToolResult.Summary)
	if summary == "" {
		return LoopSignal{}
	}

	hint := "Your last `emit_answer_document` call was rejected by the tool. Fix the exact validation error from the tool result and re-emit `emit_answer_document` now. Do not write free-form prose outside the tool call."
	reasonKey := "tool-reject"

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

	if strings.Contains(summary, "diagram required for this dispatch") {
		reasonKey = "missing-diagram"
		hint = "Your last `emit_answer_document` call was rejected because this dispatch REQUIRES a grounded diagram in `summary`. Re-emit `emit_answer_document` now with the same answer shape and payload fields, but add at least one grounded triple-backtick diagram to `summary`. This obligation is independent of answer shape. Keep every filename inside the diagram grounded by citations[] or the Log Triage frames; do not write free-form prose outside the tool call."
	}
	if strings.Contains(summary, "references file(s) not present in citations[] or attached-log frames") {
		reasonKey = "diagram-grounding"
		hint = "Your last `emit_answer_document` call was rejected by the DIAGRAM-GROUNDING gate: the fenced diagram renamed or introduced file/path labels that are not grounded. Re-emit `emit_answer_document` now with the same answer, but inside the diagram reuse the exact grounded file / symbol / path labels from citations, cited line text, or Log Triage frames. Do NOT normalize one grounded label into a different spelling unless that alternate label is itself grounded. Prefer direct grounded node names over abstract aliases. Do not write free-form prose outside the tool call."
	}
	if strings.Contains(summary, "summary introduces codename label(s) not present in any citation's") {
		reasonKey = "diagram-codename"
		hint = "Your last `emit_answer_document` call was rejected by the CODENAME-GROUNDING gate: the summary introduced abstract enumeration labels that are not grounded. Re-emit `emit_answer_document` now with the same answer, but remove invented labels such as `Level 1` / `Round 2` / `Step 3` unless those exact tokens are cited. Label the diagram directly with grounded files, functions, config keys, or other evidenced entities instead. Do not write free-form prose outside the tool call."
	}
	if strings.Contains(summary, "exact-resolution contract violated:") {
		reasonKey = "exact-resolution"
		hint = "Your last `emit_answer_document` call was rejected by the exact-resolution contract. Re-emit `emit_answer_document` now with the same grounded answer, but make `summary` explicitly name the requested exact target and resolve it first. If the investigation concluded the exact target is absent, lead with that absence; absence-only is acceptable. Any nearby context must stay clearly labeled as related context, not as an equivalent, alias, or substitute without explicit proof. Do not write free-form prose outside the tool call."
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
	if strings.Contains(summary, "not corroborated by citations[") {
		reasonKey = "literal-grounding"
		hint = "Your last `emit_answer_document` call was rejected by the LITERAL-GROUNDING gate: the cited file:line does NOT contain `value.literal`, i.e. the citation does not back the answer. " +
			"The single-action fix: re-emit now with `value.citation_ref = -1` AND add a sentence to `summary` stating the literal is drawn from the attached log / external source (no grounded repo citation). " +
			"Do NOT try to find a different file:line — if the literal came from an external trace (panic frame, log function name, etc.), no repo citation exists by definition and -1 is the tool-schema-legal escape. " +
			"Full tool error: " + strings.SplitN(summary, "\n", 2)[0]
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
		combined := warning
		if lastContent != "" {
			combined = warning + "\n\n" + lastContent
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
	priorProse := findLastPreToolCallDraft(messages)
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
