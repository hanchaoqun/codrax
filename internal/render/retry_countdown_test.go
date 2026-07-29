package render

import (
	"strings"
	"testing"
	"time"
)

// PIB-1 (pi borrow batch 1, docs/design/pi_borrow_analysis_20260729.md):
// the adapter retry backoff window must render as a LIVE countdown with
// the retry-budget denominator ("第 2/5 次重试 · 还剩 7s") instead of the
// frozen "8s 后第 2 次重试" string, and the durable/non-TTY records must
// carry the same "N/M" attempt label. These tests pin the countdown
// transform, the wording forms, and the event→state plumbing.

// TestRetryingActivityWithCountdown_Transform pins the compose-time
// transform contract: deadline-bearing retry states gain a live
// remaining value; everything else passes through untouched.
func TestRetryingActivityWithCountdown_Transform(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

	// 7.2s remaining → ceil to 8s.
	s := retryingActivityWithCountdown(activityState{
		kind:     activityRetrying,
		deadline: now.Add(7200 * time.Millisecond),
	}, now)
	if !s.retryCountdownActive || s.retryRemainingSec != 8 {
		t.Errorf("future deadline: active=%v remaining=%d, want active=true remaining=8 (ceil)", s.retryCountdownActive, s.retryRemainingSec)
	}

	// Deadline already passed → remaining clamps to 0 (re-sent form).
	s = retryingActivityWithCountdown(activityState{
		kind:     activityRetrying,
		deadline: now.Add(-time.Second),
	}, now)
	if !s.retryCountdownActive || s.retryRemainingSec != 0 {
		t.Errorf("past deadline: active=%v remaining=%d, want active=true remaining=0", s.retryCountdownActive, s.retryRemainingSec)
	}

	// No deadline (legacy constructor) → untouched, static wording lane.
	s = retryingActivityWithCountdown(activityState{kind: activityRetrying, retryDelaySec: 4}, now)
	if s.retryCountdownActive {
		t.Errorf("zero deadline must keep the static wording lane (countdown inactive)")
	}

	// Non-retry kinds never gain countdown fields from this transform.
	s = retryingActivityWithCountdown(activityState{kind: activityReceiving, deadline: now.Add(time.Second)}, now)
	if s.retryCountdownActive {
		t.Errorf("non-retry kind must pass through untouched")
	}
}

// TestActivityRetryingPhrase_CountdownForms pins the three live wording
// forms in both languages: counting down, re-sent, and the
// no-denominator fallback when the adapter did not publish a budget.
func TestActivityRetryingPhrase_CountdownForms(t *testing.T) {
	counting := activityState{
		kind:                 activityRetrying,
		retryAttempt:         2,
		retryMaxAttempts:     5,
		retryRemainingSec:    7,
		retryCountdownActive: true,
	}
	if got := activityPhrase(counting, "zh"); !strings.Contains(got, "第 2/5 次重试") || !strings.Contains(got, "还剩 7s") {
		t.Errorf("zh counting form wrong: %q", got)
	}
	if got := activityPhrase(counting, "en"); !strings.Contains(got, "attempt 2/5") || !strings.Contains(got, "7s left") {
		t.Errorf("en counting form wrong: %q", got)
	}

	resent := counting
	resent.retryRemainingSec = 0
	if got := activityPhrase(resent, "zh"); !strings.Contains(got, "已重发，等待响应") {
		t.Errorf("zh re-sent form wrong: %q", got)
	}
	if got := activityPhrase(resent, "en"); !strings.Contains(got, "re-sent, awaiting response") {
		t.Errorf("en re-sent form wrong: %q", got)
	}

	// Unknown budget → bare attempt number, never "2/0".
	noBudget := counting
	noBudget.retryMaxAttempts = 0
	for _, lang := range []string{"zh", "en"} {
		got := activityPhrase(noBudget, lang)
		if strings.Contains(got, "/0") {
			t.Errorf("%s: unknown budget must not fabricate a denominator: %q", lang, got)
		}
		if !strings.Contains(got, "2") {
			t.Errorf("%s: attempt number missing: %q", lang, got)
		}
	}

	// Static lane (no countdown info) keeps the pre-PIB-1 wording so
	// legacy constructors stay honest about what they know.
	static := activityState{kind: activityRetrying, retryAttempt: 2, retryDelaySec: 4}
	if got := activityPhrase(static, "zh"); !strings.Contains(got, "4s 后第 2 次重试") {
		t.Errorf("zh static form regressed: %q", got)
	}
}

