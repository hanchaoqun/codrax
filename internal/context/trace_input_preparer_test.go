package context

import (
	stdcontext "context"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/types"
)

type contextTraceInputPreparer struct{}

func (*contextTraceInputPreparer) Prepare(stdcontext.Context, string) (*attachment.TraceMaterial, error) {
	return nil, nil
}
func (*contextTraceInputPreparer) PreparedMaterials() []*attachment.TraceMaterial { return nil }

func TestBuildAgentContextPreservesRunTraceInputPreparer(t *testing.T) {
	p := &contextTraceInputPreparer{}
	bus := &types.BusContext{TraceInputPreparer: p, Mutable: types.NewMutableState("trace")}
	for _, stage := range []types.PipelineStage{types.StageAnalyze, types.StageExplore, types.StageExtract, types.StageFinalize, types.StagePlan} {
		ac := BuildAgentContext(bus, types.AgentExplorer, stage)
		if ac.TraceInputPreparer != p || types.ToolBusContext(ac, types.AgentExplorer).TraceInputPreparer != p {
			t.Fatalf("%s dropped or replaced the shared trace preparer", stage)
		}
	}
	if ac := BuildSubAgentContext(bus, &types.SubAgentRequest{SubAgent: "worker"}); ac.TraceInputPreparer != p {
		t.Fatal("sub-agent builder dropped shared trace preparer")
	}
}
