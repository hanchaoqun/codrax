package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// SelfConsistencyReviewer is an independent reviewer LLM that
// reads the finalizer's answer (summary + body) and reports
// FACTUAL contradictions between the two parts (commit 62).
//
// The reviewer is the read-mode mirror of the
// "model-vs-model review" pattern Devin / Reflexion (Shinn 2023)
// established for write mode (see orchestrator/reflector.go):
// the actor (finalizer) and the reviewer are different model
// dispatches with different prompts. The system supplies the
// data (summary + body strings); both models reason; there is
// no system-side "if X then say Y" classification — the
// reviewer just compares two prose blobs and emits a structured
// verdict.
//
// Why this exists: real eval s1a-20260501-083611 produced an
// answer where the per-item bullets correctly said "if !isWrite
// add 4-7" → read = 9 / write = 5, but the opening summary line
// reversed it ("write 模式 9 项,read 模式 5 项"). Internal prose
// inconsistency; system inputs were correct (verified via finalizer
// user msg #1 in run-1.log). No existing gate (contract.Check /
// runAnswerShapeOracle / grounder / EXPECT_NOT_CONTAINS /
// renderEmitSummary) catches inter-paragraph factual
// contradictions because they all check structural shape or
// per-citation grounding, not prose semantics across paragraphs.
//
// Pattern mirrors orchestrator.Reflector + AnswerReviewer:
//   - one direct adapter.Chat call (no agent ReAct loop)
//   - structured emit_self_consistency_review tool
//   - tool_choice=required so the model cannot return free text
//   - confidence floor (0.8 default) to suppress weak signals
//
// Routed via providers.yaml :: agents.self_consistency_reviewer
// when present, else inherits the default LLM. cheap-model
// routing recommended (the task is plain prose↔prose comparison).
type SelfConsistencyReviewer interface {
	Review(ctx context.Context, in SelfConsistencyInput) (*SelfConsistencyResult, error)
}

// SelfConsistencyInput bundles what the reviewer reads. The
// reviewer has TWO jobs:
//
//  1. INTERNAL consistency (original): detect contradictions
//     BETWEEN AnswerSummary and AnswerBody. Pure prose↔prose.
//
//  2. GROUNDED fact-check (Plan-D-grounded-reviewer 2026-05-02):
//     detect prose claims (specific numbers, type names, verbatim
//     labels, identifier names) that contradict the cited content
//     captured in Citations[]. Catches the 9.5/10 cases (s7a "97
//     files" while actual=101, m2a "Priority float64" while
//     actual=int, s3a English label while user asked in Chinese)
//     where the answer's main conclusion is right but auxiliary
//     metadata drifts.
//
// Citations is OPTIONAL; when empty/nil the reviewer falls back
// to job 1 only (back-compat with pre-grounded-reviewer behaviour).
type SelfConsistencyInput struct {
	OriginalRequest string // user's original question (context)
	AnswerSummary   string // doc.Summary — opening prose
	AnswerBody      string // formatted bullets / steps / symbols

	// Citations carries the cited file:line content the answer's
	// prose CLAIMS to be backed by. The reviewer verifies that
	// concrete numerical / type / label claims in the answer match
	// the cited content. Empty/nil disables job 2 (grounded
	// fact-check); the reviewer then runs internal-consistency only.
	Citations []SelfConsistencyCitation
}

// SelfConsistencyCitation is one entry in the cited-content surface
// the reviewer compares against. The reviewer reads File+Line for
// orientation and Quote (if present) as the ground-truth content.
// When Quote is empty the reviewer treats the citation as
// "anchored but unsourced" and only uses it for orientation.
type SelfConsistencyCitation struct {
	Index int    // 0-based, matches doc.Citations index
	File  string // repo-relative path
	Line  int    // 1-based line number
	Quote string // the cited line's content (truncated to citation_quote_max_chars)
}

// SelfConsistencyResult is the reviewer's verdict.
type SelfConsistencyResult struct {
	// Consistent reports whether the reviewer found NO factual
	// contradictions between summary + body. true = clean (the
	// common case); false = at least one contradiction surfaced
	// in Contradictions.
	Consistent bool

	// Contradictions lists up to 3 specific contradictions the
	// reviewer found. Each entry quotes verbatim from BOTH
	// sides + names the topic. Empty when Consistent=true.
	Contradictions []SelfConsistencyContradiction

	// Confidence is the reviewer's self-rated certainty 0-1.
	// Callers MUST gate on this against a configurable floor
	// (default 0.8) — below the floor the verdict is silently
	// dropped to avoid cried-wolf noise.
	Confidence float64

	// Reasoning is a short ≤300-char rationale the reviewer
	// optionally provides; surfaced in Run-end logs.
	Reasoning string
}

