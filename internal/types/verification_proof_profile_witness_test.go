package types

import (
	"strings"
	"testing"
)

// verification_proof_profile_witness_test.go — V5-1 (colleague_merge_audit
// §40.10): the proof profile and ledger apply the contract-kind →
// witness-kind matrix. A source-text receipt closes a file_layout contract
// only; for a runtime kind it is demoted to an advisory disclosure that never
// resolves the required obligation and never hides it from the uncovered
// count.

func witnessTestReport(planID string, records ...VerificationConfidenceRecord) *ChangeReport {
	return &ChangeReport{
		PlanID: planID, Passed: true, VerificationStatus: VerificationStatusPassed,
		ExecutedCommands: []ExecutedCommand{{Runner: "make", Suite: "test", Outcome: "executed", ExitCode: 0}},
		ChangedPathCoverage: []ChangedPathVerificationCoverage{{
			Path: "main.c", Status: ChangedPathVerificationCovered,
			Caliber: ChangedPathVerificationProjectRunner, Capability: VerificationCapabilityTargetBehavior,
		}},
		VerificationConfidence: records,
	}
}

func witnessTestPlan(kind WriteBehaviorContractKind) *ChangePlan {
	return &ChangePlan{
		ID: "plan-witness-matrix",
		BehaviorContracts: []WriteBehaviorContract{{
			ID: "retries-zero", Kind: kind,
			Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpContains,
			Expected: "0", EvidenceRef: "main.c:2", Required: true,
		}},
	}
}

func TestBuildVerificationProofProfileRejectsSourceWitnessForObservableContract(t *testing.T) {
	// Legacy-shaped record: no WitnessKind stamp, category fallback = source_text.
	legacy := VerificationConfidenceRecord{
		Source: "post_apply_source_observation", Category: "source_contract_refs",
		Status: "satisfied", ReasonCode: "post_apply_source_contract_observed",
		ContractRefs: []string{"retries-zero"},
	}
	plan := witnessTestPlan(WriteBehaviorObservable)
	report := witnessTestReport(plan.ID, legacy)
	profile := BuildVerificationProofProfile(plan, report)
	if profile.Status == VerificationProofStrong || !verificationProofHasReason(profile, "behavior_contract_observation_missing") {
		t.Fatalf("a source reading must not close an observable contract: %+v", profile)
	}
	ledger := BuildVerificationProofLedger(plan, report, nil)
	if ledger.State == VerificationProofLedgerVerified || ledger.UncoveredCount != 1 ||
		!verificationProofLedgerHasItem(ledger, "behavior_contract", VerificationProofLedgerItemMissing, "behavior_contract_observation_missing") {
		t.Fatalf("the required observable obligation must stay missing: %+v", ledger)
	}
	if !verificationProofLedgerHasDisclosure(ledger, "source_text_presence", VerificationProofLedgerItemAdvisory, "source_witness_not_admitted_for_contract_kind") {
		t.Fatalf("the rejected source witness must be disclosed on the ledger's disclosure list, not dropped: %+v", ledger.Disclosures)
	}
	if verificationProofLedgerHasItem(ledger, "source_text_presence", VerificationProofLedgerItemAdvisory, "source_witness_not_admitted_for_contract_kind") {
		t.Fatalf("a disclosure is not an obligation and must not inflate obligation/covered counts: %+v", ledger.Obligations)
	}
	if verificationProofLedgerHasItem(ledger, "behavior_contract", VerificationProofLedgerItemCovered, "post_apply_source_contract_observed") {
		t.Fatalf("the rejected source witness must not appear as a covered behavior contract: %+v", ledger.Obligations)
	}
	// The same receipt closes the same contract when its kind is file_layout.
	plan = witnessTestPlan(WriteBehaviorFileLayout)
	profile = BuildVerificationProofProfile(plan, report)
	if profile.Status != VerificationProofStrong || verificationProofHasReason(profile, "behavior_contract_observation_missing") {
		t.Fatalf("a source reading closes a file_layout contract: %+v", profile)
	}
	ledger = BuildVerificationProofLedger(plan, report, nil)
	if ledger.State != VerificationProofLedgerVerified || ledger.UncoveredCount != 0 {
		t.Fatalf("file_layout source receipt must verify the ledger: %+v", ledger)
	}
	// An executed probe closes the observable contract.
	probe := VerificationConfidenceRecord{
		Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied",
		ReasonCode: "verification_probe_contract_ref_covered", ContractRefs: []string{"retries-zero"},
		WitnessKind: WriteBehaviorWitnessVerificationProbe,
	}
	plan = witnessTestPlan(WriteBehaviorObservable)
	profile = BuildVerificationProofProfile(plan, witnessTestReport(plan.ID, legacy, probe))
	if profile.Status != VerificationProofStrong || verificationProofHasReason(profile, "behavior_contract_observation_missing") {
		t.Fatalf("an executed probe closes the observable contract: %+v", profile)
	}
}

