package repl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// DataTaskCLIConfig is the single-shot companion to the REPL data workflow.
// It intentionally shares the same planner interfaces and deterministic runner
// as the REPL path so CLI and REPL do not drift into different data-lane
// semantics.
type DataTaskCLIConfig struct {
	Planner         DataTaskPlanner
	RepoRoot        string
	RuntimeAnchor   string
	Language        string
	MaxRepairRounds int
	MaxDataRounds   int
	Progress        io.Writer
}

func RunDataTaskCLI(ctx context.Context, request string, policy TurnPolicy, cfg DataTaskCLIConfig) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.Planner == nil {
		return "", fmt.Errorf("data task planner is not configured")
	}
	repoRoot := strings.TrimSpace(cfg.RepoRoot)
	if repoRoot == "" {
		repoRoot = "."
	}
	candidates, err := dataquery.DiscoverCandidateFiles(repoRoot, 240)
	if err != nil {
		return "", fmt.Errorf("data task discover files: %w", err)
	}
	plan, err := cfg.Planner.PlanDataTask(ctx, request, repoRoot, policy, candidates)
	if err != nil {
		return "", fmt.Errorf("data task plan: %w", err)
	}
	if normalized, notes := normalizeDataTaskPlanShape(plan); len(notes) > 0 {
		logging.Info("[cli/data] normalized initial data task plan: %s", strings.Join(notes, "; "))
		plan = normalized
	}
	protectPlan := func(p dataquery.TaskPlan) dataquery.TaskPlan {
		return applyDataTaskUserMaterialFloor(request, candidates, p)
	}
	plan = protectPlan(plan)
	auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "initial", 0, plan)
	dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, plan)

	repairRoundsMax := normalizeDataTaskMaxRepairRounds(cfg.MaxRepairRounds)
	dataRoundsMax := normalizeDataTaskMaxDataRounds(cfg.MaxDataRounds)
	runner := dataquery.Runner{
		RepoRoot: repoRoot,
		TempRoot: filepath.Join(firstNonEmptyString(strings.TrimSpace(cfg.RuntimeAnchor), repoRoot), "data"),
	}
	actionRunner := dataquery.ActionRunner{
		RepoRoot: repoRoot,
		TempRoot: filepath.Join(firstNonEmptyString(strings.TrimSpace(cfg.RuntimeAnchor), repoRoot), "data"),
	}
	currentPlan := plan
	var records []dataTaskWorkflowRecord
	repairRounds := 0
	dataRounds := 0
	for {
		if errText := dataTaskTerminalPlanCompletionGateError(records, currentPlan); errText != "" {
			repaired, nextRepairRounds, ok, repairErr := repairDataTaskPlanForCLI(ctx, cfg.Planner, request, repoRoot, policy, candidates, currentPlan, errText, records, repairRounds, repairRoundsMax)
			repairRounds = nextRepairRounds
			if repairErr != nil {
				return "", repairErr
			}
			if ok {
				if normalized, notes := normalizeDataTaskPlanShape(repaired); len(notes) > 0 {
					logging.Info("[cli/data] normalized terminal-completion repaired data task plan: %s", strings.Join(notes, "; "))
					repaired = normalized
				}
				repaired = protectPlan(repaired)
				auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "repair", repairRounds, repaired)
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, repaired)
				currentPlan = repaired
				continue
			}
			return "", fmt.Errorf("%s", errText)
		}
		if handled, answer, err := terminalDataTaskPlanForCLI(currentPlan, records, cfg.Language); handled {
			return answer, err
		}
		if dataRounds >= dataRoundsMax {
			if result, ok := latestDataTaskResult(records); ok {
				return dataTaskAnswerMarkdown(cfg.Language, result), nil
			}
			return "", fmt.Errorf("data task workflow budget exhausted before producing a result")
		}
		if fallback, ok := dataTaskCoverageExpansionFallback(records, currentPlan, "missing material coverage before execution"); ok {
			records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: "missing material coverage converted to atomic coverage batch"})
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "missing material coverage converted to atomic coverage batch")
			fallback = protectPlan(fallback)
			auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
			dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
			currentPlan = fallback
			continue
		}
		if fallback, ok := dataTaskMaterialDiscoveryFallback(records, currentPlan, "broad material custom action requires objective material discovery before execution"); ok {
			records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: "broad material custom action converted to material discovery"})
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "broad material custom action converted to material discovery")
			fallback = protectPlan(fallback)
			auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
			dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
			currentPlan = fallback
			continue
		}
		if errText := dataTaskWorkflowStagingGuardError(records, currentPlan); errText != "" {
			if fallback, ok := dataTaskCoverageExpansionFallback(records, currentPlan, errText); ok {
				records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "missing material coverage converted to atomic coverage batch")
				fallback = protectPlan(fallback)
				auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
				currentPlan = fallback
				continue
			}
			if fallback, ok := dataTaskMaterialDiscoveryFallback(records, currentPlan, errText); ok {
				records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "broad material plan converted to material discovery")
				fallback = protectPlan(fallback)
				auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
				currentPlan = fallback
				continue
			}
			var repaired dataquery.TaskPlan
			var ok bool
			repaired, repairRounds, ok, err = repairDataTaskPlanForCLI(ctx, cfg.Planner, request, repoRoot, policy, candidates, currentPlan, errText, records, repairRounds, repairRoundsMax)
			if err != nil {
				return "", err
			}
			records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
			if ok {
				if normalized, notes := normalizeDataTaskPlanShape(repaired); len(notes) > 0 {
					logging.Info("[cli/data] normalized repaired data task plan: %s", strings.Join(notes, "; "))
					repaired = normalized
				}
				repaired = protectPlan(repaired)
				auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "repair", repairRounds, repaired)
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, repaired)
				currentPlan = repaired
				continue
			}
			return "", fmt.Errorf("data task planning: %s", errText)
		}
		dataRounds++
		dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "execute", dataRounds)
		var result dataquery.Result
		if len(currentPlan.Actions) > 0 {
			seededActionRunner := actionRunner
			seededActionRunner.Seed = dataTaskActionRunnerSeed(records)
			result, err = seededActionRunner.Run(ctx, currentPlan)
		} else {
			result, err = runner.Run(ctx, currentPlan)
		}
		if err != nil {
			if patched, ok, _, reason := tryPatchDataTaskResult(ctx, cfg.Planner, request, currentPlan, err, records, cfg.Language); ok {
				result = patched
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "patch", dataRounds, reason)
				err = nil
			}
		}
		if err != nil {
			errText := fmt.Sprintf("execute data task: %v", err)
			recordedErr := false
			if nodeKey, nodeCount, repeated := dataTaskRepeatedNodeFailure(records, errText, DefaultDataTaskMaxNodeFailures); repeated {
				if continuer, ok := cfg.Planner.(DataTaskContinuationPlanner); ok {
					records = append(records, dataTaskWorkflowRecordWithOptionalResult(currentPlan, result, errText))
					recordedErr = true
					dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, fmt.Sprintf("node %s failed %d times; expanding graph", nodeKey, nodeCount))
					nextPlan, contErr := continuer.ContinueDataTask(ctx, request, repoRoot, policy, dataTaskCandidatesWithWorkflowArtifacts(candidates, records), records)
					if contErr == nil {
						if normalized, notes := normalizeDataTaskPlanShape(nextPlan); len(notes) > 0 {
							logging.Info("[cli/data] normalized continuation data task plan: %s", strings.Join(notes, "; "))
							nextPlan = normalized
						}
						nextPlan = preserveDataTaskWorkflowMaterialCoverageForError(records, currentPlan, nextPlan, errText)
						nextPlan = protectPlan(nextPlan)
						auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, nextPlan)
						dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, nextPlan)
						currentPlan = nextPlan
						continue
					}
				}
			}
			var repaired dataquery.TaskPlan
			var ok bool
			repaired, repairRounds, ok, err = repairDataTaskPlanForCLI(ctx, cfg.Planner, request, repoRoot, policy, candidates, currentPlan, errText, records, repairRounds, repairRoundsMax)
			if !recordedErr {
				records = append(records, dataTaskWorkflowRecordWithOptionalResult(currentPlan, result, errText))
			}
			if err != nil {
				return "", err
			}
			if ok {
				if normalized, notes := normalizeDataTaskPlanShape(repaired); len(notes) > 0 {
					logging.Info("[cli/data] normalized repaired data task plan: %s", strings.Join(notes, "; "))
					repaired = normalized
				}
				repaired = protectPlan(repaired)
				auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "repair", repairRounds, repaired)
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, repaired)
				currentPlan = repaired
				continue
			}
			return "", fmt.Errorf("%s", errText)
		}
		if shouldValidateDataTaskWorkflowResult(currentPlan) {
			if gateErr := validateDataTaskWorkflowResult(records, currentPlan, result); gateErr != nil {
				errText := fmt.Sprintf("validate data workflow result: %v", gateErr)
				var repaired dataquery.TaskPlan
				var ok bool
				repaired, repairRounds, ok, err = repairDataTaskPlanForCLI(ctx, cfg.Planner, request, repoRoot, policy, candidates, currentPlan, errText, records, repairRounds, repairRoundsMax)
				records = append(records, dataTaskWorkflowRecordWithOptionalResult(currentPlan, result, errText))
				if err != nil {
					return "", err
				}
				if ok {
					if normalized, notes := normalizeDataTaskPlanShape(repaired); len(notes) > 0 {
						logging.Info("[cli/data] normalized repaired data task plan: %s", strings.Join(notes, "; "))
						repaired = normalized
					}
					repaired = protectPlan(repaired)
					auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "repair", repairRounds, repaired)
					dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
					dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, repaired)
					currentPlan = repaired
					continue
				}
				return "", fmt.Errorf("%s", errText)
			}
		}
		records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Result: &result})
		logDataTaskCLIResult(dataRounds, result)
		dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "result", dataRounds, dataTaskWorkflowResultSegment(cfg.Language, result))
		evaluator, evalOK := cfg.Planner.(DataTaskEvaluator)
		continuer, contOK := cfg.Planner.(DataTaskContinuationPlanner)
		if !evalOK {
			return dataTaskAnswerMarkdown(cfg.Language, result), nil
		}
		dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "evaluate", dataRounds)
		eval, err := evaluator.EvaluateDataTask(ctx, request, records, cfg.Language)
		if err != nil {
			return "", fmt.Errorf("evaluate data task: %w", err)
		}
		if len(records) > 0 {
			records[len(records)-1].Evaluation = &eval
		}
		switch eval.Status {
		case dataquery.EvalComplete:
			if errText := dataTaskWorkflowCompletionGateError(records, currentPlan, result); errText != "" {
				if completionPlan, ok := dataTaskRequiredLedgerCompletionPlan(records, currentPlan, result, errText); ok {
					completionPlan = protectPlan(completionPlan)
					auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "ledger", dataRounds+1, completionPlan)
					dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
					dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, completionPlan)
					currentPlan = completionPlan
					continue
				}
				var repaired dataquery.TaskPlan
				var ok bool
				repaired, repairRounds, ok, err = repairDataTaskPlanForCLI(ctx, cfg.Planner, request, repoRoot, policy, candidates, currentPlan, errText, records, repairRounds, repairRoundsMax)
				if len(records) > 0 {
					records[len(records)-1].Err = errText
				}
				if err != nil {
					return "", fmt.Errorf("%s\nrepair data task completion: %w", errText, err)
				}
				if ok {
					if normalized, notes := normalizeDataTaskPlanShape(repaired); len(notes) > 0 {
						logging.Info("[cli/data] normalized completion-repair data task plan: %s", strings.Join(notes, "; "))
						repaired = normalized
					}
					repaired = protectPlan(repaired)
					auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "repair", repairRounds, repaired)
					dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
					dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, repaired)
					currentPlan = repaired
					continue
				}
				return "", fmt.Errorf("%s", errText)
			}
			return dataTaskAnswerMarkdown(cfg.Language, result), nil
		case dataquery.EvalContinueData, dataquery.EvalExpandGraph, dataquery.EvalContinueTransform:
			if !contOK {
				return dataTaskAnswerMarkdown(cfg.Language, result), nil
			}
			nextPlan, err := continuer.ContinueDataTask(ctx, request, repoRoot, policy, dataTaskCandidatesWithWorkflowArtifacts(candidates, records), records)
			if err != nil {
				return "", fmt.Errorf("continue data task: %w", err)
			}
			if normalized, notes := normalizeDataTaskPlanShape(nextPlan); len(notes) > 0 {
				logging.Info("[cli/data] normalized continuation data task plan: %s", strings.Join(notes, "; "))
				nextPlan = normalized
			}
			nextPlan = preserveDataTaskWorkflowMaterialCoverage(records, currentPlan, nextPlan)
			nextPlan = protectPlan(nextPlan)
			auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, nextPlan)
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds)
			dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, nextPlan)
			currentPlan = nextPlan
			continue
		case dataquery.EvalRepairNode:
			repairer, repairOK := cfg.Planner.(DataTaskRepairPlanner)
			if !repairOK || repairRounds >= repairRoundsMax {
				return dataTaskAnswerMarkdown(cfg.Language, result), nil
			}
			repairRounds++
			repairReason := dataTaskEvaluationRepairReason(eval)
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, repairReason))
			repairedPlan, err := repairer.RepairDataTask(ctx, request, repoRoot, policy, dataTaskCandidatesWithWorkflowArtifacts(candidates, records), currentPlan, repairReason)
			if err != nil {
				return "", fmt.Errorf("repair data task node: %w", err)
			}
			repairedPlan = preserveDataTaskWorkflowMaterialCoverageForError(records, currentPlan, repairedPlan, repairReason)
			if normalized, notes := normalizeDataTaskPlanShape(repairedPlan); len(notes) > 0 {
				logging.Info("[cli/data] normalized repaired data task plan: %s", strings.Join(notes, "; "))
				repairedPlan = normalized
			}
			repairedPlan = protectPlan(repairedPlan)
			auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "repair", repairRounds, repairedPlan)
			dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, repairedPlan)
			currentPlan = repairedPlan
			continue
		case dataquery.EvalNeedsClarification:
			return dataTaskEvaluationMarkdown(cfg.Language, eval), nil
		case dataquery.EvalBlocked:
			return dataTaskEvaluationMarkdown(cfg.Language, eval), nil
		case dataquery.EvalBudgetExhausted, dataquery.EvalPartialAnswerPossible:
			return dataTaskAnswerMarkdown(cfg.Language, result), nil
		default:
			return dataTaskAnswerMarkdown(cfg.Language, result), nil
		}
	}
}

