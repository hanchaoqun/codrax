package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRepairPhaseDelegatesToRegistry pins the F2 fold: the finalize
// retry-prompt phase comes from ViolKindSpec.EffectiveRepairPhase for
// every registered kind (the historical hand-written switch is gone —
// the orchestrator registry golden snapshot pins the per-kind values),
// and unregistered kinds derive from the violation's own producer-stage
// label with the consistency default.
func TestRepairPhaseDelegatesToRegistry(t *testing.T) {
	for _, kind := range types.AllViolationKinds() {
		spec, ok := types.ViolKindSpecFor(kind)
		if !ok {
			t.Fatalf("kind %s has no registry spec", kind)
		}
		want := retryRepairPhase(spec.EffectiveRepairPhase())
		if got := retryRepairPhaseForViolation(types.ScoredViolation{Kind: kind}); got != want {
			t.Errorf("registered kind %s: phase=%s, registry says %s", kind, got, want)
		}
	}
	unregistered := types.ViolationKind("zz_unregistered_probe")
	cases := map[string]retryRepairPhase{
		"semantic_quality": retryRepairPhaseCoverage,
		"v2_oracle":        retryRepairPhaseStructure,
		"evidence_pool":    retryRepairPhaseRichness,
		"contract_check":   retryRepairPhaseConsistency,
		"":                 retryRepairPhaseConsistency,
	}
	for layer, want := range cases {
		if got := retryRepairPhaseForViolation(types.ScoredViolation{Kind: unregistered, Layer: layer}); got != want {
			t.Errorf("unregistered kind layer=%q: phase=%s, want %s", layer, got, want)
		}
	}
}
