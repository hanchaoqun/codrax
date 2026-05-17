package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
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

	// Required shape signals: abstract contradiction patterns +
	// 5 NOT-contradiction patterns + decision discipline.
	for _, want := range []string{
		"Numeric mismatch",
		"Identity mismatch",
		"Behaviour mismatch",
		"Quantifier mismatch",
		"Direction mismatch",
		"Row order mismatch",
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
	if !strings.Contains(p, "Re-read SUMMARY and BODY at least twice") {
		t.Error("prompt missing 're-read twice' rule")
	}
	if !strings.Contains(p, "rather miss a subtle contradiction than cry wolf") {
		t.Error("prompt missing conservative-floor rule")
	}
	if !strings.Contains(p, "VERBATIM") {
		t.Error("prompt missing verbatim-quote rule")
	}

	// B6-F2 (post-shape consolidated audit, 2026-05-04): the
	// SEMANTIC-NORMALIZATION section MUST be present so the reviewer
	// canonicalizes entity / scope / count expressions before
	// pattern-matching contradiction shapes. The 4 abstract moves
	// (same entity under different names, scope qualifier, same
	// count expressed differently, different facts named the same
	// way) are mandatory — adding a 5th move is fine; removing one
	// would let the qf-... case slip through.
	if !strings.Contains(p, "SEMANTIC-NORMALIZATION") {
		t.Error("B6-F2 prompt missing SEMANTIC-NORMALIZATION header")
	}
	for _, want := range []string{
		"SAME ENTITY UNDER DIFFERENT NAMES",
		"SCOPE QUALIFIER",
		"SAME COUNT EXPRESSED DIFFERENTLY",
		"DIFFERENT FACTS NAMED THE SAME WAY",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("B6-F2 prompt missing normalization move: %q", want)
		}
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
                    "contradiction_kind": "assignment_inversion",
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
	if c.Kind != SelfConsistencyContradictionAssignment {
		t.Errorf("kind = %q, want %q", c.Kind, SelfConsistencyContradictionAssignment)
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

// ── Phase 2 BODY-vs-evidence extension (2026-05-04) ─────────

// TestBuildEvidenceAnchorSet_DedupesAcrossFields pins the
// invariant that anchor_symbol + subject + object all flow into
// the set, deduped first-seen.
func TestBuildEvidenceAnchorSet_DedupesAcrossFields(t *testing.T) {
	evidence := []types.EvidenceItem{
		{AnchorSymbol: "checkCoverage", Subject: "gate.Run", Object: ""},
		{AnchorSymbol: "checkCoverage", Subject: "gate.Run", Object: "DAGClosure"}, // dup symbol+subject
		{AnchorSymbol: "checkBudget", Subject: "  ", Object: ""},
		{AnchorSymbol: "", Subject: "", Object: "criterionResolvable"},
	}
	got := BuildEvidenceAnchorSet(evidence)
	want := []string{"checkCoverage", "gate.Run", "DAGClosure", "checkBudget", "criterionResolvable"}
	if len(got) != len(want) {
		t.Fatalf("len = %d (got=%v), want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q (full=%v)", i, got[i], w, got)
		}
	}
}

// TestBuildEvidenceAnchorSet_CapTruncates pins prompt-budget
// guard: never more than SelfConsistencyAnchorCap entries.
func TestBuildEvidenceAnchorSet_CapTruncates(t *testing.T) {
	evidence := make([]types.EvidenceItem, 200)
	for i := range evidence {
		evidence[i].AnchorSymbol = fmt.Sprintf("sym%03d", i)
	}
	got := BuildEvidenceAnchorSet(evidence)
	if len(got) > SelfConsistencyAnchorCap {
		t.Errorf("len = %d, want <= %d", len(got), SelfConsistencyAnchorCap)
	}
	// First entry MUST be sym000 (first-seen preservation under cap).
	if len(got) > 0 && got[0] != "sym000" {
		t.Errorf("first-seen broken: got[0] = %q, want sym000", got[0])
	}
}

func TestBuildSelfConsistencyAnchorSet_CitationLineIdentifiersWinCap(t *testing.T) {
	mut := types.NewMutableState("q")
	evidence := make([]types.EvidenceItem, SelfConsistencyAnchorCap+10)
	for i := range evidence {
		evidence[i].AnchorSymbol = fmt.Sprintf("sym%03d", i)
	}
	mut.AppendEvidence(evidence)
	ctx := &types.BusContext{
		Mutable: mut,
		ToolResults: []types.ToolResult{{
			ToolName: "read_file",
			Success:  true,
			Summary: "[packages/opencode/src/tool/read.ts: showing lines 14-14 of 338 total]\n" +
				"    14│ const DEFAULT_READ_LIMIT = 2000\n",
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "packages/opencode/src/tool/read.ts", Line: 14}},
	}

	got := buildSelfConsistencyAnchorSet(mut, doc, ctx)
	if len(got) > SelfConsistencyAnchorCap {
		t.Fatalf("anchor set len = %d, want <= %d", len(got), SelfConsistencyAnchorCap)
	}
	if !containsString(got, "DEFAULT_READ_LIMIT") {
		t.Fatalf("citation-backed code identifier must survive broad evidence cap; got %v", got)
	}
}

func TestBuildSelfConsistencyAnchorSet_GraphSymbolBackfillsUnreadCitationLine(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetSearchGraph(&repotypes.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"packages/opencode/src/tool/read.ts": &repotypes.FileInfo{
				RelPath: "packages/opencode/src/tool/read.ts",
				Symbols: []repotypes.Symbol{{
					Name:    "DEFAULT_READ_LIMIT",
					Kind:    "const",
					File:    "packages/opencode/src/tool/read.ts",
					Line:    14,
					EndLine: 14,
				}},
			},
		},
	})
	ctx := &types.BusContext{Mutable: mut}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "packages/opencode/src/tool/read.ts", Line: 14}},
	}

	got := buildSelfConsistencyAnchorSet(mut, doc, ctx)
	if !containsString(got, "DEFAULT_READ_LIMIT") {
		t.Fatalf("graph-backed citation symbol should enter self-consistency anchors; got %v", got)
	}
}

