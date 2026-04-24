package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestBinaryBuildsAndSizeReasonable is the session-33 B0 Day-6 T2
// exit check. Locks two invariants:
//
//   1. The codrax binary builds cleanly from the current tree. A
//      build failure here means the module's top-level go build
//      broke — every other test in the suite runs against the
//      library view of the code and would miss this.
//
//   2. The resulting binary size is in a "sane" range: at least
//      1 MB (codrax has a non-trivial dependency graph — repomap,
//      go/ast, anthropic SDK, etc. — so a <1 MB binary means most
//      of the code never got compiled in, almost certainly a
//      linker mis-configuration) and at most 200 MB (a blown-up
//      binary signals an accidental embedding of a huge resource
//      or a debug-info leak).
//
// The "<5% binary size growth" check from the B0 exit criteria is
// deliberately NOT in this test. That comparison needs a baseline
// commit on disk (e.g. the pre-B0 HEAD binary built with the same
// toolchain) and is a manual pre-release step operators run with
// `git checkout <pre-b0> && go build` on both sides. A per-commit
// test that hard-codes a size would either rot (every real change
// moves the number) or be useless (the 5% window covers almost any
// legitimate growth).
//
// Cost: the test spawns `go build` once, ~2-4 seconds on a typical
// machine. Acceptable for B0 since it runs once per invocation of
// `go test ./cmd/`. Tagged with testing.Short() to skip under -short.
func TestBinaryBuildsAndSizeReasonable(t *testing.T) {
	if testing.Short() {
		t.Skip("TestBinaryBuildsAndSizeReasonable spawns go build; skipped under -short")
	}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "codrax-test-binary")

	// Build from the module root. The test runs in cmd/ but the
	// main package lives at the repo root (../); -C <dir> + "."
	// avoids relying on the test's CWD.
	modRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("abs module root: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", outPath, ".")
	cmd.Dir = modRoot
	// Preserve PATH / HOME / etc. Go build also reads GOPATH,
	// GOCACHE, GOMODCACHE — carry the full env through so the
	// module-aware build works identically to a user invocation.
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\noutput: %s", err, string(out))
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat built binary: %v", err)
	}

	const (
		minSize = 1 * 1024 * 1024   // 1 MB — sanity floor
		maxSize = 200 * 1024 * 1024 // 200 MB — sanity cap
	)

	size := info.Size()
	if size < minSize {
		t.Errorf("codrax binary is suspiciously small: %d bytes (< 1 MB). "+
			"A tiny binary usually means the linker stripped too much or a dependency failed to compile.", size)
	}
	if size > maxSize {
		t.Errorf("codrax binary is suspiciously large: %d bytes (> 200 MB). "+
			"An oversized binary usually means an embedded resource leaked, "+
			"or debug info was accidentally retained.", size)
	}
	t.Logf("codrax binary size: %d bytes (%.1f MB) — within the 1-200 MB sanity range",
		size, float64(size)/(1024*1024))
}
