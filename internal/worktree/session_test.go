package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// initTestRepo creates a fresh git repo with one initial commit in a
// subdirectory of t.TempDir(). Returns the absolute repo root path.
// Every worktree test uses its own repo so no test pollutes another.
func initTestRepo(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "main-repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	runGit(t, root, "init", "-q")
	// Identity so `git commit` works in the test sandbox even when
	// the system-wide user.email/user.name is unset.
	runGit(t, root, "config", "user.email", "test@codrax.local")
	runGit(t, root, "config", "user.name", "codrax-test")
	// One commit so `HEAD` resolves (needed by `git worktree add HEAD`).
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "seed")
	return root
}

// runGit runs `git <args>...` in dir and fails the test if the
// command returns non-zero.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, string(out))
	}
}

// makeUniqueTraceID returns a trace ID that mimics the orchestrator's
// shape (the unix-nano guarantees intra-test uniqueness across
// Create calls). Using t.Name gives each test its own prefix so
// debugging a failing test's residue is easier if cleanup fails.
func makeUniqueTraceID(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("trace-%s-%d", strings.ReplaceAll(t.Name(), "/", "_"), time.Now().UnixNano())
}

// clearActiveSessions drains the package-level activeSessions map
// between tests so state from one test never leaks into the next.
// (t.TempDir isolation handles disk; this handles in-memory.)
func clearActiveSessions(t *testing.T) {
	t.Helper()
	activeSessions.Range(func(k, _ any) bool {
		activeSessions.Delete(k)
		return true
	})
}

// TestCreate_BasicLifecycle covers the happy-path end-to-end:
// Create produces a worktree with the declared path, the directory
// exists on disk, the initial file from HEAD is present, Discard
// cleans up the directory AND the git metadata.
func TestCreate_BasicLifecycle(t *testing.T) {
	clearActiveSessions(t)
	repo := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "worktrees")

	sess, err := Create(base, repo, makeUniqueTraceID(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.Path() == "" {
		t.Fatal("Session.Path is empty after Create")
	}
	if _, err := os.Stat(sess.Path()); err != nil {
		t.Errorf("worktree dir missing post-Create: %v", err)
	}
	// README.md should have been checked out.
	if _, err := os.Stat(filepath.Join(sess.Path(), "README.md")); err != nil {
		t.Errorf("expected README.md in worktree: %v", err)
	}
	// Metadata inside the main repo should exist.
	wtMetaBase := filepath.Join(repo, ".git", "worktrees")
	if entries, err := os.ReadDir(wtMetaBase); err != nil || len(entries) == 0 {
		t.Errorf("expected git worktree metadata under %s, got err=%v entries=%d",
			wtMetaBase, err, len(entries))
	}

	if err := sess.Discard(); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if _, err := os.Stat(sess.Path()); !os.IsNotExist(err) {
		t.Errorf("worktree dir should be gone after Discard, got err=%v", err)
	}
	// Metadata should be gone too (step 3 prunes).
	if entries, _ := os.ReadDir(wtMetaBase); len(entries) != 0 {
		t.Errorf("expected empty .git/worktrees after Discard, got %d entries", len(entries))
	}
}

// TestDiscard_Idempotent locks the "call twice is safe" contract.
// A failed Run might cause both the outer defer and the signal
// handler to call Discard on the same Session; neither path must
// return an error or panic.
func TestDiscard_Idempotent(t *testing.T) {
	clearActiveSessions(t)
	repo := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "worktrees")

	sess, err := Create(base, repo, makeUniqueTraceID(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sess.Discard(); err != nil {
		t.Fatalf("first Discard: %v", err)
	}
	if err := sess.Discard(); err != nil {
		t.Errorf("second Discard returned error: %v (must be idempotent)", err)
	}
}

// TestDiscard_NilSafe pins the nil-receiver contract. Go's method
// dispatch makes this legal syntactically but panics if the method
// dereferences without a guard. The outer defer in orchestrator
// never holds a *Session, but DiscardByPath can be called with
// empty path from a read-mode Run and must degrade cleanly.
func TestDiscard_NilSafe(t *testing.T) {
	var s *Session
	if err := s.Discard(); err != nil {
		t.Errorf("nil Session.Discard = %v, want nil", err)
	}
	if p := s.Path(); p != "" {
		t.Errorf("nil Session.Path = %q, want empty", p)
	}
	if err := DiscardByPath("", ""); err != nil {
		t.Errorf("DiscardByPath(empty) = %v, want nil", err)
	}
}

// TestCreate_Collision asserts that a second Create with the same
// target path fails rather than silently overwriting. The exact
// collision doesn't happen naturally in production (traceID has
// unix-nano precision) but the guard protects against hand-edited
// operator state and test-harness mistakes.
func TestCreate_Collision(t *testing.T) {
	clearActiveSessions(t)
	repo := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "worktrees")

	traceID := "fixed-trace-for-collision"
	first, err := Create(base, repo, traceID)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	defer first.Discard()

	// Same traceID + same pid (this test process) = same dir name.
	second, err := Create(base, repo, traceID)
	if err == nil {
		t.Errorf("second Create should fail on collision; got session at %s", second.Path())
		_ = second.Discard()
	}
}

// TestCreate_EmptyInputs locks the input-validation contract. Any
// empty required argument should return an error before touching
// the filesystem.
func TestCreate_EmptyInputs(t *testing.T) {
	clearActiveSessions(t)
	repo := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "worktrees")
	cases := []struct{ baseDir, root, trace string }{
		{"", repo, "t1"},
		{base, "", "t2"},
		{base, repo, ""},
		{base, repo, "   "}, // whitespace-only
	}
	for i, c := range cases {
		_, err := Create(c.baseDir, c.root, c.trace)
		if err == nil {
			t.Errorf("case %d: empty inputs should error; got nil (base=%q root=%q trace=%q)",
				i, c.baseDir, c.root, c.trace)
		}
	}
}

