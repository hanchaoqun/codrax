package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// renderAnswerDocQueriedIOMeasurementSemantics explains already-published IO
// measurements when a broad question classification has no dedicated IO
// handoff. It deliberately does not change fact-family matching, join another
// measurement family, fetch evidence, or add an answer/completion obligation.
// The caller supplies the same source/window-filtered ledger as other prompts.
func renderAnswerDocQueriedIOMeasurementSemantics(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	rm := &ctx.AnalysisIR.RequestModel
	if profile := rm.RuntimeQuestionProfile; profile != nil &&
		(profile.RequestsFactFamily(types.RuntimeQuestionFactIOLatency) || profile.Scope == types.RuntimeQuestionScopeCausalDiagnosis) {
		return "" // Existing finite-IO and causal-IO handoffs own these lanes.
	}
	var records []types.ObservationRecord
	for _, record := range ledger.Records {
		if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			record.GroundingPolicy != types.ClaimGroundingHard ||
			!answerDocBoundedRuntimeIOLatencyPredicate(strings.TrimSpace(record.Predicate)) {
			continue
		}
		// Same explicit-query-window protection as the causal IO display lane:
		// some producers carry scope in SourceRef rather than a selected_window
		// note. Never reintroduce an already-known wider query through this card.
		if requested := rm.RuntimeArtifactScopeProfile; requested != nil && requested.HasExplicitTimeWindows() &&
			record.SourceRef.QueryWindowKnown && !requested.ContainsExplicitTimeWindow(record.SourceRef.QueryWindowStartTs, record.SourceRef.QueryWindowEndTs) {
			continue
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return ""
	}
	// Reuse the existing bounded IO display capacity and coverage reservations.
	// Only exact duplicate publications coalesce; equal counts or physical
	// endpoints must not merge different receipts, windows, targets or versions.
	rows := answerDocRuntimeFactAuthorityRowsByKey(records, types.RuntimeQuestionFactIOLatency, rm, func(record types.ObservationRecord) string {
		encoded, _ := json.Marshal(record)
		return string(encoded)
	})
	var b strings.Builder
	b.WriteString("### Already-Queried IO Measurement Meanings\n\n")
	b.WriteString("- This card explains source measurements already queried; it is not a requested fact family, a new investigation requirement, a root-cause roster, or permission to broaden the answer. Each row retains its own source and query receipt; do not combine independent rows into a new population.\n")
	b.WriteString("- Query windows select records; observed intervals describe their original event spans. Full request residence is not clipped window occupancy and is not target blocking time. Only independently published completion closure can prove issuer blocking; a shared time window or IO family is not a causal link.\n")
	b.WriteString("- Different IO layers use their published event endpoints. There is no containment or subtraction relationship between RQ and BIO populations without an explicit per-request mapping; do not infer hardware, driver, or application phases from the layer name. Do not average group quantiles or reconstruct their population from the selected request details.\n")
	fmt.Fprintf(&b, "- rendered_source_rows=%d; this is a bounded explanatory display, not a census or proof of complete capture. Keep scope, measurement units and boundaries in natural reader language rather than exposing these audit keys.\n", len(rows))
	for _, record := range rows {
		fmt.Fprintf(&b, "  - %s\n", answerDocQueriedIOMeasurementMeaningRow(record, rm, extractAnswerDocLang(ctx)))
	}
	b.WriteByte('\n')
	return b.String()
}

func answerDocQueriedIOMeasurementMeaningRow(record types.ObservationRecord, rm *types.RequestModel, lang string) string {
	parts := []string{
		answerDocBoundedRuntimeFactAuthorityRow(record, rm, lang),
		fmt.Sprintf("source_path=%q", record.SourceRef.Path),
		fmt.Sprintf("query_result=%q", record.SourceRef.PayloadRef),
	}
	if record.ObservedAt != "" {
		parts = append(parts, fmt.Sprintf("observed_at=%q", record.ObservedAt))
	}
	if record.SourceRef.QueryTargetPID > 0 {
		parts = append(parts, fmt.Sprintf("query_target_pid=%d", record.SourceRef.QueryTargetPID))
	}
	for _, field := range [][2]string{
		{"query_target_thread", record.SourceRef.QueryTargetThread},
		{"query_target_scope", record.SourceRef.QueryTargetScope},
		{"time_domain", record.SourceRef.TimeDomain},
		{"canonical_time_domain", record.SourceRef.CanonicalTimeDomain},
	} {
		if field[1] != "" {
			parts = append(parts, fmt.Sprintf("%s=%q", field[0], field[1]))
		}
	}
	switch strings.TrimSpace(record.Predicate) {
	case "io_latency_coverage":
		coverage := types.TraceIODetailCoverageFromObservation(record)
		// The ordinary row already carries CompactMeaning, exactly once.
		parts = append(parts, strings.TrimSpace(strings.TrimPrefix(coverage.PromptMeaning(), coverage.CompactMeaning())))
	case "io_latency":
		caliber := traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyIORequestResidenceCaliber)
		if start, done := tracequery.IORequestResidenceEndpoints(caliber); start != "" {
			parts = append(parts, "request_endpoints="+start+"→"+done)
		} else {
			parts = append(parts, "request_endpoints=unknown (not inferred from the event name)")
		}
	case "storage_latency_by_layer":
		// These are producer-owned display notes. Their contents are copied,
		// not parsed as identity/causal authority or reconstructed from prose.
		for _, key := range []string{"storage_request_group", "io_request_scope", "storage_group_coverage"} {
			if value := traceQueryObservationSupplementNoteValue(record, key); value != "" {
				parts = append(parts, key+"="+value)
			}
		}
		var measurements []string
		for _, label := range []string{"samples", "min", "mean", "max", "p50", "p90", "p95", "p99"} {
			key := "io_request_" + label
			if label != "samples" {
				key += "_ms"
			}
			if value := traceQueryObservationSupplementNoteValue(record, key); value != "" {
				measurements = append(measurements, label+"="+value)
			}
		}
		if len(measurements) > 0 {
			parts = append(parts, "IO请求耗时(ms): "+strings.Join(measurements, " "))
		}
	}
	return strings.Join(parts, "; ")
}
