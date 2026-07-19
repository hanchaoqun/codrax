package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestApplyPostHookCheckpointCommitKeepsOnlyPlanOwnedPaths(t *testing.T) {
	mainRoot := filepath.Join(t.TempDir(), "main")
	if err := os.MkdirAll(mainRoot, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = mainRoot
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "test@local")
	git("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(mainRoot, "owned.py"), []byte("VALUE = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "seed")

	sess, err := worktree.Create(filepath.Join(t.TempDir(), "wt"), mainRoot, "trace-owned-apply-test")
	if err != nil {
		t.Fatalf("worktree.Create: %v", err)
	}
	t.Cleanup(func() { _ = sess.Discard() })
	if err := os.WriteFile(filepath.Join(sess.Path(), "owned.py"), []byte("VALUE = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sess.Path(), "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sess.Path(), "build", "generated.c"), []byte("generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mu := types.NewMutableState("owned apply")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-owned",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"owned.py"},
		Changes: []types.FileChange{{
			Path: "owned.py",
			Kind: "patch",
		}},
	})
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:      mu,
		Mode:         types.ModeApply,
		WorktreePath: sess.Path(),
		MainRepoRoot: mainRoot,
	}}

	if err := applyPostHook(o, &agent.StageOutput{}); err != nil {
		t.Fatalf("applyPostHook: %v", err)
	}
	if strings.TrimSpace(o.currentIterCommitSHA) == "" {
		t.Fatal("applyPostHook did not create checkpoint commit")
	}
	patch, err := worktree.CaptureCommitPatch(sess.Path(), o.currentIterCommitSHA)
	if err != nil {
		t.Fatalf("CaptureCommitPatch: %v", err)
	}
	if !strings.Contains(patch, "diff --git a/owned.py b/owned.py") || !strings.Contains(patch, "+VALUE = 2") {
		t.Fatalf("checkpoint patch missing owned change:\n%s", patch)
	}
	if strings.Contains(patch, "build/generated.c") || strings.Contains(patch, "+generated") {
		t.Fatalf("checkpoint patch included unowned generated file:\n%s", patch)
	}
}

