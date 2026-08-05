package tool

import (
	"strings"
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func diagramEvidenceTestCall(subject, object string) types.EvidenceItem {
	return types.EvidenceItem{
		ID:              "ev-" + subject + "-" + object,
		Kind:            types.EvidenceRelationship,
		Subject:         subject,
		Predicate:       "calls",
		Object:          object,
		Source:          "internal/example.go",
		LineStart:       10,
		AnchorKind:      types.AnchorCall,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}
}

func diagramEvidenceTestDefinition(owner, operation, source string, line int) types.EvidenceItem {
	return types.EvidenceItem{
		ID:              "ev-def-" + owner + "-" + operation,
		Kind:            types.EvidenceDirect,
		Subject:         owner,
		Source:          source,
		LineStart:       line,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    operation,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}
}

func diagramEvidenceTestDoc(from, to string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "sequence",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramSequence,
			Language: "mermaid",
			Body: "sequenceDiagram\n" +
				"  participant A as Alpha.Run\n" +
				"  participant B as Beta.Run\n" +
				"  " + from + "->>" + to + ": invoke\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode:     from,
			ToNode:       to,
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
		}},
	}}}
}

func TestDiagramCallEdgeEvidenceMismatches_DirectionUsesTypedEvidence(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")}
	if got := DiagramCallEdgeEvidenceMismatches(diagramEvidenceTestDoc("A", "B"), view, evidence); len(got) != 0 {
		t.Fatalf("typed call direction should pass: %+v", got)
	}
	got := DiagramCallEdgeEvidenceMismatches(diagramEvidenceTestDoc("B", "A"), view, evidence)
	if len(got) != 1 || got[0].FromSymbol != "Beta.Run" || got[0].ToSymbol != "Alpha.Run" {
		t.Fatalf("reverse edge should be rejected from structured direction, got %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceReplyIsNotACallEdge(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body += "  B-->>A: result\n"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 0 {
		t.Fatalf("a standard sequence reply must not require a reverse call edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceReplyCannotHideReverseCallAnchor(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body += "  B-->>A: result\n"
	doc.Blocks[0].EdgeAnchors = append(doc.Blocks[0].EdgeAnchors, types.DiagramEdgeAnchor{
		FromNode: "B", ToNode: "A", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
	})
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 1 || got[0].FromSymbol != "Beta.Run" || got[0].ToSymbol != "Alpha.Run" {
		t.Fatalf("an explicit reverse call anchor must still prove its own direction: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_StructuralReplyStaysReplyWhenReverseCallExists(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body += "  B-->>A: result\n"
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Alpha.Run", "Beta.Run"),
		diagramEvidenceTestCall("Beta.Run", "Alpha.Run"),
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("unrelated reverse typed evidence must not recapture a structurally paired response: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_UnpairedDashedEdgeIsNotSelfAuthorizingReply(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n  participant A as Alpha.Run\n  participant B as Beta.Run\n  B-->>A: invoke\n"
	doc.Blocks[0].EdgeAnchors = nil
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingAnchor {
		t.Fatalf("an unpaired dashed edge must remain in the fail-closed call contract: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ClassParticipantsResolveUniqueTypedMethods(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant C as VisitController",
			"  participant S as VisitService",
			"  participant R as VisitRepository",
			"  C->>S: schedule(petId, reason)",
			"  S->>R: countOpenVisits(petId)",
			"  S->>R: insert(petId, reason)",
			"  R-->>S: insertedId",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "S", ToNode: "R", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("VisitController.create", "VisitService.schedule"),
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.countOpenVisits"),
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.insert"),
	}
	evidence[0].AnchorSymbol = "schedule"
	evidence[1].AnchorSymbol = "countOpenVisits"
	evidence[2].AnchorSymbol = "insert"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("class participants plus an exact message operation should resolve unique typed method edges: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ClassParticipantsFailClosedWhenMethodIsAmbiguous(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitService\n" +
		"  participant B as VisitRepository\n" +
		"  A->>B: invoke\n"
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.countOpenVisits"),
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.insert"),
	}
	evidence[0].AnchorSymbol = "countOpenVisits"
	evidence[1].AnchorSymbol = "insert"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("class-only endpoints with no exact message operation must remain unproven: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_NodeDisplayMetadataDoesNotChangeIdentity(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "flowchart TD\n" +
		`  A["Alpha.Run\ninternal/example.go:10"]` + "\n" +
		`  B["Beta.Run<br/>internal/example.go:20"]` + "\n" +
		"  A --> B\n"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 0 {
		t.Fatalf("file/line display suffix must not become part of typed endpoint identity: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SameLineSupportRefDoesNotChangeIdentity(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as Alpha.Run (internal/example.go:10)\n" +
		"  participant B as Beta.Run (internal/example.go:20)\n" +
		"  A->>B: invoke\n"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 0 {
		t.Fatalf("typed support-ref suffix must not become part of endpoint identity: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ExactCalleeAnchorSymbolAuthorizesShortLabel(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as Alpha.Run\n" +
		"  participant B as Run\n" +
		"  A->>B: invoke\n"
	evidence := diagramEvidenceTestCall("Alpha.Run", "normalizer.Run")
	evidence.AnchorSymbol = "Run"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{evidence}); len(got) != 0 {
		t.Fatalf("exact grounded callee AnchorSymbol is an authoritative display alias for Object: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_RequiredQualifiedCallerBridgesUniqueShortCall(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as gate.Run\n" +
		"  participant B as gate.RunWith\n" +
		"  A->>B: calls RunWith\n"
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: "hops", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		FacetIDs: []string{string(types.FacetPrincipalPathEdge)},
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimCallEdge, FacetID: string(types.FacetPrincipalPathEdge),
		}},
		Items: []types.AnswerBlockItem{{ID: "h1", Label: "gate.Run"}, {ID: "h2", Label: "gate.RunWith"}},
	})
	shortCall := diagramEvidenceTestCall("Run", "RunWith")
	shortCall.Source = "internal/analysis/gate/gate.go"
	shortCall.LineStart = 135
	qualifiedCallee := diagramEvidenceTestCall("buildAnalysisIR", "gate.RunWith")
	qualifiedCallee.Source = "internal/agent/analyzer.go"
	qualifiedCallee.LineStart = 2666
	callerDefinition := diagramEvidenceTestDefinition("gate", "Run", "internal/analysis/gate/gate.go", 100)
	view := &types.AnswerSemanticView{
		Family: types.QFCallChain,
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{{
			Text: "gate.Run", Kind: types.ContractTermSymbol,
		}},
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{shortCall, qualifiedCallee}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a request anchor without citable caller-owner binding must not mint qualification: %+v", got)
	}
	ownerBoundCall := shortCall
	ownerBoundCall.OwnerSymbol = "gate.Run"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{ownerBoundCall}); len(got) != 0 {
		t.Fatalf("a grounded exact OwnerSymbol should bind an unqualified same-owner call without a duplicate definition row: %+v", got)
	}
	wrongOwnerCall := ownerBoundCall
	wrongOwnerCall.OwnerSymbol = "other.Run"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{wrongOwnerCall}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a differently qualified OwnerSymbol must fail closed: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{shortCall, qualifiedCallee, callerDefinition}); len(got) != 0 {
		t.Fatalf("typed required caller plus unique short call and exact callee should preserve qualification: %+v", got)
	}
	wrongOwnerFileCall := shortCall
	wrongOwnerFileCall.Source = "other/gate.go"
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{wrongOwnerFileCall, qualifiedCallee, callerDefinition}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a short call from outside the caller definition file must not inherit its owner: %+v", got)
	}

	view.RequiredMechanismAnchors = nil
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{shortCall, qualifiedCallee, callerDefinition}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("model-authored qualification must fail without typed caller authority: %+v", got)
	}

	view.RequiredMechanismAnchors = []types.AnswerRequiredAnchor{{Text: "gate.Run", Kind: types.ContractTermSymbol}}
	ambiguous := shortCall
	ambiguous.LineStart = 42
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{shortCall, ambiguous, qualifiedCallee, callerDefinition}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("multiple short call-site locations must remain ambiguous: %+v", got)
	}

	ambiguousDefinition := callerDefinition
	ambiguousDefinition.Source = "other/gate.go"
	ambiguousDefinition.LineStart = 20
	if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{shortCall, qualifiedCallee, callerDefinition, ambiguousDefinition}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("multiple caller-owner definitions must remain ambiguous: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_GroundedOwnerBindingSupportsClosedQualifierForms(t *testing.T) {
	tests := []struct {
		name, caller, callee string
	}{
		{"dot", "gate.Run", "gate.RunWith"},
		{"colon", "clinic::Gate::run", "clinic::Gate::runWith"},
		{"hash", "Gate#run", "Gate#runWith"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := diagramEvidenceTestDoc("A", "B")
			doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
				"  participant A as " + tc.caller + "\n" +
				"  participant B as " + tc.callee + "\n" +
				"  A->>B: invoke\n"
			call := diagramEvidenceTestCall(diagramEvidenceQualifiedOperation(tc.caller), diagramEvidenceQualifiedOperation(tc.callee))
			call.AnchorSymbol = diagramEvidenceQualifiedOperation(tc.callee)
			call.OwnerSymbol = tc.caller
			view := &types.AnswerSemanticView{
				Family: types.QFCallChain,
				RequiredMechanismAnchors: []types.AnswerRequiredAnchor{{
					Text: tc.caller, Kind: types.ContractTermSymbol,
				}},
			}
			if got := DiagramCallEdgeEvidenceMismatches(doc, view, []types.EvidenceItem{call}); len(got) != 0 {
				t.Fatalf("grounded owner-bound %s edge should pass all consumers: %+v", tc.name, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeResolvesFromUniqueTypedDefinition(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "schedule")
	call.AnchorSymbol = "schedule"
	evidence := []types.EvidenceItem{
		call,
		diagramEvidenceTestDefinition("VisitService", "schedule", "VisitService.java", 16),
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("a short typed callee plus one exact citable owner definition should resolve the qualified endpoint: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeWithoutDefinitionFailsClosed(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "schedule")
	call.AnchorSymbol = "schedule"
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{call})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a bare operation must not invent its qualified owner without definition evidence: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeWrongOwnerFailsClosed(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "schedule")
	call.AnchorSymbol = "schedule"
	evidence := []types.EvidenceItem{
		call,
		diagramEvidenceTestDefinition("OtherService", "schedule", "OtherService.java", 16),
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a definition for a different owner must not authorize the endpoint: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeOverloadFailsClosed(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "schedule")
	call.AnchorSymbol = "schedule"
	evidence := []types.EvidenceItem{
		call,
		diagramEvidenceTestDefinition("VisitService", "schedule", "VisitService.java", 16),
		diagramEvidenceTestDefinition("VisitService", "schedule", "VisitService.java", 24),
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("multiple distinct definitions of the same owner operation must remain ambiguous: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeRejectsContraryQualifiedObject(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "OtherService.schedule")
	call.AnchorSymbol = "schedule"
	evidence := []types.EvidenceItem{
		call,
		diagramEvidenceTestDefinition("VisitService", "schedule", "VisitService.java", 16),
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a conflicting qualified Object must not be laundered through a short AnchorSymbol: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_QualifiedCalleeRejectsContraryBareObject(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as VisitController.create\n" +
		"  participant B as VisitService.schedule\n" +
		"  A->>B: schedule(petId, reason)\n"
	call := diagramEvidenceTestCall("VisitController.create", "enqueue")
	call.AnchorSymbol = "schedule"
	evidence := []types.EvidenceItem{
		call,
		diagramEvidenceTestDefinition("VisitService", "schedule", "VisitService.java", 16),
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("a conflicting bare Object must not be overridden by AnchorSymbol: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_UnrelatedShortLabelDoesNotMatchCallee(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n" +
		"  participant A as Alpha.Run\n" +
		"  participant B as Other\n" +
		"  A->>B: invoke\n"
	evidence := diagramEvidenceTestCall("Alpha.Run", "normalizer.Run")
	evidence.AnchorSymbol = "Run"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{evidence}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("only exact Object/AnchorSymbol surfaces may authorize the destination: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_BodyEdgeCannotOmitTypedAnchor(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].EdgeAnchors = nil
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingAnchor || got[0].FromSymbol != "Alpha.Run" || got[0].ToSymbol != "Beta.Run" {
		t.Fatalf("a parsed sequence edge without typed metadata must hard-fail: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGBodyEdgeCannotOmitTypedAnchor(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Kind = types.DiagramCallDAG
	doc.Blocks[0].Diagram.Body = "flowchart TD\n  A[Alpha.Run] --> B[Beta.Run]\n"
	doc.Blocks[0].EdgeAnchors = nil
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingAnchor {
		t.Fatalf("a parsed call_dag edge without typed metadata must hard-fail: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_NormalizerCannotMintGuardAuthority(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "fabricated", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid",
			Body: "flowchart TD\n  A[Alpha.Run] -->|guard| B[Beta.Run]\n"},
	}}}
	if fixed := normalizeDiagramEdgeAnchorMetadata(doc); fixed != 0 {
		t.Fatalf("label text must not be promoted into typed edge authority: fixed=%d anchors=%+v", fixed, doc.Blocks[0].EdgeAnchors)
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingAnchor {
		t.Fatalf("an unanchored label-shaped guard must fail closed after normalization: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGAllowsTypedGuardEdges(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "mixed", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			"  C[VisitController.create] --> S[VisitService.schedule]",
			"  S -->|countOpenVisits >= max| X[throw IllegalStateException]",
			"  S -->|pass| R[VisitRepository.insert]",
			"  R --> A[AuditLog.record]",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "S", ToNode: "X", RelationKind: types.DiagramRelGuard, ClaimForm: types.ClaimGuardCondition},
			{FromNode: "S", ToNode: "R", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "R", ToNode: "A", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("VisitController.create", "VisitService.schedule"),
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.insert"),
		diagramEvidenceTestCall("VisitRepository.insert", "AuditLog.record"),
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("a typed guard edge in a mixed call DAG must not be reclassified as a function call: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGTypedCallCannotHideBehindGuardAnchorAcrossLanguages(t *testing.T) {
	tests := []struct {
		language string
		caller   string
		callee   string
	}{
		{"go", "service.Handle", "repo.Count"},
		{"python", "service.handle", "repo.count"},
		{"javascript", "Service.handle", "Repository.count"},
		{"typescript", "Service.handle", "Repository.count"},
		{"java", "VisitService.schedule", "VisitRepository.countOpenVisits"},
		{"kotlin", "VisitService.schedule", "VisitRepository.countOpenVisits"},
		{"rust", "service::handle", "repo::count"},
		{"c", "service_handle", "repo_count"},
		{"cpp", "Service::handle", "Repository::count"},
		{"ruby", "Service::handle", "Repository::count"},
		{"swift", "Service.handle", "Repository.count"},
		{"lua", "service.handle", "repo.count"},
		{"arkts", "VisitService.schedule", "VisitRepository.countOpenVisits"},
		{"cangjie", "clinic::VisitService::schedule", "clinic::VisitRepository::countOpenVisits"},
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "compound-condition", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramCallDAG, Language: "mermaid",
					Body: "flowchart TD\n  A[\"" + tc.caller + "\"] --> B[\"" + tc.callee + "\"]\n",
				},
				EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelGuard,
					ClaimForm: types.ClaimGuardCondition,
				}},
			}}}
			got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
				[]types.EvidenceItem{diagramEvidenceTestCall(tc.caller, tc.callee)})
			if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingAnchor {
				t.Fatalf("%s exact invocation must retain a typed call anchor even inside a compound guard: %+v", tc.language, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGAllowsCallAndGuardForSameCompoundEdge(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "compound-condition", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramCallDAG, Language: "mermaid",
			Body: "flowchart TD\n  S[VisitService.schedule] --> C[VisitRepository.countOpenVisits]\n",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "S", ToNode: "C", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "S", ToNode: "C", RelationKind: types.DiagramRelGuard, ClaimForm: types.ClaimGuardCondition},
		},
	}}}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.countOpenVisits")}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("a compound edge may preserve both its exact invocation and guard context: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_PrincipalPathCallMustAppearInDiagram(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "Service.handle"}, {Label: "Repository.count"}, {Label: "Repository.insert"}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramCallDAG, Language: "mermaid",
				Body: "flowchart TD\n  S[Service.handle] --> C[Repository.count]\n  I[Repository.insert]\n",
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "S", ToNode: "C", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
	}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Service.handle", "Repository.count"),
		diagramEvidenceTestCall("Service.handle", "Repository.insert"),
	}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssuePrincipalMiss ||
		got[0].FromSymbol != "Service.handle" || got[0].ToSymbol != "Repository.insert" {
		t.Fatalf("a typed call between model-selected principal hops must remain visible in the diagram: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_NonPrincipalTypedCallDoesNotExpandDiagram(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "Service.handle"}, {Label: "Repository.count"}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramCallDAG, Language: "mermaid",
				Body: "flowchart TD\n  S[Service.handle] --> C[Repository.count]\n",
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "S", ToNode: "C", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
	}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Service.handle", "Repository.count"),
		diagramEvidenceTestCall("Service.handle", "Background.refresh"),
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("supporting calls outside the model-selected principal path must not be forced into its diagram: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_PrincipalPathCompletenessAcrossExecutableLanguages(t *testing.T) {
	tests := []struct {
		language string
		caller   string
		callee   string
	}{
		{"go", "service.Handle", "repo.Insert"},
		{"python", "service.handle", "repo.insert"},
		{"javascript", "Service.handle", "Repository.insert"},
		{"typescript", "Service.handle", "Repository.insert"},
		{"java", "VisitService.schedule", "VisitRepository.insert"},
		{"kotlin", "VisitService.schedule", "VisitRepository.insert"},
		{"rust", "service::handle", "repo::insert"},
		{"c", "service_handle", "repo_insert"},
		{"cpp", "Service::handle", "Repository::insert"},
		{"ruby", "Service::handle", "Repository::insert"},
		{"swift", "Service.handle", "Repository.insert"},
		{"lua", "service.handle", "repo.insert"},
		{"arkts", "VisitService.schedule", "VisitRepository.insert"},
		{"cangjie", "clinic::VisitService::schedule", "clinic::VisitRepository::insert"},
	}
	covered := make(map[string]bool, len(tests))
	for _, tc := range tests {
		covered[tc.language] = true
	}
	for _, language := range rmtypes.SupportedReadLanguages() {
		if language == rmtypes.LangProto {
			continue // Proto is declarative and has no source invocation edge.
		}
		if !covered[language] {
			t.Fatalf("supported executable language %q has no principal call-diagram completeness fixture", language)
		}
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
				{
					ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
					FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
					ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
					// The visible item is intentionally an edge-shaped presentation,
					// not an exact endpoint identity. Completeness must come from its
					// typed citation_ref and never from parsing model-authored prose.
					Items: []types.AnswerBlockItem{{
						Label: tc.caller + " → " + tc.callee, CitationRef: 0,
					}},
				},
				{
					ID: "diagram", Kind: types.BlockDiagram,
					Diagram: &types.AnswerDiagramBlock{
						Kind: types.DiagramCallDAG, Language: "mermaid",
						Body: "flowchart TD\n  A[\"" + tc.caller + "\"]\n  B[\"" + tc.callee + "\"]\n",
					},
				},
			}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
			got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
				[]types.EvidenceItem{diagramEvidenceTestCall(tc.caller, tc.callee)})
			if len(got) != 1 || got[0].Issue != diagramCallEdgeIssuePrincipalMiss {
				t.Fatalf("%s principal typed call must remain visible in the diagram: %+v", tc.language, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ProtoDeclarationIsNotInventedAsCall(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "VisitService → VisitRequest", CitationRef: 0}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: "flowchart TD\n  S[VisitService]\n  R[VisitRequest]\n"},
		},
	}, Citations: []types.Citation{{File: "api/visit.proto", Line: 10}}}
	declaration := diagramEvidenceTestDefinition("VisitService", "VisitRequest", "api/visit.proto", 10)
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{declaration}); len(got) != 0 {
		t.Fatalf("the declarative Proto language must stay covered without inventing an executable call edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_PrincipalCitationCallAlreadyVisible(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items: []types.AnswerBlockItem{{
				Label: "VisitController.create → VisitService.schedule", CitationRef: 0,
			}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n  participant C as VisitController\n  participant S as VisitService\n  C->>S: schedule(petId)\n",
			},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
	}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
	evidence := diagramEvidenceTestCall("VisitController.create", "VisitService.schedule")
	evidence.AnchorSymbol = "schedule"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{evidence}); len(got) != 0 {
		t.Fatalf("a class-level presentation with exact operation must cover the citation-selected call: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_PrincipalMethodItemsShareClassParticipantResolver(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "VisitController.create"}, {Label: "VisitService.schedule"}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
				"sequenceDiagram",
				"  participant C as VisitController",
				"  participant S as VisitService",
				"  C->>S: schedule(petId)",
			}, "\n")},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
	}}
	evidence := diagramEvidenceTestCall("VisitController.create", "VisitService.schedule")
	evidence.AnchorSymbol = "schedule"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{evidence}); len(got) != 0 {
		t.Fatalf("principal completeness must reuse the class-participant typed resolver: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_AmbiguousPrincipalCitationFailsOpen(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "selected call", CitationRef: 0}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: "flowchart TD\n  S[Service.handle]\n"},
		},
	}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("Service.handle", "Repository.count"),
		diagramEvidenceTestCall("Service.handle", "Repository.insert"),
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("one citation resolving to distinct call directions must not let the system guess a required edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_PrincipalCitationToNonCallDoesNotForceEdge(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "selected row", CitationRef: 0}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: "flowchart TD\n  S[Service.handle]\n"},
		},
	}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
	evidence := diagramEvidenceTestDefinition("Service", "handle", "internal/example.go", 10)
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{evidence}); len(got) != 0 {
		t.Fatalf("a definition citation must not be promoted to a call edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SupportingCitationDoesNotExpandPrincipalDiagram(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "support", Kind: types.BlockOrderedList,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items:     []types.AnswerBlockItem{{Label: "Background.refresh → Cache.load", CitationRef: 0}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: "flowchart TD\n  S[Service.handle]\n"},
		},
	}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Background.refresh", "Cache.load")}); len(got) != 0 {
		t.Fatalf("supporting citation-selected calls must not expand the principal diagram: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGRespectsEveryTypedNonCallRelation(t *testing.T) {
	for _, relation := range []types.DiagramRelationKind{
		types.DiagramRelGuard,
		types.DiagramRelImport,
		types.DiagramRelPrecedence,
		types.DiagramRelContain,
		types.DiagramRelObserve,
	} {
		t.Run(string(relation), func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "typed-non-call", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramCallDAG, Language: "mermaid",
					Body: "flowchart TD\n  A[Source] --> B[Target]\n",
				},
				EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "A", ToNode: "B", RelationKind: relation,
					ClaimForm: types.ClaimFormForRelation(relation),
				}},
			}}}
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil); len(got) != 0 {
				t.Fatalf("typed %s edge must remain outside source-call authority: %+v", relation, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGUnanchoredControlEdgeStillFailsClosed(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Kind = types.DiagramCallDAG
	doc.Blocks[0].Diagram.Body = "flowchart TD\n  A[Alpha.Run] --> G[Guard]\n"
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "A", ToNode: "Other", RelationKind: types.DiagramRelGuard, ClaimForm: types.ClaimGuardCondition,
	}}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingAnchor {
		t.Fatalf("a non-matching guard anchor must not hide an unanchored DAG edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SequenceInvocationCannotUseGuardAnchor(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
		FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelGuard, ClaimForm: types.ClaimGuardCondition,
	}}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil)
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueMissingAnchor {
		t.Fatalf("a sequence invocation arrow must retain call authority even with a guard anchor: %+v", got)
	}
}

