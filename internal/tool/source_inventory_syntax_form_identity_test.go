package tool

import (
	"reflect"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSourceInventoryConstructSurfaceTerms_DistinguishesKeywordAndMarkerSyntaxForms(t *testing.T) {
	keyword := &repotypes.Symbol{Name: "Cart", Kind: "extend"}
	if got := sourceInventoryConstructSurfaceTerms(keyword); !reflect.DeepEqual(got, []string{"extend", "extend Cart"}) {
		t.Fatalf("keyword terms = %#v", got)
	}
	if family := types.SourceInventorySurfaceFamilyKey(sourceInventoryConstructSurfaceTerms(keyword)); family != "extend" {
		t.Fatalf("keyword family = %q, want extend", family)
	}

	marker := &repotypes.Symbol{Name: "highlight", Kind: "extend", Doc: "@Extend(Text)"}
	if got := sourceInventoryConstructSurfaceTerms(marker); !reflect.DeepEqual(got, []string{"@Extend", "@Extend highlight"}) {
		t.Fatalf("marker terms = %#v", got)
	}
	terms := append(sourceInventorySurfaceTermsFromGraphNote(marker.Doc), sourceInventoryConstructSurfaceTerms(marker)...)
	if family := types.SourceInventorySurfaceFamilyKey(terms); family != "@extend" {
		t.Fatalf("marker family = %q, want @extend (terms=%#v)", family, terms)
	}
}

func TestSourceInventoryRequestedSurfaceFamilies_UsesParserSyntaxFormIdentity(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "fixtures/cart.cj",
			Language: "cangjie",
			Symbols: []repotypes.Symbol{{
				Name: "Cart", Kind: "extend", File: "fixtures/cart.cj", Line: 30,
			}},
		},
		{
			RelPath:  "fixtures/styles.ets",
			Language: "arkts",
			Symbols: []repotypes.Symbol{{
				Name: "highlight", Kind: "extend", File: "fixtures/styles.ets", Line: 11, Doc: "@Extend(Text)",
			}},
		},
	})
	index := newSourceInventoryGraphSymbolIndex(graph)
	profile := &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
		SourceQuotes:      []string{"extend 块"},
	}
	requested := sourceInventoryRequestedSurfaceFamiliesByRole(nil, index, []string{"."}, profile)
	if got := requested[types.AnswerCandidateRoleType]; !reflect.DeepEqual(got, map[string]bool{"extend": true}) {
		t.Fatalf("keyword request families = %#v, want only bare extend", got)
	}
	sets := sourceInventoryCandidateSets(
		nil,
		graph,
		nil,
		[]string{"."},
		profile,
		nil,
		false,
		false,
		"",
		sourceInventoryInactiveExecBudget(),
	)
	if got := candidateMemberNames(sets[types.AnswerCandidateRoleType].candidates); !reflect.DeepEqual(got, []string{"Cart"}) {
		t.Fatalf("keyword request candidates = %#v, want only Cart", got)
	}

	profile.SourceQuotes = []string{"@Extend"}
	requested = sourceInventoryRequestedSurfaceFamiliesByRole(nil, index, []string{"."}, profile)
	if got := requested[types.AnswerCandidateRoleType]; !reflect.DeepEqual(got, map[string]bool{"@extend": true}) {
		t.Fatalf("marker request families = %#v, want only @extend", got)
	}
	sets = sourceInventoryCandidateSets(
		nil,
		graph,
		nil,
		[]string{"."},
		profile,
		nil,
		false,
		false,
		"",
		sourceInventoryInactiveExecBudget(),
	)
	if got := candidateMemberNames(sets[types.AnswerCandidateRoleType].candidates); !reflect.DeepEqual(got, []string{"highlight"}) {
		t.Fatalf("marker request candidates = %#v, want only highlight", got)
	}
}

func TestSourceInventoryRequestedSurfaceFamilies_UsesIndependentParserMarkersAcrossRoles(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "fixtures/pages.ets",
		Language: "arkts",
		Symbols: []repotypes.Symbol{
			{Name: "Page", Kind: "component", File: "fixtures/pages.ets", Line: 4, Exported: true, Doc: "@Component @Page"},
			{Name: "Ability", Kind: "class", File: "fixtures/pages.ets", Line: 20, Exported: true},
		},
	}, {
		RelPath:  "fixtures/fragments.ets",
		Language: "arkts",
		Symbols: []repotypes.Symbol{
			{Name: "Reusable", Kind: "builder", File: "fixtures/fragments.ets", Line: 8, Exported: true, Doc: "@Reusable"},
			{Name: "Plain", Kind: "function", File: "fixtures/fragments.ets", Line: 16, Exported: true},
		},
	}})
	profile := &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles: []types.AnswerCandidateRole{
			types.AnswerCandidateRoleFunction,
			types.AnswerCandidateRoleType,
		},
		SourceQuotes: []string{"@Page entries", "@Reusable fragments"},
	}
	requested := sourceInventoryRequestedSurfaceFamiliesByRole(nil, newSourceInventoryGraphSymbolIndex(graph), []string{"."}, profile)
	if got := requested[types.AnswerCandidateRoleType]; !reflect.DeepEqual(got, map[string]bool{"@page": true}) {
		t.Fatalf("type marker families = %#v, want only @page", got)
	}
	if got := requested[types.AnswerCandidateRoleFunction]; !reflect.DeepEqual(got, map[string]bool{"@reusable": true}) {
		t.Fatalf("function marker families = %#v, want only @reusable", got)
	}
	sets := sourceInventoryCandidateSets(
		nil,
		graph,
		nil,
		[]string{"."},
		profile,
		nil,
		false,
		false,
		"",
		sourceInventoryInactiveExecBudget(),
	)
	if got := candidateMemberNames(sets[types.AnswerCandidateRoleType].candidates); !reflect.DeepEqual(got, []string{"Page"}) {
		t.Fatalf("exact type marker request leaked undecorated type: %#v", got)
	}
	if got := candidateMemberNames(sets[types.AnswerCandidateRoleFunction].candidates); !reflect.DeepEqual(got, []string{"Reusable"}) {
		t.Fatalf("exact function marker request leaked plain function: %#v", got)
	}
}

func candidateMemberNames(candidates []sourceInventoryCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.member)
	}
	return out
}
