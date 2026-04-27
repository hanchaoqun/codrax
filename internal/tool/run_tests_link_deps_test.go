package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestProjectDepCandidates_Coverage locks the runner→deps mapping
// so adding a new runner forces an explicit decision about whether
// it needs cwd-relative dep linking. Drift-guard.
func TestProjectDepCandidates_Coverage(t *testing.T) {
	for _, tc := range []struct {
		runner   string
		wantSome bool
	}{
		// Runners that need linking.
		{"node", true},
		{"ruby", true},
		{"hvigor", true},
		// Runners that resolve via global caches / source
		// rebuild and don't need cwd-relative linking.
		{"go", false},
		{"python", false}, // handled via pythonInterpreter probe
		{"rust", false},
		{"java", false},
		{"swift", false},
		{"cmake", false},
		{"meson", false},
		{"make", false},
		{"cjpm", false},
		// Defensive: unknown runner returns empty.
		{"does-not-exist", false},
	} {
		got := projectDepCandidates(tc.runner)
		if (len(got) > 0) != tc.wantSome {
			t.Errorf("projectDepCandidates(%q) = %v; wantSome=%v",
				tc.runner, got, tc.wantSome)
		}
	}
}

// TestLinkProjectDeps_NodeModulesSymlink verifies the node case:
// main has node_modules/, worktree doesn't, link is created.
func TestLinkProjectDeps_NodeModulesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires Developer Mode on Windows")
	}
	main := t.TempDir()
	worktree := t.TempDir()
	if err := os.MkdirAll(filepath.Join(main, "node_modules", "lodash"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(main, "node_modules", "lodash", "index.js"), []byte("module.exports = {}\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	linkProjectDeps(main, worktree, "node")

	dst := filepath.Join(worktree, "node_modules")
	info, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("expected symlink at %s; got: %v", dst, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("dst exists but is not a symlink: mode=%v", info.Mode())
	}
	// Reading through the symlink must reach main's content.
	stat, err := os.Stat(filepath.Join(dst, "lodash", "index.js"))
	if err != nil || stat.Size() == 0 {
		t.Errorf("symlink doesn't reach main's node_modules content: %v", err)
	}
}

// TestLinkProjectDeps_SkipsWhenWorktreeAlreadyHasIt verifies the
// "committed deps" case: worktree has its own node_modules already,
// linkProjectDeps doesn't overwrite it.
func TestLinkProjectDeps_SkipsWhenWorktreeAlreadyHasIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires Developer Mode on Windows")
	}
	main := t.TempDir()
	worktree := t.TempDir()
	_ = os.MkdirAll(filepath.Join(main, "node_modules", "lodash"), 0o755)
	// Pre-existing worktree node_modules — must not be replaced.
	preExisting := filepath.Join(worktree, "node_modules", "preserve.txt")
	_ = os.MkdirAll(filepath.Dir(preExisting), 0o755)
	_ = os.WriteFile(preExisting, []byte("worktree-owned"), 0o644)

	linkProjectDeps(main, worktree, "node")

	// File still there?
	data, err := os.ReadFile(preExisting)
	if err != nil || string(data) != "worktree-owned" {
		t.Errorf("preserve.txt was overwritten or removed: err=%v data=%q", err, data)
	}
	// Worktree's node_modules MUST NOT be a symlink (we skipped).
	info, _ := os.Lstat(filepath.Join(worktree, "node_modules"))
	if info != nil && info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("linkProjectDeps overwrote pre-existing committed dep dir with a symlink")
	}
}

// TestLinkProjectDeps_RubyMultipleCandidates verifies that ruby
// gets all three candidate dirs linked (vendor, vendor/bundle,
// .bundle) when each exists at main.
func TestLinkProjectDeps_RubyMultipleCandidates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires Developer Mode on Windows")
	}
	main := t.TempDir()
	worktree := t.TempDir()
	for _, p := range []string{"vendor", "vendor/bundle", ".bundle"} {
		if err := os.MkdirAll(filepath.Join(main, p), 0o755); err != nil {
			t.Fatalf("setup %s: %v", p, err)
		}
	}

	linkProjectDeps(main, worktree, "ruby")

	// vendor (the parent) should be a symlink. Once vendor/ is a
	// symlink, vendor/bundle is reached through it (the symlink
	// target's bundle subdir), so we don't expect a separate
	// symlink at worktree/vendor/bundle.
	if info, err := os.Lstat(filepath.Join(worktree, "vendor")); err != nil ||
		info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("vendor symlink missing: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(worktree, ".bundle")); err != nil ||
		info.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".bundle symlink missing: %v", err)
	}
}

