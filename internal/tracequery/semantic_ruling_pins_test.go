package tracequery

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// semantic_ruling_pins_test.go — RN-16 (§7.9) layer 2: the regression-pin
// family for USER-ADJUDICATED semantic rulings, table-driven off the causal
// token registry (causal_token_registry.go) so the registry and its consumers
// stay interlocked.
//
// ── Family inventory (existing ruling pins are NOT migrated here; they stay
//    where they live — this header is the index) ──────────────────────────────
//
//   - RN-15 demand/supply double-publish pins (per-thread runnable must never
//     ride compute_supply; §7.4):
//     internal/tracequery/compute_supply_runnable_rn15_test.go
//   - VS-1 periodic-source pins (in-period sleep is cadence not cause;
//     sub-tolerance jitter yields lateness EXACTLY 0; §7.8):
//     internal/tracequery/tracequery_periodic_source_vs1_test.go
//   - 墙钟不可加和 / no cross-layer Σ (v3 projection ruling; CMP-9 normalized
//     densities are the only cross-window comparison):
//     internal/tool/answer_document_mutation_runtime_tree.go (coverage
//     arithmetic) + internal/tool/answer_document_projection_compare_test.go
//   - CMP-3 cross-thread aggregate display treatment (bar-scale exclusion +
//     "(跨线程累计,非墙钟)" annotation):
//     internal/tool/answer_document_projection_* tests; display/registry
//     interlock lives in internal/tool/semantic_ruling_pins_test.go
//   - Wording lanes (算力 / 调度压力 / 需求积压 string-literal whitelist):
//     internal/tool/semantic_wording_lint_test.go
//   - §7.11 C 表防倒退(hmtrace/hiview 平行审计裁定:这些点位 codrax 已
//     更优,禁止以"对齐参考库"为由倒退):
//     C-1 limits min/max 保真(vs hmtrace 有损折叠为单值)— pinned in
//     tracequery_test.go (cpu_frequency_limits ParseLine fidelity keeps
//     Min/Max distinct + limits rows stay searchable with FrequencyMax) and
//     parse_batch_p_test.go (float-shape min/max stay distinct);
//     C-7 簇判定无硬编码(resolveCoreTopology explicit-topology-then-
//     frequency-tier inference; vs hiview 12 核 policy 目录硬编码)—
//     exercised across cpu_occupancy / supply_fold tests, no per-SoC table
//     exists to regress to;
//     C-8 binder ftrace 族独有超集(两参考库 0 命中该事件族;禁因"参考
//     没有"删)— TestSemanticRulingPin_C8BinderFamilyStillParsed below.
//   - VS-2c 簇泳道角色裁定 (§7.10 终局裁定): isCPUFrequencyClock 名字启发
//     只准喂软引导/旁证 caveat,禁作 supply-fold fmax 基准 —
//     TestSemanticRulingPin_ClockLaneNeverFeedsSupplyFoldFmax below.
//   - CFC (§7.10 VS-2c 设计): the SAME clock-lane exclusion governs the
//     WINDOW-face per-CPU frequency basis (residency / topology inference /
//     low_frequency verdicts) via the shared predicate isPerCPUFrequencySample
//     (cluster_ceilings.go); the deduped per-cluster fmax lives in
//     WindowStats.ClusterFrequencyCeilings (internal, no registry token) —
//     TestSemanticRulingPin_CFCWindowFaceClockLaneExclusion below +
//     cluster_ceilings_test.go.

// semanticPinFixtures are the in-repo trace fixtures; the RN-15 full-scan pin
// runs the real rank/evidence pipeline over each of them.
var semanticPinFixtures = []string{
	"android_perfetto_cpu_frequency_limits.systrace",
	"android_perfetto_sched_blocked.systrace",
	"harmony_openharmony_bytrace_thread.txt",
}

