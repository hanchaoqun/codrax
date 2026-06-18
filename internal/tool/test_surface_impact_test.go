package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestImpactRunnerPlansFromChangePlanTargetsPythonRelatedTest(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "pyproject.toml", "[project]\nname='x'\n")
	writeImpactSurfaceFixture(t, root, "pkg/a.py", "def value():\n    return 1\n")
	writeImpactSurfaceFixture(t, root, "tests/test_a.py", "def test_value():\n    assert True\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-python",
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:        "test_surface",
				Path:        "pkg/a.py",
				RelatedPath: "tests/test_a.py",
				Priority:    50,
				Source:      "impact_engine",
			}},
		},
	}

	plans := impactRunnerPlansFromChangePlan(root, surface, plan)
	if len(plans) != 1 {
		t.Fatalf("expected one impact runner plan, got %+v", plans)
	}
	if plans[0].Runner != "python" || plans[0].Suite != "tests/test_a.py" {
		t.Fatalf("unexpected python impact plan: %+v", plans[0])
	}

	queue := defaultRunnerPlansFromTestSurface(root, surface, "", plans...)
	if len(queue) == 0 || queue[0].Suite != "tests/test_a.py" {
		t.Fatalf("impact plan should lead the default queue, got %+v", queue)
	}
}

func TestImpactRunnerPlansPreferPytestFileSelectorOverUnittestFallback(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "pyproject.toml", "[tool.pytest.ini_options]\ntestpaths=['tests']\n")
	writeImpactSurfaceFixture(t, root, "pkg/a.py", "def value():\n    return 1\n")
	writeImpactSurfaceFixture(t, root, "tests/test_a.py", "import unittest\n\ndef test_value():\n    assert True\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-python-pytest-precision",
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:        "test_surface",
				Path:        "pkg/a.py",
				RelatedPath: "tests/test_a.py",
				Priority:    50,
				Source:      "impact_engine",
			}},
		},
	}

	plans := impactRunnerPlansFromChangePlan(root, surface, plan)
	if len(plans) != 1 {
		t.Fatalf("expected one impact runner plan, got %+v surface=%+v", plans, surface.Candidates)
	}
	if plans[0].Runner != "python" || plans[0].Framework != pythonFrameworkPytest || plans[0].Suite != "tests/test_a.py" {
		t.Fatalf("pytest file selector should beat unittest fallback for impact test path, got %+v surface=%+v", plans[0], surface.Candidates)
	}
}

func TestDefaultRunnerPlansPreservesMultipleImpactSuitesInSameWorkingDir(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "pyproject.toml", "[project]\nname='x'\n")
	writeImpactSurfaceFixture(t, root, "pkg/a.py", "def value():\n    return 1\n")
	writeImpactSurfaceFixture(t, root, "tests/test_a.py", "def test_a():\n    assert True\n")
	writeImpactSurfaceFixture(t, root, "tests/test_b.py", "def test_b():\n    assert True\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-python-multi",
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:        "test_surface",
				Path:        "pkg/a.py",
				RelatedPath: "tests/test_a.py",
				Priority:    50,
				Source:      "impact_engine",
			}, {
				Kind:        "test_surface",
				Path:        "pkg/a.py",
				RelatedPath: "tests/test_b.py",
				Priority:    50,
				Source:      "impact_engine",
			}},
		},
	}

	impactPlans := impactRunnerPlansFromChangePlan(root, surface, plan)
	if len(impactPlans) != 2 {
		t.Fatalf("expected two impact suite plans, got %+v", impactPlans)
	}
	queue := defaultRunnerPlansFromTestSurface(root, surface, "", impactPlans...)
	if len(queue) < 2 {
		t.Fatalf("default queue dropped impact suites: %+v", queue)
	}
	gotSuites := []string{queue[0].Suite, queue[1].Suite}
	if gotSuites[0] != "tests/test_a.py" || gotSuites[1] != "tests/test_b.py" {
		t.Fatalf("impact suites should lead queue in target order, got %+v queue=%+v", gotSuites, queue)
	}
}

