package tracequery

import "testing"

// core_capability_cap_test.go — CAP batch (§26, docs/design/
// real_trace_campaign_20260705.md, user ruling 2026-07-08) pins:
//
//	C1  cluster-count → core-class mapping (2/3/4 forms), explicit-label
//	    membership, and the fail-loud freq_only fallbacks (unjudgeable /
//	    single cluster / >4 clusters / fmax tie);
//	C2  formula witnesses — 小核满频 ≠ 无缺口 (a small core at its OWN fmax
//	    now mints a deficit), 大核满频 zero-deficit control (unchanged), and
//	    the R5d same-frequency cross-class witness (pre-CAP formula: exactly
//	    0);
//	C3  the typed three-state capability-source disclosure on the engine
//	    faces (basis flag + gated flag);
//	C4  direction dumps (数值前后对照 in t.Log + direction assertions) for
//	    the three representative forms.

func capTL(khz ...int) []freqSample {
	out := make([]freqSample, 0, len(khz))
	for i, k := range khz {
		out = append(out, freqSample{ts: 1.0 + float64(i), khz: k})
	}
	return out
}

// --- C1: cluster-count → class mapping over derived domains ------------------

func TestCoreCapabilityClusterClassMapping(t *testing.T) {
	cases := []struct {
		name    string
		tl      map[int][]freqSample
		wantCap map[int]float64 // cpu → coefficient
	}{
		{
			name: "two_clusters_small_big",
			tl:   map[int][]freqSample{0: capTL(1000000), 4: capTL(2000000)},
			wantCap: map[int]float64{
				0: coreCapabilityDefaultSmall,
				4: coreCapabilityDefaultBig,
			},
		},
		{
			name: "three_clusters_small_middle_big",
			tl:   map[int][]freqSample{0: capTL(1000000), 4: capTL(2000000), 7: capTL(3000000)},
			wantCap: map[int]float64{
				0: coreCapabilityDefaultSmall,
				4: coreCapabilityDefaultMiddle,
				7: coreCapabilityDefaultBig,
			},
		},
		{
			name: "four_clusters_small_middle_big_prime",
			tl: map[int][]freqSample{
				0: capTL(1000000), 3: capTL(2000000), 6: capTL(3000000), 7: capTL(4000000),
			},
			wantCap: map[int]float64{
				0: coreCapabilityDefaultSmall,
				3: coreCapabilityDefaultMiddle,
				6: coreCapabilityDefaultBig,
				7: coreCapabilityDefaultPrime,
			},
			// 复核 F1: the fold reference is the NOMINATED big class — a
			// four-cluster shape does NOT fold to prime (§26 letter).
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capability := resolveCoreCapability(deriveClusterFreqDomains(tc.tl), tc.tl)
			if capability.source != CoreCapabilitySourceDefault {
				t.Fatalf("judged structure must use the default table, got %q", capability.source)
			}
			for cpu, want := range tc.wantCap {
				if got := capability.capabilityFor(cpu); got != want {
					t.Fatalf("cpu%d coefficient = %v, want %v", cpu, got, want)
				}
			}
		})
	}
}

// The §26 coefficient table itself (常量单源 — a mutated table value reds
// here before any fold math is consulted). Derivation: 中=小×2.3;
// 大=中×1.1(=2.53); 超大=大×1.2(=3.036); 恒序 超大>大>中>小.
func TestCoreCapabilityDefaultTablePinned(t *testing.T) {
	if coreCapabilityDefaultSmall != 1.0 || coreCapabilityDefaultMiddle != 2.3 ||
		coreCapabilityDefaultBig != 2.53 || coreCapabilityDefaultPrime != 3.036 {
		t.Fatalf("§26 default table drifted: %v/%v/%v/%v",
			coreCapabilityDefaultSmall, coreCapabilityDefaultMiddle,
			coreCapabilityDefaultBig, coreCapabilityDefaultPrime)
	}
	if !(coreCapabilityDefaultPrime > coreCapabilityDefaultBig &&
		coreCapabilityDefaultBig > coreCapabilityDefaultMiddle &&
		coreCapabilityDefaultMiddle > coreCapabilityDefaultSmall) {
		t.Fatalf("§26 恒序 超大>大>中>小 violated")
	}
}

