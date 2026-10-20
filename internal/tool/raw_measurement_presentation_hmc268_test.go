package tool

import (
	"context"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

// No observation or table is fabricated: the ordinary named-source tool
// performs real preparation and queries the same SQLite fixture as the audit.
func rawPresentationFixtureHMC268(t *testing.T, fixture, queryView string, pair bool) *types.BusContext {
	t.Helper()
	bus, preparer, _ := hmc17NamedPathContext(t)
	body, err := os.ReadFile("../../eval/fixtures/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bus.RepoRoot, "capture.data")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}}}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}
	bus.Mutable.SetRequestModel(rm)
	args := map[string]any{"source": "path", "path": path, "view": queryView, "time_start": 1, "time_end": 2}
	if queryView == "event_search" {
		delete(args, "time_start")
		delete(args, "time_end")
		args["event_types"] = []string{"measure_interval"}
	}
	if pair {
		args = map[string]any{"comparison": map[string]any{"current": args}}
	}
	result := hmc17NamedQuery(t, bus, args)
	if !result.Success {
		t.Fatal(result.Summary)
	}
	if len(preparer.PreparedMaterials()) != 1 {
		t.Fatal("real preparation did not retain its material")
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return bus
}

func TestHMC268RawAcquisitionStatusDoesNotResealChangedState(t *testing.T) {
	bus, _, _ := hmc17NamedPathContext(t)
	var paths [2]string
	for i, side := range []string{"baseline", "current"} {
		body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_measurements/" + side + ".data")
		if err != nil {
			t.Fatal(err)
		}
		paths[i] = filepath.Join(bus.RepoRoot, side+".data")
		if err := os.WriteFile(paths[i], body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	comparison := &types.RuntimeArtifactComparisonScope{
		Baseline: types.RuntimeArtifactComparisonSide{Path: paths[0], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[0]}},
		Current:  types.RuntimeArtifactComparisonSide{Path: paths[1], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[1]}},
	}
	rm := types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeUnspecified, Comparison: comparison}}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
	bus.Mutable.SetRequestModel(rm)
	catalog, err := tracecatalog.DiscoverIndependentPair(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bus.Mutable.InstallTraceCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": paths[1], "view": "measurements"})
	if !result.Success {
		t.Fatal(result.Summary)
	}
	status := func(results ...types.ToolResult) types.RuntimeMeasurementTable {
		t.Helper()
		bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
		for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0)).Choices() {
			if strings.HasPrefix(table.ObservationID, "trace_measurement_acquisition:") {
				return table
			}
		}
		t.Fatal("missing actual current acquisition status")
		return types.RuntimeMeasurementTable{}
	}
	before := status(result)
	if !before.HasNativeRawFacts() || before.Rows[1][1] != "available" {
		t.Fatal("real source/query positive missing", before.Rows)
	}
	intact := result
	intact.Observations = append([]types.ObservationRecord(nil), result.Observations...)
	changed := 0
	for i := range result.Observations {
		if result.Observations[i].Predicate == "measurements_observation" {
			result.Observations[i].Object = "empty"
			changed++
		}
	}
	if changed == 0 {
		t.Fatal("no real raw records mutated")
	}
	after := status(result)
	if after.Rows[1][1] != "unavailable" || !strings.Contains(after.Rows[1][6], "success=1") {
		t.Fatalf("changed raw state became trusted status or actual execution was lost: %+v", after.Rows)
	}
	for _, results := range [][]types.ToolResult{{intact, result}, {result, intact}} {
		retained := status(results...)
		if retained.Rows[1][1] != "available" || !strings.Contains(retained.Rows[1][6], "success=1") {
			t.Fatal("invalid copy overrode independently intact same-ID state or inflated calls", retained.Rows)
		}
	}
}

func TestHMC268RawMeasurementsPublicFactSurfaces(t *testing.T) {
	for _, tc := range []struct {
		name, fixture, query string
		pair                 bool
	}{
		{"raw", "hmosperf_dual_capture_metadata/current.data", "measurements", false},
		{"process", "hmosperf_process_measurements/capture.data", "process_measurements", false},
		{"unpositioned", "hmosperf_dual_measurements/current.data", "event_search", false},
		{"pair", "hmosperf_dual_capture_metadata/current.data", "measurements", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := rawPresentationFixtureHMC268(t, tc.fixture, tc.query, tc.pair)
			contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0))
			if !contract.Active() {
				t.Fatal("missing real native tables")
			}
			published := 0
			for _, table := range contract.Choices() {
				if tc.name == "unpositioned" && !table.HasNativeRawFacts() {
					continue // Generic event inventory has its own established surface.
				}
				published++
				t.Run(table.ObservationID+"/"+string(table.View), func(t *testing.T) {
					for _, kind := range []types.AnswerBlockKind{types.BlockTable, types.BlockSummary, types.BlockDiagram} {
						t.Run(string(kind), func(t *testing.T) {
							blocks := []any{map[string]any{"id": "facts", "kind": kind, "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}}
							if kind != types.BlockSummary {
								blocks = append([]any{map[string]any{"id": "lead", "kind": "summary", "text": "Recorded facts and their original boundaries."}}, blocks...)
							}
							out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": blocks}, false)
							if !out.Success {
								t.Fatalf("public %s rejected existing exact table: %s", kind, out.Summary)
							}
							visible := html.UnescapeString(render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "en"))
							for _, row := range table.Rows {
								for _, value := range row {
									if value != "" && !strings.Contains(visible, value) {
										t.Errorf("%s lost exact native cell %q", kind, value)
									}
								}
							}
							for _, note := range table.Notes {
								if !strings.Contains(visible, note) {
									t.Errorf("lost native boundary %q", note)
								}
							}
							if kind == types.BlockDiagram && (strings.Contains(visible, "-->") || strings.Contains(visible, "->>")) {
								t.Fatal("native facts invented relationship edges")
							}
						})
					}
				})
			}
			if published == 0 {
				t.Fatal("no current raw tables reached public surface")
			}
		})
	}
}
