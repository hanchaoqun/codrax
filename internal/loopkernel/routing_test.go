package loopkernel

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSemanticRoutingPreciseSignalsOnly(t *testing.T) {
	state := LoopStateView{
		Status: LoopRunStatusInProgress,
		Localization: LocalizationAuthorityView{
			State:               LocalizationAuthorityObservedOnly,
			ReasonCode:          "localization_observed_without_owner",
			RecommendedAction:   LoopActionLocalize,
			RequiresMoreContext: true,
			SourcePaths:         []string{"pkg/owner.py"},
		},
		LastReasonCode: "model prose says run tests now",
	}
	got := ToolRouteForLoopState(state)
	if got.Surface != LoopToolSurfaceLocalization || got.Action != LoopActionLocalize {
		t.Fatalf("route = %+v, want localization", got)
	}
	if !reflect.DeepEqual(got.ToolSuggestions, []string{"repo_map", "read_file", "list_files", "grep"}) {
		t.Fatalf("tool suggestions = %+v", got.ToolSuggestions)
	}

	state.LastReasonCode = "different narrative should not affect route"
	again := ToolRouteForLoopState(state)
	if !reflect.DeepEqual(got, again) {
		t.Fatalf("route changed after prose-only field changed: before=%+v after=%+v", got, again)
	}
}

func TestReadWriteShareLoopkernelSubstrate(t *testing.T) {
	profile := types.VerificationProofProfile{
		Status:         types.VerificationProofWeak,
		RunnerEvidence: types.VerificationProofRunnerProject,
	}
	ledger := types.VerificationProofLedger{
		State:          types.VerificationProofLedgerLowConfidence,
		ProfileStatus:  types.VerificationProofWeak,
		RunnerEvidence: types.VerificationProofRunnerProject,
	}
	readProof := DeriveProofCoverageAuthorityFromSnapshot(ProofSnapshot{
		Profile: profile,
		Ledger:  ledger,
	})
	writeProof := DeriveProofCoverageAuthorityFromArtifacts(&types.WriteWorkflowAttempt{
		Kind:   "verify",
		Status: "passed",
	}, &profile, &ledger)
	if readProof.State != ProofCoverageWeak || readProof.RecommendedAction != LoopActionAddProof {
		t.Fatalf("read proof = %+v, want weak add_proof", readProof)
	}
	if writeProof.State != readProof.State || writeProof.RecommendedAction != readProof.RecommendedAction {
		t.Fatalf("read/write proof mismatch: read=%+v write=%+v", readProof, writeProof)
	}
}
