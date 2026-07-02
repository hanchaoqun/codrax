package types

import "testing"

func TestRuntimeArtifactPathTokensInText(t *testing.T) {
	got := RuntimeArtifactPathTokensInText("只分析 ../customlogs/xxx_all.systrace 这个 trace 文件和 /tmp/perf.data")
	want := []string{"../customlogs/xxx_all.systrace", "/tmp/perf.data"}
	if len(got) != len(want) {
		t.Fatalf("tokens=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokens=%v, want %v", got, want)
		}
	}
}

func TestRuntimeArtifactPathTokensInText_CJKGlued(t *testing.T) {
	// Regression: a path written by name in a Chinese question is commonly
	// flush against the surrounding prose with no whitespace separator. The
	// embedded artifact path must still be extracted.
	cases := map[string][]string{
		"分析/tmp/frame.systrace的卡顿原因":     {"/tmp/frame.systrace"},
		"为什么./logs/crash.log里有报错":        {"./logs/crash.log"},
		"看一下/data/local/tmp/perf.data看看": {"/data/local/tmp/perf.data"},
		// A CJK directory component in the middle of the path must be kept,
		// not split.
		"抓取/tmp/中文目录/x.systrace的数据": {"/tmp/中文目录/x.systrace"},
		// Pure prose with no artifact path yields nothing.
		"分析一下这个卡顿问题": nil,
	}
	for in, want := range cases {
		got := RuntimeArtifactPathTokensInText(in)
		if len(got) != len(want) {
			t.Fatalf("RuntimeArtifactPathTokensInText(%q) = %v, want %v", in, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("RuntimeArtifactPathTokensInText(%q) = %v, want %v", in, got, want)
			}
		}
	}
}

func TestRuntimeArtifactPathTokensInText_CustomerCompositeMixedTail(t *testing.T) {
	// Regression pin for the EXACT customer request shape (trace_repl.log,
	// 2026-07-02): a label prefix ("Trace:"), a composite ".sys.ftrace"
	// suffix, and a glued tail that mixes CJK prose with an ASCII term —
	// "…sys.ftrace里面这一帧Choreographer#doFrame" has NO whitespace before
	// the ASCII run. Extraction failing here left the runtime-artifact
	// preflight blind: trace_query never surfaced and completion citation
	// floors dragged a runtime-only answer back into source grep loops.
	request := "你是Android手机性能问题分析专家, 完成以下丢帧原因分析, 分析东湖Trace:record_trace_20260605224432@3279-299954687.sys.ftrace里面这一帧Choreographer#doFrame 94410(帧号：Choreographer#doFrame 94410, UI线程id:19592, 渲染线程id:19829, 时间范围3322.500833s至3322.784155s)因应用UI线程被阻塞导致丢帧的深层原因，综合各类因素分析，明确回答核心原因, 注意是Activity Resume后的首帧, 不分析代码"
	want := "record_trace_20260605224432@3279-299954687.sys.ftrace"
	got := RuntimeArtifactPathTokensInText(request)
	if len(got) != 1 || got[0] != want {
		t.Fatalf("RuntimeArtifactPathTokensInText(customer request) = %v, want [%q]", got, want)
	}
	if kind := RuntimeArtifactPathKindInText(request); kind != "trace" {
		t.Fatalf("RuntimeArtifactPathKindInText(customer request) = %q, want \"trace\"", kind)
	}
}

