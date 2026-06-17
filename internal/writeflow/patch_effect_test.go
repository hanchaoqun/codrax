package writeflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

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

func TestAnnotatePatchEffectPythonTopLevelSelfMethodHardBlocks(t *testing.T) {
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
	if !review.HardBlock || !patchReviewHasFinding(review, "python_top_level_self_method") {
		t.Fatalf("python top-level self method should hard block review: %+v", review)
	}
}

func TestAnnotatePatchEffectPythonDuplicateSymbolHardBlocks(t *testing.T) {
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
	if !review.HardBlock || !patchReviewHasFinding(review, "python_duplicate_symbol_added") {
		t.Fatalf("python duplicate symbol should hard block review: %+v", review)
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
	if finding.Category != types.PatchReviewCategorySemanticCoverage || finding.CoverageStatus != types.PatchReviewCoverageUnknown {
		t.Fatalf("nested key access finding should be semantic coverage unknown: %+v", finding)
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
