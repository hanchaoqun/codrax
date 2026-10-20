package tool

import (
	"encoding/json"
	"html"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Independent review uses the real SQLite original -> preparation -> public
// trace_query -> current contract -> public answer adapter. No fixed-fact
// witness or measurement payload is manufactured by these fixtures.
func rawReviewContextHMC268(t *testing.T, side string) (*types.BusContext, types.ToolResult, string) {
	t.Helper()
	bus, _, _ := hmc17NamedPathContext(t)
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_capture_metadata/" + side + ".data")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bus.RepoRoot, side+".data")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 2.0
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	rm.DiagramHint = &types.DiagramHint{Kind: types.DiagramFlow}
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1 to 2 seconds"}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}
	bus.Mutable.SetRequestModel(rm)
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": 2})
	if !result.Success {
		t.Fatal(result.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return bus, result, path
}

func rawReviewChoicesHMC268(t *testing.T, bus *types.BusContext) []types.RuntimeMeasurementTable {
	t.Helper()
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	view.OptionalBlocks = append(view.OptionalBlocks, types.BlockRequirement{Kind: types.BlockDiagram})
	view.DiagramPlan = &types.DiagramFacetGraph{Kind: types.DiagramFlow}
	return view.RuntimeMeasurementContract.Choices()
}

func rawReviewSelectorHMC268(table types.RuntimeMeasurementTable, kind string) map[string]any {
	return map[string]any{"id": "facts", "kind": kind, "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
}

func rawReviewMembersHMC268(t *testing.T, tables []types.RuntimeMeasurementTable) types.RuntimeMeasurementTable {
	t.Helper()
	for _, table := range tables {
		if table.View == types.RuntimeMeasurementMembers {
			return table
		}
	}
	t.Fatal("fixture did not expose actual raw members")
	return types.RuntimeMeasurementTable{}
}

func TestHMC268ReviewRawNativeSummaryAndDisconnectedDiagram(t *testing.T) {
	for _, side := range []string{"baseline", "current"} {
		t.Run(side, func(t *testing.T) {
			bus, _, _ := rawReviewContextHMC268(t, side)
			tables := rawReviewChoicesHMC268(t, bus)
			members := rawReviewMembersHMC268(t, tables)
			wantRows := map[string]int{"baseline": 13, "current": 8}[side]
			if len(members.Rows) != wantRows {
				t.Fatalf("fixture selected %d rows, want %d", len(members.Rows), wantRows)
			}
			beforeLedger, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0)))
			metadata := 0
			for _, table := range tables {
				if strings.HasPrefix(table.ObservationID, "trace_source_metadata:") {
					metadata++
				}
				for _, kind := range []string{"summary", "diagram"} {
					t.Run(string(table.View)+"/"+table.ObservationID+"/"+kind, func(t *testing.T) {
						blocks := []any{rawReviewSelectorHMC268(table, kind)}
						if kind == "diagram" {
							lead := rawReviewSelectorHMC268(table, "summary")
							lead["id"] = "lead"
							blocks = append([]any{lead}, blocks...)
						}
						// There is deliberately no model-authored copy of the values.
						out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": blocks}, false)
						if !out.Success {
							t.Fatalf("native selector-only %s was not accepted: %s", kind, out.Summary)
						}
						doc := bus.Mutable.AnswerDocumentV2()
						got := blockByID(t, doc, "facts")
						if got.RuntimeMeasurement == nil || !got.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(*got.RuntimeMeasurement.BoundTable, table) {
							t.Fatal("adapter did not bind the exact current source table")
						}
						visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
						if strings.Contains(visible, "-->") || strings.Contains(visible, "==>") || strings.Contains(visible, "-.->") {
							t.Fatal("independent observations were upgraded to an edge")
						}
						if kind == "diagram" && !strings.Contains(visible, "flowchart TB") {
							t.Fatal("fact diagram was not actually rendered")
						}
						wire, _ := json.Marshal(doc)
						if strings.Contains(string(wire), "334200000.5") || strings.Contains(string(wire), "physical_frame_rate") {
							t.Fatal("native facts leaked into the model-owned JSON selector")
						}
					})
				}
			}
			if metadata != 1 {
				t.Fatalf("actual capture metadata selector count=%d", metadata)
			}
			afterLedger, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0)))
			if string(beforeLedger) != string(afterLedger) {
				t.Fatal("presentation changed the original observation/authority ledger")
			}
		})
	}
}

