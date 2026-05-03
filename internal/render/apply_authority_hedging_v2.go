package render

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/authority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ApplyAuthorityHedgingV2 is the V2 carrier equivalent of
// ApplyAuthorityHedging (B5 落地). It projects an AnswerDocumentV2 +
// evidence pool into a hedge-aware AnswerDocumentV2 the V2 renderer
// can serialise without further awareness.
//
// Per docs/migration/block_only_carrier.md §5.5 design:
//   - hedging operates at BLOCK level, not at V1 field level. A
//     block whose principal payload rests on
//     conditional / historical evidence gets a hedge sentinel
//     prepended to its Text (or its first item's Text), and a
//     dedicated BlockCaveat block appended at document end carries
//     the bilingual drift caveat (instead of V1's doc.Caveats[]
//     auto-injection).
//   - Diagram blocks are NOT hedged in-place — the diagram body
//     stays verbatim; the dedicated BlockCaveat carries the
//     "diagram lines may have drifted from runtime" disclosure.
//   - SurfaceRole=support / prose_only / diagram_only blocks are
//     NEVER hedged — only principal blocks (the answer's main-line
//     payload) carry hedge sentinels.
//
// Returns the in-place mutated doc. Nil-safe.
//
// Per the feedback_no_system_backfill_to_user_panel red line, this
// function is a render-time TRANSFORM — it mutates the doc passed
// to the renderer, NOT the doc stored on Mutable. Callers (the
// finalizer's ParseOutput) work on a defensive clone via
// MutableState.AnswerDocumentV2(), so the on-disk Mutable copy
// stays untouched.
func ApplyAuthorityHedgingV2(doc *types.AnswerDocumentV2, evidence []types.EvidenceItem, lang string) *types.AnswerDocumentV2 {
	if doc == nil {
		return doc
	}
	l := normalizeAnswerDocLang(lang)

	hedgeV2PrincipalBlocks(doc, evidence, l)
	addV2AuthorityCaveat(doc, evidence, l)

	return doc
}

// hedgeV2PrincipalBlocks walks blocks; for each principal-role block
// whose first associated citation rests on non-factual evidence,
// prefix the block's Text (or first item's Text) with the strongest
// required hedge sentinel. Skip non-principal / caveat blocks.
func hedgeV2PrincipalBlocks(doc *types.AnswerDocumentV2, evidence []types.EvidenceItem, l answerDocLang) {
	if len(doc.Blocks) == 0 || len(doc.Citations) == 0 {
		return
	}
	for i := range doc.Blocks {
		blk := &doc.Blocks[i]
		// Skip caveat blocks (already hedge-shaped) + diagram bodies.
		if blk.Kind == types.BlockCaveat || blk.Kind == types.BlockDiagram {
			continue
		}
		// Skip non-principal blocks: support / prose_only /
		// diagram_only blocks are not load-bearing for hedging.
		if blk.SurfaceRole != "" && blk.SurfaceRole != types.SurfacePrincipal {
			continue
		}
		marker := strongestHedgeForV2Block(*blk, doc.Citations, evidence)
		if marker == "" {
			continue
		}
		applyHedgeMarkerToV2Block(blk, marker, l)
	}
}

// strongestHedgeForV2Block walks the block's items[] (and the
// block-level citations indirectly via items' CitationRef) to find
// the strongest authority ceiling that requires hedging. Returns
// the hedge marker string, or "" when no marker applies.
func strongestHedgeForV2Block(blk types.AnswerBlock, citations []types.Citation, evidence []types.EvidenceItem) string {
	var top types.AuthorityCeiling
	considered := false
	consider := func(refIdx int) {
		if refIdx < 0 || refIdx >= len(citations) {
			return
		}
		c := citations[refIdx]
		ceil := authority.HighestAuthorityFor(evidence, c.File, c.Line)
		if !considered {
			top = ceil
			considered = true
			return
		}
		if hedgeMarkerSeverity(hedgeMarkerFor(ceil)) > hedgeMarkerSeverity(hedgeMarkerFor(top)) {
			top = ceil
		}
	}
	for _, it := range blk.Items {
		consider(it.CitationRef)
	}
	if !considered {
		return ""
	}
	return hedgeMarkerFor(top)
}

