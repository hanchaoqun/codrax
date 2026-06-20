package loopkernel

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const ReadProofAdapterSource = "read_turn_a_artifacts"

// ProofSnapshot is the lane-neutral proof projection consumed by loopkernel.
// It is derived from typed artifacts only: read-mode TurnAArtifacts,
// source-localization reviews, source-inventory observations, and write-mode
// verification artifacts. It never parses user prose, model rationale, prompt
// text, terminal narrative, or visible thinking logs.
type ProofSnapshot struct {
	Source                         string                         `json:"source,omitempty"`
	UnitID                         string                         `json:"unit_id,omitempty"`
	Profile                        types.VerificationProofProfile `json:"profile,omitempty"`
	Ledger                         types.VerificationProofLedger  `json:"ledger,omitempty"`
	Authority                      ProofCoverageAuthorityView     `json:"authority,omitempty"`
	Truth                          types.TruthLedger              `json:"truth,omitempty"`
	Paths                          []string                       `json:"paths,omitempty"`
	EvidenceRefs                   []string                       `json:"evidence_refs,omitempty"`
	AcceptedResultKind             string                         `json:"accepted_result_kind,omitempty"`
	EvidenceItemCount              int                            `json:"evidence_item_count,omitempty"`
	AggregateFactCount             int                            `json:"aggregate_fact_count,omitempty"`
	SourceInventoryActive          bool                           `json:"source_inventory_active,omitempty"`
	SourceInventoryComplete        bool                           `json:"source_inventory_complete,omitempty"`
	RuntimeObservationOnlyComplete bool                           `json:"runtime_observation_only_complete,omitempty"`
	LocalizationStatus             types.SourceLocalizationStatus `json:"localization_status,omitempty"`
	LocalizationAuthority          LocalizationAuthorityView      `json:"localization_authority,omitempty"`
}

func ProofSnapshotFromReadTurnA(turnA *types.TurnAArtifacts) (ProofSnapshot, bool) {
	if turnA == nil {
		return ProofSnapshot{}, false
	}
	snapshot := ProofSnapshot{
		Source:                         ReadProofAdapterSource,
		UnitID:                         "read:explore",
		Paths:                          compactWriteWorkflowStrings(turnA.ReadFiles...),
		EvidenceRefs:                   readProofEvidenceRefs(turnA),
		AcceptedResultKind:             strings.TrimSpace(turnA.AcceptedResultKind),
		EvidenceItemCount:              len(turnA.EvidenceItems),
		AggregateFactCount:             len(turnA.AcceptedAggregateFacts),
		SourceInventoryActive:          turnA.SourceInventoryObservation.IsActive(),
		SourceInventoryComplete:        turnA.SourceInventoryObservation.IsActive() && turnA.SourceInventoryObservation.Complete,
		RuntimeObservationOnlyComplete: turnA.RuntimeObservationOnlyCompletion,
	}
	if types.SourceLocalizationReviewHasSignal(turnA.SourceLocalization) {
		localization := types.NormalizeSourceLocalizationReview(*turnA.SourceLocalization)
		snapshot.LocalizationStatus = localization.Status
		snapshot.LocalizationAuthority = DeriveLocalizationAuthority(&localization)
		snapshot.Paths = compactWriteWorkflowStrings(append(snapshot.Paths, localization.SourcePaths...)...)
		snapshot.Paths = compactWriteWorkflowStrings(append(snapshot.Paths, localization.OwnerSupportedPaths...)...)
	}
	snapshot.Profile, snapshot.Ledger = readProofArtifactsFromSnapshot(snapshot)
	snapshot.Authority = DeriveProofCoverageAuthorityFromSnapshot(snapshot)
	if truth, ok := TruthLedgerFromProofCoverageAuthority(ReadProofAdapterSource, snapshot.Authority, snapshot.Paths, snapshot.EvidenceRefs); ok {
		snapshot.Truth = truth
	}
	return normalizeProofSnapshot(snapshot), true
}

func DeriveProofCoverageAuthorityFromSnapshot(snapshot ProofSnapshot) ProofCoverageAuthorityView {
	return DeriveProofCoverageAuthority(&snapshot.Profile, &snapshot.Ledger)
}

