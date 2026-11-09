package types

import "reflect"

// NativeTestRegistrationAssertionMatches is the shared success/failure join
// for read-only native registrations. The caller's ordinary runner/path join
// remains necessary. No persisted confidence label, legacy blank invocation,
// or declaration alone can authorize a newly registered assertion.
func NativeTestRegistrationAssertionMatches(plan *ChangePlan, report *ChangeReport, observation ProjectTestObservation, commandIndex, resultIndex int) bool {
	if plan == nil || report == nil {
		return false
	}
	if !nativeTestRegistrationProofScope(plan, report) {
		return true // Preserve the existing ordinary source/test-plan protocol.
	}
	receipt, _ := nativeTestRegistrationAssertionReceipt(plan, report, observation, commandIndex, resultIndex)
	return receipt != nil
}

func nativeTestRegistrationAssertionReceipt(plan *ChangePlan, report *ChangeReport, observation ProjectTestObservation, commandIndex, resultIndex int) (*ExistingTestExecutionReceipt, bool) {
	if plan == nil || report == nil || NativeTestRegistrationDigest(plan) == "" ||
		commandIndex < 0 || commandIndex >= len(report.ExecutedCommands) ||
		resultIndex < 0 || resultIndex >= len(report.TestResults) ||
		len(report.ExistingTestExecutions) > MaxExistingTestExecutionReceipts {
		return nil, false
	}
	declared := false
	for _, current := range plan.ProjectTestObservations {
		if reflect.DeepEqual(current, observation) {
			declared = true
			break
		}
	}
	if !declared {
		return nil, false
	}
	command, row := report.ExecutedCommands[commandIndex], report.TestResults[resultIndex]
	invocations := NewNativeTestInvocationIndex(report)
	if command.InvocationID == "" || !invocations.Matches(commandIndex, resultIndex) ||
		row.ObservationScope != TestObservationScopeAssertion || row.Kind == TestResultKindBuildError ||
		row.Suite != observation.AssertionSuite || row.AssertionID != observation.AssertionID ||
		(command.ExitCode == 0) != row.Passed {
		return nil, false
	}
	delivery, ok := ResolveVerificationDelivery(plan)
	if !ok {
		return nil, false
	}
	digest := NativeTestRegistrationDigest(plan)
	assertionDigest := ExistingTestAssertionDigest(row)
	var found *ExistingTestExecutionReceipt
	for i := range report.ExistingTestExecutions {
		receipt := &report.ExistingTestExecutions[i]
		if receipt.NativeTestRegistrationDigest != digest || receipt.TestPath != observation.TestPath ||
			receipt.CommandIndex != commandIndex ||
			!existingTestExecutionReceiptMatches(plan, delivery, report, *receipt, invocations) {
			continue
		}
		for _, assertion := range receipt.AssertionDigests {
			if assertion == assertionDigest {
				if found != nil {
					return found, false // Execution is real, source binding is ambiguous.
				}
				found = receipt
			}
		}
	}
	return found, found != nil
}

// NativeTestRegistrationContractRefs keeps the exact execution identity and
// contract applicability distinct. When the active contract names a line in
// a registered test file, only the same physical method range can witness it.
// Non-test evidence and ordinary plans retain their existing protocol. A
// method range is a necessary source binding, not a semantic proof by itself.
func NativeTestRegistrationContractRefs(plan *ChangePlan, report *ChangeReport, observation ProjectTestObservation, commandIndex, resultIndex int) []string {
	if plan == nil || report == nil {
		return nil
	}
	if !nativeTestRegistrationProofScope(plan, report) {
		return observation.ContractRefs
	}
	receipt, unique := nativeTestRegistrationAssertionReceipt(plan, report, observation, commandIndex, resultIndex)
	if receipt == nil {
		return nil
	}
	var span ExistingTestAssertionSourceRange
	if unique && len(receipt.AssertionSourceRanges) <= receipt.AssertionCount {
		assertion := ExistingTestAssertionDigest(report.TestResults[resultIndex])
		for _, current := range receipt.AssertionSourceRanges {
			if current.AssertionDigest == assertion {
				if span.AssertionDigest != "" {
					span = ExistingTestAssertionSourceRange{}
					break
				}
				span = current
			}
		}
	}
	var out []string
	for _, ref := range observation.ContractRefs {
		var contract WriteBehaviorContract
		count := 0
		for _, current := range plan.BehaviorContracts {
			if current.ID == ref {
				contract = current
				count++
			}
		}
		if count != 1 {
			continue
		}
		if loc, ok := ParseAnswerSourceLocationSurface(contract.EvidenceRef); ok && nativeTestRegistrationFileSHA(plan, loc.File) != "" {
			if loc.File != receipt.TestPath || span.LineStart <= 0 || span.LineEnd < span.LineStart ||
				loc.LineStart < span.LineStart || loc.LineEnd > span.LineEnd {
				continue
			}
		}
		out = append(out, ref)
	}
	return out
}

