// merge.go — fold-back from a successful worktree to the main repo.
//
// After /approve lands a ChangePlan + verify passes, the worktree
// holds one or more codrax commits on top of the main repo's HEAD.
// Historically the user had to leave codrax, `cd` to the worktree,
// `git log` / `git diff` to inspect, then go back and `git
// cherry-pick` the SHA(s) into main themselves — five manual steps,
// any of which a less-experienced user can fumble.
//
// MergeIntoBranch automates the safe fragment of that flow without
// crossing the "main repo HEAD bytes never change automatically"
// red line. The semantics are:
//
//   - Resolve every commit on the worktree's detached HEAD that's
//     ahead of the main repo's main-branch tip (the BaseBranch).
//   - If TargetBranch == BaseBranch and a fast-forward is possible,
//     fast-forward main to the worktree HEAD.
//   - Otherwise (target branch differs, or non-ff state) create the
//     target branch on top of the base, cherry-pick the commits, and
//     leave the user on that branch ready to push / open a PR. The
//     base branch stays untouched.
//   - On any conflict, abort cleanly: `git cherry-pick --abort`,
//     delete the half-created branch, return a typed error so the
//     caller can surface the full diagnostic to the user.
//
// What this package deliberately does NOT do:
//
//   - `git push` — remote-side action, must be the user's deliberate
//     command. Returning a hint is fine; firing the push isn't.
//   - Modify the user's main-branch HEAD on a non-ff merge — that
//     would silently change history. New-branch path is the safe
//     default.
//   - `git rebase` — surprise rewrites, conflicts mid-rebase that
//     leave the operator in a complex state. cherry-pick is more
//     transparent and bounded.
//
// All git invocations go through this file. Tests exercise the
// public `MergeIntoBranch` against real fixture repos so the
// behaviour ships verified against actual git versions, not a mock.

package worktree

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// MergeMode picks the integration strategy. Callers usually leave
// this auto and let the helper decide; advanced callers force
// "branch" when they always want a PR-style workflow.
type MergeMode int

const (
	// MergeAuto picks the best strategy based on whether
	// TargetBranch == BaseBranch and whether a fast-forward is
	// possible. This is the right default for /merge.
	MergeAuto MergeMode = iota

	// MergeFastForwardOnly forces a fast-forward into TargetBranch
	// (which must equal BaseBranch). Returns ErrNonFastForward when
	// that is not possible — caller decides whether to retry as
	// MergeNewBranch.
	MergeFastForwardOnly

	// MergeNewBranch always creates a new TargetBranch on top of
	// BaseBranch and cherry-picks the worktree commits onto it.
	// BaseBranch stays untouched. Right when the user wants a PR
	// even though a fast-forward would be possible.
	MergeNewBranch
)

// MergeOptions describes the fold-back to perform.
type MergeOptions struct {
	// MainRepoRoot is the absolute path of the main checkout the
	// worktree was forked from.
	MainRepoRoot string

	// WorktreePath is the absolute path of the worktree whose HEAD
	// holds the codrax commits to fold back.
	WorktreePath string

	// BaseBranch is the branch in the main repo the worktree was
	// forked from (typically "main"). Read-only for the caller's
	// purposes — never modified except in the explicit ff-into-
	// BaseBranch case (MergeAuto with TargetBranch == BaseBranch).
	BaseBranch string

	// TargetBranch is where commits land. If == BaseBranch,
	// MergeAuto attempts ff-only first and falls back to creating a
	// throwaway branch on conflict. If != BaseBranch and the branch
	// already exists, MergeNewBranch errors out (refuse to
	// transparently rewrite an existing branch); use a fresh name.
	TargetBranch string

	// Mode selects the strategy. MergeAuto is the right default.
	Mode MergeMode
}

