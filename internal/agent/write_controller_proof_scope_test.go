package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1634ControllerProofContext(t *testing.T, confidence []types.VerificationConfidenceRecord) *types.AgentContext {
	t.Helper()
	mut := types.NewMutableState("fix only the source typo")
	mut.SetChangePlan(&types.ChangePlan{
		ID: "plan-proof-scope", Status: types.PlanStatusApplied,
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID: "line25-correct", Kind: types.WriteBehaviorFileLayout,
			Operator: types.WriteBehaviorOpEquals, Expected: "return", Required: true,
			Polarity: types.WriteBehaviorPolarityExpected,
			Placement: &types.WriteRenderedTextPlacement{Surface: types.WriteRenderedTextSurfaceRepr,
				Anchor: "retrun", Expected: "return", Relation: types.WriteRenderedTextLineLocalNotContains},
		}},
	})
	mut.SetChangeReport(&types.ChangeReport{
		PlanID: "plan-proof-scope", Channel: types.ChangeReportChannelPostApplyVerify,
		Passed: true, VerificationStatus: types.VerificationStatusPassed,
		TestResults:            []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "TestGreet", Passed: true}},
		VerificationConfidence: confidence,
	})
	return &types.AgentContext{Mutable: mut, Mode: types.ModeApply}
}

func b1634ControllerProofConfidence() []types.VerificationConfidenceRecord {
	return []types.VerificationConfidenceRecord{{
		Source: "verification_probe", Category: "probe_contract_refs", Status: "satisfied",
		ReasonCode: "verification_probe_contract_ref_covered", ContractRefs: []string{"line25-correct"},
		WitnessKind: types.WriteBehaviorWitnessVerificationProbe,
	}, {
		Source: "verification_probe", Category: "probe_placement_refs", Status: "missing",
		ReasonCode: "verification_probe_missing_required_placement_ref", ContractRefs: []string{"line25-correct"},
		Detail: "passed verification probes did not bind every required rendered-text placement contract",
	}}
}

func TestB1634ControllerProofScopeSurvivesContextPackCap(t *testing.T) {
	ctx := b1634ControllerProofContext(t, b1634ControllerProofConfidence())
	pack := &types.WriteContextPack{PackID: "crowded"}
	for i := 0; i < 24; i++ {
		pack.Items = append(pack.Items, types.WriteContextItem{
			ID: fmt.Sprintf("constraint-%02d", i), Kind: "constraint", Priority: types.WriteContextP0,
			Text: fmt.Sprintf("retained constraint %02d", i),
		})
	}
	ctx.Mutable.SetWriteContextPack(pack)
	beforePlan, _ := json.Marshal(ctx.Mutable.ChangePlan())
	beforeReport, _ := json.Marshal(ctx.Mutable.ChangeReport())
	beforePack, _ := json.Marshal(ctx.Mutable.WriteContextPack())
	got := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"verification_behavior_witness_scope: required_typed_contracts=1 covered_required_typed_contracts=1",
		"not complete proof or workflow completion",
		"verification_proof_scope: source=current_plan_report",
		"kind=rendered_text_placement_contract", "status=missing", "category=probe_placement_refs",
		"contract_ref=\"line25-correct\"", "reason_code=verification_probe_missing_required_placement_ref",
		"passed verification probes did not bind every required rendered-text placement contract",
		"... +8 more context item(s)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("controller lost exact proof scope %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "verification_completion_scope:") {
		t.Error("behavior witness coverage still masquerades as complete proof coverage")
	}
	if at := strings.Index(got, "kind=rendered_text_placement_contract"); at < 0 || at > strings.Index(got, "## Priority write context pack") {
		t.Error("unclosed proof must be visible before the independently capped context pack")
	}
	afterPlan, _ := json.Marshal(ctx.Mutable.ChangePlan())
	afterReport, _ := json.Marshal(ctx.Mutable.ChangeReport())
	afterPack, _ := json.Marshal(ctx.Mutable.WriteContextPack())
	if string(beforePlan) != string(afterPlan) || string(beforeReport) != string(afterReport) || string(beforePack) != string(afterPack) {
		t.Fatal("prompt rendering changed an authoritative artifact")
	}
	if next := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil); got != next {
		t.Fatal("proof prompt is not idempotent")
	}
}

func TestB1634ControllerProofReportSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*types.AgentContext)
		want bool
	}{
		{"no_report", func(ctx *types.AgentContext) { ctx.Mutable.SetChangeReport(nil) }, false},
		{"different_plan", func(ctx *types.AgentContext) { ctx.Mutable.ChangeReport().PlanID = "other-plan" }, false},
		{"non_verifier_channel", func(ctx *types.AgentContext) { ctx.Mutable.ChangeReport().Channel = "planner" }, false},
		{"legacy_report", func(ctx *types.AgentContext) {
			ctx.Mutable.ChangeReport().PlanID = ""
			ctx.Mutable.ChangeReport().Channel = ""
		}, true},
		{"report_without_plan", func(ctx *types.AgentContext) { ctx.Mutable.SetChangePlan(nil) }, true},
		{"active_batch_mismatch_without_plan", func(ctx *types.AgentContext) {
			ctx.Mutable.SetChangePlan(nil)
			ctx.Mutable.SetWriteWorkflowRun(&types.WriteWorkflowRun{ActiveBatchID: "active", Batches: []types.WriteWorkflowBatch{{
				ID: "active", PlanID: "other-plan",
			}}})
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := b1634ControllerProofContext(t, b1634ControllerProofConfidence())
			tc.edit(ctx)
			got := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil)
			if strings.Contains(got, "verification_proof_scope:") != tc.want {
				t.Fatalf("current report qualification changed, wanted scope=%v:\n%s", tc.want, got)
			}
			if tc.want && !strings.Contains(got, "reason_code=verification_probe_missing_required_placement_ref") {
				t.Fatalf("qualified legacy/current report lost its exact unresolved evidence:\n%s", got)
			}
		})
	}
}