// SelfConsistencyContradiction is one specific inconsistency
// pair. SummaryClaim and BodyClaim are verbatim quotes; the
// caller renders them into the Violation.Detail / Repair so the
// finalizer's rewrite (when triggered) sees concrete text to
// reconcile.
type SelfConsistencyContradiction struct {
	Topic        string // ≤ 60 chars; concise framing
	SummaryClaim string // ≤ 200 chars; verbatim from summary
	BodyClaim    string // ≤ 200 chars; verbatim from body
}

// selfConsistencyTool is the structured-output schema. Local to
// this file because it bypasses the agent framework — same
// pattern as reflectorTool / answerPatternTool.
var selfConsistencyTool = llm.ToolSchema{
	Name:        "emit_self_consistency_review",
	Description: "Emit your verdict on whether the answer summary and answer body contradict each other. Report ONLY contradictions where the same factual claim appears with opposite values across the two parts. Ignore stylistic differences, abstraction-level differences, or omissions.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "consistent": {
      "type": "boolean",
      "description": "true when no factual contradictions found; false ONLY when at least one is named in contradictions[]."
    },
    "contradictions": {
      "type": "array",
      "maxItems": 3,
      "description": "List up to 3 specific contradictions. Empty when consistent=true.",
      "items": {
        "type": "object",
        "properties": {
          "topic": {
            "type": "string",
            "description": "≤ 60 chars; concise framing of the contradiction (e.g. 'read vs write mode check count').",
            "maxLength": 60
          },
          "summary_claim": {
            "type": "string",
            "description": "Verbatim quote from the SUMMARY section asserting one value. ≤ 200 chars.",
            "maxLength": 200
          },
          "body_claim": {
            "type": "string",
            "description": "Verbatim quote from the BODY section asserting the opposite value. ≤ 200 chars.",
            "maxLength": 200
          }
        },
        "required": ["topic", "summary_claim", "body_claim"]
      }
    },
    "confidence": {
      "type": "number",
      "minimum": 0,
      "maximum": 1,
      "description": "Your self-rated certainty 0-1. Be conservative — when in doubt prefer consistent=true with confidence ~0.5; only mark consistent=false with confidence >= 0.8."
    },
    "reasoning": {
      "type": "string",
      "maxLength": 300,
      "description": "Optional 1-2 sentence rationale; surfaced in operator logs."
    }
  },
  "required": ["consistent", "confidence"]
}`),
}

// selfConsistencyReviewerSystemPrompt frames the reviewer as an
// INDEPENDENT comparator with strict scope. Carefully written to
// avoid over-fitting:
//   - 6 abstract contradiction shapes (counts, identity, behaviour,
//     quantifier, direction, assignment-inversion) instead of one
//     specific example mirroring the s1a real-eval case
//   - 5 NOT-contradiction patterns (omission, vocabulary, expansion,
//     abstraction depth, framing)
//   - decision discipline: re-read twice + compatibility check +
//     stake-reputation threshold
//   - explicit confidence floor + ground-truth scope limit
//
// Plan-D-grounded-reviewer (2026-05-02): added a SECOND check job —
// when CITATIONS section is present in the user message, also flag
// concrete claims in summary/body whose value contradicts the cited
// content. Catches "auxiliary metadata drift" (specific numbers,
// type names, verbatim labels) where the main answer is correct
// but a secondary detail is wrong.
const selfConsistencyReviewerSystemPrompt = `You are an INDEPENDENT consistency reviewer. The pipeline produced an answer that you will read in three parts:

  - SUMMARY: an opening prose paragraph (1-N sentences depending on answer shape)
  - BODY: numbered/bulleted detailed steps, often with file:line citations
  - CITATIONS (optional): the verbatim content of the file:line references the answer cites — present when ground-truth fact-checking is needed, absent otherwise

You have TWO independent jobs.

JOB 1 — INTERNAL consistency: do SUMMARY and BODY make CONTRADICTORY FACTUAL CLAIMS about the same thing? A CONTRADICTION is when the same property of the same entity is asserted with INCOMPATIBLE values across the two parts. The values must be truly incompatible — saying "the function does A" in summary and "the function does B" in body is only a contradiction if A and B cannot both be true simultaneously.

Common CONTRADICTION SHAPES for Job 1 (apply the principle, not the surface form):
  1. Numeric mismatch — summary and body assert different exact numbers (count / size / threshold / line / index / etc.) for the same thing
  2. Identity mismatch — summary names entity A as the X, body names entity B as the same X
  3. Behaviour mismatch — summary describes outcome A, body describes outcome B (return-vs-panic, success-vs-failure, sync-vs-async, returns-vs-writes)
  4. Quantifier mismatch — summary says "always / all / every / never", body says "sometimes / some / only when X / under condition Y"
  5. Direction or order mismatch — summary says A→B flow, body says B→A; summary says first/before, body says last/after
  6. Assignment inversion — same set of values is mapped to different categories across the two parts

