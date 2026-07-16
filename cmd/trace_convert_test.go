package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func traceConvertTestSystraceArtifact(path string, ready bool) hitraceconv.Artifact {
	return hitraceconv.Artifact{
		Type: hitraceconv.ArtifactSystrace,
		Path: path,
		Trace: &hitraceconv.TraceArtifactCapability{
			ProviderKind:       "builtin_modern",
			ProviderName:       "codrax_builtin_modern_profiler",
			OutputFormat:       hitraceconv.ArtifactSystrace,
			ValidationProfile:  "builtin_systrace_v1",
			Rows:               1,
			Known:              1,
			AuthoritativeKnown: 1,
			TraceQueryReady:    ready,
		},
	}
}

func TestTraceConvertExecutionDoesNotPreemptImmutableInputRoute(t *testing.T) {
	source, err := os.ReadFile("trace_convert.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "hitraceconv.ValidateOptions(opts)") {
		t.Fatal("trace convert CLI regained conservative static validation before immutable input routing")
	}
	if !strings.Contains(text, "hitraceconv.ConvertFile(cmd.Context(), opts)") {
		t.Fatal("trace convert CLI lost the conversion input authority entry point")
	}
}

func TestTraceConvertResultLinesFollowLanguage(t *testing.T) {
	result := hitraceconv.Result{
		InputPath:          "in.htrace",
		OutputPath:         "in.htrace.systrace",
		EventsWritten:      12,
		MissingFormatCount: 2,
		UnknownEventCount:  3,
		TraceCoverage: []hitraceconv.TraceDBCoverage{{
			Family:      "builtin_modern_ftrace:sched",
			Table:       "sched_switch",
			RowsRead:    1,
			RowsEmitted: 1,
			ElapsedUS:   42,
		}},
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
	if !strings.Contains(zh, "trace_coverage：1 项，输出=1，跳过=0") ||
		!strings.Contains(zh, "族=builtin_modern_ftrace:sched") ||
		!strings.Contains(zh, "耗时us=42") ||
		strings.Contains(zh, "rows_read=") ||
		strings.Contains(zh, "elapsed_us=") ||
		strings.Contains(zh, "emitted=") ||
		strings.Contains(zh, "skipped=") {
		t.Fatalf("zh result should include localized trace coverage:\n%s", zh)
	}

	en := strings.Join(traceConvertResultLines("en", result), "\n")
	if !strings.Contains(en, "converted binary hitrace") || !strings.Contains(en, "header_only_events: 3") {
		t.Fatalf("en result lines malformed:\n%s", en)
	}
	if !strings.Contains(en, "trace_coverage: 1 item") || !strings.Contains(en, "family=builtin_modern_ftrace:sched") || !strings.Contains(en, "rows_read=1") || !strings.Contains(en, "elapsed_us=42") {
		t.Fatalf("en result should include trace coverage:\n%s", en)
	}
	if strings.Contains(en, "已转换") {
		t.Fatalf("en result leaked Chinese:\n%s", en)
	}
}

func TestTraceConvertProgressLineFollowsLanguage(t *testing.T) {
	event := hitraceconv.ProgressEvent{
		Stage:      "trace_db_normalize",
		Status:     hitraceconv.ProgressStatusProgress,
		Message:    "normalizing trace_streamer SQLite DB to systrace",
		Path:       "in.trace.db",
		OutputPath: "out.systrace",
		BytesDone:  10,
		BytesTotal: 20,
		Records:    3,
		Elapsed:    1200 * time.Millisecond,
	}
	zh := traceConvertProgressLine("zh", event)
	if !strings.Contains(zh, "进度：") ||
		!strings.Contains(zh, "阶段=SQLite DB转systrace") ||
		!strings.Contains(zh, "状态=进行中") ||
		!strings.Contains(zh, "字节=10/20") ||
		!strings.Contains(zh, "耗时=1.2s") {
		t.Fatalf("zh progress line not localized enough:\n%s", zh)
	}
	en := traceConvertProgressLine("en", event)
	if !strings.Contains(en, "progress:") ||
		!strings.Contains(en, "stage=trace_db_normalize") ||
		!strings.Contains(en, "status=progress") ||
		!strings.Contains(en, "bytes=10/20") ||
		strings.Contains(en, "阶段=") {
		t.Fatalf("en progress line mismatch:\n%s", en)
	}
}

func TestTraceConvertProgressLineTerminalMessage(t *testing.T) {
	event := hitraceconv.ProgressEvent{
		Stage:   "trace_streamer_export",
		Status:  hitraceconv.ProgressStatusComplete,
		Message: "completed trace_streamer SQLite DB export",
		Elapsed: 1200 * time.Millisecond,
	}
	zh := traceConvertProgressLine("zh", event)
	if !strings.Contains(zh, "状态=完成") ||
		!strings.Contains(zh, "说明=已完成 trace_streamer 导出 SQLite DB") ||
		strings.Contains(zh, "正在运行") {
		t.Fatalf("zh terminal progress line mismatch:\n%s", zh)
	}
	en := traceConvertProgressLine("en", event)
	if !strings.Contains(en, "status=complete") ||
		!strings.Contains(en, "message=completed trace_streamer SQLite DB export") {
		t.Fatalf("en terminal progress line mismatch:\n%s", en)
	}
}

func TestTraceConvertSnapshotProgressLineFollowsLanguage(t *testing.T) {
	event := hitraceconv.ProgressEvent{
		Stage:      "trace_streamer_input_snapshot",
		Status:     hitraceconv.ProgressStatusProgress,
		Message:    "copying immutable trace_streamer input",
		Path:       "capture.htrace",
		OutputPath: "capture.systrace",
		BytesDone:  64,
		BytesTotal: 128,
	}
	zh := traceConvertProgressLine("zh", event)
	for _, want := range []string{
		"阶段=准备trace_streamer输入快照",
		"状态=进行中",
		"说明=正在复制不可变的 trace_streamer 输入快照",
		"路径=capture.htrace",
		"字节=64/128",
	} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh snapshot progress missing %q:\n%s", want, zh)
		}
	}
	en := traceConvertProgressLine("en", event)
	if !strings.Contains(en, "stage=trace_streamer_input_snapshot") ||
		!strings.Contains(en, "message=copying immutable trace_streamer input") ||
		!strings.Contains(en, "bytes=64/128") {
		t.Fatalf("en snapshot progress mismatch:\n%s", en)
	}
}

