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
	// SUPP-HYG P3-B (2026-07-14): the frame family joins the DET-1 /
	// pre-canceled sweeps now that its assembly loops carry sampling points.
	"frame_window", "frame_timeline", "frame_flow",
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
// 100. Scan-unit ledger (SUPP-HYG P3-C updated the arithmetic: the pre-switch
// framework-surface probe is now tick-instrumented and consumes the first
// 70000 units): probe scan = units 1..70000 (modulo point 65536 = consult 2),
// EventSearch = units 70001..70100 (limit 100, no modulo point), cpu-frequency
// census = units 70101..140100 (modulo point 131072 = consult 3).
// runCancelAfterN(2) survives the entry boundary sample (consult 1) and the
// probe-scan modulo point, and fires exactly inside the census scan.
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

	res := Run(idx, q.WithRunContext(newRunCancelAfterN(2)))
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

// ⑦ SUPP-HYG P3-B (2026-07-14): a fire INSIDE the frame assembly main loop
// must discard the frame face whole through the existing attach gate. The
// frame family previously had ZERO sampling points after the span scan, so a
// mid-assembly fire was structurally impossible — this pin is red without the
// BuildFramePipeline assembly tick.
//
// Scan-unit ledger (deterministic, no wall clock): 30k B/E doFrame pairs =
// 60k events. Pre-switch framework probe = units 1..60000 (no 64Ki point);
// findSpanWindowsCompacted scan = units 60001..120000 (modulo point 65536 =
// consult 2); frame assembly loop = units 120001..150000 (modulo point
// 131072 = consult 3). runCancelAfterN(2) fires exactly inside the assembly
// loop.
func runCancelFrameFixtureIndex(t *testing.T) *Index {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "frame_fire.ftrace")
	var b strings.Builder
	for i := 0; i < 30000; i++ {
		ts := 3.0 + float64(i)*0.0001
		fmt.Fprintf(&b, "     ui-100 (100) [000] .... %.6f: tracing_mark_write: B|100|doFrame\n", ts)
		fmt.Fprintf(&b, "     ui-100 (100) [000] .... %.6f: tracing_mark_write: E|100\n", ts+0.00005)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestRunCancelFrameAssemblyFireDiscardsWholeFace(t *testing.T) {
	idx := runCancelFrameFixtureIndex(t)
	q := Query{View: "frame_window", Limit: 40000}

	// Control arm: the uncanceled run publishes the full frame pipeline.
	full := Run(idx, q)
	if full.FramePipeline == nil || len(full.FramePipeline.Items) == 0 || full.ViewCancellation != nil {
		t.Fatalf("control run must publish the frame pipeline without cancellation: %+v", full.ViewCancellation)
	}

	res := Run(idx, q.WithRunContext(newRunCancelAfterN(2)))
	vc := res.ViewCancellation
	if vc == nil {
		t.Fatal("mid-assembly fire did not mint the typed record — the frame assembly sampling point is missing")
	}
	if res.FramePipeline != nil {
		t.Fatal("a frame pipeline interrupted mid-assembly must be discarded whole, never published partial")
	}
	discarded := strings.Join(vc.DiscardedFaces, ",")
	if !strings.Contains(discarded, "frame_window") {
		t.Fatalf("DiscardedFaces = %q, want frame_window listed", discarded)
	}
}

// ⑦b SUPP-HYG P3-B: same fixture, frame_timeline view, fire inside the
// pipeline assembly loop (runCancelAfterN(2), same 131072 modulo point as ⑦)
// — the timeline arm's existing attach gate must discard the whole
// frame_timeline face. The timeline/flow loops themselves are display-capped
// (frame_window MaxLimit=20), so a mid-loop modulo fire is structurally
// unreachable there; their ticks' post-fire short-circuit is pinned
// separately below.
func TestRunCancelFrameTimelineFireDiscardsWholeFace(t *testing.T) {
	idx := runCancelFrameFixtureIndex(t)
	q := Query{View: "frame_timeline", Limit: 40000}

	full := Run(idx, q)
	if full.FrameTimeline == nil || len(full.FrameTimeline.Flows) == 0 || full.ViewCancellation != nil {
		t.Fatalf("control run must publish the frame timeline with flow edges: %+v", full.ViewCancellation)
	}

	res := Run(idx, q.WithRunContext(newRunCancelAfterN(2)))
	vc := res.ViewCancellation
	if vc == nil {
		t.Fatal("mid-assembly fire did not mint the typed record — the frame assembly sampling points are missing")
	}
	if res.FrameTimeline != nil {
		t.Fatal("a frame timeline interrupted mid-assembly must be discarded whole, never published partial")
	}
	discarded := strings.Join(vc.DiscardedFaces, ",")
	if !strings.Contains(discarded, "frame_timeline") {
		t.Fatalf("DiscardedFaces = %q, want frame_timeline listed", discarded)
	}
}

// ⑦c SUPP-HYG P3-B: the frame_timeline item-assembly tick short-circuits on
// an already-fired carrier — red if the buildFrameTimelineFromPipeline item
// tick is removed (the fired run would then assemble the full item list from
// a complete pipeline value).
func TestRunCancelFrameTimelineShortCircuitsAfterFire(t *testing.T) {
	idx := runCancelFrameFixtureIndex(t)
	q := Query{View: "frame_timeline", Limit: 40000}
	frame := BuildFramePipeline(idx, q)
	if len(frame.Items) == 0 {
		t.Fatal("control: frame pipeline items expected")
	}
	control := buildFrameTimelineFromPipeline(q, frame)
	if len(control.Items) == 0 || len(control.Flows) == 0 {
		t.Fatal("control: timeline items and flows expected")
	}
	dead, stop := context.WithCancel(context.Background())
	stop()
	fq := q.WithRunContext(dead)
	if !fq.runCancel.sample() {
		t.Fatal("dead context must fire at the boundary sample")
	}
	fired := buildFrameTimelineFromPipeline(fq, frame)
	if len(fired.Items) != 0 || len(fired.Flows) != 0 {
		t.Fatalf("fired: timeline assembly must short-circuit empty, got items=%d flows=%d", len(fired.Items), len(fired.Flows))
	}
}

// ⑧ SUPP-HYG P3-C (2026-07-14): a fire INSIDE the pre-switch framework-surface
// probe scan discards the probe census whole (禁半账) and records it on the
// typed DiscardedFaces (禁裸丢). 70k events emitted by an android-marker comm:
// the probe scan is units 1..70000, so the 65536 modulo point (consult 2,
// runCancelAfterN(1)) lands inside it. Red without the detectFrameworkSurfaces
// tick — the fire would then land in the window_stats sweep instead and the
// partial framework census would publish beside the cancellation record.
func TestRunCancelPreSwitchProbeFireDiscardsFrameworkCensus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe_fire.ftrace")
	var b strings.Builder
	for i := 0; i < 70000; i++ {
		fmt.Fprintf(&b, " com.app.ui-100 (100) [002] .... %.6f: cpu_frequency: state=%d cpu_id=2\n",
			3.0+float64(i)*0.00001, 1200000+(i%4)*100000)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "window_stats"}

	full := Run(idx, q)
	if len(full.FrameworkSurfaces) == 0 || full.ViewCancellation != nil {
		t.Fatalf("control run must publish the framework-surface census: surfaces=%v vc=%v", full.FrameworkSurfaces, full.ViewCancellation)
	}

	res := Run(idx, q.WithRunContext(newRunCancelAfterN(1)))
	vc := res.ViewCancellation
	if vc == nil {
		t.Fatal("in-probe fire did not mint the typed record — the probe sampling point is missing")
	}
	if len(res.FrameworkSurfaces) != 0 {
		t.Fatalf("a probe census interrupted mid-scan must be discarded whole: %+v", res.FrameworkSurfaces)
	}
	discarded := strings.Join(vc.DiscardedFaces, ",")
	if !strings.Contains(discarded, "framework_surfaces") {
		t.Fatalf("DiscardedFaces = %q, want framework_surfaces listed", discarded)
	}
}

