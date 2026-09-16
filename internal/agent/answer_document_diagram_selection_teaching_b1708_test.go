package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A wider typed presentation pool is still candidate input. Requiring every
// support edge in that pool would contradict the model-owned subset choice.
func TestB1708PublicDiagramSelectionTeachingSharesScopeAcrossInitialAndRepair(t *testing.T) {
	for _, scoped := range []bool{true, false} {
		name := "ordinary_required_diagram"
		if scoped {
			name = "endpoint_boundary_with_support"
		}
		t.Run(name, func(t *testing.T) {
			for _, toolName := range []string{"emit_answer_document", "emit_answer_document_patch"} {
				t.Run(toolName, func(t *testing.T) {
					ctx := b1708BoundaryCandidateContext()
					if !scoped {
						ctx.Mutable.SetPrincipalSpanWaiver(nil)
					}
					ctx.Mutable.SetRetryState(&types.RetryState{
						Attempt: 1, PrevEmitJSON: []byte(`{"blocks":[{"id":"model-summary","kind":"summary","text":"model-owned"}]}`),
					})
					evaluator := &answerDocumentEvaluator{diagramRequired: true, mu: ctx.Mutable}
					initial := evaluator.BuildInitialInstruction(ctx, nil)
					signal := evaluator.Observe(ctx, LoopObservation{
						Phase: PhaseMidLoop,
						LastToolResult: &types.ToolResult{ToolName: toolName, Success: false, Repair: &types.ToolRepair{
							Code: "answer_doc_pre_emit_contract",
							Metadata: map[string]string{
								"violation_kinds":                       string(types.ViolDiagramCallEdgeUnproven),
								types.ToolRepairMetaOffendingBlockKinds: string(types.BlockDiagram),
							},
						}},
					})
					if !signal.HintRequested {
						t.Fatalf("fixture did not reach public diagram repair: %+v", signal)
					}
					if !scoped {
						if strings.Contains(initial, "omit unselected support arrows together with their anchors") ||
							strings.Contains(signal.Hint, "omit unselected support arrows together with their anchors") {
							t.Fatal("boundary-support subset teaching leaked into an ordinary diagram contract")
						}
						if !strings.Contains(initial, "exact edge topology") || !strings.Contains(signal.Hint, "as one unit") {
							t.Fatal("ordinary required-diagram teaching changed outside B1708 scope")
						}
						return
					}
					for surface, prompt := range map[string]string{"initial": initial, "repair": signal.Hint} {
						for _, want := range []string{
							"select a useful subset",
							"Keep the required diagram",
							"exact endpoint boundary, participant coverage, and unproven boundaries",
							"omit unselected support arrows together with their anchors",
							"Do not reconnect selected edges",
							"AUTHOR_BUSINESS_ACTION",
							"omission from this template does not revoke an independently grounded relation",
						} {
							if !strings.Contains(prompt, want) {
								t.Errorf("%s omitted model-owned subset/authority teaching %q", surface, want)
							}
						}
						for _, forbidden := range []string{
							"Only the explicit typed relations and supported typed flow paths listed below carry their stated authority",
							"If you include the optional diagram, preserve its node IDs, exact edge topology",
							"Keep its node IDs, edge direction/topology",
							"Preserve the following typed topology template's exact node IDs, edge topology",
							"Do not delete an already-typed relation merely to simplify the diagram",
						} {
							if strings.Contains(prompt, forbidden) {
								t.Errorf("%s retains contradictory whole-template instruction %q", surface, forbidden)
							}
						}
					}
				})
			}
		})
	}
}
