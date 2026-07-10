package tracequery

import "testing"

// ENG audit #4c (P3, §29.25 处置委托 2026-07-10): counter series identity used
// a fail-OPEN provenance fallback — when a populated composite ledger could
// not resolve a row to exactly one physical artifact, the shared idx.Path was
// used as the series source identity, so lookalike counters from different
// artifacts could join one series (contradicting the function's own comment
// and the tracePairingSourceIdentity discipline every other pairing lane
// follows). The counter lane now fails closed on the same shape.

func TestTraceCounterProvenanceFailsClosedOnUnresolvedCompositeRow(t *testing.T) {
	idx := &Index{Path: "merged.trace", TraceArtifacts: []TraceArtifactSource{
		{SourcePath: "a.systrace", CausalCompatible: true, VirtualLineBase: 0, LocalLineCount: 10},
		{SourcePath: "b.systrace", CausalCompatible: true, VirtualLineBase: 10, LocalLineCount: 10},
	}}
	if coord, ok := traceCounterSourceCoordinateForLine(idx, 25); ok {
		t.Fatalf("populated provenance ledger that cannot resolve the row must fail closed, not fall back to the shared index path: %+v", coord)
	}
	// Exactly-one-span resolution unchanged.
	if coord, ok := traceCounterSourceCoordinateForLine(idx, 5); !ok || coord.path != "a.systrace" || coord.localLine != 5 {
		t.Fatalf("single-span resolution must keep exact physical coordinates: ok=%v coord=%+v", ok, coord)
	}
	// Single-file compatibility lane unchanged.
	single := &Index{Path: "one.trace"}
	if coord, ok := traceCounterSourceCoordinateForLine(single, 3); !ok || coord.path != "one.trace" || coord.localLine != 3 {
		t.Fatalf("path-less/single-file compatibility lane must stay open: ok=%v coord=%+v", ok, coord)
	}
}

func TestTraceCounterUnresolvedRowExcludedAndDisclosed(t *testing.T) {
	idx := &Index{Path: "merged.trace",
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: "a.systrace", CausalCompatible: true, VirtualLineBase: 0, LocalLineCount: 10},
			{SourcePath: "b.systrace", CausalCompatible: true, VirtualLineBase: 10, LocalLineCount: 10},
		},
		Events: []Event{
			// Resolvable counter sample in artifact a.
			{Line: 5, Ts: 1.0, Type: EventTraceMark, PID: 10, SpanAction: "C", SpanPID: 10, SpanName: "HeapAlloc", SpanValue: "100"},
			// A row the populated ledger cannot map to one artifact.
			{Line: 25, Ts: 1.1, Type: EventTraceMark, PID: 10, SpanAction: "C", SpanPID: 10, SpanName: "HeapAlloc", SpanValue: "200"},
		},
		FirstTs: 1.0, LastTs: 1.1, TimestampOrder: TraceTimestampOrderMonotonic}
	rows, quality := computeCounterDeltas(idx, Query{TimeStart: 0.5, TimeEnd: 2.0}, 8)
	if len(rows) != 1 || rows[0].SourcePath != "a.systrace" || rows[0].Samples != 1 {
		t.Fatalf("only the provenance-proven sample may enter a series (fail-open fallback would have merged both): %+v", rows)
	}
	if quality == nil {
		t.Fatal("quality summary missing")
	}
	found := false
	for _, issue := range quality.Issues {
		if issue.Reason == "source_identity_unresolved" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the excluded row must be disclosed as a provenance issue: %+v", quality.Issues)
	}
}
