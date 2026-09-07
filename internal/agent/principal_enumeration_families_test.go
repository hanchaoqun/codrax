package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The six declaration coordinates and independent markers are the r1028
// ArkTS witness. Marker order is deliberately not the user's grouping order.
func principalEnumerationArkTSFamilyContext() *types.AgentContext {
	const prefix = "internal/thirdparty/tree-sitter-arkts/corpus/sources/"
	obs := types.SourceInventoryObservation{Active: true, Complete: true, Scopes: []string{prefix}}
	facts := []types.AnswerAggregateFact{
		{Kind: types.AnswerAggregateMemberSet, Label: "@Entry 页面入口", Value: "4", Role: types.AnswerAggregateRolePrincipalAnswer},
		{Kind: types.AnswerAggregateMemberSet, Label: "@Builder 复用片段", Value: "2", Role: types.AnswerAggregateRolePrincipalAnswer},
	}
	for _, d := range []struct {
		name, file string
		line, fact int
	}{
		{"Index", "01_entry_component_minimal.ets", 5, 0},
		{"ParentComponent", "03_state_management.ets", 32, 0},
		{"StyledPage", "04_styles_extend.ets", 17, 0},
		{"ListPage", "05_foreach_lazyforeach.ets", 30, 0},
		{"defaultHeader", "02_builder_decorator.ets", 8, 1},
		{"GlobalCard", "02_builder_decorator.ets", 26, 1},
	} {
		role, terms := types.AnswerCandidateRoleType, []string{"@Component", "@Entry"}
		if d.fact == 1 {
			role, terms = types.AnswerCandidateRoleFunction, []string{"@Builder"}
		}
		file := prefix + d.file
		member := types.SourceInventoryObservationMember{
			Name: d.name, Key: fmt.Sprintf("%s:%d", file, d.line), File: file, Line: d.line,
			Role: role, Language: "arkts", SourceClass: types.SourcePathRoleThirdParty,
			SurfaceTerms: terms, CoverageState: types.SourceInventoryCoverageObserved,
			SupportRef: fmt.Sprintf("%s @ %s:%d", d.name, file, d.line),
		}
		if len(obs.Sets) <= d.fact {
			obs.Sets = append(obs.Sets, types.SourceInventoryObservationSet{Role: role, Complete: true})
		}
		obs.Sets[d.fact].Members = append(obs.Sets[d.fact].Members, member)
		obs.Sets[d.fact].Count++
		obs.Sets[d.fact].Total++
		facts[d.fact].Members = append(facts[d.fact].Members, d.name)
		facts[d.fact].SupportRefs = append(facts[d.fact].SupportRefs, member.SupportRef)
	}
	mut := types.NewMutableState("typed inventory fixture")
	mut.SetSourceInventoryObservation(obs)
	mut.SetInvestigationAggregateFacts(facts)
	mut.SetInvestigationComplete("accepted declaration roster")
	mut.SetInvestigationResultKind("resolved")
	return &types.AgentContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:             types.IntentEnumerate,
		Predicates:         types.SemanticPredicates{IsCategoryEnumeration: true, HasPerMemberTable: true},
		SourceScopeProfile: &types.SourceScopeProfile{RequestedScope: types.SourceScopeAll},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleType, types.AnswerCandidateRoleFunction},
			RequestedFields: []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation}, Confidence: .95,
		},
	}}}
}

func TestPrincipalEnumerationFamiliesArkTSActualFinalizerHandoff(t *testing.T) {
	ctx := principalEnumerationArkTSFamilyContext()
	before := answerDocPrincipalEnumerationSets(ctx, answerSurfacePlan(ctx))
	if len(before) != 2 || len(before[0].Rows) != 4 || len(before[1].Rows) != 2 {
		t.Fatalf("fixture must retain the exact accepted 4+2 roster: %+v", before)
	}
	profileBefore, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	observationBefore := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	out := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"surface_family=`@component`, surface_families=`@component`, `@entry`",
		"typed_surface_family_row_counts=[`@builder`:2, `@component`:4, `@entry`:4]",
		"family_coverage=6/6, complete=true",
		"family memberships may overlap; do not add these counts",
		"not additional declarations",
		"not a repository-wide completeness claim",
		"a common representative key, not the only family a row can carry",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("finalizer lost typed family information %q", want)
		}
	}
	for _, forbidden := range []string{
		"These mutually exclusive counts", "These row counts are the only numeric family summary",
		"Finer row-local modifiers remain item detail", "use that exact row-local key for grouping",
		"selection_family` remains the membership authority",
	} {
		if strings.Contains(out, forbidden) {
			t.Errorf("finalizer still teaches a first-marker-only family authority: %q", forbidden)
		}
	}
	for _, set := range before {
		for _, row := range set.Rows {
			var emitted string
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "- row_id=`"+row.RowID+"`") {
					if emitted != "" {
						t.Fatalf("display duplicated accepted row %q", row.RowID)
					}
					emitted = line
				}
			}
			for _, want := range []string{"member=`" + row.Member + "`", "location=`" + row.Location + "`", "citation_key=`" + row.CitationKey + "`"} {
				if !strings.Contains(emitted, want) {
					t.Errorf("row identity/citation changed: missing %q in %s", want, emitted)
				}
			}
			wantFamilies := "surface_families=" + renderSourceInventorySurfaceFamilies(types.SourceInventorySurfaceFamilyKeys(row.SurfaceTerms))
			if !strings.Contains(emitted, wantFamilies) {
				t.Errorf("principal row lost its own independent markers %q: %s", wantFamilies, emitted)
			}
		}
	}
	profileAfter, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	if string(profileBefore) != string(profileAfter) || !reflect.DeepEqual(observationBefore, types.SourceInventoryObservationFromMutable(ctx.Mutable)) ||
		!reflect.DeepEqual(before, answerDocPrincipalEnumerationSets(ctx, answerSurfacePlan(ctx))) {
		t.Fatal("display must not change roles, membership, row IDs, citation bindings, or source facts")
	}
}

