package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The live W2 shape: original kept `{` and the statement on separate
// lines; the patch joins them.
func TestAnalyzePatchLineCompression_FlagsBraceJoin(t *testing.T) {
	patch := `--- a/repository.c
+++ b/repository.c
@@ -10,4 +10,2 @@
-    if ((error = cb_result != 0)) {
-        return error;
-    }
+    if ((error = cb_result) != 0) {        return error;    }
`
	notes := analyzePatchLineCompression("repository.c", patch)
	if len(notes) != 1 || !strings.Contains(notes[0], "separate lines") {
		t.Fatalf("brace-join compression must be flagged once, got %v", notes)
	}
}

func TestAnalyzePatchLineCompression_NormalEditsNotFlagged(t *testing.T) {
	cases := []string{
		// structure-preserving rewrite
		"@@ -1,3 +1,3 @@\n-    if (a != 0) {\n-        return a;\n-    }\n+    if ((a = b) != 0) {\n+        return a;\n+    }\n",
		// pure insertion, no removed side
		"@@ -1,0 +1,2 @@\n+    int x = 1;\n+    use(x);\n",
		// original ALREADY single-line style — not a compression
		"@@ -1,1 +1,1 @@\n-    if (a) { return a; }\n+    if (b) { return b; }\n",
		// multi-statement line where original also had two semicolons
		"@@ -1,1 +1,1 @@\n-    a = 1; b = 2;\n+    a = 3; b = 4;\n",
	}
	for i, patch := range cases {
		if notes := analyzePatchLineCompression("f.c", patch); len(notes) != 0 {
			t.Fatalf("case %d: structure-preserving edit must not be flagged, got %v", i, notes)
		}
	}
}

func TestPatchStyleAdvisoryNote_AdvisoryNeverChangesAcceptance(t *testing.T) {
	changes := []types.FileChange{{Path: "f.c", Kind: "patch", Patch: "@@ -1,2 +1,1 @@\n-if (a) {\n-    go();\n+if (a) { go(); }\n"}}
	note := patchStyleAdvisoryNote(changes)
	if note == "" || !strings.Contains(note, "accepted as-is") {
		t.Fatalf("advisory must state the plan is accepted, got %q", note)
	}
	if patchStyleAdvisoryNote(nil) != "" {
		t.Fatal("no changes → no note")
	}
}
