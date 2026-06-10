# libgit2 foreach_worktree operator precedence fixture

Source record:
- Repository: https://github.com/libgit2/libgit2
- Issue: https://github.com/libgit2/libgit2/issues/7216
- Fix PR: https://github.com/libgit2/libgit2/pull/7231

The upstream fix corrected operator precedence in `git_repository_foreach_worktree`
so negative callback and lookup errors are preserved instead of being collapsed
to boolean `1`.

This fixture is intentionally minimized to one C source file and one C test
driver. `make check` compiles and runs the driver.
