package repl

// HYG-2 G18 pins (§27.5): the REPL package's representative sites — the
// cancel-listener warn clamp (former hand-rolled rune backoff, migrated to
// the shared primitive) and readRunCompact's two-arm (max<=3 / max>3) cut.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHYG2TruncateForWarnRuneSafe(t *testing.T) {
	// 40 CJK runes = 120 bytes > 80; byte 80 lands mid-rune (80 % 3 == 2).
	out := truncateForWarn(strings.Repeat("试", 40), 80)
	if !utf8.ValidString(out) || strings.Contains(out, "�") {
		t.Fatalf("truncateForWarn produced invalid UTF-8: %q", out)
	}
	if !strings.HasSuffix(out, "…") {
		t.Fatalf("truncateForWarn lost its ellipsis: %q", out)
	}
	// ASCII identity with the pre-migration hand-rolled loop.
	if got := truncateForWarn(strings.Repeat("a", 100), 80); got != strings.Repeat("a", 80)+"…" {
		t.Fatalf("truncateForWarn ASCII behavior changed: %q", got)
	}
	if got := truncateForWarn("short", 80); got != "short" {
		t.Fatalf("truncateForWarn must pass short input through: %q", got)
	}
}

func TestHYG2ReadRunCompactRuneSafe(t *testing.T) {
	out := readRunCompact(strings.Repeat("码", 100), 50)
	if !utf8.ValidString(out) || strings.Contains(out, "�") {
		t.Fatalf("readRunCompact produced invalid UTF-8: %q", out)
	}
	if !strings.HasSuffix(out, "...") {
		t.Fatalf("readRunCompact lost its ellipsis: %q", out)
	}
	if got := readRunCompact("abcdefgh", 7); got != "abcd..." {
		t.Fatalf("readRunCompact ASCII behavior changed: %q", got)
	}
	if got := readRunCompact("abcdefgh", 3); got != "abc" {
		t.Fatalf("readRunCompact max<=3 ASCII behavior changed: %q", got)
	}
}
