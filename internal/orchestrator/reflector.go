package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// Reflector is the actor/critic split inspired by Reflexion (Shinn
// et al., NeurIPS 2023): when the verify stage fails and the retry
// budget allows another iteration, a SEPARATE LLM call produces a
// natural-language critique of the failure ("the previous patch
// fixed X but broke Y because Z"). The critique replaces (or
// supplements) the heuristic PlanningHint built by buildRetryHint
// before the planner re-dispatches.
//
// Why this is the load-bearing piece (per Self-Debug + Reflexion):
// the actor (planner) re-deriving the right fix from raw stderr
// keeps falling into the same misinterpretation. A short,
// LLM-curated paragraph that names the root cause AND the corrective
// direction empirically beats both raw-trace dumps and silence.
//
// Pattern mirrors internal/repl/chitchat.go::ChitchatClassifier:
//   - one direct adapter.Chat call (no agent ReAct loop)
//   - structured emit_* tool with a tiny enum-narrowed schema
//   - tool_choice=required so the model cannot return free text
//   - failure paths fall back to the heuristic hint (never block retry)
//
// Routed via providers.yaml :: agents.reflector when present, or the
// default LLM otherwise — same convention as memory_summarizer +
// chitchat_classifier. Run as opt-in: zero adapter = fallback to
// heuristic-only PlanningHint (preserves prior behaviour).
type Reflector interface {
	// Reflect produces a critique paragraph for the planner's next
	// iteration. Inputs come from the failed iteration's structured
	// state. Returns ("", nil) when reflection is disabled (nil
	// adapter); returns ("", err) on any LLM error so callers can
	// fall back to the heuristic hint without losing the retry.
	Reflect(input ReflectorInput) (string, error)
}

// ReflectorInput bundles every signal the critic needs to write a
// useful paragraph. All fields are optional (zero-value handled);
// callers populate from Mutable.ChangePlan + Mutable.ChangeReport.
type ReflectorInput struct {
	Attempt           int                   // 1-indexed retry attempt number
	OriginalRequest   string                // user's original ask, restated
	PlanSummary       string                // failed plan's summary text
	TargetPaths       []string              // files the failed plan touched
	FailureSummary    string                // ChangeReport.FailureSummary
	FailingTests      []ReflectorFailedTest // top failing-test details
	BuildFailed       bool                  // true if compile/build before tests
	AcceptanceTests   []string              // plan.AcceptanceTests
}

// ReflectorFailedTest is the per-test slice handed to the critic.
// Suite + AssertionID + first FailureDetail line are enough; full
// stderr is intentionally NOT included (see Self-Debug 2023: raw
// trace lets the model re-derive the same wrong fix).
type ReflectorFailedTest struct {
	Suite       string
	AssertionID string
	Detail      string // first 1-2 lines of FailureDetail, pre-trimmed
}

// reflectorTool is the structured-output schema. Local to this file
// because it bypasses the agent framework — same pattern as
// chitchatClassifierTool.
var reflectorTool = llm.ToolSchema{
	Name:        "emit_failure_critique",
	Description: "Produce a 2-4 sentence diagnostic critique of the failed iteration that the planner will use as guidance for its next attempt.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "root_cause": {
      "type": "string",
      "description": "1-2 sentences naming the most likely root cause of the failures: which assumption in the previous plan was wrong, or which edge case was overlooked. Do NOT speculate about the test runner / framework — assume tests are correct."
    },
    "corrective_direction": {
      "type": "string",
      "description": "1-2 sentences telling the next planner WHAT TO DO DIFFERENTLY: a specific file, function, or behaviour to revise. Avoid vague encouragement (\"try harder\"); name the concrete change."
    },
    "preserve_what_worked": {
      "type": "string",
      "description": "Optional. 1 sentence acknowledging any aspect of the previous plan that should be KEPT (e.g. correct file targeting, test structure recognised correctly). Empty when nothing salvageable."
    }
  },
  "required": ["root_cause", "corrective_direction"]
}`),
}

const reflectorSystemPrompt = `You are the critic in a verify→re-plan cycle for a code-modification agent.

The planner emitted a ChangePlan, the apply phase applied it to a git worktree, and the verify phase ran tests. Verify failed. Your job: produce a CONCISE diagnostic critique that helps the planner write a better revision on the next attempt.

