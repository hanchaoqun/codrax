package repl

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
)

// The observations come from the real adapter's reader callback, not merely
// from a server successfully writing bytes or from an open TCP connection.
type routeStreamBudgetObserver struct {
	llm.Adapter
	bytes      atomic.Int64
	reads      atomic.Int64
	reasoning  atomic.Int64
	lastReadNS atomic.Int64
}

func (o *routeStreamBudgetObserver) observe(opts llm.ChatOptions) llm.ChatOptions {
	previous := opts.OnStreamActivity
	opts.OnStreamActivity = func(event llm.StreamActivity) {
		if event.Kind == llm.StreamActivityTransportBytes && event.Bytes > 0 {
			o.bytes.Add(int64(event.Bytes))
			o.reads.Add(1)
			o.lastReadNS.Store(time.Now().UnixNano())
		}
		if event.Kind == llm.StreamActivityReasoning {
			o.reasoning.Add(1)
		}
		if previous != nil {
			previous(event)
		}
	}
	return opts
}

func (o *routeStreamBudgetObserver) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	return o.Adapter.Chat(ctx, messages, tools, o.observe(opts))
}

func (o *routeStreamBudgetObserver) ChatWithRequestBudget(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions, timeout time.Duration) (llm.Response, error) {
	return llm.ChatWithRequestBudget(ctx, o.Adapter, messages, tools, o.observe(opts), timeout)
}

func newRouteBudgetActiveSSE(t *testing.T, legacy bool) *routeStreamBudgetObserver {
	t.Helper()
	name, args := turnPolicyTool.Name, singleShotDataPolicyJSON
	if legacy {
		name, args = chitchatClassifierTool.Name, `{"decision":"chitchat","reason":"a casual conversation"}`
	}
	chunk, err := json.Marshal(map[string]any{"choices": []any{map[string]any{
		"delta": map[string]any{"tool_calls": []any{map[string]any{
			"index": 0, "id": "route-call", "type": "function",
			"function": map[string]any{"name": name, "arguments": args},
		}}},
		"finish_reason": "tool_calls",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		for i := 0; i < 40; i++ {
			_, _ = w.Write([]byte(": keepalive\n\ndata: {\"choices\":[{\"delta\":{\"reasoning_content\":\"working\"}}]}\n\n"))
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(3 * time.Millisecond):
			}
		}
		_, _ = w.Write(append(append([]byte("data: "), chunk...), []byte("\n\ndata: [DONE]\n\n")...))
		flusher.Flush()
	}))
	t.Cleanup(server.Close)
	return &routeStreamBudgetObserver{Adapter: llm.NewOpenAIAdapter("test-key", "route-stream", server.URL, llm.AdapterOptions{
		Stream: true, RequestTimeout: 4 * time.Millisecond, RetryMaxAttempts: 1,
		StreamFirstByteTimeout: time.Second, StreamStallTimeout: time.Second,
	})}
}

func TestRouteClassifierDefaultBudgetDoesNotCutActiveSSE(t *testing.T) {
	routeClassifierTestDefaults(t, 30*time.Millisecond)
	for _, lane := range []string{"repl", "cli", "repl_in_flight", "legacy", "legacy_in_flight"} {
		t.Run(lane, func(t *testing.T) {
			legacy := lane == "legacy" || lane == "legacy_in_flight"
			observed := newRouteBudgetActiveSSE(t, legacy)
			classifier := &llmChitchatClassifier{adapter: observed}
			r := &REPL{chitchatClassifier: classifier}
			started := time.Now()
			var policy TurnPolicy
			var isChat bool
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
				isChat, err = classifier.Classify(context.Background(), "hello", "")
			case "legacy_in_flight":
				isChat, err = r.runBoolClassifierInFlight(func(ctx context.Context) (bool, error) {
					return classifier.Classify(ctx, "hello", "")
				})
			}
			if err != nil {
				t.Fatalf("default routing budget cut a real active stream: bytes=%d reads=%d reasoning=%d last_read_age=%s elapsed=%s err=%v", observed.bytes.Load(), observed.reads.Load(), observed.reasoning.Load(), time.Since(time.Unix(0, observed.lastReadNS.Load())), time.Since(started), err)
			}
			if (legacy && !isChat) || (!legacy && policy.Route != RouteData) || observed.bytes.Load() == 0 || observed.reads.Load() < 8 || observed.reasoning.Load() != 40 || time.Since(started) < 100*time.Millisecond {
				t.Fatalf("original model-authored route must arrive without degradation: policy=%+v isChat=%t bytes=%d reads=%d reasoning=%d elapsed=%s", policy, isChat, observed.bytes.Load(), observed.reads.Load(), observed.reasoning.Load(), time.Since(started))
			}
		})
	}
}