func TestRunPreEmitChecks_MixedCallDAGGuardStaysOutsideCallAuthority(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "mixed", Kind: types.BlockDiagram,
		ClaimUses: []types.RenderedClaimUse{
			{ClaimForm: types.ClaimCallEdge},
			{ClaimForm: types.ClaimGuardCondition},
		},
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			"  C[VisitController.create] --> S[VisitService.schedule]",
			"  S -->|countOpenVisits >= max| X[throw IllegalStateException]",
			"  S -->|pass| R[VisitRepository.insert]",
			"  R --> A[AuditLog.record]",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "S", ToNode: "X", RelationKind: types.DiagramRelGuard, ClaimForm: types.ClaimGuardCondition},
			{FromNode: "S", ToNode: "R", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "R", ToNode: "A", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}}}
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("VisitController.create", "VisitService.schedule"),
		diagramEvidenceTestCall("VisitService.schedule", "VisitRepository.insert"),
		diagramEvidenceTestCall("VisitRepository.insert", "AuditLog.record"),
		{
			ID: "guard", Kind: types.EvidenceConditional, Scope: types.ScopeLine,
			Source: "VisitService.java", LineStart: 18, AnchorKind: types.AnchorCondition,
			AnchorSymbol: "countOpenVisits", Subject: "VisitService.schedule",
			Condition: "countOpenVisits >= max", GroundingStatus: types.GroundingGrounded,
		},
	}
	mut := types.NewMutableState("mixed call and guard DAG")
	mut.AppendEvidence(evidence)
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil, &types.BusContext{Mutable: mut})
	for _, hint := range hints {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven {
			t.Fatalf("the wired pre-emit call authority must accept the typed mixed DAG: %+v", hints)
		}
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DefinitionCannotAuthorizeDirection(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	evidence := []types.EvidenceItem{{
		ID:              "ev-beta-definition",
		Kind:            types.EvidenceDirect,
		Subject:         "Beta.Run",
		Source:          "internal/example.go",
		LineStart:       20,
		AnchorKind:      types.AnchorDefinition,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}}
	got := DiagramCallEdgeEvidenceMismatches(diagramEvidenceTestDoc("A", "B"), view, evidence)
	if len(got) != 1 {
		t.Fatalf("a definition proves symbol existence, not Alpha.Run -> Beta.Run: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_AbsentEvidenceCannotAuthorizeDirection(t *testing.T) {
	got := DiagramCallEdgeEvidenceMismatches(diagramEvidenceTestDoc("A", "B"),
		&types.AnswerSemanticView{Family: types.QFCallChain}, nil)
	if len(got) != 1 || got[0].FromSymbol != "Alpha.Run" || got[0].ToSymbol != "Beta.Run" {
		t.Fatalf("an empty typed call-edge pool cannot authorize a structured edge: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DoesNotGateTraceProjectionFamily(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFRootCauseTrace}
	got := DiagramCallEdgeEvidenceMismatches(diagramEvidenceTestDoc("B", "A"), view,
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	if len(got) != 0 {
		t.Fatalf("runtime/root-cause trace diagrams must stay outside source call-edge authority: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ExplicitCallAuthorityIsFamilyIndependent(t *testing.T) {
	for _, family := range types.AllQuestionFamilies() {
		if family == types.QFRootCauseTrace {
			continue
		}
		t.Run(string(family), func(t *testing.T) {
			doc := diagramEvidenceTestDoc("A", "B")
			got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: family}, nil)
			if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
				t.Fatalf("explicit typed call must require same-direction evidence in family %s: %+v", family, got)
			}
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: family},
				[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")}); len(got) != 0 {
				t.Fatalf("exact typed call must pass in family %s: %+v", family, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DuplicateTypedParticipantIdentityIsDiagnosedFirst(t *testing.T) {
	for _, endpoint := range []string{"gate.RunWith", "gate::RunWith", "Gate#runWith", "run_with"} {
		for _, family := range []types.QuestionFamily{types.QFCallChain, types.QFGeneric, types.QFArchitecture} {
			t.Run(string(family)+"/"+endpoint, func(t *testing.T) {
				doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
					ID: "sequence", Kind: types.BlockDiagram,
					Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
						"sequenceDiagram",
						"  participant A as Caller.one",
						"  participant B as Caller.two",
						"  participant R as " + endpoint,
						"  participant R2 as " + endpoint,
						"  A->>R: invoke",
						"  B->>R2: invoke",
					}, "\n")},
					EdgeAnchors: []types.DiagramEdgeAnchor{
						{FromNode: "A", ToNode: "R", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
						{FromNode: "B", ToNode: "R2", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
					},
				}}}
				got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: family},
					[]types.EvidenceItem{
						diagramEvidenceTestCall("Caller.one", endpoint),
						diagramEvidenceTestCall("Caller.two", endpoint),
					})
				if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueDuplicateParticipant ||
					got[0].FromNode != "R" || got[0].ToNode != "R2" || got[0].FromSymbol != endpoint {
					t.Fatalf("duplicate typed endpoint diagnosis=%+v", got)
				}
			})
		}
	}
}

func TestDiagramCallEdgeEvidenceMismatches_DuplicateClassCarriersRemainOperationResolved(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant C as VisitController",
			"  participant S1 as VisitService",
			"  participant S2 as VisitService",
			"  C->>S1: schedule(petId)",
			"  C->>S2: cancel(petId)",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "C", ToNode: "S1", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "C", ToNode: "S2", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}}}
	schedule := diagramEvidenceTestCall("VisitController.create", "VisitService.schedule")
	schedule.AnchorSymbol = "schedule"
	cancel := diagramEvidenceTestCall("VisitController.create", "VisitService.cancel")
	cancel.AnchorSymbol = "cancel"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{schedule, cancel}); len(got) != 0 {
		t.Fatalf("class carriers with exact operation discriminators must remain legal: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_UnusedDuplicateDeclarationDoesNotCreateIdentityGate(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = strings.Replace(
		doc.Blocks[0].Diagram.Body,
		"  A->>B: invoke\n",
		"  participant B2 as Beta.Run\n  A->>B: invoke\n",
		1,
	)
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain},
		[]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")}); len(got) != 0 {
		t.Fatalf("an unused presentation declaration cannot make a grounded edge ambiguous: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_CallDAGDuplicateTypedNodeIdentity(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "dag", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			"  A[Caller.run] --> R[gate.RunWith]",
			"  G[gate.Run] --> R2[gate.RunWith]",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "R", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
			{FromNode: "G", ToNode: "R2", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}}}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{
		diagramEvidenceTestCall("Caller.run", "gate.RunWith"),
		diagramEvidenceTestCall("gate.Run", "gate.RunWith"),
	})
	if len(got) != 1 || got[0].Issue != diagramCallEdgeIssueDuplicateParticipant || got[0].BlockID != "dag" {
		t.Fatalf("call-DAG duplicate typed identity diagnosis=%+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ClassParticipantAliasIsFamilyIndependent(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "sequence", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
			"sequenceDiagram",
			"  participant C as VisitController",
			"  participant S as VisitService",
			"  C->>S: schedule(petId)",
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
		}},
	}}}
	call := diagramEvidenceTestCall("VisitController.create", "VisitService.schedule")
	call.AnchorSymbol = "schedule"
	for _, family := range []types.QuestionFamily{types.QFCallChain, types.QFGeneric, types.QFArchitecture, types.QFRoleLookup} {
		t.Run(string(family), func(t *testing.T) {
			if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: family}, []types.EvidenceItem{call}); len(got) != 0 {
				t.Fatalf("same typed graph/evidence must not drift by family %s: %+v", family, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SiblingCarrierUsesUniqueDocumentDiagramIdentity(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "sequence", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n  participant C as VisitController\n  participant S as VisitService\n  C->>S: schedule(petId)\n"},
		},
		{
			ID: "edge-carrier", Kind: types.BlockOrderedList,
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "C", ToNode: "S", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
	}}
	call := diagramEvidenceTestCall("VisitController.create", "VisitService.schedule")
	call.AnchorSymbol = "schedule"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, []types.EvidenceItem{call}); len(got) != 0 {
		t.Fatalf("sibling anchor must consume the unique document-level node/operation registry: %+v", got)
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: "conflict", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n  participant C as OtherController\n"},
	})
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, []types.EvidenceItem{call}); len(got) != 1 || got[0].Issue != diagramCallEdgeIssueNoEvidence {
		t.Fatalf("conflicting document-level node identities must fail closed: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_ActivatedReplyKeepsStructuralRole(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n  participant A as Alpha.Run\n  participant B as Beta.Run\n  A->>+B: invoke\n  B-->>-A: result\n"
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, []types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")}); len(got) != 0 {
		t.Fatalf("activation suffix must not turn a paired dashed reply into a reverse call: %+v", got)
	}
}

