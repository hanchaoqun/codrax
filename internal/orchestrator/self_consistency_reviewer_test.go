package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

type stubSelfConsistencyAdapter struct {
	resp llm.Response
	err  error
}

func (s *stubSelfConsistencyAdapter) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	return s.resp, s.err
}
func (s *stubSelfConsistencyAdapter) ModelID() string               { return "stub" }
func (s *stubSelfConsistencyAdapter) MaxContextTokens() int         { return 128000 }
func (s *stubSelfConsistencyAdapter) MaxOutputTokens() int          { return 0 }
func (s *stubSelfConsistencyAdapter) RequestTimeout() time.Duration { return 0 }
func (s *stubSelfConsistencyAdapter) RetryMaxAttempts() int         { return 1 }

// TestSelfConsistencyReviewerPrompt_NotOverFittedToS1aCase pins
// the commit-62-followup contract: the system prompt MUST NOT
// carry repo-specific or s1a-test-specific examples that would
// over-fit the reviewer to the very case that motivated the
// feature. The prompt enumerates 6 abstract contradiction shapes
// + 5 NOT-contradiction patterns + decision discipline; no "9 /
// 5" specific numeric example, no gate.Run / read-mode mention.
func TestSelfConsistencyReviewerPrompt_NotOverFittedToS1aCase(t *testing.T) {
	p := selfConsistencyReviewerSystemPrompt

	// Forbidden surface signals (would mean the prompt parrots
	// the s1a case it was inspired by):
	for _, banned := range []string{
		"gate.Run", "read mode", "read-mode", "isWrite",
		"checkCoverage", "checkDAGClosure",
	} {
		if strings.Contains(p, banned) {
			t.Errorf("prompt over-fitted: contains s1a-specific surface %q", banned)
		}
	}

	// Forbidden specific number that mirrored s1a's 9 / 5 split.
	// We allow generic numbers (1-N, item counts) but not the
	// specific 9-vs-5 pair.
	if strings.Contains(p, "9 steps") && strings.Contains(p, "5 steps") {
		t.Error("prompt over-fitted: still contains the s1a-mirroring '9 steps / 5 steps' example")
	}

	// Required shape signals: 6 abstract contradiction patterns +
	// 5 NOT-contradiction patterns + decision discipline.
	for _, want := range []string{
		"Numeric mismatch",
		"Identity mismatch",
		"Behaviour mismatch",
		"Quantifier mismatch",
		"Direction or order mismatch",
		"Assignment inversion",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing abstract contradiction shape: %q", want)
		}
	}
	for _, want := range []string{
		"NOT-CONTRADICTIONS",
		"summarisation, not contradiction",
		"vocabulary, not contradiction",
		"expansion, not contradiction",
		"depth difference",
		"framing, not contradiction",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing NOT-contradiction class: %q", want)
		}
	}
	if !strings.Contains(p, "DECISION DISCIPLINE") {
		t.Error("prompt missing decision discipline section")
	}
	if !strings.Contains(p, "Re-read the relevant sections at least twice") {
		t.Error("prompt missing 're-read twice' rule")
	}
	// Plan-D-grounded-reviewer (2026-05-02): the upgraded prompt now
	// has TWO jobs (internal consistency + grounded fact-check) and
	// a CITATIONS-section gating clause. Pin those load-bearing
	// pieces so a future prompt edit cannot silently regress to
	// internal-only review.
	if !strings.Contains(p, "JOB 1") || !strings.Contains(p, "JOB 2") {
		t.Error("prompt missing two-job split (Job 1 / Job 2)")
	}
	if !strings.Contains(p, "CITATIONS") {
		t.Error("prompt missing CITATIONS section reference")
	}
	if !strings.Contains(p, "GROUNDED fact-check") {
		t.Error("prompt missing GROUNDED fact-check label")
	}
	if !strings.Contains(p, "When CITATIONS section is ABSENT") {
		t.Error("prompt missing back-compat clause for missing CITATIONS")
	}
	if !strings.Contains(p, "rather miss a subtle contradiction than cry wolf") {
		t.Error("prompt missing conservative-floor rule")
	}
	if !strings.Contains(p, "VERBATIM") {
		t.Error("prompt missing verbatim-quote rule")
	}
}

func TestSelfConsistencyReviewer_NilAdapterIsNoOp(t *testing.T) {
	r := NewSelfConsistencyReviewer(nil)
	got, err := r.Review(context.Background(), SelfConsistencyInput{
		AnswerSummary: "foo", AnswerBody: "bar",
	})
	if err != nil {
		t.Errorf("nil adapter should not error; got %v", err)
	}
	if got != nil {
		t.Error("nil adapter should yield nil result")
	}
}

