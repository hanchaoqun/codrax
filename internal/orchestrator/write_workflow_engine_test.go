package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWriteWorkflowEngineDefaultsLegacy(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	if got := o.WriteWorkflowEngine(); got != types.WriteWorkflowEngineLegacy {
		t.Fatalf("WriteWorkflowEngine = %q, want legacy", got)
	}
	if o.WriteWorkflowControllerEnabled() {
		t.Fatal("controller engine must be opt-in")
	}
}

func TestWriteWorkflowEngineControllerOptIn(t *testing.T) {
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, nil, nil, nil)
	if got := o.WriteWorkflowEngine(); got != types.WriteWorkflowEngineController {
		t.Fatalf("WriteWorkflowEngine = %q, want controller", got)
	}
	if !o.WriteWorkflowControllerEnabled() {
		t.Fatal("controller engine should be enabled for explicit controller setting")
	}
}
