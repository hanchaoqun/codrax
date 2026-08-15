package tool

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func relationProjectionDriftRequestModel() types.RequestModel {
	return types.RequestModel{
		// Deliberately unrelated prose: activation and scope must depend only
		// on the structured request and exact candidate/member carriers.
		RawRequest: "render every unrelated repository type as principal",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsRelationalLookup:    false,
		},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"LoopController"},
			Entities:        []string{"LoopController", "prodA", "prodB"},
		},
		CompletenessObligation: &types.CompletenessObligation{
			Required:    true,
			SourceQuote: "typed analyzer member boundary",
		},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
			Confidence:        0.95,
		},
	}
}

func relationProjectionDriftCandidates() typedRelationCandidateSourceFixture {
	return typedRelationCandidateSourceFixture{
		{
			Relation: types.TypedRelationImplements, SourceName: "LoopController",
			Member:    types.TypedRelationMember{Name: "prodA", File: "internal/agent/a.go", Line: 10},
			Precision: types.TypedRelationPrecisionExactSymbolID,
		},
		{
			Relation: types.TypedRelationImplements, SourceName: "LoopController",
			Member:    types.TypedRelationMember{Name: "prodB", File: "internal/agent/b.go", Line: 20},
			Precision: types.TypedRelationPrecisionExactSymbolID,
		},
		{
			Relation: types.TypedRelationImplements, SourceName: "LoopController",
			Member:    types.TypedRelationMember{Name: "testStub", File: "internal/agent/agent_test.go", Line: 30},
			Precision: types.TypedRelationPrecisionExactSymbolID,
		},
	}
}

func relationProjectionDriftFact(members ...string) types.AnswerAggregateFact {
	refs := map[string]string{
		"LoopController": "LoopController @ internal/agent/agent.go:5",
		"prodA":          "prodA @ internal/agent/a.go:10",
		"prodB":          "prodB @ internal/agent/b.go:20",
		"testStub":       "testStub @ internal/agent/agent_test.go:30",
	}
	support := make([]string, 0, len(members))
	for _, member := range members {
		support = append(support, refs[member])
	}
	return types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "interface and implementation types",
		Value:       strconv.Itoa(len(members)),
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     members,
		SupportRefs: support,
	}
}

func relationProjectionDriftBus() *types.BusContext {
	mut := types.NewMutableState("typed relation projection drift")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"internal/agent"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleType,
			Count:    4,
			Complete: true,
			Members: []types.SourceInventoryObservationMember{
				{Name: "LoopController", Role: types.AnswerCandidateRoleType, File: "internal/agent/agent.go", Line: 5},
				{Name: "prodA", Role: types.AnswerCandidateRoleType, File: "internal/agent/a.go", Line: 10},
				{Name: "prodB", Role: types.AnswerCandidateRoleType, File: "internal/agent/b.go", Line: 20},
				{Name: "testStub", Role: types.AnswerCandidateRoleType, File: "internal/agent/agent_test.go", Line: 30},
			},
		}},
	})
	return &types.BusContext{
		Mutable:    mut,
		MultiGraph: relationProjectionDriftCandidates(),
		AnalysisIR: &types.AnalysisIR{RequestModel: relationProjectionDriftRequestModel()},
	}
}

func TestExactTypedRelationProjectionActiveConvergesAnalyzerDriftWithoutProse(t *testing.T) {
	bus := relationProjectionDriftBus()
	facts := []types.AnswerAggregateFact{
		relationProjectionDriftFact("LoopController", "prodA", "prodB"),
	}
	if !exactTypedRelationProjectionActive(bus, facts) {
		t.Fatalf("exact relation roster should own the typed architecture member set")
	}

	rm := bus.AnalysisIR.RequestModel
	rm.DiagramHint = nil
	bus.AnalysisIR.RequestModel = rm
	if exactTypedRelationProjectionActive(bus, facts) {
		t.Fatalf("source inventory without a typed relation/diagram shape must remain inventory-owned")
	}

	rm = relationProjectionDriftRequestModel()
	bus.AnalysisIR.RequestModel = rm
	unrelated := []types.AnswerAggregateFact{
		relationProjectionDriftFact("LoopController", "prodA", "UnrelatedType"),
	}
	unrelated[0].Provenance = types.TypedRelationPrincipalMemberSetAggregateProvenance
	if exactTypedRelationProjectionActive(bus, unrelated) {
		t.Fatalf("an unrelated generic inventory member must prevent relation authority")
	}
	sanitized := markExactTypedRelationPrincipalMemberSets(bus, unrelated)
	if len(sanitized) != 1 || types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(sanitized[0]) {
		t.Fatalf("model-supplied system provenance must be stripped: %+v", sanitized)
	}
}

