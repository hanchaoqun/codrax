// Package worktree manages git worktree lifecycles for the B0
// write-mode pipeline stages (plan / apply / verify). A Session is
// one detached-HEAD worktree under <runtime-anchor>/worktrees/
// <trace-id>-<pid>/; Discard tears it down; PruneDeadSessions
// sweeps orphans from prior processes; InstallSignalHandler closes
// the SIGINT/SIGTERM escape hatch that Go's default signal disposition
// leaves open.
//
// Design notes (session 33 Day 2):
//
//  1. Worktree-as-dry-run: the apply stage never mutates the main
//     repo checkout. `git worktree add --detach HEAD` produces a
//     disposable copy; when the stage finishes (success OR failure)
//     the copy is discarded. The user can cherry-pick a successful
//     commit from the worktree into main manually — that flow lives
//     above this package.
//
//  2. Cleanup in three environments:
//     - Normal return: the orchestrator's outer defer fires
//     DiscardByPath. Idempotent with the active-sessions map.
//     - Panic: Go runs defers during unwind; same path as normal.
//     - SIGINT / SIGTERM: Go's default disposition is os.Exit(130)
//     WITHOUT running defers. InstallSignalHandler adds a
//     signal.Notify handler that walks the active-sessions map,
//     discards everyone, then re-raises so the process still
//     dies with the canonical signal exit code.
//     - SIGKILL: no cleanup possible in-process; PruneDeadSessions
//     at next startup reaps orphans by PID liveness check.
//
//  3. Idempotent Discard: the three-step sequence
//     (`git worktree remove --force` → `os.RemoveAll` → `git worktree
//     prune`) tolerates every state the disk might be in after a
//     crash: dir-only, metadata-only, both, or already clean. Each
//     step suppresses errors that reflect a prior step already
//     cleaning that surface.
//
//  4. Collision-free naming: directory format is
//     `<sanitized-trace-id>-<pid>`. Trace IDs carry `unix-nano` so
//     same-process concurrent Runs produce distinct IDs; pid
//     disambiguates across processes. Cross-process-pid-wraparound
//     is a theoretical edge case (a long-running system's PIDs
//     eventually cycle) that can leave one leaked worktree dir;
//     no correctness harm, only disk waste.
package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// Session is one git worktree instance. Zero-value Session is not
// usable; construct via Create. Once Discard has been called the
// Session's Path() stays non-empty for observability but further
// Discard calls are idempotent no-ops.
type Session struct {
	path     string // absolute path of the worktree directory
	mainRoot string // absolute path of the main repo (for `git worktree prune` / context)
}

// Create creates a detached-HEAD worktree under baseDir. baseDir is
// the per-runtime-anchor `worktrees/` directory; it is created if
// missing. mainRepoRoot is the repo whose HEAD we check out into
// the worktree. traceID is the orchestrator's per-Run identifier —
// embedding it in the directory name avoids collisions between
// concurrent Runs in the same process.
//
// On success the returned Session is registered in the package-
// level active-sessions map so InstallSignalHandler's cleanup loop
// can find it. On error every side effect (partial directory, git
// metadata) is cleaned up before returning — caller sees a
// transactional failure: either a valid Session or a pristine
// filesystem.
func Create(baseDir, mainRepoRoot, traceID string) (*Session, error) {
	if baseDir == "" {
		return nil, errors.New("worktree.Create: baseDir is empty")
	}
	if mainRepoRoot == "" {
		return nil, errors.New("worktree.Create: mainRepoRoot is empty")
	}
	if strings.TrimSpace(traceID) == "" {
		return nil, errors.New("worktree.Create: traceID is empty")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("worktree.Create: mkdir %s: %w", baseDir, err)
	}
	mainAbs, err := filepath.Abs(mainRepoRoot)
	if err != nil {
		return nil, fmt.Errorf("worktree.Create: abs mainRepoRoot: %w", err)
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("worktree.Create: abs baseDir: %w", err)
	}
	name := fmt.Sprintf("%s-%d", sanitizeTraceID(traceID), os.Getpid())
	wtPath := filepath.Join(baseAbs, name)

	// Precondition: target path must not already exist. If a prior
	// Run (same pid, same traceID — should be impossible in practice
	// because traceID carries unix-nano) left residue, bail rather
	// than silently overwriting — the operator's hand-debugging
	// state is more valuable than transparent recovery.
	if _, err := os.Stat(wtPath); err == nil {
		return nil, fmt.Errorf("worktree.Create: target %s already exists", wtPath)
	}

	cmd := exec.Command("git", "worktree", "add", "--detach", wtPath, "HEAD")
	cmd.Dir = mainAbs
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Best-effort cleanup of any partial state git may have
		// left. Errors from these are swallowed — we're already
		// returning the outer error.
		_ = os.RemoveAll(wtPath)
		runGitPrune(mainAbs)
		return nil, fmt.Errorf("worktree.Create: git worktree add: %w (output: %s)",
			err, strings.TrimSpace(string(out)))
	}

	s := &Session{path: wtPath, mainRoot: mainAbs}
	activeSessions.Store(wtPath, mainAbs)
	logging.Info("[worktree] created %s (main=%s)", wtPath, mainAbs)
	return s, nil
}