// TestSemanticRulingPin_RN15FixtureFullScan pins the RN-15 end-state on real
// fixture pipelines (full window, whole rank + evidence faces): zero rows
// where type=compute_supply carries a per-thread subject, and — generalized
// via the registry — zero rows where ANY aggregate-only token carries a
// per-thread subject. The construction guard would already panic on such a
// row; this scan is the independent output-side witness.
func TestSemanticRulingPin_RN15FixtureFullScan(t *testing.T) {
	for _, name := range semanticPinFixtures {
		t.Run(name, func(t *testing.T) {
			idx, err := BuildIndex(context.Background(), filepath.Join("testdata", name))
			if err != nil {
				t.Fatalf("BuildIndex(%s): %v", name, err)
			}
			q := normalizeQuery(idx, Query{})
			rank := BuildRootCauseRank(idx, q)
			stats := ComputeWindowStats(idx, q)
			facts := evidenceFromStats(stats)
			if len(rank.Items) == 0 && len(facts) == 0 {
				t.Fatalf("fixture %s produced neither rank items nor evidence facts — the scan pinned nothing", name)
			}
			for _, item := range rank.Items {
				perThread := item.Thread.PID > 0 || strings.TrimSpace(item.Thread.Comm) != ""
				if item.Type == "compute_supply" && perThread {
					t.Fatalf("RN-15: fixture %s published a per-thread compute_supply rank row: %+v", name, item)
				}
				spec, ok := CausalTokenSpecFor(item.Type)
				if !ok {
					t.Fatalf("fixture %s published rank row with unregistered token %q: %+v", name, item.Type, item)
				}
				if spec.Subject == CausalSubjectAggregateOnly && perThread {
					t.Fatalf("fixture %s published aggregate-only token %q with per-thread subject: %+v", name, item.Type, item)
				}
			}
			for _, fact := range facts {
				if fact.Predicate == "compute_supply" {
					t.Fatalf("RN-15: fixture %s published a compute_supply evidence fact (per-thread verdict rows must not be republished): %+v", name, fact)
				}
			}
		})
	}
}

// TestSemanticRulingPin_AdditivityEngineInterlock pins set equality between
// the registry's cross-thread cpu·ms row tokens and the engine's
// rootCauseAggregateMetricTypes (the subject_kind=aggregate_metric stamp
// source), modulo the documented exception list. Together with the tool-side
// display interlock this is the "two consumer sets locked to one registry"
// pin: a token cannot gain or lose cross-thread semantics in one place only.
func TestSemanticRulingPin_AdditivityEngineInterlock(t *testing.T) {
	want := map[string]bool{}
	for _, token := range CausalTokenUniverse() {
		spec, _ := CausalTokenSpecFor(token)
		if spec.RowToken && spec.Additivity == CausalAdditivityCrossThreadCPUms && !CausalTokenCrossThreadRowException(token) {
			want[token] = true
		}
	}
	for token := range want {
		if !rootCauseAggregateMetricTypes[token] {
			t.Errorf("registry says %q is a cross-thread cpu·ms row token but rootCauseAggregateMetricTypes does not contain it", token)
		}
	}
	for token := range rootCauseAggregateMetricTypes {
		if !want[token] {
			t.Errorf("rootCauseAggregateMetricTypes contains %q but the registry does not mark it cross_thread_cpu_ms (or it is on the exception list)", token)
		}
	}
	// The exception list itself stays honest: every entry is a registered
	// cross-thread row token that is genuinely absent from the engine set.
	for token := range causalRegistryCrossThreadRowExceptions {
		spec, ok := CausalTokenSpecFor(token)
		if !ok {
			t.Errorf("exception list entry %q is not in the registry", token)
			continue
		}
		if !spec.RowToken || spec.Additivity != CausalAdditivityCrossThreadCPUms {
			t.Errorf("exception list entry %q must be a RowToken with cross_thread_cpu_ms additivity, got %+v", token, spec)
		}
		if rootCauseAggregateMetricTypes[token] {
			t.Errorf("exception list entry %q IS in rootCauseAggregateMetricTypes — delete it from causalRegistryCrossThreadRowExceptions", token)
		}
	}
}

