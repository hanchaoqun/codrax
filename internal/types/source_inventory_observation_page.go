package types

func maxSourceInventoryObservationTotal(values ...int) int {
	max := 0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

func cloneSourceInventoryObservationPage(in *SourceInventoryObservationPage) *SourceInventoryObservationPage {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneSourceInventoryExecutionState(in *SourceInventoryExecutionState) *SourceInventoryExecutionState {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func mergeSourceInventoryExecutionState(existing, incoming *SourceInventoryExecutionState) *SourceInventoryExecutionState {
	if existing == nil {
		return cloneSourceInventoryExecutionState(incoming)
	}
	if incoming == nil {
		return cloneSourceInventoryExecutionState(existing)
	}
	return &SourceInventoryExecutionState{
		Budgeted:                 existing.Budgeted || incoming.Budgeted,
		CandidateBudgetTruncated: existing.CandidateBudgetTruncated || incoming.CandidateBudgetTruncated,
		AttributesDeferred:       existing.AttributesDeferred || incoming.AttributesDeferred,
	}
}
