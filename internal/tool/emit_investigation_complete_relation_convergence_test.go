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
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("hard reject missing %q: %s", want, res.Summary)
		}
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