func EventsFromReadProofSnapshot(runID string, snapshot ProofSnapshot) []LoopEvent {
	snapshot = normalizeProofSnapshot(snapshot)
	if snapshot.Authority.State == "" {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = "read"
	}
	unitID := strings.TrimSpace(snapshot.UnitID)
	if unitID == "" {
		unitID = "read:explore"
	}
	events := []LoopEvent{{
		ID:         fmt.Sprintf("%s:%06d:%s", runID, 1, LoopEventProofProjected),
		RunID:      runID,
		UnitID:     unitID,
		Sequence:   1,
		Kind:       LoopEventProofProjected,
		Source:     firstNonEmptyString(snapshot.Source, ReadProofAdapterSource),
		ReasonCode: snapshot.Authority.ReasonCode,
		Payload:    EventPayload(snapshot.Authority),
	}}
	if snapshot.Truth.State != types.TruthLedgerUnknown {
		events = append(events, LoopEvent{
			ID:         fmt.Sprintf("%s:%06d:%s", runID, 2, LoopEventTruthProjected),
			RunID:      runID,
			UnitID:     unitID,
			Sequence:   2,
			Kind:       LoopEventTruthProjected,
			Source:     firstNonEmptyString(snapshot.Source, ReadProofAdapterSource),
			ReasonCode: snapshot.Truth.ReasonCode,
			Payload:    EventPayload(snapshot.Truth),
		})
	}
	return NormalizeLoopEvents(events)
}

func TruthLedgerFromProofCoverageAuthority(source string, proof ProofCoverageAuthorityView, paths, evidenceRefs []string) (types.TruthLedger, bool) {
	proof = normalizeProofCoverageAuthorityView(proof)
	state := types.TruthLedgerUnknown
	action := types.TruthActionVerify
	switch proof.State {
	case ProofCoverageCovered:
		state = types.TruthLedgerCovered
		action = types.TruthActionContinue
	case ProofCoverageFailed:
		state = types.TruthLedgerFailed
		action = types.TruthActionRepair
	case ProofCoverageUnavailable:
		state = types.TruthLedgerUnverified
		action = types.TruthActionContinue
	case ProofCoverageWeak, ProofCoverageMissing:
		state = types.TruthLedgerWeak
		action = types.TruthActionAddProof
	default:
		return types.TruthLedger{}, false
	}
	reason := firstNonEmptyString(proof.ReasonCode, "proof_coverage")
	return types.NormalizeTruthLedger(types.TruthLedger{
		State:             state,
		ReasonCode:        reason,
		RecommendedAction: action,
		Signals: []types.TruthSignal{{
			Kind:              types.TruthSignalProofCoverage,
			State:             state,
			ReasonCode:        reason,
			RecommendedAction: action,
			Source:            strings.TrimSpace(source),
			Paths:             compactWriteWorkflowStrings(paths...),
			EvidenceRefs:      compactWriteWorkflowStrings(evidenceRefs...),
			RequiresAction:    state == types.TruthLedgerFailed || state == types.TruthLedgerWeak,
		}},
	}), true
}

