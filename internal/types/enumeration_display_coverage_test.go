package types

import "testing"

func TestAnswerDocumentCoversEnumerationDisplaySets_RelationRowPrincipalList(t *testing.T) {
	doc := &AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []AnswerBlock{{
			ID:          "impls",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "ExponentialBackoffRetryPolicy",
				Text:        "implements RetryPolicy",
				CitationRef: 0,
			}},
		}},
		Citations: []Citation{{File: "src/retry.ts", Line: 42}},
	}
	sets := []EnumerationDisplaySet{{
		ID: "retry_policy_impls",
		Rows: []EnumerationDisplayRow{{
			Member:       "RetryPolicy → ExponentialBackoffRetryPolicy",
			DisplayLabel: "ExponentialBackoffRetryPolicy",
			Source:       "src/retry.ts",
			LineStart:    42,
			HasCitation:  true,
			ClaimForm:    ClaimDefinitionFact,
		}},
	}}

	if !AnswerDocumentCoversEnumerationDisplaySets(doc, sets) {
		t.Fatalf("expected principal relation row to be visible and cited")
	}
}

func TestAnswerDocumentCoversEnumerationDisplaySets_RequiresCompatibleCitation(t *testing.T) {
	doc := &AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []AnswerBlock{{
			ID:          "impls",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			FacetIDs:    []string{string(FacetEnumerationItem)},
			Items: []AnswerBlockItem{{
				Label:       "ExponentialBackoffRetryPolicy",
				Text:        "implements RetryPolicy",
				CitationRef: 0,
			}},
		}},
		Citations: []Citation{{File: "src/other.ts", Line: 42}},
	}
	sets := []EnumerationDisplaySet{{
		ID: "retry_policy_impls",
		Rows: []EnumerationDisplayRow{{
			Member:       "RetryPolicy → ExponentialBackoffRetryPolicy",
			DisplayLabel: "ExponentialBackoffRetryPolicy",
			Source:       "src/retry.ts",
			LineStart:    42,
			HasCitation:  true,
			ClaimForm:    ClaimDefinitionFact,
		}},
	}}

	if AnswerDocumentCoversEnumerationDisplaySets(doc, sets) {
		t.Fatalf("wrong-file citation must not satisfy typed enumeration row coverage")
	}
}
