package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const pathScopeAddition = "def total(values):\n    return sum(values)\n"

func pathScopePlanArgs(t *testing.T, changes ...types.FileChange) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"request": "Add a total function to the selected Python module.",
		"summary": "Add the total function while preserving existing source and all unrelated plan entries.",
		"changes": changes,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func pathScopeContext(t *testing.T, scope types.WriteScope, before string) *types.BusContext {
	t.Helper()
	ctx := busCtxWithScope(scope)
	ctx.RepoRoot = t.TempDir()
	writeSurfaceFile(t, ctx.RepoRoot, "totals.py", before)
	return ctx
}

func TestPathScopeRepairPublicExistingFileOptions(t *testing.T) {
	for _, scope := range []types.WriteScope{types.ScopeMicro, types.ScopePackage, types.ScopeCross, types.ScopeProject, "", "unrecognized"} {
		for _, before := range []string{"", "BASE = 1\n"} {
			t.Run(string(scope)+"/"+map[bool]string{true: "empty", false: "populated"}[before == ""], func(t *testing.T) {
				ctx := pathScopeContext(t, scope, before)
				create := types.FileChange{Path: "totals.py", Kind: "create", NewContent: pathScopeAddition, Rationale: "add the requested function"}
				result, err := (&EmitChangePlan{}).Execute(ctx, pathScopePlanArgs(t, create))
				if err != nil || result.Success {
					t.Fatalf("existing-file create must remain rejected: %+v err=%v", result, err)
				}
				pack := mustPlanRepairPack(t, result)
				want := []string{"patch"}
				if scope == types.ScopePackage || scope == types.ScopeCross || scope == types.ScopeProject {
					want = append(want, "modify")
				}
				if pack.ReasonCode != "create_path_exists" || !reflect.DeepEqual(pack.AcceptedEnums["$.changes[].kind"], want) || !reflect.DeepEqual(pack.FailingPaths, []string{"totals.py"}) {
					t.Fatalf("current-path repair options conflict with typed scope: scope=%q pack=%+v want=%v", scope, pack, want)
				}
				if ctx.Mutable.ChangePlan() != nil {
					t.Fatal("repair must not install or rewrite the rejected plan")
				}
				if before == "" && !strings.Contains(pack.Message+pack.RetryInstruction, "insert_at_eof") {
					t.Fatalf("verified empty file needs an executable zero-line patch option: %+v", pack)
				}
				patch := types.FileChange{Path: "totals.py", Kind: "patch", Rationale: create.Rationale, Edits: []types.StructuredEdit{{Kind: "insert_at_eof", Content: pathScopeAddition}}}
				accepted, err := (&EmitChangePlan{}).Execute(ctx, pathScopePlanArgs(t, patch))
				if err != nil || !accepted.Success {
					t.Fatalf("offered local patch must be executable: %+v err=%v", accepted, err)
				}
				plan := ctx.Mutable.ChangePlan()
				if plan == nil || len(plan.Changes) != 1 || plan.Changes[0].Kind != "patch" || !reflect.DeepEqual(plan.Changes[0].Edits, patch.Edits) {
					t.Fatalf("accepted model patch must remain unchanged: %+v", plan)
				}
				data, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "totals.py"))
				if err != nil || string(data) != before {
					t.Fatalf("proposal must not change source: %q err=%v", data, err)
				}
			})
		}
	}
}

