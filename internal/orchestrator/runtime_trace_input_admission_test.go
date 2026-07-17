package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func newTraceAdmissionTestOrchestrator(calls *int, events *int) *Orchestrator {
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){}
	for _, name := range []types.AgentName{
		types.AgentLogTriager,
		types.AgentPerfTriager,
		types.AgentAnalyzer,
		types.AgentExplorer,
		types.AgentExtractor,
		types.AgentFinalizer,
	} {
		name := name
		agentFns[name] = func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			(*calls)++
			return &agent.StageOutput{Error: "trace admission test stop"}, nil
		}
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetEmitter(func(render.Event) { (*events)++ })
	return o
}

func newTypedNamedTraceAdmissionTestOrchestrator(paths []string, analyzerCalls, otherCalls *int, events *[]render.Event) *Orchestrator {
	policy := &types.ExternalObservationPolicy{
		CurrentSourceMode:    types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:        types.ExternalObservationSourceExclusionExplicitUserBoundary,
		ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
		SourceQuotes:         []string{"only analyze the named trace; do not analyze code"},
		Confidence:           1,
	}
	hints := make([]types.RequiredFileHint, 0, len(paths))
	for _, path := range paths {
		hints = append(hints, types.RequiredFileHint{Path: path, Confidence: 1, Rationale: "explicit runtime trace path"})
	}
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){}
	for _, name := range []types.AgentName{
		types.AgentLogTriager,
		types.AgentPerfTriager,
		types.AgentExplorer,
		types.AgentExtractor,
		types.AgentFinalizer,
	} {
		name := name
		agentFns[name] = func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			(*otherCalls)++
			return &agent.StageOutput{Error: "typed named trace admission test stop"}, nil
		}
	}
	agentFns[types.AgentAnalyzer] = func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
		(*analyzerCalls)++
		return &agent.StageOutput{AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Intent:                    types.IntentRootCause,
				Scenario:                  types.ScenarioPerformanceBottleneck,
				ExternalObservationPolicy: policy,
				AnalyzerHints:             types.AnalyzerHints{RequiredFileHints: hints},
			},
			TaskGraph: types.TaskGraph{Nodes: []types.TaskNode{{
				ID: "finalize", Type: types.NodeFinalize, Objective: "produce the trace answer", OneShot: true,
			}}},
		}}, nil
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetEmitter(func(event render.Event) { *events = append(*events, event) })
	return o
}

func TestRunRejectsNamedBinaryTraceFromDegradedPartialTypedIR(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	path := filepath.Join(repo, "partial.sys")
	if err := os.WriteFile(path, []byte{0xdf, 0x49, 0, 1}, 0o644); err != nil {
		t.Fatal(err)
	}
	policy := &types.ExternalObservationPolicy{
		CurrentSourceMode:    types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:        types.ExternalObservationSourceExclusionExplicitUserBoundary,
		ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
		SourceQuotes:         []string{"只分析这份 trace，不分析代码"},
		Confidence:           1,
	}
	analyzerCalls, otherCalls := 0, 0
	var events []render.Event
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){}
	for _, name := range []types.AgentName{types.AgentLogTriager, types.AgentPerfTriager, types.AgentExplorer, types.AgentExtractor, types.AgentFinalizer} {
		name := name
		agentFns[name] = func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			otherCalls++
			return &agent.StageOutput{Error: "must not run"}, nil
		}
	}
	agentFns[types.AgentAnalyzer] = func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
		analyzerCalls++
		return &agent.StageOutput{
			AnalysisIR: &types.AnalysisIR{
				Version: types.AnalysisIRVersion,
				RequestModel: types.RequestModel{
					Intent:                    types.IntentRootCause,
					Scenario:                  types.ScenarioPerformanceBottleneck,
					ExternalObservationPolicy: policy,
				},
			},
			Error: "forced post-emit analyzer gate rejection",
		}, nil
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetEmitter(func(event render.Event) { events = append(events, event) })
	bus, err := o.Run("只分析 `"+path+"` 的丢帧，不分析代码", repo, "main")
	if err == nil || bus == nil {
		t.Fatalf("degraded partial typed IR bypassed admission: bus=%+v err=%v", bus, err)
	}
	if analyzerCalls == 0 || otherCalls != 0 || hasTraceAdmissionEventKind(events, render.EventAnalysisReady) {
		t.Fatalf("degraded named trace crossed admission boundary: analyzer=%d other=%d ready=%v", analyzerCalls, otherCalls, hasTraceAdmissionEventKind(events, render.EventAnalysisReady))
	}
}

