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

// HMC313 A's original emit b334bc9e8768464fcf2eabfe0be417920eebd2ef5971d27d03a04ce4f567110a
// and first patch 974d4173228ec1f9f30cde29054977dfaefa83e596a5c1d85b1aec4c68490f0a
// supplied one misspelled native selector, then a correct native node with
// ensure_visible. Replay the diagram against fresh public query authority;
// archived opaque native/instance/lease IDs cannot grant new-run authority.
const staleWorkerHMC314 = "rt_thread_d71bccacfaa1e09b70572ae02c34ee9b9220bd6898ac89f14178a702128ee6a5"

func staleNativeRetryHMC314(t *testing.T, mutations ...func(*types.AnswerDocumentV2, map[int]string)) (*types.BusContext, *types.AnswerDocumentV2, []RuntimeDiagramRelation, []map[string]string) {
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
	for _, mutate := range mutations {
		mutate(doc, nodes)
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
	stripDeclarations := func(body string) string {
		var lines []string
		for _, line := range strings.Split(body, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "participant ") {
				lines = append(lines, line)
			}
		}
		return strings.Join(lines, "\n")
	}
	if stripDeclarations(before.Diagram.Body) != stripDeclarations(after.Diagram.Body) {
		t.Fatal("selector repair changed original notes/messages/whitespace")
	}
	for i, original := range before.Diagram.RuntimeParticipantsV1 {
		changed := after.Diagram.RuntimeParticipantsV1[i]
		if original.NodeID != staleWorkerHMC314 && changed != original {
			t.Fatal("valid native selector changed")
		}
		if original.NodeID == staleWorkerHMC314 && (changed.NodeID != original.NodeID || changed.NativeParticipant == original.NativeParticipant) {
			t.Fatal("explicit native re-selection did not preserve the old graph node")
		}
	}
	for _, edit := range edits {
		if !strings.Contains(after.Diagram.Body, edit["visible_label"]) {
			t.Fatal("lost business/native two-line label")
		}
	}
	roles, count := after.NativeDiagramCoverage()
	text := render.RenderAnswerDocument(got, "zh-CN")
	if len(roles) != 3 || count != 2 || !strings.Contains(text, "业务身份对应尚未核实") || !strings.Contains(text, "已验证的 2 条线程关系") {
		t.Fatalf("native proof and unproved business mapping drift: %v %d %s", roles, count, text)
	}
	clone := ctx.Mutable.AnswerDocumentV2()
	clone.Blocks[1].Diagram.RuntimeParticipantsV1[1].NativeParticipant = "forged"
	if !reflect.DeepEqual(got, ctx.Mutable.AnswerDocumentV2()) {
		t.Fatal("accepted selector slice aliases caller")
	}
	raw, _ = json.Marshal(got)
	full, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !full.Success {
		t.Fatalf("full/patch current native validation drift: %v %s", err, full.Summary)
	}
}

func TestHMC314PublicNativeReselectionRejectsScopeAndPreservesTransaction(t *testing.T) {
	for _, mode := range []string{"valid_selector", "missing_declaration", "duplicate_declaration", "duplicate_selector", "stale_ref", "old_generation", "unknown_lane", "conflicting_lane", "conflicting_edits", "cross_action", "no_lease", "whole_replace"} {
		t.Run(mode, func(t *testing.T) {
			ctx, doc, _, edits := staleNativeRetryHMC314(t, func(doc *types.AnswerDocumentV2, nodes map[int]string) {
				b := &doc.Blocks[1]
				switch mode {
				case "valid_selector":
					b.Diagram.RuntimeParticipantsV1[1].NativeParticipant = nodes[200]
				case "missing_declaration":
					body, count := mermaidcompat.RemoveRemovableNodeDeclaration(b.Diagram.Body, staleWorkerHMC314)
					if count != 1 {
						t.Fatal("missing declaration fixture")
					}
					b.Diagram.Body = body
				case "duplicate_declaration":
					b.Diagram.Body += "\nparticipant " + staleWorkerHMC314 + " as duplicate"
				case "duplicate_selector":
					b.Diagram.RuntimeParticipantsV1 = append(b.Diagram.RuntimeParticipantsV1, b.Diagram.RuntimeParticipantsV1[1])
				}
			})
			worker := -1
			for i, edit := range edits {
				if edit["visible_label"] == "文档工作线程<br/>document-worker-200" {
					worker = i
					edit["native_participant"], edit["node_id"] = edit["node_id"], staleWorkerHMC314
				}
			}
			if worker < 0 {
				t.Fatal("worker edit missing")
			}
			switch mode {
			case "stale_ref":
				edits[worker]["participant_ref"] = "rp1-cc08fe3436add4c3ba495739"
			case "old_generation":
				doc.Blocks[1].Diagram.Body += "\n%% next generation"
				raw, _ := json.Marshal(doc)
				first, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
				if err != nil {
					t.Fatal(err)
				}
				installRoleVisibilityPublicRetryHMC302(t, ctx, first)
			case "unknown_lane":
				edits[worker]["native_participant"] = "rt_thread_other_capture"
			case "conflicting_lane":
				edits[worker]["native_participant"] = doc.Blocks[1].Diagram.RuntimeParticipantsV1[0].NativeParticipant
			case "conflicting_edits":
				other := (worker + 1) % len(edits)
				edits[other]["node_id"] = staleWorkerHMC314
				edits[other]["native_participant"] = edits[worker]["native_participant"]
			case "cross_action":
				edits[worker]["action"] = "retain_as_context"
				edits[worker]["block_id"], edits[worker]["participant_id"] = doc.Blocks[1].ID, staleWorkerHMC314
				delete(edits[worker], "participant_ref")
				delete(edits[worker], "node_id")
			case "no_lease":
				ctx.Mutable.SetAnswerDiagramRelationRepairLease(nil)
			}
			before := ctx.Mutable.LastRejectedAnswerDocumentV2()
			pending := ctx.Mutable.PendingAnswerDocumentPatchBase()
			if mode == "valid_selector" || mode == "missing_declaration" || mode == "duplicate_declaration" || mode == "duplicate_selector" || mode == "no_lease" {
				schema := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{RepoRoot: ctx.RepoRoot, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR})
				var root map[string]any
				if err := json.Unmarshal(schema, &root); err != nil {
					t.Fatal(err)
				}
				p, _ := root["properties"].(map[string]any)
				e, _ := p["diagram_participant_edits"].(map[string]any)
				i, _ := e["items"].(map[string]any)
				branches, _ := i["oneOf"].([]any)
				for _, rawBranch := range branches {
					branch := rawBranch.(map[string]any)
					fields := branch["properties"].(map[string]any)
					if _, ok := fields["native_participant"]; ok {
						t.Fatalf("ineligible selector gained projected repair: %s %s", mode, schema)
					}
				}
			}
			raw := primaryIdentityPatchHMC311(t, edits)
			if mode == "whole_replace" {
				raw, _ = json.Marshal(map[string]any{"replace_blocks": []types.AnswerBlock{doc.Blocks[1]}, "unchanged_block_ids": []string{"summary", "table"}})
			}
			result, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
			t.Logf("negative %s: %v %s", mode, err, result.Summary)
			if err != nil || result.Success || !strings.Contains(result.Summary, "this call was not staged") || ctx.Mutable.AnswerDocumentV2() != nil {
				t.Fatalf("invalid repair acquired permission: %v %+v", err, result)
			}
			if !reflect.DeepEqual(before, ctx.Mutable.LastRejectedAnswerDocumentV2()) || !reflect.DeepEqual(pending, ctx.Mutable.PendingAnswerDocumentPatchBase()) {
				t.Fatal("failed repair mutated the baseline")
			}
		})
	}
}