// TestSemanticRulingPin_RegistryLaneRulings pins the ruling-locked registry
// rows verbatim (§7.4 demand/supply separation, §7.5 supply_pressure token
// retention) plus the cross-axis invariants that make the lanes coherent.
func TestSemanticRulingPin_RegistryLaneRulings(t *testing.T) {
	mustSpec := func(token string) CausalTokenSpec {
		t.Helper()
		spec, ok := CausalTokenSpecFor(token)
		if !ok {
			t.Fatalf("token %q missing from registry", token)
		}
		return spec
	}

	// §7.4: supply_pressure is DEMAND-side (Σ runnable backlog) despite its
	// name; §7.5: the wire token is retained and its demand-backlog wording is
	// owned by the display helper.
	sp := mustSpec("supply_pressure")
	if sp.Lane != CausalLaneSchedulingDemand || sp.Additivity != CausalAdditivityCrossThreadCPUms || sp.Subject != CausalSubjectAggregateOnly {
		t.Fatalf("supply_pressure must be scheduling_demand / cross_thread_cpu_ms / aggregate_only (§7.4, §7.5), got %+v", sp)
	}
	if sp.LabelZhRef != CausalZhLabelRefSupplyPressure {
		t.Fatalf("supply_pressure zh label must be owned by runtimeTraceSupplyPressureDisplayLabel (§7.5), got %q", sp.LabelZhRef)
	}

	// §7.4 / RN-15: compute_supply is delivery-side and aggregate-only.
	cs := mustSpec("compute_supply")
	if cs.Lane != CausalLaneComputeDelivery || cs.Subject != CausalSubjectAggregateOnly {
		t.Fatalf("compute_supply must be compute_delivery / aggregate_only (§7.4, RN-15), got %+v", cs)
	}
	csb := mustSpec("compute_supply_balance")
	if csb.Lane != CausalLaneComputeDelivery || csb.Subject != CausalSubjectAggregateOnly || csb.RowToken {
		t.Fatalf("compute_supply_balance must be compute_delivery / aggregate_only / observation-only, got %+v", csb)
	}

	// The demand family stays on the demand lane.
	for _, token := range []string{"runnable_wait", "fragmented_runnable_wait", "scheduler_latency", "cpu_pressure", "priority_inversion_candidate", "priority_inversion_runnable_wait", "runnable_occupancy"} {
		if spec := mustSpec(token); spec.Lane != CausalLaneSchedulingDemand {
			t.Errorf("%s must stay on the scheduling_demand lane (§7.4), got %s", token, spec.Lane)
		}
	}
	// The delivery family stays on the delivery lane.
	for _, token := range []string{"low_frequency", "cpu_frequency_limit"} {
		if spec := mustSpec(token); spec.Lane != CausalLaneComputeDelivery {
			t.Errorf("%s must stay on the compute_delivery lane (§7.4), got %s", token, spec.Lane)
		}
	}
	// runnable_occupancy (RN-1) is an observation predicate, never a row.
	if spec := mustSpec("runnable_occupancy"); spec.RowToken {
		t.Errorf("runnable_occupancy must be observation-only")
	}

	// Cross-axis invariants.
	for _, token := range CausalTokenUniverse() {
		spec, _ := CausalTokenSpecFor(token)
		if spec.Subject == CausalSubjectAggregateOnly && spec.Additivity == CausalAdditivityWallClockPerThread {
			t.Errorf("%s: aggregate_only subject with wall_clock_per_thread additivity is incoherent", token)
		}
		if spec.RowToken && spec.Additivity == CausalAdditivityCrossThreadCPUms && spec.Subject == CausalSubjectPerThread {
			t.Errorf("%s: a per_thread row token cannot be cross_thread_cpu_ms", token)
		}
	}
}

