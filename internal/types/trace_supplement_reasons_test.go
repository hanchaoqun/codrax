package types

// trace_supplement_reasons_test.go — SUPP-HYG P3-4 registry structural pins
// (NKR golden 同构): the closed set is pinned MEMBER-FOR-MEMBER, so adding,
// renaming, or dropping a reason without walking the change protocol
// (constant + registry row + golden row + producer/consumer/log sites in one
// change) is red here.

import (
	"strings"
	"testing"
)

// TestTraceSupplementReasonRegistryGolden pins the exact closed set, in
// registration order. Wire values are deliberately spelled verbatim (golden
// double-write): a constant edit that silently changes a wire value is red.
func TestTraceSupplementReasonRegistryGolden(t *testing.T) {
	golden := []string{
		"families_present",
		"no_typed_target",
		"no_typed_window",
		"window_inconsistent",
		"window_span_exceeded",
		"duration_budget_exceeded",
		"cold_budget_exceeded",
		"execution_failed",
		"disabled",
		"no_attached_trace",
		"windowed_census_absent",
		"canceled_by_caller",
	}
	got := TraceSupplementReasons()
	if strings.Join(got, "\n") != strings.Join(golden, "\n") {
		t.Fatalf("trace supplement reason registry drifted from the golden set\ngot:\n%s\nwant:\n%s",
			strings.Join(got, "\n"), strings.Join(golden, "\n"))
	}
}

// TestTraceSupplementReasonRegistered pins membership semantics: every
// registered reason answers true; unknown / empty / case-drifted values are
// NOT members (未注册 reason 即红 at every consumer that checks membership).
func TestTraceSupplementReasonRegistered(t *testing.T) {
	for _, reason := range TraceSupplementReasons() {
		if !TraceSupplementReasonRegistered(reason) {
			t.Fatalf("registered reason %q must report membership", reason)
		}
	}
	for _, unknown := range []string{"", "not_a_reason", "Families_Present", "duration_budget_exceeded "} {
		if TraceSupplementReasonRegistered(unknown) {
			t.Fatalf("unregistered value %q must not report membership", unknown)
		}
	}
}
