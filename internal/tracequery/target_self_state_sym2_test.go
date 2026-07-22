package tracequery

// target_self_state_sym2_test.go — SYM-2 批 engine pins (ledger
// real_trace_campaign_20260705.md §24.17, user ruling 2026-07-08, revising the
// §24.13 裁定一 scope):
//
//   R1  predicate narrowing — subject==target ∧ 等待症状族 demotes; the 自因
//       可拆解族 (runnable / running / IO / D-state) competes, takes election
//       slots, rides co-primary and may be crowned (see the amended pins in
//       target_self_state_sym_test.go).
//   R2  「(优先级低于RT)」typed disclosure — stampRunnableSelfBelowRTPreempted
//       mints only on SELF runnable-family rows with the exact typed evidence
//       pair (target class ohos_cfs + an ohos_rt competitor whose running
//       OVERLAPPED the wait on the same CPU); every other shape stamps
//       nothing (absence never guesses).
//
// 突变自查 (recorded): (M1) reverting the skip arm to全量 subject==target →
// TestSYMSelfWaitSymptomFamilyDemotesSelfCauseFamilyCompetes bites; (M2)
// mis-assigning a registry lane / hand-listing a wrong family →
// TestSYM2WaitSymptomClosedSetIsRegistryLane bites; (M4) dropping the
// RT-competitor/CFS-target requirement from the stamp → the negative arms of
// TestSYM2BelowRTPreemptedStamp bite.

import "testing"

func sym2RunnableCtx(target ThreadRef, targetClass, competitorClass string, overlapMs float64) []RunnableContextSummary {
	return []RunnableContextSummary{{
		Thread:        target,
		CPU:           2,
		PriorityClass: targetClass,
		SameCPUTopRunning: []ThreadDuration{{
			Thread:        ThreadRef{Comm: "rt-worker", PID: 9001},
			DurationMs:    overlapMs,
			PriorityClass: competitorClass,
		}},
	}}
}

func TestSYM2BelowRTPreemptedStamp(t *testing.T) {
	target := ThreadRef{Comm: "ui", PID: 100}
	mk := func(typ string, self bool) []RootCauseRankItem {
		return []RootCauseRankItem{{
			Type: typ, Thread: target, SubjectIsAnalysisTarget: self, ImpactMs: 5,
			runnableCPU: 2, runnableCPUKnown: true,
		}}
	}

	// Positive: self runnable row + CFS target + RT competitor overlap.
	items := mk("runnable_wait", true)
	stampRunnableSelfBelowRTPreempted(items, sym2RunnableCtx(target, "ohos_cfs", "ohos_rt", 3.2))
	if !items[0].RunnableBelowRTPreempted {
		t.Fatalf("the exact typed evidence pair must stamp the disclosure: %+v", items[0])
	}
	// The whole closed runnable family is covered.
	for _, typ := range []string{"fragmented_runnable_wait", "scheduler_latency"} {
		items := mk(typ, true)
		stampRunnableSelfBelowRTPreempted(items, sym2RunnableCtx(target, "ohos_cfs", "ohos_rt", 3.2))
		if !items[0].RunnableBelowRTPreempted {
			t.Fatalf("%s must ride the same runnable-family stamp: %+v", typ, items[0])
		}
	}

	// Negative: a NON-self row with identical evidence never stamps.
	items = mk("runnable_wait", false)
	stampRunnableSelfBelowRTPreempted(items, sym2RunnableCtx(target, "ohos_cfs", "ohos_rt", 3.2))
	if items[0].RunnableBelowRTPreempted {
		t.Fatalf("the disclosure is defined for the analysis target only: %+v", items[0])
	}
	// Negative: a CFS competitor is not RT preemption.
	items = mk("runnable_wait", true)
	stampRunnableSelfBelowRTPreempted(items, sym2RunnableCtx(target, "ohos_cfs", "ohos_cfs", 3.2))
	if items[0].RunnableBelowRTPreempted {
		t.Fatalf("a same-class competitor must not mint the below-RT claim: %+v", items[0])
	}
	// Negative: an RT-class target is not below RT.
	items = mk("runnable_wait", true)
	stampRunnableSelfBelowRTPreempted(items, sym2RunnableCtx(target, "ohos_rt", "ohos_rt", 3.2))
	if items[0].RunnableBelowRTPreempted {
		t.Fatalf("an RT-class target is not 低于RT: %+v", items[0])
	}
	// Negative: Android/generic raw priorities carry no class → nothing stamps.
	items = mk("runnable_wait", true)
	stampRunnableSelfBelowRTPreempted(items, sym2RunnableCtx(target, "android_raw_scheduler_prio", "android_raw_scheduler_prio", 3.2))
	if items[0].RunnableBelowRTPreempted {
		t.Fatalf("non-Harmony priority semantics must stamp nothing: %+v", items[0])
	}
	// Negative: a zero-overlap competitor is background, not displacement.
	items = mk("runnable_wait", true)
	stampRunnableSelfBelowRTPreempted(items, sym2RunnableCtx(target, "ohos_cfs", "ohos_rt", 0))
	if items[0].RunnableBelowRTPreempted {
		t.Fatalf("window-total background load must not mint the preemption claim: %+v", items[0])
	}
	// Negative: a non-runnable self row (running/binder) never stamps.
	for _, typ := range []string{"running", "binder_wait", "io_latency"} {
		items := mk(typ, true)
		stampRunnableSelfBelowRTPreempted(items, sym2RunnableCtx(target, "ohos_cfs", "ohos_rt", 3.2))
		if items[0].RunnableBelowRTPreempted {
			t.Fatalf("%s is outside the runnable-family closed set: %+v", typ, items[0])
		}
	}
}

func TestSYM2SelfRunnableCanBeCrownedEnginePrimary(t *testing.T) {
	// §24.17 witness half (engine): the cmp_78 shape — the self binder wait
	// stays demoted while the self runnable row heads the ladder as primary
	// (lead 加冕形 "主根因: <线程> 调度压力(runnable 全额 X ms)" downstream).
	items := []RootCauseRankItem{
		{Type: "binder_wait", SubjectIsAnalysisTarget: true, ImpactMs: 90, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "runnable_wait", SubjectIsAnalysisTarget: true, ImpactMs: 40, RunnableMs: 40, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "workqueue_activity", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
	}
	assignRootCauseRankOrdinalsAndTiers(items)
	// EVOLUTION RECORD (G9, §28.1 user ruling 2026-07-09): was Rank==1 /
	// Rank==2 (§24.20 榜位照发) — the demoted binder symptom row now carries
	// Rank=0 and the competing self runnable row takes ordinal #1 (ordinals
	// contiguous over board-visible rows only).
	if items[0].Tier != RootCauseTierTargetSelfState || items[0].Rank != 0 {
		t.Fatalf("binder 等对端仍降道 (G9: 不占序数): %+v", items[0])
	}
	if items[1].Tier != "primary" || items[1].Rank != 1 {
		t.Fatalf("the self runnable row must take the primary slot and ordinal #1: %+v", items[1])
	}
	if items[2].Tier != "secondary" || items[2].Rank != 2 {
		t.Fatalf("the ladder below shifts normally: %+v", items[2])
	}
}
