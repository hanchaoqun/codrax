package repl

import (
	"os/exec"
	"strings"
)

// defaultMergeTarget resolves what `/merge` (no flags) and
// `/approve` (no --merge-to) should treat as the default base /
// target branch. Live git HEAD wins so the natural "fast-forward
// the branch I'm on" expectation holds even when the user has
// switched branches mid-session. Falls back to `configured` (the
// --branch flag value) when HEAD is detached or git is missing —
// detached@<sha> is not a meaningful merge target, and the
// configured value is the closest stand-in we have.
//
// Keeping this logic in one helper means the REPL's startup
// banner, prompt sticky tag, /merge default, and runMerge base-
// branch resolution all agree on the answer.
func defaultMergeTarget(repoRoot, configured string) string {
	if br := gitBranchProbe(repoRoot); br != "" && !strings.HasPrefix(br, "detached@") {
		return br
	}
	return configured
}

// gitBranchProbe returns the human-readable name of the current
// git branch in repoRoot for prompt display. Per-prompt cost is
// dominated by an exec.Command roundtrip to local git (~1ms on
// Linux/macOS; cached by the kernel page cache for repeated calls
// on the same repo) — no need to memoise inside the REPL since
// the branch can change at any moment when the user runs
// `git checkout` in another terminal and we want the next prompt
// to reflect that immediately.
//
// Resolution order:
//
//  1. `git -C <repoRoot> symbolic-ref --short HEAD` returns the
//     branch name when HEAD is on a branch (the common case).
//  2. On a detached HEAD (worktree, mid-rebase, mid-cherry-pick,
//     `git checkout <sha>` directly), step 1 fails. Fall back to
//     `git rev-parse --short HEAD` and prefix the result with
//     "detached@" so the marker stays scannable.
//  3. If both fail (path is not a git repo, git missing on PATH,
//     permission denied), return empty string. The caller drops
//     the marker entirely — no `[git:?]` placeholder; the absence
//     of the marker IS the signal.
//
// Empty repoRoot returns empty string immediately. Whitespace
// in the git output is trimmed.
func gitBranchProbe(repoRoot string) string {
	if strings.TrimSpace(repoRoot) == "" {
		return ""
	}
	cmd := exec.Command("git", "-C", repoRoot, "symbolic-ref", "--short", "HEAD")
	out, err := cmd.Output()
	if err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name
		}
	}
	// Detached HEAD fallback. rev-parse --short returns a 7-char
	// SHA prefix that's stable across git versions.
	cmd = exec.Command("git", "-C", repoRoot, "rev-parse", "--short", "HEAD")
	out, err = cmd.Output()
	if err == nil {
		if sha := strings.TrimSpace(string(out)); sha != "" {
			return "detached@" + sha
		}
	}
	return ""
}
