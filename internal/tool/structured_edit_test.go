package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCompileStructuredEditsToPatch_ReplaceAndInsert(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	change := &types.FileChange{
		Path: "file.txt",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "replace", StartLine: 2, EndLine: 2, OldText: "two\n", Content: "TWO\n"},
			{Kind: "insert_after", StartLine: 3, OldText: "three\n", Content: "four\n"},
		},
	}
	patch, err := compileStructuredEditsToPatch(repo, change)
	if err != nil {
		t.Fatalf("compileStructuredEditsToPatch: %v", err)
	}
	for _, want := range []string{"--- a/file.txt", "+++ b/file.txt", "-two", "+TWO", "+four"} {
		if !strings.Contains(patch, want) {
			t.Fatalf("compiled patch missing %q:\n%s", want, patch)
		}
	}
}

func TestCompileStructuredEditsToPatch_RejectsStaleContext(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	change := &types.FileChange{
		Path: "file.txt",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "replace", StartLine: 2, EndLine: 2, OldText: "not-two\n", Content: "TWO\n"},
		},
	}
	if _, err := compileStructuredEditsToPatch(repo, change); err == nil || !strings.Contains(err.Error(), "old_text mismatch") {
		t.Fatalf("stale context should be rejected with old_text mismatch; got %v", err)
	}
}

func TestCompileStructuredEditsToPatch_RejectsOverlap(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	change := &types.FileChange{
		Path: "file.txt",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "replace", StartLine: 1, EndLine: 2, Content: "ONE\nTWO\n"},
			{Kind: "delete", StartLine: 2, EndLine: 3},
		},
	}
	if _, err := compileStructuredEditsToPatch(repo, change); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("overlap should be rejected; got %v", err)
	}
}

func TestEmitChangePlan_StructuredEditsCompileToPatch(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "one\ntwo\nthree\n")
	ctx.Mutable = types.NewMutableState("test")

	params := json.RawMessage(`{
		"request": "rename one line",
		"summary": "Change the second line using structured edits.",
		"changes": [
			{
				"path": "file.txt",
				"kind": "patch",
				"edits": [{"kind":"replace","start_line":2,"end_line":2,"old_text":"two\n","content":"TWO\n"}],
				"rationale": "test"
			}
		]
	}`)
	res, err := (&EmitChangePlan{}).Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("structured edit plan should be accepted: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("plan should be installed")
	}
	if len(plan.Changes[0].Edits) != 1 {
		t.Fatalf("structured edits should be preserved on plan: %+v", plan.Changes[0].Edits)
	}
	if !strings.Contains(plan.Changes[0].Patch, "-two") || !strings.Contains(plan.Changes[0].Patch, "+TWO") {
		t.Fatalf("structured edits should compile to reviewable patch; got:\n%s", plan.Changes[0].Patch)
	}
}

func TestApplyPatch_StructuredEditsOnlyPlan(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; patch apply check skipped")
	}
	ctx := gitWorktreeFixture(t, "one\ntwo\nthree\n")
	ctx.Mutable.ChangePlan().Changes[0].Edits = []types.StructuredEdit{
		{Kind: "replace", StartLine: 2, EndLine: 2, OldText: "two\n", Content: "TWO\n"},
	}
	ctx.Mutable.ChangePlan().Changes[0].Patch = ""

	res, err := (&ApplyPatch{}).Execute(ctx, json.RawMessage(`{"path":"file.txt","kind":"patch"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("apply structured edits should succeed: %s", res.Summary)
	}
	got, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "file.txt"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != "one\nTWO\nthree\n" {
		t.Fatalf("file content mismatch: %q", string(got))
	}
}

func TestEmitPlanChange_StructuredEditsFinalize(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "one\ntwo\nthree\n")
	ctx.Mutable = types.NewMutableState("test")
	ctx.Mutable.SetPartialChangePlan(&types.ChangePlan{
		ID:      "plan-partial",
		Request: "rename one line",
		Summary: "Change the second line using structured edits.",
		Status:  types.PlanStatusPending,
		Changes: []types.FileChange{{
			Path:      "file.txt",
			Kind:      "patch",
			Rationale: "test",
		}},
		TargetPaths: []string{"file.txt"},
	})

	params := json.RawMessage(`{
		"path": "file.txt",
		"edits": [{"kind":"replace","start_line":2,"end_line":2,"old_text":"two\n","content":"TWO\n"}]
	}`)
	res, err := (&EmitPlanChange{}).Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("emit_plan_change with edits should finalize: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("partial plan should be promoted")
	}
	if !strings.Contains(plan.Changes[0].Patch, "-two") || !strings.Contains(plan.Changes[0].Patch, "+TWO") {
		t.Fatalf("finalized plan should contain compiled patch; got:\n%s", plan.Changes[0].Patch)
	}
}
