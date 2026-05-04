// Package orchestrator — fallback_policy.go
//
// Block 3 of the architecture overhaul (2026-05-02): replace the
// pre-2026-05-02 "all done nodes get requeued" finalize-retry path
// (orchestrator.go:3592-3613) with a violation-kind-driven
// selective fallback. Each ViolationKind routes to one
// FallbackTarget naming the deepest stage to roll back to:
//
//   FinalizerOnly  — re-emit the answer document only; explorer +
//                    extractor state preserved
//   BackToExtract  — clear the answer + symbol slate, keep evidence
//   BackToExplore  — clear the answer + symbol slate + extract
//                    output, keep TurnA evidence + scanned set,
//                    force PendingRead repopulation
//   BackToAnalyze  — fail-loud (analyzer is the classification
//                    source; rolling back changes the question's
//                    interpretation, which the user did not request)
//   FailLoud       — abort the retry loop, ship the previous answer
//                    with violation caveats
//
// The default table is the result of the audit on each kind's
// failure mode — see the long comment on DefaultFallbackPolicy
// below for the per-kind rationale.

package orchestrator

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// FallbackTarget enumerates the stages a violation can roll back to.
// Order mirrors the read-mode pipeline: FinalizerOnly is the
// shallowest fallback, BackToAnalyze the deepest. FailLoud is the
// terminal "abort" exit.
type FallbackTarget string

const (
	// FallbackFinalizerOnly re-emits emit_answer_document. Used for
	// shape / wording / per-doc field violations where evidence and
	// extractor state are intact.
	FallbackFinalizerOnly FallbackTarget = "finalizer_only"

	// FallbackBackToExtract clears the symbol slate and re-runs the
	// extractor. Used when the symbols / step list itself is wrong
	// (count drift, declared-vs-actual mismatch).
	FallbackBackToExtract FallbackTarget = "back_to_extract"

	// FallbackBackToExplore clears the answer + symbol slate and
	// re-runs the explorer. Used when evidence is missing or wrong-
	// shaped (citation_count_ge, predicate_axis_missing, intent_
	// trace_shallow on a question that genuinely needs more hops).
	FallbackBackToExplore FallbackTarget = "back_to_explore"

	// FallbackBackToAnalyze is reserved but never the default for any
	// kind: classification is the user-facing semantic and the system
	// cannot legitimately decide on its own to re-classify.
	FallbackBackToAnalyze FallbackTarget = "back_to_analyze"

	// FallbackFailLoud aborts the retry loop and surfaces the
	// previous answer plus violation caveats. Used when the
	// violation is non-recoverable through retry (e.g. retry budget
	// exhausted, yield kill).
	FallbackFailLoud FallbackTarget = "fail_loud"
)

// IsValid reports whether t is one of the declared targets. Used by
// the runtime config loader to validate operator-supplied yaml
// overrides — an unknown target falls back to the default.
func (t FallbackTarget) IsValid() bool {
	switch t {
	case FallbackFinalizerOnly,
		FallbackBackToExtract,
		FallbackBackToExplore,
		FallbackBackToAnalyze,
		FallbackFailLoud:
		return true
	}
	return false
}

// RepairLocus classifies WHERE a violation can actually be repaired
// — the stage that legitimately owns the fix. Introduced in B3-F2
// (post_shape_retirement_consolidated_audit.md §8 Batch B3,
// 2026-05-04) to replace the pre-B3 "deepest fallback target wins"
// rule. The pre-B3 rule over-pulled when one finalizer-local
// violation co-occurred with one extract-local violation: the
// extract-local target dominated and the entire retry got dragged
// upstream, even though most violations could be fixed at finalize.
//
// The B3 rule is "primary repair locus":
//   1. Bucket violations by their locus.
//   2. Pick the locus with the most violations.
//   3. On tie, fall back to the deepest target (preserves safety —
//      if equal counts, we'd rather over-rebuild than under-repair).
//
// Locus values mirror the read-mode pipeline stages plus a terminal
// fail-loud sentinel:
type RepairLocus string