func routeClassifierTestDefaults(t *testing.T, timeout time.Duration) {
	t.Helper()
	oldREPL, oldCLI := turnPolicyClassifierTimeout, singleShotRoutePolicyTimeout
	oldREPLExplicit, oldCLIExplicit := turnPolicyClassifierTimeoutExplicit, singleShotRoutePolicyTimeoutExplicit
	turnPolicyClassifierTimeout, singleShotRoutePolicyTimeout = timeout, timeout
	turnPolicyClassifierTimeoutExplicit, singleShotRoutePolicyTimeoutExplicit = false, false
	t.Cleanup(func() {
		turnPolicyClassifierTimeout, singleShotRoutePolicyTimeout = oldREPL, oldCLI
		turnPolicyClassifierTimeoutExplicit, singleShotRoutePolicyTimeoutExplicit = oldREPLExplicit, oldCLIExplicit
	})
}

func TestRouteClassifierExplicitBudgetStillCutsActiveSSE(t *testing.T) {
	for _, cli := range []bool{false, true} {
		t.Run(map[bool]string{false: "repl", true: "cli"}[cli], func(t *testing.T) {
			routeClassifierTestDefaults(t, 30*time.Millisecond)
			// Equal-valued explicit configuration must not be mistaken for an
			// omitted default: presence, not a numeric heuristic, owns policy.
			if cli {
				SetSingleShotRoutePolicyTimeout(30 * time.Millisecond)
			} else {
				SetTurnPolicyClassifierTimeout(30 * time.Millisecond)
			}
			observed := newRouteBudgetActiveSSE(t, false)
			classifier := &llmChitchatClassifier{adapter: observed}
			var err error
			if cli {
				_, err = classifier.ClassifyPolicySingleShot(context.Background(), "inspect the supplied artifact", "", false)
			} else {
				r := &REPL{chitchatClassifier: classifier}
				_, err = r.runTurnPolicyClassifierInFlight(func(ctx context.Context) (TurnPolicy, error) {
					return classifier.ClassifyPolicy(ctx, "inspect the supplied artifact", "", false)
				})
			}
			if !errors.Is(err, context.DeadlineExceeded) || observed.reads.Load() < 2 || observed.reasoning.Load() >= 40 {
				t.Fatalf("explicit total budget remains binding even with active bytes: reads=%d reasoning=%d err=%v", observed.reads.Load(), observed.reasoning.Load(), err)
			}
		})
	}
}

func TestRouteClassifierExplicitZeroAndInvalidTimeoutProvenance(t *testing.T) {
	routeClassifierTestDefaults(t, 30*time.Millisecond)
	SetTurnPolicyClassifierTimeout(0)
	SetSingleShotRoutePolicyTimeout(-time.Second)
	if turnPolicyClassifierTimeoutExplicit || singleShotRoutePolicyTimeoutExplicit {
		t.Fatal("ignored values must not convert omitted defaults into explicit total limits")
	}
	SetSingleShotRoutePolicyTimeout(0)
	if !singleShotRoutePolicyTimeoutExplicit || singleShotRoutePolicyTimeout != 0 {
		t.Fatal("explicit zero must remain a meaningful disabled CLI total budget")
	}
	observed := newRouteBudgetActiveSSE(t, false)
	policy, err := (&llmChitchatClassifier{adapter: observed}).ClassifyPolicySingleShot(context.Background(), "inspect the supplied artifact", "", false)
	if err != nil || policy.Route != RouteData || observed.reasoning.Load() != 40 {
		t.Fatalf("explicit zero must not inherit the shorter REPL budget: policy=%+v reasoning=%d err=%v", policy, observed.reasoning.Load(), err)
	}
}

func TestRouteClassifierDefaultBudgetFollowsActualFallbackLeg(t *testing.T) {
	for _, streamFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonstream_to_stream", true: "stream_with_nonstream_backup"}[streamFirst], func(t *testing.T) {
			routeClassifierTestDefaults(t, 30*time.Millisecond)
			observed := newRouteBudgetActiveSSE(t, false)
			var adapter llm.Adapter
			if streamFirst {
				adapter = llm.NewFallbackAdapter(observed, blockingTurnPolicyAdapter{})
			} else {
				adapter = llm.NewFallbackAdapter(blockingTurnPolicyAdapter{}, observed)
			}
			adapter = llm.NewTelemetryAdapter(llm.NewFallbackAdapter(adapter), func(llm.RequestTelemetry) {})
			classifier := &llmChitchatClassifier{adapter: adapter}
			r := &REPL{chitchatClassifier: classifier}
			policy, err := r.runTurnPolicyClassifierInFlight(func(ctx context.Context) (TurnPolicy, error) {
				return classifier.ClassifyPolicy(ctx, "inspect the supplied artifact", "", false)
			})
			if err != nil || policy.Route != RouteData || observed.reasoning.Load() != 40 {
				t.Fatalf("nested mixed stack must give budgets only to entered non-streaming legs: policy=%+v reasoning=%d err=%v", policy, observed.reasoning.Load(), err)
			}
		})
	}
}

