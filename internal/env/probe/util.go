package probe

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// captureCmd runs cmd args and returns combined stdout/stderr,
// bounded by a tight timeout. Used for `--version` style probes
// — these should return in milliseconds; if they hang we don't
// want to delay the whole probe pipeline.
func captureCmd(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...)
	out, err := c.CombinedOutput()
	return string(out), err
}

// runQuick executes a command and returns nil iff exit==0. Output
// discarded — used for "does this Python have pytest" style yes/no
// probes. 1 second timeout (Python startup can be slow on cold
// caches).
func runQuick(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...)
	c.Stdout = nil
	c.Stderr = nil
	return c.Run()
}

// isExecutable returns true when path exists and is executable.
// Honours symlinks (probes the link target's mode). Returns false
// on Windows as a conservative default — Windows execute semantics
// are extension-based, not mode-bit based, so we treat the file as
// executable iff it exists.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	mode := info.Mode()
	if mode&0o111 != 0 {
		return true
	}
	// On Windows, fall back to "exists is enough" — Mode bits
	// don't carry execute info there.
	return strings.HasSuffix(strings.ToLower(path), ".exe") ||
		strings.HasSuffix(strings.ToLower(path), ".bat") ||
		strings.HasSuffix(strings.ToLower(path), ".cmd")
}

// fileExists is the lighter sibling of isExecutable for non-binary
// markers (.gitignore, package.json, etc.).
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
