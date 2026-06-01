package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

var (
	traceConvertInput  string
	traceConvertOutput string
	traceConvertFlavor string
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
		if input == "" {
			return fmt.Errorf("--input is required")
		}
		result, err := hitraceconv.ConvertFile(cmd.Context(), hitraceconv.Options{
			InputPath:  input,
			OutputPath: strings.TrimSpace(traceConvertOutput),
			Flavor:     strings.TrimSpace(traceConvertFlavor),
		})
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
		fmt.Fprintln(cmd.OutOrStdout(), traceConvertNextLine(flagLang, result.OutputPath))
		return nil
	},
}

func traceConvertResultLines(lang string, result hitraceconv.Result) []string {
	if traceConvertUseZh(lang) {
		return []string{
			fmt.Sprintf("已转换二进制 hitrace：%s", result.InputPath),
			fmt.Sprintf("输出：%s", result.OutputPath),
			fmt.Sprintf("事件：%d，缺失格式：%d，未知事件：%d", result.EventsWritten, result.MissingFormatCount, result.UnknownEventCount),
		}
	}
	return []string{
		fmt.Sprintf("converted binary hitrace: %s", result.InputPath),
		fmt.Sprintf("output: %s", result.OutputPath),
		fmt.Sprintf("events: %d, missing_formats: %d, unknown_events: %d", result.EventsWritten, result.MissingFormatCount, result.UnknownEventCount),
	}
}

func traceConvertCaveatLine(lang, caveat string) string {
	if traceConvertUseZh(lang) {
		return fmt.Sprintf("提示：%s", caveat)
	}
	return fmt.Sprintf("caveat: %s", caveat)
}

func traceConvertNextLine(lang, outputPath string) string {
	if traceConvertUseZh(lang) {
		return fmt.Sprintf("下一步：codrax --htrace %q --request <问题>", outputPath)
	}
	return fmt.Sprintf("next: codrax --htrace %q --request <question>", outputPath)
}

func traceConvertUseZh(lang string) bool {
	return !strings.EqualFold(strings.TrimSpace(lang), "en")
}

func init() {
	traceConvertCmd.Flags().StringVar(&traceConvertInput, "input", "", "binary Harmony/OpenHarmony HiTrace input path")
	traceConvertCmd.Flags().StringVar(&traceConvertOutput, "output", "", "text systrace output path; default is <input>.systrace")
	traceConvertCmd.Flags().StringVar(&traceConvertFlavor, "flavor", "harmony_hitrace", "trace flavor metadata for operator audit; default harmony_hitrace")
	traceCmd.AddCommand(traceConvertCmd)
	rootCmd.AddCommand(traceCmd)
}
