package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual producer and adapter. A successful tool shell, coverage
// receipt or partial provenance cannot become a native observation reference.
func TestHMC219NativeQualificationActualFinalizer(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_log_sources/app/session.log")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Path: path}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	producerBus := &types.BusContext{AttachedLogCatalog: catalog}
	result, err := (&tool.LogQuery{}).Execute(producerBus, json.RawMessage(`{}`))
	if err != nil || !result.Success {
		t.Fatalf("native query: %v / %s", err, result.Summary)
	}
	var record, coverage types.ObservationRecord
	for _, row := range result.Observations {
		switch row.Predicate {
		case "log_record":
			record = row
		case "log_query_coverage":
			coverage = row
		}
	}
	if record.ID == "" || coverage.ID == "" {
		t.Fatal("fixture requires a native record and independent coverage receipt")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*types.ObservationRecord)
	}{
		{"coverage_only", func(row *types.ObservationRecord) { *row = coverage }},
		{"missing_query_scope", func(row *types.ObservationRecord) { row.SourceRef.QueryScopeID = "" }},
		{"missing_payload", func(row *types.ObservationRecord) { row.SourceRef.PayloadRef = "" }},
		{"foreign_producer", func(row *types.ObservationRecord) { row.Producer = "explorer_model" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := record
			tc.mutate(&row)
			bus := hmc219NativeBus(t, types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioGeneric}, types.TurnRouteCurrentSourceEvidenceRequired)
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "log_query", Success: true, Observations: []types.ObservationRecord{row}}}})
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			ledger := answerDocObservationLedger(ctx)
			if len(ledger.Records) != 1 {
				t.Fatalf("negative control did not reach the durable ledger: %+v", ledger.Records)
			}
			if kind := types.NativeRuntimeFactPresentationKind(ledger.Records[0]); kind != "" && kind != "log_query_coverage" {
				t.Fatalf("negative control unexpectedly qualified after ledger normalization: %s", kind)
			}
			if answerDocHasNativeFactHandoff(ctx) {
				t.Fatal("unqualified/coverage-only record activated native fact handoff")
			}
			messages := dependencyObservationMessages(t, ctx)
			for _, object := range hmc218MessageJSONObjects(hmc219Section(messages, "## Typed Answer Support Lanes")) {
				if ids, ok := object["observation_ids"].([]any); ok && len(ids) > 0 {
					t.Fatalf("unqualified/coverage-only record entered native support references: %+v", object)
				}
			}
			if len(answerDocObservationLedger(ctx).Records) != 1 {
				t.Fatal("presentation erased durable evidence")
			}
		})
	}
}

func TestHMC219NativeSelectedWindowActualFinalizer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "windows.systrace")
	trace := traceWaitRawStateAgentCycle(1, "D", 1) + traceWaitRawStateAgentCycle(2, "D", 1)
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.004
	rm := types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioGeneric,
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000..1.004", Confidence: 1}}
	if !rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
		t.Fatal("fixture requires a valid user-owned explicit window")
	}
	bus := hmc219NativeBus(t, rm, types.TurnRouteCurrentSourceEvidenceRequired)
	var results []types.ToolResult
	outsideIDs := map[string]bool{}
	for _, window := range [][2]float64{{start, end}, {2, 2.004}} {
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 77,
			"time_start": window[0], "time_end": window[1], "trace_flavor": "harmony_hitrace"})
		result, err := (&tool.TraceQuery{}).Execute(bus, args)
		if err != nil || !result.Success || len(result.Observations) == 0 {
			t.Fatalf("real window query failed: %v / %s", err, result.Summary)
		}
		if window[0] == 2 {
			for _, row := range result.Observations {
				outsideIDs[row.ID] = true
			}
		}
		results = append(results, types.AttachToolHandoffCarrier(result))
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	projection := hmc218CausalProjection(t, ctx)
	messages := dependencyObservationMessages(t, ctx)
	shown := 0
	for _, object := range hmc218MessageJSONObjects(hmc219Section(messages, "## Typed Answer Support Lanes")) {
		if object["producer_kind"] != "trace_record" {
			continue
		}
		for _, rawID := range object["observation_ids"].([]any) {
			id := rawID.(string)
			shown++
			if outsideIDs[id] {
				t.Errorf("known outside-window observation acquired main native fact reference: %s", id)
			}
		}
	}
	if shown == 0 {
		t.Fatal("explicit-window native query lost eligible fact references")
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if !bytes.Equal(before, after) || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
		t.Fatal("display filtering changed durable evidence or causal projection")
	}
}
