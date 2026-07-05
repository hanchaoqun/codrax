package repl

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
)

const (
	DefaultDataTaskMaxRepairRounds                 = 6
	DefaultDataTaskMaxDataRounds                   = 18
	DefaultDataTaskMaxNodeFailures                 = 2
	DefaultDataTaskMaxCustomTransformClassFailures = 3
	dataTaskMaxRepairRoundsCeiling                 = 12
	dataTaskMaxDataRoundsCeiling                   = 36

	dataTaskOneShotScriptLineSoftLimit   = 260
	dataTaskOneShotScriptLineHardLimit   = 420
	dataTaskComplexCustomScriptLineLimit = 180
	dataTaskOneShotRequiredMaterialLimit = 8
	dataTaskOneShotValidationLedgerLimit = 3
	dataTaskBroadMaterialDiscoveryLimit  = 12
	dataTaskMaxActionsPerBatch           = 4
	dataTaskExactExtractRecordLimit      = 100000
	dataTaskUserExplicitMaterialPurpose  = "user explicitly referenced this candidate material; keep coverage verifiable for the data goal"
)

func normalizeDataTaskMaxRepairRounds(value int) int {
	if value <= 0 {
		return DefaultDataTaskMaxRepairRounds
	}
	if value > dataTaskMaxRepairRoundsCeiling {
		return dataTaskMaxRepairRoundsCeiling
	}
	return value
}

func normalizeDataTaskMaxDataRounds(value int) int {
	if value <= 0 {
		return DefaultDataTaskMaxDataRounds
	}
	if value > dataTaskMaxDataRoundsCeiling {
		return dataTaskMaxDataRoundsCeiling
	}
	return value
}

func dataTaskRepeatedNodeFailure(records []dataTaskWorkflowRecord, currentErr string, limit int) (key string, count int, repeated bool) {
	return dataworkflow.RepeatedNodeFailureFromErrors(dataTaskWorkflowErrorTexts(records), currentErr, limit)
}

func dataTaskRepeatedFailureReplacementFallback(stateRecords []dataTaskWorkflowRecord, previousErrors []string, current dataquery.TaskPlan, errText string) (dataquery.TaskPlan, string, bool) {
	state := dataTaskWorkflowState(stateRecords, current)
	if len(state.ActionScaffold) == 0 || len(state.AllowedNextActions) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	return dataworkflow.BuildRepeatedFailureReplacementPlan(dataworkflow.RepeatedFailureReplacementPlanInput{
		Current:        current,
		Coverage:       dataTaskWorkflowCoverageContract(stateRecords, current),
		Output:         dataTaskWorkflowOutputContract(stateRecords, current),
		Scaffolds:      state.ActionScaffold,
		Facts:          state.Facts(),
		PreviousErrors: previousErrors,
		CurrentError:   errText,
		FailureLimit:   DefaultDataTaskMaxNodeFailures,
		SeenActionKeys: dataTaskWorkflowSeenActionKeys(stateRecords),
		ProgressEvents: dataTaskWorkflowProgressEvents(stateRecords),
		NoProgressStop: DefaultDataTaskMaxNodeFailures,
	})
}

type dataTaskRepairPlanResult struct {
	Plan           dataquery.TaskPlan
	FallbackReason string
}

type dataTaskContinuationPlanResult struct {
	Plan           dataquery.TaskPlan
	FallbackReason string
}

func dataTaskAcceptPlannerPlan(plan dataquery.TaskPlan, scope string) (dataquery.TaskPlan, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "data task planner"
	}
	decision := dataworkflow.DecidePlannerPlanResult(dataworkflow.PlannerPlanDecisionInput{
		Plan:          plan,
		FailurePrefix: scope + " returned no executable data plan",
	})
	if decision.Action == dataworkflow.PlannerPlanAccept {
		return decision.Plan, nil
	}
	return dataquery.TaskPlan{}, newDataTaskPlannerNoPlanShapeError(scope, decision.Reason)
}

func dataTaskRunRepairPlannerWithRuntimeView(ctx context.Context, repairer DataTaskRepairPlanner, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, currentPlan dataquery.TaskPlan, errText string, view dataTaskWorkflowRuntimeView) (dataquery.TaskPlan, error) {
	if !dataTaskPlanHasRuntimeShape(view.CurrentPlan) {
		view.CurrentPlan = currentPlan
	}
	if !dataTaskPlanHasRuntimeShape(view.CurrentPlan) {
		return dataquery.TaskPlan{}, newDataTaskPlannerNoPlanShapeError("data task repair planner", "data task repair planner has no current workflow plan")
	}
	repairedPlan, err := repairDataTaskWithViolationAndRuntimeView(ctx, repairer, userLine, repoRoot, policy, dataTaskCandidatesWithWorkflowArtifacts(candidates, view.Records), view.CurrentPlan, errText, dataTaskRepairViolationFromRecords(view.Records, errText), view)
	if err == nil {
		repairedPlan, err = dataTaskAcceptPlannerPlan(repairedPlan, "data task repair planner")
	}
	if err != nil {
		return dataquery.TaskPlan{}, err
	}
	return preserveDataTaskWorkflowMaterialCoverageForError(view.Records, view.CurrentPlan, repairedPlan, errText), nil
}

func dataTaskRunContinuationPlannerWithRuntimeView(ctx context.Context, continuer DataTaskContinuationPlanner, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, view dataTaskWorkflowRuntimeView, preserveErrText string) (dataTaskContinuationPlanResult, error) {
	if continuer == nil {
		return dataTaskContinuationPlanResult{}, fmt.Errorf("data task continuation planner is unavailable")
	}
	if !dataTaskPlanHasRuntimeShape(view.CurrentPlan) {
		return dataTaskContinuationPlanResult{}, newDataTaskPlannerNoPlanShapeError("data task continuation planner", "data task continuation planner has no current workflow plan")
	}
	nextPlan, err := continueDataTaskWithRuntimeViewIfSupported(ctx, continuer, userLine, repoRoot, policy, dataTaskCandidatesWithWorkflowArtifacts(candidates, view.Records), view)
	var errText string
	var fallback dataquery.TaskPlan
	var fallbackReason string
	var fallbackOK bool
	if err != nil {
		errText = err.Error()
		fallback, fallbackReason, fallbackOK = dataTaskDeterministicContinuationFallback(view.Records, view.CurrentPlan, err)
	}
	plannerDecision := dataworkflow.DecidePlannerPlanResult(dataworkflow.PlannerPlanDecisionInput{
		Plan:              nextPlan,
		ErrorText:         errText,
		FallbackPlan:      fallback,
		FallbackReason:    fallbackReason,
		FallbackAvailable: fallbackOK,
		FailurePrefix:     "continue data task",
	})
	switch plannerDecision.Action {
	case dataworkflow.PlannerPlanFallbackPlan:
		return dataTaskContinuationPlanResult{Plan: plannerDecision.Plan, FallbackReason: plannerDecision.Reason}, nil
	case dataworkflow.PlannerPlanFail:
		return dataTaskContinuationPlanResult{}, fmt.Errorf("%s", plannerDecision.Reason)
	}
	nextPlan = plannerDecision.Plan
	if strings.TrimSpace(preserveErrText) != "" {
		nextPlan = preserveDataTaskWorkflowMaterialCoverageForError(view.Records, view.CurrentPlan, nextPlan, preserveErrText)
	} else {
		nextPlan = preserveDataTaskWorkflowMaterialCoverage(view.Records, view.CurrentPlan, nextPlan)
	}
	return dataTaskContinuationPlanResult{Plan: nextPlan}, nil
}

func dataTaskRepairFailureContinuationAvailable(planner DataTaskPlanner, view dataTaskWorkflowRuntimeView, repairErr error) bool {
	_, continuationReady := planner.(DataTaskContinuationPlanner)
	transition := dataworkflow.BuildRepairFailureTransition(dataworkflow.RepairFailureTransitionInput{
		CurrentPlan:              view.CurrentPlan,
		Records:                  view.Records,
		NoStructuredRepairPlan:   dataTaskRepairPlannerErrorAllowsContinuation(repairErr),
		ContinuationPlannerReady: continuationReady,
		RepairFailureReasonCode:  "repair_planner_no_structured_plan",
	})
	return transition.Action == dataworkflow.RepairFailureNeedsContinuation
}

func dataTaskRepairFailureContinuationWithRuntimeView(ctx context.Context, planner DataTaskPlanner, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, view dataTaskWorkflowRuntimeView, repairErr error) (dataTaskRepairPlanResult, bool, bool, error) {
	continuer, continuationReady := planner.(DataTaskContinuationPlanner)
	transition := dataworkflow.BuildRepairFailureTransition(dataworkflow.RepairFailureTransitionInput{
		CurrentPlan:              view.CurrentPlan,
		Records:                  view.Records,
		NoStructuredRepairPlan:   dataTaskRepairPlannerErrorAllowsContinuation(repairErr),
		ContinuationPlannerReady: continuationReady,
		RepairFailureReasonCode:  "repair_planner_no_structured_plan",
	})
	if transition.Action != dataworkflow.RepairFailureNeedsContinuation {
		return dataTaskRepairPlanResult{}, false, false, nil
	}
	result, err := dataTaskRunContinuationPlannerWithRuntimeView(ctx, continuer, userLine, repoRoot, policy, candidates, view, "")
	if err != nil {
		return dataTaskRepairPlanResult{}, true, false, err
	}
	reason := firstNonEmptyString(result.FallbackReason, transition.Reason, "repair planner returned no structured plan; continued from typed workflow state")
	return dataTaskRepairPlanResult{
		Plan:           result.Plan,
		FallbackReason: reason,
	}, true, true, nil
}

func dataTaskExecutionFailureTransition(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, errText string, violation dataquery.DataTaskViolation) dataworkflow.ExecutionFailureTransition {
	failureRecord := dataTaskWorkflowRecordWithExecutionViolation(current, result, errText, violation)
	recordsWithFailure := append(append([]dataTaskWorkflowRecord(nil), records...), failureRecord)
	state := dataTaskWorkflowState(recordsWithFailure, current)
	return dataworkflow.BuildExecutionFailureTransition(dataworkflow.ExecutionFailureTransitionInput{
		Current:           current,
		Records:           records,
		FailureRecord:     failureRecord,
		ErrorText:         errText,
		Violation:         violation,
		Coverage:          dataTaskWorkflowCoverageContract(recordsWithFailure, current),
		Output:            dataTaskWorkflowOutputContract(recordsWithFailure, current),
		State:             state,
		SchemaProjections: dataTaskWorkflowArtifactSchemaProjections(recordsWithFailure),
		PreviousErrors:    dataTaskWorkflowErrorTexts(records),
		FailureLimit:      DefaultDataTaskMaxNodeFailures,
		SeenActionKeys:    dataTaskWorkflowSeenActionKeys(recordsWithFailure),
		ProgressEvents:    dataTaskWorkflowProgressEvents(recordsWithFailure),
		NoProgressStop:    DefaultDataTaskMaxNodeFailures,
	})
}

func dataTaskPlanStagingGuardError(plan dataquery.TaskPlan) string {
	return dataTaskPlanStagingGuardResult(plan).ErrorText()
}

func dataTaskPlanStagingGuardResult(plan dataquery.TaskPlan) dataworkflow.GuardResult {
	status := strings.ToLower(strings.TrimSpace(plan.Status))
	if status != "" && status != "ready" {
		return dataworkflow.GuardResult{}
	}
	if len(plan.Actions) > 0 {
		return dataTaskActionStagingGuardResult(plan)
	}
	if guard := dataTaskTextConstraintCoverageGuardResult(plan); !guard.Empty() {
		return guard
	}
	lines := dataTaskScriptLineCount(plan.Script)
	requiredMaterials := len(plan.CoverageContract.RequiredMaterials)
	validationLedgers := dataTaskValidationLedgerCount(plan.CoverageContract)
	inputs := len(plan.InputPaths)
	if guard := dataworkflow.PlanShapeGuardResult(dataworkflow.PlanShapeGuardInput{
		Status:                 plan.Status,
		HasActions:             len(plan.Actions) > 0,
		HasScript:              strings.TrimSpace(plan.Script) != "",
		ScriptLines:            lines,
		ScriptHasResultEmitter: dataTaskScriptHasResultEmitter(plan.Script),
		InputCount:             inputs,
		RequiredMaterialCount:  requiredMaterials,
		ValidationLedgerCount:  validationLedgers,
		ContinueAfter:          plan.ContinueAfter,
		SoftScriptLineLimit:    dataTaskOneShotScriptLineSoftLimit,
		HardScriptLineLimit:    dataTaskOneShotScriptLineHardLimit,
		RequiredMaterialLimit:  dataTaskOneShotRequiredMaterialLimit,
		ValidationLedgerLimit:  dataTaskOneShotValidationLedgerLimit,
	}); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTerminalRequiredMaterialSchedulingGuardResult(nil, plan); !guard.Empty() {
		return guard
	}
	return dataworkflow.GuardResult{}
}

func dataTaskWorkflowStagingGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	return dataTaskWorkflowStagingGuardResult(records, plan).ErrorText()
}

func dataTaskWorkflowStagingGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	status := strings.ToLower(strings.TrimSpace(plan.Status))
	if status != "" && status != "ready" {
		return dataworkflow.GuardResult{}
	}
	if len(plan.Actions) > 0 {
		return dataTaskWorkflowActionStagingGuardResult(records, plan)
	}
	if guard := dataTaskTextConstraintCoverageGuardResult(plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTerminalRequiredMaterialSchedulingGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	return dataTaskPlanStagingGuardResult(plan)
}

func dataTaskPreExecutionDecision(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.PreExecutionDecision {
	coveragePlan, coverageOK := dataTaskCoverageExpansionFallback(records, plan, "missing material coverage before execution")
	materialPlan, materialOK := dataTaskMaterialDiscoveryFallback(records, plan, "broad material custom action requires objective material discovery before execution")
	return dataworkflow.DecidePreExecution(dataworkflow.PreExecutionDecisionInput{
		Fallbacks: []dataworkflow.PreExecutionFallbackCandidate{
			{
				Source:    "coverage",
				Plan:      coveragePlan,
				Reason:    "missing material coverage converted to atomic coverage batch after result",
				Available: coverageOK,
			},
			{
				Source:    "material_discovery",
				Plan:      materialPlan,
				Reason:    "broad material custom action converted to material discovery",
				Available: materialOK,
			},
		},
		Guard: dataTaskWorkflowStagingGuardResult(records, plan),
	})
}

func dataTaskStagingGuardRecoveryDecision(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) dataworkflow.GuardRecoveryDecision {
	errText := guard.ErrorText()
	deterministicPlan, deterministicRemainder, deterministicReason, deterministicOK := dataTaskWorkflowDeterministicFallback(records, plan, guard)
	coveragePlan, coverageOK := dataTaskCoverageExpansionFallback(records, plan, errText)
	materialPlan, materialOK := dataTaskMaterialDiscoveryFallback(records, plan, errText)
	stagePrefixPlan, stagePrefixRemainder, stagePrefixOK := dataTaskWorkflowStagePrefixFallbackWithRemainder(records, plan, guard)
	invalidRecordPlan, invalidRecordOK := dataTaskInvalidRecordActionFallback(records, plan, guard)
	missingJoinPlan, missingJoinOK := dataTaskHistoricalMissingJoinFieldFallback(records, plan)
	return dataworkflow.DecideGuardRecovery(dataworkflow.GuardRecoveryDecisionInput{
		Guard: guard,
		Candidates: []dataworkflow.GuardRecoveryFallbackCandidate{
			{
				Source:    "deterministic",
				Plan:      deterministicPlan,
				Remainder: deterministicRemainder,
				Reason:    deterministicReason,
				Available: deterministicOK,
			},
			{
				Source:    "coverage",
				Plan:      coveragePlan,
				Reason:    "missing material coverage converted to atomic coverage batch",
				Available: coverageOK,
			},
			{
				Source:    "material_discovery",
				Plan:      materialPlan,
				Reason:    "broad material plan converted to material discovery",
				Available: materialOK,
			},
			{
				Source:    "stage_prefix",
				Plan:      stagePrefixPlan,
				Remainder: stagePrefixRemainder,
				Reason:    "trimmed multi-stage data plan to current DAG stage",
				Available: stagePrefixOK,
			},
			{
				Source:    "invalid_record_action",
				Plan:      invalidRecordPlan,
				Reason:    "converted invalid record action to bounded record extraction",
				Available: invalidRecordOK,
			},
			{
				Source:    "historical_missing_join_field",
				Plan:      missingJoinPlan,
				Reason:    "materialized historical missing join field from existing artifacts",
				Available: missingJoinOK,
			},
		},
	})
}

func dataTaskDataRoundBudgetDecisionWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, dataRounds, maxDataRounds int) (dataworkflow.DataRoundBudgetDecision, dataquery.Result, bool) {
	var result dataquery.Result
	hasResult := false
	var guard dataworkflow.GuardResult
	if latest, ok := latestDataTaskResult(records); ok {
		result = latest
		hasResult = true
		guard = dataTaskWorkflowCompletionGateGuardResultWithRepo(repoRoot, records, current, latest)
	}
	decision := dataworkflow.DecideDataRoundBudget(dataworkflow.DataRoundBudgetDecisionInput{
		DataRounds:      dataRounds,
		MaxDataRounds:   maxDataRounds,
		HasResult:       hasResult,
		CompletionGuard: guard,
	})
	return decision, result, hasResult
}

func dataTaskWorkflowPreRunDecisionWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, dataRounds, maxDataRounds int) (dataworkflow.WorkflowPreRunDecision, dataquery.Result, bool) {
	budgetDecision, result, hasResult := dataTaskDataRoundBudgetDecisionWithRepo(repoRoot, records, current, dataRounds, maxDataRounds)
	return dataworkflow.DecideWorkflowPreRun(dataworkflow.WorkflowPreRunDecisionInput{
		Current:          current,
		HasResult:        hasResult,
		TerminalWorkflow: dataTaskTerminalWorkflowDecision(records, current),
		Budget:           budgetDecision,
		PreExecution:     dataTaskPreExecutionDecision(records, current),
	}), result, hasResult
}

func dataTaskPostResultDecisionWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current, deferred dataquery.TaskPlan) dataworkflow.PostResultDecision {
	if latest, ok := latestDataTaskResult(records); ok && dataTaskResultStructurallyCompleteWithRepo(repoRoot, records, current, latest) {
		return dataworkflow.PostResultDecision{
			Action: dataworkflow.PostResultEvaluate,
			Source: "completion_gate",
			Reason: "latest typed result satisfies the final output contract",
		}
	}
	nextDeferred, remainder, deferredStatus, deferredReady := dataTaskPopDeferredActionBatchWithStatus(records, deferred)
	coveragePlan, coverageOK := dataTaskCoverageExpansionFallbackAfterResult(records, current, "missing material coverage after data batch result")
	nextStagePlan, nextStageReason, nextStageOK := dataTaskWorkflowNextStageFallbackWithRepo(repoRoot, records, current, "batch result completed")
	return dataworkflow.DecidePostResult(dataworkflow.PostResultDecisionInput{
		DeferredDispatchAvailable: deferredReady,
		DeferredPlan:              firstExecutableTaskPlan(nextDeferred, deferred),
		DeferredRemainder:         remainder,
		DeferredStatus:            deferredStatus,
		Fallbacks: []dataworkflow.PostResultFallbackCandidate{
			{
				Source:    "coverage",
				Plan:      coveragePlan,
				Reason:    "missing material coverage converted to atomic coverage batch",
				Available: coverageOK,
			},
			{
				Source:    "next_stage",
				Plan:      nextStagePlan,
				Reason:    nextStageReason,
				Available: nextStageOK,
			},
		},
	})
}

func dataTaskEvaluationDecisionWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, eval dataquery.Evaluation, continuationReady, repairReady bool, repairRounds, repairRoundsMax int) dataworkflow.EvaluationDecision {
	var completionFallback dataworkflow.EvaluationFallbackCandidate
	// Completion single authority (DL-C, ledger §7.12): the completion
	// check and its guard consume the SAME typed answer selection the
	// terminal face publishes (selectDataTaskTerminalAnswer) — not a
	// separate reading of the literal latest batch. When the latest
	// batch is an answerless helper round while an earlier record
	// already holds the contract-satisfying answer, evaluating
	// completion against the helper burned repair/continuation rounds
	// on an already-satisfied workflow. A contested fallback answer
	// (latest evaluation actively contests the answer face and does not
	// pre-date the record) never satisfies completion; DecideEvaluation
	// additionally routes any actionable repair target to the repair
	// lane even when completion is satisfied.
	answerPlan, answerResult := current, result
	{
		contract := dataTaskWorkflowCoverageContract(records, current)
		output := dataTaskWorkflowOutputContract(records, current)
		if sel := selectDataTaskTerminalAnswer(records, current, result, contract, output); sel.FromFallback && !sel.Contested {
			answerPlan, answerResult = sel.Plan, sel.Result
		}
	}
	guard := dataTaskWorkflowCompletionGateGuardResultWithRepo(repoRoot, records, answerPlan, answerResult)
	completionSatisfied := dataTaskResultStructurallyCompleteWithRepo(repoRoot, records, answerPlan, answerResult)
	if !completionSatisfied &&
		dataTaskEvaluationStatusAllowsCompletionGateOverride(eval.Status) &&
		guard.Empty() &&
		dataworkflow.ResultHasAssembleAnswerArtifact(answerResult) {
		completionSatisfied = true
	}
	if !guard.Empty() {
		transition := dataTaskValidationFailureTransitionWithRepo(repoRoot, records, answerPlan, answerResult, guard)
		if transition.Action == dataworkflow.ValidationFailureFallbackPlan && transition.HasPlan() {
			completionFallback = dataworkflow.EvaluationFallbackCandidate{
				Source:    "ledger",
				Plan:      transition.Plan,
				Reason:    guard.ErrorText(),
				Available: true,
			}
		}
	}
	var repairFallback dataworkflow.EvaluationFallbackCandidate
	if eval.Status == dataquery.EvalRepairNode {
		if plan, ok := dataTaskHistoricalMissingJoinFieldFallback(records, current); ok {
			repairFallback = dataworkflow.EvaluationFallbackCandidate{
				Source:    "historical_missing_join_field",
				Plan:      plan,
				Reason:    "materialized historical missing join field from existing artifacts",
				Available: true,
			}
		}
	}
	var workflowFallback dataworkflow.EvaluationFallbackCandidate
	if dataTaskEvaluationStatusLooksTerminal(eval.Status) && eval.Status != dataquery.EvalComplete {
		if plan, ok := dataTaskCoverageExpansionFallbackAfterResult(records, current, "terminal evaluation ended before local material coverage"); ok {
			workflowFallback = dataworkflow.EvaluationFallbackCandidate{
				Source:    "coverage",
				Plan:      plan,
				Reason:    "terminal evaluation converted to atomic material coverage batch",
				Available: true,
			}
		} else if plan, reason, ok := dataTaskWorkflowNextStageFallbackWithRepo(repoRoot, records, current, "terminal evaluation ended before workflow completion"); ok {
			workflowFallback = dataworkflow.EvaluationFallbackCandidate{
				Source:    "next_stage",
				Plan:      plan,
				Reason:    reason,
				Available: true,
			}
		}
	}
	return dataworkflow.DecideEvaluation(dataworkflow.EvaluationDecisionInput{
		Evaluation:               eval,
		CompletionSatisfied:      completionSatisfied,
		CompletionGuard:          guard,
		CompletionFallback:       completionFallback,
		RepairFallback:           repairFallback,
		WorkflowFallback:         workflowFallback,
		ContinuationPlannerReady: continuationReady,
		RepairPlannerReady:       repairReady,
		RepairBudgetAvailable:    repairRounds < repairRoundsMax,
	})
}

func dataTaskResultStructurallyCompleteWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) bool {
	if err := validateDataTaskWorkflowResult(records, current, result); err != nil {
		return false
	}
	contract := dataTaskWorkflowCoverageContract(records, current)
	output := dataTaskWorkflowOutputContract(records, current)
	if !dataworkflow.ResultIsFinalAnswerCandidate(current, result, contract, output) {
		return false
	}
	if !dataworkflow.ResultHasAssembleAnswerArtifact(result) &&
		!dataworkflow.ResultIsPreservedAnswerHandoffCandidate(current, result, output) {
		return false
	}
	if dataTaskResultNeedsOutputProjection(records, current, result) {
		return false
	}
	if _, _, gapDeclared, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, result); hasReferenceGap && gapDeclared {
		return false
	}
	return true
}

func dataTaskEvaluationStatusLooksTerminal(status dataquery.EvaluationStatus) bool {
	switch status {
	case dataquery.EvalNeedsClarification, dataquery.EvalBlocked, dataquery.EvalBudgetExhausted, dataquery.EvalPartialAnswerPossible:
		return true
	default:
		return false
	}
}

func dataTaskEvaluationStatusAllowsCompletionGateOverride(status dataquery.EvaluationStatus) bool {
	switch status {
	case dataquery.EvalRepairNode,
		dataquery.EvalNeedsClarification,
		dataquery.EvalBlocked,
		dataquery.EvalBudgetExhausted,
		dataquery.EvalPartialAnswerPossible:
		return true
	default:
		return false
	}
}

func firstExecutableTaskPlan(first, fallback dataquery.TaskPlan) dataquery.TaskPlan {
	if dataTaskPlanHasRuntimeShape(first) || len(first.Actions) > 0 || len(first.InputPaths) > 0 || strings.TrimSpace(first.Script) != "" {
		return first
	}
	return fallback
}

func dataTaskActionStagingGuardResult(plan dataquery.TaskPlan) dataworkflow.GuardResult {
	if guard := dataTaskActionBatchShapeGuardResult(nil, plan, dataworkflow.ActionBatchShapeChecks{
		TopLevelScript: true,
		ActionCount:    true,
	}, false); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTextConstraintCoverageGuardResult(plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTerminalRequiredMaterialSchedulingGuardResult(nil, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionBatchShapeGuardResult(nil, plan, dataworkflow.ActionBatchShapeChecks{
		MultipleCustomScript: true,
	}, false); !guard.Empty() {
		return guard
	}
	for i, action := range plan.Actions {
		if guard := dataTaskActionDependencyGuardResult(nil, plan, action, i); !guard.Empty() {
			return guard
		}
		if guard := dataTaskSingleActionShapeGuardResult(nil, plan, action, i, false); !guard.Empty() {
			return guard
		}
	}
	if guard := dataTaskRuleCoveragePrerequisiteGuardResult(nil, plan); !guard.Empty() {
		return guard
	}
	return dataworkflow.GuardResult{}
}

func dataTaskWorkflowActionStagingGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	if guard := dataTaskActionBatchShapeGuardResult(records, plan, dataworkflow.ActionBatchShapeChecks{
		TopLevelScript: true,
		ActionCount:    true,
	}, true); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTextConstraintCoverageGuardResult(plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTerminalRequiredMaterialSchedulingGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskWorkflowCustomTransformDisabledGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionBatchShapeGuardResult(records, plan, dataworkflow.ActionBatchShapeChecks{
		MultipleCustomScript: true,
	}, true); !guard.Empty() {
		return guard
	}
	if guard := dataTaskCoverageLoopGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskWorkflowAllowedNextActionGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskWorkflowStageProgressGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskWorkflowRelationNoProgressGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskWorkflowNumericConstantReuseGuardResult(plan); !guard.Empty() {
		return guard
	}
	for i, action := range plan.Actions {
		if guard := dataTaskActionDependencyGuardResult(records, plan, action, i); !guard.Empty() {
			return guard
		}
		if guard := dataworkflow.RepeatedCustomTransformGuardResult(action, dataTaskWorkflowErrorTexts(records), DefaultDataTaskMaxNodeFailures); !guard.Empty() {
			return guard
		}
		if guard := dataworkflow.RepeatedCustomTransformClassGuardResult(
			action,
			dataTaskActionHasBroadPrerequisiteSurface(plan, action) || dataTaskActionLooksLikeWholeWorkflow(plan, action, i),
			dataTaskCustomTransformFailureErrors(records),
			DefaultDataTaskMaxCustomTransformClassFailures,
			DefaultDataTaskMaxNodeFailures,
		); !guard.Empty() {
			return guard
		}
		if guard := dataTaskTerminalRawMaterialCustomTransformGuardResult(records, plan, action, i, dataTaskScriptLineCount(action.Script)); !guard.Empty() {
			return guard
		}
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform && dataTaskActionHasBroadPrerequisiteSurface(plan, action) {
			if guard := dataTaskBroadCustomPrerequisiteGuardResult(records, plan, action, i); !guard.Empty() {
				return guard
			}
		}
		if guard := dataTaskSingleActionShapeGuardResult(records, plan, action, i, true); !guard.Empty() {
			return guard
		}
	}
	if guard := dataTaskRuleCoveragePrerequisiteGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	return dataworkflow.GuardResult{}
}

func dataTaskActionBatchShapeGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, checks dataworkflow.ActionBatchShapeChecks, workflowAware bool) dataworkflow.GuardResult {
	return dataworkflow.ActionBatchShapeGuardResult(dataworkflow.ActionBatchShapeGuardInput{
		Plan:                         plan,
		Checks:                       checks,
		TopLevelScriptLines:          dataTaskScriptLineCount(plan.Script),
		MaxActionsPerBatch:           dataTaskMaxActionsPerBatch,
		CustomScriptActionCount:      dataTaskCustomScriptActionCount(plan.Actions),
		RequiredMaterialCount:        len(plan.CoverageContract.RequiredMaterials),
		ValidationLedgerCount:        dataTaskValidationLedgerCount(plan.CoverageContract),
		ComplexCustomScriptLineLimit: dataTaskComplexCustomScriptLineLimit,
		OneShotScriptLineSoftLimit:   dataTaskOneShotScriptLineSoftLimit,
		ActionFacts:                  dataTaskActionShapeFacts(records, plan, plan.Actions),
		WorkflowAware:                workflowAware,
	})
}

func dataTaskSingleActionShapeGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int, workflowAware bool) dataworkflow.GuardResult {
	facts := dataTaskActionShapeFacts(records, plan, []dataquery.DataAction{action})
	if len(facts) > 0 {
		facts[0].ActionIndex = actionIndex
	}
	return dataworkflow.ActionBatchShapeGuardResult(dataworkflow.ActionBatchShapeGuardInput{
		Plan:                         plan,
		Checks:                       dataworkflow.ActionBatchShapeChecks{ActionScripts: true},
		RequiredMaterialCount:        len(plan.CoverageContract.RequiredMaterials),
		ValidationLedgerCount:        dataTaskValidationLedgerCount(plan.CoverageContract),
		ComplexCustomScriptLineLimit: dataTaskComplexCustomScriptLineLimit,
		OneShotScriptLineSoftLimit:   dataTaskOneShotScriptLineSoftLimit,
		ActionFacts:                  facts,
		WorkflowAware:                workflowAware,
	})
}

func dataTaskActionShapeFacts(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, actions []dataquery.DataAction) []dataworkflow.ActionShapeFact {
	out := make([]dataworkflow.ActionShapeFact, 0, len(actions))
	for offset, action := range actions {
		out = append(out, dataworkflow.ActionShapeFact{
			Action:                 action,
			ActionIndex:            offset,
			ScriptLines:            dataTaskScriptLineCount(action.Script),
			HasResultEmitter:       dataTaskScriptHasResultEmitter(action.Script),
			LooksLikeWholeWorkflow: dataTaskActionLooksLikeWholeWorkflow(plan, action, offset),
			BroadPrereqSurface:     dataTaskActionHasBroadPrerequisiteSurface(plan, action),
			RawInputCount:          len(dataTaskCustomTransformRawMaterialInputs(records, action)),
		})
	}
	return out
}

func dataTaskWorkflowDeterministicFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (fallback dataquery.TaskPlan, remainder dataquery.TaskPlan, reason string, ok bool) {
	errText := guard.ErrorText()
	if dataTaskGuardHasCode(guard, "text_constraint_coverage_required", "text_constraint_rule_coverage_required", "rule_coverage_prerequisite_missing") {
		contract := dataTaskWorkflowCoverageContract(records, plan)
		action := dataworkflow.RuleCoverageCompletionAction(contract)
		if strings.TrimSpace(action.ID) != "" {
			out := plan
			out.Status = "ready"
			out.Script = ""
			out.Actions = []dataquery.DataAction{action}
			out.InputPaths = mergeDataTaskInputPaths(out.InputPaths, action.InputPaths)
			out.ContinueAfter = true
			out.NextBatch = "continue with typed data processing after rule coverage materializes"
			out.WhyThisBatch = "complete source-backed rule coverage requested by the structural data validator"
			return out, dataquery.TaskPlan{}, "converted missing rule coverage to a typed derive_rules batch", true
		}
	}
	if fb, rem, hit := dataTaskInitialRankPrefixFallback(records, plan); hit {
		return fb, rem, "split initial data plan at typed dependency rank", true
	}
	if fb, rem, hit := dataTaskIntraBatchDependencyPrefixFallback(plan); hit {
		return fb, rem, "split data plan at intra-batch artifact dependency", true
	}
	if fb, hit := dataTaskNoEmitterScriptObservationFallback(records, plan, errText); hit {
		return fb, dataquery.TaskPlan{}, "converted exploratory script to atomic material observation batch", true
	}
	if fb, hit := dataTaskCoverageExpansionFallback(records, plan, errText); hit {
		return fb, dataquery.TaskPlan{}, "missing material coverage converted to atomic coverage batch", true
	}
	if fb, hit := dataTaskMaterialDiscoveryFallback(records, plan, errText); hit {
		return fb, dataquery.TaskPlan{}, "broad material plan converted to material discovery", true
	}
	if fb, hit := dataTaskCustomTransformDisabledFallback(records, plan, guard); hit {
		return fb, dataquery.TaskPlan{}, "custom_transform disabled plan converted to typed workflow fallback", true
	}
	if fb, rem, hit := dataTaskWorkflowStagePrefixFallbackWithRemainder(records, plan, guard); hit {
		return fb, rem, "trimmed multi-stage data plan to current DAG stage", true
	}
	if fb, hit := dataTaskExecutablePrefixFallback(records, plan, guard); hit {
		return fb, dataquery.TaskPlan{}, "executed valid typed prefix and discarded invalid suffix for replanning", true
	}
	if fb, hit := dataTaskInvalidRecordActionFallback(records, plan, guard); hit {
		return fb, dataquery.TaskPlan{}, "converted invalid record action to bounded record extraction", true
	}
	if fb, hit := dataTaskHistoricalMissingJoinFieldFallback(records, plan); hit {
		return fb, dataquery.TaskPlan{}, "materialized historical missing join field from existing artifacts", true
	}
	return dataquery.TaskPlan{}, dataquery.TaskPlan{}, "", false
}

func dataTaskGuardHasCode(guard dataworkflow.GuardResult, codes ...string) bool {
	allowed := map[string]bool{}
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code != "" {
			allowed[code] = true
		}
	}
	if len(allowed) == 0 {
		return false
	}
	if allowed[strings.TrimSpace(guard.Code)] {
		return true
	}
	for _, violation := range guard.Violations {
		if allowed[strings.TrimSpace(violation.Code)] {
			return true
		}
	}
	return false
}

func dataTaskInitialRankPrefixFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	return dataworkflow.InitialRankPrefixFallback(dataworkflow.InitialRankPrefixFallbackInput{
		Plan:                plan,
		HasSuccessfulResult: dataTaskWorkflowHasSuccessfulResult(records),
	})
}

func dataTaskIntraBatchDependencyPrefixFallback(plan dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	return dataworkflow.IntraBatchDependencyPrefixFallback(plan)
}

func dataTaskFirstIntraBatchDependency(actions []dataquery.DataAction) (int, string, dataquery.DataAction, bool) {
	return dataworkflow.FirstIntraBatchDependency(actions)
}

func dataTaskNoEmitterScriptObservationFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	if len(plan.Actions) > 0 || strings.TrimSpace(plan.Script) == "" || dataTaskScriptHasResultEmitter(plan.Script) {
		return dataquery.TaskPlan{}, false
	}
	lines := dataTaskScriptLineCount(plan.Script)
	if lines == 0 {
		return dataquery.TaskPlan{}, false
	}
	paths := dataTaskDiscoveryPaths(plan)
	requiredMaterials := len(plan.CoverageContract.RequiredMaterials)
	validationLedgers := dataTaskValidationLedgerCount(plan.CoverageContract)
	complex := len(paths) >= 3 || requiredMaterials >= 3 || validationLedgers >= 2
	if !complex {
		return dataquery.TaskPlan{}, false
	}
	state := dataTaskWorkflowState(records, plan)
	if state.MaterialCoverageSufficient && len(records) > 0 {
		return dataquery.TaskPlan{}, false
	}
	var ruleInputs, structuredInputs, inspectInputs []string
	for _, p := range paths {
		switch {
		case dataTaskPathLooksLikeTextConstraintMaterial(p) && dataTaskPlanShouldDeriveRulesForTextCoverage(plan):
			ruleInputs = append(ruleInputs, p)
		case dataTaskPathLooksLikeStructuredMaterial(p):
			structuredInputs = append(structuredInputs, p)
		default:
			inspectInputs = append(inspectInputs, p)
		}
	}
	var actions []dataquery.DataAction
	if len(ruleInputs) > 0 {
		actions = append(actions, dataquery.DataAction{
			ID:             "observe_rules",
			Kind:           dataquery.DataActionDeriveRules,
			Purpose:        "derive generic rule records from rule or constraint materials before calculation",
			InputPaths:     cleanDataTaskStrings(ruleInputs),
			OutputArtifact: "observed_rules.json",
		})
	}
	if len(structuredInputs) > 0 {
		actions = append(actions, dataquery.DataAction{
			ID:             "observe_records",
			Kind:           dataquery.DataActionExtractRecords,
			Purpose:        "extract bounded generic record samples before choosing the next typed data action",
			InputPaths:     cleanDataTaskStrings(structuredInputs),
			OutputArtifact: "observed_records.json",
			Params:         map[string]string{"limit": "120"},
		})
	}
	if len(inspectInputs) > 0 {
		actions = append(actions, dataquery.DataAction{
			ID:             "observe_materials",
			Kind:           dataquery.DataActionInspectMaterial,
			Purpose:        "inspect non-structured materials before choosing the next typed data action",
			InputPaths:     cleanDataTaskStrings(inspectInputs),
			OutputArtifact: "observed_materials.json",
		})
	}
	if len(actions) == 0 {
		return dataquery.TaskPlan{}, false
	}
	if len(actions) > dataTaskMaxActionsPerBatch {
		actions = actions[:dataTaskMaxActionsPerBatch]
	}
	contract := dataTaskWorkflowCoverageContract(records, plan)
	if strings.TrimSpace(errText) != "" {
		contract.ValidationRules = mergeDataTaskValidationRules(
			contract.ValidationRules,
			[]string{"previous exploratory script had no result emitter and was converted into typed observation actions: " + oneLineClamp(errText, 240)},
		)
	}
	out := plan
	out.Status = "ready"
	out.Script = ""
	out.Actions = actions
	out.InputPaths = paths
	out.OutputContract = dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true}
	out.CoverageContract = contract
	out.ContinueAfter = true
	out.WhyThisBatch = "observe objective material and rule structure before later bounded calculation actions"
	out.NextBatch = "continue with typed data workflow actions over the observed artifacts"
	if strings.TrimSpace(out.Goal) == "" {
		out.Goal = "observe data task materials"
	}
	return out, true
}

func dataTaskRepairPlannerErrorAllowsContinuation(err error) bool {
	return dataTaskPlannerErrorHasCode(err, dataTaskPlannerErrorNoToolCall) ||
		dataTaskPlannerErrorHasCode(err, dataTaskPlannerErrorNoPlanShape)
}

func normalizeDataTaskPlanShape(plan dataquery.TaskPlan) (dataquery.TaskPlan, []string) {
	var reasons []string
	if normalizeDataTaskCompleteStatusWithExecutablePayload(&plan) {
		reasons = append(reasons, "changed status=complete to ready because the plan contains executable script/actions")
	}
	if normalizeDataTaskPlanPathLists(&plan) {
		reasons = append(reasons, "normalized comma-separated path lists in data plan fields")
	}
	if dataworkflow.NormalizeRolePathActionInputs(&plan) {
		reasons = append(reasons, "filled typed action input_paths from role path params")
	}
	if normalizeDataTaskMaterializationActionInputs(&plan) {
		reasons = append(reasons, "copied batch input_paths into materialization actions with empty input_paths")
	}
	if normalizeDataTaskCustomActionScriptInputs(&plan) {
		reasons = append(reasons, "added top-level declared inputs referenced by custom_transform scripts")
	}
	if normalizeDataTaskDeriveRulesInputs(&plan) {
		reasons = append(reasons, "filled derive_rules input_paths from required rule/constraint materials")
	}
	if normalizeDataTaskPlanContractFromActions(&plan) {
		reasons = append(reasons, "enabled validation contracts implied by typed data actions such as derive_rules/compute_contributions/reconcile_artifacts")
	}
	if len(plan.Actions) > 0 && strings.TrimSpace(plan.Script) != "" && !dataTaskScriptHasResultEmitter(plan.Script) {
		plan.Script = ""
		reasons = append(reasons, "removed non-emitting top-level script from an actions[] plan")
	}
	if len(plan.Actions) == 0 && strings.TrimSpace(plan.Script) != "" {
		if dataTaskPlanCanWrapTopLevelScriptAsCustomAction(plan) {
			actionID := "custom_transform_1"
			if strings.TrimSpace(plan.Goal) != "" {
				actionID = "bounded_transform"
			}
			plan.Actions = []dataquery.DataAction{{
				ID:         actionID,
				Kind:       dataquery.DataActionCustomTransform,
				Purpose:    firstNonEmptyString(strings.TrimSpace(plan.WhyThisBatch), strings.TrimSpace(plan.Goal), "bounded deterministic data transform"),
				InputPaths: append([]string(nil), plan.InputPaths...),
				Script:     plan.Script,
			}}
			plan.Script = ""
			reasons = append(reasons, "wrapped bounded top-level script into a custom_transform action")
			if normalized, ok := normalizeDataTaskOversizedActionBatch(plan); ok {
				plan = normalized
				reasons = append(reasons, "trimmed oversized actions[] plan to the next executable batch")
			}
			return plan, reasons
		}
	}
	if len(plan.Actions) == 0 || strings.TrimSpace(plan.Script) == "" {
		if normalized, ok := normalizeDataTaskOversizedActionBatch(plan); ok {
			plan = normalized
			reasons = append(reasons, "trimmed oversized actions[] plan to the next executable batch")
		}
		return plan, reasons
	}
	if dataTaskTopLevelScriptDuplicatesActionScript(plan.Script, plan.Actions) {
		plan.Script = ""
		reasons = append(reasons, "removed duplicate top-level script from actions[] plan")
		if normalized, ok := normalizeDataTaskOversizedActionBatch(plan); ok {
			plan = normalized
			reasons = append(reasons, "trimmed oversized actions[] plan to the next executable batch")
		}
		return plan, reasons
	}
	if len(plan.Actions) > 0 && strings.TrimSpace(plan.Script) != "" {
		plan.Script = ""
		reasons = append(reasons, "removed stray top-level script from an actions[] plan; actions[] is the executable DAG contract")
		if normalized, ok := normalizeDataTaskOversizedActionBatch(plan); ok {
			plan = normalized
			reasons = append(reasons, "trimmed oversized actions[] plan to the next executable batch")
		}
		return plan, reasons
	}
	if normalized, ok := normalizeDataTaskOversizedActionBatch(plan); ok {
		plan = normalized
		reasons = append(reasons, "trimmed oversized actions[] plan to the next executable batch")
	}
	return plan, reasons
}

func normalizeDataTaskCompleteStatusWithExecutablePayload(plan *dataquery.TaskPlan) bool {
	if plan == nil || !strings.EqualFold(strings.TrimSpace(plan.Status), "complete") {
		return false
	}
	if strings.TrimSpace(plan.Script) == "" && len(plan.Actions) == 0 {
		return false
	}
	plan.Status = "ready"
	return true
}

func normalizeDataTaskCustomActionScriptInputs(plan *dataquery.TaskPlan) bool {
	if plan == nil || len(plan.Actions) == 0 {
		return false
	}
	topInputs := cleanDataTaskStrings(plan.InputPaths)
	if len(topInputs) == 0 {
		return false
	}
	changed := false
	for i := range plan.Actions {
		if normalizeDataActionKindForWorkflow(plan.Actions[i].Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		script := strings.TrimSpace(plan.Actions[i].Script)
		if script == "" {
			continue
		}
		inputSet := map[string]bool{}
		for _, p := range cleanDataTaskStrings(plan.Actions[i].InputPaths) {
			inputSet[normalizeDataTaskCoveragePath(p)] = true
		}
		actionChanged := false
		for _, p := range topInputs {
			normalized := normalizeDataTaskCoveragePath(p)
			if normalized == "" || inputSet[normalized] || !dataTaskScriptReferencesPathLiteral(script, normalized) {
				continue
			}
			plan.Actions[i].InputPaths = append(plan.Actions[i].InputPaths, p)
			inputSet[normalized] = true
			actionChanged = true
			changed = true
		}
		if actionChanged {
			plan.Actions[i].InputPaths = cleanDataTaskStrings(plan.Actions[i].InputPaths)
		}
	}
	return changed
}

func normalizeDataTaskMaterializationActionInputs(plan *dataquery.TaskPlan) bool {
	if plan == nil || len(plan.Actions) == 0 {
		return false
	}
	topInputs := cleanDataTaskStrings(plan.InputPaths)
	if len(topInputs) == 0 {
		return false
	}
	changed := false
	for i := range plan.Actions {
		if len(cleanDataTaskStrings(plan.Actions[i].InputPaths)) > 0 {
			continue
		}
		if !dataTaskActionCanInheritBatchInputs(plan.Actions[i]) {
			continue
		}
		plan.Actions[i].InputPaths = append([]string(nil), topInputs...)
		changed = true
	}
	return changed
}

func dataTaskActionCanInheritBatchInputs(action dataquery.DataAction) bool {
	if strings.TrimSpace(action.Script) != "" {
		return false
	}
	switch normalizeDataActionKindForWorkflow(action.Kind) {
	case dataquery.DataActionMaterialInventory, dataquery.DataActionInspectMaterial, dataquery.DataActionExtractRecords, dataquery.DataActionExtractFields, dataquery.DataActionGroupRecords:
		return true
	default:
		return false
	}
}

func dataTaskScriptReferencesPathLiteral(script, p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	return strings.Contains(script, `"`+p+`"`) || strings.Contains(script, `'`+p+`'`)
}

func normalizeDataTaskPlanShapeForPolicy(plan dataquery.TaskPlan, policy TurnPolicy) (dataquery.TaskPlan, []string) {
	var reasons []string
	if normalizeDataTaskAggregationContractFromPolicy(&plan, policy) {
		reasons = append(reasons, "enabled contribution/reconcile contract for a complex typed data aggregation plan")
	}
	if normalizeDataTaskStrictOutputContractFromShape(&plan) {
		reasons = append(reasons, "enabled contribution/reconcile contract for a complex strict-output data plan")
	}
	if normalizeDataTaskRuleCoverageContractFromShape(&plan) {
		reasons = append(reasons, "enabled rule coverage contract for required text/rule material in a validated data workflow")
	}
	normalized, shapeReasons := normalizeDataTaskPlanShape(plan)
	reasons = append(reasons, shapeReasons...)
	if normalizeDataTaskAggregationContractFromPolicy(&normalized, policy) {
		reasons = append(reasons, "preserved contribution/reconcile contract after data plan shape normalization")
	}
	if normalizeDataTaskStrictOutputContractFromShape(&normalized) {
		reasons = append(reasons, "preserved contribution/reconcile contract after strict-output shape normalization")
	}
	if normalizeDataTaskRuleCoverageContractFromShape(&normalized) {
		reasons = append(reasons, "preserved rule coverage contract after rule material shape normalization")
	}
	return normalized, reasons
}

func normalizeDataTaskAggregationContractFromPolicy(plan *dataquery.TaskPlan, policy TurnPolicy) bool {
	if plan == nil || !dataTaskPolicyRequiresAggregationLedger(policy) || !dataTaskPlanIsComplexAggregationCandidate(*plan) {
		return false
	}
	changed := false
	if !plan.CoverageContract.ContributionLedgerRequired {
		plan.CoverageContract.ContributionLedgerRequired = true
		changed = true
	}
	if !plan.CoverageContract.ReconcileRequired {
		plan.CoverageContract.ReconcileRequired = true
		changed = true
	}
	return changed
}

func dataTaskPolicyRequiresAggregationLedger(policy TurnPolicy) bool {
	switch strings.ToLower(strings.TrimSpace(firstNonEmptyString(policy.DataTaskKind, policy.Operation))) {
	case "data_aggregation":
		return true
	default:
		return false
	}
}

func dataTaskPlanIsComplexAggregationCandidate(plan dataquery.TaskPlan) bool {
	return len(plan.CoverageContract.RequiredMaterials) >= 2 ||
		len(plan.InputPaths) >= 2 ||
		len(plan.Actions) > 0
}

func normalizeDataTaskStrictOutputContractFromShape(plan *dataquery.TaskPlan) bool {
	if plan == nil || !dataTaskPlanHasStrictOutputContract(*plan) || !dataTaskPlanIsComplexStrictOutputCandidate(*plan) {
		return false
	}
	changed := false
	if !plan.CoverageContract.DecisionRecordsRequired {
		plan.CoverageContract.DecisionRecordsRequired = true
		changed = true
	}
	if !plan.CoverageContract.ContributionLedgerRequired {
		plan.CoverageContract.ContributionLedgerRequired = true
		changed = true
	}
	if !plan.CoverageContract.ReconcileRequired {
		plan.CoverageContract.ReconcileRequired = true
		changed = true
	}
	return changed
}

func normalizeDataTaskRuleCoverageContractFromShape(plan *dataquery.TaskPlan) bool {
	if plan == nil || plan.CoverageContract.RuleCoverageRequired {
		return false
	}
	if len(dataTaskRequiredRuleMaterialInputs(plan.CoverageContract)) == 0 {
		return false
	}
	if dataTaskBusinessValidationLedgerCount(plan.CoverageContract) < 2 {
		return false
	}
	if !dataTaskPlanHasStrictOutputContract(*plan) && !dataTaskPlanIsComplexAggregationCandidate(*plan) {
		return false
	}
	plan.CoverageContract.RuleCoverageRequired = true
	return true
}

func dataTaskBusinessValidationLedgerCount(contract dataquery.CoverageContract) int {
	n := 0
	if contract.DecisionRecordsRequired {
		n++
	}
	if contract.ContributionLedgerRequired {
		n++
	}
	if contract.EntityResolutionRequired {
		n++
	}
	if contract.ReconcileRequired {
		n++
	}
	return n
}

func dataTaskPlanHasStrictOutputContract(plan dataquery.TaskPlan) bool {
	contract := plan.OutputContract.Normalize()
	if contract.ExplanationAllowed {
		return false
	}
	switch contract.Format {
	case dataquery.OutputPlainSingleLine, dataquery.OutputCSVLine, dataquery.OutputJSONOnly, dataquery.OutputMarkdownTable:
		return true
	default:
		return false
	}
}

func dataTaskPlanIsComplexStrictOutputCandidate(plan dataquery.TaskPlan) bool {
	return len(plan.CoverageContract.RequiredMaterials) >= 4 ||
		len(plan.InputPaths) >= 4 ||
		len(plan.Actions) >= 2 ||
		dataTaskScriptLineCount(plan.Script) >= 40
}

func normalizeDataTaskDeriveRulesInputs(plan *dataquery.TaskPlan) bool {
	if plan == nil || len(plan.Actions) == 0 {
		return false
	}
	inputs := dataTaskRuleMaterialInputs(plan.CoverageContract)
	changed := false
	for i := range plan.Actions {
		if normalizeDataActionKindForWorkflow(plan.Actions[i].Kind) != dataquery.DataActionDeriveRules {
			continue
		}
		current := cleanDataTaskStrings(plan.Actions[i].InputPaths)
		if len(current) == 0 {
			if len(inputs) == 0 || dataTaskDeriveRulesActionHasExplicitRules(plan.Actions[i]) {
				continue
			}
			plan.Actions[i].InputPaths = append([]string(nil), inputs...)
			changed = true
			continue
		}
		filtered := filterDataTaskRuleMaterialInputs(current, inputs)
		if len(filtered) == len(current) && strings.Join(filtered, "\x00") == strings.Join(current, "\x00") {
			continue
		}
		if len(filtered) > 0 {
			plan.Actions[i].InputPaths = filtered
		} else if len(plan.CoverageContract.ValidationRules) > 0 || dataTaskDeriveRulesActionHasExplicitRules(plan.Actions[i]) {
			plan.Actions[i].InputPaths = nil
		} else if len(inputs) > 0 {
			plan.Actions[i].InputPaths = append([]string(nil), inputs...)
		} else {
			continue
		}
		changed = true
	}
	return changed
}

func dataTaskRuleMaterialInputs(contract dataquery.CoverageContract) []string {
	var out []string
	collect := func(materials []dataquery.CoverageMaterial) {
		for _, material := range materials {
			mode := normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode)
			var p string
			switch mode {
			case dataquery.MaterialUseScriptConsumed, dataquery.MaterialUsePlannerDistilled:
				p = normalizeDataTaskCoveragePath(material.Path)
			case dataquery.MaterialUseTextEvidenceConsumed:
				p = normalizeDataTaskCoveragePath(material.TextEvidencePath)
			default:
				continue
			}
			if p == "" || !dataTaskPathLooksLikeTextConstraintMaterial(p) {
				continue
			}
			out = append(out, p)
		}
	}
	collect(contract.RequiredMaterials)
	collect(contract.OptionalMaterials)
	return cleanDataTaskStrings(out)
}

func filterDataTaskRuleMaterialInputs(current, allowed []string) []string {
	allowedSet := map[string]bool{}
	for _, p := range cleanDataTaskStrings(allowed) {
		allowedSet[normalizeDataTaskCoveragePath(p)] = true
	}
	allowTextShapeFallback := len(allowedSet) == 0
	var out []string
	for _, p := range cleanDataTaskStrings(current) {
		key := normalizeDataTaskCoveragePath(p)
		if key == "" {
			continue
		}
		if allowedSet[key] || (allowTextShapeFallback && dataTaskPathLooksLikeTextConstraintMaterial(key)) {
			out = append(out, p)
		}
	}
	return cleanDataTaskStrings(out)
}

func dataTaskDeriveRulesActionHasExplicitRules(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	for _, key := range []string{"rules_json", "rules", "validation_rules_json", "rule_text", "rule"} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return true
		}
	}
	return false
}

func dataTaskRequiredRuleMaterialInputs(contract dataquery.CoverageContract) []string {
	var out []string
	for _, material := range contract.RequiredMaterials {
		mode := normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode)
		var p string
		switch mode {
		case dataquery.MaterialUseScriptConsumed:
			p = normalizeDataTaskCoveragePath(material.Path)
		case dataquery.MaterialUseTextEvidenceConsumed:
			p = normalizeDataTaskCoveragePath(material.TextEvidencePath)
		default:
			continue
		}
		if p == "" || !dataTaskPathLooksLikeTextConstraintMaterial(p) {
			continue
		}
		out = append(out, p)
	}
	return cleanDataTaskStrings(out)
}

func dataTaskTopLevelScriptDuplicatesActionScript(script string, actions []dataquery.DataAction) bool {
	normalizedTop := normalizeDataTaskScriptForDuplicateCompare(script)
	if normalizedTop == "" {
		return false
	}
	for _, action := range actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		if normalizedTop == normalizeDataTaskScriptForDuplicateCompare(action.Script) {
			return true
		}
	}
	return false
}

func normalizeDataTaskScriptForDuplicateCompare(script string) string {
	var lines []string
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func normalizeDataTaskPlanPathLists(plan *dataquery.TaskPlan) bool {
	if plan == nil {
		return false
	}
	changed := false
	normalize := func(values []string) []string {
		next := expandDataTaskPathListStrings(values)
		if strings.Join(next, "\x00") != strings.Join(cleanDataTaskStrings(values), "\x00") {
			changed = true
		}
		return next
	}
	plan.InputPaths = normalize(plan.InputPaths)
	for i := range plan.Actions {
		plan.Actions[i].InputPaths = normalize(plan.Actions[i].InputPaths)
	}
	normalizeMaterials := func(materials []dataquery.CoverageMaterial) []dataquery.CoverageMaterial {
		var out []dataquery.CoverageMaterial
		for _, material := range materials {
			expanded := expandDataTaskCoverageMaterialPaths(material)
			if len(expanded) != 1 || expanded[0].Path != material.Path || expanded[0].TextEvidencePath != material.TextEvidencePath {
				changed = true
			}
			out = append(out, expanded...)
		}
		return out
	}
	plan.CoverageContract.RequiredMaterials = normalizeMaterials(plan.CoverageContract.RequiredMaterials)
	plan.CoverageContract.OptionalMaterials = normalizeMaterials(plan.CoverageContract.OptionalMaterials)
	return changed
}

func expandDataTaskCoverageMaterialPaths(material dataquery.CoverageMaterial) []dataquery.CoverageMaterial {
	paths := expandDataTaskPathListStrings([]string{material.Path})
	textPaths := expandDataTaskPathListStrings([]string{material.TextEvidencePath})
	if len(paths) == 0 {
		paths = []string{strings.TrimSpace(material.Path)}
	}
	if len(paths) <= 1 && len(textPaths) <= 1 {
		material.Path = strings.TrimSpace(material.Path)
		material.TextEvidencePath = strings.TrimSpace(material.TextEvidencePath)
		return []dataquery.CoverageMaterial{material}
	}
	out := make([]dataquery.CoverageMaterial, 0, maxInt(len(paths), len(textPaths)))
	n := maxInt(len(paths), len(textPaths))
	for i := 0; i < n; i++ {
		next := material
		if i < len(paths) {
			next.Path = paths[i]
		} else {
			next.Path = ""
		}
		if i < len(textPaths) {
			next.TextEvidencePath = textPaths[i]
		} else if len(textPaths) == 1 {
			next.TextEvidencePath = textPaths[0]
		} else {
			next.TextEvidencePath = ""
		}
		if len(paths) > 1 || len(textPaths) > 1 {
			baseID := strings.TrimSpace(material.ID)
			if baseID != "" {
				next.ID = fmt.Sprintf("%s_%d", baseID, i+1)
			}
		}
		if strings.TrimSpace(next.Path) == "" && strings.TrimSpace(next.TextEvidencePath) == "" {
			continue
		}
		out = append(out, next)
	}
	return out
}

func expandDataTaskPathListStrings(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts := splitDataTaskPathListString(value)
		if len(parts) == 0 {
			out = append(out, value)
			continue
		}
		out = append(out, parts...)
	}
	return cleanDataTaskStrings(out)
}

func splitDataTaskPathListString(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.Contains(value, ",") {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !dataTaskStringLooksLikePath(part) {
			return nil
		}
		out = append(out, part)
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

func dataTaskStringLooksLikePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\n\r") {
		return false
	}
	if strings.Contains(value, "/") || strings.Contains(value, "\\") {
		return true
	}
	ext := path.Ext(value)
	return len(ext) > 1 && len(ext) <= 12
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func normalizeDataTaskOversizedActionBatch(plan dataquery.TaskPlan) (dataquery.TaskPlan, bool) {
	if len(plan.Actions) <= dataTaskMaxActionsPerBatch {
		return plan, false
	}
	keep := make([]dataquery.DataAction, 0, dataTaskMaxActionsPerBatch)
	for _, action := range plan.Actions {
		if len(keep) >= dataTaskMaxActionsPerBatch {
			break
		}
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if kind == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != "" && len(keep) > 0 {
			break
		}
		keep = append(keep, action)
	}
	if len(keep) == 0 {
		keep = append(keep, plan.Actions[:dataTaskMaxActionsPerBatch]...)
	}
	plan.Actions = keep
	plan.Script = ""
	plan.ContinueAfter = true
	if strings.TrimSpace(plan.NextBatch) == "" {
		plan.NextBatch = "continue with the remaining data workflow actions after this bounded batch produces artifacts"
	}
	if strings.TrimSpace(plan.WhyThisBatch) == "" {
		plan.WhyThisBatch = "execute the next atomic data workflow batch within the system action budget"
	}
	return plan, true
}

func normalizeDataTaskPlanContractFromActions(plan *dataquery.TaskPlan) bool {
	if plan == nil {
		return false
	}
	changed := false
	for _, action := range plan.Actions {
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if dataworkflow.IsLeafFallback(kind) {
			continue
		}
		if dataworkflow.ProducesLedger(kind, dataworkflow.LedgerRuleCoverage) && !plan.CoverageContract.RuleCoverageRequired {
			plan.CoverageContract.RuleCoverageRequired = true
			changed = true
		}
		if dataworkflow.ProducesLedger(kind, dataworkflow.LedgerDecisions) && !plan.CoverageContract.DecisionRecordsRequired {
			plan.CoverageContract.DecisionRecordsRequired = true
			changed = true
		}
		if dataworkflow.ProducesLedger(kind, dataworkflow.LedgerContributions) && !plan.CoverageContract.ContributionLedgerRequired {
			plan.CoverageContract.ContributionLedgerRequired = true
			changed = true
		}
		// Reconcile is required when the plan produces a NUMERIC
		// contribution ledger (real aggregation) or runs an explicit
		// reconcile action — not merely because it assembles a final
		// answer. The previous link forced reconcile_required off
		// LedgerFinalProjection, which the universal assemble_answer
		// action produces, dragging pure extraction/projection tasks
		// (list/scalar outputs with no numeric aggregation) into a
		// numeric reconcile they structurally cannot satisfy. Tying it
		// to LedgerContributions uses the precise structural signal:
		// reconcile what was numerically aggregated.
		if dataworkflow.ProducesLedger(kind, dataworkflow.LedgerContributions) && !plan.CoverageContract.ReconcileRequired {
			plan.CoverageContract.ReconcileRequired = true
			changed = true
		}
		if dataworkflow.ProducesLedger(kind, dataworkflow.LedgerReconcile) && !plan.CoverageContract.ReconcileRequired {
			plan.CoverageContract.ReconcileRequired = true
			changed = true
		}
	}
	return changed
}

func dataTaskPlanCanWrapTopLevelScriptAsCustomAction(plan dataquery.TaskPlan) bool {
	lines := dataTaskScriptLineCount(plan.Script)
	if lines == 0 || lines >= dataTaskComplexCustomScriptLineLimit {
		return false
	}
	if !dataTaskScriptHasResultEmitter(plan.Script) {
		return false
	}
	requiredMaterials := len(plan.CoverageContract.RequiredMaterials)
	validationLedgers := dataTaskValidationLedgerCount(plan.CoverageContract)
	inputs := len(plan.InputPaths)
	return requiredMaterials >= 4 || validationLedgers >= 2 || inputs >= 4
}

func dataTaskMaterialDiscoveryFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	state := dataTaskWorkflowState(records, plan)
	validationRule := ""
	if strings.TrimSpace(errText) != "" {
		validationRule = "previous broad plan was converted into material discovery: " + oneLineClamp(errText, 240)
	}
	currentInventory := len(plan.Actions) == 1 && normalizeDataActionKindForWorkflow(plan.Actions[0].Kind) == dataquery.DataActionMaterialInventory
	return dataworkflow.BuildMaterialDiscoveryTransition(dataworkflow.MaterialDiscoveryTransitionInput{
		Current:                    plan,
		Coverage:                   dataTaskWorkflowCoverageContract(records, plan),
		Paths:                      dataTaskDiscoveryPaths(plan),
		ValidationRule:             validationRule,
		MaterialCoverageSufficient: state.MaterialCoverageSufficient,
		PriorMaterialInventorySeen: dataTaskWorkflowHasActionKind(records, dataquery.DataActionMaterialInventory),
		CurrentMaterialInventory:   currentInventory,
		NonCustomActionCount:       dataTaskPlanNonCustomActionCount(plan.Actions, len(plan.Actions)),
		HasExecutableSurface:       strings.TrimSpace(plan.Script) != "" || dataTaskPlanHasCustomTransform(plan),
		BroadMaterialLimit:         dataTaskBroadMaterialDiscoveryLimit,
	})
}

func dataTaskCoverageExpansionFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	state := dataTaskWorkflowState(records, plan)
	if state.MaterialCoverageSufficient && dataTaskPlanIsCoverageOnly(plan) {
		return dataquery.TaskPlan{}, false
	}
	if !dataTaskCoverageExpansionFallbackNeeded(records, plan) {
		return dataquery.TaskPlan{}, false
	}
	return dataTaskBuildCoverageExpansionFallback(records, plan, errText)
}

func dataTaskCoverageExpansionFallbackAfterResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	missing := dataTaskCoverageExpansionMissingPaths(records, plan)
	if len(missing) == 0 {
		return dataquery.TaskPlan{}, false
	}
	return dataTaskBuildCoverageExpansionFallback(records, plan, errText)
}

func dataTaskBuildCoverageExpansionFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	missing := dataTaskCoverageExpansionMissingPaths(records, plan)
	if len(missing) == 0 {
		return dataquery.TaskPlan{}, false
	}
	contract := dataTaskWorkflowCoverageContract(records, plan)
	validationRule := ""
	if len(contract.ValidationRules) == 0 && strings.TrimSpace(errText) != "" {
		validationRule = "previous structural coverage guard requested an atomic material-coverage batch: " + oneLineClamp(errText, 240)
	}
	return dataworkflow.BuildCoverageExpansionPlan(dataworkflow.CoverageExpansionPlanInput{
		Current:            plan,
		Coverage:           contract,
		MissingPaths:       missing,
		DeriveRulesForText: dataTaskPlanShouldDeriveRulesForTextCoverage(plan),
		MaxActions:         dataTaskMaxActionsPerBatch,
		ValidationRule:     validationRule,
	})
}

func dataTaskPlanShouldDeriveRulesForTextCoverage(plan dataquery.TaskPlan) bool {
	return plan.CoverageContract.RuleCoverageRequired || dataTaskValidationLedgerCount(plan.CoverageContract) >= 2
}

func dataTaskCoverageExpansionFallbackNeeded(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) bool {
	if len(plan.Actions) == 0 && strings.TrimSpace(plan.Script) == "" && len(dataTaskCoverageExpansionMissingPaths(records, plan)) > 0 {
		return true
	}
	if !dataTaskTerminalRequiredMaterialSchedulingGuardResult(records, plan).Empty() {
		return true
	}
	for i, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		if !dataTaskActionHasBroadPrerequisiteSurface(plan, action) {
			continue
		}
		if len(dataTaskMissingCustomTransformPrerequisites(records, plan, action, i)) > 0 {
			return true
		}
	}
	return false
}

func applyDataTaskUserMaterialFloor(userLine string, candidates []dataquery.CandidateFile, plan dataquery.TaskPlan) dataquery.TaskPlan {
	materials := dataTaskUserMentionedCandidateMaterials(userLine, candidates)
	if len(materials) == 0 {
		return plan
	}
	plan.CoverageContract.RequiredMaterials = mergeDataTaskCoverageMaterials(materials, plan.CoverageContract.RequiredMaterials, true)
	requiredKeys := dataTaskCoverageMaterialKeySet(plan.CoverageContract.RequiredMaterials)
	plan.CoverageContract.OptionalMaterials = dataTaskFilterCoverageMaterials(plan.CoverageContract.OptionalMaterials, func(m dataquery.CoverageMaterial) bool {
		key := dataTaskCoverageMaterialKey(m)
		return key == "" || !requiredKeys[key]
	})
	plan.CoverageContract.ValidationRules = mergeDataTaskValidationRules(
		[]string{"user-explicit candidate materials must remain in the coverage contract with a verifiable usage_mode"},
		plan.CoverageContract.ValidationRules,
	)
	plan.InputPaths = mergeDataTaskInputPaths(plan.InputPaths, plan.CoverageContract.RequiredRunnerInputPaths())
	return plan
}

func prepareDataTaskWorkflowPlanForExecution(userLine string, candidates []dataquery.CandidateFile, records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataquery.TaskPlan {
	plan = applyDataTaskUserMaterialFloor(userLine, candidates, plan)
	normalizeDataTaskPlanPathLists(&plan)
	dataworkflow.NormalizeRolePathActionInputs(&plan)
	if bootstrap, ok := dataTaskCandidateInventoryBootstrapPlan(candidates, records, plan); ok {
		return bootstrap
	}
	normalizeDataTaskMaterializationActionInputs(&plan)
	normalizeDataTaskCustomActionScriptInputs(&plan)
	normalizeDataTaskExactExtractLimitsForWorkflow(&plan)
	plan = dataTaskNarrowSingleRecordSetActionInputsForExecution(records, plan)
	return dataTaskScopePlanToCurrentBatch(records, plan)
}

func dataTaskCandidateInventoryBootstrapPlan(candidates []dataquery.CandidateFile, records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) (dataquery.TaskPlan, bool) {
	if len(records) > 0 || len(candidates) == 0 {
		return dataquery.TaskPlan{}, false
	}
	if dataTaskValidationLedgerCount(plan.CoverageContract) < 2 && !dataTaskPlanRequiresCompleteRecordSets(plan) {
		return dataquery.TaskPlan{}, false
	}
	if dataTaskPlanHasActionKind(plan, dataquery.DataActionMaterialInventory) {
		return dataquery.TaskPlan{}, false
	}
	candidatePaths := dataTaskCandidatePaths(candidates)
	if len(candidatePaths) == 0 {
		return dataquery.TaskPlan{}, false
	}
	planned := map[string]bool{}
	for _, p := range dataTaskDiscoveryPaths(plan) {
		planned[normalizeDataTaskCoveragePath(p)] = true
	}
	var omitted []string
	for _, p := range candidatePaths {
		key := normalizeDataTaskCoveragePath(p)
		if key == "" || planned[key] {
			continue
		}
		omitted = append(omitted, p)
	}
	if len(omitted) == 0 {
		return dataquery.TaskPlan{}, false
	}
	out := plan
	out.Status = "ready"
	out.Script = ""
	out.InputPaths = candidatePaths
	out.Actions = []dataquery.DataAction{{
		ID:             "material_inventory",
		Kind:           dataquery.DataActionMaterialInventory,
		Purpose:        "discover objective candidate material inventory before classifying required, reference, or irrelevant materials",
		InputPaths:     candidatePaths,
		OutputArtifact: "candidate_material_inventory.json",
		Params:         map[string]string{"limit": fmt.Sprintf("%d", len(candidatePaths))},
	}}
	out.ContinueAfter = true
	out.OutputContract = dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true}
	if strings.TrimSpace(out.Goal) == "" {
		out.Goal = "discover candidate data materials"
	}
	out.WhyThisBatch = fmt.Sprintf("complex data workflow has %d unclassified candidate material(s); first expose objective inventory before typed calculation", len(omitted))
	out.NextBatch = "classify candidate materials for the user goal, then continue with typed data workflow actions"
	out.CoverageContract.OptionalMaterials = mergeDataTaskCoverageMaterials(
		out.CoverageContract.OptionalMaterials,
		dataTaskCandidateOptionalMaterials(candidates),
		false,
	)
	return out, true
}

func dataTaskCandidatePaths(candidates []dataquery.CandidateFile) []string {
	var paths []string
	for _, candidate := range candidates {
		p := normalizeDataTaskCoveragePath(candidate.Path)
		if p != "" {
			paths = append(paths, p)
		}
	}
	return cleanDataTaskStrings(paths)
}

func dataTaskCandidateOptionalMaterials(candidates []dataquery.CandidateFile) []dataquery.CoverageMaterial {
	out := make([]dataquery.CoverageMaterial, 0, len(candidates))
	for _, candidate := range candidates {
		p := normalizeDataTaskCoveragePath(candidate.Path)
		if p == "" {
			continue
		}
		material := dataquery.CoverageMaterial{
			ID:        p,
			Path:      p,
			Purpose:   "candidate material awaiting model classification for the current data goal",
			UsageMode: dataquery.MaterialUseReferenceOnly,
		}
		if len(candidate.TextEvidencePaths) > 0 {
			material.TextEvidencePath = normalizeDataTaskCoveragePath(candidate.TextEvidencePaths[0])
		}
		out = append(out, material)
	}
	return out
}

func normalizeDataTaskExactExtractLimitsForWorkflow(plan *dataquery.TaskPlan) bool {
	if plan == nil || !dataTaskPlanRequiresCompleteRecordSets(*plan) {
		return false
	}
	changed := false
	for i := range plan.Actions {
		action := plan.Actions[i]
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionExtractRecords {
			continue
		}
		if dataTaskExtractActionSampleOnly(action) {
			continue
		}
		if action.Params == nil {
			action.Params = map[string]string{}
		}
		current := dataTaskExtractActionLimit(action)
		if current >= dataTaskExactExtractRecordLimit {
			continue
		}
		action.Params["limit"] = fmt.Sprintf("%d", dataTaskExactExtractRecordLimit)
		action.Params["record_completeness_required"] = "true"
		if strings.TrimSpace(action.Purpose) != "" && !strings.Contains(action.Purpose, "full record-set") {
			action.Purpose = strings.TrimSpace(action.Purpose) + " (full record-set materialization required by workflow contract)"
		}
		plan.Actions[i] = action
		changed = true
	}
	return changed
}

func dataTaskPlanRequiresCompleteRecordSets(plan dataquery.TaskPlan) bool {
	contract := plan.OutputContract.Normalize()
	if plan.CoverageContract.ContributionLedgerRequired || plan.CoverageContract.ReconcileRequired {
		return true
	}
	if plan.CoverageContract.DecisionRecordsRequired && dataTaskOutputContractNeedsFinalProjection(contract) {
		return true
	}
	return false
}

func dataTaskExtractActionSampleOnly(action dataquery.DataAction) bool {
	for _, key := range []string{"sample_only", "schema_only", "preview_only"} {
		if dataTaskStringParamBool(action.Params, key) {
			return true
		}
	}
	return false
}

func dataTaskExtractActionLimit(action dataquery.DataAction) int {
	for _, key := range []string{"limit", "max_records"} {
		raw := strings.TrimSpace(action.Params[key])
		if raw == "" {
			continue
		}
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func dataTaskStringParamBool(params map[string]string, key string) bool {
	if params == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(params[key])) {
	case "true", "1", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func dataTaskNarrowSingleRecordSetActionInputsForExecution(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataquery.TaskPlan {
	if len(records) == 0 || len(plan.Actions) == 0 {
		return plan
	}
	out := plan
	for i, action := range out.Actions {
		if narrowed, ok := dataTaskNarrowSingleRecordSetActionForExecution(records, action); ok {
			out.Actions[i] = narrowed
		}
	}
	return out
}

func dataTaskNarrowSingleRecordSetActionForExecution(records []dataTaskWorkflowRecord, action dataquery.DataAction) (dataquery.DataAction, bool) {
	if len(records) == 0 {
		return dataquery.DataAction{}, false
	}
	access, _, _ := dataTaskWorkflowArtifactAvailability(records, 512)
	if len(access) == 0 {
		return dataquery.DataAction{}, false
	}
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields, dataquery.DataActionGroupRecords, dataquery.DataActionExpandRecords, dataquery.DataActionFilterRecords, dataquery.DataActionQualifyRecords, dataquery.DataActionComputeContribs:
	default:
		return dataquery.DataAction{}, false
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) <= 1 {
		return dataquery.DataAction{}, false
	}
	fieldRefs := dataTaskSingleRecordSetActionFieldRefs(kind, action)
	if len(fieldRefs) == 0 {
		return dataquery.DataAction{}, false
	}
	var matches []string
	for _, input := range inputs {
		if _, ok := dataTaskArtifactAccessByAlias(access, input); !ok {
			continue
		}
		if len(dataTaskMissingFieldsOnArtifact(access, input, fieldRefs)) == 0 {
			matches = append(matches, input)
		}
	}
	if len(matches) != 1 {
		return dataquery.DataAction{}, false
	}
	action.InputPaths = []string{matches[0]}
	if strings.TrimSpace(action.Purpose) != "" && !strings.Contains(action.Purpose, "input narrowed by field contract") {
		action.Purpose = strings.TrimSpace(action.Purpose) + " (input narrowed by field contract)"
	}
	return action, true
}

func dataTaskSingleRecordSetActionFieldRefs(kind dataquery.DataActionKind, action dataquery.DataAction) []string {
	return dataworkflow.SingleRecordSetActionFieldRefs(kind, action)
}

func dataTaskDeriveActionSourceFieldRefs(action dataquery.DataAction) []string {
	return dataworkflow.DeriveActionSourceFieldRefs(action)
}

func dataTaskGroupRecordsActionFieldRefs(action dataquery.DataAction) []string {
	return dataworkflow.GroupRecordsActionFieldRefs(action)
}

func dataTaskScopePlanToCurrentBatch(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataquery.TaskPlan {
	out := plan
	workflowContract := dataTaskWorkflowCoverageContract(records, plan)
	out.CoverageContract = dataTaskExecutionCoverageContract(records, plan, workflowContract)
	if len(plan.Actions) == 0 {
		return out
	}
	inputs := dataTaskCurrentBatchInputPaths(plan)
	if len(inputs) > 0 {
		out.InputPaths = inputs
	}
	return out
}

func dataTaskUserMentionedCandidateMaterials(userLine string, candidates []dataquery.CandidateFile) []dataquery.CoverageMaterial {
	request := normalizeDataTaskUserMaterialMatchText(userLine)
	if request == "" || len(candidates) == 0 {
		return nil
	}
	var out []dataquery.CoverageMaterial
	seen := map[string]bool{}
	for _, candidate := range candidates {
		path := normalizeDataTaskCoveragePath(candidate.Path)
		if path == "" || !dataTaskCandidatePathMentioned(request, path) {
			continue
		}
		material := dataquery.CoverageMaterial{
			ID:        dataTaskCoverageIDFromPath(path),
			Path:      path,
			Purpose:   dataTaskUserExplicitMaterialPurpose,
			Required:  true,
			UsageMode: dataTaskUsageModeForCandidate(candidate),
		}
		if material.UsageMode == dataquery.MaterialUseTextEvidenceConsumed && len(candidate.TextEvidencePaths) > 0 {
			material.TextEvidencePath = normalizeDataTaskCoveragePath(candidate.TextEvidencePaths[0])
		}
		key := dataTaskCoverageMaterialKey(material)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, material)
	}
	return out
}

func normalizeDataTaskUserMaterialMatchText(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.ToLower(value)
	return value
}

func dataTaskCandidatePathMentioned(request, candidatePath string) bool {
	candidatePath = normalizeDataTaskUserMaterialMatchText(candidatePath)
	if candidatePath == "" {
		return false
	}
	if strings.Contains(request, candidatePath) {
		return true
	}
	base := path.Base(candidatePath)
	return strings.Contains(base, ".") && strings.Contains(request, base)
}

func dataTaskCoverageIDFromPath(value string) string {
	value = normalizeDataTaskCoveragePath(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "_") {
				b.WriteByte('_')
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func dataTaskUsageModeForCandidate(candidate dataquery.CandidateFile) dataquery.CoverageMaterialUseMode {
	switch strings.TrimSpace(candidate.Kind) {
	case "csv", "tsv", "json", "jsonl", "text", "generated_json":
		return dataquery.MaterialUseScriptConsumed
	default:
		if len(candidate.TextEvidencePaths) > 0 {
			return dataquery.MaterialUseTextEvidenceConsumed
		}
		return dataquery.MaterialUseScriptConsumed
	}
}

func dataTaskFilterCoverageMaterials(materials []dataquery.CoverageMaterial, keep func(dataquery.CoverageMaterial) bool) []dataquery.CoverageMaterial {
	if len(materials) == 0 {
		return nil
	}
	out := make([]dataquery.CoverageMaterial, 0, len(materials))
	for _, material := range materials {
		if keep == nil || keep(material) {
			out = append(out, material)
		}
	}
	return out
}

func mergeDataTaskValidationRules(previous, next []string) []string {
	return cleanDataTaskStrings(append(append([]string(nil), previous...), next...))
}

func dataTaskCoverageExpansionMissingPaths(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) []string {
	seen := map[string]bool{}
	var missing []string
	add := func(values []string) {
		for _, p := range cleanDataTaskStrings(values) {
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			missing = append(missing, p)
		}
	}
	requiredContract := dataTaskWorkflowCoverageContract(records, plan)
	if !dataTaskTerminalRequiredMaterialSchedulingGuardResult(records, plan).Empty() {
		requiredContract = plan.CoverageContract
	}
	required := cleanDataTaskStrings(requiredContract.RequiredRunnerInputPaths())
	if len(required) > 0 {
		covered := dataTaskWorkflowCoveredMaterialPaths(records, dataquery.TaskPlan{}, 0)
		for _, p := range required {
			if !dataTaskCoveragePathCovered(covered, p) {
				add([]string{p})
			}
		}
	}
	for i, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		if !dataTaskActionHasBroadPrerequisiteSurface(plan, action) {
			continue
		}
		add(dataTaskMissingCustomTransformPrerequisites(records, plan, action, i))
	}
	sort.Strings(missing)
	return missing
}

func dataTaskWorkflowHasActionKind(records []dataTaskWorkflowRecord, kind dataquery.DataActionKind) bool {
	for _, rec := range records {
		for _, action := range rec.Plan.Actions {
			if normalizeDataActionKindForWorkflow(action.Kind) == kind {
				return true
			}
		}
	}
	return false
}

func dataTaskPlanHasActionKind(plan dataquery.TaskPlan, kind dataquery.DataActionKind) bool {
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) == kind {
			return true
		}
	}
	return false
}

func dataTaskDiscoveryPaths(plan dataquery.TaskPlan) []string {
	var paths []string
	paths = append(paths, plan.InputPaths...)
	paths = append(paths, plan.CoverageContract.RequiredRunnerInputPaths()...)
	paths = append(paths, plan.CoverageContract.RequiredPaths()...)
	for _, action := range plan.Actions {
		paths = append(paths, action.InputPaths...)
	}
	return cleanDataTaskStrings(paths)
}

func dataTaskPlanHasCustomTransform(plan dataquery.TaskPlan) bool {
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform {
			return true
		}
	}
	return false
}

func dataTaskActionStagingGuardError(plan dataquery.TaskPlan) string {
	return dataTaskActionStagingGuardResult(plan).ErrorText()
}

func dataTaskWorkflowActionStagingGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	return dataTaskWorkflowActionStagingGuardResult(records, plan).ErrorText()
}

func dataTaskWorkflowNumericConstantReuseGuardError(plan dataquery.TaskPlan) string {
	return dataTaskWorkflowNumericConstantReuseGuardResult(plan).ErrorText()
}

func dataTaskWorkflowNumericConstantReuseGuardResult(plan dataquery.TaskPlan) dataworkflow.GuardResult {
	if len(plan.Actions) < 2 {
		return dataworkflow.GuardResult{}
	}
	var constants []dataworkflow.DerivedConstantFieldFact
	for i, action := range plan.Actions {
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if kind != dataquery.DataActionDeriveFields && kind != dataquery.DataActionExtractFields {
			continue
		}
		for _, spec := range dataTaskActionObjectListParam(action.Params, "field_specs_json", "derive_specs_json", "extract_specs_json", "transforms_json", "field_specs", "derive_specs", "extract_specs", "transforms") {
			op := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
				dataTaskMapStringValue(spec, "operation"),
				dataTaskMapStringValue(spec, "op"),
				dataTaskMapStringValue(spec, "transform"),
			)))
			if op != "constant" {
				continue
			}
			field := strings.TrimSpace(firstNonEmptyString(
				dataTaskMapStringValue(spec, "target_field"),
				dataTaskMapStringValue(spec, "output_field"),
				dataTaskMapStringValue(spec, "derived_field"),
				dataTaskMapStringValue(spec, "name"),
			))
			value := strings.TrimSpace(dataTaskMapStringValue(spec, "value"))
			if field == "" || dataTaskStringLooksNumeric(value) {
				continue
			}
			constants = append(constants, dataworkflow.DerivedConstantFieldFact{
				ActionIndex:   i,
				ActionID:      firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(kind))),
				Action:        action,
				Field:         field,
				Value:         value,
				OutputAliases: dataTaskActionOutputAliases(action),
			})
		}
	}
	if len(constants) == 0 {
		return dataworkflow.GuardResult{}
	}
	var uses []dataworkflow.NumericFieldUseFact
	for j, action := range plan.Actions {
		for _, field := range dataTaskNumericFieldUsesFromAction(action) {
			uses = append(uses, dataworkflow.NumericFieldUseFact{
				ActionIndex:  j,
				Action:       action,
				Field:        field,
				InputAliases: cleanDataTaskStrings(action.InputPaths),
			})
		}
	}
	return dataworkflow.NumericConstantReuseGuardResult(dataworkflow.NumericConstantReuseGuardInput{
		Constants: constants,
		Uses:      uses,
	})
}

func dataTaskNumericFieldUsesFromAction(action dataquery.DataAction) []string {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	var fields []string
	switch kind {
	case dataquery.DataActionFilterRecords, dataquery.DataActionQualifyRecords:
		fields = append(fields, dataTaskNumericFilterFieldRefs(action)...)
	case dataquery.DataActionComputeContribs:
		op := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action.Params["operation"], action.Params["op"])))
		valueField := strings.TrimSpace(action.Params["value_field"])
		if valueField != "" && op != "count" {
			fields = append(fields, valueField)
		}
		fields = append(fields, dataTaskNumericFilterFieldRefs(action)...)
	}
	return cleanDataTaskStrings(fields)
}

func dataTaskNumericFilterFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	for _, spec := range dataTaskActionObjectListParam(action.Params, "filters_json", "filters") {
		op := strings.ToLower(strings.TrimSpace(dataTaskMapStringValue(spec, "op")))
		if op == "" {
			op = strings.ToLower(strings.TrimSpace(dataTaskMapStringValue(spec, "operation")))
		}
		if !dataTaskFilterOpRequiresNumeric(op) {
			continue
		}
		fields = append(fields, dataTaskFieldRefsFromSpec(spec, "field", "source_field", "input_field")...)
	}
	op := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action.Params["filter_op"], action.Params["op"])))
	if dataTaskFilterOpRequiresNumeric(op) {
		fields = append(fields, parseDataTaskActionStringListParam(action.Params["filter_field"])...)
	}
	return cleanDataTaskStrings(fields)
}

func dataTaskFilterOpRequiresNumeric(op string) bool {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "gt", "gte", "lt", "lte":
		return true
	default:
		return false
	}
}

func dataTaskActionInputsMayConsumeAliases(inputs, aliases []string) bool {
	inputs = cleanDataTaskStrings(inputs)
	aliases = cleanDataTaskStrings(aliases)
	if len(inputs) == 0 || len(aliases) == 0 {
		return false
	}
	aliasSet := map[string]bool{}
	for _, alias := range aliases {
		key := normalizeDataTaskCoveragePath(alias)
		if key != "" {
			aliasSet[key] = true
		}
	}
	for _, input := range inputs {
		if aliasSet[normalizeDataTaskCoveragePath(input)] {
			return true
		}
	}
	return false
}

func dataTaskStringSliceContainsFold(values []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), needle) {
			return true
		}
	}
	return false
}

func dataTaskStringLooksNumeric(value string) bool {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	if value == "" {
		return false
	}
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func dataTaskWorkflowCustomTransformDisabledGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	return dataTaskWorkflowCustomTransformDisabledGuardResult(records, plan).ErrorText()
}

func dataTaskWorkflowCustomTransformDisabledGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	state := dataTaskWorkflowState(records, plan)
	return dataworkflow.CustomTransformDisabledGuardResult(dataworkflow.CustomTransformDisabledGuardInput{
		RecordsPresent:      len(records) > 0,
		HasActions:          len(plan.Actions) > 0,
		HasSuccessfulResult: dataTaskWorkflowHasSuccessfulResult(records),
		State:               state,
		Actions:             plan.Actions,
	})
}

func dataTaskCustomTransformDisabledFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, bool) {
	if len(records) == 0 || guard.Empty() || !dataTaskGuardHasCode(guard, "custom_transform_disabled") {
		return dataquery.TaskPlan{}, false
	}
	state := dataTaskWorkflowState(records, plan)
	if !state.CustomTransformDisabled {
		return dataquery.TaskPlan{}, false
	}
	hasCustom := false
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform {
			hasCustom = true
			break
		}
	}
	if !hasCustom {
		return dataquery.TaskPlan{}, false
	}
	if fallback, _, ok := dataTaskWorkflowNextStageFallback(records, plan, "custom_transform disabled repair rejected"); ok {
		return fallback, true
	}
	if fallback, _, ok := dataTaskConcreteScaffoldFallback(records, plan, "custom_transform disabled repair rejected"); ok {
		return fallback, true
	}
	return dataworkflow.BuildGeneratedSchemaDiagnosticFallbackPlan(dataworkflow.GeneratedSchemaDiagnosticFallbackInput{
		Current:   plan,
		Coverage:  dataTaskWorkflowCoverageContract(records, plan),
		Output:    dataTaskWorkflowOutputContract(records, plan),
		Artifacts: dataTaskWorkflowArtifactSchemaProjections(records),
		Records:   records,
		Limit:     3,
	})
}

func dataTaskConcreteScaffoldFallback(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, reasonPrefix string) (dataquery.TaskPlan, string, bool) {
	state := dataTaskWorkflowState(records, current)
	if len(state.ActionScaffold) == 0 || len(state.AllowedNextActions) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	return dataworkflow.BuildConcreteFallbackPlan(dataworkflow.ConcreteFallbackPlanInput{
		Current:        current,
		Coverage:       dataTaskWorkflowCoverageContract(records, current),
		Output:         dataTaskWorkflowOutputContract(records, current),
		Scaffolds:      state.ActionScaffold,
		Facts:          state.Facts(),
		ReasonPrefix:   reasonPrefix,
		SeenActionKeys: dataTaskWorkflowSeenActionKeys(records),
	})
}

func dataTaskWorkflowSeenActionKeys(records []dataTaskWorkflowRecord) map[string]bool {
	var actions []dataquery.DataAction
	for _, rec := range records {
		actions = append(actions, rec.Plan.Actions...)
	}
	return dataworkflow.ActionIdempotencyKeys(actions)
}

func dataTaskWorkflowAllowedNextActionGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	return dataTaskWorkflowAllowedNextActionGuardResult(records, plan).ErrorText()
}

func dataTaskWorkflowAllowedNextActionGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	if len(records) == 0 || len(plan.Actions) == 0 {
		return dataworkflow.GuardResult{}
	}
	if !dataTaskWorkflowHasSuccessfulResult(records) {
		return dataworkflow.GuardResult{}
	}
	state := dataTaskWorkflowState(records, plan)
	if len(state.AllowedNextActions) == 0 {
		return dataworkflow.GuardResult{}
	}
	for i, action := range plan.Actions {
		if guard := dataworkflow.AllowedNextActionGuardResult(dataworkflow.AllowedNextActionGuardInput{
			State:                       state,
			Action:                      action,
			ActionIndex:                 i,
			GeneratedArtifactDiagnostic: dataTaskActionIsGeneratedArtifactDiagnostic(records, action),
			RecordMaterialization:       dataTaskActionIsRecordMaterialization(records, action),
		}); !guard.Empty() {
			return guard
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskActionIsRecordMaterialization(records []dataTaskWorkflowRecord, action dataquery.DataAction) bool {
	if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionExtractRecords {
		return false
	}
	if strings.TrimSpace(action.Script) != "" || strings.TrimSpace(action.OutputArtifact) == "" {
		return false
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	return len(inputs) > 0
}

func dataTaskWorkflowHasSuccessfulResult(records []dataTaskWorkflowRecord) bool {
	for _, rec := range records {
		if rec.Result != nil && strings.TrimSpace(rec.Err) == "" {
			return true
		}
	}
	return false
}

func dataTaskPlanActionsAllAllowedForWorkflowStage(plan dataquery.TaskPlan, state dataTaskWorkflowStateView) bool {
	if len(plan.Actions) == 0 || len(state.AllowedNextActions) == 0 {
		return false
	}
	for _, action := range plan.Actions {
		if !dataTaskWorkflowActionKindAllowed(normalizeDataActionKindForWorkflow(action.Kind), state) {
			return false
		}
	}
	return true
}

func dataTaskActionIsGeneratedArtifactDiagnostic(records []dataTaskWorkflowRecord, action dataquery.DataAction) bool {
	if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionInspectMaterial {
		return false
	}
	if strings.TrimSpace(action.Script) != "" {
		return false
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) == 0 {
		return false
	}
	generated := dataTaskWorkflowGeneratedArtifactPathSet(records)
	if len(generated) == 0 {
		return false
	}
	for _, input := range inputs {
		if !generated[normalizeDataTaskCoveragePath(input)] {
			return false
		}
	}
	return true
}

func dataTaskWorkflowActionKindAllowed(kind dataquery.DataActionKind, state dataTaskWorkflowStateView) bool {
	return dataworkflow.WorkflowActionKindAllowed(kind, state)
}

func dataTaskWorkflowStageProgressGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	return dataTaskWorkflowStageProgressGuardResult(records, plan).ErrorText()
}

func dataTaskWorkflowStageProgressGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	return dataworkflow.StageProgressGuardResult(dataworkflow.StageProgressGuardInput{
		State:                             state,
		HasScriptedCustomTransform:        dataTaskPlanHasScriptedCustomTransform(plan),
		CrossesTypedActionRanks:           dataTaskPlanCrossesTypedActionRanks(plan),
		NarrowSingleIntermediateTransform: dataTaskPlanIsNarrowSingleIntermediateCustomTransform(plan, state),
	})
}

func dataTaskWorkflowRelationNoProgressGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	return dataworkflow.RelationNoProgressGuardResult(dataworkflow.RelationNoProgressGuardInput{
		State:          dataTaskWorkflowState(records, dataquery.TaskPlan{}),
		Plan:           plan,
		ProgressEvents: dataTaskWorkflowProgressEvents(records),
		NoProgressStop: DefaultDataTaskMaxNodeFailures,
	})
}

func dataTaskWorkflowStagePrefixFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, bool) {
	prefix, _, ok := dataTaskWorkflowStagePrefixFallbackWithRemainder(records, plan, guard)
	return prefix, ok
}

func dataTaskWorkflowStagePrefixFallbackWithRemainder(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	return dataworkflow.StagePrefixFallbackWithRemainder(dataworkflow.StagePrefixFallbackInput{
		Plan:                plan,
		State:               dataTaskWorkflowState(records, plan),
		Guard:               guard,
		HasSuccessfulResult: dataTaskWorkflowHasSuccessfulResult(records),
	})
}

func dataTaskExecutablePrefixFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, bool) {
	return dataworkflow.ExecutablePrefixFallback(dataworkflow.ExecutablePrefixFallbackInput{
		Plan:  plan,
		Guard: guard,
		StagingGuard: func(candidate dataquery.TaskPlan) dataworkflow.GuardResult {
			return dataTaskWorkflowStagingGuardResult(records, candidate)
		},
	})
}

func dataTaskPopDeferredActionBatch(records []dataTaskWorkflowRecord, deferred dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	next, remainder, _, ok := dataTaskPopDeferredActionBatchWithStatus(records, deferred)
	return next, remainder, ok
}

