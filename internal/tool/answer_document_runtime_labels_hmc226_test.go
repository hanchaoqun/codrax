package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC226RuntimeEndpointLabelsPublicEmit(t *testing.T) {
	for _, carrier := range []string{"implicit_sequence", "default_sequence", "implicit_flow", "default_flow", "inline_flow"} {
		t.Run(carrier, func(t *testing.T) {
			ctx, _, rows := runtimeWakeFixture(t)
			r := rows[0]
			doc := runtimeNestingDoc(r, true)
			d := doc.Blocks[1].Diagram
			d.Kind = types.DiagramSequence
			d.Body = fmt.Sprintf("sequenceDiagram\n %s->>%s: 唤醒\n", r.FromNode, r.ToNode)
			if carrier == "default_sequence" {
				d.Body = fmt.Sprintf("sequenceDiagram\n participant %s as %q\n participant %s as %q\n %s->>%s: 唤醒\n", r.FromNode, r.FromNode, r.ToNode, r.ToNode, r.FromNode, r.ToNode)
			}
			if strings.HasSuffix(carrier, "flow") {
				d.Kind = types.DiagramFlow
				d.Body = fmt.Sprintf("flowchart TD\n %s -->|唤醒| %s\n", r.FromNode, r.ToNode)
				if carrier == "default_flow" {
					d.Body = fmt.Sprintf("flowchart TD\n %s[%q]\n %s[%q]\n %s -->|唤醒| %s\n", r.FromNode, r.FromNode, r.ToNode, r.ToNode, r.FromNode, r.ToNode)
				}
				if carrier == "inline_flow" {
					d.Body = fmt.Sprintf("flowchart TD\n %s[%q]:::source -->|唤醒| %s(%q)\n", r.FromNode, r.FromNode, r.ToNode, r.ToNode)
				}
			}
			doc.Blocks[1].EdgeAnchors[0].VisibleLabel = "唤醒"
			before := append([]types.DiagramEdgeAnchor(nil), doc.Blocks[1].EdgeAnchors...)
			raw, _ := json.Marshal(doc)
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || !res.Success {
				t.Fatalf("real native wakeup rejected: %v %+v", err, res)
			}
			after := ctx.Mutable.AnswerDocumentV2()
			hmc226AssertRuntimeEndpointLabels(t, after.Blocks[1], r)
			if !reflect.DeepEqual(before, after.Blocks[1].EdgeAnchors) {
				t.Fatal("display reuse changed relation authority")
			}
		})
	}
}

func hmc226AssertRuntimeEndpointLabels(t *testing.T, block types.AnswerBlock, row RuntimeDiagramRelation) {
	t.Helper()
	labels := diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
	if labels[row.FromNode] != row.FromLabel || labels[row.ToNode] != row.ToLabel {
		t.Fatalf("native endpoint names lost: got %+v, want %q / %q; body=%s", labels, row.FromLabel, row.ToLabel, block.Diagram.Body)
	}
	edges := mermaidcompat.ParseEdges(block.Diagram.Body)
	if len(edges) != 1 || edges[0].From != row.FromNode || edges[0].To != row.ToNode || edges[0].Label != "唤醒" {
		t.Fatalf("labels must not invent, retarget, or relabel an arrow: %+v", edges)
	}
	visible := render.RenderMermaidBlocks("```mermaid\n" + block.Diagram.Body + "\n```")
	if !strings.Contains(visible, row.FromLabel) || !strings.Contains(visible, row.ToLabel) || strings.Contains(visible, row.FromNode) || strings.Contains(visible, row.ToNode) || strings.Contains(visible, "# ⚠") {
		t.Fatalf("actual diagram renderer lost native names or exposed node IDs:\n%s", visible)
	}
}

func TestHMC226RuntimeEndpointLabelsPublicPatch(t *testing.T) {
	for _, action := range []string{"replace", "add"} {
		t.Run(action, func(t *testing.T) {
			ctx, _, rows := runtimeWakeFixture(t)
			r := rows[0]
			doc := runtimeNestingDoc(r, false)
			doc.Blocks[1].Diagram.Kind = types.DiagramSequence
			doc.Blocks[1].Diagram.Body = "sequenceDiagram\n A->>B: 未证关系\n"
			if action == "replace" {
				doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: r.FromNode, ToNode: r.ToNode, FromIdentity: r.FromIdentity, ToIdentity: r.ToIdentity, RelationKind: r.Kind, VisibleLabel: "唤醒"}}
			}
			raw, _ := json.Marshal(doc)
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || res.Success || res.Repair == nil {
				t.Fatalf("real unproved draft must reject: %v %+v", err, res)
			}
			var delta types.AnswerDiagramRelationRepairDelta
			if err := json.Unmarshal([]byte(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
				t.Fatal(err)
			}
			before := ctx.Mutable.LastRejectedAnswerDocumentV2()
			lease := types.NewAnswerDiagramRelationRepairLease(before, delta.Failures, delta.AllowedAdditions)
			ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
			var failure types.AnswerDiagramRelationRepairFailure
			var stale types.AnswerDiagramRelationRepairFailure
			var candidate types.AnswerDiagramRelationRepairCandidate
			for _, f := range lease.Failures {
				if f.FromNode == "A" && f.ToNode == "B" {
					failure = f
				}
				if f.FromIdentity == r.FromIdentity && f.ToIdentity == r.ToIdentity {
					stale = f
				}
			}
			for _, c := range lease.AllowedAdditions {
				if c.FromIdentity == r.FromIdentity && c.ToIdentity == r.ToIdentity {
					candidate = c
				}
			}
			if failure.FailureRef == "" || (action == "add" && candidate.AdditionRef == "") {
				t.Fatalf("actual rejected draft lacks repair capabilities: %+v", lease)
			}
			edit := emitAnswerDiagramEdgeEdit{Action: action, AdditionRef: candidate.AdditionRef, Edge: &types.DiagramEdgeAnchor{FromNode: r.FromNode, ToNode: r.ToNode, VisibleLabel: "唤醒"}}
			var edits []emitAnswerDiagramEdgeEdit
			edits = append(edits, emitAnswerDiagramEdgeEdit{Action: "remove", FailureRef: failure.FailureRef})
			edit.PlacementRef = sequenceEndPlacementForTest(before, "diag")
			if action == "replace" {
				if stale.FailureRef == "" {
					t.Fatalf("actual draft lacks a stale native anchor: %+v", lease)
				}
				edit.FailureRef = stale.FailureRef
				edit.AdditionRef = ""
				edit.Edge.FromIdentity, edit.Edge.ToIdentity, edit.Edge.RelationKind = r.FromIdentity, r.ToIdentity, r.Kind
			}
			edits = append(edits, edit)
			raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_edge_edits": edits})
			res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
			if err != nil || !res.Success {
				t.Fatalf("real local %s repair rejected: %v %+v", action, err, res)
			}
			after := ctx.Mutable.AnswerDocumentV2()
			hmc226AssertRuntimeEndpointLabels(t, after.Blocks[1], r)
			if !reflect.DeepEqual(before.Blocks[0], after.Blocks[0]) || !reflect.DeepEqual(before.Blocks[2], after.Blocks[2]) {
				t.Fatal("local native labels changed unrelated content")
			}
			if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, after, types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(got) != 0 {
				t.Fatalf("display labels changed the proof: %+v", got)
			}
		})
	}
}
