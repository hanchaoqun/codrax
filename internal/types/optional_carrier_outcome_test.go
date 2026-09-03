package types

import (
	"regexp"
	"strings"
	"testing"
)

// optional_carrier_outcome_test.go — V2-3 (§40.19 ①): the typed outcome's
// status is a closed set and its ONE model-facing line can never be mistaken
// for the accepted-summary count tokens the render layer binds
// (`blocks=N … citations=M`, internal/render/structured_tool_summary.go).

func TestOptionalCarrierStatusIsAClosedSet(t *testing.T) {
	for _, status := range AllOptionalCarrierStatuses() {
		if !ValidOptionalCarrierStatus(status) {
			t.Fatalf("documented status %q is not valid", status)
		}
	}
	for _, bad := range []string{"", "dropped", "IGNORED", " ignored"} {
		if ValidOptionalCarrierStatus(bad) {
			t.Fatalf("%q must not be a valid status", bad)
		}
	}
}

func TestOptionalCarrierOutcomeSummaryLineNeverSpellsTheCountTokens(t *testing.T) {
	countsRe := regexp.MustCompile(`blocks=([0-9]+).*citations=([0-9]+)`)
	line := OptionalCarrierOutcomeSummaryLine(OptionalCarrierOutcome{
		Carrier: "trace_root_causes", Status: OptionalCarrierStatusIgnored,
		Reason: "root_causes[0].candidate_id \"x\" is outside the roster blocks=9 citations=9",
		Hint:   "resend via replace_trace_root_causes\n(citations=3)",
	})
	if !strings.HasPrefix(line, "[optional_carrier_ignored: carrier=trace_root_causes reason=") || !strings.HasSuffix(line, "]") {
		t.Fatalf("unexpected line shape: %q", line)
	}
	if strings.Contains(line, "citations=") || strings.Contains(line, "blocks=") || strings.Contains(line, "\n") {
		t.Fatalf("the outcome line must never carry the accepted-summary count tokens or a line break: %q", line)
	}
	if countsRe.MatchString(line) {
		t.Fatalf("outcome line alone re-binds the accepted-count regex: %q", line)
	}
	if got := OptionalCarrierOutcomeSummaryLine(OptionalCarrierOutcome{Carrier: "c", Status: "bogus", Reason: "r"}); !strings.HasPrefix(got, "[optional_carrier_ignored: ") {
		t.Fatalf("an unknown status must render as ignored, never as a new tag: %q", got)
	}
	if got := OptionalCarrierOutcomeSummaryLine(OptionalCarrierOutcome{Carrier: "c", Status: OptionalCarrierStatusPartDropped, Reason: "r"}); got != "[optional_carrier_part_dropped: carrier=c reason=r]" {
		t.Fatalf("hint-less rendering drifted: %q", got)
	}
}
