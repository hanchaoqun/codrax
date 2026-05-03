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

// AnswerReviewer is the read-mode mirror of Reflector (commit 51
// Gap 3): an INDEPENDENT-review LLM dispatched at the END of a
// successful read-mode Run that had retry hints. It reads the
// final answer + the request + the retry history and may distill
// the difficulty into an abstract reusable pitfall the analyzer
// will see on FUTURE Runs in this repo.
//
// Pattern mirrors orchestrator.Reflector + ChitchatClassifier:
//   - one direct adapter.Chat call (no agent ReAct loop)
//   - structured emit_answer_pattern tool, tool_choice=auto
//   - failure paths return ("", nil) so callers don't break the Run
//
// Routed via providers.yaml :: agents.answer_reviewer when present,
// else inherits the default LLM. Run is opt-in: nil adapter or nil
// store yields a zero-value AnswerReviewer whose ReviewAnswer
// returns (nil, nil) — preserves prior read-mode behaviour byte-
// identically.
type AnswerReviewer interface {
	// ReviewAnswer dispatches one Chat call. Returns a nil pattern
	// when the LLM declined to abstract (the difficulty was too
	// specific or too unclear). Returns ("", err) on any LLM error
	// so callers can fall through to the next Run without losing
	// the answer.
	ReviewAnswer(ctx context.Context, input AnswerReviewerInput) (*types.AnswerPattern, error)
}

// AnswerReviewerInput bundles the read-mode signals the reviewer
// reads. All fields are optional (zero-value handled).
type AnswerReviewerInput struct {
	// OriginalRequest is the user's original ask, restated.
	OriginalRequest string

	// FinalAnswer is the rendered prose the finalizer produced.
	// Truncated to 4 KB before injection so a 50 KB answer can't
	// blow the reviewer's context. The reviewer reads the head;
	// pitfalls are about request shape, not answer length.
	FinalAnswer string

	// Scenario is RequestModel.Scenario from the analyzer's
	// classification (e.g. "explain", "root_cause", "enumerate").
	Scenario string

	// Family is the resolved AnswerSemanticView family for the
	// dispatch (e.g. "role_lookup", "config_precedence",
	// "enumeration"). Replaces the legacy Shape field per
	// docs/migration/answer_shape_retirement.md — patterns now
	// scope by family rather than shape enum.
	Family string

	// RetryEvents lists the {stage, reason} pairs that fired
	// during this Run. Non-empty implies the analyzer / explorer
	// / extractor / finalizer rejected at least one earlier
	// attempt — exactly the signal that distinguishes
	// "interesting Run" from "trivial Run". The reviewer reads
	// these verbatim and decides if there's a pattern worth
	// abstracting.
	RetryEvents []AnswerRetryEvent

	// IterationLedger is the per-Run history of completed
	// attempts. Empty when the Run had no retry hits.
	IterationLedger []types.IterationRecord
}

// AnswerRetryEvent is one entry in the read-mode retry history:
// which stage fired the retry and why (verbatim retry hint).
type AnswerRetryEvent struct {
	Stage  string // "analyze" / "explore" / "extract" / "finalize"
	Reason string // verbatim retry hint or contract-rejection note
}