// TestRenderSelfConsistencyUserMessage_OmitsAnchorSectionWhenEmpty
// pins back-compat: pre-Phase-2 callers (empty EvidenceAnchorSet)
// see the rendered message UNCHANGED — no extra anchor section.
func TestRenderSelfConsistencyUserMessage_OmitsAnchorSectionWhenEmpty(t *testing.T) {
	in := SelfConsistencyInput{AnswerSummary: "S", AnswerBody: "B"}
	got := renderSelfConsistencyUserMessage(in)
	if strings.Contains(got, "EVIDENCE ANCHORS") {
		t.Errorf("empty anchor set must NOT render the section; got\n%s", got)
	}
}

// TestRenderSelfConsistencyUserMessage_EmitsAnchorSectionWhenSet
// pins Phase 2 wiring: when caller supplies anchors, the rendered
// message exposes them as a markdown bullet list.
func TestRenderSelfConsistencyUserMessage_EmitsAnchorSectionWhenSet(t *testing.T) {
	in := SelfConsistencyInput{
		AnswerSummary:     "S",
		AnswerBody:        "B",
		EvidenceAnchorSet: []string{"checkCoverage", "gate.Run"},
	}
	got := renderSelfConsistencyUserMessage(in)
	if !strings.Contains(got, "EVIDENCE ANCHORS") {
		t.Error("non-empty anchor set MUST render section header")
	}
	if !strings.Contains(got, "- checkCoverage") || !strings.Contains(got, "- gate.Run") {
		t.Errorf("each anchor MUST appear as bullet; got\n%s", got)
	}
}

// TestSelfConsistencyReviewerPrompt_DocumentsFabricationShape pins
// that the system prompt mentions contradiction shape #7
// (fabricated identifier) so prompt drift breaks tests.
func TestSelfConsistencyReviewerPrompt_DocumentsFabricationShape(t *testing.T) {
	if !strings.Contains(selfConsistencyReviewerSystemPrompt, "Fabricated identifier") {
		t.Error("system prompt missing contradiction shape #7 (Fabricated identifier)")
	}
	if !strings.Contains(selfConsistencyReviewerSystemPrompt, "EVIDENCE ANCHORS") {
		t.Error("system prompt missing EVIDENCE ANCHORS reference")
	}
}
