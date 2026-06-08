package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

const (
	StageCoverRequiredMaterials    = "cover_required_materials"
	StageDeriveRules               = "derive_rules"
	StageNormalizeOrEnrichEntities = "normalize_or_enrich_entities"
	StagePrepareContributionInputs = "prepare_contribution_inputs"
	StageComputeContributions      = "compute_contributions"
	StageReconcileArtifacts        = "reconcile_artifacts"
	StageEmitOutputContractAnswer  = "emit_output_contract_answer"
	StageComplete                  = "complete"
)

// StageFacts is the small, typed reducer input for choosing the next workflow
// stage. It intentionally contains only structural facts, not model prose.
type StageFacts struct {
	MaterialCoverageSufficient bool `json:"material_coverage_sufficient,omitempty"`
	RuleCoverageRequired       bool `json:"rule_coverage_required,omitempty"`
	RuleCoverageRecords        int  `json:"rule_coverage_records,omitempty"`
	DecisionRecordsRequired    bool `json:"decision_records_required,omitempty"`
	DecisionRecords            int  `json:"decision_records,omitempty"`
	EntityResolutionRequired   bool `json:"entity_resolution_required,omitempty"`
	EntityResolutionRecords    int  `json:"entity_resolution_records,omitempty"`
	EntityStageMaterialized    bool `json:"entity_stage_materialized,omitempty"`
	ContributionLedgerRequired bool `json:"contribution_ledger_required,omitempty"`
	ContributionRecords        int  `json:"contribution_records,omitempty"`
	ReconcileRequired          bool `json:"reconcile_required,omitempty"`
	HasReconcile               bool `json:"has_reconcile,omitempty"`
	HasAnswer                  bool `json:"has_answer,omitempty"`
}

type StageFactsInput struct {
	MaterialCoverageSufficient bool
	Coverage                   CoverageContractView
	RuleCoverageRecords        int
	DecisionRecords            int
	EntityResolutionRecords    int
	EntityStageMaterialized    bool
	ContributionRecords        int
	HasReconcile               bool
	HasAnswer                  bool
}

func BuildStageFacts(input StageFactsInput) StageFacts {
	return StageFacts{
		MaterialCoverageSufficient: input.MaterialCoverageSufficient,
		RuleCoverageRequired:       input.Coverage.RuleCoverageRequired,
		RuleCoverageRecords:        input.RuleCoverageRecords,
		DecisionRecordsRequired:    input.Coverage.DecisionRecordsRequired,
		DecisionRecords:            input.DecisionRecords,
		EntityResolutionRequired:   input.Coverage.EntityResolutionRequired,
		EntityResolutionRecords:    input.EntityResolutionRecords,
		EntityStageMaterialized:    input.EntityStageMaterialized,
		ContributionLedgerRequired: input.Coverage.ContributionLedgerRequired,
		ContributionRecords:        input.ContributionRecords,
		ReconcileRequired:          input.Coverage.ReconcileRequired,
		HasReconcile:               input.HasReconcile,
		HasAnswer:                  input.HasAnswer,
	}
}

type ActionContract struct {
	Kind          string `json:"kind"`
	InputBoundary string `json:"input_boundary,omitempty"`
	UseWhen       string `json:"use_when,omitempty"`
	Output        string `json:"output,omitempty"`
}

func NextStage(facts StageFacts) string {
	if !facts.MaterialCoverageSufficient {
		return StageCoverRequiredMaterials
	}
	ruleCoverageMissing := facts.RuleCoverageRequired && facts.RuleCoverageRecords == 0
	if ruleCoverageMissing && !facts.HasPostRuleProgress() {
		return StageDeriveRules
	}
	if facts.EntityResolutionRequired && facts.EntityResolutionRecords == 0 && !facts.EntityStageMaterialized {
		return StageNormalizeOrEnrichEntities
	}
	if facts.ContributionLedgerRequired && facts.ContributionRecords == 0 {
		return StagePrepareContributionInputs
	}
	if facts.ReconcileRequired && !facts.HasReconcile {
		return StageReconcileArtifacts
	}
	if ruleCoverageMissing {
		return StageDeriveRules
	}
	if !facts.HasAnswer {
		if !facts.RuleCoverageRequired && !facts.EntityResolutionRequired && !facts.ContributionLedgerRequired && !facts.ReconcileRequired {
			return StageComputeContributions
		}
		return StageEmitOutputContractAnswer
	}
	return StageComplete
}

func (facts StageFacts) HasPostRuleProgress() bool {
	return facts.EntityStageMaterialized ||
		facts.EntityResolutionRecords > 0 ||
		facts.DecisionRecords > 0 ||
		facts.ContributionRecords > 0 ||
		facts.HasReconcile ||
		facts.HasAnswer
}

