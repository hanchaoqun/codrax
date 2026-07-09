package orchestrator

// HYG-2 G18 pins (§27.5 / §28.7, docs/design/real_trace_campaign_20260705.md):
// the answer reviewer's FINAL ANSWER BODY truncation is the customer-facing
// out-of-domain site ranked first in the HYG-2 filing — a raw byte slice at
// maxAnswerBytes split a CJK rune and pushed U+FFFD mojibake into the
// reviewer prompt. The cut must land on a rune boundary while staying
// byte-identical for ASCII input.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHYG2AnswerReviewerFinalAnswerRuneSafeTruncation(t *testing.T) {
	// 2000 CJK runes = 6000 bytes > 4096; byte 4096 lands mid-rune
	// (4096 % 3 == 1), so the legacy s[:4096] cut manufactured a broken
	// rune right at the "## Final answer" head's tail.
	answer := strings.Repeat("延", 2000)
	out := renderAnswerReviewerUserMessage(AnswerReviewerInput{FinalAnswer: answer})
	if !utf8.ValidString(out) {
		t.Fatalf("reviewer user message carries invalid UTF-8 after truncation")
	}
	if strings.Contains(out, "�") {
		t.Fatalf("reviewer user message carries U+FFFD mojibake:\n%s", out)
	}
	if !strings.Contains(out, "…(truncated)") {
		t.Fatalf("truncation disclosure missing from reviewer user message")
	}
	if !strings.Contains(out, "延延延") {
		t.Fatalf("answer head content missing from reviewer user message")
	}
}

func TestHYG2AnswerReviewerFinalAnswerASCIIByteIdentity(t *testing.T) {
	answer := strings.Repeat("a", 5000)
	out := renderAnswerReviewerUserMessage(AnswerReviewerInput{FinalAnswer: answer})
	// ASCII keeps the legacy shape byte-for-byte: exactly 4096 kept bytes.
	if !strings.Contains(out, strings.Repeat("a", 4096)+"\n…(truncated)") {
		t.Fatalf("ASCII truncation is no longer byte-identical to the legacy 4096-byte cut")
	}
	if strings.Contains(out, strings.Repeat("a", 4097)) {
		t.Fatalf("ASCII truncation kept more than the 4096-byte budget")
	}
}

// truncateInline is the write-scheduler's two-arm (max<=3 / max>3) inline
// clamp — representative pin for the TrimSpace(s[:max-3])+"..." class.
func TestHYG2TruncateInlineRuneSafe(t *testing.T) {
	out := truncateInline(strings.Repeat("汉", 40), 20)
	if !utf8.ValidString(out) || strings.Contains(out, "�") {
		t.Fatalf("truncateInline produced invalid UTF-8: %q", out)
	}
	if !strings.HasSuffix(out, "...") {
		t.Fatalf("truncateInline lost its ellipsis: %q", out)
	}
	// ASCII identity with the legacy shape.
	if got := truncateInline("abcdef", 5); got != "ab..." {
		t.Fatalf("truncateInline ASCII behavior changed: %q", got)
	}
	if got := truncateInline("abcdef", 3); got != "abc" {
		t.Fatalf("truncateInline max<=3 ASCII behavior changed: %q", got)
	}
}
