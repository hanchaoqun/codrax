package repl

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

type directBudgetEventCounts struct {
	requests, responses, reasoning, all atomic.Int64
}

// Exercise the same non-nil-renderer constructor as cmd's
// withRenderLLMTelemetry. Count typed events instead of inspecting UI prose.
func newDirectBudgetWrapper(t *testing.T, inner llm.Adapter) (llm.Adapter, *directBudgetEventCounts) {
	t.Helper()
	wrapped := NewDirectLLMTraceAdapter(inner, render.New(io.Discard, true), types.AgentName("chitchat_classifier"), "")
	a, ok := wrapped.(*directLLMTraceAdapter)
	if !ok {
		t.Fatal("production constructor did not install the presentation wrapper")
	}
	events := &directBudgetEventCounts{}
	a.emit = func(event render.Event) {
		events.all.Add(1)
		switch event.Kind {
		case render.EventAgentThinking:
			events.requests.Add(1)
		case render.EventAgentResponse:
			events.responses.Add(1)
		case render.EventAgentReasoning:
			events.reasoning.Add(1)
		}
	}
	return wrapped, events
}

func TestDirectLLMTraceBudgetDefaultClassifierKeepsActualStreamAlive(t *testing.T) {
	for _, lane := range []string{"repl", "cli", "repl_in_flight", "legacy", "legacy_in_flight"} {
		t.Run(lane, func(t *testing.T) {
			routeClassifierTestDefaults(t, 30*time.Millisecond)
			legacy := lane == "legacy" || lane == "legacy_in_flight"
			observed := newRouteBudgetActiveSSE(t, legacy)
			wrapped, events := newDirectBudgetWrapper(t, observed)
			classifier := &llmChitchatClassifier{adapter: wrapped}
			r := &REPL{chitchatClassifier: classifier}
			var policy TurnPolicy
			var chat bool
			var err error
			switch lane {
			case "repl":
				policy, err = classifier.ClassifyPolicy(context.Background(), "inspect the supplied artifact", "", false)
			case "cli":
				policy, err = classifier.ClassifyPolicySingleShot(context.Background(), "inspect the supplied artifact", "", false)
			case "repl_in_flight":
				policy, err = r.runTurnPolicyClassifierInFlight(func(ctx context.Context) (TurnPolicy, error) {
					return classifier.ClassifyPolicy(ctx, "inspect the supplied artifact", "", false)
				})
			case "legacy":
				chat, err = classifier.Classify(context.Background(), "hello", "")
			case "legacy_in_flight":
				chat, err = r.runBoolClassifierInFlight(func(ctx context.Context) (bool, error) {
					return classifier.Classify(ctx, "hello", "")
				})
			}
			if err != nil || (legacy && !chat) || (!legacy && policy.Route != RouteData) {
				t.Fatalf("presentation wrapper cut model-owned route: route=%s chat=%t bytes=%d reads=%d reasoning=%d err=%v", policy.Route, chat, observed.bytes.Load(), observed.reads.Load(), observed.reasoning.Load(), err)
			}
			if observed.reasoning.Load() != 40 || observed.reads.Load() < 8 || events.requests.Load() != 1 || events.responses.Load() != 1 {
				t.Fatalf("stream or request events duplicated/lost: reasoning=%d reads=%d request=%d response=%d", observed.reasoning.Load(), observed.reads.Load(), events.requests.Load(), events.responses.Load())
			}
		})
	}
}