const (
	// LocusFinalizer — the violation can be fixed by re-emitting
	// the answer document with adjusted block payload / claim_use /
	// diagram.kind / Authority disclosure. Evidence + extractor
	// state are preserved.
	LocusFinalizer RepairLocus = "finalizer"

	// LocusExtract — the violation needs the extractor to re-pick
	// citations / re-frame absence scope / re-emit the symbol
	// slate. Evidence is preserved, the extract output is reset.
	LocusExtract RepairLocus = "extract"

	// LocusExplore — the violation needs new evidence (a new file
	// read, a new grep, a new closure entry). Both extract output
	// and answer document are reset; explorer re-runs.
	LocusExplore RepairLocus = "explore"

	// LocusTerminal — the violation is non-recoverable through
	// retry (analyzer-classification disputes, retry budget, yield
	// kill, reviewer-distilled informational kinds).
	LocusTerminal RepairLocus = "terminal"
)

// LocusOfTarget returns the RepairLocus that owns the named
// FallbackTarget. Identity-mapped except BackToAnalyze →
// LocusTerminal (re-classification is a design red line and
// FailLoud → LocusTerminal).
func LocusOfTarget(t FallbackTarget) RepairLocus {
	switch t {
	case FallbackFinalizerOnly:
		return LocusFinalizer
	case FallbackBackToExtract:
		return LocusExtract
	case FallbackBackToExplore:
		return LocusExplore
	case FallbackBackToAnalyze, FallbackFailLoud:
		return LocusTerminal
	}
	return LocusFinalizer
}

// targetForLocus returns the FallbackTarget aligned with the given
// RepairLocus. Used by FallbackTargetForViolations after picking
// the primary locus.
func targetForLocus(l RepairLocus) FallbackTarget {
	switch l {
	case LocusFinalizer:
		return FallbackFinalizerOnly
	case LocusExtract:
		return FallbackBackToExtract
	case LocusExplore:
		return FallbackBackToExplore
	case LocusTerminal:
		return FallbackFailLoud
	}
	return FallbackFinalizerOnly
}

// FallbackPolicy maps each ViolationKind to its preferred
// FallbackTarget. Operators override individual entries via
// codrax.yaml :: pipeline_fallback_policy_overrides; missing entries
// default to FallbackFinalizerOnly (the safest fallback that always
// makes forward progress).
type FallbackPolicy map[types.ViolationKind]FallbackTarget

