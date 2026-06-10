package repl

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// testWorktree is a minimal git fixture for /phase rollback
// tests: a fresh git repo with two commits at known SHAs.
type testWorktree struct {
	path     string
	firstSHA string
}

// initTestWorktree creates a git repo with two commits and
// returns the path + first commit's SHA. Callers wire firstSHA
// onto the previous phase's AppliedSHA so worktree.ResetHard
// has a real target. Skips the test when git is unavailable.
func initTestWorktree(t *testing.T) testWorktree {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "first")
	first, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package x\nvar V = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "second")
	return testWorktree{
		path:     dir,
		firstSHA: strings.TrimSpace(string(first)),
	}
}

// newPhaseTestREPL builds a minimal REPL fixture wired to a
// real PlanGroupStore + PlanStore so /phase commands can read
// + write group state on disk. interactive=false so info /
// warn / success write to r.out for capture.
func newPhaseTestREPL(t *testing.T) (*REPL, *PlanGroupStore, *PlanStore, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	planStore := NewPlanStore(dir)
	groupStore := NewPlanGroupStore(dir)
	out := &bytes.Buffer{}
	r := &REPL{
		runtimeAnchor:  t.TempDir(),
		out:            out,
		in:             strings.NewReader(""),
		colorMode:      render.ColorNever,
		language:       "en",
		planStore:      planStore,
		planGroupStore: groupStore,
		writeEnabled:   true,
	}
	return r, groupStore, planStore, out
}

func threePhaseTestGroup(id string) *types.PlanGroup {
	return &types.PlanGroup{
		ID:        id,
		Goal:      "schema-then-code",
		CreatedAt: time.Now(),
		Status:    types.PlanGroupInFlight,
		Decision:  "linear",
		ActiveIdx: 1,
		Phases: []types.PhaseRecord{
			{
				Index:      0,
				Goal:       "add migration",
				Status:     types.PhaseAccepted,
				PlanID:     "plan-phase0",
				AppliedSHA: "abc1234567890def",
				AcceptanceCheck: &types.AcceptanceCheck{
					Passed:    true,
					Reasoning: "migration applied; schema test passes",
					NextHint:  "ORM needs to know about users.email",
				},
			},
			{
				Index:  1,
				Goal:   "update ORM",
				Status: types.PhaseInProgress,
				PlanID: "plan-phase1",
			},
			{
				Index:  2,
				Goal:   "deprecate old column",
				Status: types.PhasePending,
			},
		},
	}
}


// TestHandlePhaseCmd_WriteDisabled pins the write-gated path:
// when codrax.yaml has write_enabled=false, /phase refuses
// before touching state. Mirrors the gate /approve and /merge
// already enforce.
func TestHandlePhaseCmd_WriteDisabled(t *testing.T) {
	out := &bytes.Buffer{}
	r := &REPL{
		out:          out,
		in:           strings.NewReader(""),
		colorMode:    render.ColorNever,
		language:     "en",
		writeEnabled: false,
	}
	r.handlePhaseCmd("/phase show")
	got := out.String()
	if !strings.Contains(got, "/phase") {
		t.Errorf("expected /phase mentioned in disabled message; got %q", got)
	}
	if !strings.Contains(got, "write") {
		t.Errorf("expected 'write' mentioned in disabled message; got %q", got)
	}
}

// TestHandlePhaseCmd_ShowEmpty pins the no-active-group path.
func TestHandlePhaseCmd_ShowEmpty(t *testing.T) {
	r, _, _, out := newPhaseTestREPL(t)
	r.handlePhaseCmd("/phase show")
	if !strings.Contains(out.String(), "no active plan group") {
		t.Errorf("expected 'no active plan group'; got %q", out.String())
	}
}

// TestHandlePhaseCmd_ShowRendersAllPhases pins the rendered
// content: header, status, every phase row, plan / sha
// cross-link, acceptance verdict.
func TestHandlePhaseCmd_ShowRendersAllPhases(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-show-test")
	if _, err := store.Save(g); err != nil {
		t.Fatalf("Save: %v", err)
	}
	r.handlePhaseCmd("/phase show")
	got := out.String()
	for _, want := range []string{
		"group: group-show-test",
		"status: in_flight",
		"phases: 3 total",
		"active phase 2",
		"add migration",
		"update ORM",
		"deprecate old column",
		"plan: plan-phase0",
		"sha: abc12345",
		"acceptance: passed",
		"migration applied",
		"next-hint:",
		"ORM needs to know",
		"→ [2]",                 // active phase marker
		"(accepted)",            // phase 1 status rendered
		"(in_progress)",         // phase 2 status
		"(pending)",             // phase 3 status
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered output missing %q\n--- full output ---\n%s", want, got)
		}
	}
}

