package dataworkflow

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type WorkflowJournal struct {
	Status                   string                   `json:"status,omitempty"`
	Reason                   string                   `json:"reason,omitempty"`
	DataRounds               int                      `json:"data_rounds,omitempty"`
	RepairRounds             int                      `json:"repair_rounds,omitempty"`
	RecordCount              int                      `json:"record_count,omitempty"`
	ResultSummary            string                   `json:"result_summary,omitempty"`
	LastError                string                   `json:"last_error,omitempty"`
	LastNonterminalError     string                   `json:"last_nonterminal_error,omitempty"`
	PriorErrors              []WorkflowPriorError     `json:"prior_errors,omitempty"`
	ActionEvents             []ActionEvent            `json:"action_events,omitempty"`
	ActionGraph              ActionGraph              `json:"action_graph,omitempty"`
	LedgerGraph              LedgerGraph              `json:"ledger_graph,omitempty"`
	OutputGraph              OutputProjectionGraph    `json:"output_projection_graph,omitempty"`
	ArtifactGraph            ArtifactGraphState       `json:"artifact_graph,omitempty"`
	Progress                 ProgressWindow           `json:"progress,omitempty"`
	WorkflowViolations       []WorkflowViolation      `json:"workflow_violations,omitempty"`
	WorkflowViolationSummary WorkflowViolationSummary `json:"workflow_violation_summary,omitempty"`
	Decision                 WorkflowDecision         `json:"decision,omitempty"`
	ProcessEvents            []WorkflowJournalEvent   `json:"process_events,omitempty"`
	PlanTransitions          []PlanTransitionEvent    `json:"plan_transitions,omitempty"`
	Resume                   *WorkflowResumePayload   `json:"resume,omitempty"`
}

type WorkflowPriorError struct {
	Round       int      `json:"round,omitempty"`
	PlanStatus  string   `json:"plan_status,omitempty"`
	Error       string   `json:"error,omitempty"`
	ActionIDs   []string `json:"action_ids,omitempty"`
	ActionKinds []string `json:"action_kinds,omitempty"`
}

type WorkflowResumePayload struct {
	Records      json.RawMessage `json:"records,omitempty"`
	CurrentPlan  json.RawMessage `json:"current_plan,omitempty"`
	DeferredPlan json.RawMessage `json:"deferred_plan,omitempty"`
}

type WorkflowJournalEvent struct {
	Kind             string                     `json:"kind,omitempty"`
	Round            int                        `json:"round,omitempty"`
	Status           string                     `json:"status,omitempty"`
	Reason           string                     `json:"reason,omitempty"`
	Goal             string                     `json:"goal,omitempty"`
	BatchPurpose     string                     `json:"batch_purpose,omitempty"`
	NextStep         string                     `json:"next_step,omitempty"`
	ActionSummary    string                     `json:"action_summary,omitempty"`
	AuditDetails     []string                   `json:"audit_details,omitempty"`
	Guard            *GuardResult               `json:"guard,omitempty"`
	ViolationSummary WorkflowViolationSummary   `json:"violation_summary,omitempty"`
	Admission        *ActionDAGAdmissionSummary `json:"admission,omitempty"`
	Decision         *WorkflowDecision          `json:"decision,omitempty"`
}

