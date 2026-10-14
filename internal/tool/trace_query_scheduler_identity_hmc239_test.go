package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func schedulerIdentityContext239(t *testing.T) (*types.BusContext, types.ToolResult, types.ObservationRecord) {
	t.Helper()
	dir := t.TempDir()
	data := "record-11 (10) [000] .... 1.001000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000\n" +
		"record-11 (10) [000] .... 1.002000: sched_switch: prev_comm=record prev_pid=11 prev_prio=120 prev_state=S ==> next_comm=target next_pid=42 next_prio=120\n"
	if err := os.WriteFile(filepath.Join(dir, "events.systrace"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("List the recorded scheduler threads")}
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{}}
	ctx.Mutable.SetRequestModel(rm)
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}
	result, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"source":"path","path":"events.systrace","view":"event_search","time_start":1.0,"time_end":1.01}`))
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return ctx, result, requireEventSearchInventory(t, result)
}

func TestSchedulerIdentityPublicNativeSurfaces239(t *testing.T) {
	ctx, result, record := schedulerIdentityContext239(t)
	inv := record.EventSearchInventory
	if inv.Coverage.MatchedTotal != 2 || len(inv.Rows) != 2 || !inv.RowsComplete {
		t.Fatalf("endpoint display changed event population: %+v", inv)
	}
	for _, want := range []string{"唤醒目标线程编号（TID）=42", "唤醒目标线程所属进程编号（TGID）=未知", "切出线程编号（TID）=11", "切出线程所属进程编号（TGID）=10", "切入线程编号（TID）=42"} {
		if !strings.Contains(result.Summary, want) {
			t.Errorf("native tool summary lost %q", want)
		}
	}
	fields := []string{"emitter.tid", "emitter.tgid", "scheduler.wakee.comm", "scheduler.wakee.tid", "scheduler.wakee.tgid"}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	view.RequiredBlocks = []types.BlockRequirement{{Kind: types.BlockSummary, Required: true}}
	view.OptionalBlocks = []types.BlockRequirement{{Kind: types.BlockTable}, {Kind: types.BlockDiagram}}
	view.DiagramPlan = &types.DiagramFacetGraph{Kind: types.DiagramFlow}
	var commonRows [][]string
	for _, kind := range []string{"summary", "table", "diagram"} {
		wire, _ := json.Marshal(eventBlock236("facts", kind, record.ID, map[string]any{"row_ids": []string{"event-1"}, "field_keys": fields}))
		var input emitAnswerBlockV2
		if err := json.Unmarshal(wire, &input); err != nil {
			t.Fatal(err)
		}
		block, err := NormalizeEmitAnswerBlock(input, "blocks[0]")
		if err != nil {
			t.Fatal(err)
		}
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block}}
		if err := bindRuntimeMeasurementReceipts(doc, view); err != nil {
			t.Fatalf("%s native selector: %v", kind, err)
		}
		bound := doc.Blocks[0].RuntimeMeasurement.BoundTable
		if len(bound.Rows) != 1 || !reflect.DeepEqual(bound.Rows[0][:4], []string{"11", "10", `"target"`, "42"}) || !strings.Contains(bound.Rows[0][4], "未知") {
			t.Fatalf("%s thread/process fields confused: %+v", kind, bound)
		}
		if commonRows != nil && !reflect.DeepEqual(commonRows, bound.Rows) {
			t.Fatal("surface changed native values")
		}
		commonRows = bound.Rows
		visible := render.RenderAnswerDocument(doc, "zh")
		for _, want := range []string{"唤醒目标线程编号（TID）", "唤醒目标线程所属进程编号（TGID）", "target", "42"} {
			if !strings.Contains(visible, want) {
				t.Fatalf("%s did not render %q: %s", kind, want, visible)
			}
		}
	}
	// Exercise the public emit route too, not only binding/render helpers.
	input := map[string]any{"blocks": []any{eventBlock236("facts", "summary", record.ID, map[string]any{"row_ids": []string{"event-1"}, "field_keys": fields})}}
	if result := b1659bExecuteAnswer(t, ctx, input, false); !result.Success {
		t.Fatal(result.Summary)
	}
}
