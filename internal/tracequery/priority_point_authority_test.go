package tracequery

import (
	"fmt"
	"testing"
)

// These tests intentionally enter through BuildIndex (via buildTraceIndex)
// and assert only published chain/rank behavior.  A nearby priority sample is
// useful display context, but it must never become a hard priority relation
// unless the priority is exact at the decision point or stable across the
// complete interval being charged.

func ppaQuery() Query {
	return Query{
		PID:             100,
		TimeStart:       4.998,
		TimeEnd:         5.012,
		MaxDepth:        4,
		MaxBranches:     8,
		MinDurationMs:   0.001,
		Limit:           16,
		TraceFlavorHint: TraceFlavorHarmonyHitrace,
	}
}

func ppaFindEdge(t *testing.T, chain ChainResult, wakerPID, wakeePID int) WakeupEdge {
	t.Helper()
	for _, edge := range chain.Edges {
		if edge.Waker.PID == wakerPID && edge.Wakee.PID == wakeePID {
			return edge
		}
	}
	t.Fatalf("required wakeup edge %d->%d is absent (vacuous-pass guard): %+v", wakerPID, wakeePID, chain)
	return WakeupEdge{}
}

func ppaFindImpact(t *testing.T, chain ChainResult, pid int) WakeupCausalImpact {
	t.Helper()
	for _, impact := range chain.CausalImpacts {
		if impact.Thread.PID == pid {
			return impact
		}
	}
	t.Fatalf("required causal impact for pid=%d is absent (vacuous-pass guard): %+v", pid, chain)
	return WakeupCausalImpact{}
}

func ppaRankItems(rank RootCauseRankResult) []RootCauseRankItem {
	items := make([]RootCauseRankItem, 0, len(rank.Items)+len(rank.AbsorbedItems))
	items = append(items, rank.Items...)
	items = append(items, rank.AbsorbedItems...)
	return items
}

func ppaFindRankItem(rank RootCauseRankResult, pid int, typ string) (RootCauseRankItem, bool) {
	for _, item := range ppaRankItems(rank) {
		if item.Thread.PID == pid && item.Type == typ {
			return item, true
		}
	}
	return RootCauseRankItem{}, false
}

func ppaAssertNoHardEdgePriority(t *testing.T, edge WakeupEdge) {
	t.Helper()
	if edge.PriorityRelation != "" || edge.PriorityInversionCandidate {
		t.Fatalf("advisory priority minted a hard wakeup relation: %+v", edge)
	}
	if edge.WakeePriority != 159 || edge.WakeePriorityClass != "ohos_rt" {
		t.Fatalf("negative control lost the exact wakee priority: %+v", edge)
	}
}

func TestPriorityPointAuthorityEdgeSafetyMatrix(t *testing.T) {
	tests := []struct {
		name         string
		trace        string
		wantRelation string
		wantHard     bool
	}{
		{
			name: "future nearest is display-only",
			trace: `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
       idle-0 (0) [002] .... 5.005500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
       idle-0 (0) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
`,
		},
		{
			name: "one-sided point evidence is advisory",
			trace: `
       idle-0 (0) [002] .... 4.999000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
       idle-0 (0) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
`,
		},
		{
			name: "conflicting point endpoints are advisory",
			trace: `
       idle-0 (0) [002] .... 4.999000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
     worker-200 (200) [002] .... 5.005500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=52 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       idle-0 (0) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
`,
		},
		{
			name: "matching point endpoints are hard",
			trace: `
       idle-0 (0) [002] .... 4.999000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
     worker-200 (200) [002] .... 5.005500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       idle-0 (0) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
`,
			wantRelation: "lower_priority_waker",
			wantHard:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, "priority-point-edge.ftrace", tc.trace)
			chain := BuildWakeupChain(idx, ppaQuery())
			edge := ppaFindEdge(t, chain, 200, 100)
			if !tc.wantHard {
				ppaAssertNoHardEdgePriority(t, edge)
				if edge.WakerPriority <= 0 || edge.WakerPrioritySource != string(priorityCaliberAdvisoryNearest) {
					t.Fatalf("nearby value was not retained as explicitly advisory context: %+v", edge)
				}
				return
			}
			if edge.PriorityRelation != tc.wantRelation || !edge.PriorityInversionCandidate {
				t.Fatalf("closed stable point evidence did not mint the expected hard edge: %+v", edge)
			}
			if edge.WakerPriority != 20 || edge.WakerPriorityClass != "ohos_cfs" || edge.WakeePriority != 159 || edge.WakeePriorityClass != "ohos_rt" {
				t.Fatalf("stable endpoint priority/class publication drifted: %+v", edge)
			}
			if edge.WakerPrioritySource != string(priorityCaliberClosedRangeStable) || edge.WakeePriorityAuthority != string(priorityCaliberExactAtPoint) || edge.PriorityRelationCaliber != string(priorityCaliberClosedRangeStable) {
				t.Fatalf("hard edge did not disclose its exact/stable proof caliber: %+v", edge)
			}
		})
	}
}

