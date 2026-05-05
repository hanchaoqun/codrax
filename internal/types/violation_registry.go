package types

// violation_registry.go — B0 (v3 runtime consolidation, 2026-05-04).
//
// ViolKindRegistry is the single source of truth for the runtime
// classification of every ViolationKind. Adding a new kind requires
// editing ONE entry here; the legacy DeriveSeverity switch,
// defaultSoftKinds map, DefaultFallbackPolicy map, and inferViolationLayer
// switch all derive from this registry during the v3 migration window.
//
// ── Why this exists ──
//
// Pre-v3, adding a new ViolationKind required editing ≥6 sites:
//   1. internal/types/violation.go            — constant declaration
//   2. internal/types/violation.go            — AllViolationKinds slice
//   3. internal/types/retry_state.go          — DeriveSeverity switch
//   4. internal/orchestrator/contract_check.go — defaultSoftKinds map
//   5. internal/orchestrator/fallback_policy.go — DefaultFallbackPolicy map
//   6. internal/orchestrator/retry_state.go   — inferViolationLayer switch
//
// Six sites, each easy to forget. The 6th-spot-sync red line
// (feedback_typed_signal_six_spot_sync.md) flagged this class of drift
// after V2 runtime eval forensic showed cooccurrence rule omissions
// burning retry budget. v3 collapses the registry surface so a new kind
// declares its full spec once.
//
// ── Migration window ──
//
// During v3 rollout the legacy DeriveSeverity / defaultSoftKinds /
// DefaultFallbackPolicy / inferViolationLayer functions retain their
// hardcoded literals AND consult the registry. The structural test
// TestRegistryDerivesAllLegacyTables asserts byte-identical agreement
// between registry-derived and legacy-literal output for every kind in
// AllViolationKinds(). Once B1-B6 land and no caller is observed
// depending on the legacy literals, a follow-up PR deletes them.
//
// ── Contract ──
//
//   - ViolKindSpec is operator-facing internal data. Description is for
//     telemetry / dashboards / commit messages — NEVER rendered to LLM
//     prompts (R4 red line).
//   - DefaultSeverity drives ViolationProfileFor().Severity. Promotion
//     via pipeline_contract_strict_kinds yaml bumps Soft → Medium
//     uniformly (matches existing isStrict path in DeriveSeverity).
//   - SoftByDefault drives membership in defaultSoftKinds(). The two
//     fields are independent: a kind may be Severity=Medium and also
//     SoftByDefault=true (existing pattern for ViolFacetUncovered:
//     Severity Medium per R14 but in the soft map per Phase-4 default).
//   - Promotable=false means operator yaml CANNOT promote this kind to
//     STRICT. Currently the only such kind is ViolDiagramEdgeLabelMismatch
//     (label inference is permanently noisy per R3). Honoured by
//     SetSoftViolationKinds.
//   - FallbackLocus drives DefaultFallbackPolicy(); the policy map then
//     derives FallbackTarget via targetForLocus.
//   - Layer drives inferViolationLayer; consumed by RetryState rendering
//     to group violations by producer.

import (
	"fmt"
	"sync"
)

// ViolKindSpec captures the runtime classification of one
// ViolationKind. Treated as an immutable record — registration is
// append-only at package init time; runtime mutation is not
// supported.
type ViolKindSpec struct {
	// Kind is the typed identifier. Required.
	Kind ViolationKind

	// DefaultSeverity is the Severity assigned by ViolationProfileFor
	// when isStrict=false. The strict-promotion path (operator yaml)
	// bumps Soft → Medium following the existing DeriveSeverity rule.
	DefaultSeverity Severity

	// SoftByDefault, when true, places this kind in defaultSoftKinds()
	// — meaning hasAnyStrictViolation skips it and the gate does NOT
	// flip Passed=false on this kind alone. Independent from Severity:
	// some Medium-severity kinds are SoftByDefault (e.g.
	// ViolFacetUncovered) because the right repair needs new evidence
	// rather than a finalize retry.
	SoftByDefault bool

	// Promotable=false means operator yaml pipeline_contract_strict_kinds
	// CANNOT lift this kind out of soft. Used for permanently-noisy
	// signals (label inference, frequency bridges). Default true.
	Promotable bool

	// FallbackLocus is the stage that owns the fix. DefaultFallbackPolicy()
	// derives the FallbackTarget via targetForLocus(locus). Special
	// loci:
	//   - LocusFinalizer: re-emit the answer document
	//   - LocusExtract:   re-pick citations / re-emit symbol slate
	//   - LocusExplore:   re-investigate (need new evidence)
	//   - LocusTerminal:  fail-loud (non-recoverable through retry)
	FallbackLocus RepairLocus

	// Layer is the producer-stage label rendered by RetryState
	// "Active Violations (typed, by severity + layer)" section. One of:
	//   "v2_oracle" / "self_consistency" / "semantic_quality" /
	//   "evidence_pool" / "external_artifact" / "authority" / "cgec" /
	//   "reviewer" / "answer_oracle" / "contract_check"
	// Unknown / unspecified maps to "contract_check".
	Layer string

	// Description is the operator-facing one-line summary. NEVER
	// rendered into LLM-facing prompts (R4 red line). May be empty.
	Description string
}

