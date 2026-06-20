package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSetReasoningObserverInstallsRuntimeSink(t *testing.T) {
	orch := New(types.PipelineSettings{}, agent.NewRegistry(), skill.NewRegistry(), agent.NewSubAgentRegistry())
	collector := reasoninggraph.NewEventCollector("runtime-graph")

	orch.SetReasoningObserver(collector)

	if orch.reasoningObserver != collector {
		t.Fatalf("reasoning observer not installed on orchestrator")
	}
}
