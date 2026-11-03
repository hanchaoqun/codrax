package agent

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Explicit model-free regression on the user's COMPLETE independently measured
// baseline. This is neither another live model case nor default CI dependency.
func TestHMC338ReferenceContextFullDouyinTrace(t *testing.T) {
	path := os.Getenv("CODRAX_HMC338_REAL_TRACE")
	if path == "" {
		t.Skip("full real trace opt-in not supplied")
	}
	root := os.Getenv("CODRAX_HMC338_REAL_ARTIFACTS")
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) {
		t.Fatal("explicit absolute source and fresh evidence directory required")
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	const expectedSHA = "93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef"
	before, size := hmc333FullSourceHash(t, path)
	if before != expectedSHA || size != 235422121 {
		t.Fatal("not the unchanged full independently measured Douyin v100 source")
	}
	ctx := &types.AgentContext{
		RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh",
		AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable:    types.NewMutableState("Explain the observed synchronous business stages"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", Intent: types.IntentRootCause, PerfTrace: &types.PerfBundle{}, AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"}}},
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 57874, "time_start": 236627.820748, "time_end": 236628.490099, "trace_flavor": "harmony_hitrace"})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("full-source native query failed: %v %s", err, result.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	// A business-span state table is inclusive-only. The independent self
	// ruler belongs to the complete instance's original BusinessTree account,
	// not an invented extra row of that state table.
	var payloadRef string
	for _, observation := range result.Observations {
		if observation.SourceRef.PayloadRef != "" {
			payloadRef = observation.SourceRef.PayloadRef
			break
		}
	}
	payload, err := os.ReadFile(payloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var native tracequery.Result
	if err := json.Unmarshal(payload, &native); err != nil {
		t.Fatal(err)
	}
	accountFound := false
	if native.WindowStats != nil && native.WindowStats.BusinessTree != nil {
		for _, node := range native.WindowStats.BusinessTree.Nodes {
			if node.StartLine == 61440 && node.EndLine == 167407 && node.Thread.PID == 57874 && node.Name == "bindApplication" && node.Inclusive != nil && node.Self != nil && math.Abs(node.Inclusive.DurationMs-669.351) < .000001 && math.Abs(node.Self.DurationMs-227.101) < .000001 {
				accountFound = true
			}
		}
	}
	if !accountFound {
		t.Fatal("independent exact inclusive/self instance account not retained in original native BusinessTree")
	}
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	var selected types.AnswerRuntimeMeasurementReceipt
	for _, table := range contract.Choices() {
		if table.View == types.RuntimeMeasurementSummary && strings.Contains(table.Label, "bindApplication") {
			hasTotal := false
			for _, row := range table.Rows {
				if len(row) != 2 {
					continue
				}
				v, err := strconv.ParseFloat(row[1], 64)
				if err != nil {
					continue
				}
				hasTotal = hasTotal || math.Abs(v-669.351) < 0.000001
			}
			if !hasTotal {
				t.Fatalf("full-source inclusive ruler not retained in its native state table: %+v", table.Rows)
			}
			selected = types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
			break
		}
	}
	if selected.ObservationID == "" {
		t.Fatal("complete native business instance was not available")
	}
	if r := hmc303Complete(t, ctx, selected, map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{0}}); !r.Success {
		t.Fatal(r.Summary)
	}
	accepted := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	contexts := investigationNativeReferenceContexts(accepted, types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract)
	if len(contexts[0]) != 1 || !strings.Contains(contexts[0][0].Label, "bindApplication") || !strings.Contains(contexts[0][0].OriginalSource, filepath.Base(path)) {
		t.Fatal("current original instance identity did not reach its interpretation reference")
	}
	writeJSON := func(name string, value any) {
		b, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), append(b, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON("native-tool-result.json", result)
	writeJSON("accepted-handoff.json", accepted)
	writeJSON("current-reference-context.json", contexts)
	for name, prompt := range map[string]string{"extract.txt": (&extractorEvaluator{}).BuildInitialInstruction(ctx.ShallowClone(), nil), "finalize.txt": dependencyObservationMessages(t, ctx.ShallowClone())} {
		unit, _ := json.Marshal(accepted.InterpretationUnits[0])
		if !strings.Contains(prompt, string(unit)) || !strings.Contains(prompt, "native_reference_status=current_exact_rows") {
			t.Fatalf("actual %s lost the whole unit/source-address boundary", name)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(prompt), 0600); err != nil {
			t.Fatal(err)
		}
	}
	refs := map[string]string{"raw.txt": result.RawRef}
	for _, observation := range result.Observations {
		if observation.SourceRef.PayloadRef != "" {
			refs["native.json"] = observation.SourceRef.PayloadRef
			break
		}
	}
	for name, ref := range refs {
		b, err := os.ReadFile(ref)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	after, afterSize := hmc333FullSourceHash(t, path)
	if after != before || afterSize != size {
		t.Fatal("read-only regression changed the full source")
	}
	writeJSON("receipt.json", map[string]any{"source": path, "source_sha256": before, "source_bytes": size, "source_unchanged": true, "model_requests": 0, "production_queries": 1, "full_cli_run": false, "independent_oracle_generation": "HMC320", "native_table_scopes_current": true, "interpretation_semantics_verified": false})
}
