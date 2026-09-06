package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

// write_behavior_contract_retirement_test.go — V5-3 / V5-4 pins
// (colleague_merge_audit §40.23 / §40.24). Red witnesses for every pin were
// run against the untouched HEAD tree (scratch copy) before construction.

func retirementTestContracts() []WriteBehaviorContract {
	return []WriteBehaviorContract{
		{ID: "hard-api", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpEquals, Expected: "public API remains compatible", Required: true, Source: "write_analyzer"},
		{ID: "c-soft-a", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "the rejected implementation shape", Required: true, Source: "write_analyzer"},
		{ID: "c-soft-b", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "unrelated soft expectation", Required: true, Source: "write_analyzer"},
		{ID: "grounded-soft", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "wire layout remains compatible", EvidenceRef: "file:protocol.rs:42", Required: true, Source: "write_analyzer"},
		{ID: "observed-failure", Kind: WriteBehaviorObservable, Polarity: WriteBehaviorPolarityObserved, Operator: WriteBehaviorOpSatisfies, Expected: "the original check failed", Source: "write_analyzer"},
		{ID: "outcome-1", Kind: WriteBehaviorObservable, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "the stale plan shape passes", Required: true, Source: WriteBehaviorContractSourceExpectedOutcomeFallback},
	}
}

