package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC225PreparedIdentityIndependentOfWindowReplay(t *testing.T) {
	for _, selector := range []string{"none", "start_only", "end_only", "lines"} {
		t.Run(selector, func(t *testing.T) {
			bus, _, _ := hmc17NamedPathContext(t)
			body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_measurements/current.data")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(bus.RepoRoot, "current.data")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			comparison := &types.RuntimeArtifactComparisonScope{Baseline: types.RuntimeArtifactComparisonSide{Path: path, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: path}}, Current: types.RuntimeArtifactComparisonSide{Path: filepath.Join(bus.RepoRoot, "other.data"), RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: "other"}}}
			rm := types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeUnspecified, Comparison: comparison}}
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
			bus.Mutable.SetRequestModel(rm)
			params := map[string]any{"source": "path", "path": path, "view": "measurements"}
			if selector == "start_only" {
				params["time_start"] = 4.0
			}
			if selector == "end_only" {
				params["time_end"] = 4.5
			}
			if selector == "lines" {
				params["view"] = "event_search"
				params["event_names"] = []string{"measure_interval"}
				params["line_start"], params["line_end"] = 1, 200
			}
			result := hmc17NamedQuery(t, bus, params)
			if !result.Success {
				t.Fatal(result.Summary)
			}
			if _, _, replay := bus.Mutable.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay); replay {
				t.Fatal("non-continuous selector acquired window replay")
			}
			_, _, _, navigation := bus.Mutable.ResolveTraceIntervalNavigation(context.Background(), result.TraceQueryWindowReplay)
			if navigation != (selector == "none") {
				t.Fatalf("navigation=%v selector=%s", navigation, selector)
			}
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			input := types.ObservationLedgerInputFromBusContext(bus, 0)
			if selector == "lines" {
				if len(result.Observations) == 0 {
					t.Fatal("line query did not publish original records")
				}
				index := types.BuildRuntimeObservationDisplayScopes(input)
				if _, bound := index.Scope(result.Observations[0], rm.RuntimeArtifactScopeProfile); !bound {
					t.Fatal("line query lost prepared-source identity")
				}
			} else if !types.HasCurrentNativeMeasurementInvestigation(input) {
				t.Fatal("valid producer source identity lost solely because replay is unavailable")
			}
			if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
				t.Fatal(err)
			}
			if selector == "lines" {
				index := types.BuildRuntimeObservationDisplayScopes(types.ObservationLedgerInputFromBusContext(bus, 0))
				if _, bound := index.Scope(result.Observations[0], rm.RuntimeArtifactScopeProfile); bound {
					t.Fatal("replaced source retained line-query display identity")
				}
			}
			if types.HasCurrentNativeMeasurementInvestigation(types.ObservationLedgerInputFromBusContext(bus, 0)) {
				t.Fatal("prepared original replacement downgraded to still-readable derived facts")
			}
		})
	}
}
