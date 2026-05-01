package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

// TestIsStreamLevelRetryable_ExcludesHTTPStatus is the load-bearing
// guard against the duplicate-retry bug: a persistent HTTP 429 must
// NOT trigger outer-layer retry (orchestrator transient + force-
// finalize) because L1 (in-adapter HTTP loop) has its own 6-attempt
// × 62-second schedule. Pre-2026-04-30 the orchestrator used the
// broader IsRetryableDispatchError predicate, causing the user-
// visible "12-minute persistent 429 wait" cascade.
func TestIsStreamLevelRetryable_ExcludesHTTPStatus(t *testing.T) {
	apiErr := &apiError{StatusCode: 429}
	if IsStreamLevelRetryable(apiErr) {
		t.Errorf("HTTP 429 must NOT be stream-level retryable (L1 covers); got true")
	}
	if !IsRetryableDispatchError(apiErr) {
		t.Errorf("HTTP 429 still passes IsRetryableDispatchError (L1's domain); got false")
	}
	apiErr500 := &apiError{StatusCode: 503}
	if IsStreamLevelRetryable(apiErr500) {
		t.Errorf("HTTP 503 must NOT be stream-level retryable; got true")
	}
}

// TestIsStreamLevelRetryable_IncludesStreamErrors verifies the cases
// L4/L5 SHOULD retry on. These are the errors L1 doesn't cover — its
// retry loop only fires for 429/5xx, so EOF/stream-stalled/network
// blip would otherwise have zero retry coverage.
func TestIsStreamLevelRetryable_IncludesStreamErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"unexpected EOF", io.ErrUnexpectedEOF},
		{"clean EOF", io.EOF},
		{"stream stalled", ErrStreamStalled},
		{"first-byte timeout", ErrStreamFirstByteTimeout},
		{"context deadline", context.DeadlineExceeded},
		{"wrapped EOF", fmt.Errorf("read stream: %w", io.ErrUnexpectedEOF)},
	}
	for _, c := range cases {
		if !IsStreamLevelRetryable(c.err) {
			t.Errorf("%s: expected stream-level retryable; got false", c.name)
		}
	}
}

// TestIsStreamLevelRetryable_RespectsExhausted pins the L1-exhaustion
// short-circuit. Once L1 wraps its terminal error with
// ErrAllRetriesExhausted, outer layers must NOT retry — L1 already
// burned the 62-second budget on this exact error class.
func TestIsStreamLevelRetryable_RespectsExhausted(t *testing.T) {
	exhausted := fmt.Errorf("%w: %w", ErrAllRetriesExhausted, &apiError{StatusCode: 429})
	if IsStreamLevelRetryable(exhausted) {
		t.Errorf("ErrAllRetriesExhausted wrap must short-circuit retry; got true")
	}
	if IsRetryableDispatchError(exhausted) {
		t.Errorf("ErrAllRetriesExhausted wrap must short-circuit broader predicate too; got true")
	}
}

// TestFallbackAdapter_OnlySwapsOnRetryable locks the FallbackAdapter
// gate: non-retryable errors (auth / schema / config bugs) must NOT
// cascade to the fallback adapter. Cascading would waste a full
// round-trip on errors that no provider rotation can fix.
func TestFallbackAdapter_OnlySwapsOnRetryable(t *testing.T) {
	primaryAttempts := 0
	fallbackAttempts := 0
	primary := &mockAdapter{
		modelID: "primary",
		chatFn: func(ctx context.Context, msgs []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
			primaryAttempts++
			// Non-retryable: invalid api key.
			return Response{}, errors.New("401 Unauthorized: invalid api key")
		},
	}
	fb := &mockAdapter{
		modelID: "fallback",
		chatFn: func(ctx context.Context, msgs []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
			fallbackAttempts++
			return Response{}, nil
		},
	}
	adapter := NewFallbackAdapter(primary, fb)
	_, err := adapter.Chat(context.Background(), nil, nil, ChatOptions{})
	if err == nil {
		t.Fatal("expected error from non-retryable primary; got success")
	}
	if primaryAttempts != 1 {
		t.Errorf("primary should be tried once; got %d", primaryAttempts)
	}
	if fallbackAttempts != 0 {
		t.Errorf("fallback must NOT be tried for non-retryable error; got %d", fallbackAttempts)
	}
}