// DefaultFallbackPolicy returns the canonical map used in production.
// Per-kind rationale (red lines: reviewer-specific, no one-size-fits-
// all):
//
//   ViolSelfContradiction        → BackToExplore
//     The summary contradicts the body. Either the LLM hallucinated
//     a fact (need more evidence) or it has the right facts in the
//     wrong place. Re-emitting the doc alone (FinalizerOnly) tends
//     to flip the contradiction; explore-then-finalize gives the
//     LLM fresh evidence to anchor on.
//
//   ViolViewIntentMismatch      → FinalizerOnly
//     Pure shape choice; evidence is intact, only the wrapper is
//     wrong. No need to revisit upstream.
//
//   ViolDeclaredCountDrift       → BackToExtract
//     The extractor's symbol slate count and the renderer's count
//     diverge. Re-running the extractor with the same evidence is
//     the targeted repair.
//
//   ViolDiagramIdentifier        → FinalizerOnly
//     Bare-identifier hallucination in the rendered diagram.
//     Evidence still backs the answer; the diagram is a finalizer
//     stylistic choice.
//
//   ViolMustInclude / MustExclude → FinalizerOnly
//     Must-include is a shape-level claim about the rendered text;
//     re-emit with the same evidence.
//
//   ViolCitation / ViolGhostAnchor → BackToExplore
//     The cited line doesn't ground the claim. Need more evidence.
//
//   ViolAcceptance               → BackToExplore
//     AnswerContract acceptance test failed. Usually evidence-shape
//     mismatch — give the explorer another shot.
//
//   ViolSuccessCriterion         → BackToExplore
//     Same as Acceptance.
//
//   ViolChainDemoted             → BackToExplore
//     Chain promotion failure means an anchor lacks a real
//     definition site. More evidence is the cure.
//
//   ViolSelfRefLiteral / LiteralFormFailed → FinalizerOnly
//     Pure shape / literal-validation issues; evidence is intact.
//
//   ViolPreCompleteDowngrade     → FinalizerOnly
//     Soft signal already handled at emit time; Block 3 has no
//     upstream remediation.
//
//   ViolViewSwap                → FinalizerOnly
//
//   ViolSubTopicCountMismatch    → FinalizerOnly
//     Re-emit with the right shape; the evidence supports either
//     side of the count.
//
//   ViolExternalArtifactUnderdecoded → FinalizerOnly
//     The bundle is already in Mutable; re-emit with the missing
//     tokens decoded.
//
//   ViolAuthorityOverreach       → FinalizerOnly
//     Renderer bypass diagnostic — re-emit through the structured
//     emit path.
//
//   ViolPlanCritic / Reflector / AnswerReviewer (Block 1 reviewer
//     kinds) → FailLoud — IMPORTANT: this routing only ACTIVATES
//     when the operator promotes the kind to strict via
//     pipeline_contract_strict_kinds. Under default SOFT
//     classification (defaultSoftKinds in contract_check.go) these
//     violations are recorded for telemetry but do NOT flip
//     res.Passed=false, so they never reach the fallback switch.
//     The FailLoud target is the safety net for operators who
//     specifically want reviewer signals to halt the Run.
//
//   Block 2 Intent oracle kinds:
//     ViolIntentTraceShallow      → BackToExplore
//     ViolIntentEnumerateNotList  → FinalizerOnly
//     ViolIntentRootCauseNoCause  → BackToExplore
//     ViolIntentConfigNoTrail     → BackToExplore
//     ViolSubjectAnchorMissing    → BackToExtract
//     ViolPredicateAxisMissing    → BackToExplore
func DefaultFallbackPolicy() FallbackPolicy {
	return FallbackPolicy{
		// Existing read-mode kinds.
		// Phase 1-D (s1a-20260504-130143 abstraction-drift forensic,
		// 2026-05-04): ViolSelfContradiction was BackToExplore but
		// the reviewer detects FINALIZER PROSE matter — the evidence
		// pool is not the issue. Routing to BackToExplore is over-
		// reaction; let finalizer rewrite with retry hint instead.
		// The s1a-2 forensic showed back_to_explore actually
		// AMPLIFIED the abstraction drift (LLM second-pass
		// generalised more to avoid the contradiction).
		types.ViolSelfContradiction:           FallbackFinalizerOnly,
		types.ViolViewIntentMismatch:         FallbackFinalizerOnly,
		types.ViolDeclaredCountDrift:          FallbackBackToExtract,
		types.ViolDiagramIdentifier:           FallbackFinalizerOnly,
		types.ViolMustInclude:                 FallbackFinalizerOnly,
		types.ViolMustExclude:                 FallbackFinalizerOnly,
		types.ViolCitation:                    FallbackBackToExplore,
		types.ViolGhostAnchor:                 FallbackBackToExplore,
		types.ViolAcceptance:                  FallbackBackToExplore,
		types.ViolSuccessCriterion:            FallbackBackToExplore,
		types.ViolChainDemoted:                FallbackBackToExplore,
		types.ViolSelfRefLiteral:              FallbackFinalizerOnly,
		types.ViolLiteralFormFailed:           FallbackFinalizerOnly,
		types.ViolPreCompleteDowngrade:        FallbackFinalizerOnly,
		types.ViolViewSwap:                   FallbackFinalizerOnly,
		types.ViolSubTopicCountMismatch:       FallbackFinalizerOnly,
		types.ViolFamilyMismatch:                       FallbackFinalizerOnly,
		types.ViolExternalArtifactUnderdecoded: FallbackFinalizerOnly,
		types.ViolAuthorityOverreach:          FallbackFinalizerOnly,
		// Block 1 reviewer kinds — informational, no upstream
		// remediation. Mapped to FailLoud so a Run that ONLY has
		// reviewer-noise violations does NOT spin a finalize retry.
		types.ViolPlanCritic:              FallbackFailLoud,
		types.ViolReflectorObservation:    FallbackFailLoud,
		types.ViolAnswerReviewerDistilled: FallbackFailLoud,
		// Block 2 Intent / Subject / PredicateAxis oracle kinds.
		types.ViolIntentTraceShallow:     FallbackBackToExplore,
		types.ViolIntentEnumerateNotList: FallbackFinalizerOnly,
		types.ViolIntentRootCauseNoCause: FallbackBackToExplore,
		types.ViolIntentConfigNoTrail:    FallbackBackToExplore,
		types.ViolSubjectAnchorMissing:   FallbackBackToExtract,
		types.ViolPredicateAxisMissing:   FallbackBackToExplore,
		// Phase 4 (Semantic Surface Contract):
		//   ViolFacetUncovered — covering a facet typically needs new
		//     evidence (a new file read, a new grep), so re-explore.
		//     SOFT by default; only fires the fallback when promoted.
		//   ViolClaimFormUnsupported — LLM-emitted annotation conflicts
		//     with cited evidence shape. Finalizer rewrite (drop/swap
		//     the claim_use) fixes it. STRICT.
		//   ViolAbsenceScopeExceeded — extractor framed the absence
		//     too broadly. BackToExtract so the next pass surfaces the
		//     bounded scope verbatim. STRICT.
		types.ViolFacetUncovered:       FallbackBackToExplore,
		types.ViolClaimFormUnsupported: FallbackFinalizerOnly,
		types.ViolAbsenceScopeExceeded: FallbackBackToExtract,
		// Phase 4 extension — step-identifier-not-in-typed-evidence-
		// pool. Missing identifiers usually mean explorer skipped
		// emit_evidence on the load-bearing symbol; the right
		// remediation is BackToExplore so the next pass captures it
		// in a typed Subject/Object/AnchorSymbol field.
		types.ViolStepIdentifierUnverified: FallbackBackToExplore,
		// P1 #3 (2026-05-03) — repeated emit_answer_symbol line-
		// anchor rejections diagnose the explorer never reading
		// def-region for the user's principal entities; remediation
		// is a fresh investigation pass that probes def-line reads.
		types.ViolSymbolAnchorMismatch: FallbackBackToExplore,
		// post-shape s1a-20260504-064754 hallucination forensic
		// (2026-05-04) — finalizer emitted enumeration items[].label
		// values that no EvidenceItem.AnchorSymbol/Subject/Object
		// substring-supports; remediation is in extractor's hands
		// (re-emit items[] with labels copied verbatim from the
		// evidence pool), no fresh investigation pass needed.
		types.ViolEnumerationLabelUngrounded: FallbackBackToExtract,
		// s1a-20260504-130143 abstraction-drift forensic (2026-05-04)
		// — extractor emit_answer_symbol provided 9 verbatim method
		// names but finalizer rendered them as abstract placeholders.
		// The extractor signal is intact; only finalizer rendering
		// needs rewrite to copy the verbatim names. Hence
		// FinalizerOnly.
		types.ViolEnumerationItemLabelExtractorDrift: FallbackFinalizerOnly,
		// P3 #6 precise variant (2026-05-03) — code-vs-comment
		// divergence transparency. Remediation is in extractor's
		// hands (re-emit emit_answer_symbol with caveat rows or
		// emit_answer_document with explicit summary mentions);
		// no new investigation needed since the typed signal +
		// existing evidence already supply everything required.
		types.ViolStructuralEnumerationDivergence: FallbackBackToExtract,
		// B4 V2 block-only carrier validators
		// (block_only_carrier.md §5.4). All 4 are SOFT-by-default
		// during B4-B5 (telemetry only — never reach this fallback
		// switch under default classification). B6 promotes to
		// STRICT and the fallback target activates:
		//   - BlockCoverageMissing: extractor re-emits the missing
		//     block kind (FallbackBackToExtract).
		//   - PrincipalClaimUseMissing: finalizer re-emits with
		//     claim_use annotations (FallbackFinalizerOnly).
		//   - DiagramEdgeUnsupported: finalizer re-emits diagram
		//     with correct kind / matched edges (FinalizerOnly).
		//   - UncertaintyBlockMissing: extractor / finalizer adds
		//     the disclosure block — closer to a finalizer-only
		//     repair since the fact (uncertainty source) is already
		//     in evidence; the missing piece is the block itself.
		types.ViolBlockCoverageMissing:     FallbackBackToExtract,
		types.ViolPrincipalClaimUseMissing: FallbackFinalizerOnly,
		types.ViolDiagramEdgeUnsupported:   FallbackFinalizerOnly,
		// G3 (post_v2_runtime_gap_remediation, 2026-05-04) — typed
		// vs label drift is a finalize-local rewrite (relabel for
		// readability without changing typed declaration).
		types.ViolDiagramEdgeLabelMismatch: FallbackFinalizerOnly,
		// G5 (post_v2_runtime_gap_remediation, 2026-05-04) — semantic
		// quality signal. Finalize-local rewrite expands coverage
		// using already-available typed evidence; no upstream rerun.
		types.ViolAnswerSemanticUnderfilled: FallbackFinalizerOnly,
		types.ViolUncertaintyBlockMissing:  FallbackFinalizerOnly,
		// Phase 5 telemetry-only kind — never reaches the fallback
		// switch under default SOFT classification, but mapped to
		// FailLoud as a safety net so an accidental promotion to
		// strict via pipeline_contract_strict_kinds does not silently
		// trigger a retry storm. Operators who promote this kind
		// must override the policy too.
		types.ViolRichnessRegression: FallbackFailLoud,
		// Phase 6 stage 7 — secondary citation does not directly
		// support the scalar literal under the AnswerSubject.Kind
		// lookup discipline. Citation choice is the extractor's
		// surface; remediation is to re-pick citations.
		types.ViolValueSecondaryCitationOffFocus: FallbackBackToExtract,
		// B6-F1 (post-shape consolidated audit, 2026-05-04):
		// cross-citation single-locus conflict. The extractor selects
		// citations + names the principal items; resolving the
		// conflict means re-picking which (file, line) is the
		// canonical locus for the symbol. SOFT-by-default so the
		// answer ships with the conflict noted; promotion to STRICT
		// triggers BackToExtract retry (extractor must converge).
		types.ViolCrossCitationConflict: FallbackBackToExtract,
		// R10 CGEC frequency bridges (post_shape_residual_audit.md,
		// 2026-05-04). Telemetry-only — operator-side review signals,
		// no LLM retry can fix them (the LLM does not directly
		// control chain demotion / forced read counts). Mapped to
		// FailLoud as a safety net: if an operator accidentally
		// promotes them via pipeline_contract_strict_kinds, the run
		// fails fast rather than retry-storming on a non-LLM-actionable
		// signal.
		types.ViolDemotionStorm:   FallbackFailLoud,
		types.ViolForcedReadStorm: FallbackFailLoud,
	}
}

