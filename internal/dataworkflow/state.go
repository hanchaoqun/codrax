package dataworkflow

import "github.com/hanchaoqun/codrax/internal/dataquery"

// WorkflowState is the durable IR boundary for adaptive data tasks. REPL and
// CLI code should render or adapt this state instead of owning workflow rules.
type WorkflowState struct {
	Materials MaterialGraph              `json:"materials,omitempty"`
	Actions   ActionGraph                `json:"actions,omitempty"`
	Ledgers   LedgerGraph                `json:"ledgers,omitempty"`
	Artifacts []ArtifactSchemaProjection `json:"artifacts,omitempty"`
	Progress  WorkflowProgress           `json:"progress,omitempty"`
	Decision  WorkflowDecision           `json:"decision,omitempty"`
}

type MaterialGraph struct {
	WorkflowRequired []MaterialNode `json:"workflow_required,omitempty"`
	CurrentInputs    []MaterialNode `json:"current_inputs,omitempty"`
	OptionalEvidence []MaterialNode `json:"optional_evidence,omitempty"`
	Generated        []MaterialNode `json:"generated,omitempty"`
}

type MaterialNode struct {
	ID        string `json:"id,omitempty"`
	Path      string `json:"path,omitempty"`
	Purpose   string `json:"purpose,omitempty"`
	UseMode   string `json:"use_mode,omitempty"`
	Source    string `json:"source,omitempty"`
	Available bool   `json:"available,omitempty"`
}

type ActionGraph struct {
	Executed []ActionNode `json:"executed,omitempty"`
	Ready    []ActionNode `json:"ready,omitempty"`
	Deferred []ActionNode `json:"deferred,omitempty"`
	Blocked  []ActionNode `json:"blocked,omitempty"`
}

type ActionNode struct {
	ID               string   `json:"id,omitempty"`
	Kind             string   `json:"kind,omitempty"`
	InputAliases     []string `json:"input_aliases,omitempty"`
	OutputAlias      string   `json:"output_alias,omitempty"`
	ProducerStage    string   `json:"producer_stage,omitempty"`
	BlockedReason    string   `json:"blocked_reason,omitempty"`
	CanProduceLedger []string `json:"can_produce_ledger,omitempty"`
}

type LedgerGraph struct {
	RuleCoverage      LedgerStatus `json:"rule_coverage,omitempty"`
	Decisions         LedgerStatus `json:"decisions,omitempty"`
	EntityResolutions LedgerStatus `json:"entity_resolutions,omitempty"`
	Contributions     LedgerStatus `json:"contributions,omitempty"`
	Reconcile         LedgerStatus `json:"reconcile,omitempty"`
	FinalProjection   LedgerStatus `json:"final_projection,omitempty"`
}

type LedgerStatus struct {
	Required bool     `json:"required,omitempty"`
	Present  bool     `json:"present,omitempty"`
	Count    int      `json:"count,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
}

type WorkflowProgress struct {
	NextStage          string   `json:"next_stage,omitempty"`
	AllowedNextActions []string `json:"allowed_next_actions,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
}

type WorkflowDecision struct {
	Status      string   `json:"status,omitempty"`
	ReasonCode  string   `json:"reason_code,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	Violations  []string `json:"violations,omitempty"`
	NextActions []string `json:"next_actions,omitempty"`
}

type ArtifactSchemaProjection struct {
	ID          string   `json:"id,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	JSONShape   string   `json:"json_shape,omitempty"`
	Fields      []string `json:"fields,omitempty"`
	AccessHint  string   `json:"access_hint,omitempty"`
	SourcePaths []string `json:"source_paths,omitempty"`
	RowCount    int      `json:"row_count,omitempty"`
}

// ArtifactProjectionSource is intentionally small: dataquery.DataArtifact is
// the current source, but the workflow IR should stay independent of REPL UI.
type ArtifactProjectionSource = dataquery.DataArtifact
