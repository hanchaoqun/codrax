package types

import "testing"

func TestNormalizeReadDispatchPolicyCanonicalizesToolsAndScopes(t *testing.T) {
	got := NormalizeReadDispatchPolicy(ReadDispatchPolicy{
		Active:                    true,
		Action:                    " add_proof ",
		ReasonCode:                " proof_weak ",
		RouteSurface:              " verification ",
		AllowedTools:              []string{" read ", "read_file", "RepoMap"},
		DeniedTools:               []string{" ls ", "exec_command"},
		PreferredTools:            []string{" run_tests ", "read"},
		UnavailablePreferredTools: []string{" RUN_TESTS "},
		ScopePaths:                []string{" ./pkg/a.py ", `pkg\b.py`, "pkg/a.py"},
		MaxToolCalls:              -3,
		OneShot:                   true,
	})
	if !got.Active || got.Action != ReadDispatchPolicyActionAddProof ||
		got.ReasonCode != "proof_weak" || got.RouteSurface != ReadDispatchPolicySurfaceVerify ||
		got.MaxToolCalls != 0 || !got.OneShot {
		t.Fatalf("unexpected normalized policy header: %+v", got)
	}
	if want := []string{"read_file", "repo_map"}; !sameStringSliceForPolicy(got.AllowedTools, want) {
		t.Fatalf("allowed tools = %v, want %v", got.AllowedTools, want)
	}
	if want := []string{"list_files", "exec_command"}; !sameStringSliceForPolicy(got.DeniedTools, want) {
		t.Fatalf("denied tools = %v, want %v", got.DeniedTools, want)
	}
	if want := []string{"run_tests", "read_file"}; !sameStringSliceForPolicy(got.PreferredTools, want) {
		t.Fatalf("preferred tools = %v, want %v", got.PreferredTools, want)
	}
	if want := []string{"run_tests"}; !sameStringSliceForPolicy(got.UnavailablePreferredTools, want) {
		t.Fatalf("unavailable preferred tools = %v, want %v", got.UnavailablePreferredTools, want)
	}
	if want := []string{"pkg/a.py", "pkg/b.py"}; !sameStringSliceForPolicy(got.ScopePaths, want) {
		t.Fatalf("scope paths = %v, want %v", got.ScopePaths, want)
	}
}

func TestReadDispatchPolicyAllowsTool(t *testing.T) {
	p := NormalizeReadDispatchPolicy(ReadDispatchPolicy{
		Active:       true,
		Action:       ReadDispatchPolicyActionAddProof,
		AllowedTools: []string{"read_file", "emit_evidence"},
		DeniedTools:  []string{"exec_command"},
	})
	if !p.AllowsTool("read") {
		t.Fatal("read alias should be allowed as read_file")
	}
	if p.AllowsTool("grep") {
		t.Fatal("grep should be blocked when allowed set is non-empty")
	}
	if p.AllowsTool("exec_command") {
		t.Fatal("explicit deny should win")
	}
	if !NormalizeReadDispatchPolicy(ReadDispatchPolicy{}).AllowsTool("exec_command") {
		t.Fatal("inactive policy must not block tools")
	}
}

func sameStringSliceForPolicy(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