// ⑨ SUPP-HYG P3-C: every newly instrumented stats sub-builder short-circuits
// on an already-fired carrier — first tick returns immediately with an empty
// value instead of paying its full scan (the overshoot disease). Each arm is
// red if its tick is removed: the fired run would then return the same
// non-empty value as the control run.
func TestRunCancelStatsSubBuildersShortCircuitAfterFire(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub_builders.ftrace")
	fixture := `com.app.ui-100 (100) [000] .... 3.000000: sched_switch: prev_comm=com.app.ui prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
     worker-200 (200) [001] .... 3.000100: sched_wakeup: comm=com.app.ui pid=100 prio=52 target_cpu=000
 com.app.ui-100 (100) [000] .... 3.001000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=com.app.ui next_pid=100 next_prio=52
      irq-7 (7) [000] .... 3.002000: irq_handler_entry: irq=17 name=timer
      irq-7 (7) [000] .... 3.002100: irq_handler_exit: irq=17 ret=handled
     worker-20 (20) [001] .... 3.003000: workqueue_execute_start: work=0xff function=flush_cookie
     worker-20 (20) [001] .... 3.003500: workqueue_execute_end: work=0xff function=flush_cookie
    display-30 (30) [002] .... 3.004000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9
    display-30 (30) [002] .... 3.004500: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9
       task-42 (42) [001] .... 3.005000: mm_filemap_add_to_page_cache: dev 260:132 ino 0x259ff pfn=1 ofs=0
     drifty-77 (77) [003] .... 3.006000: sched_switch: prev_comm=drifty prev_pid=77 prev_prio=120 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
    renamed-77 (77) [003] .... 3.006500: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=renamed next_pid=77 next_prio=120
     ui-100 (100) [000] .... 3.007000: tracing_mark_write: B|100|doFrame
     ui-100 (100) [000] .... 3.007500: tracing_mark_write: E|100
`
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{TimeStart: 3.0, TimeEnd: 3.2, TimeStartSet: true, TimeEndSet: true, Limit: 100}
	dead, stop := context.WithCancel(context.Background())
	stop()
	fq := q.WithRunContext(dead)
	if !fq.runCancel.sample() {
		t.Fatal("dead context must fire at the boundary sample")
	}

	if got := detectFrameworkSurfaces(idx, q, TracePlatformAuto, 4); len(got) == 0 {
		t.Fatal("control: framework surfaces expected")
	}
	if got := detectFrameworkSurfaces(idx, fq, TracePlatformAuto, 4); len(got) != 0 {
		t.Fatalf("fired: framework surfaces must short-circuit empty, got %+v", got)
	}
	if got := computeIdleRunnableMismatchMs(idx, q, nil); got <= 0 {
		t.Fatal("control: idle/runnable mismatch expected > 0")
	}
	if got := computeIdleRunnableMismatchMs(idx, fq, nil); got != 0 {
		t.Fatalf("fired: idle/runnable mismatch must short-circuit 0, got %f", got)
	}
	if got := computeIRQBursts(idx, q, 8); len(got) == 0 {
		t.Fatal("control: IRQ bursts expected")
	}
	if got := computeIRQBursts(idx, fq, 8); len(got) != 0 {
		t.Fatalf("fired: IRQ bursts must short-circuit empty, got %+v", got)
	}
	if got, _ := computeInterruptActivity(idx, q, EventIRQ, nil, 8); len(got) == 0 {
		t.Fatal("control: interrupt activity expected")
	}
	if got, _ := computeInterruptActivity(idx, fq, EventIRQ, nil, 8); len(got) != 0 {
		t.Fatalf("fired: interrupt activity must short-circuit empty, got %+v", got)
	}
	if got, _ := computeWorkqueueActivity(idx, q, 8); len(got) == 0 {
		t.Fatal("control: workqueue activity expected")
	}
	if got, _ := computeWorkqueueActivity(idx, fq, 8); len(got) != 0 {
		t.Fatalf("fired: workqueue activity must short-circuit empty, got %+v", got)
	}
	if got, _ := computeDMAFenceActivity(idx, q, 8); len(got) == 0 {
		t.Fatal("control: DMA fence activity expected")
	}
	if got, _ := computeDMAFenceActivity(idx, fq, 8); len(got) != 0 {
		t.Fatalf("fired: DMA fence activity must short-circuit empty, got %+v", got)
	}
	if got := computeMemoryKinds(idx, q, 8); len(got) == 0 {
		t.Fatal("control: memory kinds expected")
	}
	if got := computeMemoryKinds(idx, fq, 8); len(got) != 0 {
		t.Fatalf("fired: memory kinds must short-circuit empty, got %+v", got)
	}
	if got := detectThreadDrifts(idx, q, 8); len(got) == 0 {
		t.Fatal("control: thread drifts expected")
	}
	if got := detectThreadDrifts(idx, fq, 8); len(got) != 0 {
		t.Fatalf("fired: thread drifts must short-circuit empty, got %+v", got)
	}
	if got := BuildFramePipeline(idx, q); len(got.Items) == 0 {
		t.Fatal("control: frame pipeline items expected")
	}
	if got := BuildFramePipeline(idx, fq); len(got.Items) != 0 {
		t.Fatalf("fired: frame pipeline must short-circuit empty, got %+v", got.Items)
	}
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
