package tool

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitChangesToFileChangesCanonicalizesPathIdentity(t *testing.T) {
	changes := emitChangesToFileChanges([]emitChangePlanChange{
		{Path: "src//base.go", Kind: "modify"},
		{Path: "./src/next.go", Kind: "rename", NewPath: "src//renamed.go", DependsOn: []string{"src//base.go"}},
	})
	if got := changes[0].Path; got != "src/base.go" {
		t.Fatalf("first path=%q, want src/base.go", got)
	}
	if got := changes[1].Path; got != "src/next.go" {
		t.Fatalf("second path=%q, want src/next.go", got)
	}
	if got := changes[1].NewPath; got != "src/renamed.go" {
		t.Fatalf("new_path=%q, want src/renamed.go", got)
	}
	if len(changes[1].DependsOn) != 1 || changes[1].DependsOn[0] != "src/base.go" {
		t.Fatalf("depends_on=%v, want [src/base.go]", changes[1].DependsOn)
	}
}

func TestNormalizePlanPathsForActiveRepoUsesReadToolRepoLabelAuthority(t *testing.T) {
	parent := t.TempDir()
	repoRoot := filepath.Join(parent, "bindings-py")
	for _, rel := range []string{"fastlex/tokenizer.py", "tests/test_tokenizer.py"} {
		path := filepath.Join(repoRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte("# fixture\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	ctx := &types.BusContext{RepoRoot: repoRoot}
	changes := normalizePlanPathsForActiveRepo(ctx, []types.FileChange{
		{Path: "bindings-py/fastlex/tokenizer.py", Kind: "patch"},
		{Path: "bindings-py/tests/test_tokenizer.py", Kind: "modify", DependsOn: []string{"bindings-py/fastlex/tokenizer.py"}},
	})
	if got := []string{changes[0].Path, changes[1].Path, changes[1].DependsOn[0]}; !reflect.DeepEqual(got, []string{
		"fastlex/tokenizer.py", "tests/test_tokenizer.py", "fastlex/tokenizer.py",
	}) {
		t.Fatalf("repo-label plan identities = %#v", got)
	}
	if rej, pack := validatePlanGraphIntegrityWithRepair("emit_change_plan", changes); rej != "" || pack != nil {
		t.Fatalf("normalized graph rejected: %q pack=%+v", rej, pack)
	}
}

func TestNormalizePlanPathsForActiveRepoExposesAliasDuplicateBeforeConsumers(t *testing.T) {
	parent := t.TempDir()
	repoRoot := filepath.Join(parent, "bindings-py")
	path := filepath.Join(repoRoot, "fastlex", "tokenizer.py")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := normalizePlanPathsForActiveRepo(&types.BusContext{RepoRoot: repoRoot}, []types.FileChange{
		{Path: "bindings-py/fastlex/tokenizer.py", Kind: "patch"},
		{Path: "fastlex/tokenizer.py", Kind: "modify"},
	})
	rej, pack := validatePlanGraphIntegrityWithRepair("emit_change_plan", changes)
	if rej == "" || pack == nil || pack.ReasonCode != "duplicate_change_path" {
		t.Fatalf("repo-label aliases must converge before duplicate validation: rejection=%q pack=%+v changes=%+v", rej, pack, changes)
	}
}

func TestNormalizePlanPathsForActiveRepoDoesNotGuessCreateOrRealNestedPath(t *testing.T) {
	parent := t.TempDir()
	repoRoot := filepath.Join(parent, "bindings-py")
	realNested := filepath.Join(repoRoot, "bindings-py", "existing.py")
	if err := os.MkdirAll(filepath.Dir(realNested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(realNested, []byte("# real nested file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := normalizePlanPathsForActiveRepo(&types.BusContext{RepoRoot: repoRoot}, []types.FileChange{
		{Path: "bindings-py/new_file.py", Kind: "create"},
		{Path: "bindings-py/existing.py", Kind: "modify"},
		{Path: "../outside.py", Kind: "modify"},
	})
	want := []string{"bindings-py/new_file.py", "bindings-py/existing.py", "../outside.py"}
	got := []string{changes[0].Path, changes[1].Path, changes[2].Path}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ambiguous/unsafe identities changed: got=%#v want=%#v", got, want)
	}
	if rej, pack := validatePlanGraphIntegrityWithRepair("emit_change_plan", changes[2:]); rej == "" || pack == nil || pack.ReasonCode != "change_path_unsafe" {
		t.Fatalf("traversal must remain fail-closed: rejection=%q pack=%+v", rej, pack)
	}
}

func TestNormalizePlanProbeAndProjectTestPathsForActiveRepo(t *testing.T) {
	parent := t.TempDir()
	repoRoot := filepath.Join(parent, "bindings-py")
	for _, rel := range []string{"fastlex/tokenizer.py", "tests/test_tokenizer.py"} {
		path := filepath.Join(repoRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# fixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &types.BusContext{RepoRoot: repoRoot}
	probes := normalizePlanProbePathsForActiveRepo(ctx, []types.VerificationProbe{{
		WorkingDir:        "bindings-py",
		ChangedSymbolRefs: []string{"path:bindings-py/fastlex/tokenizer.py", "Tokenizer.encode"},
	}})
	if probes[0].WorkingDir != "." || !reflect.DeepEqual(probes[0].ChangedSymbolRefs, []string{"path:fastlex/tokenizer.py", "Tokenizer.encode"}) {
		t.Fatalf("probe paths not normalized: %+v", probes[0])
	}
	observations, rej := normalizeProjectTestObservations(ctx, []types.ProjectTestObservation{{
		ID: "pto-1", TestPath: "bindings-py/tests/test_tokenizer.py", AssertionSuite: "TokenizerTest", AssertionID: "test_newlines", ContractRefs: []string{"bc-1"},
	}}, []types.FileChange{{Path: "fastlex/tokenizer.py", Kind: "patch"}})
	if rej != "" || len(observations) != 1 || observations[0].TestPath != "tests/test_tokenizer.py" {
		t.Fatalf("project test path normalization = %+v rejection=%q", observations, rej)
	}
}

func TestCanonicalPlanIdentityRejectsAliasDuplicate(t *testing.T) {
	changes := emitChangesToFileChanges([]emitChangePlanChange{
		{Path: "src//same.go", Kind: "modify"},
		{Path: "src/same.go", Kind: "patch"},
	})
	rej, pack := validatePlanGraphIntegrityWithRepair("emit_change_plan", changes)
	if rej == "" || pack == nil || pack.ReasonCode != "duplicate_change_path" {
		t.Fatalf("canonical aliases must be duplicate: rejection=%q pack=%+v", rej, pack)
	}
}

func TestPlanGraphRejectsUnsafeRepoRelativeIdentities(t *testing.T) {
	tests := []struct {
		name       string
		changes    []types.FileChange
		reasonCode string
	}{
		{name: "parent traversal", changes: []types.FileChange{{Path: "src/../outside.go", Kind: "modify"}}, reasonCode: "change_path_unsafe"},
		{name: "posix absolute", changes: []types.FileChange{{Path: "/tmp/outside.go", Kind: "modify"}}, reasonCode: "change_path_unsafe"},
		{name: "windows absolute", changes: []types.FileChange{{Path: `C:\repo\outside.go`, Kind: "modify"}}, reasonCode: "change_path_unsafe"},
		{name: "unc absolute", changes: []types.FileChange{{Path: `\\server\share\outside.go`, Kind: "modify"}}, reasonCode: "change_path_unsafe"},
		{name: "rename traversal", changes: []types.FileChange{{Path: "src/a.go", Kind: "rename", NewPath: "../a.go"}}, reasonCode: "rename_new_path_unsafe"},
		{name: "dependency traversal", changes: []types.FileChange{{Path: "src/a.go", Kind: "modify", DependsOn: []string{"../a.go"}}}, reasonCode: "depends_on_path_unsafe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rej, pack := validatePlanGraphIntegrityWithRepair("emit_change_plan", tt.changes)
			if rej == "" || pack == nil || pack.ReasonCode != tt.reasonCode {
				t.Fatalf("unsafe identity must be typed rejection: rejection=%q pack=%+v", rej, pack)
			}
		})
	}
}