func TestDiagramCallEdgeEvidenceMismatches_GenericLogicalRelationsRemainAvailable(t *testing.T) {
	for _, relation := range []types.DiagramRelationKind{
		types.DiagramRelGuard,
		types.DiagramRelImport,
		types.DiagramRelPrecedence,
		types.DiagramRelContain,
		types.DiagramRelObserve,
	} {
		t.Run(string(relation), func(t *testing.T) {
			doc := diagramEvidenceTestDoc("A", "B")
			doc.Blocks[0].EdgeAnchors[0].RelationKind = relation
			doc.Blocks[0].EdgeAnchors[0].ClaimForm = types.ClaimFormForRelation(relation)
			if got := DiagramCallEdgeEvidenceMismatches(doc,
				&types.AnswerSemanticView{Family: types.QFGeneric}, nil); len(got) != 0 {
				t.Fatalf("generic typed %s relation must not be recast as a source call: %+v", relation, got)
			}
		})
	}
}

func TestDiagramCallEdgeEvidenceMismatches_SiblingCarrierCallStillNeedsEvidence(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "diagram-edge-carrier", Kind: types.BlockOrderedList,
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "Parser.Parse", ToNode: "Decoder.Decode",
			RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
		}},
	}}}
	got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, nil)
	if len(got) != 1 || got[0].FromSymbol != "Parser.Parse" || got[0].ToSymbol != "Decoder.Decode" {
		t.Fatalf("a sibling carrier must not bypass explicit call authority: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFGeneric},
		[]types.EvidenceItem{diagramEvidenceTestCall("Parser.Parse", "Decoder.Decode")}); len(got) != 0 {
		t.Fatalf("a sibling carrier with exact typed authority should pass: %+v", got)
	}
}

