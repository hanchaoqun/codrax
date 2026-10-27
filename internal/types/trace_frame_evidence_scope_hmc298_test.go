package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func hmc298FrameResult(id, path, status string, start float64, edges int) ToolResult {
	r := traceCausalClaimTestRecord(id)
	r.SourceRef = ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: path, QueryScopeID: id, QueryWindowKnown: true, QueryWindowStartTs: start, QueryWindowEndTs: start + .02, QueryTargetPID: 100}
	return ToolResult{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{r}, TraceEvidenceAuthority: &TraceEvidenceAuthority{View: "frame_flow", FrameEvidenceStatus: status, FrameFlowEdgeCount: edges, FrameFlowRelationAuthority: "temporal_sequence", FrameFlowCausalConclusion: "unproven"}}
}

func TestHMC298FrameScopesKeepSourceWindowAndCountsSeparate(t *testing.T) {
	a, b, history := hmc298FrameResult("a", "/a.trace", "present", 2, 1), hmc298FrameResult("b", "/b.trace", "absent", 4, 0), hmc298FrameResult("history", "/a.trace", "present", 9, 5)
	input := ObservationLedgerInput{ToolResults: []ToolResult{history, b, a}}
	set := TraceCausalProjectionSet{Projections: []TraceCausalProjection{{PrimaryRootCause: &TraceCausalProjectionNode{EvidenceID: "a"}}, {PrimaryRootCause: &TraceCausalProjectionNode{EvidenceID: "b"}}}}
	before, _ := json.Marshal(input)
	scopes := BuildTraceFrameEvidenceScopes(input, set)
	if len(scopes) != 3 || TraceFrameSelectedStatus(scopes, 0) != "present" || TraceFrameSelectedStatus(scopes, 1) != "absent" || TraceFrameCommonSelectedStatus(scopes, set) != "" {
		t.Fatalf("mixed source/window statuses collapsed: %+v", scopes)
	}
	for _, scope := range scopes {
		if scope.SelectedFor(0) && (scope.Source.Path != "/a.trace" || scope.Source.QueryWindowStartTs != 2 || scope.Authority.FrameFlowEdgeCount != 1) {
			t.Fatalf("historical edge count transferred: %+v", scope)
		}
		if scope.ResultIndex == 0 && (len(scope.SelectedProjections) != 0 || !strings.Contains(scope.Locator(), "query-local")) {
			t.Fatalf("history upgraded: %+v", scope)
		}
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("read-only view mutated original")
	}
}

func TestHMC298FrameScopesUnknownAndConflictingIDsDoNotBind(t *testing.T) {
	for _, mode := range []string{"unknown_source", "missing_observations", "conflicting_id", "multi_child_result", "failed_result"} {
		t.Run(mode, func(t *testing.T) {
			r := hmc298FrameResult("current", "/a.trace", "absent", 2, 0)
			input := ObservationLedgerInput{}
			switch mode {
			case "unknown_source":
				r.Observations[0].SourceRef = ObservationSourceRef{}
			case "missing_observations":
				r.Observations = nil
			case "failed_result":
				r.Success = false
			case "conflicting_id":
				other := hmc298FrameResult("current", "/b.trace", "present", 9, 5)
				input.ToolResults = append(input.ToolResults, other)
			case "multi_child_result":
				other := hmc298FrameResult("other", "/a.trace", "present", 9, 5)
				r.Observations = append(r.Observations, other.Observations...)
			}
			input.ToolResults = append(input.ToolResults, r)
			set := TraceCausalProjectionSet{Projections: []TraceCausalProjection{{PrimaryRootCause: &TraceCausalProjectionNode{EvidenceID: "current"}}}}
			scopes := BuildTraceFrameEvidenceScopes(input, set)
			if TraceFrameSelectedStatus(scopes, 0) != "" {
				t.Fatalf("unknown/conflict borrowed current status: %+v", scopes)
			}
			for _, scope := range scopes {
				if len(scope.SelectedProjections) != 0 {
					t.Fatalf("unbound result selected: %+v", scope)
				}
			}
			if mode != "failed_result" && len(scopes) == 0 {
				t.Fatal("legitimate local measurements were erased")
			}
		})
	}
}

func TestHMC298FrameScopesPreferCurrentWithoutChangingResultIdentity(t *testing.T) {
	input := ObservationLedgerInput{}
	for i := 0; i < 9; i++ {
		input.ToolResults = append(input.ToolResults, hmc298FrameResult(fmt.Sprintf("probe-%d", i), "/a.trace", "absent", float64(10+i), 0))
	}
	input.ToolResults = append(input.ToolResults, hmc298FrameResult("selected", "/a.trace", "present", 2, 1))
	set := TraceCausalProjectionSet{Projections: []TraceCausalProjection{{PrimaryRootCause: &TraceCausalProjectionNode{EvidenceID: "selected"}}}}
	scopes := BuildTraceFrameEvidenceScopes(input, set)
	if len(scopes) != 10 || scopes[0].ResultIndex != 9 || !scopes[0].SelectedFor(0) || scopes[1].ResultIndex != 0 {
		t.Fatalf("current budget or original identity lost: %+v", scopes)
	}
}
