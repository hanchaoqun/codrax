package operation

import (
	"sort"
	"strings"
)

// WorkflowNextAction is a provider-authored follow-up operation suggestion.
// Codrax treats it as advisory typed data: it must still match a configured
// operation provider and pass the REPL approval flow before execution.
type WorkflowNextAction struct {
	Provider             string         `json:"provider,omitempty"`
	Tool                 string         `json:"tool,omitempty"`
	Operation            string         `json:"operation,omitempty"`
	OperationKind        string         `json:"operation_kind,omitempty"`
	TargetSurface        string         `json:"target_surface,omitempty"`
	RiskLevel            string         `json:"risk_level,omitempty"`
	SideEffects          []string       `json:"side_effects,omitempty"`
	RequiresConfirmation bool           `json:"requires_confirmation,omitempty"`
	Request              string         `json:"request,omitempty"`
	Input                map[string]any `json:"input,omitempty"`
}

func (a WorkflowNextAction) Compact() string {
	parts := []string{}
	add := func(label, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, label+"="+value)
		}
	}
	add("provider", a.Provider)
	add("tool", a.Tool)
	add("kind", firstNonEmptyWorkflow(a.OperationKind, a.Operation))
	add("target", a.TargetSurface)
	add("risk", a.RiskLevel)
	add("request", a.Request)
	return strings.Join(parts, " ")
}

// WorkflowState is compact state a provider can pass across operation-provider
// steps. It is stored as operation artifact state, not source evidence.
type WorkflowState struct {
	WorkflowID string         `json:"workflow_id,omitempty"`
	Step       string         `json:"step,omitempty"`
	ReturnTo   string         `json:"return_to,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

func (s WorkflowState) IsZero() bool {
	return strings.TrimSpace(s.WorkflowID) == "" &&
		strings.TrimSpace(s.Step) == "" &&
		strings.TrimSpace(s.ReturnTo) == "" &&
		len(s.Data) == 0
}

func (s WorkflowState) Compact() string {
	parts := []string{}
	add := func(label, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, label+"="+value)
		}
	}
	add("workflow_id", s.WorkflowID)
	add("step", s.Step)
	add("return_to", s.ReturnTo)
	if len(s.Data) > 0 {
		parts = append(parts, "data_keys="+strings.Join(sortedWorkflowKeys(s.Data), ","))
	}
	return strings.Join(parts, " ")
}

func firstNonEmptyWorkflow(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sortedWorkflowKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
