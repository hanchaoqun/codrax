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
// streaming watchdog detects no upstream BYTES after the configured
// firstByteTimeout window while still waiting for the first usable
// SSE data chunk — distinct from the stallTimeout that catches
// mid-stream pauses.
//
// EVOLUTION (2026-07-15, STREAM-WAIT §29.92): liveness vs content are
// now separate ledgers. ANY received bytes — SSE comments, blank
// separators, malformed frames, empty JSON keep-alives — reset the
// watchdog clock, because a server that heartbeats is breathing, not
// refusing to speak. But none of those count as a usable chunk: only
// a parseable data chunk with assistant progress flips first-byte /
// empty-stream state. Pre-evolution the watchdog ignored keep-alive
// bytes entirely, which killed reasoning-model gateways that heartbeat
// while holding all assistant output until thinking completes. A
// heartbeat-active stream now remains alive until real byte silence,
// transport failure, or caller cancellation; elapsed age alone is not a
// failure signal.
//
// Why two timeouts: thinking models routinely pause 30-60s between
// thinking blocks, so stallTimeout sits much higher. A byte-silent request is
// bounded by first-byte/stall watchdogs; a heartbeat-active request is not
// terminated from elapsed age alone because some reasoning gateways expose no
// model delta until the final answer. The caller still owns explicit
// cancellation/deadlines.
//
// IdleFor records how long the request went with no upstream bytes
// before the watchdog fired. Unwrap returns the underlying ctx
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

// ErrStreamNoVisibleOutputTimeout is retained for compatibility with adapters
// that may already return this typed error. The OpenAI SSE adapter no longer
// mints it from an active hidden-reasoning stream: requestTimeout is not a
// visible-output idle deadline, and elapsed age alone is not precise authority
// to terminate an active stream. Byte-silence watchdogs, transport failure,
// caller cancellation/deadlines, safety, and decode failures remain the
// bounded exit signals.
var ErrStreamNoVisibleOutputTimeout = errors.New("llm: upstream stream produced no visible output")

// StreamNoVisibleOutputTimeoutError wraps the read-side cancellation when the
// legacy/provider-specific visible-output watchdog fires. It is intentionally
// not part of the in-adapter retry allowlist: retrying a model that spends the
// whole request budget in hidden reasoning can multiply wall-clock wait without
// improving recovery. Orchestrator also must not convert this legacy signal
// into a system-authored answer: the signal proves neither stream inactivity
// nor permission to replace the model's conclusion.
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

// ErrStreamTotalTimeout is retained for compatibility with adapters that may
// already return this typed error. The OpenAI SSE adapter no longer mints it:
// elapsed age, including a heartbeat-only interval, is not precise authority to
// stop an active reasoning gateway or publish a degraded system answer.
var ErrStreamTotalTimeout = errors.New("llm: upstream stream exceeded total wall-clock cap")

// StreamTotalTimeoutError wraps the read-side cancellation when the
// transport-only wall-clock watchdog fires. Intentionally NOT in the
// in-adapter retry allowlist: the provider kept a connection alive for 2× the
// per-call budget without real model progress, so immediately replaying the
// same request is more likely to multiply wall-clock damage than recover.
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
		return fmt.Sprintf("upstream LLM stream had no model progress for %s, exceeding the transport-only wall-clock cap %s (2×request timeout): %v", e.Elapsed, e.Cap, e.Cause)
	}
	return fmt.Sprintf("upstream LLM stream had no model progress for %s, exceeding the transport-only wall-clock cap %s (2×request timeout)", e.Elapsed, e.Cap)
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

// ErrStreamEmpty is the sentinel a StreamEmptyError unwraps to. It is
// raised when the provider accepted the streaming request (HTTP 200 on
// this path — non-200 responses become *apiError before the SSE parser
// runs) and then ended the response body without EVER producing a
// usable data chunk. Retrying is provably safe: by definition zero
// content/tool-call callbacks fired, so a fresh attempt cannot
// duplicate user-visible state — the same argument that puts
// ErrStreamFirstByteTimeout on the in-adapter retry allowlist.
var ErrStreamEmpty = errors.New("llm: upstream returned an empty stream")

// StreamEmptyError is the diagnosis-rich replacement for the former
// bare "empty stream — provider closed the connection before any
// chunk" string (STREAM-WAIT §29.92, customer witness 2026-07-15:
// the bare sentence left the operator with zero provider evidence to
// escalate — no status, no request id, no body). Three sub-shapes,
// each with its own wording via Error():
//
//   - immediate EOF: 200 accepted, body closed with zero lines seen —
//     classic silent refusal / load-shed by the gateway;
//   - comment-only: the server sent SSE keep-alive comments / blank
//     separators / empty JSON frames, then closed without any data
//     chunk — it was breathing but never spoke;
//   - non-SSE payload: the 200 body was not an SSE stream at all
//     (typically a provider error JSON on a misrouted request);
//     BodyPrefix carries a ≤512-byte, credential-scrubbed prefix.
//
// Credential red line: RequestID comes from a fixed allowlist of
// request-id-shaped response headers (never Authorization / api-key
// material), and BodyPrefix is scrubbed of the configured api_key by
// the adapter before the error escapes — error text and logs must
// never contain credentials.
type StreamEmptyError struct {
	// StatusCode is the HTTP status of the accepted response. On the
	// SSE parse path this is always 200 (non-200 short-circuits into
	// *apiError earlier); the field exists so the wording can state it
	// explicitly and so future non-200 wrap points stay honest.
	StatusCode int
	// RequestID is "header-name=value" for the first present header of
	// the request-id allowlist (x-request-id / anthropic-request-id /
	// cf-ray / ...), or "" when the provider sent none.
	RequestID string
	// CommentOnly reports that the stream carried SSE comment /
	// keep-alive / blank-separator bytes but no data chunk before EOF.
	CommentOnly bool
	// BodyPrefix is a ≤512-byte credential-scrubbed prefix of non-SSE
	// payload lines (error JSON served on a 200). Empty when the body
	// was empty or contained only SSE-framing bytes.
	BodyPrefix string
}

// Error renders the developer-facing message. All three shapes keep
// the historical "empty stream" prefix (operator muscle memory + log
// scrapers) and carry the phrase "upstream LLM" so string-flattened
// copies still match the orchestrator's transient-failure probe after
// an ErrAllRetriesExhausted wrap.
func (e *StreamEmptyError) Error() string {
	if e == nil {
		return ErrStreamEmpty.Error()
	}
	evidence := fmt.Sprintf("HTTP %d", e.StatusCode)
	if e.StatusCode == 0 {
		evidence = "HTTP status unknown"
	}
	if e.RequestID != "" {
		evidence += ", " + e.RequestID
	}
	switch {
	case e.BodyPrefix != "":
		return fmt.Sprintf("empty stream — upstream LLM answered %s with a non-SSE payload instead of stream data: %s", evidence, e.BodyPrefix)
	case e.CommentOnly:
		return fmt.Sprintf("empty stream — upstream LLM sent only SSE keep-alive/comment bytes and closed without any data chunk (%s)", evidence)
	default:
		return fmt.Sprintf("empty stream — provider closed the connection before any chunk (upstream LLM sent no data after %s)", evidence)
	}
}

func (e *StreamEmptyError) Is(target error) bool {
	return target == ErrStreamEmpty
}
