package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func cpuMeasurementQuery(t *testing.T, path string, start, end float64) (types.ToolResult, types.ObservationRecord, tracequery.CPUStateFrequencyResult) {
	t.Helper()
	ctx, _, _ := hmc17NamedPathContext(t)
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "cpu_state_frequency", "time_start": start, "time_end": end})
	result, err := (&TraceQuery{}).Execute(ctx, args)
	if err != nil || !result.Success {
		t.Fatalf("public CPU query: %v / %s", err, result.Summary)
	}
	for _, record := range result.Observations {
		if record.Predicate != TraceCPUStateFrequencyPredicate {
			continue
		}
		body, err := os.ReadFile(record.SourceRef.PayloadRef)
		var native tracequery.Result
		if err != nil || json.Unmarshal(body, &native) != nil || native.CPUStateFrequency == nil {
			t.Fatalf("missing native result: %v", err)
		}
		return result, record, *native.CPUStateFrequency
	}
	t.Fatal("CPU observation absent")
	return types.ToolResult{}, types.ObservationRecord{}, tracequery.CPUStateFrequencyResult{}
}

func cpuMeasurementViews(t *testing.T, record types.ObservationRecord) map[types.RuntimeMeasurementView]types.RuntimeMeasurementTable {
	t.Helper()
	p, ok := types.DecodeRuntimeMeasurementPublication(record)
	if !ok {
		t.Fatal("CPU publication missing/invalid")
	}
	views := map[types.RuntimeMeasurementView]types.RuntimeMeasurementTable{}
	for _, table := range p.Tables {
		views[table.View] = table
	}
	return views
}

func TestCPUStateFrequencyMeasurementNativeRowsAndUnknown(t *testing.T) {
	for _, tc := range []struct {
		name, fixture string
		start, end    float64
		cpus, groups  int
		state         string
	}{
		{"ftrace", "hmosperf_cpu_state_frequency/events.systrace", 1, 1.04, 3, 0, "idle状态0"},
		{"native_sql", "hmosperf_cpu_native_intervals/capture.data", 0, .04, 2, 7, "源CPU状态码0（含义未核实）"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, _ := filepath.Abs("../../eval/fixtures/" + tc.fixture)
			_, record, native := cpuMeasurementQuery(t, path, tc.start, tc.end)
			views := cpuMeasurementViews(t, record)
			distribution, timeline := views[types.RuntimeMeasurementDistribution], views[types.RuntimeMeasurementTimeline]
			if len(views) != 3 || len(views[types.RuntimeMeasurementSummary].Rows) != tc.cpus || tc.groups > 0 && len(distribution.Rows) != tc.groups {
				t.Fatalf("wrong retained cardinality: %+v", views)
			}
			var wantDistribution, wantTimeline [][]string
			for _, cpu := range native.CPUs {
				for _, g := range cpu.Groups {
					state, freq := cpuStateFrequencyLabels(g.CPUStateFrequencyValue)
					wantDistribution = append(wantDistribution, []string{fmt.Sprintf("CPU%d", cpu.CPU), state, freq, fmt.Sprintf("%.9g", g.DurationMs), fmt.Sprintf("%.9g", g.WindowPct)})
				}
				for _, iv := range cpu.Intervals {
					state, freq := cpuStateFrequencyLabels(iv.CPUStateFrequencyValue)
					line := func(n int) string {
						if n == 0 {
							return "unknown"
						}
						return fmt.Sprint(n)
					}
					wantTimeline = append(wantTimeline, []string{fmt.Sprintf("CPU%d", cpu.CPU), traceQueryDisplaySeconds(iv.StartTs), traceQueryDisplaySeconds(iv.EndTs), state, freq, fmt.Sprintf("%.9g", iv.DurationMs), line(iv.IdleLine), line(iv.FrequencyLine)})
				}
			}
			if !reflect.DeepEqual(distribution.Rows, wantDistribution) || !reflect.DeepEqual(timeline.Rows, wantTimeline) {
				t.Fatal("receipt duplicated, omitted, relabeled or recalculated native rows")
			}
			body, _ := json.Marshal(views)
			for _, want := range []string{tc.state, "未知", "Frequency (kHz)", "Full CPU window (%)", "CPU-ms", "not wall time"} {
				if !strings.Contains(string(body), want) {
					t.Errorf("lost %q", want)
				}
			}
			// A malformed or stale typed producer result cannot publish a valid
			// display table even if the bounded handoff would hide the defect.
			native.CPUs[len(native.CPUs)-1].Groups[0].WindowPct = 999
			if got := traceQueryCPUStateFrequencyReceipt(record, native); got != "" {
				t.Fatal("corrupt native result published")
			}
		})
	}
}

func TestCPUStateFrequencyMeasurementUnavailableNoZero(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_hisys_row_identity/capture.data")
	_, record, _ := cpuMeasurementQuery(t, path, 2, 2.08)
	views := cpuMeasurementViews(t, record)
	if len(views) != 1 || views[types.RuntimeMeasurementSummary].Rows[0][1] != "unavailable; quantities unknown" || views[types.RuntimeMeasurementSummary].Rows[0][2] != "sql_measure_interval_semantics_not_preserved" {
		t.Fatalf("unavailable quantities became measurement rows: %+v", views)
	}
	for _, cell := range views[types.RuntimeMeasurementSummary].Rows[0] {
		if cell == "0" {
			t.Fatal("absence presented as measured zero")
		}
	}
}

