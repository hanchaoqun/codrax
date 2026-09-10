package tool

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// B1643 keeps the exact published failure-ref selector and replacement budget aligned.
// Both relations are actual C++ fixture calls: logger.cpp:36 write, :38 flush.
// Model selection replaces only write's exact second/first body occurrence.
func TestB1643SameVisiblePairBudgetMustNotDependOnFirstBaseAnchor(t *testing.T) {
	for _, writeFirst := range []bool{false, true} {
		name := "flush-first"
		if writeFirst {
			name = "write-first"
		}
		t.Run(name, func(t *testing.T) {
			base, evidence := b1641LoggerFixture(t)
			flush := base.Blocks[1].EdgeAnchors[0]
			write := types.DiagramEdgeAnchor{FromNode: "Logger", ToNode: "Sink", FromIdentity: "Logger.log", ToIdentity: "Sink.write",
				RelationKind: types.DiagramRelCall, VisibleLabel: "old write"}
			base.Blocks[1].Diagram.Body = "sequenceDiagram\n    participant Logger\n    participant Sink\n"
			anchors := []types.DiagramEdgeAnchor{flush, write}
			occurrence, writeIndex := 2, 1
			if writeFirst {
				anchors = []types.DiagramEdgeAnchor{write, flush}
				occurrence, writeIndex = 1, 0
			}
			base.Blocks[1].EdgeAnchors = anchors
			for _, anchor := range anchors {
				base.Blocks[1].Diagram.Body += "    Logger->>Sink: " + anchor.VisibleLabel + "\n"
			}
			before, _ := json.Marshal(base)
			lease := types.NewAnswerDiagramRelationRepairLease(base, []types.AnswerDiagramRelationRepairFailure{{
				BlockID: "diagram", Issue: diagramParticipantComponentJoinEndpointMappingIssue,
				FromNode: "Logger", ToNode: "Sink", FromIdentity: "Logger.log", ToIdentity: "Sink.write",
				RelationKind: types.DiagramRelCall, BodyOccurrence: occurrence,
			}}, nil)
			if lease == nil || len(lease.Failures) != 1 || !lease.Failures[0].AllowsAction("replace") {
				t.Fatalf("real lease failed to publish exact write replacement: %+v", lease)
			}
			edit := map[string]any{"failure_ref": lease.Failures[0].FailureRef, "action": "replace",
				"from_node_visible_label": "Logger::log", "to_node_visible_label": "Sink::write",
				"edge": map[string]any{"from_node": "Logger.log", "to_node": "Sink.write", "visible_label": "model write replacement"}}
			mut := types.NewMutableState("B1643 exact selected relation budget")
			mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
			mut.SetAnswerDiagramRelationRepairLease(lease)
			bus := &types.BusContext{Mutable: mut, EvidenceItems: evidence}
			b1641CheckSchema(t, bus, edit)
			t.Run("duplicate-ref-does-not-increase-permission", func(t *testing.T) {
				duplicateMut := types.NewMutableState("B1643 duplicate ref")
				duplicateMut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
				duplicateMut.SetAnswerDiagramRelationRepairLease(lease)
				duplicateBus := &types.BusContext{Mutable: duplicateMut, EvidenceItems: evidence}
				result, err := b1641Execute(t, duplicateBus, edit, edit)
				raw, _ := json.Marshal(result)
				if err != nil || result.Success || !strings.Contains(string(raw), "reuses failure_ref") {
					t.Fatalf("duplicate ref must fail at its existing exact-consumption gate: err=%v result=%s", err, raw)
				}
				if got, _ := json.Marshal(duplicateMut.AnswerDocumentV2()); !bytes.Equal(before, got) {
					t.Fatal("duplicate ref rejection changed the model document")
				}
			})
			var typedEdit emitAnswerDiagramEdgeEdit
			rawEdit, _ := json.Marshal(edit)
			if err := json.Unmarshal(rawEdit, &typedEdit); err != nil {
				t.Fatal(err)
			}
			patch := &types.AnswerDocumentV2Patch{}
			if err := applyModelAuthoredDiagramAtomicEdits(base, patch, []emitAnswerDiagramEdgeEdit{typedEdit}, nil, lease); err != nil {
				t.Fatalf("exact selected edit did not compile: %v", err)
			}
			if len(patch.ReplaceBlocks) != 1 {
				t.Fatalf("lost compiled graph: %+v", patch)
			}
			compiled := patch.ReplaceBlocks[0]
			if len(compiled.EdgeAnchors) != 2 || !reflect.DeepEqual(compiled.EdgeAnchors[1-writeIndex], flush) ||
				compiled.EdgeAnchors[writeIndex].FromNode != "Logger.log" || compiled.EdgeAnchors[writeIndex].ToNode != "Sink.write" ||
				compiled.EdgeAnchors[writeIndex].ToIdentity != "Sink.write" || strings.Contains(compiled.Diagram.Body, "old write") {
				t.Fatalf("this is not a budget-only witness; exact edit changed other relations: %+v", compiled)
			}
			next := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{base.Blocks[0], compiled}}
			violations := types.ValidateAnswerDiagramRelationRepairLease(lease, next)
			t.Logf("base_anchors=%+v selected=%+v compiled_anchors=%+v scope_violations=%+v", anchors, lease.Failures[0], compiled.EdgeAnchors, violations)
			if len(violations) != 0 {
				t.Errorf("two source-proven relations retain two rows; replacing only the selected write must not expand a relation: %+v", violations)
			}
			result, err := b1641Execute(t, bus, edit)
			if err != nil || !result.Success {
				t.Errorf("schema-valid exact replacement failed public executor: err=%v result=%+v", err, result)
			}
			accepted := mut.AnswerDocumentV2()
			if accepted == nil || len(accepted.Blocks) != 2 ||
				!reflect.DeepEqual(accepted.Blocks[0], base.Blocks[0]) ||
				!reflect.DeepEqual(accepted.Blocks[1].EdgeAnchors, compiled.EdgeAnchors) ||
				accepted.Blocks[1].Diagram.Body != compiled.Diagram.Body {
				t.Fatalf("public publication lost the selected edit or rewrote other model content: %+v", accepted)
			}
			after, _ := json.Marshal(base)
			if !bytes.Equal(before, after) {
				t.Fatal("immutable model base changed")
			}
		})
	}
}
