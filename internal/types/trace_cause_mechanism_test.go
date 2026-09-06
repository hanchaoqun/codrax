package types

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestTraceNodePriorityCandidateUsesOnlyTypedProducerFields(t *testing.T) {
	for _, tc := range []struct {
		node TraceCausalProjectionNode
		want bool
	}{
		{TraceCausalProjectionNode{TypeToken: "priority_inversion_candidate"}, true},
		{TraceCausalProjectionNode{TypeToken: "priority_inversion_runnable_wait"}, true},
		{TraceCausalProjectionNode{PriorityInversionCandidate: true, TypeToken: "running"}, true},
		{TraceCausalProjectionNode{TypeToken: "running", Subject: "priority_inversion_candidate", FixDirection: "lock_priority"}, false},
		{TraceCausalProjectionNode{TypeToken: "runnable_wait", GatedRunnableMS: 10}, false},
	} {
		if got := TraceNodeIsPriorityInversionCandidate(tc.node); got != tc.want {
			t.Fatalf("typed candidate predicate: got %v want %v for %+v", got, tc.want, tc.node)
		}
	}
}

func mechanismReportFixture() *TraceRootCauseReportV2 {
	amount := .009
	return &TraceRootCauseReportV2{SchemaVersion: 2, RootCauses: []*TraceRootCauseItemV2{{
		Category: TraceRootCausePriorityInversion, ThreadName: "worker-9", ImpactSeconds: &amount,
		ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierNotApplicable,
		Evidence: []string{"链上供给测量为 9 ms"},
	}}}
}

func TestTraceRootCauseMechanismExtensionPreservesLegacyAndClonesNewFacts(t *testing.T) {
	legacy := mechanismReportFixture()
	report, err := NormalizeAndValidateTraceRootCauseReport(legacy)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "mechanism_qualifier") || strings.Contains(string(raw), "impact_breakdown") || report.RootCauses[0].Summary != "worker-9线程优先级反转" {
		t.Fatalf("an absent optional extension must preserve the legacy wire: %s", raw)
	}
	item := legacy.RootCauses[0]
	item.MechanismQualifier = TraceMechanismLowerPriorityDependencyCandidate
	item.ImpactBreakdown = &TraceRootCauseImpactBreakdown{RunnableSeconds: .005, RunningDeficitSeconds: .004, CapabilitySource: "default_table"}
	report, err = NormalizeAndValidateTraceRootCauseReport(legacy)
	if err != nil || report.RootCauses[0].Summary != "worker-9线程优先级反转候选" {
		t.Fatalf("qualifier lost: %v %+v", err, report)
	}
	item.ImpactBreakdown.RunnableSeconds = 10
	if report.RootCauses[0].ImpactBreakdown.RunnableSeconds != .005 {
		t.Fatal("normalization aliases the input composition")
	}
	state := NewMutableState("trace")
	state.SetTraceRootCauseReport(report)
	report.RootCauses[0].ImpactBreakdown.RunnableSeconds = 20
	read := state.TraceRootCauseReport()
	read.RootCauses[0].ImpactBreakdown.RunnableSeconds = 30
	if state.TraceRootCauseReport().RootCauses[0].ImpactBreakdown.RunnableSeconds != .005 {
		t.Fatal("public impact composition aliases mutable state across write or read")
	}
}

func TestTraceRootCauseImpactBreakdownRejectsWrongRulerAndInvalidComponents(t *testing.T) {
	for _, mutate := range []func(*TraceRootCauseItemV2){
		func(item *TraceRootCauseItemV2) { item.ImpactCaliber = TraceImpactCaliberWindowProjection },
		func(item *TraceRootCauseItemV2) { item.ImpactBreakdown.RunnableSeconds = -1 },
		func(item *TraceRootCauseItemV2) { item.ImpactBreakdown.RunningDeficitSeconds = math.NaN() },
		func(item *TraceRootCauseItemV2) { item.ImpactBreakdown.RunningDeficitSeconds = math.Inf(1) },
		func(item *TraceRootCauseItemV2) { item.ImpactBreakdown.RunnableSeconds = .007 },
		func(item *TraceRootCauseItemV2) { item.MechanismQualifier = "confirmed" },
	} {
		in := mechanismReportFixture()
		item := in.RootCauses[0]
		item.MechanismQualifier = TraceMechanismLowerPriorityDependencyCandidate
		item.ImpactBreakdown = &TraceRootCauseImpactBreakdown{RunnableSeconds: .005, RunningDeficitSeconds: .004}
		mutate(item)
		if _, err := NormalizeAndValidateTraceRootCauseReport(in); err == nil {
			t.Fatalf("invalid component/ruler/qualifier was accepted: %+v", item)
		}
	}
	// A measured zero in one component remains distinct from absent accounting.
	for _, split := range []TraceRootCauseImpactBreakdown{{RunnableSeconds: .009}, {RunningDeficitSeconds: .009}} {
		if out, err := NormalizeTraceRootCauseImpactBreakdown(&split, TraceImpactCaliberEffectiveAttribution, .009); err != nil || out == nil {
			t.Fatalf("measured zero component was lost: %v %+v", err, out)
		}
	}
}
