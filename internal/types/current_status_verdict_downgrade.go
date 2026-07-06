package types

// current_status_verdict_downgrade.go — SPR #72 (RTC ledger §8.3, 2026-07-06).
//
// Single semantic home for the current-status verdict evidence downgrade:
// when the origin-lane observation ledger carries ZERO current_source
// evidence for a run (the exact lane predicate the mixed-origin completion
// gate logs as `missing_origin_lanes=current_source`), a side-picking
// current-status verdict (`still_present` / `fixed`) contradicts the run's
// own evidence ledger. The model was schema-forced to pick a side, so the
// contradiction is resolved at the DISPLAY/GATE layer, never by rewriting
// the model's emit:
//
//   - the renderer shows the caveat form ("未评估：本轮无源码证据") instead of
//     asserting the raw enum as a conclusion; the original verdict stays on
//     the block (typed audit position) and is named in the audit disclosure;
//   - the current-status obligation gates (pre-emit demand + post-emit
//     contract check) treat the obligation as not evaluable: they neither
//     demand a verdict nor consume a side-picked verdict as a satisfied
//     decision signal.
//
// Architectural fit: lane-record count is a PRECISE signal (hard-gate
// eligible per the precise-signals red line); the downgrade is disclosure,
// not backfill — the system never authors a replacement conclusion.

// CurrentStatusVerdictDowngradeReason is the typed reason lane for a
// current-status verdict downgrade. Exactly one reason exists today;
// the type keeps future reasons schema-checked instead of free prose.
type CurrentStatusVerdictDowngradeReason string

// CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence: the compiled
// observation ledger holds no current_source record with an exact
// current-checkout line span for this run.
const CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence CurrentStatusVerdictDowngradeReason = "zero_current_source_evidence"

// CurrentStatusVerdictDowngrade is the typed downgrade decision stamped
// onto an accepted AnswerDocumentV2 by the tool runtime at persist time.
// It is system-internal: never part of the LLM-facing emit schema and
// never parsed from model prose.
//
// OriginalVerdict is the audit position — the model's raw enum survives
// here (and untouched on the decision block itself) even though the
// rendered surface downgrades it to a caveat form. An empty
// OriginalVerdict records a demand-waiver stamp: the contract required a
// verdict, the run had zero current_source evidence, and the model emitted
// none — gates must not force a side-pick in that state.
type CurrentStatusVerdictDowngrade struct {
	BlockID         string                              `json:"block_id,omitempty"`
	OriginalVerdict CurrentStatusVerdict                `json:"original_verdict,omitempty"`
	Reason          CurrentStatusVerdictDowngradeReason `json:"reason"`
}

// Clone returns a defensive copy (nil-safe) for the MutableState
// answer-document deep-clone round-trip.
func (d *CurrentStatusVerdictDowngrade) Clone() *CurrentStatusVerdictDowngrade {
	if d == nil {
		return nil
	}
	out := *d
	return &out
}

// PicksASide reports whether the verdict asserts a definite current-code
// state (still_present / fixed) as opposed to declaring the evidence
// insufficient. Only side-picking verdicts can contradict an origin-lane
// ledger with zero current_source evidence; not_enough_evidence is
// consistent with that ledger state and is never downgraded.
func (v CurrentStatusVerdict) PicksASide() bool {
	switch v {
	case CurrentStatusStillPresent, CurrentStatusFixed:
		return true
	default:
		return false
	}
}

// ObservationLedgerHasCurrentSourceEvidence reports whether the compiled
// ledger carries at least one current_source record with an exact
// current-checkout line span. This is deliberately the SAME lane predicate
// the mixed-origin completion gate uses for its current_source arm
// (missingObservationOrigins → ObservationRecordHasCurrentSourceLineSpan),
// so the verdict obligation and the completion gate can never disagree on
// whether the source lane produced evidence.
func ObservationLedgerHasCurrentSourceEvidence(ledger ObservationLedger) bool {
	for _, record := range ledger.Records {
		if ObservationRecordHasCurrentSourceLineSpan(record) {
			return true
		}
	}
	return false
}

// BusContextHasCurrentSourceObservationEvidence compiles the run's
// observation ledger from the bus context with the same input shape and
// evidence limit the completion gate uses, then applies the shared lane
// predicate above.
func BusContextHasCurrentSourceObservationEvidence(bus *BusContext) bool {
	if bus == nil {
		return false
	}
	ledger := CompileObservationLedger(ObservationLedgerInputFromBusContext(bus, ObservationExtractLedgerEvidenceLimit))
	return ObservationLedgerHasCurrentSourceEvidence(ledger)
}

// ComputeCurrentStatusVerdictDowngrade is the single decision function for
// the downgrade. It fires only when hasCurrentSourceEvidence is false:
//
//   - a side-picking verdict on the current-status decision block →
//     downgrade carrying the original verdict (render caveat form + gate
//     non-consumption);
//   - no verdict emitted but the contract requires one → demand-waiver
//     stamp (empty OriginalVerdict) so gates cannot force a side-pick;
//   - not_enough_evidence, or any state with source evidence present →
//     nil (evidence runs are byte-identical to pre-SPR behavior).
func ComputeCurrentStatusVerdictDowngrade(
	doc *AnswerDocumentV2,
	contract *CurrentStatusDiagnosticContract,
	hasCurrentSourceEvidence bool,
) *CurrentStatusVerdictDowngrade {
	if doc == nil || hasCurrentSourceEvidence {
		return nil
	}
	block := CurrentStatusDecisionBlock(doc)
	if block != nil && block.CurrentStatusVerdict.PicksASide() {
		return &CurrentStatusVerdictDowngrade{
			BlockID:         block.ID,
			OriginalVerdict: block.CurrentStatusVerdict,
			Reason:          CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence,
		}
	}
	if contract != nil && contract.Required &&
		(block == nil || block.CurrentStatusVerdict == CurrentStatusUnknown) {
		return &CurrentStatusVerdictDowngrade{
			Reason: CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence,
		}
	}
	return nil
}

// CurrentStatusDowngradeForBlock returns the document's downgrade stamp
// IFF it targets the given block's verdict; nil otherwise. Shared by the
// renderer (caveat form) so block matching is not re-derived per caller.
func CurrentStatusDowngradeForBlock(doc *AnswerDocumentV2, block AnswerBlock) *CurrentStatusVerdictDowngrade {
	if doc == nil || doc.CurrentStatusVerdictDowngrade == nil {
		return nil
	}
	d := doc.CurrentStatusVerdictDowngrade
	if d.OriginalVerdict == CurrentStatusUnknown || d.OriginalVerdict != block.CurrentStatusVerdict {
		return nil
	}
	if d.BlockID != "" && d.BlockID != block.ID {
		return nil
	}
	return d
}
