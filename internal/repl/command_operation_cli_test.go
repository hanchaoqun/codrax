package repl

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/operation"
)

type fakeCLICommandPlanner struct {
	req operation.CommandOperationRequest
}

type adaptiveMaterialCLIPlanner struct {
	evaluatorCalls     int
	continuationCalls  int
	workDir            string
	sawMaterialPages   bool
	sawCoverageReceipt bool
}

func (p fakeCLICommandPlanner) PlanCommandOperation(ctx context.Context, userLine, repoRoot string, policy TurnPolicy) (operation.CommandOperationRequest, error) {
	return p.req, nil
}

func (p fakeCLICommandPlanner) PlanCommandOperationWithSnapshot(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, snapshot operation.CapabilitySnapshot) (operation.CommandOperationRequest, error) {
	return p.req, nil
}

func (p fakeCLICommandPlanner) AnswerCommandOperationRecords(ctx context.Context, userLine string, records []commandOperationResultRecord, lang string) (string, error) {
	if len(records) == 0 {
		return "no records", nil
	}
	return "final: " + strings.TrimSpace(records[len(records)-1].Result.OutputPreview), nil
}

func (p *adaptiveMaterialCLIPlanner) PlanCommandOperation(context.Context, string, string, TurnPolicy) (operation.CommandOperationRequest, error) {
	return operation.CommandOperationRequest{}, nil
}

func (p *adaptiveMaterialCLIPlanner) EvaluateCommandOperation(_ context.Context, _ string, records []commandOperationResultRecord, _ string) (operation.OperationEvaluation, error) {
	p.evaluatorCalls++
	last := records[len(records)-1]
	if len(last.MaterialPages) > 0 {
		p.sawMaterialPages = true
		p.sawCoverageReceipt = last.MaterialPages[0].CoverageReceiptRef != ""
	}
	if p.evaluatorCalls > 1 {
		return operation.OperationEvaluation{
			Status:                 operation.EvalComplete,
			Confidence:             "high",
			MaterialCoverageStatus: operation.MaterialCoverageComplete,
		}, nil
	}
	return operation.OperationEvaluation{
		Status:                 operation.EvalContinueCommand,
		Confidence:             "high",
		MaterialCoverageStatus: operation.MaterialCoveragePartial,
		Materials: []operation.OperationMaterial{{
			Source: operation.MaterialSourceCommand,
			Kind:   operation.MaterialKindPayloadRef,
			Role:   operation.MaterialRoleSavedPayload,
			Ref:    last.Result.PayloadRef,
		}},
	}, nil
}

func (p *adaptiveMaterialCLIPlanner) ContinueCommandOperation(context.Context, string, string, TurnPolicy, operation.CapabilitySnapshot, []commandOperationResultRecord) (CommandOperationContinuation, error) {
	p.continuationCalls++
	return CommandOperationContinuation{Request: operation.CommandOperationRequest{
		Text:      "continue bounded material read",
		WorkDir:   p.workDir,
		RiskLevel: "low",
		Steps: []operation.CommandStep{{
			ID:        "extended-read",
			Title:     "read one more bounded observation",
			Program:   "pwd",
			RiskLevel: "low",
		}},
	}}, nil
}

func (p *adaptiveMaterialCLIPlanner) AnswerCommandOperationRecords(_ context.Context, _ string, records []commandOperationResultRecord, _ string) (string, error) {
	return "final: " + strings.TrimSpace(records[len(records)-1].Result.OutputPreview), nil
}

func TestOperationPlannerErrorsUseTypedCodes(t *testing.T) {
	noTool := newOperationPlannerNoToolError("command operation planner", nil)
	if !operationPlannerErrorHasCode(noTool, operationPlannerErrorNoToolCall) {
		t.Fatal("typed no-tool operation planner error not recognized")
	}
	wrapped := newOperationPlannerUnexpectedToolError("command operation planner", "wrong_tool", noTool)
	if !operationPlannerErrorHasCode(wrapped, operationPlannerErrorUnexpectedTool) {
		t.Fatal("typed unexpected-tool operation planner error not recognized")
	}
	if operationPlannerErrorHasCode(errors.New("command operation planner: LLM returned no tool_call"), operationPlannerErrorNoToolCall) {
		t.Fatal("plain operation error text must not satisfy typed no-tool code")
	}
}

