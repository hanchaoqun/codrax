package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type requestBudgetProbe struct {
	stream      bool
	calls       int
	deadline    time.Time
	hasDeadline bool
	run         func(context.Context, ChatOptions) (Response, error)
}

func (p *requestBudgetProbe) Chat(ctx context.Context, _ []Message, _ []ToolSchema, opts ChatOptions) (Response, error) {
	p.calls++
	p.deadline, p.hasDeadline = ctx.Deadline()
	if p.run != nil {
		return p.run(ctx, opts)
	}
	return Response{Content: "complete"}, nil
}
func (*requestBudgetProbe) ModelID() string                          { return "budget-probe" }
func (*requestBudgetProbe) MaxContextTokens() int                    { return 128000 }
func (*requestBudgetProbe) MaxOutputTokens() int                     { return 4096 }
func (*requestBudgetProbe) RequestTimeout() time.Duration            { return time.Second }
func (*requestBudgetProbe) RetryMaxAttempts() int                    { return 1 }
func (p *requestBudgetProbe) StreamingLivenessWatchdogEnabled() bool { return p.stream }

func waitRequestBudget(ctx context.Context, _ ChatOptions) (Response, error) {
	<-ctx.Done()
	return Response{}, ctx.Err()
}

func TestRequestBudgetNonStreamingPrimaryTimeoutDoesNotExpireStreamingFallback(t *testing.T) {
	primary := &requestBudgetProbe{run: waitRequestBudget}
	secondary := &requestBudgetProbe{stream: true, run: func(ctx context.Context, _ ChatOptions) (Response, error) {
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return Response{Content: "stream survived"}, nil
		}
	}}
	resp, err := ChatWithRequestBudget(context.Background(), NewFallbackAdapter(primary, secondary), nil, nil, ChatOptions{}, 20*time.Millisecond)
	if err != nil || resp.Content != "stream survived" {
		t.Fatalf("expired non-streaming leg must not poison the streaming fallback context: resp=%+v err=%v", resp, err)
	}
	if !primary.hasDeadline || secondary.hasDeadline || primary.calls != 1 || secondary.calls != 1 {
		t.Fatalf("budget was not applied to the active legs: primary=%+v secondary=%+v", primary, secondary)
	}
}

func TestRequestBudgetEveryNonStreamingFallbackLegReceivesFreshBudget(t *testing.T) {
	primary := &requestBudgetProbe{run: waitRequestBudget}
	secondary := &requestBudgetProbe{run: waitRequestBudget}
	start := time.Now()
	_, err := ChatWithRequestBudget(context.Background(), NewFallbackAdapter(primary, secondary), nil, nil, ChatOptions{}, 20*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || primary.calls != 1 || secondary.calls != 1 {
		t.Fatalf("each entered non-streaming leg must run under its own budget: primary=%+v secondary=%+v err=%v", primary, secondary, err)
	}
	if !primary.hasDeadline || !secondary.hasDeadline || secondary.deadline.Sub(primary.deadline) < 15*time.Millisecond || time.Since(start) < 35*time.Millisecond {
		t.Fatalf("a whole-stack deadline must not replace per-leg budgets: primary=%+v secondary=%+v elapsed=%s", primary, secondary, time.Since(start))
	}
}

func TestRequestBudgetStreamingFailureThenNonStreamingFallbackGetsOwnBudget(t *testing.T) {
	primary := &requestBudgetProbe{stream: true, run: func(ctx context.Context, _ ChatOptions) (Response, error) {
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return Response{}, io.ErrUnexpectedEOF
		}
	}}
	var entered time.Time
	secondary := &requestBudgetProbe{run: func(ctx context.Context, opts ChatOptions) (Response, error) {
		entered = time.Now()
		return waitRequestBudget(ctx, opts)
	}}
	telemetryCalls := 0
	observe := func(RequestTelemetry) { telemetryCalls++ }
	adapter := NewTelemetryAdapter(NewFallbackAdapter(
		NewTelemetryAdapter(primary, observe),
		NewFallbackAdapter(NewTelemetryAdapter(secondary, observe)),
	), observe)
	start := time.Now()
	_, err := ChatWithRequestBudget(context.Background(), adapter, nil, nil, ChatOptions{}, 30*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("active non-streaming fallback must remain bounded: %v", err)
	}
	if primary.hasDeadline || !secondary.hasDeadline || secondary.calls != 1 || telemetryCalls != 3 {
		t.Fatalf("nested wrappers lost active-leg budget or telemetry: primary=%+v secondary=%+v telemetry=%d", primary, secondary, telemetryCalls)
	}
	if delta := secondary.deadline.Sub(entered); delta < 15*time.Millisecond || delta > 35*time.Millisecond {
		t.Fatalf("fallback budget must begin when that leg starts, not before the streaming attempt: remaining=%s", delta)
	}
	if time.Since(start) < 65*time.Millisecond {
		t.Fatal("the original whole-chain budget was incorrectly retained")
	}
}

