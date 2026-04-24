package cmd

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Pointer helpers for constructing writeModeInputs inline. Every
// yaml field is pointer-typed so nil means "field absent" and a
// pointer-to-false / pointer-to-empty means "explicitly set".
func boolPtr(b bool) *bool       { return &b }
func strPtr(s string) *string    { return &s }

// TestResolveWriteMode_DefaultIsEmpty covers the pre-B0 caller: no
// yaml, no CLI flag, no request. The resolved mode must be empty so
// the orchestrator's Normalize step coerces to ModeRead — the L1
// byte-identity red line depends on this empty-passthrough.
func TestResolveWriteMode_DefaultIsEmpty(t *testing.T) {
	m, err := resolveWriteMode(writeModeInputs{})
	if err != nil {
		t.Fatalf("default path should not error: %v", err)
	}
	if m != "" {
		t.Errorf("default mode should be empty, got %q", m)
	}
}

// TestResolveWriteMode_YamlDefaultPlan pins that a yaml
// write_default_mode of "plan" is respected when write_enabled=true
// is also set. Without write_enabled the L2 gate would reject.
func TestResolveWriteMode_YamlDefaultPlan(t *testing.T) {
	m, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:     boolPtr(true),
		YamlDefaultMode: strPtr("plan"),
	})
	if err != nil {
		t.Fatalf("yaml plan default should be legal: %v", err)
	}
	if m != types.ModePlan {
		t.Errorf("resolved mode: got %q, want %q", m, types.ModePlan)
	}
}

// TestResolveWriteMode_CLIOverridesYaml proves the precedence: CLI
// --mode=read wins even when yaml wants plan. The operator can
// always downgrade at runtime.
func TestResolveWriteMode_CLIOverridesYaml(t *testing.T) {
	m, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:     boolPtr(true),
		YamlDefaultMode: strPtr("plan"),
		CLIFlagPassed:   true,
		CLIFlagMode:     "read",
	})
	if err != nil {
		t.Fatalf("CLI override should be legal: %v", err)
	}
	if m != types.ModeRead {
		t.Errorf("CLI-overridden mode: got %q, want %q", m, types.ModeRead)
	}
}

// TestResolveWriteMode_CLIEmptyReverts pins that passing --mode=""
// explicitly reverts to the zero-value path regardless of yaml
// default. Rare but useful for scripts that build a partial
// command line dynamically.
func TestResolveWriteMode_CLIEmptyReverts(t *testing.T) {
	m, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:     boolPtr(true),
		YamlDefaultMode: strPtr("plan"),
		CLIFlagPassed:   true,
		CLIFlagMode:     "",
	})
	if err != nil {
		t.Fatalf("empty CLI override should not error: %v", err)
	}
	if m != "" {
		t.Errorf("explicit empty CLI should resolve to empty, got %q", m)
	}
}

// TestResolveWriteMode_SingleShotApplyRequiresAutoApply is
// session-33 T6. Locks the L4 red line: a single-shot invocation
// in apply mode must explicitly approve via --auto-apply. REPL
// users (HasRequest=false) skip this check because /approve
// provides interactive confirmation.
func TestResolveWriteMode_SingleShotApplyRequiresAutoApply(t *testing.T) {
	_, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:   boolPtr(true),
		CLIFlagPassed: true,
		CLIFlagMode:   "apply",
		HasRequest:    true,
		AutoApply:     false,
		PlanFile:      "/tmp/plan.json",
	})
	if err == nil {
		t.Fatal("single-shot apply without --auto-apply must error")
	}
	if !strings.Contains(err.Error(), "--auto-apply") {
		t.Errorf("error should mention --auto-apply, got: %v", err)
	}
}

// TestResolveWriteMode_REPLApplyNoAutoApply verifies that in REPL
// mode (HasRequest=false) the --auto-apply gate does NOT fire —
// the interactive /approve flow is the expected approval surface
// for REPL users.
func TestResolveWriteMode_REPLApplyNoAutoApply(t *testing.T) {
	m, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:   boolPtr(true),
		CLIFlagPassed: true,
		CLIFlagMode:   "apply",
		HasRequest:    false, // REPL mode
		AutoApply:     false,
		PlanFile:      "/tmp/plan.json",
	})
	if err != nil {
		t.Fatalf("REPL apply without --auto-apply should be legal (interactive /approve): %v", err)
	}
	if m != types.ModeApply {
		t.Errorf("resolved mode: got %q, want %q", m, types.ModeApply)
	}
}

// TestResolveWriteMode_WriteDisabledRejectsPlan is session-33 T8.
// Locks the L2 red line: codrax.yaml :: write_enabled defaults to
// false, and any write mode (plan / apply / verify) is rejected
// until an operator explicitly opts in.
func TestResolveWriteMode_WriteDisabledRejectsPlan(t *testing.T) {
	_, err := resolveWriteMode(writeModeInputs{
		// YamlEnabled nil → treated as false
		CLIFlagPassed: true,
		CLIFlagMode:   "plan",
	})
	if err == nil {
		t.Fatal("plan mode without write_enabled=true must error")
	}
	if !strings.Contains(err.Error(), "write mode disabled") {
		t.Errorf("error should mention 'write mode disabled', got: %v", err)
	}
}

// TestResolveWriteMode_WriteDisabledExplicitFalseSameResult —
// same as above but write_enabled=false explicitly rather than nil.
func TestResolveWriteMode_WriteDisabledExplicitFalseSameResult(t *testing.T) {
	_, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:   boolPtr(false),
		CLIFlagPassed: true,
		CLIFlagMode:   "apply",
		HasRequest:    true,
		AutoApply:     true,
		PlanFile:      "/tmp/plan.json",
	})
	if err == nil {
		t.Fatal("apply mode with write_enabled=false must error")
	}
	if !strings.Contains(err.Error(), "write mode disabled") {
		t.Errorf("error should mention 'write mode disabled', got: %v", err)
	}
}

