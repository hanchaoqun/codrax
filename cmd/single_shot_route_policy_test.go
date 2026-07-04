package cmd

// single_shot_route_policy_test.go — pins for the 2026-07 data-route
// failure-class fix: the single-shot CLI's one unretryable route
// classification no longer runs under the REPL-interactive 10s wall clock,
// and a timeout degrade is a loud, pinned event instead of a silent WARN.
//
// The deadline-independence behaviour itself (adapter sleeping between the
// two clocks) is pinned on the repl side by
// TestClassifyPolicy_SingleShotLaneSurvivesBetweenDeadlinesSleep; this file
// pins the cmd-side dispatch and the degrade-event wording.

import (
	"context"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/repl"
)

// TestSingleShotRouteDegradeLogLineFormatPinned pins the route-degrade event
// wording (degrade de-silencing). Evals reverse-assert on this exact line and
// log triage keys on the "route-degrade:" prefix — any edit here must update
// those consumers deliberately.
func TestSingleShotRouteDegradeLogLineFormatPinned(t *testing.T) {
	got := singleShotRouteDegradeLogLine(60 * time.Second)
	want := "route-degrade: single-shot classifier timeout after 60s; falling back to read pipeline (data-lane contract unavailable)"
	if got != want {
		t.Fatalf("pinned route-degrade line drifted:\n got: %q\nwant: %q", got, want)
	}
	if got := singleShotRouteDegradeLogLine(45 * time.Second); got != "route-degrade: single-shot classifier timeout after 45s; falling back to read pipeline (data-lane contract unavailable)" {
		t.Fatalf("timeout seconds must render from the configured knob, got %q", got)
	}
}

// laneRecordingClassifier implements both classifier lanes and records which
// one the cmd dispatch used.
type laneRecordingClassifier struct {
	lane string
}

func (c *laneRecordingClassifier) ClassifyPolicy(context.Context, string, string, bool) (repl.TurnPolicy, error) {
	c.lane = "repl"
	return repl.TurnPolicy{}, nil
}

func (c *laneRecordingClassifier) ClassifyPolicySingleShot(context.Context, string, string, bool) (repl.TurnPolicy, error) {
	c.lane = "single_shot"
	return repl.TurnPolicy{}, nil
}

// replOnlyPolicyClassifier implements only the interactive lane, standing in
// for narrow test stubs.
type replOnlyPolicyClassifier struct {
	called bool
}

func (c *replOnlyPolicyClassifier) ClassifyPolicy(context.Context, string, string, bool) (repl.TurnPolicy, error) {
	c.called = true
	return repl.TurnPolicy{}, nil
}

// TestClassifySingleShotPolicyCall_PrefersSingleShotLane structurally pins
// the lane split at the cmd dispatch: when the wired classifier supports the
// single-shot lane it MUST be entered there (own 60s-default deadline, no
// second interactive wrap); the ClassifyPolicy fallback exists only for
// stub classifiers that lack the lane.
func TestClassifySingleShotPolicyCall_PrefersSingleShotLane(t *testing.T) {
	both := &laneRecordingClassifier{}
	if _, err := classifySingleShotPolicyCall(context.Background(), both, "统计各列平均值", ""); err != nil {
		t.Fatalf("classify: %v", err)
	}
	if both.lane != "single_shot" {
		t.Fatalf("lane = %q, want single_shot — cmd dispatch reattached the interactive lane", both.lane)
	}

	narrow := &replOnlyPolicyClassifier{}
	if _, err := classifySingleShotPolicyCall(context.Background(), narrow, "统计各列平均值", ""); err != nil {
		t.Fatalf("classify (narrow stub): %v", err)
	}
	if !narrow.called {
		t.Fatal("narrow stub classifier must fall back to ClassifyPolicy")
	}
}

// TestProductionClassifierImplementsSingleShotLane pins that the classifier
// cmd/root.go actually wires (repl.NewChitchatClassifier) supports the
// single-shot lane, so the interactive fallback in
// classifySingleShotPolicyCall can never be taken in production.
func TestProductionClassifierImplementsSingleShotLane(t *testing.T) {
	c := repl.NewChitchatClassifier(nil)
	if _, ok := c.(repl.SingleShotTurnPolicyClassifier); !ok {
		t.Fatal("production classifier must implement repl.SingleShotTurnPolicyClassifier")
	}
}
