package llm

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"testing"
)

func TestIsRetryableDispatchError(t *testing.T) {
	t.Run("api retryable", func(t *testing.T) {
		err := fmt.Errorf("wrapped: %w", &apiError{StatusCode: 503})
		if !IsRetryableDispatchError(err) {
			t.Fatal("503 API errors should be retryable")
		}
	})

	t.Run("deadline", func(t *testing.T) {
		if !IsRetryableDispatchError(context.DeadlineExceeded) {
			t.Fatal("deadline errors should be retryable")
		}
	})

	t.Run("network", func(t *testing.T) {
		err := &url.Error{Err: &net.OpError{Op: "read", Net: "tcp", Err: context.DeadlineExceeded}}
		if !IsRetryableDispatchError(err) {
			t.Fatal("network transport errors should be retryable")
		}
	})

	t.Run("ordinary error", func(t *testing.T) {
		if IsRetryableDispatchError(fmt.Errorf("boom")) {
			t.Fatal("plain errors should not be classified as retryable dispatch errors")
		}
	})
}
