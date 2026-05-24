package agent

// extractor.go — Turn B (extractor) evaluator.
//
// The extractor is the second half of the two-turn explorer split.
// It runs AFTER Turn A (the explorer) has completed its
// investigation and handed off a frozen TurnAArtifacts snapshot, and
// BEFORE the finalizer synthesizes the user-visible answer. Its job
// is to convert Turn A's raw transcript (investigation notes, tool
// results, deterministic evidence) into STRUCTURED emit_* tool calls:
//
//   - emit_answer_symbol     — the answer-symbol slate with a
//                               required set-level completeness claim
//   - emit_hypothesis_verdict — per-hypothesis status + citation
//
// The extractor calls read_file / grep / repo_map NOT AT ALL. The
// LLM is explicitly forbidden from them at the prompt layer and at
// the tool-schema layer (ToolSuggestions allowlist). Every fact it
// emits must trace back to Turn A's snapshot.
//
// The cardinality validator closes the completeness-claim loophole for
// precise typed floors: explicit requested boundaries and analyzer-
// resolved sub-topic counts. Investigation candidate counts and
// AnswerContract.MustInclude stay soft because they can include proof-
// chain context. The schema-level check still rejects malformed calls;
// this is the semantic second layer.
//
// ShouldStop is deliberately one-shot (iteration >= 1). Turn B
// cannot read new files, so a retry has no new information to work
// with — downgrading to lower_bound is the honest terminal state.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/analysis/axis"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/analysis/subject"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Extractor prompt display caps. Named constants so the limits are
// grep-visible and changeable in one place.
const (
	extractorMaxNotes                       = 6    // max investigation note entries shown in prompt
	extractorMaxNoteChars                   = 1200 // max chars per note before truncation
	extractorMaxEvidence                    = 24   // max ranked evidence items shown
	extractorMinValueFacts                  = 12   // minimum value-bearing evidence facts shown when present
	extractorDefaultValueFacts              = 16   // default value-bearing evidence facts shown
	extractorMaxValueFacts                  = 32   // hard cap for value-bearing evidence facts shown
	extractorMaxEvidenceSummary             = 200  // max summary chars per evidence item
	extractorMaxFlowFindings                = 10   // max dataflow findings shown
	extractorMaxSourceInventoryAdvisoryRows = 40
	// Display-only cap for analyzer soft guidance names. The exact
	// unique count remains visible; the list is capped because analyzer
	// MustInclude can contain broad search candidates, while accepted
	// aggregate_facts.member_set is the authoritative typed answer set.
	extractorMaxSoftGuidanceNames = 32
)

// extractorEvaluator is the Turn B evaluator. It is a separate type
// from explorerEvaluator so the two turns cannot accidentally share
// state. Implements LoopController so the mid-loop path can stop
// immediately after the one-shot emit_* batch executes, instead of
// burning an extra LLM round that ShouldStop(iteration >= 1) would
// catch anyway.
type extractorEvaluator struct {
	// retriesUsed tracks soft-stop correction rounds, bounded by
	// maxRetries. Mirrors the finalizer pattern.
	retriesUsed int
	// maxRetries caps correction rounds. Set from
	// AgentSettings.ExtractorMaxCorrectionRetries at construction.
	maxRetries int

	// Iteration caps populated from AgentSettings at construction —
	// the static floor used when no per-dispatch override is present.
	// See types.AgentSettings.ExtractorSoftIterCap / ExtractorHardIterCap.
	defaultSoftIterCap int
	defaultHardIterCap int

	// Per-dispatch caps captured in BuildInitialInstruction from
	// ctx.ExtractorSoftIterCapOverride. Zero = "no override seen,
	// fall back to the defaults." Mirrors plannerEvaluator's
	// dispatch-override pattern so multi-topic explanations get
	// enough soft-cap headroom to emit one Key-Anchor row per
	// sub-topic without bumping into the 3-iter static default.
	dispatchSoftIterCap int
	dispatchHardIterCap int
}

