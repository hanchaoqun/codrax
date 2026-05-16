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
