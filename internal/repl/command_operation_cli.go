package repl

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/env"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/operation"
)

// CommandOperationCLIConfig is the single-shot companion to the REPL command
// operation path. It deliberately reuses the same planner, policy, lint,
// executor, repair, continuation, and answer synthesis contracts so CLI and
// REPL do not drift.
type CommandOperationCLIConfig struct {
	Planner       CommandOperationPlanner
	Policy        operation.CommandPolicy
	RepoRoot      string
	RuntimeAnchor string
	Language      string
	Providers     []operation.ProviderInfo
	Progress      io.Writer
}

// RunCommandOperationCLI executes a typed operation turn in single-shot CLI
// mode. Progress and command output summaries are written to cfg.Progress
// (normally stderr); the returned string is the final user-facing Markdown
// answer that the caller should print to stdout.
func RunCommandOperationCLI(ctx context.Context, userLine string, policy TurnPolicy, cfg CommandOperationCLIConfig) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	userLine = strings.TrimSpace(userLine)
	if userLine == "" {
		return "", fmt.Errorf("operation CLI: empty request")
	}
	if cfg.Planner == nil {
		return "", fmt.Errorf("operation CLI: command operation planner is not configured")
	}
	if isZeroOperationCommandPolicy(cfg.Policy) {
		cfg.Policy = operation.DefaultCommandPolicy()
	}
	if strings.TrimSpace(cfg.Language) == "" {
		cfg.Language = "zh"
	}

	snapshot := commandOperationCLICapabilitySnapshot(cfg)
	req, err := planCommandOperationForCLI(ctx, userLine, policy, cfg, snapshot)
	if err != nil {
		return "", err
	}
	plan := operation.BuildCommandOperationPlan(req, cfg.Policy)
	initial := operation.DecideCommandPlanApproval(cfg.Policy, plan, operation.CommandApprovalOptions{Phase: operation.CommandApprovalInitial})
	plan = operation.ApplyCommandPlanApprovalDecision(plan, initial)
	logging.Info("[cli/operation] command plan generated status=%s risk=%q approval=%q decision=%s reason_code=%s steps=%d request=%q",
		plan.Status, plan.RiskLevel, plan.ApprovalMode, initial.Action, initial.ReasonCode, len(plan.Steps), oneLineClamp(userLine, 160))

	switch {
	case plan.Status == operation.StatusReady && !operation.LintCommandOperationPlan(plan).OK():
		operationCLIProgress(cfg.Progress, commandOperationAutoValidateMarkdown(cfg.Language, plan))
		return runCommandOperationCLIPlan(ctx, cfg, userLine, plan, 0, nil)
	case plan.Status == operation.StatusReady && initial.AutoExecute():
		operationCLIProgress(cfg.Progress, commandOperationAutoExecuteMarkdown(cfg.Language, plan))
		return runCommandOperationCLIPlan(ctx, cfg, userLine, plan, 0, nil)
	default:
		// Single-shot CLI has no interactive /approve or clarification loop.
		// Return the same structured preflight text REPL would show, but keep
		// it as the final answer rather than falling through into repo analysis.
		return commandOperationPlanMarkdown(cfg.Language, plan), nil
	}
}

func planCommandOperationForCLI(ctx context.Context, userLine string, policy TurnPolicy, cfg CommandOperationCLIConfig, snapshot operation.CapabilitySnapshot) (operation.CommandOperationRequest, error) {
	if handoffPlanner, ok := cfg.Planner.(CommandOperationPlannerWithHandoff); ok {
		return handoffPlanner.PlanCommandOperationWithHandoff(ctx, userLine, cfg.RepoRoot, policy, snapshot, "")
	}
	if snapshotPlanner, ok := cfg.Planner.(CommandOperationPlannerWithSnapshot); ok {
		return snapshotPlanner.PlanCommandOperationWithSnapshot(ctx, userLine, cfg.RepoRoot, policy, snapshot)
	}
	return cfg.Planner.PlanCommandOperation(ctx, userLine, cfg.RepoRoot, policy)
}