func hasTraceAdmissionEventKind(events []render.Event, kind render.EventKind) bool {
	for _, event := range events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func writeTraceAdmissionRepoSource(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsDirectBinaryAttachedTraceBeforeAnyInvestigation(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	calls, events := 0, 0
	o := newTraceAdmissionTestOrchestrator(&calls, &events)
	o.SetAttachedHitrace(string([]byte{'H', 'T', 0, 1, 2, 3}))

	bus, err := o.Run("分析这份 trace", repo, "main")
	if err == nil {
		t.Fatal("binary direct attachment should fail run entry")
	}
	if bus != nil {
		t.Fatalf("rejected run bus=%+v, want nil", bus)
	}
	var issue attachment.TextIssue
	if !errors.As(err, &issue) || issue.Kind != attachment.KindTrace {
		t.Fatalf("error=%T %v, want typed trace TextIssue", err, err)
	}
	if calls != 0 || events != 0 {
		t.Fatalf("rejected attachment entered investigation: agent_calls=%d events=%d", calls, events)
	}
}

func TestRunRejectsLateBinaryDirectAttachmentBeforeAnyInvestigation(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	calls, events := 0, 0
	o := newTraceAdmissionTestOrchestrator(&calls, &events)
	o.SetAttachedHitrace(strings.Repeat("valid trace prefix\n", 5000) + "\x00")

	bus, err := o.Run("分析这份 trace", repo, "main")
	if err == nil || bus != nil {
		t.Fatalf("late binary direct attachment was admitted: bus=%+v err=%v", bus, err)
	}
	if calls != 0 || events != 0 {
		t.Fatalf("late binary attachment entered investigation: agent_calls=%d events=%d", calls, events)
	}
}

func TestRunRejectsInvalidUTF8DirectAttachmentWithoutTruncationProvenance(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	calls, events := 0, 0
	o := newTraceAdmissionTestOrchestrator(&calls, &events)
	o.SetAttachedHitrace("valid trace row\n\xe4")

	bus, err := o.Run("分析这份 trace", repo, "main")
	if err == nil || bus != nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("direct incomplete UTF-8 payload was admitted: bus=%+v err=%v", bus, err)
	}
	if calls != 0 || events != 0 {
		t.Fatalf("invalid direct attachment entered investigation: agent_calls=%d events=%d", calls, events)
	}
}

func TestRunRejectsOversizedPhysicalLineDirectAttachmentBeforeAnyInvestigation(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	calls, events := 0, 0
	o := newTraceAdmissionTestOrchestrator(&calls, &events)
	o.SetAttachedHitrace(strings.Repeat("x", attachment.TracePhysicalLineMaxBytes+1))

	bus, err := o.Run("分析这份 trace", repo, "main")
	if err == nil || bus != nil || !strings.Contains(err.Error(), "physical line exceeds") {
		t.Fatalf("oversized direct trace line was admitted: bus=%+v err=%v", bus, err)
	}
	if calls != 0 || events != 0 {
		t.Fatalf("oversized direct trace line entered investigation: agent_calls=%d events=%d", calls, events)
	}
}

func TestRunRejectsFlattenedMultiSourceAttachmentBeforeAnyInvestigation(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	calls, events := 0, 0
	o := newTraceAdmissionTestOrchestrator(&calls, &events)
	o.SetAttachedHitrace("# codrax-source: before.systrace\nsched_switch: prev_pid=1 next_pid=2\n" +
		"# codrax-source: after.systrace\nsched_switch: prev_pid=3 next_pid=4\n")
	bus, err := o.Run("对比两份 trace", repo, "main")
	if err == nil || bus != nil || !strings.Contains(err.Error(), "multiple physical source headers") {
		t.Fatalf("flattened multi-source trace was admitted: bus=%+v err=%v", bus, err)
	}
	if calls != 0 || events != 0 {
		t.Fatalf("flattened multi-source trace entered investigation: calls=%d events=%d", calls, events)
	}
}

func TestRunRejectsNaturalLanguageNamedBinarySysAfterTypedClassificationBeforeExploration(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	path := filepath.Join(repo, "customer capture.sys")
	// This is the customer's newer/unknown binary signature. Run-entry content
	// sniffing intentionally does not claim to identify it, and .sys remains a
	// generic extension. The typed analyzer policy must still bring the named
	// file through full text admission before any investigation starts.
	if err := os.WriteFile(path, append([]byte{0xdf, 0x49}, make([]byte, 64)...), 0o644); err != nil {
		t.Fatal(err)
	}
	analyzerCalls, otherCalls := 0, 0
	var events []render.Event
	// Deliberately omit RequiredFileHints: once the structured external-trace
	// policy is active, deterministic request-path enumeration carries the
	// physical set so small-model path omission cannot bypass admission.
	o := newTypedNamedTraceAdmissionTestOrchestrator(nil, &analyzerCalls, &otherCalls, &events)
	bus, err := o.Run("只分析这份 trace `"+path+"` 的丢帧根因，不分析代码", repo, "main")
	if err == nil || bus == nil {
		t.Fatalf("named binary .sys was admitted: bus=%+v err=%v", bus, err)
	}
	var admission *tracequery.TraceInputAdmissionError
	canonicalPath, canonicalErr := filepath.EvalSymlinks(path)
	if canonicalErr != nil {
		canonicalPath = path
	}
	if !errors.As(err, &admission) || admission.Code != tracequery.TraceInputAdmissionCodeConversionRequired || admission.Path != canonicalPath {
		t.Fatalf("named binary verdict=%+v err=%v", admission, err)
	}
	if analyzerCalls != 1 || otherCalls != 0 || hasTraceAdmissionEventKind(events, render.EventAnalysisReady) {
		t.Fatalf("named binary trace crossed typed classification boundary: analyzer_calls=%d other_calls=%d analysis_ready=%v", analyzerCalls, otherCalls, hasTraceAdmissionEventKind(events, render.EventAnalysisReady))
	}
}

func TestTypedNamedTraceAdmissionDoesNotArmFromRawSysWithoutTypedPolicy(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "converter-fixture.sys")
	if err := os.WriteFile(path, []byte{0xdf, 0x49, 0, 1}, 0o644); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{
		RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{
				Path: path, Confidence: 1, Rationale: "converter regression fixture",
			}}},
		}},
	}
	if err := validateTypedNamedTraceInputsBeforeExploration(
		context.Background(), bus, "修复 converter 对 `"+path+"` 的处理，不分析 trace",
	); err != nil {
		t.Fatalf("raw .sys path armed trace admission without typed policy: %v", err)
	}
}