// TestSemanticRulingPin_ClockLaneNeverFeedsSupplyFoldFmax pins the VS-2c
// cluster-lane role ruling (§7.10 终局裁定): clock_set_rate lane names are
// vendor free vocabulary, so a cpu-freq-NAMED lane (isCPUFrequencyClock hit)
// may only surface as the SupplyFoldBasis corroboration caveat — its samples
// MUST NOT move the fold fmax, neither when real cpu_frequency governance
// exists (probe A) nor when the lane is the only frequency-shaped data at
// all (probe B). Probe C pins the caveat's precise >10% comparison staying
// quiet on agreement (一致时不加注).
func TestSemanticRulingPin_ClockLaneNeverFeedsSupplyFoldFmax(t *testing.T) {
	schedBody := `
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.009900: sched_wakeup: comm=app pid=100 prio=53 target_cpu=001
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`
	foldQuery := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	depBasis := func(t *testing.T, idx *Index) (*WakeupCausalImpact, *SupplyFoldBasis) {
		t.Helper()
		chain := BuildWakeupChain(idx, foldQuery)
		for i := range chain.CausalImpacts {
			if chain.CausalImpacts[i].Thread.PID == 200 {
				if chain.CausalImpacts[i].SupplyFoldBasis == nil {
					t.Fatalf("fold must run: %+v", chain.CausalImpacts[i])
				}
				return &chain.CausalImpacts[i], chain.CausalImpacts[i].SupplyFoldBasis
			}
		}
		t.Fatalf("dependency causal impact missing: %+v", chain.CausalImpacts)
		return nil, nil
	}

	t.Run("lane sample above observed governance never raises fmax", func(t *testing.T) {
		idx := buildTraceIndex(t, "vs2c_lane_a.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
          clk-90 (  90) [000] .... 5.005000: clock_set_rate: cpu_freq 3000000`+schedBody)
		dep, basis := depBasis(t, idx)
		if basis.FmaxKHz != 2000000 || basis.FmaxSource != SupplyFoldFmaxSourceObserved {
			t.Fatalf("lane 3GHz sample must not move the 2GHz observed fmax: %+v", basis)
		}
		// ~10ms @1GHz vs 2GHz → ~5ms; a leaked 3GHz fmax would give ~6.67ms.
		if dep.SupplyFoldDeficitMs < 4.7 || dep.SupplyFoldDeficitMs > 5.3 {
			t.Fatalf("deficit must fold against the observed fmax only, got %.3f", dep.SupplyFoldDeficitMs)
		}
		if basis.ClusterLaneName != "cpu_freq" || basis.ClusterLaneMaxKHz != 3000000 {
			t.Fatalf("lane must be recorded as corroboration: %+v", basis)
		}
		if !basis.ClusterLaneDivergent {
			t.Fatalf("3GHz lane vs 2GHz fmax is >10%% divergence — caveat flag must set: %+v", basis)
		}
	})

	t.Run("lane-only trace never mints an fmax", func(t *testing.T) {
		idx := buildTraceIndex(t, "vs2c_lane_b.systrace", `
          clk-90 (  90) [000] .... 5.005000: clock_set_rate: cpu_freq 3000000`+schedBody)
		dep, basis := depBasis(t, idx)
		if basis.FmaxKHz != 0 || basis.FmaxSource != "" {
			t.Fatalf("a lane sample alone must never become the fold fmax: %+v", basis)
		}
		if dep.SupplyFoldDeficitMs != 0 || basis.KnownMs != 0 {
			t.Fatalf("without real governance the fold books unknown, zero deficit: %+v basis=%+v", dep, basis)
		}
		if basis.ClusterLaneDivergent {
			t.Fatalf("no fmax → no divergence verdict (nothing to diverge from): %+v", basis)
		}
	})

	t.Run("consistent lane stays quiet", func(t *testing.T) {
		idx := buildTraceIndex(t, "vs2c_lane_c.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
          clk-90 (  90) [000] .... 5.005000: clock_set_rate: cpu_freq 2000000`+schedBody)
		_, basis := depBasis(t, idx)
		if basis.ClusterLaneMaxKHz != 2000000 || basis.ClusterLaneDivergent {
			t.Fatalf("lane agreeing with fmax must not raise the divergence caveat (一致时不加注): %+v", basis)
		}
	})
}

