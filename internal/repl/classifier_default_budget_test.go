package repl

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Capture the actual context passed through the production classifier and
// wrapper stack, then return immediately: a ten-minute budget need not cost a
// ten-minute test. The real config factory supplies transport/default metadata;
// no HTTP request is sent and no provider configuration is read from disk.
type classifierDefaultDeadlineProbe struct {
	llm.Adapter
	ahead       time.Duration
	hasDeadline bool
}

func (p *classifierDefaultDeadlineProbe) StreamingLivenessWatchdogEnabled() bool {
	return p.Adapter.(llm.StreamingLivenessReporter).StreamingLivenessWatchdogEnabled()
}

func (p *classifierDefaultDeadlineProbe) Chat(ctx context.Context, _ []llm.Message, tools []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	deadline, ok := ctx.Deadline()
	p.hasDeadline = ok
	if ok {
		p.ahead = time.Until(deadline)
	}
	if len(tools) == 1 && tools[0].Name == chitchatClassifierTool.Name {
		return llm.Response{ToolCalls: []llm.ToolCall{{Name: chitchatClassifierTool.Name, Params: []byte(`{"decision":"chitchat","reason":"casual conversation"}`)}}}, nil
	}
	return turnPolicyResp(singleShotDataPolicyJSON), nil
}

func TestClassifierPublicDefaultBudgetMatchesFactoryWithoutWaiting(t *testing.T) {
	if turnPolicyClassifierTimeoutExplicit || singleShotRoutePolicyTimeoutExplicit {
		t.Fatal("premise: this test must exercise absent configuration, not a prior explicit override")
	}
	for _, stream := range []bool{false, true} {
		for _, lane := range []string{"repl", "cli", "repl_in_flight", "legacy", "legacy_in_flight"} {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, lane), func(t *testing.T) {
				adapter, err := llm.NewFromConfig(types.LLMProviderConfig{
					Provider: "openai", APIKey: "test-key", Model: "test-model", BaseURL: "http://example.invalid", Stream: &stream,
				})
				if err != nil {
					t.Fatal(err)
				}
				if adapter.RequestTimeout() != 10*time.Minute {
					t.Fatalf("premise: shared non-streaming factory default=%s", adapter.RequestTimeout())
				}
				probe := &classifierDefaultDeadlineProbe{Adapter: adapter}
				wrapped, _ := newDirectBudgetWrapper(t, llm.NewTelemetryAdapter(llm.NewFallbackAdapter(probe), func(llm.RequestTelemetry) {}))
				classifier := NewChitchatClassifier(wrapped)
				repl := &REPL{chitchatClassifier: classifier}
				var policy TurnPolicy
				var chat bool
				switch lane {
				case "cli":
					policy, err = classifier.(SingleShotTurnPolicyClassifier).ClassifyPolicySingleShot(context.Background(), "inspect supplied artifact", "", false)
				case "repl":
					policy, err = classifier.(TurnPolicyClassifier).ClassifyPolicy(context.Background(), "inspect supplied artifact", "", false)
				case "repl_in_flight":
					policy, err = repl.runTurnPolicyClassifierInFlight(func(ctx context.Context) (TurnPolicy, error) {
						return classifier.(TurnPolicyClassifier).ClassifyPolicy(ctx, "inspect supplied artifact", "", false)
					})
				case "legacy":
					chat, err = classifier.Classify(context.Background(), "hello", "")
				case "legacy_in_flight":
					chat, err = repl.runBoolClassifierInFlight(func(ctx context.Context) (bool, error) {
						return classifier.Classify(ctx, "hello", "")
					})
				}
				if err != nil || (lane == "legacy" || lane == "legacy_in_flight") && !chat ||
					(lane != "legacy" && lane != "legacy_in_flight") && policy.Route != RouteData {
					t.Fatalf("budget plumbing changed the model-selected route: route=%s chat=%t err=%v", policy.Route, chat, err)
				}
				if stream {
					if probe.hasDeadline {
						t.Fatalf("default classifier budget became a stream-age limit: %s", probe.ahead)
					}
				} else if !probe.hasDeadline || probe.ahead < adapter.RequestTimeout()-time.Second || probe.ahead > adapter.RequestTimeout() {
					t.Fatalf("classifier still shortens shared non-streaming default: actual=%s has_deadline=%t factory=%s", probe.ahead, probe.hasDeadline, adapter.RequestTimeout())
				}
			})
		}
	}
}
