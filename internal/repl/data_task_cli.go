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
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
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
	ResumePath      string
}

type dataTaskWorkflowResumeState struct {
	Records      []dataTaskWorkflowRecord
	CurrentPlan  dataquery.TaskPlan
	DeferredPlan dataquery.TaskPlan
	DataRounds   int
	RepairRounds int
}

func RunDataTaskCLI(ctx context.Context, request string, policy TurnPolicy, cfg DataTaskCLIConfig) (answer string, retErr error) {
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
	dataTaskCLIRequestProgress(cfg.Progress, cfg.Language, request)
	var records []dataTaskWorkflowRecord
	var dataRounds int
	var repairRounds int
	defer func() {
		if len(records) == 0 && dataRounds == 0 && repairRounds == 0 {
			return
		}
		status := "complete"
		reason := ""
		if retErr != nil {
			status = "failed"
			reason = retErr.Error()
		}
		var resultPtr *dataquery.Result
		if result, ok := latestDataTaskResult(records); ok {
			resultCopy := result
			resultPtr = &resultCopy
		}
		lastErr := dataTaskLatestError(records)
		resultSummary := dataTaskTerminalResultSummary(resultPtr, records)
		terminalPath := writeDataTaskTerminalArtifactFile(cfg.RuntimeAnchor, repoRoot, dataTaskTerminalAudit{
			Status:       status,
			Reason:       reason,
			DataRounds:   dataRounds,
			RepairRounds: repairRounds,
			Records:      records,
			Result:       resultPtr,
		}, status, reason, lastErr, resultSummary, "cli")
		logging.Info("[cli/data] terminal status=%s data_rounds=%d repair_rounds=%d records=%d result=%s reason=%q last_error=%q terminal_path=%s",
			status, dataRounds, repairRounds, len(records), resultSummary,
			oneLineClamp(reason, 500), oneLineClamp(lastErr, 500), terminalPath)
		emitDataTaskCLITerminalAuditPath(cfg.Progress, cfg.Language, status, terminalPath, reason)
	}()
	candidates, err := dataquery.DiscoverCandidateFiles(repoRoot, 240)
	if err != nil {
		return "", fmt.Errorf("data task discover files: %w", err)
	}
	var deferredPlan dataquery.TaskPlan
	var plan dataquery.TaskPlan
	var resumed bool
	var resumeCurrentPlan dataquery.TaskPlan
	if resumePath := strings.TrimSpace(cfg.ResumePath); resumePath != "" {
		resume, loadErr := loadDataTaskWorkflowResumeFile(resumePath)
		if loadErr != nil {
			return "", loadErr
		}
		records = resume.Records
		resumeCurrentPlan = resume.CurrentPlan
		deferredPlan = resume.DeferredPlan
		dataRounds = resume.DataRounds
		repairRounds = resume.RepairRounds
		resumed = true
		dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "resume", dataRounds, fmt.Sprintf("checkpoint %s", resumePath))
	}
	if !resumed {
		plan, err = cfg.Planner.PlanDataTask(ctx, request, repoRoot, policy, candidates)
		if err != nil {
			return "", fmt.Errorf("data task plan: %w", err)
		}
		if normalized, notes := normalizeDataTaskPlanShapeForPolicy(plan, policy); len(notes) > 0 {
			logging.Info("[cli/data] normalized initial data task plan: %s", strings.Join(notes, "; "))
			plan = normalized
		}
	}
	protectPlan := func(p dataquery.TaskPlan) dataquery.TaskPlan {
		return prepareDataTaskWorkflowPlanForExecution(request, candidates, records, p)
	}
	saveDeferredPlan := func(round int, remainder dataquery.TaskPlan, reason string) {
		deferredPlan = remainder
		if len(remainder.Actions) == 0 {
			return
		}
		auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "deferred", round, remainder)
		dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "deferred", round, dataTaskDeferredQueueSavedSegment(cfg.Language, remainder, reason))
	}
	discardDeferredPlan := func(round int, reason string) {
		if len(deferredPlan.Actions) > 0 {
			detail := dataTaskDeferredQueueDiscardedSegment(cfg.Language, deferredPlan, reason)
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "deferred", round, detail)
			first := deferredPlan.Actions[0]
			logging.Info("[cli/data] deferred data action queue discarded actions=%d first_action=%s:%s reason=%q", len(deferredPlan.Actions), first.ID, first.Kind, reason)
		}
		deferredPlan = dataquery.TaskPlan{}
	}
	acceptCandidatePlan := func(scope string, round int, candidate dataquery.TaskPlan) dataquery.TaskPlan {
		preflight := dataTaskPreflightWorkflowPlan(records, candidate, protectPlan)
		if preflight.Rewritten {
			records = append(records, dataTaskWorkflowRecord{Plan: preflight.Original, Err: preflight.GuardErr})
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", round, preflight.Reason)
			scope = "continue"
		}
		if strings.TrimSpace(preflight.FinalGuardErr) != "" {
			auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "rejected", round, preflight.Plan)
			logging.Info("[cli/data] data task candidate plan rejected scope=%s round=%d reason=%q", scope, round, preflight.FinalGuardErr)
			return preflight.Plan
		}
		if preflight.Rewritten {
			saveDeferredPlan(round, preflight.Remainder, preflight.Reason)
		}
		auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, scope, round, preflight.Plan)
		dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, preflight.Plan)
		return preflight.Plan
	}
	if resumed {
		plan, err = nextDataTaskPlanFromResumeForCLI(ctx, cfg.Planner, request, repoRoot, policy, candidates, records, resumeCurrentPlan, deferredPlan)
		if err != nil {
			return "", err
		}
		if normalized, notes := normalizeDataTaskPlanShapeForPolicy(plan, policy); len(notes) > 0 {
			logging.Info("[cli/data] normalized resumed data task plan: %s", strings.Join(notes, "; "))
			plan = normalized
		}
	}
	plan = acceptCandidatePlan("initial", 0, plan)

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
	for {
		if errText := dataTaskTerminalPlanCompletionGateErrorWithRepo(repoRoot, records, currentPlan); errText != "" {
			repaired, nextRepairRounds, ok, repairErr := repairDataTaskPlanForCLI(ctx, cfg.Planner, request, repoRoot, policy, candidates, currentPlan, errText, records, repairRounds, repairRoundsMax)
			repairRounds = nextRepairRounds
			if repairErr != nil {
				return "", repairErr
			}
			if ok {
				if normalized, notes := normalizeDataTaskPlanShapeForPolicy(repaired, policy); len(notes) > 0 {
					logging.Info("[cli/data] normalized terminal-completion repaired data task plan: %s", strings.Join(notes, "; "))
					repaired = normalized
				}
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
				currentPlan = acceptCandidatePlan("repair", repairRounds, repaired)
				continue
			}
			return "", fmt.Errorf("%s", errText)
		}
		if fallback, reason, ok := dataTaskTerminalWorkflowFallback(records, currentPlan); ok {
			records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: reason})
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, reason)
			fallback = protectPlan(fallback)
			auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
			dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
			currentPlan = fallback
			continue
		}
		if guard := dataTaskTerminalWorkflowGuardResult(records, currentPlan); !guard.Empty() {
			errText := guard.ErrorText()
			guardRecords := append(append([]dataTaskWorkflowRecord(nil), records...), dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
			writeDataTaskWorkflowCheckpointFile(cfg.RuntimeAnchor, repoRoot, guardRecords, currentPlan, deferredPlan, dataRounds, repairRounds, "terminal workflow guard blocked current plan", "cli", guard)
			repaired, nextRepairRounds, ok, repairErr := repairDataTaskPlanForCLI(ctx, cfg.Planner, request, repoRoot, policy, candidates, currentPlan, errText, records, repairRounds, repairRoundsMax)
			repairRounds = nextRepairRounds
			if repairErr != nil {
				return "", repairErr
			}
			records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
			if ok {
				if normalized, notes := normalizeDataTaskPlanShapeForPolicy(repaired, policy); len(notes) > 0 {
					logging.Info("[cli/data] normalized terminal-workflow repaired data task plan: %s", strings.Join(notes, "; "))
					repaired = normalized
				}
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
				currentPlan = acceptCandidatePlan("repair", repairRounds, repaired)
				continue
			}
			return "", fmt.Errorf("%s", errText)
		}
		if handled, answer, err := terminalDataTaskPlanForCLI(currentPlan, records, cfg.Language); handled {
			return answer, err
		}
		if dataRounds >= dataRoundsMax {
			if result, ok := latestDataTaskResult(records); ok {
				if errText := dataTaskWorkflowCompletionGateErrorWithRepo(repoRoot, records, currentPlan, result); errText != "" {
					return "", fmt.Errorf("data task workflow budget exhausted before final output: %s", errText)
				}
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
		if guard := dataTaskWorkflowStagingGuardResult(records, currentPlan); !guard.Empty() {
			errText := guard.ErrorText()
			guardRecords := append(append([]dataTaskWorkflowRecord(nil), records...), dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
			writeDataTaskWorkflowCheckpointFile(cfg.RuntimeAnchor, repoRoot, guardRecords, currentPlan, deferredPlan, dataRounds, repairRounds, "staging guard blocked current batch", "cli", guard)
			if fallback, remainder, reason, ok := dataTaskWorkflowDeterministicFallback(records, currentPlan, errText); ok {
				records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, reason)
				fallback = protectPlan(fallback)
				auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
				currentPlan = fallback
				saveDeferredPlan(dataRounds+1, remainder, reason)
				continue
			}
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
			if fallback, remainder, ok := dataTaskWorkflowStagePrefixFallbackWithRemainder(records, currentPlan, errText); ok {
				records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "trimmed multi-stage data plan to current DAG stage")
				fallback = protectPlan(fallback)
				auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
				currentPlan = fallback
				saveDeferredPlan(dataRounds+1, remainder, "trimmed multi-stage data plan to current DAG stage")
				continue
			}
			if fallback, ok := dataTaskInvalidRecordActionFallback(records, currentPlan, errText); ok {
				records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "converted invalid record action to bounded record extraction")
				fallback = protectPlan(fallback)
				auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
				currentPlan = fallback
				continue
			}
			if fallback, ok := dataTaskHistoricalMissingJoinFieldFallback(records, currentPlan); ok {
				records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "materialized historical missing join field from existing artifacts")
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
				if normalized, notes := normalizeDataTaskPlanShapeForPolicy(repaired, policy); len(notes) > 0 {
					logging.Info("[cli/data] normalized repaired data task plan: %s", strings.Join(notes, "; "))
					repaired = normalized
				}
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
				currentPlan = acceptCandidatePlan("repair", repairRounds, repaired)
				continue
			}
			return "", fmt.Errorf("data task planning: %s", errText)
		}
		dataRounds++
		dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "execute", dataRounds, dataTaskPlanAuditDetails(currentPlan, cfg.Language)...)
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
			discardDeferredPlan(dataRounds, fmt.Sprintf("execution failure: %s", clampDataTaskWorkflowText(errText, 240)))
			executionRecord := dataTaskWorkflowRecordWithOptionalResult(currentPlan, result, errText)
			if fallback, ok := dataTaskMissingJoinFieldFallback(append(records, executionRecord), currentPlan, errText); ok {
				records = append(records, executionRecord)
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "materialized missing join field from existing artifacts")
				fallback = protectPlan(fallback)
				auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
				currentPlan = fallback
				continue
			}
			if fallback, ok := dataTaskHistoricalMissingJoinFieldFallback(append(records, executionRecord), currentPlan); ok {
				records = append(records, executionRecord)
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "materialized historical missing join field from existing artifacts")
				fallback = protectPlan(fallback)
				auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
				currentPlan = fallback
				continue
			}
			recordedErr := false
			if nodeKey, nodeCount, repeated := dataTaskRepeatedNodeFailure(records, errText, DefaultDataTaskMaxNodeFailures); repeated {
				if continuer, ok := cfg.Planner.(DataTaskContinuationPlanner); ok {
					records = append(records, executionRecord)
					recordedErr = true
					dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, fmt.Sprintf("node %s failed %d times; expanding graph", nodeKey, nodeCount))
					nextPlan, contErr := continueDataTaskWithDeferredIfSupported(ctx, continuer, request, repoRoot, policy, dataTaskCandidatesWithWorkflowArtifacts(candidates, records), records, deferredPlan)
					if contErr == nil {
						if normalized, notes := normalizeDataTaskPlanShapeForPolicy(nextPlan, policy); len(notes) > 0 {
							logging.Info("[cli/data] normalized continuation data task plan: %s", strings.Join(notes, "; "))
							nextPlan = normalized
						}
						nextPlan = preserveDataTaskWorkflowMaterialCoverageForError(records, currentPlan, nextPlan, errText)
						currentPlan = acceptCandidatePlan("continue", dataRounds+1, nextPlan)
						continue
					}
				}
			}
			var repaired dataquery.TaskPlan
			var ok bool
			repaired, repairRounds, ok, err = repairDataTaskPlanForCLI(ctx, cfg.Planner, request, repoRoot, policy, candidates, currentPlan, errText, records, repairRounds, repairRoundsMax)
			if !recordedErr {
				records = append(records, executionRecord)
			}
			if err != nil {
				return "", err
			}
			if ok {
				if normalized, notes := normalizeDataTaskPlanShapeForPolicy(repaired, policy); len(notes) > 0 {
					logging.Info("[cli/data] normalized repaired data task plan: %s", strings.Join(notes, "; "))
					repaired = normalized
				}
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
				currentPlan = acceptCandidatePlan("repair", repairRounds, repaired)
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
					if normalized, notes := normalizeDataTaskPlanShapeForPolicy(repaired, policy); len(notes) > 0 {
						logging.Info("[cli/data] normalized repaired data task plan: %s", strings.Join(notes, "; "))
						repaired = normalized
					}
					dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
					currentPlan = acceptCandidatePlan("repair", repairRounds, repaired)
					continue
				}
				return "", fmt.Errorf("%s", errText)
			}
		}
		records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Result: &result})
		auditDataTaskResultForCLI(cfg.RuntimeAnchor, repoRoot, dataRounds, result)
		writeDataTaskWorkflowCheckpointFile(cfg.RuntimeAnchor, repoRoot, records, currentPlan, deferredPlan, dataRounds, repairRounds, "batch result completed", "cli")
		dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "result", dataRounds, append([]string{dataTaskWorkflowResultSegment(cfg.Language, result)}, dataTaskPlanAuditDetails(currentPlan, cfg.Language)...)...)
		if nextDeferred, remainingDeferred, ok := dataTaskPopDeferredActionBatch(records, deferredPlan); ok {
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "continuing deferred typed data action rank")
			nextDeferred = protectPlan(nextDeferred)
			auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, nextDeferred)
			dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, nextDeferred)
			currentPlan = nextDeferred
			saveDeferredPlan(dataRounds+1, remainingDeferred, "remaining deferred typed data action rank(s)")
			continue
		}
		if len(deferredPlan.Actions) > 0 {
			status := dataTaskDeferredQueueStatus(records, deferredPlan)
			if strings.TrimSpace(status.Reason) != "" {
				records = append(records, dataTaskWorkflowRecord{Plan: deferredPlan, Err: status.Reason})
			}
			discardDeferredPlan(dataRounds, dataTaskDeferredQueueBlockedSegment(cfg.Language, status))
		} else {
			deferredPlan = dataquery.TaskPlan{}
		}
		if fallback, ok := dataTaskCoverageExpansionFallbackAfterResult(records, currentPlan, "missing material coverage after data batch result"); ok {
			records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: "missing material coverage converted to atomic coverage batch after result"})
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "missing material coverage converted to atomic coverage batch")
			fallback = protectPlan(fallback)
			auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
			dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
			currentPlan = fallback
			continue
		}
		if fallback, reason, ok := dataTaskWorkflowNextStageFallbackWithRepo(repoRoot, records, currentPlan, "batch result completed"); ok {
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, reason)
			fallback = protectPlan(fallback)
			auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
			dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
			currentPlan = fallback
			continue
		}
		evaluator, evalOK := cfg.Planner.(DataTaskEvaluator)
		continuer, contOK := cfg.Planner.(DataTaskContinuationPlanner)
		if !evalOK {
			return dataTaskAnswerMarkdown(cfg.Language, result), nil
		}
		dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "evaluate", dataRounds, dataTaskPlanAuditDetails(currentPlan, cfg.Language)...)
		eval, err := evaluateDataTaskWithDeferredIfSupported(ctx, evaluator, request, records, deferredPlan, cfg.Language)
		if err != nil {
			return "", fmt.Errorf("evaluate data task: %w", err)
		}
		if len(records) > 0 {
			records[len(records)-1].Evaluation = &eval
		}
		switch eval.Status {
		case dataquery.EvalComplete:
			if errText := dataTaskWorkflowCompletionGateErrorWithRepo(repoRoot, records, currentPlan, result); errText != "" {
				if completionPlan, ok := dataTaskRequiredLedgerCompletionPlanWithRepo(repoRoot, records, currentPlan, result, errText); ok {
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
					if normalized, notes := normalizeDataTaskPlanShapeForPolicy(repaired, policy); len(notes) > 0 {
						logging.Info("[cli/data] normalized completion-repair data task plan: %s", strings.Join(notes, "; "))
						repaired = normalized
					}
					dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
					currentPlan = acceptCandidatePlan("repair", repairRounds, repaired)
					continue
				}
				return "", fmt.Errorf("%s", errText)
			}
			return dataTaskAnswerMarkdown(cfg.Language, result), nil
		case dataquery.EvalContinueData, dataquery.EvalExpandGraph, dataquery.EvalContinueTransform:
			if !contOK {
				return dataTaskAnswerMarkdown(cfg.Language, result), nil
			}
			nextPlan, err := continueDataTaskWithDeferredIfSupported(ctx, continuer, request, repoRoot, policy, dataTaskCandidatesWithWorkflowArtifacts(candidates, records), records, deferredPlan)
			if err != nil {
				if fallback, reason, ok := dataTaskDeterministicContinuationFallback(records, currentPlan, err); ok {
					fallback = protectPlan(fallback)
					auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
					dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, reason)
					dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
					currentPlan = fallback
					continue
				}
				return "", fmt.Errorf("continue data task: %w", err)
			}
			if normalized, notes := normalizeDataTaskPlanShapeForPolicy(nextPlan, policy); len(notes) > 0 {
				logging.Info("[cli/data] normalized continuation data task plan: %s", strings.Join(notes, "; "))
				nextPlan = normalized
			}
			nextPlan = preserveDataTaskWorkflowMaterialCoverage(records, currentPlan, nextPlan)
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds)
			currentPlan = acceptCandidatePlan("continue", dataRounds+1, nextPlan)
			continue
		case dataquery.EvalRepairNode:
			if fallback, ok := dataTaskHistoricalMissingJoinFieldFallback(records, currentPlan); ok {
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds, "materialized historical missing join field from existing artifacts")
				fallback = protectPlan(fallback)
				auditDataTaskPlanForCLI(cfg.RuntimeAnchor, repoRoot, "continue", dataRounds+1, fallback)
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, fallback)
				currentPlan = fallback
				continue
			}
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
			if normalized, notes := normalizeDataTaskPlanShapeForPolicy(repairedPlan, policy); len(notes) > 0 {
				logging.Info("[cli/data] normalized repaired data task plan: %s", strings.Join(notes, "; "))
				repairedPlan = normalized
			}
			currentPlan = acceptCandidatePlan("repair", repairRounds, repairedPlan)
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