func TestRequestBudgetCallerCancellationStopsActiveStreamAndFallback(t *testing.T) {
	entered := make(chan struct{})
	primary := &requestBudgetProbe{stream: true, run: func(ctx context.Context, opts ChatOptions) (Response, error) {
		close(entered)
		return waitRequestBudget(ctx, opts)
	}}
	secondary := &requestBudgetProbe{}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := ChatWithRequestBudget(parent, NewFallbackAdapter(primary, secondary), nil, nil, ChatOptions{}, 4*time.Millisecond)
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation must pass through unchanged: %v", err)
	}
	if primary.hasDeadline || secondary.calls != 0 {
		t.Fatalf("caller cancel must not become a budget deadline or enter fallback: primary=%+v secondary=%+v", primary, secondary)
	}
}

func TestRequestBudgetPreservesShorterCallerDeadlineForEveryActiveLeg(t *testing.T) {
	for _, stream := range []bool{false, true} {
		parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		wantDeadline, _ := parent.Deadline()
		primary := &requestBudgetProbe{stream: stream, run: waitRequestBudget}
		secondary := &requestBudgetProbe{}
		_, err := ChatWithRequestBudget(parent, NewFallbackAdapter(primary, secondary), nil, nil, ChatOptions{}, time.Second)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) || !primary.deadline.Equal(wantDeadline) || secondary.calls != 0 {
			t.Fatalf("stream=%t caller deadline must win and stop fallback: primary=%+v secondary=%+v err=%v", stream, primary, secondary, err)
		}
	}
}

func TestRequestBudgetZeroDoesNotAddDeadline(t *testing.T) {
	primary := &requestBudgetProbe{}
	if _, err := ChatWithRequestBudget(context.Background(), NewTelemetryAdapter(NewFallbackAdapter(primary), func(RequestTelemetry) {}), nil, nil, ChatOptions{}, 0); err != nil || primary.hasDeadline {
		t.Fatalf("zero evaluator budget must retain ordinary adapter behavior: primary=%+v err=%v", primary, err)
	}
}

func TestRequestBudgetPreservesNonStreamingAdapterHTTPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer server.Close()
	adapter := NewOpenAIAdapter("test-key", "non-stream", server.URL, AdapterOptions{
		Stream: false, RequestTimeout: 25 * time.Millisecond, RetryMaxAttempts: 1,
	})
	start := time.Now()
	_, err := ChatWithRequestBudget(context.Background(), NewFallbackAdapter(adapter), nil, nil, ChatOptions{}, time.Second)
	if err == nil || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("the adapter's shorter own HTTP timeout must remain effective: elapsed=%s err=%v", time.Since(start), err)
	}
}

func TestRequestBudgetDoesNotWaiveTrueStreamingByteSilence(t *testing.T) {
	for _, firstChunk := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if firstChunk {
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"}}]}\n\n"))
			}
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}))
		adapter := NewOpenAIAdapter("test-key", "stream", server.URL, AdapterOptions{
			Stream: true, RequestTimeout: 4 * time.Millisecond, RetryMaxAttempts: 1,
			StreamFirstByteTimeout: 60 * time.Millisecond, StreamStallTimeout: 60 * time.Millisecond,
		})
		_, err := ChatWithRequestBudget(context.Background(), NewTelemetryAdapter(NewFallbackAdapter(adapter), func(RequestTelemetry) {}), nil, nil, ChatOptions{}, 4*time.Millisecond)
		server.Close()
		want := ErrStreamFirstByteTimeout
		if firstChunk {
			want = ErrStreamStalled
		}
		if !errors.Is(err, want) || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("firstChunk=%t must retain precise byte-silence failure, not an evaluator age deadline: %v", firstChunk, err)
		}
	}
}
