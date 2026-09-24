package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativeTestRegistrationPublicRetainsOriginalExistingTestRequirement(t *testing.T) {
	const second = "test_required.py"
	for _, readRequired := range []bool{false, true} {
		name := "missing_second_read"
		if readRequired {
			name = "fresh_both"
		}
		t.Run(name, func(t *testing.T) {
			ctx, source, delivery := nativeRegistrationPublicFixtureForTestPath(t, nativeRegistrationTestBody, "test_widget.py", map[string]string{second: nativeRegistrationTestBody})
			nativeRegistrationPublicRead(t, ctx, true, 100)
			if readRequired {
				nativeRegistrationPublicReadPath(t, ctx, second, true, 100)
			}
			before, _ := json.Marshal(ctx.Mutable.ChangePlan())
			result := nativeRegistrationPublicEmit(t, ctx, "full", nativeRegistrationPublicPayload())
			if !readRequired {
				after, _ := json.Marshal(ctx.Mutable.ChangePlan())
				if result.Success || string(before) != string(after) {
					t.Fatalf("unread required test admitted or old source changed: %+v", result)
				}
				return
			}
			if !result.Success {
				t.Fatal(result.Summary)
			}
			plan := ctx.Mutable.ChangePlan()
			required := types.RequiredExistingTestPaths(plan)
			if len(required) != 1 || required[0] != second || len(types.RegisteredNativeTestPaths(plan)) != 2 {
				t.Fatalf("original intent lost or registration invented more requirements: %+v", plan.WriteAnalysisIR)
			}
			nativeRegistrationPublicAuthorizeExecution(t, ctx, delivery, source.BehaviorContracts)
			report := existingTestDeliveryPublicRun(t, ctx)
			seen := map[string]bool{}
			for _, receipt := range report.ExistingTestExecutions {
				seen[receipt.TestPath] = true
			}
			if len(seen) != 2 || !seen[second] || !seen["test_widget.py"] {
				t.Fatalf("both existing paths must actually execute: %+v", report.ExistingTestExecutions)
			}
			for _, record := range types.ExistingTestExecutionConfidence(plan, report) {
				if record.Status != "satisfied" {
					t.Fatalf("original execution obligation unresolved: %+v", record)
				}
			}
			if len(types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)) != 1 {
				t.Fatal("fresh mapped assertion lost behavior proof")
			}
			// Weakening the persisted user requirement cannot preserve the seal.
			wire, _ := json.Marshal(plan)
			var restored types.ChangePlan
			if err := json.Unmarshal(wire, &restored); err != nil {
				t.Fatal(err)
			}
			restored.WriteAnalysisIR.Request.Constraints = nil
			if types.NativeTestRegistrationDigest(&restored) != "" {
				t.Fatal("removing the original requirement kept registration valid")
			}
		})
	}
}