func TestExactTypedRelationProjectionMarksFactAndPreventsInventoryReplacement(t *testing.T) {
	bus := relationProjectionDriftBus()
	facts := []types.AnswerAggregateFact{
		relationProjectionDriftFact("LoopController", "prodA", "prodB"),
	}
	marked := markExactTypedRelationPrincipalMemberSets(bus, facts)
	if len(marked) != 1 ||
		!types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(marked[0]) {
		t.Fatalf("missing exact relation authority marker: %+v", marked)
	}
	landed := sourceInventoryPrincipalRowSetLandingFacts(bus, marked)
	if len(landed) != 1 || strings.Join(landed[0].Members, ",") != "LoopController,prodA,prodB" {
		t.Fatalf("source inventory must not replace the accepted relation slate: %+v", landed)
	}
	if landed[0].Provenance == types.SourceInventoryPrincipalRowSetAggregateProvenance {
		t.Fatalf("relation slate was replaced by source inventory: %+v", landed[0])
	}
}

func TestExactTypedRelationProjectionMarksBridgeLiteralRegistryIdentitySet(t *testing.T) {
	terminal := types.EvidenceItem{
		ID: "terminal-explorer", Kind: types.EvidenceConcrete,
		Subject: "SubExplorer.Name", Predicate: "returns", Object: `"explorer"`,
		Source: "internal/agent/sub_explorer.go", LineStart: 33, LineEnd: 33,
		Producer: "bridge_literal_terminal", Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}
	bridge := types.EvidenceItem{
		ID: "bridge-default-subagent", Kind: types.EvidenceDataflowPath,
		Predicate: "resolution_chain", Object: `"explorer"`,
		Source: "internal/agent/subagent.go", LineStart: 64, LineEnd: 64,
		DerivedFrom: []string{terminal.ID}, Producer: "bridge_literal", Scope: types.ScopeLine,
		AnchorSymbol: "SubExplorer.Name", OwnerSymbol: "RegisterDefaultSubAgents",
		GroundingStatus: types.GroundingGrounded,
	}
	mut := types.NewMutableState("default registered subagent names")
	bus := &types.BusContext{
		Mutable:       mut,
		EvidenceItems: []types.EvidenceItem{bridge, terminal},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentEnumerate,
			PredicateAxis: types.AxisRegister,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
				IsRelationalLookup:    true,
			},
			AnalyzerHints: types.AnalyzerHints{
				Kind:            string(types.ReqRegistration),
				PrimaryEntities: []string{"RegisterDefaultSubAgents"},
			},
			CompletenessObligation: &types.CompletenessObligation{Required: true},
		}},
	}
	fact := types.AnswerAggregateFact{
		Kind: types.AnswerAggregateMemberSet, Label: "default registered subagent names", Value: "1",
		Members: []string{"explorer"}, SupportRefs: []string{"explorer: internal/agent/sub_explorer.go:33"},
	}
	marked := markExactTypedRelationPrincipalMemberSets(bus, []types.AnswerAggregateFact{fact})
	if len(marked) != 1 || !types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(marked[0]) {
		t.Fatalf("exact bridge-literal registry identity chain must authorize the matching model member set: %+v", marked)
	}

	bus.EvidenceItems = []types.EvidenceItem{bridge}
	unmarked := markExactTypedRelationPrincipalMemberSets(bus, []types.AnswerAggregateFact{fact})
	if len(unmarked) != 1 || types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(unmarked[0]) {
		t.Fatalf("a bridge without its exact terminal companion must remain advisory: %+v", unmarked)
	}
}

