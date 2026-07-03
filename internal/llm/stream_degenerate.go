package llm

import "unicode/utf8"

// Client-side degeneration breaker.
//
// Weak models occasionally collapse into a verbatim repetition loop
// (customer dead-session 2026-07-03: the same sentence streamed for
// 14m35s / 24576 tokens / 124KB of assistant content before the
// server-side length ceiling finally fired). Every transport watchdog
// stays silent in that state because bytes AND visible deltas keep
// flowing. The breaker watches only the accumulated assistant content
// (reasoning-only and tool-call runaway are bounded by the visible-
// output and total-wall-clock watchdogs) and fires ONLY on a precise
// verbatim signal: the entire recent tail window is exactly periodic
// with a short byte period. On fire the stream is truncated in place
// (prefix + two periods kept as evidence of what the model repeated),
// finish_reason is set to finishReasonDegenerateRepetition, and the
// response returns as a normal truncated turn — no retry burn; the
// agent's existing soft-stop lane routes the no-tool content forward.
const (
	// degenerateMinContentBytes: never inspect before this much content
	// has accumulated; short answers cannot loop long enough to matter
	// and this keeps the check off the hot path for normal turns.
	degenerateMinContentBytes = 8192
	// degenerateCheckStride: rerun the periodicity check only after this
	// much NEW content since the last check.
	degenerateCheckStride = 2048
	// degenerateTailWindowBytes: the window that must be verbatim
	// periodic end to end for the breaker to fire.
	degenerateTailWindowBytes = 4096
	// degenerateMaxPeriodBytes: only periods up to this length count.
	// window/period ≥ 4 therefore guarantees at least four verbatim
	// repeats inside the window before the breaker can fire.
	degenerateMaxPeriodBytes = 1024
)

// finishReasonDegenerateRepetition is the synthetic finish_reason the
// breaker stamps on a truncated stream. mapFinishReason passes unknown
// values through, so Response.StopReason carries it verbatim; nothing
// downstream matches on it except the Chat() diagnostic log line.
const finishReasonDegenerateRepetition = "degenerate_repetition"

// degenerateTailPeriod returns the smallest period p of s (classic
// prefix-function period: s[i] == s[i-p] for all i ≥ p) when p is at
// most maxPeriod and s spans at least four full periods; otherwise 0.
// The whole window must satisfy the shift — a single divergent byte
// anywhere makes this return 0, which is what keeps the signal precise
// enough to gate a hard stream stop.
func degenerateTailPeriod(s string, maxPeriod int) int {
	n := len(s)
	if n == 0 || maxPeriod <= 0 {
		return 0
	}
	pi := make([]int, n)
	for i := 1; i < n; i++ {
		j := pi[i-1]
		for j > 0 && s[i] != s[j] {
			j = pi[j-1]
		}
		if s[i] == s[j] {
			j++
		}
		pi[i] = j
	}
	p := n - pi[n-1]
	if p > maxPeriod || p <= 0 {
		return 0
	}
	if n/p < 4 {
		return 0
	}
	return p
}

// truncateDegenerateTail walks the verbatim repetition backward from the
// end of s and cuts the flood, keeping everything before the loop plus
// two full periods so the caller (and the user-facing transcript) can
// still see what the model was repeating. The cut lands on a UTF-8 rune
// boundary.
func truncateDegenerateTail(s string, period int) string {
	if period <= 0 || len(s) < 2*period {
		return s
	}
	i := len(s) - period - 1
	for i >= 0 && s[i] == s[i+period] {
		i--
	}
	runStart := i + 1
	keep := runStart + 2*period
	if keep >= len(s) {
		return s
	}
	for keep > 0 && !utf8.RuneStart(s[keep]) {
		keep--
	}
	return s[:keep]
}
