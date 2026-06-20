package loopkernel

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestProofSnapshotFromReadTurnAOwnerSupportedIsCovered(t *testing.T) {
	snapshot, ok := ProofSnapshotFromReadTurnA(&types.TurnAArtifacts{
		ReadFiles:          []string{"pkg/owner.py"},
		AcceptedResultKind: "resolved",
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-owner",
			Source:          "pkg/owner.py",
			LineStart:       12,
			Subject:         "Owner",
			GroundingStatus: types.GroundingGrounded,
		}},
		SourceLocalization: &types.SourceLocalizationReview{
			Status:              types.SourceLocalizationSupported,
			SourcePaths:         []string{"pkg/owner.py"},
			OwnerSupportedPaths: []string{"pkg/owner.py"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:     "pkg/owner.py",
				Strength: types.SourceLocalizationAnchorOwner,
			}},
		},
	})
	if !ok {
		t.Fatal("expected read proof snapshot")
	}
	if snapshot.Authority.State != ProofCoverageCovered || snapshot.Truth.State != types.TruthLedgerCovered {
		t.Fatalf("snapshot should be covered: %+v", snapshot)
	}
	if snapshot.Profile.Status != types.VerificationProofStrong || snapshot.Ledger.State != types.VerificationProofLedgerVerified {
		t.Fatalf("profile/ledger = %+v / %+v", snapshot.Profile, snapshot.Ledger)
	}
}

func TestProofSnapshotFromReadTurnAObservedOnlyIsWeak(t *testing.T) {
	snapshot, ok := ProofSnapshotFromReadTurnA(&types.TurnAArtifacts{
		ReadFiles:          []string{"pkg/maybe.py"},
		AcceptedResultKind: "resolved",
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-observed",
			Source:          "pkg/maybe.py",
			LineStart:       3,
			Subject:         "Maybe",
			GroundingStatus: types.GroundingGrounded,
		}},
		SourceLocalization: &types.SourceLocalizationReview{
			Status:      types.SourceLocalizationObserved,
			SourcePaths: []string{"pkg/maybe.py"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:     "pkg/maybe.py",
				Strength: types.SourceLocalizationAnchorObserved,
			}},
		},
	})
	if !ok {
		t.Fatal("expected read proof snapshot")
	}
	if snapshot.Authority.State != ProofCoverageWeak || snapshot.Authority.RecommendedAction != LoopActionAddProof {
		t.Fatalf("observed-only read proof should request proof follow-up: %+v", snapshot.Authority)
	}
	if snapshot.Truth.State != types.TruthLedgerWeak || !snapshot.Truth.Weak {
		t.Fatalf("truth = %+v, want weak", snapshot.Truth)
	}
}

func TestReadProofGuidanceFromTurnAWeakIsAdvisory(t *testing.T) {
	guidance, ok := ReadProofGuidanceFromTurnA(&types.TurnAArtifacts{
		ReadFiles:          []string{"pkg/maybe.py"},
		AcceptedResultKind: "resolved",
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-observed",
			Source:          "pkg/maybe.py",
			LineStart:       3,
			Subject:         "Maybe",
			GroundingStatus: types.GroundingGrounded,
		}},
		SourceLocalization: &types.SourceLocalizationReview{
			Status:      types.SourceLocalizationObserved,
			SourcePaths: []string{"pkg/maybe.py"},
		},
	})
	if !ok || !guidance.Active {
		t.Fatalf("expected active guidance, got %+v ok=%t", guidance, ok)
	}
	if guidance.State != ProofCoverageWeak || !guidance.Advisory || guidance.HardBlock {
		t.Fatalf("weak read proof should be advisory only: %+v", guidance)
	}
	if guidance.TruthState != types.TruthLedgerWeak || guidance.RecommendedAction != LoopActionAddProof {
		t.Fatalf("truth/action = %+v, want weak/add_proof", guidance)
	}
}

