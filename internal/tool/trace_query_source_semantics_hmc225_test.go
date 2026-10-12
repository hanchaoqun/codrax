package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC225ComparisonSupplementRetainsSourceSemantics(t *testing.T) {
	bus, prep, _ := hmc17NamedPathContext(t)
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_dual_source_windows/baseline.systrace")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "peer.systrace")
	if err := os.WriteFile(other, body, 0600); err != nil {
		t.Fatal(err)
	}
	material, err := prep.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	bus.AttachedTraceMaterial, bus.AttachedHitrace = material, material.Preview()
	profile := hmc224SourceOwnedScope(path, other, false)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: profile}}
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "attached_trace", "view": "window_stats", "pid": 41, "time_start": 1, "time_end": 1.01, "trace_flavor": "harmony_hitrace", "platform": "donghu", "core_topology": "big=0,small=1"})
	if !result.Success {
		t.Fatal(result.Summary)
	}
	bus.ToolResults = []types.ToolResult{result}
	input := types.ObservationLedgerInputFromBusContext(bus, 0)
	input = traceQueryComparisonSourceInput(bus, material.QueryPath(), input)
	if len(input.ToolResults) != 1 {
		t.Fatal("test query lost before semantic check")
	}
	windows := traceQuerySourceCallWindows(bus, material.QueryPath(), "attached_trace", input)
	if len(windows) != 1 {
		t.Fatalf("windows=%+v", windows)
	}
	if windows[0].TraceFlavor != "harmony_hitrace" || windows[0].Platform != "donghu" || windows[0].CoreTopology != "big=0,small=1" {
		t.Fatalf("source-owned supplemental call lost actual semantic parameters: %+v", windows[0])
	}
}
