package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func failureObservationPromptContext(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx := b1634ControllerProofContext(t, nil)
	var report types.ChangeReport
	if err := json.Unmarshal([]byte(`{"plan_id":"plan-proof-scope","channel":"post_apply_verify","passed":true,"verification_status":"passed","test_results":[{"passed":true}],"verification_diagnostics":[{"category":"probe_comparator_authority","reason_code":"model_authored_probe_comparator_unverified","runner":"verification_probe","outcome":"observed_failure","failure_observations":[{"assertion_id":"boundary-check","suite":"verification_probe/python","failure_detail":"Expected untouched item; got collapsed item","output_ref":"/tmp/retained-output.txt"}]}]}`), &report); err != nil {
		t.Fatal(err)
	}
	ctx.Mutable.SetChangeReport(&report)
	pack := types.WriteContextPackFromChangeReport(&report).WithScope("batch", "")
	for i := 0; i < 110; i++ {
		pack.Items = append(pack.Items, types.WriteContextItem{ID: fmt.Sprint("constraint-", i), Kind: "constraint", Priority: types.WriteContextP0, Text: "preserve the public interface"})
	}
	ctx.Mutable.SetWriteContextPack(&pack)
	ctx.Mutable.SetWriteWorkflowRun(&types.WriteWorkflowRun{ActiveBatchID: "batch", Batches: []types.WriteWorkflowBatch{{ID: "batch", PlanID: report.PlanID}}})
	return ctx
}

func TestFailureObservationPromptCurrentAndResetRecovery(t *testing.T) {
	for _, reset := range []bool{false, true} {
		ctx := failureObservationPromptContext(t)
		if reset {
			ctx.Mutable.ResetChangePlan()
			ctx.Mutable.ResetChangeReport()
		}
		before, _ := json.Marshal(ctx.Mutable.WriteContextPack())
		for _, got := range []string{(&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil), (&plannerEvaluator{}).BuildInitialInstruction(ctx, nil)} {
			for _, want := range []string{"boundary-check", "Expected untouched item; got collapsed item", "/tmp/retained-output.txt", "does not prove a product defect"} {
				if !strings.Contains(got, want) {
					t.Errorf("reset=%t missing %q before ordinary context cap", reset, want)
				}
			}
			if at := strings.Index(got, "boundary-check"); at < 0 || at > strings.Index(got, "## Priority write context pack") {
				t.Errorf("reset=%t failure observation buried behind context cap", reset)
			}
			if reset && !strings.Contains(got, "historical") {
				t.Error("restored observation misrepresented as current verification")
			}
		}
		after, _ := json.Marshal(ctx.Mutable.WriteContextPack())
		if string(before) != string(after) {
			t.Fatal("prompt changed stored context")
		}
	}
}

func TestFailureObservationPromptDoesNotReviveForeignOrSupersededContext(t *testing.T) {
	for _, mode := range []string{"current_without_observation", "foreign_report", "foreign_plan", "read"} {
		ctx := failureObservationPromptContext(t)
		switch mode {
		case "current_without_observation":
			ctx.Mutable.ChangeReport().VerificationDiagnostics = nil
		case "foreign_report":
			ctx.Mutable.ChangeReport().PlanID = "other"
		case "foreign_plan":
			ctx.Mutable.ResetChangeReport()
			ctx.Mutable.ResetChangePlan()
			run := ctx.Mutable.WriteWorkflowRun()
			run.Batches[0].PlanID = "other"
			ctx.Mutable.SetWriteWorkflowRun(run)
		case "read":
			ctx.Mode = types.ModeRead
		}
		// Only the dedicated current/recovery section is under test; normal
		// archival context retains its existing scope/visibility semantics.
		if got := buildWriteFailureObservationSection(ctx, types.WriteConsumerPlanner); got != "" {
			t.Errorf("%s revived an unrelated observation: %s", mode, got)
		}
	}
}