func TestExactTypedRelationProjectionCanonicalizesRepeatedObservationsByCandidateIdentity(t *testing.T) {
	bus := relationProjectionDriftBus()
	fact := relationProjectionDriftFact("LoopController", "prodA", "prodB")
	fact.Members = []string{
		"LoopController",
		"prodA @ internal/agent/a.go:10",
		"prodA @ internal/agent/a.go:11",
		"prodB",
	}
	fact.SupportRefs = []string{
		"LoopController @ internal/agent/agent.go:5",
		"prodA @ internal/agent/a.go:10",
		"prodA @ internal/agent/a.go:11",
		"prodB @ internal/agent/b.go:20",
	}
	fact.MemberNotes = []string{"interface", "first observation", "second observation", "second implementation"}
	fact.Value = "4"

	marked := markExactTypedRelationPrincipalMemberSets(bus, []types.AnswerAggregateFact{fact})
	if len(marked) != 1 || !types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(marked[0]) {
		t.Fatalf("duplicate observations should retain typed relation authority: %+v", marked)
	}
	got := marked[0]
	if got.Value != "3" || len(got.Members) != 3 ||
		strings.Join(got.Members, ",") != "LoopController,prodA @ internal/agent/a.go:10,prodB" {
		t.Fatalf("relation members must follow source/member candidate identity, got %+v", got)
	}
	if len(got.SupportRefs) != 3 || strings.Contains(strings.Join(got.SupportRefs, ","), "a.go:11") {
		t.Fatalf("parallel support refs must remain aligned to canonical members: %+v", got.SupportRefs)
	}
	if len(got.MemberNotes) != 3 || got.MemberNotes[1] != "first observation" {
		t.Fatalf("parallel member notes must remain aligned to canonical members: %+v", got.MemberNotes)
	}
}

func TestExactTypedRelationProjectionCanonicalizesCallChainRequirementPredicateDrift(t *testing.T) {
	mut := types.NewMutableState("typed call-chain predicate drift")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            string(types.ReqCallChain),
				PrimaryEntities: []string{"appendTypedRelationKinds"},
			},
			// Deliberately preserve the real drift witness: the analyzer selected
			// call_chain but left predicate_axis/relational_lookup unset.
			PredicateAxis: types.AxisUnknown,
			Predicates:    types.SemanticPredicates{},
		}},
		MultiGraph: typedRelationCandidateSourceFixture{
			{
				Relation: types.TypedRelationCalledBy, SourceName: "appendTypedRelationKinds",
				Member:    types.TypedRelationMember{Name: "BuildTypedRelationQueryWithResolvedSources", File: "internal/types/typed_relation_hint.go", Line: 294},
				Precision: types.TypedRelationPrecisionExactSymbolID,
			},
			{
				Relation: types.TypedRelationCalledBy, SourceName: "appendTypedRelationKinds",
				Member:    types.TypedRelationMember{Name: "TypedRelationKindsForRequest", File: "internal/types/typed_relation_hint.go", Line: 321},
				Precision: types.TypedRelationPrecisionExactSymbolID,
			},
		},
	}
	fact := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "appendTypedRelationKinds production callers",
		Value: "3",
		Members: []string{
			"BuildTypedRelationQueryWithResolvedSources @ internal/types/typed_relation_hint.go:294",
			"BuildTypedRelationQueryWithResolvedSources @ internal/types/typed_relation_hint.go:295",
			"TypedRelationKindsForRequest @ internal/types/typed_relation_hint.go:321",
		},
		// This short array is one note per distinct function, not one note per
		// observation. It must not be positionally projected after dedupe.
		MemberNotes: []string{"first caller has two sites", "second caller has one site"},
		SupportRefs: []string{
			"internal/types/typed_relation_hint.go:294",
			"internal/types/typed_relation_hint.go:295",
			"internal/types/typed_relation_hint.go:321",
		},
	}

	marked := markExactTypedRelationPrincipalMemberSets(bus, []types.AnswerAggregateFact{fact})
	if len(marked) != 1 || !types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(marked[0]) {
		t.Fatalf("typed call-chain requirement must retain exact relation authority across predicate drift: %+v", marked)
	}
	got := marked[0]
	if got.Value != "2" || len(got.Members) != 2 ||
		!strings.Contains(got.Members[0], "BuildTypedRelationQueryWithResolvedSources") ||
		!strings.Contains(got.Members[1], "TypedRelationKindsForRequest") {
		t.Fatalf("callsite observations must collapse onto two unique caller identities: %+v", got)
	}
	if len(got.SupportRefs) != 2 || got.SupportRefs[0] != "internal/types/typed_relation_hint.go:294" ||
		got.SupportRefs[1] != "internal/types/typed_relation_hint.go:321" {
		t.Fatalf("canonical caller refs must retain the first exact site for each identity: %+v", got.SupportRefs)
	}
	if len(got.MemberNotes) != 0 {
		t.Fatalf("non-positional member notes must not be misattached after identity projection: %+v", got.MemberNotes)
	}
}

