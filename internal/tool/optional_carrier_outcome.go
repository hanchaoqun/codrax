package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// optional_carrier_outcome.go — V2-3 (colleague_merge_audit §40.19, fold-in
// §40.44): an optional input carrier that a tool IGNORES must be disclosed on
// the ToolResult that call returns (typed row + Summary line), never only
// logged — on EVERY returned result: accepted, pre-complete downgrade, or
// rejection.
//
// The lane is one per-call registry, the optionalCarrierLedger. Minting an
// outcome (`ignored` / `mint`) logs it through the ONE sanctioned WARN and
// records it; the creator of the ledger finalizes every result it returns
// through `finalize`, the tool-result choke point that attaches whatever was
// minted during the call. Nothing else may write ToolResult.OptionalCarrierOutcomes
// (repo-wide census), a ledger can only be obtained from newOptionalCarrierLedger,
// and every creator must `defer` the finalize on its named result directly
// after creation — optional_carrier_outcome_census_test.go pins each of those
// shapes mechanically (keyed on the identifiers, not on log wording).
//
// The invariant "every outcome minted during one call reaches the result that
// call returns" therefore holds by construction; the one runtime-detectable
// violation — a mint AFTER the result left the choke point — is a test
// failure under `go test` and a WARN in production (the row is lost, which is
// exactly what the WARN says).

// optionalCarrierLedger is the per-call minted-outcome registry.
type optionalCarrierLedger struct {
	toolName  string
	minted    []types.OptionalCarrierOutcome
	notes     []string
	finalized bool
}

// newOptionalCarrierLedger opens the registry for one tool call. The ONLY
// constructor (census: no other construction of the type exists).
func newOptionalCarrierLedger(toolName string) *optionalCarrierLedger {
	return &optionalCarrierLedger{toolName: toolName}
}

// ignored mints the "ignored" outcome for one carrier: logged through the
// sanctioned lane and registered for the choke point in the same breath.
func (l *optionalCarrierLedger) ignored(carrier, reason string, detail ...string) types.OptionalCarrierOutcome {
	return l.mint(types.OptionalCarrierOutcome{Carrier: carrier, Status: types.OptionalCarrierStatusIgnored, Reason: reason}, detail...)
}

// mint registers an outcome of any closed-set status (retained_previous /
// part_dropped ride here) and logs it. A nil ledger is a programming error
// and fails loud rather than silently dropping the disclosure.
func (l *optionalCarrierLedger) mint(o types.OptionalCarrierOutcome, detail ...string) types.OptionalCarrierOutcome {
	if l == nil {
		panic("optional carrier outcome minted without a ledger — the disclosure would be lost")
	}
	if !types.ValidOptionalCarrierStatus(o.Status) {
		o.Status = types.OptionalCarrierStatusIgnored
	}
	logOptionalCarrierIgnored(l.toolName, o, detail...)
	if l.finalized {
		// The result already left the choke point; the row cannot reach it.
		msg := "[" + l.toolName + "] optional carrier outcome minted after the tool result was finalized; disclosure for carrier " + o.Carrier + " is lost"
		if testing.Testing() {
			panic(msg)
		}
		logging.Warning("%s", msg)
		return o
	}
	l.minted = append(l.minted, o)
	return o
}

// note registers one plain advisory Summary line (§40.44 G-emit-faces
// fold-in #2): soft guidance about a carrier the tool DID honour — e.g. the
// binder's non-dropping caliber note beside a description published as
// written. A note is not an OptionalCarrierOutcome (the closed set describes
// only parts that were NOT honoured, and minting part_dropped for a kept
// part would be a false typed status inviting a needless patch round), so it
// carries no typed row, no status token, and no repair hint — only its own
// line on whichever result the call returns.
func (l *optionalCarrierLedger) note(line string) {
	if l == nil {
		panic("optional carrier note registered without a ledger — the guidance would be lost")
	}
	if strings.TrimSpace(line) == "" {
		return
	}
	if l.finalized {
		msg := "[" + l.toolName + "] optional carrier note registered after the tool result was finalized; the advisory line is lost: " + line
		if testing.Testing() {
			panic(msg)
		}
		logging.Warning("%s", msg)
		return
	}
	l.notes = append(l.notes, line)
}

// finalize is the tool-result choke point: every outcome minted during the
// call that is not yet on the result is attached (typed row + its own
// Summary line unless the Summary already words that reason verbatim —
// executors whose accepted prose already audits the drop keep it and gain
// only the typed row).
func (l *optionalCarrierLedger) finalize(res types.ToolResult) types.ToolResult {
	if l == nil {
		return res
	}
	l.finalized = true
	for _, o := range l.minted {
		if optionalCarrierOutcomeAttached(res.OptionalCarrierOutcomes, o) {
			continue
		}
		attachOptionalCarrierOutcome(&res, o)
	}
	// Plain advisory notes ride after the typed rows: one line each, no
	// typed row, no status token, skipped when the Summary already words the
	// note verbatim (same idempotence rule as the rows).
	for _, line := range l.notes {
		if strings.Contains(res.Summary, line) {
			continue
		}
		res.Summary = strings.TrimRight(res.Summary, "\n")
		if res.Summary != "" {
			res.Summary += "\n"
		}
		res.Summary += line
	}
	return res
}

func optionalCarrierOutcomeAttached(rows []types.OptionalCarrierOutcome, o types.OptionalCarrierOutcome) bool {
	for _, row := range rows {
		if row.Carrier == o.Carrier && row.Status == o.Status && row.Reason == o.Reason {
			return true
		}
	}
	return false
}

// logOptionalCarrierIgnored is the ONE sanctioned WARN for a not-honored
// optional carrier. Only the ledger's mint calls it (census), which is what
// makes the log and the disclosure inseparable. Extra detail pairs (e.g. a
// truncated rationale, the decoder's own error text) ride the log line only.
func logOptionalCarrierIgnored(toolName string, o types.OptionalCarrierOutcome, detail ...string) {
	line := "[" + toolName + "] optional carrier " + o.Carrier + " " + o.Status + ": " + o.Reason
	if len(detail) > 0 {
		line += " " + strings.Join(detail, " ")
	}
	logging.Warning("%s", line)
}

// attachOptionalCarrierOutcome appends one typed row and renders it onto its
// own Summary line unless the Summary already carries that reason verbatim.
// Only the ledger's finalize calls it (census: the field has one writer).
func attachOptionalCarrierOutcome(res *types.ToolResult, o types.OptionalCarrierOutcome) {
	if res == nil || strings.TrimSpace(o.Carrier) == "" || strings.TrimSpace(o.Reason) == "" {
		return
	}
	if !types.ValidOptionalCarrierStatus(o.Status) {
		o.Status = types.OptionalCarrierStatusIgnored
	}
	res.OptionalCarrierOutcomes = append(res.OptionalCarrierOutcomes, o)
	if strings.Contains(res.Summary, o.Reason) {
		return
	}
	line := types.OptionalCarrierOutcomeSummaryLine(o)
	res.Summary = strings.TrimRight(res.Summary, "\n")
	if res.Summary != "" {
		res.Summary += "\n"
	}
	res.Summary += line
}
