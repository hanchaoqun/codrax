package outputdump

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildBodyNormalizesAnswerMermaidFences(t *testing.T) {
	body := BuildBody(Args{
		Request: "show a diagram",
		Answer: strings.Join([]string{
			"```mermaid",
			"flowchart TD",
			`    ../A.md -->|success (measurement==true)| B[preStages\n(Conditional)]`,
			"```",
		}, "\n"),
	})
	if !strings.Contains(body, `codraxNode1["../A.md"] -->|"success (measurement==true)"| B["preStages<br/>(Conditional)"]`) {
		t.Fatalf("answer Mermaid fence was not normalized in output dump:\n%s", body)
	}
}

func TestBuildBodyPreservesQuestionMermaidSource(t *testing.T) {
	request := strings.Join([]string{
		"why does this fail?",
		"```mermaid",
		"flowchart TD",
		`    ../A.md -->|success (measurement==true)| B`,
		"```",
	}, "\n")
	body := BuildBody(Args{
		Request: request,
		Answer:  "plain answer",
	})
	if !strings.Contains(body, request) {
		t.Fatalf("request Mermaid source should remain verbatim:\n%s", body)
	}
}

func TestBuildBodyRendersRuntimeArtifactTable(t *testing.T) {
	trace := strings.Join([]string{
		"# codrax-source: frame.systrace",
		"sched_switch",
		"# codrax-source: frame.perftrace",
		`perf_sample: pid=1 tid=1 source=raw_perfdata_fallback sample_kind=off_cpu symbolization_status=unsymbolized cpu_known=false clock_confidence=assumed`,
	}, "\n")
	body := BuildBody(Args{
		Request: "why jank?",
		Answer:  "answer",
		RuntimeArtifacts: append(
			RuntimeArtifactsFromAttachment("log", "# codrax-source: crash.log\npanic"),
			RuntimeArtifactsFromAttachment("trace", trace)...,
		),
	})
	for _, want := range []string{
		"## Runtime Artifacts",
		"| log | crash.log |",
		"| trace | frame.systrace |",
		"| perftrace | frame.perftrace |",
		"source=raw_perfdata_fallback",
		"sample_kind=off_cpu",
		"symbolization_status=unsymbolized",
		"cpu_known=false",
		"clock_confidence=assumed",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("runtime artifact table missing %q:\n%s", want, body)
		}
	}
}

func TestRuntimeArtifactsFromTraceBundleMetadata(t *testing.T) {
	bundle := strings.Join([]string{
		`{`,
		`  "version": "hitraceconv-v1",`,
		`  "systrace": "capture.systrace",`,
		`  "artifacts": [`,
		`    {"type":"systrace","path":"capture.systrace","bytes":12,"converter":"hitraceconv-v1"},`,
		`    {"type":"perftrace","path":"capture.perftrace","bytes":34,"converter":"hiperf_proto","caveats":["cpu id unavailable"]},`,
		`    {"type":"perf_data","path":"capture.perf.data","bytes":56,"plugin_name":"hiperf-plugin"}`,
		`  ],`,
		`  "caveats": ["bundle caveat"]`,
		`}`,
	}, "\n")
	artifacts := RuntimeArtifactsFromAttachment("trace", "# codrax-source: capture.tracebundle.json\n"+bundle)
	body := BuildBody(Args{
		Request:          "why jank?",
		Answer:           "answer",
		RuntimeArtifacts: artifacts,
	})
	for _, want := range []string{
		"| tracebundle | capture.tracebundle.json |",
		"version=hitraceconv-v1",
		"caveats=bundle caveat",
		"| perftrace | capture.perftrace |",
		"converter=hiperf_proto",
		"cpu id unavailable",
		"| perf_data | capture.perf.data |",
		"plugin=hiperf-plugin",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("trace bundle artifact table missing %q:\n%s", want, body)
		}
	}
}

func TestRuntimeArtifactsFromAttachmentKeepsInlinePreamble(t *testing.T) {
	artifacts := RuntimeArtifactsFromAttachment("log", "first inline log\n# codrax-source: second.log\nsecond")
	body := BuildBody(Args{
		Request:          "why",
		Answer:           "answer",
		RuntimeArtifacts: artifacts,
	})
	for _, want := range []string{"| log | (inline) |", "| log | second.log |"} {
		if !strings.Contains(body, want) {
			t.Fatalf("inline and sourced artifacts should both render %q:\n%s", want, body)
		}
	}
}