func TestPriorityPointAuthorityMutationRowsPoisonHardPointEvidence(t *testing.T) {
	tests := []struct {
		name     string
		mutation string
	}{
		{
			name:     "sched_pi_setprio subject poison",
			mutation: "       boost-9 (9) [003] .... 5.003000: sched_pi_setprio: comm=worker pid=200 oldprio=20 newprio=52",
		},
		{
			name:     "binder_set_priority global poison",
			mutation: "      binder-8 (8) [003] .... 5.003000: binder_set_priority: pid=200 old_prio=20 new_prio=52",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trace := fmt.Sprintf(`
       idle-0 (0) [002] .... 4.999000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
%s
     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
     worker-200 (200) [002] .... 5.005500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       idle-0 (0) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
`, tc.mutation)
			idx := buildTraceIndex(t, "priority-mutation-poison.ftrace", trace)
			mutations := 0
			for _, event := range idx.Events {
				if event.Type == EventPriorityMutation {
					mutations++
				}
			}
			if mutations != 1 {
				t.Fatalf("mutation fixture was not admitted as one exact poison row: count=%d events=%+v", mutations, idx.Events)
			}
			edge := ppaFindEdge(t, BuildWakeupChain(idx, ppaQuery()), 200, 100)
			ppaAssertNoHardEdgePriority(t, edge)
			if edge.WakerPriority <= 0 || edge.WakerPrioritySource != string(priorityCaliberAdvisoryNearest) {
				t.Fatalf("poisoned stable-looking endpoints must degrade to advisory context: %+v", edge)
			}
		})
	}
}

func TestPriorityPointAuthorityHarmonyBoundaryMatrix(t *testing.T) {
	tests := []struct {
		prio          int
		wantClass     string
		wantRelation  string
		wantCandidate bool
	}{
		{prio: 40, wantClass: "ohos_cfs", wantRelation: "lower_priority_waker", wantCandidate: true},
		{prio: 41, wantClass: "ohos_rt", wantRelation: "lower_priority_waker", wantCandidate: true},
		{prio: 139, wantClass: "ohos_rt", wantRelation: "lower_priority_waker", wantCandidate: true},
		{prio: 140, wantClass: "ohos_rt", wantRelation: "lower_priority_waker", wantCandidate: true},
		{prio: 159, wantClass: "ohos_rt", wantRelation: "same_priority", wantCandidate: false},
		{prio: 160, wantClass: "system_or_kernel", wantRelation: "raw_priority_uninterpreted", wantCandidate: false},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("prio_%d", tc.prio), func(t *testing.T) {
			trace := fmt.Sprintf(`
       idle-0 (0) [002] .... 4.999000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=%d
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
     worker-200 (200) [002] .... 5.005500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=%d prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       idle-0 (0) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
`, tc.prio, tc.prio)
			edge := ppaFindEdge(t, BuildWakeupChain(buildTraceIndex(t, "harmony-boundary.ftrace", trace), ppaQuery()), 200, 100)
			if edge.WakerPriority != tc.prio || edge.WakerPriorityClass != tc.wantClass || edge.WakeePriority != 159 || edge.WakeePriorityClass != "ohos_rt" {
				t.Fatalf("Harmony boundary publication drifted: %+v", edge)
			}
			if edge.PriorityRelation != tc.wantRelation || edge.PriorityInversionCandidate != tc.wantCandidate {
				t.Fatalf("Harmony boundary hard relation drifted: %+v", edge)
			}
			if edge.WakerPrioritySource != string(priorityCaliberClosedRangeStable) || edge.WakeePriorityAuthority != string(priorityCaliberExactAtPoint) || edge.PriorityRelationCaliber != string(priorityCaliberClosedRangeStable) {
				t.Fatalf("Harmony boundary lost its point-proof provenance: %+v", edge)
			}
		})
	}
}

