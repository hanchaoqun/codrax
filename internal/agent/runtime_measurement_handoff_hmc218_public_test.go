package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC218RuntimeMeasurementActualFinalizerMessages(t *testing.T) {
	for _, tc := range []struct {
		name, fixture, view string
		start, end          float64
		copies              int
	}{
		{"small_generic", "hmosperf_measurements/capture.data", "measurements", 1, 2, 1},
		{"small_cpu", "hmosperf_cpu_state_frequency/events.systrace", "cpu_state_frequency", 1, 1.04, 1},
		{"spare_budget_two_captures", "hmosperf_measurements/capture.data", "measurements", 1, 2, 2},
		{"bounded_budget_five_captures", "hmosperf_measurements/capture.data", "measurements", 1, 2, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
				RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &tc.start, TimeEnd: &tc.end, Confidence: 1}}
			ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("这些数据序列的原始值和时间区间是什么？"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}},
				TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
			ctx.Mutable.SetRequestModel(rm)
			original, err := os.ReadFile("../../eval/fixtures/" + tc.fixture)
			if err != nil {
				t.Fatal(err)
			}
			var results []types.ToolResult
			for i := 0; i < tc.copies; i++ {
				path := filepath.Join(dir, fmt.Sprintf("capture-%d%s", i, filepath.Ext(tc.fixture)))
				if err := os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
				args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": tc.view, "time_start": tc.start, "time_end": tc.end})
				result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
				if err != nil || !result.Success {
					t.Fatalf("public prepare/query: %v / %s", err, result.Summary)
				}
				results = append(results, types.AttachToolHandoffCarrier(result))
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results,
				HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
			choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
			if len(choices) != 3*tc.copies {
				t.Fatalf("native measurement roster=%d, want %d", len(choices), 3*tc.copies)
			}
			before := hmc218CausalProjection(t, ctx)
			messages := dependencyObservationMessages(t, ctx)
			previews := hmc218MeasurementMessagePreviews(t, messages)
			total, shown := 0, 0
			for _, table := range choices {
				total += len(table.Rows)
				key := table.ObservationID + "/" + string(table.View)
				preview, ok := previews[key]
				if !ok {
					t.Errorf("actual finalizer omitted selector preview %s", key)
					continue
				}
				shown += len(preview.Rows)
				if !reflect.DeepEqual(preview.Columns, table.Columns) || len(preview.Rows) > len(table.Rows) || !reflect.DeepEqual(preview.Rows, table.Rows[:len(preview.Rows)]) {
					t.Errorf("source-bound native measurement cells changed: %s", key)
				}
				if preview.OutputRows != len(table.Rows) || preview.OmittedRows != len(table.Rows)-len(preview.Rows) {
					t.Errorf("preview/output completeness conflated: %s (%+v)", key, preview)
				}
			}
			want := total
			if want > 128 {
				want = 128
			}
			if shown != want {
				t.Errorf("actual finalizer preview used %d rows for %d available rows, want %d within shared budget", shown, total, want)
			}
			if total <= 128 {
				for _, table := range choices {
					preview := previews[table.ObservationID+"/"+string(table.View)]
					if !reflect.DeepEqual(preview.Rows, table.Rows) {
						t.Errorf("small complete set was prefix-clipped despite spare budget: %s/%s got=%d native=%d", table.ObservationID, table.View, len(preview.Rows), len(table.Rows))
					}
				}
			}
			if tc.name == "small_generic" {
				encoded, _ := json.Marshal(choices)
				for _, exact := range []string{"9007199254740993", "未知（NULL）", "3.342000005e+08"} {
					if !strings.Contains(string(encoded), exact) {
						t.Fatalf("generic fixture oracle missing %q", exact)
					}
				}
				for _, table := range choices {
					if table.MemberSet != nil {
						t.Fatal("generic raw measurement display acquired population authority")
					}
				}
			}
			if !bytes.Equal(before, hmc218CausalProjection(t, ctx)) || ctx.Mutable.TraceRootCauseReport() != nil {
				t.Fatal("measurement message rendering changed causal authority")
			}
		})
	}
}

type hmc218MeasurementPreview struct {
	Columns     []string   `json:"columns"`
	Rows        [][]string `json:"rows"`
	OutputRows  int
	OmittedRows int
}

func hmc218MeasurementMessagePreviews(t *testing.T, messages string) map[string]hmc218MeasurementPreview {
	t.Helper()
	// These are producer selector/data fields, not a required prose heading,
	// sentence or answer layout. Match all actual adapter messages.
	selectors := regexp.MustCompile(`observation_id=("(?:[^"\\]|\\.)*") view=("(?:[^"\\]|\\.)*")`)
	counts := regexp.MustCompile(`output_rows=([0-9]+); preview_omitted_rows=([0-9]+); preview=`)
	out := make(map[string]hmc218MeasurementPreview)
	for _, line := range strings.Split(messages, "\n") {
		match, count := selectors.FindStringSubmatch(line), counts.FindStringSubmatchIndex(line)
		if len(match) == 0 || len(count) == 0 {
			continue
		}
		id, err := strconv.Unquote(match[1])
		if err != nil {
			t.Fatal(err)
		}
		view, err := strconv.Unquote(match[2])
		if err != nil {
			t.Fatal(err)
		}
		var preview hmc218MeasurementPreview
		if err := json.Unmarshal([]byte(line[count[1]:]), &preview); err != nil {
			t.Fatalf("invalid native measurement preview JSON: %v", err)
		}
		preview.OutputRows, _ = strconv.Atoi(line[count[2]:count[3]])
		preview.OmittedRows, _ = strconv.Atoi(line[count[4]:count[5]])
		key := id + "/" + view
		if previous, ok := out[key]; ok && !reflect.DeepEqual(previous, preview) {
			t.Errorf("actual adapter contains inconsistent previews for %s", key)
		}
		out[key] = preview
	}
	return out
}
