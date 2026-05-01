package tool

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// applyPatchFixture builds a BusContext with a real temp-dir
// RepoRoot + a seeded ChangePlan so each apply_patch test can
// exercise the Execute path end-to-end without a real git repo or
// orchestrator. The temp dir is automatically cleaned via t.TempDir.
func applyPatchFixture(t *testing.T, plan *types.ChangePlan) *types.BusContext {
	t.Helper()
	ctx := &types.BusContext{
		RepoRoot: t.TempDir(),
		Mutable:  types.NewMutableState("test"),
	}
	if plan != nil {
		ctx.Mutable.SetChangePlan(plan)
	}
	return ctx
}

// simplePlan is a minimal 1-file plan factory for the happy-path tests.
func simplePlan(kind, path, content string) *types.ChangePlan {
	return &types.ChangePlan{
		ID:     "plan-test",
		Status: "pending_approval",
		Changes: []types.FileChange{
			{Path: path, Kind: kind, NewContent: content, Rationale: "test"},
		},
		TargetPaths: []string{path},
	}
}

// TestApplyPatch_Create verifies the happy-path create: file does
// not exist, apply_patch writes NewContent, WriteClosure marks it.
func TestApplyPatch_Create(t *testing.T) {
	plan := simplePlan("create", "hello.txt", "hello world\n")
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"hello.txt","kind":"create"}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Success=false; summary=%q", res.Summary)
	}
	// File exists with expected content.
	got, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "hello.txt"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != "hello world\n" {
		t.Errorf("file content = %q, want %q", string(got), "hello world\n")
	}
	// WriteClosure sees the apply.
	if !ctx.Mutable.WriteClosure().HasApplied("hello.txt") {
		t.Error("WriteClosure should record hello.txt as applied")
	}
}

// TestApplyPatch_Modify verifies overwriting an existing file.
func TestApplyPatch_Modify(t *testing.T) {
	plan := simplePlan("modify", "existing.txt", "new body\n")
	ctx := applyPatchFixture(t, plan)
	// Pre-existing file with old content.
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "existing.txt"), []byte("old body\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"existing.txt","kind":"modify"}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("Success=false; summary=%q", res.Summary)
	}
	got, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, "existing.txt"))
	if string(got) != "new body\n" {
		t.Errorf("modify result = %q", string(got))
	}
}

// TestApplyPatch_Delete verifies file removal.
func TestApplyPatch_Delete(t *testing.T) {
	plan := simplePlan("delete", "gone.txt", "")
	ctx := applyPatchFixture(t, plan)
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "gone.txt"), []byte("doomed\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"gone.txt","kind":"delete"}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("Success=false; summary=%q", res.Summary)
	}
	if _, err := os.Stat(filepath.Join(ctx.RepoRoot, "gone.txt")); !os.IsNotExist(err) {
		t.Errorf("file should be removed; stat err=%v", err)
	}
	if !ctx.Mutable.WriteClosure().HasApplied("gone.txt") {
		t.Error("delete should also mark path applied")
	}
}

// TestApplyPatch_CreateExistingRejected verifies kind=create refuses
// to overwrite an existing file (planner bug: meant modify).
func TestApplyPatch_CreateExistingRejected(t *testing.T) {
	plan := simplePlan("create", "already.txt", "new\n")
	ctx := applyPatchFixture(t, plan)
	_ = os.WriteFile(filepath.Join(ctx.RepoRoot, "already.txt"), []byte("existed\n"), 0o644)
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"already.txt","kind":"create"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("create over existing file should fail")
	}
	if !strings.Contains(res.Summary, "already exists") {
		t.Errorf("summary should mention 'already exists'; got %q", res.Summary)
	}
}

// TestApplyPatch_ModifyMissingRejected verifies kind=modify refuses
// missing files (planner should have used create).
func TestApplyPatch_ModifyMissingRejected(t *testing.T) {
	plan := simplePlan("modify", "ghost.txt", "body\n")
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"ghost.txt","kind":"modify"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("modify on non-existent file should fail")
	}
	if !strings.Contains(res.Summary, "does not exist") {
		t.Errorf("summary should mention 'does not exist'; got %q", res.Summary)
	}
}

