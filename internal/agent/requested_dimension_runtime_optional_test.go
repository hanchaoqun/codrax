package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExplorerRuntimeOptionalDimensionOwnershipPrompt(t *testing.T) {
	for _, exactBinding := range []bool{false, true} {
		t.Run(map[bool]string{false: "runtime only", true: "one precise source binding"}[exactBinding], func(t *testing.T) {
			ctx := requestedDimensionEvidenceOwnershipContext()
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "external_tool", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
			ctx.Mutable = types.NewMutableState("runtime optional dimensions")
			ctx.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{{
				ID: "trace:distribution", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
				SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "events.systrace", ArtifactID: "attached_trace", ArtifactKind: "trace", PayloadRef: "blob://trace-query"},
				Span:      types.ObservationSpan{LineStart: 5, LineEnd: 26}, Summary: "producer-owned complete request distribution",
			}}})
			if exactBinding {
				ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "worker.go", Confidence: 1, RequestedDimensionIndices: []int{3}}}
			}
			before, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			prompt := (&explorerEvaluator{}).BuildInitialInstruction(ctx, nil)
			if strings.Contains(prompt, "### Requested Explanation Evidence Ownership") != exactBinding {
				t.Fatalf("actual prompt must follow runtime/source authority; source binding=%v", exactBinding)
			}
			if exactBinding && (!strings.Contains(prompt, "index=1 role=function_or_purpose") || !strings.Contains(prompt, "index=3 source=worker.go")) {
				t.Fatal("one precise source binding must preserve the whole mixed-source ownership contract")
			}
			after, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			if string(before) != string(after) {
				t.Fatal("prompt must not rewrite requested dimensions")
			}
		})
	}
}
