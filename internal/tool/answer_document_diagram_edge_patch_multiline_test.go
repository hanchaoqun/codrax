package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitAnswerDocumentPatch_MultilineEndpointLabelPreservesLongRuntimeNode(t *testing.T) {
	const node = "rt_e215c998b0d2aab43132b4211c35b9d6897dfac6f4871e97b720427aff6ccf51"
	prev := atomicPatchTestDocument()
	prev.Blocks[1].Diagram.Kind = types.DiagramFlow
	prev.Blocks[1].Diagram.Body = "flowchart LR\n    A[\"Analyzer\"]\n    B[\"Explorer\"]\n    C[\"Extractor\"]\n    A -->|old label| B\n    B -->|keep label| C\n"
	mut := types.NewMutableState("multiline endpoint")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	mut.SetAnswerDiagramRelationRepairLease(types.NewAnswerDiagramRelationRepairLease(prev,
		[]types.AnswerDiagramRelationRepairFailure{{
			BlockID: "diag", Issue: "semantic_relation_edge_unproven",
			FromNode: "A", ToNode: "B", FromIdentity: "Analyzer", ToIdentity: "Explorer",
			RelationKind: types.DiagramRelPrecedence,
		}}, []types.AnswerDiagramRelationRepairCandidate{{
			BlockID: "diag", RelationKind: types.DiagramRelPrecedence,
			FromIdentity: "Extractor", ToIdentity: "Finalizer", Source: "stageauthority",
			FromNodeIDs: []string{"C"}, ToNodeIDs: []string{node},
		}}))
	bus := &types.BusContext{Mutable: mut}
	raw, err := json.Marshal(map[string]any{
		"unchanged_block_ids": []string{"summary"},
		"diagram_edge_edits": []map[string]any{
			{"block_id": "diag", "action": "remove", "match": map[string]string{
				"from_node": "A", "to_node": "B", "from_identity": "Analyzer", "to_identity": "Explorer", "relation_kind": "precedence",
			}},
			{"block_id": "diag", "action": "add", "to_node_visible_label": "答案组织\r\n已收集事实\n保持证据范围", "edge": map[string]string{
				"from_node": "C", "to_node": node, "from_identity": "Extractor", "to_identity": "Finalizer",
				"relation_kind": "precedence", "visible_label": "结构化事实就绪后组织答案",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || !res.Success {
		t.Fatalf("legal long endpoint ID and real label newlines must pass public patch: err=%v res=%+v", err, res)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || got.Blocks[1].Diagram == nil {
		t.Fatalf("document not persisted: %+v", got)
	}
	diagram := got.Blocks[1]
	if !strings.Contains(diagram.Diagram.Body, node+`["答案组织<br/>已收集事实<br/>保持证据范围"]`) ||
		!strings.Contains(diagram.Diagram.Body, "B -->|keep label| C") || len(mermaidcompat.ParseEdges(diagram.Diagram.Body)) != 2 {
		t.Fatalf("format repair changed graph content:\n%s", diagram.Diagram.Body)
	}
	if len(diagram.EdgeAnchors) != 2 || diagram.EdgeAnchors[1].ToNode != node || diagram.EdgeAnchors[1].ToIdentity != "Finalizer" {
		t.Fatalf("format repair changed typed endpoint identity: %+v", diagram.EdgeAnchors)
	}
}

func TestAtomicDiagramMultilineLabelsPreserveTopologyThroughRenderer(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		kind       types.DiagramKind
	}{
		{"flow", "flowchart LR\n Existing[\"Existing\"]", types.DiagramFlow},
		{"sequence", "sequenceDiagram\n participant Existing as \"Existing\"", types.DiagramSequence},
		{"class", "classDiagram\n class Existing[\"Existing\"]", types.DiagramArchitecture},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const label = "应用提交\r\nrender & \"owner\" < candidate >\n渲染服务接收"
			prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
				ID: "diag", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: tc.kind, Language: "mermaid", Body: tc.body},
			}}}
			patch := &types.AnswerDocumentV2Patch{}
			err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
				BlockID: "diag", Action: "add", ToNodeVisibleLabel: label, PlacementRef: sequenceEndPlacementForTest(prev, "diag"),
				Edge: &types.DiagramEdgeAnchor{FromNode: "Existing", ToNode: "Receiver", FromIdentity: "pkg.Existing", ToIdentity: "pkg.Receiver", RelationKind: types.DiagramRelCall, VisibleLabel: "提交任务"},
			}}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			block := patch.ReplaceBlocks[0]
			body := block.Diagram.Body
			if len(mermaidcompat.ParseEdges(body)) != 1 || len(block.EdgeAnchors) != 1 {
				t.Fatalf("newlines must not create extra graph relations:\n%s", body)
			}
			beforeReplay := body
			if err := ensureAtomicDiagramEndpointDeclarations(&block, emitAnswerDiagramEdgeEdit{
				ToNodeVisibleLabel: label, Edge: &types.DiagramEdgeAnchor{FromNode: "Existing", ToNode: "Receiver"},
			}); err != nil || block.Diagram.Body != beforeReplay {
				t.Fatalf("authored multiline label replay must be idempotent: %v\n%s", err, block.Diagram.Body)
			}
			out := render.RenderMermaidBlocks("```mermaid\n" + body + "\n```")
			if !strings.Contains(out, "```text\n") {
				t.Fatalf("actual renderer did not handle normalized diagram:\n%s", out)
			}
			if tc.name == "class" {
				// Native class labels remain a browser-supported carrier. The
				// existing terminal fallback must keep their entire source.
				if !strings.Contains(out, "# ·") || !strings.Contains(out, body) {
					t.Fatalf("class fallback lost source or its explicit notice:\n%s", out)
				}
				return
			}
			if strings.Contains(out, "# ⚠") || strings.Contains(out, "# ·") || strings.Contains(out, "<br") {
				t.Fatalf("normalization must reach actual renderer successfully:\n%s", out)
			}
			for _, token := range []string{"应用提交", "render", "&", `"owner"`, "<", "candidate", ">", "渲染服务接收", "提交任务"} {
				if !strings.Contains(out, token) {
					t.Errorf("rendered visible text lost %q:\n%s", token, out)
				}
			}
		})
	}
}