func TestPriorityPointAuthorityGenerationAndRename(t *testing.T) {
	t.Run("reused TID cannot inherit the old occupant priority", func(t *testing.T) {
		idx := buildTraceIndex(t, "priority-reuse.ftrace", `
       idle-0 (0) [002] .... 4.900000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 4.950000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=X ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
       idle-0 (0) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
`)
		if boundaries, capped := threadGenerationBoundaries(idx, 200); capped || len(boundaries) == 0 {
			t.Fatalf("fixture lacks its exact lifecycle boundary: capped=%t boundaries=%+v", capped, boundaries)
		}
		edge := ppaFindEdge(t, BuildWakeupChain(idx, ppaQuery()), 200, 100)
		ppaAssertNoHardEdgePriority(t, edge)
	})

	t.Run("same-generation rename preserves stable authority", func(t *testing.T) {
		idx := buildTraceIndex(t, "priority-rename.ftrace", `
       idle-0 (0) [002] .... 4.999000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=200 next_prio=20
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        old-200 (200) [002] .... 5.002000: task_rename: pid=200 oldcomm=old newcomm=new oom_score_adj=0
        new-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
        new-200 (200) [002] .... 5.005500: sched_switch: prev_comm=new prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       idle-0 (0) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
`)
		edge := ppaFindEdge(t, BuildWakeupChain(idx, ppaQuery()), 200, 100)
		if edge.PriorityRelation != "lower_priority_waker" || !edge.PriorityInversionCandidate {
			t.Fatalf("display rename split one generation's stable priority evidence: %+v", edge)
		}
		if edge.WakerPrioritySource != string(priorityCaliberClosedRangeStable) {
			t.Fatalf("display rename degraded a stable range to nearest context: %+v", edge)
		}
	})
}

