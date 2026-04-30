package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// scaffold_gate_test.go locks the three-tier authorization gate added
// by planPreHook: init authorization, scaffold authorization, and the
// non-empty-bare-dir bypass that lets users with existing source skip
// scaffold authorization.

// TestPlanPreHook_EmptyDirNoInitAuth — the first tier rejects when
// init authorization is absent regardless of scaffold setting.
func TestPlanPreHook_EmptyDirNoInitAuth(t *testing.T) {
	bare := t.TempDir()
	o := &Orchestrator{
		// autoInitRepo and scaffoldEnabled both false by default.
		busCtx: &types.BusContext{
			MainRepoRoot: bare,
			TraceID:      "trace-empty-no-init",
			Mutable:      types.NewMutableState("plan"),
		},
	}
	err := planPreHook(o)
	if err == nil {
		t.Fatal("planPreHook should refuse without init authorization")
	}
	msg := err.Error()
	// Must point at init authorization, NOT scaffold (init is the
	// missing piece, surface it first).
	if !strings.Contains(msg, "auto-init-repo") &&
		!strings.Contains(msg, "auto_init_repo") {
		t.Errorf("init-tier message should name --auto-init-repo; got %q", msg)
	}
}

// TestPlanPreHook_EmptyDirInitAuthNoScaffold — second tier: init
// granted, dir is structurally empty, scaffold absent → fail-fast
// with the new scaffold authorization message.
func TestPlanPreHook_EmptyDirInitAuthNoScaffold(t *testing.T) {
	bare := t.TempDir()
	o := &Orchestrator{
		autoInitRepo:    true,
		scaffoldEnabled: false,
		busCtx: &types.BusContext{
			MainRepoRoot: bare,
			TraceID:      "trace-empty-no-scaffold",
			Mutable:      types.NewMutableState("plan"),
		},
	}
	err := planPreHook(o)
	if err == nil {
		t.Fatal("planPreHook should refuse without scaffold authorization on empty dir")
	}
	msg := err.Error()
	if !strings.Contains(msg, "allow-scaffold") &&
		!strings.Contains(msg, "write_scaffold_enabled") {
		t.Errorf("scaffold-tier message should name --allow-scaffold or write_scaffold_enabled; got %q", msg)
	}
	// Must NOT leak internal jargon — "codrax" / "planner" /
	// "emit_change_plan" stay out of the user-facing surface.
	for _, banned := range []string{"emit_change_plan", "emit_plan_skeleton", "ChangePlan"} {
		if strings.Contains(msg, banned) {
			t.Errorf("scaffold message must not leak internal token %q; got %q", banned, msg)
		}
	}
}

// TestPlanPreHook_EmptyDirBothAuthorizationsPass — happy path for
// the from-scratch flow: both authorizations in place lets the gate
// pass and seeds the SCAFFOLD DIRECTIVE on PlanningHint.
func TestPlanPreHook_EmptyDirBothAuthorizationsPass(t *testing.T) {
	bare := t.TempDir()
	o := &Orchestrator{
		autoInitRepo:    true,
		scaffoldEnabled: true,
		busCtx: &types.BusContext{
			MainRepoRoot: bare,
			TraceID:      "trace-empty-full-auth",
			Mutable:      types.NewMutableState("plan"),
		},
	}
	if err := planPreHook(o); err != nil {
		t.Fatalf("planPreHook should pass with both authorizations; got %v", err)
	}
	// Empty dir + both authorized → SCAFFOLD DIRECTIVE seeded so the
	// LLM uses the multi-round emit path on first dispatch.
	hint := o.busCtx.Mutable.PlanningHint()
	if !strings.Contains(hint, "SCAFFOLD DIRECTIVE") {
		t.Errorf("PlanningHint should carry SCAFFOLD DIRECTIVE for the empty-dir + scaffold path; got %q", hint)
	}
	// .git must NOT have been created yet — plan stage defers init
	// to applyPreHook.
	if _, err := os.Stat(filepath.Join(bare, ".git")); !os.IsNotExist(err) {
		t.Errorf("planPreHook should NOT init the repo; .git exists: %v", err)
	}
}

