package types

import (
	"encoding/json"
)

// retry_state.go — R14 typed retry-state contract
// (post_shape_residual_audit.md, 2026-05-04). Replaces the
// scattered R6 / R6.1 / R11 / R13 patches with a single typed
// surface evaluator render to LLM on retry attempts.
//
// Design rationale (R14_unified_retry_state_design.md §1):
// V2 retry was stateless. LLM saw only "fix X" prose hints and
// regenerated the full payload from scratch — leading to the
// observed retry 失忆 patterns (m1a r1 finalizer iter=0 had 10
// claim_use, iter=7 had 0; block-level claim_use dropped while
// item-level preserved). RetryState gives the LLM three things
// at once:
//
//   1. A typed summary of its previous emit (what fields it
//      already filled and which block ids carry which annotations).
//   2. The full set of active violations across the three gating
//      layers (scheduler / V2 oracle / contract.Check), grouped by
//      Severity so the LLM knows which to fix first.
//   3. A precise "Required Changes" list keyed by field path so
//      the LLM has a concrete diff target.
//
// The Hard Rule rendered on top is the load-bearing invariant:
// every field NOT in "Required Changes" MUST appear byte-identical
// to the previous emit. This stops the regenerate-from-scratch
// failure mode at the prompt level, before any V2 patch / draft-
// retention protocol is wired.

// Severity classifies a violation by how aggressive its
// retry treatment should be. Replaces the implicit
// strict/soft binary that pipeline_contract_strict_kinds /
// defaultSoftKinds together encoded — Severity is the
// single SOURCE OF TRUTH that scheduler retry budget,
// composer hint priority, and RetryState rendering all
// consume.
type Severity string

const (
	// SeverityCritical:violation MUST be fixed for the answer to ship.
	// Fail-loud on retry budget exhaustion. Examples:
	// principal_claim_use_missing on a principal block with non-empty
	// AcceptableClaimForms; block_coverage_missing for a Required block
	// kind.
	SeverityCritical Severity = "critical"

	// SeverityHigh: violation strict-by-default. Eligible for retry
	// loop but does not fail-loud beyond budget. Examples:
	// facet_uncovered, diagram_edge_unsupported, uncertainty_block_missing.
	SeverityHigh Severity = "high"

	// SeverityMedium: violation strict by classification but commonly
	// recoverable in a single retry. Examples:
	// claim_form_unsupported (single-block-level rewrite),
	// declared_count_drift, self_contradiction.
	SeverityMedium Severity = "medium"

	// SeveritySoft: telemetry-only kind. Never blocks shipping; LLM
	// is informed but not required to fix. Examples:
	// richness_regression, reflector_observation, plan_critic_risk.
	SeveritySoft Severity = "soft"
)

// IsValid reports whether s is a known Severity.
func (s Severity) IsValid() bool {
	switch s {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeveritySoft:
		return true
	}
	return false
}

// Rank returns a numeric ranking suitable for sorting violations
// (highest severity first). Critical=4, High=3, Medium=2, Soft=1.
// Unknown values rank 0.
func (s Severity) Rank() int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeveritySoft:
		return 1
	}
	return 0
}

