// Package cmd implements the Cobra-based CLI for codrax.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/gate"
	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/orchestrator"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Build-time variables injected via -ldflags.
var (
	version   = "dev"
	buildTime = "unknown"
)

// Code defaults for runtime settings.
//
// Two anchor domains:
//
//   - configAnchor (= exeDir) resolves providers_config. Configuration
//     files live next to the binary so one install = one config tree.
//   - runtimeAnchor (= <CWD>/.codrax) resolves log_dir / memory_dir /
//     cache_dir / blob sessions. Runtime artifacts follow the user's
//     workspace, not the install location.
//
// Relative defaults below are interpreted by the appropriate anchor
// in initApp; absolute YAML overrides and CLI flags pass through
// verbatim.
const (
	defaultLogDir          = "logs"
	defaultLogLevel        = "debug"
	defaultLogStdout       = false
	defaultMemoryDir       = "memory"
	defaultLang            = "zh"
	defaultRepo            = "."
	defaultBranch          = "main"
	defaultMaxSteps        = 50
	defaultProvidersConfig = "providers.yaml"
	runtimeAnchorDir       = ".codrax"
	blobSubdir             = "blob"
	defaultBlobMaxSessions = 7
)

// CLI flag variables.
var (
	flagProviders      string
	flagRepo           string
	flagBranch         string
	flagRequest        string
	flagMaxSteps       int
	flagLogDir         string
	flagLogLevel       string
	flagLogStdout      bool
	flagMemoryDir      string
	flagCacheDir       string
	flagLang           string
	flagMaxRetries     int
	flagMaxStageVisits int

	// Log-triage attach flags. --log reads a runtime log excerpt from
	// a file path (or "-" for stdin), --log-text accepts an inline
	// payload. Mutually exclusive. Both feed orchestrator.SetAttachedLog
	// so the log_triage pre-stage can extract stack-frame anchors.
	flagAttachLog       string
	flagAttachLogText   string
	flagLogSourcePrefix string
)

// maxAttachedLogBytes caps the pasted log payload at 1 MB to prevent a
// firehose (kubectl logs piping an entire day's output into --log=-)
// from ballooning the orchestrator's memory. The log_triage pre-stage
// would otherwise pay unbounded LLM token cost, and oversized logs
// beyond this threshold are typically aggregated multi-day dumps
// where the user should pre-filter before attaching.
const maxAttachedLogBytes = 1 << 20 // 1 MB

// appContext holds initialized state shared between subcommands.
type appContext struct {
	renderer       *render.Renderer
	orch           *orchestrator.Orchestrator
	defaultLLM     llm.Adapter
	logger         *logging.Logger
	memorySettings types.MemorySettings
}

var app appContext

// rootCmd is the top-level Cobra command.
var rootCmd = &cobra.Command{
	Use:   "codrax [request]",
	Short: "AI-powered code analysis and implementation pipeline",
	Long: `Codrax is a read-only code analysis tool with a 6-layer, 4-agent
pipeline (analyze → explore → extract → finalize).

When invoked with a request, runs the pipeline once and exits.
When invoked with no arguments, enters interactive REPL mode.`,
	Args:              cobra.MaximumNArgs(1),
	PersistentPreRunE: initApp,
	RunE:              rootRun,
	SilenceUsage:      true,
	SilenceErrors:     true,
}

func init() {
	f := rootCmd.PersistentFlags()
	f.StringVar(&flagProviders, "providers", "", "path to providers config")
	f.StringVar(&flagRepo, "repo", ".", "repository root path")
	f.StringVar(&flagBranch, "branch", "main", "git branch")
	f.StringVarP(&flagRequest, "request", "r", "", "user request (alternative to positional arg)")
	f.IntVar(&flagMaxSteps, "pipeline-max-steps", defaultMaxSteps, "maximum total pipeline steps per Run()")
	f.StringVar(&flagLogDir, "log-dir", "", "directory for log files")
	f.StringVar(&flagLogLevel, "log-level", defaultLogLevel, "log level: error|warning|info|debug")
	f.BoolVar(&flagLogStdout, "log-stdout", false, "also mirror logs to stdout")
	f.StringVar(&flagMemoryDir, "memory-dir", "", "directory for conversation memory")
	f.StringVar(&flagCacheDir, "cache-dir", "", "base directory for repo map caches (empty = ~/.cache/codrax)")
	f.StringVar(&flagLang, "lang", defaultLang, "default response language (zh/en/...); 'off' to disable")
	f.IntVar(&flagMaxRetries, "pipeline-max-retries", 0, "override max consecutive failures per stage; 0 = inherit from codrax.yaml")
	f.IntVar(&flagMaxStageVisits, "pipeline-max-stage-visits", 0, "override max entries per stage per Run; 0 = inherit from codrax.yaml")
	f.StringVar(&flagAttachLog, "log", "", "attach a runtime log excerpt (panic / exception / traceback) from a file path, or '-' for stdin; seeds log-triage entities for the analyzer")
	f.StringVar(&flagAttachLogText, "log-text", "", "inline runtime log excerpt (mutually exclusive with --log); for scripted / piped usage")
	f.StringVar(&flagLogSourcePrefix, "log-source-prefix", "", "strip this path prefix from C/C++ stack-frame files before repo lookup (override for build-machine absolute paths)")

	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("codrax %s (built %s)\n", version, buildTime)
	},
}

// Execute runs the root command.
func Execute() error {
	return ExecuteArgs(os.Args[1:])
}

// ExecuteArgs runs the root command with an explicit argv slice.
// A small compatibility normalizer rewrites accidentally-single-dashed
// long flags like `-request` to their canonical `--request` form so
// historical docs and muscle memory do not silently corrupt the value
// into `-r equest`.
func ExecuteArgs(args []string) error {
	rootCmd.SetArgs(normalizeCompatArgs(args))
	return rootCmd.Execute()
}

// compatLongFlagNames is the allowlist of long-form codrax flags we
// accept in the legacy single-dash form for CLI compatibility.
var compatLongFlagNames = map[string]struct{}{
	"providers":                 {},
	"repo":                      {},
	"branch":                    {},
	"request":                   {},
	"pipeline-max-steps":        {},
	"log-dir":                   {},
	"log-level":                 {},
	"log-stdout":                {},
	"memory-dir":                {},
	"cache-dir":                 {},
	"lang":                      {},
	"pipeline-max-retries":      {},
	"pipeline-max-stage-visits": {},
	"log":                       {},
	"log-text":                  {},
	"log-source-prefix":         {},
}

