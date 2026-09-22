package tracefence

import "testing"

func TestWaitStateWithOriginalDoesNotInferMissingPhysicalState(t *testing.T) {
	for _, tc := range []struct {
		category, original string
		zh                 bool
		want               string
	}{
		{"调度器标记的 IO 等待", "D", true, "原始调度状态：D；统计类别：调度器标记的 IO 等待"},
		{"IO wait", "D|K", false, "original scheduler state: D|K; accounting category: IO wait"},
		{"interruptible sleep", "S", false, "original scheduler state: S; accounting category: interruptible sleep"},
		{"IO wait", "", false, "IO wait"},
		{"调度器标记的 IO 等待", " ", true, "调度器标记的 IO 等待"},
	} {
		if got := WaitStateWithOriginal(tc.category, tc.original, tc.zh); got != tc.want {
			t.Errorf("%+v: got %q", tc, got)
		}
	}
}