// ScoredViolation is the structured violation record consumed by
// the RetryState contract. Augments the legacy Violation with a
// Severity classification, the gating Layer that produced the
// signal, and a FieldPath pointing at the typed location the LLM
// must edit (when available).
//
// FieldPath syntax: dotted-bracket-style identifiers the LLM can
// match against its emit JSON. Examples:
//   - blocks[id="lifecycle"].claim_use
//   - blocks[id="<some_block>"].facet_ids
//   - citations[5].file
//   - exact_resolution.status
//
// Empty FieldPath means "no precise location" (telemetry-only kind
// or a global gating signal); the LLM falls back to Detail/Repair
// prose for guidance.
type ScoredViolation struct {
	// Kind, Detail, Repair mirror the legacy Violation fields so
	// existing producers can wrap their Violation outputs into a
	// ScoredViolation cheaply. SuspectedRoot stays on the legacy
	// surface — RetryState consumers don't need it.
	Kind   ViolationKind `json:"kind"`
	Detail string        `json:"detail,omitempty"`
	Repair string        `json:"repair,omitempty"`

	// Severity is derived once at violation production time via
	// DeriveSeverity(kind, isStrict). Persisted on the struct so
	// downstream consumers don't re-derive (avoids drift between
	// scheduler decision points and prompt rendering).
	Severity Severity `json:"severity"`

	// Layer names which gating layer surfaced this violation:
	//   - "scheduler" — finalize TaskNode SuccessCriterion failure
	//   - "v2_oracle" — runV2BlockOraclesWithMut family of validators
	//   - "contract_check" — analysis/contract.Check (citation,
	//     must_include / must_exclude, acceptance, family shape)
	//   - "self_consistency" — self_consistency_reviewer V2 dispatch
	//   - "external_artifact" — observed-artifact typed carrier check
	//   - "authority" — runAuthorityOverreachCheck
	//
	// Cross-layer rendering groups by Layer so the LLM sees all
	// gating layers at once instead of fixing one and tripping
	// another (R13).
	Layer string `json:"layer"`

	// BlockID names the V2 block id the violation applies to,
	// when applicable. Empty for non-block-scoped violations
	// (e.g. citation count, exact_resolution).
	BlockID string `json:"block_id,omitempty"`

	// FieldPath points at the typed field the LLM must edit.
	// See struct doc for syntax.
	FieldPath string `json:"field_path,omitempty"`
}

// RetryBlockSummary captures the typed state of one AnswerBlock from
// the previous emit. Used by RetryState to render the "Previous
// Emit" prompt section so the LLM sees what it already filled.
//
// Field selection prioritises the typed annotations that have
// historically been LOST on retry (R6 / R6.1) — block-level
// claim_use presence + facet_ids verbatim + surface_role.
// Item-level annotations are summarised by count rather than
// individually rendered to keep the prompt budget bounded.
type RetryBlockSummary struct {
	ID                string          `json:"id"`
	Kind              AnswerBlockKind `json:"kind"`
	SurfaceRole       SurfaceRole     `json:"surface_role,omitempty"`
	FacetIDs          []string        `json:"facet_ids,omitempty"`
	HasClaimUse       bool            `json:"has_claim_use"`
	ClaimForm         ClaimForm       `json:"claim_form,omitempty"`
	HasItems          bool            `json:"has_items"`
	ItemCount         int             `json:"item_count,omitempty"`
	ItemsWithCitation int             `json:"items_with_citation,omitempty"`
	// EdgeAnchoredClaimUses counts how many DiagramEdgeAnchor
	// entries this block carries (Phase 1-B source-fix, 2026-05-04).
	// Pre-fix, edge anchors lived as inner fields of RenderedClaimUse
	// (FromNode/ToNode) and inflated claim_use's schema density to
	// 6 inner fields; LLMs mis-filled sibling fields like
	// citation_ref into the same nested object (u3a-1 forensic).
	// Now edge anchors are typed AnswerBlock.EdgeAnchors[] and
	// claim_use is back to 4 fields. The field name retains the
	// historical "ClaimUses" suffix for tooling compatibility — the
	// underlying signal is "edge anchors on this block".
	EdgeAnchoredClaimUses int `json:"edge_anchored_claim_uses,omitempty"`
	// TextPreview is the head+tail clip of the block's text /
	// title (head 400 + tail 200 chars when over the cap, full
	// text when under). Lets the LLM identify the block by
	// content without bloating the prompt.
	TextPreview string `json:"text_preview,omitempty"`
}

// RetryStateSummary is the summary projection of the previous
// emit. Concise enough for prompt embedding; full prev emit
// JSON is held separately on RetryState.PrevEmitJSON for cases
// where a larger surface is acceptable.
type RetryStateSummary struct {
	BlockSummaries     []RetryBlockSummary `json:"block_summaries,omitempty"`
	CitationsCount     int                 `json:"citations_count"`
	CitationFiles      []string            `json:"citation_files,omitempty"`
	HasExactResolution bool                `json:"has_exact_resolution,omitempty"`
}

