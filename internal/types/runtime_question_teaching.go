package types

// RequestsTraceSchedulerTeaching scopes instructions, never evidence, tools,
// supplementary queries or causal eligibility. Unknown/legacy requests keep
// their existing teaching. Only an explicit finite profile can narrow it.
func (p *RuntimeQuestionProfile) RequestsTraceSchedulerTeaching() bool {
	if p == nil || !p.CarriesBoundedFactFamilies() {
		return true
	}
	for _, family := range p.FactFamilies {
		switch family {
		case RuntimeQuestionFactTargetSchedulerState, RuntimeQuestionFactResourcePressure,
			RuntimeQuestionFactFrequencyResidency, RuntimeQuestionFactTargetWaitOccurrences,
			RuntimeQuestionFactRecordedReason, RuntimeQuestionFactRelationPeer,
			RuntimeQuestionFactTransactionID, RuntimeQuestionFactDirectWaker,
			RuntimeQuestionFactIOLatency:
			return true
		}
	}
	// A generic occurrence timestamp or count is not a scheduler request.
	return false
}