func TestRouteClassifierCallerCancellationAndEarlierDeadlineRemainBinding(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			routeClassifierTestDefaults(t, time.Second)
			observed := newRouteBudgetActiveSSE(t, false)
			parent, cancel := context.WithCancel(context.Background())
			if deadline {
				cancel()
				parent, cancel = context.WithTimeout(context.Background(), 30*time.Millisecond)
			}
			defer cancel()
			if !deadline {
				stop := time.AfterFunc(30*time.Millisecond, cancel)
				defer stop.Stop()
			}
			_, err := (&llmChitchatClassifier{adapter: observed}).ClassifyPolicySingleShot(parent, "inspect the supplied artifact", "", false)
			want := context.Canceled
			if deadline {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) || observed.reads.Load() < 2 || observed.reasoning.Load() >= 40 {
				t.Fatalf("caller lifetime must win despite active bytes: reads=%d reasoning=%d err=%v", observed.reads.Load(), observed.reasoning.Load(), err)
			}
		})
	}
}

func TestRouteClassifierDefaultBudgetPreservesTrueByteSilenceTimeouts(t *testing.T) {
	for _, firstChunk := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_first_chunk", true: "stalled_after_reasoning"}[firstChunk], func(t *testing.T) {
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
			_, err := (&llmChitchatClassifier{adapter: observed}).ClassifyPolicy(context.Background(), "inspect the supplied artifact", "", false)
			want := llm.ErrStreamFirstByteTimeout
			if firstChunk {
				want = llm.ErrStreamStalled
			}
			if !errors.Is(err, want) || errors.Is(err, context.DeadlineExceeded) || (firstChunk && observed.reasoning.Load() != 1) || (!firstChunk && observed.bytes.Load() != 0) {
				t.Fatalf("idle classification must stay distinct from an overall-age deadline: bytes=%d reasoning=%d err=%v", observed.bytes.Load(), observed.reasoning.Load(), err)
			}
		})
	}
}

type routeBudgetRepairAdapter struct {
	blockingTurnPolicyAdapter
	calls          atomic.Int64
	firstDeadline  atomic.Int64
	secondDeadline atomic.Int64
}

func (a *routeBudgetRepairAdapter) Chat(ctx context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	deadline, _ := ctx.Deadline()
	if a.calls.Add(1) == 1 {
		a.firstDeadline.Store(deadline.UnixNano())
		select {
		case <-ctx.Done():
			return llm.Response{}, ctx.Err()
		case <-time.After(30 * time.Millisecond):
			return turnPolicyResp(`{"route":"data","requires_diagram":"not a boolean"}`), nil
		}
	}
	a.secondDeadline.Store(deadline.UnixNano())
	<-ctx.Done()
	return llm.Response{}, ctx.Err()
}

func TestRouteClassifierExplicitTotalBudgetSpansStructuralRepair(t *testing.T) {
	for _, cli := range []bool{false, true} {
		t.Run(map[bool]string{false: "repl", true: "cli"}[cli], func(t *testing.T) {
			routeClassifierTestDefaults(t, time.Second)
			if cli {
				SetSingleShotRoutePolicyTimeout(70 * time.Millisecond)
			} else {
				SetTurnPolicyClassifierTimeout(70 * time.Millisecond)
			}
			adapter := &routeBudgetRepairAdapter{}
			classifier := &llmChitchatClassifier{adapter: adapter}
			var err error
			if cli {
				_, err = classifier.ClassifyPolicySingleShot(context.Background(), "inspect the supplied artifact", "", false)
			} else {
				_, err = classifier.ClassifyPolicy(context.Background(), "inspect the supplied artifact", "", false)
			}
			if !errors.Is(err, context.DeadlineExceeded) || adapter.calls.Load() != 2 || adapter.firstDeadline.Load() == 0 || adapter.firstDeadline.Load() != adapter.secondDeadline.Load() {
				t.Fatalf("explicit budget must span repair, not restart per request: calls=%d first=%d repair=%d err=%v", adapter.calls.Load(), adapter.firstDeadline.Load(), adapter.secondDeadline.Load(), err)
			}
		})
	}
}