func TestHMC314PublicReselectionSchemaIsBoundedAndExact(t *testing.T) {
	ctx, _, rows, _ := staleNativeRetryHMC314(t)
	raw := (&EmitAnswerDocumentPatch{}).ParametersFor(&types.AgentContext{RepoRoot: ctx.RepoRoot, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR})
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	props := root["properties"].(map[string]any)
	branches := props["diagram_participant_edits"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	ordinary, explicit := 0, 0
	lanes := runtimeNativeParticipantLanes(rows)
	for _, branch := range branches {
		b := branch.(map[string]any)
		p := b["properties"].(map[string]any)
		native, ok := p["native_participant"].(map[string]any)
		if !ok {
			ordinary++
			continue
		}
		explicit++
		nodes := schemaStringValues(p["node_id"].(map[string]any)["enum"])
		ids := schemaStringValues(native["enum"])
		if !reflect.DeepEqual(nodes, []string{staleWorkerHMC314}) || len(ids) != len(lanes) || b["additionalProperties"] != false {
			t.Fatalf("schema widened selector roster: %+v", b)
		}
		for _, id := range ids {
			if _, ok := lanes[id]; !ok {
				t.Fatal("non-current native choice advertised")
			}
		}
		required := schemaStringValues(b["required"])
		found := false
		for _, name := range required {
			found = found || name == "native_participant"
		}
		if !found {
			t.Fatal("native re-selection must be explicit")
		}
	}
	if ordinary != 3 || explicit != 3 {
		t.Fatalf("want one extra compact branch per visibility failure, not node x lane cross-product: %d/%d", ordinary, explicit)
	}
	for _, id := range schemaStringValues(props["replace_blocks"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["id"].(map[string]any)["enum"]) {
		if id == "diag" {
			t.Fatal("native repair widened whole-block replacement")
		}
	}
}

func TestHMC314PublicExplicitReselectionUsesExactAliasNotIDSimilarity(t *testing.T) {
	const alias = "historical_lane_A"
	ctx, doc, _, edits := staleNativeRetryHMC314(t, func(doc *types.AnswerDocumentV2, _ map[int]string) {
		b := &doc.Blocks[1]
		b.Diagram.Body = strings.ReplaceAll(b.Diagram.Body, staleWorkerHMC314, alias)
		b.Diagram.RuntimeParticipantsV1[1].NodeID = alias
		for i := range b.EdgeAnchors {
			if b.EdgeAnchors[i].FromNode == staleWorkerHMC314 {
				b.EdgeAnchors[i].FromNode = alias
			}
			if b.EdgeAnchors[i].ToNode == staleWorkerHMC314 {
				b.EdgeAnchors[i].ToNode = alias
			}
		}
	})
	for _, edit := range edits {
		if edit["visible_label"] == "文档工作线程<br/>document-worker-200" {
			edit["native_participant"], edit["node_id"] = edit["node_id"], alias
		}
	}
	result, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, primaryIdentityPatchHMC311(t, edits))
	if err != nil || !result.Success {
		t.Fatalf("exact opaque alias repair: %v %s", err, result.Summary)
	}
	got := ctx.Mutable.AnswerDocumentV2().Blocks[1]
	if !reflect.DeepEqual(got.EdgeAnchors, doc.Blocks[1].EdgeAnchors) || got.Diagram.RuntimeParticipantsV1[1].NodeID != alias {
		t.Fatal("reselection inferred/renamed a node")
	}
}
