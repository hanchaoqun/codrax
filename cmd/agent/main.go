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

	// Initialize registries
	toolRegistry := tool.NewRegistry()
	tool.RegisterDefaults(toolRegistry)
	log.Printf("registered %d tools", len(toolRegistry.List()))

	mcpRegistry := mcp.NewRegistry()
	// MCP servers would be configured here based on environment
	log.Printf("registered %d MCP servers", len(mcpRegistry.List()))

	skillRegistry := skill.NewRegistry()
	skill.RegisterDefaults(skillRegistry)
	log.Printf("registered %d skills", len(skillRegistry.List()))

	// Load providers config
	providersCfg, err := config.LoadProviders(*providersPath)
	if err != nil {
		log.Fatalf("failed to load providers config: %v", err)
	}

	// Create default LLM adapter
	defaultLLM := createDefaultAdapter(providersCfg)

	// Create SubAgent registry
	subAgentRegistry := agent.NewSubAgentRegistry()

	// Create agent registry with per-agent LLM resolution
	deps := &agent.Dependencies{
		LLM:           defaultLLM,
		Tools:         toolRegistry,
		MCPServers:    mcpRegistry,
		SubAgents:     subAgentRegistry,
		MaxIterations: 20,
	}

	resolver := func(name types.AgentName) llm.Adapter {
		resolved := config.ResolveProvider(providersCfg, string(name))
		if adapter := llm.NewFromConfig(resolved); adapter != nil {
			return adapter
		}
		return nil // use default from deps
	}

	agentRegistry := agent.NewRegistry()
	agent.RegisterDefaults(agentRegistry, deps, resolver)
	log.Printf("registered %d agents", len(agentRegistry.List()))

	// Register default SubAgents (after deps are created)
	agent.RegisterDefaultSubAgents(subAgentRegistry, deps)
	log.Printf("registered %d sub-agents", len(subAgentRegistry.Names()))

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
