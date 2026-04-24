package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderPlanDiff_Empty(t *testing.T) {
	got := renderPlanDiff(nil, "", 1024, 256)
	if !strings.Contains(got, "no changes") {
		t.Errorf("nil plan should yield a no-changes notice; got %q", got)
	}
	got = renderPlanDiff(&types.ChangePlan{}, "", 1024, 256)
	if !strings.Contains(got, "no changes") {
		t.Errorf("plan with empty Changes should yield notice; got %q", got)
	}
}

func TestRenderPlanDiff_Create(t *testing.T) {
	plan := &types.ChangePlan{
		Changes: []types.FileChange{{
			Path:       "new.go",
			Kind:       "create",
			NewContent: "package main\n\nfunc main() {}\n",
			Rationale:  "bootstrap file",
		}},
	}
	got := renderPlanDiff(plan, "", 4096, 1024)
	if !strings.Contains(got, "(new file)") {
		t.Errorf("create should show '(new file)' header; got %q", got)
	}
	if !strings.Contains(got, "+ package main") {
		t.Errorf("create lines should prefix '+ '; got %q", got)
	}
	if !strings.Contains(got, "rationale: bootstrap file") {
		t.Errorf("rationale should surface; got %q", got)
	}
}

func TestRenderPlanDiff_ModifyWithCurrentFile(t *testing.T) {
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package foo\n\nfunc A() int { return 1 }\n"), 0o644)

	plan := &types.ChangePlan{
		Changes: []types.FileChange{{
			Path:       "foo.go",
			Kind:       "modify",
			NewContent: "package foo\n\nfunc A() int { return 2 }\n",
			Rationale:  "off-by-one",
		}},
	}
	got := renderPlanDiff(plan, repo, 4096, 1024)
	// Unified diff output has @@ hunk header + - and + line markers.
	if !strings.Contains(got, "@@") {
		t.Errorf("modify should produce a unified diff hunk header; got %q", got)
	}
	if !strings.Contains(got, "-func A() int { return 1 }") {
		t.Errorf("diff should show old body line; got %q", got)
	}
	if !strings.Contains(got, "+func A() int { return 2 }") {
		t.Errorf("diff should show new body line; got %q", got)
	}
}

func TestRenderPlanDiff_ModifyMissingFileShowsAsNewFile(t *testing.T) {
	// When the current file can't be read (absent, wrong repoRoot),
	// udiff.Unified falls back to a "new file" shape diff — the
	// preview reflects that honestly rather than erroring.
	plan := &types.ChangePlan{
		Changes: []types.FileChange{{
			Path:       "ghost.go",
			Kind:       "modify",
			NewContent: "package ghost\n",
		}},
	}
	got := renderPlanDiff(plan, t.TempDir(), 4096, 1024)
	if !strings.Contains(got, "package ghost") {
		t.Errorf("unreadable current file should still render NewContent; got %q", got)
	}
}

func TestRenderPlanDiff_Delete(t *testing.T) {
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "dying.go"), []byte("package dying\n\nfunc X() {}\n"), 0o644)

	plan := &types.ChangePlan{
		Changes: []types.FileChange{{Path: "dying.go", Kind: "delete"}},
	}
	got := renderPlanDiff(plan, repo, 4096, 1024)
	if !strings.Contains(got, "(deleting existing file)") {
		t.Errorf("delete should show deletion notice; got %q", got)
	}
	if !strings.Contains(got, "- package dying") {
		t.Errorf("delete should show head of body with '-' prefix; got %q", got)
	}
}

func TestRenderPlanDiff_DeleteAbsentFile(t *testing.T) {
	plan := &types.ChangePlan{
		Changes: []types.FileChange{{Path: "phantom.go", Kind: "delete"}},
	}
	got := renderPlanDiff(plan, t.TempDir(), 4096, 1024)
	if !strings.Contains(got, "idempotent for absent files") {
		t.Errorf("delete of absent file should carry idempotency note; got %q", got)
	}
}

