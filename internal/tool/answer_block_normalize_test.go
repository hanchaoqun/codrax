package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// flexIntPtr builds the optional wire citation_ref pointer for tests.
func flexIntPtr(v int) *FlexInt {
	f := FlexInt(v)
	return &f
}

// An ABSENT or null citation_ref means the item is uncited and MUST
// decode to types.CitationRefUnset (-1), not index 0. Regression for
// the false-grounding bug where an omitted citation_ref collapsed to 0
// and glued citations[0] onto an intentionally-uncited item. Driven
// through the real JSON -> wire-struct -> typed decode so the
// omitempty+pointer behaviour is exercised end to end.
func TestNormalizeEmitAnswerBlock_CitationRefAbsentIsUnset(t *testing.T) {
	cases := []struct {
		name    string
		itemRaw string
		want    int
	}{
		{"absent", `{"id":"i1","label":"L"}`, types.CitationRefUnset},
		{"null", `{"id":"i1","label":"L","citation_ref":null}`, types.CitationRefUnset},
		{"explicit_zero", `{"id":"i1","label":"L","citation_ref":0}`, 0},
		{"explicit_one", `{"id":"i1","label":"L","citation_ref":1}`, 1},
		{"string_zero", `{"id":"i1","label":"L","citation_ref":"0"}`, 0},
		{"explicit_unset", `{"id":"i1","label":"L","citation_ref":-1}`, types.CitationRefUnset},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"id":"b1","kind":"ordered_list","items":[` + tc.itemRaw + `]}`
			var wire emitAnswerBlockV2
			if err := json.Unmarshal([]byte(raw), &wire); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			blk, err := NormalizeEmitAnswerBlock(wire, "blocks[0]")
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			if len(blk.Items) != 1 {
				t.Fatalf("want 1 item, got %d", len(blk.Items))
			}
			if blk.Items[0].CitationRef != tc.want {
				t.Errorf("citation_ref: got %d, want %d", blk.Items[0].CitationRef, tc.want)
			}
		})
	}
}

// G2 (post_v2_runtime_gap_remediation, 2026-05-04). NormalizeEmitAnswerBlock
// is the single-source converter from emitAnswerBlockV2 to
// types.AnswerBlock. The tests here lock:
//   - happy-path field propagation (every typed field on the JSON
//     shape lands on the typed result)
//   - error wording for the per-block validation failures (id, kind,
//     surface_role, diagram body)
//   - field-path prefixing so full-emit and patch-emit produce
//     comparable error sites
//   - the EdgeAnchors regression — pre-G2 the patch path silently
//     dropped this field, this test fails immediately if the helper
//     ever reverts to omitting it.

func TestNormalizeEmitAnswerBlock_HappyPathFullProjection(t *testing.T) {
	in := emitAnswerBlockV2{
		ID:    "b1",
		Kind:  string(types.BlockSummary),
		Title: "Title",
		Text:  "body text",
		Items: []emitAnswerBlockItemV2{
			{ID: "i1", Label: "L", Text: "T", CandidateRole: string(types.AnswerCandidateRoleVariable), SourceInventoryRowID: "row-1", CitationRef: flexIntPtr(3)},
		},
		ClaimUses: []types.RenderedClaimUse{
			{ClaimForm: types.ClaimDefinitionFact, FacetID: "f1"},
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
		RelationClaims: []types.AnswerRelationClaim{{
			AuthorityID: "trace_projection:1:self_runnable_two_ruler:1:cross",
			MemberRefs:  []string{"#4", "#10"}, PhysicalRelation: types.AnswerPhysicalRelationUnresolved,
			Addition: types.AnswerRelationAdditionForbidden,
		}},
		FacetIDs:    []string{"f1", "f2"},
		SurfaceRole: string(types.SurfacePrincipal),
	}
	got, err := NormalizeEmitAnswerBlock(in, "blocks[0]")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.ID != "b1" || got.Kind != types.BlockSummary || got.Title != "Title" || got.Text != "body text" {
		t.Errorf("scalar fields lost: %+v", got)
	}
	if len(got.Items) != 1 || got.Items[0].CitationRef != 3 || got.Items[0].CandidateRole != types.AnswerCandidateRoleVariable || got.Items[0].SourceInventoryRowID != "row-1" {
		t.Errorf("items[0] fields lost: %+v", got.Items)
	}
	if len(got.ClaimUses) != 1 || got.ClaimUses[0].FacetID != "f1" {
		t.Errorf("claim_uses lost: %+v", got.ClaimUses)
	}
	if len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].FromNode != "A" {
		t.Errorf("EdgeAnchors lost: %+v", got.EdgeAnchors)
	}
	if got.SurfaceRole != types.SurfacePrincipal {
		t.Errorf("surface_role lost: %v", got.SurfaceRole)
	}
}

func TestNormalizeEmitAnswerBlock_RejectsDeclaredMultiColumnRowWithoutValues(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:      "entries",
		Kind:    string(types.BlockTable),
		Columns: []string{"文件路径", "函数名"},
		Items: []emitAnswerBlockItemV2{{
			ID:    "index",
			Label: "Index",
		}},
	}, "blocks[2]")
	if err == nil {
		t.Fatal("a two-column row with only one visible value must be rejected")
	}
	for _, want := range []string{
		"blocks[2].items[0]",
		"2 column header",
		"Preferred repair: omit item.label and item.text",
		"exactly one cells[] value per columns[] entry",
		"one canonical repair shape",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func TestNormalizeEmitAnswerBlock_RejectsTableWithoutVisibleRows(t *testing.T) {
	for _, raw := range []emitAnswerBlockV2{
		{
			ID:      "headers-only",
			Kind:    string(types.BlockTable),
			Columns: []string{"Stage", "输入", "输出", "主要状态载体"},
		},
		{
			ID:   "intro-only",
			Kind: string(types.BlockTable),
			Text: "各阶段输入、输出与状态载体如下。",
			Items: []emitAnswerBlockItemV2{{
				ID: "citation-sidecar", CitationRef: flexIntPtr(0),
			}},
		},
	} {
		_, err := NormalizeEmitAnswerBlock(raw, "blocks[6]")
		if err == nil {
			t.Fatalf("table %s without a visible row must be rejected", raw.ID)
		}
		for _, want := range []string{"blocks[6]", "kind=table has no visible rows", "items[] row"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("table %s error missing %q: %v", raw.ID, want, err)
			}
		}
	}
}

func TestNormalizeEmitAnswerBlock_AcceptsEachVisibleTableCarrier(t *testing.T) {
	for _, raw := range []emitAnswerBlockV2{
		{
			ID:   "markdown",
			Kind: string(types.BlockTable),
			Text: "| Stage | 输入 |\n|---|---|\n| analyze | request |",
		},
		{
			ID:   "fallback-row",
			Kind: string(types.BlockTable),
			Items: []emitAnswerBlockItemV2{{
				ID: "analyze", Label: "analyze", Text: "request -> AnalysisIR",
			}},
		},
		{
			ID:      "structured-row",
			Kind:    string(types.BlockTable),
			Columns: []string{"Stage", "输入"},
			Items: []emitAnswerBlockItemV2{{
				ID: "analyze", Cells: []string{"analyze", "request"},
			}},
		},
	} {
		if _, err := NormalizeEmitAnswerBlock(raw, "blocks[0]"); err != nil {
			t.Fatalf("visible table carrier %s was rejected: %v", raw.ID, err)
		}
	}
}

func TestNormalizeEmitAnswerBlock_AcceptsBothStructuredTableRowConventions(t *testing.T) {
	for _, raw := range []emitAnswerBlockV2{
		{
			ID:      "cell-only",
			Kind:    string(types.BlockTable),
			Columns: []string{"文件路径", "函数名"},
			Items:   []emitAnswerBlockItemV2{{ID: "index", Cells: []string{"src/Index.ets", "Index"}}},
		},
		{
			ID:      "label-first",
			Kind:    string(types.BlockTable),
			Columns: []string{"函数名", "文件路径"},
			Items:   []emitAnswerBlockItemV2{{ID: "index", Label: "Index", Cells: []string{"src/Index.ets"}}},
		},
		{
			ID:      "synthetic-label-header",
			Kind:    string(types.BlockTable),
			Columns: []string{"文件路径"},
			Items:   []emitAnswerBlockItemV2{{ID: "index", Label: "Index", Cells: []string{"src/Index.ets"}}},
		},
	} {
		if _, err := NormalizeEmitAnswerBlock(raw, "blocks[0]"); err != nil {
			t.Fatalf("valid structured convention rejected for %s: %v", raw.ID, err)
		}
	}
}

func TestNormalizeEmitAnswerBlock_RejectsMixedStructuredTableRowConventions(t *testing.T) {
	raw := emitAnswerBlockV2{
		ID:      "mixed-label-width",
		Kind:    string(types.BlockTable),
		Columns: []string{"Stage", "输入", "输出", "Agent", "调用位置"},
		Items: []emitAnswerBlockItemV2{
			{ID: "analyze", Label: "analyze", Cells: []string{"analyze", "request", "AnalysisIR", "Analyzer", "analyzer.go:1"}},
			{ID: "finalize", Label: "finalize", Cells: []string{"AnswerSymbols", "AnswerDocument", "Finalizer", "finalizer.go:1"}},
		},
	}
	_, err := NormalizeEmitAnswerBlock(raw, "blocks[3]")
	if err == nil {
		t.Fatal("mixed structured table row conventions must be rejected")
	}
	for _, want := range []string{"blocks[3].items[1]", "mixes row conventions", "one table-wide row shape"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("mixed convention error missing %q: %v", want, err)
		}
	}
}

func TestNormalizeEmitAnswerBlock_RejectsMixedLabelAndCellOnlyRows(t *testing.T) {
	raw := emitAnswerBlockV2{
		ID:      "mixed-label-presence",
		Kind:    string(types.BlockTable),
		Columns: []string{"Stage", "输入"},
		Items: []emitAnswerBlockItemV2{
			{ID: "analyze", Label: "analyze", Cells: []string{"request"}},
			{ID: "explore", Cells: []string{"explore", "AnalysisIR"}},
		},
	}
	_, err := NormalizeEmitAnswerBlock(raw, "blocks[4]")
	if err == nil {
		t.Fatal("mixing label-first and cell-only rows must be rejected")
	}
	for _, want := range []string{"blocks[4].items[1]", "mixes row conventions", "one table-wide row shape"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("mixed label/cell error missing %q: %v", want, err)
		}
	}
}

func TestNormalizeEmitAnswerBlock_TableRetryTeachesOneCanonicalCellsShape(t *testing.T) {
	raw := emitAnswerBlockV2{
		ID:      "ranked-trace-causes",
		Kind:    string(types.BlockTable),
		Columns: []string{"排序", "链上根因", "状态", "耗时", "可消除量", "修向"},
		Items: []emitAnswerBlockItemV2{{
			ID:    "cause-1",
			Label: "供给缺口主导",
			Text:  "保持链上证据与背景证据分栏",
			Cells: []string{"#1", "算力供给不足", "running", "74.915ms", "65.912ms", "检查频率上限"},
		}},
	}
	_, err := NormalizeEmitAnswerBlock(raw, "blocks[2]")
	if err == nil {
		t.Fatal("label/text plus a complete cells row must request a canonical repair")
	}
	for _, want := range []string{
		"blocks[2].items[0]", "cells[] already supplies exactly 6 values", "Keep cells[] unchanged",
		"omit both item.label and item.text", "separate non-table block", "Do not add a column or rebuild other rows",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("canonical table repair missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "legacy alternative") {
		t.Fatalf("retry guidance must not teach competing row conventions: %v", err)
	}
}

// G2 regression lock — pre-G2 the patch path's
// convertEmitBlocksToTyped silently dropped EdgeAnchors. Single-source
// normalizer fixes this. Lock it here so it cannot regress.
func TestNormalizeEmitAnswerBlock_EdgeAnchorsRegressionLock(t *testing.T) {
	in := emitAnswerBlockV2{
		ID:   "b1",
		Kind: string(types.BlockDiagram),
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramFlow),
			Body: "flowchart LR\n  A --> B",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}
	got, err := NormalizeEmitAnswerBlock(in, "replace_blocks[0]")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got.EdgeAnchors) != 1 {
		t.Fatalf("EdgeAnchors must propagate (G2 patch-path bug regression lock): got %+v", got.EdgeAnchors)
	}
	if got.EdgeAnchors[0].RelationKind != types.DiagramRelCall {
		t.Errorf("RelationKind lost: %+v", got.EdgeAnchors[0])
	}
}

func TestNormalizeEmitAnswerBlock_ErrorGranularityVerdictDecisionOnly(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:                      "d1",
		Kind:                    string(types.BlockDecision),
		Text:                    "The bad record is rejected while siblings continue.",
		ErrorGranularityVerdict: string(types.ErrorGranularityPerItemRejection),
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.ErrorGranularityVerdict != types.ErrorGranularityPerItemRejection {
		t.Fatalf("error_granularity_verdict lost: %+v", got)
	}
}

func TestNormalizeEmitAnswerBlock_CurrentStatusVerdictDecisionOnly(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:                   "d1",
		Kind:                 string(types.BlockDecision),
		Text:                 "The current checkout no longer has the observed path.",
		CurrentStatusVerdict: string(types.CurrentStatusFixed),
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.CurrentStatusVerdict != types.CurrentStatusFixed {
		t.Fatalf("current_status_verdict lost: %+v", got)
	}
}

func TestNormalizeEmitAnswerBlock_CaveatAliasFillsTextOnCaveatBlock(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:     "c1",
		Kind:   string(types.BlockCaveat),
		Caveat: "Evidence is limited to the inspected files.",
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Text != "Evidence is limited to the inspected files." {
		t.Fatalf("caveat alias did not populate text: %+v", got)
	}
}

func TestNormalizeEmitAnswerBlock_CaveatAliasRejectedOnNonCaveatBlock(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:     "s1",
		Kind:   string(types.BlockSummary),
		Text:   "summary",
		Caveat: "not a caveat block",
	}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject caveat alias on non-caveat blocks")
	}
	if !strings.Contains(err.Error(), "kind=caveat") {
		t.Fatalf("err should explain caveat-only alias, got %q", err.Error())
	}
}

func TestNormalizeEmitAnswerBlock_RejectsEmptyID(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{Kind: string(types.BlockSummary)}, "blocks[2]")
	if err == nil {
		t.Fatal("must reject empty id")
	}
	if !strings.Contains(err.Error(), "blocks[2]") {
		t.Errorf("err must include fieldPath; got %q", err.Error())
	}
}

func TestNormalizeEmitAnswerBlock_RejectsInvalidKind(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{ID: "b1", Kind: "bogus"}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject bogus kind")
	}
	// R4: error must NOT contain Go-internal type name.
	if strings.Contains(err.Error(), "AnswerBlockKind") {
		t.Errorf("R4 violation: err leaks Go type name: %q", err.Error())
	}
	// MUST name the bad value + allowed list.
	if !strings.Contains(err.Error(), `"bogus"`) {
		t.Errorf("err must name bad value verbatim: %q", err.Error())
	}
}

func TestNormalizeEmitAnswerBlock_RejectsInvalidSurfaceRole(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID: "b1", Kind: string(types.BlockSummary), SurfaceRole: "garbage",
	}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject bogus surface_role")
	}
	// R4 cleanup: surface role error uses lowercase contract phrasing.
	if strings.Contains(err.Error(), "SurfaceRole") {
		t.Errorf("R4 violation: err leaks Go type name: %q", err.Error())
	}
}

func TestNormalizeEmitAnswerBlock_RejectsInvalidErrorGranularityVerdict(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:                      "d1",
		Kind:                    string(types.BlockDecision),
		ErrorGranularityVerdict: "maybe_itemish",
	}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject invalid error_granularity_verdict")
	}
	if !strings.Contains(err.Error(), "error_granularity_verdict") {
		t.Errorf("err should name error_granularity_verdict, got %q", err.Error())
	}
}

func TestNormalizeEmitAnswerBlock_RejectsErrorGranularityVerdictOnNonDecision(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:                      "s1",
		Kind:                    string(types.BlockSummary),
		ErrorGranularityVerdict: string(types.ErrorGranularityPerItemRejection),
	}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject error_granularity_verdict on non-decision block")
	}
	if !strings.Contains(err.Error(), "only valid on kind=decision") {
		t.Errorf("err should explain decision-only field, got %q", err.Error())
	}
}

func TestNormalizeEmitAnswerBlock_RejectsInvalidCurrentStatusVerdict(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:                   "d1",
		Kind:                 string(types.BlockDecision),
		CurrentStatusVerdict: "maybe_fixed",
	}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject invalid current_status_verdict")
	}
	if !strings.Contains(err.Error(), "current_status_verdict") {
		t.Errorf("err should name current_status_verdict, got %q", err.Error())
	}
}

func TestNormalizeEmitAnswerBlock_RejectsCurrentStatusVerdictOnNonDecision(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:                   "s1",
		Kind:                 string(types.BlockSummary),
		CurrentStatusVerdict: string(types.CurrentStatusFixed),
	}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject current_status_verdict on non-decision block")
	}
	if !strings.Contains(err.Error(), "only valid on kind=decision") {
		t.Errorf("err should explain decision-only field, got %q", err.Error())
	}
}

func TestNormalizeEmitAnswerBlock_RepairsInvalidCandidateRoleToOther(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID: "b1", Kind: string(types.BlockOrderedList),
		Items: []emitAnswerBlockItemV2{{ID: "i1", CandidateRole: "garbage"}},
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("invalid optional candidate_role should be repaired, not rejected: %v", err)
	}
	if got.Items[0].CandidateRole != types.AnswerCandidateRoleOther {
		t.Fatalf("candidate_role = %q, want other", got.Items[0].CandidateRole)
	}
}

func TestNormalizeEmitAnswerBlock_NormalizesCandidateRoleAlias(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID: "b1", Kind: string(types.BlockOrderedList),
		Items: []emitAnswerBlockItemV2{{ID: "i1", CandidateRole: "interface"}},
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("interface alias should normalize instead of reject: %v", err)
	}
	if got.Items[0].CandidateRole != types.AnswerCandidateRoleType {
		t.Fatalf("candidate_role = %q, want type", got.Items[0].CandidateRole)
	}
}

func TestNormalizeEmitAnswerBlock_DiagramBodyRequired(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID: "b1", Kind: string(types.BlockDiagram),
		Diagram: &emitAnswerDiagramV2{Kind: string(types.DiagramFlow), Body: "  "},
	}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject empty diagram body")
	}
}

func TestNormalizeEmitAnswerBlock_DiagramKindRequiresPayload(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID: "b1", Kind: string(types.BlockDiagram),
		// No Diagram payload.
	}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject kind=diagram without diagram payload")
	}
}

func TestNormalizeEmitAnswerBlock_DiagramPayloadNormalizesDiagramKind(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:   "b1",
		Kind: string(types.BlockSection),
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramFlow),
			Body: "flowchart TD\n  A --> B",
		},
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("diagram payload with stale kind should normalize instead of reject: %v", err)
	}
	if got.Kind != types.BlockDiagram {
		t.Fatalf("kind = %q, want diagram", got.Kind)
	}
	if got.Diagram == nil || !strings.Contains(got.Diagram.Body, "A --> B") {
		t.Fatalf("diagram payload should be preserved, got %+v", got.Diagram)
	}
}

func TestNormalizeEmitAnswerBlock_DoesNotInferDiagramKindFromText(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:   "b1",
		Kind: string(types.BlockSection),
		Text: "```mermaid\nflowchart TD\n  A --> B\n```",
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("section text should not be interpreted as a typed diagram: %v", err)
	}
	if got.Kind != types.BlockSection {
		t.Fatalf("kind = %q, want section", got.Kind)
	}
	if got.Diagram != nil {
		t.Fatalf("diagram should remain nil when only text contains Mermaid: %+v", got.Diagram)
	}
}

func TestNormalizeEmitAnswerBlock_NormalizesDiagramKindFromMermaidSyntax(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:   "b1",
		Kind: string(types.BlockDiagram),
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramArchitecture),
			Body: "```mermaid\nsequenceDiagram\n  User->>Agent: ask\n```",
		},
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if got.Diagram == nil {
		t.Fatal("diagram missing")
	}
	if got.Diagram.Kind != types.DiagramSequence {
		t.Fatalf("diagram kind = %q, want sequence", got.Diagram.Kind)
	}
	if strings.Contains(got.Diagram.Body, "```") || !strings.Contains(got.Diagram.Body, "sequenceDiagram") {
		t.Fatalf("diagram body should be raw fenced content, got %q", got.Diagram.Body)
	}
	if got.Diagram.Language != "mermaid" {
		t.Fatalf("diagram language = %q, want mermaid", got.Diagram.Language)
	}
}