// activeFallbackPolicy is the runtime-mutable copy. Operators flip
// individual entries via SetFallbackPolicyOverrides; the production
// hot path reads from this map.
var activeFallbackPolicy = DefaultFallbackPolicy()

// SetFallbackPolicyOverrides applies a yaml-supplied override map to
// the active policy. Each entry of the form "kind:target" replaces
// the corresponding default; unknown kinds / targets are silently
// dropped. Operator-supplied "back_to_analyze" targets are
// REJECTED (silent-degraded to "finalizer_only" + WARN log) per
// the audit-methodology red line: re-classification of the user's
// request is fail-loud at the design level — the system cannot
// legitimately decide on its own to re-classify, no matter how
// the operator wires the policy. Restoring defaults: pass nil or
// an empty map.
func SetFallbackPolicyOverrides(overrides map[string]string) {
	policy := DefaultFallbackPolicy()
	for kindStr, targetStr := range overrides {
		kind := types.ViolationKind(strings.TrimSpace(kindStr))
		target := FallbackTarget(strings.TrimSpace(targetStr))
		if !target.IsValid() {
			continue
		}
		if target == FallbackBackToAnalyze {
			// Audit B.2 (2026-05-02): yaml override cannot legitimately
			// route any kind to BackToAnalyze. ResetForFallback(Analyze)
			// is a no-op and re-classifying the user's request is a
			// product red line. Degrade silently to FinalizerOnly so the
			// retry loop still makes progress, but log loudly so the
			// operator notices the override was rejected.
			logging.Warning(
				"[fallback] yaml override %q→back_to_analyze REJECTED: re-classification is a design red line; degraded to finalizer_only",
				kind,
			)
			target = FallbackFinalizerOnly
		}
		policy[kind] = target
	}
	activeFallbackPolicy = policy
}

