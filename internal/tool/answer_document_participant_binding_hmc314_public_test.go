package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC313 A's original emit b334bc9e8768464fcf2eabfe0be417920eebd2ef5971d27d03a04ce4f567110a
// and first patch 974d4173228ec1f9f30cde29054977dfaefa83e596a5c1d85b1aec4c68490f0a
// supplied one misspelled native selector, then a correct native node with
// ensure_visible. Replay the diagram against fresh public query authority;
// archived opaque native/instance/lease IDs cannot grant new-run authority.
const staleWorkerHMC314 = "rt_thread_d71bccacfaa1e09b70572ae02c34ee9b9220bd6898ac89f14178a702128ee6a5"

func staleNativeRetryHMC314(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2, []RuntimeDiagramRelation, []map[string]string) {
	t.Helper()
	ctx, rows, doc := nativeBoundaryFixtureHMC310(t)
	nodes := map[int]string{}
	for _, row := range rows {
		for _, side := range []struct {
			tid  int
			node string
		}{
			{row.threadEndpoints.from.PID, row.FromNode}, {row.threadEndpoints.to.PID, row.ToNode},
		} {
			if old := nodes[side.tid]; old != "" && old != side.node {
				t.Fatal("fixture requires coherent native lanes")
			}
			nodes[side.tid] = side.node
		}
	}
	if len(nodes) != 3 || nodes[200] == staleWorkerHMC314 {
		t.Fatal("bad replay premise")
	}
	b := &doc.Blocks[1]
	b.Diagram.Body = fmt.Sprintf(`sequenceDiagram
    participant %s as 主线程 app-main-100
    participant %s as 文档工作线程 document-worker-200
    participant %s as 存储完成 storage-irq-80
    Note over %s: OpenDocument 业务区间 1.000–1.050s（50.000ms）；其中 1.001–1.045s 睡眠 44.000ms，运行 7.000ms，可运行未上 CPU 1.000ms
    Note over %s: LoadDocumentIndex 区间 1.0045–1.0445s（40.000ms）；其中 1.009010–1.040010s 睡眠 31.000ms 等块设备读完成
    Note over %s: 该读请求 1.005s 发起、1.040s 完成，驻留 35.000ms；唤醒记录的目标 CPU 同为 CPU2
    %s->>%s: 唤醒 sched_wakeup 1.040010s
    %s->>%s: 唤醒 sched_wakeup 1.045000s
    Note over %s: 醒来后 1.045–1.046s 可运行未上 CPU，1.046s 起才切入运行`,
		nodes[100], staleWorkerHMC314, nodes[80], nodes[100], staleWorkerHMC314, nodes[80],
		nodes[80], staleWorkerHMC314, staleWorkerHMC314, nodes[100], nodes[100])
	b.Diagram.RuntimeParticipantsV1 = []types.RuntimeDiagramParticipantSelection{
		{NodeID: nodes[100], NativeParticipant: nodes[100]},
		{NodeID: staleWorkerHMC314, NativeParticipant: staleWorkerHMC314},
		{NodeID: nodes[80], NativeParticipant: nodes[80]},
	}
	b.EdgeAnchors = nil
	for _, row := range rows {
		from, to := row.FromNode, row.ToNode
		if from == nodes[200] {
			from = staleWorkerHMC314
		}
		if to == nodes[200] {
			to = staleWorkerHMC314
		}
		label := "唤醒 sched_wakeup 1.040010s"
		if row.threadEndpoints.from.PID == 200 {
			label = "唤醒 sched_wakeup 1.045000s"
		}
		b.EdgeAnchors = append(b.EdgeAnchors, types.DiagramEdgeAnchor{FromNode: from, ToNode: to, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind, VisibleLabel: label})
	}
	for _, p := range ctx.AnalysisIR.RequestModel.DiagramHint.Participants {
		b.ParticipantBoundaries = append(b.ParticipantBoundaries, types.DiagramParticipantBoundary{Participant: p.Identity, Status: types.DiagramParticipantBoundaryUnproven})
	}
	raw, _ := json.Marshal(doc)
	first, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	t.Logf("fresh public emit of archived diagram shape: %v %s", err, first.Summary)
	if err != nil || first.Success || !strings.Contains(first.Summary, "boundary_participant_not_visible") {
		t.Fatalf("initial rejection: %v %+v", err, first)
	}
	lease := installRoleVisibilityPublicRetryHMC302(t, ctx, first)
	if len(lease.ParticipantVisibilityFailures) != 3 {
		t.Fatalf("want three fresh failures: %+v", lease)
	}
	byRole := map[string]int{"主线程": 100, "文档工作线程": 200, "存储完成": 80}
	labels := map[int]string{100: "主线程<br/>app-main-100", 200: "文档工作线程<br/>document-worker-200", 80: "存储完成<br/>storage-irq-80"}
	var edits []map[string]string
	for _, f := range lease.ParticipantVisibilityFailures {
		tid := byRole[f.Participant]
		if nodes[tid] == "" {
			t.Fatalf("unexpected role: %+v", f)
		}
		edits = append(edits, map[string]string{"participant_ref": f.ParticipantRef, "action": "ensure_visible", "node_id": nodes[tid], "visible_label": labels[tid]})
	}
	return ctx, doc, rows, edits
}

