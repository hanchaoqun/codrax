package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// This is an executor protocol fixture, not a model-authored execution claim.
// Each case runs the public verifier and inspects its persisted, JSON-restored
// report. The changed-line table is deliberately narrower than its hunk.
func TestB1575PythonTargetExecutionPublicMatrix(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable Python")
	}
	for _, tc := range []struct {
		name, code string
		lines      []int
		executed   bool
	}{
		{"import only", "import widget\nassert widget.Client\n", []int{7, 9}, false},
		{"AST only", "import widget, ast\nfrom pathlib import Path\nassert isinstance(ast.parse(Path('widget.py').read_text()), ast.Module)\n", []int{7, 9}, false},
		{"construction only", "import widget\nassert widget.Client().ready\n", []int{7, 9}, false},
		{"synchronous call", "import widget\nassert widget.Client().sync_value() == 42\n", []int{7}, true},
		{"coroutine created", "import widget\nc = widget.Client().async_value()\nassert c is not None\nc.close()\n", []int{9}, false},
		{"coroutine awaited", "import widget, asyncio\nassert asyncio.run(widget.Client().async_value()) == 43\n", []int{9}, true},
		{"unchanged method", "import widget\nassert widget.Client().unchanged() == 1\n", []int{7}, false},
		{"one changed owner missing", "import widget\nassert widget.Client().sync_value() == 42\n", []int{7, 9}, false},
		{"both changed owners", "import widget, asyncio\nx = widget.Client()\nassert x.sync_value() == 42\nassert asyncio.run(x.async_value()) == 43\n", []int{7, 9}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, plan := b1575PythonExecutionContext(t, tc.code, tc.lines)
			result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "python", "framework": "unittest"}))
			if err != nil {
				t.Fatal(err)
			}
			report := ctx.Mutable.ChangeReport()
			if report == nil {
				t.Fatalf("no public report: %+v", result)
			}
			probePassed := false
			for _, row := range report.TestResults {
				if row.Suite == "verification_probe/python" && row.AssertionID == "target-probe" {
					probePassed = row.Passed
				}
			}
			if !probePassed {
				t.Fatalf("fixture must genuinely finish its probe: %+v", report)
			}
			encoded, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var restored types.ChangeReport
			if err := json.Unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			resolution := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], &restored)
			if (len(resolution.Paths) == 1 && resolution.Paths[0] == "widget.py") != tc.executed {
				t.Errorf("runtime receipt resolution=%+v want target=%v; receipts=%s", resolution, tc.executed, b1575TargetReceiptsJSON(&restored))
			}
			pathExecuted := false
			for _, row := range restored.ChangedPathCoverage {
				if row.Path != "widget.py" || row.Caliber != types.ChangedPathVerificationProbe {
					continue
				}
				if row.Capability == types.VerificationCapabilityTargetBehavior {
					t.Error("a process/target execution receipt must not authorize runtime behavior contracts")
				}
				pathExecuted = pathExecuted || row.Capability == types.VerificationCapabilityTargetExecution
			}
			if pathExecuted != tc.executed {
				t.Errorf("actual target execution=%v want=%v; coverage=%+v", pathExecuted, tc.executed, restored.ChangedPathCoverage)
			}
			if tc.executed && !bytes.Contains(encoded, []byte(`"target_execution":`)) {
				t.Error("actual public report lost target execution receipt")
			}
			if refs := types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, restored.VerificationConfidence); len(refs) != 0 {
				t.Errorf("target execution alone signed runtime contract refs: %v", refs)
			}
			ledger := types.BuildVerificationProofLedger(plan, &restored, nil)
			profile := types.BuildVerificationProofProfile(plan, &restored)
			if profile.TargetBehaviorPaths != 0 {
				t.Errorf("proof profile retained unsupported behavior capability: %+v", profile)
			}
			for _, obligation := range ledger.Obligations {
				if obligation.ContractRef == "value-result" && obligation.Status == types.VerificationProofLedgerItemCovered {
					t.Errorf("JSON-restored ledger signed unobserved runtime contract: %+v", obligation)
				}
			}
			after, _ := json.Marshal(&restored)
			if !bytes.Equal(encoded, after) {
				t.Error("proof projection mutated the persisted report")
			}
		})
	}
}

