package llm

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
)

// ErrAllRetriesExhausted is the sentinel the in-adapter HTTP retry
// loop wraps onto its terminal error when `retryMaxAttempts` is
// reached. Outer layers (FallbackAdapter, scheduler transient retry,
// force-finalize) consult this to avoid duplicate retry coverage:
// once L1 has already burned through its 62-second 429/5xx budget,
// further retry at the orchestrator layer just multiplies wait time
// without improving recovery odds.
var ErrAllRetriesExhausted = errors.New("all retries exhausted by adapter")

// IsRetryableDispatchError reports whether a stage-dispatch failure is
// transient enough to merit a scheduler retry. This is broader than
// the provider's in-request retry loop: by the time an error reaches
// the orchestrator, the LLM call may already have exhausted provider-
// level retries but still represent a recoverable network / deadline /
// 429 / 5xx failure for the pipeline as a whole.
//
// Returns false when the error is wrapped with ErrAllRetriesExhausted
// — that signal explicitly means "L1 already covered this; further
// retry at outer layers is wasted wall-clock".
func IsRetryableDispatchError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrAllRetriesExhausted) {
		return false
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) && isRetryable(apiErr) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	// Stream watchdog kills are transient by definition — the watchdog
	// fired because upstream went silent past the configured threshold,
	// not because the request itself was malformed. Both stall (mid-
	// stream pause) and first-byte timeout (request accepted but server
	// never started speaking) are recoverable on a fresh attempt. The
	// underlying chain unwraps to context.Canceled (the watchdog's cancel),
	// which is NOT retryable on its own — without these explicit sentinel
	// matches, stream stalls fall through to terminal failure even though
	// they are the most common transient error with thinking-mode LLMs.
	if errors.Is(err, ErrStreamStalled) ||
		errors.Is(err, ErrStreamFirstByteTimeout) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	return false
}

// IsStreamLevelRetryable is the STRICT SUBSET of retryable errors
// that L1 (the in-adapter HTTP retry loop) does NOT cover. The
// orchestrator's transient-retry and force-finalize loops use this
// instead of IsRetryableDispatchError to avoid duplicate retry
// coverage on HTTP 429 / 5xx (already covered by L1's 6-attempt ×
// 62-second exponential schedule).
//
// Returns true on:
//   - io.EOF / io.ErrUnexpectedEOF (stream truncation)
//   - ErrStreamStalled / ErrStreamFirstByteTimeout (watchdog kills)
//   - context.DeadlineExceeded (request-level timeout)
//   - net.Error / url.Error (transport-level network blip)
//
// Returns false on:
//   - HTTP 429 / 5xx wrapped in *apiError (L1's domain — retrying at
//     L4/L5 would burn 6× more wall-clock for no net benefit since
//     L1 already exhausted its budget by the time we see this error)
//   - ErrAllRetriesExhausted (L1's exhaustion sentinel)
//   - non-retryable errors (auth / schema / config bugs)
//
// The function exists because the user-visible "12-minute persistent
// 429 wait" scenario was caused by L1 (62s) × L4 (1 retry) × L5
// (3 retries) × FallbackAdapter (2 attempts) all retrying on the
// same 429 — duplicate coverage on top of L1's already-thorough
// schedule. With this guard, only L1 retries 429/5xx; L4/L5 only
// step in for stream-level errors L1 doesn't cover.
func IsStreamLevelRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrAllRetriesExhausted) {
		return false
	}
	// HTTP status errors are L1's domain — exclude.
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, ErrStreamStalled) ||
		errors.Is(err, ErrStreamFirstByteTimeout) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	return false
}
