package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
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

func TestProjectWriteExplorationHandoffFallsBackToEmittedEvidence(t *testing.T) {
	mu := types.NewMutableState("degraded exploration handoff")
	mu.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID:        "batch-1",
		Goal:           "repair autodoc typehints",
		CandidatePaths: []string{"sphinx/ext/autodoc/typehints.py"},
	})
	mu.EvidenceClosure().SetReadSet(map[string]bool{
		"sphinx/ext/autodoc/typehints.py": true,
	})
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:              "ev-record-typehints",
		Kind:            types.EvidenceMechanism,
		Subject:         "record_typehints",
		Source:          "sphinx/ext/autodoc/typehints.py",
		LineStart:       127,
		AnchorSymbol:    "record_typehints",
		Summary:         "autodoc records resolved annotations before description rendering",
		GroundingStatus: types.GroundingRecovered,
	}})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu}}

	o.projectWriteExplorationHandoffFromTurnA()

	handoff := mu.WriteExplorationHandoff()
	if handoff == nil {
		t.Fatal("expected projected handoff from emitted evidence")
	}
	if len(handoff.EvidenceRefs) != 1 || handoff.EvidenceRefs[0].ID != "ev-record-typehints" {
		t.Fatalf("fallback handoff should preserve emitted evidence refs: %+v", handoff)
	}
	if !stringSliceContains(handoff.TargetFiles, "sphinx/ext/autodoc/typehints.py") {
		t.Fatalf("fallback handoff should preserve closure/candidate target file: %+v", handoff.TargetFiles)
	}
	if ta := mu.TurnAArtifacts(); ta == nil || len(ta.EvidenceItems) != 1 {
		t.Fatalf("fallback should materialize TurnAArtifacts for later consumers: %+v", ta)
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

func TestApplyWriteExplorationRoundBudgetForDispatchUsesTypedMaxRounds(t *testing.T) {
	mu := types.NewMutableState("bounded write exploration")
	mu.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID:   "batch-1",
		Goal:      "inspect failing source paths",
		MaxRounds: 2,
	})
	ctx := &types.AgentContext{
		Stage:           types.StageExplore,
		Mode:            types.ModeApply,
		MaxIterOverride: 35,
	}
	applyWriteExplorationRoundBudgetForDispatch(ctx, &types.BusContext{Mutable: mu})
	if ctx.MaxIterOverride != 2 {
		t.Fatalf("write exploration max_rounds should tighten existing explorer budget to 2, got %d", ctx.MaxIterOverride)
	}

	ctx = &types.AgentContext{Stage: types.StageExplore, Mode: types.ModeRead, MaxIterOverride: 35}
	applyWriteExplorationRoundBudgetForDispatch(ctx, &types.BusContext{Mutable: mu})
	if ctx.MaxIterOverride != 35 {
		t.Fatalf("read exploration budget must not be affected, got %d", ctx.MaxIterOverride)
	}

	mu.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID: "batch-1",
		Goal:    "inspect source paths without explicit budget",
	})
	ctx = &types.AgentContext{Stage: types.StageExplore, Mode: types.ModeApply, MaxIterOverride: 0}
	applyWriteExplorationRoundBudgetForDispatch(ctx, &types.BusContext{Mutable: mu})
	if ctx.MaxIterOverride != defaultWriteWorkflowMaxExplorationRounds {
		t.Fatalf("write exploration without explicit max_rounds should use default hard cap %d, got %d",
			defaultWriteWorkflowMaxExplorationRounds, ctx.MaxIterOverride)
	}
}

func TestRunWriteExplorationSubflowAppliesMaxRoundsOverride(t *testing.T) {
	mu := types.NewMutableState("bounded write exploration")
	mu.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID:        "batch-1",
		Goal:           "inspect source range handling",
		CandidatePaths: []string{"src/_pytest/_code/source.py"},
		MaxRounds:      2,
	})
	captured := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			captured = ctx.MaxIterOverride
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
				ReadFiles: []string{"src/_pytest/_code/source.py"},
			})
			return &agent.StageOutput{}, nil
		},
	})
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable:    mu,
		Mode:       types.ModeApply,
		AnalysisIR: &types.AnalysisIR{},
	}
	o.cancelToken = NewCancelToken()

	if _, err := o.runWriteExplorationSubflow(); err != nil {
		t.Fatalf("runWriteExplorationSubflow returned error: %v", err)
	}
	if captured != 2 {
		t.Fatalf("explorer dispatch should receive typed max_rounds override 2, got %d", captured)
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

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
