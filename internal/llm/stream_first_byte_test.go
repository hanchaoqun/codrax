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

func TestDoStreamRequest_HiddenReasoningOnlyHitsVisibleOutputTimeout(t *testing.T) {
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
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"}}]}\n\n"))
				if f != nil {
					f.Flush()
				}
			}
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         350 * time.Millisecond,
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: time.Second,
		StreamStallTimeout:     5 * time.Second,
	})

	start := time.Now()
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected no-visible-output timeout error; got nil")
	}
	if !errors.Is(err, ErrStreamNoVisibleOutputTimeout) {
		t.Fatalf("expected ErrStreamNoVisibleOutputTimeout in chain; got %v", err)
	}
	if errors.Is(err, ErrStreamFirstByteTimeout) || errors.Is(err, ErrStreamStalled) {
		t.Fatalf("hidden reasoning should trip visible-output timeout, not first-byte/stall: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("visible-output timeout took %v; expected <2s", elapsed)
	}
}

func TestDoStreamRequest_ToolCallProgressAvoidsVisibleOutputTimeout(t *testing.T) {
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
			case <-time.After(120 * time.Millisecond):
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
		RequestTimeout:         250 * time.Millisecond,
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: time.Second,
		StreamStallTimeout:     5 * time.Second,
	})

	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	if err != nil {
		t.Fatalf("tool-call deltas are visible protocol progress and should not timeout: %v", err)
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

func TestDoStreamRequest_FirstByteTimeoutIgnoresKeepAliveFrames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				_, _ = w.Write([]byte(": keep-alive\n\n"))
				_, _ = w.Write([]byte("data: {}\n\n"))
				if f != nil {
					f.Flush()
				}
			}
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
		t.Fatalf("expected first-byte timeout despite keep-alives")
	}
	if !errors.Is(err, ErrStreamFirstByteTimeout) {
		t.Fatalf("expected ErrStreamFirstByteTimeout; got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("keep-alives must not keep first-byte watchdog alive; elapsed=%v", elapsed)
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

func TestDoStreamRequest_StallTimeoutIgnoresKeepAliveFrames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		if f != nil {
			f.Flush()
		}
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				_, _ = w.Write([]byte(": keep-alive\n\n"))
				_, _ = w.Write([]byte("data: {}\n\n"))
				if f != nil {
					f.Flush()
				}
			}
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

	start := time.Now()
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected stalled-stream error despite keep-alives")
	}
	if errors.Is(err, ErrStreamFirstByteTimeout) {
		t.Fatalf("first-byte should not fire after a real chunk; got %v", err)
	}
	if !errors.Is(err, ErrStreamStalled) {
		t.Fatalf("expected ErrStreamStalled; got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("keep-alives must not keep stall watchdog alive; elapsed=%v", elapsed)
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
