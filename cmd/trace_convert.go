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
ftrace/systrace-compatible text file that Codrax can later analyze with
--htrace, /htrace, grep/read_file, and trace_query.

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
			fmt.Sprintf("符号化预期：%s", status.SymbolizationExpectation),
		}
		lines = append(lines, traceConvertPerfProviderLine("zh", status.Hiperf))
		lines = append(lines, traceConvertPerfProviderLine("zh", status.Simpleperf))
		lines = append(lines, traceConvertPerfProviderLine("zh", status.RawFallback))
		for _, caveat := range status.Caveats {
			lines = append(lines, "提示："+caveat)
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
	if len(item.Caveats) > 0 {
		details = append(details, "caveat="+strings.Join(item.Caveats, "; "))
	}
	if !item.Available && item.InstallHint != "" {
		details = append(details, "hint="+item.InstallHint)
	}
	prefix := fmt.Sprintf("%s[%s]", item.Kind, item.Name)
	if traceConvertUseZh(lang) {
		return prefix + "：" + strings.Join(details, " ")
	}
	return prefix + ": " + strings.Join(details, " ")
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
	return lines
}

func traceConvertArtifactLines(lang string, artifacts []hitraceconv.Artifact) []string {
	var lines []string
	for _, artifact := range artifacts {
		if artifact.Type == hitraceconv.ArtifactSystrace {
			continue
		}
		if traceConvertUseZh(lang) {
			lines = append(lines, fmt.Sprintf("artifact[%s]：%s", artifact.Type, artifact.Path))
		} else {
			lines = append(lines, fmt.Sprintf("artifact[%s]: %s", artifact.Type, artifact.Path))
		}
	}
	return lines
}

func traceConvertCaveatLine(lang, caveat string) string {
	if traceConvertUseZh(lang) {
		return fmt.Sprintf("提示：%s", caveat)
	}
	return fmt.Sprintf("caveat: %s", caveat)
}

func traceConvertNextLine(lang string, result hitraceconv.Result) string {
	if result.OutputPath == "" {
		if result.BundlePath != "" {
			if traceConvertUseZh(lang) {
				return fmt.Sprintf("下一步：查看 trace bundle %q；若已有 perftrace artifact，可直接作为 trace_query 的 CPU sample 输入；否则用 --hiperf-host/--simpleperf-report-sample 指定官方工具，或用 --perf-parser=raw 走 raw perf.data fallback 重跑转换", result.BundlePath)
			}
			return fmt.Sprintf("next: inspect trace bundle %q; if it contains a perftrace artifact, feed it to trace_query for CPU-sample analysis; otherwise rerun with --hiperf-host/--simpleperf-report-sample or --perf-parser=raw", result.BundlePath)
		}
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