// RetryState is the typed contract surfaced to the LLM on every
// retry dispatch. Rendered into the agent's BuildInitialInstruction
// when EmitStageRetryAttempt > 0.
//
// Lifecycle:
//   - Allocated by the orchestrator when contract.Check fails AND
//     the scheduler decides to retry the finalizer.
//   - Populated from the previous emit (PrevEmitJSON +
//     PrevEmitSummary) and accumulated violations across all gating
//     layers.
//   - Read by renderRetryState (in the agent layer) to produce the
//     "Previous Emit / Active Violations / Required Changes / Hard
//     Rule" prompt sections.
//   - Cleared by ResetRetryState on stage entry / at fresh dispatch
//     so old state never leaks across stages or runs.
type RetryState struct {
	// PrevEmitJSON is the verbatim JSON the LLM emitted on the last
	// dispatch (raw tool-call params). Optional — when empty, the
	// LLM falls back to PrevEmitSummary alone.
	PrevEmitJSON json.RawMessage `json:"prev_emit_json,omitempty"`

	// PrevEmitSummary is the typed projection of PrevEmitJSON.
	// Always populated when PrevEmitJSON is non-empty.
	PrevEmitSummary RetryStateSummary `json:"prev_emit_summary"`

	// ActiveViolations is the full set of active violations from
	// the most recent contract.Check + V2 oracle + scheduler
	// SuccessCriterion run. Sorted by Severity descending at
	// rendering time; producers append in collection order.
	ActiveViolations []ScoredViolation `json:"active_violations,omitempty"`

	// Attempt is the retry attempt index (1 = first retry, 2 = second,
	// etc.). 0 means "no retry yet" — RetryState is unused on
	// fresh dispatches.
	Attempt int `json:"attempt,omitempty"`

	// LastPrimaryOwner names the RepairLocus picked by BuildRepairPlan
	// for the previous attempt's violations (string form of
	// orchestrator.RepairLocus — "finalizer" / "extract" / "explore" /
	// "terminal"). Empty when no plan was computed (fresh dispatch
	// or pre-Phase-1 retry path).
	//
	// Phase 1-A2 (V2 runtime consolidation, 2026-05-04): wired by
	// populateRetryState so the orchestrator can detect ping-pong
	// (same owner picked N times with no progress → escalate to
	// FailLoud). Surfaced in retry-summary trace + telemetry.
	LastPrimaryOwner string `json:"last_primary_owner,omitempty"`

	// OwnerStableAttempts counts how many consecutive retries have
	// landed on the SAME LastPrimaryOwner. Resets to 1 when the owner
	// changes; increments when the next plan picks the same owner.
	//
	// Used by escalation logic: if OwnerStableAttempts exceeds a
	// configured threshold without the violation set shrinking, the
	// router escalates one locus deeper (or to FailLoud at terminal).
	OwnerStableAttempts int `json:"owner_stable_attempts,omitempty"`

	// LastPrimaryViolation names the deepest cluster's Primary
	// ViolationKind from the previous attempt's RepairPlan. Surfaced
	// in trace for operator audit ("which root cause did the router
	// chase last attempt"). Empty when no plan computed.
	LastPrimaryViolation ViolationKind `json:"last_primary_violation,omitempty"`

	// LastEmitFromPatch flags whether the previous emit was sourced
	// via emit_answer_document_patch (true — base inherited from a
	// prior full emit, modified by patch operations) or fresh full
	// emit (false — entire doc rewritten by LLM).
	//
	// Phase 2-B4 (V2 runtime consolidation, 2026-05-04). Pure
	// observability — surfaces in retry summary so operators can
	// audit "is this a clean retry chain or a patch-extended one?"
	// without inferring from external signals.
	LastEmitFromPatch bool `json:"last_emit_from_patch,omitempty"`
}

// HasContent reports whether the RetryState carries any
// renderable signal. Used by render helpers to skip rendering
// empty state (preserves byte-identical pre-R14 prompt on
// fresh dispatches).
func (rs *RetryState) HasContent() bool {
	if rs == nil {
		return false
	}
	if rs.Attempt == 0 {
		return false
	}
	return len(rs.ActiveViolations) > 0 ||
		len(rs.PrevEmitSummary.BlockSummaries) > 0 ||
		len(rs.PrevEmitJSON) > 0
}

