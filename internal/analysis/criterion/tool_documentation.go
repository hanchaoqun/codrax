package criterion

import "github.com/hanchaoqun/codrax/internal/types"

func onlyToolDocumentation(env Env) bool {
	return env.IR != nil && types.ToolDocumentationOnlyRequested(&env.IR.RequestModel)
}

func evalToolDocumentationReady(env Env) Result {
	if env.IR == nil || env.IR.RequestModel.ToolDocumentationRequest == nil ||
		types.ValidateToolDocumentationRequest(&env.IR.RequestModel) != nil {
		return Result{Satisfied: false, Detail: "tool_documentation_ready: no valid documentation request domain"}
	}
	ready := env.ToolDocumentationReady || env.ArtifactReadiness != nil && env.ArtifactReadiness.ToolDocumentationReady
	if !ready {
		return Result{Satisfied: false, Detail: "tool_documentation_ready: current successful whole-document selection/completion receipt unavailable"}
	}
	return Result{Satisfied: true, Detail: "tool_documentation_ready: current whole-document selection is available; no source/runtime evidence authority granted"}
}
