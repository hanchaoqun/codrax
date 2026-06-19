package loopkernel

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/safety"
	"github.com/hanchaoqun/codrax/internal/types"
)

type LocalizationAuthorityState string

const (
	LocalizationAuthorityUnknown        LocalizationAuthorityState = "unknown"
	LocalizationAuthorityOwnerSupported LocalizationAuthorityState = "owner_supported"
	LocalizationAuthorityObservedOnly   LocalizationAuthorityState = "observed_only"
	LocalizationAuthorityAuxiliaryOnly  LocalizationAuthorityState = "auxiliary_only"
	LocalizationAuthorityMissing        LocalizationAuthorityState = "missing"
	LocalizationAuthorityConflicted     LocalizationAuthorityState = "conflicted"
)

type ProofCoverageAuthorityState string

const (
	ProofCoverageMissing     ProofCoverageAuthorityState = "missing"
	ProofCoverageCovered     ProofCoverageAuthorityState = "covered"
	ProofCoverageWeak        ProofCoverageAuthorityState = "weak"
	ProofCoverageUnavailable ProofCoverageAuthorityState = "unavailable"
	ProofCoverageFailed      ProofCoverageAuthorityState = "failed"
)

type PermissionAuthorityState string

const (
	PermissionAuthorityUnknown PermissionAuthorityState = "unknown"
	PermissionAuthorityAllow   PermissionAuthorityState = "allow"
	PermissionAuthorityAsk     PermissionAuthorityState = "ask"
	PermissionAuthorityDeny    PermissionAuthorityState = "deny"
)

type LoopRecommendedAction string

const (
	LoopActionContinue    LoopRecommendedAction = "continue"
	LoopActionLocalize    LoopRecommendedAction = "localize"
	LoopActionVerify      LoopRecommendedAction = "verify"
	LoopActionAddProof    LoopRecommendedAction = "add_proof"
	LoopActionRepair      LoopRecommendedAction = "repair"
	LoopActionAskApproval LoopRecommendedAction = "ask_approval"
	LoopActionBlock       LoopRecommendedAction = "block"
)

type LocalizationAuthorityView struct {
	State               LocalizationAuthorityState `json:"state,omitempty"`
	ReasonCode          string                     `json:"reason_code,omitempty"`
	RecommendedAction   LoopRecommendedAction      `json:"recommended_action,omitempty"`
	SourcePaths         []string                   `json:"source_paths,omitempty"`
	AuxiliaryPaths      []string                   `json:"auxiliary_paths,omitempty"`
	OwnerSupportedPaths []string                   `json:"owner_supported_paths,omitempty"`
	OwnerMissingPaths   []string                   `json:"owner_missing_paths,omitempty"`
	SupportRatio        float64                    `json:"support_ratio,omitempty"`
	RequiresMoreContext bool                       `json:"requires_more_context,omitempty"`
}

type ProofCoverageAuthorityView struct {
	State             ProofCoverageAuthorityState           `json:"state,omitempty"`
	ReasonCode        string                                `json:"reason_code,omitempty"`
	RecommendedAction LoopRecommendedAction                 `json:"recommended_action,omitempty"`
	ProfileStatus     types.VerificationProofStatus         `json:"profile_status,omitempty"`
	LedgerState       types.VerificationProofLedgerState    `json:"ledger_state,omitempty"`
	RunnerEvidence    types.VerificationProofRunnerEvidence `json:"runner_evidence,omitempty"`
	RequiresProof     bool                                  `json:"requires_proof,omitempty"`
	AllowsUnverified  bool                                  `json:"allows_unverified,omitempty"`
}

type PermissionAuthorityView struct {
	State             PermissionAuthorityState `json:"state,omitempty"`
	ReasonCode        string                   `json:"reason_code,omitempty"`
	RecommendedAction LoopRecommendedAction    `json:"recommended_action,omitempty"`
	Source            string                   `json:"source,omitempty"`
	RequiresUser      bool                     `json:"requires_user,omitempty"`
}

func DeriveLocalizationAuthority(review *types.SourceLocalizationReview) LocalizationAuthorityView {
	if review == nil {
		return localizationAuthority(LocalizationAuthorityMissing, "localization_missing", LoopActionLocalize, nil)
	}
	normalized := types.NormalizeSourceLocalizationReview(*review)
	view := LocalizationAuthorityView{
		SourcePaths:         append([]string(nil), normalized.SourcePaths...),
		AuxiliaryPaths:      append([]string(nil), normalized.AuxiliaryPaths...),
		OwnerSupportedPaths: append([]string(nil), normalized.OwnerSupportedPaths...),
		OwnerMissingPaths:   append([]string(nil), normalized.OwnerMissingPaths...),
		SupportRatio:        normalized.SupportRatio,
	}
	set := func(state LocalizationAuthorityState, reason string, action LoopRecommendedAction) LocalizationAuthorityView {
		view.State = state
		view.ReasonCode = reason
		view.RecommendedAction = action
		view.RequiresMoreContext = action == LoopActionLocalize
		return view
	}
	switch {
	case len(normalized.OwnerSupportedPaths) > 0 && len(normalized.OwnerMissingPaths) == 0:
		return set(LocalizationAuthorityOwnerSupported, "localization_owner_supported", LoopActionContinue)
	case len(normalized.OwnerSupportedPaths) > 0 && len(normalized.OwnerMissingPaths) > 0:
		return set(LocalizationAuthorityConflicted, "localization_owner_partial", LoopActionLocalize)
	case normalized.Status == types.SourceLocalizationSupported:
		return set(LocalizationAuthorityOwnerSupported, "localization_supported", LoopActionContinue)
	case len(normalized.SourcePaths) > 0:
		return set(LocalizationAuthorityObservedOnly, "localization_observed_without_owner", LoopActionLocalize)
	case len(normalized.AuxiliaryPaths) > 0 || len(normalized.EvidenceRefs) > 0:
		return set(LocalizationAuthorityAuxiliaryOnly, "localization_auxiliary_only", LoopActionLocalize)
	case normalized.Status == types.SourceLocalizationUnknown:
		return set(LocalizationAuthorityUnknown, "localization_unknown", LoopActionLocalize)
	default:
		return set(LocalizationAuthorityMissing, "localization_missing", LoopActionLocalize)
	}
}