func TestBuildVerifyFailureContractRelevanceJoinsFailedProbeAndAssertionRows(t *testing.T) {
	plan := &ChangePlan{
		ID:                 "plan-1",
		VerificationProbes: []VerificationProbe{{ID: "p1", ContractRefs: []string{"c1"}, PlacementRefs: []string{"c1-placement"}}, {ID: "p-passed", ContractRefs: []string{"c-passed"}}},
		ProjectTestObservations: []ProjectTestObservation{
			{ID: "o1", TestPath: "pkg/x_test.go", AssertionSuite: "pkg", AssertionID: "TestX", ContractRefs: []string{"c2"}},
			{ID: "o2", AssertionSuite: "pkg", AssertionID: "TestZ", ContractRefs: []string{"c-aggregate"}},
		},
	}
	report := &ChangeReport{PlanID: "plan-1", TestResults: []TestResult{
		{Suite: "verification_probe/python", AssertionID: "p1", Passed: false},
		{Suite: "verification_probe/python", AssertionID: "p-passed", Passed: true},
		{Suite: "repo/pkg", AssertionID: "TestX", ObservationScope: TestObservationScopeAssertion, Passed: false},
		{Suite: "repo/pkg", AssertionID: "TestY", ObservationScope: TestObservationScopeAssertion, Passed: false},
		{Suite: "repo/pkg", AssertionID: "TestZ", ObservationScope: TestObservationScopeAggregate, Passed: false},
		{Kind: TestResultKindBuildError, Suite: "build", AssertionID: "TestX", Passed: false},
	}}
	// EVOLUTION RECORD: project-test retirement now requires the runner's
	// exact execution/path binding. The old fixture contained names only and
	// could not prove which source file produced the failed assertion.
	got := BuildVerifyFailureContractRelevance(report, plan, ProjectTestFailureBinding{
		ObservationID: "o1", TestPath: "pkg/x_test.go", AssertionSuite: "pkg", AssertionID: "TestX", ResultIndex: 2,
	})
	if got.Status != VerifyFailureContractRelevanceAvailable {
		t.Fatalf("relevance = %+v, want available", got)
	}
	want := []VerifyFailureContractHit{
		{ContractID: "c1", Reason: WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:p1"}},
		{ContractID: "c1-placement", Reason: WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:p1"}},
		{ContractID: "c2", Reason: WriteBehaviorContractRetiredFailedProjectTestAssertion, EvidenceRefs: []string{"assertion:pkg/x_test.go::pkg::TestX"}},
	}
	if !reflect.DeepEqual(got.Hits, want) {
		t.Fatalf("hits = %+v, want %+v", got.Hits, want)
	}
	if mismatch := BuildVerifyFailureContractRelevance(report, &ChangePlan{ID: "plan-2"}); mismatch.Status != VerifyFailureContractRelevanceUnavailable || mismatch.ReasonCode != "plan_report_mismatch" {
		t.Fatalf("plan/report mismatch must be unavailable: %+v", mismatch)
	}
	if nilReport := BuildVerifyFailureContractRelevance(nil, plan); nilReport.Available() {
		t.Fatalf("nil report must be unavailable: %+v", nilReport)
	}
	if nilPlan := BuildVerifyFailureContractRelevance(report, nil); nilPlan.Available() {
		t.Fatalf("nil plan must be unavailable: %+v", nilPlan)
	}
}

func TestRebaseVerifyFailureWriteBehaviorContractsRetiresOnlyRelevantSoftRows(t *testing.T) {
	hits := VerifyFailureContractRelevance{Status: VerifyFailureContractRelevanceAvailable, Hits: []VerifyFailureContractHit{
		{ContractID: "c-soft-a", Reason: WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:p1"}},
		{ContractID: "hard-api", Reason: WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:p1"}},
		{ContractID: "grounded-soft", Reason: WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:p1"}},
	}}
	decision := WriteBehaviorContractRetirementDecision{Lane: FailureKindContractRetireRelevanceSubset, Relevance: hits, PlanID: "plan-1", BatchID: "batch-1", Attempt: 2, FailureKind: FailureKindTestsFailed}
	rebased, tombstones := RebaseVerifyFailureWriteBehaviorContracts(retirementTestContracts(), []string{"the repaired shape passes"}, decision)
	ids := func(in []WriteBehaviorContract) []string {
		out := []string{}
		for _, c := range in {
			out = append(out, c.ID)
		}
		return out
	}
	if got := ids(rebased); !reflect.DeepEqual(got, []string{"hard-api", "c-soft-b", "grounded-soft", "observed-failure", "outcome-1-2"}) {
		t.Fatalf("rebased ids = %v (hard/grounded/observed untouched, sibling soft retained, tombstoned outcome-1 reserved)", got)
	}
	if rebased[1] != retirementTestContracts()[2] {
		t.Fatalf("retained sibling soft row must be byte-identical: %+v", rebased[1])
	}
	if len(tombstones) != 2 || tombstones[0].ID != "c-soft-a" || tombstones[1].ID != "outcome-1" {
		t.Fatalf("tombstones = %+v", tombstones)
	}
	if tombstones[0].Reason != WriteBehaviorContractRetiredFailedVerificationProbe || !reflect.DeepEqual(tombstones[0].EvidenceRefs, []string{"probe:p1"}) ||
		tombstones[0].PlanID != "plan-1" || tombstones[0].Attempt != 2 || tombstones[0].FailureKind != FailureKindTestsFailed {
		t.Fatalf("tombstone lost its evidence / attempt identity: %+v", tombstones[0])
	}
	if tombstones[1].Reason != WriteBehaviorContractRetiredFallbackGenerationRebase || !reflect.DeepEqual(tombstones[1].EvidenceRefs, []string{"plan:plan-1"}) {
		t.Fatalf("fallback tombstone = %+v", tombstones[1])
	}

	// retain_all lane (any non tests_failed kind) with the same hits: no
	// evidence-based tombstone at all.
	retain := decision
	retain.Lane = FailureKindContractRetainAll
	retain.FailureKind = FailureKindBuildFailure
	rebased, tombstones = RebaseVerifyFailureWriteBehaviorContracts(retirementTestContracts(), []string{"the repaired shape passes"}, retain)
	if got := ids(rebased); !reflect.DeepEqual(got, []string{"hard-api", "c-soft-a", "c-soft-b", "grounded-soft", "observed-failure", "outcome-1-2"}) {
		t.Fatalf("retain_all rebased ids = %v", got)
	}
	for _, tombstone := range tombstones {
		if tombstone.Reason != WriteBehaviorContractRetiredFallbackGenerationRebase {
			t.Fatalf("retain_all minted a non-fallback tombstone: %+v", tombstone)
		}
	}

	// planner supersession: a soft id is retired on the planner's word; a
	// hard id never is, regardless of lane.
	planner := WriteBehaviorContractRetirementDecision{Lane: FailureKindContractRetainAll, PlannerSupersededIDs: []string{"c-soft-b", "hard-api", "grounded-soft", "observed-failure"}, PlanID: "plan-1", Attempt: 1, FailureKind: FailureKindBuildFailure}
	rebased, tombstones = RebaseVerifyFailureWriteBehaviorContracts(retirementTestContracts(), nil, planner)
	if got := ids(rebased); !reflect.DeepEqual(got, []string{"hard-api", "c-soft-a", "grounded-soft", "observed-failure"}) {
		t.Fatalf("planner supersession rebased ids = %v", got)
	}
	if len(tombstones) != 2 || tombstones[0].ID != "c-soft-b" || tombstones[0].Reason != WriteBehaviorContractRetiredPlannerSupersession {
		t.Fatalf("planner tombstones = %+v", tombstones)
	}
}

func TestRebaseVerifyFailureDoesNotRemintTombstonedFallbackID(t *testing.T) {
	rebased, tombstones := RebaseVerifyFailureWriteBehaviorContracts([]WriteBehaviorContract{
		{ID: "explicit-api", Kind: WriteBehaviorInvariant, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpEquals, Expected: "public API remains compatible", Required: true, Source: "write_analyzer"},
		{ID: "outcome-2", Kind: WriteBehaviorObservable, Polarity: WriteBehaviorPolarityExpected, Operator: WriteBehaviorOpSatisfies, Expected: "line 16 remains unchanged", Required: true, Source: WriteBehaviorContractSourceExpectedOutcomeFallback},
	}, []string{"line 12 returns the callback error", "line 16 returns the negative lookup error"}, WriteBehaviorContractRetirementDecision{Lane: FailureKindContractRetainAll, PlanID: "plan-1"})
	retired := map[string]bool{}
	for _, tombstone := range tombstones {
		retired[tombstone.ID] = true
	}
	for _, c := range rebased {
		if retired[c.ID] {
			t.Fatalf("rebased generation re-minted tombstoned id %q: %+v", c.ID, rebased)
		}
	}
	plan := &ChangePlan{BehaviorContracts: rebased, SupersededBehaviorContracts: tombstones, BehaviorContractGeneration: WriteBehaviorContractGenerationPlanAcceptanceRebase}
	if got := ChangePlanVerificationBehaviorContracts(plan); len(got) != 3 {
		t.Fatalf("verification view lost a new fallback row: %+v", got)
	}
}

func TestWriteBehaviorContractResolutionLookupThreeStates(t *testing.T) {
	plan := &ChangePlan{
		BehaviorContracts:           []WriteBehaviorContract{{ID: "active"}, {ID: "shadowed"}},
		SupersededBehaviorContracts: []WriteBehaviorContractTombstone{{ID: "shadowed", Reason: WriteBehaviorContractRetiredPlannerSupersession, PlanID: "plan-0", Attempt: 1}},
	}
	res := ResolveChangePlanBehaviorContractIDs(plan)
	if status, _ := res.Lookup("active"); status != WriteBehaviorContractIDActive {
		t.Fatalf("active id status = %s", status)
	}
	status, tombstone := res.Lookup("shadowed")
	if status != WriteBehaviorContractIDRetired || tombstone == nil || tombstone.PlanID != "plan-0" {
		t.Fatalf("retired lookup = %s %+v", status, tombstone)
	}
	if status, _ := res.Lookup("nope"); status != WriteBehaviorContractIDUnknown {
		t.Fatalf("unknown id status = %s", status)
	}
	if got := res.ActiveIDs(); len(got) != 1 {
		t.Fatalf("active ids = %v", got)
	}
	if got := res.RetiredIDs(); !reflect.DeepEqual(got, []string{"shadowed"}) {
		t.Fatalf("retired ids = %v", got)
	}
}

func TestChangePlanTombstoneCarrierRoundTrip(t *testing.T) {
	plan := &ChangePlan{ID: "p", SupersededBehaviorContracts: []WriteBehaviorContractTombstone{{
		ID: "c-soft-a", Reason: WriteBehaviorContractRetiredFailedProjectTestAssertion, EvidenceRefs: []string{"assertion:pkg::TestX"},
		PlanID: "plan-1", BatchID: "batch-1", Attempt: 2, FailureKind: FailureKindTestsFailed,
	}}, SupersededBehaviorContractIDs: []string{"c-soft-a"}}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var back ChangePlan
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.SupersededBehaviorContracts, plan.SupersededBehaviorContracts) {
		t.Fatalf("tombstone carrier lost fields: %+v", back.SupersededBehaviorContracts)
	}
	// Legacy persisted plan: ids only.
	var legacy ChangePlan
	if err := json.Unmarshal([]byte(`{"id":"old","behavior_contracts":[{"id":"stale"},{"id":"kept"}],"superseded_behavior_contract_ids":["stale"]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	res := ResolveChangePlanBehaviorContractIDs(&legacy)
	status, tombstone := res.Lookup("stale")
	if status != WriteBehaviorContractIDRetired || tombstone == nil || tombstone.Reason != "" || len(tombstone.EvidenceRefs) != 0 {
		t.Fatalf("legacy id must resolve retired with empty evidence: %s %+v", status, tombstone)
	}
	if got := ChangePlanVerificationBehaviorContracts(&legacy); len(got) != 1 || got[0].ID != "kept" {
		t.Fatalf("legacy tombstone must still shadow the verification view: %+v", got)
	}
}

func TestEveryFailureKindRegistersAContractRetirementLane(t *testing.T) {
	kinds := collectDeclaredFailureKinds(t)
	if len(kinds) < 12 {
		t.Fatalf("census found only %d FailureKind constants", len(kinds))
	}
	relevance := 0
	for _, kind := range kinds {
		action, registered := FailureKindReplanActions[kind]
		if !registered || action.ContractRetirement == "" {
			t.Fatalf("FailureKind %q registers no contract-retirement lane", kind)
		}
		switch action.ContractRetirement {
		case FailureKindContractRetainAll:
		case FailureKindContractRetireRelevanceSubset:
			relevance++
			if !FailureReasonCodeIndicatesCodeFailure(string(kind)) {
				t.Fatalf("FailureKind %q opens the relevance subset without being a code failure", kind)
			}
		default:
			t.Fatalf("FailureKind %q has unknown retirement lane %q", kind, action.ContractRetirement)
		}
	}
	if relevance != 1 || ContractRetirementLaneForFailureKind(FailureKindTestsFailed) != FailureKindContractRetireRelevanceSubset {
		t.Fatalf("exactly tests_failed may open the relevance subset (got %d)", relevance)
	}
	if ContractRetirementLaneForFailureKind("") != FailureKindContractRetainAll || ContractRetirementLaneForFailureKind("made_up") != FailureKindContractRetainAll {
		t.Fatal("empty / unregistered kinds must fail closed toward retention")
	}
}
