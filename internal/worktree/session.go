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
//   1. Worktree-as-dry-run: the apply stage never mutates the main
//      repo checkout. `git worktree add --detach HEAD` produces a
//      disposable copy; when the stage finishes (success OR failure)
//      the copy is discarded. The user can cherry-pick a successful
//      commit from the worktree into main manually — that flow lives
//      above this package.
//
//   2. Cleanup in three environments:
//        - Normal return: the orchestrator's outer defer fires
//          DiscardByPath. Idempotent with the active-sessions map.
//        - Panic: Go runs defers during unwind; same path as normal.
//        - SIGINT / SIGTERM: Go's default disposition is os.Exit(130)
//          WITHOUT running defers. InstallSignalHandler adds a
//          signal.Notify handler that walks the active-sessions map,
//          discards everyone, then re-raises so the process still
//          dies with the canonical signal exit code.
//        - SIGKILL: no cleanup possible in-process; PruneDeadSessions
//          at next startup reaps orphans by PID liveness check.
//
//   3. Idempotent Discard: the three-step sequence
//      (`git worktree remove --force` → `os.RemoveAll` → `git worktree
//      prune`) tolerates every state the disk might be in after a
//      crash: dir-only, metadata-only, both, or already clean. Each
//      step suppresses errors that reflect a prior step already
//      cleaning that surface.
//
//   4. Collision-free naming: directory format is
//      `<sanitized-trace-id>-<pid>`. Trace IDs carry `unix-nano` so
//      same-process concurrent Runs produce distinct IDs; pid
//      disambiguates across processes. Cross-process-pid-wraparound
//      is a theoretical edge case (a long-running system's PIDs
//      eventually cycle) that can leave one leaked worktree dir;
//      no correctness harm, only disk waste.
package worktree

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

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
//   1. `git worktree remove --force <path>` — tells git to drop the
//      worktree AND its metadata. --force tolerates a dirty tree
//      (which B0's apply-phase failures are likely to produce).
//      Errors are swallowed: a missing dir makes this a no-op but
//      still returns non-zero on some git versions.
//
//   2. `os.RemoveAll(path)` — belt-and-suspenders. If git refused
//      (wrong repo, already removed, etc) this makes sure the bytes
//      are gone. IsNotExist is suppressed because the common
//      success case from step 1 removed the dir already.
//
//   3. `git worktree prune` — sweeps any orphan metadata (the
//      `.git/worktrees/<id>` directory inside the main repo) that
//      step 1's error path might have left. Mirrors the post-SIGKILL
//      reap PruneDeadSessions does.
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
		sig := <-ch
		logging.Warning("[worktree] received %s, discarding %d active session(s)",
			sig, activeSessionCount())
		cleanActiveSessions()
		signal.Stop(ch)
		// Re-raise so the process dies with the canonical signal
		// exit code (130 for SIGINT, 143 for SIGTERM). Callers
		// expecting os.Exit(130) behavior still see it.
		if ssig, ok := sig.(syscall.Signal); ok {
			_ = syscall.Kill(os.Getpid(), ssig)
		} else {
			os.Exit(1)
		}
	}()
}

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