func TestBuildVerificationProofLedgerSourceTextPresenceAdvisoryNeverResolvesOrCountsUncovered(t *testing.T) {
	advisory := VerificationConfidenceRecord{
		Source: "post_apply_source_observation", Category: "source_text_presence",
		Status: "advisory", Severity: "info", ReasonCode: "post_apply_source_text_present",
		ContractRefs: []string{"retries-zero"}, WitnessKind: WriteBehaviorWitnessSourceText,
	}
	plan := witnessTestPlan(WriteBehaviorObservable)
	report := witnessTestReport(plan.ID, advisory)
	ledger := BuildVerificationProofLedger(plan, report, nil)
	if !verificationProofLedgerHasDisclosure(ledger, "source_text_presence", VerificationProofLedgerItemAdvisory, "post_apply_source_text_present") {
		t.Fatalf("advisory disclosure must be a ledger disclosure item: %+v", ledger.Disclosures)
	}
	for _, item := range ledger.Obligations {
		if item.Kind == "source_text_presence" {
			t.Fatalf("advisory disclosure must not be an obligation: %+v", item)
		}
	}
	if !verificationProofLedgerHasItem(ledger, "behavior_contract", VerificationProofLedgerItemMissing, "behavior_contract_observation_missing") ||
		ledger.UncoveredCount != 1 || !verificationProofLedgerHasUnresolvedObligation(&ledger) {
		t.Fatalf("advisory presence must neither resolve nor hide the required obligation: %+v", ledger)
	}
	profile := BuildVerificationProofProfile(plan, report)
	if !verificationProofHasReason(profile, "behavior_contract_observation_missing") {
		t.Fatalf("profile must keep the missing observation: %+v", profile)
	}
	// Cumulative artifacts follow the same matrix.
	legacy := VerificationConfidenceRecord{Category: "source_contract_refs", Status: "satisfied", ContractRefs: []string{"retries-zero"}}
	artifacts := []VerificationProofArtifact{{Plan: plan, Report: witnessTestReport(plan.ID, legacy)}}
	if missing := missingCumulativeRequiredWriteBehaviorContractObservationIDs(artifacts); len(missing) != 1 || missing[0] != "retries-zero" {
		t.Fatalf("cumulative coverage must apply the matrix: %v", missing)
	}
	artifacts[0].Plan = witnessTestPlan(WriteBehaviorFileLayout)
	if missing := missingCumulativeRequiredWriteBehaviorContractObservationIDs(artifacts); len(missing) != 0 {
		t.Fatalf("cumulative file_layout source receipt must cover: %v", missing)
	}
}

// A cumulative artifact whose plan could not be loaded still passes the
// witness matrix against the primary plan: a legacy observable+source receipt
// must not resolve the primary plan's obligation through the nil-plan path.
func TestBuildVerificationProofLedgerNilPlanArtifactStillAppliesWitnessMatrix(t *testing.T) {
	plan := witnessTestPlan(WriteBehaviorObservable)
	legacy := VerificationConfidenceRecord{Source: "post_apply_source_observation", Category: "source_contract_refs", Status: "satisfied", ReasonCode: "post_apply_source_contract_observed", ContractRefs: []string{"retries-zero"}}
	primary := witnessTestReport(plan.ID)
	artifacts := []VerificationProofArtifact{{Plan: nil, Report: witnessTestReport("plan-earlier", legacy)}}
	ledger := BuildVerificationProofLedger(plan, primary, artifacts)
	if !verificationProofLedgerHasItem(ledger, "behavior_contract", VerificationProofLedgerItemMissing, "behavior_contract_observation_missing") || ledger.UncoveredCount == 0 {
		t.Fatalf("nil-plan artifact must not resolve the observable obligation: %+v", ledger)
	}
	if verificationProofLedgerHasItem(ledger, "behavior_contract", VerificationProofLedgerItemCovered, "post_apply_source_contract_observed") {
		t.Fatalf("the legacy receipt must be demoted, not minted as covered: %+v", ledger.Obligations)
	}
}

