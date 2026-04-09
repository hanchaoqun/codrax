package tool

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hanchaoqun/design/internal/types"
)

// ProposeSubAgents is a built-in tool that allows an LLM to propose sub-agent
// task decomposition. The tool validates the proposal structure; actual execution
// is handled by SubAgentRuntime in the orchestrator layer.
type ProposeSubAgents struct {
	subAgentNames []string
}

// NewProposeSubAgents creates the tool with the given set of valid sub-agent names.
func NewProposeSubAgents(names []string) *ProposeSubAgents {
	return &ProposeSubAgents{subAgentNames: names}
}

func (t *ProposeSubAgents) Name() string { return "propose_sub_agents" }
func (t *ProposeSubAgents) Description() string {
	return "Propose splitting the current task into parallel sub-tasks, each executed by a SubAgent. Use when the task can be decomposed into independent sub-tasks that benefit from parallel execution."
}

func (t *ProposeSubAgents) Parameters() json.RawMessage {
	namesJSON, _ := json.Marshal(t.subAgentNames)
	return json.RawMessage(fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "reason":      {"type": "string", "description": "Why decomposing into sub-agents is beneficial for this task"},
    "goal":        {"type": "string", "description": "Expected outcome after all sub-tasks complete"},
    "sub_tasks": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id":          {"type": "string", "description": "Unique ID within this proposal (e.g. st-1)"},
          "title":       {"type": "string", "description": "Short description of the sub-task"},
          "sub_agent":   {"type": "string", "enum": %s, "description": "Which SubAgent to run"},
          "objective":   {"type": "string", "description": "What this sub-task should accomplish"},
          "scope":       {"type": "array", "items": {"type": "string"}, "description": "File/directory paths to constrain the sub-agent"},
          "constraints": {"type": "array", "items": {"type": "string"}, "description": "Additional constraints"}
        },
        "required": ["id", "title", "sub_agent", "objective", "scope"]
      }
    },
    "reduce_hint": {"type": "string", "description": "Hint for how to merge results from all sub-tasks"}
  },
  "required": ["reason", "goal", "sub_tasks"]
}`, string(namesJSON)))
}

func (t *ProposeSubAgents) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var proposal types.SubAgentProposal
	if err := json.Unmarshal(params, &proposal); err != nil {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: err.Error(), Timestamp: time.Now()}, err
	}
	if len(proposal.SubTasks) == 0 {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "proposal has no sub_tasks", Timestamp: time.Now()}, fmt.Errorf("proposal has no sub_tasks")
	}
	// Return raw proposal JSON as Summary for orchestrator extraction
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   string(params),
		Timestamp: time.Now(),
	}, nil
}
