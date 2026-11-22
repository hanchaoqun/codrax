package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// HMC377 round 11 (R10P-4, R-377-11 ④): the ×N merge of one thread's
// same-kind cause-partition rows keeps the causes on the delivered tree.
//
// Real shape: 105 元宝 后台销毁冷启动 #2, TID 58533 bindApplication
// [10834.099821,10834.620005). The engine splits the analysed thread's
// 4.256 ms D account into nine seats by proven blocked_reason caller
// (vfs_path_open_ng 3.435 ms, fscache_readahead_i 0.445 ms, mprotect_range
// 0.170 ms, …); the projection's ×N merge folded them into one row and,
// because the callers differ, the row named none — vfs_path_open_ng was on no
// reader face (fix10/review-population/out/ybb2/head_ybb2.fence_zh.txt
// 「➎ ⛓ 自身·D-state 4.256ms [E6(+8)]」, no 内核调用点; at main the separate
// rows read 「内核调用点 vfs_path_open_ng」).
//
// The synthetic trace below has that shape, run through the public
// trace_query entry and the delivered document: the analysed thread app-100
// waits in D five times — four waits each proven by its own blocked_reason
// caller (alpha 3 ms, beta 2 ms, gamma 1 ms, delta 0.5 ms) and one 0.8 ms wait
// with no marker (the honest remainder) — and a longer 20 ms runnable wait
// holds rank #1, so the D family is not the primary seat (as on ybb2, where
// class verification holds #1). worker-200 ends the thread's last sleep, so
// the chain resolves and the thread's rows render in its own stanza (as on
// ybb2), where the rank row's identity line and the stanza's detail line
// both carry the cause words, as they do for a unanimous caller.

type hmc377R10P4Event struct {
	ts   float64
	line string
}

func hmc377R10P4Trace() string {
	var events []hmc377R10P4Event
	add := func(ts float64, format string, args ...any) {
		events = append(events, hmc377R10P4Event{ts, fmt.Sprintf(format, args...) + "\n"})
	}
	sw := func(ts float64, prevComm string, prevPID int, prevState, nextComm string, nextPID int) {
		add(ts, " %s-%d (%d) [001] .... %.6f: sched_switch: prev_comm=%s prev_pid=%d prev_prio=120 prev_state=%s ==> next_comm=%s next_pid=%d next_prio=120",
			prevComm, prevPID, prevPID, ts, prevComm, prevPID, prevState, nextComm, nextPID)
	}
	add(9.9999, " app-100 (100) [001] .... 9.999900: tracing_mark_write: B|100|bindApplication")
	sw(10.000, "idle/1", 0, "R", "app", 100)
	sw(10.002, "app", 100, "R", "hog", 500)
	sw(10.022, "hog", 500, "S", "app", 100)
	for _, w := range []struct {
		start, end float64
		caller     string
	}{
		{10.030, 10.033, "alpha_wait+0x10/0x20[sysmgr.elf]"},
		{10.040, 10.042, "beta_wait+0x10/0x20[sysmgr.elf]"},
		{10.050, 10.051, "gamma_wait+0x10/0x20[sysmgr.elf]"},
		{10.060, 10.0605, "delta_wait+0x10/0x20[sysmgr.elf]"},
		{10.070, 10.0708, ""},
	} {
		sw(w.start, "app", 100, "D", "idle/1", 0)
		if w.caller != "" {
			add(w.end, " kworker-400 (400) [000] .... %.6f: sched_blocked_reason: pid=100 iowait=0 caller=%s", w.end, w.caller)
		}
		add(w.end, " kworker-400 (400) [000] .... %.6f: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001", w.end)
		sw(w.end+0.00001, "idle/1", 0, "R", "app", 100)
	}
	sw(10.080, "app", 100, "S", "idle/1", 0)
	add(10.0799, " idle/2-0 (0) [002] .... 10.079900: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=120")
	add(10.0900, " worker-200 (200) [002] .... 10.090000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=001")
	add(10.0901, " worker-200 (200) [002] .... 10.090100: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120")
	sw(10.09001, "idle/1", 0, "R", "app", 100)
	add(10.0989, " app-100 (100) [001] .... 10.098900: tracing_mark_write: E|100")
	sw(10.099, "app", 100, "S", "idle/1", 0)
	sort.SliceStable(events, func(i, j int) bool { return events[i].ts < events[j].ts })
	var b strings.Builder
	for _, e := range events {
		b.WriteString(e.line)
	}
	return b.String()
}

