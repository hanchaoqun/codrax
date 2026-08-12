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
		"exact subject/object contains the carrier binding",
		"unmodeled argument",
		"Do not rename an endpoint to a semantic role or result label",
		"participant names guide navigation only",
		"no-arrow containment grouping",
		"never proves a read, write, transfer, execution order, or directed incident edge",
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

func TestAnswerCodeIdentityIncidentViaDeclaredBindingRequiresExactTypedJoin(t *testing.T) {
	declaration := EvidenceItem{
		Source: "src/pipeline.go", LineStart: 5, Scope: ScopeLine,
		GroundingStatus: GroundingGrounded, AnchorKind: AnchorDefinition,
		DeclaredBinding: "Orchestrator.busCtx", DeclaredType: "*types.BusContext", DeclaredOwner: "Orchestrator",
	}
	operation := EvidenceItem{
		Source: "src/pipeline.go", LineStart: 20, Scope: ScopeLine,
		GroundingStatus: GroundingGrounded, AnchorKind: AnchorAssignment,
		Subject: "o.busCtx.EvidenceItems", Object: "output.EvidenceItems",
		Snippet:       "o.busCtx.EvidenceItems = output.EvidenceItems",
		OwnerIdentity: "Orchestrator.applyStageOutput",
	}
	if !AnswerCodeIdentityIncidentViaDeclaredBinding("BusContext", operation.Subject, operation, []EvidenceItem{declaration, operation}) {
		t.Fatal("exact static type, binding segment, source, and owner should align participant identity")
	}
	for _, tc := range []struct {
		name        string
		participant string
		endpoint    string
		declaration EvidenceItem
		operation   EvidenceItem
	}{
		{name: "different type", participant: "OtherContext", endpoint: operation.Subject, declaration: declaration, operation: operation},
		{name: "different binding", participant: "BusContext", endpoint: "o.other.EvidenceItems", declaration: declaration, operation: operation},
		{name: "different owner", participant: "BusContext", endpoint: operation.Subject, declaration: declaration, operation: func() EvidenceItem { got := operation; got.OwnerIdentity = "Worker.apply"; return got }()},
		{name: "different file", participant: "BusContext", endpoint: operation.Subject, declaration: declaration, operation: func() EvidenceItem { got := operation; got.Source = "src/other.go"; return got }()},
		{name: "untyped declaration", participant: "BusContext", endpoint: operation.Subject, declaration: func() EvidenceItem { got := declaration; got.DeclaredType = ""; return got }(), operation: operation},
		{name: "runtime declaration", participant: "BusContext", endpoint: operation.Subject, declaration: func() EvidenceItem { got := declaration; got.Source = "trace.systrace"; return got }(), operation: func() EvidenceItem { got := operation; got.Source = "trace.systrace"; return got }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if AnswerCodeIdentityIncidentViaDeclaredBinding(tc.participant, tc.endpoint, tc.operation, []EvidenceItem{tc.declaration, tc.operation}) {
				t.Fatal("incomplete or mismatched join must fail closed")
			}
		})
	}
}

func TestAnswerCodeIdentityIncidentViaOperationStampedBindingNeedsNoDefinitionRow(t *testing.T) {
	operation := EvidenceItem{
		Source: "src/pipeline.go", LineStart: 20, Scope: ScopeLine,
		GroundingStatus: GroundingGrounded, AnchorKind: AnchorAssignment,
		Subject: "o.busCtx.EvidenceItems", Object: "output.EvidenceItems",
		Snippet:       "o.busCtx.EvidenceItems = output.EvidenceItems",
		OwnerIdentity: "Orchestrator.applyStageOutput",
		DeclaredIdentityBindings: []EvidenceDeclaredIdentityBinding{{
			Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
		}},
	}
	if !AnswerCodeIdentityIncidentViaDeclaredBinding("BusContext", operation.Subject, operation, []EvidenceItem{operation}) {
		t.Fatal("parser-stamped operation binding should align identity without a second declaration evidence row")
	}
	if AnswerCodeIdentityIncidentViaDeclaredBinding("OtherContext", operation.Subject, operation, []EvidenceItem{operation}) {
		t.Fatal("operation binding must not cover a different participant type")
	}
	wrongOwner := operation
	wrongOwner.OwnerIdentity = "Worker.apply"
	if AnswerCodeIdentityIncidentViaDeclaredBinding("BusContext", wrongOwner.Subject, wrongOwner, []EvidenceItem{wrongOwner}) {
		t.Fatal("operation binding must remain owner-scoped")
	}
}

func TestDeclaredBindingDefinitionAloneNeverBecomesFlowOperation(t *testing.T) {
	declaration := EvidenceItem{
		Source: "src/pipeline.go", LineStart: 5, Scope: ScopeLine,
		GroundingStatus: GroundingGrounded, AnchorKind: AnchorDefinition,
		DeclaredBinding: "Orchestrator.busCtx", DeclaredType: "BusContext", DeclaredOwner: "Orchestrator",
	}
	if got := FlowOperationEvidence([]EvidenceItem{declaration}); len(got) != 0 {
		t.Fatalf("static declaration must never mint a flow operation: %+v", got)
	}
	if AnswerCodeIdentityIncidentViaDeclaredBinding("BusContext", "o.busCtx.Items", declaration, []EvidenceItem{declaration}) {
		t.Fatal("a declaration cannot serve as its own operation authority")
	}
}