func runCommandOperationCLIPlan(ctx context.Context, cfg CommandOperationCLIConfig, request string, initialPlan operation.CommandOperationPlan, initialRepairRounds int, initialRecords []commandOperationResultRecord) (string, error) {
	records := append([]commandOperationResultRecord(nil), initialRecords...)
	currentPlan := initialPlan
	repairRounds := initialRepairRounds
	commandRounds := commandOperationExecutedRoundCount(records)
	var syntheticResult *operation.CommandOperationResult

	executor := operation.CommandExecutor{
		Policy:    cfg.Policy,
		OutputDir: commandOperationCLIOutputDir(cfg.RuntimeAnchor),
		OnStepStart: func(step operation.CommandStep) {
			logging.Info("[cli/operation] command step start plan_id=%s step_id=%s cmd=%q risk=%q side_effects=%q",
				currentPlan.ID,
				step.ID,
				oneLineClamp(commandOperationStepCommand(step), 220),
				step.RiskLevel,
				strings.Join(step.SideEffects, ","))
			operationCLIProgress(cfg.Progress, commandOperationCLIStepProgress(cfg.Language, step))
		},
	}

	for {
		result := operation.CommandOperationResult{}
		if syntheticResult != nil {
			result = *syntheticResult
			syntheticResult = nil
		} else if lint := operation.LintCommandOperationPlan(currentPlan); !lint.OK() {
			result = commandOperationResultFromPlanLint(currentPlan, lint)
			logging.Info("[cli/operation] command plan lint failed plan_id=%s issues=%d summary=%q repair_rounds=%d command_rounds=%d",
				currentPlan.ID, len(lint.Issues), oneLineClamp(lint.Summary(), 240), repairRounds, commandRounds)
		} else if commandRounds >= commandOperationMaxCommandRounds {
			result = commandOperationBudgetResult(currentPlan, "command operation command-round budget exhausted before the user goal was fully satisfied")
		} else {
			commandRounds++
			result = executor.Execute(ctx, currentPlan)
			for _, stepResult := range result.StepResults {
				planStep := commandOperationStepByID(currentPlan, stepResult.StepID)
				logging.Info("[cli/operation] command step result plan_id=%s step_id=%s cmd=%q status=%s exit_code=%d timed_out=%t failure_class=%q verification=%s payload_ref=%s output_preview=%q error=%q",
					currentPlan.ID,
					stepResult.StepID,
					oneLineClamp(commandOperationStepCommand(planStep), 220),
					stepResult.Status,
					stepResult.ExitCode,
					stepResult.TimedOut,
					stepResult.FailureClass,
					stepResult.Verification.Status,
					stepResult.PayloadRef,
					oneLineClamp(stepResult.OutputPreview, 240),
					oneLineClamp(stepResult.Error, 240))
			}
		}

		currentPlan.Status = result.Status
		records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: result})
		operationCLIProgress(cfg.Progress, commandOperationResultMarkdown(cfg.Language, currentPlan, result))

		if result.Status == operation.StatusFailed && !commandResultTimedOut(result) && repairRounds < commandOperationMaxRepairRounds {
			replanner, ok := cfg.Planner.(CommandOperationReplanner)
			if ok {
				snapshot := commandOperationCLICapabilitySnapshot(cfg)
				revisedReq, err := replanner.ReplanCommandOperation(ctx, currentPlan.RequestText, cfg.RepoRoot, commandOperationPolicyFromPlan(currentPlan), snapshot, currentPlan, result)
				if err != nil {
					logging.Warning("[cli/operation] command replan failed: %v", err)
				} else {
					repairRounds++
					revisedReq = dropRepeatedFailedCommandSteps(revisedReq, currentPlan, result)
					revisedPlan := operation.BuildCommandOperationPlan(revisedReq, cfg.Policy)
					decision := operation.DecideCommandPlanApproval(cfg.Policy, revisedPlan, operation.CommandApprovalOptions{Phase: operation.CommandApprovalReplan, PreviousPlan: &currentPlan})
					revisedPlan = operation.ApplyCommandPlanApprovalDecision(revisedPlan, decision)
					logging.Info("[cli/operation] command replan generated previous_plan_id=%s status=%s risk=%q approval=%q decision=%s reason_code=%s steps=%d repair_rounds=%d",
						currentPlan.ID, revisedPlan.Status, revisedPlan.RiskLevel, revisedPlan.ApprovalMode, decision.Action, decision.ReasonCode, len(revisedPlan.Steps), repairRounds)
					if commandOperationTerminalAfterReplan(revisedPlan, records) {
						terminal := commandOperationTerminalResult(revisedPlan)
						records = append(records, commandOperationResultRecord{Plan: revisedPlan, Result: terminal})
						operationCLIProgress(cfg.Progress, commandOperationResultMarkdown(cfg.Language, revisedPlan, terminal))
						return commandOperationFinalMessageCLI(ctx, cfg, request, records), nil
					}
					if revisedPlan.Status == operation.StatusReady {
						if lint := commandRevisedPlanPreflightLint(revisedPlan, currentPlan, result); !lint.OK() {
							lintResult := commandOperationResultFromPlanLint(revisedPlan, lint)
							currentPlan = revisedPlan
							syntheticResult = &lintResult
							continue
						}
						if decision.AutoExecute() {
							operationCLIProgress(cfg.Progress, commandOperationReplanIntro(cfg.Language, revisedPlan)+"\n\n"+commandOperationAutoExecuteMarkdown(cfg.Language, revisedPlan))
							currentPlan = revisedPlan
							continue
						}
					}
					return commandOperationReplanIntro(cfg.Language, revisedPlan) + "\n\n" + commandOperationPlanMarkdown(cfg.Language, revisedPlan), nil
				}
			}
		}

		if commandOperationRepairBudgetExhausted(result, repairRounds) {
			budget := commandOperationBudgetResult(currentPlan, "command operation repair budget exhausted before the user goal was fully satisfied")
			records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: budget})
			operationCLIProgress(cfg.Progress, commandOperationResultMarkdown(cfg.Language, currentPlan, budget))
			return commandOperationFinalMessageCLI(ctx, cfg, request, records), nil
		}
		if commandOperationContinuationBudgetExhausted(currentPlan, result, records) {
			budget := commandOperationBudgetResult(currentPlan, "command operation command-round budget exhausted before the user goal was fully satisfied")
			records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: budget})
			operationCLIProgress(cfg.Progress, commandOperationResultMarkdown(cfg.Language, currentPlan, budget))
			return commandOperationFinalMessageCLI(ctx, cfg, request, records), nil
		}
		if result.Status == operation.StatusExecuted && currentPlan.ContinueAfter && commandRounds < commandOperationMaxCommandRounds {
			continuer, ok := cfg.Planner.(CommandOperationContinuationPlanner)
			if ok {
				snapshot := commandOperationCLICapabilitySnapshot(cfg)
				next, err := continuer.ContinueCommandOperation(ctx, currentPlan.RequestText, cfg.RepoRoot, commandOperationPolicyFromPlan(currentPlan), snapshot, records)
				if err != nil {
					logging.Warning("[cli/operation] command continuation planning failed: %v", err)
				} else if !next.Complete {
					nextPlan := operation.BuildCommandOperationPlan(next.Request, cfg.Policy)
					decision := operation.DecideCommandPlanApproval(cfg.Policy, nextPlan, operation.CommandApprovalOptions{Phase: operation.CommandApprovalContinuation, PreviousPlan: &currentPlan})
					nextPlan = operation.ApplyCommandPlanApprovalDecision(nextPlan, decision)
					logging.Info("[cli/operation] command continuation generated previous_plan_id=%s status=%s risk=%q approval=%q decision=%s reason_code=%s steps=%d rounds=%d",
						currentPlan.ID, nextPlan.Status, nextPlan.RiskLevel, nextPlan.ApprovalMode, decision.Action, decision.ReasonCode, len(nextPlan.Steps), len(records))
					if commandOperationTerminalAfterReplan(nextPlan, records) {
						terminal := commandOperationTerminalResult(nextPlan)
						records = append(records, commandOperationResultRecord{Plan: nextPlan, Result: terminal})
						operationCLIProgress(cfg.Progress, commandOperationResultMarkdown(cfg.Language, nextPlan, terminal))
						return commandOperationFinalMessageCLI(ctx, cfg, request, records), nil
					}
					if lint := operation.LintCommandOperationPlan(nextPlan); !lint.OK() {
						lintResult := commandOperationResultFromPlanLint(nextPlan, lint)
						currentPlan = nextPlan
						syntheticResult = &lintResult
						continue
					}
					if nextPlan.Status == operation.StatusReady && decision.AutoExecute() {
						operationCLIProgress(cfg.Progress, commandOperationContinuationIntro(cfg.Language, nextPlan)+"\n\n"+commandOperationAutoExecuteMarkdown(cfg.Language, nextPlan))
						currentPlan = nextPlan
						continue
					}
					return commandOperationContinuationIntro(cfg.Language, nextPlan) + "\n\n" + commandOperationPlanMarkdown(cfg.Language, nextPlan), nil
				}
			}
		}
		return commandOperationFinalMessageCLI(ctx, cfg, request, records), nil
	}
}

