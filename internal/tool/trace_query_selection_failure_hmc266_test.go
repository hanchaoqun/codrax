package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC266PreparedSelectionRepairDoesNotArmPhysicalLatch(t *testing.T) {
	pending := &hitraceconv.TraceArchiveError{Code: "multiple_trace_members", Cause: errors.New("select one with --archive-member")}
	for _, test := range []struct {
		name                 string
		err                  error
		selection, cancelled bool
	}{
		{"required", pending, true, false},
		{"single_wrap", fmt.Errorf("prepare: %w", errors.Join(&traceinput.Error{Code: "conversion_failed", Err: pending})), true, false},
		{"wrong_member", &hitraceconv.TraceArchiveError{Code: "explicit_trace_member_invalid"}, true, false},
		{"bad_selector", &traceinput.Error{Code: "archive_member_invalid"}, true, false},
		{"non_zip", &traceinput.Error{Code: "archive_member_requires_zip"}, true, false},
		{"plain_error_text", errors.New(pending.Error()), false, false},
		{"no_candidates", &hitraceconv.TraceArchiveError{Code: "trace_member_not_found"}, false, false},
		{"crc", &hitraceconv.TraceArchiveError{Code: "trace_member_integrity_failed"}, false, false},
		{"namespace", &hitraceconv.TraceArchiveError{Code: "invalid_archive_member"}, false, false},
		{"generation", errors.New("source changed"), false, false},
		{"joined_fatal", errors.Join(pending, errors.New("close failed")), false, false},
		{"cancel", errors.Join(pending, context.Canceled), false, true},
		{"deadline", errors.Join(pending, context.DeadlineExceeded), false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := traceQueryPreparedMaterialFailure("original.zip", "event_search", test.err)
			if result.Success || result.RawRef != "" || len(result.Observations) != 0 || result.TraceQuerySourceRead.Path() != "" {
				t.Fatal("failure acquired evidence", result)
			}
			mutable := types.NewMutableState("recover selection")
			if got := mutable.ArmTraceInputAdmissionTerminal(types.StageExplore, result); got != (!test.selection && !test.cancelled) {
				t.Fatalf("physical latch armed=%v repair=%+v", got, result.Repair)
			}
			if test.selection {
				if result.Repair == nil || result.Repair.Metadata["stage"] != types.ToolRepairStageTraceSourceSelection || result.Repair.Metadata["status"] != types.ToolRepairStatusActionRecommended || strings.Contains(result.Summary, "--archive-member") || strings.Contains(result.Summary, "readable export") {
					t.Fatalf("wrong selection teaching: %+v", result)
				}
			} else if test.cancelled && (result.Repair != nil || result.TraceViewCancellation == nil) {
				t.Fatal("cancel became input repair", result)
			}
		})
	}
}

func TestHMC266PublicSelectorFailureRetainsNoAuthorityAndRetries(t *testing.T) {
	hmc176DisableExternalProviders(t)
	zipPath := hmc176WriteSource(t, hmc176Zip(t, map[string][]byte{"good.data": hmc176PerfData(true), "other.data": hmc17NamedBinaryTailFixture()}))
	plain := hmc176WriteSource(t, hmc176PerfData(true))
	for _, test := range []struct{ name, path, member, correct string }{
		{"unselected", zipPath, "", "good.data"},
		{"missing_member", zipPath, "missing.data", "good.data"},
		{"malformed_selector", zipPath, "../good.data", "good.data"},
		{"non_zip_selector", plain, "good.data", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			bus, prep, _ := hmc17NamedPathContext(t)
			query := func(member string) types.ToolResult {
				return hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": test.path, "archive_member": member, "view": "event_search"})
			}
			failed := query(test.member)
			if failed.Success || failed.Repair == nil || failed.Repair.Metadata["stage"] != types.ToolRepairStageTraceSourceSelection || len(prep.PreparedMaterials()) != 0 || len(failed.Observations) != 0 {
				t.Fatalf("bad selector gained authority or terminal failure: %+v", failed)
			}
			if bus.Mutable.ArmTraceInputAdmissionTerminal(types.StageExplore, failed) {
				t.Fatal("selector armed run latch")
			}
			got := hmc17NamedPayload(t, query(test.correct))
			if got.EventCount != 1 || len(prep.PreparedMaterials()) != 1 {
				t.Fatalf("corrected query did not execute full selected population: %+v", got)
			}
		})
	}
}
