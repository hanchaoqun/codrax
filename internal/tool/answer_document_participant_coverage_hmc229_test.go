package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc229ParticipantContext(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	ctx, rows := hmc228CausalDiagramContext(t, &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis})
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{Kind: types.DiagramSequence, Required: true, RelationScopeQuote: "Main, Worker, and Storage", Participants: []types.DiagramParticipantHint{
		{Identity: "Main", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Main"},
		{Identity: "Worker", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Worker"},
		{Identity: "Storage", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "Storage"},
	}}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	ctx.AnalysisIR.AnswerContract.Diagram = &types.DiagramContract{Required: true, Minimum: 1, RequiredKind: types.DiagramSequence, PreferredKinds: []types.DiagramKind{types.DiagramSequence}}
	ctx.InvalidateAnswerPlanCache()
	doc := hmc229NativeDiagram(rows)
	doc.Blocks[2].Diagram.Body = "sequenceDiagram\n participant M as Main\n participant W as Worker\n participant S as Storage\n W->>M: observed wakeup\n"
	doc.Blocks[2].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "W", ToNode: "M", FromIdentity: rows[0].FromIdentity, ToIdentity: rows[0].ToIdentity, RelationKind: rows[0].Kind, VisibleLabel: "observed wakeup"}}
	return ctx, doc
}

func hmc229NativeDiagram(rows []RuntimeDiagramRelation) *types.AnswerDocumentV2 {
	doc := hmc228WakeDiagram(rows)
	doc.Blocks[2].FacetIDs = []string{string(types.FacetDiagramSpine), string(types.FacetObservedArtifactFact)}
	return doc
}

func hmc229ParticipantSetNativeHints(ctx *types.BusContext, rows []RuntimeDiagramRelation) {
	var participants []types.DiagramParticipantHint
	seen := map[string]bool{}
	for _, row := range rows {
		for _, node := range []string{row.FromNode, row.ToNode} {
			if !seen[node] {
				seen[node] = true
				participants = append(participants, types.DiagramParticipantHint{Identity: node, Role: types.DiagramParticipantIncidentRequired, SourceQuote: node})
			}
		}
	}
	ctx.AnalysisIR.RequestModel.DiagramHint.Participants = participants
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	ctx.InvalidateAnswerPlanCache()
}

func TestHMC229ParticipantPublicEmitKeepsBoundedBusinessAndExactNativeRelations(t *testing.T) {
	for _, mode := range []string{"business_unproven", "native_endpoint"} {
		t.Run(mode, func(t *testing.T) {
			ctx, _ := hmc229ParticipantContext(t)
			rows := runtimeDiagramRelationsForContext(ctx)
			doc := hmc229NativeDiagram(rows)
			if mode == "native_endpoint" {
				hmc229ParticipantSetNativeHints(ctx, rows)
			} else {
				for _, name := range []string{"Main", "Worker", "Storage"} {
					doc.Blocks[2].Diagram.Body += " participant " + name + "\n"
					doc.Blocks[2].ParticipantBoundaries = append(doc.Blocks[2].ParticipantBoundaries, types.DiagramParticipantBoundary{Participant: name, Status: types.DiagramParticipantBoundaryUnproven})
				}
			}
			raw, _ := json.Marshal(doc)
			result, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || !result.Success {
				t.Fatalf("explicit evidence boundary/native relation should remain usable: %v %s", err, result.Summary)
			}
			accepted := ctx.Mutable.AnswerDocumentV2()
			if got := projectionClusterBlock(accepted.Blocks, "sequence"); got == nil || !reflect.DeepEqual(got.EdgeAnchors, doc.Blocks[2].EdgeAnchors) {
				t.Fatal("coverage must not construct or replace the actual wakeup edges")
			}
			receipt, ok := AcceptedDiagramParticipantCoverageReceiptWithRuntimeContext(ctx, accepted, types.BuildAnswerSemanticViewForBusContext(ctx), nil)
			boundaries := 0
			if mode == "business_unproven" {
				boundaries = 3
			}
			if !ok || receipt.Required != 3 || receipt.Covered != 3 || receipt.UnprovenBoundaries != boundaries {
				t.Fatalf("coverage receipt must distinguish proved edges and honest boundaries: %+v %t", receipt, ok)
			}
		})
	}
}

