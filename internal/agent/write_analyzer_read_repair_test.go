package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The real emission tool asks for an exact read of this non-conventional
// baseline. A scripted model supplies choices only, not a successful receipt.
type writeAnalyzerReadRepairLLM struct {
	t             *testing.T
	calls         int
	repairReads   int
	prematureRead bool
}

func (l *writeAnalyzerReadRepairLLM) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if l.calls == 1 {
		var text strings.Builder
		for _, message := range messages {
			text.WriteString(message.Content)
		}
		if !strings.Contains(text.String(), types.WriteExistingTestIntentTeaching) || !strings.Contains(text.String(), "one successful emission") || strings.Contains(text.String(), "do NOT call emit_write_analysis more than once") {
			l.t.Error("actual model context lost shared execution intent or still forbids repair after rejection")
		}
	}
	available := toolSchemaNameSet(schemas)
	call := func(id, name string, params any) llm.ToolCall {
		raw, _ := json.Marshal(params)
		return llm.ToolCall{ID: id, Name: name, Params: raw}
	}
	var calls []llm.ToolCall
	switch l.calls {
	case 1:
		for _, id := range []string{"a", "b", "c", "d"} {
			calls = append(calls, call(id, "read_file", map[string]any{"path": "notes.txt"}))
		}
	case 3:
		if !available["read_file"] || available["repo_map"] || available["grep"] || available["list_files"] {
			l.t.Errorf("rejected emit needs one narrow read opportunity, got %v", available)
		}
		calls = append(calls, call("repair-read", "read_file", map[string]any{"path": "oracle.data"}))
		if l.repairReads > 1 {
			calls = append(calls, call("repair-context", "read_file", map[string]any{"path": "notes.txt"}))
		}
	default:
		if available["read_file"] {
			l.t.Error("ordinary budget exhaustion must retain emit-only surface")
		}
		calls = append(calls, call("emit", "emit_write_analysis", map[string]any{
			"task":        map[string]any{"kind": "bugfix", "scope": "micro", "summary": "retain the intentional baseline"},
			"risk":        map[string]any{"overall": "low"},
			"constraints": []map[string]string{{"kind": "preserve_regression_test", "target": "oracle.data"}},
		}))
		if l.calls == 2 && l.prematureRead {
			calls = append(calls, call("not-yet-allowed", "read_file", map[string]any{"path": "oracle.data"}))
		}
	}
	return llm.Response{ToolCalls: calls}, nil
}
func (*writeAnalyzerReadRepairLLM) ModelID() string               { return "write-read-repair-fixture" }
func (*writeAnalyzerReadRepairLLM) MaxContextTokens() int         { return 128000 }
func (*writeAnalyzerReadRepairLLM) MaxOutputTokens() int          { return 4096 }
func (*writeAnalyzerReadRepairLLM) RequestTimeout() time.Duration { return 0 }
func (*writeAnalyzerReadRepairLLM) RetryMaxAttempts() int         { return 0 }

func TestWriteAnalyzerReadRepairActualLoop(t *testing.T) {
	for _, reads := range []int{1, 2} {
		t.Run(string(rune('0'+reads)), func(t *testing.T) { testWriteAnalyzerReadRepairActualLoop(t, reads, false) })
	}
	t.Run("same-batch-unavailable-read", func(t *testing.T) { testWriteAnalyzerReadRepairActualLoop(t, 1, true) })
}

