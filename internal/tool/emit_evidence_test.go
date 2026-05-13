package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func newEmitCtx() *types.BusContext {
	return &types.BusContext{Mutable: types.NewMutableState("")}
}

func seedReadFileHistory(ctx *types.BusContext, file string, startLine int, lines ...string) {
	if ctx == nil || ctx.Mutable == nil || len(lines) == 0 {
		return
	}
	endLine := startLine + len(lines) - 1
	var b strings.Builder
	fmt.Fprintf(&b, "[%s: showing lines %d-%d of 9999]\n", file, startLine, endLine)
	for i, line := range lines {
		fmt.Fprintf(&b, "  %d│ %s\n", startLine+i, line)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "read_file",
			Success:  true,
			Summary:  b.String(),
		}},
	})
}

func TestEmitEvidence_AcceptsValidBatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "registration", "subject": "Foo", "object": "bar", "source": "internal/agent/foo.go", "line_start": 12, "summary": "Foo registers bar", "anchor_kind": "call", "anchor_symbol": "Register"},
          {"kind": "direct", "subject": "isOK", "source": "internal/agent/foo.go", "line_start": 30, "summary": "isOK returns true", "anchor_kind": "definition", "anchor_symbol": "isOK"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("want 2 items in buffer, got %d", len(got))
	}
	for _, it := range got {
		if it.Producer != EmitEvidenceProducer {
			t.Errorf("producer = %q, want %q", it.Producer, EmitEvidenceProducer)
		}
		if it.ID == "" {
			t.Errorf("missing stable ID")
		}
		if it.LineEnd == 0 {
			t.Errorf("LineEnd should default to LineStart")
		}
	}
	if got[0].Kind != types.EvidenceRegistration {
		t.Errorf("kind = %q", got[0].Kind)
	}
	if got[0].Predicate != "registration" {
		t.Errorf("predicate default failed: %q", got[0].Predicate)
	}
}

func TestEmitEvidence_AcceptsInitializerAnchorKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/orchestrator/orchestrator.go", 6362,
		"CitationReq: types.CitationReq{Required: false},",
	)
	params := json.RawMessage(`{
        "items": [
          {
            "kind": "direct",
            "subject": "CitationReq.Required",
            "object": "false",
            "source": "internal/orchestrator/orchestrator.go",
            "line_start": 6362,
            "summary": "CitationReq.Required is initialized to false",
            "snippet": "CitationReq: types.CitationReq{Required: false},",
            "anchor_kind": "initializer",
            "anchor_symbol": "CitationReq.Required"
          }
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].AnchorKind != types.AnchorInitializer {
		t.Fatalf("anchor kind = %s, want initializer", got[0].AnchorKind)
	}
	if form := types.ClaimFormOf(got[0]); form != types.ClaimAssignmentFact {
		t.Fatalf("initializer claim form = %s, want assignment_fact", form)
	}
}

func TestEmitEvidence_AcceptsTextReferenceAnchorKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/types/analysis_ir.go", 1235,
		"// ShapeStepList / ShapeValue / ShapeBoolean / ShapeConfigValue /",
	)
	params := json.RawMessage(`{
        "items": [
          {
            "kind": "direct",
            "subject": "ShapeValue",
            "source": "internal/types/analysis_ir.go",
            "line_start": 1235,
            "summary": "comment text mentions ShapeValue as a retired shape label",
            "snippet": "// ShapeStepList / ShapeValue / ShapeBoolean / ShapeConfigValue /",
            "anchor_kind": "text_reference",
            "anchor_symbol": "ShapeValue"
          }
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].AnchorKind != types.AnchorTextReference {
		t.Fatalf("anchor kind = %s, want text_reference", got[0].AnchorKind)
	}
	if got[0].ContextRole == types.EvidenceContextRoleIllustrativeOnly {
		t.Fatalf("intentional text_reference must not be auto-downgraded to illustrative_only")
	}
	if form := types.ClaimFormOf(got[0]); form != types.ClaimTextReferenceFact {
		t.Fatalf("text_reference claim form = %s, want text_reference_fact", form)
	}
}

// LoadBearingSummary opt-in surface (2026-05-08 add — u7a deep-dive).
// The flag tells the four EvidenceXxxSurfaceText helpers to append
// the trimmed summary to the typed surface line so finalize-stage
// rendering preserves scalars (hashes, version strings, counts) the
// answer cannot synthesise from typed fields alone.

func TestEmitEvidence_AcceptsLoadBearingSummary(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "TargetSymbol", "source": "internal/agent/foo.go", "line_start": 30, "summary": "first introduced in commit 0123456 with subject feat: shipping carrier", "anchor_kind": "definition", "anchor_symbol": "TargetSymbol", "load_bearing_summary": true}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if !got[0].LoadBearingSummary {
		t.Errorf("LoadBearingSummary did not propagate from input to EvidenceItem")
	}
	if got[0].Summary == "" {
		t.Errorf("Summary lost during item construction")
	}
}

func TestEmitEvidence_AcceptsGroundedSurfaceTerms(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", 1,
		"// Source: developer.huawei.com / openharmony Index.ets minimal sample",
		"// Surface: @Entry + @Component + build()",
		"",
		"@Entry",
		"@Component",
		"struct Index {",
		"  build() {}",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Index", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", "line_start": 6, "summary": "Index is defined here", "anchor_kind": "definition", "anchor_symbol": "Index", "surface_terms": ["Index.ets", "@Entry"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) == 0 {
		t.Fatal("expected emitted evidence")
	}
	if got[0].Producer != EmitEvidenceProducer {
		t.Fatalf("first item should be model-authored evidence; got producer %q", got[0].Producer)
	}
	if strings.Join(got[0].SurfaceTerms, ",") != "Index.ets,@Entry" {
		t.Fatalf("surface terms not preserved: %#v", got[0].SurfaceTerms)
	}
}

func TestEmitEvidence_SurfaceTermReviewPromptsModelAuthoredHeaderLabels(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", 1,
		"// Source: developer.huawei.com / openharmony Index.ets minimal sample",
		"// Surface: @Entry + @Component + build()",
		"",
		"@Entry",
		"@Component",
		"struct Index {",
		"  build() {}",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Index", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", "line_start": 6, "summary": "Index is defined here", "anchor_kind": "definition", "anchor_symbol": "Index", "surface_terms": ["@Entry", "@Component"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != EmitEvidenceSurfaceTermReviewCode {
		t.Fatalf("expected surface-term review repair, got %#v", res.Repair)
	}
	if !strings.Contains(res.Repair.Hint, "Index.ets") || !strings.Contains(res.Summary, "Index.ets") {
		t.Fatalf("review should name the grounded header label, repair=%q summary=%q", res.Repair.Hint, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) == 0 {
		t.Fatal("expected emitted evidence")
	}
	if strings.Contains(strings.Join(got[0].SurfaceTerms, ","), "Index.ets") {
		t.Fatalf("tool must not auto-fill model-authored surface_terms; got %#v", got[0].SurfaceTerms)
	}
}

func TestEmitEvidence_SurfaceTermReviewPromptsDecoratorCompanions(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", 1,
		"// Source: developer.huawei.com / openharmony Index.ets minimal sample",
		"",
		"@Entry",
		"@Component",
		"struct Index {",
		"  build() {}",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Index", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", "line_start": 5, "summary": "Index is defined here", "anchor_kind": "definition", "anchor_symbol": "Index", "surface_terms": ["Index.ets", "@Entry"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != EmitEvidenceSurfaceTermReviewCode {
		t.Fatalf("expected decorator surface-term review repair, got %#v", res.Repair)
	}
	if !strings.Contains(res.Repair.Hint, "@Component") || !strings.Contains(res.Summary, "@Component") {
		t.Fatalf("review should name missing decorator companion, repair=%q summary=%q", res.Repair.Hint, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) == 0 {
		t.Fatal("expected emitted evidence")
	}
	if strings.Contains(strings.Join(got[0].SurfaceTerms, ","), "@Component") {
		t.Fatalf("tool must not auto-fill decorator surface_terms; got %#v", got[0].SurfaceTerms)
	}
}

func TestEmitEvidence_SurfaceTermReviewDoesNotPullParentDecoratorOntoNestedMethod(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", 1,
		"// Source: developer.huawei.com codelabs / @Builder decorator example",
		"// Surface: @Builder + @BuilderParam + UI element reuse mechanism",
		"",
		"@Component",
		"struct CardComponent {",
		"  @BuilderParam customHeader: () => void",
		"",
		"  @Builder defaultHeader() {",
		"    Text('Default Header').fontSize(20)",
		"  }",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "registration", "object": "Builder", "predicate": "registers", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", "line_start": 8, "summary": "@Builder method defaultHeader", "anchor_kind": "definition", "anchor_symbol": "defaultHeader", "surface_terms": ["@Builder", "defaultHeader"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	if res.Repair != nil && res.Repair.Code == EmitEvidenceSurfaceTermReviewCode &&
		(strings.Contains(res.Repair.Hint, "@Component") || strings.Contains(res.Repair.Hint, "@BuilderParam")) {
		t.Fatalf("nested method review should not require parent/sibling decorators, repair=%q", res.Repair.Hint)
	}
}

func TestEmitEvidence_RejectsRequestedDecoratorObjectMismatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Entities: []string{"@Builder"}},
			SubTopics:     []types.SubTopic{{Summary: "list @Builder fragments", Entities: []string{"@Builder"}}},
		},
	}
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets", 1,
		"// Source: openharmony @Styles and @Extend decorators",
		"// Surface: @Styles function + @Extend(Type) attribute method chains",
		"",
		"@Styles function commonCardStyle() {",
		"  .width('100%')",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "registration", "object": "Builder", "predicate": "registers", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets", "line_start": 4, "summary": "@Styles function commonCardStyle", "anchor_kind": "definition", "anchor_symbol": "commonCardStyle", "surface_terms": ["@Styles", "commonCardStyle"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected decorator alignment rejection, got success summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "requested decorator \"@Builder\"") || !strings.Contains(res.Summary, "@Styles") {
		t.Fatalf("rejection should explain requested vs attached decorator, got %q", res.Summary)
	}
}

