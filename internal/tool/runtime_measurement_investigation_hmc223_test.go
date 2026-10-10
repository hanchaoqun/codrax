package tool

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc223NativeInvestigation(t *testing.T, pair bool) (*types.BusContext, types.ToolResult, string) {
	t.Helper()
	bus, _, _ := hmc17NamedPathContext(t)
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_measurements/baseline.data")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bus.RepoRoot, "records.data")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": 2}
	if pair {
		args = map[string]any{"comparison": map[string]any{"baseline": args}}
	}
	result := hmc17NamedQuery(t, bus, args)
	if !result.Success {
		t.Fatal(result.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return bus, result, path
}

func TestHMC223NativeInvestigationPreparedSourceCannotDowngradeToDerivedReplay(t *testing.T) {
	integer := func(value string) tracewire.MeasureScalar {
		return tracewire.MeasureScalar{StorageClass: "integer", Value: value}
	}
	null := tracewire.MeasureScalar{StorageClass: "null"}
	line, err := tracewire.FormatMeasureInterval(tracewire.MeasureInterval{RowID: 1, StartNS: integer("1050000000"), DurationNS: integer("100000000"), Value: integer("0"), FilterID: null, MeasureType: null, FilterStatus: "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	hmc223AssertPreparedSourceCannotDowngrade(t, "# tracer: nop\n"+line+"\n", true)
}

func TestHMC223NativeInvestigationPreparedUnavailableKeepsSourceWithoutNavigation(t *testing.T) {
	hmc223AssertPreparedSourceCannotDowngrade(t, "# tracer: nop\nworker-7 (7) [000] .... 1.050000: sched_switch: prev_comm=worker prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n", false)
}

func hmc223AssertPreparedSourceCannotDowngrade(t *testing.T, body string, wantNavigation bool) {
	t.Helper()
	bus, _, _ := hmc17NamedPathContext(t)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bus.RepoRoot, "capture.data")
	if err := os.WriteFile(path, compressed.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": 2})
	if !result.Success {
		t.Fatal(result.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	if _, _, valid := bus.Mutable.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay); !valid {
		t.Fatal("test requires a live derived-file replay")
	}
	if _, _, _, valid := bus.Mutable.ResolveTraceIntervalNavigation(context.Background(), result.TraceQueryWindowReplay); valid != wantNavigation {
		t.Fatalf("native view navigation=%v want %v; source identity must not invent a view", valid, wantNavigation)
	}
	if !hmc223HasNativeInvestigation(bus) {
		t.Fatal("fresh prepared native records rejected")
	}
	replacement := path + ".replacement"
	if err := os.WriteFile(replacement, compressed.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if _, _, valid := bus.Mutable.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay); !valid {
		t.Fatal("derived file must remain unchanged to expose the stronger original-input boundary")
	}
	if hmc223HasNativeInvestigation(bus) {
		t.Fatal("replaced original input downgraded to stale derived-file authority")
	}
}

func hmc223HasNativeInvestigation(bus *types.BusContext) bool {
	return types.HasCurrentNativeMeasurementInvestigation(types.ObservationLedgerInputFromBusContext(bus, 0))
}

// The witness starts at the actual native producer. No native publication or
// source receipt is fabricated, and both the Bus and Agent adapters preserve
// the same consumer epoch. Zero-valued rows are part of this real fixture.
func TestHMC223NativeInvestigationCurrentSingleAndPair(t *testing.T) {
	for _, pair := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "pair"}[pair], func(t *testing.T) {
			bus, result, _ := hmc223NativeInvestigation(t, pair)
			if !hmc223HasNativeInvestigation(bus) {
				path, _, current := bus.Mutable.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay)
				var sources []types.ObservationSourceRef
				for _, record := range result.Observations {
					sources = append(sources, record.SourceRef)
				}
				t.Fatalf("current native records classified as empty: replay=%q current=%v sources=%+v", path, current, sources)
			}
			ctx := &types.AgentContext{RepoRoot: bus.RepoRoot, Mutable: bus.Mutable}
			if !types.HasCurrentNativeMeasurementInvestigation(types.ObservationLedgerInputFromAgentContext(ctx, 0)) {
				t.Fatal("agent handoff lost native records")
			}
			bus.Mutable = bus.Mutable.ForkForExploreDispatch()
			if !hmc223HasNativeInvestigation(bus) {
				t.Fatal("same-run fork lost accepted native records")
			}
			bus.Mutable = types.NewMutableState("other consumer")
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			if hmc223HasNativeInvestigation(bus) {
				t.Fatal("foreign consumer inherited native investigation authority")
			}
		})
	}
}

