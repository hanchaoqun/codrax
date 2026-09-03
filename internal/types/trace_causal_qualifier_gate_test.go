package types

import "testing"

// trace_causal_qualifier_gate_test.go — QUALGATE-1 (user ruling §40.30
// V-QUAL-1 plan A, 2026-09-02): the qualifier closed set is exactly three
// values, the typed frame-question gate reads only the analyzer decision, and
// a not_applicable item normalizes with a bare summary.
func TestTraceCausalQualifierClosedSetIsExactlyThreeValues(t *testing.T) {
	all := AllTraceCausalQualifiers()
	if len(all) != 3 || all[0] != TraceCausalQualifierProven || all[1] != TraceCausalQualifierFrameUnproven || all[2] != TraceCausalQualifierNotApplicable {
		t.Fatalf("closed set drifted: %v", all)
	}
	for _, v := range all {
		if !ValidTraceCausalQualifier(v) {
			t.Fatalf("%q must be valid", v)
		}
	}
	for _, v := range []string{"", "unproven", "maybe", "PROVEN"} {
		if ValidTraceCausalQualifier(v) {
			t.Fatalf("%q must be rejected", v)
		}
	}
}

func TestFrameCausalityQualifierApplicableReadsOnlyTheTypedDecision(t *testing.T) {
	if FrameCausalityQualifierApplicable(nil) {
		t.Fatal("nil request model ⇒ not applicable (fail closed)")
	}
	rm := &RequestModel{Intent: IntentRootCause, Scenario: ScenarioPerformanceBottleneck}
	if FrameCausalityQualifierApplicable(rm) {
		t.Fatal("scenario/intent labels must not open the gate; only the typed profile decision does")
	}
	rm.RuntimeQuestionProfile = &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeCausalDiagnosis}
	if FrameCausalityQualifierApplicable(rm) || rm.RuntimeQuestionProfile.RequestsFrameCausality() {
		t.Fatal("frame_causality_requested=false ⇒ closed")
	}
	rm.RuntimeQuestionProfile.FrameCausalityRequested = true
	if !FrameCausalityQualifierApplicable(rm) {
		t.Fatal("frame_causality_requested=true ⇒ open")
	}
	var nilProfile *RuntimeQuestionProfile
	if nilProfile.RequestsFrameCausality() {
		t.Fatal("nil profile accessor must be false")
	}
}

func TestNormalizeTraceRootCauseReportAcceptsNotApplicableWithBareSummary(t *testing.T) {
	report, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{
		SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCauses: []*TraceRootCauseItemV2{{
			Category: TraceRootCauseCPUSchedulingDelay, ThreadName: "RenderThread",
			ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierNotApplicable,
			ImpactSeconds: traceImpact(0.004), Evidence: []string{"observed wait"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := report.RootCauses[0]
	if item.CausalQualifier != TraceCausalQualifierNotApplicable || item.Summary != "RenderThread线程CPU调度延迟" {
		t.Fatalf("not_applicable must normalize explicit with a bare summary: %+v", item)
	}
}
