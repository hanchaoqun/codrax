// Package tool — answer_document_pre_emit_check.go (P1, 2026-05-10).
//
// Emit-time pre-validation chokepoint. When the LLM calls
// emit_answer_document, this layer runs the four most common
// structural-compliance checks BEFORE the doc lands on Mutable.
// Violations are surfaced as a structured user-vocab fix-list
// inside failEmit's rejection envelope — the LLM retries within
// the same dispatch (BaseAgent.emitAnswerDocumentRejectSignal
// captures !LastToolResult.Success and re-prompts), so the cost is
// one tool-call round trip, NOT a full repair-loop iteration with
// orchestrator dispatch overhead.
//
// Pre-emit only covers the four STRICT-classified validator axes
// that the post-emit chain in internal/orchestrator/contract_check.go
// runs after persist:
//
//   - block_coverage_missing   (validateRequiredBlockCoverage)
//   - principal_claim_use_missing (validatePrincipalClaimUse)
//   - uncertainty_block_missing  (validateUncertaintyBlockPresence)
//   - facet_uncovered          (validateFacetCoverage)
//
// SOFT validators (richness regression / inline-code density / etc.)
// stay in the post-emit chain — they're advisory in nature and
// pre-empting them at emit time would add noise without reducing
// load-bearing failures.
//
// Forensic anchor: May-9 sweep showed block_coverage_missing +
// facet_uncovered + uncertainty_block + principal_claim_use_missing
// account for ~69% of repair-loop activations. Pre-emit chokepoint
// converts those from a full orchestrator retry round (3-5min) to
// a same-call tool retry (~5s).
//
// Architecture:
//
//   - The four checks are intentionally LIGHTWEIGHT re-implementations
//     of the post-emit logic (not copies of the full validator
//     functions). Pre-emit needs only "is the structural shape
//     compliant?" — it does NOT need the full types.Violation envelope
//     (cluster keys / suspected roots / repair locus mapping). That
//     distinction is the post-emit chain's job.
//   - Output is []emitFixHint with user-vocab Field+ExpectedShape
//     strings the failEmit rejection turns into a numbered list the
//     LLM can mechanically apply (matches the design doc's
//     "structured fix-list" surface).
//   - Empty fix-list → ApplyAndPersistMutation runs unchanged.
//   - This is a NEW chokepoint, not a replacement; the post-emit
//     chain remains the load-bearing correctness layer for SOFT
//     advisory cases + cluster-routed repair planning.
package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// emitFixHint is the user-facing fix instruction surfaced when a
// pre-emit check fails. Formatted by formatEmitFixHints into a
// rejection prose string that the LLM reads and acts on within the
// same tool dispatch.
//
// Field is the schema-level token (block.facet_ids /
// block.claim_uses / etc.) — matches the user-section vocab the
// finalizer skill prompt teaches. ExpectedShape names what the LLM
// should emit; Reason is the one-sentence "why" so the LLM
// understands the structural requirement, not just the literal fix.
type emitFixHint struct {
	Field          string
	ExpectedShape  string
	Reason         string
}

