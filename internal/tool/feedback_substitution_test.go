package tool

import (
	"strings"
	"testing"
)

// TestDetectSubstitutionInContextMistake_PatchGoTypoCase pins the
// canonical 2026-04-26 patch_go_typo eval failure: LLM kept the
// buggy `retrun` line in CONTEXT, fabricated a `\t` removal, and
// added the corrected `return` line — the diff syntactically parses
// but git refuses to apply because the file would contain BOTH lines.
//
// The detector must catch this and surface a substitution-mistake
// hint with both the LLM's `+` line and the highly-similar `' '`
// line so the planner sees the specific error class on retry.
func TestDetectSubstitutionInContextMistake_PatchGoTypoCase(t *testing.T) {
	patch := "--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -22,6 +22,6 @@ func greet(name string) string {\n" +
		" \t\tname = \"world\"\n" +
		" \t}\n" +
		" \tretrun fmt.Sprintf(\"Hello, %s!\", name)\n" +
		"-\t\n" +
		"+\treturn fmt.Sprintf(\"Hello, %s!\", name)\n" +
		" }\n"
	hint := detectSubstitutionInContextMistake(patch)
	if hint == "" {
		t.Fatal("expected substitution-in-context hint, got empty")
	}
	musts := []string{
		"DETECTED LIKELY MISTAKE",
		"retrun",
		"return",
		"ORIGINAL with `-`",
	}
	for _, m := range musts {
		if !strings.Contains(hint, m) {
			t.Errorf("hint missing required text %q\nhint=%s", m, hint)
		}
	}
}

// TestDetectSubstitutionInContextMistake_CleanPatchNoFalsePositive
// pins the negative case: a well-formed substitution where the
// buggy line IS on a `-` and the corrected line IS on a `+` must
// NOT trigger the detector. Without this clamp every patch would
// receive the substitution-mistake hint, drowning real mistakes.
func TestDetectSubstitutionInContextMistake_CleanPatchNoFalsePositive(t *testing.T) {
	patch := "--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -23,4 +23,4 @@ func greet(name string) string {\n" +
		" \t\tname = \"world\"\n" +
		" \t}\n" +
		"-\tretrun fmt.Sprintf(\"Hello, %s!\", name)\n" +
		"+\treturn fmt.Sprintf(\"Hello, %s!\", name)\n" +
		" }\n"
	hint := detectSubstitutionInContextMistake(patch)
	if hint != "" {
		t.Errorf("clean patch should not trigger substitution-mistake hint, got:\n%s", hint)
	}
}

// TestDetectSubstitutionInContextMistake_UnrelatedAddNoFalsePositive
// pins the second negative case: an addition that genuinely is new
// (not similar to any context line) must NOT trigger. e.g. adding a
// new statement inside an existing function body.
func TestDetectSubstitutionInContextMistake_UnrelatedAddNoFalsePositive(t *testing.T) {
	patch := "--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -10,3 +10,4 @@\n" +
		" func main() {\n" +
		" \tfmt.Println(\"hello\")\n" +
		"+\tlog.SetFlags(log.LstdFlags)\n" +
		" }\n"
	hint := detectSubstitutionInContextMistake(patch)
	if hint != "" {
		t.Errorf("genuine addition should not trigger substitution-mistake hint, got:\n%s", hint)
	}
}

// TestDetectSubstitutionInContextMistake_EmptyPatchSafe pins the
// nil-safety contract: empty / whitespace-only patches must return
// "" without panic. Defensive — composeApplyRejection callers can
// hand an empty payload when the planner emits a malformed plan.
func TestDetectSubstitutionInContextMistake_EmptyPatchSafe(t *testing.T) {
	cases := []string{"", "   ", "\n\n\n"}
	for _, p := range cases {
		if got := detectSubstitutionInContextMistake(p); got != "" {
			t.Errorf("empty/whitespace patch %q produced hint: %q", p, got)
		}
	}
}

// TestSubstitutionSimilarity_TypoCases pins the similarity scorer's
// behavior on the patterns we care about. The 0.7 threshold inside
// detectSubstitutionInContextMistake assumes these scores hold.
func TestSubstitutionSimilarity_TypoCases(t *testing.T) {
	cases := []struct {
		name     string
		a, b     string
		minScore float64
	}{
		// Single-letter swap (the patch_go_typo case): "retrun" vs "return".
		{"retrun_vs_return", "retrun fmt.Sprintf(\"x\", y)", "return fmt.Sprintf(\"x\", y)", 0.7},
		// One-character insertion: "name" vs "names".
		{"name_vs_names", "result = name", "result = names", 0.7},
		// Trailing change: "x = 1" vs "x = 2".
		{"x1_vs_x2", "x = 1", "x = 2", 0.7},
		// Truly different lines should score LOW (below 0.7 floor) so
		// the detector doesn't false-positive.
		{"unrelated_low_score", "fmt.Println(\"hi\")", "log.SetFlags(0)", 0.0},
	}
	for _, tc := range cases {
		got := substitutionSimilarity(tc.a, tc.b)
		if tc.minScore == 0 {
			if got >= 0.7 {
				t.Errorf("%s: expected low similarity, got %.2f for %q vs %q", tc.name, got, tc.a, tc.b)
			}
		} else if got < tc.minScore {
			t.Errorf("%s: expected similarity >= %.2f, got %.2f for %q vs %q", tc.name, tc.minScore, got, tc.a, tc.b)
		}
	}
}

// TestComposePatchRejection_SurfacesSubstitutionHint pins the
// integration: when the patch payload contains the substitution-in-
// context mistake AND git's stderr names a file:line, the rejection
// message must carry BOTH the file-content snippet AND the
// substitution-mistake hint. Without both pieces the planner sees
// only "compare context lines" generic guidance and re-derives the
// same wrong fix (the patch_go_typo 7-iteration trail).
func TestComposePatchRejection_SurfacesSubstitutionHint(t *testing.T) {
	patch := "--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -22,6 +22,6 @@\n" +
		" \tretrun fmt.Sprintf(\"x\", y)\n" +
		"-\t\n" +
		"+\treturn fmt.Sprintf(\"x\", y)\n"
	gitErr := "error: patch failed: nonexistent.go:24\nerror: nonexistent.go: patch does not apply"
	out := composePatchRejection("/tmp", "nonexistent.go", gitErr, patch)
	// The substitution hint must surface even when the file-snippet
	// path bails (file doesn't exist).
	if !strings.Contains(out, "DETECTED LIKELY MISTAKE") {
		t.Errorf("composePatchRejection must include substitution-mistake hint when patch shows the pattern; got:\n%s", out)
	}
	if !strings.Contains(out, "retrun") || !strings.Contains(out, "return") {
		t.Errorf("rejection message must echo both buggy and corrected lines; got:\n%s", out)
	}
}
