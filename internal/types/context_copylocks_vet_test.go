package types

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBusContextCopylocksVetClean is the mechanical gate for the
// "never copy BusContext by value" invariant (see the BusContext doc
// header and ShallowClone in context_clone.go). The same defect class
// shipped twice — orchestrator/explore_parallel_dispatch.go's
// `workerBus := *o.busCtx` (fixed by introducing ShallowClone) and
// tool/emit_investigation_complete.go's counterfactual
// `clonedCtx := *ctx` (b85b7a45) — because nothing in `make test`
// ran vet. This pin runs `go vet -copylocks` on the packages with
// copy history and fails on any finding.
//
// Precise-signal note: the gate reads the vet exit code + diagnostic
// text (a typed toolchain analysis), NOT a regex over source for
// `:= *ctx` shapes — a lexical scan is a noisy signal and noisy
// signals must not drive hard gates.
//
// Scope is deliberately the three packages that have carried
// BusContext/AgentContext value copies, not ./... — vet type-checks
// (and cgo-compiles) everything it analyzes, and the tree-sitter
// packages make a whole-repo vet disproportionate inside `make test`.
// Warm-cache cost of this scope is ~2-4s.
func TestBusContextCopylocksVetClean(t *testing.T) {
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go"+exeSuffix())
	if _, err := exec.LookPath(goBin); err != nil {
		var lookErr error
		goBin, lookErr = exec.LookPath("go")
		if lookErr != nil {
			// Fail loud, never skip: a silently-skipped gate is no
			// gate, and every environment that runs this test binary
			// via `go test` has the toolchain by construction.
			t.Fatalf("go toolchain not found (GOROOT=%q, PATH lookup: %v) — copylocks gate cannot run", runtime.GOROOT(), lookErr)
		}
	}

	// This file lives at <module>/internal/types; run vet from the
	// package dir so module resolution works regardless of test CWD.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate package dir for vet")
	}
	pkgDir := filepath.Dir(thisFile)

	cmd := exec.Command(goBin, "vet", "-copylocks",
		"github.com/hanchaoqun/codrax/internal/types",
		"github.com/hanchaoqun/codrax/internal/tool",
		"github.com/hanchaoqun/codrax/internal/orchestrator",
	)
	cmd.Dir = pkgDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go vet -copylocks found lock-value copies (BusContext and friends must be cloned via ShallowClone, never `cp := *ctx`):\n%s\nvet error: %v",
			strings.TrimSpace(string(out)), err)
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
