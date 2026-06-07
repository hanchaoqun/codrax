package dataworkflow

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type WorkflowJournal struct {
	Status             string                 `json:"status,omitempty"`
	Reason             string                 `json:"reason,omitempty"`
	DataRounds         int                    `json:"data_rounds,omitempty"`
	RepairRounds       int                    `json:"repair_rounds,omitempty"`
	RecordCount        int                    `json:"record_count,omitempty"`
	ResultSummary      string                 `json:"result_summary,omitempty"`
	LastError          string                 `json:"last_error,omitempty"`
	ActionEvents       []ActionEvent          `json:"action_events,omitempty"`
	ActionGraph        ActionGraph            `json:"action_graph,omitempty"`
	LedgerGraph        LedgerGraph            `json:"ledger_graph,omitempty"`
	OutputGraph        OutputProjectionGraph  `json:"output_projection_graph,omitempty"`
	ArtifactGraph      ArtifactGraphState     `json:"artifact_graph,omitempty"`
	Progress           ProgressWindow         `json:"progress,omitempty"`
	WorkflowViolations []WorkflowViolation    `json:"workflow_violations,omitempty"`
	Decision           WorkflowDecision       `json:"decision,omitempty"`
	ProcessEvents      []WorkflowJournalEvent `json:"process_events,omitempty"`
	PlanTransitions    []PlanTransitionEvent  `json:"plan_transitions,omitempty"`
	Resume             *WorkflowResumePayload `json:"resume,omitempty"`
}

type WorkflowResumePayload struct {
	Records      json.RawMessage `json:"records,omitempty"`
	CurrentPlan  json.RawMessage `json:"current_plan,omitempty"`
	DeferredPlan json.RawMessage `json:"deferred_plan,omitempty"`
}

type WorkflowJournalEvent struct {
	Kind          string                     `json:"kind,omitempty"`
	Round         int                        `json:"round,omitempty"`
	Status        string                     `json:"status,omitempty"`
	Reason        string                     `json:"reason,omitempty"`
	Goal          string                     `json:"goal,omitempty"`
	BatchPurpose  string                     `json:"batch_purpose,omitempty"`
	NextStep      string                     `json:"next_step,omitempty"`
	ActionSummary string                     `json:"action_summary,omitempty"`
	AuditDetails  []string                   `json:"audit_details,omitempty"`
	Guard         *GuardResult               `json:"guard,omitempty"`
	Admission     *ActionDAGAdmissionSummary `json:"admission,omitempty"`
	Decision      *WorkflowDecision          `json:"decision,omitempty"`
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
	ActionEvents               []ActionEvent
	ActionGraph                ActionGraph
	LedgerGraph                LedgerGraph
	OutputGraph                OutputProjectionGraph
	ArtifactGraph              ArtifactGraphState
	Progress                   ProgressWindow
	WorkflowViolations         []WorkflowViolation
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
		processEvents = BuildWorkflowJournalEvents(records)
	}
	violations := cloneWorkflowViolations(input.WorkflowViolations)
	for _, guard := range input.Guards {
		if guard.Empty() {
			continue
		}
		violations = append(violations, cloneWorkflowViolations(guard.Violations)...)
		copied := guard
		processEvents = append(processEvents, BuildWorkflowProcessEvent(WorkflowProcessEventInput{
			Kind:         "guard",
			Round:        input.GuardRound,
			Status:       firstNonEmptyJournalText(guard.Severity, "blocked"),
			Reason:       guard.ErrorText(),
			Plan:         current,
			AuditDetails: cleanJournalStrings([]string{guard.Code}),
			Guard:        &copied,
			Decision:     input.Decision,
		}))
	}
	decision := BuildWorkflowJournalDecision(input.Decision, input.Status, input.Reason, input.LastError, input.DecisionFallbackReasonCode)
	return WorkflowJournal{
		Status:             strings.TrimSpace(input.Status),
		Reason:             strings.TrimSpace(input.Reason),
		DataRounds:         input.DataRounds,
		RepairRounds:       input.RepairRounds,
		RecordCount:        len(records),
		ResultSummary:      input.ResultSummary,
		LastError:          input.LastError,
		ActionEvents:       actionEvents,
		ActionGraph:        input.ActionGraph,
		LedgerGraph:        input.LedgerGraph,
		OutputGraph:        input.OutputGraph,
		ArtifactGraph:      input.ArtifactGraph,
		Progress:           input.Progress,
		WorkflowViolations: violations,
		Decision:           decision,
		ProcessEvents:      processEvents,
		PlanTransitions:    rtPlanTransitions(rt),
		Resume:             BuildWorkflowResumePayload(records, current, deferred),
	}
}

func BuildWorkflowJournalDecision(base WorkflowDecision, status, reason, lastErr, fallbackReasonCode string) WorkflowDecision {
	decision := base
	if text := strings.TrimSpace(status); text != "" {
		decision.Status = text
	}
	if decision.ReasonCode == "" {
		decision.ReasonCode = firstNonEmptyJournalText(status, fallbackReasonCode)
	}
	if text := firstNonEmptyJournalText(reason, lastErr); text != "" {
		decision.Reason = text
	}
	decision.Violations = append([]string(nil), base.Violations...)
	decision.NextActions = append([]string(nil), base.NextActions...)
	return decision
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
		copied := event
		copied.AuditDetails = append([]string(nil), event.AuditDetails...)
		if event.Guard != nil {
			guard := cloneGuardResult(*event.Guard)
			copied.Guard = &guard
		}
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
		out = append(out, copied)
	}
	return out
}

func cloneWorkflowViolations(in []WorkflowViolation) []WorkflowViolation {
	return cloneGuardResult(GuardResult{Violations: in}).Violations
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
