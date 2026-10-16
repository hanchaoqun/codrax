package types

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

func TestHMC253PipelineExportsProjectionAndJSONBoundary(t *testing.T) {
	root := tracecatalog.NewManagedExportRoot(t.TempDir())
	capability := tracecatalog.NewPipelineExports(root)
	bus := &BusContext{DeliveryRoot: root, PipelineExports: capability, Mutable: NewMutableState("current run")}
	clone := bus.ShallowClone()
	if clone.PipelineExports != capability {
		t.Fatal("same run clone lost pending capability")
	}
	ac := SubAgentContext(bus, &SubAgentRequest{SubAgent: "child"})
	if ac.PipelineExports != capability || ToolBusContext(ac, "child").PipelineExports != capability {
		t.Fatal("tool/subagent projection lost host capability")
	}
	wire, err := json.Marshal(bus)
	if err != nil {
		t.Fatal(err)
	}
	var restored BusContext
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.PipelineExports != nil || restored.DeliveryRoot != nil {
		t.Fatal("JSON recreated host delivery authority")
	}
}
