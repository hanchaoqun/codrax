package orchestrator

import "strings"

func isReviewerNoToolCallError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "LLM returned no tool_call")
}
