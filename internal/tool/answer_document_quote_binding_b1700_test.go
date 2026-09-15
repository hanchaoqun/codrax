package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A source-format example and an observed payload may contain the same text.
// Its appearance in prose is not the model's selection of a source citation.
func b1700QuoteBindingContext(t *testing.T) (*types.BusContext, types.Citation) {
	t.Helper()
	repo := t.TempDir()
	const source = "package wire\nconst WireSyntax = \"Q|sender|payload\"\n"
	if err := os.WriteFile(filepath.Join(repo, "wire.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("explain the observations and the format"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}}
	read, err := (&ReadFile{}).Execute(ctx, json.RawMessage(`{"path":"wire.go","limit":20}`))
	if err != nil || !read.Success || read.ReadCoverage == nil {
		t.Fatalf("real source read failed: %v %+v", err, read)
	}
	ctx.Mutable.AppendDispatchToolResult(read)
	return ctx, types.Citation{File: "wire.go", Line: 2, Quote: `const WireSyntax = "Q|sender|payload"`}
}

func b1700QuoteBindingBlock(forms []types.ClaimForm, explicit bool) map[string]any {
	item := map[string]any{"id": "observed", "label": "观测条目", "text": "The observed message used `Q|sender|payload`; its arrival was delayed."}
	if explicit {
		item["citation_ref"] = 0
	}
	block := map[string]any{"id": "observations", "kind": "ordered_list", "items": []any{item}}
	if len(forms) > 0 {
		uses := make([]types.RenderedClaimUse, len(forms))
		for i, form := range forms {
			uses[i].ClaimForm = form
		}
		block["claim_uses"] = uses
	}
	return block
}

func b1700QuoteBindingEmit(t *testing.T, ctx *types.BusContext, payload map[string]any, patch bool) *types.AnswerDocumentV2 {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), raw...)
	var result types.ToolResult
	if patch {
		result, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	} else {
		result, err = (&EmitAnswerDocument{}).Execute(ctx, raw)
	}
	if err != nil || !result.Success {
		t.Fatalf("actual emit failed: %v %+v", err, result)
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("model input changed")
	}
	return ctx.Mutable.AnswerDocumentV2()
}

func TestB1700ActualFullAndPatchDoNotInventCitationFromBodyQuote(t *testing.T) {
	for _, tc := range []struct {
		name  string
		forms []types.ClaimForm
	}{
		{"unannotated", nil},
		{"external", []types.ClaimForm{types.ClaimExternalObservation}},
		{"mixed", []types.ClaimForm{types.ClaimExternalObservation, types.ClaimDefinitionFact}},
		{"source_annotation_alone_is_not_a_source_selection", []types.ClaimForm{types.ClaimDefinitionFact}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, citation := b1700QuoteBindingContext(t)
			block := b1700QuoteBindingBlock(tc.forms, false)
			doc := b1700QuoteBindingEmit(t, ctx, map[string]any{"citations": []types.Citation{citation}, "blocks": []any{
				map[string]any{"id": "summary", "kind": "summary", "text": "Keep observed facts separate from source examples."}, block,
			}}, false)
			check := func(doc *types.AnswerDocumentV2, stage string) {
				t.Helper()
				item := blockByID(t, doc, "observations").Items[0]
				if refs := types.AnswerBlockItemCitationRefs(item); len(refs) != 0 {
					t.Errorf("%s invented source citation from matching prose: refs=%v item=%+v", stage, refs, item)
				}
				if item.Text != "The observed message used `Q|sender|payload`; its arrival was delayed." || item.Label != "观测条目" {
					t.Fatalf("%s rewrote the model body: %+v", stage, item)
				}
			}
			check(doc, "full")
			doc = b1700QuoteBindingEmit(t, ctx, map[string]any{"unchanged_block_ids": []string{"summary"}, "replace_blocks": []any{block}}, true)
			check(doc, "replace patch")
			doc = b1700QuoteBindingEmit(t, ctx, map[string]any{"unchanged_block_ids": []string{"observations"}, "replace_blocks": []any{
				map[string]any{"id": "summary", "kind": "summary", "text": "A model-authored revision of the summary only."},
			}}, true)
			check(doc, "unchanged patch")
		})
	}
}

