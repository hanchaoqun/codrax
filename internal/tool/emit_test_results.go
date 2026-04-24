package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitTestResults is the verify-stage's OPTIONAL narrative
// channel. B1.3 ships run_tests as a deterministic parser that
// installs the structured ChangeReport directly on Mutable; this
// tool exists for the LLM to attach a human-readable
// FailureSummary when the tests failed and the aggregate counts
// alone don't explain the root cause.
//
// Typical flow:
//   1. run_tests Execute parses test runner output and installs
//      a ChangeReport with TestResults[] + Passed + a minimal
//      auto-generated FailureSummary ("3 of 12 tests failed").
//   2. Verifier agent's LLM sees the run_tests ToolResult
//      summary + the plan's AcceptanceTests in its prompt
//      supplement.
//   3. If the LLM determines additional narrative context helps
//      (e.g. "all failures are in the new handler's error path —
//      the edge-case test expected a wrapped error, not the raw
//      one"), it calls emit_test_results with failure_summary
//      populated.
//   4. This tool updates Mutable.ChangeReport.FailureSummary
//      with the LLM's narrative, preserving the deterministic
//      TestResults[] exactly.
//
// The LLM CANNOT override the Passed verdict — that is derived
// deterministically from the parser. Narrative is additive only.
//
// L3 red line: Execute MUST NOT invoke ground.BuildContext. Test
// results are structured verdicts, not citations. Enforced by
// write_mode_red_lines_test.go.
type EmitTestResults struct {
	ReadOnly
	NonEvidenceTool
}

// emitTestResultsParams is the wire shape. FailureSummary is the
// only LLM-authored field; Passed is advisory (tool uses
// Mutable's deterministic value).
type emitTestResultsParams struct {
	FailureSummary string `json:"failure_summary,omitempty"`
	// Passed is the LLM's best guess; we only use it for cross-
	// checking a divergence warning. The authoritative Passed
	// lives in Mutable.ChangeReport from run_tests.
	Passed *bool `json:"passed,omitempty"`
}

// Name returns the stable tool identifier.
func (t *EmitTestResults) Name() string { return "emit_test_results" }

// Description covers the narrative-only scope and the
// authoritative-truth contract.
func (t *EmitTestResults) Description() string {
	return "Add an optional human-readable FailureSummary narrative to the already-installed ChangeReport. " +
		"The deterministic test verdict (Passed + TestResults[]) is set by run_tests; this tool ONLY augments it with prose context."
}

// Parameters JSON schema.
func (t *EmitTestResults) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "failure_summary": {
      "type": "string",
      "description": "Human-readable narrative explaining the test outcome, especially failure root cause. 1-4 sentences. Leave empty when all tests passed — the deterministic aggregate is enough."
    },
    "passed": {
      "type": "boolean",
      "description": "Optional LLM judgment on overall pass. Authoritative value lives in Mutable.ChangeReport from run_tests; this is advisory only for a divergence check."
    }
  }
}`)
}

// Execute attaches the LLM's narrative to the existing
// ChangeReport. Failure paths include:
//   - no run_tests has run yet (no ChangeReport to update)
//   - divergence between LLM's Passed and deterministic Passed
//     (logged as a warning; deterministic wins)
func (t *EmitTestResults) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return errResult(t.Name(), "emit_test_results requires BusContext.Mutable"), nil
	}

	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p emitTestResultsParams
	if err := dec.Decode(&p); err != nil {
		return errResult(t.Name(), fmt.Sprintf("invalid params: %v", err)), err
	}

	report := ctx.Mutable.ChangeReport()
	if report == nil {
		return errResult(t.Name(),
			"emit_test_results rejected: no ChangeReport on Mutable — run_tests must execute before this tool"), nil
	}

	// Divergence check — informational warning only.
	if p.Passed != nil && *p.Passed != report.Passed {
		logging.Warning("[emit_test_results] LLM asserts passed=%v but deterministic report says passed=%v; deterministic wins",
			*p.Passed, report.Passed)
	}

	// Attach narrative. A non-empty LLM summary replaces any
	// auto-generated one from the parser (the LLM had access to
	// the plan's acceptance criteria, so its summary is usually
	// more useful than the raw counts).
	narrative := strings.TrimSpace(p.FailureSummary)
	if narrative != "" {
		report.FailureSummary = narrative
		ctx.Mutable.SetChangeReport(report)
	}

	logging.Info("[emit_test_results] narrative=%d chars, passed=%v", len(narrative), report.Passed)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   fmt.Sprintf("[emit_test_results: narrative added (%d chars); verdict=%v]", len(narrative), report.Passed),
		Timestamp: time.Now(),
	}, nil
}
