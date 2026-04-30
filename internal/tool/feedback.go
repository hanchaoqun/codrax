package tool

import (
	"bufio"
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
// patchPayload is the LLM-emitted unified diff (the FileChange.Patch
// bytes). When non-empty, we run detectSubstitutionInContextMistake
// against it: if a `+` line is highly similar to a ` ` (context) line
// in the same hunk, the LLM almost certainly tried to replace that
// context line by adding a `+` next to it instead of marking the
// original with `-`. This is the root cause of the patch_go_typo
// 2026-04-26 eval failure (LLM left `retrun` in context, fabricated
// a `\t` removal, and added the corrected `return` line). The
// existing "stale context / indentation drift" hint is generic
// enough that the LLM can reread the prompt 7 times without seeing
// THIS specific mistake; the explicit callout below names it.
//
// When the git error does NOT carry a file:line locator (e.g. the
// "corrupt patch at line N" class — N there is inside the patch body,
// not the file), the snippet is omitted and the caller's generic
// hint remains the sole guidance. Those classes are already handled
// structurally via runGitApply's --recount flag + trailing-newline
// normalisation, so the generic hint is sufficient.
func composePatchRejection(repoRoot, path, gitErr, patchPayload string) string {
	snippet := conflictContextSnippet(repoRoot, gitErr)
	base := fmt.Sprintf(
		"emit_change_plan rejected: change %q (kind=patch) would fail `git apply`: %s",
		path, gitErr,
	)
	subsHint := detectSubstitutionInContextMistake(patchPayload)
	// Side-by-side comparison shows the actual file bytes vs the
	// rejected hunk. The model reads both and decides what changed —
	// pre-2026-04-30 we appended a "Common causes: stale context /
	// wrong @@ start / indentation drift" enumeration here, which
	// pre-classified the failure for the model. Now we just give it
	// the data and ask it to regenerate. detectSubstitutionInContextMistake
	// stays as-is — that one is a structural diff-shape check, not
	// stderr substring matching.
	if snippet != "" {
		out := base + "\n\n" + snippet +
			"\nCompare your hunk's context lines (prefixed with ' ') to the actual file bytes above and regenerate the unified diff against the real file content."
		if subsHint != "" {
			out += "\n\n" + subsHint
		}
		return out
	}
	out := base + " — regenerate the unified diff against the real file content."
	if subsHint != "" {
		out += "\n\n" + subsHint
	}
	return out
}

// detectSubstitutionInContextMistake scans an LLM-emitted unified
// diff for the failure mode "the line you wanted to replace is in
// CONTEXT (' '), not REMOVAL ('-'); a similar line was added with
// '+' instead". This is the canonical novice-LLM mistake when asked
// to fix a typo or one-line bug. The function returns a non-empty
// hint string when the mistake is detected; "" when the diff looks
// well-formed.
//
// Algorithm: walk each hunk body, collect (kind, content) pairs.
// For every `+` line, look at every ` ` line in the SAME hunk.
// Compute a similarity score (longest common prefix / total length).
// If similarity >= 0.7 AND the lines are not equal AND not trivially
// short (<5 chars after trim), flag the pair.
//
// 0.7 is conservative: a substitution typo like `retrun` → `return`
// shares 6 / 8 chars of common-prefix-or-suffix → score ≥ 0.75; a
// one-character typo `name` → `name2` scores 0.8. False positives
// would require two truly different lines that happen to share a
// long prefix; rare in practice and the hint is advisory (the LLM
// still gets the canonical "stale context" guidance above).
//
// Generic across all 6 polyglot languages — the algorithm uses no
// language-specific syntax.
func detectSubstitutionInContextMistake(patch string) string {
	if strings.TrimSpace(patch) == "" {
		return ""
	}
	type diffLine struct {
		kind byte // '+', '-', ' '
		text string
	}
	var hunkLines []diffLine
	flushHunk := func() ([]diffLine, []diffLine) {
		var ctx, adds []diffLine
		for _, dl := range hunkLines {
			switch dl.kind {
			case '+':
				adds = append(adds, dl)
			case ' ':
				ctx = append(ctx, dl)
			}
		}
		return ctx, adds
	}

	type pair struct {
		ctx string
		add string
	}
	var hits []pair

	processHunk := func() {
		ctx, adds := flushHunk()
		for _, a := range adds {
			at := strings.TrimSpace(a.text)
			if len(at) < 5 {
				continue
			}
			for _, c := range ctx {
				ct := strings.TrimSpace(c.text)
				if len(ct) < 5 || at == ct {
					continue
				}
				if substitutionSimilarity(at, ct) >= 0.7 {
					hits = append(hits, pair{ctx: c.text, add: a.text})
				}
			}
		}
		hunkLines = nil
	}

	scanner := bufio.NewScanner(strings.NewReader(patch))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inHunk := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "@@") {
			if inHunk {
				processHunk()
			}
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		if line == "" {
			hunkLines = append(hunkLines, diffLine{kind: ' ', text: ""})
			continue
		}
		switch line[0] {
		case '+', '-', ' ':
			hunkLines = append(hunkLines, diffLine{kind: line[0], text: line[1:]})
		default:
			// Diff body should not have other prefixes; ignore.
		}
	}
	if inHunk {
		processHunk()
	}
	if len(hits) == 0 {
		return ""
	}
	first := hits[0]
	return fmt.Sprintf(
		"DETECTED LIKELY MISTAKE — substitution-in-context: your hunk has\n"+
			"   addition  (`+`):  %s\n"+
			"   context   (` `):  %s\n"+
			"These two lines look like before/after of the same logical line. To replace a line you MUST mark the ORIGINAL with `-` (removal) AND provide the corrected version with `+` (addition). Do NOT leave the original in CONTEXT (' ') and only add the correction with `+` — git would then expect both lines to exist in the file, which is what just failed. Worked example for fixing `retrun` → `return`:\n"+
			"   --- a/file.go\n"+
			"   +++ b/file.go\n"+
			"   @@ -25,3 +25,3 @@\n"+
			"    \t}\n"+
			"   -\tretrun fmt.Sprintf(\"Hello, %%s!\", name)\n"+
			"   +\treturn fmt.Sprintf(\"Hello, %%s!\", name)\n"+
			"    }\n"+
			"Notice: the buggy line is on a `-` line; the corrected line is on a `+` line; surrounding context (` `) lines describe the unchanged region.",
		strings.TrimRight(first.add, "\r"),
		strings.TrimRight(first.ctx, "\r"),
	)
}