// normalizeCompatArgs rewrites known codrax long flags from the
// historical single-dash spelling (`-request`) to the canonical POSIX
// form (`--request`). It is intentionally narrow:
//
//   - only names in compatLongFlagNames are rewritten
//   - short flags like `-r` are left untouched
//   - `--` terminates rewriting, preserving positional args verbatim
func normalizeCompatArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}

	out := make([]string, 0, len(args))
	stop := false
	for _, arg := range args {
		if stop || arg == "--" {
			out = append(out, arg)
			if arg == "--" {
				stop = true
			}
			continue
		}
		if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") || arg == "-" {
			out = append(out, arg)
			continue
		}

		name, value, hasEq := strings.Cut(strings.TrimPrefix(arg, "-"), "=")
		if len(name) <= 1 {
			out = append(out, arg)
			continue
		}
		if _, ok := compatLongFlagNames[name]; !ok {
			out = append(out, arg)
			continue
		}

		if hasEq {
			out = append(out, "--"+name+"="+value)
			continue
		}
		out = append(out, "--"+name)
	}
	return out
}

// rootRun dispatches to single-shot or REPL mode.
func rootRun(cmd *cobra.Command, args []string) error {
	// Resolve request: --request flag or positional arg.
	request := flagRequest
	if request == "" && len(args) > 0 {
		request = args[0]
	}

	// Resolve attached log (file / stdin / inline text) before Run so
	// both CLI single-shot and REPL sticky-log paths share one source.
	attached, err := loadAttachedLog()
	if err != nil {
		return err
	}
	if attached != "" {
		app.orch.SetAttachedLog(attached)
		logging.Info("[cmd] attached runtime log: %d bytes", len(attached))
	}
	if flagLogSourcePrefix != "" {
		logtriage.SetSourcePrefix(flagLogSourcePrefix)
		logging.Info("[cmd] log source prefix override: %s", flagLogSourcePrefix)
	}

	if request == "" {
		return runREPL(cmd)
	}
	return runSingleShot(cmd, request)
}

// loadAttachedLog resolves the --log / --log-text flags into a single
// payload string. Returns "" when no log was requested. Rules:
//
//   - --log and --log-text are mutually exclusive
//   - --log=- reads from stdin (for pipe usage: `kubectl logs ... | codrax --log -`)
//   - otherwise --log is a filesystem path to read
//   - payload is capped at maxAttachedLogBytes; excess bytes are dropped
//     with a warning so a truncated tail does not silently hide the
//     actual error frames
func loadAttachedLog() (string, error) {
	if flagAttachLog != "" && flagAttachLogText != "" {
		return "", fmt.Errorf("--log and --log-text are mutually exclusive")
	}
	if flagAttachLogText != "" {
		return truncateAttachedLog(flagAttachLogText), nil
	}
	if flagAttachLog == "" {
		return "", nil
	}
	var data []byte
	var err error
	if flagAttachLog == "-" {
		data, err = io.ReadAll(io.LimitReader(os.Stdin, maxAttachedLogBytes+1))
	} else {
		data, err = os.ReadFile(flagAttachLog)
	}
	if err != nil {
		return "", fmt.Errorf("load attached log: %w", err)
	}
	return truncateAttachedLog(string(data)), nil
}

func truncateAttachedLog(s string) string {
	if len(s) <= maxAttachedLogBytes {
		return s
	}
	logging.Warning("[cmd] attached log truncated: %d → %d bytes", len(s), maxAttachedLogBytes)
	return s[:maxAttachedLogBytes]
}

// runSingleShot executes one pipeline run and prints the result.
// Unlike the REPL path, single-shot outputs raw markdown (no glamour
// ANSI rendering) so the output is clean when piped to another tool
// or written to a file. The REPL owns the glamour rendering step via
// its own RenderResult callback.
func runSingleShot(_ *cobra.Command, request string) error {
	logging.Info("starting pipeline for request: %s", request)
	busCtx, err := app.orch.Run(request, flagRepo, flagBranch)
	if err != nil {
		logging.Error("pipeline failed: %v", err)
		return fmt.Errorf("pipeline failed: %w", err)
	}
	if busCtx == nil {
		fmt.Println("(no result)")
		return nil
	}
	if busCtx.TaskState.LastError != "" {
		fmt.Fprintf(os.Stderr, "error: %s\n", busCtx.TaskState.LastError)
	}
	if busCtx.Mutable != nil {
		if result := busCtx.Mutable.Result(); result != "" {
			logging.Info("final answer:\n%s", result)
			// Separator between thinking trace (stderr) and answer (stdout).
			// On a terminal both streams interleave visually, so the
			// separator helps the user see where the answer begins.
			fmt.Fprintf(os.Stderr, "\n━━━\n\n")
			fmt.Print(result)
			if !strings.HasSuffix(result, "\n") {
				fmt.Println()
			}
			return nil
		}
	}
	fmt.Println("(no result)")
	return nil
}

// runREPL starts the interactive multi-turn session.
func runREPL(_ *cobra.Command) error {
	summarizer := newLLMSummarizer(app.defaultLLM)
	store, err := memory.NewStore(flagMemoryDir, summarizer, app.memorySettings)
	if err != nil {
		logging.Error("memory store init failed: %v", err)
		return fmt.Errorf("memory store init failed: %w", err)
	}
	renderFn := func(busCtx *types.BusContext) string {
		return app.renderer.RenderResult(busCtx)
	}
	r := repl.New(repl.Config{
		Runner:   app.orch,
		Store:    store,
		Render:   renderFn,
		Renderer: app.renderer,
		RepoRoot: flagRepo,
		Branch:   flagBranch,
		Out:      os.Stdout,
	})
	if err := r.Loop(); err != nil {
		logging.Error("repl exited with error: %v", err)
		return err
	}
	return nil
}