func TestEmitEvidence_SurfaceTermReviewIgnoresUnrelatedSourcePathLabels(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 1318,
		"// constructs graph metadata for internal/types/enumeration_boundary.go",
		"func buildAnalysisIR() {",
		"  graph := analyzerGraphForNormalize(ctx, rm)",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "buildAnalysisIR", "predicate": "calls", "object": "analyzerGraphForNormalize", "source": "internal/agent/analyzer.go", "line_start": 1320, "summary": "analyzerGraphForNormalize builds the graph used by normalization", "anchor_kind": "call", "anchor_symbol": "analyzerGraphForNormalize"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	if res.Repair != nil && strings.Contains(res.Repair.Hint, "internal/types/enumeration_boundary.go") {
		t.Fatalf("unrelated source-path label should not trigger surface-term review: %q", res.Repair.Hint)
	}
}

func TestEmitEvidence_SurfaceTermReviewSatisfiedWhenHeaderLabelAuthored(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", 1,
		"// Source: developer.huawei.com / openharmony Index.ets minimal sample",
		"",
		"@Entry",
		"struct Index {",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Index", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", "line_start": 4, "summary": "Index is defined here", "anchor_kind": "definition", "anchor_symbol": "Index", "surface_terms": ["Index.ets", "@Entry"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	if res.Repair != nil && res.Repair.Code == EmitEvidenceSurfaceTermReviewCode {
		t.Fatalf("surface-term review should not fire after model-authored label is present: %#v", res.Repair)
	}
}

func TestEmitEvidence_RejectsUngroundedSurfaceTerms(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/example/file.go", 10,
		"package example",
		"type Target struct{}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Target", "source": "internal/example/file.go", "line_start": 11, "summary": "Target is defined here", "anchor_kind": "definition", "anchor_symbol": "Target", "surface_terms": ["MissingAlias.ts"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected surface_terms rejection")
	}
	if !strings.Contains(res.Summary, "surface_terms") || !strings.Contains(res.Summary, "MissingAlias.ts") {
		t.Fatalf("rejection should name surface_terms and the bad term, got %q", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Fatalf("rejected surface_terms must not persist evidence")
	}
}

func TestEmitEvidence_RejectsLoadBearingSummaryWithEmptySummary(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "TargetSymbol", "source": "internal/agent/foo.go", "line_start": 30, "anchor_kind": "definition", "anchor_symbol": "TargetSymbol", "load_bearing_summary": true}
        ]
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure (empty summary + flag set), got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "load_bearing_summary=true requires a non-empty summary") {
		t.Errorf("expected targeted error message, got: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Errorf("buffer should not be touched on failure")
	}
}

func TestEmitEvidence_LoadBearingSummaryDefaultsToFalse(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// Do NOT pass the field at all — default false should propagate.
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "TargetSymbol", "source": "internal/agent/foo.go", "line_start": 30, "summary": "free-form rationale", "anchor_kind": "definition", "anchor_symbol": "TargetSymbol"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].LoadBearingSummary {
		t.Errorf("LoadBearingSummary should default to false when field omitted from input")
	}
}

func TestEmitEvidence_RejectsUnknownKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"speculation","source":"x.go"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "unknown evidence_kind") {
		t.Errorf("error msg should mention unknown evidence_kind, got: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Errorf("buffer should not be touched on failure")
	}
}

func TestEmitEvidence_RejectsEvidenceKindValueInsideAnchorKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"evidence_kind":"direct","source":"x.go","line_start":12,"anchor_kind":"direct","anchor_symbol":"Foo"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "belongs to evidence_kind, not anchor_kind") {
		t.Fatalf("expected targeted anchor_kind guidance, got: %s", res.Summary)
	}
}

func TestEmitEvidence_SelfRefLiteralEmitsTypedClusterKey(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{
				Entities: []string{"explorer"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"evidence_kind": "direct",
				"subject": "explorer",
				"predicate": "returns",
				"object": "\"explorer\"",
				"snippet": "return \"explorer\"",
				"source": "internal/agent/sub_explorer.go",
				"line_start": 12,
				"summary": "Explorer.Name returns explorer",
				"anchor_kind": "definition",
				"anchor_symbol": "Explorer.Name"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	vs := ctx.Mutable.EvidenceClosure().ViolationsByKind(types.ViolSelfRefLiteral)
	if len(vs) != 1 {
		t.Fatalf("expected one self-ref violation, got %d", len(vs))
	}
	if got, want := vs[0].ClusterKey, "symbol:explorer|root:answer_subject.kind"; got != want {
		t.Fatalf("ClusterKey=%q, want %q", got, want)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].Confidence != 0 {
		t.Fatalf("self-ref evidence confidence=%v, want 0", got[0].Confidence)
	}
}

func TestEmitEvidence_ParametersExposeEvidenceKindNotLegacyKind(t *testing.T) {
	tool := &EmitEvidence{}
	schema := string(tool.Parameters())
	if !strings.Contains(schema, `"evidence_kind"`) {
		t.Fatalf("schema should expose evidence_kind: %s", schema)
	}
	if strings.Contains(schema, `"required":["kind"`) || strings.Contains(schema, `"required": ["kind"`) {
		t.Fatalf("schema should not require legacy kind field: %s", schema)
	}
}

func TestEmitEvidence_DemotesDocumentationStyleExactConfigMention(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectConfigKey,
				TargetLabel:  "config key",
				Targets:      []string{"explore_mid_loop_hint_budget"},
				AllowAbsence: true,
			},
		},
	}
	ctx.Mutable.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token: "explore_mid_loop_hint_budget",
		Kind:  "symbol",
	})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore_mid_loop_hint_budget",
				"predicate": "defines",
				"source": "docs/analysis_contract.md",
				"line_start": 367,
				"summary": "The token appears in a comment example only.",
				"anchor_kind": "definition",
				"anchor_symbol": "explore_mid_loop_hint_budget",
				"context_role_hint": "illustrative_only"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success with demotion feedback, got: %s", res.Summary)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "do not spend read_file budget") {
		t.Fatalf("summary should steer the model away from repairing doc mentions, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Kind != types.EvidenceUnresolved {
		t.Fatalf("kind = %q, want unresolved", got[0].Kind)
	}
	if got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("grounding status = %q, want ungrounded", got[0].GroundingStatus)
	}
	if got[0].ContextRole != types.EvidenceContextRoleIllustrativeOnly {
		t.Fatalf("context role = %q, want illustrative_only", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "do not repair this item") {
		t.Fatalf("grounding note should mark the mention non-repairable, got: %s", got[0].GroundingNote)
	}
	if res.Repair != nil && len(res.Repair.Targets) > 0 {
		t.Fatalf("drop-only illustrative mention must not queue structured repair targets, got %+v", res.Repair.Targets)
	}
}

func TestEmitEvidence_QueuesStructuredRepairTargetsForRecoveredEvidence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "conditional",
				"subject": "buildOrLoadGraph",
				"predicate": "calls",
				"object": "fullScan",
				"source": "internal/tool/repomap/tool.go",
				"line_start": 148,
				"summary": "full scan branch",
				"anchor_kind": "call",
				"anchor_symbol": "fullScan"
			},
			{
				"kind": "conditional",
				"subject": "buildOrLoadGraph",
				"predicate": "calls",
				"object": "loadFromCache",
				"source": "internal/tool/repomap/tool.go",
				"line_start": 156,
				"summary": "cache branch",
				"anchor_kind": "call",
				"anchor_symbol": "loadFromCache"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success with structured repair feedback, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "evidence_line_text_repair" {
		t.Fatalf("expected structured line-text repair metadata, got %+v", res.Repair)
	}
	if len(res.Repair.Targets) != 1 {
		t.Fatalf("expected one grouped repair target, got %+v", res.Repair.Targets)
	}
	target := res.Repair.Targets[0]
	if target.File != "internal/tool/repomap/tool.go" {
		t.Fatalf("repair target file = %q, want internal/tool/repomap/tool.go", target.File)
	}
	if !reflect.DeepEqual(target.Lines, []int{148, 156}) {
		t.Fatalf("repair target lines = %v, want [148 156]", target.Lines)
	}
	if target.Action != string(types.RepairReadFile) {
		t.Fatalf("repair target action = %q, want %q", target.Action, types.RepairReadFile)
	}
}

