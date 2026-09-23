package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderAnswerDocCausalIOMeasurements preserves already-published IO rulers
// when causal questions use the compact runtime ledger. The caller passes the
// same selected-window-filtered ledger used by all other finalizer authorities.
// Finite fact questions retain their existing dedicated contract unchanged.
// This is evidence display only: no query, join, sum, rank, or cause is added.
func renderAnswerDocCausalIOMeasurements(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	rm := &ctx.AnalysisIR.RequestModel
	if rm.RuntimeQuestionProfile == nil || rm.RuntimeQuestionProfile.Scope != types.RuntimeQuestionScopeCausalDiagnosis {
		return ""
	}
	var records []types.ObservationRecord
	for _, record := range ledger.Records {
		// Some exact IO/storage records carry their query window in SourceRef
		// rather than selected_window notes. Honor that existing typed scope as
		// well; never reintroduce a known wider query through this extra view.
		if requested := rm.RuntimeArtifactScopeProfile; requested != nil && requested.HasExplicitTimeWindows() &&
			record.SourceRef.QueryWindowKnown && !requested.ContainsExplicitTimeWindow(record.SourceRef.QueryWindowStartTs, record.SourceRef.QueryWindowEndTs) {
			continue
		}
		if record.Origin == types.AnswerEvidenceOriginRuntimeArtifact &&
			types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) &&
			record.GroundingPolicy == types.ClaimGroundingHard &&
			answerDocBoundedRuntimeIOLatencyPredicate(strings.TrimSpace(record.Predicate)) {
			records = append(records, record)
		}
	}
	if len(records) == 0 {
		return ""
	}
	rows := answerDocRuntimeFactAuthorityRowsByKey(records, types.RuntimeQuestionFactIOLatency, rm, answerDocCausalIOScopedKey)
	var b strings.Builder
	b.WriteString("### IO Measurements For Causal Interpretation\n\n")
	b.WriteString("- These are already-published source observations, not new root-cause candidates. One request's issue-to-complete residence is that request's elapsed time, not the issuing thread's wait. An issuer-blocked interval is proven only by its own completion closure and belongs to that issuer; attributing it to another thread's response still requires an independent causal chain. Keep each source, selected window, subject, interval, and measurement ruler together.\n")
	b.WriteString("- Target ownership is not a root-cause verdict. Rows marked selected_window_context are not automatically attributed to the named target: keep them as comparison/background unless independent dependency evidence places that issuer's wait on the target's causal chain. A longer background request cannot replace an on-chain wait. Missing closure is unproven, not proof of zero IO. Do not blindly add overlapping request-residence and issuer-wait intervals or totals from different threads or requests. For the same thread and window, mutually exclusive adjacent scheduler states may be summed using the published native state account; sleep plus runnable waiting can describe off-CPU time. This does not make request residence equivalent to issuer blocking or establish a cross-thread response contribution. Translate these audit fields into business language while keeping the conclusion model-authored.\n")
	b.WriteString("- A source request or completion-closed interval may extend beyond its query window. Its full physical elapsed time is not a clipped window total. Preserve both ranges; use a separately published window account for in-window occupancy. The same physical request witnessed in two requested windows remains one request, not two additive operations.\n")
	fmt.Fprintf(&b, "- rendered_source_rows=%d; this bounded display is not a request census. Use each published coverage row for its own scope, not for a different target.\n", len(rows))
	if len(rows) < len(records) {
		b.WriteString("- Repeated or additional source rows are omitted from this compact display; their original observations remain in the ledger.\n")
	}
	for _, row := range rows {
		fmt.Fprintf(&b, "  - %s; source_path=%q; query_scope_id=%q; selected_window=%q; query_window=%q\n",
			answerDocBoundedRuntimeFactAuthorityRow(row, rm, extractAnswerDocLang(ctx)),
			row.SourceRef.Path, row.SourceRef.QueryScopeID, traceQueryObservationSupplementNoteValue(row, types.TraceNoteKeySelectedWindow), answerDocCausalIOQueryWindow(row))
	}
	b.WriteByte('\n')
	return b.String()
}

func answerDocCausalIOQueryWindow(record types.ObservationRecord) string {
	if !record.SourceRef.QueryWindowKnown {
		return "unknown"
	}
	return strconv.FormatFloat(record.SourceRef.QueryWindowStartTs, 'g', -1, 64) + ".." +
		strconv.FormatFloat(record.SourceRef.QueryWindowEndTs, 'g', -1, 64)
}

// Do not collapse independent windows just because they witnessed the same
// physical request. The per-call record/query IDs are intentionally excluded:
// an identical repeated query still has only one displayed request witness.
func answerDocCausalIOScopedKey(record types.ObservationRecord) string {
	parts := []string{
		answerDocBoundedRuntimeFactPhysicalKey(record),
		traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeySelectedWindow),
		answerDocCausalIOQueryWindow(record),
		strconv.Itoa(record.SourceRef.QueryTargetPID), record.SourceRef.QueryTargetThread, record.SourceRef.QueryTargetScope,
		strconv.FormatBool(record.SourceRef.QueryLineRangeKnown), strconv.Itoa(record.SourceRef.QueryLineStart), strconv.Itoa(record.SourceRef.QueryLineEnd),
		record.SourceRef.TimeDomain, record.SourceRef.CanonicalTimeDomain, record.SourceRef.ClockAlignment,
		strconv.FormatBool(record.SourceRef.ClockCalibrated),
		record.Value, record.Unit,
		traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyIOCompletionWokeIssuer),
		traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyIOIssuerBlockedStart),
		traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyIOIssuerBlockedEnd),
		traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyIOIssuerBlocked),
	}
	for _, value := range []*float64{record.SourceRef.ClockOffsetSec, record.SourceRef.ClockSlope} {
		text := "unknown"
		if value != nil {
			text = strconv.FormatFloat(*value, 'g', -1, 64)
		}
		parts = append(parts, text)
	}
	encoded, _ := json.Marshal(parts) // A slice of strings always marshals.
	return string(encoded)
}