func TestProofSnapshotFromReadTurnARuntimeObservationOnlyIsCovered(t *testing.T) {
	snapshot, ok := ProofSnapshotFromReadTurnA(&types.TurnAArtifacts{
		AcceptedResultKind:               "resolved",
		RuntimeObservationOnlyCompletion: true,
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:  types.AnswerAggregateScalar,
			Label: "root_cause",
			Value: "binder timeout",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
		}},
	})
	if !ok {
		t.Fatal("expected read proof snapshot")
	}
	if snapshot.Authority.State != ProofCoverageCovered || snapshot.Profile.Status != types.VerificationProofAdequate {
		t.Fatalf("runtime-only completion should be adequate covered proof: %+v", snapshot)
	}
}

func TestProofSnapshotFromReadTurnACarriesSourceClassUniverse(t *testing.T) {
	snapshot, ok := ProofSnapshotFromReadTurnA(&types.TurnAArtifacts{
		AcceptedResultKind: "absence",
		SourceInventoryObservation: types.SourceInventoryObservation{
			Active:   true,
			Complete: true,
			Lens:     []string{"source_class_universe", "count"},
			SourceClasses: []types.SourceInventorySourceClassCount{{
				Role:     types.SourcePathRoleThirdParty,
				Count:    6,
				Complete: true,
			}},
			Sets: []types.SourceInventoryObservationSet{{
				Role:     types.AnswerCandidateRoleFunction,
				Complete: true,
			}},
		},
	})
	if !ok {
		t.Fatal("expected read proof snapshot")
	}
	if !snapshot.SourceClassUniverseComplete || len(snapshot.SourceClasses) != 1 ||
		snapshot.SourceClasses[0].Role != types.SourcePathRoleThirdParty {
		t.Fatalf("source-class universe not projected: %+v", snapshot)
	}
	if snapshot.Authority.State != ProofCoverageCovered {
		t.Fatalf("complete source-inventory universe should be covered proof: %+v", snapshot.Authority)
	}
	foundRef := false
	for _, ref := range snapshot.EvidenceRefs {
		if ref == "source_class:thirdparty" {
			foundRef = true
			break
		}
	}
	if !foundRef {
		t.Fatalf("source-class ref missing from proof evidence refs: %+v", snapshot.EvidenceRefs)
	}
}

func TestProofSnapshotFromReadMutableUsesClosureUniverse(t *testing.T) {
	mut := types.NewMutableState("source inventory")
	mut.EvidenceClosure().RecordSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Lens:     []string{"source_class_universe", "count"},
		SourceClasses: []types.SourceInventorySourceClassCount{{
			Role:     types.SourcePathRoleThirdParty,
			Count:    6,
			Complete: false,
		}},
	})

	snapshot, ok := ProofSnapshotFromReadMutable(mut)
	if !ok {
		t.Fatal("expected mutable proof snapshot")
	}
	if snapshot.SourceClassUniverseComplete {
		t.Fatalf("incomplete closure universe should stay incomplete: %+v", snapshot.SourceClasses)
	}
	if snapshot.Authority.State != ProofCoverageWeak ||
		snapshot.Authority.RecommendedAction != LoopActionAddProof {
		t.Fatalf("incomplete source-class universe should request proof follow-up: %+v", snapshot.Authority)
	}
}

func TestEventsFromReadProofSnapshotReduceToLoopProof(t *testing.T) {
	snapshot, ok := ProofSnapshotFromReadTurnA(&types.TurnAArtifacts{
		ReadFiles:          []string{"pkg/owner.py"},
		AcceptedResultKind: "resolved",
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-owner",
			Source:          "pkg/owner.py",
			LineStart:       12,
			Subject:         "Owner",
			GroundingStatus: types.GroundingGrounded,
		}},
		SourceLocalization: &types.SourceLocalizationReview{
			Status:              types.SourceLocalizationSupported,
			SourcePaths:         []string{"pkg/owner.py"},
			OwnerSupportedPaths: []string{"pkg/owner.py"},
		},
	})
	if !ok {
		t.Fatal("expected read proof snapshot")
	}
	view := ReduceEvents(EventsFromReadProofSnapshot("read-1", snapshot))
	if view.Proof.State != ProofCoverageCovered || view.Truth.State != types.TruthLedgerCovered {
		t.Fatalf("loop state = %+v", view)
	}
}