func terminalDataTaskPlanForCLI(plan dataquery.TaskPlan, records []dataTaskWorkflowRecord, lang string) (bool, string, error) {
	switch strings.ToLower(strings.TrimSpace(plan.Status)) {
	case "needs_clarification":
		return true, dataTaskClarificationMarkdown(lang, plan), nil
	case "blocked":
		return true, dataTaskBlockedMarkdown(lang, plan), nil
	case "complete", "completed", "partial_answer_possible", "budget_exhausted":
		if result, ok := latestDataTaskResult(records); ok {
			return true, dataTaskAnswerMarkdown(lang, result), nil
		}
		return true, "", fmt.Errorf("terminal data task plan ended without a result: %s", strings.TrimSpace(plan.Status))
	default:
		return false, "", nil
	}
}

func repairDataTaskPlanForCLI(ctx context.Context, planner DataTaskPlanner, request, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, currentPlan dataquery.TaskPlan, errText string, records []dataTaskWorkflowRecord, repairRounds, repairRoundsMax int) (dataquery.TaskPlan, int, bool, error) {
	repairer, ok := planner.(DataTaskRepairPlanner)
	if !ok || repairRounds >= repairRoundsMax {
		return dataquery.TaskPlan{}, repairRounds, false, nil
	}
	repairRounds++
	repairedPlan, err := repairer.RepairDataTask(ctx, request, repoRoot, policy, dataTaskCandidatesWithWorkflowArtifacts(candidates, records), currentPlan, errText)
	if err != nil {
		return dataquery.TaskPlan{}, repairRounds, false, fmt.Errorf("%s\nrepair data task: %w", errText, err)
	}
	repairedPlan = preserveDataTaskWorkflowMaterialCoverageForError(records, currentPlan, repairedPlan, errText)
	return repairedPlan, repairRounds, true, nil
}

