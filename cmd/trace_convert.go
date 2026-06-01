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
		fmt.Fprintf(cmd.OutOrStdout(),
			"converted binary hitrace: %s\noutput: %s\nevents: %d, missing_formats: %d, generic_rows: %d\n",
			result.InputPath, result.OutputPath, result.EventsWritten, result.MissingFormatCount, result.UnknownEventCount)
		if len(result.Caveats) > 0 {
			for _, caveat := range result.Caveats {
				fmt.Fprintf(cmd.OutOrStdout(), "caveat: %s\n", caveat)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "next: codrax --htrace %q --request <question>\n", result.OutputPath)
		return nil
	},
}

func init() {
	traceConvertCmd.Flags().StringVar(&traceConvertInput, "input", "", "binary Harmony/OpenHarmony HiTrace input path")
	traceConvertCmd.Flags().StringVar(&traceConvertOutput, "output", "", "text systrace output path; default is <input>.systrace")
	traceConvertCmd.Flags().StringVar(&traceConvertFlavor, "flavor", "harmony_hitrace", "trace flavor metadata for operator audit; default harmony_hitrace")
	traceCmd.AddCommand(traceConvertCmd)
	rootCmd.AddCommand(traceCmd)
}
