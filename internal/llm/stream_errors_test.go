package llm

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// TestStreamStalledError_Sentinel pins the errors.Is(err,
// ErrStreamStalled) match — the canonical detection idiom callers
// use to identify watchdog-aborted streams without type-asserting
// the wrapper directly.
func TestStreamStalledError_Sentinel(t *testing.T) {
	wrapped := &StreamStalledError{
		IdleFor: 30 * time.Second,
		Cause:   context.Canceled,
	}
	if !errors.Is(wrapped, ErrStreamStalled) {
		t.Errorf("errors.Is(wrapped, ErrStreamStalled) should be true")
	}
}

// TestStreamStalledError_PreservesCancelChain ensures the underlying
// context.Canceled cause is reachable via Unwrap so existing callers
// that match context.Canceled keep working — the typed wrapper is
// strictly additive.
func TestStreamStalledError_PreservesCancelChain(t *testing.T) {
	wrapped := &StreamStalledError{
		IdleFor: 45 * time.Second,
		Cause:   context.Canceled,
	}
	if !errors.Is(wrapped, context.Canceled) {
		t.Errorf("errors.Is(wrapped, context.Canceled) should still be true via Unwrap")
	}
}

// TestStreamStalledError_NestedThroughFmtErrorf covers the realistic
// production path: parseSSEStream returns `fmt.Errorf("read stream:
// %w", err)` which doStreamRequest then wraps as StreamStalledError.
// The double-wrap must not break errors.Is detection.
func TestStreamStalledError_NestedThroughFmtErrorf(t *testing.T) {
	inner := fmt.Errorf("read stream: %w", context.Canceled)
	outer := &StreamStalledError{
		IdleFor: 30 * time.Second,
		Cause:   inner,
	}
	if !errors.Is(outer, ErrStreamStalled) {
		t.Errorf("typed sentinel should match through fmt.Errorf %%w wrap")
	}
	if !errors.Is(outer, context.Canceled) {
		t.Errorf("inner context.Canceled should still match through wrap")
	}
}

// TestStreamStalledError_ErrorIncludesIdleDuration covers the
// developer-facing message — the duration must appear so a log-only
// reader can tell HOW LONG upstream was silent.
func TestStreamStalledError_ErrorIncludesIdleDuration(t *testing.T) {
	e := &StreamStalledError{IdleFor: 31 * time.Second, Cause: context.Canceled}
	got := e.Error()
	if !contains(got, "31s") {
		t.Errorf("Error() should include idle duration 31s; got %q", got)
	}
	if !contains(got, "stalled") {
		t.Errorf("Error() should mention 'stalled'; got %q", got)
	}
}

// TestStreamStalledError_NilSafe guards a nil receiver from panicking
// — Error() and Unwrap() should both degrade gracefully.
func TestStreamStalledError_NilSafe(t *testing.T) {
	var e *StreamStalledError
	if e.Error() != ErrStreamStalled.Error() {
		t.Errorf("nil receiver Error() should return sentinel; got %q", e.Error())
	}
	if u := e.Unwrap(); u != nil {
		t.Errorf("nil receiver Unwrap() should be nil; got %v", u)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
