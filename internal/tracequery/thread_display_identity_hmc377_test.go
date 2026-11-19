package tracequery

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// HMC377 thread identity (D3/D4/E5): one physical thread has one identity
// across its creation and its renames. The fixture mirrors the Honeybird
// witness shape: zygote-1 creates the app main thread 56 (which inherits the
// creator's name "zygote" and then names itself "app.main"); 56 creates 76,
// which inherits "app.main" until it names itself "Signal Catcher"; 56 sleeps
// and 76 wakes it. A third thread 90 renames per task like a pool worker.
func hmc377IdentityTraceLines() []string {
	return []string{
		" zygote-1 (1) [000] .... 1.000000: sched_wakeup_new: comm=zygote pid=56 prio=120 target_cpu=000",
		" <idle>-0 (0) [000] .... 1.001000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=zygote next_pid=56 next_prio=120",
		" app.main-56 (56) [000] .... 1.002000: sched_switch: prev_comm=app.main prev_pid=56 prev_prio=120 prev_state=R+ ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" <idle>-0 (0) [000] .... 1.003000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app.main next_pid=56 next_prio=120",
		" app.main-56 (56) [000] .... 1.004000: sched_wakeup_new: comm=app.main pid=76 prio=120 target_cpu=001",
		" app.main-56 (56) [000] .... 1.010000: sched_switch: prev_comm=app.main prev_pid=56 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" Signal Catcher-76 (56) [001] .... 1.020000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app.main next_pid=76 next_prio=120",
		" Signal Catcher-76 (56) [001] .... 1.030000: sched_switch: prev_comm=Signal Catcher prev_pid=76 prev_prio=120 prev_state=R+ ==> next_comm=worker next_pid=90 next_prio=120",
		" task-b-90 (56) [001] .... 1.040000: sched_switch: prev_comm=task-a prev_pid=90 prev_prio=120 prev_state=S ==> next_comm=Signal Catcher next_pid=76 next_prio=120",
		" Signal Catcher-76 (56) [001] .... 1.050000: sched_wakeup: comm=app.main pid=56 prio=120 target_cpu=000",
		" <idle>-0 (0) [000] .... 1.051000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app.main next_pid=56 next_prio=120",
		" Signal Catcher-76 (56) [001] .... 1.060000: sched_switch: prev_comm=Signal Catcher prev_pid=76 prev_prio=120 prev_state=S ==> next_comm=task-b next_pid=90 next_prio=120",
		" task-b-90 (56) [001] .... 1.090000: sched_switch: prev_comm=task-b prev_pid=90 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120",
		" app.main-56 (56) [000] .... 1.095000: sched_switch: prev_comm=app.main prev_pid=56 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" <idle>-0 (0) [000] .... 1.100000: cpu_idle: state=0 cpu_id=0",
	}
}

func hmc377IdentityIndexes(t *testing.T) map[string]*Index {
	t.Helper()
	path := writeSchedulerCarryTrace(t, "identity.ftrace", hmc377IdentityTraceLines()...)
	full, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]*Index{"full": full, "windowed": buildSchedulerCarryWindow(t, path, 1.0, 1.1),
		"derived": deriveWindowedIndex(full, BuildOptions{TimeStart: 1.0, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true})}
}

