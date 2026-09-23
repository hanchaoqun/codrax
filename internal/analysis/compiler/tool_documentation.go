package compiler

import "github.com/hanchaoqun/codrax/internal/types"

// This template requests a real documentation read, not a repository search
// for the host tool's implementation. It uses the normal explore/extract/final
// stage types and budgets; documentation never becomes a source evidence row.
func templateToolDocumentation(rm types.RequestModel) Output {
	ready := types.Criterion{Kind: types.CritToolDocumentationReady}
	read := types.TaskNode{
		ID: nodeID(0, "documentation"), Type: types.NodeEvidence,
		Objective: "Read the host tool documentation needed to answer the requested capabilities, contracts, units, prerequisites, and usage. Do not substitute target-repository implementation or unobserved runtime facts.",
		Inputs:    []string{"user_question", "tool_documentation_request"}, Outputs: []string{"selected_tool_documentation"},
		SuccessCriteria: []types.Criterion{ready},
	}
	extract := types.TaskNode{
		ID: nodeID(1, "extract"), Type: types.NodeExtract,
		Objective: "Distill the accepted, selected tool documentation without inventing source evidence, measurements, or capabilities absent from the documentation.",
		Inputs:    []string{"selected_tool_documentation"}, Outputs: []string{"structured_support"},
		EntryConditions: []types.Criterion{{Kind: types.CritExtractInputReady}},
	}
	final := types.TaskNode{
		ID: nodeID(2, "finalize"), Type: types.NodeFinalize,
		Objective: "Answer from the retained documentation, distinguish availability from observations, and disclose undocumented or unavailable details without fake file citations.",
		Inputs:    []string{"selected_tool_documentation", "structured_support"}, Outputs: []string{"answer_document"},
		EntryConditions: []types.Criterion{ready}, SuccessCriteria: []types.Criterion{ready},
	}
	return Output{
		TaskGraph: types.TaskGraph{Nodes: []types.TaskNode{read, extract, final}, Edges: chain(read.ID, extract.ID, final.ID),
			ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: TmplRetryBudgetMedium, CriticalPath: []string{read.ID, extract.ID, final.ID}}},
		EvidencePlan:   types.EvidencePlan{StopConditions: []types.StopCondition{{Kind: types.CritContractSatisfied}, {Kind: types.CritBudgetExhausted}}},
		AnswerContract: types.AnswerContract{Language: rm.Language},
	}
}

func requireMixedToolDocumentation(out *Output, rm types.RequestModel) {
	if rm.ToolDocumentationRequest == nil || rm.ToolDocumentationRequest.Scope != types.ToolDocumentationRequestMixed || types.ValidateToolDocumentationRequest(&rm) != nil {
		return
	}
	ready := types.Criterion{Kind: types.CritToolDocumentationReady}
	for i := range out.TaskGraph.Nodes {
		if out.TaskGraph.Nodes[i].Type == types.NodeFinalize {
			out.TaskGraph.Nodes[i].SuccessCriteria = append(out.TaskGraph.Nodes[i].SuccessCriteria, ready)
		}
	}
}
