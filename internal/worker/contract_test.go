package worker

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/safety"
)

func TestValidateRequestAllowsTypedReadOnlyWorker(t *testing.T) {
	req := Request{
		ID:    "worker-1",
		Kind:  KindLocalizer,
		Goal:  "find owner anchors",
		Scope: Scope{Paths: []string{"./pkg/a.py", "pkg/a.py"}, Symbols: []string{"Owner"}},
		InputRefs: []loopkernel.ArtifactRef{
			{Kind: "localization_authority", ID: "loc-1"},
		},
	}
	req = NormalizeRequest(req)
	if req.Role != safety.EffectRoleLocalizer {
		t.Fatalf("expected localizer role default, got %+v", req)
	}
	if len(req.Scope.Paths) != 1 || req.Scope.Paths[0] != "pkg/a.py" {
		t.Fatalf("paths should normalize and dedupe, got %+v", req.Scope.Paths)
	}
	if errs := ValidateRequest(req); len(errs) != 0 {
		t.Fatalf("valid localizer request should pass, got %+v", errs)
	}
	decision := RequestPermissionDecision(req)
	if decision.Action != safety.PermissionAllow {
		t.Fatalf("read-only localizer effect should allow, got %+v", decision)
	}
}

func TestValidateRequestDeniesMutationForEvidenceWorkers(t *testing.T) {
	req := Request{
		ID:    "worker-2",
		Kind:  KindPatchCritic,
		Goal:  "review actual diff",
		Scope: Scope{Paths: []string{"pkg/a.py"}, AllowMutation: true},
	}
	errs := ValidateRequest(req)
	if !hasValidationCode(errs, "worker_mutation_not_allowed") {
		t.Fatalf("mutation request should have explicit contract error, got %+v", errs)
	}
	if !hasValidationCode(errs, "role_effect_denied") {
		t.Fatalf("mutation effect should be denied by shared safety policy, got %+v", errs)
	}
	decision := RequestPermissionDecision(req)
	if decision.Action != safety.PermissionDeny {
		t.Fatalf("mutation-capable worker request should deny, got %+v", decision)
	}
}

func TestValidateRequestBlocksExternalDirectoryRead(t *testing.T) {
	req := Request{
		ID:    "worker-3",
		Kind:  KindImpactAnalyzer,
		Goal:  "inspect impact outside scope",
		Scope: Scope{Paths: []string{"../outside.py"}},
	}
	errs := ValidateRequest(req)
	if !hasValidationCode(errs, "external_directory_effect") {
		t.Fatalf("external path should require review and fail worker validation, got %+v", errs)
	}
	decision := RequestPermissionDecision(req)
	if decision.Action != safety.PermissionAsk {
		t.Fatalf("external read should ask at policy level, got %+v", decision)
	}
}

func TestValidateResultRequiresTypedOutput(t *testing.T) {
	empty := Result{
		RequestID: "worker-4",
		Kind:      KindProofAuditor,
		Status:    StatusComplete,
	}
	if errs := ValidateResult(empty); !hasValidationCode(errs, "worker_result_typed_output_required") {
		t.Fatalf("complete result without typed refs should fail, got %+v", errs)
	}
	ok := Result{
		RequestID: "worker-4",
		Kind:      KindProofAuditor,
		Status:    StatusComplete,
		OutputRefs: []loopkernel.ArtifactRef{
			{Kind: "proof_coverage_authority", ID: "proof-1"},
		},
	}
	if errs := ValidateResult(ok); len(errs) != 0 {
		t.Fatalf("complete result with typed ref should pass, got %+v", errs)
	}
}

func TestWorkerRoleEffectMatrix(t *testing.T) {
	tests := []struct {
		kind       Kind
		role       safety.EffectRole
		effectKind safety.EffectKind
	}{
		{KindLocalizer, safety.EffectRoleLocalizer, safety.EffectKindRepoMap},
		{KindImpactAnalyzer, safety.EffectRoleImpactAnalyzer, safety.EffectKindImpactAnalysis},
		{KindPatchCritic, safety.EffectRolePatchCritic, safety.EffectKindPatchReview},
		{KindProofAuditor, safety.EffectRoleProofAuditor, safety.EffectKindProofAudit},
		{KindFailureAnalyzer, safety.EffectRoleFailureAnalyzer, safety.EffectKindFailureAnalysis},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			if got := RoleForKind(tc.kind); got != tc.role {
				t.Fatalf("role mismatch: got %s want %s", got, tc.role)
			}
			req := Request{ID: "w", Kind: tc.kind, Goal: "produce typed evidence", Scope: Scope{Paths: []string{"pkg/a.py"}}}
			effects := RequestEffectDescriptors(req)
			if len(effects) != 1 || effects[0].Kind != tc.effectKind || effects[0].Role != tc.role {
				t.Fatalf("effect envelope mismatch: %+v", effects)
			}
			if decision := safety.DecideEffectPermission(effects[0]); decision.Action != safety.PermissionAllow {
				t.Fatalf("worker evidence effect should allow, got %+v", decision)
			}
		})
	}
}

func TestValidateResultRequiresReasonForBlockedOrFailed(t *testing.T) {
	result := Result{RequestID: "worker-5", Kind: KindFailureAnalyzer, Status: StatusBlocked}
	if errs := ValidateResult(result); !hasValidationCode(errs, "worker_result_reason_required") {
		t.Fatalf("blocked result should require typed reason, got %+v", errs)
	}
}

func hasValidationCode(errs []ValidationError, code string) bool {
	for _, err := range errs {
		if err.Code == code {
			return true
		}
	}
	return false
}
