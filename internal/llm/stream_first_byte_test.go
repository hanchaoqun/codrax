package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestStreamFirstByteTimeoutError_Sentinel pins the typed-error
// detection idiom: errors.Is(err, ErrStreamFirstByteTimeout)
// matches; the underlying ctx error stays in the chain.
func TestStreamFirstByteTimeoutError_Sentinel(t *testing.T) {
	wrapped := &StreamFirstByteTimeoutError{
		IdleFor: 20 * time.Second,
		Cause:   context.Canceled,
	}
	if !errors.Is(wrapped, ErrStreamFirstByteTimeout) {
		t.Errorf("errors.Is(wrapped, ErrStreamFirstByteTimeout) should be true")
	}
	if !errors.Is(wrapped, context.Canceled) {
		t.Errorf("errors.Is(wrapped, context.Canceled) should still match via Unwrap")
	}
	// Distinct from StreamStalled.
	if errors.Is(wrapped, ErrStreamStalled) {
		t.Errorf("first-byte error must NOT match the stalled sentinel")
	}
}

// TestStreamFirstByteTimeoutError_NilSafe guards the nil receiver.
func TestStreamFirstByteTimeoutError_NilSafe(t *testing.T) {
	var e *StreamFirstByteTimeoutError
	if e.Error() != ErrStreamFirstByteTimeout.Error() {
		t.Errorf("nil receiver Error() should fall back to sentinel; got %q", e.Error())
	}
	if u := e.Unwrap(); u != nil {
		t.Errorf("nil receiver Unwrap() should be nil; got %v", u)
	}
}

func TestStreamNoVisibleOutputTimeoutError_Sentinel(t *testing.T) {
	wrapped := &StreamNoVisibleOutputTimeoutError{
		IdleFor: 2 * time.Minute,
		Cause:   context.Canceled,
	}
	if !errors.Is(wrapped, ErrStreamNoVisibleOutputTimeout) {
		t.Errorf("errors.Is(wrapped, ErrStreamNoVisibleOutputTimeout) should be true")
	}
	if !errors.Is(wrapped, context.Canceled) {
		t.Errorf("errors.Is(wrapped, context.Canceled) should still match via Unwrap")
	}
	if errors.Is(wrapped, ErrStreamFirstByteTimeout) || errors.Is(wrapped, ErrStreamStalled) {
		t.Errorf("no-visible-output timeout must be distinct from first-byte and stall sentinels")
	}
}

// TestDoStreamRequest_FirstByteTimeoutFires covers the watchdog
// path: server returns 200 OK but never sends a body chunk; the
// watchdog cancels and Chat returns a typed
// StreamFirstByteTimeoutError. Uses a short firstByteTimeout (1s)
// so the test runs fast.
func TestDoStreamRequest_FirstByteTimeoutFires(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reply 200 OK but flush no body. Hold open for 5s to ensure
		// the watchdog fires before our deadline.
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush() // headers out, body empty
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         10 * time.Second,
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: 800 * time.Millisecond,
		StreamStallTimeout:     5 * time.Second,
	})

	start := time.Now()
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected first-byte timeout error; got nil")
	}
	if !errors.Is(err, ErrStreamFirstByteTimeout) {
		t.Errorf("expected ErrStreamFirstByteTimeout in chain; got %v", err)
	}
	// Should fire close to firstByteTimeout (800ms) — generous slack
	// for tick interval (200ms) + scheduling.
	if elapsed > 2*time.Second {
		t.Errorf("first-byte timeout took %v; expected <2s", elapsed)
	}
	if !strings.Contains(err.Error(), "no usable SSE data") {
		t.Errorf("error message should name 'no usable SSE data'; got %q", err.Error())
	}
}

