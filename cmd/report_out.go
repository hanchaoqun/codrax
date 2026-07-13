package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/outputdump"
)

// OUT-1 (§29.55.1) — explicit final-answer report outputs.
//
// --report-md / --report-html each request ONE extra copy of the run's
// final-answer report at an exact user-chosen path (directory + file
// name, used verbatim — no timestamp). The default .codrax/output dump
// is not affected in either direction:
//
//   - dump enabled  → .codrax/output keeps writing exactly as before,
//     PLUS the explicit copies (markdown byte-identical, HTML from the
//     same RenderStandaloneMarkdownHTML pipeline).
//   - dump disabled (output_dump_enabled: false) → .codrax/output stays
//     untouched, but the explicit copies still write: an explicit CLI
//     path is explicit user intent and overrides the default gate.
//
// The flags are single-shot only (per-invocation paths make no sense as
// a REPL session default); a REPL launch with either flag set fails loud
// before entering the loop. Naming follows the `--plan-out` precedent of
// artifact-scoped output-path flags; `--out` itself is already taken by
// the tracediag lane.
var (
	flagReportMD   string
	flagReportHTML string
)

func init() {
	f := rootCmd.PersistentFlags()
	f.StringVar(&flagReportMD, "report-md", "", "single-shot: additionally write the final-answer report markdown to this exact file path (byte-identical to the default .codrax/output dump; parent dirs created; existing file overwritten). The default .codrax/output dump is unaffected, and the copy is written even when output_dump_enabled=false.")
	f.StringVar(&flagReportHTML, "report-html", "", "single-shot: additionally write the self-contained HTML rendering of the final-answer report to this exact file path (same renderer as the default .codrax/output .html sibling; parent dirs created; existing file overwritten). The default .codrax/output dump is unaffected, and the copy is written even when output_dump_enabled=false.")
}

// explicitReportRegistration is the resolved plan for arming the explicit
// report outputs. Kept as a pure value so resolveExplicitReportOutputs is
// unit-testable without touching package/app state.
type explicitReportRegistration struct {
	active bool
	report outputdump.ExplicitReport
	// forceDumpDir is non-empty when output_dump_enabled=false: the
	// orchestrator gates its dump hook on a non-empty dir, so the hook
	// must be armed with a placeholder for WriteResult to run at all.
	// report.SuppressDefaultDir guarantees the placeholder dir is never
	// written to.
	forceDumpDir string
}

// resolveExplicitReportOutputs validates and resolves the --report-md /
// --report-html flags. request must be the resolved single-shot request
// (empty = REPL launch, which is rejected). outputDumpDir is the resolved
// default dump dir ("" ⇔ output_dump_enabled=false).
func resolveExplicitReportOutputs(request, reportMD, reportHTML, outputDumpDir string) (explicitReportRegistration, error) {
	md := strings.TrimSpace(reportMD)
	html := strings.TrimSpace(reportHTML)
	if md == "" && html == "" {
		return explicitReportRegistration{}, nil
	}
	if strings.TrimSpace(request) == "" {
		return explicitReportRegistration{}, fmt.Errorf("--report-md/--report-html require a single-shot request (--request or a positional argument); they do not apply to REPL mode")
	}
	var err error
	if md != "" {
		if md, err = filepath.Abs(md); err != nil {
			return explicitReportRegistration{}, fmt.Errorf("--report-md %q: %w", reportMD, err)
		}
	}
	if html != "" {
		if html, err = filepath.Abs(html); err != nil {
			return explicitReportRegistration{}, fmt.Errorf("--report-html %q: %w", reportHTML, err)
		}
	}
	if md != "" && md == html {
		return explicitReportRegistration{}, fmt.Errorf("--report-md and --report-html must be different paths (both resolve to %s)", md)
	}
	reg := explicitReportRegistration{
		active: true,
		report: outputdump.ExplicitReport{
			MarkdownPath:       md,
			HTMLPath:           html,
			SuppressDefaultDir: outputDumpDir == "",
		},
	}
	if outputDumpDir == "" {
		dir := md
		if dir == "" {
			dir = html
		}
		reg.forceDumpDir = filepath.Dir(dir)
	}
	return reg, nil
}

