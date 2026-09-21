package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestJavaProbeSourceUnitSelection(t *testing.T) {
	cases := []struct {
		name, code, file, main string
		wrapped                bool
	}{
		{"arbitrary", "\npublic class Renamed { public static void main(String[] args) {} }\n", "Renamed.java", "Renamed", false},
		{"standard", "public final class CodraxVerificationProbe { public static void main(String[] args) {} }", "CodraxVerificationProbe.java", "CodraxVerificationProbe", false},
		{"package", "/* header */ package example.probe; public class Probe { public static void main(java.lang.String[] args) {} }", "Probe.java", "example.probe.Probe", false},
		{"public_helper", "public class Helper {} class Runner { public static void main(String[] args) {} }", "Helper.java", "Runner", false},
		{"helper_before_owner", "class Helper {} class Runner { public static void main(String... args) {} }", "Runner.java", "Runner", false},
		{"array_after_name", "class Runner { public static void main(String args[]) {} }", "Runner.java", "Runner", false},
		{"enum", "public enum Probe { ONLY; public static void main(String[] args) {} }", "Probe.java", "Probe", false},
		{"interface", "public interface Probe { static void main(String[] args) {} }", "Probe.java", "Probe", false},
		{"fake_classes_in_full_source", "// class CodraxVerificationProbe {}\npublic class Actual { static String text=\"class Fake { public static void main(String[] x) {} }\"; public static void main(String[] args) {} }", "Actual.java", "Actual", false},
		{"snippet", "if (false) throw new AssertionError(\"wrong\");", "CodraxVerificationProbe.java", "CodraxVerificationProbe", true},
		{"snippet_import_after_comment", "// class CodraxVerificationProbe {}\nimport java.util.Objects;\nif (!Objects.equals(1, 1)) throw new AssertionError();", "CodraxVerificationProbe.java", "CodraxVerificationProbe", true},
		{"snippet_literal_fake_class", "String text = \"class CodraxVerificationProbe {}\"; if (text.isEmpty()) throw new AssertionError();", "CodraxVerificationProbe.java", "CodraxVerificationProbe", true},
		{"snippet_local_class", "class Helper {} new Helper();", "CodraxVerificationProbe.java", "CodraxVerificationProbe", true},
		{"snippet_syntax_left_to_javac", "if (true) { int = ; }", "CodraxVerificationProbe.java", "CodraxVerificationProbe", true},
		{"late_import_not_repaired", "System.out.println(\"before\"); import java.util.Objects; if (!Objects.equals(1, 1)) throw new AssertionError();", "CodraxVerificationProbe.java", "CodraxVerificationProbe", true},
		{"two_mains", "class A { public static void main(String[] x) {} } class B { public static void main(String[] x) {} }", "CodraxVerificationProbe.java", "", false},
		{"two_public_types", "public class A { public static void main(String[] x) {} } public class B {}", "CodraxVerificationProbe.java", "", false},
		{"nested_only", "public class Outer { public static class Inner { public static void main(String[] x) {} } }", "Outer.java", "", false},
		{"nonstatic", "public class Probe { public void main(String[] x) {} }", "Probe.java", "", false},
		{"private", "public class Probe { private static void main(String[] x) {} }", "Probe.java", "", false},
		{"wrong_return", "public class Probe { public static int main(String[] x) { return 0; } }", "Probe.java", "", false},
		{"wrong_parameter", "public class Probe { public static void main(int[] x) {} }", "Probe.java", "", false},
		{"two_dimensions", "public class Probe { public static void main(String[][] x) {} }", "Probe.java", "", false},
		{"no_parameters", "public class Probe { public static void main() {} }", "Probe.java", "", false},
		// Selection is not compilation validity: javac, not tree-sitter,
		// rejects an import from the unnamed package.
		{"illegal_import", "import Main;\npublic class Probe { public static void main(String[] x) { Main.greet(\"world\"); } }", "Probe.java", "Probe", false},
		{"incomplete_unit", "public class Broken {", "Broken.java", "", false},
		{"body_error_not_selection_authority", "public class Probe { public static void main(String[] args) {} void other() { int = ; } }", "Probe.java", "Probe", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unit := prepareJavaProbeSourceUnit(context.Background(), tc.code)
			if unit.FileName != tc.file || unit.MainClass != tc.main {
				t.Fatalf("selection file=%q main=%q issue=%q; want %q/%q", unit.FileName, unit.MainClass, unit.Issue, tc.file, tc.main)
			}
			if !tc.wrapped && unit.Source != tc.code {
				t.Fatalf("full source was rewritten:\n%s", unit.Source)
			}
			if tc.wrapped && (!strings.Contains(unit.Source, "public final class CodraxVerificationProbe") || unit.Source == tc.code) {
				t.Fatalf("statement fragment did not get the supported wrapper: %s", unit.Source)
			}
			if tc.name == "late_import_not_repaired" && !strings.Contains(unit.Source, tc.code) {
				t.Fatalf("late import was moved or removed from authored statement order: %s", unit.Source)
			}
		})
	}
}

