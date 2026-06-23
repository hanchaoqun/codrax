package repl

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// stubRunner is a no-op Runner so the REPL can be exercised without
// pulling in the full orchestrator. /clear and /exit are slash
// commands and never call Run, so the runner is unused for these
// tests.
type stubRunner struct{}

func (stubRunner) Run(_, _, _ string) (*types.BusContext, error) {
	return &types.BusContext{}, nil
}

// logAwareRunner captures every SetAttachedLog propagation plus the
// attachedLog value visible to Run, so tests can prove that REPL's
// one-shot auto-route semantics fire on the dispatch after a paste
// and only on that dispatch.
type logAwareRunner struct {
	setCalls []string // each SetAttachedLog argument, in call order
	seenLogs []string // attachedLog observed at Run() entry per dispatch
	curLog   string
}

func (r *logAwareRunner) Run(_, _, _ string) (*types.BusContext, error) {
	r.seenLogs = append(r.seenLogs, r.curLog)
	return &types.BusContext{}, nil
}
func (r *logAwareRunner) SetAttachedLog(s string) {
	r.setCalls = append(r.setCalls, s)
	r.curLog = s
}

// errorRunner simulates a pipeline that failed with a LastError
// — used to verify the REPL sanitizes what it persists to memory.
type errorRunner struct{ errText string }

func (r errorRunner) Run(_, _, _ string) (*types.BusContext, error) {
	bc := &types.BusContext{}
	bc.TaskState.LastError = r.errText
	bc.Mutable = types.NewMutableState("probe")
	return bc, nil
}

type markdownPathRunner struct {
	path     string
	htmlPath string
}

func (r markdownPathRunner) Run(_, _, _ string) (*types.BusContext, error) {
	mut := types.NewMutableState("probe")
	mut.SetResult("final answer body")
	mut.SetFinalAnswerOutputPaths(r.path, r.htmlPath)
	return &types.BusContext{Mutable: mut}, nil
}

type outputTranscriptRunner struct {
	requests           []string
	transcriptRequests []string
}

func (r *outputTranscriptRunner) SetOutputTranscriptRequest(s string) {
	r.transcriptRequests = append(r.transcriptRequests, s)
}

func (r *outputTranscriptRunner) Run(request, _, _ string) (*types.BusContext, error) {
	r.requests = append(r.requests, request)
	mut := types.NewMutableState(request)
	mut.SetResult("final answer body")
	return &types.BusContext{Mutable: mut}, nil
}

type mermaidAnswerRunner struct{}

func (mermaidAnswerRunner) Run(_, _, _ string) (*types.BusContext, error) {
	mut := types.NewMutableState("probe")
	mut.SetResult("```mermaid\nflowchart LR\n  A --> B\n```\n")
	return &types.BusContext{Mutable: mut}, nil
}

type stubMarkdownPreviewer struct {
	url  string
	path string
}

func (s *stubMarkdownPreviewer) RegisterMarkdown(path string) (string, error) {
	s.path = path
	return s.url, nil
}

// stubSummarizer mirrors the one in internal/memory's tests but is
// duplicated here so the repl package's tests do not have to import
// internal test helpers from a sibling package (Go forbids that).
type stubSummarizer struct{}

func (stubSummarizer) Summarize(_ context.Context, t memory.Turn) (memory.IndexEntry, error) {
	return memory.IndexEntry{
		ID:      t.ID,
		Topic:   "topic-" + t.ID,
		Summary: "summary",
	}, nil
}

func renderNothing(*types.BusContext) string { return "" }

func newTestREPL(store *memory.Store, in *strings.Reader, out *bytes.Buffer) *REPL {
	return New(Config{
		Runner:     stubRunner{},
		Store:      store,
		Render:     renderNothing,
		RepoRoot:   ".",
		Branch:     "main",
		In:         in,
		Out:        out,
		Prompt:     ">",
		PromptCont: ".",
		Banner:     "test-banner",
		// Pin language to English so test assertions on literal
		// English strings stay stable. Production zh-default
		// behaviour is exercised by the bilingual messages.go
		// helper tests.
		Language: "en",
	})
}

func TestHitraceConvertBareShowsLocalizedUsage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lang     string
		want     string
		mustMiss string
	}{
		{
			name:     "zh",
			lang:     "zh",
			want:     "将二进制 Harmony/OpenHarmony HiTrace 手动转换为文本 systrace",
			mustMiss: "load hitrace",
		},
		{
			name:     "en",
			lang:     "en",
			want:     "Convert a binary Harmony/OpenHarmony HiTrace file to text systrace",
			mustMiss: "load hitrace",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
			if err != nil {
				t.Fatalf("NewStore: %v", err)
			}
			var out bytes.Buffer
			r := newTestREPL(store, strings.NewReader(""), &out)
			r.language = tc.lang

			r.handleHitraceCmd("/htrace convert")

			got := out.String()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("localized usage missing %q; got:\n%s", tc.want, got)
			}
			if strings.Contains(got, tc.mustMiss) {
				t.Fatalf("bare convert should not be treated as a trace path; got:\n%s", got)
			}
		})
	}
}

func TestHitraceConvertHelpAliases(t *testing.T) {
	for _, input := range []string{
		"/htrace convert help",
		"/htrace convert --help",
		"/htrace convert -h",
	} {
		t.Run(input, func(t *testing.T) {
			store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
			if err != nil {
				t.Fatalf("NewStore: %v", err)
			}
			var out bytes.Buffer
			r := newTestREPL(store, strings.NewReader(""), &out)

			r.handleHitraceCmd(input)

			got := out.String()
			if !strings.Contains(got, "/htrace convert [--trace-engine=trace_streamer|builtin|auto] [--keep-trace-db] [--trace-db-output <path>] <binary-hitrace> [output.systrace]") {
				t.Fatalf("help alias did not print convert usage; got:\n%s", got)
			}
			if !strings.Contains(got, "Auto mode uses trace_streamer/SQL first") ||
				!strings.Contains(got, "falls back to the built-in raw trace parser") ||
				!strings.Contains(got, "Explicit trace_streamer or builtin modes do not degrade") ||
				!strings.Contains(got, "--keep-trace-db") ||
				!strings.Contains(got, "--trace-db-output") {
				t.Fatalf("help alias should explain auto fallback and explicit engine choice; got:\n%s", got)
			}
			if strings.Contains(got, "load hitrace") || strings.Contains(got, "convert hitrace:") {
				t.Fatalf("help alias should not execute conversion or load path; got:\n%s", got)
			}
		})
	}
}

