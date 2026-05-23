package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitTestResults is the verify-stage's structured-classification
// channel. run_tests is deterministic — it parses the runner's
// output and installs the structured ChangeReport.TestResults +
// Passed verdict directly. emit_test_results lets the verifier LLM
// add two layers on top:
//
//  1. A human-readable FailureSummary narrative (optional).
//  2. Three classified arrays — regression_assertions /
//     preexisting_assertions / fixed_assertions — that distinguish
//     "this plan caused it" from "predates this plan" so
//     CritNoRegression doesn't flag pre-existing failures as
//     regressions caused by the plan.
//
// Schema contract (T3): the three arrays are REQUIRED keys; empty
// is valid (e.g. all-green run / no baseline) but missing the key
// is a tool-call rejection. This forces the LLM to acknowledge the
// classification step instead of leaving the evaluator to infer.
//
// The LLM CANNOT override the Passed verdict — that is derived
// deterministically from the parser. Narrative + classification
// are additive only.
//
// L3 red line: Execute MUST NOT invoke ground.BuildContext. Test
// results are structured verdicts, not citations. Enforced by
// write_mode_red_lines_test.go.
type EmitTestResults struct {
	ReadOnly
	NonEvidenceTool
}

// emitTestResultsParams is the wire shape. The three classification
// arrays are required (schema-validated). FailureSummary is the
// optional narrative; Passed is advisory (tool uses Mutable's
// deterministic value).
type emitTestResultsParams struct {
	FailureSummary        string   `json:"failure_summary,omitempty"`
	Passed                *bool    `json:"passed,omitempty"`
	RegressionAssertions  []string `json:"regression_assertions"`
	PreexistingAssertions []string `json:"preexisting_assertions"`
	FixedAssertions       []string `json:"fixed_assertions"`
}

// Name returns the stable tool identifier.
func (t *EmitTestResults) Name() string { return "emit_test_results" }

// Description covers the structured-classification + narrative scope
// and the authoritative-truth contract.
func (t *EmitTestResults) Description() string {
	return "Annotate the already-installed ChangeReport with three structured assertion arrays " +
		"(regression / pre-existing / fixed) and an optional FailureSummary narrative. " +
		"The deterministic Passed verdict from run_tests is authoritative; this tool is additive."
}

// Parameters JSON schema. The three arrays are required so the
// validator (BaseAgent.executeTool's DisallowUnknownFields-shape
// behaviour) rejects partial payloads — empty arrays are valid.
func (t *EmitTestResults) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["regression_assertions", "preexisting_assertions", "fixed_assertions"],
  "properties": {
    "failure_summary": {
      "type": "string",
      "description": "Human-readable narrative explaining the test outcome, especially failure root cause. 1-4 sentences. Leave empty when all tests passed."
    },
    "passed": {
      "type": "boolean",
      "description": "Optional LLM judgment on overall pass. Authoritative value lives in Mutable.ChangeReport from run_tests; this is advisory only for a divergence check."
    },
    "regression_assertions": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Test AssertionIDs that PASSED in the baseline but FAIL now — caused by this plan. Empty array when nothing regressed."
    },
    "preexisting_assertions": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Test AssertionIDs that failed in BOTH baseline AND current — predate this plan, out of scope for the verify gate."
    },
    "fixed_assertions": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Test AssertionIDs that failed in baseline BUT PASS now — bonus side-effect of this plan."
    }
  }
}`)
}

// Execute attaches the LLM's narrative + classifications to the
// existing ChangeReport. Failure paths:
//   - no run_tests has run yet (no ChangeReport to update)
//   - divergence between LLM's Passed and deterministic Passed
//     (logged as a warning; deterministic wins)
func (t *EmitTestResults) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return errResult(t.Name(), "emit_test_results requires a writable run context (the orchestrator did not provide one)"), nil
	}

	params = applyStructuredPayloadCompat(t.Name(), params, t.Parameters())

	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p emitTestResultsParams
	if err := dec.Decode(&p); err != nil {
		return failStrictDecodeWithError(t.Name(), time.Now(), err, nil)
	}
	// Schema-level required-key enforcement. JSON unmarshal accepts
	// missing keys as zero-value (nil slice); we want the contract
	// "you sent the array key, even if empty" so the LLM can't ghost
	// the classification step. Strict: if the raw JSON re-parse fails
	// (couldn't happen after the typed decode succeeded for any
	// well-formed object input, but defensive for null/array inputs),
	// reject — falling through silently would let the contract leak.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(params), &raw); err != nil {
		return errResult(t.Name(),
			fmt.Sprintf("emit_test_results rejected: params must be a JSON object: %v", err)), nil
	}
	for _, key := range []string{"regression_assertions", "preexisting_assertions", "fixed_assertions"} {
		if _, present := raw[key]; !present {
			return errResult(t.Name(),
				fmt.Sprintf("emit_test_results rejected: required key %q missing — supply an array (empty [] is valid).", key)), nil
		}
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
	}
	// Structured classification. dedup-and-canonicalise here so
	// downstream evaluators don't have to defend against accidental
	// duplicates from the LLM.
	report.RegressionAssertions = dedupStrings(p.RegressionAssertions)
	report.PreexistingAssertions = dedupStrings(p.PreexistingAssertions)
	report.FixedAssertions = dedupStrings(p.FixedAssertions)
	ctx.Mutable.SetChangeReport(report)

	logging.Info("[emit_test_results] narrative=%d chars, regressions=%d preexisting=%d fixed=%d, passed=%v",
		len(narrative), len(report.RegressionAssertions), len(report.PreexistingAssertions),
		len(report.FixedAssertions), report.Passed)

	return types.ToolResult{
		ToolName: t.Name(),
		Success:  true,
		Summary: fmt.Sprintf("[emit_test_results: narrative=%d chars; regressions=%d preexisting=%d fixed=%d; verdict=%v]",
			len(narrative), len(report.RegressionAssertions), len(report.PreexistingAssertions),
			len(report.FixedAssertions), report.Passed),
		Timestamp: time.Now(),
	}, nil
}

// dedupStrings preserves first-occurrence order while filtering
// duplicates and empty strings. Used by the three classification
// arrays so a chatty LLM that lists "TestA" twice (or includes a
// blank entry from a stray comma) lands a clean canonical slice.
func dedupStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