func TestNormalizeEmitAnswerBlock_ConvertsPortableClassDiagramWithoutChangingSemanticKind(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:   "types",
		Kind: string(types.BlockDiagram),
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramArchitecture),
			Body: strings.Join([]string{
				"classDiagram",
				"  class LoopController",
				"  class analyzerEvaluator",
				"  analyzerEvaluator ..|> LoopController",
			}, "\n"),
		},
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if got.Diagram == nil || got.Diagram.Kind != types.DiagramArchitecture {
		t.Fatalf("semantic architecture kind changed: %+v", got.Diagram)
	}
	for _, want := range []string{"flowchart TD", `analyzerEvaluator -->|"implements"| LoopController`} {
		if !strings.Contains(got.Diagram.Body, want) {
			t.Fatalf("converted body missing %q:\n%s", want, got.Diagram.Body)
		}
	}
}

func TestNormalizeEmitAnswerBlock_AlignsReverseTypeRelationAnchorWithClassDiagramSemantics(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:   "types",
		Kind: string(types.BlockDiagram),
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramArchitecture),
			Body: "classDiagram\n  LoopController <|.. analyzerEvaluator",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "LoopController", FromIdentity: "pkg.LoopController",
			ToNode: "analyzerEvaluator", ToIdentity: "pkg.analyzerEvaluator",
			RelationKind: types.DiagramRelTypeRelation,
		}},
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if len(got.EdgeAnchors) != 1 {
		t.Fatalf("edge anchors = %+v", got.EdgeAnchors)
	}
	anchor := got.EdgeAnchors[0]
	if anchor.FromNode != "analyzerEvaluator" || anchor.ToNode != "LoopController" ||
		anchor.FromIdentity != "pkg.analyzerEvaluator" || anchor.ToIdentity != "pkg.LoopController" {
		t.Fatalf("anchor direction not aligned with UML semantics: %+v", anchor)
	}
	if got.Diagram == nil || !strings.Contains(got.Diagram.Body, `analyzerEvaluator -->|"implements"| LoopController`) {
		t.Fatalf("converted body and anchor must share direction: %+v", got.Diagram)
	}
}

