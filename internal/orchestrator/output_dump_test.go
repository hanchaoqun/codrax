package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildOutputDumpBody_TwoSections(t *testing.T) {
	body := buildOutputDumpBody(dumpFinalOutputArgs{
		request: "panic source",
		answer:  "Section A\n\nSection B",
	})
	// UX-ANCHOR 件d (§29.61.7): the 问题 section echoes the request as a
	// verbatim text fence (customer input is never re-rendered as markdown).
	if !strings.Contains(body, "# 问题\n\n```text codrax-user-request\npanic source\n```\n") {
		t.Fatalf("missing 问题 section:\n%s", body)
	}
	if !strings.Contains(body, "# 回答\n\nSection A\n\nSection B\n") {
		t.Fatalf("missing 回答 section:\n%s", body)
	}
	if strings.Contains(body, "附件") {
		t.Fatalf("attachment footnote unexpectedly present:\n%s", body)
	}
}

func TestRecordTaskFinalizeWritesEmptyDefaultRootsWithoutModelSelection(t *testing.T) {
	for _, answer := range []string{"original model answer", ""} {
		t.Run(answer, func(t *testing.T) {
			mut := types.NewMutableState("trace investigation")
			mut.SetTraceFindingContract(&types.TraceFindingContract{RootCauseReportEnabled: true})
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}, outputDumpDir: t.TempDir(), outputDumpMax: 10, emit: func(render.Event) {}}
			o.recordTaskFinalize(&agent.StageOutput{FinalAnswer: answer})
			body, err := os.ReadFile(mut.FinalAnswerRootCauseJSONPath())
			if err != nil {
				t.Fatal(err)
			}
			var got outputdump.DefaultRootCauseArtifact
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			wantReason := "valid_model_root_cause_selection_unavailable"
			if answer == "" {
				wantReason = "final_answer_transcript_not_available"
			}
			if got.Status != "unavailable" || got.RootCauses == nil || len(got.RootCauses) != 0 || got.ReasonCode != wantReason {
				t.Fatalf("wrong empty artifact: %s", body)
			}
			if mut.TraceRootCauseReport() != nil || mut.Result() != answer {
				t.Fatal("delivery fallback must not invent a model selection or answer")
			}
			if answer == "" && mut.FinalAnswerMarkdownPath() != "" {
				t.Fatal("must not invent a transcript")
			}
		})
	}
}

func TestRunFailureStillWritesDefaultTraceRoots(t *testing.T) {
	for _, attached := range []string{"binary\x00trace", "worker-42 (42) [000] .... 1.000000: tracing_mark_write: B|42|task\n"} {
		t.Run(attached[:6], func(t *testing.T) {
			calls, events := 0, 0
			o := newTraceAdmissionTestOrchestrator(&calls, &events)
			o.SetOutputDump(t.TempDir(), 10)
			o.SetAttachedHitrace(attached)
			// A reused REPL orchestrator must not reuse the preceding run's roots.
			old := types.NewMutableState("prior run")
			old.SetFinalAnswerArtifactPaths("old.md", "old.html", "old.root-causes.json")
			o.busCtx = &types.BusContext{Mutable: old}
			repo := t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			bus, runErr := o.Run("investigate trace", repo, "main")
			if strings.Contains(attached, "\x00") && runErr == nil {
				t.Fatal("binary admission must fail")
			}
			if !strings.Contains(attached, "\x00") && (bus == nil || calls == 0) {
				t.Fatal("valid text must exercise the post-admission failure lane")
			}
			files, err := filepath.Glob(filepath.Join(o.outputDumpDir, "*.root-causes.json"))
			if err != nil || len(files) != 1 {
				t.Fatalf("failure must publish exactly one artifact: %v %v", files, err)
			}
			body, _ := os.ReadFile(files[0])
			var got outputdump.DefaultRootCauseArtifact
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			wantReason := "final_answer_transcript_not_available"
			if bus != nil && bus.Mutable != nil && bus.Mutable.Result() != "" {
				wantReason = "trace_root_cause_contract_not_active"
			}
			if got.Status != "unavailable" || got.ReasonCode != wantReason || got.RootCauses == nil || len(got.RootCauses) != 0 {
				t.Fatalf("wrong failure artifact: %s", body)
			}
			if old.FinalAnswerRootCauseJSONPath() != "old.root-causes.json" {
				t.Fatal("previous REPL turn was mutated")
			}
		})
	}
}