func TestEmitEvidenceRepairTargets_DropsRecoveredItemCoveredByGroundedSibling(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       860,
			GroundingStatus: types.GroundingGrounded,
			AnchorSymbol:    "buildAnalysisIR",
		},
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       860,
			GroundingStatus: types.GroundingRecovered,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "buildAnalysisIR",
		},
	}
	reports := []ground.Report{
		{},
		{AdjustedLine: 860},
	}
	if got := emitEvidenceRepairTargets(items, reports); len(got) != 0 {
		t.Fatalf("recovered item already covered by a grounded sibling should not queue repair targets, got %+v", got)
	}
}

func TestBuildEmitEvidenceRepair_EmitsStructuredNoopEnvelopeForCoveredRecoveredItems(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       852,
			GroundingStatus: types.GroundingGrounded,
			AnchorSymbol:    "buildAnalysisIR",
		},
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       849,
			GroundingStatus: types.GroundingRecovered,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "buildAnalysisIR",
		},
	}
	reports := []ground.Report{
		{},
		{AdjustedLine: 852},
	}
	repair := buildEmitEvidenceRepair(nil, items, reports)
	if repair == nil {
		t.Fatal("expected structured repair envelope even when no actionable line-text repair remains")
	}
	if repair.Code != "evidence_line_text_repair" {
		t.Fatalf("repair code=%q, want evidence_line_text_repair", repair.Code)
	}
	if len(repair.Targets) != 0 {
		t.Fatalf("covered recovered sibling should not produce actionable targets, got %+v", repair.Targets)
	}
	if got := repair.Metadata["repair_status"]; got != "satisfied_or_non_actionable" {
		t.Fatalf("repair_status=%q, want satisfied_or_non_actionable", got)
	}
}

func TestEmitEvidence_PreservesValidatedDiagramRoleHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "cmd/root.go", 1635,
		"if rs.ExploreMidLoopMinIteration != nil {",
		"	h.MidLoopMinIteration = *rs.ExploreMidLoopMinIteration",
	)
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "CLI override binding",
				"predicate": "overrides",
				"source": "cmd/root.go",
				"line_start": 1635,
				"summary": "CLI override applies when non-nil.",
				"anchor_kind": "condition",
				"anchor_symbol": "ExploreMidLoopMinIteration",
				"diagram_role_hint": "override"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleOverride {
		t.Fatalf("diagram role = %q, want override", got[0].DiagramRole)
	}
}

func TestEmitEvidence_PrefersMutableExactContextFilesForDiagramRoleHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.RepoRoot = `C:\Users\ssccv\codrax`
	seedReadFileHistory(ctx, "internal/types/config.go", 876,
		"func DefaultExploreHeuristics() ExploreHeuristics {",
	)
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		EvidencePlan: types.EvidencePlan{
			RequiredFiles: []string{"internal/context/builder.go"},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	ctx.Mutable.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "DefaultExploreHeuristics",
				"predicate": "defines",
				"source": "C:\\Users\\ssccv\\codrax\\internal\\types\\config.go",
				"line_start": 876,
				"summary": "code-default lineage anchor",
				"anchor_kind": "definition",
				"anchor_symbol": "DefaultExploreHeuristics",
				"context_role_hint": "related_context",
				"diagram_role_hint": "default"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Source != "internal/types/config.go" {
		t.Fatalf("source = %q, want canonical repo-relative path", got[0].Source)
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleDefault {
		t.Fatalf("diagram role = %q, want default", got[0].DiagramRole)
	}
	if strings.Contains(got[0].GroundingNote, "outside the current same-scope required files") {
		t.Fatalf("grounding note should not treat the mutable exact-context file as out of scope, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_PreservesRequestedDiagramRoleWhenScopeRejectsValidation(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.RepoRoot = `C:\Users\ssccv\codrax`
	seedReadFileHistory(ctx, "cmd/root.go", 1635,
		"if rs.ExploreMidLoopMinIteration != nil {",
		"	h.MidLoopMinIteration = *rs.ExploreMidLoopMinIteration",
	)
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	ctx.Mutable.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "CLI override binding",
				"predicate": "maps",
				"source": "C:\\Users\\ssccv\\codrax\\cmd\\root.go",
				"line_start": 1635,
				"summary": "CLI override binding layer",
				"anchor_kind": "condition",
				"anchor_symbol": "ExploreMidLoopMinIteration",
				"context_role_hint": "related_context",
				"diagram_role_hint": "override"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleOverride {
		t.Fatalf("diagram role = %q, want override when structurally-valid precedence evidence stays within grounded config context", got[0].DiagramRole)
	}
	if got[0].RequestedDiagramRole != types.EvidenceDiagramRoleOverride {
		t.Fatalf("requested diagram role = %q, want override", got[0].RequestedDiagramRole)
	}
	if strings.Contains(got[0].GroundingNote, "outside the current same-scope required files") {
		t.Fatalf("grounding note should no longer reject structurally-valid requested precedence evidence only because it lives outside the narrow same-scope file set, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_AcceptsLegacyYAMLDiagramRoleAliasForConfigFiles(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "codrax.yaml.example", 22,
		"#   code defaults  <  <exeDir>/codrax.yaml  <  command-line flags",
	)
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 怎么生效？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "codrax.yaml",
				"object": "command-line flags",
				"predicate": "precedes",
				"source": "codrax.yaml.example",
				"line_start": 22,
				"summary": "code defaults < codrax.yaml < command-line flags",
				"anchor_kind": "definition",
				"anchor_symbol": "exeDir",
				"context_role_hint": "related_context",
				"diagram_role_hint": "yaml"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleConfig {
		t.Fatalf("diagram role = %q, want canonical config role", got[0].DiagramRole)
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("config precedence comment should stay related_context, got %q", got[0].ContextRole)
	}
}

func TestEmitEvidence_DropsConfigTraceDiagramRoleForOutOfScopeHelper(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 是怎么生效的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectConfigKey,
				TargetLabel:  "config key",
				Targets:      []string{"explore_mid_loop_hint_budget"},
				AllowAbsence: true,
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "collectConfigTraceDiagramAnchors",
				"predicate": "assembles",
				"source": "internal/context/builder.go",
				"line_start": 1763,
				"summary": "helper for answer-document prompt assembly",
				"anchor_kind": "definition",
				"anchor_symbol": "collectConfigTraceDiagramAnchors",
				"context_role_hint": "related_context",
				"diagram_role_hint": "runtime"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want unknown for out-of-scope helper evidence", got[0].DiagramRole)
	}
}

func TestEmitEvidence_DoesNotInferDiagramRoleWithoutHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario:      types.ScenarioConfigTrace,
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore",
				"predicate": "documents",
				"source": "codrax.yaml.example",
				"line_start": 20,
				"summary": "yaml precedence comment",
				"anchor_kind": "definition",
				"anchor_symbol": "explore"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want unknown without explicit hint", got[0].DiagramRole)
	}
	if !strings.Contains(res.Summary, "Config-precedence task detected") {
		t.Fatalf("summary should nudge explicit diagram_role_hint usage, got: %s", res.Summary)
	}
}

func TestEmitEvidence_DropsInconsistentDiagramRoleHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "LoadRuntimeConfig",
				"predicate": "binds",
				"source": "internal/config/runtime.go",
				"line_start": 194,
				"summary": "runtime binding layer",
				"anchor_kind": "assignment",
				"anchor_symbol": "resolved",
				"diagram_role_hint": "yaml"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want unknown for inconsistent hint", got[0].DiagramRole)
	}
	if !strings.Contains(got[0].GroundingNote, "diagram_role_hint=config was ignored") {
		t.Fatalf("grounding note should explain why the role hint was ignored, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(got[0].GroundingNote, "`default` / `runtime` / `override`") {
		t.Fatalf("grounding note should point at valid code-layer roles, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(res.Summary, "diagram_role_hint=config was ignored") {
		t.Fatalf("tool summary should surface the ignored-role guidance, got: %s", res.Summary)
	}
}

func TestEmitEvidence_ConfigCommentLineBecomesIllustrativeOnly(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary: "[codrax.yaml.example: showing lines 22-22 of 801]\n" +
					"  22│ #   code defaults  <  <exeDir>/codrax.yaml  <  command-line flags\n",
			},
		},
	})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "codrax.yaml",
				"object": "command-line flags",
				"predicate": "precedes",
				"source": "codrax.yaml.example",
				"line_start": 22,
				"summary": "code defaults < codrax.yaml < command-line flags",
				"anchor_kind": "definition",
				"anchor_symbol": "exeDir",
				"context_role_hint": "related_context",
				"diagram_role_hint": "config"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context for config-file precedence comment", got[0].ContextRole)
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleConfig {
		t.Fatalf("diagram role = %q, want config for config-file precedence comment", got[0].DiagramRole)
	}
	if strings.Contains(strings.ToLower(got[0].GroundingNote), "do not repair this item") {
		t.Fatalf("config-file precedence comment should remain usable context, got: %q", got[0].GroundingNote)
	}
	if strings.Contains(strings.ToLower(res.Summary), "do not spend read_file budget") {
		t.Fatalf("tool summary should not tell the model to drop config-file precedence comments, got: %s", res.Summary)
	}
}

