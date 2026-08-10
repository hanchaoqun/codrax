package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CHATFIX-1: chitchat is a terminal routing decision — reconcileScenario
// must neither strip it (even when reconcile-arm predicates would fire)
// nor mint it from any non-chitchat scenario.
func TestReconcileScenarioNeverStripsOrMintsChitchat(t *testing.T) {
	rm := types.RequestModel{
		Scenario: types.ScenarioChitchat,
		// Predicates that drive the generic-flip arms on analysis scenarios.
		Predicates: types.SemanticPredicates{IsScalarAnswer: true},
	}
	if got, reason := reconcileScenario(rm); got != types.ScenarioChitchat || reason != "" {
		t.Fatalf("chitchat must survive reconcile untouched, got %s (%s)", got, reason)
	}
	rm.Scenario = types.ScenarioGeneric
	if got, _ := reconcileScenario(rm); got == types.ScenarioChitchat {
		t.Fatal("reconcile must never mint chitchat from an analysis scenario")
	}
}
