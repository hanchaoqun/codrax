package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInHints is internal/agent's marker in the
// glossarylint renderer roster (§40.52). It statically scans every
// non-test .go file directly under internal/agent/ for string literals
// that contain a glossary token and are not direct logger arguments,
// import paths, or struct tags.
//
// The dominant concern is text that ends up in the model's prompt
// context — LoopSignal.Hint, BuildInitialInstruction returns, retry
// hints, mid-loop re-prompts, StageOutput.Error / .StageReport (the
// orchestrator renders these into the next dispatch's Retry
// Directive). Const values and *Error assignments are deliberately
// in scope here (Policy{}): unlike the orchestrator's enum consts and
// TaskState.LastError, prose consts and StageOutput.Error in this
// package do reach the next prompt.
//
// Comment-only mentions are not literals and are ignored. The dynamic
// twin — prompt_snapshot_test.go — renders every evaluator against a
// fixture and scans the composed output, catching leaks that only
// appear after fmt.Sprintf / helper composition.
//
// Batch 1 policy: report-only. Batch 2B cleaned the hint/error
// surfaces and batch 4B promoted the gate to t.Fatal. §40.52 retired
// the package-local matcher copy in favour of glossarylint.
func TestNoInternalTermsInHints(t *testing.T) {
	glossarylint.RunPackageScan(t, ".", glossarylint.Policy{})
}