func TestExactTypedRelationProjectionDemotesOnlyImplicitCompetingSibling(t *testing.T) {
	bus := relationProjectionDriftBus()
	exact := relationProjectionDriftFact("LoopController", "prodA", "prodB")
	implicitSibling := types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "upstream context",
		Value:       "1",
		Members:     []string{"Bootstrap"},
		SupportRefs: []string{"internal/bootstrap.go:10"},
	}

	got := markExactTypedRelationPrincipalMemberSets(bus, []types.AnswerAggregateFact{exact, implicitSibling})
	if len(got) != 2 || !types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(got[0]) {
		t.Fatalf("exact relation principal authority missing: %+v", got)
	}
	if got[1].Role != types.AnswerAggregateRoleSupportingCoverage ||
		!strings.Contains(got[1].Provenance, types.TypedRelationImplicitSiblingAggregateDemotionProvenance) {
		t.Fatalf("implicit competing member set must yield its default principal seat: %+v", got[1])
	}
}

func TestExactTypedRelationProjectionPreservesExplicitSecondPrincipalAxis(t *testing.T) {
	bus := relationProjectionDriftBus()
	exact := relationProjectionDriftFact("LoopController", "prodA", "prodB")
	explicitSibling := types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "explicit second requested axis",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"Bootstrap"},
		SupportRefs: []string{"internal/bootstrap.go:10"},
	}

	got := markExactTypedRelationPrincipalMemberSets(bus, []types.AnswerAggregateFact{exact, explicitSibling})
	if len(got) != 2 || got[1].Role != types.AnswerAggregateRolePrincipalAnswer ||
		strings.Contains(got[1].Provenance, types.TypedRelationImplicitSiblingAggregateDemotionProvenance) {
		t.Fatalf("an explicitly model-authored second principal axis must be preserved: %+v", got[1])
	}
}

func TestExactTypedRelationProjectionDoesNotDemoteWithoutExactAuthority(t *testing.T) {
	bus := relationProjectionDriftBus()
	unrelatedA := types.AnswerAggregateFact{
		Kind: types.AnswerAggregateMemberSet, Label: "first", Value: "1",
		Members: []string{"Alpha"}, SupportRefs: []string{"internal/a.go:10"},
	}
	unrelatedB := types.AnswerAggregateFact{
		Kind: types.AnswerAggregateMemberSet, Label: "second", Value: "1",
		Members: []string{"Beta"}, SupportRefs: []string{"internal/b.go:20"},
	}

	got := markExactTypedRelationPrincipalMemberSets(bus, []types.AnswerAggregateFact{unrelatedA, unrelatedB})
	if len(got) != 2 || got[0].Role != types.AnswerAggregateRoleUnknown || got[1].Role != types.AnswerAggregateRoleUnknown {
		t.Fatalf("without exact typed authority implicit model sets must remain untouched: %+v", got)
	}
}

