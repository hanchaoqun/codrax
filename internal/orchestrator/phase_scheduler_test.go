package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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
func TestRunPhaseGroup_HappyPathThreePhases(t *testing.T) {
	store := &fakeGroupStore{}
	mu := types.NewMutableState("schema-then-code")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			RawRequest:       "schema then orm then api",
			ExpectedOutcomes: []string{"users.email lands cleanly"},
		},
	})
	bus := &types.BusContext{
		Mutable:    mu,
		Mode:       types.ModeApply,
		AnalysisIR: &types.AnalysisIR{},
	}
	o := &Orchestrator{
		busCtx:            bus,
		planGroupStore:    store,
		cancelToken:       NewCancelToken(),
		acceptanceChecker: &stubAcceptanceChecker{verdict: &types.AcceptanceCheck{Passed: true, Reasoning: "ok", NextHint: "carry forward"}},
	}

	// Stub runTaskPhase to simulate plan→apply→verify success
	// per phase. Each phase gets a unique plan id + commit SHA
	// so the assertions can confirm the right phase got the
	// right SHA.
	phaseCallNum := 0
	o.runTaskPhaseFn = func(stepsUsed *int) error {
		phaseCallNum++
		planID := fmt.Sprintf("plan-phase%d", phaseCallNum)
		commitSHA := fmt.Sprintf("sha%d000000000000abcd", phaseCallNum)
		mu.SetChangePlan(&types.ChangePlan{
			ID:          planID,
			Status:      types.PlanStatusApplied,
			Summary:     "phase " + strconv.Itoa(phaseCallNum) + " summary",
			TargetPaths: []string{"x.go"},
		})
		mu.SetChangeReport(&types.ChangeReport{
			PlanID:      planID,
			Passed:      true,
			TestResults: []types.TestResult{{AssertionID: "TestX", Passed: true}},
		})
		o.currentIterCommitSHA = commitSHA
		return nil
	}

	g := &types.PlanGroup{
		ID:        "group-e2e-test",
		Goal:      "schema-then-code",
		Decision:  "linear",
		Status:    types.PlanGroupPlanning,
		ActiveIdx: 0,
		Phases: []types.PhaseRecord{
			{Index: 0, Goal: "schema migration", Status: types.PhasePending},
			{Index: 1, Goal: "ORM update", Status: types.PhasePending},
			{Index: 2, Goal: "API surface", Status: types.PhasePending},
		},
	}
	stepsUsed := 0
	if err := o.runPhaseGroup(g, &stepsUsed); err != nil {
		t.Fatalf("runPhaseGroup: %v", err)
	}

	// Group end-state assertions.
	if g.Status != types.PlanGroupCompleted {
		t.Errorf("expected PlanGroupCompleted; got %q", g.Status)
	}
	if g.ActiveIdx != 3 {
		t.Errorf("ActiveIdx should advance past all phases; got %d", g.ActiveIdx)
	}
	if phaseCallNum != 3 {
		t.Errorf("expected runTaskPhase invoked 3 times; got %d", phaseCallNum)
	}

	// Per-phase assertions.
	for i, want := range []struct {
		planID    string
		shaPrefix string
	}{
		{"plan-phase1", "sha1"},
		{"plan-phase2", "sha2"},
		{"plan-phase3", "sha3"},
	} {
		p := g.Phases[i]
		if p.Status != types.PhaseAccepted {
			t.Errorf("phase[%d].Status = %q; want Accepted", i, p.Status)
		}
		if p.PlanID != want.planID {
			t.Errorf("phase[%d].PlanID = %q; want %q", i, p.PlanID, want.planID)
		}
		if !strings.HasPrefix(p.AppliedSHA, want.shaPrefix) {
			t.Errorf("phase[%d].AppliedSHA = %q; want prefix %q", i, p.AppliedSHA, want.shaPrefix)
		}
		if p.AcceptanceCheck == nil || !p.AcceptanceCheck.Passed {
			t.Errorf("phase[%d] acceptance should be passed; got %+v", i, p.AcceptanceCheck)
		}
		if p.StartedAt == nil || p.FinishedAt == nil {
			t.Errorf("phase[%d] timestamps should both be stamped", i)
		}
	}

	// nextPhaseHint must be consume-once: after phase 3
	// completes (last phase), the slot's last value was
	// "carry forward" — but the seedPlanningHintFromPhase
	// function consumed it on phase 3 entry, then phase 3's
	// acceptance set it again. Since there is no phase 4 to
	// consume, the slot remains "carry forward" at exit.
	// This is acceptable — the next Run's seedPlanningHintFromPhase
	// will consume it (and the cmd/root.go layer creates a
	// fresh Orchestrator per Run anyway).
	if o.nextPhaseHint != "carry forward" {
		t.Errorf("expected nextPhaseHint = 'carry forward' after final phase; got %q", o.nextPhaseHint)
	}

	// Persistence: at minimum N saves per phase (in_progress +
	// accepted) plus 1 final completion + 1 initial save = 8.
	// The exact count is implementation-defined; just confirm
	// it's non-trivial.
	if store.saveCount < 4 {
		t.Errorf("expected >= 4 persistGroup saves; got %d", store.saveCount)
	}
}

