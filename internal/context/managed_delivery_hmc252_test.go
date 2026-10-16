package context

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC252ManagedDeliveryHostContextProjections(t *testing.T) {
	root := tracecatalog.NewManagedExportRoot(t.TempDir())
	bus := &types.BusContext{DeliveryRoot: root, Mutable: types.NewMutableState("export")}
	agent := BuildAgentContext(bus.ShallowClone(), types.AgentExplorer, types.StageExplore)
	if agent.DeliveryRoot != root || agent.ShallowClone().DeliveryRoot != root {
		t.Fatal("build/fork lost shared host scope")
	}
	toolBus := types.ToolBusContext(agent, types.AgentExplorer)
	if toolBus.DeliveryRoot != root || types.SubAgentContext(toolBus, nil).DeliveryRoot != root {
		t.Fatal("tool/subagent lost shared host scope")
	}
	data, err := json.Marshal(bus)
	if err != nil {
		t.Fatal(err)
	}
	var decoded types.BusContext
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.DeliveryRoot != nil {
		t.Fatal("JSON restored host delivery scope")
	}
	data, err = json.Marshal(agent)
	if err != nil {
		t.Fatal(err)
	}
	var decodedAgent types.AgentContext
	if err := json.Unmarshal(data, &decodedAgent); err != nil {
		t.Fatal(err)
	}
	if decodedAgent.DeliveryRoot != nil {
		t.Fatal("agent JSON restored host delivery scope")
	}
}