func tryPatchDataTaskResult(ctx context.Context, planner DataTaskPlanner, userLine string, currentPlan dataquery.TaskPlan, err error, records []dataTaskWorkflowRecord, lang string) (dataquery.Result, bool, bool, string) {
	patcher, ok := planner.(DataTaskResultPatchPlanner)
	if !ok || err == nil {
		return dataquery.Result{}, false, false, ""
	}
	var validationErr *dataquery.DataResultValidationError
	if !errors.As(err, &validationErr) {
		return dataquery.Result{}, false, false, ""
	}
	if !dataTaskPatchCandidate(validationErr.Result, validationErr.Violations) {
		return dataquery.Result{}, false, false, ""
	}
	patchPlan, patchErr := patcher.ProposeDataResultPatch(ctx, userLine, currentPlan, validationErr.Result, validationErr.Violations, records, lang)
	if patchErr != nil {
		logging.Warning("[data] structural patch planning failed: %v", patchErr)
		return dataquery.Result{}, false, true, ""
	}
	if raw, err := json.MarshalIndent(patchPlan, "", "  "); err == nil {
		logging.Info("[data] structural patch proposal status=%s patches=%d reason=%s\n%s",
			patchPlan.Status, len(patchPlan.Patches), oneLineClamp(patchPlan.Reason, 240), string(raw))
	}
	if strings.ToLower(strings.TrimSpace(patchPlan.Status)) != "patch" || len(patchPlan.Patches) == 0 {
		logging.Info("[data] structural patch declined status=%s reason=%s", patchPlan.Status, oneLineClamp(patchPlan.Reason, 240))
		return dataquery.Result{}, false, true, ""
	}
	patched, patches, applyErr := dataquery.ApplyDataResultPatchPlan(currentPlan, validationErr.Result, patchPlan)
	if applyErr != nil {
		logging.Warning("[data] structural patch rejected: %v", applyErr)
		return dataquery.Result{}, false, true, ""
	}
	logging.Info("[data] structural patch applied patches=%d reason=%s", len(patches), oneLineClamp(patchPlan.Reason, 240))
	if isZh(lang) {
		return patched, true, true, fmt.Sprintf("结构修复 %d", len(patches))
	}
	return patched, true, true, fmt.Sprintf("%d structural patch(es)", len(patches))
}

