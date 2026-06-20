package types

import (
	"hash/fnv"
	"sort"
)

// DowngradeLane is the typed identity of a pre-complete downgrade gate. It is
// emitted by the gate site, never inferred from the downgrade Summary prose, so
// the low-delta convergence boundary routes on a typed enum rather than reading
// model- or framework-authored text.
type DowngradeLane string

const (
	// DowngradeLaneCurrentSourceLane is the current-source-lane coverage gate.
	DowngradeLaneCurrentSourceLane DowngradeLane = "current_source_lane"
	// DowngradeLaneSourceInventoryLens is the source-inventory lens-execution gate.
	DowngradeLaneSourceInventoryLens DowngradeLane = "source_inventory_lens"
	// DowngradeLaneContractChain is the pre-complete contract-check chain gate
	// (the ~13 sub-checks aggregate into this lane; the typed BlockerKey below
	// distinguishes which sub-blocker is active).
	DowngradeLaneContractChain DowngradeLane = "contract_chain"
)

// DowngradeFingerprint is the typed, comparable identity of a single pre-complete
// downgrade attempt. Two adjacent equal fingerprints mean the model re-attempted
// emit_investigation_complete with the SAME blocking lane and the SAME typed
// blocker (same pending reads / unverified findings / raised repairs) — i.e. no
// progress on the blocker, regardless of incidental evidence/read churn. It is
// deliberately NOT keyed on evidence/read counts: the existing ClosureFingerprint
// already trips on zero-delta and never converges when one junk evidence id ticks
// the count, which is exactly the runaway this fixes.
type DowngradeFingerprint struct {
	Lane       DowngradeLane
	BlockerKey uint32
}

// CompletionCaveat records, in typed form, that a pre-complete downgrade lane was
// force-completed by the convergence boundary without the blocker being resolved.
// It is carried for downstream answer caveat / telemetry consumers.
type CompletionCaveat struct {
	Lane   DowngradeLane
	Reason string
}

// ComputeDowngradeBlockerKey hashes the typed blocker state a pre-complete gate
// is stuck on into a stable uint32. It hashes only typed identifiers — pending
// read origin+file, unverified finding kind+token, raised repair kind+subject —
// never prose Rationale/Reason fields. A blocker that genuinely shrinks (a
// pending read drained, an unverified path resolved) changes the key and resets
// convergence; incidental evidence churn elsewhere does not.
func ComputeDowngradeBlockerKey(pending []PendingRead, unverified []UnverifiedFinding, repairs []RepairDirective) uint32 {
	parts := make([]string, 0, len(pending)+len(unverified)+len(repairs))
	for _, p := range pending {
		parts = append(parts, "pr:"+p.Origin+"|"+p.File)
	}
	for _, u := range unverified {
		parts = append(parts, "uf:"+u.Kind+"|"+u.Token)
	}
	for _, r := range repairs {
		parts = append(parts, "rp:"+string(r.Kind)+"|"+r.Subject)
	}
	sort.Strings(parts)
	h := fnv.New32a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum32()
}
