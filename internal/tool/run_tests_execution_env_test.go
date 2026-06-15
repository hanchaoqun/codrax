package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunnerExecutionEnv_PythonPrependsWorktreeImportRoots(t *testing.T) {
	root := t.TempDir()
	mainRoot := filepath.Join(root, "main")
	worktree := filepath.Join(root, "wt")
	nested := filepath.Join(worktree, "pkg")
	dep := filepath.Join(root, "deps")
	if err := os.MkdirAll(filepath.Join(worktree, "src"), 0o755); err != nil {
		t.Fatalf("mkdir worktree/src: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(nested, "src"), 0o755); err != nil {
		t.Fatalf("mkdir nested/src: %v", err)
	}
	base := []string{
		"PATH=/bin",
		"PWD=" + mainRoot,
		"PYTHONPATH=" + strings.Join([]string{mainRoot, dep, worktree}, string(filepath.ListSeparator)),
	}

	env := runnerExecutionEnvForBase(base, "python", worktree, nested, mainRoot)
	if got := envValue(env, "PWD"); got != nested {
		t.Fatalf("PWD = %q, want runner root %q", got, nested)
	}
	parts := filepath.SplitList(envValue(env, "PYTHONPATH"))
	if len(parts) != 5 {
		t.Fatalf("PYTHONPATH entries = %#v, want worktree/src, worktree, nested/src, nested, dep", parts)
	}
	want := []string{filepath.Join(worktree, "src"), worktree, filepath.Join(nested, "src"), nested, dep}
	for i := range want {
		if parts[i] != want[i] {
			t.Fatalf("PYTHONPATH = %#v, want %#v", parts, want)
		}
	}
	for _, part := range parts {
		if cleanAbsPathKey(part) == cleanAbsPathKey(mainRoot) {
			t.Fatalf("PYTHONPATH should not preserve original repo root %q: %#v", mainRoot, parts)
		}
	}
}

func TestRunnerExecutionEnv_NonPythonSetsPWDOnly(t *testing.T) {
	root := t.TempDir()
	mainRoot := filepath.Join(root, "main")
	worktree := filepath.Join(root, "wt")
	base := []string{
		"PATH=/bin",
		"PWD=" + mainRoot,
		"PYTHONPATH=" + mainRoot,
	}

	env := runnerExecutionEnvForBase(base, "node", worktree, worktree, mainRoot)
	if got := envValue(env, "PWD"); got != worktree {
		t.Fatalf("PWD = %q, want runner root %q", got, worktree)
	}
	if got := envValue(env, "PYTHONPATH"); got != mainRoot {
		t.Fatalf("non-python PYTHONPATH = %q, want unchanged %q", got, mainRoot)
	}
}

func TestInferDjangoSuiteFromChangePlan_UsesUniqueTestFile(t *testing.T) {
	root := t.TempDir()
	writeRunTestsEnvFixture(t, root, "tests/auth_tests/test_validators.py")
	mu := types.NewMutableState("django-suite")
	mu.SetChangePlan(&types.ChangePlan{TargetPaths: []string{"django/contrib/auth/validators.py"}})
	ctx := &types.BusContext{Mutable: mu}

	if got := inferDjangoSuiteFromChangePlan(ctx, root); got != "auth_tests.test_validators" {
		t.Fatalf("suite = %q, want auth_tests.test_validators", got)
	}
}

func TestInferDjangoSuiteFromChangePlan_UsesUniqueDirectoryLabel(t *testing.T) {
	root := t.TempDir()
	writeRunTestsEnvFixture(t, root, "tests/responses/test_cookie.py")
	mu := types.NewMutableState("django-suite")
	mu.SetChangePlan(&types.ChangePlan{TargetPaths: []string{"django/http/response.py"}})
	ctx := &types.BusContext{Mutable: mu}

	if got := inferDjangoSuiteFromChangePlan(ctx, root); got != "responses" {
		t.Fatalf("suite = %q, want responses", got)
	}
}

func TestInferDjangoSuiteFromChangePlan_PrioritizesChangedTestPath(t *testing.T) {
	root := t.TempDir()
	writeRunTestsEnvFixture(t, root, "tests/responses/tests.py")
	writeRunTestsEnvFixture(t, root, "tests/template_tests/test_response.py")
	mu := types.NewMutableState("django-suite")
	mu.SetChangePlan(&types.ChangePlan{TargetPaths: []string{"django/http/response.py", "tests/responses/tests.py"}})
	ctx := &types.BusContext{Mutable: mu}

	if got := inferDjangoSuiteFromChangePlan(ctx, root); got != "responses.tests" {
		t.Fatalf("suite = %q, want responses.tests", got)
	}
}

func TestInferDjangoSuiteFromChangePlan_IgnoresUnrelatedExactFileMatch(t *testing.T) {
	root := t.TempDir()
	writeRunTestsEnvFixture(t, root, "tests/template_tests/test_response.py")
	writeRunTestsEnvFixture(t, root, "tests/responses/test_cookie.py")
	mu := types.NewMutableState("django-suite")
	mu.SetChangePlan(&types.ChangePlan{TargetPaths: []string{"django/http/response.py"}})
	ctx := &types.BusContext{Mutable: mu}

	if got := inferDjangoSuiteFromChangePlan(ctx, root); got != "responses" {
		t.Fatalf("suite = %q, want responses", got)
	}
}

func TestInferDjangoSuiteFromChangePlan_AmbiguousDirectoryLabelReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	writeRunTestsEnvFixture(t, root, "tests/responses/test_cookie.py")
	writeRunTestsEnvFixture(t, root, "tests/response_tests/test_cookie.py")
	mu := types.NewMutableState("django-suite")
	mu.SetChangePlan(&types.ChangePlan{TargetPaths: []string{"django/http/response.py"}})
	ctx := &types.BusContext{Mutable: mu}

	if got := inferDjangoSuiteFromChangePlan(ctx, root); got != "" {
		t.Fatalf("ambiguous suite = %q, want empty", got)
	}
}

func writeRunTestsEnvFixture(t *testing.T, root, rel string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("# test fixture\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