func dataTaskPopDeferredActionBatchWithStatus(records []dataTaskWorkflowRecord, deferred dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, dataworkflow.DeferredDispatchStatus, bool) {
	if len(deferred.Actions) == 0 {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, dataworkflow.DeferredDispatchStatus{ReasonCode: dataworkflow.DeferredBlockEmpty, Reason: "deferred queue is empty"}, false
	}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if len(state.AllowedNextActions) == 0 {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, dataworkflow.DeferredDispatchStatus{Actions: len(deferred.Actions), ReasonCode: dataworkflow.DeferredBlockNoAllowedActions, Reason: "workflow has no allowed next typed actions"}, false
	}
	out, remainder, status, ok := dataworkflow.BuildDeferredDispatchPlan(dataworkflow.DeferredDispatchInput{
		Plan:               deferred,
		Candidates:         dataTaskDeferredActionCandidates(records, deferred.Actions),
		AllowedNextActions: state.AllowedNextActions,
	})
	if !ok {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, status, false
	}
	if guard := dataTaskDeferredDispatchGuardResult(records, deferred, out.Actions); !guard.Empty() {
		status.Ready = false
		status.ReasonCode = dataworkflow.DeferredBlockAdmissionRejected
		status.Reason = guard.ErrorText()
		status.GuardCode = strings.TrimSpace(guard.Code)
		status.Guard = &guard
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, status, false
	}
	status.Ready = true
	return out, remainder, status, true
}

func dataTaskDeferredDispatchGuardResult(records []dataTaskWorkflowRecord, deferred dataquery.TaskPlan, actions []dataquery.DataAction) dataworkflow.GuardResult {
	if len(actions) == 0 {
		return dataworkflow.GuardResult{}
	}
	candidate := deferred
	candidate.Status = "ready"
	candidate.Actions = append([]dataquery.DataAction(nil), actions...)
	candidate.Script = ""
	candidate.ContinueAfter = true
	return dataTaskWorkflowStagingGuardResult(records, candidate)
}

type dataTaskDeferredQueueState = dataworkflow.DeferredDispatchStatus

func dataTaskDeferredQueueStatus(records []dataTaskWorkflowRecord, deferred dataquery.TaskPlan) dataTaskDeferredQueueState {
	workflow := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	next, _, state, ok := dataworkflow.BuildDeferredDispatchPlan(dataworkflow.DeferredDispatchInput{
		Plan:               deferred,
		Candidates:         dataTaskDeferredActionCandidates(records, deferred.Actions),
		AllowedNextActions: workflow.AllowedNextActions,
	})
	if !ok {
		return state
	}
	if guard := dataTaskDeferredDispatchGuardResult(records, deferred, next.Actions); !guard.Empty() {
		state.Reason = guard.ErrorText()
		state.ReasonCode = dataworkflow.DeferredBlockAdmissionRejected
		state.GuardCode = strings.TrimSpace(guard.Code)
		state.Guard = &guard
		state.Ready = false
		return state
	}
	state.Ready = true
	return state
}

func dataTaskDeferredActionCandidates(records []dataTaskWorkflowRecord, actions []dataquery.DataAction) []dataworkflow.DeferredActionCandidate {
	return dataworkflow.BuildDeferredActionCandidates(dataworkflow.DeferredActionCandidatesInput{
		Records:           records,
		Actions:           actions,
		SchemaProjections: dataTaskWorkflowArtifactSchemaProjections(records),
	})
}

func dataTaskDeferredQueueSavedSegment(lang string, deferred dataquery.TaskPlan, reason string) string {
	reason = oneLineClamp(reason, 120)
	first := ""
	if len(deferred.Actions) > 0 {
		action := deferred.Actions[0]
		first = firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))
	}
	if isZh(lang) {
		parts := []string{fmt.Sprintf("已保存延后 typed 队列 %d 个动作", len(deferred.Actions))}
		if first != "" {
			parts = append(parts, "首个 "+first)
		}
		if reason != "" {
			parts = append(parts, reason)
		}
		return strings.Join(parts, "；")
	}
	parts := []string{fmt.Sprintf("saved deferred typed queue with %d action(s)", len(deferred.Actions))}
	if first != "" {
		parts = append(parts, "first "+first)
	}
	if reason != "" {
		parts = append(parts, reason)
	}
	return strings.Join(parts, "; ")
}

func dataTaskDeferredQueueDiscardedSegment(lang string, deferred dataquery.TaskPlan, reason string) string {
	reason = oneLineClamp(reason, 220)
	first := ""
	if len(deferred.Actions) > 0 {
		action := deferred.Actions[0]
		first = firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))
	}
	if isZh(lang) {
		parts := []string{fmt.Sprintf("延后 typed 队列已丢弃 %d 个动作", len(deferred.Actions))}
		if first != "" {
			parts = append(parts, "首个 "+first)
		}
		if reason != "" {
			parts = append(parts, reason)
		}
		return strings.Join(parts, "；")
	}
	parts := []string{fmt.Sprintf("discarded deferred typed queue with %d action(s)", len(deferred.Actions))}
	if first != "" {
		parts = append(parts, "first "+first)
	}
	if reason != "" {
		parts = append(parts, reason)
	}
	return strings.Join(parts, "; ")
}

func dataTaskDeferredQueueRetainedSegment(lang string, state dataTaskDeferredQueueState) string {
	reason := oneLineClamp(state.Reason, 180)
	action := firstNonEmptyString(state.FirstActionID, state.FirstActionKind, "deferred action")
	if isZh(lang) {
		parts := []string{fmt.Sprintf("延后队列已保留：首个 %s 尚未就绪", action)}
		if state.ReadyActions > 0 || state.BlockedActions > 0 {
			parts = append(parts, fmt.Sprintf("ready=%d blocked=%d", state.ReadyActions, state.BlockedActions))
		}
		if strings.TrimSpace(state.ReasonCode) != "" {
			parts = append(parts, "原因码 "+state.ReasonCode)
		}
		if reason != "" {
			parts = append(parts, reason)
		}
		return strings.Join(parts, "；")
	}
	parts := []string{fmt.Sprintf("retained deferred queue: first %s is not ready", action)}
	if state.ReadyActions > 0 || state.BlockedActions > 0 {
		parts = append(parts, fmt.Sprintf("ready=%d blocked=%d", state.ReadyActions, state.BlockedActions))
	}
	if strings.TrimSpace(state.ReasonCode) != "" {
		parts = append(parts, "reason_code "+state.ReasonCode)
	}
	if reason != "" {
		parts = append(parts, reason)
	}
	return strings.Join(parts, "; ")
}

func dataTaskDeferredQueueBlockedSegment(lang string, state dataTaskDeferredQueueState) string {
	reason := oneLineClamp(state.Reason, 160)
	action := firstNonEmptyString(state.FirstActionID, state.FirstActionKind, "deferred action")
	if isZh(lang) {
		return fmt.Sprintf("延后队列未执行：首个 %s 尚未就绪；ready=%d blocked=%d；%s",
			action, state.ReadyActions, state.BlockedActions, reason)
	}
	return fmt.Sprintf("deferred queue not dispatched: first %s is not ready; ready=%d blocked=%d; %s",
		action, state.ReadyActions, state.BlockedActions, reason)
}

func dataTaskSplitActionRankForState(actions []dataquery.DataAction, state dataTaskWorkflowStateView) ([]dataquery.DataAction, []dataquery.DataAction) {
	return dataworkflow.SplitActionRankForState(actions, state)
}

func dataTaskInvalidRecordActionFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, bool) {
	if guard.Empty() || len(plan.Actions) == 0 {
		return dataquery.TaskPlan{}, false
	}
	state := dataTaskWorkflowState(records, plan)
	if state.MaterialCoverageSufficient && state.NextStage != dataworkflow.StageCoverRequiredMaterials && state.NextStage != dataworkflow.StageDeriveRules {
		for _, action := range plan.Actions {
			if !dataworkflow.ActionNeedsRecordMaterialization(action) {
				continue
			}
			inputs := cleanDataTaskStrings(action.InputPaths)
			if fallback, ok := dataTaskInvalidRecordActionEnrichFallback(records, plan, action, inputs); ok {
				return fallback, true
			}
		}
		return dataquery.TaskPlan{}, false
	}
	return dataworkflow.BuildInvalidRecordMaterializationFallbackPlan(dataworkflow.InvalidRecordMaterializationFallbackInput{
		Current:      plan,
		State:        state,
		ExtractLimit: dataTaskExactExtractRecordLimit,
	})
}

func dataTaskInvalidRecordActionEnrichFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, inputs []string) (dataquery.TaskPlan, bool) {
	return dataworkflow.InvalidRecordEnrichFallbackPlan(dataworkflow.InvalidRecordEnrichFallbackInput{
		Current:           plan,
		Records:           records,
		SchemaProjections: dataTaskWorkflowArtifactSchemaProjections(records),
		Action:            action,
		InputPaths:        inputs,
	})
}

func dataTaskHistoricalMissingJoinFieldName(records []dataTaskWorkflowRecord) string {
	return dataworkflow.HistoricalMissingJoinFieldName(records)
}

func dataTaskMissingJoinFieldFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, violation dataquery.DataTaskViolation) (dataquery.TaskPlan, bool) {
	return dataworkflow.MissingJoinFieldFallbackPlan(dataworkflow.MissingJoinFieldFallbackInput{
		Current:           plan,
		Records:           records,
		SchemaProjections: dataTaskWorkflowArtifactSchemaProjections(records),
		Violation:         violation,
	})
}

func dataTaskHistoricalMissingJoinFieldFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) (dataquery.TaskPlan, bool) {
	return dataworkflow.HistoricalMissingJoinFieldFallbackPlan(dataworkflow.MissingJoinFieldFallbackInput{
		Current:           plan,
		Records:           records,
		SchemaProjections: dataTaskWorkflowArtifactSchemaProjections(records),
	})
}

func dataTaskJoinMissingFieldViolation(record dataTaskWorkflowRecord) (dataquery.DataTaskViolation, bool) {
	return dataworkflow.JoinMissingFieldViolation(record)
}

func dataTaskFallbackPlanAlreadySeen(records []dataTaskWorkflowRecord, candidate dataquery.TaskPlan) bool {
	return dataworkflow.PlanActionSignatureAlreadySeenInRecords(records, candidate)
}

func dataTaskFallbackPlanSignature(plan dataquery.TaskPlan) string {
	return dataworkflow.PlanActionSignature(plan)
}

func dataTaskArtifactAccessByAlias(access []dataTaskArtifactAccessPrompt, alias string) (dataTaskArtifactAccessPrompt, bool) {
	want := normalizeDataTaskCoveragePath(alias)
	for _, artifact := range access {
		if normalizeDataTaskCoveragePath(artifact.ID) == want && want != "" {
			return artifact, true
		}
		for _, candidate := range artifact.Aliases {
			if normalizeDataTaskCoveragePath(candidate) == want && want != "" {
				return artifact, true
			}
		}
	}
	return dataTaskArtifactAccessPrompt{}, false
}

func dataTaskPlanCrossesTypedActionRanks(plan dataquery.TaskPlan) bool {
	firstRank := 0
	for _, action := range plan.Actions {
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if kind == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != "" {
			continue
		}
		rank := dataTaskActionDependencyRank(kind)
		if rank == 0 {
			continue
		}
		if firstRank == 0 {
			firstRank = rank
			continue
		}
		if rank != firstRank {
			return true
		}
	}
	return false
}

func dataTaskActionDependencyRank(kind dataquery.DataActionKind) int {
	return dataworkflow.DependencyRank(normalizeDataActionKindForWorkflow(kind))
}

func dataTaskPlanHasScriptedCustomTransform(plan dataquery.TaskPlan) bool {
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != "" {
			return true
		}
	}
	return false
}

func dataTaskPlanIsSingleIntermediateCustomTransform(plan dataquery.TaskPlan) bool {
	if !plan.ContinueAfter || len(plan.Actions) != 1 {
		return false
	}
	action := plan.Actions[0]
	return normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != ""
}

func dataTaskPlanIsNarrowSingleIntermediateCustomTransform(plan dataquery.TaskPlan, state dataTaskWorkflowStateView) bool {
	if !dataTaskPlanIsSingleIntermediateCustomTransform(plan) {
		return false
	}
	action := plan.Actions[0]
	lines := dataTaskScriptLineCount(action.Script)
	if lines == 0 || lines >= dataTaskComplexCustomScriptLineLimit {
		return false
	}
	if dataTaskActionLooksLikeWholeWorkflow(plan, action, 0) {
		return false
	}
	if dataTaskValidationLedgerCount(plan.CoverageContract) >= 2 {
		return false
	}
	if (state.NextStage == "prepare_contribution_inputs" || state.NextStage == "compute_contributions") && len(state.MissingValidationStages()) > 1 {
		return false
	}
	return true
}

func dataTaskTextConstraintCoverageGuardError(plan dataquery.TaskPlan) string {
	return dataTaskTextConstraintCoverageGuardResult(plan).ErrorText()
}

func dataTaskTextConstraintCoverageGuardResult(plan dataquery.TaskPlan) dataworkflow.GuardResult {
	var customInputs []string
	if len(plan.Actions) == 0 {
		if strings.TrimSpace(plan.Script) != "" {
			customInputs = append(customInputs, plan.InputPaths...)
		}
	} else {
		for _, action := range plan.Actions {
			if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform || strings.TrimSpace(action.Script) == "" {
				continue
			}
			customInputs = append(customInputs, action.InputPaths...)
		}
	}
	var materials []string
	for _, material := range plan.CoverageContract.RequiredMaterials {
		if normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode) != dataquery.MaterialUseScriptConsumed {
			continue
		}
		materials = append(materials, material.Path)
	}
	return dataworkflow.TextConstraintCoverageGuardResult(dataworkflow.TextConstraintCoverageGuardInput{
		ContinueAfter:               plan.ContinueAfter,
		RuleCoverageRequired:        plan.CoverageContract.RuleCoverageRequired,
		ValidationLedgerCount:       dataTaskValidationLedgerCount(plan.CoverageContract),
		CustomInputPaths:            customInputs,
		ScriptConsumedMaterialPaths: materials,
	})
}

func dataTaskRuleCoveragePrerequisiteGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	contract := dataTaskWorkflowCoverageContract(records, plan)
	state := dataTaskWorkflowState(records, plan)
	return dataworkflow.RuleCoveragePrerequisiteGuardResult(dataworkflow.RuleCoveragePrerequisiteGuardInput{
		RuleCoverageRequired: contract.RuleCoverageRequired,
		RuleCoverageRecords:  dataTaskWorkflowRuleCoverageRecordCount(records),
		PostRuleProgress:     state.Facts().HasPostRuleProgress(),
		Actions:              plan.Actions,
		RuleMaterialPaths:    dataTaskRequiredRuleMaterialInputs(contract),
	})
}

func dataTaskWorkflowRuleCoverageRecordCount(records []dataTaskWorkflowRecord) int {
	var ruleCoverage []dataquery.RuleCoverageRecord
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		ruleCoverage = append(ruleCoverage, rec.Result.RuleCoverage...)
	}
	return len(dataquery.DedupeRuleCoverageRecords(ruleCoverage))
}

func dataTaskPathLooksLikeTextConstraintMaterial(p string) bool {
	return dataworkflow.PathLooksLikeTextConstraintMaterial(p)
}

func normalizeCoverageMaterialUseModeForWorkflow(mode dataquery.CoverageMaterialUseMode) dataquery.CoverageMaterialUseMode {
	switch dataquery.CoverageMaterialUseMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case "", dataquery.MaterialUseScriptConsumed:
		return dataquery.MaterialUseScriptConsumed
	case dataquery.MaterialUseTextEvidenceConsumed:
		return dataquery.MaterialUseTextEvidenceConsumed
	case dataquery.MaterialUsePlannerDistilled:
		return dataquery.MaterialUsePlannerDistilled
	case dataquery.MaterialUseReferenceOnly:
		return dataquery.MaterialUseReferenceOnly
	default:
		return dataquery.MaterialUseScriptConsumed
	}
}

func dataTaskTerminalRequiredMaterialSchedulingError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	return dataTaskTerminalRequiredMaterialSchedulingGuardResult(records, plan).ErrorText()
}

func dataTaskTerminalRequiredMaterialSchedulingGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	scheduled := dataTaskScheduledMaterialConsumption(records, plan)
	scheduledPaths := make([]string, 0, len(scheduled))
	for p := range scheduled {
		scheduledPaths = append(scheduledPaths, p)
	}
	return dataworkflow.RequiredMaterialSchedulingGuardResult(dataworkflow.RequiredMaterialSchedulingGuardInput{
		ContinueAfter:  plan.ContinueAfter,
		RequiredPaths:  plan.CoverageContract.RequiredRunnerInputPaths(),
		ScheduledPaths: scheduledPaths,
	})
}

func dataTaskScheduledMaterialConsumption(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) map[string]bool {
	out := map[string]bool{}
	mark := func(values []string) {
		for _, value := range cleanDataTaskStrings(values) {
			out[value] = true
		}
	}
	for _, rec := range records {
		if rec.Result != nil {
			mark(rec.Result.ConsumedPaths)
			for _, artifact := range rec.Result.Artifacts {
				mark(artifact.SourcePaths)
				mark(dataTaskArtifactAliasPaths(artifact))
			}
		}
	}
	if len(plan.Actions) == 0 {
		mark(plan.InputPaths)
	}
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionMaterialInventory {
			continue
		}
		mark(action.InputPaths)
	}
	return out
}

func dataTaskActionDependencyGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionDependencyGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionDependencyGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	if guard := dataTaskActionIntraBatchDependencyGuardResult(plan, actionIndex); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionInputAvailabilityGuardResult(records, plan, action, actionIndex); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionSchemaShapeGuardResult(records, action, actionIndex); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionFieldContractGuardResult(records, plan, action, actionIndex); !guard.Empty() {
		return guard
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if contract, ok := dataworkflow.InputPathContract(kind); ok {
		if guard := dataworkflow.InputPathContractGuardResult(dataworkflow.InputPathContractGuardInput{
			Action:      action,
			ActionIndex: actionIndex,
			Contract:    contract,
		}); !guard.Empty() {
			return guard
		}
	}
	switch kind {
	case dataquery.DataActionInspectMaterial, dataquery.DataActionExtractRecords, dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields, dataquery.DataActionGroupRecords, dataquery.DataActionExpandRecords, dataquery.DataActionFilterRecords, dataquery.DataActionQualifyRecords:
		if kind == dataquery.DataActionDeriveFields && !dataTaskDeriveFieldsActionHasSpec(action) {
			return dataworkflow.MissingActionSpecGuardResult(dataworkflow.MissingActionSpecGuardInput{Action: action, ActionIndex: actionIndex})
		}
		if kind == dataquery.DataActionExtractFields && !dataTaskDeriveFieldsActionHasSpec(action) {
			return dataworkflow.MissingActionSpecGuardResult(dataworkflow.MissingActionSpecGuardInput{Action: action, ActionIndex: actionIndex})
		}
		if kind == dataquery.DataActionGroupRecords && !dataTaskGroupRecordsActionHasSpec(action) {
			return dataworkflow.MissingActionSpecGuardResult(dataworkflow.MissingActionSpecGuardInput{Action: action, ActionIndex: actionIndex})
		}
		if kind == dataquery.DataActionExpandRecords && !dataTaskExpandRecordsActionHasSpec(action) {
			return dataworkflow.MissingActionSpecGuardResult(dataworkflow.MissingActionSpecGuardInput{Action: action, ActionIndex: actionIndex})
		}
		if kind == dataquery.DataActionFilterRecords && !dataTaskFilterRecordsActionHasSpec(action) {
			return dataworkflow.MissingActionSpecGuardResult(dataworkflow.MissingActionSpecGuardInput{Action: action, ActionIndex: actionIndex})
		}
	case dataquery.DataActionApplyResolutions:
		if len(inputs) < 2 && strings.TrimSpace(action.Params["resolution_path"]) == "" && strings.TrimSpace(action.Params["resolution_specs_json"]) == "" {
			return dataworkflow.MissingApplyResolutionInputGuardResult(action, actionIndex)
		}
	case dataquery.DataActionReconcile:
		return dataworkflow.UpstreamLedgerGuardResult(dataworkflow.MissingUpstreamLedgerGuardInput{Action: action, ActionIndex: actionIndex, Ledger: dataworkflow.LedgerContributions}, records, plan)
	case dataquery.DataActionAssembleAnswer:
		return dataworkflow.UpstreamLedgerGuardResult(dataworkflow.MissingUpstreamLedgerGuardInput{Action: action, ActionIndex: actionIndex, Ledger: dataworkflow.LedgerReconcile}, records, plan)
	}
	return dataworkflow.GuardResult{}
}

func dataTaskActionIntraBatchDependencyGuardError(plan dataquery.TaskPlan, actionIndex int) string {
	return dataTaskActionIntraBatchDependencyGuardResult(plan, actionIndex).ErrorText()
}

func dataTaskActionIntraBatchDependencyGuardResult(plan dataquery.TaskPlan, actionIndex int) dataworkflow.GuardResult {
	dependencyIndex, input, producer, ok := dataTaskFirstIntraBatchDependency(plan.Actions)
	if !ok || dependencyIndex != actionIndex {
		return dataworkflow.GuardResult{}
	}
	return dataworkflow.IntraBatchDependencyGuardResult(dataworkflow.IntraBatchDependencyGuardInput{
		Action:        plan.Actions[actionIndex],
		ActionIndex:   actionIndex,
		ConsumedInput: input,
		Producer:      producer,
	})
}

func dataTaskActionInputAvailabilityGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionInputAvailabilityGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionInputAvailabilityGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	return dataworkflow.ActionInputAvailabilityGuardResult(dataworkflow.ActionInputAvailabilityGuardInput{
		Records:     records,
		Plan:        plan,
		Action:      action,
		ActionIndex: actionIndex,
	})
}

func dataTaskActionFieldContractGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionFieldContractGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionSchemaShapeGuardResult(records []dataTaskWorkflowRecord, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	return dataworkflow.ActionSchemaShapeGuardResult(dataworkflow.ActionSchemaShapeGuardInput{
		Records:     records,
		Action:      action,
		ActionIndex: actionIndex,
	})
}

func dataTaskActionFieldContractGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	if len(records) == 0 || !dataTaskWorkflowHasSuccessfulResult(records) {
		return dataworkflow.GuardResult{}
	}
	access := dataTaskWorkflowArtifactContractAccess(records)
	if len(access) == 0 {
		return dataworkflow.GuardResult{}
	}
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	if guard := dataTaskActionZeroMatchFilterInputGuardResult(records, plan, action, actionIndex); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionUnmatchedResolutionInputGuardResult(records, plan, action, actionIndex); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionZeroEligibleInputGuardResult(records, plan, action, actionIndex); !guard.Empty() {
		return guard
	}
	if kind == dataquery.DataActionApplyResolutions {
		return dataTaskApplyResolutionActionFieldContractGuardResult(access, action, actionIndex)
	}
	return dataworkflow.ActionFieldReferenceGuardResult(dataworkflow.ActionFieldReferenceGuardInput{
		Action:            action,
		ActionIndex:       actionIndex,
		SchemaProjections: dataTaskArtifactAccessSchemaProjection(access),
	})
}

func dataTaskActionZeroMatchFilterInputGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionZeroMatchFilterInputGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionZeroMatchFilterInputGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionJoinRecords, dataquery.DataActionComputeContribs, dataquery.DataActionReconcile, dataquery.DataActionAssembleAnswer:
	default:
		return dataworkflow.GuardResult{}
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) == 0 {
		return dataworkflow.GuardResult{}
	}
	contract := dataTaskWorkflowCoverageContract(records, plan)
	if !contract.ContributionLedgerRequired && !contract.ReconcileRequired {
		return dataworkflow.GuardResult{}
	}
	state := dataTaskWorkflowStateView{
		ContributionLedgerRequired: contract.ContributionLedgerRequired,
		ReconcileRequired:          contract.ReconcileRequired,
	}
	for _, rec := range records {
		if rec.Result != nil {
			state.ContributionRecords += len(rec.Result.Contributions)
		}
	}
	issues := dataTaskWorkflowZeroMatchFilterIssues(records, state, 16)
	if len(issues) == 0 {
		return dataworkflow.GuardResult{}
	}
	for _, input := range inputs {
		inputKey := normalizeDataTaskCoveragePath(input)
		for _, issue := range issues {
			for _, alias := range dataTaskZeroMatchFilterIssueAliases(issue) {
				if inputKey != "" && inputKey == normalizeDataTaskCoveragePath(alias) {
					return dataworkflow.ZeroMatchFilterGuardResult(dataworkflow.ZeroMatchFilterGuardInput{
						Action:            action,
						ActionIndex:       actionIndex,
						InputAlias:        input,
						ArtifactID:        issue.ArtifactID,
						InputRows:         issue.InputRows,
						OutputRows:        issue.OutputRows,
						SourcePath:        issue.InputPath,
						FilterFields:      issue.FilterFields,
						Reason:            issue.Reason,
						RepairActionHints: issue.RepairActionHints,
					})
				}
			}
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskZeroMatchFilterIssueAliases(issue dataTaskZeroMatchFilterIssue) []string {
	aliases := append([]string(nil), issue.Aliases...)
	if strings.TrimSpace(issue.ArtifactID) != "" {
		aliases = append(aliases, issue.ArtifactID)
	}
	return cleanDataTaskStrings(aliases)
}

func dataTaskActionUnmatchedResolutionInputGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionUnmatchedResolutionInputGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionUnmatchedResolutionInputGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionDeriveFields,
		dataquery.DataActionExtractFields,
		dataquery.DataActionGroupRecords,
		dataquery.DataActionExpandRecords,
		dataquery.DataActionFilterRecords,
		dataquery.DataActionQualifyRecords,
		dataquery.DataActionEnrichRecords,
		dataquery.DataActionJoinRecords,
		dataquery.DataActionComputeContribs,
		dataquery.DataActionReconcile,
		dataquery.DataActionAssembleAnswer:
	default:
		return dataworkflow.GuardResult{}
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) == 0 {
		return dataworkflow.GuardResult{}
	}
	state := dataTaskWorkflowState(records, plan)
	issues := dataTaskWorkflowUnmatchedResolutionIssues(records, state, 16)
	if len(issues) == 0 {
		return dataworkflow.GuardResult{}
	}
	for _, input := range inputs {
		inputKey := normalizeDataTaskCoveragePath(input)
		for _, issue := range issues {
			for _, alias := range dataTaskUnmatchedResolutionIssueAliases(issue) {
				if inputKey != "" && inputKey == normalizeDataTaskCoveragePath(alias) {
					return dataworkflow.UnmatchedResolutionGuardResult(dataworkflow.UnmatchedResolutionGuardInput{
						Action:            action,
						ActionIndex:       actionIndex,
						InputAlias:        input,
						ArtifactID:        issue.ArtifactID,
						BasePath:          issue.BasePath,
						BaseRows:          issue.BaseRows,
						TargetFields:      issue.TargetFields,
						Reason:            issue.Reason,
						RepairActionHints: issue.RepairActionHints,
					})
				}
			}
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskUnmatchedResolutionIssueAliases(issue dataTaskUnmatchedResolutionIssue) []string {
	aliases := append([]string(nil), issue.Aliases...)
	if strings.TrimSpace(issue.ArtifactID) != "" {
		aliases = append(aliases, issue.ArtifactID)
	}
	return cleanDataTaskStrings(aliases)
}

func dataTaskActionZeroEligibleInputGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionZeroEligibleInputGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionZeroEligibleInputGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionJoinRecords, dataquery.DataActionComputeContribs, dataquery.DataActionReconcile, dataquery.DataActionAssembleAnswer:
	default:
		return dataworkflow.GuardResult{}
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) == 0 {
		return dataworkflow.GuardResult{}
	}
	state := dataTaskWorkflowState(records, plan)
	issues := dataTaskWorkflowZeroEligibleIssues(records, state, 16)
	if len(issues) == 0 {
		return dataworkflow.GuardResult{}
	}
	for _, input := range inputs {
		inputKey := normalizeDataTaskCoveragePath(input)
		for _, issue := range issues {
			for _, alias := range dataTaskZeroEligibleIssueAliases(issue) {
				if inputKey != "" && inputKey == normalizeDataTaskCoveragePath(alias) {
					return dataworkflow.ZeroEligibleGuardResult(dataworkflow.ZeroEligibleGuardInput{
						Action:            action,
						ActionIndex:       actionIndex,
						InputAlias:        input,
						ArtifactID:        issue.ArtifactID,
						InputRows:         issue.InputRows,
						EligibleRows:      issue.EligibleRows,
						Reason:            issue.Reason,
						RepairActionHints: issue.RepairActionHints,
					})
				}
			}
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskZeroEligibleIssueAliases(issue dataTaskZeroEligibleIssue) []string {
	aliases := append([]string(nil), issue.Aliases...)
	if strings.TrimSpace(issue.ArtifactID) != "" {
		aliases = append(aliases, issue.ArtifactID)
	}
	return cleanDataTaskStrings(aliases)
}

func dataTaskActionMissingFieldContractMessage(action dataquery.DataAction, actionIndex int, input string, missing []string, access []dataTaskArtifactAccessPrompt) string {
	return dataTaskActionMissingFieldContractGuardResult(action, actionIndex, input, missing, access).ErrorText()
}

func dataTaskActionMissingFieldContractGuardResult(action dataquery.DataAction, actionIndex int, input string, missing []string, access []dataTaskArtifactAccessPrompt) dataworkflow.GuardResult {
	return dataworkflow.MissingFieldContractGuardResult(dataworkflow.MissingFieldContractGuardInput{
		Action:            action,
		ActionIndex:       actionIndex,
		InputAlias:        input,
		MissingFields:     missing,
		SchemaProjections: dataTaskArtifactAccessSchemaProjection(access),
	})
}

func dataTaskActionInputContractGuardResult(code string, action dataquery.DataAction, inputAlias string, missingFields []string, message string, hints []string) dataworkflow.GuardResult {
	return dataworkflow.ActionInputContractGuardResult(dataworkflow.ActionInputContractGuardInput{
		Code:              code,
		Action:            action,
		InputAlias:        inputAlias,
		MissingFields:     missingFields,
		Message:           message,
		RepairActionHints: hints,
	})
}

func dataTaskMissingFieldsOnArtifact(access []dataTaskArtifactAccessPrompt, alias string, fields []string) []string {
	missing := dataworkflow.MissingFieldsOnArtifactSchema(dataTaskArtifactAccessSchemaProjection(access), alias, fields)
	sort.Strings(missing)
	return missing
}

func dataTaskArtifactAccessSchemaProjection(access []dataTaskArtifactAccessPrompt) []dataworkflow.ArtifactSchemaProjection {
	return dataworkflow.ArtifactAccessSchemaProjections(access)
}

func dataTaskFilterActionFieldRefs(action dataquery.DataAction) []string {
	return dataworkflow.FilterActionFieldRefs(action)
}

func dataTaskApplyResolutionActionFieldContractGuardError(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) string {
	return dataTaskApplyResolutionActionFieldContractGuardResult(access, action, actionIndex).ErrorText()
}

func dataTaskApplyResolutionActionFieldContractGuardResult(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	return dataworkflow.ApplyResolutionContractGuardResult(dataworkflow.ApplyResolutionContractGuardInput{
		Action:            action,
		ActionIndex:       actionIndex,
		SchemaProjections: dataTaskArtifactAccessSchemaProjection(access),
	})
}

func firstDataTaskActionInput(action dataquery.DataAction, index int) string {
	inputs := cleanDataTaskStrings(action.InputPaths)
	if index < 0 || index >= len(inputs) {
		return ""
	}
	return inputs[index]
}

func dataTaskActionObjectListParam(params map[string]string, keys ...string) []map[string]any {
	for _, key := range keys {
		raw := strings.TrimSpace(params[key])
		if raw == "" {
			continue
		}
		if objects := dataTaskParseObjectList(raw); len(objects) > 0 {
			return objects
		}
	}
	return nil
}

func dataTaskParseObjectList(raw string) []map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(raw), &list); err == nil {
		return list
	}
	var one map[string]any
	if err := json.Unmarshal([]byte(raw), &one); err == nil {
		return []map[string]any{one}
	}
	return nil
}

func dataTaskFieldRefsFromSpec(spec map[string]any, keys ...string) []string {
	var out []string
	for _, key := range keys {
		value, ok := spec[key]
		if !ok {
			continue
		}
		out = append(out, dataTaskAnyStringList(value)...)
	}
	return cleanDataTaskStrings(out)
}

func dataTaskMapStringValue(spec map[string]any, key string) string {
	value, ok := spec[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.12f", typed), "0"), ".")
	case bool:
		return fmt.Sprintf("%t", typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func dataTaskAnyStringList(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return parseDataTaskActionStringListParam(typed)
	case []string:
		return cleanDataTaskStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, dataTaskAnyStringList(item)...)
		}
		return cleanDataTaskStrings(out)
	default:
		return cleanDataTaskStrings([]string{fmt.Sprint(typed)})
	}
}

func dataTaskDeriveFieldsActionHasSpec(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	for _, key := range []string{
		"field_specs_json", "extract_specs_json", "derive_specs_json", "transforms_json",
		"field_specs", "extract_specs", "derive_specs", "transforms",
		"source_field", "input_field", "field",
		"target_field", "output_field", "derived_field",
		"operation", "op", "transform",
	} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return true
		}
	}
	return false
}