func readProofArtifactsFromSnapshot(snapshot ProofSnapshot) (types.VerificationProofProfile, types.VerificationProofLedger) {
	profile := types.VerificationProofProfile{
		RunnerEvidence:     types.VerificationProofRunnerNone,
		LocalizationStatus: snapshot.LocalizationStatus,
		ImpactTargetCount:  snapshot.EvidenceItemCount + snapshot.AggregateFactCount,
	}
	ledger := types.VerificationProofLedger{
		RunnerEvidence:  types.VerificationProofRunnerNone,
		CoverageCounts:  map[string]int{},
		ReasonCodes:     nil,
		Obligations:     nil,
		Capabilities:    nil,
		ObligationCount: 0,
	}
	hasAcceptedClosure := strings.TrimSpace(snapshot.AcceptedResultKind) != ""
	hasTypedEvidence := snapshot.EvidenceItemCount > 0 ||
		snapshot.AggregateFactCount > 0 ||
		snapshot.SourceInventoryActive ||
		snapshot.RuntimeObservationOnlyComplete
	if !hasAcceptedClosure && !hasTypedEvidence {
		profile.Status = types.VerificationProofUnknown
		ledger.State = types.VerificationProofLedgerUnknown
		ledger.ProfileStatus = profile.Status
		return types.NormalizeVerificationProofProfile(profile), types.NormalizeVerificationProofLedger(ledger)
	}
	if snapshot.RuntimeObservationOnlyComplete {
		profile.Status = types.VerificationProofAdequate
		profile.RunnerEvidence = types.VerificationProofRunnerVerificationProbe
		ledger.State = types.VerificationProofLedgerVerified
		ledger.ProfileStatus = profile.Status
		ledger.RunnerEvidence = profile.RunnerEvidence
		ledger.Obligations = append(ledger.Obligations, types.VerificationProofLedgerItem{
			ID:         "read:runtime_observation",
			Kind:       "runtime_observation",
			Status:     types.VerificationProofLedgerItemCovered,
			Source:     ReadProofAdapterSource,
			ReasonCode: "runtime_observation_only_completion",
		})
		return types.NormalizeVerificationProofProfile(profile), types.NormalizeVerificationProofLedger(ledger)
	}
	localizationCovered := snapshot.LocalizationAuthority.State == LocalizationAuthorityOwnerSupported
	inventoryCovered := snapshot.SourceInventoryActive && snapshot.SourceInventoryComplete
	switch {
	case localizationCovered || inventoryCovered:
		profile.Status = types.VerificationProofStrong
		ledger.State = types.VerificationProofLedgerVerified
	case hasAcceptedClosure && hasTypedEvidence:
		profile.Status = types.VerificationProofWeak
		profile.ReasonCodes = append(profile.ReasonCodes, "read_proof_needs_owner_or_complete_inventory")
		ledger.State = types.VerificationProofLedgerLowConfidence
		ledger.ReasonCodes = append(ledger.ReasonCodes, "read_proof_needs_owner_or_complete_inventory")
	default:
		profile.Status = types.VerificationProofUnknown
		ledger.State = types.VerificationProofLedgerUnknown
	}
	ledger.ProfileStatus = profile.Status
	ledger.RunnerEvidence = profile.RunnerEvidence
	status := types.VerificationProofLedgerItemCovered
	if profile.Status == types.VerificationProofWeak {
		status = types.VerificationProofLedgerItemUnverified
	}
	for _, ref := range snapshot.EvidenceRefs {
		ledger.Obligations = append(ledger.Obligations, types.VerificationProofLedgerItem{
			ID:          "read:evidence:" + ref,
			Kind:        "read_evidence",
			Status:      status,
			Source:      ReadProofAdapterSource,
			EvidenceRef: ref,
			ReasonCode:  firstNonEmptyString(snapshot.AcceptedResultKind, "read_evidence"),
		})
	}
	if len(ledger.Obligations) == 0 && snapshot.SourceInventoryActive {
		ledger.Obligations = append(ledger.Obligations, types.VerificationProofLedgerItem{
			ID:         "read:source_inventory",
			Kind:       "source_inventory",
			Status:     status,
			Source:     ReadProofAdapterSource,
			ReasonCode: "source_inventory_observation",
		})
	}
	return types.NormalizeVerificationProofProfile(profile), types.NormalizeVerificationProofLedger(ledger)
}

func readProofEvidenceRefs(turnA *types.TurnAArtifacts) []string {
	if turnA == nil {
		return nil
	}
	var refs []string
	for _, item := range turnA.EvidenceItems {
		ref := strings.TrimSpace(item.ID)
		if ref == "" {
			ref = types.StableEvidenceID(item)
		}
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	for _, set := range turnA.SourceInventoryObservation.Sets {
		for _, member := range set.Members {
			if ref := strings.TrimSpace(member.SupportRef); ref != "" {
				refs = append(refs, ref)
			}
		}
	}
	return compactWriteWorkflowStrings(refs...)
}

func normalizeProofSnapshot(snapshot ProofSnapshot) ProofSnapshot {
	snapshot.Source = firstNonEmptyString(snapshot.Source, ReadProofAdapterSource)
	snapshot.UnitID = firstNonEmptyString(snapshot.UnitID, "read:explore")
	snapshot.Paths = compactWriteWorkflowStrings(snapshot.Paths...)
	snapshot.EvidenceRefs = compactWriteWorkflowStrings(snapshot.EvidenceRefs...)
	snapshot.Profile = types.NormalizeVerificationProofProfile(snapshot.Profile)
	snapshot.Ledger = types.NormalizeVerificationProofLedger(snapshot.Ledger)
	snapshot.Authority = normalizeProofCoverageAuthorityView(snapshot.Authority)
	if snapshot.Authority.State == "" {
		snapshot.Authority = DeriveProofCoverageAuthority(&snapshot.Profile, &snapshot.Ledger)
	}
	snapshot.Truth = types.NormalizeTruthLedger(snapshot.Truth)
	if snapshot.Truth.State == types.TruthLedgerUnknown {
		if truth, ok := TruthLedgerFromProofCoverageAuthority(snapshot.Source, snapshot.Authority, snapshot.Paths, snapshot.EvidenceRefs); ok {
			snapshot.Truth = truth
		}
	}
	return snapshot
}
