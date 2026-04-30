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

// PlanCritic is the pre-apply review split. After the planner emits
// a ChangePlan and the 11-stage validator accepts it, an independent
// LLM reads the plan and the user's framing (WriteAnalysisIR) to
// surface obvious problems BEFORE apply touches a worktree.
//
// Pattern mirrors Reflector exactly:
//   - one direct adapter.Chat call (no agent ReAct loop)
//   - structured emit_plan_review tool with a small JSON schema
//   - tool_choice=required so the model cannot return free text
//   - failure paths fall back to silent disable (never block apply)
//
// Routed via providers.yaml :: agents.plan_critic when present, or
// the default LLM otherwise — same convention as reflector. The
// critic's output is INFORMATIONAL only: it never auto-rejects a
// plan. The text is rendered for the operator's eyes via /plan show
// (commit 5 will wire that surface). The system never reads the
// critique to make a programmatic decision — that would be the
// system second-guessing the planner, exactly the trap Reflexion
// avoids.
type PlanCritic interface {
	// Review produces a critique paragraph for the operator's eyes.
	// Returns ("", nil) when the critic is disabled (nil adapter);
	// returns ("", err) on any LLM error so the caller can log and
	// continue without blocking the apply path.
	Review(ctx context.Context, input PlanCriticInput) (string, error)
}

// PlanCriticInput bundles every signal the critic needs to make
// observations from the plan. Mirrors ReflectorInput's shape — same
// "raw structured data, no system editorialising" philosophy.
type PlanCriticInput struct {
	// RawRequest is the user's original ask, verbatim. Lets the
	// critic compare what the plan does against what the user
	// asked for.
	RawRequest string

	// TaskKind / Scope / Summary come from WriteAnalysisIR when
	// present. Empty when commit 1's write_analyzer didn't produce
	// an IR (degraded path).
	TaskKind string
	Scope    string
	Summary  string

	// ExpectedOutcomes carries the LLM-emitted goal-checks from
	// WriteAnalysisIR.Request.ExpectedOutcomes. The critic compares
	// the plan against these to spot "the plan doesn't address
	// outcome X" patterns.
	ExpectedOutcomes []string

	// PlanSummary is the planner's own summary text from
	// ChangePlan.Summary. The critic spots inconsistencies between
	// what the planner says it did and what the changes[] entries
	// actually contain.
	PlanSummary string

	// Changes is the list of files the plan modifies. Path + Kind +
	// Rationale only — the critic doesn't need full new_content
	// (that's what the planner already saw; the critic looks for
	// shape-level issues, not line-level).
	Changes []PlanCriticChangeRef
}

// PlanCriticChangeRef is one line in the critic's input — a
// shape-level descriptor of one ChangePlan entry.
type PlanCriticChangeRef struct {
	Path      string
	Kind      string
	Rationale string
}

// planCriticTool is the structured-output schema. The critic emits
// a list of "risks" (each a one-sentence concern) and a confidence
// label. The risks list is informational; the operator (or a future
// /plan show enhancement) reads them. The system does NOT
// auto-reject based on risk count.
var planCriticTool = llm.ToolSchema{
	Name:        "emit_plan_review",
	Description: "Produce a short list of risks you see in the proposed plan, plus a confidence label. The risks are operator-facing observations; do not prescribe fixes — the planner already produced this plan, your job is to surface concerns the operator should consider before approving.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "risks": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Each entry is one short sentence describing a specific concern with the plan: a missing file, an inconsistency between summary and changes, a suspect dependency, a likely-broken caller, a coverage gap. Empty array when the plan looks clean — that is a valid emit. Cap entries at 5; pick the 5 most load-bearing concerns when there are more."
    },
    "confidence": {
      "type": "string",
      "enum": ["high", "medium", "low"],
      "description": "Your confidence in the risks listed. high = each risk is concretely grounded in the plan; medium = at least one risk is a judgement call; low = the plan is unclear and you cannot confidently assess it."
    }
  },
  "required": ["risks", "confidence"]
}`),
}

// planCriticSystemPrompt frames the critic as an independent
// reviewer of code-change proposals. Same neutral tone as the
// reflector's prompt — observe, don't prescribe.
const planCriticSystemPrompt = `You are an independent reviewer of a proposed code-change plan.

The planner produced a ChangePlan that has already passed deterministic validation (dependency closure, syntax check, summary fidelity). Your job is to surface the kind of risks deterministic checks miss: shape-level concerns, alignment with the user's actual ask, suspected gaps in coverage. You are NOT the planner — do not propose alternative plans or rewrites. Your output is a short list of risks the operator should consider before approving.

How to write a good risk list:
- Each risk is one short sentence (≤120 chars). Cite specific paths or rationale text from the input. Vague risks ("the plan might be wrong") are useless — say WHY a specific element is suspect.
- An empty list is the right output when the plan looks clean. Do not invent risks for completeness; honesty matters more than length.
- Only flag what the data shows. If you are unsure whether a concern is real, lower the confidence label rather than over-asserting.
- No prescriptions. Do not write "the planner should…" or "consider doing X". Describe what is suspect; the operator decides.
- Use the same language as the user's request.`

// llmPlanCritic is the default PlanCritic implementation. One Chat
// call per Review, opt-in via providers.yaml :: agents.plan_critic
// or inheriting the default adapter. Nil adapter yields a critic
// whose Review always returns ("", nil) — effectively disabled.
type llmPlanCritic struct {
	adapter llm.Adapter
}

