package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBusinessTreeInitialRecipePublicRejectInstallPatchPost(t *testing.T) {
	ctx := businessTreeFinalizerContext(t)
	ctx.AnalysisIR.RequestModel.Intent = types.IntentExplain
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisFlow
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}, SourceQuote: "business nesting"}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
	if len(rows) != 2 {
		t.Fatalf("missing relations: %+v", rows)
	}
	row := rows[0]
	if !strings.Contains(prompt, "edge_anchor=") || !strings.Contains(prompt, row.FromIdentity) || !strings.Contains(prompt, row.ToNode) {
		t.Fatal("public initial instruction omitted exact copyable relation recipe")
	}
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	projectionBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx)))
	body := fmt.Sprintf("flowchart TD\n %s[%q] -->|包含| %s[%q]\n", row.FromNode, row.FromLabel, row.ToNode, row.ToLabel)
	raw, _ := json.Marshal(map[string]any{"blocks": []map[string]any{
		{"id": "summary", "kind": "summary", "text": "同步业务打点显示直接父子层级，耗时不应重复相加。"},
		{"id": "diag", "kind": "diagram", "diagram": map[string]any{"kind": "flow", "language": "mermaid", "body": body}},
	}})
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success {
		t.Fatalf("missing anchor must reject: %v %+v", err, result)
	}
	if !installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &result, false) {
		t.Fatalf("real evaluator rejected the produced repair delta: %+v", result)
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	var f types.AnswerDiagramRelationRepairFailure
	var c types.AnswerDiagramRelationRepairCandidate
	for _, failure := range lease.Failures {
		for _, candidate := range lease.AllowedAdditions {
			if types.AnswerDiagramRelationRepairFailureCanAttachCandidate(failure, candidate) {
				f, c = failure, candidate
			}
		}
	}
	if c.AdditionRef == "" {
		t.Fatalf("no executable attach: %+v", lease)
	}
	before := ctx.Mutable.LastRejectedAnswerDocumentV2()
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": []map[string]any{{"action": "attach", "failure_ref": f.FailureRef, "addition_ref": c.AdditionRef, "edge": map[string]any{"from_node": f.FromNode, "to_node": f.ToNode, "visible_label": "包含"}}}})
	result, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("public patch rejected: %v %+v", err, result)
	}
	after := ctx.Mutable.AnswerDocumentV2()
	if !reflect.DeepEqual(before.Blocks[0], after.Blocks[0]) {
		t.Fatal("local repair changed numeric/prose sibling")
	}
	if got := tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, after, types.BuildAnswerSemanticViewForBusContext(bus), nil); len(got) > 0 {
		t.Fatalf("post validator disagrees: %+v", got)
	}
	projectionAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx)))
	if string(projectionBefore) != string(projectionAfter) {
		t.Fatal("display containment changed causal projection")
	}
	// Exercise the accepted answer's actual reader-facing surfaces, not just
	// its typed admission. The terminal renderer must produce a graph (not
	// raw syntax with a warning); HTML must keep its browser Mermaid carrier.
	markdown := render.RenderAnswerDocument(after, "zh")
	if !strings.Contains(markdown, "```mermaid") || strings.Contains(markdown, "# ⚠") {
		t.Fatalf("accepted diagram disappeared from Markdown: %s", markdown)
	}
	ascii := render.RenderMermaidBlocks(markdown)
	if !strings.Contains(ascii, "```text") || strings.Contains(ascii, "# ⚠") || strings.Contains(ascii, "flowchart TD") || strings.Contains(ascii, row.FromNode) {
		t.Fatalf("Mermaid did not render as a business graph: %s", ascii)
	}
	html, err := outputdump.BuildHTML("业务层级", markdown)
	if err != nil || !strings.Contains(html, `<div class="mermaid">`) {
		t.Fatalf("browser graph carrier missing: %v", err)
	}
	for _, label := range []string{row.FromLabel, row.ToLabel} {
		if !strings.Contains(markdown, label) || !strings.Contains(ascii, label) || !strings.Contains(html, label) {
			t.Fatalf("business label %q lost from rendered graph", label)
		}
	}
}
