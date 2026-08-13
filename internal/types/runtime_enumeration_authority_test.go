package types

import "testing"

func TestBuildRuntimeArtifactEnumerationAuthorityScopesAndDeduplicates(t *testing.T) {
	boundaryA := ToolEnumerationBoundary{Scope: "root_cause_rank", Dimension: "candidates", Emitted: 12, Total: 61, TotalKnown: true, Reason: "capacity"}
	boundaryB := ToolEnumerationBoundary{Scope: "span_window", Dimension: "spans", Emitted: 40, TotalKnown: false, Reason: "page_cap"}
	results := []ToolResult{
		{ToolName: "trace_query", EnumerationAuthority: &ToolEnumerationAuthority{Status: "incomplete", Boundaries: []ToolEnumerationBoundary{boundaryB, boundaryA}}},
		{ToolName: "trace_query:run2", EnumerationAuthority: &ToolEnumerationAuthority{Status: "incomplete", Boundaries: []ToolEnumerationBoundary{boundaryA}}},
		{ToolName: "read_file", RuntimeArtifactRead: &ToolRuntimeArtifactRead{}, EnumerationAuthority: &ToolEnumerationAuthority{Status: "incomplete", Boundaries: []ToolEnumerationBoundary{{Scope: "runtime.log", Dimension: "lines", Emitted: 200, Total: 500, TotalKnown: true}}}},
		// An ordinary current-source read and a complete trace result do not
		// create an incomplete runtime authority.
		{ToolName: "read_file", EnumerationAuthority: &ToolEnumerationAuthority{Status: "incomplete", Boundaries: []ToolEnumerationBoundary{{Scope: "source.go", Dimension: "lines", Emitted: 10}}}},
		{ToolName: "trace_query", EnumerationAuthority: &ToolEnumerationAuthority{Status: "complete", Boundaries: []ToolEnumerationBoundary{{Scope: "window_stats", Dimension: "rows", Emitted: 1, Total: 1, TotalKnown: true}}}},
	}

	got := BuildRuntimeArtifactEnumerationAuthority(results)
	if !got.Incomplete || len(got.Boundaries) != 3 {
		t.Fatalf("runtime enumeration authority did not dedupe/scope: %+v", got)
	}
	wantScopes := []string{"root_cause_rank", "runtime.log", "span_window"}
	for i, want := range wantScopes {
		if i >= len(got.Scopes) || got.Scopes[i] != want {
			t.Fatalf("scopes=%v, want %v", got.Scopes, wantScopes)
		}
	}
}

func TestBuildRuntimeArtifactEnumerationAuthorityCompleteOrUnrelatedIsInactive(t *testing.T) {
	got := BuildRuntimeArtifactEnumerationAuthority([]ToolResult{
		{ToolName: "trace_query", EnumerationAuthority: &ToolEnumerationAuthority{Status: "complete"}},
		{ToolName: "read_file", EnumerationAuthority: &ToolEnumerationAuthority{Status: "incomplete"}},
	})
	if got.Incomplete || len(got.Boundaries) != 0 || len(got.Scopes) != 0 {
		t.Fatalf("complete/unrelated results activated runtime enumeration authority: %+v", got)
	}
}

func TestBuildRuntimeArtifactEnumerationAuthorityDoesNotPublishTraceQueryBlobPathAsScope(t *testing.T) {
	privatePath := "/work/.codrax/blob/session/trace_query-deadbeef.txt"
	got := BuildRuntimeArtifactEnumerationAuthority([]ToolResult{{
		ToolName: "read_file",
		RuntimeArtifactRead: &ToolRuntimeArtifactRead{
			RequestedPath:  privatePath,
			Kind:           "blob",
			TraceQueryBlob: true,
		},
		EnumerationAuthority: &ToolEnumerationAuthority{
			Status: "incomplete",
			Boundaries: []ToolEnumerationBoundary{{
				Scope: privatePath, Dimension: "lines", Emitted: 66,
				Total: 332, TotalKnown: true, Reason: "inline_budget_clamped",
			}},
		},
	}})

	if !got.Incomplete || len(got.Boundaries) != 1 {
		t.Fatalf("trace query page boundary was lost: %+v", got)
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != runtimeTraceQueryResultPageScope {
		t.Fatalf("public scopes=%v, want stable typed scope %q", got.Scopes, runtimeTraceQueryResultPageScope)
	}
	if got.Boundaries[0].Scope != runtimeTraceQueryResultPageScope {
		t.Fatalf("public boundary leaked private scope: %+v", got.Boundaries[0])
	}
	if got.Boundaries[0].Emitted != 66 || got.Boundaries[0].Total != 332 || !got.Boundaries[0].TotalKnown {
		t.Fatalf("typed enumeration measurements changed during scope normalization: %+v", got.Boundaries[0])
	}
}
