package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func noArrowOwnershipTestContext(source string) *types.BusContext {
	mut := types.NewMutableState("explain the data flow")
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
				}},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "field-ownership", Kind: types.EvidenceDirect,
			Source: source, LineStart: 17, Scope: types.ScopeLine,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "Mutable",
			DeclaredOwner: "BusContext", DeclaredBinding: "BusContext.Mutable", DeclaredType: "*MutableState",
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	return ctx
}

func noArrowOwnershipDiagram(body string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "architecture", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: body},
	}}}
}

func TestPreCheckDiagramNoArrowOwnershipDirectionRejectsExactReverseGrouping(t *testing.T) {
	for _, source := range []string{
		"src/pipeline.go", "src/Pipeline.java", "src/pipeline.ets", "src/pipeline.cj",
		"src/pipeline.py", "src/pipeline.rs", "src/pipeline.cpp",
	} {
		t.Run(source, func(t *testing.T) {
			ctx := noArrowOwnershipTestContext(source)
			doc := noArrowOwnershipDiagram(`flowchart TD
  subgraph state_group["Mutable"]
    carrier["BusContext"]
  end`)
			hints := preCheckDiagramNoArrowOwnershipDirection(doc, newPreEmitCheckContext(ctx))
			if len(hints) != 1 || hints[0].HardSignal != preEmitHardSignalTypedDiagramParticipantCoverage {
				t.Fatalf("reverse ownership should be one precise hard mismatch: %+v", hints)
			}
			for _, want := range []string{"owner=\"BusContext\"", "member/type=\"Mutable\"", "exact owner the visible parent subgraph", "use no arrow"} {
				if !strings.Contains(hints[0].ExpectedShape, want) {
					t.Fatalf("repair missing %q: %+v", want, hints[0])
				}
			}
		})
	}
}

func TestPreCheckDiagramNoArrowOwnershipDirectionAcceptsCorrectOrUnassertedGrouping(t *testing.T) {
	ctx := noArrowOwnershipTestContext("src/pipeline.ext")
	for name, body := range map[string]string{
		"correct": `flowchart TD
  subgraph carrier_group["BusContext"]
    state["Mutable<br/>MutableState"]
  end`,
		"nested-correct": `flowchart TD
  subgraph carrier_group["BusContext"]
    subgraph state_group["Mutable"]
      detail["state details"]
    end
  end`,
		"peers": `flowchart TD
  carrier["BusContext"]
  state["Mutable"]`,
		"unrelated-layout": `flowchart TD
  subgraph request_flow["Read pipeline"]
    carrier["BusContext"]
    state["Mutable"]
  end`,
	} {
		t.Run(name, func(t *testing.T) {
			if hints := preCheckDiagramNoArrowOwnershipDirection(noArrowOwnershipDiagram(body), newPreEmitCheckContext(ctx)); len(hints) != 0 {
				t.Fatalf("non-reversed presentation must remain model-owned: %+v", hints)
			}
		})
	}
}

func TestPreCheckDiagramNoArrowOwnershipDirectionDoesNotEnterTrace(t *testing.T) {
	ctx := noArrowOwnershipTestContext("trace.systrace")
	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	doc := noArrowOwnershipDiagram(`flowchart TD
  subgraph state_group["Mutable"]
    carrier["BusContext"]
  end`)
	if hints := preCheckDiagramNoArrowOwnershipDirection(doc, newPreEmitCheckContext(ctx)); len(hints) != 0 {
		t.Fatalf("runtime Trace diagrams keep their independent causal authority: %+v", hints)
	}
}
