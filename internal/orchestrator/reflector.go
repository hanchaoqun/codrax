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

	// ReflectFull is the stage 3 superset: one Chat dispatch that
	// can emit BOTH the per-iteration observation AND a reusable
	// FailurePattern (the abstract pitfall to persist into the
	// repo's Failure Taxonomy). Returns a nil pattern when the
	// LLM declined to emit one (the failure was too specific or
	// too unclear to abstract). Stage 3 callers route through
	// here; commit-32-and-earlier callers stay on Reflect for
	// back-compat.
	ReflectFull(ctx context.Context, input ReflectorInput) (*ReflectorOutput, error)
}

// ReflectorOutput bundles both products of one reflector
// dispatch: the per-iteration Observation paragraph (consumed
// via PlanningHint) and an optional reusable Pattern (persisted
// into the per-repo FailureTaxonomy). One Chat call produces
// both via tool_choice=auto + two tools in the schema list.
type ReflectorOutput struct {
	// Observation is the same per-iteration critique Reflect
	// returns. May be empty when the LLM emitted only a Pattern.
	Observation string

	// Pattern is the abstract pitfall the reviewer distilled
	// from the failure (stage 3, Failure Taxonomy). Nil when
	// the failure was too specific / too unclear to abstract,
	// or when the LLM only emitted observation. The store's
	// Append handles dedup + decay; callers persist by passing
	// the pattern to FailureTaxonomyStore.Append.
	Pattern *types.FailurePattern
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

	// TaskSummary is the WriteAnalysisIR.Request.Task.Summary
	// (commit 7 P1-F gap-fix). Lets the reviewer compare the
	// failure observation against what the user actually asked for,
	// not just the plan's claimed work. Empty when the
	// write_analyzer was disabled or did not produce an IR.
	TaskSummary string

	// ExpectedOutcomes carries the LLM-emitted goal-checks from
	// WriteAnalysisIR.Request.ExpectedOutcomes (commit 7 P1-F
	// gap-fix). The reviewer can spot "the failing test addresses
	// outcome X but not outcome Y" patterns. Empty when the IR is
	// absent.
	ExpectedOutcomes []string
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

// failurePatternTool is the stage 3 second-tool schema: in
// addition to the per-iteration observation, the reviewer can
// distill the failure into a reusable abstract pitfall that
// the planner will see on FUTURE plans in this repo. Optional
// — the LLM emits this only when the failure is generalisable
// (a pattern, not a specific test name).
//
// Hard constraints encoded in field descriptions:
//   - name: ≤ 60 chars, abstract not specific (no test names,
//     no file:line)
//   - description: 50-300 chars, declarative + actionable
//   - trigger: 30-200 chars, "when does this fire" precondition
//   - confidence: 0-1, ≥ 0.5 to be persisted (low-confidence
//     drops at the Append validator)
//
// applies_to_kinds + applies_to_paths let the LLM scope the
// pattern when applicable — a "stage II rollback semantics"
// pitfall only makes sense for refactor tasks, not for doc
// edits. Empty = applies to everything (the planner sees it
// regardless of task kind).
var failurePatternTool = llm.ToolSchema{
	Name:        "emit_failure_pattern",
	Description: "Distill the failure into a reusable abstract pitfall the planner should remember on FUTURE plans in this repo. Emit ONLY when the underlying mistake generalises (a pattern, not a specific test name). Skip this tool when the failure is too specific or too unclear to abstract.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "Short label (max 60 chars) the planner sees when scoring relevance. Abstract — describe the KIND of mistake, not the specific instance. ALWAYS in English regardless of the user's request language: the dedup logic uses whitespace-token overlap, so a Chinese name like 共享结构方法添加 would never deduplicate against a future English-named sibling. Bad: \"TestUserAuth fails at line 42\" / \"测试失败\". Good: \"lock-scope drift on new public method\"."
    },
    "description": {
      "type": "string",
      "description": "2-4 sentences (50-300 chars total) describing the pattern: what shape of mistake produces this failure, how to recognise it, why it matters. Optimised for the planner's reading — terse, declarative, actionable."
    },
    "trigger": {
      "type": "string",
      "description": "1-2 sentences (30-200 chars) naming the precondition: when does this pitfall fire? Used as a relevance filter on future plans. Examples: \"when adding a method to a goroutine-shared struct\", \"when widening an interface that downstream tests stub\"."
    },
    "consequence": {
      "type": "string",
      "description": "Optional. 1 sentence naming the failure category (race / panic / assertion drift / build break) that follows when the pitfall is hit. Helps the planner predict downstream cost."
    },
    "example_file": {
      "type": "string",
      "description": "Optional. The repo-relative file path where this instance was observed. Lets operators trace the abstract pattern back to the concrete code."
    },
    "example_line": {
      "type": "integer",
      "description": "Optional. The line number in example_file where the issue was located. Zero/missing when not applicable."
    },
    "confidence": {
      "type": "number",
      "description": "0.0-1.0. How sure are you that this is a real reusable pattern vs a one-off coincidence. Below 0.5 the pattern is dropped at the store; emit only when you'd stake reputation on it being a real trap."
    },
    "applies_to_kinds": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional. Restrict relevance to specific WriteTask kinds: feature / bug_fix / refactor / misc. Empty = applies to all kinds. Use when the pitfall is structurally only meaningful for one task kind."
    },
    "applies_to_paths": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional. Repo-relative path PREFIXES the pattern applies to (e.g. \"internal/auth/\"). Empty = applies regardless of which files the next plan touches."
    }
  },
  "required": ["name", "description", "trigger", "confidence"]
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
- Use the same language as the original user request.

