package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise both public decoders and persistence, not just a hand-built typed
// document: the live failure accepted every cell but hid them behind a label.
func TestAnswerDocumentListCellsPublicEmitAndPatch(t *testing.T) {
	for _, kind := range []string{"section", "ordered_list", "bullet_list"} {
		for _, patch := range []bool{false, true} {
			for _, encoded := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/patch=%t/stringified=%t", kind, patch, encoded), func(t *testing.T) {
					cells := []string{"threadpool-400", "IO 等待", "11.000 ms", ""}
					var wireCells any = cells
					if encoded {
						raw, _ := json.Marshal(cells)
						wireCells = string(raw)
					}
					item, _ := json.Marshal(map[string]any{"id": "r1", "label": "#1", "cells": wireCells})
					bus, result, err := executeListCellsPayload(t, kind, "["+string(item)+"]", patch)
					if err != nil || !result.Success {
						t.Fatalf("valid authored cells rejected: result=%+v err=%v", result, err)
					}
					doc := bus.Mutable.AnswerDocumentV2()
					if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].Items) != 1 {
						t.Fatalf("accepted document lost its structured row: %+v", doc)
					}
					if got := doc.Blocks[0].Items[0]; got.Label != "#1" || got.Text != "" || !reflect.DeepEqual(got.Cells, cells) {
						t.Fatalf("decoding must preserve the original fields and positional blank: %+v", got)
					}
					out := render.RenderAnswerDocument(doc, "zh")
					if want := "**#1** — threadpool-400 | IO 等待 | 11.000 ms"; !strings.Contains(out, want) {
						t.Fatalf("accepted authored cells disappeared from the visible answer; want %q:\n%s", want, out)
					}
				})
			}
		}
	}
}

func TestAnswerDocumentListCellsDecodeCompatibilityBoundary(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, tc := range []struct {
			name  string
			items string
			valid bool
		}{
			{"absent", `[{"id":"r","label":"row"}]`, true},
			{"null_array", `[{"id":"r","label":"row","cells":null}]`, true},
			{"empty_array", `[{"id":"r","label":"row","cells":[]}]`, true},
			{"empty_strings", `[{"id":"r","label":"row","cells":[""," "]}]`, true},
			{"structural_fragment", `[{"id":"r","label":"row","cells":["kept"]},"}"]`, true},
			{"number_cell", `[{"id":"r","label":"row","cells":[11]}]`, false},
			{"bool_cell", `[{"id":"r","label":"row","cells":[true]}]`, false},
			{"object_cell", `[{"id":"r","label":"row","cells":[{"text":"do not stringify"}]}]`, false},
			{"array_cell", `[{"id":"r","label":"row","cells":[["do not flatten"]]}]`, false},
			{"singleton_string", `[{"id":"r","label":"row","cells":"do not split this prose"}]`, !patch},
			{"meaningful_item_fragment", `[{"id":"r","label":"row","cells":["kept"]},"do not discard this prose"]`, false},
		} {
			t.Run(fmt.Sprintf("patch=%t/%s", patch, tc.name), func(t *testing.T) {
				bus, result, err := executeListCellsPayload(t, "section", tc.items, patch)
				if err != nil || result.Success != tc.valid {
					t.Fatalf("decode boundary changed: valid=%t result=%+v err=%v", tc.valid, result, err)
				}
				doc := bus.Mutable.AnswerDocumentV2()
				if !tc.valid {
					if !patch && doc != nil {
						t.Fatalf("invalid full emit persisted a document: %+v", doc)
					}
					if patch && (doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Text != "original body") {
						t.Fatalf("invalid patch replaced the accepted base: %+v", doc)
					}
					return
				}
				out := render.RenderAnswerDocument(doc, "en")
				if !strings.Contains(out, "**row**") {
					t.Fatalf("valid empty/compatible cells lost the visible label:\n%s", out)
				}
				if tc.name == "structural_fragment" && !strings.Contains(out, "**row** — kept") {
					t.Fatalf("safe structural-fragment recovery lost authored cells:\n%s", out)
				}
				if tc.name == "singleton_string" {
					// Full emit's existing schema-driven compatibility may wrap a
					// string as one string-array entry; the patch decoder does not.
					// Do not broaden either route or split/stringify its contents.
					want := []string{"do not split this prose"}
					if got := doc.Blocks[0].Items[0].Cells; !reflect.DeepEqual(got, want) {
						t.Fatalf("existing singleton recovery changed authored cells: got %#v want %#v", got, want)
					}
					if !strings.Contains(out, "**row** — do not split this prose") {
						t.Fatalf("existing singleton recovery lost the visible value:\n%s", out)
					}
				}
			})
		}
	}
}

func executeListCellsPayload(t *testing.T, kind, items string, patch bool) (*types.BusContext, types.ToolResult, error) {
	t.Helper()
	bus := newV2TestBusContext()
	block := fmt.Sprintf(`{"id":"rows","kind":%q,"items":%s}`, kind, items)
	if !patch {
		result, err := (&EmitAnswerDocument{}).Execute(bus, json.RawMessage(`{"blocks":[`+block+`]}`))
		return bus, result, err
	}
	base, err := (&EmitAnswerDocument{}).Execute(bus, json.RawMessage(`{"blocks":[{"id":"rows","kind":"section","text":"original body"}]}`))
	if err != nil || !base.Success {
		t.Fatalf("cannot establish public patch base: result=%+v err=%v", base, err)
	}
	result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"replace_blocks":[`+block+`]}`))
	return bus, result, err
}