// MergeResult describes what landed.
type MergeResult struct {
	// Strategy is "fast_forward" or "cherry_pick_branch" describing
	// what actually happened. Useful for the caller's user-facing
	// summary.
	Strategy string

	// CommitsLanded is the list of SHAs that ended up on
	// TargetBranch (in chronological order). Empty when the
	// worktree had no commits ahead of base — the caller should
	// surface "nothing to merge" rather than silent success.
	CommitsLanded []string

	// FinalBranch is the branch the main repo is left on. For
	// fast_forward this is BaseBranch; for cherry_pick_branch it's
	// TargetBranch. The caller's "next steps" hint reads this.
	FinalBranch string
}

// ErrNonFastForward is returned when MergeFastForwardOnly is
// requested but BaseBranch is not an ancestor of the worktree HEAD.
// MergeAuto recovers automatically; explicit ff_only callers see
// this and choose how to fall back.
var ErrNonFastForward = errors.New("merge: target branch is not a fast-forward of worktree HEAD")

// ErrNothingToMerge is returned when the worktree HEAD is already
// equal to BaseBranch's tip — there are no codrax commits to fold
// back. Caller surfaces "nothing to merge" rather than treating it
// as a failure.
var ErrNothingToMerge = errors.New("merge: worktree HEAD is at base branch tip — nothing to fold back")

// ErrTargetBranchExists is returned when MergeNewBranch is asked to
// create a branch that already exists. The caller should pick a
// fresh name (typical: append the plan id or a timestamp).
var ErrTargetBranchExists = errors.New("merge: target branch already exists — pick a different name")