func TestHitraceToolsStatusShowsGateAndDoesNotMutateAttachment(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	var out bytes.Buffer
	r := newTestREPL(store, strings.NewReader(""), &out)
	r.attachedHitrace = "existing trace"
	r.attachedHitraceSource = "existing-source"

	r.handleHitraceCmd("/htrace tools-status")

	got := out.String()
	for _, want := range []string{
		"trace_engine: auto",
		"selected_engine:",
		"trace_provider[official_trace_db/trace_streamer_db]",
		"trace_provider[builtin_modern/codrax_builtin_modern_profiler]",
		"trace_gate[sys_binary_parity_gate/no_perf_sys_binary_parity]",
		"state=pending_representative_fixture",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("tools-status output missing %q:\n%s", want, got)
		}
	}
	if r.attachedHitrace != "existing trace" || r.attachedHitraceSource != "existing-source" {
		t.Fatalf("tools-status must not mutate attached trace: trace=%q source=%q", r.attachedHitrace, r.attachedHitraceSource)
	}
}

func TestHitraceToolsStatusChineseLocalizesGate(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	var out bytes.Buffer
	r := newTestREPL(store, strings.NewReader(""), &out)
	r.language = "zh"

	r.handleHitraceCmd("/htrace tools-status")

	got := out.String()
	for _, want := range []string{
		"trace 解析引擎：auto",
		"trace_gate[sys_binary_parity_gate/no_perf_sys_binary_parity]",
		"状态=等待代表性fixture",
		"尚未提交可再分发的真实代表性 no-perf .sys fixture",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("zh tools-status output missing %q:\n%s", want, got)
		}
	}
	for _, leak := range []string{
		"built-in sys binary parser remains an explicit guarded lane",
		"trace+perf htrace in auto mode may fall back",
	} {
		if strings.Contains(got, leak) {
			t.Fatalf("zh tools-status leaked English detail %q:\n%s", leak, got)
		}
	}
}

func TestHitraceConvertPassesTraceEngineOption(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	var out bytes.Buffer
	var got hitraceconv.Options
	calls := 0
	r := New(Config{
		Runner:   stubRunner{},
		Store:    store,
		Render:   renderNothing,
		RepoRoot: ".",
		Branch:   "main",
		In:       strings.NewReader(""),
		Out:      &out,
		Language: "en",
		HitraceConvert: func(_ context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
			calls++
			got = opts
			if opts.Progress == nil {
				t.Fatalf("hitrace convert progress hook was not installed")
			}
			opts.Progress(hitraceconv.ProgressEvent{
				Stage:      "trace_db_normalize",
				Status:     hitraceconv.ProgressStatusStarted,
				Message:    "normalizing trace_streamer SQLite DB to systrace",
				Path:       "out.trace.db",
				OutputPath: opts.OutputPath,
			})
			return hitraceconv.Result{
				OutputPath:    opts.OutputPath,
				EventsWritten: 1,
				TraceDecisions: []hitraceconv.TraceProviderDecision{{
					ProviderName:    "codrax_builtin_sys_binary",
					EngineMode:      opts.TraceEngine,
					Selected:        true,
					Attempted:       true,
					Succeeded:       true,
					TraceQueryReady: true,
				}},
			}, nil
		},
	})

	r.handleHitraceCmd("/htrace convert --trace-engine=builtin input.htrace out.systrace")

	if calls != 1 {
		t.Fatalf("converter calls = %d, want 1; output:\n%s", calls, out.String())
	}
	if got.InputPath != "input.htrace" || got.OutputPath != "out.systrace" ||
		got.TraceEngine != "builtin" || got.Flavor != "harmony_hitrace" {
		t.Fatalf("unexpected convert opts: %+v", got)
	}
	if !strings.Contains(out.String(), "converted hitrace: out.systrace") {
		t.Fatalf("conversion success not rendered:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "hitrace convert progress: stage=trace_db_normalize status=started") {
		t.Fatalf("conversion progress not rendered:\n%s", out.String())
	}
}

func TestHitraceConvertPassesTraceDBRetentionOptions(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	var out bytes.Buffer
	var got hitraceconv.Options
	calls := 0
	r := New(Config{
		Runner:   stubRunner{},
		Store:    store,
		Render:   renderNothing,
		RepoRoot: ".",
		Branch:   "main",
		In:       strings.NewReader(""),
		Out:      &out,
		Language: "en",
		HitraceConvert: func(_ context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
			calls++
			got = opts
			return hitraceconv.Result{
				OutputPath:    opts.OutputPath,
				EventsWritten: 1,
			}, nil
		},
	})

	r.handleHitraceCmd("/htrace convert --keep-trace-db --trace-db-output out.trace.db --trace-engine=trace_streamer input.htrace out.systrace")

	if calls != 1 {
		t.Fatalf("converter calls = %d, want 1; output:\n%s", calls, out.String())
	}
	if !got.KeepTraceDB || got.TraceDBOutputPath != "out.trace.db" {
		t.Fatalf("REPL did not pass trace DB retention options: %+v", got)
	}
	if got.InputPath != "input.htrace" || got.OutputPath != "out.systrace" || got.TraceEngine != "trace_streamer" {
		t.Fatalf("unexpected convert opts: %+v", got)
	}
}

func TestHitraceConvertRejectsMalformedTraceEngineBeforeConversion(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	var out bytes.Buffer
	calls := 0
	r := New(Config{
		Runner:   stubRunner{},
		Store:    store,
		Render:   renderNothing,
		RepoRoot: ".",
		Branch:   "main",
		In:       strings.NewReader(""),
		Out:      &out,
		Language: "en",
		HitraceConvert: func(_ context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
			calls++
			return hitraceconv.Result{}, nil
		},
	})

	r.handleHitraceCmd("/htrace convert --trace-engine input.htrace out.systrace")

	if calls != 0 {
		t.Fatalf("malformed trace-engine should not call converter, calls=%d", calls)
	}
	if !strings.Contains(out.String(), "--trace-engine=trace_streamer|builtin|auto") ||
		!strings.Contains(out.String(), "--keep-trace-db") ||
		!strings.Contains(out.String(), "--trace-db-output") {
		t.Fatalf("malformed trace-engine should show convert usage:\n%s", out.String())
	}
}

func TestHitraceConvertArtifactDetailFollowsChineseLanguage(t *testing.T) {
	detail := hitraceConvertArtifactDetail("zh", hitraceconv.Artifact{
		Type:      hitraceconv.ArtifactPerfData,
		Path:      "hiprofiler_data.htrace.perf.data",
		Bytes:     15700680,
		Converter: "hitraceconv-v1",
		Caveats: []string{
			"raw perf.data sidecar preserved; normalized .perftrace was generated for trace_query CPU-sample aggregation",
		},
	})
	for _, want := range []string{"格式=二进制 perf.data sidecar", "字节=15700680", "转换器=hitraceconv-v1", "提示=raw perf.data sidecar 已保留"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("zh detail missing %q: %s", want, detail)
		}
	}
	for _, leak := range []string{"bytes=", "converter=", "caveats=", "raw perf.data sidecar preserved"} {
		if strings.Contains(detail, leak) {
			t.Fatalf("zh detail leaked English %q: %s", leak, detail)
		}
	}
}