func TestAtomicDiagramEndpointDeclarationFailureNamesActualProblem(t *testing.T) {
	for _, tc := range []struct{ name, node, label, reason string }{
		{"nul", "New", "line\ninvalid\x00", "unsupported control character U+0000"},
		{"id", "New;Injected", "line\nbreak", "invalid endpoint identifier"},
		{"existing label", "Existing", "different\nlabel", "exactly match the current explicit label"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
				ID: "diag", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n Existing[\"unchanged\"]"},
			}}}
			before := prev.Blocks[0].Diagram.Body
			patch := &types.AnswerDocumentV2Patch{}
			err := applyModelAuthoredDiagramAtomicEdits(prev, patch, []emitAnswerDiagramEdgeEdit{{
				BlockID: "diag", Action: "add", ToNodeVisibleLabel: tc.label,
				Edge: &types.DiagramEdgeAnchor{FromNode: "Existing", ToNode: tc.node, RelationKind: types.DiagramRelCall, VisibleLabel: "relation"},
			}}, nil, nil)
			if err == nil || !strings.Contains(err.Error(), tc.reason) || strings.Contains(err.Error(), "unsupported Mermaid family") {
				t.Fatalf("rejection must name its actual problem %q: %v", tc.reason, err)
			}
			if prev.Blocks[0].Diagram.Body != before || len(patch.ReplaceBlocks) != 0 {
				t.Fatalf("rejected edit must not mutate the retry base or publish half a patch: %+v", patch)
			}
		})
	}
}

func TestEmitAnswerDocumentPatch_InvalidLabelPreservesLiveRetryBase(t *testing.T) {
	prev := atomicPatchTestDocument()
	mut := types.NewMutableState("invalid endpoint label")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	before, err := json.Marshal(mut.AnswerDocumentV2())
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"unchanged_block_ids":["summary"],"diagram_edge_edits":[{"block_id":"diag","action":"add","to_node_visible_label":"业务名称\n后续文字\u0000","edge":{"from_node":"C","to_node":"F","from_identity":"Extractor","to_identity":"Finalizer","relation_kind":"precedence","visible_label":"facts ready"}}]}`)
	bus := &types.BusContext{Mutable: mut}
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, sequenceEndPlacementJSONForTest(t, bus, raw))
	if err != nil || res.Success || !strings.Contains(res.Summary, "unsupported control character U+0000") || strings.Contains(res.Summary, "Mermaid family") {
		t.Fatalf("public rejection must identify the illegal label, not blame the graph family: err=%v res=%+v", err, res)
	}
	after, err := json.Marshal(mut.AnswerDocumentV2())
	if err != nil || string(before) != string(after) {
		t.Fatalf("failed public patch changed the live retry base: err=%v before=%s after=%s", err, before, after)
	}
}
