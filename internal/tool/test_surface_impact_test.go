package tool

import (
	"os"
	"path/filepath"
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
