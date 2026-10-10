package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Read native tool facts at the actual finalizer adapter boundary. The source
// oracle is independent of presentation and does not ask a model to repeat
// these invariants in either the user's question or the answer.
func TestHMC218LogQueryActualFinalizerMessages(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse_sources=%t", reverse), func(t *testing.T) {
			paths := []string{"../../eval/fixtures/hmosperf_log_sources/app/session.log", "../../eval/fixtures/hmosperf_log_sources/kernel/session.log.gz"}
			if reverse {
				paths[0], paths[1] = paths[1], paths[0]
			}
			inputs := make([]loginput.Input, len(paths))
			originals := make([][]byte, len(paths))
			for i, path := range paths {
				path, err := filepath.Abs(path)
				if err != nil {
					t.Fatal(err)
				}
				inputs[i].Path = path
				originals[i], err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			catalog, err := loginput.Prepare(context.Background(), inputs, loginput.Options{})
			if err != nil {
				t.Fatal(err)
			}
			oracle, err := catalog.Query(context.Background(), loginput.Query{})
			if err != nil || !oracle.Complete || oracle.Matched != 9 || oracle.Returned != 9 || len(oracle.Sources) != 2 {
				t.Fatalf("native fixture oracle: %v %+v", err, oracle)
			}
			dir := t.TempDir()
			rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, LogTriage: &types.LogBundle{},
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeRelationAnalysis}}
			ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("这两份日志记录了什么？哪些联系有依据？"), AttachedLogCatalog: catalog,
				AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}}
			ctx.Mutable.SetRequestModel(rm)
			var results []types.ToolResult
			var native []types.ObservationRecord
			for _, source := range oracle.Sources {
				args, _ := json.Marshal(map[string]any{"source_ids": []string{source.ID}})
				result, err := (&tool.LogQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
				if err != nil || !result.Success {
					t.Fatalf("public log query: %v / %s", err, result.Summary)
				}
				results = append(results, types.AttachToolHandoffCarrier(result))
				native = append(native, result.Observations...)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results,
				HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
			before := hmc218CausalProjection(t, ctx)
			messages := dependencyObservationMessages(t, ctx)
			objects := hmc218MessageJSONObjects(messages)
			records := make(map[string]map[string]any)
			for _, obj := range objects {
				id, _ := obj["record_id"].(string)
				if id != "" {
					records[id] = obj
				}
			}
			if len(records) != 9 {
				t.Errorf("actual finalizer has %d complete native log records, want all 9 from both sources", len(records))
			}
			for _, observation := range native {
				if observation.Predicate != "log_record" {
					continue
				}
				var want map[string]any
				decoder := json.NewDecoder(strings.NewReader(observation.Value))
				decoder.UseNumber()
				if err := decoder.Decode(&want); err != nil {
					t.Fatal(err)
				}
				id := want["record_id"].(string)
				got, ok := records[id]
				if !ok {
					t.Errorf("native log record missing or JSON scalar object truncated: %s (%s:%d-%d)", id, observation.SourceRef.Path, observation.Span.LineStart, observation.Span.LineEnd)
					continue
				}
				for _, field := range []string{"record_id", "source_id", "first_line", "last_line", "pid", "tid", "cpu", "kind", "status", "wall_timestamp_raw", "boot_timestamp_raw", "boot_timestamp_ns", "clock_domain", "parse_error", "message_preview"} {
					value, exists := got[field]
					// The parser's absent timestamp is an empty string. Either an
					// explicit null or that empty value conveys the same unknown;
					// a known timestamp must retain its exact original string.
					if (field == "wall_timestamp_raw" || field == "boot_timestamp_raw" || field == "boot_timestamp_ns") && want[field] == "" && exists && value == nil {
						continue
					}
					if !exists || !reflect.DeepEqual(value, want[field]) {
						t.Errorf("record %s field %s=%#v (present=%t), native=%#v", id, field, value, exists, want[field])
					}
				}
			}
			var lines int64
			for _, source := range oracle.Sources {
				lines += source.PhysicalLines
				for _, identity := range []string{source.ID, source.Path, source.Generation} {
					if !strings.Contains(messages, identity) {
						t.Errorf("source binding absent from actual message: %q", identity)
					}
				}
			}
			if lines != 11 {
				t.Fatal("fixture physical line oracle drifted")
			}
			if !bytes.Equal(before, hmc218CausalProjection(t, ctx)) || ctx.Mutable.TraceRootCauseReport() != nil {
				t.Fatal("log presentation changed Trace causal authority")
			}
			for i, input := range inputs {
				after, err := os.ReadFile(input.Path)
				if err != nil || !bytes.Equal(after, originals[i]) {
					t.Fatal("read-only public handoff modified attached source")
				}
			}
		})
	}
}

// Accept objects either printed as JSON or carried inside a quoted JSON value;
// titles, prose, whitespace and the choice of message are not part of the test.
func hmc218MessageJSONObjects(text string) []map[string]any {
	var out []map[string]any
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			out = append(out, value)
			for _, child := range value {
				visit(child)
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		case string:
			if strings.HasPrefix(strings.TrimSpace(value), "{") {
				decoder := json.NewDecoder(strings.NewReader(value))
				decoder.UseNumber()
				var object map[string]any
				if decoder.Decode(&object) == nil {
					visit(object)
				}
			}
		}
	}
	for pos := 0; pos < len(text); pos++ {
		if text[pos] != '{' && text[pos] != '"' {
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(text[pos:]))
		decoder.UseNumber()
		var value any
		if decoder.Decode(&value) == nil {
			visit(value)
			pos += int(decoder.InputOffset()) - 1
		}
	}
	return out
}

func hmc218CausalProjection(t *testing.T, ctx *types.AgentContext) []byte {
	t.Helper()
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromAgentContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	out, err := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	if err != nil {
		t.Fatal(err)
	}
	return out
}
