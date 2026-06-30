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
//   4. No semantic re-ranking or LLM-style synthesis. Upstream filters
//      F5..F8 already did grounding/dedup/rank/scope. The renderer formats
//      the survivors and applies only the cross-stage trust boundary.
//      "Honesty over cleverness" — empty inputs render as an explicit
//      empty skeleton, not as silence.
//   5. The output is human-readable markdown so the finalizer LLM can
//      treat it as a normal context section, but the section headers
//      are stable so future automated diffing is possible.

import (
	"fmt"
	"sort"
	"strings"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
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
	questionFamily string,
	exactResolution *types.ExactResolutionContract,
	evidence []types.EvidenceItem,
	chains []types.AnswerChain,
	symbols []types.AnswerSymbol,
	findings []types.FlowFindingDigest,
	readFiles []string,
	isEnumeration bool,
	observations ...types.ObservationRecord,
) string {
	var b strings.Builder

	externalObservations := externalObservationsForStageReport(observations)

	b.WriteString("## Investigation Summary\n")
	fmt.Fprintf(&b, "- question_kind: %s\n", emptyAsDash(questionKind))
	fmt.Fprintf(&b, "- question_family: %s\n", emptyAsDash(questionFamily))
	fmt.Fprintf(&b, "- enumeration_query: %v\n", isEnumeration)
	fmt.Fprintf(&b, "- evidence_items: %d\n", len(evidence))
	fmt.Fprintf(&b, "- external_observations: %d\n", len(externalObservations))
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
	primaryEvidence := primaryEvidenceForReport(evidence)
	if len(primaryEvidence) > 0 {
		b.WriteString("## Primary Evidence\n")
		// topN was 8 pre-2026-04-17. Widened to 12 together with the
		// mechanism-concrete ranking promotion in evidence.go so
		// programmatic cross-file mechanism items (e.g. buildToolSchemas
		// gating on SubAgents.Get) have enough exposure even when the
		// LLM emitted a dozen surface-overlap conditional items. The
		// extra ~4 bullets add ~400 bytes to the digest — negligible
		// relative to the extractor's 10KB+ budget.
		const topN = 12
		for i, ev := range primaryEvidence {
			if i >= topN {
				break
			}
			b.WriteString("- " + formatEvidenceLineForReport(ev, exactResolution) + "\n")
		}
		if len(primaryEvidence) > topN {
			fmt.Fprintf(&b, "- ... (+%d more in %s section)\n", len(primaryEvidence)-topN, promptctx.SectionEvidencePool)
		}
		b.WriteString("\n")
	}

	if len(externalObservations) > 0 {
		b.WriteString("## External Observations\n")
		b.WriteString("These facts came from runtime/MCP/external observation lanes, not current-source citations. " +
			"`evidence_items: 0` only means no current-source EvidenceItem rows were emitted; it does not erase these external observations.\n\n")
		if coverage := renderTraceObservationCoverageForStageReport(types.TraceObservationCoverageFromObservationRecords(externalObservations)); coverage != "" {
			b.WriteString(coverage)
		}
		const topN = 10
		for i, record := range externalObservations {
			if i >= topN {
				break
			}
			b.WriteString("- " + formatObservationLineForReport(record) + "\n")
		}
		if len(externalObservations) > topN {
			fmt.Fprintf(&b, "- ... (+%d more external observations)\n", len(externalObservations)-topN)
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
			b.WriteString("- " + renderAnswerChain(c, exactResolution) + "\n")
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

func renderTraceObservationCoverageForStageReport(coverage types.TraceObservationCoverage) string {
	if !coverage.Active {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- trace_query_coverage: calls=%d observations=%d", coverage.QueryCount, coverage.TotalRecords)
	if len(coverage.Windows) > 0 {
		fmt.Fprintf(&b, " windows=`%s`", strings.Join(coverage.Windows, "`, `"))
	}
	if len(coverage.SoftMissingDimensions) > 0 {
		fmt.Fprintf(&b, " soft_followups=`%s`", strings.Join(coverage.SoftMissingDimensions, "`, `"))
	}
	b.WriteByte('\n')
	if len(coverage.Dimensions) > 0 {
		parts := make([]string, 0, len(coverage.Dimensions))
		for _, dim := range coverage.Dimensions {
			item := fmt.Sprintf("%s:%d", dim.Dimension, dim.Count)
			if dim.OnChainCount > 0 || dim.AdjacentCount > 0 || dim.BackgroundCount > 0 {
				item += fmt.Sprintf("(on=%d adjacent=%d background=%d)", dim.OnChainCount, dim.AdjacentCount, dim.BackgroundCount)
			}
			parts = append(parts, item)
		}
		fmt.Fprintf(&b, "- trace_query_dimensions: %s\n", strings.Join(parts, ", "))
	}
	for i, obs := range coverage.TopObservations {
		if i >= 4 {
			break
		}
		fmt.Fprintf(&b, "- trace_query_top[%d]: dimension=%s id=%s", i+1, obs.Dimension, obs.ID)
		if obs.ChainRelevance != "" {
			fmt.Fprintf(&b, " chain_relevance=%s", obs.ChainRelevance)
		}
		if obs.Window != "" {
			fmt.Fprintf(&b, " window=%s", obs.Window)
		}
		if obs.Filter != "" {
			fmt.Fprintf(&b, " filter=%q", obs.Filter)
		}
		if obs.Value != "" {
			fmt.Fprintf(&b, " value=%q", obs.Value)
		}
		if obs.DrilldownSource != "" {
			fmt.Fprintf(&b, " drilldown_source=%s", obs.DrilldownSource)
		}
		if len(obs.RecommendedViews) > 0 {
			fmt.Fprintf(&b, " recommended_views=`%s`", strings.Join(obs.RecommendedViews, "`, `"))
		}
		if obs.Dimension == types.TraceObservationDimensionStateDrilldown {
			fmt.Fprintf(&b, " chain_required=%t recursive=%t", obs.ChainRequired, obs.RecursiveDrilldown)
		} else {
			if obs.ChainRequired {
				b.WriteString(" chain_required=true")
			}
			if obs.RecursiveDrilldown {
				b.WriteString(" recursive=true")
			}
		}
		if obs.Summary != "" {
			fmt.Fprintf(&b, " summary=%q", obs.Summary)
		}
		if len(obs.SupportRefs) > 0 {
			fmt.Fprintf(&b, " support_refs=`%s`", strings.Join(obs.SupportRefs, "`, `"))
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String()
}

func primaryEvidenceForReport(items []types.EvidenceItem) []types.EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]types.EvidenceItem, 0, len(items))
	for _, item := range items {
		if !item.IsCitable() {
			continue
		}
		switch item.Kind {
		case types.EvidenceUnresolved, types.EvidenceTruncated:
			continue
		}
		out = append(out, item)
	}
	return out
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
func formatEvidenceLineForReport(ev types.EvidenceItem, exactResolution *types.ExactResolutionContract) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("[%s]", ev.Kind))

	semantic := types.EvidenceDeterministicSurfaceText(ev, false)
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
	case types.AnchorInitializer:
		return "(initializer)"
	case types.AnchorImport:
		return "(import)"
	case types.AnchorStringLiteral:
		return "(string literal)"
	}
	return ""
}

func externalObservationsForStageReport(records []types.ObservationRecord) []types.ObservationRecord {
	if len(records) == 0 {
		return nil
	}
	out := make([]types.ObservationRecord, 0, len(records))
	for _, record := range records {
		if record.Origin == types.AnswerEvidenceOriginCurrentSource ||
			record.SourceRef.Kind == types.ObservationSourceCurrentSource {
			continue
		}
		if strings.TrimSpace(record.Summary) == "" &&
			strings.TrimSpace(record.RawExcerpt) == "" &&
			strings.TrimSpace(record.Value) == "" {
			continue
		}
		out = append(out, record)
	}
	sort.SliceStable(out, func(i, j int) bool {
		li, lj := observationLineKey(out[i]), observationLineKey(out[j])
		if li != lj {
			return li < lj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func observationLineKey(record types.ObservationRecord) int {
	if record.Span.LineStart > 0 {
		return record.Span.LineStart
	}
	if record.Span.Row > 0 {
		return record.Span.Row
	}
	return 1 << 30
}

func formatObservationLineForReport(record types.ObservationRecord) string {
	var parts []string
	origin := strings.TrimSpace(string(record.Origin))
	if origin == "" {
		origin = strings.TrimSpace(string(record.SourceRef.Kind))
	}
	if origin != "" {
		parts = append(parts, "["+origin+"]")
	}
	if ref := observationReportRef(record); ref != "" {
		parts = append(parts, ref)
	}
	text := strings.TrimSpace(record.Summary)
	if text == "" {
		text = strings.TrimSpace(record.RawExcerpt)
	}
	if text == "" {
		text = strings.TrimSpace(record.Value)
	}
	if text != "" {
		parts = append(parts, "— "+singleLine(text))
	}
	if record.Confidence > 0 {
		parts = append(parts, fmt.Sprintf("(confidence %.2f)", record.Confidence))
	}
	return strings.Join(parts, " ")
}

func observationReportRef(record types.ObservationRecord) string {
	ref := stageReportFirstNonEmptyString(
		strings.TrimSpace(record.SourceRef.ResourceURI),
		strings.TrimSpace(record.SourceRef.Path),
		strings.TrimSpace(record.SourceRef.RawRef),
		strings.TrimSpace(record.SourceRef.PayloadRef),
		strings.TrimSpace(record.SourceRef.RowSetRef),
		strings.TrimSpace(record.SourceRef.ToolCallID),
	)
	if ref == "" {
		return ""
	}
	switch {
	case record.Span.LineStart > 0 && record.Span.LineEnd > record.Span.LineStart:
		return fmt.Sprintf("%s:%d-%d", ref, record.Span.LineStart, record.Span.LineEnd)
	case record.Span.LineStart > 0:
		return fmt.Sprintf("%s:%d", ref, record.Span.LineStart)
	case record.Span.Row > 0:
		return fmt.Sprintf("%s row %d", ref, record.Span.Row)
	default:
		return ref
	}
}

func stageReportFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
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
func renderAnswerChain(c types.AnswerChain, exactResolution *types.ExactResolutionContract) string {
	ev := c.Item
	display := types.EvidenceDeterministicSurfaceText(ev, false)
	if loc := ev.DisplayLocation(true); loc != "" {
		display += " (" + loc + ")"
	}
	return display
}
