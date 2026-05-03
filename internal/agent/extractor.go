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
// The cardinality validator closes the completeness-claim loophole:
// when the LLM claims CompletenessComplete but len(items) falls
// below max(Turn A's TerminalEvidenceCount, len(AnswerContract.MustInclude)),
// ParseOutput downgrades the claim to CompletenessLowerBound, logs
// a warning diagnosing the mismatch, and lets the finalizer render
// the softened floor prompt instead of the Translation-mode prompt.
// The schema-level check still rejects malformed calls; this is the
// semantic second layer.
//
// ShouldStop is deliberately one-shot (iteration >= 1). Turn B
// cannot read new files, so a retry has no new information to work
// with — downgrading to lower_bound is the honest terminal state.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/analysis/axis"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/analysis/subject"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Extractor prompt display caps. Named constants so the limits are
// grep-visible and changeable in one place.
const (
	extractorMaxNotes           = 6    // max investigation note entries shown in prompt
	extractorMaxNoteChars       = 1200 // max chars per note before truncation
	extractorMaxEvidence        = 24   // max ranked evidence items shown
	extractorMaxEvidenceSummary = 200  // max summary chars per evidence item
	extractorMaxFlowFindings    = 10   // max dataflow findings shown
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
// notes, read files, top evidence, flow findings), the cardinality
// baseline (β + γ + floor), and the hypothesis set. Graceful degrade
// on nil TurnAArtifacts is preserved.
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
		if len(ta.EvidenceItems) > 0 {
			b.WriteString("### Deterministic evidence the investigation extracted\n\n")
			evMax := len(ta.EvidenceItems)
			if evMax > extractorMaxEvidence {
				fmt.Fprintf(&b, "*(showing top %d of %d ranked items)*\n\n", extractorMaxEvidence, evMax)
				evMax = extractorMaxEvidence
			}
			for i := 0; i < evMax; i++ {
				ev := ta.EvidenceItems[i]
				// Turn B strict: DisplayLocation(true) strips
				// LineStart for Recovered/Ungrounded items so the
				// extractor LLM cannot pick a line the finalizer's
				// stricter grounder will later reject.
				cite := ""
				if loc := ev.DisplayLocation(true); loc != "" {
					cite = " @ " + loc
				}
				summary := strings.TrimSpace(ev.Summary)
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

		// Flow findings: top 10 source→sink chains
		if len(ta.FlowFindings) > 0 {
			b.WriteString("### Dataflow findings (source → sink chains)\n\n")
			ffMax := len(ta.FlowFindings)
			if ffMax > extractorMaxFlowFindings {
				fmt.Fprintf(&b, "*(showing top %d of %d)*\n\n", extractorMaxFlowFindings, ffMax)
				ffMax = extractorMaxFlowFindings
			}
			for i := 0; i < ffMax; i++ {
				ff := ta.FlowFindings[i]
				fmt.Fprintf(&b, "- `%s` (confidence=%.2f)\n", strings.Join(ff.Path, " → "), ff.Confidence)
			}
			b.WriteString("\n")
		}

		// Cardinality baseline — β + γ + effective floor. This is
		// still rendered inline because it is dynamic (depends on
		// this dispatch's Turn A count and the analyzer's MustInclude
		// list), even though the honesty-contract EXPLANATION lives
		// in the skill config.
		b.WriteString("### Cardinality baseline (for completeness claim)\n\n")
		fmt.Fprintf(&b, "- **Investigation terminal-evidence count:** %d\n", ta.TerminalEvidenceCount)
		if ctx != nil && ctx.AnalysisIR != nil {
			must := ctx.AnalysisIR.AnswerContract.MustInclude
			fmt.Fprintf(&b, "- **Analyzer must-include count:** %d name(s)", len(must))
			if len(must) > 0 {
				fmt.Fprintf(&b, " — %s", strings.Join(must, ", "))
			}
			b.WriteString("\n")
			baseline := ta.TerminalEvidenceCount
			if len(must) > baseline {
				baseline = len(must)
			}
			if boundary := requestedEnumerationBoundary(ctx); boundary != nil {
				baseline = boundary.DeclaredCount
				fmt.Fprintf(&b, "- **Requested set boundary override:** %d item(s) from `%s`\n", boundary.DeclaredCount, boundary.SourceQuote)
				b.WriteString("- **Effective floor (bounded principal set overrides the wider evidence floor):** ")
			} else {
				b.WriteString("- **Effective floor (the larger of the two):** ")
			}
			fmt.Fprintf(&b, "%d\n", baseline)
			if baseline > 0 {
				fmt.Fprintf(&b, "\nIf you claim `complete`, your `emit_answer_symbol` batch MUST have ≥ %d items. ",
					baseline)
				b.WriteString("If you cannot reach that floor, emit what you have and choose `lower_bound`.\n")
			} else {
				b.WriteString("\nNo baseline data — your claim will be trusted as-is.\n")
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
	}

	// -------- Multi-topic explanation skeleton guide --------
	// Render the per-dispatch sub-topic list ONLY when the analyzer
	// resolved shape=explanation AND populated sub_topics. This is
	// the trigger for the "emit one anchor symbol per sub-topic"
	// mode documented in the extract-skill workflow; showing the
	// topic list here tells the LLM exactly how many anchors to
	// emit and what each anchor is for.
	if isMultiTopicExplanation(ctx) {
		st := ctx.AnalysisIR.RequestModel.SubTopics
		plan := extractorAnswerSurfacePlan(ctx)
		b.WriteString("## Anchor skeleton (one per sub-topic)\n\n")
		fmt.Fprintf(&b, "The analyzer identified %d independently-answerable sub-topic(s). ", len(st))
		b.WriteString("For each, call emit_answer_symbol with ONE anchor symbol — the load-bearing ")
		b.WriteString("identifier that the multi-paragraph answer summary will hang on. Each ")
		b.WriteString("anchor needs a concrete file:line from the 'Files the investigation read' list above; ")
		b.WriteString("use the rationale field to name the sub-topic the anchor covers.\n\n")
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
		fmt.Fprintf(&b, "The user explicitly declared a bounded principal set: `%s` (%d item(s)). Emit `emit_answer_symbol` with the principal member slate for that bounded set before the finalizer writes the step list.\n\n",
			boundary.SourceQuote, count)
		b.WriteString("Rules for this slate:\n")
		fmt.Fprintf(&b, "- Keep `items[]` within %d principal member(s).\n", count)
		b.WriteString("- Choose the members that answer the bounded set itself, not every adjacent helper, guard, compatibility shim, or side condition that appears nearby in the same owner flow.\n")
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
	fallback := extractorDeclarativeLiteralFallback(ctx)
	if len(syms) == 0 && len(fallback) > 0 {
		syms = fallback
		claim = types.CompletenessLowerBound
		logging.Info("[extractor] synthesized %d declarative literal answer symbol(s) from grounded evidence fallback", len(syms))
	} else {
		if augmented := mergeDeclarativeFallbackSymbols(syms, fallback); len(augmented) > len(syms) {
			logging.Info("[extractor] augmented answer-symbol slate from %d to %d item(s) using declarative literal fallback", len(syms), len(augmented))
			syms = augmented
			if claim == types.CompletenessUnknown {
				claim = types.CompletenessLowerBound
			}
		}
		if refined := trimDeclarativeSlateToTerminals(syms, fallback); len(refined) > 0 && len(refined) < len(syms) {
			logging.Info("[extractor] pruned answer-symbol slate from %d to %d item(s) using declarative terminal filter", len(syms), len(refined))
			syms = refined
			if claim == types.CompletenessUnknown {
				claim = types.CompletenessLowerBound
			}
		} else if len(syms) > 0 && len(fallback) > 0 && claim == types.CompletenessUnknown {
			claim = types.CompletenessLowerBound
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
	if ctx.AnalysisIR != nil && len(ctx.AnalysisIR.HypothesisSet) > 0 {
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

// validateCompletenessClaim is the cardinality validator for the
// extractor's answer-symbol slate. When the LLM claims "complete"
// but the emitted slate is smaller than the baseline Turn A
// produced OR smaller than the analyzer's MustInclude floor, the
// claim is downgraded to "lower_bound" and a warning is logged. The downgrade is the honest terminal state — the finalizer
// will render the softened floor prompt that preserves the emitted
// symbols as a floor while allowing the LLM to add evidence-backed
// names on top.
//
// The baseline is max(TerminalEvidenceCount, len(MustInclude)):
//
//   - TerminalEvidenceCount (β baseline) comes from Turn A's
//     deterministic extraction pipeline and reflects "how many
//     terminal-literal evidence items did the explorer find?". If
//     Turn A found N and the LLM emits fewer, the LLM has silently
//     dropped some — the claim cannot be "complete".
//
//   - len(MustInclude) (γ baseline) comes from the analyzer's
//     AnswerContract hints and reflects "which names does the
//     analyzer consider mandatory?". The analyzer often lists too
//     few (it runs before investigation) but what it lists is
//     authoritative — a "complete" slate cannot be missing a
//     MustInclude name.
//
// Taking max() gives us the strictest of the two floors, which is
// the correct cross-check: either baseline catches a partial slate.
//
// Claims other than "complete" are passed through unchanged.
// "lower_bound" is always honest by definition; "unknown" always
// drops the slate at rendering time. Only "complete" can be a lie.
func validateCompletenessClaim(ctx *types.AgentContext, syms []types.AnswerSymbol, claim types.CompletenessClaim) types.CompletenessClaim {
	if claim != types.CompletenessComplete {
		return claim
	}

	baseline := 0
	termCount := 0
	mustInclude := 0
	boundaryCount := 0
	if boundary := requestedEnumerationBoundary(ctx); boundary != nil {
		boundaryCount = boundary.DeclaredCount
		baseline = boundaryCount
	} else {
		if ctx != nil && ctx.Mutable != nil {
			if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
				termCount = ta.TerminalEvidenceCount
			}
		}
		if ctx != nil && ctx.AnalysisIR != nil {
			mustInclude = len(ctx.AnalysisIR.AnswerContract.MustInclude)
		}
		baseline = termCount
		if mustInclude > baseline {
			baseline = mustInclude
		}
	}

	if baseline <= 0 {
		// No baseline data to validate against. This happens on REPL
		// turns where Turn A did not produce terminal-literal
		// evidence AND the analyzer did not set MustInclude. We trust
		// the claim — there is nothing structural to cross-check.
		logging.Debug("[extractor] completeness=complete passed through: no baseline data (termCount=%d mustInclude=%d boundary=%d)",
			termCount, mustInclude, boundaryCount)
		return claim
	}

	if len(syms) >= baseline {
		logging.Debug("[extractor] completeness=complete cleared cardinality gate: %d items ≥ baseline %d (termCount=%d mustInclude=%d)",
			len(syms), baseline, termCount, mustInclude)
		return claim
	}

	logging.Warning("[extractor] completeness=complete DOWNGRADED to lower_bound: %d items < baseline %d (termCount=%d mustInclude=%d). The slate is preserved as a floor; the finalizer will use the softened prompt.",
		len(syms), baseline, termCount, mustInclude)
	return types.CompletenessLowerBound
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
	lineIndex := extractorReadFileLineIndex(ctx)
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
		for _, near := range extractorReadFileLiteralCandidates(lineIndex, ev.Source, ctx.RepoRoot, ev.LineStart, ev.LineEnd) {
			add(near.token, true, ev.Source, near.line, near.text, "literal extracted from read_file lines near grounded evidence")
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
//     (TestExtractor_ParseOutput_AugmentsDeclarativeSlateFromReadFileLiterals
//     pins this invariant: 2 current + 4 fallback extractions of skill
//     name strings → 4 unique answers).
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
	requiresSymbolSlate := viewNeedsEnumerationSlate(ctx) || isMultiTopicExplanation(ctx)
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
	if !declarativeFocusRelevant(irQuestionKind(ctx), isEnumeration, axis) {
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
	if viewNeedsBoundedPrincipalList(ctx) {
		// A user-declared bounded principal set uses the answer-symbol
		// slate to lock the member set itself. Axis-aligned condition /
		// call anchors can remain in the evidence pool and final prose;
		// forcing them into the principal slate mixes two different
		// surface authorities and destabilizes ordered-set questions.
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
	return LoopSignal{
		HintRequested: true,
		HintKey:       fmt.Sprintf("extractor.missing_emits.%d", e.retriesUsed),
		Hint: "You stopped without emitting one or more expected tool calls. Please " +
			strings.Join(missingParts, ", and ") +
			". If both apply, batch them in parallel in a single response.",
	}
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
	return viewNeedsEnumerationSlate(ctx) || isMultiTopicExplanation(ctx) || viewNeedsBoundedPrincipalList(ctx)
}

func requiredAnswerSymbolCount(ctx *types.AgentContext) int {
	if boundary := requestedEnumerationBoundary(ctx); boundary != nil && viewNeedsBoundedPrincipalList(ctx) {
		return boundary.DeclaredCount
	}
	if isMultiTopicExplanation(ctx) && ctx != nil && ctx.AnalysisIR != nil {
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
			return fmt.Sprintf("The user explicitly requested the bounded principal set `%s` (%d item(s)), but the accepted `emit_answer_symbol` slate currently contains only %d grounded item(s). Re-emit `emit_answer_symbol` now with the full principal member slate for that bounded set. Reuse the compiled candidate pool's exact file:line + symbol names when available, keep the slate within %d items, and leave adjacent caveat-only checks out of the main slate.", boundary.SourceQuote, boundary.DeclaredCount, len(syms), boundary.DeclaredCount)
		}
	}
	if isMultiTopicExplanation(ctx) && ctx.AnalysisIR != nil {
		return fmt.Sprintf("The analyzer produced %d sub-topic(s), but the accepted `emit_answer_symbol` slate currently contains only %d grounded anchor(s). Re-emit `emit_answer_symbol` now with one grounded anchor per sub-topic, reusing the compiled anchor backbone when available.", len(ctx.AnalysisIR.RequestModel.SubTopics), len(syms))
	}
	return "Re-emit `emit_answer_symbol` with the grounded answer-symbol slate before stopping."
}

// isMultiTopicExplanation reports whether the analyzer produced a
// multi-topic explanation — the shape that trace 1776439797257469553
// exposed as the extractor's worst failure mode. Analyzer sets
// shape=explanation and populates RequestModel.SubTopics; the old
// extractor treated this as "no expected emit" (not
// list_of_symbols, auto-verdicted hypotheses) and returned nothing
// structured. The finalizer then had only Turn A's raw evidence
// stream to work from and frequently invented/copied prose.
//
// The fix: require ONE anchor symbol per sub-topic as a skeleton
// the finalizer's summary hangs on. Each anchor carries a concrete
// file:line that pins the sub-topic's load-bearing identifier
// (e.g. {ProposeSubAgents @ propose_sub_agents.go:N} for a
// sub-topic about sub-agent proposal). The finalizer renders them
// as a Key Anchors skeleton beneath the multi-paragraph summary.
//
// Threshold: SubTopics > 1. Single-topic explanation keeps the old
// path — the summary IS the answer and doesn't need a skeleton.
func isMultiTopicExplanation(ctx *types.AgentContext) bool {
	if ctx == nil {
		return false
	}
	return types.IRAllowsAnchorSkeleton(ctx.AnalysisIR)
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
	"grep":         true,
	"exec_command": true,
	"list_files":   true,
	"read_file":    true,
	"repo_map":     true,
}