// Path returns the worktree's absolute filesystem path. Safe on nil
// receiver (returns "").
func (s *Session) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// MainRoot returns the main repo absolute path this worktree was
// cloned from. Safe on nil receiver.
func (s *Session) MainRoot() string {
	if s == nil {
		return ""
	}
	return s.mainRoot
}

// Discard tears down the worktree. Safe on nil receiver; safe to
// call multiple times (idempotent). The error return reflects only
// filesystem-level failures that survived all three cleanup steps;
// in practice errors here are rare and indicate an operator-level
// problem (immutable mount, permission loss mid-Run) that reaping
// cannot fix.
//
// Cleanup sequence:
//
//  1. `git worktree remove --force <path>` — tells git to drop the
//     worktree AND its metadata. --force tolerates a dirty tree
//     (which B0's apply-phase failures are likely to produce).
//     Errors are swallowed: a missing dir makes this a no-op but
//     still returns non-zero on some git versions.
//
//  2. `os.RemoveAll(path)` — belt-and-suspenders. If git refused
//     (wrong repo, already removed, etc) this makes sure the bytes
//     are gone. IsNotExist is suppressed because the common
//     success case from step 1 removed the dir already.
//
//  3. `git worktree prune` — sweeps any orphan metadata (the
//     `.git/worktrees/<id>` directory inside the main repo) that
//     step 1's error path might have left. Mirrors the post-SIGKILL
//     reap PruneDeadSessions does.
//
// Each step is independent — later steps run even if an earlier
// step errored, so the overall call is maximally self-healing.
func (s *Session) Discard() error {
	if s == nil || s.path == "" {
		return nil
	}
	path := s.path
	mainAbs := s.mainRoot

	// De-register early so a concurrent SIGINT handler doesn't
	// walk into a half-dismantled Discard.
	activeSessions.Delete(path)

	// Step 1: git worktree remove (errors ignored — step 2 covers).
	if mainAbs != "" {
		cmd := exec.Command("git", "worktree", "remove", "--force", path)
		cmd.Dir = mainAbs
		_ = cmd.Run()
	}

	// Step 2: ensure bytes are gone.
	var final error
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		final = fmt.Errorf("worktree.Discard: RemoveAll %s: %w", path, err)
		logging.Warning("[worktree] RemoveAll %s: %v", path, err)
	}

	// Step 3: orphan metadata cleanup.
	runGitPrune(mainAbs)

	return final
}

// DiscardByPath is the free-function variant of Discard. The
// orchestrator's outer defer only has BusContext.WorktreePath /
// BusContext.MainRepoRoot on hand — not a *Session pointer — so it
// reconstructs a throwaway Session and calls Discard through this
// adapter.
//
// path and mainRoot may be empty; both cases short-circuit to nil
// so the defer is safe to fire unconditionally in read-mode Runs
// where nobody populated these fields.
func DiscardByPath(path, mainRoot string) error {
	if path == "" {
		return nil
	}
	s := &Session{path: path, mainRoot: mainRoot}
	return s.Discard()
}