// TestFallbackAdapter_DoesSwapOnRetryable verifies the positive case:
// a transient EOF on primary triggers the fallback hop.
func TestFallbackAdapter_DoesSwapOnRetryable(t *testing.T) {
	primaryAttempts := 0
	fallbackAttempts := 0
	primary := &mockAdapter{
		modelID: "primary",
		chatFn: func(ctx context.Context, msgs []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
			primaryAttempts++
			return Response{}, fmt.Errorf("read stream: %w", io.ErrUnexpectedEOF)
		},
	}
	fb := &mockAdapter{
		modelID: "fallback",
		chatFn: func(ctx context.Context, msgs []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
			fallbackAttempts++
			return Response{Content: "fallback response"}, nil
		},
	}
	adapter := NewFallbackAdapter(primary, fb)
	resp, err := adapter.Chat(context.Background(), nil, nil, ChatOptions{})
	if err != nil {
		t.Fatalf("expected fallback success; got %v", err)
	}
	if resp.Content != "fallback response" {
		t.Errorf("expected fallback's response; got %q", resp.Content)
	}
	if primaryAttempts != 1 || fallbackAttempts != 1 {
		t.Errorf("expected primary=1 fallback=1; got primary=%d fallback=%d",
			primaryAttempts, fallbackAttempts)
	}
}

// TestFallbackAdapter_SwapOnL1Exhausted pins the load-bearing
// fix for the persistent-rate-limit dead-fallback bug: when primary
// returns ErrAllRetriesExhausted (its in-adapter L1 retry budget
// ran out on a 429 / 5xx storm), the FallbackAdapter MUST still
// route to the secondary because the secondary has its own quota
// budget unaffected by primary's exhaustion. Without this, the
// configured fallback was dead code precisely when it was needed.
func TestFallbackAdapter_SwapOnL1Exhausted(t *testing.T) {
	primaryAttempts := 0
	fallbackAttempts := 0
	primary := &mockAdapter{
		modelID: "primary",
		chatFn: func(ctx context.Context, msgs []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
			primaryAttempts++
			// Simulate L1 wrapping a 429 with ErrAllRetriesExhausted.
			rateErr := &apiError{StatusCode: 429, Body: `{"error":"rate_limit"}`}
			return Response{}, fmt.Errorf("%w: %w", ErrAllRetriesExhausted, rateErr)
		},
	}
	fb := &mockAdapter{
		modelID: "fallback",
		chatFn: func(ctx context.Context, msgs []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
			fallbackAttempts++
			return Response{Content: "fallback response"}, nil
		},
	}
	adapter := NewFallbackAdapter(primary, fb)
	resp, err := adapter.Chat(context.Background(), nil, nil, ChatOptions{})
	if err != nil {
		t.Fatalf("expected fallback success on L1-exhausted primary; got %v", err)
	}
	if resp.Content != "fallback response" {
		t.Errorf("expected fallback's response; got %q", resp.Content)
	}
	if primaryAttempts != 1 || fallbackAttempts != 1 {
		t.Errorf("expected primary=1 fallback=1; got primary=%d fallback=%d",
			primaryAttempts, fallbackAttempts)
	}
}

// TestFallbackAdapter_DoesNotSwapOnCancel locks the cancellation
// short-circuit: a user /cancel must NOT trigger fallback (cascading
// the cancel through every configured adapter wastes wall-clock and
// confuses the dock about "switching provider" while the user
// already wanted to stop).
func TestFallbackAdapter_DoesNotSwapOnCancel(t *testing.T) {
	primaryAttempts := 0
	fallbackAttempts := 0
	primary := &mockAdapter{
		modelID: "primary",
		chatFn: func(ctx context.Context, msgs []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
			primaryAttempts++
			return Response{}, context.Canceled
		},
	}
	fb := &mockAdapter{
		modelID: "fallback",
		chatFn: func(ctx context.Context, msgs []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
			fallbackAttempts++
			return Response{Content: "should not run"}, nil
		},
	}
	adapter := NewFallbackAdapter(primary, fb)
	_, err := adapter.Chat(context.Background(), nil, nil, ChatOptions{})
	if err == nil {
		t.Fatalf("expected error on cancelled primary; got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled in error chain; got %v", err)
	}
	if fallbackAttempts != 0 {
		t.Errorf("fallback must NOT run after cancellation; got %d attempts", fallbackAttempts)
	}
	_ = primaryAttempts
}

// mockAdapter is a minimal Adapter for testing FallbackAdapter
// behaviour without spinning up actual HTTP machinery. The chatFn
// closure carries the per-test stub.
type mockAdapter struct {
	modelID string
	chatFn  func(ctx context.Context, msgs []Message, tools []ToolSchema, opts ChatOptions) (Response, error)
}

func (m *mockAdapter) Chat(ctx context.Context, msgs []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
	return m.chatFn(ctx, msgs, tools, opts)
}
func (m *mockAdapter) ModelID() string                  { return m.modelID }
func (m *mockAdapter) MaxContextTokens() int            { return 128000 }
func (m *mockAdapter) MaxOutputTokens() int             { return 0 }
func (m *mockAdapter) RequestTimeout() time.Duration    { return 0 }
func (m *mockAdapter) RetryMaxAttempts() int            { return 1 }
