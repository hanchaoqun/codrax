package hitraceconv

import "strings"

// Source raw decode ledger states (`source_rawtrace_decode.decode_state`).
// This is the closed set every writer mints from (setUnavailable / finalize
// in source_raw_decode_ledger.go, the profile probe) and every reader
// classifies against (traceDBRawDecodeCensusComplete, the lane gate below,
// the decoder-authority reconcile table in source_name_inventory.go). A
// state's meaning is carried by its gate kind in traceDBRawDecodeStateGates —
// never by a prefix of its spelling.
const (
	traceDBRawDecodeStateUnavailable                              = "unavailable"
	traceDBRawDecodeStateNotApplicableNonOfficialProfile          = "not_applicable_non_official_profile"
	traceDBRawDecodeStateWithheldSegmentInventoryIncomplete       = "withheld_segment_inventory_incomplete"
	traceDBRawDecodeStateWithheldProfileNotReady                  = "withheld_profile_not_ready"
	traceDBRawDecodeStateWithheldRecordFormatAccountingIncomplete = "withheld_record_format_accounting_incomplete"
	traceDBRawDecodeStateIncompleteFormatWitnessCap               = "incomplete_format_witness_cap"
	traceDBRawDecodeStateComplete                                 = "strict_target_ledger_complete"
	traceDBRawDecodeStateCompleteWithFamilyRetentionWithdrawal    = "strict_target_ledger_complete_with_family_retention_withdrawal"
)

// traceDBSourceRawGateKind is the typed outcome of the source-raw lane
// pre-publication gate (colleague_merge_audit §40.53, V6-4). Every recovery
// lane that reads the strict raw decode ledger asks this ONE classifier before
// minting a zero-row publication state, so "the envelope is not official"
// (not applicable), "the envelope is official but the decode census did not
// close" (census incomplete) and "the census closed" (ready — lane-specific
// withheld_/complete_/published_ arms follow) can never wear each other's
// label. The zero value never publishes: an unrecognized decode_state fails
// the lane loud instead of being absorbed into a neighbouring label (§40.50).
type traceDBSourceRawGateKind int

const (
	traceDBSourceRawGateUnset traceDBSourceRawGateKind = iota
	traceDBSourceRawGateNotApplicable
	traceDBSourceRawGateCensusIncomplete
	traceDBSourceRawGateReady
)

// traceDBRawDecodeStateGates is the single table binding each decode_state to
// its gate kind; it is total over the declared constants (pinned by the census
// in source_raw_lane_gate_census_test.go). "unavailable" is a not-applicable
// outcome: the inventory scan returned before the raw profile probe ran
// (truncated, invalid or unsupported common envelope), so no official profile
// exists. A decode_state absent from this table resolves to
// traceDBSourceRawGateUnset.
var traceDBRawDecodeStateGates = map[string]traceDBSourceRawGateKind{
	traceDBRawDecodeStateUnavailable:                              traceDBSourceRawGateNotApplicable,
	traceDBRawDecodeStateNotApplicableNonOfficialProfile:          traceDBSourceRawGateNotApplicable,
	traceDBRawDecodeStateWithheldSegmentInventoryIncomplete:       traceDBSourceRawGateCensusIncomplete,
	traceDBRawDecodeStateWithheldProfileNotReady:                  traceDBSourceRawGateCensusIncomplete,
	traceDBRawDecodeStateWithheldRecordFormatAccountingIncomplete: traceDBSourceRawGateCensusIncomplete,
	traceDBRawDecodeStateIncompleteFormatWitnessCap:               traceDBSourceRawGateCensusIncomplete,
	traceDBRawDecodeStateComplete:                                 traceDBSourceRawGateReady,
	traceDBRawDecodeStateCompleteWithFamilyRetentionWithdrawal:    traceDBSourceRawGateReady,
}

// traceDBSourceRawDecodeGate classifies one held strict raw decode ledger for
// a recovery lane. The decision reads precise typed signals only — the
// ledger's Found bit and its decode_state membership in
// traceDBRawDecodeStateGates — and returns the decode_state verbatim as the
// reason so a census-incomplete lane can disclose WHICH census did not close
// without re-deriving it. A Found bit that contradicts the state's gate kind
// (an official ledger that says not-applicable, or a non-official one that
// claims a census outcome) is an unrecognized shape and resolves to Unset. An
// absent ledger is not-applicable with an empty reason: no source was
// profiled at all.
func traceDBSourceRawDecodeGate(decode *TraceDBCoverage) (traceDBSourceRawGateKind, string) {
	if decode == nil {
		return traceDBSourceRawGateNotApplicable, ""
	}
	state := decode.Metadata["decode_state"]
	kind := traceDBRawDecodeStateGates[state]
	switch kind {
	case traceDBSourceRawGateNotApplicable:
		if decode.Found {
			return traceDBSourceRawGateUnset, state
		}
	case traceDBSourceRawGateCensusIncomplete, traceDBSourceRawGateReady:
		if !decode.Found {
			return traceDBSourceRawGateUnset, state
		}
	}
	return kind, state
}