// An unknown/unnormalized kind (a persisted plan edited by hand) is
// discharged by an executed probe exactly like observable — never a
// permanently unresolvable obligation — and still refuses source text.
func TestBuildVerificationProofProfileUnknownKindAdmitsExecutedWitnesses(t *testing.T) {
	plan := witnessTestPlan("Layout")
	probe := VerificationConfidenceRecord{Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied", ReasonCode: "verification_probe_contract_ref_covered", ContractRefs: []string{"retries-zero"}, WitnessKind: WriteBehaviorWitnessVerificationProbe}
	profile := BuildVerificationProofProfile(plan, witnessTestReport(plan.ID, probe))
	if profile.Status != VerificationProofStrong || verificationProofHasReason(profile, "behavior_contract_observation_missing") {
		t.Fatalf("executed probe must discharge the unknown-kind contract: %+v", profile)
	}
	source := VerificationConfidenceRecord{Source: "post_apply_source_observation", Category: "source_contract_refs", Status: "satisfied", ContractRefs: []string{"retries-zero"}, WitnessKind: WriteBehaviorWitnessSourceText}
	profile = BuildVerificationProofProfile(plan, witnessTestReport(plan.ID, source))
	if !verificationProofHasReason(profile, "behavior_contract_observation_missing") {
		t.Fatalf("source text must not discharge an unknown-kind contract: %+v", profile)
	}
}

func verificationProofLedgerHasDisclosure(ledger VerificationProofLedger, kind string, status VerificationProofLedgerItemStatus, reason string) bool {
	for _, item := range ledger.Disclosures {
		if item.Kind == kind && item.Status == status && item.ReasonCode == reason {
			return true
		}
	}
	return false
}

// Disclosures are bounded separately from the 64-item capability cap, so a
// report crowded with executed-command rows cannot squeeze them out.
func TestVerificationProofLedgerDisclosuresSurviveTheCapabilityCap(t *testing.T) {
	plan := witnessTestPlan(WriteBehaviorObservable)
	report := witnessTestReport(plan.ID, VerificationConfidenceRecord{
		Source: "post_apply_source_observation", Category: "source_text_presence", Status: "advisory",
		ReasonCode: "post_apply_source_text_present", ContractRefs: []string{"retries-zero"}, WitnessKind: WriteBehaviorWitnessSourceText,
	})
	for i := 0; i < 80; i++ {
		report.ExecutedCommands = append(report.ExecutedCommands, ExecutedCommand{Runner: "make", Suite: "suite-" + strings.Repeat("x", i%7) + string(rune('a'+i%26)), Outcome: "executed"})
	}
	ledger := BuildVerificationProofLedger(plan, report, nil)
	if !verificationProofLedgerHasDisclosure(ledger, "source_text_presence", VerificationProofLedgerItemAdvisory, "post_apply_source_text_present") {
		t.Fatalf("disclosure lost under capability pressure: capabilities=%d disclosures=%+v", len(ledger.Capabilities), ledger.Disclosures)
	}
	if ledger.DisclosureCount != 1 || ledger.CapabilityCount == 0 {
		t.Fatalf("counts = disclosures %d capabilities %d", ledger.DisclosureCount, ledger.CapabilityCount)
	}
}

// V5-2: the worktree side-effect advisory is a disclosure, never an obligation.
func TestVerificationProofLedgerWorktreeSideEffectAdvisoryIsADisclosure(t *testing.T) {
	report := witnessTestReport("plan-x", VerificationConfidenceRecord{Source: "git_worktree_audit", Category: "worktree_side_effect", Status: "advisory", Severity: "warning",
		ReasonCode: VerificationTrackedSideEffectDisclosedReason, Detail: "Cargo.lock=dependency_lockfile_refresh"})
	ledger := BuildVerificationProofLedger(nil, report, nil)
	if !verificationProofLedgerHasDisclosure(ledger, "worktree_side_effect", VerificationProofLedgerItemAdvisory, VerificationTrackedSideEffectDisclosedReason) {
		t.Fatalf("worktree disclosure missing from the disclosure list: %+v", ledger)
	}
	for _, item := range ledger.Obligations {
		if item.Kind == "worktree_side_effect" {
			t.Fatalf("worktree disclosure must not be an obligation: %+v", item)
		}
	}
}
