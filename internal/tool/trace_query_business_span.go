package tool

import (
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
			}),
			SupportRefs: traceQueryObservationSupportRefs(ref, span.StartLine, span.EndLine),
			ObservedAt:  at,
			Confidence:  0.95,
		})
	}
	return out
}
