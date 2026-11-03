package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Check the model-owned native block's wire shape, not numeric substrings in
// opaque addresses or host audit metadata. This also rejects copied payloads
// whose values differ from the old 768 sentinel.
func nativeSelectorSerializationHMC337(wire []byte, blockID string, want types.AnswerRuntimeMeasurementReceipt) error {
	var doc struct {
		Blocks []map[string]json.RawMessage `json:"blocks"`
	}
	if err := json.Unmarshal(wire, &doc); err != nil {
		return err
	}
	found := 0
	for _, block := range doc.Blocks {
		var id string
		if err := json.Unmarshal(block["id"], &id); err != nil {
			return err
		}
		if id != blockID {
			continue
		}
		found++
		for key := range block {
			switch key {
			case "id", "kind", "runtime_measurement", "facet_ids", "claim_uses", "surface_role", "requested_dimension_indices":
				// Declared identity and typed presentation annotations only.
			default:
				return fmt.Errorf("native block %q serialized competing payload field %q", blockID, key)
			}
		}
		var selector map[string]json.RawMessage
		if err := json.Unmarshal(block["runtime_measurement"], &selector); err != nil {
			return err
		}
		if len(selector) != 2 || selector["observation_id"] == nil || selector["view"] == nil {
			return fmt.Errorf("native block %q must serialize only its exact observation_id/view pair", blockID)
		}
		var observationID string
		var view types.RuntimeMeasurementView
		if err := json.Unmarshal(selector["observation_id"], &observationID); err != nil {
			return err
		}
		if err := json.Unmarshal(selector["view"], &view); err != nil {
			return err
		}
		if observationID != want.ObservationID || view != want.View {
			return fmt.Errorf("native block %q changed its current source selector", blockID)
		}
	}
	if found != 1 {
		return fmt.Errorf("native block %q has %d serialized instances, want exactly one", blockID, found)
	}
	return nil
}

func TestHMC337NativeSelectorSerializationOpaqueIdentity(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		t.Run(string(kind), func(t *testing.T) {
			ref := types.AnswerRuntimeMeasurementReceipt{ObservationID: "trace_query:opaque-768-address", View: types.RuntimeMeasurementDistribution,
				BoundTable: &types.RuntimeMeasurementTable{Rows: [][]string{{"768", "other native value"}}}}
			doc := types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "native", Kind: kind, RuntimeMeasurement: &ref}},
				ReadReasoningGraph: &types.AnswerReasoningGraphSummary{GraphID: "graph-768", EventCount: 768}}
			wire, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(wire), "768") {
				t.Fatal("deterministic counterexample lost the legal opaque identity")
			}
			if err := nativeSelectorSerializationHMC337(wire, "native", ref); err != nil {
				t.Fatalf("metadata is not copied model cells: %v; %s", err, wire)
			}
		})
	}
}

func TestHMC337NativeSelectorSerializationRejectsAnyInlinePayload(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{"document_model": "v2", "blocks": []any{map[string]any{"id": "native", "kind": "table", "runtime_measurement": map[string]any{"observation_id": "current", "view": "distribution"}}}}
	}
	mutations := map[string]func(map[string]any){
		"text":    func(b map[string]any) { b["text"] = "a copied 12345, not the old sentinel" },
		"items":   func(b map[string]any) { b["items"] = []any{map[string]any{"cells": []string{"12345"}}} },
		"columns": func(b map[string]any) { b["columns"] = []string{"12345"} },
		"diagram": func(b map[string]any) {
			b["diagram"] = map[string]any{"nodes": []any{map[string]any{"label": "12345"}}}
		},
		"unknown_inline_field": func(b map[string]any) { b["copied_native_rows"] = []any{"12345"} },
		"selector_inline_rows": func(b map[string]any) { b["runtime_measurement"].(map[string]any)["rows"] = []any{"12345"} },
		"wrong_id":             func(b map[string]any) { b["runtime_measurement"].(map[string]any)["observation_id"] = "other" },
		"wrong_view":           func(b map[string]any) { b["runtime_measurement"].(map[string]any)["view"] = "members" },
		"missing_selector":     func(b map[string]any) { delete(b, "runtime_measurement") },
		"duplicate_block":      func(b map[string]any) { b["id"] = "other" },
	}
	want := types.AnswerRuntimeMeasurementReceipt{ObservationID: "current", View: types.RuntimeMeasurementDistribution}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			value := base()
			mutate(value["blocks"].([]any)[0].(map[string]any))
			wire, _ := json.Marshal(value)
			if err := nativeSelectorSerializationHMC337(wire, "native", want); err == nil {
				t.Fatalf("non-selector/model-inline data escaped exact serialization check: %s", wire)
			}
		})
	}
}
