package writeflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPatchReviewHardEventCodesAreParserPrecise pins the hard-block set
// to exactly the parser/IO/path-confirmed codes. The project red line is
// "precise signals for hard gates, noisy signals for soft guidance": a
// regex or single-line source-shape heuristic must NEVER hard-block a
// batch (it false-rejects idiomatic-but-fine patches and burns the write
// retry budget to Blocked). This guard fails loudly if a future code is
// added to patchReviewEffectHardEventCodes, forcing the author to either
// prove it is parser-precise or route it through the soft advisory lane.
func TestPatchReviewHardEventCodesAreParserPrecise(t *testing.T) {
	want := map[string]bool{
		"patch_effect_path_outside_worktree": true,
		"structured_file_missing":            true,
		"structured_file_parse_error":        true,
	}
	if len(patchReviewEffectHardEventCodes) != len(want) {
		t.Fatalf("hard event code set changed: got %v, want exactly %v — a hard gate must read a parser/IO/path-confirmed fact, not a regex/heuristic; demote noisy codes to patchReviewEffectSoftEventCodes", patchReviewEffectHardEventCodes, want)
	}
	for code := range want {
		if !patchReviewEffectHardEventCodes[code] {
			t.Errorf("expected parser-precise hard code %q missing from hard set", code)
		}
	}
	for code := range patchReviewEffectHardEventCodes {
		if !want[code] {
			t.Errorf("non-parser-precise code %q must not hard-block (precise-signals red line); demote it to the soft advisory lane", code)
		}
	}
}

func TestPatchEffectRecordFromUnifiedDiffParsesCommitPatch(t *testing.T) {
	diff := `commit abc123
Author: Codrax <codrax@example.invalid>

    apply

diff --git a/pkg/a.py b/pkg/a.py
index 1111111..2222222 100644
--- a/pkg/a.py
+++ b/pkg/a.py
@@ -1,3 +1,4 @@
 keep
-old
+new
+extra
 tail
diff --git a/pkg/old.py b/pkg/new.py
similarity index 98%
rename from pkg/old.py
rename to pkg/new.py
--- a/pkg/old.py
+++ b/pkg/new.py
@@ -10 +10 @@
-old_name()
+new_name()
diff --git a/tests/test_a.py b/tests/test_a.py
new file mode 100644
--- /dev/null
+++ b/tests/test_a.py
@@ -0,0 +1,2 @@
+def test_a():
+    pass
`

	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	if record.PlanID != "plan-1" || record.SliceID != "slice-1" || record.HeadRef != "abc123" {
		t.Fatalf("metadata not preserved: %+v", record)
	}
	if record.DiffFingerprint == "" || record.RecordID == "" || record.DiffBytes == 0 {
		t.Fatalf("diff identity fields missing: %+v", record)
	}
	if len(record.Files) != 3 {
		t.Fatalf("files = %d, want 3: %+v", len(record.Files), record.Files)
	}
	mod := findPatchEffectFile(record, "pkg/a.py")
	if mod == nil || mod.Status != "modified" || mod.AddedLines != 2 || mod.RemovedLines != 1 ||
		mod.Language != "py" || mod.PathRole != types.SourcePathRoleProduction || len(mod.Hunks) != 1 {
		t.Fatalf("modified file not parsed: %+v", mod)
	}
	if got := mod.Hunks[0].AddedLineNumbers; len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("added line numbers = %+v, want [2 3]", got)
	}
	if got := mod.Hunks[0].AddedLineTexts; len(got) != 2 || got[0].Line != 2 || got[0].Text != "new" || got[1].Text != "extra" {
		t.Fatalf("added line texts = %+v, want new/extra", got)
	}
	if got := mod.Hunks[0].RemovedLineTexts; len(got) != 1 || got[0].Line != 2 || got[0].Text != "old" {
		t.Fatalf("removed line texts = %+v, want old", got)
	}
	renamed := findPatchEffectFile(record, "pkg/new.py")
	if renamed == nil || renamed.Status != "renamed" || renamed.OldPath != "pkg/old.py" ||
		renamed.AddedLines != 1 || renamed.RemovedLines != 1 {
		t.Fatalf("rename file not parsed: %+v", renamed)
	}
	created := findPatchEffectFile(record, "tests/test_a.py")
	if created == nil || created.Status != "created" || created.AddedLines != 2 ||
		created.PathRole != types.SourcePathRoleTest {
		t.Fatalf("created test file not parsed: %+v", created)
	}
}

func TestPatchEffectRecordFromUnifiedDiffParsesDeletedFile(t *testing.T) {
	diff := `diff --git a/pkg/dead.py b/pkg/dead.py
deleted file mode 100644
--- a/pkg/dead.py
+++ /dev/null
@@ -1,2 +0,0 @@
-a
-b
`

	record := PatchEffectRecordFromUnifiedDiff("plan-1", "", "", "", "abc123", diff)
	deleted := findPatchEffectFile(record, "pkg/dead.py")
	if deleted == nil || deleted.Status != "deleted" || deleted.RemovedLines != 2 || deleted.AddedLines != 0 {
		t.Fatalf("deleted file not parsed: %+v", record.Files)
	}
}

