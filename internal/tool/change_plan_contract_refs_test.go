package tool

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// change_plan_contract_refs_test.go — V5-4 pins (colleague_merge_audit
// §40.24) plus the V5-3 planner-supersession lane. Red witnesses were run
// against an untouched HEAD copy before construction (see ledger §40.46).

func contractRefsTestIR() *types.WriteAnalysisIR {
	return &types.WriteAnalysisIR{Request: types.WriteRequestModel{BehaviorContracts: []types.WriteBehaviorContract{
		{ID: "hard-api", Kind: types.WriteBehaviorInvariant, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpEquals, Expected: "public API remains compatible", Required: true, Source: "write_analyzer"},
		{ID: "stale-soft", Kind: types.WriteBehaviorInvariant, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpSatisfies, Expected: "the rejected implementation shape", Required: true, Source: "write_analyzer"},
		{ID: "grounded-soft", Kind: types.WriteBehaviorInvariant, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpSatisfies, Expected: "wire layout remains compatible", EvidenceRef: "file:protocol.rs:42", Required: true, Source: "write_analyzer"},
		{ID: "observed-failure", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityObserved, Operator: types.WriteBehaviorOpSatisfies, Expected: "the original check failed", Source: "write_analyzer"},
	}}}
}

func testsFailedHandoffRetiring(id string) *types.VerifyFailureHandoff {
	return &types.VerifyFailureHandoff{
		PlanID: "plan-before-failure", BatchID: "batch-1", Attempt: 1, FailureKind: types.FailureKindTestsFailed,
		ContractRelevance: &types.VerifyFailureContractRelevance{Status: types.VerifyFailureContractRelevanceAvailable, Hits: []types.VerifyFailureContractHit{{
			ContractID: id, Reason: types.WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:shape_probe"},
		}}},
	}
}

func TestSiblingContractRefsGatesAgreeAfterRetirement(t *testing.T) {
	ctx := newTestBusCtx()
	ctx.Mutable.SetWriteAnalysisIR(contractRefsTestIR())
	ctx.Mutable.SetVerifyFailureHandoff(testsFailedHandoffRetiring("stale-soft"))
	plan := &types.ChangePlan{AcceptanceTests: []string{"the repaired shape passes"}}
	if rej, _ := attachWriteBehaviorContracts(ctx, plan, nil); rej != "" {
		t.Fatal(rej)
	}
	fallbackID := ""
	for _, c := range plan.BehaviorContracts {
		if types.IsExpectedOutcomeFallbackWriteBehaviorContract(c) {
			fallbackID = c.ID
		}
	}
	if fallbackID == "" {
		t.Fatalf("plan minted no fallback row: %+v", plan.BehaviorContracts)
	}
	judge := func(ref string) (probe, project string) {
		p := *plan
		p.VerificationProbes = []types.VerificationProbe{{ID: "p", ContractRefs: []string{ref}}}
		p.ProjectTestObservations = nil
		probe, _, _ = validatePlanBehaviorContractRefs(&p)
		p.VerificationProbes = nil
		p.ProjectTestObservations = []types.ProjectTestObservation{{ID: "o", ContractRefs: []string{ref}}}
		project, _, _ = validatePlanBehaviorContractRefs(&p)
		return probe, project
	}
	probe, project := judge("stale-soft")
	for _, rej := range []string{probe, project} {
		if !strings.Contains(rej, `behavior_contract id "stale-soft" which was retired after verification attempt 1 of plan plan-before-failure failed (tests_failed): failed verification probe probe:shape_probe`) {
			t.Fatalf("retired id must be rejected with the retired wording in both gates:\nprobe=%q\nproject=%q", probe, project)
		}
		if strings.Contains(rej, "unknown") || strings.Contains(rej, "stale-soft]") || strings.Contains(rej, "stale-soft,") {
			t.Fatalf("retired id must not be called unknown nor offered as valid: %q", rej)
		}
	}
	if probe, project = judge(fallbackID); probe != "" || project != "" {
		t.Fatalf("plan-minted fallback id must be accepted by both gates: probe=%q project=%q", probe, project)
	}
	if probe, project = judge("hard-api"); probe != "" || project != "" {
		t.Fatalf("active id rejected: probe=%q project=%q", probe, project)
	}
	probe, project = judge("nope")
	for _, rej := range []string{probe, project} {
		if !strings.Contains(rej, `unknown behavior_contract id "nope"; use one of [`) || strings.Contains(rej, "stale-soft") {
			t.Fatalf("unknown id must keep the unknown wording and never list the retired id: %q", rej)
		}
	}
}