func TestPriorityRangeAuthorityCrossCPUChainAndLocalBound(t *testing.T) {
	t.Run("cross-CPU dependency remains a hard inversion", func(t *testing.T) {
		idx := buildTraceIndex(t, "priority-cross-cpu-chain.ftrace", `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       helper-9 (9) [003] .... 5.001000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002
       idle-0 (0) [002] .... 5.006000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 5.007000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
     worker-200 (200) [002] .... 5.007500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       idle-0 (0) [001] .... 5.008000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
`)
		q := ppaQuery()
		chain := BuildWakeupChain(idx, q)
		edge := ppaFindEdge(t, chain, 200, 100)
		if edge.PriorityRelation != "lower_priority_waker" || !edge.PriorityInversionCandidate {
			t.Fatalf("cross-CPU edge was narrowed to a same-CPU-only notion: %+v", edge)
		}
		if edge.WakerPrioritySource != string(priorityCaliberClosedRangeStable) || edge.WakeePriorityAuthority != string(priorityCaliberExactAtPoint) {
			t.Fatalf("cross-CPU edge lost its exact/stable point proof: %+v", edge)
		}
		impact := ppaFindImpact(t, chain, 200)
		if impact.RunnableMs < 4.5 || !impact.PriorityInversionCandidate || impact.PriorityInversionGatedMs <= 0 {
			t.Fatalf("cross-CPU runnable dependency lost its hard gated inversion: %+v", impact)
		}
		rank := BuildRootCauseRank(idx, q)
		item, ok := ppaFindRankItem(rank, 200, "priority_inversion_candidate")
		if !ok || item.EffectiveImpactMs <= 0 || item.ChainRelevance != "on_chain" {
			t.Fatalf("cross-CPU dependency did not participate in root-cause rank: rank=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
		}
	})

	t.Run("a local lower-priority observation cannot inflate the whole chain interval", func(t *testing.T) {
		idx := buildTraceIndex(t, "priority-chain-local-only.ftrace", `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       helper-9 (9) [003] .... 5.001000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002
       boost-9 (9) [003] .... 5.003000: sched_pi_setprio: comm=worker pid=200 oldprio=20 newprio=159
       idle-0 (0) [002] .... 5.009000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=159
     worker-200 (200) [002] .... 5.010000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001
     worker-200 (200) [002] .... 5.010500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=159 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       idle-0 (0) [001] .... 5.011000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
`)
		q := ppaQuery()
		chain := BuildWakeupChain(idx, q)
		impact := ppaFindImpact(t, chain, 200)
		if impact.RunnableMs < 7.5 {
			t.Fatalf("fixture lost its long runnable dependency: %+v", impact)
		}
		// The only lower-priority portion is 5.001..5.003 (~2ms). A
		// conservative implementation may fail the whole interval closed;
		// if it elects to retain the provable prefix it must charge no more
		// than that prefix, never the ~8ms envelope.
		if impact.PriorityInversionCandidate && impact.PriorityInversionGatedMs > 2.001 {
			t.Fatalf("local lower-priority evidence inflated the whole causal interval: %+v", impact)
		}
		rank := BuildRootCauseRank(idx, q)
		for _, item := range ppaRankItems(rank) {
			isInversion := item.Type == "priority_inversion_candidate" || item.Type == "priority_inversion_runnable_wait"
			if item.Thread.PID == 200 && isInversion && item.EffectiveImpactMs > 2.001 {
				t.Fatalf("local lower-priority evidence inflated the root-cause rank seat: %+v", item)
			}
		}
	})
}

func TestPriorityRangeAuthorityDirectRunnableMatrix(t *testing.T) {
	tests := []struct {
		name          string
		startTarget   int
		endTarget     int
		startRunning  int
		endRunning    int
		wantInversion bool
	}{
		{name: "stable same-CPU direct overlap", startTarget: 159, endTarget: 159, startRunning: 20, endRunning: 20, wantInversion: true},
		{name: "running endpoint conflict", startTarget: 159, endTarget: 159, startRunning: 20, endRunning: 159},
		{name: "runnable endpoint conflict", startTarget: 159, endTarget: 20, startRunning: 20, endRunning: 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trace := fmt.Sprintf(`
       idle-0 (0) [001] .... 4.999000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=%d
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=%d prev_state=R+ ==> next_comm=rival next_pid=300 next_prio=%d
      rival-300 (300) [001] .... 5.005000: sched_switch: prev_comm=rival prev_pid=300 prev_prio=%d prev_state=R+ ==> next_comm=app next_pid=100 next_prio=%d
        app-100 (100) [001] .... 5.006000: sched_switch: prev_comm=app prev_pid=100 prev_prio=%d prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`, tc.startTarget, tc.startTarget, tc.startRunning, tc.endRunning, tc.endTarget, tc.endTarget)
			idx := buildTraceIndex(t, "priority-direct-runnable.ftrace", trace)
			q := ppaQuery()
			rank := BuildRootCauseRank(idx, q)
			item, gotInversion := ppaFindRankItem(rank, 100, "priority_inversion_runnable_wait")
			if gotInversion != tc.wantInversion {
				t.Fatalf("direct runnable inversion verdict=%t want=%t item=%+v rank=%+v absorbed=%+v", gotInversion, tc.wantInversion, item, rank.Items, rank.AbsorbedItems)
			}
			stats := ComputeWindowStats(idx, q)
			var runnableMs float64
			for _, row := range stats.RunnableTop {
				if row.Thread.PID == 100 {
					runnableMs += row.DurationMs
				}
			}
			if runnableMs < 4.5 {
				t.Fatalf("fixture lost its direct runnable account (vacuous-pass guard): %+v", stats.RunnableTop)
			}
			if tc.wantInversion && (item.EffectiveImpactMs < 4.5 || item.GatedRunnableMs < 4.5) {
				t.Fatalf("stable direct overlap lost its measured effective account: %+v", item)
			}
		})
	}
}

