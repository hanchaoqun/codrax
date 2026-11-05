package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC352ValueContextCoverageUsesActualFrequencyInputs(t *testing.T) {
	const sched = `idle-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=53
app-100 (100) [000] .... 1.100000: sched_switch: prev_comm=app prev_pid=100 prev_prio=53 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`
	for _, tc := range []struct {
		name, freq     string
		known, unknown float64
	}{
		{"none", "", 0, 100},
		{"head_governing", "idle-0 (-----) [000] .... 0.990000: cpu_frequency: state=1000000 cpu_id=0\n", 100, 0},
		{"known_tail_unknown_head", "app-100 (100) [000] .... 1.050000: cpu_frequency: state=1000000 cpu_id=0\n", 50, 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.ftrace")
			if err := os.WriteFile(path, []byte(tc.freq+sched), 0600); err != nil {
				t.Fatal(err)
			}
			for _, view := range []string{"window_stats", "root_cause_rank"} {
				args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 100, "time_start": 1.0, "time_end": 1.1, "trace_flavor": "harmony_hitrace"})
				r, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: filepath.Dir(path), WorkDir: t.TempDir()}, args)
				if err != nil || !r.Success {
					t.Fatalf("actual %s failed: %v %s", view, err, r.Summary)
				}
				var native tracequery.Result
				data, err := os.ReadFile(r.RawRef)
				if err != nil || json.Unmarshal(data, &native) != nil || native.WindowStats == nil || native.WindowStats.ComputeSupplyBalance == nil {
					t.Fatal("complete native result unavailable", err)
				}
				bal := native.WindowStats.ComputeSupplyBalance
				if len(bal.PerCPU) != 1 || !bal.PerCPU[0].FrequencyCoverageKnown || math.Abs(bal.PerCPU[0].FrequencyKnownRunningMs-tc.known) > 1e-6 || math.Abs(bal.PerCPU[0].FrequencyUnknownRunningMs-tc.unknown) > 1e-6 || math.Abs(bal.DeliveredComputeMs-100) > 1e-6 || bal.LowFrequencyLossMs != 0 || math.Abs(bal.SupplyRatio-1) > 1e-6 {
					t.Fatalf("availability and coverage collapsed, or numeric algorithm changed: %+v", bal)
				}
				var found bool
				for _, row := range r.Observations {
					if row.ClaimKey != "compute_supply_balance" {
						continue
					}
					found = true
					text := types.FormatObservationValueContexts(row.ValueContext)
					if row.Value != "1.000" || !strings.Contains(text, "frequency_weighted_estimate") || !strings.Contains(text, "frequency_unknown_running_ms=") {
						t.Fatal("qualified estimate was dropped or renamed")
					}
					for _, caveat := range bal.Caveats {
						retained := false
						for _, context := range row.ValueContext {
							retained = retained || context.Detail == caveat
						}
						if !retained {
							t.Fatalf("source calculation limit lost: %s", caveat)
						}
					}
				}
				if !found {
					t.Fatal("typed estimate absent")
				}
			}
		})
	}
}

func TestHMC352ValueContextKeepsMissingLegacyCoverageUnknown(t *testing.T) {
	if traceQueryComputeSupplyValueContext(nil) != nil {
		t.Fatal("absent balance gained context")
	}
	bal := &tracequery.ComputeSupplyBalance{PerCPU: []tracequery.ComputeSupplyCPUBalance{{CPU: 2, FrequencyKnown: true, RunningMs: 7, MaxFrequencyKHz: 1000000}}}
	text := types.FormatObservationValueContexts(traceQueryComputeSupplyValueContext(bal))
	if !strings.Contains(text, "not published") || strings.Contains(text, "frequency_unknown_running_ms=0") {
		t.Fatal("missing coverage became a measured zero")
	}
}
