package writeflow

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Stored labels are an input to the public qualifier, not current execution or
// assertion receipts. The process pass must survive even when authority does not.
func TestB1575NoChangeReplanUsesEffectiveReportAuthority(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*types.ChangePlan, *types.ChangeReport)
		allow  bool
		reason string
	}{
		{name: "legacy_python_behavior", reason: "planner_probe_target_authority_missing"},
		{name: "legacy_python_execution_label", modify: func(_ *types.ChangePlan, r *types.ChangeReport) {
			r.ChangedPathCoverage[0].Capability = types.VerificationCapabilityTargetExecution
		}, reason: "planner_probe_target_authority_missing"},
		{name: "legacy_python_contract_refs", modify: func(_ *types.ChangePlan, r *types.ChangeReport) {
			r.VerificationConfidence = []types.VerificationConfidenceRecord{{
				Category: "probe_contract_refs", Status: "satisfied", Severity: "info",
				ContractRefs: []string{"runtime-contract"},
			}}
		}, reason: "python_plain_probe_assertion_witness_missing"},
		{name: "legacy_python_placement_refs", modify: func(_ *types.ChangePlan, r *types.ChangeReport) {
			r.VerificationConfidence = []types.VerificationConfidenceRecord{{
				Category: "probe_placement_refs", Status: "satisfied", Severity: "info",
				ContractRefs: []string{"placement-contract"},
			}}
		}, reason: "python_plain_probe_assertion_witness_missing"},
		{name: "native_project_observation", modify: b1575NoChangeNativeReport, allow: true},
		{name: "non_python_legacy_lane_unchanged", modify: func(p *types.ChangePlan, r *types.ChangeReport) {
			p.TargetPaths = []string{"src/widget.js"}
			r.ChangedPathCoverage[0].Path = "src/widget.js"
			r.ChangedPathCoverage[0].LanguageFamilies = []types.VerificationLanguageFamily{types.VerificationLanguageJavaScript}
			r.TestResults[0].Suite = "verification_probe/javascript"
		}, allow: true},
		{name: "native_other_target_not_borrowed", modify: func(p *types.ChangePlan, r *types.ChangeReport) {
			b1575NoChangeNativeReport(p, r)
			r.ChangedPathCoverage[0].Path = "src/other.py"
		}, reason: "planner_probe_target_authority_missing"},
		{name: "native_warning_unverified", modify: func(p *types.ChangePlan, r *types.ChangeReport) {
			b1575NoChangeNativeReport(p, r)
			r.VerificationConfidence = []types.VerificationConfidenceRecord{{
				Category: "behavior", Status: "unverified", Severity: "warning", ReasonCode: "assertion_receipt_missing",
			}}
		}, reason: "assertion_receipt_missing"},
		{name: "native_warning_unverified_without_reason", modify: func(p *types.ChangePlan, r *types.ChangeReport) {
			b1575NoChangeNativeReport(p, r)
			r.VerificationConfidence = []types.VerificationConfidenceRecord{{Status: "unverified", Severity: "warning"}}
		}, reason: "planner_probe_confidence_warning"},
		{name: "native_info_unverified_not_new_warning_gate", modify: func(p *types.ChangePlan, r *types.ChangeReport) {
			b1575NoChangeNativeReport(p, r)
			r.VerificationConfidence = []types.VerificationConfidenceRecord{{Status: "unverified", Severity: "info"}}
		}, allow: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := time.Date(2026, 9, 11, 1, 0, 0, 0, time.UTC)
			plan := &types.ChangePlan{ID: "applied", AppliedCommitSHA: "abc123", TargetPaths: []string{"src/widget.py"}}
			report := &types.ChangeReport{
				PlanID: plan.ID, Channel: types.ChangeReportChannelPlannerProbe,
				Passed: true, VerificationStatus: types.VerificationStatusPassed, GeneratedAt: at.Add(time.Second),
				TestResults: []types.TestResult{{AssertionID: "one-off", Suite: "verification_probe/python", Kind: types.TestResultKindUnit, Passed: true}},
				ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
					Path: "src/widget.py", Status: types.ChangedPathVerificationCovered,
					Capability: types.VerificationCapabilityTargetBehavior, Caliber: types.ChangedPathVerificationProbe,
					Runner: "verification_probe", Source: "one-off", LanguageFamilies: []types.VerificationLanguageFamily{types.VerificationLanguagePython},
				}},
			}
			if tc.modify != nil {
				tc.modify(plan, report)
			}
			in := NoChangeReplanQualificationInput{
				PriorPlan: plan, RequireAppliedWork: true, PlannerProbeReports: []*types.ChangeReport{report},
				VerifyFailureHandoff: &types.VerifyFailureHandoff{PlanID: plan.ID, FailureKind: types.FailureKindTestsFailed, GeneratedAt: at},
			}
			before, err := json.Marshal(in)
			if err != nil {
				t.Fatal(err)
			}
			got := QualifyNoChangeReplanSentinel(in)
			if got.Allowed != tc.allow || (!tc.allow && got.ReasonCode != tc.reason) {
				t.Errorf("qualification=%+v; want allowed=%v reason=%q", got, tc.allow, tc.reason)
			}
			if tc.allow && (got.ProbePlanID != plan.ID || !got.ProbeGeneratedAt.Equal(report.GeneratedAt)) {
				t.Errorf("qualified report identity changed: %+v", got)
			}
			after, err := json.Marshal(in)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || !report.Passed || !report.TestResults[0].Passed || len(plan.VerificationProbes) != 0 {
				t.Fatal("qualification mutated original reports, process results, or prior-plan probe declarations")
			}
		})
	}
}

func b1575NoChangeNativeReport(_ *types.ChangePlan, r *types.ChangeReport) {
	r.TestResults[0].Suite = "pytest"
	r.ChangedPathCoverage[0].Caliber = types.ChangedPathVerificationProjectRunner
	r.ChangedPathCoverage[0].Runner = "pytest"
}
