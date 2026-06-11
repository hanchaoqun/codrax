// bare_dir_consent.go — interactive authorization for write-mode
// Runs against a target directory that is not (yet) a git repo.
//
// Two tiers, mirroring the orchestrator stage-hook gates:
//
//   - init: `git init` + empty initial commit (worktree.EnsureInitialCommit,
//     executed by the apply pre-hook — never here).
//   - scaffold: generating a brand-new project in an effectively empty
//     directory (planPreHook / applyPreHook scaffoldEnabled gate).
//
// An interactive REPL asks ONE y/N covering every tier the upcoming
// Run will need, in place of the --auto-init-repo / --allow-scaffold
// flag ceremony (2026-06-12 pytest forensics #2). Scripted REPL
// sessions and one-shot CLI keep the explicit-flag fail-loud via the
// orchestrator pre-hooks: unattended runs must not silently mutate
// state. Mode selection itself stays explicit (L2) — by the time the
// consent fires the user is already in /mode plan|apply|verify.

package repl

import (
	"github.com/charmbracelet/huh"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// scaffoldEnabledSetter is the optional runner capability that lifts
// the empty-dir scaffold tier for one Run. Like autoInitRepoSetter,
// test stubs may omit it — scaffold only matters on real
// orchestrator paths.
type scaffoldEnabledSetter interface {
	SetScaffoldEnabled(bool)
}

// bareDirConsentNeeded reports whether an interactive consent prompt
// is required before a write-mode Run against a target that still
// needs git init. Pure decision function: consent is needed when the
// init tier is unauthorized, or when the directory is effectively
// empty and the scaffold tier is unauthorized. All three inputs are
// typed booleans — no prose sniffing.
func bareDirConsentNeeded(autoInit, scaffoldEnabled, empty bool) bool {
	return !autoInit || (empty && !scaffoldEnabled)
}

// preRunBareDirConsent runs the pre-dispatch consent flow. Returns
// proceed=false when the user declined (the caller bails without
// dispatching) and a restore func the caller MUST defer when it is
// non-nil, so the per-Run authorization never outlives the Run (a
// later turn can target a different, already-initialized repo).
func (r *REPL) preRunBareDirConsent() (bool, func()) {
	if r.currentMode == types.ModeRead || !r.interactive() {
		return true, nil
	}
	state, derr := worktree.DetectRepoState(r.repoRoot)
	if derr != nil || !state.NeedsInit() {
		return true, nil
	}
	empty := worktree.DirIsEffectivelyEmpty(r.repoRoot)
	if !bareDirConsentNeeded(r.writeAutoInitRepo, r.writeScaffoldEnabled, empty) {
		return true, nil
	}
	title := autoInitConsentTitle(r.language, r.repoRoot, state.String())
	if empty {
		title = autoInitScaffoldConsentTitle(r.language, r.repoRoot, state.String())
	}
	consent := false
	if err := huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("No").
		Value(&consent).
		Run(); err != nil {
		consent = false
	}
	if !consent {
		for _, line := range autoInitDeclinedPreRun(r.language) {
			r.info(line)
		}
		return false, nil
	}
	r.info(autoInitAuthorizedPreRun(r.language, r.repoRoot))
	return true, r.forwardBareDirAuthorization(empty)
}

// forwardBareDirAuthorization lifts the per-Run bare-directory write
// authorizations on the runner: init always, scaffold only when
// withScaffold. Returns a restore func (never nil) that resets each
// lifted setter to the boot-time yaml/CLI value.
func (r *REPL) forwardBareDirAuthorization(withScaffold bool) func() {
	var restores []func()
	if setter, ok := r.runner.(autoInitRepoSetter); ok {
		setter.SetAutoInitRepo(true)
		prior := r.writeAutoInitRepo
		restores = append(restores, func() { setter.SetAutoInitRepo(prior) })
	}
	if withScaffold {
		if setter, ok := r.runner.(scaffoldEnabledSetter); ok {
			setter.SetScaffoldEnabled(true)
			prior := r.writeScaffoldEnabled
			restores = append(restores, func() { setter.SetScaffoldEnabled(prior) })
		}
	}
	return func() {
		for i := len(restores) - 1; i >= 0; i-- {
			restores[i]()
		}
	}
}