// BuildInitialInstruction implements Evaluator.
//
// Scope: the DYNAMIC per-dispatch Turn A digest only. All STATIC
// contract content — role, allowed/forbidden tool list, completeness
// honesty contract, output format, prohibitions — lives in the
// extract-skill declared in internal/skill/defaults.go and is
// rendered into the LLM prompt as system sections by
// context/builder.go before this evaluator-specific instruction runs.
//
// Rationale for keeping the static contract in the skill config
// rather than baked into this string builder: (a) the skill system
// exists for exactly this, (b) the stable preamble would otherwise
// inflate every dispatch, (c) a top-level declarative Config entry
// is one grep away whereas a buried `strings.Builder.WriteString`
// call is not. Moving it to the skill also lets
// BaseAgent.buildToolSchemas scope the LLM tool set from
// ToolSuggestions without any runtime append.
//
// The prompt below therefore carries ONLY what changes per dispatch:
// the user question, the Turn A transcript digest (investigation
// notes, read files, top evidence, flow findings), cardinality
// guidance (soft investigation counts plus hard typed floors), and the
// hypothesis set. Graceful degrade on nil TurnAArtifacts is preserved.
func (e *extractorEvaluator) BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string {
	e.retriesUsed = 0
	if ctx != nil && ctx.ExtractorSoftIterCapOverride > 0 {
		e.dispatchSoftIterCap = ctx.ExtractorSoftIterCapOverride
		recoverySlack := e.defaultHardIterCap - e.defaultSoftIterCap
		if recoverySlack < 1 {
			recoverySlack = 1
		}
		e.dispatchHardIterCap = e.dispatchSoftIterCap + recoverySlack
	} else {
		e.dispatchSoftIterCap = 0
		e.dispatchHardIterCap = 0
	}
	var b strings.Builder

	// User question is already rendered by builder.go as "User Request"
	// section — no need to repeat it here.

	// -------- Turn A transcript digest --------
	ta := (*types.TurnAArtifacts)(nil)
	if ctx != nil && ctx.Mutable != nil {
		ta = ctx.Mutable.TurnAArtifacts()
	}
	if ta == nil {
		b.WriteString("## Investigation transcript\n\n")
		b.WriteString("**No transcript available** — the investigation did not produce a snapshot for this ")
		b.WriteString("dispatch. This is an unusual path (unit-test bootstrap or wiring bug). ")
		b.WriteString("Produce whatever `emit_*` calls you can justify from the user question ")
		b.WriteString("alone, and set `completeness` to `unknown` for any answer-symbol emission.\n\n")
	} else {
		b.WriteString("## Investigation transcript digest\n\n")
		supportScope := extractorTranscriptSupportScope(ctx)

		if closure := renderExtractorAcceptedClosure(ctx, ta); closure != "" {
			b.WriteString(closure)
		}
		if boundary := renderExtractorValidationBoundaryNotes(ta); boundary != "" {
			b.WriteString(boundary)
		}
		if advisory := renderExtractorSourceInventoryAdvisory(ta); advisory != "" {
			b.WriteString(advisory)
		}

		// Investigation notes: up to 6 entries, trimmed for prompt length
		if len(ta.InvestigationNotes) > 0 {
			b.WriteString("### Investigation notes (per-iteration narrative)\n\n")
			maxNotes := len(ta.InvestigationNotes)
			if maxNotes > extractorMaxNotes {
				fmt.Fprintf(&b, "*(showing the %d most recent of %d iterations)*\n\n", extractorMaxNotes, maxNotes)
				ta.InvestigationNotes = ta.InvestigationNotes[maxNotes-extractorMaxNotes:]
			}
			for i, note := range ta.InvestigationNotes {
				trimmed := strings.TrimSpace(note)
				if trimmed == "" {
					continue
				}
				if len(trimmed) > extractorMaxNoteChars {
					trimmed = trimmed[:extractorMaxNoteChars] + "…"
				}
				fmt.Fprintf(&b, "**Iter %d:**\n%s\n\n", i+1, trimmed)
			}
		}

		// Files read: authoritative list for citation grounding
		if len(ta.ReadFiles) > 0 {
			b.WriteString("### Files the investigation read (authoritative citation source)\n\n")
			b.WriteString("You MUST cite only these files in emit_* calls. Any other path is a ")
			b.WriteString("hallucination and will be rejected as ungrounded.\n\n")
			for _, f := range ta.ReadFiles {
				fmt.Fprintf(&b, "- `%s`\n", f)
			}
			b.WriteString("\n")
		}

		// Deterministic evidence: top 24 ranked items
		evidenceItems := extractorTranscriptEvidenceItems(ta.EvidenceItems, supportScope)
		if len(evidenceItems) > 0 {
			b.WriteString("### Deterministic evidence the investigation extracted\n\n")
			evMax := len(evidenceItems)
			if evMax > extractorMaxEvidence {
				fmt.Fprintf(&b, "*(showing top %d of %d ranked items)*\n\n", extractorMaxEvidence, evMax)
				evMax = extractorMaxEvidence
			}
			for i := 0; i < evMax; i++ {
				ev := evidenceItems[i]
				// Turn B strict: DisplayLocation(true) strips
				// LineStart for Recovered/Ungrounded items so the
				// extractor LLM cannot pick a line the finalizer's
				// stricter grounder will later reject.
				cite := ""
				if loc := ev.DisplayLocation(true); loc != "" {
					cite = " @ " + loc
				}
				summary := strings.TrimSpace(types.EvidenceDeterministicSurfaceText(ev, false))
				if summary == "" {
					parts := []string{ev.Subject, ev.Predicate, ev.Object}
					summary = strings.TrimSpace(strings.Join(parts, " "))
				}
				if len(summary) > extractorMaxEvidenceSummary {
					summary = summary[:extractorMaxEvidenceSummary] + "…"
				}
				tag := ""
				if ev.GroundingStatus == types.GroundingRecovered {
					tag = " [recovered — read_file before citing]"
				}
				fmt.Fprintf(&b, "- [%s] %s%s%s\n", ev.Kind, summary, cite, tag)
			}
			b.WriteString("\n")
		}

		taForValueLens := ta
		if supportScope != nil {
			copy := *ta
			copy.EvidenceItems = evidenceItems
			taForValueLens = &copy
		}
		if lens := renderExtractorValueEvidenceFacts(ctx, taForValueLens); lens != "" {
			b.WriteString(lens)
		}

		// Flow findings: top 10 source→sink chains
		flowFindings := extractorTranscriptFlowFindings(ta.FlowFindings, supportScope)
		if len(flowFindings) > 0 {
			b.WriteString("### Dataflow findings (source → sink chains)\n\n")
			ffMax := len(flowFindings)
			if ffMax > extractorMaxFlowFindings {
				fmt.Fprintf(&b, "*(showing top %d of %d)*\n\n", extractorMaxFlowFindings, ffMax)
				ffMax = extractorMaxFlowFindings
			}
			for i := 0; i < ffMax; i++ {
				ff := flowFindings[i]
				fmt.Fprintf(&b, "- `%s` (confidence=%.2f)\n", strings.Join(ff.Path, " → "), ff.Confidence)
			}
			b.WriteString("\n")
		}

		if needsAnswerSymbols(ctx) {
			// Cardinality guidance is split into:
			//   - hard typed floors: explicit requested boundaries or
			//     analyzer-resolved sub-topic counts, both precise enough
			//     to reject a complete claim; and
			//   - soft context counts: Turn A terminal candidates and
			//     analyzer MustInclude hints, both useful reminders but
			//     too noisy to force mechanism helpers into the answer
			//     slate.
			b.WriteString("### Cardinality guidance (for completeness claim)\n\n")
			fmt.Fprintf(&b, "- **Investigation terminal-evidence count:** %d\n", ta.TerminalEvidenceCount)
			if ctx != nil && ctx.AnalysisIR != nil {
				must := extractorSoftGuidanceNames(ctx.AnalysisIR.AnswerContract.MustInclude)
				fmt.Fprintf(&b, "- **Analyzer required-name count:** %d name(s)", len(must))
				if len(must) > 0 {
					if extractorHasAcceptedMemberSet(ta) {
						b.WriteString(" — names omitted because accepted `aggregate_facts.member_set` already carries the authoritative principal set")
					} else {
						fmt.Fprintf(&b, " — %s", extractorSoftGuidanceNamePreview(must))
					}
				}
				b.WriteString("\n")
				hardFloor := requiredAnswerSymbolCount(ctx)
				if boundary := requestedEnumerationBoundary(ctx); boundary != nil && viewNeedsBoundedPrincipalList(ctx) {
					fmt.Fprintf(&b, "- **Hard typed floor:** %d item(s), from requested boundary `%s`\n", boundary.DeclaredCount, boundary.SourceQuote)
				} else if requiresMultiTopicAnchorSkeleton(ctx) {
					fmt.Fprintf(&b, "- **Hard typed floor:** %d anchor(s), from analyzer-resolved sub-topic count\n", hardFloor)
				} else {
					b.WriteString("- **Hard typed floor:** none. Treat terminal-evidence and required-name counts as soft search/completeness reminders only; do not add helpers, tools, files, or mechanism components to `items[]` merely to satisfy those counts.\n")
				}
				if hardFloor > 0 {
					fmt.Fprintf(&b, "\nIf you claim `complete`, your `emit_answer_symbol` batch MUST have ≥ %d item(s) because this floor is typed and precise. ", hardFloor)
					b.WriteString("If you cannot reach that floor, emit what you have and choose `lower_bound`.\n")
				} else {
					b.WriteString("\nNo hard typed floor — your claim will be trusted as-is when the slate directly matches the user's requested entity type.\n")
				}
				// Plan E (2026-05-02) — surface CompletenessObligation +
				// Buckets to the extractor prompt so the LLM knows up
				// front that lower_bound is forbidden under exhaustive
				// demands, and that symbols must be distributable across
				// user-named buckets.
				if ctx.AnalysisIR != nil {
					rm := ctx.AnalysisIR.RequestModel
					if rm.CompletenessObligation.IsActive() {
						fmt.Fprintf(&b, "- **Exhaustive demand:** the user asked for every match (`%s` in the question). `completeness=lower_bound` is REJECTED — use `complete` when you have grounded all matches, or `unknown` when the investigation could not determine the full set.\n",
							rm.CompletenessObligation.SourceQuote)
					}
					if types.HasAttributeBearingEnumeration(rm) {
						b.WriteString("- **Two-axis enumeration:** the question asks for a principal member set plus a related per-member attribute. Apply the `completeness` claim to the principal members only. Do not downgrade the principal member slate to `unknown` solely because some attributes are unresolved; put grounded attribute facts in each item's `rationale`, and mark missing attributes as unresolved in the rationale / downstream caveat.\n")
					}
					if len(rm.Buckets) >= 2 {
						labels := make([]string, 0, len(rm.Buckets))
						for _, bk := range rm.Buckets {
							labels = append(labels, fmt.Sprintf("`%s`", bk.Label))
						}
						fmt.Fprintf(&b, "- **User-named partition:** the user split the answer into %d named groups: %s. Each symbol's `rationale` should name which bucket it belongs to so downstream rendering can section the slate.\n",
							len(rm.Buckets), strings.Join(labels, ", "))
					}
				}
			}
			b.WriteString("\n")
		} else {
			b.WriteString("### Structured emit obligations\n\n")
			b.WriteString("This dispatch does NOT require `emit_answer_symbol`. The principal answer will be rendered downstream from typed ordered_list / diagram / prose support lanes, so do not manufacture a symbol slate just because the user asked for a call chain, mechanism walk, architecture explanation, relation-edge evidence enumeration, or non-symbol evidence enumeration. Only an explicit Anchor skeleton block, a Requested Set Boundary, or a symbol-backed enumeration without a principal relation/evidence lane activates `emit_answer_symbol` as a hard obligation; analyzer sub-topics alone are guidance.\n\n")
		}
	}

	// -------- Multi-topic explanation skeleton guide --------
	// Render the per-dispatch sub-topic list ONLY when the analyzer
	// resolved shape=explanation AND populated sub_topics. It is a
	// hard emit_answer_symbol obligation only when IRRequiresAnchorSkeleton
	// is true. For architecture/mechanism narratives the same list is
	// optional context; analyzer decomposition drift must not become
	// final-answer principal content.
	if isMultiTopicExplanation(ctx) {
		st := ctx.AnalysisIR.RequestModel.SubTopics
		plan := extractorAnswerSurfacePlan(ctx)
		if needsAnswerSymbols(ctx) && requiresMultiTopicAnchorSkeleton(ctx) {
			b.WriteString("## Anchor skeleton (one per sub-topic)\n\n")
			fmt.Fprintf(&b, "The analyzer identified %d independently-answerable sub-topic(s). ", len(st))
			b.WriteString("For each, call emit_answer_symbol with ONE anchor symbol — the load-bearing ")
			b.WriteString("identifier that the multi-paragraph answer summary will hang on. Each ")
			b.WriteString("anchor needs a concrete file:line from the 'Files the investigation read' list above; ")
			b.WriteString("use the rationale field to name the sub-topic the anchor covers.\n\n")
		} else {
			b.WriteString("## Optional sub-topic structure\n\n")
			fmt.Fprintf(&b, "The analyzer identified %d sub-topic hint(s). These are guidance only for this answer family: do not call `emit_answer_symbol` just to mirror them. Use the grounded evidence/support lanes and final prose/sections to preserve the user's requested structure; if a sub-topic is unsupported or out of scope, leave it out or disclose the boundary instead of promoting a nearby helper as a principal anchor.\n\n", len(st))
		}
		for i, topic := range st {
			summary := strings.TrimSpace(topic.Summary)
			if summary == "" {
				summary = "(no summary)"
			}
			fmt.Fprintf(&b, "%d. %s", i+1, summary)
			if len(topic.Entities) > 0 {
				fmt.Fprintf(&b, " — entities: %s", strings.Join(topic.Entities, ", "))
			}
			b.WriteString("\n")
		}
		if plan != nil && len(plan.ExplanationAnchorBackbone) > 0 {
			b.WriteString("\nGrounded anchor candidates already compiled from the current investigation (reuse these exact file:line anchors instead of inventing a fresh skeleton when they match the sub-topics):\n")
			for i, anchor := range plan.ExplanationAnchorBackbone {
				label := strings.TrimSpace(anchor.Rationale)
				if label == "" {
					label = fmt.Sprintf("anchor %d", i+1)
				}
				desc := strings.TrimSpace(types.RenderStepSurfaceAnchorDescription(anchor))
				if desc == "" {
					desc = strings.TrimSpace(anchor.Name)
				}
				if anchor.File != "" && anchor.Line > 0 {
					fmt.Fprintf(&b, "- %s: %s @ %s:%d", label, desc, anchor.File, anchor.Line)
				} else {
					fmt.Fprintf(&b, "- %s: %s", label, desc)
				}
				b.WriteString("\n")
			}
			if len(plan.ExplanationAnchorMissingTopics) > 0 {
				b.WriteString("If any sub-topic is still missing from that compiled anchor set, do not invent coverage — it means the investigation closed before that topic got a grounded owner/definition line.\n")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// -------- Bounded principal member slate --------
	if viewNeedsBoundedPrincipalList(ctx) {
		boundary := requestedEnumerationBoundary(ctx)
		count := boundary.DeclaredCount
		plan := extractorAnswerSurfacePlan(ctx)
		b.WriteString("## Principal member slate\n\n")
		fmt.Fprintf(&b, "The user explicitly declared a bounded principal set: `%s` (%d item(s)). Emit `emit_answer_symbol` with the principal member slate for that bounded set before the answer-rendering step writes the step list.\n\n",
			boundary.SourceQuote, count)
		b.WriteString("Rules for this slate:\n")
		fmt.Fprintf(&b, "- Keep `items[]` within %d principal member(s).\n", count)
		b.WriteString("- Choose the members that answer the bounded set itself, not every adjacent helper, guard, compatibility shim, or side condition that appears nearby in the same owner flow.\n")
		if ctx != nil && ctx.AnalysisIR != nil && types.HasAttributeBearingEnumeration(ctx.AnalysisIR.RequestModel) {
			b.WriteString("- This is an attribute-bearing enumeration: each `items[]` entry should be the principal member itself. Put the per-member attribute in `rationale` with its own grounded file:line when available; if the attribute is not grounded for that member, say so in `rationale` instead of dropping the member.\n")
			b.WriteString("- `complete` means the principal member set is complete. Attribute gaps do not make the member set incomplete; they become caveat/rationale text for downstream rendering.\n")
		}
		b.WriteString("- If the owner flow contains extra caveat-only items beyond the bounded set, leave them out of the main slate and let downstream prose mention them only as follow-on context.\n")
		b.WriteString("- Use `complete` only when you can name the full bounded set; otherwise use `lower_bound`.\n\n")
		if plan != nil && len(plan.StepBackbone) > 0 {
			fmt.Fprintf(&b, "Ordered grounded candidates already compiled from the current investigation (the pool may be wider than %d items — select the principal bounded set from it):\n", count)
			for i, anchor := range plan.StepBackbone {
				desc := strings.TrimSpace(types.RenderStepSurfaceAnchorDescription(anchor))
				if anchor.File != "" && anchor.Line > 0 {
					fmt.Fprintf(&b, "%d. `%s` @ %s:%d", i+1, anchor.Name, anchor.File, anchor.Line)
				} else {
					fmt.Fprintf(&b, "%d. `%s`", i+1, anchor.Name)
				}
				if desc != "" {
					fmt.Fprintf(&b, " — %s", desc)
				}
				b.WriteString("\n")
			}
			b.WriteString("\nPrefer these compiled candidates over inventing a fresh slate from prose memory.\n\n")
		}
	}

	// -------- Hypothesis set --------
	if ctx != nil && ctx.AnalysisIR != nil && len(ctx.AnalysisIR.HypothesisSet) > 0 {
		b.WriteString("## Hypotheses (emit a verdict for each)\n\n")
		for _, h := range ctx.AnalysisIR.HypothesisSet {
			fmt.Fprintf(&b, "- **%s** (%s): %s\n", h.ID, h.Status, strings.TrimSpace(h.Statement))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func extractorTranscriptSupportScope(ctx *types.AgentContext) *supportLaneScope {
	scope := supportLaneScopeForContext(ctx, true)
	if scope == nil || !scope.constrain {
		return nil
	}
	return scope
}

func extractorTranscriptEvidenceItems(items []types.EvidenceItem, supportScope *supportLaneScope) []types.EvidenceItem {
	if len(items) == 0 || supportScope == nil {
		return items
	}
	filtered := make([]types.EvidenceItem, 0, len(items))
	for _, item := range items {
		if supportScope.allowsEvidence(item) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func extractorTranscriptFlowFindings(findings []types.FlowFindingDigest, supportScope *supportLaneScope) []types.FlowFindingDigest {
	if len(findings) == 0 || supportScope == nil {
		return findings
	}
	filtered := make([]types.FlowFindingDigest, 0, len(findings))
	for _, ff := range findings {
		if supportScope.allowsFlowFinding(ff) {
			filtered = append(filtered, ff)
		}
	}
	return filtered
}

func extractorSoftGuidanceNames(names []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

func extractorSoftGuidanceNamePreview(names []string) string {
	if len(names) == 0 {
		return ""
	}
	limit := extractorMaxSoftGuidanceNames
	if len(names) < limit {
		limit = len(names)
	}
	preview := strings.Join(names[:limit], ", ")
	if len(names) > limit {
		preview += fmt.Sprintf(", … (+%d more)", len(names)-limit)
	}
	return preview
}

func extractorHasAcceptedMemberSet(ta *types.TurnAArtifacts) bool {
	if ta == nil {
		return false
	}
	for _, fact := range ta.AcceptedAggregateFacts {
		if fact.Kind == types.AnswerAggregateMemberSet && len(fact.Members) > 0 {
			return true
		}
	}
	return false
}

func renderExtractorAcceptedClosure(ctx *types.AgentContext, ta *types.TurnAArtifacts) string {
	reason := ""
	resultKind := ""
	var aggregateFacts []types.AnswerAggregateFact
	if ta != nil {
		reason = strings.TrimSpace(ta.AcceptedClosureReason)
		resultKind = strings.TrimSpace(ta.AcceptedResultKind)
		aggregateFacts = ta.AcceptedAggregateFacts
	}
	if reason == "" && ctx != nil && ctx.Mutable != nil {
		reason = strings.TrimSpace(ctx.Mutable.StableInvestigationCompleteReason())
	}
	if resultKind == "" && ctx != nil && ctx.Mutable != nil {
		resultKind = strings.TrimSpace(ctx.Mutable.StableInvestigationResultKind())
	}
	if len(aggregateFacts) == 0 && ctx != nil && ctx.Mutable != nil {
		aggregateFacts = ctx.Mutable.StableInvestigationAggregateFacts()
	}
	if reason == "" && resultKind == "" && len(aggregateFacts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Accepted exploration closure\n\n")
	if resultKind != "" {
		fmt.Fprintf(&b, "- result_kind: `%s`\n", resultKind)
	}
	if reason != "" {
		if suppressUnstructuredClosureReasonForPrincipalMemberSets(ctx, aggregateFacts) {
			reason = sanitizeAggregateExcludedCandidatesForPrompt(ctx, reason, aggregateFacts)
			fmt.Fprintf(&b, "- model-authored closure set-level summary (advisory only; typed `aggregate_facts.member_set` rows/counts below remain the authoritative member carrier if any number or member identity conflicts): %s\n", truncateExtractorPromptText(reason, 700))
		} else {
			reason = sanitizeAggregateExcludedCandidatesForPrompt(ctx, reason, aggregateFacts)
			fmt.Fprintf(&b, "- model-authored closure reason: %s\n", truncateExtractorPromptText(reason, 900))
		}
	}
	if len(aggregateFacts) > 0 {
		b.WriteString("- structured aggregate facts:\n")
		b.WriteString(renderStructuredAggregateFactsForContext(ctx, aggregateFacts))
	}
	if principal := renderExtractorPrincipalAggregateSlate(ctx, aggregateFacts); principal != "" {
		b.WriteString(principal)
	}
	b.WriteString("- Treat this as the investigator's structured handoff, not as a citation. Preserve resolved counts, listed members, excluded categories/counts, and scope boundaries when they are supported by the evidence/tool outputs below; do not fall back to earlier stale notes that conflict with this accepted closure.\n\n")
	return b.String()
}

func suppressUnstructuredClosureReasonForPrincipalMemberSets(ctx *types.AgentContext, facts []types.AnswerAggregateFact) bool {
	if len(facts) == 0 || ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent != types.IntentEnumerate && !rm.Predicates.IsCategoryEnumeration {
		return false
	}
	if rm.Predicates.IsHistoryLookup {
		return false
	}
	return len(structuredAggregatePrincipalMemberSetRefs(ctx, facts)) > 0
}

func renderExtractorPrincipalAggregateSlate(ctx *types.AgentContext, facts []types.AnswerAggregateFact) string {
	if len(facts) == 0 {
		return ""
	}
	refs := structuredAggregatePrincipalMemberSetRefs(ctx, facts)
	if len(refs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("- principal aggregate member_set obligations:\n")
	for _, ref := range refs {
		fact := ref.Fact
		if fact.Kind != types.AnswerAggregateMemberSet || len(fact.Members) == 0 {
			continue
		}
		label := strings.TrimSpace(fact.Label)
		if label == "" {
			label = "principal member set"
		}
		fmt.Fprintf(&b, "  - %s: %d member(s)", label, len(fact.Members))
		if strings.TrimSpace(fact.Value) != "" {
			fmt.Fprintf(&b, ", declared value=%s", fact.Value)
		}
		if needsAnswerSymbols(ctx) {
			b.WriteString("; copy every member below into the answer-symbol slate because `emit_answer_symbol` is active for this dispatch. Do not downgrade to `lower_bound` merely because earlier transcript prose was truncated; this typed aggregate fact is the accepted closure payload.\n")
		} else {
			b.WriteString("; this dispatch does NOT require `emit_answer_symbol`. Use the members below as the downstream principal member contract; do not re-emit them as an answer-symbol slate.\n")
		}
		for memberIdx, member := range fact.Members {
			member = strings.TrimSpace(member)
			if member == "" {
				continue
			}
			if line := renderExtractorPrincipalAggregateMemberLine(member, supportRefAt(fact.SupportRefs, memberIdx), supportRefAt(fact.MemberNotes, memberIdx)); line != "" {
				fmt.Fprintf(&b, "    - %s\n", line)
			}
		}
	}
	return b.String()
}

func supportRefAt(refs []string, idx int) string {
	if idx < 0 || idx >= len(refs) {
		return ""
	}
	return strings.TrimSpace(refs[idx])
}

func renderExtractorPrincipalAggregateMemberLine(member string, supportRef string, note string) string {
	member = strings.TrimSpace(member)
	if member == "" {
		return ""
	}
	note = strings.Join(strings.Fields(strings.TrimSpace(note)), " ")
	withNote := func(base string) string {
		if note == "" {
			return base
		}
		return base + " — note: " + note
	}
	if label, loc, ok := types.ParseAnswerSupportRefMemberLocation(member); ok {
		label = strings.TrimSpace(label)
		if label == "" {
			label = member
		}
		if loc.File != "" && loc.LineStart > 0 {
			return withNote(fmt.Sprintf("`%s` @ %s:%d", label, loc.File, loc.LineStart))
		}
		return withNote(fmt.Sprintf("`%s`", label))
	}
	if _, loc, ok := types.ParseAnswerSupportRefMemberLocation(supportRef); ok && loc.File != "" && loc.LineStart > 0 {
		return withNote(fmt.Sprintf("`%s` @ %s:%d", member, loc.File, loc.LineStart))
	}
	if supportRef != "" {
		return withNote(fmt.Sprintf("`%s` — support_ref: `%s`", member, supportRef))
	}
	return withNote(fmt.Sprintf("`%s`", member))
}

func renderExtractorValidationBoundaryNotes(ta *types.TurnAArtifacts) string {
	if ta == nil || len(ta.ValidationBoundaryNotes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Accepted closure validation boundaries\n\n")
	b.WriteString("These are scheduler-owned structural validation results from after the accepted exploration closure. They are not citations and do not override evidence. Preserve user-visible implications as answer boundaries or caveats instead of reopening investigation or inventing facts.\n")
	for i, note := range ta.ValidationBoundaryNotes {
		trimmed := strings.TrimSpace(note)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > extractorMaxNoteChars {
			trimmed = trimmed[:extractorMaxNoteChars] + "…"
		}
		fmt.Fprintf(&b, "- boundary %d: %s\n", i+1, trimmed)
	}
	b.WriteString("\n")
	return b.String()
}

func renderExtractorSourceInventoryAdvisory(ta *types.TurnAArtifacts) string {
	if ta == nil {
		return ""
	}
	advisory := ta.SourceInventoryAdvisory
	observation := ta.SourceInventoryObservation
	if !observation.IsActive() && advisory.IsActive() {
		observation = types.SourceInventoryObservationFromAdvisory(advisory)
	}
	if !advisory.IsActive() && !observation.IsActive() {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Source inventory advisory candidates\n\n")
	b.WriteString("- The system compiled these candidates from typed request lanes, requested scopes, and the repo-map graph. They are structured context, not final answer text and not citations by themselves.\n")
	if advisory.AdvisoryOnly {
		b.WriteString("- `advisory_only=true`: use these rows only when they match the answer shape you are emitting; disclose ambiguity instead of treating the set as mandatory.\n")
	} else {
		b.WriteString("- The active `source_inventory_profile` made this inventory shape high-confidence; preserve matching rows when the accepted aggregate facts or evidence support them.\n")
	}
	if len(advisory.Scopes) > 0 {
		fmt.Fprintf(&b, "- scopes: `%s`\n", strings.Join(advisory.Scopes, "`, `"))
	}
	if len(advisory.Provenance) > 0 {
		fmt.Fprintf(&b, "- provenance: `%s`\n", strings.Join(advisory.Provenance, "`, `"))
	}
	if observation.IsActive() {
		b.WriteString("- repo-lens observation invariants: ")
		var parts []string
		for _, set := range observation.Sets {
			if len(set.Members) == 0 {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s count=%d len(members)=%d complete=%t",
				types.SourceInventoryAdvisoryRoleLabel(set.Role), set.Count, len(set.Members), set.Complete))
		}
		if len(parts) > 0 {
			b.WriteString(strings.Join(parts, "; "))
			b.WriteByte('\n')
		} else {
			b.WriteString("none\n")
		}
	}
	total := 0
	for _, set := range advisory.Sets {
		total += len(set.Candidates)
	}
	emitted := 0
	for _, set := range advisory.Sets {
		if len(set.Candidates) == 0 || emitted >= extractorMaxSourceInventoryAdvisoryRows {
			continue
		}
		fmt.Fprintf(&b, "- role `%s` %s candidates (complete=%t):\n",
			set.Role, types.SourceInventoryAdvisoryRoleLabel(set.Role), set.Complete)
		for _, candidate := range set.Candidates {
			if emitted >= extractorMaxSourceInventoryAdvisoryRows {
				break
			}
			line := renderExtractorSourceInventoryAdvisoryCandidate(candidate)
			if line == "" {
				continue
			}
			fmt.Fprintf(&b, "  - %s\n", line)
			emitted++
		}
	}
	if total > emitted {
		fmt.Fprintf(&b, "- showing %d of %d candidate(s); keep the accepted aggregate facts/evidence as the authority if a hidden row matters.\n", emitted, total)
	}
	b.WriteString("\n")
	return b.String()
}

func renderExtractorSourceInventoryAdvisoryCandidate(candidate types.SourceInventoryAdvisoryCandidate) string {
	member := strings.TrimSpace(candidate.Member)
	if member == "" {
		return ""
	}
	parts := []string{fmt.Sprintf("`%s`", member)}
	if candidate.File != "" && candidate.Line > 0 {
		parts = append(parts, fmt.Sprintf("@ %s:%d", candidate.File, candidate.Line))
	}
	if candidate.Language != "" {
		parts = append(parts, "language="+candidate.Language)
	}
	if candidate.SupportRef != "" {
		parts = append(parts, "support_ref=`"+candidate.SupportRef+"`")
	}
	if note := truncateExtractorPromptText(candidate.Note, 180); note != "" {
		parts = append(parts, "note: "+note)
	}
	if attrs := renderExtractorSourceInventoryAdvisoryAttributes(candidate.Attributes); attrs != "" {
		parts = append(parts, attrs)
	}
	return strings.Join(parts, " — ")
}

func renderExtractorSourceInventoryAdvisoryAttributes(attrs []types.SourceInventoryAdvisoryAttribute) string {
	if len(attrs) == 0 {
		return ""
	}
	const max = 4
	var parts []string
	for _, attr := range attrs {
		if len(parts) >= max {
			break
		}
		member := strings.TrimSpace(attr.Member)
		if member == "" {
			continue
		}
		loc := strings.TrimSpace(attr.File)
		if loc != "" && attr.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, attr.Line)
		}
		role := strings.TrimSpace(string(attr.Role))
		if role == "" {
			role = "candidate"
		}
		if loc != "" {
			parts = append(parts, fmt.Sprintf("%s `%s` @ %s", role, member, loc))
		} else {
			parts = append(parts, fmt.Sprintf("%s `%s`", role, member))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	suffix := ""
	if len(attrs) > len(parts) {
		suffix = fmt.Sprintf(" (+%d more)", len(attrs)-len(parts))
	}
	return "related_candidate_attributes: " + strings.Join(parts, "; ") + suffix
}

type extractorValueFact struct {
	item    types.EvidenceItem
	label   string
	surface string
	score   int
	index   int
}

type extractorValueRankProfile string

const (
	extractorValueRankGeneric     extractorValueRankProfile = "generic"
	extractorValueRankScalar      extractorValueRankProfile = "scalar_value"
	extractorValueRankConfig      extractorValueRankProfile = "config_value"
	extractorValueRankTrace       extractorValueRankProfile = "trace"
	extractorValueRankDiagnostic  extractorValueRankProfile = "diagnostic"
	extractorValueRankComparison  extractorValueRankProfile = "comparison"
	extractorValueRankEnumeration extractorValueRankProfile = "enumeration"
)

func renderExtractorValueEvidenceFacts(ctx *types.AgentContext, ta *types.TurnAArtifacts) string {
	if ta == nil || len(ta.EvidenceItems) == 0 {
		return ""
	}
	needles := extractorValueEvidenceNeedles(ctx, ta)
	profile := extractorValueEvidenceRankProfileFor(ctx)
	limit := extractorValueEvidenceDisplayLimit(ctx, ta, profile)
	candidates := make([]extractorValueFact, 0, len(ta.EvidenceItems))
	hasRelevant := false
	for i, item := range ta.EvidenceItems {
		if !extractorValueEvidenceCandidate(item) {
			continue
		}
		surface := strings.TrimSpace(types.EvidenceDeterministicSurfaceText(item, false))
		if surface == "" {
			continue
		}
		label := strings.TrimSpace(extractorFirstNonEmptyString(item.Subject, item.AnchorSymbol, item.OwnerSymbol, item.Object))
		if label == "" {
			label = strings.TrimSpace(string(item.AnchorKind))
		}
		if label == "" {
			label = strings.TrimSpace(string(item.Kind))
		}
		score := extractorValueEvidenceScore(item, label, surface, needles, profile)
		if score > 0 {
			hasRelevant = true
		}
		candidates = append(candidates, extractorValueFact{
			item:    item,
			label:   label,
			surface: surface,
			score:   score,
			index:   i,
		})
	}
	if len(candidates) == 0 {
		return ""
	}
	if hasRelevant {
		filtered := candidates[:0]
		for _, candidate := range candidates {
			if candidate.score > 0 || candidate.item.SalienceLockedForScoring() {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].item.SalienceLockedForScoring() != candidates[j].item.SalienceLockedForScoring() {
			return candidates[i].item.SalienceLockedForScoring()
		}
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].index < candidates[j].index
	})

	var b strings.Builder
	for i, candidate := range candidates {
		if i == 0 {
			b.WriteString("### Value-bearing evidence facts\n\n")
			b.WriteString("These rows are a typed lens over evidence the investigator already emitted. Rows that overlap the request, sub-topics, aggregate facts, or answer contract are shown first, then weighted by the typed question profile (intent/scenario/answer-subject/axis/question-structure); this is soft visibility guidance, not a hard gate. Preserve exact constants, defaults, assignment targets, return literals, and config values from this section when they answer the user's requested dimensions; do not invent values that are not present here or in the evidence list.\n\n")
		}
		loc := candidate.item.DisplayLocation(true)
		label := sanitizeAggregateExcludedCandidatesForPrompt(ctx, candidate.label, ta.AcceptedAggregateFacts)
		surface := sanitizeAggregateExcludedCandidatesForPrompt(ctx, candidate.surface, ta.AcceptedAggregateFacts)
		fmt.Fprintf(&b, "- `%s`", label)
		if loc != "" {
			fmt.Fprintf(&b, " @ %s", loc)
		}
		fmt.Fprintf(&b, ": %s\n", surface)
		if i+1 >= limit {
			break
		}
	}
	b.WriteString("\n")
	return b.String()
}

func extractorValueEvidenceDisplayLimit(ctx *types.AgentContext, ta *types.TurnAArtifacts, profile extractorValueRankProfile) int {
	limit := extractorDefaultValueFacts
	if ctx != nil && ctx.AnalysisIR != nil {
		rm := ctx.AnalysisIR.RequestModel
		if len(rm.SubTopics) >= 2 {
			limit += extractorMinInt(len(rm.SubTopics)*3, 8)
		}
		if buckets := rm.QuestionStructure().Buckets; len(buckets) >= 2 {
			limit += extractorMinInt(len(buckets)*3, 8)
		}
		if rm.Complexity == types.ComplexityComplex {
			limit += 4
		}
		if rm.Predicates.IsCrossComponent || rm.Predicates.IsCategoryEnumeration || rm.Predicates.IsRelationalLookup {
			limit += 4
		}
	}
	if ta != nil {
		if len(ta.AcceptedAggregateFacts) > 0 {
			limit += extractorMinInt(len(ta.AcceptedAggregateFacts)*2, 6)
		}
		limit = widenEvidenceLimitForSalience(limit, extractorMinValueFacts, extractorMaxValueFacts, ta.EvidenceItems, "extractor")
	}
	switch profile {
	case extractorValueRankComparison, extractorValueRankEnumeration, extractorValueRankDiagnostic:
		limit += 4
	case extractorValueRankScalar:
		limit -= 2
	}
	if limit < extractorMinValueFacts {
		return extractorMinValueFacts
	}
	if limit > extractorMaxValueFacts {
		return extractorMaxValueFacts
	}
	return limit
}

func extractorMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func extractorMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func countLoadBearingOrExhaust(items []types.EvidenceItem) int {
	count := 0
	for _, item := range items {
		if item.SalienceLockedForScoring() {
			count++
		}
	}
	return count
}

func widenEvidenceLimitForSalience(limit, minLimit, maxLimit int, items []types.EvidenceItem, site string) int {
	locked := countLoadBearingOrExhaust(items)
	if locked > 0 {
		const contextReserve = 4
		limit = extractorMaxInt(limit, locked+contextReserve)
		if locked > maxLimit {
			logging.Warning("[%s] salience-locked evidence rows exceed display cap: locked=%d cap=%d", site, locked, maxLimit)
		}
	}
	if limit < minLimit {
		return minLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func extractorValueEvidenceRankProfileFor(ctx *types.AgentContext) extractorValueRankProfile {
	if ctx == nil || ctx.AnalysisIR == nil {
		return extractorValueRankGeneric
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.DiagnosticProfile.RequiresDiagnosticRootCause() || rm.Predicates.IsDiagnosticQuestion || rm.Intent == types.IntentRootCause || rm.Scenario == types.ScenarioRootCause {
		return extractorValueRankDiagnostic
	}
	if rm.Intent == types.IntentTrace || rm.PredicateAxis == types.AxisCall {
		return extractorValueRankTrace
	}
	if rm.Intent == types.IntentConfigQuery || rm.Scenario == types.ScenarioConfigTrace || rm.AnswerSubject.Kind == types.SubjectConfigKey || rm.PredicateAxis == types.AxisConfigure {
		return extractorValueRankConfig
	}
	if rm.Intent == types.IntentReturnValue ||
		rm.AnswerSubject.Kind == types.SubjectReturnValue ||
		rm.AnswerSubject.Kind == types.SubjectNumeric ||
		rm.AnswerSubject.Kind == types.SubjectStringLiteral ||
		rm.AnswerSubject.Kind == types.SubjectEnumValue ||
		rm.PredicateAxis == types.AxisReturn {
		return extractorValueRankScalar
	}
	view := rm.QuestionStructure()
	if view.ResolvedDeclaredCount() > 0 || view.CompletenessObligation.IsActive() || rm.Predicates.IsCountQuestion || rm.Predicates.IsCategoryEnumeration || rm.Intent == types.IntentEnumerate {
		return extractorValueRankEnumeration
	}
	if len(view.Buckets) >= 2 || len(rm.SubTopics) >= 2 || (rm.Intent == types.IntentExplain && rm.Scenario == types.ScenarioArchitectureExplain) {
		return extractorValueRankComparison
	}
	return extractorValueRankGeneric
}

func extractorValueEvidenceNeedles(ctx *types.AgentContext, ta *types.TurnAArtifacts) map[string]bool {
	needles := map[string]bool{}
	add := func(text string) {
		for _, tok := range extractorTokenSet(text) {
			if len(tok) >= 2 {
				needles[tok] = true
			}
		}
	}
	if ctx != nil {
		add(ctx.Objective)
		if ctx.AnalysisIR != nil {
			rm := ctx.AnalysisIR.RequestModel
			add(rm.RawRequest)
			add(string(rm.AnswerSubject.Kind))
			add(string(rm.PredicateAxis))
			if view := rm.QuestionStructure(); len(view.Buckets) > 0 {
				for _, bucket := range view.Buckets {
					add(bucket.Label)
					for _, anchor := range bucket.Anchors {
						add(anchor)
					}
				}
			}
			for _, sub := range rm.SubTopics {
				add(sub.Summary)
				for _, entity := range sub.Entities {
					add(entity)
				}
				for _, scope := range sub.Scopes {
					add(scope)
				}
			}
			hints := rm.AnalyzerHints
			for _, values := range [][]string{
				hints.Keywords,
				hints.Entities,
				hints.PrimaryEntities,
				hints.MentionedEntities,
				hints.DerivedEntities,
				hints.ExactTargets,
				ctx.AnalysisIR.AnswerContract.MustInclude,
			} {
				for _, value := range values {
					add(value)
				}
			}
		}
	}
	if ta != nil {
		add(ta.UserQuestion)
		add(ta.AcceptedClosureReason)
		for _, fact := range ta.AcceptedAggregateFacts {
			add(fact.Label)
			add(fact.Value)
			for _, dim := range fact.Dimensions {
				add(dim.Name)
				add(dim.Value)
			}
			for _, member := range fact.Members {
				add(member)
			}
			for _, excluded := range fact.Excluded {
				add(excluded)
			}
			for _, ref := range fact.SupportRefs {
				add(ref)
			}
		}
	}
	return needles
}

func extractorValueEvidenceScore(item types.EvidenceItem, label, surface string, needles map[string]bool, profile extractorValueRankProfile) int {
	if len(needles) == 0 {
		return 0
	}
	score := 0
	seen := map[string]bool{}
	for _, text := range []string{
		label,
		surface,
		item.Subject,
		item.Predicate,
		item.Object,
		item.AnchorSymbol,
		item.OwnerSymbol,
		item.Source,
		item.Summary,
		item.Snippet,
	} {
		for _, tok := range extractorTokenSet(text) {
			if needles[tok] && !seen[tok] {
				seen[tok] = true
				score += 6
			}
		}
	}
	if score == 0 && !item.SalienceLockedForScoring() {
		return 0
	}
	score += extractorValueEvidenceKindBoost(item.Kind, profile)
	score += extractorValueEvidenceAnchorBoost(item.AnchorKind, profile)
	score += extractorValueEvidenceSalienceBoost(item.Salience, profile)
	if item.LoadBearingSummary {
		score += extractorValueEvidenceLoadBearingBoost(profile)
	}
	return score
}

func extractorValueEvidenceKindBoost(kind types.EvidenceKind, profile extractorValueRankProfile) int {
	switch kind {
	case types.EvidenceConcrete:
		switch profile {
		case extractorValueRankScalar, extractorValueRankConfig, extractorValueRankEnumeration:
			return 16
		case extractorValueRankDiagnostic:
			return 14
		default:
			return 12
		}
	case types.EvidenceDirect:
		switch profile {
		case extractorValueRankComparison:
			return 10
		case extractorValueRankTrace, extractorValueRankDiagnostic:
			return 8
		default:
			return 6
		}
	default:
		return 0
	}
}

func extractorValueEvidenceAnchorBoost(kind types.AnchorKind, profile extractorValueRankProfile) int {
	switch profile {
	case extractorValueRankScalar:
		switch kind {
		case types.AnchorReturn:
			return 22
		case types.AnchorAssignment, types.AnchorInitializer:
			return 16
		case types.AnchorDefinition:
			return 4
		}
	case extractorValueRankConfig:
		switch kind {
		case types.AnchorAssignment, types.AnchorInitializer:
			return 20
		case types.AnchorDefinition:
			return 10
		case types.AnchorReturn:
			return 6
		}
	case extractorValueRankTrace:
		switch kind {
		case types.AnchorCall, types.AnchorReturn, types.AnchorCondition:
			return 16
		case types.AnchorAssignment, types.AnchorInitializer:
			return 10
		case types.AnchorDefinition:
			return 4
		}
	case extractorValueRankDiagnostic:
		switch kind {
		case types.AnchorCondition, types.AnchorReturn, types.AnchorAssignment, types.AnchorInitializer:
			return 16
		case types.AnchorCall:
			return 10
		case types.AnchorDefinition:
			return 6
		}
	case extractorValueRankComparison:
		switch kind {
		case types.AnchorDefinition:
			return 14
		case types.AnchorAssignment, types.AnchorInitializer:
			return 12
		case types.AnchorReturn:
			return 6
		}
	case extractorValueRankEnumeration:
		switch kind {
		case types.AnchorDefinition, types.AnchorInitializer, types.AnchorAssignment:
			return 12
		case types.AnchorReturn:
			return 8
		}
	default:
		switch kind {
		case types.AnchorAssignment, types.AnchorInitializer, types.AnchorReturn:
			return 10
		case types.AnchorDefinition:
			return 6
		}
	}
	return 0
}

func extractorValueEvidenceLoadBearingBoost(profile extractorValueRankProfile) int {
	switch profile {
	case extractorValueRankTrace, extractorValueRankDiagnostic:
		return 14
	default:
		return 10
	}
}

func extractorValueEvidenceSalienceBoost(salience types.EvidenceSalience, profile extractorValueRankProfile) int {
	switch salience {
	case types.SalienceLoadBearing, types.SalienceExhaustListed:
		switch profile {
		case extractorValueRankEnumeration, extractorValueRankDiagnostic, extractorValueRankTrace:
			return 8
		default:
			return 6
		}
	case types.SalienceSupporting:
		return 3
	default:
		return 0
	}
}

func extractorTokenSet(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(tok string) {
		tok = strings.Trim(strings.ToLower(tok), "_")
		if len(tok) < 2 || extractorLowSignalToken(tok) || seen[tok] {
			return
		}
		seen[tok] = true
		out = append(out, tok)
	}
	var b strings.Builder
	flush := func() {
		word := b.String()
		b.Reset()
		if word == "" {
			return
		}
		add(word)
		for _, piece := range extractorIdentifierPieces(word) {
			add(piece)
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func extractorIdentifierPieces(word string) []string {
	runes := []rune(strings.Trim(word, "_"))
	if len(runes) == 0 {
		return nil
	}
	var pieces []string
	start := 0
	flush := func(end int) {
		if end <= start {
			return
		}
		piece := string(runes[start:end])
		for _, sub := range strings.FieldsFunc(piece, func(r rune) bool { return r == '_' }) {
			if sub != "" {
				pieces = append(pieces, sub)
			}
		}
		start = end
	}
	for i := 1; i < len(runes); i++ {
		prev := runes[i-1]
		cur := runes[i]
		nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
		if cur == '_' {
			flush(i)
			start = i + 1
			continue
		}
		if unicode.IsUpper(cur) && (unicode.IsLower(prev) || unicode.IsDigit(prev) || nextLower) {
			flush(i)
			continue
		}
		if unicode.IsDigit(cur) != unicode.IsDigit(prev) && (unicode.IsDigit(cur) || unicode.IsDigit(prev)) {
			flush(i)
		}
	}
	flush(len(runes))
	return pieces
}

func extractorLowSignalToken(tok string) bool {
	switch tok {
	case "a", "an", "and", "are", "as", "at", "be", "been", "being",
		"by", "can", "could", "did", "do", "does", "for", "from", "go",
		"had", "has", "have", "how", "in", "into", "is", "it", "its",
		"of", "on", "or", "should", "that", "the", "their", "these",
		"this", "those", "to", "was", "were", "what", "when", "where",
		"which", "who", "why", "with", "would":
		return true
	case "answer", "answers", "block", "blocks", "candidate", "candidates",
		"claim", "claims", "code", "context", "count", "counts", "detail",
		"details", "fact", "facts", "file", "files", "line", "lines",
		"list", "lists", "member", "members", "node", "nodes", "question",
		"request", "result", "results", "section", "sections", "summary",
		"value", "values":
		return true
	default:
		return false
	}
}

func extractorValueEvidenceCandidate(item types.EvidenceItem) bool {
	if item.Source == "" || item.LineStart <= 0 {
		return false
	}
	switch item.Kind {
	case types.EvidenceConcrete:
		return true
	}
	switch item.AnchorKind {
	case types.AnchorAssignment, types.AnchorInitializer, types.AnchorReturn:
		return true
	case types.AnchorDefinition:
		return snippetLooksValueBearing(item.Snippet)
	}
	if item.LoadBearingSummary {
		return true
	}
	return snippetLooksValueBearing(item.Snippet)
}

func snippetLooksValueBearing(snippet string) bool {
	snippet = strings.TrimSpace(snippet)
	if snippet == "" || len(snippet) > 220 {
		return false
	}
	if !strings.ContainsAny(snippet, "=:") {
		return false
	}
	for _, r := range snippet {
		if unicode.IsDigit(r) || r == '"' || r == '\'' || r == '`' {
			return true
		}
	}
	lower := strings.ToLower(snippet)
	return strings.Contains(lower, "true") ||
		strings.Contains(lower, "false") ||
		strings.Contains(lower, "nil") ||
		strings.Contains(lower, "null")
}

func extractorFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncateExtractorPromptText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	trimmed := s[:max]
	for len(trimmed) > 0 && !utf8.RuneStart(trimmed[len(trimmed)-1]) {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return strings.TrimSpace(trimmed) + "…[truncated]"
}

// ShouldStop implements Evaluator.
//
// Turn B's happy path is parallel-batch at iter=0: both emit_answer_symbol
// and emit_hypothesis_verdict fire in the same assistant response, the
// mid-loop observer accepts the batch, and the dispatch terminates
// after one LLM call. When the LLM does NOT batch in parallel (drops
// one of the expected emits), the mid-loop keeps the loop running so
// iter=1 can pick up the missing one; a soft-stop correction at iter=1
// adds one more retry if the LLM stopped without emitting. iter >= 3
// caps this at: iter=0 partial emit, iter=1 soft-stop+correction,
// iter=2 LLM emits the missing tool, iter=3 break. Parallel-batch
// case still terminates in one call.
//
// Two-stage cap: at the soft cap (3), spare 2 extra iters when the
// LLM is retrying one of the structured emits — answer-symbol slates
// can run kilobytes when many candidates are surfaced, and a streaming
// truncation on iter=2 would otherwise be discarded by the flat cap.
func (e *extractorEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	soft, hard := e.dispatchSoftIterCap, e.dispatchHardIterCap
	if soft <= 0 || hard <= soft {
		soft, hard = e.defaultSoftIterCap, e.defaultHardIterCap
	}
	return iterationCapShouldStop(resp, iteration,
		soft, hard,
		emitAnswerSymbolToolName, emitHypothesisVerdictToolName)
}

const (
	emitAnswerSymbolToolName      = "emit_answer_symbol"
	emitHypothesisVerdictToolName = "emit_hypothesis_verdict"
)

// ParseOutput implements Evaluator. The extractor's two unique
// responsibilities drain here:
//
//  1. Answer-symbol slate + cardinality validator: extractor emits
//     a slate with a completeness claim; validateCompletenessClaim
//     cross-checks the claim against Turn A's TerminalEvidenceCount
//     + AnalysisIR MustInclude floor and downgrades a dishonest
//     "complete" to "lower_bound".
//
//  2. Hypothesis verdicts: drained by the orchestrator's
//     post-dispatch hook (drainHypothesisVerdicts), not here,
//     because MarkHypothesis needs to write through the IR and the
//     extractor's StageOutput has no IR pointer. The buffer stays
//     populated for the hook to read.
//
// Evidence is intentionally NOT drained in the extractor — that is
// Turn A's exclusive channel. The extract-skill forbids
// emit_evidence and the orchestrator already merged Turn A's
// EvidenceItems into BusContext by the time this runs.
func (e *extractorEvaluator) ParseOutput(ctx *types.AgentContext, _ []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	out := &StageOutput{
		Data: json.RawMessage(`{}`),
	}
	if ctx == nil || ctx.Mutable == nil {
		return out, nil
	}

	// R4 fail-loud gate. When Turn A read ZERO files AND produced ZERO
	// key evidence items (direct / registration / mechanism — the three
	// kinds that carry a concrete cross-file join the finalizer needs),
	// the investigation is structurally empty: the LLM stopped before
	// it did any real work. Emitting a synthesized answer at this
	// point would be silent low-quality output. Fail loud via
	// StageOutput.Error so the orchestrator's MaxRetriesPerStage
	// budget forces a re-dispatch. ctx.EvidenceItems is the merged
	// view visible at Turn B (LLM-emit + deterministic + Turn A
	// artifacts); keyEvidenceCount counts the LLM-emittable kinds
	// that name a mechanism, not prose observations.
	if e.extractorInvestigationEmpty(ctx) {
		out.Error = "extractor gate: the investigation stage produced 0 files read and 0 key evidence (direct/registration/mechanism) — investigation is structurally empty"
		logging.Warning("[extractor] R4 fail-loud: %s", out.Error)
		return out, nil
	}

	// Answer-symbol drain + cardinality validator.
	syms, claim := ctx.Mutable.EmittedAnswerSymbols()
	definitionFallback := extractorPrincipalDefinitionSymbolFallback(ctx)
	literalFallback := extractorDeclarativeLiteralFallback(ctx)
	fallback := mergeAnswerSymbolFallbacks(definitionFallback, literalFallback)
	if len(syms) == 0 && len(fallback) > 0 {
		syms = fallback
		claim = types.CompletenessLowerBound
		logging.Info("[extractor] synthesized %d answer symbol(s) from grounded structured evidence fallback", len(syms))
	} else {
		if refined := trimDeclarativeSlateToTerminals(syms, literalFallback); len(refined) > 0 && len(refined) < len(syms) {
			logging.Info("[extractor] pruned answer-symbol slate from %d to %d item(s) using declarative terminal filter", len(syms), len(refined))
			syms = refined
			if claim == types.CompletenessUnknown {
				claim = types.CompletenessLowerBound
			}
		}
	}
	if len(syms) > 0 {
		validatedClaim := validateCompletenessClaim(ctx, syms, claim)
		out.AnswerSymbols = syms
		out.AnswerSymbolCompleteness = validatedClaim
	}

	// Criterion-based hypothesis auto-verdict injection. For every
	// hypothesis with a RequiredEvidence list, evaluate the list
	// against the current env; if all criteria pass AND the LLM did
	// not emit a verdict, inject an "inconclusive" entry so the
	// drain hook downstream still records progress. For
	// FalsificationCondition: if it is satisfied, inject a rejected
	// verdict (or override an existing LLM verdict to rejected).
	if ctx.AnalysisIR != nil && len(ctx.AnalysisIR.HypothesisSet) > 0 && !extractorObservationOnlyRuntimeArtifact(ctx) {
		var taToolResults []types.ToolResult
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			taToolResults = ta.ToolResults
		}
		env := criterion.Env{
			IR:            ctx.AnalysisIR,
			Evidence:      ctx.EvidenceItems,
			ToolResults:   taToolResults,
			AnswerSymbols: out.AnswerSymbols,
			PrescanBlob:   ctx.Mutable.PrescanSummaryBlob(),
		}
		existing := ctx.Mutable.EmittedHypothesisVerdicts()
		byID := make(map[string]bool, len(existing))
		for _, v := range existing {
			byID[v.HypothesisID] = true
		}
		var injected []types.HypothesisVerdict
		for _, h := range ctx.AnalysisIR.HypothesisSet {
			fals := criterion.Eval(h.FalsificationCondition, env)
			if fals.Satisfied {
				if byID[h.ID] {
					// Override: later drain hook will read these injected
					// verdicts AFTER the LLM-emitted ones; since the drain
					// writes each verdict into the IR via MarkHypothesis,
					// a later call wins. We always emit the override.
					logging.Warning("[extractor] falsification satisfied for %s: forcing rejected", h.ID)
				}
				injected = append(injected, types.HypothesisVerdict{
					HypothesisID: h.ID,
					Status:       types.HypRejected,
					Rationale:    "falsification condition satisfied: " + fals.Detail,
				})
				continue
			}
			if byID[h.ID] {
				continue
			}
			okReq, _ := criterion.EvalAll(h.RequiredEvidence, env)
			if okReq && len(h.RequiredEvidence) > 0 {
				injected = append(injected, types.HypothesisVerdict{
					HypothesisID: h.ID,
					Status:       types.HypInconclusive,
					Rationale:    "required evidence satisfied but no LLM verdict emitted",
				})
			}
		}
		if len(injected) > 0 {
			ctx.Mutable.AppendEmittedHypothesisVerdicts(injected)
			logging.Info("[extractor] injected %d auto-verdict(s) from criterion evaluation", len(injected))
		}
	}

	return out, nil
}

func extractorObservationOnlyRuntimeArtifact(ctx *types.AgentContext) bool {
	return ctx != nil &&
		ctx.AnalysisIR != nil &&
		ctx.AnalysisIR.RequestModel.HasObservationOnlyRuntimeArtifact()
}

// validateCompletenessClaim is the hard cardinality validator for the
// extractor's answer-symbol slate.
//
// Only precise typed floors are allowed to downgrade a model's
// "complete" claim:
//   - RequestedEnumerationBoundary, when the user explicitly declared
//     the bounded principal set size.
//   - Multi-topic explanation sub-topic count, where the analyzer
//     resolved one anchor obligation per topic.
//
// Turn A TerminalEvidenceCount and analyzer MustInclude remain useful
// prompt guidance, but they are noisy: terminal candidates often
// include proof-chain helpers, and MustInclude can contain tools,
// files, or mechanism components. They must not hard-force the model
// to pad the slate with non-answer members.
//
// Claims other than "complete" are passed through unchanged. A
// complete claim with no hard typed floor is trusted after the normal
// emit_answer_symbol schema and axis gates have accepted the slate.
func validateCompletenessClaim(ctx *types.AgentContext, syms []types.AnswerSymbol, claim types.CompletenessClaim) types.CompletenessClaim {
	if claim != types.CompletenessComplete {
		return claim
	}

	termCount := 0
	mustInclude := 0
	if ctx != nil && ctx.Mutable != nil {
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			termCount = ta.TerminalEvidenceCount
		}
	}
	if ctx != nil && ctx.AnalysisIR != nil {
		mustInclude = len(ctx.AnalysisIR.AnswerContract.MustInclude)
	}
	hardFloor := requiredAnswerSymbolCount(ctx)
	if hardFloor <= 0 {
		logging.Debug("[extractor] completeness=complete passed through: no hard typed cardinality floor (termCount=%d mustInclude=%d)",
			termCount, mustInclude)
		return claim
	}

	if len(syms) >= hardFloor {
		logging.Debug("[extractor] completeness=complete cleared hard cardinality gate: %d items ≥ hardFloor %d (termCount=%d mustInclude=%d)",
			len(syms), hardFloor, termCount, mustInclude)
		return claim
	}

	logging.Warning("[extractor] completeness=complete DOWNGRADED to lower_bound: %d items < hard typed floor %d (termCount=%d mustInclude=%d). The slate is preserved as a floor; the finalizer will use the softened prompt.",
		len(syms), hardFloor, termCount, mustInclude)
	// R8: surface the silent downgrade on the AnalyzerDecision channel
	// so the end-of-Run operator summary catches it.
	if ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.AppendAnalyzerDecision(types.AnalyzerDecisionSignal{
			Kind:   "completeness_downgraded",
			Stage:  string(types.StageExtract),
			Before: string(types.CompletenessComplete),
			After:  string(types.CompletenessLowerBound),
			Reason: fmt.Sprintf("%d items < hard typed floor %d", len(syms), hardFloor),
			Detail: fmt.Sprintf("termCount=%d mustInclude=%d", termCount, mustInclude),
		})
	}
	return types.CompletenessLowerBound
}

func extractorPrincipalDefinitionSymbolFallback(ctx *types.AgentContext) []types.AnswerSymbol {
	if ctx == nil || !needsAnswerSymbols(ctx) || !viewNeedsEnumerationSlate(ctx) {
		return nil
	}
	plan := extractorAnswerSurfacePlan(ctx)
	if plan == nil {
		return nil
	}
	items := types.PrincipalSupportEvidenceItemsForFamily(types.QFEnumeration, plan)
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(items))
	out := make([]types.AnswerSymbol, 0, len(items))
	for _, ev := range items {
		if types.ClaimFormOf(ev) != types.ClaimDefinitionFact {
			continue
		}
		if !ev.IsCitable() || !ev.Scope.IsLineShaped() || ev.LineStart <= 0 {
			continue
		}
		name := extractorDefinitionSymbolName(ev)
		source := extractorEvidenceRepoSource(ctx, ev.Source)
		if name == "" || source == "" {
			continue
		}
		sym := types.AnswerSymbol{
			Name:      name,
			File:      source,
			Line:      ev.LineStart,
			Kind:      extractorDefinitionAnswerSymbolKind(ctx),
			Chain:     strings.TrimSpace(ev.Summary),
			Rationale: "principal definition extracted from grounded structured evidence",
		}
		key := answerSymbolDedupKey(sym)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, sym)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func extractorDefinitionSymbolName(ev types.EvidenceItem) string {
	for _, raw := range []string{ev.AnchorSymbol, ev.Subject, ev.Object} {
		if name := normalizeExtractorDefinitionSymbol(raw); name != "" {
			return name
		}
	}
	for _, raw := range ev.SurfaceTerms {
		if name := normalizeExtractorDefinitionSymbol(raw); name != "" {
			return name
		}
	}
	return ""
}

func normalizeExtractorDefinitionSymbol(raw string) string {
	name := normalizeExtractorFallbackToken(raw)
	if name == "" {
		return ""
	}
	if strings.ContainsAny(name, " \t\r\n{}()[];,") {
		return ""
	}
	return name
}

func extractorEvidenceRepoSource(ctx *types.AgentContext, source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	if ctx != nil {
		source = ground.CanonicalAgentPath(ctx, source)
	} else {
		source = canonicalExplorerPath(source)
	}
	if source == "" || strings.HasPrefix(source, ".codrax/") {
		return ""
	}
	return source
}

func extractorDefinitionAnswerSymbolKind(ctx *types.AgentContext) types.AnswerSymbolKind {
	switch extractorAnswerSubjectKind(ctx) {
	case types.SubjectFunctionName:
		return types.KindFunction
	case types.SubjectInterface:
		return types.KindInterface
	case types.SubjectStructField:
		return types.KindField
	case types.SubjectEnumValue:
		return types.KindConst
	case types.SubjectTypeName, types.SubjectGeneric, types.SubjectUnknown:
		return types.KindType
	default:
		return types.KindType
	}
}

func mergeAnswerSymbolFallbacks(groups ...[]types.AnswerSymbol) []types.AnswerSymbol {
	var out []types.AnswerSymbol
	seen := make(map[string]bool)
	for _, group := range groups {
		for _, sym := range group {
			if strings.TrimSpace(sym.Name) == "" {
				continue
			}
			key := answerSymbolDedupKey(sym)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, sym)
		}
	}
	return out
}

func extractorDeclarativeLiteralFallback(ctx *types.AgentContext) []types.AnswerSymbol {
	if !declarativeLiteralFallbackRelevant(ctx) {
		return nil
	}
	pool := mergeEvidenceForAxisCheck(ctx)
	if len(pool) == 0 {
		return nil
	}
	subj := extractorAnswerSubjectKind(ctx)
	seen := make(map[string]bool, len(pool))
	out := make([]types.AnswerSymbol, 0, len(pool))
	add := func(raw string, quoted bool, source string, line int, chain, rationale string) {
		name := normalizeExtractorFallbackToken(raw)
		if name == "" || source == "" || line <= 0 {
			return
		}
		if !extractorFallbackTokenMatchesSubject(name, quoted, subj) {
			return
		}
		key := name + "\x1f" + source + "\x1f" + fmt.Sprintf("%d", line)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, types.AnswerSymbol{
			Name:      name,
			File:      source,
			Line:      line,
			Kind:      types.KindLiteral,
			Chain:     strings.TrimSpace(chain),
			Rationale: rationale,
		})
	}

	for _, ev := range pool {
		// Schema-level scopes (File / Crossfile / Negative) anchor
		// layer identity / cross-file contracts / absences — they
		// have no per-line literal to extract. Only line-shaped
		// scopes (Line / LineRange / Section) feed symbol synthesis
		// candidates. Skipping them here prevents spurious file:line
		// literals from getting attributed to schema-level evidence.
		//
		// This fallback consumes only structured evidence fields the
		// investigator already emitted. It deliberately does not scan
		// raw read_file text near the evidence line; if a visible
		// literal is load-bearing, the explorer must carry it through
		// Object / Summary / surface_terms on emit_evidence so the
		// downstream answer is model-authored rather than system-filled.
		if !ev.Scope.IsLineShaped() {
			continue
		}
		if ev.Kind == types.EvidenceDataflowPath && ev.Predicate == "resolution_chain" {
			add(subject.ChainTerminalToken(ev.Summary), false, ev.Source, ev.LineStart, ev.Summary, "terminal literal inferred from resolution chain")
		}
		for _, lit := range extractQuotedLiterals(ev.Summary) {
			add(lit, true, ev.Source, ev.LineStart, ev.Summary, "literal extracted from grounded evidence")
		}
		for _, lit := range extractQuotedLiterals(ev.Object) {
			add(lit, true, ev.Source, ev.LineStart, ev.Object, "literal extracted from grounded evidence")
		}
		if ev.Kind == types.EvidenceRegistration {
			add(extractorSingleTokenCandidate(ev.Object), false, ev.Source, ev.LineStart, ev.Object, "terminal literal extracted from registration evidence")
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// answerSymbolDedupKey identifies an AnswerSymbol for merge / trim
// dedup. Kind drives the granularity:
//
//   - KindLiteral: Name alone. Literal-kind symbols carry the answer as
//     their Name ("explore-skill", "DEBUG", config keys, route strings);
//     same-Name different-line extractions are the same fact quoted
//     from adjacent grounded rows, and merging collapses duplicates
//     into a single answer entry. Matches the semantic the registration
//     / enumeration-of-literals cases rely on
//     Fallback synthesis still relies on this invariant when the model
//     emitted no slate. A model-authored slate is no longer augmented
//     from fallback literals; downstream should retry or disclose the
//     model's completeness claim instead of system-padding answers.
//
//   - symbol kinds (method / function / type / ...): (Name, File, Line).
//     Name alone collapsed cross-file same-name methods (Run on
//     explorerEvaluator / subExplorerEvaluator / extractorEvaluator —
//     four distinct facts sharing the Name "Run"), silently dropping
//     fallback-synthesized answers for the enumeration questions where
//     the slate is supposed to grow. The tri-field key mirrors the
//     (name, source, line) schema the internal fallback synthesiser
//     already uses at extractor.go:517.
func answerSymbolDedupKey(sym types.AnswerSymbol) string {
	if sym.Kind == types.KindLiteral {
		return sym.Name
	}
	return sym.Name + "\x1f" + sym.File + "\x1f" + strconv.Itoa(sym.Line)
}

func mergeDeclarativeFallbackSymbols(current, fallback []types.AnswerSymbol) []types.AnswerSymbol {
	if len(current) == 0 || len(fallback) == 0 {
		return current
	}
	seen := make(map[string]bool, len(current))
	out := append([]types.AnswerSymbol(nil), current...)
	for _, sym := range current {
		if sym.Name != "" {
			seen[answerSymbolDedupKey(sym)] = true
		}
	}
	for _, sym := range fallback {
		if sym.Name == "" {
			continue
		}
		key := answerSymbolDedupKey(sym)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, sym)
	}
	return out
}

func trimDeclarativeSlateToTerminals(current, fallback []types.AnswerSymbol) []types.AnswerSymbol {
	if len(current) == 0 || len(fallback) == 0 {
		return current
	}
	allow := make(map[string]bool, len(fallback))
	for _, sym := range fallback {
		allow[answerSymbolDedupKey(sym)] = true
	}
	out := make([]types.AnswerSymbol, 0, len(current))
	for _, sym := range current {
		if sym.Kind == types.KindLiteral || allow[answerSymbolDedupKey(sym)] {
			out = append(out, sym)
		}
	}
	if len(out) == 0 {
		return current
	}
	return out
}

func declarativeLiteralFallbackRelevant(ctx *types.AgentContext) bool {
	if ctx == nil {
		return false
	}
	requiresSymbolSlate := needsAnswerSymbols(ctx)
	if !requiresSymbolSlate {
		return false
	}
	axis := types.AxisUnknown
	if ctx.AnalysisIR != nil {
		axis = ctx.AnalysisIR.RequestModel.PredicateAxis
	}
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil && rm.PredicateAxis != types.AxisUnknown {
			axis = rm.PredicateAxis
		}
	}
	isEnumeration := viewNeedsEnumerationSlate(ctx) || enumerationIntentForContext(ctx)
	// L1 (2026-05-10) — recompute the structural registration-shape
	// gate on the extractor side. The explorer's cache is per-evaluator
	// state and doesn't ride the bus, so recompute lazily using the
	// same helper. Fail-open when graph is unavailable.
	primaryRegShape := true
	if ctx != nil && ctx.Mutable != nil {
		if g, ok := ctx.Mutable.SearchGraph().(*repomap.Graph); ok && g != nil {
			primaryRegShape = primaryEntitiesLookLikeRegistration(
				ctx.AnalysisIR, g,
				declarativeAllowedKinds(irQuestionKind(ctx), axis),
			)
		}
	}
	if !declarativeFocusRelevant(irQuestionKind(ctx), isEnumeration, axis, primaryRegShape) {
		return false
	}
	kind := extractorAnswerSubjectKind(ctx)
	if extractorAnswerSubjectSupportsLiteralFallback(kind) {
		return true
	}
	return extractorGenericDeclarativeLiteralFallbackAllowed(ctx, kind)
}

func extractorAnswerSubjectKind(ctx *types.AgentContext) types.AnswerSubjectKind {
	if ctx != nil && ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.AnswerSubject.Kind != "" {
		return ctx.AnalysisIR.RequestModel.AnswerSubject.Kind
	}
	if ctx != nil && ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			return rm.AnswerSubject.Kind
		}
	}
	return types.SubjectUnknown
}

func extractorAnswerSubjectSupportsLiteralFallback(kind types.AnswerSubjectKind) bool {
	switch kind {
	case types.SubjectStringLiteral,
		types.SubjectHandlerRoute,
		types.SubjectConfigKey,
		types.SubjectFilePath,
		types.SubjectReturnValue:
		return true
	}
	return false
}

func extractorGenericDeclarativeLiteralFallbackAllowed(ctx *types.AgentContext, kind types.AnswerSubjectKind) bool {
	switch kind {
	case types.SubjectUnknown, types.SubjectGeneric:
		return viewNeedsEnumerationSlate(ctx)
	}
	return false
}

func extractorFallbackTokenMatchesSubject(token string, quoted bool, kind types.AnswerSubjectKind) bool {
	switch kind {
	case types.SubjectStringLiteral:
		return quoted || looksLikeDeclarativeLiteralToken(token)
	case types.SubjectReturnValue:
		return quoted || looksLikeDeclarativeLiteralToken(token)
	case types.SubjectHandlerRoute, types.SubjectConfigKey, types.SubjectFilePath:
		return subject.Score(token, kind, nil) >= 0.4 || looksLikeDeclarativeLiteralToken(token)
	case types.SubjectUnknown, types.SubjectGeneric:
		return quoted || looksLikeDeclarativeLiteralToken(token)
	}
	return false
}

func normalizeExtractorFallbackToken(token string) string {
	token = strings.TrimSpace(strings.Trim(token, "`\"'"))
	if token == "" || len(token) > 120 || strings.ContainsAny(token, "\r\n\t") {
		return ""
	}
	return token
}

func extractorSingleTokenCandidate(token string) string {
	token = normalizeExtractorFallbackToken(token)
	if token == "" || strings.ContainsAny(token, " {}()[],:") {
		return ""
	}
	return token
}

type extractorReadFileLiteral struct {
	line  int
	text  string
	token string
}

func extractorReadFileLiteralCandidates(index map[string]map[int]string, source, repoRoot string, lineStart, lineEnd int) []extractorReadFileLiteral {
	// Canonicalise the source path against repoRoot so an evidence
	// item citing `/abs/<root>/pkg/foo.go` resolves against a
	// lineIndex keyed by the repo-relative `pkg/foo.go` form (and
	// vice-versa). Without repoRoot threading the two sides landed
	// on disjoint map keys whenever the LLM emitted a path shape
	// different from the read_file banner's path shape. Empty
	// repoRoot degrades to canonicalExplorerPath (historical
	// behaviour) so tests with no real root keep passing.
	if repoRoot != "" {
		source = ground.CanonicalRepoRelative(source, repoRoot)
	} else {
		source = canonicalExplorerPath(source)
	}
	if source == "" || lineStart <= 0 {
		return nil
	}
	lines := index[source]
	if len(lines) == 0 {
		return nil
	}
	if lineEnd < lineStart {
		lineEnd = lineStart
	}
	seen := make(map[string]bool)
	out := make([]extractorReadFileLiteral, 0, 4)
	for line := lineStart; line <= lineEnd+2; line++ {
		text, ok := lines[line]
		if !ok {
			continue
		}
		for _, lit := range extractQuotedLiterals(text) {
			key := fmt.Sprintf("%d\x1f%s", line, lit)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, extractorReadFileLiteral{
				line:  line,
				text:  text,
				token: lit,
			})
		}
	}
	return out
}

func extractorReadFileLineIndex(ctx *types.AgentContext) map[string]map[int]string {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	ta := ctx.Mutable.TurnAArtifacts()
	if ta == nil || len(ta.ToolResults) == 0 {
		return nil
	}
	index := make(map[string]map[int]string)
	for _, r := range ta.ToolResults {
		if !r.Success || r.ToolName != "read_file" {
			continue
		}
		path, _, _, ok := parseReadFileBanner(r.Summary)
		if !ok {
			continue
		}
		if ctx.RepoRoot != "" {
			path = ground.CanonicalRepoRelative(path, ctx.RepoRoot)
		} else {
			path = canonicalExplorerPath(path)
		}
		bodyStart := strings.IndexByte(r.Summary, '\n')
		if path == "" || bodyStart < 0 || bodyStart >= len(r.Summary)-1 {
			continue
		}
		for _, raw := range strings.Split(r.Summary[bodyStart+1:], "\n") {
			line, text, ok := extractorParseGutterLine(raw)
			if !ok {
				continue
			}
			if index[path] == nil {
				index[path] = make(map[int]string)
			}
			index[path][line] = text
		}
	}
	if len(index) == 0 {
		return nil
	}
	return index
}

func extractorParseGutterLine(raw string) (line int, text string, ok bool) {
	raw = strings.TrimLeft(raw, " ")
	if raw == "" {
		return 0, "", false
	}
	i := 0
	for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, "", false
	}
	n, err := strconv.Atoi(raw[:i])
	if err != nil || n <= 0 {
		return 0, "", false
	}
	rest := raw[i:]
	space := strings.IndexByte(rest, ' ')
	if space < 0 || space >= len(rest)-1 {
		return 0, "", false
	}
	return n, rest[space+1:], true
}

func looksLikeDeclarativeLiteralToken(token string) bool {
	token = normalizeExtractorFallbackToken(token)
	if token == "" || strings.ContainsAny(token, " {}()[],:") {
		return false
	}
	if strings.ContainsAny(token, "/._-") {
		return true
	}
	hasLetter := false
	hasUpper := false
	for _, r := range token {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsUpper(r) {
			hasUpper = true
		}
	}
	return hasLetter && !hasUpper
}

// axisStrongAffinityThreshold is the affinity-matrix cell value above
// which we consider an evidence item to "strongly realise" the
// question's PredicateAxis. Kept as a const so the threshold is grep-
// able in one place; matches the 1.2 floor empirically picked so
// neutral-leaning boosts (AxisRegister × AnchorCall = 1.2) still
// count while low-confidence pairs (AxisImplement × AnchorCall = 1.1)
// do not force retries on every mechanism question.
const axisStrongAffinityThreshold = 1.2

// axisAnchorRetryHint is the L3 validator for the extractor's
// answer_symbol slate. Returns a non-empty correction hint when the
// question carries a clear PredicateAxis, the evidence pool has at
// least one item whose AnchorKind strongly realises that axis (via
// axis.Affinity ≥ threshold), and NONE of the picked answer_symbols
// correlate with any such evidence item. Returns "" when the picks
// are aligned or the check does not apply (axis zero, no matching
// evidence, or no picks yet).
//
// Correlation: an answer_symbol at (file, line) is correlated with an
// evidence item at (source, [LineStart..LineEnd]) when file == source
// AND LineStart == 0 || line ∈ [LineStart..max(LineStart,LineEnd)].
// Zero LineEnd is treated as a single-line range. File comparison is
// strict — canonicalised by the upstream grounder.
func axisAnchorRetryHint(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil {
		return ""
	}
	if !needsAnswerSymbols(ctx) {
		// Plain call-chain / mechanism questions render their
		// principal payload downstream as ordered_list / diagram
		// blocks. If the model emitted a stray answer-symbol slate
		// anyway, do not reinforce that mistake with an axis-alignment
		// retry hint.
		return ""
	}
	if viewNeedsBoundedPrincipalList(ctx) {
		// A user-declared bounded principal set uses the answer-symbol
		// slate to lock the member set itself. Axis-aligned condition /
		// call anchors can remain in the evidence pool and final prose;
		// forcing them into the principal slate mixes two different
		// surface authorities and destabilizes ordered-set questions.
		return ""
	}
	if isMultiTopicExplanation(ctx) && !viewNeedsEnumerationSlate(ctx) {
		// Multi-topic explanations use answer_symbols as lightweight
		// prose skeleton anchors, not as the principal answer surface.
		// Axis evidence such as guard conditions, returns, assignments,
		// or call sites already flows through the evidence/support lanes;
		// forcing those non-symbol anchors into emit_answer_symbol both
		// over-constrains narrative answers and can ask for kinds that
		// the answer-symbol taxonomy deliberately does not contain.
		return ""
	}
	rm := ctx.Mutable.RequestModel()
	if rm == nil {
		return ""
	}
	pa := rm.PredicateAxis
	if pa == types.AxisUnknown {
		return ""
	}

	// Union of every reachable evidence source at extractor time:
	//   - ctx.EvidenceItems: merged BusContext pool
	//   - ctx.Mutable.EmittedEvidence(): LLM-emitted buffer (only
	//     this channel carries the anchor_kind field the LLM sent)
	//   - ctx.Mutable.TurnAArtifacts().EvidenceItems: Turn A handoff
	//     snapshot (StrictOK subset, often empty)
	// Dedup by (Source, LineStart, AnchorSymbol) keeps the matching
	// set lean. Without unioning these three, the emit_evidence
	// channel's anchor_kind signal is lost — BusContext merges only
	// typed evidence, not the raw LLM buffer.
	pool := mergeEvidenceForAxisCheck(ctx)
	if len(pool) == 0 {
		return ""
	}
	// Collect evidence items whose AnchorKind strongly realises the axis.
	matching := make([]types.EvidenceItem, 0, len(pool))
	for _, ev := range pool {
		if ev.AnchorKind == "" {
			continue
		}
		if axis.Affinity(pa, ev.AnchorKind) >= axisStrongAffinityThreshold {
			matching = append(matching, ev)
		}
	}
	if len(matching) == 0 {
		// No axis-matching evidence in the pool — cannot blame the
		// LLM for picking non-aligned anchors when none exist.
		return ""
	}

	syms, _ := ctx.Mutable.EmittedAnswerSymbols()
	if len(syms) == 0 {
		// Missing-symbols retry path owns this case.
		return ""
	}
	for _, s := range syms {
		for _, ev := range matching {
			if answerSymbolCorrelatesWithEvidence(s, ev) {
				return "" // at least one alignment → pass
			}
		}
	}

	// Violation: build a concrete hint naming a matching evidence item.
	top := matching[0]
	return fmt.Sprintf(
		"The question axis is %q but none of the %d emit_answer_symbol items you picked correlate with an evidence item of kind=%s in the investigation's pool. Re-emit emit_answer_symbol INCLUDING at least one symbol whose file:line matches an axis-aligned evidence item, e.g. %s:%d (%s). The final answer needs a call-site anchor for its prose to hang on.",
		pa,
		len(syms),
		top.AnchorKind,
		top.Source,
		top.LineStart,
		firstNonEmpty(top.AnchorSymbol, top.Subject),
	)
}

// answerSymbolCorrelatesWithEvidence reports whether the given
// answer_symbol's (File, Line) falls within the evidence item's
// (Source, LineStart..LineEnd) range. LineEnd=0 is a single-line
// point; Line=0 on the answer_symbol treats the evidence range as a
// file-level match (any line in the same file passes).
func answerSymbolCorrelatesWithEvidence(s types.AnswerSymbol, ev types.EvidenceItem) bool {
	if s.File == "" || ev.Source == "" || s.File != ev.Source {
		return false
	}
	if s.Line <= 0 {
		// File-level match — acceptable when the answer_symbol lacks a
		// precise line (rare but possible for module-level anchors).
		return true
	}
	end := ev.LineEnd
	if end < ev.LineStart {
		end = ev.LineStart
	}
	if ev.LineStart <= 0 {
		return false
	}
	return s.Line >= ev.LineStart && s.Line <= end
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// mergeEvidenceForAxisCheck returns a deduped union of every
// reachable evidence source at extractor Observe time. The three
// sources differ in shape:
//
//	ctx.EvidenceItems                     — merged pool written by
//	                                         applyStageOutput from every
//	                                         stage's StageOutput
//	ctx.Mutable.EmittedEvidence()         — raw LLM emit_evidence
//	                                         buffer (preserves
//	                                         anchor_kind verbatim)
//	ctx.Mutable.TurnAArtifacts().EvidenceItems
//	                                       — frozen StrictOK subset
//	                                         the explorer picks for
//	                                         Turn B hand-off
//
// The LLM-emitted channel is the only one that guarantees the
// anchor_kind field is populated, so unioning is load-bearing —
// skipping it means the axis validator silently passes even when
// Turn A emitted perfect call-anchored evidence (see the 2026-04-18
// bug replay log where 18 merged items had no anchor_kind but 3
// LLM-emit items had call/definition/call).
//
// Dedup key: (Source, LineStart, LineEnd, AnchorSymbol). Two items
// with identical coordinates are the same evidence in different
// channels, not independent observations.
func mergeEvidenceForAxisCheck(ctx *types.AgentContext) []types.EvidenceItem {
	if ctx == nil {
		return nil
	}
	type key struct {
		src                string
		lineStart, lineEnd int
		anchorSym          string
	}
	seen := make(map[key]bool, 16)
	var out []types.EvidenceItem
	push := func(ev types.EvidenceItem) {
		k := key{ev.Source, ev.LineStart, ev.LineEnd, ev.AnchorSymbol}
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, ev)
	}
	for _, ev := range ctx.EvidenceItems {
		push(ev)
	}
	if ctx.Mutable != nil {
		for _, ev := range ctx.Mutable.EmittedEvidence() {
			push(ev)
		}
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			for _, ev := range ta.EvidenceItems {
				push(ev)
			}
		}
	}
	return out
}

// Observe implements LoopController.
//
// Turn B has TWO expected tools — emit_answer_symbol (for
// list_of_symbols questions) and emit_hypothesis_verdict (for
// hypotheses without a pre-populated auto-verdict). The happy path
// is both fired in parallel in one LLM response; the fallback path
// lets the LLM emit the missing one on a later iteration. Mid-loop
// stops as soon as every EXPECTED emit has succeeded; soft-stop
// corrects if the LLM voluntarily stopped while an expected emit is
// still outstanding.
//
// "Pending hypothesis" = hypothesis in AnalysisIR.HypothesisSet whose
// ID is NOT yet in Mutable.EmittedHypothesisVerdicts(). Auto-verdicts
// the orchestrator pre-injects after explore windows already populate
// the buffer, so when every hypothesis is auto-verdicted needVerdicts
// is false and the LLM is not nagged to re-emit — the auto path is
// treated as sufficient fallback. The LLM CAN still override by
// emitting in parallel with emit_answer_symbol at iter=0 (drain
// semantics: last-written wins); it simply isn't forced to.
func (e *extractorEvaluator) Observe(ctx *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase == PhaseMidLoop {
		if unsupported := extractorUnsupportedToolCalls(obs.Response.ToolCalls); len(unsupported) > 0 &&
			!extractorHasSuccessfulAllowedToolResult(obs.AllToolResults) &&
			e.retriesUsed < e.maxRetries {
			e.retriesUsed++
			return LoopSignal{
				HintRequested: true,
				HintKey:       fmt.Sprintf("extractor.unsupported_tool.%d", e.retriesUsed),
				Hint: fmt.Sprintf(
					"The extractor stage cannot use tool(s): %s. Use only `emit_evidence`, `emit_answer_symbol`, and `emit_hypothesis_verdict`; do not read/search files here because the investigation snapshot is already fixed. If no structured extraction emit is required, stop without calling unavailable tools.",
					strings.Join(unsupported, ", ")),
			}
		}
		gotSymbols, gotVerdicts := false, false
		for _, r := range obs.AllToolResults {
			if !r.Success {
				continue
			}
			switch r.ToolName {
			case "emit_answer_symbol":
				gotSymbols = true
			case "emit_hypothesis_verdict":
				gotVerdicts = true
			}
		}
		needSymbols := needsAnswerSymbols(ctx)
		needVerdicts := hasPendingHypotheses(ctx)
		if needSymbols && gotSymbols && !hasSufficientAnswerSymbols(ctx) && e.retriesUsed < e.maxRetries {
			e.retriesUsed++
			hint := answerSymbolMaterializationHint(ctx)
			logging.Info("[extractor] answer-symbol materialization retry #%d: %s", e.retriesUsed, hint)
			return LoopSignal{
				HintRequested: true,
				HintKey:       fmt.Sprintf("extractor.answer_symbol_materialization.%d", e.retriesUsed),
				Hint:          hint,
			}
		}
		if (!needSymbols || hasSufficientAnswerSymbols(ctx)) && (!needVerdicts || gotVerdicts) {
			// L3 axis-anchor alignment gate. Before accepting the stop,
			// if the question carries a PredicateAxis AND the evidence
			// pool contains at least one axis-matching item, the picked
			// answer_symbols MUST include at least one that correlates
			// with an axis-matching evidence item. Otherwise the LLM
			// picked registration/definition symbols for a "how does X
			// CALL Y" question — re-dispatch with a correction hint.
			// Gated on e.retriesUsed budget; after the budget is spent
			// the mismatch is tolerated and logged (honest degrade).
			if gotSymbols && e.retriesUsed < e.maxRetries {
				if hint := axisAnchorRetryHint(ctx); hint != "" {
					e.retriesUsed++
					logging.Info("[extractor] axis-anchor mismatch retry #%d: %s", e.retriesUsed, hint)
					return LoopSignal{
						HintRequested: true,
						HintKey:       fmt.Sprintf("extractor.axis_mismatch.%d", e.retriesUsed),
						Hint:          hint,
					}
				}
			}
			return LoopSignal{StopRequested: true, StopReason: "extractor emit batch complete"}
		}
		// At least one expected emit still missing: let the loop run
		// another iteration so the LLM can fill the gap. ShouldStop
		// (iter>=3) and soft-stop correction retries bound the total.
		return LoopSignal{}
	}
	if obs.Phase != PhaseSoftStop {
		return LoopSignal{}
	}
	// Soft-stop correction: the LLM produced text without calling any
	// tool this iteration. If an expected emit is still outstanding
	// and the correction budget has room, inject a hint naming the
	// specific missing tool(s). Capped at e.maxRetries so a
	// non-compliant LLM cannot spin the loop indefinitely.
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return LoopSignal{}
	}
	missingSymbols := needsAnswerSymbols(ctx) && !hasSufficientAnswerSymbols(ctx)
	missingVerdicts := hasPendingHypotheses(ctx)
	if !missingSymbols && !missingVerdicts {
		return LoopSignal{}
	}
	if e.retriesUsed >= e.maxRetries {
		logging.Debug("[extractor] soft-stop correction retries exhausted (%d); accepting response", e.retriesUsed)
		return LoopSignal{}
	}
	e.retriesUsed++
	var missingParts []string
	if missingSymbols {
		switch {
		case viewNeedsBoundedPrincipalList(ctx):
			boundary := requestedEnumerationBoundary(ctx)
			count := 0
			quote := ""
			if boundary != nil {
				count = boundary.DeclaredCount
				quote = boundary.SourceQuote
			}
			missingParts = append(missingParts,
				fmt.Sprintf("call `emit_answer_symbol` with the principal member slate for the bounded set `%s` (%d item(s)); keep the slate within that boundary, cite each item with a concrete file:line from the 'Files the investigation read' list, and leave adjacent caveat-only items out of the main slate", quote, count))
		case viewNeedsEnumerationSlate(ctx):
			missingParts = append(missingParts,
				"call `emit_answer_symbol` with the symbols that answer this list_of_symbols question (cite each with a concrete file:line from the 'Files the investigation read' list)")
		case isMultiTopicExplanation(ctx):
			missingParts = append(missingParts,
				"call `emit_answer_symbol` with ONE anchor symbol per sub-topic — the load-bearing identifier the final answer's prose should hang on. Cite each with a concrete file:line from the 'Files the investigation read' list. Downstream rendering presents these as a Key Anchors skeleton beneath the summary")
		}
	}
	if missingVerdicts {
		missingParts = append(missingParts,
			"call `emit_hypothesis_verdict` for each hypothesis you can now judge (status + rationale + citation when confirmed/rejected, or inconclusive without citation)")
	}
	logging.Debug("[extractor] soft-stop correction retry #%d: missingSymbols=%t missingVerdicts=%t",
		e.retriesUsed, missingSymbols, missingVerdicts)
	// Fix C (2026-05-10) — when the LLM soft-stops AFTER a failed
	// emit_X call earlier in this dispatch, the missing-emits hint
	// alone leaves the LLM without the rejection reason it actually
	// needs to fix. Prepend the latest failed emit-class tool's
	// first-line summary so the LLM can address the rejection AND
	// the missing emit in one response. Without this, the s7b
	// dispatch-2 pattern repeats: LLM gets generic "you stopped
	// without emitting" and re-emits the same wrong shape.
	priorEmitContext := latestExtractorEmitFailureContext(obs.AllToolResults)
	body := "You stopped without emitting one or more expected tool calls. Please " +
		strings.Join(missingParts, ", and ") +
		". If both apply, batch them in parallel in a single response."
	return LoopSignal{
		HintRequested: true,
		HintKey:       fmt.Sprintf("extractor.missing_emits.%d", e.retriesUsed),
		Hint:          priorEmitContext + body,
	}
}

func extractorUnsupportedToolCalls(calls []llm.ToolCall) []string {
	seen := map[string]bool{}
	var out []string
	for _, call := range calls {
		name := types.CanonicalToolName(call.Name)
		if extractorAllowedToolName(name) {
			continue
		}
		if name == "" {
			name = strings.TrimSpace(call.Name)
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func extractorAllowedToolName(name string) bool {
	switch types.CanonicalToolName(name) {
	case "emit_evidence", "emit_answer_symbol", "emit_hypothesis_verdict":
		return true
	default:
		return false
	}
}

func extractorHasSuccessfulAllowedToolResult(results []types.ToolResult) bool {
	for _, result := range results {
		if result.Success && extractorAllowedToolName(result.ToolName) {
			return true
		}
	}
	return false
}

// latestExtractorEmitFailureContext returns a short prefix sentence
// citing the most recent failed emit_* tool result's first-line
// summary, or empty when none was observed in this dispatch. The
// summary is truncated so the prefix stays single-sentence and
// doesn't dwarf the body hint.
func latestExtractorEmitFailureContext(results []types.ToolResult) string {
	const maxFirstLine = 240
	for i := len(results) - 1; i >= 0; i-- {
		r := results[i]
		if r.Success {
			continue
		}
		if !strings.HasPrefix(r.ToolName, "emit_") {
			continue
		}
		first := r.Summary
		if idx := strings.IndexByte(first, '\n'); idx >= 0 {
			first = first[:idx]
		}
		first = strings.TrimSpace(first)
		if first == "" {
			return ""
		}
		if len(first) > maxFirstLine {
			first = first[:maxFirstLine] + "…"
		}
		return fmt.Sprintf("Your last `%s` call was rejected: %s. Address that rejection in your next attempt. ",
			r.ToolName, first)
	}
	return ""
}

// DetermineMissingPiece implements Evaluator.
//
// Turn B never triggers a backtrack on its own — when extraction
// fails, the orchestrator's contract checker downstream of finalize
// owns the retry decision. Returning MissingNone keeps the extractor
// out of the orchestrator's "what stage do we route to next" branch.
func (e *extractorEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingNone
}

// NewExtractorAgent constructs the Turn B agent. Mirrors
// NewFinalizerAgent in shape so the registry constructor table looks
// uniform.
func NewExtractorAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentExtractor, deps, &extractorEvaluator{
		maxRetries:         deps.AgentSettings.ExtractorMaxCorrectionRetries,
		defaultSoftIterCap: deps.AgentSettings.ExtractorSoftIterCap,
		defaultHardIterCap: deps.AgentSettings.ExtractorHardIterCap,
	})
}

// viewNeedsEnumerationSlate reports whether the answer is
// enumeration-shaped — i.e. the LLM is expected to emit a slate of
// named anchors via emit_answer_symbol. Used by Observe to decide
// whether emit_answer_symbol is an expected tool for this dispatch.
//
// Reads the compiled AnswerSemanticView's NeedsEnumerationSlate()
// helper, which is true for QFEnumeration. The legacy ShapeListOfSymbols
// gate has been retired per docs/migration/answer_shape_retirement.md.
func viewNeedsEnumerationSlate(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	return view.NeedsEnumerationSlate()
}

func extractorAnswerSurfacePlan(ctx *types.AgentContext) *types.AnswerSurfacePlan {
	return types.BuildAnswerSurfacePlanForAgentContext(ctx)
}

func requestedEnumerationBoundary(ctx *types.AgentContext) *types.RequestedEnumerationBoundary {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	boundary := ctx.AnalysisIR.RequestModel.EnumerationBoundary
	if boundary == nil || boundary.DeclaredCount <= 0 {
		return nil
	}
	return boundary
}

// viewNeedsBoundedPrincipalList reports whether the answer is a
// bounded-count ordered sequence (the user declared "the N steps" /
// "前 N 个" etc.) AND the family produces a principal ordered list.
// True for the families whose compile_<family> emits BlockOrderedList
// at SurfacePrincipal: QFCallChain, QFRootCauseTrace, QFEnumeration.
// Replaces the legacy isBoundedPrincipalStepList (RequiredShape-
// driven) per docs/migration/answer_shape_retirement.md.
func viewNeedsBoundedPrincipalList(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if requestedEnumerationBoundary(ctx) == nil {
		return false
	}
	if types.RequestedEnumerationBoundaryOwner(ctx.AnalysisIR.RequestModel) == "" {
		return false
	}
	family := types.ResolveQuestionFamily(ctx.AnalysisIR.RequestModel)
	switch family {
	case types.QFCallChain, types.QFRootCauseTrace, types.QFEnumeration:
		return true
	}
	return false
}

func needsAnswerSymbols(ctx *types.AgentContext) bool {
	if pureVCSMetadataAnswerRendersWithoutAnswerSymbols(ctx) {
		return false
	}
	if originSpecificAnswerRendersWithoutAnswerSymbols(ctx) {
		return false
	}
	if sourceInventoryPrincipalAggregateRendersWithoutAnswerSymbols(ctx) {
		return false
	}
	if relationPrincipalMemberSetRendersWithoutAnswerSymbols(ctx) {
		return false
	}
	if relationAxisPrincipalEvidenceRendersWithoutAnswerSymbols(ctx) {
		return false
	}
	if requiresMultiTopicAnchorSkeleton(ctx) || viewNeedsBoundedPrincipalList(ctx) {
		return true
	}
	if viewNeedsEnumerationSlate(ctx) {
		return !enumerationPrincipalEvidenceRendersWithoutAnswerSymbols(ctx)
	}
	return false
}

func originSpecificAnswerRendersWithoutAnswerSymbols(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil || viewNeedsBoundedPrincipalList(ctx) {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if originSpecificPrincipalAggregateRendersWithoutAnswerSymbols(ctx, &rm) {
		return true
	}
	if !originSpecificNarrativeOrValueAnswer(ctx, rm) {
		return false
	}
	return agentContextHasOriginSpecificSupport(ctx)
}

func originSpecificPrincipalAggregateRendersWithoutAnswerSymbols(ctx *types.AgentContext, rm *types.RequestModel) bool {
	if ctx == nil || ctx.Mutable == nil || rm == nil {
		return false
	}
	facts := ctx.Mutable.StableInvestigationAggregateFacts()
	if len(facts) == 0 {
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			facts = ta.AcceptedAggregateFacts
		}
	}
	if len(facts) == 0 {
		return false
	}
	facts = types.NormalizeAggregateFactRolesForRequest(facts, rm)
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateMemberSet ||
			len(fact.Members) == 0 ||
			types.AnswerAggregateFactRoleForRequest(fact, rm) != types.AnswerAggregateRolePrincipalAnswer ||
			types.AggregateMemberSetIsScalarCountSupport(rm, fact) {
			continue
		}
		for _, origin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
			if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
				return true
			}
		}
	}
	return false
}

func originSpecificNarrativeOrValueAnswer(ctx *types.AgentContext, rm types.RequestModel) bool {
	contract := types.CompileAnswerIntentContract(rm, &ctx.AnalysisIR.AnswerContract)
	hasOriginSpecific := false
	for _, origin := range contract.Origins {
		if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			hasOriginSpecific = true
			break
		}
	}
	if !hasOriginSpecific && agentContextHasOriginSpecificSupport(ctx) {
		hasOriginSpecific = true
	}
	if !hasOriginSpecific {
		return false
	}
	for _, output := range []types.AnswerRequestedOutput{
		types.AnswerRequestedOutputScalar,
		types.AnswerRequestedOutputKeyValue,
		types.AnswerRequestedOutputCount,
		types.AnswerRequestedOutputComparison,
		types.AnswerRequestedOutputMechanism,
		types.AnswerRequestedOutputTrace,
		types.AnswerRequestedOutputDiagnostic,
		types.AnswerRequestedOutputChangeImpact,
	} {
		if contract.HasOutput(output) {
			return true
		}
	}
	return false
}

func agentContextHasOriginSpecificSupport(ctx *types.AgentContext) bool {
	if ctx == nil {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromAgentContext(ctx, 64))
	for _, record := range ledger.Records {
		if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
			return true
		}
	}
	if ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return false
	}
	rm := &ctx.AnalysisIR.RequestModel
	allFacts := append([]types.AnswerAggregateFact(nil), ctx.Mutable.StableInvestigationAggregateFacts()...)
	if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
		allFacts = append(allFacts, ta.AcceptedAggregateFacts...)
	}
	for _, fact := range allFacts {
		for _, origin := range types.AnswerAggregateFactEvidenceOrigins(fact, rm) {
			if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
				return true
			}
		}
	}
	return false
}