func TestHMC314PublicCorrectVisibilityDoesNotSilentlyRebind(t *testing.T) {
	ctx, _, _, edits := staleNativeRetryHMC314(t)
	before := ctx.Mutable.LastRejectedAnswerDocumentV2()
	result, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, primaryIdentityPatchHMC311(t, edits))
	t.Logf("HMC313 correct-ID ensure_visible replay: %v %s", err, result.Summary)
	if err != nil || result.Success || !strings.Contains(result.Summary, staleWorkerHMC314) || !strings.Contains(result.Summary, "unavailable or stale") {
		t.Fatalf("display-only operation must not infer an old native binding: %v %+v", err, result)
	}
	if ctx.Mutable.AnswerDocumentV2() != nil || !reflect.DeepEqual(before, ctx.Mutable.LastRejectedAnswerDocumentV2()) {
		t.Fatal("failed display edit changed the retained draft")
	}
}

func TestHMC314PublicExplicitNativeSelectionRepair(t *testing.T) {
	ctx, doc, _, edits := staleNativeRetryHMC314(t)
	for _, edit := range edits {
		if edit["visible_label"] == "文档工作线程<br/>document-worker-200" {
			edit["native_participant"] = edit["node_id"]
			edit["node_id"] = staleWorkerHMC314
		}
	}
	raw := primaryIdentityPatchHMC311(t, edits)
	t.Logf("explicit same-generation native re-selection: %s", raw)
	result, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	t.Logf("public explicit repair: %v %s", err, result.Summary)
	if err != nil || !result.Success {
		t.Fatalf("precise explicit selector repair unavailable: %v %s", err, result.Summary)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("accepted repair lost document")
	}
	before, after := doc.Blocks[1], got.Blocks[1]
	if len(mermaidcompat.RemovableNodeDeclarations(after.Diagram.Body)) != 3 ||
		!reflect.DeepEqual(before.EdgeAnchors, after.EdgeAnchors) ||
		!reflect.DeepEqual(mermaidcompat.ParseEdges(before.Diagram.Body), mermaidcompat.ParseEdges(after.Diagram.Body)) ||
		!reflect.DeepEqual(before.ParticipantBoundaries, after.ParticipantBoundaries) ||
		!reflect.DeepEqual(doc.Blocks[0], got.Blocks[0]) || !reflect.DeepEqual(doc.Blocks[2], got.Blocks[2]) {
		t.Fatal("selector repair changed non-target bytes or graph ownership")
	}
}
