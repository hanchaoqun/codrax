package agent

// extractor.go — P2.1 Turn B (extractor) evaluator.
//
// The extractor is the second half of the P2.1 two-turn explorer
// split. It runs AFTER Turn A (the explorer) has completed its
// investigation and handed off a frozen TurnAArtifacts snapshot, and
// BEFORE the finalizer synthesizes the user-visible answer. Its job
// is to convert Turn A's raw transcript (investigation notes, tool
// results, deterministic evidence) into STRUCTURED emit_* tool calls:
//
//   - emit_evidence          — refined / re-tagged evidence items
//   - emit_answer_symbol     — the answer-symbol slate with a
//                               required set-level completeness claim
//   - emit_hypothesis_verdict — per-hypothesis status + citation
//
// The extractor calls read_file / grep / repo_map NOT AT ALL. The
// LLM is explicitly forbidden from them at the prompt layer (Phase 8)
// and at the tool-schema layer (Phase 12). Every fact it emits must
// trace back to Turn A's snapshot.
//
// Phase 9 (this ship) introduces the cardinality validator that
// structurally closes UNRESOLVED #1 on the flag=on path: when the
// LLM claims CompletenessComplete but len(items) falls below
// max(Turn A's TerminalEvidenceCount, len(AnswerContract.MustInclude)),
// ParseOutput downgrades the claim to CompletenessLowerBound, logs a
// warning diagnosing the mismatch, and lets the finalizer render the
// softened floor prompt instead of the Translation-mode prompt. The
// original schema-level check from Session 1 still rejects completely
// malformed calls; this is the semantic second layer.
//
// ShouldStop is deliberately one-shot (iteration >= 1). The retry
// pattern the design doc originally sketched (two iterations with a
// mid-loop feedback hint) was dropped after audit because Turn B
// cannot read new files — a retry has no new information to work
// with. Downgrading to lower_bound is the honest terminal state, and
// the softened prompt gives the finalizer the latitude to add
// evidence-backed symbols above the floor. Retry would add LLM cost
// for an unprovable improvement in recall; waiting for grid data in
// the flip-default session is the correct evidence bar.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// extractorEvaluator is the Turn B evaluator. It is a separate type
// from explorerEvaluator so the two turns cannot accidentally share
// state (cross-run leakage was a real problem in pre-P2.1 code, see
// memory/project_repl_equivalence_audit.md).
type extractorEvaluator struct{}