func MissingValidationStages(facts StageFacts) []string {
	var out []string
	if facts.RuleCoverageRequired && facts.RuleCoverageRecords == 0 {
		out = append(out, "rule_coverage")
	}
	if facts.EntityResolutionRequired && facts.EntityResolutionRecords == 0 && !facts.EntityStageMaterialized {
		out = append(out, "entity_resolution")
	}
	if facts.DecisionRecordsRequired && facts.DecisionRecords == 0 {
		out = append(out, "decision_records")
	}
	if facts.ContributionLedgerRequired && facts.ContributionRecords == 0 {
		out = append(out, "contribution_ledger")
	}
	if facts.ReconcileRequired && !facts.HasReconcile {
		out = append(out, "reconcile")
	}
	if !facts.HasAnswer {
		out = append(out, "final_answer")
	}
	return out
}

func PlanStatusLooksTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "needs_clarification", "blocked", "complete", "completed", "budget_exhausted", "partial_answer_possible":
		return true
	default:
		return false
	}
}

func TerminalWorkflowGuardResult(status string, facts StageFacts, allowedNextActions []string) GuardResult {
	if !PlanStatusLooksTerminal(status) {
		return GuardResult{}
	}
	missing := MissingValidationStages(facts)
	if len(missing) == 0 || len(allowedNextActions) == 0 {
		return GuardResult{}
	}
	nextStage := facts.NextStage()
	if nextStage == "" {
		nextStage = "data_workflow"
	}
	msg := fmt.Sprintf("data planning incomplete: terminal status=%q is invalid because workflow next_stage=%s still has unfinished validation stage(s): %s and legal next actions [%s]. Emit status=ready with one bounded typed action batch from workflow_state_json.allowed_next_actions; use continue_after=true when later stages remain. blocked is only for true capability/policy barriers, not for ordinary unfinished typed data workflow stages.",
		strings.TrimSpace(status),
		nextStage,
		strings.Join(missing, ", "),
		strings.Join(cleanStrings(allowedNextActions), ", "))
	violation := WorkflowViolation{
		Code:              "unfinished_validation_stage",
		Severity:          "error",
		Repairability:     RepairNeedsTypedAction,
		ActionKind:        nextStage,
		RepairActionHints: cleanStrings(allowedNextActions),
		Reason:            msg,
	}
	return NewGuardResult("unfinished_validation_stage", "error", RepairNeedsTypedAction, msg, violation)
}

type TerminalWorkflowDecisionAction string

const (
	TerminalWorkflowNoop         TerminalWorkflowDecisionAction = ""
	TerminalWorkflowFallbackPlan TerminalWorkflowDecisionAction = "fallback_plan"
	TerminalWorkflowGuard        TerminalWorkflowDecisionAction = "guard"
)

type TerminalWorkflowDecisionInput struct {
	Current                 dataquery.TaskPlan
	Facts                   StageFacts
	AllowedNextActions      []string
	FallbackPlan            dataquery.TaskPlan
	FallbackReason          string
	FallbackAvailable       bool
	SuppressFallbackActions []dataquery.DataActionKind
}

type TerminalWorkflowDecision struct {
	Action TerminalWorkflowDecisionAction `json:"action,omitempty"`
	Plan   dataquery.TaskPlan             `json:"plan,omitempty"`
	Guard  GuardResult                    `json:"guard,omitempty"`
	Reason string                         `json:"reason,omitempty"`
}

func (d TerminalWorkflowDecision) HasPlan() bool {
	return len(d.Plan.Actions) > 0 || strings.TrimSpace(d.Plan.Status) != "" || len(d.Plan.InputPaths) > 0 || strings.TrimSpace(d.Plan.Script) != ""
}

func DecideTerminalWorkflow(input TerminalWorkflowDecisionInput) TerminalWorkflowDecision {
	if !PlanStatusLooksTerminal(input.Current.Status) {
		return TerminalWorkflowDecision{}
	}
	if input.FallbackAvailable && terminalFallbackPlanAllowed(input.FallbackPlan, input.SuppressFallbackActions) {
		return TerminalWorkflowDecision{
			Action: TerminalWorkflowFallbackPlan,
			Plan:   cloneTaskPlanValue(input.FallbackPlan),
			Reason: strings.TrimSpace(input.FallbackReason),
		}
	}
	guard := TerminalWorkflowGuardResult(input.Current.Status, input.Facts, input.AllowedNextActions)
	if !guard.Empty() {
		return TerminalWorkflowDecision{
			Action: TerminalWorkflowGuard,
			Guard:  guard,
			Reason: guard.ErrorText(),
		}
	}
	return TerminalWorkflowDecision{}
}

