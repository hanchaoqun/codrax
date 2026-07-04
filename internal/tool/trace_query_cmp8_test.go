package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// CMP-8/CMP-9/CMP-10 renderer + observation pins (customer_dead_session_audit
// §7.1/§7.3/§7.4). Same synthetic fixture as the engine-side numeric pins in
// internal/tracequery/cpu_occupancy_test.go: 100ms window, two CPUs, known
// frequency and switch sequence.
const cmp8SyntheticTrace = `          <idle>-0 (-----) [000] .... 0.990000: cpu_frequency: state=1000000 cpu_id=0
          <idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hi next_pid=101 next_prio=90
          <idle>-0 (-----) [001] .... 1.000000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=201 next_prio=20
       worker-201 (  200) [001] .... 1.050000: sched_switch: prev_comm=worker prev_pid=201 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
           hi-101 (  101) [000] .... 1.060000: cpu_frequency: state=2000000 cpu_id=0
           hi-101 (  101) [000] .... 1.060000: sched_wakeup: comm=worker2 pid=202 prio=20 target_cpu=001
           hi-101 (  101) [000] .... 1.060000: sched_switch: prev_comm=hi prev_pid=101 prev_prio=90 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
          <idle>-0 (-----) [001] .... 1.080000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker2 next_pid=202 next_prio=20
      worker2-202 (  200) [001] .... 1.090000: sched_switch: prev_comm=worker2 prev_pid=202 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
`

func cmp8ExecuteView(t *testing.T, view string) types.ToolResult {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cmp8.systrace"), []byte(cmp8SyntheticTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`{"source":"path","path":"cmp8.systrace","view":"` + view + `","time_start":1.0,"time_end":1.1,"trace_flavor":"harmony_hitrace"}`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query %s failed: %s", view, res.Summary)
	}
	return res
}

// TestTraceQueryWindowStatsRendersOccupancyAndSupplyBalance pins the CMP-8
// occupancy stanza, the CMP-10 算力供给 stanza, and the CMP-9 density tokens
// on the window_stats view text.
func TestTraceQueryWindowStatsRendersOccupancyAndSupplyBalance(t *testing.T) {
	res := cmp8ExecuteView(t, "window_stats")
	// The tool path pads the requested 1.0..1.1 window by the ±0.5ms
	// shortened-timestamp tolerance → wall window 101ms: nominal 202 cpu·ms,
	// density 80/101≈0.79, core-limited remainder 202−90−30−20=62.
	for _, want := range []string{
		// CMP-8 occupancy stanza (cpu·ms unit contract on every surface).
		"- cpu_occupancy window_ms=101.000 unit=cpu·ms",
		"- cpu_occupancy_thread hi-101 running=60.000cpu·ms",
		"- cpu_occupancy_thread worker-201 running=50.000cpu·ms",
		"threads=2 running=60.000cpu·ms",
		"- cpu_occupancy_cpu cpu=1 ",
		"top=[worker-201 50.000cpu·ms; worker2-202 10.000cpu·ms]",
		"- cpu_occupancy_priority_band band=ohos_rt high_priority=true running=60.000cpu·ms threads=1",
		"- cpu_occupancy_priority_band band=ohos_cfs high_priority=false running=60.000cpu·ms threads=2",
		"- cpu_occupancy_caveat=occupancy durations are cpu-time (cpu·ms)",
		// CMP-10 supply ledger with the pinned numeric decomposition.
		"- compute_supply_balance(算力供给) window_ms=101.000 cpus=2 nominal=202.000cpu·ms delivered=90.000cpu·ms supply_ratio=0.446 low_freq_loss=30.000cpu·ms idle_mismatch=20.000ms(wall) core_limited≈62.000cpu·ms",
		"- compute_supply_balance_cpu cpu=0 ",
		"max_freq=2000000kHz freq_known=true",
		"freq_known=false",
		"无频点数据",
		// CMP-9 densities: aggregate line + per-CPU runnable-wait line.
		" window_ms=101.000 pressure_density=0.79 ",
		"- cpu_pressure cpu=1 runnable_wait=20.000ms runnable_density=0.20",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("window_stats summary missing %q:\n%s", want, res.Summary)
		}
	}
}

