package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// AcceptanceChecker is the per-phase LLM verdict surface in
// stage II's runPhaseGroup. After a phase's verify passes, this
// reviewer reads the phase goal + plan summary + test report
// and emits a (passed, reasoning, next_hint) judgement. Distinct
// from CritTestsPass / CritNoRegression: tests passing means
// "no test regressed", not "this phase achieved its stated
// goal" — a phase could leave the test suite green while doing
// something completely different from what it was supposed to.
//
// Pattern mirrors PlanCritic + Reflector exactly: one direct
// adapter.Chat call (no agent ReAct loop), structured emit_*
// tool with a small JSON schema, tool_choice=required, failure
// paths return (nil, err) so the caller logs and degrades to
// auto-accept (don't block phase advancement on a check that
// itself misbehaved).
//
// Routed via providers.yaml :: agents.acceptance_checker when
// present, or the default LLM otherwise. Cheap-model routing
// recommended; the input is bounded (phase goal + plan summary
// + failure-test list).
type AcceptanceChecker interface {
	// Check returns the LLM's verdict, or (nil, err) on any
	// failure. nil result is also returned when the adapter is
	// nil (effectively disabled — caller treats as auto-accept).
	Check(ctx context.Context, in AcceptanceCheckInput) (*types.AcceptanceCheck, error)
}

// AcceptanceCheckInput bundles the per-phase data the reviewer
// needs. All fields optional (zero-value handled); the reviewer
// renders whatever is present.
type AcceptanceCheckInput struct {
	// PhaseGoal is the per-phase objective string from
	// PhaseRecord.Goal.
	PhaseGoal string

	// PlanSummary is ChangePlan.Summary verbatim — what the
	// planner said it did this phase.
	PlanSummary string

	// TestReport is the verify-stage outcome for this phase.
	// Failures (when present) are surfaced verbatim so the
	// reviewer can spot "tests passed but the wrong tests"
	// patterns.
	TestReport *types.ChangeReport

	// Outcomes carries WriteAnalysisIR.Request.ExpectedOutcomes
	// — the overall task's expected outcomes. Lets the reviewer
	// reason about whether THIS phase contributed toward at
	// least one of them.
	Outcomes []string

	// Attempt is the 1-indexed retry attempt number for this
	// phase. Lets the reviewer notice "second attempt at the
	// same phase" patterns. 1 on the first dispatch.
	Attempt int

	// PhaseIndex is the 1-indexed phase number within the group
	// (1, 2, ... PhaseTotal). Distinct from Attempt: Attempt
	// counts retries of the SAME phase; PhaseIndex says which
	// phase we are reviewing. Letting the reviewer see both
	// prevents the "second attempt at the same phase" /
	// "first attempt at the second phase" conflation.
	PhaseIndex int

	// PhaseTotal is the total number of phases in the group, so
	// the reviewer can frame "this is phase 2 of 3, not the
	// final phase — partial completion is acceptable here".
	PhaseTotal int
}

// acceptanceTool is the structured-output schema. Three fields:
// passed (verdict), reasoning (1-2 sentence justification),
// next_hint (optional carry-over to the next phase's planner).
var acceptanceTool = llm.ToolSchema{
	Name:        "emit_phase_acceptance",
	Description: "Verdict on whether the just-finished phase satisfied its stated goal. Pass when the plan summary describes work that addresses the phase goal AND tests passed without obvious wrong-direction symptoms.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "passed": {
      "type": "boolean",
      "description": "true when this phase satisfied its goal; false when the work was unrelated, contradicted earlier phases, or tests show the phase took the wrong direction."
    },
    "reasoning": {
      "type": "string",
      "description": "1-2 sentences justifying the verdict. Cite specific evidence — name a path from the plan summary, a failing test, an expected outcome. Vague reasoning ('looks fine' / 'something is off') is useless."
    },
    "next_hint": {
      "type": "string",
      "description": "Optional 1-sentence carry-over to the NEXT phase's planner: what this phase established that the next should know about. Empty when no carry-over is useful."
    }
  },
  "required": ["passed", "reasoning"]
}`),
}

// acceptanceSystemPrompt frames the reviewer as an independent
// per-phase judge. Same neutral tone as the reflector + plan_critic
// prompts. No internal scaffolding terms.
const acceptanceSystemPrompt = `You are an independent reviewer judging whether a multi-phase code-modification task's most-recent phase satisfied its stated goal.

You will see:
- The phase's goal text (what this phase was supposed to do)
- A summary of the plan that was applied
- The test report (passed / failed counts; failure details when any)
- The overall task's expected outcomes (each phase should contribute toward at least one)

Decide whether THIS phase contributed toward what it was asked to do. A phase passes acceptance when:
- The plan summary describes work that addresses the phase goal
- AND the test outcome doesn't suggest the phase took the wrong direction (a regression in code the phase touched is a strong "fail" signal)

A phase fails acceptance when:
- The plan summary clearly describes work unrelated to the goal
- OR test failures point at code the phase modified, suggesting wrong direction
- OR the phase's work appears to undo or contradict an earlier phase

Optional next_hint: when the next phase's planner would benefit from knowing what state this phase left the worktree in (e.g. "users.email column added; ORM still references old name"), include 1 sentence. Empty when no useful carry-over.

