package tracequery

// dstate_iowait_platform_g12_test.go — G12-ENG engine-truth pins (§29.1,
// real_trace_campaign_20260705.md, 2026-07-09).
//
// Platform semantics (§29.1 新发现, berlin.systrace verbatim rows below): on
// this Harmony platform an IO-waiting kernel thread (hmfs_discard-26) sleeps
// in S state and the kernel emits a separate `sched_blocked_reason: pid=…
// iowait=1` MARKER row at wakeup — there is NO prev_state=D interval at all
// (g12_report.txt step 1: zero matches over the whole region). The engine
// treatment is pinned here as the CORRECT single-attribution behavior:
//
//   - S+iowait sleeps never enter the D-state/iowait duration lanes
//     (DStateTop/IOWaitTop book only D-opened intervals — the blocked_reason
//     marker upgrades D→io_wait, never S→anything);
//   - blocked_reason critical_blocking rows stay ZERO-duration markers
//     attributed to the payload pid (the S+iowait thread);
//   - no engine lane mints a cross-thread same-value duration pair for these
//     rows — the E23 "2次(14.272–14.272ms)" form was a DISPLAY fabrication
//     (see trace_causal_projection_valueless_fold_g12_test.go), never an
//     engine double attribution.

import (
	"strings"
	"testing"
)

// g12PlatformTrace interleaves verbatim specimen rows: three S+iowait cycles
// of hmfs_discard-26-562 (udk-irq wakes it, sched_blocked_reason pid=562
// iowait=1, it runs briefly and parks back in S) plus the target's ONE real
// 0.011ms D-state interval (oney.hmn.berlin-42591, line 1624254 form).
func g12PlatformTrace(t *testing.T) *Index {
	rows := []string{
		`        udk-irq-5-78    (    2) [005] .... 6793224.930284: sched_waking: comm=hmfs_discard-26 pid=562 prio=20 target_cpu=004`,
		`        udk-irq-5-78    (    2) [005] .... 6793224.930290: sched_wakeup: comm=hmfs_discard-26 pid=562 prio=20 target_cpu=004`,
		`        udk-irq-5-78    (    2) [005] .... 6793224.930291: sched_blocked_reason: pid=562 iowait=1 caller=0x69680100fffe0000`,
		`  hmfs_discard-26-562   (    2) [004] .... 6793224.930303: sched_switch: prev_comm=tppmgr-idle-4 prev_pid=0 prev_prio=-2 prev_state=R+ ==> next_comm=hmfs_discard-26 next_pid=562 next_prio=20`,
		`    tppmgr-idle-4-189   (    2) [004] .... 6793224.930317: sched_switch: prev_comm=hmfs_discard-26 prev_pid=562 prev_prio=20 prev_state=S ==> next_comm=tppmgr-idle-4 next_pid=0 next_prio=-2`,
		`        udk-irq-5-78    (    2) [005] .... 6793224.931763: sched_waking: comm=hmfs_discard-26 pid=562 prio=20 target_cpu=004`,
		`        udk-irq-5-78    (    2) [005] .... 6793224.931770: sched_wakeup: comm=hmfs_discard-26 pid=562 prio=20 target_cpu=004`,
		`        udk-irq-5-78    (    2) [005] .... 6793224.931771: sched_blocked_reason: pid=562 iowait=1 caller=0x383435317c45000d`,
		`  hmfs_discard-26-562   (    2) [005] .... 6793224.931789: sched_switch: prev_comm=udk-irq-5 prev_pid=78 prev_prio=142 prev_state=S ==> next_comm=hmfs_discard-26 next_pid=562 next_prio=20`,
		`    tppmgr-idle-5-190   (    2) [005] .... 6793224.931802: sched_switch: prev_comm=hmfs_discard-26 prev_pid=562 prev_prio=20 prev_state=S ==> next_comm=tppmgr-idle-5 next_pid=0 next_prio=-2`,
		`        udk-irq-5-78    (    2) [005] .... 6793224.938261: sched_waking: comm=hmfs_discard-26 pid=562 prio=20 target_cpu=005`,
		`        udk-irq-5-78    (    2) [005] .... 6793224.938268: sched_wakeup: comm=hmfs_discard-26 pid=562 prio=20 target_cpu=007`,
		`        udk-irq-5-78    (    2) [005] .... 6793224.938269: sched_blocked_reason: pid=562 iowait=1 caller=0xa65f000080700018`,
		`  hmfs_discard-26-562   (    2) [007] .... 6793224.938280: sched_switch: prev_comm=tppmgr-idle-7 prev_pid=0 prev_prio=-2 prev_state=R+ ==> next_comm=hmfs_discard-26 next_pid=562 next_prio=20`,
		`    tppmgr-idle-7-192   (    2) [007] .... 6793224.938299: sched_switch: prev_comm=hmfs_discard-26 prev_pid=562 prev_prio=20 prev_state=S ==> next_comm=tppmgr-idle-7 next_pid=0 next_prio=-2`,
		// The target's ONE real D interval (0.011ms, the rank#4 witness).
		`    tppmgr-idle-5-190   (    2) [005] .... 6793224.925415: sched_switch: prev_comm=oney.hmn.berlin prev_pid=42591 prev_prio=53 prev_state=D ==> next_comm=tppmgr-idle-5 next_pid=0 next_prio=-2`,
		`    tppmgr-idle-5-190   (    2) [005] .... 6793224.925426: sched_waking: comm=oney.hmn.berlin pid=42591 prio=53 target_cpu=005`,
		`    tppmgr-idle-5-190   (    2) [005] .... 6793224.925430: sched_wakeup: comm=oney.hmn.berlin pid=42591 prio=53 target_cpu=005`,
		`  oney.hmn.berlin-42591 (42591) [005] .... 6793224.925470: sched_switch: prev_comm=tppmgr-idle-5 prev_pid=0 prev_prio=-2 prev_state=R+ ==> next_comm=oney.hmn.berlin next_pid=42591 next_prio=53`,
		"",
	}
	// Chronological order (the D rows above are earlier than the S+iowait
	// cycles in wall clock — sort by embedding them in emission order).
	sorted := []string{
		rows[15], rows[16], rows[17], rows[18],
		rows[0], rows[1], rows[2], rows[3], rows[4],
		rows[5], rows[6], rows[7], rows[8], rows[9],
		rows[10], rows[11], rows[12], rows[13], rows[14],
		"",
	}
	return buildTraceIndex(t, "g12_platform.systrace", strings.Join(sorted, "\n"))
}

