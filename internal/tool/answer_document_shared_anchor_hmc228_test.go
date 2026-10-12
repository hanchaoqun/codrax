package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC228SharedAnchorPublicRemovalPreservesVerifiedSibling(t *testing.T) {
	for _, privateSibling := range []bool{false, true} {
		t.Run(fmt.Sprintf("private_sibling=%t", privateSibling), func(t *testing.T) {
			ctx, rows := hmc228CausalDiagramContext(t, &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis})
			doc := hmc228WakeDiagram(rows)
			want := doc.Blocks[2]
			want.Diagram = &types.AnswerDiagramBlock{Kind: want.Diagram.Kind, Language: want.Diagram.Language, Body: want.Diagram.Body}
			doc.Blocks[2].Diagram.Body += fmt.Sprintf(" %s-->>%s: invalid response operator\n", rows[0].FromNode, rows[0].ToNode)
			raw, _ := json.Marshal(doc)
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || res.Success || res.Repair == nil {
				t.Fatalf("actual emit must reject only the incompatible reply: err=%v result=%+v", err, res)
			}
			var delta types.AnswerDiagramRelationRepairDelta
			if err := json.Unmarshal([]byte(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
				t.Fatal(err)
			}
			if len(delta.Failures) != 1 || delta.Failures[0].Issue != diagramSequenceRelationReplyConflict {
				t.Fatalf("valid sibling must not be included in the failure set: %+v", delta.Failures)
			}
			base := ctx.Mutable.LastRejectedAnswerDocumentV2()
			unchanged := []string{"summary", "events"}
			if privateSibling {
				// A system supplement can coexist in the retained draft; it is not
				// model JSON and must remain outside the model's repair capability.
				base.Blocks = append(base.Blocks, types.AnswerBlock{ID: "native_sibling", Kind: types.BlockDiagram,
					SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace,
					Diagram:             &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n NativeA --> NativeB"}})
				unchanged = append(unchanged, "native_sibling")
			}
			lease := types.NewAnswerDiagramRelationRepairLease(base, delta.Failures, delta.AllowedAdditions)
			ctx.Mutable.SetPendingAnswerDocumentPatchBase(base)
			ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
			before, _ := json.Marshal(base)
			bad, _ := json.Marshal(map[string]any{"unchanged_block_ids": unchanged, "diagram_edge_edits": []map[string]string{{"failure_ref": "stale-ref", "action": "remove"}}})
			res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, bad)
			if err != nil || res.Success {
				t.Fatalf("an unrelated/stale ref must not be allowed to consume the relation: %v %+v", err, res)
			}
			afterBad, _ := json.Marshal(ctx.Mutable.PendingAnswerDocumentPatchBase())
			if string(before) != string(afterBad) {
				t.Fatal("failed patch changed the immutable retry base")
			}
			patch, _ := json.Marshal(map[string]any{"unchanged_block_ids": unchanged, "diagram_edge_edits": []map[string]string{{"failure_ref": lease.Failures[0].FailureRef, "action": "remove"}}})
			res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, patch)
			if err != nil || !res.Success {
				t.Fatalf("removing one failed presentation must retain the verified sibling and its shared anchor: %v %s", err, res.Summary)
			}
			accepted := ctx.Mutable.AnswerDocumentV2()
			got := projectionClusterBlock(accepted.Blocks, "sequence")
			if got == nil || strings.TrimSuffix(got.Diagram.Body, "\n") != strings.TrimSuffix(want.Diagram.Body, "\n") || !reflect.DeepEqual(got.EdgeAnchors, want.EdgeAnchors) {
				t.Fatalf("remaining verified relations changed: got=%+v want=%+v", got, want)
			}
			if privateSibling && !reflect.DeepEqual(projectionClusterBlock(accepted.Blocks, "native_sibling"), projectionClusterBlock(base.Blocks, "native_sibling")) {
				t.Fatal("a model relation repair changed the private system sibling")
			}
			after, _ := json.Marshal(base)
			if string(before) != string(after) {
				t.Fatal("successful patch mutated its original failed draft")
			}
		})
	}
}

func TestHMC228SharedAnchorRemoveAllOccurrences(t *testing.T) {
	base := atomicPatchTestDocument()
	base.Blocks[1].Diagram.Body = "sequenceDiagram\n A-->>B: first invalid reply\n A-->>B: second invalid reply\n"
	anchor := types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "one.from", ToIdentity: "one.to", RelationKind: types.DiagramRelWakeup}
	base.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{anchor}
	// The compatibility compiler accepts exact immutable-body coordinates.
	// The live prior-anchor lease intentionally deduplicates this one tuple;
	// do not invent a second published failure ref to test descending edits.
	edits := []emitAnswerDiagramEdgeEdit{
		{BlockID: "diag", Action: "remove", Match: &anchor, BodyOccurrence: 1},
		{BlockID: "diag", Action: "remove", Match: &anchor, BodyOccurrence: 2},
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(base, patch, edits, nil, nil); err != nil {
		t.Fatal(err)
	}
	got := patch.ReplaceBlocks[0]
	if atomicDiagramBodyPairCount(got.Diagram.Body, "A", "B") != 0 || len(got.EdgeAnchors) != 0 {
		t.Fatalf("removing the last selected occurrence left a hidden anchor: %+v", got)
	}
}

