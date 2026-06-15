package tool

import (
	"encoding/json"
	"errors"
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

func TestCompileStructuredEditsToPatch_RelocatesUniqueOldText(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	change := &types.FileChange{
		Path: "file.txt",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "replace", StartLine: 1, EndLine: 2, OldText: "two\n", Content: "TWO\n"},
		},
	}
	patch, err := compileStructuredEditsToPatch(repo, change)
	if err != nil {
		t.Fatalf("unique old_text should relocate the line range: %v", err)
	}
	if strings.Contains(patch, "-one") {
		t.Fatalf("relocated edit must not replace the originally supplied wider range:\n%s", patch)
	}
	if !strings.Contains(patch, "-two") || !strings.Contains(patch, "+TWO") {
		t.Fatalf("relocated edit should replace the unique old_text line:\n%s", patch)
	}
}

func TestCompileStructuredEditsToPatch_AmbiguousOldTextDoesNotRelocate(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("one\ntwo\nthree\ntwo\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	change := &types.FileChange{
		Path: "file.txt",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "replace", StartLine: 1, EndLine: 1, OldText: "two\n", Content: "TWO\n"},
		},
	}
	if _, err := compileStructuredEditsToPatch(repo, change); err == nil || !strings.Contains(err.Error(), "old_text mismatch") {
		t.Fatalf("ambiguous old_text relocation should be rejected, got %v", err)
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

func TestCompileStructuredEdits_DiagnosticForInvalidLineRange(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("class A {\n}\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	change := &types.FileChange{
		Path: "file.txt",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "replace", StartLine: 3, EndLine: 5, Content: "x\n"},
		},
	}
	_, err := compileStructuredEditsToPatch(repo, change)
	if err == nil {
		t.Fatal("invalid range should fail")
	}
	var diag *structuredEditDiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("error should carry structured diagnostic: %v", err)
	}
	if diag.diagnostic.ReasonCode != "invalid_line_range" ||
		diag.diagnostic.FileLineCount != 2 ||
		!containsStructuredEditString(diag.diagnostic.SafeEditKinds, "insert_before_final_brace") {
		t.Fatalf("unexpected diagnostic: %+v", diag.diagnostic)
	}
	if !strings.Contains(err.Error(), `"reason_code":"invalid_line_range"`) {
		t.Fatalf("error string should expose diagnostic JSON: %v", err)
	}
}

func TestCompileStructuredEdits_DiagnosticForOldTextMismatch(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	change := &types.FileChange{
		Path: "file.txt",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "replace", StartLine: 2, OldText: "not-two\n", Content: "TWO\n"},
		},
	}
	_, err := compileStructuredEditsToPatch(repo, change)
	if err == nil {
		t.Fatal("old_text mismatch should fail")
	}
	var diag *structuredEditDiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("error should carry structured diagnostic: %v", err)
	}
	if diag.diagnostic.ReasonCode != "old_text_mismatch" ||
		diag.diagnostic.CurrentBytes != "two\n" ||
		diag.diagnostic.SuppliedOldText != "not-two\n" ||
		diag.diagnostic.ExpectedOldText != "two\n" ||
		diag.diagnostic.CurrentByteLen != len("two\n") ||
		diag.diagnostic.SuppliedByteLen != len("not-two\n") ||
		!strings.Contains(diag.diagnostic.RetryInstruction, "expected_old_text") ||
		!strings.Contains(diag.diagnostic.RetryInstruction, "omit old_text") ||
		diag.diagnostic.StartLine != 2 {
		t.Fatalf("unexpected diagnostic: %+v", diag.diagnostic)
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
	rec := ctx.Mutable.ChangePlan().Changes[0].Apply
	if rec == nil {
		t.Fatal("structured edit apply should record per-change apply audit")
	}
	if rec.Status != "applied" || rec.Source != "structured_builder" || rec.Engine != "git_apply" {
		t.Fatalf("unexpected structured edit apply record: %+v", rec)
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

func TestCompileStructuredEdits_SingleLineReplaceWithoutEndLine(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "src/a.txt", "one\ntwo\nthree\n")
	change := &types.FileChange{
		Path: "src/a.txt",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "replace", StartLine: 2, Content: "TWO\n"},
		},
	}
	patch, err := compileStructuredEditsToPatch(root, change)
	if err != nil {
		t.Fatalf("omitted end_line must default to start_line: %v", err)
	}
	if !strings.Contains(patch, "-two") || !strings.Contains(patch, "+TWO") {
		t.Fatalf("unexpected patch:\n%s", patch)
	}
}