func TestTraceConvertPerfSnapshotProgressLineFollowsLanguage(t *testing.T) {
	for _, test := range []struct {
		provider string
		stage    string
		zhStage  string
		zhMsg    string
	}{
		{provider: "simpleperf", stage: "simpleperf_input_snapshot", zhStage: "准备simpleperf输入快照", zhMsg: "正在复制不可变的 simpleperf 输入快照"},
		{provider: "hiperf", stage: "hiperf_input_snapshot", zhStage: "准备hiperf输入快照", zhMsg: "正在复制不可变的 hiperf 输入快照"},
	} {
		t.Run(test.provider, func(t *testing.T) {
			event := hitraceconv.ProgressEvent{
				Stage: test.stage, Status: hitraceconv.ProgressStatusProgress,
				Message: "copying immutable " + test.provider + " input",
				Path:    "capture.perf.data", OutputPath: "capture.perftrace", BytesDone: 64, BytesTotal: 128,
			}
			zh := traceConvertProgressLine("zh", event)
			if !strings.Contains(zh, "阶段="+test.zhStage) || !strings.Contains(zh, "说明="+test.zhMsg) || !strings.Contains(zh, "字节=64/128") {
				t.Fatalf("zh perf snapshot progress mismatch:\n%s", zh)
			}
			en := traceConvertProgressLine("en", event)
			if !strings.Contains(en, "stage="+test.stage) || !strings.Contains(en, "message="+event.Message) || !strings.Contains(en, "bytes=64/128") {
				t.Fatalf("en perf snapshot progress mismatch:\n%s", en)
			}
		})
	}
	for message, want := range map[string]string{
		"completed official simpleperf adapter command": "已完成官方 simpleperf 适配器命令",
		"official simpleperf adapter command failed":    "官方 simpleperf 适配器命令失败",
		"simpleperf command boundary rejected":          "simpleperf 命令完成后的一致性校验失败",
		"completed official hiperf adapter command":     "已完成官方 hiperf 适配器命令",
		"official hiperf adapter command failed":        "官方 hiperf 适配器命令失败",
		"hiperf command boundary rejected":              "hiperf 命令完成后的一致性校验失败",
	} {
		if got := traceConvertProgressMessageZh(message); got != want {
			t.Fatalf("perf terminal message %q zh=%q want=%q", message, got, want)
		}
	}
}

func TestTraceConvertCoverageLinesExplainResolverRows(t *testing.T) {
	coverage := []hitraceconv.TraceDBCoverage{{
		Family:      "resolver",
		Table:       "thread",
		Role:        "resolver_index",
		RowsRead:    3,
		RowsEmitted: 0,
	}}
	zh := strings.Join(traceConvertCoverageLines("zh", "trace_db_coverage", coverage), "\n")
	if !strings.Contains(zh, "用途=解析辅助索引，不直接输出 systrace 行") ||
		!strings.Contains(zh, "输出行=0") ||
		strings.Contains(zh, "跳过原因") {
		t.Fatalf("zh resolver coverage should explain index-only rows without marking them skipped:\n%s", zh)
	}
	en := strings.Join(traceConvertCoverageLines("en", "trace_db_coverage", coverage), "\n")
	if !strings.Contains(en, "role=resolver_index") || !strings.Contains(en, "rows_emitted=0") {
		t.Fatalf("en resolver coverage should expose role:\n%s", en)
	}
}

