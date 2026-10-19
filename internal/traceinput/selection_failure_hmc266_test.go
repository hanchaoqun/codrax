package traceinput

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type hmc266ErrorCycle struct{}

func (*hmc266ErrorCycle) Error() string   { return "cycle" }
func (e *hmc266ErrorCycle) Unwrap() error { return e }

func TestHMC266SelectionFailureRequiresSingleTypedProducer(t *testing.T) {
	selection := &hitraceconv.TraceArchiveError{Code: "multiple_trace_members", Cause: errors.New("diagnostic only")}
	invalid := &hitraceconv.TraceArchiveError{Code: "explicit_trace_member_invalid", Cause: errors.New("absent or unsupported selected candidate")}
	physical := &hitraceconv.TraceArchiveError{Code: "trace_member_integrity_failed", Cause: errors.New("bad CRC")}
	for _, test := range []struct {
		name string
		err  error
		want SelectionFailureKind
	}{
		{"required", selection, SelectionFailureRequired},
		{"wrong_selected_member", invalid, SelectionFailureInvalid},
		{"bad_selector_syntax", &Error{Code: "archive_member_invalid", Err: errors.New("not canonical")}, SelectionFailureInvalid},
		{"non_zip", &Error{Code: "archive_member_requires_zip", Err: errors.New("wrong container")}, SelectionFailureRequiresZIP},
		{"conversion_single_join", fmt.Errorf("wrapper: %w", errors.Join(&Error{Code: "conversion_failed", Err: errors.Join(selection)})), SelectionFailureRequired},
		{"plain_same_text", errors.New(selection.Error()), SelectionFailureNone},
		{"different_case_code", &hitraceconv.TraceArchiveError{Code: "MULTIPLE_TRACE_MEMBERS"}, SelectionFailureNone},
		{"nil", nil, SelectionFailureNone},
		{"nil_typed", (*hitraceconv.TraceArchiveError)(nil), SelectionFailureNone},
		{"no_candidates", &hitraceconv.TraceArchiveError{Code: "trace_member_not_found"}, SelectionFailureNone},
		{"crc", physical, SelectionFailureNone},
		{"namespace", &hitraceconv.TraceArchiveError{Code: "invalid_archive_member"}, SelectionFailureNone},
		{"security", &hitraceconv.TraceArchiveError{Code: "encrypted_member_unsupported"}, SelectionFailureNone},
		{"joined_fatal", errors.Join(selection, physical), SelectionFailureNone},
		{"joined_close", errors.Join(selection, errors.New("close failed")), SelectionFailureNone},
		{"joined_cancel", errors.Join(selection, context.Canceled), SelectionFailureNone},
		{"joined_deadline", errors.Join(selection, context.DeadlineExceeded), SelectionFailureNone},
		{"nested_fatal", &hitraceconv.TraceArchiveError{Code: "multiple_trace_members", Cause: errors.Join(errors.New("two choices"), physical)}, SelectionFailureNone},
		{"outer_physical", &hitraceconv.TraceArchiveError{Code: "invalid_zip", Cause: selection}, SelectionFailureNone},
		{"outer_generation", &Error{Code: "source_changed", Err: selection}, SelectionFailureNone},
		{"outer_admission", &tracequery.TraceInputAdmissionError{Code: tracequery.TraceInputAdmissionCodeSourceUnavailable, Reason: selection.Error()}, SelectionFailureNone},
		{"nested_verdict", &hitraceconv.TraceArchiveError{Code: "multiple_trace_members", Cause: invalid}, SelectionFailureNone},
		{"cycle", &hmc266ErrorCycle{}, SelectionFailureNone},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ClassifySelectionFailure(test.err); got != test.want {
				t.Fatalf("got %v want %v for %v", got, test.want, test.err)
			}
		})
	}
}