func TestPathScopeRepairPublicStagedKindChangeUsesSkeleton(t *testing.T) {
	ctx := pathScopeContext(t, types.ScopeMicro, "")
	create := types.FileChange{Path: "totals.py", Kind: "create", Rationale: "add the requested function"}
	support := types.FileChange{Path: "support.py", Kind: "create", Rationale: "supporting constant"}
	installSkeletonForTest(t, ctx, string(pathScopePlanArgs(t, support, create)))
	filled, err := (&EmitPlanChange{}).Execute(ctx, json.RawMessage(`{"path":"support.py","new_content":"MARKER = 1\n"}`))
	if err != nil || !filled.Success {
		t.Fatalf("first staged slot should be retained: %+v err=%v", filled, err)
	}
	body, _ := json.Marshal(map[string]any{"path": "totals.py", "new_content": pathScopeAddition})
	result, err := (&EmitPlanChange{}).Execute(ctx, body)
	if err != nil || result.Success {
		t.Fatalf("staged create of existing empty file must reject: %+v err=%v", result, err)
	}
	pack := mustPlanRepairPack(t, result)
	if pack.ReasonCode != "create_path_exists" || !pack.PartialPlanRetained || !strings.Contains(pack.RetryInstruction, "emit_plan_skeleton") || strings.Contains(result.Summary, "re-emit just the offending file via emit_plan_change") {
		t.Fatalf("changing slot kind requires a new skeleton, not another body: %+v summary=%s", pack, result.Summary)
	}
	partial := ctx.Mutable.PartialChangePlan()
	if partial == nil || len(partial.Changes) != 2 || partial.Changes[0].NewContent != "MARKER = 1\n" || partial.Changes[1].Kind != "create" || partial.Changes[1].NewContent != pathScopeAddition || ctx.Mutable.ChangePlan() != nil {
		t.Fatalf("failed finalization must retain every model slot without rewriting kind: %+v", partial)
	}
	modify := create
	modify.Kind = "modify"
	blocked, err := (&EmitPlanSkeleton{}).Execute(ctx, pathScopePlanArgs(t, support, modify))
	if err != nil || blocked.Success || mustPlanRepairPack(t, blocked).ReasonCode != "scope_kind_alignment_failed" {
		t.Fatalf("staged metadata replacement must preserve the micro overwrite gate: %+v err=%v", blocked, err)
	}
	retained := ctx.Mutable.PartialChangePlan()
	if retained == nil || retained.ID != partial.ID || retained.Changes[0].NewContent != "MARKER = 1\n" || retained.Changes[1].Kind != "create" {
		t.Fatalf("rejected replacement must not mutate the retained skeleton: %+v", retained)
	}
	// Changing kind is an explicit model operation on the metadata entrypoint.
	patch := types.FileChange{Path: "totals.py", Kind: "patch", Rationale: create.Rationale}
	installSkeletonForTest(t, ctx, string(pathScopePlanArgs(t, support, patch)))
	filled, err = (&EmitPlanChange{}).Execute(ctx, json.RawMessage(`{"path":"support.py","new_content":"MARKER = 1\n"}`))
	if err != nil || !filled.Success {
		t.Fatalf("corrected skeleton must retain support slot: %+v err=%v", filled, err)
	}
	patchBody, _ := json.Marshal(map[string]any{"path": "totals.py", "edits": []types.StructuredEdit{{Kind: "insert_at_eof", Content: pathScopeAddition}}})
	accepted, err := (&EmitPlanChange{}).Execute(ctx, patchBody)
	if err != nil || !accepted.Success {
		t.Fatalf("explicit corrected skeleton and local patch must finalize: %+v err=%v", accepted, err)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.Changes) != 2 || plan.Changes[0].NewContent != "MARKER = 1\n" || plan.Changes[1].Kind != "patch" || ctx.Mutable.PartialChangePlan() != nil {
		t.Fatalf("corrected plan lost model entries: %+v", plan)
	}
	data, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "totals.py"))
	if err != nil || len(data) != 0 {
		t.Fatalf("staged proposal must not apply to empty file: %q err=%v", data, err)
	}
}

func TestPathScopeRepairPublicPathBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, path, reason string
		accept             bool
	}{
		{"new path", "new_module.py", "", true},
		{"directory", "modules", "create_path_exists", false},
		{"parent traversal", "../outside.py", "change_path_unsafe", false},
		{"unstatable child", "totals.py/child.py", "path_state_stat_failed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := pathScopeContext(t, types.ScopeMicro, "")
			if err := os.Mkdir(filepath.Join(ctx.RepoRoot, "modules"), 0755); err != nil {
				t.Fatal(err)
			}
			change := types.FileChange{Path: tc.path, Kind: "create", NewContent: pathScopeAddition, Rationale: "add the requested function"}
			result, err := (&EmitChangePlan{}).Execute(ctx, pathScopePlanArgs(t, change))
			if err != nil || result.Success != tc.accept {
				t.Fatalf("path boundary changed: %+v err=%v", result, err)
			}
			if tc.accept {
				plan := ctx.Mutable.ChangePlan()
				if plan == nil || plan.Changes[0].Kind != "create" || plan.Changes[0].NewContent != pathScopeAddition {
					t.Fatalf("new-file exception must stay intact: %+v", plan)
				}
				if _, err := os.Stat(filepath.Join(ctx.RepoRoot, tc.path)); !os.IsNotExist(err) {
					t.Fatalf("proposal must not apply the new file: %v", err)
				}
				return
			}
			pack := mustPlanRepairPack(t, result)
			if pack.ReasonCode != tc.reason || len(pack.AcceptedEnums) != 0 || ctx.Mutable.ChangePlan() != nil {
				t.Fatalf("non-file/unsafe path must not offer a current-file content edit: %+v", pack)
			}
		})
	}
}

