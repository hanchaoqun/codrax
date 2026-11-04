package agent

import (
	"encoding/json"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc344RuntimeSearchWritingContext(t *testing.T, ctx *types.AgentContext, native types.ToolResult) string {
	t.Helper()
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = nil
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	var payload string
	for _, r := range native.Observations {
		if r.SourceRef.PayloadRef != "" {
			payload = r.SourceRef.PayloadRef
			break
		}
	}
	if payload == "" {
		t.Fatal("native public JSON missing")
	}
	results := []types.ToolResult{native}
	for _, pattern := range []string{"node_count", "no_such_literal_918234"} {
		params, _ := json.Marshal(map[string]any{"path": payload, "pattern": pattern, "fixed_string": true, "line_start": 1, "line_end": 100000})
		r, err := (&tool.GrepTool{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !r.Success {
			t.Fatalf("actual native-result search: %v %s", err, r.Summary)
		}
		results = append(results, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	var last string
	for _, sub := range []bool{false, true} {
		facts := hmc276ParsedFacts(t, sub, results)
		bus := types.ToolBusContext(ctx, types.AgentFinalizer)
		bus.RepoFacts = facts
		ac := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
		_, messages := finalizerReaderFieldMessages(t, ac)
		section := hmc276KnownFactsSection(t, messages)
		count := 0
		for _, line := range strings.Split(section, "\n") {
			if strings.HasPrefix(line, "[grep]") {
				count++
				if !strings.Contains(line, `typed_origins=["runtime_artifact"]`) || strings.Contains(line, "current_source") || strings.Contains(line, "repo_negative_search") {
					t.Fatalf("actual writing context still gives search the wrong origin: %s", line)
				}
			}
		}
		if count != 2 {
			t.Fatalf("independent successful/zero searches lost: %d", count)
		}
		if !strings.Contains(messages, "business_population") || !strings.Contains(messages, "总耗时") {
			t.Fatal("classification removed native account context")
		}
		last = messages
	}
	return last
}

func TestHMC344RuntimeSearchOriginActualFinalizer(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	native := hmc320BusinessQuery(t, ctx, path, nil)
	before, _ := json.Marshal(native)
	params, _ := json.Marshal(map[string]any{"path": native.RawRef, "pattern": "node_count", "fixed_string": true, "files_only": true})
	files, err := (&tool.GrepTool{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !files.Success || files.PathDiscovery == nil || files.PathDiscovery.NoMatches {
		t.Fatalf("actual files-only result missing: %v %+v", err, files)
	}
	if analyzerGrepPrescanReady(files) || analyzerPrescanResultHasSameBatchSignal(files) {
		t.Fatal("runtime result file closed repository classification pre-scan")
	}
	hmc344RuntimeSearchWritingContext(t, ctx, native)
	after, _ := json.Marshal(native)
	if string(before) != string(after) {
		t.Fatal("Known Facts classification changed original query facts")
	}
}