// gitCheckpointTestRepo seeds a main repo + worktree pair for the
// checkpoint-commit pins below.
func gitCheckpointTestRepo(t *testing.T, seedFiles map[string]string) (mainRoot string, sessPath string) {
	t.Helper()
	mainRoot = filepath.Join(t.TempDir(), "main")
	if err := os.MkdirAll(mainRoot, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = mainRoot
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "test@local")
	git("config", "user.name", "test")
	for name, content := range seedFiles {
		if err := os.WriteFile(filepath.Join(mainRoot, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("commit", "-q", "-m", "seed")
	sess, err := worktree.Create(filepath.Join(t.TempDir(), "wt"), mainRoot, "trace-checkpoint-"+strings.ReplaceAll(t.Name(), "/", "_"))
	if err != nil {
		t.Fatalf("worktree.Create: %v", err)
	}
	t.Cleanup(func() { _ = sess.Discard() })
	return mainRoot, sess.Path()
}

// TestApplyPostHookCheckpointSurvivesGhostPlanPath is the witness-
// isomorphic pin for eval-audit 20260719 GAP-2 (zod_prefault_symptom
// run-1 log 48229:340): a multi-change plan whose unapplied ghost path
// ("check_prefault_schema.py" was planned but never created) must NOT
// abort the checkpoint commit. The applied implementation fix has to
// reach the durable recovery ref, and the typed ApplyCheckpointRecord
// must list the skipped ghost.
func TestApplyPostHookCheckpointSurvivesGhostPlanPath(t *testing.T) {
	mainRoot, wt := gitCheckpointTestRepo(t, map[string]string{"to-json-schema.ts": "truthiness\n"})
	if err := os.WriteFile(filepath.Join(wt, "to-json-schema.ts"), []byte("explicit undefined check\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("ghost path apply")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-ghost",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"to-json-schema.ts", "check_prefault_schema.py"},
		Changes: []types.FileChange{
			{Path: "to-json-schema.ts", Kind: "patch"},
			{Path: "check_prefault_schema.py", Kind: "create"},
		},
	})
	// Only the implementation fix actually applied (W1s active slice).
	mu.WriteClosure().MarkApplied("to-json-schema.ts")
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:      mu,
		Mode:         types.ModeApply,
		WorktreePath: wt,
		MainRepoRoot: mainRoot,
	}}
	if err := applyPostHook(o, &agent.StageOutput{}); err != nil {
		t.Fatalf("applyPostHook: %v", err)
	}
	if strings.TrimSpace(o.currentIterCommitSHA) == "" {
		t.Fatal("ghost plan path aborted the checkpoint commit (GAP-2 sick shape): no checkpoint SHA")
	}
	patch, err := worktree.CaptureCommitPatch(wt, o.currentIterCommitSHA)
	if err != nil {
		t.Fatalf("CaptureCommitPatch: %v", err)
	}
	if !strings.Contains(patch, "+explicit undefined check") {
		t.Fatalf("applied implementation fix missing from checkpoint (durable ref would ship without it):\n%s", patch)
	}
	// The recovery ref must resolve in the MAIN repo.
	ref := worktree.AppliedRef("plan-ghost")
	show := exec.Command("git", "cat-file", "-e", ref+"^{commit}")
	show.Dir = mainRoot
	if out, err := show.CombinedOutput(); err != nil {
		t.Fatalf("recovery ref %s missing from main repo: %v\n%s", ref, err, out)
	}
	plan := mu.ChangePlan()
	if plan.ApplyCheckpoint == nil {
		t.Fatal("typed ApplyCheckpointRecord missing from plan")
	}
	if plan.ApplyCheckpoint.DeliveryBroken() {
		t.Fatalf("healthy ghost-skip checkpoint must not read as broken: %+v", plan.ApplyCheckpoint)
	}
	if len(plan.ApplyCheckpoint.SkippedGhostPaths) != 1 || plan.ApplyCheckpoint.SkippedGhostPaths[0] != "check_prefault_schema.py" {
		t.Fatalf("SkippedGhostPaths = %v, want the planned-but-never-applied ghost", plan.ApplyCheckpoint.SkippedGhostPaths)
	}
	if plan.ApplyCheckpoint.RecoveryRef != ref {
		t.Fatalf("RecoveryRef = %q, want %q", plan.ApplyCheckpoint.RecoveryRef, ref)
	}
}