Common NOT-CONTRADICTIONS (DO NOT REPORT for Job 1):
  - Summary omits a detail body provides → summarisation, not contradiction
  - Summary uses different terminology body explains → vocabulary, not contradiction
  - Body adds context summary did not need → expansion, not contradiction
  - Different abstraction levels of the same fact → depth difference, not contradiction
  - Qualitative-vs-quantitative framings of compatible claims → framing, not contradiction

JOB 2 — GROUNDED fact-check (only when CITATIONS section is present in the user message): scan summary and body for SPECIFIC verifiable claims and check each against the cited content shown in CITATIONS. Specifically, flag a claim as a contradiction when the answer asserts a concrete value that the cited content directly contradicts.

Job-2 contradiction shapes (the same value-pair-mismatch patterns as Job 1, but ANSWER vs CITATIONS instead of SUMMARY vs BODY):
  - Specific number drift — answer says "97 files" / "8 implementations" / "line 387" but cited content shows a different exact value
  - Type/declaration drift — answer says "field X is float64" / "function Y returns bool" but cited content shows a different type / signature
  - Identifier drift — answer says "named ShapeValue" / "called handleX" but cited line names a different identifier in that role
  - Verbatim-label drift — answer renders a structural label in language A while the source / user request uses language B for the same role (e.g. flow-diagram node labels in English while the user asked in Chinese — when the same structural element verbatim appears in cited content as the other language, flag it)

Job-2 NOT-CONTRADICTIONS (DO NOT REPORT):
  - Answer's claim does NOT correspond to anything in CITATIONS (cannot verify either way → not your job to flag)
  - Answer paraphrases or summarises cited content (paraphrase ≠ contradiction)
  - Answer mentions an entity NOT present in any CITATION (unverifiable, not contradiction)

DECISION DISCIPLINE (apply before reporting any contradiction from EITHER job):
  1. Re-read the relevant sections at least twice before deciding
  2. For each candidate contradiction, ask: "would BOTH be defensible under some reasonable reading?" — if yes, NOT a contradiction
  3. For abstraction differences ("the function does X" vs body listing 3 sub-mechanisms of X), the body is just expanding — NOT a contradiction
  4. If you find yourself paraphrasing instead of quoting verbatim, you do not have a real contradiction — quote-or-skip
  5. For Job 2: if the cited Quote text is empty or absent, the citation is "anchored but unsourced" — orientation only, do NOT use it as ground truth

When CITATIONS section is ABSENT in the user message, treat Job 2 as disabled and run Job 1 only — that is the back-compat behaviour.

Output via emit_self_consistency_review:
  - consistent=true is the COMMON case; mark it true unless you can quote VERBATIM the contradiction from BOTH sides (Job 1: summary + body; Job 2: answer + citation)
  - confidence >= 0.8 to report a contradiction; below 0.8 mark consistent=true (rather miss a subtle contradiction than cry wolf)
  - When reporting, quote VERBATIM (no paraphrasing, no translation, no summarising); use the same language as the answer text
  - For Job 2 contradictions, summary_claim quotes the answer's claim (from summary or body, whichever side carries it) and body_claim quotes the verbatim cited content from the named file:line`

// llmSelfConsistencyReviewer is the default impl. nil adapter
// yields a reviewer whose Review always returns (nil, nil) —
// effectively disabled. Caller checks nil before dispatching.
type llmSelfConsistencyReviewer struct {
	adapter llm.Adapter
}

// NewSelfConsistencyReviewer builds the default reviewer. nil
// adapter = disabled (test fixtures + flows that explicitly
// don't want consistency review).
func NewSelfConsistencyReviewer(adapter llm.Adapter) SelfConsistencyReviewer {
	return &llmSelfConsistencyReviewer{adapter: adapter}
}