func terminalFallbackPlanAllowed(plan dataquery.TaskPlan, suppressed []dataquery.DataActionKind) bool {
	if len(plan.Actions) == 0 || len(suppressed) == 0 {
		return true
	}
	first := normalizeStageActionKind(plan.Actions[0].Kind)
	for _, kind := range suppressed {
		if first == normalizeStageActionKind(kind) {
			return false
		}
	}
	return true
}

func normalizeStageActionKind(kind dataquery.DataActionKind) dataquery.DataActionKind {
	return kind
}

type PreExecutionDecisionAction string

const (
	PreExecutionExecute      PreExecutionDecisionAction = "execute"
	PreExecutionFallbackPlan PreExecutionDecisionAction = "fallback_plan"
	PreExecutionGuard        PreExecutionDecisionAction = "guard"
)

type PreExecutionFallbackCandidate struct {
	Source    string             `json:"source,omitempty"`
	Plan      dataquery.TaskPlan `json:"plan,omitempty"`
	Reason    string             `json:"reason,omitempty"`
	Available bool               `json:"available,omitempty"`
}

type PreExecutionDecisionInput struct {
	Fallbacks []PreExecutionFallbackCandidate `json:"fallbacks,omitempty"`
	Guard     GuardResult                     `json:"guard,omitempty"`
}

type PreExecutionDecision struct {
	Action PreExecutionDecisionAction `json:"action,omitempty"`
	Source string                     `json:"source,omitempty"`
	Plan   dataquery.TaskPlan         `json:"plan,omitempty"`
	Guard  GuardResult                `json:"guard,omitempty"`
	Reason string                     `json:"reason,omitempty"`
}

func (d PreExecutionDecision) HasPlan() bool {
	return len(d.Plan.Actions) > 0 || strings.TrimSpace(d.Plan.Status) != "" || len(d.Plan.InputPaths) > 0 || strings.TrimSpace(d.Plan.Script) != ""
}

func DecidePreExecution(input PreExecutionDecisionInput) PreExecutionDecision {
	for _, candidate := range input.Fallbacks {
		if !candidate.Available || !taskPlanHasExecutableShape(candidate.Plan) {
			continue
		}
		return PreExecutionDecision{
			Action: PreExecutionFallbackPlan,
			Source: strings.TrimSpace(candidate.Source),
			Plan:   cloneTaskPlanValue(candidate.Plan),
			Reason: strings.TrimSpace(candidate.Reason),
		}
	}
	if !input.Guard.Empty() {
		return PreExecutionDecision{
			Action: PreExecutionGuard,
			Guard:  input.Guard,
			Reason: input.Guard.ErrorText(),
		}
	}
	return PreExecutionDecision{Action: PreExecutionExecute}
}

func taskPlanHasExecutableShape(plan dataquery.TaskPlan) bool {
	return len(plan.Actions) > 0 ||
		strings.TrimSpace(plan.Script) != "" ||
		len(plan.InputPaths) > 0 ||
		strings.TrimSpace(plan.Status) != ""
}

type GuardRecoveryDecisionAction string

const (
	GuardRecoveryRepair       GuardRecoveryDecisionAction = "repair"
	GuardRecoveryFallbackPlan GuardRecoveryDecisionAction = "fallback_plan"
)

type GuardRecoveryFallbackCandidate struct {
	Source    string             `json:"source,omitempty"`
	Plan      dataquery.TaskPlan `json:"plan,omitempty"`
	Remainder dataquery.TaskPlan `json:"remainder,omitempty"`
	Reason    string             `json:"reason,omitempty"`
	Available bool               `json:"available,omitempty"`
}

type GuardRecoveryDecisionInput struct {
	Guard      GuardResult                      `json:"guard,omitempty"`
	Candidates []GuardRecoveryFallbackCandidate `json:"candidates,omitempty"`
}

type GuardRecoveryDecision struct {
	Action    GuardRecoveryDecisionAction `json:"action,omitempty"`
	Source    string                      `json:"source,omitempty"`
	Plan      dataquery.TaskPlan          `json:"plan,omitempty"`
	Remainder dataquery.TaskPlan          `json:"remainder,omitempty"`
	Guard     GuardResult                 `json:"guard,omitempty"`
	Reason    string                      `json:"reason,omitempty"`
}

func (d GuardRecoveryDecision) HasPlan() bool {
	return taskPlanHasExecutableShape(d.Plan)
}

func (d GuardRecoveryDecision) HasRemainder() bool {
	return taskPlanHasExecutableShape(d.Remainder)
}