func TestRunPreEmitChecks_GenericExplicitCallEdgeEvidenceAlignmentIsWired(t *testing.T) {
	mut := types.NewMutableState("generic diagram call edge")
	ctx := &types.BusContext{Mutable: mut}
	hints := runPreEmitChecks(diagramEvidenceTestDoc("A", "B"),
		&types.AnswerSemanticView{Family: types.QFGeneric}, nil, ctx)
	for _, hint := range hints {
		if hint.Kind != types.ViolDiagramCallEdgeUnproven {
			continue
		}
		hard, _ := splitPreEmitHintsByGate([]emitFixHint{hint})
		if len(hard) != 1 {
			t.Fatalf("generic explicit typed call must use the precise hard lane: %+v", hints)
		}
		return
	}
	t.Fatalf("generic explicit typed call bypassed pre-emit authority: %+v", hints)
}

func TestRunPreEmitChecks_DiagramCallEdgeEvidenceAlignmentIsWired(t *testing.T) {
	mut := types.NewMutableState("diagram call edge")
	mut.AppendEvidence([]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	ctx := &types.BusContext{Mutable: mut}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	hints := runPreEmitChecks(diagramEvidenceTestDoc("B", "A"), view, nil, ctx)
	found := false
	for _, hint := range hints {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven && strings.Contains(hint.Field, "edge_anchors") &&
			strings.Contains(hint.ExpectedShape, "Beta.Run") && strings.Contains(hint.ExpectedShape, "Alpha.Run") {
			found = true
		}
	}
	if !found {
		t.Fatalf("pre-emit dispatch did not publish the structured edge-direction diagnosis: %+v", hints)
	}
	hard, _ := splitPreEmitHintsByGate(hints)
	if len(hard) != 1 || hard[0].Kind != types.ViolDiagramCallEdgeUnproven {
		t.Fatalf("typed source call-edge mismatch must reject same-turn: %+v", hints)
	}
}

func TestRunPreEmitChecks_DiagramBodyEdgeWithoutAnchorIsWired(t *testing.T) {
	mut := types.NewMutableState("diagram body edge")
	mut.AppendEvidence([]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	ctx := &types.BusContext{Mutable: mut}
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].EdgeAnchors = nil
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil, ctx)
	for _, hint := range hints {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven &&
			strings.Contains(hint.ExpectedShape, diagramCallEdgeIssueMissingAnchor) {
			hard, _ := splitPreEmitHintsByGate(hints)
			if len(hard) == 1 && hard[0].Kind == types.ViolDiagramCallEdgeUnproven {
				return
			}
		}
	}
	t.Fatalf("pre-emit dispatch must hard-reject a body edge whose typed anchor was omitted: %+v", hints)
}

