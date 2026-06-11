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
//     `git commit --allow-empty -m <message>`
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
//	user.email = codrax@local
//	user.name  = codrax
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

	// Make sure `.gitignore` excludes codrax's runtime artifacts
	// before the initial commit fires. Without this, the user's
	// next `git add -A` in the freshly-init repo will sweep
	// .codrax/logs/ / memory/ / blob/ / worktrees/ / plans/ into
	// version control — observed and reported by a customer.
	// Idempotent: appends only the entries that are missing.
	if err := EnsureCodraxGitignore(abs); err != nil {
		// Best-effort: a .gitignore write failure should not block
		// the init flow. Log + continue; the commit still lands.
		logging.Warning("[worktree] could not write .gitignore in %s: %v", abs, err)
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

// codraxRuntimeIgnoreEntries are the paths codrax writes to under
// the user's repo root. EnsureCodraxGitignore appends these to the
// repo's .gitignore so a user's `git add -A` cannot accidentally
// stage runtime artifacts (logs, conversation memory, blob caches,
// per-plan worktrees, plan JSON files). Each entry is the
// repo-rooted path; we keep them narrow and explicit rather than
// using a wildcard so a user can selectively un-ignore.
var codraxRuntimeIgnoreEntries = []string{
	".codrax/",
}

// codraxGitignoreHeader marks the codrax-managed block inside
// .gitignore. EnsureCodraxGitignore is idempotent: it scans for
// any of the exact entry strings already present (with or without
// the header) and only appends the missing ones plus the header.
const codraxGitignoreHeader = "# codrax runtime artifacts (logs, memory, blob caches, worktrees, plans)"

// EnsureCodraxGitignore makes sure repoRoot/.gitignore contains
// every entry in codraxRuntimeIgnoreEntries. Creates the file if
// it does not exist; appends with a labeled header block when it
// does. Returns nil on success or non-fatal "nothing to do".
//
// Design notes:
//
//   - Reads the existing file once so we only append truly missing
//     entries — matters for users who already gitignored .codrax/
//     manually and don't want a duplicate line.
//   - Trims line endings on read so a Windows .gitignore (CRLF)
//     and Unix .gitignore both compare cleanly.
//   - Uses os.WriteFile semantics (atomic-ish) only when creating
//     the file; existing files get appended via O_APPEND so we
//     don't truncate a user's hand-curated rules.
//
// Public so callers outside the write-mode flow (e.g. a future
// "codrax init" subcommand) can invoke it directly.
func EnsureCodraxGitignore(repoRoot string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("EnsureCodraxGitignore: empty repoRoot")
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("EnsureCodraxGitignore: abs: %w", err)
	}
	gitignorePath := filepath.Join(abs, ".gitignore")

	existing := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("EnsureCodraxGitignore: read: %w", err)
	}

	have := make(map[string]bool, len(codraxRuntimeIgnoreEntries))
	for _, line := range strings.Split(existing, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		have[line] = true
	}

	var missing []string
	for _, entry := range codraxRuntimeIgnoreEntries {
		if !have[entry] {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	var buf strings.Builder
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		buf.WriteString("\n")
	}
	if existing != "" {
		buf.WriteString("\n")
	}
	buf.WriteString(codraxGitignoreHeader)
	buf.WriteString("\n")
	for _, entry := range missing {
		buf.WriteString(entry)
		buf.WriteString("\n")
	}

	if existing == "" {
		// Fresh file — write atomically.
		if err := os.WriteFile(gitignorePath, []byte(buf.String()), 0o644); err != nil {
			return fmt.Errorf("EnsureCodraxGitignore: write: %w", err)
		}
		logging.Info("[worktree] created .gitignore with codrax runtime entries (%d) in %s", len(missing), abs)
		return nil
	}
	// Existing file — append without disturbing prior rules.
	f, err := os.OpenFile(gitignorePath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("EnsureCodraxGitignore: open append: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(buf.String()); err != nil {
		return fmt.Errorf("EnsureCodraxGitignore: append: %w", err)
	}
	logging.Info("[worktree] appended %d codrax runtime entries to %s/.gitignore", len(missing), abs)
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

// DirIsEffectivelyEmpty reports whether path contains no files the
// pipeline could analyse. "Effectively empty" means: directory is
// missing, unreadable, or its contents (after filtering out the
// canonical `.git` directory + the codrax-managed `.codrax/` runtime
// directory) carry no regular files at any depth.
//
// Canonical home for the probe: the orchestrator stage hooks use it
// to pick the scaffold authorization tier, and the REPL uses it to
// decide whether a bare-dir consent prompt must also cover the
// scaffold tier. Both sides MUST agree on emptiness or a consent
// granted in the REPL would not match the gate that later fires.
//
// Cross-platform: os.ReadDir + the recursive walk both use the
// OS-native path machinery (Windows / macOS / Linux).
//
// Walk depth caps: stops descending after 256 entries TOTAL across
// the whole walk so a deeply nested non-empty tree returns false
// quickly without scanning every file.
func DirIsEffectivelyEmpty(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	const maxScan = 256
	scanned := 0
	var walk func(string) bool
	walk = func(dir string) bool {
		es, err := os.ReadDir(dir)
		if err != nil {
			return true // unreadable subtree counts as "no usable files"
		}
		for _, e := range es {
			scanned++
			if scanned > maxScan {
				return false
			}
			name := e.Name()
			if e.IsDir() {
				if name == ".git" || name == ".codrax" {
					continue
				}
				if !walk(filepath.Join(dir, name)) {
					return false
				}
				continue
			}
			return false
		}
		return true
	}
	for _, e := range entries {
		scanned++
		if scanned > maxScan {
			return false
		}
		name := e.Name()
		if e.IsDir() {
			if name == ".git" || name == ".codrax" {
				continue
			}
			if !walk(filepath.Join(path, name)) {
				return false
			}
			continue
		}
		return false
	}
	return true
}
