package types

import "strings"

const (
	WriteWorkflowEngineLegacy     = "legacy"
	WriteWorkflowEngineController = "controller"
)

// NormalizeWriteWorkflowEngine maps operator config to the small typed engine
// set. Unknown values fall back to legacy so new controller code never changes
// stable write-mode behavior by accident.
func NormalizeWriteWorkflowEngine(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case WriteWorkflowEngineController:
		return WriteWorkflowEngineController
	case WriteWorkflowEngineLegacy, "":
		return WriteWorkflowEngineLegacy
	default:
		return WriteWorkflowEngineLegacy
	}
}

func IsControllerWriteWorkflowEngine(raw string) bool {
	return NormalizeWriteWorkflowEngine(raw) == WriteWorkflowEngineController
}
