package loopkernel

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/safety"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDeriveLocalizationAuthorityOwnerSupported(t *testing.T) {
	got := DeriveLocalizationAuthority(&types.SourceLocalizationReview{
		Status:              types.SourceLocalizationSupported,
		SourcePaths:         []string{"pkg/owner.py"},
		OwnerSupportedPaths: []string{"pkg/owner.py"},
		Anchors: []types.SourceLocalizationAnchor{{
			Path:     "pkg/owner.py",
			Strength: types.SourceLocalizationAnchorOwner,
		}},
	})
	if got.State != LocalizationAuthorityOwnerSupported {
		t.Fatalf("state = %s, want owner_supported: %+v", got.State, got)
	}
	if got.RequiresMoreContext {
		t.Fatalf("owner-supported localization should not require more context: %+v", got)
	}
}

func TestDeriveLocalizationAuthorityObservedOnlyRequiresLocalizer(t *testing.T) {
	got := DeriveLocalizationAuthority(&types.SourceLocalizationReview{
		Status:      types.SourceLocalizationObserved,
		SourcePaths: []string{"pkg/maybe.py"},
		Anchors: []types.SourceLocalizationAnchor{{
			Path:     "pkg/maybe.py",
			Strength: types.SourceLocalizationAnchorObserved,
		}},
	})
	if got.State != LocalizationAuthorityObservedOnly {
		t.Fatalf("state = %s, want observed_only: %+v", got.State, got)
	}
	if got.RecommendedAction != LoopActionLocalize || !got.RequiresMoreContext {
		t.Fatalf("observed-only localization should request localizer: %+v", got)
	}
}

func TestDeriveLocalizationAuthorityAuxiliaryOnly(t *testing.T) {
	got := DeriveLocalizationAuthority(&types.SourceLocalizationReview{
		Status:         types.SourceLocalizationWeak,
		AuxiliaryPaths: []string{"tests/test_owner.py"},
	})
	if got.State != LocalizationAuthorityAuxiliaryOnly {
		t.Fatalf("state = %s, want auxiliary_only: %+v", got.State, got)
	}
}

func TestDeriveProofCoverageAuthorityFromLedger(t *testing.T) {
	got := DeriveProofCoverageAuthority(nil, &types.VerificationProofLedger{
		State:         types.VerificationProofLedgerLowConfidence,
		ProfileStatus: types.VerificationProofWeak,
		ReasonCodes:   []string{"verification_probe_missing_changed_symbol_ref"},
	})
	if got.State != ProofCoverageWeak {
		t.Fatalf("state = %s, want weak: %+v", got.State, got)
	}
	if got.RecommendedAction != LoopActionAddProof || !got.RequiresProof {
		t.Fatalf("weak proof should recommend proof action: %+v", got)
	}
}

func TestDeriveProofCoverageAuthorityUnavailableIsNotRepair(t *testing.T) {
	got := DeriveProofCoverageAuthority(&types.VerificationProofProfile{
		Status:         types.VerificationProofUnavailable,
		RunnerEvidence: types.VerificationProofRunnerUnavailable,
		ReasonCodes:    []string{"parser_error"},
	}, nil)
	if got.State != ProofCoverageUnavailable {
		t.Fatalf("state = %s, want unavailable: %+v", got.State, got)
	}
	if got.RecommendedAction == LoopActionRepair || !got.AllowsUnverified {
		t.Fatalf("unavailable proof should not force repair: %+v", got)
	}
}

func TestDerivePermissionAuthorityDenyWins(t *testing.T) {
	got := DerivePermissionAuthority("test",
		safety.AllowPermission("allow", "safe", "safe"),
		safety.AskPermission("ask", "needs_review", "needs review"),
		safety.DenyPermission("deny", "critical", "critical"),
	)
	if got.State != PermissionAuthorityDeny {
		t.Fatalf("state = %s, want deny: %+v", got.State, got)
	}
	if got.RecommendedAction != LoopActionBlock || got.RequiresUser {
		t.Fatalf("deny should block without user ask: %+v", got)
	}
}
