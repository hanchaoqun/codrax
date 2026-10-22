package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc278MeasurementNoteDefinitions(t *testing.T, text string) map[string]string {
	t.Helper()
	definitions := map[string]string{}
	values := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, "- native_measurement_note "); ok {
			id, raw, ok := strings.Cut(rest, "=")
			var note string
			if !ok || json.Unmarshal([]byte(raw), &note) != nil || definitions[id] != "" || values[note] {
				t.Fatalf("invalid/duplicate exact note definition: %s", line)
			}
			definitions[id], values[note] = note, true
		}
		for _, key := range []string{"  scope_refs=", "  boundary_refs="} {
			if raw, ok := strings.CutPrefix(line, key); ok {
				var refs []string
				if err := json.Unmarshal([]byte(raw), &refs); err != nil {
					t.Fatal(err)
				}
				for _, ref := range refs {
					if _, ok := definitions[ref]; !ok {
						t.Fatalf("reference precedes complete definition: %q", ref)
					}
				}
			}
		}
	}
	return definitions
}

func hmc278MeasurementSelectorNotes(t *testing.T, text string, table types.RuntimeMeasurementTable, definitions map[string]string) []string {
	t.Helper()
	header := fmt.Sprintf("- observation_id=%q view=%q ", table.ObservationID, table.View)
	active := false
	var notes []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "- observation_id=") || strings.HasPrefix(line, "- native_table_reuse ") {
			active = strings.HasPrefix(line, header)
		}
		if !active {
			continue
		}
		for _, key := range []string{"  scope_refs=", "  boundary_refs="} {
			if raw, ok := strings.CutPrefix(line, key); ok {
				var refs []string
				if err := json.Unmarshal([]byte(raw), &refs); err != nil {
					t.Fatal(err)
				}
				for _, ref := range refs {
					notes = append(notes, definitions[ref])
				}
			}
		}
	}
	return notes
}

