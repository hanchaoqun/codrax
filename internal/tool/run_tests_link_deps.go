package tool

import (
	"os"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// linkProjectDeps creates symlinks in the worktree pointing to the
// main repo's gitignored dependency directories. Without this,
// runners that consume dependencies by name relative to cwd (Node's
// `node_modules/`, Ruby's `vendor/`, hvigor's `oh_modules/`) fail
// during verify with "module not found" because the worktree is a
// detached-HEAD checkout that contains exactly what `git ls-files`
// outputs and excludes anything gitignored.
//
// Per-runner candidate dirs:
//
//	node:    node_modules/
//	ruby:    vendor/, vendor/bundle/, .bundle/
//	hvigor:  oh_modules/, node_modules/
//
// Other runners are unaffected because their dep resolution doesn't
// require a project-relative cwd directory:
//
//   - python is handled by pythonInterpreter probing the venv at
//     mainRoot/.venv/bin/python directly; sys.path resolution finds
//     pytest there regardless of cwd.
//   - go uses GOPATH/pkg/mod (global cache); rebuilds from source.
//   - rust uses ~/.cargo (global); rebuilds.
//   - swift uses .build/ (rebuilds from source).
//   - java maven uses ~/.m2 (global); gradle uses ~/.gradle (global).
//   - cmake / meson run inside an explicit build dir, which the
//     verify-stage configuration path requires the user to set up
//     before /approve fires.
//   - cjpm rebuilds from source.
//   - make is project-defined; whatever the Makefile assumes.
//
// Linking rules:
//   - skip when src doesn't exist at mainRoot (project doesn't have
//     the dep dir installed)
//   - skip when dst already exists at worktreeRoot (dep is committed
//     to the repo — uncommon but valid)
//   - log at info on success, warning on failure; never block the
//     exec — the runner's own "missing deps" message is the
//     authoritative diagnostic if the link can't be created
//
// Symlink semantics: Linux/macOS create real symlinks via os.Symlink.
// Windows requires Developer Mode or admin for symlinks; without it
// the call returns an error and we fall back to a warning. Operators
// on Windows can either enable Developer Mode (Windows 10/11
// supported) or `cd worktree` and run `npm install` themselves to
// materialise node_modules in the worktree before re-running
// /approve.
//
// Cleanup: not needed. The worktree is discarded by the orchestrator
// outer defer (or preserved per pipeline_keep_worktree_on_success).
// Symlinks under the worktree are removed when the dir is removed.
//
// Read-vs-write: this assumes test runners READ from the linked dirs
// and don't WRITE. `npm test` / `bundle exec rspec` / `hvigorw test`
// follow that contract in practice. If a future runner mutates
// `node_modules` during test execution, the writes WOULD reach the
// main repo — flagging this here so a future runner addition stays
// alert to the constraint.
func linkProjectDeps(mainRoot, worktreeRoot, runner string) {
	if mainRoot == "" || worktreeRoot == "" || mainRoot == worktreeRoot {
		return
	}
	candidates := projectDepCandidates(runner)
	if len(candidates) == 0 {
		return
	}
	for _, name := range candidates {
		src := filepath.Join(mainRoot, name)
		dst := filepath.Join(worktreeRoot, name)
		srcInfo, err := os.Stat(src)
		if err != nil || !srcInfo.IsDir() {
			continue
		}
		if _, err := os.Lstat(dst); err == nil {
			// dst exists — committed dep dir or a previous link.
			// Skip rather than overwrite.
			continue
		}
		// Parent of dst must exist for some nested candidates
		// (`vendor/bundle` requires `vendor/`). MkdirAll is a no-op
		// when the parent already exists.
		if parent := filepath.Dir(dst); parent != worktreeRoot {
			if err := os.MkdirAll(parent, 0o755); err != nil {
				logging.Warning("[run_tests] linkProjectDeps %s: mkdir %s: %v",
					runner, parent, err)
				continue
			}
		}
		if err := os.Symlink(src, dst); err != nil {
			logging.Warning("[run_tests] linkProjectDeps %s: symlink %s → %s: %v",
				runner, src, dst, err)
			continue
		}
		logging.Info("[run_tests] linked %s → %s (worktree dep import)", dst, src)
	}
}

// projectDepCandidates returns the gitignored dependency directory
// names that runner consumes from cwd. Empty slice means the runner
// doesn't need cwd-relative dep linking. Kept as a separate
// function so the test suite can exhaustively assert the linking
// surface.
func projectDepCandidates(runner string) []string {
	switch runner {
	case "node":
		return []string{"node_modules"}
	case "ruby":
		// vendor first so the parent dir exists by the time we try
		// vendor/bundle (which is rare but supported via
		// `bundle install --path vendor/bundle`). .bundle/ is
		// bundler's per-project config dir.
		return []string{"vendor", "vendor/bundle", ".bundle"}
	case "hvigor":
		// HarmonyOS DevEco projects can use either the legacy
		// node_modules layout (when build tooling pulls npm deps)
		// or the newer oh_modules layout. Link both when present.
		return []string{"oh_modules", "node_modules"}
	}
	return nil
}
