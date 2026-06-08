package dataworkflow

type WorkflowStateDiagnosticsInput struct {
	Records           []WorkflowRecord
	State             WorkflowStateView
	ArtifactAccess    []ArtifactAccessView
	DiagnosticBatches []ArtifactDiagnosticBatch
	Limit             int
}

func BuildWorkflowStateDiagnostics(input WorkflowStateDiagnosticsInput) WorkflowStateDiagnostics {
	limit := defaultPositive(input.Limit, 4)
	state := input.State
	out := WorkflowStateDiagnostics{}
	if limit <= 0 {
		return out
	}
	if len(input.ArtifactAccess) > 0 {
		out.FieldContractViolations = DiscoverFieldContractIssues(FieldContractIssueInput{
			Records:            input.Records,
			ArtifactAccess:     append([]ArtifactAccessView(nil), input.ArtifactAccess...),
			AllowedNextActions: append([]string(nil), state.AllowedNextActions...),
			Limit:              limit,
		})
	}
	if len(input.DiagnosticBatches) == 0 {
		return out
	}
	contributionOrReconcileRequired := state.ContributionLedgerRequired || state.ReconcileRequired
	contributionAlreadyPresent := state.ContributionRecords > 0
	if contributionOrReconcileRequired && !contributionAlreadyPresent {
		out.ZeroMatchFilterViolations = ZeroMatchFilterIssuesFromBatches(input.DiagnosticBatches, limit)
		out.ZeroEligibleViolations = ZeroEligibleIssuesFromBatches(input.DiagnosticBatches, limit)
	}
	if state.EntityResolutionRequired && contributionOrReconcileRequired && !contributionAlreadyPresent {
		out.UnmatchedResolutionViolations = UnmatchedResolutionIssuesFromBatches(input.DiagnosticBatches, limit)
	}
	return out
}
