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
