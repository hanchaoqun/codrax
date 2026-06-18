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

func TestTraceConvertNextLinePrefersTraceBundle(t *testing.T) {
	result := hitraceconv.Result{
		OutputPath: "out.systrace",
		BundlePath: "out.tracebundle.json",
		Artifacts: []hitraceconv.Artifact{{
			Type:      hitraceconv.ArtifactPerfTrace,
			Path:      "out.perftrace",
			Converter: "hitraceconv-v1+raw-perfdata",
		}},
	}
	if got := traceConvertNextLine("en", result); !strings.Contains(got, "out.tracebundle.json") || strings.Contains(got, "out.systrace") {
		t.Fatalf("next line should prefer tracebundle when perf artifacts are present: %q", got)
	}
	if got := traceConvertNextLine("zh", result); !strings.Contains(got, "tracebundle") || !strings.Contains(got, "perftrace") {
		t.Fatalf("zh next line should mention bundled trace/perf handoff: %q", got)
	}
}

func TestTraceConvertArtifactLinesIncludeProvenance(t *testing.T) {
	lines := strings.Join(traceConvertArtifactLines("en", []hitraceconv.Artifact{{
		Type:          hitraceconv.ArtifactPerfTrace,
		Path:          "out.perftrace",
		Bytes:         123,
		DataType:      1,
		PluginName:    "hiperf-plugin",
		PluginVersion: "1",
		SourceOffset:  1024,
		SourceBytes:   99,
		Converter:     "hitraceconv-v1+raw-perfdata",
		Perf: &hitraceconv.PerfArtifactCapability{
			ProviderKind:    "raw_fallback",
			ProviderName:    "codrax_raw_perfdata",
			InputFormat:     "linux_perf_data",
			Symbolization:   "unsymbolized_ip",
			CPUIdentity:     "sample_cpu_when_recorded",
			Callchain:       "ip_only_when_recorded",
			TimeAlignment:   "assumed",
			TraceQueryReady: true,
			Degraded:        true,
		},
		Caveats: []string{"symbolization_status=unsymbolized"},
	}}), "\n")
	for _, want := range []string{"format=text_perftrace", "bytes=123", "data_type=1", "plugin=hiperf-plugin", "plugin_version=1", "source_offset=1024", "source_bytes=99", "converter=hitraceconv-v1+raw-perfdata", "perf_provider=codrax_raw_perfdata", "perf_input=linux_perf_data", "perf_symbolization=unsymbolized_ip", "trace_query_ready=true", "perf_degraded=true", "symbolization_status=unsymbolized"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("artifact detail missing %q:\n%s", want, lines)
		}
	}
	zh := strings.Join(traceConvertArtifactLines("zh", []hitraceconv.Artifact{{
		Type:      hitraceconv.ArtifactPerfData,
		Path:      "out.perf.data",
		Bytes:     456,
		Converter: "hitraceconv-v1",
		Caveats: []string{
			"raw perf.data sidecar preserved; normalized .perftrace was generated for trace_query CPU-sample aggregation",
		},
	}}), "\n")
	for _, want := range []string{"格式=二进制 perf.data sidecar", "字节=456", "转换器=hitraceconv-v1", "提示=raw perf.data sidecar 已保留"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh artifact detail missing %q:\n%s", want, zh)
		}
	}
	for _, leak := range []string{"bytes=", "converter=", "caveats=", "raw perf.data sidecar preserved"} {
		if strings.Contains(zh, leak) {
			t.Fatalf("zh artifact detail leaked English %q:\n%s", leak, zh)
		}
	}
}

func TestTraceConvertResultLinesIncludeProviderDecisions(t *testing.T) {
	result := hitraceconv.Result{
		InputPath: "capture.bin",
		ProviderDecisions: []hitraceconv.PerfProviderDecision{{
			Stage:           "direct_input",
			ProviderKind:    "raw_fallback",
			ProviderName:    "codrax_raw_perfdata",
			InputFormat:     "linux_perf_data",
			OutputPath:      "capture.perftrace",
			ParserMode:      "raw",
			Selected:        true,
			Attempted:       true,
			Succeeded:       true,
			Fallback:        false,
			TraceQueryReady: true,
			ArtifactPath:    "capture.perftrace",
		}},
	}
	en := strings.Join(traceConvertResultLines("en", result), "\n")
	for _, want := range []string{"provider_decision[raw_fallback/codrax_raw_perfdata]", "selected=true", "attempted=true", "succeeded=true", "trace_query_ready=true", "stage=direct_input", "parser=raw", "input=linux_perf_data"} {
		if !strings.Contains(en, want) {
			t.Fatalf("provider decision output missing %q:\n%s", want, en)
		}
	}
	zh := strings.Join(traceConvertResultLines("zh", result), "\n")
	if !strings.Contains(zh, "provider_decision[raw_fallback/codrax_raw_perfdata]：") || !strings.Contains(zh, "已成功=是") || !strings.Contains(zh, "回退路径=否") {
		t.Fatalf("zh provider decision output malformed:\n%s", zh)
	}
}

