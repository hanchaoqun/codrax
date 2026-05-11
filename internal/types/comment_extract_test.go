package types

import (
	"strings"
	"testing"
)

func TestExtractLeadingDocComment_Go(t *testing.T) {
	src := []byte(strings.Join([]string{
		"package foo",
		"",
		"// Bar coordinates the storm pipeline by deciding when each",
		"// stage emits and persisting the rolling baseline.",
		"func Bar() {",
		"    return",
		"}",
	}, "\n"))
	// `func Bar() {` is line 5 (1-based).
	text, line := ExtractLeadingDocComment(src, 5, "x.go")
	if text == "" {
		t.Fatalf("expected non-empty doc comment")
	}
	if !strings.Contains(text, "Bar coordinates the storm pipeline") {
		t.Fatalf("missing first sentence: %q", text)
	}
	if !strings.Contains(text, "rolling baseline") {
		t.Fatalf("missing second sentence: %q", text)
	}
	if line != 3 {
		t.Fatalf("expected startLine=3, got %d", line)
	}
}

func TestExtractLeadingDocComment_Java(t *testing.T) {
	src := []byte(strings.Join([]string{
		"package com.example;",
		"",
		"/**",
		" * StorageGateway abstracts the persistence layer.",
		" * Provides idempotent put/get with retry.",
		" */",
		"public class StorageGateway {",
		"    public void put() {}",
		"}",
	}, "\n"))
	text, line := ExtractLeadingDocComment(src, 7, "x.java")
	if text == "" {
		t.Fatalf("expected non-empty doc comment")
	}
	if !strings.Contains(text, "StorageGateway abstracts the persistence layer") {
		t.Fatalf("missing first sentence: %q", text)
	}
	if !strings.Contains(text, "idempotent") {
		t.Fatalf("missing second sentence: %q", text)
	}
	if line != 3 {
		t.Fatalf("expected startLine=3 (line of /**), got %d", line)
	}
}

func TestExtractLeadingDocComment_PythonDocstring(t *testing.T) {
	src := []byte(strings.Join([]string{
		"class Worker:",
		"    def run(self):",
		`        """Drain the queue, dispatching one job per iteration."""`,
		"        pass",
	}, "\n"))
	text, line := ExtractLeadingDocComment(src, 2, "w.py")
	if text == "" {
		t.Fatalf("expected non-empty docstring")
	}
	if !strings.Contains(text, "Drain the queue") {
		t.Fatalf("missing docstring text: %q", text)
	}
	if line != 3 {
		t.Fatalf("expected startLine=3, got %d", line)
	}
}

func TestExtractLeadingDocComment_PythonDocstringMultiline(t *testing.T) {
	src := []byte(strings.Join([]string{
		"class Worker:",
		"    def run(self):",
		`        """Drain the queue.`,
		"",
		"        Dispatches one job per iteration until empty.",
		`        """`,
		"        pass",
	}, "\n"))
	text, line := ExtractLeadingDocComment(src, 2, "w.py")
	if text == "" {
		t.Fatalf("expected non-empty docstring")
	}
	if !strings.Contains(text, "Drain the queue") {
		t.Fatalf("missing docstring text: %q", text)
	}
	if !strings.Contains(text, "Dispatches one job per iteration") {
		t.Fatalf("missing docstring tail: %q", text)
	}
	if line != 3 {
		t.Fatalf("expected startLine=3, got %d", line)
	}
}

func TestExtractLeadingDocComment_PythonHashFallback(t *testing.T) {
	src := []byte(strings.Join([]string{
		"# Worker drains the job queue, one job per iteration.",
		"def run():",
		"    pass",
	}, "\n"))
	text, line := ExtractLeadingDocComment(src, 2, "w.py")
	if text == "" {
		t.Fatalf("expected non-empty hash comment")
	}
	if !strings.Contains(text, "Worker drains the job queue") {
		t.Fatalf("missing comment text: %q", text)
	}
	if line != 1 {
		t.Fatalf("expected startLine=1, got %d", line)
	}
}

func TestExtractLeadingDocComment_RustTripleSlash(t *testing.T) {
	src := []byte(strings.Join([]string{
		"/// EventLoop polls the registered listeners and",
		"/// invokes their callbacks in registration order.",
		"pub fn event_loop() {",
		"    todo!()",
		"}",
	}, "\n"))
	text, line := ExtractLeadingDocComment(src, 3, "x.rs")
	if text == "" {
		t.Fatalf("expected non-empty doc comment")
	}
	if !strings.Contains(text, "EventLoop polls the registered listeners") {
		t.Fatalf("missing text: %q", text)
	}
	if line != 1 {
		t.Fatalf("expected startLine=1, got %d", line)
	}
}

func TestExtractLeadingDocComment_LongBlockRejected(t *testing.T) {
	// 14-line comment block (exceeds maxInLines=12) — typical of
	// a license/copyright header. Must return empty.
	var b strings.Builder
	for i := 0; i < 14; i++ {
		b.WriteString("// line ")
		b.WriteString(strings.Repeat("a", 5))
		b.WriteString("\n")
	}
	b.WriteString("func F() {}\n")
	text, _ := ExtractLeadingDocComment([]byte(b.String()), 15, "x.go")
	if text != "" {
		t.Fatalf("expected empty (length filter), got %q", text)
	}
}

