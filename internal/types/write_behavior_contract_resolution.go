package types

import (
	"sort"
	"strings"
)

// write_behavior_contract_resolution.go — V5-4 (colleague_merge_audit §40.24):
// ONE projection of the behavior-contract id space. Every contract_refs gate,
// the planner/controller framing, and the cumulative-scope shadowing read the
// same post-rebase generation with its tombstones, so a retired id is
// "retired by <evidence>" everywhere and never "unknown" in one gate and valid
// in its sibling. The pre-rebase analyzer snapshot (ir.Request.BehaviorContracts)
// is only ever the INPUT of ProjectWriteBehaviorContractGeneration; no gate
// reads it directly (census: behavior_contract_id_resolution_census_test.go).

// WriteBehaviorContractIDStatus is the three-state answer of a lookup.
type WriteBehaviorContractIDStatus string

const (
	WriteBehaviorContractIDActive  WriteBehaviorContractIDStatus = "active"
	WriteBehaviorContractIDRetired WriteBehaviorContractIDStatus = "retired"
	WriteBehaviorContractIDUnknown WriteBehaviorContractIDStatus = "unknown"
)

// WriteBehaviorContractResolution is the resolved generation: the active
// contracts, the tombstones that shadow retired ids, and the generation marker.
type WriteBehaviorContractResolution struct {
	Contracts  []WriteBehaviorContract
	Tombstones []WriteBehaviorContractTombstone
	Generation WriteBehaviorContractGeneration
}

// Lookup classifies one id. An id that is both active and tombstoned cannot
// occur for a projection (ids are reserved against re-minting); for a
// hand-built plan the tombstone wins, matching ChangePlanVerificationBehaviorContracts.
func (r WriteBehaviorContractResolution) Lookup(id string) (WriteBehaviorContractIDStatus, *WriteBehaviorContractTombstone) {
	id = strings.TrimSpace(id)
	if id == "" {
		return WriteBehaviorContractIDUnknown, nil
	}
	for i := range r.Tombstones {
		if strings.TrimSpace(r.Tombstones[i].ID) == id {
			tombstone := r.Tombstones[i]
			return WriteBehaviorContractIDRetired, &tombstone
		}
	}
	if _, ok := writeBehaviorContractIDs(r.Contracts)[id]; ok {
		return WriteBehaviorContractIDActive, nil
	}
	return WriteBehaviorContractIDUnknown, nil
}

// ActiveIDs is the set of ids a contract_refs entry may name.
func (r WriteBehaviorContractResolution) ActiveIDs() map[string]struct{} {
	ids := writeBehaviorContractIDs(r.Contracts)
	for _, tombstone := range r.Tombstones {
		delete(ids, strings.TrimSpace(tombstone.ID))
	}
	return ids
}

// RetiredIDs is the sorted, deduplicated tombstone id list.
func (r WriteBehaviorContractResolution) RetiredIDs() []string {
	ids := make([]string, 0, len(r.Tombstones))
	for _, tombstone := range r.Tombstones {
		ids = append(ids, tombstone.ID)
	}
	return dedupSortedWriteBehaviorContractIDs(ids)
}

// ProjectWriteBehaviorContractGeneration is THE projection from the analyzer
// snapshot to the active generation. Its inputs are the snapshot, the run's
// tombstone ledger (prior), the verify-failure handoff of the latest failed
// attempt, the plan's acceptance tests and the planner's declared
// supersessions. Without a handoff, a ledger row or a supersession it is a
// copy of base (no tombstones, no generation marker). Otherwise it applies
// RebaseVerifyFailureWriteBehaviorContracts under the typed decision (lane
// from FailureKind, relevance hits, planner supersessions, prior ledger) and
// stamps the plan_acceptance_rebase generation; the returned Tombstones are
// prior ∪ new, so every emitted plan carries the ledger as of its emission.
//
// Outside package types the projection is reached through
// MutableState.ProjectBehaviorContractGeneration, which supplies the ledger
// from state (census: behavior_contract_id_resolution_census_test.go rule e);
// the workflow seed at IR time is the one generation-0 caller of this pure
// function.
func ProjectWriteBehaviorContractGeneration(base []WriteBehaviorContract, prior []WriteBehaviorContractTombstone, handoff *VerifyFailureHandoff, acceptanceTests []string, plannerSupersededIDs []string) WriteBehaviorContractResolution {
	if len(base) == 0 {
		return WriteBehaviorContractResolution{}
	}
	contracts := append([]WriteBehaviorContract(nil), base...)
	decision := WriteBehaviorContractRetirementDecisionFromHandoff(handoff, prior, plannerSupersededIDs)
	if handoff == nil && len(decision.Prior) == 0 && len(decision.PlannerSupersededIDs) == 0 {
		return WriteBehaviorContractResolution{Contracts: contracts}
	}
	rebased, tombstones := RebaseVerifyFailureWriteBehaviorContracts(contracts, acceptanceTests, decision)
	return WriteBehaviorContractResolution{
		Contracts:  rebased,
		Tombstones: tombstones,
		Generation: WriteBehaviorContractGenerationPlanAcceptanceRebase,
	}
}

// ResolveChangePlanBehaviorContractIDs resolves the id space of an emitted
// plan: its own contract snapshot minus tombstoned ids, plus the typed
// tombstones (legacy persisted plans carrying only
// superseded_behavior_contract_ids resolve as retired with empty evidence).
func ResolveChangePlanBehaviorContractIDs(plan *ChangePlan) WriteBehaviorContractResolution {
	if plan == nil {
		return WriteBehaviorContractResolution{}
	}
	tombstones := SupersededWriteBehaviorContractTombstones(plan)
	out := WriteBehaviorContractResolution{Generation: plan.BehaviorContractGeneration}
	for _, contract := range plan.BehaviorContracts {
		if _, retired := tombstones[strings.TrimSpace(contract.ID)]; retired {
			continue
		}
		out.Contracts = append(out.Contracts, contract)
	}
	ids := make([]string, 0, len(tombstones))
	for id := range tombstones {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		out.Tombstones = append(out.Tombstones, tombstones[id])
	}
	return out
}

// SupersededWriteBehaviorContractTombstones unions the typed tombstone carrier
// with the legacy bare-id list (reason empty) keyed by id. It is the one
// tombstone-membership authority for cumulative verification scope and the
// controller's scope stamping.
func SupersededWriteBehaviorContractTombstones(plan *ChangePlan) map[string]WriteBehaviorContractTombstone {
	out := map[string]WriteBehaviorContractTombstone{}
	if plan == nil {
		return out
	}
	for _, tombstone := range plan.SupersededBehaviorContracts {
		id := strings.TrimSpace(tombstone.ID)
		if id == "" {
			continue
		}
		tombstone.ID = id
		out[id] = tombstone
	}
	for _, raw := range plan.SupersededBehaviorContractIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := out[id]; !ok {
			out[id] = WriteBehaviorContractTombstone{ID: id}
		}
	}
	return out
}
