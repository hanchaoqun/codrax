package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 E3 reader faces: an own phase says how much of its time is its own
// and how that time split between running and waiting; a chain member counted
// only inside the waits it ended says so, with the full span beside it; a
// record without those facts renders exactly as before.
func TestHMC377BusinessClueAccountClauseFaces(t *testing.T) {
	own := types.TraceCausalProjectionBusinessSpanMention{Subject: "app-100", Name: "attachBaseContext", Count: 1, TotalMS: 303.032, MaxMS: 303.032,
		StartLine: 10, EndLine: 20, Basis: "self", WallMS: 303.032, SelfKnown: true, SelfMS: 250.48,
		States: &types.TraceCausalProjectionBusinessSpanStates{RunningMS: 179.581, RunnableMS: 8.047, SleepMS: 102.941, DStateMS: 2.444, IOWaitMS: 10.019}}
	dep := types.TraceCausalProjectionBusinessSpanMention{Subject: "HeapTaskDaemon-11098", Name: "Background concurrent copying GC", Count: 1,
		TotalMS: 122.897, MaxMS: 122.897, StartLine: 30, EndLine: 40, Basis: "chain_member", WallMS: 123.314}
	legacy := types.TraceCausalProjectionBusinessSpanMention{Subject: "Worker-9", Name: "ParseCards", Count: 2, TotalMS: 4.2, MaxMS: 2.1,
		StartLine: 50, EndLine: 60, Basis: "chain_member"}

	zhOwn, ok := runtimeTraceProjBusinessSpanMentionRowText(own, nil, true)
	if !ok || !strings.Contains(zhOwn, "合计303.032ms(其中自身250.480ms;运行 179.581ms、可运行但尚未获调度 8.047ms、可中断睡眠 102.941ms、不可中断等待 2.444ms、IO 等待 10.019ms) 行10..20") {
		t.Fatalf("own phase tree row: %s", zhOwn)
	}
	zhDep, _ := runtimeTraceProjBusinessSpanMentionRowText(dep, nil, true)
	if !strings.Contains(zhDep, "合计122.897ms(仅计其唤醒结束的等待区间内部分,span全长123.314ms) 行30..40 凭证:链上节点") {
		t.Fatalf("dependency part tree row: %s", zhDep)
	}
	enDep, _ := runtimeTraceProjBusinessSpanMentionRowText(dep, nil, false)
	if !strings.Contains(enDep, "total 122.897ms (only the part inside the waits its wakeups ended; full span 123.314ms) · lines 30..40") {
		t.Fatalf("dependency part EN row: %s", enDep)
	}
	compact, _ := runtimeTraceProjBusinessSpanMentionCompactRowText(own, nil, true)
	if !strings.HasSuffix(compact, "(其中自身250.480ms)") || strings.Contains(compact, "运行") {
		t.Fatalf("the compact zone row carries own time without the split: %s", compact)
	}
	legacyRow, _ := runtimeTraceProjBusinessSpanMentionRowText(legacy, nil, true)
	if legacyRow != "Worker-9 ParseCards 单次最大2.100ms/2次 合计4.200ms 行50..60 凭证:链上节点" {
		t.Fatalf("a record without the new facts renders byte-identically: %s", legacyRow)
	}
	// The occupancy table orders by its own ruler: the analysed thread's own
	// phases first, then other threads, each by cumulative wall clock — a
	// longer dependency row never reads ahead of the target's own phase.
	big := dep
	big.TotalMS, big.MaxMS, big.WallMS = 400, 400, 400
	rows := runtimeTraceOccupancyBusinessSpanCandidates(types.TraceCausalProjection{BusinessSpanMentions: []types.TraceCausalProjectionBusinessSpanMention{big, legacy, own}}, true)
	if len(rows) != 3 || !strings.Contains(rows[0].subject, "attachBaseContext") || !strings.Contains(rows[1].subject, "Background concurrent copying GC") || !strings.Contains(rows[2].subject, "ParseCards") {
		t.Fatalf("occupancy business rows must read own phases first, then by total: %+v", rows)
	}
	if !strings.Contains(rows[0].caliber, "其中自身250.480ms") {
		t.Fatalf("occupancy caliber carries the same account clause: %s", rows[0].caliber)
	}
	for _, internal := range []string{"chain_member", "basis", "self_ms", "wall_ms", "dependency window"} {
		for _, row := range []string{zhOwn, zhDep, enDep, compact} {
			if strings.Contains(row, internal) {
				t.Fatalf("reader face leaked internal term %q: %s", internal, row)
			}
		}
	}
}