func b1575TargetReceiptsJSON(report *types.ChangeReport) string {
	var receipts []*types.VerificationProbeTargetExecutionReceipt
	for _, command := range report.ExecutedCommands {
		if command.ProbeExecution != nil {
			receipts = append(receipts, command.ProbeExecution.TargetExecution)
		}
	}
	data, _ := json.Marshal(receipts)
	return string(data)
}

func TestB1575PythonTargetExecutionPublicDefaultDefinition(t *testing.T) {
	for _, language := range []string{"", "py", "python"} {
		t.Run("language="+language, func(t *testing.T) {
			ctx, plan := b1575PythonExecutionSourceContext(t, "def increment(value):\n    return value + 1\n", "import widget\nassert widget.increment(2) == 3\n", []int{2})
			plan.VerificationProbes[0].Language = language
			ctx.Mutable.SetChangePlan(plan)
			report := b1575PublicPythonReport(t, ctx)
			resolution := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], report)
			if len(resolution.Paths) != 1 || !report.HasTargetExecutionCoverage() {
				t.Fatalf("default declaration did not retain actual execution: resolution=%+v receipts=%s", resolution, b1575TargetReceiptsJSON(report))
			}
		})
	}
}

func b1575PythonExecutionContext(t *testing.T, code string, lines []int) (*types.BusContext, *types.ChangePlan) {
	t.Helper()
	source := "class Client:\n    def __init__(self):\n        self.ready = True\n    def unchanged(self):\n        return 1\n    def sync_value(self):\n        return 42\n    async def async_value(self):\n        return 43\n"
	return b1575PythonExecutionSourceContext(t, source, code, lines)
}

func b1575PythonExecutionSourceContext(t *testing.T, source, code string, lines []int) (*types.BusContext, *types.ChangePlan) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := &types.ChangePlan{
		ID: "plan-python-execution", Status: types.PlanStatusApplied, TargetPaths: []string{"widget.py"},
		Changes:            []types.FileChange{{Path: "widget.py", Kind: "modify", NewContent: source}},
		BehaviorContracts:  []types.WriteBehaviorContract{{ID: "value-result", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpEquals, Expected: "42", Required: true, Source: "write_analyzer"}},
		VerificationProbes: []types.VerificationProbe{{ID: "target-probe", Language: "python", Code: code, ContractRefs: []string{"value-result"}, ChangedSymbolRefs: []string{"path:widget.py"}}},
	}
	mu := types.NewMutableState("target execution protocol fixture")
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
	b1575BindAppliedPythonLines(t, ctx, plan, "widget.py", lines)
	return ctx, plan
}