func TestTypedNamedTraceAdmissionDoesNotTreatExternalLogPolicyAsSysTraceIntent(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "converter-fixture.sys")
	if err := os.WriteFile(path, []byte{0xdf, 0x49, 0, 1}, 0o644); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{
		RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioRootCause,
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode:    types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:        types.ExternalObservationSourceExclusionExplicitUserBoundary,
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				SourceQuotes:         []string{"只分析外部日志"},
			},
		}},
	}
	if err := validateTypedNamedTraceInputsBeforeExploration(
		context.Background(), bus, "分析外部 error.log；converter fixture 位于 `"+path+"`",
	); err != nil {
		t.Fatalf("external log policy upgraded a .sys fixture into trace intent: %v", err)
	}
}

func TestRunNamedMultiTraceAdmissionIsAtomic(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	good := filepath.Join(repo, "before.trace")
	bad := filepath.Join(repo, "after.htrace")
	row := "app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n"
	if err := os.WriteFile(good, []byte(row), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte{'P', 'K', 0x03, 0x04, 0, 1}, 0o644); err != nil {
		t.Fatal(err)
	}
	analyzerCalls, otherCalls := 0, 0
	var events []render.Event
	o := newTypedNamedTraceAdmissionTestOrchestrator([]string{good, bad}, &analyzerCalls, &otherCalls, &events)
	bus, err := o.Run("对比 trace "+good+" 和 "+bad+" 的调度差异", repo, "main")
	if err == nil || bus == nil {
		t.Fatalf("mixed multi-trace set was admitted: bus=%+v err=%v", bus, err)
	}
	var admission *tracequery.TraceInputAdmissionError
	canonicalBad, canonicalErr := filepath.EvalSymlinks(bad)
	if canonicalErr != nil {
		canonicalBad = bad
	}
	if !errors.As(err, &admission) || admission.Path != canonicalBad || admission.Code != tracequery.TraceInputAdmissionCodeTextExportRequired {
		t.Fatalf("mixed multi-trace verdict=%+v err=%v", admission, err)
	}
	if analyzerCalls != 1 || otherCalls != 0 || hasTraceAdmissionEventKind(events, render.EventAnalysisReady) {
		t.Fatalf("partial multi-trace investigation started: analyzer_calls=%d other_calls=%d analysis_ready=%v", analyzerCalls, otherCalls, hasTraceAdmissionEventKind(events, render.EventAnalysisReady))
	}
}