func commandOperationFinalMessageCLI(ctx context.Context, cfg CommandOperationCLIConfig, userLine string, records []commandOperationResultRecord) string {
	if len(records) == 0 {
		return operationFinalReportFallback(cfg.Language, operation.StatusFailed, 0)
	}
	last := records[len(records)-1]
	fallback := operationFinalReportFallback(cfg.Language, last.Result.Status, len(records))
	if answerer, ok := cfg.Planner.(CommandOperationRecordsAnswerer); ok && answerer != nil {
		answer, err := answerer.AnswerCommandOperationRecords(ctx, userLine, records, cfg.Language)
		if err != nil {
			logging.Warning("[cli/operation] command result answer synthesis failed: %v", err)
			return fallback
		}
		thoughts, answer := splitVisibleThinkBlocks(strings.TrimSpace(answer))
		operationCLIThoughts(cfg.Progress, cfg.Language, thoughts)
		if strings.TrimSpace(answer) != "" {
			return operationFinalReportWithRecordStatus(cfg.Language, strings.TrimSpace(answer), records)
		}
		return fallback
	}
	if answerer, ok := cfg.Planner.(CommandOperationAnswerer); ok && answerer != nil {
		answer, err := answerer.AnswerCommandOperationResult(ctx, userLine, last.Plan, last.Result, cfg.Language)
		if err != nil {
			logging.Warning("[cli/operation] command result answer synthesis failed: %v", err)
			return fallback
		}
		thoughts, answer := splitVisibleThinkBlocks(strings.TrimSpace(answer))
		operationCLIThoughts(cfg.Progress, cfg.Language, thoughts)
		if strings.TrimSpace(answer) != "" {
			return operationFinalReportWithRecordStatus(cfg.Language, strings.TrimSpace(answer), records)
		}
	}
	return fallback
}

