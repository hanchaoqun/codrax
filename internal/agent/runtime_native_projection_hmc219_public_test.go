package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A parent query receipt and a row-local selected window are distinct rulers.
// Isolate that boundary using a real producer row, then widen only the parent
// query envelope; never erase a valid selected-window projection on display.
func TestHMC219NativeLocalProjectionRetainsReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local.systrace")
	if err := os.WriteFile(path, []byte(traceWaitRawStateAgentCycle(1, "D", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.004
	rm := types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioGeneric,
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000..1.004", Confidence: 1}}
	bus := hmc219NativeBus(t, rm, types.TurnRouteCurrentSourceEvidenceRequired)
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 77,
		"time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
	result, err := (&tool.TraceQuery{}).Execute(bus, args)
	if err != nil || !result.Success {
		t.Fatalf("native query failed: %v / %s", err, result.Summary)
	}
	var row types.ObservationRecord
	for _, candidate := range result.Observations {
		if candidate.Predicate == "target_window_states" {
			row = candidate
		}
	}
	a, b, ok := types.TraceCausalProjectionSelectedWindowNote(row.RichNotes)
	if !ok || a != start || b != end || row.SourceRef.QueryScopeID == "" {
		t.Fatal("fixture lacks the producer-owned exact local projection and query receipt")
	}
	row.SourceRef.QueryWindowKnown = true
	row.SourceRef.QueryWindowStartTs, row.SourceRef.QueryWindowEndTs = .5, 2
	result.Observations = []types.ObservationRecord{row}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	messages := dependencyObservationMessages(t, ctx)
	hmc219AssertNativeSupportReferences(t, messages, "trace_record", []string{row.ID})
}
