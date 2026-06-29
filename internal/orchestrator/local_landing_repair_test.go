package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLocalLandingRepairDispatchPolicyCompletionFormDebt(t *testing.T) {
	repairs := []types.RepairDirective{
		{
			Kind:          types.RepairStructuredHandoff,
			Origin:        types.RepairOriginCompletionFormPrefix + "member_set_support_refs",
			Subject:       "completion_form:member_set_support_refs",
			DowngradeLane: types.DowngradeLaneCompletionForm,
		},
	}
	policy, hint, ok := localLandingRepairDispatchPolicy(repairs, nil, nil, nil, "", "", nil)
	if !ok {
		t.Fatal("completion-form repair should activate local landing")
	}
	if !policy.IsActive() ||
		policy.Action != types.ReadDispatchPolicyActionLandingRepair ||
		policy.RouteSurface != types.ReadDispatchPolicySurfaceHandoff {
		t.Fatalf("unexpected policy: %+v", policy)
	}
	for _, allowed := range []string{"emit_investigation_complete"} {
		if !policy.AllowsTool(allowed) {
			t.Fatalf("policy should allow %s: %+v", allowed, policy)
		}
	}
	for _, denied := range []string{"repo_map", "grep", "read_file", "list_files", "exec_command", "trace_query", "run_tests", "emit_evidence"} {
		if policy.AllowsTool(denied) {
			t.Fatalf("policy should deny %s: %+v", denied, policy)
		}
	}
	if !strings.Contains(hint, "Local landing repair") ||
		!strings.Contains(hint, "Do not reopen evidence collection") ||
		!strings.Contains(hint, "emit_investigation_complete") ||
		!strings.Contains(hint, "do not call emit_evidence") {
		t.Fatalf("landing hint missing required boundary: %s", hint)
	}
	repairHint := renderWindowHint(nil, nil, nil, func(string) string { return "" }, "", "", repairs)
	combined := prependRetryHint(hint, repairHint)
	if strings.Contains(combined, "DAG-scheduled investigation window") {
		t.Fatalf("local landing hint must not include broad DAG window objectives:\n%s", combined)
	}
	if strings.Contains(combined, "re-emit grounded evidence if needed") {
		t.Fatalf("local landing hint must not suggest unavailable evidence emission:\n%s", combined)
	}
	if !strings.Contains(combined, "Structured Handoff Repair") {
		t.Fatalf("local landing hint should preserve structured repair details:\n%s", combined)
	}
}

func TestLocalLandingRepairDispatchPolicyDoesNotMaskEvidenceDebt(t *testing.T) {
	completionForm := types.RepairDirective{
		Kind:          types.RepairStructuredHandoff,
		Origin:        types.RepairOriginCompletionFormPrefix + "aggregate_facts",
		DowngradeLane: types.DowngradeLaneCompletionForm,
	}
	cases := []struct {
		name        string
		repairs     []types.RepairDirective
		pendingRead []types.PendingRead
		blocked     []nodeBlock
		validation  []string
		violation   string
		stageRetry  string
	}{
		{
			name:    "read repair",
			repairs: []types.RepairDirective{completionForm, {Kind: types.RepairReadFile, Files: []string{"internal/a.go"}, Origin: "grounder"}},
		},
		{
			name:        "pending read",
			repairs:     []types.RepairDirective{completionForm},
			pendingRead: []types.PendingRead{{File: "internal/a.go", Origin: "grounder"}},
		},
		{
			name:    "blocked entry condition",
			repairs: []types.RepairDirective{completionForm},
			blocked: []nodeBlock{{NodeID: "n1"}},
		},
		{
			name:       "validation target",
			repairs:    []types.RepairDirective{completionForm},
			validation: []string{"n1"},
		},
		{
			name:      "final answer violation",
			repairs:   []types.RepairDirective{completionForm},
			violation: "citation floor failed",
		},
		{
			name:       "stage retry",
			repairs:    []types.RepairDirective{completionForm},
			stageRetry: "missing evidence",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if policy, hint, ok := localLandingRepairDispatchPolicy(tc.repairs, tc.pendingRead, tc.blocked, tc.validation, tc.violation, tc.stageRetry, nil); ok {
				t.Fatalf("local landing must not activate for evidence/control debt; policy=%+v hint=%q", policy, hint)
			}
		})
	}
}

func TestLocalLandingRepairStopsAfterCompletionFormCaveat(t *testing.T) {
	repairs := []types.RepairDirective{
		{
			Kind:    types.RepairStructuredHandoff,
			Origin:  types.RepairOriginCompletionFormPrefix + "member_set_support_refs",
			Subject: "completion_form:member_set_support_refs",
		},
		{
			Kind:          types.RepairStructuredHandoff,
			Origin:        "emit_investigation_complete.principal_member_set_handoff",
			Subject:       "principal_member_set_handoff:exact_members",
			DowngradeLane: types.DowngradeLanePrincipalMemberSetHandoff,
		},
	}
	caveats := []types.CompletionCaveat{{Lane: types.DowngradeLaneCompletionForm, ReasonCode: types.ProgressReasonConverged}}

	filtered, dropped := filterConvergedDowngradeLaneRepairs(repairs, caveats)
	if dropped != 1 || len(filtered) != 1 || filtered[0].DowngradeLane != types.DowngradeLanePrincipalMemberSetHandoff {
		t.Fatalf("completion-form caveat should drop only completion-form repair; dropped=%d filtered=%+v", dropped, filtered)
	}
	if policy, hint, ok := localLandingRepairDispatchPolicy(repairs[:1], nil, nil, nil, "", "", caveats); ok {
		t.Fatalf("local landing must not activate after completion-form convergence; policy=%+v hint=%q", policy, hint)
	}
}
