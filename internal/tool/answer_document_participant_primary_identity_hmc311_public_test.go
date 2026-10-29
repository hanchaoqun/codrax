package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC310 A's 5ab11.../08a9ed46 public patch used these exact visible labels.
// The opaque participant refs are reissued from the current public rejection;
// old-run refs must never import authority into a new query or diagram.
func primaryIdentityRetryHMC311(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2, []RuntimeDiagramRelation, []map[string]string) {
	t.Helper()
	ctx, rows, doc := nativeBoundaryFixtureHMC310(t)
	block := &doc.Blocks[1]
	block.Diagram.Body += " Note over WORKER: preserved state interval\n"
	for _, p := range ctx.AnalysisIR.RequestModel.DiagramHint.Participants {
		block.ParticipantBoundaries = append(block.ParticipantBoundaries, types.DiagramParticipantBoundary{Participant: p.Identity, Status: types.DiagramParticipantBoundaryUnproven})
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || result.Success || !strings.Contains(result.Summary, "boundary_participant_not_visible") {
		t.Fatalf("public initial rejection: %v %+v", err, result)
	}
	t.Logf("initial public emit: %s", result.Summary)
	lease := installRoleVisibilityPublicRetryHMC302(t, ctx, result)
	if len(lease.ParticipantVisibilityFailures) != 3 {
		t.Fatalf("want 3 current exact visibility failures: %+v", lease)
	}
	nodes := map[string]string{"主线程": "MAIN", "文档工作线程": "WORKER", "存储完成": "STORAGE"}
	labels := map[string]string{
		"主线程":    "主线程<br/>app-main-100",
		"文档工作线程": "文档工作线程<br/>document-worker-200",
		"存储完成":   "存储完成<br/>storage-irq-80",
	}
	var edits []map[string]string
	for _, f := range lease.ParticipantVisibilityFailures {
		if nodes[f.Participant] == "" || labels[f.Participant] == "" {
			t.Fatalf("unexpected public failure: %+v", f)
		}
		edits = append(edits, map[string]string{
			"participant_ref": f.ParticipantRef, "action": "ensure_visible",
			"node_id": nodes[f.Participant], "visible_label": labels[f.Participant],
		})
	}
	return ctx, doc, rows, edits
}

func primaryIdentityPatchHMC311(t *testing.T, edits []map[string]string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"unchanged_block_ids":       []string{"summary", "table"},
		"diagram_participant_edits": edits,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestHMC311PrimaryIdentityPublicAtomicRepair(t *testing.T) {
	ctx, original, rows, edits := primaryIdentityRetryHMC311(t)
	raw := primaryIdentityPatchHMC311(t, edits)
	t.Logf("public ensure_visible patch (fresh refs, HMC310 visible labels): %s", raw)
	result, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	t.Logf("public atomic result: err=%v success=%t summary=%s", err, result.Success, result.Summary)
	if err != nil || !result.Success {
		t.Fatalf("documented first-line role plus native second-line must survive public atomic repair: %v %s", err, result.Summary)
	}
	accepted := ctx.Mutable.AnswerDocumentV2()
	if accepted == nil {
		t.Fatal("accepted patch lost document")
	}
	got, before := accepted.Blocks[1], original.Blocks[1]
	if len(mermaidcompat.RemovableNodeDeclarations(got.Diagram.Body)) != 3 ||
		!reflect.DeepEqual(mermaidcompat.ParseEdges(got.Diagram.Body), mermaidcompat.ParseEdges(before.Diagram.Body)) ||
		!reflect.DeepEqual(got.EdgeAnchors, before.EdgeAnchors) ||
		!reflect.DeepEqual(got.ParticipantBoundaries, before.ParticipantBoundaries) ||
		!reflect.DeepEqual(got.Diagram.RuntimeParticipantsV1, before.Diagram.RuntimeParticipantsV1) {
		t.Fatalf("display repair changed native nodes/relations/unproved boundaries: %+v", got)
	}
	for _, edit := range edits {
		if !strings.Contains(got.Diagram.Body, edit["visible_label"]) {
			t.Fatalf("lost exact multiline display %q: %s", edit["visible_label"], got.Diagram.Body)
		}
	}
	// Only declaration labels may change. All messages, state notes and all
	// non-diagram model blocks retain their original bytes and typed metadata.
	stripDeclarations := func(body string) string {
		var lines []string
		for _, line := range strings.Split(body, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "participant ") {
				lines = append(lines, line)
			}
		}
		return strings.Join(lines, "\n")
	}
	if stripDeclarations(got.Diagram.Body) != stripDeclarations(before.Diagram.Body) ||
		!reflect.DeepEqual(accepted.Blocks[0], original.Blocks[0]) ||
		!reflect.DeepEqual(accepted.Blocks[2], original.Blocks[2]) {
		t.Fatal("local repair mutated non-target bytes")
	}
	for _, row := range rows {
		if len(row.requestedFrom)+len(row.requestedTo) != 0 {
			t.Fatal("display label created requested-to-native identity authority")
		}
	}
	roles, count := got.NativeDiagramCoverage()
	if len(roles) != 3 || count != 2 {
		t.Fatalf("current private display receipt lost: %v %d", roles, count)
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if bad := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, accepted, view, nil); len(bad) != 0 {
		t.Fatalf("native edge check disagrees after atomic repair: %+v", bad)
	}
	text := render.RenderAnswerDocument(accepted, "zh-CN")
	if !strings.Contains(text, "业务身份对应尚未核实") || !strings.Contains(text, "已验证的 2 条线程关系") {
		t.Fatalf("render collapsed role uncertainty and native edge proof: %s", text)
	}
	// The ordinary full emitter must independently accept the same serialized
	// display, revalidating current evidence rather than trusting a receipt.
	raw, err = json.Marshal(accepted)
	if err != nil {
		t.Fatal(err)
	}
	full, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !full.Success {
		t.Fatalf("full/atomic identity contract drift: %v %s", err, full.Summary)
	}
}

func TestHMC311PrimaryIdentityPublicAtomicRejectsNonPrimaryOrStale(t *testing.T) {
	for _, mode := range []string{"secondary_business", "prefix_similarity", "qualified_business", "wrong_role", "stale_ref"} {
		t.Run(mode, func(t *testing.T) {
			ctx, _, _, edits := primaryIdentityRetryHMC311(t)
			before := ctx.Mutable.PendingAnswerDocumentPatchBase()
			if before == nil {
				before = ctx.Mutable.LastRejectedAnswerDocumentV2()
			}
			switch mode {
			case "secondary_business":
				edits[0]["visible_label"] = "app-main-100<br/>主线程"
			case "prefix_similarity":
				edits[0]["visible_label"] = "主线程池<br/>app-main-100"
			case "qualified_business":
				edits[0]["visible_label"] = "主线程（推测）<br/>app-main-100"
			case "wrong_role":
				edits[0]["visible_label"] = "其他角色<br/>app-main-100"
			case "stale_ref":
				edits[0]["participant_ref"] = "rp1-95f3ba834799179210b8b576"
			}
			result, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, primaryIdentityPatchHMC311(t, edits))
			t.Logf("negative public patch %s: %s", mode, result.Summary)
			if err != nil || result.Success || !strings.Contains(result.Summary, "this call was not staged") {
				t.Fatalf("non-primary identity or old-run ref gained permission: %v %+v", err, result)
			}
			after := ctx.Mutable.PendingAnswerDocumentPatchBase()
			if after == nil {
				after = ctx.Mutable.LastRejectedAnswerDocumentV2()
			}
			if !reflect.DeepEqual(before, after) || ctx.Mutable.AnswerDocumentV2() != nil {
				t.Fatal("rejected visibility patch mutated the live baseline")
			}
		})
	}
}

