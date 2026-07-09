package tool

// HYG-2 G18 pins (§27.5): representative sites for the guarded
// `s[:n] + suffix` byte-truncation class in the test-runner parsers —
// failure details and stdout heads carry raw test output that can be CJK.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHYG2RunTestsTruncateDetailRuneSafe(t *testing.T) {
	// 2000 CJK runes = 6000 bytes; byte 4000 lands mid-rune (4000 % 3 == 1).
	out := truncateDetail(strings.Repeat("测", 2000), 4000)
	if !utf8.ValidString(out) || strings.Contains(out, "�") {
		t.Fatalf("truncateDetail produced invalid UTF-8 tail: %q", out[len(out)-40:])
	}
	if !strings.HasSuffix(out, "\n…[truncated]") {
		t.Fatalf("truncateDetail lost its truncation marker")
	}
	// ASCII identity with the legacy shape.
	if got := truncateDetail("abcdef", 4); got != "abcd\n…[truncated]" {
		t.Fatalf("truncateDetail ASCII behavior changed: %q", got)
	}
}

func TestHYG2RunTestsStdoutHeadRuneSafe(t *testing.T) {
	out := stdoutHead(strings.Repeat("出", 100), 100)
	if !utf8.ValidString(out) || strings.Contains(out, "�") {
		t.Fatalf("stdoutHead produced invalid UTF-8: %q", out)
	}
	if !strings.HasSuffix(out, "…") {
		t.Fatalf("stdoutHead lost its ellipsis")
	}
	if got := stdoutHead("abcdef", 4); got != "abcd…" {
		t.Fatalf("stdoutHead ASCII behavior changed: %q", got)
	}
}