func TestRunTestsJavaSourceUnitInvalidFragmentsStayCompilerFailures(t *testing.T) {
	for name, code := range map[string]string{
		"late_import":   "System.out.println(\"before\"); import java.util.Objects; if (!Objects.equals(1, 1)) throw new AssertionError();",
		"bad_statement": "if (true) { int = ; throw new AssertionError(); }",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			capture := filepath.Join(t.TempDir(), "source")
			pathCapture := filepath.Join(t.TempDir(), "source-path")
			javaCapture := filepath.Join(t.TempDir(), "java-invoked")
			installJavaSourceUnitObservingJDK(t, capture, pathCapture, javaCapture, true)
			ctx := &types.BusContext{Mutable: types.NewMutableState("invalid Java fragment"), Mode: types.ModeApply, PipelineStage: types.StagePlan, RepoRoot: root, MainRepoRoot: root}
			result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
				"dry_run": true, "verification_probe": map[string]any{"id": name, "language": "java", "code": code},
			}))
			if err != nil || result.Success {
				t.Fatalf("javac refusal must be retained: %+v / %v", result, err)
			}
			wantSource := "public final class CodraxVerificationProbe {\n  public static void main(String[] args) throws Exception {\n" + code + "\n  }\n}\n"
			assertJavaSourceUnitCompilerInput(t, capture, pathCapture, wantSource, "CodraxVerificationProbe.java")
			reports := ctx.Mutable.PlanStageProbeReports()
			if len(reports) != 1 || reports[0].FailureReasonCode != "verification_probe_java_compile_error" || len(reports[0].ExecutedCommands) != 1 {
				t.Fatalf("compiler failure receipt missing: %+v", reports)
			}
			if _, err := os.Stat(javaCapture); !os.IsNotExist(err) {
				t.Fatalf("invalid fragment launched java: %v", err)
			}
		})
	}
}

func TestJavaProbeSourceUnitCanceledSelectionPreservesSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code := "public class Probe { public static void main(String[] args) {} }"
	unit := prepareJavaProbeSourceUnit(ctx, code)
	if unit.Source != code {
		t.Fatal("cancellation must not rewrite authored source")
	}
}

func TestRunTestsJavaSourceUnitAmbiguityDoesNotLaunch(t *testing.T) {
	root := t.TempDir()
	capture, javaArgs := filepath.Join(t.TempDir(), "source.java"), filepath.Join(t.TempDir(), "args")
	installFakeJavaProbeRuntime(t, capture, true)
	t.Setenv("CODRAX_TEST_JAVA_ARGS_CAPTURE", javaArgs)
	code := "class A { public static void main(String[] x) { throw new AssertionError(); } } class B { public static void main(String[] x) {} }"
	mu := types.NewMutableState("ambiguous Java source")
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StagePlan, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"dry_run": true, "verification_probe": map[string]any{"id": "ambiguous", "language": "java", "code": code},
	}))
	if err != nil || result.Success || !strings.Contains(result.Summary, "unambiguous") {
		t.Fatalf("ambiguous entry must be unavailable, not a guessed runtime: %+v / %v", result, err)
	}
	if got, err := os.ReadFile(capture); err != nil || string(got) != code {
		t.Fatalf("javac did not receive unchanged source: %s / %v", got, err)
	}
	if _, err := os.Stat(javaArgs); !os.IsNotExist(err) {
		t.Fatalf("ambiguous source launched java: %v", err)
	}
}