func TestTraceConvertCoverageLinesExposeCaptureSelfAuditWithoutCountingSkip(t *testing.T) {
	coverage := []hitraceconv.TraceDBCoverage{{
		Family: "capture_completeness", Table: "stat", Role: "capture_completeness", Found: true, RowsRead: 5,
		CaptureCompleteness: &hitraceconv.TraceCaptureCompleteness{
			State: "parser_self_audit_degraded", RowsAccepted: 5, Received: 10, DataLost: 2, NotMatch: 1,
		},
	}}
	zh := strings.Join(traceConvertCoverageLines("zh", "trace_db_coverage", coverage), "\n")
	for _, want := range []string{
		"trace_db_coverage：1 项，输出=0，跳过=0",
		"用途=trace_streamer 解析自审，不直接输出 systrace 行",
		"解析自审状态=parser_self_audit_degraded",
		"解析器接收计数=10",
		"解析器丢失计数=2",
		"上下文不匹配计数=1",
	} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh capture self-audit missing %q:\n%s", want, zh)
		}
	}
	en := strings.Join(traceConvertCoverageLines("en", "trace_db_coverage", coverage), "\n")
	for _, want := range []string{
		"trace_db_coverage: 1 item(s), emitted=0, skipped=0",
		"role=capture_completeness",
		"capture_state=parser_self_audit_degraded",
		"capture_received=10",
		"capture_data_lost=2",
		"capture_not_match=1",
	} {
		if !strings.Contains(en, want) {
			t.Fatalf("en capture self-audit missing %q:\n%s", want, en)
		}
	}
}

func TestTraceConvertNextLineFollowsLanguage(t *testing.T) {
	result := hitraceconv.Result{
		OutputPath: "out.systrace",
		Artifacts:  []hitraceconv.Artifact{traceConvertTestSystraceArtifact("out.systrace", true)},
	}
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
			Perf:      &hitraceconv.PerfArtifactCapability{TraceQueryReady: true},
		}, traceConvertTestSystraceArtifact("out.systrace", true)},
	}
	if got := traceConvertNextLine("en", result); !strings.Contains(got, "out.tracebundle.json") ||
		!strings.Contains(got, "joint systrace event and validated CPU-sample queries") ||
		strings.Contains(got, "out.systrace") {
		t.Fatalf("next line should prefer the joint tracebundle only for receipt-validated perf artifacts: %q", got)
	}
	if got := traceConvertNextLine("zh", result); !strings.Contains(got, "tracebundle") ||
		!strings.Contains(got, "联合查询 systrace 核心事件与已验证 CPU sample") {
		t.Fatalf("zh next line should disclose joint receipt-validated tracebundle querying: %q", got)
	}
}