func TestDispatchPropagatesExpandedRequestForOutputTranscript(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	runner := &outputTranscriptRunner{}
	var out bytes.Buffer
	r := New(Config{
		Runner:   runner,
		Store:    store,
		Render:   renderNothing,
		RepoRoot: ".",
		Branch:   "main",
		In:       strings.NewReader(""),
		Out:      &out,
		Language: "en",
	})
	expanded := "请分析下面这段长文本:\n第一行原文\n第二行原文"
	folded := "请分析下面这段长文本:\n[Pasted text #0 +2 lines +20 chars]"

	r.dispatch(expanded, folded)

	if len(runner.transcriptRequests) != 2 {
		t.Fatalf("SetOutputTranscriptRequest calls = %d, want expanded + clear", len(runner.transcriptRequests))
	}
	if got := runner.transcriptRequests[0]; got != expanded {
		t.Fatalf("transcript request = %q, want expanded %q", got, expanded)
	}
	if got := runner.transcriptRequests[1]; got != "" {
		t.Fatalf("transcript request was not cleared after dispatch: %q", got)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("Run calls = %d, want 1", len(runner.requests))
	}
	if strings.Contains(runner.requests[0], folded) || !strings.Contains(runner.requests[0], expanded) {
		t.Fatalf("pipeline request should contain expanded text only:\n%s", runner.requests[0])
	}
	recent := store.Recent()
	if len(recent) == 0 {
		t.Fatalf("expected turn in memory")
	}
	last := recent[len(recent)-1]
	if last.Request != folded {
		t.Fatalf("memory request = %q, want folded display %q", last.Request, folded)
	}
	if last.RequestForSummary != expanded {
		t.Fatalf("memory summary request = %q, want expanded %q", last.RequestForSummary, expanded)
	}
}