func dataTaskExpandRecordsActionHasSpec(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	for _, key := range []string{"source_field", "input_field", "field", "value_field"} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return true
		}
	}
	return false
}

func dataTaskGroupRecordsActionHasSpec(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	hasGroup := parseBoolDataTaskActionParam(action.Params["group_all"])
	if !hasGroup {
		for _, key := range []string{"group_fields", "group_by_fields", "key_fields", "group_field", "group_by", "key_field"} {
			if strings.TrimSpace(action.Params[key]) != "" {
				hasGroup = true
				break
			}
		}
	}
	if !hasGroup {
		return false
	}
	for _, key := range []string{"text_fields", "concat_fields", "aggregate_fields", "source_fields", "text_field", "source_field", "field"} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return true
		}
	}
	return true
}

func parseBoolDataTaskActionParam(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func dataTaskFilterRecordsActionHasSpec(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	for _, key := range []string{"filters_json", "filters", "filter_field"} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return true
		}
	}
	return false
}

func dataTaskCoverageLoopGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	return dataTaskCoverageLoopGuardResult(records, plan).ErrorText()
}

func dataTaskCoverageLoopGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	state := dataTaskWorkflowState(records, plan)
	return dataworkflow.CoverageLoopGuardResult(dataworkflow.CoverageLoopGuardInput{
		State:                            state,
		CoverageOnly:                     dataTaskPlanIsCoverageOnly(plan),
		GeneratedArtifactDiagnostic:      dataTaskPlanIsGeneratedArtifactDiagnostic(records, plan),
		ActionsAllAllowedForCurrentStage: dataTaskPlanActionsAllAllowedForWorkflowStage(plan, state),
	})
}

func dataTaskPlanIsGeneratedArtifactDiagnostic(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) bool {
	if len(plan.Actions) == 0 || strings.TrimSpace(plan.Script) != "" {
		return false
	}
	for _, action := range plan.Actions {
		if !dataTaskActionIsGeneratedArtifactDiagnostic(records, action) {
			return false
		}
	}
	return true
}

func dataTaskRepeatedCustomTransformGuardError(records []dataTaskWorkflowRecord, action dataquery.DataAction) string {
	return dataworkflow.RepeatedCustomTransformGuardResult(action, dataTaskWorkflowErrorTexts(records), DefaultDataTaskMaxNodeFailures).ErrorText()
}

func dataTaskRepeatedCustomTransformClassGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	broad := dataTaskActionHasBroadPrerequisiteSurface(plan, action) || dataTaskActionLooksLikeWholeWorkflow(plan, action, actionIndex)
	return dataworkflow.RepeatedCustomTransformClassGuardResult(
		action,
		broad,
		dataTaskCustomTransformFailureErrors(records),
		DefaultDataTaskMaxCustomTransformClassFailures,
		DefaultDataTaskMaxNodeFailures,
	).ErrorText()
}

func dataTaskCustomTransformFailureClassStats(records []dataTaskWorkflowRecord) (total int, topCode string, topCodeCount int) {
	return dataworkflow.CustomTransformFailureClassStats(dataTaskCustomTransformFailureErrors(records))
}

func dataTaskWorkflowErrorTexts(records []dataTaskWorkflowRecord) []string {
	var out []string
	for _, rec := range records {
		if strings.TrimSpace(rec.Err) == "" {
			continue
		}
		out = append(out, rec.Err)
	}
	return out
}

func dataTaskCustomTransformFailureErrors(records []dataTaskWorkflowRecord) []string {
	var out []string
	for _, rec := range records {
		if strings.TrimSpace(rec.Err) == "" || !dataTaskRecordHasCustomTransformScript(rec) {
			continue
		}
		out = append(out, rec.Err)
	}
	return out
}

func dataTaskRecordHasCustomTransformScript(rec dataTaskWorkflowRecord) bool {
	if strings.TrimSpace(rec.Plan.Script) != "" {
		return true
	}
	return dataTaskPlanHasCustomScript(rec.Plan.Actions, len(rec.Plan.Actions))
}

func dataTaskCustomScriptActionCount(actions []dataquery.DataAction) int {
	count := 0
	for _, action := range actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		if strings.TrimSpace(action.Script) == "" {
			continue
		}
		count++
	}
	return count
}

func normalizeDataActionKindForWorkflow(kind dataquery.DataActionKind) dataquery.DataActionKind {
	return dataworkflow.NormalizeActionKind(kind)
}

func dataTaskActionLooksLikeWholeWorkflow(plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) bool {
	if dataTaskPlanHasTypedActionContext(plan.Actions, actionIndex) &&
		dataTaskScriptLineCount(action.Script) < dataTaskOneShotScriptLineSoftLimit &&
		len(cleanDataTaskStrings(action.InputPaths)) <= 1 &&
		len(plan.CoverageContract.RequiredMaterials) <= 1 &&
		dataTaskValidationLedgerCount(plan.CoverageContract) <= 1 {
		return false
	}
	return len(action.InputPaths) >= 4 ||
		len(plan.CoverageContract.RequiredMaterials) >= 4 ||
		dataTaskValidationLedgerCount(plan.CoverageContract) >= 2
}

func dataTaskActionHasBroadPrerequisiteSurface(plan dataquery.TaskPlan, action dataquery.DataAction) bool {
	inputs := cleanDataTaskStrings(action.InputPaths)
	lines := dataTaskScriptLineCount(action.Script)
	return len(inputs) >= 4 ||
		(lines >= dataTaskComplexCustomScriptLineLimit && dataTaskValidationLedgerCount(plan.CoverageContract) >= 2)
}

func dataTaskTerminalRawMaterialCustomTransformGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex, lines int) string {
	return dataTaskTerminalRawMaterialCustomTransformGuardResult(records, plan, action, actionIndex, lines).ErrorText()
}

func dataTaskTerminalRawMaterialCustomTransformGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex, lines int) dataworkflow.GuardResult {
	state := dataTaskWorkflowState(records, plan)
	return dataworkflow.TerminalRawMaterialCustomTransformGuardResult(dataworkflow.TerminalRawMaterialCustomTransformGuardInput{
		RecordsPresent:         len(records) > 0,
		ContinueAfter:          plan.ContinueAfter,
		State:                  state,
		Action:                 action,
		ActionIndex:            actionIndex,
		ScriptLines:            lines,
		RawInputAliases:        dataTaskCustomTransformRawMaterialInputs(records, action),
		ComplexScriptLineLimit: dataTaskComplexCustomScriptLineLimit,
	})
}

func dataTaskCustomTransformRawMaterialInputs(records []dataTaskWorkflowRecord, action dataquery.DataAction) []string {
	generated := dataTaskWorkflowGeneratedArtifactPathSet(records)
	var raw []string
	for _, input := range cleanDataTaskStrings(action.InputPaths) {
		normalized := normalizeDataTaskCoveragePath(input)
		if normalized == "" || generated[normalized] {
			continue
		}
		raw = append(raw, input)
	}
	sort.Strings(raw)
	return raw
}

func dataTaskBroadCustomPrerequisiteGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	return dataTaskBroadCustomPrerequisiteGuardResult(records, plan, dataquery.DataAction{}, -1).ErrorText()
}

func dataTaskBroadCustomPrerequisiteGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, target dataquery.DataAction, targetIndex int) dataworkflow.GuardResult {
	if len(plan.Actions) == 0 {
		return dataworkflow.GuardResult{}
	}
	for i, action := range plan.Actions {
		if targetIndex >= 0 && i != targetIndex {
			continue
		}
		if targetIndex < 0 && strings.TrimSpace(target.ID) != "" && strings.TrimSpace(action.ID) != strings.TrimSpace(target.ID) {
			continue
		}
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		missing := dataTaskMissingCustomTransformPrerequisites(records, plan, action, i)
		guard := dataworkflow.BroadCustomPrerequisiteGuardResult(dataworkflow.BroadCustomPrerequisiteGuardInput{
			Action:      action,
			ActionIndex: i,
			IsBroad:     dataTaskActionHasBroadPrerequisiteSurface(plan, action),
			Missing:     missing,
		})
		if !guard.Empty() {
			return guard
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskMissingCustomTransformPrerequisites(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) []string {
	required := dataTaskCustomTransformPrerequisitePaths(plan, action)
	if len(required) == 0 {
		return nil
	}
	covered := dataTaskWorkflowCoveredMaterialPaths(records, plan, actionIndex)
	var missing []string
	for _, p := range required {
		if p == "" || dataTaskCoveragePathCovered(covered, p) {
			continue
		}
		missing = append(missing, p)
	}
	sort.Strings(missing)
	return missing
}

func dataTaskCustomTransformPrerequisitePaths(plan dataquery.TaskPlan, action dataquery.DataAction) []string {
	paths := append([]string(nil), action.InputPaths...)
	return cleanDataTaskStrings(paths)
}

func dataTaskWorkflowCoveredMaterialPaths(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, beforeActionIndex int) map[string]bool {
	return dataworkflow.CoveredMaterialPaths(records, plan, beforeActionIndex)
}

func dataTaskCoveragePathCovered(covered map[string]bool, required string) bool {
	required = normalizeDataTaskCoveragePath(required)
	if required == "" {
		return true
	}
	if covered[required] {
		return true
	}
	if path.Ext(required) != "" {
		return false
	}
	prefix := strings.TrimSuffix(required, "/") + "/"
	for value := range covered {
		value = normalizeDataTaskCoveragePath(value)
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func dataTaskActionOutputAliases(action dataquery.DataAction) []string {
	var out []string
	if id := strings.TrimSpace(action.ID); id != "" {
		out = append(out, id)
	}
	if artifact := strings.TrimSpace(action.OutputArtifact); artifact != "" {
		out = append(out, artifact, path.Base(artifact))
	}
	return cleanDataTaskStrings(out)
}

func dataTaskPlanHasTypedActionContext(actions []dataquery.DataAction, beforeIndex int) bool {
	return dataTaskPlanNonCustomActionCount(actions, beforeIndex) >= 2
}

func dataTaskPlanNonCustomActionCount(actions []dataquery.DataAction, beforeIndex int) int {
	if beforeIndex > len(actions) {
		beforeIndex = len(actions)
	}
	count := 0
	for i := 0; i < beforeIndex; i++ {
		if normalizeDataActionKindForWorkflow(actions[i].Kind) == dataquery.DataActionCustomTransform {
			continue
		}
		count++
	}
	return count
}

func dataTaskPlanHasCustomScript(actions []dataquery.DataAction, beforeIndex int) bool {
	if beforeIndex > len(actions) {
		beforeIndex = len(actions)
	}
	for i := 0; i < beforeIndex; i++ {
		if normalizeDataActionKindForWorkflow(actions[i].Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		if strings.TrimSpace(actions[i].Script) != "" {
			return true
		}
	}
	return false
}

func dataTaskScriptHasResultEmitter(script string) bool {
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "emit_result(") || strings.Contains(trimmed, "emit(") {
			return true
		}
		if strings.HasPrefix(trimmed, "result =") || strings.HasPrefix(trimmed, "result=") {
			return true
		}
	}
	return false
}

func dataTaskValidationLedgerCount(contract dataquery.CoverageContract) int {
	n := 0
	if contract.DecisionRecordsRequired {
		n++
	}
	if contract.RuleCoverageRequired {
		n++
	}
	if contract.ContributionLedgerRequired {
		n++
	}
	if contract.EntityResolutionRequired {
		n++
	}
	if contract.ReconcileRequired {
		n++
	}
	return n
}

type dataTaskWorkflowRecord = dataworkflow.WorkflowRecord

type dataTaskWorkflowPlanPreflight = dataworkflow.ActionDAGAdmissionDecision

const dataTaskPreflightMaxRewrites = 3

func dataTaskPreflightWorkflowPlan(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, protect func(dataquery.TaskPlan) dataquery.TaskPlan) dataTaskWorkflowPlanPreflight {
	return dataworkflow.AdmitActionDAGPlan(dataTaskPreflightWorkflowPlanInput(records, plan, protect))
}

func dataTaskPreflightWorkflowPlanInput(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, protect func(dataquery.TaskPlan) dataquery.TaskPlan) dataworkflow.ActionDAGAdmissionInput {
	return dataworkflow.ActionDAGAdmissionInput{
		Plan:        plan,
		Protect:     protect,
		MaxRewrites: dataTaskPreflightMaxRewrites,
		PrefixFallback: func(current dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, string, bool) {
			fallback, remainder, ok := dataTaskInitialRankPrefixFallback(records, current)
			if !ok {
				return dataquery.TaskPlan{}, dataquery.TaskPlan{}, "", false
			}
			return fallback, remainder, "split initial data plan at typed dependency rank", true
		},
		Guard: func(current dataquery.TaskPlan) dataworkflow.GuardResult {
			return dataTaskWorkflowStagingGuardResult(records, current)
		},
		DeterministicFallback: func(current dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, dataquery.TaskPlan, string, bool) {
			return dataTaskWorkflowDeterministicFallback(records, current, guard)
		},
	}
}

func dataTaskAdmissionDecisionForPlan(decision dataworkflow.ActionDAGAdmissionDecision, plan dataquery.TaskPlan) *dataworkflow.ActionDAGAdmissionDecision {
	if !dataTaskAdmissionPlanMatches(decision.Plan, plan) {
		return nil
	}
	copied := decision
	return &copied
}

func dataTaskWorkflowRecordForGuard(plan dataquery.TaskPlan, guard dataworkflow.GuardResult) dataTaskWorkflowRecord {
	errText := guard.ErrorText()
	return dataTaskWorkflowRecord{
		Plan:      plan,
		Err:       errText,
		Admission: dataTaskAdmissionDecisionForGuard(plan, guard),
	}
}

func dataTaskAdmissionDecisionForGuard(plan dataquery.TaskPlan, guard dataworkflow.GuardResult) *dataworkflow.ActionDAGAdmissionDecision {
	if guard.Empty() {
		return nil
	}
	copiedPlan := plan
	decision := dataworkflow.ActionDAGAdmissionDecision{
		Plan:          copiedPlan,
		Original:      copiedPlan,
		Guard:         guard,
		FinalGuard:    guard,
		GuardErr:      guard.ErrorText(),
		FinalGuardErr: guard.ErrorText(),
	}
	return &decision
}

func dataTaskAdmissionPlanMatches(a, b dataquery.TaskPlan) bool {
	if len(a.Actions) != len(b.Actions) {
		return false
	}
	if strings.TrimSpace(a.Status) != strings.TrimSpace(b.Status) {
		return false
	}
	if strings.TrimSpace(a.Script) != strings.TrimSpace(b.Script) {
		return false
	}
	for i := range a.Actions {
		if dataworkflow.ActionIdempotencyKey(a.Actions[i]) != dataworkflow.ActionIdempotencyKey(b.Actions[i]) {
			return false
		}
	}
	return true
}

type dataTaskWorkflowPromptRecord struct {
	Round               int                       `json:"round"`
	PlanStatus          string                    `json:"plan_status,omitempty"`
	Goal                string                    `json:"goal,omitempty"`
	Actions             []dataquery.DataAction    `json:"actions,omitempty"`
	InputPaths          []string                  `json:"input_paths,omitempty"`
	OutputContract      dataquery.OutputContract  `json:"output_contract,omitempty"`
	SuccessCriteria     []string                  `json:"success_criteria,omitempty"`
	MissingObservations []string                  `json:"missing_observations,omitempty"`
	NextBatch           string                    `json:"next_batch,omitempty"`
	WhyThisBatch        string                    `json:"why_this_batch,omitempty"`
	ContinueAfter       bool                      `json:"continue_after,omitempty"`
	ScriptPreview       string                    `json:"script_preview,omitempty"`
	Result              *dataTaskResultPromptView `json:"result,omitempty"`
	Error               string                    `json:"error,omitempty"`
	Evaluation          *dataquery.Evaluation     `json:"evaluation,omitempty"`
}

type dataTaskWorkflowStateView = dataworkflow.WorkflowStateView

type dataTaskActionContract = dataworkflow.ActionContract

type dataTaskFieldContractViolation = dataworkflow.FieldContractIssue

type dataTaskZeroMatchFilterIssue = dataworkflow.ZeroMatchFilterIssue

type dataTaskUnmatchedResolutionIssue = dataworkflow.UnmatchedResolutionIssue

type dataTaskZeroEligibleIssue = dataworkflow.ZeroEligibleIssue

type dataTaskResultPromptView = dataworkflow.ResultPromptView

type dataTaskLedgerProjection = dataworkflow.LedgerProjection

type dataTaskArtifactAccessPrompt = dataworkflow.ArtifactAccessView

type dataTaskMaterialSetHandlePrompt = dataworkflow.MaterialCollectionView

func renderDataTaskRecordsForPrompt(records []dataTaskWorkflowRecord) string {
	return renderDataTaskRecordsForPromptWithBudget(records, dataTaskPromptRecordBudget{
		MaxRecords:          6,
		MaxActions:          8,
		ActionScriptLimit:   1200,
		ActionParamLimit:    1200,
		ScriptPreviewLimit:  1800,
		ErrorLimit:          1600,
		EvalReasonLimit:     1000,
		ResultAnswerLimit:   2400,
		ResultAuditLimit:    1000,
		ResultDecisionLimit: 6,
		ResultRuleLimit:     4,
		ResultContribLimit:  6,
		ResultArtifactLimit: 6,
		ResultAccessLimit:   10,
		ResultMaterialLimit: 8,
	})
}

func renderCompactDataTaskRecordsForPrompt(records []dataTaskWorkflowRecord) string {
	return renderDataTaskRecordsForPromptWithBudget(records, dataTaskPromptRecordBudget{
		MaxRecords:          3,
		MaxActions:          4,
		ActionScriptLimit:   320,
		ActionParamLimit:    480,
		ScriptPreviewLimit:  480,
		ErrorLimit:          700,
		EvalReasonLimit:     600,
		ResultAnswerLimit:   700,
		ResultAuditLimit:    500,
		ResultDecisionLimit: 2,
		ResultRuleLimit:     2,
		ResultContribLimit:  3,
		ResultArtifactLimit: 4,
		ResultAccessLimit:   6,
		ResultMaterialLimit: 4,
	})
}

type dataTaskPromptRecordBudget struct {
	MaxRecords          int
	MaxActions          int
	ActionScriptLimit   int
	ActionParamLimit    int
	ScriptPreviewLimit  int
	ErrorLimit          int
	EvalReasonLimit     int
	ResultAnswerLimit   int
	ResultAuditLimit    int
	ResultDecisionLimit int
	ResultRuleLimit     int
	ResultContribLimit  int
	ResultArtifactLimit int
	ResultAccessLimit   int
	ResultMaterialLimit int
}

func renderDataTaskRecordsForPromptWithBudget(records []dataTaskWorkflowRecord, budget dataTaskPromptRecordBudget) string {
	return dataworkflow.RenderWorkflowRecordViewsForPrompt(records, dataworkflow.WorkflowRecordViewBudget{
		MaxRecords:          budget.MaxRecords,
		MaxActions:          budget.MaxActions,
		ActionScriptLimit:   budget.ActionScriptLimit,
		ActionParamLimit:    budget.ActionParamLimit,
		ScriptPreviewLimit:  budget.ScriptPreviewLimit,
		ErrorLimit:          budget.ErrorLimit,
		EvalReasonLimit:     budget.EvalReasonLimit,
		ResultAnswerLimit:   budget.ResultAnswerLimit,
		ResultAuditLimit:    budget.ResultAuditLimit,
		ResultDecisionLimit: budget.ResultDecisionLimit,
		ResultRuleLimit:     budget.ResultRuleLimit,
		ResultContribLimit:  budget.ResultContribLimit,
		ResultArtifactLimit: budget.ResultArtifactLimit,
		ResultAccessLimit:   budget.ResultAccessLimit,
		ResultMaterialLimit: budget.ResultMaterialLimit,
	}, func(result dataquery.Result, budget dataworkflow.WorkflowRecordViewBudget) any {
		return compactDataTaskResultPromptViewWithArtifactLimits(result,
			budget.ResultAnswerLimit,
			budget.ResultAuditLimit,
			budget.ResultDecisionLimit,
			budget.ResultRuleLimit,
			budget.ResultContribLimit,
			budget.ResultArtifactLimit,
			budget.ResultAccessLimit,
			budget.ResultMaterialLimit,
		)
	})
}

func dataTaskWorkflowState(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataTaskWorkflowStateView {
	return dataTaskWorkflowStateWithDeferred(records, current, dataquery.TaskPlan{})
}

type dataTaskWorkflowRuntimeView = dataworkflow.WorkflowRuntimeView

func dataTaskWorkflowRuntimeViewFrom(rt *dataworkflow.WorkflowRuntime, fallbackRecords []dataTaskWorkflowRecord, fallbackCurrent, fallbackDeferred dataquery.TaskPlan, fallbackDataRounds, fallbackRepairRounds int) dataTaskWorkflowRuntimeView {
	return dataworkflow.BuildWorkflowRuntimeView(dataworkflow.WorkflowRuntimeViewInput{
		Runtime:              rt,
		FallbackRecords:      fallbackRecords,
		FallbackCurrent:      fallbackCurrent,
		FallbackDeferred:     fallbackDeferred,
		FallbackDataRounds:   fallbackDataRounds,
		FallbackRepairRounds: fallbackRepairRounds,
	})
}

func dataTaskWorkflowStateFromRuntimeView(view dataTaskWorkflowRuntimeView) dataTaskWorkflowStateView {
	return dataTaskWorkflowStateWithDeferredQueue(view.Records, view.CurrentPlan, view.DeferredQueue)
}

func dataTaskPlanHasRuntimeShape(plan dataquery.TaskPlan) bool {
	return dataworkflow.TaskPlanHasRuntimeShape(plan)
}

func dataTaskWorkflowStateWithDeferred(records []dataTaskWorkflowRecord, current, deferred dataquery.TaskPlan) dataTaskWorkflowStateView {
	return dataTaskWorkflowStateWithDeferredQueue(records, current, dataworkflow.NewDeferredQueue(deferred))
}

func dataTaskWorkflowStateWithDeferredQueue(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, deferredQueue dataworkflow.DeferredQueueState) dataTaskWorkflowStateView {
	contract := dataTaskWorkflowCoverageContract(records, current)
	currentBatchContract, currentBatchLayer := dataTaskWorkflowCurrentBatchContract(records, current, contract)
	outputContract := dataTaskWorkflowOutputContract(records, current)
	covered := dataTaskWorkflowCoveredMaterialPaths(records, dataquery.TaskPlan{}, 0)
	var decisionRecords []dataquery.RowDecision
	var ruleCoverageRecords []dataquery.RuleCoverageRecord
	var contributionRecords []dataquery.ContributionRecord
	var entityResolutionRecords []dataquery.EntityResolutionRecord
	hasReconcile := false
	hasAnswer := false
	hasProjectionArtifact := false
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		ruleCoverageRecords = append(ruleCoverageRecords, rec.Result.RuleCoverage...)
		decisionRecords = append(decisionRecords, rec.Result.Rows...)
		entityResolutionRecords = append(entityResolutionRecords, rec.Result.EntityResolutions...)
		contributionRecords = append(contributionRecords, rec.Result.Contributions...)
		if rec.Result.Reconcile != nil {
			hasReconcile = true
		}
		if dataworkflow.ResultIsFinalAnswerCandidate(rec.Plan, *rec.Result, contract, outputContract) {
			hasAnswer = true
		}
		if dataworkflow.ResultHasAssembleAnswerArtifact(*rec.Result) {
			hasProjectionArtifact = true
		}
	}
	customTransformFailures, _, _ := dataTaskCustomTransformFailureClassStats(records)
	artifactAvailability, artifactAvailabilityCount, artifactAvailabilityTruncated := dataTaskWorkflowArtifactAvailability(records, 48)
	dedupedEntityResolutions := dataquery.DedupeEntityResolutionRecords(entityResolutionRecords)
	var latestEvaluation *dataquery.Evaluation
	if eval, ok := latestDataTaskEvaluation(records); ok {
		copied := eval
		latestEvaluation = &copied
	}
	guardViolations := dataTaskWorkflowGuardViolations(records)
	return dataworkflow.BuildWorkflowStateView(dataworkflow.WorkflowStateViewBuildInput{
		Records:              records,
		Current:              current,
		DeferredQueue:        deferredQueue,
		WorkflowContract:     contract,
		CurrentBatchContract: currentBatchContract,
		CurrentBatchLayer:    currentBatchLayer,
		OutputContract:       outputContract,
		CoveredMaterialPaths: covered,
		HasMaterialProgress:  dataTaskWorkflowHasMaterialProgress(records),
		LedgerCounts: dataworkflow.WorkflowStateLedgerCounts{
			RuleCoverageRecords:     len(dataquery.DedupeRuleCoverageRecords(ruleCoverageRecords)),
			DecisionRecords:         len(dataquery.DedupeRowDecisionRecords(decisionRecords)),
			EntityResolutionRecords: len(dedupedEntityResolutions),
			EntityStageMaterialized: len(dedupedEntityResolutions) > 0 || dataTaskWorkflowEntityStageMaterialized(records),
			ContributionRecords:     len(dataquery.DedupeContributionRecords(contributionRecords)),
			HasReconcile:            hasReconcile,
			HasAnswer:               hasAnswer,
			HasProjectionArtifact:   hasProjectionArtifact,
		},
		CustomTransformFailures: customTransformFailures,
		CustomTransformDisabled: dataTaskCustomTransformCooldown(records),
		CustomTransformDisabledFunc: func(state dataworkflow.WorkflowStateView) bool {
			return dataTaskCustomTransformCooldownForState(records, state)
		},
		ArtifactAvailability:          artifactAvailability,
		ArtifactAvailabilityCount:     artifactAvailabilityCount,
		ArtifactAvailabilityTruncated: artifactAvailabilityTruncated,
		ActionScaffoldArtifacts:       dataTaskWorkflowArtifactSchemaProjections(records),
		ActionScaffoldLimit:           10,
		DiagnosticArtifactAccess:      dataTaskWorkflowArtifactContractAccess(records),
		DiagnosticBatches:             dataTaskArtifactDiagnosticBatches(records),
		DiagnosticLimit:               4,
		GuardViolations:               guardViolations,
		NoProgressThreshold:           DefaultDataTaskMaxNodeFailures,
		LatestEvaluation:              latestEvaluation,
		ArtifactLimit:                 48,
		ProgressLimit:                 6,
		ActionEventLimit:              48,
	})
}

func dataTaskWorkflowGuardViolations(records []dataTaskWorkflowRecord) []dataworkflow.WorkflowViolation {
	return dataworkflow.ActiveGuardViolationsFromRecords(records)
}

func dataTaskRecentJoinNoProgressCount(records []dataTaskWorkflowRecord) int {
	count, kinds := dataTaskRecentRelationNoProgressCount(records)
	if len(kinds) == 1 && kinds[0] == string(dataquery.DataActionJoinRecords) {
		return count
	}
	return 0
}

func dataTaskRecentRelationNoProgressCount(records []dataTaskWorkflowRecord) (int, []string) {
	return dataworkflow.RecentRelationNoProgressCount(dataTaskWorkflowProgressEvents(records))
}

func dataTaskWorkflowProgressEvents(records []dataTaskWorkflowRecord) []dataworkflow.ProgressEvent {
	return dataworkflow.ProgressEventsFromRecords(records)
}

func dataTaskPlanSingleRelationMaterializationKind(plan dataquery.TaskPlan) (string, bool) {
	return dataworkflow.SingleRelationMaterializationKind(plan.Actions)
}

func dataTaskActionKindIsRelationMaterialization(kind dataquery.DataActionKind) bool {
	return dataworkflow.IsRelationMaterialization(kind)
}

func dataTaskPlanHasOnlyActionKind(plan dataquery.TaskPlan, kind dataquery.DataActionKind) bool {
	if len(plan.Actions) == 0 {
		return false
	}
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != kind {
			return false
		}
	}
	return true
}

func dataTaskWorkflowActionGraph(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, limit int) dataworkflow.ActionGraph {
	return dataTaskWorkflowActionGraphWithDeferredAndViolations(records, current, dataquery.TaskPlan{}, nil, limit)
}

func dataTaskWorkflowActionGraphWithViolations(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, violations []dataworkflow.WorkflowViolation, limit int) dataworkflow.ActionGraph {
	return dataTaskWorkflowActionGraphWithDeferredAndViolations(records, current, dataquery.TaskPlan{}, violations, limit)
}

func dataTaskWorkflowActionGraphWithDeferred(records []dataTaskWorkflowRecord, current, deferred dataquery.TaskPlan, limit int) dataworkflow.ActionGraph {
	return dataTaskWorkflowActionGraphWithDeferredAndViolations(records, current, deferred, nil, limit)
}

func dataTaskWorkflowActionGraphWithDeferredAndViolations(records []dataTaskWorkflowRecord, current, deferred dataquery.TaskPlan, violations []dataworkflow.WorkflowViolation, limit int) dataworkflow.ActionGraph {
	return dataTaskWorkflowActionGraphWithDeferredQueueAndViolations(records, current, dataworkflow.NewDeferredQueue(deferred), violations, limit)
}

func dataTaskWorkflowActionGraphWithDeferredQueue(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, deferredQueue dataworkflow.DeferredQueueState, limit int) dataworkflow.ActionGraph {
	return dataTaskWorkflowActionGraphWithDeferredQueueAndViolations(records, current, deferredQueue, nil, limit)
}

func dataTaskWorkflowActionGraphWithDeferredQueueAndViolations(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, deferredQueue dataworkflow.DeferredQueueState, violations []dataworkflow.WorkflowViolation, limit int) dataworkflow.ActionGraph {
	return dataworkflow.ReduceActionGraphState(dataTaskWorkflowActionGraphReducerInput(records, current, deferredQueue, violations, limit))
}

func dataTaskWorkflowActionGraphReducerInput(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, deferredQueue dataworkflow.DeferredQueueState, violations []dataworkflow.WorkflowViolation, limit int) dataworkflow.ActionGraphInput {
	var ready []dataquery.DataAction
	if dataTaskPlanHasExecutableBatch(current) {
		ready = current.Actions
	}
	deferred := dataworkflow.DeferredQueuePlan(deferredQueue)
	var deferredActions []dataquery.DataAction
	if dataTaskPlanHasExecutableBatch(deferred) {
		deferredActions = deferred.Actions
	}
	return dataworkflow.ActionGraphInput{
		Events:        dataTaskWorkflowActionEvents(records),
		Ready:         ready,
		Deferred:      deferredActions,
		DeferredPlan:  deferred,
		DeferredQueue: deferredQueue,
		Blocked:       violations,
		EventLimit:    limit,
	}
}

func dataTaskWorkflowActionEvents(records []dataTaskWorkflowRecord) []dataworkflow.ActionEvent {
	return dataworkflow.BuildWorkflowActionEvents(records)
}

func dataTaskWorkflowCurrentBatchContract(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, workflow dataquery.CoverageContract) (dataquery.CoverageContract, dataworkflow.CoverageLayer) {
	if dataTaskPlanHasExecutableBatch(current) {
		return dataTaskExecutionCoverageContract(records, current, workflow), dataworkflow.CoverageLayerCurrentBatch
	}
	if latest, ok := dataTaskLatestWorkflowPlan(records); ok {
		return latest.CoverageContract, dataworkflow.CoverageLayerLastBatch
	}
	return dataquery.CoverageContract{}, dataworkflow.CoverageLayerCurrentBatch
}

func dataTaskExecutionCoverageContract(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, workflow dataquery.CoverageContract) dataquery.CoverageContract {
	currentContract := dataTaskConstrainCurrentRequiredMaterials(dataquery.CoverageContract{}, plan)
	currentContract.DecisionRecordsRequired = workflow.DecisionRecordsRequired
	currentContract.RuleCoverageRequired = workflow.RuleCoverageRequired
	currentContract.ContributionLedgerRequired = workflow.ContributionLedgerRequired
	currentContract.EntityResolutionRequired = workflow.EntityResolutionRequired
	currentContract.ReconcileRequired = workflow.ReconcileRequired
	if len(currentContract.ValidationRules) == 0 {
		currentContract.ValidationRules = append([]string(nil), workflow.ValidationRules...)
	}
	return currentContract
}

func dataTaskPlanHasExecutableBatch(plan dataquery.TaskPlan) bool {
	return len(plan.Actions) > 0 || strings.TrimSpace(plan.Script) != "" || len(cleanDataTaskStrings(plan.InputPaths)) > 0
}

func dataTaskLatestWorkflowPlan(records []dataTaskWorkflowRecord) (dataquery.TaskPlan, bool) {
	for i := len(records) - 1; i >= 0; i-- {
		if dataTaskPlanHasExecutableBatch(records[i].Plan) || len(records[i].Plan.CoverageContract.RequiredMaterials) > 0 || len(records[i].Plan.CoverageContract.OptionalMaterials) > 0 {
			return records[i].Plan, true
		}
	}
	return dataquery.TaskPlan{}, false
}

func dataTaskWorkflowHasMaterialProgress(records []dataTaskWorkflowRecord) bool {
	for _, rec := range records {
		if rec.Result == nil || strings.TrimSpace(rec.Err) != "" {
			continue
		}
		if len(cleanDataTaskStrings(rec.Result.ConsumedPaths)) > 0 {
			return true
		}
		for _, artifact := range rec.Result.Artifacts {
			if strings.TrimSpace(artifact.ID) != "" || len(cleanDataTaskStrings(artifact.SourcePaths)) > 0 {
				return true
			}
			if artifact.Fields != nil && strings.TrimSpace(artifact.Fields["artifact_path"]) != "" {
				return true
			}
		}
	}
	return false
}

func dataTaskFieldContractViolationsFromRecord(round int, rec dataTaskWorkflowRecord, access []dataTaskArtifactAccessPrompt, allowed map[string]bool) []dataTaskFieldContractViolation {
	allowedActions := make([]string, 0, len(allowed))
	for action, ok := range allowed {
		if ok {
			allowedActions = append(allowedActions, action)
		}
	}
	sort.Strings(allowedActions)
	issues := dataworkflow.DiscoverFieldContractIssues(dataworkflow.FieldContractIssueInput{
		Records:            []dataworkflow.WorkflowRecord{rec},
		ArtifactAccess:     access,
		AllowedNextActions: allowedActions,
		Limit:              16,
	})
	for i := range issues {
		issues[i].Round = round
	}
	return issues
}

func dataTaskWorkflowZeroMatchFilterIssues(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView, limit int) []dataTaskZeroMatchFilterIssue {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if (!state.ContributionLedgerRequired && !state.ReconcileRequired) || state.ContributionRecords > 0 {
		return nil
	}
	return dataworkflow.ZeroMatchFilterIssuesFromBatches(dataTaskArtifactDiagnosticBatches(records), limit)
}

func dataTaskWorkflowUnmatchedResolutionIssues(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView, limit int) []dataTaskUnmatchedResolutionIssue {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if !state.EntityResolutionRequired || (!state.ContributionLedgerRequired && !state.ReconcileRequired) || state.ContributionRecords > 0 {
		return nil
	}
	return dataworkflow.UnmatchedResolutionIssuesFromBatches(dataTaskArtifactDiagnosticBatches(records), limit)
}

func dataTaskWorkflowZeroEligibleIssues(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView, limit int) []dataTaskZeroEligibleIssue {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if (!state.ContributionLedgerRequired && !state.ReconcileRequired) || state.ContributionRecords > 0 {
		return nil
	}
	return dataworkflow.ZeroEligibleIssuesFromBatches(dataTaskArtifactDiagnosticBatches(records), limit)
}

func dataTaskArtifactDiagnosticBatches(records []dataTaskWorkflowRecord) []dataworkflow.ArtifactDiagnosticBatch {
	var out []dataworkflow.ArtifactDiagnosticBatch
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if rec.Result == nil || len(rec.Result.Artifacts) == 0 {
			continue
		}
		out = append(out, dataworkflow.ArtifactDiagnosticBatch{
			Round:     i + 1,
			Artifacts: rec.Result.Artifacts,
		})
	}
	return out
}

func dataTaskWorkflowEntityStageMaterialized(records []dataTaskWorkflowRecord) bool {
	for _, rec := range records {
		if rec.Result == nil || strings.TrimSpace(rec.Err) != "" {
			continue
		}
		for _, artifact := range rec.Result.Artifacts {
			if dataTaskArtifactMaterializesEntityStage(artifact) {
				return true
			}
		}
	}
	return false
}

func dataTaskArtifactMaterializesEntityStage(artifact dataquery.DataArtifact) bool {
	kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
	if strings.Contains(kind, string(dataquery.DataActionNormalizeEntities)) ||
		strings.Contains(kind, string(dataquery.DataActionEnrichRecords)) ||
		strings.Contains(kind, string(dataquery.DataActionJoinRecords)) {
		return true
	}
	for _, child := range artifact.Children {
		if dataTaskArtifactMaterializesEntityStage(child) {
			return true
		}
	}
	return false
}

func dataTaskArtifactIsDiagnosticChild(artifact dataTaskArtifactAccessPrompt) bool {
	return strings.TrimSpace(artifact.NodeClass) == dataworkflow.ArtifactNodeClassDiagnosticChild ||
		dataworkflow.IsDiagnosticChildArtifact(artifact.ID, artifact.Kind)
}

func dataTaskArtifactLooksLikeApplyResolutionLedger(artifact dataTaskArtifactAccessPrompt) bool {
	if dataTaskArtifactLooksLikeEntityResolutionLedger(artifact) {
		return true
	}
	return dataTaskArtifactLooksLikeGeneratedEntityMapping(artifact) &&
		firstDataTaskExistingField(artifact.Fields, "item_id", "source_locator", "_source_locator", "source_line", "source_index") != ""
}

func dataTaskArtifactLooksLikeGeneratedEntityMapping(artifact dataTaskArtifactAccessPrompt) bool {
	kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
	if strings.Contains(kind, "normalize_entities") || strings.Contains(kind, "entity_resolution") {
		return true
	}
	fields := dataTaskFieldSet(artifact.Fields)
	hasSource := fields["source_value"] != "" || fields["source_field"] != ""
	hasCanonical := fields["canonical_id"] != "" || fields["canonical_label"] != "" || fields["canonical_value"] != ""
	return hasSource && hasCanonical
}

func dataTaskArtifactLooksLikeMapping(artifact dataTaskArtifactAccessPrompt) bool {
	kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
	if strings.Contains(kind, "normalize_entities") || strings.Contains(kind, "entity_resolution") {
		return true
	}
	fields := dataTaskFieldSet(artifact.Fields)
	if fields["source_value"] != "" && (fields["canonical_id"] != "" || fields["canonical_label"] != "") {
		return true
	}
	if dataTaskReferenceValueField(artifact.Fields) != "" && len(dataTaskCandidateReferenceSourceFields(artifact.Fields, dataTaskReferenceValueField(artifact.Fields), 3)) > 0 {
		return true
	}
	return false
}

func dataTaskArtifactLooksLikeEntityResolutionLedger(artifact dataTaskArtifactAccessPrompt) bool {
	fields := dataTaskFieldSet(artifact.Fields)
	hasLocator := fields["item_id"] != "" || fields["source_locator"] != "" || fields["_source_locator"] != "" || fields["record_id"] != "" || fields["row_id"] != ""
	if !hasLocator {
		return false
	}
	hasSource := fields["source_value"] != "" || fields["source_field"] != "" || fields["evidence_refs"] != ""
	hasCanonical := fields["canonical_id"] != "" || fields["canonical_label"] != "" || fields["canonical_value"] != ""
	return hasSource && hasCanonical
}

func dataTaskReferenceValueField(fields []string) string {
	if field := firstDataTaskExistingField(fields, "canonical_id", "canonical_label", "value", "target_value", "code"); field != "" {
		return field
	}
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || strings.HasPrefix(lower, "_") {
			continue
		}
		if strings.Contains(lower, "canonical") && (strings.Contains(lower, "id") || strings.Contains(lower, "code") || strings.Contains(lower, "value")) {
			return strings.TrimSpace(field)
		}
	}
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || strings.HasPrefix(lower, "_") {
			continue
		}
		if strings.HasSuffix(lower, "_id") || strings.HasSuffix(lower, "_code") {
			return strings.TrimSpace(field)
		}
	}
	return ""
}

