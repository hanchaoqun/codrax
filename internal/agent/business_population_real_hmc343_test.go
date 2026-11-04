package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Whole user capture, original independent integer B/E oracle. This is an
// opt-in model-free public seam regression, not a third real-model eval.
func TestHMC343FullDouyinPopulationPublicHandoff(t *testing.T) {
	path, out, oraclePath := os.Getenv("CODRAX_HMC343_REAL_TRACE"), os.Getenv("CODRAX_HMC343_REAL_OUT"), os.Getenv("CODRAX_HMC343_REAL_ORACLE")
	if path == "" {
		t.Skip("whole real trace not supplied")
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(out) || !filepath.IsAbs(oraclePath) {
		t.Fatal("explicit absolute evidence inputs required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	before, size := hmc333FullSourceHash(t, path)
	if size != 235422121 || before != "93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef" {
		t.Fatal("whole original trace changed")
	}
	oracleRaw, err := os.ReadFile(oraclePath)
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		SourceSHA string            `json:"source_sha256"`
		Selected  int               `json:"selected_complete_markers"`
		OpenAtEOF int               `json:"open_at_eof"`
		Spans     []json.RawMessage `json:"spans"`
	}
	if err := json.Unmarshal(oracleRaw, &oracle); err != nil || oracle.SourceSHA != before || oracle.Selected != 2611 || len(oracle.Spans) != 2611 || oracle.OpenAtEOF != 0 {
		t.Fatal("independent whole-source target oracle absent")
	}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Explain the observed business stages"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 57874, "time_start": 236627.820748, "time_end": 236628.490099})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("whole-source query: %v %s", err, result.Summary)
	}
	var payloadRef string
	for _, r := range result.Observations {
		if r.SourceRef.PayloadRef != "" {
			payloadRef = r.SourceRef.PayloadRef
			break
		}
	}
	raw, err := os.ReadFile(payloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var native tracequery.Result
	if err := json.Unmarshal(raw, &native); err != nil || native.WindowStats == nil || native.WindowStats.BusinessTree == nil {
		t.Fatal("native tree absent")
	}
	tree := native.WindowStats.BusinessTree
	p := tree.Population
	if p == nil || p.Target == nil {
		t.Fatal("native producer did not publish the selected-thread census")
	}
	if !p.ValidCounts(tree.NodeCount, tree.OmittedNodes) || p.Target.TID != 57874 || p.Target.NodeCount != oracle.Selected || p.Target.DisplayedNodes != 32 || p.Target.OmittedNodes != oracle.Selected-32 || tree.NodeCount != 25139 || tree.OmittedNodes != 25107 {
		t.Fatalf("all-emitter/target/display census mixed: total=%d omitted=%d population=%+v target=%+v", tree.NodeCount, tree.OmittedNodes, p, p.Target)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"全线程观察库存=25139", "目标线程 TID=57874 的观察库存=2611", "原生未展示=2579"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("actual finalizer lost census %q", want)
		}
	}
	for name, data := range map[string][]byte{"native.json": raw, "finalizer.txt": []byte(prompt)} {
		if err := os.WriteFile(filepath.Join(out, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	rawText, err := os.ReadFile(result.RawRef)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "raw.txt"), rawText, 0600); err != nil {
		t.Fatal(err)
	}
	after, afterSize := hmc333FullSourceHash(t, path)
	if before != after || size != afterSize {
		t.Fatal("query changed source")
	}
	receipt, _ := json.MarshalIndent(map[string]any{"source": path, "source_sha256": before, "source_bytes": size, "source_unchanged": true, "original_total": tree.NodeCount, "original_omitted": tree.OmittedNodes, "population": p, "target_oracle_count": oracle.Selected, "production_queries": 1, "model_requests": 0, "oracle_recomputed": false, "causal_or_scheduler_oracle": false}, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "receipt.json"), receipt, 0600); err != nil {
		t.Fatal(err)
	}
}
