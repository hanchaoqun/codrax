package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A grouped occupancy account has a different ruler from individual IO
// residence/closed-wait witnesses. Keep its bounded roster beside, not inside,
// that account: neither more requests nor more device groups may evict the
// other's measurements. This display never grants target or causal ownership.
func answerDocIOInFlightMeasurementRows(ledger types.ObservationLedger, rm *types.RequestModel) ([]types.ObservationRecord, int) {
	const limit = 10
	var records []types.ObservationRecord
	for _, r := range ledger.Records {
		if r.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) ||
			r.GroundingPolicy != types.ClaimGroundingHard ||
			(r.Predicate != "io_inflight" && r.Predicate != "io_inflight_coverage") {
			continue
		}
		if rm != nil && rm.RuntimeArtifactScopeProfile != nil && rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() &&
			r.SourceRef.QueryWindowKnown && !rm.RuntimeArtifactScopeProfile.ContainsExplicitTimeWindow(r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs) {
			continue
		}
		records = append(records, r)
	}
	var selected []types.ObservationRecord
	seen := map[string]bool{}
	add := func(r types.ObservationRecord) {
		// Coalesce only identical publications; different source receipts,
		// windows, population, coverage or values remain independent.
		key, _ := json.Marshal(r)
		if len(selected) < limit && !seen[string(key)] {
			seen[string(key)] = true
			selected = append(selected, r)
		}
	}
	// Reserve one representative of each pairing family, then measured
	// groups. Repeated independent query diagnostics must not consume the
	// entire display before any values appear; omitted receipts stay explicit.
	for _, family := range []string{"block", "storage"} {
		for _, r := range records {
			if r.Predicate == "io_inflight_coverage" && r.Subject == family {
				add(r)
				break
			}
		}
	}
	for _, r := range records {
		if r.Predicate == "io_inflight" {
			add(r)
		}
	}
	for _, r := range records {
		add(r)
	}
	return selected, len(records) - len(selected)
}

func renderAnswerDocIOInFlightMeasurements(ledger types.ObservationLedger, rm *types.RequestModel, lang string) string {
	rows, omitted := answerDocIOInFlightMeasurementRows(ledger, rm)
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("#### IO In-Flight Measurements\n\n")
	b.WriteString("- These are source-window resource measurements, not IO latency or target blocking. Each group covers all issuers on its own physical source, layer, endpoint family, device and operation. Depth and residence use admitted complete pairs; starts inside the query have their separate endpoint population and can include unpaired or ambiguous starts. Never add groups across layers or turn a larger depth into a root cause. Only independent dependency evidence can establish response impact.\n")
	b.WriteString("- Depth counts requests on half-open intervals clipped to the selected window; simultaneous completions and starts are processed together. Mean depth uses the full window including idle time. Busy time is the union with at least one admitted request; request·ms is depth integrated over time, not wall-clock delay. Start counts are arrivals, not depth. Missing/ambiguous pairs are excluded from depth and residence, not extended to the window end. A missing value is not measured zero; available pairing is not proof of complete capture. Translate audit labels into reader language.\n")
	fmt.Fprintf(&b, "- rendered_source_rows=%d omitted_source_rows=%d; retain each row's separate pairing coverage and display omissions.\n", len(rows), omitted)
	for _, r := range rows {
		parts := []string{answerDocBoundedRuntimeFactAuthorityRow(r, rm, lang),
			fmt.Sprintf("source_path=%q; query_result=%q", r.SourceRef.Path, r.SourceRef.PayloadRef)}
		fmt.Fprintf(&b, "  - %s\n", strings.Join(parts, "; "))
	}
	b.WriteByte('\n')
	return b.String()
}

// One display projection serves finite and causal IO accounts. It copies
// producer fields, never parses prose to join populations or infer authority.
func answerDocIOInFlightDisplayParts(r types.ObservationRecord) []string {
	parts := []string{fmt.Sprintf("summary=%q", r.Summary)}
	for _, key := range []string{types.TraceNoteKeyIOInFlightGroup, types.TraceNoteKeyIOInFlightBasis,
		types.TraceNoteKeyIOInFlightCoverage, types.TraceNoteKeyIOInFlightTimeline,
		types.TraceNoteKeyIOInFlightScope, types.TraceNoteKeyIOInFlightReasons} {
		if value := traceQueryObservationSupplementNoteValue(r, key); value != "" {
			parts = append(parts, fmt.Sprintf("%s=%q", key, value))
		}
	}
	return parts
}