func dataTaskPatchCandidate(result dataquery.Result, violations []dataquery.DataTaskViolation) bool {
	if len(violations) == 0 {
		return false
	}
	for _, v := range violations {
		switch strings.TrimSpace(v.Code) {
		case "missing_required_ledger", "reconcile_status_failed":
			continue
		}
		path := strings.TrimSpace(v.JSONPath)
		switch {
		case strings.HasPrefix(path, "/contributions/") && len(result.Contributions) > 0:
			return true
		case strings.HasPrefix(path, "/entity_resolutions/") && len(result.EntityResolutions) > 0:
			return true
		case strings.HasPrefix(path, "/reconcile/") && result.Reconcile != nil:
			return true
		}
	}
	return false
}

func logDataTaskCLIResult(round int, result dataquery.Result) {
	reconcileStatus := ""
	if result.Reconcile != nil {
		reconcileStatus = result.Reconcile.Status.String()
	}
	logging.Info("[cli/data] data task result round=%d answer_len=%d decisions=%d rules=%d contributions=%d resolutions=%d reconcile=%s consumed=%d warnings=%d",
		round,
		len(strings.TrimSpace(result.Answer)),
		len(result.Rows),
		len(result.RuleCoverage),
		len(result.Contributions),
		len(result.EntityResolutions),
		reconcileStatus,
		len(result.ConsumedPaths),
		len(result.ContractWarnings))
}