func TestTraceConvertNextLineDoesNotInferPerfReadinessFromArtifactType(t *testing.T) {
	tests := []struct {
		name      string
		artifacts []hitraceconv.Artifact
	}{
		{name: "nil artifacts"},
		{
			name: "type only with nil capability",
			artifacts: []hitraceconv.Artifact{{
				Type: hitraceconv.ArtifactPerfTrace,
				Path: "out.perftrace",
			}},
		},
		{
			name: "capability explicitly not ready",
			artifacts: []hitraceconv.Artifact{{
				Type: hitraceconv.ArtifactPerfTrace,
				Path: "out.perftrace",
				Perf: &hitraceconv.PerfArtifactCapability{TraceQueryReady: false},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifacts := append([]hitraceconv.Artifact{traceConvertTestSystraceArtifact("out.systrace", true)}, test.artifacts...)
			result := hitraceconv.Result{
				OutputPath: "out.systrace",
				BundlePath: "out.tracebundle.json",
				Artifacts:  artifacts,
			}
			en := traceConvertNextLine("en", result)
			if !strings.Contains(en, "core systrace event queries") ||
				!strings.Contains(en, "no query-ready perftrace CPU samples are available") ||
				strings.Contains(en, "joint systrace event and validated CPU-sample queries") {
				t.Fatalf("type-only perf artifact gained a CPU-query claim:\n%s", en)
			}
			zh := traceConvertNextLine("zh", result)
			if !strings.Contains(zh, "可查询 systrace 核心事件") ||
				!strings.Contains(zh, "当前没有可供 trace_query 消费的 perftrace CPU sample") ||
				strings.Contains(zh, "联合查询 systrace 核心事件与已验证 CPU sample") {
				t.Fatalf("zh type-only perf artifact gained a CPU-query claim:\n%s", zh)
			}
		})
	}
}

func TestTraceConvertNextLinePerfOnlyDisclosesMissingSystraceCorrelation(t *testing.T) {
	readyPerf := hitraceconv.Artifact{
		Type: hitraceconv.ArtifactPerfTrace,
		Path: "out.perftrace",
		Perf: &hitraceconv.PerfArtifactCapability{TraceQueryReady: true},
	}
	t.Run("bundle", func(t *testing.T) {
		result := hitraceconv.Result{
			BundlePath: "out.tracebundle.json",
			Artifacts:  []hitraceconv.Artifact{readyPerf},
		}
		en := traceConvertNextLine("en", result)
		for _, want := range []string{
			"out.tracebundle.json",
			"aggregate validated CPU samples",
			"no systrace trace body",
			"cannot correlate trace windows or scheduling causality",
		} {
			if !strings.Contains(en, want) {
				t.Fatalf("perf-only bundle omitted %q:\n%s", want, en)
			}
		}
		zh := traceConvertNextLine("zh", result)
		for _, want := range []string{
			"out.tracebundle.json",
			"可聚合已验证的 CPU sample",
			"没有 systrace trace body",
			"不能做 trace 时间窗或调度因果关联",
		} {
			if !strings.Contains(zh, want) {
				t.Fatalf("zh perf-only bundle omitted %q:\n%s", want, zh)
			}
		}
	})

	t.Run("direct perftrace without bundle", func(t *testing.T) {
		result := hitraceconv.Result{Artifacts: []hitraceconv.Artifact{readyPerf}}
		en := traceConvertNextLine("en", result)
		for _, want := range []string{
			`codrax --htrace "out.perftrace"`,
			"validated CPU samples can be aggregated",
			"no systrace trace body",
			"trace-window or scheduling-causality correlation",
		} {
			if !strings.Contains(en, want) {
				t.Fatalf("direct perf-only next line omitted %q:\n%s", want, en)
			}
		}
		zh := traceConvertNextLine("zh", result)
		for _, want := range []string{
			`codrax --htrace "out.perftrace"`,
			"可聚合已验证的 CPU sample",
			"没有 systrace trace body",
			"不能做 trace 时间窗或调度因果关联",
		} {
			if !strings.Contains(zh, want) {
				t.Fatalf("zh direct perf-only next line omitted %q:\n%s", want, zh)
			}
		}
	})
}

func TestTraceConvertNextLineNeitherArtifactIsMetadataOnly(t *testing.T) {
	result := hitraceconv.Result{
		BundlePath: "out.tracebundle.json",
		Artifacts: []hitraceconv.Artifact{{
			Type: hitraceconv.ArtifactPerfTrace,
			Path: "out.perftrace",
		}},
	}
	en := traceConvertNextLine("en", result)
	if !strings.Contains(en, "preserves artifact/provenance metadata only") ||
		!strings.Contains(en, "no query-ready systrace or validated perftrace is available") ||
		strings.Contains(en, "aggregate validated CPU samples") {
		t.Fatalf("metadata-only bundle gained a query-ready claim: %q", en)
	}
	zh := traceConvertNextLine("zh", result)
	if !strings.Contains(zh, "仅保存 artifact/provenance") ||
		!strings.Contains(zh, "没有可直接查询的 systrace 或已验证 perftrace") ||
		strings.Contains(zh, "可聚合已验证的 CPU sample") {
		t.Fatalf("zh metadata-only bundle gained a query-ready claim: %q", zh)
	}
}

func TestTraceConvertNextLineKeepsInventoryPrimaryOffCausalLane(t *testing.T) {
	result := hitraceconv.Result{
		OutputPath: "primary.systrace",
		BundlePath: "capture.tracebundle",
		Artifacts: []hitraceconv.Artifact{
			traceConvertTestSystraceArtifact("primary.systrace", false),
			traceConvertTestSystraceArtifact("secondary.systrace", true),
		},
	}
	for lang, wants := range map[string][]string{
		"en": {"capture.tracebundle", "systrace inventory artifact", "not receipt-validated as query-ready", "cannot support core-event or scheduling-causality queries"},
		"zh": {"capture.tracebundle", "systrace 库存 artifact", "未经收据验证为可查询", "不能用于核心事件或调度因果查询"},
	} {
		got := traceConvertNextLine(lang, result)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Fatalf("%s inventory disclosure missing %q: %s", lang, want, got)
			}
		}
		for _, forbidden := range []string{"core systrace event queries", "可查询 systrace 核心事件", "secondary.systrace"} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("%s inventory primary borrowed readiness via %q: %s", lang, forbidden, got)
			}
		}
	}
}

func TestTraceConvertNextLineOutputPathAloneCannotMintReadiness(t *testing.T) {
	result := hitraceconv.Result{OutputPath: "unattested.systrace"}
	for _, lang := range []string{"en", "zh"} {
		got := traceConvertNextLine(lang, result)
		if strings.Contains(got, "unattested.systrace") || strings.Contains(got, "--htrace") {
			t.Fatalf("%s output path alone minted an attach/query instruction: %s", lang, got)
		}
	}
}

