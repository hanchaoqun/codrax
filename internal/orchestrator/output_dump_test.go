package orchestrator

import (
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
	if !strings.Contains(body, "# 问题\n\npanic source\n") {
		t.Fatalf("missing 问题 section:\n%s", body)
	}
	if !strings.Contains(body, "# 回答\n\nSection A\n\nSection B\n") {
		t.Fatalf("missing 回答 section:\n%s", body)
	}
	if strings.Contains(body, "附件") {
		t.Fatalf("attachment footnote unexpectedly present:\n%s", body)
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
	if !strings.Contains(string(got), "# 问题\n\nwhat does X do") {
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
	if !strings.Contains(string(body), "# 问题\n\n"+expanded) ||
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
	if !strings.Contains(text, "# 问题\n\n"+secondTurn+"\n") {
		t.Fatalf("markdown dump did not use current run transcript request:\n%s", text)
	}
	if strings.Contains(text, "# 问题\n\n"+firstTurn+"\n") {
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
