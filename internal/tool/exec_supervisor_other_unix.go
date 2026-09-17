//go:build !windows && !darwin

package tool

import (
	"context"
	"os/exec"
)

func supervisedRunPlatform(ctx context.Context, cmd *exec.Cmd, opts SupervisedRunOptions) SupervisedResult {
	return supervisedRunUnixOneShot(ctx, cmd, opts)
}
