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

// The live patch_go_typo shape: the diff fixes a token but absorbs the
// next line's bare `}` onto the statement line (`-stmt` `-}` → `+stmt}`),
// silently changing the file's line structure.
func TestAnalyzePatchLineCompression_FlagsClosingBraceJoin(t *testing.T) {
	patch := `--- a/main.go
+++ b/main.go
@@ -22,8 +22,7 @@
 	if name == "" {
 		name = "world"
 	}
-	retrun fmt.Sprintf("Hello, %s!", name)
-}
+	return fmt.Sprintf("Hello, %s!", name)}
`
	notes := analyzePatchLineCompression("main.go", patch)
	if len(notes) != 1 || !strings.Contains(notes[0], "closing `}`") {
		t.Fatalf("closing-brace join must be flagged once, got %v", notes)
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
		// removed block close stays a separate added line — no join
		"@@ -1,2 +1,2 @@\n-    return a\n-}\n+    return b\n+}\n",
		// empty composite literal `{}` suffix is not a brace join
		"@@ -1,2 +1,1 @@\n-    m := old()\n-}\n+    m := map[string]int{}\n",
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

// The once-only nudge: the first line-compressing emission bounces
// with re-emit guidance; the second identical emission is accepted
// (advisory note only) — a bounded retry-hint, never a gate.
func TestEmitChangePlan_LineStructureNudgeFiresOnce(t *testing.T) {
	mu := types.NewMutableState("nudge")
	if !mu.TestAndSetPatchStyleNudge() {
		t.Fatal("first test-and-set must report the nudge may fire")
	}
	if mu.TestAndSetPatchStyleNudge() {
		t.Fatal("second test-and-set must report already consumed")
	}
	var nilState *types.MutableState
	if nilState.TestAndSetPatchStyleNudge() {
		t.Fatal("nil state must never fire the nudge")
	}
}
