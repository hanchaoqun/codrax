package loopkernel

type LoopToolSurface string

const (
	LoopToolSurfaceNone         LoopToolSurface = "none"
	LoopToolSurfaceLocalization LoopToolSurface = "localization"
	LoopToolSurfaceVerification LoopToolSurface = "verification"
	LoopToolSurfaceRepair       LoopToolSurface = "repair"
	LoopToolSurfaceApproval     LoopToolSurface = "approval"
)

type LoopToolRoute struct {
	Action          LoopRecommendedAction `json:"action,omitempty"`
	Surface         LoopToolSurface       `json:"surface,omitempty"`
	ReasonCode      string                `json:"reason_code,omitempty"`
	ToolSuggestions []string              `json:"tool_suggestions,omitempty"`
}

func ToolRouteForLoopState(state LoopStateView) LoopToolRoute {
	action := RecommendLoopAction(state)
	route := ToolRouteForAction(action)
	route.ReasonCode = loopAdvanceReasonCode(state, action)
	return route
}

func ToolRouteForAction(action LoopRecommendedAction) LoopToolRoute {
	action = normalizeLoopRecommendedAction(action)
	route := LoopToolRoute{
		Action:     action,
		Surface:    LoopToolSurfaceNone,
		ReasonCode: "loop_tool_route_none",
	}
	switch action {
	case LoopActionLocalize:
		route.Surface = LoopToolSurfaceLocalization
		route.ReasonCode = "loop_tool_route_localization"
		route.ToolSuggestions = []string{"repo_map", "read_file", "list_files", "grep"}
	case LoopActionVerify, LoopActionAddProof:
		route.Surface = LoopToolSurfaceVerification
		route.ReasonCode = "loop_tool_route_verification"
		route.ToolSuggestions = []string{"run_tests"}
	case LoopActionRepair:
		route.Surface = LoopToolSurfaceRepair
		route.ReasonCode = "loop_tool_route_repair"
		route.ToolSuggestions = []string{"emit_change_plan", "read_file"}
	case LoopActionAskApproval:
		route.Surface = LoopToolSurfaceApproval
		route.ReasonCode = "loop_tool_route_approval"
	case LoopActionBlock:
		route.ReasonCode = "loop_tool_route_block"
	case LoopActionContinue:
		route.ReasonCode = "loop_tool_route_continue"
	default:
		route.Action = LoopActionContinue
		route.ReasonCode = "loop_tool_route_continue"
	}
	return route
}