func TestHistoryStringsUseExpandedPasteForRecall(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	folded := "请分析下面这段长文本:\n[Pasted text #0 +2 lines +20 chars]"
	expanded := "请分析下面这段长文本:\n第一行原文\n第二行原文"
	if err := store.Append(memory.Turn{
		ID:                "t-folded",
		Request:           folded,
		RequestForSummary: expanded,
		Response:          "ok",
		Kind:              memory.KindPipeline,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	r := &REPL{store: store}

	got := r.historyStrings()
	if len(got) != 1 {
		t.Fatalf("historyStrings len=%d, want 1: %#v", len(got), got)
	}
	if got[0] != expanded {
		t.Fatalf("history recall should use expanded paste, got %q want %q", got[0], expanded)
	}
	if strings.Contains(got[0], "[Pasted text") {
		t.Fatalf("history recall leaked folded placeholder: %q", got[0])
	}
}

func TestHistoryStringsSkipUnrecoverableFoldedPaste(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Append(memory.Turn{
		ID:       "t-stale",
		Request:  "[Pasted text #0 +1 lines +802 chars]",
		Response: "clarify",
		Kind:     memory.KindPipeline,
	}); err != nil {
		t.Fatalf("Append stale: %v", err)
	}
	if err := store.Append(memory.Turn{
		ID:       "t-ok",
		Request:  "正常问题",
		Response: "ok",
		Kind:     memory.KindPipeline,
	}); err != nil {
		t.Fatalf("Append ok: %v", err)
	}
	r := &REPL{store: store}

	got := r.historyStrings()
	if len(got) != 1 || got[0] != "正常问题" {
		t.Fatalf("historyStrings should skip unrecoverable folded paste, got %#v", got)
	}
}

func TestBorderedLineFragments_WrapsOrdinaryProse(t *testing.T) {
	line := strings.Repeat("ordinary prose keeps wrapping ", 4)
	got := borderedLineFragments(line, 32)
	if len(got) < 2 {
		t.Fatalf("ordinary prose should wrap; got %d fragment(s): %q", len(got), got)
	}
	for _, frag := range got {
		if w := displayWidth(frag); w > 32 {
			t.Fatalf("wrapped prose fragment exceeded width: width=%d fragment=%q", w, frag)
		}
	}
}

func TestBorderedLineFragments_PreservesWidePipeTableRow(t *testing.T) {
	line := "| Agent | " + strings.Repeat("explorer inspects repository evidence ", 3) + "|"
	got := borderedLineFragments(line, 40)
	if len(got) != 1 {
		t.Fatalf("wide table row must remain one visual row; got %d: %q", len(got), got)
	}
	if got[0] != line {
		t.Fatalf("wide table row must not be clipped or rewritten; got %q", got[0])
	}
}

func TestBorderedLineFragments_PreservesWideBoxDrawingRow(t *testing.T) {
	line := "│ " + strings.Repeat("planner -> explorer -> finalizer ", 3) + "│"
	got := borderedLineFragments(line, 42)
	if len(got) != 1 {
		t.Fatalf("wide box-drawing row must remain one visual row; got %d: %q", len(got), got)
	}
	if got[0] != line {
		t.Fatalf("wide box-drawing row must not be clipped or rewritten; got %q", got[0])
	}
}

func TestBorderedLineFragments_DoesNotClipIncidentalPipeProse(t *testing.T) {
	line := strings.Repeat("choose read | write based on context ", 3)
	got := borderedLineFragments(line, 36)
	if len(got) < 2 {
		t.Fatalf("incidental pipe prose should still wrap; got %d fragment(s): %q", len(got), got)
	}
}

func TestBorderedLineFragments_PreservesWideANSIVisualRows(t *testing.T) {
	line := "\x1b[31m│ " + strings.Repeat("colored evidence cell ", 4) + "│\x1b[0m"
	got := borderedLineFragments(line, 38)
	if len(got) != 1 {
		t.Fatalf("wide ANSI visual row must remain one visual row; got %d: %q", len(got), got)
	}
	if got[0] != line {
		t.Fatalf("wide ANSI visual row must not be clipped or rewritten; got %q", got[0])
	}
}

func TestBorderedLineFragments_PreservesWideMermaidSourceRows(t *testing.T) {
	line := "    explorerEvaluator -->|dispatches a long grounded mechanism through validated evidence| SubAgentRuntime"
	got := borderedLineFragments(line, 44)
	if len(got) != 1 {
		t.Fatalf("wide mermaid source row must remain one visual row; got %d: %q", len(got), got)
	}
	if got[0] != line {
		t.Fatalf("wide mermaid source row must not be clipped or rewritten; got %q", got[0])
	}
}

func TestBorderedLineFragments_PreservesForcedVisualRows(t *testing.T) {
	line := "    " + strings.Repeat("long raw diagram label without arrows ", 4)
	got := borderedLineFragmentsPreserve(line, 40, true)
	if len(got) != 1 {
		t.Fatalf("forced visual row must remain one physical row; got %d: %q", len(got), got)
	}
	if got[0] != line {
		t.Fatalf("forced visual row must not be clipped or rewritten; got %q", got[0])
	}
}

func TestBorderedResponseLines_PreservesRawFencedVisualBlock(t *testing.T) {
	long := "    " + strings.Repeat("raw diagram node label with no syntactic arrows ", 3)
	resp := strings.Join([]string{
		"before",
		"```text",
		"─── flowchart ───",
		long,
		"```",
		"after",
	}, "\n")
	lines := borderedResponseLines(resp)
	var found bool
	for _, line := range lines {
		if line.text != long {
			continue
		}
		found = true
		if !line.visual {
			t.Fatalf("line inside fenced visual block must be marked visual: %+v", line)
		}
		got := borderedLineFragmentsPreserve(line.text, 40, line.visual)
		if len(got) != 1 || got[0] != long {
			t.Fatalf("fenced visual row must stay byte-identical; got %q", got)
		}
	}
	if !found {
		t.Fatalf("expected long fenced line in response lines: %+v", lines)
	}
}

func TestBorderedResponseLines_PreservesRenderedMermaidRows(t *testing.T) {
	md := render.RenderMermaidBlocks(strings.Join([]string{
		"```mermaid",
		"flowchart LR",
		"    A[very long source collector component] --> B[very long validated answer document renderer]",
		"```",
	}, "\n"))
	renderer := render.New(&bytes.Buffer{}, true)
	rendered := renderer.RenderMarkdown(md)
	lines := borderedResponseLines(rendered)

	seenHeader := false
	seenRenderedRow := false
	for _, line := range lines {
		plain := strings.TrimSpace(stripANSIOnly(line.text))
		if isRenderedCodeBlockHeaderLine(plain) {
			seenHeader = true
			if !line.visual {
				t.Fatalf("rendered diagram header must be visual: %+v", line)
			}
			continue
		}
		if !seenHeader || !strings.ContainsAny(plain, "│┌┐└┘─") {
			continue
		}
		seenRenderedRow = true
		if !line.visual {
			t.Fatalf("rendered diagram row must be visual: %+v", line)
		}
		fragments := borderedLineFragmentsPreserve(line.text, 30, line.visual)
		if len(fragments) != 1 || fragments[0] != line.text {
			t.Fatalf("rendered diagram row must not wrap or rewrite; got %q", fragments)
		}
	}
	if !seenHeader || !seenRenderedRow {
		t.Fatalf("expected rendered mermaid header and rows; markdown=%q rendered=%q lines=%+v", md, rendered, lines)
	}
}

func TestBorderedResponseLines_PreservesMarkdownTableWithoutEdgePipes(t *testing.T) {
	long := "agent | " + strings.Repeat("explorer collects source evidence ", 3)
	resp := strings.Join([]string{
		"name | role",
		"--- | ---",
		long,
	}, "\n")
	lines := borderedResponseLines(resp)
	var found bool
	for _, line := range lines {
		if line.text != long {
			continue
		}
		found = true
		if !line.visual {
			t.Fatalf("markdown table row must be marked visual: %+v", line)
		}
		got := borderedLineFragmentsPreserve(line.text, 40, line.visual)
		if len(got) != 1 || got[0] != long {
			t.Fatalf("markdown table row must stay byte-identical; got %q", got)
		}
	}
	if !found {
		t.Fatalf("expected table row in response lines: %+v", lines)
	}
}

func TestBorderedContentLine_DisablesAutoWrapForWideVisualRows(t *testing.T) {
	line := "│ " + strings.Repeat("wide visual cell ", 5) + "│"
	got := borderedContentLine("│", line, true)
	if !strings.HasPrefix(got, "\x1b[?7l") || !strings.Contains(got, "\x1b[?7h\r\n") {
		t.Fatalf("wide visual terminal row must bracket content with DECAWM off/on: %q", got)
	}
	if !strings.Contains(got, line) {
		t.Fatalf("wide visual terminal row must keep content verbatim: %q", got)
	}
}

func TestWriteBorderedContentLine_DoesNotEmitTerminalControlsToBuffers(t *testing.T) {
	var out strings.Builder
	r := &REPL{out: &out}
	line := "│ " + strings.Repeat("wide visual cell ", 5) + "│"
	r.writeBorderedContentLine("│", line, true, 20)
	if strings.Contains(out.String(), "\x1b[?7l") {
		t.Fatalf("non-terminal test writer must not receive terminal auto-wrap controls: %q", out.String())
	}
}

// TestClearPromptDeclined verifies that /clear's confirmation step
// actually blocks the wipe when the user does not type 'y'. We
// scripted "/clear\nn\n/exit\n" through stdin and check that the
// store still has its turn afterwards.
func TestClearPromptDeclined(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.Append(memory.Turn{ID: "t1", Request: "hello", Response: "hi"}); err != nil {
		t.Fatalf("seed append: %v", err)
	}

	in := strings.NewReader("/clear\nn\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if !strings.Contains(out.String(), "Clear cancelled") {
		t.Errorf("expected 'clear cancelled' in output, got:\n%s", out.String())
	}
	if got := len(store.Recent()); got != 1 {
		t.Errorf("turn was wiped despite declining; recent=%d, want 1", got)
	}
}

// TestClearPromptAccepted verifies the wipe happens when the user
// types 'y'. The complementary test to TestClearPromptDeclined.
func TestClearPromptAccepted(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.Append(memory.Turn{ID: "t1", Request: "hello", Response: "hi"}); err != nil {
		t.Fatalf("seed append: %v", err)
	}

	in := strings.NewReader("/clear\ny\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	r.operationResults = []commandOperationResultRecord{{
		Plan: operation.CommandOperationPlan{ID: "op-result", Status: operation.StatusExecuted},
		Result: operation.CommandOperationResult{
			PlanID: "op-result",
			Status: operation.StatusExecuted,
		},
	}}
	r.providerOperationResults = []providerOperationResultRecord{{
		Plan: operation.Plan{Kind: "demo_provider"},
		Result: providerOperationResult{
			Status:  operation.StatusExecuted,
			Summary: "provider result",
		},
	}}
	r.pendingOperation = &operation.CommandOperationPlan{ID: "op-pending", Status: operation.StatusReady}
	r.operationHistory = []operation.CommandOperationPlan{{ID: "op-history", Status: operation.StatusReady}}
	r.providerWorkflow = &operation.WorkflowInstance{ID: "wf-1"}
	memPath := filepath.Join(dir, "operation", "memory.jsonl")
	r.operationMemory = operation.NewMemoryStore(memPath)
	if err := r.operationMemory.Append(operation.MemoryEntry{
		Workspace: ".",
		OS:        "test",
		Arch:      "test",
		Command:   "demo --version",
		Outcome:   "executed",
		Summary:   "demo learned output",
	}); err != nil {
		t.Fatalf("seed operation memory: %v", err)
	}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if !strings.Contains(out.String(), "Conversation memory, operation context, and learned operation memory cleared") {
		t.Errorf("expected confirmation in output, got:\n%s", out.String())
	}
	if got := len(store.Recent()); got != 0 {
		t.Errorf("turn was not wiped after accepting; recent=%d, want 0", got)
	}
	if r.pendingOperation != nil || r.providerWorkflow != nil || len(r.operationResults) != 0 || len(r.providerOperationResults) != 0 || len(r.operationHistory) != 0 {
		t.Fatalf("operation context was not cleared: pending=%+v workflow=%+v results=%d provider_results=%d history=%d",
			r.pendingOperation, r.providerWorkflow, len(r.operationResults), len(r.providerOperationResults), len(r.operationHistory))
	}
	if _, err := os.Stat(memPath); !os.IsNotExist(err) {
		t.Fatalf("operation memory file still exists after /clear, err=%v", err)
	}
}

// TestClearPromptShowsPeerCount verifies that the warning text
// reflects the number of live peer Stores. With one peer alive in
// the same dir, the prompt must mention "1 other live codrax
// instance".
func TestClearPromptShowsPeerCount(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Spin up a peer Store on the same dir; its sidecar makes it
	// visible to LivePeerCount.
	peer, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("peer NewStore: %v", err)
	}
	defer peer.Close()

	in := strings.NewReader("/clear\nn\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if !strings.Contains(out.String(), "1 other live codrax instance") {
		t.Errorf("expected peer-count warning, got:\n%s", out.String())
	}
}

