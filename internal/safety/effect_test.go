package safety

import "testing"

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
