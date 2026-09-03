// Package tracefinding validates model-selected findings against deterministic
// candidates and the causal-token registry.
package tracefinding

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func Validate(finding *types.TraceFindingV1, contract *types.TraceFindingContract) error {
	if contract == nil || !contract.Required {
		return nil
	}
	if err := ValidateStored(finding); err != nil {
		return err
	}
	wantVersion := contract.FindingSchemaVersion
	if wantVersion == 0 {
		wantVersion = types.TraceFindingSchemaVersion
	}
	if finding.SchemaVersion != wantVersion {
		return fmt.Errorf("trace_finding schema_version=%d, want %d", finding.SchemaVersion, wantVersion)
	}
	if contract.FindingID != "" && finding.FindingID != contract.FindingID {
		return fmt.Errorf("trace_finding finding_id=%q, want %q", finding.FindingID, contract.FindingID)
	}
	if contract.AnalysisKey != "" && finding.AnalysisKey != contract.AnalysisKey {
		return fmt.Errorf("trace_finding analysis_key=%q, want %q", finding.AnalysisKey, contract.AnalysisKey)
	}
	if contract.ContractHash != "" && finding.Revision.ContractHash != contract.ContractHash {
		return fmt.Errorf("trace_finding revision.contract_hash does not match candidate contract")
	}
	if contract.Artifact.ArtifactID != "" && finding.Artifact.ArtifactID != contract.Artifact.ArtifactID {
		return fmt.Errorf("trace_finding artifact_id does not match candidate contract")
	}
	acceptedEvidence := stringSet(contract.AcceptedEvidenceIDs)
	for _, ref := range finding.EvidenceRefs {
		if !acceptedEvidence[ref] {
			return fmt.Errorf("trace_finding evidence_ref %q is outside accepted evidence", ref)
		}
	}
	for _, ref := range finding.CounterEvidenceRefs {
		if !acceptedEvidence[ref] {
			return fmt.Errorf("trace_finding counter_evidence_ref %q is outside accepted evidence", ref)
		}
	}
	if finding.PrimaryCause != nil {
		if err := validateDecision(*finding.PrimaryCause, stringSet(contract.PrimaryCandidateIDs), acceptedEvidence, contract); err != nil {
			return fmt.Errorf("primary_cause: %w", err)
		}
	}
	if finding.Unresolved != nil && len(finding.Contributors) > 0 {
		return fmt.Errorf("unresolved trace_finding cannot contain contributors")
	}
	contributorIDs := stringSet(contract.ContributorCandidateIDs)
	seen := map[string]bool{}
	for i, decision := range finding.Contributors {
		if seen[decision.CandidateID] {
			return fmt.Errorf("contributors[%d]: duplicate candidate_id %q", i, decision.CandidateID)
		}
		seen[decision.CandidateID] = true
		if err := validateDecision(decision, contributorIDs, acceptedEvidence, contract); err != nil {
			return fmt.Errorf("contributors[%d]: %w", i, err)
		}
	}
	return nil
}

// ValidateStored checks the self-contained invariants required before a
// persisted finding may be consumed by clustering.
func ValidateStored(finding *types.TraceFindingV1) error {
	if finding == nil {
		return fmt.Errorf("trace_finding is required")
	}
	if finding.SchemaVersion != types.TraceFindingSchemaVersion {
		return fmt.Errorf("unsupported trace_finding schema_version %d", finding.SchemaVersion)
	}
	if strings.TrimSpace(finding.FindingID) == "" || strings.TrimSpace(finding.AnalysisKey) == "" {
		return fmt.Errorf("trace_finding requires finding_id and analysis_key")
	}
	if (finding.PrimaryCause == nil) == (finding.Unresolved == nil) {
		return fmt.Errorf("trace_finding must contain exactly one of primary_cause or unresolved")
	}
	if finding.Unresolved != nil && len(finding.Contributors) > 0 {
		return fmt.Errorf("unresolved trace_finding cannot contain contributors")
	}
	seen := map[string]bool{}
	if finding.PrimaryCause != nil {
		if err := validateStoredDecision(*finding.PrimaryCause); err != nil {
			return fmt.Errorf("primary_cause: %w", err)
		}
		seen[finding.PrimaryCause.CandidateID] = true
	}
	for i, decision := range finding.Contributors {
		if seen[decision.CandidateID] {
			return fmt.Errorf("contributors[%d]: duplicate candidate_id %q", i, decision.CandidateID)
		}
		seen[decision.CandidateID] = true
		if err := validateStoredDecision(decision); err != nil {
			return fmt.Errorf("contributors[%d]: %w", i, err)
		}
	}
	return nil
}

