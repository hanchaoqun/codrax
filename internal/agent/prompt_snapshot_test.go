package agent

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	agentctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPromptSnapshot_NoInternalTermsInRenderedOutput is the dynamic
// twin of TestNoInternalTermsInHints (the static AST scan in
// glossary_lint_test.go). It closes the audit gap where:
//
//   - A string literal contains nothing illegal IN ISOLATION
//     (`"the contract carries %s"`)
//   - A field-access value contains nothing illegal IN ISOLATION
//     (`req.AcceptableClaimForms` produces an enum slice)
//   - But fmt.Sprintf composition or helper-rendered prose joins
//     them into a leak ("the contract carries AcceptableClaimForms")
//
// The static scan only sees the literal fragments and misses the
// composition. This test runs each prompt-producing entry-point
// end-to-end against a populated fixture, accumulates the rendered
// text, and runs the shared glossary scan against the WHOLE rendered
// output. Any leak introduced by helper renderers, fmt.Sprintf
// templates, or struct-field reflection-style printing fails here.
//
// Coverage (kept total by TestPromptRendererCensus):
//   - every evaluator BuildInitialInstruction implementation in this
//     package (13: analyzer, answer-document, extractor, explorer,
//     planner, coder, verifier, write_analyzer, sub_explorer,
//     log_triager, perf_triager, multi_repo_focus, write_controller)
//   - context.BuildPromptContext applied with the populated fixture
//     (renders the canonical user-section / system-section corpus
//     including LogTriage / PerfTrace / Evidence / Hint sections)
//   - every tool ParametersFor(ctx) projection (3: EmitAnswerDocument,
//     EmitAnswerDocumentPatch, EmitWriteWorkflowDecision) — the dynamic
//     schema surfaces the static TestNoInternalTermsInToolSchemas scan
//     does NOT cover (it walks Parameters() only)
func TestPromptSnapshot_NoInternalTermsInRenderedOutput(t *testing.T) {
	captures := allPromptSnapshotCaptures(t)
	if len(captures) == 0 {
		t.Fatalf("no prompt snapshot captures gathered — fixture or driver is broken")
	}

	var hits []glossarylint.Hit
	for _, c := range captures {
		hits = append(hits, glossarylint.ScanText(c.label, c.text)...)
	}

	if len(hits) == 0 {
		return
	}
	for _, h := range hits {
		t.Errorf("  %s", h)
	}
	t.Fatalf(
		"TestPromptSnapshot_NoInternalTermsInRenderedOutput found %d violation(s) "+
			"surfaced by RUNTIME composition (helper renders, fmt.Sprintf templates "+
			"with field access). The static AST scan in TestNoInternalTermsInHints "+
			"missed these because the leaked term only appears after composition. "+
			"Fix by replacing the dynamic source with user-facing language, then "+
			"either re-run or extend internal/skill/glossary.go :: "+
			"InternalTermsBlocklist with the now-clean term.",
		len(hits),
	)
}

