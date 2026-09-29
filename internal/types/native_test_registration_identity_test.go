package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNativeRegistrationIdentityGrantBoundaries(t *testing.T) {
	for _, condition := range []string{"valid", "wrong_token", "wrong_plan", "wrong_channel", "no_invocation", "duplicate_invocation", "unsupported_runner", "framework_base_only", "revoke", "different_run", "different_batch", "different_root", "new_grant", "restore", "report_mutated_after_install", "dispatch_reset"} {
		t.Run(condition, func(t *testing.T) {
			plan, report := nativeRegistrationProofFixture(t)
			delivery := plan.NativeTestRegistration.Delivery
			report.PlanID = delivery.SourcePlanID
			mu := NewMutableState("register existing tests")
			run := &WriteWorkflowRun{RunID: "run", ActiveBatchID: "proof", Status: WriteWorkflowRunInProgress,
				Batches: []WriteWorkflowBatch{{ID: "proof", Purpose: "verification_proof_followup", Status: WriteWorkflowBatchReadyToPlan}}}
			mu.SetWriteWorkflowRun(run)
			if err := mu.AuthorizeNativeTestRegistration("/repo", delivery, plan.BehaviorContracts, plan.TargetPaths); err != nil {
				t.Fatal(err)
			}
			grant := mu.NativeTestRegistrationAuthorization()
			token := grant.ID
			switch condition {
			case "wrong_token":
				token = "other"
			case "wrong_plan":
				report.PlanID = "other"
			case "wrong_channel":
				report.Channel = ChangeReportChannelPlannerProbe
			case "no_invocation":
				report.TestResults[0].InvocationID = ""
			case "duplicate_invocation":
				report.ExecutedCommands = append(report.ExecutedCommands, report.ExecutedCommands[0])
			case "unsupported_runner":
				report.ExecutedCommands[0].Runner = "go"
			case "framework_base_only":
				report.TestResults[0].ObservationScope = TestObservationScopeAggregate
			}
			before, _ := json.Marshal([]any{plan, report})
			mu.InstallNativeTestRegistrationIdentity(token, report)
			root := "/repo"
			switch condition {
			case "revoke":
				mu.RevokeNativeTestRegistrationAuthorization()
			case "different_run":
				run.RunID = "other"
				mu.SetWriteWorkflowRun(run)
			case "different_batch":
				run.ActiveBatchID = "other"
				mu.SetWriteWorkflowRun(run)
			case "different_root":
				root = "/other"
			case "new_grant":
				_ = mu.AuthorizeNativeTestRegistration("/repo", delivery, plan.BehaviorContracts, plan.TargetPaths)
			case "restore":
				body, _ := json.Marshal(run)
				var restored WriteWorkflowRun
				_ = json.Unmarshal(body, &restored)
				mu = NewMutableState("restored")
				mu.SetWriteWorkflowRun(&restored)
			case "dispatch_reset":
				mu.ResetDispatchToolResults()
			}
			got := mu.NativeTestRegistrationIdentitySnapshot(root)
			want := condition == "valid" || condition == "report_mutated_after_install" || condition == "dispatch_reset"
			if (got != "") != want {
				t.Fatalf("view present=%t want=%t: %s", got != "", want, got)
			}
			if want && (!strings.Contains(got, `"assertion_suite":"`+report.TestResults[0].Suite+`"`) || !strings.Contains(got, "not the current plan's report")) {
				t.Fatalf("source identity or historical boundary missing: %s", got)
			}
			after, _ := json.Marshal([]any{plan, report})
			if string(before) != string(after) || mu.ChangePlan() != nil || mu.ChangeReport() != nil || mu.NativeTestRegistrationExecutionAuthorized(plan, root) {
				t.Fatal("display rewrote evidence or granted active verification")
			}
			if condition == "report_mutated_after_install" {
				report.TestResults[0].Suite = "mutated"
				if mu.NativeTestRegistrationIdentitySnapshot(root) != got {
					t.Fatal("source mutation changed sealed display")
				}
			}
		})
	}
}