func TestB1700ActualExplicitSourceCitationStillPreserved(t *testing.T) {
	ctx, citation := b1700QuoteBindingContext(t)
	block := b1700QuoteBindingBlock([]types.ClaimForm{types.ClaimExternalObservation, types.ClaimDefinitionFact}, true)
	doc := b1700QuoteBindingEmit(t, ctx, map[string]any{"citations": []types.Citation{citation}, "blocks": []any{
		map[string]any{"id": "summary", "kind": "summary", "text": "The source example documents the format, not an observed arrival."}, block,
	}}, false)
	item := blockByID(t, doc, "observations").Items[0]
	if !reflect.DeepEqual(types.AnswerBlockItemCitationRefs(item), []int{0}) || !item.CitationRefsModelSubmitted {
		t.Fatalf("explicit model source selection was removed: %+v", item)
	}
}

func TestB1700ActualSelectedEvidenceGrowsPoolWithoutBindingSiblingProse(t *testing.T) {
	ctx, _ := b1700QuoteBindingContext(t)
	evidence, err := (&EmitEvidence{}).Execute(ctx, json.RawMessage(`{"items":[{"evidence_kind":"direct","scope":"line","source":"wire.go","line_start":2,"anchor_kind":"definition","anchor_symbol":"WireSyntax","subject":"WireSyntax","summary":"Documents the wire format."}]}`))
	if err != nil || !evidence.Success {
		t.Fatalf("real source evidence prerequisite: %v %+v", err, evidence)
	}
	ctx.Mutable.AppendDispatchToolResult(evidence)
	items := ctx.Mutable.EmittedEvidence()
	if len(items) != 1 || !items[0].IsCitable() || items[0].ID == "" {
		t.Fatalf("expected one real source selection: %+v", items)
	}
	selected := map[string]any{"id": "source", "kind": "bullet_list", "items": []any{
		map[string]any{"id": "selected", "label": "格式定义", "text": "Only this item selects the source definition.", "evidence_ids": []string{items[0].ID}},
	}}
	doc := b1700QuoteBindingEmit(t, ctx, map[string]any{"blocks": []any{
		map[string]any{"id": "summary", "kind": "summary", "text": "The format and the observed delay are distinct facts."},
		selected, b1700QuoteBindingBlock(nil, false),
	}}, false)
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			doc = b1700QuoteBindingEmit(t, ctx, map[string]any{"unchanged_block_ids": []string{"source", "observations"}, "replace_blocks": []any{
				map[string]any{"id": "summary", "kind": "summary", "text": "Model revision preserving both selected and unselected items."},
			}}, true)
		}
		bound := blockByID(t, doc, "source").Items[0]
		if len(types.AnswerBlockItemCitationRefs(bound)) != 1 || !reflect.DeepEqual(bound.EvidenceIDs, []string{items[0].ID}) {
			t.Fatalf("explicit evidence selection did not survive attempt %d: %+v", attempt, bound)
		}
		unbound := blockByID(t, doc, "observations").Items[0]
		if len(types.AnswerBlockItemCitationRefs(unbound)) != 0 {
			t.Fatalf("pool growth bound an unrelated prose-only sibling on attempt %d: %+v", attempt, unbound)
		}
	}
}

func TestB1700QuoteBindingUsesWholeStructuredReferenceSet(t *testing.T) {
	for _, tc := range []struct {
		name string
		refs []int
		want int
	}{
		{"absent", nil, 0},
		{"negative_only", []int{-1, -2}, 0},
		{"additional_selection", []int{0}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := types.AnswerBlockItem{Label: "observed format", Text: "The format is `Q|sender|payload`.", CitationRef: -1, CitationRefs: tc.refs}
			doc := &types.AnswerDocumentV2{Citations: []types.Citation{{File: "wire.go", Line: 2, Quote: `const WireSyntax = "Q|sender|payload"`}},
				Blocks: []types.AnswerBlock{{Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{item}}}}
			if got := normalizeItemCitationRefsByUniqueBacktickCitationQuote(doc); got != tc.want {
				t.Fatalf("fixed=%d, want %d", got, tc.want)
			}
			got := doc.Blocks[0].Items[0]
			if !reflect.DeepEqual(got.CitationRefs, item.CitationRefs) || got.Text != item.Text || got.Label != item.Label {
				t.Fatalf("compatibility repair changed original references/text: %+v", got)
			}
		})
	}
}