// namesByTID collects every name a JSON result shows for each TID, in typed
// fields and in system labels, excluding the identity disclosure itself.
func hmc377NamesByTID(t *testing.T, res Result, tids ...int) map[int]map[string]bool {
	t.Helper()
	res.ThreadIdentities = nil
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var walk func(any, string)
	out := map[int]map[string]bool{}
	for _, tid := range tids {
		out[tid] = map[string]bool{}
	}
	names := []string{"zygote", "app.main", "Signal Catcher", "task-a", "task-b", "worker"}
	walk = func(v any, key string) {
		switch x := v.(type) {
		case map[string]any:
			if pid, ok := x["pid"].(float64); ok {
				if comm, ok := x["comm"].(string); ok && out[int(pid)] != nil {
					out[int(pid)][comm] = true
				}
			}
			for k, vv := range x {
				walk(vv, k)
			}
		case []any:
			for _, vv := range x {
				walk(vv, key)
			}
		case string:
			if key != "summary" && key != "subject" && key != "object" {
				return
			}
			for _, tid := range tids {
				for _, name := range names {
					if strings.Contains(x, " "+name+"-"+itoaHMC377(tid)) || strings.HasPrefix(x, name+"-"+itoaHMC377(tid)) || strings.Contains(x, "="+name+"-"+itoaHMC377(tid)) {
						out[tid][name] = true
					}
				}
			}
		}
	}
	var decoded any
	_ = json.Unmarshal(data, &decoded)
	walk(decoded, "")
	return out
}

func itoaHMC377(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestHMC377OneThreadOneDisplayNameAcrossFacesAndIndexes(t *testing.T) {
	for name, idx := range hmc377IdentityIndexes(t) {
		t.Run(name, func(t *testing.T) {
			before, _ := json.Marshal(idx.Events)
			for _, view := range []string{"wakeup_chain", "root_cause_rank", "window_stats", "thread_timeline"} {
				res := Run(idx, Query{View: view, PID: 56, TimeStart: 1.0, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true})
				got := hmc377NamesByTID(t, res, 56, 76, 90)
				if got[56]["zygote"] || got[76]["app.main"] {
					t.Fatalf("%s: an inherited creation name must never name the thread: %v", view, got)
				}
				if len(got[56]) > 1 || len(got[76]) > 1 || len(got[90]) > 1 {
					t.Fatalf("%s: one physical thread published under several names: %v", view, got)
				}
				if got[76]["Signal Catcher"] == false && view == "wakeup_chain" {
					t.Fatalf("%s: the waker must carry its own name: %v", view, got)
				}
				for _, identity := range res.ThreadIdentities {
					if identity.TID == 76 && (identity.DisplayComm != "Signal Catcher" || len(identity.Names) != 2 ||
						identity.Names[0].Comm != "app.main" || !identity.Names[0].InheritedAtCreation || identity.Names[0].FirstLine != 6) {
						t.Fatalf("%s: waker identity disclosure wrong: %+v", view, identity)
					}
					if identity.TID == 56 && identity.DisplayComm != "app.main" {
						t.Fatalf("%s: main thread display wrong: %+v", view, identity)
					}
				}
			}
			after, _ := json.Marshal(idx.Events)
			if string(before) != string(after) {
				t.Fatal("publication mutated the indexed source events")
			}
		})
	}
}

// A pool worker that takes each task's name is shown by the name it carried
// for most of the asked window, with its other names disclosed.
func TestHMC377TaskRenamingWorkerTakesItsNameInTheWindow(t *testing.T) {
	idx := hmc377IdentityIndexes(t)["full"]
	res := Run(idx, Query{View: "wakeup_chain", PID: 56, TimeStart: 1.0, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true})
	ledger := ensureThreadDisplayLedger(idx)
	record := ledger.record(90)
	if record == nil {
		t.Fatal("a renamed worker must be unified")
	}
	// Runs: worker (1.030), task-a (1.040), task-b (1.040..): task-b covers
	// the most of [1.0, 1.1].
	if got, basis := record.displayForWindow(1.0, 1.1); got != "task-b" || basis != ThreadDisplayBasisLongestInWindow {
		t.Fatalf("worker display for the window = %q (%s)", got, basis)
	}
	if got, _ := record.displayForWindow(1.031, 1.039); got != "worker" {
		t.Fatalf("worker display for an early window = %q", got)
	}
	_ = res
}

// Two generations of one TID are two threads: names are never unified across
// a recorded exit and reuse, and the reuse keeps its boundary disclosure.
func TestHMC377ReusedTIDKeepsGenerationsApart(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "reuse.ftrace",
		" idle-0 (0) [000] .... 0.700000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=55 next_prio=120",
		" old-55 (55) [000] .... 0.800000: sched_switch: prev_comm=old prev_pid=55 prev_prio=120 prev_state=X ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" creator-7 (7) [000] .... 1.010000: sched_wakeup_new: comm=frag pid=55 prio=120 target_cpu=000",
		" idle-0 (0) [000] .... 1.030000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=frag next_pid=55 next_prio=120",
		" frag-55 (55) [000] .... 1.090000: sched_switch: prev_comm=renamed prev_pid=55 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" idle-0 (0) [000] .... 1.100000: cpu_idle: state=0 cpu_id=0",
	)
	full, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	for name, idx := range map[string]*Index{"full": full, "windowed": buildSchedulerCarryWindow(t, path, 1, 1.1)} {
		if ensureThreadDisplayLedger(idx).record(55) != nil && name == "full" {
			t.Fatalf("%s: a reused TID must not be unified across generations", name)
		}
		tl := Run(idx, Query{View: "thread_timeline", PID: 55, TimeStart: 1, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true}).Timeline
		if tl == nil || !containsSubstring(tl.Caveats, "thread_generation_boundary=true") || containsSubstring(tl.Caveats, "thread created at") {
			t.Fatalf("%s: the reuse boundary disclosure must stay: %+v", name, tl)
		}
	}
}

