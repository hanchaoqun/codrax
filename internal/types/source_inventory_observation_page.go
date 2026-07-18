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
		// Defensive only — unreachable in the present mint topology. The
		// executed-empty carrier is never IsActive, so it travels the early
		// credential arms of MergeSourceInventoryObservation; this function is
		// reached only on the both-active merge path, which never sees a
		// LensExecutedEmpty side today. The OR is kept so a future field drop
		// or a new both-active mint shape cannot silently lose the flag.
		LensExecutedEmpty: existing.LensExecutedEmpty || incoming.LensExecutedEmpty,
	}
}