// Only for a newly created test repository. Build a real before/applied pair
// and obtain the exact line/text table from the production diff producer.
func b1575BindAppliedPythonLines(t *testing.T, ctx *types.BusContext, plan *types.ChangePlan, path string, lines []int) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(ctx.RepoRoot, ".git")); !os.IsNotExist(err) {
		t.Fatal("applied-source fixture expects a fresh temporary repository")
	}
	full := filepath.Join(ctx.RepoRoot, filepath.FromSlash(path))
	current, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	before := strings.Split(string(current), "\n")
	for _, line := range lines {
		if line < 1 || line > len(before) {
			t.Fatal("invalid fixture added line")
		}
		before[line-1] += " # previous source"
	}
	if err := os.WriteFile(full, []byte(strings.Join(before, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	b1575FixtureGit(t, ctx.RepoRoot, "init", "-q")
	b1575FixtureGit(t, ctx.RepoRoot, "add", "--all")
	b1575FixtureGit(t, ctx.RepoRoot, "-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "before")
	if err := os.WriteFile(full, current, 0o644); err != nil {
		t.Fatal(err)
	}
	b1575FixtureGit(t, ctx.RepoRoot, "add", "--", path)
	b1575FixtureGit(t, ctx.RepoRoot, "-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "applied")
	head := strings.TrimSpace(b1575FixtureGit(t, ctx.RepoRoot, "rev-parse", "HEAD"))
	diff, err := worktree.CaptureCommitPatch(ctx.RepoRoot, head)
	if err != nil {
		t.Fatal(err)
	}
	effect := writeflow.PatchEffectRecordFromUnifiedDiff(plan.ID, "", "applied_commit", head+"^", head, diff)
	plan.PatchEffect, plan.AppliedCommitSHA, plan.WorktreePath = &effect, head, ctx.RepoRoot
	ctx.Mutable.SetChangePlan(plan)
}

func b1575FixtureGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func TestB1575PythonTargetExecutionPublicIntegrity(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable Python")
	}
	for _, tc := range []struct {
		name, code string
		edit       func(*types.ChangePlan)
	}{
		{"hook disabled", "import widget, sys\nsys.settrace(None)\nassert widget.Client().sync_value() == 42\n", nil},
		{"hook replaced and restored", "import widget, sys\nh = sys.gettrace()\nsys.settrace(None)\nassert widget.Client().sync_value() == 42\nsys.settrace(h)\n", nil},
		{"profile replaced", "import widget, sys\nsys.setprofile(None)\nassert widget.Client().sync_value() == 42\n", nil},
		{"child execution", "import widget, subprocess, sys\nassert widget.Client().sync_value() == 42\nsubprocess.run([sys.executable, '-c', 'pass'], check=True)\n", nil},
		{"thread execution", "import widget, threading\nt=threading.Thread(target=widget.Client().sync_value)\nt.start()\nt.join()\n", nil},
		// Protocol simulation on older Python: exercises runtime builtin
		// identity dispatch, not a claim that this machine ran Python 3.13.
		{"joinable builtin protocol", "import widget, _thread, time\n_thread.start_joinable_thread = time.sleep\nassert widget.Client().sync_value() == 42\n_thread.start_joinable_thread(0)\n", nil},
		{"source changed", "import widget\nfrom pathlib import Path\nassert widget.Client().sync_value() == 42\np=Path('widget.py')\np.write_text(p.read_text()+'# later change\\n')\n", nil},
		{"filename spoof", "import widget\nns={}\nexec(compile('def sync_value():\\n    return 42\\n', widget.__file__, 'exec'), ns)\nassert ns['sync_value']() == 42\n", nil},
		{"wrong plan effect", "import widget\nassert widget.Client().sync_value() == 42\n", func(p *types.ChangePlan) { p.PatchEffect.PlanID = "other" }},
		{"missing effect", "import widget\nassert widget.Client().sync_value() == 42\n", func(p *types.ChangePlan) { p.PatchEffect = nil }},
		{"old path cannot bind", "import widget\nassert widget.Client().sync_value() == 42\n", func(p *types.ChangePlan) {
			p.PatchEffect.Files[0].OldPath = "widget.py"
			p.PatchEffect.Files[0].Path = "other.py"
		}},
		{"context lines cannot bind", "import widget\nassert widget.Client().sync_value() == 42\n", func(p *types.ChangePlan) { p.PatchEffect.Files[0].Hunks[0].AddedLineNumbers = nil }},
		{"pure deletion unknown", "import widget\nassert widget.Client().sync_value() == 42\n", func(p *types.ChangePlan) {
			p.PatchEffect.Files[0].Hunks = append(p.PatchEffect.Files[0].Hunks, types.PatchEffectHunk{RemovedLines: 1, NewStart: 2, NewLines: 5})
		}},
		{"event overflow", "import widget\nassert widget.Client().sync_value() == 42\nfor i in range(600000):\n    pass\n", nil},
		{"missing observer terminal", "import widget, os\nassert widget.Client().sync_value() == 42\nos._exit(0)\n", nil},
		{"missing line texts", "import widget\nassert widget.Client().sync_value() == 42\n", func(p *types.ChangePlan) { p.PatchEffect.Files[0].Hunks[0].AddedLineTexts = nil }},
		{"wrong line text", "import widget\nassert widget.Client().sync_value() == 42\n", func(p *types.ChangePlan) { p.PatchEffect.Files[0].Hunks[0].AddedLineTexts[0].Text = "return 42" }},
		{"line text wrong location", "import widget\nassert widget.Client().sync_value() == 42\n", func(p *types.ChangePlan) { p.PatchEffect.Files[0].Hunks[0].AddedLineTexts[0].Line = 5 }},
		{"unbound diff fingerprint", "import widget\nassert widget.Client().sync_value() == 42\n", func(p *types.ChangePlan) { p.PatchEffect.DiffFingerprint = strings.Repeat("a", 64) }},
		{"unbound head", "import widget\nassert widget.Client().sync_value() == 42\n", func(p *types.ChangePlan) { p.PatchEffect.HeadRef = strings.Repeat("a", 40) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, plan := b1575PythonExecutionContext(t, tc.code, []int{7})
			if tc.edit != nil {
				tc.edit(plan)
				ctx.Mutable.SetChangePlan(plan)
			}
			report := b1575PublicPythonReport(t, ctx)
			resolution := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], report)
			if len(resolution.Paths) != 0 {
				t.Fatalf("invalid observation authorized execution: %+v", resolution)
			}
		})
	}
}

func TestB1575PythonTargetExecutionPublicAppliedSourceBinding(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable Python")
	}
	for _, tc := range []struct {
		name                                                                        string
		cumulative, changedCurrent, changedHead, badTextRoster, duplicateTextRoster bool
	}{
		{name: "exact applied source"},
		{name: "same added text different current owner", changedCurrent: true},
		{name: "same added text different committed owner", changedCurrent: true, changedHead: true},
		{name: "complete two-owner source roster"},
		{name: "partial added text roster", badTextRoster: true},
		{name: "duplicate added text roster", duplicateTextRoster: true},
		{name: "cumulative current HEAD", cumulative: true},
		{name: "cumulative moved HEAD", cumulative: true, changedCurrent: true, changedHead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, plan := b1575PythonExecutionContext(t, "import widget, asyncio\nx=widget.Client()\nassert x.sync_value() == 42\nassert asyncio.run(x.async_value()) == 43\n", []int{7, 9})
			if tc.cumulative {
				base := plan.PatchEffect.BaseRef
				diff, err := worktree.CaptureRangePatchForPaths(ctx.RepoRoot, base, "HEAD", []string{"widget.py"})
				if err != nil {
					t.Fatal(err)
				}
				effect := writeflow.PatchEffectRecordFromUnifiedDiff(plan.ID, "", "workflow_cumulative_owned_diff", base, "HEAD", diff)
				plan.PatchEffect = &effect
			}
			if tc.badTextRoster {
				plan.PatchEffect.Files[0].Hunks[0].AddedLineTexts = plan.PatchEffect.Files[0].Hunks[0].AddedLineTexts[:1]
			}
			if tc.duplicateTextRoster {
				texts := plan.PatchEffect.Files[0].Hunks[0].AddedLineTexts
				texts[1] = texts[0]
			}
			if tc.changedCurrent {
				path := filepath.Join(ctx.RepoRoot, "widget.py")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = bytes.Replace(data, []byte("def sync_value("), []byte("def replacement("), 1)
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
				plan.VerificationProbes[0].Code = strings.ReplaceAll(plan.VerificationProbes[0].Code, ".sync_value()", ".replacement()")
				if tc.changedHead {
					b1575FixtureGit(t, ctx.RepoRoot, "add", "--", "widget.py")
					b1575FixtureGit(t, ctx.RepoRoot, "-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "different owner")
				}
			}
			ctx.Mutable.SetChangePlan(plan)
			report := b1575PublicPythonReport(t, ctx)
			resolution := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], report)
			want := !tc.changedCurrent && !tc.badTextRoster && !tc.duplicateTextRoster
			if (len(resolution.Paths) == 1) != want {
				t.Fatalf("applied source binding=%+v want=%v receipts=%s", resolution, want, b1575TargetReceiptsJSON(report))
			}
		})
	}
}