// TestSelfConsistencyReviewer_HappyPathConsistent pins the
// common case: reviewer says consistent=true, no violations.
func TestSelfConsistencyReviewer_HappyPathConsistent(t *testing.T) {
	resp := llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name: selfConsistencyTool.Name,
			Params: json.RawMessage(`{
                "consistent": true,
                "confidence": 0.9,
                "reasoning": "summary and body align on all key claims"
            }`),
		}},
	}
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{resp: resp})
	got, err := r.Review(context.Background(), SelfConsistencyInput{
		OriginalRequest: "what is X",
		AnswerSummary:   "X is foo",
		AnswerBody:      "1. X = foo (file:1)",
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if got == nil || !got.Consistent {
		t.Errorf("expected consistent=true; got %+v", got)
	}
	if len(got.Contradictions) != 0 {
		t.Errorf("consistent verdict should have no contradictions; got %v", got.Contradictions)
	}
}

// TestSelfConsistencyReviewer_ContradictionEmitted pins the s1a-
// style case: read↔write reversal between summary and body.
func TestSelfConsistencyReviewer_ContradictionEmitted(t *testing.T) {
	resp := llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name: selfConsistencyTool.Name,
			Params: json.RawMessage(`{
                "consistent": false,
                "confidence": 0.9,
                "contradictions": [{
                    "topic": "read vs write mode check count",
                    "summary_claim": "write mode runs 9 checks, read mode runs 5",
                    "body_claim": "if !isWrite (read mode) adds checks 4-7, write skips them"
                }],
                "reasoning": "summary inverts the read/write count assignment"
            }`),
		}},
	}
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{resp: resp})
	got, err := r.Review(context.Background(), SelfConsistencyInput{
		OriginalRequest: "list checks",
		AnswerSummary:   "write mode runs 9 checks, read mode runs 5",
		AnswerBody:      "1. if !isWrite (read mode) adds checks 4-7, write skips them",
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if got.Consistent {
		t.Error("expected consistent=false")
	}
	if len(got.Contradictions) != 1 {
		t.Fatalf("expected 1 contradiction; got %d", len(got.Contradictions))
	}
	c := got.Contradictions[0]
	if !strings.Contains(c.Topic, "read") || !strings.Contains(c.Topic, "write") {
		t.Errorf("topic missing read/write framing: %q", c.Topic)
	}
}

// TestSelfConsistencyReviewer_InconsistentNoContradictionsRejected
// pins the cross-check: emit declares inconsistent but lists no
// contradictions = malformed; caller should see error.
func TestSelfConsistencyReviewer_InconsistentNoContradictionsRejected(t *testing.T) {
	resp := llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name: selfConsistencyTool.Name,
			Params: json.RawMessage(`{
                "consistent": false,
                "confidence": 0.9,
                "contradictions": []
            }`),
		}},
	}
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{resp: resp})
	_, err := r.Review(context.Background(), SelfConsistencyInput{
		AnswerSummary: "x", AnswerBody: "y",
	})
	if err == nil {
		t.Error("inconsistent + no contradictions should error")
	}
}

// TestSelfConsistencyReviewer_ChatErrorPropagates pins the
// graceful-degradation contract: LLM error is non-fatal but
// returned to caller for LearningFailure recording.
func TestSelfConsistencyReviewer_ChatErrorPropagates(t *testing.T) {
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{err: errors.New("boom")})
	_, err := r.Review(context.Background(), SelfConsistencyInput{
		AnswerSummary: "x", AnswerBody: "y",
	})
	if err == nil {
		t.Error("Chat error must propagate")
	}
}

// TestSelfConsistencyReviewer_NoToolCallErrors pins the strict
// schema: tool_choice=required + LLM returned no tool call =
// schema violation; caller treats as failure.
func TestSelfConsistencyReviewer_NoToolCallErrors(t *testing.T) {
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{resp: llm.Response{}})
	_, err := r.Review(context.Background(), SelfConsistencyInput{
		AnswerSummary: "x", AnswerBody: "y",
	})
	if err == nil {
		t.Error("missing tool call should error")
	}
}

// TestSelfConsistencyReviewer_OutOfRangeConfidenceRejected pins
// the schema-validity gate: confidence > 1 is an LLM bug; caller
// should see error rather than mis-compare against floor.
func TestSelfConsistencyReviewer_OutOfRangeConfidenceRejected(t *testing.T) {
	resp := llm.Response{
		ToolCalls: []llm.ToolCall{{
			Name: selfConsistencyTool.Name,
			Params: json.RawMessage(`{
                "consistent": true,
                "confidence": 1.5
            }`),
		}},
	}
	r := NewSelfConsistencyReviewer(&stubSelfConsistencyAdapter{resp: resp})
	_, err := r.Review(context.Background(), SelfConsistencyInput{
		AnswerSummary: "x", AnswerBody: "y",
	})
	if err == nil {
		t.Error("confidence > 1 should error")
	}
}