func TestTraceConvertPerfToolStatusLines(t *testing.T) {
	status := hitraceconv.PerfToolStatus{
		ParserMode:               "auto",
		SelectedParser:           "auto",
		SymbolizationExpectation: "official first, raw fallback",
		Hiperf: hitraceconv.PerfToolProviderStatus{
			Name:            "openharmony_hiperf",
			Kind:            "official_harmony",
			Available:       true,
			Path:            "/tmp/hiperf_host",
			Source:          "configured hiperf tool",
			CheckCommand:    "hiperf_host --help",
			AuxiliaryChecks: []string{"symbol_root=/symbols check=test -d /symbols"},
			InstallCommand:  "git clone https://gitee.com/openharmony/developtools_hiperf",
			DocsURL:         "https://gitee.com/openharmony/developtools_hiperf",
		},
		Simpleperf: hitraceconv.PerfToolProviderStatus{
			Name:            "android_simpleperf_report_sample",
			Kind:            "official_android",
			Available:       false,
			CheckCommand:    "python3 report_sample.py --help",
			AuxiliaryChecks: []string{"symfs=/symfs check=test -d /symfs", "kallsyms=/proc/kallsyms check=test -r /proc/kallsyms"},
			InstallCommand:  "fetch simpleperf",
			DocsURL:         "https://android.googlesource.com/platform/system/extras/+/refs/heads/main/simpleperf/",
			InstallHint:     "Use Android simpleperf scripts/report_sample.py, then pass --simpleperf-report-sample or set CODRAX_SIMPLEPERF_REPORT_SAMPLE; add --simpleperf-python, --simpleperf-symfs, and --simpleperf-kallsyms as needed.",
		},
		RawFallback: hitraceconv.PerfToolProviderStatus{
			Name:           "codrax_raw_perfdata",
			Kind:           "raw_fallback",
			Available:      true,
			Source:         "built-in",
			CheckCommand:   "codrax trace convert --perf-tools-status --perf-parser=raw",
			InstallCommand: "built-in",
		},
	}
	en := strings.Join(traceConvertPerfToolStatusLines("en", status), "\n")
	for _, want := range []string{"perf_parser: auto", "official_harmony", "/tmp/hiperf_host", "check=hiperf_host --help", "aux_check=symbol_root=/symbols", "docs=https://gitee.com/openharmony/developtools_hiperf", "official_android", "check=python3 report_sample.py --help", "symfs=/symfs", "kallsyms=/proc/kallsyms", "install=fetch simpleperf", "hint=Use Android simpleperf", "raw_fallback", "built-in", "--perf-parser=raw"} {
		if !strings.Contains(en, want) {
			t.Fatalf("status lines missing %q:\n%s", want, en)
		}
	}
	zh := strings.Join(traceConvertPerfToolStatusLines("zh", status), "\n")
	for _, want := range []string{"perf 解析模式：auto", "符号化预期：auto 优先使用官方", "状态=可用", "状态=缺失", "辅助检查=符号目录=/symbols", "提示=使用 Android simpleperf", "安装=内置，无需安装"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh status lines missing %q:\n%s", want, zh)
		}
	}
	for _, leak := range []string{"auto prefers official", "Install or build OpenHarmony", "Use Android simpleperf", "state=missing", "hint=install simpleperf"} {
		if strings.Contains(zh, leak) {
			t.Fatalf("zh status lines leaked English detail %q:\n%s", leak, zh)
		}
	}
	if !strings.Contains(zh, "符号化预期") {
		t.Fatalf("zh status lines malformed:\n%s", zh)
	}
}
