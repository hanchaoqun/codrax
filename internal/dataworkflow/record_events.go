package dataworkflow

import "strings"

func BuildWorkflowJournalEvents(records []WorkflowRecord) []WorkflowJournalEvent {
	events := make([]WorkflowJournalEvent, 0, len(records))
	for i, rec := range records {
		if rec.Admission != nil {
			events = append(events, BuildAdmissionProcessEvent(i+1, *rec.Admission))
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
		if strings.TrimSpace(rec.Err) != "" {
			status = "failed"
			reason = firstNonEmptyProcessText(clampWorkflowRecordText(rec.Err, 240), reason)
		}
		events = append(events, BuildWorkflowProcessEvent(WorkflowProcessEventInput{
			Kind:   kind,
			Round:  i + 1,
			Status: status,
			Reason: reason,
			Plan:   rec.Plan,
			Result: rec.Result,
		}))
	}
	return events
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