// TestAdapterRetryEvent_ArmsCountdownAndDenominator pins the
// event→state plumbing: an EventAdapterRetry with a published budget
// must arm the countdown deadline off the event timestamp and store
// the denominator, and the durable scrollback record must carry the
// same "N/M" label.
func TestAdapterRetryEvent_ArmsCountdownAndDenominator(t *testing.T) {
	getLogs := withTempLogger(t)
	r := newTestRenderer("zh")
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{
		Kind:             EventAdapterRetry,
		Timestamp:        t0,
		RetryAttempt:     2,
		RetryMaxAttempts: 5,
		RetryDelay:       8 * time.Second,
		RetryReason:      "rate limit",
	})

	r.mu.Lock()
	activity := r.activity
	r.mu.Unlock()
	if activity.kind != activityRetrying {
		t.Fatalf("activity kind = %v, want activityRetrying", activity.kind)
	}
	if activity.retryMaxAttempts != 5 {
		t.Errorf("retryMaxAttempts = %d, want 5", activity.retryMaxAttempts)
	}
	if want := t0.Add(8 * time.Second); !activity.deadline.Equal(want) {
		t.Errorf("deadline = %v, want event timestamp + delay = %v", activity.deadline, want)
	}

	// The durable record (mirrored to the [render] log) carries the
	// denominator so post-hoc run audits see the budget too.
	if logs := getLogs(); !strings.Contains(logs, "第 2/5 次") {
		t.Errorf("durable retry record missing N/M label; log:\n%s", logs)
	}
}

// TestAdapterRetryEvent_NonTTYCarriesDenominator pins the CI/piped
// lane: the single mirrored line must show "第 2/5 次" when the budget
// is known and must NOT fabricate one when it is not.
func TestAdapterRetryEvent_NonTTYCarriesDenominator(t *testing.T) {
	getLogs := withTempLogger(t)
	r := newTestRenderer("zh")
	r.dockSuppressed = true
	emit := r.Emitter()
	emit(Event{
		Kind:             EventAdapterRetry,
		Timestamp:        time.Now(),
		RetryAttempt:     2,
		RetryMaxAttempts: 5,
		RetryDelay:       1200 * time.Millisecond,
		RetryReason:      "rate limit",
	})
	logs := getLogs()
	if !strings.Contains(logs, "第 2/5 次") {
		t.Errorf("non-TTY retry line missing N/M label; log:\n%s", logs)
	}

	getLogs2 := withTempLogger(t)
	r2 := newTestRenderer("zh")
	r2.dockSuppressed = true
	emit2 := r2.Emitter()
	emit2(Event{
		Kind:         EventAdapterRetry,
		Timestamp:    time.Now(),
		RetryAttempt: 3,
		RetryDelay:   0,
		RetryReason:  "stream first-byte timeout",
	})
	logs2 := getLogs2()
	if strings.Contains(logs2, "/0") {
		t.Errorf("unknown budget must not print a /0 denominator; log:\n%s", logs2)
	}
	if !strings.Contains(logs2, "第 3 次") {
		t.Errorf("bare attempt label missing; log:\n%s", logs2)
	}
}

// TestComposeDockRow1_RetryCountdownLive pins the end-to-end compose
// path: with an armed deadline the rendered row 1 shows the live
// remaining seconds, and once the deadline passes the wording flips to
// the honest re-sent form instead of freezing at "还剩 1s".
func TestComposeDockRow1_RetryCountdownLive(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{
		Kind:             EventAdapterRetry,
		Timestamp:        t0,
		RetryAttempt:     2,
		RetryMaxAttempts: 5,
		RetryDelay:       time.Hour, // far future: remaining stays > 0 during the test
		RetryReason:      "rate limit",
	})
	r.mu.Lock()
	rows := r.composeCurrentDockRows()
	r.mu.Unlock()
	plain := stripAnsiEscapes(rows[0])
	if !strings.Contains(plain, "第 2/5 次重试") || !strings.Contains(plain, "还剩") {
		t.Errorf("live row1 missing countdown wording: %q", plain)
	}

	// Simulate the deadline having passed: re-arm with zero delay.
	emit(Event{
		Kind:             EventAdapterRetry,
		Timestamp:        t0.Add(-time.Minute),
		RetryAttempt:     2,
		RetryMaxAttempts: 5,
		RetryDelay:       0,
		RetryReason:      "rate limit",
	})
	r.mu.Lock()
	rows = r.composeCurrentDockRows()
	r.mu.Unlock()
	plain = stripAnsiEscapes(rows[0])
	if !strings.Contains(plain, "已重发，等待响应") {
		t.Errorf("expired countdown must flip to the re-sent form: %q", plain)
	}
}