// PruneDeadSessions scans baseDir and reaps any session directory
// whose embedded PID is no longer a live process. Called at codrax
// startup to sweep residue from SIGKILL-terminated prior runs.
//
// mainRepoRoot is optional (empty string skips the git metadata
// prune step and the `git worktree remove --force` invocation per
// orphan; only the filesystem rm sweep runs). In practice cmd/root.go
// always passes a real repo root so step 1 and step 3 from Discard
// both fire.
//
// Best-effort: every error (missing base, unreadable dir, failed
// rm) is logged at debug level and swept under the rug. Reaping
// is hygiene, not correctness — correctness lives in the
// active-sessions map + signal handler.
func PruneDeadSessions(baseDir, mainRepoRoot string) {
	if baseDir == "" {
		return
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		// Most common: baseDir doesn't exist yet (fresh user). No-op.
		return
	}
	mainAbs := ""
	if mainRepoRoot != "" {
		if abs, err := filepath.Abs(mainRepoRoot); err == nil {
			mainAbs = abs
		}
	}
	selfPid := os.Getpid()
	reaped := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid := pidFromSessionName(e.Name())
		if pid == 0 {
			continue // not a session-shaped name; respect operator data
		}
		if pid == selfPid {
			// Should not exist (we haven't created one yet) but skip
			// to be safe — a paranoid operator might have laid a
			// future-state directory by hand.
			continue
		}
		if logging.IsPidAlive(pid) {
			continue // live peer, leave alone
		}
		orphan := filepath.Join(baseDir, e.Name())
		if mainAbs != "" {
			cmd := exec.Command("git", "worktree", "remove", "--force", orphan)
			cmd.Dir = mainAbs
			_ = cmd.Run()
		}
		if err := os.RemoveAll(orphan); err != nil && !os.IsNotExist(err) {
			logging.Warning("[worktree] reap %s: %v", orphan, err)
			continue
		}
		reaped++
	}
	if mainAbs != "" {
		runGitPrune(mainAbs)
	}
	if reaped > 0 {
		logging.Info("[worktree] reaped %d orphan session(s) in %s", reaped, baseDir)
	}
}

// PruneByAgeAndQuota walks baseDir and removes preserved (keep-on-
// success) worktree directories that exceed the supplied retention
// rules. Two independent gates, both opt-in:
//
//  1. Age TTL: dirs older than ttl (modified time) are removed.
//     ttl == 0 disables the age check entirely.
//  2. Quota: at most maxCount dirs survive; LRU-ordered (oldest
//     mtime evicts first). maxCount == 0 disables the quota.
//
// Within-process safety: never removes a dir registered in
// activeSessions (a Run that's currently using it). Cross-process:
// callers should run PruneDeadSessions first to clear orphans
// from dead peer processes; this function targets *kept* dirs
// from THIS process or from past sessions whose PID is no longer
// reachable but whose dir was deliberately preserved.
//
// Returns the count of dirs removed for logging visibility.
func PruneByAgeAndQuota(baseDir, mainRepoRoot string, ttl time.Duration, maxCount int) int {
	if baseDir == "" || (ttl == 0 && maxCount == 0) {
		return 0
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return 0 // missing baseDir = nothing to prune
	}
	mainAbs := ""
	if mainRepoRoot != "" {
		if abs, err := filepath.Abs(mainRepoRoot); err == nil {
			mainAbs = abs
		}
	}
	type candidate struct {
		path  string
		mtime time.Time
	}
	now := time.Now()
	var cands []candidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only operate on session-shaped names so we never touch
		// operator-laid directories that happen to live under
		// baseDir.
		if pidFromSessionName(e.Name()) == 0 {
			continue
		}
		full := filepath.Join(baseDir, e.Name())
		// Active-session lock: a dir registered in activeSessions
		// is in-use by a Run NOW. Never reap.
		if _, inUse := activeSessions.Load(full); inUse {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		cands = append(cands, candidate{path: full, mtime: info.ModTime()})
	}

	// Stage 1: age TTL. Remove dirs whose mtime is older than ttl.
	removed := 0
	if ttl > 0 {
		survivors := cands[:0]
		for _, c := range cands {
			if now.Sub(c.mtime) > ttl {
				if err := removeKeptWorktree(c.path, mainAbs); err == nil {
					removed++
				}
				continue
			}
			survivors = append(survivors, c)
		}
		cands = survivors
	}

	// Stage 2: quota. After TTL eviction, if dirs still exceed
	// maxCount, drop the oldest until count <= maxCount. Sort by
	// mtime asc (oldest first) so the bottom of the slice is the
	// LRU.
	if maxCount > 0 && len(cands) > maxCount {
		sort.Slice(cands, func(i, j int) bool { return cands[i].mtime.Before(cands[j].mtime) })
		excess := len(cands) - maxCount
		for i := 0; i < excess; i++ {
			if err := removeKeptWorktree(cands[i].path, mainAbs); err == nil {
				removed++
			}
		}
	}

	if mainAbs != "" && removed > 0 {
		runGitPrune(mainAbs)
	}
	if removed > 0 {
		logging.Info("[worktree] reaped %d kept worktree(s) by ttl=%v/quota=%d in %s",
			removed, ttl, maxCount, baseDir)
	}
	return removed
}

