package writeflow

import (
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

func findPatchEffectFile(record types.PatchEffectRecord, path string) *types.PatchEffectFile {
	for i := range record.Files {
		if record.Files[i].Path == path {
			return &record.Files[i]
		}
	}
	return nil
}
