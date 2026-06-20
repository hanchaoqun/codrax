package types

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestTaskNodeExecSlotsHaveNoRuntimeConsumersToday pins the M0c boundary from
// docs/design/ir_driven_execution_engine_delivery_20260621.md. The artifact
// slots are allowed to exist in serialized TaskGraph data, compiler templates,
// and schema wellformed checks, but they must not silently become runtime
// scheduling inputs before Phase 2 wires a typed artifact contract.
func TestTaskNodeExecSlotsHaveNoRuntimeConsumersToday(t *testing.T) {
	root := taskNodeExecSlotsRepoRoot(t)
	allowedExitArtifactRefs := map[string]bool{
		"internal/analysis/compiler/templates.go":     true,
		"internal/analysis/gate/gate.go":              true,
		"internal/analysis/gate/gate_test.go":         true,
		"internal/types/analysis_ir.go":               true,
		"internal/types/analysis_ir_test.go":          true,
		"internal/types/task_node_exec_slots_test.go": true,
	}
	allowedSelectorRefs := map[string]bool{
		"internal/analysis/compiler/compile_test.go":  true,
		"internal/analysis/gate/gate.go":              true,
		"internal/analysis/gate/gate_test.go":         true,
		"internal/types/task_node_exec_slots_test.go": true,
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
		if strings.Contains(text, "ExitArtifacts") && !allowedExitArtifactRefs[rel] {
			t.Errorf("TaskNode.ExitArtifacts referenced outside pending/gate boundary: %s", rel)
		}
		if taskNodeSlotSelectorLikelyRuntimeRead(text) && !allowedSelectorRefs[rel] {
			t.Errorf("TaskNode artifact slot selector read outside gate boundary: %s", rel)
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
		strings.Contains(text, "n.Outputs") ||
		strings.Contains(text, "n.ExitArtifacts")
}