func TestExtractLeadingDocComment_NoComment(t *testing.T) {
	src := []byte(strings.Join([]string{
		"package foo",
		"",
		"func Bar() {}",
	}, "\n"))
	text, line := ExtractLeadingDocComment(src, 3, "x.go")
	if text != "" || line != 0 {
		t.Fatalf("expected empty result, got text=%q line=%d", text, line)
	}
}

func TestExtractLeadingDocComment_UnknownExt(t *testing.T) {
	src := []byte("// hello world\nthing")
	text, line := ExtractLeadingDocComment(src, 2, "x.unknownext")
	if text != "" || line != 0 {
		t.Fatalf("expected empty for unsupported ext, got %q line=%d", text, line)
	}
}

func TestExtractLeadingDocComment_TruncatesToMaxOutChars(t *testing.T) {
	// Build a comment whose payload exceeds 300 chars but stays
	// within input limits (≤12 lines, ≤800 chars). Verify the
	// returned text is clipped and ends sensibly.
	var b strings.Builder
	for i := 0; i < 8; i++ {
		// 6 chars prefix "// abc" + 50-char payload = ~56 chars/line; 8 lines ≈ 448 chars.
		// After // strip + trim: ~50 chars/line × 8 = ~400 chars joined.
		b.WriteString("// ")
		b.WriteString(strings.Repeat("payload-text-", 4))
		b.WriteString("\n")
	}
	b.WriteString("func F() {}\n")
	text, _ := ExtractLeadingDocComment([]byte(b.String()), 9, "x.go")
	if text == "" {
		t.Fatalf("expected non-empty (within input limits), got empty")
	}
	if len([]rune(text)) > maxOutChars+4 {
		t.Fatalf("expected truncation to ~%d chars, got %d: %q", maxOutChars, len(text), text)
	}
}

func TestExtractLeadingDocComment_TypeScript(t *testing.T) {
	src := []byte(strings.Join([]string{
		"// Dispatcher routes incoming intents to the matching",
		"// handler registry; falls back to GenericHandler.",
		"export class Dispatcher {",
		"  route() {}",
		"}",
	}, "\n"))
	text, line := ExtractLeadingDocComment(src, 3, "x.ts")
	if text == "" {
		t.Fatalf("expected non-empty doc comment for ts file")
	}
	if !strings.Contains(text, "Dispatcher routes incoming intents") {
		t.Fatalf("missing text: %q", text)
	}
	if line != 1 {
		t.Fatalf("expected startLine=1, got %d", line)
	}
}

func TestExtractLeadingDocComment_ArkTSDecoratorStack(t *testing.T) {
	src := []byte(strings.Join([]string{
		"// Source: developer.huawei.com / openharmony Index.ets minimal sample",
		"// Surface: @Entry + @Component + build()",
		"",
		"@Entry",
		"@Component",
		"struct Index {",
		"  build() {}",
		"}",
	}, "\n"))
	text, line := ExtractLeadingDocComment(src, 6, "Index.ets")
	if text == "" {
		t.Fatalf("expected decorator-aware doc comment")
	}
	if !strings.Contains(text, "Index.ets minimal sample") {
		t.Fatalf("missing source filename alias: %q", text)
	}
	if !strings.Contains(text, "@Entry + @Component") {
		t.Fatalf("missing decorator surface: %q", text)
	}
	if line != 1 {
		t.Fatalf("expected startLine=1, got %d", line)
	}
}

func TestExtractLeadingDocComment_MultiLanguageMetadataStacks(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		lines     []string
		lineStart int
		want      string
	}{
		{
			name: "python decorator",
			path: "mod.py",
			lines: []string{
				"# build_card renders the reusable card body.",
				"@builder",
				"def build_card():",
				"    pass",
			},
			lineStart: 3,
			want:      "reusable card body",
		},
		{
			name: "java annotation",
			path: "Gateway.java",
			lines: []string{
				"/**",
				" * Gateway handles request fanout.",
				" */",
				"@Deprecated",
				"public class Gateway {}",
			},
			lineStart: 5,
			want:      "request fanout",
		},
		{
			name: "rust attribute",
			path: "lib.rs",
			lines: []string{
				"/// Engine coordinates render passes.",
				"#[derive(Debug)]",
				"pub struct Engine;",
			},
			lineStart: 3,
			want:      "render passes",
		},
		{
			name: "cpp attribute",
			path: "engine.cpp",
			lines: []string{
				"// render_fast returns the cached result.",
				"[[nodiscard]]",
				"int render_fast();",
			},
			lineStart: 3,
			want:      "cached result",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, _ := ExtractLeadingDocComment([]byte(strings.Join(tc.lines, "\n")), tc.lineStart, tc.path)
			if !strings.Contains(text, tc.want) {
				t.Fatalf("ExtractLeadingDocComment() = %q, want substring %q", text, tc.want)
			}
		})
	}
}

func TestExtractLeadingDocComment_BlankLineAboveDef(t *testing.T) {
	// Single blank line between comment and def should still be
	// captured (Go's gofmt-respecting style).
	src := []byte(strings.Join([]string{
		"package foo",
		"",
		"// Bar handles the request.",
		"",
		"func Bar() {}",
	}, "\n"))
	text, line := ExtractLeadingDocComment(src, 5, "x.go")
	if text == "" {
		t.Fatalf("expected non-empty (blank tolerated)")
	}
	if !strings.Contains(text, "Bar handles the request") {
		t.Fatalf("missing text: %q", text)
	}
	if line != 3 {
		t.Fatalf("expected startLine=3, got %d", line)
	}
}
