package types

import "testing"

// A matching test name is not a file or execution identity. In particular,
// a failure in one test file must not retire an expectation declared for its
// identically named sibling test. The types layer has no runner resolver;
// without a binding from that resolver it must retain the expectation.
func TestVerifyFailureContractRelevanceDoesNotBorrowUnboundProjectAssertion(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-sibling-assertions",
		ProjectTestObservations: []ProjectTestObservation{{
			ID: "a-edge", TestPath: "tests/a_test.py",
			AssertionSuite: "BoundaryTests", AssertionID: "test_edge",
			ContractRefs: []string{"a-expectation"},
		}},
	}
	for _, suite := range []string{"tests/b_test.py::BoundaryTests", "tests/a_test.py::BoundaryTests"} {
		t.Run(suite, func(t *testing.T) {
			report := &ChangeReport{
				PlanID: plan.ID,
				TestResults: []TestResult{{
					Kind: TestResultKindUnit, ObservationScope: TestObservationScopeAssertion,
					Suite: suite, AssertionID: "test_edge", Passed: false,
				}},
			}
			got := BuildVerifyFailureContractRelevance(report, plan)
			if len(got.Hits) != 0 {
				t.Fatalf("unbound project failure must retain the contract, got %+v", got)
			}
			if got.ReasonCode != "typed_failed_rows_joined_with_unbound_project_assertions" {
				t.Fatalf("missing execution/path binding must be disclosed: %+v", got)
			}
		})
	}
}
