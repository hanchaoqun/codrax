package types

import "strings"

// OptionalCarrierOutcome (V2-3, colleague_merge_audit §40.19) is the typed
// disclosure that an OPTIONAL input carrier on an otherwise accepted tool
// call was not honored in full. "Optional" is the precise class: the carrier
// never owns whether the main transaction is accepted (an invalid root-cause
// selector never rejects the full answer, an invalid waiver never rejects the
// completion, a wrong request echo never rejects the write analysis) — so
// the ONLY way the model learns that its carrier was dropped is this record.
// The rule the census pins: an optional-carrier ignore must reach the
// ToolResult (this slot + one Summary line), never only a log line.
//
// System-produced only (never an LLM emit field, so no tool-schema /
// skill-prompt / retry-hint sync obligations apply; ReusedFromRunMemo
// precedent). Reason is the producer's own precise wording (binder error,
// waiver rejection prose) so the model reads exactly what was checked; Hint
// is the repair route when one exists.
type OptionalCarrierOutcome struct {
	// Carrier is the wire field name the model submitted (e.g.
	// "trace_root_causes", "evidence_floor_waiver", "raw_request").
	Carrier string `json:"carrier"`
	// Status is one of the OptionalCarrierStatus* constants.
	Status string `json:"status"`
	// Reason is the precise reason the carrier (or the named part of it) was
	// not honored — the producer's verbatim wording.
	Reason string `json:"reason"`
	// Hint is the optional repair route (which field / which call to resend).
	Hint string `json:"hint,omitempty"`
}

const (
	// OptionalCarrierStatusIgnored: the carrier was dropped and nothing of
	// it took effect.
	OptionalCarrierStatusIgnored = "ignored"
	// OptionalCarrierStatusRetainedPrevious: the carrier was dropped and the
	// previously accepted value of the same carrier stays in force (patch
	// semantics: a bad replacement never withdraws an accepted selection).
	OptionalCarrierStatusRetainedPrevious = "retained_previous"
	// OptionalCarrierStatusPartDropped: the carrier was accepted but one
	// named part of it was dropped (e.g. a root-cause description that cited
	// an internal reference; the typed selection itself stands).
	OptionalCarrierStatusPartDropped = "part_dropped"
)

// ValidOptionalCarrierStatus reports closed-set membership.
func ValidOptionalCarrierStatus(status string) bool {
	switch status {
	case OptionalCarrierStatusIgnored, OptionalCarrierStatusRetainedPrevious, OptionalCarrierStatusPartDropped:
		return true
	}
	return false
}

// AllOptionalCarrierStatuses lists the closed set in its documented order.
func AllOptionalCarrierStatuses() []string {
	return []string{OptionalCarrierStatusIgnored, OptionalCarrierStatusRetainedPrevious, OptionalCarrierStatusPartDropped}
}

// OptionalCarrierOutcomeSummaryLine is the ONE model-facing rendering of an
// outcome, appended by the tool layer on its own line after the accepted
// summary: `[optional_carrier_<status>: carrier=<c> reason=<r>; <hint>]`.
// The line never spells the token `citations=` (the accepted-summary
// renderer binds its registered-count regex to that token) — the pin in
// optional_carrier_outcome_test.go holds the renderer to it.
func OptionalCarrierOutcomeSummaryLine(o OptionalCarrierOutcome) string {
	status := strings.TrimSpace(o.Status)
	if !ValidOptionalCarrierStatus(status) {
		status = OptionalCarrierStatusIgnored
	}
	var b strings.Builder
	b.WriteString("[optional_carrier_")
	b.WriteString(status)
	b.WriteString(": carrier=")
	b.WriteString(optionalCarrierSummaryToken(o.Carrier))
	b.WriteString(" reason=")
	b.WriteString(optionalCarrierSummaryToken(o.Reason))
	if hint := strings.TrimSpace(o.Hint); hint != "" {
		b.WriteString("; ")
		b.WriteString(optionalCarrierSummaryToken(hint))
	}
	b.WriteString("]")
	return b.String()
}

// optionalCarrierSummaryToken compacts whitespace and neutralises the exact
// accepted-summary count tokens so a reason quoting model text can never be
// mistaken for the registered block/citation counts on the same summary.
func optionalCarrierSummaryToken(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	for _, token := range []string{"citations=", "blocks="} {
		text = strings.ReplaceAll(text, token, strings.TrimSuffix(token, "=")+" =")
	}
	return text
}
