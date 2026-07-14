package tracequery

// run_cancel_real_trace_test.go — SUPP-CANCEL (2026-07-14) real-trace pins.
//
// ① DET-1 双 trace 直读对照 (the batch's heaviest obligation, in-repo form):
//    on BOTH shipped real traces (donghu.ftrace / donghu_tieba_frame
//    .systrace, the golden-sample windows), an ARMED but never-fired context
//    produces byte-identical Result JSON per heavy view. The binary-level
//    twin of this pin (HEAD vs batch tracediag reports, byte-compared) is an
//    acceptance artifact, not a test.
// ② real-trace wall-clock deadline red/green: a live few-ms deadline on the
//    donghu rank view MUST fire mid-run (typed record present, published
//    faces byte-equal to the complete run's faces), while the no-context run
//    stays cancellation-free.
//
// The fixture paths follow the in-package precedent
// (rank_chain_anchor_rspa_test.go); missing fixtures skip, never pass
// silently green.

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

type runCancelRealTraceCase struct {
	name      string
	path      string
	pid       int
	timeStart float64
	timeEnd   float64
}

var runCancelRealTraces = []runCancelRealTraceCase{
	{name: "donghu", path: "../../eval/fixtures/real_traces/donghu.ftrace", pid: 17267, timeStart: 13762.791708, timeEnd: 13763.024898},
	{name: "tieba", path: "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace", pid: 59566, timeStart: 34579.472865, timeEnd: 34579.502785},
}

func runCancelRealTraceIndex(t *testing.T, tc runCancelRealTraceCase) *Index {
	t.Helper()
	if _, err := os.Stat(tc.path); err != nil {
		t.Skipf("real trace fixture unavailable: %v", err)
	}
	idx, err := BuildIndex(context.Background(), tc.path)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func runCancelRealTraceQuery(tc runCancelRealTraceCase, view string) Query {
	return Query{
		View: view, PID: tc.pid,
		TimeStart: tc.timeStart, TimeEnd: tc.timeEnd,
		TimeStartSet: true, TimeEndSet: true,
	}
}

// ① DET-1: dual real traces, heavy views, armed-but-unfired context.
func TestRunCancelDET1RealTracesByteIdentical(t *testing.T) {
	armed, stop := context.WithCancel(context.Background())
	defer stop()
	for _, tc := range runCancelRealTraces {
		idx := runCancelRealTraceIndex(t, tc)
		for _, view := range []string{"window_stats", "root_cause_rank", "wakeup_chain", "critical_blocking_calls"} {
			q := runCancelRealTraceQuery(tc, view)
			plain, err := json.Marshal(Run(idx, q))
			if err != nil {
				t.Fatal(err)
			}
			withCtx, err := json.Marshal(Run(idx, q.WithRunContext(armed)))
			if err != nil {
				t.Fatal(err)
			}
			if string(plain) != string(withCtx) {
				t.Fatalf("%s/%s: armed-but-unfired context changed the result", tc.name, view)
			}
		}
	}
}

// ② real-trace wall-clock deadline red/green on the donghu rank view.
func TestRunCancelRealTraceDeadlineRedGreen(t *testing.T) {
	tc := runCancelRealTraces[0]
	idx := runCancelRealTraceIndex(t, tc)
	q := runCancelRealTraceQuery(tc, "root_cause_rank")

	// RED arm: no context — the complete run, zero cancellation.
	full := Run(idx, q)
	if full.ViewCancellation != nil {
		t.Fatalf("no-context run minted a cancellation: %+v", full.ViewCancellation)
	}
	fullFaces := runCancelFaceJSON(t, full)

	// GREEN arm: a 2ms live deadline cannot cover the multi-pass rank build
	// on this trace — the cooperative sampling points must fire mid-run.
	deadline, cancelDeadline := context.WithTimeout(context.Background(), 2*time.Millisecond)
	defer cancelDeadline()
	res := Run(idx, q.WithRunContext(deadline))
	if res.ViewCancellation == nil {
		t.Fatal("2ms deadline on the donghu rank view did not trigger in-view cancellation")
	}
	if res.ViewCancellation.Reason != "deadline_exceeded" {
		t.Fatalf("reason=%q, want deadline_exceeded", res.ViewCancellation.Reason)
	}
	// Published ⇒ complete: every attached face must byte-match the full run.
	for face, got := range runCancelFaceJSON(t, res) {
		if got == "null" || got == "" {
			continue
		}
		if want := fullFaces[face]; got != want {
			t.Fatalf("published face %s differs from the complete run — a partial aggregate escaped", face)
		}
	}
}
