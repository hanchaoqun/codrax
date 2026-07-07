package tracequery

// wakeup_causal_effective_mirror_ptv5_test.go — 复核 Med (2026-07-06, PTV5
// #68 Q1 真镜像): the exported per-impact effective-attribution helper MUST
// publish the SAME number the rank lane publishes for the same input —
// two lanes, one implementation-equivalence pin. A drift in either branch
// chain (periodic / gated inversion / gated-0 inversion backfill / plain
// TotalMs backfill / state-total backfill / blocking-impact tail) fails here.

import (
	"fmt"
	"testing"
)

func TestWakeupCausalImpactEffectiveMirrorsRankLane(t *testing.T) {
	base := WakeupCausalImpact{
		Thread: ThreadRef{Comm: "dep", PID: 21}, ChainDepth: 1, OnChain: true,
		DominantState: string(StateSSleep), DominantImpactMs: 4,
		TotalMs: 7, RunningMs: 1, RunnableMs: 1, SleepMs: 4, DStateMs: 0.5, IOWaitMs: 0.5,
		LineStart: 5, LineEnd: 9,
	}
	shapes := map[string]WakeupCausalImpact{
		"plain_sleep": base,
		"plain_no_total": func() WakeupCausalImpact {
			v := base
			v.TotalMs = 0
			return v
		}(),
		"plain_blocking_tail": func() WakeupCausalImpact {
			v := base
			v.TotalMs = 0
			v.RunningMs, v.RunnableMs, v.SleepMs, v.DStateMs, v.IOWaitMs = 0, 0, 0, 0, 0
			return v
		}(),
		"d_state_blocking_tail": func() WakeupCausalImpact {
			v := base
			v.DominantState = string(StateDSleep)
			v.TotalMs = 0
			v.RunningMs, v.RunnableMs, v.SleepMs = 0, 0, 0
			v.DStateMs, v.IOWaitMs = 0, 0
			return v
		}(),
		"inversion_gated": func() WakeupCausalImpact {
			v := base
			v.PriorityInversionCandidate = true
			v.PriorityInversionGatedMs = 2.5
			v.GatedRunnableMs, v.GatedRunningDeficitMs = 2, 0.5
			return v
		}(),
		"inversion_gated_zero": func() WakeupCausalImpact {
			v := base
			v.PriorityInversionCandidate = true
			v.PriorityInversionGatedMs = 0
			return v
		}(),
		"periodic": func() WakeupCausalImpact {
			v := base
			v.PeriodicSource = true
			v.DetectedPeriodMs = 16.6
			v.LatenessMs = 0.5
			v.EffectivePeriodicImpactMs = 1.5
			return v
		}(),
		"periodic_zero": func() WakeupCausalImpact {
			v := base
			v.PeriodicSource = true
			v.DetectedPeriodMs = 16.6
			v.EffectivePeriodicImpactMs = 0
			return v
		}(),
		// EVOLUTION RECORD (§20.2 user ruling 2026-07-07, overturns the
		// §20.1甲 raw side clause — not a regression): a non-inversion
		// RUNNING row's effective is its ELIMINABLE supply-fold deficit,
		// authoritative at 0 — never the raw DominantImpactMs (this shape's
		// pre-§20.2 expectation) and never the sleep-inclusive TotalMs
		// backfill. plain_running has no fold (deficit 0 = full workload,
		// attribution ~0); plain_running_deficit carries a computed fold.
		"plain_running": func() WakeupCausalImpact {
			v := base
			v.DominantState = string(StateRunning)
			v.DominantImpactMs = 3
			v.RunningMs, v.SleepMs = 3, 3
			v.TotalMs = 7
			return v
		}(),
		"plain_running_deficit": func() WakeupCausalImpact {
			v := base
			v.DominantState = string(StateRunning)
			v.DominantImpactMs = 3
			v.RunningMs, v.SleepMs = 3, 3
			v.TotalMs = 7
			v.SupplyFoldDeficitMs = 1.2
			v.SupplyFoldIdealMs = 1.8
			v.SupplyFoldBasis = &SupplyFoldBasis{KnownMs: 3}
			return v
		}(),
		// A running-dominant inversion candidate still takes the gated branch
		// ahead of the running branch (branch-order pin).
		"running_inversion_gated": func() WakeupCausalImpact {
			v := base
			v.DominantState = string(StateRunning)
			v.DominantImpactMs = 4
			v.RunningMs, v.SleepMs = 4, 2
			v.TotalMs = 7
			v.PriorityInversionCandidate = true
			v.PriorityInversionGatedMs = 2.5
			v.GatedRunnableMs, v.GatedRunningDeficitMs = 1, 1.5
			return v
		}(),
	}
	for name, impact := range shapes {
		want := rootCauseEffectiveImpactMs(rootCauseItemFromCausalImpact(impact))
		got := WakeupCausalImpactEffectiveImpactMs(impact)
		if fmt.Sprintf("%.6f", got) != fmt.Sprintf("%.6f", want) {
			t.Fatalf("%s: exported helper %.6f != rank lane %.6f (两 lane 同输入必须同输出)", name, got, want)
		}
	}
}
