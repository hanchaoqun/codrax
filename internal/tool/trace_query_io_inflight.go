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

// These are projections of the engine's admitted-pair account. They neither
// pair endpoints nor create a root-cause fact or a target-thread wait measure.
func traceQueryIOInFlightGroup(group tracequery.IOInFlightGroup) string {
	return fmt.Sprintf("layer=%s endpoint_family=%s dev=%s op=%s all_issuers",
		sanitizeForBanner(group.Layer), sanitizeForBanner(group.EndpointFamily),
		sanitizeForBanner(group.Dev), sanitizeForBanner(group.Operation))
}

func traceQueryIOInFlightSummary(group tracequery.IOInFlightGroup) string {
	if v := group.Values; v != nil {
		return fmt.Sprintf("IO在途 peak_requests=%d mean_requests=%.9g (requests); busy_ms=%.9g ms request_ms=%.9g request·ms; 完整配对请求=%d 范围内发起=%d; 非完成次数/覆盖率；非目标等待/因果",
			v.PeakRequests, v.MeanRequests, v.BusyMs, v.RequestMs, group.AcceptedPairCount, group.IssueCount)
	}
	return fmt.Sprintf("IO在途未测量 / unavailable (%s); 完整配对请求=%d 范围内发起=%d; 非完成次数/覆盖率；not measured zero or causal proof", group.ValuesUnavailableReason, group.AcceptedPairCount, group.IssueCount)
}

func traceQueryDisplaySeconds(value float64) string {
	// Preserve every observed fractional digit; six places is a minimum for
	// readability, not a rounding boundary that could collapse short spans.
	text := strconv.FormatFloat(value, 'f', -1, 64)
	whole, fraction, found := strings.Cut(text, ".")
	if !found {
		fraction = ""
	}
	if len(fraction) < 6 {
		fraction += strings.Repeat("0", 6-len(fraction))
	}
	return whole + "." + fraction
}

func traceQueryIOInFlightBasis(stats *tracequery.IOInFlightStats) string {
	window := "unknown"
	if w := stats.Window; w != nil {
		window = traceQueryDisplaySeconds(w.StartTs) + ".." + traceQueryDisplaySeconds(w.EndTs) + " seconds"
	}
	lines := ""
	if stats.LineStart > 0 || stats.LineEnd > 0 {
		lines = fmt.Sprintf("; lines=%d..%d", stats.LineStart, stats.LineEnd)
	}
	return "selected_window=" + window + lines + "; accepted_complete_pairs; no causal proof"
}

func traceQueryIOInFlightCoverageSummary(c tracequery.IOInFlightPairingCoverage) string {
	return fmt.Sprintf("%s %s: pairs=%d unpaired_start=%d unpaired_done=%d ambiguous_cohorts=%d pairing_suppressed=%d rejected=%d unresolved=%d; capture unknown",
		c.Family, c.Status, c.AcceptedPairCount, c.UnpairedStartCount, c.UnpairedDoneCount,
		c.AmbiguousCohortCount, c.PairingSuppressedCount, c.RejectedEndpointRows, c.UnresolvedSources)
}

func traceQueryIOInFlightGroupCoverage(stats *tracequery.IOInFlightStats, group tracequery.IOInFlightGroup) string {
	family := "storage"
	if group.Layer == "block" {
		family = "block"
	}
	status := "unknown"
	for _, c := range stats.Coverage {
		if c.Family == family {
			status = c.Status
		}
	}
	return fmt.Sprintf("%s pairing=%s; groups=%d omitted_groups=%d omitted_segments=%d; capture unknown",
		family, status, stats.GroupCount, stats.OmittedGroups, group.OmittedSegments)
}

// A compact prompt's note budget is smaller than the public timeline. Publish
// a bounded exact prefix with its own omission count, never silently clip an
// endpoint token or describe this prefix as the complete timeline.
func traceQueryIOInFlightTimeline(group tracequery.IOInFlightGroup) string {
	parts := make([]string, 0, len(group.Segments))
	for _, s := range group.Segments {
		part := fmt.Sprintf("[%s,%s):%d", traceQueryDisplaySeconds(s.StartTs), traceQueryDisplaySeconds(s.EndTs), s.Requests)
		if len(strings.Join(append(parts, part), ";")) > 90 {
			break
		}
		parts = append(parts, part)
	}
	return fmt.Sprintf("seconds:requests; omitted=%d; %s", group.OmittedSegments+len(group.Segments)-len(parts), strings.Join(parts, ";"))
}