// runPreEmitChecks runs the 4 STRICT chokepoint checks on the
// just-built (but-not-yet-persisted) document. Returns nil when the
// shape is compliant; otherwise a non-empty list of fix hints the
// caller wraps in failEmit. view nil → returns nil (no view, no
// expected shape — pre-emit silently passes through and the
// post-emit chain takes over).
func runPreEmitChecks(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if doc == nil || view == nil {
		return nil
	}
	var hints []emitFixHint

	// 1. Required block kind + count compliance.
	if h := preCheckRequiredBlocks(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 2. Principal-block claim_use presence.
	if h := preCheckPrincipalClaimUse(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 3. Uncertainty block presence (when contract requires it).
	if h := preCheckUncertaintyBlock(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}

	// 4. Required facet coverage.
	if h := preCheckFacetCoverage(doc, view); len(h) > 0 {
		hints = append(hints, h...)
	}

	return hints
}

// preCheckRequiredBlocks mirrors the post-emit
// validateRequiredBlockCoverage core logic (block kind+count) but
// emits user-vocab fix-list entries instead of types.Violation.
func preCheckRequiredBlocks(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if len(view.RequiredBlocks) == 0 {
		return nil
	}
	counts := make(map[types.AnswerBlockKind]int, len(doc.Blocks))
	for _, b := range doc.Blocks {
		counts[b.Kind]++
	}
	var out []emitFixHint
	for _, req := range view.RequiredBlocks {
		if !req.Required {
			continue
		}
		got := counts[req.Kind]
		if got < req.MinCount {
			out = append(out, emitFixHint{
				Field: fmt.Sprintf("blocks[].kind=%s", req.Kind),
				ExpectedShape: fmt.Sprintf(
					"emit at least %d block(s) of kind=%s (currently emitted: %d)",
					req.MinCount, req.Kind, got),
				Reason: strings.TrimSpace(req.Rationale),
			})
			continue
		}
		if req.MaxCount > 0 && got > req.MaxCount {
			out = append(out, emitFixHint{
				Field: fmt.Sprintf("blocks[].kind=%s", req.Kind),
				ExpectedShape: fmt.Sprintf(
					"reduce kind=%s blocks to at most %d (currently emitted: %d)",
					req.Kind, req.MaxCount, got),
				Reason: strings.TrimSpace(req.Rationale),
			})
		}
	}
	return out
}

// preCheckPrincipalClaimUse mirrors validatePrincipalClaimUse's
// "principal block lacks claim_use annotation" branch + the
// 2026-05-04 single-AcceptableClaimForm relaxation that lets blocks
// with structural grounding (facet_ids non-empty + at least one
// item with citation_ref >= 0) skip the explicit claim_uses
// annotation. Returning a fix hint here directs the LLM to emit
// claim_uses[] before persist; without this the LLM ships and we
// pay a full orchestrator retry round.
func preCheckPrincipalClaimUse(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	reqByKind := make(map[types.AnswerBlockKind]types.BlockRequirement, len(view.RequiredBlocks))
	for _, r := range view.RequiredBlocks {
		if _, ok := reqByKind[r.Kind]; !ok {
			reqByKind[r.Kind] = r
		}
	}
	var out []emitFixHint
	for _, b := range doc.Blocks {
		req, ok := reqByKind[b.Kind]
		if !ok {
			continue
		}
		if len(req.AcceptableClaimForms) == 0 {
			continue
		}
		isPrincipal := b.SurfaceRole == types.SurfacePrincipal ||
			(b.SurfaceRole == "" && req.SurfaceRoleHint == types.SurfacePrincipal)
		if !isPrincipal {
			continue
		}
		if hasAnyClaimUse(b) {
			continue
		}
		// Single-form relaxation: when the contract declares exactly
		// one AcceptableClaimForm AND the block already carries
		// structural grounding, the LLM's emit is unambiguously
		// correct even without the explicit claim_uses[]. Skip
		// the fix hint in that path.
		if len(req.AcceptableClaimForms) == 1 && hasStructuralGrounding(b) {
			continue
		}
		formNames := make([]string, 0, len(req.AcceptableClaimForms))
		for _, f := range req.AcceptableClaimForms {
			formNames = append(formNames, fmt.Sprintf("%q", string(f)))
		}
		out = append(out, emitFixHint{
			Field: fmt.Sprintf("blocks[id=%q].claim_uses", b.ID),
			ExpectedShape: fmt.Sprintf(
				"add a one-element claim_uses[] array on this principal block; claim_form must be one of [%s]",
				strings.Join(formNames, ", ")),
			Reason: "principal blocks need a claim annotation so the answer's typed payload can be matched to its evidence shape",
		})
	}
	return out
}

// preCheckUncertaintyBlock checks the contract's uncertainty rules.
// Mirrors validateUncertaintyBlockPresence's load-bearing path:
// when an UncertaintyRule fires (resolved fact: a layer the user
// asked about has no grounded binding) and no caveat block sits in
// the doc, surface a fix hint.
//
// Conservative pre-emit shape: we fire the hint only when the
// view's UncertaintyRules slice is non-empty AND the doc has zero
// caveat blocks. The full post-emit validator runs the per-rule
// trigger logic; here we just want the LLM to emit the caveat at
// all, which is the most common miss.
func preCheckUncertaintyBlock(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if len(view.UncertaintyRules) == 0 {
		return nil
	}
	hasCaveat := false
	for _, b := range doc.Blocks {
		if b.Kind == types.BlockCaveat {
			hasCaveat = true
			break
		}
	}
	if hasCaveat {
		return nil
	}
	return []emitFixHint{{
		Field: "blocks[].kind=caveat",
		ExpectedShape: "emit a caveat block disclosing what was searched and what remained uncertain",
		Reason: "the question's contract carries uncertainty rules that require explicit boundary disclosure",
	}}
}

// preCheckFacetCoverage mirrors the post-emit validateFacetCoverage
// "required facet has no block declaring its facet_id" branch. We
// gate on req.IsPromoted() the same way the post-emit validator
// does, so SOFT/Optional facets do NOT trigger pre-emit retries.
func preCheckFacetCoverage(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	if view.FacetCoverage == nil || len(view.FacetCoverage.Required) == 0 {
		return nil
	}
	covered := make(map[string]bool, 8)
	for _, b := range doc.Blocks {
		for _, fid := range b.FacetIDs {
			covered[strings.TrimSpace(fid)] = true
		}
		for _, cu := range b.ClaimUses {
			if cu.FacetID != "" {
				covered[strings.TrimSpace(cu.FacetID)] = true
			}
		}
	}
	var out []emitFixHint
	for _, req := range view.FacetCoverage.Required {
		if req.EffectivePromotionPolicy() == types.PromotionAdvisoryOnly {
			continue
		}
		if !req.IsPromoted() {
			continue
		}
		kind := strings.TrimSpace(string(req.Kind))
		if kind == "" || covered[kind] {
			continue
		}
		out = append(out, emitFixHint{
			Field: "blocks[].facet_ids OR blocks[].claim_uses[].facet_id",
			ExpectedShape: fmt.Sprintf(
				"declare facet_id=%q on at least one block whose payload covers this facet",
				kind),
			Reason: "the question's contract requires this facet to be anchored on the answer surface",
		})
	}
	return out
}

// hasAnyClaimUse reports whether the block carries at least one
// block-level claim_uses[] entry. Mirrors the post-emit
// blockHasClaimUse helper but stays package-local in tool/.
func hasAnyClaimUse(b types.AnswerBlock) bool {
	return len(b.ClaimUses) > 0
}

// hasStructuralGrounding reports whether the block already carries
// the structural grounding the single-AcceptableClaimForm relaxation
// recognises (facet_ids non-empty + at least one item with
// citation_ref >= 0). Mirrors hasGroundedCoverage in
// internal/orchestrator/contract_check_block.go.
func hasStructuralGrounding(b types.AnswerBlock) bool {
	if len(b.FacetIDs) == 0 {
		return false
	}
	for _, item := range b.Items {
		if item.CitationRef >= 0 {
			return true
		}
	}
	return false
}

// formatEmitFixHints renders a fix-hint list as a human-readable
// numbered list embedded in the failEmit rejection prose. The LLM
// reads it inside the tool-result and re-emits within the same
// dispatch (BaseAgent's emit-rejection signal handling, see
// emitAnswerDocumentRejectSignal in internal/agent).
//
// Red-line audit (R3/R4/R5/R6/R7/SST/CN+EN-only):
//   - R3 typed: each Field is a schema field name the LLM emits;
//     ExpectedShape names a concrete action; Reason gives the
//     structural why
//   - R4 generic: no project / case names; templated
//   - R5: rejection envelope, never writes answer body
//   - R6 no internal vocab: schema field names + user-vocab prose;
//     "the answer document" is the user-visible carrier name (already
//     in skill prompt); no ViolKind / ClusterKey / SuspectedRoot /
//     "orchestrator" / "pre-emit" architectural terms surface
//   - R7: actionable next steps the LLM can mechanically apply;
//     explicitly names the "same tool call" retry budget so the
//     LLM doesn't escalate to a heavier rewrite path
//   - SST: matches the existing failEmit message shape (e.g.
//     "blocks[3]: id is required ...")
//   - CN+EN-only: prompt is English, matches the surrounding
//     failEmit envelopes used by NormalizeEmitAnswerBlock
func formatEmitFixHints(hints []emitFixHint) string {
	if len(hints) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The answer document does not yet meet the structural contract for this question. Apply each correction below and re-emit emit_answer_document in the SAME tool turn — this rejection is light-weight and does NOT consume a heavier rewrite round, just re-call with the fixes:\n\n")
	for i, h := range hints {
		fmt.Fprintf(&b, "  %d. Field: `%s`\n", i+1, h.Field)
		fmt.Fprintf(&b, "     Action: %s\n", h.ExpectedShape)
		if reason := strings.TrimSpace(h.Reason); reason != "" {
			fmt.Fprintf(&b, "     Why: %s\n", reason)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
