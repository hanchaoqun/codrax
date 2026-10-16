package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC250ResourceDefaultActualFinalizer(t *testing.T) {
	// Public EmitAnalysis, a restored no-attachment catalog and real queries
	// precede this finalizer. The default is display only, not a new request.
	ctx := resourceFactsContextHMC249(t)
	if ctx.AttachedHitrace != "" || ctx.AnalysisIR.RequestModel.PerfTrace != nil {
		t.Fatal("test acquired an attachment")
	}
	prompt := dependencyObservationMessages(t, ctx)
	if !strings.Contains(prompt, "Native resource fact choices") || !strings.Contains(prompt, "deterministic_runtime_queries=4") {
		t.Fatal("actual finalizer lost the current four resource queries")
	}
	before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts().ToolResults)
	const interpretation = "仅解释已有记录；没有记录不证明线程不存在，栈深度不是执行顺序。"
	raw, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "interpretation", "kind": "summary", "text": interpretation}}})
	out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
	if err != nil || !out.Success {
		t.Fatalf("public emit: %v %s", err, out.Summary)
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	count, rows, empty := 0, 0, 0
	for _, block := range doc.Blocks {
		if block.SystemGeneratedKind != types.AnswerSystemGeneratedNativeFacts {
			continue
		}
		if block.Kind != types.BlockTable || block.RuntimeMeasurement == nil || !block.RuntimeMeasurement.IsBound() || block.RuntimeMeasurement.View != types.RuntimeMeasurementSummary {
			t.Fatal("default is not an authenticated resource summary")
		}
		table := block.RuntimeMeasurement.BoundTable
		if !table.HasNativeResourceFacts() || table.MemberSet != nil || table.HasCompleteNativeDisplayPopulation() {
			t.Fatal("default summary acquired population authority")
		}
		count++
		rows += len(table.Rows)
		if len(table.Rows) == 0 {
			empty++ // Executed-empty is still a query, not a sixth event.
		}
	}
	if count != 4 || rows != 5 || empty != 1 {
		t.Fatalf("actual finalizer emission has %d summaries/%d event rows/%d empty queries, want 4/5/1", count, rows, empty)
	}
	if doc.Blocks[0].Text != interpretation {
		t.Fatal("native defaults rewrote independent interpretation")
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	frames := 0
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.HasNativeResourceFacts() && table.View == types.RuntimeMeasurementMembers {
			if table.DefaultPresentation {
				t.Fatal("full frame view unexpectedly became default")
			}
			frames += len(table.Rows)
		}
	}
	if frames != 14 {
		t.Fatalf("optional physical frame population changed: %d", frames)
	}
	after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts().ToolResults)
	if string(before) != string(after) {
		t.Fatal("default rendering changed original query evidence")
	}
	visible := render.RenderAnswerDocument(doc, "zh")
	for _, want := range []string{"FreeEvent", "无匹配记录", "线程名=gallery-main", "线程名=editor-main", "单位未在该载体声明", "不是函数执行顺序"} {
		if !strings.Contains(visible, want) {
			t.Errorf("default resource facts lost %q", want)
		}
	}
	wire, _ := json.Marshal(doc)
	var restored types.AnswerDocumentV2
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	ownership := types.CaptureSystemGeneratedBlockKinds(doc)
	if got := types.ReauthenticateSystemSnapshotBlockKinds(&restored, ownership); got != len(ownership) {
		t.Fatalf("private snapshot ownership restored %d of %d system blocks", got, len(ownership))
	}
	if !types.RebindRuntimeAnswerReceipts(&restored, view) || render.RenderAnswerDocument(&restored, "zh") != visible {
		t.Fatal("current snapshot rebind changed resource facts")
	}
	accepted := ctx.Mutable.AnswerDocumentV2()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
	ctx.Mutable.ResetDispatchToolResults()
	var stale types.AnswerDocumentV2
	_ = json.Unmarshal(wire, &stale)
	if types.RebindRuntimeAnswerReceipts(&stale, types.BuildAnswerSemanticViewForAgentContext(ctx)) {
		t.Fatal("JSON snapshot recreated missing native facts")
	}
	if !reflect.DeepEqual(ctx.Mutable.AnswerDocumentV2(), accepted) {
		t.Fatal("failed rebind replaced the accepted document")
	}
}
