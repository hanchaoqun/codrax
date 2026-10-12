package tool

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc224SourceOwnedScope(path, other string, swapped bool) *types.RuntimeArtifactScopeProfile {
	a, b, c, d := 1.0, 2.0, 4.0, 4.5
	if swapped {
		a, b, c, d = c, d, a, b
	}
	first := types.RuntimeArtifactComparisonSide{Path: path, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{TimeStart: &a, TimeEnd: &b, SourceQuote: "first"}}
	second := types.RuntimeArtifactComparisonSide{Path: other, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{TimeStart: &c, TimeEnd: &d, SourceQuote: "second"}}
	return &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, Comparison: &types.RuntimeArtifactComparisonScope{Baseline: first, Current: second}, TimeWindows: []types.RuntimeArtifactTimeWindow{first.RuntimeArtifactTimeWindow, second.RuntimeArtifactTimeWindow}}
}

func TestHMC224NativeMeasurementSourceOwnedWindows(t *testing.T) {
	for _, pair := range []bool{false, true} {
		for _, mutation := range []string{"matching", "other_source_window", "unknown_source", "json_profile", "role_swapped"} {
			t.Run(map[bool]string{false: "single", true: "pair"}[pair]+"/"+mutation, func(t *testing.T) {
				bus, _, path := hmc223NativeInvestigation(t, pair)
				profile := hmc224SourceOwnedScope(path, filepath.Join(bus.RepoRoot, "other.data"), mutation == "other_source_window")
				if mutation == "unknown_source" {
					profile.Comparison.Baseline.Path = filepath.Join(bus.RepoRoot, "unqueried.data")
				}
				if mutation == "role_swapped" {
					profile.Comparison.Baseline, profile.Comparison.Current = profile.Comparison.Current, profile.Comparison.Baseline
				}
				if mutation == "json_profile" {
					encoded, err := json.Marshal(profile)
					if err != nil {
						t.Fatal(err)
					}
					profile = &types.RuntimeArtifactScopeProfile{}
					if err := json.Unmarshal(encoded, profile); err != nil {
						t.Fatal(err)
					}
				}
				rm := types.RequestModel{RuntimeArtifactScopeProfile: profile}
				bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
				bus.Mutable.SetRequestModel(rm)
				rows := 0
				for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0)).Choices() {
					if table.View == types.RuntimeMeasurementMembers {
						rows += len(table.Rows)
					}
				}
				want := mutation == "matching" || mutation == "json_profile" || mutation == "role_swapped" && !pair
				if (rows > 0) != want {
					t.Fatalf("native source/window authority rows=%d want facts=%v", rows, want)
				}
			})
		}
	}
}

func TestHMC224ComparisonDispatchRejectsWrongRole(t *testing.T) {
	bus, _, path := hmc223NativeInvestigation(t, false)
	profile := hmc224SourceOwnedScope(path, filepath.Join(bus.RepoRoot, "other.data"), false)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: profile}}
	result := hmc17NamedQuery(t, bus, map[string]any{"comparison": map[string]any{"current": map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": 2}}})
	report, ok := result.RuntimeMeasurementPair.Report()
	if !ok || report.Sides[1].Status != "failed" || len(report.Sides[1].Publications) != 0 {
		t.Fatalf("wrong-role source dispatched: %+v", report)
	}
}