func TestTraceConvertCoverageLinesPrioritizeExactPerfReceipt(t *testing.T) {
	coverage := make([]hitraceconv.TraceDBCoverage, 0, 10)
	for i := 0; i < 5; i++ {
		coverage = append(coverage, hitraceconv.TraceDBCoverage{
			Family: "regular", Table: fmt.Sprintf("table_%d", i), Role: "query_ready_export",
			RowsRead: 1, RowsEmitted: 1,
		})
	}
	coverage = append(coverage,
		hitraceconv.TraceDBCoverage{
			Family: "trace_cross_validation_v2", Table: "perftrace_simpleperf_text",
			Role: "tracequery_cross_validation", ArtifactPath: "wrong_family.perftrace",
		},
		hitraceconv.TraceDBCoverage{
			Family: "trace_cross_validation", Table: "perftrace_future",
			Role: "tracequery_cross_validation", ArtifactPath: "future_table.perftrace",
		},
		hitraceconv.TraceDBCoverage{
			Family: "trace_cross_validation", Table: "perftrace_simpleperf_text",
			Role: "tracequery_cross_validation_v2", ArtifactPath: "wrong_role.perftrace",
		},
		hitraceconv.TraceDBCoverage{
			Family: "trace_cross_validation", Table: "perftrace_simpleperf_text",
			Role: "tracequery_cross_validation", ArtifactPath: " padded.perftrace ",
		},
		hitraceconv.TraceDBCoverage{
			Family: "trace_cross_validation", Table: "perftrace_simpleperf_text",
			Role: "tracequery_cross_validation", ArtifactPath: "artifacts/capture.perftrace",
			RowsRead: 7, RowsEmitted: 7,
		},
	)

	en := strings.Join(traceConvertCoverageLines("en", "trace_coverage", coverage), "\n")
	for _, want := range []string{
		"trace_coverage[0]:",
		"trace_coverage[4]:",
		"trace_coverage[9]:",
		"artifact=artifacts/capture.perftrace",
		"role=tracequery_cross_validation",
		"trace_coverage_compacted: total=10 shown=6 omitted=4",
	} {
		if !strings.Contains(en, want) {
			t.Fatalf("exact perf receipt coverage omitted %q:\n%s", want, en)
		}
	}
	for _, absent := range []string{
		"trace_coverage[5]:",
		"trace_coverage[6]:",
		"trace_coverage[7]:",
		"trace_coverage[8]:",
		"artifact=wrong_family.perftrace",
		"artifact=future_table.perftrace",
		"artifact=wrong_role.perftrace",
		"artifact= padded.perftrace ",
	} {
		if strings.Contains(en, absent) {
			t.Fatalf("fuzzy perf receipt gained a priority seat via %q:\n%s", absent, en)
		}
	}
	zh := strings.Join(traceConvertCoverageLines("zh", "trace_coverage", coverage), "\n")
	for _, want := range []string{
		"trace_coverage[9]：",
		"artifact=artifacts/capture.perftrace",
		"用途=trace_query 交叉验证",
		"trace_coverage 明细已压缩：总计=10 已显示=6 省略=4",
	} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh exact perf receipt coverage omitted %q:\n%s", want, zh)
		}
	}

	db := strings.Join(traceConvertCoverageLines("en", "trace_db_coverage", coverage), "\n")
	if strings.Contains(db, "trace_db_coverage[9]:") || strings.Contains(db, "artifact=artifacts/capture.perftrace") {
		t.Fatalf("perf receipt escaped its trace_coverage lane:\n%s", db)
	}
	if !strings.Contains(db, "trace_db_coverage_compacted: total=10 shown=5 omitted=5") {
		t.Fatalf("DB lane compacted summary should disclose that no perf priority seat was granted:\n%s", db)
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

func TestTraceConvertArtifactLinesExposeSystraceInventoryCapability(t *testing.T) {
	artifact := traceConvertTestSystraceArtifact("capture.systrace", false)
	artifact.Bytes = 456
	artifact.Trace.Rows = 7
	artifact.Trace.Known = 5
	artifact.Trace.AuthoritativeKnown = 4
	artifact.Trace.AdvisoryRows = 1
	artifact.Trace.IntentionalUnknown = 1
	artifact.Trace.IntentionalHeaderOnly = 1
	for lang, wants := range map[string][]string{
		"en": {"artifact[systrace]", "format=text_systrace", "trace_provider=codrax_builtin_modern_profiler", "validation_profile=builtin_systrace_v1", "trace_rows=7", "trace_known=5", "authoritative_known=4", "advisory_rows=1", "intentional_unknown=1", "intentional_header_only=1", "trace_query_ready=false", "trace_state=inventory_only_not_causal_query_ready"},
		"zh": {"artifact[systrace]", "格式=文本 systrace", "trace提供方=codrax_builtin_modern_profiler", "验证profile=builtin_systrace_v1", "trace行数=7", "trace已知行=5", "权威已知行=4", "可供trace_query消费=否", "trace状态=仅库存，不可用于因果查询"},
	} {
		got := strings.Join(traceConvertArtifactLines(lang, []hitraceconv.Artifact{artifact}), "\n")
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Fatalf("%s systrace capability missing %q:\n%s", lang, want, got)
			}
		}
	}
}

