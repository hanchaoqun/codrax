package types

import (
	"strings"
	"testing"
)

func TestFlowOperationEvidenceGuideRequiresSyntaxEndpoints(t *testing.T) {
	for _, want := range []string{
		"exact syntax-authored subject/object",
		"exact enclosing callable and invoked callee",
		"exact writer and reader operation sites",
		"field/type declaration does not prove that data entered or left it",
		"declaration-only field/member line is a definition, never an initializer",
		"initializer requires an exact value-bearing member binding",
		"Do not rename an endpoint to a semantic role or result label",
		"participant names guide navigation only",
	} {
		if !strings.Contains(FlowOperationEvidenceEmissionGuide, want) {
			t.Fatalf("flow-operation teaching lost cross-language endpoint rule %q", want)
		}
	}
}

func TestFlowOperationEvidenceRejectsDefinitionsAndRuntimeRows(t *testing.T) {
	definition := EvidenceItem{
		Source: "src/pipeline.go", LineStart: 10, GroundingStatus: GroundingGrounded,
		AnchorKind: AnchorDefinition, Subject: "Pipeline", Object: "stages",
	}
	runtime := EvidenceItem{
		Source: "trace.systrace", LineStart: 20, GroundingStatus: GroundingGrounded,
		AnchorKind: AnchorCall, Subject: "A", Object: "B",
	}
	missingEndpoint := EvidenceItem{
		Source: "src/pipeline.go", LineStart: 30, GroundingStatus: GroundingGrounded,
		AnchorKind: AnchorAssignment, Subject: "state",
	}
	if got := FlowOperationEvidence([]EvidenceItem{definition, runtime, missingEndpoint}); len(got) != 0 {
		t.Fatalf("identity/runtime/incomplete rows must not become source-flow operations: %+v", got)
	}
}

func TestFlowOperationEvidenceAcceptsCrossLanguageAxisForms(t *testing.T) {
	forms := []AnchorKind{AnchorCall, AnchorCallback, AnchorAssignment, AnchorInitializer, AnchorReturn, AnchorPrecedence}
	items := make([]EvidenceItem, 0, len(forms))
	for i, anchor := range forms {
		item := EvidenceItem{
			Source: "src/pipeline.ets", LineStart: i + 1, GroundingStatus: GroundingGrounded,
			AnchorKind: anchor, Subject: "Producer", Object: "Consumer",
		}
		if anchor == AnchorAssignment {
			item.Snippet = "Producer = Consumer"
		}
		if anchor == AnchorInitializer {
			item.Snippet = "Producer: Consumer"
		}
		items = append(items, item)
	}
	if got := FlowOperationEvidence(items); len(got) != len(forms) {
		t.Fatalf("all typed AxisFlow operation forms should survive language-neutral projection: %+v", got)
	}
}

func TestFlowOperationEvidenceForRequestKeepsAuxiliaryOutOfProductionCarrier(t *testing.T) {
	production := EvidenceItem{
		Source: "src/pipeline.go", LineStart: 10, GroundingStatus: GroundingGrounded,
		AnchorKind: AnchorCall, Subject: "A", Object: "B",
	}
	testOnly := EvidenceItem{
		Source: "src/pipeline_test.go", LineStart: 20, GroundingStatus: GroundingGrounded,
		AnchorKind: AnchorCall, Subject: "FixtureA", Object: "FixtureB",
	}
	rm := RequestModel{SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeProduction}}
	got := FlowOperationEvidenceForRequest([]EvidenceItem{testOnly, production}, rm)
	if len(got) != 1 || got[0].Source != production.Source {
		t.Fatalf("production operation carrier was polluted by auxiliary evidence: %+v", got)
	}
}

func TestExplorerAuthoredFlowOperationEvidenceExcludesDeterministicExpansion(t *testing.T) {
	selected := EvidenceItem{
		Producer: EvidenceProducerExplorerEmitEvidence,
		Source:   "src/pipeline.go", LineStart: 10, GroundingStatus: GroundingGrounded,
		AnchorKind: AnchorAssignment, Subject: "Producer", Object: "Carrier", Snippet: "Producer = Carrier",
	}
	automatic := EvidenceItem{
		Producer: "dataflow.lowerer.go",
		Source:   "src/helper.go", LineStart: 20, GroundingStatus: GroundingGrounded,
		AnchorKind: AnchorCall, Subject: "Helper", Object: "Sink",
	}
	got := ExplorerAuthoredFlowOperationEvidenceForRequest([]EvidenceItem{automatic, selected}, RequestModel{})
	if len(got) != 1 || got[0].Source != selected.Source {
		t.Fatalf("principal operation scope must retain only Explorer-selected rows: %+v", got)
	}
}