func TestRunExposesMandatoryTraceRootWriteFailureSeparately(t *testing.T) {
	calls, events := 0, 0
	o := newTraceAdmissionTestOrchestrator(&calls, &events)
	dir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(dir, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	o.SetOutputDump(dir, 10)
	o.SetAttachedHitrace("binary\x00trace")
	_, err := o.Run("investigate", t.TempDir(), "main")
	if err == nil || !strings.Contains(err.Error(), "trace input admission") || o.RootCauseOutputError() == nil || !strings.Contains(o.RootCauseOutputError().Error(), "root-cause output directory") {
		t.Fatalf("both pipeline and delivery failures must be retained: %v", err)
	}
}

func TestRunCancelledTraceWithoutFinalAnswerWritesEmptyRoots(t *testing.T) {
	calls, events := 0, 0
	o := newTraceAdmissionTestOrchestrator(&calls, &events)
	o.SetOutputDump(t.TempDir(), 10)
	o.SetAttachedHitrace("worker-42 (42) [000] .... 1.000000: tracing_mark_write: B|42|task\n")
	o.SetEmitter(func(event render.Event) {
		if event.Kind == render.EventPipelineStart {
			o.Cancel("test cancel before final answer")
		}
	})
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	bus, _ := o.Run("investigate trace", repo, "main")
	if bus == nil || bus.Mutable == nil {
		t.Fatal("cancel after admission should have a run-scoped bus")
	}
	if bus.Mutable.Result() != "" {
		t.Fatalf("fixture must cancel before final answer: %s", bus.Mutable.Result())
	}
	body, err := os.ReadFile(bus.Mutable.FinalAnswerRootCauseJSONPath())
	if err != nil {
		t.Fatal(err)
	}
	var got outputdump.DefaultRootCauseArtifact
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "unavailable" || got.ReasonCode != "final_answer_transcript_not_available" || got.RootCauses == nil || len(got.RootCauses) != 0 {
		t.Fatalf("cancelled run must still publish empty roots: %s", body)
	}
}

func TestTraceRootOutputScopeUsesTypedSignalsNotProse(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("analyze trace root-causes.json"), RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "fixture.systrace"}}}}
	o := &Orchestrator{}
	if o.hasTraceRootCauseOutputContext(bus) {
		t.Fatal("fixture mention alone must not activate diagnosis output")
	}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{ExternalObservationPolicy: &types.ExternalObservationPolicy{ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly}}}
	if !o.hasTraceRootCauseOutputContext(bus) {
		t.Fatal("typed external trace policy must activate output without an attachment")
	}
	bus.AnalysisIR = nil
	bus.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{{
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact},
	}}})
	if !o.hasTraceRootCauseOutputContext(bus) {
		t.Fatal("actual typed Trace observations must also cover mixed source/trace investigations")
	}
}

func TestBuildOutputDumpBody_AttachmentFootnotes(t *testing.T) {
	body := buildOutputDumpBody(dumpFinalOutputArgs{
		request:  "analyse crash",
		answer:   "ok",
		hasLog:   true,
		logBytes: 12 * 1024,
		hasTrace: true,
		traceB:   3 * 1024 * 1024,
	})
	if !strings.Contains(body, "> 附件: log (12.0 KiB)") {
		t.Fatalf("log footnote missing or mis-formatted:\n%s", body)
	}
	if !strings.Contains(body, "> 附件: htrace (3.0 MiB)") {
		t.Fatalf("htrace footnote missing or mis-formatted:\n%s", body)
	}
	// Footnotes must precede the answer section header.
	logIdx := strings.Index(body, "附件: log")
	ansIdx := strings.Index(body, "# 回答")
	if logIdx < 0 || ansIdx < 0 || logIdx >= ansIdx {
		t.Fatalf("attachment footnote ordering wrong: log@%d 回答@%d\n%s", logIdx, ansIdx, body)
	}
}

