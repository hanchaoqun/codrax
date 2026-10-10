package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This exercises the real prepare/query, accepted handoff and finalizer adapter.
// The two labels are call roles, not a claim of aligned clocks or populations.
func TestHMC220MeasurementPairActualFinalizer(t *testing.T) {
	for _, name := range []string{"both", "one_failed", "one_changed", "cancelled", "independent_units", "unavailable"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}}}
			ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("并排列出两份采集中的量测记录"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}},
				TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
			ctx.Mutable.SetRequestModel(rm)
			copySource := func(fixture, filename string) string {
				body, err := os.ReadFile("../../eval/fixtures/" + fixture)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, filename)
				if err := os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
				return path
			}
			left := copySource("hmosperf_measurements/capture.data", "baseline.data")
			right := copySource("hmosperf_measurements/capture.data", "current.data")
			currentView, currentEnd := "measurements", 1.8
			if name == "one_failed" {
				right += ".missing"
			}
			if name == "independent_units" {
				right = copySource("hmosperf_cpu_state_frequency/events.systrace", "current.systrace")
				currentView, currentEnd = "cpu_state_frequency", 1.04
			}
			if name == "unavailable" {
				right = copySource("hmosperf_cpu_state_frequency/events.systrace", "current.systrace")
			}
			args, _ := json.Marshal(map[string]any{"comparison": map[string]any{
				"baseline": map[string]any{"source": "path", "path": left, "view": "measurements", "time_start": 1, "time_end": 2},
				"current":  map[string]any{"source": "path", "path": right, "view": currentView, "time_start": 1, "time_end": currentEnd},
			}})
			bus := types.ToolBusContext(ctx, types.AgentExplorer)
			if name == "cancelled" {
				cancelled, cancel := context.WithCancel(context.Background())
				cancel()
				bus.Ctx = cancelled
			}
			result, err := (&tool.TraceQuery{}).Execute(bus, args)
			if err != nil || !result.Success {
				t.Fatalf("dual native query failed: %v / %s", err, result.Summary)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{types.AttachToolHandoffCarrier(result)}})
			if name == "one_changed" {
				if err := os.WriteFile(left, []byte("changed capture\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := hmc218CausalProjection(t, ctx)
			choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
			want := 7
			if name == "one_failed" || name == "one_changed" {
				want = 4
			}
			if name == "cancelled" {
				want = 1
			}
			if len(choices) != want {
				t.Fatalf("choices=%d want=%d", len(choices), want)
			}
			messages := dependencyObservationMessages(t, ctx)
			previews := hmc218MeasurementMessagePreviews(t, messages)
			var state *types.RuntimeMeasurementTable
			for i := range choices {
				table := choices[i]
				if strings.HasPrefix(table.ObservationID, "trace_measurement_pair:") {
					state = &choices[i]
				}
				if _, ok := previews[table.ObservationID+"/"+string(table.View)]; !ok {
					t.Errorf("finalizer missing %s/%s", table.ObservationID, table.View)
				}
			}
			if state == nil || len(state.Rows) != 2 {
				t.Fatalf("no complete two-side status table: %+v", state)
			}
			encoded, _ := json.Marshal(state)
			for _, exact := range []string{"baseline", "current", "设备未知", "负载未知", left, right} {
				if !strings.Contains(string(encoded), exact) {
					t.Errorf("pair status omitted %q: %s", exact, encoded)
				}
			}
			if name == "both" {
				encoded, _ = json.Marshal(choices)
				for _, exact := range []string{"9007199254740993", "未知（NULL）", "0 (integer)", "3.342000005e+08"} {
					if !strings.Contains(string(encoded), exact) {
						t.Errorf("native value lost %q", exact)
					}
				}
			}
			status := map[string]string{"one_failed": "failed", "one_changed": "stale", "cancelled": "cancelled", "unavailable": "unavailable"}[name]
			if status != "" && !strings.Contains(string(encoded), status) {
				t.Errorf("missing independent status %s", status)
			}
			if name == "unavailable" && (state.Rows[0][1] != "available" || state.Rows[1][1] != "unavailable") {
				t.Errorf("missing measurements were promoted to available: %+v", state.Rows)
			}
			if !bytes.Equal(before, hmc218CausalProjection(t, ctx)) || ctx.Mutable.TraceRootCauseReport() != nil {
				t.Fatal("pair display changed causal authority")
			}
		})
	}
}
