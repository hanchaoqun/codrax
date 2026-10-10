package repl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
)

type namedInputRoutingAdapter struct {
	llm.Adapter
	messages []llm.Message
}

func (a *namedInputRoutingAdapter) Chat(_ context.Context, messages []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	a.messages = append([]llm.Message(nil), messages...)
	return turnPolicyResp(`{"route":"repo","needs_repo_access":true,"operation":"investigate","source":"current_message","confidence":0.95,"reason":"read supplied runtime measurements","requires_diagram":false}`), nil
}
func (*namedInputRoutingAdapter) RequestTimeout() time.Duration { return 0 }
func (*namedInputRoutingAdapter) MaxContextTokens() int         { return 100000 }
func (*namedInputRoutingAdapter) MaxOutputTokens() int          { return 0 }
func (*namedInputRoutingAdapter) RetryMaxAttempts() int         { return 0 }
func (*namedInputRoutingAdapter) ModelID() string               { return "routing-test" }

func TestHMC221NamedNativeContentReachesActualREPLClassifier(t *testing.T) {
	fixture, err := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/baseline.data")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Fatal(err)
	}
	request := "列出 " + fixture + " 的原始量测记录和区间"
	a := &namedInputRoutingAdapter{}
	r, _, _ := newTurnPolicyREPL(t, newPolicyStore(t), NewChitchatClassifier(a), &stubResponder{}, request+"\n/exit\n")
	r.runtimeAnchor = t.TempDir()
	if err := r.Loop(); err != nil {
		t.Fatal(err)
	}
	var content string
	for _, m := range a.messages {
		if m.Role == "user" {
			content = m.Content
		}
	}
	if !strings.Contains(content, "## current_named_inputs") || !strings.Contains(content, "trace_query") || !strings.Contains(content, "measurements") {
		t.Fatalf("native content capability missing at actual classifier adapter: %s", content)
	}
	if !strings.Contains(content, "## current: "+request) {
		t.Fatalf("current request changed: %s", content)
	}
}

func TestHMC221NamedInputContextIsCurrentAndKeepsLLMBudget(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/baseline.data")
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithNamedInputRoutingContext(context.Background(), path, "", t.TempDir())
	if namedInputRoutingContext(ctx) == "" {
		t.Fatal("missing current capability")
	}
	if _, ok := ctx.Deadline(); ok {
		t.Fatal("local probe budget leaked into LLM context")
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("deferred probe cancellation leaked into classifier: %v", err)
	}
	ctx = WithNamedInputRoutingContext(ctx, "explain this code", "", t.TempDir())
	if namedInputRoutingContext(ctx) != "" {
		t.Fatal("old named input became sticky")
	}
}
