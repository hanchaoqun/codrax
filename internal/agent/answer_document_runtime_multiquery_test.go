package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeMultiQueryRecipesKeepDistinctEvents(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_dependencies/events.systrace")
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("请画出这些线程的唤醒关系"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", Intent: types.IntentExplain, PredicateAxis: types.AxisFlow}}}
	for _, args := range []map[string]any{
		{"view": "wakeup_chain", "pid": 200, "time_start": 5, "time_end": 5.041},
		{"view": "wakeup_chain", "pid": 100, "time_start": 5, "time_end": 5.041},
		{"view": "event_search", "event_types": []string{"sched_wakeup"}, "time_start": 5, "time_end": 5.041},
		{"view": "event_search", "event_types": []string{"sched_wakeup"}},
	} {
		args["path"] = path
		raw, _ := json.Marshal(args)
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !result.Success {
			t.Fatalf("query: %v %+v", err, result)
		}
		ctx.Mutable.AppendDispatchToolResult(result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	ledger := answerDocObservationLedger(ctx)
	authority := tool.RuntimeDiagramRelations(ledger, &ctx.AnalysisIR.RequestModel)
	if len(authority) <= 8 {
		t.Fatalf("fixture did not exceed old prompt budget: %d", len(authority))
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	var anchors []types.DiagramEdgeAnchor
	for _, line := range strings.Split(prompt, "\n") {
		if i := strings.Index(line, "edge_anchor="); i >= 0 {
			var anchor types.DiagramEdgeAnchor
			if err := json.Unmarshal([]byte(line[i+len("edge_anchor="):]), &anchor); err != nil {
				t.Fatal(err)
			}
			anchors = append(anchors, anchor)
		}
	}
	if len(anchors) != 4 {
		t.Fatalf("repeated queries hid distinct events: %d recipes", len(anchors))
	}
	nodes, seenEvents := map[string]string{}, map[string]bool{}
	var arrows strings.Builder
	for _, anchor := range anchors {
		found := false
		for _, row := range authority {
			if row.FromIdentity != anchor.FromIdentity || row.ToIdentity != anchor.ToIdentity {
				continue
			}
			found = true
			key := strings.Join(row.SupportRefs, ";")
			if seenEvents[key] {
				t.Fatal("same physical event repeated in the display")
			}
			seenEvents[key] = true
			nodes[row.FromNode], nodes[row.ToNode] = row.FromLabel, row.ToLabel
			fmt.Fprintf(&arrows, " %s->>%s: 唤醒\n", row.FromNode, row.ToNode)
		}
		if !found {
			t.Fatal("recipe created authority not in producer pool")
		}
	}
	if len(nodes) != 5 {
		t.Fatalf("query-coherent selection split the five thread lanes: %d", len(nodes))
	}
	var body strings.Builder
	body.WriteString("sequenceDiagram\n")
	for _, anchor := range anchors {
		for _, node := range []string{anchor.FromNode, anchor.ToNode} {
			if label, ok := nodes[node]; ok {
				fmt.Fprintf(&body, " participant %s as %s\n", node, label)
				delete(nodes, node)
			}
		}
	}
	body.WriteString(arrows.String())
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, TraceCausalClaimCaliber: "no_causal_conclusion", Text: "四次记录到的唤醒，不代表全部等待的根因。"},
		{ID: "diag", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: body.String()}, EdgeAnchors: anchors},
	}}
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	raw, _ := json.Marshal(doc)
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("multi-query public emit: %v %+v", err, result)
	}
	markdown := render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "zh")
	if strings.Count(markdown, "participant ") != 5 || strings.Count(markdown, "->>") != 4 || strings.Contains(render.RenderMermaidBlocks(markdown), "# ⚠") {
		t.Fatalf("accepted graph malformed: %s", markdown)
	}
	// Display de-duplication never transfers credentials between queries.
	originalAnchor := anchors[0]
	for _, row := range authority {
		if row.FromLabel == "dep3-300" && row.FromIdentity != anchors[0].FromIdentity {
			doc.Blocks[1].EdgeAnchors[0].FromIdentity = row.FromIdentity
			break
		}
	}
	raw, _ = json.Marshal(doc)
	result, err = (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success {
		t.Fatalf("cross-query endpoint borrowing accepted: %v %+v", err, result)
	}
	// Ask the actual repair path for candidates for an unanchored relation.
	// Alias-conflict failures above intentionally do not authorize additions.
	results := ctx.Mutable.DispatchToolResults()
	ctx.Mutable = types.NewMutableState("repair the unanchored relation")
	for _, r := range results {
		ctx.Mutable.AppendDispatchToolResult(r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	bus = types.ToolBusContext(ctx, types.AgentFinalizer)
	doc.Blocks[1].Diagram.Kind = types.DiagramFlow
	doc.Blocks[1].Diagram.Body = fmt.Sprintf("flowchart TD\n %s[\"dep3-300\"] -->|唤醒| %s[\"dep2-200\"]\n", originalAnchor.FromNode, originalAnchor.ToNode)
	doc.Blocks[1].EdgeAnchors = nil
	raw, _ = json.Marshal(doc)
	result, err = (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success {
		t.Fatalf("missing anchor did not require repair: %v %+v", err, result)
	}
	if !installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &result, false) {
		t.Fatal("actual finalizer could not install the public repair lease")
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if len(lease.AllowedAdditions) != 4 {
		t.Fatalf("repair budget was consumed by repeated query credentials: %+v", lease.AllowedAdditions)
	}
	var failure types.AnswerDiagramRelationRepairFailure
	var candidate types.AnswerDiagramRelationRepairCandidate
	for _, f := range lease.Failures {
		for _, c := range lease.AllowedAdditions {
			if c.FromIdentity == originalAnchor.FromIdentity && c.ToIdentity == originalAnchor.ToIdentity && types.AnswerDiagramRelationRepairFailureCanAttachCandidate(f, c) {
				failure, candidate = f, c
			}
		}
	}
	if candidate.AdditionRef == "" {
		t.Fatal("missing exact original event repair")
	}
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": []map[string]any{{"action": "attach", "failure_ref": failure.FailureRef, "addition_ref": candidate.AdditionRef, "edge": map[string]any{"from_node": failure.FromNode, "to_node": failure.ToNode, "visible_label": "唤醒"}}}})
	result, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("public multi-query repair: %v %+v", err, result)
	}
	if got := tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, ctx.Mutable.AnswerDocumentV2(), types.BuildAnswerSemanticViewForBusContext(bus), nil); len(got) != 0 {
		t.Fatalf("post gate: %+v", got)
	}
}