func TestNormalizeClassDiagramTypeRelationAnchorDirections_FailsOpenOutsideExactUniquePair(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		anchor   types.DiagramEdgeAnchor
		wantFrom string
	}{
		{
			name: "already semantic", body: "classDiagram\n  Base <|.. Impl",
			anchor:   types.DiagramEdgeAnchor{FromNode: "Impl", ToNode: "Base", RelationKind: types.DiagramRelTypeRelation},
			wantFrom: "Impl",
		},
		{
			name: "call relation", body: "classDiagram\n  Base <|.. Impl",
			anchor:   types.DiagramEdgeAnchor{FromNode: "Base", ToNode: "Impl", RelationKind: types.DiagramRelCall},
			wantFrom: "Base",
		},
		{
			name: "ambiguous duplicate", body: "classDiagram\n  Base <|.. Impl\n  Impl ..|> Base",
			anchor:   types.DiagramEdgeAnchor{FromNode: "Base", ToNode: "Impl", RelationKind: types.DiagramRelTypeRelation},
			wantFrom: "Base",
		},
		{
			name: "one sided identity", body: "classDiagram\n  Base <|.. Impl",
			anchor:   types.DiagramEdgeAnchor{FromNode: "Base", ToNode: "Impl", FromIdentity: "pkg.Base", RelationKind: types.DiagramRelTypeRelation},
			wantFrom: "Base",
		},
		{
			name: "case is not guessed", body: "classDiagram\n  Base <|.. Impl",
			anchor:   types.DiagramEdgeAnchor{FromNode: "base", ToNode: "Impl", RelationKind: types.DiagramRelTypeRelation},
			wantFrom: "base",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeClassDiagramTypeRelationAnchorDirections(tc.body, []types.DiagramEdgeAnchor{tc.anchor})
			if len(got) != 1 || got[0].FromNode != tc.wantFrom {
				t.Fatalf("got %+v, want from=%q", got, tc.wantFrom)
			}
		})
	}
}

