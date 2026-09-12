package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestVerificationCardsDiscloseObservationWithoutChangingVerdict(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, render := range []func(*types.ChangeReport, string) string{renderVerifySuccess, renderVerifyUnverified, func(report *types.ChangeReport, language string) string {
			return renderVerifyFailure(report, "", language)
		}} {
			report := &types.ChangeReport{PlanID: "p", Passed: true, TestResults: []types.TestResult{{Passed: true}}}
			original := render(report, lang)
			report.VerificationDiagnostics = []types.VerificationDiagnostic{{Category: "probe_comparator_authority", ReasonCode: "model_authored_probe_comparator_unverified", Runner: "verification_probe", Outcome: "observed_failure", FailureObservations: []types.VerificationFailureObservation{{AssertionID: "check-boundary", Suite: "verification_probe/python", FailureDetail: "unchanged input became changed"}}}}
			before, _ := json.Marshal(report)
			got := render(report, lang)
			if !strings.HasPrefix(got, original) || !strings.Contains(got, "check-boundary") || !strings.Contains(got, "unchanged input became changed") {
				t.Errorf("%s card lost the original verdict or supplementary observation: %s", lang, got)
			}
			if lang == "zh" {
				for _, forbidden := range []string{"assertion_id=", "failure_detail=", "output_ref=", "unavailable（"} {
					if strings.Contains(got, forbidden) {
						t.Errorf("Chinese card leaks %s", forbidden)
					}
				}
				if !strings.Contains(got, "不等于已证实产品缺陷") {
					t.Error("missing observation boundary")
				}
			} else if !strings.Contains(got, "does not prove a product defect") {
				t.Error("missing observation boundary")
			}
			after, _ := json.Marshal(report)
			if string(before) != string(after) {
				t.Error("card mutated the report")
			}
		}
	}
}
