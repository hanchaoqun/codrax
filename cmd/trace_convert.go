package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

var (
	traceConvertInput       string
	traceConvertOutput      string
	traceConvertFlavor      string
	traceConvertHiperfHost  string
	traceConvertSymbolDirs  []string
	traceConvertSimpleperf  string
	traceConvertSPPython    string
	traceConvertSPSymfs     string
	traceConvertSPKallsyms  string
	traceConvertPerfParser  string
	traceConvertNoPerfTrace bool
	traceConvertToolsStatus bool
)

var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Runtime trace utilities",
}

var traceConvertCmd = &cobra.Command{
	Use:   "convert --input <binary-hitrace> [--output <text.systrace>]",
	Short: "Convert a binary Harmony/OpenHarmony HiTrace file to text systrace",
	Long: `Convert a binary Harmony/OpenHarmony HiTrace capture to an
ftrace/systrace-compatible text file plus tracebundle metadata that Codrax can
later analyze with --htrace, /htrace, and trace_query. When perf sidecars are
present, the bundle preserves the systrace + perftrace pair so trace_query can
correlate scheduler/running evidence with CPU samples.

Perf sample conversion uses a two-engine strategy. In --perf-parser=auto,
Codrax prefers official OpenHarmony hiperf or Android simpleperf adapters for
symbolized output, then falls back to its built-in raw perf.data parser when
possible. Use --perf-tools-status to inspect discovered tools, selected parser,
raw fallback status, and install hints. Use --perf-parser=official to require
official tooling or --perf-parser=raw for the built-in fallback only.

The command is intentionally manual and does not attach the generated file.
When --output is omitted, Codrax writes <input>.systrace. Existing output files
are never overwritten; delete the file first or choose another output path.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		input := strings.TrimSpace(traceConvertInput)
		opts := hitraceconv.Options{
			InputPath:              input,
			OutputPath:             strings.TrimSpace(traceConvertOutput),
			Flavor:                 strings.TrimSpace(traceConvertFlavor),
			HiperfPath:             strings.TrimSpace(traceConvertHiperfHost),
			HiperfSymbolDirs:       append([]string(nil), traceConvertSymbolDirs...),
			SimpleperfReportPath:   strings.TrimSpace(traceConvertSimpleperf),
			SimpleperfPythonPath:   strings.TrimSpace(traceConvertSPPython),
			SimpleperfSymfsDir:     strings.TrimSpace(traceConvertSPSymfs),
			SimpleperfKallsymsPath: strings.TrimSpace(traceConvertSPKallsyms),
			PerfParser:             strings.TrimSpace(traceConvertPerfParser),
			DisablePerfAdapter:     traceConvertNoPerfTrace,
		}
		if traceConvertToolsStatus {
			status, err := hitraceconv.BuildPerfToolStatus(opts)
			if err != nil {
				return err
			}
			for _, line := range traceConvertPerfToolStatusLines(flagLang, status) {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		}
		if input == "" {
			return fmt.Errorf("--input is required")
		}
		result, err := hitraceconv.ConvertFile(cmd.Context(), opts)
		if err != nil {
			return err
		}
		for _, line := range traceConvertResultLines(flagLang, result) {
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
		if len(result.Caveats) > 0 {
			for _, caveat := range result.Caveats {
				fmt.Fprintln(cmd.OutOrStdout(), traceConvertCaveatLine(flagLang, caveat))
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), traceConvertNextLine(flagLang, result))
		return nil
	},
}

func traceConvertPerfToolStatusLines(lang string, status hitraceconv.PerfToolStatus) []string {
	if traceConvertUseZh(lang) {
		lines := []string{
			fmt.Sprintf("perf 解析模式：%s", status.ParserMode),
			fmt.Sprintf("选中策略：%s", status.SelectedParser),
			fmt.Sprintf("符号化预期：%s", traceConvertPerfSymbolizationExpectationZh(status)),
		}
		lines = append(lines, traceConvertPerfProviderLine("zh", status.Hiperf))
		lines = append(lines, traceConvertPerfProviderLine("zh", status.Simpleperf))
		lines = append(lines, traceConvertPerfProviderLine("zh", status.RawFallback))
		for _, caveat := range status.Caveats {
			lines = append(lines, "提示："+traceConvertPerfMessageZh(caveat))
		}
		return lines
	}
	lines := []string{
		fmt.Sprintf("perf_parser: %s", status.ParserMode),
		fmt.Sprintf("selected_parser: %s", status.SelectedParser),
		fmt.Sprintf("symbolization_expectation: %s", status.SymbolizationExpectation),
	}
	lines = append(lines, traceConvertPerfProviderLine("en", status.Hiperf))
	lines = append(lines, traceConvertPerfProviderLine("en", status.Simpleperf))
	lines = append(lines, traceConvertPerfProviderLine("en", status.RawFallback))
	for _, caveat := range status.Caveats {
		lines = append(lines, "caveat: "+caveat)
	}
	return lines
}

func traceConvertPerfProviderLine(lang string, item hitraceconv.PerfToolProviderStatus) string {
	prefix := fmt.Sprintf("%s[%s]", item.Kind, item.Name)
	if traceConvertUseZh(lang) {
		return prefix + "：" + strings.Join(traceConvertPerfProviderDetailsZh(item), " ")
	}
	return prefix + ": " + strings.Join(traceConvertPerfProviderDetailsEn(item), " ")
}

func traceConvertPerfProviderDetailsEn(item hitraceconv.PerfToolProviderStatus) []string {
	state := "missing"
	if item.Available {
		state = "available"
	}
	details := []string{
		fmt.Sprintf("state=%s", state),
	}
	if item.Path != "" {
		details = append(details, "path="+item.Path)
	}
	if item.Python != "" {
		details = append(details, "python="+item.Python)
	}
	if item.Source != "" {
		details = append(details, "source="+item.Source)
	}
	if item.Version != "" {
		details = append(details, "version="+item.Version)
	}
	if item.CheckCommand != "" {
		details = append(details, "check="+item.CheckCommand)
	}
	if len(item.AuxiliaryChecks) > 0 {
		details = append(details, "aux_check="+strings.Join(item.AuxiliaryChecks, "; "))
	}
	if item.InstallCommand != "" {
		details = append(details, "install="+item.InstallCommand)
	}
	if item.DocsURL != "" {
		details = append(details, "docs="+item.DocsURL)
	}
	if len(item.Caveats) > 0 {
		details = append(details, "caveat="+strings.Join(item.Caveats, "; "))
	}
	if !item.Available && item.InstallHint != "" {
		details = append(details, "hint="+item.InstallHint)
	}
	return details
}

func traceConvertPerfProviderDetailsZh(item hitraceconv.PerfToolProviderStatus) []string {
	state := "缺失"
	if item.Available {
		state = "可用"
	}
	details := []string{
		fmt.Sprintf("状态=%s", state),
	}
	if item.Path != "" {
		details = append(details, "路径="+item.Path)
	}
	if item.Python != "" {
		details = append(details, "Python="+item.Python)
	}
	if item.Source != "" {
		details = append(details, "来源="+traceConvertPerfSourceZh(item.Source))
	}
	if item.Version != "" {
		details = append(details, "版本="+item.Version)
	}
	if item.CheckCommand != "" {
		details = append(details, "检查="+item.CheckCommand)
	}
	if len(item.AuxiliaryChecks) > 0 {
		details = append(details, "辅助检查="+strings.Join(traceConvertPerfAuxChecksZh(item.AuxiliaryChecks), "; "))
	}
	if item.InstallCommand != "" {
		details = append(details, "安装="+traceConvertPerfInstallCommandZh(item.InstallCommand))
	}
	if item.DocsURL != "" {
		details = append(details, "文档="+item.DocsURL)
	}
	if len(item.Caveats) > 0 {
		details = append(details, "注意="+strings.Join(traceConvertPerfMessagesZh(item.Caveats), "; "))
	}
	if !item.Available && item.InstallHint != "" {
		details = append(details, "提示="+traceConvertPerfMessageZh(item.InstallHint))
	}
	return details
}

func traceConvertPerfSymbolizationExpectationZh(status hitraceconv.PerfToolStatus) string {
	if strings.EqualFold(strings.TrimSpace(status.SelectedParser), "disabled") {
		return "不会生成 .perftrace"
	}
	switch strings.ToLower(strings.TrimSpace(status.ParserMode)) {
	case "official":
		return "要求使用官方 hiperf/simpleperf 适配器；提供匹配符号后可输出符号化结果"
	case "raw", "fallback":
		return "仅使用 Codrax 内置 raw perf.data 保底解析；存在官方 HIPERF_FILES_SYMBOL 时可使用其中保存的函数名，否则输出为 IP/DSO 级上下文"
	default:
		return "auto 优先使用官方 hiperf/simpleperf 生成符号化结果；官方工具不可用时回退到 raw perf.data，并在存在 HIPERF_FILES_SYMBOL 时使用其中保存的函数名"
	}
}

func traceConvertPerfAuxChecksZh(checks []string) []string {
	out := make([]string, 0, len(checks))
	for _, check := range checks {
		out = append(out, traceConvertPerfAuxCheckZh(check))
	}
	return out
}

func traceConvertPerfMessagesZh(messages []string) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		out = append(out, traceConvertPerfMessageZh(msg))
	}
	return out
}

func traceConvertPerfAuxCheckZh(check string) string {
	trimmed := strings.TrimSpace(check)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "symbol_roots=not_configured"):
		return "符号目录未配置；可传 --hiperf-symbol-dir /path/to/symbols，并用 test -d /path/to/symbols 验证"
	case strings.Contains(lower, "symfs=not_configured"):
		return "symfs 未配置；可传 --simpleperf-symfs /path/to/symfs，并用 test -d /path/to/symfs 验证"
	case strings.Contains(lower, "kallsyms=not_configured"):
		return "kallsyms 未配置；可传 --simpleperf-kallsyms /path/to/kallsyms，并用 test -r /path/to/kallsyms 验证"
	default:
		replacer := strings.NewReplacer(
			"symbol_root=", "符号目录=",
			"symbol_roots=", "符号目录=",
			"symfs=", "symfs=",
			"kallsyms=", "kallsyms=",
			" check=", " 检查=",
		)
		return replacer.Replace(trimmed)
	}
}

func traceConvertPerfSourceZh(source string) string {
	trimmed := strings.TrimSpace(source)
	lower := strings.ToLower(trimmed)
	switch {
	case trimmed == "":
		return ""
	case strings.EqualFold(trimmed, "built-in"):
		return "内置"
	case strings.Contains(lower, "configured hiperf"):
		return "已配置 hiperf 工具"
	case strings.Contains(lower, "configured simpleperf_report_lib.py"):
		return "已配置 simpleperf_report_lib.py"
	case strings.Contains(lower, "configured simpleperf"):
		return "已配置 simpleperf report_sample.py"
	case strings.Contains(lower, "next to") && strings.Contains(lower, "on path"):
		return trimmed + "（从 PATH 发现 wrapper）"
	case strings.Contains(lower, "on path"):
		return trimmed + "（从 PATH 发现）"
	default:
		return trimmed
	}
}

func traceConvertPerfInstallCommandZh(command string) string {
	trimmed := strings.TrimSpace(command)
	if strings.EqualFold(trimmed, "built-in") {
		return "内置，无需安装"
	}
	if trimmed == "" {
		return ""
	}
	return trimmed
}

func traceConvertPerfMessageZh(message string) string {
	trimmed := strings.TrimSpace(message)
	lower := strings.ToLower(trimmed)
	switch {
	case trimmed == "":
		return ""
	case strings.Contains(lower, "install or build openharmony developtools_hiperf"):
		return "安装或构建 OpenHarmony developtools_hiperf host 工具，然后传 --hiperf-host 或设置 CODRAX_HIPERF_HOST；需要符号化时补 --hiperf-symbol-dir"
	case strings.Contains(lower, "use android simpleperf scripts/report_sample.py"):
		return "使用 Android simpleperf 的 scripts/report_sample.py，然后传 --simpleperf-report-sample 或设置 CODRAX_SIMPLEPERF_REPORT_SAMPLE；按需补 --simpleperf-python、--simpleperf-symfs、--simpleperf-kallsyms"
	case strings.Contains(lower, "built into codrax"):
		return "Codrax 内置能力；输出会标记 source=raw_perfdata_fallback；存在官方 HIPERF_FILES_SYMBOL 时可使用保存的函数名，否则适合时间/线程/DSO/IP 关联，不等同完整符号化"
	case strings.Contains(lower, "perftrace generation is disabled"):
		return "已禁用 perftrace 生成；不会运行官方适配器和 raw fallback"
	case strings.Contains(lower, "disabled by --no-perftrace"):
		return "已被 --no-perftrace 禁用"
	case strings.Contains(lower, "disabled by --perf-parser=official"):
		return "已被 --perf-parser=official 禁用"
	case strings.Contains(lower, "python executable was not discovered"):
		return "没有发现可执行的 Python，无法运行 report_sample.py"
	case strings.Contains(lower, "simpleperf_report_lib.py is the official library"):
		return "simpleperf_report_lib.py 是官方库文件；Codrax 需要执行 report_sample.py wrapper，请把 report_sample.py 放到同目录，或通过 --simpleperf-report-sample 指定"
	case strings.Contains(lower, "configured path is not readable"):
		return strings.Replace(trimmed, "configured path is not readable", "配置路径不可读", 1)
	case strings.Contains(lower, "configured path is a directory"):
		return "配置路径是目录，需要传具体可执行文件或脚本"
	default:
		return trimmed
	}
}

func traceConvertResultLines(lang string, result hitraceconv.Result) []string {
	if traceConvertUseZh(lang) {
		lines := []string{
			fmt.Sprintf("已转换二进制 hitrace：%s", result.InputPath),
			fmt.Sprintf("事件：%d，跳过缺失格式：%d，仅行头事件：%d", result.EventsWritten, result.MissingFormatCount, result.UnknownEventCount),
		}
		if result.OutputPath != "" {
			lines = append(lines, fmt.Sprintf("输出：%s", result.OutputPath))
		} else {
			lines = append(lines, "输出：未生成 systrace（仅抽取 sidecar artifact）")
		}
		lines = append(lines, traceConvertArtifactLines("zh", result.Artifacts)...)
		lines = append(lines, traceConvertProviderDecisionLines("zh", result.ProviderDecisions)...)
		return lines
	}
	lines := []string{
		fmt.Sprintf("converted binary hitrace: %s", result.InputPath),
		fmt.Sprintf("events: %d, skipped_missing_formats: %d, header_only_events: %d", result.EventsWritten, result.MissingFormatCount, result.UnknownEventCount),
	}
	if result.OutputPath != "" {
		lines = append(lines, fmt.Sprintf("output: %s", result.OutputPath))
	} else {
		lines = append(lines, "output: no systrace produced (sidecar artifacts only)")
	}
	lines = append(lines, traceConvertArtifactLines("en", result.Artifacts)...)
	lines = append(lines, traceConvertProviderDecisionLines("en", result.ProviderDecisions)...)
	return lines
}

func traceConvertArtifactLines(lang string, artifacts []hitraceconv.Artifact) []string {
	var lines []string
	for _, artifact := range artifacts {
		if artifact.Type == hitraceconv.ArtifactSystrace {
			continue
		}
		details := traceConvertArtifactDetails(artifact)
		if traceConvertUseZh(lang) {
			if details != "" {
				lines = append(lines, fmt.Sprintf("artifact[%s]：%s（%s）", artifact.Type, artifact.Path, details))
			} else {
				lines = append(lines, fmt.Sprintf("artifact[%s]：%s", artifact.Type, artifact.Path))
			}
		} else {
			if details != "" {
				lines = append(lines, fmt.Sprintf("artifact[%s]: %s (%s)", artifact.Type, artifact.Path, details))
			} else {
				lines = append(lines, fmt.Sprintf("artifact[%s]: %s", artifact.Type, artifact.Path))
			}
		}
	}
	return lines
}

func traceConvertArtifactDetails(artifact hitraceconv.Artifact) string {
	var details []string
	if artifact.Bytes > 0 {
		details = append(details, fmt.Sprintf("bytes=%d", artifact.Bytes))
	}
	if artifact.Converter != "" {
		details = append(details, "converter="+artifact.Converter)
	}
	if artifact.Perf != nil {
		details = append(details, traceConvertPerfCapabilityDetails(*artifact.Perf)...)
	}
	if artifact.DataType != 0 {
		details = append(details, fmt.Sprintf("data_type=%d", artifact.DataType))
	}
	if artifact.PluginName != "" {
		details = append(details, "plugin="+artifact.PluginName)
	}
	if artifact.PluginVersion != "" {
		details = append(details, "plugin_version="+artifact.PluginVersion)
	}
	if artifact.SourceOffset > 0 {
		details = append(details, fmt.Sprintf("source_offset=%d", artifact.SourceOffset))
	}
	if artifact.SourceBytes > 0 {
		details = append(details, fmt.Sprintf("source_bytes=%d", artifact.SourceBytes))
	}
	if len(artifact.Caveats) > 0 {
		details = append(details, "caveats="+strings.Join(artifact.Caveats, "; "))
	}
	return strings.Join(details, " ")
}

func traceConvertPerfCapabilityDetails(cap hitraceconv.PerfArtifactCapability) []string {
	var details []string
	if cap.ProviderName != "" {
		details = append(details, "perf_provider="+cap.ProviderName)
	}
	if cap.ProviderKind != "" {
		details = append(details, "perf_provider_kind="+cap.ProviderKind)
	}
	if cap.InputFormat != "" {
		details = append(details, "perf_input="+cap.InputFormat)
	}
	if cap.Symbolization != "" {
		details = append(details, "perf_symbolization="+cap.Symbolization)
	}
	if cap.CPUIdentity != "" {
		details = append(details, "perf_cpu="+cap.CPUIdentity)
	}
	if cap.Callchain != "" {
		details = append(details, "perf_callchain="+cap.Callchain)
	}
	if cap.TimeAlignment != "" {
		details = append(details, "perf_time_alignment="+cap.TimeAlignment)
	}
	if cap.TraceQueryReady {
		details = append(details, "trace_query_ready=true")
	}
	if cap.Degraded {
		details = append(details, "perf_degraded=true")
	}
	return details
}

func traceConvertProviderDecisionLines(lang string, decisions []hitraceconv.PerfProviderDecision) []string {
	var lines []string
	for _, decision := range decisions {
		details := traceConvertProviderDecisionDetails(decision)
		prefix := fmt.Sprintf("provider_decision[%s/%s]", decision.ProviderKind, decision.ProviderName)
		if traceConvertUseZh(lang) {
			lines = append(lines, prefix+"："+strings.Join(details, " "))
		} else {
			lines = append(lines, prefix+": "+strings.Join(details, " "))
		}
	}
	return lines
}

func traceConvertProviderDecisionDetails(decision hitraceconv.PerfProviderDecision) []string {
	details := []string{
		fmt.Sprintf("selected=%t", decision.Selected),
		fmt.Sprintf("attempted=%t", decision.Attempted),
		fmt.Sprintf("succeeded=%t", decision.Succeeded),
		fmt.Sprintf("fallback=%t", decision.Fallback),
		fmt.Sprintf("trace_query_ready=%t", decision.TraceQueryReady),
	}
	if decision.Stage != "" {
		details = append(details, "stage="+decision.Stage)
	}
	if decision.ParserMode != "" {
		details = append(details, "parser="+decision.ParserMode)
	}
	if decision.InputFormat != "" {
		details = append(details, "input="+decision.InputFormat)
	}
	if decision.OutputPath != "" {
		details = append(details, "output="+decision.OutputPath)
	}
	if decision.ArtifactPath != "" {
		details = append(details, "artifact="+decision.ArtifactPath)
	}
	if decision.Reason != "" {
		details = append(details, "reason="+decision.Reason)
	}
	if decision.Caveat != "" {
		details = append(details, "caveat="+decision.Caveat)
	}
	return details
}

func traceConvertCaveatLine(lang, caveat string) string {
	if traceConvertUseZh(lang) {
		return fmt.Sprintf("提示：%s", caveat)
	}
	return fmt.Sprintf("caveat: %s", caveat)
}

func traceConvertNextLine(lang string, result hitraceconv.Result) string {
	if result.BundlePath != "" {
		if traceConvertUseZh(lang) {
			return fmt.Sprintf("下一步：codrax --htrace %q --request <问题>；优先附加 tracebundle，让 systrace 与 perftrace/raw perf provenance 一起进入 trace_query", result.BundlePath)
		}
		return fmt.Sprintf("next: codrax --htrace %q --request <question>; prefer the tracebundle so systrace plus perftrace/raw-perf provenance stay together for trace_query", result.BundlePath)
	}
	if result.OutputPath == "" {
		if traceConvertUseZh(lang) {
			return "下一步：未生成可直接附加的 systrace"
		}
		return "next: no attachable systrace was produced"
	}
	if traceConvertHasArtifact(result.Artifacts, hitraceconv.ArtifactPerfTrace) {
		if traceConvertUseZh(lang) {
			return fmt.Sprintf("下一步：codrax --htrace %q --request <问题>；同时可将 perftrace artifact 用于 trace_query 的 CPU sample 视图", result.OutputPath)
		}
		return fmt.Sprintf("next: codrax --htrace %q --request <question>; the perftrace artifact can also feed trace_query CPU-sample views", result.OutputPath)
	}
	if traceConvertUseZh(lang) {
		return fmt.Sprintf("下一步：codrax --htrace %q --request <问题>", result.OutputPath)
	}
	return fmt.Sprintf("next: codrax --htrace %q --request <question>", result.OutputPath)
}

func traceConvertHasArtifact(artifacts []hitraceconv.Artifact, typ string) bool {
	for _, artifact := range artifacts {
		if artifact.Type == typ {
			return true
		}
	}
	return false
}

func traceConvertUseZh(lang string) bool {
	return !strings.EqualFold(strings.TrimSpace(lang), "en")
}

func init() {
	traceConvertCmd.Flags().StringVar(&traceConvertInput, "input", "", "binary Harmony/OpenHarmony HiTrace input path")
	traceConvertCmd.Flags().StringVar(&traceConvertOutput, "output", "", "text systrace output path; default is <input>.systrace")
	traceConvertCmd.Flags().StringVar(&traceConvertFlavor, "flavor", "harmony_hitrace", "trace flavor metadata for operator audit; default harmony_hitrace")
	traceConvertCmd.Flags().StringVar(&traceConvertHiperfHost, "hiperf-host", "", "official OpenHarmony hiperf_host/hiperf path used to convert HIPERF_DATA perf.data sidecars to .perftrace")
	traceConvertCmd.Flags().StringSliceVar(&traceConvertSymbolDirs, "hiperf-symbol-dir", nil, "symbol directories passed to hiperf report --symbol-dir; repeat or comma-separate values")
	traceConvertCmd.Flags().StringVar(&traceConvertSimpleperf, "simpleperf-report-sample", "", "official Android simpleperf report_sample.py path used to convert perf.data to .perftrace")
	traceConvertCmd.Flags().StringVar(&traceConvertSPPython, "simpleperf-python", "", "python executable used for report_sample.py; default discovers python3/python")
	traceConvertCmd.Flags().StringVar(&traceConvertSPSymfs, "simpleperf-symfs", "", "symfs directory passed to simpleperf report_sample.py --symfs")
	traceConvertCmd.Flags().StringVar(&traceConvertSPKallsyms, "simpleperf-kallsyms", "", "kallsyms file passed to simpleperf report_sample.py --kallsyms")
	traceConvertCmd.Flags().StringVar(&traceConvertPerfParser, "perf-parser", "auto", "perf.data parser strategy: auto uses official hiperf/simpleperf first then raw fallback; official disables raw fallback; raw uses Codrax raw perf.data fallback only")
	traceConvertCmd.Flags().BoolVar(&traceConvertNoPerfTrace, "no-perftrace", false, "preserve perf.data sidecars without generating .perftrace")
	traceConvertCmd.Flags().BoolVar(&traceConvertToolsStatus, "perf-tools-status", false, "print discovered official perf tools, raw fallback availability, selected parser strategy, and install hints")
	traceCmd.AddCommand(traceConvertCmd)
	rootCmd.AddCommand(traceCmd)
}
