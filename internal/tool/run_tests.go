package tool

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RunTests is the verify-stage's test-execution tool. B0 ships it as
// a STUB because the real implementation requires the multi-language
// test-framework parser (one of the B3 open questions) plus safe
// subprocess handling inside a worktree. Execute returns a clear
// "not yet implemented" error so runVerifyPhase can dispatch the
// verify stage today and surface a fail-loud message rather than
// silently pass or hang.
//
// Red-line discipline (L3): Execute MUST NOT invoke ground.BuildContext.
// Test results are structured metric + pass/fail data, not citations.
// Enforced by write_mode_red_lines_test.go.
//
// Classified ReadOnly (tests do not mutate the repo they run inside;
// they read source + produce reports) + NonEvidenceTool (test output
// is not a citable repo fact — it's a verdict). This marks the tool
// as safe to include in any skill's ToolSuggestions without the
// is-write gate tripping, which matches B2/B3's intent.
type RunTests struct {
	ReadOnly
	NonEvidenceTool
}

// Name returns the tool's stable identifier.
func (t *RunTests) Name() string { return "run_tests" }

// Description covers both the B2/B3 intent and the current stub state
// so operators reading logs aren't surprised by the no-op behavior.
func (t *RunTests) Description() string {
	return "Run the project test suite inside the active worktree and stream results. " +
		"B0 SKELETON STUB — returns 'not yet implemented'; real implementation ships in B3 " +
		"with multi-language runner parsers (go test / pytest / npm test / cargo test)."
}

// Parameters describes the shape B3 will use. Execute ignores
// everything today, but the schema is stable so skills can declare
// the tool without adjustments when B3 lands.
func (t *RunTests) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "suite": {
      "type": "string",
      "description": "Test suite / package / pattern selector. Empty = all tests. Semantics are framework-specific (go test ./..., pytest tests/foo, npm test --testPathPattern=...)."
    },
    "timeout_seconds": {
      "type": "integer",
      "description": "Optional per-suite timeout override; defaults to 300."
    }
  }
}`)
}

// Execute is the B0 stub: always returns a clear error.
func (t *RunTests) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	_ = ctx
	_ = params
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   false,
		Summary:   "[B0 skeleton] run_tests not yet implemented — real test runner ships in B3 with multi-language parsers.",
		Timestamp: time.Now(),
	}, fmt.Errorf("run_tests: B0 skeleton stub; implementation deferred to B3")
}