func TestHMC228SharedAnchorPublicReplacePreservesUnselectedOwner(t *testing.T) {
	for _, moveOwner := range []bool{false, true} {
		t.Run(fmt.Sprintf("move_owner=%t", moveOwner), func(t *testing.T) {
			ctx, rows := hmc228CausalDiagramContext(t, &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis})
			doc := hmc228WakeDiagram(rows)
			doc.Blocks[2].Diagram.Body += fmt.Sprintf(" %s-->>%s: invalid response operator\n", rows[0].FromNode, rows[0].ToNode)
			raw, _ := json.Marshal(doc)
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || res.Success || res.Repair == nil {
				t.Fatalf("expected actual relation rejection: %v %+v", err, res)
			}
			var delta types.AnswerDiagramRelationRepairDelta
			if err := json.Unmarshal([]byte(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil || len(delta.Failures) != 1 {
				t.Fatalf("expected exactly one failed reply: %v %+v", err, delta.Failures)
			}
			base := ctx.Mutable.LastRejectedAnswerDocumentV2()
			lease := types.NewAnswerDiagramRelationRepairLease(base, delta.Failures, delta.AllowedAdditions)
			ctx.Mutable.SetPendingAnswerDocumentPatchBase(base)
			ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
			before, _ := json.Marshal(base)
			from, to := rows[0].FromNode, rows[0].ToNode
			edit := map[string]any{"failure_ref": lease.Failures[0].FailureRef, "action": "replace"}
			if moveOwner {
				from, to = "newFrom", "newTo"
				edit["from_node_visible_label"], edit["to_node_visible_label"] = "New sender", "New receiver"
			}
			edit["edge"] = map[string]string{"from_node": from, "to_node": to, "visible_label": "observed wakeup again"}
			patch, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "events"}, "diagram_edge_edits": []any{edit}})
			res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, patch)
			if moveOwner {
				if err != nil || res.Success || !strings.Contains(res.Summary, "unselected visible occurrence") {
					t.Fatalf("moving a shared owner must fail before staging, with a precise local error: %v %s", err, res.Summary)
				}
				after, _ := json.Marshal(ctx.Mutable.PendingAnswerDocumentPatchBase())
				if string(before) != string(after) {
					t.Fatal("a failed partial replacement staged a new draft or revoked its sibling owner")
				}
				return
			}
			if err != nil || !res.Success {
				t.Fatalf("same-tuple operator and label repair must remain accepted: %v %s", err, res.Summary)
			}
			got := projectionClusterBlock(ctx.Mutable.AnswerDocumentV2().Blocks, "sequence")
			if got == nil || atomicDiagramBodyPairCount(got.Diagram.Body, from, to) != 2 || len(got.EdgeAnchors) != 2 || !atomicDiagramAnchorSameTuple(got.EdgeAnchors[0], base.Blocks[2].EdgeAnchors[0]) {
				t.Fatalf("same-tuple repair lost a presentation or its owner: %+v", got)
			}
		})
	}
}

func TestHMC228SharedAnchorRequiresOneExplicitOwner(t *testing.T) {
	anchor := types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "one.from", ToIdentity: "one.to", RelationKind: types.DiagramRelWakeup}
	for _, mode := range []string{"no_from_identity", "no_to_identity", "invalid_relation", "reversed_body"} {
		t.Run(mode, func(t *testing.T) {
			candidate := anchor
			body := "sequenceDiagram\n A->>B: retained\n"
			switch mode {
			case "no_from_identity":
				candidate.FromIdentity = ""
			case "no_to_identity":
				candidate.ToIdentity = ""
			case "invalid_relation":
				candidate.RelationKind = "unrecognized"
			case "reversed_body":
				body = "sequenceDiagram\n B->>A: reversed\n"
			}
			if atomicDiagramAnchorOwnsRemainingBody([]types.DiagramEdgeAnchor{candidate}, 0, body) {
				t.Fatal("incomplete or different owner was retained from visible endpoint coincidence")
			}
		})
	}
}

