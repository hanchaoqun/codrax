package orchestrator

import (
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBuildPlanGroupFromProposal_MapsPhases pins the seed →
// PhaseRecord conversion. Each PhaseSeed becomes a PhaseRecord
// with the same Goal + RoughTargetPaths and PhasePending status.
func TestBuildPlanGroupFromProposal_MapsPhases(t *testing.T) {
	o := &Orchestrator{}
	ir := &types.WriteAnalysisIR{
		Request: types.WriteRequestModel{RawRequest: "schema-then-code"},
		PhaseProposal: types.PhaseProposal{
			Split: "sequential",
			Phases: []types.PhaseSeed{
				{Goal: "add migration", RoughTargetPaths: []string{"migrations/0001.sql"}},
				{Goal: "update ORM", RoughTargetPaths: []string{"internal/db/user.go"}},
			},
		},
	}
	g := o.buildPlanGroupFromProposal(ir)
	if g == nil {
		t.Fatal("expected non-nil group")
	}
	if g.Goal != "schema-then-code" {
		t.Errorf("Goal drift: got %q", g.Goal)
	}
	if g.Decision != "linear" {
		t.Errorf("Decision drift: got %q", g.Decision)
	}
	if g.Status != types.PlanGroupPlanning {
		t.Errorf("Status drift: got %q", g.Status)
	}
	if len(g.Phases) != 2 {
		t.Fatalf("Phases length drift: got %d", len(g.Phases))
	}
	for i, want := range []struct {
		goal string
		path string
	}{
		{"add migration", "migrations/0001.sql"},
		{"update ORM", "internal/db/user.go"},
	} {
		if g.Phases[i].Index != i {
			t.Errorf("Phase[%d].Index = %d", i, g.Phases[i].Index)
		}
		if g.Phases[i].Goal != want.goal {
			t.Errorf("Phase[%d].Goal = %q; want %q", i, g.Phases[i].Goal, want.goal)
		}
		if len(g.Phases[i].RoughTargetPaths) != 1 || g.Phases[i].RoughTargetPaths[0] != want.path {
			t.Errorf("Phase[%d].RoughTargetPaths = %v", i, g.Phases[i].RoughTargetPaths)
		}
		if g.Phases[i].Status != types.PhasePending {
			t.Errorf("Phase[%d].Status = %q; want pending", i, g.Phases[i].Status)
		}
	}
	// ID shape: group-<unix-nano>-<pid>
	if !strings.HasPrefix(g.ID, "group-") {
		t.Errorf("group ID prefix drift: got %q", g.ID)
	}
}

// TestIsMultiPhaseRun_PreconditionsAllRequired pins the gate's
// AND-of-three semantics: any single missing precondition
// means single-phase fallback.
func TestIsMultiPhaseRun_PreconditionsAllRequired(t *testing.T) {
	mu := types.NewMutableState("x")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		PhaseProposal: types.PhaseProposal{
			Split: "sequential",
			Phases: []types.PhaseSeed{
				{Goal: "p1"}, {Goal: "p2"},
			},
		},
	})
	bus := &types.BusContext{Mutable: mu, Mode: types.ModeApply}
	o := &Orchestrator{busCtx: bus, planGroupStore: &fakeGroupStore{}}

	if !o.isMultiPhaseRun() {
		t.Error("all preconditions met → expected true")
	}

	// Strip 1: no PlanGroupStore.
	o.planGroupStore = nil
	if o.isMultiPhaseRun() {
		t.Error("nil store → should be false")
	}
	o.planGroupStore = &fakeGroupStore{}

	// Strip 2: split = single.
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		PhaseProposal: types.PhaseProposal{Split: "single"},
	})
	if o.isMultiPhaseRun() {
		t.Error("split=single → should be false")
	}

	// Strip 3: only one phase.
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		PhaseProposal: types.PhaseProposal{
			Split: "sequential", Phases: []types.PhaseSeed{{Goal: "only"}},
		},
	})
	if o.isMultiPhaseRun() {
		t.Error("single-phase 'sequential' → should be false")
	}

	// Strip 4: ModePlan (not ModeApply).
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		PhaseProposal: types.PhaseProposal{
			Split: "sequential", Phases: []types.PhaseSeed{{Goal: "p1"}, {Goal: "p2"}},
		},
	})
	bus.Mode = types.ModePlan
	if o.isMultiPhaseRun() {
		t.Error("ModePlan → should be false (only ModeApply drives multi-phase)")
	}

	// Strip 5: nil IR.
	bus.Mode = types.ModeApply
	mu.SetWriteAnalysisIR(nil)
	if o.isMultiPhaseRun() {
		t.Error("nil IR → should be false")
	}

	// Strip 6: ModeRead — never multi-phase.
	bus.Mode = types.ModeRead
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		PhaseProposal: types.PhaseProposal{
			Split: "sequential", Phases: []types.PhaseSeed{{Goal: "p1"}, {Goal: "p2"}},
		},
	})
	if o.isMultiPhaseRun() {
		t.Error("ModeRead → must NEVER be multi-phase (red line)")
	}
}