// TestRunPhaseGroup_AcceptanceErrorAdvances pins commit 26 +
// 27 in the e2e flow: when the AcceptanceChecker errors on a
// phase, the phase is marked PhaseAcceptanceUnverified and
// the scheduler still advances (does NOT abort the group).
// This is the fail-safer-not-fail-open contract.
func TestRunPhaseGroup_AcceptanceErrorAdvances(t *testing.T) {
	store := &fakeGroupStore{}
	mu := types.NewMutableState("test")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{}})
	bus := &types.BusContext{
		Mutable:    mu,
		Mode:       types.ModeApply,
		AnalysisIR: &types.AnalysisIR{},
	}
	o := &Orchestrator{
		busCtx:            bus,
		planGroupStore:    store,
		cancelToken:       NewCancelToken(),
		acceptanceChecker: &stubAcceptanceChecker{err: errors.New("network blip")},
	}
	o.runTaskPhaseFn = func(stepsUsed *int) error {
		mu.SetChangePlan(&types.ChangePlan{
			ID:          "plan-x",
			TargetPaths: []string{"x.go"},
		})
		mu.SetChangeReport(&types.ChangeReport{Passed: true})
		o.currentIterCommitSHA = "sha000000000000"
		return nil
	}

	g := &types.PlanGroup{
		ID:       "group-acc-err",
		Decision: "linear",
		Status:   types.PlanGroupPlanning,
		Phases: []types.PhaseRecord{
			{Index: 0, Goal: "p0", Status: types.PhasePending},
			{Index: 1, Goal: "p1", Status: types.PhasePending},
		},
	}
	stepsUsed := 0
	if err := o.runPhaseGroup(g, &stepsUsed); err != nil {
		t.Fatalf("runPhaseGroup: should not abort on acceptance err; got %v", err)
	}
	for i := range g.Phases {
		if g.Phases[i].Status != types.PhaseAcceptanceUnverified {
			t.Errorf("phase[%d] should be AcceptanceUnverified; got %q",
				i, g.Phases[i].Status)
		}
	}
	if g.Status != types.PlanGroupCompleted {
		t.Errorf("group should still complete despite acceptance err; got %q", g.Status)
	}
}

// TestRunPhaseGroup_AcceptanceRejectAborts pins the rejection
// path in the e2e flow: passed=false stops the group at the
// rejected phase with PlanGroupFailed; subsequent phases stay
// Pending.
func TestRunPhaseGroup_AcceptanceRejectAborts(t *testing.T) {
	store := &fakeGroupStore{}
	mu := types.NewMutableState("test")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{}})
	bus := &types.BusContext{
		Mutable:    mu,
		Mode:       types.ModeApply,
		AnalysisIR: &types.AnalysisIR{},
	}
	o := &Orchestrator{
		busCtx:            bus,
		planGroupStore:    store,
		cancelToken:       NewCancelToken(),
		acceptanceChecker: &stubAcceptanceChecker{verdict: &types.AcceptanceCheck{Passed: false, Reasoning: "wrong direction"}},
	}
	o.runTaskPhaseFn = func(stepsUsed *int) error {
		mu.SetChangePlan(&types.ChangePlan{ID: "plan-x", TargetPaths: []string{"x.go"}})
		mu.SetChangeReport(&types.ChangeReport{Passed: true})
		o.currentIterCommitSHA = "sha000000000000"
		return nil
	}

	g := &types.PlanGroup{
		ID:       "group-rej",
		Decision: "linear",
		Status:   types.PlanGroupPlanning,
		Phases: []types.PhaseRecord{
			{Index: 0, Goal: "p0", Status: types.PhasePending},
			{Index: 1, Goal: "p1", Status: types.PhasePending},
		},
	}
	stepsUsed := 0
	err := o.runPhaseGroup(g, &stepsUsed)
	if err == nil {
		t.Fatal("runPhaseGroup should error on acceptance reject")
	}
	if !strings.Contains(err.Error(), "wrong direction") {
		t.Errorf("error should embed verdict reasoning; got %v", err)
	}
	if g.Phases[0].Status != types.PhaseRolledBack {
		t.Errorf("phase[0] should be RolledBack; got %q", g.Phases[0].Status)
	}
	if g.Phases[1].Status != types.PhasePending {
		t.Errorf("phase[1] should remain Pending after group failure; got %q",
			g.Phases[1].Status)
	}
	if g.Status != types.PlanGroupFailed {
		t.Errorf("group should be Failed; got %q", g.Status)
	}
}