// substitutionSimilarity returns a similarity score in [0, 1] for two
// strings that are "candidates for substitution typo of each other".
// Uses max(common-prefix, common-suffix, longest-common-prefix-after-
// trim) over max(len(a), len(b)). Bounded, monotone, no false 1.0 on
// distinct strings (because a == b is filtered out by the caller).
func substitutionSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	la, lb := len(a), len(b)
	if la == 0 || lb == 0 {
		return 0
	}
	min := la
	if lb < min {
		min = lb
	}
	// Common prefix.
	prefix := 0
	for prefix < min && a[prefix] == b[prefix] {
		prefix++
	}
	// Common suffix.
	suffix := 0
	for suffix < min-prefix && a[la-1-suffix] == b[lb-1-suffix] {
		suffix++
	}
	score := float64(prefix+suffix) / float64(la)
	if float64(lb) > 0 {
		alt := float64(prefix+suffix) / float64(lb)
		if alt > score {
			score = alt
		}
	}
	if score > 1.0 {
		score = 1.0
	}
	return score
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
func composeApplyRejection(repoRoot, path, gitErr, patchPayload string) string {
	snippet := conflictContextSnippet(repoRoot, gitErr)
	base := fmt.Sprintf("apply_patch: git apply failed for %q: %s", path, gitErr)
	subsHint := detectSubstitutionInContextMistake(patchPayload)
	if snippet != "" {
		out := base + "\n\n" + snippet +
			"\nThe plan's patch did not match the worktree at apply time. If the plan was generated against an older snapshot, the verify→plan retry loop will regenerate against the current file state."
		if subsHint != "" {
			out += "\n\n" + subsHint
		}
		return out
	}
	if subsHint != "" {
		return base + "\n\n" + subsHint
	}
	return base
}