func TestRenderPlanDiff_Patch(t *testing.T) {
	patchBody := `--- a/x.go
+++ b/x.go
@@ -1,2 +1,2 @@
-old line
+new line
`
	plan := &types.ChangePlan{
		Changes: []types.FileChange{{
			Path:  "x.go",
			Kind:  "patch",
			Patch: patchBody,
		}},
	}
	got := renderPlanDiff(plan, "", 4096, 1024)
	if !strings.Contains(got, "-old line") || !strings.Contains(got, "+new line") {
		t.Errorf("patch body should render verbatim; got %q", got)
	}
}

func TestRenderPlanDiff_EmptyPatchFlagged(t *testing.T) {
	plan := &types.ChangePlan{
		Changes: []types.FileChange{{Path: "x.go", Kind: "patch", Patch: "  \n"}},
	}
	got := renderPlanDiff(plan, "", 4096, 1024)
	if !strings.Contains(got, "empty patch body") {
		t.Errorf("empty patch should be called out; got %q", got)
	}
}

func TestRenderPlanDiff_UnknownKind(t *testing.T) {
	plan := &types.ChangePlan{
		Changes: []types.FileChange{{Path: "x.go", Kind: "rename"}},
	}
	got := renderPlanDiff(plan, "", 4096, 1024)
	if !strings.Contains(got, "unknown kind") {
		t.Errorf("unknown kind should surface a warning; got %q", got)
	}
}

func TestRenderPlanDiff_DependsOnRendered(t *testing.T) {
	plan := &types.ChangePlan{
		Changes: []types.FileChange{
			{Path: "a.go", Kind: "create", NewContent: "x"},
			{Path: "b.go", Kind: "modify", NewContent: "y", DependsOn: []string{"a.go"}},
		},
	}
	got := renderPlanDiff(plan, "", 4096, 1024)
	if !strings.Contains(got, "depends_on: a.go") {
		t.Errorf("depends_on should render on the dependent change; got %q", got)
	}
}

func TestRenderPlanDiff_PerChangeTruncation(t *testing.T) {
	// 10 KB worth of content → clip to maxPerChangeBytes=256.
	big := strings.Repeat("abc\n", 3000)
	plan := &types.ChangePlan{
		Changes: []types.FileChange{{Path: "big.txt", Kind: "create", NewContent: big}},
	}
	got := renderPlanDiff(plan, "", 16*1024, 256)
	if !strings.Contains(got, "change body truncated") {
		t.Errorf("over-cap change should emit per-change truncation notice; got tail %q", got[len(got)-200:])
	}
}

func TestRenderPlanDiff_TotalTruncation(t *testing.T) {
	// 5 changes, each big enough that after 2 the total cap trips.
	big := strings.Repeat("x\n", 800)
	changes := make([]types.FileChange, 5)
	for i := range changes {
		changes[i] = types.FileChange{
			Path:       "f" + string(rune('A'+i)) + ".txt",
			Kind:       "create",
			NewContent: big,
		}
	}
	plan := &types.ChangePlan{Changes: changes}
	got := renderPlanDiff(plan, "", 2048, 1024)
	if !strings.Contains(got, "diff output truncated") {
		t.Errorf("over-total-cap should emit total truncation notice; got %q", got[len(got)-200:])
	}
	// Remaining paths should surface by name so user knows what got dropped.
	if !strings.Contains(got, "fC.txt") || !strings.Contains(got, "fD.txt") {
		t.Errorf("remaining-change roster should name truncated paths; got %q", got)
	}
}

func TestRenderPlanDiff_NoTextualChange(t *testing.T) {
	// planner emits modify with NewContent identical to current file
	// → udiff.Unified returns "" → renderer emits an explicit note.
	repo := t.TempDir()
	body := "unchanged\n"
	os.WriteFile(filepath.Join(repo, "same.go"), []byte(body), 0o644)
	plan := &types.ChangePlan{
		Changes: []types.FileChange{{Path: "same.go", Kind: "modify", NewContent: body}},
	}
	got := renderPlanDiff(plan, repo, 4096, 1024)
	if !strings.Contains(got, "no textual change") {
		t.Errorf("identical modify should surface 'no textual change'; got %q", got)
	}
}