// TestSeedPlanningHintFromPhase pins the per-phase hint
// content: phase number, goal, rough target paths, and
// optional carry-over hint from prior acceptance.
func TestSeedPlanningHintFromPhase(t *testing.T) {
	mu := types.NewMutableState("x")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu}}
	g := &types.PlanGroup{
		ID:    "group-x",
		Goal:  "x",
		Phases: []types.PhaseRecord{
			{Index: 0, Goal: "phase one", RoughTargetPaths: []string{"a.go", "b.go"}},
			{Index: 1, Goal: "phase two", RoughTargetPaths: []string{"c.go"}},
		},
	}
	o.nextPhaseHint = "users.email column was added"

	o.seedPlanningHintFromPhase(&g.Phases[1], g)
	hint := mu.PlanningHint()
	if !strings.Contains(hint, "## Phase 2 of 2: phase two") {
		t.Errorf("expected phase header; got %q", hint)
	}
	if !strings.Contains(hint, "- c.go") {
		t.Errorf("expected rough target paths; got %q", hint)
	}
	if !strings.Contains(hint, "users.email column was added") {
		t.Errorf("expected carry-over hint; got %q", hint)
	}
	// nextPhaseHint should be consumed.
	if o.nextPhaseHint != "" {
		t.Errorf("nextPhaseHint should be cleared after consume; got %q", o.nextPhaseHint)
	}
}

// TestRunPhaseGroup_SkipsTerminalPhases pins the iteration
// gate: phases already at terminal status (Accepted, RolledBack,
// Skipped) get stepped over without re-running.
func TestRunPhaseGroup_SkipsTerminalPhases(t *testing.T) {
	store := &fakeGroupStore{}
	mu := types.NewMutableState("test")
	bus := &types.BusContext{
		Mutable:  mu,
		Mode:     types.ModeApply,
		AnalysisIR: &types.AnalysisIR{},
	}
	// Set a CancelToken so o.CancelContext() returns a real ctx
	// rather than panicking.
	o := &Orchestrator{
		busCtx:         bus,
		planGroupStore: store,
		cancelToken:    NewCancelToken(),
	}

	g := &types.PlanGroup{
		ID:        "group-skip-test",
		Decision:  "linear",
		Status:    types.PlanGroupPlanning,
		Phases: []types.PhaseRecord{
			{Index: 0, Goal: "p0", Status: types.PhaseAccepted},  // already done
			{Index: 1, Goal: "p1", Status: types.PhaseSkipped},   // operator skipped
			{Index: 2, Goal: "p2", Status: types.PhaseRolledBack}, // failed earlier
		},
	}
	// All phases are terminal — runPhaseGroup should skip past
	// all of them and mark the group Completed without ever
	// dispatching runTaskPhase. We can't actually call
	// runTaskPhase from a test (it requires a full agent
	// stack), but with all phases terminal it never runs.
	stepsUsed := 0
	if err := o.runPhaseGroup(g, &stepsUsed); err != nil {
		t.Fatalf("runPhaseGroup: %v", err)
	}
	if g.Status != types.PlanGroupCompleted {
		t.Errorf("expected PlanGroupCompleted; got %q", g.Status)
	}
	if g.ActiveIdx != 3 {
		t.Errorf("ActiveIdx should advance past all phases; got %d", g.ActiveIdx)
	}
	// Persistence happened.
	if store.saveCount == 0 {
		t.Error("expected at least one Save call")
	}
}