// initApp implements the 3-tier config precedence and initializes all
// subsystems. It runs as PersistentPreRunE so it executes before any
// subcommand, including the implicit root run.
func initApp(cmd *cobra.Command, _ []string) error {
	// Skip init for version subcommand.
	if cmd.Name() == "version" {
		return nil
	}

	// --- Phase 1: locate runtime settings file ---
	//
	// Lookup order (flat-first, exe-owned):
	//
	//   1. $CODRAX_SETTINGS (absolute override)
	//   2. <exeDir>/codrax.yaml               ← canonical new location
	//   3. <exeDir>/codrax/codrax.yaml        (bin + share split)
	//   4. <exeDir>/config/codrax.yaml        legacy; deprecation-warned
	//   5. <exeDir>/../config/codrax.yaml     legacy; deprecation-warned
	//   6. <CWD>/config/codrax.yaml           legacy; deprecation-warned
	//
	// Paths 4-6 stay functional so existing installs don't break; a
	// warning is emitted after logger init so operators see exactly
	// which file needs to move.
	exeDir := executableDir()
	settingsPath := ""
	settingsExplicit := false
	legacySettingsPath := ""
	if p := os.Getenv("CODRAX_SETTINGS"); p != "" {
		settingsPath = p
		settingsExplicit = true
	} else {
		type candidate struct {
			path   string
			legacy bool
		}
		var candidates []candidate
		if exeDir != "" {
			candidates = append(candidates,
				candidate{filepath.Join(exeDir, "codrax.yaml"), false},
				candidate{filepath.Join(exeDir, "codrax", "codrax.yaml"), false},
				candidate{filepath.Join(exeDir, "config", "codrax.yaml"), true},
				candidate{filepath.Join(exeDir, "..", "config", "codrax.yaml"), true},
			)
		}
		candidates = append(candidates, candidate{"config/codrax.yaml", true})
		for _, c := range candidates {
			if _, err := os.Stat(c.path); err == nil {
				settingsPath = c.path
				if c.legacy {
					legacySettingsPath = c.path
				}
				break
			}
		}
	}

	// --- Anchor resolution ---
	//
	// configAnchor roots configuration files (providers.yaml) to the
	// binary's directory. Falls back to CWD when os.Executable() is
	// unavailable (rare, e.g. some embedded scenarios).
	configAnchor := exeDir
	if configAnchor == "" {
		if cwd, err := os.Getwd(); err == nil {
			configAnchor = cwd
		}
	}
	// runtimeAnchor roots runtime artifacts to <CWD>/.codrax/. Absolute
	// so later joins are stable even if a tool chdir's (which codrax
	// doesn't do today, but the contract should still hold).
	runtimeAnchor := runtimeAnchorDir
	if abs, err := filepath.Abs(runtimeAnchor); err == nil {
		runtimeAnchor = abs
	}

	// Start with code defaults.
	mergedLogDir := defaultLogDir
	mergedLogLevel := defaultLogLevel
	mergedLogStdout := defaultLogStdout
	mergedMemoryDir := defaultMemoryDir
	mergedCacheDir := ""
	mergedLang := defaultLang
	mergedRepo := defaultRepo
	mergedBranch := defaultBranch
	mergedMaxSteps := defaultMaxSteps
	mergedProvidersConfig := defaultProvidersConfig

	// Overlay config file values.
	var rs *config.RuntimeSettings
	if settingsPath != "" {
		loaded, err := config.LoadRuntimeSettings(settingsPath)
		if err == nil {
			rs = loaded
		} else if !config.IsNotExist(err) || settingsExplicit {
			return fmt.Errorf("failed to load runtime settings from %s: %w", settingsPath, err)
		}
	}
	if rs != nil {
		// log_triage_source_prefix mirrors the --log-source-prefix CLI
		// flag for users who prefer persistent config. The CLI flag
		// still wins because rootRun applies it after initApp returns.
		// The log_triage_enabled knob is read later into LogTriageSettings
		// at agent construction, not here — the log_triager agent's
		// per-Execute gate reads settings.Enabled rather than a
		// package-level toggle.
		if rs.LogTriageSourcePrefix != nil && *rs.LogTriageSourcePrefix != "" {
			logtriage.SetSourcePrefix(*rs.LogTriageSourcePrefix)
		}
		if rs.LogDir != nil {
			mergedLogDir = *rs.LogDir
		}
		if rs.LogLevel != nil {
			mergedLogLevel = *rs.LogLevel
		}
		if rs.LogStdout != nil {
			mergedLogStdout = *rs.LogStdout
		}
		if rs.MemoryDir != nil {
			mergedMemoryDir = *rs.MemoryDir
		}
		if rs.CacheDir != nil {
			mergedCacheDir = *rs.CacheDir
		}
		if rs.Lang != nil {
			mergedLang = *rs.Lang
		}
		if rs.Repo != nil {
			mergedRepo = *rs.Repo
		}
		if rs.Branch != nil {
			mergedBranch = *rs.Branch
		}
		if rs.PipelineMaxSteps != nil {
			mergedMaxSteps = *rs.PipelineMaxSteps
		}
		if rs.ProvidersConfig != nil {
			mergedProvidersConfig = *rs.ProvidersConfig
		}
	}

	// Anchor relative paths.
	//
	// Runtime artifacts go under runtimeAnchor (<CWD>/.codrax/):
	// log_dir, memory_dir, cache_dir. Configuration files go under
	// configAnchor (exeDir): providers_config. Absolute YAML overrides
	// pass through anchorPath unchanged.
	mergedLogDir = anchorPath(runtimeAnchor, mergedLogDir)
	mergedMemoryDir = anchorPath(runtimeAnchor, mergedMemoryDir)
	if mergedCacheDir != "" {
		mergedCacheDir = anchorPath(runtimeAnchor, mergedCacheDir)
	}
	mergedProvidersConfig = anchorPath(configAnchor, mergedProvidersConfig)

	// Per-target-repo namespacing.
	logDirBase := mergedLogDir
	memoryDirBase := mergedMemoryDir
	if slug := repoSlug(mergedRepo); slug != "" {
		mergedLogDir = filepath.Join(logDirBase, slug)
		mergedMemoryDir = filepath.Join(memoryDirBase, slug)
	}

	// --- Phase 2: apply CLI flag overrides (3rd tier) ---
	// Only override if the flag was explicitly set on the command line.
	if !cmd.Flags().Changed("providers") {
		flagProviders = mergedProvidersConfig
	}
	if !cmd.Flags().Changed("repo") {
		flagRepo = mergedRepo
	}
	if !cmd.Flags().Changed("branch") {
		flagBranch = mergedBranch
	}
	if !cmd.Flags().Changed("pipeline-max-steps") {
		flagMaxSteps = mergedMaxSteps
	}
	if !cmd.Flags().Changed("log-dir") {
		flagLogDir = mergedLogDir
	}
	if !cmd.Flags().Changed("log-level") {
		flagLogLevel = mergedLogLevel
	}
	if !cmd.Flags().Changed("log-stdout") {
		flagLogStdout = mergedLogStdout
	}
	if !cmd.Flags().Changed("memory-dir") {
		flagMemoryDir = mergedMemoryDir
	}
	if !cmd.Flags().Changed("cache-dir") {
		flagCacheDir = mergedCacheDir
	}
	if !cmd.Flags().Changed("lang") {
		flagLang = mergedLang
	}

	// Re-slug if -repo was overridden but log/memory dirs were not.
	if cmd.Flags().Changed("repo") {
		if slug := repoSlug(flagRepo); slug != "" {
			if !cmd.Flags().Changed("log-dir") {
				flagLogDir = filepath.Join(logDirBase, slug)
			}
			if !cmd.Flags().Changed("memory-dir") {
				flagMemoryDir = filepath.Join(memoryDirBase, slug)
			}
		}
	}

	// --- Phase 3: initialize subsystems ---
	//
	// Create the runtime anchor up-front so the logger + memory store
	// can rely on their parent directory existing. MkdirAll is idempotent
	// so repeated invocations in the same CWD are free.
	if err := os.MkdirAll(runtimeAnchor, 0o755); err != nil {
		return fmt.Errorf("create runtime anchor %s: %w", runtimeAnchor, err)
	}

	// Apply log retention override before constructing the logger so
	// its startup sweep (pruneOldFiles) honors the configured cap.
	if rs != nil && rs.LogMaxFiles != nil {
		logging.SetMaxTotalFiles(*rs.LogMaxFiles)
	}

	logger, err := logging.NewFromFlags(flagLogDir, flagLogLevel, flagLogStdout)
	if err != nil {
		return fmt.Errorf("logging init failed: %w", err)
	}
	app.logger = logger
	logging.SetDefault(logger)
	log.SetOutput(logger.InfoWriter())
	log.SetFlags(0)

	if legacySettingsPath != "" {
		logging.Warning("runtime settings loaded from legacy path %s; move to <exeDir>/codrax.yaml to silence this warning", legacySettingsPath)
	}

	// Set up the blob session directory. With blob_max_sessions > 0
	// (the default, matching log retention), each codrax process owns
	// <CWD>/.codrax/blob/<timestamp>-<pid>/ for the lifetime of the
	// process; the orchestrator injects this into BusContext.WorkDir
	// on every Run so tool blobs land in one grep-able tree. A config
	// of 0 disables the layout and lets the orchestrator fall back to
	// the historical per-trace os.MkdirTemp + RemoveAll behavior.
	blobMaxSessions := defaultBlobMaxSessions
	if rs != nil && rs.BlobMaxSessions != nil {
		blobMaxSessions = *rs.BlobMaxSessions
	}
	blobSessionDir := ""
	if blobMaxSessions > 0 {
		blobBase := filepath.Join(runtimeAnchor, blobSubdir)
		if err := os.MkdirAll(blobBase, 0o755); err != nil {
			logging.Warning("create blob base %s failed: %v (falling back to per-trace tmpdir)", blobBase, err)
		} else {
			tool.PruneBlobSessions(blobBase, blobMaxSessions)
			blobSessionDir = filepath.Join(blobBase, tool.SessionDirName())
			if err := os.MkdirAll(blobSessionDir, 0o755); err != nil {
				logging.Warning("create blob session %s failed: %v (falling back to per-trace tmpdir)", blobSessionDir, err)
				blobSessionDir = ""
			}
		}
	}

	// Resolve --repo to an absolute path before any tool / orchestrator
	// reads it. Tools key file-system access off BusContext.RepoRoot, so
	// a literal "." here would silently fall through to the process CWD
	// at every Execute (codrax's own dir, not the user's --repo target).
	// EvalSymlinks normalizes module-cache paths so two -repo invocations
	// pointing at the same content produce the same canonical RepoRoot.
	if abs, err := filepath.Abs(flagRepo); err == nil {
		if resolved, err2 := filepath.EvalSymlinks(abs); err2 == nil {
			flagRepo = resolved
		} else {
			flagRepo = abs
		}
	}
	repomap.SetCacheDir(flagCacheDir)
	logging.Info("paths: repo=%s log-dir=%s memory-dir=%s cache-dir=%s blob-session=%s", flagRepo, flagLogDir, flagMemoryDir, flagCacheDir, blobSessionDir)

	// Build pipeline settings from codrax.yaml overrides.
	var pipelineSettings types.PipelineSettings
	if rs != nil {
		blobMax, blobHead, blobTail := 0, 0, 0
		if rs.BlobMaxInlineBytes != nil {
			blobMax = *rs.BlobMaxInlineBytes
		}
		if rs.BlobPreviewHeadBytes != nil {
			blobHead = *rs.BlobPreviewHeadBytes
		}
		if rs.BlobPreviewTailBytes != nil {
			blobTail = *rs.BlobPreviewTailBytes
		}
		tool.SetBlobLimits(blobMax, blobHead, blobTail)

		if rs.ReadFileSmallLimitThreshold != nil {
			tool.SetReadFileSmallLimitThreshold(*rs.ReadFileSmallLimitThreshold)
		}

		// Per-shape Summary caps. Start from code defaults and
		// overlay any non-nil yaml fields; a partial override leaves
		// the other entries at their default.
		{
			scc := types.DefaultSummaryCapConfig()
			if rs.SummaryCapEnabled != nil {
				scc.Enabled = *rs.SummaryCapEnabled
			}
			if rs.SummaryCapExplanation != nil {
				scc.Explanation = *rs.SummaryCapExplanation
			}
			if rs.SummaryCapValue != nil {
				scc.Value = *rs.SummaryCapValue
			}
			if rs.SummaryCapConfigValue != nil {
				scc.ConfigValue = *rs.SummaryCapConfigValue
			}
			if rs.SummaryCapBoolean != nil {
				scc.Boolean = *rs.SummaryCapBoolean
			}
			if rs.SummaryCapStepListBase != nil {
				scc.StepListBase = *rs.SummaryCapStepListBase
			}
			if rs.SummaryCapStepListPerItem != nil {
				scc.StepListPerItem = *rs.SummaryCapStepListPerItem
			}
			if rs.SummaryCapStepListMax != nil {
				scc.StepListMax = *rs.SummaryCapStepListMax
			}
			if rs.SummaryCapSymbolsBase != nil {
				scc.SymbolsBase = *rs.SummaryCapSymbolsBase
			}
			if rs.SummaryCapSymbolsPerItem != nil {
				scc.SymbolsPerItem = *rs.SummaryCapSymbolsPerItem
			}
			if rs.SummaryCapSymbolsMax != nil {
				scc.SymbolsMax = *rs.SummaryCapSymbolsMax
			}
			if rs.SummaryCapDefault != nil {
				scc.Default = *rs.SummaryCapDefault
			}
			types.SetSummaryCapConfig(scc)
		}

		// CGEC (Citation-Grounded Evidence Closure) tunables. Each
		// is a positive integer; SetCGECPolicy ignores zero/negative
		// values and keeps the current default, so a partial override
		// is safe.
		{
			forced, soft, hard := orchestrator.CGECPolicy()
			if rs.CGECForcedReadsPerRound != nil {
				forced = *rs.CGECForcedReadsPerRound
			}
			if rs.CGECStallThresholdSoft != nil {
				soft = *rs.CGECStallThresholdSoft
			}
			if rs.CGECStallThresholdHard != nil {
				hard = *rs.CGECStallThresholdHard
			}
			orchestrator.SetCGECPolicy(forced, soft, hard)
		}

		// emit_analysis runtime validation knobs. Start from the
		// code defaults and overlay any non-nil yaml fields so a
		// partial override leaves the other knobs alone.
		analysisLimits := tool.DefaultAnalysisLimits()
		if rs.AnalysisWarnBelowKeywords != nil {
			analysisLimits.WarnBelowKeywords = *rs.AnalysisWarnBelowKeywords
		}
		if rs.AnalysisRejectBelowKeywords != nil {
			analysisLimits.RejectBelowKeywords = *rs.AnalysisRejectBelowKeywords
		}
		if rs.AnalysisGenericEntityBlocklist != nil {
			// Non-nil but possibly empty slice — empty explicitly
			// disables the filter, honoring operator intent.
			analysisLimits.GenericEntityBlocklist = append(
				[]string(nil), rs.AnalysisGenericEntityBlocklist...)
		}
		if rs.AnalysisRejectMultipleEmit != nil {
			analysisLimits.RejectMultipleEmit = *rs.AnalysisRejectMultipleEmit
		}
		if rs.AnalysisMaxPrescanRounds != nil {
			analysisLimits.MaxPrescanRounds = *rs.AnalysisMaxPrescanRounds
		}
		if rs.AnalysisWarnBelowKeywordHitRatio != nil {
			analysisLimits.WarnBelowKeywordHitRatio = *rs.AnalysisWarnBelowKeywordHitRatio
		}
		if rs.AnalysisWarnBelowEntityHitRatio != nil {
			analysisLimits.WarnBelowEntityHitRatio = *rs.AnalysisWarnBelowEntityHitRatio
		}
		if rs.CGECPhase1UnreadTopK != nil {
			analysisLimits.Phase1UnreadTopK = *rs.CGECPhase1UnreadTopK
		}
		if rs.CGECPhase1UnreadMinUnread != nil {
			analysisLimits.Phase1UnreadMinUnread = *rs.CGECPhase1UnreadMinUnread
		}
		tool.SetAnalysisLimits(analysisLimits)

		// Evidence grounding policy — overridden by codrax.yaml
		// evidence_grounding_floor (default 0.5) and evidence_tier1_floor
		// (default 0.3, session-8 upstream-intercept of pure-recovery
		// investigations).
		groundingPolicy := tool.DefaultGroundingPolicy()
		if rs.EvidenceGroundingFloor != nil {
			groundingPolicy.GroundingFloor = *rs.EvidenceGroundingFloor
		}
		if rs.EvidenceTier1Floor != nil {
			groundingPolicy.Tier1Floor = *rs.EvidenceTier1Floor
		}
		tool.SetGroundingPolicy(groundingPolicy)

		if rs.PipelineMaxRetriesPerStage != nil {
			pipelineSettings.MaxRetriesPerStage = *rs.PipelineMaxRetriesPerStage
		}
		if rs.PipelineMaxStageVisits != nil {
			pipelineSettings.MaxStageVisits = *rs.PipelineMaxStageVisits
		}

		// Gate thresholds → package-global in gate package.
		var gt gate.Thresholds
		if rs.GateCoverageMin != nil {
			gt.CoverageMin = float32(*rs.GateCoverageMin)
		}
		if rs.GateCoverageWeightSymbol != nil {
			gt.CoverageWeights.Symbol = float32(*rs.GateCoverageWeightSymbol)
		}
		if rs.GateCoverageWeightConfig != nil {
			gt.CoverageWeights.Config = float32(*rs.GateCoverageWeightConfig)
		}
		if rs.GateCoverageWeightConcept != nil {
			gt.CoverageWeights.Concept = float32(*rs.GateCoverageWeightConcept)
		}
		if rs.GateHypothesisMinPriority != nil {
			gt.HypothesisMinPrio = *rs.GateHypothesisMinPriority
		}
		gate.SetGlobalThresholds(gt)

		// Explorer per-tool default cap.
		if rs.ExplorePerToolDefaultCap != nil {
			pipelineSettings.Explore.PerToolDefaultCap = *rs.ExplorePerToolDefaultCap
		}

		// Explorer heuristic thresholds.
		h := &pipelineSettings.Explore.Heuristics
		if rs.ExploreMidLoopMinIteration != nil {
			h.MidLoopMinIteration = *rs.ExploreMidLoopMinIteration
		}
		if rs.ExploreSerialBatchThreshold != nil {
			h.SerialBatchThreshold = *rs.ExploreSerialBatchThreshold
		}
		if rs.ExploreSerialStreakThreshold != nil {
			h.SerialStreakThreshold = *rs.ExploreSerialStreakThreshold
		}
		if rs.ExplorePartialReadLineThreshold != nil {
			h.PartialReadLineThreshold = *rs.ExplorePartialReadLineThreshold
		}
		if rs.ExploreMidLoopEnumCoverage != nil {
			h.MidLoopEnumCoverage = *rs.ExploreMidLoopEnumCoverage
		}
		if rs.ExploreSoftStopEnumCoverage != nil {
			h.SoftStopEnumCoverage = *rs.ExploreSoftStopEnumCoverage
		}
		if rs.ExplorePhase0MinDiscovered != nil {
			h.Phase0MinDiscoveredFiles = *rs.ExplorePhase0MinDiscovered
		}
		if rs.ExplorePhase0MaxBroaden != nil {
			h.Phase0MaxBroadenAttempts = *rs.ExplorePhase0MaxBroaden
		}
		if rs.ExploreSymbolMinLenMethod != nil {
			h.SymbolMinLenMethod = *rs.ExploreSymbolMinLenMethod
		}
		if rs.ExploreSymbolMinLenOther != nil {
			h.SymbolMinLenOther = *rs.ExploreSymbolMinLenOther
		}
		if rs.ExploreMaxPreScannedPushes != nil {
			h.MaxPreScannedPushes = *rs.ExploreMaxPreScannedPushes
		}
		if rs.ExploreCVPreviewMaxLen != nil {
			h.CVPreviewMaxLen = *rs.ExploreCVPreviewMaxLen
		}
		if rs.ExploreParallelUnreadFloor != nil {
			h.ParallelUnreadFloor = *rs.ExploreParallelUnreadFloor
		}
		if rs.ExploreEnumMidLoopUnreadFloor != nil {
			h.EnumMidLoopUnreadFloor = *rs.ExploreEnumMidLoopUnreadFloor
		}
		if rs.ExploreErmSuggestLimit != nil {
			h.ErmSuggestLimit = *rs.ExploreErmSuggestLimit
		}
	}
	// CLI flag overrides for pipeline budget.
	if cmd.Flags().Changed("pipeline-max-retries") && flagMaxRetries > 0 {
		pipelineSettings.MaxRetriesPerStage = flagMaxRetries
	}
	if cmd.Flags().Changed("pipeline-max-stage-visits") && flagMaxStageVisits > 0 {
		pipelineSettings.MaxStageVisits = flagMaxStageVisits
	}
	// Resolve zero-valued heuristic fields to code defaults.
	pipelineSettings.Explore.Heuristics = types.ResolvedExploreHeuristics(pipelineSettings.Explore.Heuristics)

	// Agent-level limits from YAML.
	if rs != nil {
		a := &pipelineSettings.Agent
		if rs.AgentMaxIterations != nil {
			a.MaxIterations = *rs.AgentMaxIterations
		}
		if rs.AgentMaxToolHistoryBytes != nil {
			a.MaxToolHistoryBytes = *rs.AgentMaxToolHistoryBytes
		}
		if rs.AgentLoopMinInjectInterval != nil {
			a.LoopMinInjectInterval = *rs.AgentLoopMinInjectInterval
		}
		if rs.AgentLoopMaxContinuations != nil {
			a.LoopMaxContinuations = *rs.AgentLoopMaxContinuations
		}
		if rs.AgentLoopMaxMidLoopInjects != nil {
			a.LoopMaxMidLoopInjects = *rs.AgentLoopMaxMidLoopInjects
		}
		if rs.AgentLoopIdleStopThreshold != nil {
			a.LoopIdleStopThreshold = *rs.AgentLoopIdleStopThreshold
		}
		if rs.AgentFinalizerMaxCorrectionRetries != nil {
			a.FinalizerMaxCorrectionRetries = *rs.AgentFinalizerMaxCorrectionRetries
		}
		if rs.AgentFinalizerPreservePriorProse != nil {
			a.FinalizerPreservePriorProse = rs.AgentFinalizerPreservePriorProse
		}
		if rs.AgentFinalizerShrinkageMinProseLen != nil {
			a.FinalizerShrinkageMinProseLen = *rs.AgentFinalizerShrinkageMinProseLen
		}
		if rs.AgentFinalizerShrinkageRatio != nil {
			a.FinalizerShrinkageRatio = *rs.AgentFinalizerShrinkageRatio
		}
		if rs.AgentExtractorMaxCorrectionRetries != nil {
			a.ExtractorMaxCorrectionRetries = *rs.AgentExtractorMaxCorrectionRetries
		}
		if rs.AgentSubTopicPrescanExtra != nil {
			a.SubTopicPrescanBudgetExtra = *rs.AgentSubTopicPrescanExtra
		}
		if rs.AgentSubTopicExplorerExtra != nil {
			a.SubTopicExplorerBudgetExtra = *rs.AgentSubTopicExplorerExtra
		}
		if rs.AgentSubTopicPipelineExtra != nil {
			a.SubTopicPipelineStepsExtra = *rs.AgentSubTopicPipelineExtra
		}
		if rs.AgentSubTopicRetryExtra != nil {
			a.SubTopicRetryBudgetExtra = *rs.AgentSubTopicRetryExtra
		}
		if rs.AgentInvestigationCompletePolicy != nil {
			a.InvestigationCompletePolicy = *rs.AgentInvestigationCompletePolicy
		}
		if rs.AgentPriorConvPolicy != nil {
			a.PriorConvPolicy = *rs.AgentPriorConvPolicy
		}
	}
	pipelineSettings.Agent = types.ResolvedAgentSettings(pipelineSettings.Agent)
	// Mirror the resolved investigation_complete_policy into the
	// tool layer so emit_investigation_complete's preCompleteContractCheck
	// can honor "override" mode (skip pre-complete gates when the
	// orchestrator's DAG scheduler is already configured to skip
	// criteria).
	tool.SetInvestigationCompletePolicy(pipelineSettings.Agent.InvestigationCompletePolicy)

	// Memory store limits from YAML.
	var memCfg types.MemorySettings
	if rs != nil {
		if rs.MemoryMaxRecentTurns != nil {
			memCfg.MaxRecentTurns = *rs.MemoryMaxRecentTurns
		}
		if rs.MemoryMaxRecentBytes != nil {
			memCfg.MaxRecentBytes = *rs.MemoryMaxRecentBytes
		}
		if rs.MemoryMaxTurnBodyBytes != nil {
			memCfg.MaxTurnBodyBytes = *rs.MemoryMaxTurnBodyBytes
		}
		if rs.MemoryMaxBuildContextMatches != nil {
			memCfg.MaxBuildContextMatches = *rs.MemoryMaxBuildContextMatches
		}
		if rs.MemoryMaxInlinedTurnBytes != nil {
			memCfg.MaxInlinedTurnBytes = *rs.MemoryMaxInlinedTurnBytes
		}
		if rs.MemoryMaxBuildContextTotalBytes != nil {
			memCfg.MaxBuildContextTotalBytes = *rs.MemoryMaxBuildContextTotalBytes
		}
	}
	app.memorySettings = types.ResolvedMemorySettings(memCfg)

	logging.Info("pipeline_settings: max_steps=%d max_retries_per_stage=%d max_stage_visits=%d",
		flagMaxSteps,
		pipelineSettings.MaxRetriesPerStage,
		pipelineSettings.MaxStageVisits)
	logging.Info("blob_limits: max_inline_bytes=%d preview_head_bytes=%d preview_tail_bytes=%d",
		tool.MaxInlineBytes, tool.PreviewHeadBytesValue(), tool.PreviewTailBytesValue())
	al := tool.CurrentAnalysisLimits()
	logging.Info("analysis_limits: warn_below_keywords=%d reject_below_keywords=%d generic_entity_blocklist=%d reject_multiple_emit=%t max_prescan_rounds=%d warn_kw_hit_ratio=%.2f warn_ent_hit_ratio=%.2f",
		al.WarnBelowKeywords, al.RejectBelowKeywords, len(al.GenericEntityBlocklist), al.RejectMultipleEmit, al.MaxPrescanRounds,
		al.WarnBelowKeywordHitRatio, al.WarnBelowEntityHitRatio)

	providersCfg, err := config.LoadProviders(flagProviders)
	if err != nil {
		return fmt.Errorf("failed to load providers config: %w", err)
	}
	app.defaultLLM = createDefaultAdapter(providersCfg)

	// Initialize registries.
	mcpRegistry := mcp.NewRegistry()
	logging.Info("registered %d MCP servers", len(mcpRegistry.List()))

	skillRegistry := skill.NewRegistry()
	skill.RegisterDefaults(skillRegistry)
	logging.Info("registered %d skills", len(skillRegistry.List()))

	renderer := render.New(os.Stdout, false)
	app.renderer = renderer

	toolRegistry := tool.NewRegistry()
	tool.RegisterDefaults(toolRegistry)
	toolRegistry.Register(&repomap.RepoMapV2{})
	toolRegistry.Register(tool.NewProposeSubAgents())

	// Runtime tool registration. After the 2026-04-14 simplification
	// all three runtime flags (evidence_tool_mode / two_turn_explorer_mode
	// / answer_document_mode) were deleted; every structured tool
	// below is unconditionally registered. The explore-skill gets
	// a runtime append for emit_evidence so the declarative skill
	// config does not need to hard-code the tool name.
	toolRegistry.Register(&tool.EmitEvidence{})
	toolRegistry.Register(&tool.EmitInvestigationComplete{})
	if exploreSkill, err := skillRegistry.Get("explore-skill"); err == nil {
		exploreSkill.ToolSuggestions = append(exploreSkill.ToolSuggestions, "emit_evidence", "emit_investigation_complete")
	}
	toolRegistry.Register(&tool.EmitAnswerSymbol{})
	toolRegistry.Register(&tool.EmitHypothesisVerdict{})
	toolRegistry.Register(&tool.EmitAnswerDocument{})
	toolRegistry.Register(&tool.EmitLogTriage{})
	toolRegistry.Register(&tool.EmitLogSegmentation{})

	subAgentRegistry := agent.NewSubAgentRegistry()

	agentCfg := pipelineSettings.Agent
	deps := &agent.Dependencies{
		LLM:               app.defaultLLM,
		Tools:             toolRegistry,
		MCPServers:        mcpRegistry,
		SubAgents:         subAgentRegistry,
		Skills:            skillRegistry,
		MaxIterations:     agentCfg.MaxIterations,
		Emit:              renderer.Emitter(),
		ExploreHeuristics: pipelineSettings.Explore.Heuristics,
		AgentSettings:     agentCfg,
		LoopPolicy: agent.LoopPolicy{
			MinInjectInterval: agentCfg.LoopMinInjectInterval,
			MaxContinuations:  agentCfg.LoopMaxContinuations,
			MaxMidLoopInjects: agentCfg.LoopMaxMidLoopInjects,
			IdleStopThreshold: agentCfg.LoopIdleStopThreshold,
		},
	}

	agent.RegisterDefaultSubAgents(subAgentRegistry, deps)
	logging.Info("registered %d tools, %d sub-agents", len(toolRegistry.List()), len(subAgentRegistry.Names()))

	resolver := func(name types.AgentName) llm.Adapter {
		resolved := config.ResolveProvider(providersCfg, string(name))
		if adapter := llm.NewFromConfig(resolved); adapter != nil {
			return adapter
		}
		return nil
	}

	// Build LogTriageSettings from codrax.yaml with defaults filling
	// any missing knob. cmd/root.go's settings loader already populated
	// rs with nil-pointer absence semantics — overlay selectively.
	triageSettings := agent.DefaultLogTriageSettings()
	if rs != nil {
		if rs.LogTriageEnabled != nil {
			triageSettings.Enabled = *rs.LogTriageEnabled
		}
		if rs.LogTriageMinBytes != nil {
			triageSettings.MinBytes = *rs.LogTriageMinBytes
		}
		if rs.LogTriageMaxRetries != nil {
			triageSettings.MaxRetries = *rs.LogTriageMaxRetries
		}
		if rs.LogTriageTwoStepEnabled != nil {
			triageSettings.TwoStepEnabled = *rs.LogTriageTwoStepEnabled
		}
		if rs.LogTriageTwoStepBytes != nil {
			triageSettings.TwoStepBytes = *rs.LogTriageTwoStepBytes
		}
		if rs.LogTriageTwoStepCoverage != nil {
			triageSettings.TwoStepCoverage = *rs.LogTriageTwoStepCoverage
		}
		if rs.LogTriageMaxLLMCalls != nil {
			triageSettings.MaxLLMCalls = *rs.LogTriageMaxLLMCalls
		}
	}

	agentRegistry := agent.NewRegistry()
	agent.RegisterDefaults(agentRegistry, deps, resolver, triageSettings)
	logging.Info("registered %d agents", len(agentRegistry.List()))

	orch := orchestrator.New(pipelineSettings, agentRegistry, skillRegistry, subAgentRegistry)
	orch.SetMaxSteps(flagMaxSteps)
	orch.SetLanguage(flagLang)
	orch.SetEmitter(renderer.Emitter())
	orch.SetBlobSessionDir(blobSessionDir)

	// Resolve per-agent think_aloud from providers.yaml. The default
	// is true (directive included); per-agent overrides can disable it.
	taMap := make(map[types.AgentName]bool)
	defaultTA := true
	if providersCfg.LLM.Default.ThinkAloud != nil {
		defaultTA = *providersCfg.LLM.Default.ThinkAloud
	}
	for _, name := range []types.AgentName{
		types.AgentAnalyzer, types.AgentExplorer,
		types.AgentExtractor, types.AgentFinalizer,
	} {
		resolved := config.ResolveProvider(providersCfg, string(name))
		if resolved.ThinkAloud != nil {
			taMap[name] = *resolved.ThinkAloud
		} else {
			taMap[name] = defaultTA
		}
	}
	orch.SetThinkAloudMap(taMap)
	app.orch = orch

	return nil
}

