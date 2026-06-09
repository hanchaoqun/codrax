package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func ViolationNodeKey(v dataquery.DataTaskViolation) string {
	id := strings.TrimSpace(v.ActionID)
	if id == "" {
		return ""
	}
	kind := strings.TrimSpace(v.ActionKind)
	return id + "|" + kind
}

func RepeatedNodeFailureFromErrors(previousErrors []string, currentErr string, limit int) (key string, count int, repeated bool) {
	if limit <= 0 {
		limit = 1
	}
	current := dataquery.ClassifyExecutionError(currentErr)
	key = ViolationNodeKey(current)
	if key == "" {
		return "", 0, false
	}
	count = 1
	for _, errText := range previousErrors {
		if strings.TrimSpace(errText) == "" {
			continue
		}
		if ViolationNodeKey(dataquery.ClassifyExecutionError(errText)) == key {
			count++
		}
	}
	return key, count, count >= limit
}

type RepeatedFailureReplacementPlanInput struct {
	Current        dataquery.TaskPlan
	Coverage       dataquery.CoverageContract
	Output         dataquery.OutputContract
	Scaffolds      []ActionScaffold
	Facts          StageFacts
	PreviousErrors []string
	CurrentError   string
	FailureLimit   int
	SeenActionKeys map[string]bool
	ProgressEvents []ProgressEvent
	NoProgressStop int
}

type ExecutionFailureTransitionAction string

const (
	ExecutionFailureFallbackPlan      ExecutionFailureTransitionAction = "fallback_plan"
	ExecutionFailureNeedsContinuation ExecutionFailureTransitionAction = "needs_continuation"
	ExecutionFailureNeedsRepair       ExecutionFailureTransitionAction = "needs_repair"
)

type ExecutionFailureTransitionInput struct {
	Current           dataquery.TaskPlan
	Records           []WorkflowRecord
	FailureRecord     WorkflowRecord
	ErrorText         string
	Violation         dataquery.DataTaskViolation
	Coverage          dataquery.CoverageContract
	Output            dataquery.OutputContract
	State             WorkflowStateView
	SchemaProjections []ArtifactSchemaProjection
	PreviousErrors    []string
	FailureLimit      int
	SeenActionKeys    map[string]bool
	ProgressEvents    []ProgressEvent
	NoProgressStop    int
}

type ExecutionFailureTransition struct {
	Action    ExecutionFailureTransitionAction `json:"action,omitempty"`
	Plan      dataquery.TaskPlan               `json:"plan,omitempty"`
	Reason    string                           `json:"reason,omitempty"`
	NodeKey   string                           `json:"node_key,omitempty"`
	NodeCount int                              `json:"node_count,omitempty"`
}

type FailureRecoveryDecisionAction string

const (
	FailureRecoveryFallbackPlan FailureRecoveryDecisionAction = "fallback_plan"
	FailureRecoveryContinuation FailureRecoveryDecisionAction = "continuation"
	FailureRecoveryRepair       FailureRecoveryDecisionAction = "repair"
	FailureRecoveryFail         FailureRecoveryDecisionAction = "fail"
)

type FailureRecoveryDecision struct {
	Action FailureRecoveryDecisionAction `json:"action,omitempty"`
	Plan   dataquery.TaskPlan            `json:"plan,omitempty"`
	Reason string                        `json:"reason,omitempty"`
	Guard  GuardResult                   `json:"guard,omitempty"`
}

type RepairFailureTransitionAction string

const (
	RepairFailureNeedsContinuation RepairFailureTransitionAction = "needs_continuation"
	RepairFailureNeedsFailure      RepairFailureTransitionAction = "needs_failure"
)

type RepairFailureTransitionInput struct {
	CurrentPlan              dataquery.TaskPlan
	Records                  []WorkflowRecord
	NoStructuredRepairPlan   bool
	ContinuationPlannerReady bool
	RepairFailureReasonCode  string
	RepairFailureDescription string
}

type RepairFailureTransition struct {
	Action     RepairFailureTransitionAction `json:"action,omitempty"`
	Reason     string                        `json:"reason,omitempty"`
	ReasonCode string                        `json:"reason_code,omitempty"`
}

