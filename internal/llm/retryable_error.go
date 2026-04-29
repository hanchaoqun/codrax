package llm

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
)

// IsRetryableDispatchError reports whether a stage-dispatch failure is
// transient enough to merit a scheduler retry. This is broader than
// the provider's in-request retry loop: by the time an error reaches
// the orchestrator, the LLM call may already have exhausted provider-
// level retries but still represent a recoverable network / deadline /
// 429 / 5xx failure for the pipeline as a whole.
func IsRetryableDispatchError(err error) bool {
	if err == nil {
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
