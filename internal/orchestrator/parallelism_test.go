package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEffectiveParallelism_ZeroConfiguredIsUnlimited pins the
// three-state semantics: 0 means "unlimited; every candidate runs
// concurrently".
func TestEffectiveParallelism_ZeroConfiguredIsUnlimited(t *testing.T) {
	cases := []struct {
		candidates int
		want       int
	}{
		{1, 1},
		{2, 2},
		{5, 5},
		{100, 100},
	}
	for _, c := range cases {
		if got := effectiveParallelism(0, c.candidates); got != c.want {
			t.Errorf("effectiveParallelism(0, %d) = %d, want %d (unlimited)", c.candidates, got, c.want)
		}
	}
}

// TestEffectiveParallelism_OneConfiguredIsSerial pins the
// debug/rate-limit knob: 1 means "strict serial regardless of
// candidate count".
func TestEffectiveParallelism_OneConfiguredIsSerial(t *testing.T) {
	cases := []int{1, 2, 5, 100}
	for _, candidates := range cases {
		if got := effectiveParallelism(1, candidates); got != 1 {
			t.Errorf("effectiveParallelism(1, %d) = %d, want 1 (strict serial)", candidates, got)
		}
	}
}

// TestEffectiveParallelism_HigherConfiguredCapsToMin pins the
// production case: configured > 1 caps at min(configured, candidates).
func TestEffectiveParallelism_HigherConfiguredCapsToMin(t *testing.T) {
	cases := []struct {
		configured int
		candidates int
		want       int
	}{
		{2, 1, 1},   // cap > candidates → all of them
		{2, 2, 2},   // cap == candidates → all of them
		{2, 5, 2},   // cap < candidates → cap
		{4, 3, 3},   // cap > candidates
		{4, 10, 4},  // cap < candidates
		{16, 5, 5},  // ceiling-level cap, few candidates
	}
	for _, c := range cases {
		if got := effectiveParallelism(c.configured, c.candidates); got != c.want {
			t.Errorf("effectiveParallelism(%d, %d) = %d, want %d",
				c.configured, c.candidates, got, c.want)
		}
	}
}

// TestEffectiveParallelism_NegativeFallsBackToDefault pins the
// "configured field unset (negative)" fallback path: should behave
// as if DefaultPipelineMaxParallelism (= 2) was set.
func TestEffectiveParallelism_NegativeFallsBackToDefault(t *testing.T) {
	want := effectiveParallelism(types.DefaultPipelineMaxParallelism, 5)
	if got := effectiveParallelism(-1, 5); got != want {
		t.Errorf("negative configured should fall back to default; got %d, want %d", got, want)
	}
}

// TestEffectiveParallelism_ZeroCandidatesReturnsZero pins
// short-circuit: no work means no goroutine count.
func TestEffectiveParallelism_ZeroCandidatesReturnsZero(t *testing.T) {
	for _, configured := range []int{0, 1, 2, 8, -1} {
		if got := effectiveParallelism(configured, 0); got != 0 {
			t.Errorf("effectiveParallelism(%d, 0) = %d, want 0", configured, got)
		}
	}
}

// TestEffectiveParallelism_NegativeCandidatesReturnsZero pins
// defensive nil-safety.
func TestEffectiveParallelism_NegativeCandidatesReturnsZero(t *testing.T) {
	if got := effectiveParallelism(2, -1); got != 0 {
		t.Errorf("negative candidate count must return 0; got %d", got)
	}
}

// TestOrchestratorMaxParallelism_NilSafe pins nil-receiver safety.
func TestOrchestratorMaxParallelism_NilSafe(t *testing.T) {
	var o *Orchestrator
	if got := o.orchestratorMaxParallelism(); got != types.DefaultPipelineMaxParallelism {
		t.Errorf("nil orchestrator must return default; got %d", got)
	}
}

// TestOrchestratorMaxParallelism_ClampsToCeil pins the misconfigured
// "huge value" path: orchestrator must hard-cap to
// MaxPipelineParallelismCeil so a runaway yaml value cannot spawn
// thousands of goroutines.
func TestOrchestratorMaxParallelism_ClampsToCeil(t *testing.T) {
	o := &Orchestrator{settings: types.PipelineSettings{MaxParallelism: 100}}
	if got := o.orchestratorMaxParallelism(); got != types.MaxPipelineParallelismCeil {
		t.Errorf("100 must clamp to ceil %d; got %d", types.MaxPipelineParallelismCeil, got)
	}
}

// TestOrchestratorMaxParallelism_NegativeFallsBackToDefault pins the
// "yaml field absent" path: PipelineSettings.MaxParallelism is int
// (zero-initialised); negative is invalid; the helper must fall back.
func TestOrchestratorMaxParallelism_NegativeFallsBackToDefault(t *testing.T) {
	o := &Orchestrator{settings: types.PipelineSettings{MaxParallelism: -1}}
	if got := o.orchestratorMaxParallelism(); got != types.DefaultPipelineMaxParallelism {
		t.Errorf("negative must fall back to default; got %d", got)
	}
}

// TestOrchestratorMaxParallelism_ZeroPreserved pins the
// "unlimited" knob: configured 0 means unlimited and the helper
// must return 0 (NOT fall back to default; that would change semantics).
func TestOrchestratorMaxParallelism_ZeroPreserved(t *testing.T) {
	o := &Orchestrator{settings: types.PipelineSettings{MaxParallelism: 0}}
	if got := o.orchestratorMaxParallelism(); got != 0 {
		t.Errorf("0 (unlimited) must be preserved; got %d", got)
	}
}

// TestOrchestratorMaxParallelism_OnePreserved pins the "serial" knob:
// configured 1 means strict serial and must NOT be clamped/default-replaced.
func TestOrchestratorMaxParallelism_OnePreserved(t *testing.T) {
	o := &Orchestrator{settings: types.PipelineSettings{MaxParallelism: 1}}
	if got := o.orchestratorMaxParallelism(); got != 1 {
		t.Errorf("1 (serial) must be preserved; got %d", got)
	}
}
