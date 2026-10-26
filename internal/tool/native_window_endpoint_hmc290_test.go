package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Real native query -> preparation -> publication -> shared source title.
// Requested half-open display selection never rewrites the original query.
func TestHMC290NativeWindowEndpointPublic(t *testing.T) {
	for _, tc := range []struct {
		name              string
		explicit, project bool
	}{
		{"native_closed", false, false},
		{"explicit_half_open", true, false},
		{"closed_original_with_half_open_request_projection", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, work := t.TempDir(), t.TempDir()
			data, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "capture.data")
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			ctx := &types.BusContext{RepoRoot: root, WorkDir: work, Mutable: types.NewMutableState("native measurements"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")})}
			args := map[string]any{"source": "path", "path": path, "view": "resource_stack", "pid": 101}
			if tc.explicit {
				args["time_start"], args["time_end"] = 10.0, 10.05
			}
			raw, _ := json.Marshal(args)
			result, err := (&TraceQuery{}).Execute(ctx, raw)
			if err != nil || !result.Success {
				t.Fatalf("public query: %+v %v", result, err)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			start, end := 10.0, 10.05
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
				RuntimeTargets:         []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101}},
			}}
			if tc.project {
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "recorded target interval"}
			}
			before, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
			contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(ctx, 0))
			checked, distribution := 0, 0
			for _, record := range result.Observations {
				p, ok := DecodeTraceResourceStack(record)
				if !ok {
					continue
				}
				if p.Window.EndInclusive == tc.explicit || p.Window.StartTs != start || p.Window.EndTs != end {
					t.Fatalf("native query receipt changed: %+v", p.Window)
				}
				publication, valid := types.DecodeRuntimeMeasurementPublication(record)
				if !valid {
					t.Fatal("native resource publication absent")
				}
				for _, table := range contract.Choices() {
					if table.ObservationID != record.ID || !table.HasNativeResourceFacts() {
						continue
					}
					right := ")"
					if p.Window.EndInclusive {
						right = "]"
					}
					assertNativeWindowTitleHMC290(t, table, fmt.Sprintf("[%g,%g%s s", start, end, right))
					if table.View == types.RuntimeMeasurementDistribution {
						for _, original := range publication.Tables {
							if original.View == table.View && (!reflect.DeepEqual(table.Rows, original.Rows) || !reflect.DeepEqual(table.Columns, original.Columns)) {
								t.Fatal("display selection changed original statistics or denominators")
							}
						}
						distribution++
					}
					if table.View != types.RuntimeMeasurementSummary {
						continue
					}
					t.Logf("native_window=%+v projected=%v rows=%d original_matched=%d", p.Window, tc.project, len(table.Rows), p.MatchedEvents)
					wantRows, wantOriginal := 4, 4
					if tc.explicit {
						wantRows, wantOriginal = 3, 3
					} else if tc.project {
						wantRows = 3
					}
					if len(table.Rows) != wantRows || p.MatchedEvents != wantOriginal || tc.project && !strings.Contains(table.Label, "请求范围") {
						t.Fatalf("query/projection mixed: rows=%d original=%d label=%s", len(table.Rows), p.MatchedEvents, table.Label)
					}
					checked++
				}
			}
			if checked != 1 || distribution != 1 {
				t.Fatalf("expected one original summary/statistics population, got %d/%d", checked, distribution)
			}
			after, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
			if string(before) != string(after) {
				t.Fatal("display compilation changed original native evidence")
			}
		})
	}
}

func assertNativeWindowTitleHMC290(t *testing.T, table types.RuntimeMeasurementTable, want string) {
	t.Helper()
	basis, owned := table.NativeSourceTitleBasis()
	if !owned || basis.QueryWindow != want {
		t.Errorf("source title endpoint: owned=%t got=%q want=%q", owned, basis.QueryWindow, want)
	}
	for _, lang := range []string{"zh", "en"} {
		_, notes, valid := table.NativeSourcePresentation(lang)
		if !valid || !strings.Contains(strings.Join(notes, "\n"), want) {
			t.Errorf("%s native source notes lost actual query endpoint: valid=%t notes=%q want=%q", lang, valid, notes, want)
		}
	}
}

func TestHMC290SamplingWindowEndpointPublic(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit_%t", explicit), func(t *testing.T) {
			path, _ := samplingSourceBundleHMC241(t)
			dir := t.TempDir()
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampling facts"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, "prepared")})}
			args := map[string]any{"source": "path", "path": path, "view": "perf_stats", "pid": 11, "pattern": "dso=/app/A.so"}
			want := "[1.1,3] s"
			if explicit {
				args["time_start"], args["time_end"] = 1.1, 3.0
				want = "[1.1,3) s"
			}
			raw, _ := json.Marshal(args)
			result, err := (&TraceQuery{}).Execute(ctx, raw)
			if err != nil || !result.Success {
				t.Fatalf("sampling query: %v %s", err, result.Summary)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(ctx, 0))
			count := 0
			for _, table := range contract.Choices() {
				if table.HasNativeSamplingFacts() {
					assertNativeWindowTitleHMC290(t, table, want)
					count++
				}
			}
			if count != 2 {
				t.Fatalf("wanted independent sampling summary and quality: %d", count)
			}
		})
	}
}