func TestTraceConvertResultLinesIncludeProviderDecisions(t *testing.T) {
	result := hitraceconv.Result{
		InputPath: "capture.bin",
		TraceDecisions: []hitraceconv.TraceProviderDecision{{
			Stage:           "trace_body",
			ProviderKind:    "builtin_modern",
			ProviderName:    "codrax_builtin_modern_profiler",
			OutputPath:      "capture.systrace",
			EngineMode:      "auto",
			Selected:        true,
			Attempted:       true,
			Succeeded:       true,
			Fallback:        true,
			TraceQueryReady: true,
			ArtifactPath:    "capture.systrace",
		}},
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
	for _, want := range []string{"trace_provider_decision[builtin_modern/codrax_builtin_modern_profiler]", "engine=auto", "provider_decision[raw_fallback/codrax_raw_perfdata]", "selected=true", "attempted=true", "succeeded=true", "trace_query_ready=true", "stage=direct_input", "parser=raw", "input=linux_perf_data"} {
		if !strings.Contains(en, want) {
			t.Fatalf("provider decision output missing %q:\n%s", want, en)
		}
	}
	zh := strings.Join(traceConvertResultLines("zh", result), "\n")
	if !strings.Contains(zh, "trace_provider_decision[builtin_modern/codrax_builtin_modern_profiler]：") || !strings.Contains(zh, "provider_decision[raw_fallback/codrax_raw_perfdata]：") || !strings.Contains(zh, "已成功=是") || !strings.Contains(zh, "回退路径=否") {
		t.Fatalf("zh provider decision output malformed:\n%s", zh)
	}
}

func TestTraceConvertTraceToolStatusLines(t *testing.T) {
	status := hitraceconv.TraceToolStatus{
		RequestedEngine: "auto",
		PreflightEngine: "trace_streamer",
		FirstLane:       "trace_streamer",
		OrderedRoute:    []string{"trace_streamer", "builtin"},
		TraceStreamer: hitraceconv.TraceToolProviderStatus{
			Name:            "trace_streamer_db",
			Kind:            "official_trace_db",
			Available:       true,
			Path:            "/tmp/trace_streamer",
			Source:          "configured trace_streamer",
			CheckCommand:    "trace_streamer --help",
			AuxiliaryChecks: []string{"so_dir=/symbols check=test -d /symbols", "db_output=/tmp/trace.db check=parent_writable"},
			InstallCommand:  "Install OpenHarmony/SmartPerf trace_streamer",
			DocsURL:         "https://gitcode.com/diting/hmtrace/tree/main",
			Caveats:         []string{"trace_streamer DB export is the preferred trace body path for trace+perf htrace and can also normalize trace-only captures to systrace with tracebundle coverage for trace_query; auto falls back to the built-in raw trace parser when SQL is unavailable or fails"},
		},
		BuiltinModern: hitraceconv.TraceToolProviderStatus{
			Name:           "codrax_builtin_modern_profiler",
			Kind:           "builtin_modern",
			Available:      true,
			Source:         "built-in",
			CheckCommand:   "codrax trace convert --trace-engine=builtin",
			InstallCommand: "built-in",
			Caveats:        []string{"built-in modern/sys parser is selected explicitly with --trace-engine=builtin or used by auto after trace_streamer is unavailable/fails; explicit trace_streamer mode does not fall back"},
		},
		SysBinaryParity: hitraceconv.TraceToolGateStatus{
			Name:                 "no_perf_sys_binary_parity",
			State:                "pending_representative_fixture",
			Proven:               false,
			FixtureManifestCount: 0,
			RequiredEvidence:     "commit a redistributable real no-perf Harmony/Donghu .sys fixture manifest under internal/hitraceconv/testdata/representative_sys_traces and pass TestRepresentativeSysTraceFixtures",
			Evidence:             []string{"synthetic scheduler/raw-ftrace parity guards are delivered", "representative_fixture_manifests=0"},
			Caveats: []string{
				"no redistributable representative no-perf .sys fixture has been committed; the built-in sys binary parser remains an explicit guarded lane",
				"trace+perf htrace in auto mode may fall back to the built-in raw trace parser when SQL is unavailable or fails; explicit trace_streamer mode never falls back",
			},
		},
		Caveats: []string{"auto trace engine discovered trace_streamer; conversion will use SQL first and fall back to the built-in raw trace parser if SQL execution or normalization fails"},
	}
	en := strings.Join(traceConvertTraceToolStatusLines("en", status), "\n")
	for _, want := range []string{"requested_engine: auto", "ordered_route: trace_streamer -> builtin", "first_lane: trace_streamer", "preflight_engine: trace_streamer", "actual_engine: authoritative in post-conversion trace_provider_decision", "trace_provider[official_trace_db/trace_streamer_db]", "state=available", "/tmp/trace_streamer", "aux_check=so_dir=/symbols", "docs=https://gitcode.com/diting/hmtrace/tree/main", "trace_provider[builtin_modern/codrax_builtin_modern_profiler]", "trace_gate[sys_binary_parity_gate/no_perf_sys_binary_parity]", "state=pending_representative_fixture", "fixture_manifests=0", "built-in sys binary parser remains an explicit guarded lane"} {
		if !strings.Contains(en, want) {
			t.Fatalf("trace status lines missing %q:\n%s", want, en)
		}
	}
	zh := strings.Join(traceConvertTraceToolStatusLines("zh", status), "\n")
	for _, want := range []string{"请求引擎：auto", "有序路由：trace_streamer → builtin", "首车道：trace_streamer", "预检引擎：trace_streamer", "实际引擎：以转换后的 trace_provider_decision 为准", "状态=可用", "来源=已配置 trace_streamer", "辅助检查=so_dir=/symbols", "文档=https://gitcode.com/diting/hmtrace/tree/main", "注意=trace_streamer DB export 是 trace+perf htrace 的优先 trace body 路径", "trace_gate[sys_binary_parity_gate/no_perf_sys_binary_parity]", "状态=等待代表性fixture", "fixture清单=0", "尚未提交可再分发的真实代表性 no-perf .sys fixture", "提示：auto trace 引擎已发现 trace_streamer"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh trace status lines missing %q:\n%s", want, zh)
		}
	}
	for _, leak := range []string{"trace_streamer DB export is the preferred", "auto trace engine discovered trace_streamer", "built-in sys binary parser remains an explicit guarded lane", "trace+perf htrace in auto mode may fall back"} {
		if strings.Contains(zh, leak) {
			t.Fatalf("zh trace status leaked English detail %q:\n%s", leak, zh)
		}
	}
}