// FallbackTargetForKind returns the configured FallbackTarget for
// the given ViolationKind. Unknown kinds default to
// FallbackFinalizerOnly — the safest fallback that always makes
// forward progress without burning evidence.
func FallbackTargetForKind(kind types.ViolationKind) FallbackTarget {
	if t, ok := activeFallbackPolicy[kind]; ok {
		return t
	}
	return FallbackFinalizerOnly
}

// FallbackTargetForViolation returns the FallbackTarget for one
// specific violation, including any per-violation overrides that
// inspect violation.Detail. Introduced in B3-F2 (2026-05-04) to
// fix P36: ViolBlockCoverageMissing was statically mapped to
// BackToExtract, but when the missing block is `diagram` or
// `table` (kinds the finalizer alone can re-emit from existing
// evidence) the upstream fallback is wasteful and slow.
//
// The override rule is conservative: only the Detail patterns we
// can confidently classify trigger a non-default target. Any
// uncertain Detail falls back to the kind's default policy entry,
// preserving pre-B3 behaviour for those cases.
func FallbackTargetForViolation(v types.Violation) FallbackTarget {
	def := FallbackTargetForKind(v.Kind)
	if v.Kind == types.ViolBlockCoverageMissing {
		// Detail format example:
		//   "required block kind=diagram appears 0 time(s) in answer; the family contract requires at least 1"
		// or:
		//   "required block kind=scalar appears 2 time(s); the family contract caps it at 1"
		//
		// kind=diagram / kind=table missing blocks are pure
		// finalizer-emit failures (the visualisation / tabulation
		// is constructed from already-present evidence; the
		// extractor has no diagram/table channel to re-pick from).
		// Overflow / count-cap variants stay BackToExtract because
		// the extractor's slate may need to drop items.
		detail := strings.ToLower(v.Detail)
		if strings.Contains(detail, "appears 0 time(s)") {
			if strings.Contains(detail, "kind=diagram") || strings.Contains(detail, "kind=table") {
				return FallbackFinalizerOnly
			}
		}
	}
	return def
}