func TestHMC311PrimaryIdentityDisplayCannotProveNativeRelations(t *testing.T) {
	for _, mode := range []string{"missing_anchors", "wrong_endpoint", "wrong_identity", "missing_unproven_boundary"} {
		t.Run(mode, func(t *testing.T) {
			ctx, doc, rows, edits := primaryIdentityRetryHMC311(t)
			block := &doc.Blocks[1]
			for _, edit := range edits {
				var count int
				block.Diagram.Body, count = mermaidcompat.RewriteRemovableNodeDeclarationLabel(block.Diagram.Body, edit["node_id"], edit["visible_label"])
				if count != 1 {
					t.Fatal("fixture needs one exact declaration per node")
				}
			}
			switch mode {
			case "missing_anchors":
				block.EdgeAnchors = nil
			case "wrong_endpoint":
				block.EdgeAnchors[0].FromNode = "OTHER"
			case "wrong_identity":
				block.EdgeAnchors[0].FromIdentity = "unobserved-thread-80"
			case "missing_unproven_boundary":
				block.ParticipantBoundaries = nil
			}
			for _, row := range rows {
				if len(row.requestedFrom)+len(row.requestedTo) != 0 {
					t.Fatal("fixture accidentally proved a business role")
				}
			}
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			t.Logf("negative public emit %s: %s", mode, result.Summary)
			if err != nil || result.Success || ctx.Mutable.AnswerDocumentV2() != nil {
				t.Fatalf("display identity granted native relation or business mapping: %v %+v", err, result)
			}
		})
	}
}

func TestHMC311PrimaryIdentityExactProjectionAcrossLabels(t *testing.T) {
	for _, tc := range []struct {
		name, identity, label string
		want                  bool
	}{
		{"chinese_first", "工作线程", "工作线程<br/>native-worker-17", true},
		{"multiword_first", "document worker", "document worker<br>native-worker-17", true},
		{"newline_first", "lecture du document", "lecture du document\nnative-worker-17", true},
		{"escaped_newline_first", "document worker", `document worker\nnative-worker-17`, true},
		{"whole_business", "document worker", "document worker", true},
		{"old_code_secondary", "Analyzer", "analyze<br/>Analyzer", true},
		{"hidden_business", "document worker", "native-worker-17<br/>document worker", false},
		{"hidden_chinese", "工作线程", "native-worker-17<br/>工作线程", false},
		{"prefix_only", "document worker", "document workers<br/>native-worker-17", false},
		{"qualifier_not_identity", "document worker", "document worker (assumed)<br/>native-worker-17", false},
		{"whole_label_not_split", "document worker", "document worker native-worker-17", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := diagramParticipantEndpointExplicitlyDisplaysIdentity([]string{tc.identity}, "n", map[string]string{"n": tc.label})
			if got != tc.want {
				t.Fatalf("exact projection identity=%q label=%q got=%t want=%t", tc.identity, tc.label, got, tc.want)
			}
		})
	}
}