func auditDataTaskPlanForCLI(runtimeAnchor, repoRoot, scope string, round int, plan dataquery.TaskPlan) {
	dir := filepath.Join(firstNonEmptyString(strings.TrimSpace(runtimeAnchor), strings.TrimSpace(repoRoot), "."), "data-audit")
	if err := os.MkdirAll(dir, 0700); err != nil {
		logging.Warning("[cli/data] create audit dir failed: %v", err)
		return
	}
	stamp := time.Now().Format("20060102-150405")
	prefix := fmt.Sprintf("%s-%d-%s-r%d", stamp, os.Getpid(), dataTaskAuditNamePart(scope), round)
	planPath := filepath.Join(dir, prefix+".plan.json")
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		logging.Warning("[cli/data] marshal data task plan failed scope=%s round=%d: %v", scope, round, err)
	} else if err := os.WriteFile(planPath, raw, 0600); err != nil {
		logging.Warning("[cli/data] write data task plan audit failed path=%s: %v", planPath, err)
	} else {
		logging.Info("[cli/data] data task plan audit scope=%s round=%d path=%s", scope, round, planPath)
		logging.Info("[cli/data] data task plan full scope=%s round=%d path=%s\n%s", scope, round, planPath, string(raw))
	}
	if script := strings.TrimSpace(plan.Script); script != "" {
		scriptPath := filepath.Join(dir, prefix+".script.py")
		if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
			logging.Warning("[cli/data] write data task script audit failed path=%s: %v", scriptPath, err)
		} else {
			logging.Info("[cli/data] data task script full scope=%s round=%d path=%s\n%s", scope, round, scriptPath, script)
		}
	}
	for i, action := range plan.Actions {
		script := strings.TrimSpace(action.Script)
		if script == "" {
			continue
		}
		actionID := dataTaskAuditNamePart(firstNonEmptyString(action.ID, fmt.Sprintf("action-%d", i+1)))
		scriptPath := filepath.Join(dir, fmt.Sprintf("%s.action-%02d-%s.script.py", prefix, i+1, actionID))
		if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
			logging.Warning("[cli/data] write data task action script audit failed path=%s: %v", scriptPath, err)
		} else {
			logging.Info("[cli/data] data task action script full scope=%s round=%d action=%s path=%s\n%s", scope, round, firstNonEmptyString(action.ID, fmt.Sprintf("action_%d", i+1)), scriptPath, script)
		}
	}
}

