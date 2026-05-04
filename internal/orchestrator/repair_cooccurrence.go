// Package orchestrator — repair_cooccurrence.go
//
// Phase 1-A1 (V2 runtime consolidation, 2026-05-04). Typed
// cooccurrence rules for repair routing root-cause clustering.
//
// PRECISE-SIGNAL CONTRACT (red line R3): every rule below MUST be
// derivable from a typed pipeline invariant — "X is the verified
// PRECONDITION of Y; without X, Y's validator necessarily fires".
// NO frequency analysis, NO similarity scoring, NO grep heuristics.
// If a rule cannot cite a single typed invariant in its Reason, it
// does NOT belong here.
//
// The cluster algorithm (BuildRepairPlan in repair_plan.go) uses
// these rules to identify which violation in a co-occurring set is
// the ORIGINAL failure (Primary) and which are downstream
// CONSEQUENCES (Derived). The router then picks an owner based on
// the Primary alone — fixing the root cause necessarily clears the
// derived consequences in the next attempt.
//
// Pre-Phase 1, the router bucketed by RepairLocus and picked by
// count + deepest-locus tiebreak. That model treats every violation
// as independent, which over-pulls when a single root cause
// generates 3+ derived violations: count + deepest-tiebreak picks a
// deeper locus even though the root is finalize-local.
//
// Phase 1 model: SAME-DISPATCH cooccurrence + typed invariant ⇒
// Derived violations DO NOT contribute to bucket counts; only
// Primaries do. Independent violations (no rule applies) remain
// their own Primary.

package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// CooccurrenceRule asserts: when Primary fires in the SAME dispatch
// as ANY of Derived, Derived violations are downstream consequences
// of Primary's failure mode. The Reason must cite a typed pipeline
// invariant — readers should be able to grep the codebase for the
// invariant and verify the rule's claim.
//
// Same-dispatch is identified by matching DispatchID on the
// Violations (set by violation production sites). When DispatchID is
// empty, fall back to whole-result matching (treat the entire
// validator output as one dispatch).
type CooccurrenceRule struct {
	// Primary is the violation kind that owns the root cause. Its
	// FallbackTargetForKind value determines the cluster's repair
	// owner.
	Primary types.ViolationKind

	// Derived are violation kinds whose validator necessarily fires
	// when Primary fires (because the typed pipeline invariant
	// described in Reason makes Derived's success contingent on
	// Primary's success).
	Derived []types.ViolationKind

	// Reason names the typed invariant that links Primary → Derived.
	// Must be specific enough to grep — avoid "X usually causes Y";
	// say "X is the precondition of Y because Z (pipeline-stage
	// invariant)".
	Reason string
}