func TestHMC278MeasurementNotesActualFinalizerExactDictionary(t *testing.T) {
	for _, mode := range []string{"different_query_window", "different_physical_source"} {
		t.Run(mode, func(t *testing.T) {
			first, second := hmc275IOPath(t), ""
			second = first
			end := 1.009
			if mode == "different_physical_source" {
				second, end = hmc275IOPath(t), 1.01
			}
			ctx, results := hmc275IOContext(t, []string{first, second}, []map[string]any{{"time_start": 1.0, "time_end": 1.01}, {"time_start": 1.0, "time_end": end}})
			before, _ := json.Marshal(results)
			causal := hmc218CausalProjection(t, ctx)
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			choices := view.RuntimeMeasurementContract.Choices()
			messages := dependencyObservationMessages(t, ctx)
			definitions := hmc278MeasurementNoteDefinitions(t, messages)
			if len(definitions) == 0 {
				t.Fatal("actual Finalizer still expands repeated measurement notes without exact references")
			}
			if strings.Contains(messages, "native_table_reuse") {
				t.Fatal("note reuse merged independent query/table previews")
			}
			previews := hmc275MessagePreviews(t, messages)
			rows, repeatedBytes := 0, 0
			for _, p := range previews {
				rows += len(p.Rows)
			}
			if rows > 128 || len(choices) != 54 || len(previews) != 54 {
				t.Fatalf("note sharing changed row/selector population: rows=%d choices=%d previews=%d", rows, len(choices), len(previews))
			}
			var blocks []types.AnswerBlock
			for _, table := range choices {
				got := hmc278MeasurementSelectorNotes(t, messages, table, definitions)
				if len(got) < 2 || !reflect.DeepEqual(got[:2], table.Notes[:2]) {
					t.Fatalf("source/window scope lost or borrowed: %s/%s got=%q want=%q", table.ObservationID, table.View, got, table.Notes[:2])
				}
				for _, note := range got {
					repeatedBytes += len(note)
				}
				if table.View == types.RuntimeMeasurementSummary {
					if !reflect.DeepEqual(got, table.Notes) {
						t.Fatal("complete summary notes changed in actual Finalizer")
					}
					blocks = append(blocks, types.AnswerBlock{ID: fmt.Sprintf("native-%d", len(blocks)), Kind: types.BlockTable, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}})
				}
			}
			uniqueBytes := 0
			for _, note := range definitions {
				uniqueBytes += len(note)
			}
			if uniqueBytes >= repeatedBytes {
				t.Fatal("fixture did not exercise real repeated-note savings")
			}
			t.Logf("actual Finalizer note text bytes: expanded=%d dictionary=%d saved=%d; selectors=%d preview_rows=%d", repeatedBytes, uniqueBytes, repeatedBytes-uniqueBytes, len(choices), rows)
			blocks = append([]types.AnswerBlock{{ID: "intro", Kind: types.BlockSummary, Text: "各查询分别呈现自身窗口内的原生事实，不据此推断线程等待或根因。"}}, blocks...)
			for _, patch := range []bool{false, true} {
				key := "blocks"
				if patch {
					key = "replace_blocks"
				}
				args, _ := json.Marshal(map[string]any{key: blocks})
				var result types.ToolResult
				var err error
				if patch {
					result, err = (&tool.EmitAnswerDocumentPatch{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), args)
				} else {
					result, err = (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), args)
				}
				if err != nil || !result.Success {
					t.Fatalf("full original table binding patch=%t: %v %s", patch, err, result.Summary)
				}
				for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
					if block.RuntimeMeasurement == nil {
						continue
					}
					for _, table := range choices {
						if table.ObservationID == block.RuntimeMeasurement.ObservationID && table.View == block.RuntimeMeasurement.View && (!reflect.DeepEqual(table.Rows, block.RuntimeMeasurement.BoundTable.Rows) || !reflect.DeepEqual(table.Notes, block.RuntimeMeasurement.BoundTable.Notes)) {
							t.Fatal("display dictionary replaced original rows/notes at emit/patch")
						}
					}
				}
			}
			after, _ := json.Marshal(results)
			if !bytes.Equal(before, after) || !bytes.Equal(causal, hmc218CausalProjection(t, ctx)) {
				t.Fatal("note presentation mutated native evidence or causal projection")
			}
		})
	}
}

func TestHMC278MeasurementNotesShortAndExactBytes(t *testing.T) {
	for _, notes := range [][]string{nil, {""}, {"scope only"}, {"same text", "same text "}, {"scope", "window", "line one\nline two\t\"quoted\"", "same text", "same text "}} {
		t.Run(fmt.Sprintf("notes_%d_%q", len(notes), strings.Join(notes, "/")), func(t *testing.T) {
			ctx, result, _ := ioInFlightPublicContext(t, "en", types.RuntimeQuestionScopeBoundedFactSet, "")
			for i := range result.Observations {
				row := &result.Observations[i]
				p, ok := types.DecodeRuntimeMeasurementPublication(*row)
				if !ok {
					continue
				}
				for j := range p.Tables {
					p.Tables[j].Notes = append([]string(nil), notes...)
				}
				raw, _ := json.Marshal(p)
				for j, note := range row.RichNotes {
					if strings.HasPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"=") {
						row.RichNotes[j] = types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)
					}
				}
			}
			ctx.Mutable.ResetDispatchToolResults()
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
			messages := dependencyObservationMessages(t, ctx)
			definitions := hmc278MeasurementNoteDefinitions(t, messages)
			if len(definitions) != len(notes) {
				t.Fatalf("complete exact notes lost/normalized: got=%q want=%q", definitions, notes)
			}
			for _, table := range choices {
				if table.View == types.RuntimeMeasurementSummary {
					got := hmc278MeasurementSelectorNotes(t, messages, table, definitions)
					if !reflect.DeepEqual(got, notes) {
						t.Fatalf("note order/content changed: got=%q want=%q", got, notes)
					}
				}
			}
		})
	}
}