func TestImpactRunnerPlansFromChangePlanTargetsGoPackage(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "go.mod", "module example.com/x\n\ngo 1.22\n")
	writeImpactSurfaceFixture(t, root, "pkg/a.go", "package pkg\n")
	writeImpactSurfaceFixture(t, root, "pkg/a_test.go", "package pkg\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-go",
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:        "test_surface",
				Path:        "pkg/a.go",
				RelatedPath: "pkg/a_test.go",
				Priority:    50,
				Source:      "impact_engine",
			}},
		},
	}

	plans := impactRunnerPlansFromChangePlan(root, surface, plan)
	if len(plans) != 1 {
		t.Fatalf("expected one impact runner plan, got %+v", plans)
	}
	if plans[0].Runner != "go" || plans[0].Suite != "./pkg" {
		t.Fatalf("unexpected go impact plan: %+v", plans[0])
	}
}

func TestImpactRunnerPlansFromChangePlanTargetsNodeRelatedTest(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "package.json", `{"scripts":{"test":"jest"}}`)
	writeImpactSurfaceFixture(t, root, "src/widget.ts", "export function value() { return 1 }\n")
	writeImpactSurfaceFixture(t, root, "src/widget.test.ts", "test('value', () => {})\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-node",
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:        "test_surface",
				Path:        "src/widget.ts",
				RelatedPath: "src/widget.test.ts",
				Priority:    50,
				Source:      "impact_engine",
			}},
		},
	}

	plans := impactRunnerPlansFromChangePlan(root, surface, plan)
	if len(plans) != 1 {
		t.Fatalf("expected one node impact runner plan, got %+v surface=%+v", plans, surface.Candidates)
	}
	if plans[0].Runner != "node" || plans[0].Suite != "src/widget.test.ts" {
		t.Fatalf("unexpected node impact plan: %+v", plans[0])
	}
	queue := defaultRunnerPlansFromTestSurface(root, surface, "", plans...)
	if len(queue) == 0 || queue[0].Runner != "node" || queue[0].Suite != "src/widget.test.ts" {
		t.Fatalf("node impact plan should lead default queue, got %+v", queue)
	}
}

func TestImpactRunnerPlansFromChangePlanTargetsJavaRelatedTest(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "pom.xml", "<project/>\n")
	writeImpactSurfaceFixture(t, root, "src/main/java/com/acme/Widget.java", "package com.acme;\nclass Widget {}\n")
	writeImpactSurfaceFixture(t, root, "src/test/java/com/acme/WidgetTest.java", "package com.acme;\nclass WidgetTest {}\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-java",
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:        "test_surface",
				Path:        "src/main/java/com/acme/Widget.java",
				RelatedPath: "src/test/java/com/acme/WidgetTest.java",
				Priority:    50,
				Source:      "impact_engine",
			}},
		},
	}

	plans := impactRunnerPlansFromChangePlan(root, surface, plan)
	if len(plans) != 1 {
		t.Fatalf("expected one java impact runner plan, got %+v surface=%+v", plans, surface.Candidates)
	}
	if plans[0].Runner != "java" || plans[0].Suite != "com.acme.WidgetTest" {
		t.Fatalf("unexpected java impact plan: %+v", plans[0])
	}
	queue := defaultRunnerPlansFromTestSurface(root, surface, "", plans...)
	if len(queue) == 0 || queue[0].Runner != "java" || queue[0].Suite != "com.acme.WidgetTest" {
		t.Fatalf("java impact plan should lead default queue, got %+v", queue)
	}
}

