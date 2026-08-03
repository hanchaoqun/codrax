package tool

import (
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