// TestApplyPostHookChainedPlansKeepEarlierFixInDurableRef pins the
// delivery-chain shape from zod run-1: plan-1 applies the implementation
// fix (with a ghost sibling), a replan's plan-2 applies only tests. The
// FINAL plan's recovery ref must materialize a tree containing BOTH the
// implementation fix and the tests — pre-fix the plan-1 checkpoint
// aborted and the final ref shipped red tests with no fix.
//
// Coverage-scope note (rework P3-④): this integration pin drives the
// chain through TWO sequential applyPostHook calls in ONE worktree,
// with the cumulative AppliedSet simulated via explicit MarkApplied
// calls. The cross-plan cumulative-AppliedSet selection arm itself
// (prior-plan paths self-healing into a later checkpoint, ghost
// exclusion, owned fallback) is load-borne by the unit pin
// TestWriteApplyCommitCheckpointPaths below — this test's job is the
// end-to-end git materialization of the chained refs.
func TestApplyPostHookChainedPlansKeepEarlierFixInDurableRef(t *testing.T) {
	mainRoot, wt := gitCheckpointTestRepo(t, map[string]string{"to-json-schema.ts": "truthiness\n"})
	mu := types.NewMutableState("chained plans")
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:      mu,
		Mode:         types.ModeApply,
		WorktreePath: wt,
		MainRepoRoot: mainRoot,
	}}
	// Plan 1: implementation fix applied, test file planned but not applied.
	if err := os.WriteFile(filepath.Join(wt, "to-json-schema.ts"), []byte("explicit undefined check\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-impl",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"to-json-schema.ts", "check_prefault_schema.py"},
		Changes: []types.FileChange{
			{Path: "to-json-schema.ts", Kind: "patch"},
			{Path: "check_prefault_schema.py", Kind: "create"},
		},
	})
	mu.WriteClosure().MarkApplied("to-json-schema.ts")
	if err := applyPostHook(o, &agent.StageOutput{}); err != nil {
		t.Fatalf("applyPostHook plan-1: %v", err)
	}
	// Replan: test-only plan-2 in the same worktree; AppliedSet is
	// cumulative (ResetExceptApplied semantics in the real flow).
	if err := os.WriteFile(filepath.Join(wt, "to-json-schema.test.ts"), []byte("falsy prefault regression tests\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-tests",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"to-json-schema.test.ts"},
		Changes: []types.FileChange{
			{Path: "to-json-schema.test.ts", Kind: "create"},
		},
	})
	mu.WriteClosure().MarkApplied("to-json-schema.test.ts")
	if err := applyPostHook(o, &agent.StageOutput{}); err != nil {
		t.Fatalf("applyPostHook plan-2: %v", err)
	}
	// Materialize the FINAL plan's ref from the main repo: both the
	// implementation fix and the tests must be present.
	ref := worktree.AppliedRef("plan-tests")
	showFile := func(path string) string {
		c := exec.Command("git", "show", ref+":"+path)
		c.Dir = mainRoot
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git show %s:%s: %v\n%s", ref, path, err, out)
		}
		return string(out)
	}
	if got := showFile("to-json-schema.ts"); !strings.Contains(got, "explicit undefined check") {
		t.Fatalf("final durable ref lost the earlier plan's implementation fix (delivery-chain break): %q", got)
	}
	if got := showFile("to-json-schema.test.ts"); !strings.Contains(got, "falsy prefault regression tests") {
		t.Fatalf("final durable ref missing the test plan's bytes: %q", got)
	}
	// Multi-plan session guidance: both refs listed in apply order.
	if len(o.appliedRecoveryRefs) != 2 ||
		o.appliedRecoveryRefs[0] != worktree.AppliedRef("plan-impl") ||
		o.appliedRecoveryRefs[1] != worktree.AppliedRef("plan-tests") {
		t.Fatalf("appliedRecoveryRefs = %v, want both refs in apply order", o.appliedRecoveryRefs)
	}
	result := mu.Result()
	if !strings.Contains(result, worktree.AppliedRef("plan-impl")) || !strings.Contains(result, worktree.AppliedRef("plan-tests")) {
		t.Fatalf("multi-plan landing guidance must list every applied ref; got:\n%s", result)
	}
}

// TestWriteApplyCommitCheckpointPaths pins the checkpoint path
// selection mechanics directly: the ACTUALLY-APPLIED set drives the
// commit (cumulative across plans — self-healing after an earlier
// failed checkpoint), declared-but-unapplied paths stay out, and the
// owned-path fallback covers lanes without an AppliedSet.
func TestWriteApplyCommitCheckpointPaths(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-paths",
		TargetPaths: []string{"impl.ts", "ghost.py"},
		Changes: []types.FileChange{
			{Path: "impl.ts", Kind: "patch"},
			{Path: "ghost.py", Kind: "create"},
		},
	}
	got := writeApplyCommitCheckpointPaths(plan, map[string]bool{"impl.ts": true})
	if len(got) != 1 || got[0] != "impl.ts" {
		t.Fatalf("applied-set selection = %v, want [impl.ts] (ghost excluded)", got)
	}
	// Cumulative applied set from a prior plan self-heals into this
	// plan's checkpoint even though the path is not plan-owned.
	got = writeApplyCommitCheckpointPaths(plan, map[string]bool{"impl.ts": true, "earlier/fix.go": true})
	if len(got) != 2 || got[0] != "earlier/fix.go" || got[1] != "impl.ts" {
		t.Fatalf("cumulative applied selection = %v, want sorted union [earlier/fix.go impl.ts]", got)
	}
	// Empty applied set falls back to the plan's declared owned paths.
	got = writeApplyCommitCheckpointPaths(plan, nil)
	if len(got) != 2 || got[0] != "impl.ts" || got[1] != "ghost.py" {
		t.Fatalf("owned fallback = %v, want plan-owned order [impl.ts ghost.py]", got)
	}
}

