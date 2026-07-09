package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracediag"
)

// runTraceDiagCLI executes the --tracediag collection mode (§28.12): a
// deterministic, zero-LLM, read-only run of a YAML collection script against
// a trace file through the tracequery engine. It never touches providers,
// worktrees, the orchestrator, or any repo path — rootPreRun skips initApp
// for this mode. The report always covers every step (collection
// completeness); any failed step turns into a nonzero exit AFTER the full
// report is written.
func runTraceDiagCLI(ctx context.Context, stdout io.Writer, args []string) error {
	scriptPath := strings.TrimSpace(flagTraceDiag)
	tracePath := strings.TrimSpace(flagTraceDiagTrace)
	if tracePath == "" {
		return fmt.Errorf("--tracediag requires --trace <trace-file>")
	}
	// P2-3 (对抗复核 2026-07-09): pipeline-mode flags combined with
	// --tracediag used to be silently ignored (they do nothing because
	// initApp is skipped). A deterministic CLI refuses the ambiguity.
	if conflicts := traceDiagConflictingFlags(args); len(conflicts) > 0 {
		return fmt.Errorf("--tracediag is a standalone collection mode and cannot be combined with: %s", strings.Join(conflicts, ", "))
	}
	// General logging flags are harmless but INERT here (tracediag installs
	// no logger); disclose on stderr instead of silently swallowing. Never
	// on the report stream.
	if inert := traceDiagInertLogFlags(); len(inert) > 0 {
		fmt.Fprintf(os.Stderr, "tracediag: 忽略日志 flag(本模式不装日志器): %s\n", strings.Join(inert, ", "))
	}
	opts := tracediag.Options{
		ScriptPath: scriptPath,
		TracePath:  tracePath,
		FlavorHint: strings.TrimSpace(flagTraceDiagFlavor),
		Version:    version,
		BuildTime:  buildTime,
	}
	outPath := strings.TrimSpace(flagTraceDiagOut)
	if outPath == "" {
		failed, err := tracediag.Run(ctx, opts, stdout)
		if err != nil {
			return err
		}
		return traceDiagExitError(failed)
	}
	// P2-1 (对抗复核 2026-07-09): never truncate an existing report before
	// the run is known to produce one. The report writes to a same-directory
	// temp file and renames over --out only when Run itself succeeded.
	// Failed STEPS still rename — the completed report carrying their
	// verbatim errors IS the product; a failed RUN (script/trace/flavor
	// errors) removes the temp file and leaves any previous report
	// byte-identical.
	tmp, err := os.CreateTemp(filepath.Dir(outPath), filepath.Base(outPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("--out: %w", err)
	}
	tmpPath := tmp.Name()
	failed, runErr := tracediag.Run(ctx, opts, tmp)
	closeErr := tmp.Close()
	if runErr != nil || closeErr != nil {
		os.Remove(tmpPath)
		if runErr != nil {
			return runErr
		}
		return fmt.Errorf("--out: %w", closeErr)
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("--out: %w", err)
	}
	fmt.Fprintf(stdout, "tracediag: report written to %s\n", outPath)
	return traceDiagExitError(failed)
}

// traceDiagConflictingFlags lists the pipeline-mode flags/arguments that are
// meaningless under --tracediag. Value-based detection (non-default value =
// conflict): precise, testable without a cobra parse, and an explicitly
// passed default is a harmless no-op either way.
func traceDiagConflictingFlags(args []string) []string {
	conflicts := []string{}
	if strings.TrimSpace(flagRequest) != "" {
		conflicts = append(conflicts, "--request")
	}
	if len(args) > 0 {
		conflicts = append(conflicts, "positional request argument")
	}
	if flagMode != "auto" {
		conflicts = append(conflicts, "--mode")
	}
	if flagWritePhase != "apply" {
		conflicts = append(conflicts, "--write-phase")
	}
	if strings.TrimSpace(flagWriteAudit) != "" {
		conflicts = append(conflicts, "--write-audit")
	}
	if strings.TrimSpace(flagPlanOut) != "" {
		conflicts = append(conflicts, "--plan-out")
	}
	if strings.TrimSpace(flagPlanFile) != "" {
		conflicts = append(conflicts, "--plan-file")
	}
	if strings.TrimSpace(flagDataResume) != "" {
		conflicts = append(conflicts, "--data-resume")
	}
	if len(flagAttachLog) > 0 || strings.TrimSpace(flagAttachLogText) != "" {
		conflicts = append(conflicts, "--log/--log-text")
	}
	if len(flagAttachHitrace) > 0 || strings.TrimSpace(flagAttachHitraceText) != "" ||
		len(flagAttachAtrace) > 0 || strings.TrimSpace(flagAttachAtraceText) != "" {
		conflicts = append(conflicts, "--htrace/--atrace (use --trace for tracediag)")
	}
	return conflicts
}

// traceDiagInertLogFlags names the general logging flags that are inert in
// tracediag mode — disclosed on stderr, never silently swallowed (P2-3).
func traceDiagInertLogFlags() []string {
	inert := []string{}
	if flagLogLevel != defaultLogLevel {
		inert = append(inert, "--log-level")
	}
	if flagLogStdout {
		inert = append(inert, "--log-stdout")
	}
	return inert
}

// traceDiagExitError converts the completed run's failed-step count into the
// CLI exit contract (§28.12): any failed step ⇒ nonzero exit, but only AFTER
// the full report was written (collection completeness beats early abort).
func traceDiagExitError(failed int) error {
	if failed > 0 {
		return fmt.Errorf("tracediag: %d step(s) failed; the report is complete and carries each step's verbatim engine error", failed)
	}
	return nil
}
