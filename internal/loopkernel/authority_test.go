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

func TestDeriveProofCoverageAuthorityFromAttemptClassifiesTypedVerifierStates(t *testing.T) {
	cases := []struct {
		name       string
		attempt    *types.WriteWorkflowAttempt
		wantState  ProofCoverageAuthorityState
		wantAction LoopRecommendedAction
	}{
		{
			name:       "passed",
			attempt:    &types.WriteWorkflowAttempt{Kind: "verify", Status: "passed", ReasonCode: "tests_passed"},
			wantState:  ProofCoverageCovered,
			wantAction: LoopActionContinue,
		},
		{
			name:       "unavailable",
			attempt:    &types.WriteWorkflowAttempt{Kind: "verify", Status: "unverified", ReasonCode: "runner_missing"},
			wantState:  ProofCoverageUnavailable,
			wantAction: LoopActionContinue,
		},
		{
			name:       "unverified code failure",
			attempt:    &types.WriteWorkflowAttempt{Kind: "verify", Status: "unverified", ReasonCode: "tests_failed"},
			wantState:  ProofCoverageFailed,
			wantAction: LoopActionRepair,
		},
		{
			name:       "failed unavailable",
			attempt:    &types.WriteWorkflowAttempt{Kind: "verify", Status: "failed", ReasonCode: "parser_error"},
			wantState:  ProofCoverageUnavailable,
			wantAction: LoopActionContinue,
		},
		{
			name:       "missing",
			attempt:    nil,
			wantState:  ProofCoverageMissing,
			wantAction: LoopActionVerify,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveProofCoverageAuthorityFromAttempt(tc.attempt)
			if got.State != tc.wantState || got.RecommendedAction != tc.wantAction {
				t.Fatalf("proof authority = %+v, want state=%s action=%s", got, tc.wantState, tc.wantAction)
			}
		})
	}
}

func TestDeriveProofCoverageAuthorityFromArtifactsDowngradesPassedWeakLedger(t *testing.T) {
	got := DeriveProofCoverageAuthorityFromArtifacts(
		&types.WriteWorkflowAttempt{Kind: "verify", Status: "passed", ReasonCode: "tests_passed"},
		nil,
		&types.VerificationProofLedger{
			State:          types.VerificationProofLedgerLowConfidence,
			ProfileStatus:  types.VerificationProofWeak,
			RunnerEvidence: types.VerificationProofRunnerProject,
			ReasonCodes:    []string{"impact_targets_unverified"},
		},
	)
	if got.State != ProofCoverageWeak || got.RecommendedAction != LoopActionAddProof || !got.RequiresProof {
		t.Fatalf("passed attempt with weak ledger should require proof follow-up: %+v", got)
	}
	if got.ReasonCode != "proof_weak" {
		t.Fatalf("reason = %q, want proof_weak", got.ReasonCode)
	}
}

func TestDeriveProofCoverageAuthorityFromArtifactsUnavailableWinsOverWeakLedger(t *testing.T) {
	got := DeriveProofCoverageAuthorityFromArtifacts(
		&types.WriteWorkflowAttempt{Kind: "verify", Status: "unverified", ReasonCode: "runner_missing"},
		nil,
		&types.VerificationProofLedger{
			State:         types.VerificationProofLedgerLowConfidence,
			ProfileStatus: types.VerificationProofWeak,
			ReasonCodes:   []string{"impact_targets_unverified"},
		},
	)
	if got.State != ProofCoverageUnavailable || got.RecommendedAction == LoopActionRepair || !got.AllowsUnverified {
		t.Fatalf("unavailable runner should remain unverified, not repair: %+v", got)
	}
}

func TestDeriveProofCoverageAuthorityFromArtifactsFailedLedgerOverridesPassedAttempt(t *testing.T) {
	got := DeriveProofCoverageAuthorityFromArtifacts(
		&types.WriteWorkflowAttempt{Kind: "verify", Status: "passed", ReasonCode: "tests_passed"},
		nil,
		&types.VerificationProofLedger{
			State:         types.VerificationProofLedgerFailed,
			ProfileStatus: types.VerificationProofFailed,
			ReasonCodes:   []string{"patch_review_hard_block"},
		},
	)
	if got.State != ProofCoverageFailed || got.RecommendedAction != LoopActionRepair {
		t.Fatalf("failed ledger should override passed attempt: %+v", got)
	}
	if got.ReasonCode != "proof_failed" {
		t.Fatalf("reason = %q, want proof_failed", got.ReasonCode)
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

func TestDerivePermissionAuthorityFromEffects(t *testing.T) {
	got := DerivePermissionAuthorityFromEffects("test",
		safety.EffectDescriptor{Role: safety.EffectRolePlanner, Kind: safety.EffectKindRepoMap},
		safety.EffectDescriptor{Role: safety.EffectRolePlanner, Kind: safety.EffectKindCommand},
	)
	if got.State != PermissionAuthorityDeny || got.RecommendedAction != LoopActionBlock {
		t.Fatalf("planner command effect should deny through loop authority: %+v", got)
	}
	if got.ReasonCode != "role_effect_denied" {
		t.Fatalf("reason = %q, want role_effect_denied", got.ReasonCode)
	}
}

func TestDerivePermissionAuthorityFromEffectsExternalReadAsks(t *testing.T) {
	got := DerivePermissionAuthorityFromEffects("test",
		safety.EffectDescriptor{Role: safety.EffectRoleLocalizer, Kind: safety.EffectKindReadRepo, ExternalDirectory: true},
	)
	if got.State != PermissionAuthorityAsk || got.RecommendedAction != LoopActionAskApproval || !got.RequiresUser {
		t.Fatalf("external read effect should ask through loop authority: %+v", got)
	}
	if got.ReasonCode != "external_directory_effect" {
		t.Fatalf("reason = %q, want external_directory_effect", got.ReasonCode)
	}
}
