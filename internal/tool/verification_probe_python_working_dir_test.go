package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// The planner probe and post-apply probe use the public verifier. The plan is
// emitted and applied through public tools; its actual committed diff is bound
// by the production patch-effect producer, not a hand-written line table. This
// is not a controller end-to-end test; no result/execution receipt is fabricated.
func TestPythonProbeWorkingDirPublicEmitApplyExecution(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git unavailable")
	}
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("Python unavailable")
	}
	for _, tc := range []struct {
		name, project, cwd, source, module string
		importOnly                         bool
	}{
		{name: "root", project: ".", cwd: ".", source: "widget.py", module: "widget"},
		{name: "nested", project: "packages/widget", cwd: "packages/widget", source: "packages/widget/widget.py", module: "widget"},
		{name: "internal_cwd_uses_project_root", project: "packages/widget", cwd: "packages/widget/tests", source: "packages/widget/widget.py", module: "widget"},
		{name: "nested_src_package", project: "packages/widget", cwd: "packages/widget/src/feature", source: "packages/widget/src/feature/counter.py", module: "feature.counter"},
		{name: "nested_lib_package", project: "packages/widget", cwd: "packages/widget", source: "packages/widget/lib/feature/counter.py", module: "feature.counter"},
		{name: "renamed", project: "components/clock", cwd: "components/clock", source: "components/clock/ticker.py", module: "ticker"},
		{name: "import_is_not_execution", project: "packages/widget", cwd: "packages/widget", source: "packages/widget/widget.py", module: "widget", importOnly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			const before = "def increment(value):\n    return value\n"
			writeSurfaceFile(t, root, ".gitignore", "__pycache__/\n*.pyc\n")
			writeSurfaceFile(t, root, filepath.Join(tc.project, "setup.py"), "from setuptools import setup\n")
			writeSurfaceFile(t, root, tc.source, before)
			if err := os.MkdirAll(filepath.Join(root, tc.cwd), 0o755); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(tc.module, ".") {
				writeSurfaceFile(t, root, filepath.Join(filepath.Dir(tc.source), "__init__.py"), "")
			}
			b1575FixtureGit(t, root, "init", "-q")
			b1575FixtureGit(t, root, "add", ".")
			b1575FixtureGit(t, root, "-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "baseline")
			ctx := newTestBusCtx()
			ctx.RepoRoot, ctx.MainRepoRoot = root, root
			ctx.Mode, ctx.PipelineStage = types.ModeApply, types.StagePlan
			ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
				Task:              types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "increment(2) returns 3"},
				BehaviorContracts: []types.WriteBehaviorContract{{ID: "value-result", Kind: types.WriteBehaviorObservable, Operator: types.WriteBehaviorOpEquals, Expected: "3", Required: true}},
			}})
			code := "from " + tc.module + " import increment\nassert increment(2) == 3"
			if tc.importOnly {
				code = "from " + tc.module + " import increment\nassert callable(increment)"
			}
			probe := types.VerificationProbe{ID: "actual-increment", Language: "python", WorkingDir: tc.cwd, Code: code,
				ContractRefs: []string{"value-result"}, ChangedSymbolRefs: []string{"path:" + tc.source}}
			_, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"dry_run": true, "verification_probe": probe}))
			if err != nil {
				t.Fatal(err)
			}
			baseline := ctx.Mutable.PlanStageProbeReports()
			if len(baseline) != 1 || len(baseline[0].ExecutedCommands) != 1 {
				t.Fatalf("HARNESS: actual baseline probe receipt missing: %+v", baseline)
			}
			command := baseline[0].ExecutedCommands[0]
			if command.Outcome != types.ExecutedCommandOutcomeExecuted || command.WorkingDir != tc.project || (command.ExitCode == 0) != tc.importOnly {
				t.Fatalf("HARNESS: unexpected real baseline execution: %+v", command)
			}
			if !tc.importOnly && (len(baseline[0].TestResults) != 1 || !strings.Contains(baseline[0].TestResults[0].FailureDetail, "AssertionError")) {
				t.Fatalf("HARNESS: baseline must call real source and fail its assertion, not fail import: %+v", baseline[0])
			}
			t.Logf("ACTUAL_BASELINE cwd=%s outcome=%s exit=%d", command.WorkingDir, command.Outcome, command.ExitCode)
			read, err := (&ReadFile{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"path": tc.source}))
			if err != nil || !read.Success {
				t.Fatalf("source read: %v %+v", err, read)
			}
			emitted, err := (&EmitChangePlan{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
				"request": "increment(2) returns 3", "summary": "Correct increment in the existing project and execute its real implementation.",
				"changes": []map[string]any{{"path": tc.source, "kind": "patch", "rationale": "return input plus one",
					"edits": []map[string]any{{"kind": "replace", "start_line": 2, "old_text": "    return value", "content": "    return value + 1"}}}},
				"verification_probes": []types.VerificationProbe{probe},
			}))
			if err != nil || !emitted.Success {
				t.Fatalf("PUBLIC_COUPLING: a runtime-valid project-local import must be admitted: %v %+v", err, emitted)
			}
			plan := ctx.Mutable.ChangePlan()
			if plan == nil || len(plan.VerificationProbes) != 1 || plan.VerificationProbes[0].Code != code || plan.VerificationProbes[0].WorkingDir != tc.cwd {
				t.Fatalf("probe definition changed: %+v", plan)
			}
			ctx.PipelineStage = types.StageApply
			applied, err := (&ApplyPatch{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"path": tc.source, "kind": "patch"}))
			if err != nil || !applied.Success || !ctx.Mutable.WriteClosure().HasApplied(tc.source) {
				t.Fatalf("actual apply: %v %+v", err, applied)
			}
			b1575FixtureGit(t, root, "add", "--", tc.source)
			b1575FixtureGit(t, root, "-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "actual apply")
			head := strings.TrimSpace(b1575FixtureGit(t, root, "rev-parse", "HEAD"))
			diff, err := worktree.CaptureCommitPatch(root, head)
			if err != nil {
				t.Fatal(err)
			}
			effect := writeflow.PatchEffectRecordFromUnifiedDiff(plan.ID, "", "applied_commit", head+"^", head, diff)
			plan.PatchEffect, plan.AppliedCommitSHA, plan.WorktreePath = &effect, head, root
			ctx.Mutable.SetChangePlan(plan)
			ctx.PipelineStage = types.StageVerify
			_, err = (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "python", "framework": "unittest", "working_dir": tc.project}))
			if err != nil {
				t.Fatal(err)
			}
			report := ctx.Mutable.ChangeReport()
			if report == nil || report.PlanID != plan.ID {
				t.Fatalf("post-apply report missing: %+v", report)
			}
			probePassed := false
			for _, row := range report.TestResults {
				if row.Suite == "verification_probe/python" && row.AssertionID == probe.ID {
					probePassed = row.Passed
				}
			}
			if !probePassed {
				t.Fatalf("actual post-apply probe failed: %+v", report)
			}
			// Restore persisted bytes before asking the existing receipt resolver.
			wire, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var restored types.ChangeReport
			if err := json.Unmarshal(wire, &restored); err != nil {
				t.Fatal(err)
			}
			resolution := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], &restored)
			executed := len(resolution.Paths) == 1 && resolution.Paths[0] == tc.source
			if executed == tc.importOnly {
				t.Fatalf("changed-line execution authority wrong: importOnly=%t resolution=%+v receipts=%s", tc.importOnly, resolution, b1575TargetReceiptsJSON(&restored))
			}
			if len(types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, restored.VerificationConfidence)) != 0 {
				t.Fatal("plain probe incorrectly gained per-contract behavior authority")
			}
			for _, row := range restored.ChangedPathCoverage {
				if row.Caliber == types.ChangedPathVerificationProbe && row.Capability == types.VerificationCapabilityTargetBehavior {
					t.Fatal("plain probe incorrectly gained target_behavior authority")
				}
			}
			t.Logf("ACTUAL_APPLY source=%s probe_passed=%t target_execution=%t import_only=%t receipts=%s", tc.source, probePassed, executed, tc.importOnly, b1575TargetReceiptsJSON(&restored))
		})
	}
}

