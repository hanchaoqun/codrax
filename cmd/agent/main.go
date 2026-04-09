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
)

func main() {
	configPath := flag.String("config", "config/orchestrator.yaml", "path to orchestrator config")
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

	// Create LLM adapter
	// In production, this would be configured with API keys and model selection.
	// For now, we use a placeholder that returns structured responses.
	llmAdapter := createLLMAdapter()

	// Create SubAgent registry
	subAgentRegistry := agent.NewSubAgentRegistry()

	// Create agent registry
	deps := &agent.Dependencies{
		LLM:           llmAdapter,
		Tools:         toolRegistry,
		MCPServers:    mcpRegistry,
		SubAgents:     subAgentRegistry,
		MaxIterations: 20,
	}
	agentRegistry := agent.NewRegistry()
	agent.RegisterDefaults(agentRegistry, deps)
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

// createLLMAdapter creates the LLM adapter chain.
func createLLMAdapter() llm.Adapter {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		model := os.Getenv("OPENAI_MODEL")
		if model == "" {
			model = "gpt-4o"
		}
		baseURL := os.Getenv("OPENAI_BASE_URL")
		return llm.NewOpenAIAdapter(key, model, baseURL)
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
