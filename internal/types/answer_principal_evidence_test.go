package types

import "testing"

func TestAnswerDocumentPrincipalEvidenceViewCountsOnlyPrincipalValidCitations(t *testing.T) {
	doc := &AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []AnswerBlock{{
			ID:          "principal",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			Items: []AnswerBlockItem{{
				ID:          "valid",
				CitationRef: 0,
			}, {
				ID:          "invalid",
				CitationRef: 4,
			}},
			ClaimUses: []RenderedClaimUse{{ClaimForm: ClaimDefinitionFact}},
		}, {
			ID:   "support",
			Kind: BlockSummary,
			Items: []AnswerBlockItem{{
				ID:          "ignored",
				CitationRef: 0,
			}},
		}},
		Citations: []Citation{{File: "pkg/owner.py", Line: 12}},
	}
	got := AnswerDocumentPrincipalEvidenceView(doc)
	if got.PrincipalBlocks != 1 || got.PrincipalItems != 2 || got.ValidCitations != 1 ||
		got.InvalidCitations != 1 || got.ClaimUses != 1 {
		t.Fatalf("unexpected principal evidence view: %+v", got)
	}
	if !got.HasGroundedPrincipalEvidence() {
		t.Fatalf("valid principal citation should satisfy grounded evidence: %+v", got)
	}
}

func TestAnswerDocumentPrincipalEvidenceViewRejectsSupportingOnlyCitations(t *testing.T) {
	doc := &AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []AnswerBlock{{
			ID:   "support",
			Kind: BlockSummary,
			Items: []AnswerBlockItem{{
				ID:          "citation",
				CitationRef: 0,
			}},
		}},
		Citations: []Citation{{File: "pkg/owner.py", Line: 12}},
	}
	got := AnswerDocumentPrincipalEvidenceView(doc)
	if got.HasPrincipalSurface() || got.HasGroundedPrincipalEvidence() {
		t.Fatalf("supporting-only citations must not satisfy principal evidence: %+v", got)
	}
}

func TestAnswerDocumentPrincipalEvidenceViewCountsSectionEnumerationItems(t *testing.T) {
	doc := &AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []AnswerBlock{{
			ID:          "entries",
			Kind:        BlockSection,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				ID:          "index",
				Label:       "Index",
				Text:        "ArkTS page entry",
				CitationRef: 0,
			}, {
				ID:          "list",
				Label:       "ListPage",
				Text:        "ArkTS page entry",
				CitationRef: 1,
			}},
		}},
		Citations: []Citation{
			{File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", Line: 7},
			{File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/05_foreach_lazyforeach.ets", Line: 32},
		},
	}
	got := AnswerDocumentPrincipalEvidenceView(doc)
	if !got.HasGroundedPrincipalEvidence() {
		t.Fatalf("section item citations should satisfy principal evidence: %+v", got)
	}
	if !got.HasGroundedPrincipalEnumerationEvidence() {
		t.Fatalf("section enumeration items should satisfy grounded enumeration evidence: %+v", got)
	}
	if got.PrincipalEnumerationBlocks != 1 || got.PrincipalEnumerationItems != 2 ||
		got.PrincipalEnumerationValidCitations != 2 || got.PrincipalEnumerationInvalidCitations != 0 {
		t.Fatalf("unexpected enumeration evidence view: %+v", got)
	}
}

func TestAnswerDocumentPrincipalEvidenceViewRequiresAllEnumerationItemCitations(t *testing.T) {
	doc := &AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []AnswerBlock{{
			ID:          "entries",
			Kind:        BlockSection,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				ID:          "index",
				Label:       "Index",
				CitationRef: 0,
			}, {
				ID:          "list",
				Label:       "ListPage",
				CitationRef: -1,
			}},
		}},
		Citations: []Citation{{File: "entry.ets", Line: 7}},
	}
	got := AnswerDocumentPrincipalEvidenceView(doc)
	if got.HasGroundedPrincipalEnumerationEvidence() {
		t.Fatalf("partial enumeration citations must not satisfy grounded enumeration evidence: %+v", got)
	}
}
