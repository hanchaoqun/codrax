package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC280ClockWitnessPublicNativePublication(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "tracequery", "testdata", "hmc280_clock_sync.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.ftrace")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("Read recorded clock information")}
	query := func(start, end float64) types.ToolResult {
		t.Helper()
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": start, "time_end": end, "pattern": "trace_event_clock_sync"})
		result, err := (&TraceQuery{}).Execute(bus, args)
		if err != nil || !result.Success {
			t.Fatalf("query failed: %v %s", err, result.Summary)
		}
		return result
	}
	result := query(170441.373, 170441.374)
	var inventory *types.TraceEventSearchInventory
	for _, row := range result.Observations {
		if row.EventSearchInventory == nil {
			continue
		}
		inventory = row.EventSearchInventory
		if !types.IsValidTraceEventSearchInventoryRecord(row) {
			t.Fatal("new fields broke native validation")
		}
		if row.SourceRef.ClockCalibrated || row.SourceRef.Path != path {
			t.Fatalf("witness changed source clock authority: %+v", row.SourceRef)
		}
		if _, ok := types.DecodeRuntimeMeasurementPublication(row); !ok {
			t.Fatal("missing existing native publication")
		}
	}
	if inventory == nil || inventory.Coverage.MatchedTotal != 2 || len(inventory.Rows) != 2 {
		t.Fatalf("missing native rows: %+v", inventory)
	}
	for _, row := range inventory.Rows {
		if row.SourcePath != path || row.LocalLine < 3 || row.Semantics == nil {
			t.Fatalf("missing physical source identity: %+v", row)
		}
		encoded, _ := json.Marshal(row.Semantics)
		for _, want := range []string{"unmapped_observation_only", "source_producer_identity_not_verified", "point_witness_only"} {
			if !strings.Contains(string(encoded), want) {
				t.Fatalf("lost limitation %s", want)
			}
		}
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
	for _, p := range types.CompileTraceCausalProjectionSet(ledger).Projections {
		if p.PrimaryRootCause != nil || len(p.OnChainCauses) != 0 || len(p.WakeupPath) != 0 {
			t.Fatal("clock marker granted causal authority")
		}
	}
	empty := query(170442, 170443)
	for _, row := range empty.Observations {
		if row.EventSearchInventory != nil && (row.EventSearchInventory.Coverage.MatchedTotal != 0 || len(row.EventSearchInventory.Rows) != 0) {
			t.Fatal("keyword search rescued outside requested window")
		}
	}
}
