package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/orchestrator"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Code defaults for runtime settings. Kept as named constants so the
// precedence chain (code default → config file → command line) is
// easy to read and test.
const (
	defaultLogDir    = "logs"
	defaultLogLevel  = "info"
	defaultLogStdout = false
	defaultMemoryDir = "memory"
	defaultLang      = "zh"
)

func main() {
	// --- Phase 1: load the runtime settings file so the values inside
	// it can serve as flag defaults. The file path is fixed at
	// config/codrax.yaml, overridable via the CODRAX_SETTINGS env var
	// for scripted/multi-environment setups. Picking this up *before*
	// flag registration avoids a two-pass flag.Parse.
	settingsPath := "config/codrax.yaml"
	settingsExplicit := false
	if p := os.Getenv("CODRAX_SETTINGS"); p != "" {
		settingsPath = p
		settingsExplicit = true
	}

	// Start with pure code defaults.
	mergedLogDir := defaultLogDir
	mergedLogLevel := defaultLogLevel
	mergedLogStdout := defaultLogStdout
	mergedMemoryDir := defaultMemoryDir
	mergedLang := defaultLang

	// Overlay config file values. Missing default file = silent skip;
	// explicit path via env var must exist. Logger is not up yet, so
	// diagnostics go to stderr.
	if rs, err := config.LoadRuntimeSettings(settingsPath); err == nil {
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
		if rs.Lang != nil {
			mergedLang = *rs.Lang
		}
	} else if !config.IsNotExist(err) || settingsExplicit {
		fmt.Fprintf(os.Stderr, "failed to load runtime settings from %s: %v\n", settingsPath, err)
		os.Exit(1)
	}

	// --- Phase 2: register flags using the merged values as defaults.
	// When the user omits a flag, the merged value wins; when they pass
	// one, the command line wins. That gives the
	// code < config file < command line precedence the task called for.
	configPath := flag.String("config", "config/orchestrator.yaml", "path to orchestrator config")
	providersPath := flag.String("providers", "config/providers.yaml", "path to providers config")
	repoRoot := flag.String("repo", ".", "repository root path")
	branch := flag.String("branch", "main", "git branch")
	request := flag.String("request", "", "user request to process (empty enters interactive mode)")
	maxSteps := flag.Int("max-steps", 50, "maximum pipeline steps")
	logDir := flag.String("log-dir", mergedLogDir, "directory for log files")
	logLevel := flag.String("log-level", mergedLogLevel, "log level: error|warning|info|debug")
	logStdout := flag.Bool("log-stdout", mergedLogStdout, "also mirror logs to stdout")
	memoryDir := flag.String("memory-dir", mergedMemoryDir, "directory for conversation memory")
	lang := flag.String("lang", mergedLang, "default response language (zh/en/...); 'off' to disable")
	flag.Parse()

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

	toolRegistry := tool.NewRegistry()
	tool.RegisterDefaults(toolRegistry)
	toolRegistry.Register(tool.NewProposeSubAgents())

	subAgentRegistry := agent.NewSubAgentRegistry()

	deps := &agent.Dependencies{
		LLM:           defaultLLM,
		Tools:         toolRegistry,
		MCPServers:    mcpRegistry,
		SubAgents:     subAgentRegistry,
		MaxIterations: 20,
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
	orch.SetMaxSteps(*maxSteps)
	orch.SetLanguage(*lang)

	// Branch: interactive REPL vs single-shot.
	if *request == "" {
		summarizer := newLLMSummarizer(defaultLLM)
		store, err := memory.NewStore(*memoryDir, summarizer)
		if err != nil {
			logging.Error("memory store init failed: %v", err)
			os.Exit(1)
		}
		r := repl.New(orch, store, renderResult, *repoRoot, *branch, os.Stdin, os.Stdout)
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
	fmt.Print(renderResult(busCtx))
}

// renderResult formats a finished BusContext for the user. Shared
// between the single-shot path and the REPL so the two stay aligned.
func renderResult(busCtx *types.BusContext) string {
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
