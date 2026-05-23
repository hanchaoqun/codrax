package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type captureBusContextTool struct {
	toolpkg.ReadOnly
	toolpkg.NonEvidenceTool
	got *types.BusContext
}

func (t *captureBusContextTool) Name() string        { return "capture_bus_context" }
func (t *captureBusContextTool) Description() string { return "captures the bus context for tests" }
func (t *captureBusContextTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (t *captureBusContextTool) Execute(ctx *types.BusContext, _ json.RawMessage) (types.ToolResult, error) {
	t.got = ctx
	return types.ToolResult{ToolName: t.Name(), Success: true}, nil
}

func TestExecuteTool_PropagatesStageAndAttachmentsToBusContext(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureBusContextTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentLogTriager, &Dependencies{Tools: reg}, nil)
	ctx := &types.AgentContext{
		AgentName:       types.AgentLogTriager,
		Stage:           types.StageLogTriage,
		RepoRoot:        t.TempDir(),
		WorkDir:         t.TempDir(),
		Branch:          "main",
		Commit:          "deadbeef",
		AttachedLog:     "panic: boom",
		AttachedHitrace: "trace: frame",
		Mutable:         types.NewMutableState("test"),
	}

	res, _ := base.executeTool(ctx, llm.ToolCall{Name: capture.Name(), Params: json.RawMessage(`{}`)})
	if res == nil || !res.Success {
		t.Fatalf("executeTool failed: %+v", res)
	}
	if capture.got == nil {
		t.Fatal("tool did not receive BusContext")
	}
	if capture.got.PipelineStage != types.StageLogTriage {
		t.Fatalf("PipelineStage = %s, want %s", capture.got.PipelineStage, types.StageLogTriage)
	}
	if capture.got.ActiveAgent != types.AgentLogTriager {
		t.Fatalf("ActiveAgent = %s, want %s", capture.got.ActiveAgent, types.AgentLogTriager)
	}
	if capture.got.AttachedLog != ctx.AttachedLog {
		t.Fatalf("AttachedLog = %q, want %q", capture.got.AttachedLog, ctx.AttachedLog)
	}
	if capture.got.AttachedHitrace != ctx.AttachedHitrace {
		t.Fatalf("AttachedHitrace = %q, want %q", capture.got.AttachedHitrace, ctx.AttachedHitrace)
	}
}

func TestExecuteTool_MalformedParamsRejectedBeforeToolExecution(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureBusContextTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "bad-json",
		Name:   capture.Name(),
		Params: json.RawMessage(`}`),
	})
	if res == nil {
		t.Fatal("expected failed ToolResult")
	}
	if res.Success {
		t.Fatalf("malformed params should fail before execution: %+v", res)
	}
	if !strings.Contains(res.Summary, "malformed JSON tool arguments") {
		t.Fatalf("summary should explain malformed JSON, got %q", res.Summary)
	}
	if capture.got != nil {
		t.Fatalf("tool executed despite malformed params: %+v", capture.got)
	}
}

func TestExecuteTool_UnknownToolReturnsFailedResult(t *testing.T) {
	base := NewBaseAgent(types.AgentExtractor, &Dependencies{Tools: toolpkg.NewRegistry()}, nil)
	res, mcp := base.executeTool(&types.AgentContext{Stage: types.StageExtract}, llm.ToolCall{
		ID:     "unknown",
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"a.go"}`),
	})
	if mcp != nil {
		t.Fatalf("unknown local tool should not return MCP response: %+v", mcp)
	}
	if res == nil {
		t.Fatal("unknown tool should return a failed ToolResult, not disappear")
	}
	if res.Success {
		t.Fatalf("unknown tool result should fail: %+v", res)
	}
	for _, want := range []string{"tool \"read_file\" is not available", "stage extract", "emit_answer_symbol"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("unknown-tool summary missing %q: %q", want, res.Summary)
		}
	}
}

func TestExecuteTool_TruncatedParamsGetsTypedCompactGuidance(t *testing.T) {
	reg := toolpkg.NewRegistry()
	capture := &captureBusContextTool{}
	reg.Register(capture)

	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	res, _ := base.executeTool(&types.AgentContext{Stage: types.StageExplore}, llm.ToolCall{
		ID:     "truncated-json",
		Name:   capture.Name(),
		Params: json.RawMessage(`{"items":[{"summary":"unterminated}`),
	})
	if res == nil {
		t.Fatal("expected failed ToolResult")
	}
	if res.Success {
		t.Fatalf("truncated params should fail before execution: %+v", res)
	}
	for _, want := range []string{
		"truncated JSON tool arguments",
		"smaller native JSON object",
		"preserve model-authored prose",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
	if capture.got != nil {
		t.Fatalf("tool executed despite truncated params: %+v", capture.got)
	}
}
