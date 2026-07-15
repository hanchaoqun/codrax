package tracequery

import "testing"

// R5d (§7.30.1) positive cases: only wake-source runnable time, plus running
// time on a weaker-supplied CPU, counts as priority-inversion impact.

func TestPriorityInversionCandidateFromRunnableDominantDependency(t *testing.T) {
	idx := buildTraceIndex(t, "r5d_runnable.systrace", `
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.018000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
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
	if dep.DominantState != string(StateRunnable) {
		t.Fatalf("dependency should be runnable-dominant, got %+v", dep)
	}
	if !dep.PriorityInversionCandidate {
		t.Fatalf("runnable-dominant lower-priority dependency must be an inversion candidate: %+v", dep)
	}
	if dep.PriorityInversionGatedMs < 17 || dep.PriorityInversionGatedMs > 19.5 {
		t.Fatalf("gated impact should be the ~18ms runnable wait, got %.3f", dep.PriorityInversionGatedMs)
	}
}

func TestPriorityInversionWeakCoreRunningGate(t *testing.T) {
	trace := `
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=800000 cpu_id=7
      <idle>-0 (-----) [001] .... 4.900000: cpu_frequency: state=2000000 cpu_id=1
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.019500: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`
	idx := buildTraceIndex(t, "r5d_weakcore.systrace", trace)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	var dep *WakeupCausalImpact
	for i := range chain.CausalImpacts {
		if chain.CausalImpacts[i].Thread.PID == 200 {
			dep = &chain.CausalImpacts[i]
		}
	}
	if dep == nil || dep.DominantState != string(StateRunning) {
		t.Fatalf("dependency should be running-dominant, got %+v", chain.CausalImpacts)
	}
	if !dep.PriorityInversionCandidate {
		t.Fatalf("running on a weaker-supplied CPU (800MHz vs consumer 2GHz) must gate IN: %+v", dep)
	}
	// R5d-2 capacity-proportional estimate. CAP (§26) evolution: the derived
	// 2-cluster shape prices cpu7=小(×1.0) / cpu1=大(×2.53), so ~19.0ms
	// running × (1 − (0.8×1.0)/(2.0×2.53)) ≈ 16.0ms (pre-CAP pure frequency:
	// ≈11.4ms — 消费核在大核簇, the weak-core deficit grows). The whole slice
	// would still inflate the inversion (§7.30.2).
	t.Logf("CAP §26 direction dump (R5d weak-core form): gated pre-CAP≈11.4 → now %.3f", dep.PriorityInversionGatedMs)
	if dep.PriorityInversionGatedMs < 15.6 || dep.PriorityInversionGatedMs > 16.4 {
		t.Fatalf("gated impact should be the capacity-proportional ~16.0ms (CAP §26), got %.3f", dep.PriorityInversionGatedMs)
	}
	if dep.GatedCapabilitySource != CoreCapabilitySourceDefault {
		t.Fatalf("judged 2-cluster gate must disclose the default capability table, got %+v", dep.GatedCapabilitySource)
	}

	// Control: the same shape WITHOUT cpu_frequency samples must gate OUT —
	// unknown supply is never guessed (conservative precise fallback).
	noFreq := buildTraceIndex(t, "r5d_nofreq.systrace", `
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.019500: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain2 := BuildWakeupChain(noFreq, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	for i := range chain2.CausalImpacts {
		if chain2.CausalImpacts[i].Thread.PID == 200 {
			if chain2.CausalImpacts[i].PriorityInversionCandidate || chain2.CausalImpacts[i].PriorityInversionGatedMs != 0 {
				t.Fatalf("without frequency data the running gate must stay closed: %+v", chain2.CausalImpacts[i])
			}
		}
	}
}

// R5e (§7.30.2): in-window frequency changes are honored per segment, and a
// window that precedes the first cpu_frequency event falls back to the
// nearest later sample instead of reading as unknown supply.
func TestWeakCoreGatePerSegmentAndNearestFallback(t *testing.T) {
	// cpu7 runs at 800MHz until 5.010, then jumps to 2.4GHz: only the first
	// ~10ms slice contributes. R5 (§29.88.3, 2026-07-15) EVOLUTION: the rate
	// reads the 全域最高频点 basis (freq_only arm: full-file max over every
	// core = 2400000), not the retired downstream-consumer 2GHz —
	// 10×(1−800/2400) = 10×0.6667 ≈ 6.667ms; the second slice at 2.4GHz ==
	// basis contributes 0.
	//
	// CAP (§26, 2026-07-08) fixture evolution: a trailing post-window 2.4GHz
	// sample on cpu1 ties the two derived clusters' full-trace fmax — the
	// capability judgment fails loud to freq_only (簇结构不可判: no defensible
	// order on an fmax tie), so this pin keeps exercising the PURE per-segment
	// frequency behavior it exists for. It doubles as the tie→freq_only
	// witness; the R5e slicing rule itself is unchanged.
	midChange := buildTraceIndex(t, "r5e_midchange.systrace", `
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=800000 cpu_id=7
      <idle>-0 (-----) [001] .... 4.900000: cpu_frequency: state=2000000 cpu_id=1
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
      <idle>-0 (-----) [007] .... 5.010000: cpu_frequency: state=2400000 cpu_id=7
        dep-200 (100) [007] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.019500: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      <idle>-0 (-----) [001] .... 5.030000: cpu_frequency: state=2400000 cpu_id=1
	`)
	chain := BuildWakeupChain(midChange, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	foundMid := false
	for i := range chain.CausalImpacts {
		if chain.CausalImpacts[i].Thread.PID != 200 {
			continue
		}
		foundMid = true
		got := chain.CausalImpacts[i].PriorityInversionGatedMs
		if got < 6.5 || got > 6.85 {
			t.Fatalf("mid-interval frequency jump must be honored per segment (≈6.667ms low-freq deficit vs the 2.4GHz global basis), got %.3f", got)
		}
		// CAP (§26 C1 fail-loud): the fmax tie must have demoted the
		// capability judgment to the typed freq_only disclosure.
		if chain.CausalImpacts[i].GatedCapabilitySource != CoreCapabilitySourceFreqOnly {
			t.Fatalf("tied cluster fmax must fail loud to freq_only, got %q", chain.CausalImpacts[i].GatedCapabilitySource)
		}
	}
	if !foundMid {
		t.Fatalf("dependency causal impact missing (vacuous pass guard): %+v", chain.CausalImpacts)
	}

	// R5 (§29.88.12 单基准单算法, 2026-07-15) EVOLUTION RECORD — the
	// nearest-LATER fallback arm INVERTED: the only samples appear AFTER the
	// governance window (first change at 5.030 > window end 5.020). The
	// retired consumer-core algorithm guessed the pre-first-witness state
	// from the later sample (≈16.0ms deficit); the unified fold applies the
	// adjudicated absence discipline (§29.11 真缺失 direction analysis:
	// carrying a value BACKWARDS from a later sample fabricates state the
	// trace never witnessed) — the slice books UNKNOWN, folds at ratio 1 and
	// mints ZERO deficit (下界: 频率缺失段计 0, never a guess). The R5e
	// head rule survives where it belongs: a window whose FIRST governing
	// sample sits inside it still prices pre-sample slices from that sample
	// (governedFrequencyAt), pinned by the fold governance tests.
	lateSamples := buildTraceIndex(t, "r5e_latesample.systrace", `
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.000000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.019500: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      <idle>-0 (-----) [007] .... 5.030000: cpu_frequency: state=800000 cpu_id=7
      <idle>-0 (-----) [001] .... 5.030000: cpu_frequency: state=2000000 cpu_id=1
	`)
	chain2 := BuildWakeupChain(lateSamples, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	foundLate := false
	for i := range chain2.CausalImpacts {
		if chain2.CausalImpacts[i].Thread.PID != 200 {
			continue
		}
		foundLate = true
		got := chain2.CausalImpacts[i].PriorityInversionGatedMs
		// Post-window-only samples = 真缺失 for the governance window: the
		// unified fold books UNKNOWN and mints no deficit (absence never
		// guesses; the retired consumer algorithm minted ≈16.0ms here from a
		// carried-back guess).
		if got != 0 {
			t.Fatalf("post-window-only samples must stay UNKNOWN basis with zero deficit (R5 unified absence discipline), got %.3f", got)
		}
	}
	if !foundLate {
		t.Fatalf("dependency causal impact missing (vacuous pass guard): %+v", chain2.CausalImpacts)
	}
}

// §7.30.3 D3: the gated impact publishes its composition — runnable time in
// full plus the capacity-discounted weak-core running deficit — as separate
// typed fields whose sum IS the gated total. Mixed shape: dep is runnable for
// ~10ms, then runs ~9.5ms on a weak core (800MHz vs the consumer's 2GHz).
func TestPriorityInversionGatedComponentsSplit(t *testing.T) {
	idx := buildTraceIndex(t, "d3_mixed.systrace", `
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=800000 cpu_id=7
      <idle>-0 (-----) [001] .... 4.900000: cpu_frequency: state=2000000 cpu_id=1
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.010000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.019500: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
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
	if dep.GatedRunnableMs < 9.5 || dep.GatedRunnableMs > 10.5 {
		t.Fatalf("gated runnable component should be the ~10ms wait, got %.3f", dep.GatedRunnableMs)
	}
	// CAP (§26) evolution: ~9.0ms running × (1 − (0.8×1.0)/(2.0×2.53)) ≈
	// 7.58ms discounted deficit (pre-CAP pure frequency ≈5.4ms).
	if dep.GatedRunningDeficitMs < 7.3 || dep.GatedRunningDeficitMs > 7.9 {
		t.Fatalf("gated running deficit should be the ~7.58ms discount (CAP §26), got %.3f", dep.GatedRunningDeficitMs)
	}
	if got, want := dep.PriorityInversionGatedMs, dep.GatedRunnableMs+dep.GatedRunningDeficitMs; got != want {
		t.Fatalf("gated total must equal the component sum: %.6f != %.6f", got, want)
	}
	if !dep.PriorityInversionCandidate {
		t.Fatalf("mixed runnable+weak-core dependency must stay an inversion candidate: %+v", dep)
	}
}