// RepairLocus is declared in internal/orchestrator/fallback_policy.go;
// the registry references it via a string alias here to avoid a
// cyclic import (orchestrator → types → orchestrator). The orchestrator
// package's RepairLocus values match these constants exactly; the
// translation is verified by TestRegistryDerivesAllLegacyTables.
type RepairLocus string

const (
	LocusFinalizer RepairLocus = "finalizer"
	LocusExtract   RepairLocus = "extract"
	LocusExplore   RepairLocus = "explore"
	LocusTerminal  RepairLocus = "terminal"
)

// IsValid reports whether l is a known RepairLocus.
func (l RepairLocus) IsValid() bool {
	switch l {
	case LocusFinalizer, LocusExtract, LocusExplore, LocusTerminal:
		return true
	}
	return false
}

var (
	violRegistryMu sync.RWMutex
	violRegistry   = map[ViolationKind]ViolKindSpec{}
	// violRegistryOrder preserves declaration order for AllViolKindSpecs.
	violRegistryOrder []ViolationKind
)

// RegisterViolKind appends a spec to the registry. Idempotent: a
// second call with the same Kind overwrites the previous spec
// (used by tests; production registers once at package init).
//
// Required fields: Kind, DefaultSeverity, FallbackLocus.
// Promotable defaults to true when not explicitly set by the caller
// (Go zero value is false; see RegisterViolKindStrict for explicit-only).
func RegisterViolKind(spec ViolKindSpec) {
	if spec.Kind == "" {
		panic("RegisterViolKind: Kind required")
	}
	if !spec.DefaultSeverity.IsValid() {
		panic(fmt.Sprintf("RegisterViolKind(%q): invalid DefaultSeverity %q", spec.Kind, spec.DefaultSeverity))
	}
	if !spec.FallbackLocus.IsValid() {
		panic(fmt.Sprintf("RegisterViolKind(%q): invalid FallbackLocus %q", spec.Kind, spec.FallbackLocus))
	}
	violRegistryMu.Lock()
	defer violRegistryMu.Unlock()
	if _, existed := violRegistry[spec.Kind]; !existed {
		violRegistryOrder = append(violRegistryOrder, spec.Kind)
	}
	violRegistry[spec.Kind] = spec
}

// ViolKindSpecFor returns the registered spec for kind, or
// (zero, false) when no spec exists. Lookups are read-locked; safe
// under concurrent access.
func ViolKindSpecFor(kind ViolationKind) (ViolKindSpec, bool) {
	violRegistryMu.RLock()
	defer violRegistryMu.RUnlock()
	spec, ok := violRegistry[kind]
	return spec, ok
}

// AllViolKindSpecs returns every registered spec in declaration
// order. Used by structural tests to sweep the registry. Returns a
// fresh slice; safe to mutate.
func AllViolKindSpecs() []ViolKindSpec {
	violRegistryMu.RLock()
	defer violRegistryMu.RUnlock()
	out := make([]ViolKindSpec, 0, len(violRegistryOrder))
	for _, k := range violRegistryOrder {
		out = append(out, violRegistry[k])
	}
	return out
}

// IsViolKindPromotable reports whether the registered kind allows
// operator yaml to promote it to STRICT. Defaults to true for
// unregistered kinds (back-compat — pre-registry the only opt-out
// was the SeveritySoft permanent path, which Promotable=false
// emulates).
func IsViolKindPromotable(kind ViolationKind) bool {
	if spec, ok := ViolKindSpecFor(kind); ok {
		return spec.Promotable
	}
	return true
}

