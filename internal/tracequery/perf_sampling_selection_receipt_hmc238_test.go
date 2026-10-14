package tracequery

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSamplingStreamSelectionReceiptHMC238MatchesIndexedDualSelector(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, thread string
		pid          int
		start, end   float64
		count        int
		mismatch     bool
	}{
		{"wrong numeric thread", "document-worker", 10, 1, 2, 0, true},
		{"correct thread", "document-worker", 11, 1, 2, 5, false},
		{"true empty window", "document-worker", 11, 4, 5, 0, false},
		{"different display name", "other-worker", 11, 1, 2, 5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := Query{View: ViewPerfSamplingTree, PID: tc.pid, Thread: tc.thread, TimeStart: tc.start, TimeEnd: tc.end}
			streamed, err := StreamPerfSamplingTree(context.Background(), path, q)
			if err != nil {
				t.Fatal(err)
			}
			indexed := Run(idx, q)
			if streamed.ThreadSelection == nil || !reflect.DeepEqual(streamed.ThreadSelection, indexed.ThreadSelection) {
				t.Fatalf("stream dropped or changed shared selection receipt: stream=%+v indexed=%+v", streamed.ThreadSelection, indexed.ThreadSelection)
			}
			if streamed.ThreadSelection.NameMismatch != tc.mismatch || streamed.ThreadSelection.Selected.PID != tc.pid || streamed.PerfSamplingTree.Status != "available" || streamed.PerfSamplingTree.SampleCount != tc.count {
				t.Fatalf("receipt changed target/population or confused empty with mismatch: %+v / %+v", streamed.ThreadSelection, streamed.PerfSamplingTree)
			}
			if !reflect.DeepEqual(streamed.PerfSamplingTree, indexed.PerfSamplingTree) {
				t.Fatal("selection receipt changed tree/cohort denominators")
			}
		})
	}
}

func TestSamplingStreamSelectionReceiptHMC238WithholdsUnavailable(t *testing.T) {
	idx := buildTraceIndex(t, "conflicting-thread.perftrace", "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=3 event=cycles symbol=work source=raw\nworker-11 (12) [000] .... 1.2: perf_sample: pid=12 tid=11 sample_weight=7 event=cycles symbol=work source=raw\n")
	q := Query{View: ViewPerfSamplingTree, PID: 11, Thread: "worker", TimeStart: 1, TimeEnd: 2}
	for _, tc := range []struct {
		name   string
		limit  int
		reason string
	}{{"unproven identity", 100, "typed_identity_unproven"}, {"retention overflow", 0, "complete_scan_retention_budget_exceeded"}} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := streamPerfSamplingTree(context.Background(), idx.Path, q, tc.limit, 1<<20)
			if err != nil || r.PerfSamplingTree == nil || r.PerfSamplingTree.Status != "unavailable" || r.PerfSamplingTree.Reason != tc.reason || r.ThreadSelection != nil {
				t.Fatalf("unavailable retained identity gained selector certificate: err=%v result=%+v", err, r)
			}
		})
	}
}

func TestSamplingStreamSelectionReceiptHMC238UnknownIdentityDoesNotReuseHeader(t *testing.T) {
	idx := buildTraceIndex(t, "unknown-thread.perftrace", "worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 thread_identity_known=false sample_weight=3 event=cycles symbol=work source=raw\n")
	r, err := StreamPerfSamplingTree(context.Background(), idx.Path, Query{View: ViewPerfSamplingTree, PID: 11, Thread: "worker", TimeStart: 1, TimeEnd: 2})
	// The pre-existing tree currently reports available/0 for a source-only
	// sample. This receipt-only fix must not turn that into proven identity,
	// nor silently expand into changing population availability semantics.
	if err != nil || r.ThreadSelection != nil || r.PerfSamplingTree.Status != "available" || r.PerfSamplingTree.SampleCount != 0 ||
		!strings.Contains(strings.Join(r.Caveats, " "), "not establish absence") {
		t.Fatalf("unknown typed identity gained a header-derived receipt: %v %+v", err, r)
	}
}

func TestSamplingStreamSelectionReceiptHMC238CandidatesDoNotRetarget(t *testing.T) {
	idx := buildTraceIndex(t, "same-name.perftrace", `
worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 sample_weight=3 event=cycles symbol=work source=raw
worker-12 (10) [000] .... 1.2: perf_sample: pid=10 tid=12 sample_weight=7 event=cycles symbol=work source=raw
non-sampled-99 (99) [000] .... 1.3: tracing_mark_write: I|99|task
`)
	r, err := StreamPerfSamplingTree(context.Background(), idx.Path, Query{View: ViewPerfSamplingTree, PID: 10, Thread: "worker", TimeStart: 1, TimeEnd: 2})
	if err != nil || r.ThreadSelection == nil {
		t.Fatalf("missing selection diagnostics: %v %+v", err, r)
	}
	if r.ThreadSelection.Selected.PID != 10 || !r.ThreadSelection.NameMismatch || len(r.ThreadSelection.NameCandidates) != 2 || r.PerfSamplingTree.SampleCount != 0 {
		t.Fatalf("same-name candidates silently changed target: %+v / %+v", r.ThreadSelection, r.PerfSamplingTree)
	}
	r, err = StreamPerfSamplingTree(context.Background(), idx.Path, Query{View: ViewPerfSamplingTree, PID: 99, Thread: "non-sampled", TimeStart: 1, TimeEnd: 2})
	if err != nil || r.ThreadSelection != nil || r.PerfSamplingTree.SampleCount != 0 ||
		!strings.Contains(strings.Join(r.Caveats, " "), "not establish absence") {
		t.Fatalf("sample-only metadata claimed full-capture identity absence: %v %+v", err, r)
	}
}
