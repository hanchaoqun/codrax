package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPlannerBuildInitialInstruction_ActivePitfallsSection
// pins commit 37: when the orchestrator populated
// ctx.ActivePitfalls (stage 3 Failure Taxonomy match),
// BuildInitialInstruction renders the pitfalls under a
// "## Known active pitfalls in this repo" heading + each
// pitfall as a name + description bullet + trigger sub-bullet.
// Empty slice → no section emitted (back-compat).
func TestPlannerBuildInitialInstruction_ActivePitfallsSection(t *testing.T) {
	mu := types.NewMutableState("test pitfalls")
	e := &plannerEvaluator{}
	ctx := &types.AgentContext{
		Mutable: mu,
		ActivePitfalls: []types.FailurePattern{
			{
				ID:          "fp-1",
				Name:        "lock-scope drift on new public method",
				Description: "When an exported method is added to a goroutine-shared struct, the existing mutex coverage tends to leave it unguarded.",
				Trigger:     "adding a method to a goroutine-shared struct",
				Consequence: "data race detected by go test -race",
				Confidence:  0.8,
			},
			{
				ID:          "fp-2",
				Name:        "stale-fixture-on-interface-widen",
				Description: "Test fixtures that stub an interface lag when the interface gains a new method; the new method's stub returns zero values that pass obvious assertions but break code paths exercising the new method.",
				Trigger:     "widening an interface that downstream tests stub",
				Confidence:  0.7,
			},
		},
	}
	got := e.BuildInitialInstruction(ctx, nil)
	if !strings.Contains(got, "## Known active pitfalls in this repo") {
		t.Errorf("expected pitfalls section; got:\n%s", got)
	}
	if !strings.Contains(got, "lock-scope drift on new public method") {
		t.Errorf("first pitfall name missing; got:\n%s", got)
	}
	if !strings.Contains(got, "stale-fixture-on-interface-widen") {
		t.Errorf("second pitfall name missing; got:\n%s", got)
	}
	if !strings.Contains(got, "Trigger: adding a method to a goroutine-shared struct") {
		t.Errorf("first trigger missing; got:\n%s", got)
	}
	if !strings.Contains(got, "Typical consequence: data race") {
		t.Errorf("first consequence missing; got:\n%s", got)
	}
	// Framing must be descriptive, not prescriptive.
	for _, banned := range []string{"you must", "DO NOT", "always avoid"} {
		if strings.Contains(got, banned) {
			t.Errorf("pitfalls section must not be prescriptive; banned %q in:\n%s", banned, got)
		}
	}
}

// TestPlannerBuildInitialInstruction_NoPitfallsNoSection pins
// the back-compat path: zero pitfalls → no heading rendered.
func TestPlannerBuildInitialInstruction_NoPitfallsNoSection(t *testing.T) {
	mu := types.NewMutableState("test")
	e := &plannerEvaluator{}
	ctx := &types.AgentContext{Mutable: mu, ActivePitfalls: nil}
	got := e.BuildInitialInstruction(ctx, nil)
	if strings.Contains(got, "## Known active pitfalls") {
		t.Errorf("empty pitfalls should not render section; got:\n%s", got)
	}
}
