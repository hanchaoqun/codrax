package tool

import "github.com/hanchaoqun/codrax/internal/types"

// preEmitDerivedBuildCounts is observability for the request-local immutable
// derivation cache. Tests pin one build per emit context so large rosters cannot
// silently return to item × citation × graph reconstruction.
type preEmitDerivedBuildCounts struct {
	surfacePlan            int
	stableAggregateFacts   int
	principalAggregateRefs int
}

func (c *preEmitCheckContext) answerSurfacePlanForCheck() *types.AnswerSurfacePlan {
	if c == nil || c.ctx == nil {
		return nil
	}
	if !c.surfacePlanBuilt {
		c.surfacePlanBuilt = true
		c.derivedBuilds.surfacePlan++
		c.surfacePlan = answerSurfacePlan(c.ctx)
	}
	return c.surfacePlan
}

func (c *preEmitCheckContext) stableAggregateFactsForCheck() []types.AnswerAggregateFact {
	if c == nil || c.ctx == nil {
		return nil
	}
	if c.stableAggregateFactsBuilt {
		return c.stableAggregateFacts
	}
	c.stableAggregateFactsBuilt = true
	c.derivedBuilds.stableAggregateFacts++
	if plan := c.answerSurfacePlanForCheck(); plan != nil && len(plan.StableAggregateFacts) > 0 {
		// answerSurfacePlan already applies typed exclusion normalization.
		c.stableAggregateFacts = plan.StableAggregateFacts
		c.stableFactsExcluded = true
		return c.stableAggregateFacts
	}
	if c.ctx.Mutable != nil {
		c.stableAggregateFacts = c.ctx.Mutable.StableInvestigationAggregateFacts()
	}
	return c.stableAggregateFacts
}

func (c *preEmitCheckContext) principalAggregateMemberSetFactRefsForCheck() []types.AnswerAggregateFactRef {
	if c == nil || c.ctx == nil {
		return nil
	}
	if c.principalAggregateRefsBuilt {
		return c.principalAggregateRefs
	}
	c.principalAggregateRefsBuilt = true
	c.derivedBuilds.principalAggregateRefs++
	facts := c.stableAggregateFactsForCheck()
	if !c.stableFactsExcluded {
		facts = normalizeAggregateFactsForTypedExclusion(c.ctx, facts)
	}
	facts = types.PruneAggregateMemberSetsByStructuredExclusions(facts)
	if c.ctx.AnalysisIR == nil {
		c.principalAggregateRefs = types.PrincipalAggregateMemberSetFactRefs(facts)
	} else {
		c.principalAggregateRefs = types.PrincipalAggregateMemberSetFactRefsForRequest(facts, &c.ctx.AnalysisIR.RequestModel)
	}
	return c.principalAggregateRefs
}

// forAutomaticSourceCitation is a request-local candidate view, not a new
// answer contract. Model-selected citations still use the original context.
// Share immutable evidence caches, but allocate a distinct refs slice so
// filtering automatic candidates cannot erase retained model aggregates.
func (c *preEmitCheckContext) forAutomaticSourceCitation() *preEmitCheckContext {
	if c == nil || c.ctx == nil || c.sourceQualifiedAggregateRefs {
		return c
	}
	if c.automaticCitationContext != nil {
		return c.automaticCitationContext
	}
	refs := c.principalAggregateMemberSetFactRefsForCheck()
	source := types.AnswerAggregateSourceContextFromBusContext(c.ctx)
	qualified := make([]types.AnswerAggregateFactRef, 0, len(refs))
	for _, ref := range refs {
		if types.AnswerAggregateFactHasObservedSourceMembers(ref.Fact, source) {
			qualified = append(qualified, ref)
		}
	}
	facts := c.stableAggregateFactsForCheck()
	qualifiedFacts := make([]types.AnswerAggregateFact, 0, len(facts))
	for _, fact := range facts {
		if types.AnswerAggregateFactHasObservedSourceMembers(fact, source) {
			qualifiedFacts = append(qualifiedFacts, fact)
		}
	}
	auto := *c
	auto.stableAggregateFacts = qualifiedFacts
	auto.stableAggregateFactsBuilt = true
	auto.principalAggregateRefs = qualified
	auto.principalAggregateRefsBuilt = true
	auto.sourceQualifiedAggregateRefs = true
	auto.automaticCitationContext = nil
	c.automaticCitationContext = &auto
	return c.automaticCitationContext
}