func dataTaskCLIPlanProgress(w io.Writer, lang string, plan dataquery.TaskPlan) {
	if w == nil {
		return
	}
	label, segs := dataTaskPlanAuditSummary(plan, lang)
	cliLightRouteSummary(w, label, segs)
}

func dataTaskCLIWorkflowProgress(w io.Writer, lang, kind string, round int, details ...string) {
	if w == nil {
		return
	}
	label := "数据工作流"
	segs := []string{}
	if isZh(lang) {
		switch kind {
		case "execute":
			segs = append(segs, fmt.Sprintf("执行第 %d 批", round))
		case "repair":
			segs = append(segs, fmt.Sprintf("修复第 %d 次", round))
		case "patch":
			segs = append(segs, fmt.Sprintf("结构修复第 %d 批", round))
		case "result":
			segs = append(segs, fmt.Sprintf("结果第 %d 批", round))
		case "evaluate":
			segs = append(segs, fmt.Sprintf("评估第 %d 批", round))
		case "continue":
			segs = append(segs, fmt.Sprintf("继续第 %d 批", round+1))
		default:
			segs = append(segs, strings.TrimSpace(kind))
		}
		segs = append(segs, "未读源码")
	} else {
		label = "data workflow"
		switch kind {
		case "execute":
			segs = append(segs, fmt.Sprintf("execute batch %d", round))
		case "repair":
			segs = append(segs, fmt.Sprintf("repair %d", round))
		case "patch":
			segs = append(segs, fmt.Sprintf("structural patch batch %d", round))
		case "result":
			segs = append(segs, fmt.Sprintf("result batch %d", round))
		case "evaluate":
			segs = append(segs, fmt.Sprintf("evaluate batch %d", round))
		case "continue":
			segs = append(segs, fmt.Sprintf("continue batch %d", round+1))
		default:
			segs = append(segs, strings.TrimSpace(kind))
		}
		segs = append(segs, "no source read")
	}
	for _, detail := range details {
		if detail = strings.TrimSpace(detail); detail != "" {
			segs = append(segs, detail)
		}
	}
	cliLightRouteSummary(w, label, segs)
}

func cliLightRouteSummary(w io.Writer, label string, segs []string) {
	if w == nil {
		return
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return
	}
	clean := make([]string, 0, len(segs))
	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			clean = append(clean, seg)
		}
	}
	if len(clean) == 0 {
		fmt.Fprintf(w, "◇ %s\n", label)
		return
	}
	fmt.Fprintf(w, "◇ %s · %s\n", label, strings.Join(clean, " · "))
}
