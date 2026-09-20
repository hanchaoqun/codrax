package types

import (
	"fmt"
	"strconv"
	"strings"
)

// TraceIODetailCoverage describes one native query's accepted exact pairs and
// detail selection. It does not describe capture/scan completeness, currently
// projected prompt rows, failed pairing, group display capacity, or causality.
// Pointers keep an absent legacy counter distinct from a published zero.
// This is a display-only value: it is not a serialized observation authority.
type TraceIODetailCoverage struct {
	AcceptedPairs   *int
	SelectedDetails *int
	BeyondDetailCap *int
}

// TraceIODetailCoverageFromObservation reads only the producer's typed note
// keys. Missing, invalid, or conflicting copies remain unknown; no counter is
// reconstructed by subtraction or inferred from a coverage-status enum.
func TraceIODetailCoverageFromObservation(record ObservationRecord) TraceIODetailCoverage {
	if record.Predicate != "io_latency_coverage" || record.Subject != "block_request_pairs" || record.Unit != "requests" {
		return TraceIODetailCoverage{}
	}
	coverage := TraceIODetailCoverage{
		AcceptedPairs:   traceIODetailCoverageCount(record.RichNotes, TraceNoteKeyTotal),
		SelectedDetails: traceIODetailCoverageCount(record.RichNotes, TraceNoteKeyIOCoverageEmitted),
		BeyondDetailCap: traceIODetailCoverageCount(record.RichNotes, TraceNoteKeyIOOverflowPairs),
	}
	if coverage.AcceptedPairs != nil {
		if raw := strings.TrimSpace(record.Value); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value != *coverage.AcceptedPairs {
				coverage.AcceptedPairs = nil
			}
		}
		if coverage.AcceptedPairs != nil && record.ResultCount != nil && *record.ResultCount != *coverage.AcceptedPairs {
			coverage.AcceptedPairs = nil
		}
	}
	return coverage
}

func traceIODetailCoverageCount(notes []string, key string) *int {
	var count *int
	for _, note := range notes {
		name, raw, found := strings.Cut(strings.TrimSpace(note), "=")
		if !found || name != key {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || value < 0 || (count != nil && *count != value) {
			return nil
		}
		count = &value
	}
	return count
}

// CompactMeaning puts the numeric population and its detail-only boundary
// before summary clipping. E counts native query-selected details, not the
// potentially smaller set of observation/prompt rows after later projection.
func (coverage TraceIODetailCoverage) CompactMeaning() string {
	if coverage.AcceptedPairs == nil || coverage.SelectedDetails == nil || coverage.BeyondDetailCap == nil ||
		*coverage.AcceptedPairs < 0 || *coverage.SelectedDetails < 0 || *coverage.BeyondDetailCap < 0 ||
		*coverage.SelectedDetails > *coverage.AcceptedPairs ||
		*coverage.BeyondDetailCap != *coverage.AcceptedPairs-*coverage.SelectedDetails {
		return "Request-detail counts are incomplete or inconsistent; no reconciled pair population is asserted; not capture or scan completeness."
	}
	return fmt.Sprintf("Accepted complete pairs=%d; query-selected details=%d; paired details beyond cap=%d (not failed/missing pairs); not capture or scan completeness.",
		*coverage.AcceptedPairs, *coverage.SelectedDetails, *coverage.BeyondDetailCap)
}

// PromptMeaning is shared by query publication and the finalizer relation
// bridge. Each group distribution keeps its own scope and accepted samples;
// this detail cap neither truncates that statistic nor proves it is present.
func (coverage TraceIODetailCoverage) PromptMeaning() string {
	return coverage.CompactMeaning() + " These are native query detail counts, not current prompt row counts. " +
		"Pairs beyond the detail cap are already complete, not failed or missing-endpoint pairs. " +
		"Published group distributions use their own admitted samples independently of this detail cap; absent distributions remain unknown. " +
		"Unpaired/ambiguous counts need their own counters; neither pair count nor request residence proves target blocking or a root cause."
}
