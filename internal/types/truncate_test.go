package types

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateBytesEllipsis(t *testing.T) {
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"empty string", "", 10, ""},
		{"ascii under cap", "hello", 10, "hello"},
		{"ascii exactly at cap not truncated", "hello", 5, "hello"},
		{"ascii over cap legacy-identical", "hello world", 5, "hello" + "…"},
		{"non-positive budget is no-op", "hello world", 0, "hello world"},
		{"negative budget is no-op", "hello world", -1, "hello world"},
		// "你好" = E4 BD A0 E5 A5 BD (6 bytes). Cut at 4 lands inside
		// the second rune → back off to 3.
		{"cjk cut mid-rune backs off", "你好", 4, "你" + "…"},
		{"cjk cut on boundary keeps rune", "你好吗", 6, "你好" + "…"},
		{"cjk exactly at cap not truncated", "你好", 6, "你好"},
		// Mixed ASCII + CJK: "ab界" = 61 62 E7 95 8C. Cut at 3 keeps "ab".
		{"mixed ascii cjk mid-rune", "ab界xy", 3, "ab" + "…"},
		{"mixed ascii cjk after lead byte", "ab界xy", 4, "ab" + "…"},
		{"mixed ascii cjk full rune", "ab界xy", 5, "ab界" + "…"},
		// Emoji "🙂" = F0 9F 99 82 (4 bytes).
		{"emoji cut mid-rune", "a🙂b", 3, "a" + "…"},
		{"emoji whole rune kept", "a🙂b", 5, "a🙂" + "…"},
		{"budget smaller than first rune", "界x", 2, "…"},
		// "界" is 3 bytes > budget 1 → prefix backs off to 0 bytes → bare ellipsis.
		{"budget 1 on multibyte start", "界", 1, "…"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := TruncateBytesEllipsis(c.s, c.max)
			if got != c.want {
				t.Errorf("TruncateBytesEllipsis(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("TruncateBytesEllipsis(%q, %d) = %q is not valid UTF-8", c.s, c.max, got)
			}
		})
	}
}

// TestTruncateBytesEllipsis_LegacyASCIIPin pins the zero-behavior-change
// contract for pure-ASCII input: identical to the legacy byte-slice
// shape `s[:n] + "…"` at every budget.
func TestTruncateBytesEllipsis_LegacyASCIIPin(t *testing.T) {
	s := "abcdefghij"
	for max := 1; max < len(s); max++ {
		want := s[:max] + "…"
		if got := TruncateBytesEllipsis(s, max); got != want {
			t.Errorf("budget %d: got %q, want legacy %q", max, got, want)
		}
	}
}

func TestTruncateBytesEllipsis_NeverManufacturesInvalidUTF8(t *testing.T) {
	inputs := []string{
		"分析线程 16547 在 33872.289161s 至 33872.408222s 时间范围内的执行情况，找出导致帧延迟的事件",
		"mixed 中文 and ASCII with emoji 🙂🎯⛔ tail",
		strings.Repeat("界", 100),
	}
	for _, s := range inputs {
		for max := 0; max <= len(s)+1; max++ {
			got := TruncateBytesEllipsis(s, max)
			if !utf8.ValidString(got) {
				t.Fatalf("budget %d over %q produced invalid UTF-8: %q", max, s, got)
			}
			if strings.ContainsRune(got, utf8.RuneError) {
				t.Fatalf("budget %d over %q produced U+FFFD: %q", max, s, got)
			}
		}
	}
}

func TestTruncateRunesEllipsis(t *testing.T) {
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"empty string", "", 5, ""},
		{"n zero keeps nothing", "hello", 0, ""},
		{"n negative keeps nothing", "hello", -2, ""},
		{"n one over-cap collapses to ellipsis", "hello", 1, "…"},
		{"n one exact single rune passes", "h", 1, "h"},
		{"n one exact single cjk rune passes", "界", 1, "界"},
		{"ascii under cap", "hello", 10, "hello"},
		{"ascii exactly at cap not truncated", "hello", 5, "hello"},
		{"ascii over cap keeps n-1 plus ellipsis", "hello!", 5, "hell…"},
		{"cjk exactly at cap not truncated", "你好吗", 3, "你好吗"},
		{"cjk over cap keeps n-1 plus ellipsis", "你好吗吧", 3, "你好…"},
		{"mixed over cap", "a界b🙂c", 4, "a界b…"},
		{"mixed exactly at cap", "a界b🙂c", 5, "a界b🙂c"},
		{"emoji counted as one rune", "🙂🙂🙂", 2, "🙂…"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := TruncateRunesEllipsis(c.s, c.max)
			if got != c.want {
				t.Errorf("TruncateRunesEllipsis(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("result %q is not valid UTF-8", got)
			}
			if c.max >= 0 && utf8.RuneCountInString(got) > c.max {
				t.Errorf("result %q has %d runes, cap %d", got, utf8.RuneCountInString(got), c.max)
			}
		})
	}
}

func TestCutPrefixRuneSafe(t *testing.T) {
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"empty", "", 4, ""},
		{"zero budget", "abc", 0, ""},
		{"negative budget", "abc", -3, ""},
		{"fits entirely", "abc", 3, "abc"},
		{"fits with room", "abc", 10, "abc"},
		{"ascii cut", "abcdef", 4, "abcd"},
		{"cjk boundary exact", "你好", 3, "你"},
		{"cjk mid-rune backs off", "你好", 4, "你"},
		{"cjk mid-rune backs off twice", "你好", 5, "你"},
		{"budget below first rune", "你好", 2, ""},
		{"emoji mid-rune", "🙂x", 3, ""},
		{"emoji boundary", "🙂x", 4, "🙂"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CutPrefixRuneSafe(c.s, c.max)
			if got != c.want {
				t.Errorf("CutPrefixRuneSafe(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("result %q is not valid UTF-8", got)
			}
			if len(got) > c.max && c.max >= 0 {
				t.Errorf("result %q is %d bytes, budget %d", got, len(got), c.max)
			}
		})
	}
}

func TestCutSuffixRuneSafe(t *testing.T) {
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"empty", "", 4, ""},
		{"zero budget", "abc", 0, ""},
		{"negative budget", "abc", -1, ""},
		{"fits entirely", "abc", 3, "abc"},
		{"ascii cut keeps tail", "abcdef", 4, "cdef"},
		{"cjk boundary exact", "你好", 3, "好"},
		{"cjk mid-rune advances", "你好", 4, "好"},
		{"cjk mid-rune advances twice", "你好", 5, "好"},
		{"budget below last rune", "你好", 2, ""},
		{"emoji mid-rune", "x🙂", 3, ""},
		{"emoji boundary", "x🙂", 4, "🙂"},
		{"mixed tail", "界abc", 4, "abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CutSuffixRuneSafe(c.s, c.max)
			if got != c.want {
				t.Errorf("CutSuffixRuneSafe(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("result %q is not valid UTF-8", got)
			}
			if c.max >= 0 && len(got) > c.max {
				t.Errorf("result %q is %d bytes, budget %d", got, len(got), c.max)
			}
		})
	}
}
