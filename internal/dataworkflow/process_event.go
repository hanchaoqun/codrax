package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type WorkflowProcessEventInput struct {
	Kind         string
	Round        int
	Status       string
	Reason       string
	Plan         dataquery.TaskPlan
	Result       *dataquery.Result
	AuditDetails []string
	Guard        *GuardResult
}

func BuildWorkflowProcessEvent(input WorkflowProcessEventInput) WorkflowJournalEvent {
	plan := input.Plan
	event := WorkflowJournalEvent{
		Kind:          strings.TrimSpace(input.Kind),
		Round:         input.Round,
		Status:        strings.TrimSpace(input.Status),
		Reason:        firstNonEmptyProcessText(input.Reason, plan.WhyThisBatch, plan.NextBatch, plan.Goal),
		Goal:          strings.TrimSpace(plan.Goal),
		BatchPurpose:  strings.TrimSpace(plan.WhyThisBatch),
		NextStep:      strings.TrimSpace(plan.NextBatch),
		ActionSummary: ActionIntentSummary(plan.Actions, 3),
		AuditDetails:  cleanStrings(input.AuditDetails),
	}
	if input.Result != nil {
		event.AuditDetails = append(event.AuditDetails, ResultAuditDetails(*input.Result)...)
		event.AuditDetails = cleanStrings(event.AuditDetails)
	}
	if input.Guard != nil && !input.Guard.Empty() {
		copied := *input.Guard
		event.Guard = &copied
		if event.Status == "" {
			event.Status = firstNonEmptyProcessText(input.Guard.Severity, "blocked")
		}
		if event.Reason == "" {
			event.Reason = input.Guard.ErrorText()
		}
	}
	return event
}

func ActionIntentSummary(actions []dataquery.DataAction, limit int) string {
	actions = normalizeProcessActions(actions)
	if limit <= 0 || limit > len(actions) {
		limit = len(actions)
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		action := actions[i]
		text := strings.TrimSpace(action.Purpose)
		if text == "" {
			text = strings.TrimSpace(string(NormalizeActionKind(action.Kind)))
		}
		if text == "" {
			text = strings.TrimSpace(action.ID)
		}
		if text == "" {
			continue
		}
		parts = append(parts, clampProcessText(text, 120))
	}
	if len(actions) > limit {
		parts = append(parts, fmt.Sprintf("+%d", len(actions)-limit))
	}
	return strings.Join(parts, " -> ")
}

func ResultAuditDetails(result dataquery.Result) []string {
	var details []string
	if len(result.Rows) > 0 {
		details = append(details, fmt.Sprintf("decision_records=%d", len(result.Rows)))
	}
	if len(result.ConsumedPaths) > 0 {
		details = append(details, fmt.Sprintf("consumed_materials=%d", len(result.ConsumedPaths)))
	}
	if len(result.RuleCoverage) > 0 {
		details = append(details, fmt.Sprintf("rule_coverage=%d", len(result.RuleCoverage)))
	}
	if len(result.Contributions) > 0 {
		details = append(details, fmt.Sprintf("contributions=%d", len(result.Contributions)))
	}
	if len(result.EntityResolutions) > 0 {
		details = append(details, fmt.Sprintf("entity_resolutions=%d", len(result.EntityResolutions)))
	}
	if result.Reconcile != nil {
		if status := strings.TrimSpace(result.Reconcile.Status.String()); status != "" {
			details = append(details, "reconcile="+status)
		}
	}
	return details
}

func normalizeProcessActions(actions []dataquery.DataAction) []dataquery.DataAction {
	out := make([]dataquery.DataAction, 0, len(actions))
	for _, action := range actions {
		action, _ = NormalizeRolePathAction(action)
		if strings.TrimSpace(action.ID) == "" && strings.TrimSpace(string(action.Kind)) == "" && strings.TrimSpace(action.Purpose) == "" {
			continue
		}
		out = append(out, action)
	}
	return out
}

func firstNonEmptyProcessText(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func clampProcessText(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 || len([]rune(text)) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}