// ── Plan-D grounded reviewer (2026-05-02) ──────────────────────

// TestRenderSelfConsistencyUserMessage_BackCompat_NoCitations pins
// that absent Citations renders no ## CITATIONS section, preserving
// pre-grounded-reviewer back-compat behaviour.
func TestRenderSelfConsistencyUserMessage_BackCompat_NoCitations(t *testing.T) {
	out := renderSelfConsistencyUserMessage(SelfConsistencyInput{
		OriginalRequest: "what does X do?",
		AnswerSummary:   "X does Y",
		AnswerBody:      "1. X does Y",
	})
	if strings.Contains(out, "## CITATIONS") {
		t.Errorf("absent Citations must NOT render CITATIONS section: %q", out)
	}
	if !strings.Contains(out, "## SUMMARY") || !strings.Contains(out, "## BODY") {
		t.Errorf("SUMMARY/BODY sections required: %q", out)
	}
}

// TestRenderSelfConsistencyUserMessage_GroundedRendersCitations pins
// that the CITATIONS section appears with file:line + Quote when the
// caller passes Citations[].
func TestRenderSelfConsistencyUserMessage_GroundedRendersCitations(t *testing.T) {
	out := renderSelfConsistencyUserMessage(SelfConsistencyInput{
		OriginalRequest: "list types",
		AnswerSummary:   "found 3 types",
		AnswerBody:      "TypeA, TypeB, TypeC",
		Citations: []SelfConsistencyCitation{
			{Index: 0, File: "a.go", Line: 10, Quote: "type TypeA struct {"},
			{Index: 1, File: "b.go", Line: 20, Quote: "type TypeB struct {"},
			{Index: 2, File: "c.go", Line: 0, Quote: ""}, // unsourced anchor
		},
	})
	if !strings.Contains(out, "## CITATIONS") {
		t.Errorf("CITATIONS section missing: %q", out)
	}
	if !strings.Contains(out, "[0] a.go:10 — type TypeA struct") {
		t.Errorf("citation 0 missing or malformed: %q", out)
	}
	if !strings.Contains(out, "[1] b.go:20 — type TypeB struct") {
		t.Errorf("citation 1 missing or malformed: %q", out)
	}
	if !strings.Contains(out, "(unsourced; orientation only)") {
		t.Errorf("unsourced anchor must render orientation-only marker: %q", out)
	}
}

// TestBuildSelfConsistencyCitations_ProjectsCitationScope pins the
// per-Scope projection: line+quote → quote text; file → orientation
// note; negative → absence-pattern note; etc.
func TestBuildSelfConsistencyCitations_ProjectsCitationScope(t *testing.T) {
	in := []types.Citation{
		{File: "a.go", Line: 10, Quote: "x := 42"},
		{File: "b.go", Line: 0, Scope: types.ScopeFile},
		{File: "c.go", Line: 0, Scope: types.ScopeNegative, NegativePattern: "ShapeFoo"},
		{File: "d.go", Line: 5, Scope: types.ScopeSection, SectionPath: "init"},
		{File: "", Line: 0}, // empty file → dropped
	}
	out := buildSelfConsistencyCitations(in)
	if len(out) != 4 {
		t.Fatalf("want 4 entries (5 minus empty-file drop); got %d", len(out))
	}
	if out[0].Quote != "x := 42" {
		t.Errorf("line scope quote not preserved: %q", out[0].Quote)
	}
	if !strings.Contains(out[1].Quote, "file-scope") {
		t.Errorf("file scope must render orientation note: %q", out[1].Quote)
	}
	if !strings.Contains(out[2].Quote, "absent: ShapeFoo") {
		t.Errorf("negative scope must render absence pattern: %q", out[2].Quote)
	}
	if !strings.Contains(out[3].Quote, "section: init") {
		t.Errorf("section scope must render section path: %q", out[3].Quote)
	}
}

// TestBuildSelfConsistencyCitations_NilEmpty pins nil-safe
// behaviour for callers without citations.
func TestBuildSelfConsistencyCitations_NilEmpty(t *testing.T) {
	if got := buildSelfConsistencyCitations(nil); got != nil {
		t.Errorf("nil input → got %v, want nil", got)
	}
	if got := buildSelfConsistencyCitations([]types.Citation{}); got != nil {
		t.Errorf("empty input → got %v, want nil", got)
	}
}