func TestNormalizeEmitAnswerBlock_NormalizesSequenceParticipantMessagePrefix(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:   "b1",
		Kind: string(types.BlockDiagram),
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramSequence),
			Body: strings.Join([]string{
				"sequenceDiagram",
				"    participant build as buildAnalysisIR",
				"    participant normalizer->>resolver: resolver",
				"    resolver->>build: done",
			}, "\n"),
		},
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if got.Diagram == nil {
		t.Fatal("diagram missing")
	}
	if strings.Contains(got.Diagram.Body, "participant normalizer->>resolver") {
		t.Fatalf("invalid participant-prefixed edge survived:\n%s", got.Diagram.Body)
	}
	if !strings.Contains(got.Diagram.Body, "    normalizer->>resolver: resolver") {
		t.Fatalf("edge was not preserved:\n%s", got.Diagram.Body)
	}
	if !strings.Contains(got.Diagram.Body, "participant build as buildAnalysisIR") {
		t.Fatalf("valid participant declaration changed:\n%s", got.Diagram.Body)
	}
}

func TestNormalizeEmitAnswerBlock_NormalizesFlowchartNodeLabel(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:   "b1",
		Kind: string(types.BlockDiagram),
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramFlow),
			Body: strings.Join([]string{
				"flowchart TD",
				`    PS[preStages: LogTriage, PerfTriage\n(Conditional)]`,
			}, "\n"),
		},
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if got.Diagram == nil {
		t.Fatal("diagram missing")
	}
	if !strings.Contains(got.Diagram.Body, `PS["preStages: LogTriage, PerfTriage<br/>(Conditional)"]`) {
		t.Fatalf("parser-sensitive flowchart node label was not normalized:\n%s", got.Diagram.Body)
	}
}

