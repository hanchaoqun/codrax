package cmd

import (
	"errors"
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
	flagReportMD      string
	flagReportHTML    string
	flagRootCausesOut string
)

func init() {
	f := rootCmd.PersistentFlags()
	f.StringVar(&flagReportMD, "report-md", "", "single-shot: additionally write the final-answer report markdown to this exact file path (byte-identical to the default .codrax/output dump; parent dirs created; existing file overwritten). The default .codrax/output dump is unaffected, and the copy is written even when output_dump_enabled=false.")
	f.StringVar(&flagReportHTML, "report-html", "", "single-shot: additionally write the self-contained HTML rendering of the final-answer report to this exact file path (same renderer as the default .codrax/output .html sibling; parent dirs created; existing file overwritten). The default .codrax/output dump is unaffected, and the copy is written even when output_dump_enabled=false.")
	f.StringVar(&flagRootCausesOut, "root-causes-out", "", "single-shot: write a guaranteed-delivery structured Trace root-cause artifact to this exact file path (typed available/unavailable status; no root cause is inferred when model selection is unavailable; parent dirs created; existing file overwritten). This path is attempted even when output_dump_enabled=false; a write failure makes the command fail.")
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
func resolveExplicitReportOutputs(request, reportMD, reportHTML, rootCausesOut, outputDumpDir string) (explicitReportRegistration, error) {
	md := strings.TrimSpace(reportMD)
	html := strings.TrimSpace(reportHTML)
	rootCauses := strings.TrimSpace(rootCausesOut)
	if md == "" && html == "" && rootCauses == "" {
		return explicitReportRegistration{}, nil
	}
	if strings.TrimSpace(request) == "" {
		return explicitReportRegistration{}, fmt.Errorf("--report-md/--report-html/--root-causes-out require a single-shot request (--request or a positional argument); they do not apply to REPL mode")
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
	if rootCauses != "" {
		if rootCauses, err = filepath.Abs(rootCauses); err != nil {
			return explicitReportRegistration{}, fmt.Errorf("--root-causes-out %q: %w", rootCausesOut, err)
		}
	}
	resolved := []struct {
		name string
		path string
	}{{"--report-md", md}, {"--report-html", html}, {"--root-causes-out", rootCauses}}
	for i := range resolved {
		if resolved[i].path == "" {
			continue
		}
		for j := i + 1; j < len(resolved); j++ {
			if resolved[i].path == resolved[j].path {
				return explicitReportRegistration{}, fmt.Errorf("%s and %s must be different paths (both resolve to %s)", resolved[i].name, resolved[j].name, resolved[i].path)
			}
		}
	}
	reg := explicitReportRegistration{
		active: true,
		report: outputdump.ExplicitReport{
			MarkdownPath:       md,
			HTMLPath:           html,
			RootCauseJSONPath:  rootCauses,
			SuppressDefaultDir: outputDumpDir == "",
		},
	}
	if outputDumpDir == "" {
		dir := md
		if dir == "" {
			dir = html
		}
		if dir == "" {
			dir = rootCauses
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
	if strings.TrimSpace(flagRootCausesOut) != "" {
		conflicts = append(conflicts, "--root-causes-out")
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
	reg, err := resolveExplicitReportOutputs(request, flagReportMD, flagReportHTML, flagRootCausesOut, app.outputDumpDir)
	if err != nil || !reg.active {
		return err
	}
	logging.Info("[cmd] explicit report outputs armed: md=%q html=%q root_causes=%q suppress_default_dir=%t",
		reg.report.MarkdownPath, reg.report.HTMLPath, reg.report.RootCauseJSONPath, reg.report.SuppressDefaultDir)
	var orch outputDumpArmer
	if app.orch != nil {
		orch = app.orch
	}
	armExplicitReportOutputs(reg, orch, app.outputDumpMax, outputdump.SetExplicitReport)
	return nil
}

// printExplicitReportStatus reports the fate of all explicit output flags
// after a single-shot run, mirroring the --plan-out status precedent:
// success prints "[report written: <path>]" to stderr, failure prints a
// stderr line plus a WARN log. Markdown/HTML remain best-effort presentation
// copies. A --root-causes-out write failure is returned to the outer CLI so
// the explicit machine-readable delivery contract cannot fail silently.
func printExplicitReportStatus() (reportErr error) {
	// Default Trace sidecar delivery is mandatory too, but file IO failure
	// must not make runSingleShot skip printing a successfully generated answer.
	defer func() {
		if app.orch != nil {
			if err := app.orch.RootCauseOutputError(); err != nil {
				fmt.Fprintf(os.Stderr, "root-cause report write failed: %v\n", err)
				reportErr = errors.Join(reportErr, err)
			}
		}
	}()
	if strings.TrimSpace(flagReportMD) == "" && strings.TrimSpace(flagReportHTML) == "" && strings.TrimSpace(flagRootCausesOut) == "" {
		return nil
	}
	outputdump.EnsureExplicitRootCauseArtifact(outputdump.RootCauseReasonTranscriptNotAvailable)
	writes := outputdump.ExplicitReportWrites()
	if len(writes) == 0 {
		const msg = "report not written: this run produced no final-answer transcript (write/data/operation lanes and empty answers do not dump)"
		logging.Warning("[cmd] %s", msg)
		fmt.Fprintf(os.Stderr, "%s\n", msg)
		return nil
	}
	var rootCauseWriteErr error
	for _, w := range writes {
		if w.Err != nil {
			logging.Warning("[cmd] report %s write failed: %s: %v", w.Kind, w.Path, w.Err)
			fmt.Fprintf(os.Stderr, "report %s write failed: %s: %v\n", w.Kind, w.Path, w.Err)
			if w.Kind == "root-causes" {
				rootCauseWriteErr = fmt.Errorf("--root-causes-out write failed: %s: %w", w.Path, w.Err)
			}
			continue
		}
		logging.Info("[cmd] report written: %s", w.Path)
		fmt.Fprintf(os.Stderr, "\n[report written: %s]\n", w.Path)
	}
	return rootCauseWriteErr
}