func commandOperationCLICapabilitySnapshot(cfg CommandOperationCLIConfig) operation.CapabilitySnapshot {
	facts := env.Probe(env.ProbeOptions{RepoRoot: cfg.RepoRoot, ProbeNetwork: false})
	return operation.BuildCapabilitySnapshotWithProviders(facts, cfg.RepoRoot, cfg.Policy, cfg.Providers)
}

func commandOperationCLIOutputDir(runtimeAnchor string) string {
	runtimeAnchor = strings.TrimSpace(runtimeAnchor)
	if runtimeAnchor == "" {
		return ""
	}
	if abs, err := filepath.Abs(runtimeAnchor); err == nil {
		runtimeAnchor = abs
	}
	return filepath.Join(runtimeAnchor, "operation")
}

func commandOperationCLIStepProgress(lang string, step operation.CommandStep) string {
	title := strings.TrimSpace(step.Title)
	if title == "" {
		title = strings.TrimSpace(step.ID)
	}
	if title == "" {
		title = commandOperationStepCommand(step)
	}
	if isZh(lang) {
		return "操作中：" + title
	}
	return "Running: " + title
}

func operationCLIProgress(w io.Writer, msg string) {
	msg = strings.TrimSpace(msg)
	if w == nil || msg == "" {
		return
	}
	fmt.Fprintln(w, msg)
	fmt.Fprintln(w)
}

func operationCLIThoughts(w io.Writer, lang string, thoughts []string) {
	if w == nil || len(thoughts) == 0 {
		return
	}
	for i, thought := range thoughts {
		thought = strings.TrimSpace(thought)
		if thought == "" {
			continue
		}
		fmt.Fprintln(w, operationThoughtsHeaderMsg(lang, i+1, len(thoughts)))
		for _, line := range strings.Split(thought, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) == "" {
				continue
			}
			fmt.Fprintln(w, "  "+line)
		}
		fmt.Fprintln(w)
	}
}