// removeKeptWorktree wraps the standard "git worktree remove +
// rm -rf" cleanup so PruneByAgeAndQuota's two stages share one
// implementation. Logs at warning when removal fails (operator
// can manually clean) but does not abort.
func removeKeptWorktree(path, mainAbs string) error {
	if mainAbs != "" {
		cmd := exec.Command("git", "worktree", "remove", "--force", path)
		cmd.Dir = mainAbs
		_ = cmd.Run()
	}
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		logging.Warning("[worktree] reap %s: %v", path, err)
		return err
	}
	return nil
}

// activeSessions tracks every outstanding Session for the signal
// handler's cleanup loop. Key: worktree path. Value: mainRoot.
//
// sync.Map over a plain mutex because the typical access pattern is
// heavy-read (signal handler walk) with rare writes (create/discard),
// and Go's sync.Map is optimised for exactly that shape.
var activeSessions sync.Map

// signalHandlerInstalled latches InstallSignalHandler so repeated
// calls from cmd/root.go / tests do not register N handlers. The
// Go runtime fans a single signal to every channel registered via
// signal.Notify, so calling InstallSignalHandler twice would double-
// fire cleanup (harmless but wasteful).
var signalHandlerInstalled bool
var signalHandlerMu sync.Mutex
var signalCleanupHooksMu sync.Mutex
var signalCleanupHooks []func()

// InstallSignalHandler registers a SIGINT / SIGTERM handler that
// walks activeSessions, discards every session, then re-raises the
// signal so the process dies with the canonical exit code. Calling
// more than once is a no-op after the first install.
//
// Race with normal Discard: activeSessions.Delete runs BEFORE the
// filesystem work in Discard, so if a Discard is in flight when the
// signal fires the handler finds an already-emptied map. Two calls
// to rm -rf on the same path are safe (second is no-op / IsNotExist).
//
// Must be called exactly once at process startup from cmd/root.go.
// Tests that exercise the handler directly do so by calling
// cleanActiveSessions() — the extracted inner function — which
// bypasses signal registration entirely.
func InstallSignalHandler() {
	signalHandlerMu.Lock()
	defer signalHandlerMu.Unlock()
	if signalHandlerInstalled {
		return
	}
	signalHandlerInstalled = true

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range ch {
			// Suppression check: when the REPL is alive it owns the
			// signal disposition (one Ctrl+C = cancel current Run,
			// double-tap = exit). Without this gate both this
			// handler and the REPL's would race on the first
			// signal — this handler would always win because it
			// terminates the process immediately, and the REPL's
			// cancel + double-tap UX would never engage. The REPL
			// is responsible for calling CleanActiveSessions itself
			// on its own exit paths so worktree state is still
			// recovered correctly.
			if signalHandlerSuppressed.Load() {
				continue
			}
			logging.Warning("[worktree] received %s, discarding %d active session(s)",
				sig, activeSessionCount())
			runSignalCleanupHooks()
			cleanActiveSessions()
			signal.Stop(ch)
			// Terminate after cleanup. Unix can re-raise the original
			// signal so the shell sees the canonical status; Windows
			// falls back to an equivalent exit code.
			terminateProcessAfterCleanup(sig)
			return
		}
	}()
}

// signalHandlerSuppressed gates the package-level SIGINT/SIGTERM
// handler. The REPL flips this to true while it is alive so its own
// signal handler can implement the cancel-current-Run vs. force-exit
// UX without the worktree handler racing it to os.Exit.
var signalHandlerSuppressed atomic.Bool