func TestHMC268ReviewRawLiteralUnitsAndMetadataQualification(t *testing.T) {
	bus, _, _ := rawReviewContextHMC268(t, "current")
	tables := rawReviewChoicesHMC268(t, bus)
	var metadata types.RuntimeMeasurementTable
	for _, table := range tables {
		if strings.HasPrefix(table.ObservationID, "trace_source_metadata:") {
			metadata = table
		}
	}
	for _, table := range []types.RuntimeMeasurementTable{rawReviewMembersHMC268(t, tables), metadata} {
		out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{rawReviewSelectorHMC268(table, "summary")}}, false)
		if !out.Success {
			t.Fatal(out.Summary)
		}
		visible := html.UnescapeString(render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh"))
		if strings.HasPrefix(table.ObservationID, "trace_source_metadata:") {
			for _, want := range []string{"physical_frame_rate", "invalid", "60", "device_name"} {
				if !strings.Contains(visible, want) {
					t.Fatalf("metadata literal/qualification %q omitted", want)
				}
			}
		} else {
			// SQLite REAL may be formatted as an exact scientific literal.
			// Verify its value independently, then require the untouched cell.
			foundReal := false
			want, _ := new(big.Rat).SetString("250000000.25")
			for _, row := range table.Rows {
				if !strings.HasSuffix(row[7], " (real)") {
					continue
				}
				got, ok := new(big.Rat).SetString(strings.TrimSuffix(row[7], " (real)"))
				if !ok || got.Cmp(want) != 0 || !strings.Contains(visible, row[7]) {
					t.Fatalf("original REAL value changed: %q", row[7])
				}
				foundReal = true
			}
			if !foundReal || !strings.Contains(visible, "9007199254740995") {
				t.Fatal("exact REAL or large INTEGER omitted")
			}
		}
		for _, forbidden := range []string{"60Hz", "60 Hz", "334.2 MHz", "334.2000005 MHz", "状态翻转", "设备不同"} {
			if strings.Contains(visible, forbidden) {
				t.Fatalf("raw fields gained unverified physical semantics: %q", forbidden)
			}
		}
	}
}

