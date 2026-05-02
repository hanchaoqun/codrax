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
//   ViolShapeIntentMismatch      → FinalizerOnly
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
//   ViolShapeSwap                → FinalizerOnly
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
//     kinds) → FailLoud (informational; they should not drive retries)
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
		types.ViolSelfContradiction:           FallbackBackToExplore,
		types.ViolShapeIntentMismatch:         FallbackFinalizerOnly,
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
		types.ViolShapeSwap:                   FallbackFinalizerOnly,
		types.ViolSubTopicCountMismatch:       FallbackFinalizerOnly,
		types.ViolShape:                       FallbackFinalizerOnly,
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
	}
}

// activeFallbackPolicy is the runtime-mutable copy. Operators flip
// individual entries via SetFallbackPolicyOverrides; the production
// hot path reads from this map.
var activeFallbackPolicy = DefaultFallbackPolicy()

// SetFallbackPolicyOverrides applies a yaml-supplied override map to
// the active policy. Each entry of the form "kind:target" replaces
// the corresponding default; unknown kinds / targets are silently
// dropped (with a logged warning at the cmd layer). Restoring
// defaults: pass nil or an empty map.
func SetFallbackPolicyOverrides(overrides map[string]string) {
	policy := DefaultFallbackPolicy()
	for kindStr, targetStr := range overrides {
		kind := types.ViolationKind(strings.TrimSpace(kindStr))
		target := FallbackTarget(strings.TrimSpace(targetStr))
		if !target.IsValid() {
			continue
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

// FallbackTargetForViolations returns the deepest FallbackTarget
// across the supplied violation set. "Deepest" follows the order
// FailLoud > BackToAnalyze > BackToExplore > BackToExtract >
// FinalizerOnly so a single high-signal violation can drag the
// whole retry up the pipeline. Empty input returns
// FallbackFinalizerOnly — caller treats that as "no upstream
// fallback needed".
func FallbackTargetForViolations(vs []types.Violation) FallbackTarget {
	rank := func(t FallbackTarget) int {
		switch t {
		case FallbackFinalizerOnly:
			return 1
		case FallbackBackToExtract:
			return 2
		case FallbackBackToExplore:
			return 3
		case FallbackBackToAnalyze:
			return 4
		case FallbackFailLoud:
			return 5
		}
		return 0
	}
	best := FallbackFinalizerOnly
	for _, v := range vs {
		t := FallbackTargetForKind(v.Kind)
		if rank(t) > rank(best) {
			best = t
		}
	}
	return best
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