// TestPromptRendererCensus keeps the snapshot roster total (§40.52,
// V11-3): (1) every `BuildInitialInstruction` receiver declared in a
// non-test file of this package must have a capture labelled
// "<receiver>.BuildInitialInstruction", and every such capture must
// name a declared receiver; (2) every `ParametersFor` receiver declared
// in internal/tool must have a "<receiver>.ParametersFor" capture;
// (3) no capture may have panicked or rendered empty text — a fixture
// that does not exercise a renderer is silent non-coverage, so the
// former `_ = recover()` swallow is now a recorded failure naming the
// evaluator.
func TestPromptRendererCensus(t *testing.T) {
	captures := allPromptSnapshotCaptures(t)
	labels := map[string]bool{}
	for _, c := range captures {
		if labels[c.label] {
			t.Errorf("duplicate capture label %s", c.label)
		}
		labels[c.label] = true
		if c.panicked != "" {
			t.Errorf("%s panicked against the snapshot fixture: %s — extend the fixture so the renderer is exercised", c.label, c.panicked)
			continue
		}
		if strings.TrimSpace(c.text) == "" {
			t.Errorf("%s rendered empty text — the fixture does not exercise this renderer", c.label)
		}
	}

	declared := methodReceivers(t, ".", "BuildInitialInstruction")
	for _, recv := range declared {
		if want := recv + ".BuildInitialInstruction"; !labels[want] {
			t.Errorf("evaluator %s declares BuildInitialInstruction but has no capture in allPromptSnapshotCaptures", recv)
		}
	}
	declaredSet := map[string]bool{}
	for _, recv := range declared {
		declaredSet[recv] = true
	}
	parametersFor := methodReceivers(t, filepath.Join("..", "tool"), "ParametersFor")
	parametersForSet := map[string]bool{}
	for _, recv := range parametersFor {
		parametersForSet[recv] = true
		if want := recv + ".ParametersFor"; !labels[want] {
			t.Errorf("tool %s declares ParametersFor but has no capture in allPromptSnapshotCaptures", recv)
		}
	}
	for label := range labels {
		switch {
		case strings.HasSuffix(label, ".BuildInitialInstruction"):
			if recv := strings.TrimSuffix(label, ".BuildInitialInstruction"); !declaredSet[recv] {
				t.Errorf("capture %s names an evaluator that declares no BuildInitialInstruction", label)
			}
		case strings.HasSuffix(label, ".ParametersFor"):
			if recv := strings.TrimSuffix(label, ".ParametersFor"); !parametersForSet[recv] {
				t.Errorf("capture %s names a tool that declares no ParametersFor", label)
			}
		}
	}
}

// methodReceivers returns, sorted, the receiver type names of every
// method named name declared in the non-test .go files directly under dir.
func methodReceivers(t *testing.T, dir, name string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", dir, err)
	}
	set := map[string]bool{}
	fset := token.NewFileSet()
	for _, e := range entries {
		fn := e.Name()
		if e.IsDir() || !strings.HasSuffix(fn, ".go") || strings.HasSuffix(fn, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, fn), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", fn, err)
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || fd.Name.Name != name || len(fd.Recv.List) != 1 {
				continue
			}
			recv := fd.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			if id, ok := recv.(*ast.Ident); ok {
				set[id.Name] = true
			}
		}
	}
	if len(set) == 0 {
		t.Fatalf("no %s receivers found under %s — census walker is broken", name, dir)
	}
	out := make([]string, 0, len(set))
	for recv := range set {
		out = append(out, recv)
	}
	sort.Strings(out)
	return out
}

// promptCapture pairs a label (for diagnostic output) with the
// rendered text gathered from one entry-point, plus the recorded panic
// message when the renderer did not survive the fixture.
type promptCapture struct {
	label    string
	text     string
	panicked string
}

