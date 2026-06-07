package dataworkflow

type WorkflowJournal struct {
	Status             string                     `json:"status,omitempty"`
	Reason             string                     `json:"reason,omitempty"`
	DataRounds         int                        `json:"data_rounds,omitempty"`
	RepairRounds       int                        `json:"repair_rounds,omitempty"`
	RecordCount        int                        `json:"record_count,omitempty"`
	ResultSummary      string                     `json:"result_summary,omitempty"`
	LastError          string                     `json:"last_error,omitempty"`
	ActionEvents       []ActionEvent              `json:"action_events,omitempty"`
	ActionGraph        ActionGraph                `json:"action_graph,omitempty"`
	ArtifactGraph      []ArtifactSchemaProjection `json:"artifact_graph,omitempty"`
	WorkflowViolations []WorkflowViolation        `json:"workflow_violations,omitempty"`
	ProcessEvents      []WorkflowJournalEvent     `json:"process_events,omitempty"`
}

type WorkflowJournalEvent struct {
	Kind   string `json:"kind,omitempty"`
	Round  int    `json:"round,omitempty"`
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}
