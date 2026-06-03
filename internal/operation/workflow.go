package operation

import (
	"fmt"
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

// WorkflowEdgeKind describes why one workflow action leads to another.
type WorkflowEdgeKind string

const (
	WorkflowEdgeNext     WorkflowEdgeKind = "next"
	WorkflowEdgeReturn   WorkflowEdgeKind = "return"
	WorkflowEdgeFallback WorkflowEdgeKind = "fallback"
)

// WorkflowActionStatus is the deterministic lifecycle of an operation workflow
// node. It belongs to the operation lane and must not be interpreted as source
// evidence status.
type WorkflowActionStatus string

const (
	WorkflowActionQueued    WorkflowActionStatus = "queued"
	WorkflowActionPending   WorkflowActionStatus = "pending"
	WorkflowActionExecuted  WorkflowActionStatus = "executed"
	WorkflowActionFailed    WorkflowActionStatus = "failed"
	WorkflowActionSkipped   WorkflowActionStatus = "skipped"
	WorkflowActionCancelled WorkflowActionStatus = "cancelled"
)

// WorkflowAction is one node in the operation workflow DAG. The Plan and
// Request fields are resolved by Codrax from provider descriptors; provider
// output can only suggest NextAction/ReturnAction.
type WorkflowAction struct {
	ID             string
	ParentID       string
	SourceProvider string
	NextAction     WorkflowNextAction
	ReturnAction   WorkflowNextAction
	Plan           Plan
	Request        Request
	Input          map[string]any
	WorkflowState  WorkflowState
	Depth          int
	Status         WorkflowActionStatus
	Diagnostics    []string
}

func (a WorkflowAction) Compact() string {
	parts := []string{}
	add := func(label, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, label+"="+value)
		}
	}
	add("id", a.ID)
	add("parent", a.ParentID)
	add("provider", firstNonEmptyWorkflow(a.Plan.Provider, a.NextAction.Provider))
	add("kind", firstNonEmptyWorkflow(a.Plan.Kind, a.Request.OperationKind, a.NextAction.OperationKind, a.NextAction.Operation))
	add("target", firstNonEmptyWorkflow(a.Plan.TargetSurface, a.Request.TargetSurface, a.NextAction.TargetSurface))
	if a.Status != "" {
		add("status", string(a.Status))
	}
	if a.Depth > 0 {
		add("depth", fmt.Sprintf("%d", a.Depth))
	}
	return strings.Join(parts, " ")
}

// WorkflowEdge records the DAG relation between two operation workflow actions.
// V1 schedules serially, but retaining edges lets later fanout scheduling use
// the same provider result contract.
type WorkflowEdge struct {
	From string
	To   string
	Kind WorkflowEdgeKind
}

func (e WorkflowEdge) Compact() string {
	kind := string(e.Kind)
	if kind == "" {
		kind = string(WorkflowEdgeNext)
	}
	return strings.TrimSpace(e.From + " -" + kind + "-> " + e.To)
}

// WorkflowInstance is the operation workflow DAG plus the current serial
// scheduling queue. Queue contains action IDs, not copies; Actions is the
// graph source of truth.
type WorkflowInstance struct {
	ID          string
	RootRequest string
	Actions     []WorkflowAction
	Edges       []WorkflowEdge
	CurrentID   string
	Queue       []string
	Cancelled   bool
	MaxDepth    int
	MaxActions  int
}

func NewWorkflowInstance(id, rootRequest string, maxDepth, maxActions int) WorkflowInstance {
	return WorkflowInstance{
		ID:          strings.TrimSpace(id),
		RootRequest: strings.TrimSpace(rootRequest),
		MaxDepth:    maxDepth,
		MaxActions:  maxActions,
	}
}

func (w *WorkflowInstance) AddAction(parentID string, edgeKind WorkflowEdgeKind, action WorkflowAction) (WorkflowAction, bool, string) {
	if w == nil {
		return WorkflowAction{}, false, "workflow instance is nil"
	}
	if w.MaxActions > 0 && len(w.Actions) >= w.MaxActions {
		return WorkflowAction{}, false, fmt.Sprintf("workflow action limit %d reached", w.MaxActions)
	}
	parentID = strings.TrimSpace(parentID)
	action.ParentID = parentID
	if strings.TrimSpace(action.ID) == "" {
		action.ID = fmt.Sprintf("wf-action-%d", len(w.Actions)+1)
	}
	if action.Status == "" {
		action.Status = WorkflowActionQueued
	}
	if w.MaxDepth > 0 && action.Depth > w.MaxDepth {
		return WorkflowAction{}, false, fmt.Sprintf("workflow depth limit %d reached", w.MaxDepth)
	}
	w.Actions = append(w.Actions, action)
	w.Queue = append(w.Queue, action.ID)
	if parentID != "" {
		if edgeKind == "" {
			edgeKind = WorkflowEdgeNext
		}
		w.Edges = append(w.Edges, WorkflowEdge{From: parentID, To: action.ID, Kind: edgeKind})
	}
	if w.CurrentID == "" {
		w.CurrentID = action.ID
	}
	return action, true, ""
}

func (w WorkflowInstance) ActionByID(id string) (WorkflowAction, bool) {
	id = strings.TrimSpace(id)
	for _, action := range w.Actions {
		if strings.TrimSpace(action.ID) == id {
			return action, true
		}
	}
	return WorkflowAction{}, false
}

func (w WorkflowInstance) CurrentAction() (WorkflowAction, bool) {
	return w.ActionByID(w.CurrentID)
}

func (w WorkflowInstance) Compact() string {
	parts := []string{}
	add := func(label, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, label+"="+value)
		}
	}
	add("workflow_id", w.ID)
	add("current", w.CurrentID)
	if len(w.Queue) > 0 {
		add("queued", fmt.Sprintf("%d", len(w.Queue)))
	}
	if len(w.Actions) > 0 {
		add("actions", fmt.Sprintf("%d", len(w.Actions)))
	}
	if len(w.Edges) > 0 {
		add("edges", fmt.Sprintf("%d", len(w.Edges)))
	}
	if w.Cancelled {
		add("cancelled", "true")
	}
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
