package tool

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC230RepeatedNativeFactsQueryEmitRender(t *testing.T) {
	for _, order := range []string{"explicit_inherited", "inherited_explicit", "single_pair", "pair_single", "repeat", "process_explicit_inherited", "process_single_pair"} {
		t.Run(order, func(t *testing.T) {
			bus, _, _ := hmc17NamedPathContext(t)
			fixture, queryView, wantRows := "hmosperf_dual_capture_metadata/baseline.data", "measurements", 13
			if strings.HasPrefix(order, "process_") {
				order = strings.TrimPrefix(order, "process_")
				fixture, queryView, wantRows = "hmosperf_process_measurements/capture.data", "process_measurements", 11
			}
			body, err := os.ReadFile("../../eval/fixtures/" + fixture)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(bus.RepoRoot, "records.data")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			start, end := 1.0, 2.0
			rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
				RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1 to 2 seconds"},
			}
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}
			bus.Mutable.SetRequestModel(rm)
			bus.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "file"}}}
			query := func(inherited, pair bool) types.ToolResult {
				args := map[string]any{"source": "path", "path": path, "view": queryView}
				if !inherited {
					args["time_start"], args["time_end"] = 1, 2
				}
				if pair {
					args = map[string]any{"comparison": map[string]any{"baseline": args}}
				}
				encoded, _ := json.Marshal(args)
				result, err := (&TraceQuery{skipNativeMeasurementMemo: true}).Execute(bus, encoded)
				if err != nil {
					t.Fatal(err)
				}
				if !result.Success {
					t.Fatal(result.Summary)
				}
				return result
			}
			first := query(order == "inherited_explicit" || order == "pair_single", order == "pair_single")
			second := query(order != "inherited_explicit" && order != "pair_single", order == "single_pair")
			results := []types.ToolResult{first, second}
			if order == "repeat" {
				results = append(results, first, second)
			}
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0))
			var members []types.RuntimeMeasurementTable
			for _, table := range contract.Choices() {
				if table.View == types.RuntimeMeasurementMembers {
					members = append(members, table)
				}
			}
			if len(members) != 2 {
				t.Fatalf("keep both original selectors: members=%d", len(members))
			}
			for _, table := range members {
				if len(table.Rows) != wantRows || table.MemberSet != nil {
					t.Fatal("raw source population changed")
				}
			}
			for _, explicit := range []bool{false, true} {
				blocks := []any{map[string]any{"id": "lead", "kind": "summary", "text": "Model interpretation stays unchanged."}}
				if explicit {
					blocks = append(blocks, map[string]any{"id": "chosen", "kind": "table", "runtime_measurement": map[string]any{"observation_id": members[1].ObservationID, "view": "members"}})
				}
				out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": blocks}, false)
				if !out.Success {
					t.Fatal(out.Summary)
				}
				doc := bus.Mutable.AnswerDocumentV2()
				rows := 0
				for _, block := range doc.Blocks {
					if block.RuntimeMeasurement != nil && block.RuntimeMeasurement.View == types.RuntimeMeasurementMembers {
						rows += len(block.RuntimeMeasurement.BoundTable.Rows)
					}
				}
				if rows != wantRows {
					t.Fatalf("explicit=%v repeated exact source population rendered %d rows, want %d", explicit, rows, wantRows)
				}
				visible := render.RenderAnswerDocument(doc, "en")
				if !strings.Contains(visible, "Model interpretation stays unchanged.") {
					t.Fatal("model prose rewritten")
				}
				row := "| " + strings.Join(members[0].Rows[0], " | ") + " |"
				if strings.Count(html.UnescapeString(visible), row) != 1 {
					t.Fatal("render duplicated physical row")
				}
			}
			// Display reuse must not edit the original results or discard their diagnostics.
			var payloads []string
			for _, result := range results {
				for _, record := range result.Observations {
					payloads = append(payloads, record.SourceRef.PayloadRef)
				}
				if report, ok := result.RuntimeMeasurementPair.Report(); ok {
					for _, side := range report.Sides {
						for _, pub := range side.Publications {
							payloads = append(payloads, pub.Source.PayloadRef)
						}
					}
				}
			}
			inheritedDiagnostic := false
			for _, path := range payloads {
				if path == "" {
					continue
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				inheritedDiagnostic = inheritedDiagnostic || strings.Contains(string(raw), "trace_query_request_window_inherited")
			}
			if !inheritedDiagnostic {
				t.Fatal("inherited query diagnostic disappeared")
			}
		})
	}
}

