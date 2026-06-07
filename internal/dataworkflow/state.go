package dataworkflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// WorkflowState is the durable IR boundary for adaptive data tasks. REPL and
// CLI code should render or adapt this state instead of owning workflow rules.
type WorkflowState struct {
	Materials MaterialGraph         `json:"materials,omitempty"`
	Actions   ActionGraph           `json:"actions,omitempty"`
	Ledgers   LedgerGraph           `json:"ledgers,omitempty"`
	Output    OutputProjectionGraph `json:"output,omitempty"`
	Artifacts ArtifactGraphState    `json:"artifacts,omitempty"`
	Progress  WorkflowProgress      `json:"progress,omitempty"`
	Decision  WorkflowDecision      `json:"decision,omitempty"`
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
	Executed      []ActionNode          `json:"executed,omitempty"`
	Ready         []ActionNode          `json:"ready,omitempty"`
	Deferred      []ActionNode          `json:"deferred,omitempty"`
	Blocked       []ActionNode          `json:"blocked,omitempty"`
	DeferredQueue DeferredQueueSnapshot `json:"deferred_queue,omitempty"`
	DeferredPlan  *dataquery.TaskPlan   `json:"deferred_plan,omitempty"`
}

type DeferredQueueSnapshot struct {
	Actions         int    `json:"actions,omitempty"`
	ReadyActions    int    `json:"ready_actions,omitempty"`
	BlockedActions  int    `json:"blocked_actions,omitempty"`
	FirstActionID   string `json:"first_action_id,omitempty"`
	FirstActionKind string `json:"first_action_kind,omitempty"`
	Ready           bool   `json:"ready,omitempty"`
	ReasonCode      string `json:"reason_code,omitempty"`
	Reason          string `json:"reason,omitempty"`
	LifecycleAction string `json:"lifecycle_action,omitempty"`
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
	RuleCoverage      LedgerStatus       `json:"rule_coverage,omitempty"`
	Decisions         LedgerStatus       `json:"decisions,omitempty"`
	EntityResolutions LedgerStatus       `json:"entity_resolutions,omitempty"`
	Contributions     LedgerStatus       `json:"contributions,omitempty"`
	Reconcile         LedgerStatus       `json:"reconcile,omitempty"`
	FinalProjection   LedgerStatus       `json:"final_projection,omitempty"`
	Dependencies      []LedgerDependency `json:"dependencies,omitempty"`
	NextStage         string             `json:"next_stage,omitempty"`
	FirstMissing      string             `json:"first_missing,omitempty"`
}

type LedgerStatus struct {
	Required             bool     `json:"required,omitempty"`
	Present              bool     `json:"present,omitempty"`
	Count                int      `json:"count,omitempty"`
	Status               string   `json:"status,omitempty"`
	Stage                string   `json:"stage,omitempty"`
	ProducesActions      []string `json:"produces_actions,omitempty"`
	DependsOn            []string `json:"depends_on,omitempty"`
	MissingPrerequisites []string `json:"missing_prerequisites,omitempty"`
	Aliases              []string `json:"aliases,omitempty"`
}

type LedgerDependency struct {
	Ledger               string   `json:"ledger,omitempty"`
	Required             bool     `json:"required,omitempty"`
	Present              bool     `json:"present,omitempty"`
	Count                int      `json:"count,omitempty"`
	Status               string   `json:"status,omitempty"`
	Stage                string   `json:"stage,omitempty"`
	ProducesActions      []string `json:"produces_actions,omitempty"`
	DependsOn            []string `json:"depends_on,omitempty"`
	MissingPrerequisites []string `json:"missing_prerequisites,omitempty"`
}

type OutputProjectionGraph struct {
	Required                  bool     `json:"required,omitempty"`
	StrictContract            bool     `json:"strict_contract,omitempty"`
	AnswerPresent             bool     `json:"answer_present,omitempty"`
	ProjectionArtifactPresent bool     `json:"projection_artifact_present,omitempty"`
	ReconcilePresent          bool     `json:"reconcile_present,omitempty"`
	ReconcileGroups           int      `json:"reconcile_groups,omitempty"`
	ReferenceCompleteRequired bool     `json:"reference_complete_required,omitempty"`
	ReferenceComplete         bool     `json:"reference_complete,omitempty"`
	ReferenceKeyCount         int      `json:"reference_key_count,omitempty"`
	AnswerItemCount           int      `json:"answer_item_count,omitempty"`
	Status                    string   `json:"status,omitempty"`
	ReasonCode                string   `json:"reason_code,omitempty"`
	ProducesActions           []string `json:"produces_actions,omitempty"`
	MissingPrerequisites      []string `json:"missing_prerequisites,omitempty"`
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
	LedgerGraph        LedgerGraph
	OutputGraph        OutputProjectionGraph
}