func TestRunCommandOperationCLI_ExecutesAutoPlanAndReturnsFinalAnswer(t *testing.T) {
	t.Parallel()
	policy := operation.DefaultCommandPolicy()
	policy.DefaultWorkDir = t.TempDir()
	req := operation.CommandOperationRequest{
		Text:      "query local environment",
		WorkDir:   policy.DefaultWorkDir,
		RiskLevel: "low",
		Goal:      "query local environment",
		Steps: []operation.CommandStep{{
			ID:        "info",
			Title:     "print fixture",
			Shell:     "printf cli-operation-ok",
			RiskLevel: "low",
		}},
	}
	var progress bytes.Buffer
	answer, err := RunCommandOperationCLI(context.Background(), req.Text, TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "computer_operation",
		OperationKind:        "computer_operation",
		Source:               "current_message",
		RiskLevel:            "low",
	}, CommandOperationCLIConfig{
		Planner:       fakeCLICommandPlanner{req: req},
		Policy:        policy,
		RepoRoot:      policy.DefaultWorkDir,
		RuntimeAnchor: t.TempDir(),
		Language:      "zh",
		Progress:      &progress,
	})
	if err != nil {
		t.Fatalf("RunCommandOperationCLI returned error: %v", err)
	}
	if !strings.Contains(answer, "cli-operation-ok") {
		t.Fatalf("final answer missing command observation:\n%s", answer)
	}
	if out := progress.String(); !strings.Contains(out, "操作计划") || !strings.Contains(out, "cli-operation-ok") {
		t.Fatalf("progress should include plan and execution result, got:\n%s", out)
	}
}

func TestRunCommandOperationCLI_EvaluatesBaseLimitAndExtendsIncompleteMaterial(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	policy := operation.DefaultCommandPolicy()
	policy.DefaultWorkDir = workDir
	planner := &adaptiveMaterialCLIPlanner{workDir: workDir}
	initialRecords := make([]commandOperationResultRecord, 0, commandOperationMaxCommandRounds-1)
	for i := 0; i < commandOperationMaxCommandRounds-1; i++ {
		initialRecords = append(initialRecords, commandOperationResultRecord{
			Plan:   operation.CommandOperationPlan{ID: "prior"},
			Result: operation.CommandOperationResult{Status: operation.StatusExecuted},
		})
	}
	initialPlan := operation.CommandOperationPlan{
		ID:          "base-limit-material",
		RequestText: "read the complete material",
		Status:      operation.StatusReady,
		RiskLevel:   "low",
		WorkDir:     workDir,
		Steps: []operation.CommandStep{{
			ID:        "large-observation",
			Title:     "emit a large bounded observation",
			Program:   "seq",
			Args:      []string{"1", "20000"},
			RiskLevel: "low",
		}},
	}

	answer, err := runCommandOperationCLIPlan(context.Background(), CommandOperationCLIConfig{
		Planner:       planner,
		Policy:        policy,
		RepoRoot:      workDir,
		RuntimeAnchor: t.TempDir(),
		Language:      "zh",
	}, initialPlan.RequestText, initialPlan, commandOperationAttemptState{
		// the prior rounds are this operation's OWN rounds, resumed at the
		// counter they reached (review round five #2: no re-derivation)
		Own:           initialRecords,
		CommandRounds: commandOperationMaxCommandRounds - 1,
	})
	if err != nil {
		t.Fatalf("runCommandOperationCLIPlan: %v", err)
	}
	if planner.evaluatorCalls != 1 || planner.continuationCalls != 1 {
		t.Fatalf("base-limit material did not earn exactly one continuation: evaluator=%d continuation=%d", planner.evaluatorCalls, planner.continuationCalls)
	}
	if !planner.sawMaterialPages || !planner.sawCoverageReceipt {
		t.Fatalf("CLI evaluator did not receive system material pages/receipt: pages=%t receipt=%t", planner.sawMaterialPages, planner.sawCoverageReceipt)
	}
	if strings.Contains(answer, "预算上限") || !strings.Contains(answer, workDir) {
		t.Fatalf("extended bounded command did not become the final observation:\n%s", answer)
	}
}