func TestPathScopeRepairPublicMissingIRConservativeAdviceNotNewGate(t *testing.T) {
	for _, scope := range []types.WriteScope{types.ScopeMicro, types.ScopePackage, types.ScopeCross, types.ScopeProject, "", "unrecognized", "no_ir"} {
		t.Run(string(scope), func(t *testing.T) {
			ctx := pathScopeContext(t, scope, "BASE = 1\n")
			if scope == "no_ir" {
				ctx.Mutable = types.NewMutableState("no write analysis")
				create := types.FileChange{Path: "totals.py", Kind: "create", NewContent: pathScopeAddition, Rationale: "add the function"}
				result, err := (&EmitChangePlan{}).Execute(ctx, pathScopePlanArgs(t, create))
				if err != nil || result.Success {
					t.Fatalf("existing-file create must reject before apply: %+v err=%v", result, err)
				}
				pack := mustPlanRepairPack(t, result)
				if !reflect.DeepEqual(pack.AcceptedEnums["$.changes[].kind"], []string{"patch"}) {
					t.Fatalf("missing scope must not recommend overwrite: %+v", pack)
				}
			}
			modify := types.FileChange{Path: "totals.py", Kind: "modify", NewContent: "BASE = 1\n" + pathScopeAddition, Rationale: "replace the complete module"}
			result, err := (&EmitChangePlan{}).Execute(ctx, pathScopePlanArgs(t, modify))
			if err != nil || result.Success != (scope != types.ScopeMicro) {
				t.Fatalf("advisory repair must not widen or tighten overwrite admission: %+v err=%v", result, err)
			}
			if scope == types.ScopeMicro {
				if pack := mustPlanRepairPack(t, result); pack.ReasonCode != "scope_kind_alignment_failed" {
					t.Fatalf("micro gate changed: %+v", pack)
				}
			} else if plan := ctx.Mutable.ChangePlan(); plan == nil || plan.Changes[0].Kind != "modify" || plan.Changes[0].NewContent != modify.NewContent {
				t.Fatalf("legacy accepted overwrite was rewritten: %+v", plan)
			}
		})
	}
}

func TestPathScopeRepairPublicMultiFileAdviceOnlyNamesFailingPath(t *testing.T) {
	ctx := pathScopeContext(t, types.ScopeMicro, "BASE = 1\n")
	newFile := types.FileChange{Path: "support.py", Kind: "create", NewContent: "MARKER = 1\n", Rationale: "independent supporting file"}
	existing := types.FileChange{Path: "totals.py", Kind: "create", NewContent: pathScopeAddition, Rationale: "add function to selected file"}
	result, err := (&EmitChangePlan{}).Execute(ctx, pathScopePlanArgs(t, newFile, existing))
	if err != nil || result.Success {
		t.Fatalf("bad second slot should reject the proposal: %+v err=%v", result, err)
	}
	pack := mustPlanRepairPack(t, result)
	if !reflect.DeepEqual(pack.FailingPaths, []string{"totals.py"}) || !reflect.DeepEqual(pack.AcceptedEnums["$.changes[].kind"], []string{"patch"}) || !strings.Contains(pack.RetryInstruction, "not to other entries") || ctx.Mutable.ChangePlan() != nil {
		t.Fatalf("repair must be localized to the failing existing path: %+v", pack)
	}
	existing.Kind, existing.NewContent = "patch", ""
	existing.Edits = []types.StructuredEdit{{Kind: "insert_at_eof", Content: pathScopeAddition}}
	accepted, err := (&EmitChangePlan{}).Execute(ctx, pathScopePlanArgs(t, newFile, existing))
	if err != nil || !accepted.Success {
		t.Fatalf("independent genuine create must remain valid beside repaired existing patch: %+v err=%v", accepted, err)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.Changes) != 2 || plan.Changes[0].Kind != "create" || plan.Changes[0].NewContent != newFile.NewContent || plan.Changes[1].Kind != "patch" {
		t.Fatalf("only explicit model repair should change the second slot: %+v", plan)
	}
}
