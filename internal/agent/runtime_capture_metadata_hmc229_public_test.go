package agent

import (
	"bytes"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC229CaptureMetadataActualToolFinalizer(t *testing.T) {
	for _, mode := range []string{"single", "pair", "separate", "prepared_unexecuted"} {
		for _, ruler := range []string{"unbounded", "shared", "independent"} {
			t.Run(mode+"/"+ruler, func(t *testing.T) {
				dir := t.TempDir()
				var paths [2]string
				for i, name := range []string{"baseline", "current"} {
					raw, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_capture_metadata/" + name + ".data")
					if err != nil {
						t.Fatal(err)
					}
					paths[i] = filepath.Join(dir, name+".data")
					if err := os.WriteFile(paths[i], raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				start, end := 1.0, 2.0
				scope := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeUnspecified}
				if mode != "single" {
					scope.Comparison = &types.RuntimeArtifactComparisonScope{Baseline: types.RuntimeArtifactComparisonSide{Path: paths[0], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[0]}}, Current: types.RuntimeArtifactComparisonSide{Path: paths[1], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[1]}}}
				}
				if ruler == "shared" || mode == "single" && ruler == "independent" {
					scope.RequestedScope = types.RuntimeArtifactScopeExplicitWindow
					scope.TimeStart = &start
					scope.TimeEnd = &end
					scope.SourceQuote = "1到2秒"
				}
				if ruler == "independent" && scope.Comparison != nil {
					scope.Comparison.Baseline.TimeStart = &start
					scope.Comparison.Baseline.TimeEnd = &end
					scope.Comparison.Current.TimeStart = &start
					scope.Comparison.Current.TimeEnd = &end
				}
				rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{}, RuntimeArtifactScopeProfile: scope, RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}}}
				ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("两份采集的量测与设备记录"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}, TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
				ctx.Mutable.SetRequestModel(rm)
				if mode != "single" {
					catalog, err := tracecatalog.DiscoverIndependentPair(t.Context(), paths)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = ctx.Mutable.InstallTraceCatalog(catalog); err != nil {
						t.Fatal(err)
					}
				}
				query := func(path string) map[string]any {
					m := map[string]any{"source": "path", "path": path, "view": "measurements"}
					if ruler != "unbounded" {
						m["time_start"] = start
						m["time_end"] = end
					}
					return m
				}
				var results []types.ToolResult
				execute := func(args any) {
					raw, _ := json.Marshal(args)
					result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
					if err != nil || !result.Success {
						t.Fatalf("actual query: %v %s", err, result.Summary)
					}
					results = append(results, result)
				}
				if mode == "pair" {
					execute(map[string]any{"comparison": map[string]any{"baseline": query(paths[0]), "current": query(paths[1])}})
				} else {
					execute(query(paths[0]))
					if mode == "separate" {
						execute(query(paths[1]))
					}
					if mode == "prepared_unexecuted" {
						if _, err := ctx.TraceInputPreparer.Prepare(t.Context(), paths[1]); err != nil {
							t.Fatal(err)
						}
					}
				}
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
				before := hmc218CausalProjection(t, ctx)
				choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
				var status *types.RuntimeMeasurementTable
				for i := range choices {
					if strings.HasPrefix(choices[i].ObservationID, "trace_measurement_acquisition:") || strings.HasPrefix(choices[i].ObservationID, "trace_source_metadata:") {
						status = &choices[i]
					}
				}
				if status == nil {
					t.Fatalf("metadata status absent: %+v", choices)
				}
				encoded, _ := json.Marshal(status)
				for _, want := range []string{"physical_width", "1176", "meta", "workload_cmd"} {
					if !strings.Contains(string(encoded), want) {
						t.Errorf("metadata lost %s: %s", want, encoded)
					}
				}
				if mode != "single" {
					for _, want := range []string{"1080", "missing", "invalid", "负载未知"} {
						if !strings.Contains(string(encoded), want) {
							t.Errorf("current metadata lost %s: %s", want, encoded)
						}
					}
				}
				if mode == "prepared_unexecuted" && status.Rows[1][1] != "not_executed" {
					t.Fatalf("metadata claimed query: %+v", status.Rows)
				}
				messages := dependencyObservationMessages(t, ctx)
				previews := hmc218MeasurementMessagePreviews(t, messages)
				if _, ok := previews[status.ObservationID+"/"+string(status.View)]; !ok {
					t.Fatal("real finalizer context lost metadata")
				}
				raw, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "采集元数据仅作背景。"}, map[string]any{"id": "metadata", "kind": "table", "runtime_measurement": map[string]any{"observation_id": status.ObservationID, "view": status.View}}}})
				result, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
				if err != nil || !result.Success {
					t.Fatalf("finalizer emit: %v %s", err, result.Summary)
				}
				visible := html.UnescapeString(render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "zh"))
				if !strings.Contains(visible, "physical_width") || !strings.Contains(visible, "1176") {
					t.Fatalf("render lost producer metadata: %s", visible)
				}
				if !bytes.Equal(before, hmc218CausalProjection(t, ctx)) || ctx.Mutable.TraceRootCauseReport() != nil {
					t.Fatal("metadata changed causal authority")
				}
				ctx.Mutable.ResetTurnAArtifacts()
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
				for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
					if table.ObservationID == status.ObservationID {
						t.Fatal("new epoch revived old metadata")
					}
				}
			})
		}
	}
}