type ActionDAGAdmissionSummary struct {
	Status           string `json:"status,omitempty"`
	Rewritten        bool   `json:"rewritten,omitempty"`
	PlanActions      int    `json:"plan_actions,omitempty"`
	RemainderActions int    `json:"remainder_actions,omitempty"`
	GuardCode        string `json:"guard_code,omitempty"`
	FinalGuardCode   string `json:"final_guard_code,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

type WorkflowJournalBuildInput struct {
	Status                     string
	Reason                     string
	DataRounds                 int
	RepairRounds               int
	ResultSummary              string
	LastError                  string
	Records                    []WorkflowRecord
	CurrentPlan                dataquery.TaskPlan
	DeferredPlan               dataquery.TaskPlan
	State                      WorkflowStateSnapshot
	ActionEvents               []ActionEvent
	ActionGraph                ActionGraph
	LedgerGraph                LedgerGraph
	OutputGraph                OutputProjectionGraph
	ArtifactGraph              ArtifactGraphState
	Progress                   ProgressWindow
	WorkflowViolations         []WorkflowViolation
	WorkflowViolationSummary   WorkflowViolationSummary
	Decision                   WorkflowDecision
	DecisionFallbackReasonCode string
	ProcessEvents              []WorkflowJournalEvent
	Guards                     []GuardResult
	GuardRound                 int
}

func (rt *WorkflowRuntime) BuildJournalSnapshot(input WorkflowJournalBuildInput) WorkflowJournal {
	records := cloneWorkflowRecords(input.Records)
	if input.Records == nil && rt != nil {
		records = rt.Records()
	}
	current := cloneTaskPlanValue(input.CurrentPlan)
	if taskPlanIsZero(current) && rt != nil {
		current = rt.CurrentPlan()
	}
	deferred := cloneTaskPlanValue(input.DeferredPlan)
	if taskPlanIsZero(deferred) && rt != nil {
		deferred = rt.DeferredPlan()
	}
	actionEvents := cloneActionEvents(input.ActionEvents)
	if input.ActionEvents == nil {
		actionEvents = BuildWorkflowActionEvents(records)
	}
	processEvents := cloneWorkflowJournalEvents(input.ProcessEvents)
	if input.ProcessEvents == nil {
		processEvents = runtimeProcessEventsForRecords(rt, records)
	}
	state := input.State
	stateProvided := !state.IsZero()
	if !stateProvided {
		state = WorkflowStateSnapshot{
			ActionGraph:                input.ActionGraph,
			LedgerGraph:                input.LedgerGraph,
			OutputGraph:                input.OutputGraph,
			ArtifactGraph:              input.ArtifactGraph,
			Progress:                   input.Progress,
			WorkflowViolations:         input.WorkflowViolations,
			WorkflowViolationSummary:   input.WorkflowViolationSummary,
			Decision:                   input.Decision,
			DecisionFallbackReasonCode: input.DecisionFallbackReasonCode,
		}
	}
	violations := cloneWorkflowViolations(input.WorkflowViolations)
	if stateProvided || input.WorkflowViolations == nil {
		violations = cloneWorkflowViolations(state.WorkflowViolations)
	}
	violations = appendWorkflowViolationsUnique(violations, workflowViolationsFromProcessEvents(processEvents)...)
	for _, guard := range input.Guards {
		if guard.Empty() {
			continue
		}
		violations = appendWorkflowViolationsUnique(violations, guard.Violations...)
		copied := guard
		processEvents = append(processEvents, BuildWorkflowProcessEvent(WorkflowProcessEventInput{
			Kind:         "guard",
			Round:        input.GuardRound,
			Status:       firstNonEmptyJournalText(guard.Severity, "blocked"),
			Reason:       guard.ErrorText(),
			Plan:         current,
			AuditDetails: cleanJournalStrings([]string{guard.Code}),
			Guard:        &copied,
			Decision:     state.Decision,
		}))
	}
	summary := firstNonEmptyWorkflowViolationSummary(input.WorkflowViolationSummary, state.WorkflowViolationSummary, BuildWorkflowViolationSummary(violations))
	requestedStatus := strings.TrimSpace(input.Status)
	completionSatisfied := WorkflowStateSnapshotCompletionSatisfied(state)
	if completionSatisfied && workflowDecisionStatusLooksComplete(requestedStatus) {
		violations = nil
		summary = WorkflowViolationSummary{}
	}
	preserveBaseStatus := workflowJournalShouldPreserveBaseStatus(state.Decision.Status, requestedStatus) && !completionSatisfied
	lastError := strings.TrimSpace(input.LastError)
	priorErrors := workflowJournalPriorErrors(records, 8)
	lastNonterminalError := ""
	if workflowDecisionStatusLooksComplete(requestedStatus) && !preserveBaseStatus {
		lastNonterminalError = firstNonEmptyJournalText(lastError, latestWorkflowPriorErrorText(priorErrors))
		lastError = ""
	}
	decision := BuildWorkflowJournalDecision(state.Decision, input.Status, input.Reason, lastError, state.DecisionFallbackReasonCode, completionSatisfied)
	// Batch-6 B4: completion scrubbing erased the typed override
	// trail. When the completion gate normalizes a final evaluator
	// verdict (e.g. repair_node -> complete), the override marker
	// "original_status=<verdict>" survives only deep inside
	// resume.records[last].Evaluation — the top-level decision read
	// {status:complete, reason_code:complete} and the terminal log
	// line showed reason="", hiding that the system overrode the
	// model's final typed request. Surface the marker on the
	// top-level decision so the override is auditable at a glance.
	if completionSatisfied && strings.TrimSpace(decision.Reason) == "" {
		if note := latestEvaluationOverrideNote(records); note != "" {
			decision.Reason = note
		}
	}
	status := workflowJournalStatus(input.Status, decision)
	reason := workflowJournalReason(input.Status, input.Reason, decision)
	return WorkflowJournal{
		Status:                   status,
		Reason:                   reason,
		DataRounds:               input.DataRounds,
		RepairRounds:             input.RepairRounds,
		RecordCount:              len(records),
		ResultSummary:            input.ResultSummary,
		LastError:                lastError,
		LastNonterminalError:     lastNonterminalError,
		PriorErrors:              priorErrors,
		ActionEvents:             actionEvents,
		ActionGraph:              state.ActionGraph,
		LedgerGraph:              state.LedgerGraph,
		OutputGraph:              state.OutputGraph,
		ArtifactGraph:            state.ArtifactGraph,
		Progress:                 state.Progress,
		WorkflowViolations:       violations,
		WorkflowViolationSummary: summary,
		Decision:                 decision,
		ProcessEvents:            processEvents,
		PlanTransitions:          rtPlanTransitions(rt),
		Resume:                   BuildWorkflowResumePayload(records, current, deferred),
	}
}

// latestEvaluationOverrideNote returns the completion-gate override
// marker ("original_status=<verdict>" plus the normalization note)
// from the newest record whose evaluation reason carries one; ""
// when no override happened.
func latestEvaluationOverrideNote(records []WorkflowRecord) string {
	for i := len(records) - 1; i >= 0; i-- {
		eval := records[i].Evaluation
		if eval == nil {
			continue
		}
		reason := strings.TrimSpace(string(eval.Reason))
		if idx := strings.Index(reason, "original_status="); idx >= 0 {
			return "completion gate normalized the final evaluator verdict: " + reason[idx:]
		}
	}
	return ""
}

func workflowJournalPriorErrors(records []WorkflowRecord, limit int) []WorkflowPriorError {
	if limit <= 0 {
		limit = 8
	}
	out := make([]WorkflowPriorError, 0)
	for i, rec := range records {
		errText := strings.TrimSpace(rec.Err)
		if errText == "" {
			continue
		}
		item := WorkflowPriorError{
			Round:      i + 1,
			PlanStatus: strings.TrimSpace(rec.Plan.Status),
			Error:      errText,
		}
		for _, action := range rec.Plan.Actions {
			if id := strings.TrimSpace(action.ID); id != "" {
				item.ActionIDs = append(item.ActionIDs, id)
			}
			if kind := strings.TrimSpace(string(action.Kind)); kind != "" {
				item.ActionKinds = append(item.ActionKinds, kind)
			}
		}
		out = append(out, item)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func latestWorkflowPriorErrorText(errors []WorkflowPriorError) string {
	for i := len(errors) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(errors[i].Error); text != "" {
			return text
		}
	}
	return ""
}

func BuildWorkflowJournalDecision(base WorkflowDecision, status, reason, lastErr, fallbackReasonCode string, completionSatisfied bool) WorkflowDecision {
	decision := base
	if text := strings.TrimSpace(status); text != "" {
		if !workflowJournalShouldPreserveBaseStatus(base.Status, text) || completionSatisfied {
			decision.Status = text
		}
	}
	if decision.ReasonCode == "" {
		decision.ReasonCode = firstNonEmptyJournalText(status, fallbackReasonCode)
	}
	if completionSatisfied && workflowDecisionStatusLooksComplete(status) {
		decision.Reason = strings.TrimSpace(reason)
		decision.Violations = nil
		decision.NextActions = nil
		return decision
	}
	if text := firstNonEmptyJournalText(reason, lastErr); text != "" {
		decision.Reason = text
	}
	decision.Violations = append([]string(nil), base.Violations...)
	decision.NextActions = append([]string(nil), base.NextActions...)
	return decision
}

func workflowJournalShouldPreserveBaseStatus(baseStatus, requestedStatus string) bool {
	return workflowDecisionStatusLooksComplete(requestedStatus) &&
		strings.TrimSpace(baseStatus) != "" &&
		!workflowDecisionStatusLooksComplete(baseStatus)
}

func workflowJournalStatus(requested string, decision WorkflowDecision) string {
	status := strings.TrimSpace(requested)
	if workflowJournalShouldPreserveBaseStatus(decision.Status, status) {
		return strings.TrimSpace(decision.Status)
	}
	return status
}

func workflowJournalReason(requestedStatus, requestedReason string, decision WorkflowDecision) string {
	reason := strings.TrimSpace(requestedReason)
	if reason == "" && workflowJournalShouldPreserveBaseStatus(decision.Status, strings.TrimSpace(requestedStatus)) {
		return strings.TrimSpace(decision.Reason)
	}
	// Batch-6 B4: on a clean completion the requested reason is empty;
	// fall through to the decision's reason so a completion-gate
	// override note (original_status=...) reaches the terminal log
	// line instead of reason="".
	if reason == "" {
		return strings.TrimSpace(decision.Reason)
	}
	return reason
}

func BuildWorkflowResumePayload(records []WorkflowRecord, current, deferred dataquery.TaskPlan) *WorkflowResumePayload {
	recordsRaw, recordsErr := json.Marshal(cloneWorkflowRecords(records))
	currentRaw, currentErr := json.Marshal(cloneTaskPlanValue(current))
	deferredRaw, deferredErr := json.Marshal(cloneTaskPlanValue(deferred))
	if recordsErr != nil || currentErr != nil || deferredErr != nil {
		return nil
	}
	return &WorkflowResumePayload{
		Records:      recordsRaw,
		CurrentPlan:  currentRaw,
		DeferredPlan: deferredRaw,
	}
}

func BuildWorkflowActionEvents(records []WorkflowRecord) []ActionEvent {
	events := make([]ActionEvent, 0, len(records))
	for _, rec := range records {
		status := ActionStatusExecuted
		if strings.TrimSpace(rec.Err) != "" {
			status = ActionStatusFailed
		}
		if rec.Admission != nil && strings.TrimSpace(rec.Admission.FinalGuardErr) != "" {
			status = ActionStatusBlocked
		}
		events = append(events, ActionEvent{
			Actions: cloneDataActions(rec.Plan.Actions),
			Status:  status,
		})
	}
	return events
}

func rtPlanTransitions(rt *WorkflowRuntime) []PlanTransitionEvent {
	if rt == nil {
		return nil
	}
	return rt.PlanTransitions()
}

func taskPlanIsZero(plan dataquery.TaskPlan) bool {
	return strings.TrimSpace(plan.Status) == "" &&
		len(plan.InputPaths) == 0 &&
		strings.TrimSpace(plan.Script) == "" &&
		len(plan.Actions) == 0 &&
		len(plan.Questions) == 0 &&
		strings.TrimSpace(plan.Goal) == "" &&
		len(plan.CoverageContract.RequiredMaterials) == 0 &&
		len(plan.CoverageContract.OptionalMaterials) == 0
}

func cloneActionEvents(in []ActionEvent) []ActionEvent {
	out := make([]ActionEvent, 0, len(in))
	for _, event := range in {
		out = append(out, ActionEvent{
			Actions: cloneDataActions(event.Actions),
			Status:  event.Status,
		})
	}
	return out
}

func cloneDataActions(in []dataquery.DataAction) []dataquery.DataAction {
	plan := dataquery.TaskPlan{Actions: in}
	return cloneTaskPlanValue(plan).Actions
}

func cloneWorkflowJournalEvents(in []WorkflowJournalEvent) []WorkflowJournalEvent {
	out := make([]WorkflowJournalEvent, 0, len(in))
	for _, event := range in {
		out = append(out, cloneWorkflowJournalEvent(event))
	}
	return out
}

func cloneWorkflowJournalEvent(event WorkflowJournalEvent) WorkflowJournalEvent {
	copied := event
	copied.AuditDetails = append([]string(nil), event.AuditDetails...)
	if event.Guard != nil {
		guard := cloneGuardResult(*event.Guard)
		copied.Guard = &guard
	}
	copied.ViolationSummary = CloneWorkflowViolationSummary(event.ViolationSummary)
	if event.Admission != nil {
		admission := *event.Admission
		copied.Admission = &admission
	}
	if event.Decision != nil {
		decision := *event.Decision
		decision.Violations = append([]string(nil), event.Decision.Violations...)
		decision.NextActions = append([]string(nil), event.Decision.NextActions...)
		copied.Decision = &decision
	}
	return copied
}

func cloneWorkflowViolations(in []WorkflowViolation) []WorkflowViolation {
	return cloneGuardResult(GuardResult{Violations: in}).Violations
}

func workflowViolationsFromProcessEvents(events []WorkflowJournalEvent) []WorkflowViolation {
	var out []WorkflowViolation
	for _, event := range events {
		if event.Guard == nil || event.Guard.Empty() {
			continue
		}
		out = append(out, event.Guard.Violations...)
	}
	return cloneWorkflowViolations(out)
}

func appendWorkflowViolationsUnique(base []WorkflowViolation, extra ...WorkflowViolation) []WorkflowViolation {
	out := cloneWorkflowViolations(base)
	seen := make(map[string]bool, len(out)+len(extra))
	for _, violation := range out {
		seen[workflowViolationJournalKey(violation)] = true
	}
	for _, violation := range cloneWorkflowViolations(extra) {
		key := workflowViolationJournalKey(violation)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, violation)
	}
	return out
}

func workflowViolationJournalKey(violation WorkflowViolation) string {
	return strings.Join([]string{
		strings.TrimSpace(violation.Code),
		strings.TrimSpace(violation.Severity),
		string(violation.Repairability),
		strings.TrimSpace(violation.ActionID),
		strings.TrimSpace(violation.ActionKind),
		strings.TrimSpace(violation.Param),
		strings.TrimSpace(violation.InputAlias),
		strings.Join(cleanJournalStrings(violation.InputAliases), ","),
		strings.TrimSpace(violation.OutputAlias),
		strings.TrimSpace(violation.IdempotencyKey),
		strings.Join(cleanJournalStrings(violation.MissingFields), ","),
		fmt.Sprintf("%d", violation.Limit),
		fmt.Sprintf("%d", violation.Observed),
		strings.TrimSpace(violation.Reason),
	}, "\x1f")
}

func cleanJournalStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		if text := strings.TrimSpace(item); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func firstNonEmptyJournalText(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}
