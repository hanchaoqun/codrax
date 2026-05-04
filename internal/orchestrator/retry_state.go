package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// retry_state.go — R14-c3 + c4 (post_shape_residual_audit.md, 2026-05-04).
// Populates types.RetryState from contract.Result + the latest
// finalizer emit before each retry dispatch. The agent layer's
// renderRetryState consumes this on the next BuildInitialInstruction
// to surface "Previous Emit / Active Violations / Required Changes /
// Hard Rule" sections to the LLM.
//
// Single-source-of-truth principle (R14): every retry-decision site
// in orchestrator.go calls populateRetryState BEFORE requeuing the
// finalizer. Producer Layer tagging happens here based on
// ViolationKind (kind → layer is a deterministic map; if a future
// producer needs finer-grained layer info, extend the map).

// populateRetryState writes types.RetryState to the bus mutable
// after a contract.Check failure, capturing:
//
//   1. The previous emit (PrevEmitJSON + PrevEmitSummary projection)
//      so the LLM sees what fields it already filled.
//   2. Every violation in the result, scored with severity / layer
//      / field path so the LLM can prioritise.
//   3. The retry attempt counter so renderers know we're in a retry.
//
// Called BEFORE state.requeue(fin.ID) so the next finalizer dispatch
// observes the populated state on its first BuildInitialInstruction
// pass.
func populateRetryState(mut *types.MutableState, res contract.Result, prevAttempt int) {
	if mut == nil {
		return
	}
	rs := &types.RetryState{
		Attempt: prevAttempt + 1,
	}
	if doc := mut.AnswerDocumentV2(); doc != nil {
		rs.PrevEmitSummary = summarizeAnswerDocV2ForRetry(doc)
		// PrevEmitJSON: serialise the V2 doc as canonical JSON so
		// the LLM has the verbatim previous payload as a fallback
		// when summary alone is insufficient. Best-effort —
		// serialisation failure is non-fatal (summary still
		// renders).
		if raw, err := json.Marshal(doc); err == nil {
			rs.PrevEmitJSON = raw
		}
	}
	rs.ActiveViolations = scoreViolations(res.Violations)
	mut.SetRetryState(rs)
}

// scoreViolations turns the contract.Check Violation slice into
// the structured ScoredViolation slice consumed by RetryState.
// Severity comes from types.DeriveSeverity (single source of truth);
// Layer is inferred from kind via inferViolationLayer; FieldPath is
// extracted from the violation Detail when present (e.g. block id).
func scoreViolations(in []contract.Violation) []types.ScoredViolation {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.ScoredViolation, 0, len(in))
	for _, v := range in {
		isStrict := !isSoftViolationKind(v.Kind)
		sv := types.ScoredViolation{
			Kind:      v.Kind,
			Detail:    v.Detail,
			Repair:    v.Repair,
			Severity:  types.DeriveSeverity(v.Kind, isStrict),
			Layer:     inferViolationLayer(v.Kind),
			BlockID:   extractBlockIDFromDetail(v.Detail),
			FieldPath: inferFieldPathFromKind(v.Kind, v.Detail),
		}
		out = append(out, sv)
	}
	return out
}

