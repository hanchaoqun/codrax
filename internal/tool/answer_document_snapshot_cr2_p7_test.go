package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CR-2 组③ P7 — Trace 指标快照覆盖口径守真 (ledger §29.42 P7 / §29.49 F5-1
// 已立案复现, 2026-07-12; witnesses: donghu 20260712-133933 CompThread 「数据
// 实际覆盖 13762.988–13763.010(21.885ms)」 while the thread's raw events span
// the whole window — the "coverage" is the underlying segment envelope of ONE
// chain episode; tieba 20260712-135155 main thread 「running 0.000ms …
// 数据实际覆盖 34579.577–34579.588」 while raw full-window running is 26.9ms —
// the per-state durations are chain-episode-scoped values read as full-window
// statistics). 判据: churn/线程合计行必须披露真实覆盖口径(typed:事件覆盖窗
// vs 活动切片),「数据实际覆盖」词面禁自由文本.

func cr2P7SnapshotWakeupRecord() types.ObservationRecord {
	return types.ObservationRecord{
		Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:  "trace_query",
		Predicate: "wakeup_causal_impact",
		Subject:   "com.example.app-1",
		Value:     "11.103",
		Unit:      "ms",
		RichNotes: []string{
			"dominant_state=s_sleep",
			"running=0.000ms", "runnable=0.025ms", "sleep=11.103ms", "d_state=0.000ms", "io_wait=0.000ms",
			"fragments=2", "switches=1", "max_segment=11.103ms", "p95_segment=11.103ms",
			"selected_window=34579.473..34579.588",
			"actual_window=34579.577..34579.588",
			"actual_runnable=0.025", "actual_sleep=11.103",
		},
	}
}

// F5-1 形 pin ①: the aligned actual-window clause must speak the segment-span
// caliber, never the 「数据实际覆盖」 full-window-coverage reading.
func TestCR2P7SnapshotActualInlineSpeaksSegmentSpanCaliber(t *testing.T) {
	got := runtimeTraceMetricSnapshotActualInline(cr2P7SnapshotWakeupRecord(), true)
	if strings.Contains(got, "数据实际覆盖") {
		t.Fatalf("the segment envelope must not claim data coverage: %q", got)
	}
	if !strings.Contains(got, "实际状态段跨度 34579.577s~34579.588s(活动切片,非全窗事件覆盖)") {
		t.Fatalf("the clause must carry the typed segment-span caliber: %q", got)
	}
}

// F5-1 形 pin ②: a wakeup-lane snapshot row's per-state durations are
// chain-episode-scoped — the head must say so, and the query window must read
// as search scope, not accounting scope.
func TestCR2P7SnapshotEpisodeScopedHeadForWakeupLanes(t *testing.T) {
	text := runtimeTraceMetricSnapshotDisplayText(cr2P7SnapshotWakeupRecord(), true)
	if !strings.Contains(text, "链上发生段内状态时长") {
		t.Fatalf("wakeup-lane snapshot rows must disclose the episode scope:\n%s", text)
	}
	if !strings.Contains(text, "非查询窗全量") {
		t.Fatalf("the head must say the values are not full-window statistics:\n%s", text)
	}
	if !strings.Contains(text, "查询窗 34579.473s~34579.588s(检索范围,非该行统计范围)") {
		t.Fatalf("the window basis must read as search scope:\n%s", text)
	}
	if strings.Contains(text, "状态时长(括号为占该线程观测时长比例)") {
		t.Fatalf("the full-window churn head must not render on an episode-scoped record:\n%s", text)
	}
}

// donghu replay witness (2026-07-12): a chain-derived RANK record qualifies
// for the snapshot through its summary tokens while carrying a root_cause_*
// predicate — the actual_window note (occurrence-scoped lanes only) must
// carry the episode verdict there.
func TestCR2P7SnapshotEpisodeScopedRankLaneViaActualWindow(t *testing.T) {
	record := cr2P7SnapshotWakeupRecord()
	record.Predicate = "root_cause_tertiary"
	text := runtimeTraceMetricSnapshotDisplayText(record, true)
	if !strings.Contains(text, "链上发生段内状态时长") {
		t.Fatalf("a rank-lane record carrying actual_window must disclose the episode scope:\n%s", text)
	}
}

// Control: a real state_churn record (query-window accumulation) keeps the
// legacy head byte-for-byte — the scope fork is predicate-typed, never a
// heuristic.
func TestCR2P7SnapshotChurnHeadUnchanged(t *testing.T) {
	record := cr2P7SnapshotWakeupRecord()
	record.Predicate = "state_churn"
	text := runtimeTraceMetricSnapshotDisplayText(record, true)
	if !strings.Contains(text, "状态时长(括号为占该线程观测时长比例)") {
		t.Fatalf("churn records keep the legacy head:\n%s", text)
	}
	if strings.Contains(text, "链上发生段内状态时长") {
		t.Fatalf("churn records must not claim episode scope:\n%s", text)
	}
}