// traceDBSourceRawLaneGate classifies the strict raw decode ledger held by an
// immutable source inventory (nil inventory: not applicable, empty reason).
func traceDBSourceRawLaneGate(inventory *traceDBSourceNameInventory) (traceDBSourceRawGateKind, string) {
	return traceDBSourceRawDecodeGate(traceDBSourceRawInventoryDecode(inventory))
}

func traceDBSourceRawInventoryDecode(inventory *traceDBSourceNameInventory) *TraceDBCoverage {
	if inventory == nil {
		return nil
	}
	return &inventory.RawDecode
}

// traceDBSourceRawLaneStateKey is the closed set of coverage metadata keys
// under which a source-raw lane publishes its state. Every key is gated by
// the same classifier and mints the same two non-ready states through the
// same funnel (pinned by the go/ast census: every declared key has a gate
// call site, and the shared states are never hand-typed under any key).
type traceDBSourceRawLaneStateKey string

const (
	traceDBSourceRawLaneStateKeyPublication      traceDBSourceRawLaneStateKey = "publication_state"
	traceDBSourceRawLaneStateKeyReconciliation   traceDBSourceRawLaneStateKey = "reconciliation_state"
	traceDBSourceRawLaneStateKeyJoin             traceDBSourceRawLaneStateKey = "join_state"
	traceDBSourceRawLaneStateKeyAuthority        traceDBSourceRawLaneStateKey = "authority_state"
	traceDBSourceRawLaneStateKeyRecovery         traceDBSourceRawLaneStateKey = "recovery_state"
	traceDBSourceRawLaneStateKeyLedger           traceDBSourceRawLaneStateKey = "ledger_state"
	traceDBSourceRawLaneStateKeyAsyncReplacement traceDBSourceRawLaneStateKey = "raw_async_replacement_state"
)

// Shared publication states of the two non-ready gate outcomes and the
// pre-publication placeholder. Every lane mints the two outcomes through
// traceDBMintSourceRawLaneGateOutcome, so the class has one spelling per
// outcome; the placeholder is seeded only by coverage constructors and is
// left standing only on a fail-loud exit (out.Error set), never on a
// successful return.
const (
	traceDBSourceRawLanePlaceholderState      = "unavailable"
	traceDBSourceRawLaneNotApplicableState    = "not_applicable_source_profile"
	traceDBSourceRawLaneCensusIncompleteState = "census_incomplete_source_raw_decode"
	// traceDBSourceRawLaneFamilyRetentionWithdrawnState is the post-gate
	// withheld arm shared by the retained-family lanes: the census closed but
	// this family's retained record store was withdrawn by its byte budget
	// (decode_state strict_target_ledger_complete_with_family_retention_withdrawal).
	traceDBSourceRawLaneFamilyRetentionWithdrawnState = "withheld_family_retention_budget_exceeded"
	traceDBSourceRawLaneGateUnresolvedReason          = "source_raw_lane_gate_unresolved"
)

// traceDBMintSourceRawLaneGateOutcome is the ONE write funnel for the two
// non-ready gate outcomes. It writes the state under `key`, the verbatim
// decode_state as typed reason metadata, and the customer-visible Skipped
// prose. `lane` is the user-facing lane phrase that opens every Skipped
// sentence of that lane (e.g. "raw block recovery"), so the class shares one
// prose shape per outcome:
//
//	<lane> not applicable: strict official source raw profile absent
//	<lane> not evaluated: source raw decode census incomplete (<decode_state>)
//
// The census-incomplete prose deliberately says neither "withheld" nor "not
// applicable": the lane was never evaluated, and the verbatim decode_state is
// also carried as typed metadata `census_incomplete_reason`; the not-applicable
// arm carries its decode_state as `not_applicable_reason` when one exists. It
// returns stop=true when the lane must return immediately (err is set only
// for the fail-loud Unset shape, which mints no state) and stop=false when
// the census closed and the lane's own eligibility arms apply.
func traceDBMintSourceRawLaneGateOutcome(
	out *TraceDBCoverage,
	key traceDBSourceRawLaneStateKey,
	lane string,
	kind traceDBSourceRawGateKind,
	reason string,
) (stop bool, err error) {
	if out == nil {
		return true, &traceDBOutputInvariantError{Reason: traceDBSourceRawLaneGateUnresolvedReason}
	}
	if out.Metadata == nil {
		out.Metadata = map[string]string{}
	}
	stateKey := string(key)
	switch kind {
	case traceDBSourceRawGateReady:
		return false, nil
	case traceDBSourceRawGateNotApplicable:
		out.Metadata[stateKey] = traceDBSourceRawLaneNotApplicableState
		if reason != "" {
			out.Metadata["not_applicable_reason"] = reason
		}
		out.Skipped = lane + " not applicable: strict official source raw profile absent"
		return true, nil
	case traceDBSourceRawGateCensusIncomplete:
		out.Metadata[stateKey] = traceDBSourceRawLaneCensusIncompleteState
		out.Metadata["census_incomplete_reason"] = reason
		out.Skipped = lane + " not evaluated: source raw decode census incomplete (" + reason + ")"
		return true, nil
	default:
		gateErr := &traceDBOutputInvariantError{Reason: traceDBSourceRawLaneGateUnresolvedReason}
		out.Error = gateErr.Error()
		return true, gateErr
	}
}

