package tool

import (
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
	executed := map[string]bool{testSurfaceCandidateKey("go", "sub"): true}
	next := nextTestSurfaceEscalation(surface, executed)
	if next == nil || next.Runner != "make" {
		t.Fatalf("escalation should pick make (unexecuted, has signal), got %+v", next)
	}
	executed[testSurfaceCandidateKey("make", ".")] = true
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

// The escalation is bounded: at most one extra candidate per Execute, and
// auto-detect (which already runs every candidate) never escalates.
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
