package tracefinding

import "github.com/hanchaoqun/codrax/internal/types"

// BuildDeterministicFinding selects the highest-ranked eligible candidate
// already compiled from typed trace evidence. The model is deliberately not
// involved, so adding a sidecar cannot make the original final answer fail.
func BuildDeterministicFinding(contract *types.TraceFindingContract) *types.TraceFindingV1 {
	if contract == nil || contract.FindingSchemaVersion != types.TraceFindingSchemaVersion {
		return nil
	}
	finding := &types.TraceFindingV1{
		SchemaVersion: types.TraceFindingSchemaVersion,
		FindingID:     contract.FindingID,
		AnalysisKey:   contract.AnalysisKey,
		Artifact:      contract.Artifact,
		Scope:         contract.Scope,
		Revision: types.TraceFindingRevision{
			ContractHash: contract.ContractHash,
		},
		Symptom: contract.Symptom,
	}
	for _, candidate := range contract.Candidates {
		if !candidate.PrimaryEligible {
			continue
		}
		decision := cloneTraceCauseDecision(candidate.Decision)
		finding.PrimaryCause = &decision
		finding.EvidenceRefs = append([]string(nil), decision.EvidenceRefs...)
		finding.Coverage.Complete = true
		return finding
	}
	finding.Unresolved = &types.TraceUnresolvedDecision{
		Reason: "the attached trace did not produce a supported root-cause candidate",
	}
	finding.EvidenceRefs = append([]string(nil), contract.AcceptedEvidenceIDs...)
	finding.Coverage = types.TraceFindingCoverage{
		Complete: false,
		Caveats:  []string{"root cause remains unresolved because typed trace evidence was insufficient"},
	}
	return finding
}

func cloneTraceCauseDecision(in types.TraceCauseDecision) types.TraceCauseDecision {
	out := in
	out.EvidenceRefs = append([]string(nil), in.EvidenceRefs...)
	if in.Magnitude != nil {
		magnitude := *in.Magnitude
		out.Magnitude = &magnitude
	}
	return out
}