func dataTaskCandidateSourceFields(fields []string, limit int) []string {
	var out []string
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || strings.HasPrefix(lower, "_") || lower == "amount" || lower == "year" {
			continue
		}
		if strings.Contains(lower, "name") || strings.Contains(lower, "raw") || strings.Contains(lower, "description") || strings.Contains(lower, "purpose") || strings.Contains(lower, "label") || strings.Contains(lower, "text") || strings.Contains(lower, "title") {
			out = append(out, trimmed)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func dataTaskCandidateReferenceSourceFields(fields []string, valueField string, limit int) []string {
	var out []string
	valueLower := strings.ToLower(strings.TrimSpace(valueField))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || strings.HasPrefix(lower, "_") || lower == valueLower {
			continue
		}
		if strings.Contains(lower, "negative") || strings.Contains(lower, "exclude") || strings.Contains(lower, "except") {
			continue
		}
		if strings.Contains(lower, "source") || strings.Contains(lower, "term") || strings.Contains(lower, "name") || strings.Contains(lower, "label") || strings.Contains(lower, "keyword") || strings.Contains(lower, "example") || strings.Contains(lower, "description") || strings.Contains(lower, "text") {
			out = append(out, trimmed)
			if len(out) >= limit {
				return out
			}
		}
	}
	if len(out) == 0 {
		for _, field := range fields {
			trimmed := strings.TrimSpace(field)
			lower := strings.ToLower(trimmed)
			if trimmed == "" || strings.HasPrefix(lower, "_") || lower == valueLower {
				continue
			}
			if strings.Contains(lower, "negative") || strings.Contains(lower, "exclude") || strings.Contains(lower, "except") {
				continue
			}
			out = append(out, trimmed)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func firstDataTaskExistingField(fields []string, candidates ...string) string {
	set := dataTaskFieldSet(fields)
	for _, candidate := range candidates {
		if field := set[strings.ToLower(strings.TrimSpace(candidate))]; field != "" {
			return field
		}
	}
	return ""
}

func mustCompactJSONString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func dataTaskWorkflowArtifactAccess(records []dataTaskWorkflowRecord, limit int) []dataTaskArtifactAccessPrompt {
	artifacts := dataTaskWorkflowArtifactsNewestFirst(records)
	return dataworkflow.BuildArtifactAccessViews(artifacts, limit)
}

func dataTaskWorkflowArtifactContractAccess(records []dataTaskWorkflowRecord) []dataTaskArtifactAccessPrompt {
	projections := dataTaskWorkflowArtifactSchemaProjections(records)
	return dataworkflow.ArtifactAccessViewsFromProjections(projections, len(projections))
}

func dataTaskWorkflowArtifactSchemaProjections(records []dataTaskWorkflowRecord) []dataworkflow.ArtifactSchemaProjection {
	artifacts := dataTaskWorkflowArtifactsNewestFirst(records)
	if len(artifacts) == 0 {
		return nil
	}
	return dataworkflow.ProjectArtifactSchemasNewestFirst(artifacts)
}

func dataTaskWorkflowArtifactAvailability(records []dataTaskWorkflowRecord, limit int) ([]dataTaskArtifactAccessPrompt, int, bool) {
	artifacts := dataTaskWorkflowArtifactsNewestFirst(records)
	total := dataTaskFlattenedArtifactCount(artifacts)
	access := sampleDataTaskArtifactAccess(artifacts, limit)
	for i := range access {
		access[i].FieldSamples = nil
	}
	return access, total, total > len(access)
}

func dataTaskWorkflowArtifactsNewestFirst(records []dataTaskWorkflowRecord) []dataquery.DataArtifact {
	return dataworkflow.ArtifactsNewestFirst(records)
}

func dataTaskFlattenedArtifactCount(artifacts []dataquery.DataArtifact) int {
	return dataworkflow.FlattenedArtifactCount(artifacts)
}

func firstDataTaskArtifactAlias(artifact dataTaskArtifactAccessPrompt) string {
	for _, alias := range artifact.Aliases {
		alias = strings.TrimSpace(alias)
		if alias != "" {
			return alias
		}
	}
	return strings.TrimSpace(artifact.ID)
}

func dataTaskFieldSet(fields []string) map[string]string {
	out := map[string]string{}
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		key := strings.ToLower(trimmed)
		if key != "" && out[key] == "" {
			out[key] = trimmed
		}
	}
	return out
}

func dataTaskCustomTransformCooldown(records []dataTaskWorkflowRecord) bool {
	total, _, topCodeCount := dataTaskCustomTransformFailureClassStats(records)
	if total >= DefaultDataTaskMaxCustomTransformClassFailures || topCodeCount >= DefaultDataTaskMaxNodeFailures {
		return true
	}
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if strings.TrimSpace(rec.Err) != "" && dataTaskRecordHasCustomTransformScript(rec) {
			return true
		}
		if strings.TrimSpace(rec.Err) == "" && dataTaskRecordHasPostScriptTypedProgress(rec) {
			return false
		}
	}
	return false
}

func dataTaskCustomTransformCooldownForState(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView) bool {
	if dataTaskCustomTransformCooldown(records) {
		return true
	}
	if dataTaskWorkflowShouldPreferTypedActions(state) {
		return true
	}
	total, _, _ := dataTaskCustomTransformFailureClassStats(records)
	if total == 0 {
		return false
	}
	// Once a broad custom transform has failed in a ledgered workflow, keep
	// the remaining validation DAG on typed actions until only final answer
	// projection remains. A later successful typed node is progress, but it
	// should not reopen the same broad-script escape hatch while contribution
	// or reconcile ledgers are still structurally missing.
	for _, missing := range state.MissingValidationStages() {
		if missing != "final_answer" {
			return true
		}
	}
	return false
}

func dataTaskWorkflowShouldPreferTypedActions(state dataTaskWorkflowStateView) bool {
	if state.NextStage == "" || state.NextStage == "complete" || state.NextStage == "emit_output_contract_answer" {
		return false
	}
	if !(state.DecisionRecordsRequired || state.EntityResolutionRequired || state.ContributionLedgerRequired || state.ReconcileRequired) {
		return false
	}
	for _, missing := range state.MissingValidationStages() {
		if missing != "final_answer" {
			return true
		}
	}
	return false
}

func dataTaskRecordHasPostScriptTypedProgress(rec dataTaskWorkflowRecord) bool {
	if rec.Result == nil || dataTaskRecordHasCustomTransformScript(rec) {
		return false
	}
	if len(rec.Result.Contributions) > 0 || len(rec.Result.EntityResolutions) > 0 || rec.Result.Reconcile != nil {
		return true
	}
	for _, action := range rec.Plan.Actions {
		switch normalizeDataActionKindForWorkflow(action.Kind) {
		case dataquery.DataActionDeriveFields,
			dataquery.DataActionExtractFields,
			dataquery.DataActionGroupRecords,
			dataquery.DataActionExpandRecords,
			dataquery.DataActionFilterRecords,
			dataquery.DataActionQualifyRecords,
			dataquery.DataActionMappingCandidate,
			dataquery.DataActionNormalizeEntities,
			dataquery.DataActionApplyResolutions,
			dataquery.DataActionEnrichRecords,
			dataquery.DataActionJoinRecords,
			dataquery.DataActionComputeContribs,
			dataquery.DataActionReconcile,
			dataquery.DataActionAssembleAnswer:
			return true
		}
	}
	return false
}

func dataTaskWorkflowOutputContract(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataquery.OutputContract {
	var values []dataquery.OutputContract
	for _, rec := range records {
		values = append(values, rec.Plan.OutputContract)
		if rec.Result != nil {
			values = append(values, rec.Result.OutputContract)
		}
	}
	values = append(values, current.OutputContract)
	return firstNonEmptyOutputContract(values...)
}

func dataTaskPlanIsCoverageOnly(plan dataquery.TaskPlan) bool {
	if strings.TrimSpace(plan.Script) != "" || len(plan.Actions) == 0 {
		return false
	}
	for _, action := range plan.Actions {
		if strings.TrimSpace(action.Script) != "" {
			return false
		}
		switch normalizeDataActionKindForWorkflow(action.Kind) {
		case dataquery.DataActionExtractRecords:
			if strings.TrimSpace(action.OutputArtifact) != "" {
				return false
			}
			continue
		case dataquery.DataActionMaterialInventory, dataquery.DataActionInspectMaterial, dataquery.DataActionDeriveRules:
			continue
		default:
			return false
		}
	}
	return true
}

func compactDataTaskResultPromptView(result dataquery.Result, answerLimit, auditLimit, decisionLimit, ruleLimit, contributionLimit int) *dataTaskResultPromptView {
	return dataworkflow.BuildCompactResultPromptView(result, answerLimit, auditLimit, decisionLimit, ruleLimit, contributionLimit)
}

func compactDataTaskResultPromptViewWithArtifactLimits(result dataquery.Result, answerLimit, auditLimit, decisionLimit, ruleLimit, contributionLimit, artifactLimit, artifactAccessLimit, materialSetLimit int) *dataTaskResultPromptView {
	return dataworkflow.BuildResultPromptView(result, dataworkflow.ResultPromptViewBudget{
		AnswerLimit:       answerLimit,
		AuditLimit:        auditLimit,
		DecisionLimit:     decisionLimit,
		RuleLimit:         ruleLimit,
		ContributionLimit: contributionLimit,
		ArtifactLimit:     artifactLimit,
		ArtifactAccess:    artifactAccessLimit,
		MaterialSetLimit:  materialSetLimit,
	})
}

func sampleDataTaskMaterialSetHandles(artifacts []dataquery.DataArtifact, limit int) []dataTaskMaterialSetHandlePrompt {
	return dataworkflow.BuildMaterialCollectionViews(artifacts, limit)
}

func sampleDataTaskArtifactAccess(artifacts []dataquery.DataArtifact, limit int) []dataTaskArtifactAccessPrompt {
	return dataworkflow.BuildArtifactAccessViews(artifacts, limit)
}

func sampleDataTaskArtifactAccessWithFieldSamples(artifacts []dataquery.DataArtifact, limit, maxSampleFields, maxSampleValues int) []dataTaskArtifactAccessPrompt {
	return dataworkflow.BuildArtifactAccessViewsWithFieldSamples(artifacts, limit, maxSampleFields, maxSampleValues)
}

func compactDataTaskActionsForPrompt(actions []dataquery.DataAction, limit, scriptLimit int) []dataquery.DataAction {
	return compactDataTaskActionsForPromptWithParamLimit(actions, limit, scriptLimit, scriptLimit)
}

func compactDataTaskActionsForPromptWithParamLimit(actions []dataquery.DataAction, limit, scriptLimit, paramLimit int) []dataquery.DataAction {
	if limit <= 0 || len(actions) == 0 {
		return nil
	}
	if len(actions) > limit {
		actions = actions[:limit]
	}
	out := make([]dataquery.DataAction, 0, len(actions))
	for _, action := range actions {
		action.Purpose = clampDataTaskWorkflowText(action.Purpose, 240)
		action.InputPaths = clampDataTaskStringSlice(action.InputPaths, 12)
		action.Script = clampDataTaskWorkflowText(action.Script, scriptLimit)
		action.Params = compactDataTaskActionParamsForPrompt(action.Params, 16, paramLimit)
		action.SuccessCriteria = clampDataTaskStringSlice(action.SuccessCriteria, 8)
		out = append(out, action)
	}
	return out
}

func compactDataTaskActionParamsForPrompt(params map[string]string, maxKeys, valueLimit int) map[string]string {
	if len(params) == 0 || maxKeys <= 0 {
		return nil
	}
	if valueLimit <= 0 {
		valueLimit = 240
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(keys))
	for i, key := range keys {
		if i >= maxKeys {
			out["_truncated_params"] = fmt.Sprintf("%d additional param(s) omitted from prompt preview", len(keys)-maxKeys)
			break
		}
		out[key] = clampDataTaskWorkflowText(params[key], valueLimit)
	}
	return out
}

func sampleDataTaskArtifacts(artifacts []dataquery.DataArtifact, limit int) []dataquery.DataArtifact {
	return dataworkflow.SampleArtifactsForPrompt(artifacts, limit)
}

func inferDataTaskAnswerItemCount(answer string, contract dataquery.OutputContract) int {
	return dataworkflow.InferAnswerItemCount(answer, contract)
}

func promptReconcileGroupSummary(report *dataquery.ReconcileReport, limit int) (int, []string, bool) {
	return dataworkflow.ReconcileGroupSummary(report, limit)
}

func dataTaskLedgerProjections(result dataquery.Result, sampleLimit int) []dataTaskLedgerProjection {
	return dataworkflow.BuildLedgerProjections(result, sampleLimit)
}

func sampleDataTaskRuleCoverage(records []dataquery.RuleCoverageRecord, limit int) []dataquery.RuleCoverageRecord {
	return dataworkflow.SampleRuleCoverage(records, limit)
}

func sampleDataTaskContributions(records []dataquery.ContributionRecord, limit int) []dataquery.ContributionRecord {
	return dataworkflow.SampleContributions(records, limit)
}

func sampleDataTaskEntityResolutions(records []dataquery.EntityResolutionRecord, limit int) []dataquery.EntityResolutionRecord {
	return dataworkflow.SampleEntityResolutions(records, limit)
}

func clampPromptReconcileReport(report *dataquery.ReconcileReport) *dataquery.ReconcileReport {
	return dataworkflow.ClampPromptReconcileReport(report)
}

func sampleDataTaskRowDecisions(rows []dataquery.RowDecision, limit int) []dataquery.RowDecision {
	return dataworkflow.SampleRowDecisions(rows, limit)
}

func clampDataTaskWorkflowText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "\n...[truncated]"
}

func clampDataTaskStringSlice(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return append([]string(nil), values...)
	}
	out := append([]string(nil), values[:limit]...)
	out = append(out, fmt.Sprintf("...%d more", len(values)-limit))
	return out
}

func latestDataTaskResult(records []dataTaskWorkflowRecord) (dataquery.Result, bool) {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Result != nil {
			return *records[i].Result, true
		}
	}
	return dataquery.Result{}, false
}

func latestDataTaskEvaluation(records []dataTaskWorkflowRecord) (dataquery.Evaluation, bool) {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Evaluation != nil {
			return *records[i].Evaluation, true
		}
	}
	return dataquery.Evaluation{}, false
}

// latestDataTaskAnswerContestingEvaluation reports whether the answer
// face is under an active typed contest, returning the record index
// and evaluation of the governing contest. The contest is STICKY
// (second-review P1): a repair round is often an answerless
// materialization helper batch (exactly the multi-batch rhythm the
// rung-3 advisory steers into), and that helper round's routine
// continue evaluation must not launder the contest. Scanning
// newest-first, only evaluations whose typed anchor targets the answer
// face (normalized action_kind == assemble_answer) govern the contest:
//
//   - an actionable repair_node anchored at assemble_answer OPENS (or
//     renews) the contest at its record index;
//   - an answer-face-anchored evaluation that is NOT a repair_node
//     CLEARS it (explicit re-evaluation of the answer face);
//   - an answer-face-anchored repair_node without an actionable target
//     (noisy/low-confidence) neither opens nor clears — noise decides
//     nothing in either direction;
//   - every other evaluation (helper-round continue_data/expand_graph,
//     repair anchors on other nodes, ...) is skipped: it neither opens
//     nor clears the contest.
//
// The second clearing event — an answer-bearing record NEWER than the
// contest (the repair output itself) — is index-based and owned by the
// consumers: selectDataTaskTerminalAnswer compares the candidate index
// against the contest index. This is the single typed authority for
// "the answer face is contested" — the runner's answer-repair lane
// activation (dataTaskAssembleAnswerRepairContext), the completion
// check, and the terminal publish consult all consume the same sticky
// predicate (DL-A(a)/DL-C, ledger §7.12); no surface re-derives it.
func latestDataTaskAnswerContestingEvaluation(records []dataTaskWorkflowRecord) (int, dataquery.Evaluation, bool) {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Evaluation == nil {
			continue
		}
		eval := *records[i].Evaluation
		if dataworkflow.NormalizeActionKind(dataquery.DataActionKind(eval.ActionKind)) != dataquery.DataActionAssembleAnswer {
			// Not anchored at the answer face: sticky — keep scanning.
			continue
		}
		if dataworkflow.EvaluationHasActionableRepairTarget(eval) {
			return i, eval, true
		}
		if eval.Status == dataquery.EvalRepairNode {
			// Answer-face repair status without an actionable target:
			// noise, skip.
			continue
		}
		// Explicit answer-face re-evaluation that is not a repair
		// request: the contest is cleared.
		return -1, dataquery.Evaluation{}, false
	}
	return -1, dataquery.Evaluation{}, false
}

// dataTaskAssembleAnswerRepairContext derives the typed answer-repair
// anchor for the action runner (DL-A(a), ledger §7.12). Active exactly
// while the answer face is under an active STICKY contest
// (latestDataTaskAnswerContestingEvaluation — the same predicate the
// completion check and terminal publish consult consume, single
// authority): an intervening helper round's routine continue
// evaluation neither deactivates the lane (a two-batch repair's later
// assemble round must not run bare) nor launders the contest. Every
// input is a typed evaluator field, no prose is inspected. While
// active, assemble_answer walks the projection priority ladder
// (declared mapping first, then answer-bearing payload fallback)
// instead of being structurally bound to the contested reconcile-group
// lineage.
func dataTaskAssembleAnswerRepairContext(records []dataTaskWorkflowRecord) dataquery.AnswerRepairContext {
	_, eval, ok := latestDataTaskAnswerContestingEvaluation(records)
	if !ok {
		return dataquery.AnswerRepairContext{}
	}
	return dataquery.AnswerRepairContext{
		Active:   true,
		ActionID: strings.TrimSpace(eval.ActionID),
	}
}

// dataTaskTerminalAnswerSelection is the answer the workflow currently
// stands on, as one typed value shared by every consuming face.
type dataTaskTerminalAnswerSelection struct {
	Plan   dataquery.TaskPlan
	Result dataquery.Result
	// FromFallback marks that the literal latest result was not a
	// final-answer candidate and an earlier answer-bearing record was
	// selected instead.
	FromFallback bool
	// Contested marks that the latest evaluation actively contests the
	// answer face and does not pre-date the selected answer-bearing
	// record: the selected answer is exactly the one under contest, so
	// terminal publication must fail loud instead of publishing it
	// silently, and completion must not be satisfied by it (DL-C).
	Contested bool
}

// selectDataTaskTerminalAnswer is the single typed authority for which
// record's answer this workflow stands on — the same predicate that
// publishes has_answer=true on the workflow state face
// (ResultIsFinalAnswerCandidate with the producing record's plan). The
// evaluation-path completion check and the terminal answer projection
// both consume it; neither derives a private reading of the records
// (DL-C, ledger §7.12; specimen data_basic_sum_with_rules-20260705-050649:
// artifact answer "17" satisfied every ledger, but the terminal
// projection only read the literal latest batch result — an answerless
// inspect_material round — and failed the run with "result.answer is
// empty"). An answer-bearing record produced AFTER the contesting
// evaluation is a repair output and publishes normally; only an answer
// at or before the contest index is Contested.
func selectDataTaskTerminalAnswer(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, contract dataquery.CoverageContract, output dataquery.OutputContract) dataTaskTerminalAnswerSelection {
	if dataworkflow.ResultIsFinalAnswerCandidate(current, result, contract, output) {
		return dataTaskTerminalAnswerSelection{Plan: current, Result: result}
	}
	idx, ok := latestDataTaskFinalAnswerCandidateIndex(records, contract, output)
	if !ok {
		return dataTaskTerminalAnswerSelection{Plan: current, Result: result}
	}
	sel := dataTaskTerminalAnswerSelection{
		Plan:         records[idx].Plan,
		Result:       dataquery.NormalizeResult(*records[idx].Result),
		FromFallback: true,
	}
	if evalIdx, _, contesting := latestDataTaskAnswerContestingEvaluation(records); contesting && evalIdx >= idx {
		sel.Contested = true
	}
	return sel
}

func latestDataTaskFinalAnswerCandidateIndex(records []dataTaskWorkflowRecord, contract dataquery.CoverageContract, output dataquery.OutputContract) (int, bool) {
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if rec.Result == nil {
			continue
		}
		if dataworkflow.ResultIsFinalAnswerCandidate(rec.Plan, *rec.Result, contract, output) {
			return i, true
		}
	}
	return -1, false
}

func dataTaskActionRunnerSeed(records []dataTaskWorkflowRecord) dataquery.Result {
	var out dataquery.Result
	seenArtifacts := map[string]int{}
	seenConsumed := map[string]bool{}
	// Same-key later-seen wins (as-built ruling, ledger §7.12): a
	// repair/continuation round that re-produces an artifact key
	// supersedes the stale copy, and the surviving entry moves to the
	// tail so seed slice order tracks production freshness (answer-repair
	// payload source selection consumes that ordering). The prior
	// first-seen-wins dedup let the oldest copy of the constant-ID
	// "emitted_payload" envelope permanently suppress every repair
	// batch's fresh payload.
	addArtifact := func(artifact dataquery.DataArtifact) {
		key := strings.TrimSpace(artifact.ID)
		if key == "" && len(artifact.SourcePaths) > 0 {
			key = strings.Join(cleanDataTaskStrings(artifact.SourcePaths), "\x00")
		}
		if key == "" {
			out.Artifacts = append(out.Artifacts, artifact)
			return
		}
		if idx, ok := seenArtifacts[key]; ok {
			out.Artifacts = append(out.Artifacts[:idx], out.Artifacts[idx+1:]...)
			for k, v := range seenArtifacts {
				if v > idx {
					seenArtifacts[k] = v - 1
				}
			}
		}
		seenArtifacts[key] = len(out.Artifacts)
		out.Artifacts = append(out.Artifacts, artifact)
	}
	addConsumed := func(values []string) {
		for _, p := range cleanDataTaskStrings(values) {
			if seenConsumed[p] {
				continue
			}
			seenConsumed[p] = true
			out.ConsumedPaths = append(out.ConsumedPaths, p)
		}
	}
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		result := *rec.Result
		addConsumed(result.ConsumedPaths)
		for _, artifact := range result.Artifacts {
			addArtifact(artifact)
		}
		out.Rows = dataquery.DedupeRowDecisionRecords(append(out.Rows, result.Rows...))
		out.RuleCoverage = append(out.RuleCoverage, result.RuleCoverage...)
		out.Contributions = append(out.Contributions, result.Contributions...)
		out.EntityResolutions = append(out.EntityResolutions, result.EntityResolutions...)
		out.Metrics = append(out.Metrics, result.Metrics...)
		out.ContractWarnings = append(out.ContractWarnings, result.ContractWarnings...)
		if strings.TrimSpace(result.AuditSummary) != "" {
			if strings.TrimSpace(out.AuditSummary) == "" {
				out.AuditSummary = result.AuditSummary
			} else {
				out.AuditSummary += "; " + result.AuditSummary
			}
		}
		if result.Reconcile != nil {
			reconcile := *result.Reconcile
			out.Reconcile = &reconcile
		}
		if strings.TrimSpace(result.Answer) != "" && (dataworkflow.ResultAnswerPresent(result) || strings.TrimSpace(out.Answer) == "" || !dataworkflow.ResultAnswerPresent(out)) {
			out.Answer = result.Answer
		}
		if result.OutputContract.Format != "" {
			out.OutputContract = result.OutputContract
		}
	}
	return dataquery.NormalizeResult(out)
}

