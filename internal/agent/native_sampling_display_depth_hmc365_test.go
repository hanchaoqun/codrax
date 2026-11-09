package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc365AssertDepthReuse(t *testing.T, ctx *types.AgentContext, a, b, expectedSamples int) string {
	t.Helper()
	type item struct {
		id, path, query string
		tree            tracequery.PerfSamplingTreeResult
	}
	var items []item
	for _, r := range answerDocObservationLedger(ctx).Records {
		p, ok := tool.DecodeTracePerfSamplingTree(r)
		if !ok || p.Status != "available" {
			continue
		}
		items = append(items, item{r.ID, r.SourceRef.PayloadRef, r.SourceRef.QueryScopeID, p})
	}
	if len(items) != 2 || items[0].id == items[1].id || items[0].path == items[1].path || items[0].query == items[1].query {
		t.Fatal("fixture did not produce two independently addressed tree publications")
	}
	if items[0].tree.DisplayDepth != a || items[1].tree.DisplayDepth != b {
		t.Fatalf("unexpected native depth receipts %d,%d", items[0].tree.DisplayDepth, items[1].tree.DisplayDepth)
	}
	for _, item := range items {
		if item.tree.SampleCount != expectedSamples || item.tree.OmittedCohorts != 0 {
			t.Fatalf("wrong full population: samples=%d omitted cohorts=%d", item.tree.SampleCount, item.tree.OmittedCohorts)
		}
		for _, c := range item.tree.Cohorts {
			if c.OmittedNodes != 0 {
				t.Fatal("fixture must not use a node-limited display")
			}
			for _, n := range c.Nodes {
				if n.OmittedChildren != 0 || n.Depth+1 >= item.tree.DisplayDepth {
					t.Fatal("fixture must have slack after the deepest visible level")
				}
			}
		}
	}
	prompt := dependencyObservationMessages(t, ctx)
	if got := strings.Count(prompt, "Sampling prefix tree: status=available"); got != 1 {
		t.Fatalf("actual Finalizer received %d copies of one complete display population (two different nonbinding depth limits)", got)
	}
	for _, field := range []string{items[1].id, items[1].path, items[1].query, items[0].id} {
		if field == "" || !strings.Contains(prompt, field) {
			t.Fatal("coalescing dropped one of the exact original query coordinates")
		}
	}
	reuse := ""
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "native_tree_reuse ") {
			reuse = line
			break
		}
	}
	if reuse == "" || !strings.Contains(reuse, items[1].id) || !strings.Contains(reuse, items[0].id) ||
		!strings.Contains(reuse, "query_display_depth="+strconv.Itoa(b)) || !strings.Contains(reuse, "shared_display_depth="+strconv.Itoa(a)) {
		t.Fatalf("reuse did not retain the independent depth settings and coordinates: %q", reuse)
	}
	return prompt
}

func TestHMC365PublicSamplingDisplayDepthReuse(t *testing.T) {
	ctx := hmc267PresentationQueries(t, []map[string]any{{"view": "perf_sampling_tree", "sampling_depth": 6}, {"view": "perf_timeline", "bucket_ms": 100}})
	hmc365AssertDepthReuse(t, ctx, 6, 4, 5)
	t.Run("actually_hidden_nodes", func(t *testing.T) {
		ctx := hmc267PresentationQueries(t, []map[string]any{{"view": "perf_sampling_tree", "sampling_depth": 1}, {"view": "perf_timeline", "bucket_ms": 100}})
		prompt := dependencyObservationMessages(t, ctx)
		if strings.Count(prompt, "Sampling prefix tree: status=available") != 2 || strings.Contains(prompt, "native_tree_reuse ") {
			t.Fatal("a depth-clipped graph was aliased to a full one")
		}
	})
}

func TestHMC365WholeOriginalSamplingDepthReuse(t *testing.T) {
	path := os.Getenv("CODRAX_HMC365_RAW_ORIGINAL")
	if path == "" {
		t.Skip("requires the pinned whole raw sample file")
	}
	const digest = "da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632"
	if h, n := hmc333FullSourceHash(t, path); h != digest || n != 30062585 {
		t.Fatal("not the original pinned file")
	}
	for _, key := range []string{"CODRAX_TRACE_STREAMER", "CODRAX_HIPERF_HOST", "CODRAX_SIMPLEPERF_REPORT_SAMPLE"} {
		t.Setenv(key, filepath.Join(t.TempDir(), "missing-provider"))
	}
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, RuntimeAnchor: t.TempDir(), PreviewBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	const tid = 1609
	start, end := 486489.4, 486489.6
	rm := types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
		RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: "thread", PID: tid, Thread: "poll_report", Source: "user_explicit", Confidence: 1}}}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		AttachedTraceMaterial: material, AttachedHitrace: material.Preview(), AttachedHitraceSource: path,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: types.NewMutableState("附件中这个时窗的采样热点是什么？")}
	ctx.Mutable.SetRequestModel(rm)
	// Mirror the original natural pair; neither query is inferred from its
	// sibling's selected views or bypasses the current artifact preparation.
	var results []types.ToolResult
	for _, selector := range []map[string]any{{"view": "perf_sampling_tree", "sampling_depth": 6}, {"view": "perf_timeline", "bucket_ms": 10}} {
		selector["source"], selector["pid"], selector["time_start"], selector["time_end"] = "attached_trace", tid, start, end
		raw, _ := json.Marshal(selector)
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !r.Success {
			t.Fatalf("original native query %s: %v %s", raw, err, r.Summary)
		}
		results = append(results, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	prompt := hmc365AssertDepthReuse(t, ctx, 6, 4, 164)
	if strings.Count(prompt, "original_record_header_offset=") != 8 || !strings.Contains(prompt, "total_weight=259") {
		t.Fatal("shared display did not preserve the original eight displayed headers and unchanged weight")
	}
	if h, _ := hmc333FullSourceHash(t, path); h != digest {
		t.Fatal("read-only replay changed the original")
	}
}
