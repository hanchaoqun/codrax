package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceQueryIOActivityGroup(g tracequery.IOActivityGroup) string {
	return fmt.Sprintf("layer=%s family=%s phase=%s dev=%s byte_caliber=%s", g.Layer, g.EndpointFamily, g.Phase, g.Dev, g.ByteCaliber)
}

func traceQueryIOActivityNumber(v *float64) string {
	if v == nil {
		return "unavailable"
	}
	return strconv.FormatFloat(*v, 'g', 12, 64)
}

func traceQueryIOActivityBytes(v *uint64) string {
	if v == nil {
		return "unavailable"
	}
	return strconv.FormatUint(*v, 10)
}

func traceQueryIOActivitySummary(g tracequery.IOActivityGroup) string {
	v := g.Values
	s := fmt.Sprintf("endpoint_events=%d known_size=%d unknown_size=%d invalid_size=%d overflow_size=%d known_bytes=%s byte_sum_overflow=%t",
		v.EventCount, v.KnownByteEventCount, v.UnknownByteEventCount, v.InvalidByteEventCount, v.OverflowByteEventCount, traceQueryIOActivityBytes(v.KnownBytes), v.BytesOverflow)
	if g.Rates != nil {
		s += fmt.Sprintf(" events_per_second=%s known_bytes_per_second=%s", traceQueryIOActivityNumber(&g.Rates.EventsPerSecond), traceQueryIOActivityNumber(g.Rates.KnownBytesPerSecond))
	} else {
		s += " rates=unavailable"
	}
	return s
}

func traceQueryIOActivityBasis(s *tracequery.IOActivityStats) string {
	if s.Window != nil {
		if s.Window.EndInclusive {
			return "observed_capture_range=" + traceQueryDisplaySeconds(s.Window.StartTs) + ".." + traceQueryDisplaySeconds(s.Window.EndTs) + "; capture-end endpoint included (not an explicit half-open query)"
		}
		return "selected_window=" + traceQueryDisplaySeconds(s.Window.StartTs) + ".." + traceQueryDisplaySeconds(s.Window.EndTs) + "; half-open endpoint timestamps"
	}
	return fmt.Sprintf("lines=%d..%d; continuous_time_denominator_unavailable=%s", s.LineStart, s.LineEnd, s.WindowUnavailableReason)
}

func writeTraceIOActivity(b *strings.Builder, s *tracequery.IOActivityStats) {
	if s == nil {
		return
	}
	fmt.Fprintf(b, "- io_activity %s; groups=%d omitted_groups=%d; supported_endpoints=%d rejected_endpoints=%d unresolved_sources=%d; capture completeness unknown\n",
		traceQueryIOActivityBasis(s), s.GroupCount, s.OmittedGroups, s.Coverage.SupportedEndpointCount, s.Coverage.RejectedEndpointCount, s.Coverage.UnresolvedSourceCount)
	for _, g := range s.Groups {
		fmt.Fprintf(b, "  - %s source=%q; %s; time_buckets=%d omitted_buckets=%d\n", traceQueryIOActivityGroup(g), g.SourcePath, traceQueryIOActivitySummary(g), g.BucketCount, g.OmittedBuckets)
	}
}

func traceQueryTypedIOActivityObservations(s *tracequery.IOActivityStats, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if s == nil {
		return nil
	}
	var out []types.ObservationRecord
	if s.Window != nil && s.Window.EndInclusive {
		// An implicitly closed capture extent is not the same population as
		// an explicit half-open query with identical numeric endpoints.
		ref.QueryWindowKnown = false
	}
	for i, g := range s.Groups {
		groupRef := ref
		groupRef.Path = g.SourcePath
		identity, _ := json.Marshal([]string{g.SourcePath, g.Layer, g.EndpointFamily, g.Phase, g.Dev, g.ByteCaliber})
		digest := sha256.Sum256(identity)
		notes := traceQueryTypedKVNotes([][2]string{
			{types.TraceNoteKeyIOActivityGroup, traceQueryIOActivityGroup(g)},
			{types.TraceNoteKeyIOActivityBasis, traceQueryIOActivityBasis(s)},
			{"storage_source_path", g.SourcePath},
			{types.TraceNoteKeyIOActivityScope, "all issuers; independently observed endpoint events, not unique requests or complete pairs; source/layer/phase/byte calibers are not additive; no target waiting or root-cause authority"},
		})
		if s.Window != nil && !s.Window.EndInclusive {
			notes = append(notes, types.TraceNoteKeySelectedWindow+"="+traceQueryDisplaySeconds(s.Window.StartTs)+".."+traceQueryDisplaySeconds(s.Window.EndTs))
		}
		r := types.ObservationRecord{ID: fmt.Sprintf("trace_query:%s#io_activity:%d", scope, i+1),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: groupRef,
			ClaimKey: "io_activity:" + hex.EncodeToString(digest[:]), Subject: g.Layer, Predicate: "io_activity", Object: g.EndpointFamily,
			Value: strconv.Itoa(g.Values.EventCount), Unit: "events", Summary: traceQueryIOActivitySummary(g), RichNotes: notes, ObservedAt: at, Confidence: 1}
		if note := traceQueryIOActivityReceipt(r, s, g); note != "" {
			r.RichNotes = append(r.RichNotes, note)
		}
		out = append(out, r)
	}
	// Even a rejected-only input needs honest coverage; it is not a device
	// zero-rate measurement and is deliberately not a selectable data table.
	out = append(out, types.ObservationRecord{ID: "trace_query:" + scope + "#io_activity_coverage:1",
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
		ClaimKey: "io_activity_coverage", Subject: "IO endpoint capture", Predicate: "io_activity_coverage",
		Summary:   fmt.Sprintf("supported_endpoints=%d rejected_endpoints=%d unresolved_sources=%d groups=%d omitted_groups=%d; capture completeness unknown", s.Coverage.SupportedEndpointCount, s.Coverage.RejectedEndpointCount, s.Coverage.UnresolvedSourceCount, s.GroupCount, s.OmittedGroups),
		RichNotes: traceQueryTypedKVNotes([][2]string{{types.TraceNoteKeyIOActivityBasis, traceQueryIOActivityBasis(s)}, {types.TraceNoteKeyIOActivityScope, strings.Join(s.Coverage.Reasons, "; ") + "; no absence or target-blocking proof"}}), ObservedAt: at, Confidence: 1})
	return out
}