func relationPrincipalMemberSetRendersWithoutAnswerSymbols(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil || viewNeedsBoundedPrincipalList(ctx) {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	facts := ctx.Mutable.StableInvestigationAggregateFacts()
	if len(facts) == 0 {
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
			facts = ta.AcceptedAggregateFacts
		}
	}
	if len(facts) == 0 {
		return false
	}
	facts = types.NormalizeAggregateFactRolesForRequest(facts, &rm)
	if (types.RequiresRelationMemberSetHandoff(rm) || types.HasTypedRelationMemberSetShape(rm)) &&
		len(types.PrincipalAggregateMemberSetFactRefsForRequest(facts, &rm)) > 0 {
		return true
	}
	return types.PrincipalMemberSetHasRelationEvidence(facts, mergeEvidenceForAxisCheck(ctx), &rm)
}

func sourceInventoryPrincipalAggregateRendersWithoutAnswerSymbols(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if !rm.SourceInventoryProfile.Active() || viewNeedsBoundedPrincipalList(ctx) {
		return false
	}
	plan := extractorAnswerSurfacePlan(ctx)
	sets := types.CompileEnumerationDisplaySets(&rm, plan)
	if len(sets) == 0 {
		return false
	}
	for _, set := range sets {
		if !set.Role.IsPrincipal() || len(set.Rows) == 0 {
			continue
		}
		completeGroundedRows := true
		for _, row := range set.Rows {
			if strings.TrimSpace(row.DisplayLabel) == "" ||
				strings.TrimSpace(row.Source) == "" ||
				row.LineStart <= 0 {
				completeGroundedRows = false
				break
			}
		}
		if completeGroundedRows {
			return true
		}
	}
	return false
}