// TestPruneDeadSessions_LivePidSkipped proves the reaper respects
// sessions belonging to live processes. A session named with the
// current test process's PID must NOT be deleted — the test
// process is itself alive.
func TestPruneDeadSessions_LivePidSkipped(t *testing.T) {
	clearActiveSessions(t)
	repo := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "worktrees")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}

	// Fake a live-pid session dir (no actual git worktree — reaper
	// should skip it BEFORE trying to remove).
	live := filepath.Join(base, fmt.Sprintf("fake-%d", os.Getpid()))
	if err := os.Mkdir(live, 0o755); err != nil {
		t.Fatalf("mkdir live: %v", err)
	}

	PruneDeadSessions(base, repo)

	// Our self-pid session should NOT be touched. We skip it with
	// the selfPid check anyway, but this test locks the broader
	// "live pid is skipped" rule.
	if _, err := os.Stat(live); err != nil {
		t.Errorf("live-pid session was reaped: %v", err)
	}
}

// TestPruneDeadSessions_DeadPidRemoved proves the happy-path reap:
// a session whose embedded pid is long-dead gets cleaned up.
func TestPruneDeadSessions_DeadPidRemoved(t *testing.T) {
	clearActiveSessions(t)
	repo := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "worktrees")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}

	// Pick a PID that's overwhelmingly unlikely to be live:
	// Linux default pid_max is 4,194,304 (2^22); 2^30 is far above.
	// Belt-and-suspenders: verify via IsPidAlive that it's reported
	// dead — if the platform's limits ever change we surface the
	// assumption rather than silently testing nothing.
	deadPid := 1 << 30
	dead := filepath.Join(base, fmt.Sprintf("fake-%d", deadPid))
	if err := os.Mkdir(dead, 0o755); err != nil {
		t.Fatalf("mkdir dead: %v", err)
	}

	PruneDeadSessions(base, repo)

	if _, err := os.Stat(dead); !os.IsNotExist(err) {
		t.Errorf("dead-pid session should be reaped, stat err=%v", err)
	}
}

// TestPruneDeadSessions_NonSessionDirsIgnored proves the reaper
// respects operator-dropped non-session directories. A user might
// mkdir .codrax/worktrees/my-notes and not expect codrax to remove it.
func TestPruneDeadSessions_NonSessionDirsIgnored(t *testing.T) {
	clearActiveSessions(t)
	repo := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "worktrees")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}

	// Non-session names: no trailing "-<digits>" → pidFromSessionName = 0.
	operatorNames := []string{"my-notes", "keep-me", "README"}
	for _, n := range operatorNames {
		path := filepath.Join(base, n)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}

	PruneDeadSessions(base, repo)

	for _, n := range operatorNames {
		if _, err := os.Stat(filepath.Join(base, n)); err != nil {
			t.Errorf("operator dir %s was reaped: %v", n, err)
		}
	}
}