// defaultCooccurrenceRules is the canonical rule set. Each rule
// links a typed pipeline invariant — the comment below each rule
// names the invariant + a code/doc reference where it lives.
//
// Rule audit checklist (apply when adding a new rule):
//   1. Can you point to ONE concrete pipeline invariant that makes
//      Derived's success contingent on Primary's success? If no →
//      rule does not belong here (this would be heuristic).
//   2. Is the dependency one-way? If Primary can fire without
//      causing Derived OR Derived can fire without Primary, it's
//      not a true precondition — at most a soft signal.
//   3. Does the Reason text name the invariant in a way a future
//      reader can grep?
//
// Order is significant: rules are evaluated top-to-bottom in
// matchRule, and the first match wins. Place stronger
// (more-restrictive) rules first.
var defaultCooccurrenceRules = []CooccurrenceRule{
	// ─────────────────────────────────────────────────────────────
	// Rule C1 — SubjectAnchor missing ⇒ step/symbol verification fails.
	//
	// Invariant: validateStepIdentifierUnverified and
	// validateSymbolAnchorMismatch both check that the step's anchor
	// symbol resolves to a typed Subject/Object/AnchorSymbol field
	// in some EvidenceItem. ViolSubjectAnchorMissing means the
	// extractor never wrote any such typed anchor for the subject;
	// the step/symbol verifier therefore has no typed pool to match
	// against and necessarily emits both Derived violations.
	// Source: contract_check.go validateStepIdentifierUnverified
	// (typed pool = EvidenceItem.AnchorSymbol/Subject/Object).
	{
		Primary: types.ViolSubjectAnchorMissing,
		Derived: []types.ViolationKind{
			types.ViolStepIdentifierUnverified,
			types.ViolSymbolAnchorMismatch,
		},
		Reason: "step/symbol verifiers require a typed Subject/Object/AnchorSymbol pool entry; SubjectAnchorMissing means the pool is empty for this subject",
	},

	// ─────────────────────────────────────────────────────────────
	// Rule C2 — Block coverage missing ⇒ block-internal validators
	// have no payload to inspect.
	//
	// Invariant: validatePrincipalClaimUse, validateDiagramEdgeSupport,
	// and validateUncertaintyBlockPresence all walk doc.Blocks and
	// fire when the required block KIND is not present. If the block
	// itself is missing (ViolBlockCoverageMissing fires for that
	// kind), every block-internal validator firing on the same kind
	// is a Derived consequence — they cannot succeed without the
	// block to inspect.
	// Source: contract_check_block.go validateRequiredBlockCoverage
	// + validatePrincipalClaimUse + validateDiagramEdgeSupport.
	{
		Primary: types.ViolBlockCoverageMissing,
		Derived: []types.ViolationKind{
			types.ViolPrincipalClaimUseMissing,
			types.ViolDiagramEdgeUnsupported,
			types.ViolUncertaintyBlockMissing,
		},
		Reason: "block-internal validators (claim_use/diagram_edge/uncertainty) walk doc.Blocks; a missing block has no payload for them to inspect, so they necessarily fire",
	},

	// ─────────────────────────────────────────────────────────────
	// Rule C3 — Facet uncovered ⇒ multi-derived: principal
	// claim_use anchor target empty + richness regression on same
	// facet + diagram edge unsupported on same dispatch.
	//
	// Invariant: validatePrincipalClaimUse requires each principal
	// block's claim_use[].facet_id reference an entry in
	// view.FacetCoverage.Required; FacetUncovered means no facet_id
	// match → claim_use anchor empty.
	// validateRichnessRegression (Phase 5 SOFT) fires on the SAME
	// uncovered facet when SourceCandidate evidence exists.
	// Phase 3-C5 validateDiagramEdgeSupport Layer 2 fires when the
	// diagram-related facet is uncovered and the diagram block's
	// edges lack typed claim_use anchors.
	//
	// V2 runtime eval Phase 1-C (2026-05-04): pre-fix, the latter
	// two were singletons → cluster count inflated → SOFT
	// RichnessRegression's FailLoud safety-net mapping flipped the
	// whole plan to fail_loud (m1a-2 / u3a-1 outlier root cause).
	// Now they cluster under FacetUncovered as a single repair
	// target.
	//
	// Source: contract_check_block.go validateFacetCoverage +
	// validatePrincipalClaimUse + validateRichnessRegression +
	// validateDiagramRelationLegality.
	{
		Primary: types.ViolFacetUncovered,
		Derived: []types.ViolationKind{
			types.ViolPrincipalClaimUseMissing,
			types.ViolRichnessRegression,
			types.ViolDiagramEdgeUnsupported,
			// Phase 1-C: AuthorityOverreach + ClaimFormUnsupported
			// also key on the typed evidence whose facet is
			// uncovered — when the answer drops the facet, downstream
			// authority/claim_form gates often fire on the same
			// missing surface. Cluster reduces the same root cause
			// to one repair target.
			types.ViolAuthorityOverreach,
			types.ViolClaimFormUnsupported,
		},
		Reason: "principal claim_use, richness regression, diagram edge support, authority overreach, and claim_form support all key on FacetCoverage typed evidence; an uncovered facet collapses these downstream signals into one repair target — re-cover the facet to clear them at once",
	},

	// ─────────────────────────────────────────────────────────────
	// Rule C4 — Predicate axis missing ⇒ axis-typed facets cannot be
	// covered.
	//
	// Invariant: PredicateAxisMissing fires when the analyzer's
	// AnswerSemanticView demands an axis (e.g. enumeration axis,
	// comparison axis) that the EvidenceItem pool does not carry.
	// The axis-typed facets in FacetCoverage.Required.SourceCandidate
	// would be empty in that case, guaranteeing ViolFacetUncovered
	// for those facets.
	// Source: types/answer_semantic_view.go AnswerSemanticView.PredicateAxis
	// + types/facet_plan.go FacetRequirement.SourceCandidate.
	{
		Primary: types.ViolPredicateAxisMissing,
		Derived: []types.ViolationKind{
			types.ViolFacetUncovered,
		},
		Reason: "axis-typed facets draw their SourceCandidate pool from the predicate axis; a missing axis leaves the pool empty, guaranteeing FacetUncovered downstream",
	},

	// ─────────────────────────────────────────────────────────────
	// Rule C5 — Citation / GhostAnchor errors ⇒ step verification
	// and enumeration grounding fail.
	//
	// Invariant: validateStepIdentifierUnverified and
	// validateEnumerationLabelGrounded both treat citation entries
	// as the ground-truth pool for matching anchor text. A citation
	// that does not ground its claim (ViolCitation) or that points
	// to no real anchor (ViolGhostAnchor) means the matchers run
	// against a corrupted pool — Derived's validators necessarily
	// fail to find their evidence even though the LLM emitted them.
	// Source: contract_check.go validateStepIdentifierUnverified
	// (citation pool) + validateEnumerationLabelGrounded (label
	// substring-match against citation pool).
	{
		Primary: types.ViolCitation,
		Derived: []types.ViolationKind{
			types.ViolStepIdentifierUnverified,
			types.ViolEnumerationLabelUngrounded,
		},
		Reason: "step verification and enumeration grounding both match against citation pool entries; a corrupted citation guarantees the downstream matchers fail",
	},
	{
		Primary: types.ViolGhostAnchor,
		Derived: []types.ViolationKind{
			types.ViolStepIdentifierUnverified,
			types.ViolEnumerationLabelUngrounded,
		},
		Reason: "ghost anchors corrupt the citation pool the same way citations do; downstream pool-matching validators necessarily fail",
	},

	// ─────────────────────────────────────────────────────────────
	// Rule C6 — Declared count drift ⇒ enumeration labels lose
	// alignment.
	//
	// Invariant: validateEnumerationLabelGrounded matches each
	// declared enumeration item label against an EvidenceItem
	// AnchorSymbol/Subject/Object substring. Count drift means the
	// extractor emitted N declared symbols but the renderer surfaced
	// M ≠ N items; the surplus items have no extractor-side anchor
	// and necessarily fail label grounding.
	// Source: contract_check.go validateDeclaredCountDrift +
	// validateEnumerationLabelGrounded (extractor-side anchor pool).
	{
		Primary: types.ViolDeclaredCountDrift,
		Derived: []types.ViolationKind{
			types.ViolEnumerationLabelUngrounded,
			types.ViolEnumerationItemLabelExtractorDrift,
		},
		Reason: "enumeration labels are grounded against the extractor's declared symbol slate; a count drift means surplus items have no extractor anchor, guaranteeing UngroundedLabel and ExtractorDrift downstream",
	},

	// ─────────────────────────────────────────────────────────────
	// Rule C6.1 — Extractor drift ⇒ pool-substring grounding can
	// still pass (s1a-20260504-130143 abstraction-drift forensic).
	//
	// Invariant: validateEnumerationItemLabelExtractorMatch checks
	// items[].label preserves extractor's VERBATIM identifiers, which
	// is stricter than validateEnumerationLabelGrounding's substring
	// match against evidence pool. When ExtractorDrift fires alone,
	// LabelUngrounded does NOT necessarily fire (placeholder labels
	// like "check 1" still match the substring "check" in evidence).
	// They share root cause (finalizer abstracted away identifiers),
	// but Drift is the precise signal — cluster them so the repair
	// targets the renderer once.
	// Source: contract_check_block.go
	// validateEnumerationItemLabelExtractorMatch +
	// validateEnumerationItemLabelGrounding.
	{
		Primary: types.ViolEnumerationItemLabelExtractorDrift,
		Derived: []types.ViolationKind{
			types.ViolEnumerationLabelUngrounded,
		},
		Reason: "extractor-drift and ungrounded-label both fire when finalizer abstracted enumeration identifiers; cluster so a single rewrite restores verbatim names",
	},

	// ─────────────────────────────────────────────────────────────
	// Rule C7 — Chain demoted ⇒ step verifier sees no real def-line.
	//
	// Invariant: ChainDemoted fires when no anchor in a resolution
	// chain reaches a typed definition site (def-line read).
	// validateStepIdentifierUnverified treats a chain step as
	// verified iff at least one of its anchors has a def-line in
	// the EvidenceItem pool. A demoted chain has no such anchor,
	// guaranteeing the step verifier fails for every step in the
	// chain.
	// Source: types/evidence.go ResolutionChain (def-line invariant)
	// + contract_check.go validateStepIdentifierUnverified.
	{
		Primary: types.ViolChainDemoted,
		Derived: []types.ViolationKind{
			types.ViolStepIdentifierUnverified,
			types.ViolPrincipalClaimUseMissing,
		},
		Reason: "step verification and principal claim_use both rely on chain anchors reaching def-lines; a demoted chain has none, guaranteeing both downstream validators fail",
	},
}