func (t ExecutionFailureTransition) HasPlan() bool {
	return len(t.Plan.Actions) > 0 || strings.TrimSpace(t.Plan.Script) != "" || len(cleanStrings(t.Plan.InputPaths)) > 0
}

func (d FailureRecoveryDecision) HasPlan() bool {
	return len(d.Plan.Actions) > 0 || strings.TrimSpace(d.Plan.Script) != "" || len(cleanStrings(d.Plan.InputPaths)) > 0
}

type PlannerPlanDecisionAction string

const (
	PlannerPlanAccept       PlannerPlanDecisionAction = "accept_plan"
	PlannerPlanFallbackPlan PlannerPlanDecisionAction = "fallback_plan"
	PlannerPlanFail         PlannerPlanDecisionAction = "fail"
)

type PlannerPlanDecisionInput struct {
	Plan              dataquery.TaskPlan `json:"plan,omitempty"`
	ErrorText         string             `json:"error_text,omitempty"`
	FallbackPlan      dataquery.TaskPlan `json:"fallback_plan,omitempty"`
	FallbackReason    string             `json:"fallback_reason,omitempty"`
	FallbackAvailable bool               `json:"fallback_available,omitempty"`
	FailurePrefix     string             `json:"failure_prefix,omitempty"`
}

type PlannerPlanDecision struct {
	Action PlannerPlanDecisionAction `json:"action,omitempty"`
	Plan   dataquery.TaskPlan        `json:"plan,omitempty"`
	Reason string                    `json:"reason,omitempty"`
}

func (d PlannerPlanDecision) HasPlan() bool {
	return len(d.Plan.Actions) > 0 || strings.TrimSpace(d.Plan.Script) != "" || len(cleanStrings(d.Plan.InputPaths)) > 0
}

func DecidePlannerPlanResult(input PlannerPlanDecisionInput) PlannerPlanDecision {
	errText := strings.TrimSpace(input.ErrorText)
	if errText == "" {
		if taskPlanHasExecutableShape(input.Plan) {
			return PlannerPlanDecision{
				Action: PlannerPlanAccept,
				Plan:   cloneTaskPlanValue(input.Plan),
			}
		}
		return PlannerPlanDecision{
			Action: PlannerPlanFail,
			Reason: firstNonEmpty(input.FailurePrefix, "planner returned no executable data plan"),
		}
	}
	if input.FallbackAvailable && taskPlanHasExecutableShape(input.FallbackPlan) {
		return PlannerPlanDecision{
			Action: PlannerPlanFallbackPlan,
			Plan:   cloneTaskPlanValue(input.FallbackPlan),
			Reason: strings.TrimSpace(input.FallbackReason),
		}
	}
	reason := errText
	if prefix := strings.TrimSpace(input.FailurePrefix); prefix != "" {
		reason = prefix + ": " + errText
	}
	return PlannerPlanDecision{
		Action: PlannerPlanFail,
		Reason: reason,
	}
}

func DecideExecutionFailureRecovery(transition ExecutionFailureTransition, continuationReady, repairReady, repairBudgetAvailable bool, fallbackReason string) FailureRecoveryDecision {
	switch transition.Action {
	case ExecutionFailureFallbackPlan:
		if transition.HasPlan() {
			return FailureRecoveryDecision{
				Action: FailureRecoveryFallbackPlan,
				Plan:   cloneTaskPlanValue(transition.Plan),
				Reason: firstNonEmpty(transition.Reason, fallbackReason),
			}
		}
	case ExecutionFailureNeedsContinuation:
		if continuationReady {
			return FailureRecoveryDecision{
				Action: FailureRecoveryContinuation,
				Reason: firstNonEmpty(transition.Reason, fallbackReason),
			}
		}
	}
	if repairReady && repairBudgetAvailable {
		return FailureRecoveryDecision{
			Action: FailureRecoveryRepair,
			Reason: firstNonEmpty(transition.Reason, fallbackReason),
		}
	}
	return FailureRecoveryDecision{
		Action: FailureRecoveryFail,
		Reason: firstNonEmpty(transition.Reason, fallbackReason),
	}
}