func nativeTestRegistrationProofScope(plan *ChangePlan, report *ChangeReport) bool {
	if plan != nil && (plan.NativeTestRegistration != nil || plan.PersistenceKind == PlanPersistenceNativeTestRegistration) {
		return true
	}
	if report != nil {
		for _, receipt := range report.ExistingTestExecutions {
			if receipt.NativeTestRegistrationDigest != "" {
				return true
			}
		}
	}
	return false
}

func nativeTestRegistrationFileSHA(plan *ChangePlan, path string) string {
	if plan == nil || plan.NativeTestRegistration == nil {
		return ""
	}
	for _, file := range plan.NativeTestRegistration.Tests {
		if file.Path == path {
			return file.SHA256
		}
	}
	return ""
}

func nativeTestRegistrationFailureRefMatches(plan *ChangePlan, report *ChangeReport, observation ProjectTestObservation, resultIndex int, ref string) bool {
	if !nativeTestRegistrationProofScope(plan, report) {
		return true
	}
	for commandIndex := range report.ExecutedCommands {
		for _, covered := range NativeTestRegistrationContractRefs(plan, report, observation, commandIndex, resultIndex) {
			if ref == covered {
				return true
			}
		}
	}
	return false
}

func nativeTestRegistrationCommandProofMatches(plan *ChangePlan, report *ChangeReport, commandIndex int) bool {
	if !nativeTestRegistrationProofScope(plan, report) {
		return true
	}
	if plan == nil || report == nil {
		return false
	}
	for _, observation := range plan.ProjectTestObservations {
		for resultIndex := range report.TestResults {
			if NativeTestRegistrationAssertionMatches(plan, report, observation, commandIndex, resultIndex) {
				return true
			}
		}
	}
	return false
}

// Rebuild this lane's contract records from current receipts on every read,
// including JSON recovery. Raw success/failure labels remain in the stored
// report, but cannot rebind themselves after a registration or contract change.
func effectiveNativeTestRegistrationConfidence(plan *ChangePlan, report *ChangeReport, records []VerificationConfidenceRecord) []VerificationConfidenceRecord {
	if !nativeTestRegistrationProofScope(plan, report) {
		return records
	}
	out := make([]VerificationConfidenceRecord, 0, len(records)+2)
	for _, record := range records {
		witness, _ := VerificationConfidenceRecordWitnessKind(record)
		if record.Category != "project_test_contract_refs" && witness != WriteBehaviorWitnessProjectTest {
			out = append(out, record)
		}
	}
	if plan == nil {
		return out
	}
	required := RequiredWriteBehaviorContractIDs(ChangePlanVerificationBehaviorContracts(plan), true)
	declared, observed := map[string]bool{}, map[string]bool{}
	for _, observation := range plan.ProjectTestObservations {
		for _, ref := range observation.ContractRefs {
			if _, ok := required[ref]; ok {
				declared[ref] = true
			}
		}
		if report != nil {
			for ri, row := range report.TestResults {
				if !row.Passed || row.Suite != observation.AssertionSuite || row.AssertionID != observation.AssertionID {
					continue
				}
				for ci := range report.ExecutedCommands {
					for _, ref := range NativeTestRegistrationContractRefs(plan, report, observation, ci, ri) {
						if _, ok := required[ref]; ok {
							observed[ref] = true
						}
					}
				}
			}
		}
	}
	for _, status := range []string{"satisfied", "missing"} {
		var refs []string
		for ref := range declared {
			if observed[ref] == (status == "satisfied") {
				refs = append(refs, ref)
			}
		}
		if len(refs) == 0 {
			continue
		}
		record := VerificationConfidenceRecord{Source: "project_test_observation", Category: "project_test_contract_refs", Status: status, Severity: "warning",
			ReasonCode: VerificationProjectTestAssertionNotObservedReasonCode, ContractRefs: dedupVerificationScopeStrings(refs),
			Detail: "Registered native assertion proof requires this plan's fresh file-bound invocation and any explicitly cited test lines to be in that assertion's observed method range."}
		if status == "satisfied" {
			record.Severity, record.ReasonCode, record.WitnessKind = "info", "project_test_contract_ref_observed", WriteBehaviorWitnessProjectTest
		}
		out = append(out, record)
	}
	return out
}