func loadDataTaskWorkflowResumeFile(path string) (dataTaskWorkflowResumeState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return dataTaskWorkflowResumeState{}, fmt.Errorf("read data workflow checkpoint: %w", err)
	}
	var snapshot struct {
		DataRounds   int                                 `json:"data_rounds"`
		RepairRounds int                                 `json:"repair_rounds"`
		Resume       *dataworkflow.WorkflowResumePayload `json:"resume"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return dataTaskWorkflowResumeState{}, fmt.Errorf("parse data workflow checkpoint: %w", err)
	}
	if snapshot.Resume == nil || len(snapshot.Resume.Records) == 0 {
		return dataTaskWorkflowResumeState{}, fmt.Errorf("data workflow checkpoint has no resume payload: %s", path)
	}
	var out dataTaskWorkflowResumeState
	out.DataRounds = snapshot.DataRounds
	out.RepairRounds = snapshot.RepairRounds
	if err := json.Unmarshal(snapshot.Resume.Records, &out.Records); err != nil {
		return dataTaskWorkflowResumeState{}, fmt.Errorf("parse data workflow checkpoint records: %w", err)
	}
	if len(snapshot.Resume.CurrentPlan) > 0 {
		if err := json.Unmarshal(snapshot.Resume.CurrentPlan, &out.CurrentPlan); err != nil {
			return dataTaskWorkflowResumeState{}, fmt.Errorf("parse data workflow checkpoint current plan: %w", err)
		}
	}
	if len(snapshot.Resume.DeferredPlan) > 0 {
		if err := json.Unmarshal(snapshot.Resume.DeferredPlan, &out.DeferredPlan); err != nil {
			return dataTaskWorkflowResumeState{}, fmt.Errorf("parse data workflow checkpoint deferred plan: %w", err)
		}
	}
	if len(out.Records) == 0 {
		return dataTaskWorkflowResumeState{}, fmt.Errorf("data workflow checkpoint has no recorded data rounds: %s", path)
	}
	return out, nil
}

func nextDataTaskPlanFromResumeForCLI(ctx context.Context, planner DataTaskPlanner, request, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, records []dataTaskWorkflowRecord, current, deferred dataquery.TaskPlan) (dataquery.TaskPlan, error) {
	if nextDeferred, _, ok := dataTaskPopDeferredActionBatch(records, deferred); ok {
		return nextDeferred, nil
	}
	if fallback, _, ok := dataTaskTerminalWorkflowFallback(records, current); ok {
		return fallback, nil
	}
	if fallback, reason, ok := dataTaskWorkflowNextStageFallbackWithRepo(repoRoot, records, current, "resumed data workflow checkpoint"); ok {
		logging.Info("[cli/data] resume selected deterministic next-stage plan: %s", reason)
		return fallback, nil
	}
	if continuer, ok := planner.(DataTaskContinuationPlanner); ok {
		nextPlan, err := continueDataTaskWithDeferredIfSupported(ctx, continuer, request, repoRoot, policy, dataTaskCandidatesWithWorkflowArtifacts(candidates, records), records, deferred)
		if err == nil && dataTaskResumePlanHasShape(nextPlan) {
			return nextPlan, nil
		}
		if err == nil {
			logging.Info("[cli/data] resume continuation planner returned an empty plan shape; falling back to typed checkpoint state")
		}
		if fallback, reason, ok := dataTaskDeterministicContinuationFallback(records, current, err); ok {
			logging.Info("[cli/data] resume continuation planner failed; %s", reason)
			return fallback, nil
		}
		if err != nil {
			return dataquery.TaskPlan{}, fmt.Errorf("resume data workflow continuation: %w", err)
		}
	}
	if result, ok := latestDataTaskResult(records); ok {
		out := current
		out.Status = "complete"
		out.ContinueAfter = false
		out.BlockReason = "resumed checkpoint already has a terminal result and no continuation planner"
		if strings.TrimSpace(out.Goal) == "" {
			out.Goal = strings.TrimSpace(request)
		}
		if out.OutputContract.Format == "" {
			out.OutputContract = result.OutputContract
		}
		return out, nil
	}
	return dataquery.TaskPlan{}, fmt.Errorf("resume data workflow checkpoint cannot determine the next data action")
}

func dataTaskResumePlanHasShape(plan dataquery.TaskPlan) bool {
	if dataTaskPlanHasExecutableBatch(plan) || len(plan.Questions) > 0 {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(plan.Status))
	return dataTaskPlanStatusLooksTerminal(status) || status == "needs_clarification" || status == "blocked"
}

func repairDataTaskPlanForCLI(ctx context.Context, planner DataTaskPlanner, request, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, currentPlan dataquery.TaskPlan, errText string, records []dataTaskWorkflowRecord, repairRounds, repairRoundsMax int) (dataquery.TaskPlan, int, bool, error) {
	repairer, ok := planner.(DataTaskRepairPlanner)
	if !ok || repairRounds >= repairRoundsMax {
		return dataquery.TaskPlan{}, repairRounds, false, nil
	}
	repairRounds++
	repairedPlan, err := repairer.RepairDataTask(ctx, request, repoRoot, policy, dataTaskCandidatesWithWorkflowArtifacts(candidates, records), currentPlan, errText)
	if err != nil {
		if fallback, reason, ok, contErr := dataTaskRepairFailureContinuationFallbackForCLI(ctx, planner, request, repoRoot, policy, candidates, currentPlan, records, err); ok {
			logging.Info("[cli/data] repair planner failed structurally; %s", reason)
			return fallback, repairRounds, true, nil
		} else if contErr != nil {
			logging.Warning("[cli/data] repair planner continuation fallback failed: %v", contErr)
		}
		return dataquery.TaskPlan{}, repairRounds, false, fmt.Errorf("%s\nrepair data task: %w", errText, err)
	}
	repairedPlan = preserveDataTaskWorkflowMaterialCoverageForError(records, currentPlan, repairedPlan, errText)
	return repairedPlan, repairRounds, true, nil
}

func dataTaskRepairFailureContinuationFallbackForCLI(ctx context.Context, planner DataTaskPlanner, request, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, currentPlan dataquery.TaskPlan, records []dataTaskWorkflowRecord, repairErr error) (dataquery.TaskPlan, string, bool, error) {
	if !dataTaskRepairPlannerErrorAllowsContinuation(repairErr) || len(records) == 0 {
		return dataquery.TaskPlan{}, "", false, nil
	}
	continuer, ok := planner.(DataTaskContinuationPlanner)
	if !ok {
		return dataquery.TaskPlan{}, "", false, nil
	}
	nextPlan, err := continuer.ContinueDataTask(ctx, request, repoRoot, policy, dataTaskCandidatesWithWorkflowArtifacts(candidates, records), records)
	if err != nil {
		if fallback, reason, ok := dataTaskDeterministicContinuationFallback(records, currentPlan, err); ok {
			return fallback, reason, true, nil
		}
		return dataquery.TaskPlan{}, "", false, err
	}
	nextPlan = preserveDataTaskWorkflowMaterialCoverage(records, currentPlan, nextPlan)
	return nextPlan, "repair planner returned no structured plan; continued from typed workflow state", true, nil
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

func auditDataTaskResultForCLI(runtimeAnchor, repoRoot string, round int, result dataquery.Result) {
	dir := filepath.Join(firstNonEmptyString(strings.TrimSpace(runtimeAnchor), strings.TrimSpace(repoRoot), "."), "data-audit")
	if err := os.MkdirAll(dir, 0700); err != nil {
		logging.Warning("[cli/data] create audit dir failed: %v", err)
		logDataTaskCLIResult(round, result)
		return
	}
	stamp := dataTaskAuditStamp()
	resultPath := filepath.Join(dir, fmt.Sprintf("%s-%d-result-r%d.json", stamp, os.Getpid(), round))
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		logging.Warning("[cli/data] marshal data task result failed round=%d: %v", round, err)
		logDataTaskCLIResult(round, result)
		return
	}
	if err := os.WriteFile(resultPath, raw, 0600); err != nil {
		logging.Warning("[cli/data] write data task result audit failed path=%s: %v", resultPath, err)
		logDataTaskCLIResult(round, result)
		return
	}
	reconcileStatus := ""
	if result.Reconcile != nil {
		reconcileStatus = result.Reconcile.Status.String()
	}
	logging.Info("[cli/data] data task result round=%d answer_len=%d decisions=%d rules=%d contributions=%d resolutions=%d reconcile=%s consumed=%d warnings=%d result_path=%s",
		round,
		len(strings.TrimSpace(result.Answer)),
		len(result.Rows),
		len(result.RuleCoverage),
		len(result.Contributions),
		len(result.EntityResolutions),
		reconcileStatus,
		len(result.ConsumedPaths),
		len(result.ContractWarnings),
		resultPath)
	logging.Info("[cli/data] data task result full round=%d path=%s\n%s", round, resultPath, string(raw))
}

func dataTaskAuditStamp() string {
	now := time.Now()
	return fmt.Sprintf("%s-%06d", now.Format("20060102-150405"), now.Nanosecond()/1000)
}

func auditDataTaskPlanForCLI(runtimeAnchor, repoRoot, scope string, round int, plan dataquery.TaskPlan) {
	dir := filepath.Join(firstNonEmptyString(strings.TrimSpace(runtimeAnchor), strings.TrimSpace(repoRoot), "."), "data-audit")
	if err := os.MkdirAll(dir, 0700); err != nil {
		logging.Warning("[cli/data] create audit dir failed: %v", err)
		return
	}
	stamp := dataTaskAuditStamp()
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
	cliLightRouteDetails(w, dataTaskPlanAuditDetails(plan, lang))
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
		case "resume":
			segs = append(segs, fmt.Sprintf("恢复第 %d 批", maxInt(round, 1)))
		default:
			segs = append(segs, strings.TrimSpace(kind))
		}
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
		case "resume":
			segs = append(segs, fmt.Sprintf("resume batch %d", maxInt(round, 1)))
		default:
			segs = append(segs, strings.TrimSpace(kind))
		}
	}
	segs = append(segs, dataTaskWorkflowInlineSegments(kind, lang, details...)...)
	if isZh(lang) {
		segs = append(segs, "未读源码")
	} else {
		segs = append(segs, "no source read")
	}
	cliLightRouteSummary(w, label, segs)
	cliLightRouteDetails(w, dataTaskWorkflowDetailLines(kind, round, lang, details...))
}

func dataTaskCLIRequestProgress(w io.Writer, lang, request string) {
	if w == nil {
		return
	}
	request = strings.TrimSpace(request)
	if request == "" {
		return
	}
	if isZh(lang) {
		cliLightRouteSummary(w, "数据请求", []string{"已接收"})
		cliLightRouteDetails(w, []string{"问题：" + oneLineClamp(request, 1000)})
		return
	}
	cliLightRouteSummary(w, "data request", []string{"received"})
	cliLightRouteDetails(w, []string{"Request: " + oneLineClamp(request, 1000)})
}

func emitDataTaskCLITerminalAuditPath(w io.Writer, lang, status, terminalPath string, reason ...string) {
	if w == nil || strings.TrimSpace(terminalPath) == "" {
		return
	}
	label := "data audit"
	segs := []string{strings.TrimSpace(status), "terminal " + terminalPath}
	if isZh(lang) {
		label = "数据审计"
		segs = []string{strings.TrimSpace(status), "完整终态 " + terminalPath}
	}
	cliLightRouteSummary(w, label, cleanDataTaskStrings(segs))
	reasonText := ""
	if len(reason) > 0 {
		reasonText = strings.TrimSpace(reason[0])
	}
	if reasonText == "" || strings.EqualFold(strings.TrimSpace(status), "complete") {
		return
	}
	if isZh(lang) {
		cliLightRouteDetails(w, []string{"原因：" + oneLineClamp(reasonText, 1000)})
		return
	}
	cliLightRouteDetails(w, []string{"Reason: " + oneLineClamp(reasonText, 1000)})
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

func cliLightRouteDetails(w io.Writer, lines []string) {
	if w == nil || len(lines) == 0 {
		return
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fmt.Fprintf(w, "  · %s\n", line)
	}
}