func TestHMC229ParticipantNativeRepairUsesCurrentTupleNotLabelOrPrivateSibling(t *testing.T) {
	for _, mode := range []string{"missing", "label_only", "decorated_entity", "foreign_tuple", "private_sibling", "private_only"} {
		t.Run(mode, func(t *testing.T) {
			ctx, _ := hmc229ParticipantContext(t)
			rows := runtimeDiagramRelationsForContext(ctx)
			hmc229ParticipantSetNativeHints(ctx, rows)
			doc := hmc229NativeDiagram(rows)
			private := doc.Blocks[2]
			private.ID = "native_projection"
			private.SystemGeneratedKind = types.AnswerSystemGeneratedRuntimeTrace
			doc.Blocks[2].Diagram = &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: fmt.Sprintf("sequenceDiagram\n participant %s as %s\n participant %s as %s", rows[0].FromNode, rows[0].FromLabel, rows[0].ToNode, rows[0].ToLabel)}
			doc.Blocks[2].EdgeAnchors = nil
			switch mode {
			case "label_only":
				doc = hmc229NativeDiagram(rows)
				ctx.AnalysisIR.RequestModel.DiagramHint.Participants = []types.DiagramParticipantHint{{Identity: rows[0].FromLabel, Role: types.DiagramParticipantIncidentRequired, SourceQuote: rows[0].FromLabel}}
				ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
				ctx.InvalidateAnswerPlanCache()
			case "decorated_entity":
				doc = hmc229NativeDiagram(rows)
				identity := "业务角色 " + rows[0].FromNode
				ctx.AnalysisIR.RequestModel.DiagramHint.Participants = []types.DiagramParticipantHint{{Identity: identity, Role: types.DiagramParticipantIncidentRequired, SourceQuote: identity}}
				ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{rows[0].FromNode}
				ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
				ctx.InvalidateAnswerPlanCache()
			case "foreign_tuple":
				doc = hmc229NativeDiagram(rows)
				for i := range doc.Blocks[2].EdgeAnchors {
					doc.Blocks[2].EdgeAnchors[i].FromIdentity += ":foreign"
				}
			case "private_sibling":
				doc.Blocks = append(doc.Blocks, private)
			case "private_only":
				doc.Blocks[2] = private
			}
			before, _ := json.Marshal(doc)
			view := types.BuildAnswerSemanticViewForBusContext(ctx)
			got := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, doc, view, nil)
			if mode == "private_only" {
				if len(got) != 0 {
					t.Fatalf("private projection is not a model participant claim: %+v", got)
				}
			} else if len(got) == 0 {
				t.Fatal("label/foreign/private evidence discharged a missing model participant")
			}
			if _, ok := AcceptedDiagramParticipantCoverageReceiptWithRuntimeContext(ctx, doc, view, nil); ok {
				t.Fatal("incomplete or private-only graph minted model coverage receipt")
			}
			if mode == "decorated_entity" && len(diagramParticipantMissingRuntimeRelations(rows, []DiagramParticipantCoverageMismatch{{Participant: ctx.AnalysisIR.RequestModel.DiagramHint.Participants[0].Identity, Issue: DiagramParticipantCoverageTypedEdgeMissing}}, ctx.AnalysisIR.RequestModel)) != 0 {
				t.Fatal("soft entity expansion minted a native repair candidate")
			}
			after, _ := json.Marshal(doc)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("coverage mutated the source document")
			}
			if mode == "missing" {
				encoded := diagramParticipantRepairAdditionDeltaJSON(doc, ctx, got, nil, nil)
				var delta types.AnswerDiagramRelationRepairDelta
				if json.Unmarshal([]byte(encoded), &delta) != nil || len(delta.AllowedAdditions) == 0 {
					t.Fatalf("native omission must publish executable current tuples: %s %+v", encoded, got)
				}
				for _, candidate := range delta.AllowedAdditions {
					if !runtimeDiagramRelationProved(rows, candidate.FromIdentity, candidate.ToIdentity, candidate.RelationKind) {
						t.Fatalf("unproved repair candidate: %+v", candidate)
					}
				}
			}
		})
	}
}

func TestHMC229ParticipantPublicEmitRejectsSilentRuntimeOmission(t *testing.T) {
	ctx, doc := hmc229ParticipantContext(t)
	raw, _ := json.Marshal(doc)
	result, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || result.Success || result.Repair == nil || result.Repair.Metadata[types.ToolRepairMetaDiagramParticipantRepairDeltaJSON] == "" {
		t.Fatalf("a valid local native edge cannot silently discharge the three declared participants: %v %+v", err, result)
	}
	if !strings.Contains(result.Repair.Metadata[types.ToolRepairMetaDiagramParticipantRepairDeltaJSON], "Storage") {
		t.Fatalf("repair lost the unfulfilled explicit participant: %+v", result.Repair)
	}
	if ctx.Mutable.AnswerDocumentV2() != nil || ctx.Mutable.LastRejectedAnswerDocumentV2() == nil {
		t.Fatal("incomplete participant coverage was accepted or the failed draft was lost")
	}
}

func TestHMC229ParticipantPublicEmitDoesNotInventOptionalObligations(t *testing.T) {
	for _, mode := range []string{"optional", "context_only", "unanchored"} {
		t.Run(mode, func(t *testing.T) {
			ctx, doc := hmc229ParticipantContext(t)
			switch mode {
			case "optional":
				ctx.AnalysisIR.RequestModel.DiagramHint.Required = false
				ctx.AnalysisIR.AnswerContract.Diagram.Required = false
			case "context_only":
				for i := range ctx.AnalysisIR.RequestModel.DiagramHint.Participants {
					ctx.AnalysisIR.RequestModel.DiagramHint.Participants[i].Role = types.DiagramParticipantContextOnly
				}
			case "unanchored":
				for i := range ctx.AnalysisIR.RequestModel.DiagramHint.Participants {
					ctx.AnalysisIR.RequestModel.DiagramHint.Participants[i].SourceQuote = ""
				}
			}
			ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			ctx.InvalidateAnswerPlanCache()
			raw, _ := json.Marshal(doc)
			result, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || !result.Success {
				t.Fatalf("inactive slate gained a hard obligation: %v %s", err, result.Summary)
			}
			if _, ok := AcceptedDiagramParticipantCoverageReceiptWithRuntimeContext(ctx, ctx.Mutable.AnswerDocumentV2(), types.BuildAnswerSemanticViewForBusContext(ctx), nil); ok {
				t.Fatal("inactive slate minted a coverage receipt")
			}
		})
	}
}