// TestRunPhaseGroup_NilGroupRejected pins the defensive guard.
func TestRunPhaseGroup_NilGroupRejected(t *testing.T) {
	o := &Orchestrator{}
	stepsUsed := 0
	err := o.runPhaseGroup(nil, &stepsUsed)
	if err == nil {
		t.Fatal("nil group should error")
	}
}

// TestRunPhaseGroup_EmptyPhasesRejected pins the empty-phases
// guard.
func TestRunPhaseGroup_EmptyPhasesRejected(t *testing.T) {
	o := &Orchestrator{}
	g := &types.PlanGroup{ID: "empty"}
	stepsUsed := 0
	err := o.runPhaseGroup(g, &stepsUsed)
	if err == nil {
		t.Fatal("zero-phase group should error")
	}
	if !strings.Contains(err.Error(), "zero phases") {
		t.Errorf("expected 'zero phases' in error; got %v", err)
	}
}

// TestPlanSummaryForAcceptance_NilSafe + ExpectedOutcomesForAcceptance
// pin the small accessor helpers.
func TestPlanSummaryForAcceptance_NilSafe(t *testing.T) {
	if got := planSummaryForAcceptance(nil); got != "" {
		t.Errorf("nil plan should yield empty summary; got %q", got)
	}
	if got := planSummaryForAcceptance(&types.ChangePlan{Summary: "hello"}); got != "hello" {
		t.Errorf("Summary drift; got %q", got)
	}
}

func TestExpectedOutcomesForAcceptance_NilSafe(t *testing.T) {
	if got := expectedOutcomesForAcceptance(nil); got != nil {
		t.Errorf("nil bus should yield nil; got %v", got)
	}
	mu := types.NewMutableState("x")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{ExpectedOutcomes: []string{"a", "b"}},
	})
	got := expectedOutcomesForAcceptance(&types.BusContext{Mutable: mu})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("ExpectedOutcomes drift; got %v", got)
	}
}

// TestApplyAcceptanceVerdict_ErrorMarksUnverified pins commit
// 26 + 27: when the AcceptanceChecker returns a non-nil error,
// the phase becomes PhaseAcceptanceUnverified (terminal but
// distinct from Accepted), the AcceptanceCheck slot carries the
// error in the Reasoning field, and the helper returns
// (rejected=false, rejErr=nil) so the caller advances the
// group rather than aborting the Run on infra failure.
func TestApplyAcceptanceVerdict_ErrorMarksUnverified(t *testing.T) {
	store := &fakeGroupStore{}
	o := &Orchestrator{
		busCtx:         &types.BusContext{Mutable: types.NewMutableState("probe")},
		planGroupStore: store,
	}
	phase := &types.PhaseRecord{Index: 1, Goal: "update ORM", Status: types.PhaseVerified}
	group := &types.PlanGroup{
		ID:        "group-unverified-test",
		Status:    types.PlanGroupInFlight,
		ActiveIdx: 1,
		Phases:    []types.PhaseRecord{{}, *phase, {}},
	}

	rejected, rejErr := o.applyAcceptanceVerdict(phase, group,
		nil, errors.New("network blip: timeout after 5s"))

	if rejected {
		t.Errorf("rejected should be false on infra error; group should still advance")
	}
	if rejErr != nil {
		t.Errorf("rejErr should be nil on infra error; got %v", rejErr)
	}
	if phase.Status != types.PhaseAcceptanceUnverified {
		t.Errorf("phase.Status should be PhaseAcceptanceUnverified; got %q", phase.Status)
	}
	if phase.AcceptanceCheck == nil {
		t.Fatal("AcceptanceCheck should be populated with the infra failure")
	}
	if !strings.Contains(phase.AcceptanceCheck.Reasoning, "infra failure") {
		t.Errorf("Reasoning should mention infra failure; got %q", phase.AcceptanceCheck.Reasoning)
	}
	if !strings.Contains(phase.AcceptanceCheck.Reasoning, "network blip") {
		t.Errorf("Reasoning should carry the underlying error; got %q", phase.AcceptanceCheck.Reasoning)
	}
	if phase.FinishedAt == nil {
		t.Errorf("FinishedAt should be stamped")
	}
	if group.Status != types.PlanGroupInFlight {
		t.Errorf("group should remain InFlight on infra failure; got %q", group.Status)
	}
}

