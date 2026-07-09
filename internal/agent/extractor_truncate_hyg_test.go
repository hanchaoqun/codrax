package agent

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// HYG review P1-1: truncateExtractorPromptText hand-rolled its rune-boundary
// walk and kept a dangling lead byte, feeding invalid UTF-8 into LLM prompts
// (REPRO: ("分析线程延迟事件", 7) → "分析\xe7…[truncated]"). Pinned against
// every cut point so a regression to byte slicing bites immediately.
func TestTruncateExtractorPromptTextRuneSafe(t *testing.T) {
	const s = "分析线程延迟事件 latency 分解"
	for max := 1; max <= len(s)+3; max++ {
		got := truncateExtractorPromptText(s, max)
		if !utf8.ValidString(got) {
			t.Fatalf("max=%d: invalid UTF-8: %q", max, got)
		}
		if strings.ContainsRune(got, utf8.RuneError) {
			t.Fatalf("max=%d: replacement char leaked: %q", max, got)
		}
		if max >= len(s) && got != s {
			t.Fatalf("max=%d: full string must pass through, got %q", max, got)
		}
		if max < len(s) && !strings.HasSuffix(got, "…[truncated]") {
			t.Fatalf("max=%d: missing truncation marker: %q", max, got)
		}
	}
}