func DecideValidationFailureRecovery(transition ValidationFailureTransition, repairReady, repairBudgetAvailable bool, fallbackReason string) FailureRecoveryDecision {
	if transition.Action == ValidationFailureFallbackPlan && transition.HasPlan() {
		return FailureRecoveryDecision{
			Action: FailureRecoveryFallbackPlan,
			Plan:   cloneTaskPlanValue(transition.Plan),
			Reason: firstNonEmpty(transition.Reason, transition.ErrorText, fallbackReason),
			Guard:  transition.Guard,
		}
	}
	if repairReady && repairBudgetAvailable {
		return FailureRecoveryDecision{
			Action: FailureRecoveryRepair,
			Reason: firstNonEmpty(transition.ErrorText, transition.Reason, fallbackReason),
			Guard:  transition.Guard,
		}
	}
	return FailureRecoveryDecision{
		Action: FailureRecoveryFail,
		Reason: firstNonEmpty(transition.ErrorText, transition.Reason, fallbackReason),
		Guard:  transition.Guard,
	}
}

func BuildRepairFailureTransition(input RepairFailureTransitionInput) RepairFailureTransition {
	reasonCode := strings.TrimSpace(input.RepairFailureReasonCode)
	if reasonCode == "" && input.NoStructuredRepairPlan {
		reasonCode = "repair_planner_no_structured_plan"
	}
	reason := strings.TrimSpace(input.RepairFailureDescription)
	if reason == "" && reasonCode != "" {
		reason = reasonCode
	}
	if input.NoStructuredRepairPlan && input.ContinuationPlannerReady && len(input.Records) > 0 {
		return RepairFailureTransition{
			Action:     RepairFailureNeedsContinuation,
			Reason:     firstNonEmpty(reason, "repair planner returned no structured plan; continue from typed workflow state"),
			ReasonCode: reasonCode,
		}
	}
	return RepairFailureTransition{
		Action:     RepairFailureNeedsFailure,
		Reason:     reason,
		ReasonCode: reasonCode,
	}
}