func TestCommandOperationMaterialExtensionCapsAndPublishesTypedBudget(t *testing.T) {
	t.Parallel()
	records := make([]commandOperationResultRecord, 0, commandOperationExtendedCommandRounds)
	for i := 0; i < commandOperationExtendedCommandRounds; i++ {
		record := commandOperationResultRecord{
			Plan: operation.CommandOperationPlan{ID: "material"},
			Result: operation.CommandOperationResult{
				Status:     operation.StatusExecuted,
				PayloadRef: "/tmp/material-page.txt",
			},
		}
		if i == commandOperationMaxCommandRounds-1 || i == commandOperationExtendedCommandRounds-1 {
			record.Evaluation = &operation.OperationEvaluation{
				Status:                 operation.EvalContinueCommand,
				MaterialCoverageStatus: operation.MaterialCoveragePartial,
			}
		}
		records = append(records, record)
	}
	if got := commandOperationCommandRoundLimit(records); got != commandOperationExtendedCommandRounds {
		t.Fatalf("adaptive material round limit=%d", got)
	}
	last := records[len(records)-1]
	if !commandOperationMaterialEvaluationNeedsBudget(last.Result, last.Evaluation, records) {
		t.Fatal("incomplete material at the extended limit must become budget-exhausted")
	}
	budget := commandOperationBudgetResult(last.Plan, commandOperationBudgetCommandRounds, "zh")
	if budget.Status != operation.StatusBudgetExhausted || budget.FailureClass != "budget_exhausted" || len(budget.StepResults) != 0 {
		t.Fatalf("budget result lost typed status/class or minted a synthetic step: %+v", budget)
	}
}

// replanCountingCLIPlanner answers the CLI lane's replan and continuation
// requests with a low-risk repair and records the records the answerer
// was handed.
type replanCountingCLIPlanner struct {
	replanCalls       int
	continuationCalls int
	answered          []commandOperationResultRecord
}

func (p *replanCountingCLIPlanner) PlanCommandOperation(context.Context, string, string, TurnPolicy) (operation.CommandOperationRequest, error) {
	return operation.CommandOperationRequest{}, nil
}

func (p *replanCountingCLIPlanner) ReplanCommandOperation(context.Context, string, string, TurnPolicy, operation.CapabilitySnapshot, operation.CommandOperationPlan, operation.CommandOperationResult) (operation.CommandOperationRequest, error) {
	p.replanCalls++
	return operation.CommandOperationRequest{
		Text:      "write the fallback marker",
		Goal:      "write fallback marker",
		RiskLevel: "low",
		WorkDir:   ".",
		Steps:     []operation.CommandStep{{ID: "write", Title: "write fallback", Program: "true", RiskLevel: "low"}},
	}, nil
}

func (p *replanCountingCLIPlanner) ContinueCommandOperation(context.Context, string, string, TurnPolicy, operation.CapabilitySnapshot, []commandOperationResultRecord) (CommandOperationContinuation, error) {
	p.continuationCalls++
	return CommandOperationContinuation{Complete: true, Reason: "nothing left"}, nil
}

func (p *replanCountingCLIPlanner) AnswerCommandOperationRecords(_ context.Context, _ string, records []commandOperationResultRecord, _ string) (string, error) {
	p.answered = append([]commandOperationResultRecord(nil), records...)
	return "final: " + strings.TrimSpace(records[len(records)-1].Result.OutputPreview), nil
}