func TestPythonProbeWorkingDirPublicRejectsUnrelatedImports(t *testing.T) {
	for _, tc := range []struct{ name, cwd, code string }{
		{"sibling_same_basename", "packages/other", "from widget import increment\nassert increment(2) == 3"},
		{"root_cannot_borrow_nested_alias", ".", "from widget import increment\nassert increment(2) == 3"},
		{"copy", "packages/widget", "def increment(value):\n    return value + 1\nassert increment(2) == 3"},
		{"comment", "packages/widget", "# from widget import increment\nassert 2 + 1 == 3"},
		{"string", "packages/widget", "note = 'from widget import increment'\nassert len(note) > 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			const source = "packages/widget/widget.py"
			for _, project := range []string{"packages/widget", "packages/other"} {
				writeSurfaceFile(t, root, project+"/setup.py", "")
				writeSurfaceFile(t, root, project+"/widget.py", "def increment(value):\n    return value\n")
			}
			ctx := newTestBusCtx()
			ctx.RepoRoot = root
			read, err := (&ReadFile{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"path": source}))
			if err != nil || !read.Success {
				t.Fatalf("source read: %v %s", err, read.Summary)
			}
			result, err := (&EmitChangePlan{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
				"request": "increment returns input plus one", "summary": "Repair the existing increment function.",
				"changes":             []map[string]any{{"path": source, "kind": "patch", "rationale": "correct increment", "edits": []map[string]any{{"kind": "replace", "start_line": 2, "old_text": "    return value", "content": "    return value + 1"}}}},
				"verification_probes": []types.VerificationProbe{{ID: "check", Language: "python", WorkingDir: tc.cwd, Code: tc.code}},
			}))
			if err != nil || result.Success || ctx.Mutable.ChangePlan() != nil {
				t.Fatalf("unrelated source must not gain coupling: %v %s", err, result.Summary)
			}
			if pack := mustPlanRepairPack(t, result); pack.ReasonCode != "verification_probe_coupling_failed" {
				t.Fatalf("unexpected rejection: %+v", pack)
			}
		})
	}
}

