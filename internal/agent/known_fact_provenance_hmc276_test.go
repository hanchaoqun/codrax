package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc276ParsedFacts(t *testing.T, sub bool, results []types.ToolResult) []types.RepoFact {
	t.Helper()
	var out *StageOutput
	var err error
	ctx := &types.AgentContext{}
	if sub {
		out, err = (&subExplorerEvaluator{}).ParseOutput(ctx, []llm.Message{{Role: "assistant", Content: "done"}}, results, nil)
	} else {
		out, err = (&explorerEvaluator{}).ParseOutput(ctx, []llm.Message{{Role: "assistant", Content: "done"}}, results, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return out.NewFacts
}

func hmc276KnownFactsSection(t *testing.T, messages string) string {
	t.Helper()
	_, rest, ok := strings.Cut(messages, "## Known Facts\n")
	if !ok {
		t.Fatal("actual Finalizer did not receive Known Facts")
	}
	text, _, _ := strings.Cut(rest, "\n## ")
	return text
}

func TestHMC276KnownFactSourcesActualFinalizer(t *testing.T) {
	ctx, native := hmc275IOContext(t, []string{hmc275IOPath(t)}, []map[string]any{{"time_start": 1.0, "time_end": 1.01}})
	// Match the real A generic route: a native result alone does not activate
	// the separate typed-support suppression of Known Facts.
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = nil
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	results := append([]types.ToolResult(nil), native...)
	for _, value := range []int{0, 5, 7, 10} {
		args, _ := json.Marshal(map[string]any{"command": fmt.Sprintf("awk 'BEGIN { print \"busy_ms=%d\" }'", value)})
		r, err := (&tool.ExecCommand{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !r.Success || r.CommandMeasurement != nil {
			t.Fatalf("ordinary command fixture: %v / %+v", err, r)
		}
		results = append(results, r)
	}
	args := json.RawMessage(`{"command":"awk 'BEGIN { print 7 }'"}`)
	count, err := (&tool.ExecCommand{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !count.Success || count.CommandMeasurement == nil {
		t.Fatalf("count fixture: %v / %+v", err, count)
	}
	results = append(results, count)
	before, _ := json.Marshal(results)
	for _, sub := range []bool{false, true} {
		t.Run(fmt.Sprintf("sub=%t", sub), func(t *testing.T) {
			facts := hmc276ParsedFacts(t, sub, results)
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			bus.RepoFacts = facts
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			ac := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			_, messages := finalizerReaderFieldMessages(t, ac)
			section := hmc276KnownFactsSection(t, messages)
			if strings.Contains(section, "confidence: 0.80") {
				t.Fatal("tool-category confidence still displayed as factual confidence beside conflicting calculations")
			}
			if strings.Count(section, "interpretation_unverified") < 4 {
				t.Fatalf("ordinary calculations lack explicit interpretation boundary: %s", section)
			}
			for _, want := range []string{"busy_ms=0", "busy_ms=5", "busy_ms=7", "busy_ms=10", "command_measurement", "typed_records=", "fact_slot=", "source_ref="} {
				if !strings.Contains(section, want) {
					t.Errorf("lost independent output/provenance %q: %s", want, section)
				}
			}
			if !strings.Contains(messages, "1.009") || !strings.Contains(messages, "1.01") {
				t.Fatal("native write-tail source projection disappeared")
			}
		})
	}
	after, _ := json.Marshal(results)
	if string(before) != string(after) {
		t.Fatal("presentation changed tool success, measurements or original output")
	}
}
