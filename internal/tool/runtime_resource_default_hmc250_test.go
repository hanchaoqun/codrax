package tool

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func resourceDefaultsHMC250(doc *types.AnswerDocumentV2) (summaries, members, system int) {
	for _, block := range doc.Blocks {
		if block.RuntimeMeasurement == nil || !block.RuntimeMeasurement.IsBound() || !block.RuntimeMeasurement.BoundTable.HasNativeResourceFacts() {
			continue
		}
		if block.RuntimeMeasurement.View == types.RuntimeMeasurementSummary {
			summaries++
		} else if block.RuntimeMeasurement.View == types.RuntimeMeasurementMembers {
			members++
		}
		if block.SystemGeneratedKind == types.AnswerSystemGeneratedNativeFacts {
			system++
		}
	}
	return
}

func TestHMC250ResourceDefaultPublisherShape(t *testing.T) {
	_, result, _ := resourceMeasurementContextHMC249(t, false, 101)
	var original types.ObservationRecord
	for _, r := range result.Observations {
		if r.Predicate == TraceResourceStackPredicate {
			original = r
		}
	}
	pub, ok := types.DecodeRuntimeMeasurementPublication(original)
	if !ok {
		t.Fatal("native source publication missing")
	}
	base := original
	base.RichNotes = nil
	for _, note := range original.RichNotes {
		if !strings.HasPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"=") {
			base.RichNotes = append(base.RichNotes, note)
		}
	}
	// Decode public tables afresh so a mutation cannot merely fail because an
	// earlier table's private digest is stale. This exercises publisher policy.
	wire, _ := json.Marshal(pub.Tables)
	for _, tc := range []struct {
		name             string
		summary, members bool
		want             bool
	}{
		{"summary_only", true, false, true},
		{"no_default", false, false, false},
		{"members_only", false, true, false},
		{"two_defaults", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tables []types.RuntimeMeasurementTable
			if json.Unmarshal(wire, &tables) != nil {
				t.Fatal("decode tables")
			}
			tables[0].DefaultPresentation, tables[1].DefaultPresentation = tc.summary, tc.members
			r, published := types.PublishRuntimeResourceMeasurement(base, tables, false)
			if published != tc.want {
				t.Fatalf("published=%t want=%t", published, tc.want)
			}
			if published {
				if _, decoded := types.DecodeRuntimeMeasurementPublication(r); !decoded {
					t.Fatal("legitimate fixed default shape did not decode")
				}
				encoded, _ := json.Marshal(r)
				var restored types.ObservationRecord
				_ = json.Unmarshal(encoded, &restored)
				if _, decoded := types.DecodeRuntimeMeasurementPublication(restored); decoded {
					t.Fatal("JSON revived native default authority")
				}
			}
		})
	}
}

func TestHMC250ResourceDefaultSelectionAndPatch(t *testing.T) {
	bus, _, view := resourceMeasurementContextHMC249(t, false, 101)
	const prose = "模型的独立解释不由默认表替换。"
	lead := map[string]any{"id": "lead", "kind": "summary", "text": prose}
	var id string
	for _, table := range resourceNativeTablesHMC249(view) {
		id = table.ObservationID
	}
	for _, mode := range []string{"none", "summary", "members"} {
		t.Run(mode, func(t *testing.T) {
			blocks := []any{lead}
			if mode != "none" {
				blocks = append(blocks, map[string]any{"id": "selected", "kind": "table", "runtime_measurement": map[string]any{"observation_id": id, "view": mode}})
			}
			out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": blocks}, false)
			if !out.Success {
				t.Fatal(out.Summary)
			}
			doc := bus.Mutable.AnswerDocumentV2()
			summary, members, system := resourceDefaultsHMC250(doc)
			wantMembers, wantSystem := 0, 1
			if mode == "members" {
				wantMembers = 1
			} else if mode == "summary" {
				wantSystem = 0
			}
			if summary != 1 || members != wantMembers || system != wantSystem {
				t.Fatalf("summary/members/system=%d/%d/%d", summary, members, system)
			}
			unchanged := []string{}
			for _, block := range doc.Blocks {
				if block.ID != "lead" {
					unchanged = append(unchanged, block.ID)
				}
			}
			out = b1659bExecuteAnswer(t, bus, map[string]any{"replace_blocks": []any{lead}, "unchanged_block_ids": unchanged}, true)
			if !out.Success {
				t.Fatal(out.Summary)
			}
			a, b, c := resourceDefaultsHMC250(bus.Mutable.AnswerDocumentV2())
			if a != summary || b != members || c != system || bus.Mutable.AnswerDocumentV2().Blocks[0].Text != prose {
				t.Fatal("patch dropped/duplicated defaults or changed model interpretation")
			}
		})
	}
}