func g12PlatformQuery() Query {
	return Query{TimeStart: 6793224.900, TimeEnd: 6793225.000, Limit: 40}
}

// TestG12SPlusIOWaitNeverEntersDStateLanes: the S+iowait thread must not
// appear in DStateTop/IOWaitTop (its sleeps opened from prev_state=S), while
// the target's real D interval books normally. This is the platform-semantics
// half of the G12 勘察: hmfs_discard physically has no 14.272ms-class D or
// iowait wall clock anywhere, so no engine lane can attribute one.
func TestG12SPlusIOWaitNeverEntersDStateLanes(t *testing.T) {
	idx := g12PlatformTrace(t)
	stats := ComputeWindowStats(idx, g12PlatformQuery())
	for _, td := range stats.DStateTop {
		if td.Thread.PID == 562 {
			t.Fatalf("S+iowait sleeps must not book into DStateTop: %+v", td)
		}
	}
	for _, td := range stats.IOWaitTop {
		if td.Thread.PID == 562 {
			t.Fatalf("S+iowait sleeps must not book into IOWaitTop: %+v", td)
		}
	}
	foundTargetD := false
	for _, td := range stats.DStateTop {
		if td.Thread.PID == 42591 {
			foundTargetD = true
			if td.DurationMs > 0.1 {
				t.Fatalf("target D interval stays ~0.011ms, got %.3f", td.DurationMs)
			}
		}
	}
	if !foundTargetD {
		t.Fatalf("the target's real D interval must book into DStateTop: %+v", stats.DStateTop)
	}
	// The marker rows themselves stay attributed to the payload pid.
	if len(stats.BlockedReasons) == 0 {
		t.Fatalf("expected blocked_reason marker rows")
	}
	for _, br := range stats.BlockedReasons {
		if br.Thread.PID != 562 || br.IOWait != 1 {
			t.Fatalf("blocked_reason attribution must follow the payload pid: %+v", br)
		}
	}
}

// TestG12CriticalBlockingNoCrossThreadSameValuePair: the critical_blocking
// lane mints the S+iowait thread's rows ONLY as zero-duration blocked_reason
// markers; its d_state_or_io_wait rows name the target alone, and no two
// candidates of different subjects share the same positive duration (the
// engine-side 双归属绝迹 assertion behind the E23 display finding).
func TestG12CriticalBlockingNoCrossThreadSameValuePair(t *testing.T) {
	idx := g12PlatformTrace(t)
	res := BuildCriticalBlockingCalls(idx, g12PlatformQuery())
	for _, item := range res.Items {
		switch item.Type {
		case "blocked_reason":
			if item.DurationMs != 0 {
				t.Fatalf("blocked_reason rows are zero-duration markers: %+v", item)
			}
		case "d_state_or_io_wait":
			if item.Thread.PID == 562 {
				t.Fatalf("the S+iowait thread must not mint a d_state_or_io_wait candidate: %+v", item)
			}
		}
	}
	type key struct{ v float64 }
	subjectsByValue := map[key]map[int]bool{}
	for _, item := range res.Items {
		if item.DurationMs <= 0 {
			continue
		}
		k := key{item.DurationMs}
		if subjectsByValue[k] == nil {
			subjectsByValue[k] = map[int]bool{}
		}
		subjectsByValue[k][item.Thread.PID] = true
		if len(subjectsByValue[k]) > 1 {
			t.Fatalf("cross-thread same-value candidate pair minted for %.3fms: %+v", item.DurationMs, res.Items)
		}
	}
}