// TestApplyPostHookCheckpointCommitFailureIsTypedDisclosed pins the
// non-silent arm: when the checkpoint commit itself fails, the failure
// must land as a typed ApplyCheckpointRecord and the user-facing apply
// summary must withdraw the cherry-pick guidance instead of pointing at
// a ref that does not exist (pre-fix: WARN-only + confident guidance).
func TestApplyPostHookCheckpointCommitFailureIsTypedDisclosed(t *testing.T) {
	mainRoot, wt := gitCheckpointTestRepo(t, map[string]string{"owned.py": "VALUE = 1\n"})
	mu := types.NewMutableState("checkpoint failure")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-broken",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"owned.py"},
		Changes:     []types.FileChange{{Path: "owned.py", Kind: "patch"}},
	})
	// An unsafe pathspec in the applied set forces CommitChangesForPaths
	// into its typed error path deterministically.
	mu.WriteClosure().MarkApplied(":(top)owned.py")
	o := &Orchestrator{busCtx: &types.BusContext{
		Mutable:      mu,
		Mode:         types.ModeApply,
		WorktreePath: wt,
		MainRepoRoot: mainRoot,
	}}
	if err := applyPostHook(o, &agent.StageOutput{}); err != nil {
		t.Fatalf("applyPostHook: %v", err)
	}
	plan := mu.ChangePlan()
	if plan.ApplyCheckpoint == nil || !plan.ApplyCheckpoint.DeliveryBroken() {
		t.Fatalf("checkpoint failure must be recorded as a typed broken-delivery record; got %+v", plan.ApplyCheckpoint)
	}
	result := mu.Result()
	if !strings.Contains(result, "不可用") {
		t.Fatalf("apply summary must disclose the unusable recovery ref; got:\n%s", result)
	}
	if strings.Contains(result, "git cherry-pick refs/codrax/applied/plan-broken") {
		t.Fatalf("apply summary must not hand out cherry-pick guidance for a missing ref; got:\n%s", result)
	}
	if renderApplyCheckpointDisclosure(plan, "zh") == "" {
		t.Fatal("verify-face disclosure helper must render for a broken checkpoint")
	}
	if renderApplyCheckpointDisclosure(plan, "en") == "" {
		t.Fatal("verify-face disclosure helper must render for a broken checkpoint (en)")
	}
}