func TestRetiredContractRefRepairPackIsPrecise(t *testing.T) {
	ctx := newTestBusCtx()
	ctx.Mutable.SetWriteAnalysisIR(contractRefsTestIR())
	ctx.Mutable.SetVerifyFailureHandoff(testsFailedHandoffRetiring("stale-soft"))
	plan := &types.ChangePlan{VerificationProbes: []types.VerificationProbe{{ID: "p", ContractRefs: []string{"stale-soft"}}}}
	if rej, _ := attachWriteBehaviorContracts(ctx, plan, nil); rej != "" {
		t.Fatal(rej)
	}
	rej, reason, fields := validatePlanBehaviorContractRefs(plan)
	if reason != "verification_probe_contract_ref_retired" {
		t.Fatalf("reason = %q, want verification_probe_contract_ref_retired (rej=%q)", reason, rej)
	}
	if !strings.Contains(rej, "attempt 1") || !strings.Contains(rej, "plan-before-failure") || !strings.Contains(rej, "tests_failed") || strings.Contains(rej, "unknown") {
		t.Fatalf("retired rejection must name attempt/plan/failure kind and never say unknown: %q", rej)
	}
	if !reflect.DeepEqual(fields, []string{"$.verification_probes[].contract_refs", "$.changes[].verification_probes[].contract_refs"}) {
		t.Fatalf("fields = %v", fields)
	}
	plan.VerificationProbes = nil
	plan.ProjectTestObservations = []types.ProjectTestObservation{{ID: "o", ContractRefs: []string{"stale-soft"}}}
	if _, reason, _ := validatePlanBehaviorContractRefs(plan); reason != "project_test_observation_contract_ref_retired" {
		t.Fatalf("project reason = %q", reason)
	}
	// Placement refs keep their two-step wording: retired/unknown first, then
	// "without placement{}".
	plan.ProjectTestObservations = nil
	plan.VerificationProbes = []types.VerificationProbe{{ID: "p", PlacementRefs: []string{"hard-api"}}}
	if rej, _, _ := validatePlanBehaviorContractRefs(plan); !strings.Contains(rej, "without placement{}") {
		t.Fatalf("placement wording lost: %q", rej)
	}
}

func TestAttachWriteBehaviorContractsIsTheProjection(t *testing.T) {
	for _, handoff := range []*types.VerifyFailureHandoff{nil, testsFailedHandoffRetiring("stale-soft")} {
		ctx := newTestBusCtx()
		ctx.Mutable.SetWriteAnalysisIR(contractRefsTestIR())
		ctx.Mutable.SetVerifyFailureHandoff(handoff)
		plan := &types.ChangePlan{AcceptanceTests: []string{"the repaired shape passes"}}
		if rej, _ := attachWriteBehaviorContracts(ctx, plan, nil); rej != "" {
			t.Fatal(rej)
		}
		want := types.ProjectWriteBehaviorContractGeneration(contractRefsTestIR().Request.BehaviorContracts, nil, handoff, plan.AcceptanceTests, nil)
		if !reflect.DeepEqual(plan.BehaviorContracts, want.Contracts) || !reflect.DeepEqual(plan.SupersededBehaviorContracts, want.Tombstones) || plan.BehaviorContractGeneration != want.Generation {
			t.Fatalf("attach diverged from the projection (handoff=%v):\nplan=%+v\nwant=%+v", handoff != nil, plan, want)
		}
		if !reflect.DeepEqual(plan.SupersededBehaviorContractIDs, want.RetiredIDs()) {
			t.Fatalf("derived id projection = %v, want %v", plan.SupersededBehaviorContractIDs, want.RetiredIDs())
		}
	}
}

