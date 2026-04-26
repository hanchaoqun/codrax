//go:build windows

package tool

// wrapShellCommandWithCaps is a no-op on Windows: the JobObject in
// supervisedRunPlatform_windows already enforces memory and CPU
// limits at the kernel layer (JobMemoryLimit + PerJobUserTimeLimit).
// Wrapping the shell command with `ulimit` would do nothing on cmd.exe
// (no such builtin) and would only confuse a Git-Bash sh fallback
// when the limits are already installed at the JobObject level.
func wrapShellCommandWithCaps(cmdStr string, opts SupervisedRunOptions) string {
	_ = opts
	return cmdStr
}
