package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Dispatch-local presentation state, never an admission source. Retain earlier
// tool failures as evidence of their own turn while making a changed current
// schema explicit. The message is request-only, so it cannot accumulate as a
// second, durable tool allowlist or leak into another stage/dispatch.
type toolSurfaceTransition struct {
	stage       types.PipelineStage
	names       []string
	initialized bool
}

func (s *toolSurfaceTransition) message(ctx *types.AgentContext, base, effective []llm.ToolSchema) (llm.Message, bool) {
	stage := types.PipelineStage("")
	if ctx != nil {
		stage = ctx.Stage
	}
	names := sortedToolSchemaNames(effective)
	changed := s.initialized && stage == s.stage && !sameStringSlice(s.names, names)
	s.stage, s.names, s.initialized = stage, names, true
	// Preserve the existing, more specific filter/emit-only instruction. A
	// transition never adds a second surface explanation in the same request.
	if message, ok := currentTurnToolSurfaceMessage(base, effective); ok {
		return message, true
	}
	if !changed || len(names) == 0 || stage == "" {
		return llm.Message{}, false
	}
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = "`" + name + "`"
	}
	return llm.Message{Role: "system", Content: fmt.Sprintf(
		"[current tool surface changed]\nCallable tools for this turn: %s. Earlier tool-availability rejections describe their earlier turn only; the current tool schema is authoritative. Use its exact names and parameters. This update does not change source access, execution admission, or the evidence status of earlier results.",
		strings.Join(quoted, ", "))}, true
}
