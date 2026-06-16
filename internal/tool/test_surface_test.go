package tool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func writeSurfaceFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func surfaceCandidate(t *testing.T, surface types.TestSurface, runner string) types.TestSurfaceCandidate {
	t.Helper()
	for _, c := range surface.Candidates {
		if c.Runner == runner {
			return c
		}
	}
	t.Fatalf("no %s candidate in surface: %+v", runner, surface.Candidates)
	return types.TestSurfaceCandidate{}
}

func fakePython3WithoutPytest(t *testing.T) {
	t.Helper()
	realPython, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available on PATH")
	}
	fakeBin := t.TempDir()
	fakePython := filepath.Join(fakeBin, "python3")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "-m" ] && [ "$2" = "pytest" ]; then
  echo "No module named pytest" >&2
  exit 1
fi
exec %q "$@"
`, realPython)
	if err := os.WriteFile(fakePython, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake python3: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// The generalized selection rule: a Makefile with a declared check target
// (real test work) must outrank a higher-priority manifest whose tree has no
// test work. This is the commons-lang / Zod shape without any fixture
// coupling: manifest priority alone must not pick the runner.
func TestBuildTestSurface_TestWorkOutranksManifestPriority(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "pom.xml", "<project/>\n")
	writeSurfaceFile(t, root, "Makefile", "check:\n\t@echo ok\n")
	writeSurfaceFile(t, root, "src/Main.java", "class Main {}\n")

	surface := BuildTestSurface(root, "")
	if len(surface.Candidates) < 2 {
		t.Fatalf("expected java + make candidates, got %+v", surface.Candidates)
	}
	makeCand := surfaceCandidate(t, surface, "make")
	javaCand := surfaceCandidate(t, surface, "java")
	if !makeCand.HasTestSignal {
		t.Fatalf("make candidate with declared check target must have test signal: %+v", makeCand)
	}
	if javaCand.HasTestSignal {
		t.Fatalf("java candidate without Test*.java sources must not have test signal: %+v", javaCand)
	}
	if surface.Candidates[0].Runner != "make" {
		t.Fatalf("selection must rank make first, got %+v", surface.Candidates)
	}
	if surface.SelectedID != makeCand.ID {
		t.Fatalf("SelectedID = %q, want %q", surface.SelectedID, makeCand.ID)
	}
	if makeCand.MakeTarget != "check" {
		t.Fatalf("make target = %q, want check", makeCand.MakeTarget)
	}
}

// Manifest priority stays the tiebreaker when both candidates carry test
// work, preserving the existing detection order for healthy repos.
func TestBuildTestSurface_PriorityTiebreakWhenBothHaveTests(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "pom.xml", "<project/>\n")
	writeSurfaceFile(t, root, "Makefile", "check:\n\t@echo ok\n")
	writeSurfaceFile(t, root, "src/MainTest.java", "class MainTest {}\n")

	surface := BuildTestSurface(root, "")
	if surface.Candidates[0].Runner != "java" {
		t.Fatalf("java (priority 12) should outrank make (18) when both have test work: %+v", surface.Candidates)
	}
}

func TestBuildTestSurface_MakeWithoutDeclaredTargetHasNoSignal(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "Makefile", "build:\n\t@echo building\n")

	surface := BuildTestSurface(root, "")
	makeCand := surfaceCandidate(t, surface, "make")
	if makeCand.HasTestSignal {
		t.Fatalf("Makefile without check/test/tests target must not carry test signal: %+v", makeCand)
	}
}

func TestBuildTestSurface_PythonFrameworkAndCommand(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "pytest.ini", "[pytest]\n")
	writeSurfaceFile(t, root, "test_sample.py", "def test_ok():\n    assert True\n")

	surface := BuildTestSurface(root, "")
	py := surfaceCandidate(t, surface, "python")
	if !py.HasTestSignal {
		t.Fatalf("python with pytest.ini + test file must have signal: %+v", py)
	}
	if py.Framework != "pytest" {
		t.Fatalf("framework = %q, want pytest", py.Framework)
	}
	if py.WorkingDir != "." {
		t.Fatalf("working dir = %q, want .", py.WorkingDir)
	}
	if strings.TrimSpace(py.Command) == "" {
		t.Fatal("candidate command preview must render")
	}
}

func TestBuildTestSurface_UnconfiguredCMakeHasNoSignal(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "CMakeLists.txt", "project(x)\n")

	surface := BuildTestSurface(root, "")
	cm := surfaceCandidate(t, surface, "cmake")
	if cm.HasTestSignal {
		t.Fatalf("cmake without configured build dir must not carry test signal: %+v", cm)
	}
}

func TestNextTestSurfaceEscalation_SkipsExecutedAndNoSignal(t *testing.T) {
	surface := types.NormalizeTestSurface(types.TestSurface{
		Candidates: []types.TestSurfaceCandidate{
			{ID: "java@.", Runner: "java", WorkingDir: ".", Priority: 12, HasTestSignal: false},
			{ID: "make@.", Runner: "make", WorkingDir: ".", Priority: 18, HasTestSignal: true},
			{ID: "go@sub", Runner: "go", WorkingDir: "sub", Priority: 1, HasTestSignal: true},
		},
	})
	executed := map[string]bool{testSurfaceCandidateKey("go", "", "sub"): true}
	next := nextTestSurfaceEscalation(surface, executed)
	if next == nil || next.Runner != "make" {
		t.Fatalf("escalation should pick make (unexecuted, has signal), got %+v", next)
	}
	executed[testSurfaceCandidateKey("make", "", ".")] = true
	if got := nextTestSurfaceEscalation(surface, executed); got != nil {
		t.Fatalf("no candidate should remain, got %+v", got)
	}
}

// End-to-end escalation: the model picks a runner with zero test work while
// the surface holds a runnable Makefile check target. The zero-test outcome
// must not stand alone — the make candidate runs and its verdict decides.
func TestRunTests_ZeroTestChoiceEscalatesToSurfaceCandidate(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available on PATH")
	}
	root := t.TempDir()
	writeSurfaceFile(t, root, "Makefile", "check:\n\t@echo surface ok\n")
	writeSurfaceFile(t, root, "src/index.ts", "export const x = 1\n")

	mu := types.NewMutableState("escalate")
	ctx := &types.BusContext{Mutable: mu, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("make check passes, so the merged verdict must pass: %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("verify run must install ChangeReport")
	}
	if len(report.NoTestsRunners) == 0 || report.NoTestsRunners[0] != "python" {
		t.Fatalf("python zero-test outcome must stay visible: %+v", report.NoTestsRunners)
	}
	var sawMakeExec, sawPythonSynthetic bool
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "make" && cmd.Outcome == "executed" && cmd.Source == "no_tests_escalation" {
			sawMakeExec = true
		}
		if cmd.Runner == "python" && cmd.Outcome == "synthetic_no_tests" && cmd.Source == "llm_choice" {
			sawPythonSynthetic = true
		}
	}
	if !sawPythonSynthetic {
		t.Fatalf("executed commands must record the synthetic python outcome: %+v", report.ExecutedCommands)
	}
	if !sawMakeExec {
		t.Fatalf("executed commands must record the escalated make execution: %+v", report.ExecutedCommands)
	}
	if report.TestSurface == nil || len(report.TestSurface.Candidates) == 0 {
		t.Fatal("report must carry the typed test surface")
	}
	if report.GeneratedAt.IsZero() {
		t.Fatal("report must carry a non-zero GeneratedAt")
	}
}

// Syntax fallback is useful for bare script edits, but it is not
// authoritative when the typed surface still has a runnable contract. This is
// the multi-repo SDK shape: a source file parses, while Makefile check is the
// real package-level contract.
func TestRunTests_SyntaxFallbackEscalatesToSurfaceCandidate(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available on PATH")
	}
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python dry-build runner on PATH; skip")
	}
	root := t.TempDir()
	writeSurfaceFile(t, root, "Makefile", "check:\n\t@echo contract ok\n")
	writeSurfaceFile(t, root, "pkg/client.py", "def search(query):\n    return query\n")
	writeSurfaceFile(t, root, "tests/check_contract.py", "print('contract')\n")

	mu := types.NewMutableState("syntax-fallback-escalate")
	mu.SetChangePlan(&types.ChangePlan{
		ID:          "plan-syntax-fallback",
		TargetPaths: []string{"pkg/client.py"},
	})
	ctx := &types.BusContext{Mutable: mu, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("make check passes, so syntax fallback escalation must pass: %+v report=%+v", result, mu.ChangeReport())
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("verify run must install ChangeReport")
	}
	var sawPythonSyntax, sawMakeExec bool
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "python" && cmd.Outcome == "syntax_check_fallback" && cmd.Source == "llm_choice" {
			sawPythonSyntax = true
		}
		if cmd.Runner == "make" && cmd.Outcome == "executed" && cmd.Source == "syntax_check_fallback_escalation" && cmd.Command == "make check" {
			sawMakeExec = true
		}
	}
	if !sawPythonSyntax || !sawMakeExec {
		t.Fatalf("expected python syntax fallback then make check escalation, got %+v", report.ExecutedCommands)
	}
	if report.TestSurface == nil {
		t.Fatal("report must carry the typed test surface")
	}
}

func TestRunTests_RunnerMissingEscalationDoesNotLeakSuiteToSurfaceCandidate(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available on PATH")
	}
	root := t.TempDir()
	writeSurfaceFile(t, root, "pom.xml", "<project/>\n")
	writeSurfaceFile(t, root, "Makefile", "check:\n\t@echo surface ok\n")
	writeSurfaceFile(t, root, "src/test/java/org/example/ExampleTest.java", "class ExampleTest {}\n")
	fakeBin := t.TempDir()
	fakeMvn := filepath.Join(fakeBin, "mvn")
	if err := os.WriteFile(fakeMvn, []byte("#!/bin/sh\necho mvn missing >&2\nexit 127\n"), 0o755); err != nil {
		t.Fatalf("write fake mvn: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	mu := types.NewMutableState("escalate-suite")
	ctx := &types.BusContext{Mutable: mu, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "java",
		"suite":  "org.example.ExampleTest",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("make check passes, so runner-missing escalation must pass: %+v report=%+v", result, mu.ChangeReport())
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("verify run must install ChangeReport")
	}
	var sawJavaMissing, sawMakeCheck bool
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "java" && cmd.Outcome == "runner_missing" && cmd.Source == "llm_choice" {
			sawJavaMissing = true
		}
		if cmd.Runner == "make" && cmd.Source == "runner_missing_escalation" {
			if cmd.Command != "make check" {
				t.Fatalf("surface make candidate must not inherit LLM suite; command=%q", cmd.Command)
			}
			sawMakeCheck = true
		}
	}
	if !sawJavaMissing || !sawMakeCheck {
		t.Fatalf("executed commands should show java missing then make check escalation: %+v", report.ExecutedCommands)
	}
}

func TestRunTests_ParserZeroTestsEscalatesAgainToMake(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available on PATH")
	}
	fakePython3WithoutPytest(t)
	root := t.TempDir()
	writeSurfaceFile(t, root, "pyproject.toml", "[tool.pytest.ini_options]\n")
	writeSurfaceFile(t, root, "Makefile", "check:\n\tPYTHONPATH=. python3 -m unittest discover -s tests -v\n")
	writeSurfaceFile(t, root, "fastlex/tokenizer.py", "class FastTokenizer:\n    pass\n")
	writeSurfaceFile(t, root, "tests/test_tokenizer.py", "import unittest\n\nclass TokenizerTest(unittest.TestCase):\n    def test_ok(self):\n        self.assertTrue(True)\n")

	mu := types.NewMutableState("zero-tests-multi-hop")
	ctx := &types.BusContext{Mutable: mu, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("make check passes, so multi-hop zero-test escalation must pass: %+v report=%+v", result, mu.ChangeReport())
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("verify run must install ChangeReport")
	}
	var sawPytestMissing, sawUnittestZero, sawMakeCheck bool
	for _, cmd := range report.ExecutedCommands {
		switch {
		case cmd.Runner == "python" && cmd.Framework == "pytest" && cmd.Outcome == "runner_missing" && cmd.Source == "llm_choice":
			sawPytestMissing = true
		case cmd.Runner == "python" && cmd.Framework == "unittest" && cmd.Outcome == "zero_tests" && cmd.Source == "runner_missing_escalation":
			sawUnittestZero = true
		case cmd.Runner == "make" && cmd.Outcome == "executed" && cmd.Source == "zero_tests_escalation" && cmd.Command == "make check":
			sawMakeCheck = true
		}
	}
	if !sawPytestMissing || !sawUnittestZero || !sawMakeCheck {
		t.Fatalf("expected pytest missing -> unittest zero tests -> make check, got %+v", report.ExecutedCommands)
	}
	if len(report.NoTestsRunners) == 0 {
		t.Fatalf("zero-test runner must stay visible in aggregate report: %+v", report)
	}
}

func TestNextTestSurfaceEscalationForRunnerDoesNotCrossLanguage(t *testing.T) {
	surface := types.TestSurface{Candidates: []types.TestSurfaceCandidate{
		{Runner: "python", Framework: "django", WorkingDir: ".", HasTestSignal: true},
		{Runner: "node", WorkingDir: ".", HasTestSignal: true},
	}}
	executed := map[string]bool{
		testSurfaceCandidateKey("python", "django", "."): true,
	}
	if got := nextTestSurfaceEscalationForRunner(surface, executed, "python"); got != nil {
		t.Fatalf("python-only change should not escalate to unrelated runner: %+v", got)
	}
	if got := nextTestSurfaceEscalationForRunner(surface, executed, ""); got == nil || got.Runner != "node" {
		t.Fatalf("unrestricted escalation should still find node fallback, got %+v", got)
	}
}

func TestRunTests_ParserZeroTestsWithoutEscalationFinishesUnverified(t *testing.T) {
	fakePython3WithoutPytest(t)
	root := t.TempDir()
	writeSurfaceFile(t, root, "pyproject.toml", "[tool.pytest.ini_options]\n")
	writeSurfaceFile(t, root, "tests/test_tokenizer.py", "import unittest\n\nclass TokenizerTest(unittest.TestCase):\n    def test_ok(self):\n        self.assertTrue(True)\n")

	mu := types.NewMutableState("zero-tests-no-escalation")
	ctx := &types.BusContext{Mutable: mu, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("zero-test parser result with no runnable fallback should finish unverified, got %+v report=%+v", result, mu.ChangeReport())
	}
	report := mu.ChangeReport()
	if report == nil || !report.Passed {
		t.Fatalf("report must record no-tests as a non-code-failure verdict: %+v", report)
	}
	var sawUnittestZero bool
	for _, cmd := range report.ExecutedCommands {
		if cmd.Runner == "python" && cmd.Framework == "unittest" && cmd.Outcome == "zero_tests" {
			sawUnittestZero = true
		}
	}
	if !sawUnittestZero {
		t.Fatalf("executed commands must record parser zero-tests outcome: %+v", report.ExecutedCommands)
	}
	if report.FailureKind != types.FailureKindNoTests {
		t.Fatalf("zero-tests should be classified as no_tests, got %q", report.FailureKind)
	}
	if len(report.NoTestsRunners) == 0 {
		t.Fatalf("NoTestsRunners must preserve the unverified reason: %+v", report)
	}
}

// A failing escalated candidate must fail the merged verdict — escalation is
// real verification, not a pass-rescue.
func TestRunTests_EscalatedCandidateFailureFailsVerdict(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available on PATH")
	}
	root := t.TempDir()
	writeSurfaceFile(t, root, "Makefile", "check:\n\t@echo red\n\t@exit 1\n")
	writeSurfaceFile(t, root, "src/index.ts", "export const x = 1\n")

	mu := types.NewMutableState("escalate-fail")
	ctx := &types.BusContext{Mutable: mu, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"runner": "python",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Success {
		t.Fatalf("failing make check must fail the merged verdict: %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil || report.Passed {
		t.Fatalf("report must record the failure: %+v", report)
	}
}

// Escalation is bounded and auto-detect, which already runs every candidate,
// never escalates.
func TestRunTests_AutoDetectDoesNotEscalate(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available on PATH")
	}
	root := t.TempDir()
	writeSurfaceFile(t, root, "Makefile", "check:\n\t@echo ok\n")

	mu := types.NewMutableState("auto")
	ctx := &types.BusContext{Mutable: mu, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("make check passes: %+v", result)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("report missing")
	}
	for _, cmd := range report.ExecutedCommands {
		if cmd.Source != "auto_detect" {
			t.Fatalf("auto-detect run must not contain escalation rows: %+v", report.ExecutedCommands)
		}
	}
}

func TestActiveSubRepoWalkRootIgnoresStaleRootOutsideCurrentRepo(t *testing.T) {
	parent := t.TempDir()
	active := filepath.Join(parent, "bindings-py")
	if err := os.MkdirAll(active, 0o755); err != nil {
		t.Fatalf("mkdir active: %v", err)
	}
	ctx := &types.BusContext{
		RepoRoot:      parent,
		ActiveSubRepo: &types.SubRepoSnapshot{RootAbs: active, RootRel: "bindings-py"},
	}
	if got := activeSubRepoWalkRoot(ctx); got == "" || !samePath(got, active) {
		t.Fatalf("active sub-repo inside repo must be used as walk root, got %q", got)
	}

	worktree := t.TempDir()
	ctx.RepoRoot = worktree
	if got := activeSubRepoWalkRoot(ctx); got != "" {
		t.Fatalf("stale active sub-repo outside current worktree must be ignored, got %q", got)
	}
}

func TestExtractExitCode_SuccessIsZero(t *testing.T) {
	if got := extractExitCode(nil); got != 0 {
		t.Fatalf("nil error must report exit 0, got %d", got)
	}
}

// A bare Python directory (no manifest at all) must still produce a
// runnable surface candidate from test-file conventions, and a pytest
// candidate gains a stdlib unittest sibling when the unittest signal is
// present — that sibling is the escalation target when pytest's binary is
// missing.
func TestBuildTestSurface_BarePythonDirSynthesizesCandidates(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "relativedelta.py", "class relativedelta:\n    pass\n")
	writeSurfaceFile(t, root, "test_relativedelta.py", "import unittest\n\nclass T(unittest.TestCase):\n    def test_ok(self):\n        self.assertTrue(True)\n")

	surface := BuildTestSurface(root, "")
	if len(surface.Candidates) == 0 {
		t.Fatal("bare python dir must synthesize a candidate")
	}
	py := surfaceCandidate(t, surface, "python")
	if !py.HasTestSignal {
		t.Fatalf("synthesized python candidate must carry the test signal: %+v", py)
	}
	if py.Framework != "unittest" {
		t.Fatalf("unittest-signal repo should synthesize the unittest framework, got %q", py.Framework)
	}
	if py.Source != "test-file convention" {
		t.Fatalf("synthesized candidate must declare its provenance, got %q", py.Source)
	}
}

func TestBuildTestSurface_PytestCandidateGainsUnittestSibling(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "pytest.ini", "[pytest]\n")
	writeSurfaceFile(t, root, "test_sample.py", "import unittest\n\nclass T(unittest.TestCase):\n    def test_ok(self):\n        self.assertTrue(True)\n")

	surface := BuildTestSurface(root, "")
	var sawPytest, sawUnittest bool
	for _, c := range surface.Candidates {
		if c.Runner != "python" {
			continue
		}
		switch c.Framework {
		case "pytest":
			sawPytest = true
		case "unittest":
			sawUnittest = true
		}
	}
	if !sawPytest || !sawUnittest {
		t.Fatalf("expected pytest + unittest sibling candidates, got %+v", surface.Candidates)
	}
	// pytest dead end escalates to the unittest sibling of the same dir.
	executed := map[string]bool{testSurfaceCandidateKey("python", "pytest", "."): true}
	next := nextTestSurfaceEscalation(surface, executed)
	if next == nil || next.Framework != "unittest" {
		t.Fatalf("pytest dead end must escalate to the unittest sibling, got %+v", next)
	}
}
