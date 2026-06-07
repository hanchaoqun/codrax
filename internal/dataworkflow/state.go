package dataworkflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

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
	Status           string   `json:"status,omitempty"`
	DependencyRank   int      `json:"dependency_rank,omitempty"`
	IdempotencyKey   string   `json:"idempotency_key,omitempty"`
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

type WorkflowDecisionInput struct {
	Status             string
	ReasonCode         string
	Reason             string
	NextStage          string
	AllowedNextActions []string
	Violations         []WorkflowViolation
}

func BuildWorkflowDecision(input WorkflowDecisionInput) WorkflowDecision {
	status := trimStateText(input.Status)
	reasonCode := trimStateText(input.ReasonCode)
	reason := trimStateText(input.Reason)
	nextActions := cleanStrings(input.AllowedNextActions)
	var violationCodes []string
	for _, violation := range input.Violations {
		if code := trimStateText(violation.Code); code != "" {
			violationCodes = append(violationCodes, code)
		}
		if reasonCode == "" {
			reasonCode = trimStateText(violation.Code)
		}
		if reason == "" {
			reason = trimStateText(violation.Reason)
		}
		if len(nextActions) == 0 {
			nextActions = cleanStrings(violation.RepairActionHints)
		}
	}
	if status == "" {
		switch {
		case len(violationCodes) > 0:
			status = "blocked"
		case trimStateText(input.NextStage) == StageComplete:
			status = "complete"
		default:
			status = "continue"
		}
	}
	if reasonCode == "" {
		reasonCode = trimStateText(input.NextStage)
	}
	if reason == "" {
		switch status {
		case "complete":
			reason = "workflow validation stages are complete"
		case "blocked":
			reason = "workflow has typed violations that need repair"
		default:
			reason = "workflow has remaining typed stages"
		}
	}
	return WorkflowDecision{
		Status:      status,
		ReasonCode:  reasonCode,
		Reason:      reason,
		Violations:  cleanStrings(violationCodes),
		NextActions: nextActions,
	}
}

func trimStateText(value string) string {
	return strings.TrimSpace(value)
}

type ArtifactSchemaProjection struct {
	ID                string   `json:"id,omitempty"`
	Kind              string   `json:"kind,omitempty"`
	NodeClass         string   `json:"node_class,omitempty"`
	Aliases           []string `json:"aliases,omitempty"`
	JSONShape         string   `json:"json_shape,omitempty"`
	Fields            []string `json:"fields,omitempty"`
	AccessHint        string   `json:"access_hint,omitempty"`
	SourcePaths       []string `json:"source_paths,omitempty"`
	SourceRecordPaths []string `json:"source_record_paths,omitempty"`
	ReferencePaths    []string `json:"reference_paths,omitempty"`
	EvidencePaths     []string `json:"evidence_paths,omitempty"`
	RowCount          int      `json:"row_count,omitempty"`
}

// ArtifactProjectionSource is intentionally small: dataquery.DataArtifact is
// the current source, but the workflow IR should stay independent of REPL UI.
type ArtifactProjectionSource = dataquery.DataArtifact