// TestRenderApplySummaryBrokenCheckpointGuidanceIsReachable pins the
// rework P2-2 wording contract for the broken-delivery apply summary:
//
//  1. The checkpoint is rebuilt by REPLAYING the apply (`/approve
//     --retry`) — never by /verify, which does not touch checkpoints
//     (pre-fix the guidance sent users down an unreachable path).
//  2. The worktree /merge channel may only be promised when the
//     worktree actually survives the Run (willPreserve=true); a
//     discarded worktree means the bytes are honestly unrecoverable.
func TestRenderApplySummaryBrokenCheckpointGuidanceIsReachable(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-broken-guidance",
		ApplyCheckpoint: &types.ApplyCheckpointRecord{
			CommitError: "fatal: unable to write new index file",
		},
		Changes: []types.FileChange{{Path: "owned.py", Kind: "patch"}},
	}
	ref := worktree.AppliedRef(plan.ID)
	applied := map[string]bool{"owned.py": true}
	for _, lang := range []string{"zh", "en"} {
		preserved := renderApplySummary(plan, applied, "/wt/x", ref, true, lang, nil)
		if !strings.Contains(preserved, "/merge") {
			t.Fatalf("[%s] preserved-worktree arm must offer the worktree /merge channel:\n%s", lang, preserved)
		}
		if !strings.Contains(preserved, "/approve --retry") {
			t.Fatalf("[%s] preserved arm must name the real checkpoint-rebuild path (/approve --retry):\n%s", lang, preserved)
		}
		if strings.Contains(preserved, "/verify") {
			t.Fatalf("[%s] /verify does not rebuild checkpoints — unreachable guidance:\n%s", lang, preserved)
		}
		discarded := renderApplySummary(plan, applied, "/wt/x", ref, false, lang, nil)
		if strings.Contains(discarded, "/merge") {
			t.Fatalf("[%s] discarded-worktree arm must not promise a destroyed worktree channel:\n%s", lang, discarded)
		}
		if !strings.Contains(discarded, "不可恢复") && !strings.Contains(discarded, "NOT recoverable") {
			t.Fatalf("[%s] discarded arm must state the bytes are unrecoverable:\n%s", lang, discarded)
		}
		if !strings.Contains(discarded, "/approve --retry") {
			t.Fatalf("[%s] discarded arm must point at the replay path:\n%s", lang, discarded)
		}
		if strings.Contains(discarded, "/verify") {
			t.Fatalf("[%s] discarded arm must not point at /verify either:\n%s", lang, discarded)
		}
		for _, out := range []string{preserved, discarded} {
			if strings.Contains(out, "git cherry-pick "+ref) {
				t.Fatalf("[%s] broken delivery must never hand out cherry-pick guidance:\n%s", lang, out)
			}
		}
	}
}

// TestRunResetsAppliedRecoveryRefsAcrossRuns is the rework P1-2 pin:
// the REPL keeps ONE Orchestrator across independent write tasks, so a
// new Run() must reset the session applied-ref list — a leaked ref from
// a previous Run (possibly already merged or rejected) would poison the
// second task's landing guidance with stale cherry-pick targets.
func TestRunResetsAppliedRecoveryRefsAcrossRuns(t *testing.T) {
	ir := dagIR(types.AnswerContract{Language: "en"})
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentVerifier: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, Error: "stub verify"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModeVerify)
	o.SetAutoInitRepo(true)
	o.SetScaffoldEnabled(true)
	// The exact state a previous multi-plan write Run leaves behind
	// (TestApplyPostHookChainedPlansKeepEarlierFixInDurableRef proves
	// applyPostHook populates this list).
	o.appliedRecoveryRefs = []string{
		worktree.AppliedRef("plan-old-impl"),
		worktree.AppliedRef("plan-old-tests"),
	}
	if _, err := o.Run("rerun verify", t.TempDir(), "main"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(o.appliedRecoveryRefs) != 0 {
		t.Fatalf("appliedRecoveryRefs must reset at Run entry (cross-Run leak); got %v", o.appliedRecoveryRefs)
	}
	// Second-Run shape: a fresh single-plan apply summary must list ONLY
	// the new plan's ref — zero stale refs, no multi-plan guidance line.
	mainRoot, wt := gitCheckpointTestRepo(t, map[string]string{"fresh.go": "package fresh\n"})
	mu := types.NewMutableState("second run apply")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-new",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"fresh.go"},
		Changes:     []types.FileChange{{Path: "fresh.go", Kind: "patch"}},
	})
	mu.WriteClosure().MarkApplied("fresh.go")
	o.busCtx = &types.BusContext{
		Mutable:      mu,
		Mode:         types.ModeApply,
		WorktreePath: wt,
		MainRepoRoot: mainRoot,
	}
	if err := os.WriteFile(filepath.Join(wt, "fresh.go"), []byte("package fresh // v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyPostHook(o, &agent.StageOutput{}); err != nil {
		t.Fatalf("applyPostHook: %v", err)
	}
	result := mu.Result()
	if strings.Contains(result, "plan-old-impl") || strings.Contains(result, "plan-old-tests") {
		t.Fatalf("second Run's landing guidance leaked a previous Run's refs:\n%s", result)
	}
	if strings.Contains(result, "本会话共落地") || strings.Contains(result, "This session landed") {
		t.Fatalf("single-plan second Run must not render multi-plan session guidance:\n%s", result)
	}
	if len(o.appliedRecoveryRefs) != 1 || o.appliedRecoveryRefs[0] != worktree.AppliedRef("plan-new") {
		t.Fatalf("second Run must track only its own ref; got %v", o.appliedRecoveryRefs)
	}
}

