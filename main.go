package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
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

// Code defaults for runtime settings. Kept as named constants so the
// precedence chain (code default → config file → command line) is
// easy to read and test. These mirror the fields of
// config.RuntimeSettings exactly.
const (
	defaultLogDir             = "logs"
	defaultLogLevel           = "info"
	defaultLogStdout          = false
	defaultMemoryDir          = "memory"
	defaultLang               = "zh"
	defaultRepo               = "."
	defaultBranch             = "main"
	defaultMaxSteps           = 50
	defaultOrchestratorConfig = "config/orchestrator.yaml"
	defaultProvidersConfig    = "config/providers.yaml"
)

func main() {
	// --- Phase 1: locate the runtime settings file so the values inside
	// it can serve as flag defaults. Search order:
	//
	//   1. $CODRAX_SETTINGS — explicit override, may live anywhere
	//   2. <CWD>/config/codrax.yaml — `go run .` from the repo
	//   3. <exeDir>/config/codrax.yaml — installed binary with its
	//      config alongside it
	//
	// Picking this up *before* flag registration avoids a two-pass
	// flag.Parse.
	exeDir := executableDir()
	settingsPath := ""
	settingsExplicit := false
	if p := os.Getenv("CODRAX_SETTINGS"); p != "" {
		settingsPath = p
		settingsExplicit = true
	} else {
		// Search order: CWD (dev: `go run .`), then alongside the
		// binary, then one level up (handles the bin/ install layout
		// like /opt/codrax/{bin/codrax, config/codrax.yaml}).
		candidates := []string{"config/codrax.yaml"}
		if exeDir != "" {
			candidates = append(candidates,
				filepath.Join(exeDir, "config", "codrax.yaml"),
				filepath.Join(exeDir, "..", "config", "codrax.yaml"),
			)
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				settingsPath = c
				break
			}
		}
	}

	// Anchor directory for resolving relative *default* paths
	// (log_dir, memory_dir, orchestrator_config, providers_config).
	// We anchor on the parent of the settings file's "config/" dir so
	// logs/ and memory/ land next to the install, not next to whatever
	// CWD the user happened to be in. If no settings file was found,
	// fall back to CWD — that preserves the historical behavior for
	// people who rely on it.
	anchorDir := computeAnchorDir(settingsPath)

	// Start with pure code defaults.
	mergedLogDir := defaultLogDir
	mergedLogLevel := defaultLogLevel
	mergedLogStdout := defaultLogStdout
	mergedMemoryDir := defaultMemoryDir
	mergedCacheDir := "" // empty = os.UserCacheDir()/codrax
	mergedLang := defaultLang
	mergedRepo := defaultRepo
	mergedBranch := defaultBranch
	mergedMaxSteps := defaultMaxSteps
	mergedOrchestratorConfig := defaultOrchestratorConfig
	mergedProvidersConfig := defaultProvidersConfig

	// Overlay config file values. Missing default file = silent skip
	// (settingsPath == ""); explicit path via env var must exist.
	// Logger is not up yet, so diagnostics go to stderr.
	//
	// rs is hoisted outside the load block so the blob and pipeline
	// fields stay reachable after orchestrator config is loaded — they
	// are applied later in the startup sequence, once we know what
	// orchestrator.yaml provided as the base layer.
	var rs *config.RuntimeSettings
	if settingsPath != "" {
		loaded, err := config.LoadRuntimeSettings(settingsPath)
		if err == nil {
			rs = loaded
		} else if !config.IsNotExist(err) || settingsExplicit {
			fmt.Fprintf(os.Stderr, "failed to load runtime settings from %s: %v\n", settingsPath, err)
			os.Exit(1)
		}
	}
	if rs != nil {
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
		if rs.OrchestratorConfig != nil {
			mergedOrchestratorConfig = *rs.OrchestratorConfig
		}
		if rs.ProvidersConfig != nil {
			mergedProvidersConfig = *rs.ProvidersConfig
		}
	}

	// Anchor relative *default* path values to anchorDir, before they
	// become flag defaults. This way `-h` shows the actual resolved
	// path the binary will use, and a binary invoked from an arbitrary
	// CWD still finds its configs and writes its logs/memory next to
	// itself. User-supplied flag values are not touched (the flag
	// package passes them through verbatim) so `-log-dir ./tmp` keeps
	// doing what it always did: CWD-relative.
	//
	// -repo is intentionally excluded: its default "." means "the repo
	// I'm currently sitting in", which is a CWD concept by design.
	mergedLogDir = anchorPath(anchorDir, mergedLogDir)
	mergedMemoryDir = anchorPath(anchorDir, mergedMemoryDir)
	if mergedCacheDir != "" {
		mergedCacheDir = anchorPath(anchorDir, mergedCacheDir)
	}
	mergedOrchestratorConfig = anchorPath(anchorDir, mergedOrchestratorConfig)
	mergedProvidersConfig = anchorPath(anchorDir, mergedProvidersConfig)

	// Per-target-repo namespacing: append a stable per-repo slug to the
	// default log/memory dirs so multiple target repos sharing one
	// codrax install don't trample each other's state. Computed from
	// mergedRepo so the value baked into the flag default (and shown by
	// `-h`) reflects the repo the binary will actually use *unless* the
	// user overrides -repo on the command line — in which case we
	// re-slug after flag.Parse below. Keep the un-slugged bases around
	// for that re-slug step.
	logDirBase := mergedLogDir
	memoryDirBase := mergedMemoryDir
	if slug := repoSlug(mergedRepo); slug != "" {
		mergedLogDir = filepath.Join(logDirBase, slug)
		mergedMemoryDir = filepath.Join(memoryDirBase, slug)
	}

	// --- Phase 2: register flags using the merged values as defaults.
	// When the user omits a flag, the merged value wins; when they pass
	// one, the command line wins. That gives the
	// code < config file < command line precedence the task called for.
	configPath := flag.String("config", mergedOrchestratorConfig, "path to orchestrator config")
	providersPath := flag.String("providers", mergedProvidersConfig, "path to providers config")
	repoRoot := flag.String("repo", mergedRepo, "repository root path")
	branch := flag.String("branch", mergedBranch, "git branch")
	request := flag.String("request", "", "user request to process (empty enters interactive mode)")
	// pipelineMaxSteps is the global Run() step budget. The default
	// shown by -h reflects the merged code → codrax.yaml value.
	pipelineMaxSteps := flag.Int("pipeline-max-steps", mergedMaxSteps, "maximum total pipeline steps per Run()")
	logDir := flag.String("log-dir", mergedLogDir, "directory for log files")
	logLevel := flag.String("log-level", mergedLogLevel, "log level: error|warning|info|debug")
	logStdout := flag.Bool("log-stdout", mergedLogStdout, "also mirror logs to stdout")
	memoryDir := flag.String("memory-dir", mergedMemoryDir, "directory for conversation memory")
	cacheDir := flag.String("cache-dir", mergedCacheDir, "base directory for repo map caches (empty = ~/.cache/codrax or platform equivalent)")
	lang := flag.String("lang", mergedLang, "default response language (zh/en/...); 'off' to disable")
	// Pipeline budget knobs. Defaults are 0 sentinels — flag.Visit
	// after Parse tells us whether the user actually passed them, and
	// we apply only those into cfg.PipelineSettings (which already
	// holds the codrax.yaml merged values by then).
	// 0 means "leave whatever codrax.yaml produced". Non-zero means
	// "operator override, beats YAML".
	pipelineMaxRetries := flag.Int("pipeline-max-retries", 0, "override pipeline_max_retries_per_stage (max consecutive failures per stage before forcing finalize); 0 = inherit from yaml")
	pipelineMaxStageVisits := flag.Int("pipeline-max-stage-visits", 0, "override pipeline_max_stage_visits (oscillation guard: max entries per stage per Run); 0 = inherit from yaml")
	flag.Parse()

	// Re-slug log/memory dirs if -repo was overridden on the command
	// line but -log-dir / -memory-dir were not. The default values
	// registered above were already slugged for mergedRepo; we only
	// need to fix them when the user picked a *different* repo and
	// still wants the per-repo namespacing behavior. Explicit
	// -log-dir / -memory-dir always wins — the user opted in, we
	// don't second-guess.
	explicitFlags := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicitFlags[f.Name] = true })
	if explicitFlags["repo"] {
		if slug := repoSlug(*repoRoot); slug != "" {
			if !explicitFlags["log-dir"] {
				*logDir = filepath.Join(logDirBase, slug)
			}
			if !explicitFlags["memory-dir"] {
				*memoryDir = filepath.Join(memoryDirBase, slug)
			}
		}
	}

	// Initialize the leveled logger first so every subsequent message
	// flows through it. Failures here surface to stderr because there
	// is no logger yet to record them.
	logger, err := logging.NewFromFlags(*logDir, *logLevel, *logStdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logging init failed: %v\n", err)
		os.Exit(1)
	}
	logging.SetDefault(logger)
	defer logger.Close()
	// Safety net: any third-party code (or stragglers we missed)
	// using stdlib log will land in the same file.
	log.SetOutput(logger.InfoWriter())
	log.SetFlags(0)
	// Apply cache-dir override for repo map. Empty = default (os.UserCacheDir).
	repomap.SetCacheDir(*cacheDir)
	logging.Info("paths: repo=%s log-dir=%s memory-dir=%s cache-dir=%s", *repoRoot, *logDir, *memoryDir, *cacheDir)

	// Resolve the request: -request flag wins, otherwise positional
	// arg, otherwise empty (which triggers interactive mode below).
	if *request == "" {
		if args := flag.Args(); len(args) > 0 {
			*request = args[0]
		}
	}

	// Load configuration.
	cfg, err := config.LoadAndResolve(*configPath)
	if err != nil {
		logging.Error("failed to load config: %v", err)
		os.Exit(1)
	}
	logging.Info("loaded config: %d stages, %d policies", len(cfg.Stages), len(cfg.TaskPolicies))

	// Apply runtime overrides from codrax.yaml.
	//
	// Precedence chain (lowest wins last):
	//   code default → codrax.yaml pipeline_* keys → CLI flags
	if rs != nil {
		// Tool blob sizing knobs. SetBlobLimits ignores non-positive
		// values, so any field the user omitted leaves the historical
		// default in place.
		blobMax := 0
		blobHead := 0
		blobTail := 0
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
		// Pipeline behavior from codrax.yaml.
		if rs.PipelineMaxRetriesPerStage != nil {
			cfg.PipelineSettings.MaxRetriesPerStage = *rs.PipelineMaxRetriesPerStage
		}
		if rs.PipelineMaxStageVisits != nil {
			cfg.PipelineSettings.MaxStageVisits = *rs.PipelineMaxStageVisits
		}
		if rs.PipelineEnableVerify != nil {
			cfg.PipelineSettings.EnableVerify = *rs.PipelineEnableVerify
		}
		if rs.PipelineRequireReview != nil {
			cfg.PipelineSettings.RequireReview = *rs.PipelineRequireReview
		}
		if rs.PipelineAllowSkipPlanForSmall != nil {
			cfg.PipelineSettings.AllowSkipPlanForSmallChange = *rs.PipelineAllowSkipPlanForSmall
		}
	}
	// CLI flag overrides for the two pipeline budget params.
	if explicitFlags["pipeline-max-retries"] && *pipelineMaxRetries > 0 {
		cfg.PipelineSettings.MaxRetriesPerStage = *pipelineMaxRetries
	}
	if explicitFlags["pipeline-max-stage-visits"] && *pipelineMaxStageVisits > 0 {
		cfg.PipelineSettings.MaxStageVisits = *pipelineMaxStageVisits
	}
	logging.Info("pipeline_settings: max_steps=%d enable_verify=%v require_review=%v max_retries_per_stage=%d max_stage_visits=%d allow_skip_plan_for_small_change=%v",
		*pipelineMaxSteps,
		cfg.PipelineSettings.EnableVerify,
		cfg.PipelineSettings.RequireReview,
		cfg.PipelineSettings.MaxRetriesPerStage,
		cfg.PipelineSettings.MaxStageVisits,
		cfg.PipelineSettings.AllowSkipPlanForSmallChange)
	logging.Info("blob_limits: max_inline_bytes=%d preview_head_bytes=%d preview_tail_bytes=%d",
		tool.MaxInlineBytes, tool.PreviewHeadBytesValue(), tool.PreviewTailBytesValue())

	providersCfg, err := config.LoadProviders(*providersPath)
	if err != nil {
		logging.Error("failed to load providers config: %v", err)
		os.Exit(1)
	}
	defaultLLM := createDefaultAdapter(providersCfg)

	// Initialize registries.
	mcpRegistry := mcp.NewRegistry()
	logging.Info("registered %d MCP servers", len(mcpRegistry.List()))

	skillRegistry := skill.NewRegistry()
	skill.RegisterDefaults(skillRegistry)
	logging.Info("registered %d skills", len(skillRegistry.List()))

	// Initialize CLI renderer. Color is auto-detected from stdout TTY.
	renderer := render.New(os.Stdout, false)

	toolRegistry := tool.NewRegistry()
	tool.RegisterDefaults(toolRegistry)
	toolRegistry.Register(&repomap.RepoMapV2{})
	toolRegistry.Register(tool.NewProposeSubAgents())

	subAgentRegistry := agent.NewSubAgentRegistry()

	deps := &agent.Dependencies{
		LLM:           defaultLLM,
		Tools:         toolRegistry,
		MCPServers:    mcpRegistry,
		SubAgents:     subAgentRegistry,
		MaxIterations: 20,
		Emit:          renderer.Emitter(),
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

	agentRegistry := agent.NewRegistry()
	agent.RegisterDefaults(agentRegistry, deps, resolver)
	logging.Info("registered %d agents", len(agentRegistry.List()))

	orch := orchestrator.New(cfg, agentRegistry, skillRegistry, subAgentRegistry)
	orch.SetMaxSteps(*pipelineMaxSteps)
	orch.SetLanguage(*lang)
	orch.SetEmitter(renderer.Emitter())

	// Branch: interactive REPL vs single-shot.
	if *request == "" {
		summarizer := newLLMSummarizer(defaultLLM)
		store, err := memory.NewStore(*memoryDir, summarizer)
		if err != nil {
			logging.Error("memory store init failed: %v", err)
			os.Exit(1)
		}
		renderFn := func(busCtx *types.BusContext) string {
			return renderer.RenderResult(busCtx)
		}
		r := repl.New(orch, store, renderFn, *repoRoot, *branch, os.Stdin, os.Stdout)
		if err := r.Loop(); err != nil {
			logging.Error("repl exited with error: %v", err)
			os.Exit(1)
		}
		return
	}

	logging.Info("starting pipeline for request: %s", *request)
	busCtx, err := orch.Run(*request, *repoRoot, *branch)
	if err != nil {
		logging.Error("pipeline failed: %v", err)
		os.Exit(1)
	}
	rendered := renderer.RenderResult(busCtx)
	// Mirror the final user-visible output into the log file so the
	// answer is recoverable from logs alone after the terminal session
	// is gone. Single Info call so multi-line content stays one record.
	logging.Info("final answer:\n%s", rendered)
	fmt.Print(rendered)
}