func TestHMC223NativeInvestigationRejectsStaleAndWrongScope(t *testing.T) {
	for _, pair := range []bool{false, true} {
		for _, mutation := range []string{"reset", "replace", "json", "outside_window", "unknown_window", "failed_wrapper", "memo_wrapper"} {
			t.Run(map[bool]string{false: "single", true: "pair"}[pair]+"/"+mutation, func(t *testing.T) {
				bus, result, path := hmc223NativeInvestigation(t, pair)
				switch mutation {
				case "failed_wrapper":
					result.Success = false
				case "memo_wrapper":
					result.ReusedFromRunMemo = true
				case "reset":
					bus.Mutable.ResetTurnAArtifacts()
				case "replace":
					body, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					replacement := path + ".replacement"
					if err := os.WriteFile(replacement, body, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(replacement, path); err != nil {
						t.Fatal(err)
					}
				case "json":
					body, err := json.Marshal(result)
					if err != nil {
						t.Fatal(err)
					}
					result = types.ToolResult{}
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatal(err)
					}
				case "outside_window", "unknown_window":
					start, end := 4.0, 4.5
					if mutation == "unknown_window" {
						start, end = 1, 2
						args := map[string]any{"source": "path", "path": path, "view": "measurements"}
						if pair {
							args = map[string]any{"comparison": map[string]any{"baseline": args}}
						}
						result = hmc17NamedQuery(t, bus, args)
						if !result.Success {
							t.Fatal(result.Summary)
						}
					}
					rm := types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "selected window", Confidence: 1}}
					bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
					bus.Mutable.SetRequestModel(rm)
				}
				bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
				if hmc223HasNativeInvestigation(bus) {
					t.Fatal("stale/out-of-scope result rescued empty investigation")
				}
			})
		}
	}
}

func TestHMC223NativeInvestigationRejectsSuccessAndStatusOnly(t *testing.T) {
	bus, _, _ := hmc17NamedPathContext(t)
	for _, result := range []types.ToolResult{
		{ToolName: "trace_query", Success: true, Summary: "query succeeded; 13 records"},
		hmc17NamedQuery(t, bus, map[string]any{"comparison": map[string]any{"baseline": map[string]any{"source": "path", "path": filepath.Join(bus.RepoRoot, "missing.data"), "view": "measurements"}}}),
	} {
		bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
		if hmc223HasNativeInvestigation(bus) {
			t.Fatal("success/status without native facts rescued empty investigation")
		}
	}
	queried, navigationOnly, _ := hmc223NativeInvestigation(t, false)
	navigationOnly.Observations = nil
	queried.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{navigationOnly}})
	if hmc223HasNativeInvestigation(queried) {
		t.Fatal("current navigation without a native publication rescued empty investigation")
	}
}

func TestHMC223NativeInvestigationAcceptsExecutedEmptyWindow(t *testing.T) {
	for _, pair := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "pair"}[pair], func(t *testing.T) {
			bus, _, path := hmc223NativeInvestigation(t, pair)
			args := map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 20, "time_end": 21}
			if pair {
				args = map[string]any{"comparison": map[string]any{"baseline": args}}
			}
			result := hmc17NamedQuery(t, bus, args)
			if !result.Success {
				t.Fatal(result.Summary)
			}
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0))
			foundEmpty := false
			for _, table := range contract.Choices() {
				if table.View == types.RuntimeMeasurementMembers && len(table.Rows) == 0 {
					foundEmpty = true
				}
			}
			if !foundEmpty || !hmc223HasNativeInvestigation(bus) {
				t.Fatalf("executed empty native window treated as no investigation: empty=%v", foundEmpty)
			}
		})
	}
}