func TestAttachWriteBehaviorContractsPlannerSupersessionOnlySoftIDs(t *testing.T) {
	ctx := newTestBusCtx()
	ctx.Mutable.SetWriteAnalysisIR(contractRefsTestIR())
	ctx.Mutable.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{PlanID: "failed-plan", BatchID: "batch-1", Attempt: 1, FailureKind: types.FailureKindBuildFailure})

	plan := &types.ChangePlan{}
	if rej, _ := attachWriteBehaviorContracts(ctx, plan, []string{"stale-soft"}); rej != "" {
		t.Fatal(rej)
	}
	if len(plan.SupersededBehaviorContracts) != 1 || plan.SupersededBehaviorContracts[0].ID != "stale-soft" || plan.SupersededBehaviorContracts[0].Reason != types.WriteBehaviorContractRetiredPlannerSupersession {
		t.Fatalf("planner supersession of a soft id must mint a planner tombstone: %+v", plan.SupersededBehaviorContracts)
	}
	for _, c := range plan.BehaviorContracts {
		if c.ID == "stale-soft" {
			t.Fatalf("superseded soft id still active: %+v", plan.BehaviorContracts)
		}
	}
	for ref, class := range map[string]string{"hard-api": "hard required", "grounded-soft": "evidence-grounded", "observed-failure": "observed", "ghost": "unknown"} {
		plan := &types.ChangePlan{}
		rej, reason := attachWriteBehaviorContracts(ctx, plan, []string{ref})
		if rej == "" || reason != "superseded_contract_refs_invalid" || !strings.Contains(rej, class) {
			t.Fatalf("ref %q: rej=%q reason=%q (want class %q named)", ref, rej, reason, class)
		}
		if plan.BehaviorContracts != nil || plan.BehaviorContractGeneration != "" {
			t.Fatalf("rejected supersession must leave the plan untouched: %+v", plan)
		}
	}
	// Without a failed verification the lane is closed.
	ctx.Mutable.ResetVerifyFailureHandoff()
	plan = &types.ChangePlan{}
	if rej, reason := attachWriteBehaviorContracts(ctx, plan, []string{"stale-soft"}); rej == "" || reason != "superseded_contract_refs_without_verification_failure" {
		t.Fatalf("supersession without a failed verification must be rejected: rej=%q reason=%q", rej, reason)
	}
}

func TestVerifyFailureRequiredBehaviorContractIDsIgnoreRetiredIRContracts(t *testing.T) {
	mut := types.NewMutableState("repair")
	mut.SetChangePlan(&types.ChangePlan{BehaviorContracts: []types.WriteBehaviorContract{{ID: "hard-api", Required: true, Polarity: types.WriteBehaviorPolarityExpected}}})
	mut.SetWriteAnalysisIR(contractRefsTestIR())
	mut.SetVerifyFailureHandoff(testsFailedHandoffRetiring("stale-soft"))
	got := verifyFailureRequiredBehaviorContractIDs(&types.BusContext{Mutable: mut})
	if _, ok := got["stale-soft"]; ok || len(got) != 1 {
		t.Fatalf("required set must read the live plan only: %v", got)
	}
	// No plan yet (replan round): the projection through the handoff still
	// excludes the retired id while keeping the retained soft ones.
	mut.ResetChangePlan()
	got = verifyFailureRequiredBehaviorContractIDs(&types.BusContext{Mutable: mut})
	if _, ok := got["stale-soft"]; ok {
		t.Fatalf("projection advertised a retired id: %v", got)
	}
	if _, ok := got["grounded-soft"]; !ok {
		t.Fatalf("retained soft contract missing from the projection: %v", got)
	}
}