// executableDir returns the directory of the running binary, with
// symlinks resolved so a `cp` of the binary into /usr/local/bin still
// resolves to the right neighborhood. Returns "" if the OS can't tell
// us — callers must tolerate that.
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

// repoSlug derives a stable, human-recognizable directory name for a
// target repo path. Format: "<sanitized-basename>-<fnv32-hash8>". The
// hash is taken from the repo's absolute, symlink-resolved path so the
// same repo entered via different CWDs / symlinks shares one slug
// (and therefore one memory bucket). Returns "" only when filepath.Abs
// itself fails, which on Linux means we have bigger problems than
// per-repo namespacing.
func repoSlug(repo string) string {
	if repo == "" {
		repo = "."
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		return ""
	}
	// EvalSymlinks needs the path to exist; tolerate "not yet" by
	// falling back to the lexical absolute path.
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

// sanitizeSlug folds a string down to characters that are universally
// safe in directory names: [A-Za-z0-9._-]. Anything else collapses to
// '_'. We never use this for parsing — only for keeping `ls` output
// readable when the repo basename contains spaces, slashes (shouldn't
// happen post-Base, but defensive), or non-ASCII.
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

// anchorPath joins p onto anchor when p is a non-empty relative path,
// then cleans the result. Absolute paths and empty strings pass
// through unchanged. Used to resolve default path values like "logs"
// into something that doesn't depend on CWD. The Clean step collapses
// the "/foo/bar/../config/codrax.yaml" the bin/ candidate produces.
func anchorPath(anchor, p string) string {
	if anchor == "" || p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(anchor, p))
}

