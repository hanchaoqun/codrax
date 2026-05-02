package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── G1 pre-complete coverage gate REVERTED (2026-05-02) ──
//
// G1 was implemented as a structural enforcement at
// emit_investigation_complete time: refuse result_kind=resolved when
// closure.ScannedSet contained files not in ReadSet. The s1a-125430
// real-LLM trace revealed this enforcement was at the wrong
// architectural layer:
//
//   - ScannedSet is the breadth-scan keyword ranker output, which
//     surfaces files by import-count and similarity score. For an
//     EnumerationBoundary-only question like "gate.Run 的 9 项检查",
//     the ranker returned 12+ files including emit_answer_document.go
//     and agent.go (popular but unrelated). G1 forced the explorer
//     to read every one, looping into a 10-minute timeout.
//
//   - The investigation-time signal "should this be read" is hard
//     to extract precisely without per-question candidate tracking
//     (closure-side bookkeeping that doesn't exist).
//
//   - Answer-side enforcement (G2 + G3) is precise: G2 reads the
//     completeness flag directly, G3 does verbatim label substring
//     matching. Both have zero false-positive surface.
//
// The lesson: enforce coverage at the answer side where signals are
// precise, not at the investigation side where they are noisy. G1
// is reverted; coverage discipline now relies on:
//   - skill prompt soft guidance (E-7 explore-skill COVERAGE BEFORE
//     COMPLETION rule)
//   - G2 hard rejection of completeness=lower_bound at emit_answer_symbol
//   - G3 hard rejection of bucket-label omission at emit_answer_document
//
// G1's tests were removed with the implementation. Future option:
// add precise per-question candidate tracking to the closure (files
// the LLM grep'd for the analyzer's PrimaryEntities during this
// run), then a hard structural gate could supersede the soft
// guidance — out of scope for this commit.

// ── G2 completeness floor (emit_answer_symbol) ──

// TestG2_CompletenessFloor_RejectsLowerBoundUnderObligation pins the
// s5a fix at the answer-symbol layer: when the user demanded
// completeness, lower_bound is dishonest — reject.
func TestG2_CompletenessFloor_RejectsLowerBoundUnderObligation(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	mut := types.NewMutableState("list every implementation of LoopController")
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "list every implementation of LoopController",
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "every implementation",
				},
			},
		},
	}
	params, _ := json.Marshal(map[string]interface{}{
		"items": []map[string]interface{}{
			{"name": "X", "file": "a.go", "line": 10, "kind": "function"},
		},
		"completeness": "lower_bound",
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected G2 rejection on completeness=lower_bound under obligation")
	}
	if !strings.Contains(res.Summary, "exhaustive") {
		t.Errorf("rejection should mention exhaustive: %q", res.Summary)
	}
}

// TestG2_CompletenessFloor_AllowsUnknown pins that completeness=
// unknown is the honest fallback.
func TestG2_CompletenessFloor_AllowsUnknown(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	mut := types.NewMutableState("list every X")
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "list every X",
				CompletenessObligation: &types.CompletenessObligation{
					Required: true, SourceQuote: "every X",
				},
			},
		},
	}
	params, _ := json.Marshal(map[string]interface{}{
		"items":        []map[string]interface{}{},
		"completeness": "unknown",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success && strings.Contains(res.Summary, "completeness=lower_bound is not honest") {
		t.Fatal("G2 must NOT reject completeness=unknown — that's the honest fallback")
	}
}

// TestG2_CompletenessFloor_SkipsWithoutObligation pins back-compat.
func TestG2_CompletenessFloor_SkipsWithoutObligation(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	mut := types.NewMutableState("list X")
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{RawRequest: "list X"},
		},
	}
	params, _ := json.Marshal(map[string]interface{}{
		"items": []map[string]interface{}{
			{"name": "X", "file": "a.go", "line": 10, "kind": "function"},
		},
		"completeness": "lower_bound",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success && strings.Contains(res.Summary, "exhaustive") {
		t.Fatalf("G2 must skip without obligation; got %q", res.Summary)
	}
}

// ── G3 bucket-aligned shape (emit_answer_document) ──

// TestG3_BucketAlignment_RejectsMissingLabel pins the m1a fix:
// when buckets[] is non-empty, every Label must appear verbatim
// in summary / steps / symbols. A missing label is hard reject.
func TestG3_BucketAlignment_RejectsMissingLabel(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "list X for Turn A and Turn B separately",
			Buckets: []types.QuestionBucket{
				{Label: "Turn A", Index: 1},
				{Label: "Turn B", Index: 2},
			},
		},
	}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "The first stage emits emit_evidence and emit_investigation_complete. The second stage emits emit_answer_symbol and emit_hypothesis_verdict.",
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected G3 rejection: summary mentions neither 'Turn A' nor 'Turn B'")
	}
	if !strings.Contains(res.Summary, "bucket_alignment") {
		t.Errorf("rejection should reference bucket_alignment: %q", res.Summary)
	}
}

// TestG3_BucketAlignment_PassesWhenLabelsPresent pins that summary
// containing every label verbatim satisfies the gate.
func TestG3_BucketAlignment_PassesWhenLabelsPresent(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "list X for Turn A and Turn B separately",
			Buckets: []types.QuestionBucket{
				{Label: "Turn A", Index: 1},
				{Label: "Turn B", Index: 2},
			},
		},
		AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeExplanation},
	}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "### Turn A\n\nFirst-stage tools.\n\n### Turn B\n\nSecond-stage tools.",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success && strings.Contains(res.Summary, "bucket_alignment") {
		t.Fatalf("G3 must pass when both labels appear verbatim: %q", res.Summary)
	}
}

// TestG3_BucketAlignment_SkipsForSingletonShapes pins that
// value/boolean/config_value/none don't trigger G3 (singleton
// answers can't carry a partition; the deeper drift surfaces
// elsewhere).
func TestG3_BucketAlignment_SkipsForSingletonShapes(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "compare X and Y in summary",
			Buckets: []types.QuestionBucket{
				{Label: "X", Index: 1},
				{Label: "Y", Index: 2},
			},
		},
		AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeValue},
	}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "value",
		"value":   map[string]interface{}{"literal": "42", "citation_ref": -1},
		"summary": "no X mention here.",
	})
	res, _ := tool.Execute(ctx, params)
	// May reject for other reasons (shape mismatch with intent etc),
	// but NOT for bucket_alignment.
	if !res.Success && strings.Contains(res.Summary, "bucket_alignment") {
		t.Fatalf("G3 must skip for ShapeValue: %q", res.Summary)
	}
}

// TestG3_BucketAlignment_SkipsWithoutBuckets pins back-compat.
func TestG3_BucketAlignment_SkipsWithoutBuckets(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := newDocBusCtx("")
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel:   types.RequestModel{RawRequest: "what does X do?"},
		AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeExplanation},
	}
	params := mustDocJSON(t, map[string]interface{}{
		"shape":   "explanation",
		"summary": "X does Y.",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success && strings.Contains(res.Summary, "bucket_alignment") {
		t.Fatalf("G3 must skip without Buckets: %q", res.Summary)
	}
}
