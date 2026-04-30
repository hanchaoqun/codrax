package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// applyPatchTestBus builds a minimal BusContext seeded with a
// ChangePlan that includes the requested entries on TargetPaths.
func applyPatchTestBus(t *testing.T, repoRoot string, changes []types.FileChange) *types.BusContext {
	t.Helper()
	mu := types.NewMutableState("apply rename test")
	plan := &types.ChangePlan{
		ID:      "rename-test",
		Changes: changes,
	}
	for _, c := range changes {
		plan.TargetPaths = append(plan.TargetPaths, c.Path)
		if c.NewPath != "" {
			plan.TargetPaths = append(plan.TargetPaths, c.NewPath)
		}
	}
	mu.SetChangePlan(plan)
	return &types.BusContext{
		Mutable:  mu,
		RepoRoot: repoRoot,
	}
}

// TestApplyPatch_RenameHappyPath: source exists, destination does
// not, kind=rename moves the file and marks both paths applied.
func TestApplyPatch_RenameHappyPath(t *testing.T) {
	repo := t.TempDir()
	src := "old/x.go"
	dst := "new/x.go"
	if err := os.MkdirAll(filepath.Join(repo, "old"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, src), []byte("package x\n"), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}
	bus := applyPatchTestBus(t, repo, []types.FileChange{{
		Path:    src,
		Kind:    "rename",
		NewPath: dst,
	}})
	tool := &ApplyPatch{}
	params, _ := json.Marshal(map[string]string{"path": src, "kind": "rename"})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("rename rejected: %s", res.Summary)
	}
	if _, err := os.Stat(filepath.Join(repo, src)); !os.IsNotExist(err) {
		t.Errorf("source path should be gone after rename; stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, dst)); err != nil {
		t.Errorf("destination should exist after rename; stat err=%v", err)
	}
	applied := bus.Mutable.WriteClosure().AppliedSet()
	if !applied[src] || !applied[dst] {
		t.Errorf("AppliedSet should contain both paths; got %+v", applied)
	}
}

// TestApplyPatch_RenameMissingSource rejects when the old path
// doesn't exist.
func TestApplyPatch_RenameMissingSource(t *testing.T) {
	repo := t.TempDir()
	bus := applyPatchTestBus(t, repo, []types.FileChange{{
		Path:    "missing.go",
		Kind:    "rename",
		NewPath: "renamed.go",
	}})
	tool := &ApplyPatch{}
	params, _ := json.Marshal(map[string]string{"path": "missing.go", "kind": "rename"})
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Fatal("rename of nonexistent source should reject")
	}
}

// TestApplyPatch_RenameDestinationExists rejects when destination
// already exists.
func TestApplyPatch_RenameDestinationExists(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "src.go"), []byte("a"), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "dst.go"), []byte("b"), 0o644); err != nil {
		t.Fatalf("seed dst: %v", err)
	}
	bus := applyPatchTestBus(t, repo, []types.FileChange{{
		Path:    "src.go",
		Kind:    "rename",
		NewPath: "dst.go",
	}})
	tool := &ApplyPatch{}
	params, _ := json.Marshal(map[string]string{"path": "src.go", "kind": "rename"})
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Fatal("rename to existing destination should reject")
	}
}

// TestApplyPatch_RenameEmptyNewPath rejects an emit that names
// rename without a new_path.
func TestApplyPatch_RenameEmptyNewPath(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "src.go"), []byte("a"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bus := applyPatchTestBus(t, repo, []types.FileChange{{
		Path:    "src.go",
		Kind:    "rename",
		NewPath: "", // missing
	}})
	tool := &ApplyPatch{}
	params, _ := json.Marshal(map[string]string{"path": "src.go", "kind": "rename"})
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Fatal("rename with empty new_path should reject")
	}
}

// TestApplyPatch_RenameTraversalRejected pins the security check —
// a NewPath that escapes RepoRoot via .. is refused.
func TestApplyPatch_RenameTraversalRejected(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "x.go"), []byte("a"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bus := applyPatchTestBus(t, repo, []types.FileChange{{
		Path:    "x.go",
		Kind:    "rename",
		NewPath: "../escaped.go",
	}})
	tool := &ApplyPatch{}
	params, _ := json.Marshal(map[string]string{"path": "x.go", "kind": "rename"})
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Fatal("rename to ..-prefixed path should reject (escapes RepoRoot)")
	}
}

// TestValidatePlanGraph_RenameRequiresNewPath: emit-time validation
// rejects rename without new_path.
func TestValidatePlanGraph_RenameRequiresNewPath(t *testing.T) {
	changes := []types.FileChange{
		{Path: "x.go", Kind: "rename", NewPath: ""},
	}
	if rej := validatePlanGraphIntegrity(changes); rej == "" {
		t.Fatal("rename without new_path must be rejected by validatePlanGraphIntegrity")
	}
}

// TestValidatePlanGraph_RenameEqualNewPath: rename with new_path ==
// path is rejected.
func TestValidatePlanGraph_RenameEqualNewPath(t *testing.T) {
	changes := []types.FileChange{
		{Path: "x.go", Kind: "rename", NewPath: "x.go"},
	}
	if rej := validatePlanGraphIntegrity(changes); rej == "" {
		t.Fatal("rename to same path must be rejected")
	}
}

// TestValidatePlanGraph_NewPathOnNonRename: new_path on a kind that
// is not rename is rejected.
func TestValidatePlanGraph_NewPathOnNonRename(t *testing.T) {
	changes := []types.FileChange{
		{Path: "x.go", Kind: "modify", NewPath: "y.go", NewContent: "package x\n"},
	}
	if rej := validatePlanGraphIntegrity(changes); rej == "" {
		t.Fatal("new_path on kind=modify must be rejected")
	}
}

// TestValidatePlanGraph_RenameDestCollision: another change in the
// plan claiming the rename destination is rejected.
func TestValidatePlanGraph_RenameDestCollision(t *testing.T) {
	changes := []types.FileChange{
		{Path: "old.go", Kind: "rename", NewPath: "new.go"},
		{Path: "new.go", Kind: "modify", NewContent: "package x\n"},
	}
	if rej := validatePlanGraphIntegrity(changes); rej == "" {
		t.Fatal("rename dest colliding with another change's path must be rejected")
	}
}

// TestApplyPatch_LegalKindEnumIncludesRename pins the kind whitelist
// at the legality check layer so apply_patch accepts kind=rename.
func TestApplyPatch_LegalKindEnumIncludesRename(t *testing.T) {
	if !isLegalChangeKind("rename") {
		t.Error("isLegalChangeKind must accept 'rename'")
	}
}
