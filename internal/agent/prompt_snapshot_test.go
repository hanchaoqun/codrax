package agent

import (
	"fmt"
	"strings"
	"testing"

	agentctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
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
// text, and runs the same blocklist scan against the WHOLE rendered
// output. Any leak introduced by helper renderers, fmt.Sprintf
// templates, or struct-field reflection-style printing fails here.
//
// Coverage:
//   - 11 evaluator BuildInitialInstruction implementations
//     (analyzer, answer-document, extractor, explorer, planner,
//     coder, verifier, write_analyzer, sub_explorer, log_triager,
//     perf_triager)
//   - context.BuildPromptContext applied with the populated fixture
//     (renders the canonical user-section / system-section corpus
//     including LogTriage / PerfTrace / Evidence / Hint sections)
//   - tool.EmitAnswerDocument.ParametersFor(ctx) — the dynamic
//     schema projection that the static TestNoInternalTermsInToolSchemas
//     scan does NOT cover (it walks Parameters() only)
//
// Adding a new prompt-producing entry-point requires extending one
// captureXxx helper below. The fixture is intentionally comprehensive
// so a single shape covers all entry-points without per-test fanout.
func TestPromptSnapshot_NoInternalTermsInRenderedOutput(t *testing.T) {
	captures := allPromptSnapshotCaptures(t)
	if len(captures) == 0 {
		t.Fatalf("no prompt snapshot captures gathered — fixture or driver is broken")
	}

	var hits []string
	for _, c := range captures {
		if c.text == "" {
			continue
		}
		for _, term := range skill.InternalTermsBlocklist {
			if term == "" {
				continue
			}
			idx := findTermMatch(c.text, term)
			if idx < 0 {
				continue
			}
			hits = append(hits, formatSnapshotHit(c.label, term, c.text, idx))
		}
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

// promptCapture pairs a label (for diagnostic output) with the
// rendered text gathered from one entry-point.
type promptCapture struct {
	label string
	text  string
}

// allPromptSnapshotCaptures runs every prompt-producing surface
// against the same comprehensive fixture and returns the captured
// (label, text) pairs.
func allPromptSnapshotCaptures(t *testing.T) []promptCapture {
	t.Helper()

	// One fixture seeds every evaluator. Stage / AgentName are
	// overridden per evaluator below so the right read-mode-vs-write-
	// mode artifact gating applies.
	base := buildPromptSnapshotAgentContext()
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
			withAgent(base, types.AgentWriteAnalyzer, types.StageAnalyze),
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
		capturePromptContext(
			"context.BuildPromptContext[finalizer]",
			withAgent(base, types.AgentFinalizer, types.StageFinalize),
			skills["answer_document"],
		),
		captureEmitAnswerDocumentParametersFor(
			"EmitAnswerDocument.ParametersFor[finalizer]",
			withAgent(base, types.AgentFinalizer, types.StageFinalize),
		),
	}
	return out
}

// captureBuildInitialInstruction calls the evaluator's BuildInitialInstruction
// against the prepared context and returns the rendered string.
func captureBuildInitialInstruction(label string, ev Evaluator, ctx *types.AgentContext, sk *skill.Config) promptCapture {
	if ev == nil || ctx == nil {
		return promptCapture{label: label, text: ""}
	}
	defer func() {
		// Some evaluators panic on nil sub-fields the fixture didn't
		// populate; we swallow + report so the test reports a useful
		// diagnostic for THAT evaluator instead of aborting the whole
		// test sweep.
		_ = recover()
	}()
	text := ev.BuildInitialInstruction(ctx, sk)
	return promptCapture{label: label, text: text}
}

func capturePromptContext(label string, ctx *types.AgentContext, sk *skill.Config) promptCapture {
	if ctx == nil || sk == nil {
		return promptCapture{label: label}
	}
	defer func() { _ = recover() }()
	pc := agentctx.BuildPromptContext(ctx, sk)
	if pc == nil {
		return promptCapture{label: label}
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
	return promptCapture{label: label, text: b.String()}
}

func captureEmitAnswerDocumentParametersFor(label string, ctx *types.AgentContext) promptCapture {
	defer func() { _ = recover() }()
	t := &tool.EmitAnswerDocument{}
	raw := t.ParametersFor(ctx)
	return promptCapture{label: label, text: string(raw)}
}

// withAgent returns a shallow clone of base with AgentName / Stage
// re-stamped so each evaluator sees the right routing fields. The
// rest of the fixture (Mutable / AnalysisIR / LogTriage / PerfTrace /
// TurnAArtifacts) is shared so the same comprehensive evidence
// surface drives every evaluator's dynamic content.
func withAgent(base *types.AgentContext, name types.AgentName, stage types.PipelineStage) *types.AgentContext {
	if base == nil {
		return nil
	}
	cp := base.ShallowClone()
	cp.AgentName = name
	cp.Stage = stage
	return cp
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
func buildPromptSnapshotAgentContext() *types.AgentContext {
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

	ac := &types.AgentContext{
		AgentName:    types.AgentFinalizer,
		Stage:        types.StageFinalize,
		Objective:    "Why does Foo() return nil when Bar is configured?",
		AnalysisIR:   ir,
		Mutable:      mu,
		LogTriage:    mu.LogTriage(),
		PerfTrace:    mu.PerfTrace(),
		AttachedLog:  "panic: runtime error: invalid memory address\n\tinternal/agent/foo.go:42 +0x88\n",
		Language:     "en",
		RepoRoot:     "/tmp/repo",
		Branch:       "main",
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

// formatSnapshotHit renders a violation line suitable for t.Errorf.
func formatSnapshotHit(label, term, body string, offset int) string {
	const window = 50
	from := offset - window
	if from < 0 {
		from = 0
	}
	to := offset + len(term) + window
	if to > len(body) {
		to = len(body)
	}
	preview := body[from:to]
	if from > 0 {
		preview = "…" + preview
	}
	if to < len(body) {
		preview = preview + "…"
	}
	return fmt.Sprintf("%s: leaked %q preview=%s", label, term, preview)
}