// answerPatternTool is the structured-output schema for the
// reviewer's emit. Mirror of failurePatternTool with read-mode
// scoping fields (applies_to_scenarios + applies_to_shapes
// instead of applies_to_kinds + applies_to_paths).
var answerPatternTool = llm.ToolSchema{
	Name:        "emit_answer_pattern",
	Description: "Distill the answer-finding difficulty into a reusable abstract pitfall the analyzer should remember on FUTURE requests in this repo. Emit ONLY when the underlying mis-classification or mis-shaping generalises (a pattern, not a single instance). Skip this tool when the difficulty was one-off, request-specific, or too unclear to abstract.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "Short label (max 60 chars) the analyzer sees when scoring relevance. Abstract — describe the KIND of mis-classification or mis-shaping, not the specific request. ALWAYS in English regardless of the user's request language: dedup uses whitespace-token overlap so a Chinese name like 多主题分类不当 would never deduplicate against a future English-named sibling. Bad: \"FooBar question got wrong answer\". Good: \"enumeration on this repo counts interfaces not implementations\"."
    },
    "description": {
      "type": "string",
      "description": "2-4 sentences (50-300 chars total) describing the pattern: what shape of mis-classification or mis-shaping produces this difficulty, how to recognise it, why it matters. Optimised for the analyzer's reading — terse, declarative, actionable."
    },
    "trigger": {
      "type": "string",
      "description": "1-2 sentences (30-200 chars) naming the precondition: when does this pitfall fire? Used as a relevance filter on future requests. Examples: \"when the user asks 'how many X' and X spans 2+ subsystems\", \"when an explain-shape request has a scalar-typed subject\"."
    },
    "consequence": {
      "type": "string",
      "description": "Optional. 1 sentence naming the failure category (coherence-gate fail / shape-subject mismatch / hypothesis-coverage gap / multi-topic under-decomposition) that follows when the pitfall is hit."
    },
    "example_file": {
      "type": "string",
      "description": "Optional. The repo-relative file path where this instance was observed (the file the answer eventually cited as load-bearing). Lets operators trace the abstract pattern back to the concrete code."
    },
    "example_line": {
      "type": "integer",
      "description": "Optional. The line number in example_file where the issue was located. Zero/missing when not applicable."
    },
    "confidence": {
      "type": "number",
      "description": "0.0-1.0. How sure are you that this is a real reusable pattern vs a one-off. Below 0.5 the pattern is dropped at the store; emit only when you'd stake reputation on it being a real trap."
    },
    "applies_to_scenarios": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional. Restrict relevance to specific Scenario values: explain / root_cause / enumerate / count / status / impact / behaviour / generic. Empty = applies to all scenarios."
    },
    "applies_to_families": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional. Restrict relevance to specific AnswerSemanticView family values: role_lookup / config_precedence / enumeration / call_chain / root_cause_trace / architecture / generic. Empty = applies to all families."
    }
  },
  "required": ["name", "description", "trigger", "confidence"]
}`),
}

// answerReviewerSystemPrompt frames the reviewer as an
// independent-reader of the read-mode Run, not a fix-prescriber.
// Mirror of reflectorSystemPrompt with read-mode scoping.
const answerReviewerSystemPrompt = `You are an independent reviewer reading the data from a code-question Run that produced an answer but had to retry one or more pipeline stages along the way.

The pipeline is read-only: it does NOT modify code. It classifies the user's request, explores the repo, structures the evidence, and emits a final answer. Stages can self-reject ("re-analyze, the family doesn't match the subject"; "re-explore, the coverage is too thin"; "re-extract, the required blocks contradict the hypothesis verdict"). When that happens, the orchestrator retries with an updated hint.

Your job: read the original request, the final answer, the Scenario / Family classification, and the retry history. If you see a generalisable pattern in WHY the retries fired — a kind of mis-classification or mis-shaping that is likely to recur on similar requests in this repo — emit one emit_answer_pattern call.

How to write a useful pattern:
- Describe the kind of mistake, not the specific request. "FooBar question got wrong answer" is useless. "enumeration on this repo tends to count interfaces rather than implementations" is reusable.
- Cite the data. Name the Scenario, the Family, the retry stage. Vague observations are useless.
- Stay in English for the Name field — dedup logic uses whitespace tokens.
- Description / Trigger may be in any language but English keeps cross-Run dedup tightest.

When NOT to emit:
- The difficulty was a one-off (a single typo, a transient infrastructure flake).
- The retries were all from the same stage with the same root cause already fixed by the next attempt.
- You are not confident below 0.5 — emit nothing.

