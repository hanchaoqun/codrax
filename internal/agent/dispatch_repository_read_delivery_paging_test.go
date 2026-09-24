package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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

type repositoryPagedDeliveryLLM struct {
	t                  *testing.T
	ctx                *types.AgentContext
	root, digest, body string
	page, round        int
	changeVersion      bool
}

func (l *repositoryPagedDeliveryLLM) Chat(_ context.Context, messages []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.round++
	if l.ctx.Mutable.CompleteDispatchDeliveredRepositoryFileReadVersion(l.root, "check.py", l.digest) {
		l.t.Fatal("full delivery certified before the request containing the final page succeeded")
	}
	if l.round > 1 {
		wanted := fmt.Sprintf("old_%03d = 7", (l.round-2)*l.page)
		if l.changeVersion && l.round == 3 {
			wanted = strings.Replace(wanted, "old_", "new_", 1)
		}
		found := false
		for _, message := range messages {
			if message.Role == "tool" && message.ToolCallID == fmt.Sprintf("page-%d", l.round-1) {
				found = strings.Contains(message.Content, wanted)
			}
		}
		if !found {
			l.t.Fatalf("actual request lacks expected page content %q", wanted)
		}
	}
	if l.round == 3 {
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "consume", Name: "emit_change_plan", Params: json.RawMessage(`{}`)}}}, nil
	}
	if l.changeVersion && l.round == 2 {
		if err := os.WriteFile(filepath.Join(l.root, "check.py"), []byte(strings.ReplaceAll(l.body, "old_", "new_")), 0600); err != nil {
			l.t.Fatal(err)
		}
	}
	params := fmt.Sprintf(`{"path":"check.py","offset":%d,"limit":%d}`, (l.round-1)*l.page, l.page)
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("page-%d", l.round), Name: "read_file", Params: json.RawMessage(params)}}}, nil
}
func (*repositoryPagedDeliveryLLM) ModelID() string               { return "paged-delivery-public" }
func (*repositoryPagedDeliveryLLM) MaxContextTokens() int         { return 128000 }
func (*repositoryPagedDeliveryLLM) MaxOutputTokens() int          { return 4096 }
func (*repositoryPagedDeliveryLLM) RequestTimeout() time.Duration { return 0 }
func (*repositoryPagedDeliveryLLM) RetryMaxAttempts() int         { return 0 }

func TestRepositoryReadDeliveryActualPagedAgentRequest(t *testing.T) {
	for _, changeVersion := range []bool{false, true} {
		t.Run(fmt.Sprintf("change_version_%v", changeVersion), func(t *testing.T) {
			root := t.TempDir()
			physical, err := filepath.EvalSymlinks(root)
			if err != nil {
				t.Fatal(err)
			}
			page := tool.ReadFileSmallLimitThreshold + 1
			var source strings.Builder
			for i := 0; i < 2*page; i++ {
				fmt.Fprintf(&source, "old_%03d = 7\n", i)
			}
			body := source.String()
			if err := os.WriteFile(filepath.Join(root, "check.py"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
			ctx := &types.AgentContext{RepoRoot: root, WorktreePath: root, WorkDir: t.TempDir(), Mode: types.ModePlan, Stage: types.StagePlan, AgentName: types.AgentPlanner, Mutable: types.NewMutableState("inspect all pages")}
			calls := 0
			registry := tool.NewRegistry()
			registry.Register(&tool.ReadFile{})
			registry.Register(repositoryDeliveryConsumer{t: t, root: physical, digest: digest, want: !changeVersion, calls: &calls})
			adapter := &repositoryPagedDeliveryLLM{t: t, ctx: ctx, root: physical, digest: digest, body: body, page: page, changeVersion: changeVersion}
			deps := &Dependencies{Tools: registry, LLM: adapter, MaxIterations: 3, Emit: func(render.Event) {}}
			out, err := NewBaseAgent(types.AgentPlanner, deps, &stubEvaluator{}).Execute(ctx, &skill.Config{ToolSuggestions: []string{"read_file", "emit_change_plan"}})
			if err != nil || out == nil || adapter.round != 3 || calls != 1 {
				t.Fatalf("actual paged dispatch: rounds=%d consumers=%d err=%v output=%+v", adapter.round, calls, err, out)
			}
			if got := ctx.Mutable.CompleteDispatchDeliveredRepositoryFileReadVersion(physical, "check.py", digest); got != !changeVersion {
				t.Fatalf("original complete delivery=%v", got)
			}
			if changeVersion {
				changed := strings.ReplaceAll(body, "old_", "new_")
				newDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(changed)))
				if ctx.Mutable.CompleteDispatchDeliveredRepositoryFileReadVersion(physical, "check.py", newDigest) {
					t.Fatal("pages from different file versions combined into delivery")
				}
			}
		})
	}
}
