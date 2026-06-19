package safety

import (
	"encoding/json"
	"testing"
)

func TestDecideEffectPermissionPlannerAllowsOnlyTypedDryRunProbe(t *testing.T) {
	allowed := DecideEffectPermission(EffectDescriptor{
		Role:   EffectRolePlanner,
		Kind:   EffectKindDryRunTest,
		DryRun: true,
	})
	if allowed.Action != PermissionAllow {
		t.Fatalf("planner dry-run probe should be allowed, got %+v", allowed)
	}
	notDryRun := DecideEffectPermission(EffectDescriptor{
		Role: EffectRolePlanner,
		Kind: EffectKindDryRunTest,
	})
	if notDryRun.Action != PermissionDeny || notDryRun.ReasonCode != "planner_probe_requires_dry_run" {
		t.Fatalf("planner non-dry-run probe should be denied, got %+v", notDryRun)
	}
	shell := DecideEffectPermission(EffectDescriptor{
		Role: EffectRolePlanner,
		Kind: EffectKindCommand,
	})
	if shell.Action != PermissionDeny || shell.ReasonCode != "role_effect_denied" {
		t.Fatalf("planner shell effect should be denied, got %+v", shell)
	}
}

func TestDecideEffectPermissionVerifierOnlyTypedVerifySurface(t *testing.T) {
	tests := DecideEffectPermission(EffectDescriptor{Role: EffectRoleVerifier, Kind: EffectKindTestSuite})
	if tests.Action != PermissionAllow {
		t.Fatalf("verifier typed test suite should be allowed, got %+v", tests)
	}
	edit := DecideEffectPermission(EffectDescriptor{Role: EffectRoleVerifier, Kind: EffectKindEdit, Paths: []string{"pkg/a.go"}})
	if edit.Action != PermissionDeny {
		t.Fatalf("verifier edit effect should be denied, got %+v", edit)
	}
	command := DecideEffectPermission(EffectDescriptor{Role: EffectRoleVerifier, Kind: EffectKindCommand})
	if command.Action != PermissionDeny {
		t.Fatalf("verifier command effect should be denied, got %+v", command)
	}
}

func TestRunTestsEffectDescriptorVerifierDefaultOnly(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"nil":       nil,
		"empty":     json.RawMessage(``),
		"null":      json.RawMessage(`null`),
		"empty_obj": json.RawMessage(`{}`),
	} {
		t.Run(name, func(t *testing.T) {
			effect := RunTestsEffectDescriptor(EffectRoleVerifier, raw)
			if effect.Kind != EffectKindTestSuite || effect.TestMode != EffectTestModeDefault {
				t.Fatalf("default verifier effect mismatch: %+v", effect)
			}
			if decision := DecideEffectPermission(effect); decision.Action != PermissionAllow {
				t.Fatalf("default verifier run_tests should allow, got %+v", decision)
			}
		})
	}
	for name, raw := range map[string]json.RawMessage{
		"explicit_runner": json.RawMessage(`{"runner":"python"}`),
		"dry_run_probe":   json.RawMessage(`{"dry_run":true,"verification_probe":{"code":"assert True"}}`),
		"invalid_params":  json.RawMessage(`{"runner":`),
	} {
		t.Run(name, func(t *testing.T) {
			decision := DecideEffectPermission(RunTestsEffectDescriptor(EffectRoleVerifier, raw))
			if decision.Action != PermissionDeny || decision.ReasonCode != "verifier_run_tests_default_required" {
				t.Fatalf("non-default verifier run_tests should deny with default-required reason, got %+v", decision)
			}
		})
	}
}

func TestRunTestsEffectDescriptorPlannerDryRunProbeOnly(t *testing.T) {
	allowed := DecideEffectPermission(RunTestsEffectDescriptor(
		EffectRolePlanner,
		json.RawMessage(`{"dry_run":true,"verification_probe":{"code":"assert True"}}`),
	))
	if allowed.Action != PermissionAllow {
		t.Fatalf("planner typed dry-run probe should allow, got %+v", allowed)
	}
	missingProbe := DecideEffectPermission(RunTestsEffectDescriptor(
		EffectRolePlanner,
		json.RawMessage(`{"dry_run":true}`),
	))
	if missingProbe.Action != PermissionDeny || missingProbe.ReasonCode != "planner_probe_requires_verification_probe" {
		t.Fatalf("planner dry-run without probe should deny with probe reason, got %+v", missingProbe)
	}
	suite := DecideEffectPermission(RunTestsEffectDescriptor(
		EffectRolePlanner,
		json.RawMessage(`{"runner":"python"}`),
	))
	if suite.Action != PermissionDeny {
		t.Fatalf("planner explicit suite execution should deny, got %+v", suite)
	}
}

