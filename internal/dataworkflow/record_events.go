package dataworkflow

import "strings"

func BuildWorkflowJournalEvents(records []WorkflowRecord) []WorkflowJournalEvent {
	events := make([]WorkflowJournalEvent, 0, len(records))
	for i, rec := range records {
		events = append(events, BuildWorkflowJournalEventsForRecord(i+1, rec)...)
	}
	return events
}

func BuildWorkflowJournalEventsForRecord(round int, rec WorkflowRecord) []WorkflowJournalEvent {
	var events []WorkflowJournalEvent
	if rec.Admission != nil {
		events = append(events, BuildAdmissionProcessEvent(round, *rec.Admission))
	}
	kind := "data_batch"
	if len(rec.Plan.Actions) > 0 {
		kind = "action_batch"
	} else if strings.TrimSpace(rec.Plan.Script) != "" {
		kind = "script_batch"
	}
	status := "completed"
	reason := firstNonEmptyProcessText(
		strings.TrimSpace(rec.Plan.WhyThisBatch),
		strings.TrimSpace(rec.Plan.NextBatch),
		strings.TrimSpace(rec.Plan.Goal),
	)
	guard := workflowRecordExecutionGuard(rec)
	if strings.TrimSpace(rec.Err) != "" || guard != nil {
		status = "failed"
		reason = firstNonEmptyProcessText(clampWorkflowRecordText(rec.Err, 240), guardErrorText(guard), reason)
	}
	events = append(events, BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind:   kind,
		Round:  round,
		Status: status,
		Reason: reason,
		Plan:   rec.Plan,
		Result: rec.Result,
		Guard:  guard,
	}))
	return events
}

func workflowRecordExecutionGuard(rec WorkflowRecord) *GuardResult {
	violations := WorkflowViolationsFromRecordExecution([]WorkflowRecord{rec})
	if len(violations) == 0 {
		return nil
	}
	code := "execution_violation"
	if len(violations) == 1 && strings.TrimSpace(violations[0].Code) != "" {
		code = strings.TrimSpace(violations[0].Code)
	}
	repairability := RepairNeedsTypedAction
	for _, violation := range violations {
		if violation.Repairability != "" {
			repairability = violation.Repairability
			break
		}
	}
	message := firstNonEmptyProcessText(clampWorkflowRecordText(rec.Err, 240), violations[0].Reason, code)
	guard := NewGuardResult(code, "error", repairability, message, violations...)
	return &guard
}

func guardErrorText(guard *GuardResult) string {
	if guard == nil {
		return ""
	}
	return guard.ErrorText()
}

func clampWorkflowRecordText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return strings.TrimSpace(text[:limit-3]) + "..."
}