func DeriveProofCoverageAuthority(profile *types.VerificationProofProfile, ledger *types.VerificationProofLedger) ProofCoverageAuthorityView {
	view := ProofCoverageAuthorityView{}
	if ledger != nil {
		normalized := types.NormalizeVerificationProofLedger(*ledger)
		view.LedgerState = normalized.State
		view.ProfileStatus = normalized.ProfileStatus
		view.RunnerEvidence = normalized.RunnerEvidence
		switch normalized.State {
		case types.VerificationProofLedgerVerified:
			return proofAuthority(view, ProofCoverageCovered, "proof_covered", LoopActionContinue)
		case types.VerificationProofLedgerFailed:
			return proofAuthority(view, ProofCoverageFailed, "proof_failed", LoopActionRepair)
		case types.VerificationProofLedgerUnavailable:
			return proofAuthority(view, ProofCoverageUnavailable, "proof_unavailable", LoopActionContinue)
		case types.VerificationProofLedgerLowConfidence:
			return proofAuthority(view, ProofCoverageWeak, "proof_weak", LoopActionAddProof)
		}
	}
	if profile != nil {
		normalized := types.NormalizeVerificationProofProfile(*profile)
		view.ProfileStatus = normalized.Status
		view.RunnerEvidence = normalized.RunnerEvidence
		switch normalized.Status {
		case types.VerificationProofStrong, types.VerificationProofAdequate:
			return proofAuthority(view, ProofCoverageCovered, "proof_covered", LoopActionContinue)
		case types.VerificationProofFailed:
			return proofAuthority(view, ProofCoverageFailed, "proof_failed", LoopActionRepair)
		case types.VerificationProofUnavailable:
			return proofAuthority(view, ProofCoverageUnavailable, "proof_unavailable", LoopActionContinue)
		case types.VerificationProofWeak:
			return proofAuthority(view, ProofCoverageWeak, "proof_weak", LoopActionAddProof)
		}
	}
	return proofAuthority(view, ProofCoverageMissing, "proof_missing", LoopActionVerify)
}

func DerivePermissionAuthority(source string, decisions ...safety.PermissionDecision) PermissionAuthorityView {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "loopkernel"
	}
	folded := safety.FoldPermissionDecisions(source, decisions...)
	switch folded.Action {
	case safety.PermissionDeny:
		return PermissionAuthorityView{
			State:             PermissionAuthorityDeny,
			ReasonCode:        folded.ReasonCode,
			RecommendedAction: LoopActionBlock,
			Source:            folded.Source,
		}
	case safety.PermissionAsk:
		return PermissionAuthorityView{
			State:             PermissionAuthorityAsk,
			ReasonCode:        folded.ReasonCode,
			RecommendedAction: LoopActionAskApproval,
			Source:            folded.Source,
			RequiresUser:      true,
		}
	case safety.PermissionAllow:
		return PermissionAuthorityView{
			State:             PermissionAuthorityAllow,
			ReasonCode:        folded.ReasonCode,
			RecommendedAction: LoopActionContinue,
			Source:            folded.Source,
		}
	default:
		return PermissionAuthorityView{
			State:             PermissionAuthorityUnknown,
			ReasonCode:        "permission_unknown",
			RecommendedAction: LoopActionAskApproval,
			Source:            source,
			RequiresUser:      true,
		}
	}
}

func localizationAuthority(state LocalizationAuthorityState, reason string, action LoopRecommendedAction, paths []string) LocalizationAuthorityView {
	return LocalizationAuthorityView{
		State:               state,
		ReasonCode:          reason,
		RecommendedAction:   action,
		SourcePaths:         append([]string(nil), paths...),
		RequiresMoreContext: action == LoopActionLocalize,
	}
}

func proofAuthority(view ProofCoverageAuthorityView, state ProofCoverageAuthorityState, reason string, action LoopRecommendedAction) ProofCoverageAuthorityView {
	view.State = state
	view.ReasonCode = reason
	view.RecommendedAction = action
	view.RequiresProof = action == LoopActionVerify || action == LoopActionAddProof
	view.AllowsUnverified = state == ProofCoverageUnavailable
	return view
}
