package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func laneBus(perMember bool) *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Predicates: types.SemanticPredicates{HasPerMemberTable: perMember},
			},
		},
	}
}

// Obligation truth table: declared shape without a member_set handoff
// is pending; a stable member_set fact or a retained absence
// justification satisfies it; the undeclared shape never obliges.
func TestPendingCompletionObligations(t *testing.T) {
	if got := pendingCompletionObligations(nil); got != nil {
		t.Fatalf("nil bus must report nothing")
	}
	if got := pendingCompletionObligations(laneBus(false)); got != nil {
		t.Fatalf("undeclared shape must report nothing, got %v", got)
	}

	bus := laneBus(true)
	if got := pendingCompletionObligations(bus); len(got) != 1 {
		t.Fatalf("declared shape without handoff must be pending, got %v", got)
	}

	bus.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{Kind: types.AnswerAggregateMemberSet, Label: "stages", Members: []string{"A", "B"}},
	})
	bus.Mutable.SetInvestigationComplete("members verified")
	bus.Mutable.RetainInvestigationAggregateFacts()
	bus.Mutable.ResetInvestigationComplete()
	if got := pendingCompletionObligations(bus); got != nil {
		t.Fatalf("member_set fact must satisfy the obligation, got %v", got)
	}
}

// A retained absence justification is the typed escape lane.
func TestPendingCompletionObligationsAbsenceEscape(t *testing.T) {
	bus := laneBus(true)
	bus.Mutable.SetAbsenceJustification("the asked range has no members")
	bus.Mutable.SetInvestigationResultKind("absence")
	if j := bus.Mutable.StableAbsenceJustification(); strings.TrimSpace(j) == "" {
		t.Fatalf("absence declaration must surface through the stable getter")
	}
	if got := pendingCompletionObligations(bus); got != nil {
		t.Fatalf("retained absence justification must satisfy the obligation, got %v", got)
	}
}

// The lane hint names the pending obligation and BOTH typed exits.
func TestCompletionObligationLaneHint(t *testing.T) {
	hint := completionObligationLaneHint([]string{"per-member table requires a member_set aggregate fact"})
	for _, want := range []string{"member_set", "absence_justification", "emit_investigation_complete", "support_refs"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint missing %q: %s", want, hint)
		}
	}
}

// The lane never fires twice, never fires on an accepted completion,
// and never fires without a pending obligation — the three guards
// that keep stable scenarios byte-identical.
func TestRunCompletionObligationLaneGuards(t *testing.T) {
	o := &Orchestrator{busCtx: laneBus(true)}
	steps := 0

	// Guard 1: already-fired flag short-circuits before any dispatch.
	fired := true
	o.runCompletionObligationLane(&fired, &steps)
	if steps != 0 {
		t.Fatalf("fired flag must short-circuit")
	}

	// Guard 2: accepted completion short-circuits.
	fired = false
	o.busCtx.Mutable.SetInvestigationComplete("done")
	o.runCompletionObligationLane(&fired, &steps)
	if steps != 0 || fired {
		t.Fatalf("accepted completion must short-circuit (steps=%d fired=%t)", steps, fired)
	}

	// Guard 3: no pending obligation short-circuits.
	o2 := &Orchestrator{busCtx: laneBus(false)}
	fired = false
	o2.runCompletionObligationLane(&fired, &steps)
	if steps != 0 || fired {
		t.Fatalf("no-obligation path must short-circuit (steps=%d fired=%t)", steps, fired)
	}
}