func TestRuntimeArtifactsFromRequestReportsExplicitPaths(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "capture")
	if err := os.WriteFile(tracePath, []byte("sched_switch: prev_comm=app prev_pid=1 ==> next_comm=worker next_pid=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	perfPath := filepath.Join(dir, "sample.perftrace")
	if err := os.WriteFile(perfPath, []byte("perf_sample: pid=1 tid=2 symbol=foo dso=app source=raw_perfdata_fallback sample_kind=off_cpu symbolization_status=unsymbolized cpu_known=false clock_confidence=assumed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	artifacts := RuntimeArtifactsFromRequest("请分析 " + tracePath + " 和 " + perfPath)
	body := BuildBody(Args{
		Request:          "why",
		Answer:           "answer",
		RuntimeArtifacts: artifacts,
	})
	for _, want := range []string{
		"| trace | " + tracePath + " |",
		"referenced in request",
		"| perftrace | " + perfPath + " |",
		"source=raw_perfdata_fallback",
		"sample_kind=off_cpu",
		"symbolization_status=unsymbolized",
		"cpu_known=false",
		"clock_confidence=assumed",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("request runtime artifact table missing %q:\n%s", want, body)
		}
	}
}

func TestRuntimeArtifactsFromRequestExpandsTraceBundlePath(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "capture.tracebundle.json")
	bundle := strings.Join([]string{
		`{`,
		`  "version": "hitraceconv-v1",`,
		`  "systrace": "capture.systrace",`,
		`  "artifacts": [`,
		`    {"type":"perftrace","path":"capture.perftrace","bytes":34,"converter":"hiperf_proto","caveats":["cpu id unavailable"]}`,
		`  ]`,
		`}`,
	}, "\n")
	if err := os.WriteFile(bundlePath, []byte(bundle), 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts := RuntimeArtifactsFromRequest("use " + bundlePath)
	body := BuildBody(Args{Request: "why", Answer: "answer", RuntimeArtifacts: artifacts})
	for _, want := range []string{
		"| tracebundle | " + bundlePath + " |",
		"version=hitraceconv-v1",
		"referenced in request",
		"| systrace | capture.systrace |",
		"| perftrace | capture.perftrace |",
		"converter=hiperf_proto",
		"cpu id unavailable",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("request tracebundle artifact table missing %q:\n%s", want, body)
		}
	}
}

func TestWriteResultWritesStandaloneHTMLSibling(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 26, 9, 30, 0, 0, time.UTC)
	result := WriteResult(Args{
		Dir:     dir,
		Max:     10,
		Request: "draw it",
		Answer: strings.Join([]string{
			"```mermaid",
			"flowchart LR",
			"  A --> B",
			"```",
		}, "\n"),
		Now: now,
		PID: 1001,
	})
	wantMarkdown := filepath.Join(dir, "20260526-093000.000-1001.md")
	wantHTML := filepath.Join(dir, "20260526-093000.000-1001.html")
	if result.MarkdownPath != wantMarkdown || result.HTMLPath != wantHTML {
		t.Fatalf("result = %#v, want markdown=%q html=%q", result, wantMarkdown, wantHTML)
	}
	htmlBytes, err := os.ReadFile(wantHTML)
	if err != nil {
		t.Fatalf("read html sibling: %v", err)
	}
	html := string(htmlBytes)
	if !strings.Contains(html, "<!doctype html>") ||
		!strings.Contains(html, `<div class="mermaid">`) ||
		!strings.Contains(html, "flowchart LR") ||
		!strings.Contains(html, "window.mermaid.render") {
		t.Fatalf("standalone html missing rendered markdown/mermaid support:\n%s", html[:min(len(html), 2000)])
	}
	if strings.Contains(html, `src="/assets/mermaid.min.js`) ||
		strings.Contains(html, "Raw Markdown</a>") {
		t.Fatalf("standalone html should not depend on preview server routes:\n%s", html[:min(len(html), 2000)])
	}
}

func TestWriteResultPrunesHTMLWithMarkdownRetention(t *testing.T) {
	dir := t.TempDir()
	old := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)
	mid := old.Add(time.Minute)
	for _, file := range []struct {
		name string
		mod  time.Time
	}{
		{"20260526-080000.000-1.md", old},
		{"20260526-080000.000-1.html", old},
		{"20260526-080100.000-2.md", mid},
		{"20260526-080100.000-2.html", mid},
		{"20260526-080200.000-3.html", mid.Add(time.Minute)},
	} {
		path := filepath.Join(dir, file.name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", file.name, err)
		}
		if err := os.Chtimes(path, file.mod, file.mod); err != nil {
			t.Fatalf("chtimes %s: %v", file.name, err)
		}
	}
	result := WriteResult(Args{
		Dir:     dir,
		Max:     2,
		Request: "q",
		Answer:  "a",
		Now:     old.Add(2 * time.Minute),
		PID:     4,
	})
	if result.MarkdownPath == "" || result.HTMLPath == "" {
		t.Fatalf("expected both artifacts, got %#v", result)
	}
	for _, gone := range []string{
		"20260526-080000.000-1.md",
		"20260526-080000.000-1.html",
		"20260526-080200.000-3.html",
	} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s should have been pruned, stat err=%v", gone, err)
		}
	}
	for _, kept := range []string{
		"20260526-080100.000-2.md",
		"20260526-080100.000-2.html",
		"20260526-080200.000-4.md",
		"20260526-080200.000-4.html",
	} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Fatalf("%s should be kept: %v", kept, err)
		}
	}
}
