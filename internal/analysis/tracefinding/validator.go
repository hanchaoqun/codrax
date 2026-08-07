// Package tracefinding validates model-selected findings against deterministic
// candidates and the causal-token registry.
package tracefinding

import (
	"fmt"
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
	if finding.PrimaryCause != nil {
		if err := validateStoredDecision(*finding.PrimaryCause); err != nil {
			return fmt.Errorf("primary_cause: %w", err)
		}
	}
	for i, decision := range finding.Contributors {
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
	if contract.CausalCeiling == "unproven" && decision.Status == types.TraceCausalProven {
		return fmt.Errorf("status proven exceeds causal ceiling unproven")
	}
	return nil
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