// TestRunPhaseGroup_CancelBetweenPhases pins commit 32's
// P0 #1: when the cancel token is set between phases, the
// runPhaseGroup outer loop unwinds with a CanceledError on
// the next iteration check rather than silently marching
// into the next phase. Pre-commit-32 the loop had zero
// checkCanceled() calls and a /cancel mid-phase only landed
// when the in-flight Chat call's context was cancelled —
// the loop would then happily start phase 2 anyway.
func TestRunPhaseGroup_CancelBetweenPhases(t *testing.T) {
	store := &fakeGroupStore{}
	mu := types.NewMutableState("cancel-test")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{}})
	bus := &types.BusContext{
		Mutable:    mu,
		Mode:       types.ModeApply,
		AnalysisIR: &types.AnalysisIR{},
	}
	tok := NewCancelToken()
	o := &Orchestrator{
		busCtx:            bus,
		planGroupStore:    store,
		cancelToken:       tok,
		acceptanceChecker: &stubAcceptanceChecker{verdict: &types.AcceptanceCheck{Passed: true, Reasoning: "ok"}},
	}
	// Stub runTaskPhase: phase 0 succeeds normally, then
	// triggers cancel before returning. Phase 1 should NOT
	// start.
	phase0Done := false
	o.runTaskPhaseFn = func(stepsUsed *int) error {
		mu.SetChangePlan(&types.ChangePlan{ID: "plan-x", TargetPaths: []string{"x.go"}})
		mu.SetChangeReport(&types.ChangeReport{Passed: true})
		o.currentIterCommitSHA = "sha000"
		if !phase0Done {
			phase0Done = true
			tok.Cancel("user pressed Ctrl+C")
		}
		return nil
	}

	g := &types.PlanGroup{
		ID:       "group-cancel-test",
		Decision: "linear",
		Status:   types.PlanGroupPlanning,
		Phases: []types.PhaseRecord{
			{Index: 0, Goal: "p0", Status: types.PhasePending},
			{Index: 1, Goal: "p1", Status: types.PhasePending},
		},
	}
	stepsUsed := 0
	err := o.runPhaseGroup(g, &stepsUsed)
	if err == nil {
		t.Fatal("expected CanceledError after cancel")
	}
	if _, ok := err.(*CanceledError); !ok {
		t.Errorf("expected *CanceledError; got %T: %v", err, err)
	}
	// Phase 0 ran (advanced); phase 1 must NOT have executed.
	if g.Phases[0].Status != types.PhaseAccepted {
		t.Errorf("phase 0 should be Accepted; got %q", g.Phases[0].Status)
	}
	if g.Phases[1].Status != types.PhasePending {
		t.Errorf("phase 1 should be untouched (Pending); got %q", g.Phases[1].Status)
	}
}

