package tracequery

// run_cancel_test.go — SUPP-CANCEL (2026-07-14) engine pins.
//
// Pin family:
//   ① DET-1 (the batch's heaviest obligation): with an ARMED but never-fired
//     context, every view's Result is byte-identical (JSON) to the nil-carrier
//     run — the sampling points are pure reads on the untriggered path.
//   ② pre-canceled context ⇒ typed ViewCancellation + exactly one
//     "in_view_cancellation=true" caveat + ZERO published faces (禁半账 whole
//     discard, 禁裸丢 disclosure).
//   ③ mid-flight determinism sweep: with a context that fires after k
//     samples, every face still attached to the Result must be byte-equal to
//     the same face of the UNCANCELED run — published ⇒ complete, never a
//     partial aggregate.
//   ④ non-cancelable contexts (Background/TODO/value-only) attach no carrier.
//   ⑤ tick/fired/sample nil-safety and modulo behavior.
//
// MUTATION self-checks (verified by hand during the batch, red on):
//   - removing an attach gate (e.g. window_stats) reds ②/③ (partial face
//     published while ViewCancellation is set);
//   - making WithRunContext attach Background contexts reds ④;
//   - dropping the exit mint reds ② (no caveat / no typed record).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runCancelTrace exercises the rank/stats/chain/critical family: worker-200
// D-blocks with blocked reasons, peer-300 wakes it, plus frequency and mark
// rows so several stats sub-passes have material.
const runCancelTrace = `        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.000200: tracing_mark_write: B|300|frameit
     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     kworker-77 (77) [000] .... 3.001500: cpu_frequency: state=1800000 cpu_id=2
     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.010500: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x1a8/0x2b8
       peer-300 (300) [003] .... 3.030000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
     worker-200 (200) [003] .... 3.031000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.040000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
     worker-200 (200) [003] .... 3.040500: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x1a8/0x2b8
       peer-300 (300) [003] .... 3.070000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
     worker-200 (200) [003] .... 3.071000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
       peer-300 (300) [003] .... 3.080000: tracing_mark_write: E|300
     worker-200 (200) [003] .... 3.090000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.150000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
     worker-200 (200) [003] .... 3.151000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.160000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 3.200000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func runCancelIndex(t *testing.T) *Index {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "run_cancel.ftrace")
	if err := os.WriteFile(path, []byte(runCancelTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func runCancelQuery(view string) Query {
	return Query{
		View: view, PID: 200, Thread: "worker",
		TimeStart: 3.0, TimeEnd: 3.2, TimeStartSet: true, TimeEndSet: true,
	}
}

var runCancelViews = []string{
	"window_stats", "root_cause_rank", "wakeup_chain", "thread_timeline",
	"scheduler_latency_stats", "critical_blocking_calls", "ipc_graph",
	"trace_perf_bundle", "frame_root_cause_bundle", "evidence_pack",
	"event_search", "interaction_stats", "perf_timeline",
}

func runCancelResultJSON(t *testing.T, res Result) []byte {
	t.Helper()
	payload, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// ① DET-1: armed-but-never-fired context ⇒ byte-identical Result per view.
func TestRunCancelDET1ArmedContextByteIdentical(t *testing.T) {
	idx := runCancelIndex(t)
	armed, stop := context.WithCancel(context.Background())
	defer stop()
	for _, view := range runCancelViews {
		q := runCancelQuery(view)
		plain := runCancelResultJSON(t, Run(idx, q))
		withCtx := runCancelResultJSON(t, Run(idx, q.WithRunContext(armed)))
		if string(plain) != string(withCtx) {
			t.Fatalf("view=%s: armed-but-unfired context changed the result\nplain: %s\narmed: %s", view, plain, withCtx)
		}
	}
}

// ④ non-cancelable contexts attach no carrier.
func TestRunCancelNonCancelableContextsAttachNothing(t *testing.T) {
	for _, ctx := range []context.Context{nil, context.Background(), context.TODO(), context.WithValue(context.Background(), struct{}{}, 1)} {
		q := Query{View: "window_stats"}.WithRunContext(ctx)
		if q.runCancel != nil {
			t.Fatalf("context %v must not attach a carrier", ctx)
		}
	}
	live, stop := context.WithCancel(context.Background())
	defer stop()
	if q := (Query{View: "window_stats"}).WithRunContext(live); q.runCancel == nil {
		t.Fatal("cancelable context must attach a carrier")
	}
}

// ② pre-canceled context ⇒ typed record + one caveat + zero faces.
func TestRunCancelPreCanceledDiscardsWholeAndDiscloses(t *testing.T) {
	idx := runCancelIndex(t)
	dead, stop := context.WithCancel(context.Background())
	stop()
	for _, view := range runCancelViews {
		res := Run(idx, runCancelQuery(view).WithRunContext(dead))
		if res.ViewCancellation == nil {
			t.Fatalf("view=%s: pre-canceled run must mint the typed ViewCancellation", view)
		}
		if res.ViewCancellation.Reason != "canceled" {
			t.Fatalf("view=%s: reason=%q, want canceled", view, res.ViewCancellation.Reason)
		}
		caveats := 0
		for _, caveat := range res.Caveats {
			if strings.HasPrefix(caveat, "in_view_cancellation=true;") {
				caveats++
			}
		}
		if caveats != 1 {
			t.Fatalf("view=%s: want exactly one in_view_cancellation caveat, got %d (%v)", view, caveats, res.Caveats)
		}
		assertRunCancelNoPartialFaces(t, view, res)
	}
}

// assertRunCancelNoPartialFaces: on a pre-canceled run every publishable face
// must be absent (the first sampling point fires before anything completes).
func assertRunCancelNoPartialFaces(t *testing.T, view string, res Result) {
	t.Helper()
	if res.WindowStats != nil || res.WakeupChain != nil || res.RootCauseRank != nil ||
		res.SchedulerLatency != nil || res.CriticalBlocking != nil || res.IPCGraph != nil ||
		res.Timeline != nil || res.FramePipeline != nil || res.FrameTimeline != nil ||
		res.FrameRootCauseBundle != nil || res.InteractionStats != nil || res.PerfTimeline != nil ||
		res.TargetWindowStates != nil || len(res.Events) > 0 || len(res.EvidencePack) > 0 {
		t.Fatalf("view=%s: pre-canceled run published a face: %+v", view, res)
	}
}

// runCancelAfterN is a context.Context whose Err() flips non-nil after n
// reads — a deterministic mid-flight fire without wall-clock timing.
type runCancelAfterN struct {
	remaining int
	done      chan struct{}
}

func newRunCancelAfterN(n int) *runCancelAfterN {
	return &runCancelAfterN{remaining: n, done: make(chan struct{})}
}
func (c *runCancelAfterN) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *runCancelAfterN) Done() <-chan struct{}       { return c.done }
func (c *runCancelAfterN) Value(any) any               { return nil }
func (c *runCancelAfterN) Err() error {
	if c.remaining > 0 {
		c.remaining--
		return nil
	}
	return context.Canceled
}

// ③ mid-flight sweep: any face published by a canceled run must be byte-equal
// to the uncanceled run's same face (published ⇒ complete).
func TestRunCancelMidFlightPublishedFacesAreComplete(t *testing.T) {
	idx := runCancelIndex(t)
	sawPublishedAny := false
	for _, view := range []string{"root_cause_rank", "trace_perf_bundle", "evidence_pack", "window_stats"} {
		q := runCancelQuery(view)
		full := Run(idx, q)
		fullFaces := runCancelFaceJSON(t, full)
		sawCancel, sawPublishedFace := false, false
		for k := 0; k < 64; k++ {
			res := Run(idx, q.WithRunContext(newRunCancelAfterN(k)))
			if res.ViewCancellation == nil {
				// fired after the last sampling point — full run, must be
				// byte-identical on every face.
				for face, want := range fullFaces {
					if got := runCancelFaceJSON(t, res)[face]; got != want {
						t.Fatalf("view=%s k=%d: uncanceled face %s diverged", view, k, face)
					}
				}
				break
			}
			sawCancel = true
			for face, got := range runCancelFaceJSON(t, res) {
				if got == "null" || got == "" {
					continue
				}
				sawPublishedFace = true
				want := fullFaces[face]
				if got != want {
					t.Fatalf("view=%s k=%d: published face %s differs from the complete run — a partial aggregate escaped\ngot:  %s\nwant: %s", view, k, face, got, want)
				}
				for _, discarded := range res.ViewCancellation.DiscardedFaces {
					if discarded == face {
						t.Fatalf("view=%s k=%d: face %s is both published and listed as discarded", view, k, face)
					}
				}
			}
		}
		if !sawCancel {
			t.Fatalf("view=%s: sweep never triggered a cancellation — sampling points missing", view)
		}
		sawPublishedAny = sawPublishedAny || sawPublishedFace
	}
	if !sawPublishedAny {
		t.Fatal("sweep never produced a canceled run with a published complete face — the partial-publish lane went untested")
	}
}

// runCancelFaceJSON snapshots every gated face as JSON for byte comparison.
func runCancelFaceJSON(t *testing.T, res Result) map[string]string {
	t.Helper()
	faces := map[string]any{
		"window_stats":            res.WindowStats,
		"wakeup_chain":            res.WakeupChain,
		"root_cause_rank":         res.RootCauseRank,
		"scheduler_latency_stats": res.SchedulerLatency,
		"critical_blocking_calls": res.CriticalBlocking,
		"ipc_graph":               res.IPCGraph,
		"thread_timeline":         res.Timeline,
		"frame_root_cause_bundle": res.FrameRootCauseBundle,
		"interaction_stats":       res.InteractionStats,
		"perf_timeline":           res.PerfTimeline,
		"target_window_states":    res.TargetWindowStates,
		"events":                  res.Events,
	}
	out := map[string]string{}
	for name, face := range faces {
		payload, err := json.Marshal(face)
		if err != nil {
			t.Fatal(err)
		}
		out[name] = string(payload)
	}
	return out
}

// ⑥ 复核件1 (P3-A): a fire INSIDE a census scan must reach DiscardedFaces.
// The census builders tick-abort and return nil — before the gate-order fix
// the arm's `census != nil &&` short-circuit swallowed the discard record and
// the caveat claimed "discarded: none" while two faces were abandoned.
// Deterministic shape: 70k all-matching cpu_frequency events, display limit
// 100 ⇒ EventSearch consumes ~100 scan units, the cpu-frequency census scans
// all 70k, so the ONE 64Ki modulo sampling point (unit 65536) lands inside
// the census scan; runCancelAfterN(1) survives the entry boundary sample and
// fires exactly there.
func TestRunCancelCensusFireReachesDiscardedFaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "census_fire.ftrace")
	var b strings.Builder
	for i := 0; i < 70000; i++ {
		fmt.Fprintf(&b, " kworker-77 (77) [002] .... %.6f: cpu_frequency: state=%d cpu_id=2\n",
			3.0+float64(i)*0.00001, 1200000+(i%4)*100000)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", EventTypes: []EventType{EventCPUFrequency}, Limit: 100}

	// Control arm: uncanceled run publishes the truncation census.
	full := Run(idx, q)
	if full.CPUFrequencyCensus == nil || full.ViewCancellation != nil {
		t.Fatalf("control run must publish the census without cancellation: census=%v vc=%v", full.CPUFrequencyCensus, full.ViewCancellation)
	}

	res := Run(idx, q.WithRunContext(newRunCancelAfterN(1)))
	vc := res.ViewCancellation
	if vc == nil {
		t.Fatal("census-scan fire did not mint the typed record")
	}
	discarded := strings.Join(vc.DiscardedFaces, ",")
	if discarded != "cpu_frequency_census,vsync_generator_census" {
		t.Fatalf("DiscardedFaces = %q, want both censuses (gate-order regression)", discarded)
	}
	if res.CPUFrequencyCensus != nil || res.VsyncGeneratorCensus != nil {
		t.Fatal("aborted censuses must not publish")
	}
	// The events face completed BEFORE the fire and stays published complete.
	if len(res.Events) != len(full.Events) {
		t.Fatalf("pre-fire events face must stay published complete: got %d want %d", len(res.Events), len(full.Events))
	}
	for _, caveat := range res.Caveats {
		if strings.HasPrefix(caveat, "in_view_cancellation=true;") {
			if !strings.Contains(caveat, "discarded: cpu_frequency_census,vsync_generator_census") {
				t.Fatalf("caveat must name the discarded censuses: %s", caveat)
			}
			return
		}
	}
	t.Fatal("in_view_cancellation caveat missing")
}

// ⑤ carrier primitives: nil-safety + modulo sampling + fire ordering.
func TestRunCancelStatePrimitives(t *testing.T) {
	var nilState *runCancelState
	if nilState.tick() || nilState.fired() || nilState.sample() {
		t.Fatal("nil carrier must never fire")
	}
	nilState.discardFace("x") // must not panic
	if nilState.record("v") != nil {
		t.Fatal("nil carrier must not mint a record")
	}

	dead, stop := context.WithCancel(context.Background())
	stop()
	c := newRunCancelState(dead)
	// unit 1: modulo miss (1 & 0xFFFF != 0) — no fire even on a dead context.
	if c.tick() {
		t.Fatal("tick must sample only at the 64Ki modulo")
	}
	if c.sample() != true || !c.fired() {
		t.Fatal("boundary sample on a dead context must fire")
	}
	if !c.tick() {
		t.Fatal("post-fire ticks must short-circuit true")
	}
	c.discardFace("a")
	c.discardFace("a")
	c.discardFace("b")
	rec := c.record("window_stats")
	if rec == nil || rec.View != "window_stats" || rec.Reason != "canceled" || strings.Join(rec.DiscardedFaces, ",") != "a,b" {
		t.Fatalf("record mismatch: %+v", rec)
	}
	if !strings.Contains(c.caveat("window_stats"), "in_view_cancellation=true;") ||
		!strings.Contains(c.caveat("window_stats"), "discarded: a,b") {
		t.Fatalf("caveat mismatch: %s", c.caveat("window_stats"))
	}

	deadline, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()
	d := newRunCancelState(deadline)
	if !d.sample() {
		t.Fatal("expired deadline must fire at a boundary sample")
	}
	if d.reason != "deadline_exceeded" {
		t.Fatalf("reason=%q, want deadline_exceeded", d.reason)
	}
}