// TestRestoreBestIfRegressedKeepsCheckpointDisclosure is the rework
// P2-4 pin: restoreBestIfRegressed REPLACES Mutable.Result wholesale on
// both its arms, so a broken apply-checkpoint disclosure must be
// re-attached — the restored best plan's delivery ref being unusable
// must stay visible on the restore face.
func TestRestoreBestIfRegressedKeepsCheckpointDisclosure(t *testing.T) {
	brokenPlan := func(id string) *types.ChangePlan {
		return &types.ChangePlan{
			ID: id,
			ApplyCheckpoint: &types.ApplyCheckpointRecord{
				RecoveryRef: "refs/codrax/applied/" + id,
				CommitError: "fatal: unable to write new index file",
			},
		}
	}
	// Arm 1: restored best PASSED (success banner arm).
	mu := types.NewMutableState("restore disclosure pass")
	mu.SetChangePlan(&types.ChangePlan{ID: "plan-cur"})
	mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-cur", Passed: false,
		TestResults: []types.TestResult{{Passed: false}}})
	mu.SetBestPlanReport(brokenPlan("plan-best-pass"), &types.ChangeReport{
		PlanID: "plan-best-pass", Passed: true,
		TestResults: []types.TestResult{{Passed: true}}})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "zh"}}
	restoreBestIfRegressed(o)
	result := mu.Result()
	if !strings.Contains(result, "refs/codrax/applied/plan-best-pass") || !strings.Contains(result, "不可用") {
		t.Fatalf("restore success arm dropped the broken-checkpoint disclosure:\n%s", result)
	}
	// Arm 2: restored best still failing (failure summary arm).
	mu = types.NewMutableState("restore disclosure fail")
	mu.SetChangePlan(&types.ChangePlan{ID: "plan-cur"})
	mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-cur", Passed: false,
		TestResults: []types.TestResult{{Passed: false}}})
	mu.SetBestPlanReport(brokenPlan("plan-best-fail"), &types.ChangeReport{
		PlanID: "plan-best-fail", Passed: false,
		TestResults: []types.TestResult{{Passed: true}, {Passed: false}}})
	o = &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "en"}}
	restoreBestIfRegressed(o)
	result = mu.Result()
	if !strings.Contains(result, "refs/codrax/applied/plan-best-fail") || !strings.Contains(result, "not usable") {
		t.Fatalf("restore failure arm dropped the broken-checkpoint disclosure:\n%s", result)
	}
}

