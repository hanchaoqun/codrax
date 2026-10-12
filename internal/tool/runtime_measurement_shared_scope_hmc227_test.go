package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc227SharedScope(path, other string, start, end float64) *types.RuntimeArtifactScopeProfile {
	return &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "shared interval",
		Comparison: &types.RuntimeArtifactComparisonScope{Baseline: types.RuntimeArtifactComparisonSide{Path: path, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: path}}, Current: types.RuntimeArtifactComparisonSide{Path: other, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: other}}}}
}

func TestHMC227SharedScopeStatusBindsCurrentRange(t *testing.T) {
	bus, _, path := hmc223NativeInvestigation(t, false)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(bus.RepoRoot, "independent.data")
	if err := os.WriteFile(other, body, 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := tracecatalog.DiscoverIndependentPair(context.Background(), [2]string{path, other})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bus.Mutable.InstallTraceCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	setScope := func(start, end float64) {
		rm := types.RequestModel{RuntimeArtifactScopeProfile: hmc227SharedScope(path, other, start, end)}
		bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
		bus.Mutable.SetRequestModel(rm)
	}
	setScope(1, 2)
	// The actual producer must inherit the common interval through preparation.
	result := hmc17NamedQuery(t, bus, map[string]any{"path": path, "view": "measurements"})
	if !result.Success {
		t.Fatal(result.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	check := func(wantState, window string, wantMembers bool) string {
		t.Helper()
		var id string
		members := 0
		for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0)).Choices() {
			if strings.HasPrefix(table.ObservationID, "trace_measurement_acquisition:") {
				if id != "" || len(table.Rows) != 2 || table.Rows[0][1] != wantState || table.Rows[1][1] != "not_executed" || !strings.Contains(table.Rows[0][2], window) || !strings.Contains(table.Rows[1][2], window) {
					t.Fatalf("shared range/status lost: %+v", table.Rows)
				}
				id = table.ObservationID
			} else if table.View == types.RuntimeMeasurementMembers {
				members += len(table.Rows)
			}
		}
		if id == "" || (members > 0) != wantMembers {
			t.Fatalf("id=%q members=%d wantMembers=%v", id, members, wantMembers)
		}
		return id
	}
	first := check("available", "[1,2) 秒", true)
	setScope(1.1, 1.9)
	if next := check("unavailable", "[1.1,1.9) 秒", false); next == first {
		t.Fatal("changed common range reused the old status identity")
	}
	setScope(1, 2)
	if restored := check("available", "[1,2) 秒", true); restored != first {
		t.Fatal("unchanged current binding lost stable identity")
	}
}

func TestHMC227SharedScopeDoesNotReplaceSourceReceipts(t *testing.T) {
	for _, pair := range []bool{false, true} {
		for _, mutation := range []string{"current", "alias", "foreign_source", "outside_window", "reset", "replace", "json"} {
			t.Run(map[bool]string{false: "single", true: "pair"}[pair]+"/"+mutation, func(t *testing.T) {
				bus, result, path := hmc223NativeInvestigation(t, pair)
				if mutation == "alias" {
					alias := filepath.Join(t.TempDir(), "alias.data")
					if err := os.Symlink(path, alias); err != nil {
						t.Fatal(err)
					}
					path = alias
					args := map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": 2}
					if pair {
						args = map[string]any{"comparison": map[string]any{"baseline": args}}
					}
					result = hmc17NamedQuery(t, bus, args)
					if !result.Success {
						t.Fatal(result.Summary)
					}
				}
				scope := hmc227SharedScope(path, filepath.Join(bus.RepoRoot, "other.data"), 1, 2)
				switch mutation {
				case "foreign_source":
					scope.Comparison.Baseline.Path = path + ".unread"
				case "outside_window":
					*scope.TimeStart, *scope.TimeEnd = 4, 5
				case "reset":
					bus.Mutable.ResetTurnAArtifacts()
				case "replace":
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
				case "json":
					data, err := json.Marshal(result)
					if err != nil {
						t.Fatal(err)
					}
					result = types.ToolResult{}
					if err := json.Unmarshal(data, &result); err != nil {
						t.Fatal(err)
					}
				}
				rm := types.RequestModel{RuntimeArtifactScopeProfile: scope}
				bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
				bus.Mutable.SetRequestModel(rm)
				bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
				members := 0
				for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(bus, 0)).Choices() {
					if table.View == types.RuntimeMeasurementMembers {
						members += len(table.Rows)
					}
				}
				if (members > 0) != (mutation == "current" || mutation == "alias") {
					if report, ok := result.RuntimeMeasurementPair.Report(); ok {
						t.Logf("actual comparison report: %+v", report)
					}
					t.Fatalf("shared window replaced current original-source proof: members=%d", members)
				}
			})
		}
	}
}