func TestBuildOutputDumpBody_RuntimeArtifactTable(t *testing.T) {
	body := buildOutputDumpBody(dumpFinalOutputArgs{
		request: "analyse jank",
		answer:  "ok",
		artifacts: []outputdump.RuntimeArtifact{
			{Kind: "trace", Source: "frame.systrace", Bytes: 12, Detail: "runtime trace"},
			{Kind: "perftrace", Source: "frame.perftrace", Bytes: 34, Detail: "source=raw_perfdata_fallback; symbolization_status=unsymbolized"},
		},
	})
	for _, want := range []string{"## 运行时附件", "| 类型 | 来源 | 大小 | 详情 |", "| trace | frame.systrace |", "| perftrace | frame.perftrace |", "symbolization_status=unsymbolized"} {
		if !strings.Contains(body, want) {
			t.Fatalf("runtime artifact table missing %q:\n%s", want, body)
		}
	}
}

func TestBuildOutputDumpBody_RuntimeArtifactTableEnglish(t *testing.T) {
	body := buildOutputDumpBody(dumpFinalOutputArgs{
		language: "en",
		request:  "analyse jank",
		answer:   "ok",
		artifacts: []outputdump.RuntimeArtifact{
			{Kind: "trace", Source: "frame.systrace", Bytes: 12, Detail: "runtime trace"},
		},
	})
	for _, want := range []string{"# Question", "## Runtime Artifacts", "| kind | source | size | detail |", "# Answer"} {
		if !strings.Contains(body, want) {
			t.Fatalf("english runtime artifact table missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "## 运行时附件") {
		t.Fatalf("english output dump leaked zh runtime title:\n%s", body)
	}
}

func TestBuildOutputDumpBody_EmptyFallbacks(t *testing.T) {
	body := buildOutputDumpBody(dumpFinalOutputArgs{})
	if !strings.Contains(body, "# 问题\n\n(空)\n") {
		t.Fatalf("empty request fallback missing:\n%s", body)
	}
	if !strings.Contains(body, "# 回答\n\n(空)\n") {
		t.Fatalf("empty answer fallback missing:\n%s", body)
	}
}

func TestBuildOutputDumpBody_PreservesMermaidFence(t *testing.T) {
	answer := "lead-in\n\n```mermaid\ngraph TD; A-->B\n```\n\ntrailing"
	body := buildOutputDumpBody(dumpFinalOutputArgs{
		request: "draw it",
		answer:  answer,
	})
	if !strings.Contains(body, "```mermaid\ngraph TD; A-->B\n```") {
		t.Fatalf("mermaid fence was rewritten or lost:\n%s", body)
	}
}

func TestOutputDumpFileName_Shape(t *testing.T) {
	now := time.Date(2026, 5, 8, 9, 12, 3, 742_000_000, time.UTC)
	name := outputDumpFileName(now, 12345)
	want := "20260508-091203.742-12345.md"
	if name != want {
		t.Fatalf("filename: got %q want %q", name, want)
	}
}

func TestPruneOutputDumpDir_KeepsMostRecent(t *testing.T) {
	dir := t.TempDir()
	// Lay down 5 files with strictly increasing mtimes.
	base := time.Now().Add(-time.Hour)
	names := []string{"a.md", "b.md", "c.md", "d.md", "e.md"}
	for i, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mod := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	// Drop a non-md file — prune must leave it untouched.
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	// max=3 → after prune only 2 newest stay (slot reserved for incoming write).
	pruneOutputDumpDir(dir, 3)

	got := mdNamesIn(t, dir)
	if len(got) != 2 || got[0] != "d.md" || got[1] != "e.md" {
		t.Fatalf("expected [d.md e.md], got %v", got)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("non-md file was deleted: %v", err)
	}
}

func TestPruneOutputDumpDir_BelowCapNoop(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pruneOutputDumpDir(dir, 10)
	if got := mdNamesIn(t, dir); len(got) != 2 {
		t.Fatalf("expected 2 files preserved, got %v", got)
	}
}

func TestPruneOutputDumpDir_ZeroMaxDisabled(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pruneOutputDumpDir(dir, 0)
	if got := mdNamesIn(t, dir); len(got) != 3 {
		t.Fatalf("zero max must disable prune; got %v", got)
	}
}

func TestWriteFinalOutputDump_EndToEnd(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "output")
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	gotPath := writeFinalOutputDump(dumpFinalOutputArgs{
		dir:     dir,
		max:     10,
		request: "what does X do",
		answer:  "X does Y.",
		now:     now,
		pid:     999,
	})
	want := filepath.Join(dir, "20260508-120000.000-999.md")
	if gotPath != want {
		t.Fatalf("written path = %q, want %q", gotPath, want)
	}
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("expected file %s: %v", want, err)
	}
	if !strings.Contains(string(got), "# 问题\n\n```text codrax-user-request\nwhat does X do") {
		t.Fatalf("body missing 问题 section:\n%s", got)
	}
	if !strings.Contains(string(got), "# 回答\n\nX does Y.\n") {
		t.Fatalf("body missing 回答 section:\n%s", got)
	}
}