// TestLinkProjectDeps_NoOpWhenMainLacksDir verifies the helper is
// silent when the main repo doesn't have the candidate dep dir
// (project never installed deps; common on a fresh scaffold).
func TestLinkProjectDeps_NoOpWhenMainLacksDir(t *testing.T) {
	main := t.TempDir()
	worktree := t.TempDir()
	// main has no node_modules.

	linkProjectDeps(main, worktree, "node")

	if _, err := os.Lstat(filepath.Join(worktree, "node_modules")); err == nil {
		t.Error("linkProjectDeps created a symlink when main had no node_modules to link to")
	}
}

// TestLinkProjectDeps_PythonNoOp verifies the runner-router skips
// python entirely (handled by pythonInterpreter, not by linking).
func TestLinkProjectDeps_PythonNoOp(t *testing.T) {
	main := t.TempDir()
	worktree := t.TempDir()
	_ = os.MkdirAll(filepath.Join(main, ".venv", "bin"), 0o755)

	linkProjectDeps(main, worktree, "python")

	// Worktree must NOT have a .venv symlink — python flow uses
	// the interpreter path probe, not symlinking.
	if _, err := os.Lstat(filepath.Join(worktree, ".venv")); err == nil {
		t.Error("python runner should not trigger .venv linking")
	}
}

// TestLinkProjectDeps_EmptyRoots verifies defensive no-op when
// either root is empty (defends against future caller bugs).
func TestLinkProjectDeps_EmptyRoots(t *testing.T) {
	worktree := t.TempDir()
	_ = os.MkdirAll(filepath.Join(worktree, "guard"), 0o755) // no-op marker

	linkProjectDeps("", worktree, "node")
	linkProjectDeps("/some/path", "", "node")
	linkProjectDeps("/same/path", "/same/path", "node") // mainRoot==worktreeRoot → no-op

	// Worktree shouldn't have any node_modules from any of the calls.
	if _, err := os.Lstat(filepath.Join(worktree, "node_modules")); err == nil {
		t.Error("linkProjectDeps with empty / equal roots should be a no-op")
	}
}

// TestPythonInterpreter_ProbesMainRootWhenWorktreeLacksVenv covers
// the customer-reported regression: pytest installed at <main>/.venv
// but verify runs in <worktree>/, which doesn't have .venv. The
// fix threads MainRepoRoot through so the probe finds the venv at
// the second root.
func TestPythonInterpreter_ProbesMainRootWhenWorktreeLacksVenv(t *testing.T) {
	worktree := t.TempDir()
	main := t.TempDir()
	pyDir := filepath.Join(main, ".venv", "bin")
	if err := os.MkdirAll(pyDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pyPath := filepath.Join(pyDir, "python")
	if err := os.WriteFile(pyPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Worktree first (empty), main second (has .venv). Caller
	// passes both in the order the verify-stage code does. Probe
	// MUST find main's venv.
	interp, asModule := pythonInterpreter(worktree, main)
	if !asModule {
		t.Errorf("asModule should be true when main has .venv")
	}
	if !strings.Contains(interp, ".venv") {
		t.Errorf("interpreter should point at venv; got %q", interp)
	}
	if !strings.Contains(interp, filepath.ToSlash(main)) {
		t.Errorf("interpreter should resolve from MAIN root, not worktree; got %q (main=%q)",
			interp, main)
	}
}

// TestPythonInterpreter_WorktreeWinsOverMainWhenBothHaveVenv: if
// the worktree happens to have its own .venv (rare — venv tracked
// in git or copied in), it wins as the first probed root. Locks the
// priority order.
func TestPythonInterpreter_WorktreeWinsOverMainWhenBothHaveVenv(t *testing.T) {
	worktree := t.TempDir()
	main := t.TempDir()
	for _, r := range []string{worktree, main} {
		dir := filepath.Join(r, ".venv", "bin")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "python"), []byte("#!/bin/sh\n"), 0o755)
	}
	interp, _ := pythonInterpreter(worktree, main)
	if !strings.Contains(interp, filepath.ToSlash(worktree)) {
		t.Errorf("when both have .venv, worktree (first arg) should win; got %q", interp)
	}
}