// A failed verify attempt must leave durable evidence on disk before any
// cleanup can run: the typed report JSON plus the applied-commit patch, with
// the diff artifact ref attached to the batch's verify attempt record.
func TestPersistVerifyFailureEvidence_WritesReportAndDiff(t *testing.T) {
	mainRoot := filepath.Join(t.TempDir(), "main")
	if err := os.MkdirAll(mainRoot, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = mainRoot
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "test@local")
	git("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(mainRoot, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "seed")

	sess, err := worktree.Create(filepath.Join(t.TempDir(), "wt"), mainRoot, "trace-evidence-test")
	if err != nil {
		t.Fatalf("worktree.Create: %v", err)
	}
	t.Cleanup(func() { _ = sess.Discard() })
	if err := os.WriteFile(filepath.Join(sess.Path(), "seed.txt"), []byte("changed by plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, err := worktree.CommitChanges(sess.Path(), "codrax apply iter (plan=plan-evidence)")
	if err != nil {
		t.Fatalf("CommitChanges: %v", err)
	}

	planDir := t.TempDir()
	mu := types.NewMutableState("evidence")
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, WorktreePath: sess.Path(), MainRepoRoot: mainRoot}
	o.reportDir = planDir
	o.currentIterCommitSHA = sha

	run := types.WriteWorkflowRun{
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-1",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", PlanID: "plan-evidence", ReportID: "plan-evidence.report.json"},
			},
		}},
	}
	report := &types.ChangeReport{PlanID: "plan-evidence", Passed: false, FailureSummary: "red"}
	o.persistVerifyFailureEvidence(&run, report, 1)

	reportPath := filepath.Join(planDir, "plan-evidence.report.json")
	loaded := readChangeReportFile(t, reportPath)
	if loaded.GeneratedAt.IsZero() {
		t.Fatal("persisted report must carry a non-zero generated_at")
	}
	diffPath := filepath.Join(planDir, "plan-evidence.attempt-1.diff")
	diffBytes, err := os.ReadFile(diffPath)
	if err != nil {
		t.Fatalf("attempt diff not persisted: %v", err)
	}
	if !strings.Contains(string(diffBytes), "changed by plan") {
		t.Fatalf("attempt diff should contain the applied change, got:\n%s", string(diffBytes))
	}
	attempt := run.Batches[0].Attempts[0]
	if attempt.ArtifactRef != "plan-evidence.attempt-1.diff" {
		t.Fatalf("verify attempt ArtifactRef should point at the diff, got %q", attempt.ArtifactRef)
	}
}

// The controller workflow's verify-failure branch must persist the report
// even when stage hooks are bypassed — the durable artifact chain cannot
// depend on a single save site (the missing-report eval class).
func TestRunWriteControllerWorkflow_VerifyFailurePersistsReportArtifact(t *testing.T) {
	store := &fakeWorkflowRunStore{}
	planDir := t.TempDir()
	mu := types.NewMutableState("persist failure report")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: "persist failure report"}}})
	decisions := []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "batch-1", Goal: "attempt"}},
		{Action: writeflow.ActionApplyPlan, ReasonCode: "ready"},
		{Action: writeflow.ActionVerifyBatch, ReasonCode: "applied"},
		{Action: writeflow.ActionBlock, ReasonCode: "post_apply_failed"},
	}
	controllerCalls := 0
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: scriptedController(t, decisions, &controllerCalls),
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	o.reportDir = planDir
	o.SetWriteRetryBudget(3)
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		switch stage {
		case types.StagePlan:
			mu.SetChangePlan(&types.ChangePlan{ID: "plan-durable", Status: types.PlanStatusPending, Summary: "attempt", TargetPaths: []string{"fix.go"}})
		case types.StageVerify:
			mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-durable", Passed: false, FailureSummary: "still red"})
		}
		*stepsUsed++
		return &agent.StageOutput{}, nil
	}
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err == nil {
		t.Fatal("runWriteControllerWorkflow should block after persisted verify failure")
	}
	reportPath := filepath.Join(planDir, "plan-durable.report.json")
	loaded := readChangeReportFile(t, reportPath)
	if loaded.Passed {
		t.Fatalf("persisted report should record the failure: %+v", loaded)
	}
	if loaded.GeneratedAt.IsZero() {
		t.Fatal("persisted report must carry generated_at")
	}
}

