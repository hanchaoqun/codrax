package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// cgec_storm_test.go — R10 lock tests for the CGEC frequency-to-
// violation bridge. Pins the threshold semantics + storm
// classification + ledger append behaviour.

// TestSetCGECStormThresholds_Defaults pins the default values +
// reset-on-negative behaviour.
func TestSetCGECStormThresholds_Defaults(t *testing.T) {
	t.Cleanup(func() { SetCGECStormThresholds(-1, -1) }) // reset

	// Negative values reset to defaults.
	SetCGECStormThresholds(-1, -1)
	if demotionStormThreshold != 10 {
		t.Errorf("demotion default = %d, want 10", demotionStormThreshold)
	}
	if forcedReadStormThreshold != 8 {
		t.Errorf("forced_read default = %d, want 8", forcedReadStormThreshold)
	}

	// Positive overrides honoured.
	SetCGECStormThresholds(20, 5)
	if demotionStormThreshold != 20 {
		t.Errorf("demotion override = %d, want 20", demotionStormThreshold)
	}
	if forcedReadStormThreshold != 5 {
		t.Errorf("forced_read override = %d, want 5", forcedReadStormThreshold)
	}

	// Zero = explicit disable.
	SetCGECStormThresholds(0, 0)
	if demotionStormThreshold != 0 || forcedReadStormThreshold != 0 {
		t.Errorf("zero must disable; got %d / %d", demotionStormThreshold, forcedReadStormThreshold)
	}
}

// TestEmitCGECStormViolations_BelowThresholdNoEmit pins the
// no-fire path: counter under threshold → no ledger entry.
func TestEmitCGECStormViolations_BelowThresholdNoEmit(t *testing.T) {
	t.Cleanup(func() { SetCGECStormThresholds(-1, -1) })
	SetCGECStormThresholds(10, 8)

	closure := &types.EvidenceClosure{}
	emitCGECStormViolations(closure, types.ClosureStats{
		ChainsDemoted: 5, ForcedReads: 3,
	})
	if got := len(closure.Violations()); got != 0 {
		t.Errorf("below-threshold must not emit; got %d violations", got)
	}
}

// TestEmitCGECStormViolations_DemotionStormFires pins the
// chains_demoted >= threshold path.
func TestEmitCGECStormViolations_DemotionStormFires(t *testing.T) {
	t.Cleanup(func() { SetCGECStormThresholds(-1, -1) })
	SetCGECStormThresholds(10, 8)

	closure := &types.EvidenceClosure{}
	emitCGECStormViolations(closure, types.ClosureStats{
		ChainsDemoted: 18, // m1a r1 真值
		ForcedReads:   3,
	})
	vs := closure.Violations()
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %d (%+v)", len(vs), vs)
	}
	if vs[0].Kind != types.ViolDemotionStorm {
		t.Errorf("kind = %q, want demotion_storm", vs[0].Kind)
	}
	// Detail names the storm count + threshold for operator visibility.
	for _, want := range []string{"chains_demoted=18", "threshold 10"} {
		if !strings.Contains(vs[0].Detail, want) {
			t.Errorf("Detail missing %q: %q", want, vs[0].Detail)
		}
	}
}

// TestEmitCGECStormViolations_ForcedReadStormFires pins the
// forced_reads >= threshold path.
func TestEmitCGECStormViolations_ForcedReadStormFires(t *testing.T) {
	t.Cleanup(func() { SetCGECStormThresholds(-1, -1) })
	SetCGECStormThresholds(10, 8)

	closure := &types.EvidenceClosure{}
	emitCGECStormViolations(closure, types.ClosureStats{
		ChainsDemoted: 0,
		ForcedReads:   12,
	})
	vs := closure.Violations()
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %d", len(vs))
	}
	if vs[0].Kind != types.ViolForcedReadStorm {
		t.Errorf("kind = %q, want forced_read_storm", vs[0].Kind)
	}
}

// TestEmitCGECStormViolations_BothFire confirms both bridges
// can co-fire on the same Run.
func TestEmitCGECStormViolations_BothFire(t *testing.T) {
	t.Cleanup(func() { SetCGECStormThresholds(-1, -1) })
	SetCGECStormThresholds(10, 8)

	closure := &types.EvidenceClosure{}
	emitCGECStormViolations(closure, types.ClosureStats{
		ChainsDemoted: 18, ForcedReads: 12,
	})
	if got := len(closure.Violations()); got != 2 {
		t.Errorf("both bridges must fire; got %d violations", got)
	}
}

// TestEmitCGECStormViolations_DisabledThresholdNoEmit pins the
// 0-threshold disable behaviour.
func TestEmitCGECStormViolations_DisabledThresholdNoEmit(t *testing.T) {
	t.Cleanup(func() { SetCGECStormThresholds(-1, -1) })
	SetCGECStormThresholds(0, 0) // disable both

	closure := &types.EvidenceClosure{}
	emitCGECStormViolations(closure, types.ClosureStats{
		ChainsDemoted: 100, ForcedReads: 100,
	})
	if got := len(closure.Violations()); got != 0 {
		t.Errorf("disabled bridges must not fire; got %d", got)
	}
}

// TestEmitCGECStormViolations_NilClosureNoOp covers nil safety.
func TestEmitCGECStormViolations_NilClosureNoOp(t *testing.T) {
	emitCGECStormViolations(nil, types.ClosureStats{ChainsDemoted: 100})
	// Just shouldn't panic.
}

// TestStormViolationProfile_IsSoft confirms storm violations are
// classified Soft via DeriveSeverity (R15 ViolationProfile).
func TestStormViolationProfile_IsSoft(t *testing.T) {
	for _, kind := range []types.ViolationKind{
		types.ViolDemotionStorm, types.ViolForcedReadStorm,
	} {
		// Both strict and non-strict must classify as Soft —
		// reviewer-permanent semantic per types.DeriveSeverity.
		for _, strict := range []bool{true, false} {
			p := types.ViolationProfileFor(kind, strict)
			if p.Severity != types.SeveritySoft {
				t.Errorf("%q strict=%v: severity = %q, want soft", kind, strict, p.Severity)
			}
			if p.RetryEligible {
				t.Errorf("%q strict=%v: must not be retry-eligible", kind, strict)
			}
		}
	}
}

