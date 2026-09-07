package context

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1586aSourceInventoryDossierCarriesTypedConstructFamilies(t *testing.T) {
	for _, tc := range []struct {
		language string
		terms    [][]string
		families []string
	}{
		{"cangjie", [][]string{{"public class", "public class Cart"}, {"extend", "extend Cart"}}, []string{"public class", "extend"}},
		{"arkts", [][]string{{"@Entry", "@Component", "struct", "struct Cart"}, {"@Reusable(Card)", "@Component"}}, []string{"@entry, @component, struct", "@reusable, @component"}},
		{"java", [][]string{{"public class", "public class Cart"}, {"public interface", "public interface Cart"}}, []string{"public class", "public interface"}},
		{"rust", [][]string{{"struct", "struct Cart"}, {"trait", "trait Cart"}}, []string{"struct", "trait"}},
	} {
		t.Run(tc.language, func(t *testing.T) {
			members := make([]types.SourceInventoryObservationMember, len(tc.terms))
			for i, terms := range tc.terms {
				members[i] = types.SourceInventoryObservationMember{
					Name: "Cart", File: "src/declarations", Line: 10 + i, Language: tc.language,
					Role: types.AnswerCandidateRoleType, SurfaceTerms: terms,
					CoverageState: types.SourceInventoryCoverageObserved,
				}
			}
			mut := types.NewMutableState("inspect declarations")
			mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
				Active: true, AdvisoryOnly: true, Scopes: []string{"src"}, QueryPathScopes: []string{"module/src"},
				Sets: []types.SourceInventoryObservationSet{{Role: types.AnswerCandidateRoleType, Count: 2, Total: 2, Complete: true, Members: members}},
			})
			before, _ := json.Marshal(mut.SourceInventoryObservation())
			pc := BuildPromptContext(&types.AgentContext{AgentName: types.AgentExplorer, Stage: types.StageExplore, Mutable: mut}, &skill.Config{Name: "explore-skill"})
			section := findSectionTitle(pc, SectionRelationDossier)
			if section == nil {
				t.Fatal("real prompt omitted source-inventory dossier")
			}
			for i, family := range tc.families {
				want := fmt.Sprintf("Cart (src/declarations:%d) lang=%s families=[%s]", 10+i, tc.language, family)
				if !strings.Contains(section.Content, want) {
					t.Errorf("same-name constructs must retain their own typed families, missing %q:\n%s", want, section.Content)
				}
			}
			for _, want := range []string{"Advisory only", "not treat candidate rows as final answer members", "scopes=src", "query_path_scopes=module/src", "shown=2 omitted=0", "Omissions are not exclusions"} {
				if !strings.Contains(section.Content, want) {
					t.Errorf("dossier must preserve display/scope/authority boundary %q:\n%s", want, section.Content)
				}
			}
			after, _ := json.Marshal(mut.SourceInventoryObservation())
			if string(before) != string(after) {
				t.Fatal("rendering changed source-inventory observation")
			}
		})
	}
}

func TestB1586aSourceInventoryDossierBoundsDoNotClaimCompleteDisplay(t *testing.T) {
	members := make([]types.SourceInventoryObservationMember, 8)
	for i := range members {
		members[i] = types.SourceInventoryObservationMember{Name: fmt.Sprintf("Member%d", i), SurfaceTerms: []string{"class", fmt.Sprintf("class Member%d", i)}}
	}
	observation := types.SourceInventoryObservation{
		Active: true, Complete: false,
		Scopes: []string{"scope0", "scope1", "scope2", "scope3", "scope4", "scope5"},
		Sets:   []types.SourceInventoryObservationSet{{Role: types.AnswerCandidateRoleType, Complete: false, Count: 8, Total: 12, Members: members}},
	}
	out := formatRelationDossierSourceInventory(observation)
	for _, want := range []string{"complete=false count=8", "total_or_lower_bound=12 shown=5 omitted=3", "scopes=scope0, scope1, scope2, scope3, +2 not shown", "+3 more"} {
		if !strings.Contains(out, want) {
			t.Errorf("bounded display lost inventory/display distinction %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Member5") || strings.Contains(out, "scope4") {
		t.Fatalf("existing member budget or bounded scope display exceeded:\n%s", out)
	}
	observation.Sets[0].Complete = true
	observation.Sets[0].Total = 8
	out = formatRelationDossierSourceInventory(observation)
	if !strings.Contains(out, "complete=true count=8") || !strings.Contains(out, "shown=5 omitted=3") {
		t.Fatalf("complete inventory must remain distinct from truncated display:\n%s", out)
	}
}

func TestB1586aSourceInventoryDossierFamiliesAreBoundedAndTypedOnly(t *testing.T) {
	member := types.SourceInventoryObservationMember{
		Name: "Cart", SurfaceTerms: []string{"@Entry", "@Component", "@Reusable", "@Preview", "@Observed", "@Extra"},
		Attributes: []types.SourceInventoryObservationAttribute{{Name: "build", Role: types.AnswerCandidateRoleFunction, SurfaceTerms: []string{"@Builder", "@Reusable(Fragment)"}}},
	}
	out := formatRelationDossierSourceInventory(types.SourceInventoryObservation{Active: true, Sets: []types.SourceInventoryObservationSet{{Members: []types.SourceInventoryObservationMember{member}}}})
	for _, want := range []string{"families=[@entry, @component, @reusable, @preview, +2 not shown]", "attrs=[build:function families=[@builder, @reusable]]", "count=1", "total_or_lower_bound=unknown"} {
		if !strings.Contains(out, want) {
			t.Errorf("typed family display missing %q:\n%s", want, out)
		}
	}
	member.Name = "public class Fake @Entry extend Cart"
	member.SurfaceTerms = nil
	member.Attributes = nil
	out = formatRelationDossierSourceInventory(types.SourceInventoryObservation{Active: true, Sets: []types.SourceInventoryObservationSet{{Members: []types.SourceInventoryObservationMember{member}}}})
	if strings.Contains(out, "families=") {
		t.Fatalf("names must not mint construct families:\n%s", out)
	}
	member.SurfaceTerms = []string{"@" + strings.Repeat("x", 1000), "@Entry"}
	out = formatRelationDossierSourceInventory(types.SourceInventoryObservation{Active: true, Sets: []types.SourceInventoryObservationSet{{Members: []types.SourceInventoryObservationMember{member}}}})
	if strings.Contains(out, strings.Repeat("x", 181)) || !strings.Contains(out, "+2 not shown") {
		t.Fatalf("overlong typed family should be omitted explicitly, not published as an altered family:\n%s", out)
	}
}

func TestB1586aSourceInventoryDossierSetOmissionsCountActualDisplay(t *testing.T) {
	sets := make([]types.SourceInventoryObservationSet, 5)
	for i := range sets {
		sets[i] = types.SourceInventoryObservationSet{Role: types.AnswerCandidateRoleType, Complete: true, Count: 1, Total: 1, Members: []types.SourceInventoryObservationMember{{Name: fmt.Sprintf("Set%d", i)}}}
	}
	sets[0] = types.SourceInventoryObservationSet{Role: types.AnswerCandidateRoleFunction, Complete: true}
	out := formatRelationDossierSourceInventory(types.SourceInventoryObservation{Active: true, Sets: sets})
	if !strings.Contains(out, "role=function complete=true count=0") ||
		!strings.Contains(out, "1 additional source-inventory set(s) omitted") ||
		strings.Contains(out, "Set4") {
		t.Fatalf("empty inventories and set display cap must be counted honestly:\n%s", out)
	}
}
