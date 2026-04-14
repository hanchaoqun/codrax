// Package cmd implements the Cobra-based CLI for codrax.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

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

// Build-time variables injected via -ldflags.
var (
	version   = "dev"
	buildTime = "unknown"
)

// Code defaults for runtime settings.
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

// CLI flag variables.
var (
	flagConfig             string
	flagProviders          string
	flagRepo               string
	flagBranch             string
	flagRequest            string
	flagMaxSteps           int
	flagLogDir             string
	flagLogLevel           string
	flagLogStdout          bool
	flagMemoryDir          string
	flagCacheDir           string
	flagLang               string
	flagMaxRetries         int
	flagMaxStageVisits     int
)

// appContext holds initialized state shared between subcommands.
type appContext struct {
	renderer   *render.Renderer
	orch       *orchestrator.Orchestrator
	defaultLLM llm.Adapter
	cfg        *config.ResolvedConfig
	logger     *logging.Logger
}

var app appContext

// rootCmd is the top-level Cobra command.
var rootCmd = &cobra.Command{
	Use:   "codrax [request]",
	Short: "AI-powered code analysis and implementation pipeline",
	Long: `Codrax is a 5-layer multi-agent AI system that decomposes software
engineering tasks through a YAML-driven state machine pipeline.

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
	f.StringVar(&flagConfig, "config", "", "path to orchestrator config")
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
	f.IntVar(&flagMaxRetries, "pipeline-max-retries", 0, "override max consecutive failures per stage; 0 = inherit from yaml")
	f.IntVar(&flagMaxStageVisits, "pipeline-max-stage-visits", 0, "override max entries per stage per Run; 0 = inherit from yaml")

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
	return rootCmd.Execute()
}

// rootRun dispatches to single-shot or REPL mode.
func rootRun(cmd *cobra.Command, args []string) error {
	// Resolve request: --request flag or positional arg.
	request := flagRequest
	if request == "" && len(args) > 0 {
		request = args[0]
	}

	if request == "" {
		return runREPL(cmd)
	}
	return runSingleShot(cmd, request)
}

// runSingleShot executes one pipeline run and prints the result.
func runSingleShot(_ *cobra.Command, request string) error {
	logging.Info("starting pipeline for request: %s", request)
	busCtx, err := app.orch.Run(request, flagRepo, flagBranch)
	if err != nil {
		logging.Error("pipeline failed: %v", err)
		return fmt.Errorf("pipeline failed: %w", err)
	}
	rendered := app.renderer.RenderResult(busCtx)
	logging.Info("final answer:\n%s", rendered)
	fmt.Print(rendered)
	return nil
}

// runREPL starts the interactive multi-turn session.
func runREPL(_ *cobra.Command) error {
	summarizer := newLLMSummarizer(app.defaultLLM)
	store, err := memory.NewStore(flagMemoryDir, summarizer)
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
	exeDir := executableDir()
	settingsPath := ""
	settingsExplicit := false
	if p := os.Getenv("CODRAX_SETTINGS"); p != "" {
		settingsPath = p
		settingsExplicit = true
	} else {
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

	anchorDir := computeAnchorDir(settingsPath)

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
	mergedOrchestratorConfig := defaultOrchestratorConfig
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

	// Anchor relative paths.
	mergedLogDir = anchorPath(anchorDir, mergedLogDir)
	mergedMemoryDir = anchorPath(anchorDir, mergedMemoryDir)
	if mergedCacheDir != "" {
		mergedCacheDir = anchorPath(anchorDir, mergedCacheDir)
	}
	mergedOrchestratorConfig = anchorPath(anchorDir, mergedOrchestratorConfig)
	mergedProvidersConfig = anchorPath(anchorDir, mergedProvidersConfig)

	// Per-target-repo namespacing.
	logDirBase := mergedLogDir
	memoryDirBase := mergedMemoryDir
	if slug := repoSlug(mergedRepo); slug != "" {
		mergedLogDir = filepath.Join(logDirBase, slug)
		mergedMemoryDir = filepath.Join(memoryDirBase, slug)
	}

	// --- Phase 2: apply CLI flag overrides (3rd tier) ---
	// Only override if the flag was explicitly set on the command line.
	if !cmd.Flags().Changed("config") {
		flagConfig = mergedOrchestratorConfig
	}
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
	logger, err := logging.NewFromFlags(flagLogDir, flagLogLevel, flagLogStdout)
	if err != nil {
		return fmt.Errorf("logging init failed: %w", err)
	}
	app.logger = logger
	logging.SetDefault(logger)
	log.SetOutput(logger.InfoWriter())
	log.SetFlags(0)

	repomap.SetCacheDir(flagCacheDir)
	logging.Info("paths: repo=%s log-dir=%s memory-dir=%s cache-dir=%s", flagRepo, flagLogDir, flagMemoryDir, flagCacheDir)

	// Load configuration.
	cfg, err := config.LoadAndResolve(flagConfig)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	app.cfg = cfg
	logging.Info("loaded config: %d stages, %d policies", len(cfg.Stages), len(cfg.TaskPolicies))

	// Apply runtime overrides from codrax.yaml.
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
		if rs.TwoTurnExplorerMode != nil {
			agent.SetTwoTurnExplorerMode(*rs.TwoTurnExplorerMode)
		}
		if rs.AnswerDocumentMode != nil {
			agent.SetAnswerDocumentMode(*rs.AnswerDocumentMode)
		}
	}
	// CLI flag overrides for pipeline budget.
	if cmd.Flags().Changed("pipeline-max-retries") && flagMaxRetries > 0 {
		cfg.PipelineSettings.MaxRetriesPerStage = flagMaxRetries
	}
	if cmd.Flags().Changed("pipeline-max-stage-visits") && flagMaxStageVisits > 0 {
		cfg.PipelineSettings.MaxStageVisits = flagMaxStageVisits
	}
	logging.Info("pipeline_settings: max_steps=%d enable_verify=%v require_review=%v max_retries_per_stage=%d max_stage_visits=%d allow_skip_plan_for_small_change=%v",
		flagMaxSteps,
		cfg.PipelineSettings.EnableVerify,
		cfg.PipelineSettings.RequireReview,
		cfg.PipelineSettings.MaxRetriesPerStage,
		cfg.PipelineSettings.MaxStageVisits,
		cfg.PipelineSettings.AllowSkipPlanForSmallChange)
	logging.Info("blob_limits: max_inline_bytes=%d preview_head_bytes=%d preview_tail_bytes=%d",
		tool.MaxInlineBytes, tool.PreviewHeadBytesValue(), tool.PreviewTailBytesValue())

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

	// emit_evidence is unconditionally registered for the explorer.
	// The 2026-04-14 simplification removed the evidence_tool_mode
	// flag gate — the structured channel is always on and the
	// markdown parser (parseEvidenceItems) still runs as a secondary
	// merge channel downstream. repo-explore-skill's ToolSuggestions
	// gets a runtime append so the declarative skill config does not
	// need to hard-code the tool name.
	toolRegistry.Register(&tool.EmitEvidence{})
	if exploreSkill, err := skillRegistry.Get("repo-explore-skill"); err == nil {
		exploreSkill.ToolSuggestions = append(exploreSkill.ToolSuggestions, "emit_evidence")
	}
	// P2.1 (two_turn_explorer_mode): register the extractor's tool
	// types. The extract-skill itself is declared declaratively in
	// internal/skill/defaults.go with its ToolSuggestions pre-bound to
	// the three emit_* names — no runtime append needed (post P2.1
	// Session 2 cleanup). The actual dispatch gating happens at the
	// orchestrator layer (extractStageEnabled in scheduler.go), so
	// registering the tool TYPES unconditionally is safe: when
	// two-turn mode is off, the scheduler never dispatches the extract
	// stage and the tools are simply unused in the registry.
	if agent.TwoTurnExplorerEnabled() {
		toolRegistry.Register(&tool.EmitAnswerSymbol{})
		toolRegistry.Register(&tool.EmitHypothesisVerdict{})
		logging.Info("two_turn_explorer_mode=on — emit_answer_symbol + emit_hypothesis_verdict tool types registered (extract-skill already declares them declaratively)")
	}
	// P2.2 (answer_document_mode): register the emit_answer_document
	// tool type process-wide. The per-skill binding is already
	// declared in answer-document-skill.ToolSuggestions in
	// internal/skill/defaults.go — the skill is registered
	// unconditionally, and Orchestrator.dispatchStage routes the
	// finalize stage to it when AnswerDocumentEnabled() is true.
	// This flag-driven routing replaces the pre-cleanup approach of
	// patching both legacy finalize skills' ToolSuggestions at
	// runtime, which would have left their markdown Answer/Evidence
	// OutputFormat sections in place and contradicted the evaluator's
	// "call the tool, do not write prose" directive.
	if agent.AnswerDocumentEnabled() {
		toolRegistry.Register(&tool.EmitAnswerDocument{})
		logging.Info("answer_document_mode=on — emit_answer_document registered; dispatchStage will route finalize to answer-document-skill")
	}

	subAgentRegistry := agent.NewSubAgentRegistry()

	deps := &agent.Dependencies{
		LLM:           app.defaultLLM,
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
	orch.SetMaxSteps(flagMaxSteps)
	orch.SetLanguage(flagLang)
	orch.SetEmitter(renderer.Emitter())
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