// RegisterSignalCleanupHook installs a process-shutdown cleanup callback for
// resources that live beside worktrees (for example external helper processes).
// Hooks run from the same SIGINT/SIGTERM handler before worktree cleanup and
// should be best-effort/non-blocking.
func RegisterSignalCleanupHook(fn func()) {
	if fn == nil {
		return
	}
	signalCleanupHooksMu.Lock()
	defer signalCleanupHooksMu.Unlock()
	signalCleanupHooks = append(signalCleanupHooks, fn)
}

func runSignalCleanupHooks() {
	signalCleanupHooksMu.Lock()
	hooks := append([]func(){}, signalCleanupHooks...)
	signalCleanupHooksMu.Unlock()
	for _, fn := range hooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logging.Warning("[worktree] signal cleanup hook panic: %v", r)
				}
			}()
			fn()
		}()
	}
}

// SetSignalHandlerSuppressed toggles whether the package-level
// SIGINT/SIGTERM handler should run cleanup-and-exit when a signal
// arrives. The REPL sets this to true at Loop start so a single
// Ctrl+C is interpreted as "cancel the in-flight Run" rather than
// "exit codrax". Single-shot CLI callers leave it false (default)
// so historical "Ctrl+C exits cleanly" behaviour is preserved.
//
// CleanActiveSessions remains exported so the REPL can drive
// worktree cleanup itself on its own exit paths (idle Ctrl+C,
// double-tap escalate, /exit).
func SetSignalHandlerSuppressed(b bool) {
	signalHandlerSuppressed.Store(b)
}

// CleanActiveSessions is the exported variant of cleanActiveSessions.
// REPL calls it on exit paths the suppressed package handler can no
// longer cover. Internal callers (the package handler's pre-exit
// cleanup) keep using the lowercase version.
func CleanActiveSessions() { cleanActiveSessions() }

// cleanActiveSessions walks the active-sessions map and invokes
// Discard on each. Exported to the package for the signal handler
// goroutine and for the signal-path test to call directly (so tests
// do not need to actually raise SIGINT).
func cleanActiveSessions() {
	var sessions []*Session
	activeSessions.Range(func(k, v any) bool {
		path, ok := k.(string)
		if !ok || path == "" {
			return true
		}
		mainAbs, _ := v.(string)
		sessions = append(sessions, &Session{path: path, mainRoot: mainAbs})
		return true
	})
	for _, s := range sessions {
		_ = s.Discard()
	}
}

