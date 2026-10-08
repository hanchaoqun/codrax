package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func ownershipStringCarrier(t *testing.T, field, body string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{field: body})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

const ownershipCompleteBlocks = `[{"id":"summary","kind":"summary","text":"Six events."},{"id":"events","kind":"table","items":[{"label":"one","cells":["2.01","CAMERA"]}]}]`

func TestAnswerRecoveryOwnershipOrphanVisiblePayload(t *testing.T) {
	for _, tail := range []string{
		`, "columns":["Time","Domain"]}]`,
		`, "items":[{"label":"lost list entry"}]}]`,
		`, "diagram":{"kind":"sequence","body":"sequenceDiagram\nA->>B: message"}}]`,
		`, "text":"lost visible prose"}]`,
	} {
		t.Run(tail, func(t *testing.T) {
			raw := ownershipStringCarrier(t, "blocks", ownershipCompleteBlocks+tail)
			_, report, ok := repairBlocksAsStringDetailed(raw)
			if !ok || report.Lossless || !report.DroppedVisiblePayload {
				t.Fatalf("equal block counts must not certify missing visible fields: %+v ok=%v", report, ok)
			}
			bus := newV2TestBusContext()
			result, err := (&EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || result.Success || result.Repair == nil || result.Repair.Code != "answer_doc_visible_payload_ownership" {
				t.Fatalf("must repair ownership before publishing: err=%v result=%+v", err, result)
			}
			if bus.Mutable.AnswerDocumentV2() != nil {
				t.Fatal("partial answer was published")
			}
			var found bool
			for _, attachment := range bus.Mutable.AnswerDisplayAttachments() {
				found = found || strings.Contains(attachment.Body, string(raw))
			}
			if !found {
				t.Fatal("original carrier, including unowned values, was not retained")
			}
			if _, ok := RepairEmitAnswerDocumentMalformedParams(raw); ok {
				t.Fatal("agent-facing pre-parser must not erase the ownership defect")
			}
			recovered, ok := RecoverAnswerDocumentV2FromText(string(raw))
			if !ok || recovered.Lossless || len(recovered.Diagnostics) == 0 {
				t.Fatalf("display fallback must disclose incomplete ownership: %+v", recovered)
			}
		})
	}
}

func TestAnswerRecoveryOwnershipQuotedExamplesAndMetadata(t *testing.T) {
	block := map[string]any{"id": "summary", "kind": "summary", "text": `Example: {"id":"x","kind":"table","columns":["shown as text"]}; escaped quote: "`}
	blocks, _ := json.Marshal([]any{block})
	for _, suffix := range []string{"", `, "debug_metadata":{"kind":"table","id":"meta","columns":["not an answer"],"text":"metadata"}, trailing`} {
		bus := newV2TestBusContext()
		raw, _ := json.Marshal(map[string]any{"blocks": string(blocks) + suffix, "debug_metadata": map[string]any{"items": []string{"not a list"}, "text": "metadata"}})
		result, err := (&EmitAnswerDocument{}).Execute(bus, raw)
		if err != nil || !result.Success {
			t.Fatalf("quoted JSON and metadata must not become structural fields: %v %+v", err, result)
		}
		if bus.Mutable.AnswerDocumentV2().Blocks[0].Text != block["text"] {
			t.Fatal("quoted JSON changed")
		}
		if len(bus.Mutable.AnswerDisplayAttachments()) != 0 {
			t.Fatal("lossless carrier produced degraded attachments")
		}
	}
}

func TestAnswerRecoveryOwnershipDoesNotGuessLastTable(t *testing.T) {
	body := `[{"id":"s","kind":"summary","text":"lead"},{"id":"one","kind":"table","items":[{"label":"first"}]},{"id":"two","kind":"table","items":[{"label":"second"}]}],"columns":["ambiguous"]}]`
	raw := ownershipStringCarrier(t, "blocks", body)
	bus := newV2TestBusContext()
	result, err := (&EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || result.Success {
		t.Fatalf("ambiguous table field accepted: %v %+v", err, result)
	}
	base := bus.Mutable.LastRejectedAnswerDocumentV2()
	if base == nil || len(base.Blocks) != 3 {
		t.Fatalf("valid blocks unavailable for local repair: %+v", base)
	}
	for _, block := range base.Blocks {
		if len(block.Columns) > 0 {
			t.Fatal("orphan columns were assigned by proximity")
		}
	}
	patch := json.RawMessage(`{"replace_blocks":[{"id":"one","kind":"table","columns":["model-chosen owner"],"items":[{"label":"first"}]}],"unchanged_block_ids":["s","two"]}`)
	result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, patch)
	if err != nil || !result.Success {
		t.Fatalf("explicit owner correction should close locally: %v %+v", err, result)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if len(doc.Blocks) != 3 || doc.Blocks[1].Columns[0] != "model-chosen owner" || len(doc.Blocks[2].Columns) != 0 {
		t.Fatalf("patch lost intact blocks: %+v", doc)
	}
	if len(bus.Mutable.AnswerDisplayAttachments()) != 0 {
		t.Fatal("successful correction must not publish failed raw JSON as an extra answer")
	}
}

func TestAnswerRecoveryOwnershipAnnotationMustActuallySurvive(t *testing.T) {
	for _, suffix := range []string{
		`,"columns":["x"],"title":"rows"}]`,
		`,"metadata":{"nested":"separator"},"title":"lost title"}]`,
	} {
		body := `[{"id":"s","kind":"summary","text":"lead"},{"id":"t","kind":"table","columns":["x"],"items":[{"label":"row"}]}]` + suffix
		if paths := answerDocumentPayloadOwnershipPaths(ownershipStringCarrier(t, "blocks", body), answerDocumentFullEmitQuarantineProfile); len(paths) == 0 {
			t.Fatal("equality or nearby annotation did not prove ownership")
		}
	}
}

func TestAnswerRecoveryOwnershipStructuralCounter(t *testing.T) {
	for _, test := range []struct {
		body  string
		count int
	}{
		{`[{"id":"s","kind":"summary","text":"literal \"kind\":\"table\""}],trailing`, 1},
		{`[{"id":"s","kind":"summary","text":"lead"},{"id":"list","kind":"ordered_list","items":[{"label":"broken "quote"}]}],trailing`, 2},
		{`[{"id":"s","kind":"summary","text":"lead"}],"metadata":{"id":"m","kind":"table"},trailing`, 1},
		{`[{"kind":"summary","text":"missing-id candidate"}],trailing`, 1},
	} {
		if got := countAnswerBlockKindMarkers(test.body); got != test.count {
			t.Fatalf("count=%d want=%d body=%s", got, test.count, test.body)
		}
	}
}

func TestAnswerRecoveryOwnershipMalformedOuterEnvelopeCannotDiscardSiblings(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"blocks":[{"id":"s","kind":"summary","text":"lead"}]:,"columns":["lost"]}`),
		json.RawMessage(`{"metadata":{"text":"not answer"},"blocks":[{"id":"s","kind":"summary","text":"lead"}]:,"items":[{"label":"lost"}]}`),
	} {
		if _, ok := RepairEmitAnswerDocumentMalformedParams(raw); ok {
			t.Fatalf("malformed outer recovery discarded visible sibling: %s", raw)
		}
	}
}

func TestAnswerRecoveryOwnershipNativeRootAndPatch(t *testing.T) {
	for _, patch := range []bool{false, true} {
		for _, wrapped := range []bool{false, true} {
			bus := newV2TestBusContext()
			base := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "original"}}}
			field := "blocks"
			if patch {
				field = "replace_blocks"
				bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, base)
			}
			var raw json.RawMessage
			if wrapped {
				raw = ownershipStringCarrier(t, field, `[{"id":"summary","kind":"summary","text":"new"}], "columns":["lost header"]}]`)
			} else {
				raw = json.RawMessage(`{"` + field + `":[{"id":"summary","kind":"summary","text":"new"}],"columns":["lost header"]}`)
			}
			var result types.ToolResult
			var err error
			if patch {
				result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			} else {
				result, err = (&EmitAnswerDocument{}).Execute(bus, raw)
			}
			if err != nil || result.Success || result.Repair == nil || result.Repair.Code != "answer_doc_visible_payload_ownership" {
				t.Fatalf("patch=%v wrapped=%v: err=%v result=%+v", patch, wrapped, err, result)
			}
			if patch && bus.Mutable.AnswerDocumentV2().Blocks[0].Text != "original" {
				t.Fatal("rejected patch mutated live answer")
			}
		}
	}
}
