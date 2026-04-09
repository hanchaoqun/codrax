package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/hanchaoqun/design/internal/agent"
	"github.com/hanchaoqun/design/internal/config"
	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/mcp"
	"github.com/hanchaoqun/design/internal/orchestrator"
	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/tool"
	"github.com/hanchaoqun/design/internal/types"
)

func main() {
	configPath := flag.String("config", "config/orchestrator.yaml", "path to orchestrator config")
	providersPath := flag.String("providers", "config/providers.yaml", "path to providers config")
	repoRoot := flag.String("repo", ".", "repository root path")
	branch := flag.String("branch", "main", "git branch")
	request := flag.String("request", "", "user request to process")
	maxSteps := flag.Int("max-steps", 50, "maximum pipeline steps")
	flag.Parse()

	if *request == "" {
		// Read from remaining args
		args := flag.Args()
		if len(args) > 0 {
			*request = args[0]
		} else {
			fmt.Fprintln(os.Stderr, "usage: agent -request 'your task description'")
			fmt.Fprintln(os.Stderr, "       agent 'your task description'")
			os.Exit(1)
		}
	}

	// Load and resolve configuration
	cfg, err := config.LoadAndResolve(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	log.Printf("loaded config: %d stages, %d policies",
		len(cfg.Stages), len(cfg.TaskPolicies))

	// Load providers config
	providersCfg, err := config.LoadProviders(*providersPath)
	if err != nil {
		log.Fatalf("failed to load providers config: %v", err)
	}
	defaultLLM := createDefaultAdapter(providersCfg)

	// Initialize registries
	mcpRegistry := mcp.NewRegistry()
	log.Printf("registered %d MCP servers", len(mcpRegistry.List()))

	skillRegistry := skill.NewRegistry()
	skill.RegisterDefaults(skillRegistry)
	log.Printf("registered %d skills", len(skillRegistry.List()))

	// Tool + SubAgent registries
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
	log.Printf("registered %d tools, %d sub-agents", len(toolRegistry.List()), len(subAgentRegistry.Names()))

	// Create agent registry with per-agent LLM resolution
	resolver := func(name types.AgentName) llm.Adapter {
		resolved := config.ResolveProvider(providersCfg, string(name))
		if adapter := llm.NewFromConfig(resolved); adapter != nil {
			return adapter
		}
		return nil
	}

	agentRegistry := agent.NewRegistry()
	agent.RegisterDefaults(agentRegistry, deps, resolver)
	log.Printf("registered %d agents", len(agentRegistry.List()))

	// Create and run orchestrator
	orch := orchestrator.New(cfg, agentRegistry, skillRegistry, subAgentRegistry)
	orch.SetMaxSteps(*maxSteps)

	log.Printf("starting pipeline for request: %s", *request)
	busCtx, err := orch.Run(*request, *repoRoot, *branch)
	if err != nil {
		log.Fatalf("pipeline failed: %v", err)
	}

	// Output results
	fmt.Printf("\n=== Pipeline Complete ===\n")
	fmt.Printf("Trace ID:   %s\n", busCtx.TraceID)
	fmt.Printf("Final Stage: %s\n", busCtx.PipelineStage)
	fmt.Printf("Terminal:    %v\n", busCtx.TaskState.IsTerminal)
	fmt.Printf("Completed:   %v\n", busCtx.TaskState.Completed)
	fmt.Printf("Facts:       %d\n", len(busCtx.RepoFacts))
	fmt.Printf("Tool Calls:  %d\n", len(busCtx.ToolResults))
	fmt.Printf("MCP Calls:   %d\n", len(busCtx.MCPResponses))

	if busCtx.TaskState.LastError != "" {
		fmt.Printf("Last Error:  %s\n", busCtx.TaskState.LastError)
	}

	// Working todolist (populated by analyzer + any agent that called todo_write).
	if busCtx.Mutable != nil {
		if tl := busCtx.Mutable.TaskList(); len(tl.Tasks) > 0 {
			fmt.Printf("\n=== Todolist ===\n")
			for _, item := range tl.Tasks {
				fmt.Printf("  %s %s\n", todoStatusIcon(item.Status), item.Title)
			}
		}
	}

	// User-facing final answer (populated by the finalizer agent).
	if busCtx.FinalAnswer != "" {
		fmt.Printf("\n=== Final Answer ===\n%s\n", busCtx.FinalAnswer)
	} else {
		fmt.Printf("\n(no final answer was produced)\n")
	}
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
	// Extract the last user/system message to understand context
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

func (p *placeholderAdapter) ModelID() string          { return "placeholder-v1" }
func (p *placeholderAdapter) MaxContextTokens() int    { return 200000 }

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
