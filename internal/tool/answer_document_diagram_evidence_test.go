package tool

import (
	"strings"
	"testing"

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