func TestEmitInvestigationCompletePersistsExactPrincipalAndImplicitSupportingSibling(t *testing.T) {
	bus := relationProjectionDriftBus()
	exact := relationProjectionDriftFact("LoopController", "prodA", "prodB")
	exact.Role = types.AnswerAggregateRoleUnknown
	implicitSibling := types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "upstream context",
		Value:       "1",
		Members:     []string{"Bootstrap"},
		SupportRefs: []string{"internal/bootstrap.go:10"},
	}
	raw, err := json.Marshal(map[string]any{
		"reason":          "exact typed relation roster with contextual upstream coverage",
		"confidence":      "high",
		"result_kind":     "resolved",
		"aggregate_facts": []types.AnswerAggregateFact{exact, implicitSibling},
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := (&EmitInvestigationComplete{}).Execute(bus, raw)
	if err != nil || !res.Success || bus.Mutable.InvestigationCompleteReason() == "" {
		t.Fatalf("completion must reach the production acceptance path: res=%+v err=%v", res, err)
	}
	got := bus.Mutable.StableInvestigationAggregateFacts()
	if len(got) != 2 || !types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(got[0]) {
		t.Fatalf("stable ledger lost exact typed principal authority: %+v", got)
	}
	if got[1].Role != types.AnswerAggregateRoleSupportingCoverage ||
		!strings.Contains(got[1].Provenance, types.TypedRelationImplicitSiblingAggregateDemotionProvenance) {
		t.Fatalf("stable ledger must retain the contextual sibling as supporting coverage: %+v", got[1])
	}
}

func TestExactTypedRelationProjectionKeepsSameNameCandidatesFromDistinctFiles(t *testing.T) {
	fact := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "handlers",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"Handle @ internal/a/handler.go:10",
			"Handle @ internal/b/handler.go:20",
		},
		SupportRefs: []string{
			"Handle @ internal/a/handler.go:10",
			"Handle @ internal/b/handler.go:20",
		},
	}
	candidates := []types.TypedRelationCandidate{
		{
			Relation: types.TypedRelationCalledBy, SourceName: "Dispatch",
			Member:    types.TypedRelationMember{Name: "Handle", File: "internal/a/handler.go", Line: 10},
			Precision: types.TypedRelationPrecisionExactSymbolID,
		},
		{
			Relation: types.TypedRelationCalledBy, SourceName: "Dispatch",
			Member:    types.TypedRelationMember{Name: "Handle", File: "internal/b/handler.go", Line: 20},
			Precision: types.TypedRelationPrecisionExactSymbolID,
		},
	}

	got, removed := canonicalizeExactTypedRelationFactMembers(fact, candidates)
	if removed != 0 || len(got.Members) != 2 {
		t.Fatalf("same-name candidates in distinct files are distinct typed identities: removed=%d fact=%+v", removed, got)
	}
}

func TestExactTypedRelationProjectionKeepsSameNameSourcesFromDistinctFiles(t *testing.T) {
	fact := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "dispatchers",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"Dispatch @ internal/a/dispatcher.go:10",
			"Dispatch @ internal/b/dispatcher.go:20",
		},
		SupportRefs: []string{
			"Dispatch @ internal/a/dispatcher.go:10",
			"Dispatch @ internal/b/dispatcher.go:20",
		},
	}
	candidates := []types.TypedRelationCandidate{
		{
			Relation: types.TypedRelationCalledBy, SourceName: "Dispatch", SourceFile: "internal/a/dispatcher.go",
			Member:    types.TypedRelationMember{Name: "HandleA", File: "internal/a/handler.go", Line: 11},
			Precision: types.TypedRelationPrecisionExactSymbolID,
		},
		{
			Relation: types.TypedRelationCalledBy, SourceName: "Dispatch", SourceFile: "internal/b/dispatcher.go",
			Member:    types.TypedRelationMember{Name: "HandleB", File: "internal/b/handler.go", Line: 21},
			Precision: types.TypedRelationPrecisionExactSymbolID,
		},
	}

	got, removed := canonicalizeExactTypedRelationFactMembers(fact, candidates)
	if removed != 0 || len(got.Members) != 2 {
		t.Fatalf("same-name relation sources in distinct files are distinct typed identities: removed=%d fact=%+v", removed, got)
	}
}

func TestExactTypedRelationProjectionCanonicalizesRepeatedSourceObservationsInSameFile(t *testing.T) {
	fact := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "dispatchers",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"Dispatch @ internal/a/dispatcher.go:10",
			"Dispatch @ internal/a/dispatcher.go:30",
		},
		SupportRefs: []string{
			"Dispatch @ internal/a/dispatcher.go:10",
			"Dispatch @ internal/a/dispatcher.go:30",
		},
	}
	candidates := []types.TypedRelationCandidate{
		{
			Relation: types.TypedRelationCalledBy, SourceName: "Dispatch", SourceFile: "internal/a/dispatcher.go",
			Member:    types.TypedRelationMember{Name: "HandleA", File: "internal/a/handler.go", Line: 11},
			Precision: types.TypedRelationPrecisionExactSymbolID,
		},
	}

	got, removed := canonicalizeExactTypedRelationFactMembers(fact, candidates)
	if removed != 1 || got.Value != "1" || len(got.Members) != 1 || len(got.SupportRefs) != 1 {
		t.Fatalf("same-file source observations must share one typed source identity: removed=%d fact=%+v", removed, got)
	}
}