func DecideGuardRecovery(input GuardRecoveryDecisionInput) GuardRecoveryDecision {
	for _, candidate := range input.Candidates {
		if !candidate.Available || !taskPlanHasExecutableShape(candidate.Plan) {
			continue
		}
		return GuardRecoveryDecision{
			Action:    GuardRecoveryFallbackPlan,
			Source:    strings.TrimSpace(candidate.Source),
			Plan:      cloneTaskPlanValue(candidate.Plan),
			Remainder: cloneTaskPlanValue(candidate.Remainder),
			Guard:     input.Guard,
			Reason:    strings.TrimSpace(candidate.Reason),
		}
	}
	return GuardRecoveryDecision{
		Action: GuardRecoveryRepair,
		Guard:  input.Guard,
		Reason: input.Guard.ErrorText(),
	}
}

type DataRoundBudgetDecisionAction string

const (
	DataRoundBudgetContinue     DataRoundBudgetDecisionAction = "continue"
	DataRoundBudgetReturnResult DataRoundBudgetDecisionAction = "return_result"
	DataRoundBudgetFail         DataRoundBudgetDecisionAction = "fail"
)

type DataRoundBudgetDecisionInput struct {
	DataRounds      int         `json:"data_rounds,omitempty"`
	MaxDataRounds   int         `json:"max_data_rounds,omitempty"`
	HasResult       bool        `json:"has_result,omitempty"`
	CompletionGuard GuardResult `json:"completion_guard,omitempty"`
}

type DataRoundBudgetDecision struct {
	Action DataRoundBudgetDecisionAction `json:"action,omitempty"`
	Status string                        `json:"status,omitempty"`
	Guard  GuardResult                   `json:"guard,omitempty"`
	Reason string                        `json:"reason,omitempty"`
}

func DecideDataRoundBudget(input DataRoundBudgetDecisionInput) DataRoundBudgetDecision {
	if input.MaxDataRounds <= 0 || input.DataRounds < input.MaxDataRounds {
		return DataRoundBudgetDecision{Action: DataRoundBudgetContinue}
	}
	if !input.CompletionGuard.Empty() {
		reason := "data task workflow budget exhausted before final output: " + input.CompletionGuard.ErrorText()
		return DataRoundBudgetDecision{
			Action: DataRoundBudgetFail,
			Status: "budget_exhausted",
			Guard:  input.CompletionGuard,
			Reason: reason,
		}
	}
	if input.HasResult {
		return DataRoundBudgetDecision{
			Action: DataRoundBudgetReturnResult,
			Status: "budget_exhausted",
			Reason: "data task workflow budget exhausted after producing a result",
		}
	}
	return DataRoundBudgetDecision{
		Action: DataRoundBudgetFail,
		Status: "budget_exhausted",
		Reason: "data task workflow budget exhausted before producing a result",
	}
}

type WorkflowPreRunDecisionAction string

const (
	WorkflowPreRunTerminalWorkflowFallback WorkflowPreRunDecisionAction = "terminal_workflow_fallback"
	WorkflowPreRunTerminalWorkflowGuard    WorkflowPreRunDecisionAction = "terminal_workflow_guard"
	WorkflowPreRunTerminalPlan             WorkflowPreRunDecisionAction = "terminal_plan"
	WorkflowPreRunBudgetFail               WorkflowPreRunDecisionAction = "budget_fail"
	WorkflowPreRunBudgetReturnResult       WorkflowPreRunDecisionAction = "budget_return_result"
	WorkflowPreRunPreExecutionFallback     WorkflowPreRunDecisionAction = "pre_execution_fallback"
	WorkflowPreRunPreExecutionGuard        WorkflowPreRunDecisionAction = "pre_execution_guard"
	WorkflowPreRunExecute                  WorkflowPreRunDecisionAction = "execute"
)

type WorkflowPreRunDecisionInput struct {
	Current          dataquery.TaskPlan       `json:"current,omitempty"`
	HasResult        bool                     `json:"has_result,omitempty"`
	TerminalWorkflow TerminalWorkflowDecision `json:"terminal_workflow,omitempty"`
	Budget           DataRoundBudgetDecision  `json:"budget,omitempty"`
	PreExecution     PreExecutionDecision     `json:"pre_execution,omitempty"`
}

type WorkflowPreRunDecision struct {
	Action         WorkflowPreRunDecisionAction `json:"action,omitempty"`
	Status         string                       `json:"status,omitempty"`
	Source         string                       `json:"source,omitempty"`
	Plan           dataquery.TaskPlan           `json:"plan,omitempty"`
	Guard          GuardResult                  `json:"guard,omitempty"`
	Reason         string                       `json:"reason,omitempty"`
	HasResult      bool                         `json:"has_result,omitempty"`
	TerminalStatus string                       `json:"terminal_status,omitempty"`
}

func (d WorkflowPreRunDecision) HasPlan() bool {
	return taskPlanHasExecutableShape(d.Plan)
}

