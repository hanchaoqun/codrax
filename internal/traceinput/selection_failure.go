package traceinput

import (
	"context"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// SelectionFailureKind describes a correctable coordinate, never a readable
// source or permission to skip full preparation on the next attempt.
type SelectionFailureKind uint8

const (
	SelectionFailureNone SelectionFailureKind = iota
	SelectionFailureRequired
	SelectionFailureInvalid
	SelectionFailureRequiresZIP
)

// ClassifySelectionFailure accepts only one producer verdict through ordinary
// wrapping. Do not use errors.As here: selection plus failed close, rollback,
// source-generation validation or cancellation is not a selection-only error.
// The diagnostic cause may be plain text, but its contents confer no authority.
func ClassifySelectionFailure(err error) SelectionFailureKind {
	kind := SelectionFailureNone
	for depth := 0; depth < 32; depth++ {
		if err == nil {
			return kind
		}
		if err == context.Canceled || err == context.DeadlineExceeded {
			return SelectionFailureNone
		}
		current := SelectionFailureNone
		switch failure := err.(type) {
		case *Error:
			if failure == nil {
				return SelectionFailureNone
			}
			switch failure.Code {
			case "archive_member_invalid":
				current = SelectionFailureInvalid
			case "archive_member_requires_zip":
				current = SelectionFailureRequiresZIP
			case "conversion_failed": // the preparation transport wrapper
			default:
				return SelectionFailureNone
			}
		case *hitraceconv.TraceArchiveError:
			if failure == nil {
				return SelectionFailureNone
			}
			switch failure.Code {
			case "multiple_trace_members":
				current = SelectionFailureRequired
			case "explicit_trace_member_invalid":
				current = SelectionFailureInvalid
			default:
				return SelectionFailureNone
			}
		case *tracequery.TraceInputAdmissionError:
			return SelectionFailureNone
		}
		if current != SelectionFailureNone {
			if kind != SelectionFailureNone {
				return SelectionFailureNone
			}
			kind = current
		}
		switch cause := err.(type) {
		case interface{ Unwrap() []error }:
			var single error
			for _, child := range cause.Unwrap() {
				if child == nil {
					continue
				}
				if single != nil {
					return SelectionFailureNone
				}
				single = child
			}
			err = single
		case interface{ Unwrap() error }:
			err = cause.Unwrap()
		default:
			return kind
		}
	}
	return SelectionFailureNone
}
