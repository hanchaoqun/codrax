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

func TestAnswerDocumentAcceptedEnumerationDisplayCoverage_ProjectsSharedView(t *testing.T) {
	mut := NewMutableState("list retry policy implementations")
	facts := []AnswerAggregateFact{{
		Kind:        AnswerAggregateMemberSet,
		Label:       "RetryPolicy implementations",
		Value:       "1",
		Role:        AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"RetryPolicy -> ExponentialBackoffRetryPolicy"},
		SupportRefs: []string{"RetryPolicy -> ExponentialBackoffRetryPolicy @ src/retry.ts:42"},
	}}
	mut.SetInvestigationAggregateFacts(facts)
	mut.RetainInvestigationAggregateFacts()
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
	rm := RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
	}
	ctx := &BusContext{Mutable: mut, AnalysisIR: &AnalysisIR{RequestModel: rm}}

	view := AnswerDocumentAcceptedEnumerationDisplayCoverage(ctx, nil, doc)
	if len(view.Sets) != 1 || view.Coverage.RowCount != 1 || !view.Complete() || !view.RowsFullyCited() {
		t.Fatalf("shared coverage view should expose one complete fully-cited row-set, got %+v", view)
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
