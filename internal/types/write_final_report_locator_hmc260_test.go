package types

import "testing"

func TestHMC260FinalReportRetainsProducerLocatorAndExplicitGap(t *testing.T) {
	report := &ChangeReport{PlanID: "plan", Passed: true}
	ref := "plan.exact-original.report.json"
	out := BuildWriteFinalReport(WriteFinalReportInput{Report: report, ReportID: &ref, ReportPath: "/retained/" + ref})
	if out.Verification.ReportID != ref || out.Artifacts.ReportPath != "/retained/"+ref || !out.Verification.Passed {
		t.Fatalf("exact source degraded back to mutable plan view: %+v", out)
	}
	empty := ""
	out = BuildWriteFinalReport(WriteFinalReportInput{Report: report, ReportID: &empty})
	if out.Verification.ReportID != "" || !out.Verification.Passed {
		t.Fatal("publication gap was invented or changed real execution status")
	}
	out = BuildWriteFinalReport(WriteFinalReportInput{ReportID: &ref})
	if out.Verification.ReportID != ref || out.Verification.Passed || out.Verification.Status != "" {
		t.Fatal("unavailable original became success or lost its missing reference")
	}
	out = BuildWriteFinalReport(WriteFinalReportInput{Report: report})
	if out.Verification.ReportID != "plan.report.json" {
		t.Fatal("legacy standalone caller lost current-view compatibility")
	}
}
