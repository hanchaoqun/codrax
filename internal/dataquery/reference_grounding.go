package dataquery

import (
	"fmt"
	"math/big"
	"strings"
)

// Reference-set output grounding (DATAGATE-1 follow-up, audit GAP-3 replay
// 2026-07-19, eval data_multifile_reference_projection): a reconcile report
// only proves contributions↔answer internal consistency — it never proves the
// answer projects the user's reference set. Both replay runs published
// internally-consistent wrong answers: "17,5" (2 items for 3 reference keys)
// and "17,4,5" (a non-reference group's total occupying a reference slot that
// must be 0). EvaluateReferenceGrounding is the typed hard check: given the
// resolved reference key universe (ordered), the contributions ledger, and
// the parsed answer items, it demands
//
//	arm A (cardinality):  len(answer items) == len(reference keys)
//	arm B (per-slot):     answer[i] == contribution total(keys[i]),
//	                      and == 0 when the key has no contribution records.
//
// All carriers are typed (reference records, contribution ledger, strict
// list answer); totals and value comparison reuse the exact reconcile-side
// machinery (aggregateContributionGroups / contributionAggregateMatchesValue
// / parseDecimalRat) so this face can never disagree with the reconcile
// validator about what a group sums to. Fail-open is PER-KEY, never global
// (merged review 2026-07-19, F2): a single noisy ledger row (a second metric
// on one group, a text aggregate) makes only THAT key's slot unjudgeable;
// every other slot and the cardinality arm — which needs nothing but keys
// and answer items — keep their hard verdicts. The whole check goes
// inapplicable only when it cannot be applied at all: no reference keys, an
// empty answer, or a ledger with no numeric target domain anywhere.

// ReferenceGroundingMismatch describes one reference slot whose answer item
// does not equal the ledger total for that slot's key.
type ReferenceGroundingMismatch struct {
	// Slot is the 1-based position in the reference order.
	Slot int `json:"slot"`
	// Key is the reference key that owns the slot.
	Key string `json:"key"`
	// Expected is the contribution-ledger total for Key ("0" when the key
	// has no contribution records).
	Expected string `json:"expected"`
	// Actual is the answer item occupying the slot ("(missing)" when the
	// answer has fewer items than the reference has keys).
	Actual string `json:"actual"`
	// HasContributions reports whether any contribution records exist for
	// Key (false means the expected 0 comes from the no-records rule).
	HasContributions bool `json:"has_contributions,omitempty"`
}

// ReferenceGroundingReport is the typed verdict of EvaluateReferenceGrounding.
type ReferenceGroundingReport struct {
	Path                string `json:"path,omitempty"`
	Field               string `json:"field,omitempty"`
	ReferenceKeyCount   int    `json:"reference_key_count"`
	AnswerItemCount     int    `json:"answer_item_count"`
	CardinalityMismatch bool   `json:"cardinality_mismatch,omitempty"`
	// LedgerDomainMismatch reports that the contribution ledger has numeric
	// totals but NONE of them is keyed by a reference key: the ledger does
	// not speak the reference domain at all (witness replay#3 run-1: every
	// contribution carried the literal field name "canonical_label" as its
	// group key and the grand total 37 shipped). Such a ledger can never
	// discharge a per-reference-key output obligation; zero-filling every
	// slot from it would launder an empty projection, so it is a violation,
	// not a pass.
	LedgerDomainMismatch bool                         `json:"ledger_domain_mismatch,omitempty"`
	Mismatches           []ReferenceGroundingMismatch `json:"mismatches,omitempty"`
}

// Violated reports whether the answer fails reference grounding.
func (r ReferenceGroundingReport) Violated() bool {
	return r.CardinalityMismatch || r.LedgerDomainMismatch || len(r.Mismatches) > 0
}