func TestWriteFinalOutputDump_EmptyDirNoop(t *testing.T) {
	// dir == "" must be a clean no-op (caller-side gate already
	// ensures the orchestrator only invokes us when a real dir is
	// configured).
	writeFinalOutputDump(dumpFinalOutputArgs{
		dir:     "",
		max:     10,
		request: "x",
		answer:  "y",
		now:     time.Now(),
		pid:     1,
	})
}

func TestRecordTaskFinalizeWritesOutputDumpForFallbackAnswer(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "output")
	mut := types.NewMutableState("why did the answer fallback?")
	o := &Orchestrator{
		busCtx:        &types.BusContext{Mutable: mut},
		outputDumpDir: dir,
		outputDumpMax: 10,
		emit:          func(render.Event) {},
	}

	o.recordTaskFinalize(&agent.StageOutput{FinalAnswer: "· 未能生成结构化答案\n\nraw fallback"})

	path := mut.FinalAnswerMarkdownPath()
	if path == "" {
		t.Fatal("expected fallback answer dump path to be recorded")
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("dump path dir = %q, want %q", filepath.Dir(path), dir)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dump %s: %v", path, err)
	}
	if !strings.Contains(string(body), "# 回答\n\n· 未能生成结构化答案\n\nraw fallback\n") {
		t.Fatalf("fallback answer body not dumped:\n%s", body)
	}
}

func TestRecordTaskFinalizeRecordsRootCauseJSONPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "output")
	mut := types.NewMutableState("分析 trace 根因")
	impact := 0.004
	mut.SetTraceRootCauseReport(&types.TraceRootCauseReportV2{
		SchemaVersion: types.TraceRootCauseReportSchemaVersion,
		RootCauses: []*types.TraceRootCauseItemV2{{
			Rank: 1, Category: types.TraceRootCauseCPUSchedulingDelay,
			ThreadName: "RenderThread", ImpactSeconds: &impact,
			Summary: "RenderThread线程CPU调度延迟", Evidence: []string{"typed evidence"},
		}},
	})
	o := &Orchestrator{
		busCtx:        &types.BusContext{Mutable: mut},
		outputDumpDir: dir,
		outputDumpMax: 10,
		emit:          func(render.Event) {},
	}

	o.recordTaskFinalize(&agent.StageOutput{FinalAnswer: "answer body"})
	rootPath := mut.FinalAnswerRootCauseJSONPath()
	if rootPath == "" {
		t.Fatal("structured root-cause path must be retained for programmatic discovery")
	}
	if _, err := os.Stat(rootPath); err != nil {
		t.Fatalf("recorded structured path does not exist: %v", err)
	}
}