func TestRuntimeArtifactPathInToken(t *testing.T) {
	cases := map[string]string{
		// Trailing CJK prose glued after the extension.
		"分析/tmp/frame.systrace的卡顿": "/tmp/frame.systrace",
		// Leading CJK prose glued before the path root.
		"抓取/tmp/app.log": "/tmp/app.log",
		// Both sides glued.
		"看/var/log/x.ftrace再看": "/var/log/x.ftrace",
		// Composite trace suffixes must still be recognized when prose is glued.
		"分析record_trace_20260605224432@3279-299954687.sys.ftrace里面": "record_trace_20260605224432@3279-299954687.sys.ftrace",
		// Labels before a path are presentation, not part of the artifact path.
		"Trace:record_trace_20260605224432@3279-299954687.sys.ftrace里面": "record_trace_20260605224432@3279-299954687.sys.ftrace",
		"东湖Trace：/tmp/frame.systrace里面":                                 "/tmp/frame.systrace",
		// Mixed glued tail: CJK prose immediately after the extension, then an
		// ASCII term with no separator (real customer shape, trace_repl.log
		// 2026-07-02). The pure-CJK trailing walk stops at the ASCII run and
		// never reaches the extension; the carve cuts at the extension→CJK
		// boundary instead.
		"分析东湖Trace:record_trace_20260605224432@3279-299954687.sys.ftrace里面这一帧Choreographer#doFrame": "record_trace_20260605224432@3279-299954687.sys.ftrace",
		"分析record_trace_20260605224432@3279-299954687.sys.ftrace里面这一帧Choreographer#doFrame":         "record_trace_20260605224432@3279-299954687.sys.ftrace",
		"看/tmp/frame.systrace的卡顿traceview":                                                          "/tmp/frame.systrace",
		"抓/var/log/app.log里的NullPointerException":                                                   "/var/log/app.log",
		// CJK path components between two artifact suffixes stay intact: the
		// rightmost extension→CJK boundary wins.
		"x.log里y.log看": "x.log里y.log",
		// Mixed tail without a recognized artifact suffix still carves to nothing.
		"分析main.go里面这一帧Choreographer#doFrame": "",
		// Artifact-suffixed DIRECTORY/infix components of longer non-artifact
		// paths must NOT mint phantom artifacts (adversarial review
		// 2026-07-02): the remainder after the boundary still reads as a
		// path (separator or trailing dot-extension).
		"/tmp/a.log子dir/notes.txt": "",
		"handler.trace包/foo.go":    "",
		"崩溃日志.log分析报告.md":          "",
		// Non-ASCII case folding must not skew carve offsets (U+0130 İ grows
		// when lowered; U+212A KELVIN K shrinks) — these contain a valid
		// extension→CJK boundary and must still carve.
		"İnfo.log里面Choreographer": "İnfo.log",
		"a.logKb.log里x":           "a.logKb.log",
		// The remainder after the "a.log" boundary contains a path
		// separator, so ".log" sat on an infix component of a longer
		// path-shaped token: carving "a.log" out of it would be exactly the
		// phantom class above. Nothing verifies → no carve.
		"a.log里/.log里x": "",
		// Basename special cases carve too.
		"抓到perf.data里面这一帧Frame": "perf.data",
		// Windows drive prefixes are path syntax, not labels.
		`D:\temp\frame.systrace`: `D:\temp\frame.systrace`,
		// Already-clean paths pass through unchanged.
		"/tmp/frame.systrace": "/tmp/frame.systrace",
		"capture.atrace":      "capture.atrace",
		// CJK directory component in the middle is preserved.
		"/tmp/中文目录/x.systrace": "/tmp/中文目录/x.systrace",
		// A fully-CJK artifact filename is preserved (cannot be split from
		// glued prose without a separator, so we keep the whole token).
		"报告.systrace": "报告.systrace",
		// Non-artifact tokens carve to nothing.
		"main.go": "",
		"分析这个问题":  "",
		"":        "",
	}
	for in, want := range cases {
		if got := RuntimeArtifactPathInToken(in); got != want {
			t.Errorf("RuntimeArtifactPathInToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsCodeOrConfigPathExtension(t *testing.T) {
	for _, ext := range []string{
		".go", ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx",
		".cj", ".cjo", ".ets", ".ts", ".tsx", ".js", ".jsx", ".mjs",
		".java", ".kt", ".kts", ".py", ".pyi", ".rs", ".rb", ".php", ".swift",
		".lua", ".proto", ".m", ".mm", ".cu", ".cuh",
		".yaml", ".yml", ".json", ".toml", ".ini", ".xml", ".md",
	} {
		if !IsCodeOrConfigPathExtension(ext) {
			t.Errorf("IsCodeOrConfigPathExtension(%q) = false; want true", ext)
		}
	}
	for _, ext := range []string{
		"", ".unknown", ".txt", ".log", "go", "no-dot",
	} {
		if IsCodeOrConfigPathExtension(ext) {
			t.Errorf("IsCodeOrConfigPathExtension(%q) = true; want false", ext)
		}
	}
	if !IsCodeOrConfigPathExtension(".GO") || !IsCodeOrConfigPathExtension(".YAML") {
		t.Error("IsCodeOrConfigPathExtension must be case-insensitive")
	}
}

func TestHasCodeOrConfigPathSuffix(t *testing.T) {
	hits := map[string]bool{
		"foo.go":             true,
		"a/b/foo.go":         true,
		"bar.YAML":           true,
		"weird_FILE.PYI":     true,
		"some-name.proto":    true,
		"package_clause.cjo": true,
	}
	for s, want := range hits {
		if got := HasCodeOrConfigPathSuffix(s); got != want {
			t.Errorf("HasCodeOrConfigPathSuffix(%q) = %v; want %v", s, got, want)
		}
	}
	misses := []string{
		"", "bare", "foo.unknown", "foo.txt", "no_extension_dot_here",
	}
	for _, s := range misses {
		if HasCodeOrConfigPathSuffix(s) {
			t.Errorf("HasCodeOrConfigPathSuffix(%q) = true; want false", s)
		}
	}
}

func TestLooksLikeRuntimeArtifactPath(t *testing.T) {
	for _, s := range []string{
		"record_trace.systrace",
		"/tmp/app.log",
		"trace.HTRACE",
		"capture.atrace",
		"capture.ftrace",
		"record_trace_20260605224432@3279-299954687.sys.ftrace",
		"sample.perfetto",
		"sample.perftrace",
		"sample.tracebundle.json",
		"perf.data",
		"sample.perf.data",
		"attached_trace.txt",
		"/tmp/.codrax/blob/session/attached_trace.txt",
		"/tmp/.codrax/blob/session/attached_log.txt",
	} {
		if !LooksLikeRuntimeArtifactPath(s) {
			t.Errorf("LooksLikeRuntimeArtifactPath(%q) = false; want true", s)
		}
	}
	for _, s := range []string{"", ".log", ".trace", ".systrace", "main.go", "config.yaml", "README.md", "trace.txt"} {
		if LooksLikeRuntimeArtifactPath(s) {
			t.Errorf("LooksLikeRuntimeArtifactPath(%q) = true; want false", s)
		}
	}
}
