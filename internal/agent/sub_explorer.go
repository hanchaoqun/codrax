package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// SubExplorer is a SubAgent that scans files within a given scope
// and discovers facts about the codebase.
type SubExplorer struct {
	base *BaseAgent
}

// NewSubExplorer creates a SubExplorer backed by a BaseAgent with its own evaluator.
func NewSubExplorer(deps *Dependencies) *SubExplorer {
	eval := &subExplorerEvaluator{}
	base := NewBaseAgent("sub_explorer", deps, eval)
	return &SubExplorer{base: base}
}

func (s *SubExplorer) Name() string {
	return "explorer"
}

func (s *SubExplorer) Run(req *types.SubAgentRequest) (*types.SubAgentResult, error) {
	// Shared read: build read-only AgentContext from shared BusContext
	agentCtx := ctxbuilder.BuildSubAgentContext(req.ReadView, req)

	// Build a scan-oriented skill
	sk := &skill.Config{
		Name: "sub_explore",
		Goal: fmt.Sprintf("Explore files in scope [%s] and discover facts", strings.Join(req.Scope, ", ")),
		Workflow: []string{
			"List files in the scoped directories",
			"Read key files to understand structure",
			"Identify types, functions, interfaces, and dependencies",
			"Report discovered facts",
		},
		ToolSuggestions: []string{"read_file", "list_files", "grep"},
		OutputFormat:    "List of discovered facts as JSON",
		Prohibitions:    []string{"Do not modify any files", "Do not explore outside the given scope"},
	}

	// Isolated write: Execute returns StageOutput without touching BusContext
	output, err := s.base.Execute(agentCtx, sk)
	if err != nil {
		return &types.SubAgentResult{
			RequestID: req.ID,
			SubAgent:  req.SubAgent,
			Error:     err.Error(),
		}, err
	}

	// Convert StageOutput to SubAgentResult (private write buffer)
	return &types.SubAgentResult{
		RequestID: req.ID,
		SubAgent:  req.SubAgent,
		Output:    output.Data,
		Facts:     output.NewFacts,
		Tools:     output.ToolResults,
		MCPResps:  output.MCPResponses,
		Signals:   output.SignalUpdates,
	}, nil
}

// subExplorerEvaluator implements Evaluator for the SubExplorer's BaseAgent.
type subExplorerEvaluator struct{}

func (e *subExplorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	return "Scan the files in scope. Discover key types, functions, interfaces, and dependencies. Report your findings as structured facts."
}

func (e *subExplorerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	return len(resp.ToolCalls) == 0
}

func (e *subExplorerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, mcpResponses []types.MCPResponse) (*StageOutput, error) {
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	// Extract facts from successful tool results
	var facts []types.RepoFact
	for _, r := range toolResults {
		if r.Success {
			facts = append(facts, types.RepoFact{
				Key:        r.ToolName,
				Value:      r.Summary,
				Source:     r.RawRef,
				Confidence: 0.8,
			})
		}
	}

	signals := &types.ExecutionSignals{HasEnoughFacts: len(facts) > 0}

	return &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent)),
		NewFacts:      facts,
		SignalUpdates: signals,
	}, nil
}

func (e *subExplorerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output.SignalUpdates != nil && output.SignalUpdates.HasEnoughFacts {
		return types.MissingPlan
	}
	return types.MissingFacts
}