// TestHandlePhaseCmd_ShowExplicitID pins the path that loads
// a specific group by ID.
func TestHandlePhaseCmd_ShowExplicitID(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	if _, err := store.Save(threePhaseTestGroup("group-A")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(threePhaseTestGroup("group-B")); err != nil {
		t.Fatal(err)
	}
	r.handlePhaseCmd("/phase show group-A")
	got := out.String()
	if !strings.Contains(got, "group: group-A") {
		t.Errorf("expected explicit group; got %q", got)
	}
	if strings.Contains(got, "group: group-B") {
		t.Errorf("explicit ID should not load group-B; got %q", got)
	}
}


// TestHandlePhaseCmd_UnknownSubcommand pins the catch-all warn.
func TestHandlePhaseCmd_UnknownSubcommand(t *testing.T) {
	r, _, _, out := newPhaseTestREPL(t)
	r.handlePhaseCmd("/phase blarg")
	if !strings.Contains(out.String(), "unknown") {
		t.Errorf("expected 'unknown' warn; got %q", out.String())
	}
}


// TestHandlePhaseCmd_ShowRendersWorktreePath pins P2 #3 — the
// worktree path appears in /phase show output when any phase
// plan has a non-empty WorktreePath.
func TestHandlePhaseCmd_ShowRendersWorktreePath(t *testing.T) {
	r, store, planStore, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-show-wt")
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	wtPath := t.TempDir()
	plan := &types.ChangePlan{
		ID:           "plan-phase0",
		Status:       types.PlanStatusApplied,
		CreatedAt:    time.Now(),
		WorktreePath: wtPath,
		TargetPaths:  []string{"x.go"},
		Changes:      []types.FileChange{{Path: "x.go", Kind: "create"}},
	}
	if _, err := planStore.Save(plan); err != nil {
		t.Fatal(err)
	}
	r.handlePhaseCmd("/phase show")
	got := out.String()
	if !strings.Contains(got, "worktree:") {
		t.Errorf("expected 'worktree:' header; got %q", got)
	}
	if !strings.Contains(got, wtPath) {
		t.Errorf("expected worktree path %q in output; got %q", wtPath, got)
	}
}

// TestHandlePhaseCmd_ShowWithPlanIDHints pins P2 #5 — passing a
// "plan-" prefixed id to /phase show points the operator at
// /plan show instead of failing with a generic "not found".
func TestHandlePhaseCmd_ShowWithPlanIDHints(t *testing.T) {
	r, _, _, out := newPhaseTestREPL(t)
	r.handlePhaseCmd("/phase show plan-1234567")
	got := out.String()
	if !strings.Contains(got, "/plan show") {
		t.Errorf("expected hint pointing at /plan show; got %q", got)
	}
	if !strings.Contains(got, "plan-1234567") {
		t.Errorf("expected the supplied id to be echoed; got %q", got)
	}
}


// TestHandlePhaseCmd_ShowFlagsOrphanedPhase pins commit 29:
// when a phase is in_progress with an OwnerPID that the
// liveness probe says is dead, /phase show tags it as
// "ORPHANED" so the operator sees a recoverable state and
// knows to run /phase rollback.
func TestHandlePhaseCmd_ShowFlagsOrphanedPhase(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-orphan-test")
	now := time.Now()
	g.Phases[1].Status = types.PhaseInProgress
	g.Phases[1].OwnerPID = 999999
	g.Phases[1].StartedAt = &now
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	// Stub liveness probe to "always dead" so the orphan tag
	// fires deterministically. Restore the production probe
	// after the test so other cases see the real probe.
	saved := phaseLivenessProbe
	phaseLivenessProbe = func(pid int) bool { return false }
	defer func() { phaseLivenessProbe = saved }()

	r.handlePhaseCmd("/phase show")
	got := out.String()
	if !strings.Contains(got, "ORPHANED") {
		t.Errorf("expected ORPHANED tag on stale in_progress phase; got %q", got)
	}
	if !strings.Contains(got, "999999") {
		t.Errorf("expected the orphaned pid to be echoed; got %q", got)
	}
}

// TestHandlePhaseCmd_ShowOrphanRecoveryHint pins commit 41
// UX#2: when /phase show flags an ORPHANED phase, it ALSO
// surfaces an explicit recovery-action hint pointing at
// /phase rollback + /mode apply. Pre-commit-41 the tag
// rendered without next steps, leaving the operator to
// guess.
func TestHandlePhaseCmd_ShowOrphanRecoveryHint(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-orphan-hint")
	now := time.Now()
	g.Phases[1].Status = types.PhaseInProgress
	g.Phases[1].OwnerPID = 999999
	g.Phases[1].StartedAt = &now
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	saved := phaseLivenessProbe
	phaseLivenessProbe = func(pid int) bool { return false }
	defer func() { phaseLivenessProbe = saved }()

	r.handlePhaseCmd("/phase show")
	got := out.String()
	if !strings.Contains(got, "ORPHANED") {
		t.Errorf("expected ORPHANED tag; got %q", got)
	}
	if !strings.Contains(got, "/phase rollback") {
		t.Errorf("expected /phase rollback recovery hint; got %q", got)
	}
	if !strings.Contains(got, "/mode apply") {
		t.Errorf("expected /mode apply recovery hint; got %q", got)
	}
}

// TestHandlePhaseCmd_ShowDoesNotFlagLivePhase pins the
// negative case — when the liveness probe says the pid IS
// alive, no orphan tag is emitted.
func TestHandlePhaseCmd_ShowDoesNotFlagLivePhase(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-alive-test")
	now := time.Now()
	g.Phases[1].Status = types.PhaseInProgress
	g.Phases[1].OwnerPID = 12345
	g.Phases[1].StartedAt = &now
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	saved := phaseLivenessProbe
	phaseLivenessProbe = func(pid int) bool { return true }
	defer func() { phaseLivenessProbe = saved }()

	r.handlePhaseCmd("/phase show")
	got := out.String()
	if strings.Contains(got, "ORPHANED") {
		t.Errorf("live phase should not be tagged ORPHANED; got %q", got)
	}
}

// TestHandlePhaseCmd_ShowRendersAcceptanceUnverified pins
// commit 26: when a phase carries the new
// PhaseAcceptanceUnverified status (LLM acceptance check
// errored), /phase show surfaces the status string + the
// recorded reasoning so the operator sees the gap rather
// than mistaking it for a clean Accepted.
//
// Commit 33 extension: the rendering also surfaces a
// "→ /phase next or /phase rollback" recovery-action hint
// so the operator isn't left wondering what to do next.
func TestHandlePhaseCmd_ShowRendersAcceptanceUnverified(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-unverified")
	g.Phases[0].Status = types.PhaseAcceptanceUnverified
	g.Phases[0].AcceptanceCheck = &types.AcceptanceCheck{
		Passed:    false,
		Reasoning: "acceptance_check infra failure: timeout after 5s",
	}
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	r.handlePhaseCmd("/phase show")
	got := out.String()
	if !strings.Contains(got, "acceptance_unverified") {
		t.Errorf("expected acceptance_unverified status; got %q", got)
	}
	if !strings.Contains(got, "infra failure") {
		t.Errorf("expected reasoning text; got %q", got)
	}
	// Commit 33: action hint pointing at the two recovery
	// levers so the operator knows what to do.
	if !strings.Contains(got, "/phase next") {
		t.Errorf("expected /phase next recovery hint; got %q", got)
	}
	if !strings.Contains(got, "/phase rollback") {
		t.Errorf("expected /phase rollback recovery hint; got %q", got)
	}
}

// TestPhaseTotalForGroup pins the helper used by /plan show
// + /plan list to render "phase X of Y" cross-links. Existing
// group → returns total phase count; missing group → 0.
func TestPhaseTotalForGroup(t *testing.T) {
	r, store, _, _ := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-total-test")
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	if got := r.phaseTotalForGroup("group-total-test"); got != 3 {
		t.Errorf("expected 3 phases; got %d", got)
	}
	if got := r.phaseTotalForGroup("group-missing"); got != 0 {
		t.Errorf("missing group should return 0; got %d", got)
	}
	if got := r.phaseTotalForGroup(""); got != 0 {
		t.Errorf("empty id should return 0; got %d", got)
	}
}

// TestShortSHA pins the small util.
func TestShortSHA(t *testing.T) {
	cases := map[string]string{
		"abc1234567890def": "abc12345",
		"abc123":           "abc123",
		"":                 "",
		"  abc12345  ":     "abc12345",
	}
	for in, want := range cases {
		if got := shortSHA(in); got != want {
			t.Errorf("shortSHA(%q) = %q; want %q", in, got, want)
		}
	}
}


// The PlanGroup execution-queue verbs were retired with the scheduler lane.
// Every former mutation verb now explains the retirement, points at the live
// workflow surface, and leaves the legacy group bytes UNTOUCHED — honoring
// them would queue work nothing drains.
func TestHandlePhaseCmd_RetiredVerbsExplainAndDoNotMutate(t *testing.T) {
	for _, verb := range []string{"next", "rollback", "resume", "skip 2"} {
		t.Run(verb, func(t *testing.T) {
			r, store, _, out := newPhaseTestREPL(t)
			g := threePhaseTestGroup("group-retired-" + strings.Fields(verb)[0])
			if _, err := store.Save(g); err != nil {
				t.Fatalf("Save: %v", err)
			}
			before, err := store.Load(g.ID)
			if err != nil {
				t.Fatalf("Load before: %v", err)
			}
			r.handlePhaseCmd("/phase " + verb)
			got := out.String()
			if !strings.Contains(got, "retired") {
				t.Fatalf("verb %q must explain the retirement, got %q", verb, got)
			}
			if !strings.Contains(got, "/workflow") {
				t.Fatalf("verb %q must point at the live workflow surface, got %q", verb, got)
			}
			after, err := store.Load(g.ID)
			if err != nil {
				t.Fatalf("Load after: %v", err)
			}
			if after.Status != before.Status || after.ActiveIdx != before.ActiveIdx {
				t.Fatalf("retired verb %q mutated the legacy group: before %+v after %+v", verb, before, after)
			}
			for i := range before.Phases {
				if after.Phases[i].Status != before.Phases[i].Status {
					t.Fatalf("retired verb %q mutated phase %d status", verb, i)
				}
			}
		})
	}
}

// Bare /phase with no group store and no workflow store degrades to a clear
// no-data message instead of a hard "disabled" wall.
func TestHandlePhaseCmd_NoStoresShowsNoData(t *testing.T) {
	out := &bytes.Buffer{}
	r := &REPL{
		out:          out,
		in:           strings.NewReader(""),
		colorMode:    render.ColorNever,
		language:     "en",
		writeEnabled: true,
	}
	r.handlePhaseCmd("/phase show")
	if !strings.Contains(out.String(), "no legacy plan group store") {
		t.Errorf("expected no-data message; got %q", out.String())
	}
}

// Legacy groups render read-only with the retirement banner.
func TestHandlePhaseCmd_LegacyShowCarriesBanner(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-banner")
	if _, err := store.Save(g); err != nil {
		t.Fatalf("Save: %v", err)
	}
	r.handlePhaseCmd("/phase show")
	got := out.String()
	if !strings.Contains(got, "read-only") || !strings.Contains(got, "retired") {
		t.Fatalf("legacy view must carry the retirement banner, got %q", got)
	}
	if !strings.Contains(got, "group: group-banner") {
		t.Fatalf("legacy view must still render the group, got %q", got)
	}
}

// With an active write-workflow run on disk, bare /phase shows the batch
// view (the live phases) instead of any legacy group.
func TestHandlePhaseCmd_ActiveWorkflowRunWinsOverLegacy(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-shadowed")
	if _, err := store.Save(g); err != nil {
		t.Fatalf("Save: %v", err)
	}
	wfStore := NewWriteWorkflowRunStore(t.TempDir())
	r.writeWorkflowRunStore = wfStore
	run := &types.WriteWorkflowRun{
		RunID:         "wf-live-1",
		Status:        types.WriteWorkflowRunInProgress,
		Goal:          "live workflow goal",
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Goal:   "first live batch",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}
	if _, err := wfStore.Save(run); err != nil {
		t.Fatalf("workflow save: %v", err)
	}
	r.handlePhaseCmd("/phase")
	got := out.String()
	if !strings.Contains(got, "wf-live-1") && !strings.Contains(got, "first live batch") {
		t.Fatalf("active run must render the batch view, got %q", got)
	}
	if strings.Contains(got, "group-shadowed") {
		t.Fatalf("legacy group must not shadow the live run, got %q", got)
	}
}