func TestTraceConvertTraceToolStatusLinesIncludeInputClassification(t *testing.T) {
	status := hitraceconv.TraceToolStatus{
		RequestedEngine:     "auto",
		PreflightEngine:     "builtin",
		FirstLane:           "trace_streamer",
		OrderedRoute:        []string{"trace_streamer", "builtin"},
		InputPath:           "capture.htrace",
		InputInspected:      true,
		InputKind:           "trace_perf",
		InputHasPerfSidecar: true,
		Caveats: []string{
			"auto trace engine did not discover trace_streamer; inspected input contains a standalone perf sidecar, so conversion will use built-in raw trace parsing and standalone perf fallback",
		},
	}
	en := strings.Join(traceConvertTraceToolStatusLines("en", status), "\n")
	for _, want := range []string{"trace_input: path=capture.htrace", "kind=trace_perf", "inspected=true", "has_perf_sidecar=true", "requested_engine: auto", "ordered_route: trace_streamer -> builtin", "first_lane: trace_streamer", "preflight_engine: builtin (reason=trace_streamer_unavailable)", "standalone perf fallback"} {
		if !strings.Contains(en, want) {
			t.Fatalf("trace status input classification missing %q:\n%s", want, en)
		}
	}
	zh := strings.Join(traceConvertTraceToolStatusLines("zh", status), "\n")
	for _, want := range []string{"trace 输入：路径=capture.htrace", "类型=trace+perf", "已检查=true", "包含perf=true", "请求引擎：auto", "有序路由：trace_streamer → builtin", "首车道：trace_streamer", "预检引擎：builtin（原因=trace_streamer不可用）", "已检查输入包含独立 perf sidecar"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh trace status input classification missing %q:\n%s", want, zh)
		}
	}
	for _, leak := range []string{"inspected input contains a standalone perf sidecar", "standalone perf fallback"} {
		if strings.Contains(zh, leak) {
			t.Fatalf("zh trace status leaked English %q:\n%s", leak, zh)
		}
	}
}

func TestTraceConvertTraceToolStatusLinesExposeExecutionBlocker(t *testing.T) {
	blocker := "direct perf input has no trace body and cannot be combined with trace-only option(s) --trace-streamer"
	status := hitraceconv.TraceToolStatus{
		RequestedEngine:  "auto",
		OrderedRoute:     []string{"direct_perf"},
		FirstLane:        "direct_perf",
		PreflightEngine:  "direct_perf",
		ExecutionBlocker: blocker,
		Caveats: []string{
			"trace provider route is not applicable because the inspected input is a typed standalone perf capture with no trace body",
			"execution_blocked: " + blocker,
		},
		TraceStreamer: hitraceconv.TraceToolProviderStatus{Kind: "official_trace_db", Name: "trace_streamer_db", InstallCommand: "must not render"},
		BuiltinModern: hitraceconv.TraceToolProviderStatus{Kind: "builtin_modern", Name: "codrax_builtin_modern_profiler", InstallCommand: "must not render"},
	}
	en := strings.Join(traceConvertTraceToolStatusLines("en", status), "\n")
	if !strings.Contains(en, "execution_blocker: "+blocker) || strings.Count(en, blocker) != 1 {
		t.Fatalf("English status did not expose exactly one explicit execution blocker:\n%s", en)
	}
	for _, want := range []string{"trace_provider[official_trace_db/trace_streamer_db]: state=not_applicable", "trace_provider[builtin_modern/codrax_builtin_modern_profiler]: state=not_applicable"} {
		if !strings.Contains(en, want) {
			t.Fatalf("direct-perf provider status missing %q:\n%s", want, en)
		}
	}
	if strings.Contains(en, "state=missing") || strings.Contains(en, "install=must not render") {
		t.Fatalf("direct-perf status falsely advertised a trace provider dependency gap:\n%s", en)
	}
	zh := strings.Join(traceConvertTraceToolStatusLines("zh", status), "\n")
	for _, want := range []string{"执行阻断：", "direct perf 输入不包含 trace body", "--trace-streamer"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("Chinese status execution blocker missing %q:\n%s", want, zh)
		}
	}
	if strings.Contains(zh, "direct perf input has no trace body") || strings.Contains(zh, "trace provider route is not applicable") || strings.Count(zh, "执行阻断：") != 1 || strings.Contains(zh, "execution_blocker") || strings.Contains(zh, "状态=缺失") || strings.Contains(zh, "安装=must not render") {
		t.Fatalf("Chinese status leaked/duplicated execution blocker:\n%s", zh)
	}
}

