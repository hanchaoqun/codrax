package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestProjectWriteExplorationHandoffFromTurnARequiresRequestAndArtifacts(t *testing.T) {
	mu := types.NewMutableState("x")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu}}
	o.projectWriteExplorationHandoffFromTurnA()
	if got := mu.WriteExplorationHandoff(); got != nil {
		t.Fatalf("handoff should remain nil without request/artifacts: %+v", got)
	}

	mu.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID:        "batch-1",
		Goal:           "patch planner",
		CandidatePaths: []string{"internal/agent/planner.go"},
	})
	o.projectWriteExplorationHandoffFromTurnA()
	if got := mu.WriteExplorationHandoff(); got != nil {
		t.Fatalf("handoff should remain nil without artifacts: %+v", got)
	}

	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/agent/planner.go"},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev1",
			Kind:            types.EvidenceMechanism,
			Subject:         "planner handoff",
			Source:          "internal/agent/planner.go",
			LineStart:       105,
			Summary:         "planner renders prompt sections",
			GroundingStatus: types.GroundingRecovered,
		}},
	})
	o.projectWriteExplorationHandoffFromTurnA()
	handoff := mu.WriteExplorationHandoff()
	if handoff == nil {
		t.Fatal("expected projected handoff")
	}
	if handoff.BatchID != "batch-1" || handoff.Goal != "patch planner" {
		t.Fatalf("handoff identity drift: %+v", handoff)
	}
	pack := mu.WriteContextPack()
	if pack == nil {
		t.Fatal("expected priority context pack")
	}
	view := pack.View(types.WriteConsumerPlanner, 10)
	if !writeContextViewContains(view, "target_file", "internal/agent/planner.go") {
		t.Fatalf("planner context pack missing target file: %+v", view.Items)
	}
}

func TestShouldRunWriteExplorationSubflowUsesTypedEvaluator(t *testing.T) {
	if shouldRunWriteExplorationSubflow(types.WriteExplorationRequest{}) {
		t.Fatal("empty request must not run exploration")
	}
	if !shouldRunWriteExplorationSubflow(types.WriteExplorationRequest{
		BatchID:        "batch-1",
		Goal:           "inspect planner",
		CandidatePaths: []string{"internal/agent/planner.go"},
	}) {
		t.Fatal("valid typed request should continue_explore")
	}
}

// TestExtractPhaseGoalFromPrefix pins commit 40's helper used
// by the dispatch-time keyword-boost wiring: parses the
// "## Phase X of Y: <goal>" header back into just the goal.
// Empty / non-matching prefixes return empty so callers can
// degrade silently.
func TestExtractPhaseGoalFromPrefix(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string
	}{
		{"", ""},
		{"## Phase 2 of 3: update ORM\n\n", "update ORM"},
		{"## Phase 1 of 5: add migration to schema\n\nRough target paths:\n- a.sql\n", "add migration to schema"},
		{"## Phase 1 of 1: solo phase\n", "solo phase"},
		{"## Not a phase header", ""},
		{"## Phase no colon here", ""},
		// Trailing whitespace / newlines stripped:
		{"## Phase 1 of 1: x  \n", "x"},
	} {
		if got := extractPhaseGoalFromPrefix(c.in); got != c.want {
			t.Errorf("extractPhaseGoalFromPrefix(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

// TestRunPhaseGroup_HappyPathThreePhases is the commit 30 e2e
// pin: drive the multi-phase scheduler through three
// non-terminal phases with a stub runTaskPhase that pre-seeds
// the phase output (plan + report + commit SHA). Confirms:
//   - every phase transitions Pending → InProgress → Accepted
//   - PlanID + AppliedSHA are recorded per phase
//   - PhaseContextPrefix gets seeded at each phase entry
//   - acceptance verdict propagates NextHint to next phase
//   - group reaches PlanGroupCompleted
//   - persistGroup is called on every transition
//
// The runTaskPhase stub seeds the bus state that the real
// implementation would have produced via runTaskGraph (which
// requires a full ReAct stack — out of reach for a unit test).
// All three phases get the same plan + clean report, so
// acceptance is unambiguously "passed" each time.

func writeContextViewContains(view types.WriteContextView, kind, substring string) bool {
	for _, item := range view.Items {
		if item.Kind == kind && strings.Contains(item.Text, substring) {
			return true
		}
	}
	return false
}
