package tool

import (
	"context"
	"os/exec"
	"time"
)

// gitDefaultTimeout bounds every git invocation so a wedged network
// (git-lfs, git-prompt credential helpers) or a pathological repo
// (millions of untracked files) cannot stall the pipeline.
//
// Chosen by call-site surface:
//   - Metadata calls (rev-parse, ls-files, diff --name-only, log --name-only)
//     finish in under 1 s on 500k-file monorepos when warm; 30 s gives
//     a 30× headroom.
//   - User-facing git_diff / git_log tools can reasonably take longer
//     on very large ranges; they use NewGitLongCommand (60 s).
const (
	gitMetadataTimeout = 30 * time.Second
	gitLongTimeout     = 60 * time.Second
)

// NewGitCommand returns a context-bounded `git` *exec.Cmd for quick
// metadata queries. The cancel func MUST be called (typically via
// defer) regardless of whether Run/Output succeeds, to release the
// timer. When parent is nil, context.Background is used.
func NewGitCommand(parent context.Context, args ...string) (*exec.Cmd, context.CancelFunc) {
	return newGitCommand(parent, gitMetadataTimeout, args...)
}

// NewGitLongCommand is like NewGitCommand but with a more permissive
// deadline, intended for user-facing diff/log tools where the work
// is proportional to ref range and can legitimately be slower.
func NewGitLongCommand(parent context.Context, args ...string) (*exec.Cmd, context.CancelFunc) {
	return newGitCommand(parent, gitLongTimeout, args...)
}

func newGitCommand(parent context.Context, timeout time.Duration, args ...string) (*exec.Cmd, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	return exec.CommandContext(ctx, "git", args...), cancel
}