// The first creation record of a TID starts its first lifetime: never a
// boundary or a reset, in a full index, a windowed parse and a derived window.
func TestHMC377FirstCreationIsNotAGenerationBoundary(t *testing.T) {
	for name, idx := range hmc377IdentityIndexes(t) {
		tl := Run(idx, Query{View: "thread_timeline", PID: 76, TimeStart: 1.0, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true}).Timeline
		if tl == nil || containsSubstring(tl.Caveats, "thread_generation_boundary") || !containsSubstring(tl.Caveats, "thread created at 1.004000 s (line 6): TID 76 starts its first lifetime here") {
			t.Fatalf("%s: first creation misreported: %+v", name, tl)
		}
		ready := 0.0
		for _, iv := range tl.Intervals {
			if iv.State == StateRunnable {
				ready += iv.DurationMs
			}
		}
		// 1.004 -> 1.020 after creation, and 1.030 -> 1.040 preempted.
		if !near(ready, 26, 1e-6) {
			t.Fatalf("%s: ready time after creation must be measured ready time: %.6f %+v", name, ready, tl.Intervals)
		}
	}
}

// Two process numbers for one TID: no unification (the TID may name two
// tasks), names stay as produced.
func TestHMC377TwoProcessNumbersWithholdUnification(t *testing.T) {
	path := writeSchedulerCarryTrace(t, "tgid.ftrace",
		" a-40 (90) [000] .... 1.000000: sched_switch: prev_comm=a prev_pid=40 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" b-40 (91) [000] .... 1.010000: sched_switch: prev_comm=b prev_pid=40 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" idle-0 (0) [000] .... 1.100000: cpu_idle: state=0 cpu_id=0",
	)
	idx, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if ensureThreadDisplayLedger(idx).record(40) != nil {
		t.Fatal("a TID recorded under two process numbers must not be unified")
	}
}