// BuildInitialPrompt implements Evaluator.
//
// Phase 8: the real prompt reads TurnAArtifacts and bakes Turn A's
// transcript into the LLM context. The extractor never calls tools
// that would re-fetch files; everything it emits must trace back to
// the snapshot in this prompt. The prompt is organized as:
//
//  1. Role + user question (who and what to answer)
//  2. Tool contract (allowed emit_*, forbidden read_file / grep /
//     repo_map, schema of each emit_answer_symbol / emit_evidence /
//     emit_hypothesis_verdict call including the completeness claim
//     honesty contract)
//  3. Turn A transcript digest: investigation notes, read files,
//     prior deterministic evidence, flow findings, hypothesis set
//  4. Baseline floor: TerminalEvidenceCount + MustInclude with a
//     clear statement of what "complete" requires
//  5. Output requirements: emit one batched call per tool, no
//     narrative, no speculation
//
// The prompt builder degrades gracefully when any section is empty:
// a missing TurnAArtifacts (can happen on unit-test paths or
// first-session bootstrap) produces a "no transcript available"
// notice instead of panicking. This is a structural defense against
// a Session 2 wiring bug that forgets to populate the snapshot.
func (e *extractorEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	var b strings.Builder

	// -------- Role + question --------
	b.WriteString("# Extractor (Turn B) — structured synthesis of Turn A's investigation\n\n")
	b.WriteString("You are the extractor. Turn A (the explorer) has finished investigating the ")
	b.WriteString("user's question and handed you a frozen snapshot of its findings: the raw ")
	b.WriteString("assistant notes, the files it read, the structured evidence it extracted, the ")
	b.WriteString("dataflow chains it found, and — for enumeration / list-of-symbols questions — ")
	b.WriteString("a deterministic count of terminal-literal evidence items. Your job is to ")
	b.WriteString("convert that snapshot into STRUCTURED emit_* tool calls that the finalizer ")
	b.WriteString("will render as the user-visible answer. You do NOT investigate. You do NOT ")
	b.WriteString("speculate. You do NOT call read_file, grep, or repo_map — any fact you emit ")
	b.WriteString("must trace back to the snapshot below.\n\n")

	if ctx != nil && strings.TrimSpace(ctx.CurrentTask) != "" {
		fmt.Fprintf(&b, "## User question\n\n%s\n\n", strings.TrimSpace(ctx.CurrentTask))
		if desc := strings.TrimSpace(ctx.CurrentTaskDescription); desc != "" && desc != strings.TrimSpace(ctx.CurrentTask) {
			fmt.Fprintf(&b, "**Expanded description:** %s\n\n", desc)
		}
	}

	// -------- Tool contract --------
	b.WriteString("## Allowed tools — call ONLY these\n\n")
	b.WriteString("- **`emit_evidence`** — emit one or more refined evidence items from Turn A's ")
	b.WriteString("snapshot. One batched call per tool use; each item is `{kind, subject, object, ")
	b.WriteString("source, line_start, line_end?, condition?, summary?}`. `kind` is one of `direct`, ")
	b.WriteString("`conditional`, `registration`, `mechanism`, `relationship`, `absent`. `source` ")
	b.WriteString("must be a repo-relative path; blob paths inside the per-trace WorkDir are ")
	b.WriteString("REJECTED.\n")
	b.WriteString("- **`emit_answer_symbol`** — emit the structured answer slate for ")
	b.WriteString("list-of-symbols / enumeration / call-chain questions. ONE batched call with all ")
	b.WriteString("symbols. Each item is `{name, file, line, kind, chain?, rationale?}`. `line` ")
	b.WriteString("must be > 0 (never estimated). `kind` ∈ {function, method, type, struct, class, ")
	b.WriteString("const, var}. The call MUST ALSO carry a set-level `completeness` claim — see ")
	b.WriteString("the honesty contract below.\n")
	b.WriteString("- **`emit_hypothesis_verdict`** — for each hypothesis in the hypothesis set, ")
	b.WriteString("emit `{hypothesis_id, status, rationale, citation}`. `status` is one of ")
	b.WriteString("`confirmed`, `rejected`, `inconclusive`. `confirmed` and `rejected` REQUIRE a ")
	b.WriteString("`file:line` citation; `inconclusive` may omit it. If you cannot reach a ")
	b.WriteString("definitive verdict, choose `inconclusive` — do NOT pick `confirmed` without a ")
	b.WriteString("concrete cite.\n\n")
	b.WriteString("## Forbidden tools\n\n")
	b.WriteString("- **`read_file`** — all files you need are already in the snapshot; if something ")
	b.WriteString("is missing, emit what you have and mark the hypothesis `inconclusive`.\n")
	b.WriteString("- **`grep`** — same reason. No fresh search.\n")
	b.WriteString("- **`repo_map`** — same reason. No fresh structure queries.\n\n")

	// -------- Completeness contract --------
	b.WriteString("## emit_answer_symbol completeness honesty contract\n\n")
	b.WriteString("Every `emit_answer_symbol` call must declare a `completeness` value:\n\n")
	b.WriteString("- **`complete`** — you assert this list enumerates EVERY symbol that answers ")
	b.WriteString("the question. The finalizer will render it with a \"MUST NOT add or remove\" ")
	b.WriteString("directive. Before you claim complete, verify that `len(items)` is at least ")
	b.WriteString("`max(terminal_evidence_count, len(must_include))` from the baseline section ")
	b.WriteString("below. The extractor validates this cross-check: a short claim of `complete` is ")
	b.WriteString("AUTOMATICALLY downgraded to `lower_bound` with a warning.\n")
	b.WriteString("- **`lower_bound`** — these symbols are all confirmed present, but additional ")
	b.WriteString("symbols may also be part of the answer. The finalizer will render a softened ")
	b.WriteString("\"at least these, may add more\" prompt. This is the HONEST DEFAULT when Turn A ")
	b.WriteString("found more terminal evidence than you can confidently map to distinct symbols.\n")
	b.WriteString("- **`unknown`** — you investigated but cannot reach a definitive slate. The ")
	b.WriteString("finalizer will DROP the answer-symbol section entirely and fall back to a ")
	b.WriteString("shape-based prompt (step_list, explanation, etc.). Choose this for mechanism ")
	b.WriteString("questions, value questions where the answer is a literal rather than a set of ")
	b.WriteString("names, or when the evidence is genuinely ambiguous.\n\n")

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
		fmt.Fprintf(&b, "## Turn A transcript digest\n\n")

		// Investigation notes: up to 6 entries, trimmed for prompt length
		if len(ta.InvestigationNotes) > 0 {
			b.WriteString("### Investigation notes (per-iteration narrative)\n\n")
			max := len(ta.InvestigationNotes)
			if max > 6 {
				fmt.Fprintf(&b, "*(showing the 6 most recent of %d iterations)*\n\n", max)
				ta.InvestigationNotes = ta.InvestigationNotes[max-6:]
			}
			for i, note := range ta.InvestigationNotes {
				trimmed := strings.TrimSpace(note)
				if trimmed == "" {
					continue
				}
				if len(trimmed) > 1200 {
					trimmed = trimmed[:1200] + "…"
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

		// Deterministic evidence slate: top items only (the prompt has
		// no budget for hundreds of items and the LLM cannot absorb
		// them anyway; the Turn A extraction pipeline already ranked).
		if len(ta.EvidenceItems) > 0 {
			b.WriteString("### Deterministic evidence Turn A extracted\n\n")
			evMax := len(ta.EvidenceItems)
			if evMax > 24 {
				fmt.Fprintf(&b, "*(showing top 24 of %d ranked items)*\n\n", evMax)
				evMax = 24
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
				if len(summary) > 200 {
					summary = summary[:200] + "…"
				}
				fmt.Fprintf(&b, "- [%s] %s%s\n", ev.Kind, summary, cite)
			}
			b.WriteString("\n")
		}

		// Flow findings: source→sink chains from dataflow analysis
		if len(ta.FlowFindings) > 0 {
			b.WriteString("### Dataflow findings (source → sink chains)\n\n")
			ffMax := len(ta.FlowFindings)
			if ffMax > 10 {
				fmt.Fprintf(&b, "*(showing top 10 of %d)*\n\n", ffMax)
				ffMax = 10
			}
			for i := 0; i < ffMax; i++ {
				ff := ta.FlowFindings[i]
				fmt.Fprintf(&b, "- `%s` (confidence=%.2f)\n", strings.Join(ff.Path, " → "), ff.Confidence)
			}
			b.WriteString("\n")
		}

		// Baseline floor: the β + γ data for the completeness
		// cross-check. Echoed back so the LLM can self-compute the
		// threshold rather than trust its memory of the claim.
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

	// -------- Hypothesis set (if any) --------
	if ctx != nil && ctx.AnalysisIR != nil && len(ctx.AnalysisIR.HypothesisSet) > 0 {
		b.WriteString("## Hypotheses (emit a verdict for each)\n\n")
		for _, h := range ctx.AnalysisIR.HypothesisSet {
			fmt.Fprintf(&b, "- **%s** (%s): %s\n", h.ID, h.Status, strings.TrimSpace(h.Statement))
		}
		b.WriteString("\nFor each hypothesis above, call `emit_hypothesis_verdict` with its ")
		b.WriteString("`hypothesis_id`, a `status` (confirmed / rejected / inconclusive), a short ")
		b.WriteString("`rationale`, and a `citation` (required for confirmed/rejected).\n\n")
	}

	// -------- Output requirements --------
	b.WriteString("## Output requirements\n\n")
	b.WriteString("1. Call each emit_* tool AT MOST ONCE per tool name. Batch all items in a ")
	b.WriteString("single call per tool.\n")
	b.WriteString("2. Do NOT produce any free-form narrative — your entire output is the emit_* ")
	b.WriteString("tool calls. The finalizer reads the drained buffers, not your assistant text.\n")
	b.WriteString("3. Do NOT invent line numbers. If the transcript does not have a line for a ")
	b.WriteString("symbol, omit that symbol.\n")
	b.WriteString("4. Do NOT cite files outside the \"Files Turn A read\" list above.\n")
	b.WriteString("5. If the question is a mechanism / step-by-step / value / boolean question ")
	b.WriteString("where an answer-symbol list is not the right shape, SKIP emit_answer_symbol ")
	b.WriteString("entirely — the finalizer will use shape-based rendering. Do NOT fabricate an ")
	b.WriteString("answer-symbol list to fill the section.\n")

	return b.String()
}

// ShouldStop implements Evaluator.
//
// Turn B is one-shot: one LLM call produces the entire emit_* batch.
// No soft-stop, no continuation, no ReAct loop. Validation and
// downgrade happen in ParseOutput after the loop exits.
func (e *extractorEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return iteration >= 1
}

// ParseOutput implements Evaluator. This is where Phase 9's
// cardinality validator lives: drain the three emit_* buffers, check
// the answer-symbol claim against Turn A's baseline + the analyzer's
// MustInclude floor, downgrade on mismatch, and package everything
// into a StageOutput the orchestrator can merge.
func (e *extractorEvaluator) ParseOutput(ctx *types.AgentContext, _ []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	out := &StageOutput{
		Data: json.RawMessage(`{}`),
	}
	if ctx == nil || ctx.Mutable == nil {
		return out, nil
	}

	// Evidence drain — no validation, the schema already enforced
	// per-item integrity at tool-call time. Merging with prior-stage
	// items happens downstream in applyStageOutput.
	if items := ctx.Mutable.EmittedEvidence(); len(items) > 0 {
		out.EvidenceItems = items
	}

	// Answer-symbol drain + cardinality validator.
	syms, claim := ctx.Mutable.EmittedAnswerSymbols()
	if len(syms) > 0 {
		validatedClaim := validateCompletenessClaim(ctx, syms, claim)
		out.AnswerSymbols = syms
		out.AnswerSymbolCompleteness = validatedClaim
	}
	// If len(syms) == 0: leave both fields zero (CompletenessUnknown).
	// The builder drops the section and the finalizer falls back to
	// the shape-based prompt.

	// HypothesisVerdicts are drained by Phase 10's orchestrator hook,
	// not here, because MarkHypothesis needs to write through the IR
	// and the extractor's StageOutput has no IR pointer. The buffer
	// stays populated for the Phase 10 post-dispatch read.

	return out, nil
}

// validateCompletenessClaim is the P2.1 Phase 9 cardinality
// validator. It is the structural closure for UNRESOLVED #1
// (extractAnswerSymbols enumeration completeness gap): when the LLM
// claims "complete" but the emitted slate is smaller than the
// baseline Turn A produced OR smaller than the analyzer's MustInclude
// floor, the claim is downgraded to "lower_bound" and a warning is
// logged. The downgrade is the honest terminal state — the finalizer
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
	return NewBaseAgent(types.AgentExtractor, deps, &extractorEvaluator{})
}