// --- Helper functions (moved from main.go) ---

func executableDir() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

func repoSlug(repo string) string {
	if repo == "" {
		repo = "."
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	base := filepath.Base(abs)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "repo"
	}
	base = sanitizeSlug(base)
	h := fnv.New32a()
	_, _ = h.Write([]byte(abs))
	return fmt.Sprintf("%s-%08x", base, h.Sum32())
}

func sanitizeSlug(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "repo"
	}
	return out
}

func anchorPath(anchor, p string) string {
	if anchor == "" || p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(anchor, p))
}

func createDefaultAdapter(cfg *types.ProvidersConfig) llm.Adapter {
	resolved := config.ResolveProvider(cfg, "")
	if adapter := llm.NewFromConfig(resolved); adapter != nil {
		return adapter
	}
	return &placeholderAdapter{}
}

// placeholderAdapter is a minimal LLM adapter for testing the pipeline structure.
type placeholderAdapter struct{}

func (p *placeholderAdapter) Chat(messages []llm.Message, tools []llm.ToolSchema) (llm.Response, error) {
	var lastMsg string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" || messages[i].Role == "system" {
			lastMsg = messages[i].Content
			break
		}
	}
	return llm.Response{
		Content:    fmt.Sprintf("Processed: %s", truncate(lastMsg, 200)),
		StopReason: "end_turn",
		Usage: llm.TokenUsage{
			InputTokens:  len(lastMsg),
			OutputTokens: 50,
		},
	}, nil
}