// TestResolveWriteMode_WriteDisabledAllowsRead verifies the L2 gate
// only fires on WRITE modes — read mode always works, regardless
// of write_enabled. This is the escape hatch that keeps codrax
// usable under the safe-by-default posture.
func TestResolveWriteMode_WriteDisabledAllowsRead(t *testing.T) {
	m, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:   boolPtr(false),
		CLIFlagPassed: true,
		CLIFlagMode:   "read",
	})
	if err != nil {
		t.Fatalf("read mode should work with write_enabled=false: %v", err)
	}
	if m != types.ModeRead {
		t.Errorf("resolved mode: got %q, want %q", m, types.ModeRead)
	}
}

// TestResolveWriteMode_UnknownModeRejected verifies CLI-supplied
// garbage surfaces a clean error rather than falling through to
// the orchestrator's runtime default branch.
func TestResolveWriteMode_UnknownModeRejected(t *testing.T) {
	_, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:   boolPtr(true),
		CLIFlagPassed: true,
		CLIFlagMode:   "bogus",
	})
	if err == nil {
		t.Fatal("unknown mode must error")
	}
	if !strings.Contains(err.Error(), "unknown pipeline mode") {
		t.Errorf("error should mention 'unknown pipeline mode', got: %v", err)
	}
}

// TestResolveWriteMode_YamlApplyRejected pins that the yaml
// default cannot be 'apply' — apply is a per-run side-effecting
// mode that should not be silently the default of a shared config.
// The test value is explicitly 'apply'.
func TestResolveWriteMode_YamlApplyRejected(t *testing.T) {
	_, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:     boolPtr(true),
		YamlDefaultMode: strPtr("apply"),
	})
	if err == nil {
		t.Fatal("yaml write_default_mode=apply must be rejected")
	}
	if !strings.Contains(err.Error(), "write_default_mode") {
		t.Errorf("error should mention write_default_mode, got: %v", err)
	}
}

// TestResolveWriteMode_YamlVerifyRejected same guard for verify.
func TestResolveWriteMode_YamlVerifyRejected(t *testing.T) {
	_, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:     boolPtr(true),
		YamlDefaultMode: strPtr("verify"),
	})
	if err == nil {
		t.Fatal("yaml write_default_mode=verify must be rejected")
	}
	if !strings.Contains(err.Error(), "write_default_mode") {
		t.Errorf("error should mention write_default_mode, got: %v", err)
	}
}

// TestResolveWriteMode_YamlUnknownRejected — yaml invalid mode
// also rejected (not just empty-or-valid).
func TestResolveWriteMode_YamlUnknownRejected(t *testing.T) {
	_, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:     boolPtr(true),
		YamlDefaultMode: strPtr("garbage"),
	})
	if err == nil {
		t.Fatal("yaml invalid mode must be rejected")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention 'invalid', got: %v", err)
	}
}

// TestResolveWriteMode_ApplyNeedsPlanFile verifies --mode=apply
// (or verify) without --plan-file is rejected. Those modes
// consume an existing plan; they do not produce one.
func TestResolveWriteMode_ApplyNeedsPlanFile(t *testing.T) {
	_, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:   boolPtr(true),
		CLIFlagPassed: true,
		CLIFlagMode:   "apply",
		HasRequest:    true,
		AutoApply:     true,
		PlanFile:      "", // missing
	})
	if err == nil {
		t.Fatal("apply without --plan-file must error")
	}
	if !strings.Contains(err.Error(), "--plan-file") {
		t.Errorf("error should mention --plan-file, got: %v", err)
	}
}

// TestResolveWriteMode_VerifyNeedsPlanFile same for verify.
func TestResolveWriteMode_VerifyNeedsPlanFile(t *testing.T) {
	_, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:   boolPtr(true),
		CLIFlagPassed: true,
		CLIFlagMode:   "verify",
		PlanFile:      "",
	})
	if err == nil {
		t.Fatal("verify without --plan-file must error")
	}
}

// TestResolveWriteMode_PlanModeNoPlanFileNeeded verifies plan mode
// does NOT require --plan-file — plan mode is the producer, not
// consumer. This is the negative of the previous two tests.
func TestResolveWriteMode_PlanModeNoPlanFileNeeded(t *testing.T) {
	m, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:   boolPtr(true),
		CLIFlagPassed: true,
		CLIFlagMode:   "plan",
		// PlanFile empty, intentionally.
	})
	if err != nil {
		t.Fatalf("plan mode without --plan-file should be legal: %v", err)
	}
	if m != types.ModePlan {
		t.Errorf("resolved mode: got %q, want %q", m, types.ModePlan)
	}
}

// TestResolveWriteMode_WhitespaceHandling verifies that leading /
// trailing whitespace in mode strings does not silently flip
// semantic classification. Common in scripted invocations where
// bash var substitution leaves a trailing space.
func TestResolveWriteMode_WhitespaceHandling(t *testing.T) {
	m, err := resolveWriteMode(writeModeInputs{
		YamlEnabled:   boolPtr(true),
		CLIFlagPassed: true,
		CLIFlagMode:   "  plan  ",
	})
	if err != nil {
		t.Fatalf("whitespace-wrapped valid mode should parse: %v", err)
	}
	if m != types.ModePlan {
		t.Errorf("whitespace handling: got %q, want %q", m, types.ModePlan)
	}
}
