package dataworkflow

// WorkflowStateSnapshot is the package-neutral graph snapshot used by journals
// and reducers. It carries structural workflow state only; prompt-only samples
// and UI adapter fields stay outside this IR.
type WorkflowStateSnapshot struct {
	StageFacts                 StageFacts            `json:"stage_facts,omitempty"`
	ActionGraph                ActionGraph           `json:"action_graph,omitempty"`
	LedgerGraph                LedgerGraph           `json:"ledger_graph,omitempty"`
	OutputGraph                OutputProjectionGraph `json:"output_projection_graph,omitempty"`
	ArtifactGraph              ArtifactGraphState    `json:"artifact_graph,omitempty"`
	Progress                   ProgressWindow        `json:"progress,omitempty"`
	WorkflowViolations         []WorkflowViolation   `json:"workflow_violations,omitempty"`
	Decision                   WorkflowDecision      `json:"decision,omitempty"`
	DecisionFallbackReasonCode string                `json:"decision_fallback_reason_code,omitempty"`
}

type WorkflowStateSnapshotInput struct {
	StageFacts                 StageFacts
	ActionGraph                ActionGraphInput
	OutputGraph                OutputProjectionGraphInput
	Artifacts                  []ArtifactProjectionSource
	ArtifactLimit              int
	ProgressEvents             []ProgressEvent
	ProgressLimit              int
	WorkflowViolations         []WorkflowViolation
	Decision                   WorkflowDecisionInput
	DecisionFallbackReasonCode string
}

func BuildWorkflowStateSnapshot(input WorkflowStateSnapshotInput) WorkflowStateSnapshot {
	facts := input.StageFacts
	actionInput := input.ActionGraph
	if len(actionInput.Blocked) == 0 {
		actionInput.Blocked = append([]WorkflowViolation(nil), input.WorkflowViolations...)
	}
	ledgerGraph := BuildLedgerGraph(facts)
	outputGraph := BuildOutputProjectionGraph(input.OutputGraph)
	progressLimit := input.ProgressLimit
	if progressLimit == 0 {
		progressLimit = 6
	}
	decisionInput := input.Decision
	if decisionInput.NextStage == "" {
		decisionInput.NextStage = NextStage(facts)
	}
	if len(decisionInput.AllowedNextActions) == 0 {
		decisionInput.AllowedNextActions = ActionKindsFromContracts(AllowedNextActionContractsForFacts(facts))
	}
	if len(decisionInput.Violations) == 0 {
		decisionInput.Violations = append([]WorkflowViolation(nil), input.WorkflowViolations...)
	}
	if decisionInput.LedgerGraph.FirstMissing == "" && decisionInput.LedgerGraph.NextStage == "" {
		decisionInput.LedgerGraph = ledgerGraph
	}
	if decisionInput.OutputGraph.Status == "" && decisionInput.OutputGraph.ReasonCode == "" {
		decisionInput.OutputGraph = outputGraph
	}
	return WorkflowStateSnapshot{
		StageFacts:                 facts,
		ActionGraph:                ReduceActionGraphState(actionInput),
		LedgerGraph:                ledgerGraph,
		OutputGraph:                outputGraph,
		ArtifactGraph:              BuildArtifactGraphState(input.Artifacts, input.ArtifactLimit),
		Progress:                   BuildProgressWindow(input.ProgressEvents, progressLimit),
		WorkflowViolations:         append([]WorkflowViolation(nil), input.WorkflowViolations...),
		Decision:                   BuildWorkflowDecision(decisionInput),
		DecisionFallbackReasonCode: input.DecisionFallbackReasonCode,
	}
}

func (s WorkflowStateSnapshot) IsZero() bool {
	return stageFactsIsZero(s.StageFacts) &&
		len(s.ActionGraph.Executed) == 0 &&
		len(s.ActionGraph.Ready) == 0 &&
		len(s.ActionGraph.Deferred) == 0 &&
		len(s.ActionGraph.Blocked) == 0 &&
		s.ActionGraph.DeferredQueue.Actions == 0 &&
		s.ActionGraph.DeferredPlan == nil &&
		len(s.ActionGraph.DeferredEvents) == 0 &&
		s.LedgerGraph.FirstMissing == "" &&
		s.LedgerGraph.NextStage == "" &&
		s.OutputGraph.Status == "" &&
		s.OutputGraph.ReasonCode == "" &&
		s.ArtifactGraph.NodeCount == 0 &&
		len(s.ArtifactGraph.Nodes) == 0 &&
		s.Progress.LatestSignature == "" &&
		len(s.Progress.Recent) == 0 &&
		len(s.WorkflowViolations) == 0 &&
		s.Decision.Status == "" &&
		s.Decision.ReasonCode == "" &&
		s.Decision.Reason == "" &&
		len(s.Decision.NextActions) == 0 &&
		s.DecisionFallbackReasonCode == ""
}

func (s WorkflowStateSnapshot) Facts() StageFacts {
	return s.StageFacts
}

func (s WorkflowStateSnapshot) NextStage() string {
	return NextStage(s.Facts())
}

func (s WorkflowStateSnapshot) AllowedNextActionContracts() []ActionContract {
	return AllowedNextActionContractsForFacts(s.Facts())
}

func (s WorkflowStateSnapshot) AllowedNextActions() []string {
	return ActionKindsFromContracts(s.AllowedNextActionContracts())
}

func stageFactsIsZero(facts StageFacts) bool {
	return !facts.MaterialCoverageSufficient &&
		!facts.RuleCoverageRequired &&
		facts.RuleCoverageRecords == 0 &&
		!facts.DecisionRecordsRequired &&
		facts.DecisionRecords == 0 &&
		!facts.EntityResolutionRequired &&
		facts.EntityResolutionRecords == 0 &&
		!facts.EntityStageMaterialized &&
		!facts.ContributionLedgerRequired &&
		facts.ContributionRecords == 0 &&
		!facts.ReconcileRequired &&
		!facts.HasReconcile &&
		!facts.HasAnswer
}
