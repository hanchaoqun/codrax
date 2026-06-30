package types

import (
	"slices"
	"strings"
	"testing"
)

func TestTraceObservationCoveragePreservesDimensionsWindowsAndChainRelevance(t *testing.T) {
	records := []ObservationRecord{
		traceCoverageRecord("root", "trace_query:1", "root_cause_primary", "root_cause_primary", "target-1", "runnable", "12.000", []string{"chain_relevance=on_chain", "cumulative_impact_ms=12.000"}, ObservationSpan{StartTs: 10.0, EndTs: 10.5}),
		traceCoverageRecord("path", "trace_query:1", "wakeup_chain", "wakeup_chain:path", "target-1", "worker-2 -> target-1", "", []string{"path=worker-2 -> target-1"}, ObservationSpan{StartTs: 10.0, EndTs: 10.5}),
		traceCoverageRecord("blocking", "trace_query:2", "critical_blocking", "critical_blocking:binder_wait", "target-1", "binder_wait", "4.000", []string{"chain_relevance=adjacent", "peer=Binder:1"}, ObservationSpan{StartTs: 10.2, EndTs: 10.4}),
		traceCoverageRecord("io", "trace_query:2", "io_pressure", "io_pressure:block_latency", "io_pressure", "inode=7", "8.000", []string{"chain_relevance=background", "actual_window=9.900..10.600"}, ObservationSpan{}),
	}

	got := TraceObservationCoverageFromObservationRecords(records)
	if !got.Active {
		t.Fatal("coverage should be active")
	}
	if got.TotalRecords != 4 || got.QueryCount != 2 {
		t.Fatalf("coverage counts = records:%d queries:%d", got.TotalRecords, got.QueryCount)
	}
	if !slices.Contains(got.Windows, "10.000000..10.500000") ||
		!slices.Contains(got.Windows, "9.900..10.600") {
		t.Fatalf("windows should preserve span and note windows: %+v", got.Windows)
	}
	root := traceCoverageDimensionFor(got, TraceObservationDimensionRootCauseRank)
	if root.Count != 1 || root.OnChainCount != 1 {
		t.Fatalf("root dimension should preserve on-chain count: %+v", root)
	}
	blocking := traceCoverageDimensionFor(got, TraceObservationDimensionCriticalBlocking)
	if blocking.Count != 1 || blocking.AdjacentCount != 1 {
		t.Fatalf("blocking dimension should preserve adjacent count: %+v", blocking)
	}
	resource := traceCoverageDimensionFor(got, TraceObservationDimensionResourcePressure)
	if resource.Count != 1 || resource.BackgroundCount != 1 {
		t.Fatalf("resource dimension should preserve background count: %+v", resource)
	}
	if slices.Contains(got.SoftMissingDimensions, "wakeup_chain") ||
		slices.Contains(got.SoftMissingDimensions, "critical_blocking_calls") {
		t.Fatalf("observed wakeup/blocking dimensions should not be suggested missing: %+v", got.SoftMissingDimensions)
	}
	if len(got.TopObservations) == 0 || got.TopObservations[0].Dimension != TraceObservationDimensionRootCauseRank {
		t.Fatalf("top observations should start with root-cause rows: %+v", got.TopObservations)
	}
	if got.TopObservations[0].Filter == "" || !strings.Contains(got.TopObservations[0].Filter, "tool_call=trace_query:1") {
		t.Fatalf("top observation filter should keep tool call id: %+v", got.TopObservations[0])
	}
}

func TestTraceObservationCoverageSuggestsSoftFollowupsForSingleRootCauseDimension(t *testing.T) {
	got := TraceObservationCoverageFromObservationRecords([]ObservationRecord{
		traceCoverageRecord("root", "trace_query:1", "root_cause_primary", "root_cause_primary", "target-1", "runnable", "12.000", []string{"chain_relevance=on_chain"}, ObservationSpan{}),
	})
	for _, want := range []string{"wakeup_chain", "critical_blocking_calls", "thread_timeline_or_window_stats", "window_stats_resource_pressure"} {
		if !slices.Contains(got.SoftMissingDimensions, want) {
			t.Fatalf("soft missing dimensions missing %q: %+v", want, got.SoftMissingDimensions)
		}
	}
}

func traceCoverageRecord(id, toolCall, predicate, claimKey, subject, object, value string, notes []string, span ObservationSpan) ObservationRecord {
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane:  ObservationProvenanceObservedDirectCause,
		SourceRef: ObservationSourceRef{
			Kind:       ObservationSourceRuntimeArtifact,
			ToolCallID: toolCall,
			PayloadRef: "blob://trace/" + toolCall,
			RawRef:     "blob://trace/raw/" + toolCall,
		},
		Span:        span,
		ClaimKey:    claimKey,
		Subject:     subject,
		Predicate:   predicate,
		Object:      object,
		Value:       value,
		Unit:        "ms",
		Summary:     predicate + " " + subject,
		RichNotes:   notes,
		SupportRefs: []string{"trace.systrace:10-20"},
		Confidence:  0.9,
	}
}

func traceCoverageDimensionFor(coverage TraceObservationCoverage, dimension string) TraceObservationDimensionCoverage {
	for _, row := range coverage.Dimensions {
		if row.Dimension == dimension {
			return row
		}
	}
	return TraceObservationDimensionCoverage{}
}
