package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 11 (R10P-4, R-377-11 ④): the ×N merge of one thread's
// same-kind cause-partition rows keeps the causes on the finalizer handoff,
// the final reader card and the .root-causes.json side file. Real shape: 105
// 元宝 后台销毁冷启动 #2 (TID 58533 bindApplication [10834.099821,
// 10834.620005)): at main the side file read 「阻塞记录调用者=vfs_path_open_ng」
// on the partition rows; after the merge of nine partition seats into one
// 4.256 ms row it named no caller
// (fix10/review-population/out/ybb2/head_ybb2.rootcauses.json).
//
// hmc377R10P4AgentTrace mirrors internal/tool's hmc377R10P4Trace verbatim
// (intentional cross-package double-write): four D waits of the analysed
// thread app-100, each proven by its own blocked_reason caller (alpha 3 ms,
// beta 2 ms, gamma 1 ms, delta 0.5 ms), one 0.8 ms D wait with no marker, and
// a 20 ms runnable wait holding rank #1; worker-200 ends the last sleep.

type hmc377R10P4AgentEvent struct {
	ts   float64
	line string
}

func hmc377R10P4AgentTrace() string {
	var events []hmc377R10P4AgentEvent
	add := func(ts float64, format string, args ...any) {
		events = append(events, hmc377R10P4AgentEvent{ts, fmt.Sprintf(format, args...) + "\n"})
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

func TestHMC377R10P4MergedDIOCausesReachHandoffAndSideFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hmc377_r10p4.systrace")
	if err := os.WriteFile(path, []byte(hmc377R10P4AgentTrace()), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := hmc377CaptureFinalizerContext(t, hmc377Capture{name: "r10p4", tid: 100, start: 10.000, end: 10.100}, path)
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatal(err)
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || !contract.RootCauseReportEnabled {
		t.Fatalf("no root-cause report contract: %+v", contract)
	}
	t.Run("side_file", func(t *testing.T) {
		in := &types.TraceRootCauseReportV2{SchemaVersion: types.TraceRootCauseReportSchemaVersion}
		for _, c := range tracefinding.SelectableRootCauseCandidates(contract) {
			in.RootCauses = append(in.RootCauses, &types.TraceRootCauseItemV2{CandidateID: c.Decision.CandidateID})
		}
		report, err := tracefinding.BindRootCauseReportSelection(in, contract)
		if err != nil {
			t.Fatal(err)
		}
		const want = "阻塞记录调用者=alpha_wait 3.000ms、beta_wait 2.000ms、gamma_wait 1.000ms 等4类；原因未证 0.800ms"
		var evidence []string
		for _, item := range report.RootCauses {
			for _, line := range item.Evidence {
				if strings.Contains(line, "阻塞记录调用者") {
					evidence = append(evidence, line)
				}
				if strings.Contains(line, want) {
					return
				}
			}
		}
		t.Fatalf("the merged D row's side-file evidence must name its causes (%s); caller lines: %q", want, evidence)
	})
	t.Run("finalizer_handoff", func(t *testing.T) {
		const want = "blocked_reason_caller=`alpha_wait 3.000ms, beta_wait 2.000ms, gamma_wait 1.000ms and 1 more; cause unproven 0.800ms`"
		if handoff := renderAnswerDocTraceDecisionHandoff(ctx); !strings.Contains(handoff, want) {
			t.Fatalf("the finalizer handoff must carry the merged row's causes (%s):\n%s", want, handoff)
		}
	})
	t.Run("final_reader_card", func(t *testing.T) {
		const want = "内核记录的等待调用位置为 `alpha_wait 3.000ms、beta_wait 2.000ms、gamma_wait 1.000ms 等4类；原因未证 0.800ms`"
		if card := renderAnswerDocTraceFinalDecisionBoundary(ctx); !strings.Contains(card, want) {
			t.Fatalf("the final reader card must carry the merged row's causes (%s):\n%s", want, card)
		}
	})
}