// TestTraceQuerySupplyPressureObservationCarriesDensityAndGuidance pins the
// CMP-9 typed rich notes (window_ms= / pressure_density= for downstream
// cross-trace comparison) and the CMP-10 occupancy-side guidance notes on the
// supply_pressure observation.
func TestTraceQuerySupplyPressureObservationCarriesDensityAndGuidance(t *testing.T) {
	res := cmp8ExecuteView(t, "window_stats")
	var supply *types.ObservationRecord
	for i := range res.Observations {
		if strings.Contains(res.Observations[i].ID, "#supply_pressure:") {
			supply = &res.Observations[i]
			break
		}
	}
	if supply == nil {
		t.Fatal("expected the typed #supply_pressure observation")
	}
	notes := strings.Join(supply.RichNotes, "\n")
	for _, want := range []string{
		"window_ms=101.000",
		"pressure_density=0.79",
		"recommended_views=window_stats",
		"recommended_sections=cpu_occupancy,compute_supply_balance,process_cpu_load",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("supply_pressure rich notes missing %q: %v", want, supply.RichNotes)
		}
	}
}

// TestTraceQueryComputeSupplyBalanceObservationContract pins adversarial-
// review F5 (2026-07-04): the window_stats path publishes exactly ONE typed
// compute_supply_balance observation when the engine produced a balance —
// ClaimKey "compute_supply_balance", Producer trace_query, and the full
// cross-lane rich-note contract (supply_ratio / delivered_cpu_ms /
// low_freq_loss_cpu_ms / idle_mismatch_ms / core_limited_cpu_ms / window_ms
// all %.3f, cpu_count %d) — so the downstream comparison lane never has to
// re-parse the rendered stanza. No balance → nothing is published.
func TestTraceQueryComputeSupplyBalanceObservationContract(t *testing.T) {
	res := cmp8ExecuteView(t, "window_stats")
	var rows []types.ObservationRecord
	for _, record := range res.Observations {
		if record.ClaimKey == "compute_supply_balance" {
			rows = append(rows, record)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("compute_supply_balance observations = %d, want exactly 1", len(rows))
	}
	bal := rows[0]
	if bal.Producer != "trace_query" {
		t.Fatalf("Producer = %q, want trace_query", bal.Producer)
	}
	notes := strings.Join(bal.RichNotes, "\n")
	// Padded 101ms window: nominal 202 cpu·ms, delivered 90, ratio 0.446,
	// loss 30, idle mismatch 20, core-limited remainder 62 (same pinned
	// numbers as the rendered stanza above).
	for _, want := range []string{
		"supply_ratio=0.446",
		"delivered_cpu_ms=90.000",
		"low_freq_loss_cpu_ms=30.000",
		"idle_mismatch_ms=20.000",
		"core_limited_cpu_ms=62.000",
		"window_ms=101.000",
		"cpu_count=2",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("compute_supply_balance rich notes missing %q: %v", want, bal.RichNotes)
		}
	}

	// No balance (e.g. unbounded window: the engine refuses to estimate a
	// nominal capacity) → the observation is not published.
	empty := traceQueryTypedWindowStatsObservations(tracequery.WindowStats{}, types.ObservationSourceRef{}, "scope", "2026-07-04T00:00:00Z")
	for _, record := range empty {
		if record.ClaimKey == "compute_supply_balance" {
			t.Fatalf("window stats without a balance must not publish compute_supply_balance: %+v", record)
		}
	}
}

