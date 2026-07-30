package render

import (
	"strings"
	"testing"
	"time"
)

// PIB-5 (ledger docs/design/pi_borrow_analysis_20260729.md §3.2 item
// 3): run-cumulative token usage on the dock and context-watermark
// hue thresholds. Usage was captured per-response since day one but
// had ZERO consumers — these pins make the display lane the first.

func TestDockUsageAccount_AccumulatesAndResets(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	emit(Event{Kind: EventObjectiveStarted, Timestamp: time.Now(), Objective: "q1"})
	emit(Event{Kind: EventAgentResponse, Timestamp: time.Now(), UsageInputTokens: 1200, UsageOutputTokens: 300})
	emit(Event{Kind: EventAgentResponse, Timestamp: time.Now(), UsageInputTokens: 800, UsageOutputTokens: 200})

	r.mu.Lock()
	in, out := r.usageInputTotal, r.usageOutputTotal
	rows := r.composeCurrentDockRows()
	r.mu.Unlock()
	if in != 2000 || out != 500 {
		t.Fatalf("usage totals = %d/%d, want 2000/500", in, out)
	}
	plain := stripAnsiEscapes(rows[1])
	if !strings.Contains(plain, "↑2.0k ↓500 tok") {
		t.Errorf("row2 missing usage segment: %q", plain)
	}

	// SWEEPFIX S12: the per-turn boundary is pipeline start ONLY — a
	// fresh turn starts a fresh account there, while objective start
	// (which fires AFTER analyze in read mode) must not wipe the
	// analyzer call's tokens.
	emit(Event{Kind: EventPipelineStart, Timestamp: time.Now()})
	r.mu.Lock()
	in, out = r.usageInputTotal, r.usageOutputTotal
	r.mu.Unlock()
	if in != 0 || out != 0 {
		t.Errorf("pipeline start must reset usage totals; got %d/%d", in, out)
	}
}

func TestMetaUsagePhrase_HiddenUntilReported(t *testing.T) {
	if got := metaUsagePhrase(0, 0); got != "" {
		t.Errorf("zero usage must hide the segment; got %q", got)
	}
	if got := metaUsagePhrase(999, 0); got != "↑999 ↓0 tok" {
		t.Errorf("usage phrase = %q", got)
	}
}

// TestContextTokensStyleFor pins the watermark thresholds: ≥90% error
// red, ≥70% warning yellow, else calm meta gray; no window = gray.
func TestContextTokensStyleFor(t *testing.T) {
	cases := []struct {
		used, window int
		want         interface{}
	}{
		{50_000, 100_000, statusMeta},        // 50%
		{69_999, 100_000, statusMeta},        // 69.9%
		{70_000, 100_000, statusRecoverable}, // 70%
		{89_999, 100_000, statusRecoverable}, // 89.9%
		{90_000, 100_000, statusFatal},       // 90%
		{99_000, 100_000, statusFatal},
		{99_000, 0, statusMeta}, // unknown window: never alarm
	}
	for _, tc := range cases {
		if got := contextTokensStyleFor(tc.used, tc.window); got != tc.want {
			t.Errorf("style(%d/%d) wrong hue", tc.used, tc.window)
		}
	}
}

// TestDockUsageAccount_ResetsOnPipelineStart pins REGFIX-C #12: the
// per-turn boundary is pipeline start, not the first objective (which
// fires after analyze and never in write mode) — otherwise the dock
// shows a stale token account carried over from a previous turn.
func TestDockUsageAccount_ResetsOnPipelineStart(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	emit(Event{Kind: EventAgentResponse, Timestamp: time.Now(), UsageInputTokens: 5000, UsageOutputTokens: 900})
	r.mu.Lock()
	stale := r.usageInputTotal
	r.mu.Unlock()
	if stale != 5000 {
		t.Fatalf("precondition: usage should have accumulated; got %d", stale)
	}

	emit(Event{Kind: EventPipelineStart, Timestamp: time.Now()})
	r.mu.Lock()
	in, out := r.usageInputTotal, r.usageOutputTotal
	r.mu.Unlock()
	if in != 0 || out != 0 {
		t.Fatalf("pipeline start must reset the usage account; got %d/%d", in, out)
	}
}

// TestDockUsageAccount_ObjectiveStartDoesNotReset pins SWEEPFIX S12:
// the per-turn boundary is pipeline start ONLY — the first objective
// fires after analyze in read mode, and re-zeroing there wiped the
// analyzer call's tokens from the turn's account.
func TestDockUsageAccount_ObjectiveStartDoesNotReset(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	emit(Event{Kind: EventPipelineStart, Timestamp: time.Now()})
	emit(Event{Kind: EventAgentResponse, Timestamp: time.Now(), UsageInputTokens: 8000, UsageOutputTokens: 700})
	emit(Event{Kind: EventObjectiveStarted, Timestamp: time.Now(), Objective: "定位崩溃根因"})
	r.mu.Lock()
	in := r.usageInputTotal
	r.mu.Unlock()
	if in != 8000 {
		t.Fatalf("objective start must NOT wipe the analyze-stage tokens; got %d, want 8000", in)
	}
}
