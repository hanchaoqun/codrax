package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// explicitRuntimeTraceArtifactOnlyRequest reports whether the current request
// names a runtime trace artifact path and asks about that artifact itself,
// without asking to verify or explain it against the current checkout.
//
// This is deliberately a soft-prompt routing helper, not a gate: false
// negatives only keep the older repo-first behavior. The conservative
// current-source cue list prevents the helper from suppressing source
// exploration for mixed trace+source questions.
func explicitRuntimeTraceArtifactOnlyRequest(ctx *types.AgentContext) bool {
	raw := runtimeTraceRequestText(ctx)
	if !requestNamesRuntimeTraceArtifact(raw) {
		return false
	}
	return !requestMentionsCurrentSourceLane(raw)
}

func runtimeTraceRequestText(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	parts := []string{types.StripConversationPrefix(ctx.Objective)}
	if ctx.AnalysisIR != nil {
		parts = append(parts, ctx.AnalysisIR.RequestModel.RawRequest)
	}
	return strings.Join(parts, "\n")
}

func requestMentionsCurrentSourceLane(raw string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return false
	}
	for _, cue := range []string{
		"源码",
		"源代码",
		"当前代码",
		"仓库代码",
		"当前仓库",
		"当前工程",
		"对照代码",
		"对照源码",
		"结合代码",
		"结合源码",
		"代码实现",
		"源码实现",
		"实现代码",
		"source code",
		"current source",
		"current code",
		"current checkout",
		"current repo",
		"repository code",
		"repo code",
		"codebase",
		"implementation in this repo",
		"against the repo",
		"against current source",
		"compare with current source",
	} {
		if strings.Contains(raw, cue) {
			return true
		}
	}
	return false
}