// inferViolationLayer maps each ViolationKind to the gating layer
// that produces it. Used by RetryState rendering to group active
// violations by Layer (gives the LLM cross-layer visibility — fixes
// R13 by ensuring the LLM sees scheduler-level + V2 oracle + contract
// check violations together).
//
// When a kind has multiple producers (rare), prefer the most
// frequent. Unknown kinds default to "contract_check".
func inferViolationLayer(kind types.ViolationKind) string {
	switch kind {
	// V2 oracle layer (runV2BlockOraclesWithMut + V2 carrier oracles
	// in contract_check.go).
	case types.ViolBlockCoverageMissing,
		types.ViolPrincipalClaimUseMissing,
		types.ViolDiagramEdgeUnsupported,
		types.ViolUncertaintyBlockMissing,
		types.ViolFacetUncovered,
		types.ViolRichnessRegression,
		types.ViolClaimFormUnsupported,
		types.ViolAbsenceScopeExceeded,
		types.ViolStructuralEnumerationDivergence,
		types.ViolSymbolAnchorMismatch,
		types.ViolCrossCitationConflict,
		types.ViolDiagramIdentifier,
		types.ViolDeclaredCountDrift:
		return "v2_oracle"

	// Self-consistency reviewer (R2.1 V2 重接).
	case types.ViolSelfContradiction:
		return "self_consistency"

	// External artifact decoded check.
	case types.ViolExternalArtifactUnderdecoded:
		return "external_artifact"

	// Authority overreach check.
	case types.ViolAuthorityOverreach:
		return "authority"

	// CGEC / explorer-level enforcer kinds.
	case types.ViolGhostAnchor,
		types.ViolChainDemoted,
		types.ViolSelfRefLiteral,
		types.ViolPreCompleteDowngrade,
		types.ViolLiteralFormFailed,
		types.ViolViewSwap:
		return "cgec"

	// Reviewer-side telemetry kinds.
	case types.ViolPlanCritic,
		types.ViolReflectorObservation,
		types.ViolAnswerReviewerDistilled:
		return "reviewer"

	// Block 2 intent / subject / axis oracles (when V1 dispatch
	// is restored or V2 sibling lands).
	case types.ViolViewIntentMismatch,
		types.ViolSubTopicCountMismatch,
		types.ViolIntentTraceShallow,
		types.ViolIntentEnumerateNotList,
		types.ViolIntentRootCauseNoCause,
		types.ViolIntentConfigNoTrail,
		types.ViolSubjectAnchorMissing,
		types.ViolPredicateAxisMissing,
		types.ViolStepIdentifierUnverified,
		types.ViolValueSecondaryCitationOffFocus:
		return "answer_oracle"
	}

	// contract.Check classic kinds:
	// ViolFamilyMismatch / ViolCitation / ViolMustInclude /
	// ViolMustExclude / ViolAcceptance / ViolSuccessCriterion default
	// to contract_check layer.
	return "contract_check"
}

