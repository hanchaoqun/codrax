package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

const b1694Source = "package p\nfunc Worker() int {\n value := 1\n return value\n}\nconst Other = 7\n"

func b1694RangeContext(t *testing.T) (*types.BusContext, types.EvidenceItem) {
	t.Helper()
	ctx := b1698ReadCommentSource(t, "worker.go", b1694Source, 0, 100)
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}
	raw := json.RawMessage(`{"items":[{"evidence_kind":"direct","scope":"line_range","source":"worker.go","line_start":2,"line_end":5,"anchor_kind":"definition","anchor_symbol":"Worker","subject":"Worker","summary":"The model-selected source definition."}]}`)
	result, err := (&EmitEvidence{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("real evidence emit prerequisite: %v %+v", err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	evidence := ctx.Mutable.EmittedEvidence()
	if len(evidence) != 1 || !evidence[0].IsCitable() || evidence[0].LineEnd != 5 {
		t.Fatalf("expected one grounded source range: %+v", evidence)
	}
	return ctx, evidence[0]
}

func b1694Emit(t *testing.T, ctx *types.BusContext, payload map[string]any, patch bool) *types.AnswerDocumentV2 {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), raw...)
	beforeEvidence, _ := json.Marshal(ctx.Mutable.EmittedEvidence())
	var result types.ToolResult
	if patch {
		result, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	} else {
		result, err = (&EmitAnswerDocument{}).Execute(ctx, raw)
	}
	if err != nil || !result.Success {
		t.Fatalf("public answer emit failed: %v %+v", err, result)
	}
	afterEvidence, _ := json.Marshal(ctx.Mutable.EmittedEvidence())
	if !bytes.Equal(before, raw) || !bytes.Equal(beforeEvidence, afterEvidence) {
		t.Fatal("answer transport mutated model input bytes or source evidence")
	}
	actualSource, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "worker.go"))
	if err != nil || string(actualSource) != b1694Source {
		t.Fatalf("answer transport changed source bytes: %v %q", err, actualSource)
	}
	return ctx.Mutable.AnswerDocumentV2()
}

func b1694SelectedBlock(id string) map[string]any {
	return map[string]any{"id": "selected", "kind": "bullet_list", "items": []any{
		map[string]any{"id": "range", "label": "Worker", "text": "The model's exact description stays intact.", "evidence_ids": []string{id}},
	}}
}

func b1694AssertSelectedRange(t *testing.T, doc *types.AnswerDocumentV2, id string, end int) int {
	t.Helper()
	item := blockByID(t, doc, "selected").Items[0]
	refs := types.AnswerBlockItemCitationRefs(item)
	if item.Label != "Worker" || item.Text != "The model's exact description stays intact." || !reflect.DeepEqual(item.EvidenceIDs, []string{id}) {
		t.Fatalf("citation transport rewrote model-authored content: %+v", item)
	}
	if len(refs) != 1 || refs[0] < 0 || refs[0] >= len(doc.Citations) {
		t.Fatalf("selected evidence must bind exactly one valid citation: %+v pool=%+v", item, doc.Citations)
	}
	cit := doc.Citations[refs[0]]
	if cit.File != "worker.go" || cit.Line != 2 || cit.LineEnd != end {
		t.Fatalf("selected range was stolen by a same-start citation: got %+v, want worker.go:2-%d", cit, end)
	}
	before, _ := json.Marshal(doc)
	rendered := render.RenderAnswerDocument(doc, "en")
	after, _ := json.Marshal(doc)
	if (cit.Scope == types.ScopeLineRange && !strings.Contains(rendered, fmt.Sprintf("worker.go:2-%d", end))) || !bytes.Equal(before, after) {
		t.Fatalf("render lost the range or mutated accepted content: %s", rendered)
	}
	return refs[0]
}

func TestB1694PublicEmitSelectedRangeSurvivesLabelPoint(t *testing.T) {
	ctx, ev := b1694RangeContext(t)
	doc := b1694Emit(t, ctx, map[string]any{"blocks": []any{
		map[string]any{"id": "summary", "kind": "summary", "text": "The selected definition has an exact source extent."},
		b1694SelectedBlock(ev.ID),
	}}, false)
	b1694AssertSelectedRange(t, doc, ev.ID, 5)
	if len(doc.Citations) != 1 || doc.Citations[0].Scope != types.ScopeLineRange {
		t.Fatalf("explicit selection should not produce a redundant label-derived point: %+v", doc.Citations)
	}
}

