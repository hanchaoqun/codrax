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

// ErrStreamFirstByteTimeout is the sentinel a StreamFirstByteTimeoutError
// unwraps to. Distinct from ErrStreamStalled because the operator
// remediation differs: first-byte timeout almost always means
// "provider accepted the request but never started streaming"
// (provider-side deadlock, middlebox holding the response, or a
// model genuinely stuck before its first thinking block). Stalled
// streams typically mean a flaky upstream MID-stream — different
// retry / provider-switch decisions.
var ErrStreamFirstByteTimeout = errors.New("llm: upstream first-byte timeout")

// StreamFirstByteTimeoutError wraps the timeout error when the
// streaming watchdog detects no usable SSE data chunk after the
// configured firstByteTimeout window — distinct from the longer
// stallTimeout that catches mid-stream pauses. SSE comments, blank
// separators, malformed frames, and empty JSON keep-alives do not
// count as usable chunks because they do not carry assistant progress.
//
// Why two timeouts: thinking models routinely pause 30-60s between
// thinking blocks, so stallTimeout sits much higher. But "request
// accepted, server just won't speak" still needs a bounded pre-output
// watchdog. The default first-byte window is intentionally shorter
// than the mid-stream stall ceiling, while leaving enough cold-start
// headroom for slower OpenAI-compatible providers.
//
// IdleFor records how long the request was open with no usable SSE
// chunks before the watchdog fired. Unwrap returns the underlying ctx
// cancellation so existing matchers keep working.
type StreamFirstByteTimeoutError struct {
	IdleFor time.Duration
	Cause   error
}

func (e *StreamFirstByteTimeoutError) Error() string {
	if e == nil {
		return ErrStreamFirstByteTimeout.Error()
	}
	if e.Cause != nil {
		return fmt.Sprintf("upstream LLM produced no usable SSE data within %s of the request being accepted: %v", e.IdleFor, e.Cause)
	}
	return fmt.Sprintf("upstream LLM produced no usable SSE data within %s of the request being accepted", e.IdleFor)
}

func (e *StreamFirstByteTimeoutError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *StreamFirstByteTimeoutError) Is(target error) bool {
	return target == ErrStreamFirstByteTimeout
}

// ErrStreamNoVisibleOutputTimeout is raised when a stream keeps sending
// provider-native hidden reasoning / keepalive-like progress but never emits
// user-visible assistant content or protocol tool-call data within the
// request timeout. Transport-level watchdogs intentionally count hidden
// reasoning as bytes, but the product must still bound "the user sees
// 请求模型中 forever" hangs.
var ErrStreamNoVisibleOutputTimeout = errors.New("llm: upstream stream produced no visible output")

// StreamNoVisibleOutputTimeoutError wraps the read-side cancellation when the
// visible-output watchdog fires. It is intentionally not part of the in-adapter
// retry allowlist: retrying a model that spends the whole request budget in
// hidden reasoning can multiply wall-clock wait without improving recovery.
type StreamNoVisibleOutputTimeoutError struct {
	IdleFor time.Duration
	Cause   error
}

func (e *StreamNoVisibleOutputTimeoutError) Error() string {
	if e == nil {
		return ErrStreamNoVisibleOutputTimeout.Error()
	}
	if e.Cause != nil {
		return fmt.Sprintf("upstream LLM produced no visible assistant/tool output within %s: %v", e.IdleFor, e.Cause)
	}
	return fmt.Sprintf("upstream LLM produced no visible assistant/tool output within %s", e.IdleFor)
}

func (e *StreamNoVisibleOutputTimeoutError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *StreamNoVisibleOutputTimeoutError) Is(target error) bool {
	return target == ErrStreamNoVisibleOutputTimeout
}

// ErrStreamTotalTimeout is raised when a streaming request exceeds the total
// wall-clock cap (2× the operator's request timeout). Streaming intentionally
// carries no http.Client.Timeout so legitimate long answers are not killed
// while visible bytes flow — but that left the worst case unbounded: a
// degenerate stream that keeps emitting visible deltas resets every idle
// watchdog and can run arbitrarily long (customer dead-session 2026-07-03:
// 14m35s against timeout=10m). The total cap preserves the flowing-bytes
// headroom while bounding the whole request.
var ErrStreamTotalTimeout = errors.New("llm: upstream stream exceeded total wall-clock cap")

// StreamTotalTimeoutError wraps the read-side cancellation when the total
// wall-clock watchdog fires. Intentionally NOT in the in-adapter retry
// allowlist: content callbacks have usually fired by then, and replaying a
// request that already consumed 2× the per-call budget multiplies wall-clock
// damage instead of recovering.
type StreamTotalTimeoutError struct {
	Elapsed time.Duration
	Cap     time.Duration
	Cause   error
}

func (e *StreamTotalTimeoutError) Error() string {
	if e == nil {
		return ErrStreamTotalTimeout.Error()
	}
	if e.Cause != nil {
		return fmt.Sprintf("upstream LLM stream ran %s, exceeding the total wall-clock cap %s (2×request timeout): %v", e.Elapsed, e.Cap, e.Cause)
	}
	return fmt.Sprintf("upstream LLM stream ran %s, exceeding the total wall-clock cap %s (2×request timeout)", e.Elapsed, e.Cap)
}

func (e *StreamTotalTimeoutError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *StreamTotalTimeoutError) Is(target error) bool {
	return target == ErrStreamTotalTimeout
}
