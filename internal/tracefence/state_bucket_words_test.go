package tracefence

import "testing"

func TestStateNonIODStateWordKeepsBucketAndPhysicalFoldDistinct(t *testing.T) {
	for _, tc := range []struct {
		zh           bool
		bucket, fold string
	}{
		{true, "D 状态（未标记为 IO 等待）", "不可中断等待"},
		{false, "D-state without an IO-wait mark", "uninterruptible wait"},
	} {
		if got := StateNonIODStateWord(tc.zh); got != tc.bucket {
			t.Errorf("exclusive D bucket word = %q, want %q", got, tc.bucket)
		}
		if got, ok := StateLaneWord(StateLaneDState, tc.zh); !ok || got != tc.fold {
			t.Errorf("physical D+IO fold word changed: %q, %t", got, ok)
		}
	}
}
