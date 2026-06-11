package cmd

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/types"
)

func boolPtr(b bool) *bool { return &b }

func TestResolveUserMode_DefaultAutoReadPhase(t *testing.T) {
	mode, phase, err := resolveUserModeAndWritePhase(modeResolutionInputs{})
	if err != nil {
		t.Fatalf("default path should not error: %v", err)
	}
	if mode != repl.UserModeAuto {
		t.Fatalf("user mode = %q, want %q", mode, repl.UserModeAuto)
	}
	if phase != types.ModeRead {
		t.Fatalf("phase = %q, want %q", phase, types.ModeRead)
	}
}

func TestResolveUserMode_ExplicitReadOnlyLanesDoNotNeedWriteEnabled(t *testing.T) {
	for _, want := range []repl.UserMode{repl.UserModeCode, repl.UserModeOperation, repl.UserModeData} {
		mode, phase, err := resolveUserModeAndWritePhase(modeResolutionInputs{
			YamlEnabled: boolPtr(false),
			CLIFlagMode: string(want),
		})
		if err != nil {
			t.Fatalf("%s should not require write_enabled: %v", want, err)
		}
		if mode != want || phase != types.ModeRead {
			t.Fatalf("%s resolved to mode=%q phase=%q", want, mode, phase)
		}
	}
}

func TestResolveUserMode_WriteDefaultsToPlan(t *testing.T) {
	mode, phase, err := resolveUserModeAndWritePhase(modeResolutionInputs{
		YamlEnabled: boolPtr(true),
		CLIFlagMode: "write",
	})
	if err != nil {
		t.Fatalf("write default should not error: %v", err)
	}
	if mode != repl.UserModeWrite || phase != types.ModePlan {
		t.Fatalf("resolved mode=%q phase=%q, want write/plan", mode, phase)
	}
}

func TestResolveUserMode_WriteDisabledRejected(t *testing.T) {
	_, _, err := resolveUserModeAndWritePhase(modeResolutionInputs{
		YamlEnabled: boolPtr(false),
		CLIFlagMode: "write",
	})
	if err == nil {
		t.Fatal("write mode without write_enabled=true must error")
	}
	if !strings.Contains(err.Error(), "write mode disabled") {
		t.Fatalf("error should mention write mode disabled, got: %v", err)
	}
}

func TestResolveUserMode_WritePhaseApplyRequiresAutoApplyForSingleShot(t *testing.T) {
	_, _, err := resolveUserModeAndWritePhase(modeResolutionInputs{
		YamlEnabled:   boolPtr(true),
		CLIFlagMode:   "write",
		CLIWritePhase: "apply",
		HasRequest:    true,
		AutoApply:     false,
		PlanFile:      "/tmp/plan.json",
	})
	if err == nil || !strings.Contains(err.Error(), "--auto-apply") {
		t.Fatalf("apply without auto-apply should mention --auto-apply, got: %v", err)
	}

	mode, phase, err := resolveUserModeAndWritePhase(modeResolutionInputs{
		YamlEnabled:   boolPtr(true),
		CLIFlagMode:   "write",
		CLIWritePhase: "apply",
		HasRequest:    true,
		AutoApply:     true,
	})
	if err != nil {
		t.Fatalf("controller-first apply without plan-file should not error: %v", err)
	}
	if mode != repl.UserModeWrite || phase != types.ModeApply {
		t.Fatalf("resolved mode=%q phase=%q, want write/apply", mode, phase)
	}

	mode, phase, err = resolveUserModeAndWritePhase(modeResolutionInputs{
		YamlEnabled:   boolPtr(true),
		CLIFlagMode:   "write",
		CLIWritePhase: "apply",
		HasRequest:    true,
		AutoApply:     true,
		PlanFile:      "/tmp/plan.json",
	})
	if err != nil {
		t.Fatalf("valid apply should not error: %v", err)
	}
	if mode != repl.UserModeWrite || phase != types.ModeApply {
		t.Fatalf("resolved mode=%q phase=%q, want write/apply", mode, phase)
	}
}

