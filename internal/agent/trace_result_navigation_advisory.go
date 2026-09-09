package agent

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// appendTraceResultReturnNavigation runs only after the existing boundary has
// rejected a call. The shared resolver supplies navigation, never permission;
// neither the original rejection nor its structured repair lane is replaced.
func appendTraceResultReturnNavigation(ctx *types.AgentContext, tc llm.ToolCall, result *types.ToolResult) *types.ToolResult {
	if ctx == nil || result == nil || result.Success {
		return result
	}
	switch types.CanonicalToolName(tc.Name) {
	case "read_file", "grep":
	default:
		return result
	}
	var params struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(tc.Params, &params) != nil || params.Path == "" {
		return result
	}
	advice := tool.ArtifactReadReturnNavigationAdvisory(types.ToolBusContext(ctx, ctx.AgentName), params.Path)
	if advice == "" {
		return result
	}
	result.Summary += "\n" + advice
	if result.Repair != nil {
		result.Repair.Hint += "\n" + advice
	}
	return result
}
