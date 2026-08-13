package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func verifyFailureProbeRefTestContextForPath(path string) *types.BusContext {
	mut := types.NewMutableState("repair Unicode behavior verification")
	mut.SetChangePlan(&types.ChangePlan{BehaviorContracts: []types.WriteBehaviorContract{{
		ID:       "unicode-output",
		Required: true,
		Polarity: types.WriteBehaviorPolarityExpected,
		Subject:  "RandomStringUtils.random",
	}}})
	mut.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{
		PlanID:            "plan-1",
		BatchID:           "batch-1",
		FailureReasonCode: changedPathVerificationUncoveredReasonCode,
		Confidence: []types.VerificationConfidenceRecord{{
			ReasonCode:        changedPathVerificationUncoveredReasonCode,
			ChangedSymbolRefs: []string{"path:" + path},
		}},
	})
	return &types.BusContext{Mutable: mut, Mode: types.ModeApply, PipelineStage: types.StagePlan}
}

func verifyFailureProbeRefTestContext() *types.BusContext {
	return verifyFailureProbeRefTestContextForPath("src/main/java/org/example/RandomStringUtils.java")
}

func TestValidateVerifyFailureProofFollowupProbeRefsRequiresExactChangedPathRef(t *testing.T) {
	ctx := verifyFailureProbeRefTestContext()
	probes := []types.VerificationProbe{{
		ID:           "unicode",
		Language:     "java",
		ContractRefs: []string{"unicode-output"},
	}}
	got := validateVerifyFailureProofFollowupProbeRefs(ctx, probes)
	if !strings.Contains(got, `changed_symbol_refs=["path:src/main/java/org/example/RandomStringUtils.java"]`) {
		t.Fatalf("missing exact-ref repair: %s", got)
	}
}

func TestValidateVerifyFailureProofFollowupProbeRefsRejectsCrossLanguageSourceCheck(t *testing.T) {
	ctx := verifyFailureProbeRefTestContext()
	probes := []types.VerificationProbe{{
		ID:                "unicode-source-regex",
		Language:          "python",
		ChangedSymbolRefs: []string{"path:src/main/java/org/example/RandomStringUtils.java"},
		ContractRefs:      []string{"unicode-output"},
	}}
	got := validateVerifyFailureProofFollowupProbeRefs(ctx, probes)
	if !strings.Contains(got, "cross-language/static source check cannot sign target execution or behavior") ||
		!strings.Contains(got, "java") {
		t.Fatalf("cross-language capability repair missing: %s", got)
	}
}

func TestValidateVerifyFailureProofFollowupProbeRefsRequiresBehaviorContractBinding(t *testing.T) {
	ctx := verifyFailureProbeRefTestContext()
	probes := []types.VerificationProbe{{
		ID:                "unicode-java",
		Language:          "java",
		ChangedSymbolRefs: []string{"path:src/main/java/org/example/RandomStringUtils.java"},
	}}
	got := validateVerifyFailureProofFollowupProbeRefs(ctx, probes)
	if !strings.Contains(got, "must bind at least one required behavior contract") ||
		!strings.Contains(got, "unicode-output") {
		t.Fatalf("contract binding repair missing: %s", got)
	}
}

func TestValidateVerifyFailureProofFollowupProbeRefsAcceptsTargetLanguageExactBinding(t *testing.T) {
	ctx := verifyFailureProbeRefTestContext()
	probes := []types.VerificationProbe{{
		ID:                "unicode-java",
		Language:          "java",
		ChangedSymbolRefs: []string{"path:src/main/java/org/example/RandomStringUtils.java"},
		ContractRefs:      []string{"unicode-output"},
	}}
	if got := validateVerifyFailureProofFollowupProbeRefs(ctx, probes); got != "" {
		t.Fatalf("precisely bound target-language proof followup rejected: %s", got)
	}
}

func TestValidateVerifyFailureProofFollowupProbeRefsInactiveWithoutTypedHandoff(t *testing.T) {
	ctx := verifyFailureProbeRefTestContext()
	ctx.Mutable.ResetVerifyFailureHandoff()
	if got := validateVerifyFailureProofFollowupProbeRefs(ctx, nil); got != "" {
		t.Fatalf("ordinary plan must not inherit proof-followup requirements: %s", got)
	}
}

func TestValidateVerifyFailureProofFollowupProbeRefsDoesNotRequireUnavailableInlineRuntime(t *testing.T) {
	paths := []string{
		"src/lib.rs",
		"src/native.cpp",
		"src/Main.kt",
		"entry/src/main/ets/pages/Index.ets",
		"src/main.cj",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			ctx := verifyFailureProbeRefTestContextForPath(path)
			if got := validateVerifyFailureProofFollowupProbeRefs(ctx, nil); got != "" {
				t.Fatalf("language without an inline runtime inherited an impossible probe contract: %s", got)
			}
		})
	}
}

func TestVerificationProbeRuntimeSupportsTargetFamiliesMatchesExecutableMatrix(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"main.py", true},
		{"main.js", true},
		{"Main.java", true},
		{"main.rb", true},
		{"main.ts", true},
		{"Main.kt", false},
		{"main.rs", false},
		{"main.c", false},
		{"main.cpp", false},
		{"Index.ets", false},
		{"main.cj", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			families := sourceVerificationLanguageFamilies(types.VerificationLanguageFamiliesFromPath(tt.path))
			if got := verificationProbeRuntimeSupportsTargetFamilies(families); got != tt.want {
				t.Fatalf("supported=%v, want %v for families %v", got, tt.want, families)
			}
		})
	}
}

func TestValidatePlanFullContentHooksTypedProofFollowupBeforeApply(t *testing.T) {
	ctx := verifyFailureProbeRefTestContext()
	ctx.RepoRoot = t.TempDir()
	probes := []types.VerificationProbe{{
		ID:                "unicode-source-regex",
		Language:          "python",
		ChangedSymbolRefs: []string{"path:src/main/java/org/example/RandomStringUtils.java"},
		ContractRefs:      []string{"unicode-output"},
	}}
	rej, pack := validatePlanFullContentWithRepair(ctx, "emit_change_plan", "verification proof followup", nil, probes)
	if !strings.Contains(rej, "cross-language/static source check cannot sign target execution or behavior") {
		t.Fatalf("shipping validation hook did not reject weak cross-language proof: %s", rej)
	}
	if pack == nil || pack.ReasonCode != "verification_probe_proof_followup_refs_failed" {
		t.Fatalf("repair pack=%+v, want verification_probe_proof_followup_refs_failed", pack)
	}
}