func relationAxisPrincipalEvidenceRendersWithoutAnswerSymbols(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil ||
		!viewNeedsEnumerationSlate(ctx) || viewNeedsBoundedPrincipalList(ctx) {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.PredicateAxis == types.AxisUnknown || rm.PredicateAxis == types.AxisDefine {
		return false
	}
	facts := ctx.Mutable.StableInvestigationAggregateFacts()
	if len(facts) == 0 {
		return false
	}
	if !types.PrincipalMemberSetHasRelationEvidence(facts, mergeEvidenceForAxisCheck(ctx), &rm) {
		return false
	}
	for _, item := range mergeEvidenceForAxisCheck(ctx) {
		if item.AnchorKind != "" && item.AnchorKind != types.AnchorDefinition &&
			axis.Affinity(rm.PredicateAxis, item.AnchorKind) >= axisStrongAffinityThreshold {
			return true
		}
	}
	return false
}

func pureVCSMetadataAnswerRendersWithoutAnswerSymbols(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if !rm.Predicates.IsHistoryLookup {
		return false
	}
	contract := types.CompileAnswerIntentContract(rm, &ctx.AnalysisIR.AnswerContract)
	if !contract.HasOrigin(types.AnswerEvidenceOriginVCSMetadata) ||
		contract.HasOrigin(types.AnswerEvidenceOriginCurrentSource) {
		return false
	}
	if contract.HasOutput(types.AnswerRequestedOutputMechanism) ||
		contract.HasOutput(types.AnswerRequestedOutputTrace) ||
		contract.HasOutput(types.AnswerRequestedOutputDiagram) ||
		contract.HasOutput(types.AnswerRequestedOutputDiagnostic) ||
		contract.HasOutput(types.AnswerRequestedOutputChangeImpact) {
		return false
	}
	return true
}

func enumerationPrincipalEvidenceRendersWithoutAnswerSymbols(ctx *types.AgentContext) bool {
	if ctx == nil || !viewNeedsEnumerationSlate(ctx) || viewNeedsBoundedPrincipalList(ctx) {
		return false
	}
	nonSymbolPrincipal := 0
	symbolPrincipal := 0
	if support := types.BuildAnswerSupportPlanForAgentContext(ctx); support != nil {
		nonSymbolPrincipal, symbolPrincipal = types.AnswerSupportPlanPrincipalSurfaceCounts(support)
		if nonSymbolPrincipal > 0 || symbolPrincipal > 0 {
			return nonSymbolPrincipal > 0 && symbolPrincipal == 0
		}
	}
	plan := extractorAnswerSurfacePlan(ctx)
	items := types.PrincipalSupportEvidenceItemsForFamily(types.QFEnumeration, plan)
	if len(items) == 0 {
		return false
	}
	rm := types.RequestModel{}
	if ctx.AnalysisIR != nil {
		rm = ctx.AnalysisIR.RequestModel
	}
	for _, item := range items {
		if !extractorEvidenceIsModelAuthoredPrincipal(item) {
			continue
		}
		surface := types.PrincipalMemberSurfaceForRequest(rm, item, items)
		if surface.IsNonSymbol() {
			nonSymbolPrincipal++
			continue
		}
		if surface.IsSymbolLike() {
			symbolPrincipal++
		}
	}
	return nonSymbolPrincipal > 0 && symbolPrincipal == 0
}

func extractorEvidenceIsModelAuthoredPrincipal(item types.EvidenceItem) bool {
	if strings.TrimSpace(item.Producer) == "explorer.emit_evidence" {
		return true
	}
	return strings.TrimSpace(item.Producer) != "" && item.Kind.IsLLMEmittable()
}

func requiredAnswerSymbolCount(ctx *types.AgentContext) int {
	if boundary := requestedEnumerationBoundary(ctx); boundary != nil && viewNeedsBoundedPrincipalList(ctx) {
		return boundary.DeclaredCount
	}
	if requiresMultiTopicAnchorSkeleton(ctx) && ctx != nil && ctx.AnalysisIR != nil {
		return len(ctx.AnalysisIR.RequestModel.SubTopics)
	}
	return 0
}

func hasSufficientAnswerSymbols(ctx *types.AgentContext) bool {
	if !needsAnswerSymbols(ctx) {
		return true
	}
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	syms, _ := ctx.Mutable.EmittedAnswerSymbols()
	required := requiredAnswerSymbolCount(ctx)
	if required > 0 {
		return len(syms) >= required
	}
	return len(syms) > 0
}

func answerSymbolMaterializationHint(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return "Re-emit `emit_answer_symbol` with the grounded answer-symbol slate before stopping."
	}
	syms, _ := ctx.Mutable.EmittedAnswerSymbols()
	if viewNeedsBoundedPrincipalList(ctx) {
		if boundary := requestedEnumerationBoundary(ctx); boundary != nil {
			if ctx.AnalysisIR != nil && types.HasAttributeBearingEnumeration(ctx.AnalysisIR.RequestModel) {
				return fmt.Sprintf("The user explicitly requested the bounded principal set `%s` (%d item(s)), but the accepted `emit_answer_symbol` slate currently contains only %d grounded principal member(s). Re-emit `emit_answer_symbol` with the full principal member slate for that bounded set. For this attribute-bearing enumeration, each item is the principal member; put the related per-member attribute in `rationale` with a grounded file:line when available, and mark unresolved attributes in the rationale/caveat instead of dropping the member. Keep the slate within %d items.", boundary.SourceQuote, boundary.DeclaredCount, len(syms), boundary.DeclaredCount)
			}
			return fmt.Sprintf("The user explicitly requested the bounded principal set `%s` (%d item(s)), but the accepted `emit_answer_symbol` slate currently contains only %d grounded item(s). Re-emit `emit_answer_symbol` now with the full principal member slate for that bounded set. Reuse the compiled candidate pool's exact file:line + symbol names when available, keep the slate within %d items, and leave adjacent caveat-only checks out of the main slate.", boundary.SourceQuote, boundary.DeclaredCount, len(syms), boundary.DeclaredCount)
		}
	}
	if requiresMultiTopicAnchorSkeleton(ctx) && ctx.AnalysisIR != nil {
		return fmt.Sprintf("The analyzer produced %d sub-topic(s), but the accepted `emit_answer_symbol` slate currently contains only %d grounded anchor(s). Re-emit `emit_answer_symbol` now with one grounded anchor per sub-topic, reusing the resolved anchor list when available.", len(ctx.AnalysisIR.RequestModel.SubTopics), len(syms))
	}
	return "Re-emit `emit_answer_symbol` with the grounded answer-symbol slate before stopping."
}