// TestCommandOperationCLIFailedRoundAtCounterLimitEndsWithoutReplan is the
// CLI twin of pin i: the operation resumes with four executed own rounds at
// counter 4, the fifth round fails with the counter at the limit, and the
// lane mints the command-round budget terminal for the failed round's plan
// without consulting the replanner.
//
// EVOLUTION RECORD (review round seven #0): the CLI replan lane was gated on
// status/timeout/repair rounds only, so the replanner was consulted for a
// round the lane could never execute; its revised plan needed manual
// approval (it changes the working directory) and the single-shot lane —
// which has no /approve — returned the plan's approval text as the
// answer, never reaching the answerer. Red on 42fcf3fd1 and 6f98f839d
// (replan_calls=1, plan text returned) and identically on 533a939fb,
// b6f7eeec3 and 480939385, where the lane took the prior rounds as its
// record window and re-derived the counter (scratch copy of this pin on
// that signature).
func TestCommandOperationCLIFailedRoundAtCounterLimitEndsWithoutReplan(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()
	policy := operation.DefaultCommandPolicy()
	policy.DefaultWorkDir = workDir
	planner := &replanCountingCLIPlanner{}
	prior := make(commandOperationOwnRecords, 0, commandOperationMaxCommandRounds-1)
	for i := 0; i < commandOperationMaxCommandRounds-1; i++ {
		prior = append(prior, commandOperationResultRecord{
			Plan:   operation.CommandOperationPlan{ID: "prior"},
			Result: operation.CommandOperationResult{PlanID: "prior", Status: operation.StatusExecuted},
		})
	}
	failing := operation.CommandOperationPlan{
		ID:          "failing-fifth",
		RequestText: "probe, then write the fallback marker",
		Status:      operation.StatusReady,
		RiskLevel:   "low",
		WorkDir:     workDir,
		Steps: []operation.CommandStep{{
			ID:        "probe",
			Title:     "failing probe",
			Shell:     "printf 'probe failed\\n'; exit 2",
			RiskLevel: "low",
		}},
	}
	answer, err := runCommandOperationCLIPlan(context.Background(), CommandOperationCLIConfig{
		Planner:       planner,
		Policy:        policy,
		RepoRoot:      workDir,
		RuntimeAnchor: t.TempDir(),
		Language:      "en",
	}, failing.RequestText, failing, commandOperationAttemptState{
		Own:           prior,
		CommandRounds: commandOperationMaxCommandRounds - 1,
	})
	if err != nil {
		t.Fatalf("runCommandOperationCLIPlan: %v", err)
	}
	if planner.replanCalls != 0 || planner.continuationCalls != 0 {
		t.Fatalf("replan_calls=%d continuation_calls=%d, want 0/0 — a failed round at the command-round limit must not consult the planner", planner.replanCalls, planner.continuationCalls)
	}
	// four prior rounds, the failed fifth, the budget terminal
	if len(planner.answered) != commandOperationMaxCommandRounds+1 {
		t.Fatalf("answerer records=%d, want %d", len(planner.answered), commandOperationMaxCommandRounds+1)
	}
	failed := planner.answered[commandOperationMaxCommandRounds-1]
	if failed.Result.Status != operation.StatusFailed || failed.Plan.ID != failing.ID {
		t.Fatalf("fifth record=%s/%s, want the failed round of %s", failed.Plan.ID, failed.Result.Status, failing.ID)
	}
	last := planner.answered[len(planner.answered)-1]
	if last.Result.Status != operation.StatusBudgetExhausted || last.Plan.ID != failing.ID || !strings.Contains(last.Result.OutputPreview, "command-round budget exhausted") {
		t.Fatalf("terminal record=%s/%s preview=%q, want the command-round budget terminal for %s", last.Plan.ID, last.Result.Status, last.Result.OutputPreview, failing.ID)
	}
	if !strings.Contains(answer, "final: command operation command-round budget exhausted") {
		t.Fatalf("answer must be synthesised over the budget terminal:\n%s", answer)
	}
}

func TestCommandOperationCLIFinalAnswerPreservesBudgetTerminalStatus(t *testing.T) {
	t.Parallel()
	records := []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{
			ID:     "op-budget",
			Status: operation.StatusExecuted,
		},
		Result: operation.CommandOperationResult{
			PlanID:        "op-budget",
			Status:        operation.StatusBudgetExhausted,
			OutputPreview: "command operation command-round budget exhausted before the user goal was fully satisfied",
		},
	}}
	answer := commandOperationFinalMessageCLI(context.Background(), CommandOperationCLIConfig{
		Planner:  fakeCLICommandPlanner{},
		Language: "zh",
	}, "读取长网页并总结", records)
	if !strings.Contains(answer, "状态：部分结果") || !strings.Contains(answer, "预算上限") {
		t.Fatalf("budget terminal status must be visible before model summary:\n%s", answer)
	}
	if !strings.Contains(answer, "final:") {
		t.Fatalf("model summary should still be preserved after status prefix:\n%s", answer)
	}
}