func TestB1575PythonTargetExecutionPublicOwnerRoles(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable Python")
	}
	for _, tc := range []struct {
		name, source, code string
		line               int
		executed           bool
	}{
		{"module initialized", "VALUE = 42\n", "import widget\nassert widget.VALUE == 42\n", 1, true},
		{"class initialized", "class Client:\n    FLAG = 42\n", "import widget\nassert widget.Client.FLAG == 42\n", 2, true},
		{"definition is not invocation", "def value():\n    return 42\n", "import widget\nassert widget.value\n", 1, false},
		{"changed signature invoked", "def value():\n    return 42\n", "import widget\nassert widget.value() == 42\n", 1, true},
		{"nested definition not invoked", "def outer():\n    def inner():\n        return 42\n    return inner\n", "import widget\nassert callable(widget.outer())\n", 3, false},
		{"nested invoked", "def outer():\n    def inner():\n        return 42\n    return inner\n", "import widget\nassert widget.outer()() == 42\n", 3, true},
		{"generator created", "def values():\n    yield 42\n", "import widget\nx=widget.values()\nx.close()\n", 2, false},
		{"generator advanced", "def values():\n    yield 42\n", "import widget\nassert next(widget.values()) == 42\n", 2, true},
		{"lambda ownership unknown", "def value():\n    return lambda: 42\n", "import widget\nassert widget.value()() == 42\n", 2, false},
		{"decorated function invoked", "def deco(fn):\n    return fn\n@deco\ndef value():\n    return 42\n", "import widget\nassert widget.value() == 42\n", 5, true},
		{"same names different owners", "class A:\n    def value(self):\n        return 42\nclass B:\n    def value(self):\n        return 43\n", "import widget\nassert widget.B().value() == 43\n", 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, plan := b1575PythonExecutionSourceContext(t, tc.source, tc.code, []int{tc.line})
			report := b1575PublicPythonReport(t, ctx)
			resolution := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], report)
			if (len(resolution.Paths) == 1) != tc.executed {
				var receipt *types.VerificationProbeTargetExecutionReceipt
				for _, cmd := range report.ExecutedCommands {
					if cmd.ProbeExecution != nil {
						receipt = cmd.ProbeExecution.TargetExecution
					}
				}
				t.Fatalf("role observation=%+v want=%v receipt=%+v", resolution, tc.executed, receipt)
			}
		})
	}
}