// isMultiTopicExplanation reports whether the analyzer produced a
// multi-topic explanation that may carry optional anchor-skeleton
// context. The hard obligation is narrower: use
// requiresMultiTopicAnchorSkeleton when deciding whether to force
// emit_answer_symbol. Architecture / mechanism families already have
// prose, section, diagram, and evidence support lanes; forcing an
// anchor per analyzer sub-topic there can promote pre-scan drift into
// user-visible principal content.
func isMultiTopicExplanation(ctx *types.AgentContext) bool {
	if ctx == nil {
		return false
	}
	return types.IRAllowsAnchorSkeleton(ctx.AnalysisIR)
}

func requiresMultiTopicAnchorSkeleton(ctx *types.AgentContext) bool {
	if ctx == nil {
		return false
	}
	return types.IRRequiresAnchorSkeleton(ctx.AnalysisIR)
}

// hasPendingHypotheses reports whether the analyzer posed hypotheses
// that still lack a verdict in Mutable.EmittedHypothesisVerdicts.
// Used by Observe to decide whether emit_hypothesis_verdict is an
// expected tool for this dispatch.
//
// The orchestrator pre-injects criterion-based auto-verdicts after
// each explore window (runAutoVerdicts + drainHypothesisVerdicts), so
// by the time Turn B runs, every hypothesis whose RequiredEvidence or
// FalsificationCondition is fully decidable already has a verdict in
// the buffer. "Pending" therefore means: a hypothesis the LLM could
// judge but that the deterministic path did NOT already verdict —
// exactly the set worth prompting the LLM for. Hypotheses that are
// already verdicted (auto or LLM-emitted) do not count; the LLM can
// still override by emitting in parallel at iter=0, but is not nagged
// to do so.
func hasPendingHypotheses(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.Mutable == nil {
		return false
	}
	if len(ctx.AnalysisIR.HypothesisSet) == 0 {
		return false
	}
	if extractorObservationOnlyRuntimeArtifact(ctx) {
		return false
	}
	verdicted := make(map[string]bool)
	for _, v := range ctx.Mutable.EmittedHypothesisVerdicts() {
		verdicted[v.HypothesisID] = true
	}
	for _, h := range ctx.AnalysisIR.HypothesisSet {
		if !verdicted[h.ID] {
			return true
		}
	}
	return false
}

