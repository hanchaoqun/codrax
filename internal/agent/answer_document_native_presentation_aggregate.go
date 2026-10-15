package agent

import "github.com/hanchaoqun/codrax/internal/types"

// observeSamplingAggregate is called only after the native tree's actual
// publication. Every cohort, node and child must be visible. Only the audit
// reference list can be partial; its omission remains explicitly disclosed.
func (p *answerDocNativePresentationReceipt) observeSamplingAggregate(r types.ObservationRecord, complete bool) {
	if p == nil || nativePresentationFamily(r) != "perf_sampling" {
		return
	}
	key := nativePresentationKey(r)
	if key == "" {
		return
	}
	if p.samplingAggregates == nil {
		p.samplingAggregates = make(map[string]bool)
	}
	if prior, exists := p.samplingAggregates[key]; exists {
		complete = complete && prior
	}
	p.samplingAggregates[key] = complete
}

// aggregateFactsAnswerRequested is an exact answer-shape boundary, not an
// interpretation of prose or a claim that every event detail was displayed.
// Unknown/extra values and occurrence/relationship questions retain strict
// member completeness. Existing source/causal obligations are checked by the
// caller before this narrower presentation-only equivalence is considered.
func aggregateFactsAnswerRequested(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := &ctx.AnalysisIR.RequestModel
	q := rm.RuntimeQuestionProfile
	if !q.BoundedFactSet() || q.RequestsRuntimeWorkRelation() || q.RequestsFrameCausality() ||
		len(q.FactFamilies) == 0 || rm.CompletenessObligation.IsActive() {
		return false
	}
	for _, family := range q.FactFamilies {
		if family != types.RuntimeQuestionFactCountOrDuration {
			return false
		}
	}
	if dimensions := rm.RequestedAnswerDimensions; dimensions != nil {
		for _, d := range dimensions.Dimensions {
			if !d.Required {
				continue
			}
			switch d.Role {
			case types.RequestedAnswerDimensionCount, types.RequestedAnswerDimensionMemberSet,
				types.RequestedAnswerDimensionObservedValue,
				types.RequestedAnswerDimensionEvidenceSource, types.RequestedAnswerDimensionBoundary,
				types.RequestedAnswerDimensionDiagram:
			default:
				return false
			}
		}
	}
	return true
}

// answerFactsCompleteFor leaves completeFor's strict audit/member contract
// unchanged. A complete native aggregate may replace only the redundant
// current synopsis; independent facts, source explanations, relation claims
// and investigation notes are never removed by this receipt.
func (p *answerDocNativePresentationReceipt) answerFactsCompleteFor(ctx *types.AgentContext) bool {
	if p.completeFor(ctx) {
		return true
	}
	if p == nil || !aggregateFactsAnswerRequested(ctx) {
		return false
	}
	seen := false
	for _, r := range answerDocObservationLedger(ctx).Records {
		if r.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) {
			continue
		}
		key := nativePresentationKey(r)
		if key == "" {
			key = p.eventMirrorKey(r)
		}
		if key == "" || !p.queries[key] && !(nativePresentationFamily(r) == "perf_sampling" && p.samplingAggregates[key]) {
			return false
		}
		seen = true
	}
	return seen
}