func TestB1634ControllerProofCoveredIsNotCumulativeCompletion(t *testing.T) {
	confidence := b1634ControllerProofConfidence()
	confidence[1].Status = "satisfied"
	confidence[1].ReasonCode = "verification_probe_placement_ref_covered"
	ctx := b1634ControllerProofContext(t, confidence)
	ctx.Mutable.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		ActiveBatchID: "current", Batches: []types.WriteWorkflowBatch{{
			ID: "earlier", Status: types.WriteWorkflowBatchComplete,
			Goal: "earlier independent proof remains unknown",
		}, {ID: "current", PlanID: "plan-proof-scope", Status: types.WriteWorkflowBatchComplete}},
	})
	got := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"unresolved_items=0 displayed_items=0",
		"not the cumulative workflow verdict",
		"zero local unresolved items do not establish that earlier batches or unavailable proof are resolved",
		"all_verified requires every applied batch to pass its latest verification and every required typed proof obligation to be closed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("local success acquired workflow authority or lost its scope %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "verification_proof_unresolved:") {
		t.Fatalf("covered records must not be republished as unresolved:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if !strings.HasPrefix(line, "- verification_witness_boundary:") {
			continue
		}
		for _, forbidden := range []string{"durable workflow proof verdict", "final_report", "final artifact", "final verdict"} {
			if strings.Contains(line, forbidden) {
				t.Fatalf("completion teaching requires an artifact/verdict that may only exist after finish: %s", line)
			}
		}
	}
}

func TestB1634ControllerProofBoundedDeduplicatedAndExplicitlyClipped(t *testing.T) {
	confidence := b1634ControllerProofConfidence()[:1]
	for i := 0; i < 12; i++ {
		row := types.VerificationConfidenceRecord{
			Source: "verification_probe", Category: "probe_placement_refs", Status: "missing",
			ReasonCode:   "verification_probe_missing_required_placement_ref",
			ContractRefs: []string{fmt.Sprintf("placement-%02d", i)}, Detail: "bounded missing placement detail",
		}
		confidence = append(confidence, row, row) // Real ledger normalization, not renderer fuzzy dedup.
	}
	ctx := b1634ControllerProofContext(t, confidence)
	got := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"unresolved_items=12 displayed_items=8", "... +4 more current-plan/report item(s); omitted items are not resolved",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing exact bounded-ledger disclosure %q:\n%s", want, got)
		}
	}
	if count := strings.Count(got, "verification_proof_unresolved: axis="); count != 8 {
		t.Fatalf("expected exactly 8 unique visible typed rows, got %d:\n%s", count, got)
	}
	for i := 0; i < 8; i++ {
		if count := strings.Count(got, fmt.Sprintf("contract_ref=\"placement-%02d\"", i)); count != 1 {
			t.Fatalf("exact duplicate/row lost at %d count=%d:\n%s", i, count, got)
		}
	}
	long := b1634ControllerProofConfidence()
	long[1].Detail = strings.Repeat("长", 1000)
	long[1].ContractRefs = []string{strings.Repeat("x", 1000)}
	ctx = b1634ControllerProofContext(t, long)
	got = (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"scalar_truncation=ellipsis", "contract_ref=\"" + strings.Repeat("x", 120) + "...\"",
		"detail=\"" + strings.Repeat("长", 200) + "...\"",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("long scalar not explicitly bounded %q:\n%s", want, got)
		}
	}
	if len(ctx.Mutable.ChangeReport().VerificationConfidence[1].Detail) != len(strings.Repeat("长", 1000)) {
		t.Fatal("display clipping truncated the original proof")
	}
}

func TestB1634ControllerProofPreservesUnresolvedStatusDomains(t *testing.T) {
	for _, status := range []string{"missing", "unverified", "unavailable", "failed", "unknown", "future_status"} {
		t.Run(status, func(t *testing.T) {
			confidence := b1634ControllerProofConfidence()
			confidence[1].Status = status
			ctx := b1634ControllerProofContext(t, confidence)
			before, _ := json.Marshal(types.BuildVerificationProofLedger(ctx.Mutable.ChangePlan(), ctx.Mutable.ChangeReport(), nil))
			got := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil)
			wantStatus := status
			if status == "future_status" {
				wantStatus = "unknown" // Existing ledger normalization owns this decision.
			}
			if !strings.Contains(got, "kind=rendered_text_placement_contract status="+wantStatus) {
				t.Fatalf("typed unresolved status %q lost or upgraded:\n%s", status, got)
			}
			after, _ := json.Marshal(types.BuildVerificationProofLedger(ctx.Mutable.ChangePlan(), ctx.Mutable.ChangeReport(), nil))
			if string(before) != string(after) {
				t.Fatal("controller display changed the proof ledger verdict")
			}
		})
	}
}

func TestB1634ControllerProofRetainsUnavailableExecutionCapability(t *testing.T) {
	ctx := b1634ControllerProofContext(t, b1634ControllerProofConfidence()[:1])
	ctx.Mutable.ChangeReport().Passed = false
	ctx.Mutable.ChangeReport().FailureKind = types.FailureKindRunnerMissing
	ctx.Mutable.ChangeReport().FailureSummary = "native runner unavailable"
	got := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(got, "axis=capability kind=local_verification status=unavailable") || !strings.Contains(got, "native runner unavailable") {
		t.Fatalf("execution capability failure was hidden behind witness coverage:\n%s", got)
	}
}
