package agent

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// verifierEvaluator drives the B1 verify-stage agent. The verify
// workflow is DETERMINISTIC by design — run_tests does the actual
// work (detect runner, execute command, parse output, install
// Mutable.ChangeReport). The LLM's role is optional: it may emit
// emit_test_results to ADD a narrative FailureSummary to the
// already-installed report. If the LLM skips the emit, the
// verifier's ParseOutput auto-promotes Mutable.ChangeReport's
// Passed / FailureSummary from the parser output.
//
// This mirrors the B1 Q2 design where apply agent is a "dumb
// marshaller": the LLM's judgment-bearing work happens in the
// planner stage; apply and verify execute the plan mechanically.
// The LLM at verify may contribute narrative context (e.g.
// summarise which failure mode hit) but is never the source of
// truth for pass/fail verdicts.
//
// Stop condition: Mutable.ChangeReport != nil. run_tests installs
// it synchronously in Execute, so the LLM typically sees a
// populated report on its FIRST turn and either:
//  (a) calls emit_test_results with narrative
//  (b) returns empty content → soft-stop → ShouldStop fires
// Either path ends the loop on iteration 0 or 1.
type verifierEvaluator struct {
	// mu is captured in BuildInitialInstruction. ShouldStop
	// consults Mutable.ChangeReport to decide when the report
	// is complete.
	mu *types.MutableState
}

// BuildInitialInstruction captures Mutable and renders a dynamic
// supplement that surfaces the plan's AcceptanceTests so the LLM
// can gauge whether the run_tests output actually covered them.
// B1.3 does NOT automate that matching (deferred to B3); the prose
// hint is purely informative.
func (e *verifierEvaluator) BuildInitialInstruction(ctx *types.AgentContext, _ *skill.Config) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	e.mu = ctx.Mutable
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return "No ChangePlan installed — verify phase cannot proceed. Return without tool calls."
	}
	if len(plan.AcceptanceTests) == 0 {
		return ""
	}
	// Single-purpose prompt supplement: remind the LLM what the
	// plan's stated acceptance criteria are. The verifier may
	// use this as context when drafting an emit_test_results
	// FailureSummary (narrative-only; no automated matching).
	var s string
	s += "## Plan acceptance criteria\n\n"
	s += "The plan declared these acceptance tests (natural-language):\n\n"
	for _, a := range plan.AcceptanceTests {
		s += "- " + a + "\n"
	}
	s += "\nAfter the run_tests tool has populated Mutable.ChangeReport, " +
		"you may OPTIONALLY call emit_test_results to add a short FailureSummary " +
		"narrative explaining how the actual test outcome relates to these criteria. " +
		"If all tests passed, no narrative is needed — return without calling emit_test_results."
	return s
}

// ShouldStop terminates the ReAct loop as soon as a ChangeReport
// is installed on Mutable. run_tests populates it on first call
// so this is typically iter=0 or iter=1. Defensive cap at iter=5
// prevents runaway loops if the runner is mis-configured.
func (e *verifierEvaluator) ShouldStop(_ llm.Response, iteration int) bool {
	if e.mu == nil {
		return true
	}
	if e.mu.ChangeReport() != nil {
		return true
	}
	return iteration >= 5
}

// ParseOutput reads Mutable.ChangeReport and surfaces the verdict.
// When the report is missing (run_tests never ran or failed
// catastrophically), returns a fail-loud error so the orchestrator
// sets TaskState.LastError.
func (e *verifierEvaluator) ParseOutput(
	ctx *types.AgentContext,
	_ []llm.Message,
	_ []types.ToolResult,
	_ []types.MCPResponse,
) (*StageOutput, error) {
	out := &StageOutput{}
	if ctx == nil || ctx.Mutable == nil {
		out.MissingPiece = types.MissingFacts
		out.Error = "verifier: Mutable is nil"
		return out, fmt.Errorf("%s", out.Error)
	}
	report := ctx.Mutable.ChangeReport()
	if report == nil {
		out.MissingPiece = types.MissingFacts
		out.Error = "verifier: no ChangeReport installed — run_tests did not execute or its output failed to parse"
		return out, fmt.Errorf("%s", out.Error)
	}

	if report.Passed {
		logging.Info("[verifier] all tests passed: %d results", len(report.TestResults))
		out.MissingPiece = types.MissingNone
		return out, nil
	}

	// Failed verify is NOT an agent-level error — it's a structured
	// outcome. The verifier returns MissingFacts + an Error so
	// runVerifyPhase renders the failure for the user. When the
	// orchestrator's verify→plan retry loop is enabled
	// (pipeline_max_verify_retries > 0), runVerifyPhase's caller
	// inspects the error and may dispatch a fresh planner round
	// with PlanningHint seeded from this report; otherwise the Run
	// terminates cleanly with the failure surfaced in LastError.
	failSummary := report.FailureSummary
	if failSummary == "" {
		failSummary = fmt.Sprintf("%d test(s) failed", countFailedResults(report.TestResults))
	}
	out.MissingPiece = types.MissingFacts
	out.Error = "verify failed: " + failSummary
	return out, fmt.Errorf("%s", out.Error)
}

// DetermineMissingPiece reflects ParseOutput's decision.
func (e *verifierEvaluator) DetermineMissingPiece(_ *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output == nil {
		return types.MissingFacts
	}
	return output.MissingPiece
}

// NewVerifierAgent constructs the B1 verify-stage agent.
func NewVerifierAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentVerifier, deps, &verifierEvaluator{})
}

// countFailedResults — helper (duplicated from tool.countFailed to
// avoid cross-package imports just for a 5-line counter).
func countFailedResults(results []types.TestResult) int {
	n := 0
	for _, r := range results {
		if !r.Passed {
			n++
		}
	}
	return n
}