Optionally, you may ALSO emit a second tool call: emit_failure_pattern. Use this to distill the failure into an abstract reusable pitfall — the KIND of mistake, not the specific instance. The planner will see this on FUTURE plans in this repo and try to avoid it. Emit a pattern only when:
- The underlying mistake generalises (other plans in this repo could repeat it).
- You can name it abstractly (no specific test names, no file:line in the name).
- You'd stake reputation on it being a real trap (confidence ≥ 0.5).

Skip emit_failure_pattern when the failure is one-off, too specific, or too unclear to abstract. The default is to skip; only emit when the abstraction is solid.`

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
	out, err := r.ReflectFull(ctx, in)
	if out == nil {
		return "", err
	}
	return out.Observation, err
}

// ReflectFull is the stage 3 superset: one Chat dispatch with
// BOTH the per-iteration emit_failure_observation tool AND the
// reusable emit_failure_pattern tool. tool_choice=auto so the
// LLM can emit either, both, or neither — the observation is
// effectively required (no observation = empty Observation
// field), but the pattern is encouraged-only (skip when the
// failure doesn't generalise).
//
// Failure handling: any unmarshal error on a single tool call
// downgrades that field to zero rather than failing the whole
// dispatch. A useful observation with a malformed pattern emit
// is still better than no reflection at all.
func (r *llmReflector) ReflectFull(ctx context.Context, in ReflectorInput) (*ReflectorOutput, error) {
	if r == nil || r.adapter == nil {
		return nil, nil
	}
	user := renderReflectorUserMessage(in)
	if strings.TrimSpace(user) == "" {
		return nil, fmt.Errorf("reflector: empty input")
	}
	messages := []llm.Message{
		{Role: "system", Content: reflectorSystemPrompt},
		{Role: "user", Content: user},
	}
	// Both tools available; tool_choice=auto so the LLM
	// decides whether to emit the pattern. Pre-stage-3 the
	// schema offered only emit_failure_observation with
	// tool_choice=required; that contract still holds (the
	// LLM almost always emits at least the observation).
	tools := []llm.ToolSchema{reflectorTool, failurePatternTool}
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := r.adapter.Chat(ctx, messages, tools, llm.ChatOptions{ToolChoice: "auto"})
	if err != nil {
		return nil, fmt.Errorf("reflector llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return nil, fmt.Errorf("reflector: LLM returned no tool_call")
	}
	out := &ReflectorOutput{}
	for _, call := range resp.ToolCalls {
		switch call.Name {
		case reflectorTool.Name:
			var parsed struct {
				Observation string `json:"observation"`
				Uncertainty string `json:"uncertainty"`
			}
			if err := json.Unmarshal(call.Params, &parsed); err != nil {
				logging.Warning("[reflector] observation unmarshal failed (skipping): %v", err)
				continue
			}
			out.Observation = assembleObservation(parsed.Observation, parsed.Uncertainty)
		case failurePatternTool.Name:
			pattern, perr := unmarshalFailurePattern(call.Params)
			if perr != nil {
				logging.Warning("[reflector] failure_pattern unmarshal failed (skipping): %v", perr)
				continue
			}
			out.Pattern = pattern
		default:
			logging.Warning("[reflector] unexpected tool %q in response (skipping)", call.Name)
		}
	}
	logging.Info("[reflector] attempt=%d observation=%q pattern=%v",
		in.Attempt, oneLineClamp(out.Observation, 200), out.Pattern != nil)
	return out, nil
}

// unmarshalFailurePattern decodes the LLM's emit_failure_pattern
// arguments + applies emit-time validation. Returns nil pattern
// + non-nil err when the schema is too lax (low confidence,
// missing required fields). Successful parses populate
// FirstSeen/LastSeen/HitCount via the store's Append.
func unmarshalFailurePattern(raw json.RawMessage) (*types.FailurePattern, error) {
	var parsed struct {
		Name           string   `json:"name"`
		Description    string   `json:"description"`
		Trigger        string   `json:"trigger"`
		Consequence    string   `json:"consequence"`
		ExampleFile    string   `json:"example_file"`
		ExampleLine    int      `json:"example_line"`
		Confidence     float64  `json:"confidence"`
		AppliesToKinds []string `json:"applies_to_kinds"`
		AppliesToPaths []string `json:"applies_to_paths"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode emit_failure_pattern: %w", err)
	}
	pattern := &types.FailurePattern{
		Name:           strings.TrimSpace(parsed.Name),
		Description:    strings.TrimSpace(parsed.Description),
		Trigger:        strings.TrimSpace(parsed.Trigger),
		Consequence:    strings.TrimSpace(parsed.Consequence),
		ExampleFile:    strings.TrimSpace(parsed.ExampleFile),
		ExampleLine:    parsed.ExampleLine,
		Confidence:     parsed.Confidence,
		AppliesToKinds: parsed.AppliesToKinds,
		AppliesToPaths: parsed.AppliesToPaths,
	}
	if !pattern.IsValid() {
		return nil, fmt.Errorf("emit_failure_pattern fails validation: name=%q desc-len=%d trig-len=%d conf=%.2f",
			pattern.Name, len(pattern.Description), len(pattern.Trigger), pattern.Confidence)
	}
	if pattern.Confidence < 0.5 {
		return nil, fmt.Errorf("emit_failure_pattern confidence %.2f < 0.5 floor; dropping",
			pattern.Confidence)
	}
	// Defense in depth for the dedup contract: the name must
	// be ASCII (English) so SimilarTo's whitespace-token
	// tokenizer can compute meaningful overlap. The schema
	// description tells the LLM, but a non-compliant emit
	// would silently break dedup and inflate the cache. Drop
	// the pattern instead. Description / Trigger / etc may
	// stay in the user's language — only Name participates
	// in dedup.
	if !isASCII(pattern.Name) {
		return nil, fmt.Errorf("emit_failure_pattern name %q contains non-ASCII characters; SimilarTo dedup requires English-language names regardless of user request language",
			pattern.Name)
	}
	return pattern, nil
}

// isASCII reports whether s contains only 7-bit ASCII bytes.
// Used by unmarshalFailurePattern to enforce the English-name
// contract on emit_failure_pattern (the SimilarTo dedup
// heuristic uses whitespace-token overlap, which only works
// on whitespace-separated languages).
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
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
	if strings.TrimSpace(in.TaskSummary) != "" || len(in.ExpectedOutcomes) > 0 {
		b.WriteString("## Task framing\n\n")
		if s := strings.TrimSpace(in.TaskSummary); s != "" {
			fmt.Fprintf(&b, "Task summary: %s\n", s)
		}
		if len(in.ExpectedOutcomes) > 0 {
			b.WriteString("Expected outcomes:\n")
			for _, o := range in.ExpectedOutcomes {
				fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(o))
			}
		}
		b.WriteString("\n")
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