// applyHedgeMarkerToV2Block prepends the marker to the block's Text
// when non-empty, otherwise to the first item's Text. Keeps existing
// content; the marker becomes the leading prose so the renderer
// emits it before the body.
func applyHedgeMarkerToV2Block(blk *types.AnswerBlock, marker string, _ answerDocLang) {
	if marker == "" {
		return
	}
	if strings.TrimSpace(blk.Text) != "" {
		blk.Text = marker + " " + blk.Text
		return
	}
	for j := range blk.Items {
		if strings.TrimSpace(blk.Items[j].Text) != "" {
			blk.Items[j].Text = marker + " " + blk.Items[j].Text
			return
		}
		if strings.TrimSpace(blk.Items[j].Label) != "" {
			blk.Items[j].Label = marker + " " + blk.Items[j].Label
			return
		}
	}
}

// addV2AuthorityCaveat appends a BlockCaveat block to doc.Blocks
// when the evidence pool carries any drift-bounded items. Mirrors
// V1's addAuthorityCaveat (which writes to doc.Caveats[]) but uses
// the V2 carrier's caveat block surface.
//
// The caveat block carries SurfaceRole=ProseOnly so V2 validators
// don't treat it as principal and require a claim_use.
func addV2AuthorityCaveat(doc *types.AnswerDocumentV2, evidence []types.EvidenceItem, l answerDocLang) {
	caveatText := authorityCaveatProse(evidence, l)
	if caveatText == "" {
		return
	}
	// If a caveat block already exists, append rather than duplicate.
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind == types.BlockCaveat {
			if existing := strings.TrimSpace(doc.Blocks[i].Text); existing != "" {
				doc.Blocks[i].Text = existing + "\n\n" + caveatText
			} else {
				doc.Blocks[i].Text = caveatText
			}
			return
		}
	}
	// Otherwise append a new caveat block.
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:          "_authority_caveat",
		Kind:        types.BlockCaveat,
		Text:        caveatText,
		SurfaceRole: types.SurfaceProseOnly,
	})
}

// authorityCaveatProse returns the bilingual caveat prose when the
// evidence pool contains any drift-bounded items, or "" otherwise.
// Reuses the V1 hedging templates (en/zh) so wording is consistent
// across V1+V2 during the migration window.
func authorityCaveatProse(evidence []types.EvidenceItem, l answerDocLang) string {
	hasConditional, hasHistorical, hasIllustrative := false, false, false
	for _, e := range evidence {
		switch e.Authority {
		case types.AuthorityConditional:
			hasConditional = true
		case types.AuthorityHistorical:
			hasHistorical = true
		case types.AuthorityIllustrative:
			hasIllustrative = true
		}
	}
	if !hasConditional && !hasHistorical && !hasIllustrative {
		return ""
	}
	parts := make([]string, 0, 3)
	if l == answerDocLangZH {
		if hasConditional {
			parts = append(parts, "部分论据来自条件分支，结论在该条件下成立")
		}
		if hasHistorical {
			parts = append(parts, "部分论据来自历史快照（log/perf），与当前源码可能已漂移")
		}
		if hasIllustrative {
			parts = append(parts, "部分论据是说明性的，不具直接因果约束力")
		}
		return strings.Join(parts, "；") + "。"
	}
	if hasConditional {
		parts = append(parts, "some evidence is conditional and the conclusion holds under that branch")
	}
	if hasHistorical {
		parts = append(parts, "some evidence is historical (log / perf) and may have drifted from current source")
	}
	if hasIllustrative {
		parts = append(parts, "some evidence is illustrative and does not establish direct causation")
	}
	return strings.Join(parts, "; ") + "."
}