func TestRunPreEmitChecks_DuplicateParticipantIdentityGivesSingleExecutableRepair(t *testing.T) {
	doc := diagramEvidenceTestDoc("A", "B")
	doc.Blocks[0].Diagram.Body = strings.Replace(
		doc.Blocks[0].Diagram.Body,
		"  A->>B: invoke\n",
		"  participant B2 as Beta.Run\n  A->>B: invoke\n  A->>B2: invoke\n",
		1,
	)
	doc.Blocks[0].EdgeAnchors = append(doc.Blocks[0].EdgeAnchors, types.DiagramEdgeAnchor{
		FromNode: "A", ToNode: "B2", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
	})
	mut := types.NewMutableState("duplicate participant identity")
	mut.AppendEvidence([]types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")})
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil, &types.BusContext{Mutable: mut})
	var matches []emitFixHint
	for _, hint := range hints {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven {
			matches = append(matches, hint)
		}
	}
	if len(matches) != 1 || !strings.Contains(matches[0].ExpectedShape, diagramCallEdgeIssueDuplicateParticipant) &&
		!strings.Contains(matches[0].ExpectedShape, "identity=Beta.Run aliases=B,B2") {
		t.Fatalf("duplicate participant must produce one precise repair: %+v", hints)
	}
	for _, want := range []string{"reuse that one alias", "verbatim alias IDs", "Remove the duplicate participant declaration"} {
		if !strings.Contains(matches[0].ExpectedShape, want) {
			t.Fatalf("duplicate repair missing %q: %+v", want, matches[0])
		}
	}
	hard, _ := splitPreEmitHintsByGate(matches)
	if len(hard) != 1 {
		t.Fatalf("typed duplicate identity diagnosis must remain a single hard repair: %+v", matches)
	}
}

