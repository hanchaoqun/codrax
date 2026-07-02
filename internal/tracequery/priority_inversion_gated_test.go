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
	if dep.PriorityInversionGatedMs < 17 || dep.PriorityInversionGatedMs > 20 {
		t.Fatalf("gated impact should be the ~19.5ms weak-core running time, got %.3f", dep.PriorityInversionGatedMs)
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