// allPromptSnapshotCaptures runs every prompt-producing surface
// against the same comprehensive fixture and returns the captured
// (label, text) pairs.
func allPromptSnapshotCaptures(t *testing.T) []promptCapture {
	t.Helper()

	// One fixture seeds every evaluator. Stage / AgentName are
	// overridden per evaluator below so the right read-mode-vs-write-
	// mode artifact gating applies.
	base := buildPromptSnapshotAgentContext(t)
	skills := buildPromptSnapshotSkills()

	out := []promptCapture{
		captureBuildInitialInstruction(
			"analyzerEvaluator.BuildInitialInstruction",
			&analyzerEvaluator{},
			withAgent(base, types.AgentAnalyzer, types.StageAnalyze),
			skills["analysis"],
		),
		captureBuildInitialInstruction(
			"explorerEvaluator.BuildInitialInstruction",
			&explorerEvaluator{},
			withAgent(base, types.AgentExplorer, types.StageExplore),
			skills["explore"],
		),
		captureBuildInitialInstruction(
			"extractorEvaluator.BuildInitialInstruction",
			&extractorEvaluator{},
			withAgent(base, types.AgentExtractor, types.StageExtract),
			skills["extract"],
		),
		captureBuildInitialInstruction(
			"answerDocumentEvaluator.BuildInitialInstruction",
			&answerDocumentEvaluator{},
			withAgent(base, types.AgentFinalizer, types.StageFinalize),
			skills["answer_document"],
		),
		captureBuildInitialInstruction(
			"plannerEvaluator.BuildInitialInstruction",
			&plannerEvaluator{},
			withAgent(base, types.AgentPlanner, types.StagePlan),
			skills["plan"],
		),
		captureBuildInitialInstruction(
			"coderEvaluator.BuildInitialInstruction",
			&coderEvaluator{},
			withAgent(base, types.AgentCoder, types.StageApply),
			skills["apply"],
		),
		captureBuildInitialInstruction(
			"verifierEvaluator.BuildInitialInstruction",
			&verifierEvaluator{},
			withAgent(base, types.AgentVerifier, types.StageVerify),
			skills["verify"],
		),
		captureBuildInitialInstruction(
			"writeAnalyzerEvaluator.BuildInitialInstruction",
			&writeAnalyzerEvaluator{},
			withAnalyzerRetryHint(withAgent(base, types.AgentWriteAnalyzer, types.StageAnalyze)),
			skills["analysis"],
		),
		captureBuildInitialInstruction(
			"subExplorerEvaluator.BuildInitialInstruction",
			&subExplorerEvaluator{},
			withAgent(base, types.AgentName("sub_explorer"), types.StageExplore),
			skills["explore"],
		),
		captureBuildInitialInstruction(
			"logTriagerEvaluator.BuildInitialInstruction",
			&logTriagerEvaluator{},
			withAgent(base, types.AgentLogTriager, types.StageLogTriage),
			skills["log_triage"],
		),
		captureBuildInitialInstruction(
			"perfTriagerEvaluator.BuildInitialInstruction",
			&perfTriagerEvaluator{},
			withAgent(base, types.AgentPerfTriager, types.StagePerfTriage),
			skills["perf_triage"],
		),
		captureBuildInitialInstruction(
			"multiRepoFocusEvaluator.BuildInitialInstruction",
			&multiRepoFocusEvaluator{},
			withAgent(base, types.AgentMultiRepoFocus, types.StageAnalyze),
			nil,
		),
		captureBuildInitialInstruction(
			"writeControllerEvaluator.BuildInitialInstruction",
			&writeControllerEvaluator{},
			withWriteMode(withAgent(base, types.AgentWriteController, types.StagePlan)),
			nil,
		),
		capturePromptContext(
			"context.BuildPromptContext[finalizer]",
			withAgent(base, types.AgentFinalizer, types.StageFinalize),
			skills["answer_document"],
		),
		captureParametersFor(
			"EmitAnswerDocument.ParametersFor",
			&tool.EmitAnswerDocument{},
			withAgent(base, types.AgentFinalizer, types.StageFinalize),
		),
		captureParametersFor(
			"EmitAnswerDocumentPatch.ParametersFor",
			&tool.EmitAnswerDocumentPatch{},
			withAgent(base, types.AgentFinalizer, types.StageFinalize),
		),
		captureParametersFor(
			"EmitWriteWorkflowDecision.ParametersFor",
			&tool.EmitWriteWorkflowDecision{},
			withWriteMode(withAgent(base, types.AgentWriteController, types.StagePlan)),
		),
	}
	return out
}

// captureBuildInitialInstruction calls the evaluator's BuildInitialInstruction
// against the prepared context and returns the rendered string. A
// panic is recorded on the capture (TestPromptRendererCensus fails on
// it) instead of being swallowed, so a fixture gap can never read as
// coverage.
func captureBuildInitialInstruction(label string, ev Evaluator, ctx *types.AgentContext, sk *skill.Config) (out promptCapture) {
	out = promptCapture{label: label}
	if ev == nil || ctx == nil {
		out.panicked = "nil evaluator or context"
		return out
	}
	defer func() {
		if r := recover(); r != nil {
			out.panicked = fmt.Sprint(r)
		}
	}()
	out.text = ev.BuildInitialInstruction(ctx, sk)
	return out
}

