// repo_state.go — bare-directory + commitless-repo handling for the
// write-mode entry path.
//
// `git worktree add --detach HEAD` (the call inside Create) fails for
// two related preconditions:
//
//   1. The target dir is not a git repo (no .git).
//   2. .git exists but HEAD doesn't resolve (initial repo with no
//      commits).
//
// Both are legitimate user states ("scaffold a new project from
// scratch"), but blindly running `git init` on the user's dir is a
// silent state mutation. The caller (cmd/root.go for single-shot,
// REPL handleApproveCmd for interactive) gates EnsureInitialCommit
// behind explicit authorization (yaml knob, CLI flag, or REPL
// y/N prompt) so the user always opts in.
//
// Design contract:
//
//   - DetectRepoState is read-only — it never modifies the filesystem.
//   - EnsureInitialCommit performs a NARROW set of writes (git init
//     and/or `git commit --allow-empty`) and is idempotent: calling
//     it on a RepoReady state is a no-op success.
//   - User identity is configured locally (`git config user.email/name`
//     scoped to the repo) when missing, because `git commit` refuses
//     without identity. The locally-set identity uses a clearly
//     synthetic email so the user can override at any time.

package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// RepoState describes the git readiness of a directory for worktree
// provisioning.
type RepoState int

const (
	// RepoReady — `.git` exists AND `HEAD` resolves to a commit.
	// `git worktree add --detach HEAD` will succeed.
	RepoReady RepoState = iota

	// RepoNotInitialized — no `.git` directory found at the path.
	// EnsureInitialCommit will run `git init` + `git commit
	// --allow-empty`.
	RepoNotInitialized

	// RepoNoCommits — `.git` exists but `HEAD` doesn't resolve. Most
	// commonly seen on a freshly `git init`'d repo. EnsureInitialCommit
	// will run `git commit --allow-empty` (skipping the init step).
	RepoNoCommits
)

// String returns a stable diagnostic label for logging / error text.
func (s RepoState) String() string {
	switch s {
	case RepoReady:
		return "ready"
	case RepoNotInitialized:
		return "not_initialized"
	case RepoNoCommits:
		return "no_commits"
	default:
		return "unknown"
	}
}

// NeedsInit reports whether the state requires an EnsureInitialCommit
// call before worktree.Create can succeed.
func (s RepoState) NeedsInit() bool {
	return s == RepoNotInitialized || s == RepoNoCommits
}

// DetectRepoState classifies repoRoot's git readiness. repoRoot must
// be an absolute path (callers absolutise via filepath.Abs upstream).
// Errors here are limited to filesystem access failures (path
// missing, permission denied) — git binary missing is reported as a
// dedicated error so callers can surface a setup hint.
func DetectRepoState(repoRoot string) (RepoState, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return 0, errors.New("DetectRepoState: empty repoRoot")
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return 0, fmt.Errorf("DetectRepoState: abs %s: %w", repoRoot, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return 0, fmt.Errorf("DetectRepoState: stat %s: %w", abs, err)
	}
	gitDir := filepath.Join(abs, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		if os.IsNotExist(err) {
			return RepoNotInitialized, nil
		}
		return 0, fmt.Errorf("DetectRepoState: stat .git: %w", err)
	}
	// .git exists. Probe HEAD via `git rev-parse --verify HEAD`. Exit
	// 128 with stderr "fatal: ..." means no commits yet.
	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	cmd.Dir = abs
	if err := cmd.Run(); err != nil {
		// Distinguish "git missing on PATH" from "git ran but HEAD
		// missing": the former is an *exec.Error with ENOENT, the
		// latter an *exec.ExitError.
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return 0, fmt.Errorf("DetectRepoState: git not on PATH: %w", err)
		}
		return RepoNoCommits, nil
	}
	return RepoReady, nil
}

// EnsureInitialCommit transitions repoRoot from RepoNotInitialized
// or RepoNoCommits → RepoReady by running the minimum set of git
// commands needed:
//
//   - RepoNotInitialized: `git init` → configure local identity if
//     missing → `git commit --allow-empty -m <message>`
//   - RepoNoCommits:      configure local identity if missing →
//                         `git commit --allow-empty -m <message>`
//   - RepoReady:          no-op (idempotent).
//
// message is the commit subject; callers should pass something
// recognisable so a future `git log` reveals provenance — e.g.
// "codrax: initial commit for plan-<id>".
//
// Identity configuration uses `git config --local`, scoped to this
// repo only. The synthetic values are chosen to make later
// override obvious:
//
//   user.email = codrax@local
//   user.name  = codrax
//
// Errors are wrapped with the failed step name so the caller's
// surfaced message tells the operator what to fix.
func EnsureInitialCommit(repoRoot, message string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("EnsureInitialCommit: empty repoRoot")
	}
	if strings.TrimSpace(message) == "" {
		message = "codrax: initial commit"
	}
	state, err := DetectRepoState(repoRoot)
	if err != nil {
		return fmt.Errorf("EnsureInitialCommit: detect: %w", err)
	}
	if state == RepoReady {
		return nil // idempotent
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("EnsureInitialCommit: abs: %w", err)
	}

	if state == RepoNotInitialized {
		if out, err := runGitCapture(abs, "init"); err != nil {
			return fmt.Errorf("EnsureInitialCommit: git init: %w (%s)", err, out)
		}
		logging.Info("[worktree] git init in %s", abs)
	}

	// Ensure identity exists locally. `git config user.email` exits
	// non-zero (1) when the key is missing, which is the signal we
	// need to set defaults. Don't overwrite an existing identity.
	if !hasGitConfig(abs, "user.email") {
		if out, err := runGitCapture(abs, "config", "--local", "user.email", "codrax@local"); err != nil {
			return fmt.Errorf("EnsureInitialCommit: set user.email: %w (%s)", err, out)
		}
	}
	if !hasGitConfig(abs, "user.name") {
		if out, err := runGitCapture(abs, "config", "--local", "user.name", "codrax"); err != nil {
			return fmt.Errorf("EnsureInitialCommit: set user.name: %w (%s)", err, out)
		}
	}

	if out, err := runGitCapture(abs, "commit", "--allow-empty", "-m", message); err != nil {
		return fmt.Errorf("EnsureInitialCommit: git commit: %w (%s)", err, out)
	}
	logging.Info("[worktree] created initial commit in %s (%q)", abs, message)
	return nil
}

// runGitCapture invokes git with cwd=dir and returns trimmed combined
// output for error-message embedding. The tiny helper exists so
// EnsureInitialCommit's body reads as a sequence of git commands
// rather than 5 lines of exec ceremony each.
func runGitCapture(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// hasGitConfig reports whether `git config <key>` exits 0. A zero
// exit here means the key is set (value irrelevant for our purposes:
// we only need to know whether to install our defaults).
func hasGitConfig(dir, key string) bool {
	cmd := exec.Command("git", "config", key)
	cmd.Dir = dir
	return cmd.Run() == nil
}
