package tool

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Invocation-local identity, separate from the original source plan. The
// resolver returns a deep copy; neither a later plan switch nor an in-place
// delivery mutation can silently rebind an already running observer.
type verificationDeliveryBinding struct {
	planID, root, digest string
	snapshot             types.VerificationDeliverySnapshot
}

func bindVerificationDelivery(ctx *types.BusContext) (verificationDeliveryBinding, bool) {
	if ctx == nil || ctx.Mutable == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return verificationDeliveryBinding{}, false
	}
	plan := ctx.Mutable.ChangePlan()
	snapshot, ok := types.ResolveVerificationDelivery(plan)
	if !ok {
		return verificationDeliveryBinding{}, false
	}
	root, err := filepath.Abs(ctx.RepoRoot)
	if err != nil {
		return verificationDeliveryBinding{}, false
	}
	return verificationDeliveryBinding{planID: plan.ID, root: root, digest: verificationProbeExecutionDigest(snapshot), snapshot: snapshot}, true
}

func (b verificationDeliveryBinding) matches(ctx *types.BusContext) bool {
	current, ok := bindVerificationDelivery(ctx)
	return ok && current.planID == b.planID && current.root == b.root && current.digest == b.digest
}

// Native file execution always requires the exact current applied commit and
// clean tracked bytes. Borrowed probe delivery uses the same check; the own
// probe protocol retains its historical optional AppliedCommitSHA semantics.
func (b verificationDeliveryBinding) currentCommit(ctx *types.BusContext, requireCurrent bool) (string, error) {
	if !b.matches(ctx) || b.snapshot.PatchEffect == nil || requireCurrent && b.snapshot.AppliedCommitSHA == "" {
		return "", os.ErrInvalid
	}
	budget := 3 * time.Second
	if !requireCurrent {
		budget = 5 * time.Second // Preserve the own Python probe's original git-read budget.
	}
	deadline, cancel := context.WithTimeout(ctx.Context(), budget)
	defer cancel()
	commit, err := pythonTargetAppliedCommit(deadline, b.root, b.snapshot.PatchEffect)
	if err != nil || requireCurrent && commit != b.snapshot.AppliedCommitSHA {
		return "", os.ErrInvalid
	}
	if requireCurrent {
		head, err := pythonTargetGit(deadline, b.root, 256, "rev-parse", "HEAD")
		if err != nil || strings.TrimSpace(string(head)) != commit {
			return "", os.ErrInvalid
		}
		if _, err := pythonTargetGit(deadline, b.root, 4096, "diff", "--quiet", "HEAD", "--"); err != nil {
			return "", os.ErrInvalid
		}
	}
	if !b.matches(ctx) {
		return "", os.ErrInvalid
	}
	return commit, nil
}

func (b verificationDeliveryBinding) currentTestSHA(ctx *types.BusContext, target string) (string, bool) {
	commit, err := b.currentCommit(ctx, true)
	if err != nil {
		return "", false
	}
	deadline, cancel := context.WithTimeout(ctx.Context(), 3*time.Second)
	defer cancel()
	committed, err := pythonTargetCommitSource(deadline, b.root, commit, target)
	if err != nil {
		return "", false
	}
	current, err := os.ReadFile(filepath.Join(b.root, filepath.FromSlash(target)))
	if err != nil || !bytes.Equal(current, committed) || !b.matches(ctx) {
		return "", false
	}
	return pythonTargetSHA(current), true
}

func existingTestCurrentDelivery(ctx *types.BusContext, target string) (string, bool) {
	binding, ok := bindVerificationDelivery(ctx)
	if !ok {
		return "", false
	}
	return binding.currentTestSHA(ctx, target)
}