func TestTraceConvertTraceMessageZhKeepsSoDirHintSpecific(t *testing.T) {
	got := traceConvertTraceMessageZh("so_dirs=not_configured; pass --trace-streamer-so-dir /path/to/so when native symbol reload is needed")
	if !strings.Contains(got, "未配置 so_dir") || !strings.Contains(got, "--trace-streamer-so-dir") {
		t.Fatalf("so-dir auxiliary check was swallowed by the generic trace_streamer install hint: %q", got)
	}
	if strings.Contains(got, "CODRAX_TRACE_STREAMER") {
		t.Fatalf("so-dir auxiliary check was mislabeled as binary discovery guidance: %q", got)
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

func TestTraceConvertPerfToolsStatusIncludesTraceStreamerStatus(t *testing.T) {
	langFlag := traceConvertCmd.Flag("lang")
	oldLangChanged := langFlag.Changed
	oldInput := traceConvertInput
	oldOutput := traceConvertOutput
	oldFlavor := traceConvertFlavor
	oldHiperfHost := traceConvertHiperfHost
	oldSymbolDirs := append([]string(nil), traceConvertSymbolDirs...)
	oldSimpleperf := traceConvertSimpleperf
	oldSPPython := traceConvertSPPython
	oldSPSymfs := traceConvertSPSymfs
	oldSPKallsyms := traceConvertSPKallsyms
	oldPerfParser := traceConvertPerfParser
	oldNoPerfTrace := traceConvertNoPerfTrace
	oldToolsStatus := traceConvertToolsStatus
	oldTraceToolsStatus := traceConvertTraceToolsStatus
	oldTraceEngine := traceConvertTraceEngine
	oldTraceStreamer := traceConvertTraceStreamer
	oldTraceDBOutput := traceConvertTraceDBOutput
	oldKeepTraceDB := traceConvertKeepTraceDB
	oldTraceStreamerSoDirs := append([]string(nil), traceConvertTraceStreamerSoDirs...)
	oldFlagLang := flagLang
	t.Cleanup(func() {
		traceConvertInput = oldInput
		traceConvertOutput = oldOutput
		traceConvertFlavor = oldFlavor
		traceConvertHiperfHost = oldHiperfHost
		traceConvertSymbolDirs = oldSymbolDirs
		traceConvertSimpleperf = oldSimpleperf
		traceConvertSPPython = oldSPPython
		traceConvertSPSymfs = oldSPSymfs
		traceConvertSPKallsyms = oldSPKallsyms
		traceConvertPerfParser = oldPerfParser
		traceConvertNoPerfTrace = oldNoPerfTrace
		traceConvertToolsStatus = oldToolsStatus
		traceConvertTraceToolsStatus = oldTraceToolsStatus
		traceConvertTraceEngine = oldTraceEngine
		traceConvertTraceStreamer = oldTraceStreamer
		traceConvertTraceDBOutput = oldTraceDBOutput
		traceConvertKeepTraceDB = oldKeepTraceDB
		traceConvertTraceStreamerSoDirs = oldTraceStreamerSoDirs
		flagLang = oldFlagLang
		langFlag.Changed = oldLangChanged
		traceConvertCmd.SetOut(nil)
	})

	traceConvertInput = ""
	traceConvertOutput = ""
	traceConvertFlavor = ""
	traceConvertHiperfHost = ""
	traceConvertSymbolDirs = nil
	traceConvertSimpleperf = ""
	traceConvertSPPython = ""
	traceConvertSPSymfs = ""
	traceConvertSPKallsyms = ""
	traceConvertPerfParser = "auto"
	traceConvertNoPerfTrace = false
	traceConvertToolsStatus = true
	traceConvertTraceToolsStatus = false
	traceConvertTraceEngine = "auto"
	traceConvertTraceStreamer = ""
	traceConvertTraceDBOutput = ""
	traceConvertKeepTraceDB = false
	traceConvertTraceStreamerSoDirs = nil
	flagLang = "en"
	langFlag.Changed = true
	t.Setenv("CODRAX_TRACE_STREAMER", "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OHOS_SDK_HOME", "")
	t.Setenv("HARMONYOS_SDK_HOME", "")
	t.Setenv("DEVECO_SDK_HOME", "")
	t.Setenv("TRACE_STREAMER_HOME", "")

	var out bytes.Buffer
	traceConvertCmd.SetOut(&out)
	if err := traceConvertCmd.RunE(traceConvertCmd, nil); err != nil {
		t.Fatalf("run perf tools status: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"trace_provider[official_trace_db/trace_streamer_db]",
		"requested_engine: auto",
		"first_lane: trace_streamer",
		"perf_parser: auto",
		"raw_fallback",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("--perf-tools-status output missing %q:\n%s", want, got)
		}
	}
}