// TestSemanticRulingPin_CFCWindowFaceClockLaneExclusion pins the CFC batch's
// P0 ruling extension (§7.10 VS-2c 设计): the clock-lane exclusion the fold
// face already pins governs the WINDOW face too, through the one shared
// predicate. Three probes on a lane-bearing trace (cpu-freq-NAMED
// clock_set_rate at 3.0e9 landing on the emitting cpu0, real per-CPU samples
// 1.0/1.2/2.0 GHz on cpus 0/2/7):
//
//	(1) residency purity — cpu0's CPUStats.Frequency/FrequencyResidency read
//	    only the genuine 1.0GHz sample, never the lane value;
//	(2) topology stability — the frequency-tier inference keeps cpu0=small /
//	    cpu2=middle / cpu7=big (a leaked 3.0e9 lane max would flip cpu0 to
//	    big and displace every other class);
//	(3) no fabricated low_frequency — the 6.9ms runnable wait on cpu0 spans
//	    the lane sample; a leaked lane would make weighted_freq≈1.7e9 vs
//	    observed_max=3.0e9 (≤65%) and mint a low_frequency root-cause row.
func TestSemanticRulingPin_CFCWindowFaceClockLaneExclusion(t *testing.T) {
	idx := buildTraceIndex(t, "cfc_window_lane.systrace", `
      <idle>-0 (-----) [000] .... 4.900000: cpu_frequency: state=1000000 cpu_id=0
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1200000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
        app-100 (100) [000] .... 5.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120
        app-100 (100) [000] .... 5.002000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
        wkr-300 (300) [002] .... 5.002100: sched_wakeup: comm=lat pid=400 prio=120 target_cpu=000
          clk-90 (  90) [000] .... 5.005000: clock_set_rate: cpu_freq 3000000000
        lat-400 (400) [000] .... 5.009000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=lat next_pid=400 next_prio=120
        lat-400 (400) [000] .... 5.010000: sched_switch: prev_comm=lat prev_pid=400 prev_prio=120 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`)
	q := Query{TimeStart: 5.0, TimeEnd: 5.010, MinDurationMs: 1, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	stats := ComputeWindowStats(idx, q)

	// Probe 1: residency purity on the lane's landing CPU.
	var cpu0 *CPUStats
	for i := range stats.CPU {
		if stats.CPU[i].CPU == 0 {
			cpu0 = &stats.CPU[i]
		}
	}
	if cpu0 == nil {
		t.Fatalf("cpu0 missing from window stats: %+v", stats.CPU)
	}
	if cpu0.Frequency != 1000000 {
		t.Fatalf("probe 1: cpu0 latest frequency must stay the genuine 1.0GHz sample, got %+v", cpu0)
	}
	for _, res := range cpu0.FrequencyResidency {
		if res.Frequency != 1000000 {
			t.Fatalf("probe 1: lane sample leaked into cpu0 residency: %+v", cpu0.FrequencyResidency)
		}
	}

	// Probe 2: frequency-tier topology unmoved by the lane.
	wantClass := map[int]string{0: "small", 2: "middle", 7: "big"}
	for _, cpu := range stats.CPU {
		if want, ok := wantClass[cpu.CPU]; ok && cpu.CoreClass != want {
			t.Fatalf("probe 2: lane flipped the inferred topology: cpu=%d got %q want %q (%+v)", cpu.CPU, cpu.CoreClass, want, stats.CPU)
		}
	}

	// Probe 3: no fabricated low_frequency root-cause row; the runnable-wait
	// row itself must exist so the probe is live.
	rank := BuildRootCauseRank(idx, q)
	sawLatency := false
	for _, item := range rank.Items {
		if item.Type == "low_frequency" {
			t.Fatalf("probe 3: lane fabricated a low_frequency root-cause row: %+v", item)
		}
		if item.Type == "scheduler_latency" && item.Thread.PID == 400 {
			sawLatency = true
		}
	}
	if !sawLatency {
		t.Fatalf("probe 3 liveness: the 6.9ms runnable wait must surface as scheduler_latency: %+v", rank.Items)
	}

	// CFC snapshot side-witness: the window ceilings read only genuine
	// samples (big cluster = 2.0GHz observed, never the 3.0e9 lane).
	for _, ceiling := range stats.ClusterFrequencyCeilings {
		if ceiling.FmaxKHz >= 3000000000 {
			t.Fatalf("lane leaked into ClusterFrequencyCeilings: %+v", stats.ClusterFrequencyCeilings)
		}
	}
}

// TestSemanticRulingPin_C8BinderFamilyStillParsed is the §7.11 C-8 existence
// pin: the binder ftrace event family is a codrax-only SUPERSET (zero hits in
// both reference libraries) and must never be deleted "for reference
// alignment" — every family member keeps its typed EventType.
func TestSemanticRulingPin_C8BinderFamilyStillParsed(t *testing.T) {
	intern := newStringInterner()
	cases := []struct {
		line string
		want EventType
	}{
		{`      waker-10   (   10) [000] .... 2.090000: binder_transaction: transaction=7 dest_node=0 dest_proc=40 dest_thread=41 reply=1 flags=0x0 code=0x1`, EventBinderTransaction},
		{`      binder-41  (   40) [001] .... 2.090100: binder_transaction_received: transaction=7`, EventBinderReceived},
		{`      binder-41  (   40) [001] .... 2.090200: binder_transaction_alloc_buf: transaction=7 data_size=64 offsets_size=8`, EventBinderAllocBuf},
		{`      binder-41  (   40) [001] .... 2.090300: binder_lock: tag=binder_main`, EventBinderLock},
	}
	for _, tc := range cases {
		ev, ok := ParseLine(1, tc.line, intern)
		if !ok {
			t.Fatalf("binder family line must parse: %s", tc.line)
		}
		if ev.Type != tc.want {
			t.Fatalf("binder family member lost its typed EventType: got %s want %s (%s)", ev.Type, tc.want, tc.line)
		}
	}
	if ev, ok := ParseLine(2, cases[0].line, intern); !ok || ev.BinderFields.TransactionID != 7 || ev.BinderFields.DestProc != 40 {
		t.Fatalf("binder_transaction typed fields must survive: %+v", ev)
	}
}

// TestSemanticRulingPin_ConstructionGuard pins the guard mechanism itself
// (RN-16 layer 1): under `go test` an unregistered token, an observation-only
// token on a row face, or an aggregate-only token with a concrete thread
// subject panics at the construction site; clean constructions never panic.
func TestSemanticRulingPin_ConstructionGuard(t *testing.T) {
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: expected a causal-token registry panic, got none", name)
			}
		}()
		fn()
	}
	mustNotPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("%s: unexpected panic: %v", name, r)
			}
		}()
		fn()
	}

	probe := ThreadRef{Comm: "guard-probe", PID: 4242}

	mustPanic("unregistered token", func() {
		_ = rootCauseItem("zz_unregistered_probe_token", ThreadRef{}, 1, 0.5, 1, 2, "pin", "probe")
	})
	mustPanic("observation-only token on rank face", func() {
		_ = rootCauseItem("runnable_occupancy", probe, 1, 0.5, 1, 2, "pin", "probe")
	})

	for _, token := range CausalTokenUniverse() {
		spec, _ := CausalTokenSpecFor(token)
		if !spec.RowToken {
			continue
		}
		if spec.Subject == CausalSubjectAggregateOnly {
			mustPanic(token+" with thread subject", func() {
				_ = rootCauseItem(token, probe, 1, 0.5, 1, 2, "pin", "probe")
			})
			mustNotPanic(token+" without subject", func() {
				_ = rootCauseItem(token, ThreadRef{}, 1, 0.5, 1, 2, "pin", "probe")
			})
			continue
		}
		mustNotPanic(token+" with thread subject", func() {
			_ = rootCauseItem(token, probe, 1, 0.5, 1, 2, "pin", "probe")
		})
	}
}

