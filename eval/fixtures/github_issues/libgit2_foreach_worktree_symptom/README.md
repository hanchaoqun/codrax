# libgit2 foreach_worktree error propagation symptom fixture

This fixture is reduced from libgit2 issue #7216 and PR #7231. Worktree traversal should preserve the exact negative status returned by callback and lookup paths.
