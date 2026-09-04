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

func typedRelationBridgeLabeledDocument(from, to string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "type-relations",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramArchitecture,
			Language: "mermaid",
			Body: "flowchart TD\n  A[\"" + from + "\"]\n  B[\"" + to +
				"\"]\n  A --> B",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B", FromIdentity: from, ToIdentity: to,
			RelationKind: types.DiagramRelTypeRelation,
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

func TestPreCheckDiagramCallEdgeEvidenceAlignment_FlowAliasKeepsExplicitLabelAcrossBareSubgraphReference(t *testing.T) {
	ctx := typedRelationBridgeTestContext(exactImplementerCandidate("internal/agent/analyzer.go"))
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "type-relations", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramArchitecture, Language: "mermaid",
			Body: strings.Join([]string{
				"flowchart TD",
				`  AE["analyzerEvaluator\ninternal/agent/analyzer.go:49"] -->|implements| LC["LoopController\ninternal/agent/agent.go:519"]`,
				`  subgraph agent["internal/agent"]`,
				"    AE",
				"  end",
			}, "\n"),
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "AE", ToNode: "LC", RelationKind: types.DiagramRelTypeRelation,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisImplement}

	labels := diagramEvidenceNodeLabels(doc.Blocks[0].Diagram.Body, types.DiagramArchitecture)
	if got := labels["ae"]; got != `analyzerEvaluator\ninternal/agent/analyzer.go:49` {
		t.Fatalf("bare subgraph reference must inherit the unique explicit node label, got %q labels=%+v", got, labels)
	}
	if hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, newPreEmitCheckContext(ctx)); len(hints) != 0 {
		t.Fatalf("exact provider must authorize an alias whose later bare reference only places it in a subgraph: %+v", hints)
	}
}

func TestPreCheckDiagramCallEdgeEvidenceAlignment_InlineImplementsLabelSurvivesProductionNormalization(t *testing.T) {
	ctx := typedRelationBridgeTestContext(exactImplementerCandidate("internal/agent/analyzer.go"))
	wireDiagram := &emitAnswerDiagramV2{
		Kind:     string(types.DiagramArchitecture),
		Language: "mermaid",
		Body:     "flowchart TD\n  AE[\"analyzerEvaluator\"] -.implements.-> LC[\"LoopController\"]",
	}
	normalizeEmitAnswerDiagram(wireDiagram)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "type-relations", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramArchitecture, Language: "mermaid", Body: wireDiagram.Body,
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "AE", ToNode: "LC", RelationKind: types.DiagramRelTypeRelation,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisImplement}

	if strings.Contains(wireDiagram.Body, "codraxNode") ||
		!strings.Contains(wireDiagram.Body, `AE["analyzerEvaluator"] -.->|implements| LC["LoopController"]`) {
		t.Fatalf("production normalization changed inline implements topology: %q", wireDiagram.Body)
	}
	if hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, newPreEmitCheckContext(ctx)); len(hints) != 0 {
		t.Fatalf("normalized inline implements relation and exact typed anchor must pass together: %+v", hints)
	}
}

func TestDiagramEvidenceNodeLabels_TwoExplicitLabelsRemainAmbiguousDespiteBareReference(t *testing.T) {
	body := strings.Join([]string{
		"flowchart TD",
		`  A["First.run"]`,
		`  A["Second.run"]`,
		"  A",
	}, "\n")
	if labels := diagramEvidenceNodeLabels(body, types.DiagramArchitecture); labels["a"] != "" {
		t.Fatalf("two distinct explicit labels must remain fail-closed; bare reference cannot choose one: %+v", labels)
	}
}