func TestHMC268ReviewRawSourceWitnessCannotBeReplayed(t *testing.T) {
	for _, mode := range []string{"json", "observation_json_with_current_source", "memo", "cancelled", "reset", "request_window", "same_bytes_new_generation", "cross_source", "observation_mutation", "row_mutation", "note_mutation", "state_mutation", "unit_mutation", "other_source_rows_same_id"} {
		t.Run(mode, func(t *testing.T) {
			bus, result, path := rawReviewContextHMC268(t, "baseline")
			original := rawReviewMembersHMC268(t, rawReviewChoicesHMC268(t, bus))
			if !original.HasNativeFixedFacts() {
				t.Fatal("native positive control must qualify before replay negatives are meaningful")
			}
			switch mode {
			case "json":
				raw, _ := json.Marshal(result)
				result = types.ToolResult{}
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatal(err)
				}
			case "memo":
				result.ReusedFromRunMemo = true
			case "observation_json_with_current_source":
				raw, _ := json.Marshal(result.Observations)
				result.Observations = nil
				if err := json.Unmarshal(raw, &result.Observations); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				result.TraceViewCancellation = &types.TraceViewCancellation{}
			case "reset":
				bus.Mutable.ResetTurnAArtifacts()
			case "request_window":
				rm := bus.AnalysisIR.RequestModel
				start, end := 1.1, 1.9
				rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "new window"}
				bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
				bus.Mutable.SetRequestModel(rm)
			case "same_bytes_new_generation":
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path+".replacement", body, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(path+".replacement", path); err != nil {
					t.Fatal(err)
				}
			case "cross_source":
				bus, _, _ = rawReviewContextHMC268(t, "current")
			case "observation_mutation":
				changed := false
				for i := range result.Observations {
					if result.Observations[i].Predicate == TraceMeasurementsPredicate {
						result.Observations[i].SourceRef.Path = filepath.Join(bus.RepoRoot, "unread.data")
						result.Observations[i].Subject = "unverified physical GPU"
						changed = true
					}
				}
				if !changed {
					t.Fatal("fixture lacks native raw publication")
				}
			case "row_mutation", "note_mutation", "state_mutation", "unit_mutation", "other_source_rows_same_id":
				changed := false
				for i := range result.Observations {
					r := &result.Observations[i]
					if r.Predicate != TraceMeasurementsPredicate {
						continue
					}
					if mode == "state_mutation" {
						r.Object = "not_executed"
						changed = true
						continue
					}
					r.RichNotes = append([]string(nil), r.RichNotes...)
					for j, note := range r.RichNotes {
						prefix := types.TraceNoteKeyRuntimeMeasurement + "="
						if !strings.HasPrefix(note, prefix) {
							continue
						}
						var p types.RuntimeMeasurementPublication
						if err := json.Unmarshal([]byte(strings.TrimPrefix(note, prefix)), &p); err != nil {
							t.Fatal(err)
						}
						for k := range p.Tables {
							table := &p.Tables[k]
							switch mode {
							case "row_mutation":
								table.Rows[0][0] = "invented record"
							case "note_mutation":
								table.Notes = append(table.Notes, "Physical identity is verified.")
							case "unit_mutation":
								table.Columns[len(table.Columns)-1] = "physical frequency (Hz)"
							case "other_source_rows_same_id":
								table.Rows[0][0] = filepath.Join(bus.RepoRoot, "current.data") + " / 1 / 1"
							}
						}
						encoded, err := json.Marshal(p)
						if err != nil {
							t.Fatal(err)
						}
						r.RichNotes[j] = prefix + string(encoded)
						changed = true
					}
				}
				if !changed {
					t.Fatal("fixture publication was not mutated")
				}
			}
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{rawReviewSelectorHMC268(original, "summary")}}, false)
			if out.Success {
				t.Fatalf("%s minted a current fixed-fact receipt", mode)
			}
		})
	}
}

func TestHMC268ReviewDualSourcesAndFailedSideRemainIndependent(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "two_current_sources", true: "one_missing_source"}[missing], func(t *testing.T) {
			bus, _, baseline := rawReviewContextHMC268(t, "baseline")
			body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_capture_metadata/current.data")
			if err != nil {
				t.Fatal(err)
			}
			current := filepath.Join(bus.RepoRoot, "current.data")
			if !missing {
				if err := os.WriteFile(current, body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			rm := bus.AnalysisIR.RequestModel
			rm.RuntimeArtifactScopeProfile = hmc227SharedScope(baseline, current, 1, 2)
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
			bus.Mutable.SetRequestModel(rm)
			args, _ := json.Marshal(map[string]any{"comparison": map[string]any{
				"baseline": map[string]any{"source": "path", "path": baseline, "view": "measurements", "time_start": 1, "time_end": 2},
				"current":  map[string]any{"source": "path", "path": current, "view": "measurements", "time_start": 1, "time_end": 2},
			}})
			result, err := (&TraceQuery{skipNativeMeasurementMemo: true}).Execute(bus, args)
			if err != nil || !result.Success {
				t.Fatalf("actual dual query: %v %s", err, result.Summary)
			}
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			report, ok := result.RuntimeMeasurementPair.Report()
			if !ok {
				t.Fatal("actual current dual report missing")
			}
			roles := map[string]string{}
			originalRows := map[string][][]string{}
			for _, side := range report.Sides {
				for _, publication := range side.Publications {
					roles[publication.ObservationID] = side.Role
					for _, table := range publication.Tables {
						if table.View == types.RuntimeMeasurementMembers {
							originalRows[publication.ObservationID] = table.Rows
						}
					}
				}
			}
			tables := rawReviewChoicesHMC268(t, bus)
			counts := map[string]int{}
			statusSeen := false
			for _, table := range tables {
				if table.View == types.RuntimeMeasurementMembers {
					role := roles[table.ObservationID]
					if role != "baseline" && role != "current" || !reflect.DeepEqual(table.Rows, originalRows[table.ObservationID]) {
						t.Fatal("raw member rows lost exact query-to-original role binding")
					}
					counts[role] += len(table.Rows)
				}
				if len(table.Rows) == 2 && len(table.Rows[0]) >= 2 && table.Rows[0][0] == "baseline" && table.Rows[1][0] == "current" {
					statusSeen = true
					if table.Rows[0][1] != "available" || (table.Rows[1][1] == "available") == missing {
						t.Fatalf("current execution state lost: %+v", table.Rows)
					}
				}
				out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{rawReviewSelectorHMC268(table, "summary")}}, false)
				if !out.Success {
					t.Fatalf("current pair/status summary failed: %s", out.Summary)
				}
				if !reflect.DeepEqual(*blockByID(t, bus.Mutable.AnswerDocumentV2(), "facts").RuntimeMeasurement.BoundTable, table) {
					t.Fatal("pair summary mixed source/role/state/metadata")
				}
			}
			wantCurrent := 8
			if missing {
				wantCurrent = 0
			}
			if counts["baseline"] != 13 || counts["current"] != wantCurrent || !statusSeen {
				t.Fatalf("source populations/status: %v status=%t", counts, statusSeen)
			}
		})
	}
}