// ViolationProfile is the **R15 single source of truth** for every
// runtime decision driven by a ViolationKind:
//
//   - Severity (R14): drives prompt rendering priority in
//     RetryState (Critical → Required Changes top, Soft → telemetry
//     section only).
//   - RetryEligible (R15): drives whether contract.Check flips
//     Passed=false and the scheduler triggers a retry. Replaces the
//     legacy defaultSoftKinds / softViolationKinds map gate.
//   - FallbackTargetHint (R15): the canonical fallback locus for
//     this kind, sourced from the orchestrator's FallbackPolicy at
//     RuntimeViolationProfile time. Optional — pure-types callers
//     can derive without it.
//
// Replaces the m1a r2 deep-audit divergence:
//
//   - R14 marks ViolPrincipalClaimUseMissing Severity=Critical
//   - But defaultSoftKinds happens to NOT include it, so it
//     IS strict for retry-trigger purposes (good)
//
// However for OTHER kinds (Block 2 Intent / Subject /
// PredicateAxis oracle kinds), R14 marks them High but
// defaultSoftKinds marks them SOFT — divergence. R15 unifies:
// every production decision reads ViolationProfile, ensuring
// Severity and RetryEligible cannot drift.
type ViolationProfile struct {
	Kind          ViolationKind
	Severity      Severity
	RetryEligible bool
}

// ViolationProfileFor returns the canonical profile for a kind.
// Single source of truth — consumed by isSoftViolationKind,
// FallbackTargetForKind (transitively), and the R14 retry-state
// scoreViolations path.
//
// RetryEligible derivation:
//
//	Severity Critical / High / Medium → true (retry triggers)
//	Severity Soft                     → false (telemetry only,
//	                                            never blocks ship)
//
// isStrict honours pipeline_contract_strict_kinds yaml override:
// when operator promotes a Soft kind to strict, the profile
// bumps Severity Soft → Medium AND RetryEligible false → true.
//
// v3 B0 (2026-05-04): when the kind has a registered ViolKindSpec
// AND the spec marks it as not Promotable, isStrict is silently
// ignored (the operator-promote path is a no-op). This honours
// the permanently-soft contract (label inference / frequency
// bridges / richness regression) at the profile layer rather than
// only at SetSoftViolationKinds.
func ViolationProfileFor(kind ViolationKind, isStrict bool) ViolationProfile {
	if isStrict && !IsViolKindPromotable(kind) {
		isStrict = false
	}
	sev := DeriveSeverity(kind, isStrict)
	return ViolationProfile{
		Kind:          kind,
		Severity:      sev,
		RetryEligible: sev != SeveritySoft,
	}
}

// DeriveSeverity is the single SOURCE OF TRUTH for Severity
// classification of every ViolationKind. Called by every producer
// (scheduler / V2 oracle / contract.Check / self_consistency /
// external_artifact / authority) so the same kind never gets
// classified differently across layers.
//
// isStrict argument honours pipeline_contract_strict_kinds yaml
// override: when an operator promotes a Soft kind to strict, this
// function bumps SeveritySoft → SeverityMedium. When the operator
// relaxes a Critical kind, this function does NOT downgrade —
// Critical is intrinsic ("answer cannot ship without").
//
// v3 B0 (2026-05-04): the registry is the canonical source for the
// non-strict (operator-default) classification. When a spec exists
// for kind AND isStrict=false, the registry's DefaultSeverity wins.
// Strict promotion (isStrict=true) currently delegates to the legacy
// switch so existing nuanced bump rules (Medium→High for V2 oracle
// kinds; Soft→Medium for some kinds; Soft-permanent for others)
// remain byte-identical. A follow-up PR can push strict semantics
// into the registry once eval validates the rules.
//
// Permanently non-promotable kinds (spec.Promotable=false) ignore
// isStrict at the ViolationProfileFor layer (see promotion guard
// there); DeriveSeverity itself does not need to special-case them
// because the legacy switch happens to encode their strict-stays-
// soft behaviour already.
func DeriveSeverity(kind ViolationKind, isStrict bool) Severity {
	if !isStrict {
		if spec, ok := ViolKindSpecFor(kind); ok {
			return spec.DefaultSeverity
		}
	}
	return legacyDeriveSeverity(kind, isStrict)
}

