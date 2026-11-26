package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFrameWindowIndexReusesBundleForTimeline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frame.ftrace")
	trace := strings.Join([]string{
		"         <idle>-0 (-----) [000] d..3  99.500000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=10 next_prio=100",
		"            app-10 (-----) [000] d..3 100.100000: sched_switch: prev_comm=app prev_pid=10 prev_prio=100 prev_state=D ==> next_comm=worker next_pid=11 next_prio=120",
		"         worker-11 (-----) [000] d..3 100.399999: sched_blocked_reason: pid=10 iowait=0 caller=ion_client_destroy+0x5c/0xcc",
		"         worker-11 (-----) [000] d..3 100.400000: sched_wakeup: comm=app pid=10 prio=100 success=1 target_cpu=000",
		"         worker-11 (-----) [000] d..3 100.500000: sched_switch: prev_comm=worker prev_pid=11 prev_prio=120 prev_state=R ==> next_comm=app next_pid=10 next_prio=100",
		"            app-10 (-----) [000] d..3 101.000000: sched_switch: prev_comm=app prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=worker next_pid=11 next_prio=120",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}
	request := UseIndexLimitMemo(WithIndexLimitMemo(t.Context()))
	producer := UseFrameWindowIndex(request, "frame_root_cause_bundle")
	consumer := UseFrameWindowIndex(request, "thread_timeline")
	narrow := BuildOptions{
		AllowWindowedParse: true, ScopePID: 10,
		TimeStart: 100, TimeEnd: 100.6, TimeStartSet: true, TimeEndSet: true,
		TimePaddingBefore: .25, TimePaddingAfter: .25,
	}
	wide := narrow
	wide.TimePaddingBefore, wide.TimePaddingAfter = .5, .5
	build := func(ctx context.Context, opts BuildOptions) (*Index, bool) {
		t.Helper()
		built := false
		idx, err := buildIndexWithObserver(ctx, path, opts, func(phase traceIndexBuildPhase, _ parseCacheKey) {
			if phase == traceIndexPhaseBeforeSchedulerHead {
				built = true
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		return idx, built
	}
	build(producer, wide)
	followup, built := build(consumer, narrow)
	if built || followup.IndexTimeEnd != paddedTimeEnd(narrow) {
		t.Fatal("follow-up did not reuse the bundle events at normal timeline bounds")
	}
	standalone, _ := build(t.Context(), narrow)
	q := Query{View: "thread_timeline", PID: 10, TimeStart: 100, TimeEnd: 100.6, TimeStartSet: true, TimeEndSet: true}
	want, got := Run(standalone, q), Run(followup, q)
	if len(got.Timeline.Intervals) == 0 || !reflect.DeepEqual(got.Timeline, want.Timeline) ||
		!reflect.DeepEqual(got.EvidencePack, want.EvidencePack) {
		t.Fatalf("reused timeline/evidence differ\nstandalone: %+v\nreused: %+v", want.Timeline, got.Timeline)
	}
	otherThread, otherWindow, extraPadding := narrow, narrow, narrow
	otherThread.ScopePID = 11
	otherWindow.TimeEnd = 100.7
	extraPadding.TimePaddingBefore = .6
	for _, tc := range []struct {
		name string
		ctx  context.Context
		opts BuildOptions
	}{
		{"new request", UseFrameWindowIndex(UseIndexLimitMemo(WithIndexLimitMemo(t.Context())), "thread_timeline"), narrow},
		{"different thread", consumer, otherThread},
		{"different window", consumer, otherWindow},
		{"insufficient padding", consumer, extraPadding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, built := build(tc.ctx, tc.opts); !built {
				t.Fatal("reused an incompatible window index")
			}
		})
	}
	if err := os.WriteFile(path, []byte(trace+"# changed source generation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, built := build(consumer, narrow); !built {
		t.Fatal("reused the previous source generation")
	}
}

func TestFrameWindowIndexKeepsBothComparisonSides(t *testing.T) {
	const trace = "app-10 (-----) [000] d..3 100.100000: sched_switch: prev_comm=app prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=worker next_pid=11 next_prio=120\n" +
		"worker-11 (-----) [000] d..3 100.400000: sched_wakeup: comm=app pid=10 prio=100 success=1 target_cpu=000\n" +
		"worker-11 (-----) [000] d..3 100.500000: sched_switch: prev_comm=worker prev_pid=11 prev_prio=120 prev_state=R ==> next_comm=app next_pid=10 next_prio=100\n"
	dir := t.TempDir()
	paths := [2]string{filepath.Join(dir, "baseline.ftrace"), filepath.Join(dir, "current.ftrace")}
	request := UseIndexLimitMemo(WithIndexLimitMemo(t.Context()))
	opts := BuildOptions{AllowWindowedParse: true, ScopePID: 10, TimeStart: 100, TimeEnd: 100.6,
		TimeStartSet: true, TimeEndSet: true, TimePaddingBefore: .5, TimePaddingAfter: .5}
	build := func(ctx context.Context, view, path string) *Index {
		t.Helper()
		idx, err := BuildIndexWithOptions(UseFrameWindowIndex(ctx, view), path, opts)
		if err != nil {
			t.Fatal(err)
		}
		return idx
	}
	var stats [2]*Index
	for i, path := range paths {
		if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
			t.Fatal(err)
		}
		stats[i] = build(request, "window_stats", path)
	}
	memo := request.Value(activeIndexLimitMemoKey{}).(*indexLimitMemo)
	// Two callers can finish the same singleflight build before either sees
	// the request memo. A second publication must leave room for both roles.
	memo.storeFrameWindow(parseCacheKey{}, opts, stats[1], "window_stats")
	for i, path := range paths {
		rank := build(request, "root_cause_rank", path)
		wakeup := build(request, "wakeup_chain", path)
		if rank != stats[i] || wakeup != rank {
			t.Fatal("statistics, rank and chain did not share the same source window")
		}
		q := Query{View: "root_cause_rank", PID: 10, TimeStart: 100, TimeEnd: 100.6, TimeStartSet: true, TimeEndSet: true}
		ordered := Run(rank, q.WithRunContext(request))
		q.View = "wakeup_chain"
		want := Run(rank, q)
		got := Run(wakeup, q.WithRunContext(request))
		if ordered.WakeupChain == nil || len(want.WakeupChain.Edges) == 0 ||
			!reflect.DeepEqual(got.WakeupChain, want.WakeupChain) || !reflect.DeepEqual(got.EvidencePack, want.EvidencePack) {
			t.Fatal("reused chain or evidence differs from the independently computed wakeup view")
		}
		q.MaxBranches = 1
		want = Run(rank, q)
		got = Run(wakeup, q.WithRunContext(request))
		if !reflect.DeepEqual(got.WakeupChain, want.WakeupChain) || !reflect.DeepEqual(got.EvidencePack, want.EvidencePack) {
			t.Fatal("the explicitly narrowed wakeup view received rank's different budget")
		}
	}
	if idx := build(UseIndexLimitMemo(WithIndexLimitMemo(t.Context())), "root_cause_rank", paths[0]); idx == stats[0] {
		t.Fatal("reused an index from a different request")
	}
}
