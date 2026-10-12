package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC225AcquisitionStatusSharedAcrossSinglePairAndHistory(t *testing.T) {
	bus, _, _ := hmc17NamedPathContext(t)
	var paths [2]string
	for i, name := range []string{"baseline.data", "current.data"} {
		body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_measurements/" + name)
		if err != nil {
			t.Fatal(err)
		}
		paths[i] = filepath.Join(bus.RepoRoot, name)
		if err := os.WriteFile(paths[i], body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	comparison := &types.RuntimeArtifactComparisonScope{Baseline: types.RuntimeArtifactComparisonSide{Path: paths[0], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[0]}}, Current: types.RuntimeArtifactComparisonSide{Path: paths[1], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[1]}}}
	rm := types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeUnspecified, Comparison: comparison}}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
	bus.Mutable.SetRequestModel(rm)
	catalog, err := tracecatalog.DiscoverIndependentPair(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err = bus.Mutable.InstallTraceCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	query := map[string]any{"source": "path", "path": paths[1], "view": "measurements"}
	results := []types.ToolResult{hmc17NamedQuery(t, bus, query)}
	if !results[0].Success {
		t.Fatal(results[0].Summary)
	}
	assertStatus := func(wantCurrent string, wantRows int, history string) {
		t.Helper()
		bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
		contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0))
		statusCount, rows := 0, 0
		for _, table := range contract.Choices() {
			if strings.HasPrefix(table.ObservationID, "trace_measurement_acquisition:") {
				statusCount++
				if len(table.Rows) != 2 || table.Rows[0][1] != "not_executed" || table.Rows[1][1] != wantCurrent {
					t.Fatalf("wrong states: %+v", table.Rows)
				}
				if history != "" && !strings.Contains(table.Rows[1][6], history) {
					t.Fatalf("history lost: %+v", table.Rows)
				}
			} else if table.View == types.RuntimeMeasurementMembers {
				rows += len(table.Rows)
			}
		}
		if statusCount != 1 || rows != wantRows {
			t.Fatalf("status dedupe or native records changed: statuses=%d rows=%d want=%d", statusCount, rows, wantRows)
		}
	}
	assertStatus("available", 9, "")
	// The same capture/query may arrive via either calling form. There is one
	// current acquisition table, not a pair-only or latest-call-only ledger.
	results = append(results, hmc17NamedQuery(t, bus, map[string]any{"comparison": map[string]any{"current": query}}))
	assertStatus("available", 9, "")
	for _, q := range catalog.Snapshot().Queries {
		if q.View == "measurements" && len(q.Attempts) > 0 {
			_, err = catalog.Complete(context.Background(), q.ID, tracecatalog.Completion{Outcome: tracecatalog.OutcomeFailure, Error: &tracecatalog.Issue{Code: "query_failed", Message: "controlled failed retry after a valid result"}})
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	assertStatus("available", 9, "failure=1")
	// Role correction binds the current source, not old catalog row order.
	rm.RuntimeArtifactScopeProfile.Comparison.Baseline, rm.RuntimeArtifactScopeProfile.Comparison.Current = comparison.Current, comparison.Baseline
	bus.AnalysisIR.RequestModel = rm
	bus.Mutable.SetRequestModel(rm)
	for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0)).Choices() {
		if strings.HasPrefix(table.ObservationID, "trace_measurement_acquisition:") && (table.Rows[0][1] != "available" || table.Rows[1][1] != "not_executed") {
			t.Fatal("status roles did not follow current exact source", table.Rows)
		}
	}
}