func testWriteAnalyzerReadRepairActualLoop(t *testing.T, reads int, prematureRead bool) {
	root := t.TempDir()
	for path, content := range map[string]string{"notes.txt": "repository overview\n", "oracle.data": "intentional baseline bytes\n"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &types.AgentContext{AgentName: types.AgentWriteAnalyzer, Stage: types.StageWriteAnalyze, Mode: types.ModePlan, RepoRoot: root, WorkDir: root, Mutable: types.NewMutableState("fix behavior and preserve oracle.data")}
	registry := toolpkg.NewRegistry()
	registry.Register(&toolpkg.ReadFile{})
	registry.Register(&toolpkg.EmitWriteAnalysis{})
	adapter := &writeAnalyzerReadRepairLLM{t: t, repairReads: reads, prematureRead: prematureRead}
	ag := NewWriteAnalyzerAgent(&Dependencies{Tools: registry, LLM: adapter, MaxIterations: 6, Emit: func(render.Event) {}})
	skills := skill.NewRegistry()
	skill.RegisterDefaults(skills)
	sk, err := skills.Get("write-analysis-skill")
	if err != nil {
		t.Fatal(err)
	}
	out, err := ag.Execute(ctx, sk)
	if err != nil || out == nil || out.Error != "" {
		t.Fatalf("actual loop: %v %+v", err, out)
	}
	rejectedReads := 0
	if prematureRead {
		rejectedReads = 1
	}
	if adapter.calls != 4 || len(out.ToolResults) != 6+reads+rejectedReads {
		t.Fatalf("unexpected dispatch: calls=%d out=%+v", adapter.calls, out)
	}
	if out.ToolResults[4].Success || !strings.Contains(out.ToolResults[4].Summary, "no matching current read") || !out.ToolResults[5+rejectedReads].Success || !out.ToolResults[len(out.ToolResults)-1].Success {
		t.Fatalf("must reject, read real file, then accept: %+v", out.ToolResults)
	}
	if prematureRead && out.ToolResults[5].Success {
		t.Fatal("repair opened in the old batch instead of the next round")
	}
	ir := ctx.Mutable.WriteAnalysisIR()
	if ir == nil || len(ir.Request.Constraints) != 1 || ir.Request.Constraints[0].Target != "oracle.data" || ctx.Mutable.ChangePlan() != nil || ctx.Mutable.ChangeReport() != nil {
		t.Fatalf("repair lost constraint or minted write/verification authority: %+v", ir)
	}
	data, err := os.ReadFile(filepath.Join(root, "oracle.data"))
	if err != nil || string(data) != "intentional baseline bytes\n" {
		t.Fatal("repair modified protected file")
	}
}

func TestWriteAnalyzerReadRepairIsOnceBoundedAndTyped(t *testing.T) {
	for _, success := range []bool{true, false} {
		e := &writeAnalyzerEvaluator{prescanToolCalls: writeAnalyzerPrescanToolBudget}
		observe := func(results ...types.ToolResult) {
			e.ObserveToolResults(nil, LoopObservation{CurrentToolResults: results})
		}
		check := func(read bool) {
			t.Helper()
			names := toolSchemaNameSet(e.FilterToolSchemas(nil, writeAnalyzerTestSchemas()))
			if names["read_file"] != read || !names["emit_write_analysis"] || names["repo_map"] || names["grep"] || names["list_files"] {
				t.Fatalf("read=%v surface=%v", read, names)
			}
		}
		check(false)
		observe(types.ToolResult{ToolName: "read_file", Summary: "emit_write_analysis failed: read again"})
		check(false) // Failure prose from another tool is not an emission receipt.
		observe(types.ToolResult{ToolName: "emit_write_analysis", Success: true})
		check(false)
		observe(types.ToolResult{ToolName: "emit_write_analysis", Success: false, Summary: "arbitrary localized reason"})
		check(true)
		observe(types.ToolResult{ToolName: "read_file", Success: success})
		check(false) // Even a failed repair read consumes the one opportunity.
		observe(types.ToolResult{ToolName: "emit_write_analysis", Success: false})
		check(false)
		e.BuildInitialInstruction(&types.AgentContext{Mutable: types.NewMutableState("next dispatch")}, nil)
		e.ObserveToolResults(nil, LoopObservation{CurrentToolResults: []types.ToolResult{{ToolName: "read_file", Success: true}, {ToolName: "read_file", Success: true}, {ToolName: "read_file", Success: true}, {ToolName: "read_file", Success: true}}})
		observe(types.ToolResult{ToolName: "emit_write_analysis", Success: false})
		check(true) // A fresh dispatch gets its own bounded repair opportunity.
		observe(types.ToolResult{ToolName: "emit_write_analysis", Success: false})
		check(false) // Not using an offered read round does not renew it.
	}
}
