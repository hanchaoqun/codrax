package orchestrator

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Harness for the answer-request refusal tests: the real read scheduler with
// stub stages, a real non-TTY renderer and a captured operator log.

// refusedAnswerRequest is the typed error the answer stage returns when its
// first request is larger than it may send. The byte figure contains the
// digits 429 so a string probe for a rate-limit status would misread it.
func refusedAnswerRequest() error {
	return &agent.AnswerContextByteBudgetError{ActualBytes: 742912, AllocatedBytes: 700000}
}

type refusalRun struct {
	orchestrator   *Orchestrator
	bus            *types.BusContext
	finalizerCalls int
	extractorCalls int
	log            string
	rendered       string
	dumpDir        string
}

type refusalRunOptions struct {
	language string
	// extractor replaces the default extract stub.
	extractor func(call int, ctx *types.AgentContext) (*agent.StageOutput, error)
	// finalizer replaces the default "every request is refused" stub.
	finalizer func(call int, ctx *types.AgentContext) (*agent.StageOutput, error)
	// requireExtract makes the extract stage carry required work.
	requireExtract bool
	// repoFiles are written under a temporary repository root.
	repoFiles map[string]string
	// traceSideFile activates the root-cause side file for this run.
	traceSideFile bool
	// forcedAttempts overrides the forced dispatch's attempt budget (3).
	forcedAttempts int
	// configure adjusts the orchestrator before the run starts.
	configure func(o *Orchestrator, repo string)
	// explorer runs inside the explore stub, on the stage's own context.
	explorer func(ctx *types.AgentContext)
}

// runRefusalScenario drives the real read scheduler with stub stages. The
// answer stub refuses every request unless the options say otherwise.
func runRefusalScenario(t *testing.T, opts refusalRunOptions) *refusalRun {
	t.Helper()
	if opts.language == "" {
		opts.language = "zh"
	}
	if opts.forcedAttempts == 0 {
		opts.forcedAttempts = 3
	}
	run := &refusalRun{}
	ir := dagIR(types.AnswerContract{Language: opts.language})
	ir.RequestModel.Language = opts.language
	if opts.requireExtract {
		requireExtractAnswerSymbols(ir)
	}
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if opts.traceSideFile {
				ctx.Mutable.SetTraceFindingContract(&types.TraceFindingContract{RootCauseReportEnabled: true})
			}
			if opts.explorer != nil {
				opts.explorer(ctx)
			}
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{
					ID:           "ev",
					Source:       "src.go",
					LineStart:    1,
					AnchorKind:   types.AnchorDefinition,
					AnchorSymbol: "Source",
				}},
			}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			run.extractorCalls++
			if opts.extractor != nil {
				return opts.extractor(run.extractorCalls, ctx)
			}
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			run.finalizerCalls++
			if opts.finalizer != nil {
				return opts.finalizer(run.finalizerCalls, ctx)
			}
			return nil, refusedAnswerRequest()
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetForceFinalizeAttempts(opts.forcedAttempts)
	o.SetLanguage(opts.language)
	run.dumpDir = t.TempDir()
	o.SetOutputDump(run.dumpDir, 10)

	var rendered bytes.Buffer
	renderer := render.New(&rendered, false)
	renderer.SetLang(opts.language)
	o.SetEmitter(renderer.Emitter())

	readLog := captureOperatorLog(t)

	// A repository root without source files short-circuits before any stage.
	repo := t.TempDir()
	files := map[string]string{"src.go": "package src\n\nfunc Source() {}\n"}
	for name, body := range opts.repoFiles {
		files[name] = body
	}
	for name, body := range files {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	request := "解释 Source"
	if opts.language == "en" {
		request = "explain Source"
	}
	if opts.configure != nil {
		opts.configure(o, repo)
	}
	bus, err := o.Run(request, repo, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	run.orchestrator, run.bus = o, bus
	run.rendered = rendered.String()
	run.log = readLog()
	return run
}

// unsampleableBus returns a state the typed-input sample cannot read.
func unsampleableBus() *types.BusContext {
	bus := &types.BusContext{Mutable: types.NewMutableState("question")}
	makeUnsampleable(bus)
	return bus
}

// makeUnsampleable puts a sync.Map into a carrier the sample reads by content.
func makeUnsampleable(bus *types.BusContext) {
	bus.Mutable.SetRepairExecutionPlan(&sync.Map{})
}

// captureOperatorLog routes the operator log to a temporary directory for the
// rest of the test and returns a reader of what was written.
func captureOperatorLog(t *testing.T) func() string {
	t.Helper()
	dir := t.TempDir()
	lg, err := logging.NewFromFlags(dir, "debug", false)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	prev := logging.Default
	logging.SetDefault(lg)
	t.Cleanup(func() { logging.SetDefault(prev) })
	return func() string { return readRefusalRunLog(t, dir) }
}

// pendingLateRead queues a forced read the explore phase never consumed; the
// forced path drains it before dispatching.
func pendingLateRead(ctx *types.AgentContext) {
	ctx.Mutable.EvidenceClosure().AddPendingRead(types.PendingRead{
		File: "late.go", Rationale: "queued and never consumed", Origin: "test.pending",
	})
}

var lateReadRepo = map[string]string{"late.go": "package late\n\nfunc Late() {}\n"}

func readRefusalRunLog(t *testing.T, dir string) string {
	t.Helper()
	var all strings.Builder
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		all.Write(body)
		return nil
	})
	if err != nil {
		t.Fatalf("read run log: %v", err)
	}
	return all.String()
}

func lastLogLines(log string, n int) string {
	lines := strings.Split(strings.TrimRight(log, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (r *refusalRun) sideFileReason(t *testing.T) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(r.dumpDir, "*.root-causes.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("the run must publish exactly one root-cause side file: %v %v", files, err)
	}
	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var got outputdump.DefaultRootCauseArtifact
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "unavailable" || len(got.RootCauses) != 0 {
		t.Fatalf("a run without an answer publishes the empty artifact: %s", body)
	}
	return got.ReasonCode
}

// modelFacingDump renders what a stage is handed, for negative pins on text
// that must never reach a model.
func modelFacingDump(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	body, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("stage context must serialize: %v", err)
	}
	ir, err := json.Marshal(ctx.AnalysisIR)
	if err != nil {
		t.Fatalf("analysis must serialize: %v", err)
	}
	return ctx.Objective + "\n" + ctx.RetryHint + "\n" + string(body) + "\n" + string(ir)
}