func BuildExecutionFailureTransition(input ExecutionFailureTransitionInput) ExecutionFailureTransition {
	errText := strings.TrimSpace(input.ErrorText)
	if errText == "" {
		errText = strings.TrimSpace(input.FailureRecord.Err)
	}
	recordsWithFailure := append([]WorkflowRecord(nil), input.Records...)
	if strings.TrimSpace(input.FailureRecord.Err) != "" ||
		len(input.FailureRecord.Violations) > 0 ||
		len(input.FailureRecord.Plan.Actions) > 0 ||
		input.FailureRecord.Result != nil {
		recordsWithFailure = append(recordsWithFailure, input.FailureRecord)
	}
	violation := input.Violation
	if strings.TrimSpace(violation.Code) == "" {
		for _, v := range input.FailureRecord.Violations {
			if strings.TrimSpace(v.Code) != "" {
				violation = v
				break
			}
		}
	}
	if strings.TrimSpace(violation.Code) == "" && errText != "" {
		violation = dataquery.ClassifyExecutionError(errText)
	}
	schemas := input.SchemaProjections
	if len(schemas) == 0 {
		schemas = ProjectArtifactSchemasNewestFirst(ArtifactsNewestFirst(recordsWithFailure))
	}
	if plan, ok := MissingJoinFieldFallbackPlan(MissingJoinFieldFallbackInput{
		Current:           input.Current,
		Records:           recordsWithFailure,
		SchemaProjections: schemas,
		Violation:         violation,
	}); ok {
		return ExecutionFailureTransition{
			Action: ExecutionFailureFallbackPlan,
			Plan:   plan,
			Reason: "materialized missing join field from existing artifacts",
		}
	}
	if plan, ok := MissingComputeGroupFieldFallbackPlan(MissingComputeGroupFieldFallbackInput{
		Current:           input.Current,
		Records:           recordsWithFailure,
		SchemaProjections: schemas,
		Violation:         violation,
	}); ok {
		return ExecutionFailureTransition{
			Action: ExecutionFailureFallbackPlan,
			Plan:   plan,
			Reason: "materialized missing contribution group field from existing artifacts",
		}
	}
	if plan, ok := HistoricalMissingJoinFieldFallbackPlan(MissingJoinFieldFallbackInput{
		Current:           input.Current,
		Records:           recordsWithFailure,
		SchemaProjections: schemas,
	}); ok {
		return ExecutionFailureTransition{
			Action: ExecutionFailureFallbackPlan,
			Plan:   plan,
			Reason: "materialized historical missing join field from existing artifacts",
		}
	}
	if plan, ok := HistoricalMissingComputeGroupFieldFallbackPlan(MissingComputeGroupFieldFallbackInput{
		Current:           input.Current,
		Records:           recordsWithFailure,
		SchemaProjections: schemas,
	}); ok {
		return ExecutionFailureTransition{
			Action: ExecutionFailureFallbackPlan,
			Plan:   plan,
			Reason: "materialized historical missing contribution group field from existing artifacts",
		}
	}
	state := input.State
	if len(state.ActionScaffold) > 0 && len(state.AllowedNextActions) > 0 {
		if plan, reason, ok := BuildRepeatedFailureReplacementPlan(RepeatedFailureReplacementPlanInput{
			Current:        input.Current,
			Coverage:       input.Coverage,
			Output:         input.Output,
			Scaffolds:      state.ActionScaffold,
			Facts:          state.Facts(),
			PreviousErrors: input.PreviousErrors,
			CurrentError:   errText,
			FailureLimit:   input.FailureLimit,
			SeenActionKeys: input.SeenActionKeys,
			ProgressEvents: input.ProgressEvents,
			NoProgressStop: input.NoProgressStop,
		}); ok {
			return ExecutionFailureTransition{
				Action: ExecutionFailureFallbackPlan,
				Plan:   plan,
				Reason: reason,
			}
		}
	}
	nodeKey, nodeCount, repeated := RepeatedNodeFailureFromErrors(input.PreviousErrors, errText, input.FailureLimit)
	if repeated {
		return ExecutionFailureTransition{
			Action:    ExecutionFailureNeedsContinuation,
			Reason:    fmt.Sprintf("node %s failed %d times; expanding graph", nodeKey, nodeCount),
			NodeKey:   nodeKey,
			NodeCount: nodeCount,
		}
	}
	return ExecutionFailureTransition{
		Action: ExecutionFailureNeedsRepair,
		Reason: errText,
	}
}

func BuildRepeatedFailureReplacementPlan(input RepeatedFailureReplacementPlanInput) (dataquery.TaskPlan, string, bool) {
	nodeKey, nodeCount, repeated := RepeatedNodeFailureFromErrors(input.PreviousErrors, input.CurrentError, input.FailureLimit)
	if !repeated {
		return dataquery.TaskPlan{}, "", false
	}
	reasonPrefix := fmt.Sprintf("node %s failed %d times", nodeKey, nodeCount)
	plan, reason, ok := BuildConcreteFallbackPlan(ConcreteFallbackPlanInput{
		Current:        input.Current,
		Coverage:       input.Coverage,
		Output:         input.Output,
		Scaffolds:      input.Scaffolds,
		Facts:          input.Facts,
		ReasonPrefix:   reasonPrefix,
		SeenActionKeys: input.SeenActionKeys,
		ProgressEvents: input.ProgressEvents,
		NoProgressStop: input.NoProgressStop,
	})
	if !ok {
		return dataquery.TaskPlan{}, "", false
	}
	if strings.TrimSpace(reason) == "" {
		reason = reasonPrefix + "; replaced repeated node with a concrete typed scaffold"
	}
	return plan, reason, true
}