func TestDirectLLMTraceBudgetNestedFallbackUsesActualLeg(t *testing.T) {
	for _, streamFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonstream_then_stream", true: "stream_with_nonstream_backup"}[streamFirst], func(t *testing.T) {
			routeClassifierTestDefaults(t, 30*time.Millisecond)
			observed := newRouteBudgetActiveSSE(t, false)
			var inner llm.Adapter
			if streamFirst {
				inner = llm.NewFallbackAdapter(observed, blockingTurnPolicyAdapter{})
			} else {
				inner = llm.NewFallbackAdapter(blockingTurnPolicyAdapter{}, observed)
			}
			var telemetry atomic.Int64
			inner = llm.NewTelemetryAdapter(llm.NewFallbackAdapter(inner), func(llm.RequestTelemetry) { telemetry.Add(1) })
			wrapped, events := newDirectBudgetWrapper(t, inner)
			policy, err := (&llmChitchatClassifier{adapter: wrapped}).ClassifyPolicy(context.Background(), "inspect the supplied artifact", "", false)
			if err != nil || policy.Route != RouteData || observed.reasoning.Load() != 40 {
				t.Fatalf("mixed fallback inherited whole-stack timeout: route=%s reasoning=%d err=%v", policy.Route, observed.reasoning.Load(), err)
			}
			if telemetry.Load() != 1 || events.requests.Load() != 1 || events.responses.Load() != 1 {
				t.Fatalf("wrapper duplicated telemetry: telemetry=%d request=%d response=%d", telemetry.Load(), events.requests.Load(), events.responses.Load())
			}
		})
	}
}

type directBudgetWaitingLeaf struct {
	directTraceStubAdapter
	calls    atomic.Int64
	deadline atomic.Int64
}

func (a *directBudgetWaitingLeaf) Chat(ctx context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	a.calls.Add(1)
	deadline, _ := ctx.Deadline()
	a.deadline.Store(deadline.UnixNano())
	<-ctx.Done()
	return llm.Response{}, ctx.Err()
}

func TestDirectLLMTraceBudgetEveryNonStreamingLegKeepsItsOwnTimeout(t *testing.T) {
	first, second := &directBudgetWaitingLeaf{}, &directBudgetWaitingLeaf{}
	wrapped, events := newDirectBudgetWrapper(t, llm.NewFallbackAdapter(first, second))
	start := time.Now()
	_, err := chatWithClassifierBudget(context.Background(), wrapped, nil, nil, llm.ChatOptions{}, classifierCallBudget{requestTimeout: 30 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) || first.calls.Load() != 1 || second.calls.Load() != 1 ||
		second.deadline.Load()-first.deadline.Load() < int64(20*time.Millisecond) || time.Since(start) < 50*time.Millisecond {
		t.Fatalf("nonstream legs lost independent budgets: first=%d second=%d deadline_delta=%s elapsed=%s err=%v", first.calls.Load(), second.calls.Load(), time.Duration(second.deadline.Load()-first.deadline.Load()), time.Since(start), err)
	}
	if events.requests.Load() != 1 || events.responses.Load() != 1 {
		t.Fatalf("one logical request must produce one completion: request=%d response=%d", events.requests.Load(), events.responses.Load())
	}
}

func TestDirectLLMTraceBudgetStreamingFailureStillBoundsNonStreamingFallback(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush() // A real empty SSE response, eligible for fallback.
	}))
	defer server.Close()
	stream := llm.NewOpenAIAdapter("test-key", "closed-stream", server.URL, llm.AdapterOptions{
		Stream: true, RequestTimeout: time.Second, RetryMaxAttempts: 1,
	})
	nonstream := &directBudgetWaitingLeaf{}
	wrapped, events := newDirectBudgetWrapper(t, llm.NewFallbackAdapter(stream, nonstream))
	var fallback atomic.Int64
	_, err := chatWithClassifierBudget(context.Background(), wrapped, nil, nil, llm.ChatOptions{
		OnFallback: func(string, string, string) { fallback.Add(1) },
	}, classifierCallBudget{requestTimeout: 30 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) || requests.Load() != 1 || nonstream.calls.Load() != 1 || fallback.Load() != 1 {
		t.Fatalf("failed stream must not waive a nonstream backup budget: requests=%d backup=%d fallback=%d err=%v", requests.Load(), nonstream.calls.Load(), fallback.Load(), err)
	}
	if events.requests.Load() != 1 || events.responses.Load() != 1 {
		t.Fatalf("fallback duplicated logical request events: request=%d response=%d", events.requests.Load(), events.responses.Load())
	}
}