func TestB1694PublicFullAndPatchReuseExactRangePreserveModelPool(t *testing.T) {
	for _, order := range []string{"point_first", "range_first"} {
		for _, scope := range []types.EvidenceScope{types.ScopeLineRange, types.ScopeLine, ""} {
			t.Run(order+"/"+string(scope), func(t *testing.T) {
				ctx, ev := b1694RangeContext(t)
				point := types.Citation{File: "worker.go", Line: 2, Quote: "func Worker() int {"}
				rangeCitation := types.Citation{File: "worker.go", Line: 2, LineEnd: 5, Scope: scope,
					Quote: "func Worker() int {\n value := 1\n return value\n}"}
				other := types.Citation{File: "worker.go", Line: 6, Scope: types.ScopeLine,
					Quote: "const Other = 7"}
				pool := []types.Citation{point, rangeCitation, other}
				pointRef, rangeRef := 0, 1
				if order == "range_first" {
					pool[0], pool[1] = pool[1], pool[0]
					pointRef, rangeRef = 1, 0
				}
				pointBlock := map[string]any{"id": "point", "kind": "bullet_list", "items": []any{
					map[string]any{"id": "model-point", "label": "Worker", "text": "The model selected this single location.", "citation_ref": pointRef},
					map[string]any{"id": "unrelated", "label": "Other", "text": "The unrelated model citation must remain intact.", "citation_ref": 2},
				}}
				doc := b1694Emit(t, ctx, map[string]any{"citations": pool, "blocks": []any{
					map[string]any{"id": "summary", "kind": "summary", "text": "Point and extent are separate selections."},
					pointBlock, b1694SelectedBlock(ev.ID),
				}}, false)
				check := func(doc *types.AnswerDocumentV2) {
					t.Helper()
					if !reflect.DeepEqual(doc.Citations, pool) {
						t.Fatalf("exact extent reuse changed/reordered/duplicated model pool: got %+v want %+v", doc.Citations, pool)
					}
					if got := b1694AssertSelectedRange(t, doc, ev.ID, 5); got != rangeRef {
						t.Fatalf("selected range ref=%d, want exact matching pool index %d", got, rangeRef)
					}
					item := blockByID(t, doc, "point").Items[0]
					if !reflect.DeepEqual(types.AnswerBlockItemCitationRefs(item), []int{pointRef}) || len(item.EvidenceIDs) != 0 || item.Label != "Worker" || item.Text != "The model selected this single location." {
						t.Fatalf("model-selected point was promoted into an overlapping evidence range: %+v", item)
					}
				}
				check(doc)
				beforeSummary := blockByID(t, doc, "summary")
				doc = b1694Emit(t, ctx, map[string]any{"unchanged_block_ids": []string{"summary", "point"}, "replace_blocks": []any{b1694SelectedBlock(ev.ID)}}, true)
				check(doc)
				if !reflect.DeepEqual(beforeSummary, blockByID(t, doc, "summary")) {
					t.Fatal("citation patch changed an explicitly unchanged block")
				}
				doc = b1694Emit(t, ctx, map[string]any{"unchanged_block_ids": []string{"point", "selected"}, "replace_blocks": []any{
					map[string]any{"id": "summary", "kind": "summary", "text": "Only the model's summary was revised."},
				}}, true)
				check(doc)
			})
		}
	}
}

func TestB1694PublicPatchAddsRangeBesideUnchangedModelPoint(t *testing.T) {
	ctx, ev := b1694RangeContext(t)
	point := types.Citation{File: "worker.go", Line: 2, Quote: "func Worker() int {"}
	doc := b1694Emit(t, ctx, map[string]any{"citations": []types.Citation{point}, "blocks": []any{
		map[string]any{"id": "summary", "kind": "summary", "text": "The first draft selects one location."},
		map[string]any{"id": "point", "kind": "bullet_list", "items": []any{
			map[string]any{"id": "point", "label": "Worker", "text": "Keep this original selection unchanged.", "citation_ref": 0},
		}},
	}}, false)
	beforePoint := blockByID(t, doc, "point")
	if len(doc.Citations) != 1 || !reflect.DeepEqual(doc.Citations[0], point) || len(beforePoint.Items[0].EvidenceIDs) != 0 {
		t.Fatalf("initial point selection was widened before patch: %+v", doc)
	}
	doc = b1694Emit(t, ctx, map[string]any{"unchanged_block_ids": []string{"summary", "point"}, "add_blocks": []any{b1694SelectedBlock(ev.ID)}}, true)
	if len(doc.Citations) != 2 || !reflect.DeepEqual(doc.Citations[0], point) || !reflect.DeepEqual(blockByID(t, doc, "point"), beforePoint) {
		t.Fatalf("patch range addition changed the original point or its block: %+v", doc)
	}
	if ref := b1694AssertSelectedRange(t, doc, ev.ID, 5); ref != 1 {
		t.Fatalf("patch rebound new range to old point index: ref=%d", ref)
	}
}