// traceDBApplySourceRawDecodeGate runs the gate over one strict raw decode
// ledger (nil: absent) for a lane publishing under `key`.
func traceDBApplySourceRawDecodeGate(
	out *TraceDBCoverage,
	decode *TraceDBCoverage,
	key traceDBSourceRawLaneStateKey,
	lane string,
) (stop bool, err error) {
	kind, reason := traceDBSourceRawDecodeGate(decode)
	return traceDBMintSourceRawLaneGateOutcome(out, key, lane, kind, reason)
}

// traceDBApplySourceRawLaneGateKeyed runs the gate over the ledger held by an
// immutable source inventory for a lane publishing under `key`. It never
// touches out.Found: lanes whose Found bit means "raw records retained" keep
// their own meaning.
func traceDBApplySourceRawLaneGateKeyed(
	out *TraceDBCoverage,
	inventory *traceDBSourceNameInventory,
	key traceDBSourceRawLaneStateKey,
	lane string,
) (stop bool, err error) {
	return traceDBApplySourceRawDecodeGate(out, traceDBSourceRawInventoryDecode(inventory), key, lane)
}

// traceDBApplySourceRawLaneGate runs the gate for a lane publishing under
// `publication_state`, whose Found bit mirrors the decode ledger's.
func traceDBApplySourceRawLaneGate(
	out *TraceDBCoverage,
	inventory *traceDBSourceNameInventory,
	lane string,
) (stop bool, err error) {
	if out != nil && inventory != nil {
		out.Found = inventory.RawDecode.Found
	}
	return traceDBApplySourceRawLaneGateKeyed(out, inventory, traceDBSourceRawLaneStateKeyPublication, lane)
}

// traceDBInheritSourceRawLaneGate lets a lane that consumes another lane's
// published coverage (not the inventory) carry that lane's non-ready gate
// outcome forward under its own key and lane phrase: the same two states, the
// same reason metadata, the same prose shape, through the same funnel. It
// reports whether an outcome was inherited (the caller returns immediately);
// any other upstream state means the upstream lane passed the gate and the
// caller's own arms apply.
func traceDBInheritSourceRawLaneGate(
	out *TraceDBCoverage,
	from TraceDBCoverage,
	fromKey traceDBSourceRawLaneStateKey,
	key traceDBSourceRawLaneStateKey,
	lane string,
) bool {
	var kind traceDBSourceRawGateKind
	var reason string
	switch from.Metadata[string(fromKey)] {
	case traceDBSourceRawLaneNotApplicableState:
		kind, reason = traceDBSourceRawGateNotApplicable, from.Metadata["not_applicable_reason"]
	case traceDBSourceRawLaneCensusIncompleteState:
		kind, reason = traceDBSourceRawGateCensusIncomplete, from.Metadata["census_incomplete_reason"]
	default:
		return false
	}
	stop, _ := traceDBMintSourceRawLaneGateOutcome(out, key, lane, kind, reason)
	return stop
}

// traceDBSourceRawPublicationStatePrefix declares one vocabulary prefix of the
// source-raw lane `publication_state` class and whether a state under it lets
// downstream quality consumers evaluate the lane's metrics. The roster is the
// single source for the class contract: every publication_state value
// assigned by a source_raw_*_recovery.go lane starts with exactly one of these
// prefixes (pinned by the go/ast census in source_raw_lane_gate_census_test.go;
// constructors seed only the placeholder), and the semantic-quality consumer
// decides "evaluate or not" from the roster rather than from a hand-typed
// prefix.
type traceDBSourceRawPublicationStatePrefix struct {
	Prefix    string
	Evaluates bool
}

var traceDBSourceRawPublicationStatePrefixes = []traceDBSourceRawPublicationStatePrefix{
	{Prefix: "not_applicable_", Evaluates: false},
	{Prefix: "census_incomplete_", Evaluates: false},
	{Prefix: "withheld_", Evaluates: false},
	{Prefix: "complete_", Evaluates: true},
	{Prefix: "published_", Evaluates: true},
	{Prefix: "submitted_", Evaluates: true},
}

// traceDBSourceRawPublicationStateBlocksEvaluation reports whether a lane's
// publication_state is one under which no downstream consumer may evaluate
// the lane's metrics: the lane did not run (not applicable), was not evaluated
// (census incomplete) or evaluated and withheld. Membership is decided by the
// roster above; a state under no roster prefix is not blocked here — the
// class census keeps that set empty for every assigned value.
func traceDBSourceRawPublicationStateBlocksEvaluation(state string) bool {
	for _, entry := range traceDBSourceRawPublicationStatePrefixes {
		if strings.HasPrefix(state, entry.Prefix) {
			return !entry.Evaluates
		}
	}
	return false
}