func DecideWorkflowPreRun(input WorkflowPreRunDecisionInput) WorkflowPreRunDecision {
	switch input.TerminalWorkflow.Action {
	case TerminalWorkflowFallbackPlan:
		return WorkflowPreRunDecision{
			Action: WorkflowPreRunTerminalWorkflowFallback,
			Plan:   cloneTaskPlanValue(input.TerminalWorkflow.Plan),
			Reason: strings.TrimSpace(input.TerminalWorkflow.Reason),
		}
	case TerminalWorkflowGuard:
		return WorkflowPreRunDecision{
			Action: WorkflowPreRunTerminalWorkflowGuard,
			Guard:  input.TerminalWorkflow.Guard,
			Reason: firstNonEmpty(input.TerminalWorkflow.Reason, input.TerminalWorkflow.Guard.ErrorText()),
		}
	}
	status := strings.ToLower(strings.TrimSpace(input.Current.Status))
	if PlanStatusLooksTerminal(status) {
		return WorkflowPreRunDecision{
			Action:         WorkflowPreRunTerminalPlan,
			Status:         strings.TrimSpace(input.Current.Status),
			Reason:         strings.TrimSpace(input.Current.BlockReason),
			HasResult:      input.HasResult,
			TerminalStatus: status,
		}
	}
	switch input.Budget.Action {
	case DataRoundBudgetFail:
		return WorkflowPreRunDecision{
			Action:    WorkflowPreRunBudgetFail,
			Status:    input.Budget.Status,
			Guard:     input.Budget.Guard,
			Reason:    strings.TrimSpace(input.Budget.Reason),
			HasResult: input.HasResult,
		}
	case DataRoundBudgetReturnResult:
		return WorkflowPreRunDecision{
			Action:    WorkflowPreRunBudgetReturnResult,
			Status:    input.Budget.Status,
			Reason:    strings.TrimSpace(input.Budget.Reason),
			HasResult: input.HasResult,
		}
	}
	switch input.PreExecution.Action {
	case PreExecutionFallbackPlan:
		return WorkflowPreRunDecision{
			Action: WorkflowPreRunPreExecutionFallback,
			Source: strings.TrimSpace(input.PreExecution.Source),
			Plan:   cloneTaskPlanValue(input.PreExecution.Plan),
			Reason: strings.TrimSpace(input.PreExecution.Reason),
		}
	case PreExecutionGuard:
		return WorkflowPreRunDecision{
			Action: WorkflowPreRunPreExecutionGuard,
			Guard:  input.PreExecution.Guard,
			Reason: firstNonEmpty(input.PreExecution.Reason, input.PreExecution.Guard.ErrorText()),
		}
	}
	return WorkflowPreRunDecision{Action: WorkflowPreRunExecute}
}

type PostResultDecisionAction string

const (
	PostResultDispatchDeferred PostResultDecisionAction = "dispatch_deferred"
	PostResultUpdateDeferred   PostResultDecisionAction = "update_deferred"
	PostResultFallbackPlan     PostResultDecisionAction = "fallback_plan"
	PostResultEvaluate         PostResultDecisionAction = "evaluate"
)

type PostResultFallbackCandidate struct {
	Source    string             `json:"source,omitempty"`
	Plan      dataquery.TaskPlan `json:"plan,omitempty"`
	Reason    string             `json:"reason,omitempty"`
	Available bool               `json:"available,omitempty"`
}

type PostResultDecisionInput struct {
	DeferredDispatchAvailable bool                          `json:"deferred_dispatch_available,omitempty"`
	DeferredPlan              dataquery.TaskPlan            `json:"deferred_plan,omitempty"`
	DeferredStatus            DeferredDispatchStatus        `json:"deferred_status,omitempty"`
	DeferredRemainder         dataquery.TaskPlan            `json:"deferred_remainder,omitempty"`
	Fallbacks                 []PostResultFallbackCandidate `json:"fallbacks,omitempty"`
}

type PostResultDecision struct {
	Action             PostResultDecisionAction `json:"action,omitempty"`
	Source             string                   `json:"source,omitempty"`
	Plan               dataquery.TaskPlan       `json:"plan,omitempty"`
	Remainder          dataquery.TaskPlan       `json:"remainder,omitempty"`
	DeferredStatus     DeferredDispatchStatus   `json:"deferred_status,omitempty"`
	DeferredPlan       dataquery.TaskPlan       `json:"deferred_plan,omitempty"`
	Reason             string                   `json:"reason,omitempty"`
	LifecycleCandidate bool                     `json:"lifecycle_candidate,omitempty"`
}

func (d PostResultDecision) HasPlan() bool {
	return taskPlanHasExecutableShape(d.Plan)
}

func (d PostResultDecision) HasRemainder() bool {
	return taskPlanHasExecutableShape(d.Remainder)
}

