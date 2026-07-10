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
//   - Non-principal blocks (SurfaceRole empty) are NEVER hedged —
//     only principal blocks (the answer's main-line payload) carry
//     hedge sentinels.
//
// Returns the in-place mutated doc. Nil-safe.
//
// Per the feedback_no_system_backfill_to_user_panel red line, this
// function is a render-time TRANSFORM — it mutates the doc passed
// to the renderer, NOT the doc stored on Mutable. Callers (the
// finalizer's ParseOutput) work on a defensive clone via
// MutableState.AnswerDocumentV2(), so the on-disk Mutable copy
// stays untouched.
func ApplyAuthorityHedging(doc *types.AnswerDocumentV2, evidence []types.EvidenceItem, lang string) *types.AnswerDocumentV2 {
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
	upsert := func(text string) string {
		trimmed := strings.TrimLeft(text, " \t")
		leading := text[:len(text)-len(trimmed)]
		if ok, existing := leadingHedgeMarker(trimmed); ok {
			// Repeated render/finalize passes must converge. Keep an equal or
			// stronger marker; when authority tightens, replace the old system
			// marker (and its optional system-generated reason) instead of
			// stacking another prefix.
			if hedgeMarkerSeverity(existing) >= hedgeMarkerSeverity(marker) {
				return text
			}
			trimmed = strings.TrimSpace(stripLeadingMarkerAndReason(trimmed))
		}
		if trimmed == "" {
			return leading + marker
		}
		return leading + marker + " " + trimmed
	}
	if strings.TrimSpace(blk.Text) != "" {
		blk.Text = upsert(blk.Text)
		return
	}
	for j := range blk.Items {
		if strings.TrimSpace(blk.Items[j].Text) != "" {
			blk.Items[j].Text = upsert(blk.Items[j].Text)
			return
		}
		if strings.TrimSpace(blk.Items[j].Label) != "" {
			blk.Items[j].Label = upsert(blk.Items[j].Label)
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
	caveatText := authorityCaveatText(authority.AuthorityHistogram(evidence), l)
	target := -1
	// Reconcile, rather than append, the private-tagged system paragraph.
	// This makes finalize/replay idempotent while preserving every
	// user/model-authored caveat paragraph verbatim.
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind != types.BlockCaveat {
			continue
		}
		if target < 0 {
			target = i
		}
		paragraphs := strings.Split(doc.Blocks[i].Text, "\n\n")
		kept := paragraphs[:0]
		for _, paragraph := range paragraphs {
			if strings.Contains(paragraph, authorityCaveatTag) {
				continue
			}
			kept = append(kept, paragraph)
		}
		doc.Blocks[i].Text = strings.TrimSpace(strings.Join(kept, "\n\n"))
	}
	if caveatText == "" {
		return
	}
	if target >= 0 {
		if existing := strings.TrimSpace(doc.Blocks[target].Text); existing != "" {
			doc.Blocks[target].Text = existing + "\n\n" + caveatText
		} else {
			doc.Blocks[target].Text = caveatText
		}
		return
	}
	// Otherwise append a new caveat block.
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:   "_authority_caveat",
		Kind: types.BlockCaveat,
		Text: caveatText,
	})
}
