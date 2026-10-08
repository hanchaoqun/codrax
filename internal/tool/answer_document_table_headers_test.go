package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerTableHeadersPublicEmitAndPatchRejectMalformedCells(t *testing.T) {
	for name, table := range map[string]string{
		"missing":             `{"id":"table","kind":"table","items":[{"id":"a","cells":["alpha","10"]}]}`,
		"empty":               `{"id":"table","kind":"table","columns":[],"items":[{"cells":["alpha","10"]}]}`,
		"blank_middle":        `{"id":"table","kind":"table","columns":["Owner"," ","Count"],"items":[{"cells":["alpha","x","10"]}]}`,
		"blank_tail":          `{"id":"table","kind":"table","columns":["Owner"," "],"items":[{"cells":["alpha"]}]}`,
		"unequal_width":       `{"id":"table","kind":"table","columns":["Owner","Count"],"items":[{"cells":["alpha","10"]},{"cells":["beta"]}]}`,
		"empty_unequal_width": `{"id":"table","kind":"table","columns":["Owner","Count"],"items":[{"cells":["alpha","10"]},{"cells":[" "]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			for _, mode := range []string{"full", "replace", "add"} {
				t.Run(mode, func(t *testing.T) {
					bus := newV2TestBusContext()
					seed := json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"Keep this conclusion."},{"id":"table","kind":"table","items":[{"label":"before","text":"unchanged"}]}]}`)
					result, err := (&EmitAnswerDocument{}).Execute(bus, seed)
					if err != nil || !result.Success {
						t.Fatalf("seed: %v %+v", err, result)
					}
					before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
					switch mode {
					case "full":
						result, err = (&EmitAnswerDocument{}).Execute(bus, json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"Keep this conclusion."},`+table+`]}`))
					case "replace":
						result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"replace_blocks":[`+table+`],"unchanged_block_ids":["summary"]}`))
					case "add":
						result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"add_blocks":[`+strings.Replace(table, `"id":"table"`, `"id":"new-table"`, 1)+`],"unchanged_block_ids":["summary","table"]}`))
					}
					if err != nil || result.Success {
						t.Fatalf("invalid table accepted: %v %+v", err, result)
					}
					if !strings.Contains(result.Summary, "blocks[") || !strings.Contains(result.Summary, "column") {
						t.Fatalf("failure must locate this table/column: %+v", result)
					}
					after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
					if string(before) != string(after) {
						t.Fatal("rejected table changed accepted document")
					}
				})
			}
		})
	}
}

func TestAnswerTableHeadersPublicCompatibleCarriers(t *testing.T) {
	for name, table := range map[string]string{
		"markdown":              `{"id":"table","kind":"table","text":"| Owner | Count |\n|---|---|\n| alpha | 10 |"}`,
		"label_text":            `{"id":"table","kind":"table","items":[{"label":"alpha","text":"10"}]}`,
		"cells":                 `{"id":"table","kind":"table","columns":["Owner","Count"],"items":[{"cells":["alpha","10"]},{"cells":["beta",""]}]}`,
		"label_first":           `{"id":"table","kind":"table","columns":["Owner","Count"],"items":[{"label":"alpha","cells":["10"]}]}`,
		"synthetic_label":       `{"id":"table","kind":"table","columns":["Count"],"items":[{"label":"alpha","cells":["10"]}]}`,
		"list_cells_unaffected": `{"id":"table","kind":"bullet_list","items":[{"cells":["alpha","10"]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			bus := newV2TestBusContext()
			full := json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"Read this."},` + table + `]}`)
			result, err := (&EmitAnswerDocument{}).Execute(bus, full)
			if err != nil || !result.Success {
				t.Fatalf("compatible full: %v %+v", err, result)
			}
			before := bus.Mutable.AnswerDocumentV2()
			result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"replace_blocks":[`+table+`],"unchanged_block_ids":["summary"]}`))
			if err != nil || !result.Success {
				t.Fatalf("compatible patch: %v %+v", err, result)
			}
			if !reflect.DeepEqual(before.Blocks, bus.Mutable.AnswerDocumentV2().Blocks) {
				t.Fatal("same payload patch changed rows")
			}
		})
	}
}