func TestRunNamedTextMultiTracePreservesNormalPipeline(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	first := filepath.Join(repo, "before.trace")
	second := filepath.Join(repo, "after.htrace")
	row := "app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n"
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte(row), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	analyzerCalls, otherCalls := 0, 0
	var events []render.Event
	o := newTypedNamedTraceAdmissionTestOrchestrator([]string{first, second}, &analyzerCalls, &otherCalls, &events)
	_, _ = o.Run("对比 "+first+" 与 "+second+" 两份 trace 的调度差异", repo, "main")
	if analyzerCalls != 1 || !hasTraceAdmissionEventKind(events, render.EventAnalysisReady) {
		t.Fatalf("valid multi-trace request did not cross admission into normal pipeline: analyzer_calls=%d other_calls=%d analysis_ready=%v", analyzerCalls, otherCalls, hasTraceAdmissionEventKind(events, render.EventAnalysisReady))
	}
}

func TestTypedNamedTraceAdmissionPathKeyPreservesPOSIXCase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows path identity is case-insensitive by contract")
	}
	upper := typedNamedTraceAdmissionPathKey(filepath.Join(t.TempDir(), "A.trace"))
	lower := typedNamedTraceAdmissionPathKey(filepath.Join(filepath.Dir(upper), "a.trace"))
	if upper == lower {
		t.Fatalf("POSIX trace path dedupe collapsed case-distinct files: %q", upper)
	}
}

func TestRunDoesNotInferTraceAnalysisIntentFromRawBinaryFixturePath(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	path := filepath.Join(repo, "converter bad fixture.htrace")
	if err := os.WriteFile(path, []byte{0xce, 0x0a, 1, 0, 1, 0, 0, 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	calls, events := 0, 0
	o := newTraceAdmissionTestOrchestrator(&calls, &events)
	_, _ = o.Run("修复 parser 对 \""+path+"\" 的转换处理，不要分析 trace 内容", repo, "main")
	if calls == 0 || events == 0 {
		t.Fatalf("raw path shape blocked ordinary code investigation: agent_calls=%d events=%d", calls, events)
	}
}
