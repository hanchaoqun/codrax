package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type repositoryDeliveryLLM struct {
	t                  *testing.T
	ctx                *types.AgentContext
	kind, root, digest string
	round              int
}

func (l *repositoryDeliveryLLM) Chat(_ context.Context, messages []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.round++
	if l.round == 1 {
		read := llm.ToolCall{ID: "read-1", Name: "read_file", Params: json.RawMessage(`{"path":"check.py"}`)}
		calls := []llm.ToolCall{read}
		switch l.kind {
		case "same_batch":
			calls = append(calls, llm.ToolCall{ID: "emit-1", Name: "emit_change_plan", Params: json.RawMessage(`{}`)})
		case "empty_call_id":
			calls[0].ID = ""
		case "duplicate_call_id":
			calls = append(calls, read)
		}
		return llm.Response{ToolCalls: calls}, nil
	}
	if l.ctx.Mutable.CompleteDispatchDeliveredRepositoryFileReadVersion(l.root, "check.py", l.digest) {
		l.t.Fatal("a request was certified before its successful response")
	}
	var full, stub bool
	for _, message := range messages {
		if message.Role != "tool" || message.ToolCallID != "read-1" {
			continue
		}
		full = full || strings.Contains(message.Content, "assert result == 7")
		stub = stub || strings.HasPrefix(message.Content, "[earlier tool result elided")
	}
	if l.kind == "pruned" {
		if full || !stub {
			l.t.Fatal("real request did not exercise history pruning")
		}
	} else if l.kind != "empty_call_id" && !full {
		l.t.Fatal("real source bytes missing from actual model request")
	}
	if l.kind == "failed_llm" {
		return llm.Response{}, errors.New("model request failed")
	}
	if l.kind == "cross_dispatch" {
		l.ctx.Mutable.ResetDispatchToolResults()
	}
	if l.kind == "same_batch" {
		return llm.Response{Content: "done"}, nil
	}
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: "emit-2", Name: "emit_change_plan", Params: json.RawMessage(`{}`)}}}, nil
}
func (*repositoryDeliveryLLM) ModelID() string               { return "read-delivery-public" }
func (*repositoryDeliveryLLM) MaxContextTokens() int         { return 128000 }
func (*repositoryDeliveryLLM) MaxOutputTokens() int          { return 4096 }
func (*repositoryDeliveryLLM) RequestTimeout() time.Duration { return 0 }
func (*repositoryDeliveryLLM) RetryMaxAttempts() int         { return 0 }

// A consumer at the real tool-execution boundary observes the same private
// qualification the registration transaction will consume. It grants no plan.
type repositoryDeliveryConsumer struct {
	tool.NonEvidenceTool
	t            *testing.T
	root, digest string
	want         bool
	calls        *int
}

func (repositoryDeliveryConsumer) Name() string  { return "emit_change_plan" }
func (repositoryDeliveryConsumer) IsWrite() bool { return false }
func (repositoryDeliveryConsumer) Description() string {
	return "Inspect native delivery qualification without changing a plan"
}
func (repositoryDeliveryConsumer) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (c repositoryDeliveryConsumer) Execute(ctx *types.BusContext, _ json.RawMessage) (types.ToolResult, error) {
	*c.calls++
	if got := ctx.Mutable.CompleteDispatchDeliveredRepositoryFileReadVersion(c.root, "check.py", c.digest); got != c.want {
		c.t.Errorf("tool boundary delivered=%v, want %v", got, c.want)
	}
	return types.ToolResult{ToolName: c.Name(), Success: true, Summary: "delivery checked"}, nil
}

