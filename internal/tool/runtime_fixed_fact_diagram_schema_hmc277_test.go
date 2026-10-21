package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC277FixedFactDiagramWithoutRelationshipPlanPublic(t *testing.T) {
	ctx, view, table := samplingMeasurementContextHMC243(t)
	if view.DiagramPlan != nil || !table.HasNativeFixedFacts() {
		t.Fatal("fixture must offer current fixed facts without a relationship plan")
	}
	beforeRequired, _ := json.Marshal(view.RequiredBlocks)
	block := map[string]any{"id": "facts", "kind": "diagram", "runtime_measurement": map[string]any{
		"observation_id": table.ObservationID, "view": table.View,
	}}
	for _, patch := range []bool{false, true} {
		key, schema := "blocks", BuildAnswerDocumentParametersFor(view)
		blocks := []any{map[string]any{"id": "summary", "kind": "summary", "text": "Independent sampling observations."}, block}
		if patch {
			key, schema, blocks = "replace_blocks", BuildAnswerDocumentPatchParametersFor(view), []any{block}
		}
		payload := map[string]any{key: blocks}
		raw, _ := json.Marshal(payload)
		if err := toolparam.Validate(raw, schema); err != nil {
			t.Fatalf("patch=%t selector-only schema: %v", patch, err)
		}
		if result := b1659bExecuteAnswer(t, ctx, payload, patch); !result.Success {
			t.Fatalf("patch=%t public emit: %s", patch, result.Summary)
		}
		bound := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "facts")
		if !runtimeMeasurementBoundFactSurface(bound) || bound.Diagram != nil || len(bound.EdgeAnchors) != 0 ||
			!reflect.DeepEqual(bound.RuntimeMeasurement.BoundTable.Rows, table.Rows) {
			t.Fatal("fact carrier lost exact rows or acquired relationship authority")
		}
		visible := render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "en")
		if !strings.Contains(visible, "15/35") || strings.Contains(visible, "-->") || strings.Contains(visible, "->>") {
			t.Fatal("fact diagram lost its independent denominator or minted a relationship")
		}
	}
	afterRequired, _ := json.Marshal(view.RequiredBlocks)
	if view.DiagramPlan != nil || string(beforeRequired) != string(afterRequired) {
		t.Fatal("optional fact display created or replaced a required diagram obligation")
	}
}

func TestHMC277FixedFactDiagramCannotBorrowRelationshipPayload(t *testing.T) {
	ctx, view, table := samplingMeasurementContextHMC243(t)
	if view.DiagramPlan != nil {
		t.Fatal("fixture unexpectedly has a relationship plan")
	}
	for _, mode := range []string{"missing_selector", "ordinary_diagram", "stale_pair", "competing_text", "competing_edge"} {
		t.Run(mode, func(t *testing.T) {
			block := map[string]any{"id": "facts", "kind": "diagram", "runtime_measurement": map[string]any{
				"observation_id": table.ObservationID, "view": table.View,
			}}
			switch mode {
			case "missing_selector":
				delete(block, "runtime_measurement")
			case "ordinary_diagram":
				delete(block, "runtime_measurement")
				block["diagram"] = map[string]any{"kind": "flow", "language": "mermaid", "body": "flowchart TD\n A --> B"}
			case "stale_pair":
				block["runtime_measurement"].(map[string]any)["observation_id"] = table.ObservationID + ".not-current"
			case "competing_text":
				block["text"] = "A model-authored replacement for measured facts."
			case "competing_edge":
				block["edge_anchors"] = []any{map[string]any{"from_node": "A", "to_node": "B", "relation_kind": "call"}}
			}
			for _, patch := range []bool{false, true} {
				key, schema := "blocks", BuildAnswerDocumentParametersFor(view)
				if patch {
					key, schema = "replace_blocks", BuildAnswerDocumentPatchParametersFor(view)
				}
				raw, _ := json.Marshal(map[string]any{key: []any{block}})
				if toolparam.Validate(raw, schema) == nil {
					t.Fatalf("patch=%t borrowed an unavailable relationship surface", patch)
				}
			}
			// Ordinary Mermaid content is rejected by the projected tool schema;
			// direct legacy Execute deliberately has broader compatibility. The
			// native selector's exact binding remains independently enforced there.
			if mode != "ordinary_diagram" {
				payload := map[string]any{"blocks": []any{map[string]any{"id": "summary", "kind": "summary", "text": "Independent observations."}, block}}
				if result := b1659bExecuteAnswer(t, ctx, payload, false); result.Success {
					t.Fatal("direct public Execute accepted an invalid native fact selector/payload")
				}
			}
		})
	}
}

func TestHMC277FixedFactDiagramRequiresCurrentPrivateSeal(t *testing.T) {
	_, original, table := samplingMeasurementContextHMC243(t)
	for _, mode := range []string{"changed_rows", "json_restored", "ambiguous_pair", "no_facts"} {
		t.Run(mode, func(t *testing.T) {
			view := *original
			candidate := table.Clone()
			tables := []types.RuntimeMeasurementTable{candidate}
			switch mode {
			case "changed_rows":
				tables[0].Rows[0][0] += ".changed"
			case "json_restored":
				raw, _ := json.Marshal(candidate)
				var restored types.RuntimeMeasurementTable
				if err := json.Unmarshal(raw, &restored); err != nil {
					t.Fatal(err)
				}
				tables[0] = restored
			case "ambiguous_pair":
				tables = append(tables, candidate.Clone())
			case "no_facts":
				tables = nil
			}
			view.RuntimeMeasurementContract = &types.RuntimeMeasurementContract{Tables: tables}
			for _, kind := range types.AnswerSemanticViewAllowedBlockKinds(&view) {
				if kind == types.BlockDiagram {
					t.Fatal("unavailable/ambiguous fact seal granted a diagram carrier")
				}
			}
			payload := map[string]any{"blocks": []any{map[string]any{"id": "facts", "kind": "diagram", "runtime_measurement": map[string]any{
				"observation_id": table.ObservationID, "view": table.View,
			}}}}
			raw, _ := json.Marshal(payload)
			if toolparam.Validate(raw, BuildAnswerDocumentParametersFor(&view)) == nil {
				t.Fatal("stale native selector passed the projected schema")
			}
		})
	}
}