// MergeIntoBranch is the only public entry. Validates inputs,
// classifies the situation, runs the right git incantations, and
// returns a typed result. On any failure it leaves the main repo's
// working tree in the state it found it (cherry-pick aborted,
// throwaway branch deleted).
func MergeIntoBranch(opts MergeOptions) (*MergeResult, error) {
	if strings.TrimSpace(opts.MainRepoRoot) == "" {
		return nil, errors.New("MergeIntoBranch: empty MainRepoRoot")
	}
	if strings.TrimSpace(opts.WorktreePath) == "" {
		return nil, errors.New("MergeIntoBranch: empty WorktreePath")
	}
	if strings.TrimSpace(opts.BaseBranch) == "" {
		return nil, errors.New("MergeIntoBranch: empty BaseBranch")
	}
	if strings.TrimSpace(opts.TargetBranch) == "" {
		return nil, errors.New("MergeIntoBranch: empty TargetBranch")
	}

	// Pin the SHAs we're working with up front. Any later git
	// operation that fails we can roll back deterministically by
	// restoring these.
	worktreeSHA, err := revParse(opts.WorktreePath, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("merge: read worktree HEAD: %w", err)
	}
	baseSHA, err := revParse(opts.MainRepoRoot, opts.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("merge: read base branch %s: %w", opts.BaseBranch, err)
	}
	if worktreeSHA == baseSHA {
		return nil, ErrNothingToMerge
	}

	// Enumerate the commits ahead of base. `git rev-list base..HEAD`
	// run inside the worktree gives us the list. The order is
	// reverse-chronological by default; flip to chronological so
	// cherry-pick replays them in the same order codrax produced.
	commits, err := revList(opts.WorktreePath, opts.BaseBranch+".."+worktreeSHA)
	if err != nil {
		return nil, fmt.Errorf("merge: enumerate commits: %w", err)
	}
	if len(commits) == 0 {
		return nil, ErrNothingToMerge
	}

	mode := opts.Mode
	wantFF := opts.TargetBranch == opts.BaseBranch
	if mode == MergeAuto {
		if wantFF && isAncestor(opts.MainRepoRoot, baseSHA, worktreeSHA) {
			mode = MergeFastForwardOnly
		} else {
			mode = MergeNewBranch
		}
	}

	switch mode {
	case MergeFastForwardOnly:
		if !wantFF {
			return nil, fmt.Errorf("merge: fast_forward_only requires TargetBranch == BaseBranch (got %q vs %q)",
				opts.TargetBranch, opts.BaseBranch)
		}
		if !isAncestor(opts.MainRepoRoot, baseSHA, worktreeSHA) {
			return nil, ErrNonFastForward
		}
		// Refuse to fast-forward if the working tree is dirty —
		// the user hasn't committed something else they care about
		// and an ff would silently move HEAD past it.
		if dirty, _ := isWorkingTreeDirty(opts.MainRepoRoot); dirty {
			return nil, fmt.Errorf("merge: working tree at %s has uncommitted changes — commit or stash before /merge",
				opts.MainRepoRoot)
		}
		// Make sure we're on BaseBranch. `git merge --ff-only`
		// updates the current branch.
		if err := checkoutBranch(opts.MainRepoRoot, opts.BaseBranch); err != nil {
			return nil, fmt.Errorf("merge: checkout %s: %w", opts.BaseBranch, err)
		}
		if out, err := runGitCapture(opts.MainRepoRoot, "merge", "--ff-only", worktreeSHA); err != nil {
			return nil, fmt.Errorf("merge: git merge --ff-only: %w (%s)", err, out)
		}
		logging.Info("[worktree] merged %d commit(s) ff-only into %s",
			len(commits), opts.BaseBranch)
		return &MergeResult{
			Strategy:      "fast_forward",
			CommitsLanded: commits,
			FinalBranch:   opts.BaseBranch,
		}, nil

	case MergeNewBranch:
		if branchExists(opts.MainRepoRoot, opts.TargetBranch) {
			return nil, ErrTargetBranchExists
		}
		if dirty, _ := isWorkingTreeDirty(opts.MainRepoRoot); dirty {
			return nil, fmt.Errorf("merge: working tree at %s has uncommitted changes — commit or stash before /merge",
				opts.MainRepoRoot)
		}
		// Stash the current branch so we can restore it on any
		// failure (cherry-pick conflict, etc.). `git symbolic-ref`
		// returns a clean ref string without the "refs/heads/"
		// prefix when given --short, suitable for `git checkout`.
		priorBranch, _ := runGitCapture(opts.MainRepoRoot, "symbolic-ref", "--short", "HEAD")
		priorBranch = strings.TrimSpace(priorBranch)
		// Detach if priorBranch is empty (HEAD was already detached
		// — unusual but possible). Continue without restore in
		// that case.

		if out, err := runGitCapture(opts.MainRepoRoot, "checkout", "-b", opts.TargetBranch, opts.BaseBranch); err != nil {
			return nil, fmt.Errorf("merge: create branch %s: %w (%s)", opts.TargetBranch, err, out)
		}

		// Cherry-pick each commit. On the first conflict, abort and
		// delete the branch so the user lands back on prior state.
		for _, sha := range commits {
			if out, err := runGitCapture(opts.MainRepoRoot, "cherry-pick", sha); err != nil {
				logging.Warning("[worktree] cherry-pick %s onto %s failed: %s",
					sha, opts.TargetBranch, out)
				_, _ = runGitCapture(opts.MainRepoRoot, "cherry-pick", "--abort")
				if priorBranch != "" {
					_ = checkoutBranch(opts.MainRepoRoot, priorBranch)
				}
				_, _ = runGitCapture(opts.MainRepoRoot, "branch", "-D", opts.TargetBranch)
				return nil, fmt.Errorf(
					"merge: cherry-pick %s into %s conflicted — branch rolled back, main repo restored to %s. git output:\n%s",
					sha, opts.TargetBranch, priorBranch, out)
			}
		}
		logging.Info("[worktree] cherry-picked %d commit(s) onto new branch %s",
			len(commits), opts.TargetBranch)
		return &MergeResult{
			Strategy:      "cherry_pick_branch",
			CommitsLanded: commits,
			FinalBranch:   opts.TargetBranch,
		}, nil

	default:
		return nil, fmt.Errorf("merge: unknown mode %d", mode)
	}
}

// revParse resolves a ref to a SHA inside dir.
func revParse(dir, ref string) (string, error) {
	out, err := runGitCapture(dir, "rev-parse", ref)
	if err != nil {
		return "", fmt.Errorf("rev-parse %s: %w (%s)", ref, err, out)
	}
	return strings.TrimSpace(out), nil
}

