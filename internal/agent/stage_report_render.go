package agent

// P1.2 — deterministic StageReport rendering.
//
// Background. Before this file, the explorer's StageReport (which the
// finalizer reads as its "Prior Stage Findings" section) was the LLM's
// own synthesis prose: BaseAgent.Execute auto-captured the last
// assistant message into output.StageReport, and that prose flowed
// into BusContext.StageReports → finalizer prompt verbatim. The P1.2
// remediation audit called that channel the "free-text escape hatch"
// — the structured Evidence / AnswerChains /
// AnswerSymbols channel and the prose channel were both visible to
// the finalizer, with no single source of truth, so the finalizer
// could (and did) pick prose details that contradicted the structured
// fields. F9 (`scrubSiblingEvidenceBlocks`) was a band-aid that tried
// to scrub one specific shape of leak (sibling-file `## Evidence
// from <path>` blocks the LLM copied into its synthesis output).
//
// P1.2 closes the channel: the explorer's ParseOutput now sets
// out.StageReport explicitly to the byte-deterministic markdown
// produced by renderExplorerStageReport. BaseAgent.Execute's
// auto-capture (`if output.StageReport == ""`) is therefore skipped,
// and the synthesis LLM's prose never reaches the finalizer through
// the StageReport channel. F9 becomes structurally unnecessary and
// is deleted in the same commit.
//
// Design rules for this renderer:
//
//   1. Pure function over structured types — no LLM input, no
//      `interface{}`, no captured state.
//   2. Byte-deterministic — same inputs always produce the same
//      output (relied on by TestRenderExplorerStageReport_Deterministic
//      and by every grid run that wants a stable diff).
//   3. No word lists, no regex pattern matching, no language-specific
//      heuristics. The structured fields are the source of truth;
//      this file just formats them.
//   4. No dropping or filtering. Upstream filters F5..F8 already did
//      grounding/dedup/rank/scope. This renderer formats the survivors.
//      "Honesty over cleverness" — empty inputs render as an explicit
//      empty skeleton, not as silence.
//   5. The output is human-readable markdown so the finalizer LLM can
//      treat it as a normal context section, but the section headers
//      are stable so future automated diffing is possible.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderExplorerStageReport produces the canonical, byte-deterministic
// markdown that becomes BusContext.StageReports[explorer].Findings and
// is rendered by the prompt builder under "Prior Stage Findings". It
// reads only structured fields the explorer has already computed by
// the time ParseOutput runs.
//
// readFiles is the unique set of source paths the explorer touched
// during the ReAct loop, ordered for stable rendering. The caller is
// responsible for de-duplication; this function only sorts.
func renderExplorerStageReport(
	questionKind string,
	answerShape string,
	evidence []types.EvidenceItem,
	chains []types.AnswerChain,
	symbols []types.AnswerSymbol,
	findings []types.FlowFindingDigest,
	readFiles []string,
	isEnumeration bool,
) string {
	var b strings.Builder

	b.WriteString("## Investigation Summary\n")
	fmt.Fprintf(&b, "- question_kind: %s\n", emptyAsDash(questionKind))
	fmt.Fprintf(&b, "- answer_shape: %s\n", emptyAsDash(answerShape))
	fmt.Fprintf(&b, "- enumeration_query: %v\n", isEnumeration)
	fmt.Fprintf(&b, "- evidence_items: %d\n", len(evidence))
	fmt.Fprintf(&b, "- answer_chains: %d\n", len(chains))
	fmt.Fprintf(&b, "- answer_symbols: %d\n", len(symbols))
	fmt.Fprintf(&b, "- flow_findings: %d\n", len(findings))
	fmt.Fprintf(&b, "- files_read: %d\n", len(readFiles))
	b.WriteString("\n")

	// Primary Evidence: the top items in their post-F8 order, kept
	// short. The full evidence list still reaches the finalizer via
	// the structured Evidence Items channel (formatEvidenceItems in
	// internal/context/builder.go, top-18). This section is a
	// compact recap so the finalizer prompt does not need to
	// cross-reference two sections to see what the explorer found.
	if len(evidence) > 0 {
		b.WriteString("## Primary Evidence\n")
		// topN was 8 pre-2026-04-17. Widened to 12 together with the
		// mechanism-concrete ranking promotion in evidence.go so
		// programmatic cross-file mechanism items (e.g. buildToolSchemas
		// gating on SubAgents.Get) have enough exposure even when the
		// LLM emitted a dozen surface-overlap conditional items. The
		// extra ~4 bullets add ~400 bytes to the digest — negligible
		// relative to the extractor's 10KB+ budget.
		const topN = 12
		for i, ev := range evidence {
			if i >= topN {
				break
			}
			b.WriteString("- " + formatEvidenceLineForReport(ev) + "\n")
		}
		if len(evidence) > topN {
			fmt.Fprintf(&b, "- ... (+%d more in Structured Evidence section)\n", len(evidence)-topN)
		}
		b.WriteString("\n")
	}

	if len(chains) > 0 {
		b.WriteString("## Resolution Chains\n")
		// Directive prose previously duplicated as the top-level
		// "Ground Truth" section in context/builder.go. Consolidated
		// here so the same chain list isn't rendered twice with
		// different framings — the section is now the single
		// canonical home for deterministically-extracted answer chains.
		b.WriteString("These facts were extracted deterministically from source code and directly answer the question. " +
			"Use them as the primary basis for your answer — do NOT contradict or ignore them. " +
			"The rightmost hop of each chain (after the final `→`) is the ANSWER TERMINAL the question resolves to; " +
			"intermediate nodes are MECHANISM, not answer.\n\n")
		for _, c := range chains {
			b.WriteString("- " + renderAnswerChain(c) + "\n")
		}
		b.WriteString("\n")
	}

	if len(symbols) > 0 {
		b.WriteString("## Answer Symbols\n")
		for _, s := range symbols {
			if s.File != "" && s.Line > 0 {
				fmt.Fprintf(&b, "- %s (%s:%d)\n", s.Name, s.File, s.Line)
			} else if s.File != "" {
				fmt.Fprintf(&b, "- %s (%s)\n", s.Name, s.File)
			} else {
				fmt.Fprintf(&b, "- %s\n", s.Name)
			}
		}
		b.WriteString("\n")
	}

	if len(findings) > 0 {
		b.WriteString("## Dataflow Findings\n")
		for _, f := range findings {
			line := strings.Join(f.Path, " → ")
			if line == "" {
				line = f.ID
			}
			if f.UnsupportedReason != "" {
				line += " [unsupported: " + f.UnsupportedReason + "]"
			}
			b.WriteString("- " + line + "\n")
		}
		b.WriteString("\n")
	}

	if len(readFiles) > 0 {
		b.WriteString("## Files Read\n")
		sorted := append([]string(nil), readFiles...)
		sort.Strings(sorted)
		for _, f := range sorted {
			b.WriteString("- " + f + "\n")
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

// formatEvidenceLineForReport renders one EvidenceItem as a single
// markdown bullet for the Primary Evidence section. It reads only
// struct fields — never LLM-authored prose summary — so the output
// is stable across runs.
//
// Format: `[KIND] subject predicate object — source:line` with
// missing fields gracefully elided. GroundingStatus is surfaced as a
// trailing tag: recovered items get `[recovered]`, ungrounded items
// get `[UNGROUNDED]` so the finalizer can see the trust level
// attached to each cite.
//
// Session-8 strict rule: the StageReport is a cross-stage artifact
// (produced by explorer, consumed by Turn B + finalize), so non-
// Tier-1-grounded items show source WITHOUT LineStart — downstream
// LLMs cannot pick up a recovered line number that the finalizer
// grounder's stricter Tier 2 will later reject. Routed through
// types.EvidenceItem.DisplayLocation(true) for consistency with the
// context/builder.go renderers.
func formatEvidenceLineForReport(ev types.EvidenceItem) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("[%s]", ev.Kind))

	semantic := strings.TrimSpace(strings.Join(nonEmpty([]string{ev.Subject, ev.Predicate, ev.Object}), " "))
	if semantic == "" {
		semantic = strings.TrimSpace(ev.Summary)
	}
	if semantic != "" {
		parts = append(parts, semantic)
	}

	if loc := ev.DisplayLocation(true); loc != "" {
		parts = append(parts, "— "+loc)
		// Anchor-kind suffix disambiguates what the cited line is for
		// downstream consumers. Without this, "[relationship] X calls
		// Y — file:line" was previously read by the extractor LLM as
		// "X is defined at line", because the line is in fact the
		// call site (line text contains Y, not X). The tag makes the
		// semantic explicit so emit_answer_symbol items derived from
		// these evidence rows pick a real definition line, not the
		// call-site line.
		if tag := anchorKindDisplayTag(ev.AnchorKind); tag != "" {
			parts = append(parts, tag)
		}
	}

	switch ev.GroundingStatus {
	case types.GroundingRecovered:
		parts = append(parts, "[recovered — line stripped; read_file before citing]")
	case types.GroundingUngrounded:
		parts = append(parts, "[UNGROUNDED]")
	}

	return strings.Join(parts, " ")
}

// anchorKindDisplayTag returns the parenthetical suffix appended to
// each rendered evidence line. The tag tells downstream LLMs what the
// cited line is so they cannot mistake e.g. a call-site line for the
// caller's definition line. Empty AnchorKind (legacy items that
// pre-date the anchor contract) renders no tag — preserves backward
// compat for any producer that has not been updated.
func anchorKindDisplayTag(k types.AnchorKind) string {
	switch k {
	case types.AnchorCall:
		return "(call site)"
	case types.AnchorDefinition:
		return "(definition)"
	case types.AnchorCondition:
		return "(condition)"
	case types.AnchorReturn:
		return "(return)"
	case types.AnchorAssignment:
		return "(assignment)"
	case types.AnchorImport:
		return "(import)"
	}
	return ""
}

func emptyAsDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// renderAnswerChain flattens a typed AnswerChain to the same one-line
// display string the finalizer's Ground Truth section uses. Kept in
// sync with context/builder.go:renderAnswerChainForPrompt — if the
// two ever drift, an integration test should catch it since both
// write to the same `## Resolution Chains` markdown section reader.
//
// Format: `<summary> (<source>:<line>)` with sane fallbacks.
// Session-8: non-Tier-1 lines are stripped via DisplayLocation(true)
// — same cross-stage strictness as formatEvidenceLineForReport.
func renderAnswerChain(c types.AnswerChain) string {
	ev := c.Item
	display := ev.Summary
	if display == "" {
		display = fmt.Sprintf("[%s] %s %s %s", ev.Kind, ev.Subject, ev.Predicate, ev.Object)
	}
	if loc := ev.DisplayLocation(true); loc != "" {
		display += " (" + loc + ")"
	}
	return display
}
