package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
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

// The reader: its occupancy table lists a recorded wait apart from executed
// work, and it reads the same stance the engine priced the row with. Public
// entry and real delivery, both report languages.
func TestHMC378ReaderListsBuiltInWaitNamesAsRecordedWaits(t *testing.T) {
	for _, c := range hmc378StanceNames {
		t.Run(c.name, func(t *testing.T) {
			delivered := hmc377DeliverWindow(t, hmc378RecordedWaitCapture(t, c.name), 100, 10.0, 10.31)
			if delivered.rank == nil {
				t.Fatal("no ranking was published")
			}
			engineOwnWork := false
			for _, row := range delivered.rank.Items {
				if row.Thread.PID == 100 && row.Type == "gc_pause" && row.OnChainBasis == tracequery.RootCauseOnChainBasisSelfDeterministicSpan {
					engineOwnWork = true
				}
			}
			if engineOwnWork == c.waits {
				t.Fatalf("engine: own-work row present = %v for a name whose recorded-wait stance is %v", engineOwnWork, c.waits)
			}
			for _, face := range []struct{ language, text, waitGroup, workGroup string }{
				{"zh", delivered.zh, "已记录的等待（墙钟）", "确定性工作 span（墙钟）"},
				{"en", delivered.en, "Recorded waits (wall clock)", "Deterministic work spans (wall clock)"},
			} {
				// Both threads that record the marker: the target (240.000 ms
				// of marker) and the relay (189.300 ms).
				for _, row := range []string{"main-100 / " + c.name + " | 240.000ms |", "relay-200 / " + c.name + " | 189.300ms |"} {
					asWait := strings.Contains(face.text, "| "+face.waitGroup+" | "+row)
					asWork := strings.Contains(face.text, "| "+face.workGroup+" | "+row)
					if asWait != c.waits || asWork == c.waits {
						t.Errorf("%s occupancy table: %q listed as a recorded wait = %v, as executed work = %v; the engine's stance: records a wait = %v",
							face.language, row, asWait, asWork, c.waits)
					}
				}
			}
		})
	}
}

// The reader's predicate is the engine's decision, for a row of one record and
// for a merged row alike. A merged row is read from the stance its merge
// decided from all members, never from the name it inherited; a fold without a
// name is read the same way.
func TestHMC378ReaderAndEngineStanceAgree(t *testing.T) {
	const target = "main-100"
	leaf := func(span string) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{Subject: target, SpanName: span, SemanticClass: "gc_pause", ImpactMS: 7, EffectiveImpactMS: 7}
	}
	merged := func(span string, count int, everyMemberWaits bool) types.TraceCausalProjectionNode {
		node := leaf(span)
		node.MergedCount, node.MergedRecordsWait = count, everyMemberWaits
		return node
	}
	for _, c := range []struct {
		label string
		node  types.TraceCausalProjectionNode
		waits bool
	}{
		{"the vocabulary's wait row", leaf("GC: Wait For Completion Alloc"), true},
		{"the shared-heap wait row", leaf("H:SuspendTime::WaitGCFinished"), true},
		{"a built-in compound wait", leaf("WaitForGcToComplete"), true},
		{"a built-in compound wait, snake case", leaf("wait_for_gc_to_complete"), true},
		{"a built-in compound wait behind the hitrace prefix", leaf("H:WaitForGcToComplete"), true},
		{"the wait head with a cause word the built-in arm reads", leaf("GC: Wait For Completion collector"), true},
		{"a built-in compound that executes", leaf("SuspendAllForGC"), false},
		{"a vocabulary row that executes", leaf("H:FullGC::RunPhases;GCReason2"), false},
		{"another built-in class", leaf("VerifyClass com.example.Foo"), false},
		{"an unclassified name", leaf("Choreographer#doFrame"), false},
		{"a merged row seeded by a wait whose members are not all waits", merged("GC: Wait For Completion Alloc", 3, false), false},
		{"a merged row whose members are all waits", merged("GC: Wait For Completion Alloc", 3, true), true},
		{"a fold without a name whose members are all waits", merged("", 4, true), true},
		{"a fold without a name with other members", merged("", 4, false), false},
	} {
		if got := runtimeTraceProjSemanticRowRecordsWait(c.node); got != c.waits {
			t.Errorf("%s: the reader reads records a wait = %v, want %v", c.label, got, c.waits)
		}
		// Both reader lanes follow it. The occupancy table lists the row
		// under the wait group or the work group.
		model := runtimeTraceProjTreeModel{Target: target, TreeRows: []runtimeTraceProjTreeRow{{
			Kind: runtimeTraceProjTreeRowSemantic, HasData: true, EvidenceTag: "E1", Node: c.node,
		}}}
		rows := runtimeTraceOccupancySemanticCandidates(model, true)
		if len(rows) != 1 || (rows[0].group == "已记录的等待（墙钟）") != c.waits || (rows[0].group == "确定性工作 span（墙钟）") == c.waits {
			t.Errorf("%s: occupancy rows %+v, want the wait group = %v", c.label, rows, c.waits)
		}
		// The running breakdown counts executed work only: a row of waits is
		// no class of deterministic work and is never the row it points at.
		tag, count, _ := runtimeTraceProjFourStateSemanticPointer(model)
		if (tag == "" && count == 0) != c.waits {
			t.Errorf("%s: the running breakdown points at %q and counts %d classes, want a pointer only for executed work (waits = %v)", c.label, tag, count, c.waits)
		}
	}
}

// The fold of the small anchored seats at a batch head is one more row made of
// other rows and carries no span name: like every fold it is a row of waits
// only when every seat it folded records a wait.
func TestHMC378MicroAnchorFoldCarriesTheStanceOfItsMembers(t *testing.T) {
	seat := func(subject, span string) *runtimeTraceProjTreeRow {
		return &runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowChain, HasData: true, Node: types.TraceCausalProjectionNode{
			Subject: subject, SpanName: span, ImpactMS: 0.05, EffectiveImpactMS: 0.05,
		}}
	}
	waits := runtimeTraceProjBuildMicroAnchorFoldRow([]*runtimeTraceProjTreeRow{seat("a-1", "WaitForGcToComplete"), seat("b-2", "GC: Wait For Completion Alloc")})
	if waits.Node.MergedCount != 2 || waits.Node.SpanName != "" || !runtimeTraceProjSemanticRowRecordsWait(waits.Node) {
		t.Errorf("a fold of two recorded waits is a row of waits: %+v", waits.Node)
	}
	mixed := runtimeTraceProjBuildMicroAnchorFoldRow([]*runtimeTraceProjTreeRow{seat("a-1", "WaitForGcToComplete"), seat("b-2", "SuspendAllForGC")})
	if mixed.Node.MergedCount != 2 || runtimeTraceProjSemanticRowRecordsWait(mixed.Node) {
		t.Errorf("a fold that holds executed work is no row of waits: %+v", mixed.Node)
	}
}
