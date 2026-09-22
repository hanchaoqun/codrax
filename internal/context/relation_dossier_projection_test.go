package context

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRelationDossierProjectionRespectsStageAndAbsentPlan(t *testing.T) {
	for _, stage := range []types.PipelineStage{types.StageFinalize, types.StageExplore, types.StageExtract} {
		for _, withPlan := range []bool{false, true} {
			for _, noPath := range []bool{false, true} {
				name := string(stage) + map[bool]string{false: "/legacy", true: "/plan"}[withPlan] + map[bool]string{false: "/source", true: "/no_path"}[noPath]
				t.Run(name, func(t *testing.T) {
					mu := types.NewMutableState("inspect source relationships")
					current := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Label: "current_relation_marker", Value: "1", Role: types.AnswerAggregateRolePrincipalAnswer, Members: []string{"First -> Second"}}
					old := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Label: "accepted_relation_marker", Value: "1", Role: types.AnswerAggregateRoleSupportingCoverage, Members: []string{"Other -> Leaf"}}
					mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{current})
					mu.RetainInvestigationAggregateFacts()
					mu.SetTurnAArtifacts(types.TurnAArtifacts{AcceptedAggregateFacts: []types.AnswerAggregateFact{old}})
					ac := &types.AgentContext{AgentName: types.AgentFinalizer, Stage: stage, Mutable: mu}
					if withPlan {
						ac.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}
					}
					if noPath {
						mu.SetPrincipalSpanWaiver(&types.PrincipalSpanWaiver{Reason: types.PrincipalSpanWaiverNoDirectedPath, Rationale: "no directed path"})
						if withPlan {
							ac.AnalysisIR.RequestModel = types.RequestModel{
								Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
								CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "First", Sink: "Second"},
								AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain), ExactTargets: []string{"First", "Second"}},
							}
						}
					}
					snapshot := func() string {
						b, _ := json.Marshal([]any{ac.AnalysisIR, mu.StableInvestigationAggregateFacts(), mu.TurnAArtifacts(), mu.PrincipalSpanWaiver()})
						return string(b)
					}
					before := snapshot()
					raw := types.MergeAnswerAggregateFacts(mu.StableInvestigationAggregateFacts(), mu.TurnAArtifacts().AcceptedAggregateFacts)
					plan := types.BuildAnswerSurfacePlanForAgentContext(ac)
					if (plan != nil) != withPlan {
						t.Fatal("fixture plan presence drifted")
					}
					if withPlan && noPath && len(plan.StableAggregateFacts) != 0 {
						t.Fatalf("typed no-path must actually exclude these rosters: %+v", plan.StableAggregateFacts)
					}
					want := raw
					if stage == types.StageFinalize && plan != nil {
						want = plan.StableAggregateFacts
					}
					if got := relationDossierAggregateFacts(ac); !reflect.DeepEqual(got, want) {
						t.Errorf("aggregate source differs from stage authority: got %+v want %+v", got, want)
					}
					var actual strings.Builder
					for _, message := range ToMessages(BuildPromptContext(ac, &skill.Config{Name: "relation-projection-test"})) {
						actual.WriteString(message.Content)
					}
					for _, label := range []string{current.Label, old.Label} {
						if got := strings.Contains(actual.String(), label); got != (len(want) > 0) {
							t.Errorf("actual message relation roster %q present=%v want=%v", label, got, len(want) > 0)
						}
					}
					if before != snapshot() {
						t.Fatal("projection changed durable audit or typed request")
					}
				})
			}
		}
	}
}
