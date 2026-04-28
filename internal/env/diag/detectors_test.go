package diag

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDiagnose_RunnerMissing covers the runner_missing branch
// across the 12-runner catalog.
func TestDiagnose_RunnerMissing(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		runner string
		wantOK bool
	}{
		{"pytest exit_127", "/bin/sh: pytest: not found\n", "python", true},
		{"npm not found", "bash: npm: command not found\n", "node", true},
		{"cargo not found", "zsh: command not found: cargo\n", "rust", true},
		{"python module pytest", "/usr/bin/python3: No module named pytest", "python", true},
		{"python module-style err", "/path/to/python: No module named 'pytest'", "python", true},
		{"unknown runner is rejected", "go: command not found\n", "weird", false},
		{"clean stderr is rejected", "tests passed\n", "python", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Diagnose(c.stderr, c.runner, nil)
			isMissing := d.Kind == types.DiagRunnerMissing
			if isMissing != c.wantOK {
				t.Errorf("got Kind=%q want runner_missing=%t", d.Kind, c.wantOK)
			}
		})
	}
}

// TestDiagnose_GitState covers the three git state diagnoses.
func TestDiagnose_GitState(t *testing.T) {
	cases := []struct {
		state string
		want  types.DiagKind
	}{
		{"git_missing", types.DiagGitNotInstalled},
		{"not_initialized", types.DiagGitRepoNotInitialized},
		{"no_commits", types.DiagGitRepoNoCommits},
		{"ready", ""},
	}
	for _, c := range cases {
		env := &types.EnvFacts{GitRepoState: c.state}
		d := Diagnose("", "", env)
		if c.want == "" {
			if d.Kind != types.DiagUnknown {
				t.Errorf("state=%s expected Unknown, got %q", c.state, d.Kind)
			}
		} else if d.Kind != c.want {
			t.Errorf("state=%s expected %q, got %q", c.state, c.want, d.Kind)
		}
	}
}

// TestDiagnose_SystemLibMissing covers the canonical linker /
// header / runtime patterns.
func TestDiagnose_SystemLibMissing(t *testing.T) {
	cases := []struct {
		stderr string
		wantOK bool
		tool   string
	}{
		{"ld: library not found for -lssl\n", true, "ssl"},
		{"/usr/bin/ld: cannot find -lcrypto: No such file or directory\n", true, "crypto"},
		{"fatal error: openssl/ssl.h: No such file or directory\n", true, "openssl/ssl.h"},
		{"error while loading shared libraries: libGL.so.1\n", true, "libgl.so.1"},
		{"random unrelated error\n", false, ""},
	}
	for _, c := range cases {
		d := Diagnose(c.stderr, "make", nil)
		ok := d.Kind == types.DiagSystemLibMissing
		if ok != c.wantOK {
			t.Errorf("stderr=%q got Kind=%q want system_lib_missing=%t", c.stderr, d.Kind, c.wantOK)
		}
		if c.wantOK && d.Tool != c.tool {
			t.Errorf("stderr=%q got Tool=%q want %q", c.stderr, d.Tool, c.tool)
		}
	}
}

// TestDiagnose_ConfigMissing fires when the runner is set but no
// matching manifest is present in EnvFacts. Provide Pythons so
// toolchain_missing (which fires earlier in the chain) doesn't
// short-circuit.
func TestDiagnose_ConfigMissing(t *testing.T) {
	env := &types.EnvFacts{
		ProjectFiles: map[string]string{},
		Pythons:      []types.PythonInterp{{Path: "/usr/bin/python3"}},
	}
	d := Diagnose("", "python", env)
	if d.Kind != types.DiagConfigMissing {
		t.Errorf("no manifest should diag config_missing; got %q", d.Kind)
	}
	envWithFile := &types.EnvFacts{
		ProjectFiles: map[string]string{"pyproject.toml": "/abs"},
		Pythons:      []types.PythonInterp{{Path: "/usr/bin/python3"}},
	}
	d = Diagnose("", "python", envWithFile)
	if d.Kind == types.DiagConfigMissing {
		t.Errorf("manifest present should not diag config_missing; got %q", d.Kind)
	}
}

// TestDiagnose_ToolchainMissing fires when the language's runtime
// itself is absent from the env.
func TestDiagnose_ToolchainMissing(t *testing.T) {
	env := &types.EnvFacts{
		ProjectFiles: map[string]string{"pyproject.toml": "/abs"},
		// Pythons empty
	}
	// Use a python-specific runner_missing pattern so detectRunnerMissing
	// would have fired — but provide an env with no Pythons so toolchain
	// fires first… actually toolchain fires after runner_missing in the
	// detector chain. Use clean stderr to bypass runner_missing.
	d := Diagnose("", "python", env)
	if d.Kind != types.DiagBuildToolchainMissing {
		t.Errorf("no python should diag toolchain_missing; got %q", d.Kind)
	}
}

// TestDiagnose_Unknown fires when nothing matches — caller should
// treat this as "route to LLM fallback".
func TestDiagnose_Unknown(t *testing.T) {
	env := &types.EnvFacts{
		Pythons:      []types.PythonInterp{{Path: "/x", HasPytest: true}},
		ProjectFiles: map[string]string{"pyproject.toml": "/abs"},
		GitRepoState: "ready",
	}
	d := Diagnose("totally unrecognised error", "python", env)
	if d.Kind != types.DiagUnknown {
		t.Errorf("expected Unknown sentinel; got %q", d.Kind)
	}
}