func TestRepositoryReadDeliveryActualAgentRequest(t *testing.T) {
	for _, kind := range []string{"delivered", "same_batch", "pruned", "failed_llm", "cross_dispatch", "empty_call_id", "duplicate_call_id", "read_mode"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			const body = "result = 7\nassert result == 7\n"
			if err := os.WriteFile(filepath.Join(root, "check.py"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			physical, err := filepath.EvalSymlinks(root)
			if err != nil {
				t.Fatal(err)
			}
			digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
			ctx := &types.AgentContext{RepoRoot: root, WorktreePath: root, WorkDir: t.TempDir(), Mode: types.ModePlan, Stage: types.StagePlan, AgentName: types.AgentPlanner, Mutable: types.NewMutableState("inspect existing test")}
			if kind == "read_mode" {
				ctx.Mode = types.ModeRead
			}
			calls := 0
			registry := tool.NewRegistry()
			registry.Register(&tool.ReadFile{})
			registry.Register(repositoryDeliveryConsumer{t: t, root: physical, digest: digest, want: kind == "delivered", calls: &calls})
			adapter := &repositoryDeliveryLLM{t: t, ctx: ctx, kind: kind, root: physical, digest: digest}
			deps := &Dependencies{Tools: registry, LLM: adapter, MaxIterations: 2, Emit: func(render.Event) {}}
			if kind == "pruned" {
				deps.AgentSettings.MaxToolHistoryBytes = 1
			}
			base := NewBaseAgent(types.AgentPlanner, deps, &stubEvaluator{})
			out, err := base.Execute(ctx, &skill.Config{ToolSuggestions: []string{"read_file", "emit_change_plan"}})
			if (err != nil) != (kind == "failed_llm") {
				t.Fatalf("unexpected dispatch error: %v", err)
			}
			wantCalls := 1
			if kind == "failed_llm" {
				wantCalls = 0
			}
			if out == nil || adapter.round != 2 || calls != wantCalls {
				t.Fatalf("actual loop not exercised: rounds=%d consumers=%d output=%+v", adapter.round, calls, out)
			}
			want := kind == "delivered" || kind == "same_batch"
			if got := ctx.Mutable.CompleteDispatchDeliveredRepositoryFileReadVersion(physical, "check.py", digest); got != want {
				t.Fatalf("final delivered=%v want %v", got, want)
			}
			if kind != "cross_dispatch" && kind != "read_mode" && !ctx.Mutable.CompleteDispatchRepositoryFileReadVersion(physical, "check.py", digest) {
				t.Fatal("legacy successful read qualification changed")
			}
			if ctx.Mutable.ChangePlan() != nil || ctx.Mutable.ChangeReport() != nil {
				t.Fatal("inspection gained plan/execution authority")
			}
			got, err := os.ReadFile(filepath.Join(root, "check.py"))
			if err != nil || string(got) != body {
				t.Fatal("read changed source bytes")
			}
		})
	}
}

func TestRepositoryReadDeliveryExactRequestMessage(t *testing.T) {
	root := t.TempDir()
	body := "assert 1 == 1\n"
	if err := os.WriteFile(filepath.Join(root, "check.py"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{RepoRoot: root, WorkDir: t.TempDir(), Mode: types.ModePlan, Mutable: types.NewMutableState("exact delivery")}
	result, err := (&tool.ReadFile{}).Execute(types.ToolBusContext(ctx, types.AgentPlanner), json.RawMessage(`{"path":"check.py"}`))
	if err != nil || !result.Success {
		t.Fatalf("native read: %v %s", err, result.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	d := newDispatchRepositoryReadDelivery(ctx)
	d.observe("read", result)
	exact := llm.Message{Role: "tool", ToolCallID: "read", Content: result.Summary}
	for _, kind := range []string{"exact", "changed_body", "wrong_id", "wrong_role", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			message := exact
			switch kind {
			case "changed_body":
				message.Content = strings.ReplaceAll(message.Content, "1 == 1", "1 == 2")
			case "wrong_id":
				message.ToolCallID = "other"
			case "wrong_role":
				message.Role = "user"
			}
			messages := []llm.Message{message}
			if kind == "duplicate" {
				messages = append(messages, message)
			}
			if got := len(d.snapshot(messages)); (got == 1) != (kind == "exact") {
				t.Fatalf("snapshot count=%d for %s", got, kind)
			}
		})
	}
}
