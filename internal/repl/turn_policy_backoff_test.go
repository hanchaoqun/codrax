package repl

import (
	"bytes"
	"strings"
	"testing"
)

// turn_policy_backoff_test.go — CHATFIX-1 (customer log 2026-08-10):
// after turnPolicyTimeoutBackoffThreshold consecutive classifier
// timeouts the per-turn classifier is skipped for the rest of the
// session, with a single visible notice. On slow self-hosted models
// every turn otherwise pays the full 10s deadline as dead wait.

func TestTurnPolicyBackoffSkipsClassifierAfterConsecutiveTimeouts(t *testing.T) {
	out := &bytes.Buffer{}
	r := newTestREPL(nil, strings.NewReader(""), out)

	if !r.turnPolicyClassifierAvailable() {
		t.Fatal("fresh session must run the classifier")
	}
	r.turnPolicyTimeoutStreak = turnPolicyTimeoutBackoffThreshold - 1
	if !r.turnPolicyClassifierAvailable() {
		t.Fatal("below-threshold streak must still run the classifier")
	}
	r.turnPolicyTimeoutStreak = turnPolicyTimeoutBackoffThreshold
	if r.turnPolicyClassifierAvailable() {
		t.Fatal("at-threshold streak must skip the classifier")
	}
	first := out.String()
	if !strings.Contains(first, "mostly skipping it for this session") {
		t.Fatalf("backoff must disclose itself once, got %q", first)
	}
	if r.turnPolicyClassifierAvailable() {
		t.Fatal("backoff must stay active")
	}
	if got := out.String(); strings.Count(got, "mostly skipping it for this session") != 1 {
		t.Fatalf("the notice must print exactly once, got %q", got)
	}
	// 复核 F-E: every turnPolicyBackoffProbeEvery-th skipped turn runs
	// the classifier as a recovery probe (backoff is never irreversible).
	probed := false
	for i := 0; i < turnPolicyBackoffProbeEvery+1; i++ {
		if r.turnPolicyClassifierAvailable() {
			probed = true
			break
		}
	}
	if !probed {
		t.Fatal("the backoff must periodically probe the classifier")
	}
}

func TestTurnPolicyBackoffStreakResetReenablesClassifier(t *testing.T) {
	out := &bytes.Buffer{}
	r := newTestREPL(nil, strings.NewReader(""), out)
	r.turnPolicyTimeoutStreak = turnPolicyTimeoutBackoffThreshold
	if r.turnPolicyClassifierAvailable() {
		t.Fatal("premise: backoff active")
	}
	// A later classifier success (e.g. operator rerouted to a faster
	// model mid-session and streak reset ran) re-enables the lane.
	r.turnPolicyTimeoutStreak = 0
	if !r.turnPolicyClassifierAvailable() {
		t.Fatal("reset streak must re-enable the classifier")
	}
}
