package llm

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"
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

	t.Run("stream stalled", func(t *testing.T) {
		err := &StreamStalledError{IdleFor: 61 * time.Second, Cause: context.Canceled}
		if !IsRetryableDispatchError(err) {
			t.Fatal("stream-stall watchdog kills should be retryable")
		}
		wrapped := fmt.Errorf("dispatch: %w", err)
		if !IsRetryableDispatchError(wrapped) {
			t.Fatal("wrapped stream-stall should still be retryable")
		}
	})

	t.Run("stream first-byte timeout", func(t *testing.T) {
		err := &StreamFirstByteTimeoutError{IdleFor: 21 * time.Second, Cause: context.Canceled}
		if !IsRetryableDispatchError(err) {
			t.Fatal("stream first-byte timeout should be retryable")
		}
	})

	t.Run("stream no-visible-output timeout stays terminal", func(t *testing.T) {
		err := &StreamNoVisibleOutputTimeoutError{IdleFor: 4 * time.Minute, Cause: context.Canceled}
		if IsRetryableDispatchError(err) {
			t.Fatal("hidden-reasoning/no-visible-output timeout should not be multiplied by retry layers")
		}
		if IsStreamLevelRetryable(err) {
			t.Fatal("hidden-reasoning/no-visible-output timeout should not trigger stream-level retries")
		}
	})

	t.Run("plain context.Canceled stays terminal", func(t *testing.T) {
		// User-initiated Ctrl+C must not be classified as retryable —
		// only the typed stream errors above are.
		if IsRetryableDispatchError(context.Canceled) {
			t.Fatal("plain context.Canceled (user cancel) should NOT be retryable")
		}
	})
}