Hard rules:
- One emit_failure_critique call. No prose, no other tools.
- 2-4 sentences total across root_cause + corrective_direction. Be specific (name files, functions, edge cases). Be brief.
- DO NOT dump raw stderr or copy test output — the planner already sees a structured failure list. Your value is INTERPRETATION, not transcription.
- DO NOT blame the test runner, the framework, or the user. Tests are authoritative; the plan was wrong.
- If a build failed before tests ran, root_cause names the syntax / type / import issue and corrective_direction tells the planner to fix that first.
- Use the same language as the original user request.`

// llmReflector is the default Reflector implementation. One Chat
// call per Reflect, opt-in via providers.yaml :: agents.reflector or
// inheriting the default adapter.
type llmReflector struct {
	adapter llm.Adapter
}

// NewReflector builds the default Reflector. Nil adapter yields a
// reflector whose Reflect always returns ("", nil) — effectively
// disabled. clearForReplan checks this and falls back to the
// heuristic hint when reflection produced nothing.
func NewReflector(adapter llm.Adapter) Reflector {
	return &llmReflector{adapter: adapter}
}

// Reflect dispatches one structured-emit Chat call. Failure paths
// (nil adapter, no tool call, malformed JSON) return ("", err) so
// the caller falls back to the heuristic hint. The retry itself is
// never blocked by reflection failure.
func (r *llmReflector) Reflect(in ReflectorInput) (string, error) {
	if r == nil || r.adapter == nil {
		return "", nil
	}
	user := renderReflectorUserMessage(in)
	if strings.TrimSpace(user) == "" {
		return "", fmt.Errorf("reflector: empty input")
	}
	messages := []llm.Message{
		{Role: "system", Content: reflectorSystemPrompt},
		{Role: "user", Content: user},
	}
	tools := []llm.ToolSchema{reflectorTool}
	resp, err := r.adapter.Chat(messages, tools, llm.ChatOptions{ToolChoice: "required"})
	if err != nil {
		return "", fmt.Errorf("reflector llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return "", fmt.Errorf("reflector: LLM returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != reflectorTool.Name {
		return "", fmt.Errorf("reflector: unexpected tool %q", call.Name)
	}
	var parsed struct {
		RootCause           string `json:"root_cause"`
		CorrectiveDirection string `json:"corrective_direction"`
		PreserveWhatWorked  string `json:"preserve_what_worked"`
	}
	if err := json.Unmarshal(call.Params, &parsed); err != nil {
		return "", fmt.Errorf("reflector: unmarshal tool params: %w", err)
	}
	out := assembleCritique(parsed.RootCause, parsed.CorrectiveDirection, parsed.PreserveWhatWorked)
	logging.Info("[reflector] attempt=%d critique=%q", in.Attempt, oneLineClamp(out, 200))
	return out, nil
}

// renderReflectorUserMessage assembles the structured input as a
// short Markdown blob. Caller-side caps + trimming live here so the
// schema's `description` fields stay accurate (we promise the model
// 2-4 sentences; we owe it a small input).
func renderReflectorUserMessage(in ReflectorInput) string {
	var b strings.Builder
	if strings.TrimSpace(in.OriginalRequest) != "" {
		fmt.Fprintf(&b, "## Original user request\n%s\n\n", strings.TrimSpace(in.OriginalRequest))
	}
	fmt.Fprintf(&b, "## Failed iteration %d\n", in.Attempt)
	if strings.TrimSpace(in.PlanSummary) != "" {
		fmt.Fprintf(&b, "Plan summary: %s\n\n", strings.TrimSpace(in.PlanSummary))
	}
	if len(in.TargetPaths) > 0 {
		fmt.Fprintf(&b, "Files modified: %s\n\n", strings.Join(in.TargetPaths, ", "))
	}
	if in.BuildFailed {
		b.WriteString("Build failed BEFORE tests ran (compile / type / import error).\n\n")
	}
	if strings.TrimSpace(in.FailureSummary) != "" {
		fmt.Fprintf(&b, "Failure summary: %s\n\n", strings.TrimSpace(in.FailureSummary))
	}
	if len(in.FailingTests) > 0 {
		b.WriteString("Failing tests (top entries; runner stderr extract):\n")
		for _, t := range in.FailingTests {
			fmt.Fprintf(&b, "- %s::%s\n", t.Suite, t.AssertionID)
			if strings.TrimSpace(t.Detail) != "" {
				// Multi-line detail rendered as an indented block so
				// the model sees the assertion + expected/actual rows
				// together. The signal extractor already stripped
				// fixture noise — what arrives here is high-density.
				for _, line := range strings.Split(t.Detail, "\n") {
					fmt.Fprintf(&b, "    %s\n", strings.TrimRight(line, " \t"))
				}
			}
		}
		b.WriteString("\n")
	}
	if len(in.AcceptanceTests) > 0 {
		b.WriteString("Acceptance criteria the plan was supposed to satisfy:\n")
		for _, a := range in.AcceptanceTests {
			fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(a))
		}
	}
	return b.String()
}

func assembleCritique(rootCause, correctiveDirection, preserveWhatWorked string) string {
	parts := []string{}
	if s := strings.TrimSpace(rootCause); s != "" {
		parts = append(parts, "Root cause: "+s)
	}
	if s := strings.TrimSpace(correctiveDirection); s != "" {
		parts = append(parts, "Next attempt: "+s)
	}
	if s := strings.TrimSpace(preserveWhatWorked); s != "" {
		parts = append(parts, "Preserve: "+s)
	}
	return strings.Join(parts, " ")
}

// oneLineClamp mirrors the helper in internal/repl/chitchat.go but
// duplicated here to keep the orchestrator package's import surface
// minimal (no need to depend on internal/repl just for one helper).
func oneLineClamp(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	if n > 0 {
		runes := []rune(s)
		if len(runes) > n {
			return string(runes[:n]) + "…"
		}
	}
	return s
}
