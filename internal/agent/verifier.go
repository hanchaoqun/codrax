package agent

import (
	"fmt"
	"strings"

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
//
//	(a) calls emit_test_results with narrative
//	(b) returns empty content → soft-stop → ShouldStop fires
//
// Either path ends the loop on iteration 0 or 1.
type verifierEvaluator struct {
	// mu is captured in BuildInitialInstruction. ShouldStop
	// consults Mutable.ChangeReport to decide when the report
	// is complete.
	mu *types.MutableState
}

// BuildInitialInstruction captures Mutable and renders a dynamic
// supplement with up to two sections:
//
//  1. AcceptanceTests — natural-language criteria the plan declared
//     (when non-empty). Informational only; no automated matching.
//
//  2. Baseline failures — present ONLY when the orchestrator captured
//     a pre-apply BaselineReport and that baseline had failing
//     tests. Gives the LLM the data it needs to distinguish a
//     regression (passed in baseline, fails now) from a pre-existing
//     failure (failed in baseline AND now) when drafting its
//     emit_test_results narrative.
//
// Returns "" when the plan has no acceptance tests AND there is no
// baseline with failures — keeping the happy-path prompt clean.
func (e *verifierEvaluator) BuildInitialInstruction(ctx *types.AgentContext, _ *skill.Config) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	e.mu = ctx.Mutable
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return "No ChangePlan installed — verify phase cannot proceed. Return without tool calls."
	}
	// Session-35 invariant: always emit a stage-stating directive so
	// the verifier LLM has an unambiguous anchor even when the plan
	// declares no AcceptanceTests and no baseline captured. The prior
	// "return empty string in the trivial case" shape left the
	// verifier leaning on the raw User Request section, which in
	// write mode carries the user's plan-shaped phrasing ("please
	// generate a plan to fix X") and conflicts with the verifier's
	// actual role (run tests). The User Request section is now
	// suppressed for StageVerify in builder.go, but the positive
	// directive belt + suspenders is worth keeping — the LLM should
	// never need to triangulate the stage intent from the system
	// prompt alone.
	var s strings.Builder
	s.WriteString("## Verify phase\n\n")
	s.WriteString("The plan on Mutable.ChangePlan has been applied to the worktree. " +
		"Your job: call run_tests once to execute the project test suite. The tool " +
		"auto-detects the runner (Go / Node / Python / Rust / Java / Ruby / CMake / " +
		"Meson / Make) from manifests in the worktree root, runs it, parses the output, " +
		"and installs Mutable.ChangeReport. On return the evaluator's ShouldStop fires " +
		"on ChangeReport presence and the stage completes.\n\n" +
		"Do NOT emit_change_plan (that was the plan stage; your role is only to verify). " +
		"Do NOT read files or shell out to construct a diff — the plan is already applied.\n")
	baseline := ctx.Mutable.BaselineReport()
	baselineFailures := collectBaselineFailures(baseline)
	if len(plan.AcceptanceTests) == 0 && len(baselineFailures) == 0 {
		return s.String()
	}
	s.WriteString("\n")
	if len(plan.AcceptanceTests) > 0 {
		s.WriteString("## Plan acceptance criteria\n\n")
		s.WriteString("The plan declared these acceptance tests (natural-language):\n\n")
		for _, a := range plan.AcceptanceTests {
			s.WriteString("- " + a + "\n")
		}
		s.WriteString("\nAfter the run_tests tool has populated Mutable.ChangeReport, " +
			"you may OPTIONALLY call emit_test_results to add a short FailureSummary " +
			"narrative explaining how the actual test outcome relates to these criteria. " +
			"If all tests passed, no narrative is needed — return without calling emit_test_results.")
	}
	if len(baselineFailures) > 0 {
		if s.Len() > 0 {
			s.WriteString("\n\n")
		}
		s.WriteString("## Pre-existing baseline failures\n\n")
		s.WriteString("Before the plan was applied, the following test(s) were ALREADY failing " +
			"(not caused by this change — the snapshot was taken against the unmodified worktree):\n\n")
		const maxShown = 15
		shown := 0
		for _, r := range baselineFailures {
			if shown >= maxShown {
				fmt.Fprintf(&s, "- … (+%d more)\n", len(baselineFailures)-shown)
				break
			}
			if r.Suite != "" {
				fmt.Fprintf(&s, "- %s (%s)\n", r.AssertionID, r.Suite)
			} else {
				fmt.Fprintf(&s, "- %s\n", r.AssertionID)
			}
			shown++
		}
		s.WriteString("\nWhen drafting your emit_test_results narrative, classify each failing test in the " +
			"current ChangeReport as either:\n" +
			"  - REGRESSION: passed in baseline, fails now — this plan caused it\n" +
			"  - PRE-EXISTING: failed in baseline AND fails now — unrelated to this plan\n" +
			"  - (FIXED: failed in baseline, passes now — bonus; mention if relevant)\n\n" +
			"The Passed verdict on Mutable.ChangeReport is authoritative (parser-driven) — do not " +
			"try to override it. Use the narrative to tell the operator what's actually regressing.")
	}
	return s.String()
}

// collectBaselineFailures returns the subset of baseline TestResults
// that failed, preserving order so the LLM sees the same ranking the
// runner produced. Empty / nil baseline returns an empty slice; the
// caller uses len(...) to gate section rendering.
func collectBaselineFailures(baseline *types.ChangeReport) []types.TestResult {
	if baseline == nil || len(baseline.TestResults) == 0 {
		return nil
	}
	out := make([]types.TestResult, 0, len(baseline.TestResults))
	for _, r := range baseline.TestResults {
		if !r.Passed {
			out = append(out, r)
		}
	}
	return out
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