func TestResolveUserMode_WritePhaseVerifyAllowsActiveWorkflow(t *testing.T) {
	mode, phase, err := resolveUserModeAndWritePhase(modeResolutionInputs{
		YamlEnabled:   boolPtr(true),
		CLIFlagMode:   "write",
		CLIWritePhase: "verify",
	})
	if err != nil {
		t.Fatalf("controller-first verify without plan-file should not error: %v", err)
	}
	if mode != repl.UserModeWrite || phase != types.ModeVerify {
		t.Fatalf("resolved mode=%q phase=%q, want write/verify", mode, phase)
	}
}

func TestResolveUserMode_WritePhaseOnlyValidWithWriteMode(t *testing.T) {
	_, _, err := resolveUserModeAndWritePhase(modeResolutionInputs{
		CLIFlagMode:         "data",
		CLIWritePhase:       "plan",
		CLIWritePhasePassed: true,
	})
	if err == nil || !strings.Contains(err.Error(), "--write-phase") {
		t.Fatalf("write-phase outside write mode should error, got: %v", err)
	}
}

func TestResolveUserMode_PlanFileOnlyValidWithWriteMode(t *testing.T) {
	_, _, err := resolveUserModeAndWritePhase(modeResolutionInputs{
		CLIFlagMode: "operation",
		PlanFile:    "/tmp/plan.json",
	})
	if err == nil || !strings.Contains(err.Error(), "--plan-file") {
		t.Fatalf("plan-file outside write mode should error, got: %v", err)
	}
}

func TestResolveUserMode_DataResumeOnlyValidWithDataMode(t *testing.T) {
	_, _, err := resolveUserModeAndWritePhase(modeResolutionInputs{
		CLIFlagMode: "auto",
		DataResume:  "/tmp/checkpoint.json",
	})
	if err == nil || !strings.Contains(err.Error(), "--data-resume") {
		t.Fatalf("data-resume outside data mode should error, got: %v", err)
	}

	mode, phase, err := resolveUserModeAndWritePhase(modeResolutionInputs{
		CLIFlagMode: "data",
		DataResume:  "/tmp/checkpoint.json",
	})
	if err != nil {
		t.Fatalf("data-resume with data mode should not error: %v", err)
	}
	if mode != repl.UserModeData || phase != types.ModeRead {
		t.Fatalf("resolved mode=%q phase=%q, want data/read", mode, phase)
	}
}

func TestResolveUserMode_UnknownRejected(t *testing.T) {
	_, _, err := resolveUserModeAndWritePhase(modeResolutionInputs{CLIFlagMode: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "unknown mode") {
		t.Fatalf("unknown mode should error, got: %v", err)
	}
}

// write_enabled semantics: absent (nil) defaults to ENABLED — the
// per-invocation --mode=write flag is the consent; an explicit false
// is the organisational kill switch and refuses with the config hint.
func TestResolveUserMode_WriteEnabledDefaultsOn(t *testing.T) {
	mode, phase, err := resolveUserModeAndWritePhase(modeResolutionInputs{
		YamlEnabled: nil,
		CLIFlagMode: "write",
	})
	if err != nil {
		t.Fatalf("absent write_enabled must default to enabled: %v", err)
	}
	if mode != repl.UserModeWrite || phase != types.ModePlan {
		t.Fatalf("mode=%q phase=%q, want write/plan", mode, phase)
	}
}

func TestResolveUserMode_ExplicitFalseIsKillSwitch(t *testing.T) {
	_, _, err := resolveUserModeAndWritePhase(modeResolutionInputs{
		YamlEnabled: boolPtr(false),
		CLIFlagMode: "write",
	})
	if err == nil || !strings.Contains(err.Error(), "kill switch") {
		t.Fatalf("explicit false must refuse with the kill-switch hint, got %v", err)
	}
}
