package agent

import (
	"bytes"
	"encoding/json"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPreferredFrameRateActualFinalizerEmitPatch(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_preferred_frame_rate/capture.data")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	start, end := 1.0, 2.0
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{}, RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}}, RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到2秒的期望帧率", Confidence: 1}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("1到2秒的期望帧率"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}, TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	ctx.Mutable.SetRequestModel(rm)
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "preferred_frame_rate", "time_start": start, "time_end": end})
	out, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !out.Success {
		t.Fatalf("default SQLite query: %v %s", err, out.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{out}})
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	if len(choices) != 3 {
		t.Fatalf("wrong tables: %+v", choices)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"runtime_measurement", "render_service", "app.video", "119.88", "500000000", "100000000", "200000000", "filter 11", "不代表屏幕实际刷新"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual finalizer lost %q", want)
		}
	}
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	projection := func() []byte {
		v, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))))
		return v
	}
	originalProjection := projection()
	for _, table := range choices {
		if table.MemberSet != nil {
			t.Fatal("rate table got member completion authority")
		}
		block := map[string]any{"id": "rates", "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
		raw, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "explain", "kind": "summary", "text": "这是期望帧率观测，实际刷新仍需额外证据。"}, block}})
		got, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
		if err != nil || !got.Success {
			t.Fatalf("emit: %v %s", err, got.Summary)
		}
		patch, _ := json.Marshal(map[string]any{"replace_blocks": []any{block}, "unchanged_block_ids": []string{"explain"}})
		got, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, patch)
		if err != nil || !got.Success {
			t.Fatalf("patch: %v %s", err, got.Summary)
		}
		doc := ctx.Mutable.AnswerDocumentV2()
		matched := false
		for _, b := range doc.Blocks {
			if b.ID == "rates" {
				matched = b.RuntimeMeasurement != nil && b.RuntimeMeasurement.IsBound() && reflect.DeepEqual(*b.RuntimeMeasurement.BoundTable, table)
			}
		}
		if !matched {
			t.Fatal("native receipt changed")
		}
		visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
		for _, row := range table.Rows {
			if strings.Count(visible, "| "+strings.Join(row, " | ")+" |") != 1 {
				t.Fatalf("lost or duplicated row %v", row)
			}
		}
	}
	if !bytes.Equal(originalProjection, projection()) {
		t.Fatal("display changed causal projection")
	}
	for _, r := range out.Observations {
		if r.Predicate != tool.TracePreferredFrameRatePredicate {
			continue
		}
		for _, mutate := range []func(*types.ObservationRecord){func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" }, func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = 999 }, func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs = 3 }, func(r *types.ObservationRecord) { r.Producer = "model" }, func(r *types.ObservationRecord) {
			r.RichNotes = append(append([]string{}, r.RichNotes...), r.RichNotes[0])
		}} {
			bad := r
			mutate(&bad)
			if _, ok := types.DecodeRuntimeMeasurementPublication(bad); ok {
				t.Fatal("rebound receipt accepted")
			}
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("input database changed")
	}
}