// activeSessionCount returns the number of outstanding sessions
// (for logging only; not load-bearing).
func activeSessionCount() int {
	n := 0
	activeSessions.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

// signalExitCode maps the cleanup signal to the shell-visible exit
// code we preserve when the platform cannot re-raise the signal on
// itself (for example Windows).
func signalExitCode(sig os.Signal) int {
	ssig, ok := sig.(syscall.Signal)
	if !ok {
		return 1
	}
	switch ssig {
	case syscall.SIGINT:
		return 130
	case syscall.SIGTERM:
		return 143
	default:
		return 1
	}
}

// CommitChanges stages every file (modified, untracked, deleted) in
// the worktree at path and creates a single commit with the given
// message. Returns the resulting HEAD SHA on success.
//
// Used by the orchestrator's warm-worktree retry loop: each apply
// iteration's content is committed so a later retry can `ResetHard`
// back to the best iteration's commit instead of having to discard
// the whole worktree and rebuild from the original stub.
//
// Empty messages get a "codrax retry checkpoint" default. Identity is
// pinned to a non-attributable codrax-internal author so the commit
// is obviously not a user authorship signal in case the worktree
// leaks into a code review surface.
//
// `--allow-empty` is set so a no-op apply (nothing changed) still
// produces a SHA; without it the retry loop would need to special-
// case "no changes to commit" and the bookkeeping fragments.
func CommitChanges(path, message string) (string, error) {
	if path == "" {
		return "", errors.New("worktree.CommitChanges: path is empty")
	}
	if strings.TrimSpace(message) == "" {
		message = "codrax retry checkpoint"
	}
	if out, err := runGitIn(path, "add", "-A"); err != nil {
		return "", fmt.Errorf("worktree.CommitChanges: git add: %w (output: %s)", err, out)
	}
	commitArgs := []string{
		"-c", "user.email=codrax-retry@local",
		"-c", "user.name=codrax-retry",
		"commit", "--allow-empty", "-m", message,
	}
	if out, err := runGitIn(path, commitArgs...); err != nil {
		return "", fmt.Errorf("worktree.CommitChanges: git commit: %w (output: %s)", err, out)
	}
	out, err := runGitIn(path, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("worktree.CommitChanges: rev-parse HEAD: %w (output: %s)", err, out)
	}
	return strings.TrimSpace(out), nil
}

// ResetHard runs `git reset --hard <sha>` in the worktree at path so
// the working tree contents match the named commit exactly. Used to
// rewind a regressed retry iteration back to the best-known-good
// iteration's content before the next planner dispatch.
//
// `--hard` is the right flag here: we INTENTIONALLY want both the
// index and the working tree restored to <sha>, since the apply
// stage is about to overwrite them with new content anyway. Soft or
// mixed reset would leave the regressed-iteration's working-tree
// state behind, defeating the warmup.
func ResetHard(path, sha string) error {
	if path == "" {
		return errors.New("worktree.ResetHard: path is empty")
	}
	if strings.TrimSpace(sha) == "" {
		return errors.New("worktree.ResetHard: sha is empty")
	}
	if out, err := runGitIn(path, "reset", "--hard", sha); err != nil {
		return fmt.Errorf("worktree.ResetHard: %w (output: %s)", err, out)
	}
	return nil
}

// TagAppliedCommit pins the apply commit in the main repository's
// ref namespace under refs/codrax/applied/<plan-id>. Worktree commits
// share the main repo's object store; without a ref pointing to them,
// `git gc` will eventually prune the apply commit. The pinned ref:
//
//   - Survives worktree.Discard (the bytes live in main_repo/.git/
//     objects, the ref is in main_repo/.git/refs).
//   - Lets /merge fall back to `git cherry-pick refs/codrax/applied/<id>`
//     even when keep_on_success was off and the worktree dir is gone.
//   - Lets the operator manually recover via the same ref name from
//     the main repo without ever needing the SHA in clipboard form.
//
// Best-effort: failures log at warning. The "pinning is hygiene, not
// correctness" model mirrors PruneDeadSessions — operators get the
// same recovery path even on filesystem oddities.
func TagAppliedCommit(mainRepoRoot, planID, sha string) (string, error) {
	if strings.TrimSpace(mainRepoRoot) == "" {
		return "", errors.New("worktree.TagAppliedCommit: empty mainRepoRoot")
	}
	if strings.TrimSpace(planID) == "" {
		return "", errors.New("worktree.TagAppliedCommit: empty planID")
	}
	if strings.TrimSpace(sha) == "" {
		return "", errors.New("worktree.TagAppliedCommit: empty sha")
	}
	ref := AppliedRef(planID)
	if out, err := runGitIn(mainRepoRoot, "update-ref", ref, sha); err != nil {
		return "", fmt.Errorf("worktree.TagAppliedCommit: update-ref %s %s: %w (output: %s)",
			ref, sha, err, out)
	}
	return ref, nil
}

// AppliedRef returns the canonical recovery ref name for a plan ID.
// Kept stable so /merge, error messages, and tests all agree.
func AppliedRef(planID string) string {
	return "refs/codrax/applied/" + planID
}

// DeleteAppliedRef tears down a recovery ref previously created by
// TagAppliedCommit. Used by /worktree discard / /reject paths so
// the ref namespace doesn't grow unbounded across abandoned plans.
// Best-effort: a missing ref is a successful no-op.
func DeleteAppliedRef(mainRepoRoot, planID string) error {
	if strings.TrimSpace(mainRepoRoot) == "" || strings.TrimSpace(planID) == "" {
		return nil
	}
	ref := AppliedRef(planID)
	// `git update-ref -d <ref>` returns 0 when the ref existed and was
	// removed; non-zero when it didn't exist. We tolerate both.
	_, _ = runGitIn(mainRepoRoot, "update-ref", "-d", ref)
	return nil
}

// runGitIn runs `git <args>` with cwd=path and returns combined
// stdout/stderr. Internal helper for CommitChanges / ResetHard.
func runGitIn(path string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// runGitPrune runs `git worktree prune` in mainRoot. Errors are
// silently swallowed — prune is hygiene.
func runGitPrune(mainRoot string) {
	if mainRoot == "" {
		return
	}
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = mainRoot
	_ = cmd.Run()
}

// pidFromSessionName parses "<trace-id>-<pid>" and returns the pid.
// Returns 0 on any parse failure (caller treats as "not a session
// dir" and skips). Uses LastIndex so trace IDs containing internal
// hyphens (they do — format "trace-<unix-nano>") still parse.
func pidFromSessionName(name string) int {
	idx := strings.LastIndex(name, "-")
	if idx <= 0 {
		return 0
	}
	pid, err := strconv.Atoi(name[idx+1:])
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

// sanitizeTraceID strips filesystem-unsafe characters from the
// trace identifier so it can form a directory name. Allowed:
// [A-Za-z0-9_-]. Everything else becomes '_'. Empty result
// degrades to "trace".
func sanitizeTraceID(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "trace"
	}
	return b.String()
}

// CaptureCommitPatch returns the unified patch (commit header + diff) of one
// commit inside the worktree. Used to persist durable attempt evidence for
// failed verifications BEFORE the unconditional cleanup defer discards the
// tree — cleanup behaviour itself is never altered. Read-only.
func CaptureCommitPatch(path, sha string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("worktree.CaptureCommitPatch: path is empty")
	}
	if strings.TrimSpace(sha) == "" {
		return "", errors.New("worktree.CaptureCommitPatch: sha is empty")
	}
	out, err := runGitIn(path, "show", "--format=medium", "--no-color", sha)
	if err != nil {
		return "", fmt.Errorf("worktree.CaptureCommitPatch: %w (output: %s)", err, out)
	}
	return out, nil
}

// CaptureRangePatch returns the unified patch between two refs inside the
// worktree. It is read-only and intended for final/cumulative review surfaces
// that must reason about the actual bytes being delivered rather than one
// individual apply commit.
func CaptureRangePatch(path, baseRef, headRef string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("worktree.CaptureRangePatch: path is empty")
	}
	baseRef = strings.TrimSpace(baseRef)
	if baseRef == "" {
		return "", errors.New("worktree.CaptureRangePatch: baseRef is empty")
	}
	headRef = strings.TrimSpace(headRef)
	if headRef == "" {
		headRef = "HEAD"
	}
	out, err := runGitIn(path, "diff", "--no-color", "--binary", baseRef, headRef)
	if err != nil {
		return "", fmt.Errorf("worktree.CaptureRangePatch: %w (output: %s)", err, out)
	}
	return out, nil
}