func TestJavaProbeSourceUnitRealJDK(t *testing.T) {
	if err := exec.Command("javac", "-version").Run(); err != nil {
		t.Skip("real JDK unavailable; source/filename/main transport is covered separately")
	}
	root := t.TempDir()
	mu := types.NewMutableState("real Java source")
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StagePlan, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"dry_run": true, "verification_probe": map[string]any{
			"id": "real-source", "language": "java",
			"code":            "package actual.probe; public class ArbitraryName { public static void main(String... args) { System.out.println(\"REAL_JDK_OK\"); } }",
			"expected_stdout": []string{"REAL_JDK_OK"},
		},
	}))
	if err != nil || !result.Success {
		t.Fatalf("real JDK must compile and launch the authored public main class: %+v / %v", result, err)
	}
}

func TestJavaProbeSourceUnitSyntaxAndExecutionShareCompilerInput(t *testing.T) {
	root := t.TempDir()
	capture := filepath.Join(t.TempDir(), "source")
	pathCapture := filepath.Join(t.TempDir(), "source-path")
	javaCapture := filepath.Join(t.TempDir(), "java-invoked")
	installJavaSourceUnitObservingJDK(t, capture, pathCapture, javaCapture, false)
	code := "package source.shared; public class UniqueProbe { public static void main(String[] args) { if (false) throw new AssertionError(); } }"
	ctx := &types.BusContext{Mutable: types.NewMutableState("shared Java preparation"), Mode: types.ModeApply, PipelineStage: types.StagePlan, RepoRoot: root, MainRepoRoot: root}
	if got := javaVerificationProbeSyntaxError(ctx, code); got != "" {
		t.Fatalf("syntax preparation failed: %s", got)
	}
	assertJavaSourceUnitCompilerInput(t, capture, pathCapture, code, "UniqueProbe.java")
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"dry_run": true, "verification_probe": map[string]any{"id": "shared", "language": "java", "code": code},
	}))
	if err != nil || !result.Success {
		t.Fatalf("execution preparation failed: %+v / %v", result, err)
	}
	assertJavaSourceUnitCompilerInput(t, capture, pathCapture, code, "UniqueProbe.java")
	args, err := os.ReadFile(javaCapture)
	if err != nil || !strings.HasSuffix(strings.TrimSpace(string(args)), "source.shared.UniqueProbe") {
		t.Fatalf("wrong prepared launch: %s / %v", args, err)
	}
}

func TestRunTestsJavaSourceUnitIllegalImportStaysCompilerFailure(t *testing.T) {
	root := t.TempDir()
	capture := filepath.Join(t.TempDir(), "source")
	pathCapture := filepath.Join(t.TempDir(), "source-path")
	javaCapture := filepath.Join(t.TempDir(), "java-invoked")
	installJavaSourceUnitObservingJDK(t, capture, pathCapture, javaCapture, true)
	code := "import Main;\npublic class ProbeMain { public static void main(String[] args) { if (!Main.greet(\"world\").equals(\"Hello, world!\")) throw new AssertionError(); } }"
	ctx := &types.BusContext{Mutable: types.NewMutableState("invalid Java import"), Mode: types.ModeApply, PipelineStage: types.StagePlan, RepoRoot: root, MainRepoRoot: root}
	// Only the existing precise javac diagnostic can reject emit-time syntax.
	if got := javaVerificationProbeSyntaxError(ctx, code); !strings.Contains(got, "compiler.err.expected") {
		t.Fatalf("definite javac rejection was lost: %s", got)
	}
	assertJavaSourceUnitCompilerInput(t, capture, pathCapture, code, "ProbeMain.java")
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"dry_run": true, "verification_probe": map[string]any{"id": "illegal-import", "language": "java", "code": code},
	}))
	if err != nil || result.Success {
		t.Fatalf("illegal import must not be repaired into a pass: %+v / %v", result, err)
	}
	assertJavaSourceUnitCompilerInput(t, capture, pathCapture, code, "ProbeMain.java")
	reports := ctx.Mutable.PlanStageProbeReports()
	if len(reports) != 1 || reports[0].FailureReasonCode != "verification_probe_java_compile_error" || len(reports[0].ExecutedCommands) != 1 {
		t.Fatalf("compile failure identity lost: %+v", reports)
	}
	if _, err := os.Stat(javaCapture); !os.IsNotExist(err) {
		t.Fatalf("java must not execute after a compiler refusal: %v", err)
	}
}

