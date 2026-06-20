package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWriteWorkflowEngineDefaultsController(t *testing.T) {
	o := New(types.PipelineSettings{}, nil, nil, nil)
	if got := o.WriteWorkflowEngine(); got != types.WriteWorkflowEngineController {
		t.Fatalf("WriteWorkflowEngine = %q, want controller", got)
	}
	if !o.WriteWorkflowControllerEnabled() {
		t.Fatal("controller engine must be the default write workflow engine")
	}
}

func TestWriteWorkflowEngineLegacySettingIsCompatibilityAlias(t *testing.T) {
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineLegacy}, nil, nil, nil)
	if got := o.WriteWorkflowEngine(); got != types.WriteWorkflowEngineController {
		t.Fatalf("legacy setting resolved to %q, want controller", got)
	}
	if !o.WriteWorkflowControllerEnabled() {
		t.Fatal("controller engine should stay enabled for legacy compatibility setting")
	}
}
