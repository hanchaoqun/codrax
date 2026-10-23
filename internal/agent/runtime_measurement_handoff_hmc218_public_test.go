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
			metadataRanges := map[string][3]string{}
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
				if filepath.Ext(tc.fixture) == ".data" {
					metadataRanges[path] = [3]string{"0", "3000000000", "3000000000"}
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results,
				HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
			choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
			measurements, _ := splitSourceMetadataHMC287(t, choices, metadataRanges)
			if len(measurements) != 3*tc.copies || len(choices) != 3*tc.copies+len(metadataRanges) {
				t.Fatalf("native measurement roster=%d, metadata=%d, want measurements=%d", len(measurements), len(metadataRanges), 3*tc.copies)
			}
			before := hmc218CausalProjection(t, ctx)
			messages := dependencyObservationMessages(t, ctx)
			previews := hmc218MeasurementMessagePreviews(t, messages)
			total, shown := 0, 0
			partialTimeline := false
			// All selectors, including independent source metadata, share the
			// same 128-row preview budget; never filter metadata out here.
			for _, table := range choices {
				total += len(table.Rows)
				key := table.ObservationID + "/" + string(table.View)
				preview, ok := previews[key]
				if !ok {
					t.Errorf("actual finalizer omitted selector preview %s", key)
					continue
				}
				shown += len(preview.Rows)
				if !reflect.DeepEqual(preview.Columns, table.Columns) || len(preview.Rows) > len(table.Rows) {
					t.Errorf("source-bound native measurement cells changed: %s", key)
					continue
				}
				if table.View != types.RuntimeMeasurementTimeline {
					// Non-temporal tables retain the original prefix contract.
					if !reflect.DeepEqual(preview.Rows, table.Rows[:len(preview.Rows)]) || len(preview.SourceRowIndices) != 0 || preview.RowIndexBase != nil || preview.Selection != "" || preview.SeriesBoundaryCoverage != "" {
						t.Errorf("non-timeline source prefix or metadata changed: %s", key)
					}
				} else {
					// Independently check the published source coordinates, not
					// the renderer helper's own output as its expected oracle.
					if preview.RowIndexBase == nil || *preview.RowIndexBase != 0 || len(preview.SourceRowIndices) != len(preview.Rows) {
						t.Errorf("timeline lacks exact source row coordinates: %s (%+v)", key, preview)
						continue
					}
					partial := len(preview.Rows) < len(table.Rows)
					partialTimeline = partialTimeline || partial
					selection, coverage := "all_retained_rows", ""
					if partial {
						selection, coverage = "table_boundary_sample", "not_claimed"
						if len(preview.Rows) == 0 {
							selection = "no_rows"
						} else if len(preview.Rows) == 1 {
							selection = "first_retained_row"
						}
					}
					if preview.Selection != selection || preview.SeriesBoundaryCoverage != coverage {
						t.Errorf("timeline preview overstates retained/series coverage: %s (%+v)", key, preview)
					}
					for row, index := range preview.SourceRowIndices {
						want := row
						if partial && len(preview.Rows) > 1 && row == len(preview.Rows)-1 {
							want = len(table.Rows) - 1
						}
						if index != want || index < 0 || index >= len(table.Rows) || row > 0 && index <= preview.SourceRowIndices[row-1] {
							t.Errorf("timeline boundary/order changed: %s row=%d index=%d want=%d", key, row, index, want)
							continue
						}
						if !reflect.DeepEqual(preview.Rows[row], table.Rows[index]) {
							t.Errorf("source-bound timeline cells changed: %s row=%d source_index=%d", key, row, index)
						}
					}
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
			if tc.name == "bounded_budget_five_captures" && !partialTimeline {
				t.Fatal("bounded public fixture did not exercise a partial source-indexed timeline")
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
	Columns                []string   `json:"columns"`
	Rows                   [][]string `json:"rows"`
	SourceRowIndices       []int      `json:"source_row_indices"`
	RowIndexBase           *int       `json:"row_index_base"`
	Selection              string     `json:"selection"`
	SeriesBoundaryCoverage string     `json:"series_boundary_coverage"`
	OutputRows             int
	OmittedRows            int
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
