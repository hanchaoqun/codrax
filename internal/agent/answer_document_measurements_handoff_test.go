package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Default preparation and the real finalizer share native data. Raw values
// are displayable without becoming a GPU protocol, source obligation or cause.
func TestMeasurementsPublicFinalizerEmitPatchRender(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_measurements/capture.data")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	start, end := 1.0, 2.0
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到2秒的量测记录", Confidence: 1}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("1到2秒的量测记录"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}},
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	ctx.Mutable.SetRequestModel(rm)
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": start, "time_end": end})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("default prepare/query: %v / %s", err, result.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	if len(choices) != 3 {
		t.Fatalf("expected raw summary/members/timeline: %+v", choices)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"runtime_measurement", "system fills", "gpufreq", "gpu_state", "gpuload"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual finalizer lost %q", want)
		}
	}
	if strings.Contains(prompt, "999000000") {
		t.Fatal("right-boundary record leaked into finalizer")
	}
	// Prompt previews are intentionally bounded. The native selectable tables
	// must still retain every row, including series outside the four-row preview.
	encodedTables, _ := json.Marshal(choices)
	for _, want := range []string{"3.342000005e+08", "9007199254740993", "未知（NULL）"} {
		if !strings.Contains(string(encodedTables), want) {
			t.Errorf("complete native handoff lost %q", want)
		}
	}
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	projection := func() []byte {
		b, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))))
		return b
	}
	beforeProjection := projection()
	for _, table := range choices {
		if table.MemberSet != nil {
			t.Fatal("raw display acquired complete-population authority")
		}
		if table.View == types.RuntimeMeasurementMembers || table.View == types.RuntimeMeasurementTimeline {
			if len(table.Rows) != 13 {
				t.Fatalf("native window rows=%d, want 13", len(table.Rows))
			}
		}
		for _, patch := range []bool{false, true} {
			block := map[string]any{"id": "raw", "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
			input := map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "原始量测尚不能直接确认GPU实际运行频段。"}, block}}
			schema := (&tool.EmitAnswerDocument{}).ParametersFor(ctx)
			if patch {
				input = map[string]any{"replace_blocks": []any{block}, "unchanged_block_ids": []string{"lead"}}
				schema = (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)
			}
			raw, _ := json.Marshal(input)
			if err := toolparam.Validate(raw, schema); err != nil {
				t.Fatalf("native selector schema: %v", err)
			}
			var out types.ToolResult
			if patch {
				out, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
			} else {
				out, err = (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			}
			if err != nil || !out.Success {
				t.Fatalf("emit/patch native table: %v / %s", err, out.Summary)
			}
			doc := ctx.Mutable.AnswerDocumentV2()
			if len(doc.Blocks) != 2 || doc.Blocks[1].RuntimeMeasurement == nil || !reflect.DeepEqual(doc.Blocks[1].RuntimeMeasurement.BoundTable, &table) {
				t.Fatal("native data changed at emit/patch")
			}
			visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
			for _, row := range table.Rows {
				if strings.Count(visible, "| "+strings.Join(row, " | ")+" |") != 1 {
					t.Fatalf("row not displayed exactly once: %v", row)
				}
			}
			for _, note := range table.Notes {
				if !strings.Contains(visible, note) {
					t.Fatalf("unknown/scope disclosure lost: %q", note)
				}
			}
		}
	}
	if !bytes.Equal(beforeProjection, projection()) || ctx.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("raw measurement presentation acquired causal authority")
	}
	endNarrow := 1.5
	rm.RuntimeArtifactScopeProfile.TimeEnd = &endNarrow
	ctx.AnalysisIR.RequestModel = rm
	ctx.Mutable.SetRequestModel(rm)
	if c := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract; c != nil && len(c.Choices()) != 0 {
		t.Fatal("wider source data relabelled as narrow request window")
	}
	accepted := ctx.Mutable.AnswerDocumentV2()
	bad, _ := json.Marshal(map[string]any{"replace_blocks": []any{map[string]any{"id": "raw", "kind": "table", "runtime_measurement": map[string]any{"observation_id": choices[0].ObservationID, "view": choices[0].View}}}, "unchanged_block_ids": []string{"lead"}})
	for _, clear := range []bool{false, true} {
		if clear {
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
			ctx.Mutable.ResetDispatchToolResults()
		}
		out, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, bad)
		if err != nil || out.Success || !reflect.DeepEqual(accepted, ctx.Mutable.AnswerDocumentV2()) {
			t.Fatalf("out-of-scope/missing receipt modified answer: %v / %s", err, out.Summary)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(original) != sha256.Sum256(after) {
		t.Fatal("read-only measurement query modified database")
	}
}
