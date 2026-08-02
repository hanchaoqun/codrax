package tool

// answer_document_projection_dio_partition_test.go — §29.50.5 证明分区 display
// pins (v5 P1 批 件②, 2026-07-13): the proof partition's THREE faces through
// the full chain (trace → Run(frame_root_cause_bundle) → typed observations →
// compile → tree model → fence/detail):
//   - the cause seat wears the proven 内核调用点 word (cross-token D+iowait
//     merged into ONE seat);
//   - the honest remainder wears the 「(原因未证)」 qualifier (§29.50.5
//     「D-state(原因未证)」类诚实余数);
//   - the same-thread partition never re-merges at any display fold, and the
//     absorbed chain rows hold no seats (CASE-1 symmetry).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// dioPartitionDisplayTrace mirrors the tracequery-side
// dioPartitionPartialTrace verbatim (intentional cross-package double-write):
// seg1 D cpu2 20ms proves dma_fence_default_wait (iowait=0 marker); seg2
// cpu3 30ms proves THE SAME symbol via iowait=1 (cross-token); seg3 cpu4
// 10ms carries no marker (honest remainder). worker-200 wakes the target
// at 3.119 so its seats ride the ON-CHAIN lane (the pre-aggregation
// background bucket cap must not evict the remainder in this fixture).
const dioPartitionDisplayTrace = `
        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.030000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
       peer-300 (300) [003] .... 3.030000: sched_blocked_reason: pid=200 iowait=0 caller=dma_fence_default_wait+0x74/0x160[sysmgr.elf] delay=842
     worker-200 (200) [003] .... 3.031000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.040000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.070000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=004
       peer-300 (300) [003] .... 3.070000: sched_blocked_reason: pid=200 iowait=1 caller=dma_fence_default_wait+0x88/0x160[sysmgr.elf] delay=913
     worker-200 (200) [004] .... 3.071000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [004] .... 3.080000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.090000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=004
     worker-200 (200) [004] .... 3.091000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [004] .... 3.119000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (200) [004] .... 3.119500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120
        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func TestDIOPartitionDisplayRemainderWordEndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dio_partition_display.systrace")
	if err := os.WriteFile(path, []byte(dioPartitionDisplayTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "frame_root_cause_bundle", PID: 100, TimeStart: 3.0, TimeEnd: 3.2, MinDurationMs: 0.05})
	records := traceQueryTypedObservations(result, "dio_partition_display.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	if len(records) == 0 {
		t.Fatal("engine fixture minted no observation records")
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(projection, evidence, true)
	var rows []runtimeTraceProjTreeRow
	for _, group := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		rows = append(rows, group...)
	}
	var cause, remainder *runtimeTraceProjTreeRow
	for i := range rows {
		row := &rows[i]
		if row.Node.AbsorbedByRankFamily {
			t.Fatalf("absorbed chain row must hold no seat: %+v", row.Node)
		}
		if !strings.Contains(row.Node.Subject, "worker-200") {
			continue
		}
		if row.Node.BlockedReasonCaller == "dma_fence_default_wait" {
			if cause != nil {
				t.Fatalf("exactly one cause seat may render, found a second: %+v", row.Node)
			}
			cause = row
			continue
		}
		if row.Node.DStateCauseUnprovenRemainder {
			if remainder != nil {
				t.Fatalf("exactly one remainder seat may render, found a second: %+v", row.Node)
			}
			remainder = row
		}
	}
	if cause == nil {
		t.Fatalf("the proven cause seat must render; rows: %d", len(rows))
	}
	if remainder == nil {
		t.Fatal("the honest remainder seat must render with the typed marker")
	}
	// The remainder wears the 「(原因未证)」 qualifier on its cause word.
	remainderWord := runtimeTraceCausalProjectionDisplayCauseNameNode(remainder.Node, true)
	if !strings.Contains(remainderWord, "(原因未证)") {
		t.Fatalf("remainder cause word must wear the 原因未证 qualifier, got %q", remainderWord)
	}
	causeWord := runtimeTraceCausalProjectionDisplayCauseNameNode(cause.Node, true)
	if strings.Contains(causeWord, "原因未证") {
		t.Fatalf("the cause seat must never wear the remainder qualifier, got %q", causeWord)
	}
	// Rendered faces: the 内核调用点 word rides the cause seat's fence 行2
	// (the rcr identity lane) and the remainder qualifier reaches the fence.
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "内核调用点 dma_fence_default_wait") {
		t.Fatalf("cause seat fence 行2 must carry the 内核调用点 word:\n%s", fence)
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	full := fence + "\n" + detail
	if !strings.Contains(full, "(原因未证)") {
		t.Fatalf("the remainder qualifier must reach a rendered face:\n%s", full)
	}
	// EN parity for the remainder qualifier word home.
	remainderWordEN := runtimeTraceCausalProjectionDisplayCauseNameNode(remainder.Node, false)
	if !strings.Contains(remainderWordEN, "(cause unproven)") {
		t.Fatalf("EN remainder word must carry the qualifier, got %q", remainderWordEN)
	}
	// CASE-3 terminal form (§29.52 独立立案 CASE-3; W-A 双行存续 + WO-C1
	// 关系句): the chain-projection account row and the full-window
	// mutually-exclusive account row of one thread BOTH persist and wear the
	// account-relation sentence pair (链上聚合归账 vs 状态类互斥归账) — the
	// dual-rank adjudication's resting shape until a typed account identity
	// becomes provable (these two accounts are NOT identities by design).
	if !strings.Contains(fence, "按链上聚合归账") || !strings.Contains(fence, "按状态类互斥归账") {
		t.Fatalf("the CASE-3 dual-account relation sentences must ride both rows:\n%s", fence)
	}
	if !strings.Contains(fence, "物理时间重叠(不可相加)") {
		t.Fatalf("the non-additive qualifier must ride the dual-account pair:\n%s", fence)
	}
}