func capturePromptContext(label string, ctx *types.AgentContext, sk *skill.Config) (out promptCapture) {
	out = promptCapture{label: label}
	if ctx == nil || sk == nil {
		out.panicked = "nil context or skill"
		return out
	}
	defer func() {
		if r := recover(); r != nil {
			out.panicked = fmt.Sprint(r)
		}
	}()
	pc := agentctx.BuildPromptContext(ctx, sk)
	if pc == nil {
		return out
	}
	var b strings.Builder
	for _, s := range pc.SystemSections {
		b.WriteString("[SYS:")
		b.WriteString(s.Title)
		b.WriteString("]\n")
		b.WriteString(s.Content)
		b.WriteString("\n")
	}
	for _, s := range pc.UserSections {
		b.WriteString("[USR:")
		b.WriteString(s.Title)
		b.WriteString("]\n")
		b.WriteString(s.Content)
		b.WriteString("\n")
	}
	out.text = b.String()
	return out
}

func captureParametersFor(label string, tl interface {
	ParametersFor(ctx *types.AgentContext) json.RawMessage
}, ctx *types.AgentContext) (out promptCapture) {
	out = promptCapture{label: label}
	defer func() {
		if r := recover(); r != nil {
			out.panicked = fmt.Sprint(r)
		}
	}()
	out.text = string(tl.ParametersFor(ctx))
	return out
}

// withAgent returns a shallow clone of base with AgentName / Stage
// re-stamped so each evaluator sees the right routing fields. The
// rest of the fixture (Mutable / AnalysisIR / LogTriage / PerfTrace /
// TurnAArtifacts / MultiGraph / write-workflow state) is shared so the
// same comprehensive evidence surface drives every evaluator's dynamic
// content.
func withAgent(base *types.AgentContext, name types.AgentName, stage types.PipelineStage) *types.AgentContext {
	if base == nil {
		return nil
	}
	cp := base.ShallowClone()
	cp.AgentName = name
	cp.Stage = stage
	return cp
}

// withAnalyzerRetryHint re-arms the one-shot analyzer retry hint that
// both analyzer-shaped evaluators consume (and reset) when they render
// their "previous attempt rejected" section; the analyzer capture above
// has already drained the fixture's copy by the time this runs.
func withAnalyzerRetryHint(ctx *types.AgentContext) *types.AgentContext {
	if ctx == nil || ctx.Mutable == nil {
		return ctx
	}
	ctx.Mutable.SetAnalyzerRetryHint("scope_anchors must name files that exist in the repository; previous emit listed a directory")
	return ctx
}

// withWriteMode stamps the plan-mode write lane on a clone so the write
// controller renders its mode-projected action contract and the
// write-workflow decision schema projects the plan-mode action enum.
func withWriteMode(ctx *types.AgentContext) *types.AgentContext {
	if ctx == nil {
		return nil
	}
	ctx.Mode = types.ModePlan
	return ctx
}

