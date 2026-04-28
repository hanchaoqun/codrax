package llm

import (
	"errors"
	"fmt"
	"time"
)

// ErrStreamStalled is the sentinel a StreamStalledError unwraps to.
// Callers that want to detect "upstream LLM streaming stalled" without
// caring about the specific idle duration use:
//
//	if errors.Is(err, llm.ErrStreamStalled) { ... }
//
// The friendlyRunError translator in internal/repl matches against
// this sentinel BEFORE the generic "context canceled" substring path
// so the user-facing message clearly attributes the abort to upstream
// stalling rather than a Ctrl+C.
var ErrStreamStalled = errors.New("llm: upstream stream stalled")

// StreamStalledError wraps the underlying read-side error (typically
// a context.Canceled propagating from httpResp.Body Read) when the
// streaming watchdog detects no upstream bytes for more than
// streamStallTimeout. The watchdog cancels the request context to
// abort the stream, so without this typed wrapper the error
// surfaces as "read stream: context canceled" — which the REPL's
// friendlyRunError treats as a Ctrl+C.
//
// IdleFor records how long upstream went silent before the watchdog
// fired so user-facing messages can render "no bytes for 30s".
//
// Unwrap support: errors.Is(err, ErrStreamStalled) and
// errors.Is(err, context.Canceled) both return true so callers
// already matching context.Canceled keep working.
type StreamStalledError struct {
	IdleFor time.Duration
	Cause   error
}

// Error renders a developer-facing message; the REPL translator
// produces the user-facing prose separately.
func (e *StreamStalledError) Error() string {
	if e == nil {
		return ErrStreamStalled.Error()
	}
	if e.Cause != nil {
		return fmt.Sprintf("upstream LLM stream stalled (no bytes for %s): %v", e.IdleFor, e.Cause)
	}
	return fmt.Sprintf("upstream LLM stream stalled (no bytes for %s)", e.IdleFor)
}

// Unwrap surfaces the underlying error so callers using
// errors.Is(err, context.Canceled) continue to match — the original
// cancellation cause is still in the chain.
func (e *StreamStalledError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is supports errors.Is(err, ErrStreamStalled) — the sentinel match.
// Combined with Unwrap()'s context.Canceled chain this gives callers
// flexible matching without forcing them to type-assert
// *StreamStalledError directly.
func (e *StreamStalledError) Is(target error) bool {
	return target == ErrStreamStalled
}
