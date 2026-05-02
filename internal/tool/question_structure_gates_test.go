package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── G1 pre-complete coverage gate (emit_investigation_complete) ──

// TestG1_PreCompleteCoverageGate_RejectsUnderCoverage pins the s5a
// fix: when the user's question carries a CompletenessObligation
// AND the closure shows ScannedSet > ReadSet, refuse
// emit_investigation_complete with result_kind=resolved.
func TestG1_PreCompleteCoverageGate_RejectsUnderCoverage(t *testing.T) {
	ctx := buildG1Ctx(t,
		&types.RequestModel{
			RawRequest: "list all the X handlers",
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "all the X handlers",
			},
		},
		// Closure: 5 candidates scanned, only 2 read.
		map[string]bool{"a.go": true, "b.go": true, "c.go": true, "d.go": true, "e.go": true},
		map[string]bool{"a.go": true, "b.go": true},
	)
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]interface{}{
		"reason":      "investigation done",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected G1 rejection on under-coverage")
	}
	if !strings.Contains(res.Summary, "candidate file(s) that have NOT been read_file") {
		t.Errorf("rejection should mention unread candidates: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "completeness obligation") {
		t.Errorf("rejection should reference the obligation: %q", res.Summary)
	}
}

// TestG1_PreCompleteCoverageGate_PassesOnAbsence pins that the
// gate skips when result_kind=absence (honest zero — scanning
// without reading is OK to declare absence).
func TestG1_PreCompleteCoverageGate_PassesOnAbsence(t *testing.T) {
	ctx := buildG1Ctx(t,
		&types.RequestModel{
			RawRequest: "list all the X handlers",
			CompletenessObligation: &types.CompletenessObligation{
				Required: true, SourceQuote: "all the X handlers",
			},
		},
		map[string]bool{"a.go": true, "b.go": true},
		map[string]bool{},
	)
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]interface{}{
		"reason":                "no X handlers found in repo",
		"confidence":            "high",
		"result_kind":           "absence",
		"absence_justification": "grep returned only test fixtures, no production handlers exist for X",
	})
	res, _ := tool.Execute(ctx, params)
	// G1 itself doesn't reject absence; downstream gates may but
	// the under-coverage signal is intentionally absence-tolerant.
	if !res.Success && strings.Contains(res.Summary, "candidate file(s) that have NOT been read_file") {
		t.Fatal("G1 must NOT fire on result_kind=absence")
	}
}

// TestG1_PreCompleteCoverageGate_NoObligationSkips pins back-compat:
// pre-Plan-E callers (no QuestionStructure axes) get byte-identical
// behaviour to before the gate.
func TestG1_PreCompleteCoverageGate_NoObligationSkips(t *testing.T) {
	ctx := buildG1Ctx(t,
		&types.RequestModel{RawRequest: "what does X do?"},
		map[string]bool{"a.go": true, "b.go": true},
		map[string]bool{},
	)
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]interface{}{
		"reason":      "found",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, _ := tool.Execute(ctx, params)
	// G1 must skip — but downstream grounding gates may reject for
	// other reasons. We only check the G1 signal is absent.
	if !res.Success && strings.Contains(res.Summary, "candidate file(s) that have NOT been read_file") {
		t.Fatalf("G1 must skip when no obligation; got G1 reject: %q", res.Summary)
	}
}

// TestG1_PreCompleteCoverageGate_FullCoverageSkips pins that when
// every ScannedSet entry is ALSO in ReadSet, the gate passes.
func TestG1_PreCompleteCoverageGate_FullCoverageSkips(t *testing.T) {
	ctx := buildG1Ctx(t,
		&types.RequestModel{
			RawRequest: "list all the X handlers",
			CompletenessObligation: &types.CompletenessObligation{
				Required: true, SourceQuote: "all the X handlers",
			},
		},
		map[string]bool{"a.go": true, "b.go": true},
		map[string]bool{"a.go": true, "b.go": true},
	)
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]interface{}{
		"reason":      "all candidates verified",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, _ := tool.Execute(ctx, params)
	if !res.Success && strings.Contains(res.Summary, "candidate file(s) that have NOT been read_file") {
		t.Fatalf("G1 must pass when ScannedSet ⊆ ReadSet; got %q", res.Summary)
	}
}

// buildG1Ctx is a minimal G1 fixture builder.
func buildG1Ctx(t *testing.T, rm *types.RequestModel, scanned, read map[string]bool) *types.BusContext {
	t.Helper()
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(*rm)
	closure := types.NewEvidenceClosure("/repo")
	closure.SetScannedSet(scanned)
	closure.SetReadSet(read)
	mut.SetEvidenceClosure(closure)
	return &types.BusContext{
		Mutable:    mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: *rm},
	}
}

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