func assertJavaSourceUnitCompilerInput(t *testing.T, capture, pathCapture, code, file string) {
	t.Helper()
	got, err := os.ReadFile(capture)
	if err != nil || string(got) != code {
		t.Fatalf("compiler source changed: %s / %v", got, err)
	}
	path, err := os.ReadFile(pathCapture)
	if err != nil || filepath.Base(string(path)) != file {
		t.Fatalf("compiler filename=%s / %v; want %s", path, err, file)
	}
}

func installJavaSourceUnitObservingJDK(t *testing.T, capture, pathCapture, javaCapture string, reject bool) {
	t.Helper()
	bin := t.TempDir()
	compiler := `#!/bin/sh
set -eu
last=""
for arg in "$@"; do
  case "$arg" in *.java) last="$arg" ;; esac
done
cat "$last" > "$CODRAX_JAVA_UNIT_SOURCE"
printf '%s' "$last" > "$CODRAX_JAVA_UNIT_PATH"
if [ "$CODRAX_JAVA_UNIT_REJECT" = "true" ]; then
  echo "$last:1:12: compiler.err.expected: '.'" >&2
  exit 1
fi
`
	runner := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CODRAX_JAVA_UNIT_RUN\"\n"
	for name, code := range map[string]string{"javac": compiler, "java": runner} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(code), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CODRAX_JAVA_UNIT_SOURCE", capture)
	t.Setenv("CODRAX_JAVA_UNIT_PATH", pathCapture)
	t.Setenv("CODRAX_JAVA_UNIT_RUN", javaCapture)
	t.Setenv("CODRAX_JAVA_UNIT_REJECT", map[bool]string{true: "true", false: "false"}[reject])
}

// This public RunTests regression checks the actual compiler input and launch
// receipt, not a substitute Java parser. The fake JDK only observes that boundary;
// real-JDK tests below remain responsible for compilation semantics.
func TestRunTestsJavaSourceUnitArbitraryPublicMain(t *testing.T) {
	root := t.TempDir()
	capture := filepath.Join(t.TempDir(), "source.java")
	installFakeJavaProbeRuntime(t, capture, true)
	code := "public class ProbeMain {\n" +
		"  public static void main(String[] args) {\n" +
		"    if (42 != 42) throw new AssertionError(\"wrong\");\n" +
		"    System.out.println(\"VALUE=42\");\n" +
		"  }\n}\n"
	mu := types.NewMutableState("Java complete source")
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StagePlan, RepoRoot: root, MainRepoRoot: root}
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
		"dry_run":            true,
		"verification_probe": map[string]any{"id": "source-unit", "language": "java", "code": code, "expected_stdout": []string{"VALUE=42"}},
	}))
	if err != nil || !result.Success {
		t.Fatalf("public RunTests failed: result=%+v err=%v", result, err)
	}
	got, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	// RunTests' existing probe normalization trims outer whitespace. The Java
	// transport must preserve those normalized authored bytes exactly.
	if string(got) != strings.TrimSpace(code) {
		t.Errorf("complete Java unit must reach javac unchanged; got:\n%s", got)
	}
	reports := mu.PlanStageProbeReports()
	if len(reports) != 1 || len(reports[0].ExecutedCommands) != 2 {
		t.Fatalf("want compiler and runtime receipts: %+v", reports)
	}
	compile := reports[0].ExecutedCommands[0].ProbeExecution
	run := reports[0].ExecutedCommands[1].ProbeExecution
	if compile == nil || run == nil {
		t.Fatal("missing execution identity")
	}
	if filepath.Base(compile.Args[len(compile.Args)-1]) != "ProbeMain.java" {
		t.Errorf("compiler source file must match public type ProbeMain: %v", compile.Args)
	}
	if run.Args[len(run.Args)-1] != "ProbeMain" {
		t.Errorf("runtime must launch authored main class: %v", run.Args)
	}
	if !strings.Contains(result.Summary, "VALUE=42") {
		t.Fatalf("probe result lost runtime output: %s", result.Summary)
	}
}
