package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestOpenAIAdapter_ChatNilContextRejected pins the nil-ctx fail-loud
// contract: callers must pass at least context.Background(); silent
// fallback would mask cancel-plumbing mistakes.
func TestOpenAIAdapter_ChatNilContextRejected(t *testing.T) {
	adapter := NewOpenAIAdapter("k", "m", "http://localhost", AdapterOptions{
		RequestTimeout:   1 * time.Second,
		RetryMaxAttempts: 1,
	})
	_, err := adapter.Chat(nil, []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	if err == nil {
		t.Fatalf("nil ctx should error fail-loud")
	}
}

// TestOpenAIAdapter_ChatPreCanceledCtxExitsImmediately covers the
// retry-loop ctx check: a ctx that is ALREADY canceled at Chat
// entry must NOT issue any HTTP request and return the ctx error
// promptly.
func TestOpenAIAdapter_ChatPreCanceledCtxExitsImmediately(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		RequestTimeout:   1 * time.Second,
		RetryMaxAttempts: 6,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	start := time.Now()
	_, err := adapter.Chat(ctx, []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	elapsed := time.Since(start)

	if err == nil || !errors.Is(err, context.Canceled) {
		t.Errorf("expected ctx.Canceled error; got %v", err)
	}
	if hits != 0 {
		t.Errorf("pre-canceled ctx should not issue any HTTP request; got %d hits", hits)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("pre-canceled ctx should exit instantly; took %v", elapsed)
	}
}

// TestOpenAIAdapter_ChatMidFlightCancelInterrupts covers the
// Phase 2 mid-flight cancel: ctx canceled WHILE the HTTP request
// is in flight closes the connection immediately rather than
// waiting for the request's natural completion.
func TestOpenAIAdapter_ChatMidFlightCancelInterrupts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response — would take 2 seconds normally; mid-flight
		// cancel should cut WAY below that.
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[]}`))
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		RequestTimeout:   10 * time.Second,
		RetryMaxAttempts: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := adapter.Chat(ctx, []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Errorf("expected error from mid-flight cancel; got nil")
	}
	// Mid-flight cancel must unwind well before the slow server's
	// 5s natural completion. Allow 1s slack for goroutine
	// scheduling.
	if elapsed >= 2*time.Second {
		t.Errorf("mid-flight cancel took %v; should be < 2s", elapsed)
	}
}
