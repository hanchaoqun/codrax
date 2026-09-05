package repl

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/operation"
)

// TestCommandOperationParkedPlanCarriesOperationState pins the carrier
// itself (review round four #9): a parked continuation plan carries the
// executed round's record keyed by the parked plan's ID; a parked repair
// plan carries the failed round's record and the repair round spent;
// /reject drops the carry; a carry for another plan ID never resumes.
func TestCommandOperationParkedPlanCarriesOperationState(t *testing.T) {
	t.Run("continuation", func(t *testing.T) {
		store := newPolicyStore(t)
		classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
		adapter := &scriptedChatAdapter{responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"continue_after":true,"work_dir":".","goal":"print go version","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"medium","requires_confirmation":true,"work_dir":".","goal":"print git version","steps":[{"id":"s2","title":"show git version","program":"git","args":["version"],"risk_level":"medium","side_effects":["process_spawn"]}]}`),
		}}
		r, _, _ := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "print both the go and git versions\n/exit\n")
		r.operationEnabled = true
		r.operationPlanner = NewCommandOperationPlanner(adapter)
		r.operationPolicy = operation.DefaultCommandPolicy()
		r.operationPolicy.AutoApprove = false
		r.operationPolicy.AutoLowRisk = true
		if err := r.Loop(); err != nil {
			t.Fatalf("Loop: %v", err)
		}
		if r.pendingOperation == nil {
			t.Fatal("continuation plan should park for approval")
		}
		carry := r.pendingOperationCarry
		if carry == nil || carry.PlanID != r.pendingOperation.ID {
			t.Fatalf("parked plan %s must carry the operation state keyed by its ID, got %+v", r.pendingOperation.ID, carry)
		}
		if len(carry.Records) != 1 || carry.Records[0].Result.Status != operation.StatusExecuted || carry.Records[0].Plan.ID != r.operationResults[0].Plan.ID {
			t.Fatalf("carry records = %+v, want the executed go-version round", carry.Records)
		}
		if carry.ReplanAttempts != 0 {
			t.Fatalf("continuation carry ReplanAttempts = %d, want 0", carry.ReplanAttempts)
		}
		r.handleOperationRejectCmd("/reject")
		if r.pendingOperation != nil || r.pendingOperationCarry != nil {
			t.Fatalf("/reject must drop the parked plan and its carry: plan=%v carry=%v", r.pendingOperation, r.pendingOperationCarry)
		}
	})
	t.Run("replan", func(t *testing.T) {
		store := newPolicyStore(t)
		classifier := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
		adapter := &scriptedChatAdapter{responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"probe","steps":[{"id":"bad","title":"failing probe","shell":"printf 'probe failed\\n'; exit 2","risk_level":"low","side_effects":[]}]}`),
			commandOperationPlanResp(`{"status":"ready","risk_level":"high","requires_confirmation":true,"work_dir":".","goal":"write fallback","steps":[{"id":"write","title":"write fallback","program":"touch","args":["` + t.TempDir() + `/marker"],"risk_level":"high","side_effects":["local_file_write"]}]}`),
		}}
		r, _, _ := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "record a marker file, probing first\n/exit\n")
		r.operationEnabled = true
		r.operationPlanner = NewCommandOperationPlanner(adapter)
		r.operationPolicy = operation.DefaultCommandPolicy()
		if err := r.Loop(); err != nil {
			t.Fatalf("Loop: %v", err)
		}
		if r.pendingOperation == nil {
			t.Fatal("risk-escalating repair plan should park for approval")
		}
		carry := r.pendingOperationCarry
		if carry == nil || carry.PlanID != r.pendingOperation.ID {
			t.Fatalf("parked repair plan %s must carry the operation state keyed by its ID, got %+v", r.pendingOperation.ID, carry)
		}
		if len(carry.Records) != 1 || carry.Records[0].Result.Status != operation.StatusFailed {
			t.Fatalf("carry records = %+v, want the failed probe round", carry.Records)
		}
		if carry.ReplanAttempts != 1 {
			t.Fatalf("repair carry ReplanAttempts = %d, want 1 (the repair round already spent)", carry.ReplanAttempts)
		}
	})
	t.Run("foreign_plan_id_never_resumes", func(t *testing.T) {
		r := &REPL{pendingOperationCarry: &pendingCommandOperationCarry{PlanID: "plan-a", ReplanAttempts: 2, Records: []commandOperationResultRecord{{}}}}
		got := r.takePendingOperationCarry("plan-b")
		if got.PlanID != "plan-b" || got.ReplanAttempts != 0 || len(got.Records) != 0 {
			t.Fatalf("carry for plan-a must not resume plan-b, got %+v", got)
		}
		if r.pendingOperationCarry != nil {
			t.Fatal("a taken carry must be dropped")
		}
	})
}