func TestB1694PublicSameStartDistinctRangesStayDistinctInEitherOrder(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse_%t", reverse), func(t *testing.T) {
			ctx, full := b1694RangeContext(t)
			result, err := (&EmitEvidence{}).Execute(ctx, json.RawMessage(`{"items":[{"evidence_kind":"direct","scope":"line_range","source":"worker.go","line_start":2,"line_end":4,"anchor_kind":"definition","anchor_symbol":"Worker","subject":"Worker","summary":"The separately selected partial extent."}]}`))
			if err != nil || !result.Success {
				t.Fatalf("second distinct source extent prerequisite: %v %+v", err, result)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			var partial types.EvidenceItem
			for _, ev := range ctx.Mutable.EmittedEvidence() {
				if ev.LineEnd == 4 {
					partial = ev
				}
			}
			if partial.ID == "" || partial.ID == full.ID {
				t.Fatalf("distinct accepted range identities required: %+v", ctx.Mutable.EmittedEvidence())
			}
			selected := []types.EvidenceItem{full, partial}
			if reverse {
				selected[0], selected[1] = selected[1], selected[0]
			}
			items := make([]any, 0, 3)
			for i, ev := range selected {
				items = append(items, map[string]any{"id": fmt.Sprintf("extent-%d", i), "label": "Selected extent", "text": "Preserve this independently selected extent.", "evidence_ids": []string{ev.ID}})
			}
			items = append(items, map[string]any{"id": "repeat", "label": "Repeated extent", "text": "Reuse the exact same source extent.", "evidence_ids": []string{selected[0].ID}})
			block := map[string]any{"id": "extents", "kind": "bullet_list", "items": items}
			doc := b1694Emit(t, ctx, map[string]any{"blocks": []any{
				map[string]any{"id": "summary", "kind": "summary", "text": "These selected extents begin at the same line."}, block,
			}}, false)
			check := func(doc *types.AnswerDocumentV2) {
				t.Helper()
				if len(doc.Citations) != 2 {
					t.Fatalf("distinct extents must survive and exact repeats must reuse: %+v", doc.Citations)
				}
				got := blockByID(t, doc, "extents").Items
				for i, item := range got {
					refs := types.AnswerBlockItemCitationRefs(item)
					want := selected[i%2]
					if len(refs) != 1 || refs[0] < 0 || refs[0] >= len(doc.Citations) || doc.Citations[refs[0]].Line != want.LineStart || doc.Citations[refs[0]].LineEnd != want.LineEnd || !reflect.DeepEqual(item.EvidenceIDs, []string{want.ID}) {
						t.Fatalf("same-start extent collapsed: item=%+v pool=%+v want=%+v", item, doc.Citations, want)
					}
				}
				if got[0].CitationRef == got[1].CitationRef || got[0].CitationRef != got[2].CitationRef {
					t.Fatalf("range reuse is order-dependent: %+v", got)
				}
			}
			check(doc)
			doc = b1694Emit(t, ctx, map[string]any{"unchanged_block_ids": []string{"summary"}, "replace_blocks": []any{block}}, true)
			check(doc)
		})
	}
}

func TestB1694PublicAmbiguousOverlappingRangesCannotBroadenModelSelection(t *testing.T) {
	for _, selectedEnd := range []int{0, 3} {
		t.Run(fmt.Sprintf("selected_end_%d", selectedEnd), func(t *testing.T) {
			ctx, _ := b1694RangeContext(t)
			result, err := (&EmitEvidence{}).Execute(ctx, json.RawMessage(`{"items":[{"evidence_kind":"direct","scope":"line_range","source":"worker.go","line_start":2,"line_end":4,"anchor_kind":"definition","anchor_symbol":"Worker","subject":"Worker","summary":"The separately selected partial extent."}]}`))
			if err != nil || !result.Success {
				t.Fatalf("second overlapping range prerequisite: %v %+v", err, result)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			citation := types.Citation{File: "worker.go", Line: 2, LineEnd: selectedEnd, Quote: "func Worker() int {"}
			if selectedEnd > 2 {
				citation.Scope = types.ScopeLineRange
				citation.Quote += "\n value := 1"
			}
			block := map[string]any{"id": "selection", "kind": "bullet_list", "items": []any{
				map[string]any{"id": "model-selected", "label": "Worker", "text": "The model selected only this source extent.", "citation_ref": 0},
			}}
			doc := b1694Emit(t, ctx, map[string]any{"citations": []types.Citation{citation}, "blocks": []any{
				map[string]any{"id": "summary", "kind": "summary", "text": "Overlapping evidence does not select a larger extent."}, block,
			}}, false)
			check := func(doc *types.AnswerDocumentV2) {
				t.Helper()
				item := blockByID(t, doc, "selection").Items[0]
				if !reflect.DeepEqual(doc.Citations, []types.Citation{citation}) || !reflect.DeepEqual(types.AnswerBlockItemCitationRefs(item), []int{0}) || len(item.EvidenceIDs) != 0 || item.CitationRefsEvidenceIDAdoptionRequired || item.Text != "The model selected only this source extent." {
					t.Fatalf("overlap/ambiguity broadened model selection or manufactured evidence ownership: %+v pool=%+v", item, doc.Citations)
				}
			}
			check(doc)
			doc = b1694Emit(t, ctx, map[string]any{"unchanged_block_ids": []string{"selection"}, "replace_blocks": []any{
				map[string]any{"id": "summary", "kind": "summary", "text": "The model changed only the summary."},
			}}, true)
			check(doc)
		})
	}
}