// TestErrorResponseNotPersistedToMemory is the write-side lock for
// the 2026-04-12 REPL memory-pollution fix. When the pipeline ends
// with a non-empty TaskState.LastError, the REPL must NOT persist
// the full error-laden render into memory — it must persist a
// clean placeholder so subsequent turns' prior-conversation block
// doesn't leak historical failure text into the LLM's context.
func TestErrorResponseNotPersistedToMemory(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("some broken question\n/exit\n")
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:     errorRunner{errText: "task deadbeef stuck at stage explore after 5 visits"},
		Store:      store,
		Render:     renderErrorFromBus,
		RepoRoot:   ".",
		Branch:     "main",
		In:         in,
		Out:        out,
		Prompt:     ">",
		PromptCont: ".",
		Banner:     "test-banner",
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	recent := store.Recent()
	if len(recent) != 1 {
		t.Fatalf("expected 1 turn persisted, got %d", len(recent))
	}
	stored := recent[0].Response
	// The raw error details must not leak into the stored response.
	if strings.Contains(stored, "deadbeef") {
		t.Errorf("task UUID leaked into memory: %q", stored)
	}
	if strings.Contains(stored, "stuck at stage") {
		t.Errorf("raw error phrase leaked into memory: %q", stored)
	}
	// The clean placeholder should be what got stored.
	if !strings.Contains(stored, "previous attempt ended in error") {
		t.Errorf("expected placeholder in stored response, got: %q", stored)
	}
}

// renderErrorFromBus mimics the real renderer's behavior for an
// errored BusContext: prepends "error: <LastError>" to whatever
// answer text exists. Kept separate from renderNothing so the
// error-response test can exercise the render→recordTurn path.
func renderErrorFromBus(bc *types.BusContext) string {
	if bc == nil {
		return ""
	}
	if bc.TaskState.LastError != "" {
		return "\n  error: " + bc.TaskState.LastError + "\n"
	}
	return ""
}

func renderResultFromBus(bc *types.BusContext) string {
	if bc == nil || bc.Mutable == nil {
		return ""
	}
	return bc.Mutable.Result()
}

func renderMermaidTerminalFromBus(bc *types.BusContext) string {
	if bc == nil || bc.Mutable == nil {
		return ""
	}
	return render.RenderMermaidBlocks(bc.Mutable.Result())
}

func TestPipelineMemoryStoresRawMarkdownNotTerminalMermaidRender(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("draw it\n/exit\n")
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:     mermaidAnswerRunner{},
		Store:      store,
		Render:     renderMermaidTerminalFromBus,
		RepoRoot:   ".",
		Branch:     "main",
		In:         in,
		Out:        out,
		Prompt:     ">",
		PromptCont: ".",
		Banner:     "test-banner",
		Language:   "en",
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	recent := store.Recent()
	if len(recent) != 1 {
		t.Fatalf("expected one persisted turn, got %d", len(recent))
	}
	if !strings.Contains(recent[0].Response, "```mermaid") {
		t.Fatalf("memory must keep raw Mermaid markdown, got %q", recent[0].Response)
	}
	if strings.Contains(recent[0].Response, "```text") {
		t.Fatalf("terminal Mermaid rendering must not be persisted to memory: %q", recent[0].Response)
	}
}