// SplitAnswerListItems splits a strict-list answer into trimmed non-empty
// items using the same comma discipline as InferAnswerItemCount's list lane.
// A single-item answer without a delimiter is one item.
func SplitAnswerListItems(answer string) []string {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(answer, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// EvaluateReferenceGrounding runs arms A and B against the resolved reference
// candidate. The boolean reports applicability: false means the check could
// not be applied at all (no keys, empty answer, or a ledger with no numeric
// target domain anywhere) and MUST NOT be treated as a pass or a failure —
// callers stay fail-open. A group key whose numeric total is ambiguous
// (multi-metric split, text aggregate) exempts only its OWN slot: arm A and
// every unambiguous slot keep their verdicts (merged review 2026-07-19, F2 —
// the previous whole-check bail let one poisoned ledger row silence the gate
// and resurrect the "17,5"/"17,4,5" disease shapes).
func EvaluateReferenceGrounding(candidate ReferenceKeyCandidate, contributions []ContributionRecord, answer string) (ReferenceGroundingReport, bool) {
	// Reference order is load-bearing (slots are positional): preserve the
	// candidate's key sequence, never sort it.
	keys := cleanStringListPreserveOrder(candidate.Keys)
	if len(keys) == 0 {
		return ReferenceGroundingReport{}, false
	}
	items := SplitAnswerListItems(answer)
	if len(items) == 0 {
		return ReferenceGroundingReport{}, false
	}
	totals, ambiguous := contributionNumericTotalsByGroup(contributions)
	// The whole check is a NUMERIC TOTALS projection gate: it applies only
	// when the ledger holds at least one unambiguous numeric target total.
	// An empty totals map means the case is not a totals projection at all —
	// audit-role-only ledgers (replay#5 run-2: zero-expectation inversion
	// damned the correct "17,0,5" and blessed "0,0,0"), and selection-shaped
	// ledgers whose target groups are all text aggregates (a JSON id-subset
	// answer legitimately has fewer items than reference keys, so even the
	// cardinality arm is unsound there). Inapplicable — the required-ledger
	// validator owns empty-target-ledger violations. Note this boundary is
	// what keeps per-key ambiguity honest: one noisy row can silence only
	// its own key, never the last unambiguous numeric total of another key.
	if len(totals) == 0 {
		return ReferenceGroundingReport{}, false
	}
	report := ReferenceGroundingReport{
		Path:              candidate.Path,
		Field:             candidate.Field,
		ReferenceKeyCount: len(keys),
		AnswerItemCount:   len(items),
	}
	// Arm A (cardinality) depends on nothing but the resolved keys and the
	// parsed answer items — it must never die with the totals map (merged
	// review F2 adversarial A2b: one poisoned row silenced the 2-vs-3
	// mismatch and "17,5" sailed).
	if len(items) != len(keys) {
		report.CardinalityMismatch = true
	}
	// Domain arm: an ambiguous group still proves the ledger SPEAKS that
	// key, so ambiguity never manufactures a domain mismatch.
	covered := 0
	for _, key := range keys {
		if _, has := totals[key]; has || ambiguous[key] {
			covered++
		}
	}
	if covered == 0 {
		report.LedgerDomainMismatch = true
	}
	// Key-echo exemption is UNIFORM (merged review F1): only an answer that
	// echoes every slot's own key in order is a pure key projection. A mixed
	// answer ("17,GroupX,5" — numeric slots plus one key echo standing where
	// a 0 must be) is not a projection of anything; its echo slots are
	// violations like any other non-numeric slot.
	uniformKeyEcho := len(items) == len(keys)
	if uniformKeyEcho {
		for i, key := range keys {
			if items[i] != key {
				uniformKeyEcho = false
				break
			}
		}
	}
	zero := new(big.Rat)
	for i, key := range keys {
		if ambiguous[key] {
			// Per-key fail-open: this slot's expected total is ambiguous
			// (multi-metric split or text aggregate) — no verdict on this
			// slot, full verdicts everywhere else.
			continue
		}
		expected := zero
		aggregate, has := totals[key]
		if has {
			expected = aggregate
		}
		if i >= len(items) {
			report.Mismatches = append(report.Mismatches, ReferenceGroundingMismatch{
				Slot:             i + 1,
				Key:              key,
				Expected:         formatRat(expected),
				Actual:           "(missing)",
				HasContributions: has,
			})
			continue
		}
		actual, err := parseDecimalRat(items[i])
		if err != nil {
			// Non-numeric slot: a slot-faithful echo inside a PURE key
			// projection is legitimate; anything else (ledger prose,
			// key=value dumps, a key echo mixed into numeric slots) is not
			// a projection at all. Replay#5 run-1 published "value=11;
			// GroupA/value=17; …" through the old blanket non-numeric
			// fail-open — on a reference-bound case, an unparseable answer
			// is a violation, not an exemption.
			if uniformKeyEcho {
				continue
			}
			report.Mismatches = append(report.Mismatches, ReferenceGroundingMismatch{
				Slot:             i + 1,
				Key:              key,
				Expected:         formatRat(expected),
				Actual:           items[i],
				HasContributions: has,
			})
			continue
		}
		if expected.Cmp(actual) != 0 {
			report.Mismatches = append(report.Mismatches, ReferenceGroundingMismatch{
				Slot:             i + 1,
				Key:              key,
				Expected:         formatRat(expected),
				Actual:           items[i],
				HasContributions: has,
			})
		}
	}
	return report, true
}

// contributionNumericTotalsByGroup projects the reconcile-side aggregation
// onto bare group keys (metric-agnostic). A group key whose numeric total is
// split across multiple metrics, or whose aggregate is text-only, cannot
// ground a numeric slot — that KEY joins the ambiguous set and only its own
// slot goes unjudged (per-key fail-open, merged review 2026-07-19 F2; the
// previous whole-map bail let one noisy row disarm every slot and the
// cardinality arm with it). The outcome is order-independent: once a key is
// ambiguous it never re-enters totals.
func contributionNumericTotalsByGroup(contributions []ContributionRecord) (map[string]*big.Rat, map[string]bool) {
	aggregates := aggregateContributionGroups(reconcileTargetContributions(contributions))
	totals := map[string]*big.Rat{}
	ambiguous := map[string]bool{}
	for _, aggregate := range aggregates {
		if aggregate == nil {
			continue
		}
		key := strings.TrimSpace(aggregate.GroupKey)
		if key == "" {
			continue
		}
		if ambiguous[key] {
			continue
		}
		if _, dup := totals[key]; dup || aggregate.Numeric == nil {
			delete(totals, key)
			ambiguous[key] = true
			continue
		}
		totals[key] = new(big.Rat).Set(aggregate.Numeric)
	}
	return totals, ambiguous
}

// ValidateContributionDecisionConsistency is the typed cross-check between
// the decision ledger and the contribution ledger (E-3 neighbor arm, eval
// replay#4 run-1 2026-07-19): a contribution that sums a row the decision
// ledger EXCLUDES is a self-contradiction between two published ledgers and
// must fail loud with a per-row repair detail. Carriers are fully typed
// (RowDecision.RowID/Decision ↔ ContributionRecord.ItemID); rows without a
// matching identifier stay silent (fail-open — no fuzzy joins). Deliberate
// boundary: a pipeline that consistently FORGETS an eligibility rule at
// every layer (decision=include, contribution present, witness GroupA=20
// including the inactive row) is invisible to any precise engine-side
// signal — judging it needs the prose task rules, which is the model's
// domain; this arm reds the reachable class (ledger self-contradiction),
// not the unreachable one.
//
// Identity key (V9-1 §40.15): both sides join on the runner-owned row
// identity when present (RowDecision.RowIdentity ↔ ContributionRecord.
// RowIdentity) so 1:N siblings that share a parent locator may hold
// different decisions; carriers without it join on RowID/ItemID exactly as
// before. A contribution whose derivation ANCESTOR is excluded (parent row
// filtered out, then expanded and summed — B461 class) is still a
// contradiction: the ancestors are enumerated precisely from the identity
// formatting rule (rowIdentityAncestors), never by fuzzy prefix matching.
func ValidateContributionDecisionConsistency(rows []RowDecision, contributions []ContributionRecord) error {
	if len(rows) == 0 || len(contributions) == 0 {
		return nil
	}
	excluded := map[string]RowDecision{}
	for _, row := range rows {
		id := firstNonEmptyString(strings.TrimSpace(row.RowIdentity), strings.TrimSpace(row.RowID))
		if id == "" {
			continue
		}
		if rowDecisionIsExclude(row.Decision) {
			excluded[id] = row
		}
	}
	if len(excluded) == 0 {
		return nil
	}
	for i, rec := range contributions {
		if !contributionParticipatesInReconcile(rec) {
			continue
		}
		identity := strings.TrimSpace(rec.RowIdentity.String())
		id := firstNonEmptyString(identity, strings.TrimSpace(rec.ItemID.String()))
		if id == "" {
			continue
		}
		row, ok := excluded[id]
		ancestorNote := ""
		if !ok && identity != "" {
			for _, ancestor := range rowIdentityAncestors(identity, rec.SourceLocator.String()) {
				if row, ok = excluded[ancestor]; ok {
					ancestorNote = fmt.Sprintf(" (ancestor %s excluded)", ancestor)
					break
				}
			}
		}
		if !ok {
			continue
		}
		return dataValidationError(
			"contribution_excluded_row",
			fmt.Sprintf("/contributions/%d", i),
			"contribution records only for rows the decision ledger includes",
			id,
			RepairabilityNeedsRecompute,
			"data validation incomplete: contribution %d (item_id %q, group %q, value %s) sums a row the decision ledger excludes (decision %q at %s)%s; recompute contributions over included rows only, or correct the decision record",
			i, id, strings.TrimSpace(rec.GroupKey.String()), strings.TrimSpace(rec.Value.String()),
			strings.TrimSpace(row.Decision), strings.TrimSpace(row.SourceLocator), ancestorNote,
		)
	}
	return nil
}

func rowDecisionIsExclude(decision string) bool {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "exclude", "excluded":
		return true
	default:
		return false
	}
}

// DescribeReferenceGroundingMismatches renders the per-slot repair detail the
// repair planner consumes.
func DescribeReferenceGroundingMismatches(report ReferenceGroundingReport) string {
	var parts []string
	if report.CardinalityMismatch {
		parts = append(parts, fmt.Sprintf("answer has %d item(s) but the reference set defines %d key(s)", report.AnswerItemCount, report.ReferenceKeyCount))
	}
	if report.LedgerDomainMismatch {
		parts = append(parts, fmt.Sprintf("no contribution total is keyed by any reference key (field %q); the contribution ledger must be recomputed grouped by the reference key domain before projection", report.Field))
	}
	for _, mismatch := range report.Mismatches {
		detail := fmt.Sprintf("slot %d (key %q) expected %s but answer has %s", mismatch.Slot, mismatch.Key, mismatch.Expected, mismatch.Actual)
		if !mismatch.HasContributions {
			detail += " (no contribution records for this key; it must be 0)"
		}
		parts = append(parts, detail)
	}
	return strings.Join(parts, "; ")
}
