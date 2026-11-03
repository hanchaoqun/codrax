package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestHMC334PublicPatchReportsAllContainerShapesWithoutStaging(t *testing.T) {
	bus := newPatchTestBusContext()
	before := bus.Mutable.AnswerDocumentV2()
	raw := json.RawMessage(`{"replace_blocks":[{"id":"s1","kind":"summary","text":"retain this exact business explanation","facet_ids":{"item":"current_code_path"},"requested_dimension_indices":{"item":"1"}},{"id":"list1","kind":"ordered_list","items":[{"id":"i1","cells":{"item":"retain the cell"}}]}],"add_blocks":[{"id":"extra","kind":"table","columns":{"item":"Business name"}}]}`)
	unchanged := append([]byte(nil), raw...)
	result, _ := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if result.Success || !bytes.Equal(raw, unchanged) || !reflect.DeepEqual(before, bus.Mutable.AnswerDocumentV2()) {
		t.Fatal("rejected container shapes changed input or accepted patch base")
	}
	for _, want := range []string{"replace_blocks[0].facet_ids", "replace_blocks[0].requested_dimension_indices", "replace_blocks[1].items[0].cells", "add_blocks[0].columns", "native JSON array", "preserve all intended"} {
		if !strings.Contains(result.Summary, want) {
			t.Errorf("public rejection lost indexed shape guidance %q: %s", want, result.Summary)
		}
	}
	if result.Repair == nil || result.Repair.Code != "tool_param_container_shape" || len(result.Repair.Fields) != 4 {
		t.Errorf("missing same-dispatch structured shape repair: %+v", result.Repair)
	}
}

func TestHMC334PublicFullEmitReportsSiblingContainerShapes(t *testing.T) {
	bus := newPatchTestBusContext()
	before := bus.Mutable.AnswerDocumentV2()
	raw := json.RawMessage(`{"blocks":[{"kind":"summary","text":"full model-authored content","columns":{"item":"Name"},"facet_ids":{"item":"current_code_path"}}],"caveats":{"item":"retain uncertainty"}}`)
	result, _ := (&EmitAnswerDocument{}).Execute(bus, raw)
	if result.Success || !reflect.DeepEqual(before, bus.Mutable.AnswerDocumentV2()) {
		t.Fatal("failed full emit overwrote accepted content")
	}
	for _, want := range []string{"blocks[0].columns", "blocks[0].facet_ids", "caveats"} {
		if !strings.Contains(result.Summary, want) {
			t.Errorf("full emit omits %s: %s", want, result.Summary)
		}
	}
}

type hmc334ShapeRow struct {
	Names   []string `json:"names"`
	IDs     []int    `json:"ids"`
	Options *struct {
		Enabled bool `json:"enabled"`
	} `json:"options"`
}

func TestHMC334SharedDecoderIndexedShapesAndBounds(t *testing.T) {
	for _, normalized := range []bool{false, true} {
		t.Run(fmt.Sprintf("normalized_%t", normalized), func(t *testing.T) {
			raw := json.RawMessage(`{"rows":[{"names":{"item":"A"},"ids":{"item":1},"options":[]},{"names":{"item":"B"},"ids":null,"options":null}]}`)
			var dst struct {
				Rows []hmc334ShapeRow `json:"rows"`
			}
			var resultSummary string
			if normalized {
				_, r, err := decodeStrictNormalizedToolParams("example", raw, &dst, nil)
				if r == nil || err == nil {
					t.Fatal("wrong shape must reject")
				}
				resultSummary = r.Summary
			} else {
				_, r, err := decodeStrictToolParams("example", raw, json.RawMessage(`{"type":"object"}`), &dst, nil)
				if r == nil || err == nil {
					t.Fatal("wrong shape must reject")
				}
				resultSummary = r.Summary
			}
			for _, want := range []string{"rows[0].names", "array of strings", "rows[0].ids", "array of integers", "rows[0].options", "native JSON object", "rows[1].names"} {
				if !strings.Contains(resultSummary, want) {
					t.Errorf("missing %s: %s", want, resultSummary)
				}
			}
			if strings.Contains(resultSummary, "rows[1].ids") || strings.Contains(resultSummary, "rows[1].options") {
				t.Fatal("nullable native values misdiagnosed")
			}
		})
	}
	var rows []map[string]any
	for i := 0; i < 20; i++ {
		rows = append(rows, map[string]any{"names": map[string]any{"item": fmt.Sprint(i)}})
	}
	raw, _ := json.Marshal(map[string]any{"rows": rows})
	var first string
	for i := 0; i < 2; i++ {
		var dst struct {
			Rows []hmc334ShapeRow `json:"rows"`
		}
		_, r, err := decodeStrictNormalizedToolParams("example", raw, &dst, nil)
		if r == nil || err == nil {
			t.Fatal("expected rejection")
		}
		if !strings.Contains(r.Summary, "rows[11].names") || strings.Contains(r.Summary, "rows[12].names") || !strings.Contains(r.Summary, "additional shape mismatches omitted=8") {
			t.Errorf("unbounded or undisclosed diagnostic roster: %s", r.Summary)
		}
		if i == 0 {
			first = r.Summary
		} else if r.Summary != first {
			t.Fatal("shape feedback order is nondeterministic")
		}
	}
}