// TestApplyPatch_W1PathNotInPlan verifies a path outside plan.
// TargetPaths is rejected with a W1 message.
func TestApplyPatch_W1PathNotInPlan(t *testing.T) {
	plan := simplePlan("create", "allowed.txt", "x")
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"unauthorized.go","kind":"create"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("path outside TargetPaths must be rejected")
	}
	if !strings.Contains(res.Summary, "W1") {
		t.Errorf("summary should name W1 invariant; got %q", res.Summary)
	}
}

// TestApplyPatch_W1bDependsOnNotApplied verifies W1b — dependency
// ordering enforcement.
func TestApplyPatch_W1bDependsOnNotApplied(t *testing.T) {
	plan := &types.ChangePlan{
		ID:     "plan-order",
		Status: "pending_approval",
		Changes: []types.FileChange{
			{Path: "helper.go", Kind: "create", NewContent: "h", Rationale: "helper"},
			{Path: "caller.go", Kind: "create", NewContent: "c", Rationale: "caller",
				DependsOn: []string{"helper.go"}},
		},
		TargetPaths: []string{"helper.go", "caller.go"},
	}
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	// Attempt caller.go FIRST (before helper.go) — must fail W1b.
	params := json.RawMessage(`{"path":"caller.go","kind":"create"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("caller.go without helper.go applied first must fail W1b")
	}
	if !strings.Contains(res.Summary, "W1b") {
		t.Errorf("summary should name W1b; got %q", res.Summary)
	}

	// Now apply helper.go first, then caller.go — both should succeed.
	helperParams := json.RawMessage(`{"path":"helper.go","kind":"create"}`)
	if res, _ := tool.Execute(ctx, helperParams); !res.Success {
		t.Fatalf("helper.go should apply first: %q", res.Summary)
	}
	if res, _ := tool.Execute(ctx, params); !res.Success {
		t.Fatalf("caller.go should apply after helper.go: %q", res.Summary)
	}
}

// TestApplyPatch_Idempotent verifies second call on same path is a
// no-op success.
func TestApplyPatch_Idempotent(t *testing.T) {
	plan := simplePlan("create", "once.txt", "content")
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"once.txt","kind":"create"}`)
	// First call creates.
	if res, _ := tool.Execute(ctx, params); !res.Success {
		t.Fatalf("first call should succeed: %q", res.Summary)
	}
	// Second call is a no-op.
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("idempotent call should succeed: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "idempotent") {
		t.Errorf("summary should mark idempotent no-op; got %q", res.Summary)
	}
}

// TestApplyPatch_NoPlan verifies reject when Mutable.ChangePlan is
// nil (caller misuse — apply tool outside apply stage).
func TestApplyPatch_NoPlan(t *testing.T) {
	ctx := applyPatchFixture(t, nil)
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"x.go","kind":"create"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("apply_patch without a ChangePlan must fail")
	}
	if !strings.Contains(res.Summary, "no ChangePlan") {
		t.Errorf("summary should explain missing ChangePlan; got %q", res.Summary)
	}
}

