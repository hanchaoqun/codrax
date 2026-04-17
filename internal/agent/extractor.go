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
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Extractor prompt display caps. Named constants so the limits are
// grep-visible and changeable in one place.
const (
	extractorMaxNotes       = 6    // max investigation note entries shown in prompt
	extractorMaxNoteChars   = 1200 // max chars per note before truncation
	extractorMaxEvidence    = 24   // max ranked evidence items shown
	extractorMaxEvidenceSummary = 200 // max summary chars per evidence item
	extractorMaxFlowFindings = 10  // max dataflow findings shown
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
	var b strings.Builder

	// User question is already rendered by builder.go as "User Request"
	// section — no need to repeat it here.

	// -------- Turn A transcript digest --------
	ta := (*types.TurnAArtifacts)(nil)
	if ctx != nil && ctx.Mutable != nil {
		ta = ctx.Mutable.TurnAArtifacts()
	}
	if ta == nil {
		b.WriteString("## Turn A transcript\n\n")
		b.WriteString("**No transcript available** — Turn A did not produce a snapshot for this ")
		b.WriteString("dispatch. This is an unusual path (unit-test bootstrap or wiring bug). ")
		b.WriteString("Produce whatever `emit_*` calls you can justify from the user question ")
		b.WriteString("alone, and set `completeness` to `unknown` for any answer-symbol emission.\n\n")
	} else {
		b.WriteString("## Turn A transcript digest\n\n")

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
			b.WriteString("### Files Turn A read (authoritative citation source)\n\n")
			b.WriteString("You MUST cite only these files in emit_* calls. Any other path is a ")
			b.WriteString("hallucination and will be rejected as ungrounded.\n\n")
			for _, f := range ta.ReadFiles {
				fmt.Fprintf(&b, "- `%s`\n", f)
			}
			b.WriteString("\n")
		}

		// Deterministic evidence: top 24 ranked items
		if len(ta.EvidenceItems) > 0 {
			b.WriteString("### Deterministic evidence Turn A extracted\n\n")
			evMax := len(ta.EvidenceItems)
			if evMax > extractorMaxEvidence {
				fmt.Fprintf(&b, "*(showing top %d of %d ranked items)*\n\n", extractorMaxEvidence, evMax)
				evMax = extractorMaxEvidence
			}
			for i := 0; i < evMax; i++ {
				ev := ta.EvidenceItems[i]
				cite := ""
				if ev.Source != "" {
					if ev.LineStart > 0 {
						cite = fmt.Sprintf(" @ %s:%d", ev.Source, ev.LineStart)
					} else {
						cite = " @ " + ev.Source
					}
				}
				summary := strings.TrimSpace(ev.Summary)
				if summary == "" {
					parts := []string{ev.Subject, ev.Predicate, ev.Object}
					summary = strings.TrimSpace(strings.Join(parts, " "))
				}
				if len(summary) > extractorMaxEvidenceSummary {
					summary = summary[:extractorMaxEvidenceSummary] + "…"
				}
				fmt.Fprintf(&b, "- [%s] %s%s\n", ev.Kind, summary, cite)
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
		fmt.Fprintf(&b, "- **Turn A terminal-evidence count (β):** %d\n", ta.TerminalEvidenceCount)
		if ctx != nil && ctx.AnalysisIR != nil {
			must := ctx.AnalysisIR.AnswerContract.MustInclude
			fmt.Fprintf(&b, "- **Analyzer MustInclude (γ):** %d name(s)", len(must))
			if len(must) > 0 {
				fmt.Fprintf(&b, " — %s", strings.Join(must, ", "))
			}
			b.WriteString("\n")
			baseline := ta.TerminalEvidenceCount
			if len(must) > baseline {
				baseline = len(must)
			}
			fmt.Fprintf(&b, "- **Effective floor (max of β and γ):** %d\n", baseline)
			if baseline > 0 {
				fmt.Fprintf(&b, "\nIf you claim `complete`, your `emit_answer_symbol` batch MUST have ≥ %d items. ",
					baseline)
				b.WriteString("If you cannot reach that floor, emit what you have and choose `lower_bound`.\n")
			} else {
				b.WriteString("\nNo baseline data — your claim will be trusted as-is.\n")
			}
		}
		b.WriteString("\n")
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
func (e *extractorEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return iteration >= 3
}

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
		out.Error = "extractor gate: Turn A produced 0 files read and 0 key evidence (direct/registration/mechanism) — investigation is structurally empty"
		logging.Warning("[extractor] R4 fail-loud: %s", out.Error)
		return out, nil
	}

	// Answer-symbol drain + cardinality validator.
	syms, claim := ctx.Mutable.EmittedAnswerSymbols()
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
	baseline := termCount
	if mustInclude > baseline {
		baseline = mustInclude
	}

	if baseline <= 0 {
		// No baseline data to validate against. This happens on REPL
		// turns where Turn A did not produce terminal-literal
		// evidence AND the analyzer did not set MustInclude. We trust
		// the claim — there is nothing structural to cross-check.
		logging.Debug("[extractor] completeness=complete passed through: no baseline data (termCount=%d mustInclude=%d)",
			termCount, mustInclude)
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
		needSymbols := isListOfSymbolsShape(ctx)
		needVerdicts := hasPendingHypotheses(ctx)
		if (!needSymbols || gotSymbols) && (!needVerdicts || gotVerdicts) {
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
	syms, _ := ctx.Mutable.EmittedAnswerSymbols()
	missingSymbols := isListOfSymbolsShape(ctx) && len(syms) == 0
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
		missingParts = append(missingParts,
			"call `emit_answer_symbol` with the symbols that answer this list_of_symbols question (cite each with a concrete file:line from the 'Files Turn A read' list)")
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
		maxRetries: deps.AgentSettings.ExtractorMaxCorrectionRetries,
	})
}

// isListOfSymbolsShape reports whether the analyzer resolved the
// answer shape to list_of_symbols. Used by Observe to decide whether
// emit_answer_symbol is an expected tool for this dispatch.
func isListOfSymbolsShape(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	return ctx.AnalysisIR.AnswerContract.RequiredAnswerShape == types.ShapeListOfSymbols
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
// extractor nothing real to work with: zero files read AND zero
// key evidence items (Direct / Registration / Mechanism). Both
// conditions must hold — a real answer can legitimately come from
// concrete_value + resolution-chain extraction even when the LLM
// read no files (R7/R8 bypass LLM noise), and a pure file-read
// investigation that hasn't produced emit_evidence yet could still
// be one ToolSuggestion away from useful output. The AND catches
// the one structurally-empty combination.
func (e *extractorEvaluator) extractorInvestigationEmpty(ctx *types.AgentContext) bool {
	readFiles := 0
	if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
		readFiles = len(ta.ReadFiles)
	}
	if readFiles > 0 {
		return false
	}
	for _, it := range ctx.EvidenceItems {
		switch it.Kind {
		case types.EvidenceDirect, types.EvidenceRegistration, types.EvidenceMechanism:
			return false
		}
	}
	return true
}
