package types

// The explicit primary plan, not artifact ordering, owns current registration
// authority. Historical assertions remain visible but cannot discharge its
// newly registered obligations, even after another effective-report projection.
func effectiveCumulativeNativeRegistrationArtifacts(primary *ChangePlan, artifacts []VerificationProofArtifact) []VerificationProofArtifact {
	if primary == nil || (primary.NativeTestRegistration == nil && primary.PersistenceKind != PlanPersistenceNativeTestRegistration) {
		return artifacts
	}
	refs := map[string]bool{}
	for _, observation := range primary.ProjectTestObservations {
		for _, ref := range observation.ContractRefs {
			refs[ref] = true
		}
	}
	if len(refs) == 0 || len(artifacts) < 2 {
		return artifacts
	}
	out := append([]VerificationProofArtifact(nil), artifacts...)
	for i := 1; i < len(out); i++ {
		if out[i].Report == nil {
			continue
		}
		report := *out[i].Report
		report.verificationExcludedContractRefs = make(map[string]bool, len(refs)+len(report.verificationExcludedContractRefs))
		for ref, excluded := range out[i].Report.verificationExcludedContractRefs {
			report.verificationExcludedContractRefs[ref] = excluded
		}
		for ref := range refs {
			report.verificationExcludedContractRefs[ref] = true
		}
		report.VerificationConfidence = EffectiveVerificationConfidence(out[i].Plan, &report)
		out[i].Report = &report
	}
	return out
}

func excludeCumulativeNativeRegistrationConfidence(report *ChangeReport, records []VerificationConfidenceRecord) []VerificationConfidenceRecord {
	if report == nil || len(report.verificationExcludedContractRefs) == 0 {
		return records
	}
	out := make([]VerificationConfidenceRecord, 0, len(records))
	for _, record := range records {
		if record.Status != "satisfied" || len(record.ContractRefs) == 0 {
			out = append(out, record)
			continue
		}
		var retained, excluded []string
		for _, ref := range record.ContractRefs {
			if report.verificationExcludedContractRefs[ref] {
				excluded = append(excluded, ref)
			} else {
				retained = append(retained, ref)
			}
		}
		if len(excluded) == 0 {
			out = append(out, record)
			continue
		}
		if len(retained) > 0 {
			kept := record
			kept.ContractRefs = retained
			out = append(out, kept)
		}
		record.ContractRefs = excluded
		record.Status, record.Severity = "advisory", "info"
		record.ReasonCode = "native_test_registration_current_receipt_required"
		record.Detail = "Historical contract proof cannot substitute for the current native test registration's receipt."
		out = append(out, record)
	}
	return out
}
