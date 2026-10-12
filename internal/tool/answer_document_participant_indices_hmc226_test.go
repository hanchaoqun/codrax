package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC226ParticipantOriginalIndexPublicPatch(t *testing.T) {
	for _, name := range []string{"missing_orphan_fields", "missing_orphan_block", "duplicate_orphan", "invalid_orphan_label", "stale_visibility", "duplicate_visibility"} {
		t.Run(name, func(t *testing.T) {
			ctx, params, visible, orphan := hmc226ParticipantIndexFixture(t)
			edits := []emitAnswerDiagramParticipantEdit{visible, orphan}
			wantIndex, wantReason := "diagram_participant_edits[1]", ""
			switch name {
			case "missing_orphan_fields":
				edits[1].BlockID, edits[1].ParticipantID = "", ""
				wantReason = "requires block_id and participant_id"
			case "missing_orphan_block":
				edits[1].BlockID = "absent"
				wantReason = "absent"
			case "duplicate_orphan":
				edits = append(edits, orphan)
				wantIndex, wantReason = "diagram_participant_edits[2]", "duplicates"
			case "invalid_orphan_label":
				edits[1].VisibleLabel = ""
				wantReason = "non-empty model-authored visible_label"
			case "stale_visibility":
				visible.ParticipantRef = "stale"
				edits = []emitAnswerDiagramParticipantEdit{orphan, visible}
				wantReason = "stale, unknown"
			case "duplicate_visibility":
				edits = []emitAnswerDiagramParticipantEdit{orphan, visible, visible}
				wantIndex, wantReason = "diagram_participant_edits[2]", "reuses participant_ref"
			}
			params["diagram_participant_edits"] = edits
			before, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
			raw, _ := json.Marshal(params)
			res, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
			if err != nil || res.Success || res.Repair == nil {
				t.Fatalf("invalid mixed patch must reject atomically: err=%v result=%+v", err, res)
			}
			if !strings.Contains(res.Summary, wantIndex) || !strings.Contains(res.Summary, wantReason) {
				t.Fatalf("repair must identify the original request element %s (%s), not its classified sub-slice: %s", wantIndex, wantReason, res.Summary)
			}
			after, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
			if string(before) != string(after) || ctx.Mutable.PendingAnswerDocumentPatchBase() != nil {
				t.Fatal("invalid mixed edits mutated or staged the accepted base")
			}
		})
	}
}

func TestHMC226ParticipantOriginalIndexPublicPatchValidMixed(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		ctx, params, visible, orphan := hmc226ParticipantIndexFixture(t)
		edits := []emitAnswerDiagramParticipantEdit{visible, orphan}
		if reverse {
			edits[0], edits[1] = edits[1], edits[0]
		}
		params["diagram_participant_edits"] = edits
		before := ctx.Mutable.AnswerDocumentV2()
		raw, _ := json.Marshal(params)
		res, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
		if err != nil || !res.Success {
			t.Fatalf("valid mixed branches must accept without inventing shared required fields: reverse=%t err=%v result=%+v", reverse, err, res)
		}
		after := ctx.Mutable.AnswerDocumentV2()
		block := after.Blocks[1]
		if !reflect.DeepEqual(before.Blocks[0], after.Blocks[0]) ||
			!strings.Contains(block.Diagram.Body, `participant subject as "BusContext"`) ||
			!strings.Contains(block.Diagram.Body, `participant A as "分析入口（背景）"`) ||
			len(mermaidcompat.ParseEdges(block.Diagram.Body)) != 1 || len(block.EdgeAnchors) != 1 ||
			block.EdgeAnchors[0] != before.Blocks[1].EdgeAnchors[1] ||
			!reflect.DeepEqual(block.ParticipantBoundaries, before.Blocks[1].ParticipantBoundaries) {
			t.Fatalf("mixed participant edits changed unselected data, relations, or authority: %+v", after)
		}
	}
}

func hmc226ParticipantIndexFixture(t *testing.T) (*types.BusContext, map[string]any, emitAnswerDiagramParticipantEdit, emitAnswerDiagramParticipantEdit) {
	t.Helper()
	base := atomicPatchTestDocument()
	base.Blocks[1].ParticipantBoundaries = []types.DiagramParticipantBoundary{{Participant: "BusContext", Status: types.DiagramParticipantBoundaryUnproven}}
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "diag", Issue: "semantic_relation_edge_unproven", FromNode: "A", ToNode: "B",
		FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence, BodyOccurrence: 1,
	}}, nil)
	lease = types.WithAnswerDiagramParticipantVisibilityRepairFailures(base, lease, []types.AnswerDiagramParticipantVisibilityRepairFailure{{
		BlockID: "diag", Participant: "BusContext", Issue: "boundary_participant_not_visible",
	}})
	if lease == nil || len(lease.Failures) != 1 || len(lease.ParticipantVisibilityFailures) != 1 {
		t.Fatalf("missing exact mixed repair fixture: %+v", lease)
	}
	lease.OptionalOrphanCleanups = testDiagramOrphanCandidates("diag", "A")
	mut := types.NewMutableState("mixed participant error coordinates")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
	mut.SetAnswerDiagramRelationRepairLease(lease)
	params := map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": []map[string]string{{"failure_ref": lease.Failures[0].FailureRef, "action": "remove"}}}
	visible := emitAnswerDiagramParticipantEdit{ParticipantRef: lease.ParticipantVisibilityFailures[0].ParticipantRef, Action: "ensure_visible", NodeID: "subject", VisibleLabel: "BusContext"}
	orphan := emitAnswerDiagramParticipantEdit{BlockID: "diag", ParticipantID: "A", Action: "retain_as_context", VisibleLabel: "分析入口（背景）"}
	return &types.BusContext{Mutable: mut}, params, visible, orphan
}
