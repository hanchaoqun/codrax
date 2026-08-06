package tracefinding

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ValidateTraceFinding checks structural, candidate, token, evidence, and
// causal-ceiling invariants for a TraceFindingV1 against a compiled candidate set.
func ValidateTraceFinding(finding *types.TraceFindingV1, candidates types.TraceDecisionCandidateSetV1) error {
	if finding == nil {
		return fmt.Errorf("finding_missing: nil TraceFindingV1")
	}
	if finding.SchemaVersion != 0 && finding.SchemaVersion != types.TraceFindingSchemaVersion {
		return fmt.Errorf("finding_schema_invalid: unsupported schema_version %d", finding.SchemaVersion)
	}
	if strings.TrimSpace(finding.FindingID) == "" {
		return fmt.Errorf("finding_schema_invalid: empty finding_id")
	}
	hasPrimary := finding.PrimaryCause != nil
	hasUnresolved := finding.Unresolved != nil
	if !hasPrimary && !hasUnresolved {
		return fmt.Errorf("finding_schema_invalid: primary_cause and unresolved are both empty")
	}
	if hasPrimary && hasUnresolved {
		return fmt.Errorf("finding_schema_invalid: primary_cause and unresolved cannot both be final conclusions")
	}

	primaryIDs := candidateIDSet(candidates.PrimaryEligible)
	contributorIDs := candidateIDSet(candidates.ContributorEligible)
	contextIDs := candidateIDSet(candidates.ContextOnly)
	acceptedEvidence := stringSet(candidates.AcceptedEvidenceIDs)

	if hasPrimary {
		if err := validateCauseDecision(*finding.PrimaryCause, primaryIDs, contextIDs, acceptedEvidence, candidates, true); err != nil {
			return err
		}
	}
	for i, c := range finding.Contributors {
		if err := validateCauseDecision(c, contributorIDs, contextIDs, acceptedEvidence, candidates, false); err != nil {
			return fmt.Errorf("contributors[%d]: %w", i, err)
		}
	}
	for _, id := range finding.EvidenceRefs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !acceptedEvidence[id] {
			return fmt.Errorf("finding_evidence_violation: evidence_refs %q not in accepted evidence", id)
		}
	}
	for _, id := range finding.CounterEvidenceRefs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !acceptedEvidence[id] {
			return fmt.Errorf("finding_evidence_violation: counter_evidence_refs %q not in accepted evidence", id)
		}
	}
	if candidates.CausalCeiling.ConclusionUnproven && hasPrimary && finding.PrimaryCause.Status == types.TraceCausalProven {
		return fmt.Errorf("finding_causal_ceiling_violation: proven status forbidden when causal ceiling is unproven")
	}
	return nil
}

func validateCauseDecision(
	cause types.TraceCauseDecision,
	eligible map[string]types.TraceCauseCandidate,
	contextIDs map[string]types.TraceCauseCandidate,
	acceptedEvidence map[string]bool,
	candidates types.TraceDecisionCandidateSetV1,
	primary bool,
) error {
	id := strings.TrimSpace(cause.CandidateID)
	if id == "" {
		return fmt.Errorf("finding_candidate_violation: empty candidate_id")
	}
	if _, isContext := contextIDs[id]; isContext && primary {
		return fmt.Errorf("finding_candidate_violation: context-only candidate %q cannot be primary", id)
	}
	cand, ok := eligible[id]
	if !ok {
		return fmt.Errorf("finding_candidate_violation: candidate_id %q not in eligible set", id)
	}
	if err := validateTokenSnapshot(cause.Token, cand.Token); err != nil {
		return err
	}
	if len(cause.EvidenceRefs) == 0 {
		return fmt.Errorf("finding_evidence_violation: cause requires at least one evidence_ref")
	}
	for _, eid := range cause.EvidenceRefs {
		eid = strings.TrimSpace(eid)
		if eid == "" || !acceptedEvidence[eid] {
			return fmt.Errorf("finding_evidence_violation: evidence_ref %q not accepted", eid)
		}
	}
	switch cause.Status {
	case types.TraceCausalProven, types.TraceCausalSupportedCandidate, types.TraceCausalUnresolved, "":
	default:
		return fmt.Errorf("finding_schema_invalid: unknown status %q", cause.Status)
	}
	if candidates.CausalCeiling.ConclusionUnproven && cause.Status == types.TraceCausalProven {
		return fmt.Errorf("finding_causal_ceiling_violation: proven forbidden under unproven ceiling")
	}
	_ = candidates
	return nil
}

func validateTokenSnapshot(got, want types.TraceCausalTokenSnapshot) error {
	if strings.TrimSpace(got.Token) == "" {
		return fmt.Errorf("finding_schema_invalid: empty token")
	}
	live, err := SnapshotToken(got.Token)
	if err != nil {
		return fmt.Errorf("finding_schema_invalid: %w", err)
	}
	if got.Token != live.Token ||
		got.Lane != live.Lane ||
		got.Additivity != live.Additivity ||
		got.SubjectKind != live.SubjectKind {
		return fmt.Errorf("finding_schema_invalid: token snapshot diverges from registry for %q", got.Token)
	}
	if want.Token != "" && got.Token != want.Token {
		return fmt.Errorf("finding_candidate_violation: token %q does not match candidate token %q", got.Token, want.Token)
	}
	if want.RegistryHash != "" && got.RegistryHash != "" && got.RegistryHash != want.RegistryHash {
		return fmt.Errorf("finding_schema_invalid: registry_hash mismatch")
	}
	return nil
}

func candidateIDSet(in []types.TraceCauseCandidate) map[string]types.TraceCauseCandidate {
	out := make(map[string]types.TraceCauseCandidate, len(in))
	for _, c := range in {
		id := strings.TrimSpace(c.CandidateID)
		if id == "" {
			continue
		}
		out[id] = c
	}
	return out
}

func stringSet(in []string) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out[s] = true
	}
	return out
}