func (p *placeholderAdapter) ModelID() string       { return "placeholder-v1" }
func (p *placeholderAdapter) MaxContextTokens() int { return 200000 }

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// llmSummarizer adapts the default LLM into a memory.Summarizer.
type llmSummarizer struct {
	adapter llm.Adapter
}

func newLLMSummarizer(adapter llm.Adapter) *llmSummarizer {
	return &llmSummarizer{adapter: adapter}
}

func (s *llmSummarizer) Summarize(_ context.Context, turn memory.Turn) (memory.IndexEntry, error) {
	prompt := strings.Builder{}
	prompt.WriteString("Summarize the following conversation turn into a compact JSON object with fields: ")
	prompt.WriteString(`{"topic":"<short phrase>","keywords":["k1","k2"],"summary":"<~150 word recap>"}.`)
	prompt.WriteString(" Reply with ONLY the JSON object — no prose, no fences.\n\n")
	prompt.WriteString("USER REQUEST:\n")
	prompt.WriteString(turn.Request)
	prompt.WriteString("\n\nASSISTANT RESPONSE:\n")
	prompt.WriteString(turn.Response)
	prompt.WriteString("\n")

	fallback := memory.IndexEntry{
		ID:       turn.ID,
		Topic:    truncate(strings.ReplaceAll(turn.Request, "\n", " "), 80),
		Keywords: extractKeywords(turn.Request),
		Summary:  truncate(strings.ReplaceAll(turn.Response, "\n", " "), 400),
	}
	if s.adapter == nil {
		return fallback, nil
	}
	resp, err := s.adapter.Chat([]llm.Message{
		{Role: "user", Content: prompt.String()},
	}, nil)
	if err != nil {
		logging.Warning("[memory] summarizer LLM error, using fallback: %v", err)
		return fallback, nil
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var parsed struct {
		Topic    string   `json:"topic"`
		Keywords []string `json:"keywords"`
		Summary  string   `json:"summary"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		logging.Warning("[memory] summarizer JSON parse error, using fallback: %v", err)
		return fallback, nil
	}
	return memory.IndexEntry{
		ID:       turn.ID,
		Topic:    parsed.Topic,
		Keywords: parsed.Keywords,
		Summary:  parsed.Summary,
	}, nil
}

func extractKeywords(text string) []string {
	stop := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "is": true,
		"are": true, "was": true, "were": true, "to": true, "of": true, "in": true,
		"on": true, "for": true, "with": true, "this": true, "that": true, "it": true,
		"as": true, "be": true, "by": true, "at": true, "from": true,
	}
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	seen := map[string]bool{}
	var out []string
	for _, w := range words {
		if len(w) < 3 || stop[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
		if len(out) >= 8 {
			break
		}
	}
	return out
}