func TestRecordTaskFinalizeSurfacesCJKGluedRequestArtifact(t *testing.T) {
	// Regression: a trace named by path in the question (no --htrace flag),
	// written flush against Chinese prose, must still surface as a runtime
	// artifact in the generated markdown/HTML dump.
	dir := filepath.Join(t.TempDir(), "output")
	traceDir := t.TempDir()
	tracePath := filepath.Join(traceDir, "frame.systrace")
	if err := os.WriteFile(tracePath, []byte("tracing_mark_write: B|1|doFrame\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mut := types.NewMutableState("分析" + tracePath + "的卡顿")
	o := &Orchestrator{
		busCtx:        &types.BusContext{Mutable: mut},
		outputDumpDir: dir,
		outputDumpMax: 10,
		emit:          func(render.Event) {},
	}

	o.recordTaskFinalize(&agent.StageOutput{FinalAnswer: "answer body"})

	path := mut.FinalAnswerMarkdownPath()
	if path == "" {
		t.Fatal("expected markdown dump path")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read markdown dump %s: %v", path, err)
	}
	for _, want := range []string{"## 运行时附件", "| trace | " + tracePath + " |"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("dump missing %q:\n%s", want, body)
		}
	}
}

func TestRecordTaskFinalizeUsesExpandedTranscriptRequest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "output")
	folded := "[Pasted text #0 +3 lines +42 chars]"
	expanded := "请分析下面这段长文本:\n第一行原文\n第二行原文"
	mut := types.NewMutableState("## Prior conversation\nold\n\n## Current request\n" + folded)
	o := &Orchestrator{
		busCtx:        &types.BusContext{Mutable: mut},
		outputDumpDir: dir,
		outputDumpMax: 10,
		emit:          func(render.Event) {},
	}
	mut.SetOutputTranscriptRequest(expanded)

	o.recordTaskFinalize(&agent.StageOutput{FinalAnswer: "answer body"})

	path := mut.FinalAnswerMarkdownPath()
	if path == "" {
		t.Fatal("expected markdown dump path")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read markdown dump %s: %v", path, err)
	}
	if !strings.Contains(string(body), "# 问题\n\n```text codrax-user-request\n"+expanded) ||
		!strings.Contains(string(body), "# 回答\n\nanswer body") {
		t.Fatalf("markdown dump did not use expanded request:\n%s", body)
	}
	if strings.Contains(string(body), folded) {
		t.Fatalf("markdown dump leaked folded paste placeholder:\n%s", body)
	}

	htmlPath := mut.FinalAnswerHTMLPath()
	if htmlPath == "" {
		t.Fatal("expected html dump path")
	}
	htmlBody, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read html dump %s: %v", htmlPath, err)
	}
	if !strings.Contains(string(htmlBody), "第一行原文") || strings.Contains(string(htmlBody), folded) {
		t.Fatalf("html dump did not reflect expanded request:\n%s", htmlBody)
	}
}

func TestRecordTaskFinalizeUsesRunScopedTranscriptOverStaleSetter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "output")
	firstTurn := "第一个问题"
	secondTurn := "第二个问题\n需要展开保留"
	mut := types.NewMutableState("## Prior conversation\n- You: " + firstTurn + "\n\n## Current request\n[folded second]")
	mut.SetOutputTranscriptRequest(secondTurn)
	o := &Orchestrator{
		busCtx:                  &types.BusContext{Mutable: mut},
		outputDumpDir:           dir,
		outputDumpMax:           10,
		outputTranscriptRequest: firstTurn, // stale cross-REPL-turn setter field
		emit:                    func(render.Event) {},
	}

	o.recordTaskFinalize(&agent.StageOutput{FinalAnswer: "second answer"})

	path := mut.FinalAnswerMarkdownPath()
	if path == "" {
		t.Fatal("expected markdown dump path")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read markdown dump %s: %v", path, err)
	}
	text := string(body)
	if !strings.Contains(text, "# 问题\n\n```text codrax-user-request\n"+secondTurn+"\n```\n") {
		t.Fatalf("markdown dump did not use current run transcript request:\n%s", text)
	}
	if strings.Contains(text, "# 问题\n\n```text codrax-user-request\n"+firstTurn+"\n```\n") {
		t.Fatalf("markdown dump leaked stale first-turn request:\n%s", text)
	}

	htmlPath := mut.FinalAnswerHTMLPath()
	if htmlPath == "" {
		t.Fatal("expected html dump path")
	}
	htmlBody, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read html dump %s: %v", htmlPath, err)
	}
	htmlText := string(htmlBody)
	if !strings.Contains(htmlText, "第二个问题") || strings.Contains(htmlText, "第一个问题") {
		t.Fatalf("html dump did not use current run transcript request:\n%s", htmlText)
	}
}

func mdNamesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			out = append(out, e.Name())
		}
	}
	// Sorted by ReadDir already (lexical), good enough for assertions.
	return out
}