// buildPromptSnapshotAgentContext returns a single, comprehensively
// populated AgentContext that exercises every dynamic-rendering path
// each evaluator's BuildInitialInstruction follows.
//
// Field coverage rationale:
//   - AnalysisIR.RequestModel populated with Intent / Scenario /
//     SubTopics / SemanticPredicates / AnswerSubject so view-resolution
//     paths (ResolveQuestionFamily) reach a non-trivial branch.
//   - Mutable.LogTriage with crash-shaped frames + signals → exercises
//     formatLogTriageStructured + the IsCrashSourcedRootCause
//     gating in answer_document_evaluator.
//   - Mutable.PerfTrace with jank-shaped events → exercises
//     formatPerfTriageStructured.
//   - Mutable.TurnAArtifacts with EvidenceItems of mixed AnchorKinds →
//     exercises evidence-rendering helpers + the cached
//     LabelSupportTokens path.
//   - EvidenceItems / AnswerChains / AnswerSymbols set on the
//     AgentContext so context.BuildPromptContext renders the full
//     canonical user-section block.
//   - MultiGraph with a two-repo topology → the multi-repo focus
//     selector renders its topology rows.
//   - Mutable write-analysis / context-pack / workflow-run state → the
//     write controller renders its task, run, artifact and pack
//     sections and the decision schema projects a non-empty enum.
//   - AnalyzerRetryHint set → the analyzer / write-analyzer render their
//     "previous attempt rejected" section (re-armed per capture because
//     rendering resets the one-shot hint).
//   - AttachedLog with one `[orchestrator] … attempt N/M failed` line →
//     the log-triage renderer emits its operational-semantics section.
func buildPromptSnapshotAgentContext(t *testing.T) *types.AgentContext {
	t.Helper()
	mu := types.NewMutableState("Why does Foo() return nil when Bar is configured?")
	mu.SetLogTriage(&types.LogBundle{
		Meta: types.LogMeta{
			Lang:    "go",
			Signals: []types.LogSignal{types.SignalPanic},
			Summary: "panic: runtime error: invalid memory address",
		},
		Errors: []types.LogError{
			{
				Type:    "runtime error",
				Message: "invalid memory address",
				Frames: []types.LogFrame{
					{File: "internal/agent/foo.go", Line: 42, Func: "Foo", Confidence: 0.9},
					{File: "internal/agent/bar.go", Line: 88, Func: "Bar.Run", Confidence: 0.85},
				},
			},
		},
		ResolvedFiles: []string{"internal/agent/foo.go", "internal/agent/bar.go"},
		Entities:      []string{"Foo", "Bar"},
		IntentHint:    types.IntentRootCause,
		Coverage:      0.85,
	})
	mu.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{
			Source:     "hitrace",
			DurationMs: 5000,
			Signals:    []string{"jank", "cold-start-slow"},
		},
		Janks: []types.PerfJank{
			{StartTsMs: 100, DurationMs: 32, TriggerSpan: "Foo.Run", Reason: "long render"},
		},
		IntentHint:    "performance",
		ResolvedFiles: []string{"app/main.ets"},
	})
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		UserQuestion: "Why does Foo() return nil when Bar is configured?",
		ReadFiles:    []string{"internal/agent/foo.go", "internal/agent/bar.go"},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/agent/foo.go",
				LineStart:       42,
				AnchorKind:      types.AnchorReturn,
				AnchorSymbol:    "Foo",
				Subject:         "Foo",
				Object:          "nil",
				Snippet:         "return nil, errors.New(\"foo: bar not set\")",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
			{
				Kind:            types.EvidenceConditional,
				Source:          "internal/agent/foo.go",
				LineStart:       40,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "Foo",
				Snippet:         "if cfg.Bar == nil {",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
		},
	})
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{
				Kind:    types.WriteTaskFeature,
				Scope:   types.ScopeCross,
				Summary: "make Foo() fall back when Bar is not configured",
			},
			Risk:         types.WriteRiskProfile{Overall: types.RiskBandMedium},
			ScopeAnchors: []string{"internal/agent/foo.go"},
		},
	})
	mu.SetWriteContextPack(&types.WriteContextPack{
		PackID:  "pack-1",
		BatchID: "batch-1",
		Goal:    "first batch",
		Items: []types.WriteContextItem{{
			Priority:  types.WriteContextP0,
			Kind:      "constraint",
			Text:      "preserve the existing Foo() signature",
			Consumers: []types.WriteContextConsumer{types.WriteConsumerController},
		}},
	})
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Goal:   "first batch",
			Status: types.WriteWorkflowBatchNeedsExploration,
		}},
		Budget: types.WriteWorkflowBudget{MaxBatches: 5, MaxExplorationRounds: 2},
	})

	ir := &types.AnalysisIR{
		Version: "v4",
		TraceID: "test-trace",
		RequestModel: types.RequestModel{
			RawRequest:    "Why does Foo() return nil when Bar is configured?",
			Intent:        types.IntentRootCause,
			Scenario:      types.ScenarioRootCause,
			Complexity:    types.ComplexityModerate,
			Predicates:    types.SemanticPredicates{IsScalarAnswer: false},
			PredicateAxis: types.AxisCall,
			AnswerSubject: types.AnswerSubject{
				Kind:       types.SubjectFunctionName,
				Confidence: 0.7,
			},
			AnalyzerHints: types.AnalyzerHints{
				Keywords: []string{"foo", "bar", "nil", "configured"},
				Entities: []string{"Foo", "Bar"},
			},
		},
		HypothesisSet: []types.Hypothesis{
			{ID: "h1", Statement: "Foo returns nil when Bar config is missing", Status: types.HypUnknown},
		},
	}
	ir.AnswerContract.Language = "en"

	mu.SetAnalyzerRetryHint("scope_anchors must name files that exist in the repository; previous emit listed a directory")

	ac := &types.AgentContext{
		AgentName:  types.AgentFinalizer,
		Stage:      types.StageFinalize,
		Objective:  "Why does Foo() return nil when Bar is configured?",
		AnalysisIR: ir,
		Mutable:    mu,
		LogTriage:  mu.LogTriage(),
		PerfTrace:  mu.PerfTrace(),
		// The attached log carries one operational-semantics line so the
		// log-triage renderer emits its typed-semantics section instead
		// of an empty string.
		AttachedLog: "panic: runtime error: invalid memory address\n\tinternal/agent/foo.go:42 +0x88\n" +
			"12:00:01.000 ERROR [orchestrator] worker attempt 2/3 failed: dispatch timeout\n",
		Language:   "en",
		RepoRoot:   "/tmp/repo",
		Branch:     "main",
		MultiGraph: buildPromptSnapshotMultiGraph(t),
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/agent/foo.go",
				LineStart:       42,
				AnchorKind:      types.AnchorReturn,
				AnchorSymbol:    "Foo",
				Snippet:         "return nil, errors.New(\"foo: bar not set\")",
				GroundingStatus: types.GroundingGrounded,
			},
		},
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "Foo", File: "internal/agent/foo.go", Line: 42, Kind: types.KindFunction},
		},
		AnswerSymbolCompleteness: types.CompletenessLowerBound,
	}
	return ac
}