// FallbackTargetForViolations returns the FallbackTarget for the
// supplied violation set, picking by **primary repair locus** (the
// locus that owns the most violations in the set). Replaces the
// pre-B3 "deepest target wins" rule — see RepairLocus comment for
// rationale and the post_shape_retirement_consolidated_audit.md
// §8 Batch B3 entry for the issue this fixes (P10 / P36).
//
// Empty input returns FallbackFinalizerOnly — caller treats that as
// "no upstream fallback needed".
//
// Algorithm:
//
//   1. Collect (locus, target) for each violation via
//      FallbackTargetForViolation (per-violation Detail-aware).
//   2. Bucket by locus and count.
//   3. Pick the locus with the highest count as the **primary
//      repair locus**. On tie, prefer the deepest locus (Terminal >
//      Explore > Extract > Finalizer) — when equal-count buckets
//      disagree the safer choice is to over-rebuild rather than
//      under-repair.
//   4. Return the target aligned with that locus, **except** when
//      any single violation in the set is FailLoud — that one
//      still drags the whole retry to FailLoud regardless of
//      majority (failure-of-last-resort red line).
func FallbackTargetForViolations(vs []types.Violation) FallbackTarget {
	// math.MaxInt32 effectively disables the R2.2 downgrade — the
	// budget-less wrapper is byte-identical to pre-R2.2 behaviour
	// (existing tests and any caller that hasn't opted into the
	// per-Run counter).
	return FallbackTargetForViolationsWithBudget(vs, 1<<30)
}

