package types

func (c *RuntimeMeasurementContract) qualifyNativeDisplayDimensions(rm *RequestModel) {
	c.displayDimensionIndices = nativeDisplayDimensionIndices(rm)
	c.displayPopulationKeys = map[string]bool{}
	for _, table := range c.Choices() {
		if table.HasCompleteNativeDisplayPopulation() || table.CoversMemberSet(rm) {
			c.displayPopulationKeys[table.ObservationID+"\x00"+string(table.View)] = true
		}
	}
}

// Index ownership is a current typed presentation decision. No label, source
// quote, user wording or answer prose is inspected. Source-bound dimensions
// remain source obligations even when native facts are available beside them.
func nativeDisplayDimensionIndices(rm *RequestModel) []int {
	if rm == nil || rm.RuntimeQuestionProfile == nil || rm.SourceInventoryProfile.Active() || !rm.RequestedAnswerDimensions.Active() {
		return nil
	}
	switch rm.RuntimeQuestionProfile.Scope {
	case RuntimeQuestionScopeBoundedFactSet, RuntimeQuestionScopeBoundedEffectVerdict, RuntimeQuestionScopeCausalDiagnosis, RuntimeQuestionScopeRelationAnalysis, RuntimeQuestionScopeSystemOverview:
	default:
		return nil
	}
	counts, sourceOwned := map[int]int{}, map[int]bool{}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if CanonicalRequestedDimensionSource(hint.Path) == "" {
			continue
		}
		for _, index := range hint.RequestedDimensionIndices {
			sourceOwned[index] = true
		}
	}
	for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
		counts[dim.Index]++
	}
	var indices []int
	for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
		if dim.Index > 0 && counts[dim.Index] == 1 && !sourceOwned[dim.Index] && dim.Required && dim.Role == RequestedAnswerDimensionMemberSet {
			indices = append(indices, dim.Index)
		}
	}
	return indices
}

// NativeDisplayDimensionChoices publishes metadata only when at least one
// complete producer-owned display population is currently available.
func (c *RuntimeMeasurementContract) NativeDisplayDimensionChoices() []int {
	for _, table := range c.Choices() {
		if c.displayPopulationKeys[table.ObservationID+"\x00"+string(table.View)] {
			return append([]int(nil), c.displayDimensionIndices...)
		}
	}
	return nil
}

// BoundNativeDisplayDimensions rebinds against the current contract. This is
// visible display coverage for explicitly selected indices, not complete
// investigation/capture coverage or semantic/causal relevance certification.
func BoundNativeDisplayDimensions(block AnswerBlock, c *RuntimeMeasurementContract) ([]int, bool) {
	if block.Kind != BlockTable || block.RuntimeMeasurement == nil || len(block.RequestedDimensionIndices) == 0 {
		return nil, false
	}
	receipt := *block.RuntimeMeasurement
	if !BindRuntimeMeasurementReceipt(&receipt, c) || !c.displayPopulationKeys[receipt.ObservationID+"\x00"+string(receipt.View)] {
		return nil, false
	}
	allowed, seen := map[int]bool{}, map[int]bool{}
	for _, index := range c.NativeDisplayDimensionChoices() {
		allowed[index] = true
	}
	for _, index := range block.RequestedDimensionIndices {
		if !allowed[index] || seen[index] {
			return nil, false
		}
		seen[index] = true
	}
	return append([]int(nil), block.RequestedDimensionIndices...), true
}
