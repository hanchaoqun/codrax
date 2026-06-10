package types

const (
	WriteWorkflowEngineLegacy     = "legacy"
	WriteWorkflowEngineController = "controller"
)

// NormalizeWriteWorkflowEngine keeps the historical yaml field parseable while
// resolving every value to the controller-first write engine. The old legacy
// write scheduler is no longer a public runtime choice.
func NormalizeWriteWorkflowEngine(raw string) string {
	_ = raw
	return WriteWorkflowEngineController
}

func IsControllerWriteWorkflowEngine(raw string) bool {
	return NormalizeWriteWorkflowEngine(raw) == WriteWorkflowEngineController
}