// TestSemanticRulingPin_ClusterLaneUnitHypotheses pins the F3 correction
// (2026-07-04 review): vendors are free vocabulary about lane UNITS too, so
// the corroboration comparison is magnitude-tolerant — the raw lane sample is
// normalized under the {÷1, ÷1e3, ÷1e6} unit hypotheses and ANY hypothesis
// within ±10% of the fold fmax declares agreement. The old single-sided
// "≥1e8 must be Hz" threshold produced REVERSED caveats for sub-100MHz-Hz
// lanes (96MHz emitted as 96000000 stayed "kHz" → false divergence) and for
// MHz lanes. Three review shapes pinned: 96MHz-in-Hz, 2200-in-MHz, true-kHz;
// plus the ≥1e8 true-Hz shape that must keep matching (regression parity).
func TestSemanticRulingPin_ClusterLaneUnitHypotheses(t *testing.T) {
	schedBody := `
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.009900: sched_wakeup: comm=app pid=100 prio=53 target_cpu=001
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`
	foldQuery := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	laneBasis := func(t *testing.T, name, freqRows string) *SupplyFoldBasis {
		t.Helper()
		idx := buildTraceIndex(t, name, freqRows+schedBody)
		chain := BuildWakeupChain(idx, foldQuery)
		for i := range chain.CausalImpacts {
			if chain.CausalImpacts[i].Thread.PID == 200 {
				if chain.CausalImpacts[i].SupplyFoldBasis == nil {
					t.Fatalf("fold must run: %+v", chain.CausalImpacts[i])
				}
				return chain.CausalImpacts[i].SupplyFoldBasis
			}
		}
		t.Fatalf("dependency causal impact missing: %+v", chain.CausalImpacts)
		return nil
	}

	t.Run("96MHz lane emitted in Hz matches under the ÷1e3 hypothesis", func(t *testing.T) {
		basis := laneBasis(t, "f3_lane_hz_sub100mhz.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=48000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=96000 cpu_id=7
          clk-90 (  90) [000] .... 5.005000: clock_set_rate: cpu_freq 96000000`)
		if basis.FmaxKHz != 96000 {
			t.Fatalf("observed fmax must be 96000 kHz, got %+v", basis)
		}
		if basis.ClusterLaneDivergent {
			t.Fatalf("96000000 Hz IS 96000 kHz — the old <1e8 blind spot must not flag a reversed divergence: %+v", basis)
		}
		if basis.ClusterLaneMaxKHz != 96000 {
			t.Fatalf("matched hypothesis must store the resolved kHz value, got %+v", basis)
		}
	})

	t.Run("MHz-shaped lane matches no hypothesis and hedges as unit-unknown", func(t *testing.T) {
		basis := laneBasis(t, "f3_lane_mhz.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1100000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2200000 cpu_id=7
          clk-90 (  90) [000] .... 5.005000: clock_set_rate: cpu_freq 2200`)
		if !basis.ClusterLaneDivergent {
			t.Fatalf("2200 matches 2200000 kHz under no {÷1,÷1e3,÷1e6} hypothesis — flag must set (display hedges 单位不明): %+v", basis)
		}
		if basis.ClusterLaneMaxKHz != 2200 {
			t.Fatalf("unresolved unit must keep the RAW lane value, got %+v", basis)
		}
	})

	t.Run("true kHz lane matches under the identity hypothesis", func(t *testing.T) {
		basis := laneBasis(t, "f3_lane_khz.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1100000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2200000 cpu_id=7
          clk-90 (  90) [000] .... 5.005000: clock_set_rate: cpu_freq 2200000`)
		if basis.ClusterLaneDivergent || basis.ClusterLaneMaxKHz != 2200000 {
			t.Fatalf("true-kHz lane must agree quietly (一致时不加注), got %+v", basis)
		}
	})

	t.Run("true Hz lane at or above 1e8 keeps matching (old-path parity)", func(t *testing.T) {
		basis := laneBasis(t, "f3_lane_hz_big.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1100000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2200000 cpu_id=7
          clk-90 (  90) [000] .... 5.005000: clock_set_rate: cpu_freq 2200000000`)
		if basis.ClusterLaneDivergent || basis.ClusterLaneMaxKHz != 2200000 {
			t.Fatalf("2.2e9 Hz must normalize to 2200000 kHz via ÷1e3, got %+v", basis)
		}
	})
}
