package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC280ClockWitnessActualFinalizerAndSourceGeneration(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "tracequery", "testdata", "hmc280_clock_sync.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.ftrace")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "trace_event_clock_sync", "time_start": 170441.373, "time_end": 170441.374})
	ctx := traceEventInventoryPublicContext(nil)
	ctx.RepoRoot, ctx.WorkDir = dir, dir
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("query failed: %v %s", err, result.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	if _, _, current := ctx.Mutable.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay); !current {
		t.Fatal("missing current source-generation receipt")
	}
	before, _ := json.Marshal([]any{result, answerDocObservationLedger(ctx), types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))})
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	for _, want := range []string{"clock_sync.trace_time_ns", "170441373302000", "1787277345616", "10143.297852", "unmapped_observation_only", "source_producer_identity_not_verified", "point_witness_only"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("finalizer lost %s", want)
		}
	}
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || len(views[0].Inventory.Rows) != 2 {
		t.Fatal("clock witness bypassed native inventory")
	}
	after, _ := json.Marshal([]any{result, answerDocObservationLedger(ctx), types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))})
	if string(before) != string(after) {
		t.Fatal("display changed source or causal authority")
	}
	if err := os.WriteFile(path, []byte("changed source generation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, current := ctx.Mutable.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay); current {
		t.Fatal("changed source retained current clock-witness permission")
	}
	// Historical query observations stay in the audit ledger. A new query of
	// the same path must read the new generation, not reuse its old witnesses.
	fresh, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !fresh.Success {
		t.Fatalf("fresh query failed: %v %s", err, fresh.Summary)
	}
	for _, row := range fresh.Observations {
		if row.EventSearchInventory != nil && len(row.EventSearchInventory.Rows) != 0 {
			t.Fatal("new generation borrowed old clock observations")
		}
	}
}
