package repl

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

const turnOutcomesTeaching = "First record all user-requested result kinds in required_outcomes, independently of route and of which subtask seems primary. source_explanation means an explanation grounded in the current code's implementation, including a secondary explanation alongside a count or measurement; describing your own command/receipt is not that result. measurement is a requested observed statistic. external_artifact and computer_action are requested deliverables/actions, not shell commands used internally to gather evidence. source_change requires separate explicit write authorization. Use answer for other answer-only results; do not list inferred implementation steps."

const turnOutcomesSchema = `{"type":"array","maxItems":6,"uniqueItems":true,"items":{"type":"string","enum":["answer","measurement","source_explanation","external_artifact","computer_action","source_change"]}}`

func renderTurnOutcomesForPrompt(outcomes types.TurnOutcomeSet) string {
	if outcomes == 0 {
		return ""
	}
	return fmt.Sprintf("required_outcomes=%q (requested results, not execution steps; each needs evidence or an explicit remaining limitation)\n", outcomes.Names())
}

// Reconcile independent typed result obligations before route-specific guards.
// No prose inspection; no outcome bit authorizes write mode or a side effect.
func applyTurnOutcomeObligations(p TurnPolicy) TurnPolicy {
	if !p.RequiredOutcomes.Has(types.TurnOutcomeSourceExplanation) {
		return p
	}
	p.CurrentSourceEvidenceMode = types.TurnRouteCurrentSourceEvidenceRequired
	p.NeedsRepoAccess = true
	if p.RequiredOutcomes.Has(types.TurnOutcomeExternalArtifact|types.TurnOutcomeComputerAction|types.TurnOutcomeSourceChange) ||
		p.WriteIntent == WriteIntentExplicitChange || p.Route == RouteWrite || len(p.SideEffects) > 0 {
		return p
	}
	if p.Route != RouteHybrid {
		p.Route = RouteRepo
	}
	p.Operation, p.OperationKind, p.DataTaskKind = "investigate", "", ""
	p.Source = "repo"
	if p.RequiredOutcomes.Has(types.TurnOutcomeMeasurement) {
		p.Source = "mixed"
	}
	p.NeedsOperationAccess, p.NeedsDataAccess = false, false
	p.TargetSurface = ""
	return p
}