func TestPreCheckDiagramCallEdgeEvidenceAlignment_ExactProviderOwnsCanonicalClassDiagramEdge(t *testing.T) {
	ctx := typedRelationBridgeTestContext(exactImplementerCandidate("internal/agent/analyzer.go"))
	doc := typedRelationBridgeClassDiagramDocument("analyzerEvaluator", "LoopController")
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

// §40.57 V10-4 (production witness r417/r418, B698): the model draws
// `LoopController <|.. analyzerEvaluator` (UML: analyzerEvaluator -> LoopController)
// but serializes the sibling type_relation anchor in visual token order. The
// gate must report that reversal precisely and teach alignment — swap the
// anchor or reverse the arrow — never delete-the-diagram, and never swap the
// anchor on the model's behalf.
func TestPreCheckDiagramCallEdgeEvidenceAlignment_ReversedClassDiagramAnchorTeachesSwapNotDeletion(t *testing.T) {
	ctx := typedRelationBridgeTestContext(exactImplementerCandidate("internal/agent/analyzer.go"))
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisImplement}
	doc := typedRelationBridgeClassDiagramDocument("LoopController", "analyzerEvaluator")

	hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, newPreEmitCheckContext(ctx))
	if len(hints) != 1 {
		t.Fatalf("reversed anchor must produce exactly one relation-gate hint: %+v", hints)
	}
	hint := hints[0]
	issues := strings.Join(hint.DiagramRelationFailureIssues, ",")
	if !strings.Contains(issues, diagramCallEdgeIssueAnchorReversedAgainstVisibleEdge) ||
		strings.Contains(issues, diagramCallEdgeIssueAnchorWithoutBodyEdge) {
		t.Fatalf("issue set must carry the reversed issue and not the generic stale issue: %q", issues)
	}
	if !strings.Contains(hint.ExpectedShape, diagramReversedAnchorBoundaryTeaching) ||
		!strings.Contains(hint.ExpectedShape, "swapping from_node/to_node (and from_identity/to_identity together)") ||
		!strings.Contains(hint.ExpectedShape, "Do not delete the diagram") {
		t.Fatalf("hint must teach alignment, not deletion:\n%s", hint.ExpectedShape)
	}
	if doc.Blocks[0].EdgeAnchors[0].FromNode != "LoopController" || doc.Blocks[0].EdgeAnchors[0].ToNode != "analyzerEvaluator" {
		t.Fatalf("pre-emit gate must never rewrite the model anchor: %+v", doc.Blocks[0].EdgeAnchors)
	}

	// Typed escape lane, full-emit shape: the model aligns its anchor to its
	// own arrow and the same provider authorizes the diagram with zero hints.
	aligned := typedRelationBridgeClassDiagramDocument("analyzerEvaluator", "LoopController")
	if hints := preCheckDiagramCallEdgeEvidenceAlignment(aligned, view, newPreEmitCheckContext(ctx)); len(hints) != 0 {
		t.Fatalf("swapped anchor must clear the gate: %+v", hints)
	}
}

func typedRelationBridgeClassDiagramDocument(from, to string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "type-relations", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramArchitecture,
			Language: "mermaid",
			Body:     "classDiagram\n  class LoopController\n  class analyzerEvaluator\n  LoopController <|.. analyzerEvaluator",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: from, ToNode: to, RelationKind: types.DiagramRelTypeRelation,
		}},
	}}}
}

func TestDiagramCallEdgeEvidenceMismatchesWithRuntimeContext_UsesExactTypedRelationProvider(t *testing.T) {
	ctx := typedRelationBridgeTestContext(exactImplementerCandidate("internal/agent/analyzer.go"))
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisImplement}

	doc := typedRelationBridgeClassDiagramDocument("analyzerEvaluator", "LoopController")
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc, view, nil); len(got) != 0 {
		t.Fatalf("post-finalizer must consume the same exact typed provider as pre-emit: %+v", got)
	}

	// EVOLUTION RECORD (§40.57 V10-4): tightened from "non-empty" to the
	// exact issue set so pre-emit and post-finalizer share one diagnosis —
	// the reversed anchor is reported as reversed (alignment teaching), the
	// visible class edge keeps its missing-owner row, and nothing is swapped.
	reversed := typedRelationBridgeClassDiagramDocument("LoopController", "analyzerEvaluator")
	got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, reversed, view, nil)
	gotIssues := map[string]int{}
	for _, m := range got {
		gotIssues[m.Issue]++
	}
	if len(got) != 2 || gotIssues[diagramCallEdgeIssueAnchorReversedAgainstVisibleEdge] != 1 ||
		gotIssues[diagramCallEdgeIssueMissingRelationAnchor] != 1 {
		t.Fatalf("post-finalizer exact provider must not authorize the reverse edge and must name the reversal exactly: %+v", got)
	}
	if reversed.Blocks[0].EdgeAnchors[0].FromNode != "LoopController" || reversed.Blocks[0].EdgeAnchors[0].ToNode != "analyzerEvaluator" {
		t.Fatalf("the gate must never rewrite the model anchor: %+v", reversed.Blocks[0].EdgeAnchors)
	}

	nameOnly := exactImplementerCandidate("internal/agent/analyzer.go")
	nameOnly.Precision = types.TypedRelationPrecisionNameOnly
	nameOnlyCtx := typedRelationBridgeTestContext(nameOnly)
	got = DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(nameOnlyCtx, doc, view, nil)
	if len(got) == 0 || got[0].Issue != diagramTypeRelationEdgeIssueNoEvidence {
		t.Fatalf("post-finalizer must not promote name-only relation hints: %+v", got)
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
			doc := typedRelationBridgeLabeledDocument("analyzerEvaluator", "LoopController")
			rows := preEmitEvidenceWithExactTypedDiagramRelations(doc, ctx, nil)
			if len(rows) != 1 || !types.IsRepoMapTypeRelationEvidence(rows[0]) ||
				rows[0].Subject != "analyzerEvaluator" || rows[0].Object != "LoopController" || !rows[0].IsCitable() {
				t.Fatalf("file %q did not produce one exact cross-language type relation: %+v", file, rows)
			}
		})
	}
}