// gitWorktreeFixture initialises a git repo inside t.TempDir() with
// a single committed file, then returns a BusContext rooted at that
// path + a ChangePlan declaring "file.txt" as a patch target. Skips
// the test when git isn't available. Used by the kind=patch tests
// so they can exercise `git apply` end-to-end.
//
// Post session-35: the plan ChangeUnit's Patch field is now the
// authoritative source (the tool no longer reads patch from LLM
// params). Tests that want a specific diff applied must mutate
// ctx.Mutable.ChangePlan().Changes[0].Patch before calling Execute.
func gitWorktreeFixture(t *testing.T, seedContent string) *types.BusContext {
	t.Helper()
	if !GitAvailable() {
		t.Skip("git not available; skipping kind=patch tests")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd, cancel := NewGitCommand(nil, args...)
		defer cancel()
		cmd.Dir = dir
		// Suppress author prompts — git commit needs identity.
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	// Keep patch fixtures deterministic across developer machines:
	// global autocrlf settings would otherwise rewrite the worktree
	// content differently on Windows vs. Linux.
	run("config", "core.autocrlf", "false")
	run("config", "core.eol", "lf")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(seedContent), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	run("add", "file.txt")
	run("commit", "-q", "-m", "seed")

	ctx := &types.BusContext{
		RepoRoot: dir,
		Mutable:  types.NewMutableState("test"),
	}
	ctx.Mutable.SetChangePlan(&types.ChangePlan{
		ID: "plan-patch",
		Changes: []types.FileChange{
			{Path: "file.txt", Kind: "patch", Rationale: "test"},
		},
		TargetPaths: []string{"file.txt"},
	})
	return ctx
}

// TestApplyPatch_PatchHappyPath verifies a valid unified diff
// applies cleanly and WriteClosure records the path. Post session-35:
// Patch lives on the plan ChangeUnit, not the tool call params.
func TestApplyPatch_PatchHappyPath(t *testing.T) {
	ctx := gitWorktreeFixture(t, "hello\nworld\n")
	tool := &ApplyPatch{}

	// Unified diff: change "world" → "codrax". Installed on the plan
	// ChangeUnit (source of truth); the tool reads it from Mutable.
	ctx.Mutable.ChangePlan().Changes[0].Patch = `--- a/file.txt
+++ b/file.txt
@@ -1,2 +1,2 @@
 hello
-world
+codrax
`
	params := json.RawMessage(`{"path":"file.txt","kind":"patch"}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("Success=false; summary=%q", res.Summary)
	}
	got, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "file.txt"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != "hello\ncodrax\n" {
		t.Errorf("file content = %q, want %q", string(got), "hello\ncodrax\n")
	}
	if !ctx.Mutable.WriteClosure().HasApplied("file.txt") {
		t.Error("WriteClosure should record the patched path")
	}
}

// TestApplyPatch_PatchEmpty verifies we reject a plan ChangeUnit with
// empty Patch (planner bug — kind=patch must carry a unified diff).
// Post session-35: the check is against unit.Patch on Mutable, not
// an LLM-supplied parameter.
func TestApplyPatch_PatchEmpty(t *testing.T) {
	plan := simplePlan("patch", "file.go", "")
	ctx := applyPatchFixture(t, plan)
	// simplePlan leaves Patch empty — that's the failure condition.
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"file.go","kind":"patch"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("empty plan.Patch should be rejected")
	}
	if !strings.Contains(res.Summary, "empty Patch") {
		t.Errorf("summary should explain the empty-Patch rejection; got %q", res.Summary)
	}
}

// TestApplyPatch_ContentFieldsRejectedBySchema pins the session-35
// red line: the LLM must not send `patch` or `new_content` as tool
// parameters. The schema uses additionalProperties=false + Go decoder
// DisallowUnknownFields so any attempt fails loud. If a future
// refactor re-introduces content params, this test lights up.
func TestApplyPatch_ContentFieldsRejectedBySchema(t *testing.T) {
	plan := simplePlan("patch", "file.go", "")
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}

	forbidden := []string{
		`{"path":"file.go","kind":"patch","patch":"diff text"}`,
		`{"path":"file.go","kind":"create","new_content":"body"}`,
		`{"path":"file.go","kind":"modify","new_content":"body","patch":"diff"}`,
	}
	for _, raw := range forbidden {
		res, err := tool.Execute(ctx, json.RawMessage(raw))
		if err == nil && res.Success {
			t.Errorf("schema must reject %s; got Success=true, err=%v", raw, err)
		}
		if !strings.Contains(res.Summary, "unknown field") {
			t.Errorf("summary should name the rejected unknown field; raw=%s summary=%q", raw, res.Summary)
		}
	}
}

// TestApplyPatch_PatchRejectedByGit verifies a context-mismatched
// patch surfaces git's own rejection reason, not a silent success.
func TestApplyPatch_PatchRejectedByGit(t *testing.T) {
	ctx := gitWorktreeFixture(t, "hello\nworld\n")
	tool := &ApplyPatch{}

	// Patch targets lines that don't exist ("foo" / "bar" not in seed).
	// Installed on the plan ChangeUnit; tool reads from Mutable.
	ctx.Mutable.ChangePlan().Changes[0].Patch = `--- a/file.txt
+++ b/file.txt
@@ -1,2 +1,2 @@
 foo
-bar
+baz
`
	params := json.RawMessage(`{"path":"file.txt","kind":"patch"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("mismatched-context patch should fail")
	}
	if !strings.Contains(res.Summary, "git apply failed") {
		t.Errorf("summary should mention git apply failure; got %q", res.Summary)
	}
	// File should be unchanged.
	got, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "file.txt"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != "hello\nworld\n" {
		t.Errorf("file should be unchanged after failed patch; got %q", string(got))
	}
}

// TestApplyPatch_KindMismatch verifies the kind parameter must
// match the plan's declared kind.
func TestApplyPatch_KindMismatch(t *testing.T) {
	plan := simplePlan("create", "x.go", "body")
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	// Plan says create, tool call says modify.
	params := json.RawMessage(`{"path":"x.go","kind":"modify"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("kind mismatch should fail")
	}
	if !strings.Contains(res.Summary, "kind mismatch") {
		t.Errorf("summary should name 'kind mismatch'; got %q", res.Summary)
	}
}

// TestApplyPatch_TraversalRejected verifies path-traversal safety.
func TestApplyPatch_TraversalRejected(t *testing.T) {
	plan := &types.ChangePlan{
		ID:     "plan-traversal",
		Status: "pending_approval",
		Changes: []types.FileChange{
			{Path: "../escape.go", Kind: "create", NewContent: "x", Rationale: "evil"},
		},
		TargetPaths: []string{"../escape.go"},
	}
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"../escape.go","kind":"create"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("path traversal outside RepoRoot must be rejected")
	}
}

// TestApplyPatch_CreateMakesParentDirs verifies parent dirs for
// a nested path are created automatically.
func TestApplyPatch_CreateMakesParentDirs(t *testing.T) {
	plan := simplePlan("create", "dir/sub/new.txt", "nested")
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"dir/sub/new.txt","kind":"create"}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("nested create should succeed: %q", res.Summary)
	}
	got, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, "dir/sub/new.txt"))
	if string(got) != "nested" {
		t.Errorf("nested file content = %q", string(got))
	}
}

// TestApplyPatch_W1_RejectionEnumeratesValidTargetPaths pins the
// session-35 feedback-quality invariant: a W1 rejection must list
// every valid path in plan.TargetPaths so the LLM can self-correct
// without re-reading the plan. Generic "path X is not in plan" leaves
// the LLM guessing which path it meant; enumerating "valid: [a, b, c]"
// gives it the exhaustive corrective set.
func TestApplyPatch_W1_RejectionEnumeratesValidTargetPaths(t *testing.T) {
	plan := &types.ChangePlan{
		ID:     "plan-w1",
		Status: "pending_approval",
		Changes: []types.FileChange{
			{Path: "allowed_a.go", Kind: "create", NewContent: "x", Rationale: "a"},
			{Path: "allowed_b.go", Kind: "create", NewContent: "y", Rationale: "b"},
		},
		TargetPaths: []string{"allowed_a.go", "allowed_b.go"},
	}
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"unauthorized.go","kind":"create"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("unauthorized path must be rejected")
	}
	// commit 24: W1 message tightened to "Retry apply_patch with one
	// of the listed paths" — explicit next-step matching W1b's
	// shape. Old "pick one" wording was the prior phrasing.
	for _, w := range []string{"W1", "unauthorized.go", "allowed_a.go", "allowed_b.go", "Retry apply_patch with one of the listed paths"} {
		if !strings.Contains(res.Summary, w) {
			t.Errorf("W1 rejection missing %q; got %q", w, res.Summary)
		}
	}
}

// TestApplyPatch_W1b_RejectionEnumeratesAppliedSet mirrors the W1
// enrichment: a W1b ordering violation must surface the current
// applied set so the LLM knows exactly which sibling to apply first.
// Without the set, "apply X first" is advice in a vacuum — with it,
// the retry is mechanical.
func TestApplyPatch_W1b_RejectionEnumeratesAppliedSet(t *testing.T) {
	plan := &types.ChangePlan{
		ID:     "plan-w1b-enum",
		Status: "pending_approval",
		Changes: []types.FileChange{
			{Path: "helper.go", Kind: "create", NewContent: "h", Rationale: "helper"},
			{Path: "util.go", Kind: "create", NewContent: "u", Rationale: "util"},
			{Path: "caller.go", Kind: "create", NewContent: "c", Rationale: "caller",
				DependsOn: []string{"helper.go", "util.go"}},
		},
		TargetPaths: []string{"helper.go", "util.go", "caller.go"},
	}
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	// Apply helper first so the applied set has one entry but not both.
	if res, _ := tool.Execute(ctx, json.RawMessage(`{"path":"helper.go","kind":"create"}`)); !res.Success {
		t.Fatalf("helper.go should apply: %q", res.Summary)
	}
	// Attempt caller.go — util.go not yet applied, so W1b fires.
	res, _ := tool.Execute(ctx, json.RawMessage(`{"path":"caller.go","kind":"create"}`))
	if res.Success {
		t.Fatal("caller.go without util.go applied first must fail W1b")
	}
	for _, w := range []string{"W1b", "util.go", "helper.go", "Currently-applied"} {
		if !strings.Contains(res.Summary, w) {
			t.Errorf("W1b rejection missing %q; got %q", w, res.Summary)
		}
	}
}

// TestApplyPatch_HydratesFromMutable_NotParams pins the session-35 fix:
// apply_patch reads NewContent from Mutable.ChangePlan().Changes[i],
// NOT from LLM params. We seed the plan with ONE body ("from-plan")
// and verify that's what lands on disk — even though LLM params
// carry nothing beyond path+kind.
//
// Without this guarantee, a legitimate plan could be silently
// clobbered by an LLM misfire (the bug: session-35 coder re-derived
// a patch body from read_file, lost a trailing \n, crashed git apply
// with "corrupt patch at line 11" even though plan held a valid
// diff).
func TestApplyPatch_HydratesFromMutable_NotParams(t *testing.T) {
	plan := simplePlan("create", "canon.txt", "from-plan\n")
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	// LLM only sends {path, kind} — no content.
	params := json.RawMessage(`{"path":"canon.txt","kind":"create"}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Success=false; summary=%q", res.Summary)
	}
	got, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "canon.txt"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != "from-plan\n" {
		t.Errorf("content should come from plan; got %q, want %q", string(got), "from-plan\n")
	}
}

// TestApplyPatch_PatchToolFallback_AcceptsSloppyHunk pins the
// session-35 tolerance chain: when git apply rejects a patch that
// patch(1) can recover via fuzz matching, the fallback lands the
// change. The canonical shape is a hunk with a miscounted @@ header
// AND no trailing context — git apply --recount still rejects this
// (needs *some* post-change context to confirm hunk boundaries);
// patch(1) handles it via its fuzz heuristic.
//
// If this test fails, either git got more tolerant (delete the
// fallback), patch(1) isn't installed (skip), or the fallback is
// mis-wired.
func TestApplyPatch_PatchToolFallback_AcceptsSloppyHunk(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available")
	}
	if _, err := exec.LookPath("patch"); err != nil {
		t.Skip("GNU patch(1) not available")
	}
	// Seed the same 3-line file the session-35 Python repro used;
	// file itself is boring, the patch is what's sloppy.
	ctx := gitWorktreeFixture(t, "l1\nl2\nl3\nl4\nl5\nl6\n")
	// Miscounted header (,5 declared, ,4 actual old) + no trailing
	// context. git apply --recount rejects with "patch failed";
	// patch(1) applies with "fuzz 2".
	sloppy := `--- a/file.txt
+++ b/file.txt
@@ -2,5 +2,5 @@
 l2
 l3
-l4
+lFOUR
`
	ctx.Mutable.ChangePlan().Changes[0].Patch = sloppy

	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"file.txt","kind":"patch"}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("fallback should accept sloppy LLM-shaped patch; got FAIL: %s", res.Summary)
	}
	got, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, "file.txt"))
	if string(got) != "l1\nl2\nl3\nlFOUR\nl5\nl6\n" {
		t.Errorf("post-apply file = %q, want lFOUR at line 4", string(got))
	}
}