func RepeatedCustomTransformGuardResult(action dataquery.DataAction, previousErrors []string, limit int) GuardResult {
	if NormalizeActionKind(action.Kind) != dataquery.DataActionCustomTransform {
		return GuardResult{}
	}
	actionID := strings.TrimSpace(action.ID)
	if actionID == "" {
		return GuardResult{}
	}
	if limit <= 0 {
		limit = 1
	}
	key := actionID + "|" + string(dataquery.DataActionCustomTransform)
	count := 0
	for _, errText := range previousErrors {
		if strings.TrimSpace(errText) == "" {
			continue
		}
		if ViolationNodeKey(dataquery.ClassifyExecutionError(errText)) == key {
			count++
		}
	}
	if count < limit {
		return GuardResult{}
	}
	msg := fmt.Sprintf("data planning incomplete: custom_transform node %q already failed %d time(s). Do not retry the same free-form script node; replace it with smaller typed atomic actions such as extract_records, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, and reconcile_artifacts, or emit a new narrow custom_transform over one known artifact.",
		actionID, count)
	return NewGuardResult(
		"repeated_custom_transform_node",
		"error",
		RepairNeedsTypedAction,
		msg,
		WorkflowViolation{
			Code:              "repeated_custom_transform_node",
			Severity:          "error",
			Repairability:     RepairNeedsTypedAction,
			ActionID:          actionID,
			ActionKind:        string(dataquery.DataActionCustomTransform),
			IdempotencyKey:    actionID + "|" + string(dataquery.DataActionCustomTransform),
			RepairActionHints: typedRepairActionHints(),
			Reason:            msg,
		},
	)
}

func CustomTransformFailureClassStats(errorTexts []string) (total int, topCode string, topCodeCount int) {
	counts := map[string]int{}
	for _, errText := range errorTexts {
		if strings.TrimSpace(errText) == "" {
			continue
		}
		total++
		v := dataquery.ClassifyExecutionError(errText)
		code := strings.TrimSpace(v.Code)
		if code == "" {
			code = "unknown_custom_transform_failure"
		}
		counts[code]++
		if counts[code] > topCodeCount {
			topCode = code
			topCodeCount = counts[code]
		}
	}
	return total, topCode, topCodeCount
}

func RepeatedCustomTransformClassGuardResult(action dataquery.DataAction, broadOrWholeWorkflow bool, previousErrors []string, totalLimit, topCodeLimit int) GuardResult {
	if NormalizeActionKind(action.Kind) != dataquery.DataActionCustomTransform || strings.TrimSpace(action.Script) == "" {
		return GuardResult{}
	}
	if !broadOrWholeWorkflow {
		return GuardResult{}
	}
	if totalLimit <= 0 {
		totalLimit = 1
	}
	if topCodeLimit <= 0 {
		topCodeLimit = 1
	}
	total, topCode, topCodeCount := CustomTransformFailureClassStats(previousErrors)
	if total < totalLimit && topCodeCount < topCodeLimit {
		return GuardResult{}
	}
	codeHint := ""
	if topCode != "" {
		codeHint = fmt.Sprintf(" most_common_failure=%s count=%d.", topCode, topCodeCount)
	}
	msg := fmt.Sprintf("data planning incomplete: workflow already has %d custom_transform failure(s).%s Do not bypass this by changing action_id; avoid another broad free-form script. Continue with typed atomic actions that produce reusable artifacts and consume fields from artifact_access.",
		total, codeHint)
	return NewGuardResult(
		"repeated_custom_transform_class",
		"error",
		RepairNeedsTypedAction,
		msg,
		WorkflowViolation{
			Code:              "repeated_custom_transform_class",
			Severity:          "error",
			Repairability:     RepairNeedsTypedAction,
			ActionID:          strings.TrimSpace(action.ID),
			ActionKind:        string(dataquery.DataActionCustomTransform),
			RepairActionHints: typedRepairActionHints(),
			Reason:            msg,
		},
	)
}

func typedRepairActionHints() []string {
	return []string{
		string(dataquery.DataActionExtractRecords),
		string(dataquery.DataActionDeriveRules),
		string(dataquery.DataActionDeriveFields),
		string(dataquery.DataActionNormalizeEntities),
		string(dataquery.DataActionEnrichRecords),
		string(dataquery.DataActionJoinRecords),
		string(dataquery.DataActionComputeContribs),
		string(dataquery.DataActionReconcile),
	}
}