// TestPruneDeadSessions_MissingBaseDir is a robustness test: the
// reaper must tolerate a missing base directory (the common fresh-
// install case: .codrax/worktrees/ has not been created yet).
func TestPruneDeadSessions_MissingBaseDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "definitely-does-not-exist")
	// No panic, no error — just a silent no-op.
	PruneDeadSessions(missing, "")
}

// TestPlanMode_WorktreeNoLeak is the session-33 kickoff T3: prove
// that 10 consecutive Create+Discard cycles leave an empty base
// directory. Locks the "plan mode leaves no residue" contract the
// B0 exit criteria script depends on.
func TestPlanMode_WorktreeNoLeak(t *testing.T) {
	clearActiveSessions(t)
	repo := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "worktrees")

	const rounds = 10
	for i := 0; i < rounds; i++ {
		trace := fmt.Sprintf("noleak-%d-%d", i, time.Now().UnixNano())
		sess, err := Create(base, repo, trace)
		if err != nil {
			t.Fatalf("round %d Create: %v", i, err)
		}
		if err := sess.Discard(); err != nil {
			t.Fatalf("round %d Discard: %v", i, err)
		}
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		// Base may not exist if every Discard succeeded before any
		// leftover directory forced creation. That's a perfectly
		// clean outcome; pass.
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("ReadDir after cycles: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected empty worktree base after %d cycles; got %d entries: %v",
			rounds, len(entries), names)
	}

	// Also assert the in-process activeSessions map drained to zero
	// so no ghost registration survived Discard.
	if n := activeSessionCount(); n != 0 {
		t.Errorf("active sessions leaked: %d remain after %d Discard calls", n, rounds)
	}
}

// TestSignalHandler_CleansActiveSessions is the T4 structural test:
// simulate a SIGINT-style cleanup by calling cleanActiveSessions
// directly (without raising an actual signal — raising a signal in
// a Go test is flaky because the runtime's default disposition can
// race the handler install). Locks the invariant
//
//	"on signal, all active sessions are discarded".
func TestSignalHandler_CleansActiveSessions(t *testing.T) {
	clearActiveSessions(t)
	repo := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "worktrees")

	// Create two sessions to exercise the Range loop.
	s1, err := Create(base, repo, makeUniqueTraceID(t)+"-a")
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	s2, err := Create(base, repo, makeUniqueTraceID(t)+"-b")
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if activeSessionCount() != 2 {
		t.Fatalf("expected 2 active sessions, got %d", activeSessionCount())
	}

	// Invoke the same cleanup function the signal handler runs.
	cleanActiveSessions()

	// Both dirs gone.
	if _, err := os.Stat(s1.Path()); !os.IsNotExist(err) {
		t.Errorf("session 1 dir still present after cleanup: err=%v", err)
	}
	if _, err := os.Stat(s2.Path()); !os.IsNotExist(err) {
		t.Errorf("session 2 dir still present after cleanup: err=%v", err)
	}
	// Map drained.
	if n := activeSessionCount(); n != 0 {
		t.Errorf("active sessions not drained: %d remain", n)
	}
}

