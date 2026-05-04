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
	//   - "external_artifact" — runExternalArtifactDecodedCheck
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
	ItemsWithClaimUse int             `json:"items_with_claim_use,omitempty"`
	ItemsWithCitation int             `json:"items_with_citation,omitempty"`
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
	BlockSummaries []RetryBlockSummary `json:"block_summaries,omitempty"`
	CitationsCount int            `json:"citations_count"`
	CitationFiles  []string       `json:"citation_files,omitempty"`
	HasExactResolution bool       `json:"has_exact_resolution,omitempty"`
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
func ViolationProfileFor(kind ViolationKind, isStrict bool) ViolationProfile {
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
func DeriveSeverity(kind ViolationKind, isStrict bool) Severity {
	switch kind {
	// Critical: answer cannot ship — finalizer-only fixable but
	// fail-loud on budget exhaustion.
	case ViolPrincipalClaimUseMissing,
		ViolBlockCoverageMissing,
		ViolAuthorityOverreach:
		return SeverityCritical

	// High: strict-by-default V2 oracle violations + structural
	// answer-shape mismatches. Eligible for finalize retry.
	case ViolFacetUncovered,
		ViolDiagramEdgeUnsupported,
		ViolUncertaintyBlockMissing,
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
		ViolStructuralEnumerationDivergence,
		ViolAbsenceScopeExceeded,
		ViolStepIdentifierUnverified,
		ViolValueSecondaryCitationOffFocus,
		ViolSymbolAnchorMismatch,
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
		ViolAnswerReviewerDistilled:
		return SeveritySoft
	}
	// Unknown kinds default to Medium — safer than Soft (won't
	// silently drop) but not Critical (won't unexpectedly fail-loud).
	return SeverityMedium
}