func traceQueryIOInFlightNotes(stats *tracequery.IOInFlightStats, group tracequery.IOInFlightGroup) []string {
	notes := traceQueryTypedKVNotes([][2]string{
		{types.TraceNoteKeyIOInFlightGroup, traceQueryIOInFlightGroup(group)},
		{types.TraceNoteKeyIOInFlightBasis, traceQueryIOInFlightBasis(stats)},
		{types.TraceNoteKeyIOInFlightCoverage, traceQueryIOInFlightGroupCoverage(stats, group)},
		{"storage_source_path", group.SourcePath},
		{types.TraceNoteKeyIOInFlightTimeline, traceQueryIOInFlightTimeline(group)},
		{types.TraceNoteKeyIOInFlightScope, "values_unavailable=" + group.ValuesUnavailableReason + "; 完整配对区间裁入时间窗；行范围优先时无时间分母；未完成不补窗尾；跨层不可加；非目标等待"},
	})
	return append(notes, traceQueryIOInFlightWindowNotes(stats)...)
}

func traceQueryIOInFlightWindowNotes(stats *tracequery.IOInFlightStats) []string {
	if stats.Window == nil {
		return nil
	}
	// The existing finalizer scope projector consumes this precise producer
	// field, not the explanatory basis prose above. Preserve native precision.
	return []string{types.TraceNoteKeySelectedWindow + "=" + traceQueryDisplaySeconds(stats.Window.StartTs) + ".." + traceQueryDisplaySeconds(stats.Window.EndTs)}
}

func writeTraceIOInFlight(b *strings.Builder, stats *tracequery.IOInFlightStats) {
	if stats == nil {
		return
	}
	fmt.Fprintf(b, "- io_inflight %s; all_issuers; groups=%d omitted_groups=%d; window_unavailable=%s\n",
		traceQueryIOInFlightBasis(stats), stats.GroupCount, stats.OmittedGroups, sanitizeForBanner(stats.WindowUnavailableReason))
	for _, c := range stats.Coverage {
		fmt.Fprintf(b, "  - pairing coverage: %s; topology_complete=%t; reasons=%s\n",
			traceQueryIOInFlightCoverageSummary(c), c.TopologyComplete, sanitizeForBanner(strings.Join(c.Reasons, ",")))
	}
	for _, group := range stats.Groups {
		fmt.Fprintf(b, "  - %s source=%s — %s; omitted_segments=%d\n",
			traceQueryIOInFlightGroup(group), sanitizeForBanner(group.SourcePath), traceQueryIOInFlightSummary(group), group.OmittedSegments)
		for _, segment := range group.Segments {
			fmt.Fprintf(b, "    - [%s,%s) seconds: %d requests\n", traceQueryDisplaySeconds(segment.StartTs), traceQueryDisplaySeconds(segment.EndTs), segment.Requests)
		}
	}
}

func traceQueryTypedIOInFlightObservations(stats *tracequery.IOInFlightStats, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if stats == nil {
		return nil
	}
	var out []types.ObservationRecord
	for i, group := range stats.Groups {
		// The physical source is part of the group identity, not a representative
		// path copied from an arbitrary request or the bundle wrapper.
		groupRef := ref
		groupRef.Path = group.SourcePath
		identity, _ := json.Marshal([]string{group.SourcePath, group.Layer, group.EndpointFamily, group.Dev, group.Operation})
		digest := sha256.Sum256(identity)
		value := ""
		if group.Values != nil {
			value = strconv.Itoa(group.Values.PeakRequests)
		}
		out = append(out, types.ObservationRecord{
			ID:     fmt.Sprintf("trace_query:%s#io_inflight:%d", scope, i+1),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: groupRef,
			ClaimKey: "io_inflight:" + hex.EncodeToString(digest[:]), Subject: group.Layer,
			Predicate: "io_inflight", Object: group.EndpointFamily, Value: value, Unit: "requests",
			Summary: traceQueryIOInFlightSummary(group), RichNotes: traceQueryIOInFlightNotes(stats, group),
			ObservedAt: at, Confidence: .72,
		})
	}
	for i, c := range stats.Coverage {
		out = append(out, types.ObservationRecord{
			ID:     fmt.Sprintf("trace_query:%s#io_inflight_coverage:%d", scope, i+1),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
			ClaimKey: "io_inflight_coverage:" + c.Family, Subject: c.Family,
			Predicate: "io_inflight_coverage", Value: c.Status, Unit: "status",
			Summary: traceQueryIOInFlightCoverageSummary(c),
			RichNotes: append(traceQueryTypedKVNotes([][2]string{
				{types.TraceNoteKeyIOInFlightBasis, traceQueryIOInFlightBasis(stats)},
				{types.TraceNoteKeyIOInFlightCoverage, fmt.Sprintf("topology_complete=%t; groups=%d omitted_groups=%d; capture completeness unknown", c.TopologyComplete, stats.GroupCount, stats.OmittedGroups)},
				{types.TraceNoteKeyIOInFlightScope, "family-wide pairing diagnostics, not per-device exclusions or proof of target blocking"},
				{types.TraceNoteKeyIOInFlightReasons, strings.Join(c.Reasons, "; ")},
			}), traceQueryIOInFlightWindowNotes(stats)...), ObservedAt: at, Confidence: 1,
		})
	}
	return out
}
