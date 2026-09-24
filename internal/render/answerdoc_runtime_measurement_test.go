package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeMeasurementRenderPreservesTrustedCellsAndNotes(t *testing.T) {
	table := types.RuntimeMeasurementTable{ObservationID: "scope:group", View: types.RuntimeMeasurementTimeline, Label: "Actual IO segments", Columns: []string{"start s", "end s", "requests", "unknown"}, Rows: [][]string{{"0", "0.0000001", "0", ""}, {"0.0000001", "0.001", "2", ""}}, Notes: []string{"source /capture/a; selected [0, 0.002) s", "2 displayed segments, 4 omitted; not a target wait", "issuer a|b <tag> [link](https://invalid)"}}
	receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
	types.BindRuntimeMeasurementReceipt(receipt, &types.RuntimeMeasurementContract{Tables: []types.RuntimeMeasurementTable{table}})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "m", Kind: types.BlockTable, RuntimeMeasurement: receipt}, {ID: "explain", Kind: types.BlockSummary, Text: "The interpretation is unchanged."}}}
	before := receipt.Clone()
	surface := RenderAnswerDocumentSurfaces(doc, "en")
	for _, want := range []string{"| start s | end s | requests | unknown |", "| 0 | 0.0000001 | 0 |  |", "| 0.0000001 | 0.001 | 2 |  |", "4 omitted", "not a target wait", "issuer a&#124;b &lt;tag&gt; &#91;link&#93;", "The interpretation is unchanged."} {
		if !strings.Contains(surface.Answer, want) || !strings.Contains(surface.Primary, want) {
			t.Errorf("trusted table missing %q in visible/primary surface: %s", want, surface.Answer)
		}
	}
	if !reflect.DeepEqual(receipt, before) {
		t.Fatal("render mutated accepted table")
	}
	receipt.ObservationID = "stale"
	stale := RenderAnswerDocument(doc, "en")
	if strings.Contains(stale, "0.0000001") || !strings.Contains(stale, "not bound") {
		t.Fatal("stale selection displayed trusted values")
	}
}