// TestApplyPatch_PatchHydratesFromMutable pins the session-35 fix for
// the patch branch specifically. Sentinel: the exact diff that
// reproduced the session-35 failure — trailing `\n` after `func main() {`
// — applies cleanly when sourced from Mutable. The pre-fix tool took
// `patch` from LLM params, and LLMs silently drop trailing newlines;
// sourcing from Mutable guarantees bit-for-bit fidelity.
func TestApplyPatch_PatchHydratesFromMutable(t *testing.T) {
	// Seed: 3-line file where only the middle line changes. Matches
	// the "one-line typo in a function body" shape from the eval.
	seed := "line1\nline2\nline3\n"
	ctx := gitWorktreeFixture(t, seed)
	tool := &ApplyPatch{}
	// Authoritative patch with a clean trailing newline.
	ctx.Mutable.ChangePlan().Changes[0].Patch = `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,3 @@
 line1
-line2
+lineTWO
 line3
`
	params := json.RawMessage(`{"path":"file.txt","kind":"patch"}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("Success=false; summary=%q", res.Summary)
	}
	got, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, "file.txt"))
	if string(got) != "line1\nlineTWO\nline3\n" {
		t.Errorf("post-patch content = %q, want %q", string(got), "line1\nlineTWO\nline3\n")
	}
}