func TestCPUStateFrequencyMeasurementRetainsProducerCapsNotHandoffCaps(t *testing.T) {
	for _, tc := range []struct {
		name            string
		cpus, intervals int
	}{{"handoff_only", 20, 20}, {"native_cap", 70, 80}} {
		t.Run(tc.name, func(t *testing.T) {
			var body strings.Builder
			for cpu := 0; cpu < tc.cpus; cpu++ {
				fmt.Fprintf(&body, "emitter-1 (1) [000] .... 1.000000: cpu_idle: state=0 cpu_id=%d\n", cpu)
				for row := 0; row < tc.intervals; row++ {
					fmt.Fprintf(&body, "emitter-1 (1) [000] .... %.6f: cpu_frequency: state=%d cpu_id=%d\n", 1+float64(row)/1000, 1000000+row, cpu)
				}
			}
			path := filepath.Join(t.TempDir(), "cpu.systrace")
			if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
				t.Fatal(err)
			}
			_, record, native := cpuMeasurementQuery(t, path, 1, 1+float64(tc.intervals)/1000)
			handoff, ok := DecodeTraceCPUStateFrequency(record)
			views := cpuMeasurementViews(t, record)
			retainedCPUs, retainedIntervals := min(tc.cpus, 64), min(tc.intervals, 64)
			if !ok || len(handoff.CPUs) != 16 || len(handoff.CPUs[0].Groups) != 16 || len(handoff.CPUs[0].Intervals) != 16 {
				t.Fatal("fixture did not cross all handoff limits")
			}
			if len(views[types.RuntimeMeasurementSummary].Rows) != retainedCPUs || len(views[types.RuntimeMeasurementDistribution].Rows) != retainedCPUs*retainedIntervals || len(views[types.RuntimeMeasurementTimeline].Rows) != retainedCPUs*retainedIntervals {
				t.Fatal("table was rebuilt from abbreviated handoff or exceeded native retained rows")
			}
			for _, table := range views {
				notes := strings.Join(table.Notes, " ")
				for _, want := range []string{fmt.Sprintf("CPUs displayed %d of %d; omitted %d", retainedCPUs, tc.cpus, tc.cpus-retainedCPUs), "displayed rows need not sum", fmt.Sprintf("full-window CPU-time %.9g CPU-ms", native.CPUTimeMs)} {
					if !strings.Contains(notes, want) {
						t.Errorf("lost cap/denominator %q", want)
					}
				}
			}
			for view, kind := range map[types.RuntimeMeasurementView]string{types.RuntimeMeasurementDistribution: "groups", types.RuntimeMeasurementTimeline: "intervals"} {
				notes := strings.Join(views[view].Notes, " ")
				if !strings.Contains(notes, fmt.Sprintf("%s displayed %d of %d; omitted %d", kind, retainedCPUs*retainedIntervals, retainedCPUs*tc.intervals, retainedCPUs*(tc.intervals-retainedIntervals))) {
					t.Fatal("lost native display counts")
				}
				if tc.intervals > 64 && !strings.Contains(notes, fmt.Sprintf("CPU0 %s displayed 64 of %d, omitted %d", kind, tc.intervals, tc.intervals-64)) {
					t.Fatal("lost per-CPU omitted count")
				}
			}
			// Validate the full result before publishing: corruption outside the
			// first 16 CPUs must not disappear behind the prompt lens.
			native.CPUs[17].Groups[0].WindowPct = 999
			if got := traceQueryCPUStateFrequencyReceipt(record, native); got != "" {
				t.Fatal("hidden invalid tail acquired table authority")
			}
		})
	}
}

func TestCPUStateFrequencyMeasurementExactSourceWindowConflict(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_cpu_state_frequency/events.systrace")
	first, record, native := cpuMeasurementQuery(t, path, 1, 1.04)
	second, _, _ := cpuMeasurementQuery(t, path, 1.01, 1.03)
	data, _ := os.ReadFile(path)
	otherPath := filepath.Join(t.TempDir(), "different-source.systrace")
	if err := os.WriteFile(otherPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	third, _, _ := cpuMeasurementQuery(t, otherPath, 1, 1.04)
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{first, second, third, first}}
	if got := types.BuildRuntimeMeasurementContract(input); len(got.Choices()) != 9 {
		t.Fatalf("sources/windows merged or repeat duplicated: %d", len(got.Choices()))
	}
	start, end := 1.01, 1.03
	input.RequestModel = &types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "one window"}}
	if got := types.BuildRuntimeMeasurementContract(input); len(got.Choices()) != 3 {
		t.Fatalf("out-of-window publication used: %d", len(got.Choices()))
	}
	for _, mutate := range []func(*types.ObservationRecord){
		func(r *types.ObservationRecord) { r.Negative = true },
		func(r *types.ObservationRecord) { r.SourceRef.Path = otherPath },
		func(r *types.ObservationRecord) { r.SourceRef.PayloadRef += ".stale" },
		func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID += "other" },
		func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs = 1.03 },
	} {
		copy := record
		mutate(&copy)
		if _, ok := types.DecodeRuntimeMeasurementPublication(copy); ok {
			t.Fatal("wrong source/window/authority admitted")
		}
	}
	publication, _ := types.DecodeRuntimeMeasurementPublication(record)
	publication.Tables[0].Rows[0][0] = "conflicting CPU"
	encoded, _ := json.Marshal(publication)
	conflict := record
	conflict.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(encoded)}
	input = types.ObservationLedgerInput{ToolResults: []types.ToolResult{first, {ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{conflict}}}}
	if got := types.BuildRuntimeMeasurementContract(input); got.Active() {
		t.Fatal("conflicting publication chose a version")
	}
	stale := record
	stale.SourceRef.QueryWindowEndTs = 1.03
	if got := traceQueryCPUStateFrequencyReceipt(stale, native); got != "" {
		t.Fatal("old typed window rebound to new scope")
	}
}
