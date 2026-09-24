package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This exercises a legal provider-protocol boundary, not a claim that the
// native IO producer omits its scope/coverage notes. Keep actual query identity,
// rows and views, and shorten only the trusted publication's optional notes.
func TestRuntimeMeasurementHandoffShortProviderNotes(t *testing.T) {
	for _, noteCount := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("notes_%d", noteCount), func(t *testing.T) {
			ctx, result, native := ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeBoundedFactSet, "")
			beforeView := types.BuildAnswerSemanticViewForAgentContext(ctx)
			beforeChoices := beforeView.RuntimeMeasurementContract.Choices()
			if len(native.Groups) != 4 || len(beforeChoices) != 27 {
				t.Fatal("actual native producer lost old 12 paired or new 15 activity views")
			}
			assertRuntimeMeasurementFixturePopulations(t, result, beforeChoices)
			beforeSchemas := []json.RawMessage{tool.BuildAnswerDocumentParametersFor(beforeView), tool.BuildAnswerDocumentPatchParametersFor(beforeView)}
			modified := map[string]int{}
			for i := range result.Observations {
				r := &result.Observations[i]
				publication, ok := types.DecodeRuntimeMeasurementPublication(*r)
				if !ok {
					continue
				}
				for j := range publication.Tables {
					table := &publication.Tables[j]
					if len(table.Notes) < noteCount {
						t.Fatal("native fixture unexpectedly lacks notes before protocol substitution")
					}
					table.Notes = append([]string(nil), table.Notes[:noteCount]...)
					if !table.IsValid() {
						t.Fatal("optional-note boundary is no longer a valid provider table")
					}
				}
				encoded, err := json.Marshal(publication)
				if err != nil {
					t.Fatal(err)
				}
				for j, note := range r.RichNotes {
					if strings.HasPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"=") {
						r.RichNotes[j] = types.TraceNoteKeyRuntimeMeasurement + "=" + string(encoded)
						modified[r.Predicate]++
					}
				}
				if _, ok := types.DecodeRuntimeMeasurementPublication(*r); !ok {
					t.Fatal("short optional notes invalidated the exact source-bound receipt")
				}
			}
			if !reflect.DeepEqual(modified, map[string]int{"io_inflight": 4, "io_activity": 5}) {
				t.Fatalf("shortened publications changed populations: got=%v", modified)
			}
			ctx.Mutable.ResetDispatchToolResults()
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			choices := view.RuntimeMeasurementContract.Choices()
			if len(choices) != len(beforeChoices) {
				t.Fatalf("short notes changed the selectable population: %d -> %d", len(beforeChoices), len(choices))
			}
			assertRuntimeMeasurementFixturePopulations(t, result, choices)
			// Calling the actual handoff must not panic for any legal note count.
			beforeRender, _ := json.Marshal(result)
			prompt := renderAnswerDocRuntimeMeasurementChoices(ctx)
			for i, table := range choices {
				want := beforeChoices[i].Clone()
				want.Notes = append([]string(nil), want.Notes[:noteCount]...)
				if !reflect.DeepEqual(table, want) {
					t.Fatalf("short notes changed native identity, rows or view: %+v", table)
				}
				selector := fmt.Sprintf("observation_id=%q view=%q", table.ObservationID, table.View)
				if strings.Count(prompt, selector) != 1 {
					t.Fatalf("handoff lost/duplicated short-note selector %s", selector)
				}
				for schemaIndex, schema := range []json.RawMessage{tool.BuildAnswerDocumentParametersFor(view), tool.BuildAnswerDocumentPatchParametersFor(view)} {
					if !bytes.Equal(schema, beforeSchemas[schemaIndex]) {
						t.Fatal("optional provider notes changed the model-facing schema contract")
					}
					key := "blocks"
					if schemaIndex == 1 {
						key = "replace_blocks"
					}
					block := map[string]any{"id": "measurement", "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
					payload := map[string]any{key: []any{block}}
					raw, _ := json.Marshal(payload)
					if err := toolparam.Validate(raw, schema); err != nil {
						t.Fatalf("valid selector rejected with short notes: %v", err)
					}
					block["columns"] = []string{"model-authored replacement"}
					raw, _ = json.Marshal(payload)
					if err := toolparam.Validate(raw, schema); err == nil {
						t.Fatal("short notes waived the selector-only table contract")
					}
				}
			}
			for _, text := range []string{"not a dependency or root-cause proof", "No source evidence_items are needed", "The system fills data", "Preview omissions never mean absent events"} {
				if !strings.Contains(prompt, text) {
					t.Fatalf("short notes lost shared selector teaching: %s", text)
				}
			}
			afterRender, _ := json.Marshal(result)
			if !bytes.Equal(beforeRender, afterRender) || ctx.Mutable.TraceRootCauseReport() != nil {
				t.Fatal("display handoff mutated the query or acquired causal authority")
			}
		})
	}
}
