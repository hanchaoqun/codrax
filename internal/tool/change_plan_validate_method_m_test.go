package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// busCtxWithScope builds a minimal *types.BusContext whose
// MutableState already carries a WriteAnalysisIR with the supplied
// task scope. Used by Method M tests to exercise the scope→kind
// gate without spinning up the full analyzer pipeline.
func busCtxWithScope(scope types.WriteScope) *types.BusContext {
	mut := types.NewMutableState("test")
	ir := &types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{
				Kind:    types.WriteTaskBugfix,
				Scope:   scope,
				Summary: "test",
			},
		},
	}
	mut.SetWriteAnalysisIR(ir)
	return &types.BusContext{Mutable: mut}
}

// TestValidatePlanScopeKindAlignment_PatchGoTypoR1Reproduction pins
// the production failure that motivated Method M. The patch_go_typo
// case spec asserts the planner picks kind=patch for a one-line typo
// fix; production r1 emitted kind=modify (whole-file overwrite) and
// failed the eval's PLAN_EXPECT_REGEX. With Method M, the same
// kind=modify emit on a scope=micro task is rejected at structural
// validation time before the plan is committed.
func TestValidatePlanScopeKindAlignment_PatchGoTypoR1Reproduction(t *testing.T) {
	ctx := busCtxWithScope(types.ScopeMicro)
	changes := []types.FileChange{
		{Path: "main.go", Kind: "modify", NewContent: "// full file body...\n"},
	}
	rej := validatePlanScopeKindAlignment(ctx, changes)
	if rej == "" {
		t.Fatal("Method M: scope=micro + kind=modify MUST be rejected; got pass")
	}
	if !strings.Contains(rej, `"main.go"`) {
		t.Errorf("rejection message must name the offending path; got: %s", rej)
	}
	if !strings.Contains(rej, "kind=patch") {
		t.Errorf("rejection message must point at kind=patch as the fix; got: %s", rej)
	}
	if !strings.Contains(rej, "WORKED EXAMPLE") {
		t.Errorf("rejection message must reference the WORKED EXAMPLE for diff syntax; got: %s", rej)
	}
}

// TestValidatePlanScopeKindAlignment_MicroPatchPasses — the happy
// path: scope=micro + kind=patch is the contract Method M enforces.
func TestValidatePlanScopeKindAlignment_MicroPatchPasses(t *testing.T) {
	ctx := busCtxWithScope(types.ScopeMicro)
	changes := []types.FileChange{
		{Path: "main.go", Kind: "patch", Patch: "--- a/main.go\n+++ b/main.go\n@@ -1,1 +1,1 @@\n-old\n+new\n"},
	}
	if rej := validatePlanScopeKindAlignment(ctx, changes); rej != "" {
		t.Errorf("scope=micro + kind=patch MUST pass; got rejection: %s", rej)
	}
}

// TestValidatePlanScopeKindAlignment_MicroCreateBypasses — kind=create
// is the carve-out for new files (cannot patch what doesn't exist).
func TestValidatePlanScopeKindAlignment_MicroCreateBypasses(t *testing.T) {
	ctx := busCtxWithScope(types.ScopeMicro)
	changes := []types.FileChange{
		{Path: "newfile.go", Kind: "create", NewContent: "package main\n"},
	}
	if rej := validatePlanScopeKindAlignment(ctx, changes); rej != "" {
		t.Errorf("scope=micro + kind=create MUST bypass (carve-out); got: %s", rej)
	}
}

// TestValidatePlanScopeKindAlignment_MicroDeleteBypasses — kind=delete
// is the carve-out for file removal (no content to patch).
func TestValidatePlanScopeKindAlignment_MicroDeleteBypasses(t *testing.T) {
	ctx := busCtxWithScope(types.ScopeMicro)
	changes := []types.FileChange{
		{Path: "obsolete.go", Kind: "delete"},
	}
	if rej := validatePlanScopeKindAlignment(ctx, changes); rej != "" {
		t.Errorf("scope=micro + kind=delete MUST bypass; got: %s", rej)
	}
}

// TestValidatePlanScopeKindAlignment_MicroRenameBypasses — kind=rename
// is the carve-out for pure file moves (rename is a git-level
// operation, not a content edit; a content edit alongside a rename
// requires a separate kind=patch entry which is independently
// scope-checked).
func TestValidatePlanScopeKindAlignment_MicroRenameBypasses(t *testing.T) {
	ctx := busCtxWithScope(types.ScopeMicro)
	changes := []types.FileChange{
		{Path: "old.go", Kind: "rename", NewPath: "new.go"},
	}
	if rej := validatePlanScopeKindAlignment(ctx, changes); rej != "" {
		t.Errorf("scope=micro + kind=rename MUST bypass; got: %s", rej)
	}
}