// CaptureRangePatchForPaths returns the unified patch between two refs, limited
// to a typed repo-relative path allowlist. It is read-only. Callers use this
// when a later build/test step may have generated tracked artifacts in the
// worktree but the review surface must remain bound to plan-owned paths.
func CaptureRangePatchForPaths(path, baseRef, headRef string, paths []string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("worktree.CaptureRangePatchForPaths: path is empty")
	}
	baseRef = strings.TrimSpace(baseRef)
	if baseRef == "" {
		return "", errors.New("worktree.CaptureRangePatchForPaths: baseRef is empty")
	}
	headRef = strings.TrimSpace(headRef)
	if headRef == "" {
		headRef = "HEAD"
	}
	cleanPaths, err := normalizeRangePatchPaths(paths)
	if err != nil {
		return "", fmt.Errorf("worktree.CaptureRangePatchForPaths: %w", err)
	}
	if len(cleanPaths) == 0 {
		return "", errors.New("worktree.CaptureRangePatchForPaths: paths is empty")
	}
	args := []string{"diff", "--no-color", "--binary", baseRef, headRef, "--"}
	args = append(args, cleanPaths...)
	out, err := runGitIn(path, args...)
	if err != nil {
		return "", fmt.Errorf("worktree.CaptureRangePatchForPaths: %w (output: %s)", err, out)
	}
	return out, nil
}

func normalizeRangePatchPaths(paths []string) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range paths {
		p := strings.TrimSpace(filepath.ToSlash(raw))
		p = strings.TrimPrefix(p, "./")
		if p == "" || p == "." {
			continue
		}
		if filepath.IsAbs(p) || strings.HasPrefix(p, "../") || strings.Contains(p, "/../") || p == ".." || strings.HasPrefix(p, ":") {
			return nil, fmt.Errorf("unsafe path %q", raw)
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}