// FallbackTargetForViolationsWithBudget routes a violation set to
// a FallbackTarget using the Phase 1-A3 cooccurrence-cluster algorithm
// (BuildRepairPlan in repair_plan.go), with the R2.2 finalize-local
// priority preserved as a cost-optimization tiebreak.
//
// Phase 1-A3 (V2 runtime consolidation, 2026-05-04) replaced the
// pre-A3 "bucket-by-locus + count + deepest-tiebreak" model:
//
//   Pre-A3 model picked the locus with the MOST violations as primary.
//   This treats each violation as independent — when one root cause
//   emits 3+ derived consequences, the count tilted the picker toward
//   the locus owning the consequences instead of the locus owning the
//   root cause. Tests covered this as "majority wins"; the model is
//   correct when violations ARE independent but wrong when they share
//   a typed cause→consequence relationship.
//
//   Post-A3 model uses BuildRepairPlan: typed cooccurrence rules
//   (repair_cooccurrence.go) cluster violations by root cause; each
//   cluster has ONE Primary + zero-or-more Derived; PrimaryOwner is
//   the deepest cluster Owner. Derived do NOT contribute to owner
//   selection — fixing Primary necessarily clears them. Independent
//   violations remain singleton clusters, so a single explore-local
//   issue cannot be drowned out by N finalize-local issues from a
//   different root cause.
//
// R2.2 finalize-local downgrade preserved: when primary is deeper
// than Finalizer AND any cluster has Finalizer owner AND used <
// FinalizerLocalRetryBudget(), the result is downgraded to
// FinalizerOnly so the cheap re-emit gets up to budget attempts
// before the run escalates upstream. The downgrade is COST-OPT, not
// correctness — at budget exhaustion the original PrimaryOwner
// pick activates.
//
// finalizerLocalUsed is the per-Run counter the caller increments
// each time this picker downgrades; when used >= budget, downgrade
// stops firing.
func FallbackTargetForViolationsWithBudget(vs []types.Violation, finalizerLocalUsed int) FallbackTarget {
	if len(vs) == 0 {
		return FallbackFinalizerOnly
	}

	// A3: route via BuildRepairPlan. The plan returns FailLoud
	// outright when any violation has LocusTerminal owner — that
	// failure-of-last-resort semantic is preserved exactly.
	plan := BuildRepairPlan(vs)
	if plan.HasFailLoud {
		return FallbackFailLoud
	}

	primary := targetForLocus(plan.PrimaryOwner)

	// R2.2 (2026-05-04) finalize-local priority preserved. When the
	// primary pick is deeper than Finalizer BUT at least one cluster
	// has Finalizer owner AND we still have budget, downgrade to
	// FinalizerOnly so the cheap re-emit gets a try first.
	hasFinalizerLocal := false
	for _, c := range plan.Clusters {
		if c.Owner == LocusFinalizer {
			hasFinalizerLocal = true
			break
		}
	}
	budget := FinalizerLocalRetryBudget()
	if budget > 0 && finalizerLocalUsed < budget && hasFinalizerLocal {
		switch primary {
		case FallbackBackToExtract, FallbackBackToExplore, FallbackBackToAnalyze:
			return FallbackFinalizerOnly
		}
	}
	return primary
}

// finalizerLocalRetryBudget is the per-Run cap on how many times
// FallbackTargetForViolationsWithBudget will downgrade a deeper
// primary locus to FinalizerOnly. When the per-Run counter reaches
// this budget, the original primary-locus pick activates and the
// retry escalates upstream as before. Default 2 — gives finalize
// two cheap re-emit attempts before paying for upstream rework.
//
// Wired via cmd/root.go from yaml
// `pipeline_finalizer_local_retries_before_escalate`. Tests may
// override directly via SetFinalizerLocalRetryBudget.
var finalizerLocalRetryBudget = 2

// FinalizerLocalRetryBudget exposes the current budget for the
// fallback picker.
func FinalizerLocalRetryBudget() int {
	return finalizerLocalRetryBudget
}

// SetFinalizerLocalRetryBudget overrides the budget. Non-positive
// values reset to default (2). Call from cmd/root.go after merging
// codrax.yaml.
func SetFinalizerLocalRetryBudget(n int) {
	if n <= 0 {
		finalizerLocalRetryBudget = 2
		return
	}
	finalizerLocalRetryBudget = n
}

// PolicySnapshot returns a defensive copy of the active policy for
// tests and observability. Keys are sorted by ViolationKind string
// to keep the output deterministic.
func PolicySnapshot() []struct {
	Kind   types.ViolationKind
	Target FallbackTarget
} {
	keys := make([]string, 0, len(activeFallbackPolicy))
	for k := range activeFallbackPolicy {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	out := make([]struct {
		Kind   types.ViolationKind
		Target FallbackTarget
	}, 0, len(keys))
	for _, k := range keys {
		out = append(out, struct {
			Kind   types.ViolationKind
			Target FallbackTarget
		}{
			Kind:   types.ViolationKind(k),
			Target: activeFallbackPolicy[types.ViolationKind(k)],
		})
	}
	return out
}