func TestOperationFinalAnswerWarnsOnUncoveredMaterialRef(t *testing.T) {
	t.Parallel()
	records := []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{ID: "op-fetch", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID:     "op-fetch",
			Status:     operation.StatusExecuted,
			PayloadRef: "/tmp/manual.html",
		},
	}}
	answer := operationFinalReportWithRecordStatus("zh", "任务完成", records)
	if !strings.Contains(answer, "材料覆盖未完全验证") {
		t.Fatalf("uncovered payload ref should surface material coverage caveat:\n%s", answer)
	}
}

func TestOperationFinalAnswerNoMaterialWarningWhenPayloadExcerptExists(t *testing.T) {
	t.Parallel()
	payload := filepath.Join(t.TempDir(), "manual.txt")
	if err := os.WriteFile(payload, []byte("section one\nsection two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	records := []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{ID: "op-fetch", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID:     "op-fetch",
			Status:     operation.StatusExecuted,
			PayloadRef: payload,
		},
	}}
	answer := operationFinalReportWithRecordStatus("zh", "任务完成", records)
	if strings.Contains(answer, "材料覆盖未完全验证") {
		t.Fatalf("text payload excerpt should satisfy material coverage caveat:\n%s", answer)
	}
}

func TestOperationFinalAnswerNoMaterialWarningWhenLaterStepConsumesRef(t *testing.T) {
	t.Parallel()
	records := []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{ID: "op-fetch", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID:     "op-fetch",
			Status:     operation.StatusExecuted,
			PayloadRef: "/tmp/manual.html",
		},
	}, {
		Plan: operation.CommandOperationPlan{
			ID:     "op-extract",
			Status: operation.StatusExecuted,
			Steps:  []operation.CommandStep{{ID: "extract", Shell: "sed -n '/<article/,/<\\/article>/p' /tmp/manual.html"}},
		},
		Result: operation.CommandOperationResult{
			PlanID: "op-extract",
			Status: operation.StatusExecuted,
		},
	}}
	answer := operationFinalReportWithRecordStatus("zh", "任务完成", records)
	if strings.Contains(answer, "材料覆盖未完全验证") {
		t.Fatalf("consumed payload ref should not surface material coverage caveat:\n%s", answer)
	}
}

func TestOperationFinalAnswerUsesTaskScopedCompleteCoverageOverAuxiliaryPayload(t *testing.T) {
	t.Parallel()
	covered := filepath.Join(t.TempDir(), "manual.html")
	if err := os.WriteFile(covered, []byte("<html><body>complete manual</body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	records := commandOperationAttachMaterialPages([]commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{ID: "op-manual", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID:     "op-manual",
			Status:     operation.StatusExecuted,
			PayloadRef: covered,
		},
	}})
	if len(records[0].MaterialPages) == 0 || records[0].MaterialPages[0].CoverageReceiptRef == "" {
		t.Fatal("fixture did not produce complete material coverage receipt")
	}
	receipt := records[0].MaterialPages[0].CoverageReceiptRef
	records = append(records, commandOperationResultRecord{
		Plan: operation.CommandOperationPlan{ID: "op-aux", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID:     "op-aux",
			Status:     operation.StatusExecuted,
			PayloadRef: "/tmp/unconsumed-auxiliary.html",
		},
		Evaluation: &operation.OperationEvaluation{
			Status:                 operation.EvalComplete,
			MaterialCoverageStatus: operation.MaterialCoverageComplete,
			CoverageMaterialRefs:   []string{receipt},
		},
	})

	answer := operationFinalReportWithRecordStatus("zh", "模型结论", records)
	if answer != "模型结论" {
		t.Fatalf("validated task coverage must preserve the model answer without a global-history downgrade: %q", answer)
	}
}

func TestOperationFinalAnswerRetainsWarningWithoutCompleteTaskCoverage(t *testing.T) {
	t.Parallel()
	records := []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{ID: "op-aux", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID:     "op-aux",
			Status:     operation.StatusExecuted,
			PayloadRef: "/tmp/unconsumed-required.html",
		},
		Evaluation: &operation.OperationEvaluation{
			Status:                 operation.EvalContinueCommand,
			MaterialCoverageStatus: operation.MaterialCoveragePartial,
		},
	}}

	answer := operationFinalReportWithRecordStatus("zh", "模型结论", records)
	if !strings.Contains(answer, "材料覆盖未完全验证") {
		t.Fatalf("partial task coverage must retain the material warning: %q", answer)
	}
}
