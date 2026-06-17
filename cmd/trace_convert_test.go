package cmd

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func TestTraceConvertResultLinesFollowLanguage(t *testing.T) {
	result := hitraceconv.Result{
		InputPath:          "in.htrace",
		OutputPath:         "in.htrace.systrace",
		EventsWritten:      12,
		MissingFormatCount: 2,
		UnknownEventCount:  3,
	}

	zh := strings.Join(traceConvertResultLines("zh", result), "\n")
	if !strings.Contains(zh, "已转换二进制 hitrace") || !strings.Contains(zh, "跳过缺失格式：2") {
		t.Fatalf("zh result lines not localized enough:\n%s", zh)
	}
	if !strings.Contains(zh, "仅行头事件：3") {
		t.Fatalf("zh result lines should report header-only rows:\n%s", zh)
	}
	if strings.Contains(zh, "converted binary hitrace") {
		t.Fatalf("zh result leaked English:\n%s", zh)
	}

	en := strings.Join(traceConvertResultLines("en", result), "\n")
	if !strings.Contains(en, "converted binary hitrace") || !strings.Contains(en, "header_only_events: 3") {
		t.Fatalf("en result lines malformed:\n%s", en)
	}
	if strings.Contains(en, "已转换") {
		t.Fatalf("en result leaked Chinese:\n%s", en)
	}
}

func TestTraceConvertNextLineFollowsLanguage(t *testing.T) {
	result := hitraceconv.Result{OutputPath: "out.systrace"}
	if got := traceConvertNextLine("zh", result); !strings.Contains(got, "下一步") || !strings.Contains(got, "<问题>") {
		t.Fatalf("zh next line malformed: %q", got)
	}
	if got := traceConvertNextLine("en", result); !strings.Contains(got, "next:") || !strings.Contains(got, "<question>") {
		t.Fatalf("en next line malformed: %q", got)
	}
}

func TestTraceConvertPerfToolStatusLines(t *testing.T) {
	status := hitraceconv.PerfToolStatus{
		ParserMode:               "auto",
		SelectedParser:           "auto",
		SymbolizationExpectation: "official first, raw fallback",
		Hiperf: hitraceconv.PerfToolProviderStatus{
			Name:      "openharmony_hiperf",
			Kind:      "official_harmony",
			Available: true,
			Path:      "/tmp/hiperf_host",
			Source:    "configured hiperf tool",
		},
		Simpleperf: hitraceconv.PerfToolProviderStatus{
			Name:        "android_simpleperf_report_sample",
			Kind:        "official_android",
			Available:   false,
			InstallHint: "install simpleperf",
		},
		RawFallback: hitraceconv.PerfToolProviderStatus{
			Name:      "codrax_raw_perfdata",
			Kind:      "raw_fallback",
			Available: true,
			Source:    "built-in",
		},
	}
	en := strings.Join(traceConvertPerfToolStatusLines("en", status), "\n")
	for _, want := range []string{"perf_parser: auto", "official_harmony", "/tmp/hiperf_host", "official_android", "hint=install simpleperf", "raw_fallback", "built-in"} {
		if !strings.Contains(en, want) {
			t.Fatalf("status lines missing %q:\n%s", want, en)
		}
	}
	zh := strings.Join(traceConvertPerfToolStatusLines("zh", status), "\n")
	if !strings.Contains(zh, "perf 解析模式：auto") || !strings.Contains(zh, "符号化预期") {
		t.Fatalf("zh status lines malformed:\n%s", zh)
	}
}