func TestPipelineResponseShowsMarkdownOutputPathWithoutPersistingIt(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("answer me\n/exit\n")
	out := &bytes.Buffer{}
	preview := &stubMarkdownPreviewer{url: "http://127.0.0.1:49152/preview/abc?token=t"}
	r := New(Config{
		Runner: markdownPathRunner{
			path:     ".codrax/output/20260516-120000.000-42.md",
			htmlPath: ".codrax/output/20260516-120000.000-42.html",
		},
		Store:           store,
		Render:          renderResultFromBus,
		RepoRoot:        ".",
		Branch:          "main",
		In:              in,
		Out:             out,
		Prompt:          ">",
		PromptCont:      ".",
		Banner:          "test-banner",
		Language:        "en",
		MarkdownPreview: preview,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	printed := out.String()
	if !strings.Contains(printed, "Raw Markdown saved: .codrax/output/20260516-120000.000-42.md") {
		t.Fatalf("markdown path hint missing from REPL output:\n%s", printed)
	}
	if !strings.Contains(printed, "Standalone HTML saved: .codrax/output/20260516-120000.000-42.html") {
		t.Fatalf("html path hint missing from REPL output:\n%s", printed)
	}
	if !strings.Contains(printed, "Browser preview: http://127.0.0.1:49152/preview/abc?token=t") {
		t.Fatalf("markdown preview hint missing from REPL output:\n%s", printed)
	}
	if preview.path != ".codrax/output/20260516-120000.000-42.md" {
		t.Fatalf("preview registered path = %q", preview.path)
	}
	recent := store.Recent()
	if len(recent) != 1 {
		t.Fatalf("expected one persisted turn, got %d", len(recent))
	}
	if strings.Contains(recent[0].Response, "Raw Markdown saved") ||
		strings.Contains(recent[0].Response, "Standalone HTML saved") ||
		strings.Contains(recent[0].Response, "Browser preview") ||
		strings.Contains(recent[0].Response, ".codrax/output") {
		t.Fatalf("markdown path hint should not be persisted to memory: %q", recent[0].Response)
	}
}

func TestFinalAnswerMarkdownNoticeLocalizes(t *testing.T) {
	mut := types.NewMutableState("probe")
	mut.SetFinalAnswerOutputPaths(".codrax/output/a.md", ".codrax/output/a.html")
	bus := &types.BusContext{Mutable: mut}

	if got := finalAnswerMarkdownNotice(bus, "zh"); got != "Markdown 原文已保存：.codrax/output/a.md" {
		t.Fatalf("zh notice = %q", got)
	}
	if got := finalAnswerMarkdownNotice(bus, "en"); got != "Raw Markdown saved: .codrax/output/a.md" {
		t.Fatalf("en notice = %q", got)
	}
	if got := finalAnswerHTMLNotice(bus, "zh"); got != "HTML 单页已保存：.codrax/output/a.html" {
		t.Fatalf("zh html notice = %q", got)
	}
	if got := finalAnswerHTMLNotice(bus, "en"); got != "Standalone HTML saved: .codrax/output/a.html" {
		t.Fatalf("en html notice = %q", got)
	}
}

// TestPasteSlashCapturesToPending verifies the /paste command
// collects lines via bufio until `/end`, and stashes the capture in
// r.pendingPaste ready for the next interactive readInput to
// consume. Scripted mode skips the bubbletea seeding step but the
// capture path is identical.
func TestPasteSlashCapturesToPending(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("/paste\nfunc foo() {\n    return 42\n}\n/end\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	want := "func foo() {\n    return 42\n}"
	if r.pendingPaste != want {
		t.Errorf("pendingPaste = %q, want %q", r.pendingPaste, want)
	}
	if !strings.Contains(out.String(), "captured 3 lines") {
		t.Errorf("expected 'captured 3 lines' in output, got:\n%s", out.String())
	}
}

// TestPasteSlashEmptyCaptureNoOp verifies that pressing /end
// immediately without content leaves pendingPaste empty.
func TestPasteSlashEmptyCaptureNoOp(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("/paste\n/end\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if r.pendingPaste != "" {
		t.Errorf("pendingPaste should be empty on no-input capture, got %q", r.pendingPaste)
	}
	if !strings.Contains(out.String(), "No input captured") {
		t.Errorf("expected 'no input captured' in output, got:\n%s", out.String())
	}
}

// TestAutoRoutedLogIsOneShot verifies C5: a log body auto-detected
// inside a request is attached for that single dispatch and then
// cleared, so subsequent turns do not re-run log_triage against the
// same bytes. Explicit /log path stays sticky (covered separately).
func TestAutoRoutedLogIsOneShot(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Turn 1 embeds a 3-line timestamped log block that splitPastedLog
	// detects. Turn 2 is an unrelated follow-up. Line continuations
	// ("\\") let a single logical request span multiple input lines.
	input := "analyze this log \\\n" +
		"2026-04-21T14:54:37.362 INFO [x] dispatching agent=a skill=s \\\n" +
		"2026-04-21T14:54:56.671 INFO [x] dispatching agent=b skill=t \\\n" +
		"2026-04-21T14:56:00.822 INFO [x] dispatching agent=c skill=u\n" +
		"follow-up question\n" +
		"/exit\n"
	in := strings.NewReader(input)
	out := &bytes.Buffer{}
	runner := &logAwareRunner{}
	r := New(Config{
		Runner:     runner,
		Store:      store,
		Render:     renderNothing,
		RepoRoot:   ".",
		Branch:     "main",
		In:         in,
		Out:        out,
		Prompt:     ">",
		PromptCont: ".",
		Banner:     "test-banner",
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	// Two dispatches happened.
	if got := len(runner.seenLogs); got != 2 {
		t.Fatalf("expected 2 dispatches, got %d: %v", got, runner.seenLogs)
	}
	// Turn 1 saw the log at Run() entry.
	if runner.seenLogs[0] == "" {
		t.Errorf("turn 1 should have seen auto-routed log; got empty")
	}
	if !strings.Contains(runner.seenLogs[0], "14:54:37") {
		t.Errorf("turn 1 log missing expected timestamp; got %q", runner.seenLogs[0])
	}
	// Turn 2 saw an empty log — one-shot clear fired.
	if runner.seenLogs[1] != "" {
		t.Errorf("turn 2 should see no attached log after one-shot clear; got %q", runner.seenLogs[1])
	}
	// REPL internal state after exit: both flag and value cleared.
	if r.attachedLog != "" {
		t.Errorf("attachedLog not cleared after one-shot turn; got %q", r.attachedLog)
	}
	if r.attachedLogAutoRouted {
		t.Errorf("attachedLogAutoRouted should be false after clear")
	}
}

// TestExplicitLogStaysStickyAcrossTurns is the positive counterpart
// to TestAutoRoutedLogIsOneShot: an explicit /log paste-capture
// attachment must survive into subsequent dispatches so users can
// keep asking about the same panic from different angles.
func TestExplicitLogStaysStickyAcrossTurns(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// /log (paste mode) captures three log lines, then two questions
	// run back to back. Both dispatches must see the same attachedLog.
	input := "/log\n" +
		"2026-04-21T14:54:37.362 INFO [x] dispatching\n" +
		"2026-04-21T14:54:56.671 INFO [x] dispatching\n" +
		"2026-04-21T14:56:00.822 INFO [x] dispatching\n" +
		"/end\n" +
		"first question about the log\n" +
		"second question about the log\n" +
		"/exit\n"
	in := strings.NewReader(input)
	out := &bytes.Buffer{}
	runner := &logAwareRunner{}
	r := New(Config{
		Runner:     runner,
		Store:      store,
		Render:     renderNothing,
		RepoRoot:   ".",
		Branch:     "main",
		In:         in,
		Out:        out,
		Prompt:     ">",
		PromptCont: ".",
		Banner:     "test-banner",
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if got := len(runner.seenLogs); got != 2 {
		t.Fatalf("expected 2 dispatches, got %d: %v", got, runner.seenLogs)
	}
	for i, got := range runner.seenLogs {
		if !strings.Contains(got, "14:54:37") {
			t.Errorf("turn %d should see sticky log with timestamp; got %q", i+1, got)
		}
	}
	if r.attachedLogAutoRouted {
		t.Errorf("explicit /log should never set attachedLogAutoRouted")
	}
	if r.attachedLog == "" {
		t.Errorf("attachedLog should still be set after /exit")
	}
}

// TestMultilineInput verifies that lines ending with \ are joined
// into a single multi-line request.
func TestMultilineInput(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("hello \\\nworld\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	recent := store.Recent()
	if len(recent) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(recent))
	}
	if !strings.Contains(recent[0].Request, "hello") || !strings.Contains(recent[0].Request, "world") {
		t.Errorf("expected multiline request with both parts, got: %q", recent[0].Request)
	}
	if !strings.Contains(recent[0].Request, "\n") {
		t.Errorf("expected newline in multiline request, got: %q", recent[0].Request)
	}
	// Continuation prompt should appear in output.
	if !strings.Contains(out.String(), ".") {
		t.Errorf("expected continuation prompt in output")
	}
}

// TestMultilineThreeLines verifies continuation across three lines.
func TestMultilineThreeLines(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("aaa \\\nbbb \\\nccc\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	recent := store.Recent()
	if len(recent) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(recent))
	}
	req := recent[0].Request
	if !strings.Contains(req, "aaa") || !strings.Contains(req, "bbb") || !strings.Contains(req, "ccc") {
		t.Errorf("expected all three parts in request, got: %q", req)
	}
	// Should have exactly 2 newlines (3 lines joined).
	if strings.Count(req, "\n") != 2 {
		t.Errorf("expected 2 newlines in request, got %d in: %q", strings.Count(req, "\n"), req)
	}
}

func TestBackslashQuitAliasHandledLocally(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("\\q\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if got := len(store.Recent()); got != 0 {
		t.Fatalf("\\q should exit before dispatch, persisted turns=%d", got)
	}
	if !strings.Contains(out.String(), "Goodbye!") {
		t.Errorf("expected goodbye output for \\q, got:\n%s", out.String())
	}
}

// TestVersionSlashCommand verifies the /version REPL command prints
// the build identifier passed in via Config and continues the loop.
// Also covers the /v alias and the legacy backslash form.
func TestVersionSlashCommand(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("/version\n/v\n\\version\n/exit\n")
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:     stubRunner{},
		Store:      store,
		Render:     renderNothing,
		RepoRoot:   ".",
		Branch:     "main",
		In:         in,
		Out:        out,
		Prompt:     ">",
		PromptCont: ".",
		Banner:     "test-banner",
		Version:    "1.2.3-test",
		BuildTime:  "2026-04-22T00:00:00Z",
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	got := out.String()
	want := "codrax 1.2.3-test (built 2026-04-22T00:00:00Z)"
	if c := strings.Count(got, want); c != 3 {
		t.Errorf("expected /version output to appear 3 times (one per alias), got %d:\n%s", c, got)
	}
	if n := len(store.Recent()); n != 0 {
		t.Errorf("/version must not record a turn, recent=%d", n)
	}
}

// TestStickyTag_PropagatesToPasteAndLogPasteModes locks that capture-
// mode prompts (`/paste`, `/log paste`) prepend the sticky tag so a
// user typing into paste mode while in write mode does not lose the
// [task:write] context. Pre-fix path printed bare "paste> "/"log> ".
func TestStickyTag_PropagatesToPasteAndLogPasteModes(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// /mode write flips userMode to the sticky write lane; then /paste enters capture and
	// expects "[task:write] paste> " in its prompt.
	in := strings.NewReader("/mode write\n/paste\n/end\n/exit\n")
	out := &bytes.Buffer{}
	r := New(Config{
		Runner: stubRunner{}, Store: store, Render: renderNothing,
		RepoRoot: ".", Branch: "main", In: in, Out: out,
		Banner: "test", WriteEnabled: true,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "[task:write]") {
		t.Errorf("scripted mode echo lost [task:write]; out:\n%s", got)
	}
	// /paste prompt was wired to scripted scanner so the prompt itself
	// is only printed when r.interactive(); the substring assertion is
	// thus on the [task:write] tag in OTHER places (echo, banner) — the
	// scripted-mode path tests the wiring; the interactive prompt path
	// is exercised by inspecting r.currentStickyTag at handler time.
	r.userMode = UserModeWrite
	r.currentMode = types.ModePlan
	r.attachedLog = "x"
	if tag := r.currentStickyTag(); !strings.Contains(tag, "[task:write]") || !strings.Contains(tag, "[log]") {
		t.Errorf("currentStickyTag must compose mode + attachments; got %q", tag)
	}
	r.attachedLog = "# codrax-source: first.log\none\n# codrax-source: second.log\ntwo"
	r.attachedHitrace = "# codrax-source: frame.systrace\ntrace\n# codrax-source: frame.perftrace\nperf_sample: pid=1 tid=1"
	if tag := r.currentStickyTag(); !strings.Contains(tag, "[log:2]") || !strings.Contains(tag, "[trace:2+perf]") {
		t.Errorf("currentStickyTag should expose multi runtime artifact state; got %q", tag)
	}
}

// TestStickyTag_RendersInScriptedMode pins the contract that the
// per-turn sticky-state tag (task/phase/log/trace/plan/mem!) reaches the
// scripted-mode output stream, not just the Bubble Tea path. Real
// bug: readInputLines used to ignore its caller's prompt and fall
// back to r.prompt, so `/mode write` switched the mode but the next
// turn's prompt still rendered as bare `>` with no `[task:write]`
// indicator visible to anyone tailing the session.
func TestStickyTag_RendersInScriptedMode(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// /mode write flips userMode to the sticky write lane; the next turn's prompt must
	// carry [task:write]. /exit ends the loop after one dispatch.
	in := strings.NewReader("/mode write\nsome request\n/exit\n")
	out := &bytes.Buffer{}
	r := New(Config{
		Runner: stubRunner{}, Store: store, Render: renderNothing,
		RepoRoot: ".", Branch: "main", In: in, Out: out,
		Prompt: ">", PromptCont: ".", Banner: "test-banner",
		WriteEnabled: true,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if !strings.Contains(out.String(), "[task:write]") {
		t.Errorf("expected [task:write] sticky tag in prompt; out:\n%s", out.String())
	}
}

// TestVersionConfigDefaults verifies an empty Version/BuildTime in
// Config falls back to "dev" / "unknown" so a `go run` build still
// produces a coherent /version line.
func TestVersionConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("/version\n/exit\n")
	out := &bytes.Buffer{}
	r := New(Config{
		Runner: stubRunner{}, Store: store, Render: renderNothing,
		RepoRoot: ".", Branch: "main", In: in, Out: out,
		Prompt: ">", PromptCont: ".", Banner: "test-banner",
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if !strings.Contains(out.String(), "codrax dev (built unknown)") {
		t.Errorf("expected dev/unknown fallback, got:\n%s", out.String())
	}
}

// TestHumanByteSize pins the banner digest's unit selection so a
// tiny memory (a few bytes) does not show up as "0.0 KB" and a
// real multi-MB directory does not overflow the single-line format.
func TestHumanByteSize(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0 B"},
		{42, "42 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048575, "1024.0 KB"}, // boundary: below MB threshold
		{1048576, "1.0 MB"},
		{5 * 1024 * 1024, "5.0 MB"},
	}
	for _, c := range cases {
		if got := humanByteSize(c.in); got != c.want {
			t.Errorf("humanByteSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBannerIncludesLocalizedModelAndStatusLines(t *testing.T) {
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Append(memory.Turn{
		ID:        "turn-1",
		Request:   "hello",
		Response:  "hi",
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	for _, tc := range []struct {
		name      string
		lang      string
		listLine  string
		modelLine string
		wantOrder []string
	}{
		{
			name:      "en",
			lang:      "en",
			listLine:  "Available models: 1. MiniMax-M2.7 ← selected",
			modelLine: "Model: MiniMax-M2.7 · ctx=200k",
			wantOrder: []string{"Available models:", "Model:", "Modes:", "Memory:"},
		},
		{
			name:      "zh",
			lang:      "zh",
			listLine:  "可用模型: 1. MiniMax-M2.7 ← 已选择",
			modelLine: "模型: MiniMax-M2.7 · 上下文=200k",
			wantOrder: []string{"可用模型:", "模型:", "模式:", "记忆:"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			r := New(Config{
				Runner:           stubRunner{},
				Store:            store,
				Render:           renderNothing,
				RepoRoot:         ".",
				Branch:           "main",
				In:               strings.NewReader(""),
				Out:              &out,
				Language:         tc.lang,
				ModelListLine:    tc.listLine,
				ModelSummaryLine: tc.modelLine,
			})

			r.banner()

			got := out.String()
			if !strings.Contains(got, "CODRAX") {
				t.Fatalf("banner should render the CODRAX header for the REPL status section; got:\n%s", got)
			}
			last := -1
			for _, needle := range tc.wantOrder {
				idx := strings.Index(got, needle)
				if idx < 0 {
					t.Fatalf("banner missing %q; got:\n%s", needle, got)
				}
				if idx <= last {
					t.Fatalf("banner order wrong around %q; got:\n%s", needle, got)
				}
				last = idx
			}
		})
	}
}

func TestPrintOAuthAuthorizationPrompt(t *testing.T) {
	var out bytes.Buffer
	PrintOAuthAuthorizationPrompt(&out, "zh", "https://auth.example/authorize")
	got := out.String()
	if !strings.Contains(got, "需要完成 LLM OAuth 授权") {
		t.Fatalf("zh OAuth prompt missing localized text: %q", got)
	}
	if !strings.Contains(got, "https://auth.example/authorize") {
		t.Fatalf("OAuth prompt missing URL: %q", got)
	}
	if !strings.Contains(got, "›") {
		t.Fatalf("OAuth prompt should use REPL-style prefix: %q", got)
	}
}

func TestPrintOAuthAuthorizationComplete(t *testing.T) {
	var out bytes.Buffer
	PrintOAuthAuthorizationComplete(&out, "zh")
	got := out.String()
	if !strings.Contains(got, "LLM OAuth 授权已完成") {
		t.Fatalf("zh OAuth complete notice missing localized text: %q", got)
	}
	if !strings.Contains(got, "✓") {
		t.Fatalf("OAuth complete notice should use success marker: %q", got)
	}
}

func TestPrintTopologyDiscoveryNotices(t *testing.T) {
	var out bytes.Buffer
	PrintTopologyDiscoveryStart(&out, "zh")
	PrintTopologyDiscoveryComplete(&out, "zh", 2, "18ms")
	PrintTopologyDiscoveryError(&out, "en", assertErr("boom"), "19ms")
	got := out.String()
	for _, want := range []string{
		"正在发现工作区子仓拓扑",
		"工作区子仓拓扑已就绪：2 个子仓",
		"--multi-repo=false",
		"Workspace topology discovery failed: boom",
		"·",
		"✓",
		"✗",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("topology notice missing %q:\n%s", want, got)
		}
	}
}

func TestBannerSkipsStartupHeaderWhenAlreadyPrinted(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	var out bytes.Buffer
	r := New(Config{
		Runner:           stubRunner{},
		Store:            store,
		Render:           renderNothing,
		RepoRoot:         ".",
		Branch:           "main",
		In:               strings.NewReader(""),
		Out:              &out,
		Language:         "zh",
		HeaderPrinted:    true,
		ModelSummaryLine: "模型: MiniMax-M2.7 · 上下文=200k",
	})

	r.banner()

	got := out.String()
	if strings.Contains(got, "CODRAX") {
		t.Fatalf("banner should not repeat CODRAX header after startup prelude printed it:\n%s", got)
	}
	if !strings.Contains(got, "模型: MiniMax-M2.7") || !strings.Contains(got, "模式:") {
		t.Fatalf("banner should still render status rows after skipped header:\n%s", got)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
