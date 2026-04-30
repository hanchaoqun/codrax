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

// Reflector is the independent-review split inspired by Reflexion
// (Shinn et al., NeurIPS 2023): when verify fails and retry budget
// remains, a SEPARATE LLM call READS THE FULL DATA — the iteration
// ledger, the verbatim failure summaries, the plan diff — and emits
// ONE observation paragraph describing what it sees.
//
// Crucially this is a SECOND LLM (model-to-model review), NOT a
// system rule. The actor (planner) is one model; the reviewer is
// another model with a different prompt and (often) different
// routing. The system supplies data; both models reason. There is
// NO substring matching, NO failure-kind classification injected
// into the reviewer's input, NO "if X then say Y" prompt language.
//
// Pattern mirrors internal/repl/chitchat.go::ChitchatClassifier:
//   - one direct adapter.Chat call (no agent ReAct loop)
//   - structured emit_* tool with a small JSON schema
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
	//
	// ctx is the cancellation-aware context the orchestrator hands
	// down via BusContext.Ctx. A canceled ctx causes immediate
	// HTTP-socket close on the LLM call so a Ctrl+C between verify
	// failures and retry-plan dispatch unwinds without the reflector
	// burning the cooperative checkpoint window.
	Reflect(ctx context.Context, input ReflectorInput) (string, error)
}

// ReflectorInput bundles every signal the reviewer needs to make
// observations from raw data. All fields are optional (zero-value
// handled); callers populate from Mutable.ChangePlan +
// Mutable.ChangeReport + Mutable.IterationLedger.
//
// Module G upgrade: the reviewer now sees the FULL IterationLedger
// (all prior attempts in this Run, verbatim) so it can comment on
// patterns the system has no business pre-classifying — repeated
// approaches, regressions, plateau detection. The system never
// labels these for the reviewer; the reviewer reads the ledger and
// observes what's true.
type ReflectorInput struct {
	Attempt         int                    // 1-indexed retry attempt number
	OriginalRequest string                 // user's original ask, restated
	PlanSummary     string                 // failed plan's summary text
	TargetPaths     []string               // files the failed plan touched
	FailureSummary  string                 // ChangeReport.FailureSummary verbatim
	FailingTests    []ReflectorFailedTest  // failing-test details (verbatim from runner)
	BuildFailed     bool                   // true if compile/build before tests
	AcceptanceTests []string               // plan.AcceptanceTests
	// IterationLedger is the per-Run history of completed attempts
	// (from Mutable.IterationLedger()). Empty on the first retry
	// (no prior attempts yet). The reviewer reads it verbatim.
	IterationLedger []types.IterationRecord
	// BaselineAvailable is true when a pre-apply baseline test snapshot
	// was captured, false otherwise. The reviewer is never told what
	// to conclude FROM this fact (no "you must NOT say pre-existing"
	// guards) — only that the fact is true. The reviewer is a model;
	// it can reason from the fact.
	BaselineAvailable bool
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
//
// Module G upgrade: the schema asks for OBSERVATIONS, not
// prescriptions. The reviewer is an independent reader of the data,
// not an agent telling the planner what to do. The two fields ask
// "what did you see?" — the planner reads that observation
// alongside the raw data and decides the fix on its own. This is
// the model-vs-model review pattern Devin uses (Coder + Critic),
// stripped of the prescriptive scaffolding that creeps in when the
// system pre-encodes "best-practice fixes".
var reflectorTool = llm.ToolSchema{
	Name:        "emit_failure_observation",
	Description: "Produce one short paragraph of OBSERVATIONS about the failed iteration — what you see in the data. Do not prescribe a fix; the planner reads your observation alongside the raw data and decides for itself.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "observation": {
      "type": "string",
      "description": "2-4 sentences describing what you observe in the data. Cite specific files / functions / lines / test names from the input. Stick to factual statements you can ground in the input (\"the plan modified handler.py at lines X-Y; the failing test test_a calls handler.foo which now returns Z instead of W\"). No prescriptions, no advice phrased as instruction to the planner."
    },
    "uncertainty": {
      "type": "string",
      "description": "Optional. 1 sentence naming what you are NOT sure about — a place where the input data is insufficient to draw a confident observation. Empty when the data is clear."
    }
  },
  "required": ["observation"]
}`),
}

// reflectorSystemPrompt frames the reviewer as an INDEPENDENT
// reader, not a fix-prescriber. There are no "DO NOT" lists for
// specific failure classifications — the reviewer is a model and
// can reason about what's defensible from the data. The only
// behavioural ask is "describe what you observe", which is what an
// honest reviewer does anyway.
const reflectorSystemPrompt = `You are an independent reviewer reading the data from a failed verify→re-plan cycle for a code-modification agent.

