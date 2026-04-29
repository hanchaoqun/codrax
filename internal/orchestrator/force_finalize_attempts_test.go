package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestSetForceFinalizeAttempts_DefaultZeroFallsBack ensures a
// pre-resolution Run (no yaml override + no explicit setter call)
// gets the type-default attempt count. Defends against a regression
// that sets forceFinalizeAttempts=0 inline and accidentally
// disables the retry loop entirely.
func TestSetForceFinalizeAttempts_DefaultZeroFallsBack(t *testing.T) {
	o := &Orchestrator{}
	o.SetForceFinalizeAttempts(0)
	if o.forceFinalizeAttempts != types.DefaultForceFinalizeAttempts {
		t.Errorf("zero must fall back to %d; got %d",
			types.DefaultForceFinalizeAttempts, o.forceFinalizeAttempts)
	}
	o.SetForceFinalizeAttempts(-5)
	if o.forceFinalizeAttempts != types.DefaultForceFinalizeAttempts {
		t.Errorf("negative must fall back to %d; got %d",
			types.DefaultForceFinalizeAttempts, o.forceFinalizeAttempts)
	}
}

// TestSetForceFinalizeAttempts_RespectsCeiling locks the absolute
// hard cap. Operator typos like setting the yaml knob to 50 must
// not let force-finalize burn 50× the LLM budget on a genuinely-
// down upstream.
func TestSetForceFinalizeAttempts_RespectsCeiling(t *testing.T) {
	o := &Orchestrator{}
	o.SetForceFinalizeAttempts(50)
	if o.forceFinalizeAttempts != types.MaxForceFinalizeAttempts {
		t.Errorf("over-cap value must clamp to %d; got %d",
			types.MaxForceFinalizeAttempts, o.forceFinalizeAttempts)
	}
}

// TestSetForceFinalizeAttempts_AcceptsValidRange verifies the
// usable range [1, MaxForceFinalizeAttempts] passes through.
func TestSetForceFinalizeAttempts_AcceptsValidRange(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 5} {
		o := &Orchestrator{}
		o.SetForceFinalizeAttempts(n)
		if o.forceFinalizeAttempts != n {
			t.Errorf("value %d should pass through; got %d", n, o.forceFinalizeAttempts)
		}
	}
}