func TestAttachActivePatchEffectRecordCapturesAppliedCommitDiff(t *testing.T) {
	mainRoot := filepath.Join(t.TempDir(), "main")
	if err := os.MkdirAll(mainRoot, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = mainRoot
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "test@local")
	git("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(mainRoot, "seed.py"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "seed")

	sess, err := worktree.Create(filepath.Join(t.TempDir(), "wt"), mainRoot, "trace-patch-effect-test")
	if err != nil {
		t.Fatalf("worktree.Create: %v", err)
	}
	t.Cleanup(func() { _ = sess.Discard() })
	if err := os.WriteFile(filepath.Join(sess.Path(), "seed.py"), []byte("new\nextra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, err := worktree.CommitChanges(sess.Path(), "codrax apply iter (plan=plan-effect)")
	if err != nil {
		t.Fatalf("CommitChanges: %v", err)
	}

	plan := &types.ChangePlan{
		ID:          "plan-effect",
		TargetPaths: []string{"seed.py"},
		Changes: []types.FileChange{{
			Path:  "seed.py",
			Kind:  "modify",
			Apply: &types.FileChangeApplyRecord{Status: "applied"},
		}},
	}
	mu := types.NewMutableState("effect")
	mu.SetChangePlan(plan)
	mu.SetSearchGraph(&repotypes.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"seed.py":   {RelPath: "seed.py"},
			"caller.py": {RelPath: "caller.py"},
		},
		ImportGraph: map[string][]string{},
		ReverseImports: map[string][]string{
			"seed.py": []string{"caller.py"},
		},
	})
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, WorktreePath: sess.Path(), MainRepoRoot: mainRoot}
	o.currentIterCommitSHA = sha

	effect := o.attachActivePatchEffectRecord(plan, types.ChangePlanSlice{ID: "slice-1", Paths: []string{"seed.py"}})
	if effect == nil {
		t.Fatal("expected patch effect record")
	}
	if effect.PlanID != "plan-effect" || effect.SliceID != "slice-1" || effect.HeadRef != sha {
		t.Fatalf("patch effect metadata not preserved: %+v", effect)
	}
	if effect.DiffFingerprint == "" || effect.RecordID == "" || effect.DiffBytes == 0 {
		t.Fatalf("patch effect identity missing: %+v", effect)
	}
	if len(effect.Files) != 1 {
		t.Fatalf("effect files = %d, want 1: %+v", len(effect.Files), effect.Files)
	}
	file := effect.Files[0]
	if file.Path != "seed.py" || file.AddedLines != 2 || file.RemovedLines != 1 ||
		file.Language != "py" || file.PathRole != types.SourcePathRoleProduction {
		t.Fatalf("applied commit diff not parsed as expected: %+v", file)
	}
	if got := mu.ChangePlan(); got == nil || got.PatchEffect == nil || got.PatchEffect.HeadRef != sha {
		t.Fatalf("patch effect was not persisted onto mutable ChangePlan: %+v", got)
	}
	got := mu.ChangePlan()
	if got.ImpactObligations == nil || !impactObligationsContain(got.ImpactObligations, "changed_file", "actual_diff", "seed.py") {
		t.Fatalf("actual diff impact obligation missing: %+v", got.ImpactObligations)
	}
	if got.ImpactAnalysis == nil || got.ImpactAnalysis.ObligationSet == nil || len(got.ImpactAnalysis.VerificationTargets) == 0 {
		t.Fatalf("impact analysis result missing from applied patch: %+v", got.ImpactAnalysis)
	}
	if !impactObligationsContainRelated(got.ImpactObligations, "dependent", "reverse_import", "seed.py", "caller.py") {
		t.Fatalf("graph-derived dependent impact obligation missing: %+v", got.ImpactObligations)
	}
}

func readChangeReportFile(t *testing.T, path string) *types.ChangeReport {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("durable report JSON missing at %s: %v", path, err)
	}
	var report types.ChangeReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("report JSON did not parse: %v", err)
	}
	return &report
}

func impactObligationsContain(set *types.ImpactObligationSet, kind, relation, path string) bool {
	if set == nil {
		return false
	}
	for _, ob := range set.Obligations {
		if ob.Kind == kind && ob.Relation == relation && ob.SubjectPath == path {
			return true
		}
	}
	return false
}

func impactObligationsContainRelated(set *types.ImpactObligationSet, kind, relation, path, related string) bool {
	if set == nil {
		return false
	}
	for _, ob := range set.Obligations {
		if ob.Kind == kind && ob.Relation == relation && ob.SubjectPath == path && ob.RelatedPath == related {
			return true
		}
	}
	return false
}
