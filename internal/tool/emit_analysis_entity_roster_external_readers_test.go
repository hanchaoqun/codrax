package tool

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Cross-package readers remain subject to the same body/no-roster-write and
// stale-registration checks as local RequestModel helpers, not an exemption.
var emitAnalysisRequestModelExternalReaders = map[string]string{
	"types.ToolDocumentationOnlyRequested":            "../types/tool_documentation_request.go",
	"types.CompileDimensionOwnerUnresolvedForRequest": "../types/requested_dimension_file_ownership.go",
}

func TestEmitAnalysisEntityRosterExternalReaders(t *testing.T) {
	execute, err := os.ReadFile("emit_analysis.go")
	if err != nil {
		t.Fatal(err)
	}
	for qualified, path := range emitAnalysisRequestModelExternalReaders {
		t.Run(qualified, func(t *testing.T) {
			name := strings.TrimPrefix(qualified, "types.")
			if name == qualified || !strings.Contains(string(execute), qualified+"(&rm)") {
				t.Fatalf("stale/invalid external RequestModel reader registration: %s", qualified)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			readers := map[string]bool{name: true}
			offenders, seen, err := emitAnalysisMutatorBodyOffenders(path, string(body), readers)
			if err != nil || !seen[name] || len(offenders) != 0 {
				t.Fatalf("external reader rewrites roster or disappeared: seen=%v offenders=%v err=%v", seen, offenders, err)
			}
			start := strings.Index(string(body), "func "+name+"(")
			brace := start + strings.Index(string(body[start:]), "{")
			for _, field := range []string{"Entities", "PrimaryEntities"} {
				mutated := string(body[:brace+1]) + "\nrm.AnalyzerHints." + field + " = nil\n" + string(body[brace+1:])
				offenders, _, err := emitAnalysisMutatorBodyOffenders(path, mutated, readers)
				if err != nil || len(offenders) == 0 {
					t.Fatalf("self-red external %s rewrite escaped: offenders=%v err=%v", field, offenders, err)
				}
			}
		})
	}
}

func TestEmitAnalysisEntityRosterExternalReadersDoNotMutateRequest(t *testing.T) {
	for _, scope := range []types.ToolDocumentationRequestScope{"", types.ToolDocumentationRequestOnly, types.ToolDocumentationRequestMixed, "invalid"} {
		t.Run(string(scope), func(t *testing.T) {
			rm := types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Entities: []string{"Mutable/BusContext", "analyzer"}, PrimaryEntities: []string{"Mutable/BusContext"}},
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
					{Index: 1, Label: "units", Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
					{Index: 2, Label: "implementation", Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
				}},
			}
			if scope != "" {
				rm.ToolDocumentationRequest = &types.ToolDocumentationRequest{Scope: scope}
				if scope == types.ToolDocumentationRequestMixed {
					rm.ToolDocumentationRequest.DimensionIndices = []int{1}
				}
			}
			before, err := json.Marshal(rm)
			if err != nil {
				t.Fatal(err)
			}
			_ = types.ToolDocumentationOnlyRequested(&rm)
			_ = types.CompileDimensionOwnerUnresolvedForRequest(&rm)
			_ = validateEmitToolDocumentationRequest(&rm, nil)
			after, err := json.Marshal(rm)
			if err != nil || string(after) != string(before) {
				t.Fatalf("RequestModel reader mutated original input: before=%s after=%s err=%v", before, after, err)
			}
		})
	}
}