func TestRunPreEmitChecks_PrincipalPathDiagramCompletenessIsWired(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			FacetIDs:  []string{string(types.FacetPrincipalPathEdge)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			Items: []types.AnswerBlockItem{{
				Label: "Service.handle → Repository.insert", CitationRef: 0,
			}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramCallDAG, Language: "mermaid",
				Body: "flowchart TD\n  S[Service.handle]\n  I[Repository.insert]\n",
			},
		},
	}, Citations: []types.Citation{{File: "internal/example.go", Line: 10}}}
	mut := types.NewMutableState("principal path diagram completeness")
	mut.AppendEvidence([]types.EvidenceItem{diagramEvidenceTestCall("Service.handle", "Repository.insert")})
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil,
		&types.BusContext{Mutable: mut})
	for _, hint := range hints {
		if hint.Kind == types.ViolDiagramCallEdgeUnproven &&
			strings.Contains(hint.ExpectedShape, diagramCallEdgeIssuePrincipalMiss) &&
			strings.Contains(hint.ExpectedShape, "Service.handle") &&
			strings.Contains(hint.ExpectedShape, "Repository.insert") {
			hard, _ := splitPreEmitHintsByGate(hints)
			if len(hard) == 1 && hard[0].Kind == types.ViolDiagramCallEdgeUnproven {
				return
			}
		}
	}
	t.Fatalf("principal typed call omitted from diagram must be wired to the hard typed gate: %+v", hints)
}