// dataTaskSeedArtifactRounds maps each seeded artifact ID to the
// 1-based record round that produced its surviving (newest) copy — the
// typed freshness input for answer-repair payload source selection
// (dataquery.ActionRunner.SeedArtifactRounds; as-built ruling, ledger
// §7.12). Later records overwrite earlier ones, mirroring the same-key
// later-seen-wins dedup in dataTaskActionRunnerSeed.
func dataTaskSeedArtifactRounds(records []dataTaskWorkflowRecord) map[string]int {
	rounds := map[string]int{}
	for i, rec := range records {
		if rec.Result == nil {
			continue
		}
		for _, artifact := range rec.Result.Artifacts {
			id := strings.TrimSpace(artifact.ID)
			if id == "" {
				continue
			}
			rounds[id] = i + 1
		}
	}
	return rounds
}

func dataTaskResultHasHandoffSignal(result dataquery.Result) bool {
	return strings.TrimSpace(result.Answer) != "" ||
		strings.TrimSpace(result.AuditSummary) != "" ||
		len(result.Artifacts) > 0 ||
		len(result.Rows) > 0 ||
		len(result.RuleCoverage) > 0 ||
		len(result.Contributions) > 0 ||
		len(result.EntityResolutions) > 0 ||
		result.Reconcile != nil ||
		len(result.ConsumedPaths) > 0 ||
		len(result.Metrics) > 0 ||
		len(result.ContractWarnings) > 0
}

func dataTaskWorkflowRecordWithOptionalResult(plan dataquery.TaskPlan, result dataquery.Result, errText string) dataTaskWorkflowRecord {
	rec := dataTaskWorkflowRecord{Plan: plan, Err: errText}
	if dataTaskResultHasHandoffSignal(result) {
		rec.Result = &result
	}
	return rec
}

func dataTaskWorkflowRecordWithExecutionViolation(plan dataquery.TaskPlan, result dataquery.Result, errText string, violation dataquery.DataTaskViolation) dataTaskWorkflowRecord {
	rec := dataTaskWorkflowRecordWithOptionalResult(plan, result, errText)
	if strings.TrimSpace(violation.Code) != "" {
		rec.Violations = []dataquery.DataTaskViolation{violation}
	}
	return rec
}

func dataTaskRepairViolationFromRecords(records []dataTaskWorkflowRecord, errText string) dataquery.DataTaskViolation {
	for i := len(records) - 1; i >= 0; i-- {
		for _, violation := range records[i].Violations {
			if strings.TrimSpace(violation.Code) != "" {
				return violation
			}
		}
	}
	return dataquery.ClassifyExecutionError(errText)
}

func dataTaskArtifactAliasPaths(artifact dataquery.DataArtifact) []string {
	var out []string
	if strings.TrimSpace(artifact.ID) != "" {
		out = append(out, artifact.ID)
		if !strings.HasSuffix(strings.TrimSpace(artifact.ID), ".json") {
			out = append(out, artifact.ID+".json")
		}
	}
	if artifact.Fields != nil {
		for _, alias := range strings.Split(artifact.Fields["artifact_aliases"], ",") {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				out = append(out, alias)
			}
		}
	}
	return cleanDataTaskStrings(out)
}

func dataTaskWorkflowGeneratedArtifactPathSet(records []dataTaskWorkflowRecord) map[string]bool {
	out := map[string]bool{}
	var mark func([]string)
	mark = func(values []string) {
		for _, value := range cleanDataTaskStrings(values) {
			normalized := normalizeDataTaskCoveragePath(value)
			if normalized != "" {
				out[normalized] = true
			}
		}
	}
	var walk func(dataquery.DataArtifact)
	walk = func(artifact dataquery.DataArtifact) {
		mark(dataTaskArtifactAliasPaths(artifact))
		if artifact.Fields != nil {
			mark([]string{artifact.Fields["artifact_path"]})
		}
		for _, child := range artifact.Children {
			walk(child)
		}
	}
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		for _, artifact := range rec.Result.Artifacts {
			walk(artifact)
		}
	}
	return out
}

func dataTaskWorkflowHasArtifactAlias(records []dataTaskWorkflowRecord, alias string) bool {
	want := normalizeDataTaskCoveragePath(alias)
	if want == "" {
		return false
	}
	return dataTaskWorkflowGeneratedArtifactPathSet(records)[want]
}

func dataTaskCoverageContractWithoutGeneratedScriptMaterials(contract dataquery.CoverageContract, generated map[string]bool) dataquery.CoverageContract {
	if len(generated) == 0 {
		return contract
	}
	filter := func(materials []dataquery.CoverageMaterial) []dataquery.CoverageMaterial {
		out := make([]dataquery.CoverageMaterial, 0, len(materials))
		for _, material := range materials {
			mode := normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode)
			if mode == dataquery.MaterialUseScriptConsumed && dataTaskCoverageMaterialIsGeneratedArtifact(material, generated) {
				continue
			}
			out = append(out, material)
		}
		return out
	}
	contract.RequiredMaterials = filter(contract.RequiredMaterials)
	contract.OptionalMaterials = filter(contract.OptionalMaterials)
	return contract
}

func dataTaskCoverageMaterialIsGeneratedArtifact(material dataquery.CoverageMaterial, generated map[string]bool) bool {
	for _, raw := range []string{material.Path, material.ID} {
		normalized := normalizeDataTaskCoveragePath(raw)
		if normalized != "" && generated[normalized] {
			return true
		}
	}
	return false
}

func dataTaskCandidatesWithWorkflowArtifacts(base []dataquery.CandidateFile, records []dataTaskWorkflowRecord) []dataquery.CandidateFile {
	out := append([]dataquery.CandidateFile(nil), base...)
	seen := map[string]bool{}
	for _, candidate := range out {
		if strings.TrimSpace(candidate.Path) != "" {
			seen[strings.TrimSpace(candidate.Path)] = true
		}
	}
	var addArtifact func(dataquery.DataArtifact)
	addArtifact = func(artifact dataquery.DataArtifact) {
		aliases := dataTaskArtifactAliasPaths(artifact)
		if len(aliases) == 0 {
			for _, child := range artifact.Children {
				addArtifact(child)
			}
			return
		}
		alias := aliases[0]
		if seen[alias] {
			return
		}
		seen[alias] = true
		out = append(out, dataquery.CandidateFile{
			Path:    alias,
			Kind:    "generated_json",
			Headers: artifact.Headers,
			Sample:  dataTaskArtifactCandidateSample(artifact),
		})
		for _, child := range artifact.Children {
			addArtifact(child)
		}
	}
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		for _, artifact := range rec.Result.Artifacts {
			addArtifact(artifact)
		}
	}
	return out
}

func dataTaskArtifactCandidateSample(artifact dataquery.DataArtifact) []string {
	var sample []string
	if strings.TrimSpace(artifact.Summary) != "" {
		sample = append(sample, "summary: "+strings.TrimSpace(artifact.Summary))
	}
	if artifact.Fields != nil {
		if aliases := strings.TrimSpace(artifact.Fields["artifact_aliases"]); aliases != "" {
			sample = append(sample, "aliases: "+aliases)
		}
		if path := strings.TrimSpace(artifact.Fields["artifact_path"]); path != "" {
			sample = append(sample, "materialized: "+path)
		}
		if shape := strings.TrimSpace(artifact.Fields["json_shape"]); shape != "" {
			sample = append(sample, "json_shape: "+shape)
		}
	}
	sample = append(sample, artifact.Sample...)
	return clampDataTaskStringSlice(sample, 4)
}

func dataTaskWorkflowCoverageContract(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataquery.CoverageContract {
	contract := dataquery.CoverageContract{}
	for _, rec := range records {
		contract = mergeDataTaskCoverageContracts(contract, dataTaskWorkflowRecordCoverageContract(rec))
	}
	currentContract := current.CoverageContract
	currentContract = dataTaskConstrainWorkflowRequiredMaterials(contract, current)
	contract = mergeDataTaskCoverageContracts(contract, currentContract)
	return dataTaskCoverageContractWithoutGeneratedScriptMaterials(contract, dataTaskWorkflowGeneratedArtifactPathSet(records))
}

func dataTaskWorkflowRecordCoverageContract(rec dataTaskWorkflowRecord) dataquery.CoverageContract {
	contract := rec.Plan.CoverageContract
	if strings.TrimSpace(rec.Err) == "" {
		return contract
	}
	out := contract
	out.RequiredMaterials = nil
	for _, material := range contract.RequiredMaterials {
		if dataTaskCoverageMaterialIsUserExplicitFloor(material) {
			out.RequiredMaterials = append(out.RequiredMaterials, material)
			continue
		}
		if strings.TrimSpace(string(material.UsageMode)) == "" {
			material.UsageMode = dataquery.MaterialUseReferenceOnly
		}
		material.Required = false
		out.OptionalMaterials = append(out.OptionalMaterials, material)
	}
	return out
}

func dataTaskConstrainCurrentRequiredMaterials(previous dataquery.CoverageContract, current dataquery.TaskPlan) dataquery.CoverageContract {
	if len(current.CoverageContract.RequiredMaterials) == 0 {
		return current.CoverageContract
	}
	previousRequired := dataTaskCoverageMaterialKeySet(previous.RequiredMaterials)
	currentInputs := dataTaskCurrentBatchInputPathSet(current)
	if len(currentInputs) == 0 {
		return current.CoverageContract
	}
	out := current.CoverageContract
	out.RequiredMaterials = nil
	for _, material := range current.CoverageContract.RequiredMaterials {
		if dataTaskCoverageMaterialShouldRemainRequiredInCurrentBatch(material, previousRequired, currentInputs) {
			out.RequiredMaterials = append(out.RequiredMaterials, material)
			continue
		}
		if strings.TrimSpace(string(material.UsageMode)) == "" {
			material.UsageMode = dataquery.MaterialUseReferenceOnly
		}
		// Keep workflow-level requirements only for deterministic user-pinned
		// material floors. Model-authored future materials outside this batch
		// remain optional so they do not create spurious hard coverage gates.
		material.Required = dataTaskCoverageMaterialIsUserExplicitFloor(material)
		out.OptionalMaterials = append(out.OptionalMaterials, material)
	}
	return out
}

func dataTaskConstrainWorkflowRequiredMaterials(previous dataquery.CoverageContract, current dataquery.TaskPlan) dataquery.CoverageContract {
	if len(current.CoverageContract.RequiredMaterials) == 0 {
		return current.CoverageContract
	}
	currentInputs := dataTaskCurrentBatchInputPathSet(current)
	if len(currentInputs) == 0 {
		return current.CoverageContract
	}
	previousRequired := dataTaskCoverageMaterialMap(previous.RequiredMaterials)
	userFloorMode := dataTaskCoverageHasUserExplicitFloor(previous.RequiredMaterials) ||
		dataTaskCoverageHasUserExplicitFloor(previous.OptionalMaterials) ||
		dataTaskCoverageHasUserExplicitFloor(current.CoverageContract.RequiredMaterials) ||
		dataTaskCoverageHasUserExplicitFloor(current.CoverageContract.OptionalMaterials)
	out := current.CoverageContract
	out.RequiredMaterials = nil
	for _, material := range current.CoverageContract.RequiredMaterials {
		if dataTaskCoverageMaterialShouldRemainRequiredInWorkflow(material, previousRequired, currentInputs, userFloorMode) {
			out.RequiredMaterials = append(out.RequiredMaterials, material)
			continue
		}
		if strings.TrimSpace(string(material.UsageMode)) == "" {
			material.UsageMode = dataquery.MaterialUseReferenceOnly
		}
		material.Required = dataTaskCoverageMaterialIsUserExplicitFloor(material)
		out.OptionalMaterials = append(out.OptionalMaterials, material)
	}
	return out
}

func dataTaskCoverageMaterialIsUserExplicitFloor(material dataquery.CoverageMaterial) bool {
	return strings.TrimSpace(material.Purpose) == dataTaskUserExplicitMaterialPurpose
}

func dataTaskCoverageHasUserExplicitFloor(materials []dataquery.CoverageMaterial) bool {
	for _, material := range materials {
		if dataTaskCoverageMaterialIsUserExplicitFloor(material) {
			return true
		}
	}
	return false
}

func dataTaskCoverageMaterialMap(materials []dataquery.CoverageMaterial) map[string]dataquery.CoverageMaterial {
	out := map[string]dataquery.CoverageMaterial{}
	for _, material := range materials {
		if key := dataTaskCoverageMaterialKey(material); key != "" {
			out[key] = material
		}
	}
	return out
}

func dataTaskCoverageMaterialShouldRemainRequiredInCurrentBatch(material dataquery.CoverageMaterial, previousRequired, currentInputs map[string]bool) bool {
	key := dataTaskCoverageMaterialKey(material)
	if key != "" && previousRequired[key] {
		return true
	}
	for _, raw := range []string{material.Path, material.TextEvidencePath} {
		p := normalizeDataTaskCoveragePath(raw)
		if p != "" && currentInputs[p] {
			return true
		}
	}
	mode := normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode)
	return mode == dataquery.MaterialUsePlannerDistilled && len(material.DistilledNotes) > 0
}

func dataTaskCoverageMaterialShouldRemainRequiredInWorkflow(material dataquery.CoverageMaterial, previousRequired map[string]dataquery.CoverageMaterial, currentInputs map[string]bool, userFloorMode bool) bool {
	if dataTaskCoverageMaterialIsUserExplicitFloor(material) {
		return true
	}
	key := dataTaskCoverageMaterialKey(material)
	if key != "" {
		if previous, ok := previousRequired[key]; ok {
			return !userFloorMode || dataTaskCoverageMaterialIsUserExplicitFloor(previous)
		}
	}
	if userFloorMode {
		return false
	}
	for _, raw := range []string{material.Path, material.TextEvidencePath} {
		p := normalizeDataTaskCoveragePath(raw)
		if p != "" && currentInputs[p] {
			return true
		}
	}
	mode := normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode)
	return mode == dataquery.MaterialUsePlannerDistilled && len(material.DistilledNotes) > 0
}

func dataTaskCurrentBatchInputPathSet(plan dataquery.TaskPlan) map[string]bool {
	out := map[string]bool{}
	for _, p := range dataTaskCurrentBatchInputPaths(plan) {
		normalized := normalizeDataTaskCoveragePath(p)
		if normalized != "" {
			out[normalized] = true
		}
	}
	return out
}

func dataTaskCurrentBatchInputPaths(plan dataquery.TaskPlan) []string {
	var inputs []string
	for _, action := range plan.Actions {
		inputs = append(inputs, action.InputPaths...)
	}
	if len(cleanDataTaskStrings(inputs)) == 0 && len(plan.Actions) == 0 {
		inputs = append(inputs, plan.InputPaths...)
	}
	return cleanDataTaskStrings(inputs)
}

func preserveDataTaskWorkflowMaterialCoverage(records []dataTaskWorkflowRecord, current, next dataquery.TaskPlan) dataquery.TaskPlan {
	workflow := current
	workflow.CoverageContract = dataTaskWorkflowCoverageContract(records, current)
	preserved := preserveDataTaskMaterialRepairCoverage(workflow, next)
	preserved.CoverageContract = dataTaskCoverageContractWithoutGeneratedScriptMaterials(preserved.CoverageContract, dataTaskWorkflowGeneratedArtifactPathSet(records))
	return preserved
}

func preserveDataTaskWorkflowMaterialCoverageForError(records []dataTaskWorkflowRecord, current, next dataquery.TaskPlan, errText string) dataquery.TaskPlan {
	workflow := current
	workflow.CoverageContract = dataTaskWorkflowCoverageContract(records, current)
	preserved := preserveDataTaskMaterialRepairCoverageForError(workflow, next, errText)
	preserved.CoverageContract = dataTaskCoverageContractWithoutGeneratedScriptMaterials(preserved.CoverageContract, dataTaskWorkflowGeneratedArtifactPathSet(records))
	return preserved
}

func validateDataTaskWorkflowResult(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) error {
	result = dataquery.NormalizeResult(result)
	contract := dataTaskWorkflowCoverageContract(records, current)
	return dataquery.ValidateResultAgainstContract(contract, result)
}

func dataTaskWorkflowCompletionGateError(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) string {
	return dataTaskWorkflowCompletionGateGuardResultWithRepo("", records, current, result).ErrorText()
}

func dataTaskWorkflowCompletionGateErrorWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) string {
	return dataTaskWorkflowCompletionGateGuardResultWithRepo(repoRoot, records, current, result).ErrorText()
}

func dataTaskWorkflowCompletionGateGuardResult(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataworkflow.GuardResult {
	return dataTaskWorkflowCompletionGateGuardResultWithRepo("", records, current, result)
}

func dataTaskWorkflowCompletionGateGuardResultWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataworkflow.GuardResult {
	result = dataquery.NormalizeResult(result)
	var validationErr error
	if err := validateDataTaskWorkflowResult(records, current, result); err != nil {
		validationErr = err
	}
	var gap dataworkflow.ReferenceProjectionGap
	candidate, _, gapDeclared, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, result)
	if hasReferenceGap {
		gap = dataworkflow.ReferenceProjectionGap{Candidate: candidate, Present: true, Declared: gapDeclared}
	}
	return dataworkflow.CompletionGateGuardResult(dataworkflow.CompletionGateGuardInput{
		ValidationErr: validationErr,
		LedgerGraph:   dataTaskWorkflowCompletionLedgerGraph(records, current, result),
		OutputGraph:   dataTaskWorkflowCompletionOutputProjectionGraph(repoRoot, records, current, result),
		ReferenceGap:  gap,
	})
}

func dataTaskWorkflowCompletionLedgerGuardResult(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataworkflow.GuardResult {
	return dataworkflow.LedgerGraphCompletionGuardResult(dataTaskWorkflowCompletionLedgerGraph(records, current, result))
}

func dataTaskWorkflowCompletionLedgerGraph(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataworkflow.LedgerGraph {
	result = dataquery.NormalizeResult(result)
	completionRecords := make([]dataTaskWorkflowRecord, 0, len(records)+1)
	completionRecords = append(completionRecords, records...)
	completionRecords = append(completionRecords, dataTaskWorkflowRecord{Plan: current, Result: &result})
	state := dataTaskWorkflowState(completionRecords, current)
	return state.LedgerGraph
}

func dataTaskResultNeedsOutputProjection(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) bool {
	return dataworkflow.ResultNeedsOutputProjection(dataworkflow.ResultProjectionNeedInput{
		Current:                current,
		Coverage:               dataTaskWorkflowCoverageContract(records, current),
		Output:                 dataTaskWorkflowOutputContract(records, current),
		Result:                 result,
		PlanHasCustomTransform: dataTaskPlanHasCustomTransform(current),
	})
}

func dataTaskOutputContractNeedsFinalProjection(contract dataquery.OutputContract) bool {
	return dataworkflow.OutputContractNeedsFinalProjection(contract)
}

func dataTaskWorkflowCompletionOutputProjectionGraph(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataworkflow.OutputProjectionGraph {
	result = dataquery.NormalizeResult(result)
	var referenceGap dataworkflow.ReferenceProjectionGap
	var answerItems int
	candidate, count, gapDeclared, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, result)
	if hasReferenceGap {
		referenceGap = dataworkflow.ReferenceProjectionGap{Candidate: candidate, Present: true, Declared: gapDeclared}
		answerItems = count
	}
	reconcileGroups := 0
	if result.Reconcile != nil {
		reconcileGroups = len(result.Reconcile.Groups)
	}
	return dataworkflow.BuildOutputProjectionGraph(dataworkflow.OutputProjectionGraphInput{
		Output:                    firstNonEmptyOutputContract(result.OutputContract, dataTaskWorkflowOutputContract(records, current), current.OutputContract),
		Coverage:                  dataTaskWorkflowCoverageContract(records, current),
		AnswerPresent:             dataworkflow.ResultAnswerPresent(result),
		ProjectionArtifactPresent: dataworkflow.ResultHasAssembleAnswerArtifact(result),
		ReconcilePresent:          result.Reconcile != nil,
		ReconcileGroups:           reconcileGroups,
		PlanHasCustomTransform:    dataTaskPlanHasCustomTransform(current),
		ReferenceGapPresent:       referenceGap.Present && referenceGap.Declared,
		ReferenceKeyCount:         referenceGap.Candidate.KeyCount,
		AnswerItemCount:           answerItems,
	})
}

// dataTaskOutputReferenceProjectionGap reports a reference-projection
// gap plus whether it is DECLARED: true only when the output contract
// itself carries complete_reference=true (typed declaration); false
// for gaps inferred by comparing answer items against a candidate
// reference's key universe. Hard consumers (completion-gate error,
// zero-fill plan synthesis) act only on declared gaps.
func dataTaskOutputReferenceProjectionGap(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) (dataquery.ReferenceKeyCandidate, int, bool, bool) {
	if result.Reconcile == nil || len(result.Reconcile.Groups) == 0 {
		return dataquery.ReferenceKeyCandidate{}, 0, false, false
	}
	contract := firstNonEmptyOutputContract(result.OutputContract, dataTaskWorkflowOutputContract(records, current), current.OutputContract).Normalize()
	if contract.Format == "" || (contract.Format == dataquery.OutputFreeform && contract.ExplanationAllowed) {
		return dataquery.ReferenceKeyCandidate{}, 0, false, false
	}
	groupKeys := dataTaskReconcileGroupKeys(result.Reconcile)
	if len(groupKeys) == 0 {
		return dataquery.ReferenceKeyCandidate{}, 0, false, false
	}
	if dataquery.ReconcileGroupsPreferListProjection(result.Reconcile.Groups, result.Contributions) {
		return dataquery.ReferenceKeyCandidate{}, inferDataTaskAnswerItemCount(result.Answer, contract), false, false
	}
	candidatePaths := dataTaskReferenceCandidatePaths(records, current, result, contract)
	primaryCandidatePaths, aggregateCandidatePaths := dataTaskReferenceProjectionCandidatePathBuckets(records, result, candidatePaths)
	candidateFields := cleanDataTaskStrings([]string{contract.ReferenceKeyField})
	runner := dataquery.ActionRunner{
		RepoRoot: strings.TrimSpace(repoRoot),
		Seed:     result,
	}
	if contract.CompleteReference && strings.TrimSpace(contract.ReferencePath) != "" {
		candidate, ok := dataTaskExplicitReferenceProjectionCandidate(runner, contract.ReferencePath, contract.ReferenceKeyField, groupKeys)
		if ok {
			answerItems := inferDataTaskAnswerItemCount(result.Answer, contract)
			if dataTaskResultHasReferenceProjection(result, candidate, answerItems, contract) {
				return dataquery.ReferenceKeyCandidate{}, answerItems, false, false
			}
			if dataTaskResultHasReferenceProjectionMetadata(result, candidate, answerItems, contract) {
				return candidate, answerItems, true, true
			}
			if dataTaskReferenceCandidateNeedsProjection(candidate, groupKeys, answerItems, dataworkflow.ResultAnswerPresent(result)) {
				return candidate, answerItems, true, true
			}
			return dataquery.ReferenceKeyCandidate{}, answerItems, false, false
		}
	}
	if candidate, answerItems, ok := dataTaskAssembleActionReferenceProjectionGap(runner, current, contract, groupKeys, result); ok {
		// The model's own assemble action named the reference material
		// as its input (with a group-key field) — a typed declaration
		// of the intended key universe, same standing as
		// contract.complete_reference.
		return candidate, answerItems, true, true
	}
	answerItems := inferDataTaskAnswerItemCount(result.Answer, contract)
	candidate, ok := dataTaskBestReferenceProjectionGapCandidate(runner, primaryCandidatePaths, candidateFields, groupKeys, result, answerItems, contract)
	if !ok && len(candidateFields) > 0 {
		candidate, ok = dataTaskBestReferenceProjectionGapCandidate(runner, primaryCandidatePaths, nil, groupKeys, result, answerItems, contract)
	}
	if !ok && len(aggregateCandidatePaths) > 0 {
		candidate, ok = dataTaskBestReferenceProjectionGapCandidate(runner, aggregateCandidatePaths, candidateFields, groupKeys, result, answerItems, contract)
		if !ok && len(candidateFields) > 0 {
			candidate, ok = dataTaskBestReferenceProjectionGapCandidate(runner, aggregateCandidatePaths, nil, groupKeys, result, answerItems, contract)
		}
	}
	if !ok {
		return dataquery.ReferenceKeyCandidate{}, 0, false, false
	}
	declared := dataTaskPlanDeclaresCompleteReference(current, contract)
	if !dataworkflow.ResultAnswerPresent(result) {
		return candidate, answerItems, declared, true
	}
	if dataTaskReferenceCandidateNeedsProjection(candidate, groupKeys, answerItems, true) {
		return candidate, answerItems, declared, true
	}
	return dataquery.ReferenceKeyCandidate{}, answerItems, false, false
}

// dataTaskPlanDeclaresCompleteReference reports the typed declaration:
// either the resolved contract or any plan-side output contract in the
// workflow carries complete_reference=true. Inferred gap candidates
// never count as declared.
func dataTaskPlanDeclaresCompleteReference(current dataquery.TaskPlan, contract dataquery.OutputContract) bool {
	return contract.CompleteReference || current.OutputContract.CompleteReference
}

func dataTaskBestReferenceProjectionGapCandidate(runner dataquery.ActionRunner, paths, fields, groupKeys []string, result dataquery.Result, answerItems int, contract dataquery.OutputContract) (dataquery.ReferenceKeyCandidate, bool) {
	answerPresent := dataworkflow.ResultAnswerPresent(result)
	var best dataquery.ReferenceKeyCandidate
	bestSet := false
	for _, path := range cleanDataTaskStrings(paths) {
		for _, candidate := range dataTaskReferenceCandidatesForPath(runner, path, fields, groupKeys) {
			if candidate.ExistingMatchCount == 0 && len(cleanDataTaskStrings(groupKeys)) > 0 {
				continue
			}
			if dataTaskResultHasReferenceProjection(result, candidate, answerItems, contract) {
				continue
			}
			if dataTaskResultHasReferenceProjectionMetadata(result, candidate, answerItems, contract) {
				return candidate, true
			}
			if !dataTaskReferenceCandidateNeedsProjection(candidate, groupKeys, answerItems, answerPresent) {
				continue
			}
			if !bestSet || dataTaskReferenceProjectionCandidateBetter(candidate, best) {
				best = candidate
				bestSet = true
			}
		}
	}
	if !bestSet {
		return dataquery.ReferenceKeyCandidate{}, false
	}
	return best, true
}

func dataTaskExplicitReferenceProjectionCandidate(runner dataquery.ActionRunner, path, field string, groupKeys []string) (dataquery.ReferenceKeyCandidate, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return dataquery.ReferenceKeyCandidate{}, false
	}
	var best dataquery.ReferenceKeyCandidate
	bestSet := false
	for _, candidate := range dataTaskReferenceCandidatesForPath(runner, path, cleanDataTaskStrings([]string{field}), groupKeys) {
		if !bestSet || dataTaskReferenceProjectionCandidateBetter(candidate, best) {
			best = candidate
			bestSet = true
		}
	}
	for _, candidate := range dataTaskReferenceCandidatesForPath(runner, path, nil, groupKeys) {
		if !bestSet || dataTaskReferenceProjectionCandidateBetter(candidate, best) {
			best = candidate
			bestSet = true
		}
	}
	if !bestSet {
		return dataquery.ReferenceKeyCandidate{}, false
	}
	return best, true
}

func dataTaskAssembleActionReferenceProjectionGap(runner dataquery.ActionRunner, current dataquery.TaskPlan, contract dataquery.OutputContract, groupKeys []string, result dataquery.Result) (dataquery.ReferenceKeyCandidate, int, bool) {
	answerItems := inferDataTaskAnswerItemCount(result.Answer, contract)
	answerPresent := dataworkflow.ResultAnswerPresent(result)
	var best dataquery.ReferenceKeyCandidate
	bestSet := false
	for _, action := range current.Actions {
		if dataworkflow.NormalizeActionKind(action.Kind) != dataquery.DataActionAssembleAnswer {
			continue
		}
		fields := dataTaskAssembleActionDeclaredReferenceFields(action, contract)
		if len(fields) == 0 {
			continue
		}
		paths := cleanDataTaskStrings(append(append([]string{}, action.InputPaths...), action.Params["reference_path"], action.Params["reference_paths"]))
		for _, path := range paths {
			for _, candidate := range dataTaskReferenceCandidatesForPath(runner, path, fields, groupKeys) {
				if candidate.ExistingMatchCount == 0 && len(cleanDataTaskStrings(groupKeys)) > 0 {
					continue
				}
				if dataTaskResultHasReferenceProjection(result, candidate, answerItems, contract) {
					continue
				}
				if dataTaskResultHasReferenceProjectionMetadata(result, candidate, answerItems, contract) {
					return candidate, answerItems, true
				}
				if !dataTaskReferenceCandidateNeedsProjection(candidate, groupKeys, answerItems, answerPresent) {
					continue
				}
				if !bestSet || dataTaskReferenceProjectionCandidateBetter(candidate, best) {
					best = candidate
					bestSet = true
				}
			}
			if bestSet {
				continue
			}
			for _, candidate := range dataTaskReferenceCandidatesForPath(runner, path, nil, groupKeys) {
				if candidate.ExistingMatchCount == 0 && len(cleanDataTaskStrings(groupKeys)) > 0 {
					continue
				}
				if dataTaskResultHasReferenceProjection(result, candidate, answerItems, contract) {
					continue
				}
				if dataTaskResultHasReferenceProjectionMetadata(result, candidate, answerItems, contract) {
					return candidate, answerItems, true
				}
				if !dataTaskReferenceCandidateNeedsProjection(candidate, groupKeys, answerItems, answerPresent) {
					continue
				}
				if !bestSet || dataTaskReferenceProjectionCandidateBetter(candidate, best) {
					best = candidate
					bestSet = true
				}
			}
		}
	}
	if bestSet {
		return best, answerItems, true
	}
	return dataquery.ReferenceKeyCandidate{}, answerItems, false
}

func dataTaskAssembleActionDeclaredReferenceFields(action dataquery.DataAction, contract dataquery.OutputContract) []string {
	fields := []string{
		action.Params["reference_key_field"],
		action.Params["group_key_field"],
		action.Params["key_field"],
	}
	if contract.CompleteReference || parseBoolDataTaskString(action.Params["complete_reference"]) {
		fields = append(fields, contract.ReferenceKeyField)
	}
	return cleanDataTaskStrings(fields)
}

func dataTaskResultHasReferenceProjection(result dataquery.Result, candidate dataquery.ReferenceKeyCandidate, answerItems int, contract dataquery.OutputContract) bool {
	return dataTaskResultHasReferenceProjectionMatch(result, candidate, answerItems, contract, true)
}

func dataTaskResultHasReferenceProjectionMetadata(result dataquery.Result, candidate dataquery.ReferenceKeyCandidate, answerItems int, contract dataquery.OutputContract) bool {
	return dataTaskResultHasReferenceProjectionMatch(result, candidate, answerItems, contract, false)
}

func dataTaskResultHasReferenceProjectionMatch(result dataquery.Result, candidate dataquery.ReferenceKeyCandidate, answerItems int, contract dataquery.OutputContract, requireValueBinding bool) bool {
	if candidate.KeyCount <= 0 || !dataworkflow.ResultAnswerPresent(result) {
		return false
	}
	if dataTaskReferenceProjectionItemCountMismatch(answerItems, candidate.KeyCount) {
		return false
	}
	wantPath := normalizeDataTaskCoveragePath(firstNonEmptyString(candidate.Path, contract.ReferencePath))
	wantField := strings.TrimSpace(firstNonEmptyString(candidate.Field, contract.ReferenceKeyField))
	for i := len(result.Artifacts) - 1; i >= 0; i-- {
		artifact := result.Artifacts[i]
		if strings.TrimSpace(artifact.Kind) != string(dataquery.DataActionAssembleAnswer) {
			continue
		}
		fields := artifact.Fields
		if !strings.EqualFold(strings.TrimSpace(fields["reference_projected"]), "true") {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(fields["reference_key_count"]))
		if err != nil || count != candidate.KeyCount {
			continue
		}
		gotPath := normalizeDataTaskCoveragePath(fields["reference_path"])
		if wantPath != "" && gotPath != wantPath {
			continue
		}
		gotField := strings.TrimSpace(fields["reference_key_field"])
		if wantField != "" && !strings.EqualFold(gotField, wantField) {
			continue
		}
		if requireValueBinding && !dataTaskReferenceProjectionValuesBound(result, artifact, candidate, contract) {
			continue
		}
		return true
	}
	return false
}

func dataTaskReferenceProjectionValuesBound(result dataquery.Result, artifact dataquery.DataArtifact, candidate dataquery.ReferenceKeyCandidate, contract dataquery.OutputContract) bool {
	if result.Reconcile == nil || len(candidate.Keys) == 0 {
		return true
	}
	projection := strings.ToLower(strings.TrimSpace(artifact.Fields["projection"]))
	if projection == "" {
		switch contract.Normalize().Format {
		case dataquery.OutputCSVLine:
			projection = "values"
		case dataquery.OutputPlainSingleLine:
			if !contract.Normalize().ExplanationAllowed {
				projection = "values"
			}
		}
	}
	switch projection {
	case "values", "value_list", "csv_values":
	default:
		return true
	}
	if parseBoolDataTaskString(artifact.Fields["include_keys"]) {
		return true
	}
	delimiter := firstNonEmptyString(artifact.Fields["delimiter"], contract.Normalize().Delimiter, ",")
	valueField := firstNonEmptyString(artifact.Fields["value_field"], "actual")
	metric := firstNonEmptyString(artifact.Fields["metric"], dataTaskSingleBusinessReconcileMetric(result.Reconcile.Groups))
	expected, ok := dataTaskReferenceProjectionExpectedValues(result.Reconcile.Groups, candidate.Keys, metric, valueField)
	if !ok {
		return true
	}
	actual := splitDataTaskProjectionValues(result.Answer, delimiter)
	if len(actual) != len(expected) {
		return false
	}
	for i := range expected {
		if !dataTaskProjectionValueEqual(actual[i], expected[i]) {
			return false
		}
	}
	return true
}

func dataTaskReferenceProjectionExpectedValues(groups []dataquery.ReconcileGroup, keys []string, metric, valueField string) ([]string, bool) {
	if len(keys) == 0 {
		return nil, false
	}
	metric = strings.TrimSpace(metric)
	existing := make(map[string]string, len(groups))
	for _, group := range groups {
		if dataTaskReconcileGroupIsFinalAnswerProjection(group) {
			continue
		}
		groupMetric := strings.TrimSpace(group.Metric.String())
		if metric != "" && groupMetric != "" && groupMetric != metric {
			continue
		}
		key := strings.TrimSpace(group.GroupKey.String())
		if key == "" {
			continue
		}
		if _, seen := existing[key]; seen {
			continue
		}
		value := dataTaskReconcileGroupValueByField(group, valueField)
		if value == "" {
			value = "0"
		}
		existing[key] = value
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value := existing[key]
		if value == "" {
			value = "0"
		}
		out = append(out, value)
	}
	return out, len(out) > 0
}

func dataTaskSingleBusinessReconcileMetric(groups []dataquery.ReconcileGroup) string {
	var metric string
	for _, group := range groups {
		if dataTaskReconcileGroupIsFinalAnswerProjection(group) {
			continue
		}
		value := strings.TrimSpace(group.Metric.String())
		if value == "" {
			continue
		}
		if metric == "" {
			metric = value
			continue
		}
		if metric != value {
			return ""
		}
	}
	return metric
}

func dataTaskReconcileGroupIsFinalAnswerProjection(group dataquery.ReconcileGroup) bool {
	if strings.EqualFold(strings.TrimSpace(group.Scope.String()), "final_answer") &&
		strings.EqualFold(strings.TrimSpace(group.Role.String()), "output") &&
		strings.EqualFold(strings.TrimSpace(group.Metric.String()), "projection") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(group.GroupKey.String()), "final_answer") &&
		strings.EqualFold(strings.TrimSpace(group.Metric.String()), "projection")
}

