package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

var (
	traceConvertInput               string
	traceConvertOutput              string
	traceConvertFlavor              string
	traceConvertHiperfHost          string
	traceConvertSymbolDirs          []string
	traceConvertSimpleperf          string
	traceConvertSPPython            string
	traceConvertSPSymfs             string
	traceConvertSPKallsyms          string
	traceConvertPerfParser          string
	traceConvertNoPerfTrace         bool
	traceConvertToolsStatus         bool
	traceConvertTraceToolsStatus    bool
	traceConvertTraceEngine         string
	traceConvertTraceStreamer       string
	traceConvertTraceDBOutput       string
	traceConvertKeepTraceDB         bool
	traceConvertTraceStreamerSoDirs []string
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
possible. Use --trace-tools-status to inspect trace_streamer discovery and
trace-engine selection. Use --perf-tools-status to inspect the full conversion
toolchain: trace_streamer/trace engine plus perf tools, selected parser, raw
fallback status, and install hints. Use
--perf-parser=official to require official tooling or --perf-parser=raw for the
built-in fallback only. Trace body conversion defaults to --trace-engine=auto:
when trace_streamer is discovered, Codrax uses SQL first; if trace_streamer is
absent or SQL execution/export fails, auto falls back to the built-in raw trace
parser. Explicit trace_streamer or builtin modes do not degrade to another
trace engine.

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
			TraceEngine:            strings.TrimSpace(traceConvertTraceEngine),
			TraceStreamerPath:      strings.TrimSpace(traceConvertTraceStreamer),
			TraceDBOutputPath:      strings.TrimSpace(traceConvertTraceDBOutput),
			KeepTraceDB:            traceConvertKeepTraceDB,
			TraceStreamerSoDirs:    append([]string(nil), traceConvertTraceStreamerSoDirs...),
		}
		opts.Progress = func(event hitraceconv.ProgressEvent) {
			fmt.Fprintln(cmd.OutOrStdout(), traceConvertProgressLine(flagLang, event))
		}
		if traceConvertTraceToolsStatus || traceConvertToolsStatus {
			status, err := hitraceconv.BuildTraceToolStatus(opts)
			if err != nil {
				return err
			}
			for _, line := range traceConvertTraceToolStatusLines(flagLang, status) {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			if traceConvertTraceToolsStatus && !traceConvertToolsStatus {
				return nil
			}
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

func traceConvertTraceToolStatusLines(lang string, status hitraceconv.TraceToolStatus) []string {
	if traceConvertUseZh(lang) {
		lines := []string{
			fmt.Sprintf("trace 解析引擎：%s", status.EngineMode),
			fmt.Sprintf("当前选择：%s", status.SelectedEngine),
		}
		if inputLine := traceConvertTraceInputLine("zh", status); inputLine != "" {
			lines = append(lines, inputLine)
		}
		lines = append(lines, traceConvertTraceProviderLine("zh", status.TraceStreamer))
		lines = append(lines, traceConvertTraceProviderLine("zh", status.BuiltinModern))
		if gateLine := traceConvertTraceGateLine("zh", status.SysBinaryParity); gateLine != "" {
			lines = append(lines, gateLine)
		}
		for _, caveat := range status.Caveats {
			lines = append(lines, "提示："+traceConvertTraceMessageZh(caveat))
		}
		return lines
	}
	lines := []string{
		fmt.Sprintf("trace_engine: %s", status.EngineMode),
		fmt.Sprintf("selected_engine: %s", status.SelectedEngine),
	}
	if inputLine := traceConvertTraceInputLine("en", status); inputLine != "" {
		lines = append(lines, inputLine)
	}
	lines = append(lines, traceConvertTraceProviderLine("en", status.TraceStreamer))
	lines = append(lines, traceConvertTraceProviderLine("en", status.BuiltinModern))
	if gateLine := traceConvertTraceGateLine("en", status.SysBinaryParity); gateLine != "" {
		lines = append(lines, gateLine)
	}
	for _, caveat := range status.Caveats {
		lines = append(lines, "caveat: "+caveat)
	}
	return lines
}

func traceConvertTraceInputLine(lang string, status hitraceconv.TraceToolStatus) string {
	if strings.TrimSpace(status.InputPath) == "" {
		return ""
	}
	if traceConvertUseZh(lang) {
		parts := []string{"trace 输入：路径=" + status.InputPath}
		if status.InputKind != "" {
			parts = append(parts, "类型="+traceConvertTraceInputKindZh(status.InputKind))
		}
		parts = append(parts, fmt.Sprintf("已检查=%t", status.InputInspected))
		parts = append(parts, fmt.Sprintf("包含perf=%t", status.InputHasPerfSidecar))
		if status.InputInspectionError != "" {
			parts = append(parts, "检查错误="+status.InputInspectionError)
		}
		return strings.Join(parts, " ")
	}
	parts := []string{"trace_input: path=" + status.InputPath}
	if status.InputKind != "" {
		parts = append(parts, "kind="+status.InputKind)
	}
	parts = append(parts, fmt.Sprintf("inspected=%t", status.InputInspected))
	parts = append(parts, fmt.Sprintf("has_perf_sidecar=%t", status.InputHasPerfSidecar))
	if status.InputInspectionError != "" {
		parts = append(parts, "inspection_error="+status.InputInspectionError)
	}
	return strings.Join(parts, " ")
}

func traceConvertTraceInputKindZh(kind string) string {
	switch strings.TrimSpace(kind) {
	case "trace_perf":
		return "trace+perf"
	case "trace_only_or_unknown":
		return "纯trace或未知"
	case "unreadable":
		return "不可读"
	case "inspection_error":
		return "检查失败"
	default:
		return kind
	}
}

func traceConvertTraceProviderLine(lang string, item hitraceconv.TraceToolProviderStatus) string {
	prefix := fmt.Sprintf("trace_provider[%s/%s]", item.Kind, item.Name)
	if traceConvertUseZh(lang) {
		return prefix + "：" + strings.Join(traceConvertTraceProviderDetailsZh(item), " ")
	}
	return prefix + ": " + strings.Join(traceConvertTraceProviderDetailsEn(item), " ")
}

func traceConvertTraceGateLine(lang string, gate hitraceconv.TraceToolGateStatus) string {
	if strings.TrimSpace(gate.Name) == "" {
		return ""
	}
	prefix := fmt.Sprintf("trace_gate[sys_binary_parity_gate/%s]", gate.Name)
	if traceConvertUseZh(lang) {
		return prefix + "：" + strings.Join(traceConvertTraceGateDetailsZh(gate), " ")
	}
	return prefix + ": " + strings.Join(traceConvertTraceGateDetailsEn(gate), " ")
}

func traceConvertTraceGateDetailsEn(gate hitraceconv.TraceToolGateStatus) []string {
	details := []string{
		"state=" + gate.State,
		fmt.Sprintf("proven=%t", gate.Proven),
		fmt.Sprintf("fixture_manifests=%d", gate.FixtureManifestCount),
	}
	if gate.RequiredEvidence != "" {
		details = append(details, "required="+gate.RequiredEvidence)
	}
	if len(gate.Evidence) > 0 {
		details = append(details, "evidence="+strings.Join(gate.Evidence, "; "))
	}
	if len(gate.Caveats) > 0 {
		details = append(details, "caveat="+strings.Join(gate.Caveats, "; "))
	}
	return details
}

func traceConvertTraceGateDetailsZh(gate hitraceconv.TraceToolGateStatus) []string {
	proven := "否"
	if gate.Proven {
		proven = "是"
	}
	details := []string{
		"状态=" + traceConvertTraceGateStateZh(gate.State),
		"已证明=" + proven,
		fmt.Sprintf("fixture清单=%d", gate.FixtureManifestCount),
	}
	if gate.RequiredEvidence != "" {
		details = append(details, "需要="+traceConvertTraceMessageZh(gate.RequiredEvidence))
	}
	if len(gate.Evidence) > 0 {
		details = append(details, "证据="+strings.Join(traceConvertTraceMessagesZh(gate.Evidence), "; "))
	}
	if len(gate.Caveats) > 0 {
		details = append(details, "注意="+strings.Join(traceConvertTraceMessagesZh(gate.Caveats), "; "))
	}
	return details
}

func traceConvertTraceGateStateZh(state string) string {
	switch strings.TrimSpace(state) {
	case "pending_representative_fixture":
		return "等待代表性fixture"
	case "representative_fixture_manifest_present":
		return "已发现代表性fixture清单"
	default:
		return state
	}
}

func traceConvertTraceProviderDetailsEn(item hitraceconv.TraceToolProviderStatus) []string {
	state := "missing"
	if item.Available {
		state = "available"
	}
	details := []string{fmt.Sprintf("state=%s", state)}
	if item.Path != "" {
		details = append(details, "path="+item.Path)
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

func traceConvertTraceProviderDetailsZh(item hitraceconv.TraceToolProviderStatus) []string {
	state := "缺失"
	if item.Available {
		state = "可用"
	}
	details := []string{fmt.Sprintf("状态=%s", state)}
	if item.Path != "" {
		details = append(details, "路径="+item.Path)
	}
	if item.Source != "" {
		details = append(details, "来源="+traceConvertTraceSourceZh(item.Source))
	}
	if item.Version != "" {
		details = append(details, "版本="+item.Version)
	}
	if item.CheckCommand != "" {
		details = append(details, "检查="+item.CheckCommand)
	}
	if len(item.AuxiliaryChecks) > 0 {
		details = append(details, "辅助检查="+strings.Join(traceConvertTraceMessagesZh(item.AuxiliaryChecks), "; "))
	}
	if item.InstallCommand != "" {
		details = append(details, "安装="+traceConvertTraceMessageZh(item.InstallCommand))
	}
	if item.DocsURL != "" {
		details = append(details, "文档="+item.DocsURL)
	}
	if len(item.Caveats) > 0 {
		details = append(details, "注意="+strings.Join(traceConvertTraceMessagesZh(item.Caveats), "; "))
	}
	if !item.Available && item.InstallHint != "" {
		details = append(details, "提示="+traceConvertTraceMessageZh(item.InstallHint))
	}
	return details
}

func traceConvertTraceMessagesZh(messages []string) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		out = append(out, traceConvertTraceMessageZh(msg))
	}
	return out
}

func traceConvertTraceSourceZh(source string) string {
	trimmed := strings.TrimSpace(source)
	lower := strings.ToLower(trimmed)
	switch {
	case trimmed == "":
		return ""
	case strings.EqualFold(trimmed, "built-in"):
		return "内置"
	case strings.Contains(lower, "configured trace_streamer"):
		return "已配置 trace_streamer"
	case strings.Contains(lower, "codrax_trace_streamer"):
		return "CODRAX_TRACE_STREAMER"
	case strings.Contains(lower, "on path"):
		return trimmed + "（从 PATH 发现）"
	case strings.Contains(lower, "known openharmony"):
		return "常见 OpenHarmony/SmartPerf/hmtrace 位置"
	default:
		return trimmed
	}
}

func traceConvertTraceMessageZh(message string) string {
	trimmed := strings.TrimSpace(message)
	lower := strings.ToLower(trimmed)
	switch {
	case trimmed == "":
		return ""
	case strings.Contains(lower, "trace_streamer db export is the preferred trace body path"), strings.Contains(lower, "trace_streamer db export is the required trace body path"):
		return "trace_streamer DB export 是 trace+perf htrace 的优先 trace body 路径，也可把纯 trace 转成 systrace，并将 coverage 写入 tracebundle 供 trace_query 使用"
	case strings.Contains(lower, "auto trace engine discovered trace_streamer"):
		return "auto trace 引擎已发现 trace_streamer；转换会优先走 SQL，SQL 执行或导出失败会回退到内置 raw trace 解析器"
	case strings.Contains(lower, "inspected input contains a standalone perf sidecar"):
		return "auto trace 引擎未发现 trace_streamer；已检查输入包含独立 perf sidecar，因此会使用内置 raw trace 解析和 standalone perf 兜底"
	case strings.Contains(lower, "auto trace engine did not discover trace_streamer"):
		return "auto trace 引擎未发现 trace_streamer；转换会使用内置 raw trace 解析器"
	case strings.Contains(lower, "trace_streamer is selected"):
		return "已显式选择 trace_streamer；转换只走 SQL，SQL 执行失败不会回退到内置解析器"
	case strings.Contains(lower, "trace_streamer was not discovered"):
		return "未发现 trace_streamer；当前显式选择的 SQL trace 转换无法生成 systrace"
	case strings.Contains(lower, "trace_streamer engine was selected but"):
		return "已选择 trace_streamer 引擎，但 trace_streamer 当前不可用"
	case strings.Contains(lower, "built-in modern/sys parser remains"):
		return "内置 modern/sys parser 是纯 trace 引擎；可显式选择，也可在 auto 未发现 trace_streamer 时用于纯 trace；不用于 trace+perf htrace"
	case strings.Contains(lower, "built-in modern/sys parser is selected explicitly"):
		return "内置 modern/sys parser 可通过 --trace-engine=builtin 显式选择，也会在 auto 下 trace_streamer 不可用或失败时作为 raw trace 兜底；显式 trace_streamer 不退化"
	case strings.Contains(lower, "commit a redistributable real no-perf harmony/donghu .sys fixture manifest"):
		return "提交可再分发的真实 no-perf Harmony/Donghu .sys fixture manifest 到 internal/hitraceconv/testdata/representative_sys_traces，并通过 TestRepresentativeSysTraceFixtures"
	case strings.Contains(lower, "synthetic scheduler/raw-ftrace parity guards are delivered"):
		return "synthetic scheduler/raw-ftrace parity 看护已交付"
	case strings.HasPrefix(lower, "representative_fixture_manifests="):
		return strings.Replace(trimmed, "representative_fixture_manifests", "代表性fixture清单", 1)
	case strings.Contains(lower, "no redistributable representative no-perf .sys fixture has been committed"):
		return "尚未提交可再分发的真实代表性 no-perf .sys fixture；内置 sys binary parser 仍保留为显式受控能力"
	case strings.Contains(lower, "trace+perf htrace in auto mode may fall back"):
		return "trace+perf htrace 在 auto 模式下 SQL 不可用或失败时可回退到内置 raw trace parser；显式 trace_streamer 模式永不回退"
	case strings.Contains(lower, "representative fixture manifest is present"):
		return "已发现代表性 fixture manifest；退休内置 sys binary parser 前必须先验证 TestRepresentativeSysTraceFixtures"
	case strings.Contains(lower, "representative fixture manifest directory could not be inspected"):
		return strings.Replace(trimmed, "representative fixture manifest directory could not be inspected", "代表性 fixture manifest 目录无法检查", 1)
	case strings.Contains(lower, "built-in modern parser"):
		return "内置 modern parser 是需要显式选择的纯 trace 引擎"
	case strings.Contains(lower, "built into codrax"):
		return "Codrax 内置；支持 modern profiler/session 文本载荷和 sys binary 转换；auto 下 trace_streamer 不可用或失败时可作为 raw trace 兜底"
	case strings.Contains(lower, "pass --trace-streamer"):
		return "传 --trace-streamer /path/to/trace_streamer 或设置 CODRAX_TRACE_STREAMER；确认可运行 trace_streamer --help，并支持 trace_streamer <input> -e <output.db>"
	case strings.Contains(lower, "install openharmony/smartperf trace_streamer"):
		return "安装 OpenHarmony/SmartPerf trace_streamer，或把对应平台的 trace_streamer 放在 Codrax 二进制同目录或 trace_streamer/<platform>/ 下"
	case strings.Contains(lower, "so_dirs=not_configured"):
		return "未配置 so_dir；native 符号 reload 需要时可传 --trace-streamer-so-dir /path/to/so"
	case strings.Contains(lower, "configured path is not readable"):
		return strings.Replace(trimmed, "configured path is not readable", "配置路径不可读", 1)
	case strings.Contains(lower, "configured path is a directory"):
		return "配置路径是目录，需要传 trace_streamer 可执行文件"
	case strings.Contains(lower, "configured path is not executable"):
		return "配置路径不可执行"
	default:
		return trimmed
	}
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
	case strings.Contains(lower, "codrax executable directory"):
		return "Codrax 可执行文件目录"
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
		return "安装或构建 OpenHarmony developtools_hiperf host 工具，然后传 --hiperf-host、设置 CODRAX_HIPERF_HOST，或把 hiperf_host/hiperf 放到 Codrax 可执行文件同目录/平台子目录；需要符号化时补 --hiperf-symbol-dir"
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
	case strings.Contains(lower, "configured path is not executable"):
		return "配置路径不可执行"
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
		lines = append(lines, traceConvertCoverageLines("zh", "trace_coverage", result.TraceCoverage)...)
		lines = append(lines, traceConvertCoverageLines("zh", "trace_db_coverage", result.TraceDBCoverage)...)
		lines = append(lines, traceConvertTraceProviderDecisionLines("zh", result.TraceDecisions)...)
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
	lines = append(lines, traceConvertCoverageLines("en", "trace_coverage", result.TraceCoverage)...)
	lines = append(lines, traceConvertCoverageLines("en", "trace_db_coverage", result.TraceDBCoverage)...)
	lines = append(lines, traceConvertTraceProviderDecisionLines("en", result.TraceDecisions)...)
	lines = append(lines, traceConvertProviderDecisionLines("en", result.ProviderDecisions)...)
	return lines
}

func traceConvertCoverageLines(lang, label string, coverage []hitraceconv.TraceDBCoverage) []string {
	if len(coverage) == 0 {
		return nil
	}
	emitted := 0
	skipped := 0
	for _, item := range coverage {
		emitted += item.RowsEmitted
		if strings.TrimSpace(item.Skipped) != "" || strings.TrimSpace(item.Error) != "" {
			skipped++
		}
	}
	var lines []string
	if traceConvertUseZh(lang) {
		lines = append(lines, fmt.Sprintf("%s：%d 项，输出=%d，跳过=%d", label, len(coverage), emitted, skipped))
	} else {
		lines = append(lines, fmt.Sprintf("%s: %d item(s), emitted=%d, skipped=%d", label, len(coverage), emitted, skipped))
	}
	limit := len(coverage)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		item := coverage[i]
		details := []string{
			traceConvertDetailKV(lang, "family", item.Family),
			traceConvertDetailKV(lang, "table", item.Table),
		}
		if item.Role != "" {
			details = append(details, traceConvertDetailKV(lang, "role", traceConvertCoverageRoleLabel(lang, item.Role)))
		}
		details = append(details,
			traceConvertDetailKV(lang, "rows_read", fmt.Sprintf("%d", item.RowsRead)),
			traceConvertDetailKV(lang, "rows_emitted", fmt.Sprintf("%d", item.RowsEmitted)),
		)
		if item.ElapsedUS > 0 {
			details = append(details, traceConvertDetailKV(lang, "elapsed_us", fmt.Sprintf("%d", item.ElapsedUS)))
		}
		if item.Skipped != "" {
			details = append(details, traceConvertDetailKV(lang, "skipped", item.Skipped))
		}
		if item.Error != "" {
			details = append(details, traceConvertDetailKV(lang, "error", item.Error))
		}
		if traceConvertUseZh(lang) {
			lines = append(lines, fmt.Sprintf("%s[%d]：%s", label, i, strings.Join(details, " ")))
		} else {
			lines = append(lines, fmt.Sprintf("%s[%d]: %s", label, i, strings.Join(details, " ")))
		}
	}
	return lines
}

func traceConvertArtifactLines(lang string, artifacts []hitraceconv.Artifact) []string {
	var lines []string
	for _, artifact := range artifacts {
		if artifact.Type == hitraceconv.ArtifactSystrace {
			continue
		}
		details := traceConvertArtifactDetails(lang, artifact)
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

func traceConvertArtifactDetails(lang string, artifact hitraceconv.Artifact) string {
	var details []string
	if format := traceConvertArtifactFormatDetail(lang, artifact.Type); format != "" {
		details = append(details, format)
	}
	if artifact.Bytes > 0 {
		details = append(details, traceConvertDetailKV(lang, "bytes", fmt.Sprintf("%d", artifact.Bytes)))
	}
	if artifact.Converter != "" {
		details = append(details, traceConvertDetailKV(lang, "converter", artifact.Converter))
	}
	if artifact.Perf != nil {
		details = append(details, traceConvertPerfCapabilityDetails(lang, *artifact.Perf)...)
	}
	if artifact.DataType != 0 {
		details = append(details, traceConvertDetailKV(lang, "data_type", fmt.Sprintf("%d", artifact.DataType)))
	}
	if artifact.PluginName != "" {
		details = append(details, traceConvertDetailKV(lang, "plugin", artifact.PluginName))
	}
	if artifact.PluginVersion != "" {
		details = append(details, traceConvertDetailKV(lang, "plugin_version", artifact.PluginVersion))
	}
	if artifact.SourceOffset > 0 {
		details = append(details, traceConvertDetailKV(lang, "source_offset", fmt.Sprintf("%d", artifact.SourceOffset)))
	}
	if artifact.SourceBytes > 0 {
		details = append(details, traceConvertDetailKV(lang, "source_bytes", fmt.Sprintf("%d", artifact.SourceBytes)))
	}
	if len(artifact.Caveats) > 0 {
		details = append(details, traceConvertDetailKV(lang, "caveats", strings.Join(traceConvertLocalizedMessages(lang, artifact.Caveats), "; ")))
	}
	return strings.Join(details, " ")
}

func traceConvertCoverageRoleLabel(lang, role string) string {
	role = strings.TrimSpace(role)
	if !traceConvertUseZh(lang) {
		return role
	}
	switch role {
	case "resolver_index":
		return "解析辅助索引，不直接输出 systrace 行"
	case "systrace_text_output":
		return "systrace 文本输出"
	case "perftrace_text_output":
		return "perftrace 文本输出"
	case "tracequery_cross_validation":
		return "trace_query 交叉验证"
	case "query_ready_export":
		return "query-ready 文本导出"
	case "unsupported_input":
		return "未支持输入"
	default:
		return role
	}
}

func traceConvertArtifactFormatDetail(lang, typ string) string {
	switch typ {
	case hitraceconv.ArtifactPerfData:
		if traceConvertUseZh(lang) {
			return "格式=二进制 perf.data sidecar"
		}
		return "format=binary_perf_data_sidecar"
	case hitraceconv.ArtifactPerfTrace:
		if traceConvertUseZh(lang) {
			return "格式=文本 perftrace"
		}
		return "format=text_perftrace"
	case hitraceconv.ArtifactTraceDB:
		if traceConvertUseZh(lang) {
			return "格式=trace_streamer SQLite DB"
		}
		return "format=trace_streamer_sqlite_db"
	case hitraceconv.ArtifactTraceBundle:
		if traceConvertUseZh(lang) {
			return "格式=tracebundle 元数据"
		}
		return "format=tracebundle_metadata"
	default:
		return ""
	}
}

func traceConvertPerfCapabilityDetails(lang string, cap hitraceconv.PerfArtifactCapability) []string {
	var details []string
	if cap.ProviderName != "" {
		details = append(details, traceConvertDetailKV(lang, "perf_provider", cap.ProviderName))
	}
	if cap.ProviderKind != "" {
		details = append(details, traceConvertDetailKV(lang, "perf_provider_kind", cap.ProviderKind))
	}
	if cap.InputFormat != "" {
		details = append(details, traceConvertDetailKV(lang, "perf_input", cap.InputFormat))
	}
	if cap.Symbolization != "" {
		details = append(details, traceConvertDetailKV(lang, "perf_symbolization", cap.Symbolization))
	}
	if cap.CPUIdentity != "" {
		details = append(details, traceConvertDetailKV(lang, "perf_cpu", cap.CPUIdentity))
	}
	if cap.Callchain != "" {
		details = append(details, traceConvertDetailKV(lang, "perf_callchain", cap.Callchain))
	}
	if cap.TimeAlignment != "" {
		details = append(details, traceConvertDetailKV(lang, "perf_time_alignment", cap.TimeAlignment))
	}
	if cap.TraceQueryReady {
		details = append(details, traceConvertDetailKV(lang, "trace_query_ready", traceConvertBoolValue(lang, true)))
	}
	if cap.Degraded {
		details = append(details, traceConvertDetailKV(lang, "perf_degraded", traceConvertBoolValue(lang, true)))
	}
	return details
}

func traceConvertTraceProviderDecisionLines(lang string, decisions []hitraceconv.TraceProviderDecision) []string {
	var lines []string
	for _, decision := range decisions {
		details := traceConvertTraceProviderDecisionDetails(lang, decision)
		prefix := fmt.Sprintf("trace_provider_decision[%s/%s]", decision.ProviderKind, decision.ProviderName)
		if traceConvertUseZh(lang) {
			lines = append(lines, prefix+"："+strings.Join(details, " "))
		} else {
			lines = append(lines, prefix+": "+strings.Join(details, " "))
		}
	}
	return lines
}

func traceConvertTraceProviderDecisionDetails(lang string, decision hitraceconv.TraceProviderDecision) []string {
	details := []string{
		traceConvertDetailKV(lang, "selected", traceConvertBoolValue(lang, decision.Selected)),
		traceConvertDetailKV(lang, "attempted", traceConvertBoolValue(lang, decision.Attempted)),
		traceConvertDetailKV(lang, "succeeded", traceConvertBoolValue(lang, decision.Succeeded)),
		traceConvertDetailKV(lang, "fallback", traceConvertBoolValue(lang, decision.Fallback)),
		traceConvertDetailKV(lang, "trace_query_ready", traceConvertBoolValue(lang, decision.TraceQueryReady)),
	}
	if decision.Stage != "" {
		details = append(details, traceConvertDetailKV(lang, "stage", decision.Stage))
	}
	if decision.EngineMode != "" {
		details = append(details, traceConvertDetailKV(lang, "engine", decision.EngineMode))
	}
	if decision.OutputPath != "" {
		details = append(details, traceConvertDetailKV(lang, "output", decision.OutputPath))
	}
	if decision.DBPath != "" {
		details = append(details, traceConvertDetailKV(lang, "db", decision.DBPath))
	}
	if decision.ArtifactPath != "" {
		details = append(details, traceConvertDetailKV(lang, "artifact", decision.ArtifactPath))
	}
	if decision.Reason != "" {
		details = append(details, traceConvertDetailKV(lang, "reason", decision.Reason))
	}
	if decision.Caveat != "" {
		details = append(details, traceConvertDetailKV(lang, "caveat", hitraceconv.LocalizeConvertMessage(lang, decision.Caveat)))
	}
	return details
}

func traceConvertProviderDecisionLines(lang string, decisions []hitraceconv.PerfProviderDecision) []string {
	var lines []string
	for _, decision := range decisions {
		details := traceConvertProviderDecisionDetails(lang, decision)
		prefix := fmt.Sprintf("provider_decision[%s/%s]", decision.ProviderKind, decision.ProviderName)
		if traceConvertUseZh(lang) {
			lines = append(lines, prefix+"："+strings.Join(details, " "))
		} else {
			lines = append(lines, prefix+": "+strings.Join(details, " "))
		}
	}
	return lines
}

func traceConvertProviderDecisionDetails(lang string, decision hitraceconv.PerfProviderDecision) []string {
	details := []string{
		traceConvertDetailKV(lang, "selected", traceConvertBoolValue(lang, decision.Selected)),
		traceConvertDetailKV(lang, "attempted", traceConvertBoolValue(lang, decision.Attempted)),
		traceConvertDetailKV(lang, "succeeded", traceConvertBoolValue(lang, decision.Succeeded)),
		traceConvertDetailKV(lang, "fallback", traceConvertBoolValue(lang, decision.Fallback)),
		traceConvertDetailKV(lang, "trace_query_ready", traceConvertBoolValue(lang, decision.TraceQueryReady)),
	}
	if decision.Stage != "" {
		details = append(details, traceConvertDetailKV(lang, "stage", decision.Stage))
	}
	if decision.ParserMode != "" {
		details = append(details, traceConvertDetailKV(lang, "parser", decision.ParserMode))
	}
	if decision.InputFormat != "" {
		details = append(details, traceConvertDetailKV(lang, "input", decision.InputFormat))
	}
	if decision.OutputPath != "" {
		details = append(details, traceConvertDetailKV(lang, "output", decision.OutputPath))
	}
	if decision.ArtifactPath != "" {
		details = append(details, traceConvertDetailKV(lang, "artifact", decision.ArtifactPath))
	}
	if decision.Reason != "" {
		details = append(details, traceConvertDetailKV(lang, "reason", decision.Reason))
	}
	if decision.Caveat != "" {
		details = append(details, traceConvertDetailKV(lang, "caveat", hitraceconv.LocalizeConvertMessage(lang, decision.Caveat)))
	}
	return details
}

func traceConvertCaveatLine(lang, caveat string) string {
	if traceConvertUseZh(lang) {
		return fmt.Sprintf("提示：%s", hitraceconv.LocalizeConvertMessage(lang, caveat))
	}
	return fmt.Sprintf("caveat: %s", caveat)
}

func traceConvertLocalizedMessages(lang string, messages []string) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		out = append(out, hitraceconv.LocalizeConvertMessage(lang, msg))
	}
	return out
}

func traceConvertDetailKV(lang, key, value string) string {
	if traceConvertUseZh(lang) {
		key = traceConvertDetailKeyZh(key)
	}
	return key + "=" + value
}

func traceConvertBoolValue(lang string, value bool) string {
	if !traceConvertUseZh(lang) {
		return fmt.Sprintf("%t", value)
	}
	if value {
		return "是"
	}
	return "否"
}

func traceConvertDetailKeyZh(key string) string {
	switch key {
	case "bytes":
		return "字节"
	case "converter":
		return "转换器"
	case "data_type":
		return "数据类型"
	case "plugin":
		return "插件"
	case "plugin_version":
		return "插件版本"
	case "source_offset":
		return "源偏移"
	case "source_bytes":
		return "源字节"
	case "caveats", "caveat":
		return "提示"
	case "perf_provider":
		return "perf提供方"
	case "perf_provider_kind":
		return "perf提供方类型"
	case "perf_input":
		return "perf输入"
	case "perf_symbolization":
		return "perf符号化"
	case "perf_cpu":
		return "perfCPU信息"
	case "perf_callchain":
		return "perf调用栈"
	case "perf_time_alignment":
		return "perf时间对齐"
	case "trace_query_ready":
		return "可供trace_query消费"
	case "perf_degraded":
		return "perf降级"
	case "selected":
		return "已选择"
	case "attempted":
		return "已尝试"
	case "succeeded":
		return "已成功"
	case "fallback":
		return "回退路径"
	case "stage":
		return "阶段"
	case "parser":
		return "解析模式"
	case "engine":
		return "引擎"
	case "input":
		return "输入"
	case "output":
		return "输出"
	case "db":
		return "DB"
	case "artifact":
		return "artifact"
	case "reason":
		return "原因"
	case "family":
		return "族"
	case "table":
		return "表"
	case "role":
		return "用途"
	case "rows_read":
		return "读取行"
	case "rows_emitted":
		return "输出行"
	case "elapsed_us":
		return "耗时us"
	case "skipped":
		return "跳过原因"
	case "error":
		return "错误"
	default:
		return key
	}
}

func traceConvertNextLine(lang string, result hitraceconv.Result) string {
	if result.BundlePath != "" {
		if traceConvertUseZh(lang) {
			return fmt.Sprintf("下一步：codrax --htrace %q --request <问题>；优先附加 tracebundle 保留转换/coverage/clock provenance；也可直接附加 systrace 做核心事件查询", result.BundlePath)
		}
		return fmt.Sprintf("next: codrax --htrace %q --request <question>; prefer the tracebundle to keep conversion/coverage/clock provenance; attaching systrace directly is also enough for core event queries", result.BundlePath)
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

func traceConvertProgressLine(lang string, event hitraceconv.ProgressEvent) string {
	if traceConvertUseZh(lang) {
		return "进度：" + strings.Join(traceConvertProgressPartsZh(event), " ")
	}
	return "progress: " + strings.Join(traceConvertProgressPartsEn(event), " ")
}

func traceConvertProgressPartsEn(event hitraceconv.ProgressEvent) []string {
	parts := []string{
		"stage=" + traceConvertProgressStageEn(event.Stage),
		"status=" + traceConvertProgressStatusEn(event.Status),
	}
	if strings.TrimSpace(event.Message) != "" {
		parts = append(parts, "message="+event.Message)
	}
	if strings.TrimSpace(event.Path) != "" {
		parts = append(parts, "path="+event.Path)
	}
	if strings.TrimSpace(event.OutputPath) != "" {
		parts = append(parts, "output="+event.OutputPath)
	}
	if event.BytesTotal > 0 {
		parts = append(parts, fmt.Sprintf("bytes=%d/%d", event.BytesDone, event.BytesTotal))
	} else if event.BytesDone > 0 {
		parts = append(parts, fmt.Sprintf("bytes=%d", event.BytesDone))
	}
	if event.Records > 0 {
		parts = append(parts, fmt.Sprintf("records=%d", event.Records))
	}
	if event.Elapsed > 0 {
		parts = append(parts, "elapsed="+event.Elapsed.Round(100*time.Millisecond).String())
	}
	return parts
}

func traceConvertProgressPartsZh(event hitraceconv.ProgressEvent) []string {
	parts := []string{
		"阶段=" + traceConvertProgressStageZh(event.Stage),
		"状态=" + traceConvertProgressStatusZh(event.Status),
	}
	if strings.TrimSpace(event.Message) != "" {
		parts = append(parts, "说明="+traceConvertProgressMessageZh(event.Message))
	}
	if strings.TrimSpace(event.Path) != "" {
		parts = append(parts, "路径="+event.Path)
	}
	if strings.TrimSpace(event.OutputPath) != "" {
		parts = append(parts, "输出="+event.OutputPath)
	}
	if event.BytesTotal > 0 {
		parts = append(parts, fmt.Sprintf("字节=%d/%d", event.BytesDone, event.BytesTotal))
	} else if event.BytesDone > 0 {
		parts = append(parts, fmt.Sprintf("字节=%d", event.BytesDone))
	}
	if event.Records > 0 {
		parts = append(parts, fmt.Sprintf("记录=%d", event.Records))
	}
	if event.Elapsed > 0 {
		parts = append(parts, "耗时="+event.Elapsed.Round(100*time.Millisecond).String())
	}
	return parts
}

func traceConvertProgressStageEn(stage string) string {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return "unknown"
	}
	return stage
}

func traceConvertProgressStageZh(stage string) string {
	switch strings.TrimSpace(stage) {
	case "trace_streamer_export":
		return "trace_streamer导出DB"
	case "trace_db_normalize":
		return "SQLite DB转systrace"
	case "raw_perf_parse":
		return "raw perf.data解析"
	case "perftrace_write":
		return "写入perftrace"
	case "simpleperf_adapter":
		return "simpleperf官方适配器"
	case "hiperf_adapter":
		return "hiperf官方适配器"
	default:
		return traceConvertProgressStageEn(stage)
	}
}

func traceConvertProgressStatusEn(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return hitraceconv.ProgressStatusProgress
	}
	return status
}

func traceConvertProgressStatusZh(status string) string {
	switch strings.TrimSpace(status) {
	case hitraceconv.ProgressStatusStarted:
		return "开始"
	case hitraceconv.ProgressStatusProgress:
		return "进行中"
	case hitraceconv.ProgressStatusComplete:
		return "完成"
	case hitraceconv.ProgressStatusFailed:
		return "失败"
	default:
		return traceConvertProgressStatusEn(status)
	}
}

func traceConvertProgressMessageZh(message string) string {
	switch strings.TrimSpace(message) {
	case "running trace_streamer SQLite DB export":
		return "正在运行 trace_streamer 导出 SQLite DB"
	case "completed trace_streamer SQLite DB export":
		return "已完成 trace_streamer 导出 SQLite DB"
	case "trace_streamer SQLite DB export failed":
		return "trace_streamer 导出 SQLite DB 失败"
	case "normalizing trace_streamer SQLite DB to systrace":
		return "正在把 trace_streamer SQLite DB 转成文本 systrace"
	case "normalized trace_streamer SQLite DB to systrace":
		return "已把 trace_streamer SQLite DB 转成文本 systrace"
	case "trace_streamer SQLite DB normalization failed":
		return "trace_streamer SQLite DB 转 systrace 失败"
	case "trace_streamer SQLite DB normalization produced no systrace rows":
		return "trace_streamer SQLite DB 没有导出可用 systrace 行"
	case "running official simpleperf adapter":
		return "正在运行官方 simpleperf 适配器"
	case "completed official simpleperf adapter":
		return "已完成官方 simpleperf 适配器"
	case "official simpleperf adapter failed":
		return "官方 simpleperf 适配器失败"
	case "running official hiperf adapter":
		return "正在运行官方 hiperf 适配器"
	case "completed official hiperf adapter":
		return "已完成官方 hiperf 适配器"
	case "official hiperf adapter failed":
		return "官方 hiperf 适配器失败"
	case "parsing raw perf.data records":
		return "正在解析 raw perf.data 记录"
	case "parsed raw perf.data records":
		return "已解析 raw perf.data 记录"
	case "raw perf.data parse failed":
		return "raw perf.data 解析失败"
	case "raw perf.data contains no supported sample records":
		return "raw perf.data 没有可支持的 sample 记录"
	case "writing normalized perftrace text":
		return "正在写入标准化 perftrace 文本"
	case "wrote normalized perftrace text":
		return "已写入标准化 perftrace 文本"
	case "perftrace text write failed":
		return "perftrace 文本写入失败"
	case "perftrace text flush failed":
		return "perftrace 文本刷新失败"
	case "perftrace text close failed":
		return "perftrace 文本关闭失败"
	case "external command failed to start":
		return "外部命令启动失败"
	default:
		return message
	}
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
	traceConvertCmd.Flags().StringVar(&traceConvertTraceEngine, "trace-engine", "auto", "trace body conversion engine: auto, trace_streamer, or builtin; auto uses trace_streamer SQL first and falls back to built-in raw parsing when SQL is unavailable or fails")
	traceConvertCmd.Flags().StringVar(&traceConvertTraceStreamer, "trace-streamer", "", "OpenHarmony/SmartPerf trace_streamer executable used to export .htrace/perf.data into SQLite DB")
	traceConvertCmd.Flags().StringVar(&traceConvertTraceDBOutput, "trace-db-output", "", "trace_streamer SQLite DB output path; default is derived from --output or input when DB export is enabled")
	traceConvertCmd.Flags().BoolVar(&traceConvertKeepTraceDB, "keep-trace-db", false, "debug only: keep generated trace_streamer SQLite DB and tool sidecars instead of cleaning temporary files")
	traceConvertCmd.Flags().StringSliceVar(&traceConvertTraceStreamerSoDirs, "trace-streamer-so-dir", nil, "native .so directories passed through to trace_streamer for symbol reload; repeat or comma-separate values")
	traceConvertCmd.Flags().StringVar(&traceConvertPerfParser, "perf-parser", "auto", "perf.data parser strategy: auto uses official hiperf/simpleperf first then raw fallback; official disables raw fallback; raw uses Codrax raw perf.data fallback only")
	traceConvertCmd.Flags().BoolVar(&traceConvertNoPerfTrace, "no-perftrace", false, "preserve perf.data sidecars without generating .perftrace")
	traceConvertCmd.Flags().BoolVar(&traceConvertTraceToolsStatus, "trace-tools-status", false, "print trace_streamer discovery, trace engine selection, DB export readiness, and install hints")
	traceConvertCmd.Flags().BoolVar(&traceConvertToolsStatus, "perf-tools-status", false, "print trace_streamer/trace-engine status plus discovered official perf tools, raw fallback availability, selected parser strategy, and install hints")
	traceCmd.AddCommand(traceConvertCmd)
	rootCmd.AddCommand(traceCmd)
}