func BuildWorkflowDecision(input WorkflowDecisionInput) WorkflowDecision {
	status := trimStateText(input.Status)
	reasonCode := trimStateText(input.ReasonCode)
	reason := trimStateText(input.Reason)
	nextActions := cleanStrings(input.AllowedNextActions)
	var violationCodes []string
	var violationNextActions []string
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
		if len(violationNextActions) == 0 {
			violationNextActions = cleanStrings(violation.RepairActionHints)
		}
	}
	graphReasonCode, graphReason, graphNextActions := workflowGraphDecisionDetails(input.LedgerGraph, input.OutputGraph)
	if reasonCode == "" {
		reasonCode = graphReasonCode
	}
	if reason == "" {
		reason = graphReason
	}
	switch {
	case len(violationNextActions) > 0:
		nextActions = violationNextActions
	case len(graphNextActions) > 0 && len(violationCodes) == 0:
		nextActions = graphNextActions
	case len(nextActions) == 0:
		nextActions = cleanStrings(input.AllowedNextActions)
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

func workflowGraphDecisionDetails(ledgerGraph LedgerGraph, outputGraph OutputProjectionGraph) (string, string, []string) {
	switch outputGraph.Status {
	case OutputProjectionStatusIncompleteReference, OutputProjectionStatusMissingProjection:
		return "output_" + outputGraph.Status,
			outputProjectionDecisionReason(outputGraph),
			cleanStrings(outputGraph.ProducesActions)
	}
	if dep, ok := FirstIncompleteRequiredLedger(ledgerGraph); ok {
		code := workflowLedgerDecisionCode(dep)
		reason := ledgerCompletionMessage(dep)
		return code, reason, workflowLedgerDecisionNextActions(dep)
	}
	switch outputGraph.Status {
	case OutputProjectionStatusMissingAnswer:
		return "output_" + outputGraph.Status,
			outputProjectionDecisionReason(outputGraph),
			nil
	default:
		return "", "", nil
	}
}

func workflowLedgerDecisionNextActions(dep LedgerDependency) []string {
	if dep.Status == LedgerStatusBlockedByPrerequisite || len(dep.MissingPrerequisites) > 0 {
		return nil
	}
	return cleanStrings(dep.ProducesActions)
}

func workflowLedgerDecisionCode(dep LedgerDependency) string {
	status := strings.TrimSpace(dep.Status)
	switch status {
	case LedgerStatusMissing:
		status = "missing"
	case LedgerStatusBlockedByPrerequisite:
		status = "blocked"
	case "":
		status = "incomplete"
	}
	ledger := strings.TrimSpace(dep.Ledger)
	if ledger == "" {
		ledger = "unknown"
	}
	return "ledger_" + status + "_" + strings.ReplaceAll(ledger, "/", "_")
}

func outputProjectionDecisionReason(graph OutputProjectionGraph) string {
	switch graph.Status {
	case OutputProjectionStatusIncompleteReference:
		return "output projection is incomplete for the declared reference key universe"
	case OutputProjectionStatusMissingProjection:
		return "strict output contract requires final answer projection"
	case OutputProjectionStatusMissingAnswer:
		return "workflow has not produced a final answer yet"
	default:
		return ""
	}
}

func trimStateText(value string) string {
	return strings.TrimSpace(value)
}

type ArtifactSchemaProjection struct {
	ID                string            `json:"id,omitempty"`
	Kind              string            `json:"kind,omitempty"`
	NodeClass         string            `json:"node_class,omitempty"`
	Aliases           []string          `json:"aliases,omitempty"`
	JSONShape         string            `json:"json_shape,omitempty"`
	Fields            []string          `json:"fields,omitempty"`
	Diagnostics       map[string]string `json:"diagnostics,omitempty"`
	AccessHint        string            `json:"access_hint,omitempty"`
	SourcePaths       []string          `json:"source_paths,omitempty"`
	SourceRecordPaths []string          `json:"source_record_paths,omitempty"`
	ReferencePaths    []string          `json:"reference_paths,omitempty"`
	EvidencePaths     []string          `json:"evidence_paths,omitempty"`
	RowCount          int               `json:"row_count,omitempty"`
}

type ArtifactGraphState struct {
	Nodes                   []ArtifactGraphNode    `json:"nodes,omitempty"`
	NodeCount               int                    `json:"node_count,omitempty"`
	Truncated               bool                   `json:"truncated,omitempty"`
	AliasIndex              []ArtifactAliasBinding `json:"alias_index,omitempty"`
	ExecutableRecordAliases []string               `json:"executable_record_aliases,omitempty"`
}

type ArtifactGraphNode struct {
	ID                    string            `json:"id,omitempty"`
	Kind                  string            `json:"kind,omitempty"`
	ProducerKind          string            `json:"producer_kind,omitempty"`
	NodeClass             string            `json:"node_class,omitempty"`
	PrimaryAlias          string            `json:"primary_alias,omitempty"`
	Aliases               []string          `json:"aliases,omitempty"`
	JSONShape             string            `json:"json_shape,omitempty"`
	Fields                []string          `json:"fields,omitempty"`
	Diagnostics           map[string]string `json:"diagnostics,omitempty"`
	AccessHint            string            `json:"access_hint,omitempty"`
	RowCount              int               `json:"row_count,omitempty"`
	ExecutableRecordInput bool              `json:"executable_record_input,omitempty"`
	Lineage               ArtifactLineage   `json:"lineage,omitempty"`
}

type ArtifactLineage struct {
	SourcePaths       []string `json:"source_paths,omitempty"`
	SourceRecordPaths []string `json:"source_record_paths,omitempty"`
	ReferencePaths    []string `json:"reference_paths,omitempty"`
	EvidencePaths     []string `json:"evidence_paths,omitempty"`
}

type ArtifactAliasBinding struct {
	Alias                 string `json:"alias,omitempty"`
	NodeID                string `json:"node_id,omitempty"`
	NodeClass             string `json:"node_class,omitempty"`
	ExecutableRecordInput bool   `json:"executable_record_input,omitempty"`
}

// ArtifactProjectionSource is intentionally small: dataquery.DataArtifact is
// the current source, but the workflow IR should stay independent of REPL UI.
type ArtifactProjectionSource = dataquery.DataArtifact