func TestAnswerTableHeadersStoredLegacyAndVisibleRecoveryRemainReadable(t *testing.T) {
	legacy := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Old answer."},
		{ID: "table", Kind: types.BlockTable, Items: []types.AnswerBlockItem{{ID: "a", Cells: []string{"alpha", "10"}}, {ID: "b", Cells: []string{"beta", "20"}}}},
	}}
	before := render.RenderAnswerDocument(legacy, "zh")
	if !strings.Contains(before, "alpha") || !strings.Contains(before, "beta") {
		t.Fatal("stored table lost values")
	}
	bus := newV2TestBusContext()
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, legacy)
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["summary","table"],"add_blocks":[{"id":"note","kind":"caveat","text":"New note."}]}`))
	if err != nil || !result.Success {
		t.Fatalf("unchanged old table must stay compatible: %v %+v", err, result)
	}
	if !reflect.DeepEqual(legacy.Blocks[:2], bus.Mutable.AnswerDocumentV2().Blocks[:2]) {
		t.Fatal("old document changed")
	}
	raw := `{"blocks":[{"id":"summary","kind":"summary","text":"Old answer."},{"id":"table","kind":"table","items":[{"id":"a","cells":["alpha","10"]},{"id":"b","cells":["beta","20"]}]}]}`
	recovered, ok := RecoverAnswerDocumentV2FromText(raw)
	if !ok || !strings.Contains(render.RenderAnswerDocument(recovered.Document, "zh"), "beta") {
		t.Fatalf("display-only recovery lost table: %+v", recovered)
	}
}

func TestAnswerTableHeadersSchemaFullPatchTeachingParity(t *testing.T) {
	view := &types.AnswerSemanticView{RequiredBlocks: []types.BlockRequirement{{Kind: types.BlockSummary, Required: true}}, OptionalBlocks: []types.BlockRequirement{{Kind: types.BlockTable}}}
	for _, schema := range []json.RawMessage{(&EmitAnswerDocument{}).Parameters(), BuildAnswerDocumentParametersFor(view), BuildAnswerDocumentPatchParametersFor(view)} {
		for _, want := range []string{"non-empty column headers", "every structured cells row", "same width"} {
			if !strings.Contains(string(schema), want) {
				t.Fatalf("schema omits %q", want)
			}
		}
		if strings.Contains(string(schema), "Optional table headers") {
			t.Fatal("schema teaches headers as optional for cells")
		}
	}
}

func TestAnswerTableHeadersFirstRejectedDraftCanBePatchedWithoutLoss(t *testing.T) {
	for _, headers := range []string{"", `,"columns":["Owner"]`} {
		t.Run(headers, func(t *testing.T) { testAnswerTableHeadersRejectedDraftPatch(t, headers) })
	}
}

func testAnswerTableHeadersRejectedDraftPatch(t *testing.T, headers string) {
	t.Helper()
	bus := newV2TestBusContext()
	bus.Mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	raw := json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"Keep my conclusion."},{"id":"table","kind":"table","title":"Observed values"` + headers + `,"items":[{"id":"a","cells":["alpha","10"]},{"id":"b","cells":["beta","20"]}]},{"id":"caveat","kind":"caveat","text":"Keep the boundary."}],"trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}}`)
	result, err := (&EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success {
		t.Fatalf("missing headers must reject: %v %+v", err, result)
	}
	base := bus.Mutable.LastRejectedAnswerDocumentV2()
	if base == nil || len(base.Blocks) != 3 || len(base.Blocks[1].Items) != 2 {
		t.Fatalf("must retain a complete unpublished patch base without guessing headers: %+v", base)
	}
	selected := bus.Mutable.PendingTraceRootCauseReport()
	if selected == nil || len(selected.RootCauses) != 1 || selected.RootCauses[0].Summary != "RenderThread线程CPU调度延迟" {
		t.Fatalf("table rejection lost the separately bound root selection: %+v", selected)
	}
	result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["summary","table","caveat"]}`))
	if err != nil || result.Success {
		t.Fatalf("unchanged rejected table must not bypass shape checks: %v %+v", err, result)
	}
	result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"replace_blocks":[{"id":"table","kind":"table","title":"Observed values","columns":["Owner","Count"],"items":[{"id":"a","cells":["alpha","10"]},{"id":"b","cells":["beta","20"]}]}],"unchanged_block_ids":["summary","caveat"]}`))
	if err != nil || !result.Success {
		t.Fatalf("headers-only correction must close locally: %v %+v", err, result)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if !reflect.DeepEqual(base.Blocks[0], doc.Blocks[0]) || !reflect.DeepEqual(base.Blocks[2], doc.Blocks[2]) || !reflect.DeepEqual(base.Blocks[1].Items, doc.Blocks[1].Items) {
		t.Fatal("local correction changed unrelated rows or blocks")
	}
	if report := bus.Mutable.TraceRootCauseReport(); !reflect.DeepEqual(report, selected) {
		t.Fatalf("table correction lost the separate model-selected root cause: %+v", report)
	}
}

func TestAnswerTableHeadersIncompleteOrAmbiguousDraftIsNotPatchBase(t *testing.T) {
	for _, sibling := range []string{
		`{"id":"table","kind":"summary","text":"conflicting identity"}`,
		`{"id":"invalid","kind":"not_a_kind","text":"incomplete typed projection"}`,
	} {
		bus := newV2TestBusContext()
		raw := json.RawMessage(`{"blocks":[` + sibling + `,{"id":"table","kind":"table","items":[{"cells":["alpha","10"]}]}]}`)
		result, err := (&EmitAnswerDocument{}).Execute(bus, raw)
		if err != nil || result.Success {
			t.Fatalf("invalid document accepted: %v %+v", err, result)
		}
		if bus.Mutable.LastRejectedAnswerDocumentV2() != nil {
			t.Fatal("ambiguous/incomplete projection became inheritable")
		}
	}
}

func TestAnswerTableHeadersStringRecoveryKeepsValidPositionalValues(t *testing.T) {
	const blocks = `[{"id":"summary","kind":"summary","text":"Keep this."},{"id":"table","kind":"table","columns":["Owner","Count","Note"],"items":[{"id":"a","cells":["alpha","10",""]}]}]`
	bus := newV2TestBusContext()
	raw, _ := json.Marshal(map[string]any{"blocks": blocks})
	result, err := (&EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("valid JSON string recovery changed: %v %+v", err, result)
	}
	if got := bus.Mutable.AnswerDocumentV2().Blocks[1].Items[0].Cells; !reflect.DeepEqual(got, []string{"alpha", "10", ""}) {
		t.Fatalf("positional values changed: %q", got)
	}
}
