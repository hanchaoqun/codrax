package types

import (
	"fmt"
	"sort"
	"strings"
)

// Unified observation-view budget constants (Batch E2/E3, 2026-07-02).
//
// The three observation-view layers used to carry three independent magic
// numbers — checkpoint ledger input 24 (agent tool-history checkpoints),
// extract/gate ledger input 128, and finalize prompt render 18 — plus an
// implicit trace-coverage top-5 render cap and package-local aggregate-fact
// render caps. This block is the single constant source: consumers MUST
// reference these constants instead of re-hardcoding literals so the layers
// cannot silently drift apart again. All of these are prompt/summary display
// budgets (soft guidance); none of them may be consumed by hard gates.
const (
	// ObservationCheckpointLedgerEvidenceLimit bounds evidence rows folded
	// into checkpoint-scale ledger views (the explorer tool-history prune
	// checkpoint). Checkpoint ORIGIN statistics must NOT use this bounded
	// view — they compile the full ledger (evidenceLimit = 0) so origin
	// counts reflect everything accepted, not the first N evidence rows.
	ObservationCheckpointLedgerEvidenceLimit = 32

	// ObservationExtractLedgerEvidenceLimit bounds evidence rows folded into
	// extract/gate-scale ledger views (runtime-source authority snapshots,
	// tier-1 floor, contract checks, closure fallbacks, parallel-dispatch
	// reconciliation).
	ObservationExtractLedgerEvidenceLimit = 128

	// ObservationPromptRecordLimit bounds typed observation records rendered
	// into the finalize (answer-writing) prompt's Observation Ledger section.
	// Kept symmetric with ObservationCheckpointLedgerEvidenceLimit so the
	// checkpoint view and the answer-writing view describe the same scale of
	// accepted observations.
	ObservationPromptRecordLimit = 32

	// AggregateFactsPromptDefaultLimit / AggregateFactsPromptMaxLimit bound
	// structured aggregate-fact rows rendered into prompts (base budget and
	// the ceiling after shape-based widening). Principal answer-payload facts
	// are never hidden by these caps — the answer-accuracy red line — the
	// caps bound auxiliary audit context only.
	AggregateFactsPromptDefaultLimit = 16
	AggregateFactsPromptMaxLimit     = 48

	// TraceCoverageTopObservationPromptLimit bounds the trace-coverage
	// top-observation rows rendered into the finalize prompt (Batch E3).
	// Renderers that apply this cap MUST disclose how many further trace
	// observations exist so a 100+ observation trace never looks like it
	// produced only five rows.
	TraceCoverageTopObservationPromptLimit = 5
)

// SummarizeDroppedObservationRecords renders a deterministic
// "origin[/principal]×count" listing for ledger records that were NOT rendered
// into a bounded prompt/checkpoint view, identified by record ID. It is
// advisory display text for "(showing N of M; dropped: ...)" truncation
// notices only — never a gate input. Categories are sorted by count
// (descending) then name so the largest dropped class is visible first.
func SummarizeDroppedObservationRecords(records []ObservationRecord, renderedIDs map[string]bool) string {
	if len(records) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, record := range records {
		if renderedIDs[strings.TrimSpace(record.ID)] {
			continue
		}
		key := string(record.Origin)
		if key == "" {
			key = string(AnswerEvidenceOriginUnknown)
		}
		if NormalizeAnswerAggregateRole(record.Role).IsPrincipal() {
			key += "/principal"
		}
		counts[key]++
	}
	return formatCategoryCounts(counts)
}

// formatCategoryCounts renders a category→count map as
// "name×count, name×count" with count-desc, name-asc ordering.
func formatCategoryCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s×%d", name, counts[name]))
	}
	return strings.Join(parts, ", ")
}