// Review dispatches one structured-emit Chat call. Failure paths
// (nil adapter, no tool call, malformed JSON, error from Chat)
// return (nil, err) so the caller falls through without
// blocking the user's answer. The reviewer is strictly
// advisory — it must NEVER mask the answer the user sees.
func (r *llmSelfConsistencyReviewer) Review(ctx context.Context, in SelfConsistencyInput) (*SelfConsistencyResult, error) {
	if r == nil || r.adapter == nil {
		return nil, nil
	}
	user := renderSelfConsistencyUserMessage(in)
	if strings.TrimSpace(user) == "" {
		return nil, fmt.Errorf("self_consistency_reviewer: empty input")
	}
	messages := []llm.Message{
		{Role: "system", Content: selfConsistencyReviewerSystemPrompt},
		{Role: "user", Content: user},
	}
	tools := []llm.ToolSchema{selfConsistencyTool}
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := r.adapter.Chat(ctx, messages, tools, llm.ChatOptions{ToolChoice: "required"})
	if err != nil {
		return nil, fmt.Errorf("self_consistency_reviewer llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return nil, fmt.Errorf("self_consistency_reviewer: LLM returned no tool_call")
	}
	for _, call := range resp.ToolCalls {
		if call.Name != selfConsistencyTool.Name {
			logging.Warning("[self_consistency_reviewer] unexpected tool %q (skipping)", call.Name)
			continue
		}
		return unmarshalSelfConsistencyResult(call.Params)
	}
	return nil, fmt.Errorf("self_consistency_reviewer: no matching tool emit")
}

// unmarshalSelfConsistencyResult decodes the LLM's emit args.
// Schema-violating emits (negative confidence, missing required
// fields) return (nil, err); the caller treats this as a
// non-fatal reviewer failure (records LearningFailure).
func unmarshalSelfConsistencyResult(raw json.RawMessage) (*SelfConsistencyResult, error) {
	var parsed struct {
		Consistent     bool    `json:"consistent"`
		Contradictions []struct {
			Topic        string `json:"topic"`
			SummaryClaim string `json:"summary_claim"`
			BodyClaim    string `json:"body_claim"`
		} `json:"contradictions"`
		Confidence float64 `json:"confidence"`
		Reasoning  string  `json:"reasoning"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode emit_self_consistency_review: %w", err)
	}
	if parsed.Confidence < 0 || parsed.Confidence > 1 {
		return nil, fmt.Errorf("self_consistency_reviewer: confidence %.2f out of [0,1]", parsed.Confidence)
	}
	out := &SelfConsistencyResult{
		Consistent: parsed.Consistent,
		Confidence: parsed.Confidence,
		Reasoning:  strings.TrimSpace(parsed.Reasoning),
	}
	for _, c := range parsed.Contradictions {
		topic := strings.TrimSpace(c.Topic)
		summaryClaim := strings.TrimSpace(c.SummaryClaim)
		bodyClaim := strings.TrimSpace(c.BodyClaim)
		// Drop empties — emit-time validation; the LLM may have
		// emitted a placeholder we don't want to surface.
		if topic == "" || summaryClaim == "" || bodyClaim == "" {
			continue
		}
		out.Contradictions = append(out.Contradictions, SelfConsistencyContradiction{
			Topic:        topic,
			SummaryClaim: summaryClaim,
			BodyClaim:    bodyClaim,
		})
	}
	// Cross-check: consistent=false REQUIRES at least one
	// contradiction (otherwise the LLM is being inconsistent
	// itself). Promote to err so caller drops the verdict.
	if !out.Consistent && len(out.Contradictions) == 0 {
		return nil, fmt.Errorf("self_consistency_reviewer: consistent=false but no contradictions named")
	}
	return out, nil
}

// renderSelfConsistencyUserMessage formats the comparison input
// as a labelled markdown blob with explicit SUMMARY / BODY (and
// optional CITATIONS) section headers so the reviewer's
// prompt-instructed examples match the input shape.
//
// CITATIONS is included only when in.Citations is non-empty —
// when absent, the reviewer's Job 2 (grounded fact-check) is
// disabled per the system prompt's back-compat clause. Each
// citation renders as `[i] file:line — Quote` (Quote elided when
// empty so unsourced anchors don't pollute the comparison).
func renderSelfConsistencyUserMessage(in SelfConsistencyInput) string {
	var b strings.Builder
	if s := strings.TrimSpace(in.OriginalRequest); s != "" {
		fmt.Fprintf(&b, "## Original user request (context only — do not verify)\n%s\n\n", s)
	}
	fmt.Fprintf(&b, "## SUMMARY\n%s\n\n", strings.TrimSpace(in.AnswerSummary))
	fmt.Fprintf(&b, "## BODY\n%s\n", strings.TrimSpace(in.AnswerBody))
	if len(in.Citations) > 0 {
		b.WriteString("\n## CITATIONS (cited content for ground-truth fact-check)\n")
		for _, c := range in.Citations {
			loc := strings.TrimSpace(c.File)
			if c.Line > 0 {
				loc = fmt.Sprintf("%s:%d", loc, c.Line)
			}
			quote := strings.TrimSpace(c.Quote)
			if quote == "" {
				fmt.Fprintf(&b, "[%d] %s — (unsourced; orientation only)\n", c.Index, loc)
				continue
			}
			fmt.Fprintf(&b, "[%d] %s — %s\n", c.Index, loc, quote)
		}
	}
	return b.String()
}
