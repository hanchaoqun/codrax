package types

import (
	"sort"
	"strings"
)

// write_behavior_contract_ledger.go — §40.46 (fold-in of the V5-3/V5-4 review):
// contract retirement is MONOTONIC within a run. A tombstone minted by any
// generation (verification evidence, planner supersession, fallback rebase)
// is recorded in one per-run ledger that survives every handoff replacement
// and every per-round planning-state reset; the projection of the next
// generation takes the ledger as input (prior tombstones ∪ this handoff's),
// so a retired id can never become active again because a later attempt
// failed for an unrelated reason. The only reinstatement lane is a NEW
// analyzer IR for a new request (emit_write_analysis resets the ledger).
//
// The ledger is durable across processes through two typed carriers that are
// merged back whenever they are installed on MutableState: every emitted
// ChangePlan carries the ledger as of its emission (SupersededBehaviorContracts)
// and the WriteWorkflowRun envelope carries it on every persist.

// WriteBehaviorContractTombstoneLedger is the per-run, id-keyed, append-only
// retirement record. The zero value is an empty ledger.
type WriteBehaviorContractTombstoneLedger struct {
	rows map[string]WriteBehaviorContractTombstone
}

// Merge records every tombstone whose id is not yet retired. The first
// retirement of an id is the audited one (its evidence authorized the
// action); a later row for the same id never overwrites it. It returns the
// number of ids newly retired.
func (l *WriteBehaviorContractTombstoneLedger) Merge(tombstones ...WriteBehaviorContractTombstone) int {
	if l == nil {
		return 0
	}
	added := 0
	for _, tombstone := range tombstones {
		id := strings.TrimSpace(tombstone.ID)
		if id == "" {
			continue
		}
		if l.rows == nil {
			l.rows = map[string]WriteBehaviorContractTombstone{}
		}
		if _, ok := l.rows[id]; ok {
			continue
		}
		tombstone.ID = id
		tombstone.EvidenceRefs = dedupSortedWriteBehaviorContractIDs(tombstone.EvidenceRefs)
		l.rows[id] = tombstone
		added++
	}
	return added
}

// Rows returns the tombstones sorted by id (a copy).
func (l *WriteBehaviorContractTombstoneLedger) Rows() []WriteBehaviorContractTombstone {
	if l == nil || len(l.rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(l.rows))
	for id := range l.rows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]WriteBehaviorContractTombstone, 0, len(ids))
	for _, id := range ids {
		row := l.rows[id]
		row.EvidenceRefs = append([]string(nil), row.EvidenceRefs...)
		out = append(out, row)
	}
	return out
}

// Len is the number of retired ids.
func (l *WriteBehaviorContractTombstoneLedger) Len() int {
	if l == nil {
		return 0
	}
	return len(l.rows)
}

// Reset empties the ledger. Only the new-analyzer-IR lane may call it.
func (l *WriteBehaviorContractTombstoneLedger) Reset() {
	if l == nil {
		return
	}
	l.rows = nil
}

// MergeWriteBehaviorContractTombstones is the pure union used by carriers
// (plan / run) that hold the ledger as a sorted slice: existing rows win,
// new ids are appended, the result is sorted by id.
func MergeWriteBehaviorContractTombstones(existing []WriteBehaviorContractTombstone, incoming ...WriteBehaviorContractTombstone) []WriteBehaviorContractTombstone {
	var ledger WriteBehaviorContractTombstoneLedger
	ledger.Merge(existing...)
	ledger.Merge(incoming...)
	return ledger.Rows()
}

// PlannerSupersedableWriteBehaviorContract is the ONE predicate behind the
// planner supersession lane: the validator accepts a superseded_contract_refs
// id iff this returns ok, and the rebase tombstones an accepted id iff this
// returns ok — accept-set == retire-set by construction (§40.46 C2/C5). The
// second result names the refusing row class for the rejection wording.
// Supersedable: fallback rows, planning-only rows, and ungrounded soft
// expected rows. Never: observed facts, hard-required contracts, or rows with
// any evidence_ref.
func PlannerSupersedableWriteBehaviorContract(contract WriteBehaviorContract) (ok bool, refusingClass string) {
	if IsExpectedOutcomeFallbackWriteBehaviorContract(contract) || IsPlanningOnlyWriteBehaviorContract(contract) {
		return true, ""
	}
	switch {
	case contract.Polarity == WriteBehaviorPolarityObserved:
		return false, "observed"
	case IsHardRequiredWriteBehaviorContract(contract):
		return false, "hard required"
	case isUngroundedSoftExpectedWriteBehaviorContract(contract):
		return true, ""
	case !contract.Required:
		// Unreachable for normalized IR (Required=false ⇒ observed or
		// planning-only, both handled above); a hand-built row without
		// requirement authority and without evidence carries no obligation
		// and may be superseded like any soft row.
		return !writeBehaviorContractHasEvidence(contract), "evidence-grounded"
	default:
		return false, "evidence-grounded"
	}
}

func writeBehaviorContractHasEvidence(contract WriteBehaviorContract) bool {
	if strings.TrimSpace(contract.EvidenceRef) != "" {
		return true
	}
	if contract.Comparator != nil && strings.TrimSpace(contract.Comparator.EvidenceRef) != "" {
		return true
	}
	if contract.Transition != nil {
		for _, step := range contract.Transition.Steps {
			if strings.TrimSpace(step.EvidenceRef) != "" {
				return true
			}
		}
	}
	return false
}
