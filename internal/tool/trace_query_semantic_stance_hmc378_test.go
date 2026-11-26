package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// A marker that records the thread waiting for a collection is a recorded
// wait whichever layer of the classifier owns its name. Public trace_query
// entry, synthetic capture.

// hmc378RecordedWaitCapture: main-100 records name over [10.010, 10.250). It
// runs 2.000 ms inside the marker, sleeps at 10.012, is woken at 10.2005 and
// runs again from 10.2006 to the end of the marker — 51.400 ms on a CPU,
// 188.500 ms asleep and 0.100 ms ready inside 240.000 ms of marker. relay-200
// (same process) records the same name over [10.011, 10.2003): it sleeps at
// 10.0115, is woken by the collector at 10.200 and wakes main. The collector
// (HeapTaskDaemon-150) records a background collection cycle.
func hmc378RecordedWaitCapture(t *testing.T, name string) string {
	t.Helper()
	var c hmc376Trace
	c.run(10.000000, "main", 100, 100, 1)
	c.run(10.000200, "relay", 200, 100, 2)
	c.run(10.000100, "HeapTaskDaemon", 150, 100, 3)
	c.span(10.005000, 10.199900, "HeapTaskDaemon", 150, 100, 3, "Background concurrent copying GC")
	c.span(10.010000, 10.250000, "main", 100, 100, 1, name)
	c.span(10.011000, 10.200300, "relay", 200, 100, 2, name)
	c.sleep(10.011500, "relay", 200, 100, 2)
	c.sleep(10.012000, "main", 100, 100, 1)
	c.add(10.200000, "HeapTaskDaemon", 150, 100, 3, "sched_wakeup: comm=relay pid=200 prio=120 target_cpu=002")
	c.sleep(10.200050, "HeapTaskDaemon", 150, 100, 3)
	c.run(10.200100, "relay", 200, 100, 2)
	c.add(10.200500, "relay", 200, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.sleep(10.201000, "relay", 200, 100, 2)
	c.run(10.200600, "main", 100, 100, 1)
	c.sleep(10.300000, "main", 100, 100, 1)
	return c.write(t)
}

type hmc378StanceName struct {
	name  string
	waits bool
}

// The spellings: the vocabulary's wait row (already a wait), the built-in
// collection arm's compound wait shapes, the wait head the built-in arm owns
// because of a tail word, and a built-in compound with no wait in it.
var hmc378StanceNames = []hmc378StanceName{
	{"GC: Wait For Completion Alloc", true},
	{"WaitForGcToComplete", true},
	{"wait_for_gc_to_complete", true},
	{"H:WaitForGcToComplete", true},
	{"GC: Wait For Completion collector", true},
	{"SuspendAllForGC", false},
}

// The engine: a recorded wait is priced by the sleep it records and is never
// the thread's own executed work. It mints no own-work row and none of the
// thread's running time is deterministic work because of it. A compound name
// with no wait in it keeps its own-work row and its running share.
func TestHMC378BuiltInWaitNameIsARecordedWait(t *testing.T) {
	for _, c := range hmc378StanceNames {
		t.Run(c.name, func(t *testing.T) {
			path := hmc378RecordedWaitCapture(t, c.name)
			published, native := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 100, "target_scope": "thread"}, 10.0, 10.31)
			rank, states := native.RootCauseRank, native.TargetWindowStates
			if rank == nil || states == nil {
				t.Fatal("no ranking or no state partition was published")
			}
			if !hmc376Near(states.RunningMs, 111.4) || !hmc376Near(states.SleepMs, 188.5) || !hmc376Near(states.RunnableMs, 0.1) {
				t.Fatalf("the scheduler partition of the capture is 111.400 running / 188.500 asleep / 0.100 ready: %+v", states)
			}
			var wait, own *tracequery.RootCauseRankItem
			for i := range rank.Items {
				row := &rank.Items[i]
				if row.Thread.PID != 100 || row.Type != "gc_pause" {
					continue
				}
				if row.OnChainBasis == tracequery.RootCauseOnChainBasisSelfDeterministicSpan {
					own = row
				} else {
					wait = row
				}
			}
			// Whatever the stance, the wait inside the marker is the first
			// cause and is priced by the sleep.
			if wait == nil || wait.Rank != 1 || wait.DominantState != "s_sleep" || !hmc376Near(wait.EffectiveImpactMs, 188.5) || wait.OwnWorkOnCPUMs != 0 {
				t.Fatalf("the wait inside the marker stays the first cause, priced by its 188.500 ms of sleep:\n%s", hmc376Board(rank))
			}
			if c.waits {
				if own != nil {
					t.Errorf("a recorded wait gained an own-work row: %s on_cpu=%.3f", hmc376RowLabel(*own), own.OwnWorkOnCPUMs)
				}
				if states.DeterministicRunningMs != 0 {
					t.Errorf("no running time is deterministic work because of a recorded wait: deterministic_running_ms=%.3f", states.DeterministicRunningMs)
				}
				if strings.Contains(published.Summary, "own_work_on_cpu=") {
					t.Error("the model-facing board prices a recorded wait as work the thread executed on a CPU")
				}
				return
			}
			if own == nil || !hmc376Near(own.EffectiveImpactMs, 51.5) || !hmc376Near(own.OwnWorkOnCPUMs, 51.4) {
				t.Errorf("a compound name with no wait in it keeps its own-work row (51.500 ms outside the wait, 51.400 on a CPU):\n%s", hmc376Board(rank))
			}
			if !hmc376Near(states.DeterministicRunningMs, 51.4) {
				t.Errorf("the running time inside executed work stays deterministic: deterministic_running_ms=%.3f", states.DeterministicRunningMs)
			}
		})
	}
}