func TestHMC268ReviewActualPreferredFrameRateProtocolKeepsHz(t *testing.T) {
	bus, _, _ := hmc17NamedPathContext(t)
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_preferred_frame_rate/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
	bus.Mutable.SetRequestModel(rm)
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "preferred_frame_rate", "time_start": 1, "time_end": 2})
	if !result.Success {
		t.Fatal(result.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	var table types.RuntimeMeasurementTable
	for _, candidate := range rawReviewChoicesHMC268(t, bus) {
		if candidate.View == types.RuntimeMeasurementSummary {
			table = candidate
			break
		}
	}
	if table.ObservationID == "" {
		t.Fatal("actual preferred-frame-rate summary missing")
	}
	out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{
		map[string]any{"id": "lead", "kind": "summary", "text": "记录中的协议字段如下。"}, rawReviewSelectorHMC268(table, "table"),
	}}, false)
	if !out.Success {
		t.Fatal(out.Summary)
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh"))
	if !strings.Contains(visible, "Hz") || !strings.Contains(visible, "PreferredFrameRate") {
		t.Fatal("genuine preferred-frame-rate protocol lost its independently declared Hz semantics")
	}
}

func TestHMC268ReviewRawSelectorRejectsCompetingOrCausalPayload(t *testing.T) {
	bus, _, _ := rawReviewContextHMC268(t, "baseline")
	table := rawReviewMembersHMC268(t, rawReviewChoicesHMC268(t, bus))
	for _, mode := range []string{"text", "columns", "diagram", "edge_anchors", "bound_table"} {
		t.Run(mode, func(t *testing.T) {
			good := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{rawReviewSelectorHMC268(table, "summary")}}, false)
			if !good.Success {
				t.Fatalf("positive native receipt needed before negative control: %s", good.Summary)
			}
			before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			block := rawReviewSelectorHMC268(table, "summary")
			switch mode {
			case "text":
				block[mode] = "Physical GPU operated at 334.2 MHz."
			case "columns":
				block[mode] = []string{"Physical frequency"}
			case "diagram":
				block[mode] = "flowchart TB\nA-->B"
			case "edge_anchors":
				block[mode] = []any{map[string]any{"from": "A", "to": "B", "observation_id": table.ObservationID}}
			case "bound_table":
				block["runtime_measurement"].(map[string]any)[mode] = table
			}
			out := b1659bExecuteAnswer(t, bus, map[string]any{"replace_blocks": []any{block}}, true)
			if out.Success {
				t.Fatalf("competing/causal model payload accepted: %s", mode)
			}
			after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			if string(before) != string(after) {
				t.Fatal("rejected patch mutated the accepted native answer")
			}
		})
	}
}
