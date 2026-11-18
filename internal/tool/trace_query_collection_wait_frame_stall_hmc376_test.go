package tool

import (
	"math"
	"os"
	"testing"
)

// Whole-original public-entry regression for the eval case "main thread parked until a
// collection completes". Opt-in per capture through an environment variable, like the
// Honeybird whole-capture tests; digest and size are checked before anything is parsed.
// Every expected number comes from the independent raw oracle (oracle376b.py: standalone
// Python over the raw ftrace text, integer microseconds), never from this engine.
//
// Three recordings of the same application show the same shape. The device-reboot recording is
// the batch's second real eval case; the capability fixture was not cut from any of them.
func TestHMC376CollectionWaitOnMainThreadIsOnChainWork(t *testing.T) {
	type states struct{ running, runnable, sleep, dState, ioWait float64 }
	cases := []struct {
		name, env, digest string
		size              int64
		tid               int
		start, end        float64
		want              states
		waitSleepMs       float64 // the target's sleep inside the wait marker
		waitWallMs        float64 // the wait marker's wall time
		collectorTID      int     // the thread whose wakeup ended the wait
		collectorRunning  float64 // its on-CPU time inside the wait
		collectorReady    float64 // its ready-not-running time inside the wait
		pacingIdleMs      float64 // one ordinary frame-pacing sleep inside the window (0 = none)
	}{
		{
			name: "killed-in-background cold start, stalled frame interval", env: "CODRAX_HMC376_NETEASE_GCWAIT",
			digest: "ae52285a00c4afce43e48bae519615110973052370537e7c53f5e4351073f2e0", size: 106869666,
			tid: 34044, start: 5775.523119, end: 5775.790999,
			want:        states{running: 4.309, runnable: 0.418, sleep: 263.153},
			waitSleepMs: 247.304, waitWallMs: 247.466,
			collectorTID: 34060, collectorRunning: 241.290, collectorReady: 1.972,
			pacingIdleMs: 15.393,
		},
		{
			name: "device-reboot cold start, one frame callback", env: "CODRAX_HMC376_NETEASE_REBOOT_FRAME",
			digest: "48cf2fd8b91a81f2276a16a5988ef14729c078e5d1f23615eb166d2f6e2c8cbc", size: 104144939,
			tid: 11077, start: 153.763595, end: 153.887340,
			want:        states{running: 0.727, runnable: 0.015, sleep: 123.003},
			waitSleepMs: 123.003, waitWallMs: 123.089,
			collectorTID: 11098, collectorRunning: 115.347, collectorReady: 0.820,
		},
		{
			name: "container-restart cold start, stalled frame interval", env: "CODRAX_HMC376_NETEASE_CONTAINER_FRAME",
			digest: "45e304445ea5f661a21c36df65b87863e48142e26e82bfab2fc5a9d582a408f8", size: 95407103,
			tid: 42477, start: 2151.959183, end: 2152.225413,
			want:        states{running: 3.345, runnable: 0.147, sleep: 262.707, dState: 0.031},
			waitSleepMs: 247.514, waitWallMs: 247.609,
			collectorTID: 42491, collectorRunning: 245.765, collectorReady: 1.749,
		},
	}
	near := func(a, b float64) bool { return math.Abs(a-b) <= 0.0015 }
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := os.Getenv(c.env)
			if path == "" {
				t.Skipf("requires %s (the pinned complete capture)", c.env)
			}
			if h, n := hmc176bRepresentativeDigest(t, path); h != c.digest || n != c.size {
				t.Fatalf("%s is not the pinned capture", c.env)
			}
			hmc176DisableExternalProviders(t)
			ctx, _, _ := hmc17NamedPathContext(t)
			r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": c.tid, "target_scope": "thread", "time_start": c.start, "time_end": c.end})
			value := hmc367NativeResult(t, r, "root_cause_rank")

			// (1) Scheduler partition of the target equals the raw oracle (green before the batch).
			st := value.TargetWindowStates
			if st == nil || !near(st.RunningMs, c.want.running) || !near(st.RunnableMs, c.want.runnable) || !near(st.SleepMs, c.want.sleep) || !near(st.DStateMs, c.want.dState) || !near(st.IOWaitMs, c.want.ioWait) {
				t.Fatalf("target state partition disagrees with the raw oracle: %+v", st)
			}
			if value.RootCauseRank == nil {
				t.Fatal("no native ranking")
			}
			offCPU := c.want.runnable + c.want.sleep + c.want.dState + c.want.ioWait
			targetWait, collector, pacing := 0, false, false
			for _, row := range value.RootCauseRank.Items {
				own := row.Thread.PID == c.tid
				switch {
				// (2) The wait the target itself recorded is the first on-chain cause, priced by the
				// time the target was actually parked, with the marker's own wall time kept beside it.
				case own && row.Type == "gc_pause" && row.ChainRelevance == "on_chain" && row.Rank > 0:
					targetWait++
					if row.Rank != 1 || row.Tier != "primary" || !near(row.EffectiveImpactMs, c.waitSleepMs) || !near(row.ActualImpactMs, c.waitWallMs) {
						t.Errorf("the target's own collection wait is not the first on-chain cause at its measured size: rank=%d tier=%s effective=%.3fms actual=%.3fms (raw oracle: parked %.3fms, marker %.3fms)", row.Rank, row.Tier, row.EffectiveImpactMs, row.ActualImpactMs, c.waitSleepMs, c.waitWallMs)
					}
				// (3) Other threads parked by the same collection are fellow waiters, never causes.
				case !own && row.Type == "gc_pause":
					if row.ChainRelevance != "background" || row.EffectiveImpactMs != 0 || row.Rank != 0 {
						t.Errorf("a fellow waiter %s-%d is not background: chain=%s rank=%d effective=%.3fms", row.Thread.Comm, row.Thread.PID, row.ChainRelevance, row.Rank, row.EffectiveImpactMs)
					}
				// (6) An ordinary frame-pacing sleep stays out of the ranking (protected behaviour).
				case own && row.Type == "pacing_idle":
					if c.pacingIdleMs > 0 && near(row.ImpactMs, c.pacingIdleMs) && row.Rank == 0 {
						pacing = true
					}
				}
				if !own && row.ChainRelevance == "on_chain" && row.Rank > 0 {
					// (4) The thread that ended the wait keeps its measured state account.
					if row.Thread.PID == c.collectorTID && (row.RunningMs > 0 || row.RunnableMs > 0) {
						collector = true
						if !near(row.RunningMs, c.collectorRunning) || !near(row.RunnableMs, c.collectorReady) {
							t.Errorf("state account of the thread that ended the wait disagrees with the raw oracle: running=%.3fms ready=%.3fms (oracle %.3f / %.3f)", row.RunningMs, row.RunnableMs, c.collectorRunning, c.collectorReady)
						}
					}
					// (5) Nobody can have delayed the target for longer than the target was off a CPU.
					if row.EffectiveImpactMs > offCPU+0.0015 {
						t.Errorf("on-chain %s row of %s-%d claims %.3fms, more than the %.3fms the target was off a CPU in the whole window", row.Type, row.Thread.Comm, row.Thread.PID, row.EffectiveImpactMs, offCPU)
					}
				}
			}
			if targetWait != 1 {
				t.Errorf("want exactly one ranked on-chain collection-wait row on the target, got %d", targetWait)
			}
			if !collector {
				t.Errorf("the thread that ended the wait (tid %d) lost its on-chain state row", c.collectorTID)
			}
			if c.pacingIdleMs > 0 && !pacing {
				t.Errorf("the %.3fms frame-pacing sleep is no longer published as unranked pacing context", c.pacingIdleMs)
			}
			// (7) The wait is published as collection work, not as file IO.
			if value.WindowStats != nil {
				for _, span := range value.WindowStats.TraceSpans {
					if span.Thread.PID == c.tid && span.Name == "GC: Wait For Completion Alloc" && (span.Category == "file_io" || span.SemanticClass != "gc_pause") {
						t.Errorf("the target's collection wait is published as category=%q class=%q", span.Category, span.SemanticClass)
					}
				}
			}
		})
	}
}
