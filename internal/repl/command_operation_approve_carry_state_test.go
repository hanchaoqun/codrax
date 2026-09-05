package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/operation"
)

// command_operation_approve_carry_state_test.go — direct pins on the carry
// a parked plan travels with, one per park lane (colleague_merge_audit
// §40.52, review round four #9 and review round five #2).
//
// The carry is {PlanID, Context, Own, CommandRounds, ReplanAttempts}:
// Context are earlier operations' rounds (planner/answerer context only),
// Own are this operation's rounds, CommandRounds is the loop's counter at
// the park, ReplanAttempts the repair rounds spent. Fresh lanes (initial
// ready plan, follow-up park, provider-to-command continuation) start with
// Own=nil, CommandRounds=0 and Context = the window they already pass;
// in-loop parks (continuation, evaluation continuation, repair) carry the
// loop's context, own rounds, counter and repair rounds spent.
//
// EVOLUTION RECORD (review round five #2): on 533a939fb the carry held one
// Records slice and the approve arm re-derived the counter from it, so a
// follow-up parked with the cross-operation window resumed with its budget
// already spent (see command_operation_followup_budget_test.go).
//
// EVOLUTION RECORD (review round six #0): on 6f98f839d the two continuation
// parks (evaluation continuation and continuation) carried the counter but
// not the repair rounds spent, so a continuation parked after a repair
// resumed with ReplanAttempts=0 and an approved failure restarted the
// repair budget (see command_operation_approve_budget_carry_test.go); the
// two *_after_repair_* arms below were red there (replan_attempts=0).
func TestCommandOperationParkedPlanCarriesOperationState(t *testing.T) {
	const (
		goRound = `{"status":"ready","risk_level":"low","requires_confirmation":false,"continue_after":true,"work_dir":".","goal":"print go version","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`
		gitPark = `{"status":"ready","risk_level":"medium","requires_confirmation":true,"work_dir":".","goal":"print git version","steps":[{"id":"s2","title":"show git version","program":"git","args":["version"],"risk_level":"medium","side_effects":["process_spawn"]}]}`
		failing = `{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"probe","steps":[{"id":"bad","title":"failing probe","shell":"printf 'probe failed\\n'; exit 2","risk_level":"low","side_effects":[]}]}`
	)
	requireCarry := func(t *testing.T, r *REPL, wantContext, wantOwn, wantRounds, wantReplan int) *pendingCommandOperationCarry {
		t.Helper()
		if r.pendingOperation == nil {
			t.Fatal("plan should park for approval")
		}
		carry := r.pendingOperationCarry
		if carry == nil || carry.PlanID != r.pendingOperation.ID {
			t.Fatalf("parked plan %s must carry the operation state keyed by its ID, got %+v", r.pendingOperation.ID, carry)
		}
		if len(carry.Context) != wantContext || len(carry.Own) != wantOwn || carry.CommandRounds != wantRounds || carry.ReplanAttempts != wantReplan {
			t.Fatalf("carry context=%d own=%d command_rounds=%d replan_attempts=%d, want %d/%d/%d/%d",
				len(carry.Context), len(carry.Own), carry.CommandRounds, carry.ReplanAttempts, wantContext, wantOwn, wantRounds, wantReplan)
		}
		return carry
	}
	newREPL := func(t *testing.T, classifier ChitchatClassifier, adapter *scriptedChatAdapter, input string) *REPL {
		t.Helper()
		r, _, _ := newTurnPolicyREPL(t, newPolicyStore(t), classifier, &stubLocalResponder{}, input)
		r.operationEnabled = true
		r.operationPlanner = NewCommandOperationPlanner(adapter)
		r.operationPolicy = operation.DefaultCommandPolicy()
		r.operationPolicy.AutoApprove = false
		r.operationPolicy.AutoLowRisk = true
		return r
	}
	lowOperation := &stubTurnPolicyClassifier{policy: commandOperationPolicy("low")}
	// highPark is a high-risk write that parks on its risk signal even
	// under AutoApprove
	highPark := func(t *testing.T) string {
		return `{"status":"ready","risk_level":"high","requires_confirmation":true,"work_dir":".","goal":"write fallback","steps":[{"id":"write","title":"write fallback","program":"touch","args":["` + filepath.Join(t.TempDir(), "marker") + `"],"risk_level":"high","side_effects":["local_file_write"]}]}`
	}

	t.Run("initial_ready_plan_starts_fresh", func(t *testing.T) {
		adapter := &scriptedChatAdapter{responses: []llm.Response{commandOperationPlanResp(gitPark)}}
		r := newREPL(t, lowOperation, adapter, "print the git version\n/exit\n")
		if err := r.Loop(); err != nil {
			t.Fatalf("Loop: %v", err)
		}
		requireCarry(t, r, 0, 0, 0, 0)
	})
	t.Run("in_loop_continuation_carries_own_rounds_and_counter", func(t *testing.T) {
		adapter := &scriptedChatAdapter{responses: []llm.Response{commandOperationPlanResp(goRound), commandOperationPlanResp(gitPark)}}
		r := newREPL(t, lowOperation, adapter, "print both the go and git versions\n/exit\n")
		if err := r.Loop(); err != nil {
			t.Fatalf("Loop: %v", err)
		}
		carry := requireCarry(t, r, 0, 1, 1, 0)
		if carry.Own[0].Result.Status != operation.StatusExecuted || carry.Own[0].Plan.ID != r.operationResults[0].Plan.ID {
			t.Fatalf("carry own rounds = %+v, want the executed go-version round", carry.Own)
		}
		r.handleOperationRejectCmd("/reject")
		if r.pendingOperation != nil || r.pendingOperationCarry != nil {
			t.Fatalf("/reject must drop the parked plan and its carry: plan=%v carry=%v", r.pendingOperation, r.pendingOperationCarry)
		}
	})
	t.Run("in_loop_evaluation_continuation_carries_evaluated_round", func(t *testing.T) {
		// a low-risk read whose output exceeds the panel preview mints a
		// payload ref, which is what admits the material evaluator
		material := filepath.Join(t.TempDir(), "material.txt")
		if err := os.WriteFile(material, []byte(strings.Repeat("material line\n", 20000)), 0o644); err != nil {
			t.Fatalf("write material: %v", err)
		}
		adapter := &scriptedChatAdapter{responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"read the material","steps":[{"id":"read","title":"read the material file","program":"cat","args":["` + material + `"],"risk_level":"low","side_effects":[]}]}`),
			operationEvaluationResp(`{"status":"continue_command","reason":"the observation needs a second bounded read","confidence":"high","material_coverage_status":"partial"}`),
			commandOperationPlanResp(gitPark),
		}}
		r := newREPL(t, lowOperation, adapter, "read the complete material\n/exit\n")
		r.runtimeAnchor = t.TempDir()
		if err := r.Loop(); err != nil {
			t.Fatalf("Loop: %v", err)
		}
		if len(adapter.calls) != 3 {
			t.Fatalf("adapter calls=%d, want 3 (planner, evaluator, continuation)", len(adapter.calls))
		}
		carry := requireCarry(t, r, 0, 1, 1, 0)
		if carry.Own[0].Result.Status != operation.StatusExecuted || carry.Own[0].Evaluation == nil {
			t.Fatalf("carry own round = %+v, want the executed round with its evaluation attached", carry.Own[0])
		}
	})
	t.Run("in_loop_continuation_after_repair_carries_repair_count", func(t *testing.T) {
		// the probe fails and spends one repair; the auto-executed repair
		// round continues into a high-risk continuation that parks
		adapter := &scriptedChatAdapter{responses: []llm.Response{
			commandOperationPlanResp(failing),
			commandOperationPlanResp(goRound),
			commandOperationPlanResp(highPark(t)),
		}}
		r := newREPL(t, lowOperation, adapter, "probe, print the go version, then record a marker file\n/exit\n")
		r.operationPolicy.AutoApprove = true
		if err := r.Loop(); err != nil {
			t.Fatalf("Loop: %v", err)
		}
		if len(adapter.calls) != 3 {
			t.Fatalf("adapter calls=%d, want 3 (planner, repair, continuation)", len(adapter.calls))
		}
		carry := requireCarry(t, r, 0, 2, 2, 1)
		if carry.Own[0].Result.Status != operation.StatusFailed || carry.Own[1].Result.Status != operation.StatusExecuted {
			t.Fatalf("carry own rounds = %+v, want the failed probe then the executed repair round", carry.Own)
		}
	})
	t.Run("in_loop_evaluation_continuation_after_repair_carries_repair_count", func(t *testing.T) {
		material := filepath.Join(t.TempDir(), "material.txt")
		if err := os.WriteFile(material, []byte(strings.Repeat("material line\n", 20000)), 0o644); err != nil {
			t.Fatalf("write material: %v", err)
		}
		adapter := &scriptedChatAdapter{responses: []llm.Response{
			commandOperationPlanResp(failing),
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","goal":"read the material","steps":[{"id":"read","title":"read the material file","program":"cat","args":["` + material + `"],"risk_level":"low","side_effects":[]}]}`),
			operationEvaluationResp(`{"status":"continue_command","reason":"the observation needs a second bounded read","confidence":"high","material_coverage_status":"partial"}`),
			commandOperationPlanResp(highPark(t)),
		}}
		r := newREPL(t, lowOperation, adapter, "probe, then read the complete material\n/exit\n")
		r.runtimeAnchor = t.TempDir()
		r.operationPolicy.AutoApprove = true
		if err := r.Loop(); err != nil {
			t.Fatalf("Loop: %v", err)
		}
		if len(adapter.calls) != 4 {
			t.Fatalf("adapter calls=%d, want 4 (planner, repair, evaluator, continuation)", len(adapter.calls))
		}
		carry := requireCarry(t, r, 0, 2, 2, 1)
		if carry.Own[0].Result.Status != operation.StatusFailed || carry.Own[1].Result.Status != operation.StatusExecuted || carry.Own[1].Evaluation == nil {
			t.Fatalf("carry own rounds = %+v, want the failed probe then the executed repair round with its evaluation attached", carry.Own)
		}
	})
	t.Run("in_loop_repair_carries_failed_round_and_repair_count", func(t *testing.T) {
		adapter := &scriptedChatAdapter{responses: []llm.Response{
			commandOperationPlanResp(failing),
			commandOperationPlanResp(highPark(t)),
		}}
		r := newREPL(t, lowOperation, adapter, "record a marker file, probing first\n/exit\n")
		// the shell-form probe is auto-eligible only under AutoApprove; the
		// high-risk repair plan parks on its risk signal regardless
		r.operationPolicy.AutoApprove = true
		if err := r.Loop(); err != nil {
			t.Fatalf("Loop: %v", err)
		}
		carry := requireCarry(t, r, 0, 1, 1, 1)
		if carry.Own[0].Result.Status != operation.StatusFailed {
			t.Fatalf("carry own rounds = %+v, want the failed probe round", carry.Own)
		}
	})
	t.Run("followup_carries_window_as_context_only", func(t *testing.T) {
		classifier := &sequenceTurnPolicyClassifier{policies: []TurnPolicy{
			commandOperationPolicy("low"),
			{Route: RouteLocal, Operation: "elaborate", Source: "last_answer", Confidence: 0.9, Reason: "continue"},
		}}
		adapter := &scriptedChatAdapter{responses: []llm.Response{
			commandOperationPlanResp(goRound), commandOperationPlanResp(goRound), commandOperationPlanResp(goRound), commandOperationPlanResp(goRound), commandOperationPlanResp(goRound),
			{Content: "five rounds done.", StopReason: "end_turn"},
			commandOperationPlanResp(gitPark),
		}}
		r := newREPL(t, classifier, adapter, "run five go rounds\nnow the git version too\n/exit\n")
		if err := r.Loop(); err != nil {
			t.Fatalf("Loop: %v", err)
		}
		if len(adapter.calls) != 7 {
			t.Fatalf("adapter calls=%d, want 7 (planner, 4 continuations, answer, follow-up continuation)", len(adapter.calls))
		}
		window := len(r.operationResults)
		if window != 6 {
			t.Fatalf("operationResults=%d, want the capped window of 6 (five executed rounds + the budget round)", window)
		}
		carry := requireCarry(t, r, window, 0, 0, 0)
		for i := range carry.Context {
			if carry.Context[i].Plan.ID != r.operationResults[i].Plan.ID {
				t.Fatalf("carry context[%d] plan %s, want the window's %s", i, carry.Context[i].Plan.ID, r.operationResults[i].Plan.ID)
			}
		}
	})
	t.Run("provider_to_command_continuation_starts_fresh", func(t *testing.T) {
		classifier := &stubTurnPolicyClassifier{policy: TurnPolicy{
			Route:                RouteOperation,
			NeedsOperationAccess: true,
			Operation:            "presentation_generation",
			OperationKind:        "presentation_generation",
			Source:               "current_message",
			RiskLevel:            "low",
			TargetSurface:        "slides",
			SideEffects:          []string{"local_file_write"},
			Confidence:           0.9,
			Reason:               "user asked for a presentation artifact",
		}}
		server := &operationProviderMCPServer{}
		reg := mcp.NewRegistry()
		if err := reg.Register(server); err != nil {
			t.Fatalf("register MCP server: %v", err)
		}
		adapter := &scriptedChatAdapter{responses: []llm.Response{
			operationEvaluationResp(`{"status":"continue_command","reason":"provider returned a payload ref that needs bounded extraction","confidence":"high","material_refs":["/tmp/codrax/deck.pptx"]}`),
			commandOperationPlanResp(gitPark),
		}}
		r := newREPL(t, classifier, adapter, "生成一份 PPT 并说明结果\n/approve\n/exit\n")
		r.mcpServers = reg
		r.operationProviders = []operation.ProviderInfo{{
			Name:         "mcp:slides",
			Kind:         "presentation_generation",
			Surfaces:     []string{"slides"},
			SideEffects:  []string{"local_file_write"},
			RequiresGate: true,
			ToolName:     "run_operation",
		}}
		if err := r.Loop(); err != nil {
			t.Fatalf("Loop: %v", err)
		}
		if len(server.got) == 0 {
			t.Fatal("the provider operation must run before the command continuation is planned")
		}
		if len(adapter.calls) != 2 {
			t.Fatalf("adapter calls=%d, want 2 (evaluator, command plan)", len(adapter.calls))
		}
		requireCarry(t, r, 0, 0, 0, 0)
	})
	t.Run("foreign_plan_id_never_resumes", func(t *testing.T) {
		r := &REPL{pendingOperationCarry: &pendingCommandOperationCarry{PlanID: "plan-a", commandOperationAttemptState: commandOperationAttemptState{
			Context:        commandOperationContextRecords{{}},
			Own:            commandOperationOwnRecords{{}},
			CommandRounds:  3,
			ReplanAttempts: 2,
		}}}
		got := r.takePendingOperationCarry("plan-b")
		if got.PlanID != "plan-b" || got.ReplanAttempts != 0 || got.CommandRounds != 0 || len(got.Context) != 0 || len(got.Own) != 0 {
			t.Fatalf("carry for plan-a must not resume plan-b, got %+v", got)
		}
		if r.pendingOperationCarry != nil {
			t.Fatal("a taken carry must be dropped")
		}
	})
}