func TestEmitChangePlanRejectsRetiredContractRefBeforeContentValidation(t *testing.T) {
	ctx := newTestBusCtx()
	ctx.Mutable.SetWriteAnalysisIR(contractRefsTestIR())
	ctx.Mutable.SetVerifyFailureHandoff(testsFailedHandoffRetiring("stale-soft"))
	res, err := (&EmitChangePlan{}).Execute(ctx, []byte(`{
		"request": "repair",
		"summary": "Modify widget.py and bind a probe to a contract that the failed verification retired.",
		"changes": [{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}],
		"verification_probes": [{"id": "shape_probe", "language": "python", "code": "import widget\nassert widget.VALUE == 42\n", "contract_refs": ["stale-soft"]}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || !strings.Contains(res.Summary, `"stale-soft" which was retired`) {
		t.Fatalf("retired ref accepted or worded as unknown: %+v", res)
	}
	if pack := mustPlanRepairPack(t, res); pack.ReasonCode != "verification_probe_contract_ref_retired" {
		t.Fatalf("repair pack = %+v", pack)
	}
}

func TestEmitChangePlanSupersededContractRefsLaneIsTaughtFromOneSource(t *testing.T) {
	for _, schema := range []string{string((&EmitChangePlan{}).Parameters()), string((&EmitPlanSkeleton{}).Parameters())} {
		if !strings.Contains(schema, `"superseded_contract_refs"`) || strings.Contains(schema, "__SUPERSEDED_CONTRACT_REFS_DESCRIPTION__") {
			t.Fatalf("schema must carry the superseded_contract_refs field with its injected description: %s", schema[:200])
		}
		if !strings.Contains(schema, "hard, evidence-grounded, or observed contract cannot be superseded") {
			t.Fatal("schema description must be the single-source teaching sentence")
		}
	}
	for _, reminder := range []string{emitChangePlanSchemaReminder, emitPlanSkeletonSchemaReminder} {
		if !strings.Contains(reminder, "superseded_contract_refs") {
			t.Fatalf("schema reminder must name the lane: %s", reminder)
		}
	}
}

// ---- §40.46 fold-in pins (C3/C0 monotonic retirement, C2/C5 planning-only
// supersession). Red witnesses: scratch copy of the staged tree, see ledger.

func buildFailureHandoffFor(planID string, attempt int) *types.VerifyFailureHandoff {
	return &types.VerifyFailureHandoff{PlanID: planID, BatchID: "batch-1", Attempt: attempt, FailureKind: types.FailureKindBuildFailure,
		ContractRelevance: &types.VerifyFailureContractRelevance{Status: types.VerifyFailureContractRelevanceAvailable, ReasonCode: "typed_failed_rows_joined"}}
}

func TestAttachWriteBehaviorContractsRetirementIsMonotonicAcrossRounds(t *testing.T) {
	for _, round2 := range []*types.VerifyFailureHandoff{
		buildFailureHandoffFor("plan-2", 2),
		{PlanID: "plan-2", BatchID: "batch-1", Attempt: 2, FailureKind: types.FailureKindTestsFailed,
			ContractRelevance: &types.VerifyFailureContractRelevance{Status: types.VerifyFailureContractRelevanceAvailable, Hits: []types.VerifyFailureContractHit{{ContractID: "grounded-soft", Reason: types.WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:p2"}}}}},
	} {
		ctx := newTestBusCtx()
		ctx.Mutable.SetWriteAnalysisIR(contractRefsTestIR())
		// Round 1: tests_failed with a probe hit on stale-soft → plan-2 retires it.
		ctx.Mutable.SetVerifyFailureHandoff(testsFailedHandoffRetiring("stale-soft"))
		plan2 := &types.ChangePlan{ID: "plan-2", AcceptanceTests: []string{"the repaired shape passes"}}
		if rej, _ := attachWriteBehaviorContracts(ctx, plan2, nil); rej != "" {
			t.Fatal(rej)
		}
		if status, _ := resolveBehaviorContractIDs(plan2).Lookup("stale-soft"); status != types.WriteBehaviorContractIDRetired {
			t.Fatalf("round 1 did not retire: %+v", plan2)
		}
		ctx.Mutable.SetChangePlan(plan2)
		// Round 2: plan-2 fails for an unrelated reason; the scheduler replaces
		// the carrier and resets the planning state.
		ctx.Mutable.SetVerifyFailureHandoff(round2)
		ctx.Mutable.ResetChangePlan()
		if _, ok := verifyFailureRequiredBehaviorContractIDs(ctx)["stale-soft"]; ok {
			t.Fatalf("round 2 (%s, no plan) advertised the retired id as required", round2.FailureKind)
		}
		plan3 := &types.ChangePlan{ID: "plan-3", AcceptanceTests: []string{"repaired again"}, VerificationProbes: []types.VerificationProbe{{ID: "p3", ContractRefs: []string{"stale-soft"}}}}
		if rej, _ := attachWriteBehaviorContracts(ctx, plan3, nil); rej != "" {
			t.Fatal(rej)
		}
		status, tombstone := resolveBehaviorContractIDs(plan3).Lookup("stale-soft")
		if status != types.WriteBehaviorContractIDRetired || tombstone == nil || tombstone.PlanID != "plan-before-failure" || tombstone.Attempt != 1 {
			t.Fatalf("round 2 (%s) reinstated stale-soft or lost its round-1 evidence: %+v", round2.FailureKind, plan3)
		}
		rej, reason, _ := validatePlanBehaviorContractRefs(plan3)
		if reason != verificationProbeContractRefRetiredReason || !strings.Contains(rej, "attempt 1 of plan plan-before-failure failed (tests_failed): failed verification probe probe:shape_probe") {
			t.Fatalf("round-2 refs gate must reject with the round-1 evidence: %q (%s)", rej, reason)
		}
		ctx.Mutable.SetChangePlan(plan3)
		if _, ok := verifyFailureRequiredBehaviorContractIDs(ctx)["stale-soft"]; ok {
			t.Fatalf("round 2 (live plan) advertised the retired id as required")
		}
		if round2.FailureKind == types.FailureKindTestsFailed {
			if status, _ := resolveBehaviorContractIDs(plan3).Lookup("grounded-soft"); status != types.WriteBehaviorContractIDActive {
				t.Fatalf("an evidence-grounded contract must never be retired by a hit: %+v", plan3)
			}
		}
		// Green verify / batch switch clears the carrier; the ledger stays.
		ctx.Mutable.ResetVerifyFailureHandoff()
		ctx.Mutable.ResetChangePlan()
		plan4 := &types.ChangePlan{ID: "plan-4"}
		if rej, _ := attachWriteBehaviorContracts(ctx, plan4, nil); rej != "" {
			t.Fatal(rej)
		}
		if status, _ := resolveBehaviorContractIDs(plan4).Lookup("stale-soft"); status != types.WriteBehaviorContractIDRetired {
			t.Fatalf("clearing the handoff reinstated stale-soft: %+v", plan4)
		}
		// Only a new analyzer IR reinstates.
		ctx.Mutable.ResetBehaviorContractTombstoneLedger()
		plan5 := &types.ChangePlan{ID: "plan-5"}
		attachWriteBehaviorContracts(ctx, plan5, nil)
		if status, _ := resolveBehaviorContractIDs(plan5).Lookup("stale-soft"); status != types.WriteBehaviorContractIDActive {
			t.Fatalf("new-IR lane must reinstate: %+v", plan5)
		}
	}
}

func TestAttachWriteBehaviorContractsPlannerSupersessionSurvivesNextRound(t *testing.T) {
	ctx := newTestBusCtx()
	ctx.Mutable.SetWriteAnalysisIR(contractRefsTestIR())
	ctx.Mutable.SetVerifyFailureHandoff(buildFailureHandoffFor("plan-1", 1))
	plan2 := &types.ChangePlan{ID: "plan-2"}
	if rej, _ := attachWriteBehaviorContracts(ctx, plan2, []string{"stale-soft"}); rej != "" {
		t.Fatal(rej)
	}
	ctx.Mutable.SetChangePlan(plan2)
	ctx.Mutable.SetVerifyFailureHandoff(buildFailureHandoffFor("plan-2", 2))
	ctx.Mutable.ResetChangePlan()
	plan3 := &types.ChangePlan{ID: "plan-3"}
	if rej, _ := attachWriteBehaviorContracts(ctx, plan3, nil); rej != "" {
		t.Fatal(rej)
	}
	status, tombstone := resolveBehaviorContractIDs(plan3).Lookup("stale-soft")
	if status != types.WriteBehaviorContractIDRetired || tombstone.Reason != types.WriteBehaviorContractRetiredPlannerSupersession || tombstone.PlanID != "plan-1" {
		t.Fatalf("round-1 planner supersession evaporated on round 2: %+v", plan3)
	}
	// Re-declaring an already-retired id is an accepted no-op.
	if rej, _ := attachWriteBehaviorContracts(ctx, &types.ChangePlan{ID: "plan-3b"}, []string{"stale-soft"}); rej != "" {
		t.Fatalf("re-declaration of a retired id must be accepted: %s", rej)
	}
}

// TestPlannerSupersessionOfPlanningOnlyRowIsTombstoned — §40.46 C2/C5: the
// quality-repaired planning-only row (Required=false,
// quality_repaired:planning_only_ungrounded) is exactly the class the teaching
// names; accepting it must mint a tombstone with evidence, never a silent
// no-op, and the id must then be rejected by the refs gate.
func TestPlannerSupersessionOfPlanningOnlyRowIsTombstoned(t *testing.T) {
	ir := contractRefsTestIR()
	ir.Request.BehaviorContracts = append(ir.Request.BehaviorContracts, types.WriteBehaviorContract{
		ID: "planning-only", Kind: types.WriteBehaviorInvariant, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpSatisfies,
		Expected: "quality-repaired planning prose", Required: false, Source: "write_analyzer;" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded,
	})
	for _, handoff := range []*types.VerifyFailureHandoff{buildFailureHandoffFor("plan-1", 1), testsFailedHandoffRetiring("grounded-soft")} {
		ctx := newTestBusCtx()
		ctx.Mutable.SetWriteAnalysisIR(ir)
		ctx.Mutable.SetVerifyFailureHandoff(handoff)
		plan := &types.ChangePlan{ID: "plan-2"}
		if rej, _ := attachWriteBehaviorContracts(ctx, plan, []string{"planning-only"}); rej != "" {
			t.Fatalf("planning-only row must be supersedable: %s", rej)
		}
		status, tombstone := resolveBehaviorContractIDs(plan).Lookup("planning-only")
		if status != types.WriteBehaviorContractIDRetired || tombstone == nil {
			t.Fatalf("accepted declaration was a silent no-op (%s): %+v", handoff.FailureKind, plan)
		}
		if tombstone.Reason != types.WriteBehaviorContractRetiredPlannerSupersession || tombstone.Attempt != handoff.Attempt || tombstone.PlanID != handoff.PlanID ||
			!reflect.DeepEqual(tombstone.EvidenceRefs, []string{"plan:" + handoff.PlanID}) {
			t.Fatalf("planner tombstone lacks evidence/attempt: %+v", tombstone)
		}
		plan.VerificationProbes = []types.VerificationProbe{{ID: "p", ContractRefs: []string{"planning-only"}}}
		if rej, reason, _ := validatePlanBehaviorContractRefs(plan); reason != verificationProbeContractRefRetiredReason || !strings.Contains(rej, "was superseded by the repair plan") {
			t.Fatalf("superseded planning-only id still referenceable: %q (%s)", rej, reason)
		}
		// The ledger and the plan agree, and the merged pack shows it retired.
		if ids := ctx.Mutable.BehaviorContractTombstoneLedger(); len(ids) == 0 || ids[len(ids)-1].ID != "planning-only" && ids[0].ID != "planning-only" {
			found := false
			for _, row := range ids {
				found = found || row.ID == "planning-only"
			}
			if !found {
				t.Fatalf("ledger missing the accepted supersession: %+v", ids)
			}
		}
	}
	// The predicate the validator uses IS the predicate the rebase uses.
	ctx := newTestBusCtx()
	ctx.Mutable.SetWriteAnalysisIR(ir)
	ctx.Mutable.SetVerifyFailureHandoff(buildFailureHandoffFor("plan-1", 1))
	for _, id := range []string{"hard-api", "grounded-soft", "observed-failure"} {
		rej, reason := attachWriteBehaviorContracts(ctx, &types.ChangePlan{}, []string{id})
		if reason != plannerSupersededContractRefsInvalidReason || !strings.Contains(rej, "cannot be superseded by the planner") {
			t.Fatalf("%s must be refused: %q (%s)", id, rej, reason)
		}
	}
}