func TestPythonProbeWorkingDirCandidatesArePerProbe(t *testing.T) {
	root := t.TempDir()
	for _, project := range []string{"packages/left", "packages/right"} {
		writeSurfaceFile(t, root, project+"/setup.py", "")
		writeSurfaceFile(t, root, project+"/widget.py", "def increment(value):\n    return value\n")
	}
	changes := []types.FileChange{{Path: "packages/left/widget.py", Kind: "patch"}, {Path: "packages/right/widget.py", Kind: "patch"}}
	probes := []types.VerificationProbe{
		{ID: "left", Language: "python", WorkingDir: "packages/left", Code: "from widget import increment\nassert increment(2) == 3", ChangedSymbolRefs: []string{"symbol:left.increment"}},
		{ID: "right", Language: "python", WorkingDir: "packages/right", Code: "from widget import increment\nassert increment(2) == 3", ChangedSymbolRefs: []string{"symbol:right.increment"}},
	}
	got := enrichVerificationProbeChangedTargetRefsFromCoupling(root, changes, probes)
	for i, probe := range got {
		want := "path:" + changes[i].Path
		other := "path:" + changes[1-i].Path
		if !containsString(probe.ChangedSymbolRefs, want) || containsString(probe.ChangedSymbolRefs, other) || !containsString(probe.ChangedSymbolRefs, probes[i].ChangedSymbolRefs[0]) {
			t.Fatalf("per-probe unique owner or authored symbol lost: %+v", got)
		}
	}
	// No probe imports from its own effective root. A union of aliases across
	// probes would incorrectly admit this plan, even though neither matches.
	writeSurfaceFile(t, root, "packages/left/alpha.py", "VALUE = 1\n")
	writeSurfaceFile(t, root, "packages/right/beta.py", "VALUE = 1\n")
	changes = []types.FileChange{{Path: "packages/left/alpha.py", Kind: "patch"}, {Path: "packages/right/beta.py", Kind: "patch"}}
	probes[0].Code, probes[1].Code = "import beta\nassert beta.VALUE == 1", "import alpha\nassert alpha.VALUE == 1"
	ctx := newTestBusCtx()
	ctx.RepoRoot = root
	if got := validateVerificationProbeCoupling(ctx, changes, probes); got == "" {
		t.Fatal("a different probe's working directory supplied coupling")
	}
}