The planner emitted a ChangePlan, the apply phase applied it to a git worktree, and the verify phase ran tests. Verify failed. You are NOT the planner — your job is to OBSERVE what the data shows. The planner will read your observation alongside the same data and decide the fix.

How to write a good observation:
- Cite the data. Name specific files, function names, line numbers, test names, and assertion messages from the input. Vague observations ("the test failed because of an issue") are useless.
- Stick to what's defensible from the input. If the data does not let you draw a confident observation, say so in the uncertainty field rather than guessing.
- One paragraph. 2-4 sentences in observation, optionally 1 sentence in uncertainty.
- Do not prescribe a fix. Do not write "the planner should…" or "next attempt must…". Describe what is true; the planner decides what to do.
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
//
// Module G: the reviewer's input is the raw ledger + verbatim
// failure data. There is no per-baseline-state prompt mutation —
// the reviewer is a model and reads BaselineAvailable as a fact in
// its input, no system rule layered on top.
func (r *llmReflector) Reflect(ctx context.Context, in ReflectorInput) (string, error) {
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
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := r.adapter.Chat(ctx, messages, tools, llm.ChatOptions{ToolChoice: "required"})
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
		Observation string `json:"observation"`
		Uncertainty string `json:"uncertainty"`
	}
	if err := json.Unmarshal(call.Params, &parsed); err != nil {
		return "", fmt.Errorf("reflector: unmarshal tool params: %w", err)
	}
	out := assembleObservation(parsed.Observation, parsed.Uncertainty)
	logging.Info("[reflector] attempt=%d observation=%q", in.Attempt, oneLineClamp(out, 200))
	return out, nil
}

// renderReflectorUserMessage assembles the reviewer's input as a
// Markdown blob. Verbatim throughout — the reviewer needs the full
// data to make grounded observations. Pre-Module-G this function
// truncated FailingTests detail to 600 chars and called
// ExtractFailureSignal to "isolate the error-bearing line"; both
// were system-side editorialising that hid context the reviewer
// needed. Now we pass the data as-is and let the reviewer (a model)
// decide what's relevant.
func renderReflectorUserMessage(in ReflectorInput) string {
	var b strings.Builder
	if strings.TrimSpace(in.OriginalRequest) != "" {
		fmt.Fprintf(&b, "## Original user request\n%s\n\n", strings.TrimSpace(in.OriginalRequest))
	}
	if len(in.IterationLedger) > 0 {
		// Full prior-attempt history. Verbatim — RenderIterationLedger
		// already keeps PlanSummary + FailureSummary intact.
		b.WriteString(types.RenderIterationLedger(in.IterationLedger))
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "## Current failed iteration %d\n", in.Attempt)
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
		fmt.Fprintf(&b, "Failure summary (verbatim runner output):\n%s\n\n", in.FailureSummary)
	}
	if len(in.FailingTests) > 0 {
		b.WriteString("Failing tests (verbatim runner output):\n")
		for _, t := range in.FailingTests {
			fmt.Fprintf(&b, "- %s::%s\n", t.Suite, t.AssertionID)
			if strings.TrimSpace(t.Detail) != "" {
				// Detail rendered as indented block — indentation is
				// purely visual, no editing of content.
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
		b.WriteString("\n")
	}
	if !in.BaselineAvailable {
		// State the FACT, no prescription. The reviewer reasons from it.
		b.WriteString("Note: no pre-apply baseline test snapshot was captured for this run.\n")
	}
	return b.String()
}

// assembleObservation joins the reviewer's observation + optional
// uncertainty into a single paragraph the planner consumes via
// PlanningHint. No prescriptive framing — the labels stay neutral
// ("Observation:" / "Uncertainty:") so the planner reads facts, not
// instructions.
func assembleObservation(observation, uncertainty string) string {
	parts := []string{}
	if s := strings.TrimSpace(observation); s != "" {
		parts = append(parts, "Reviewer observation: "+s)
	}
	if s := strings.TrimSpace(uncertainty); s != "" {
		parts = append(parts, "Reviewer uncertainty: "+s)
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
