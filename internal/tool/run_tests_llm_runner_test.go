package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveLLMRunnerChoice_HappyPath: a whitelisted runner +
// existing repo-relative dir resolves to a runnerPlan whose Root is
// the absolute joined path. Manifest is the sentinel "(LLM-selected)"
// so logs make the source explicit.
func TestResolveLLMRunnerChoice_HappyPath(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "backend"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cases := []struct {
		name       string
		runner     string
		workingDir string
		wantSubdir string
	}{
		{"python at root", "python", "", "."},
		{"python at root via dot", "python", ".", "."},
		{"go in backend subdir", "go", "backend", "backend"},
		{"go with trailing slash", "go", "backend/", "backend"},
		{"runner case-insensitive", "PyThOn", "", "."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan, rej := resolveLLMRunnerChoice(repo, c.runner, "", c.workingDir)
			if rej != "" {
				t.Fatalf("unexpected rejection: %s", rej)
			}
			if strings.ToLower(c.runner) != plan.Runner {
				t.Errorf("Runner = %q, want %q", plan.Runner, strings.ToLower(c.runner))
			}
			if plan.Manifest != "(LLM-selected)" {
				t.Errorf("Manifest = %q, want sentinel (LLM-selected)", plan.Manifest)
			}
			abs, _ := filepath.Abs(filepath.Join(repo, c.wantSubdir))
			if plan.Root != abs {
				t.Errorf("Root = %q, want %q", plan.Root, abs)
			}
		})
	}
}

// TestResolveLLMRunnerChoice_RejectsBadRunner: the whitelist is
// load-bearing — anything outside it is rejected with a helpful
// listing of the legal values.
func TestResolveLLMRunnerChoice_RejectsBadRunner(t *testing.T) {
	repo := t.TempDir()
	cases := []string{"pytest", "jest", "kotlin", "", "invalid"}
	for _, r := range cases {
		t.Run("runner="+r, func(t *testing.T) {
			_, rej := resolveLLMRunnerChoice(repo, r, "", "")
			if rej == "" {
				t.Fatalf("expected rejection for runner=%q", r)
			}
			if !strings.Contains(rej, "supported runners") {
				t.Errorf("rejection should list supported runners, got: %s", rej)
			}
		})
	}
}

// TestResolveLLMRunnerChoice_RejectsTraversal: working_dir cannot
// escape the repo via "..", an absolute path, or a relative path
// that resolves outside RepoRoot. Each of those is a security gap if
// allowed (LLM-decided shell exec on arbitrary host paths).
func TestResolveLLMRunnerChoice_RejectsTraversal(t *testing.T) {
	repo := t.TempDir()
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		t.Fatalf("abs repo: %v", err)
	}
	cases := []struct {
		name       string
		workingDir string
		expectMsg  string
	}{
		{"parent escape", "../../etc", "escapes the repository"},
		{"nested parent escape", "subdir/../../..", "escapes the repository"},
		{"absolute path", filepath.Join(absRepo, "outside"), "must be repo-relative"},
		{"absolute path dir", absRepo, "must be repo-relative"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, rej := resolveLLMRunnerChoice(repo, "go", "", c.workingDir)
			if rej == "" {
				t.Fatalf("expected rejection for working_dir=%q", c.workingDir)
			}
			if !strings.Contains(rej, c.expectMsg) {
				t.Errorf("rejection should contain %q, got: %s", c.expectMsg, rej)
			}
		})
	}
}

// TestResolveLLMRunnerChoice_RejectsMissingDir: a working_dir that
// doesn't exist (or is a file, not a dir) is rejected — preventing
// silently running tests in the wrong place.
func TestResolveLLMRunnerChoice_RejectsMissingDir(t *testing.T) {
	repo := t.TempDir()
	_, rej := resolveLLMRunnerChoice(repo, "go", "", "does-not-exist")
	if rej == "" {
		t.Fatalf("expected rejection for missing dir")
	}
	if !strings.Contains(rej, "does not exist") {
		t.Errorf("rejection should mention nonexistence, got: %s", rej)
	}
}

func TestResolveLLMRunnerChoice_PythonFramework(t *testing.T) {
	repo := t.TempDir()
	plan, rej := resolveLLMRunnerChoice(repo, "python", "unittest", "")
	if rej != "" {
		t.Fatalf("unexpected rejection: %s", rej)
	}
	if plan.Framework != pythonFrameworkUnittest {
		t.Fatalf("Framework = %q, want unittest", plan.Framework)
	}
	if _, rej := resolveLLMRunnerChoice(repo, "go", "unittest", ""); rej == "" {
		t.Fatal("non-python framework should be rejected")
	}
	if _, rej := resolveLLMRunnerChoice(repo, "python", "nose", ""); rej == "" {
		t.Fatal("unsupported python framework should be rejected")
	}
}

func TestResolveLLMRunnerChoice_DjangoRuntestsOverridesPytest(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "tests"), 0o755); err != nil {
		t.Fatalf("mkdir tests: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tests", "runtests.py"), []byte("print('django tests')\n"), 0o644); err != nil {
		t.Fatalf("write runtests.py: %v", err)
	}
	plan, rej := resolveLLMRunnerChoice(repo, "python", "pytest", "tests")
	if rej != "" {
		t.Fatalf("unexpected rejection: %s", rej)
	}
	if plan.Framework != pythonFrameworkDjango {
		t.Fatalf("Framework = %q, want django", plan.Framework)
	}
	if rel := runnerPlanRel(repo, plan); rel != "tests" {
		t.Fatalf("runner root = %q, want tests", rel)
	}
	cmd, extra := buildRunCommandForPlan(plan, "tests/auth_tests/test_validators.py::UsernameValidatorsTests", "")
	if extra != "" {
		t.Fatalf("django runner should parse stdout, extra=%q", extra)
	}
	if !strings.Contains(cmd, "runtests.py") || !strings.Contains(cmd, "auth_tests.test_validators.UsernameValidatorsTests") ||
		strings.Contains(cmd, "::") || strings.Contains(cmd, "tests/auth_tests") {
		t.Fatalf("django command should call runtests.py with suite: %s", cmd)
	}
}

func TestBuildRunCommandForPlan_NodeFileSuiteUsesPositionalSelector(t *testing.T) {
	repo := t.TempDir()
	plan := runnerPlan{Runner: "node", Root: repo}
	cmd, extra := buildRunCommandForPlan(plan, "src/widget.test.ts", "")
	if extra != "" {
		t.Fatalf("node runner should parse stdout, extra=%q", extra)
	}
	if !strings.Contains(cmd, "src/widget.test.ts") || strings.Contains(cmd, " -t ") {
		t.Fatalf("node file suite should be positional file selector, got %q", cmd)
	}

	nameCmd, _ := buildRunCommandForPlan(plan, "renders empty state", "")
	if !strings.Contains(nameCmd, " -t ") {
		t.Fatalf("node test-name suite should keep -t selector, got %q", nameCmd)
	}
}

// TestAllowedRunnerList_SortedAndComplete locks the runner whitelist
// against drift: every case in buildRunCommand should appear here,
// and the exposed list should be sorted (deterministic for prompt
// strings + rejection messages).
func TestAllowedRunnerList_SortedAndComplete(t *testing.T) {
	got := allowedRunnerList()
	want := []string{"cjpm", "cmake", "go", "hvigor", "java", "make", "meson", "node", "python", "ruby", "rust", "swift"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("idx %d: got %q want %q (full=%v)", i, got[i], want[i], got)
		}
	}
}
