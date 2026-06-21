package types

import (
	"encoding/json"
	"testing"
)

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

func TestValidateReadDispatchPolicyPathScope(t *testing.T) {
	policy := NormalizeReadDispatchPolicy(ReadDispatchPolicy{
		Active:     true,
		Action:     ReadDispatchPolicyActionAddProof,
		ScopePaths: []string{"pkg/a.py", "internal/agent"},
	})
	cases := []struct {
		name       string
		tool       string
		params     string
		wantAllow  bool
		wantReason string
	}{
		{
			name:      "read file exact scope",
			tool:      "read_file",
			params:    `{"path":"pkg/a.py"}`,
			wantAllow: true,
		},
		{
			name:      "read file under directory scope",
			tool:      "read_file",
			params:    `{"path":"internal/agent/agent.go"}`,
			wantAllow: true,
		},
		{
			name:       "read file outside scope",
			tool:       "read_file",
			params:     `{"path":"pkg/b.py"}`,
			wantAllow:  false,
			wantReason: ReadDispatchPathScopeOutside,
		},
		{
			name:       "grep omitted path is broad root",
			tool:       "grep",
			params:     `{"pattern":"x"}`,
			wantAllow:  false,
			wantReason: ReadDispatchPathScopeOutside,
		},
		{
			name:       "parent escape is malformed",
			tool:       "read_file",
			params:     `{"path":"../outside.py"}`,
			wantAllow:  false,
			wantReason: ReadDispatchPathScopeMalformed,
		},
		{
			name:      "blob ref is virtual not repo exploration",
			tool:      "read_file",
			params:    `{"path":"blob://tool-output/grep.txt"}`,
			wantAllow: true,
		},
		{
			name:      "repo map root with narrow scope",
			tool:      "repo_map",
			params:    `{"path":".","view":"source_inventory","scope":"internal/agent"}`,
			wantAllow: true,
		},
		{
			name:       "repo map root without narrow field is broad",
			tool:       "repo_map",
			params:     `{"path":".","view":"task_map","query":"agent"}`,
			wantAllow:  false,
			wantReason: ReadDispatchPathScopeOutside,
		},
		{
			name:      "repo map joins base path and scope",
			tool:      "repo_map",
			params:    `{"path":"internal","view":"source_inventory","scope":"agent"}`,
			wantAllow: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ValidateReadDispatchPolicyPathScope(policy, c.tool, json.RawMessage(c.params), "")
			if got.Allowed != c.wantAllow {
				t.Fatalf("Allowed=%v, want %v; decision=%+v", got.Allowed, c.wantAllow, got)
			}
			if c.wantReason != "" && got.ReasonCode != c.wantReason {
				t.Fatalf("ReasonCode=%q, want %q; decision=%+v", got.ReasonCode, c.wantReason, got)
			}
		})
	}
}

func TestValidateReadDispatchPolicyPathScopeAbsoluteRepoPath(t *testing.T) {
	policy := NormalizeReadDispatchPolicy(ReadDispatchPolicy{
		Active:     true,
		Action:     ReadDispatchPolicyActionAddProof,
		ScopePaths: []string{"pkg"},
	})
	got := ValidateReadDispatchPolicyPathScope(policy, "read_file", json.RawMessage(`{"path":"/repo/pkg/a.py"}`), "/repo")
	if !got.Allowed {
		t.Fatalf("absolute repo path should normalize under scope, got %+v", got)
	}
	got = ValidateReadDispatchPolicyPathScope(policy, "read_file", json.RawMessage(`{"path":"/other/pkg/a.py"}`), "/repo")
	if got.Allowed || got.ReasonCode != ReadDispatchPathScopeMalformed {
		t.Fatalf("absolute path outside repo should be malformed, got %+v", got)
	}
}

func TestValidateReadDispatchPolicyPathScopeRepoMapAbsoluteBase(t *testing.T) {
	policy := NormalizeReadDispatchPolicy(ReadDispatchPolicy{
		Active:     true,
		Action:     ReadDispatchPolicyActionAddProof,
		ScopePaths: []string{"internal/agent"},
	})
	got := ValidateReadDispatchPolicyPathScope(policy, "repo_map", json.RawMessage(`{"path":"/repo/internal","scope":"agent"}`), "/repo")
	if !got.Allowed {
		t.Fatalf("repo_map absolute base plus relative scope should normalize under proof scope, got %+v", got)
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