// TestNormalizeEmitAnswerBlock_AllFieldsPropagate uses reflection to
// verify every field on emitAnswerBlockV2 surfaces as a non-zero
// value on the resulting types.AnswerBlock when populated from a
// fully-filled fixture. This is the structural lock that prevents a
// future field addition from silently being dropped — the test fails
// until the new field is wired into NormalizeEmitAnswerBlock.
func TestNormalizeEmitAnswerBlock_AllFieldsPropagate(t *testing.T) {
	in := emitAnswerBlockV2{
		ID:                    "b1",
		Kind:                  string(types.BlockDiagram),
		Title:                 "Title",
		Text:                  "body text",
		SourceInventoryFamily: "public class",
		Columns:               []string{"维度", "结论"},
		Items: []emitAnswerBlockItemV2{
			{ID: "i1", Label: "L", Text: "T", Cells: []string{"C1", "C2"}, CandidateRole: string(types.AnswerCandidateRoleFunction), SourceInventoryRowID: "row-1", CitationRef: flexIntPtr(3)},
		},
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramFlow), Body: "flowchart LR\n  A --> B",
		},
		ClaimUses: []types.RenderedClaimUse{
			{ClaimForm: types.ClaimDefinitionFact, FacetID: "f1"},
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
		ParticipantBoundaries: []types.DiagramParticipantBoundary{{
			Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnproven,
		}},
		RelationClaims: []types.AnswerRelationClaim{{
			AuthorityID: "trace:self_runnable_two_ruler:test:cross",
			MemberRefs:  []string{"#4", "#10"}, PhysicalRelation: types.AnswerPhysicalRelationUnresolved,
			Addition: types.AnswerRelationAdditionForbidden,
		}},
		FacetIDs:    []string{"f1", "f2"},
		SurfaceRole: string(types.SurfacePrincipal),
	}
	got, err := NormalizeEmitAnswerBlock(in, "blocks[0]")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	decisionGot, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:                      "d1",
		Kind:                    string(types.BlockDecision),
		Text:                    "verdict",
		ErrorGranularityVerdict: string(types.ErrorGranularityWholeBatch),
		CurrentStatusVerdict:    string(types.CurrentStatusFixed),
	}, "blocks[1]")
	if err != nil {
		t.Fatalf("decision normalize failed: %v", err)
	}
	scopeGot, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:              "c1",
		Kind:            string(types.BlockCaveat),
		Text:            "active set excludes repo-tools-py",
		ScopeDisclosure: string(types.ScopeDisclosureInactiveScopeNamed),
	}, "blocks[2]")
	if err != nil {
		t.Fatalf("scope disclosure normalize failed: %v", err)
	}
	traceGot, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID:                      "trace-summary",
		Kind:                    string(types.BlockSummary),
		Text:                    "bounded candidate",
		TraceCausalClaimCaliber: string(types.TraceCausalClaimBoundedWindow),
	}, "blocks[3]")
	if err != nil {
		t.Fatalf("trace causal caliber normalize failed: %v", err)
	}
	// Per-field lock: every emitAnswerBlockV2 input field must surface
	// a corresponding non-zero typed field. When a new field is added
	// to emitAnswerBlockV2, fixturise it above AND extend this map.
	checks := map[string]func() bool{
		"ID":          func() bool { return got.ID != "" },
		"Kind":        func() bool { return got.Kind != "" },
		"Title":       func() bool { return got.Title != "" },
		"Text":        func() bool { return got.Text != "" },
		"Columns":     func() bool { return len(got.Columns) == 2 && got.Columns[0] == "维度" },
		"Items":       func() bool { return len(got.Items) > 0 },
		"Diagram":     func() bool { return got.Diagram != nil && got.Diagram.Body != "" },
		"ClaimUses":   func() bool { return len(got.ClaimUses) > 0 },
		"EdgeAnchors": func() bool { return len(got.EdgeAnchors) > 0 },
		"ParticipantBoundaries": func() bool {
			return len(got.ParticipantBoundaries) == 1 && got.ParticipantBoundaries[0].Participant == "MutableState"
		},
		"RelationClaims": func() bool { return len(got.RelationClaims) > 0 },
		"FacetIDs":       func() bool { return len(got.FacetIDs) > 0 },
		"SurfaceRole":    func() bool { return got.SurfaceRole != "" },
		"ErrorGranularityVerdict": func() bool {
			return decisionGot.ErrorGranularityVerdict == types.ErrorGranularityWholeBatch
		},
		"CurrentStatusVerdict": func() bool {
			return decisionGot.CurrentStatusVerdict == types.CurrentStatusFixed
		},
		"TraceCausalClaimCaliber": func() bool {
			return traceGot.TraceCausalClaimCaliber == types.TraceCausalClaimBoundedWindow
		},
		"ScopeDisclosure": func() bool {
			return scopeGot.ScopeDisclosure == types.ScopeDisclosureInactiveScopeNamed
		},
		"SourceInventoryFamily": func() bool {
			return got.SourceInventoryFamily == "public class"
		},
	}
	for name, check := range checks {
		if !check() {
			t.Errorf("field %q dropped by normalizer (got AnswerBlock = %+v)", name, got)
		}
	}
	if len(got.Items) != 1 || len(got.Items[0].Cells) != 2 || got.Items[0].Cells[1] != "C2" {
		t.Errorf("items[].cells dropped by normalizer; got %+v", got.Items)
	}
	if got.Items[0].SourceInventoryRowID != "row-1" {
		t.Errorf("items[].source_inventory_row_id dropped by normalizer; got %+v", got.Items)
	}

	// Sanity: the JSON shape and the typed shape must have field-name
	// agreement on the load-bearing fields. Reflection over the JSON
	// shape catches a future emitAnswerBlockV2 field rename.
	jsonShape := reflect.TypeOf(emitAnswerBlockV2{})
	typedShape := reflect.TypeOf(types.AnswerBlock{})
	for i := 0; i < jsonShape.NumField(); i++ {
		f := jsonShape.Field(i)
		// The JSON-shape has fields "Diagram" (pointer to emit shape)
		// and a few items[].* sub-fields that don't appear by name on
		// the typed AnswerBlock surface. Limit the reflection check to
		// fields whose names also exist on AnswerBlock.
		if _, ok := typedShape.FieldByName(f.Name); !ok {
			continue
		}
		if _, ok := checks[f.Name]; !ok {
			t.Errorf("emitAnswerBlockV2 field %q has a typed counterpart but is NOT covered by the propagation lock; add a fixture+check for it", f.Name)
		}
	}
}

