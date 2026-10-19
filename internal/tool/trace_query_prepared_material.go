package tool

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hanchaoqun/codrax/internal/traceinput"
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
	if selection := traceinput.ClassifySelectionFailure(err); selection != traceinput.SelectionFailureNone {
		code := "trace_archive_selection_invalid"
		reason := "the archive_member coordinate is invalid"
		hint := "Retry trace_query with source=path, the original ZIP path and its intended exact existing archive_member. Do not guess a member."
		if selection == traceinput.SelectionFailureRequired {
			code = "trace_archive_selection_required"
			reason = "multiple_trace_members: select the intended exact archive_member"
		} else if selection == traceinput.SelectionFailureRequiresZIP {
			reason = "archive_member_requires_zip: the selected input is not a ZIP"
			hint = "This input is not a ZIP. Retry trace_query on the same input without archive_member."
		}
		// Producer diagnostics remain at the preparation boundary. In particular,
		// do not repeat the converter CLI's --archive-member instruction in the
		// ordinary JSON tool feedback or derive teaching from error text.
		summary := fmt.Sprintf("trace_query selection for %q is incomplete or invalid: %s; no query evidence was published. %s", path, reason, hint)
		return types.ToolResult{ToolName: "trace_query", Summary: summary, Timestamp: time.Now(), Repair: &types.ToolRepair{
			Code: code, Hint: summary, Fields: []string{"source", "path", "archive_member"},
			Metadata: map[string]string{"stage": types.ToolRepairStageTraceSourceSelection, "status": types.ToolRepairStatusActionRecommended, "path": path},
		}}
	}
	result := traceQueryInputAdmissionFailure(path, traceQueryPreparedMaterialError(err))
	// The low-level text admission error says conversion was not attempted.
	// That is not a fact at this upper boundary: conversion may have completed
	// before the source/derived generation or publication check failed.
	reason := err.Error()
	var admission *tracequery.TraceInputAdmissionError
	if errors.As(err, &admission) {
		reason = admission.Reason
	}
	result.Summary = fmt.Sprintf("trace_query could not prepare or validate the selected trace %q: %s; no query evidence was published. Retry with a stable supported capture or its readable export.", path, reason)
	result.Repair.Hint = result.Summary
	return result
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