// Explicit topology: labels contribute MEMBERSHIP only — classes map from the
// sampled-cluster count + fmax order (§26 structural mapping), so a declared
// "prime" cluster in a 3-sampled-cluster trace takes the big coefficient.
func TestCoreCapabilityExplicitTopologyMembershipOnly(t *testing.T) {
	tl := map[int][]freqSample{0: capTL(1000000), 4: capTL(2000000), 7: capTL(3000000)}
	domains := parseClusterFreqDomains("small=0-3;big=4-6;prime=7")
	capability := resolveCoreCapability(domains, tl)
	if capability.source != CoreCapabilitySourceDefault {
		t.Fatalf("3 sampled clusters must judge, got %q", capability.source)
	}
	// Membership rides the labels (cpu1 inherits small=0-3's cluster even
	// without its own samples); the CLASS comes from the fmax order.
	if got := capability.capabilityFor(1); got != coreCapabilityDefaultSmall {
		t.Fatalf("cpu1 (declared small member) = %v, want small %v", got, coreCapabilityDefaultSmall)
	}
	if got := capability.capabilityFor(7); got != coreCapabilityDefaultBig {
		t.Fatalf("cpu7 (declared prime, top fmax of 3 clusters) = %v, want big %v", got, coreCapabilityDefaultBig)
	}
}

// --- C1: fail-loud freq_only fallbacks (§26: 簇结构不可判 → 纯频率比) --------