func validateStoredDecision(decision types.TraceCauseDecision) error {
	if strings.TrimSpace(decision.CandidateID) == "" {
		return fmt.Errorf("candidate_id is required")
	}
	if len(decision.EvidenceRefs) == 0 {
		return fmt.Errorf("candidate_id %q requires at least one evidence_ref", decision.CandidateID)
	}
	spec, ok := tracequery.CausalTokenSpecFor(decision.Token.Token)
	if !ok || !spec.RowToken {
		return fmt.Errorf("token %q is not a registered root-cause token", decision.Token.Token)
	}
	if decision.Token.Lane != string(spec.Lane) || decision.Token.Additivity != string(spec.Additivity) || decision.Token.SubjectKind != string(spec.Subject) {
		return fmt.Errorf("token %q registry snapshot does not match current registry", decision.Token.Token)
	}
	if decision.Token.FixDirection != string(tracequery.CausalTokenFixDirectionFor(decision.Token.Token)) {
		return fmt.Errorf("token %q fix_direction does not match current registry", decision.Token.Token)
	}
	if decision.Status != types.TraceCausalProven && decision.Status != types.TraceCausalSupportedCandidate {
		return fmt.Errorf("status %q is not valid for a selected cause", decision.Status)
	}
	return nil
}

func validateDecision(decision types.TraceCauseDecision, eligible, acceptedEvidence map[string]bool, contract *types.TraceFindingContract) error {
	if !eligible[decision.CandidateID] {
		return fmt.Errorf("candidate_id %q is not eligible", decision.CandidateID)
	}
	for _, ref := range decision.EvidenceRefs {
		if !acceptedEvidence[ref] {
			return fmt.Errorf("evidence_ref %q is outside accepted evidence", ref)
		}
	}
	if contract.RegistryHash != "" && decision.Token.RegistryHash != contract.RegistryHash {
		return fmt.Errorf("token %q registry_hash does not match candidate contract", decision.Token.Token)
	}
	// SIDECAR-Q1 (§40.28 ②): the ceiling is the typed closed-set qualifier
	// derived from the seats (never a bare literal — the "unproven" string
	// the compiler stopped producing left this arm dead until the batch's
	// adversarial review caught it).
	if contract.CausalCeiling == types.TraceCausalQualifierFrameUnproven && decision.Status == types.TraceCausalProven {
		return fmt.Errorf("status proven exceeds causal ceiling %s", types.TraceCausalQualifierFrameUnproven)
	}
	if len(contract.Candidates) > 0 {
		candidate, ok := contractCandidate(contract.Candidates, decision.CandidateID)
		if !ok {
			return fmt.Errorf("candidate_id %q has no deterministic candidate snapshot", decision.CandidateID)
		}
		if err := validateCandidateSnapshot(decision, candidate.Decision); err != nil {
			return err
		}
	}
	return nil
}

func contractCandidate(candidates []types.TraceFindingCandidateV1, id string) (types.TraceFindingCandidateV1, bool) {
	for _, candidate := range candidates {
		if candidate.Decision.CandidateID == id {
			return candidate, true
		}
	}
	return types.TraceFindingCandidateV1{}, false
}

func validateCandidateSnapshot(got, want types.TraceCauseDecision) error {
	if got.Token != want.Token ||
		got.SubjectName != want.SubjectName ||
		got.SubjectRole != want.SubjectRole || got.UpstreamRole != want.UpstreamRole ||
		got.ResourceName != want.ResourceName || got.PhaseName != want.PhaseName ||
		got.BlockingKind != want.BlockingKind ||
		got.CausalShape != want.CausalShape || got.Phase != want.Phase ||
		got.Rank != want.Rank || got.Tier != want.Tier ||
		got.BoardFingerprint != want.BoardFingerprint ||
		got.NormalizedEventKey != want.NormalizedEventKey ||
		got.NormalizedStackKey != want.NormalizedStackKey {
		return fmt.Errorf("candidate_id %q rewrites system-owned candidate fields", got.CandidateID)
	}
	if !reflect.DeepEqual(got.Magnitude, want.Magnitude) {
		return fmt.Errorf("candidate_id %q rewrites system-owned magnitude", got.CandidateID)
	}
	if !reflect.DeepEqual(got.EvidenceFacts, want.EvidenceFacts) {
		return fmt.Errorf("candidate_id %q rewrites system-owned evidence facts", got.CandidateID)
	}
	if !sameStringSet(got.EvidenceRefs, want.EvidenceRefs) {
		return fmt.Errorf("candidate_id %q evidence_refs do not match the deterministic candidate", got.CandidateID)
	}
	// SIDECAR-Q1 (§40.28 ②): the seat-level qualifier is system-owned and
	// ALWAYS explicit — the model copies it verbatim (blank or spoofed values
	// are rejected, never inferred), and a frame-unproven seat caps its own
	// status below proven regardless of the contract-wide ceiling.
	if got.CausalQualifier != want.CausalQualifier {
		return fmt.Errorf("candidate_id %q rewrites system-owned causal_qualifier (%q ≠ %q)", got.CandidateID, got.CausalQualifier, want.CausalQualifier)
	}
	if want.CausalQualifier == types.TraceCausalQualifierFrameUnproven && got.Status == types.TraceCausalProven {
		return fmt.Errorf("candidate_id %q status proven exceeds its seat-level causal qualifier %s", got.CandidateID, want.CausalQualifier)
	}
	return nil
}

func sameStringSet(a, b []string) bool {
	left, right := stringSet(a), stringSet(b)
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if !right[value] {
			return false
		}
	}
	return true
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out[value] = true
		}
	}
	return out
}