// extractBlockIDFromDetail tries to pull a block id out of the
// violation Detail string when present. Conservative — only
// matches the canonical pattern `block id="<X>"` that the V2
// oracles use; returns "" on no match.
func extractBlockIDFromDetail(detail string) string {
	const marker = `block id="`
	idx := strings.Index(detail, marker)
	if idx < 0 {
		return ""
	}
	rest := detail[idx+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// inferFieldPathFromKind suggests the V2 field path the LLM should
// edit to fix this violation. Conservative — only specific kinds
// have stable field paths; the rest fall back to the empty string
// (LLM uses Detail/Repair prose instead).
//
// Field path syntax: dotted-bracket form so the LLM can match
// against its emit JSON. Examples:
//
//	blocks[id="lifecycle"].claim_use
//	blocks[id="lifecycle"].facet_ids
//	citations[*].file
//	exact_resolution.status
func inferFieldPathFromKind(kind types.ViolationKind, detail string) string {
	blockID := extractBlockIDFromDetail(detail)
	switch kind {
	case types.ViolPrincipalClaimUseMissing:
		if blockID != "" {
			return fmt.Sprintf("blocks[id=%q].claim_use", blockID)
		}
		return "blocks[*].claim_use"
	case types.ViolFacetUncovered:
		// Detail names the missing facet but the FIX is to set
		// facet_ids on some block — we can't say which one
		// here, so the LLM picks via reverse-lookup hint from R7.
		return "blocks[*].facet_ids"
	case types.ViolBlockCoverageMissing:
		// Detail names the missing block kind. Field path is
		// "add a new block of that kind".
		return "blocks (add new block kind=...)"
	case types.ViolDiagramEdgeUnsupported:
		if blockID != "" {
			return fmt.Sprintf("blocks[id=%q].diagram", blockID)
		}
		return "blocks[kind=diagram].diagram"
	case types.ViolUncertaintyBlockMissing:
		return "blocks (add new block kind=caveat)"
	case types.ViolClaimFormUnsupported:
		if blockID != "" {
			return fmt.Sprintf("blocks[id=%q].claim_use.claim_form", blockID)
		}
		return "blocks[*].claim_use.claim_form"
	case types.ViolAbsenceScopeExceeded:
		return "exact_resolution OR citations[*] (add scope=negative + negative_pattern)"
	case types.ViolCitation:
		return "citations[]"
	case types.ViolMustInclude, types.ViolMustExclude:
		return "answer text"
	case types.ViolDiagramIdentifier:
		if blockID != "" {
			return fmt.Sprintf("blocks[id=%q].diagram.body", blockID)
		}
		return "blocks[kind=diagram].diagram.body"
	}
	return ""
}

// summarizeAnswerDocV2ForRetry builds a RetryStateSummary from the
// previous V2 emit. Captures the typed fields R6/R6.1 found being
// silently dropped on retry (block-level claim_use existence +
// facet_ids verbatim + surface_role) so the LLM sees what it
// already filled and the Hard Rule rendering can demand
// preservation.
func summarizeAnswerDocV2ForRetry(doc *types.AnswerDocumentV2) types.RetryStateSummary {
	if doc == nil {
		return types.RetryStateSummary{}
	}
	out := types.RetryStateSummary{
		CitationsCount:     len(doc.Citations),
		HasExactResolution: doc.ExactResolution != nil,
	}
	if len(doc.Citations) > 0 {
		// Top-N (cap 8) verbatim file paths so the LLM can
		// match its citations[] back to prev emit.
		max := len(doc.Citations)
		if max > 8 {
			max = 8
		}
		seen := make(map[string]bool, max)
		for i := 0; i < len(doc.Citations) && len(out.CitationFiles) < max; i++ {
			f := strings.TrimSpace(doc.Citations[i].File)
			if f == "" || seen[f] {
				continue
			}
			seen[f] = true
			out.CitationFiles = append(out.CitationFiles, f)
		}
	}
	out.BlockSummaries = make([]types.RetryBlockSummary, 0, len(doc.Blocks))
	for _, b := range doc.Blocks {
		bs := types.RetryBlockSummary{
			ID:          b.ID,
			Kind:        b.Kind,
			SurfaceRole: b.SurfaceRole,
			FacetIDs:    append([]string(nil), b.FacetIDs...),
			HasItems:    len(b.Items) > 0,
			ItemCount:   len(b.Items),
		}
		// Block-level claim_use presence (R6.1 historical drop site).
		if len(b.ClaimUses) > 0 {
			bs.HasClaimUse = true
			// Pick first non-empty form for summary.
			for _, cu := range b.ClaimUses {
				if cu.ClaimForm != "" {
					bs.ClaimForm = cu.ClaimForm
					break
				}
			}
		}
		// Item-level claim_use / citation count (R6.1 sibling layer).
		for _, it := range b.Items {
			if it.ClaimUse != nil && !it.ClaimUse.IsEmpty() {
				bs.ItemsWithClaimUse++
			}
			if it.CitationRef >= 0 {
				bs.ItemsWithCitation++
			}
		}
		// Text preview (head 400 + tail 200 when over 600 chars).
		bs.TextPreview = textHeadTail(b.Text, 400, 200)
		out.BlockSummaries = append(out.BlockSummaries, bs)
	}
	return out
}

// textHeadTail clips s to head + tail when over head+tail+marker
// length. Returns the full string when shorter. The marker
// "…<truncated N chars>…" surfaces the omission so the LLM
// doesn't quietly assume the prev text was just head+tail.
func textHeadTail(s string, head, tail int) string {
	r := []rune(s)
	if len(r) <= head+tail+10 {
		return s
	}
	dropped := len(r) - head - tail
	return string(r[:head]) + fmt.Sprintf("…<truncated %d chars>…", dropped) + string(r[len(r)-tail:])
}

// _ keeps agent import used (re-export bridge for future c5+ render
// helpers that may move into a sibling file). Removed in c5.
var _ = agent.StageOutput{}
