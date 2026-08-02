package orchestrator

import "github.com/hanchaoqun/codrax/internal/agent"

// analyzeStageOutputUsable is the normal analyze-loop success join. A non-nil
// pointer alone is insufficient: an empty IR would otherwise be accepted even
// though the read scheduler must immediately fail closed. Quality/coherence
// gates still report through StageOutput.Error; this helper does not duplicate
// them. Approved-plan stub IRs bypass the normal loop before this join.
func analyzeStageOutputUsable(out *agent.StageOutput) bool {
	return out != nil && out.Error == "" && out.AnalysisIR != nil && len(out.AnalysisIR.TaskGraph.Nodes) > 0
}

func analyzeStageOutputFailure(dispatchErr error, out *agent.StageOutput) string {
	if dispatchErr != nil {
		return dispatchErr.Error()
	}
	if out == nil {
		return "analyzer returned no result"
	}
	if out.Error != "" {
		return out.Error
	}
	if out.AnalysisIR != nil && len(out.AnalysisIR.TaskGraph.Nodes) == 0 {
		return "analyzer returned a structurally empty analysis result: no executable steps"
	}
	return "analyzer returned no usable analysis result"
}