// TestValidatePlanScopeKindAlignment_PackageScopeUnconstrained —
// scope=package + kind=modify is allowed (broader scopes are not
// gated; the planner judges patch vs modify on size).
func TestValidatePlanScopeKindAlignment_PackageScopeUnconstrained(t *testing.T) {
	ctx := busCtxWithScope(types.ScopePackage)
	changes := []types.FileChange{
		{Path: "service.go", Kind: "modify", NewContent: "// rewritten\n"},
	}
	if rej := validatePlanScopeKindAlignment(ctx, changes); rej != "" {
		t.Errorf("scope=package + kind=modify MUST pass (unconstrained); got: %s", rej)
	}
}

// TestValidatePlanScopeKindAlignment_CrossScopeUnconstrained
func TestValidatePlanScopeKindAlignment_CrossScopeUnconstrained(t *testing.T) {
	ctx := busCtxWithScope(types.ScopeCross)
	changes := []types.FileChange{
		{Path: "a.go", Kind: "modify"},
		{Path: "b.go", Kind: "modify"},
	}
	if rej := validatePlanScopeKindAlignment(ctx, changes); rej != "" {
		t.Errorf("scope=cross MUST be unconstrained; got: %s", rej)
	}
}

// TestValidatePlanScopeKindAlignment_ProjectScopeUnconstrained
func TestValidatePlanScopeKindAlignment_ProjectScopeUnconstrained(t *testing.T) {
	ctx := busCtxWithScope(types.ScopeProject)
	changes := []types.FileChange{
		{Path: "go.mod", Kind: "modify"},
	}
	if rej := validatePlanScopeKindAlignment(ctx, changes); rej != "" {
		t.Errorf("scope=project MUST be unconstrained; got: %s", rej)
	}
}

// TestValidatePlanScopeKindAlignment_NilCtxSkips — defensive: nil
// ctx returns no rejection (validator can't read scope without a
// MutableState handle; downstream tools have other guards for nil
// ctx).
func TestValidatePlanScopeKindAlignment_NilCtxSkips(t *testing.T) {
	changes := []types.FileChange{
		{Path: "x.go", Kind: "modify"},
	}
	if rej := validatePlanScopeKindAlignment(nil, changes); rej != "" {
		t.Errorf("nil ctx MUST skip; got: %s", rej)
	}
}

// TestValidatePlanScopeKindAlignment_NoIRSkips — defensive: when no
// WriteAnalysisIR has been emitted (e.g. plan stage running before
// write_analyze finished), the gate stays inert.
func TestValidatePlanScopeKindAlignment_NoIRSkips(t *testing.T) {
	mut := types.NewMutableState("test")
	ctx := &types.BusContext{Mutable: mut}
	changes := []types.FileChange{
		{Path: "x.go", Kind: "modify"},
	}
	if rej := validatePlanScopeKindAlignment(ctx, changes); rej != "" {
		t.Errorf("no-IR ctx MUST skip; got: %s", rej)
	}
}

// TestValidatePlanScopeKindAlignment_MicroMixedKinds — patch +
// create + delete pass together (only the modify entry would fire
// if present).
func TestValidatePlanScopeKindAlignment_MicroMixedKinds(t *testing.T) {
	ctx := busCtxWithScope(types.ScopeMicro)
	changes := []types.FileChange{
		{Path: "patched.go", Kind: "patch", Patch: "--- a/patched.go\n..."},
		{Path: "new.go", Kind: "create", NewContent: "..."},
		{Path: "obsolete.go", Kind: "delete"},
		{Path: "moved.go", Kind: "rename", NewPath: "renamed.go"},
	}
	if rej := validatePlanScopeKindAlignment(ctx, changes); rej != "" {
		t.Errorf("micro-scope mixed kinds (no modify) MUST pass; got: %s", rej)
	}
}

// TestValidatePlanScopeKindAlignment_MicroFirstModifyCaught — when
// multiple changes are present and the first kind=modify appears
// after legitimate kinds, the rejection still names the offending
// path (validator returns on first hit).
func TestValidatePlanScopeKindAlignment_MicroFirstModifyCaught(t *testing.T) {
	ctx := busCtxWithScope(types.ScopeMicro)
	changes := []types.FileChange{
		{Path: "patched.go", Kind: "patch", Patch: "..."},
		{Path: "modified.go", Kind: "modify", NewContent: "..."},
	}
	rej := validatePlanScopeKindAlignment(ctx, changes)
	if rej == "" {
		t.Fatal("micro + later kind=modify MUST be rejected")
	}
	if !strings.Contains(rej, `"modified.go"`) {
		t.Errorf("rejection must name the offending path; got: %s", rej)
	}
}
