package dataworkflow

import "encoding/json"

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
	ArtifactGraph      ArtifactGraphState     `json:"artifact_graph,omitempty"`
	Progress           ProgressWindow         `json:"progress,omitempty"`
	WorkflowViolations []WorkflowViolation    `json:"workflow_violations,omitempty"`
	Decision           WorkflowDecision       `json:"decision,omitempty"`
	ProcessEvents      []WorkflowJournalEvent `json:"process_events,omitempty"`
	Resume             *WorkflowResumePayload `json:"resume,omitempty"`
}

type WorkflowResumePayload struct {
	Records      json.RawMessage `json:"records,omitempty"`
	CurrentPlan  json.RawMessage `json:"current_plan,omitempty"`
	DeferredPlan json.RawMessage `json:"deferred_plan,omitempty"`
}

type WorkflowJournalEvent struct {
	Kind          string       `json:"kind,omitempty"`
	Round         int          `json:"round,omitempty"`
	Status        string       `json:"status,omitempty"`
	Reason        string       `json:"reason,omitempty"`
	Goal          string       `json:"goal,omitempty"`
	BatchPurpose  string       `json:"batch_purpose,omitempty"`
	NextStep      string       `json:"next_step,omitempty"`
	ActionSummary string       `json:"action_summary,omitempty"`
	AuditDetails  []string     `json:"audit_details,omitempty"`
	Guard         *GuardResult `json:"guard,omitempty"`
}
