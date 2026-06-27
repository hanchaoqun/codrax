package types

// AnswerPrincipalEvidenceView is a compact, typed projection of whether an
// AnswerDocumentV2 already carries load-bearing evidence on its principal
// surface. It is derived only from block roles, citation_ref indexes, citation
// pool entries, and claim_use records; it never inspects user prose or
// model-authored rationale text.
type AnswerPrincipalEvidenceView struct {
	PrincipalBlocks                      int `json:"principal_blocks,omitempty"`
	PrincipalItems                       int `json:"principal_items,omitempty"`
	ValidCitations                       int `json:"valid_citations,omitempty"`
	ClaimUses                            int `json:"claim_uses,omitempty"`
	InvalidCitations                     int `json:"invalid_citations,omitempty"`
	PrincipalEnumerationBlocks           int `json:"principal_enumeration_blocks,omitempty"`
	PrincipalEnumerationItems            int `json:"principal_enumeration_items,omitempty"`
	PrincipalEnumerationValidCitations   int `json:"principal_enumeration_valid_citations,omitempty"`
	PrincipalEnumerationInvalidCitations int `json:"principal_enumeration_invalid_citations,omitempty"`
}

// HasPrincipalSurface reports whether the document contains at least one
// block explicitly marked as principal answer content.
func (v AnswerPrincipalEvidenceView) HasPrincipalSurface() bool {
	return v.PrincipalBlocks > 0
}

// HasGroundedPrincipalEvidence reports whether at least one principal block
// item cites a valid source citation. ClaimUses are intentionally not enough:
// they classify claim shape, while citations bind the visible surface to a
// concrete evidence anchor.
func (v AnswerPrincipalEvidenceView) HasGroundedPrincipalEvidence() bool {
	return v.HasPrincipalSurface() && v.ValidCitations > 0
}

// HasGroundedPrincipalEnumerationEvidence reports whether the visible
// principal enumeration surface is fully backed by valid citations. This is a
// typed presentation signal, not a prose scan: callers use it to avoid adding
// generic source-localization or degraded-floor caveats after an exhaustive
// member answer already carries per-item source anchors.
func (v AnswerPrincipalEvidenceView) HasGroundedPrincipalEnumerationEvidence() bool {
	return v.PrincipalEnumerationBlocks > 0 &&
		v.PrincipalEnumerationItems > 0 &&
		v.PrincipalEnumerationValidCitations == v.PrincipalEnumerationItems &&
		v.PrincipalEnumerationInvalidCitations == 0
}

// AnswerBlockHasStructuralPrincipalEnumerationCarrier reports whether a
// rendered block already carries a visible, cited principal item surface that
// can safely stand as the enumeration_item carrier even when the LLM omitted
// the non-visible facet metadata. It is intentionally narrower than general
// "visible payload": summaries, diagrams, uncited rows, and non-principal
// support lists do not qualify.
func AnswerBlockHasStructuralPrincipalEnumerationCarrier(doc *AnswerDocumentV2, block AnswerBlock) bool {
	if doc == nil || block.SurfaceRole != SurfacePrincipal || !AnswerBlockKindRendersStructuredItems(block.Kind) {
		return false
	}
	for _, item := range block.Items {
		if AnswerBlockItemVisibleSurface(item) == "" {
			continue
		}
		if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
			continue
		}
		cit := doc.Citations[item.CitationRef]
		if cit.File != "" && cit.Line > 0 {
			return true
		}
	}
	return false
}

// AnswerDocumentPrincipalEvidenceView computes the typed principal-evidence
// projection used by renderers, caveat gates, and audit/status cards. It keeps
// the hard signal precise: a citation counts only when the index is in-range
// and points at a non-empty file with a positive line.
func AnswerDocumentPrincipalEvidenceView(doc *AnswerDocumentV2) AnswerPrincipalEvidenceView {
	if doc == nil {
		return AnswerPrincipalEvidenceView{}
	}
	var out AnswerPrincipalEvidenceView
	for _, block := range doc.Blocks {
		if block.SurfaceRole != SurfacePrincipal {
			continue
		}
		out.PrincipalBlocks++
		out.ClaimUses += len(block.ClaimUses)
		enumBlock := answerPrincipalBlockCarriesEnumeration(block)
		if enumBlock {
			out.PrincipalEnumerationBlocks++
		}
		for _, item := range block.Items {
			out.PrincipalItems++
			enumItem := enumBlock &&
				answerPrincipalBlockKindRendersItems(block.Kind) &&
				AnswerBlockItemVisibleSurface(item) != ""
			if enumItem {
				out.PrincipalEnumerationItems++
			}
			if item.CitationRef < 0 {
				continue
			}
			if item.CitationRef >= len(doc.Citations) {
				out.InvalidCitations++
				if enumItem {
					out.PrincipalEnumerationInvalidCitations++
				}
				continue
			}
			citation := doc.Citations[item.CitationRef]
			if citation.File == "" || citation.Line <= 0 {
				out.InvalidCitations++
				if enumItem {
					out.PrincipalEnumerationInvalidCitations++
				}
				continue
			}
			out.ValidCitations++
			if enumItem {
				out.PrincipalEnumerationValidCitations++
			}
		}
	}
	return out
}

func answerPrincipalBlockCarriesEnumeration(block AnswerBlock) bool {
	for _, facet := range block.FacetIDs {
		if facet == string(FacetEnumerationItem) {
			return true
		}
	}
	for _, claim := range block.ClaimUses {
		if claim.FacetID == string(FacetEnumerationItem) {
			return true
		}
	}
	return false
}

func answerPrincipalBlockKindRendersItems(kind AnswerBlockKind) bool {
	return AnswerBlockKindRendersStructuredItems(kind)
}