func TestDirectLLMTraceBudgetExplicitAndCallerLifetimesStillWin(t *testing.T) {
	for _, lane := range []string{"explicit_repl", "explicit_cli", "caller_cancel", "caller_deadline"} {
		t.Run(lane, func(t *testing.T) {
			routeClassifierTestDefaults(t, 30*time.Millisecond)
			if lane == "explicit_repl" {
				SetTurnPolicyClassifierTimeout(30 * time.Millisecond)
			}
			if lane == "explicit_cli" {
				SetSingleShotRoutePolicyTimeout(30 * time.Millisecond)
			}
			observed := newRouteBudgetActiveSSE(t, false)
			backup := &directBudgetWaitingLeaf{}
			wrapped, _ := newDirectBudgetWrapper(t, llm.NewFallbackAdapter(observed, backup))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if lane == "caller_deadline" {
				ctx, cancel = context.WithTimeout(ctx, 30*time.Millisecond)
				defer cancel()
			}
			if lane == "caller_cancel" {
				stop := time.AfterFunc(30*time.Millisecond, cancel)
				defer stop.Stop()
			}
			classifier := &llmChitchatClassifier{adapter: wrapped}
			var err error
			if lane == "explicit_repl" {
				_, err = classifier.ClassifyPolicy(ctx, "inspect the supplied artifact", "", false)
			} else {
				_, err = classifier.ClassifyPolicySingleShot(ctx, "inspect the supplied artifact", "", false)
			}
			want := context.DeadlineExceeded
			if lane == "caller_cancel" {
				want = context.Canceled
			}
			if !errors.Is(err, want) || observed.reads.Load() < 2 || observed.reasoning.Load() >= 40 || backup.calls.Load() != 0 {
				t.Fatalf("explicit lifetime was bypassed: reads=%d reasoning=%d fallback=%d err=%v", observed.reads.Load(), observed.reasoning.Load(), backup.calls.Load(), err)
			}
		})
	}
}

func TestDirectLLMTraceBudgetTrueByteSilenceStillFailsPrecisely(t *testing.T) {
	for _, firstChunk := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_data", true: "after_reasoning"}[firstChunk], func(t *testing.T) {
			routeClassifierTestDefaults(t, 4*time.Millisecond)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				if firstChunk {
					_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"working\"}}]}\n\n"))
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			observed := &routeStreamBudgetObserver{Adapter: llm.NewOpenAIAdapter("test-key", "idle-route", server.URL, llm.AdapterOptions{
				Stream: true, RequestTimeout: time.Second, RetryMaxAttempts: 1, StreamFirstByteTimeout: 60 * time.Millisecond, StreamStallTimeout: 60 * time.Millisecond,
			})}
			wrapped, _ := newDirectBudgetWrapper(t, observed)
			_, err := (&llmChitchatClassifier{adapter: wrapped}).ClassifyPolicy(context.Background(), "inspect the supplied artifact", "", false)
			want := llm.ErrStreamFirstByteTimeout
			if firstChunk {
				want = llm.ErrStreamStalled
			}
			if !errors.Is(err, want) || errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("wrapper changed byte-silence cause: firstChunk=%t err=%v", firstChunk, err)
			}
		})
	}
}

func TestDirectLLMTraceBudgetCallbacksAndResponseStayExactlyOnce(t *testing.T) {
	response := llm.Response{Content: "original content", ReasoningContent: "original reasoning"}
	for _, budget := range []time.Duration{0, time.Second} {
		t.Run(budget.String(), func(t *testing.T) {
			wrapped, events := newDirectBudgetWrapper(t, directTraceStubAdapter{response: response})
			var content, reasoning atomic.Int64
			got, err := llm.ChatWithRequestBudget(context.Background(), wrapped, nil, nil, llm.ChatOptions{
				OnContentDelta: func(string) { content.Add(1) }, OnReasoningDelta: func(string) { reasoning.Add(1) },
			}, budget)
			if err != nil || !reflect.DeepEqual(got, response) || content.Load() != 1 || reasoning.Load() != 1 ||
				events.requests.Load() != 1 || events.responses.Load() != 1 || events.reasoning.Load() != 1 {
				t.Fatalf("budget wrapper changed response/callback count: response=%+v content=%d reasoning=%d request=%d done=%d final_reasoning=%d err=%v", got, content.Load(), reasoning.Load(), events.requests.Load(), events.responses.Load(), events.reasoning.Load(), err)
			}
		})
	}
}

