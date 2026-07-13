package hitraceconv

import (
	"context"
	"errors"
	"math"
)

// profilerCanonicalLineValidContext is the single cancellable pre-publication
// authority for a typed ProfilerPluginData event. Source-local canonical
// failures remain a false verdict. Cancellation/deadline, custom Context
// invariants, and non-endpoint internal failures remain fail-loud errors;
// source-shaped formatter invariants are localized to the false canonical
// verdict because all typed/internal checks have already run upstream.
func profilerCanonicalLineValidContext(ctx context.Context, event profilerFtraceEventRecord, name, body string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if event.TSNS > math.MaxInt64 {
		return false, nil
	}
	task := firstNonEmpty(event.Comm, "unknown")
	_, err := prepareTraceDBRenderedRowWithTraceFlagsContext(
		ctx, int64(event.TSNS), 0, task, event.PID, event.TGID, event.CPU,
		event.CommonFlags, event.CommonPreemptCount, name+": "+body,
	)
	if err == nil {
		return true, nil
	}
	// A custom Context may surface a fail-loud invariant rather than one of the
	// two standard cancellation sentinels. It still outranks source-local
	// formatter rejection and must retain its exact identity.
	if contextErr := ctx.Err(); contextErr != nil {
		return false, contextErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false, err
	}
	if _, invariant := traceDBOutputInvariantReason(err); invariant {
		// The endpoint's source-shaped envelope/body rejections are deliberately
		// localized to this event. Internal typed-walker invariants never enter
		// through this formatter and are propagated by their callers beforehand.
		return false, nil
	}
	return false, err
}