// TestApplyAcceptanceVerdict_RejectedRollsBack pins the
// rejection path: passed=false → phase RolledBack, group
// Failed, helper returns (rejected=true, error). Caller exits
// runPhaseGroup with this error; the group stops here.
func TestApplyAcceptanceVerdict_RejectedRollsBack(t *testing.T) {
	store := &fakeGroupStore{}
	o := &Orchestrator{
		busCtx:         &types.BusContext{Mutable: types.NewMutableState("probe")},
		planGroupStore: store,
	}
	phase := &types.PhaseRecord{Index: 0, Goal: "add migration", Status: types.PhaseVerified}
	group := &types.PlanGroup{
		ID:        "group-reject-test",
		Status:    types.PlanGroupInFlight,
		ActiveIdx: 0,
		Phases:    []types.PhaseRecord{*phase, {}, {}},
	}
	verdict := &types.AcceptanceCheck{
		Passed:    false,
		Reasoning: "plan summary unrelated to phase goal",
	}

	rejected, rejErr := o.applyAcceptanceVerdict(phase, group, verdict, nil)

	if !rejected {
		t.Errorf("rejected should be true on passed=false")
	}
	if rejErr == nil {
		t.Errorf("rejErr should describe the rejection; got nil")
	}
	if !strings.Contains(rejErr.Error(), "unrelated to phase goal") {
		t.Errorf("rejErr should embed the verdict reasoning; got %v", rejErr)
	}
	if phase.Status != types.PhaseRolledBack {
		t.Errorf("phase.Status should be RolledBack; got %q", phase.Status)
	}
	if group.Status != types.PlanGroupFailed {
		t.Errorf("group.Status should be Failed; got %q", group.Status)
	}
	if store.saveCount == 0 {
		t.Errorf("expected persistGroup to fire on rejection; got %d saves", store.saveCount)
	}
}

// TestApplyAcceptanceVerdict_AcceptedAdvances pins the happy
// path: passed=true → phase Accepted, NextHint propagated to
// o.nextPhaseHint, helper returns (false, nil) so caller
// advances.
func TestApplyAcceptanceVerdict_AcceptedAdvances(t *testing.T) {
	o := &Orchestrator{
		busCtx: &types.BusContext{Mutable: types.NewMutableState("probe")},
	}
	phase := &types.PhaseRecord{Index: 0, Goal: "add migration", Status: types.PhaseVerified}
	group := &types.PlanGroup{
		ID:        "group-accept-test",
		Status:    types.PlanGroupInFlight,
		ActiveIdx: 0,
		Phases:    []types.PhaseRecord{*phase, {}, {}},
	}
	verdict := &types.AcceptanceCheck{
		Passed:    true,
		Reasoning: "migration applied; schema test passes",
		NextHint:  "ORM needs to know about users.email",
	}

	rejected, rejErr := o.applyAcceptanceVerdict(phase, group, verdict, nil)

	if rejected {
		t.Errorf("rejected should be false on passed=true")
	}
	if rejErr != nil {
		t.Errorf("rejErr should be nil on accepted; got %v", rejErr)
	}
	if phase.Status != types.PhaseAccepted {
		t.Errorf("phase.Status should be Accepted; got %q", phase.Status)
	}
	if o.nextPhaseHint != "ORM needs to know about users.email" {
		t.Errorf("nextPhaseHint should be propagated; got %q", o.nextPhaseHint)
	}
}

// fakeGroupStore is a minimal test impl of the PlanGroupSaver
// interface — counts Save calls and stores the latest snapshot.
type fakeGroupStore struct {
	saveCount int
	last      *types.PlanGroup
}

func (f *fakeGroupStore) Save(g *types.PlanGroup) (string, error) {
	f.saveCount++
	f.last = g
	return "/fake/path/" + g.ID + ".json", nil
}