func TestHMC377R10P4MergedTargetDIORowNamesItsCauses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hmc377_r10p4.systrace")
	if err := os.WriteFile(path, []byte(hmc377R10P4Trace()), 0o644); err != nil {
		t.Fatal(err)
	}
	d := hmc377DeliverWindow(t, path, 100, 10.000, 10.100)
	if d.rank == nil {
		t.Fatal("no native ranking")
	}
	// Premise: the engine publishes one D seat per proven caller plus the
	// honest remainder, and none of them is the primary seat.
	callers := map[string]bool{}
	remainders := 0
	for _, item := range d.rank.Items {
		if item.Thread.PID != 100 || item.Type != "d_state_or_io_wait" {
			continue
		}
		if item.Rank == 1 {
			t.Fatalf("fixture premise: the D family must not hold rank #1: %+v", item)
		}
		if item.BlockedReasonCaller != "" {
			callers[item.BlockedReasonCaller] = true
		} else if item.DStateCauseUnprovenRemainder {
			remainders++
		}
	}
	if len(callers) != 4 || remainders != 1 {
		t.Fatalf("fixture premise: four proven-cause D seats and one remainder expected, got callers=%v remainders=%d", callers, remainders)
	}
	for _, tc := range []struct {
		lang, doc string
		causes    string
		unproven  string
	}{
		{"zh", d.zh, "内核调用点alpha_wait3.000ms、beta_wait2.000ms、gamma_wait1.000ms等4类", "原因未证0.800ms"},
		{"en", d.en, "kernelwaitcall-sitealpha_wait3.000ms,beta_wait2.000ms,gamma_wait1.000msand1more", "causeunproven0.800ms"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			joined := hmc377Joined(tc.doc)
			if n := strings.Count(joined, tc.causes); n < 2 {
				t.Fatalf("the merged D row must name its causes largest first with the count of the rest (%s) on its identity line and its stanza line, found %d:\n%s", tc.causes, n, tc.doc)
			}
			if !strings.Contains(joined, tc.unproven) {
				t.Fatalf("the merged D row must state the ms no proven cause holds (%s):\n%s", tc.unproven, tc.doc)
			}
		})
	}
}

// The micro anchored-seat fold (RNB-5B 件⑦) is a display fold whose value is
// the account Σ of its member seats; a micro D/IO seat folded into it keeps
// its proven cause. Real shape: 105 抖音极速版 终端重启后冷启动 #2
// (ftrace-20261019_20.22.03), worker 10734 window [144.520342,144.705491)
// (fix10 review-population w42w2): at main the 0.073 ms posix_lock_file D
// seat of 10536 rendered on its own row with 「内核调用点 posix_lock_file」;
// at the batch head it is an anchored cut seat below 0.1 ms, folded into
// 「其余 2 项微额锚定席」, and the cause left the document
// (fix11/dcause/out/pop/base/base_w42w2.fence_zh.txt).
func TestHMC377R10P4MicroAnchorFoldKeepsItsMembersCauses(t *testing.T) {
	projection := rnb5bMicroAnchorFoldProjection()
	for i := range projection.OnChainCauses {
		node := &projection.OnChainCauses[i]
		switch node.EvidenceID {
		case "E-m1":
			node.Object, node.TypeToken, node.StateKind = "d_state_or_io_wait", "d_state_or_io_wait", "d_sleep"
			node.BlockedReasonCaller = "posix_lock_file"
		case "E-m3":
			node.Object, node.TypeToken, node.StateKind = "d_state_or_io_wait", "d_state_or_io_wait", "d_sleep"
			node.BlockedReasonCaller = "sync_buffer_read_wi"
		}
	}
	for _, tc := range []struct {
		lang string
		zh   bool
		want string
	}{
		{"zh", true, "·内核调用点posix_lock_file0.026ms、sync_buffer_read_wi0.016ms"},
		{"en", false, "·kernelwaitcall-siteposix_lock_file0.026ms,sync_buffer_read_wi0.016ms"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), tc.zh)
			fence := runtimeTraceProjTreeFence(model, tc.zh)
			if !strings.Contains(fence, "微额锚定席") && !strings.Contains(fence, "micro anchored seats") {
				t.Fatalf("fixture premise: the micro seats fold:\n%s", fence)
			}
			if joined := hmc377Joined(fence); !strings.Contains(joined, tc.want) {
				t.Fatalf("the micro fold row must keep its members' causes (%s):\n%s", tc.want, fence)
			}
			if detail := hmc377Joined(runtimeTraceProjDetailFullText(model, tc.zh)); !strings.Contains(detail, tc.want) {
				t.Fatalf("the micro fold's detail must keep its members' causes (%s):\n%s", tc.want, detail)
			}
		})
	}
}
