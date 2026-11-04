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

// Whole user material, ordinary native query and actual writing context.
// This opt-in is model-free and cannot consume another live-evaluation slot.
func TestHMC344FullDouyinRuntimeSearchPublicHandoff(t *testing.T) {
	path, out := os.Getenv("CODRAX_HMC344_REAL_TRACE"), os.Getenv("CODRAX_HMC344_REAL_OUT")
	if path == "" {
		t.Skip("whole user trace not supplied")
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(out) {
		t.Fatal("absolute inputs required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	before, size := hmc333FullSourceHash(t, path)
	if size != 235422121 || before != "93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef" {
		t.Fatal("whole source changed")
	}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Explain the observed business stages"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 57874, "time_start": 236627.820748, "time_end": 236628.490099})
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !r.Success {
		t.Fatalf("whole native query: %v %s", err, r.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(r)
	message := hmc344RuntimeSearchWritingContext(t, ctx, r)
	var payload string
	for _, record := range r.Observations {
		if record.SourceRef.PayloadRef != "" {
			payload = record.SourceRef.PayloadRef
			break
		}
	}
	raw, err := os.ReadFile(payload)
	if err != nil {
		t.Fatal(err)
	}
	var native tracequery.Result
	if err := json.Unmarshal(raw, &native); err != nil || native.WindowStats == nil || native.WindowStats.BusinessTree == nil {
		t.Fatal("whole native tree missing")
	}
	tree := native.WindowStats.BusinessTree
	if tree.NodeCount != 25139 || tree.Population == nil || tree.Population.Target == nil || tree.Population.Target.NodeCount != 2611 || len(tree.Nodes) != 32 || !strings.Contains(message, "目标线程 TID=57874 的观察库存=2611") {
		t.Fatal("whole-query facts or census lost")
	}
	for name, data := range map[string][]byte{"native.json": raw, "actual-finalizer.txt": []byte(message)} {
		if err := os.WriteFile(filepath.Join(out, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	text, err := os.ReadFile(r.RawRef)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "raw.txt"), text, 0600); err != nil {
		t.Fatal(err)
	}
	after, afterSize := hmc333FullSourceHash(t, path)
	if before != after || size != afterSize {
		t.Fatal("source modified")
	}
}