// legacyDeriveSeverity is the pre-B0 hardcoded switch. Retained for
// migration-window verification and as a fallback for any
// not-yet-registered kind. Deletion follows once registry coverage
// is enforced via TestEveryViolKindHasSpec.
func legacyDeriveSeverity(kind ViolationKind, isStrict bool) Severity {
	switch kind {
	// Critical: answer cannot ship — finalizer-only fixable but
	// fail-loud on budget exhaustion.
	case ViolPrincipalClaimUseMissing,
		ViolBlockCoverageMissing,
		ViolAuthorityOverreach:
		return SeverityCritical

	// High by default: wrong-topic final answers are not richness
	// gaps; they must block shipping even without operator promotion.
	case ViolAnswerTopicMismatch:
		return SeverityHigh

	// High: strict-by-default V2 oracle violations + structural
	// answer-shape mismatches. Eligible for finalize retry.
	//
	// 2026-05-17 T3 soft-demote moved ViolUncertaintyBlockMissing,
	// ViolStructuralEnumerationDivergence, ViolSymbolAnchorMismatch
	// out of this group into the SOFT-by-default block below — their
	// violation.go comments explicitly named them as SOFT, and the
	// failure modes (missing caveat / partial transparency / telemetry
	// signal first) do not warrant a retry round. Strict-promote via
	// pipeline_contract_strict_kinds still returns SeverityHigh via
	// the soft block's isStrict branch.
	case ViolFacetUncovered,
		ViolDiagramEdgeUnsupported,
		ViolCurrentStatusVerdictMissing,
		ViolClaimFormUnsupported,
		ViolFamilyMismatch,
		ViolViewIntentMismatch,
		ViolViewSwap,
		ViolSubTopicCountMismatch,
		ViolIntentTraceShallow,
		ViolIntentEnumerateNotList,
		ViolIntentRootCauseNoCause,
		ViolIntentConfigNoTrail,
		ViolSubjectAnchorMissing,
		ViolPredicateAxisMissing,
		ViolAbsenceScopeExceeded,
		ViolStepIdentifierUnverified,
		ViolValueSecondaryCitationOffFocus,
		ViolEnumerationLabelUngrounded,
		ViolEnumerationLabelHallucinated,
		ViolInlineIdentifierHallucinated,
		ViolEnumerationItemLabelExtractorDrift,
		ViolCrossCitationConflict,
		ViolDeclaredCountDrift,
		ViolSelfContradiction,
		ViolDiagramIdentifier,
		ViolGhostAnchor,
		ViolSelfRefLiteral,
		ViolPreCompleteDowngrade,
		ViolLiteralFormFailed,
		ViolChainDemoted,
		ViolCitation,
		ViolMustInclude,
		ViolMustExclude,
		ViolAcceptance,
		ViolSuccessCriterion,
		ViolExternalArtifactUnderdecoded:
		if isStrict {
			return SeverityHigh
		}
		// Soft promotion to Medium gives operators a way to boost
		// without going all the way to Critical.
		return SeverityMedium

	// Soft: telemetry-only — never blocks shipping.
	case ViolRichnessRegression,
		ViolPlanCritic,
		ViolReflectorObservation,
		ViolAnswerReviewerDistilled,
		// R10 CGEC frequency bridges — telemetry-only, never block.
		ViolDemotionStorm,
		ViolForcedReadStorm,
		// G3 (post_v2_runtime_gap_remediation, 2026-05-04) —
		// diagram edge label-vs-typed drift. Permanently SOFT
		// (label inference is noisy; isStrict has no effect — the
		// operator-promote path is intentionally a no-op).
		// Diagram relation label-only is even weaker: the visible label
		// already satisfies the contract, and the missing edge_anchors
		// metadata is telemetry only.
		// Diagram endpoint resolution is also a visual-carrier signal:
		// Mermaid nodes may be component labels rather than exact code
		// declarations, and graph coverage differs by language. Keep it
		// permanent SOFT so diagrams requested by the user are caveated,
		// not rewritten away.
		ViolDiagramEdgeLabelMismatch,
		ViolDiagramRelationLabelOnly,
		ViolDiagramEdgeEndpointHallucinated:
		return SeveritySoft
	}
	// G5 (post_v2_runtime_gap_remediation, 2026-05-04) — semantic
	// quality reviewer signal. SOFT-by-default; isStrict promotes
	// to Medium so an operator who configures
	// pipeline_contract_strict_kinds can lift it into the retry
	// loop. Never reaches Critical (the reviewer's confidence is
	// load-bearing but not infallible).
	if kind == ViolAnswerSemanticUnderfilled {
		if isStrict {
			return SeverityMedium
		}
		return SeveritySoft
	}
	// 修 B (post_v2_runtime_gap_remediation, 2026-05-04) —
	// enumeration evidence underspecification. SOFT-by-default
	// (typed signal is precise but too noisy for hard gate without
	// structural pattern match — see design doc). isStrict
	// promotes to Medium so an operator with high-confidence
	// explorer pipelines can lift it into the retry loop.
	if kind == ViolEnumerationEvidenceUnderspecified {
		if isStrict {
			return SeverityMedium
		}
		return SeveritySoft
	}
	// 2026-05-17 T3 soft-demote: 5 oracle kinds whose violation.go
	// comments document SOFT-by-default intent. Failure mode is
	// style / transparency / telemetry, not answer correctness —
	// the answer ships with a residual-concerns caveat (via
	// MaterializeUnresolvedViolationsAsCaveats) instead of burning a
	// finalize retry round. Operators who want strict enforcement
	// promote via pipeline_contract_strict_kinds; the isStrict branch
	// below returns SeverityHigh to land the kind in the retry group.
	//
	//   - ViolRichnessGlaringGap            : enrichment advisory
	//   - ViolPrincipalProseUnderfilled     : prose density hint
	//   - ViolUncertaintyBlockMissing       : caveat transparency hint
	//   - ViolStructuralEnumerationDivergence : partial enumeration disclosure
	//   - ViolSymbolAnchorMismatch          : explorer-coverage telemetry
	if kind == ViolRichnessGlaringGap ||
		kind == ViolPrincipalProseUnderfilled ||
		kind == ViolUncertaintyBlockMissing ||
		kind == ViolStructuralEnumerationDivergence ||
		kind == ViolSymbolAnchorMismatch {
		if isStrict {
			return SeverityHigh
		}
		return SeveritySoft
	}
	// Multi-repo inactive-scope disclosure is a runtime/workspace
	// boundary note. The orchestrator appends a system caveat when
	// needed, so it must not force a model rewrite by default.
	if kind == ViolInactiveScopeDisclosureMissing {
		return SeveritySoft
	}
	// Multi-repo write fail-loud (design §4.5.5, 2026-05-08).
	// SeveritySoft regardless of isStrict — there is no retry
	// pathway (the user MUST split the work into separate runs).
	// Promotable=false at the registry layer means isStrict has no
	// effect. The actual fail-loud comes from planPostHook returning
	// an error which terminates the Run — Severity is purely a
	// telemetry/ledger classification here.
	if kind == ViolWriteCrossSubRepoForbidden {
		return SeveritySoft
	}
	// L3 negative-knowledge answer validator (TypedDenials Phase F,
	// 2026-05-08). SOFT-by-default for ledger-only; isStrict promotes
	// to Medium so operators with attached-log-heavy workflows can
	// lift it into the retry loop after baseline eval validates the
	// false-positive rate is low.
	if kind == ViolDeniedTokenUndeclared {
		if isStrict {
			return SeverityMedium
		}
		return SeveritySoft
	}
	// Unknown kinds default to Medium — safer than Soft (won't
	// silently drop) but not Critical (won't unexpectedly fail-loud).
	return SeverityMedium
}