Respond via emit_phase_acceptance. Keep reasoning concrete; cite the plan summary or test names verbatim.`

// llmAcceptanceChecker is the default AcceptanceChecker. One
// Chat call per Check; nil adapter yields a checker whose
// Check always returns (nil, nil) — effectively disabled
// (caller treats as auto-accept).
type llmAcceptanceChecker struct {
	adapter llm.Adapter
}

// NewAcceptanceChecker builds the default checker. Nil adapter
// returns a checker whose Check always returns (nil, nil).
// Wired from cmd/root.go using providers.yaml ::
// agents.acceptance_checker (fallback to default LLM).
func NewAcceptanceChecker(adapter llm.Adapter) AcceptanceChecker {
	return &llmAcceptanceChecker{adapter: adapter}
}

// Check dispatches one structured-emit Chat call. Failure paths
// (nil adapter, no tool call, malformed JSON) return (nil, err)
// so the caller logs WARN and treats as auto-accept. Phase
// advancement is never blocked by acceptance-check plumbing.
func (a *llmAcceptanceChecker) Check(ctx context.Context, in AcceptanceCheckInput) (*types.AcceptanceCheck, error) {
	if a == nil || a.adapter == nil {
		return nil, nil
	}
	user := renderAcceptanceUserMessage(in)
	if strings.TrimSpace(user) == "" {
		return nil, fmt.Errorf("acceptance_check: empty input")
	}
	messages := []llm.Message{
		{Role: "system", Content: acceptanceSystemPrompt},
		{Role: "user", Content: user},
	}
	tools := []llm.ToolSchema{acceptanceTool}
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := a.adapter.Chat(ctx, messages, tools, llm.ChatOptions{ToolChoice: "required"})
	if err != nil {
		return nil, fmt.Errorf("acceptance_check llm: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return nil, fmt.Errorf("acceptance_check: LLM returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != acceptanceTool.Name {
		return nil, fmt.Errorf("acceptance_check: unexpected tool %q", call.Name)
	}
	var parsed struct {
		Passed    bool   `json:"passed"`
		Reasoning string `json:"reasoning"`
		NextHint  string `json:"next_hint"`
	}
	if err := json.Unmarshal(call.Params, &parsed); err != nil {
		return nil, fmt.Errorf("acceptance_check: unmarshal: %w", err)
	}
	logging.Info("[acceptance_check] phase-attempt=%d passed=%v reasoning=%q",
		in.Attempt, parsed.Passed, oneLineClamp(parsed.Reasoning, 120))
	return &types.AcceptanceCheck{
		Passed:    parsed.Passed,
		Reasoning: strings.TrimSpace(parsed.Reasoning),
		NextHint:  strings.TrimSpace(parsed.NextHint),
	}, nil
}

// renderAcceptanceUserMessage assembles the reviewer's input as
// a Markdown blob — phase header (X of Y) + goal + plan summary
// + outcomes + test report. Verbatim throughout (no system
// editorialising) to match the reflector / plan_critic pattern.
func renderAcceptanceUserMessage(in AcceptanceCheckInput) string {
	var b strings.Builder
	if in.PhaseIndex > 0 && in.PhaseTotal > 0 {
		fmt.Fprintf(&b, "## Reviewing phase %d of %d (attempt %d)\n\n",
			in.PhaseIndex, in.PhaseTotal, in.Attempt)
	}
	if strings.TrimSpace(in.PhaseGoal) != "" {
		b.WriteString("## Phase goal\n\n")
		b.WriteString(strings.TrimSpace(in.PhaseGoal))
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(in.PlanSummary) != "" {
		b.WriteString("## Plan summary (from the planner)\n\n")
		b.WriteString(strings.TrimSpace(in.PlanSummary))
		b.WriteString("\n\n")
	}
	if len(in.Outcomes) > 0 {
		b.WriteString("## Overall task expected outcomes\n\n")
		for _, o := range in.Outcomes {
			fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(o))
		}
		b.WriteString("\n")
	}
	if in.TestReport != nil {
		fmt.Fprintf(&b, "## Test report\n\nPassed=%v Tests=%d\n",
			in.TestReport.Passed, len(in.TestReport.TestResults))
		failed := 0
		for _, tr := range in.TestReport.TestResults {
			if !tr.Passed {
				failed++
			}
		}
		if failed > 0 {
			fmt.Fprintf(&b, "Failures (%d):\n", failed)
			for _, tr := range in.TestReport.TestResults {
				if tr.Passed {
					continue
				}
				detail := strings.TrimSpace(tr.FailureDetail)
				if len(detail) > 200 {
					detail = detail[:200] + "..."
				}
				fmt.Fprintf(&b, "- %s::%s — %s\n",
					strings.TrimSpace(tr.Suite),
					strings.TrimSpace(tr.AssertionID),
					oneLineClamp(detail, 160))
			}
		}
		if strings.TrimSpace(in.TestReport.FailureSummary) != "" {
			fmt.Fprintf(&b, "\nFailure summary (verbatim runner output):\n%s\n",
				strings.TrimSpace(in.TestReport.FailureSummary))
		}
	}
	return b.String()
}