func TestPriorityRangeVerdictProvesPhysicalSegmentBeforeNumericClip(t *testing.T) {
	base := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		{Line: 10, Ts: 5.000, Type: EventSchedSwitch, NextPID: 100, NextPrio: 159},
		{Line: 40, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 100, PrevPrio: 159, PrevState: "R"},
	}}
	td := ThreadDuration{
		Thread: ThreadRef{PID: 100}, StartTs: 5.002, EndTs: 5.008, DurationMs: 6,
		priorityRangeStartTs: 5.000, priorityRangeEndTs: 5.010,
		priorityRangeStartLine: 10, priorityRangeEndLine: 40, priorityRangeExact: true,
	}
	if got := priorityRangeVerdictForDuration(newPriorityPointAuthority(base), 100, td, 5.002, 5.008); !got.hardEvidence() {
		t.Fatalf("clean physical segment lost its closed proof: %+v", got)
	}

	// The mutation and the post-mutation reassertion share one timestamp but
	// have distinct physical line order. The post-mutation subrange is stable;
	// pairing its timestamps with the original line-10 endpoint would create a
	// non-physical tuple and incorrectly rescue the segment that crossed the
	// mutation. The complete original segment must therefore fail closed.
	crossing := &Index{TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		{Line: 10, Ts: 5.000, Type: EventSchedSwitch, NextPID: 100, NextPrio: 159},
		{Line: 15, Ts: 5.001, Type: EventPriorityMutation, Name: "sched_pi_setprio", WakeePID: 100},
		{Line: 20, Ts: 5.001, Type: EventSchedSwitch, NextPID: 100, NextPrio: 159},
		{Line: 40, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 100, PrevPrio: 159, PrevState: "R"},
	}}
	authority := newPriorityPointAuthority(crossing)
	if postMutation := priorityRangeVerdictForDuration(authority, 100, ThreadDuration{
		Thread: ThreadRef{PID: 100}, StartTs: 5.002, EndTs: 5.008, DurationMs: 6,
		priorityRangeStartTs: 5.001, priorityRangeEndTs: 5.010,
		priorityRangeStartLine: 20, priorityRangeEndLine: 40, priorityRangeExact: true,
	}, 5.002, 5.008); !postMutation.hardEvidence() {
		t.Fatalf("fixture lacks its stable post-mutation control range: %+v", postMutation)
	}
	if got := priorityRangeVerdictForDuration(authority, 100, td, 5.002, 5.008); got.hardEvidence() {
		t.Fatalf("numeric overlap rescued a physical segment that crossed a priority mutation: %+v", got)
	}
}

func TestDirectRunnableInversionCannotJoinPriorityProofAcrossArtifacts(t *testing.T) {
	idx := &Index{
		TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: "/trace/target.systrace", VirtualLineBase: 0, LocalLineCount: 80, CausalCompatible: true, timestampOrder: TraceTimestampOrderMonotonic},
			{SourcePath: "/trace/competitor.systrace", VirtualLineBase: 100, LocalLineCount: 80, CausalCompatible: true, timestampOrder: TraceTimestampOrderMonotonic},
		},
		Events: []Event{
			{Line: 10, Ts: 5.000, Type: EventSchedSwitch, NextPID: 100, NextPrio: 159},
			{Line: 110, Ts: 5.000, Type: EventSchedSwitch, NextPID: 200, NextPrio: 20},
			{Line: 40, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 100, PrevPrio: 159, PrevState: "R"},
			{Line: 140, Ts: 5.010, Type: EventSchedSwitch, PrevPID: 200, PrevPrio: 20, PrevState: "R"},
		},
	}
	target := ThreadDuration{
		Thread: ThreadRef{Comm: "app", PID: 100}, CPU: 2,
		StartTs: 5.002, EndTs: 5.008, DurationMs: 6,
		priorityRangeStartTs: 5.000, priorityRangeEndTs: 5.010,
		priorityRangeStartLine: 10, priorityRangeEndLine: 40, priorityRangeExact: true,
	}
	competitor := ThreadDuration{
		Thread: ThreadRef{Comm: "worker", PID: 200}, CPU: 2,
		StartTs: 5.002, EndTs: 5.008, DurationMs: 6,
		priorityRangeStartTs: 5.000, priorityRangeEndTs: 5.010,
		priorityRangeStartLine: 110, priorityRangeEndLine: 140, priorityRangeExact: true,
	}
	authority := newPriorityPointAuthority(idx)
	targetVerdict := priorityRangeVerdictForDuration(authority, 100, target, 5.002, 5.008)
	competitorVerdict := priorityRangeVerdictForDuration(authority, 200, competitor, 5.002, 5.008)
	if !targetVerdict.hardEvidence() || !competitorVerdict.hardEvidence() ||
		targetVerdict.Source == competitorVerdict.Source {
		t.Fatalf("fixture lacks two individually hard but physically distinct proofs: target=%+v competitor=%+v", targetVerdict, competitorVerdict)
	}
	stats := WindowStats{RunnableContext: []RunnableContextSummary{{
		Thread: target.Thread, CPU: 2, RunnableWaitMs: 6,
		sameCPURunningSegments: []ThreadDuration{competitor},
	}}}
	item := RootCauseRankItem{}
	applyRunnableTopPriorityInversionScopes(idx, Query{TraceFlavor: TraceFlavorHarmonyHitrace}, stats, target, []ThreadDuration{target}, nil, &item)
	if item.Type == "priority_inversion_runnable_wait" || item.EffectiveImpactMs != 0 || item.GatedRunnableMs != 0 {
		t.Fatalf("two bundle children were joined into a fabricated same-CPU inversion: %+v", item)
	}
}
