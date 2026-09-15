package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The source relation is produced by ReadFile/EmitEvidence. Answer repair then
// crosses the actual Emit -> Observe -> dynamic ParametersFor -> Patch -> Render
// boundaries, without a hand-built repair delta or lease and without an LLM.
func b1690StandaloneFixture(t *testing.T, kind types.AnswerBlockKind, broken bool) (*types.BusContext, *types.AgentContext, *types.AnswerDocumentV2, types.EvidenceItem) {
	t.Helper()
	repo := t.TempDir()
	source := "package sample\nfunc StoreSave() {}\nfunc Caller(kind string) {\n if kind == \"store\" { StoreSave() }\n}\n"
	if err := os.WriteFile(filepath.Join(repo, "calls.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("Explain the selected operation")}
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"calls.go","offset":0,"limit":50}`))
	if err != nil || !read.Success {
		t.Fatalf("read: err=%v result=%+v", err, read)
	}
	bus.ToolResults = []types.ToolResult{read}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: bus.ToolResults})
	form, object := types.ClaimCallEdge, "StoreSave"
	evidenceParams := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"mechanism","subject":"sample.Caller","predicate":"calls","object":"StoreSave","source":"calls.go","line_start":4,"anchor_kind":"call","anchor_symbol":"StoreSave"}]}`)
	if broken {
		form, object = types.ClaimGuardCondition, "kind"
		evidenceParams = json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"mechanism","subject":"sample.Caller","predicate":"guards","object":"kind","source":"calls.go","line_start":4,"anchor_kind":"condition","anchor_symbol":"kind","condition":"kind == \"store\""}]}`)
	}
	emit, err := (&tool.EmitEvidence{}).Execute(bus, evidenceParams)
	if err != nil || !emit.Success {
		t.Fatalf("source evidence: err=%v result=%+v", err, emit)
	}
	var ev types.EvidenceItem
	for _, item := range bus.Mutable.EmittedEvidence() {
		if types.ClaimFormOf(item) == form && item.IsCitable() {
			if ev.ID != "" {
				t.Fatal("ambiguous fixture evidence")
			}
			ev = item
		}
	}
	if ev.ID == "" || ev.Subject != "sample.Caller" || ev.Object != object || ev.LineStart != 4 {
		t.Fatalf("expected actual grounded qualified call: %+v", ev)
	}
	bus.EvidenceItems = []types.EvidenceItem{ev}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}}}
	ctx := &types.AgentContext{Mutable: bus.Mutable, EvidenceItems: bus.EvidenceItems, AnalysisIR: bus.AnalysisIR, RepoRoot: repo}
	block := types.AnswerBlock{ID: "path", Kind: kind, SurfaceRole: types.SurfacePrincipal,
		Title: "Model title", Text: "Model introduction",
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: form, EvidenceID: ev.ID}},
		Items:     []types.AnswerBlockItem{{ID: "step", Text: "Model explanation remains unchanged", EvidenceIDs: []string{ev.ID}, CitationRef: 0}}}
	if kind == types.BlockTable {
		block.Columns = []string{"Explanation"}
		block.Items[0].Cells = []string{block.Items[0].Text}
	}
	if broken {
		// The old row chose the wrong relation kind. The exact source alternative
		// is a guard, so attach may not change this call into a different relation.
		block.ClaimUses = append(block.ClaimUses, types.RenderedClaimUse{ClaimForm: types.ClaimCallEdge})
		block.EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: ev.Subject, ToNode: ev.Object,
			ToIdentity: ev.Object, RelationKind: types.DiagramRelCall, VisibleLabel: "Model original relation"}}
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Citations: []types.Citation{{File: "calls.go", Line: 4, Quote: "StoreSave()"}},
		Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "Model conclusion"}, block}}
	return bus, ctx, doc, ev
}

func b1690JSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func b1690AdditionBranch(t *testing.T, raw json.RawMessage, ref string) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	var found []map[string]any
	var visit func(any)
	visit = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			props, _ := node["properties"].(map[string]any)
			selector, _ := props["addition_ref"].(map[string]any)
			values, _ := selector["enum"].([]any)
			action, _ := props["action"].(map[string]any)
			actions, _ := action["enum"].([]any)
			if len(values) == 1 && values[0] == ref && len(actions) == 1 && actions[0] == "add" {
				found = append(found, node)
			}
			for _, child := range node {
				visit(child)
			}
		case []any:
			for _, child := range node {
				visit(child)
			}
		}
	}
	visit(schema)
	if len(found) != 1 {
		t.Fatalf("want one exact add branch, got %d: %s", len(found), raw)
	}
	return found[0]
}

func TestB1690ActualStandaloneRepairLabelContract(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockOrderedList, types.BlockBulletList, types.BlockTable} {
		for _, broken := range []bool{false, true} {
			for _, language := range []string{"en", "zh"} {
				name := "missing"
				if broken {
					name = "wrong_relation"
				}
				t.Run(string(kind)+"/"+name+"/"+language, func(t *testing.T) {
					bus, ctx, doc, ev := b1690StandaloneFixture(t, kind, broken)
					bus.AnalysisIR.RequestModel.Language = language
					fromLabel, toLabel, relationLabel := "Request entry", "Order storage", "save order"
					if language == "zh" {
						fromLabel, toLabel, relationLabel = "业务入口", "订单存储", "保存订单"
					}
					before := string(b1690JSON(t, doc))
					result, err := (&tool.EmitAnswerDocument{}).Execute(bus, b1690JSON(t, doc))
					if err != nil || result.Success || result.Repair == nil {
						t.Fatalf("actual first emit must publish local repair: err=%v result=%+v", err, result)
					}
					evaluator := &answerDocumentEvaluator{mu: bus.Mutable, maxRetries: 3}
					signal := evaluator.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, LastToolResult: &result})
					lease := bus.Mutable.AnswerDiagramRelationRepairLease()
					if !signal.HintRequested || signal.StopRequested || lease == nil || len(lease.AllowedAdditions) != 1 {
						t.Fatalf("real Observe must install source-owned lease: signal=%+v lease=%+v result=%+v", signal, lease, result)
					}
					candidate := lease.AllowedAdditions[0]
					if candidate.BlockID != "path" || candidate.EvidenceID != ev.ID || candidate.FromIdentity != ev.Subject || candidate.ToIdentity != ev.Object {
						t.Fatalf("repair changed selected source tuple: %+v", candidate)
					}
					if len(candidate.FromNodeIDs) != 0 || len(candidate.ToNodeIDs) != 0 {
						t.Errorf("list/table repair supplied syntax aliases as reader choices: %+v", candidate)
					}
					if strings.Contains(result.Summary, "stable local presentation ids") || strings.Contains(signal.Hint, mermaidcompat.CanonicalFlowchartNodeID(ev.Subject)) {
						t.Error("public repair teaching confuses reader labels with generated syntax carriers")
					}
					// The compact retry and shared handbook must not repeat the
					// same label-domain or current-schema capability instruction.
					for _, teaching := range []string{
						"from_node/to_node are business/code reader labels, not Mermaid ids",
						"visible_label describes the relation. Technical identities stay separate.",
						"Use `diagram_participant_edits` only when the current schema publishes it.",
					} {
						if count := strings.Count(signal.Hint, teaching); count != 1 {
							t.Errorf("public retry must teach %q once, got %d", teaching, count)
						}
					}
					branch := b1690AdditionBranch(t, (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx), candidate.AdditionRef)
					props := branch["properties"].(map[string]any)
					edgeProps := props["edge"].(map[string]any)["properties"].(map[string]any)
					for _, field := range []string{"from_node", "to_node", "visible_label"} {
						description, _ := edgeProps[field].(map[string]any)["description"].(string)
						if !strings.Contains(description, "reader") {
							t.Errorf("public %s schema lacks reader contract: %s", field, description)
						}
					}
					for _, field := range []string{"from_node_visible_label", "to_node_visible_label"} {
						if _, exposed := props[field]; exposed {
							t.Errorf("standalone schema publishes ignored display field %s", field)
						}
					}
					edits := []map[string]any{}
					for _, failure := range lease.Failures {
						if !failure.AllowsAction("remove") {
							t.Fatalf("expected precise remove choice: %+v", failure)
						}
						edits = append(edits, map[string]any{"action": "remove", "failure_ref": failure.FailureRef})
					}
					edits = append(edits, map[string]any{"action": "add", "addition_ref": candidate.AdditionRef,
						"edge": map[string]string{"from_node": fromLabel, "to_node": toLabel, "visible_label": relationLabel}})
					// Unknown refs never become authority merely because the target's
					// display domain is a list/table.
					stateBefore := string(b1690JSON(t, []any{bus.Mutable.AnswerDocumentV2(), bus.Mutable.LastRejectedAnswerDocumentV2(), bus.Mutable.AnswerDiagramRelationRepairLease()}))
					bad, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, b1690JSON(t, map[string]any{"diagram_edge_edits": []any{map[string]any{
						"action": "add", "addition_ref": candidate.AdditionRef + "-wrong-source", "edge": map[string]string{"from_node": fromLabel, "to_node": toLabel, "visible_label": relationLabel}}}}))
					if err != nil || bad.Success || stateBefore != string(b1690JSON(t, []any{bus.Mutable.AnswerDocumentV2(), bus.Mutable.LastRejectedAnswerDocumentV2(), bus.Mutable.AnswerDiagramRelationRepairLease()})) {
						t.Fatalf("wrong ref was accepted or altered state: err=%v result=%+v", err, bad)
					}
					patched, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, b1690JSON(t, map[string]any{"diagram_edge_edits": edits}))
					if err != nil || !patched.Success {
						t.Fatalf("model-selected label patch: err=%v result=%+v", err, patched)
					}
					got := bus.Mutable.AnswerDocumentV2()
					if got == nil || len(got.Blocks) != 2 || len(got.Blocks[1].EdgeAnchors) != 1 {
						t.Fatalf("published repair missing: %+v", got)
					}
					wantBlock := doc.Blocks[1]
					wantBlock.EdgeAnchors = got.Blocks[1].EdgeAnchors
					if !reflect.DeepEqual(wantBlock, got.Blocks[1]) || !reflect.DeepEqual(doc.Blocks[0], got.Blocks[0]) || before != string(b1690JSON(t, doc)) {
						t.Fatal("repair changed model body, sibling, or caller-owned input")
					}
					anchor := got.Blocks[1].EdgeAnchors[0]
					if anchor.FromIdentity != ev.Subject || anchor.ToIdentity != ev.Object || anchor.FromNode != fromLabel || anchor.ToNode != toLabel {
						t.Fatalf("label/identity conflation: %+v", anchor)
					}
					output := render.RenderAnswerDocument(got, language)
					if !strings.Contains(output, fromLabel+" → "+toLabel) || !strings.Contains(output, relationLabel) || strings.Contains(output, mermaidcompat.CanonicalFlowchartNodeID(ev.Subject)) {
						t.Fatalf("model labels did not reach reader: %s", output)
					}
				})
			}
		}
	}
}

func TestB1690ActualMixedDiagramRetainsAliasesAndAttach(t *testing.T) {
	for _, graphKind := range []types.DiagramKind{types.DiagramFlow, types.DiagramSequence} {
		t.Run(string(graphKind), func(t *testing.T) {
			bus, ctx, doc, ev := b1690StandaloneFixture(t, types.BlockBulletList, false)
			alias := mermaidcompat.CanonicalFlowchartNodeID(ev.Subject)
			body := "flowchart LR\n " + alias + "[\"sample.Caller\"]\n B[\"StoreSave\"]\n " + alias + " -->|model call| B\n"
			if graphKind == types.DiagramSequence {
				body = "sequenceDiagram\n participant " + alias + " as sample.Caller\n participant B as StoreSave\n " + alias + "->>B: model call\n"
			}
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "diagram", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: graphKind, Language: "mermaid", Body: body}})
			result, err := (&tool.EmitAnswerDocument{}).Execute(bus, b1690JSON(t, doc))
			if err != nil || result.Success {
				t.Fatalf("mixed source-backed repair: err=%v result=%+v", err, result)
			}
			// Existing quote/format normalization runs in the first emit. The
			// local metadata repair must preserve that exact live graph base.
			rejected := bus.Mutable.LastRejectedAnswerDocumentV2()
			if rejected == nil || len(rejected.Blocks) != 3 || rejected.Blocks[2].Diagram == nil {
				t.Fatal("mixed rejected graph base missing")
			}
			baseBody := rejected.Blocks[2].Diagram.Body
			e := &answerDocumentEvaluator{mu: bus.Mutable, maxRetries: 3}
			signal := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, LastToolResult: &result})
			lease := bus.Mutable.AnswerDiagramRelationRepairLease()
			if !signal.HintRequested || lease == nil {
				t.Fatalf("mixed Observe failed: %+v %+v", signal, result)
			}
			var edits []map[string]any
			seenList, seenDiagram := false, false
			schema := (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)
			for _, candidate := range lease.AllowedAdditions {
				switch candidate.BlockID {
				case "path":
					seenList = true
					if len(candidate.FromNodeIDs)+len(candidate.ToNodeIDs) != 0 {
						t.Fatalf("mixed list leaked diagram aliases: %+v", candidate)
					}
					b1690AdditionBranch(t, schema, candidate.AdditionRef)
					edits = append(edits, map[string]any{"action": "add", "addition_ref": candidate.AdditionRef,
						"edge": map[string]string{"from_node": "业务入口", "to_node": "订单存储", "visible_label": "保存订单"}})
				case "diagram":
					seenDiagram = true
					if !strings.Contains(strings.Join(candidate.FromNodeIDs, "|"), alias) {
						t.Fatalf("true diagram lost syntax alias: %+v", candidate)
					}
					var matches []types.AnswerDiagramRelationRepairFailure
					for _, failure := range lease.Failures {
						if types.AnswerDiagramRelationRepairFailureCanAttachCandidate(failure, candidate) {
							matches = append(matches, failure)
						}
					}
					if len(matches) != 1 || !matches[0].AllowsAction("attach") {
						t.Fatalf("true diagram lost exact attach: %+v %+v", matches, candidate)
					}
					edits = append(edits, map[string]any{"action": "attach", "failure_ref": matches[0].FailureRef, "addition_ref": candidate.AdditionRef,
						"edge": map[string]string{"from_node": alias, "to_node": "B", "visible_label": "model call"}})
				default:
					t.Fatalf("unexpected mixed candidate: %+v", candidate)
				}
			}
			if !seenList || !seenDiagram {
				t.Fatalf("vacuous mixed roster: %+v", lease)
			}
			patched, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, b1690JSON(t, map[string]any{"diagram_edge_edits": edits}))
			if err != nil || !patched.Success {
				t.Fatalf("mixed model patch: err=%v result=%+v", err, patched)
			}
			got := bus.Mutable.AnswerDocumentV2()
			if got == nil || len(got.Blocks) != 3 || got.Blocks[2].Diagram.Body != baseBody || len(got.Blocks[2].EdgeAnchors) != 1 ||
				got.Blocks[2].EdgeAnchors[0].FromNode != alias || got.Blocks[2].EdgeAnchors[0].FromIdentity != ev.Subject || !reflect.DeepEqual(got.Blocks[1].Items, doc.Blocks[1].Items) {
				if got != nil && len(got.Blocks) == 3 {
					t.Fatalf("mixed repair changed graph: body=%q base=%q anchors=%+v itemsEqual=%v", got.Blocks[2].Diagram.Body, baseBody, got.Blocks[2].EdgeAnchors, reflect.DeepEqual(got.Blocks[1].Items, doc.Blocks[1].Items))
				}
				t.Fatalf("mixed repair lost document: %+v", got)
			}
			if output := render.RenderAnswerDocument(got, "zh"); !strings.Contains(output, "业务入口 → 订单存储") || !strings.Contains(output, alias) {
				t.Fatalf("distinct graph/list display domains lost: %s", output)
			}
		})
	}
}

func TestB1690ActualWrongEvidenceStillRejects(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockOrderedList, types.BlockBulletList, types.BlockTable} {
		for _, wrong := range []string{"foreign_source_identity", "reverse"} {
			t.Run(string(kind)+"/"+wrong, func(t *testing.T) {
				bus, _, doc, ev := b1690StandaloneFixture(t, kind, false)
				from, to := "foreign.Caller", ev.Object
				if wrong == "reverse" {
					from, to = ev.Object, ev.Subject
				}
				doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "业务入口", ToNode: "订单存储", FromIdentity: from, ToIdentity: to,
					RelationKind: types.DiagramRelCall, VisibleLabel: "模型声称的调用"}}
				result, err := (&tool.EmitAnswerDocument{}).Execute(bus, b1690JSON(t, doc))
				if err != nil || result.Success || result.Repair == nil || bus.Mutable.AnswerDocumentV2() != nil {
					t.Fatalf("reader labels granted wrong evidence/direction: err=%v result=%+v", err, result)
				}
			})
		}
	}
}