func TestEmitEvidence_ConfigPrecedenceCommentCanGroundWithoutExactAnchorSymbol(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary: "[codrax.yaml.example: showing lines 20-22 of 801]\n" +
					"  20│ # Precedence (lowest wins last):\n" +
					"  21│ #\n" +
					"  22│ #   code defaults  <  <exeDir>/codrax.yaml  <  command-line flags\n",
			},
		},
	})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "codrax.yaml",
				"object": "command-line flags",
				"predicate": "precedes",
				"source": "codrax.yaml.example",
				"line_start": 22,
				"summary": "code defaults < codrax.yaml < command-line flags",
				"anchor_kind": "definition",
				"anchor_symbol": "exeDir",
				"context_role_hint": "related_context",
				"diagram_role_hint": "config"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].GroundingStatus != types.GroundingGrounded {
		t.Fatalf("grounding status = %q, want grounded", got[0].GroundingStatus)
	}
	if got[0].GroundingTier != types.TierLineText {
		t.Fatalf("grounding tier = %q, want line_text", got[0].GroundingTier)
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleConfig {
		t.Fatalf("diagram role = %q, want config", got[0].DiagramRole)
	}
	if res.Repair != nil && len(res.Repair.Targets) > 0 {
		t.Fatalf("grounded config precedence comment must not queue repair targets, got %+v", res.Repair.Targets)
	}
}

func TestEmitEvidence_KeepsFreeformExactMentionAsRelatedContextWithoutAnchoredTargetWindow(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "FooHandler 在哪里定义？",
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"FooHandler"},
				ExactTargets:    []string{"FooHandler"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectFunctionName,
				TargetLabel:  "symbol",
				Targets:      []string{"FooHandler"},
				AllowAbsence: true,
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "mechanism",
				"subject": "buildFallbackHandlers",
				"predicate": "describes",
				"source": "internal/agent/router.go",
				"line_start": 42,
				"summary": "The fallback path explains why FooHandler is absent from the runtime router.",
				"anchor_kind": "definition",
				"anchor_symbol": "buildFallbackHandlers",
				"context_role_hint": "related_context"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "nearby context only") {
		t.Fatalf("freeform exact mention should stay nearby-context only, got: %q", got[0].GroundingNote)
	}
	if strings.Contains(strings.ToLower(got[0].Summary), "context only") {
		t.Fatalf("evidence summary should stay semantic; context-only guidance belongs in grounding_note, got: %q", got[0].Summary)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "nearby context only") {
		t.Fatalf("tool summary should still surface nearby-context guidance, got: %q", res.Summary)
	}
}

func TestEmitEvidence_PromotesAbsenceSupportWhenAnchoredWindowNamesExactTarget(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary: "[internal/agent/router.go: showing lines 40-42 of 60]\n" +
					"  40│func buildFallbackHandlers() {\n" +
					"  41│    // FooHandler is absent from the runtime router on this branch.\n" +
					"  42│    _ = 0\n",
			},
		},
	})
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "FooHandler 在哪里定义？",
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"FooHandler"},
				ExactTargets:    []string{"FooHandler"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectFunctionName,
				TargetLabel:  "symbol",
				Targets:      []string{"FooHandler"},
				AllowAbsence: true,
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "mechanism",
				"subject": "buildFallbackHandlers",
				"predicate": "describes",
				"source": "internal/agent/router.go",
				"line_start": 40,
				"summary": "The fallback path explains why FooHandler is absent from the runtime router.",
				"anchor_kind": "definition",
				"anchor_symbol": "buildFallbackHandlers",
				"context_role_hint": "related_context"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleAbsenceSupport {
		t.Fatalf("context role = %q, want absence_support", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "absence support") {
		t.Fatalf("anchored exact mention should be promoted to absence_support, got: %q", got[0].GroundingNote)
	}
	if strings.Contains(strings.ToLower(got[0].Summary), "absence support") {
		t.Fatalf("evidence summary should stay semantic; absence-support guidance belongs in grounding_note, got: %q", got[0].Summary)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "absence support") {
		t.Fatalf("tool summary should still surface absence-support guidance, got: %q", res.Summary)
	}
}

func TestEmitEvidence_NegativeAbsenceProbeDoesNotQueueLineRepair(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary: "[internal/types/config.go: showing lines 768-770 of 1188]\n" +
					"  768│ type ExploreHeuristics struct {\n" +
					"  769│ \t// --- Mid-loop thresholds ---\n" +
					"  770│ \n",
			},
		},
	})
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectConfigKey,
				TargetLabel:  "config key",
				Targets:      []string{"explore_mid_loop_hint_budget"},
				AllowAbsence: true,
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore_mid_loop_hint_budget",
				"object": "ExploreHeuristics",
				"predicate": "absent_from",
				"source": "internal/types/config.go",
				"line_start": 768,
				"summary": "explore_mid_loop_hint_budget 不存在于 ExploreHeuristics 中",
				"anchor_kind": "definition",
				"anchor_symbol": "explore_mid_loop_hint_budget",
				"context_role_hint": "absence_support"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleAbsenceSupport {
		t.Fatalf("context role = %q, want absence_support", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "do not repair this item") {
		t.Fatalf("negative exact-absence probe should be marked non-repairable, got: %q", got[0].GroundingNote)
	}
	if res.Repair != nil && len(res.Repair.Targets) > 0 {
		t.Fatalf("negative exact-absence probe must not queue repair targets, got %+v", res.Repair.Targets)
	}
}

func TestEmitEvidence_DowngradesIllustrativeExactTargetMentionWithoutPendingFinding(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore_mid_loop_hint_budget",
				"predicate": "appears_in",
				"source": "internal/skill/analysis_contract.go",
				"line_start": 367,
				"summary": "analysis_contract.go uses explore_mid_loop_hint_budget only as a docstring example",
				"anchor_kind": "definition",
				"anchor_symbol": "explore_mid_loop_hint_budget",
				"context_role_hint": "absence_support"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleIllustrativeOnly {
		t.Fatalf("context role = %q, want illustrative_only", got[0].ContextRole)
	}
	if got[0].Kind != types.EvidenceUnresolved || got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("illustrative exact-target mention should be downgraded to unresolved/ungrounded, got kind=%q grounding=%q", got[0].Kind, got[0].GroundingStatus)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "illustrative mention of the exact") {
		t.Fatalf("grounding note should explain the downgrade, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_DowngradesNegativeExactProbeDefiningHintToAbsenceSupport(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "RuntimeSettings",
				"predicate": "does not bind",
				"object": "explore_mid_loop_hint_budget",
				"source": "internal/config/runtime.go",
				"line_start": 231,
				"summary": "RuntimeSettings does not bind the missing exact key",
				"anchor_kind": "definition",
				"anchor_symbol": "RuntimeSettings",
				"context_role_hint": "defining"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleAbsenceSupport {
		t.Fatalf("context role = %q, want absence_support", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "negative probe") {
		t.Fatalf("grounding note should explain negative exact-probe downgrade, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_DowngradesSubjectOnlyTargetDefiningHintToAbsenceSupport(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore_mid_loop_hint_budget",
				"predicate": "is absent from YAML struct",
				"object": "explore_mid_loop_hint_budget",
				"source": "internal/config/runtime.go",
				"line_start": 334,
				"summary": "RuntimeSettings does not expose the missing exact key",
				"anchor_kind": "definition",
				"anchor_symbol": "RuntimeSettings",
				"context_role_hint": "defining"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleAbsenceSupport {
		t.Fatalf("context role = %q, want absence_support", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "absence support") && !strings.Contains(strings.ToLower(got[0].GroundingNote), "negative probe") {
		t.Fatalf("grounding note should explain subject-only exact-target downgrade, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_DowngradesSameFamilyNegativeProbeDefiningHintToRelatedContext(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "ExploreHeuristics",
				"predicate": "does not define",
				"object": "mid_loop_hint_budget",
				"source": "internal/types/config.go",
				"line_start": 627,
				"summary": "ExploreHeuristics does not define a mid_loop_hint_budget field",
				"anchor_kind": "definition",
				"anchor_symbol": "ExploreHeuristics",
				"context_role_hint": "defining"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "negative probe") {
		t.Fatalf("grounding note should explain negative same-family probe downgrade, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_DowngradesCrossFamilyDefiningHintDuringExactAbsence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	ctx.Mutable.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token: "explore_mid_loop_hint_budget",
		Kind:  "symbol",
	})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "AgentLoopMaxMidLoopInjects",
				"predicate": "defines",
				"source": "internal/config/runtime.go",
				"line_start": 246,
				"summary": "yaml key agent_loop_max_midloop_injects",
				"anchor_kind": "definition",
				"anchor_symbol": "AgentLoopMaxMidLoopInjects",
				"context_role_hint": "defining"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "different nearby config family") {
		t.Fatalf("grounding note should call out cross-family context, got: %q", got[0].GroundingNote)
	}
	if strings.Contains(strings.ToLower(got[0].Summary), "different nearby config family") {
		t.Fatalf("evidence summary should stay semantic; cross-family guidance belongs in grounding_note, got: %q", got[0].Summary)
	}
}

func TestEmitEvidence_KeepsExactTargetPendingWithoutAnalyzerUnverifiedWhenNoDefiningProofExists(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 鐨勬渶缁堟湁鏁堝€兼槸鎬庝箞璁＄畻鍑烘潵鐨勶紵",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "AgentLoopMaxMidLoopInjects",
				"predicate": "defines",
				"source": "internal/config/runtime.go",
				"line_start": 246,
				"summary": "yaml key agent_loop_max_midloop_injects",
				"anchor_kind": "definition",
				"anchor_symbol": "AgentLoopMaxMidLoopInjects",
				"context_role_hint": "defining",
				"diagram_role_hint": "config"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want cleared while the exact target remains unresolved", got[0].DiagramRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "different nearby config family") {
		t.Fatalf("grounding note should call out cross-family context, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_RejectsUnknownTopLevelField(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"evidence":[{"kind":"direct","source":"x.go"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown top-level field")
	}
	if !strings.Contains(res.Summary, "invalid params") {
		t.Errorf("got: %s", res.Summary)
	}
}

func TestEmitEvidence_RejectsUnknownItemField(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","note":"hi"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown item field")
	}
	if !strings.Contains(res.Summary, "unknown field") {
		t.Errorf("error should mention unknown field: %s", res.Summary)
	}
}

func TestEmitEvidence_RejectsMissingSource(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","subject":"Foo"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on missing source")
	}
}

