package tool

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ProposeSubAgents is a built-in tool that allows an Agent to propose sub-agent
// task decomposition. The tool validates the proposal structure; actual execution
// is handled by SubAgentRuntime in the orchestrator layer.
//
// This tool is name-scoped: the Agent layer auto-injects it only for agents
// whose name matches a registered sub-agent, and the schema's sub_agent enum
// is restricted to that single name.
type ProposeSubAgents struct {
	ReadOnly
	NonEvidenceTool
}

type proposeSubAgentsParams struct {
	Reason     string                `json:"reason"`
	Goal       string                `json:"goal"`
	SubTasks   []proposeSubTaskParam `json:"sub_tasks"`
	ReduceHint string                `json:"reduce_hint,omitempty"`
}

type proposeSubTaskParam struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Objective   string   `json:"objective"`
	Scope       []string `json:"scope"`
	Constraints []string `json:"constraints,omitempty"`
}

// NewProposeSubAgents creates the tool.
func NewProposeSubAgents() *ProposeSubAgents {
	return &ProposeSubAgents{}
}

func (t *ProposeSubAgents) Name() string { return "propose_sub_agents" }
func (t *ProposeSubAgents) Description() string {
	return "Decompose the current question into independent parallel sub-tasks, each investigating a disjoint scope. " +
		"USE WHEN: the question asks about MULTIPLE unrelated modules/packages/subsystems that can be explored in parallel " +
		"(e.g., 'compare the X implementation across module A and module B', 'enumerate all handlers in package P and package Q', " +
		"'how does feature F manifest in frontend code and backend code'). " +
		"Each sub_task declares its own scope (file/directory prefixes) so sub-agents do not overlap. " +
		"DO NOT USE when the question is a single-target investigation (follow a call chain, explain one mechanism, find one config value) — " +
		"sequential read_file + grep is faster than fanning out. " +
		"DO NOT USE when sub-tasks would share files — results will collide. " +
		"Call this EARLY, before you start reading files, so the sub-agents do not duplicate work you already did."
}

// Parameters returns a schema with an empty enum. The Agent layer calls
// SchemaFor(agentName) to get a per-agent scoped schema instead.
func (t *ProposeSubAgents) Parameters() json.RawMessage {
	return t.SchemaFor("")
}

// SchemaFor returns the tool schema scoped to a specific agent name.
// The sub_agent field is omitted from the schema because sub-tasks are
// always routed to a SubAgent of the same name as the calling Agent;
// it is injected automatically at execution time.
func (t *ProposeSubAgents) SchemaFor(agentName string) json.RawMessage {
	_ = agentName // kept for API symmetry and future per-agent customization
	return json.RawMessage(`{
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
          "objective":   {"type": "string", "description": "What this sub-task should accomplish"},
          "scope":       {"type": "array", "items": {"type": "string"}, "description": "File/directory paths to constrain the sub-agent"},
          "constraints": {"type": "array", "items": {"type": "string"}, "description": "Additional constraints"}
        },
        "required": ["id", "title", "objective", "scope"]
      }
    },
    "reduce_hint": {"type": "string", "description": "Hint for how to merge results from all sub-tasks"}
  },
  "required": ["reason", "goal", "sub_tasks"]
}`)
}

func (t *ProposeSubAgents) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	// Validate structure; the orchestrator fills in sub_agent at extraction time.
	var p proposeSubAgentsParams
	normalized, decodeFailure, err := decodeStrictToolParams(t.Name(), params, t.Parameters(), &p, nil)
	if err != nil {
		return *decodeFailure, err
	}
	proposal := types.SubAgentProposal{
		Reason:     p.Reason,
		Goal:       p.Goal,
		ReduceHint: p.ReduceHint,
		SubTasks:   make([]types.SubTask, 0, len(p.SubTasks)),
	}
	for _, task := range p.SubTasks {
		proposal.SubTasks = append(proposal.SubTasks, types.SubTask{
			ID:          task.ID,
			Title:       task.Title,
			Objective:   task.Objective,
			Scope:       append([]string(nil), task.Scope...),
			Constraints: append([]string(nil), task.Constraints...),
		})
	}
	if len(proposal.SubTasks) == 0 {
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "proposal has no sub_tasks", Timestamp: time.Now()}, fmt.Errorf("proposal has no sub_tasks")
	}
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   string(normalized),
		Timestamp: time.Now(),
	}, nil
}