// matchRule returns the first rule whose Primary appears in vs AND
// whose Derived list intersects vs. Returns (rule, true) on match,
// (zero, false) otherwise.
//
// The matcher is order-stable: rules earlier in defaultCooccurrenceRules
// take precedence (so when a violation could be the Derived of two
// different Primaries, the first declared rule wins).
func matchRule(vs []types.Violation, rules []CooccurrenceRule) (CooccurrenceRule, bool) {
	if len(vs) == 0 {
		return CooccurrenceRule{}, false
	}
	kinds := map[types.ViolationKind]bool{}
	for _, v := range vs {
		kinds[v.Kind] = true
	}
	for _, r := range rules {
		if !kinds[r.Primary] {
			continue
		}
		for _, d := range r.Derived {
			if kinds[d] {
				return r, true
			}
		}
	}
	return CooccurrenceRule{}, false
}

// dispatchKey returns the same-dispatch grouping key for a Violation.
// Empty DispatchID falls back to a sentinel so violations with no
// dispatch tag still cluster together (preserves pre-A1 behaviour
// for legacy callers / unit tests).
func dispatchKey(v types.Violation) string {
	if v.DispatchID != "" {
		return v.DispatchID
	}
	return "_no_dispatch_"
}

// groupByDispatch partitions violations by DispatchID. Each group is
// independently fed to matchRule + cluster classification.
func groupByDispatch(vs []types.Violation) map[string][]types.Violation {
	out := map[string][]types.Violation{}
	for _, v := range vs {
		k := dispatchKey(v)
		out[k] = append(out[k], v)
	}
	return out
}
