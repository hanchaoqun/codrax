package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/types"
)

type multiRepoFocusEvaluator struct {
	emitSeen bool
}

func (e *multiRepoFocusEvaluator) BuildInitialInstruction(ctx *types.AgentContext, _ *skill.Config) string {
	e.emitSeen = false
	if ctx == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("Select the minimal sub-repo focus for the CURRENT user request before source exploration starts.\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Use only the topology rows below and the current request.\n")
	b.WriteString("- Do not inspect files, do not call repo_map/grep/read_file, and do not answer the user.\n")
	b.WriteString("- Return 1-2 sub-repos by default; use more only when the request clearly spans them.\n")
	b.WriteString("- Use source=model_recommended unless the current request literally names an exact RootRel/path from the topology; only then use source=user_explicit_in_request.\n")
	b.WriteString("- Call emit_multi_repo_focus once with exact root_rel values from the topology.\n\n")
	if mg, _ := ctx.MultiGraph.(*multigraph.MultiGraph); mg != nil && mg.Topology() != nil {
		fmt.Fprintf(&b, "Active cap: %d\n", mg.Cap())
		b.WriteString("Topology:\n")
		for _, sr := range mg.Topology().Repos {
			langs := strings.Join(sr.PrimaryLangs, ",")
			if langs == "" {
				langs = "-"
			}
			fmt.Fprintf(&b, "- root_rel=%s files=%d langs=%s\n", sr.RootRel, sr.FileCount, langs)
		}
	}
	return b.String()
}

func (e *multiRepoFocusEvaluator) ShouldStop(_ llm.Response, _ int) bool {
	return e.emitSeen
}

func (e *multiRepoFocusEvaluator) ParseOutput(ctx *types.AgentContext, _ []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	if ctx == nil || ctx.Mutable == nil {
		return &StageOutput{MissingPiece: types.MissingUnderstanding, Error: "multi-repo focus selector has no mutable context"}, nil
	}
	if ctx.Mutable.MultiRepoFocusDecision() == nil {
		return &StageOutput{MissingPiece: types.MissingUnderstanding, Error: "multi-repo focus selector did not emit a usable recommendation"}, nil
	}
	return &StageOutput{MissingPiece: types.MissingNone}, nil
}

func (e *multiRepoFocusEvaluator) DetermineMissingPiece(_ *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output != nil && output.Error == "" {
		return types.MissingNone
	}
	return types.MissingUnderstanding
}

func (e *multiRepoFocusEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.LastToolResult != nil && obs.LastToolResult.ToolName == "emit_multi_repo_focus" && obs.LastToolResult.Success {
		e.emitSeen = true
		return LoopSignal{Progress: true, StopRequested: true, StopReason: "multi_repo_focus_selected"}
	}
	return LoopSignal{}
}

// NewMultiRepoFocusAgent constructs the lightweight focus selector used
// before analyzer only for multi-repo/no-focus read-mode runs.
func NewMultiRepoFocusAgent(deps *Dependencies) Agent {
	eval := &multiRepoFocusEvaluator{}
	d := *deps
	if d.MaxIterations == 0 || d.MaxIterations > 3 {
		d.MaxIterations = 3
	}
	d.LoopPolicy.MinInjectInterval = 1
	return NewBaseAgent(types.AgentMultiRepoFocus, &d, eval)
}
