package context

import "github.com/hanchaoqun/codrax/internal/types"

// Finalization must not replay raw aggregates that the answer projection has
// excluded, even if no facts survived. Exploration/extraction still need raw
// advisory leads; a plan can exist in those stages without granting it ownership
// of their context. Neither branch modifies the retained audit carriers.
func relationDossierAggregateFacts(ac *types.AgentContext) []types.AnswerAggregateFact {
	if ac == nil || ac.Mutable == nil {
		return nil
	}
	if ac.Stage == types.StageFinalize {
		if plan := types.BuildAnswerSurfacePlanForAgentContext(ac); plan != nil {
			return plan.StableAggregateFacts
		}
	}
	facts := ac.Mutable.StableInvestigationAggregateFacts()
	if ta := ac.Mutable.TurnAArtifacts(); ta != nil && len(ta.AcceptedAggregateFacts) > 0 {
		facts = types.MergeAnswerAggregateFacts(facts, ta.AcceptedAggregateFacts)
	}
	return facts
}