// extractorInvestigationEmpty reports whether Turn A left the
// extractor nothing real to work with. Three acceptance signals;
// ANY one passes:
//
//  1. ReadFiles > 0 — the LLM opened at least one file.
//  2. One or more key evidence kinds (Direct / Registration /
//     Mechanism) surfaced from emit_evidence or deterministic
//     concrete_value extraction.
//  3. At least one successful investigation-class tool call
//     (grep / exec_command / list_files / read_file / repo_map).
//     This rescues count / existence / "how many X files" shapes
//     that legitimately answer with exec_command `find | wc -l` or
//     `grep files_only=true` without ever opening a file or calling
//     emit_evidence. Without this branch the gate falsely rejects
//     structurally-valid shallow investigations.
//
// Only when ALL three signals are absent does this gate fire — a
// zero-tool zero-read zero-emit run is genuinely empty and the
// extractor should fail loud.
func (e *extractorEvaluator) extractorInvestigationEmpty(ctx *types.AgentContext) bool {
	return InvestigationStructurallyEmpty(ctx.Mutable.TurnAArtifacts(), ctx.EvidenceItems)
}

// InvestigationStructurallyEmpty reports whether Turn A left
// downstream stages without any real investigation product. Shared by
// the extractor's fail-loud gate and the orchestrator's pre-finalize
// backtrack so a zero-read / zero-search / zero-evidence explore
// window cannot silently flow into finalization.
func InvestigationStructurallyEmpty(ta *types.TurnAArtifacts, evidence []types.EvidenceItem) bool {
	if ta != nil {
		if ta.RuntimeObservationOnlyCompletion {
			return false
		}
		if len(ta.ReadFiles) > 0 {
			return false
		}
		if len(ta.EvidenceItems) > 0 || ta.TerminalEvidenceCount > 0 {
			return false
		}
		for _, r := range ta.ToolResults {
			if !r.Success {
				continue
			}
			if investigationToolKinds[r.ToolName] {
				return false
			}
		}
	}
	for _, it := range evidence {
		switch it.Kind {
		case types.EvidenceDirect, types.EvidenceRegistration, types.EvidenceMechanism:
			return false
		}
	}
	return true
}

// investigationToolKinds mirrors the orchestrator's
// contract_check.go list so the extractor gate and the contract
// audit agree on what "real investigation work" is. Keep in sync.
var investigationToolKinds = map[string]bool{
	"grep":               true,
	"exec_command":       true,
	"list_files":         true,
	"read_file":          true,
	"repo_map":           true,
	"git_log":            true,
	"git_show":           true,
	"git_diff":           true,
	"git_history_search": true,
}
