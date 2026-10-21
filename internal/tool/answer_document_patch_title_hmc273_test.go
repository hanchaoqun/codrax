package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public dispatch projection against real prepared native facts.
// A full replacement is not a checklist of mutually exclusive payload fields:
// the envelope's teaching must agree with its source-owned selector condition.
func TestHMC273NativePatchTitleTeachingMatchesProjectedSchema(t *testing.T) {
	bus, _, _ := rawReviewContextHMC268(t, "current")
	ctx := &types.AgentContext{Mutable: bus.Mutable, AnalysisIR: bus.AnalysisIR}
	patch := &EmitAnswerDocumentPatch{}
	schema := patch.ParametersFor(ctx)
	var root struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &root); err != nil {
		t.Fatal(err)
	}
	texts := map[string]string{"description": patch.DescriptionFor(ctx)}
	for _, field := range []string{"replace_blocks", "add_blocks"} {
		texts[field] = root.Properties[field].Description
	}
	for surface, text := range texts {
		if !strings.Contains(text, "model-owned") || !strings.Contains(text, types.RuntimeMeasurementSourceTitleTeaching) {
			t.Errorf("%s still teaches an unconditional full payload instead of model-owned fields with source-title ownership: %s", surface, text)
		}
	}

	view := types.BuildAnswerSemanticViewForBusContext(bus)
	table := rawReviewMembersHMC268(t, view.RuntimeMeasurementContract.Choices())
	for _, field := range []string{"replace_blocks", "add_blocks"} {
		for _, kind := range []string{"table", "summary"} {
			for _, title := range []string{"", "Model heading must not replace the source"} {
				block := rawReviewSelectorHMC268(table, kind)
				if title != "" {
					block["title"] = title
				}
				raw, _ := json.Marshal(map[string]any{field: []any{block}})
				err := toolparam.Validate(raw, schema)
				if (err == nil) != (title == "") {
					t.Errorf("%s %s title=%q executable condition disagrees: %v", field, kind, title, err)
				}
			}
		}
		ordinary, _ := json.Marshal(map[string]any{field: []any{map[string]any{
			"id": "ordinary", "kind": "summary", "title": "Independent explanation", "text": "Preserve this title.",
		}}})
		if err := toolparam.Validate(ordinary, schema); err != nil {
			t.Errorf("%s rejects an ordinary model-owned title: %v", field, err)
		}
	}
}

func TestHMC273NativePatchKeepsOrdinaryTitleAndSelectorOwnership(t *testing.T) {
	bus, _, _ := rawReviewContextHMC268(t, "current")
	const title = "Independent explanation remains model-owned"
	ordinary := map[string]any{"id": "ordinary", "kind": "summary", "title": title, "text": "Initial explanation."}
	if out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{ordinary}}, false); !out.Success {
		t.Fatal(out.Summary)
	}
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	table := rawReviewMembersHMC268(t, view.RuntimeMeasurementContract.Choices())
	native := rawReviewSelectorHMC268(table, "table")
	if out := b1659bExecuteAnswer(t, bus, map[string]any{"add_blocks": []any{native}}, true); !out.Success {
		t.Fatal(out.Summary)
	}
	ordinary["text"] = "Updated independent explanation."
	if out := b1659bExecuteAnswer(t, bus, map[string]any{"replace_blocks": []any{ordinary}}, true); !out.Success {
		t.Fatal(out.Summary)
	}
	if got := blockByID(t, bus.Mutable.AnswerDocumentV2(), "ordinary"); got.Title != title || got.Text != ordinary["text"] {
		t.Fatal("full replacement changed a model-owned title or failed to update its text")
	}
	before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
	native["title"] = "An unrelated model heading"
	if out := b1659bExecuteAnswer(t, bus, map[string]any{"replace_blocks": []any{native}}, true); out.Success {
		t.Fatal("source-owned native selector accepted a model title")
	}
	after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
	if string(after) != string(before) {
		t.Fatal("rejected title patch changed the accepted answer")
	}
	delete(native, "title")
	if out := b1659bExecuteAnswer(t, bus, map[string]any{"replace_blocks": []any{native}}, true); !out.Success {
		t.Fatal(out.Summary)
	}
	if got := blockByID(t, bus.Mutable.AnswerDocumentV2(), "facts"); got.Title != "" || got.RuntimeMeasurement == nil || !got.RuntimeMeasurement.IsBound() {
		t.Fatal("title repair lost the exact current native selector")
	}
	if got := blockByID(t, bus.Mutable.AnswerDocumentV2(), "ordinary"); got.Title != title {
		t.Fatal("native selector repair changed an unrelated ordinary title")
	}
}
