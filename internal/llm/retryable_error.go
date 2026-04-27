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