// revList enumerates commits matching range, oldest-first so
// cherry-pick replays in the same order codrax produced.
func revList(dir, rangeSpec string) ([]string, error) {
	// `--reverse` flips the default new-first to old-first.
	out, err := runGitCapture(dir, "rev-list", "--reverse", rangeSpec)
	if err != nil {
		return nil, fmt.Errorf("rev-list %s: %w (%s)", rangeSpec, err, out)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// isAncestor reports whether ancestor is reachable from descendant.
// Used to decide whether a fast-forward is possible.
func isAncestor(dir, ancestor, descendant string) bool {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// isWorkingTreeDirty reports whether `git status --porcelain` shows
// any modifications. Used to refuse merges that would silently move
// HEAD past uncommitted user work.
func isWorkingTreeDirty(dir string) (bool, error) {
	out, err := runGitCapture(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// checkoutBranch is a thin wrapper. Errors include the branch name
// for diagnosis.
func checkoutBranch(dir, branch string) error {
	if out, err := runGitCapture(dir, "checkout", branch); err != nil {
		return fmt.Errorf("checkout %s: %w (%s)", branch, err, out)
	}
	return nil
}

// branchExists reports whether `git rev-parse --verify
// refs/heads/<branch>` exits 0.
func branchExists(dir, branch string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// MergeFromRefOptions parameterises MergeFromRef. Same shape as
// MergeOptions but rooted on a ref (typically refs/codrax/applied/
// <plan-id>) instead of a worktree path. Used by /merge when the
// worktree was discarded but the apply commit is still pinned in
// the main repo via TagAppliedCommit.
type MergeFromRefOptions struct {
	MainRepoRoot string
	Ref          string
	BaseBranch   string
	TargetBranch string
	Mode         MergeMode
}

// MergeFromRef folds a refs/codrax/applied/<id>-style ref into a
// branch in the main repo. Mirrors MergeIntoBranch's safety machinery
// (clean working tree, restore-on-fail, MergeAuto FF/new-branch
// classification) but reads the source SHAs from a ref instead of a
// worktree's HEAD.
//
// This is the keep_on_success=false recovery path: even when the
// worktree directory is gone, the apply commit lives in main_repo/
// .git/objects, and the ref keeps it reachable so cherry-pick works.
func MergeFromRef(opts MergeFromRefOptions) (*MergeResult, error) {
	if strings.TrimSpace(opts.MainRepoRoot) == "" {
		return nil, errors.New("MergeFromRef: empty MainRepoRoot")
	}
	if strings.TrimSpace(opts.Ref) == "" {
		return nil, errors.New("MergeFromRef: empty Ref")
	}
	if strings.TrimSpace(opts.BaseBranch) == "" {
		return nil, errors.New("MergeFromRef: empty BaseBranch")
	}
	if strings.TrimSpace(opts.TargetBranch) == "" {
		return nil, errors.New("MergeFromRef: empty TargetBranch")
	}

	// Resolve the ref to a SHA in the main repo's view. If the ref
	// is missing, the apply state has been GC'd or never tagged —
	// surface a typed error so /merge can suggest the operator open
	// a fresh plan.
	tipSHA, err := revParse(opts.MainRepoRoot, opts.Ref)
	if err != nil {
		return nil, fmt.Errorf("MergeFromRef: resolve %s: %w (the ref may have been pruned by `git gc`; re-run /approve to refresh)",
			opts.Ref, err)
	}
	baseSHA, err := revParse(opts.MainRepoRoot, opts.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("MergeFromRef: read base branch %s: %w", opts.BaseBranch, err)
	}
	if tipSHA == baseSHA {
		return nil, ErrNothingToMerge
	}

	// Enumerate commits ahead of base. Same as MergeIntoBranch but
	// the rev-list runs in the main repo since the worktree is gone.
	commits, err := revList(opts.MainRepoRoot, opts.BaseBranch+".."+tipSHA)
	if err != nil {
		// Falls through when the ref points at a commit not yet
		// reachable from base — most commonly because base moved on.
		// Single-commit cherry-pick still works if isAncestor reports
		// the relationship cleanly; otherwise we need explicit list.
		commits = []string{tipSHA}
	}
	if len(commits) == 0 {
		commits = []string{tipSHA}
	}

	mode := opts.Mode
	wantFF := opts.TargetBranch == opts.BaseBranch
	if mode == MergeAuto {
		if wantFF && isAncestor(opts.MainRepoRoot, baseSHA, tipSHA) {
			mode = MergeFastForwardOnly
		} else {
			mode = MergeNewBranch
		}
	}

	if dirty, _ := isWorkingTreeDirty(opts.MainRepoRoot); dirty {
		return nil, fmt.Errorf("merge: working tree at %s has uncommitted changes — commit or stash before /merge",
			opts.MainRepoRoot)
	}

	switch mode {
	case MergeFastForwardOnly:
		if !wantFF {
			return nil, fmt.Errorf("merge: fast_forward_only requires TargetBranch == BaseBranch (got %q vs %q)",
				opts.TargetBranch, opts.BaseBranch)
		}
		if !isAncestor(opts.MainRepoRoot, baseSHA, tipSHA) {
			return nil, ErrNonFastForward
		}
		if err := checkoutBranch(opts.MainRepoRoot, opts.BaseBranch); err != nil {
			return nil, fmt.Errorf("merge: checkout %s: %w", opts.BaseBranch, err)
		}
		if out, err := runGitCapture(opts.MainRepoRoot, "merge", "--ff-only", tipSHA); err != nil {
			return nil, fmt.Errorf("merge: git merge --ff-only %s: %w (%s)", tipSHA, err, out)
		}
		logging.Info("[worktree] merged %d commit(s) ff-only into %s via ref %s",
			len(commits), opts.BaseBranch, opts.Ref)
		return &MergeResult{
			Strategy:      "fast_forward",
			CommitsLanded: commits,
			FinalBranch:   opts.BaseBranch,
		}, nil

	case MergeNewBranch:
		if branchExists(opts.MainRepoRoot, opts.TargetBranch) {
			return nil, ErrTargetBranchExists
		}
		priorBranch, _ := runGitCapture(opts.MainRepoRoot, "symbolic-ref", "--short", "HEAD")
		priorBranch = strings.TrimSpace(priorBranch)
		if out, err := runGitCapture(opts.MainRepoRoot, "checkout", "-b", opts.TargetBranch, opts.BaseBranch); err != nil {
			return nil, fmt.Errorf("merge: create branch %s: %w (%s)", opts.TargetBranch, err, out)
		}
		// Cherry-pick the apply commit. On conflict abort and roll
		// back to priorBranch so the operator's working state isn't
		// left half-merged.
		if out, err := runGitCapture(opts.MainRepoRoot, "cherry-pick", tipSHA); err != nil {
			_, _ = runGitCapture(opts.MainRepoRoot, "cherry-pick", "--abort")
			if priorBranch != "" {
				_ = checkoutBranch(opts.MainRepoRoot, priorBranch)
			}
			_, _ = runGitCapture(opts.MainRepoRoot, "branch", "-D", opts.TargetBranch)
			return nil, fmt.Errorf("merge: cherry-pick %s onto %s conflicted — rolled back to %s: %w (%s)",
				tipSHA, opts.TargetBranch, priorBranch, err, out)
		}
		logging.Info("[worktree] cherry-picked %s onto new branch %s via ref %s",
			tipSHA, opts.TargetBranch, opts.Ref)
		return &MergeResult{
			Strategy:      "cherry_pick_branch",
			CommitsLanded: commits,
			FinalBranch:   opts.TargetBranch,
		}, nil
	}
	return nil, fmt.Errorf("MergeFromRef: unknown mode %d", mode)
}
