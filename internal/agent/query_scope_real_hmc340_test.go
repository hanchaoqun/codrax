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

func TestHMC340FullRealEqualPayloadNativeHandoff(t *testing.T) {
	path, root := os.Getenv("CODRAX_HMC340_REAL_TRACE"), os.Getenv("CODRAX_HMC340_REAL_ARTIFACTS")
	if path == "" {
		t.Skip("whole user trace opt-in not supplied")
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) {
		t.Fatal("absolute source and fresh artifact directory required")
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	before, size := hmc333FullSourceHash(t, path)
	if before != "93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef" || size != 235422121 {
		t.Fatal("not the original whole user capture")
	}
	start, end := 236627.820748, 236628.490099
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "236627.820748s 到 236628.490099s"}}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("分析该线程的业务阶段及状态分布"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ctx.Mutable.SetRequestModel(rm)
	var records []types.ObservationRecord
	var artifacts []map[string]string
	// A cold index reports the complete scan, while its hot reuse reports the
	// indexed subset. Keep that genuine first publication, then exercise equal
	// bytes between two hot queries rather than assuming cold/hot metadata equal.
	for i, limit := range []int{199, 200, 201} {
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 57874, "time_start": start, "time_end": end, "limit": limit, "trace_flavor": "harmony_hitrace"})
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !r.Success {
			t.Fatalf("whole-source native query: %v %s", err, r.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(r)
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
		for _, row := range r.Observations {
			if row.Predicate != types.TraceBusinessSpanPredicate || row.Object != "bindApplication" {
				continue
			}
			records = append(records, row)
			for role, original := range map[string]string{"payload": row.SourceRef.PayloadRef, "raw": row.SourceRef.RawRef} {
				data, err := os.ReadFile(original)
				if err != nil {
					t.Fatal(err)
				}
				copy := filepath.Join(root, string(rune('a'+i))+"-"+role+".txt")
				if err := os.WriteFile(copy, data, 0600); err != nil {
					t.Fatal(err)
				}
				artifacts = append(artifacts, map[string]string{"original": original, "copy": copy, "role": role})
			}
			break
		}
	}
	if len(records) != 3 || records[1].SourceRef.PayloadRef != records[2].SourceRef.PayloadRef || records[1].SourceRef.QueryScopeID == records[2].SourceRef.QueryScopeID || records[1].ID == records[2].ID {
		t.Fatal("real independent equal-payload queries lost distinct observation coordinates")
	}
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	for _, row := range records {
		r := types.AnswerRuntimeMeasurementReceipt{ObservationID: row.ID, View: types.RuntimeMeasurementSummary}
		if !types.BindRuntimeMeasurementReceipt(&r, contract) || len(r.BoundTable.Rows) != 13 || !strings.Contains(strings.Join(r.BoundTable.Notes, "\n"), "自身账户（窗口内墙钟，不是运行时间）：227.101 毫秒；") {
			t.Fatal("one real query withdrew another query's complete native account")
		}
	}
	// No completion or explicit native display choice: verify discoverability
	// through the actual finalizer adapter, not a forced selected-view test.
	prompt := dependencyObservationMessages(t, ctx.ShallowClone())
	if err := os.WriteFile(filepath.Join(root, "finalize.txt"), []byte(prompt), 0600); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "自身账户（窗口内墙钟，不是运行时间）：227.101 毫秒；") {
		t.Fatal("unselected native account was not discoverable in actual finalizer context")
	}
	after, afterSize := hmc333FullSourceHash(t, path)
	if before != after || size != afterSize {
		t.Fatal("read-only query changed the whole user source")
	}
	data, _ := json.MarshalIndent(map[string]any{"source": path, "source_bytes": size, "source_sha256": before, "production_queries": 3, "cold_query_included": true, "equal_payload_query_indices": []int{1, 2}, "external_models": 0, "native_records": records, "raw_artifact_copies": artifacts, "explicit_native_selection": false, "source_unchanged": true}, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "receipt.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
