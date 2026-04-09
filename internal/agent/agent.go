package agent

import (
	"encoding/json"
	"fmt"
	"log"

	agentctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// StageOutput is the structured result an Agent returns to the Orchestrator.
type StageOutput struct {
	// Raw JSON output produced by the Agent
	Data json.RawMessage `json:"data"`

	// Signals to update in BusContext
	SignalUpdates *types.ExecutionSignals `json:"signal_updates,omitempty"`

	// Facts discovered during execution
	NewFacts []types.RepoFact `json:"new_facts,omitempty"`

	// Tool results collected during execution
	ToolResults []types.ToolResult `json:"tool_results,omitempty"`

	// MCP responses collected during execution
	MCPResponses []types.MCPResponse `json:"mcp_responses,omitempty"`

	// The missing piece after this stage completes
	MissingPiece types.MissingPiece `json:"missing_piece"`

	// Error if the stage failed
	Error string `json:"error,omitempty"`

	// FinalAnswer is the user-facing text the finalizer agent produces.
	// applyStageOutput copies it to BusContext.FinalAnswer when non-empty
	// so the CLI (and any other caller of Run) can render it. Other
	// agents leave this empty.
	FinalAnswer string `json:"final_answer,omitempty"`
}

// Agent defines the interface for all agent types.
type Agent interface {
	// Name returns the agent identifier.
	Name() types.AgentName

	// Execute runs the agent's ReAct loop and returns structured output.
	Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error)
}

// Dependencies bundles the external dependencies an Agent needs.
type Dependencies struct {
	LLM           llm.Adapter
	Tools         *tool.Registry
	MCPServers    *mcp.Registry
	SubAgents     *SubAgentRegistry
	MaxIterations int
}

// BaseAgent provides the common ReAct loop implementation.
// Concrete agents embed this and customize behavior via the Evaluator interface.
type BaseAgent struct {
	name types.AgentName
	deps *Dependencies
	eval Evaluator
}

// Evaluator allows concrete agents to customize the ReAct loop behavior.
type Evaluator interface {
	// BuildInitialPrompt creates the first user message for the ReAct loop.
	BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string

	// ShouldStop decides if the loop should terminate based on the LLM response.
	ShouldStop(resp llm.Response, iteration int) bool

	// ParseOutput extracts the structured StageOutput from accumulated results.
	ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, mcpResponses []types.MCPResponse) (*StageOutput, error)

	// DetermineMissingPiece decides what's still missing after this stage.
	DetermineMissingPiece(ctx *types.AgentContext, output *StageOutput) types.MissingPiece
}

// NewBaseAgent creates a new BaseAgent.
func NewBaseAgent(name types.AgentName, deps *Dependencies, eval Evaluator) *BaseAgent {
	maxIter := deps.MaxIterations
	if maxIter <= 0 {
		maxIter = 20
	}
	return &BaseAgent{
		name: name,
		deps: &Dependencies{
			LLM:           deps.LLM,
			Tools:         deps.Tools,
			MCPServers:    deps.MCPServers,
			SubAgents:     deps.SubAgents,
			MaxIterations: maxIter,
		},
		eval: eval,
	}
}

func (b *BaseAgent) Name() types.AgentName {
	return b.name
}

// Execute implements the ReAct (Reason → Act → Observe) loop.
func (b *BaseAgent) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {
	// Build tool schemas for LLM
	toolSchemas := b.buildToolSchemas(sk)

	// Initialize message history
	messages := b.buildInitialMessages(ctx, sk)

	var allToolResults []types.ToolResult
	var allMCPResponses []types.MCPResponse

	// ReAct loop
	for i := 0; i < b.deps.MaxIterations; i++ {
		// Reason — call LLM
		resp, err := b.deps.LLM.Chat(messages, toolSchemas)
		if err != nil {
			return &StageOutput{
				Error:        fmt.Sprintf("LLM call failed: %v", err),
				MissingPiece: ctx.MissingPiece,
			}, err
		}

		// Check if we should stop
		if b.eval.ShouldStop(resp, i) || (len(resp.ToolCalls) == 0 && resp.Content != "") {
			// Add final assistant message
			messages = append(messages, llm.Message{
				Role:      "assistant",
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			})
			break
		}

		// Record assistant message with tool calls
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Act — execute tool calls
		for _, tc := range resp.ToolCalls {
			result, mcpResp := b.executeTool(ctx, tc)
			if result != nil {
				allToolResults = append(allToolResults, *result)
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    result.Summary,
					ToolCallID: tc.ID,
				})
			}
			if mcpResp != nil {
				allMCPResponses = append(allMCPResponses, *mcpResp)
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    mcpResp.Summary,
					ToolCallID: tc.ID,
				})
			}
		}
	}

	// Parse final output
	output, err := b.eval.ParseOutput(ctx, messages, allToolResults, allMCPResponses)
	if err != nil {
		return &StageOutput{
			Error:        fmt.Sprintf("output parsing failed: %v", err),
			ToolResults:  allToolResults,
			MCPResponses: allMCPResponses,
			MissingPiece: ctx.MissingPiece,
		}, err
	}

	output.ToolResults = allToolResults
	output.MCPResponses = allMCPResponses
	output.MissingPiece = b.eval.DetermineMissingPiece(ctx, output)

	return output, nil
}