// Flexible/custom values remain governed by their owning decoder, not by a
// reflection-shaped approximation. Unknown fields retain their existing lane.
type hmc334Opaque struct{ Accepted bool }

func (v *hmc334Opaque) UnmarshalJSON(raw []byte) error { v.Accepted = true; return nil }

func TestHMC334ShapeFeedbackPreservesFlexibleAndUnknownLanes(t *testing.T) {
	var dst struct {
		Raw      json.RawMessage `json:"raw"`
		Flexible hmc334Opaque    `json:"flexible"`
		Number   FlexInt         `json:"number"`
		Any      any             `json:"any"`
	}
	raw := json.RawMessage(`{"raw":{"item":"literal evidence"},"flexible":[],"number":"17","any":{"nested":[]}}`)
	_, r, err := decodeStrictNormalizedToolParams("example", raw, &dst, nil)
	if r != nil || err != nil || !dst.Flexible.Accepted || int(dst.Number) != 17 {
		t.Fatalf("new diagnostic policy changed valid flexible decoding: %v %+v", err, r)
	}
	var ordinary struct {
		Rows []hmc334ShapeRow `json:"rows"`
	}
	_, r, err = decodeStrictNormalizedToolParams("example", json.RawMessage(`{"unrecognized":1,"rows":[]}`), &ordinary, nil)
	if r == nil || err == nil || r.Repair == nil || r.Repair.Code != "tool_param_unknown_field" || strings.Contains(r.Summary, "container shape") {
		t.Fatalf("unknown-field lane was reclassified: %v %+v", err, r)
	}
}

func TestHMC334RejectedSiblingKeepsOpaqueValuesAndExactMapKeys(t *testing.T) {
	var dst struct {
		Names    []string         `json:"names"`
		Numbers  []json.Number    `json:"numbers"`
		Bag      map[string][]int `json:"bag"`
		Raw      json.RawMessage  `json:"raw"`
		Flexible hmc334Opaque     `json:"flexible"`
		Number   json.Number      `json:"number"`
	}
	raw := json.RawMessage(`{"names":{"item":"A"},"numbers":{"item":3},"bag":{"a.b[0]":{"item":1}},"raw":{"literal":[]},"flexible":[],"number":7}`)
	_, r, err := decodeStrictNormalizedToolParams("example", raw, &dst, nil)
	if r == nil || err == nil || r.Repair == nil {
		t.Fatal("expected container rejection")
	}
	for _, want := range []string{`bag["a.b[0]"]`, "array of numbers", "names", "numbers"} {
		if !strings.Contains(r.Summary, want) {
			t.Errorf("missing exact typed diagnostic %s: %s", want, r.Summary)
		}
	}
	for _, path := range r.Repair.Fields {
		if path == "flexible" || path == "raw" || path == "number" {
			t.Errorf("opaque/custom sibling was reflected as invalid: %s", path)
		}
	}
}
