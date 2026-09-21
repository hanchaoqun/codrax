package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Ordinary B/E and async work intervals are measured facts even when their
// names do not identify a known optimization mechanism. Keep this channel
// separate from semantic families: it mints no chain, rank, or causal token.
// The common publication tail still binds query scope, artifact-local lines,
// clock provenance, and capacity coverage in exactly the same way as other
// trace_query observations.
func traceQueryTypedBusinessSpanObservations(stats tracequery.WindowStats, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	var out []types.ObservationRecord
	for _, span := range stats.TraceSpans {
		if len(out) >= traceQueryWidthTypedFamilyRowCap() {
			break
		}
		if strings.TrimSpace(span.SemanticClass) != "" || strings.TrimSpace(span.Name) == "" ||
			span.DurationMs <= 0 || math.IsNaN(span.DurationMs) || math.IsInf(span.DurationMs, 0) ||
			span.EndTs <= span.StartTs || span.StartLine <= 0 || span.EndLine < span.StartLine {
			continue
		}
		subject := traceThreadLabel(span.Thread)
		if subject == "" {
			continue
		}
		out = append(out, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:%s#trace_business_span:%d", scope, len(out)+1),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane:  types.ObservationProvenanceArtifactSpan,
			SourceRef:       ref,
			Span: types.ObservationSpan{
				LineStart: span.StartLine, LineEnd: span.EndLine,
				StartTs: span.StartTs, EndTs: span.EndTs,
			},
			ClaimKey:  types.TraceBusinessSpanPredicate + ":" + span.Name,
			Subject:   subject,
			Predicate: types.TraceBusinessSpanPredicate,
			Object:    span.Name,
			Value:     traceQueryObservationMSValue(span.DurationMs),
			Unit:      "ms",
			Summary: fmt.Sprintf("observed business span %q on %s: %.3fms in selected window; elapsed interval, not CPU execution or causal contribution",
				span.Name, subject, span.DurationMs),
			RichNotes: traceQueryTypedKVNotes([][2]string{
				{types.TraceNoteKeySpanName, span.Name},
				{types.TraceNoteKeySpanKind, firstNonEmptyTraceString(span.Kind, "sync")},
				{types.TraceNoteKeySpanCategory, span.Category},
				{types.TraceNoteKeySpanSubcategory, span.Subcategory},
				{types.TraceNoteKeySelectedWindow, traceQuerySelectedWindowNoteValue(stats.Window)},
				{types.TraceNoteKeyActualImpactMS, traceQueryObservationMSValue(span.ActualDurationMs)},
				{types.TraceNoteKeyActualWindow, traceQueryWindowValue(span.ActualStartTs, span.ActualEndTs)},
				{types.TraceNoteKeyBusinessSpanSchedulerStates, traceQueryBusinessSpanSchedulerNote(span)},
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, span.StartLine, span.EndLine),
			ObservedAt:  at,
			Confidence:  0.95,
		})
	}
	return out
}

func traceQueryBusinessSpanSchedulerNote(span tracequery.TraceSpanSummary) string {
	if span.Kind != "sync" || !span.SchedulerStates.Matches(span.SourcePath, traceThreadLabel(span.Thread), span.StartTs, span.EndTs) {
		return ""
	}
	data, err := json.Marshal(span.SchedulerStates)
	if err != nil {
		return ""
	}
	return string(data)
}

// Expose the same native account to the investigator that the finalizer
// receives via the typed note. A late-stage fact card alone cannot prevent
// exploration from substituting the wider query's totals for a marker.
func traceQueryBusinessSpanSchedulerSummary(span tracequery.TraceSpanSummary) string {
	states := span.SchedulerStates
	if span.Kind != "sync" || !states.Matches(span.SourcePath, traceThreadLabel(span.Thread), span.StartTs, span.EndTs) {
		return ""
	}
	prefix := fmt.Sprintf("marker_state_account %q owner=%s interval=%.6f..%.6f", span.Name, traceThreadLabel(span.Thread), span.StartTs, span.EndTs)
	if states.Coverage == "unavailable" {
		return prefix + " scheduler states unavailable, not zero; do not substitute wider-query totals"
	}
	coverage := "complete coverage"
	if states.Coverage == "partial" {
		coverage = "partial coverage; unobserved or unclassified time is not zero"
	}
	return prefix + fmt.Sprintf(" running=%.3fms runnable=%.3fms sleep=%.3fms d_state=%.3fms scheduler_marked_io_wait=%.3fms accounted=%.3fms (%s); sleep_iowait=%.3fms is included in sleep, not an addend; marker-local states, not wider-query totals; states do not prove a wait mechanism",
		states.RunningMs, states.RunnableMs, states.SleepMs, states.DStateMs, states.IOWaitMs, states.AccountedMs, coverage, states.SleepIOWaitMs)
}

func writeTraceBusinessSpanSchedulerPreview(b *strings.Builder, spans []tracequery.TraceSpanSummary, payloadRef string) {
	count, emitted := 0, 0
	for _, span := range spans {
		line := traceQueryBusinessSpanSchedulerSummary(span)
		if line == "" {
			continue
		}
		count++
		if emitted >= traceQueryWidthStateDrilldownSummaryCap() {
			continue
		}
		fmt.Fprintf(b, "- %s source=%s lines=%d-%d\n", line, traceQuerySourceBasename(span.SourcePath), span.StartLine, span.EndLine)
		emitted++
	}
	if count > emitted {
		fmt.Fprintf(b, "- marker-local state preview: %d of %d returned accounts shown; remaining accounts are in payload_ref=%s (not an all-trace inventory)\n", emitted, count, sanitizeForBanner(payloadRef))
	}
}