func TestPythonProbeWorkingDirAliasSafety(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeSurfaceFile(t, root, "packages/widget/setup.py", "")
	writeSurfaceFile(t, root, "packages/widget/widget.py", "VALUE = 1\n")
	writeSurfaceFile(t, outside, "setup.py", "")
	writeSurfaceFile(t, outside, "outside.py", "VALUE = 1\n")
	for _, pair := range [][2]string{{outside, "external"}, {filepath.Join(outside, "outside.py"), "packages/widget/link.py"}, {filepath.Join(outside, "missing"), "packages/widget/dangling"}} {
		if err := os.Symlink(pair[0], filepath.Join(root, pair[1])); err != nil {
			t.Fatal(err)
		}
	}
	var provider verificationProbeCouplingProvider
	for _, candidate := range verificationProbeCouplingProviders {
		if candidate.Language == "python" {
			provider = candidate
		}
	}
	for _, tc := range []struct{ name, cwd, path, kind, forbidden string }{
		{"escaping_cwd", "../elsewhere", "packages/widget/widget.py", "patch", "widget"},
		{"absolute_cwd", filepath.Join(root, "packages/widget"), "packages/widget/widget.py", "patch", "widget"},
		{"external_cwd", "external", "external/outside.py", "patch", "outside"},
		{"external_source", "packages/widget", "packages/widget/link.py", "patch", "link"},
		{"external_create", "external", "external/new.py", "create", "new"},
		{"dangling_create", "packages/widget", "packages/widget/dangling/new.py", "create", "dangling.new"},
		{"missing_not_create", "packages/widget", "packages/widget/absent.py", "patch", "absent"},
		{"sibling_create", "packages/widget", "packages/other/absent.py", "create", "absent"},
		{"invalid_identifier", "packages/widget", "packages/widget/not-valid/new.py", "create", "not-valid.new"},
		{"literal_dot_directory", "packages/widget", "packages/widget/not.a.package/new.py", "create", "not.a.package.new"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := verificationProbeCouplingTargets(root, []types.FileChange{{Path: tc.path, Kind: tc.kind}}, types.VerificationProbe{WorkingDir: tc.cwd}, provider)
			if _, exists := got[tc.forbidden]; exists {
				t.Fatalf("unsafe new alias %q: %v", tc.forbidden, got)
			}
		})
	}
}

func TestPythonProbeWorkingDirPublicCreate(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git unavailable")
	}
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("Python unavailable")
	}
	for _, cwd := range []string{"packages/widget", "packages/other"} {
		t.Run(cwd, func(t *testing.T) {
			root := t.TempDir()
			for _, project := range []string{"packages/widget", "packages/other"} {
				writeSurfaceFile(t, root, project+"/setup.py", "")
			}
			b1575FixtureGit(t, root, "init", "-q")
			b1575FixtureGit(t, root, "add", ".")
			b1575FixtureGit(t, root, "-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "baseline")
			ctx := newTestBusCtx()
			ctx.RepoRoot, ctx.MainRepoRoot, ctx.Mode = root, root, types.ModeApply
			const source = "packages/widget/newpkg/counter.py"
			probe := types.VerificationProbe{ID: "created", Language: "python", WorkingDir: cwd, Code: "from newpkg.counter import increment\nassert increment(2) == 3"}
			result, err := (&EmitChangePlan{}).Execute(ctx, runTestsJSONParams(t, map[string]any{
				"request": "add increment", "summary": "Add an increment function to the widget project.",
				"changes":             []map[string]any{{"path": source, "kind": "create", "new_content": "def increment(value):\n    return value + 1\n", "rationale": "new increment function"}},
				"verification_probes": []types.VerificationProbe{probe},
			}))
			if cwd == "packages/other" {
				if err != nil || result.Success || !strings.Contains(result.Summary, "changed Python production module") {
					t.Fatalf("sibling create gained coupling: %v %s", err, result.Summary)
				}
				return
			}
			if err != nil || !result.Success {
				t.Fatalf("nested create rejected: %v %s", err, result.Summary)
			}
			plan := ctx.Mutable.ChangePlan()
			if !containsString(plan.VerificationProbes[0].ChangedSymbolRefs, "path:"+source) {
				t.Fatalf("created source not enriched: %+v", plan.VerificationProbes)
			}
			ctx.PipelineStage = types.StageApply
			applied, err := (&ApplyPatch{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"path": source, "kind": "create"}))
			if err != nil || !applied.Success || !ctx.Mutable.WriteClosure().HasApplied(source) {
				t.Fatalf("actual create: %v %s", err, applied.Summary)
			}
			b1575FixtureGit(t, root, "add", "--", source)
			b1575FixtureGit(t, root, "-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "actual create")
			head := strings.TrimSpace(b1575FixtureGit(t, root, "rev-parse", "HEAD"))
			diff, err := worktree.CaptureCommitPatch(root, head)
			if err != nil {
				t.Fatal(err)
			}
			effect := writeflow.PatchEffectRecordFromUnifiedDiff(plan.ID, "", "applied_commit", head+"^", head, diff)
			plan.PatchEffect, plan.AppliedCommitSHA, plan.WorktreePath = &effect, head, root
			ctx.Mutable.SetChangePlan(plan)
			ctx.PipelineStage = types.StageVerify
			_, err = (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "python", "framework": "unittest", "working_dir": cwd}))
			if err != nil {
				t.Fatal(err)
			}
			report := ctx.Mutable.ChangeReport()
			resolution := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], report)
			if !containsString(resolution.Paths, source) {
				t.Fatalf("created source did not really execute: %+v receipts=%s", resolution, b1575TargetReceiptsJSON(report))
			}
		})
	}
}
