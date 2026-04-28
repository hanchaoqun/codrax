package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
)

// CancelToken is the cooperative cancellation handle the orchestrator
// hands out for the duration of one Run(). REPL's Ctrl+C / `/cancel`
// path calls Cancel(reason); pipeline machinery (dispatchStage, agent
// loops, tool executor) polls IsCanceled() at well-known checkpoints
// and unwinds with a CanceledError when set.
//
// Cooperative — not preemptive. Cancellation lands at the next
// checkpoint, not mid-syscall. The longest single in-flight operation
// today is an LLM Chat call (typically 5-30 s). Phase 2 will plumb
// context.Context into the LLM adapter so HTTP requests cancel
// immediately; Phase 1 (this commit) keeps the change scope minimal
// and accepts the worst-case "wait until current Chat returns" delay.
//
// Thread-safety: atomic.Bool + atomic.Pointer give lock-free reads
// from any goroutine. Cancel() is idempotent; multiple Cancel() calls
// keep the FIRST recorded reason so tests / logs see a stable value.
//
// Phase 2 (this commit): tokens carry a context.Context so HTTP-level
// callers (LLM Adapter, exec subprocess via context) get immediate
// cancellation via the standard ctx.Done() channel — Phase 1's
// cooperative checkpoint pattern (poll IsCanceled() at iteration
// boundaries) is preserved as a complement for non-ctx-aware code
// paths and as a fast-path inside agent loops.
type CancelToken struct {
	flag   atomic.Bool
	reason atomic.Pointer[string]
	ctx    context.Context
	cancel context.CancelFunc
}

// NewCancelToken returns an active token. The orchestrator allocates
// one per Run; idle tokens (between Runs) are nil.
func NewCancelToken() *CancelToken {
	ctx, cancel := context.WithCancel(context.Background())
	return &CancelToken{ctx: ctx, cancel: cancel}
}

// Cancel marks the token as canceled. Safe to call from any goroutine,
// safe to call multiple times. The first non-empty reason wins so
// "Ctrl+C" cannot be overwritten by a later "/cancel".
//
// Cancel also propagates to the embedded context so any caller
// holding a derived ctx (HTTP request, exec.CommandContext, etc.)
// observes ctx.Done() the moment Cancel returns — no cooperative
// polling needed for ctx-aware paths.
func (t *CancelToken) Cancel(reason string) {
	if t == nil {
		return
	}
	if reason == "" {
		reason = "user interrupt"
	}
	// CompareAndSwap on the pointer — first writer wins.
	t.reason.CompareAndSwap(nil, &reason)
	t.flag.Store(true)
	if t.cancel != nil {
		t.cancel()
	}
}

// Context returns the cancellation-aware context backing this token.
// HTTP requests, subprocess.CommandContext, and any other
// ctx.Done()-aware code path should derive from this ctx so Cancel()
// produces immediate (HTTP socket close / SIGKILL) interruption
// rather than waiting for the next cooperative checkpoint. Callers
// MUST nil-check: idle tokens (between Runs) return context.TODO()
// so a derive-then-store pattern still works.
func (t *CancelToken) Context() context.Context {
	if t == nil || t.ctx == nil {
		return context.TODO()
	}
	return t.ctx
}

// IsCanceled reports whether Cancel has been called on this token.
// Lock-free, suitable for hot-path checks (every agent iteration,
// every tool dispatch, every stage boundary).
func (t *CancelToken) IsCanceled() bool {
	return t != nil && t.flag.Load()
}

// Reason returns the user-facing reason recorded by the first Cancel
// call. Empty string when not canceled.
func (t *CancelToken) Reason() string {
	if t == nil {
		return ""
	}
	r := t.reason.Load()
	if r == nil {
		return ""
	}
	return *r
}

// CanceledError is the typed error pipeline checkpoints return when
// the cancel token fires. errors.Is(err, ErrCanceled) and
// errors.Is(err, context.Canceled) both succeed so callers can match
// either form. Stage / Iter context are populated by the checkpoint
// that detected the cancel — the REPL renders them in the user-facing
// "✗ canceled" line so operators can see how far the run got.
type CanceledError struct {
	Reason  string
	AtStage string
	Iter    int
}

// ErrCanceled is the sentinel CanceledError unwraps to. Tests should
// use errors.Is(err, orchestrator.ErrCanceled) rather than type-assert
// CanceledError, since the wrapper shape may grow new fields.
var ErrCanceled = errors.New("orchestrator: run canceled")

// Stage returns the most-specific pipeline stage label observed at
// the cancel checkpoint. The REPL renders this in its user-facing
// "✗ canceled (interrupted at stage=X)" line via a small interface
// it defines so the messages package does not depend on this one.
func (e *CanceledError) Stage() string {
	if e == nil {
		return ""
	}
	return e.AtStage
}

func (e *CanceledError) Error() string {
	if e == nil {
		return ErrCanceled.Error()
	}
	if e.AtStage != "" && e.Iter > 0 {
		return "canceled at stage=" + e.AtStage + " iter=" + itoa(e.Iter) + ": " + e.Reason
	}
	if e.AtStage != "" {
		return "canceled at stage=" + e.AtStage + ": " + e.Reason
	}
	return "canceled: " + e.Reason
}

// Is supports errors.Is(err, ErrCanceled) — the sentinel match path.
// We do NOT match context.Canceled here because that would silently
// swallow legitimate context-deadline-exceeded errors in HTTP / exec
// layers; callers that need the unified-cancellation semantics can
// chain errors.Is(err, ErrCanceled) || errors.Is(err, context.Canceled)
// at their boundary.
func (e *CanceledError) Is(target error) bool {
	return target == ErrCanceled
}

// itoa is the local strconv-free shim — keeps cancel.go from pulling
// strconv just for an error string.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