func TestCompileStructuredEdits_SingleLineDeleteWithoutEndLine(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "src/a.txt", "one\ntwo\nthree\n")
	change := &types.FileChange{
		Path:  "src/a.txt",
		Kind:  "patch",
		Edits: []types.StructuredEdit{{Kind: "delete", StartLine: 2}},
	}
	patch, err := compileStructuredEditsToPatch(root, change)
	if err != nil {
		t.Fatalf("omitted end_line on delete must default to start_line: %v", err)
	}
	if !strings.Contains(patch, "-two") {
		t.Fatalf("unexpected patch:\n%s", patch)
	}
}

func TestCompileStructuredEdits_OldTextTrailingNewlineTolerated(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "src/a.txt", "one\ntwo\nthree\n")
	change := &types.FileChange{
		Path: "src/a.txt",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			// Model quoted the line without the terminal newline.
			{Kind: "replace", StartLine: 2, OldText: "two", Content: "TWO\n"},
		},
	}
	if _, err := compileStructuredEditsToPatch(root, change); err != nil {
		t.Fatalf("missing final newline in old_text must be tolerated: %v", err)
	}
}

func TestCompileStructuredEdits_OldTextMismatchEchoesCurrentBytes(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "src/a.txt", "one\ntwo\nthree\n")
	change := &types.FileChange{
		Path: "src/a.txt",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "replace", StartLine: 2, EndLine: 2, OldText: "stale line\n", Content: "TWO\n"},
		},
	}
	_, err := compileStructuredEditsToPatch(root, change)
	if err == nil {
		t.Fatal("stale old_text must be rejected")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"two\n"`) {
		t.Fatalf("mismatch diagnostic must echo the current bytes, got: %s", msg)
	}
	if !strings.Contains(msg, "expected_old_text") {
		t.Fatalf("mismatch diagnostic must carry the repair direction, got: %s", msg)
	}
	if !strings.Contains(msg, "supplied_old_text") || !strings.Contains(msg, "supplied_byte_len") {
		t.Fatalf("mismatch diagnostic must expose submitted old_text metadata, got: %s", msg)
	}
}

func TestCompileStructuredEdits_InsertAnchorMismatchEchoesCurrentBytes(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "src/a.txt", "one\ntwo\n")
	change := &types.FileChange{
		Path: "src/a.txt",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "insert_after", StartLine: 1, OldText: "wrong anchor", Content: "inserted\n"},
		},
	}
	_, err := compileStructuredEditsToPatch(root, change)
	if err == nil {
		t.Fatal("stale anchor must be rejected")
	}
	if !strings.Contains(err.Error(), `"one\n"`) {
		t.Fatalf("anchor diagnostic must echo current bytes, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), `"expected_old_text":"one\n"`) {
		t.Fatalf("anchor diagnostic must carry reusable old_text, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), `"supplied_old_text":"wrong anchor"`) {
		t.Fatalf("anchor diagnostic must carry supplied old_text, got: %s", err.Error())
	}
}

func TestStructuredEditOldTextMatches_ByteRules(t *testing.T) {
	cases := []struct {
		got, old string
		want     bool
	}{
		{"two\n", "two\n", true},
		{"two\n", "two", true},
		{"two", "two\n", true},
		{"two\n", "Two\n", false},
		{"two\n", " two\n", false},
		{"a\nb\n", "a\nb", true},
	}
	for _, c := range cases {
		if got := structuredEditOldTextMatches(c.got, c.old); got != c.want {
			t.Fatalf("match(%q, %q) = %v, want %v", c.got, c.old, got, c.want)
		}
	}
}

// The live patch_go_typo root cause: a mid-file replace whose content
// omits the trailing "\n" must NOT fuse with the following line. The
// builder owns line integrity — the model quotes line content, with or
// without the terminal newline, exactly like old_text.
func TestCompileStructuredEdits_ContentWithoutNewlineDoesNotFuseNextLine(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "main.go", "func greet() string {\n\tretrun greet()\n}\n")
	change := &types.FileChange{
		Path: "main.go",
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "replace", StartLine: 2, Content: "\treturn greet()"},
		},
	}
	patch, err := compileStructuredEditsToPatch(root, change)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if strings.Contains(patch, "return greet()}") {
		t.Fatalf("content without trailing newline fused with the next line:\n%s", patch)
	}
	if strings.Contains(patch, "-}") {
		t.Fatalf("single-line replace must not touch the closing brace line:\n%s", patch)
	}
	if !strings.Contains(patch, "+\treturn greet()") {
		t.Fatalf("unexpected patch:\n%s", patch)
	}
}

// EOF-newline convention is preserved both ways: replacing the final
// line of a newline-terminated file keeps the newline even when the
// content omits it; a file without the EOF newline stays without it.
func TestCompileStructuredEdits_EOFNewlineConventionPreserved(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "a.txt", "one\ntwo\n")
	change := &types.FileChange{
		Path:  "a.txt",
		Kind:  "patch",
		Edits: []types.StructuredEdit{{Kind: "replace", StartLine: 2, Content: "TWO"}},
	}
	patch, err := compileStructuredEditsToPatch(root, change)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if strings.Contains(patch, "No newline at end of file") {
		t.Fatalf("EOF newline must be preserved for a newline-terminated file:\n%s", patch)
	}

	writeSurfaceFile(t, root, "b.txt", "one\ntwo")
	change = &types.FileChange{
		Path:  "b.txt",
		Kind:  "patch",
		Edits: []types.StructuredEdit{{Kind: "replace", StartLine: 1, Content: "ONE"}},
	}
	patch, err = compileStructuredEditsToPatch(root, change)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if strings.Contains(patch, "ONEtwo") {
		t.Fatalf("mid-file fusion against no-EOF-newline file:\n%s", patch)
	}
}

// insert_after the final line of a no-EOF-newline file historically
// fused the new line onto the unterminated last line — same integrity
// invariant, different edit kind.
func TestCompileStructuredEdits_InsertAfterUnterminatedLastLine(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "a.txt", "one\ntwo")
	change := &types.FileChange{
		Path:  "a.txt",
		Kind:  "patch",
		Edits: []types.StructuredEdit{{Kind: "insert_after", StartLine: 2, Content: "three"}},
	}
	patch, err := compileStructuredEditsToPatch(root, change)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if strings.Contains(patch, "twothree") {
		t.Fatalf("insert_after fused with the unterminated last line:\n%s", patch)
	}
}

func TestCompileStructuredEdits_InsertAtEOFWithoutLineCount(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "src/a.txt", "one\n")
	change := &types.FileChange{
		Path:  "src/a.txt",
		Kind:  "patch",
		Edits: []types.StructuredEdit{{Kind: "insert_at_eof", Content: "two\n"}},
	}
	patch, err := compileStructuredEditsToPatch(root, change)
	if err != nil {
		t.Fatalf("insert_at_eof should compile without start_line: %v", err)
	}
	if !strings.Contains(patch, "+two") {
		t.Fatalf("unexpected patch:\n%s", patch)
	}
}

func TestCompileStructuredEdits_InsertBeforeFinalBrace(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "src/A.java", "class A {\n    String value;\n}\n")
	change := &types.FileChange{
		Path: "src/A.java",
		Kind: "patch",
		Edits: []types.StructuredEdit{{
			Kind:    "insert_before_final_brace",
			Content: "\n    @Override\n    public int hashCode() {\n        return value.hashCode();\n    }\n",
		}},
	}
	patch, err := compileStructuredEditsToPatch(root, change)
	if err != nil {
		t.Fatalf("insert_before_final_brace should compile without line-count arithmetic: %v", err)
	}
	if !strings.Contains(patch, "+    public int hashCode()") {
		t.Fatalf("unexpected patch:\n%s", patch)
	}
}

func containsStructuredEditString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
