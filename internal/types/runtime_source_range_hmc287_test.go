package types_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Real preparation and native queries, not restored metadata or public rows,
// establish each source's range display. Neither pair status nor metadata may
// replace the actual query window or acquire completion/causal permissions.
func TestHMC287SourceRangeCurrentPreparationSinglePairAndStale(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_catalog_objects/captures/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	preparer := traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, "runtime")})
	owner := types.NewMutableState("采集记录")
	rm := types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}}}
	owner.SetRequestModel(rm)
	ctx := &types.BusContext{Ctx: t.Context(), RepoRoot: dir, WorkDir: dir, Mutable: owner, AnalysisIR: &types.AnalysisIR{RequestModel: rm}, TraceInputPreparer: preparer}
	var sides [2]types.NativeMeasurementPairSide
	var paths [2]string
	for i, name := range []string{"baseline.data", "current.data"} {
		path := filepath.Join(dir, name)
		paths[i] = path
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec("UPDATE trace_range SET end_ts=10200000000")
			closeErr := db.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
		}
		material, err := preparer.Prepare(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "resource_stack", "time_start": 10, "time_end": 10.05})
		result, err := (&tool.TraceQuery{}).Execute(ctx, args)
		if err != nil || !result.Success {
			t.Fatalf("native query failed: %v %s", err, result.Summary)
		}
		sides[i] = types.NativeMeasurementPairSide{Request: args, Result: result, Material: material}
	}
	owner.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{sides[0].Result, sides[1].Result}})
	singles := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(ctx, 0))
	seen := 0
	for _, table := range singles.Choices() {
		if !strings.HasPrefix(table.ObservationID, "trace_source_metadata:") {
			continue
		}
		seen++
		if table.MemberSet != nil || table.HasCompleteNativeDisplayPopulation() {
			t.Fatal("source range acquired measured population authority")
		}
		raw, _ := json.Marshal(table)
		if !strings.Contains(string(raw), "source_range") || !strings.Contains(string(raw), "原件声明") {
			t.Fatal(string(raw))
		}
	}
	if seen != 2 {
		t.Fatalf("single source range tables=%d want 2", seen)
	}
	pair := types.NewNativeMeasurementPair(owner, sides)
	report, ok := pair.Report()
	if !ok {
		t.Fatal("no current pair")
	}
	for i, side := range report.Sides {
		if strings.Contains(side.Device, "trace_range") || strings.Contains(side.Workload, "trace_range") {
			t.Fatal("range promoted to device/workload identity", side)
		}
		want := []string{"10100000000", "10200000000"}[i]
		found := false
		for _, field := range side.SourceMetadata {
			found = found || field.Kind == "source_range" && field.Name == "end_ts" && field.Value == want
		}
		if !found {
			t.Fatalf("side %d borrowed/dropped range: %+v", i, side.SourceMetadata)
		}
	}
	owner.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, RuntimeMeasurementPair: pair}}})
	paired := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(ctx, 0))
	found := false
	for _, table := range paired.Choices() {
		if table.ObservationID != report.ID {
			continue
		}
		found = true
		if len(table.Rows) != 2 {
			t.Fatal(table.Rows)
		}
		for i, row := range table.Rows {
			want, other := []string{"10100000000", "10200000000"}[i], []string{"10200000000", "10100000000"}[i]
			if !strings.Contains(row[5], "原件声明时间范围") || !strings.Contains(row[5], want) || strings.Contains(row[5], other) {
				t.Fatal("paired range display conflated sources", row)
			}
		}
	}
	if !found {
		t.Fatal("pair status table missing")
	}
	// The shared acquisition catalog is the other display consumer. A freshly
	// discovered catalog has no attempts: prepared source ranges must appear
	// independently without making those plans look executed.
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeUnspecified, Comparison: &types.RuntimeArtifactComparisonScope{
		Baseline: types.RuntimeArtifactComparisonSide{Path: paths[0], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[0]}},
		Current:  types.RuntimeArtifactComparisonSide{Path: paths[1], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[1]}},
	}}
	ctx.AnalysisIR.RequestModel = rm
	owner.SetRequestModel(rm)
	catalog, err := tracecatalog.DiscoverIndependentPair(t.Context(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.InstallTraceCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	acquisition := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(ctx, 0))
	found = false
	for _, table := range acquisition.Choices() {
		if !strings.HasPrefix(table.ObservationID, "trace_measurement_acquisition:") {
			continue
		}
		found = true
		if len(table.Rows) != 2 {
			t.Fatal(table.Rows)
		}
		for i, row := range table.Rows {
			want, other := []string{"10100000000", "10200000000"}[i], []string{"10200000000", "10100000000"}[i]
			if row[1] != "not_executed" || !strings.Contains(row[5], want) || strings.Contains(row[5], other) {
				t.Fatal("prepared range became execution or borrowed a peer", row)
			}
		}
	}
	if !found {
		t.Fatal("shared acquisition metadata absent")
	}
	raw, _ := json.Marshal(sides[0].Material)
	var restored attachment.TraceMaterial
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if _, err := restored.SourceMetadata(t.Context()); err == nil {
		t.Fatal("JSON revived source range")
	}
	if err := os.WriteFile(sides[0].Material.QueryPath(), []byte("changed derived bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	report, ok = pair.Report()
	if !ok || report.Sides[0].Status != "stale" || len(report.Sides[0].SourceMetadata) != 0 || report.Sides[1].Status == "stale" {
		t.Fatalf("stale side borrowed current peer: %+v", report)
	}
}