func TestImpactRunnerPlansFromChangePlanTargetsKotlinGradleRelatedTest(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "build.gradle.kts", "plugins { kotlin(\"jvm\") version \"1.9.0\" }\n")
	writeImpactSurfaceFixture(t, root, "src/main/kotlin/com/acme/Widget.kt", "package com.acme\nclass Widget\n")
	writeImpactSurfaceFixture(t, root, "src/test/kotlin/com/acme/WidgetTests.kt", "package com.acme\nclass WidgetTests\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-kotlin",
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:        "test_surface",
				Path:        "src/main/kotlin/com/acme/Widget.kt",
				RelatedPath: "src/test/kotlin/com/acme/WidgetTests.kt",
				Priority:    50,
				Source:      "impact_engine",
			}},
		},
	}

	plans := impactRunnerPlansFromChangePlan(root, surface, plan)
	if len(plans) != 1 {
		t.Fatalf("expected one kotlin/java impact runner plan, got %+v surface=%+v", plans, surface.Candidates)
	}
	if plans[0].Runner != "java" || plans[0].Suite != "com.acme.WidgetTests" {
		t.Fatalf("unexpected kotlin impact plan: %+v", plans[0])
	}
}

func TestImpactRunnerPlansFromChangePlanTargetsRustIntegrationTest(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "Cargo.toml", "[package]\nname='x'\nversion='0.1.0'\nedition='2021'\n")
	writeImpactSurfaceFixture(t, root, "src/lib.rs", "pub fn value() -> i32 { 1 }\n")
	writeImpactSurfaceFixture(t, root, "tests/widget.rs", "#[test]\nfn widget() {}\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-rust",
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:        "test_surface",
				Path:        "src/lib.rs",
				RelatedPath: "tests/widget.rs",
				Priority:    50,
				Source:      "impact_engine",
			}},
		},
	}

	plans := impactRunnerPlansFromChangePlan(root, surface, plan)
	if len(plans) != 1 {
		t.Fatalf("expected one rust impact runner plan, got %+v surface=%+v", plans, surface.Candidates)
	}
	if plans[0].Runner != "rust" || plans[0].Suite != "tests/widget.rs" {
		t.Fatalf("unexpected rust impact plan: %+v", plans[0])
	}
}

func TestImpactRunnerPlansFromChangePlanTargetsSwiftPackageTest(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "Package.swift", "// swift-tools-version: 5.9\n")
	writeImpactSurfaceFixture(t, root, "Sources/App/Widget.swift", "struct Widget {}\n")
	writeImpactSurfaceFixture(t, root, "Tests/AppTests/WidgetTests.swift", "final class WidgetTests {}\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-swift",
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:        "test_surface",
				Path:        "Sources/App/Widget.swift",
				RelatedPath: "Tests/AppTests/WidgetTests.swift",
				Priority:    50,
				Source:      "impact_engine",
			}},
		},
	}

	plans := impactRunnerPlansFromChangePlan(root, surface, plan)
	if len(plans) != 1 {
		t.Fatalf("expected one swift impact runner plan, got %+v surface=%+v", plans, surface.Candidates)
	}
	if plans[0].Runner != "swift" || plans[0].Suite != "WidgetTests" {
		t.Fatalf("unexpected swift impact plan: %+v", plans[0])
	}
}

func TestBuildRunCommandNormalizesPathSelectors(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "pom.xml", "<project/>\n")
	writeImpactSurfaceFixture(t, root, "build.gradle.kts", "plugins { kotlin(\"jvm\") version \"1.9.0\" }\n")

	javaCmd, _ := buildRunCommandWithFramework("java", "", "src/test/java/com/acme/WidgetTest.java", root, root)
	if !strings.Contains(javaCmd, `-Dtest="com.acme.WidgetTest"`) {
		t.Fatalf("java path selector should become class selector, got %q", javaCmd)
	}

	gradleRoot := t.TempDir()
	writeImpactSurfaceFixture(t, gradleRoot, "build.gradle.kts", "plugins { kotlin(\"jvm\") version \"1.9.0\" }\n")
	kotlinCmd, _ := buildRunCommandWithFramework("java", "", "src/test/kotlin/com/acme/WidgetTests.kt", gradleRoot, gradleRoot)
	if !strings.Contains(kotlinCmd, `--tests "com.acme.WidgetTests"`) {
		t.Fatalf("kotlin path selector should become Gradle class selector, got %q", kotlinCmd)
	}

	rustCmd, _ := buildRunCommandWithFramework("rust", "", "tests/widget.rs", root, root)
	if rustCmd != "cargo test --test widget" {
		t.Fatalf("rust integration-test path should become --test selector, got %q", rustCmd)
	}

	swiftCmd, _ := buildRunCommandWithFramework("swift", "", "Tests/AppTests/WidgetTests.swift", root, root)
	if swiftCmd != `swift test --filter "WidgetTests"` {
		t.Fatalf("swift test path should become basename filter, got %q", swiftCmd)
	}
}

