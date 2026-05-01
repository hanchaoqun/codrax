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

// TestHandlePhaseCmd_NoStoreDisables pins the no-store path:
// /phase commands surface a clear "disabled" warn rather than
// crashing. writeEnabled=true so the upstream write-gate
// passes and the no-store branch fires.
func TestHandlePhaseCmd_NoStoreDisables(t *testing.T) {
	out := &bytes.Buffer{}
	r := &REPL{
		out:          out,
		in:           strings.NewReader(""),
		colorMode:    render.ColorNever,
		language:     "en",
		writeEnabled: true,
	}
	r.handlePhaseCmd("/phase show")
	if !strings.Contains(out.String(), "disabled") {
		t.Errorf("expected 'disabled' warn; got %q", out.String())
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

// TestHandlePhaseCmd_Next advances ActiveIdx past the current
// phase and persists the change.
func TestHandlePhaseCmd_Next(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-next-test")
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	r.handlePhaseCmd("/phase next")
	got := out.String()
	if !strings.Contains(got, "advanced past phase 2") {
		t.Errorf("expected advancement message; got %q", got)
	}
	// Reload from disk to verify persistence.
	loaded, err := store.Load("group-next-test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ActiveIdx != 2 {
		t.Errorf("ActiveIdx persistence drift; got %d", loaded.ActiveIdx)
	}
	if loaded.Phases[1].Status != types.PhaseAccepted {
		t.Errorf("phase 2 should be Accepted; got %q", loaded.Phases[1].Status)
	}
}

// TestHandlePhaseCmd_NextSkipsTerminalGroups pins the
// active-group lookup behaviour: a terminal group is invisible
// to FindActiveGroup, so /phase next (which uses
// FindActiveGroup) surfaces "no active plan group" rather than
// touching the completed group. This is the right behaviour —
// terminal groups should not be advanceable through the
// operator's default `/phase next` invocation.
func TestHandlePhaseCmd_NextSkipsTerminalGroups(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-done")
	g.Status = types.PlanGroupCompleted
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	r.handlePhaseCmd("/phase next")
	if !strings.Contains(out.String(), "no active plan group") {
		t.Errorf("expected 'no active plan group'; got %q", out.String())
	}
}

// TestHandlePhaseCmd_RollbackFirstPhaseRefuses pins the
// red-line guard: rolling back phase 1 has no prior SHA to
// rewind to; refuse and point at /reject.
func TestHandlePhaseCmd_RollbackFirstPhaseRefuses(t *testing.T) {
	r, store, planStore, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-rollback-first")
	g.ActiveIdx = 0
	g.Phases[0].Status = types.PhaseInProgress
	g.Phases[0].PlanID = "plan-phase0"
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	// Seed a plan with WorktreePath so findGroupWorktree returns
	// non-empty (otherwise the earlier "no worktree" branch
	// fires and shadows the "phase 1 no prior SHA" branch).
	plan := &types.ChangePlan{
		ID:           "plan-phase0",
		Status:       types.PlanStatusApplied,
		CreatedAt:    time.Now(),
		WorktreePath: t.TempDir(),
		TargetPaths:  []string{"x.go"},
		Changes:      []types.FileChange{{Path: "x.go", Kind: "create"}},
	}
	if _, err := planStore.Save(plan); err != nil {
		t.Fatal(err)
	}
	r.handlePhaseCmd("/phase rollback")
	got := out.String()
	if !strings.Contains(got, "cannot roll back phase 1") {
		t.Errorf("expected first-phase refusal; got %q", got)
	}
	if !strings.Contains(got, "/reject") {
		t.Errorf("expected /reject hint; got %q", got)
	}
}

// TestHandlePhaseCmd_Skip marks a phase Skipped and persists.
func TestHandlePhaseCmd_Skip(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-skip-test")
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	r.handlePhaseCmd("/phase skip 3")
	if !strings.Contains(out.String(), "phase 3 marked skipped") {
		t.Errorf("expected skip success; got %q", out.String())
	}
	loaded, _ := store.Load("group-skip-test")
	if loaded.Phases[2].Status != types.PhaseSkipped {
		t.Errorf("phase 3 should be Skipped; got %q", loaded.Phases[2].Status)
	}
}

// TestHandlePhaseCmd_SkipOutOfRange pins the index-bounds
// guard.
func TestHandlePhaseCmd_SkipOutOfRange(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-skip-oob")
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	r.handlePhaseCmd("/phase skip 99")
	if !strings.Contains(out.String(), "out of range") {
		t.Errorf("expected out-of-range error; got %q", out.String())
	}
}

// TestHandlePhaseCmd_SkipNonNumeric pins the integer-parse
// guard.
func TestHandlePhaseCmd_SkipNonNumeric(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-skip-bad")
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	r.handlePhaseCmd("/phase skip abc")
	if !strings.Contains(out.String(), "not a number") {
		t.Errorf("expected non-numeric error; got %q", out.String())
	}
}

// TestHandlePhaseCmd_SkipTerminalPhaseRefuses pins the
// already-terminal guard.
func TestHandlePhaseCmd_SkipTerminalPhaseRefuses(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-skip-terminal")
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	// Phase 1 (index 0) is already PhaseAccepted in the fixture.
	r.handlePhaseCmd("/phase skip 1")
	if !strings.Contains(out.String(), "already terminal") {
		t.Errorf("expected terminal refusal; got %q", out.String())
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

// TestHandlePhaseCmd_RollbackResetsPhaseToPending pins the
// resumability contract: after rollback succeeds the active
// phase is PhasePending and the group is PlanGroupInFlight, so
// the scheduler can re-enter that phase on the next /mode apply
// instead of the previous deadlock where both became terminal.
func TestHandlePhaseCmd_RollbackResetsPhaseToPending(t *testing.T) {
	r, store, planStore, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-rollback-resume")
	// Active phase = 1 (index 1, "update ORM"). Phase 0 has
	// AppliedSHA=abc1234567890def from the fixture so rollback
	// has a target.
	g.Phases[1].Status = types.PhaseInProgress
	g.Phases[1].PlanID = "plan-phase1"
	g.Phases[1].AppliedSHA = "deadbeef00000000"
	now := time.Now()
	g.Phases[1].StartedAt = &now
	g.Phases[1].AcceptanceCheck = &types.AcceptanceCheck{Passed: false, Reasoning: "stale"}
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	// Seed a real git worktree so worktree.ResetHard succeeds —
	// init the dir, seed a file, two commits at SHAs we know.
	wt := initTestWorktree(t)
	plan := &types.ChangePlan{
		ID:           "plan-phase1",
		Status:       types.PlanStatusApplied,
		CreatedAt:    time.Now(),
		WorktreePath: wt.path,
		TargetPaths:  []string{"x.go"},
		Changes:      []types.FileChange{{Path: "x.go", Kind: "create"}},
	}
	if _, err := planStore.Save(plan); err != nil {
		t.Fatal(err)
	}
	// The fixture's prev.AppliedSHA (phase 0) is the literal
	// "abc1234567890def" string, which is not a real git SHA in
	// the seeded worktree. To actually exercise the success path
	// we patch it to the worktree's real first-commit SHA.
	g.Phases[0].AppliedSHA = wt.firstSHA
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}

	r.handlePhaseCmd("/phase rollback")
	got := out.String()
	if !strings.Contains(got, "reset to pending") {
		t.Errorf("expected 'reset to pending' wording; got %q", got)
	}
	if !strings.Contains(got, "replay with /mode apply") {
		t.Errorf("expected replay hint; got %q", got)
	}
	loaded, err := store.Load("group-rollback-resume")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Phases[1].Status != types.PhasePending {
		t.Errorf("phase 2 should be Pending; got %q", loaded.Phases[1].Status)
	}
	if loaded.Status != types.PlanGroupInFlight {
		t.Errorf("group should be InFlight; got %q", loaded.Status)
	}
	if loaded.Phases[1].PlanID != "" {
		t.Errorf("PlanID should be cleared on rollback; got %q", loaded.Phases[1].PlanID)
	}
	if loaded.Phases[1].AppliedSHA != "" {
		t.Errorf("AppliedSHA should be cleared on rollback; got %q", loaded.Phases[1].AppliedSHA)
	}
	if loaded.Phases[1].AcceptanceCheck != nil {
		t.Errorf("AcceptanceCheck should be cleared on rollback")
	}
	if loaded.Phases[1].StartedAt != nil || loaded.Phases[1].FinishedAt != nil {
		t.Errorf("phase timestamps should be cleared")
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

// TestHandlePhaseCmd_ResumeOnPendingPhase pins commit 25's
// /phase resume info-only hint: when the active phase is
// Pending (typically just after /phase rollback), surface the
// "type /mode apply to drive plan→apply→verify" guidance.
func TestHandlePhaseCmd_ResumeOnPendingPhase(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-resume-pending")
	g.Phases[1].Status = types.PhasePending
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	r.handlePhaseCmd("/phase resume")
	got := out.String()
	if !strings.Contains(got, "/mode apply") {
		t.Errorf("expected /mode apply hint; got %q", got)
	}
	if !strings.Contains(got, "phase 2") {
		t.Errorf("expected phase 2 mentioned; got %q", got)
	}
}

// TestHandlePhaseCmd_ResumeOnTerminalGroupRefuses pins the
// guard: completed / failed / rolled-back groups cannot be
// resumed; the operator gets a clear "nothing to resume" warn.
func TestHandlePhaseCmd_ResumeOnTerminalGroupRefuses(t *testing.T) {
	r, store, _, out := newPhaseTestREPL(t)
	g := threePhaseTestGroup("group-resume-done")
	g.Status = types.PlanGroupCompleted
	if _, err := store.Save(g); err != nil {
		t.Fatal(err)
	}
	r.handlePhaseCmd("/phase resume")
	got := out.String()
	if !strings.Contains(got, "no active plan group") && !strings.Contains(got, "nothing to resume") {
		t.Errorf("expected refusal; got %q", got)
	}
}

// TestHandlePhaseCmd_ShowRendersAcceptanceUnverified pins
// commit 26: when a phase carries the new
// PhaseAcceptanceUnverified status (LLM acceptance check
// errored), /phase show surfaces the status string + the
// recorded reasoning so the operator sees the gap rather
// than mistaking it for a clean Accepted.
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
