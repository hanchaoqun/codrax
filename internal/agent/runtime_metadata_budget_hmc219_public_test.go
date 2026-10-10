package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The query metadata and fact budgets are independent. A selected native
// event must not lose its source-generation receipt merely because its query
// was later in a bounded list of queries.
func TestHMC219SelectedLogSourcesOwnMetadataBudget(t *testing.T) {
	for _, sourceCount := range []int{40, 64} {
		t.Run(fmt.Sprintf("last_nine_of_%d_sources", sourceCount), func(t *testing.T) {
			hmc219LogMetadataBudgetMessages(t, sourceCount)
		})
	}
}

func hmc219LogMetadataBudgetMessages(t *testing.T, sourceCount int) {
	t.Helper()
	var inputs []loginput.Input
	for i := 0; i < sourceCount; i++ {
		inputs = append(inputs, loginput.Input{Name: fmt.Sprintf("capture-%d/session.log", i),
			Data: []byte(fmt.Sprintf("10-09 09:00:00.001 %d %d I Worker: capture %d event\n", 10+i, 100+i, i))})
	}
	catalog, err := loginput.Prepare(context.Background(), inputs, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, LogTriage: &types.LogBundle{},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeRelationAnalysis}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("这些日志各记录了什么？"), AttachedLogCatalog: catalog,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}}
	ctx.Mutable.SetRequestModel(rm)
	sources := catalog.Sources()
	queried := sources[len(sources)-9:]
	var results []types.ToolResult
	// An earlier no-hit query has a full validated source roster but no facts.
	catalogResult, err := (&tool.LogQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"contains":"not-present-anywhere"}`))
	if err != nil || !catalogResult.Success {
		t.Fatalf("catalog query: %v / %s", err, catalogResult.Summary)
	}
	results = append(results, types.AttachToolHandoffCarrier(catalogResult))
	for _, source := range queried {
		args, _ := json.Marshal(map[string]any{"source_ids": []string{source.ID}})
		result, err := (&tool.LogQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !result.Success {
			t.Fatalf("public query: %v / %s", err, result.Summary)
		}
		results = append(results, types.AttachToolHandoffCarrier(result))
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results,
		HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
	messages := dependencyObservationMessages(t, ctx)
	fields, records := map[string]map[string]any{}, map[string]bool{}
	metadata, ok := strings.CutPrefix(hmc219Section(messages, "## Observation Ledger"), "## Observation Ledger")
	if !ok {
		t.Fatal("no observed fact handoff")
	}
	for _, object := range hmc218MessageJSONObjects(metadata) {
		id, _ := object["source_id"].(string)
		if id == "" {
			continue
		}
		if _, ok := object["record_id"].(string); ok {
			records[id] = true
		}
		if _, ok := object["generation"].(string); ok {
			fields[id] = object
		}
	}
	if len(records) != 9 {
		t.Errorf("small native record set omitted: got %d sources, want 9", len(records))
	}
	for _, source := range queried {
		object, ok := fields[source.ID]
		if !ok {
			t.Errorf("displayed native record lost source-generation metadata: %s (%s)", source.ID, source.Name)
			continue
		}
		for key, want := range map[string]string{"source_id": source.ID, "name": source.Name, "generation": source.Generation,
			"original_sha256": source.OriginalSHA256, "decoded_sha256": source.DecodedSHA256, "compression": source.Compression} {
			if object[key] != want {
				t.Errorf("source %s metadata %s=%#v, native=%q", source.ID, key, object[key], want)
			}
		}
	}
}
