package tool

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitTestResults is the verify-stage's structured exit channel — it
// transforms raw test runner stdout into a parsed TestResult slice
// the verifier agent can emit as its final answer. B0 ships it as a
// STUB paired with RunTests; real implementation lands in B3 when
// the per-language parsers are available.
//
// When B3 lands this tool will:
//   - decode the verifier's emit payload (pass/fail per assertion)
//   - validate that the assertion IDs map to ChangePlan.AcceptanceTests
//   - install a ChangeReport on BusContext.Mutable for finalize
//     (or plan-out disk writer) to serialize
//
// For B0 Execute returns a clear "not yet implemented" error so the
// verify-stage dispatch chain surfaces an honest state.
//
// Red-line discipline (L3): Execute MUST NOT invoke ground.BuildContext.
// TestResults are structured pass/fail + metric tuples; the grounder
// does not apply. Enforced by write_mode_red_lines_test.go.
//
// Classified ReadOnly + NonEvidenceTool, same family as emit_*.
type EmitTestResults struct {
	ReadOnly
	NonEvidenceTool
}

// Name returns the tool's stable identifier.
func (t *EmitTestResults) Name() string { return "emit_test_results" }

// Description covers intent + stub state.
func (t *EmitTestResults) Description() string {
	return "Emit the structured test-result bundle for the verify stage. " +
		"B0 SKELETON STUB — returns 'not yet implemented'; real implementation ships in B3 " +
		"alongside the run_tests runner and language-specific parsers."
}

// Parameters describes the B3 shape; ignored by the stub Execute.
func (t *EmitTestResults) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "passed": {
      "type": "boolean",
      "description": "Overall verdict: true iff every assertion passed and no metric regressed."
    },
    "results": {
      "type": "array",
      "description": "One entry per assertion.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "assertion_id": {"type": "string"},
          "suite": {"type": "string"},
          "passed": {"type": "boolean"},
          "duration_ms": {"type": "integer"},
          "failure_detail": {"type": "string"}
        },
        "required": ["assertion_id", "suite", "passed"]
      }
    },
    "failure_summary": {
      "type": "string",
      "description": "Human-readable summary when passed=false; empty otherwise."
    }
  },
  "required": ["passed", "results"]
}`)
}

// Execute is the B0 stub: always returns a clear error.
func (t *EmitTestResults) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	_ = ctx
	_ = params
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   false,
		Summary:   "[B0 skeleton] emit_test_results not yet implemented — pairs with run_tests in B3.",
		Timestamp: time.Now(),
	}, fmt.Errorf("emit_test_results: B0 skeleton stub; implementation deferred to B3")
}
