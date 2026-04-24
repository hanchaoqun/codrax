package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseGitConflictLocator covers the session-35 class fix:
// extract (file, line) from git's conflict stderr so the
// rejection message can enrich itself with actual file bytes.
// Git emits "patch failed: <file>:<N>" when a hunk's context
// doesn't match the file at the claimed position; other error
// classes ("corrupt patch at line N") refer to patch body and
// should NOT match this pattern (they're handled structurally
// via --recount).
func TestParseGitConflictLocator(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantFile string
		wantLine int
	}{
		{
			"context mismatch",
			"error: patch failed: Main.java:13\nerror: Main.java: patch does not apply",
			"Main.java", 13,
		},
		{
			"nested path",
			"error: patch failed: src/main/java/Foo.java:42",
			"src/main/java/Foo.java", 42,
		},
		{
			"corrupt patch (body-internal, NOT file:line)",
			"error: corrupt patch at line 11",
			"", 0,
		},
		{
			"empty input",
			"",
			"", 0,
		},
		{
			"non-numeric line",
			"error: patch failed: foo.txt:xyz",
			"", 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file, line := parseGitConflictLocator(c.input)
			if file != c.wantFile {
				t.Errorf("file = %q, want %q", file, c.wantFile)
			}
			if line != c.wantLine {
				t.Errorf("line = %d, want %d", line, c.wantLine)
			}
		})
	}
}

// TestConflictContextSnippet_RendersFileContext confirms that when
// git flags a context mismatch at line N, the helper reads the file
// and renders ±5 lines around N with a visual marker on the claim
// line. This is the exact ground-truth signal the LLM lacked in
// the session-35 Java eval — without it, the planner hallucinated
// content three retries in a row.
func TestConflictContextSnippet_RendersFileContext(t *testing.T) {
	dir := t.TempDir()
	content := ""
	for i := 1; i <= 20; i++ {
		// Plain numbered lines so the assertions can pin exact text.
		content += "line " + itoaStr(i) + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	gitErr := "error: patch failed: file.txt:10\nerror: file.txt: patch does not apply"
	snippet := conflictContextSnippet(dir, gitErr)
	if snippet == "" {
		t.Fatal("expected a rendered snippet; got empty")
	}
	// Window is ±5 lines around line 10, so lines 5-15 must appear.
	for i := 5; i <= 15; i++ {
		needle := "line " + itoaStr(i)
		if !strings.Contains(snippet, needle) {
			t.Errorf("snippet missing %q; got:\n%s", needle, snippet)
		}
	}
	// Lines outside the window must NOT appear.
	for _, i := range []int{4, 16} {
		needle := "line " + itoaStr(i)
		if strings.Contains(snippet, needle) {
			t.Errorf("snippet should not include %q; got:\n%s", needle, snippet)
		}
	}
	// The claim line must be visually marked.
	if !strings.Contains(snippet, "▶") {
		t.Errorf("snippet should mark the claim line with ▶; got:\n%s", snippet)
	}
	// Header mentions file name + line range.
	for _, needle := range []string{"file.txt", "5-15"} {
		if !strings.Contains(snippet, needle) {
			t.Errorf("snippet header missing %q; got:\n%s", needle, snippet)
		}
	}
}

// TestConflictContextSnippet_NoLocatorReturnsEmpty pins the guard:
// git errors that don't carry a file:line (corrupt patch at line N
// inside the body, generic errors, timeouts) silently return ""
// so the caller falls back to its generic hint.
func TestConflictContextSnippet_NoLocatorReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	for _, input := range []string{
		"",
		"error: corrupt patch at line 11",
		"fatal: not a git repository",
	} {
		if got := conflictContextSnippet(dir, input); got != "" {
			t.Errorf("locator-less input %q should yield empty snippet; got %q", input, got)
		}
	}
}

// TestConflictContextSnippet_FileMissingReturnsEmpty pins the
// defensive-degrade: if git named a file that isn't on disk
// (worktree state drifted between emit and read, or the tool is
// mis-wired), the helper returns "" rather than injecting garbage
// into the LLM prompt.
func TestConflictContextSnippet_FileMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got := conflictContextSnippet(dir, "error: patch failed: absent.txt:5")
	if got != "" {
		t.Errorf("missing file should yield empty snippet; got %q", got)
	}
}

// TestComposePatchRejection_WithSnippet_IncludesGroundTruth mirrors
// the session-35 Java regression: a well-formed rejection should
// carry actual file bytes so the planner LLM's retry has ground
// truth to diff against.
func TestComposePatchRejection_WithSnippet_IncludesGroundTruth(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "Main.java"),
		[]byte("package x;\npublic class Main {\n    public static String greet() {\n        return \"hi\";\n    }\n}\n"),
		0o644,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	gitErr := "error: patch failed: Main.java:3\nerror: Main.java: patch does not apply"
	msg := composePatchRejection(dir, "Main.java", gitErr)

	wants := []string{
		"emit_change_plan rejected",
		"Main.java",
		"would fail `git apply`",
		"Actual file content",
		"▶",
		"public class Main",       // within ±5 of line 3
		"return \"hi\";",          // within ±5 of line 3
		"stale context",           // coaching hint about root cause
	}
	for _, w := range wants {
		if !strings.Contains(msg, w) {
			t.Errorf("rejection missing %q; full message:\n%s", w, msg)
		}
	}
}

// TestComposePatchRejection_NoSnippet_FallsBackToGenericHint checks
// the degrade path: when git's error shape doesn't let us locate
// ground truth (e.g. corrupt patch at line N, --recount domain),
// the message still names the path + the git error verbatim + the
// generic retry hint.
func TestComposePatchRejection_NoSnippet_FallsBackToGenericHint(t *testing.T) {
	dir := t.TempDir()
	gitErr := "error: corrupt patch at line 11"
	msg := composePatchRejection(dir, "foo.go", gitErr)

	if strings.Contains(msg, "Actual file content") {
		t.Errorf("corrupt-patch error should NOT render a snippet; got:\n%s", msg)
	}
	for _, w := range []string{
		"emit_change_plan rejected",
		"foo.go",
		"corrupt patch at line 11",
		"common causes",
	} {
		if !strings.Contains(msg, w) {
			t.Errorf("rejection missing %q; full message:\n%s", w, msg)
		}
	}
}

// TestComposeApplyRejection_IncludesContextWhenAvailable pairs with
// the planner-side test — the apply_patch tool benefits from the
// same ground-truth enrichment on its own git-apply failure path.
// A plan that was valid at emit time could stale by the time apply
// fires (worktree drift, concurrent edits) and the LLM retrying
// that dispatch needs the same side-by-side signal.
func TestComposeApplyRejection_IncludesContextWhenAvailable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte("a\nb\nc\nd\ne\nf\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	gitErr := "error: patch failed: foo.go:3"
	msg := composeApplyRejection(dir, "foo.go", gitErr)
	for _, w := range []string{
		"apply_patch: git apply failed",
		"foo.go",
		"Actual file content",
		"▶",
		"verify→plan retry", // coaching on the recovery path
	} {
		if !strings.Contains(msg, w) {
			t.Errorf("rejection missing %q; full message:\n%s", w, msg)
		}
	}
}

// itoaStr is a stdlib-free int→string helper so the fixture-seeder
// loop above doesn't pull in strconv just for test data generation.
// Suffix distinguishes from the package-private `itoa` in
// analysis_limits.go (which has a Builder-first signature).
func itoaStr(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