func TestEmitEvidence_RejectsNonPathSource(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"FooBar"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure: 'FooBar' has no slash or dot, not path-shaped")
	}
}

func TestEmitEvidence_RejectsStringLineStart(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","line_start":"twelve"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on string line_start")
	}
}

func TestEmitEvidence_RejectsRelationshipWithoutObject(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"A","source":"x.go","line_start":1}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure: relationship needs object")
	}
}

func TestEmitEvidence_NormalizesCallDirectionToCallerCallee(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/tool/repomap/tool.go": {
				RelPath: "internal/tool/repomap/tool.go",
				Symbols: []repomap.Symbol{
					{Name: "buildOrLoadGraph", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 121, EndLine: 170},
					{Name: "fullScan", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 172, EndLine: 220},
				},
				Relations: []repomap.Relation{
					{
						Kind: "call",
						File: "internal/tool/repomap/tool.go",
						Line: 149,
						To:   "fullScan",
						ToEP: repomap.RelationEndpoint{Name: "fullScan", File: "internal/tool/repomap/tool.go", Line: 149},
					},
				},
			},
		},
	}
	ctx.Mutable.SetSearchGraph(graph)
	params := json.RawMessage(`{"items":[{"kind":"conditional","subject":"fullScan","predicate":"calls","object":"buildOrLoadGraph","source":"internal/tool/repomap/tool.go","line_start":149,"condition":"index.NeedsFullRescan(cacheDir)","summary":"If cache directory needs a full rescan, it calls fullScan.","anchor_kind":"call","anchor_symbol":"fullScan","snippet":"return fullScan(repoRoot, cacheDir, entries, query)"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Subject != "buildOrLoadGraph" {
		t.Fatalf("subject = %q, want buildOrLoadGraph", got[0].Subject)
	}
	if got[0].Object != "fullScan" {
		t.Fatalf("object = %q, want fullScan", got[0].Object)
	}
	if got[0].Predicate != "calls" {
		t.Fatalf("predicate = %q, want calls", got[0].Predicate)
	}
}

func TestEmitEvidence_NormalizeCallDirection_DoesNotFallbackToWrongSameLineCall(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/tool/repomap/tool.go": {
				RelPath: "internal/tool/repomap/tool.go",
				Symbols: []repomap.Symbol{
					{Name: "buildOrLoadGraph", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 121, EndLine: 170},
					{Name: "fullScan", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 172, EndLine: 220},
				},
				Relations: []repomap.Relation{
					{
						Kind: "call",
						File: "internal/tool/repomap/tool.go",
						Line: 147,
						To:   "NeedsFullRescan",
						ToEP: repomap.RelationEndpoint{Name: "NeedsFullRescan", Receiver: "index", File: "internal/tool/repomap/tool.go", Line: 147},
					},
					{
						Kind: "call",
						File: "internal/tool/repomap/tool.go",
						Line: 149,
						To:   "fullScan",
						ToEP: repomap.RelationEndpoint{Name: "fullScan", File: "internal/tool/repomap/tool.go", Line: 149},
					},
				},
			},
		},
	}
	ctx.Mutable.SetSearchGraph(graph)
	params := json.RawMessage(`{"items":[{"kind":"conditional","subject":"buildOrLoadGraph","predicate":"calls","object":"fullScan","source":"internal/tool/repomap/tool.go","line_start":147,"condition":"index.NeedsFullRescan(cacheDir)","summary":"Calls fullScan if no cache is found or a full scan is needed.","anchor_kind":"call","anchor_symbol":"fullScan","snippet":"if index.NeedsFullRescan(cacheDir) {\n    logging.Info(\"repo_map: full scan (%d files, no cache)\", len(entries))\n    return fullScan(repoRoot, cacheDir, entries, query)"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Object != "fullScan" {
		t.Fatalf("object = %q, want fullScan", got[0].Object)
	}
	if got[0].LineStart != 149 {
		t.Fatalf("line_start = %d, want recovery to 149", got[0].LineStart)
	}
}

// TestEmitEvidence_AutoSwapsLineEndBeforeStart pins the 2026-04-17
// change: line_end < line_start is treated as an obvious
// transposition typo and auto-swapped. The summary includes a
// "double-check the range" warning so the LLM can notice. Earlier
// behaviour rejected the whole batch over this single keystroke
// mistake; trace 1776439797257469553 iter=2 lost four otherwise-
// valid evidence items to a single typo on item[3].
func TestEmitEvidence_AutoSwapsLineEndBeforeStart(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{
		"kind":"direct","source":"x.go","line_start":50,"line_end":10,
		"anchor_kind":"definition","anchor_symbol":"Foo",
		"subject":"Foo","summary":"test"
	}]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("auto-swap path must succeed, got rejection: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "AUTO-SWAPPED") {
		t.Errorf("summary must warn about auto-swap: %q", res.Summary)
	}
	// Item should be in the accepted buffer with line_start=10, line_end=50.
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 accepted item, got %d", len(emitted))
	}
	if emitted[0].LineStart != 10 || emitted[0].LineEnd != 50 {
		t.Errorf("swapped range wrong: got [%d, %d], want [10, 50]",
			emitted[0].LineStart, emitted[0].LineEnd)
	}
}

// TestEmitEvidence_PerItemRejectSkipsInsteadOfFailingBatch locks in
// the graceful-degrade behaviour: a single invalid item in a mostly-
// healthy batch skips that item and keeps the rest. Fires ONLY when
// the reject ratio is below 50%; above that the batch is rejected
// wholesale.
func TestEmitEvidence_PerItemRejectSkipsInsteadOfFailingBatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// 4 valid + 1 invalid (missing anchor_kind). 1/5 = 20% → sparse
	// reject path. Batch succeeds with 4 accepted; 1 skipped in the
	// Summary. (The healthy items all need required fields filled.)
	params := json.RawMessage(`{"items":[
		{"kind":"direct","source":"a.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"A","subject":"A","summary":"x"},
		{"kind":"direct","source":"b.go","line_start":2,"anchor_kind":"definition","anchor_symbol":"B","subject":"B","summary":"x"},
		{"kind":"direct","source":"c.go","line_start":3,"subject":"C","summary":"x"},
		{"kind":"direct","source":"d.go","line_start":4,"anchor_kind":"definition","anchor_symbol":"D","subject":"D","summary":"x"},
		{"kind":"direct","source":"e.go","line_start":5,"anchor_kind":"definition","anchor_symbol":"E","subject":"E","summary":"x"}
	]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("sparse-reject batch must succeed, got rejection: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 4 {
		t.Errorf("expected 4 accepted, got %d", len(ctx.Mutable.EmittedEvidence()))
	}
	if !strings.Contains(res.Summary, "SKIPPED") {
		t.Errorf("summary must mention skipped item: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "items[2]") {
		t.Errorf("summary must identify the bad item index: %q", res.Summary)
	}
}

// TestEmitEvidence_PartialBatchSurvives pins the commit-61 Batch
// F.1 contract (red line "no system hard-cap"): pre-fix, ≥ 50%
// rejected items hard-failed the WHOLE call — even the valid 1
// item was discarded. Now the 1 valid item survives + per-item
// reject reasons surface in Summary so the LLM can re-emit just
// the failed ones. Only zero-survivor batches still fail.
func TestEmitEvidence_PartialBatchSurvives(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// 1 valid + 2 invalid (missing anchor_kind on two items).
	// Pre-commit-61: hard-failed 2/3. Post-fix: 1 valid item kept,
	// 2 rejections surfaced in Summary.
	params := json.RawMessage(`{"items":[
		{"kind":"direct","source":"a.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"A","subject":"A","summary":"x"},
		{"kind":"direct","source":"b.go","line_start":2,"subject":"B","summary":"x"},
		{"kind":"direct","source":"c.go","line_start":3,"subject":"C","summary":"x"}
	]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("partial batch should succeed (1 valid item kept); got failure: %s", res.Summary)
	}
	// Summary must still surface per-item reject reasons so the LLM
	// knows to re-emit the failed items.
	if !strings.Contains(strings.ToLower(res.Summary), "skipped") &&
		!strings.Contains(strings.ToLower(res.Summary), "reject") &&
		!strings.Contains(res.Summary, "items[id=") {
		t.Errorf("partial batch Summary must surface per-item reject reasons: %q", res.Summary)
	}
}

// TestEmitEvidence_AllInvalidStillFails pins the empty-survival
// gate: when ZERO items survived per-item validation, the call
// can't proceed (no closure entry to write).
func TestEmitEvidence_AllInvalidStillFails(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[
		{"kind":"direct","source":"a.go","line_start":1,"subject":"A","summary":"x"},
		{"kind":"direct","source":"b.go","line_start":2,"subject":"B","summary":"x"}
	]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("zero-survivor batch must fail; got success: %s", res.Summary)
	}
}

func TestEmitEvidence_ConfigAbsentPrimaryMarksSubstituteEvidenceContextOnly(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RawRequest: missingKey + " 在哪里定义？",
		Scenario:   types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			Kind:            "config_mapping",
			PrimaryEntities: []string{missingKey},
			Entities:        []string{missingKey},
			ExactTargets:    []string{missingKey},
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
	}}
	ctx.StageReports = []types.StageReport{{
		Stage:    types.StageAnalyze,
		Agent:    types.AgentAnalyzer,
		Findings: "The key ~~`" + missingKey + "`~~ [unverified: symbol not in repo graph] was not found exactly.",
	}}
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "AgentLoopMaxMidLoopInjects", "source": "internal/config/runtime.go", "line_start": 184, "summary": "related YAML key controls midloop injection budget", "anchor_kind": "definition", "anchor_symbol": "AgentLoopMaxMidLoopInjects"}
        ]
    }`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("related substitute evidence context role=%q, want related_context", got[0].ContextRole)
	}
	if strings.Contains(strings.ToLower(got[0].Summary), "context only") {
		t.Fatalf("evidence summary should stay semantic; context-only guidance belongs in grounding_note, got: %q", got[0].Summary)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "do not repair this item") {
		t.Fatalf("grounding note should suppress substitute repair loops: %q", got[0].GroundingNote)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "do not spend read_file budget") {
		t.Fatalf("tool summary should still steer away from repairing substitute exact hits, got: %q", res.Summary)
	}
}

func TestEmitEvidence_DemotesCrossCallableLineLocalOwnerMismatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 883,
		"func buildAnalysisIR(ctx *types.AgentContext) (*types.AnalysisIR, error) {",
		"if ctx == nil || ctx.Mutable == nil {",
		`	return nil, errors.New("analyzer: missing AgentContext.Mutable")`,
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/analyzer.go": {
				RelPath: "internal/agent/analyzer.go",
				Symbols: []repomap.Symbol{
					{
						Name:     "ParseOutput",
						Receiver: "(*analyzerEvaluator)",
						Kind:     "method",
						Line:     629,
						EndLine:  879,
					},
					{
						Name:    "buildAnalysisIR",
						Kind:    "function",
						Line:    883,
						EndLine: 1100,
					},
				},
			},
		},
	})
	params := json.RawMessage(`{
		"items": [
			{
				"evidence_kind": "conditional",
				"subject": "ParseOutput",
				"predicate": "checks",
				"source": "internal/agent/analyzer.go",
				"line_start": 884,
				"condition": "ctx == nil || ctx.Mutable == nil",
				"summary": "ParseOutput checks ctx before the panic path continues.",
				"anchor_kind": "condition",
				"anchor_symbol": "buildAnalysisIR"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success with owner-mismatch demotion, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].OwnerSymbol != "buildAnalysisIR" {
		t.Fatalf("owner symbol = %q, want buildAnalysisIR", got[0].OwnerSymbol)
	}
	if got[0].Kind != types.EvidenceUnresolved {
		t.Fatalf("kind = %q, want unresolved", got[0].Kind)
	}
	if got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("grounding status = %q, want ungrounded", got[0].GroundingStatus)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "line-local statement") {
		t.Fatalf("grounding note should explain the owner mismatch, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "drop the item") {
		t.Fatalf("summary should steer away from repairing the cross-owner claim, got: %q", res.Summary)
	}
}

func TestEmitEvidence_DemotesConditionClaimThatDoesNotMatchGroundedLine(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 884,
		"if ctx == nil || ctx.Mutable == nil {",
		`	return nil, errors.New("analyzer: missing AgentContext.Mutable")`,
		"}",
		"raw := ctx.Mutable.RequestModel()",
		"if raw == nil {",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/analyzer.go": {
				RelPath: "internal/agent/analyzer.go",
				Symbols: []repomap.Symbol{
					{
						Name:    "buildAnalysisIR",
						Kind:    "function",
						Line:    883,
						EndLine: 930,
					},
				},
			},
		},
	})
	params := json.RawMessage(`{
		"items": [
			{
				"evidence_kind": "conditional",
				"source": "internal/agent/analyzer.go",
				"line_start": 887,
				"condition": "ctx != nil && ctx.Mutable == nil",
				"summary": "guard only checks ctx != nil and misses nil Mutable",
				"anchor_kind": "condition",
				"anchor_symbol": "RequestModel"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success with unsupported-condition demotion, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Kind != types.EvidenceUnresolved {
		t.Fatalf("kind = %q, want unresolved", got[0].Kind)
	}
	if got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("grounding status = %q, want ungrounded", got[0].GroundingStatus)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "condition anchor is not supported") {
		t.Fatalf("grounding note should explain unsupported condition claim, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "do not repair") {
		t.Fatalf("summary should steer away from repairing the speculative condition claim, got: %q", res.Summary)
	}
}

func TestEmitEvidence_ConditionSnippetCorroboratesAbbreviatedClaimAcrossLanguages(t *testing.T) {
	cases := []struct {
		name   string
		file   string
		line   string
		cond   string
		anchor string
	}{
		{
			name:   "go selector chain with elided threshold",
			file:   "internal/analysis/criterion/eval.go",
			line:   "if env.IR.AnswerContract.CitationReq.Required && env.DraftCitations < env.IR.AnswerContract.CitationReq.MinCitations {",
			cond:   "if env.IR.AnswerContract.CitationReq.Required && ...",
			anchor: "Required",
		},
		{
			name:   "arkts property chain with elided rhs",
			file:   "entry/src/main/ets/pages/Index.ets",
			line:   "if (this.config.citation.required && this.state.ready) {",
			cond:   "if this.config.citation.required && ...",
			anchor: "required",
		},
		{
			name:   "cpp member chain with elided rhs",
			file:   "src/citation.cpp",
			line:   "if (ctx.config.citation.required && budget.remaining() > 0) {",
			cond:   "if ctx.config.citation.required && ...",
			anchor: "required",
		},
		{
			name:   "cangjie member chain with elided rhs",
			file:   "src/citation.cj",
			line:   "if (ctx.config.citation.required && ctx.ready) {",
			cond:   "if ctx.config.citation.required && ...",
			anchor: "required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &EmitEvidence{}
			ctx := newEmitCtx()
			seedReadFileHistory(ctx, tc.file, 42, tc.line)
			body, err := json.Marshal(map[string]any{
				"items": []map[string]any{{
					"scope":         "line",
					"evidence_kind": "direct",
					"subject":       "owner",
					"source":        tc.file,
					"line_start":    42,
					"condition":     tc.cond,
					"summary":       "guard reads the target member before deciding the path",
					"anchor_kind":   "condition",
					"anchor_symbol": tc.anchor,
					"snippet":       tc.line,
				}},
			})
			if err != nil {
				t.Fatalf("marshal params: %v", err)
			}

			res, err := tool.Execute(ctx, json.RawMessage(body))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Success {
				t.Fatalf("expected success, got: %s", res.Summary)
			}
			got := ctx.Mutable.EmittedEvidence()
			if len(got) != 1 {
				t.Fatalf("want 1 item, got %d", len(got))
			}
			if got[0].Kind == types.EvidenceUnresolved || got[0].GroundingStatus != types.GroundingGrounded {
				t.Fatalf("condition with exact snippet should remain grounded, kind=%s status=%s note=%q",
					got[0].Kind, got[0].GroundingStatus, got[0].GroundingNote)
			}
		})
	}
}

func TestEmitEvidence_RejectsEmptyItems(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on empty items")
	}
}

func TestEmitEvidence_RejectsMissingMutable(t *testing.T) {
	tool := &EmitEvidence{}
	res, _ := tool.Execute(&types.BusContext{}, json.RawMessage(`{"items":[]}`))
	if res.Success {
		t.Fatal("expected failure when Mutable is nil")
	}
}

func TestEmitEvidence_StableIDDedups(t *testing.T) {
	// Same content emitted twice produces identical IDs so the
	// downstream merger can dedup. Verify the IDs are stable.
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	body := `{"items":[{"kind":"direct","subject":"Foo","source":"a/b.go","line_start":7,"summary":"x","anchor_kind":"definition","anchor_symbol":"Foo"}]}`
	_, _ = tool.Execute(ctx, json.RawMessage(body))
	_, _ = tool.Execute(ctx, json.RawMessage(body))
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("want 2 items in buffer (dedup happens later), got %d", len(got))
	}
	if got[0].ID == "" || got[0].ID != got[1].ID {
		t.Errorf("IDs should match for identical content: %q vs %q", got[0].ID, got[1].ID)
	}
}

func TestEmitEvidence_ResetEmittedEvidence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	_, _ = tool.Execute(ctx, json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"X"}]}`))
	if len(ctx.Mutable.EmittedEvidence()) != 1 {
		t.Fatalf("expected 1 item before reset")
	}
	ctx.Mutable.ResetEmittedEvidence()
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Errorf("expected empty buffer after reset")
	}
}