func DecidePostResult(input PostResultDecisionInput) PostResultDecision {
	if input.DeferredDispatchAvailable && taskPlanHasExecutableShape(input.DeferredPlan) {
		return PostResultDecision{
			Action:         PostResultDispatchDeferred,
			Source:         "deferred_dispatch",
			Plan:           cloneTaskPlanValue(input.DeferredPlan),
			Remainder:      cloneTaskPlanValue(input.DeferredRemainder),
			DeferredStatus: input.DeferredStatus,
			Reason:         "continuing deferred typed data action rank",
		}
	}
	if taskPlanHasExecutableShape(input.DeferredPlan) {
		return PostResultDecision{
			Action:             PostResultUpdateDeferred,
			Source:             "deferred_lifecycle",
			DeferredPlan:       cloneTaskPlanValue(input.DeferredPlan),
			DeferredStatus:     input.DeferredStatus,
			Reason:             strings.TrimSpace(input.DeferredStatus.Reason),
			LifecycleCandidate: true,
		}
	}
	for _, candidate := range input.Fallbacks {
		if !candidate.Available || !taskPlanHasExecutableShape(candidate.Plan) {
			continue
		}
		return PostResultDecision{
			Action: PostResultFallbackPlan,
			Source: strings.TrimSpace(candidate.Source),
			Plan:   cloneTaskPlanValue(candidate.Plan),
			Reason: strings.TrimSpace(candidate.Reason),
		}
	}
	return PostResultDecision{Action: PostResultEvaluate}
}

func actionContract(kind dataquery.DataActionKind, boundary, useWhen, output string) ActionContract {
	return ActionContract{
		Kind:          string(kind),
		InputBoundary: boundary,
		UseWhen:       useWhen,
		Output:        output,
	}
}

func deriveRulesActionContract() ActionContract {
	return actionContract(dataquery.DataActionDeriveRules, "rule/constraint/instruction materials or explicit rules_json only", "emit missing rule_coverage records as an audit ledger without resetting already materialized graph progress", "rule coverage artifact and records")
}