func TestNormalizeEmitAnswerBlock_RejectsInvalidParticipantBoundaries(t *testing.T) {
	base := emitAnswerBlockV2{
		ID: "diagram", Kind: string(types.BlockDiagram),
		Diagram: &emitAnswerDiagramV2{Kind: string(types.DiagramFlow), Body: "flowchart LR\n M[MutableState]"},
	}
	for name, mutate := range map[string]func(*emitAnswerBlockV2){
		"non diagram": func(block *emitAnswerBlockV2) {
			block.Kind = string(types.BlockSummary)
			block.Diagram = nil
			block.ParticipantBoundaries = []types.DiagramParticipantBoundary{{Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnproven}}
		},
		"empty participant": func(block *emitAnswerBlockV2) {
			block.ParticipantBoundaries = []types.DiagramParticipantBoundary{{Status: types.DiagramParticipantBoundaryUnproven}}
		},
		"invalid status": func(block *emitAnswerBlockV2) {
			block.ParticipantBoundaries = []types.DiagramParticipantBoundary{{Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnknown}}
		},
		"duplicate": func(block *emitAnswerBlockV2) {
			block.ParticipantBoundaries = []types.DiagramParticipantBoundary{
				{Participant: "MutableState", Status: types.DiagramParticipantBoundaryUnproven},
				{Participant: "mutablestate", Status: types.DiagramParticipantBoundaryUnproven},
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			block := base
			mutate(&block)
			if _, err := NormalizeEmitAnswerBlock(block, "blocks[0]"); err == nil || !strings.Contains(err.Error(), "participant_boundaries") {
				t.Fatalf("invalid participant boundary must fail at its typed field, got %v", err)
			}
		})
	}
}

func TestNormalizeEmitAnswerBlock_QuotesSequenceParticipantDisplayLabelsWithoutChangingEdges(t *testing.T) {
	got, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID: "pipeline", Kind: string(types.BlockDiagram),
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramSequence), Language: "mermaid",
			Body: strings.Join([]string{
				"sequenceDiagram",
				"    participant Run as Orchestrator.Run",
				"    participant BusCtx as o.busCtx.AnalysisIR",
				"    Run->>BusCtx: read AnalysisIR",
			}, "\n"),
		},
	}, "blocks[0]")
	if err != nil {
		t.Fatalf("normalize sequence diagram: %v", err)
	}
	for _, want := range []string{
		`participant Run as "Orchestrator.Run"`,
		`participant BusCtx as "o.busCtx.AnalysisIR"`,
		`Run->>BusCtx: read AnalysisIR`,
	} {
		if got.Diagram == nil || !strings.Contains(got.Diagram.Body, want) {
			t.Fatalf("normalized diagram missing %q: %+v", want, got.Diagram)
		}
	}
}