// TestRunPhaseGroup_StampsPhaseFieldsOnReport pins commit 32's
// P1 #3: the per-phase verify report gets PhaseGroupID +
// PhaseIndex stamped so /history can cluster reports by group.
// Without this stamp, multi-phase reports look like N unrelated
// single-phase tasks in the history listing.
func TestRunPhaseGroup_StampsPhaseFieldsOnReport(t *testing.T) {
	store := &fakeGroupStore{}
	mu := types.NewMutableState("test")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{}})
	bus := &types.BusContext{
		Mutable:    mu,
		Mode:       types.ModeApply,
		AnalysisIR: &types.AnalysisIR{},
	}
	o := &Orchestrator{
		busCtx:            bus,
		planGroupStore:    store,
		cancelToken:       NewCancelToken(),
		acceptanceChecker: &stubAcceptanceChecker{verdict: &types.AcceptanceCheck{Passed: true}},
	}
	var capturedReport *types.ChangeReport
	o.runTaskPhaseFn = func(stepsUsed *int) error {
		report := &types.ChangeReport{PlanID: "plan-x", Passed: true}
		mu.SetChangeReport(report)
		mu.SetChangePlan(&types.ChangePlan{ID: "plan-x", TargetPaths: []string{"x.go"}})
		o.currentIterCommitSHA = "sha000"
		capturedReport = report
		return nil
	}

	g := &types.PlanGroup{
		ID:       "group-stamp-test",
		Decision: "linear",
		Status:   types.PlanGroupPlanning,
		Phases:   []types.PhaseRecord{{Index: 1, Goal: "p1", Status: types.PhasePending}},
	}
	stepsUsed := 0
	if err := o.runPhaseGroup(g, &stepsUsed); err != nil {
		t.Fatalf("runPhaseGroup: %v", err)
	}
	if capturedReport == nil {
		t.Fatal("expected report to be captured")
	}
	if capturedReport.PhaseGroupID != "group-stamp-test" {
		t.Errorf("PhaseGroupID = %q; want group-stamp-test", capturedReport.PhaseGroupID)
	}
	if capturedReport.PhaseIndex != 1 {
		t.Errorf("PhaseIndex = %d; want 1", capturedReport.PhaseIndex)
	}
}

// TestRunPhaseGroup_RecordsRetryAttemptCount pins commit 32's
// P1 #4: phase.RetryAttempts reflects how many verify→plan
// retry cycles the phase consumed (read from the iteration
// ledger that clearForReplan appends to). Used by /phase show
// to surface "this phase was hard (3 retries)" vs "first try".
func TestRunPhaseGroup_RecordsRetryAttemptCount(t *testing.T) {
	store := &fakeGroupStore{}
	mu := types.NewMutableState("retry-test")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{}})
	bus := &types.BusContext{
		Mutable:    mu,
		Mode:       types.ModeApply,
		AnalysisIR: &types.AnalysisIR{},
	}
	o := &Orchestrator{
		busCtx:            bus,
		planGroupStore:    store,
		cancelToken:       NewCancelToken(),
		acceptanceChecker: &stubAcceptanceChecker{verdict: &types.AcceptanceCheck{Passed: true}},
	}
	o.runTaskPhaseFn = func(stepsUsed *int) error {
		mu.SetChangePlan(&types.ChangePlan{ID: "plan-x", TargetPaths: []string{"x.go"}})
		mu.SetChangeReport(&types.ChangeReport{Passed: true})
		// Simulate two retries by appending to the ledger
		// (clearForReplan would normally do this on each
		// verify SC failure).
		mu.AppendIteration(types.IterationRecord{Attempt: 1})
		mu.AppendIteration(types.IterationRecord{Attempt: 2})
		o.currentIterCommitSHA = "sha000"
		return nil
	}

	g := &types.PlanGroup{
		ID:       "group-retry-test",
		Decision: "linear",
		Status:   types.PlanGroupPlanning,
		Phases:   []types.PhaseRecord{{Index: 0, Goal: "p0", Status: types.PhasePending}},
	}
	stepsUsed := 0
	if err := o.runPhaseGroup(g, &stepsUsed); err != nil {
		t.Fatalf("runPhaseGroup: %v", err)
	}
	if g.Phases[0].RetryAttempts != 2 {
		t.Errorf("RetryAttempts = %d; want 2", g.Phases[0].RetryAttempts)
	}
}

// stubAcceptanceChecker is a deterministic AcceptanceChecker
// for tests: returns the configured (verdict, err) on every
// Check call regardless of input. Mirrors the no-op stubs in
// other test files.
type stubAcceptanceChecker struct {
	verdict *types.AcceptanceCheck
	err     error
}

func (s *stubAcceptanceChecker) Check(ctx context.Context, in AcceptanceCheckInput) (*types.AcceptanceCheck, error) {
	return s.verdict, s.err
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
