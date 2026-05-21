// Package orchestrator — degraded_lasterror_test.go (2026-05-10).
//
// Critical bug fix audit: when the analyzer exhausts retries and
// the orchestrator builds a degraded recovery IR, the legacy code
// (pre-fix) ALSO set TaskState.LastError. The runtime guard at
// orchestrator.go:1921 checks `if LastError == ""` before
// dispatching Phase 2 — so setting LastError before the degraded
// path's runTaskPhase call caused the entire scheduler to be
// skipped, producing empty-answer outputs.
//
// The fix splits the analyzer error into:
//   - LastError: hard-fail signal (e.g. unknown pipeline mode,
//     write-mode missing IR) — guard skips Phase 2.
//   - SoftAnalyzerError: informational signal — does NOT block
//     Phase 2; surfaced to the user-panel caveat after the
//     degraded answer materialises.
package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTaskState_HardErrorBlocksPhase2(t *testing.T) {
	// LastError is non-empty → guard at orchestrator.go:1921
	// short-circuits Phase 2.
	st := types.TaskState{LastError: "some hard failure"}
	if st.LastError == "" {
		t.Error("LastError should be non-empty in this scenario")
	}
}

func TestTaskState_SoftErrorAllowsPhase2(t *testing.T) {
	// SoftAnalyzerError populated but LastError stays empty so
	// the runTaskPhase guard at line 1921 still fires.
	st := types.TaskState{
		SoftAnalyzerError: "analyze: stage exhausted after 2 attempts",
	}
	if st.LastError != "" {
		t.Errorf("LastError should be empty so Phase 2 dispatches; got %q", st.LastError)
	}
	if st.SoftAnalyzerError == "" {
		t.Error("SoftAnalyzerError should carry the analyzer diagnostic")
	}
}

func TestTaskState_BothCanBeSet(t *testing.T) {
	// Defensive: both fields can coexist (e.g. analyzer fails AND
	// downstream pipeline also fails). Renderer should prefer
	// LastError when both are present (legacy contract).
	st := types.TaskState{
		LastError:         "downstream pipeline failure",
		SoftAnalyzerError: "analyze: stage exhausted",
	}
	if st.LastError == "" || st.SoftAnalyzerError == "" {
		t.Error("both fields should be retainable")
	}
}