// TestPlanPreHook_NonEmptyBareDirNoScaffoldNeeded — a bare directory
// with existing source bypasses the scaffold tier even when scaffold
// is unauthorized. The planner has real code to read, so only init
// authorization is required.
func TestPlanPreHook_NonEmptyBareDirNoScaffoldNeeded(t *testing.T) {
	bare := t.TempDir()
	// Drop a source file so dirIsEffectivelyEmpty returns false.
	if err := os.WriteFile(filepath.Join(bare, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	o := &Orchestrator{
		autoInitRepo:    true,
		scaffoldEnabled: false, // intentionally unauthorized
		busCtx: &types.BusContext{
			MainRepoRoot: bare,
			TraceID:      "trace-nonempty-bare",
			Mutable:      types.NewMutableState("plan"),
		},
	}
	if err := planPreHook(o); err != nil {
		t.Fatalf("planPreHook should pass on non-empty bare dir without scaffold auth: %v", err)
	}
	// SCAFFOLD DIRECTIVE must NOT be seeded — there is existing
	// source to read, so the LLM doesn't need scaffold-mode hint.
	hint := o.busCtx.Mutable.PlanningHint()
	if strings.Contains(hint, "SCAFFOLD DIRECTIVE") {
		t.Errorf("PlanningHint should NOT carry SCAFFOLD DIRECTIVE on non-empty bare dir; got %q", hint)
	}
}

// TestApplyPreHook_EmptyDirInitAuthNoScaffold — single-shot
// `--mode=apply --plan-file=X` against an empty bare dir with only
// init authorized must be refused at applyPreHook with the same
// scaffold-tier message planPreHook produces. This closes the
// audit-found gap where a CLI user could skip the plan stage and
// implicitly get scaffold behaviour.
func TestApplyPreHook_EmptyDirInitAuthNoScaffold(t *testing.T) {
	bare := t.TempDir()
	wtBase := t.TempDir()
	o := &Orchestrator{
		worktreeBase:    wtBase,
		autoInitRepo:    true,
		scaffoldEnabled: false,
		busCtx: &types.BusContext{
			MainRepoRoot: bare,
			TraceID:      "trace-apply-empty-no-scaffold",
			Mutable:      types.NewMutableState("apply"),
		},
	}
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{
		ID:          "plan-empty-1",
		Changes:     []types.FileChange{{Path: "main.py", Kind: "create"}},
		TargetPaths: []string{"main.py"},
	})
	err := applyPreHook(o)
	if err == nil {
		t.Fatal("applyPreHook should refuse empty bare dir without scaffold authorization")
	}
	if !strings.Contains(err.Error(), "allow-scaffold") &&
		!strings.Contains(err.Error(), "write_scaffold_enabled") {
		t.Errorf("applyPreHook scaffold-tier message should name the authorization route; got %q", err.Error())
	}
	// Worktree must NOT have been provisioned.
	if o.busCtx.WorktreePath != "" {
		t.Errorf("WorktreePath should remain empty when scaffold gate refuses; got %q", o.busCtx.WorktreePath)
	}
}

// TestStallPlateauMessage_AuthTiers exercises the three new
// auth-tier branches added to stallPlateauMessage so the localized
// remediation tracks which authorization is actually missing.
func TestStallPlateauMessage_AuthTiers(t *testing.T) {
	makeBus := func() *types.BusContext {
		return &types.BusContext{
			Mode:     types.ModePlan,
			Language: "zh",
			EnvFacts: &types.EnvFacts{GitRepoState: "not_initialized"},
		}
	}

	cases := []struct {
		name             string
		auto, scaffold   bool
		mustContain      []string
		mustNotContain   []string
	}{
		{
			name:        "no init authorization",
			auto:        false,
			scaffold:    false,
			mustContain: []string{"--auto-init-repo"},
		},
		{
			name:        "init only, no scaffold",
			auto:        true,
			scaffold:    false,
			mustContain: []string{"--allow-scaffold", "write_scaffold_enabled"},
		},
		{
			name:           "both authorized",
			auto:           true,
			scaffold:       true,
			mustContain:    []string{"更强的模型"},
			mustNotContain: []string{"--auto-init-repo", "--allow-scaffold"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := stallPlateauMessage(makeBus(), types.StagePlan, "transient", c.auto, c.scaffold)
			for _, want := range c.mustContain {
				if !strings.Contains(msg, want) {
					t.Errorf("auth-tier branch %s: missing %q; got %q", c.name, want, msg)
				}
			}
			for _, banned := range c.mustNotContain {
				if strings.Contains(msg, banned) {
					t.Errorf("auth-tier branch %s: must NOT mention %q; got %q", c.name, banned, msg)
				}
			}
			// User-facing — no internal jargon leaks.
			for _, leak := range []string{"codrax.yaml", "planner", "emit_change_plan"} {
				if strings.Contains(msg, leak) {
					t.Errorf("plateau message leaks internal token %q in branch %s; got %q", leak, c.name, msg)
				}
			}
		})
	}
}