func AllowedNextActionContracts(stage string) []ActionContract {
	contract := actionContract
	switch stage {
	case StageCoverRequiredMaterials:
		return []ActionContract{
			contract(dataquery.DataActionMaterialInventory, "many local material paths are OK", "discover objective file metadata before choosing specific inputs", "material inventory artifact"),
			contract(dataquery.DataActionInspectMaterial, "one or more concrete material paths are OK", "profile file shape, headers, samples, or text preview", "inspection artifact"),
			contract(dataquery.DataActionExtractRecords, "one or more structured/text materials are OK", "convert selected materials into bounded generic record samples", "record sample artifact"),
			contract(dataquery.DataActionDeriveRules, "rule/constraint/instruction materials or explicit rules_json", "turn task rules or constraints into rule_coverage records", "rule coverage artifact and records"),
		}
	case StageDeriveRules:
		return []ActionContract{
			contract(dataquery.DataActionDeriveRules, "rule/constraint/instruction materials or explicit rules_json only", "emit rule_coverage records before later decisions, contributions, or reconcile", "rule coverage artifact and records"),
		}
	case StageNormalizeOrEnrichEntities:
		return []ActionContract{
			contract(dataquery.DataActionExtractRecords, "covered source material or generated artifact with output_artifact", "materialize a record artifact needed by normalization, enrichment, join, filtering, or contribution actions; this is record materialization, not repeated material coverage", "one reusable record artifact"),
			contract(dataquery.DataActionDeriveFields, "exactly one existing record artifact/path", "derive same-record fields such as parsed numbers, extracted year, trimmed/lowercase values, constants, regex fields, or maps that do not require a second table", "one enriched record artifact"),
			contract(dataquery.DataActionExtractFields, "one or more same-schema record/text artifacts/paths", "extract structured fields from text/record artifacts using one declared regex/parse spec set before filtering, joining, or contribution calculation", "one merged structured record artifact"),
			contract(dataquery.DataActionGroupRecords, "exactly one existing record artifact/path", "group rows/spans by declared key fields and concatenate text/value fields before extraction, joining, filtering, or contribution calculation", "one grouped record artifact"),
			contract(dataquery.DataActionExpandRecords, "exactly one existing record artifact/path", "expand one multi-value field into multiple records before enrichment or join, for aliases, tags, categories, roles, labels, terms, or other delimited values", "one expanded record artifact"),
			contract(dataquery.DataActionFilterRecords, "exactly one existing record artifact/path", "keep/drop rows using existing fields before enrichment, join, or contribution calculation", "one filtered record artifact"),
			contract(dataquery.DataActionValueDistribution, "exactly one existing record artifact/path", "inspect distinct/top values and empty counts for existing fields before choosing filters, mappings, joins, or contribution params", "value distribution diagnostic artifact"),
			contract(dataquery.DataActionMappingCandidate, "source record artifact plus reference/mapping artifact", "generate domain-neutral source/reference candidate matches, ambiguity counts, and evidence before deciding canonical mappings", "mapping candidate artifact"),
			contract(dataquery.DataActionNormalizeEntities, "one or more inputs are OK when producing source-to-canonical mappings", "produce canonical mappings for identifiers, names, categories, accounts, people, devices, labels, or other entities", "entity_resolution records and mapping artifact"),
			contract(dataquery.DataActionApplyResolutions, "base record artifact plus entity_resolution artifact(s)", "apply existing source-to-canonical mappings back onto base rows before enrichment, join, filtering, or contribution calculation; preserves base rows by default", "one record artifact with canonical id/label/status fields"),
			contract(dataquery.DataActionEnrichRecords, "base record artifact plus mapping/reference artifact(s)", "apply mapping/reference records to base rows and materialize added canonical or derived fields", "one enriched record artifact"),
			contract(dataquery.DataActionJoinRecords, "two structured record artifacts/paths", "join two record sets using explicit key fields before later derivation or contribution calculation; join_type=inner drops unmatched left rows, join_type=left preserves them", "one joined record artifact"),
			contract(dataquery.DataActionCustomTransform, "small bounded fallback over known artifacts only", "use only when typed actions cannot express this single normalization/enrichment step", "one reusable artifact or ledger slice"),
		}
	case StagePrepareContributionInputs:
		return []ActionContract{
			contract(dataquery.DataActionExtractRecords, "covered source material or generated artifact with output_artifact", "materialize a record artifact needed before field derivation, filtering, joining, contribution calculation, or final projection", "one reusable record artifact"),
			contract(dataquery.DataActionDeriveFields, "exactly one existing record artifact/path", "derive numeric, grouping, filtering, or reference fields that are missing before contribution calculation", "one enriched record artifact"),
			contract(dataquery.DataActionExtractFields, "one or more same-schema record/text artifacts/paths", "extract structured fields from text or mixed record fields before filtering, joining, or contribution calculation", "one merged structured record artifact"),
			contract(dataquery.DataActionGroupRecords, "exactly one existing record artifact/path", "group rows/spans by declared keys when one logical record spans multiple source rows before extraction, joining, filtering, or contribution calculation", "one grouped record artifact"),
			contract(dataquery.DataActionExpandRecords, "exactly one existing record artifact/path", "expand one multi-value field into multiple records before enrichment, join, or contribution calculation", "one expanded record artifact"),
			contract(dataquery.DataActionFilterRecords, "exactly one existing record artifact/path", "keep/drop rows with existing filter fields before contribution calculation", "one filtered record artifact"),
			contract(dataquery.DataActionValueDistribution, "exactly one existing record artifact/path", "inspect distinct/top values and empty counts for fields before selecting filters or grouping/contribution params", "value distribution diagnostic artifact"),
			contract(dataquery.DataActionQualifyRecords, "exactly one existing record artifact/path", "execute model-declared record eligibility conditions and emit include/exclude decision records before contribution calculation", "one eligible/annotated record artifact plus decision rows"),
			contract(dataquery.DataActionMappingCandidate, "source record artifact plus reference/mapping artifact", "inspect source/reference candidate matches and ambiguity before materializing canonical mappings or applying reference fields", "mapping candidate artifact"),
			contract(dataquery.DataActionNormalizeEntities, "one or more inputs are OK when producing mappings", "materialize canonical mappings needed by later enrichment, joins, or contribution calculation", "entity_resolution records and mapping artifact"),
			contract(dataquery.DataActionApplyResolutions, "base record artifact plus entity_resolution artifact(s)", "materialize existing canonical mappings as fields on contribution candidate rows; preserves base rows by default", "one record artifact with canonical id/label/status fields"),
			contract(dataquery.DataActionEnrichRecords, "base record artifact plus mapping/reference artifact(s)", "apply mappings or reference fields needed by contribution calculation", "one enriched record artifact"),
			contract(dataquery.DataActionJoinRecords, "two structured record artifacts/paths", "join base records with target/query/reference rows before contribution calculation; use join_type=left if unmatched base rows must remain observable", "one joined record artifact"),
			contract(dataquery.DataActionComputeContribs, "one structured record artifact/path with existing value/group/filter fields", "use only when the input artifact already contains every value, group, and filter field named in params", "contribution records and contribution artifact"),
		}
	case StageComputeContributions:
		return []ActionContract{
			contract(dataquery.DataActionExtractRecords, "covered source material or generated artifact with output_artifact", "materialize a missing record artifact when contribution inputs still exist only as covered material or generated non-record payload", "one reusable record artifact"),
			contract(dataquery.DataActionDeriveFields, "exactly one existing record artifact/path", "derive final numeric/group/filter fields before contribution calculation", "one enriched record artifact"),
			contract(dataquery.DataActionExtractFields, "one or more same-schema record/text artifacts/paths", "extract structured fields from text or mixed record fields before contribution calculation", "one merged structured record artifact"),
			contract(dataquery.DataActionGroupRecords, "exactly one existing record artifact/path", "group rows/spans by declared keys when contribution fields require neighboring source rows or text spans", "one grouped record artifact"),
			contract(dataquery.DataActionExpandRecords, "exactly one existing record artifact/path", "expand one multi-value field into multiple records before enrichment, join, or contribution calculation", "one expanded record artifact"),
			contract(dataquery.DataActionFilterRecords, "exactly one existing record artifact/path", "keep/drop rows with existing filter fields before contribution calculation", "one filtered record artifact"),
			contract(dataquery.DataActionValueDistribution, "exactly one existing record artifact/path", "inspect distinct/top values and empty counts for fields before selecting filters or contribution params", "value distribution diagnostic artifact"),
			contract(dataquery.DataActionQualifyRecords, "exactly one existing record artifact/path", "execute model-declared eligibility conditions when rule/evidence-qualified item decisions are needed before contribution calculation", "one eligible/annotated record artifact plus decision rows"),
			contract(dataquery.DataActionMappingCandidate, "source record artifact plus reference/mapping artifact", "diagnose source/reference mapping coverage when contribution fields depend on a reference set", "mapping candidate artifact"),
			contract(dataquery.DataActionNormalizeEntities, "one or more inputs are OK when producing mappings", "repair missing canonical mappings needed by contribution calculation", "entity_resolution records and mapping artifact"),
			contract(dataquery.DataActionApplyResolutions, "base record artifact plus entity_resolution artifact(s)", "apply existing canonical mappings onto rows before contribution calculation; preserves base rows by default", "one record artifact with canonical id/label/status fields"),
			contract(dataquery.DataActionEnrichRecords, "base record artifact plus mapping/reference artifact(s)", "apply mappings or reference fields needed by contribution calculation", "one enriched record artifact"),
			contract(dataquery.DataActionJoinRecords, "two structured record artifacts/paths", "join base records with target/query/reference rows before contribution calculation; use join_type=left if unmatched base rows must remain observable", "one joined record artifact"),
			contract(dataquery.DataActionComputeContribs, "one structured record artifact/path with existing value/group/filter fields", "compute generic sums, counts, grouped totals, or contribution ledgers", "contribution records and contribution artifact"),
		}
	case StageReconcileArtifacts:
		return []ActionContract{
			contract(dataquery.DataActionReconcile, "prior contribution records are required", "recompute group totals from contribution records and verify expected/actual values", "reconcile report"),
		}
	case StageEmitOutputContractAnswer:
		return []ActionContract{
			contract(dataquery.DataActionReconcile, "prior contribution records are required", "refresh deterministic reconcile before answer assembly when needed", "reconcile report"),
			contract(dataquery.DataActionAssembleAnswer, "prior reconcile report is required", "project reconcile groups into the strict user-facing output contract without changing business decisions or numeric values", "final answer matching output_contract"),
			contract(dataquery.DataActionCustomTransform, "small projection over reconcile/contribution artifacts only", "assemble the strict user-facing output format without changing business decisions or numeric values", "final answer matching output_contract"),
		}
	default:
		return nil
	}
}