// buildPromptSnapshotMultiGraph returns a two-repo topology so the
// multi-repo focus selector renders its topology rows; graphs are
// built lazily and never loaded here.
func buildPromptSnapshotMultiGraph(t *testing.T) *multigraph.MultiGraph {
	t.Helper()
	topo := &topology.RepoTopology{
		ParentRoot: "/tmp/repo",
		ParentSlug: "parent-snapshot",
		Repos: []topology.SubRepo{
			{Slug: "core", RootAbs: "/tmp/repo/core", RootRel: "core", FileCount: 12, PrimaryLangs: []string{"go"}},
			{Slug: "ui", RootAbs: "/tmp/repo/ui", RootRel: "ui", FileCount: 7, PrimaryLangs: []string{"ets"}},
		},
		DiscoveredAt: time.Now(),
	}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build: func(repoRoot, _ string) (*rmtypes.Graph, error) {
			return rmTestGraphWithScore(filepath.Base(repoRoot), "internal/foo.go", 1.0), nil
		},
		Cap: 2,
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}
	return mg
}

// buildPromptSnapshotSkills returns the skill configs each evaluator
// expects. We use the production skills so any drift between the
// canonical skill prose and a future evaluator-side override fails
// here too.
func buildPromptSnapshotSkills() map[string]*skill.Config {
	out := make(map[string]*skill.Config, 8)
	reg := skill.NewRegistry()
	skill.RegisterDefaults(reg)
	for _, name := range reg.List() {
		sk, err := reg.Get(name)
		if err != nil {
			continue
		}
		switch name {
		case "analysis-skill":
			out["analysis"] = sk
		case "explore-skill":
			out["explore"] = sk
		case "extract-skill":
			out["extract"] = sk
		case "answer-document-skill":
			out["answer_document"] = sk
		case "change-plan-skill":
			out["plan"] = sk
		case "code-write-skill":
			out["apply"] = sk
		case "test-execute-skill":
			out["verify"] = sk
		case "log-triage-skill":
			out["log_triage"] = sk
		case "perf-triage-skill":
			out["perf_triage"] = sk
		}
	}
	// Provide a benign fallback skill for any evaluator whose skill
	// is not in the default registry (defensive — the registry covers
	// every production skill today).
	fallback := &skill.Config{Name: "snapshot-fallback"}
	for _, key := range []string{"analysis", "explore", "extract", "answer_document", "plan", "apply", "verify", "log_triage", "perf_triage"} {
		if out[key] == nil {
			out[key] = fallback
		}
	}
	return out
}
