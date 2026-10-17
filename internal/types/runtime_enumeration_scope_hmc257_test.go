package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func enumerationQueryHMC257(source, payload, query string, start float64) ToolResult {
	return ToolResult{ToolName: "trace_query", Success: true, RawRef: payload,
		EnumerationAuthority: &ToolEnumerationAuthority{Status: "incomplete", Boundaries: []ToolEnumerationBoundary{{Scope: "resource_stack", Dimension: "events", Emitted: 2, Total: 5, TotalKnown: true}}},
		Observations:         []ObservationRecord{{SourceRef: ObservationSourceRef{Path: source, PayloadRef: payload, QueryScopeID: query, QueryWindowKnown: true, QueryWindowStartTs: start, QueryWindowEndTs: start + .02, QueryTargetPID: 41, QueryTargetScope: "thread", TimeDomain: "trace"}}},
	}
}

func TestHMC257EnumerationSourceQueryScope(t *testing.T) {
	a := enumerationQueryHMC257("a.capture", "payload-a", "query-a", 2)
	copyA := a
	copyA.Observations = append([]ObservationRecord(nil), a.Observations...)
	copyA.Observations[0].SourceRef.ToolCallID = "navigation-mirror"
	copyA.Observations[0].SourceRef.Range = "lines 200-205"
	b := enumerationQueryHMC257("b.capture", "payload-b", "query-b", 2)
	w := enumerationQueryHMC257("a.capture", "payload-other-window", "query-c", 3)
	before, _ := json.Marshal([]ToolResult{a, copyA, b, w})
	got := BuildRuntimeArtifactEnumerationAuthority([]ToolResult{a, copyA, b, w})
	if len(got.ScopedBoundaries) != 3 || len(got.Boundaries) != 3 || !got.Incomplete {
		t.Fatalf("source/window identity merged or mirror duplicated: %+v", got)
	}
	for _, entry := range got.ScopedBoundaries {
		if entry.CoverageKind != RuntimeEnumerationQueryResult || !entry.BoundaryKnown || len(entry.SourceRefs) != 1 || entry.SourceRefs[0].Range != "" || entry.SourceRefs[0].ToolCallID != "" {
			t.Fatalf("not producer-local: %+v", entry)
		}
	}
	after, _ := json.Marshal([]ToolResult{a, copyA, b, w})
	if string(before) != string(after) {
		t.Fatal("scope presentation mutated producer results")
	}
}

func TestHMC257EnumerationUnknownAndPageIdentity(t *testing.T) {
	for _, source := range []ObservationSourceRef{{}, {Kind: ObservationSourceRuntimeArtifact, TimeDomain: "trace"}, {Path: "same.capture", TimeDomain: "trace"}} {
		r := ToolResult{ToolName: "trace_query", Observations: []ObservationRecord{{SourceRef: source}}, EnumerationAuthority: &ToolEnumerationAuthority{Status: "incomplete"}}
		got := BuildRuntimeArtifactEnumerationAuthority([]ToolResult{r, r}).WithoutScope("root_cause_rank")
		if !got.Incomplete || len(got.ScopedBoundaries) != 2 {
			t.Fatalf("unknown source falsely merged or cleared: %+v", got)
		}
		for _, e := range got.ScopedBoundaries {
			if e.BoundaryKnown || e.UnlocatedResultOrdinal == 0 {
				t.Fatalf("unknown became measured zero: %+v", e)
			}
		}
	}
	page := func(path string, start int) ToolResult {
		return ToolResult{ToolName: "read_file", RuntimeArtifactRead: &ToolRuntimeArtifactRead{RequestedPath: path, TraceQueryBlob: true, LineStart: start, LineEnd: start + 4, TotalLines: 100}, EnumerationAuthority: &ToolEnumerationAuthority{Status: "incomplete", Boundaries: []ToolEnumerationBoundary{{Scope: path, Dimension: "lines", Emitted: 5, Total: 100, TotalKnown: true}}}}
	}
	a, b, next := page("a.json", 1), page("b.json", 1), page("a.json", 6)
	got := BuildRuntimeArtifactEnumerationAuthority([]ToolResult{a, a, b, next})
	if len(got.ScopedBoundaries) != 3 {
		t.Fatalf("different pages collapsed: %+v", got)
	}
	for _, entry := range got.ScopedBoundaries {
		if entry.CoverageKind != RuntimeEnumerationArtifactPage || entry.ReadPage == nil {
			t.Fatalf("page kind lost: %+v", entry)
		}
	}
}

func TestHMC257EnumerationTypedRoleAndQueryLimits(t *testing.T) {
	q := enumerationQueryHMC257("looks-like-result.json", "q", "q", 2)
	q.Success = false // failure is not reclassified by extension
	q.EnumerationAuthority.Boundaries[0].TotalKnown = false
	q.EnumerationAuthority.Boundaries[0].Reason = "cancelled"
	ordinary := ToolResult{ToolName: "read_file", RawRef: "fake-trace.json", EnumerationAuthority: q.EnumerationAuthority}
	complete := q
	complete.Success = true
	complete.EnumerationAuthority = &ToolEnumerationAuthority{Status: "complete"}
	got := BuildRuntimeArtifactEnumerationAuthority([]ToolResult{q, ordinary, complete})
	if len(got.ScopedBoundaries) != 1 || !got.Incomplete || got.ScopedBoundaries[0].CoverageKind != RuntimeEnumerationQueryResult || got.Boundaries[0].TotalKnown || got.Boundaries[0].Reason != "cancelled" {
		t.Fatalf("failure/unknown total erased or JSON sniffed: %+v", got)
	}
}

func TestHMC257EnumerationProjectionAndLargeMemberCoordinates(t *testing.T) {
	q := enumerationQueryHMC257("a.capture", "q", "q", 2)
	for n := 0; n < 1000; n++ {
		r := q.Observations[0]
		r.SourceRef.Range = fmt.Sprintf("%d-%d", n, n+1)
		r.SourceRef.Command = strings.Repeat("noise", 100)
		q.Observations = append(q.Observations, r)
	}
	rank := q
	rank.EnumerationAuthority = &ToolEnumerationAuthority{Status: "incomplete", Boundaries: []ToolEnumerationBoundary{{Scope: "root_cause_rank", Dimension: "candidates", Emitted: 2}}}
	got := BuildRuntimeArtifactEnumerationAuthority([]ToolResult{q, rank}).WithoutScope("root_cause_rank")
	if len(got.Boundaries) != 1 || len(got.ScopedBoundaries) != 1 || len(got.Scopes) != 1 || got.Scopes[0] != "resource_stack" || len(got.ScopedBoundaries[0].SourceRefs) != 1 {
		t.Fatalf("filter or per-event budget drift: %+v", got)
	}
	encoded, _ := json.Marshal(got)
	if len(encoded) > 2000 || strings.Contains(string(encoded), "noise") {
		t.Fatalf("producer scope retained event noise: bytes=%d", len(encoded))
	}
}