// TestTraceQueryWindowStatsRendersClusterFrequencyCeilings pins the CFC F2(a)
// (2026-07-05 review) display consumer: the window ceilings snapshot renders
// as ONE soft line beside the compute-supply stanza on view=window_stats —
// per-cluster GHz (%.1f) with the VS-2b ladder provenance — and the line is
// omitted entirely when the window minted no ceiling.
func TestTraceQueryWindowStatsRendersClusterFrequencyCeilings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cfc.systrace"), []byte(cmp8SyntheticTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	// Explicit topology: cpu0 (the only CPU with cpu_frequency samples,
	// residency fmax 2.0GHz) is the big cluster; cpu1 has no observed sample
	// and must not mint a ceiling.
	params := json.RawMessage(`{"source":"path","path":"cfc.systrace","view":"window_stats","time_start":1.0,"time_end":1.1,"trace_flavor":"harmony_hitrace","core_topology":"big=0,small=1"}`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query window_stats failed: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "- cluster_frequency_ceilings big=2.0GHz(observed)") {
		t.Fatalf("window_stats summary missing the cluster ceilings display line:\n%s", res.Summary)
	}

	// No cpu_frequency samples anywhere → no ceilings → no line.
	schedOnly := `          <idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hi next_pid=101 next_prio=90
           hi-101 (  101) [000] .... 1.060000: sched_switch: prev_comm=hi prev_pid=101 prev_prio=90 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`
	if err := os.WriteFile(filepath.Join(dir, "nofreq.systrace"), []byte(schedOnly), 0o644); err != nil {
		t.Fatal(err)
	}
	params = json.RawMessage(`{"source":"path","path":"nofreq.systrace","view":"window_stats","time_start":1.0,"time_end":1.1,"trace_flavor":"harmony_hitrace"}`)
	res, err = (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query window_stats (no-freq) failed: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "- cluster_frequency_ceilings") {
		t.Fatalf("no-ceiling window must not render the cluster ceilings line:\n%s", res.Summary)
	}
}

// TestTraceQueryOccupancySectionsGatedToWindowStatsView pins the width
// decision: composite views already sit at the blob-preview cliff, so the
// CMP-8/CMP-10 stanzas render on view=window_stats ONLY (typed fields still
// ride the JSON payload; guidance steers follow-ups to window_stats).
func TestTraceQueryOccupancySectionsGatedToWindowStatsView(t *testing.T) {
	res := cmp8ExecuteView(t, "root_cause_rank")
	for _, forbidden := range []string{"- cpu_occupancy", "- compute_supply_balance", "- cluster_frequency_ceilings"} {
		if strings.Contains(res.Summary, forbidden) {
			t.Fatalf("composite view must not render %q (width cliff):\n%s", forbidden, res.Summary)
		}
	}
	// The CMP-9 density tokens are NOT gated — they ride the existing
	// supply/cpu_pressure lines on every view that prints them.
	if !strings.Contains(res.Summary, "runnable_density=0.20") {
		t.Fatalf("per-CPU runnable density must stay on the cpu_pressure line:\n%s", res.Summary)
	}
}

// TestTraceQueryStateFirstHintLeadsRunnableWithOccupancySide pins the CMP-8/
// CMP-10 guidance rewiring of the index-limit denial: the runnable branch
// names the occupancy side (top occupiers + compute supply) BEFORE the
// wait-side views.
func TestTraceQueryStateFirstHintLeadsRunnableWithOccupancySide(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "hint.systrace")
	if err := os.WriteFile(tracePath, []byte(cmp8SyntheticTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	p := traceQueryParams{
		View:      "root_cause_rank",
		Thread:    "worker",
		TimeStart: traceSecondFromAutoWindow(1.0),
		TimeEnd:   traceSecondFromAutoWindow(2.0),
	}
	res, ok := (&TraceQuery{}).traceQueryIndexLimitResult(ctx, p, tracePath, "path", &tracequery.IndexEventLimitError{
		Path:           tracePath,
		MaxEvents:      3,
		Events:         3,
		Line:           4,
		ScannedLines:   4,
		Windowed:       true,
		IndexTimeStart: 1.0,
		IndexTimeEnd:   2.0,
	})
	if !ok {
		t.Fatal("expected index limit result")
	}
	if !strings.Contains(res.Summary, "runnable->window_stats occupancy first (cpu_occupancy top occupiers + compute_supply_balance), then scheduler_latency/root_cause_rank with same CPU competitors") {
		t.Fatalf("state_first_hint runnable branch must lead with the occupancy side:\n%s", res.Summary)
	}
}