func TestAnnotatePatchEffectStructuredFileParsesInvalidJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":`), 0o644); err != nil {
		t.Fatal(err)
	}
	record := types.PatchEffectRecord{
		RecordID: "patch-effect:plan-1:slice-1:abcdef123456",
		Files: []types.PatchEffectFile{{
			Path:   "package.json",
			Status: "modified",
		}},
	}

	AnnotatePatchEffectStructuredFileParses(&record, root)
	if len(record.Files) != 1 || len(record.Files[0].Events) != 1 {
		t.Fatalf("expected one structured parse event: %+v", record.Files)
	}
	event := record.Files[0].Events[0]
	if event.Code != "structured_file_parse_error" || event.Severity != "error" || event.Path != "package.json" {
		t.Fatalf("unexpected structured parse event: %+v", event)
	}

	review := ReviewAppliedPatchScope(&types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"package.json"},
		PatchEffect: &record,
	}, types.ChangePlanSlice{})
	if !review.HardBlock || !patchReviewHasFinding(review, "structured_file_parse_error") {
		t.Fatalf("structured parser event should hard block review: %+v", review)
	}
}

func TestAnnotatePatchEffectPythonTopLevelSelfMethodIsSoftAdvisory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "axis.py"), []byte("class Axis:\n    pass\n\ndef _set_lim(self, v0):\n    return v0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `diff --git a/pkg/axis.py b/pkg/axis.py
--- a/pkg/axis.py
+++ b/pkg/axis.py
@@ -1,2 +1,5 @@
 class Axis:
     pass
+
+def _set_lim(self, v0):
+    return v0
`
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, "pkg/axis.py")
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if !patchEffectHasEvent(*file, "python_top_level_self_method") {
		t.Fatalf("python top-level self method event missing: %+v", file.Events)
	}

	review := ReviewAppliedPatchScope(&types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"pkg/axis.py"},
		PatchEffect: &record,
	}, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("python top-level self method is a regex heuristic and must NOT hard block (precise-signals red line): %+v", review)
	}
	finding := patchReviewFindingByCode(review, "python_top_level_self_method")
	if finding.Category != types.PatchReviewCategorySemanticCoverage ||
		finding.CoverageStatus != types.PatchReviewCoverageUnknown ||
		finding.ImpactKind != types.PatchReviewImpactKindEffectFollowup {
		t.Fatalf("python top-level self method should be a soft semantic-coverage advisory: %+v", finding)
	}
}

func TestAnnotatePatchEffectPythonDuplicateSymbolIsSoftAdvisory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sympy/tensor/array"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sympy/tensor/array/ndim_array.py"), []byte("class NDimArray:\n    def __len__(self):\n        return 1\n\n    def __len__(self):\n        return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `diff --git a/sympy/tensor/array/ndim_array.py b/sympy/tensor/array/ndim_array.py
--- a/sympy/tensor/array/ndim_array.py
+++ b/sympy/tensor/array/ndim_array.py
@@ -1,3 +1,6 @@
 class NDimArray:
     def __len__(self):
         return 1
+
+    def __len__(self):
+        return 2
`
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, "sympy/tensor/array/ndim_array.py")
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if !patchEffectHasEvent(*file, "python_duplicate_symbol_added") {
		t.Fatalf("duplicate symbol event missing: %+v", file.Events)
	}

	review := ReviewAppliedPatchScope(&types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"sympy/tensor/array/ndim_array.py"},
		PatchEffect: &record,
	}, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("python duplicate symbol is a decorator-blind regex heuristic and must NOT hard block (precise-signals red line; @property/@setter/@overload pairs are legitimate): %+v", review)
	}
	finding := patchReviewFindingByCode(review, "python_duplicate_symbol_added")
	if finding.Category != types.PatchReviewCategorySemanticCoverage ||
		finding.CoverageStatus != types.PatchReviewCoverageUnknown ||
		finding.ImpactKind != types.PatchReviewImpactKindEffectFollowup {
		t.Fatalf("python duplicate symbol should be a soft semantic-coverage advisory: %+v", finding)
	}
}

func TestAnnotatePatchEffectPythonAddedReturnBeforeExistingBodyIsSoftAdvisory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `class Field:
    def _check(self):
        return []
        if self.max_length is None:
            return []
        return []
`
	if err := os.WriteFile(filepath.Join(root, "pkg", "field.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `diff --git a/pkg/field.py b/pkg/field.py
--- a/pkg/field.py
+++ b/pkg/field.py
@@ -1,5 +1,6 @@
 class Field:
     def _check(self):
+        return []
         if self.max_length is None:
             return []
         return []
`
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, "pkg/field.py")
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if !patchEffectHasEvent(*file, "python_unreachable_body_after_added_return") {
		t.Fatalf("unreachable body event missing: %+v", file.Events)
	}

	review := ReviewAppliedPatchScope(&types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"pkg/field.py"},
		PatchEffect: &record,
	}, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("unreachable body after added return is an indentation heuristic and must NOT hard block (precise-signals red line): %+v", review)
	}
	finding := patchReviewFindingByCode(review, "python_unreachable_body_after_added_return")
	if finding.Category != types.PatchReviewCategorySemanticCoverage ||
		finding.CoverageStatus != types.PatchReviewCoverageUnknown ||
		finding.ImpactKind != types.PatchReviewImpactKindEffectFollowup {
		t.Fatalf("unreachable body after added return should be a soft semantic-coverage advisory: %+v", finding)
	}
}

func TestAnnotatePatchEffectPythonNestedReturnDoesNotMarkFollowingFunctionBodyUnreachable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `def check(flag):
    if flag:
        return []
    return ["fallback"]
`
	if err := os.WriteFile(filepath.Join(root, "pkg", "field.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `diff --git a/pkg/field.py b/pkg/field.py
--- a/pkg/field.py
+++ b/pkg/field.py
@@ -1,3 +1,4 @@
 def check(flag):
     if flag:
+        return []
     return ["fallback"]
`
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, "pkg/field.py")
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if patchEffectHasEvent(*file, "python_unreachable_body_after_added_return") {
		t.Fatalf("nested guard return should not mark the following function body unreachable: %+v", file.Events)
	}
}

func TestAnnotatePatchEffectPythonDocstringSectionExecutableIsSoftAdvisory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sympy/ntheory"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `def nthroot_mod(a, n, p, all_roots=False):
    """
    Find the solutions to x**n = a mod p
    Parameters
    if a % p == 0:
        if all_roots:
            return [0]
        return 0
    ==========
    a : integer
    """
    return []
`
	if err := os.WriteFile(filepath.Join(root, "sympy/ntheory/residue_ntheory.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `diff --git a/sympy/ntheory/residue_ntheory.py b/sympy/ntheory/residue_ntheory.py
--- a/sympy/ntheory/residue_ntheory.py
+++ b/sympy/ntheory/residue_ntheory.py
@@ -1,8 +1,12 @@
 def nthroot_mod(a, n, p, all_roots=False):
     """
     Find the solutions to x**n = a mod p
     Parameters
+    if a % p == 0:
+        if all_roots:
+            return [0]
+        return 0
     ==========
     a : integer
`
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, "sympy/ntheory/residue_ntheory.py")
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if !patchEffectHasEvent(*file, "python_docstring_section_executable_added") {
		t.Fatalf("docstring executable section event missing: %+v", file.Events)
	}

	review := ReviewAppliedPatchScope(&types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"sympy/ntheory/residue_ntheory.py"},
		PatchEffect: &record,
	}, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("docstring executable section is a line-shape heuristic and must NOT hard block (precise-signals red line): %+v", review)
	}
	finding := patchReviewFindingByCode(review, "python_docstring_section_executable_added")
	if finding.Category != types.PatchReviewCategorySemanticCoverage ||
		finding.CoverageStatus != types.PatchReviewCoverageUnknown ||
		finding.ImpactKind != types.PatchReviewImpactKindEffectFollowup {
		t.Fatalf("docstring executable section should be a soft semantic-coverage advisory: %+v", finding)
	}
}

func TestAnnotatePatchEffectPythonDocstringCodeExampleDoesNotHardBlock(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `def example(value):
    """
    Examples
    ========

    Use a normal code block::
        if value:
            return value
    """
    return value
`
	if err := os.WriteFile(filepath.Join(root, "pkg", "example.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `diff --git a/pkg/example.py b/pkg/example.py
--- a/pkg/example.py
+++ b/pkg/example.py
@@ -5,6 +5,8 @@
     Use a normal code block::
+        if value:
+            return value
     """
     return value
`
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, "pkg/example.py")
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if patchEffectHasEvent(*file, "python_docstring_section_executable_added") {
		t.Fatalf("docstring code example should not emit section-disruption event: %+v", file.Events)
	}
}

func TestAnnotatePatchEffectDuplicateInsertedBlockIsSoftAdvisory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sympy/ntheory"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `def nthroot_mod(a, n, p, all_roots=False):
    if not isprime(p):
        raise NotImplementedError("Not implemented for composite p")

    if a % p == 0:
        if all_roots:
            return [0]
        return 0
    if a % p == 0:
        if all_roots:
            return [0]
        return 0
    return None
`
	if err := os.WriteFile(filepath.Join(root, "sympy/ntheory/residue_ntheory.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `diff --git a/sympy/ntheory/residue_ntheory.py b/sympy/ntheory/residue_ntheory.py
--- a/sympy/ntheory/residue_ntheory.py
+++ b/sympy/ntheory/residue_ntheory.py
@@ -1,4 +1,12 @@
 def nthroot_mod(a, n, p, all_roots=False):
     if not isprime(p):
         raise NotImplementedError("Not implemented for composite p")

+    if a % p == 0:
+        if all_roots:
+            return [0]
+        return 0
+    if a % p == 0:
+        if all_roots:
+            return [0]
+        return 0
     return None
`
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, "sympy/ntheory/residue_ntheory.py")
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if !patchEffectHasEvent(*file, "duplicate_inserted_block_added") {
		t.Fatalf("duplicate inserted block event missing: %+v", file.Events)
	}

	review := ReviewAppliedPatchScope(&types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"sympy/ntheory/residue_ntheory.py"},
		PatchEffect: &record,
	}, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("duplicate inserted block is an exact-text heuristic and must NOT hard block (precise-signals red line; repeated case arms / builder chains are legitimate): %+v", review)
	}
	finding := patchReviewFindingByCode(review, "duplicate_inserted_block_added")
	if finding.Category != types.PatchReviewCategorySemanticCoverage ||
		finding.CoverageStatus != types.PatchReviewCoverageUnknown ||
		finding.ImpactKind != types.PatchReviewImpactKindEffectFollowup {
		t.Fatalf("duplicate inserted block should be a soft semantic-coverage advisory: %+v", finding)
	}
}

func TestAnnotatePatchEffectNonASCIISourceCommentWarns(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sympy/ntheory"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `def nthroot_mod(a, n, p, all_roots=False):
    a, n, p = as_int(a), as_int(n), as_int(p)
    # a % p == 0 时, x^n == 0 (mod p)
    if a % p == 0:
        return 0
    return None
`
	if err := os.WriteFile(filepath.Join(root, "sympy/ntheory/residue_ntheory.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `diff --git a/sympy/ntheory/residue_ntheory.py b/sympy/ntheory/residue_ntheory.py
--- a/sympy/ntheory/residue_ntheory.py
+++ b/sympy/ntheory/residue_ntheory.py
@@ -1,3 +1,6 @@
 def nthroot_mod(a, n, p, all_roots=False):
     a, n, p = as_int(a), as_int(n), as_int(p)
+    # a % p == 0 时, x^n == 0 (mod p)
+    if a % p == 0:
+        return 0
     return None
`
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, "sympy/ntheory/residue_ntheory.py")
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if !patchEffectHasEvent(*file, "non_ascii_source_comment_added") {
		t.Fatalf("non-ASCII source comment event missing: %+v", file.Events)
	}

	review := ReviewAppliedPatchScope(&types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"sympy/ntheory/residue_ntheory.py"},
		PatchEffect: &record,
	}, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("non-ASCII source comment is a soft semantic finding, not a hard block: %+v", review)
	}
	finding := patchReviewFindingByCode(review, "non_ascii_source_comment_added")
	if finding.Category != types.PatchReviewCategorySemanticCoverage ||
		finding.CoverageStatus != types.PatchReviewCoverageUnknown ||
		finding.ImpactKind != types.PatchReviewImpactKindEffectFollowup {
		t.Fatalf("non-ASCII source comment should be semantic coverage unknown: %+v", finding)
	}
}

func TestAnnotatePatchEffectNearbyDuplicateStatementWarns(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sympy/ntheory"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `def nthroot_mod(a, n, p, all_roots=False):
    from sympy.core.numbers import igcdex
    a, n, p = as_int(a), as_int(n), as_int(p)
    if a % p == 0:
        if all_roots:
            return [0]
        return 0
    a, n, p = as_int(a), as_int(n), as_int(p)
    if n == 2:
        return sqrt_mod(a, p, all_roots)
`
	if err := os.WriteFile(filepath.Join(root, "sympy/ntheory/residue_ntheory.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `diff --git a/sympy/ntheory/residue_ntheory.py b/sympy/ntheory/residue_ntheory.py
--- a/sympy/ntheory/residue_ntheory.py
+++ b/sympy/ntheory/residue_ntheory.py
@@ -1,4 +1,10 @@
 def nthroot_mod(a, n, p, all_roots=False):
     from sympy.core.numbers import igcdex
     a, n, p = as_int(a), as_int(n), as_int(p)
+    if a % p == 0:
+        if all_roots:
+            return [0]
+        return 0
+    a, n, p = as_int(a), as_int(n), as_int(p)
     if n == 2:
         return sqrt_mod(a, p, all_roots)
`
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, "sympy/ntheory/residue_ntheory.py")
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if !patchEffectHasEvent(*file, "nearby_duplicate_statement_added") {
		t.Fatalf("nearby duplicate statement event missing: %+v", file.Events)
	}

	review := ReviewAppliedPatchScope(&types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"sympy/ntheory/residue_ntheory.py"},
		PatchEffect: &record,
	}, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("nearby duplicate statement is a soft semantic finding, not a hard block: %+v", review)
	}
	finding := patchReviewFindingByCode(review, "nearby_duplicate_statement_added")
	if finding.Category != types.PatchReviewCategorySemanticCoverage ||
		finding.CoverageStatus != types.PatchReviewCoverageUnknown ||
		finding.ImpactKind != types.PatchReviewImpactKindEffectFollowup {
		t.Fatalf("nearby duplicate statement should be semantic coverage unknown: %+v", finding)
	}
}

func TestAnnotatePatchEffectProductionTestScaffoldWarns(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src/_pytest/assertion"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src/_pytest/assertion/rewrite.py"), []byte("def helper():\n    pass\n\nclass _AssertionRewriterTests:\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `diff --git a/src/_pytest/assertion/rewrite.py b/src/_pytest/assertion/rewrite.py
--- a/src/_pytest/assertion/rewrite.py
+++ b/src/_pytest/assertion/rewrite.py
@@ -1,2 +1,5 @@
 def helper():
     pass
+
+class _AssertionRewriterTests:
+    pass
`
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, "src/_pytest/assertion/rewrite.py")
	if file == nil || file.PathRole != types.SourcePathRoleProduction {
		t.Fatalf("patch effect production file missing: %+v", record.Files)
	}
	if !patchEffectHasEvent(*file, "production_test_scaffold_added") {
		t.Fatalf("production test scaffold event missing: %+v", file.Events)
	}

	review := ReviewAppliedPatchScope(&types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"src/_pytest/assertion/rewrite.py"},
		PatchEffect: &record,
	}, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("production test scaffold is soft semantic coverage, not a hard block: %+v", review)
	}
	finding := patchReviewFindingByCode(review, "production_test_scaffold_added")
	if finding.Category != types.PatchReviewCategorySemanticCoverage || finding.CoverageStatus != types.PatchReviewCoverageUnknown {
		t.Fatalf("production test scaffold finding should be semantic coverage unknown: %+v", finding)
	}
}

func TestAnnotatePatchEffectPythonNestedStringKeyAccessWarns(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "widget.py"), []byte("class BoundWidget:\n    @property\n    def id_for_label(self):\n        return self.data['attrs']['id']\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `diff --git a/pkg/widget.py b/pkg/widget.py
--- a/pkg/widget.py
+++ b/pkg/widget.py
@@ -1,3 +1,4 @@
 class BoundWidget:
     @property
     def id_for_label(self):
+        return self.data['attrs']['id']
`
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, "pkg/widget.py")
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if !patchEffectHasEvent(*file, "python_nested_string_key_direct_access_added") {
		t.Fatalf("nested key access event missing: %+v", file.Events)
	}

	review := ReviewAppliedPatchScope(&types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"pkg/widget.py"},
		PatchEffect: &record,
	}, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("nested key access is a soft semantic finding, not a hard block: %+v", review)
	}
	finding := patchReviewFindingByCode(review, "python_nested_string_key_direct_access_added")
	if finding.Category != types.PatchReviewCategorySemanticCoverage ||
		finding.CoverageStatus != types.PatchReviewCoverageUnknown ||
		finding.ImpactKind != types.PatchReviewImpactKindEffectFollowup {
		t.Fatalf("nested key access finding should be semantic coverage unknown: %+v", finding)
	}
}

func TestAnnotatePatchEffectNestedCollectionBranchExclusionWarnsAcrossProviders(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		after []string
	}{
		{
			name: "python",
			path: "pkg/fields.py",
			after: []string{
				"for value, label in choices:",
				"    if isinstance(label, (list, tuple)):",
				"        is_flat = False",
				"        break",
				"if max_value_length > self.max_length:",
				"    return [checks.Error(\"too long\")]",
			},
		},
		{
			name: "javascript",
			path: "src/fields.js",
			after: []string{
				"for (const choice of choices) {",
				"  if (Array.isArray(choice[1])) {",
				"    skipNested = true;",
				"    break;",
				"  }",
				"  if (String(choice[0]).length > maxLength) { throw new Error('too long'); }",
				"}",
			},
		},
		{
			name: "typescript",
			path: "src/fields.ts",
			after: []string{
				"for (const choice of choices) {",
				"  if (Array.isArray(choice[1])) {",
				"    skipNested = true;",
				"    break;",
				"  }",
				"  if (String(choice[0]).length > maxLength) { throw new Error('too long'); }",
				"}",
			},
		},
		{
			name: "ruby",
			path: "lib/fields.rb",
			after: []string{
				"choices.each do |choice|",
				"  if choice[1].is_a?(Array)",
				"    next",
				"  end",
				"  raise 'too long' if choice[0].length > max_length",
				"end",
			},
		},
		{
			name: "java",
			path: "src/main/java/Fields.java",
			after: []string{
				"for (Object value : values) {",
				"  if (value instanceof List) {",
				"    skipNested = true;",
				"    break;",
				"  }",
				"  if (value.toString().length() > maxLength) { throw new IllegalArgumentException(\"too long\"); }",
				"}",
			},
		},
		{
			name: "kotlin",
			path: "src/main/kotlin/Fields.kt",
			after: []string{
				"for (value in values) {",
				"  if (value is List<*>) {",
				"    skipNested = true",
				"    break",
				"  }",
				"  if (value.toString().length > maxLength) { throw IllegalArgumentException(\"too long\") }",
				"}",
			},
		},
		{
			name: "go",
			path: "pkg/fields.go",
			after: []string{
				"for _, value := range values {",
				"  if _, ok := value.([]string); ok {",
				"    continue",
				"  }",
				"  if len(fmt.Sprint(value)) > maxLength {",
				"    return fmt.Errorf(\"too long\")",
				"  }",
				"}",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			abs := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, []byte(strings.Join(tc.after, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			diff := patchEffectSingleHunkDiff(tc.path, []string{"pass"}, tc.after)
			record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
			AnnotatePatchEffectStructuredFileParses(&record, root)
			file := findPatchEffectFile(record, tc.path)
			if file == nil {
				t.Fatalf("patch effect file missing: %+v", record.Files)
			}
			if !patchEffectHasEvent(*file, "nested_collection_branch_exclusion_added") {
				t.Fatalf("nested collection exclusion event missing: %+v", file.Events)
			}

			review := ReviewAppliedPatchScope(&types.ChangePlan{
				ID:          "plan-1",
				Status:      types.PlanStatusAppliedPendingVerify,
				TargetPaths: []string{tc.path},
				PatchEffect: &record,
			}, types.ChangePlanSlice{})
			if review.HardBlock {
				t.Fatalf("nested collection exclusion is a soft semantic finding, not a hard block: %+v", review)
			}
			finding := patchReviewFindingByCode(review, "nested_collection_branch_exclusion_added")
			if finding.Category != types.PatchReviewCategorySemanticCoverage ||
				finding.CoverageStatus != types.PatchReviewCoverageUnknown ||
				finding.ImpactKind != types.PatchReviewImpactKindEffectFollowup {
				t.Fatalf("nested collection exclusion finding should be semantic coverage unknown: %+v", finding)
			}
		})
	}
}

func TestAnnotatePatchEffectNestedCollectionHandledBranchDoesNotWarn(t *testing.T) {
	root := t.TempDir()
	path := "pkg/fields.py"
	after := []string{
		"values = []",
		"for value, label in choices:",
		"    if isinstance(label, (list, tuple)):",
		"        values.extend(choice for choice, _ in label)",
		"    else:",
		"        values.append(value)",
		"if max_value_length > self.max_length:",
		"    return [checks.Error(\"too long\")]",
	}
	abs := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(strings.Join(after, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := patchEffectSingleHunkDiff(path, []string{"pass"}, after)
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, path)
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if patchEffectHasEvent(*file, "nested_collection_branch_exclusion_added") {
		t.Fatalf("handled nested collection branch should not emit exclusion event: %+v", file.Events)
	}
}

func TestAnnotatePatchEffectOwnerBoundaryWarnings(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		before    []string
		after     []string
		eventCode string
	}{
		{
			name:      "python caller return wrapper",
			path:      "pkg/predict.py",
			before:    []string{"return estimator.predict(X)"},
			after:     []string{"return np.asarray(estimator.predict(X))"},
			eventCode: "caller_return_shape_adapter_added",
		},
		{
			name:      "javascript caller return wrapper",
			path:      "src/predict.js",
			before:    []string{"return service.load(input);"},
			after:     []string{"return Promise.resolve(service.load(input));"},
			eventCode: "caller_return_shape_adapter_added",
		},
		{
			name:      "java caller return wrapper",
			path:      "src/main/java/Predictor.java",
			before:    []string{"return service.compute(input);"},
			after:     []string{"return Result.wrap(service.compute(input));"},
			eventCode: "caller_return_shape_adapter_added",
		},
		{
			name:      "python diagnostic suppression",
			path:      "pkg/warnings.py",
			before:    []string{`warnings.warn("slow path")`},
			after:     []string{"if verbose:", `    warnings.warn("slow path")`},
			eventCode: "diagnostic_signal_conditionally_suppressed",
		},
		{
			name:      "typescript diagnostic suppression",
			path:      "src/warnings.ts",
			before:    []string{`logger.warn("slow path");`},
			after:     []string{"if (verbose) {", `  logger.warn("slow path");`, "}"},
			eventCode: "diagnostic_signal_conditionally_suppressed",
		},
		{
			name:      "python external private state",
			path:      "pkg/state.py",
			before:    []string{"pass"},
			after:     []string{"other._cache = value"},
			eventCode: "external_private_state_sync_workaround",
		},
		{
			name:      "ruby external private state",
			path:      "lib/state.rb",
			before:    []string{"nil"},
			after:     []string{"other.instance_variable_set(:@cache, value)"},
			eventCode: "external_private_state_sync_workaround",
		},
		{
			name:      "kotlin external private state",
			path:      "src/main/kotlin/State.kt",
			before:    []string{"return"},
			after:     []string{"other._cache = value"},
			eventCode: "external_private_state_sync_workaround",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			abs := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, []byte(strings.Join(tc.after, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			diff := patchEffectSingleHunkDiff(tc.path, tc.before, tc.after)
			record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
			AnnotatePatchEffectStructuredFileParses(&record, root)
			file := findPatchEffectFile(record, tc.path)
			if file == nil {
				t.Fatalf("patch effect file missing: %+v", record.Files)
			}
			if !patchEffectHasEvent(*file, tc.eventCode) {
				t.Fatalf("event %s missing: %+v", tc.eventCode, file.Events)
			}

			review := ReviewAppliedPatchScope(&types.ChangePlan{
				ID:          "plan-1",
				Status:      types.PlanStatusAppliedPendingVerify,
				TargetPaths: []string{tc.path},
				PatchEffect: &record,
			}, types.ChangePlanSlice{})
			if review.HardBlock {
				t.Fatalf("owner-boundary signal is a soft semantic finding, not a hard block: %+v", review)
			}
			finding := patchReviewFindingByCode(review, tc.eventCode)
			if finding.Category != types.PatchReviewCategorySemanticCoverage ||
				finding.CoverageStatus != types.PatchReviewCoverageUnknown ||
				finding.ImpactKind != types.PatchReviewImpactKindEffectFollowup {
				t.Fatalf("owner-boundary finding should be semantic coverage unknown: %+v", finding)
			}
		})
	}
}

func TestAnnotatePatchEffectOwnerBoundaryInternalReceiverIgnored(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		after string
	}{
		{name: "python self", path: "pkg/state.py", after: "self._cache = value"},
		{name: "javascript this", path: "src/state.js", after: "this._cache = value;"},
		{name: "java this", path: "src/main/java/State.java", after: "this._cache = value;"},
		{name: "ruby self", path: "lib/state.rb", after: "self.instance_variable_set(:@cache, value)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			abs := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, []byte(tc.after+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			diff := patchEffectSingleHunkDiff(tc.path, []string{"pass"}, []string{tc.after})
			record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
			AnnotatePatchEffectStructuredFileParses(&record, root)
			file := findPatchEffectFile(record, tc.path)
			if file == nil {
				t.Fatalf("patch effect file missing: %+v", record.Files)
			}
			if patchEffectHasEvent(*file, "external_private_state_sync_workaround") {
				t.Fatalf("internal receiver should not emit external private-state event: %+v", file.Events)
			}
		})
	}
}

func TestAnnotatePatchEffectMultiLanguageLineShapeWarnings(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		line      string
		eventCode string
	}{
		{
			name:      "javascript nested string key access",
			path:      "src/widget.js",
			line:      `const id = data["attrs"]["id"];`,
			eventCode: "javascript_nested_string_key_direct_access_added",
		},
		{
			name:      "typescript nested string key access",
			path:      "src/widget.ts",
			line:      `const id = this.data["attrs"]["id"];`,
			eventCode: "typescript_nested_string_key_direct_access_added",
		},
		{
			name:      "ruby nested hash key access",
			path:      "lib/widget.rb",
			line:      `id = data[:attrs][:id]`,
			eventCode: "ruby_nested_key_direct_access_added",
		},
		{
			name:      "java chained map get",
			path:      "src/main/java/Widget.java",
			line:      `String id = data.get("attrs").get("id");`,
			eventCode: "java_chained_string_map_get_added",
		},
		{
			name:      "kotlin chained map get",
			path:      "src/main/kotlin/Widget.kt",
			line:      `val id = data.get("attrs").get("id")`,
			eventCode: "kotlin_chained_string_map_get_added",
		},
		{
			name:      "go nested map assignment",
			path:      "pkg/widget.go",
			line:      `data["attrs"]["id"] = id`,
			eventCode: "go_nested_string_map_assignment_added",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			abs := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, []byte(tc.line+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			diff := "diff --git a/" + tc.path + " b/" + tc.path + "\n" +
				"--- a/" + tc.path + "\n" +
				"+++ b/" + tc.path + "\n" +
				"@@ -0,0 +1 @@\n" +
				"+" + tc.line + "\n"
			record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
			AnnotatePatchEffectStructuredFileParses(&record, root)
			file := findPatchEffectFile(record, tc.path)
			if file == nil {
				t.Fatalf("patch effect file missing: %+v", record.Files)
			}
			if !patchEffectHasEvent(*file, tc.eventCode) {
				t.Fatalf("event %s missing: %+v", tc.eventCode, file.Events)
			}
			review := ReviewAppliedPatchScope(&types.ChangePlan{
				ID:          "plan-1",
				Status:      types.PlanStatusAppliedPendingVerify,
				TargetPaths: []string{tc.path},
				PatchEffect: &record,
			}, types.ChangePlanSlice{})
			if review.HardBlock {
				t.Fatalf("line shape event is a soft semantic finding, not a hard block: %+v", review)
			}
			finding := patchReviewFindingByCode(review, tc.eventCode)
			if finding.Category != types.PatchReviewCategorySemanticCoverage || finding.CoverageStatus != types.PatchReviewCoverageUnknown {
				t.Fatalf("finding should be semantic coverage unknown: %+v", finding)
			}
		})
	}
}

func TestAnnotatePatchEffectBraceReturnBeforeExistingStatementWarnsAcrossProviders(t *testing.T) {
	cases := []struct {
		name string
		path string
		text string
		diff string
	}{
		{
			name: "javascript",
			path: "src/render.js",
			text: strings.Join([]string{
				"function render(value) {",
				"  return value;",
				"  compute(value);",
				"}",
				"",
			}, "\n"),
			diff: strings.Join([]string{
				"diff --git a/src/render.js b/src/render.js",
				"--- a/src/render.js",
				"+++ b/src/render.js",
				"@@ -1,3 +1,4 @@",
				" function render(value) {",
				"+  return value;",
				"   compute(value);",
				" }",
				"",
			}, "\n"),
		},
		{
			name: "typescript",
			path: "src/render.ts",
			text: strings.Join([]string{
				"export function render(value: string) {",
				"  return value;",
				"  compute(value);",
				"}",
				"",
			}, "\n"),
			diff: strings.Join([]string{
				"diff --git a/src/render.ts b/src/render.ts",
				"--- a/src/render.ts",
				"+++ b/src/render.ts",
				"@@ -1,3 +1,4 @@",
				" export function render(value: string) {",
				"+  return value;",
				"   compute(value);",
				" }",
				"",
			}, "\n"),
		},
		{
			name: "java",
			path: "src/main/java/Widget.java",
			text: strings.Join([]string{
				"class Widget {",
				"  String render(String value) {",
				"    return value;",
				"    compute(value);",
				"  }",
				"}",
				"",
			}, "\n"),
			diff: strings.Join([]string{
				"diff --git a/src/main/java/Widget.java b/src/main/java/Widget.java",
				"--- a/src/main/java/Widget.java",
				"+++ b/src/main/java/Widget.java",
				"@@ -1,5 +1,6 @@",
				" class Widget {",
				"   String render(String value) {",
				"+    return value;",
				"     compute(value);",
				"   }",
				" }",
				"",
			}, "\n"),
		},
		{
			name: "go",
			path: "pkg/render.go",
			text: strings.Join([]string{
				"package p",
				"",
				"func Render(value string) string {",
				"  return value",
				"  compute(value)",
				"}",
				"",
			}, "\n"),
			diff: strings.Join([]string{
				"diff --git a/pkg/render.go b/pkg/render.go",
				"--- a/pkg/render.go",
				"+++ b/pkg/render.go",
				"@@ -1,5 +1,6 @@",
				" package p",
				" ",
				" func Render(value string) string {",
				"+  return value",
				"   compute(value)",
				" }",
				"",
			}, "\n"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			abs := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, []byte(tc.text), 0o644); err != nil {
				t.Fatal(err)
			}
			record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", tc.diff)
			AnnotatePatchEffectStructuredFileParses(&record, root)
			file := findPatchEffectFile(record, tc.path)
			if file == nil {
				t.Fatalf("patch effect file missing: %+v", record.Files)
			}
			if !patchEffectHasEvent(*file, "brace_return_before_existing_statement_added") {
				t.Fatalf("brace return event missing: %+v", file.Events)
			}
			review := ReviewAppliedPatchScope(&types.ChangePlan{
				ID:          "plan-1",
				Status:      types.PlanStatusAppliedPendingVerify,
				TargetPaths: []string{tc.path},
				PatchEffect: &record,
			}, types.ChangePlanSlice{})
			if review.HardBlock {
				t.Fatalf("brace return signal is soft coverage, not a hard block: %+v", review)
			}
			finding := patchReviewFindingByCode(review, "brace_return_before_existing_statement_added")
			if finding.Category != types.PatchReviewCategorySemanticCoverage || finding.CoverageStatus != types.PatchReviewCoverageUnknown {
				t.Fatalf("brace return finding should be semantic coverage unknown: %+v", finding)
			}
		})
	}
}

func TestAnnotatePatchEffectBraceGuardReturnDoesNotWarn(t *testing.T) {
	root := t.TempDir()
	path := "src/render.js"
	text := strings.Join([]string{
		"function render(value) {",
		"  if (!value) {",
		"    return value;",
		"  }",
		"  compute(value);",
		"}",
		"",
	}, "\n")
	abs := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := strings.Join([]string{
		"diff --git a/src/render.js b/src/render.js",
		"--- a/src/render.js",
		"+++ b/src/render.js",
		"@@ -1,5 +1,6 @@",
		" function render(value) {",
		"   if (!value) {",
		"+    return value;",
		"   }",
		"   compute(value);",
		" }",
		"",
	}, "\n")
	record := PatchEffectRecordFromUnifiedDiff("plan-1", "slice-1", "applied_commit", "HEAD^", "abc123", diff)
	AnnotatePatchEffectStructuredFileParses(&record, root)
	file := findPatchEffectFile(record, path)
	if file == nil {
		t.Fatalf("patch effect file missing: %+v", record.Files)
	}
	if patchEffectHasEvent(*file, "brace_return_before_existing_statement_added") {
		t.Fatalf("guard return at block end should not emit brace return event: %+v", file.Events)
	}
}

func TestPatchEffectSourceProviderRegistryCoverage(t *testing.T) {
	wantKinds := map[string]string{
		"pkg/widget.py":              "python",
		"src/widget.js":              "javascript",
		"src/widget.jsx":             "javascript",
		"src/widget.mjs":             "javascript",
		"src/widget.cjs":             "javascript",
		"src/widget.ts":              "typescript",
		"src/widget.tsx":             "typescript",
		"src/widget.mts":             "typescript",
		"src/widget.cts":             "typescript",
		"lib/widget.rb":              "ruby",
		"src/main/java/Widget.java":  "java",
		"src/main/kotlin/Widget.kt":  "kotlin",
		"src/main/kotlin/Widget.kts": "kotlin",
		"pkg/widget.go":              "go",
	}
	for path, want := range wantKinds {
		t.Run(path, func(t *testing.T) {
			provider := patchEffectSourceProviderForPath(path)
			if provider == nil {
				t.Fatalf("missing provider for %s", path)
			}
			if provider.Kind != want {
				t.Fatalf("provider kind for %s = %q, want %q", path, provider.Kind, want)
			}
			if got := patchEffectSourceShapeKind(path); got != want {
				t.Fatalf("source shape kind for %s = %q, want %q", path, got, want)
			}
			if len(provider.LineRules) == 0 {
				t.Fatalf("provider %q has no line-shape rules", provider.Kind)
			}
			if provider.ProductionTestRule.code == "" || provider.ProductionTestRule.match == nil {
				t.Fatalf("provider %q has no production-test scaffold rule: %+v", provider.Kind, provider.ProductionTestRule)
			}
			if !provider.OwnerBoundary.ReturnWrapper {
				t.Fatalf("provider %q has no owner-boundary return-wrapper rule", provider.Kind)
			}
			if provider.CollectionBoundary.ShapeCheck == nil || provider.CollectionBoundary.ExclusionAction == nil ||
				provider.CollectionBoundary.ValidationSignal == nil {
				t.Fatalf("provider %q has incomplete collection-boundary rules: %+v", provider.Kind, provider.CollectionBoundary)
			}
		})
	}

	seenKinds := map[string]bool{}
	seenExtensions := map[string]string{}
	for _, provider := range patchEffectSourceProviders {
		if provider.Kind == "" {
			t.Fatal("provider kind must be non-empty")
		}
		if seenKinds[provider.Kind] {
			t.Fatalf("duplicate provider kind %q", provider.Kind)
		}
		seenKinds[provider.Kind] = true
		if len(provider.Extensions) == 0 {
			t.Fatalf("provider %q has no extensions", provider.Kind)
		}
		for _, ext := range provider.Extensions {
			if ext == "" || ext[0] != '.' {
				t.Fatalf("provider %q has invalid extension %q", provider.Kind, ext)
			}
			if prior := seenExtensions[ext]; prior != "" {
				t.Fatalf("extension %q is claimed by both %q and %q", ext, prior, provider.Kind)
			}
			seenExtensions[ext] = provider.Kind
		}
	}
}

func TestPatchEffectOwnerBoundaryEventsRequireCoverage(t *testing.T) {
	for _, code := range []string{
		"brace_return_before_existing_statement_added",
		"caller_return_shape_adapter_added",
		"diagnostic_signal_conditionally_suppressed",
		"external_private_state_sync_workaround",
		"nested_collection_branch_exclusion_added",
	} {
		t.Run(code, func(t *testing.T) {
			if !PatchReviewEffectUnknownCoverage(code) {
				t.Fatalf("event %s must remain semantic coverage unknown", code)
			}
		})
	}
}

func findPatchEffectFile(record types.PatchEffectRecord, path string) *types.PatchEffectFile {
	for i := range record.Files {
		if record.Files[i].Path == path {
			return &record.Files[i]
		}
	}
	return nil
}

func patchEffectHasEvent(file types.PatchEffectFile, code string) bool {
	for _, event := range file.Events {
		if event.Code == code {
			return true
		}
	}
	return false
}

func patchEffectSingleHunkDiff(path string, before, after []string) string {
	var b strings.Builder
	b.WriteString("diff --git a/" + path + " b/" + path + "\n")
	b.WriteString("--- a/" + path + "\n")
	b.WriteString("+++ b/" + path + "\n")
	b.WriteString("@@ -1," + testItoa(len(before)) + " +1," + testItoa(len(after)) + " @@\n")
	for _, line := range before {
		b.WriteString("-" + line + "\n")
	}
	for _, line := range after {
		b.WriteString("+" + line + "\n")
	}
	return b.String()
}

func testItoa(n int) string {
	if n <= 0 {
		return "0"
	}
	return fmt.Sprintf("%d", n)
}