// init registers the canonical spec for every ViolationKind declared
// in violation.go. The order mirrors AllViolationKinds() for
// telemetry stability. Adding a new kind: declare the constant in
// violation.go, append to AllViolationKinds, register here.
func init() {
	// ── Original contract-checker kinds ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolFamilyMismatch, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "contract_check",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolCitation, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "contract_check",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolMustInclude, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "contract_check",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolMustExclude, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "contract_check",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolAcceptance, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "contract_check",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSuccessCriterion, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "contract_check",
	})

	// ── Session 11 enforcer kinds ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolGhostAnchor, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "cgec",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolChainDemoted, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "cgec",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSelfRefLiteral, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "cgec",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolPreCompleteDowngrade, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "cgec",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolLiteralFormFailed, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "cgec",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolViewSwap, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "cgec",
	})

	// ── Commit 53 P2 read-mode shape oracle ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolViewIntentMismatch, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "answer_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSubTopicCountMismatch, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "answer_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDiagramIdentifier, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDeclaredCountDrift, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSelfContradiction, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "self_consistency",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolExternalArtifactUnderdecoded, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "external_artifact",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolAuthorityOverreach, DefaultSeverity: SeverityCritical,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "authority",
	})

	// ── Block 1 reviewer kinds (telemetry-only) ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolPlanCritic, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusTerminal,
		Layer: "reviewer",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolReflectorObservation, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusTerminal,
		Layer: "reviewer",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolAnswerReviewerDistilled, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusTerminal,
		Layer: "reviewer",
	})

	// ── Block 2 Intent / Subject / PredicateAxis oracle ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolIntentTraceShallow, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "answer_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolIntentEnumerateNotList, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "answer_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolIntentRootCauseNoCause, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "answer_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolIntentConfigNoTrail, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "answer_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSubjectAnchorMissing, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "answer_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolPredicateAxisMissing, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "answer_oracle",
	})

	// ── Phase 4 Semantic Surface Contract ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolFacetUncovered, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolClaimFormUnsupported, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolAbsenceScopeExceeded, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolStepIdentifierUnverified, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "answer_oracle",
	})
	// Phase 5 — telemetry-only, permanently SOFT.
	RegisterViolKind(ViolKindSpec{
		Kind: ViolRichnessRegression, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: false, FallbackLocus: LocusTerminal,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolValueSecondaryCitationOffFocus, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExtract,
		// Legacy inferViolationLayer groups this under answer_oracle
		// alongside other Block 2 oracle kinds; preserved verbatim
		// for migration parity. v3 follow-up may regroup.
		Layer: "answer_oracle",
	})

	// ── B4 V2 block-only carrier validators ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolBlockCoverageMissing, DefaultSeverity: SeverityCritical,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolPrincipalClaimUseMissing, DefaultSeverity: SeverityCritical,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDiagramEdgeUnsupported, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDiagramEdgeLabelMismatch, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: false, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolUncertaintyBlockMissing, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle",
	})

	// ── G5 / 修 B post_v2_runtime_gap_remediation ──
	RegisterViolKind(ViolKindSpec{
		Kind: ViolAnswerSemanticUnderfilled, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "semantic_quality",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolEnumerationEvidenceUnderspecified, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "evidence_pool",
	})

	// ── B3 v3 (2026-05-04) — diagram relation typed-first ──
	// SOFT-by-default; promotable. Encourages typed relation_kind
	// declaration when label-only inference would fill the contract
	// minimum.
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDiagramRelationLabelOnly, DefaultSeverity: SeveritySoft,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle",
	})

	// ── B2 v3 (2026-05-04) — three-layer quality contract ──
	// Severity=Medium so RetryEligible=true; SoftByDefault=false so
	// the gate flips Passed=false. Operators may demote via
	// pipeline_contract_soft_kinds when noise rate is too high.
	RegisterViolKind(ViolKindSpec{
		Kind: ViolRichnessGlaringGap, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolPrincipalProseUnderfilled, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "v2_oracle",
	})

	// ── Structural / forensic kinds ──
	// Legacy defaultSoftKinds does NOT list StructuralEnumerationDivergence;
	// the kind's SOFT semantic comes from its DefaultSeverity flowing
	// through ViolationProfileFor. Preserved verbatim for migration parity.
	RegisterViolKind(ViolKindSpec{
		Kind: ViolStructuralEnumerationDivergence, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolCrossCitationConflict, DefaultSeverity: SeverityMedium,
		SoftByDefault: true, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "v2_oracle",
	})

	// ── R10 CGEC frequency bridges (telemetry-only, permanently SOFT) ──
	// Legacy defaultSoftKinds does NOT list these; their SOFT classification
	// comes from DefaultSeverity. Preserved verbatim.
	RegisterViolKind(ViolKindSpec{
		Kind: ViolDemotionStorm, DefaultSeverity: SeveritySoft,
		SoftByDefault: false, Promotable: false, FallbackLocus: LocusTerminal,
		// Legacy inferViolationLayer falls through to "contract_check".
		Layer: "contract_check",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolForcedReadStorm, DefaultSeverity: SeveritySoft,
		SoftByDefault: false, Promotable: false, FallbackLocus: LocusTerminal,
		Layer: "contract_check",
	})

	// ── P1 / forensic anchors ──
	// Legacy defaultSoftKinds does NOT list SymbolAnchorMismatch
	// explicitly; the kind is treated as Medium retry-eligible. Layer
	// is v2_oracle in the legacy switch.
	RegisterViolKind(ViolKindSpec{
		Kind: ViolSymbolAnchorMismatch, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExplore,
		Layer: "v2_oracle",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolEnumerationLabelUngrounded, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusExtract,
		Layer: "contract_check",
	})
	RegisterViolKind(ViolKindSpec{
		Kind: ViolEnumerationItemLabelExtractorDrift, DefaultSeverity: SeverityMedium,
		SoftByDefault: false, Promotable: true, FallbackLocus: LocusFinalizer,
		Layer: "contract_check",
	})
}
