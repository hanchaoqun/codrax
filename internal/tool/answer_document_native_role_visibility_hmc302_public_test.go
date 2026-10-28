package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The public emitter, not a label matcher, owns the failure and retry base.
// These roles deliberately have no request-to-thread identity proof.
func nativeRoleVisibilityFixtureHMC302(t *testing.T) (*types.BusContext, []RuntimeDiagramRelation, *types.AnswerDocumentV2) {
	t.Helper()
	quote := "主线程、加载线程和工作线程"
	ctx, rows := requestedThreadFixtureCustomHMC242(t, "看看附件中 1.000 到 1.020 秒 "+hmc242ParticipantQuote+"被谁唤醒了；另说明"+quote+"的唤醒关系，再画一张唤醒时序图。", func(p map[string]any) {
		p["diagram_hint"] = map[string]any{"required": true, "kind": "sequence", "relation_scope_quote": quote + "的唤醒关系", "participants": []any{
			map[string]any{"identity": "主线程", "source_quote": "主线程", "role": "incident_required"},
			map[string]any{"identity": "加载线程", "source_quote": "加载线程", "role": "incident_required"},
			map[string]any{"identity": "工作线程", "source_quote": "工作线程", "role": "incident_required"},
		}}
	})
	doc := requestedThreadDocHMC242(rows, true)
	b := &doc.Blocks[1]
	aliases := map[string]string{}
	for _, r := range rows {
		for _, node := range []string{r.FromNode, r.ToNode} {
			if aliases[node] == "" {
				aliases[node] = fmt.Sprintf("actor%d", len(aliases)+1)
			}
		}
	}
	for native, alias := range aliases {
		b.Diagram.Body = strings.ReplaceAll(b.Diagram.Body, native, alias)
		b.Diagram.RuntimeParticipantsV1 = append(b.Diagram.RuntimeParticipantsV1, types.RuntimeDiagramParticipantSelection{NodeID: alias, NativeParticipant: native})
		for i := range b.EdgeAnchors {
			if b.EdgeAnchors[i].FromNode == native {
				b.EdgeAnchors[i].FromNode = alias
			}
			if b.EdgeAnchors[i].ToNode == native {
				b.EdgeAnchors[i].ToNode = alias
			}
		}
	}
	sort.Slice(b.Diagram.RuntimeParticipantsV1, func(i, j int) bool {
		return b.Diagram.RuntimeParticipantsV1[i].NodeID < b.Diagram.RuntimeParticipantsV1[j].NodeID
	})
	b.Diagram.Body += "Note over actor1: original state note\n"
	return ctx, rows, doc
}

func installRoleVisibilityPublicRetryHMC302(t *testing.T, ctx *types.BusContext, result types.ToolResult) *types.AnswerDiagramRelationRepairLease {
	t.Helper()
	if result.Success || result.Repair == nil {
		t.Fatalf("expected public rejection: %+v", result)
	}
	var delta struct {
		Mismatches []struct {
			BlockID     string `json:"block_id"`
			Participant string `json:"participant"`
			Issue       string `json:"issue"`
		} `json:"mismatches"`
	}
	if err := json.Unmarshal([]byte(result.Repair.Metadata[types.ToolRepairMetaDiagramParticipantRepairDeltaJSON]), &delta); err != nil {
		t.Fatal(err)
	}
	base := ctx.Mutable.PendingAnswerDocumentPatchBase()
	if base == nil {
		base = ctx.Mutable.LastRejectedAnswerDocumentV2()
	}
	if base == nil {
		t.Fatal("public rejection lost retry base")
	}
	var boundaries []types.AnswerDiagramParticipantBoundaryRepairFailure
	var visibility []types.AnswerDiagramParticipantVisibilityRepairFailure
	for _, m := range delta.Mismatches {
		boundaries = append(boundaries, types.AnswerDiagramParticipantBoundaryRepairFailure{BlockID: m.BlockID, Participant: m.Participant, Issue: m.Issue})
		visibility = append(visibility, types.AnswerDiagramParticipantVisibilityRepairFailure{BlockID: m.BlockID, Participant: m.Participant, Issue: m.Issue})
	}
	lease := types.WithAnswerDiagramParticipantBoundaryRepairFailures(base, nil, boundaries)
	lease = types.WithAnswerDiagramParticipantVisibilityRepairFailures(base, lease, visibility)
	if lease == nil {
		t.Fatalf("no executable repair: %s", result.Summary)
	}
	ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	return lease
}

