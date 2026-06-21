package loopkernel

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLoopkernelShadowMatchesImperative(t *testing.T) {
	guidance := ReadProofGuidance{
		Active:            true,
		State:             ProofCoverageWeak,
		ReasonCode:        "proof_weak",
		RecommendedAction: LoopActionAddProof,
		TruthAction:       types.TruthActionAddProof,
		Advisory:          true,
	}
	got, ok := CompareReadLoopShadow(guidance, LoopActionAddProof)
	if !ok {
		t.Fatal("expected active shadow comparison")
	}
	if !got.Match || got.RecommendedAction != LoopActionAddProof || got.ImperativeAction != LoopActionAddProof {
		t.Fatalf("shadow comparison mismatch: %+v", got)
	}
	if got.ProofState != ProofCoverageWeak || !got.Advisory || got.HardBlock {
		t.Fatalf("proof metadata not carried: %+v", got)
	}
}

func TestReadLoopShadowReportsMismatchWithoutChangingGuidance(t *testing.T) {
	guidance := ReadProofGuidance{
		Active:            true,
		State:             ProofCoverageWeak,
		ReasonCode:        "proof_weak",
		RecommendedAction: LoopActionAddProof,
		TruthAction:       types.TruthActionAddProof,
		Advisory:          true,
	}
	got, ok := CompareReadLoopShadow(guidance, LoopActionContinue)
	if !ok {
		t.Fatal("expected active shadow comparison")
	}
	if got.Match {
		t.Fatalf("continue should not shadow-match add_proof: %+v", got)
	}
	if guidance.RecommendedAction != LoopActionAddProof {
		t.Fatalf("comparison must not mutate guidance: %+v", guidance)
	}
}

func TestReadLoopShadowIgnoresMissingInputs(t *testing.T) {
	if got, ok := CompareReadLoopShadow(ReadProofGuidance{}, LoopActionContinue); ok || got.Active {
		t.Fatalf("empty guidance should not produce comparison: %+v", got)
	}
	if got, ok := CompareReadLoopShadow(ReadProofGuidance{Active: true, RecommendedAction: LoopActionContinue}, "bogus"); ok || got.Active {
		t.Fatalf("unknown imperative action should not produce comparison: %+v", got)
	}
}