func TestHMC250ResourceDefaultEmptyAndBudget(t *testing.T) {
	for _, mode := range []string{"empty", "event_limit", "frame_budget"} {
		t.Run(mode, func(t *testing.T) {
			var edits []func(*sql.DB)
			pid, nested := 202, true
			if mode == "event_limit" {
				pid, nested = 101, false
			}
			if mode == "frame_budget" {
				pid, nested = 101, false
				edits = append(edits, func(db *sql.DB) {
					if _, err := db.Exec("UPDATE data_dict SET data=? WHERE id=100", strings.Repeat("<", 4096)); err != nil {
						t.Fatal(err)
					}
				})
			}
			bus, _, _ := resourceMeasurementContextHMC249(t, nested, pid, edits...)
			if mode == "event_limit" {
				raw, _ := json.Marshal(map[string]any{"source": "path", "path": filepath.Join(bus.RepoRoot, "capture.data"), "view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05, "limit": 1})
				result, err := (&TraceQuery{}).Execute(bus, raw)
				if err != nil || !result.Success {
					t.Fatalf("limited query: %v %s", err, result.Summary)
				}
				bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
				bus.Mutable.ResetDispatchToolResults()
			}
			out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "只读原记录。"}}}, false)
			if !out.Success {
				t.Fatal(out.Summary)
			}
			doc := bus.Mutable.AnswerDocumentV2()
			a, b, c := resourceDefaultsHMC250(doc)
			if a != 1 || b != 0 || c != 1 {
				t.Fatalf("defaults=%d/%d/%d", a, b, c)
			}
			visible := render.RenderAnswerDocument(doc, "zh")
			if mode == "empty" {
				if !strings.Contains(visible, "无匹配记录") || strings.Contains(visible, "background") || strings.Contains(visible, "线程ID=0") {
					t.Fatal("empty query lost its boundary or borrowed an observed identity")
				}
			} else if mode == "event_limit" {
				if !strings.Contains(visible, "原查询匹配资源事件=3；保留事件=1；原交接省略事件=2") {
					t.Fatal("event limit recast the original population or hid omission")
				}
			} else if !strings.Contains(visible, "展示省略帧=1") && !strings.Contains(visible, "展示省略帧=2") && !strings.Contains(visible, "展示省略帧=3") {
				t.Fatal("default summary hid the actual frame budget omission")
			}
		})
	}
}

func TestHMC250ResourceDefaultKeepsDomainAndSourceBoundary(t *testing.T) {
	for _, mode := range []string{"source", "clock", "window", "json", "relation", "source_location"} {
		t.Run(mode, func(t *testing.T) {
			bus, result, _ := resourceMeasurementContextHMC249(t, false, 101)
			switch mode {
			case "source", "clock":
				for i := range result.Observations {
					if mode == "source" {
						result.Observations[i].SourceRef.Path += ".foreign"
					} else {
						result.Observations[i].SourceRef.TimeDomain = "other"
					}
				}
			case "window":
				start, end := 11.0, 12.0
				bus.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[11,12)"}
			case "json":
				raw, _ := json.Marshal(result)
				var clean types.ToolResult
				_ = json.Unmarshal(raw, &clean)
				result = clean
			case "relation", "source_location":
				role := types.RequestedAnswerDimensionRelationPath
				if mode == "source_location" {
					role = types.RequestedAnswerDimensionSourceLocation
				}
				bus.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
				bus.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions[0].Role = role
			}
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			bus.Mutable.ResetDispatchToolResults()
			out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "解释保留。"}}}, false)
			if !out.Success {
				t.Fatal(out.Summary)
			}
			if a, b, c := resourceDefaultsHMC250(bus.Mutable.AnswerDocumentV2()); a+b+c != 0 {
				t.Fatalf("%s acquired resource default %d/%d/%d", mode, a, b, c)
			}
		})
	}
}
