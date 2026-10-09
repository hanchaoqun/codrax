package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// The public navigation calls use a genuinely loaded parent graph and a scoped
// projection before planning. Apply then changes the physical repository root.
func TestWriteGraphRootPublicScopedNavigationApplyAndNativeReceipt(t *testing.T) {
	for _, binary := range []string{"git", "python3"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skip(binary + " unavailable")
		}
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		".gitignore":                             "__pycache__/\n*.pyc\n.codrax/\n",
		"packages/widget/setup.py":               "from setuptools import setup\nsetup(name='widget', version='0.1', py_modules=['widget'])\n",
		"packages/widget/widget.py":              "def increment(value):\n    return value\n",
		"packages/widget/tests/__init__.py":      "",
		"packages/widget/tests/test_widget.py":   "import unittest\nfrom widget import increment\nclass IncrementTest(unittest.TestCase):\n    def test_value(self): self.assertEqual(increment(0), 1)\n",
		"packages/other/tests/test_unrelated.py": "def unrelated():\n    return 1\n",
	} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string { return runGitForWorkflowRestoreTest(t, root, args...) }
	git("init", "-q")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.invalid")
	git("add", ".")
	git("-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "before")
	mg, err := repomap.BuildOrLoadMultiGraph(&topology.RepoTopology{ParentRoot: root, Repos: []topology.SubRepo{{Slug: "root", RootAbs: root, RootRel: "."}}}, "", 2, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("fix increment and run its existing tests")
	ctx := &types.BusContext{Mutable: mu, RepoRoot: root, MainRepoRoot: root, MultiGraph: mg, WorkDir: t.TempDir(), Mode: types.ModeApply, PipelineStage: types.StagePlan}
	for _, params := range []string{`{"path":".","view":"file_map"}`, `{"path":"packages/widget","view":"source_inventory","scope":".","roles":["function","method","type"]}`} {
		result, err := (&repomap.RepoMapV2{}).Execute(ctx, json.RawMessage(params))
		if err != nil || !result.Success {
			t.Fatalf("public navigation failed: %+v %v", result, err)
		}
	}
	if g, ok := mu.SearchGraph().(*repomap.Graph); !ok || g == nil || g.Root != root || g.FileIndex[relatedNativeTestPath] == nil {
		t.Errorf("scoped navigation replaced canonical graph: %#v", mu.SearchGraph())
	}
	controllerRegistrationTool(t, ctx, (&tool.EmitChangePlan{}).Execute, map[string]any{"request": "correct increment", "summary": "Return the next integer.", "changes": []map[string]any{{"path": "packages/widget/widget.py", "kind": "modify", "new_content": "def increment(value):\n    return value + 1\n", "rationale": "fix result"}}})
	plan := mu.ChangePlan()
	session, err := worktree.Create(t.TempDir(), root, "graph-root-public")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Discard() })
	ctx.RepoRoot, ctx.WorktreePath, ctx.PipelineStage = session.Path(), session.Path(), types.StageApply
	plan.WorktreePath = session.Path()
	mu.SetChangePlan(plan)
	controllerRegistrationTool(t, ctx, (&tool.ApplyPatch{}).Execute, map[string]any{"path": "packages/widget/widget.py", "kind": "modify"})
	sha, err := worktree.CommitChanges(session.Path(), "graph-root applied")
	if err != nil {
		t.Fatal(err)
	}
	plan.AppliedCommitSHA = sha
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx, o.currentIterCommitSHA = ctx, sha
	if o.attachActivePatchEffectRecord(plan, types.ChangePlanSlice{ID: "slice-1", Paths: []string{"packages/widget/widget.py"}}) == nil {
		t.Fatal("no applied effect")
	}
	plan = mu.ChangePlan()
	found := false
	for _, target := range plan.ImpactAnalysis.VerificationTargets {
		if target.Kind == "test_surface" {
			found = true
			if target.RelatedPath != relatedNativeTestPath {
				t.Errorf("graph-local path became repository obligation: %+v", target)
			}
		}
	}
	if !found {
		t.Error("actual graph-derived related test was lost")
	}
	ctx.PipelineStage = types.StageVerify
	result, err := (&tool.RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	report := mu.ChangeReport()
	if err != nil || !result.Success || report == nil || !report.Passed {
		t.Fatalf("native verification failed: %+v %v", report, err)
	}
	applyVerifyCoverageToChangePlan(plan, report, nil)
	for _, target := range plan.ImpactAnalysis.VerificationTargets {
		if target.Kind == "test_surface" && target.CoverageStatus != "verified" {
			t.Errorf("precise file receipt not consumed: %+v", target)
		}
	}
	paths := types.VerifiedNativeTestExecutionPaths(plan, report)
	if len(paths) != 1 || paths[0] != relatedNativeTestPath {
		t.Fatalf("unexpected current native receipt paths: %v", paths)
	}
	for _, path := range paths {
		if strings.HasPrefix(path, "packages/other/") {
			t.Fatal("sibling borrowed receipt")
		}
	}
	// The same coordinate contract is used by convention learning and the
	// later cumulative review; neither may reintroduce child-local paths.
	o.reportDir = t.TempDir()
	if err := types.WritePlanToFile(plan, filepath.Join(o.reportDir, writeWorkflowArtifactFileStem(plan.ID)+".json")); err != nil {
		t.Fatal(err)
	}
	run := &types.WriteWorkflowRun{RunID: "graph-root-public", ActiveBatchID: "batch-1", Batches: []types.WriteWorkflowBatch{{ID: "batch-1", Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: plan.ID, ArtifactRef: sha}}}}}
	conventions := o.conventionGraphForPatchReview(run, plan)
	foundConvention := false
	if conventions != nil {
		for _, node := range conventions.Nodes {
			if node.Source == relatedNativeTestPath && node.Subject == "packages/widget/widget.py" {
				foundConvention = true
			}
		}
	}
	if !foundConvention {
		t.Fatal("convention consumer lost repository-root test coordinates")
	}
	cumulative := o.buildCumulativePatchReviewPlan(run, report, nil)
	if cumulative == nil || cumulative.ImpactAnalysis == nil {
		t.Fatal("missing actual cumulative review")
	}
	foundCumulative := false
	for _, target := range cumulative.ImpactAnalysis.VerificationTargets {
		if target.Kind != "test_surface" {
			continue
		}
		foundCumulative = true
		if target.RelatedPath != relatedNativeTestPath || target.CoverageStatus != "verified" {
			t.Errorf("cumulative graph coordinate/receipt mismatch: %+v", target)
		}
	}
	if !foundCumulative {
		t.Fatal("cumulative consumer lost graph-derived test obligation")
	}
}