// Label re-rendering: only exact recorded names of that TID, longest first; a
// display name that ends with an alias, a decimal number, or another TID's
// label is never touched.
func TestHMC377LabelRewriteIsExactAndLongestFirst(t *testing.T) {
	ledger := &threadDisplayLedger{records: map[int]*threadDisplayRecord{76: {
		tid: 76,
		runs: []threadNameRun{{comm: "Catcher", firstTs: 1, lastTs: 1.1, inherited: true, firstLine: 1, lastLine: 2},
			{comm: "Signal Catcher", firstTs: 1.2, lastTs: 1.3, firstLine: 3, lastLine: 4}},
		known:    map[string]bool{"Catcher": true, "Signal Catcher": true},
		byLength: []string{"Signal Catcher", "Catcher"},
	}}}
	view := newThreadDisplayView(ledger, 1, 2)
	cases := map[string]string{
		"Signal Catcher-76 wakes x":            "Signal Catcher-76 wakes x",
		"competitor=Catcher-76 ran":            "competitor=Signal Catcher-76 ran",
		"(Catcher-76, Catcher-760, Catcher-7)": "(Signal Catcher-76, Catcher-760, Catcher-7)",
		"xCatcher-76 and Catcher-76.5ms":       "xCatcher-76 and Catcher-76.5ms",
	}
	for in, want := range cases {
		got, _ := view.rewriteLabels(in, nil)
		if got != want {
			t.Fatalf("rewriteLabels(%q) = %q, want %q", in, got, want)
		}
	}
}

// The publication pass is copy-on-write: a second Run on the same index gives
// the same result, and a caller mutating the first result cannot reach it.
func TestHMC377PublicationIsCopyOnWrite(t *testing.T) {
	idx := hmc377IdentityIndexes(t)["full"]
	q := Query{View: "wakeup_chain", PID: 56, TimeStart: 1.0, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true}
	first := Run(idx, q)
	if first.WakeupChain == nil || len(first.WakeupChain.Nodes) == 0 {
		t.Fatalf("no chain: %+v", first.WakeupChain)
	}
	first.WakeupChain.Nodes[0].Thread.Comm = "mutated"
	second := Run(idx, q)
	third := Run(idx, q)
	if !reflect.DeepEqual(second.WakeupChain, third.WakeupChain) || second.WakeupChain.Nodes[0].Thread.Comm == "mutated" {
		t.Fatal("publication aliases a later query")
	}
}

// The waker census keys a pair by thread numbers: a waker that renames itself
// between two wakeups of the same target is one pair of count 2, not two rows
// that read identically after publication.
func TestHMC377WakeupCensusPairSurvivesWakerRename(t *testing.T) {
	trace := strings.ReplaceAll(wakeupCensusChainTrace,
		"wakerA-200 (200) [002] .... 5.019000: sched_wakeup: comm=app pid=100",
		"renamedA-200 (200) [002] .... 5.019000: sched_wakeup: comm=app pid=100")
	if trace == wakeupCensusChainTrace {
		t.Fatal("fixture drifted")
	}
	idx := buildTraceIndex(t, "wake_census_rename.systrace", trace)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	pairs := 0
	for _, pair := range chain.WakeupEdgeCensus {
		if pair.Waker.PID == 200 && pair.Wakee.PID == 100 {
			pairs++
			if pair.Count != 2 || pair.FirstTs != 5.010 || pair.LastTs != 5.019 {
				t.Fatalf("renamed waker split or miscounted: %+v", pair)
			}
		}
	}
	if pairs != 1 {
		t.Fatalf("one physical waker->wakee pair must be one census row: %+v", chain.WakeupEdgeCensus)
	}
}

// A former name still selects the thread together with its TID.
func TestHMC377FormerNameSelectsTheSameThread(t *testing.T) {
	idx := hmc377IdentityIndexes(t)["full"]
	res := Run(idx, Query{View: "thread_timeline", PID: 76, Thread: "app.main", TimeStart: 1.0, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true})
	if res.ThreadSelection == nil || res.ThreadSelection.NameMismatch {
		t.Fatalf("a recorded former name of TID 76 is not a mismatch: %+v", res.ThreadSelection)
	}
	res = Run(idx, Query{View: "thread_timeline", PID: 76, Thread: "task-b", TimeStart: 1.0, TimeEnd: 1.1, TimeStartSet: true, TimeEndSet: true})
	if res.ThreadSelection == nil || !res.ThreadSelection.NameMismatch {
		t.Fatalf("another thread's name stays a mismatch: %+v", res.ThreadSelection)
	}
}