func TestCoreCapabilityUnjudgeableFailLoud(t *testing.T) {
	cases := []struct {
		name string
		tl   map[int][]freqSample
	}{
		{name: "no_domains", tl: map[int][]freqSample{}},
		{name: "single_cluster", tl: map[int][]freqSample{0: capTL(2000000)}},
		{name: "five_clusters_exceed_table", tl: map[int][]freqSample{
			0: capTL(1000000), 1: capTL(1200000), 2: capTL(1400000),
			3: capTL(1600000), 4: capTL(1800000),
		}},
		// An fmax TIE leaves no defensible order between two clusters (the
		// timelines differ — distinct clusters — but their maxima agree):
		// class assignment would be a coin flip on the label sort. 禁猜.
		{name: "fmax_tie", tl: map[int][]freqSample{
			0: {{ts: 1.0, khz: 2000000}},
			4: {{ts: 2.0, khz: 2000000}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capability := resolveCoreCapability(deriveClusterFreqDomains(tc.tl), tc.tl)
			if capability.source != CoreCapabilitySourceFreqOnly {
				t.Fatalf("unjudgeable structure must fail loud to freq_only, got %q", capability.source)
			}
			// freq_only prices every CPU at 1 — the pre-CAP pure ratio — and
			// never reports a known class (复核 F2: the R5d degrade keys on it).
			for cpu := range tc.tl {
				if got := capability.capabilityFor(cpu); got != 1 {
					t.Fatalf("freq_only must price cpu%d at 1, got %v", cpu, got)
				}
				if got := capability.sliceCapRatio(cpu, coreCapabilityDefaultBig); got != 1 {
					t.Fatalf("freq_only slice cap ratio for cpu%d must be 1, got %v", cpu, got)
				}
				if _, known := capability.capabilityForKnown(cpu); known {
					t.Fatalf("freq_only must never report a known class for cpu%d", cpu)
				}
			}
		})
	}
}

// --- C2 witness: 小核满频 ≠ 无供给缺口 (§26 判词重判) ------------------------

// The dep runs ~9.9ms on the small core AT ITS OWN FULL FREQUENCY (1.8GHz =
// the small cluster's fmax). Pre-CAP the fold read 9.9×(1−1.8/2.0)≈0.99ms —
// nearly the affirmative "已按大核满频(或接近)运行,无供给缺口" shape. CAP §26:
// 9.9×(1−(1.8×1.0)/(2.0×2.53)) ≈ 6.38ms — a small core at fmax still owes the
// class gap, so the affirmative verdict (deficit==0) is structurally
// unreachable for it.
func TestSupplyFoldSmallCoreFullFrequencyStillDeficit(t *testing.T) {
	idx := buildTraceIndex(t, "cap_small_fmax.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1800000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil || !dep.SupplyFoldBasis.AllKnown() {
		t.Fatalf("fold must run on a fully-known basis: %+v", dep.SupplyFoldBasis)
	}
	running := dep.SupplyFoldIdealMs + dep.SupplyFoldDeficitMs
	preCAP := running * (1 - 1.8/2.0)
	t.Logf("CAP §26 direction dump (小核满频 form): deficit pre-CAP≈%.3f → now %.3f", preCAP, dep.SupplyFoldDeficitMs)
	if dep.SupplyFoldDeficitMs <= 0 {
		t.Fatalf("小核满频 must NOT read as 无供给缺口 (§26 判词重判 witness), got deficit %.3f", dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldDeficitMs < 6.1 || dep.SupplyFoldDeficitMs > 6.7 {
		t.Fatalf("small-core-at-fmax deficit should be ≈6.38ms, got %.3f", dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldDeficitMs <= preCAP {
		t.Fatalf("direction: the capability fold must widen the small-core deficit (pre-CAP %.3f, got %.3f)", preCAP, dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldBasis.CapabilitySource != CoreCapabilitySourceDefault {
		t.Fatalf("default-table pricing must disclose itself, got %+v", dep.SupplyFoldBasis)
	}
}

// --- C2 control: 大核满频 zero deficit stays zero (§26 方向断言) -------------

func TestSupplyFoldBigCoreFullFrequencyZeroDeficitControl(t *testing.T) {
	idx := buildTraceIndex(t, "cap_big_fmax.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil || !dep.SupplyFoldBasis.AllKnown() {
		t.Fatalf("fold must run on a fully-known basis: %+v", dep.SupplyFoldBasis)
	}
	t.Logf("CAP §26 direction dump (大核满频 control): deficit pre-CAP=0.000 → now %.3f (must stay 0)", dep.SupplyFoldDeficitMs)
	if dep.SupplyFoldDeficitMs != 0 {
		t.Fatalf("big-core-at-fmax must keep zero deficit (方向断言: 大核满频缺口不变), got %.3f", dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldBasis.CapabilitySource != CoreCapabilitySourceDefault {
		t.Fatalf("the zero-deficit fold still discloses its pricing table, got %+v", dep.SupplyFoldBasis)
	}
}

// --- C2 witness: R5d 同频点跨核类 (waker and consumer at the SAME frequency) -

// The dep (waker) runs ~19ms at 2GHz on the SMALL cluster while the big
// cluster's FULL-TRACE fmax is 2.4GHz (cpu1's sample at ts=5.030, outside the
// window). Pre-CAP the pure frequency comparison read waker(2GHz) ≥
// consumer(2GHz) → EXACTLY 0. CAP §26 priced the class gap against the
// consumer's governed state: 19.0×(1−(2.0×1.0)/(2.0×2.53)) ≈ 11.49ms.
// R5 (§29.88.3/§29.88.12, 2026-07-15) EVOLUTION: the basis is the 全域最大核
// 最高频点 — the big cluster's FULL-FILE fmax, not the consumer's governed
// frequency: 19.0×(1−(2000000×1.0)/(2400000×2.53)) = 19.0×(1−2000/6072) =
// 19.0×0.670619 ≈ 12.742ms.
func TestWeakCoreDeficitSameFrequencyCrossClass(t *testing.T) {
	idx := buildTraceIndex(t, "cap_r5d_samefreq.systrace", `
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
      <idle>-0 (-----) [001] .... 4.900000: cpu_frequency: state=2000000 cpu_id=1
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.019500: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      <idle>-0 (-----) [001] .... 5.030000: cpu_frequency: state=2400000 cpu_id=1
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	var dep *WakeupCausalImpact
	for i := range chain.CausalImpacts {
		if chain.CausalImpacts[i].Thread.PID == 200 {
			dep = &chain.CausalImpacts[i]
		}
	}
	if dep == nil {
		t.Fatalf("dependency causal impact missing: %+v", chain.CausalImpacts)
	}
	t.Logf("CAP §26/R5 direction dump (同频点跨核类 form): gated running deficit pre-CAP=0.000 → now %.3f", dep.GatedRunningDeficitMs)
	if dep.GatedRunningDeficitMs < 12.5 || dep.GatedRunningDeficitMs > 13.0 {
		t.Fatalf("same-frequency cross-class deficit should be ≈12.742ms under the R5 global basis (pre-CAP: exactly 0), got %.3f", dep.GatedRunningDeficitMs)
	}
	if dep.GatedCapabilitySource != CoreCapabilitySourceDefault {
		t.Fatalf("R5d default-table pricing must disclose itself, got %q", dep.GatedCapabilitySource)
	}

	// Control (reversed classes): the waker on the BIG cluster at a frequency
	// whose equivalent capacity exceeds the small-cluster consumer's — the
	// floor keeps the deficit at zero (§26: 下限 0).
	control := buildTraceIndex(t, "cap_r5d_reversed.systrace", `
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000 cpu_id=7
      <idle>-0 (-----) [001] .... 4.900000: cpu_frequency: state=1000000 cpu_id=1
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.019500: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain2 := BuildWakeupChain(control, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	for i := range chain2.CausalImpacts {
		if chain2.CausalImpacts[i].Thread.PID == 200 && chain2.CausalImpacts[i].GatedRunningDeficitMs != 0 {
			t.Fatalf("big-class waker above the consumer's equivalent capacity must floor at 0, got %.3f", chain2.CausalImpacts[i].GatedRunningDeficitMs)
		}
	}
}

// --- C3: fail-loud freq_only end to end (>4 clusters → pure ratio + flag) ----

func TestSupplyFoldFreqOnlyDisclosureEndToEnd(t *testing.T) {
	idx := buildTraceIndex(t, "cap_freq_only.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [003] .... 4.910000: cpu_frequency: state=1200000 cpu_id=3
      <idle>-0 (-----) [004] .... 4.920000: cpu_frequency: state=1400000 cpu_id=4
      <idle>-0 (-----) [005] .... 4.930000: cpu_frequency: state=1600000 cpu_id=5
      <idle>-0 (-----) [006] .... 4.940000: cpu_frequency: state=1800000 cpu_id=6
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil {
		t.Fatalf("fold must run: %+v", dep)
	}
	// 5 distinct sampled clusters exceed the §26 table → fail-loud typed
	// freq_only; the numbers are the PURE frequency ratio against the
	// governed big fmax 1.8GHz: 9.9×(1−1/1.8) ≈ 4.4ms.
	if dep.SupplyFoldBasis.CapabilitySource != CoreCapabilitySourceFreqOnly {
		t.Fatalf(">4 clusters must fail loud to the typed freq_only disclosure, got %+v", dep.SupplyFoldBasis)
	}
	if dep.SupplyFoldDeficitMs < 4.2 || dep.SupplyFoldDeficitMs > 4.6 {
		t.Fatalf("freq_only fold must keep the pre-CAP pure ratio (≈4.4ms), got %.3f", dep.SupplyFoldDeficitMs)
	}
}

// --- 复核 F1: fold basis (fmax, cap) 同簇同源 ---------------------------------

// Probe A (复核 F1 witness): a four-cluster trace whose PRIME cluster has no
// window-governing samples (its only sample sits after the window). The
// pre-fix code mixed the governed big cluster's fmax with the capability
// ladder's prime cap and minted 9.9×(1−(3×2.53)/(3×3.036)) ≈ 1.650ms on a
// big core running AT ITS OWN FMAX — a CROSS-CLUSTER product.
// R5 (§29.88.3, 2026-07-15) EVOLUTION: the basis is the 全域最大核最高频点
// pair — the PRIME cluster with ITS OWN full-file fmax (4GHz, 3.036); 同簇
// 同源 holds (the F1 disease — mixing one cluster's fmax with another's cap
// — stays impossible), and the deficit is now the HONEST gap of a big core
// against the machine's global max core:
//
//	9.9 × (1 − (3000000×2.53)/(4000000×3.036)) = 9.9 × (1 − 0.625) ≈ 3.713ms
func TestSupplyFoldReferenceSameClusterProbeA(t *testing.T) {
	idx := buildTraceIndex(t, "cap_probe_a.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [004] .... 4.900000: cpu_frequency: state=2000000 cpu_id=4
      <idle>-0 (-----) [006] .... 4.900000: cpu_frequency: state=3000000 cpu_id=6
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [006] .... 5.000000: sched_switch: prev_comm=idle/6 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [006] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [006] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/6 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      <idle>-0 (-----) [007] .... 5.030000: cpu_frequency: state=4000000 cpu_id=7
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	basis := dep.SupplyFoldBasis
	if basis == nil || !basis.AllKnown() {
		t.Fatalf("fold must run on a fully-known basis: %+v", basis)
	}
	if basis.CapabilitySource != CoreCapabilitySourceDefault {
		t.Fatalf("four judged clusters must use the default table, got %+v", basis)
	}
	if basis.ReferenceClass != coreCapabilityClassPrime || basis.FmaxKHz != 4000000 {
		t.Fatalf("R5: the basis pair must be the global max cluster's own (4GHz, prime): %+v", basis)
	}
	// 3.7125 = 9.9 × (1 − 7590000/12144000); the pre-fix mixed-cluster
	// product minted ~1.650 and the pre-R5 big nomination minted exactly 0.
	if dep.SupplyFoldDeficitMs < 3.6 || dep.SupplyFoldDeficitMs > 3.83 {
		t.Fatalf("Probe A: big core vs global prime basis must fold ≈3.713ms, got %.3f", dep.SupplyFoldDeficitMs)
	}
}

// Probe B (复核 F1 witness): only the SMALL cluster has window-governing
// samples (the big cluster's sole sample sits after the window). The pre-fix
// code folded the small cluster's fmax against the big-class cap and minted
// 9.9×(1−1/2.53) ≈ 5.987ms while the verdict claimed 按大核满频 — a
// CROSS-CLUSTER product. The interim fix DEMOTED the reference to the
// governed small cluster (deficit 0).
// R5 (§29.88.3, 2026-07-15) EVOLUTION — demotion RETIRED: the basis is the
// 全域最大核最高频点 over FULL-FILE curves, so the big cluster's post-window
// 2.5GHz sample anchors it (window governance no longer starves the basis).
// 同簇同源 still holds (2.5GHz AND 2.53 both from the big cluster):
//
//	9.9 × (1 − (1800000×1.0)/(2500000×2.53)) = 9.9 × (1 − 0.284585) ≈ 7.083ms
func TestSupplyFoldReferenceDemotionProbeB(t *testing.T) {
	idx := buildTraceIndex(t, "cap_probe_b.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1800000 cpu_id=2
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      <idle>-0 (-----) [007] .... 5.030000: cpu_frequency: state=2500000 cpu_id=7
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	basis := dep.SupplyFoldBasis
	if basis == nil || !basis.AllKnown() {
		t.Fatalf("fold must run on a fully-known basis: %+v", basis)
	}
	if basis.ReferenceClass != coreCapabilityClassBig || basis.FmaxKHz != 2500000 {
		t.Fatalf("Probe B (R5): the basis pair must be the big cluster's own full-file (2.5GHz, big): %+v", basis)
	}
	// 7.0826 = 9.9 × (1 − 1800000/6325000); pre-fix mixed product ~5.987,
	// interim demotion 0 — both retired forms.
	if dep.SupplyFoldDeficitMs < 6.95 || dep.SupplyFoldDeficitMs > 7.2 {
		t.Fatalf("Probe B: small core vs global big basis must fold ≈7.083ms, got %.3f", dep.SupplyFoldDeficitMs)
	}
}

// R5 (§29.88.3, 2026-07-15) EVOLUTION: under the 全域最大核 basis the prime
// slice IS the reference cluster at its own fmax — ratio exactly 1, deficit
// exactly 0 (the affirmative fourth-branch shape). The reference is now
// PRIME (the §26-letter big nomination is superseded: R5's 「最大核」 means
// the machine's actual top cluster). The above-basis CLAMP survives for
// explicit-topology shapes where a slice's governed frequency can exceed the
// declared basis (supplyFoldSliceIdeal min(1,·) — unchanged).
func TestSupplyFoldPrimeSliceClampsAboveBigReference(t *testing.T) {
	idx := buildTraceIndex(t, "cap_prime_clamp.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000 cpu_id=2
      <idle>-0 (-----) [004] .... 4.900000: cpu_frequency: state=2000000 cpu_id=4
      <idle>-0 (-----) [006] .... 4.900000: cpu_frequency: state=3000000 cpu_id=6
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=4000000 cpu_id=7
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	basis := dep.SupplyFoldBasis
	if basis == nil || !basis.AllKnown() {
		t.Fatalf("fold must run on a fully-known basis: %+v", basis)
	}
	if basis.ReferenceClass != coreCapabilityClassPrime || basis.FmaxKHz != 4000000 {
		t.Fatalf("R5: the reference must be the global max cluster (prime, 4GHz): %+v", basis)
	}
	if dep.SupplyFoldDeficitMs != 0 || !floatNear(dep.SupplyFoldIdealMs, dep.RunningMs) {
		t.Fatalf("a prime slice at the global max basis folds at ratio 1 — zero deficit: deficit=%.3f ideal=%.3f running=%.3f",
			dep.SupplyFoldDeficitMs, dep.SupplyFoldIdealMs, dep.RunningMs)
	}
}

// --- 复核 F2: unknown membership on either side → pure frequency -------------

// An explicit topology that fails to declare the FASTEST core: the waker runs
// on the undeclared cpu7 (2.5GHz) while the consumer sits on the declared big
// cpu2 (2GHz). The pre-fix waker-side cap=1 fallback understated the waker
// (2.5 vs 2×2.53) and fabricated ~9.6ms of inversion deficit out of a true 0;
// F2 degrades the slice to the pure frequency comparison on BOTH sides
// (2.5 ≥ 2.0 → zero).
func TestWeakCoreDeficitUnknownWakerMembershipPureFrequency(t *testing.T) {
	idx := buildTraceIndex(t, "cap_f2_unknown_waker.systrace", `
      <idle>-0 (-----) [001] .... 4.900000: cpu_frequency: state=1000000 cpu_id=1
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=2000000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2500000 cpu_id=7
        app-100 (100) [002] .... 4.990000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [002] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=002
        dep-200 (100) [007] .... 5.019500: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [002] .... 5.020000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05,
		TraceFlavorHint: TraceFlavorHarmonyHitrace, CoreTopology: "small=1;big=2"})
	found := false
	for i := range chain.CausalImpacts {
		if chain.CausalImpacts[i].Thread.PID != 200 {
			continue
		}
		found = true
		t.Logf("CAP 复核 F2 direction dump: gated running deficit pre-fix≈9.6 (fabricated) → now %.3f", chain.CausalImpacts[i].GatedRunningDeficitMs)
		if chain.CausalImpacts[i].GatedRunningDeficitMs != 0 {
			t.Fatalf("unknown waker membership must degrade to the pure frequency comparison (2.5GHz ≥ 2.0GHz → 0), got %.3f",
				chain.CausalImpacts[i].GatedRunningDeficitMs)
		}
	}
	if !found {
		t.Fatalf("dependency causal impact missing (vacuous pass guard): %+v", chain.CausalImpacts)
	}
}

// --- R5 单基准单算法 (§29.88.3/§29.88.12, was 复核 F3 consumer ordering) -------

// EVOLUTION RECORD (R5, 2026-07-15): this seat pinned the retired
// downstream-consumer f×cap product selection (weakCoreDeficitMs). Under the
// unified single algorithm the running conversion reads the 全域最大核最高
// 频点 basis — trace-GLOBAL fmax, not the window-governed consumer state:
// cpu7's 3.1GHz sample sits at ts=5.100, OUTSIDE the 5.0..5.020 governance
// window, and MUST still anchor the basis (R6 规则4 — a window-local basis
// systematically under-states fmax). Hand computation:
//
//	clusters: {2} fmax 2.3GHz → small (cap 1.0); {7} fmax 3.1GHz → big (2.53)
//	basis    = 3100000 × 2.53 = 7843000 equivalent-kHz
//	slice    = 10ms on cpu2 governed at 2300000 (cap 1.0)
//	deficit  = 10 × (1 − 2300000/7843000) = 10 × 0.706745… ≈ 7.067ms
func TestUnifiedRunningConversionGlobalMaxBasis(t *testing.T) {
	idx := buildTraceIndex(t, "cap_f3_product_order.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=2300000 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=1200000 cpu_id=7
        mid-300 (300) [002] .... 4.980000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=mid next_pid=300 next_prio=30
        mid-300 (300) [002] .... 4.990000: sched_switch: prev_comm=mid prev_pid=300 prev_prio=30 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [007] .... 4.990000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [007] .... 5.020000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
      <idle>-0 (-----) [007] .... 5.100000: cpu_frequency: state=3100000 cpu_id=7
	`)
	cache := newChainQueryCache(idx, nil)
	capability := cache.coreCapability("")
	if capability.source != CoreCapabilitySourceDefault {
		t.Fatalf("fixture must judge two clusters, got %q", capability.source)
	}
	q := Query{TimeStart: 5.0, TimeEnd: 5.020}
	intervals := []Interval{{State: StateRunning, CPU: 2, CPUKnown: true, StartTs: 5.000, EndTs: 5.010, DurationMs: 10}}
	ideal, basis := cache.supplyFoldRunningIntervals(q, 5.0, 5.020, intervals)
	deficit := 10 - ideal
	if deficit < 7.0 || deficit > 7.14 {
		t.Fatalf("global-max basis fold must price ≈7.067ms (10×(1−2.3/7.843)), got %.3f", deficit)
	}
	if basis.FmaxKHz != 3100000 || basis.FmaxSource != SupplyFoldFmaxSourceObserved {
		t.Fatalf("basis must be the FULL-TRACE big-cluster fmax 3.1GHz (sample outside the window), got %+v", basis)
	}
	if basis.ReferenceClass != "big" {
		t.Fatalf("basis class must be the global max cluster's class, got %q", basis.ReferenceClass)
	}
	// 同源可互推 (§29.88.12): the gated lane's running component IS this fold
	// deficit — same number, no second algorithm.
	runnable, running := priorityInversionGatedMs(cache, []ThreadRef{{Comm: "app", PID: 100}}, intervals, deficit)
	if runnable != 0 || running != deficit {
		t.Fatalf("gated running component must be the unified fold deficit verbatim, got %.3f vs %.3f", running, deficit)
	}
}

// A DECLARED but never-sampled cluster is excluded from the ladder AND the
// count: three declared labels with two sampled clusters map small+big (never
// the 3-cluster row), and the unsampled cluster's members price unknown.
func TestCoreCapabilityDeclaredUnsampledClusterExcluded(t *testing.T) {
	tl := map[int][]freqSample{0: capTL(1000000), 7: capTL(3000000)}
	capability := resolveCoreCapability(parseClusterFreqDomains("small=0;middle=4;big=7"), tl)
	if capability.source != CoreCapabilitySourceDefault {
		t.Fatalf("two sampled clusters must judge, got %q", capability.source)
	}
	if got := capability.capabilityFor(0); got != coreCapabilityDefaultSmall {
		t.Fatalf("cpu0 = %v, want small %v", got, coreCapabilityDefaultSmall)
	}
	if got := capability.capabilityFor(7); got != coreCapabilityDefaultBig {
		t.Fatalf("cpu7 = %v, want big %v (the 2-cluster row, not the 3-cluster middle assignment)", got, coreCapabilityDefaultBig)
	}
	if _, known := capability.capabilityForKnown(4); known {
		t.Fatalf("the declared-but-unsampled cluster's member must price UNKNOWN")
	}
	if members := capability.classClusterMembers(coreCapabilityClassMiddle); len(members) != 0 {
		t.Fatalf("no middle class may exist on the 2-sampled-cluster mapping: %v", members)
	}
}

// R5e per-segment slicing composes with the judged capability map: a mid-
// interval DVFS jump on the small waker changes the discount RATE per segment
// (both segments contribute, at different rates) — a single-sample-per-
// interval mutant would price the whole interval at the first rate.
func TestWeakCoreGatePerSegmentJudgedCapability(t *testing.T) {
	idx := buildTraceIndex(t, "cap_f3_r5e_judged.systrace", `
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=800000 cpu_id=7
      <idle>-0 (-----) [001] .... 4.900000: cpu_frequency: state=2400000 cpu_id=1
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
      <idle>-0 (-----) [007] .... 5.010000: cpu_frequency: state=2000000 cpu_id=7
        dep-200 (100) [007] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.019500: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	found := false
	for i := range chain.CausalImpacts {
		if chain.CausalImpacts[i].Thread.PID != 200 {
			continue
		}
		found = true
		got := chain.CausalImpacts[i].PriorityInversionGatedMs
		// cpu7 small (fmax 2.0GHz) vs consumer cpu1 big (2.4×2.53=6.072):
		// slice1 10ms×(1−0.8/6.072)=8.683 + slice2 ~9ms×(1−2.0/6.072)=6.03
		// → ≈14.7-15.1; the single-rate mutant would price ~16.5.
		if got < 14.4 || got > 15.4 {
			t.Fatalf("per-segment discount under the judged map should compose to ≈14.8ms, got %.3f", got)
		}
		if chain.CausalImpacts[i].GatedCapabilitySource != CoreCapabilitySourceDefault {
			t.Fatalf("judged map must disclose default pricing, got %q", chain.CausalImpacts[i].GatedCapabilitySource)
		}
	}
	if !found {
		t.Fatalf("dependency causal impact missing (vacuous pass guard): %+v", chain.CausalImpacts)
	}
}