The default is to skip. Only emit when the abstraction is solid.`

// llmAnswerReviewer is the default AnswerReviewer implementation.
type llmAnswerReviewer struct {
	adapter llm.Adapter
}

// NewAnswerReviewer builds the default AnswerReviewer. Nil adapter
// yields a reviewer whose ReviewAnswer always returns (nil, nil) —
// effectively disabled. Call sites check this and fall through.
func NewAnswerReviewer(adapter llm.Adapter) AnswerReviewer {
	return &llmAnswerReviewer{adapter: adapter}
}

// ReviewAnswer dispatches one structured-emit Chat call. nil
// adapter / no tool call / malformed JSON returns (nil, err) so
// the caller falls through without breaking the Run. The Run is
// already complete by the time this is invoked — the reviewer is
// strictly informational.
func (r *llmAnswerReviewer) ReviewAnswer(ctx context.Context, in AnswerReviewerInput) (*types.AnswerPattern, error) {
	if r == nil || r.adapter == nil {
		return nil, nil
	}
	user := renderAnswerReviewerUserMessage(in)
	if strings.TrimSpace(user) == "" {
		return nil, fmt.Errorf("answer_reviewer: empty input")
	}
	messages := []llm.Message{
		{Role: "system", Content: answerReviewerSystemPrompt},
		{Role: "user", Content: user},
	}
	tools := []llm.ToolSchema{answerPatternTool}
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := r.adapter.Chat(ctx, messages, tools, llm.ChatOptions{ToolChoice: "auto"})
	if err != nil {
		return nil, fmt.Errorf("answer_reviewer llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		// LLM declined to emit a pattern — the difficulty wasn't
		// generalisable. This is the expected default; not an error.
		return nil, nil
	}
	for _, call := range resp.ToolCalls {
		if call.Name != answerPatternTool.Name {
			logging.Warning("[answer_reviewer] unexpected tool %q in response (skipping)", call.Name)
			continue
		}
		pattern, perr := unmarshalAnswerPattern(call.Params)
		if perr != nil {
			logging.Warning("[answer_reviewer] emit_answer_pattern unmarshal failed: %v", perr)
			return nil, perr
		}
		logging.Info("[answer_reviewer] emitted pattern name=%q conf=%.2f", pattern.Name, pattern.Confidence)
		return pattern, nil
	}
	return nil, nil
}

// unmarshalAnswerPattern decodes the LLM's emit_answer_pattern
// arguments + applies emit-time validation. Returns (nil, err)
// when the schema is too lax (low confidence, missing required
// fields, non-ASCII Name).
func unmarshalAnswerPattern(raw json.RawMessage) (*types.AnswerPattern, error) {
	var parsed struct {
		Name               string   `json:"name"`
		Description        string   `json:"description"`
		Trigger            string   `json:"trigger"`
		Consequence        string   `json:"consequence"`
		ExampleFile        string   `json:"example_file"`
		ExampleLine        int      `json:"example_line"`
		Confidence         float64  `json:"confidence"`
		AppliesToScenarios []string `json:"applies_to_scenarios"`
		AppliesToFamilies  []string `json:"applies_to_families"`
		AppliesToShapes    []string `json:"applies_to_shapes"` // legacy alias for back-compat reads
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode emit_answer_pattern: %w", err)
	}
	families := parsed.AppliesToFamilies
	if len(families) == 0 && len(parsed.AppliesToShapes) > 0 {
		families = parsed.AppliesToShapes
	}
	pattern := &types.AnswerPattern{
		Name:               strings.TrimSpace(parsed.Name),
		Description:        strings.TrimSpace(parsed.Description),
		Trigger:            strings.TrimSpace(parsed.Trigger),
		Consequence:        strings.TrimSpace(parsed.Consequence),
		ExampleFile:        strings.TrimSpace(parsed.ExampleFile),
		ExampleLine:        parsed.ExampleLine,
		Confidence:         parsed.Confidence,
		AppliesToScenarios: parsed.AppliesToScenarios,
		AppliesToShapes:    families,
	}
	if !pattern.IsValid() {
		return nil, fmt.Errorf("emit_answer_pattern fails validation: name=%q desc-len=%d trig-len=%d conf=%.2f",
			pattern.Name, len(pattern.Description), len(pattern.Trigger), pattern.Confidence)
	}
	if pattern.Confidence < 0.5 {
		return nil, fmt.Errorf("emit_answer_pattern confidence %.2f < 0.5 floor; dropping",
			pattern.Confidence)
	}
	if !isASCII(pattern.Name) {
		return nil, fmt.Errorf("emit_answer_pattern name %q contains non-ASCII characters; SimilarTo dedup requires English-language names regardless of user request language",
			pattern.Name)
	}
	return pattern, nil
}

// renderAnswerReviewerUserMessage assembles the reviewer's input
// as a Markdown blob. The truncation is conservative — head of
// the answer (4 KB) so a 50 KB answer doesn't dominate; full
// retry history because it's the load-bearing signal.
func renderAnswerReviewerUserMessage(in AnswerReviewerInput) string {
	var b strings.Builder
	if s := strings.TrimSpace(in.OriginalRequest); s != "" {
		fmt.Fprintf(&b, "## Original user request\n%s\n\n", s)
	}
	if s := strings.TrimSpace(in.Scenario); s != "" {
		fmt.Fprintf(&b, "Scenario classification: %s\n", s)
	}
	if s := strings.TrimSpace(in.Family); s != "" {
		fmt.Fprintf(&b, "Question family: %s\n", s)
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	if s := strings.TrimSpace(in.FinalAnswer); s != "" {
		const maxAnswerBytes = 4096
		if len(s) > maxAnswerBytes {
			s = s[:maxAnswerBytes] + "\n…(truncated)"
		}
		fmt.Fprintf(&b, "## Final answer (head, possibly truncated)\n%s\n\n", s)
	}
	if len(in.RetryEvents) > 0 {
		b.WriteString("## Retry history\n")
		for i, e := range in.RetryEvents {
			fmt.Fprintf(&b, "%d. stage=%s — %s\n", i+1, e.Stage, strings.TrimSpace(e.Reason))
		}
		b.WriteString("\n")
	}
	if len(in.IterationLedger) > 0 {
		b.WriteString(types.RenderIterationLedger(in.IterationLedger))
		b.WriteString("\n")
	}
	return b.String()
}
