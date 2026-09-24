package tool

import (
	"sort"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Discovery walks must see the same physical repository that registration
// admitted. Never redirect an unrelated current root from stored plan JSON,
// and never mutate the caller's context or ordinary-plan path semantics.
func nativeRegistrationPhysicalExecutionContext(ctx *types.BusContext) *types.BusContext {
	plan := ctx.Mutable.ChangePlan()
	if !types.IsPersistedNativeTestRegistrationPlan(plan) {
		return ctx
	}
	root := repositoryReadPhysicalIdentity(ctx.RepoRoot)
	if root == "" || root != plan.NativeTestRegistration.RepositoryRoot || root == ctx.RepoRoot {
		return ctx
	}
	copy := ctx.ShallowClone()
	copy.RepoRoot = root
	return copy
}

// Registration paths extend physical observation, not the user's independent
// run_existing_test obligation. Ordinary plans keep their existing behavior.
func nativeObservedTestPaths(plan *types.ChangePlan) []string {
	out := append([]string(nil), types.RequiredExistingTestPaths(plan)...)
	seen := make(map[string]bool, len(out))
	for _, path := range out {
		seen[path] = true
	}
	for _, path := range types.RegisteredNativeTestPaths(plan) {
		if !seen[path] {
			out = append(out, path)
			seen[path] = true
		}
	}
	sort.Strings(out)
	return out
}

func nativeRegistrationExecutionBinding(ctx *types.BusContext, target, sha string) (string, bool) {
	if ctx == nil || ctx.Mutable == nil {
		return "", false
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || plan.NativeTestRegistration == nil && plan.PersistenceKind != types.PlanPersistenceNativeTestRegistration {
		return "", true
	}
	digest := types.NativeTestRegistrationDigest(plan)
	root := repositoryReadPhysicalIdentity(ctx.RepoRoot)
	if digest == "" || root == "" || !ctx.Mutable.NativeTestRegistrationExecutionAuthorized(plan, root) {
		return "", false
	}
	for _, test := range plan.NativeTestRegistration.Tests {
		if test.Path == target && test.SHA256 == sha {
			return digest, true
		}
	}
	return "", false
}

func (r *existingTestUnittestInvocation) registrationMatches(ctx *types.BusContext) bool {
	digest, ok := nativeRegistrationExecutionBinding(ctx, r.target, r.targetSHA)
	return ok && digest == r.registrationDigest
}

func isRegisteredNativeTestPath(plan *types.ChangePlan, target string) bool {
	for _, path := range types.RegisteredNativeTestPaths(plan) {
		if path == target {
			return true
		}
	}
	return false
}