func (b *BaseAgent) buildToolSchemas(sk *skill.Config) []llm.ToolSchema {
	var schemas []llm.ToolSchema

	// Add suggested tools from skill
	if b.deps.Tools != nil {
		for _, toolName := range sk.ToolSuggestions {
			t, err := b.deps.Tools.Get(toolName)
			if err != nil {
				continue
			}
			schemas = append(schemas, llm.ToolSchema{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			})
		}
	}

	// Add MCP tools
	if b.deps.MCPServers != nil {
		for _, ts := range b.deps.MCPServers.ListAllTools() {
			schemas = append(schemas, llm.ToolSchema{
				Name:        ts.Name,
				Description: ts.Description,
				Parameters:  ts.Parameters,
			})
		}
	}

	// Auto-inject propose_sub_agents if a sub-agent with the same name as this
	// agent is registered. The schema's sub_agent enum is scoped to [self name],
	// so the agent can only propose sub-tasks for its own kind.
	if b.deps.SubAgents != nil && b.deps.Tools != nil {
		if _, err := b.deps.SubAgents.Get(string(b.name)); err == nil {
			if t, err := b.deps.Tools.Get("propose_sub_agents"); err == nil {
				params := t.Parameters()
				if psa, ok := t.(*tool.ProposeSubAgents); ok {
					params = psa.SchemaFor(string(b.name))
				}
				schemas = append(schemas, llm.ToolSchema{
					Name:        t.Name(),
					Description: t.Description(),
					Parameters:  params,
				})
			}
		}
	}

	return schemas
}

func (b *BaseAgent) buildInitialMessages(ctx *types.AgentContext, sk *skill.Config) []llm.Message {
	// Build full prompt context from agent context + skill config
	pc := agentctx.BuildPromptContext(ctx, sk)
	ctxMsgs := agentctx.ToMessages(pc)

	// Convert context.Message to llm.Message
	var messages []llm.Message
	for _, m := range ctxMsgs {
		messages = append(messages, llm.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// Append evaluator-specific instruction if provided
	if instruction := b.eval.BuildInitialPrompt(ctx, sk); instruction != "" {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: instruction,
		})
	}

	return messages
}

func (b *BaseAgent) executeTool(ctx *types.AgentContext, tc llm.ToolCall) (*types.ToolResult, *types.MCPResponse) {
	// Try local tool first
	if b.deps.Tools != nil {
		if _, err := b.deps.Tools.Get(tc.Name); err == nil {
			// The busCtx handed to a tool is intentionally narrow:
			// only RepoRoot/Branch/Commit (read-only env info) plus
			// Mutable (the shared, tool-writable region) are populated.
			// All other BusContext fields are zero-valued, so tools
			// physically cannot mutate stage-output state.
			busCtx := &types.BusContext{
				RepoRoot: ctx.RepoRoot,
				Branch:   ctx.Branch,
				Commit:   ctx.Commit,
				Mutable:  ctx.Mutable,
			}
			result, execErr := b.deps.Tools.Execute(busCtx, tc.Name, tc.Params)
			if execErr != nil {
				log.Printf("tool %s execution error: %v", tc.Name, execErr)
			}
			return &result, nil
		}
	}

	// Try MCP servers
	if b.deps.MCPServers != nil {
		for _, serverName := range b.deps.MCPServers.List() {
			server, _ := b.deps.MCPServers.Get(serverName)
			for _, t := range server.ListTools() {
				if t.Name == tc.Name {
					resp, err := server.CallTool(tc.Name, tc.Params)
					if err != nil {
						log.Printf("mcp %s.%s error: %v", serverName, tc.Name, err)
					}
					return nil, &resp
				}
			}
		}
	}

	log.Printf("tool not found: %s", tc.Name)
	return nil, nil
}