func dataTaskReconcileGroupValueByField(group dataquery.ReconcileGroup, field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "expected":
		return strings.TrimSpace(firstNonEmptyString(group.Expected.String(), group.Actual.String()))
	case "actual", "":
		return strings.TrimSpace(firstNonEmptyString(group.Actual.String(), group.Expected.String()))
	default:
		return strings.TrimSpace(firstNonEmptyString(group.Actual.String(), group.Expected.String()))
	}
}

func splitDataTaskProjectionValues(answer, delimiter string) []string {
	delimiter = firstNonEmptyString(delimiter, ",")
	raw := strings.Split(strings.TrimSpace(answer), delimiter)
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

func dataTaskProjectionValueEqual(actual, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	if actual == expected {
		return true
	}
	a, aOK := new(big.Rat).SetString(actual)
	b, bOK := new(big.Rat).SetString(expected)
	return aOK && bOK && a.Cmp(b) == 0
}

func parseBoolDataTaskString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func dataTaskReferenceCandidatesForPath(runner dataquery.ActionRunner, path string, fields []string, groupKeys []string) []dataquery.ReferenceKeyCandidate {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	var candidates []dataquery.ReferenceKeyCandidate
	if len(fields) == 0 {
		candidates = runner.ReferenceKeyCandidatesForPath(path, 100000)
	} else {
		for _, field := range cleanDataTaskStrings(fields) {
			candidate, ok := runner.ReferenceKeyCandidateForPath(path, field, 100000)
			if !ok || candidate.KeyCount == 0 {
				continue
			}
			candidates = append(candidates, candidate)
		}
	}
	for i := range candidates {
		candidates[i] = dataTaskAnnotateReferenceCandidateOverlap(candidates[i], groupKeys)
	}
	return candidates
}

func dataTaskAnnotateReferenceCandidateOverlap(candidate dataquery.ReferenceKeyCandidate, groupKeys []string) dataquery.ReferenceKeyCandidate {
	groups := cleanDataTaskStrings(groupKeys)
	groupSet := make(map[string]bool, len(groups))
	for _, key := range groups {
		groupSet[key] = true
	}
	match := 0
	for _, key := range cleanDataTaskStrings(candidate.Keys) {
		if groupSet[key] {
			match++
		}
	}
	candidate.ExistingMatchCount = match
	candidate.MissingCount = candidate.KeyCount - match
	if candidate.MissingCount < 0 {
		candidate.MissingCount = 0
	}
	return candidate
}

func dataTaskReferenceProjectionCandidateBetter(candidate, current dataquery.ReferenceKeyCandidate) bool {
	candidateHasReferenceOnly := candidate.MissingCount > 0
	currentHasReferenceOnly := current.MissingCount > 0
	if candidateHasReferenceOnly != currentHasReferenceOnly {
		return candidateHasReferenceOnly
	}
	if candidate.ExistingMatchCount != current.ExistingMatchCount {
		return candidate.ExistingMatchCount > current.ExistingMatchCount
	}
	if candidate.MissingCount != current.MissingCount {
		return candidate.MissingCount < current.MissingCount
	}
	if candidate.KeyCount != current.KeyCount {
		return candidate.KeyCount < current.KeyCount
	}
	if candidate.Path != current.Path {
		return candidate.Path < current.Path
	}
	return candidate.Field < current.Field
}

func dataTaskReferenceCandidateNeedsProjection(candidate dataquery.ReferenceKeyCandidate, groupKeys []string, answerItems int, answerPresent bool) bool {
	if candidate.KeyCount <= 0 {
		return false
	}
	if !answerPresent {
		return true
	}
	if dataTaskReferenceProjectionItemCountMismatch(answerItems, candidate.KeyCount) {
		return true
	}
	return dataTaskReferenceKeySetMismatch(candidate.Keys, groupKeys)
}

func dataTaskReferenceKeySetMismatch(referenceKeys, groupKeys []string) bool {
	reference := cleanDataTaskStrings(referenceKeys)
	groups := cleanDataTaskStrings(groupKeys)
	if len(reference) != len(groups) {
		return true
	}
	if len(reference) == 0 {
		return false
	}
	groupSet := make(map[string]bool, len(groups))
	for _, key := range groups {
		groupSet[key] = true
	}
	for _, key := range reference {
		if !groupSet[key] {
			return true
		}
	}
	return false
}

func dataTaskReferenceProjectionItemCountMismatch(answerItems, referenceKeys int) bool {
	if referenceKeys <= 0 {
		return false
	}
	if referenceKeys == 1 {
		return answerItems > 1
	}
	return answerItems != referenceKeys
}

func dataTaskReconcileGroupKeys(report *dataquery.ReconcileReport) []string {
	if report == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, group := range report.Groups {
		key := strings.TrimSpace(group.GroupKey.String())
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return cleanDataTaskStrings(out)
}

func dataTaskWorkflowArtifacts(records []dataTaskWorkflowRecord, result dataquery.Result) []dataquery.DataArtifact {
	var out []dataquery.DataArtifact
	for _, rec := range records {
		if rec.Result != nil {
			out = append(out, rec.Result.Artifacts...)
		}
	}
	out = append(out, result.Artifacts...)
	return out
}

func dataTaskReferenceCandidatePaths(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, contract dataquery.OutputContract) []string {
	var paths []string
	paths = append(paths, contract.ReferencePath)
	paths = append(paths, current.InputPaths...)
	for _, action := range current.Actions {
		paths = append(paths, action.InputPaths...)
		paths = append(paths, action.Params["reference_path"], action.Params["reference_paths"])
	}
	paths = append(paths, result.ConsumedPaths...)
	for _, rec := range records {
		paths = append(paths, rec.Plan.InputPaths...)
		for _, action := range rec.Plan.Actions {
			paths = append(paths, action.InputPaths...)
			paths = append(paths, action.Params["reference_path"], action.Params["reference_paths"])
		}
		if rec.Result != nil {
			paths = append(paths, rec.Result.ConsumedPaths...)
		}
	}
	return cleanDataTaskStrings(paths)
}

func dataTaskReferenceProjectionCandidatePathBuckets(records []dataTaskWorkflowRecord, result dataquery.Result, paths []string) ([]string, []string) {
	aggregateAliases := dataTaskMultiSourceArtifactAliasSet(records, result)
	var primary []string
	var aggregate []string
	for _, path := range cleanDataTaskStrings(paths) {
		normalized := normalizeDataTaskCoveragePath(path)
		if normalized != "" && aggregateAliases[normalized] {
			aggregate = append(aggregate, path)
			continue
		}
		primary = append(primary, path)
	}
	return cleanDataTaskStrings(primary), cleanDataTaskStrings(aggregate)
}

func dataTaskMultiSourceArtifactAliasSet(records []dataTaskWorkflowRecord, latest dataquery.Result) map[string]bool {
	out := map[string]bool{}
	var mark func(dataquery.DataArtifact)
	mark = func(artifact dataquery.DataArtifact) {
		if dataTaskArtifactLooksMultiSourceAggregate(artifact) {
			for _, alias := range dataTaskArtifactAliasPaths(artifact) {
				normalized := normalizeDataTaskCoveragePath(alias)
				if normalized != "" {
					out[normalized] = true
				}
			}
			if artifact.Fields != nil {
				normalized := normalizeDataTaskCoveragePath(artifact.Fields["artifact_path"])
				if normalized != "" {
					out[normalized] = true
				}
			}
		}
		for _, child := range artifact.Children {
			mark(child)
		}
	}
	for _, artifact := range latest.Artifacts {
		mark(artifact)
	}
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		for _, artifact := range rec.Result.Artifacts {
			mark(artifact)
		}
	}
	return out
}

func dataTaskArtifactLooksMultiSourceAggregate(artifact dataquery.DataArtifact) bool {
	if len(cleanDataTaskStrings(artifact.SourcePaths)) > 1 {
		return true
	}
	childSources := map[string]bool{}
	var collectChildSources func(dataquery.DataArtifact)
	collectChildSources = func(child dataquery.DataArtifact) {
		for _, path := range cleanDataTaskStrings(child.SourcePaths) {
			normalized := normalizeDataTaskCoveragePath(path)
			if normalized != "" {
				childSources[normalized] = true
			}
		}
		for _, grandchild := range child.Children {
			collectChildSources(grandchild)
		}
	}
	for _, child := range artifact.Children {
		collectChildSources(child)
	}
	return len(childSources) > 1
}

func dataTaskRequiredOutputProjectionPlan(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) (dataquery.TaskPlan, bool) {
	return dataTaskRequiredOutputProjectionPlanWithRepo("", records, current, result)
}

func dataTaskRequiredOutputProjectionPlanWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) (dataquery.TaskPlan, bool) {
	var gap dataworkflow.ReferenceProjectionGap
	candidate, _, gapDeclared, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, result)
	if hasReferenceGap {
		gap = dataworkflow.ReferenceProjectionGap{Candidate: candidate, Present: true, Declared: gapDeclared}
	}
	return dataworkflow.BuildRequiredOutputProjectionPlan(dataworkflow.OutputProjectionPlanInput{
		Current:                current,
		Coverage:               dataTaskWorkflowCoverageContract(records, current),
		Output:                 dataTaskWorkflowOutputContract(records, current),
		Result:                 result,
		OutputGraph:            dataTaskWorkflowCompletionOutputProjectionGraph(repoRoot, records, current, result),
		UseOutputGraph:         true,
		ReferenceGap:           gap,
		PlanHasCustomTransform: dataTaskPlanHasCustomTransform(current),
	})
}

func dataTaskRequiredLedgerCompletionPlan(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, guard dataworkflow.GuardResult) (dataquery.TaskPlan, bool) {
	return dataTaskRequiredLedgerCompletionPlanWithRepo("", records, current, result, guard)
}

func dataTaskRequiredLedgerCompletionPlanWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, guard dataworkflow.GuardResult) (dataquery.TaskPlan, bool) {
	transition := dataTaskCompletionRepairTransitionWithRepo(repoRoot, records, current, result, guard)
	if transition.HasPlan() {
		return transition.Plan, true
	}
	return dataquery.TaskPlan{}, false
}

func dataTaskCompletionRepairTransitionWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, guard dataworkflow.GuardResult) dataworkflow.CompletionRepairTransition {
	return dataworkflow.BuildCompletionRepairTransition(dataTaskCompletionRepairTransitionInputWithRepo(repoRoot, records, current, result, guard))
}

func dataTaskValidationFailureTransitionWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, guard dataworkflow.GuardResult) dataworkflow.ValidationFailureTransition {
	return dataworkflow.BuildValidationFailureTransition(dataTaskCompletionRepairTransitionInputWithRepo(repoRoot, records, current, result, guard))
}

func dataTaskCompletionRepairTransitionInputWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, guard dataworkflow.GuardResult) dataworkflow.CompletionRepairTransitionInput {
	result = dataquery.NormalizeResult(result)
	var gap dataworkflow.ReferenceProjectionGap
	candidate, _, gapDeclared, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, result)
	if hasReferenceGap {
		gap = dataworkflow.ReferenceProjectionGap{Candidate: candidate, Present: true, Declared: gapDeclared}
	}
	completionRecords := make([]dataTaskWorkflowRecord, 0, len(records)+1)
	completionRecords = append(completionRecords, records...)
	completionRecords = append(completionRecords, dataTaskWorkflowRecord{Plan: current, Result: &result})
	return dataworkflow.CompletionRepairTransitionInput{
		Current:                current,
		Coverage:               dataTaskWorkflowCoverageContract(records, current),
		Output:                 dataTaskWorkflowOutputContract(records, current),
		Result:                 result,
		LedgerGraph:            dataTaskWorkflowCompletionLedgerGraph(records, current, result),
		UseLedgerGraph:         true,
		OutputGraph:            dataTaskWorkflowCompletionOutputProjectionGraph(repoRoot, records, current, result),
		UseOutputGraph:         true,
		Guard:                  guard,
		Artifacts:              dataTaskWorkflowArtifactSchemaProjections(completionRecords),
		SeenActionKeys:         dataTaskWorkflowSeenActionKeys(records),
		ReferenceGap:           gap,
		PlanHasCustomTransform: dataTaskPlanHasCustomTransform(current),
	}
}

func dataTaskTerminalWorkflowFallback(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) (dataquery.TaskPlan, string, bool) {
	if !dataTaskPlanStatusLooksTerminal(current.Status) {
		return dataquery.TaskPlan{}, "", false
	}
	fallback, reason, ok := dataTaskWorkflowNextStageFallback(records, current, "terminal plan ended")
	if !ok {
		return dataquery.TaskPlan{}, "", false
	}
	if len(fallback.Actions) == 1 && normalizeDataActionKindForWorkflow(fallback.Actions[0].Kind) == dataquery.DataActionExtractRecords {
		return dataquery.TaskPlan{}, "", false
	}
	return fallback, reason, true
}

func dataTaskTerminalWorkflowDecision(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataworkflow.TerminalWorkflowDecision {
	if !dataTaskPlanStatusLooksTerminal(current.Status) {
		return dataworkflow.TerminalWorkflowDecision{}
	}
	fallback, reason, ok := dataTaskWorkflowNextStageFallback(records, current, "terminal plan ended")
	state := dataTaskWorkflowState(records, current)
	return dataworkflow.DecideTerminalWorkflow(dataworkflow.TerminalWorkflowDecisionInput{
		Current:                 current,
		Facts:                   state.Facts(),
		AllowedNextActions:      state.AllowedNextActions,
		FallbackPlan:            fallback,
		FallbackReason:          reason,
		FallbackAvailable:       ok,
		SuppressFallbackActions: []dataquery.DataActionKind{dataquery.DataActionExtractRecords},
	})
}

func dataTaskDeterministicContinuationFallback(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, err error) (dataquery.TaskPlan, string, bool) {
	if err == nil {
		return dataquery.TaskPlan{}, "", false
	}
	return dataTaskWorkflowNextStageFallback(records, current, "continuation planner failed")
}

func dataTaskWorkflowNextStageFallback(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, reasonPrefix string) (dataquery.TaskPlan, string, bool) {
	return dataTaskWorkflowNextStageFallbackWithRepo("", records, current, reasonPrefix)
}

func dataTaskWorkflowNextStageFallbackWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, reasonPrefix string) (dataquery.TaskPlan, string, bool) {
	state := dataTaskWorkflowState(records, current)
	contract := dataTaskWorkflowCoverageContract(records, current)
	output := dataTaskWorkflowOutputContract(records, current)
	access := dataTaskWorkflowArtifactContractAccess(records)
	if len(access) == 0 {
		access = dataTaskWorkflowArtifactAccess(records, 20)
	}
	var latest *dataquery.Result
	if result, ok := latestDataTaskResult(records); ok {
		copyResult := result
		latest = &copyResult
	}
	var gap dataworkflow.ReferenceProjectionGap
	if latest != nil {
		candidate, _, gapDeclared, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, *latest)
		if hasReferenceGap {
			gap = dataworkflow.ReferenceProjectionGap{Candidate: candidate, Present: true, Declared: gapDeclared}
		}
	}
	return dataworkflow.BuildWorkflowNextStageFallbackPlan(dataworkflow.WorkflowNextStageFallbackPlanInput{
		Current:                current,
		Coverage:               contract,
		Output:                 output,
		Facts:                  state.Facts(),
		AllowedNextActions:     state.AllowedNextActions,
		Artifacts:              dataTaskArtifactAccessSchemaProjection(access),
		ExtraScaffolds:         state.ActionScaffold,
		Result:                 latest,
		ReferenceGap:           gap,
		PlanHasCustomTransform: dataTaskPlanHasCustomTransform(current),
		ReasonPrefix:           reasonPrefix,
		SeenActionKeys:         dataTaskWorkflowSeenActionKeys(records),
		ProgressEvents:         dataTaskWorkflowProgressEvents(records),
		NoProgressStop:         DefaultDataTaskMaxNodeFailures,
		ExtractLimit:           dataTaskExactExtractRecordLimit,
	})
}

func dataTaskTerminalWorkflowGuardResult(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataworkflow.GuardResult {
	state := dataTaskWorkflowState(records, current)
	return dataworkflow.TerminalWorkflowGuardResult(current.Status, state.Facts(), state.AllowedNextActions)
}

func dataTaskTerminalWorkflowGuardError(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) string {
	return dataTaskTerminalWorkflowGuardResult(records, current).ErrorText()
}

func dataTaskPlanStatusLooksTerminal(status string) bool {
	return dataworkflow.PlanStatusLooksTerminal(status)
}

func firstNonEmptyOutputContract(values ...dataquery.OutputContract) dataquery.OutputContract {
	return dataworkflow.BestOutputContract(values...)
}

func dataTaskEntityResolutionCompletionInputs(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) []string {
	var candidates []string
	contract := dataTaskWorkflowCoverageContract(records, current)
	for _, p := range contract.RequiredRunnerInputPaths() {
		if dataTaskPathLooksLikeStructuredMaterial(p) {
			candidates = append(candidates, p)
		}
	}
	for _, p := range result.ConsumedPaths {
		if dataTaskPathLooksLikeStructuredMaterial(p) {
			candidates = append(candidates, p)
		}
	}
	for _, artifact := range result.Artifacts {
		for _, p := range artifact.SourcePaths {
			if dataTaskPathLooksLikeStructuredMaterial(p) {
				candidates = append(candidates, p)
			}
		}
	}
	return cleanDataTaskStrings(candidates)
}

func dataTaskPathLooksLikeStructuredMaterial(p string) bool {
	return dataworkflow.PathLooksLikeStructuredMaterial(p)
}

func dataTaskTerminalPlanCompletionGateError(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) string {
	return dataTaskTerminalPlanCompletionGateErrorWithRepo("", records, current)
}

func dataTaskTerminalPlanCompletionGateErrorWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan) string {
	return dataTaskTerminalPlanCompletionGateGuardResultWithRepo(repoRoot, records, current).ErrorText()
}

func dataTaskTerminalPlanCompletionGateGuardResult(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataworkflow.GuardResult {
	return dataTaskTerminalPlanCompletionGateGuardResultWithRepo("", records, current)
}

func dataTaskTerminalPlanCompletionGateGuardResultWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataworkflow.GuardResult {
	status := strings.ToLower(strings.TrimSpace(current.Status))
	if status != "complete" && status != "completed" {
		return dataworkflow.GuardResult{}
	}
	result, ok := latestDataTaskResult(records)
	if !ok {
		return dataworkflow.GuardResult{}
	}
	return dataTaskWorkflowCompletionGateGuardResultWithRepo(repoRoot, records, current, result)
}

func shouldValidateDataTaskWorkflowResult(current dataquery.TaskPlan) bool {
	return !current.ContinueAfter
}

func preserveDataTaskRepairCoverage(previous, repaired dataquery.TaskPlan) dataquery.TaskPlan {
	repaired.CoverageContract = mergeDataTaskCoverageContracts(previous.CoverageContract, repaired.CoverageContract)
	repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredRunnerInputPaths())
	return repaired
}

func preserveDataTaskMaterialRepairCoverage(previous, repaired dataquery.TaskPlan) dataquery.TaskPlan {
	repaired.CoverageContract = mergeDataTaskCoverageContracts(previous.CoverageContract, repaired.CoverageContract)
	repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredRunnerInputPaths())
	return repaired
}

func preserveDataTaskMaterialRepairCoverageForError(previous, repaired dataquery.TaskPlan, errText string) dataquery.TaskPlan {
	violation := dataquery.ClassifyExecutionError(errText)
	if shouldKeepDataTaskRepairCoverageScoped(previous, repaired, violation) {
		repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredRunnerInputPaths())
		repaired.ContinueAfter = true
		return repaired
	}
	if violation.Code == "oversized_data_plan" && repaired.ContinueAfter && (len(repaired.CoverageContract.RequiredMaterials) > 0 || len(repaired.CoverageContract.OptionalMaterials) > 0) {
		repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredRunnerInputPaths())
		return repaired
	}
	return preserveDataTaskMaterialRepairCoverage(previous, repaired)
}

func shouldKeepDataTaskRepairCoverageScoped(previous, repaired dataquery.TaskPlan, violation dataquery.DataTaskViolation) bool {
	if len(repaired.Actions) == 0 {
		return false
	}
	if len(repaired.CoverageContract.RequiredMaterials) == 0 && len(repaired.CoverageContract.OptionalMaterials) == 0 {
		return false
	}
	switch violation.Code {
	case "oversized_data_plan", "oversized_action_plan", "action_top_level_script", "data_action_failed":
		return dataTaskRepairCoverageIsScoped(previous, repaired)
	case "terminal_required_material_not_scheduled", "required_material_not_consumed", "required_material_not_declared", "text_evidence_not_consumed":
		return false
	default:
		return false
	}
}

func dataTaskRepairCoverageIsScoped(previous, repaired dataquery.TaskPlan) bool {
	prevRequired := len(previous.CoverageContract.RequiredMaterials)
	nextRequired := len(repaired.CoverageContract.RequiredMaterials)
	if prevRequired > 0 && nextRequired > 0 && nextRequired < prevRequired {
		return true
	}
	prevLedgers := dataTaskValidationLedgerCount(previous.CoverageContract)
	nextLedgers := dataTaskValidationLedgerCount(repaired.CoverageContract)
	if prevLedgers > 0 && nextLedgers < prevLedgers {
		return true
	}
	prevInputs := len(cleanDataTaskStrings(previous.InputPaths))
	nextInputs := len(cleanDataTaskStrings(repaired.InputPaths))
	if prevInputs > 0 && nextInputs > 0 && nextInputs < prevInputs {
		return true
	}
	for _, action := range repaired.Actions {
		if len(cleanDataTaskStrings(action.InputPaths)) > 0 && prevInputs > 0 && len(cleanDataTaskStrings(action.InputPaths)) < prevInputs {
			return true
		}
	}
	return false
}

func cleanDataTaskStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func parseDataTaskActionStringListParam(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var stringsValue []string
		if err := json.Unmarshal([]byte(raw), &stringsValue); err == nil {
			return cleanDataTaskStrings(stringsValue)
		}
		var anyValue []any
		if err := json.Unmarshal([]byte(raw), &anyValue); err == nil {
			out := make([]string, 0, len(anyValue))
			for _, item := range anyValue {
				if item == nil {
					continue
				}
				out = append(out, strings.TrimSpace(fmt.Sprint(item)))
			}
			return cleanDataTaskStrings(out)
		}
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '|', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	return cleanDataTaskStrings(fields)
}

func mergeDataTaskCoverageContracts(previous, next dataquery.CoverageContract) dataquery.CoverageContract {
	out := next
	out.RequiredMaterials = mergeDataTaskCoverageMaterialsExcept(previous.RequiredMaterials, next.RequiredMaterials, true, dataTaskCoverageOptionalOverrideKeySet(next.OptionalMaterials))
	out.RequiredMaterials = mergeDataTaskCoverageMaterials(out.RequiredMaterials, dataTaskRequiredOptionalCoverageMaterials(next.OptionalMaterials), true)
	out.OptionalMaterials = mergeDataTaskCoverageMaterials(previous.OptionalMaterials, next.OptionalMaterials, false)
	requiredKeys := dataTaskCoverageMaterialKeySet(out.RequiredMaterials)
	out.OptionalMaterials = dataTaskFilterCoverageMaterials(out.OptionalMaterials, func(m dataquery.CoverageMaterial) bool {
		key := dataTaskCoverageMaterialKey(m)
		return key == "" || !requiredKeys[key]
	})
	if len(out.ValidationRules) == 0 && len(previous.ValidationRules) > 0 {
		out.ValidationRules = append([]string(nil), previous.ValidationRules...)
	}
	out.DecisionRecordsRequired = previous.DecisionRecordsRequired || next.DecisionRecordsRequired
	out.RuleCoverageRequired = previous.RuleCoverageRequired || next.RuleCoverageRequired
	out.ContributionLedgerRequired = previous.ContributionLedgerRequired || next.ContributionLedgerRequired
	out.EntityResolutionRequired = previous.EntityResolutionRequired || next.EntityResolutionRequired
	out.ReconcileRequired = previous.ReconcileRequired || next.ReconcileRequired
	return out
}

func dataTaskRequiredOptionalCoverageMaterials(materials []dataquery.CoverageMaterial) []dataquery.CoverageMaterial {
	var out []dataquery.CoverageMaterial
	for _, material := range materials {
		if material.Required && dataTaskCoverageMaterialIsUserExplicitFloor(material) {
			out = append(out, material)
		}
	}
	return out
}

func dataTaskCoverageOptionalOverrideKeySet(materials []dataquery.CoverageMaterial) map[string]bool {
	out := map[string]bool{}
	for _, material := range materials {
		if material.Required {
			continue
		}
		if key := dataTaskCoverageMaterialKey(material); key != "" {
			out[key] = true
		}
	}
	return out
}

func mergeDataTaskCoverageMaterials(previous, next []dataquery.CoverageMaterial, forceRequired bool) []dataquery.CoverageMaterial {
	return mergeDataTaskCoverageMaterialsExcept(previous, next, forceRequired, nil)
}

func mergeDataTaskCoverageMaterialsExcept(previous, next []dataquery.CoverageMaterial, forceRequired bool, skipPreviousKeys map[string]bool) []dataquery.CoverageMaterial {
	out := make([]dataquery.CoverageMaterial, 0, len(previous)+len(next))
	seen := map[string]int{}
	appendOne := func(m dataquery.CoverageMaterial) {
		path := strings.TrimSpace(m.Path)
		id := strings.TrimSpace(m.ID)
		purpose := strings.TrimSpace(m.Purpose)
		if path == "" && id == "" && purpose == "" {
			return
		}
		key := dataTaskCoverageMaterialKey(m)
		if key == "" {
			return
		}
		if idx, ok := seen[key]; ok {
			if forceRequired {
				out[idx].Required = true
			}
			if out[idx].ID == "" && id != "" {
				out[idx].ID = id
			}
			if out[idx].Purpose == "" && purpose != "" {
				out[idx].Purpose = purpose
			}
			if strings.TrimSpace(string(out[idx].UsageMode)) == "" && strings.TrimSpace(string(m.UsageMode)) != "" {
				out[idx].UsageMode = m.UsageMode
			}
			if out[idx].TextEvidencePath == "" && strings.TrimSpace(m.TextEvidencePath) != "" {
				out[idx].TextEvidencePath = strings.TrimSpace(m.TextEvidencePath)
			}
			if len(out[idx].DistilledNotes) == 0 && len(m.DistilledNotes) > 0 {
				out[idx].DistilledNotes = append([]string(nil), m.DistilledNotes...)
			}
			return
		}
		seen[key] = len(out)
		if forceRequired {
			m.Required = true
		}
		out = append(out, m)
	}
	for _, m := range previous {
		if key := dataTaskCoverageMaterialKey(m); key != "" && skipPreviousKeys[key] {
			continue
		}
		appendOne(m)
	}
	for _, m := range next {
		appendOne(m)
	}
	return out
}

func dataTaskCoverageMaterialKeySet(materials []dataquery.CoverageMaterial) map[string]bool {
	out := map[string]bool{}
	for _, material := range materials {
		key := dataTaskCoverageMaterialKey(material)
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func dataTaskCoverageMaterialKey(m dataquery.CoverageMaterial) string {
	if normalized := normalizeDataTaskCoveragePath(m.Path); normalized != "" {
		return "path:" + normalized
	}
	if normalized := normalizeDataTaskCoveragePath(m.TextEvidencePath); normalized != "" {
		return "evidence:" + normalized
	}
	id := strings.TrimSpace(m.ID)
	purpose := strings.TrimSpace(m.Purpose)
	if id == "" && purpose == "" {
		return ""
	}
	return "meta:" + id + "\x00" + purpose
}

func normalizeDataTaskCoveragePath(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	raw = strings.TrimPrefix(raw, "./")
	if raw == "" {
		return ""
	}
	cleaned := path.Clean(raw)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func mergeDataTaskInputPaths(paths, required []string) []string {
	out := make([]string, 0, len(paths)+len(required))
	seen := map[string]bool{}
	for _, list := range [][]string{paths, required} {
		for _, path := range list {
			path = strings.TrimSpace(path)
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	return out
}
