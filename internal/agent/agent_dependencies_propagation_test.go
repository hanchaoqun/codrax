package agent

import (
	"errors"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestNewBaseAgent_PropagatesAllDependencyFields pins the 2026-04-28
// regression fix: NewBaseAgent must propagate ALL Dependencies fields
// from the input deps, not the historical 8-field subset. Pre-fix
// the explicit copy list dropped CancelChecker / AgentSettings /
// ExploreHeuristics / Skills, breaking Ctrl+C interruption,
// neutering yaml-tuned tool-history + context-pressure overrides at
// the BaseAgent layer, and disabling the log_triager / perf_triager
// two-step segmentation skill switching. The dereference-copy
// pattern auto-picks up future fields; this test pins the four
// fields the audit caught + the original eight so a future
// regression on either side fails loud.
func TestNewBaseAgent_PropagatesAllDependencyFields(t *testing.T) {
	cancelSentinel := errors.New("cancel-sentinel")
	skills := skill.NewRegistry()
	heuristics := types.ExploreHeuristics{SerialStreakThreshold: 7}
	settings := types.AgentSettings{MaxToolHistoryBytes: 99999}

	deps := &Dependencies{
		CancelChecker:     func() error { return cancelSentinel },
		AgentSettings:     settings,
		ExploreHeuristics: heuristics,
		Skills:            skills,
	}

	b := NewBaseAgent(types.AgentExplorer, deps, nil)
	if b == nil {
		t.Fatal("NewBaseAgent returned nil")
	}

	// CancelChecker — the canonical regression. Must be non-nil and
	// must return the sentinel error from the source closure (proves
	// it's the SAME function pointer, not zero-value).
	if b.deps.CancelChecker == nil {
		t.Fatal("CancelChecker dropped — Ctrl+C would never fire on this agent")
	}
	if err := b.deps.CancelChecker(); !errors.Is(err, cancelSentinel) {
		t.Errorf("CancelChecker not the original closure; got %v", err)
	}

	// AgentSettings — yaml-tunable tool history budget reaches the
	// BaseAgent loop.
	if b.deps.AgentSettings.MaxToolHistoryBytes != 99999 {
		t.Errorf("AgentSettings dropped: got %+v", b.deps.AgentSettings)
	}

	// ExploreHeuristics — explorer's mid-loop / soft-stop tunables.
	if b.deps.ExploreHeuristics.SerialStreakThreshold != 7 {
		t.Errorf("ExploreHeuristics dropped: got %+v", b.deps.ExploreHeuristics)
	}

	// Skills — log_triager / perf_triager two-step skill lookup.
	if b.deps.Skills == nil {
		t.Error("Skills dropped — log_triager / perf_triager segmentation fallback would silently fail")
	}
}

// TestNewBaseAgent_OverridesMaxIterations verifies the MaxIterations
// override the function explicitly applies (zero falls through to
// the 20 default; non-zero is honored). Locks the override surface
// so a future "deps.MaxIterations = N" change doesn't accidentally
// stop being applied through the deref-copy path.
func TestNewBaseAgent_OverridesMaxIterations(t *testing.T) {
	deps := &Dependencies{MaxIterations: 0}
	b := NewBaseAgent(types.AgentExplorer, deps, nil)
	if b.deps.MaxIterations != 20 {
		t.Errorf("zero MaxIterations should default to 20; got %d", b.deps.MaxIterations)
	}

	deps2 := &Dependencies{MaxIterations: 42}
	b2 := NewBaseAgent(types.AgentExplorer, deps2, nil)
	if b2.deps.MaxIterations != 42 {
		t.Errorf("explicit MaxIterations=42 not honored; got %d", b2.deps.MaxIterations)
	}
}

// TestNewBaseAgent_DefaultsZeroLoopPolicy guards the zero-LoopPolicy
// substitution so a caller passing the default-zero value gets the
// production DefaultLoopPolicy() rather than an inert zero policy.
func TestNewBaseAgent_DefaultsZeroLoopPolicy(t *testing.T) {
	deps := &Dependencies{LoopPolicy: LoopPolicy{}}
	b := NewBaseAgent(types.AgentExplorer, deps, nil)
	if b.deps.LoopPolicy == (LoopPolicy{}) {
		t.Error("zero LoopPolicy should be replaced with DefaultLoopPolicy")
	}
}
