package orchestrator

import (
	"context"
	"errors"
	"testing"
)

// TestCancelToken_ContextCancelsOnCancel pins the Phase 2 contract:
// CancelToken.Context() returns a ctx whose Done channel closes the
// moment Cancel() returns. HTTP-level callers (LLM Adapter,
// exec.CommandContext) derive from this ctx so user-cancel is
// instant rather than cooperative.
func TestCancelToken_ContextCancelsOnCancel(t *testing.T) {
	tok := NewCancelToken()
	ctx := tok.Context()

	select {
	case <-ctx.Done():
		t.Fatalf("ctx should not be Done before Cancel")
	default:
	}

	tok.Cancel("test")

	select {
	case <-ctx.Done():
		// expected
	default:
		t.Fatalf("ctx.Done() should fire immediately after Cancel")
	}

	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Errorf("ctx.Err() = %v, want context.Canceled", ctx.Err())
	}

	if !tok.IsCanceled() {
		t.Errorf("token.IsCanceled() should be true post Cancel")
	}
	if tok.Reason() != "test" {
		t.Errorf("Reason = %q, want 'test'", tok.Reason())
	}
}

// TestCancelToken_NilContextSafe pins the nil-receiver contract:
// callers can derive from token.Context() unconditionally without
// nil-checking; idle / nil tokens return context.TODO() rather
// than panicking.
func TestCancelToken_NilContextSafe(t *testing.T) {
	var tok *CancelToken
	ctx := tok.Context()
	if ctx == nil {
		t.Fatalf("nil token Context() must return a non-nil ctx")
	}
	// context.TODO() is never canceled by itself.
	select {
	case <-ctx.Done():
		t.Errorf("nil token's ctx should not be Done")
	default:
	}
}

// TestOrchestrator_CancelContextHandlesIdleToken pins the
// between-Run nil-token path: Orchestrator.CancelContext() must
// return a usable ctx even when no Run is in flight (no token
// allocated).
func TestOrchestrator_CancelContextHandlesIdleToken(t *testing.T) {
	o := &Orchestrator{}
	ctx := o.CancelContext()
	if ctx == nil {
		t.Fatalf("CancelContext on idle orchestrator must return non-nil ctx")
	}
	select {
	case <-ctx.Done():
		t.Errorf("idle orchestrator's ctx should not be Done")
	default:
	}
}

// TestCancelToken_CancelIdempotent guards against double-Cancel
// closing an already-closed channel (would panic). Multiple
// Cancel calls remain a no-op after the first.
func TestCancelToken_CancelIdempotent(t *testing.T) {
	tok := NewCancelToken()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("double Cancel panicked: %v", r)
		}
	}()
	tok.Cancel("first")
	tok.Cancel("second")
	tok.Cancel("third")
	if tok.Reason() != "first" {
		t.Errorf("first reason should win; got %q", tok.Reason())
	}
}