func TestPrincipalEnumerationFamiliesCountsAreMembershipsNotNewDeclarations(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		terms := []string{"@Component", "@Entry", "@Entry", "@ENTRY"}
		if reverse {
			terms = []string{"@Entry", "@ENTRY", "@Entry", "@Component"}
		}
		sets := []types.EnumerationDisplaySet{{Rows: []types.EnumerationDisplayRow{
			{Member: "Page", Location: "a.ets:5", SurfaceTerms: terms},
			{Member: "Page", Location: "b.ets:5", SurfaceTerms: terms},
			{Member: "Cart", Location: "Cart.cj:14", SurfaceTerms: []string{"public class", "public class Cart"}},
			{Member: "Cart", Location: "Cart.cj:30", SurfaceTerms: []string{"extend", "extend Cart"}},
			{Member: "unknown", Location: "unknown:2"},
		}}}
		got, covered, total := answerDocPrincipalEnumerationSurfaceFamilyCounts(sets)
		if want := "`@component`:2, `@entry`:2, `extend`:1, `public class`:1"; got != want || covered != 4 || total != 5 {
			t.Errorf("reverse=%t: got counts=%q coverage=%d/%d; want %q coverage=4/5", reverse, got, covered, total, want)
		}
	}
	for _, sets := range [][]types.EnumerationDisplaySet{nil, {{Rows: []types.EnumerationDisplayRow{{}}}}} {
		got, covered, total := answerDocPrincipalEnumerationSurfaceFamilyCounts(sets)
		if got != "" || covered != 0 || total != len(sets) {
			t.Fatalf("missing markers must not manufacture family membership: %q %d/%d", got, covered, total)
		}
	}
}

func TestPrincipalEnumerationFamiliesDisplayLimitsDoNotLimitCounting(t *testing.T) {
	var terms []string
	for i := 0; i < 80; i++ {
		terms = append(terms, fmt.Sprintf("@marker%02d%s", i, strings.Repeat("x", 2000)))
	}
	sets := []types.EnumerationDisplaySet{{Rows: []types.EnumerationDisplayRow{{SurfaceTerms: terms}}}}
	got, covered, total := answerDocPrincipalEnumerationSurfaceFamilyCounts(sets)
	if covered != 1 || total != 1 || len(got) > 8000 || !strings.Contains(got, "+48 family counts omitted") || !strings.Contains(got, "characters omitted") {
		t.Fatalf("family display must be bounded with honest omission, not a second unbounded copy: bytes=%d covered=%d total=%d text=%s", len(got), covered, total, got)
	}
	if len(sets[0].Rows[0].SurfaceTerms) != 80 || sets[0].Rows[0].SurfaceTerms[79] != terms[79] {
		t.Fatal("display bounds must not truncate the typed source")
	}
	ctx := principalEnumerationArkTSFamilyContext()
	obs := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	obs.Sets[0].Members[0].SurfaceTerms = append(obs.Sets[0].Members[0].SurfaceTerms, terms...)
	ctx.Mutable.SetSourceInventoryObservation(obs)
	before := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	out := renderAnswerDocPrincipalEnumerationRows(ctx, answerSurfacePlan(ctx))
	var row string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "- row_id=") && strings.Contains(line, "member=`Index`") {
			row = line
		}
	}
	if len(row) > 2200 || !strings.Contains(row, "+74 families omitted") || !strings.Contains(row, "characters omitted") ||
		!strings.Contains(row, "surface_families=`@component`, `@entry`") || !strings.Contains(row, "citation_key=") {
		t.Fatalf("principal row must preserve common markers and citation while bounding only display: %s", row)
	}
	if !reflect.DeepEqual(before, types.SourceInventoryObservationFromMutable(ctx.Mutable)) {
		t.Fatal("principal display must not truncate source marker facts")
	}
}

func TestPrincipalEnumerationFamiliesDoesNotInferMissingAnalyzerRoles(t *testing.T) {
	ctx := principalEnumerationArkTSFamilyContext()
	ctx.AnalysisIR.RequestModel.SourceInventoryProfile.TargetRoles = []types.AnswerCandidateRole{
		types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleMethod,
	}
	before := answerDocPrincipalEnumerationSets(ctx, answerSurfacePlan(ctx))
	profileBefore, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	(&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	profileAfter, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	if string(profileBefore) != string(profileAfter) || !reflect.DeepEqual(before, answerDocPrincipalEnumerationSets(ctx, answerSurfacePlan(ctx))) {
		t.Fatal("displaying complete markers must not infer a missing type role or change the admitted row registry")
	}
}