func TestHMC228SharedAnchorWholeSelectedTransactionCanRetireOwner(t *testing.T) {
	base := atomicPatchTestDocument()
	base.Blocks[1].Diagram.Body = "sequenceDiagram\n A->>B: first\n A->>B: second\n"
	owner := types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "one.from", ToIdentity: "one.to", RelationKind: types.DiagramRelWakeup}
	base.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{owner}
	replacement := owner
	replacement.FromNode, replacement.ToNode, replacement.VisibleLabel = "C", "D", "replacement"
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	err := applyModelAuthoredDiagramAtomicEdits(base, patch, []emitAnswerDiagramEdgeEdit{
		{BlockID: "diag", Action: "replace", Match: &owner, BodyOccurrence: 1, Edge: &replacement},
		{BlockID: "diag", Action: "remove", Match: &owner, BodyOccurrence: 2},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := patch.ReplaceBlocks[0]
	if atomicDiagramBodyPairCount(got.Diagram.Body, "A", "B") != 0 || atomicDiagramBodyPairCount(got.Diagram.Body, "C", "D") != 1 || !reflect.DeepEqual(got.EdgeAnchors, []types.DiagramEdgeAnchor{replacement}) {
		t.Fatalf("a complete selected transaction must be able to retire its old owner: %+v", got)
	}
}

func TestHMC228SharedAnchorDoesNotPreserveAnotherTuple(t *testing.T) {
	for _, mode := range []string{"different_identity", "different_relation", "duplicate_tuple"} {
		t.Run(mode, func(t *testing.T) {
			base := atomicPatchTestDocument()
			base.Blocks[1].Diagram.Body = "sequenceDiagram\n A->>B: retained\n A->>B: selected\n"
			first := types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "one.from", ToIdentity: "one.to", RelationKind: types.DiagramRelCall, VisibleLabel: "retained"}
			second := first
			second.VisibleLabel = "selected"
			switch mode {
			case "different_identity":
				second.FromIdentity, second.ToIdentity = "other.from", "other.to"
			case "different_relation":
				second.RelationKind = types.DiagramRelWakeup
			}
			base.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{first, second}
			before, _ := json.Marshal(base)
			occurrence := 1
			if mode == "duplicate_tuple" {
				occurrence = 2
			}
			patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
			err := applyModelAuthoredDiagramAtomicEdits(base, patch, []emitAnswerDiagramEdgeEdit{{BlockID: "diag", Action: "remove", Match: &second, BodyOccurrence: 2, Occurrence: occurrence}}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			got := patch.ReplaceBlocks[0]
			if !reflect.DeepEqual(got.EdgeAnchors, []types.DiagramEdgeAnchor{first}) || strings.Contains(got.Diagram.Body, "selected") || !strings.Contains(got.Diagram.Body, "retained") {
				t.Fatalf("a same-pair sibling must not preserve the selected separate anchor: %+v", got)
			}
			after, _ := json.Marshal(base)
			if string(before) != string(after) {
				t.Fatal("atomic compiler mutated its base")
			}
		})
	}
}

func TestHMC228SharedBodyRemovalKeepsSurvivingTuple(t *testing.T) {
	base := atomicPatchTestDocument()
	base.Blocks[1].Diagram.Body = "sequenceDiagram\n A->>B: retained\n A->>B: selected\n"
	anchor := types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "State.Value", ToIdentity: "Bus.Value", RelationKind: types.DiagramRelAssignment}
	base.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{anchor}
	lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{
		{BlockID: "diag", Issue: "diagram_visible_label_mismatch", FromNode: "A", ToNode: "B", FromIdentity: "State.Value", ToIdentity: "Bus.Value", RelationKind: types.DiagramRelAssignment, BodyOccurrence: 2},
		{BlockID: "diag", Issue: "missing_call_anchor", FromNode: "A", ToNode: "B", FromIdentity: "Other.Call", ToIdentity: "Other.Run", RelationKind: types.DiagramRelCall, BodyOccurrence: 2},
	}, nil)
	var edits []emitAnswerDiagramEdgeEdit
	for _, failure := range lease.Failures {
		edits = append(edits, emitAnswerDiagramEdgeEdit{FailureRef: failure.FailureRef, Action: "remove"})
	}
	patch := &types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"summary"}}
	if err := applyModelAuthoredDiagramAtomicEdits(base, patch, edits, nil, lease); err != nil {
		t.Fatal(err)
	}
	got := patch.ReplaceBlocks[0]
	if !reflect.DeepEqual(got.EdgeAnchors, []types.DiagramEdgeAnchor{anchor}) || strings.Contains(got.Diagram.Body, "selected") || !strings.Contains(got.Diagram.Body, "retained") {
		t.Fatalf("joint body claims deleted the surviving exact tuple: %+v", got)
	}
}