// TestEmitEvidence_AcceptsExplicitLineScope pins the 2026-05+ scope
// migration: when the LLM explicitly sets scope=line on a line-shaped
// emit, the tool accepts it without redirect. The transitional
// default-to-ScopeLine fallback covers tests that omit the field;
// this test confirms the explicit path works the same.
func TestEmitEvidence_AcceptsExplicitLineScope(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "line", "kind": "direct", "subject": "isOK", "source": "internal/agent/foo.go", "line_start": 30, "summary": "isOK returns true", "anchor_kind": "definition", "anchor_symbol": "isOK"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 emitted item, got %d", len(emitted))
	}
	if emitted[0].Scope != types.ScopeLine {
		t.Errorf("Scope = %q, want %q", emitted[0].Scope, types.ScopeLine)
	}
}

// TestEmitEvidence_RejectsInvalidScopeValue confirms the schema
// validator rejects unknown scope values with a useful error.
func TestEmitEvidence_RejectsInvalidScopeValue(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "wrong_scope", "kind": "direct", "source": "a.go", "line_start": 1, "anchor_kind": "definition", "anchor_symbol": "X"}
        ]
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected rejection on invalid scope value")
	}
	if !strings.Contains(res.Summary, "scope") {
		t.Errorf("rejection should mention scope; got: %s", res.Summary)
	}
}

