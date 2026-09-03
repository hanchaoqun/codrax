package tracefence

// state_lane_words_test.go — V3-1 Table ⑧ pins (colleague_merge_audit
// §40.20 ③, 2026-09-03): the target-state lane words are a deliberate closed
// set read by every prompt face (types.FormatTargetStateAccount plus the
// internal/agent label switches); a change here is a wordface decision.

import (
	"reflect"
	"testing"
)

func TestStateLaneWords_ClosedSet(t *testing.T) {
	wantLanes := []string{"running", "runnable", "sleep", "d_state", "io_wait", "sleep_io_wait"}
	if got := StateLanes(); !reflect.DeepEqual(got, wantLanes) {
		t.Fatalf("target-state lane closed set changed: got %v want %v", got, wantLanes)
	}
	want := map[string][2]string{
		"running":       {"运行", "running"},
		"runnable":      {"可运行但尚未获调度", "runnable but not yet scheduled"},
		"sleep":         {"可中断睡眠", "interruptible sleep"},
		"d_state":       {"不可中断等待", "uninterruptible wait"},
		"io_wait":       {"IO 等待", "IO wait"},
		"sleep_io_wait": {"带 IO 等待标记的可中断睡眠", "interruptible sleep carrying an IO-wait marker"},
	}
	for _, lane := range StateLanes() {
		zh, ok := StateLaneWord(lane, true)
		if !ok || zh != want[lane][0] {
			t.Fatalf("lane %q zh word: got %q ok=%v want %q", lane, zh, ok, want[lane][0])
		}
		en, ok := StateLaneWord(lane, false)
		if !ok || en != want[lane][1] {
			t.Fatalf("lane %q en word: got %q ok=%v want %q", lane, en, ok, want[lane][1])
		}
		if up, ok := StateLaneWord(" "+lane+" ", true); !ok || up != zh {
			t.Fatalf("lane lookup must trim/normalize the wire token: %q", lane)
		}
	}
	for _, unknown := range []string{"", "d", "d_sleep", "total", "uninterruptible", "running_ms"} {
		if word, ok := StateLaneWord(unknown, true); ok || word != "" {
			t.Fatalf("non-lane token %q must resolve ok=false, got %q ok=%v", unknown, word, ok)
		}
	}
	if StateSchedulerMarkedQualifierZH != "调度器标记的" || StateSchedulerMarkedQualifierEN != "scheduler-marked" {
		t.Fatalf("scheduler-marked qualifier drifted: %q / %q", StateSchedulerMarkedQualifierZH, StateSchedulerMarkedQualifierEN)
	}
}

func TestStateCoverageWord_ClosedSet(t *testing.T) {
	cases := map[string][2]string{
		StateCoverageComplete:            {"覆盖完整", "complete coverage"},
		StateCoveragePartialUnaccounted:  {"部分覆盖，仍有未计入时间", "partial coverage with unaccounted time"},
		"window_unknown":                 {"窗口覆盖范围未知", "window coverage unknown"},
		"":                               {"窗口覆盖范围未知", "window coverage unknown"},
		"lower_bound_capacity_truncated": {"窗口覆盖范围未知", "window coverage unknown"},
	}
	for status, want := range cases {
		if got := StateCoverageWord(status, true); got != want[0] {
			t.Fatalf("coverage %q zh: got %q want %q", status, got, want[0])
		}
		if got := StateCoverageWord(status, false); got != want[1] {
			t.Fatalf("coverage %q en: got %q want %q", status, got, want[1])
		}
	}
}