// Characterize the current safe boundary, not a completed role-mapping feature.
// The separate frozen RED records the still-open 3-to-6-subject display debt.
func TestHMC302NativeRoleVisibilityPublicRepairPreservesUnprovenBoundary(t *testing.T) {
	ctx, rows, doc := nativeRoleVisibilityFixtureHMC302(t)
	for _, row := range rows {
		if len(row.requestedFrom)+len(row.requestedTo) != 0 {
			t.Fatal("a technical target must not confer unrelated business-role identity")
		}
	}
	raw, _ := json.Marshal(doc)
	initial, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("initial emit: %s", initial.Summary)
	if initial.Success || !strings.Contains(initial.Summary, "missing_unproven_boundary") {
		t.Fatalf("unproved role gained relation authority: %+v", initial)
	}
	var boundaries []types.DiagramParticipantBoundary
	for _, p := range ctx.AnalysisIR.RequestModel.DiagramHint.Participants {
		boundaries = append(boundaries, types.DiagramParticipantBoundary{Participant: p.Identity, Status: types.DiagramParticipantBoundaryUnproven})
	}
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_boundary_replacements": []any{map[string]any{"block_id": doc.Blocks[1].ID, "participant_boundaries": boundaries}}})
	boundary, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("boundary patch: %s", boundary.Summary)
	lease := installRoleVisibilityPublicRetryHMC302(t, ctx, boundary)
	if len(lease.ParticipantVisibilityFailures) != 3 {
		t.Fatalf("want three exact role failures: %+v", lease)
	}
	var visible []map[string]string
	for i, f := range lease.ParticipantVisibilityFailures {
		visible = append(visible, map[string]string{"participant_ref": f.ParticipantRef, "action": "ensure_visible", "node_id": fmt.Sprintf("actor%d", i+1), "visible_label": f.Participant + " observed-thread"})
	}
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_participant_edits": visible})
	beforeRelabel := ctx.Mutable.PendingAnswerDocumentPatchBase()
	relabel, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("same-node relabel result: success=%t %s", relabel.Success, relabel.Summary)
	if relabel.Success || !strings.Contains(relabel.Summary, "does not visibly carry the exact required participant identity") || !strings.Contains(relabel.Summary, "this call was not staged") || !reflect.DeepEqual(beforeRelabel, ctx.Mutable.PendingAnswerDocumentPatchBase()) {
		t.Fatalf("unproved role relabel changed current authority or retry base: %s", relabel.Summary)
	}
	for i, f := range lease.ParticipantVisibilityFailures {
		visible[i]["node_id"] = f.Participant
	}
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_participant_edits": visible})
	fallback, callErr := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if callErr != nil || !fallback.Success {
		t.Fatalf("published fallback failed: %v %s", callErr, fallback.Summary)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("no accepted document")
	}
	b := got.Blocks[1]
	if !reflect.DeepEqual(b.EdgeAnchors, doc.Blocks[1].EdgeAnchors) || !strings.Contains(b.Diagram.Body, "Note over actor1: original state note") {
		t.Fatalf("repair changed native anchors/notes: %+v", b)
	}
	want := len(mermaidcompat.RemovableNodeDeclarations(doc.Blocks[1].Diagram.Body)) + len(boundaries)
	if count := len(mermaidcompat.RemovableNodeDeclarations(b.Diagram.Body)); count != want {
		t.Fatalf("current unresolved-role fallback changed without an identity contract: got %d declarations, want %d; body=%s", count, want, b.Diagram.Body)
	}
	if !reflect.DeepEqual(b.ParticipantBoundaries, boundaries) || !reflect.DeepEqual(b.Diagram.RuntimeParticipantsV1, doc.Blocks[1].Diagram.RuntimeParticipantsV1) {
		t.Fatalf("fallback altered native selectors or unresolved roles: %+v", b)
	}
	for _, edge := range mermaidcompat.ParseEdges(b.Diagram.Body) {
		for _, boundary := range boundaries {
			if edge.From == boundary.Participant || edge.To == boundary.Participant {
				t.Fatal("unproved role acquired a native incident edge")
			}
		}
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if mismatches := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, got, view, nil); len(mismatches) != 0 {
		t.Fatalf("original native relation no longer valid: %+v", mismatches)
	}
	if mismatches := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, got, view, nil); len(mismatches) != 0 {
		t.Fatalf("final boundary differs from public acceptance: %+v", mismatches)
	}
	rendered := render.RenderAnswerDocument(got, "zh")
	if !strings.Contains(rendered, "未证关系边界") || !strings.Contains(rendered, "original state note") {
		t.Fatalf("final rendering dropped boundary or note: %s", rendered)
	}
	for _, boundary := range boundaries {
		if !strings.Contains(rendered, "`"+boundary.Participant+"`") {
			t.Fatalf("final surface omitted exact unresolved role %q", boundary.Participant)
		}
	}
}