func AllowedNextActionContractsForFacts(facts StageFacts) []ActionContract {
	stage := NextStage(facts)
	contracts := AllowedNextActionContracts(stage)
	if facts.RuleCoverageRequired && facts.RuleCoverageRecords == 0 && stage != StageCoverRequiredMaterials && stage != StageDeriveRules {
		contracts = prependUniqueActionContract(contracts, deriveRulesActionContract())
	}
	return contracts
}

func prependUniqueActionContract(contracts []ActionContract, extra ActionContract) []ActionContract {
	if strings.TrimSpace(extra.Kind) == "" {
		return contracts
	}
	for _, contract := range contracts {
		if strings.TrimSpace(contract.Kind) == strings.TrimSpace(extra.Kind) {
			return contracts
		}
	}
	out := make([]ActionContract, 0, len(contracts)+1)
	out = append(out, extra)
	out = append(out, contracts...)
	return out
}

func ActionKindsFromContracts(contracts []ActionContract) []string {
	out := make([]string, 0, len(contracts))
	for _, contract := range contracts {
		if contract.Kind != "" {
			out = append(out, contract.Kind)
		}
	}
	return out
}

func FilterCustomTransformContracts(contracts []ActionContract) []ActionContract {
	out := make([]ActionContract, 0, len(contracts))
	for _, contract := range contracts {
		if contract.Kind == string(dataquery.DataActionCustomTransform) {
			continue
		}
		out = append(out, contract)
	}
	return out
}