// explicitReportFlagConflicts reports which of the explicit report flags
// were passed (value-based, same shape as traceDiagConflictingFlags).
// Consumed by the standalone lanes that never produce a final-answer
// transcript (--tracediag, --write-audit): the requested report file would
// silently never appear there, so a deterministic CLI refuses the
// ambiguity instead of ignoring the flag.
func explicitReportFlagConflicts() []string {
	conflicts := []string{}
	if strings.TrimSpace(flagReportMD) != "" {
		conflicts = append(conflicts, "--report-md")
	}
	if strings.TrimSpace(flagReportHTML) != "" {
		conflicts = append(conflicts, "--report-html")
	}
	return conflicts
}

// outputDumpArmer is the single orchestrator method the explicit-report
// wiring needs. Narrowed to an interface so tests can pin the arming with
// a recording stub (*orchestrator.Orchestrator satisfies it).
type outputDumpArmer interface {
	SetOutputDump(dir string, max int)
}

// armExplicitReportOutputs applies an active registration: installs the
// outputdump sink and, when the default dump is disabled
// (reg.forceDumpDir non-empty), arms the orchestrator dump hook with the
// placeholder dir (never written to — see
// explicitReportRegistration.forceDumpDir). Split from
// configureExplicitReportOutputs so the wiring itself is pinned by tests.
func armExplicitReportOutputs(reg explicitReportRegistration, orch outputDumpArmer, outputDumpMax int, setSink func(outputdump.ExplicitReport)) {
	if !reg.active {
		return
	}
	setSink(reg.report)
	if reg.forceDumpDir != "" && orch != nil {
		orch.SetOutputDump(reg.forceDumpDir, outputDumpMax)
	}
}

// configureExplicitReportOutputs resolves the CLI flags and applies the
// registration against the live app state.
func configureExplicitReportOutputs(request string) error {
	reg, err := resolveExplicitReportOutputs(request, flagReportMD, flagReportHTML, app.outputDumpDir)
	if err != nil || !reg.active {
		return err
	}
	logging.Info("[cmd] explicit report outputs armed: md=%q html=%q suppress_default_dir=%t",
		reg.report.MarkdownPath, reg.report.HTMLPath, reg.report.SuppressDefaultDir)
	var orch outputDumpArmer
	if app.orch != nil {
		orch = app.orch
	}
	armExplicitReportOutputs(reg, orch, app.outputDumpMax, outputdump.SetExplicitReport)
	return nil
}

// printExplicitReportStatus reports the fate of --report-md/--report-html
// after a single-shot run, mirroring the --plan-out status precedent:
// success prints "[report written: <path>]" to stderr, failure prints a
// stderr line plus a WARN log. Neither changes the exit code — the answer
// already reached the terminal and the explicit copies follow the same
// best-effort policy as the default dump.
func printExplicitReportStatus() {
	if strings.TrimSpace(flagReportMD) == "" && strings.TrimSpace(flagReportHTML) == "" {
		return
	}
	writes := outputdump.ExplicitReportWrites()
	if len(writes) == 0 {
		const msg = "report not written: this run produced no final-answer transcript (write/data/operation lanes and empty answers do not dump)"
		logging.Warning("[cmd] %s", msg)
		fmt.Fprintf(os.Stderr, "%s\n", msg)
		return
	}
	for _, w := range writes {
		if w.Err != nil {
			logging.Warning("[cmd] report %s write failed: %s: %v", w.Kind, w.Path, w.Err)
			fmt.Fprintf(os.Stderr, "report %s write failed: %s: %v\n", w.Kind, w.Path, w.Err)
			continue
		}
		logging.Info("[cmd] report written: %s", w.Path)
		fmt.Fprintf(os.Stderr, "\n[report written: %s]\n", w.Path)
	}
}