func b1575PublicPythonReport(t *testing.T, ctx *types.BusContext) *types.ChangeReport {
	t.Helper()
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "python", "framework": "unittest"}))
	if err != nil {
		t.Fatal(err)
	}
	report := ctx.Mutable.ChangeReport()
	if report == nil {
		t.Fatalf("missing public report: %+v", result)
	}
	probePassed := false
	for _, row := range report.TestResults {
		if row.Suite == "verification_probe/python" && row.AssertionID == "target-probe" {
			probePassed = row.Passed
		}
	}
	if !probePassed {
		t.Fatalf("original process result changed: %+v", report.TestResults)
	}
	data, _ := json.Marshal(report)
	var restored types.ChangeReport
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	return &restored
}

func TestB1575PythonTargetExecutionSetupFailurePreservesProbe(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable Python")
	}
	for _, tc := range []struct {
		name               string
		passed, priorHooks bool
	}{
		{"original pass", true, false}, {"original assertion failure", false, false}, {"original hooks", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := "import widget, sys\nassert widget.Client().sync_value() == 42\n"
			if tc.priorHooks {
				code += "import sitecustomize\nassert sys.gettrace() is sitecustomize.original_trace\nassert sys.getprofile() is sitecustomize.original_profile\n"
			} else {
				code += "assert sys.gettrace() is None\nassert sys.getprofile() is None\n"
			}
			if !tc.passed {
				code += "assert False, 'original-probe-failure'\n"
			}
			ctx, plan := b1575PythonExecutionContext(t, code, []int{7})
			if tc.priorHooks {
				startup := "import sys\ndef original_trace(frame, event, arg):\n    return original_trace\ndef original_profile(frame, event, arg):\n    pass\nsys.settrace(original_trace)\nsys.setprofile(original_profile)\n"
				if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "sitecustomize.py"), []byte(startup), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			// A repository module shadows an observer-only stdlib import. The
			// original probe does not import it and must keep its own outcome.
			if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "ast.py"), []byte("raise RuntimeError('observer-setup-only')\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "python", "framework": "unittest"}))
			if err != nil {
				t.Fatal(err)
			}
			report := ctx.Mutable.ChangeReport()
			found := false
			for _, row := range report.TestResults {
				if row.Suite != "verification_probe/python" || row.AssertionID != "target-probe" {
					continue
				}
				found = true
				if row.Passed != tc.passed || (!tc.passed && !strings.Contains(row.FailureDetail, "original-probe-failure")) {
					t.Errorf("observer setup replaced the real probe outcome: %+v", row)
				}
			}
			if !found {
				t.Fatal("missing original probe result")
			}
			if resolution := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], report); len(resolution.Paths) != 0 {
				t.Fatalf("failed observer setup granted execution: %+v", resolution)
			}
		})
	}
}
