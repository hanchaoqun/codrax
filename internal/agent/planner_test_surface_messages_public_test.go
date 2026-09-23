package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the real planner's first adapter request. Its ordinary keyword
// search builds the graph from fixture bytes; no test-profile helper is called.
func plannerManifestPublicMessage(t *testing.T, files map[string]string, held bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	for path, body := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mu := types.NewMutableState("Inspect increment and propose the bounded implementation change.")
	bus := &types.BusContext{RepoRoot: root, WorkDir: root, Mode: types.ModePlan, Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{AnalyzerHints: types.AnalyzerHints{Keywords: []string{"increment"}}}}}
	if held {
		mu.SetChangePlan(&types.ChangePlan{ID: "held-native-plan"})
		mu.SetChangeReport(&types.ChangeReport{PlanID: "held-native-plan", Channel: types.ChangeReportChannelPostApplyVerify,
			TestResults: []types.TestResult{{Kind: "unit", ObservationScope: types.TestObservationScopeAssertion,
				Suite: "python/unittest@packages/widget::tests.test_widget.IncrementTest", AssertionID: "python/unittest@packages/widget::test_zero", Passed: true}}})
	}
	snapshot := func() string {
		data, err := json.Marshal([]any{bus.AnalysisIR, mu.ChangePlan(), mu.ChangeReport(), mu.WriteAnalysisIR()})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	before := snapshot()
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentPlanner, types.StagePlan)
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured planner manifest navigation")}
	planner := NewPlannerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1})
	_, err := planner.Execute(ctx, traceTeachingSkill(t, "change-plan-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("HARNESS: missing real initial request: calls=%d err=%v", capture.calls, err)
	}
	if mu.SearchGraph() == nil {
		t.Fatal("HARNESS: real fixture search did not populate the graph")
	}
	if snapshot() != before || len(mu.DispatchToolResults()) != 0 {
		t.Fatal("navigation changed plan/report/analysis or executed an agent tool")
	}
	for path, want := range files {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil || string(got) != want {
			t.Fatalf("navigation changed fixture %s: %v", path, err)
		}
	}
	var message strings.Builder
	for _, row := range capture.messages {
		message.WriteString(row.Content)
		message.WriteByte('\n')
	}
	all := message.String()
	section := ""
	if start := strings.Index(all, "## Test surface\n"); start >= 0 {
		section = all[start:]
		if end := strings.Index(section[1:], "\n## "); end >= 0 {
			section = section[:end+1]
		}
	}
	if !strings.Contains(all, "Use the exact TestResult.suite and TestResult.assertion_id pair") ||
		!strings.Contains(all, "The project-test declaration is not proof") {
		t.Fatal("exact native observation teaching was lost")
	}
	foundEmit := false
	for _, schema := range capture.tools {
		if schema.Name == "emit_change_plan" {
			foundEmit = json.Valid(schema.Parameters) && strings.Contains(string(schema.Parameters), "project_test_observations")
		}
	}
	if !foundEmit {
		t.Fatal("ordinary planner emit/PTO tool surface was changed")
	}
	return section, all
}

func TestPlannerManifestPublicInitialMessageDoesNotChooseProtocol(t *testing.T) {
	for _, tc := range []struct{ manifest, label, forbidden string }{
		{"setup.py", "python (package manifest)", "python (pytest)"},
		{"pyproject.toml", "python (project manifest)", "python (pytest)"},
		{"Gemfile", "ruby (Bundler manifest)", "rspec"},
		{"package.json", "node (package manifest)", "npm test"},
		{"Makefile", "make (build file)", "make check"},
		{"build.gradle", "java (Gradle project)", "gradlew test"},
		{"build.gradle.kts", "java (Gradle project)", "gradlew test"},
		{"oh-package.json5", "hvigor (project/build marker)", "hvigorw test"},
		{"build-profile.json5", "hvigor (project/build marker)", "hvigorw test"},
		{"hvigorfile.ts", "hvigor (project/build marker)", "hvigorw test"},
		{"CMakeLists.txt", "cmake (build project)", "ctest"},
		{"meson.build", "meson (build project)", "meson test"},
		{"go.mod", "go (module)", "go test"},
		{"Cargo.toml", "rust (Cargo project)", "cargo test"},
		{"pom.xml", "java (Maven project)", "mvn test"},
		{"cjpm.toml", "cjpm (Cangjie project)", "cjpm test"},
		{"Package.swift", "swift (Swift package)", "swift test"},
	} {
		t.Run(tc.manifest, func(t *testing.T) {
			section, _ := plannerManifestPublicMessage(t, map[string]string{
				"service/" + tc.manifest: "", "service/worker.py": "def increment(value):\n    return value\n",
			}, false)
			if !strings.Contains(section, "service/"+tc.manifest) || !strings.Contains(section, tc.label) || strings.Contains(section, tc.forbidden) {
				t.Fatalf("generic manifest selected a test protocol or lost its project marker:\n%s", section)
			}
			if !strings.Contains(section, "navigation") || !strings.Contains(section, "selected native protocol") {
				t.Fatalf("manifest authority boundary not taught:\n%s", section)
			}
		})
	}
}

func TestPlannerManifestPublicNestedFixtureAndHeldIdentity(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(map[bool]string{false: "first_plan", true: "current_native_report"}[held], func(t *testing.T) {
			files := map[string]string{}
			for _, path := range []string{"packages/widget/widget.py", "packages/widget/setup.py", "packages/widget/tests/test_widget.py", "packages/widget/tests/__init__.py"} {
				data, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "testdata", "nested_python_increment", filepath.FromSlash(path)))
				if err != nil {
					t.Fatal(err)
				}
				files[path] = string(data)
			}
			section, all := plannerManifestPublicMessage(t, files, held)
			if !strings.Contains(section, "packages/widget/setup.py") || !strings.Contains(section, "packages/widget/tests") || strings.Contains(section, "pytest") || strings.Contains(section, "unittest") {
				t.Fatalf("manifest-only profile guessed inspected tests' native protocol:\n%s", section)
			}
			identity := "python/unittest@packages/widget::tests.test_widget.IncrementTest"
			if strings.Contains(all, identity) != held {
				t.Fatalf("current native identity was invented, overwritten, or dropped (held=%v)", held)
			}
		})
	}
}

func TestPlannerManifestPublicExplicitConfigAndNoMarker(t *testing.T) {
	for _, tc := range []struct {
		name, config string
	}{
		{"explicit_pytest", "packages/widget/pytest.ini"},
		{"separate_project_pytest", "other/pytest.ini"},
		{"no_manifest", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{"packages/widget/worker.py": "def increment(value):\n    return value\n"}
			if tc.config != "" {
				files[tc.config] = "[pytest]\n"
				files["packages/widget/setup.py"] = "from setuptools import setup\n"
			}
			section, _ := plannerManifestPublicMessage(t, files, false)
			if tc.config == "" {
				if section != "" {
					t.Fatalf("no marker invented test surface: %s", section)
				}
				return
			}
			if !strings.Contains(section, tc.config) || !strings.Contains(section, "python (pytest configuration marker)") ||
				!strings.Contains(section, "configuration markers apply only to their containing project") {
				t.Fatalf("explicit configuration lost or leaked project scope:\n%s", section)
			}
		})
	}
}
