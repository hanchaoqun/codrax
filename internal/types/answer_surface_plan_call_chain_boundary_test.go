package types

import "testing"

func TestBuildAnswerSurfacePlan_NoDirectedPathProjectsOutPathRosterAndClosureReason(t *testing.T) {
	mut := NewMutableState("trace Source.run to Sink.run")
	mut.SetPrincipalSpanWaiver(&PrincipalSpanWaiver{
		Reason:    PrincipalSpanWaiverNoDirectedPath,
		Rationale: "Source.run reaches Helper.run while Sink.run calls Helper.run",
	})
	mut.SetInvestigationComplete("model says a complete path exists")
	mut.SetInvestigationAggregateFacts([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "model complete path roster",
			Value:   "2",
			Role:    AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Source.run", "Sink.run"},
		},
		{
			Kind:  AnswerAggregateScalar,
			Label: "inspected edge count",
			Value: "2",
			Role:  AnswerAggregateRoleSupportingCoverage,
		},
	})
	mut.RetainInvestigationAggregateFacts()
	ir := &AnalysisIR{RequestModel: RequestModel{
		Intent:                   IntentTrace,
		PredicateAxis:            AxisCall,
		CallChainEndpointProfile: &CallChainEndpointProfile{Source: "Source.run", Sink: "Sink.run"},
		AnalyzerHints: AnalyzerHints{
			Kind:         string(ReqCallChain),
			ExactTargets: []string{"Source.run", "Sink.run"},
		},
	}}
	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, nil)
	if plan == nil {
		t.Fatal("plan is nil")
	}
	if plan.StableInvestigationReason != "" {
		t.Fatalf("model closure reason must not compete with typed no-path boundary: %q", plan.StableInvestigationReason)
	}
	if len(plan.StableAggregateFacts) != 1 || plan.StableAggregateFacts[0].Kind != AnswerAggregateScalar {
		t.Fatalf("path member_set must be absent from answer authority while non-path audit facts survive: %+v", plan.StableAggregateFacts)
	}
	if got := mut.StableInvestigationAggregateFacts(); len(got) != 2 {
		t.Fatalf("raw accepted facts must remain available for audit/resume: %+v", got)
	}
}