// computeAnchorDir picks the directory that relative default paths
// (logs/, memory/, config/orchestrator.yaml, ...) should resolve
// against. The anchor is the parent of the settings file's "config/"
// directory when the file lives under one — that matches the layout
// the repo ships with — and the file's own directory otherwise. If
// no settings file was located, fall back to CWD so the historical
// "everything is CWD-relative" behavior still applies for users who
// run codrax from a checkout without a codrax.yaml at all.
func computeAnchorDir(settingsPath string) string {
	if settingsPath != "" {
		if abs, err := filepath.Abs(filepath.Clean(settingsPath)); err == nil {
			parent := filepath.Dir(abs)
			if filepath.Base(parent) == "config" {
				return filepath.Dir(parent)
			}
			return parent
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}

// renderResultPlain is a fallback plain-text renderer for environments
// without the rich renderer (e.g. tests). It is no longer used in
// main.go but kept for reference and testing.
func renderResultPlain(busCtx *types.BusContext) string {
	if busCtx == nil {
		return "(no result)\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n=== Pipeline Complete ===\n")
	fmt.Fprintf(&b, "Trace ID:    %s\n", busCtx.TraceID)
	fmt.Fprintf(&b, "Final Stage: %s\n", busCtx.PipelineStage)
	fmt.Fprintf(&b, "Terminal:    %v\n", busCtx.TaskState.IsTerminal)
	fmt.Fprintf(&b, "Completed:   %v\n", busCtx.TaskState.Completed)
	fmt.Fprintf(&b, "Facts:       %d\n", len(busCtx.RepoFacts))
	fmt.Fprintf(&b, "Tool Calls:  %d\n", len(busCtx.ToolResults))
	fmt.Fprintf(&b, "MCP Calls:   %d\n", len(busCtx.MCPResponses))
	if busCtx.TaskState.LastError != "" {
		fmt.Fprintf(&b, "Last Error:  %s\n", busCtx.TaskState.LastError)
	}

	if busCtx.Mutable != nil {
		tl := busCtx.Mutable.TaskList()
		if len(tl.Tasks) > 0 {
			fmt.Fprintf(&b, "\n=== Todolist ===\n")
			for _, item := range tl.Tasks {
				fmt.Fprintf(&b, "  %s %s\n", todoStatusIcon(item.Status), item.Title)
			}
			fmt.Fprintf(&b, "\n=== Task Results ===\n")
			for _, item := range tl.Tasks {
				fmt.Fprintf(&b, "\n--- %s %s ---\n", todoStatusIcon(item.Status), item.Title)
				if item.Result != "" {
					fmt.Fprintln(&b, item.Result)
				} else {
					fmt.Fprintln(&b, "(no result)")
				}
			}
		} else {
			fmt.Fprintf(&b, "\n(no tasks were produced)\n")
		}
	}
	return b.String()
}

func todoStatusIcon(s types.TaskStatus) string {
	switch s {
	case types.TaskInProgress:
		return "[~]"
	case types.TaskDone:
		return "[x]"
	case types.TaskBlocked:
		return "[!]"
	case types.TaskFailed:
		return "[F]"
	default:
		return "[ ]"
	}
}

// createDefaultAdapter builds the default LLM adapter from providers config.
func createDefaultAdapter(cfg *types.ProvidersConfig) llm.Adapter {
	resolved := config.ResolveProvider(cfg, "")
	if adapter := llm.NewFromConfig(resolved); adapter != nil {
		return adapter
	}
	return &placeholderAdapter{}
}

// placeholderAdapter is a minimal LLM adapter for testing the pipeline structure.
// It returns simple responses that allow the pipeline to progress through stages.
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

// llmSummarizer adapts the default LLM into a memory.Summarizer. It
// asks the model for a compact JSON object describing the turn so the
// store can append a structured IndexEntry to MEMORY.md. On any failure
// — model error, malformed JSON — it falls back to a deterministic
// truncation so compaction never blocks the REPL.
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
	// Strip code fences if the model added any.
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

// extractKeywords pulls a handful of distinctive words out of text as a
// fallback when the LLM summarizer fails. Stop words are filtered.
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
