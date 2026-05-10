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

	// emitFullDocFailStreak (2026-05-10 P3) counts consecutive
	// failures of emit_answer_document (NOT _patch) within the
	// current dispatch. After ≥ 2 failures the evaluator emits a
	// nudge suggesting the LLM switch to emit_answer_document_patch
	// for the next attempt. Reset on a successful emit OR a
	// successful patch attempt (the LLM has accepted the
	// switch and we don't need to keep nagging).
	emitFullDocFailStreak int

	// emitPatchNudgeFired latches once the LLM is observed calling
	// emit_answer_document_patch (success or failure) — i.e. the LLM
	// has SEEN and acted on the switch-to-patch nudge. Until that
	// happens the nudge re-fires on each subsequent
	// emit_answer_document failure, capped by the per-HintKey cap
	// (P2 MaxPerKeyInjects=5).
	//
	// The 2026-05-10 sweep digest forensic (s7b finalizer iter=1)
	// exposed why the original one-shot semantics were wrong: the
	// nudge fired at iter=1, was throttled by MinInjectInterval (a
	// prior tool-reject hint had injected at iter=0), and the
	// one-shot guard then prevented it from re-firing at iter=2-4
	// when the throttle had cleared. The LLM never saw the nudge
	// and retried emit_answer_document four more times. The fix
	// pairs this latch with BypassThrottle=true on the LoopSignal
	// so cross-key spacing can't swallow the nudge a second time.
	emitPatchNudgeFired bool

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

	// W3.3 (2026-05-05): one-line pointer to the schema's
	// authoritative PRE-EMIT CONSTRAINTS section. The prompt below
	// gives rationale + workflow + per-dispatch dynamic data; the
	// answer-document tool description has the structural
	// constraints (block contracts, claim_uses, edge_anchors,
	// enumeration labels, declared count) that the validator
	// enforces — when the prompt and the tool description disagree
	// on emit shape, the tool description is authoritative.
	b.WriteString("> Note — structural emit-time constraints are listed under PRE-EMIT CONSTRAINTS in the answer-document tool's description. This prompt focuses on rationale + workflow + per-dispatch data. When in doubt about emit shape, the tool description is authoritative.\n\n")

	// R14 (post_shape_residual_audit.md, 2026-05-04): render
	// typed retry-state surface BEFORE any other dynamic content
	// when this dispatch is a retry. The LLM sees Previous Emit
	// + Active Violations + Required Changes + Hard Rule at the
	// top, anchoring the retry around prev typed-state instead
	// of regenerating from scratch.
	if retryState := renderAnswerDocRetryState(ctx); retryState != "" {
		b.WriteString(retryState)
	}

	// User question is already rendered by builder.go as "User Request"
	// section — no need to repeat it here.

	// V2 block contract section carries the family-aware block
	// requirements that drive prompt structure.
	if blockContract := renderAnswerDocBlockContract(ctx); blockContract != "" {
		b.WriteString(blockContract)
	}

	// view is the typed runtime semantic contract; downstream prompt
	// branches gate on view obligations (NeedsPrincipalScalar /
	// NeedsOrderedPrincipalList / NeedsEnumerationSlate /
	// AllowsAnchorSkeleton) instead of the legacy AnswerShape enum.
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)

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
	if capability := renderAnswerDocCapabilitySurface(ctx); capability != "" {
		b.WriteString(capability)
	}
	if facets := renderAnswerDocFacetCoverage(ctx); facets != "" {
		b.WriteString(facets)
	}
	if exact := renderAnswerDocExactResolutionContract(ctx); exact != "" {
		b.WriteString(exact)
	}
	if closure := renderAnswerDocAcceptedClosure(ctx); closure != "" {
		b.WriteString(closure)
	}
	if disposition := renderAnswerDocRuntimeGroundingDisposition(ctx); disposition != "" {
		b.WriteString(disposition)
	}
	if currentStatus := renderAnswerDocCurrentStatusDiagnostic(ctx); currentStatus != "" {
		b.WriteString(currentStatus)
	}
	renderedTypedSupport := false
	if support := renderAnswerDocSupportPlan(ctx); support != "" {
		renderedTypedSupport = true
		b.WriteString(support)
	}
	if drift := renderAnswerDocLogSourceDrift(ctx); drift != "" {
		b.WriteString(drift)
	}
	if !renderedTypedSupport {
		if observations := renderAnswerDocExternalObservationSeeds(ctx); observations != "" {
			b.WriteString(observations)
		}
	}
	if checklist := renderAnswerDocSubmissionChecklist(ctx, view, e.diagramRequired); checklist != "" {
		b.WriteString(checklist)
	}
	if backbone := renderAnswerDocStepBackbone(ctx, view); backbone != "" {
		b.WriteString(backbone)
	}
	if boundary := renderAnswerDocEnumerationBoundary(ctx, view); boundary != "" {
		b.WriteString(boundary)
	}
	if ctx != nil && ctx.AnalysisIR != nil && types.IsScalarSourceLiteralLookup(ctx.AnalysisIR.RequestModel) {
		b.WriteString("## Scalar Lookup Discipline\n\n")
		b.WriteString("- This dispatch asks for one named source-code literal, not for a walkthrough of the surrounding pipeline.\n")
		b.WriteString("- Keep the `summary` block's text narrow: identify the literal, give its grounded file:line location, and add only the minimal role sentence needed to justify why it is the answer.\n")
		b.WriteString("- The principal `scalar` block still requires a real `summary` block alongside it. The literal or decision alone is not a complete answer.\n")
		b.WriteString("- Do not expand into adjacent helpers, orchestrated stages, or nearby components unless the user explicitly asked how the mechanism works.\n")
		b.WriteString("- Every non-negative `citation_ref` on a scalar payload must point at a real entry in `citations[]`. Do not emit `citation_ref: 0` with an empty citations pool.\n")
		b.WriteString("- If you include a secondary citation beyond the defining one, it must still directly name or call/reference the SAME emitted literal. Drop type comments, nearby docstrings, or broad background citations even when they are grounded.\n")
		b.WriteString("- If a related-context evidence item mentions surrounding pipeline pieces, treat that as background noise rather than answer content.\n\n")
		if types.IsScalarRoleLocateLookup(ctx.AnalysisIR.RequestModel) {
			b.WriteString("- This is a role-locate lookup: the question names a clue or output, but the answer is the function / file / symbol that plays that role. Do not promote the clue itself into the exact target lane or the lead sentence.\n")
			b.WriteString("- For this kind of lookup, answer with the located literal and its file:line first. Mention surrounding pipeline stages only if they are strictly necessary to disambiguate the role.\n\n")
		}
	}

	if view.NeedsEnumerationSlate() {
		must := []string(nil)
		if ctx != nil && ctx.AnalysisIR != nil {
			must = ctx.AnalysisIR.AnswerContract.MustInclude
		}
		b.WriteString("## Expected principal-item floor\n\n")
		if len(must) > 0 {
			fmt.Fprintf(&b, "Required-symbol floor: **%d name(s)** — %s\n\n",
				len(must), strings.Join(must, ", "))
			fmt.Fprintf(&b,
				"Your principal enumeration block must preserve all %d grounded name(s). "+
					"If the investigation only established a lower bound or could not prove "+
					"the full set, disclose that bound in prose or a `caveat` block instead "+
					"of inventing a retired completeness field.\n\n", len(must))
		} else {
			b.WriteString("Required-member floor is empty. No explicit minimum member set is enforced for this dispatch — ")
			b.WriteString("keep the rendered list/table aligned with the prior extraction slate and surfaced evidence.\n\n")
		}

		if ctx != nil && len(ctx.AnswerSymbols) > 0 {
			b.WriteString("## Prior slate from the extraction pipeline\n\n")
			b.WriteString("The prior analysis phase produced this symbol list. ")
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
	if answerDocAllowSubTopicStructure(ctx, view) {
		b.WriteString("## Answer Structure (multi-topic)\n\n")
		b.WriteString("The user asked about multiple topics. " +
			"Your principal blocks MUST address each one with a clearly labeled section (use `section` blocks per topic OR a single `summary` block whose text carries one labeled section per topic):\n\n")
		for i, st := range ctx.AnalysisIR.RequestModel.SubTopics {
			fmt.Fprintf(&b, "%d. %s\n", i+1, st.Summary)
		}
		b.WriteString("\nProvide citations for each section.\n\n")

		// Anchor skeleton: when the extractor produced a per-topic
		// anchor slate (answer-symbols emitted during Turn B for
		// multi-topic explanation questions, sub_topics ≥ 1), echo
		// those anchors in the finalizer prompt so the LLM re-emits
		// them as items inside an optional `ordered_list` Key Anchors
		// block alongside the principal blocks. The renderer draws
		// the Key Anchors block beneath the summary block when the
		// LLM emits one. This pins the load-bearing identifiers in
		// the rendered output and stops the finalizer from
		// synthesizing prose that drifts from Turn A's evidence.
		if view.AllowsAnchorSkeleton(ctx.AnalysisIR.RequestModel) && len(ctx.AnswerSymbols) > 0 {
			b.WriteString("### Anchor skeleton (emit as one optional ordered_list block)\n\n")
			b.WriteString("The extractor produced these per-sub-topic anchors. " +
				"Re-emit them verbatim as an additional `ordered_list` block (one item per anchor; each item carries `id`, `label=<symbol-name>`, `text=<rationale>`, top-level `citation_ref=N` (zero-based index into doc.citations[])), and declare the block-level `claim_uses=[{claim_form=definition_fact}]` (or whichever claim_form matches the cited lines). " +
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

func answerDocAllowSubTopicStructure(ctx *types.AgentContext, view *types.AnswerSemanticView) bool {
	if ctx == nil || ctx.AnalysisIR == nil || len(ctx.AnalysisIR.RequestModel.SubTopics) <= 1 {
		return false
	}
	if view != nil && view.Family == types.QFRootCauseTrace && answerSupportPlan(ctx) != nil {
		return false
	}
	return true
}

func renderAnswerDocSubmissionChecklist(ctx *types.AgentContext, view *types.AnswerSemanticView, diagramRequired bool) string {
	var items []string
	if view != nil {
		// Walk RequiredBlocks and emit one prose item per principal
		// obligation. Each Required block at SurfacePrincipal is a
		// load-bearing payload the LLM must fill; principal blocks
		// at non-principal hints are surfaced via their facet/diagram
		// sections elsewhere in this prompt.
		seenKinds := map[types.AnswerBlockKind]bool{}
		for _, br := range view.RequiredBlocks {
			if !br.Required || br.SurfaceRoleHint != types.SurfacePrincipal {
				continue
			}
			if seenKinds[br.Kind] {
				continue
			}
			seenKinds[br.Kind] = true
			switch br.Kind {
			case types.BlockScalar:
				switch view.Family {
				case types.QFConfigPrecedence:
					items = append(items,
						"Emit the principal `scalar` block with the literal in block `text`, and attach a one-element `items=[{id:\"v\", citation_ref:N}]` when you need a citation anchor. Attach block-level `claim_uses=[{claim_form=definition_fact}]` (plural array). Use `assignment_fact` when the cited line is a config / variable assignment.",
						"Fill the block's `text` (or the lead summary block) with prose that names the config key / subject and states how the literal was obtained (lookup / file:line / chain).",
						"Keep the complete scalar answer inside the answer document blocks: the literal belongs in the principal `scalar` block, and any framing prose belongs in the `summary` block or the scalar block's `title`.",
					)
				default:
					items = append(items,
						"Emit the principal `scalar` block with the literal in block `text`, and attach a one-element `items=[{id:\"v\", citation_ref:N}]` when you need a citation anchor. Attach block-level `claim_uses=[{claim_form=definition_fact}]` (plural array). Use `external_observation` when the literal is from an attached log / external trace.",
						"Fill the block's `text` (or the lead summary block) with prose that names the subject being measured and states how the literal was obtained (lookup / file:line / command / chain).",
						"Keep the complete scalar answer inside the answer document blocks: the literal belongs in the principal `scalar` block, and any framing prose belongs in the `summary` block or the scalar block's `title`.",
					)
				}
			case types.BlockOrderedList:
				if br.SurfaceRoleHint == types.SurfacePrincipal && view.Family == types.QFEnumeration {
					items = append(items,
						"Emit the principal `ordered_list` block with `items[]` (each item carries `id`, optional `label`, `text`, top-level `citation_ref=N` (zero-based index into doc.citations[])); declare the block-level `claim_uses=[{claim_form=definition_fact}]` (or `call_edge` / `assignment_fact` when the cited lines are call sites or assignments). EVERY item.label MUST be grounded in the evidence pool: prefer a verbatim anchor_symbol / subject / object, or a selector-qualified identifier visibly present on a grounded snippet line (for example a qualified call or type name). Fabricated labels are rejected by the structural enumeration grounding oracle.",
						"Preserve every grounded member from the prior slate / required-member floor. If the investigation only established a lower bound or an unknown full set, disclose that bound in prose or a `caveat` block; do NOT invent a retired completeness field.",
						"Use the lead summary block to frame what the list enumerates; do not let the list stand alone without context.",
					)
				} else {
					items = append(items,
						"Emit the principal `ordered_list` block with `items[]` of ordered logical hops. Per-item top-level `citation_ref=N` (zero-based index into doc.citations[]). Declare the block-level `claim_uses[]` to cover every form the items use — list one entry per form (`call_edge` for caller→callee sites, `guard_condition` for conditional branches, `return_fact` for return-value hops).",
						"Keep the hop-by-hop detail in items[] `text`; the lead summary block is only the lead-in.",
						"Do not invent shorthand labels from citation line numbers (for example `L877` or `Line 42`) unless that exact token is itself grounded in cited text.",
						"Keep each cited item at the abstraction directly named by its own citation. If one hop needs both a guard/condition and a downstream action that are named on different lines, split the hop or cite the line that actually names the action.",
					)
				}
			case types.BlockSection:
				items = append(items,
					"Emit one `section` block per layer / component / topic, each with a grounded `title` and prose `text`. When the user-section contract lists allowed `claim_form` values for this block, attach block-level `claim_uses=[{claim_form=definition_fact}]` (plural array). Section blocks have no built-in citation field — if the section needs a citation, restructure to put the cited fact in a child scalar/list block where citation_ref lives natively.",
				)
			case types.BlockTable:
				items = append(items,
					"Emit the principal `table` block with the markdown table inside its `text`; each row corresponds to a grounded entity, with citations[] entries for every cited file:line.",
				)
			case types.BlockSummary:
				items = append(items,
					"The `summary` block is REQUIRED and (when no other principal block exists) is the main answer body for this dispatch.",
				)
			case types.BlockDiagram:
				items = append(items,
					"Emit the `diagram` block with `diagram.kind` set to the SEMANTIC family the contract names (`flow` / `sequence` / `architecture` / `call_dag` — NOT a Mermaid keyword) and Mermaid syntax inside `diagram.body` with `diagram.language=\"mermaid\"`.",
				)
			case types.BlockDecision:
				items = append(items,
					"Emit the principal `decision` block with the verdict at the START of block `text`, followed by the rationale prose. Attach a one-element `items=[{id:\"d\", citation_ref:N}]` when you need a citation anchor, and attach block-level `claim_uses=[{claim_form=guard_condition|definition_fact}]` (plural array).",
					"Keep the complete decision answer inside the answer document blocks: the verdict and rationale belong in the principal `decision` block, with any extra framing prose in a `summary` or `caveat` block as needed.",
				)
			case types.BlockCaveat:
				items = append(items,
					"Emit a `caveat` block with honesty marker prose inside `text`. When the caveat names a search confirmed absent, set block-level `claim_uses=[{claim_form=absence_fact}]`; when the caveat reports an external runtime/log observation that diverges from repo, set `claim_uses=[{claim_form=external_observation}]`; otherwise leave claim_uses off (caveat blocks rarely require a typed claim_form).",
				)
			}
		}
		if !view.AllowsAnchorSkeleton(answerDocRequestModel(ctx)) && view.Family != types.QFEnumeration {
			items = append(items,
				"Do NOT add an enumeration anchor skeleton block unless the prompt explicitly attached an Anchor skeleton section for a multi-topic explanation.",
			)
		}
	}
	if ctx != nil && ctx.AnalysisIR != nil && ctx.AnalysisIR.AnswerContract.ExactResolution != nil {
		items = append(items,
			"Fill `exact_resolution{status,anchor?,context_mode}` to match the current exact-target state; do not leave the status implicit in prose.",
		)
		if view != nil && len(view.MissingRequestedRoles) > 0 {
			items = append(items,
				"Populate document-level `missing_requested_roles[]` with every user-requested precedence layer that remained unbound in this dispatch. Each entry is `{role:<default|config|runtime|override>, label?:<user bucket label>}`. The renderer materialises the explicit missing-layer prose from this typed field, so do not hide missing requested layers behind `N/A` / `not applicable` / vague placeholders.",
			)
		}
	}
	items = append(items,
		"Keep the answer at the abstraction already grounded by the evidence. A cited struct / function / type name does NOT license an invented field inventory, member count, default table, or exhaustive list unless a cited line or structured evidence item explicitly enumerates those members.",
	)
	if ctx != nil && ctx.LogTriage != nil && len(ctx.LogTriage.Errors) > 0 {
		items = append(items,
			"If this answer explains an attached log / stack trace, name each structured log error type or exception identifier from Log Triage at least once in the `summary` block's text. Do not paraphrase the type name away.",
		)
		if explicit := renderRequiredLogTriageTypes(ctx.LogTriage); explicit != "" {
			items = append(items, explicit)
		}
	}
	if diagramRequired {
		items = append(items,
			// PREFERRED form must be Mermaid — duplicate the
			// preference here so this checklist agrees with the
			// Diagram Contract section. ASCII art is a fallback
			// only when the Mermaid subset cannot express the shape.
			"This dispatch must include at least one grounded fenced diagram via a `diagram` block (preferred — `diagram.kind` ∈ flow / sequence / architecture / call_dag, `diagram.language=\"mermaid\"`) OR a fenced block embedded in the `summary` block's text. PREFERRED Mermaid: for flow / architecture / call_dag use `flowchart TD` by default (switch direction only when it materially improves readability); for sequence use `sequenceDiagram`, but keep participant labels short because actors render horizontally. ASCII art is the fallback only.",
			"When a `Diagram Seeds` / `First-Pass Diagram Reference` section is present, treat its grounded labels as a FLOOR you can extend, not a verbatim ceiling. You SHOULD add additional grounded nodes / branches / fan-out when your investigation supports a richer mechanism. The hard rule: every file/path label inside the fenced block stays grounded in citations[] or Log Triage frames.",
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

func answerSurfacePlan(ctx *types.AgentContext) *types.AnswerSurfacePlan {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	plan := types.BuildAnswerSurfacePlanForAgentContext(ctx)
	if plan != nil && len(ctx.AnswerSymbols) > 0 {
		types.ApplyAnswerSymbolStepBackbone(plan, ctx.AnalysisIR, ctx.AnswerSymbols, ctx.AnswerSymbolCompleteness)
	}
	return plan
}

func answerSupportPlan(ctx *types.AgentContext) *types.AnswerSupportPlan {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return types.BuildAnswerSupportPlanForAgentContext(ctx)
}

func answerDocRequestModel(ctx *types.AgentContext) types.RequestModel {
	if ctx == nil || ctx.AnalysisIR == nil {
		return types.RequestModel{}
	}
	return ctx.AnalysisIR.RequestModel
}

func renderAnswerDocStepBackbone(ctx *types.AgentContext, view *types.AnswerSemanticView) string {
	// Step backbone is a hop-chain anchor list — it applies to
	// trace / root-cause families specifically (not plain enumeration
	// of items). Use Family directly so we can distinguish hop chain
	// vs enumeration slate while both produce a principal ordered list.
	if view == nil || (view.Family != types.QFCallChain && view.Family != types.QFRootCauseTrace) {
		return ""
	}
	// When a typed AnswerSupportPlan is present for drift-bounded
	// root-cause answers, it becomes the authoritative principal-input
	// contract. Keeping the older step-backbone scaffold alongside it
	// reintroduces summary/rationale prose from answer symbols and can
	// overpower the stricter support lanes with a more speculative
	// sequence. Suppress the legacy scaffold in that mode.
	if view.Family == types.QFRootCauseTrace && answerSupportPlan(ctx) != nil {
		return ""
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || len(plan.StepBackbone) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Resolved Step Sequence\n\n")
	switch plan.StepBackboneCompleteness {
	case types.CompletenessComplete:
		b.WriteString("The analysis phase identified a complete ordered sequence for this step list. Use these anchors as the default core sequence of the principal `ordered_list` block's `items[]`.\n\n")
	case types.CompletenessLowerBound:
		b.WriteString("The analysis phase identified an ordered lower-bound sequence for this step list. Keep these anchors in order in the principal `ordered_list` block's `items[]`, and only add more cited items when another grounded line independently supports them.\n\n")
	default:
		b.WriteString("The analysis phase identified an ordered grounded sequence for this step list. Use these anchors as the default core sequence of the principal `ordered_list` block's `items[]`.\n\n")
	}
	b.WriteString("- Keep each cited item at the abstraction directly corroborated by its own citation.\n")
	b.WriteString("- If a cited anchor is a call site / assignment / guard, describe that call site / assignment / guard there; helper internals belong in a separately grounded item or in the `summary` block's text.\n")
	b.WriteString("- Do not merge one anchor's citation with semantics that only appear in another file / definition.\n\n")
	for i, anchor := range plan.StepBackbone {
		desc := types.RenderStepSurfaceAnchorDescription(anchor)
		if anchor.File != "" && anchor.Line > 0 {
			fmt.Fprintf(&b, "%d. `%s` (%s:%d)", i+1, anchor.Name, anchor.File, anchor.Line)
		} else {
			fmt.Fprintf(&b, "%d. `%s`", i+1, anchor.Name)
		}
		if desc != "" {
			fmt.Fprintf(&b, " — %s", desc)
		}
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	return b.String()
}

func renderAnswerDocEnumerationBoundary(ctx *types.AgentContext, view *types.AnswerSemanticView) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	rm := ctx.AnalysisIR.RequestModel
	qs := rm.QuestionStructure()
	// Plan E (2026-05-02): when ANY axis is populated, render the
	// per-axis instructions. Pre-Plan-E behaviour preserved when
	// only EnumerationBoundary is set (no Completeness, no Buckets).
	if !qs.HasAnyObligation() {
		return ""
	}
	boundary := rm.EnumerationBoundary
	hasBoundary := boundary != nil && boundary.DeclaredCount > 0 && strings.TrimSpace(boundary.SourceQuote) != ""
	hasCompleteness := qs.CompletenessObligation.IsActive()
	hasBuckets := len(qs.Buckets) >= 2
	var b strings.Builder
	b.WriteString("## Requested Set Boundary\n\n")
	if hasBoundary {
		fmt.Fprintf(&b, "The user explicitly asked for a bounded principal set: `%s` (%d item(s)). Preserve that boundary in the main answer body.\n\n",
			boundary.SourceQuote, boundary.DeclaredCount)
	}
	if hasCompleteness {
		fmt.Fprintf(&b, "The user demanded an exhaustive answer (`%s` in the question). Every grounded match must appear in the rendered answer. If the investigation legitimately could not determine the full set, disclose that bound explicitly in prose or a `caveat` block — do not invent a separate completeness payload field.\n\n",
			qs.CompletenessObligation.SourceQuote)
	}
	if hasBuckets {
		labels := make([]string, 0, len(qs.Buckets))
		for _, bk := range qs.Buckets {
			labels = append(labels, fmt.Sprintf("`%s`", bk.Label))
		}
		fmt.Fprintf(&b, "The user partitioned the answer into %d named groups: %s. Each label MUST appear verbatim in your rendered answer (a heading inside the `summary` block's text is the preferred surface; an item label inside an `ordered_list` / `bullet_list` block is also acceptable). The user's mental partition must survive end-to-end.\n\n",
			len(qs.Buckets), strings.Join(labels, ", "))
	}
	if !hasBoundary {
		// No count axis — skip the per-shape count rules below.
		return b.String()
	}
	switch {
	case view != nil && (view.Family == types.QFCallChain || view.Family == types.QFRootCauseTrace):
		// Hop-chain principal payload: keep items[] count to the
		// declared boundary. The orchestrator's contract.Check fires
		// declared_count_drift (soft) on item-count below the bound.
		fmt.Fprintf(&b, "- Keep the principal `ordered_list` block's `items[]` slate to %d step(s) — these are the items the question enumerated.\n", boundary.DeclaredCount)
		b.WriteString("- Each item is one logical hop; do not collapse two distinct hops into a single item to fit the count, and do not pad with narration items just to hit the number.\n")
	case view != nil && view.Family == types.QFEnumeration:
		fmt.Fprintf(&b, "- Keep the principal `ordered_list` block's `items[]` slate to %d item(s) when the grounded evidence supports that boundary.\n", boundary.DeclaredCount)
	default:
		fmt.Fprintf(&b, "- Keep the principal enumerated set (the `ordered_list` / `bullet_list` block's `items[]` AND any per-topic items mentioned in the `summary` block) to %d principal item(s).\n", boundary.DeclaredCount)
	}
	b.WriteString("- If current code also contains adjacent guards, helpers, or later-added side conditions around the same owner, do not silently blend them into the principal set.\n")
	b.WriteString("- Mention auxiliary items as a short caveat or follow-on note after the principal set, and only when grounded evidence makes the distinction clear.\n")
	b.WriteString("- Prefer members declared/appended in the same grounded owner file before drifting to helper definitions in sibling files.\n\n")
	return b.String()
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

func renderAnswerDocCapabilitySurface(ctx *types.AgentContext) string {
	return renderCapabilityAuthoritySection(detectStageToolCapabilityQueryFromContext(ctx), "Capability Surface Authority")
}

// answerDocClaimFormLabels names each ClaimForm in evidence-shape
// vocabulary the LLM already understands. Used to render the
// AcceptableForms whitelist as a human-readable hint per facet.
var answerDocClaimFormLabels = map[types.ClaimForm]string{
	types.ClaimDefinitionFact:      "definition citation",
	types.ClaimAssignmentFact:      "assignment / config-leaf citation",
	types.ClaimReturnFact:          "return-value citation",
	types.ClaimCallEdge:            "call-edge citation",
	types.ClaimImportEdge:          "import / dependency-edge citation",
	types.ClaimGuardCondition:      "guard / branch-condition citation",
	types.ClaimPrecedenceRole:      "precedence-role citation",
	types.ClaimExternalObservation: "log / perf observation",
	types.ClaimAbsenceFact:         "grounded absence (negative scope)",
}

// renderAnswerDocFacetCoverage emits the "Required Answer Facets"
// section of the finalizer prompt. Source of truth: the compiled
// FacetCoverageContract on AnswerSurfacePlan (Phase 1). Phase 2 is
// prompt-only — no hard gate fires from this section; LLM may
// rebalance the answer based on the facets listed but is not rejected
// for missing them. Phase 4 will add validators.
//
// Returns "" when the contract is empty or nil so existing prompt
// shapes stay byte-identical.
// renderAnswerDocBlockContract renders the V2 block contract for the
// finalizer prompt (B6-T1, block_only_carrier.md §5.6). Returns "" when
// no semantic view can be compiled — V2 is the only carrier since B8.
//
// LLM-facing language only (R4 red line); no internal Go terminology.
// The two sub-sections are:
//
//	## Required Answer Blocks   — what kinds of blocks the answer
//	                              must include (Summary/Section/
//	                              OrderedList/Diagram/etc.) + min/max
//	                              counts + the family rationale.
//
// renderAnswerDocRetryState (R14-c5..c8, post_shape_residual_audit.md
// 2026-05-04) is the unified retry-state prompt section. Rendered
// at the TOP of BuildInitialInstruction when ctx.EmitStageRetryAttempt > 0
// AND ctx.Mutable.RetryState() carries content.
//
// Renders 4 sections in fixed order:
//
//  1. ## Hard Rule (top) — preserve every unchanged field byte-identical.
//  2. ## Required Changes — typed FieldPath + Repair text per violation,
//     grouped by Severity (Critical first, then High, Medium; Soft skipped).
//  3. ## Active Violations — full set, by Severity + Layer, so the
//     LLM has cross-layer visibility (R13 fix).
//  4. ## Previous Emit — typed projection of prev emit (R6/R6.1 fix:
//     LLM sees what fields it already filled).
//
// Returns "" when no retry state to render (preserves byte-identical
// pre-R14 behaviour on fresh dispatches).
func renderAnswerDocRetryState(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	rs := ctx.Mutable.RetryState()
	if rs == nil || !rs.HasContent() {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Hard Rule (retry attempt %d)\n\n", rs.Attempt)
	b.WriteString("Your last `emit_answer_document` was rejected. Re-emit by **starting from your previous payload (shown in 'Previous Emit' below) and changing ONLY the field paths listed in 'Required Changes' below**. Every other field — `blocks[].id`, `blocks[].kind`, `blocks[].facet_ids`, `blocks[].claim_uses[]`, `blocks[].surface_role`, `blocks[].items[]`, `citations[]`, `exact_resolution` — MUST appear byte-identical to your Previous Emit. Do NOT regenerate from scratch. The validator will reject any retry that loses fields you already filled correctly.\n\n")

	// 2. Required Changes (sorted by severity desc, soft excluded).
	if changes := renderRetryRequiredChanges(rs); changes != "" {
		b.WriteString(changes)
	}

	// 3. Active Violations (full set, grouped by severity then layer).
	if active := renderRetryActiveViolations(rs); active != "" {
		b.WriteString(active)
	}

	// 4. Previous Emit (typed summary).
	if prev := renderRetryPrevEmit(rs); prev != "" {
		b.WriteString(prev)
	}

	// 5. P2 (2026-05-10) — Live coverage status. Computes the
	// structural-coverage gaps the post-emit validators are about
	// to check (block kind balance, facet anchoring depth) and
	// surfaces them so the LLM sees them BEFORE re-emitting.
	// Only fires on retry (rs.HasContent already gated above);
	// 0-impact on first dispatch.
	if status := renderLiveCoverageStatus(ctx, rs); status != "" {
		b.WriteString(status)
	}

	// 6. P3 (2026-05-10) — Mechanical Fix Actions. Same input as
	// renderRetryRequiredChanges but rendered as structured
	// Action+Target+Value triples for the 4 typed validator-driven
	// cluster kinds (block_coverage_missing / facet_uncovered /
	// principal_claim_use_missing / uncertainty_block_missing).
	// Reduces "LLM has to re-read prose to figure out the fix" to
	// "LLM applies each row mechanically". Subjective concerns
	// (semantic_quality_underfilled / etc.) continue rendering
	// through the prose path above (Required Changes); both
	// channels coexist.
	if mech := renderRetryStructuredFixList(rs); mech != "" {
		b.WriteString(mech)
	}
	return b.String()
}

// renderRetryStructuredFixList is the P3 surface — emits a
// structured Action+Target+Value list for the violation kinds the
// post-emit chain consistently raises. Validator-driven kinds with
// a known mechanical template render here; subjective LLM-reviewer
// concerns (semantic_quality_underfilled, richness regressions)
// fall through to the existing prose path.
//
// Output shape:
//
//	## Mechanical Fix Actions
//
//	Apply each action below mechanically; each entry names a single
//	emit_answer_document field path to set OR a single block to add.
//
//	1. Action: `add_block`
//	   - Target: `blocks[]`
//	   - kind: `ordered_list`
//	   - hint: <one-line user-vocab Why>
//
//	2. Action: `set_field`
//	   - Target: `blocks[id="b1"].claim_uses`
//	   - Value: `[{claim_form: "definition_fact"}]`
//	   - hint: <one-line user-vocab Why>
//
// Empty when no covered ViolKind is in rs.ActiveViolations (the
// section silently disappears for non-typed retry shapes).
//
// Red-line audit (R3/R4/R5/R6/R7/SST/CN+EN-only):
//   - R3 typed: each Action / Target / Value comes from the
//     ScoredViolation's typed Kind / FieldPath / BlockID
//   - R4 generic: action names + schema fields are universal;
//     no project / case names appear
//   - R5: structured retry hint, never writes answer body
//   - R6 no internal vocab: action names (add_block / set_field /
//     add_caveat_block) are abstract instructions, not Go types;
//     schema field paths (blocks[].kind / blocks[id].claim_uses)
//     are the LLM's emit vocabulary already — they appear in the
//     skill prompt and in the JSON schema's examples
//   - R7 actionable: each row is one mechanical edit; LLM can
//     apply without prose interpretation
//   - SST: shape mirrors the existing Required Changes section
//     (## heading, numbered list)
//   - CN+EN-only: prompt is English (consistent with the rest
//     of the finalizer skill prose)
func renderRetryStructuredFixList(rs *types.RetryState) string {
	if rs == nil || len(rs.ActiveViolations) == 0 {
		return ""
	}
	var rows []structuredFixRow
	for _, sv := range rs.ActiveViolations {
		row, ok := classifyViolationToStructuredFix(sv)
		if !ok {
			continue
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("## Mechanical Fix Actions\n\n")
	out.WriteString("Apply each action below mechanically; each entry names a single `emit_answer_document` field path to set OR a single block to add. The Hint line is informational; it does not need interpretation.\n\n")
	for i, r := range rows {
		fmt.Fprintf(&out, "%d. Action: `%s`\n", i+1, r.Action)
		fmt.Fprintf(&out, "   - Target: `%s`\n", r.Target)
		for k, v := range r.Args {
			fmt.Fprintf(&out, "   - %s: `%s`\n", k, v)
		}
		if r.Hint != "" {
			fmt.Fprintf(&out, "   - Hint: %s\n", r.Hint)
		}
		out.WriteString("\n")
	}
	return out.String()
}

// structuredFixRow is the typed surface a P3 row renders. Action
// is one of a small enum of mechanical operations the LLM can
// recognize; Target is a schema field path or `blocks[]`; Args is
// an ordered key→value map of operation-specific parameters; Hint
// is an optional one-line user-vocab "why" string.
type structuredFixRow struct {
	Action string
	Target string
	Args   map[string]string
	Hint   string
}

// classifyViolationToStructuredFix returns true with a typed row
// when the violation's Kind matches one of the four covered
// validator-driven cluster shapes. Unknown / subjective kinds
// return ok=false and the caller falls through to the prose
// renderer.
func classifyViolationToStructuredFix(sv types.ScoredViolation) (structuredFixRow, bool) {
	switch sv.Kind {
	case types.ViolBlockCoverageMissing:
		// Detail typically reads "required block kind=X appears
		// N times; the family contract requires at least M".
		// Extract the kind from the Detail tail (best-effort) so
		// the row's structured kind matches.
		row := structuredFixRow{
			Action: "add_block",
			Target: "blocks[]",
			Args: map[string]string{
				"kind": extractMissingBlockKind(sv),
			},
			Hint: strings.TrimSpace(sv.Repair),
		}
		return row, true
	case types.ViolPrincipalClaimUseMissing:
		row := structuredFixRow{
			Action: "set_field",
			Target: principalClaimUseTarget(sv),
			Args: map[string]string{
				"value": "[{claim_form: <one of the contract's acceptable forms>}]",
			},
			Hint: strings.TrimSpace(sv.Repair),
		}
		return row, true
	case types.ViolFacetUncovered:
		row := structuredFixRow{
			Action: "set_field",
			Target: "blocks[].facet_ids OR blocks[].claim_uses[].facet_id",
			Args: map[string]string{
				"value": extractFacetIDFromDetail(sv),
			},
			Hint: strings.TrimSpace(sv.Repair),
		}
		return row, true
	case types.ViolUncertaintyBlockMissing:
		row := structuredFixRow{
			Action: "add_caveat_block",
			Target: "blocks[]",
			Args: map[string]string{
				"kind": "caveat",
				"text": "(disclose what was searched and what remained uncertain)",
			},
			Hint: strings.TrimSpace(sv.Repair),
		}
		return row, true
	}
	return structuredFixRow{}, false
}

// extractMissingBlockKind reads the typed FieldPath when present
// (carries `blocks[].kind=X`), otherwise scans Detail for the
// kind=<token> hint the post-emit validator embeds.
func extractMissingBlockKind(sv types.ScoredViolation) string {
	if sv.FieldPath != "" {
		// FieldPath looks like "blocks[].kind=X"
		if idx := strings.Index(sv.FieldPath, "kind="); idx >= 0 {
			return strings.TrimSpace(sv.FieldPath[idx+len("kind="):])
		}
	}
	// Fall back to Detail scan: look for "kind=X" first occurrence.
	if idx := strings.Index(sv.Detail, "kind="); idx >= 0 {
		tail := sv.Detail[idx+len("kind="):]
		end := strings.IndexAny(tail, " ;,)\n")
		if end < 0 {
			end = len(tail)
		}
		return strings.TrimSpace(tail[:end])
	}
	return "<missing block kind from validator detail>"
}

// principalClaimUseTarget builds the FieldPath for a
// principal-claim-use-missing violation. Falls back to a generic
// path when BlockID is empty.
func principalClaimUseTarget(sv types.ScoredViolation) string {
	if sv.BlockID == "" {
		return "blocks[].claim_uses"
	}
	return fmt.Sprintf("blocks[id=%q].claim_uses", sv.BlockID)
}

// extractFacetIDFromDetail reads the typed FieldPath when present
// (validator emits "facet:<kind>"), otherwise scans Detail for the
// quoted facet kind. Returns the kind or a placeholder.
func extractFacetIDFromDetail(sv types.ScoredViolation) string {
	// Detail typically contains: required facet "X" (essential) is not covered: ...
	if idx := strings.Index(sv.Detail, "facet "); idx >= 0 {
		tail := sv.Detail[idx+len("facet "):]
		// Look for the quoted name following "facet ".
		if start := strings.IndexByte(tail, '"'); start >= 0 {
			rest := tail[start+1:]
			if end := strings.IndexByte(rest, '"'); end > 0 {
				return strings.TrimSpace(rest[:end])
			}
		}
	}
	return "<missing facet kind from validator detail>"
}

// renderLiveCoverageStatus is the P2 surface — computes per-block-kind
// counts and per-facet declaration depth from the prior emit and
// surfaces them in user-vocab prose so the LLM sees the gaps BEFORE
// re-emitting. Fires on retry path only (caller already gated on
// rs.HasContent()).
//
// User-vocab translation table (R6):
//   - "DeclaredCount" → "declared on N blocks"
//   - "AnchoredCount" → "N of them grounded with evidence"
//   - "BlockRequirement.MinCount" → "required: at least N"
//   - "BlockRequirement.MaxCount" → "max: N"
//   - facet kind → AnswerFacetPublicLabel (already user-vocab)
//
// Empty when no view (single-shot test path) OR PrevEmitJSON
// undecodable; the section is opportunistic enrichment, never
// load-bearing.
//
// Red-line audit (R3/R4/R5/R6/R7/SST/CN+EN-only):
//   - R3 typed: every count comes from the typed BlockRequirement /
//     FacetRequirement / decoded prior emit doc
//   - R4 generic: templated; no project / case names
//   - R5 informational: section is read-only over the prior emit;
//     never writes answer body
//   - R6 no internal vocab: "Block kind balance" / "Aspect
//     coverage" / "grounded with evidence" are user-vocab
//     paraphrases of the internal AnchoredCount / DeclaredCount /
//     BlockRequirement metrics
//   - R7 actionable: each gap line tells the LLM exactly what to
//     change on retry; passing items are marked so the LLM
//     doesn't accidentally regress them
//   - SST: matches the surrounding renderAnswerDocRetryState
//     section structure (same `##` heading depth, same numbering
//     model)
//   - CN+EN-only: prompt is English (consistent with the
//     surrounding finalizer skill prose)
func renderLiveCoverageStatus(ctx *types.AgentContext, rs *types.RetryState) string {
	if rs == nil || len(rs.PrevEmitJSON) == 0 {
		return ""
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil {
		return ""
	}
	var prevDoc types.AnswerDocumentV2
	if err := json.Unmarshal(rs.PrevEmitJSON, &prevDoc); err != nil {
		return ""
	}

	var blockLines []string
	if line := liveBlockKindBalance(&prevDoc, view); line != "" {
		blockLines = append(blockLines, line)
	}
	var facetLines []string
	if line := liveAspectCoverage(&prevDoc, view); line != "" {
		facetLines = append(facetLines, line)
	}
	if len(blockLines) == 0 && len(facetLines) == 0 {
		return ""
	}

	var out strings.Builder
	out.WriteString("## Coverage Gaps in Previous Emit (informational)\n\n")
	out.WriteString("Your previous emit had the following structural-coverage status. Items marked ⚠ are gaps the validator will reject again unless fixed; items marked ✓ are clean (preserve them as-is on retry).\n\n")
	for _, l := range blockLines {
		out.WriteString(l)
	}
	for _, l := range facetLines {
		out.WriteString(l)
	}
	return out.String()
}

// liveBlockKindBalance renders the per-required-block kind status
// line(s) — one bullet per RequiredBlock comparing prior emit
// count vs MinCount / MaxCount.
func liveBlockKindBalance(prev *types.AnswerDocumentV2, view *types.AnswerSemanticView) string {
	if prev == nil || view == nil || len(view.RequiredBlocks) == 0 {
		return ""
	}
	counts := make(map[types.AnswerBlockKind]int, len(prev.Blocks))
	for _, b := range prev.Blocks {
		counts[b.Kind]++
	}
	var b strings.Builder
	b.WriteString("**Block kind balance:**\n\n")
	any := false
	for _, req := range view.RequiredBlocks {
		if !req.Required {
			continue
		}
		got := counts[req.Kind]
		marker := "✓"
		var detail string
		if got < req.MinCount {
			marker = "⚠"
			detail = fmt.Sprintf("required at least %d", req.MinCount)
		} else if req.MaxCount > 0 && got > req.MaxCount {
			marker = "⚠"
			detail = fmt.Sprintf("over max %d", req.MaxCount)
		} else if req.MaxCount > 0 && req.MaxCount == req.MinCount {
			detail = fmt.Sprintf("required exactly %d", req.MinCount)
		} else {
			detail = fmt.Sprintf("required at least %d", req.MinCount)
		}
		fmt.Fprintf(&b, "  %s `%s` — emitted %d (%s)\n",
			marker, string(req.Kind), got, detail)
		any = true
	}
	if !any {
		return ""
	}
	b.WriteString("\n")
	return b.String()
}

// liveAspectCoverage renders the per-facet declaration depth line(s).
// Mirrors the post-emit countFacetCoverageDepth metric but renders
// in user-vocab prose. AnchoredCount = blocks whose payload also
// carries citation_ref >= 0 OR a claim_uses[] entry with non-empty
// EvidenceID/ClaimForm.
func liveAspectCoverage(prev *types.AnswerDocumentV2, view *types.AnswerSemanticView) string {
	if prev == nil || view == nil || view.FacetCoverage == nil {
		return ""
	}
	if len(view.FacetCoverage.Required) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("**Aspect coverage:**\n\n")
	any := false
	for _, req := range view.FacetCoverage.Required {
		if req.EffectivePromotionPolicy() == types.PromotionAdvisoryOnly {
			continue
		}
		if !req.IsPromoted() {
			continue
		}
		kind := strings.TrimSpace(string(req.Kind))
		if kind == "" {
			continue
		}
		declared, anchored := liveCountFacetDepth(prev, kind)
		label := types.AnswerFacetPublicLabel(req.Kind)
		marker := "✓"
		var detail string
		switch {
		case declared == 0:
			marker = "⚠"
			detail = "not declared on any block — set block.facet_ids OR claim_uses[].facet_id"
		case anchored == 0:
			marker = "⚠"
			detail = fmt.Sprintf("declared on %d block(s) but %d grounded with evidence — attach citation_ref to one item, or add a claim_uses[] entry with claim_form/evidence_id",
				declared, anchored)
		case anchored < declared:
			marker = "⚠"
			detail = fmt.Sprintf("declared on %d block(s); %d grounded with evidence (the rest are label-only)",
				declared, anchored)
		default:
			detail = fmt.Sprintf("declared on %d block(s), all grounded with evidence",
				declared)
		}
		fmt.Fprintf(&b, "  %s %q — %s\n", marker, label, detail)
		any = true
	}
	if !any {
		return ""
	}
	b.WriteString("\n")
	return b.String()
}

// liveCountFacetDepth re-implements the post-emit countFacetCoverageDepth
// helper inline (the orchestrator-package original lives behind an
// agent-package import boundary). Only the load-bearing variant —
// block-level FacetIDs declarations + claim_uses[].FacetID — is
// rendered; the post-emit chain still runs the deeper
// blockHasAnchoredClaim / blockHasAnchoredListSurface checks.
func liveCountFacetDepth(doc *types.AnswerDocumentV2, kind string) (declared, anchored int) {
	if doc == nil || kind == "" {
		return 0, 0
	}
	for _, b := range doc.Blocks {
		blockDeclares := false
		for _, fid := range b.FacetIDs {
			if strings.TrimSpace(fid) == kind {
				blockDeclares = true
				declared++
				break
			}
		}
		if blockDeclares {
			// Block-level declaration is "anchored" when ANY
			// claim_uses[] entry on this block names a ClaimForm
			// or EvidenceID OR any item carries a non-negative
			// citation_ref (matches the post-emit
			// blockHasAnchoredListSurface lite path).
			anchoredHere := false
			for _, cu := range b.ClaimUses {
				if cu.ClaimForm != "" || cu.EvidenceID != "" {
					anchoredHere = true
					break
				}
			}
			if !anchoredHere {
				for _, item := range b.Items {
					if item.CitationRef >= 0 {
						anchoredHere = true
						break
					}
				}
			}
			if anchoredHere {
				anchored++
			}
		}
		// claim_uses[].facet_id channel — same kind on a claim_use
		// declares the facet; the per-claim_use's ClaimForm /
		// EvidenceID counts as anchored.
		for _, cu := range b.ClaimUses {
			if strings.TrimSpace(cu.FacetID) != kind {
				continue
			}
			declared++
			if cu.ClaimForm != "" || cu.EvidenceID != "" {
				anchored++
			}
		}
	}
	return declared, anchored
}

// renderRetryRequiredChanges renders the typed FieldPath + Repair
// list for every violation with Severity >= Medium. Soft violations
// are listed in the Active Violations section but NOT in Required
// Changes (LLM is informed they exist but not required to fix).
//
// Sort order: Critical first, then High, then Medium. Within a
// severity bucket, preserve producer order (deterministic).
func renderRetryRequiredChanges(rs *types.RetryState) string {
	if rs == nil || len(rs.ActiveViolations) == 0 {
		return ""
	}
	type bucket struct {
		severity types.Severity
		entries  []types.ScoredViolation
	}
	buckets := []*bucket{
		{severity: types.SeverityCritical},
		{severity: types.SeverityHigh},
		{severity: types.SeverityMedium},
	}
	for _, sv := range rs.ActiveViolations {
		for _, b := range buckets {
			if sv.Severity == b.severity {
				b.entries = append(b.entries, sv)
				break
			}
		}
	}
	totalActionable := 0
	for _, b := range buckets {
		totalActionable += len(b.entries)
	}
	if totalActionable == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("## Required Changes\n\n")
	out.WriteString("Apply each change below. Each entry names the typed FIELD PATH the LLM must edit (relative to your `emit_answer_document` params) and the validator's repair guidance.\n\n")
	for _, b := range buckets {
		if len(b.entries) == 0 {
			continue
		}
		fmt.Fprintf(&out, "**%s** (must fix):\n\n", strings.ToUpper(string(b.severity)))
		for _, sv := range b.entries {
			out.WriteString("- ")
			if sv.FieldPath != "" {
				fmt.Fprintf(&out, "field path `%s`", sv.FieldPath)
			} else {
				fmt.Fprintf(&out, "kind=%s", sv.Kind)
			}
			if sv.BlockID != "" && !strings.Contains(sv.FieldPath, sv.BlockID) {
				fmt.Fprintf(&out, " (block id=%q)", sv.BlockID)
			}
			repair := strings.TrimSpace(sv.Repair)
			if repair == "" {
				repair = strings.TrimSpace(sv.Detail)
			}
			if repair != "" {
				fmt.Fprintf(&out, " — %s", repair)
			}
			out.WriteString("\n")
		}
		out.WriteString("\n")
	}
	return out.String()
}

// renderRetryActiveViolations renders the full violation set
// grouped by Severity+Layer. Includes Soft violations (telemetry
// only — LLM is informed but not required to fix). Cross-layer
// rendering ensures LLM sees scheduler / V2 oracle / contract.Check
// violations together (R13).
func renderRetryActiveViolations(rs *types.RetryState) string {
	if rs == nil || len(rs.ActiveViolations) == 0 {
		return ""
	}
	// Group: severity → layer → []entries
	bySeverity := map[types.Severity]map[string][]types.ScoredViolation{}
	for _, sv := range rs.ActiveViolations {
		layers := bySeverity[sv.Severity]
		if layers == nil {
			layers = map[string][]types.ScoredViolation{}
			bySeverity[sv.Severity] = layers
		}
		layers[sv.Layer] = append(layers[sv.Layer], sv)
	}
	var out strings.Builder
	out.WriteString("## Active Violations (typed, by severity + layer)\n\n")
	severityOrder := []types.Severity{
		types.SeverityCritical, types.SeverityHigh,
		types.SeverityMedium, types.SeveritySoft,
	}
	for _, sev := range severityOrder {
		layers := bySeverity[sev]
		if len(layers) == 0 {
			continue
		}
		fmt.Fprintf(&out, "**%s**:\n\n", strings.ToUpper(string(sev)))
		// Stable iteration: sort layers by name (deterministic prompt).
		layerNames := make([]string, 0, len(layers))
		for n := range layers {
			layerNames = append(layerNames, n)
		}
		// Simple sort without import; len is small.
		for i := 0; i < len(layerNames); i++ {
			for j := i + 1; j < len(layerNames); j++ {
				if layerNames[j] < layerNames[i] {
					layerNames[i], layerNames[j] = layerNames[j], layerNames[i]
				}
			}
		}
		for _, layer := range layerNames {
			for _, sv := range layers[layer] {
				fmt.Fprintf(&out, "- [%s · %s] %s", layer, sv.Kind, strings.TrimSpace(sv.Detail))
				if sv.Severity == types.SeveritySoft {
					out.WriteString(" *(telemetry only)*")
				}
				out.WriteString("\n")
			}
		}
		out.WriteString("\n")
	}
	return out.String()
}

// renderRetryPrevEmit renders the typed projection of the previous
// emit so the LLM sees what fields it already filled (R6/R6.1 fix).
//
// For each block, lists:
//   - id + kind + surface_role
//   - facet_ids verbatim (if non-empty)
//   - block-level claim_use presence + claim_form (if any)
//   - item count + items_with_citation (if any)
//   - text preview (head 400 + tail 200 truncation)
func renderRetryPrevEmit(rs *types.RetryState) string {
	if rs == nil || len(rs.PrevEmitSummary.BlockSummaries) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("## Previous Emit (preserve every field below byte-identical unless listed in Required Changes)\n\n")
	for _, bs := range rs.PrevEmitSummary.BlockSummaries {
		fmt.Fprintf(&out, "Block id=%q kind=%s", bs.ID, bs.Kind)
		if bs.SurfaceRole != "" {
			fmt.Fprintf(&out, " surface_role=%q", string(bs.SurfaceRole))
		}
		out.WriteString(":\n")
		if len(bs.FacetIDs) > 0 {
			fmt.Fprintf(&out, "  facet_ids: %s\n", renderQuotedList(bs.FacetIDs))
		}
		if bs.HasClaimUse {
			fmt.Fprintf(&out, "  claim_use: present (claim_form=%q)\n", string(bs.ClaimForm))
		} else {
			out.WriteString("  claim_use: ABSENT (verify whether contract requires it)\n")
		}
		if bs.HasItems {
			fmt.Fprintf(&out, "  items: %d total, %d with citation\n",
				bs.ItemCount, bs.ItemsWithCitation)
		}
		if bs.EdgeAnchoredClaimUses > 0 {
			fmt.Fprintf(&out, "  edge_anchors: %d entries (block-level edge_anchors[] array — preserve these on retry)\n",
				bs.EdgeAnchoredClaimUses)
		}
		if bs.TextPreview != "" {
			// Indent preview for readability.
			preview := strings.ReplaceAll(strings.TrimSpace(bs.TextPreview), "\n", " ")
			if len([]rune(preview)) > 200 {
				preview = string([]rune(preview)[:200]) + "…"
			}
			fmt.Fprintf(&out, "  text preview: %q\n", preview)
		}
		out.WriteString("\n")
	}
	if rs.PrevEmitSummary.CitationsCount > 0 {
		fmt.Fprintf(&out, "Citations: %d entries", rs.PrevEmitSummary.CitationsCount)
		if len(rs.PrevEmitSummary.CitationFiles) > 0 {
			fmt.Fprintf(&out, " (top files: %s)", strings.Join(rs.PrevEmitSummary.CitationFiles, ", "))
		}
		out.WriteString("\n\n")
	}
	if rs.PrevEmitSummary.HasExactResolution {
		out.WriteString("exact_resolution: present (preserve byte-identical)\n\n")
	}
	return out.String()
}

// ## Facets each block must cover — which semantic surface each
//
//	block should ground (cross-ref to
//	the existing Required Answer Facets
//	section above).
func renderAnswerDocBlockContract(ctx *types.AgentContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Required Answer Blocks\n\n")
	b.WriteString("Your answer should be composed from one or more BLOCKS. Each block has a kind " +
		"(summary / section / ordered_list / bullet_list / scalar / decision / table / diagram / caveat) " +
		"and counts toward the contract for this question's family. " +
		"Required blocks marked below MUST appear; optional blocks add richness when their facet matches your evidence.\n\n")
	if len(view.RequiredBlocks) > 0 {
		b.WriteString("**Required (must emit):**\n\n")
		for _, req := range view.RequiredBlocks {
			renderAnswerDocBlockRequirement(&b, req, true)
		}
		b.WriteString("\n")
	}
	if len(view.OptionalBlocks) > 0 {
		b.WriteString("**Optional (recommended when evidence supports):**\n\n")
		for _, req := range view.OptionalBlocks {
			renderAnswerDocBlockRequirement(&b, req, false)
		}
		b.WriteString("\n")
	}
	if view.DiagramPlan != nil && view.DiagramPlan.Required {
		fmt.Fprintf(&b, "**Diagram contract:** required (kind=%s); the diagram block must cover the family's structural relationships.\n\n",
			view.DiagramPlan.Kind)
	}
	if len(view.UncertaintyRules) > 0 {
		b.WriteString("**Uncertainty disclosures:** when the listed conditions hold, include a caveat block disclosing the boundary so the user knows what was searched and what was assumed.\n\n")
	}
	return b.String()
}

func renderAnswerDocBlockRequirement(b *strings.Builder, req types.BlockRequirement, _ bool) {
	countTag := ""
	switch {
	case req.MinCount == 0 && req.MaxCount == 1:
		countTag = "0..1"
	case req.MinCount == 1 && req.MaxCount == 1:
		countTag = "exactly 1"
	case req.MinCount > 0 && req.MaxCount == 0:
		countTag = fmt.Sprintf("%d+", req.MinCount)
	case req.MinCount == 0 && req.MaxCount == 0:
		countTag = "any number"
	default:
		countTag = fmt.Sprintf("%d-%d", req.MinCount, req.MaxCount)
	}
	fmt.Fprintf(b, "- **%s** (%s)", req.Kind, countTag)
	rationale := strings.TrimSpace(req.Rationale)
	if rationale != "" {
		fmt.Fprintf(b, " — %s", rationale)
	}
	b.WriteString("\n")

	// R7 (post_shape_residual_audit.md, 2026-05-04): typed-set
	// fields verbatim — LLM was filling block.facet_ids /
	// block.claim_uses[].claim_form / block.surface_role using GUESS-WORK
	// because the prose only listed field NAMES, not the typed string
	// VALUES. Now we expose the values verbatim under each block
	// requirement so the LLM can copy them directly into the emit.
	if len(req.FacetIDs) > 0 {
		fmt.Fprintf(b, "  - `block.facet_ids` MUST include: %s (copy verbatim into the emit).\n",
			renderQuotedList(req.FacetIDs))
	}
	if len(req.AcceptableClaimForms) > 0 {
		forms := make([]string, 0, len(req.AcceptableClaimForms))
		for _, f := range req.AcceptableClaimForms {
			forms = append(forms, string(f))
		}
		fmt.Fprintf(b, "  - At least one `block.claim_uses[]` entry's `claim_form` MUST be one of: %s (copy verbatim).\n",
			renderQuotedList(forms))
	}
	if req.SurfaceRoleHint != "" {
		fmt.Fprintf(b, "  - `block.surface_role` SHOULD be %q (copy verbatim).\n",
			string(req.SurfaceRoleHint))
	}
}

// renderQuotedList formats a slice of typed string values as a
// comma-separated list of double-quoted tokens so the LLM sees
// JSON-ready string values it can copy directly into the emit:
//
//	["enumeration_item", "uncertainty_boundary"]
//
// R7: LLM-facing typed-set field rendering — the verbatim form
// removes the "guess what string to put here" failure mode.
func renderQuotedList(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, fmt.Sprintf("%q", v))
	}
	return "[" + strings.Join(out, ", ") + "]"
}

func renderAnswerDocFacetCoverage(ctx *types.AgentContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.FacetCoverage == nil {
		return ""
	}
	fc := plan.FacetCoverage
	if len(fc.Required) == 0 && len(fc.Optional) == 0 {
		return ""
	}

	// R7 (post_shape_residual_audit.md, 2026-05-04): build a reverse
	// map of facet kind → block kind(s) so each facet line can name
	// the block where it should live. Reads view.RequiredBlocks /
	// OptionalBlocks (already in the prompt above) without
	// duplicating typed values.
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	facetToBlocks := map[types.AnswerFacetKind][]types.AnswerBlockKind{}
	if view != nil {
		collect := func(blocks []types.BlockRequirement) {
			for _, blk := range blocks {
				for _, fid := range blk.FacetIDs {
					k := types.AnswerFacetKind(fid)
					facetToBlocks[k] = append(facetToBlocks[k], blk.Kind)
				}
			}
		}
		collect(view.RequiredBlocks)
		collect(view.OptionalBlocks)
	}
	formatBlocks := func(kinds []types.AnswerBlockKind) string {
		if len(kinds) == 0 {
			return ""
		}
		seen := map[types.AnswerBlockKind]bool{}
		out := make([]string, 0, len(kinds))
		for _, k := range kinds {
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, fmt.Sprintf("`%s`", string(k)))
		}
		return strings.Join(out, ", ")
	}

	var b strings.Builder
	b.WriteString("## Required Answer Facets\n\n")
	b.WriteString("The user's question imposes the following coverage requirements on the rendered answer. " +
		"Each facet names a semantic surface the answer must touch; the parenthesised hint names the evidence shape that supports it. " +
		"Cover every HARD facet that has grounded evidence; SOFT facets are recommended but not strictly required; OPTIONAL facets add richness when they fit.\n\n")
	b.WriteString("To declare a facet on a block, set `block.facet_ids` (or `block.claim_uses[j].facet_id`) to the verbatim string shown after `facet_id:` on each line below — that exact string MUST appear in the emit. The rightmost hint (when present) names the block kind(s) that typically cover the facet so you know where to place it.\n\n")
	b.WriteString("Each line ends with `(evidence: N)` — N is the count of typed evidence items already bound to the facet. " +
		"For SOFT facets this count gates the coverage demand: when N=0 the facet is skipped (no typed evidence to surface), so do NOT invent claims for it. " +
		"For HARD facets, coverage is demanded regardless of N (the question pinned the facet as essential). " +
		"A SOFT facet with N≥1 is **elevated** — its line carries an `(elevated)` marker AFTER the evidence count, and you must cover it as if it were HARD; the typed evidence is available, so the answer must surface it.\n\n")

	emit := func(req types.FacetRequirement) {
		label := types.AnswerFacetPublicLabel(req.Kind)
		var tag string
		switch req.Required {
		case types.FacetHardRequired:
			tag = "HARD"
		case types.FacetSoftRequired:
			tag = "SOFT"
		default:
			tag = "OPTIONAL"
		}
		fmt.Fprintf(&b, "- **%s**: %s.", tag, label)
		// R7: verbatim facet_id string the LLM must copy.
		fmt.Fprintf(&b, " `facet_id: %q`.", string(req.Kind))
		// Phase 5-E2: typed evidence count surfaces the gate input.
		fmt.Fprintf(&b, " (evidence: %d)", len(req.SourceCandidate))
		// G6-3 (post_v2_runtime_gap_remediation, 2026-05-04): SOFT
		// facets with typed evidence available are elevated to HARD
		// gating. Surface the typed signal so the LLM knows this
		// SOFT facet currently behaves as HARD.
		if req.Required == types.FacetSoftRequired && req.IsPromoted() {
			b.WriteString(" (elevated)")
		}
		if len(req.AcceptableForms) > 0 {
			forms := make([]string, 0, len(req.AcceptableForms))
			seen := map[string]bool{}
			for _, f := range req.AcceptableForms {
				name, ok := answerDocClaimFormLabels[f]
				if !ok || seen[name] {
					continue
				}
				seen[name] = true
				forms = append(forms, name)
			}
			if len(forms) > 0 {
				fmt.Fprintf(&b, " Acceptable evidence: %s.", strings.Join(forms, ", "))
			}
		}
		// R7: reverse lookup — which block kind(s) cover this facet.
		if owners, ok := facetToBlocks[req.Kind]; ok && len(owners) > 0 {
			fmt.Fprintf(&b, " Place on block kind: %s.", formatBlocks(owners))
		}
		b.WriteString("\n")
	}

	if len(fc.Required) > 0 {
		for _, req := range fc.Required {
			emit(req)
		}
	}
	if len(fc.Optional) > 0 {
		b.WriteString("\nOptional richness facets:\n")
		for _, req := range fc.Optional {
			emit(req)
		}
	}
	b.WriteString("\n")
	return b.String()
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
	b.WriteString("- This requirement is independent of question family: if it says `Required: yes`, the dispatch must contain at least one grounded fenced diagram via a principal `diagram` block (preferred) or a fenced block embedded in the `summary` block's text.\n")
	b.WriteString("- PREFERRED form: a ` ```mermaid ` fenced block. For flow / architecture / call_dag families, default to `flowchart TD` and switch to LR / RL / BT only when that genuinely improves readability; vertical is preferred because code-bearing labels are often long. For `sequenceDiagram`, keep participant labels short and move detailed prose into messages / surrounding text because actors render horizontally. The renderer applies a deterministic-alignment pass to Mermaid bodies so the diagram looks clean across terminals / locales / CJK content. ASCII art (a bare ``` fence with `+ - | > <` connectors) is the FALLBACK ONLY when the supported Mermaid subset cannot express the shape.\n")
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
			// Reframed alongside the First-Pass reference: these
			// sections are GROUNDED FLOORS (everything listed is
			// already cited) the model can extend, not ceilings the
			// model must paste verbatim. The hard constraint stays
			// at the label-grounding level — file/path nodes inside
			// the fenced block must come from citations[] or Log
			// Triage frames — but the structure (number of nodes,
			// direction, branching) is the model's call.
			b.WriteString("These are grounded reference structures the diagram MAY draw from. Each section lists evidence-grounded nodes / labels you can use; you are encouraged to extend them with additional grounded nodes, change the direction, or introduce branching / fan-out where the mechanism warrants. The only hard rule: file or path nodes inside the fenced block must come from citations[] or the Log Triage frames.\n\n")
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
	b.WriteString("## First-Pass Diagram Reference\n\n")
	// Pre-2026-04-30 this section said "copy this fenced skeleton
	// verbatim ... you may delete unused nodes, but do not rename
	// the remaining ones." Combined with "explain any extra
	// semantics in prose around it", it forced any richer-than-
	// skeleton mechanism into prose and capped the diagram at the
	// seed shape. The reframing below presents the skeleton as a
	// GROUNDED FLOOR (every node here is already evidence-grounded)
	// rather than a CEILING: the model is explicitly authorised to
	// add nodes, change `flowchart TD` to LR / RL / BT when needed, introduce branches /
	// fan-out, or switch to `sequenceDiagram` — the only hard rule
	// being that every label remains grounded in citations[] or
	// Log Triage frames.
	b.WriteString("Below is a grounded starting reference — every node listed here is already corroborated by evidence in this dispatch, and some scenarios may supply prevalidated role labels whose supporting anchors are listed elsewhere in the prompt. Use it as a FLOOR, not a ceiling:\n\n")
	b.WriteString("- You SHOULD extend it with additional grounded nodes when the investigation supports a richer mechanism (e.g. more call-chain hops, branch points, fan-out, error paths).\n")
	b.WriteString("- You MAY change the direction (`flowchart TD` ↔ `LR` / `RL` / `BT`), introduce subgraph-like clusters as flat nodes, or switch to `sequenceDiagram` if actor-to-actor better fits the mechanism — none of these are forbidden.\n")
	b.WriteString("- You MAY add branches / fan-out / multi-source / multi-sink shapes; the linear chain in the reference is just the safest default skeleton, not a structural requirement.\n")
	b.WriteString("- HARD RULE: every node label must remain grounded in citations[] or Log Triage frames. Renaming an existing node, abstracting it, or inventing a node without grounded backing is rejected by the diagram-grounding gate.\n\n")
	b.WriteString(fence)
	b.WriteString("\n\n")
	return b.String()
}

func renderAnswerDocDiagramLabelSeed() string {
	return strings.TrimSpace(
		"Label each node with grounded names you already have: cited repo files, cited symbols, log-frame functions, or path literals that appear in cited line text.\n" +
			"- If the evidence names one spelling, keep that spelling in the diagram instead of renaming it to a nearby alias.\n" +
			"- If you need an alternate label, only use it when that exact label appears in citations or log frames.\n" +
			"- Do not synthesize bare line-number aliases such as `L877`, `Line 42`, or similar citation-derived shorthand unless the cited text itself contains that exact token.\n" +
			"- Supporting anchor locations belong OUTSIDE the fence. Do not append `(<file>:<line>)`, `<br/>(<file>)`, or similar support-location suffixes to a node label unless that exact combined label is itself grounded.\n" +
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
		appendLabel(anchor.Source)
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
	// Use the canonical Mermaid renderer so the seed shape agrees
	// with the skill's Mermaid-preferred contract. Pre-2026-04-30
	// this renderer hand-built a bare ``` fence with `innermost
	// failure: …` / `  -> caller:` ASCII prose, which trained the
	// model to copy ASCII verbatim despite the prompt-level Mermaid
	// preference.
	bundleSubset := &types.LogBundle{
		Errors: []types.LogError{{Frames: resolved}},
	}
	fence := types.RenderLogDiagramFence(bundleSubset)
	if fence == "" {
		return ""
	}
	return "When you draw a call-chain / sequence diagram, start from these resolved frames (extend with additional grounded callers if your investigation supports a richer chain):\n\n" + fence
}

func renderRequiredLogTriageTypes(bundle *types.LogBundle) string {
	typeNames := types.LogBundleErrorTypes(bundle)
	if len(typeNames) == 0 {
		return ""
	}
	return "For this dispatch, the exact structured log error type(s) you must mention literally in `summary` are: `" + strings.Join(typeNames, "`, `") + "`."
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
	// Render the precedence chain as a Mermaid flowchart via the
	// canonical renderer so the seed shape agrees with the skill's
	// Mermaid-preferred contract. Pre-2026-04-30 this hand-built a
	// bare ``` fence with `node\n  ->\n` ASCII shape that taught
	// the model to copy ASCII whenever a config-trace question fired.
	b.WriteString("For config-precedence questions, prefer the validated role-labeled precedence chain below. The chain is ordered from highest precedence at the top to lowest precedence at the bottom. Each node label is a compiled role abstraction backed by grounded evidence in this dispatch; the supporting file:line anchors are listed immediately after the fence. Treat it as a grounded FLOOR — keep any role label you retain verbatim, but you MAY add additional grounded precedence layers when your investigation supports a richer chain (still subject to the citation-grade rules below).\n\n")
	b.WriteString(types.RenderConfigTraceDiagramFence(anchors))
	b.WriteByte('\n')
	for _, anchor := range anchors {
		support := types.ConfigTraceDiagramAnchorSupportLabel(anchor)
		if support == "" {
			continue
		}
		fmt.Fprintf(&b, "- `%s` [%s] → `%s`\n", anchor.Label, anchor.Role, support)
	}
	b.WriteByte('\n')
	b.WriteString("The support locations listed above stay outside the fence. Keep the node labels themselves bare role labels / compound role labels / grounded file-path labels; do not append `:line`, `(<file>:<line>)`, or `<br/>` support text to the label body unless that exact combined label is itself grounded.\n")
	b.WriteString("Precedence semantics live in prose, not in invented node names: `override` = highest-precedence operator-supplied layer (CLI flag / env override / SDK setter / RPC override — all the same tier), `config` = grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.), `default` = code-default fallback. `runtime` is the binding / merge code path between those layers, not a standalone user-config tier.\n")
	if missing := missingConfigTraceDiagramRoles(rolesPresent); len(missing) > 0 {
		fmt.Fprintf(&b, "Current grounded evidence does NOT include anchor(s) for these precedence role(s): %s. Do not add fenced-diagram nodes for missing roles unless you first cite a real repo anchor for them; if you need to explain those semantics, keep them in prose as general precedence rules rather than grounded nodes in this dispatch.\n", strings.Join(missing, ", "))
	}
	// Two-channel structural rule. The downstream validator
	// (validateSummaryConfigTraceFenceLabels) accepts ANY label that
	// fits one of the channels below, regardless of the specific
	// word the LLM picks. Examples like `CLI` / `RPC` / `UI` are
	// teaching aids — the validator does NOT keyword-match on them;
	// it runs the structural channel checks. So the rule
	// generalises to any vocabulary the user happens to reach for.
	fmt.Fprintf(&b, "Every node label inside this fenced diagram must satisfy ONE of three structural grounding channels (the validator runs them in order and accepts the first match):\n")
	fmt.Fprintf(&b, "  1. Use a role label EXACTLY as listed in the supporting-anchor block above: `%s`, `%s`, `%s`, or `%s`. The role marker is the binding.\n",
		types.ConfigTraceDiagramRoleNodeLabel(types.EvidenceDiagramRoleOverride),
		types.ConfigTraceDiagramRoleNodeLabel(types.EvidenceDiagramRoleConfig),
		types.ConfigTraceDiagramRoleNodeLabel(types.EvidenceDiagramRoleRuntime),
		types.ConfigTraceDiagramRoleNodeLabel(types.EvidenceDiagramRoleDefault))
	fmt.Fprintf(&b, "  2. Write a compound `<content> (<role-marker>)` label whose parenthetical is one of `override` / `config` / `runtime` / `default` (or their long forms). The role-marker carries the binding; the content phrase is the human-readable surface.\n")
	fmt.Fprintf(&b, "  3. Use a concrete file path or symbol that appears in `citations[]`.\n")
	fmt.Fprintf(&b, "Any label that fits NONE of the three channels (a one-word concept like `CLI` / `RPC` / `UI` without a role marker, a numbered band such as `band 1`, an architectural archetype) is rejected. Drop those nodes from the fence and explain the concept in prose instead — do not invent a new bucket name.\n")
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
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	stableAbsent := ctx.Mutable != nil && strings.EqualFold(strings.TrimSpace(ctx.Mutable.InvestigationResultKind()), "absence")
	anchors := types.CollectConfigTraceDiagramAnchors(
		ctx.AnalysisIR.RequestModel.Scenario,
		stableAbsent,
		ctx.AnalysisIR.AnswerContract.ExactResolution,
		answerDocExactContextRequiredFiles(ctx),
		ctx.EvidenceItems,
	)
	out := make([]configTraceDiagramAnchor, 0, len(anchors))
	for _, anchor := range anchors {
		out = append(out, configTraceDiagramAnchor(anchor))
	}
	return out
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
			b.WriteString("- Because the exact config key is absent, do NOT emit a principal scalar block with a synthetic literal such as `(missing)` / `(不存在)`. Prefer a summary-led explanation so the answer can lead with the exact absence before any nearby grounded context.\n")
		}
		if ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.Scenario == types.ScenarioConfigTrace {
			b.WriteString("- Because the exact config key is absent, do NOT emit a principal scalar block with a synthetic literal such as `(missing)` / `(不存在)`. Prefer a summary-led explanation so the answer can lead with the exact absence and then explain any grounded same-family precedence chain as related context only.\n")
			b.WriteString("- For config-trace related context, grounded same-scope anchors may appear in `summary` even when they do not carry a validated diagram role. But fenced diagrams and diagram citations are stricter: only anchors with a validated `diagram_role_hint` (`default`, `config`, `runtime`, or `override`) may become diagram nodes.\n")
			b.WriteString("- When a requested precedence layer has no grounded binding, say that layer absence explicitly (for example, `no config-file key matches this target` or `no CLI flag binds this key`) instead of vague placeholders like `N/A` / `不适用`.\n")
			b.WriteString("- For config-precedence answers, only create a separate numbered step when that layer has its own grounded repo anchor. If a layer is absent or only inferred from the exact-absence state, keep it in `summary` or set that step's `citation_ref=-1` instead of borrowing a nearby config-file / struct citation.\n")
			b.WriteString("- In an ordered-list block, any step with `citation_ref >= 0` must mention at least one identifier that appears on the cited line or its nearby corroboration window. If the step summarizes a whole struct/range/absence conclusion rather than one corroborated line, use `citation_ref=-1` and keep the precise line-backed facts in neighboring steps.\n")
			b.WriteString("- A repo-wide search result, aggregate absence conclusion, or test-only proof step usually has no single corroborating production line. In `step_list`, default those steps to `citation_ref=-1` unless one cited line literally states the same claim.\n")
			if roles := types.JoinEvidenceDiagramRoles(contract.RequestedContextRoles); roles != "" {
				fmt.Fprintf(&b, "- The user explicitly asked to cover these nearby precedence roles when you keep follow-on context: `%s`. Do not collapse the answer to only one surviving layer when grounded anchors for the other requested roles are still available in this dispatch.\n", roles)
			}
		}
	}
	if plan != nil && plan.PreferredExactResolution != nil {
		fmt.Fprintf(&b, "- Preferred exact_resolution object for this dispatch: `{\"status\":\"%s\",\"context_mode\":\"%s\"}`.\n",
			plan.PreferredExactResolution.Status, plan.PreferredExactResolution.ContextMode)
	}
	if plan != nil && plan.SummarySurfaceMode == types.AnswerSummarySurfaceFollowOnGroundedContext {
		b.WriteString("- Summary surface mode: follow-on grounded context only. The renderer already prints the exact-target absence lead, so `summary` should start directly on the grounded nearby context/mechanism and must not restate the exact target.\n")
	}
	if plan != nil && plan.SummarySurfaceMode == types.AnswerSummarySurfaceDriftBoundedRootCause {
		b.WriteString("- Summary surface mode: drift-bounded root cause. The renderer will prepend the log-source-drift lead, but it will keep your grounded `summary`. Use `summary` for the concise present-tense cause/mechanism sentence (and the compiled call-chain fence when helpful). Do not assert a more specific historical failure cause than the current cited code proves.\n")
		// B8-T1 part 3: the original step_list-only nudge is now
		// covered by the V2 block contract's BlockOrderedList
		// requirement on QFCallChain / QFRootCauseTrace families.
		// The general drift-bounded surface guidance still
		// applies to all shapes.
		b.WriteString("- For ordered-step or call-chain answers, keep every step directly citation-backed instead of adding uncited root-cause hypotheses.\n")
	}
	if ctx.AnalysisIR != nil &&
		ctx.AnalysisIR.RequestModel.Scenario == types.ScenarioConfigTrace &&
		contract.TargetKind == types.SubjectConfigKey {
		b.WriteString("- When you mention a precedence layer that lacks a grounded binding for the requested target, say that layer absence explicitly (for example `no config-file key matches this target` or `no CLI flag binds this key`) instead of vague placeholders like `N/A` / `不适用`.\n")
	}
	b.WriteString("\n")
	citationGradeRendered := false
	if citationGrade := renderAnswerDocCitationGradeExactContextAnchors(ctx, contract); citationGrade != "" {
		citationGradeRendered = true
		b.WriteString(citationGrade)
	}
	if primary := renderAnswerDocPrimaryExactProofCitationSeeds(ctx, contract); primary != "" {
		b.WriteString(primary)
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
	if missing := renderAnswerDocConfigTraceMissingLayerWording(ctx, contract); missing != "" {
		b.WriteString(missing)
	}
	if forbidden := renderAnswerDocForbiddenExactContextAnchors(ctx, contract); forbidden != "" {
		b.WriteString(forbidden)
	}
	if seeds := renderAnswerDocExactResolutionSeeds(ctx, contract); seeds != "" {
		b.WriteString(seeds)
	}
	return b.String()
}

func renderAnswerDocConfigTraceMissingLayerWording(ctx *types.AgentContext, contract *types.ExactResolutionContract) string {
	if ctx == nil || ctx.AnalysisIR == nil || contract == nil {
		return ""
	}
	if ctx.AnalysisIR.RequestModel.Scenario != types.ScenarioConfigTrace || contract.TargetKind != types.SubjectConfigKey {
		return ""
	}
	missing := types.ConfigTraceMissingRequestedDiagramRoles(contract, answerDocExactContextRequiredFiles(ctx), ctx.EvidenceItems)
	if len(missing) == 0 {
		return ""
	}
	labelByRole := types.ConfigTraceRequestedRoleLabels(ctx.AnalysisIR.RequestModel, contract)
	var lines []string
	lines = append(lines, "- Populate document-level `missing_requested_roles[]` with the missing requested layers below; the renderer will materialise the explicit missing-layer prose from this typed field.")
	for _, role := range missing {
		label := strings.TrimSpace(labelByRole[role])
		switch role {
		case types.EvidenceDiagramRoleConfig:
			if label == "" {
				label = "config-file"
			}
			lines = append(lines,
				fmt.Sprintf("- For the missing `%s` layer, prefer explicit absence wording such as `%s 层没有匹配该键` or `no %s key matches this target`.", role, label, label))
		case types.EvidenceDiagramRoleOverride:
			if strings.EqualFold(strings.TrimSpace(label), "cli") {
				lines = append(lines,
					"- For the missing `override` layer when the user names it as `CLI`, prefer explicit absence wording such as `CLI 层未绑定该键` or `no CLI flag binds this key`.")
				continue
			}
			if label == "" {
				label = "override"
			}
			lines = append(lines,
				fmt.Sprintf("- For the missing `%s` layer, prefer explicit absence wording such as `%s 层未绑定该键` or `no %s binding exists for this target`.", role, label, label))
		case types.EvidenceDiagramRoleDefault:
			if label == "" {
				label = "code default"
			}
			lines = append(lines,
				fmt.Sprintf("- For the missing `%s` layer, prefer explicit absence wording such as `%s 层没有为该精确键提供绑定` or `no %s binding exists for this exact key`.", role, label, label))
		case types.EvidenceDiagramRoleRuntime:
			if label == "" {
				label = "runtime"
			}
			lines = append(lines,
				fmt.Sprintf("- For the missing `%s` layer, prefer explicit absence wording such as `%s 层未提供该键的运行时绑定` or `no %s binding exists for this target`.", role, label, label))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Explicit Missing-Layer Wording\n\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func renderAnswerDocAcceptedClosure(ctx *types.AgentContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		return ""
	}
	if strings.TrimSpace(plan.StableInvestigationReason) == "" && strings.TrimSpace(plan.StableAbsenceJustification) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Accepted Closure Status\n\n")
	b.WriteString("- A prior exploration window already reached a stable closure for this dispatch.\n")
	if plan.StableAbsent {
		b.WriteString("- The stable state currently remains an accepted bounded absence / no-hit result for the exact target under the explored scope.\n")
	}
	b.WriteString("- Treat prior closure state only as a scope / completeness floor.\n")
	b.WriteString("- Rebuild user-visible principal claims from the typed support lanes, current citations, and the semantic view below. Do not copy prior closure prose into the `summary` block or principal `ordered_list` items.\n\n")
	return b.String()
}

func renderAnswerDocRuntimeGroundingDisposition(ctx *types.AgentContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || !plan.RuntimeGroundingDisposition.IsActive() {
		return ""
	}
	d := plan.RuntimeGroundingDisposition
	var b strings.Builder
	b.WriteString("## Runtime Grounding Disposition\n\n")
	b.WriteString("A validated runtime artifact disposition says ordinary current-repo citation floors do not apply to the artifact's own observations.\n")
	fmt.Fprintf(&b, "- Source: `%s`\n", d.Source)
	fmt.Fprintf(&b, "- Reason: `%s`\n", d.Reason)
	fmt.Fprintf(&b, "- Rationale: %s\n", strings.TrimSpace(d.Rationale))
	b.WriteString("- Citation policy: claims whose source is the attached log / trace itself must use `citation_ref=-1` unless a cited current-repo line literally states the same claim.\n")
	b.WriteString("- Current repository citations may still be used for explicitly read current-code context, but do not present them as the source of the runtime observation or as proof that the current checkout produced the captured log / trace.\n\n")
	return b.String()
}

func renderAnswerDocCurrentStatusDiagnostic(ctx *types.AgentContext) string {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.AnalysisIR.AnswerContract.CurrentStatusDiagnostic == nil {
		return ""
	}
	contract := ctx.AnalysisIR.AnswerContract.CurrentStatusDiagnostic
	if !contract.Required {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Current Status Diagnostic\n\n")
	b.WriteString("This answer must stay on two lanes and end with one bounded verdict.\n\n")
	b.WriteString("- Historical observation: state what the attached artifact, trace, or user-described prior symptom actually observed. Do not treat that historical fact as proof that the current checkout still behaves the same way.\n")
	b.WriteString("- Current code verification: cite the current code that was read and explain whether the same risk path is still present, blocked, removed, or not provable from the available evidence.\n")
	b.WriteString("- Verdict: emit a principal `decision` block whose text starts with exactly one of `still_present`, `fixed`, or `not_enough_evidence`, then explain the evidence boundary.\n\n")
	if profile := ctx.AnalysisIR.RequestModel.ArtifactObservationProfile; profile != nil {
		b.WriteString("Typed artifact observations available for the historical lane:\n")
		if profile.SymptomSummary != "" {
			fmt.Fprintf(&b, "- Summary: %s\n", profile.SymptomSummary)
		}
		if len(profile.ObservationKinds) > 0 {
			fmt.Fprintf(&b, "- Observation kinds: %s\n", strings.Join(profile.ObservationKinds, ", "))
		}
		if profile.HasRetryLoop {
			b.WriteString("- Runtime/process observation includes a retry loop.\n")
		}
		if profile.HasLineMismatch {
			b.WriteString("- Runtime/process observation includes a line-mapping mismatch.\n")
		}
		if profile.HasCompletionRewrite {
			b.WriteString("- Runtime/process observation includes a completion rewrite / topical repair signal.\n")
		}
		limit := len(profile.EvidenceSnippets)
		if limit > 4 {
			limit = 4
		}
		for i := 0; i < limit; i++ {
			fmt.Fprintf(&b, "- Evidence snippet: %s\n", profile.EvidenceSnippets[i])
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderAnswerDocSupportPlan(ctx *types.AgentContext) string {
	plan := answerSupportPlan(ctx)
	if plan == nil || len(plan.Lanes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Typed Answer Support Lanes\n\n")
	b.WriteString("- Build the principal `summary` block and principal `ordered_list` items only from the lanes below.\n")
	b.WriteString("- Keep observed runtime facts, current code path facts, nearest grounded mechanism facts, and uncertainty disclosures in their own lanes.\n")
	b.WriteString("- Do not promote an observation lane into caller-side provenance, old-build internals, or exact mechanism unless a current cited line explicitly proves that stronger claim.\n\n")
	b.WriteString("- Treat each lane's `Allowed block kinds` as a hard surface boundary. If a lane does not list `ordered_list`, do not turn its entries into principal hop items. If a lane does not list `diagram`, do not turn its entries into diagram edges or nodes.\n\n")
	b.WriteString("- Items rendered under the **Observed artifact facts** lane are runtime trace observations (the system tags them by typed source: panic / exception / traceback / perf bundle, regardless of language). They prove that the runtime hit the cited frame, but do NOT, by themselves, prove which source parameter was nil, which caller originated the value, or which exact downstream branch executed. Promotion to caller-side provenance / source-parameter mapping / exact mechanism requires a separately-cited current-code line.\n\n")
	b.WriteString("- If a function, file, or hop appears elsewhere in the prompt but does NOT appear in the current code path / nearest mechanism / boundary lanes below, treat it as background only: do not turn it into a principal ordered-list hop, summary claim, or diagram node.\n\n")
	b.WriteString("- In drift-bounded root-cause answers, a current guard plus a later dereference proves the current code contains both sites; it does NOT by itself prove the runtime artifact actually passed the guard and reached the dereference path.\n\n")
	b.WriteString("- If the lanes below do NOT recover a grounded inner trigger statement, do NOT fill the gap with generic language-runtime guesses such as nil-map write, nil-slice index, field dereference, or similar builtin panic classes. State only that the exact internal trigger remains unrecovered in the current checkout.\n\n")
	hasMechanismLane := false
	for _, lane := range plan.Lanes {
		if lane.Kind == types.SupportLaneNearestMechanism && len(lane.Entries) > 0 {
			hasMechanismLane = true
			break
		}
	}
	if !hasMechanismLane {
		b.WriteString("- If the **Nearest grounded mechanism** lane is absent, do NOT invent a likely internal cause, specific nil-bearing variable, receiver field, or caller-side provenance from the current code path or boundary lanes alone. Keep the answer at the level of the observed frame, the grounded current path, and the explicit uncertainty boundary.\n\n")
	}
	for _, lane := range plan.Lanes {
		title := strings.TrimSpace(lane.Title)
		if title == "" {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", title)
		if guidance := strings.TrimSpace(lane.Guidance); guidance != "" {
			b.WriteString(guidance)
			b.WriteString("\n\n")
		}
		if len(lane.AllowedBlocks) > 0 {
			fmt.Fprintf(&b, "Allowed block kinds: %s\n\n", strings.Join(lane.AllowedBlocks, ", "))
		}
		for _, entry := range lane.Entries {
			text := strings.TrimSpace(entry.Text)
			if text == "" {
				continue
			}
			if loc := strings.TrimSpace(entry.Location); loc != "" {
				fmt.Fprintf(&b, "- %s (`%s`)\n", text, loc)
			} else {
				fmt.Fprintf(&b, "- %s\n", text)
			}
		}
		b.WriteString("\n")
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

func answerDocumentAuthorityEvidencePool(ctx *types.AgentContext) []types.EvidenceItem {
	if ctx == nil {
		return nil
	}
	var emitted []types.EvidenceItem
	if ctx.Mutable != nil {
		emitted = ctx.Mutable.EmittedEvidence()
	}
	return mergeEvidenceItems(emitted, ctx.EvidenceItems)
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
	b.WriteString("These grounded same-scope anchors may stay in `summary` as nearby context even when they do not yet carry a validated precedence role. Keep them out of fenced diagrams and `citations[]` unless a validated `diagram_role_hint` is already attached to them.\n\n")
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
		fmt.Fprintf(&b, "- `%s` → `%s`\n", anchor.Role, types.ConfigTraceDiagramAnchorSupportLabel(anchor))
	}
	b.WriteString("\n")
	return b.String()
}

func renderAnswerDocLogSourceDrift(ctx *types.AgentContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || len(plan.LogSourceDriftAnchors) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Log Source Drift\n\n")
	if plan.RuntimeGroundingDisposition.IsActive() {
		b.WriteString("The runtime artifact has also been marked as observation-first for citation purposes. Treat the log / trace bytes as authoritative for what was observed; use current repository code only as separately grounded adjacent context, not as proof that this checkout produced the captured runtime state.\n\n")
	} else {
		b.WriteString("The runtime log resolves to files in this repo, but at least one resolved frame no longer aligns to the same line range in the current checkout. Treat the current repo as the authoritative explanation surface and the log line numbers as an older or shifted build snapshot.\n\n")
	}
	b.WriteString("- Do not claim that the current cited line is the exact crashing line from the log unless that exact line is itself cited.\n")
	b.WriteString("- Use the current grounded code to explain the nearest verified function / call-chain / guard path, and keep the line-number mismatch as a caveat rather than guessing how the old build differed internally.\n")
	b.WriteString("- Avoid speculative bypass stories (race, skipped guard, alternate branch) unless a cited anchor explicitly proves them.\n\n")
	limit := len(plan.LogSourceDriftAnchors)
	if limit > 4 {
		limit = 4
	}
	for i := 0; i < limit; i++ {
		anchor := plan.LogSourceDriftAnchors[i]
		funcLabel := strings.TrimSpace(anchor.Func)
		if funcLabel == "" {
			funcLabel = "(unknown symbol)"
		}
		fmt.Fprintf(&b, "- `%s:%d` in `%s` aligns most closely to current grounded anchor `%s:%d`.\n",
			anchor.File, anchor.ObservedLine, funcLabel, anchor.File, anchor.AnchoredLine)
	}
	if len(plan.DriftBoundedSurfaceItems) > 0 && answerSupportPlan(ctx) == nil {
		b.WriteString("\n### Drift-Bounded Current Surface\n\n")
		b.WriteString("Only claims directly expressible from the grounded anchors below belong in the `summary` block's text or in any principal `ordered_list` / `bullet_list` items. Treat anything beyond them as unproven older-build detail.\n\n")
		for _, item := range plan.DriftBoundedSurfaceItems {
			label := strings.TrimSpace(types.EvidenceStructuredSemanticLine(item, false))
			if label == "" {
				label = strings.TrimSpace(item.AnchorSymbol)
			}
			if label == "" {
				continue
			}
			location := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
			if location != "" && item.LineStart > 0 {
				location = fmt.Sprintf("%s:%d", location, item.LineStart)
			}
			if location != "" {
				fmt.Fprintf(&b, "- %s (`%s`)\n", label, location)
			} else {
				fmt.Fprintf(&b, "- %s\n", label)
			}
		}
	}
	b.WriteString("\n")
	return b.String()
}

func renderAnswerDocExternalObservationSeeds(ctx *types.AgentContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || len(plan.ExternalObservationSeeds) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## External Observation Seeds\n\n")
	b.WriteString("These observations come from the attached runtime / external trace rather than directly from current repo code. They are answer-grade context, but if you surface one as its own `step_list` hop, scalar provenance fact, or boolean rationale without a repo line that literally states the same claim, default that field's `citation_ref` to `-1` instead of borrowing a nearby repo citation.\n\n")
	limit := len(plan.ExternalObservationSeeds)
	if limit > 6 {
		limit = 6
	}
	for i := 0; i < limit; i++ {
		seed := plan.ExternalObservationSeeds[i]
		switch strings.TrimSpace(seed.Kind) {
		case "error_type":
			fmt.Fprintf(&b, "- Structured log error type: `%s`\n", seed.Raw)
		case "signal":
			fmt.Fprintf(&b, "- Structured runtime signal: `%s`\n", seed.Raw)
		case "log_observation":
			raw := strings.TrimSpace(seed.Raw)
			if raw == "" {
				continue
			}
			if subject := strings.TrimSpace(seed.Func); subject != "" {
				fmt.Fprintf(&b, "- Structured log observation about `%s`: %s\n", subject, raw)
			} else {
				fmt.Fprintf(&b, "- Structured log observation: %s\n", raw)
			}
		case "perf_jank":
			raw := strings.TrimSpace(seed.Raw)
			if raw == "" {
				continue
			}
			if span := strings.TrimSpace(seed.Func); span != "" {
				fmt.Fprintf(&b, "- Structured performance jank around `%s`: %s\n", span, raw)
			} else {
				fmt.Fprintf(&b, "- Structured performance jank: %s\n", raw)
			}
		case "perf_stall":
			raw := strings.TrimSpace(seed.Raw)
			if raw == "" {
				continue
			}
			if sym := strings.TrimSpace(seed.Func); sym != "" {
				fmt.Fprintf(&b, "- Structured performance stall at `%s`: %s", sym, raw)
			} else {
				fmt.Fprintf(&b, "- Structured performance stall: %s", raw)
			}
			if seed.File != "" && seed.Line > 0 {
				fmt.Fprintf(&b, " → observed at `%s:%d`", seed.File, seed.Line)
			}
			b.WriteString("\n")
		case "perf_frame", "perf_startup":
			raw := strings.TrimSpace(seed.Raw)
			if raw == "" {
				continue
			}
			fmt.Fprintf(&b, "- Structured performance observation: %s\n", raw)
		default:
			raw := strings.TrimSpace(seed.Raw)
			if raw == "" {
				raw = strings.TrimSpace(seed.Func)
			}
			if raw == "" {
				continue
			}
			fmt.Fprintf(&b, "- Runtime frame: `%s`", raw)
			if seed.AnchoredFile != "" && seed.AnchoredLine > 0 {
				fmt.Fprintf(&b, " → current code anchor `%s:%d`", seed.AnchoredFile, seed.AnchoredLine)
			} else if seed.File != "" && seed.Line > 0 {
				fmt.Fprintf(&b, " → observed at `%s:%d`", seed.File, seed.Line)
			}
			b.WriteString("\n")
		}
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
	b.WriteString("These nearby same-family / illustrative anchors are NOT answer-grade context for this dispatch. Do not cite them, do not turn them into diagram nodes, and do not surface them in the `summary` block's text or in any principal block's items unless the investigation is explicitly reopened with new grounded proof.\n\n")
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

func renderAnswerDocPrimaryExactProofCitationSeeds(ctx *types.AgentContext, contract *types.ExactResolutionContract) string {
	seeds := collectPrimaryExactProofCitationSeeds(ctx, contract)
	if len(seeds) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Primary Exact-Proof / Absence-Proof Citation Seeds\n\n")
	b.WriteString("Copy one or more of these citation objects verbatim into `citations[]` for the primary exact-target proof. In `exact_resolution.status=\"absent\"` mode, at least one cited seed MUST be an absence-proof object (for example `scope:\"negative\"` + `negative_pattern`). Nearby grounded context citations are secondary.\n\n")
	for _, seed := range seeds {
		fmt.Fprintf(&b, "- `%s`\n", seed)
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

func collectPrimaryExactProofCitationSeeds(ctx *types.AgentContext, contract *types.ExactResolutionContract) []string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || contract == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, ev := range plan.SurfaceEvidence {
		if !exactResolutionEvidenceIsPrimaryProof(contract, ev) {
			continue
		}
		seed := formatExactProofCitationSeed(ev)
		if seed == "" || seen[seed] {
			continue
		}
		seen[seed] = true
		out = append(out, seed)
	}
	sort.Strings(out)
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func exactResolutionEvidenceIsPrimaryProof(contract *types.ExactResolutionContract, ev types.EvidenceItem) bool {
	switch ev.GroundingStatus {
	case types.GroundingGrounded, types.GroundingRecovered:
	default:
		return false
	}
	if types.ExactResolutionEvidenceSupportsAbsence(contract, ev) {
		return true
	}
	if !types.ExactResolutionSourceIsDefiningPrimaryProofLike(contract, ev.Source) {
		return false
	}
	return types.ExactResolutionDirectAnchorMatchesAnyTarget(contract, ev.Subject, ev.AnchorSymbol, ev.Object)
}

func formatExactProofCitationSeed(ev types.EvidenceItem) string {
	if ev.Scope == types.ScopeNegative && ev.NegativeQuery != nil {
		cite := map[string]any{
			"file":             strings.TrimSpace(ev.NegativeQuery.File),
			"scope":            string(types.ScopeNegative),
			"negative_pattern": strings.TrimSpace(ev.NegativeQuery.Pattern),
		}
		if cite["file"] == "" || cite["negative_pattern"] == "" {
			return ""
		}
		buf, err := json.Marshal(cite)
		if err != nil {
			return ""
		}
		return string(buf)
	}
	cite := map[string]any{
		"file": strings.TrimSpace(ev.Source),
	}
	switch ev.Scope {
	case types.ScopeLine, "":
		if ev.LineStart <= 0 {
			return ""
		}
		cite["line"] = ev.LineStart
	case types.ScopeLineRange:
		if ev.LineStart <= 0 {
			return ""
		}
		cite["line"] = ev.LineStart
		if ev.LineEnd > ev.LineStart {
			cite["line_end"] = ev.LineEnd
		}
		cite["scope"] = string(types.ScopeLineRange)
	case types.ScopeSection:
		cite["scope"] = string(types.ScopeSection)
		if ev.LineStart > 0 {
			cite["line"] = ev.LineStart
		}
		if section := strings.TrimSpace(ev.SectionPath); section != "" {
			cite["section_path"] = section
		}
	case types.ScopeFile:
		cite["scope"] = string(types.ScopeFile)
		if ev.FileRoleLabel != "" {
			cite["file_role_label"] = string(ev.FileRoleLabel)
		}
	case types.ScopeCrossfile:
		cite["scope"] = string(types.ScopeCrossfile)
		if ev.CrossfileQuery != nil {
			if summary := strings.TrimSpace(ev.CrossfileQuery.Context); summary != "" {
				cite["crossfile_summary"] = summary
			}
		}
	default:
		if ev.LineStart <= 0 {
			return ""
		}
		cite["line"] = ev.LineStart
	}
	if strings.TrimSpace(ev.Source) == "" {
		return ""
	}
	buf, err := json.Marshal(cite)
	if err != nil {
		return ""
	}
	return string(buf)
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
		// B8-T4: V2 carrier — non-empty Blocks slice signals "emit
		// happened" (V2 docs are non-empty by validator construction;
		// any successful emit produces ≥1 block).
		if e.mu != nil {
			if docV2 := e.mu.AnswerDocumentV2(); docV2 != nil && len(docV2.Blocks) > 0 {
				return LoopSignal{StopRequested: true, StopReason: "emit_answer_document called"}
			}
		}
		if sig := e.unexpectedFinalizerToolSignal(obs); sig.HintRequested {
			return sig
		}
		// P3 (2026-05-10): switch-to-patch nudge after ≥ 2
		// consecutive emit_answer_document failures. Fires once
		// per dispatch (one-shot via emitPatchNudgeFired) and
		// returns BEFORE the field-specific reject signal so the
		// strategic guidance reaches the LLM at the highest
		// salience slot. The reject signal still fires on the
		// next iter via emitAnswerDocumentRejectSignal.
		if sig := e.emitSwitchToPatchSignal(obs); sig.HintRequested {
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
		// B8-T4: V2 carrier presence check — non-empty Blocks means
		// the LLM emitted; soft-stop accepts the response.
		if docV2 := e.mu.AnswerDocumentV2(); docV2 != nil && len(docV2.Blocks) > 0 {
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
		HintKey:       fmt.Sprintf("answer_doc.missing_document.%d", e.retriesUsed),
		// Instruct the model to REUSE its prior prose rather than rewrite it.
		// Without this directive, the second pass tends to produce a
		// compressed paraphrase instead of copying the richer draft into
		// the `summary` field — a measurable shrinkage of answer quality.
		Hint: "The answer must be delivered through the `emit_answer_document` tool call — text " +
			"written outside it does not ship. You already drafted the answer in your previous " +
			"message; treat that draft as your final text. Call `emit_answer_document` now and copy " +
			"your previous answer VERBATIM into the `summary` field. Do not trim for length unless " +
			"a prior tool rejection named an active summary cap. Derive the remaining required structured fields (citations[] " +
			"and any other block payloads the user-section's Required Answer Blocks list calls for) " +
			"from the same draft. Do NOT rewrite, compress, or paraphrase the content — the richness " +
			"of the original draft is the answer.",
	}
}

func (e *answerDocumentEvaluator) unexpectedFinalizerToolSignal(obs LoopObservation) LoopSignal {
	if obs.Phase != PhaseMidLoop || len(obs.Response.ToolCalls) == 0 {
		return LoopSignal{}
	}
	// Both emit_answer_document AND emit_answer_document_patch are
	// legitimate finalizer-stage tools (per
	// answer-document-skill.ToolSuggestions). The patch tool is the
	// preferred surface on retry paths because it preserves typed
	// annotations on unchanged blocks byte-identical. Treating patch
	// as "unexpected" was a regression that nudged the model away
	// from the more efficient path whenever a single patch call
	// failed (e.g. for a streaming-bug repair fix-up). The hint
	// fires only for tools genuinely outside the finalizer's
	// allowlist (read_file / grep / etc, which a pure synthesizer
	// stage never legitimately calls).
	var unexpected []string
	for _, tc := range obs.Response.ToolCalls {
		name := strings.TrimSpace(tc.Name)
		if name == "" || name == "emit_answer_document" || name == "emit_answer_document_patch" {
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
		HintKey:       fmt.Sprintf("answer_doc.unexpected_tool.%d.%s", obs.Iteration, toolList),
		Hint:          fmt.Sprintf("This stage is a pure answer synthesizer. Do NOT call `%s` or any other read/search tool. Re-emit `emit_answer_document` using only the already-provided grounded evidence: `citations[]`, Diagram Seeds, Exact Resolution Seeds, and the prompt's Diagram Node Allowlist. If a step cannot honestly cite one grounded line, keep that fact in `summary` or set that step's `citation_ref=-1` instead of reopening files. Do not write free-form prose outside the tool call.", toolList),
	}
}

// emitSwitchToPatchSignal (P3 2026-05-10) emits a strategic nudge
// after ≥ 2 consecutive emit_answer_document failures within the
// current dispatch, suggesting the LLM switch to
// emit_answer_document_patch on the next attempt. The patch path
// only requires the LLM to specify the changed blocks (instead of
// re-emitting the full document byte-identical), which is far less
// error-prone for the kind of small structural fix that drove the
// 2026-05-10 sweep digest's u10a 8-finalizer-iter outlier.
//
// Streak update side effect: this function maintains
// emitFullDocFailStreak as a side effect of being called per Observe
// pass. When LastToolResult is a successful emit (either path), the
// streak resets — a successful prior attempt means the LLM doesn't
// need this nudge again.
//
// Re-fire semantics (2026-05-10 follow-up after sweep forensic): the
// nudge re-fires on every emit_answer_document failure beyond
// streak>=2, capped by the per-HintKey cap (P2 MaxPerKeyInjects=5).
// emitPatchNudgeFired is set ONLY after the LLM is observed calling
// emit_answer_document_patch — i.e. the LLM has SEEN and acted on
// the recommendation. The signal also carries BypassThrottle=true
// so the cross-key MinInjectInterval can't suppress it the way it
// did at s7b finalizer iter=1.
//
// Returns empty signal when:
//   - LastToolResult is nil (no recent tool call)
//   - LastToolResult is not emit_answer_document (a patch call
//     latches the nudge; other tools are ignored)
//   - LastToolResult succeeded (streak resets)
//   - streak < 2 (only fires after the second failure to give the
//     LLM one fair chance with the simpler full-doc path)
//   - emitPatchNudgeFired (LLM has switched to patch; no need to
//     keep nagging)
//
// The hint key is "answer_doc.switch_to_patch" — operators can
// grep traces for this key to measure adoption / impact.
func (e *answerDocumentEvaluator) emitSwitchToPatchSignal(obs LoopObservation) LoopSignal {
	if obs.LastToolResult == nil {
		return LoopSignal{}
	}
	tn := obs.LastToolResult.ToolName
	// LLM acknowledged the nudge by switching to patch. Latch the
	// gate so further failures don't keep nagging. Patch SUCCESS
	// also resets the streak so a subsequent unrelated full-doc
	// retry gets a fresh count.
	if tn == "emit_answer_document_patch" {
		e.emitPatchNudgeFired = true
		if obs.LastToolResult.Success {
			e.emitFullDocFailStreak = 0
		}
		return LoopSignal{}
	}
	// Successful full-doc emit → reset the streak (LLM is making
	// progress; further attempts shouldn't be told to switch).
	if obs.LastToolResult.Success && tn == "emit_answer_document" {
		e.emitFullDocFailStreak = 0
		return LoopSignal{}
	}
	// Only count failures of the FULL emit path. Other tool failures
	// are unrelated — don't bump the streak on them.
	if tn != "emit_answer_document" {
		return LoopSignal{}
	}
	// Failed emit_answer_document → bump the streak.
	e.emitFullDocFailStreak++
	if e.emitFullDocFailStreak < 2 {
		return LoopSignal{}
	}
	if e.emitPatchNudgeFired {
		return LoopSignal{}
	}
	// Honor the same rejectHintBudget the legacy reject-hint path
	// uses. Re-firing the patch nudge alongside legacy rejects
	// would otherwise burn the budget twice as fast; respecting
	// the cap keeps cross-budget accounting consistent and lets
	// the legacy "stay silent after exhaustion" contract hold.
	// MidLoopRejectStopsHintingAfterBudget pins this behaviour.
	if budget := e.rejectHintBudget(); budget > 0 && e.rejectHintsUsed >= budget {
		return LoopSignal{}
	}
	// The nudge fires INSTEAD of the legacy reject signal on this
	// observation (the Observe ordering returns this signal first).
	// Bump rejectHintsUsed so the cross-budget accounting stays
	// consistent — every fire stole one legacy reject-hint slot,
	// so the budget tracks parity per-fire, not per-dispatch.
	e.rejectHintsUsed++
	return LoopSignal{
		HintRequested:  true,
		BypassThrottle: true,
		HintKey:        "answer_doc.switch_to_patch",
		Hint:           "Your last 2+ attempts to call `emit_answer_document` were rejected. Switch to `emit_answer_document_patch` on the next attempt — it lets you specify ONLY the blocks that need to change, instead of re-emitting the full document byte-identical. Pass `unchanged_block_ids: [\"id1\", \"id2\", ...]` to assert preservation of every typed annotation field (claim_uses, edge_anchors, facet_ids, surface_role) on blocks you do NOT need to edit — the system clones them byte-identical from your previous emit, so you cannot accidentally drop a field. Use `replace_blocks` for the blocks you DO need to fix; use `add_blocks` for new blocks. The patch tool rejects empty patches and unknown ids, so you only need to focus on the actual fix.",
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

	hint := answerDocPreserveHintIntro + " Re-emit `emit_answer_document` now: paste the FULL previous payload byte-identical, then change ONLY the field(s) named in the tool error. Every other field — `blocks[]` (every block id and item id you previously emitted), `citations[]`, `claim_uses[]` annotations, `exact_resolution` — must reproduce the prior emit byte-identical. Do not reopen files or call read/search tools. Do not write free-form prose outside the tool call."
	reasonKey := "tool-reject"
	if repair != nil && strings.TrimSpace(repair.Hint) != "" {
		hint = strings.TrimSpace(repair.Hint)
		reasonKey = firstNonEmptyString(rejectCode, "tool-reject")
		if len(repair.Fields) > 0 && !repairHintMentionsFields(hint, repair.Fields) {
			hint = answerDocFixOnlyDirective(repair.Fields) + " " + hint
		}
		if !strings.Contains(hint, "Do not write free-form prose outside the tool call.") {
			hint += " Do not write free-form prose outside the tool call."
		}
	}
	if detail := compactToolRejectSummary(summary); detail != "" && (repair == nil || strings.TrimSpace(repair.Hint) == "") {
		hint = answerDocPreserveHintIntro + " Re-emit `emit_answer_document` now: paste the FULL previous payload byte-identical, then change ONLY the field(s) named in this exact tool error: " + detail + ". Every other field must stay byte-identical to the prior emit; do not reopen files. Do not write free-form prose outside the tool call."
	}

	var summaryLen, summaryCap int
	if _, err := fmt.Sscanf(summary, "summary length %d exceeds cap %d", &summaryLen, &summaryCap); err == nil && summaryCap > 0 {
		reasonKey = "summary-cap"
		if e.diagramRequired {
			hint = fmt.Sprintf("Your last `emit_answer_document` call was rejected because `summary` was too long (cap %d chars, current %d). Re-emit `emit_answer_document` now with the same grounded answer but shorten `summary` below %d chars. Preserve the required grounded diagram; compress prose, repeated headings, and repeated citation prose first. Keep the facts in the tool fields and `citations[]`; do not write free-form prose outside the tool call.", summaryCap, summaryLen, summaryCap)
		} else {
			hint = fmt.Sprintf("Your last `emit_answer_document` call was rejected because `summary` was too long (cap %d chars, current %d). Re-emit `emit_answer_document` now with the same grounded answer but shorten `summary` below %d chars. Cut large diagrams, repeated headings, and repeated citation prose first. Keep the facts in the tool fields and `citations[]`; do not write free-form prose outside the tool call.", summaryCap, summaryLen, summaryCap)
		}
	}

	if rejectCode == answerDocRejectCodeMissingDiagram || strings.Contains(summary, "diagram required for this dispatch") {
		reasonKey = "missing-diagram"
		hint = "Your last `emit_answer_document` call was rejected because this dispatch REQUIRES a grounded `diagram` block. Re-emit `emit_answer_document` now with the same answer payload, but add a `diagram` block: set `diagram.kind` to the SEMANTIC family the user section names (`flow` / `sequence` / `architecture` / `call_dag`) — NOT a Mermaid keyword — and put the Mermaid syntax inside `diagram.body` (using `flowchart` for `flow`/`architecture`/`call_dag` family, `sequenceDiagram` for `sequence`). Set `diagram.language=\"mermaid\"`. Keep every filename inside the diagram grounded by `citations[]` or the Log Triage frames; do not write free-form prose outside the tool call."
		if e.configTraceDiagram {
			hint += " For config-precedence diagrams, do not invent a new box chart or layer aliases on retry; the seeded grounded precedence chain is the FLOOR — keep every node, but you MAY add additional grounded layers if your evidence supports a richer chain. Stay in ` ```mermaid ` form."
		}
		hint = appendRetryDiagramSeedHint(hint, ctx, repair)
	}
	if rejectCode == answerDocRejectCodeDiagramGrounding || strings.Contains(summary, "references file(s) not present in citations[] or attached-log frames") {
		reasonKey = "diagram-grounding"
		hint = "Your last `emit_answer_document` call was rejected by the DIAGRAM-GROUNDING gate: the fenced diagram renamed or introduced file/path labels that are not grounded. Re-emit `emit_answer_document` now with the same answer, but inside the diagram reuse the exact grounded file / symbol / path labels from citations, cited line text, or Log Triage frames. Prefer copying directly from the prompt's `Diagram Node Allowlist` section or from the tool error's allowed-label list. Do NOT normalize one grounded label into a different spelling unless that alternate label is itself grounded. Keep support locations outside the fence: do not append `(<file>:<line>)`, `<br/>(<file>)`, or similar support suffixes to a node label unless that exact combined label is itself grounded. Prefer direct grounded node names over abstract aliases. Keep `citations[]` byte-identical while repairing this gate unless a later tool error explicitly names `citations[]` as invalid. Do NOT call `read_file`, `grep`, or any other tool to repair this — use the existing citations / seeds only. Do not write free-form prose outside the tool call."
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
		hint = "Your last `emit_answer_document` call was rejected because the `summary` block's text leaked exact-target / nearby-context material that does not belong on the user-visible answer surface. Re-emit `emit_answer_document` now with the same `exact_resolution` object and the same block layout, but treat the `summary` block as the follow-on grounded-context block only."
		if repair != nil && repair.Metadata != nil {
			if repeated := strings.TrimSpace(repair.Metadata["repeated_target"]); repeated != "" {
				hint += " Do NOT restate " + repeated + " in the `summary` block's text: the renderer already prints the exact-target lead for this dispatch."
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
	if rejectCode == answerDocRejectCodeLogSourceDriftStepCitation && repair != nil {
		reasonKey = "log-source-drift-step-citation"
		hint = strings.TrimSpace(repair.Hint)
		hint += " Keep the drift explanation in the renderer-generated lead / caveat, keep the call-chain fence grounded, and make every remaining step directly citation-backed."
	}
	if repair != nil && repair.Code == "exact_resolution" && repair.Metadata != nil {
		if locked := strings.TrimSpace(repair.Metadata["locked_status"]); locked != "" {
			reasonKey = "exact-resolution-locked"
			hint = "Your last `emit_answer_document` call was rejected by the exact-resolution contract because the status is already locked. Re-emit `emit_answer_document` now with `exact_resolution.status=\"" + locked + "\"`."
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
		plan := answerSurfacePlan(ctx)
		if allowedCitations != "" {
			if plan != nil && plan.SummarySurfaceMode == types.AnswerSummarySurfaceFollowOnGroundedContext {
				hint += " This dispatch is in follow-on grounded-context mode, so do NOT collapse the answer to the exact-absence lead alone. Keep the nearby precedence / lineage context on the user-visible surface, cite at least one allowed file:line lineage anchor when you keep citation-grade lineage, and move any prose-only anchors out of `citations[]` / fenced diagrams."
			} else {
				hint += " Choose one valid repair path now: either (a) keep nearby precedence / lineage context, cite at least one allowed file:line lineage anchor, and move any prose-only anchors out of `citations[]` / fenced diagrams, or (b) delete that nearby context from the user-visible answer surface and keep only the exact-absence lead. Do not keep broad same-family background as the cited explanation."
			}
		} else {
			if plan != nil && plan.SummarySurfaceMode == types.AnswerSummarySurfaceFollowOnGroundedContext {
				hint += " This dispatch is in follow-on grounded-context mode. No citation-grade nearby lineage anchors are available for this dispatch, so do not invent a replacement citation. Keep the nearby grounded context visible as uncited prose-only explanation instead of collapsing to the exact-absence lead alone."
			} else {
				hint += " No citation-grade nearby lineage anchors are available for this dispatch, so do not invent a replacement citation. Either keep only short uncited background that does not become the answer's visible lineage explanation, or delete the nearby context entirely and keep only the exact-absence lead."
			}
		}
		hint = appendRetryDiagramSeedHint(hint, ctx, repair)
	}
	if rejectCode == answerDocRejectCodeFollowOnGroundedContext && repair != nil {
		reasonKey = "follow-on-grounded-context"
		hint = strings.TrimSpace(repair.Hint)
		if repair.Metadata != nil {
			if allowed := strings.TrimSpace(repair.Metadata["allowed_anchors"]); allowed != "" {
				hint += " Keep the visible nearby context on this validated anchor set: `" + strings.ReplaceAll(allowed, ", ", "`, `") + "`."
			}
			if mode := strings.TrimSpace(repair.Metadata["preferred_context_mode"]); mode != "" && !strings.Contains(hint, "exact_resolution.context_mode") {
				hint += " Keep `exact_resolution.context_mode=\"" + mode + "\"` while that nearby context remains visible."
			}
		}
		hint += " Do not collapse the answer to the renderer-generated exact-absence lead alone."
	}
	if rejectCode == answerDocRejectCodeLogTriageCoverage && repair != nil && strings.TrimSpace(repair.Hint) != "" {
		reasonKey = "log-triage-coverage"
		hint = strings.TrimSpace(repair.Hint)
	}
	if rejectCode == answerDocRejectCodeScalarSummaryRequired && repair != nil && strings.TrimSpace(repair.Hint) != "" {
		reasonKey = "scalar-summary-required"
		hint = strings.TrimSpace(repair.Hint)
	}
	// Session-22 special-case: the literal-grounding gate on a
	// resolved-literal answer fires when the cited line has zero
	// identifier overlap with the emitted literal. The tool's error body
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
		hint = answerDocFixOnlyDirective(repair.Fields) + " " + hint
	}
	// Catastrophic-regression override: when the tool flagged
	// payload_regression in the Repair code, the existing hint is
	// secondary — payload restoration must come FIRST. The metadata
	// summary names which fields collapsed (e.g. "citations 18→0;
	// steps 16→0; summary runes 4250→0") and the override hint
	// instructs the LLM to paste back the prior payload before
	// applying any field-specific fix. Without this, the trace at
	// /home/chatpp/pytest 2026-04-29 23:11 keeps repeating: each
	// reject hint targets one field, model interprets it as "emit
	// only that field", payload shrinks further, next reject fires
	// on a now-different missing field, retry budget exhausts.
	if repair != nil && answerDocRepairIsRegression(repair) {
		fields := repair.Fields
		if len(fields) == 0 {
			fields = []string{"the field(s) named in the tool error"}
		}
		regressionSummary := ""
		if repair.Metadata != nil {
			regressionSummary = repair.Metadata["payload_regression_summary"]
		}
		hint = answerDocPayloadRegressionHint(regressionSummary, fields)
		reasonKey = "payload-regression"
	}
	if !strings.Contains(hint, "Do not write free-form prose outside the tool call.") {
		hint += " Do not write free-form prose outside the tool call."
	}

	// B2-F4 retry escalation: the dedup key already embeds the
	// retry counter so each retry gets a fresh delivery, but the
	// hint *text* itself was historically identical across retries
	// — the LLM saw the same wording and made the same mistake
	// again. Prepend an explicit escalation marker on retry ≥ 1 so
	// the model knows this is a re-prompt of an unchanged issue:
	// the text should be read MORE strictly (focus only on the
	// named field; do not re-investigate; do not re-frame the
	// answer). On retry ≥ 2 escalate further to "this is your
	// LAST retry on this issue — fix THIS field now or the answer
	// ships with the violation as a caveat".
	e.rejectHintsUsed++
	hint = answerDocAttachEscalation(hint, e.rejectHintsUsed)
	return LoopSignal{
		HintRequested:  true,
		HintKey:        fmt.Sprintf("answer_doc.reject.%s.%d", reasonKey, e.rejectHintsUsed),
		Hint:           hint,
		Progress:       true,
		BypassThrottle: true,
		BypassBudget:   true,
	}
}

// answerDocAttachEscalation prepends a retry-iteration aware
// marker to the hint text so retries are signal-progressive rather
// than text-identical. P14 escalation contract:
//
//	retry 1 (rejectHintsUsed == 1): no escalation — first hint at
//	  this issue, the LLM hasn't seen this text before.
//	retry 2 (rejectHintsUsed == 2): "RETRY — same issue persisted
//	  from the previous attempt" prefix to make explicit that this
//	  is a re-prompt of an unchanged failure.
//	retry 3+ (rejectHintsUsed >= 3): "FINAL RETRY — fix THIS field
//	  now or the answer ships with the violation as a caveat" so
//	  the LLM treats it as an absolute, not a suggestion.
func answerDocAttachEscalation(hint string, attempt int) string {
	if attempt <= 1 {
		return hint
	}
	if attempt == 2 {
		return "RETRY (this is your 2nd attempt on the SAME issue — your previous fix did not address it; re-read the named field and constraint below before re-emitting): " + hint
	}
	return "FINAL RETRY (this is attempt #" + fmt.Sprint(attempt) + " on the SAME issue — fix the named field NOW or the answer ships with the violation surfaced as a caveat): " + hint
}

// answerDocRepairIsRegression reports whether the repair's Code
// indicates the catastrophic-regression detector fired in the tool.
// The tool stamps Code as either "payload_regression" (no underlying
// reject) or "payload_regression:<original>" (regression layered on
// top of an existing reject — the prefix preserves the original
// reject reason for log forensics).
func answerDocRepairIsRegression(repair *types.ToolRepair) bool {
	if repair == nil {
		return false
	}
	code := strings.TrimSpace(repair.Code)
	return code == "payload_regression" || strings.HasPrefix(code, "payload_regression:")
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

// answerDocPreserveHintIntro is the standard "preserve everything,
// change only X" preamble shared by every retry hint that asks the
// LLM to fix a small subset of fields. Pre-2026-04-30 the wording
// was "Fix ONLY these field(s)" — that intent ("change only X")
// was misread by some models as "emit only those fields", and the
// next iteration's payload dropped citations[], steps[], summary,
// etc. The trace at /home/chatpp/pytest 2026-04-29 23:11 shows the
// exact failure: iter=2 reject named two fields → iter=3 emit had
// summary="", steps missing, citations missing → exact_resolution
// guard surfaced because the rest of the payload was gone too.
//
// New wording is explicit and positive: paste the full prior emit
// verbatim, then change ONLY the named field(s). The byte-identical
// language nails down what "preserve" means without ambiguity.
const answerDocPreserveHintIntro = "Your last `emit_answer_document` call was rejected by the tool."

// answerDocFixOnlyDirective renders the "preserve everything except
// these fields" directive in the new positive-preserve wording.
// Replaces the bare "Fix ONLY these field(s): `X`. " prefix used
// pre-2026-04-30, which empirically caused models to drop the rest
// of the payload on the retry round.
func answerDocFixOnlyDirective(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	return "Re-emit `emit_answer_document` with the FULL previous payload byte-identical, then change ONLY these field(s): `" + strings.Join(fields, "`, `") + "`. Every other field — `blocks[]` (each block id, kind, payload, item ids, item text, item claim_use), `citations[]`, doc-level `claim_use` annotations, `exact_resolution` — must reproduce the prior emit byte-identical. Do not drop, blank, or shrink any other field."
}

// answerDocPayloadRegressionHint surfaces the "you dropped fields"
// override that takes precedence over a normal field-specific
// rejection hint when the latest emit is catastrophically smaller
// than the previous emit. The orchestrator-level catastrophic-
// regression detector (registered as Repair on the tool result)
// flags this; the evaluator surfaces a hint that focuses on
// payload restoration FIRST, then field correction.
func answerDocPayloadRegressionHint(droppedSummary string, fieldsToFix []string) string {
	var b strings.Builder
	b.WriteString("Your last `emit_answer_document` call REGRESSED — multiple grounded fields were dropped or blanked compared to the previous emit (")
	b.WriteString(droppedSummary)
	b.WriteString("). Re-emit `emit_answer_document` now: PASTE THE FULL PRIOR PAYLOAD BACK byte-identical (`blocks[]` with every block id and item, `citations[]`, doc-level `claim_use` annotations, `exact_resolution` — every field that was present last round), then change ONLY: `")
	b.WriteString(strings.Join(fieldsToFix, "`, `"))
	b.WriteString("`. Restoring the payload comes FIRST; field correction is secondary. Do not reopen files or call read/search tools. Do not write free-form prose outside the tool call.")
	return b.String()
}

const answerDocRejectPrefix = "[answer_doc_reject:"

const (
	answerDocRejectCodeMissingDiagram             = "missing_diagram"
	answerDocRejectCodeDiagramGrounding           = "diagram_grounding"
	answerDocRejectCodeDiagramCodename            = "diagram_codename"
	answerDocRejectCodeExactContextSurface        = "exact_context_surface"
	answerDocRejectCodeFollowOnGroundedContext    = "follow_on_grounded_context"
	answerDocRejectCodeExactResolution            = "exact_resolution"
	answerDocRejectCodeLogSourceDriftStepCitation = "log_source_drift_step_citation"
	answerDocRejectCodeLiteralGrounding           = "literal_grounding"
	answerDocRejectCodeScalarSummaryRequired      = "scalar_summary_required"
	answerDocRejectCodeLogTriageCoverage          = "log_triage_coverage"
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

func appendRetryDiagramSeedHint(hint string, ctx *types.AgentContext, repair *types.ToolRepair) string {
	seed := renderRetryDiagramSeedFenceForRepair(ctx, repair)
	if seed == "" {
		return hint
	}
	// Reframed alongside the First-Pass Diagram Reference: the
	// seed is a grounded FLOOR, not a paste-only ceiling. Pre-fix
	// said "copy this seeded fenced diagram verbatim ... do not
	// rename"; combined with the seed shape being ASCII art and
	// the model's default training prior to use ASCII, this drove
	// the entire "model only emits ASCII" failure mode. Now: keep
	// every node grounded, add more grounded nodes if richer
	// mechanism is supported, and the seed shape itself is
	// Mermaid so an extension stays in the preferred form.
	return hint + " A grounded reference fence is below — every node is already corroborated by the current dispatch, and some scenarios may use prevalidated role labels whose supporting anchors are listed elsewhere in the prompt. It is the safest FLOOR for the repair. You MAY extend it with additional grounded nodes / branches when your evidence supports a richer mechanism; the only hard rule is that every label inside the fence stays grounded or is an unchanged role label supplied by the prompt's diagram seed. ` ```mermaid ` form is preferred:\n\n" + seed
}

func renderRetryDiagramSeedFence(ctx *types.AgentContext) string {
	return renderRetryDiagramSeedFenceForRepair(ctx, nil)
}

type retryDiagramSeed struct {
	Fence         string
	MatchKeys     []string
	PreserveFence bool
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
	if repair == nil && !filter.Strict {
		if ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.Scenario == types.ScenarioConfigTrace {
			if seed := buildRetryConfigTraceDiagramSeed(ctx); strings.TrimSpace(seed.Fence) != "" {
				return seed.Fence
			}
		}
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
	if repair != nil {
		if fence := buildRetryCitationSeedFence(repair, filter); fence != "" {
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

// renderRetryDiagramSeedFenceForKind walks every applicable seed
// source for the given diagram kind, MERGES the grounded nodes
// across sources, and emits one unified Mermaid fence. Pre-fix this
// function returned the FIRST allowed seed and discarded the rest —
// a 3-node FlowFinding seed could shadow a 6-node AnswerChain seed,
// so the model saw fewer grounded nodes than the investigation
// actually surfaced. Merging plus the bumped retryDiagramSeedNodeCap
// (12 nodes) lets the seed reflect the full grounded mechanism the
// model can extend from.
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
		// Architecture chains often add useful upstream context to
		// a flow seed (entry-point components → handlers).
		seeds = appendRetryDiagramSeed(seeds, buildRetryAnswerChainSeed(ctx.AnswerChains))
	case types.DiagramSequence, types.DiagramCallDAG:
		seeds = appendRetryDiagramSeed(seeds, buildRetryLogDiagramSeed(ctx.LogTriage))
		seeds = appendRetryDiagramSeed(seeds, buildRetryFlowFindingSeed(ctx.FlowFindings))
		seeds = appendRetryDiagramSeed(seeds, buildRetryAnswerChainSeed(ctx.AnswerChains))
	case types.DiagramArchitecture:
		seeds = appendRetryDiagramSeed(seeds, buildRetryAnswerChainSeed(ctx.AnswerChains))
		seeds = appendRetryDiagramSeed(seeds, buildRetryFlowFindingSeed(ctx.FlowFindings))
	}
	// Filter then merge. mergeRetrySeedNodes preserves the order of
	// the FIRST seed (which is the strongest match) and appends
	// any additional unique nodes from later seeds, capped by
	// retryDiagramSeedNodeCap.
	allowed := seeds[:0]
	for _, seed := range seeds {
		if filter.Allows(seed) {
			allowed = append(allowed, seed)
		}
	}
	if len(allowed) == 0 {
		return ""
	}
	if allowed[0].PreserveFence {
		return allowed[0].Fence
	}
	merged := mergeRetrySeedNodes(allowed)
	if len(merged) >= 2 {
		return types.RenderLinearDiagramFence(merged, retryDiagramSeedNodeCap)
	}
	// Fallback: at least surface the strongest single seed's fence
	// when merging didn't yield enough distinct nodes (e.g. one
	// seed has a fully qualified path the others share by basename).
	return allowed[0].Fence
}

// mergeRetrySeedNodes combines node match-keys from multiple seeds,
// preserving order from the first seed and deduping. The match-keys
// are already grounded labels, so concatenating and deduping yields
// a unified grounded node list the model can read.
func mergeRetrySeedNodes(seeds []retryDiagramSeed) []string {
	seen := make(map[string]bool)
	var out []string
	for _, seed := range seeds {
		for _, key := range seed.MatchKeys {
			k := strings.TrimSpace(key)
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, k)
			if len(out) >= retryDiagramSeedNodeCap {
				return out
			}
		}
	}
	return out
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
	fence := types.RenderConfigTraceDiagramFence(anchors)
	if strings.TrimSpace(fence) == "" {
		return retryDiagramSeed{}
	}
	for _, anchor := range anchors {
		support := types.ConfigTraceDiagramAnchorSupportLabel(anchor)
		if support == "" {
			continue
		}
		keys = append(keys, retryDiagramSeedMatchKeys(support)...)
	}
	return retryDiagramSeed{
		Fence:         strings.TrimSpace(fence),
		MatchKeys:     dedupeRetryDiagramNodes(keys, 0),
		PreserveFence: true,
	}
}

func buildRetryLogDiagramSeed(bundle *types.LogBundle) retryDiagramSeed {
	resolved := collectRetryLogFrames(bundle)
	if len(resolved) < 2 {
		return retryDiagramSeed{}
	}
	keys := make([]string, 0, len(resolved)*2)
	for _, frame := range resolved {
		location := fmt.Sprintf("%s:%d", frame.File, frame.Line)
		keys = append(keys, retryDiagramSeedMatchKeys(location)...)
	}
	// Reuse the single canonical Mermaid renderer — types.RenderLogDiagramFence
	// emits the same ```mermaid``` flowchart shape used by every other
	// diagram seed. Pre-2026-04-30 this duplicated the bare ASCII art
	// shape, so even when the contract said "Mermaid preferred", the
	// model copied an ASCII skeleton verbatim.
	return retryDiagramSeed{
		Fence:     types.RenderLogDiagramFence(bundle),
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
	// Cap at 12 nodes — production stack chains and dispatch DAGs
	// routinely have 8-10 hops; the previous 6-node cap silently
	// truncated the seed and the model treated the truncated seed
	// as the authoritative chain length, omitting the rest from the
	// final diagram. 12 is empirically large enough for every
	// observed real-world chain without spilling into pathological
	// "everything is connected" mega-graphs.
	return dedupeRetryDiagramNodes(nodes, retryDiagramSeedNodeCap)
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

func buildRetryCitationSeedFence(repair *types.ToolRepair, filter retryDiagramSeedFilter) string {
	if repair == nil || repair.Metadata == nil {
		return ""
	}
	raw := strings.TrimSpace(repair.Metadata["allowed_citations"])
	if raw == "" {
		return ""
	}
	var nodes []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		nodes = append(nodes, part)
	}
	nodes = dedupeRetryDiagramNodes(nodes, 8)
	if len(nodes) < 2 {
		return ""
	}
	seed := retryDiagramSeed{
		Fence:     buildRetryDiagramFence(nodes),
		MatchKeys: dedupeRetryDiagramNodes(nodes, 0),
	}
	if !filter.Allows(seed) {
		return ""
	}
	return seed.Fence
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

// retryDiagramSeedNodeCap is the upper bound on nodes the seed
// renderer surfaces. Pre-2026-04-30 it was hardcoded as 6 at
// every callsite, which silently truncated real-world chains and
// trained the model to treat the truncated chain as authoritative
// — when production logs and dispatch DAGs routinely have 8-12
// hops. 12 covers every observed chain length without overflowing
// the LLM's working memory for one diagram block.
const retryDiagramSeedNodeCap = 12

func buildRetryDiagramFence(nodes []string) string {
	nodes = dedupeRetryDiagramNodes(nodes, retryDiagramSeedNodeCap)
	if len(nodes) < 2 {
		return ""
	}
	// Reuse the canonical Mermaid renderer instead of duplicating
	// the (formerly ASCII-art) builder here. Single source of seed
	// shape across `agent/` and `types/` keeps the runtime
	// injection in lockstep with the skill prompt's Mermaid
	// preference.
	return types.RenderLinearDiagramFence(nodes, 0)
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
//
// B8-T7: AnswerDocumentV2 is the only carrier; the legacy
// AnswerDocument field is gone. parseOutputV2 maintains a parallel
// data shape (final_answer + answer_document_v2) via its own type.
type answerDocumentStageData struct {
	FinalAnswer string `json:"final_answer"`
}

// ParseOutput reads the final AnswerDocument from Mutable, renders
// it to prose, and packages the result into a StageOutput. On a
// zero/missing document, emits a fail-loud warning prefixed to the
// raw last LLM content.
func (e *answerDocumentEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	out := &StageOutput{}

	// B5 落地 — V2 carrier branch. Once emit_answer_document has
	// persisted a block-only answer, AnswerDocumentV2 is non-nil on
	// Mutable (mutual exclusion guarantees AnswerDocument is nil in
	// that case). Route to the V2 renderer + V2 hedging instead of
	// the V1 path. The two paths NEVER run on the same dispatch.
	if ctx != nil && ctx.Mutable != nil {
		if docV2 := ctx.Mutable.AnswerDocumentV2(); docV2 != nil {
			return e.parseOutputV2(ctx, docV2, out)
		}
	}

	// B8-T4 (block_only_carrier.md §5.8): V1 carrier code path
	// removed. emit_answer_document V1 schema was rejected at
	// runtime since B8-T3, so reaching this point with
	// AnswerDocumentV2() == nil means the LLM produced no emit at
	// all. Synthesise the same fallback warning + authority caveat
	// the prior path used — the LLM's last assistant message is the
	// only prose available.
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}
	warning := "· answer_document emission missing — the answer-rendering pass could not " +
		"produce a structured answer. The text below is the raw LLM response; no " +
		"schema-level validation ran on it."
	safeFallback := sanitizePriorDraftForSummary(lastContent)
	if safeFallback == "" {
		safeFallback = strings.TrimSpace(lastContent)
	}
	combined := warning
	if ctx != nil && ctx.Mutable != nil {
		caveat := render.SynthesiseAuthorityCaveatFor(answerDocumentAuthorityEvidencePool(ctx), e.language)
		if caveat != "" {
			caveat = render.StripAuthorityArtifactsForRender(caveat)
			combined = warning + "\n\n*" + caveat + "*"
		}
	}
	if safeFallback != "" {
		combined = combined + "\n\n" + safeFallback
	}
	logging.Warning("[finalizer/answer_document] emit_answer_document missing after retries; falling back to raw content (len=%d)",
		len(lastContent))
	out.Data = marshalStageData(answerDocumentStageData{FinalAnswer: combined})
	out.FinalAnswer = combined
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
// stripSystemHedgeArtifactsFromDoc removes hedge markers + Authority
// caveat tokens from every LLM-authored prose field on the doc
// BEFORE ApplyAuthorityHedging re-injects them. Idempotent; runs once
// per ParseOutput.
func sanitizePriorDraftForSummary(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	s = priorDraftThinkBlockRe.ReplaceAllString(s, "")
	s = priorDraftToolCallRe.ReplaceAllString(s, "")
	s = priorDraftInvokeRe.ReplaceAllString(s, "")
	s = priorDraftParameterRe.ReplaceAllString(s, "")
	s = priorDraftJSONFenceRe.ReplaceAllString(s, "")
	// Strip system-injected hedge markers + Authority caveat from the
	// prior draft before salvaging into Summary. Without this, the LLM
	// sees its own prior assistant content carrying [hedged] /
	// [historical] / [illustrative] tokens it never wrote and the
	// Authority: caveat line — and on retry may "edit" them as if
	// they were its own prose, either deleting them (gate fires) or
	// expanding them into long hedge paragraphs (richness regresses).
	// The renderer will re-inject the markers on each emit; the LLM
	// sees a clean draft as its working surface.
	s = render.StripAuthorityArtifacts(s)
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

// parseOutputV2 is the V2 carrier branch of ParseOutput (B5 落地).
// It runs the V2-specific hedging + render pipeline and returns the
// StageOutput. Mirror of the V1 ParseOutput tail; uses
// ApplyAuthorityHedgingV2 + RenderAnswerDocumentV2 so V2 docs never
// touch V1's shape-specific helpers.
//
// V2 stage data is intentionally narrower than V1's:
// answerDocumentStageData carries V1-shaped FinalAnswer + AnswerDocument
// fields; V2 stores nil in the AnswerDocument slot and surfaces the
// V2 doc via stageData's V2 sibling field (added at B5-T3 if absent).
//
// To avoid expanding the agent struct shape during B5 we keep
// answerDocumentStageData V1-shaped and route V2 prose through
// FinalAnswer (the only field downstream consumers read). The
// mutable already holds the typed AnswerDocumentV2 for any
// reviewer / oracle that needs the structured data.
func (e *answerDocumentEvaluator) parseOutputV2(ctx *types.AgentContext, docV2 *types.AnswerDocumentV2, out *StageOutput) (*StageOutput, error) {
	if docV2 == nil {
		// Defensive: caller checked already, but make sure.
		return out, nil
	}
	// Apply V2 hedging in-place on the defensive copy
	// (ctx.Mutable.AnswerDocumentV2() returns a clone).
	if ctx != nil {
		render.ApplyAuthorityHedging(docV2, answerDocumentAuthorityEvidencePool(ctx), e.language)
	}
	prose := render.RenderAnswerDocument(docV2, e.language)
	out.Data = marshalStageData(answerDocumentStageData{
		FinalAnswer: prose,
	})
	out.FinalAnswer = prose
	return out, nil
}
