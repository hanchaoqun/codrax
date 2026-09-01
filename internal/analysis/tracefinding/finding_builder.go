package tracefinding

import "github.com/hanchaoqun/codrax/internal/types"

// BuildDeterministicFinding preserves the legacy artifact envelope without
// selecting a conclusion. Candidate ranking is system evidence, not authority
// to decide which diagnosis the model adopts.
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
	finding.Unresolved = &types.TraceUnresolvedDecision{
		Reason: "no model-owned typed candidate selection was committed to this legacy artifact",
	}
	finding.EvidenceRefs = append([]string(nil), contract.AcceptedEvidenceIDs...)
	finding.Coverage = types.TraceFindingCoverage{
		Complete: false,
		Caveats:  []string{"typed candidates are evidence inputs; the system does not select a root cause"},
	}
	return finding
}
