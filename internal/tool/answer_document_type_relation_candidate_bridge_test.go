package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func typedRelationBridgeTestContext(candidate types.TypedRelationCandidate) *types.BusContext {
	return &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisImplement,
			AnalyzerHints: types.AnalyzerHints{PrimaryEntities: []string{"LoopController"}},
		}},
		Mutable:    types.NewMutableState("type relation diagram"),
		MultiGraph: typedRelationCandidateSourceFixture{candidate},
	}
}

func typedRelationBridgeTestDocument(from, to string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "type-relations",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramCallDAG,
			Language: "mermaid",
			Body:     "flowchart TD\n  " + from + " --> " + to,
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: from, ToNode: to, RelationKind: types.DiagramRelTypeRelation,
		}},
	}}}
}

func exactImplementerCandidate(file string) types.TypedRelationCandidate {
	return types.TypedRelationCandidate{
		Relation:   types.TypedRelationImplements,
		SourceName: "LoopController",
		SourceKind: "interface",
		Member: types.TypedRelationMember{
			Name: "analyzerEvaluator", File: file, Line: 49, Kind: "struct",
			SourceRole: types.ClassifySourcePathRole(file), Distance: 1,
		},
		Carrier:   types.TypedRelationCarrierGraph,
		Precision: types.TypedRelationPrecisionExactSymbolID,
	}
}

func TestPreCheckDiagramCallEdgeEvidenceAlignment_ExactTypedImplementerProviderOwnsEdge(t *testing.T) {
	ctx := typedRelationBridgeTestContext(exactImplementerCandidate("internal/agent/analyzer.go"))
	doc := typedRelationBridgeTestDocument("analyzerEvaluator", "LoopController")
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisImplement}

	if hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, newPreEmitCheckContext(ctx)); len(hints) != 0 {
		t.Fatalf("exact provider implementer relation must own the model-authored edge: %+v", hints)
	}

	reversed := typedRelationBridgeTestDocument("LoopController", "analyzerEvaluator")
	hints := preCheckDiagramCallEdgeEvidenceAlignment(reversed, view, newPreEmitCheckContext(ctx))
	if len(hints) != 1 || !strings.Contains(hints[0].ExpectedShape, diagramTypeRelationEdgeIssueNoEvidence) {
		t.Fatalf("exact provider relation must not authorize the reverse edge: %+v", hints)
	}
}

func TestPreCheckDiagramCallEdgeEvidenceAlignment_ExactProviderOwnsCanonicalClassDiagramEdge(t *testing.T) {
	ctx := typedRelationBridgeTestContext(exactImplementerCandidate("internal/agent/analyzer.go"))
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "type-relations", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramArchitecture,
			Language: "mermaid",
			Body:     "classDiagram\n  class LoopController\n  class analyzerEvaluator\n  LoopController <|.. analyzerEvaluator",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "analyzerEvaluator", ToNode: "LoopController", RelationKind: types.DiagramRelTypeRelation,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisImplement}

	if hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, newPreEmitCheckContext(ctx)); len(hints) != 0 {
		t.Fatalf("exact provider relation must authorize the canonical classDiagram realization edge: %+v", hints)
	}

	doc.Blocks[0].EdgeAnchors = nil
	hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, newPreEmitCheckContext(ctx))
	if len(hints) != 1 || !strings.Contains(hints[0].ExpectedShape, diagramCallEdgeIssueMissingRelationAnchor) {
		t.Fatalf("classDiagram edge must not bypass ownership by deleting its typed anchor: %+v", hints)
	}
}

func TestPreCheckDiagramCallEdgeEvidenceAlignment_NameOnlyTypedCandidateCannotOwnEdge(t *testing.T) {
	candidate := exactImplementerCandidate("internal/agent/analyzer.go")
	candidate.Precision = types.TypedRelationPrecisionNameOnly
	ctx := typedRelationBridgeTestContext(candidate)
	doc := typedRelationBridgeTestDocument("analyzerEvaluator", "LoopController")
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisImplement}

	hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, newPreEmitCheckContext(ctx))
	if len(hints) != 1 || !strings.Contains(hints[0].ExpectedShape, diagramTypeRelationEdgeIssueNoEvidence) {
		t.Fatalf("prompt/name-only candidate must remain unable to satisfy a hard edge gate: %+v", hints)
	}
}

func TestPreEmitEvidenceWithExactTypedDiagramRelations_CrossLanguageCarrierIsExtensionAgnostic(t *testing.T) {
	files := []string{
		"controller.go",
		"src/Controller.java",
		"src/Controller.kt",
		"src/controller.ets",
		"src/controller.cj",
		"include/controller.hpp",
		"src/controller.rs",
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			candidate := exactImplementerCandidate(file)
			ctx := typedRelationBridgeTestContext(candidate)
			doc := typedRelationBridgeTestDocument("analyzerEvaluator", "LoopController")
			rows := preEmitEvidenceWithExactTypedDiagramRelations(doc, ctx, nil)
			if len(rows) != 1 || !types.IsRepoMapTypeRelationEvidence(rows[0]) ||
				rows[0].Subject != "analyzerEvaluator" || rows[0].Object != "LoopController" || !rows[0].IsCitable() {
				t.Fatalf("file %q did not produce one exact cross-language type relation: %+v", file, rows)
			}
		})
	}
}

func TestPreEmitEvidenceWithExactTypedDiagramRelations_DoesNotMintUnrequestedEdge(t *testing.T) {
	ctx := typedRelationBridgeTestContext(exactImplementerCandidate("internal/agent/analyzer.go"))
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "plain", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: "flowchart TD\n  A\n  B"},
	}}}
	if rows := preEmitEvidenceWithExactTypedDiagramRelations(doc, ctx, nil); len(rows) != 0 {
		t.Fatalf("provider rows must not enter diagram evidence unless the model authored type_relation: %+v", rows)
	}
}