func TestDirectLLMTraceBudgetTransparentCapabilityReporting(t *testing.T) {
	for _, stream := range []bool{false, true} {
		inner := llm.NewOpenAIAdapter("test-key", "capability-only", "http://127.0.0.1", llm.AdapterOptions{
			Stream: stream, RequestTimeout: time.Second, RetryMaxAttempts: 1, StreamFirstByteTimeout: 7 * time.Second,
		})
		wrapped, _ := newDirectBudgetWrapper(t, inner)
		capability, ok := wrapped.(llm.StreamingLivenessReporter)
		if !ok || capability.StreamingLivenessWatchdogEnabled() != stream {
			t.Errorf("presentation wrapper lost stream capability: stream=%t reporter=%t", stream, ok)
		}
		if got := llm.BuildRequestTelemetry(wrapped, nil, nil).StreamFirstByteTimeout; got != 7*time.Second {
			t.Errorf("first-byte telemetry changed: %s", got)
		}
	}
	wrapped, _ := newDirectBudgetWrapper(t, directTraceStubAdapter{})
	if reporter, ok := wrapped.(llm.StreamingLivenessReporter); !ok || reporter.StreamingLivenessWatchdogEnabled() {
		t.Error("unknown inner adapter must not acquire streaming authority")
	}
}

type directBudgetIgnoringLeaf struct {
	directTraceStubAdapter
	release, returned chan struct{}
}

func (a *directBudgetIgnoringLeaf) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	defer close(a.returned)
	<-a.release // Deliberately ignores cancellation, as a third-party adapter may.
	if opts.OnContentDelta != nil {
		opts.OnContentDelta("late content")
	}
	if opts.OnReasoningDelta != nil {
		opts.OnReasoningDelta("late reasoning")
	}
	if opts.OnToolCallDelta != nil {
		opts.OnToolCallDelta(0, "emit_turn_policy", `{"route":"repo"}`)
	}
	if opts.OnRetry != nil {
		opts.OnRetry(1, 2, 0, "late retry")
	}
	if opts.OnFallback != nil {
		opts.OnFallback("first", "second", "late fallback")
	}
	return llm.Response{Content: "late result"}, nil
}

func TestDirectLLMTraceBudgetLateNonResponsiveLeafCannotReopenFinishedPreview(t *testing.T) {
	leaf := &directBudgetIgnoringLeaf{release: make(chan struct{}), returned: make(chan struct{})}
	wrapped, events := newDirectBudgetWrapper(t, leaf)
	var originalCallbacks atomic.Int64
	_, err := chatWithClassifierBudget(context.Background(), wrapped, nil, nil, llm.ChatOptions{
		OnContentDelta:   func(string) { originalCallbacks.Add(1) },
		OnReasoningDelta: func(string) { originalCallbacks.Add(1) },
		OnToolCallDelta:  func(int, string, string) { originalCallbacks.Add(1) },
		OnRetry:          func(int, int, time.Duration, string) { originalCallbacks.Add(1) },
		OnFallback:       func(string, string, string) { originalCallbacks.Add(1) },
	}, classifierCallBudget{requestTimeout: 20 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("uncooperative nonstream leaf must remain bounded: %v", err)
	}
	finishedEvents := events.all.Load()
	close(leaf.release)
	select {
	case <-leaf.returned:
	case <-time.After(time.Second):
		t.Fatal("late adapter did not finish after release")
	}
	if events.all.Load() != finishedEvents {
		t.Fatalf("finished request gained late preview/retry/fallback events: before=%d after=%d", finishedEvents, events.all.Load())
	}
	if originalCallbacks.Load() != 5 {
		t.Fatalf("presentation lifetime must not suppress original adapter callbacks: %d", originalCallbacks.Load())
	}
}
