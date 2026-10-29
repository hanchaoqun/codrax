package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Reproduce the HMC309 A shape through public analysis/query/emit, without
// importing old result authority or claiming the business roles are proved.
func nativeBoundaryFixtureHMC310(t *testing.T) (*types.BusContext, []RuntimeDiagramRelation, *types.AnswerDocumentV2) {
	t.Helper()
	const roles = "主线程、文档工作线程和存储完成"
	question := "分析附件中 OpenDocument 为什么慢，用时序图说明" + roles + "之间的先后与唤醒关系。"
	ctx, payload, _ := requestedThreadAnalysisInputHMC242(t, question, nil)
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx.AttachedHitrace = string(data)
	payload["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "bounded_selector", "source_quote": "OpenDocument", "confidence": 1.0}
	payload["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": roles, "confidence": .7}
	payload["runtime_targets"] = []any{
		map[string]any{"kind": "thread", "thread": "主线程", "source": "user_explicit", "confidence": .75},
		map[string]any{"kind": "thread", "thread": "文档工作线程", "source": "user_explicit", "confidence": .75},
	}
	payload["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": .95, "dimensions": []any{map[string]any{"index": 1, "label": "时序图", "required": true, "role": "diagram", "source_quote": "用时序图说明"}}}
	var participants []any
	for _, role := range []string{"主线程", "文档工作线程", "存储完成"} {
		participants = append(participants, map[string]any{"identity": role, "source_quote": role, "role": "incident_required"})
	}
	payload["diagram_hint"] = map[string]any{"required": true, "kind": "sequence", "relation_scope_quote": roles + "之间的先后与唤醒关系", "participants": participants}
	raw, _ := json.Marshal(payload)
	analysis, err := (&EmitAnalysis{}).Execute(ctx, raw)
	if err != nil || !analysis.Success {
		t.Fatalf("analysis: %v %s", err, analysis.Summary)
	}
	rm := ctx.Mutable.RequestModel()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: *rm, AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{Required: true, RequiredKind: types.DiagramSequence, Participants: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)}}}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "wakeup_chain", "pid": 100, "time_start": 1.0, "time_end": 1.05, "platform": "harmony"})
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	rows := runtimeDiagramRelationsForContext(ctx)
	if len(rows) != 2 {
		t.Fatalf("want two current native wakeups: %+v", rows)
	}
	for _, row := range rows {
		if len(row.requestedFrom)+len(row.requestedTo) != 0 {
			t.Fatal("unproved business role borrowed native thread identity")
		}
	}
	doc := requestedThreadDocHMC242(rows, true)
	doc.Blocks[0].Text = "当前窗口记录两次唤醒；原生线程与业务角色的对应仍待核实。"
	block := &doc.Blocks[1]
	aliases, labels := map[string]string{}, map[string]string{}
	roleForTID := map[int]string{100: "主线程", 200: "文档工作线程", 80: "存储完成"}
	aliasForTID := map[int]string{100: "MAIN", 200: "WORKER", 80: "STORAGE"}
	for _, row := range rows {
		if row.threadEndpoints == nil {
			t.Fatal("native thread endpoint receipt absent")
		}
		for _, side := range []struct {
			node, label string
			tid         int
		}{{row.FromNode, row.FromLabel, row.threadEndpoints.from.PID}, {row.ToNode, row.ToLabel, row.threadEndpoints.to.PID}} {
			aliases[side.node] = aliasForTID[side.tid]
			labels[side.node] = roleForTID[side.tid] + " " + side.label
		}
	}
	var body strings.Builder
	body.WriteString("sequenceDiagram\n")
	seen := map[string]bool{}
	for _, row := range rows {
		for _, node := range []string{row.FromNode, row.ToNode} {
			if !seen[node] {
				fmt.Fprintf(&body, " participant %s as %s\n", aliases[node], labels[node])
				seen[node] = true
			}
		}
		fmt.Fprintf(&body, " %s->>%s: 唤醒\n", aliases[row.FromNode], aliases[row.ToNode])
	}
	block.Diagram.Body = body.String()
	for i := range block.EdgeAnchors {
		block.EdgeAnchors[i].FromNode = aliases[block.EdgeAnchors[i].FromNode]
		block.EdgeAnchors[i].ToNode = aliases[block.EdgeAnchors[i].ToNode]
	}
	return ctx, rows, doc
}

