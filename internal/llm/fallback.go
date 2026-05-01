package llm

import (
	"context"
	"fmt"
	"time"
)

// FallbackAdapter tries adapters in order until one succeeds OR a
// non-retryable error fires.
//
// Pre-2026-04-30 the loop swapped to the next adapter on ANY error,
// which wasted a full round-trip on errors that no provider rotation
// could fix:
//   - Auth / API-key wrong on primary → tried fallback (different
//     creds may help, but if BOTH have wrong creds the user paid
//     2× the latency before failing)
//   - Schema validation rejected on primary → tried fallback
//     (replays the same bad schema, guaranteed to fail again)
//
// Updated rule: only swap when the primary's error is something a
// fresh upstream attempt could plausibly recover from
// (IsRetryableDispatchError — 429/5xx/EOF/stream-stalled/network).
// Non-retryable errors return verbatim from the primary so the
// caller sees the real diagnostic immediately.
type FallbackAdapter struct {
	adapters []Adapter
}

func NewFallbackAdapter(adapters ...Adapter) *FallbackAdapter {
	return &FallbackAdapter{adapters: adapters}
}

func (f *FallbackAdapter) Chat(ctx context.Context, messages []Message, tools []ToolSchema, opts ChatOptions) (Response, error) {
	var lastErr error
	for i, a := range f.adapters {
		// Honour ctx between adapters too — a canceled outer ctx
		// must NOT keep iterating through fallback list.
		if cerr := ctx.Err(); cerr != nil {
			return Response{}, cerr
		}
		resp, err := a.Chat(ctx, messages, tools, opts)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		// Last adapter — nothing left to try.
		if i+1 >= len(f.adapters) {
			break
		}
		// Skip fallback hop only when the error is structurally
		// non-retryable (auth / schema / config bugs cascade to the
		// same outcome on every adapter). IsFallbackEligible is a
		// SUPERSET of IsRetryableDispatchError that also accepts
		// ErrAllRetriesExhausted: when primary's L1 retry budget
		// runs out on a 429 / 5xx storm, secondary still gets a
		// chance because it has its own quota / region / model
		// budget unaffected by primary's exhaustion. Without this
		// distinction the configured fallback was effectively dead
		// code under persistent rate-limit conditions — exactly the
		// scenario it was added for.
		if !IsFallbackEligible(err) {
			return Response{}, err
		}
		// Notify the renderer (or any other observer) that we're
		// about to swap providers so the dock status row can flip
		// to "切换 provider 中". Panic-recovered.
		if cb := opts.OnFallback; cb != nil {
			next := f.adapters[i+1]
			func() {
				defer func() { _ = recover() }()
				cb(a.ModelID(), next.ModelID(), "primary failed")
			}()
		}
	}
	return Response{}, fmt.Errorf("all LLM adapters failed: %w", lastErr)
}

func (f *FallbackAdapter) ModelID() string {
	if len(f.adapters) > 0 {
		return f.adapters[0].ModelID()
	}
	return "none"
}

func (f *FallbackAdapter) MaxContextTokens() int {
	if len(f.adapters) > 0 {
		return f.adapters[0].MaxContextTokens()
	}
	return 0
}

// MaxOutputTokens / RequestTimeout / RetryMaxAttempts delegate to the
// first wrapped adapter. The fallback stack is for failover, not for
// heterogeneous sizing — operators are expected to wrap adapters with
// compatible knobs, so reporting the head's values is correct.
func (f *FallbackAdapter) MaxOutputTokens() int {
	if len(f.adapters) > 0 {
		return f.adapters[0].MaxOutputTokens()
	}
	return 0
}

func (f *FallbackAdapter) RequestTimeout() time.Duration {
	if len(f.adapters) > 0 {
		return f.adapters[0].RequestTimeout()
	}
	return 0
}

func (f *FallbackAdapter) RetryMaxAttempts() int {
	if len(f.adapters) > 0 {
		return f.adapters[0].RetryMaxAttempts()
	}
	return 0
}