// TestEmitEvidence_AcceptsScopeFile pins File-scope emission. No
// line_start required; FileRoleLabel mandatory.
func TestEmitEvidence_AcceptsScopeFile(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "file", "kind": "direct", "source": "codrax.yaml", "file_role_label": "config_canonical", "summary": "codrax.yaml is the canonical config layer"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 || emitted[0].Scope != types.ScopeFile {
		t.Fatalf("expected 1 ScopeFile item; got %+v", emitted)
	}
	if emitted[0].FileRoleLabel != types.FileRoleConfigCanonical {
		t.Errorf("FileRoleLabel = %q, want %q", emitted[0].FileRoleLabel, types.FileRoleConfigCanonical)
	}
	if emitted[0].LineStart != 0 {
		t.Errorf("ScopeFile must keep LineStart=0; got %d", emitted[0].LineStart)
	}
}

// TestEmitEvidence_AcceptsScopeNegative pins Negative-scope emission.
// Requires kind=absent + NegativeQuery + NegativeScope.
func TestEmitEvidence_AcceptsScopeNegative(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "negative", "kind": "absent", "source": "codrax.yaml", "negative_query": {"file": "codrax.yaml", "pattern": "explore_mid_loop_hint_budget"}, "negative_scope": "file", "summary": "key not in codrax.yaml"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 || emitted[0].Scope != types.ScopeNegative {
		t.Fatalf("expected 1 ScopeNegative item; got %+v", emitted)
	}
	if emitted[0].Kind != types.EvidenceAbsent {
		t.Errorf("Kind = %q, want %q", emitted[0].Kind, types.EvidenceAbsent)
	}
	if emitted[0].NegativeQuery == nil || emitted[0].NegativeQuery.Pattern != "explore_mid_loop_hint_budget" {
		t.Errorf("NegativeQuery missing or wrong pattern: %+v", emitted[0].NegativeQuery)
	}
}

// TestEmitEvidence_AcceptsScopeCrossfile pins Crossfile-scope.
// Requires CrossfileQuery (files+pattern) and CrossfileAssertion.
func TestEmitEvidence_AcceptsScopeCrossfile(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "crossfile", "kind": "direct",
           "crossfile_query": {"files": ["cmd/root.go"], "pattern": "flag\\..*Explore"},
           "crossfile_assertion": {"kind": "forbidden"},
           "summary": "no CLI flag for explore_*"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 || emitted[0].Scope != types.ScopeCrossfile {
		t.Fatalf("expected 1 ScopeCrossfile item; got %+v", emitted)
	}
	if emitted[0].CrossfileQuery == nil || emitted[0].CrossfileAssertion == nil {
		t.Errorf("CrossfileQuery / Assertion missing on emitted item")
	}
}

// TestEmitEvidence_AcceptsScopeSection pins Section-scope. Requires
// SectionPath; LineStart/End optional.
func TestEmitEvidence_AcceptsScopeSection(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "section", "kind": "direct", "source": "codrax.yaml", "section_path": "explore_*", "summary": "explore_* group"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 || emitted[0].Scope != types.ScopeSection {
		t.Fatalf("expected 1 ScopeSection item; got %+v", emitted)
	}
	if emitted[0].SectionPath != "explore_*" {
		t.Errorf("SectionPath = %q, want %q", emitted[0].SectionPath, "explore_*")
	}
}

// TestEmitEvidence_AcceptsScopeLineRange pins LineRange. Requires
// line_end > line_start.
func TestEmitEvidence_AcceptsScopeLineRange(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "line_range", "kind": "direct", "source": "a.go", "line_start": 10, "line_end": 50, "summary": "struct definition block"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 || emitted[0].Scope != types.ScopeLineRange {
		t.Fatalf("expected 1 ScopeLineRange item; got %+v", emitted)
	}
	if emitted[0].LineEnd != 50 {
		t.Errorf("LineEnd = %d, want 50", emitted[0].LineEnd)
	}
}

// TestEmitEvidence_RejectsScopeMissingRequiredField confirms per-scope
// required fields are enforced.
func TestEmitEvidence_RejectsScopeMissingRequiredField(t *testing.T) {
	tool := &EmitEvidence{}
	cases := []struct {
		name string
		json string
		hint string
	}{
		{"file scope missing role label", `{"items": [{"scope": "file", "kind": "direct", "source": "a.yaml"}]}`, "file_role_label"},
		{"section scope missing path", `{"items": [{"scope": "section", "kind": "direct", "source": "a.go"}]}`, "section_path"},
		{"crossfile missing query", `{"items": [{"scope": "crossfile", "kind": "direct"}]}`, "crossfile_query"},
		{"negative missing query", `{"items": [{"scope": "negative", "kind": "absent", "source": "a.yaml", "negative_scope": "file"}]}`, "negative_query"},
		// LineRange with line_end == line_start is degenerate (single
		// line, should use ScopeLine). The auto-swap heuristic fixes
		// `5,10` → `5,10`. This case uses identical values to confirm
		// strict line_range > line_start invariant rejects degenerate.
		{"line_range needs line_end > line_start", `{"items": [{"scope": "line_range", "kind": "direct", "source": "a.go", "line_start": 10, "line_end": 10, "anchor_kind": "definition", "anchor_symbol": "X"}]}`, "line_end"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := newEmitCtx()
			res, _ := tool.Execute(ctx, json.RawMessage(c.json))
			if res.Success {
				t.Errorf("expected rejection for case %q; res.Summary=%q", c.name, res.Summary)
				return
			}
			if !strings.Contains(strings.ToLower(res.Summary), c.hint) {
				t.Errorf("rejection should mention %q; got: %s", c.hint, res.Summary)
			}
		})
	}
}

// TestEmitEvidence_AutoPairsRoleDescriptionFromLeadingDocComment is the
// Plan 2 v2 (2026-05-05) integration test for the deterministic
// role-description pairing hook. Setup: a Go source whose definition
// at line 5 has a 2-line `//` doc comment immediately above. The LLM
// emits one definition-anchor evidence item; the system MUST auto-pair
// a parallel evidence_kind=mechanism item carrying the doc comment
// text as Summary.
func TestEmitEvidence_AutoPairsRoleDescriptionFromLeadingDocComment(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// Seed read_file gutter with a Go file whose def line is 5 and
	// the two lines above are //-comments.
	seedReadFileHistory(ctx, "internal/agent/foo.go", 1,
		"package foo",
		"",
		"// Foo coordinates the storm pipeline by deciding when each",
		"// stage emits and persisting the rolling baseline.",
		"func Foo() {",
		"    return",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Foo", "source": "internal/agent/foo.go", "line_start": 5, "summary": "Foo is defined here", "anchor_kind": "definition", "anchor_symbol": "Foo"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("want 2 items (1 manual + 1 auto-paired), got %d", len(got))
	}
	var paired *types.EvidenceItem
	for i := range got {
		if got[i].Producer == "auto_pair_role_description" {
			paired = &got[i]
			break
		}
	}
	if paired == nil {
		t.Fatalf("auto-paired mechanism item missing; producers seen: %s", producersOf(got))
	}
	if paired.Kind != types.EvidenceMechanism {
		t.Errorf("auto-paired item kind = %q, want mechanism", paired.Kind)
	}
	if !strings.Contains(paired.Summary, "Foo coordinates the storm pipeline") {
		t.Errorf("auto-paired Summary missing doc-comment text: %q", paired.Summary)
	}
	if paired.LineStart != 3 {
		t.Errorf("auto-paired LineStart = %d, want 3 (first comment line)", paired.LineStart)
	}
	if len(paired.DerivedFrom) != 1 || paired.DerivedFrom[0] != got[0].ID {
		t.Errorf("auto-paired DerivedFrom = %v, want [%q]", paired.DerivedFrom, got[0].ID)
	}
	stats := ctx.Mutable.EvidenceClosure().Stats()
	if stats.AutoPairedRoleDescriptions != 1 {
		t.Errorf("AutoPairedRoleDescriptions counter = %d, want 1", stats.AutoPairedRoleDescriptions)
	}
}

