package types

import (
	"hash/fnv"
	"sort"
	"strconv"
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
	// DowngradeLaneSourceInventoryCompletion is the source-inventory completion
	// gate: the lens ran, but the typed inventory universe is still incomplete
	// or budget-truncated for a resolved exhaustive answer.
	DowngradeLaneSourceInventoryCompletion DowngradeLane = "source_inventory_completion"
	// DowngradeLaneSourceClassUniverse is the source-class-universe absence gate:
	// a source-family exact absence declared while the typed source-class
	// universe (production/test/fixture/corpus/thirdparty/...) is still open.
	DowngradeLaneSourceClassUniverse DowngradeLane = "source_class_universe"
	// DowngradeLanePathDiscoveryAbsence is the file-family absence proof gate:
	// a typed inventory/file-family zero answer still lacks recursive path
	// discovery or an authoritative source_inventory lens. It can force-complete
	// only as a caveated answer after low-delta convergence.
	DowngradeLanePathDiscoveryAbsence DowngradeLane = "path_discovery_absence"
	// DowngradeLaneExactAbsenceContext is the exact-absence related-context
	// gate: the target absence is plausible, but the same-scope context anchor
	// or precedence role evidence has not been materialized. It is a repair lane,
	// not an infinite hard reject.
	DowngradeLaneExactAbsenceContext DowngradeLane = "exact_absence_context"
	// DowngradeLaneExactResolvedDefiningProof is the positive exact-resolution
	// gate: the model claims result_kind=resolved for an exact target, but the
	// typed evidence pool does not yet contain a grounded defining proof for that
	// exact target. It should delay closure and request proof, then caveat after
	// repeated no-progress attempts instead of tool-failing forever.
	DowngradeLaneExactResolvedDefiningProof DowngradeLane = "exact_resolved_defining_proof"
	// DowngradeLanePrincipalMemberSetHandoff is the principal/per-member-table
	// handoff gate: the answer shape requires an exact member_set carrier, but
	// the current completion attempt omitted it. The requirement is precise, but
	// repeated no-progress attempts should still converge with a typed caveat
	// instead of leaving the model in a form-retry loop.
	DowngradeLanePrincipalMemberSetHandoff DowngradeLane = "principal_member_set_handoff"
	// DowngradeLaneGroundingCitationFloor is the configured grounding / Tier-1
	// citation-floor gate. The floor is based on typed evidence grounding
	// verdicts, but it still must not become an infinite hard loop when the same
	// proof-strength blocker recurs without progress.
	DowngradeLaneGroundingCitationFloor DowngradeLane = "grounding_citation_floor"
	// DowngradeLaneCompletionForm is the framework-owned completion-form lane:
	// the model has attempted to close investigation, but the structured
	// handoff has a repairable presentation/form defect such as member-set
	// support_refs alignment. The defect may block an uncaveated strong answer,
	// but it must not tool-fail forever or reopen broad exploration.
	DowngradeLaneCompletionForm DowngradeLane = "completion_form"
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
	Lane       DowngradeLane
	ReasonCode string
	Reason     string
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
		// The advisory bit joins the hashed identity: the flip is
		// monotonic, so it changes the key exactly once — a deliberate
		// state change, not churn.
		parts = append(parts, "uf:"+u.Kind+"|"+u.Token+"|adv="+strconv.FormatBool(u.Advisory))
	}
	for _, r := range repairs {
		parts = append(parts, "rp:"+string(r.Kind)+"|"+r.Subject+"|lane="+string(r.DowngradeLane))
	}
	sort.Strings(parts)
	h := fnv.New32a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum32()
}
