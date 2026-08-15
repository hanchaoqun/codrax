package types

import (
	"slices"
	"testing"
)

func TestAnswerBlockItemCitationRefsPrimaryFirstDeduplicated(t *testing.T) {
	item := AnswerBlockItem{CitationRef: 2, CitationRefs: []int{2, -1, 4, 4}}
	if got := AnswerBlockItemCitationRefs(item); !slices.Equal(got, []int{2, 4}) {
		t.Fatalf("refs=%v", got)
	}
	SetAnswerBlockItemCitationRefs(&item, []int{-1, 3, 3, 5})
	if item.CitationRef != 3 || !slices.Equal(item.CitationRefs, []int{5}) {
		t.Fatalf("canonical item=%+v", item)
	}
}

func TestMutableAnswerDocumentCloneDeepCopiesAdditionalCitationRefs(t *testing.T) {
	state := NewMutableState("multi citation clone")
	state.SetAnswerDocumentV2WithMutation(MutationReplaceAll, &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "b", Kind: BlockOrderedList,
		Items: []AnswerBlockItem{{ID: "i", CitationRef: 0, CitationRefs: []int{1, 2}}},
	}}})
	first := state.AnswerDocumentV2()
	first.Blocks[0].Items[0].CitationRefs[0] = 99
	second := state.AnswerDocumentV2()
	if !slices.Equal(second.Blocks[0].Items[0].CitationRefs, []int{1, 2}) {
		t.Fatalf("additional refs aliased across clone: %+v", second.Blocks[0].Items[0])
	}
}

func TestAnswerSupportIndexProjectsOneItemAcrossAllCitedLocations(t *testing.T) {
	doc := &AnswerDocumentV2{
		Citations: []Citation{
			{File: "registry.go", Line: 10},
			{File: "plugin.go", Line: 20},
		},
		Blocks: []AnswerBlock{{
			ID:          "plugins",
			Kind:        BlockOrderedList,
			SurfaceRole: SurfacePrincipal,
			Items: []AnswerBlockItem{{
				ID: "json", Label: "JsonPlugin", CitationRef: 0, CitationRefs: []int{1},
			}},
		}},
	}
	index := newAnswerSupportDocumentIndex(doc)
	for _, location := range []string{"registry.go:10", "plugin.go:20"} {
		if got := len(index.principalByLocation[location]); got != 1 {
			t.Fatalf("principal location %q indexed %d times, want 1", location, got)
		}
	}
}