func TestEmitEvidence_AutoPairsRoleDescriptionAcrossDecoratorStack(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", 1,
		"// Source: developer.huawei.com / openharmony Index.ets minimal sample",
		"// Surface: @Entry + @Component + build()",
		"",
		"@Entry",
		"@Component",
		"struct Index {",
		"  build() {}",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Index", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", "line_start": 6, "summary": "Index is defined here", "anchor_kind": "definition", "anchor_symbol": "Index"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	var paired *types.EvidenceItem
	for i := range got {
		if got[i].Producer == "auto_pair_role_description" {
			paired = &got[i]
			break
		}
	}
	if paired == nil {
		t.Fatalf("auto-paired mechanism item missing; producers seen: %s", producersOf(got))
	}
	if !strings.Contains(paired.Summary, "Index.ets minimal sample") {
		t.Fatalf("auto-paired Summary missing decorated-file header: %q", paired.Summary)
	}
	if paired.LineStart != 1 {
		t.Fatalf("auto-paired LineStart = %d, want 1", paired.LineStart)
	}
}

// TestEmitEvidence_DoesNotAutoPairWhenManualMechanismExists guards the
// dedup rule: if the LLM already supplied a mechanism evidence at the
// same (source, line_start), the auto-pair MUST stand down so the
// authored explanation remains canonical.
func TestEmitEvidence_DoesNotAutoPairWhenManualMechanismExists(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/foo.go", 1,
		"package foo",
		"",
		"// Foo runs the storm pipeline and persists the baseline.",
		"func Foo() {",
		"    return",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Foo", "source": "internal/agent/foo.go", "line_start": 4, "summary": "Foo is defined here", "anchor_kind": "definition", "anchor_symbol": "Foo"},
          {"kind": "mechanism", "subject": "Foo", "predicate": "documents", "summary": "Foo handles the storm pipeline (LLM authored).", "source": "internal/agent/foo.go", "line_start": 4, "anchor_kind": "definition", "anchor_symbol": "Foo"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("execute failed: err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	for _, it := range got {
		if it.Producer == "auto_pair_role_description" {
			t.Errorf("auto-pair should stand down when manual mechanism present at same anchor; got auto-paired item %q", it.Summary)
		}
	}
}

// TestEmitEvidence_AutoPairSkipsWhenSourceUnread exercises the safety
// rail: when the source's lines around lineStart are NOT in the
// read_file gutter, the extractor must decline rather than risk a
// false-positive comment from an unread region.
func TestEmitEvidence_AutoPairSkipsWhenSourceUnread(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// No seedReadFileHistory call → gc.LineIndex is empty.
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Foo", "source": "internal/agent/foo.go", "line_start": 5, "summary": "Foo is defined here", "anchor_kind": "definition", "anchor_symbol": "Foo"}
        ]
    }`)
	_, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := ctx.Mutable.EmittedEvidence()
	for _, it := range got {
		if it.Producer == "auto_pair_role_description" {
			t.Errorf("auto-pair must not fire without read_file coverage; got %+v", it)
		}
	}
}

func producersOf(items []types.EvidenceItem) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = it.Producer
	}
	return strings.Join(parts, ",")
}

func TestEmitEvidence_ToolSurface(t *testing.T) {
	tool := &EmitEvidence{}
	if tool.Name() != "emit_evidence" {
		t.Errorf("name = %q", tool.Name())
	}
	if tool.IsWrite() {
		t.Errorf("emit_evidence must not be write-classified")
	}
	if tool.Confidence() != 0 {
		t.Errorf("confidence = %f, want 0 (NonEvidenceTool — the tool itself isn't a fact source)", tool.Confidence())
	}
	if !strings.Contains(tool.Description(), "emit_evidence") &&
		!strings.Contains(tool.Description(), "structured evidence") {
		// loose check; the description must guide the LLM
		t.Errorf("description should explain the tool's purpose")
	}
	if !json.Valid(tool.Parameters()) {
		t.Errorf("parameters JSON schema is invalid")
	}
}

// TestFindCallRelationAtLineForCandidates_SinglePass confirms the
// post-2026-05-07 refactor: the helper now walks fi.Relations once
// (set-based candidate matching) instead of K passes (one per
// candidate). Behaviour-equivalent to the old per-candidate loop.
func TestFindCallRelationAtLineForCandidates_SinglePass(t *testing.T) {
	fi := &repomap.FileInfo{
		Relations: []repomap.Relation{
			{Kind: "call", Line: 42, ToEP: repomap.RelationEndpoint{Name: "Bar"}},
			{Kind: "call", Line: 42, ToEP: repomap.RelationEndpoint{Name: "Baz"}},
			{Kind: "call", Line: 50, ToEP: repomap.RelationEndpoint{Name: "Qux"}},
			{Kind: "import", Line: 42, ToEP: repomap.RelationEndpoint{Name: "fmt"}},
		},
	}
	cases := []struct {
		name       string
		candidates []string
		line       int
		wantOK     bool
		wantTarget string
	}{
		{name: "first candidate hits", candidates: []string{"Bar", "X"}, line: 42, wantOK: true, wantTarget: "Bar"},
		{name: "second candidate hits", candidates: []string{"X", "Baz"}, line: 42, wantOK: true, wantTarget: "Baz"},
		{name: "qualified candidate matches short", candidates: []string{"pkg.Bar"}, line: 42, wantOK: true, wantTarget: "Bar"},
		{name: "no candidate match does not substitute another call at line", candidates: []string{"Nope"}, line: 42, wantOK: false},
		{name: "wrong line returns false", candidates: []string{"Bar"}, line: 999, wantOK: false},
		{name: "empty candidates falls through to anchor=\"\" path", candidates: nil, line: 42, wantOK: true, wantTarget: "Bar"},
		{name: "all-whitespace candidates → fallback", candidates: []string{"", "  "}, line: 42, wantOK: true, wantTarget: "Bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rel, ok := findCallRelationAtLineForCandidates(fi, tc.line, tc.candidates)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if rel.ToEP.Name != tc.wantTarget {
				t.Errorf("target=%q want %q", rel.ToEP.Name, tc.wantTarget)
			}
		})
	}
}

func TestNormalizeCallEvidenceDirection_UsesSourceLineCallTargetBeforeRelationFallback(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 1818,
		"out := compiler.Compile(rm, sig)",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/analyzer.go": {
				RelPath: "internal/agent/analyzer.go",
				Symbols: []repomap.Symbol{
					{Name: "buildAnalysisIR", Kind: "function", Line: 1800, EndLine: 1900},
				},
				Relations: []repomap.Relation{
					{Kind: "call", Line: 1818, ToEP: repomap.RelationEndpoint{Name: "RecomputeBudget"}},
				},
			},
		},
	})
	params := json.RawMessage(`{
		"items": [
			{
				"evidence_kind": "direct",
				"subject": "buildAnalysisIR",
				"predicate": "calls",
				"object": "Compile",
				"source": "internal/agent/analyzer.go",
				"line_start": 1818,
				"summary": "buildAnalysisIR calls compiler.Compile",
				"anchor_kind": "call",
				"anchor_symbol": "Compile"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].Object != "compiler.Compile" {
		t.Fatalf("call target = %q, want compiler.Compile", got[0].Object)
	}
}

func TestSourceLineCallTargetForCandidates_MatchesCrossLanguageSelectors(t *testing.T) {
	cases := []struct {
		name       string
		line       string
		candidate  string
		wantTarget string
	}{
		{name: "go selector", line: "out := compiler.Compile(rm, sig)", candidate: "Compile", wantTarget: "compiler.Compile"},
		{name: "arkts member", line: "this.router.pushUrl({ url: 'pages/Index' })", candidate: "pushUrl", wantTarget: "this.router.pushUrl"},
		{name: "cpp namespace", line: "auto ok = analysis::compiler::Compile(ir);", candidate: "Compile", wantTarget: "analysis::compiler::Compile"},
		{name: "cangjie-style module selector", line: "let graph = normalizer.Normalize(request)", candidate: "Normalize", wantTarget: "normalizer.Normalize"},
		{name: "ts generic", line: "const view = factory.create<ViewModel>(state)", candidate: "create", wantTarget: "factory.create"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ""
			for _, target := range sourceLineCallTargets(tc.line) {
				if callExpressionTargetMatchesCandidate(target, tc.candidate) {
					got = target
					break
				}
			}
			if got != tc.wantTarget {
				t.Fatalf("target = %q, want %q", got, tc.wantTarget)
			}
		})
	}
}
