package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// composePatchRejection builds the rejection-summary string the LLM
// sees after a `git apply` failure. The class-level pattern: every
// content-validation rejection in write mode must carry enough signal
// for the LLM to self-correct on retry, not just a generic "failed".
//
// For patch failures, the single most useful signal is a side-by-side
// view of "what your hunk claimed" vs "what the file actually has".
// Git's stderr carries the CLAIM position ("patch failed: <file>:<N>");
// we pair it with the file's ACTUAL bytes at that position so the LLM
// can diff them and regenerate. The session-35 Java eval showed this
// matters: the planner hallucinated a blank line at line 16 of the
// file, git said "patch failed: Main.java:13", and the LLM had no
// way to see "oh, line 13 of the actual file is <X>, not what I
// wrote". Three retries all made the same mistake because the
// feedback never contained ground truth.
//
// When the git error does NOT carry a file:line locator (e.g. the
// "corrupt patch at line N" class — N there is inside the patch body,
// not the file), the snippet is omitted and the caller's generic
// hint remains the sole guidance. Those classes are already handled
// structurally via runGitApply's --recount flag + trailing-newline
// normalisation, so the generic hint is sufficient.
func composePatchRejection(repoRoot, path, gitErr string) string {
	snippet := conflictContextSnippet(repoRoot, gitErr)
	base := fmt.Sprintf(
		"emit_change_plan rejected: change %q (kind=patch) would fail `git apply`: %s",
		path, gitErr,
	)
	if snippet != "" {
		return base + "\n\n" + snippet +
			"\nCompare your hunk's context lines (prefixed with ' ') to the actual file bytes above. " +
			"Common causes: stale context (a line in your hunk doesn't exist at the claimed position), " +
			"wrong @@ start line (off by a few lines from the truth), or indentation drift (spaces vs tabs). " +
			"Regenerate the unified diff using the actual file content, not a remembered version."
	}
	return base +
		" — regenerate the unified diff (common causes: wrong @@ hunk line counts, missing trailing newline, stale context lines that no longer match the file)."
}

// conflictContextSnippet parses `patch failed: <file>:<line>` from
// git's stderr and returns a formatted ±5 line window of the actual
// file content at that position. Returns "" when the error doesn't
// match the expected shape or the file is unreadable.
//
// The format embeds gutter line numbers so the LLM can align its
// hunk header's declared start line against what the file truly has
// at that position:
//
//	Actual file content around the conflict (Main.java lines 11-21):
//	  11: public class Main {
//	  12:     public static String greet(String name) {
//	▶ 13:         if (name == null || name.isEmpty()) {
//	  14:             name = "world";
//	  15:         }
//	  16:         retrun "Hello, " + name + "!";
//	  ...
//
// The ▶ marks the line git named as the conflict anchor so the LLM
// instantly sees the claim vs the ground truth.
func conflictContextSnippet(repoRoot, gitErr string) string {
	file, lineNum := parseGitConflictLocator(gitErr)
	if file == "" || lineNum <= 0 {
		return ""
	}
	abs := filepath.Join(repoRoot, file)
	data, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	// Some editors leave a trailing empty entry after a final \n;
	// strip it so the rendered snippet doesn't tail with a blank.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	if lineNum > len(lines) {
		// Git's line number can exceed EOF when the hunk claims a
		// position past the file end. Clamp to last line so we
		// still render useful surrounding context.
		lineNum = len(lines)
	}
	const window = 5
	start := lineNum - window
	if start < 1 {
		start = 1
	}
	end := lineNum + window
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Actual file content around the conflict (%s lines %d-%d):\n", file, start, end)
	for i := start; i <= end; i++ {
		marker := "  "
		if i == lineNum {
			marker = "▶ "
		}
		fmt.Fprintf(&b, "%s%3d: %s\n", marker, i, lines[i-1])
	}
	return b.String()
}

// gitPatchFailedRe captures the target-file locator from git's
// conflict error. Git emits this shape for context-mismatch failures:
//
//	error: patch failed: path/to/file.ext:NN
//	error: path/to/file.ext: patch does not apply
//
// Captures (1) file path, (2) line number. The path is repo-relative
// and matches the first "+++" line of the hunk. Line numbers are the
// hunk's declared start (1-based).
var gitPatchFailedRe = regexp.MustCompile(`patch failed: ([^:\n]+):(\d+)`)

func parseGitConflictLocator(gitErr string) (string, int) {
	m := gitPatchFailedRe.FindStringSubmatch(gitErr)
	if len(m) < 3 {
		return "", 0
	}
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return "", 0
	}
	return strings.TrimSpace(m[1]), n
}

// composeApplyRejection mirrors composePatchRejection but for the
// apply_patch tool's own git-apply failure path. The SHAPE of the
// rejection is different (different tool name in the prefix) but the
// class-level contract is identical: when git rejects a patch at
// apply time, surface the file context so the LLM can re-plan with
// ground-truth data rather than a remembered hallucination.
//
// Kept as a sibling function rather than a shared composer so the
// tool names stay accurate in the message — the LLM uses the prefix
// to route retries ("emit_change_plan rejected" vs "apply_patch:
// git apply failed"). Both call conflictContextSnippet so the
// snippet-rendering code has one owner.
func composeApplyRejection(repoRoot, path, gitErr string) string {
	snippet := conflictContextSnippet(repoRoot, gitErr)
	base := fmt.Sprintf("apply_patch: git apply failed for %q: %s", path, gitErr)
	if snippet != "" {
		return base + "\n\n" + snippet +
			"\nThe plan's patch did not match the worktree at apply time. If the plan was generated against an older snapshot, the verify→plan retry loop will regenerate against the current file state."
	}
	return base
}