func TestDoStreamRequest_ActiveHiddenReasoningMayOutlastRequestTimeoutAndFinish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		for i := 0; i < 25; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(50 * time.Millisecond):
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"}}]}\n\n"))
				if f != nil {
					f.Flush()
				}
			}
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if f != nil {
			f.Flush()
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         500 * time.Millisecond, // old absolute cap = 1s
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: time.Second,
		StreamStallTimeout:     5 * time.Second,
	})

	start := time.Now()
	resp, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("active hidden reasoning must not be mistaken for absent output: %v", err)
	}
	if resp.Content != "answer" {
		t.Fatalf("late visible answer = %q, want answer", resp.Content)
	}
	if elapsed <= 500*time.Millisecond {
		t.Fatalf("test did not outlast requestTimeout; elapsed=%v", elapsed)
	}
	if elapsed <= time.Second {
		t.Fatalf("test did not outlast the old absolute cap; elapsed=%v", elapsed)
	}
}

func TestDoStreamRequest_ActiveToolCallMayOutlastTotalCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		chunks := []string{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"emit_answer_document","arguments":"{\"blocks\":"}}]}}]}` + "\n\n",
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"[]}"}}]}}]}` + "\n\n",
			`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n",
			"data: [DONE]\n\n",
		}
		for _, chunk := range chunks {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(300 * time.Millisecond):
			}
			_, _ = w.Write([]byte(chunk))
			if f != nil {
				f.Flush()
			}
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         400 * time.Millisecond, // absolute cap = 800ms
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: time.Second,
		StreamStallTimeout:     5 * time.Second,
	})

	start := time.Now()
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	if err != nil {
		t.Fatalf("active tool-call progress must outlive the old absolute total cap: %v", err)
	}
	if elapsed := time.Since(start); elapsed <= time.Second {
		t.Fatalf("test did not reach the old watchdog firing tick; elapsed=%v", elapsed)
	}
}

func TestDoStreamRequest_FirstByteTimeoutFiresBeforeHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate OpenAI-compatible local servers that do not flush
		// response headers until the first token is ready. Before this
		// path was covered by ResponseHeaderTimeout, the SSE watchdog
		// never started because http.Client.Do had not returned yet.
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         10 * time.Second,
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: 500 * time.Millisecond,
		StreamStallTimeout:     5 * time.Second,
	})

	start := time.Now()
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected pre-header first-byte timeout error; got nil")
	}
	if !errors.Is(err, ErrStreamFirstByteTimeout) {
		t.Fatalf("expected ErrStreamFirstByteTimeout in chain; got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("pre-header first-byte timeout took %v; expected <2s", elapsed)
	}
	if !strings.Contains(err.Error(), "no usable SSE data") {
		t.Errorf("error message should name 'no usable SSE data'; got %q", err.Error())
	}
}

type blockingPreHeaderRoundTripper struct{}

func (blockingPreHeaderRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	<-r.Context().Done()
	return nil, r.Context().Err()
}

func TestDoStreamRequest_FirstByteTimeoutCancelsPreHeaderDo(t *testing.T) {
	adapter := NewOpenAIAdapter("k", "m", "http://example.test", AdapterOptions{
		Stream:                 true,
		RequestTimeout:         10 * time.Second,
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: 250 * time.Millisecond,
		StreamStallTimeout:     5 * time.Second,
	})
	// Exercise the adapter-owned pre-header watchdog, not the net/http
	// transport's ResponseHeaderTimeout. Some OpenAI-compatible providers
	// and intermediaries can stall before response headers in ways that are
	// easier to reason about when the request context itself is canceled.
	adapter.streamHTTPClient = &http.Client{Transport: blockingPreHeaderRoundTripper{}}

	start := time.Now()
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected pre-header first-byte timeout error; got nil")
	}
	if !errors.Is(err, ErrStreamFirstByteTimeout) {
		t.Fatalf("expected ErrStreamFirstByteTimeout in chain; got %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("pre-header request context watchdog took %v; expected <1s", elapsed)
	}
}

// EVOLUTION RECORD (2026-07-15, STREAM-WAIT §29.92): this test used to
// be TestDoStreamRequest_FirstByteTimeoutIgnoresKeepAliveFrames and
// pinned the OPPOSITE behaviour — keep-alive frames did not reset the
// first-byte watchdog, so a server heartbeating past the window was
// killed. That rule starved reasoning-model gateways that heartbeat
// while holding all assistant output until thinking completes
// (customer witness MiniMax-M2.7). The pinned contract is now: any
// received bytes reset the byte-liveness clock ("the server is
// breathing"), so a stream that heartbeats past firstByteTimeout and
// THEN delivers real data must succeed. The empty-stream verdict
// (gotAnyChunk) still counts only parseable data chunks — covered by
// the StreamEmptyError matrix tests. A heartbeat-active stream is not
// terminated from elapsed age alone; the caller's explicit cancellation is
// pinned by TestDoStreamRequest_KeepAliveOnlyStreamOutlivesOldTotalCapUntilCallerCancel.
func TestDoStreamRequest_KeepAlivesResetFirstByteWatchdog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		// Heartbeat every 100ms for 1.2s — well past the 500ms
		// first-byte window — then deliver the real answer.
		for i := 0; i < 12; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			_, _ = w.Write([]byte(": keep-alive\n\n"))
			_, _ = w.Write([]byte("data: {}\n\n"))
			if f != nil {
				f.Flush()
			}
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if f != nil {
			f.Flush()
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         10 * time.Second,
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: 500 * time.Millisecond,
		StreamStallTimeout:     5 * time.Second,
	})

	resp, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	if err != nil {
		t.Fatalf("keep-alive bytes must reset the first-byte watchdog; request failed: %v", err)
	}
	if resp.Content != "hi" {
		t.Fatalf("expected the post-heartbeat answer to arrive intact, got %q", resp.Content)
	}
}

// A live stream can produce partial frame bytes before the first newline. The
// byte watchdog owns transport silence only: 4ms-spaced raw bytes must not
// authorize a degraded answer merely because no complete SSE line is ready.
// This pins the Reader boundary, not just complete SSE heartbeat lines.
func TestDoStreamRequest_PartialFrameBytesOutliveFourMillisecondThreshold(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		// Establish usable stream content first, then hold the next answer
		// inside a partial frame. This isolates the mid-stream byte-liveness
		// contract from HTTP response-header scheduling.
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"))
		if f != nil {
			f.Flush()
		}
		prefix := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"")
		for _, b := range prefix {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(4 * time.Millisecond):
			}
			_, _ = w.Write([]byte{b})
			if f != nil {
				f.Flush()
			}
		}
		_, _ = w.Write([]byte("ok\"}}]}\n\ndata: [DONE]\n\n"))
		if f != nil {
			f.Flush()
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         2 * time.Second,
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: 100 * time.Millisecond,
		StreamStallTimeout:     25 * time.Millisecond,
	})
	resp, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	if err != nil {
		t.Fatalf("byte-producing partial SSE frame must not degrade at 4ms: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("partial SSE answer = %q, want ok", resp.Content)
	}
}

// TestDoStreamRequest_StallAfterFirstByteUsesStallTimeout pins the
// two-threshold gate: once the first chunk arrives, the watchdog
// switches from firstByteTimeout to stallTimeout. Test sends one
// chunk then goes silent; first-byte (1s) should NOT fire because
// chunk arrived; stall (3s) eventually fires.
func TestDoStreamRequest_StallAfterFirstByteUsesStallTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		// One real chunk then go silent. Watchdog should track
		// firstByte=true and switch to stallTimeout (3s).
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		if f != nil {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(10 * time.Second):
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         15 * time.Second,
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: 1 * time.Second,
		StreamStallTimeout:     3 * time.Second,
	})

	start := time.Now()
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected stalled-stream error; got nil")
	}
	// Critical: must NOT be the first-byte error — first chunk arrived.
	if errors.Is(err, ErrStreamFirstByteTimeout) {
		t.Errorf("first-byte fired even though chunk arrived; should be StreamStalled. err=%v", err)
	}
	if !errors.Is(err, ErrStreamStalled) {
		t.Errorf("expected ErrStreamStalled; got %v", err)
	}
	// Stall fires at ~3s + tick slack; should be >2s, <5s.
	if elapsed < 2*time.Second {
		t.Errorf("stall fired too early (%v); first-byte may have wrongly tripped", elapsed)
	}
	if elapsed > 6*time.Second {
		t.Errorf("stall took %v; expected <6s", elapsed)
	}
}

// EVOLUTION RECORD (2026-07-15, STREAM-WAIT §29.92): formerly
// TestDoStreamRequest_StallTimeoutIgnoresKeepAliveFrames, pinning the
// opposite contract. Same reversal as the first-byte twin above: a
// mid-stream heartbeat proves the connection and the server are alive,
// so a stream that pauses its content past stallTimeout while
// heartbeating and then finishes must succeed. There is deliberately no
// total wall-clock cap for a heartbeat-active stream; the following test
// pins that only caller cancellation/deadline may end the heartbeat-only
// shape. This test finishes normally after the pause.
func TestDoStreamRequest_KeepAlivesResetStallWatchdog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		if f != nil {
			f.Flush()
		}
		// Heartbeat every 100ms for 1.5s — past the 800ms stall
		// window — then finish the answer.
		for i := 0; i < 15; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			_, _ = w.Write([]byte(": keep-alive\n\n"))
			_, _ = w.Write([]byte("data: {}\n\n"))
			if f != nil {
				f.Flush()
			}
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" there\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if f != nil {
			f.Flush()
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         10 * time.Second,
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: 500 * time.Millisecond,
		StreamStallTimeout:     800 * time.Millisecond,
	})

	resp, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	if err != nil {
		t.Fatalf("keep-alive bytes must reset the stall watchdog; request failed: %v", err)
	}
	if resp.Content != "hi there" {
		t.Fatalf("expected the paused answer to complete intact, got %q", resp.Content)
	}
}

// TestDoStreamRequest_KeepAliveOnlyStreamOutlivesOldTotalCapUntilCallerCancel
// pins the ownership boundary: a reasoning gateway may expose only heartbeat
// bytes while holding its hidden reasoning and final answer. Elapsed age alone
// must not cancel that active connection or authorize a degraded answer; the
// caller's explicit context cancellation remains authoritative.
func TestDoStreamRequest_KeepAliveOnlyStreamOutlivesOldTotalCapUntilCallerCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				_, _ = w.Write([]byte(": keep-alive\n\n"))
				if f != nil {
					f.Flush()
				}
			}
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         400 * time.Millisecond, // total cap = 800ms
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: 10 * time.Second,
		StreamStallTimeout:     10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := adapter.Chat(ctx, []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("heartbeat-active stream must end only from caller deadline, got %v", err)
	}
	if errors.Is(err, ErrStreamTotalTimeout) {
		t.Fatalf("elapsed age must not mint the legacy total-timeout signal: %v", err)
	}
	if elapsed <= 800*time.Millisecond {
		t.Fatalf("stream did not outlive the old 2×request-timeout cap; elapsed=%v", elapsed)
	}
}

// TestStreamFirstByteTimeoutError_ErrorIncludesIdle pins the
// developer-facing error message. Idle duration must be visible
// for log-only debugging.
func TestStreamFirstByteTimeoutError_ErrorIncludesIdle(t *testing.T) {
	e := &StreamFirstByteTimeoutError{IdleFor: 21 * time.Second, Cause: context.Canceled}
	got := e.Error()
	if !strings.Contains(got, "21s") {
		t.Errorf("Error() should include idle duration 21s; got %q", got)
	}
	if !strings.Contains(got, "no usable SSE data") {
		t.Errorf("Error() should mention 'no usable SSE data'; got %q", got)
	}
}
