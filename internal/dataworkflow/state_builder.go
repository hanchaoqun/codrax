package dataworkflow

import (
	"path"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type WorkflowStateLedgerCounts struct {
	RuleCoverageRecords     int
	DecisionRecords         int
	EntityResolutionRecords int
	EntityStageMaterialized bool
	ContributionRecords     int
	HasReconcile            bool
	HasAnswer               bool
	HasProjectionArtifact   bool
}

type WorkflowStateDiagnostics struct {
	FieldContractViolations       []FieldContractIssue
	ZeroMatchFilterViolations     []ZeroMatchFilterIssue
	UnmatchedResolutionViolations []UnmatchedResolutionIssue
	ZeroEligibleViolations        []ZeroEligibleIssue
}

type WorkflowStateBoolFunc func(WorkflowStateView) bool

type WorkflowStateViewBuildInput struct {
	Records                       []WorkflowRecord
	Current                       dataquery.TaskPlan
	DeferredQueue                 DeferredQueueState
	WorkflowContract              dataquery.CoverageContract
	CurrentBatchContract          dataquery.CoverageContract
	CurrentBatchLayer             CoverageLayer
	OutputContract                dataquery.OutputContract
	CoveredMaterialPaths          map[string]bool
	HasMaterialProgress           bool
	LedgerCounts                  WorkflowStateLedgerCounts
	CustomTransformFailures       int
	CustomTransformDisabled       bool
	CustomTransformDisabledFunc   WorkflowStateBoolFunc
	ArtifactAvailability          []ArtifactAccessView
	ArtifactAvailabilityCount     int
	ArtifactAvailabilityTruncated bool
	Artifacts                     []ArtifactProjectionSource
	ActionScaffoldArtifacts       []ArtifactSchemaProjection
	ActionScaffoldLimit           int
	DiagnosticArtifactAccess      []ArtifactAccessView
	DiagnosticBatches             []ArtifactDiagnosticBatch
	DiagnosticLimit               int
	ProgressEvents                []ProgressEvent
	GuardViolations               []WorkflowViolation
	AdditionalViolations          []WorkflowViolation
	NoProgressThreshold           int
	LatestEvaluation              *dataquery.Evaluation
	ArtifactLimit                 int
	ProgressLimit                 int
	ActionEventLimit              int
}

func BuildWorkflowStateView(input WorkflowStateViewBuildInput) WorkflowStateView {
	workflowContract := input.WorkflowContract
	currentBatchLayer := input.CurrentBatchLayer
	if currentBatchLayer == "" {
		currentBatchLayer = CoverageLayerCurrentBatch
	}
	required, coveredRequired, missing := MaterialCoverageSets(workflowContract, input.CoveredMaterialPaths)
	materialCoverageSufficient := len(required) > 0 && len(missing) == 0
	if len(required) == 0 && input.HasMaterialProgress {
		materialCoverageSufficient = true
	}
	artifactAvailability := append([]ArtifactAccessView(nil), input.ArtifactAvailability...)
	artifactAvailabilityCount := input.ArtifactAvailabilityCount
	if artifactAvailabilityCount == 0 && len(artifactAvailability) > 0 {
		artifactAvailabilityCount = len(artifactAvailability)
	}
	state := WorkflowStateView{
		MaterialCoverageAuthoritative: true,
		WorkflowContract:              CoverageContractViewFor(CoverageLayerWorkflow, workflowContract),
		CurrentBatchContract:          CoverageContractViewFor(currentBatchLayer, input.CurrentBatchContract),
		RequiredMaterialCount:         len(required),
		RequiredMaterials:             required,
		CoveredRequiredMaterials:      coveredRequired,
		MissingRequiredMaterialCount:  len(missing),
		MissingRequiredMaterials:      missing,
		CoverageNote:                  "required/covered/missing material fields are deterministic workflow truth; compact result.artifact_access and previous_data_rounds are samples, not coverage authority",
		OutputContract:                input.OutputContract,
		MaterialCoverageSufficient:    materialCoverageSufficient,
		RuleCoverageRequired:          workflowContract.RuleCoverageRequired,
		DecisionRecordsRequired:       workflowContract.DecisionRecordsRequired,
		EntityResolutionRequired:      workflowContract.EntityResolutionRequired,
		ContributionLedgerRequired:    workflowContract.ContributionLedgerRequired,
		ReconcileRequired:             workflowContract.ReconcileRequired,
		RuleCoverageRecords:           input.LedgerCounts.RuleCoverageRecords,
		DecisionRecords:               input.LedgerCounts.DecisionRecords,
		EntityResolutionRecords:       input.LedgerCounts.EntityResolutionRecords,
		EntityStageMaterialized:       input.LedgerCounts.EntityStageMaterialized,
		ContributionRecords:           input.LedgerCounts.ContributionRecords,
		HasReconcile:                  input.LedgerCounts.HasReconcile,
		HasAnswer:                     input.LedgerCounts.HasAnswer,
		CustomTransformFailures:       input.CustomTransformFailures,
		CustomTransformDisabled:       input.CustomTransformDisabled,
		ArtifactAvailability:          artifactAvailability,
		ArtifactAvailabilityCount:     artifactAvailabilityCount,
		ArtifactAvailabilityTruncated: input.ArtifactAvailabilityTruncated,
	}
	facts := state.Facts()
	state.NextStage = NextStage(facts)
	if input.CustomTransformDisabledFunc != nil {
		state.CustomTransformDisabled = input.CustomTransformDisabledFunc(state)
	}
	state.AllowedNextActionContracts = AllowedNextActionContractsForFacts(facts)
	if state.CustomTransformDisabled {
		state.CustomTransformDisabledNote = "free-form custom_transform scripts are disabled after workflow/script risk; typed actions remain executable and are the preferred path"
		state.AllowedNextActionContracts = FilterCustomTransformContracts(state.AllowedNextActionContracts)
	}
	state.AllowedNextActions = ActionKindsFromContracts(state.AllowedNextActionContracts)
	diagnostics := BuildWorkflowStateDiagnostics(WorkflowStateDiagnosticsInput{
		Records:           input.Records,
		State:             state,
		ArtifactAccess:    input.DiagnosticArtifactAccess,
		DiagnosticBatches: input.DiagnosticBatches,
		Limit:             input.DiagnosticLimit,
	})
	state.FieldContractViolations = append([]FieldContractIssue(nil), diagnostics.FieldContractViolations...)
	state.ZeroMatchFilterViolations = append([]ZeroMatchFilterIssue(nil), diagnostics.ZeroMatchFilterViolations...)
	state.UnmatchedResolutionViolations = append([]UnmatchedResolutionIssue(nil), diagnostics.UnmatchedResolutionViolations...)
	state.ZeroEligibleViolations = append([]ZeroEligibleIssue(nil), diagnostics.ZeroEligibleViolations...)
	state.ActionScaffold = BuildActionScaffolds(ActionScaffoldBuildInput{
		State:     state,
		Artifacts: append([]ArtifactSchemaProjection(nil), input.ActionScaffoldArtifacts...),
		Limit:     input.ActionScaffoldLimit,
	})
	state.WorkflowViolations = BuildWorkflowStateViolations(WorkflowStateViolationInput{
		Records:             input.Records,
		State:               state,
		GuardViolations:     append([]WorkflowViolation(nil), input.GuardViolations...),
		NoProgressThreshold: input.NoProgressThreshold,
	})
	state.WorkflowViolations = append(state.WorkflowViolations, append([]WorkflowViolation(nil), input.AdditionalViolations...)...)
	state.WorkflowViolationSummary = BuildWorkflowViolationSummary(state.WorkflowViolations)
	outputGraphInput := OutputProjectionGraphInput{
		Output:                    input.OutputContract,
		Coverage:                  workflowContract,
		AnswerPresent:             state.HasAnswer,
		ProjectionArtifactPresent: input.LedgerCounts.HasProjectionArtifact,
		ReconcilePresent:          state.HasReconcile,
		PlanHasCustomTransform:    PlanHasActionKind(input.Current, dataquery.DataActionCustomTransform),
	}
	decisionInput := WorkflowDecisionInput{
		NextStage:          state.NextStage,
		AllowedNextActions: state.AllowedNextActions,
		Violations:         state.WorkflowViolations,
		LedgerGraph:        BuildLedgerGraph(facts),
		OutputGraph:        BuildOutputProjectionGraph(outputGraphInput),
	}
	if input.LatestEvaluation != nil {
		decisionInput.Status = string(input.LatestEvaluation.Status)
		decisionInput.ReasonCode = string(input.LatestEvaluation.Status)
		decisionInput.Reason = input.LatestEvaluation.Reason
	}
	snapshot := BuildWorkflowReducerSnapshot(WorkflowReducerInput{
		Records:                    input.Records,
		Current:                    input.Current,
		DeferredQueue:              input.DeferredQueue,
		StageFacts:                 facts,
		OutputGraph:                outputGraphInput,
		Artifacts:                  input.Artifacts,
		ArtifactLimit:              defaultPositive(input.ArtifactLimit, 48),
		ProgressEvents:             input.ProgressEvents,
		ProgressLimit:              defaultPositive(input.ProgressLimit, 6),
		WorkflowViolations:         state.WorkflowViolations,
		WorkflowViolationSummary:   state.WorkflowViolationSummary,
		Decision:                   decisionInput,
		DecisionFallbackReasonCode: state.NextStage,
		ActionEventLimit:           defaultPositive(input.ActionEventLimit, 48),
	})
	state.ActionGraph = snapshot.ActionGraph
	state.LedgerGraph = snapshot.LedgerGraph
	state.OutputProjectionGraph = snapshot.OutputGraph
	state.ArtifactGraph = snapshot.ArtifactGraph
	state.ProgressSignatures = snapshot.Progress
	state.Decision = snapshot.Decision
	return state
}

func MaterialCoverageSets(contract dataquery.CoverageContract, covered map[string]bool) (required, coveredRequired, missing []string) {
	normalizedCovered := normalizeCoveragePathSet(covered)
	required = cleanStrings(contract.RequiredRunnerInputPaths())
	for _, p := range required {
		if p == "" {
			continue
		}
		if CoveragePathCovered(normalizedCovered, p) {
			coveredRequired = append(coveredRequired, p)
		} else {
			missing = append(missing, p)
		}
	}
	sort.Strings(required)
	sort.Strings(coveredRequired)
	sort.Strings(missing)
	return required, coveredRequired, missing
}

func CoveragePathCovered(covered map[string]bool, required string) bool {
	required = normalizeCoveragePath(required)
	if required == "" {
		return true
	}
	if covered[required] {
		return true
	}
	if path.Ext(required) != "" {
		return false
	}
	prefix := strings.TrimSuffix(required, "/") + "/"
	for value := range covered {
		value = normalizeCoveragePath(value)
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func PlanHasActionKind(plan dataquery.TaskPlan, kind dataquery.DataActionKind) bool {
	for _, action := range plan.Actions {
		if NormalizeActionKind(action.Kind) == kind {
			return true
		}
	}
	return false
}

func normalizeCoveragePathSet(values map[string]bool) map[string]bool {
	out := map[string]bool{}
	for value, ok := range values {
		if !ok {
			continue
		}
		if normalized := normalizeCoveragePath(value); normalized != "" {
			out[normalized] = true
		}
	}
	return out
}

func normalizeCoveragePath(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	raw = strings.TrimPrefix(raw, "./")
	if raw == "" {
		return ""
	}
	cleaned := path.Clean(raw)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func defaultPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
