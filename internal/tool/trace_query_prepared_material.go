package tool

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceQueryPreparedMaterialError(err error) error {
	if err == nil || traceQueryIsCancellation(err) {
		return err
	}
	return &tracequery.TraceInputAdmissionError{Code: tracequery.TraceInputAdmissionCodeSourceUnavailable, Reason: err.Error()}
}

func traceQueryPreparedMaterialFailure(path, view string, err error) types.ToolResult {
	if traceQueryIsCancellation(err) {
		return traceQueryCancellationResult(view, path, err)
	}
	return traceQueryInputAdmissionFailure(path, traceQueryPreparedMaterialError(err))
}

func traceQueryIsCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func traceQueryCancellationResult(view, path string, err error) types.ToolResult {
	reason := "canceled"
	if errors.Is(err, context.DeadlineExceeded) {
		reason = "deadline_exceeded"
	}
	return types.ToolResult{
		ToolName: "trace_query", Success: false,
		Summary:               fmt.Sprintf("trace_query run on %s was canceled before completion (%v); no partial results were published — narrow the time window or reduce the scope and re-run", path, err),
		TraceViewCancellation: &types.TraceViewCancellation{View: tracequery.CanonicalViewName(view), Reason: reason},
		Timestamp:             time.Now(),
	}
}
