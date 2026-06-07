package dataworkflow

import "strings"

type GuardResult struct {
	Code          string                 `json:"code,omitempty"`
	Severity      string                 `json:"severity,omitempty"`
	Repairability ViolationRepairability `json:"repairability,omitempty"`
	Message       string                 `json:"message,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	Violations    []WorkflowViolation    `json:"violations,omitempty"`
}

func NewGuardResult(code, severity string, repairability ViolationRepairability, message string, violations ...WorkflowViolation) GuardResult {
	result := GuardResult{
		Code:          strings.TrimSpace(code),
		Severity:      strings.TrimSpace(severity),
		Repairability: repairability,
		Message:       strings.TrimSpace(message),
		Violations:    append([]WorkflowViolation(nil), violations...),
	}
	if result.Reason == "" {
		result.Reason = firstNonEmptyGuardText(result.Message, result.guardReason())
	}
	return result
}

func (r GuardResult) Empty() bool {
	return strings.TrimSpace(r.Code) == "" &&
		strings.TrimSpace(r.Message) == "" &&
		strings.TrimSpace(r.Reason) == "" &&
		len(r.Violations) == 0
}

func (r GuardResult) ErrorText() string {
	if msg := strings.TrimSpace(r.Message); msg != "" {
		return msg
	}
	return r.guardReason()
}

func (r GuardResult) guardReason() string {
	if reason := strings.TrimSpace(r.Reason); reason != "" {
		return reason
	}
	if msg := strings.TrimSpace(r.Message); msg != "" {
		return msg
	}
	for _, violation := range r.Violations {
		if reason := strings.TrimSpace(violation.Reason); reason != "" {
			return reason
		}
	}
	return ""
}

func firstNonEmptyGuardText(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
