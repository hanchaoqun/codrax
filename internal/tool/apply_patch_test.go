package tool

import (
	"encoding/json"
	"os"
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
		ID:      "plan-test",
		Status:  "pending_approval",
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
	params := json.RawMessage(`{"path":"hello.txt","kind":"create","new_content":"hello world\n"}`)
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
	params := json.RawMessage(`{"path":"existing.txt","kind":"modify","new_content":"new body\n"}`)
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
	params := json.RawMessage(`{"path":"already.txt","kind":"create","new_content":"new\n"}`)
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
	params := json.RawMessage(`{"path":"ghost.txt","kind":"modify","new_content":"body\n"}`)
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
	params := json.RawMessage(`{"path":"unauthorized.go","kind":"create","new_content":"x"}`)
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
	params := json.RawMessage(`{"path":"caller.go","kind":"create","new_content":"c"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("caller.go without helper.go applied first must fail W1b")
	}
	if !strings.Contains(res.Summary, "W1b") {
		t.Errorf("summary should name W1b; got %q", res.Summary)
	}

	// Now apply helper.go first, then caller.go — both should succeed.
	helperParams := json.RawMessage(`{"path":"helper.go","kind":"create","new_content":"h"}`)
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
	params := json.RawMessage(`{"path":"once.txt","kind":"create","new_content":"content"}`)
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
	params := json.RawMessage(`{"path":"x.go","kind":"create","new_content":"x"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("apply_patch without a ChangePlan must fail")
	}
	if !strings.Contains(res.Summary, "no ChangePlan") {
		t.Errorf("summary should explain missing ChangePlan; got %q", res.Summary)
	}
}

// TestApplyPatch_PatchKindStubbed verifies B1 stubbing of kind=patch.
func TestApplyPatch_PatchKindStubbed(t *testing.T) {
	plan := simplePlan("patch", "file.go", "")
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	params := json.RawMessage(`{"path":"file.go","kind":"patch","patch":"@@ ... @@"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("kind=patch should return stub error in B1")
	}
	if !strings.Contains(res.Summary, "B2") {
		t.Errorf("summary should reference B2; got %q", res.Summary)
	}
}

// TestApplyPatch_KindMismatch verifies the kind parameter must
// match the plan's declared kind.
func TestApplyPatch_KindMismatch(t *testing.T) {
	plan := simplePlan("create", "x.go", "body")
	ctx := applyPatchFixture(t, plan)
	tool := &ApplyPatch{}
	// Plan says create, tool call says modify.
	params := json.RawMessage(`{"path":"x.go","kind":"modify","new_content":"body"}`)
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
	params := json.RawMessage(`{"path":"../escape.go","kind":"create","new_content":"x"}`)
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
	params := json.RawMessage(`{"path":"dir/sub/new.txt","kind":"create","new_content":"nested"}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("nested create should succeed: %q", res.Summary)
	}
	got, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, "dir/sub/new.txt"))
	if string(got) != "nested" {
		t.Errorf("nested file content = %q", string(got))
	}
}
