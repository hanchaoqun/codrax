package types

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestTaskNodeExecSlotsHaveOnlyExplicitContractConsumers pins the M2c boundary
// from docs/design/ir_driven_execution_engine_delivery_20260621.md. Inputs and
// Outputs may be read only by compiler/gate tests and the explicit immutable
// TaskArtifactContract projection. Runtime artifact IDs must not be hidden in
// TaskNode.
func TestTaskNodeExecSlotsHaveOnlyExplicitContractConsumers(t *testing.T) {
	root := taskNodeExecSlotsRepoRoot(t)
	allowedSelectorRefs := map[string]bool{
		"internal/analysis/compiler/compile_test.go":    true,
		"internal/analysis/gate/gate.go":                true,
		"internal/analysis/gate/gate_test.go":           true,
		"internal/types/task_artifact_contract.go":      true,
		"internal/types/task_artifact_contract_test.go": true,
		"internal/types/task_node_exec_slots_test.go":   true,
	}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		if strings.Contains(text, "Exit"+"Artifacts") {
			t.Errorf("TaskNode.%s must not be reintroduced: %s", "Exit"+"Artifacts", rel)
		}
		if taskNodeSlotSelectorLikelyRuntimeRead(text) && !allowedSelectorRefs[rel] {
			t.Errorf("TaskNode artifact slot selector read outside explicit contract boundary: %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
}

func taskNodeExecSlotsRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func taskNodeSlotSelectorLikelyRuntimeRead(text string) bool {
	return strings.Contains(text, "n.Inputs") ||
		strings.Contains(text, "n.Outputs")
}