func TestDiagramParticipantCoverage_ExactTypedRelationProviderIsSharedAcrossLanguagesAndPasses(t *testing.T) {
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
			ctx := typedRelationBridgeTestContext(exactImplementerCandidate(file))
			rm := ctx.AnalysisIR.RequestModel
			rm.Intent = types.IntentExplain
			rm.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramArchitecture, Required: true,
				Participants: []types.DiagramParticipantHint{{
					Identity: "LoopController", Role: types.DiagramParticipantIncidentRequired,
				}},
			}
			ctx.AnalysisIR.RequestModel = rm
			view := &types.AnswerSemanticView{
				Family: types.QFGeneric, RelationAxis: types.AxisImplement,
				DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramArchitecture, Required: true},
				DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...),
			}
			doc := typedRelationBridgeLabeledDocument("analyzerEvaluator", "LoopController")

			if got := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, doc, view, nil); len(got) != 0 {
				t.Fatalf("post-finalizer participant gate rejected exact provider relation for %q: %+v", file, got)
			}
			if hints := preCheckDiagramParticipantCoverage(doc, view, newPreEmitCheckContext(ctx)); len(hints) != 0 {
				t.Fatalf("pre-emit participant gate rejected exact provider relation for %q: %+v", file, hints)
			}
		})
	}
}

func TestDiagramParticipantCoverage_ExactTypedRelationRemainsDirectionalAndExact(t *testing.T) {
	base := exactImplementerCandidate("internal/agent/analyzer.go")
	build := func(candidate types.TypedRelationCandidate, from, to string) (*types.BusContext, *types.AnswerDocumentV2, *types.AnswerSemanticView) {
		ctx := typedRelationBridgeTestContext(candidate)
		rm := ctx.AnalysisIR.RequestModel
		rm.Intent = types.IntentExplain
		rm.DiagramHint = &types.DiagramHint{
			Kind: types.DiagramArchitecture, Required: true,
			Participants: []types.DiagramParticipantHint{{
				Identity: "LoopController", Role: types.DiagramParticipantIncidentRequired,
			}},
		}
		ctx.AnalysisIR.RequestModel = rm
		return ctx, typedRelationBridgeLabeledDocument(from, to), &types.AnswerSemanticView{
			Family: types.QFGeneric, RelationAxis: types.AxisImplement,
			DiagramPlan:                   &types.DiagramFacetGraph{Kind: types.DiagramArchitecture, Required: true},
			DiagramParticipantObligations: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...),
		}
	}

	nameOnly := base
	nameOnly.Precision = types.TypedRelationPrecisionNameOnly
	ctx, doc, view := build(nameOnly, "analyzerEvaluator", "LoopController")
	if got := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, doc, view, nil); len(got) != 1 ||
		got[0].Issue != DiagramParticipantCoverageMissingBoundary {
		t.Fatalf("name-only relation must not satisfy participant authority: %+v", got)
	}

	ctx, doc, view = build(base, "LoopController", "analyzerEvaluator")
	if got := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, doc, view, nil); len(got) == 0 {
		t.Fatalf("reverse relation must not satisfy participant authority: %+v", got)
	}

	unrelated := base
	unrelated.SourceName = "OtherController"
	ctx, doc, view = build(unrelated, "analyzerEvaluator", "LoopController")
	if got := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, doc, view, nil); len(got) == 0 {
		t.Fatalf("unrelated exact target must not satisfy request-scoped participant authority: %+v", got)
	}
}

func TestExactTypedRelationEvidenceForRequest_IsPromptValidatorSharedCarrier(t *testing.T) {
	ctx := typedRelationBridgeTestContext(exactImplementerCandidate("internal/agent/analyzer.go"))
	rows := ExactTypedRelationEvidenceForRequest(ctx, ctx.AnalysisIR.RequestModel)
	if len(rows) != 1 || !types.IsRepoMapTypeRelationEvidence(rows[0]) ||
		rows[0].Subject != "analyzerEvaluator" || rows[0].Object != "LoopController" ||
		rows[0].Predicate != "implements" || !rows[0].IsCitable() {
		t.Fatalf("shared prompt/validator carrier lost exact typed direction: %+v", rows)
	}

	nameOnly := exactImplementerCandidate("internal/agent/analyzer.go")
	nameOnly.Precision = types.TypedRelationPrecisionNameOnly
	nameOnlyCtx := typedRelationBridgeTestContext(nameOnly)
	if rows := ExactTypedRelationEvidenceForRequest(nameOnlyCtx, nameOnlyCtx.AnalysisIR.RequestModel); len(rows) != 0 {
		t.Fatalf("shared carrier must fail closed on name-only relation rows: %+v", rows)
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