func TestDecideEffectPermissionWorkerRolesAreEvidenceOnly(t *testing.T) {
	for _, role := range []EffectRole{EffectRoleLocalizer, EffectRoleImpactAnalyzer, EffectRolePatchCritic, EffectRoleProofAuditor, EffectRoleFailureAnalyzer} {
		t.Run(string(role), func(t *testing.T) {
			read := DecideEffectPermission(EffectDescriptor{Role: role, Kind: EffectKindRepoMap})
			if read.Action != PermissionAllow {
				t.Fatalf("%s should allow repo_map evidence, got %+v", role, read)
			}
			write := DecideEffectPermission(EffectDescriptor{Role: role, Kind: EffectKindApplyPatch, Paths: []string{"pkg/a.py"}})
			if write.Action != PermissionDeny {
				t.Fatalf("%s must not mutate, got %+v", role, write)
			}
		})
	}
}

func TestDecideEffectPermissionExternalDirectoryAndGitMetadataAreHardGates(t *testing.T) {
	externalRead := DecideEffectPermission(EffectDescriptor{Role: EffectRoleLocalizer, Kind: EffectKindReadRepo, ExternalDirectory: true})
	if externalRead.Action != PermissionAsk || externalRead.ReasonCode != "external_directory_effect" {
		t.Fatalf("external read should ask, got %+v", externalRead)
	}
	externalWrite := DecideEffectPermission(EffectDescriptor{Role: EffectRoleCoder, Kind: EffectKindEdit, ExternalDirectory: true, Paths: []string{"../outside.py"}})
	if externalWrite.Action != PermissionDeny || externalWrite.ReasonCode != "external_directory_write" {
		t.Fatalf("external write should deny, got %+v", externalWrite)
	}
	externalPathWrite := DecideEffectPermission(EffectDescriptor{Role: EffectRoleCoder, Kind: EffectKindEdit, Paths: []string{"../outside.py"}})
	if externalPathWrite.Action != PermissionDeny || externalPathWrite.ReasonCode != "external_directory_write" {
		t.Fatalf("external path should be detected and denied, got %+v", externalPathWrite)
	}
	gitMeta := DecideEffectPermission(EffectDescriptor{Role: EffectRoleCoder, Kind: EffectKindEdit, Paths: []string{".git/config"}})
	if gitMeta.Action != PermissionDeny || gitMeta.ReasonCode != "git_metadata_effect" {
		t.Fatalf("git metadata should deny, got %+v", gitMeta)
	}
	mainRepo := DecideEffectPermission(EffectDescriptor{Role: EffectRoleCoder, Kind: EffectKindEdit, MutatesMainRepo: true})
	if mainRepo.Action != PermissionDeny || mainRepo.ReasonCode != "mutates_main_repo" {
		t.Fatalf("main repo mutation should deny, got %+v", mainRepo)
	}
}

func TestDecideEffectPermissionDoomLoopAsksReadAndDeniesMutation(t *testing.T) {
	read := DecideEffectPermission(EffectDescriptor{
		Role:        EffectRoleLocalizer,
		Kind:        EffectKindRepoMap,
		RepeatCount: 4,
		RepeatLimit: 3,
	})
	if read.Action != PermissionAsk || read.ReasonCode != "effect_doom_loop" {
		t.Fatalf("read doom loop should ask, got %+v", read)
	}
	write := DecideEffectPermission(EffectDescriptor{
		Role:        EffectRoleCoder,
		Kind:        EffectKindEdit,
		RepeatCount: 4,
		RepeatLimit: 3,
		Paths:       []string{"pkg/a.py"},
	})
	if write.Action != PermissionDeny || write.ReasonCode != "mutating_doom_loop" {
		t.Fatalf("mutating doom loop should deny, got %+v", write)
	}
}

func TestEffectFingerprintNormalizesPathOrder(t *testing.T) {
	a := EffectFingerprint(EffectDescriptor{
		Role:  EffectRoleCoder,
		Kind:  EffectKindEdit,
		Paths: []string{"b.go", "./a.go", "a.go"},
	})
	b := EffectFingerprint(EffectDescriptor{
		Role:  EffectRoleCoder,
		Kind:  EffectKindEdit,
		Paths: []string{"a.go", "b.go"},
	})
	if a == "" || a != b {
		t.Fatalf("fingerprint should be stable after path normalization, got %q / %q", a, b)
	}
}

func TestEffectDescriptorForToolMapsKnownToolSurface(t *testing.T) {
	got := EffectDescriptorForTool(EffectRolePlanner, "exec_command")
	if got.Role != EffectRolePlanner || got.Kind != EffectKindCommand {
		t.Fatalf("exec_command descriptor mismatch: %+v", got)
	}
	decision := DecideEffectPermission(got)
	if decision.Action != PermissionDeny {
		t.Fatalf("mapped planner exec_command should be denied, got %+v", decision)
	}
}