func TestHMC310NativeBoundaryPublicCoverageDisclosure(t *testing.T) {
	ctx, rows, doc := nativeBoundaryFixtureHMC310(t)
	raw, _ := json.Marshal(doc)
	first, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || first.Success || !strings.Contains(first.Summary, "missing_unproven_boundary") {
		t.Fatalf("unknown role mapping must remain rejected: %v %s", err, first.Summary)
	}
	t.Logf("first public rejection (correct arrows, unresolved roles): %s", first.Summary)
	if !strings.Contains(first.Summary, "requested_native_identity_unverified=") || !strings.Contains(first.Summary, "second <br/> line") {
		t.Fatal("repair conflates identity uncertainty with missing native edges or drops exact identity guidance")
	}
	block := &doc.Blocks[1]
	for _, p := range ctx.AnalysisIR.RequestModel.DiagramHint.Participants {
		block.ParticipantBoundaries = append(block.ParticipantBoundaries, types.DiagramParticipantBoundary{Participant: p.Identity, Status: types.DiagramParticipantBoundaryUnproven})
		// A model-authored exact first line preserves the boundary identity while
		// a secondary native label retains the observed thread, without new lanes.
		block.Diagram.Body = strings.ReplaceAll(block.Diagram.Body, " as "+p.Identity+" ", " as "+p.Identity+"<br/>")
	}
	raw, _ = json.Marshal(doc)
	result, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("bounded native diagram: %v %s", err, result.Summary)
	}
	accepted := ctx.Mutable.AnswerDocumentV2()
	got := accepted.Blocks[1]
	if !reflect.DeepEqual(got.EdgeAnchors, block.EdgeAnchors) || len(mermaidcompat.RemovableNodeDeclarations(got.Diagram.Body)) != 3 {
		t.Fatalf("native anchors/lanes changed: %+v", got)
	}
	for _, row := range rows {
		if !strings.Contains(got.Diagram.Body, row.FromLabel) || !strings.Contains(got.Diagram.Body, row.ToLabel) {
			t.Fatal("native labels lost")
		}
	}
	text := render.RenderAnswerDocument(accepted, "zh-CN")
	t.Logf("accepted public render:\n%s", text)
	if !strings.Contains(text, "业务身份对应尚未核实") || strings.Contains(text, "未证明用户所请求的有向关系") {
		t.Fatalf("native edge proof and unproved role identity were collapsed into one negative disclosure:\n%s", text)
	}
	if !strings.Contains(text, "已验证的 2 条线程关系") {
		t.Fatal("wrong verified relation count")
	}
	roles, n := got.NativeDiagramCoverage()
	if len(roles) != 3 || n != 2 {
		t.Fatalf("persisted clone lost display receipt: %v %d", roles, n)
	}
	raw, _ = json.Marshal(accepted)
	var restored types.AnswerDocumentV2
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if _, n := restored.Blocks[1].NativeDiagramCoverage(); n != 0 {
		t.Fatal("JSON granted relation display validation")
	}
	if text := render.RenderAnswerDocument(&restored, "zh-CN"); strings.Contains(text, "业务身份对应尚未核实") {
		t.Fatal("unvalidated JSON rendered an accepted receipt")
	}
	// A new public emit may legitimately revalidate against current evidence.
	if result, err := (&EmitAnswerDocument{}).Execute(ctx, raw); err != nil || !result.Success {
		t.Fatalf("fresh public revalidation failed: %v %s", err, result.Summary)
	}
	if _, n := ctx.Mutable.AnswerDocumentV2().Blocks[1].NativeDiagramCoverage(); n != 2 {
		t.Fatal("fresh emit did not rebind current relation facts")
	}
}

func TestHMC310NativeBoundaryReceiptDoesNotBorrowLegacyOrInvisibleAnchors(t *testing.T) {
	for _, mode := range []string{"legacy", "no_visible_edge", "wrong_endpoint", "wrong_relation", "no_private_thread"} {
		t.Run(mode, func(t *testing.T) {
			ctx, rows, doc := nativeBoundaryFixtureHMC310(t)
			b := &doc.Blocks[1]
			b.ParticipantBoundaries = []types.DiagramParticipantBoundary{{Participant: "主线程", Status: types.DiagramParticipantBoundaryUnproven}}
			switch mode {
			case "legacy":
				doc.Blocks[0].EdgeAnchors = b.EdgeAnchors
				b.EdgeAnchors = nil
			case "no_visible_edge":
				b.Diagram.Body = "sequenceDiagram\nparticipant MAIN as 主线程"
			case "wrong_endpoint":
				for i := range b.EdgeAnchors {
					b.EdgeAnchors[i].ToIdentity = "not-current"
				}
			case "wrong_relation":
				for i := range b.EdgeAnchors {
					b.EdgeAnchors[i].RelationKind = types.DiagramRelCall
				}
			case "no_private_thread":
				row := rows[0]
				row.threadEndpoints = nil
				if runtimeThreadDiagramRelationProved([]RuntimeDiagramRelation{row}, b.EdgeAnchors[0]) {
					t.Fatal("public tuple minted private endpoint proof")
				}
				return
			}
			bindNativeDiagramCoverage(doc, ctx)
			if p, n := b.NativeDiagramCoverage(); len(p) != 0 || n != 0 {
				t.Fatalf("invalid display proof borrowed: %v %d", p, n)
			}
		})
	}
}
