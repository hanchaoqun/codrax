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
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestSingleShotRouteDegradeLogLineFormatPinned pins the route-degrade event
// wording (degrade de-silencing). Evals reverse-assert on this exact line and
// log triage keys on the "route-degrade:" prefix — any edit here must update
// those consumers deliberately.
// EVOLUTION RECORD (终判⑩ §29.96.2, 2026-07-15): the sample duration follows
// the default knob 60s → 120s (value-only change; the wording is unchanged).
func TestSingleShotRouteDegradeLogLineFormatPinned(t *testing.T) {
	got := singleShotRouteDegradeLogLine(120 * time.Second)
	want := "route-degrade: single-shot classifier timeout after 120s; falling back to read pipeline (data-lane contract unavailable)"
	if got != want {
		t.Fatalf("pinned route-degrade line drifted:\n got: %q\nwant: %q", got, want)
	}
	if got := singleShotRouteDegradeLogLine(45 * time.Second); got != "route-degrade: single-shot classifier timeout after 45s; falling back to read pipeline (data-lane contract unavailable)" {
		t.Fatalf("timeout seconds must render from the configured knob, got %q", got)
	}
}

// TestSingleShotRoutePolicyTimeoutDefaultPinned pins the DEFAULT deadline
// value itself (终判⑩ §29.96.2, 2026-07-15: 60s → 120s, reasoning-model
// tier; DR lane structure untouched — value-only ruling). A silent default
// regression would re-open the DR coin-flip class the 2026-07 attribution
// closed, so the default is a pinned contract, not an incidental literal.
func TestSingleShotRoutePolicyTimeoutDefaultPinned(t *testing.T) {
	if got := repl.SingleShotRoutePolicyTimeout(); got != 120*time.Second {
		t.Fatalf("single-shot route-policy default deadline = %s, want 120s (终判⑩ §29.96.2)", got)
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

type singleShotRouteSinkRecorder struct {
	hint            types.TurnRouteHint
	directive       string
	diagramRequired bool
	hintCalls       int
	directiveCalls  int
	diagramReqCalls int
}

func (r *singleShotRouteSinkRecorder) SetTurnRouteHint(hint types.TurnRouteHint) {
	r.hint = hint
	r.hintCalls++
}

func (r *singleShotRouteSinkRecorder) SetPresentationDirective(directive string) {
	r.directive = directive
	r.directiveCalls++
}

func (r *singleShotRouteSinkRecorder) SetPresentationDiagramRequired(required bool) {
	r.diagramRequired = required
	r.diagramReqCalls++
}

func (c *replOnlyPolicyClassifier) ClassifyPolicy(context.Context, string, string, bool) (repl.TurnPolicy, error) {
	c.called = true
	return repl.TurnPolicy{}, nil
}

// TestClassifySingleShotPolicyCall_PrefersSingleShotLane structurally pins
// the lane split at the cmd dispatch: when the wired classifier supports the
// single-shot lane it MUST be entered there (own 120s-default deadline, no
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

func TestApplySingleShotRoutePolicyCarriesTypedPresentationAuthority(t *testing.T) {
	sink := &singleShotRouteSinkRecorder{}
	policy := repl.TurnPolicy{
		Route:                 repl.RouteRepo,
		NeedsRepoAccess:       true,
		PresentationDirective: "Mermaid type relation view",
		RequiresDiagram:       true,
	}
	applySingleShotRoutePolicy(sink, policy, true)
	if sink.hintCalls != 1 || sink.directiveCalls != 1 || sink.diagramReqCalls != 1 {
		t.Fatalf("typed policy carrier calls = hint:%d directive:%d diagram:%d", sink.hintCalls, sink.directiveCalls, sink.diagramReqCalls)
	}
	if sink.directive != policy.PresentationDirective || !sink.diagramRequired {
		t.Fatalf("presentation authority lost: %+v", sink)
	}

	untouched := &singleShotRouteSinkRecorder{}
	applySingleShotRoutePolicy(untouched, policy, false)
	if untouched.hintCalls+untouched.directiveCalls+untouched.diagramReqCalls != 0 {
		t.Fatalf("unclassified policy must not mint presentation authority: %+v", untouched)
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