func TestHMC230NativeFactReuseRequiresCurrentSourceRead(t *testing.T) {
	for _, mode := range []string{"memo", "json", "reset", "generation_swap"} {
		t.Run(mode, func(t *testing.T) {
			bus, result, path := hmc223NativeInvestigation(t, false)
			rm := types.RequestModel{RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}}}
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
			bus.Mutable.SetRequestModel(rm)
			find := func(c *types.RuntimeMeasurementContract) types.AnswerRuntimeMeasurementReceipt {
				for _, table := range c.Choices() {
					if table.View == types.RuntimeMeasurementMembers {
						return types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
					}
				}
				t.Fatal("native members unavailable")
				return types.AnswerRuntimeMeasurementReceipt{}
			}
			before := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0))
			original := find(before)
			if !strings.HasPrefix(types.RuntimeMeasurementDisplayKey(original, before), "native-population:") {
				t.Fatal("fresh public query lacks identity")
			}
			switch mode {
			case "memo":
				result.ReusedFromRunMemo = true
			case "json":
				encoded, _ := json.Marshal(result)
				result = types.ToolResult{}
				if err := json.Unmarshal(encoded, &result); err != nil {
					t.Fatal(err)
				}
			case "reset":
				bus.Mutable.ResetTurnAArtifacts()
				bus.Mutable.SetRequestModel(rm)
			case "generation_swap":
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path+".new", body, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(path+".new", path); err != nil {
					t.Fatal(err)
				}
			}
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			after := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0))
			if strings.HasPrefix(types.RuntimeMeasurementDisplayKey(original, after), "native-population:") {
				t.Fatal("stale/replayed facts retained current display reuse witness")
			}
			if mode == "generation_swap" || mode == "reset" {
				if mode == "generation_swap" {
					// A replaced admitted capture is intentionally refused in the
					// original run. Re-admit it in a new real preparation context.
					bus, _, _ = hmc17NamedPathContext(t)
					bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
					bus.Mutable.SetRequestModel(rm)
				}
				fresh := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": 2})
				if !fresh.Success {
					t.Fatal(fresh.Summary)
				}
				bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{fresh}})
				current := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0))
				if !strings.HasPrefix(types.RuntimeMeasurementDisplayKey(find(current), current), "native-population:") {
					t.Fatal("fresh generation lost independent reuse identity")
				}
				if mode == "generation_swap" && types.RuntimeMeasurementDisplayKey(find(current), current) == types.RuntimeMeasurementDisplayKey(original, before) {
					t.Fatal("identical bytes from a different physical generation reused old fact identity")
				}
			}
		})
	}
}

func TestHMC230DualCaptureMetadataKeepsTwentyOneNativeRows(t *testing.T) {
	bus, _, _ := hmc17NamedPathContext(t)
	paths := [2]string{}
	for i, name := range []string{"baseline", "current"} {
		body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_capture_metadata/" + name + ".data")
		if err != nil {
			t.Fatal(err)
		}
		paths[i] = filepath.Join(bus.RepoRoot, name+".data")
		if err := os.WriteFile(paths[i], body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}},
		RuntimeArtifactScopeProfile: hmc227SharedScope(paths[0], paths[1], 1, 2)}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}
	bus.Mutable.SetRequestModel(rm)
	var results []types.ToolResult
	query := func(args map[string]any) {
		raw, _ := json.Marshal(args)
		result, err := (&TraceQuery{skipNativeMeasurementMemo: true}).Execute(bus, raw)
		if err != nil || !result.Success {
			t.Fatalf("query: %v / %s", err, result.Summary)
		}
		results = append(results, result)
	}
	for _, path := range paths {
		query(map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": 2})
	}
	query(map[string]any{"comparison": map[string]any{
		"baseline": map[string]any{"source": "path", "path": paths[0], "view": "measurements"},
		"current":  map[string]any{"source": "path", "path": paths[1], "view": "measurements"},
	}})
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0))
	var selectors, availableRows int
	for _, table := range contract.Choices() {
		if table.View == types.RuntimeMeasurementMembers {
			selectors++
			availableRows += len(table.Rows)
		}
	}
	if selectors != 4 || availableRows != 42 {
		t.Fatalf("original full populations lost: selectors=%d rows=%d", selectors, availableRows)
	}
	out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "Independent sources remain independent."}}}, false)
	if !out.Success {
		t.Fatal(out.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	rows, tables := 0, 0
	for _, block := range doc.Blocks {
		if block.RuntimeMeasurement != nil && block.RuntimeMeasurement.View == types.RuntimeMeasurementMembers {
			tables++
			rows += len(block.RuntimeMeasurement.BoundTable.Rows)
		}
	}
	if tables != 2 || rows != 21 {
		t.Fatalf("duplicate live-eval population: displayed tables=%d rows=%d, want 2/21", tables, rows)
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(doc, "en"))
	for _, table := range contract.Choices() {
		if table.View == types.RuntimeMeasurementMembers {
			for _, row := range table.Rows {
				if strings.Count(visible, row[0]) != 1 {
					t.Fatalf("physical native row not displayed exactly once: %s", row[0])
				}
			}
		}
	}
}