func TestExactTypedRelationProjectionFailsOpenWhenSourceFileAxisIsUnavailable(t *testing.T) {
	fact := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "dispatchers",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"Dispatch @ internal/a/dispatcher.go:10",
			"Dispatch @ internal/b/dispatcher.go:20",
		},
		SupportRefs: []string{
			"Dispatch @ internal/a/dispatcher.go:10",
			"Dispatch @ internal/b/dispatcher.go:20",
		},
	}
	candidates := []types.TypedRelationCandidate{
		{
			Relation: types.TypedRelationCalledBy, SourceName: "Dispatch",
			Member:    types.TypedRelationMember{Name: "Handle", File: "internal/handler.go", Line: 11},
			Precision: types.TypedRelationPrecisionExactSymbolID,
		},
	}

	got, removed := canonicalizeExactTypedRelationFactMembers(fact, candidates)
	if removed != 0 || len(got.Members) != 2 {
		t.Fatalf("source identity without a typed file axis must fail open: removed=%d fact=%+v", removed, got)
	}
}

func TestExactTypedRelationProjectionFailsOpenOnSupportFileMismatch(t *testing.T) {
	fact := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "handlers",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"Handle @ internal/a/handler.go:10",
			"Handle @ internal/untyped/handler.go:20",
		},
		SupportRefs: []string{
			"Handle @ internal/a/handler.go:10",
			"Handle @ internal/untyped/handler.go:20",
		},
	}
	candidates := []types.TypedRelationCandidate{{
		Relation: types.TypedRelationCalledBy, SourceName: "Dispatch",
		Member:    types.TypedRelationMember{Name: "Handle", File: "internal/a/handler.go", Line: 10},
		Precision: types.TypedRelationPrecisionExactSymbolID,
	}}

	got, removed := canonicalizeExactTypedRelationFactMembers(fact, candidates)
	if removed != 0 || len(got.Members) != 2 {
		t.Fatalf("a same-name row anchored outside the typed candidate file must fail open: removed=%d fact=%+v", removed, got)
	}
}

func TestEmitInvestigationCompleteHardRejectsExactAuxiliaryRelationPromotion(t *testing.T) {
	bus := relationProjectionDriftBus()
	fact := relationProjectionDriftFact("LoopController", "prodA", "prodB", "testStub")
	raw, err := json.Marshal(map[string]any{
		"reason":          "typed relation roster selected",
		"confidence":      "high",
		"result_kind":     "resolved",
		"aggregate_facts": []types.AnswerAggregateFact{fact},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := (&EmitInvestigationComplete{}).Execute(bus, raw)
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if res.Success {
		t.Fatalf("exact auxiliary promotion must be a non-converging hard reject: %s", res.Summary)
	}
	for _, want := range []string{
		"outside the requested source scope",
		"testStub",
		"source_role=test",
		"internal/agent/agent_test.go",
		"role=\"supporting_coverage\"",
		"do not create an excluded-only empty member_set",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("hard reject missing %q: %s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "Move these rows to excluded[]") {
		t.Fatalf("repair teaching must not recommend an aggregate shape rejected by the next contract: %s", res.Summary)
	}
	if bus.Mutable.InvestigationCompleteReason() != "" {
		t.Fatalf("rejected completion must not mutate investigation completion")
	}
}

func TestExactTypedRelationProjectionSkipsSourceInventoryCompletionOwners(t *testing.T) {
	bus := relationProjectionDriftBus()
	facts := markExactTypedRelationPrincipalMemberSets(bus, []types.AnswerAggregateFact{
		relationProjectionDriftFact("LoopController", "prodA", "prodB"),
	})
	if got := sourceInventoryResolvedCompletionDowngrade(bus, "resolved", facts); got != "" {
		t.Fatalf("source-inventory completion must yield to exact relation authority: %s", got)
	}
	if got := sourceInventoryLensExecutionDowngrade(bus, facts); got != "" {
		t.Fatalf("source-inventory lens must not become a second completion owner: %s", got)
	}
	if got := exhaustiveEnumerationMemberSetDowngrade(bus, bus.Mutable.EvidenceClosure(), facts); got != "" {
		t.Fatalf("generic exhaustive gate must yield to the preceding relation gate: %s", got)
	}
}