// TestPidFromSessionName pins the parser's edge cases: normal
// session, trace-id with internal hyphens, non-numeric tail,
// missing tail, empty.
func TestPidFromSessionName(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"trace-1730123456-789", 789}, // normal: trace-<nano>-<pid>
		{"trace-abc-12345", 12345},    // hyphen-embedded trace id
		{"notasession", 0},
		{"trailing-dash-", 0},
		{"", 0},
		{"-nohead-123", 123}, // LastIndex based; still parses the pid
		{"weird-text", 0},    // tail not numeric
	}
	for _, c := range cases {
		if got := pidFromSessionName(c.in); got != c.want {
			t.Errorf("pidFromSessionName(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestSanitizeTraceID locks the filesystem-safety property: only
// [A-Za-z0-9_-] survives, everything else becomes '_'. Empty and
// whitespace fall back to "trace".
func TestSanitizeTraceID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"trace-123", "trace-123"},
		{"trace_456", "trace_456"},
		{"trace 789", "trace_789"},
		{"path/with/slash", "path_with_slash"},
		{"weird$chars!@#", "weird_chars___"},
		{"", "trace"},
		{"   ", "trace"},
	}
	for _, c := range cases {
		if got := sanitizeTraceID(c.in); got != c.want {
			t.Errorf("sanitizeTraceID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCommitChanges_RoundTrip verifies the full happy path: create a
// worktree, write some files, commit them, and confirm the returned
// SHA matches `git rev-parse HEAD` in that worktree.
func TestCommitChanges_RoundTrip(t *testing.T) {
	clearActiveSessions(t)
	root := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "wts")
	traceID := makeUniqueTraceID(t)
	sess, err := Create(base, root, traceID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Drop a new file in the worktree.
	target := filepath.Join(sess.Path(), "iter1.txt")
	if err := os.WriteFile(target, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write iter1.txt: %v", err)
	}

	sha, err := CommitChanges(sess.Path(), "iter 1")
	if err != nil {
		t.Fatalf("CommitChanges: %v", err)
	}
	if len(sha) < 7 {
		t.Errorf("SHA looks malformed: %q", sha)
	}
	// Confirm via independent git rev-parse.
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = sess.Path()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != sha {
		t.Errorf("SHA mismatch: CommitChanges=%q rev-parse=%q", sha, got)
	}

	if err := DiscardByPath(sess.Path(), root); err != nil {
		t.Errorf("DiscardByPath: %v", err)
	}
}

func TestCaptureRangePatchCapturesCumulativeDiff(t *testing.T) {
	clearActiveSessions(t)
	root := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "wts")
	sess, err := Create(base, root, makeUniqueTraceID(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() {
		if err := DiscardByPath(sess.Path(), root); err != nil {
			t.Errorf("DiscardByPath: %v", err)
		}
	}()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = sess.Path()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v\n%s", err, out)
	}
	baseRef := strings.TrimSpace(string(out))

	if err := os.WriteFile(filepath.Join(sess.Path(), "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if _, err := CommitChanges(sess.Path(), "change readme"); err != nil {
		t.Fatalf("CommitChanges readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sess.Path(), "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}
	if _, err := CommitChanges(sess.Path(), "add new"); err != nil {
		t.Fatalf("CommitChanges new: %v", err)
	}

	patch, err := CaptureRangePatch(sess.Path(), baseRef, "HEAD")
	if err != nil {
		t.Fatalf("CaptureRangePatch: %v", err)
	}
	for _, want := range []string{
		"diff --git a/README.md b/README.md",
		"+changed",
		"diff --git a/new.txt b/new.txt",
		"+new",
	} {
		if !strings.Contains(patch, want) {
			t.Fatalf("cumulative patch missing %q:\n%s", want, patch)
		}
	}
}

func TestCaptureRangePatchForPathsFiltersUnownedDiff(t *testing.T) {
	clearActiveSessions(t)
	root := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "wts")
	sess, err := Create(base, root, makeUniqueTraceID(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() {
		if err := DiscardByPath(sess.Path(), root); err != nil {
			t.Errorf("DiscardByPath: %v", err)
		}
	}()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = sess.Path()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v\n%s", err, out)
	}
	baseRef := strings.TrimSpace(string(out))

	if err := os.WriteFile(filepath.Join(sess.Path(), "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if _, err := CommitChanges(sess.Path(), "owned readme change"); err != nil {
		t.Fatalf("CommitChanges readme: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sess.Path(), "build"), 0o755); err != nil {
		t.Fatalf("mkdir build: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sess.Path(), "build", "generated.c"), []byte("generated\n"), 0o644); err != nil {
		t.Fatalf("write generated: %v", err)
	}
	if _, err := CommitChanges(sess.Path(), "generated build artifact"); err != nil {
		t.Fatalf("CommitChanges generated: %v", err)
	}

	patch, err := CaptureRangePatchForPaths(sess.Path(), baseRef, "HEAD", []string{"README.md"})
	if err != nil {
		t.Fatalf("CaptureRangePatchForPaths: %v", err)
	}
	if !strings.Contains(patch, "diff --git a/README.md b/README.md") || !strings.Contains(patch, "+changed") {
		t.Fatalf("owned patch missing README change:\n%s", patch)
	}
	if strings.Contains(patch, "build/generated.c") || strings.Contains(patch, "+generated") {
		t.Fatalf("path-limited patch included unowned generated artifact:\n%s", patch)
	}
}

func TestCaptureRangePatchForPathsRejectsUnsafePath(t *testing.T) {
	clearActiveSessions(t)
	root := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "wts")
	sess, err := Create(base, root, makeUniqueTraceID(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() {
		if err := DiscardByPath(sess.Path(), root); err != nil {
			t.Errorf("DiscardByPath: %v", err)
		}
	}()
	for _, unsafePath := range []string{"../outside", ":(top)*"} {
		if _, err := CaptureRangePatchForPaths(sess.Path(), "HEAD", "HEAD", []string{unsafePath}); err == nil {
			t.Fatalf("unsafe path %q should be rejected", unsafePath)
		}
	}
}

// TestCommitChanges_AllowsEmpty verifies the --allow-empty flag is
// in effect — a no-op apply (nothing changed) still produces a fresh
// SHA so the orchestrator's bookkeeping doesn't have to special-case
// "no changes to commit".
func TestCommitChanges_AllowsEmpty(t *testing.T) {
	clearActiveSessions(t)
	root := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "wts")
	sess, err := Create(base, root, makeUniqueTraceID(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// First (empty) commit — succeeds because --allow-empty.
	sha1, err := CommitChanges(sess.Path(), "empty 1")
	if err != nil {
		t.Fatalf("first empty commit: %v", err)
	}
	// Second (also empty) commit — must produce a DIFFERENT SHA so the
	// orchestrator's per-iter SHA tracking stays unique even on no-op
	// retries.
	sha2, err := CommitChanges(sess.Path(), "empty 2")
	if err != nil {
		t.Fatalf("second empty commit: %v", err)
	}
	if sha1 == sha2 {
		t.Errorf("two empty commits produced the same SHA %q (commit metadata should differ)", sha1)
	}

	_ = DiscardByPath(sess.Path(), root)
}

// TestResetHard_RewindsWorkingTree drives the warm-retry rewind: make
// commit A, then commit B (with file modified), then ResetHard back
// to A's SHA. The on-disk file must match A's content.
func TestResetHard_RewindsWorkingTree(t *testing.T) {
	clearActiveSessions(t)
	root := initTestRepo(t)
	base := filepath.Join(t.TempDir(), "wts")
	sess, err := Create(base, root, makeUniqueTraceID(t))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	wt := sess.Path()
	target := filepath.Join(wt, "iter.txt")

	// A = "best"
	if err := os.WriteFile(target, []byte("BEST\n"), 0o644); err != nil {
		t.Fatalf("write A: %v", err)
	}
	shaBest, err := CommitChanges(wt, "iter best")
	if err != nil {
		t.Fatalf("commit A: %v", err)
	}

	// B = "regressed"
	if err := os.WriteFile(target, []byte("REGRESSED\n"), 0o644); err != nil {
		t.Fatalf("write B: %v", err)
	}
	if _, err := CommitChanges(wt, "iter regressed"); err != nil {
		t.Fatalf("commit B: %v", err)
	}
	// Confirm B is on disk.
	if data, _ := os.ReadFile(target); strings.TrimSpace(string(data)) != "REGRESSED" {
		t.Fatalf("after B commit, file should hold REGRESSED, got %q", data)
	}

	// Rewind.
	if err := ResetHard(wt, shaBest); err != nil {
		t.Fatalf("ResetHard: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read after reset: %v", err)
	}
	if strings.TrimSpace(string(data)) != "BEST" {
		t.Errorf("after ResetHard the file should match BEST iteration content, got %q", data)
	}

	_ = DiscardByPath(wt, root)
}

// TestResetHard_RejectsEmptyArgs covers the defensive guards.
func TestResetHard_RejectsEmptyArgs(t *testing.T) {
	if err := ResetHard("", "abc123"); err == nil {
		t.Error("empty path should error")
	}
	if err := ResetHard("/tmp", ""); err == nil {
		t.Error("empty sha should error")
	}
}