// NewPlanCritic builds the default PlanCritic. Nil adapter yields a
// disabled critic — Review always returns ("", nil). yaml-gated by
// pipeline_plan_critic_enabled in cmd/root.go.
func NewPlanCritic(adapter llm.Adapter) PlanCritic {
	return &llmPlanCritic{adapter: adapter}
}

// Review dispatches one structured-emit Chat call. Failure paths
// (nil adapter, no tool call, malformed JSON) return ("", err) so
// the caller logs WARN and continues. Apply is never blocked.
func (c *llmPlanCritic) Review(ctx context.Context, in PlanCriticInput) (string, error) {
	if c == nil || c.adapter == nil {
		return "", nil
	}
	user := renderPlanCriticUserMessage(in)
	if strings.TrimSpace(user) == "" {
		return "", fmt.Errorf("plan_critic: empty input")
	}
	messages := []llm.Message{
		{Role: "system", Content: planCriticSystemPrompt},
		{Role: "user", Content: user},
	}
	tools := []llm.ToolSchema{planCriticTool}
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := c.adapter.Chat(ctx, messages, tools, llm.ChatOptions{ToolChoice: "required"})
	if err != nil {
		return "", fmt.Errorf("plan_critic llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return "", fmt.Errorf("plan_critic: LLM returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != planCriticTool.Name {
		return "", fmt.Errorf("plan_critic: unexpected tool %q", call.Name)
	}
	var parsed struct {
		Risks      []string `json:"risks"`
		Confidence string   `json:"confidence"`
	}
	if err := json.Unmarshal(call.Params, &parsed); err != nil {
		return "", fmt.Errorf("plan_critic: unmarshal tool params: %w", err)
	}
	out := assemblePlanCritique(parsed.Risks, parsed.Confidence)
	logging.Info("[plan_critic] risks=%d confidence=%q", len(parsed.Risks), parsed.Confidence)
	return out, nil
}

// renderPlanCriticUserMessage assembles the critic's input as a
// Markdown blob — verbatim, mirrors renderReflectorUserMessage.
func renderPlanCriticUserMessage(in PlanCriticInput) string {
	var b strings.Builder
	if strings.TrimSpace(in.RawRequest) != "" {
		b.WriteString("## User request\n\n")
		b.WriteString(strings.TrimSpace(in.RawRequest))
		b.WriteString("\n\n")
	}
	if in.TaskKind != "" || in.Scope != "" || in.Summary != "" {
		b.WriteString("## Task framing\n\n")
		if in.TaskKind != "" {
			fmt.Fprintf(&b, "- kind: %s\n", in.TaskKind)
		}
		if in.Scope != "" {
			fmt.Fprintf(&b, "- scope: %s\n", in.Scope)
		}
		if in.Summary != "" {
			fmt.Fprintf(&b, "- summary: %s\n", in.Summary)
		}
		b.WriteString("\n")
	}
	if len(in.ExpectedOutcomes) > 0 {
		b.WriteString("## Expected outcomes (from task framing)\n\n")
		for _, o := range in.ExpectedOutcomes {
			fmt.Fprintf(&b, "- %s\n", o)
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(in.PlanSummary) != "" {
		b.WriteString("## Plan summary (from the planner)\n\n")
		b.WriteString(strings.TrimSpace(in.PlanSummary))
		b.WriteString("\n\n")
	}
	if len(in.Changes) > 0 {
		b.WriteString("## Plan changes\n\n")
		for _, c := range in.Changes {
			fmt.Fprintf(&b, "- `%s` (%s) — %s\n", c.Path, c.Kind, c.Rationale)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// assemblePlanCritique formats the LLM's structured output for
// downstream rendering / logging.
//
// Low-confidence suppression (commit 10 #7): when the LLM flagged
// itself as confidence=low, the risks are signal-noise; surfacing
// them in /plan show would crowd out high-confidence critiques on
// other plans (the "cried wolf" effect). Drop them. The decision
// is the LLM's — it self-reports "I'm not sure" — and we honor
// that by hiding rather than over-asserting. The Info log line
// in Review still records the call so operators can grep for
// suppressed cases.
func assemblePlanCritique(risks []string, confidence string) string {
	if len(risks) == 0 {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(confidence), "low") {
		return ""
	}
	var b strings.Builder
	for _, r := range risks {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s\n", r)
	}
	if confidence != "" {
		fmt.Fprintf(&b, "(confidence: %s)\n", strings.TrimSpace(confidence))
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildPlanCriticInput wires the critic's input from BusContext.
// Pulls the user request, the WriteAnalysisIR (when commit 1's
// write_analyzer ran), and the active ChangePlan.
func buildPlanCriticInput(busCtx *types.BusContext) PlanCriticInput {
	in := PlanCriticInput{}
	if busCtx == nil {
		return in
	}
	if mu := busCtx.Mutable; mu != nil {
		in.RawRequest = strings.TrimSpace(mu.Objective())
		if ir := mu.WriteAnalysisIR(); ir != nil {
			in.TaskKind = string(ir.Request.Task.Kind)
			in.Scope = string(ir.Request.Task.Scope)
			in.Summary = ir.Request.Task.Summary
			in.ExpectedOutcomes = append(in.ExpectedOutcomes, ir.Request.ExpectedOutcomes...)
		}
		if plan := mu.ChangePlan(); plan != nil {
			in.PlanSummary = plan.Summary
			for _, c := range plan.Changes {
				in.Changes = append(in.Changes, PlanCriticChangeRef{
					Path:      c.Path,
					Kind:      c.Kind,
					Rationale: c.Rationale,
				})
			}
		}
	}
	return in
}