func TestImpactRunnerPlansFromChangePlanConsumesObligationFallback(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "pyproject.toml", "[project]\nname='x'\n")
	writeImpactSurfaceFixture(t, root, "pkg/a.py", "def value():\n    return 1\n")
	writeImpactSurfaceFixture(t, root, "tests/test_a.py", "def test_value():\n    assert True\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-obligation",
		ImpactObligations: &types.ImpactObligationSet{
			PlanID: "plan-impact-obligation",
			Obligations: []types.ImpactObligation{{
				Kind:        "test_surface",
				Relation:    "related_test",
				Obligation:  "verify_related_test",
				SubjectPath: "pkg/a.py",
				RelatedPath: "tests/test_a.py",
				Source:      "impact_engine",
				Strength:    types.ImpactObligationStrengthInferred,
			}},
		},
	}

	plans := impactRunnerPlansFromChangePlan(root, surface, plan)
	if len(plans) != 1 || plans[0].Suite != "tests/test_a.py" {
		t.Fatalf("expected obligation fallback to produce related-test plan, got %+v", plans)
	}
}

func TestImpactRunnerPlansFromChangePlanConsumesAnalysisObligationSet(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "pyproject.toml", "[project]\nname='x'\n")
	writeImpactSurfaceFixture(t, root, "pkg/a.py", "def value():\n    return 1\n")
	writeImpactSurfaceFixture(t, root, "tests/test_a.py", "def test_value():\n    assert True\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-analysis-obligations",
		ImpactAnalysis: &types.ImpactAnalysisResult{
			PlanID: "plan-impact-analysis-obligations",
			ObligationSet: &types.ImpactObligationSet{
				PlanID: "plan-impact-analysis-obligations",
				Obligations: []types.ImpactObligation{{
					Kind:        "test_surface",
					Relation:    "related_test",
					Obligation:  "verify_related_test",
					SubjectPath: "pkg/a.py",
					RelatedPath: "tests/test_a.py",
					Source:      "impact_engine",
					Strength:    types.ImpactObligationStrengthInferred,
				}},
			},
		},
	}

	plans := impactRunnerPlansFromChangePlan(root, surface, plan)
	if len(plans) != 1 || plans[0].Suite != "tests/test_a.py" {
		t.Fatalf("expected analysis obligation set to produce related-test plan, got %+v", plans)
	}
}

func TestImpactRunnerPlansFromChangePlanRejectsUnsafeOrUnsupportedTargets(t *testing.T) {
	root := t.TempDir()
	writeImpactSurfaceFixture(t, root, "package.json", `{"scripts":{"test":"jest"}}`)
	writeImpactSurfaceFixture(t, root, "tests/a.test.js", "test('a', () => {})\n")

	surface := BuildTestSurface(root, "")
	plan := &types.ChangePlan{
		ID: "plan-impact-unsafe",
		ImpactAnalysis: &types.ImpactAnalysisResult{
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:        "test_surface",
				RelatedPath: "../escape.py",
				Priority:    50,
			}, {
				Kind:        "test_surface",
				RelatedPath: "tests/a.txt",
				Priority:    50,
			}},
		},
	}

	if plans := impactRunnerPlansFromChangePlan(root, surface, plan); len(plans) != 0 {
		t.Fatalf("unsafe path and unsupported selector should be ignored, got %+v", plans)
	}
}

func writeImpactSurfaceFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
