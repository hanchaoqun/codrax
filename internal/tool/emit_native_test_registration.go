package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Both structured exits use one atomic publication path. This is a new
// controller-authorized, read-only registration, not a relaxed source plan or
// a probe-only sentinel. Nothing below edits source, tests or old reports.
func emitNativeTestRegistration(ctx *types.BusContext, toolName string, p emitChangePlanParams) (types.ToolResult, error) {
	reject := func(reason, detail string) (types.ToolResult, error) {
		return rejectPlanToolResult(toolName, toolName+" rejected: "+detail,
			planRepairPackFromReason(toolName, reason, detail, []string{"$.project_test_observations"}, nil)), nil
	}
	if ctx == nil || ctx.Mutable == nil || !ctx.Mode.IsWrite() || ctx.PipelineStage != types.StagePlan {
		return reject("project_test_registration_unauthorized", "read-only test registration requires the current controller-authorized planning dispatch")
	}
	grant := ctx.Mutable.NativeTestRegistrationAuthorization()
	if grant == nil {
		return reject("project_test_observation_without_changes", "no current controller authorization for read-only existing-test registration; do not edit tests merely to register evidence")
	}
	if len(p.Changes) != 0 || len(p.VerificationProbes) != 0 || len(p.SupersededContractRefs) != 0 {
		return reject("project_test_registration_shape", "read-only registration accepts only existing project_test_observations: no changes, verification probes, or retired contracts")
	}
	observations, reason := normalizeProjectTestObservations(ctx, p.ProjectTestObservations, nil)
	if reason != "" || len(observations) == 0 {
		return reject("project_test_registration_invalid", "existing-test declarations are invalid: "+reason)
	}
	root, err := filepath.EvalSymlinks(ctx.RepoRoot)
	if err != nil {
		return reject("project_test_registration_delivery_changed", "the authorized repository is unavailable")
	}
	root, err = filepath.Abs(root)
	if err != nil || root != grant.RepositoryRoot {
		return reject("project_test_registration_delivery_changed", "the current physical repository differs from the authorized source delivery")
	}
	plan := newChangePlanFromChanges(p.Request, p.Summary, nil, p.AcceptanceTests, nil, observations)
	plan.Status = types.PlanStatusNoChangeRequired
	plan.TargetPaths = append([]string(nil), grant.TargetPaths...)
	plan.BehaviorContracts = append([]types.WriteBehaviorContract(nil), grant.Contracts...)
	plan.WorktreePath = root
	if ir := ctx.Mutable.WriteAnalysisIR(); ir != nil {
		encoded, err := json.Marshal(ir)
		if err != nil || json.Unmarshal(encoded, &plan.WriteAnalysisIR) != nil {
			return reject("project_test_registration_context_changed", "the current execution constraints could not be retained")
		}
	}
	if reason, code, fields := validatePlanBehaviorContractRefs(plan); reason != "" {
		return rejectPlanToolResult(toolName, toolName+" rejected: "+reason,
			planRepairPackFromReason(toolName, code, reason, fields, nil)), nil
	}
	surface := BuildTestSurface(root, root)
	versions := make(map[string]string)
	targets := append([]string(nil), types.RequiredExistingTestPaths(plan)...)
	for _, observation := range observations {
		targets = append(targets, observation.TestPath)
	}
	for _, target := range targets {
		if _, found := versions[target]; found {
			continue
		}
		if nativeRegistrationCandidate(surface, target) == nil {
			return reject("project_test_registration_runner_unavailable", "the existing file has no exact Python unittest execution candidate: "+target)
		}
		sha, ok := nativeRegistrationPhysicalTestSHA(ctx, root, grant.Delivery, target)
		if !ok {
			return reject("project_test_registration_delivery_changed", "source HEAD or existing test bytes no longer match the authorized delivery: "+target)
		}
		versions[target] = sha
	}
	tests := make([]types.NativeTestFileVersion, 0, len(versions))
	for path, sha := range versions {
		tests = append(tests, types.NativeTestFileVersion{Path: path, SHA256: sha})
	}
	sort.Slice(tests, func(i, j int) bool { return tests[i].Path < tests[j].Path })
	if err := ctx.Mutable.InstallNativeTestRegistration(plan, grant.ID, tests); err != nil {
		return reject("project_test_registration_not_current_or_delivered", err.Error())
	}
	return types.ToolResult{ToolName: toolName, Success: true, Timestamp: time.Now(), Summary: fmt.Sprintf(
		"[%s: id=%s changes=0 status=%s project_test_observations=%d]\nRegistered existing test declarations without editing files. This is not a passing test result; verification must execute these exact tests again against the retained source delivery.",
		toolName, plan.ID, plan.Status, len(observations))}, nil
}

func nativeRegistrationPhysicalTestSHA(ctx *types.BusContext, root string, delivery types.VerificationDeliverySnapshot, target string) (string, bool) {
	commit, err := verificationDeliveryPhysicalCommit(ctx, root, delivery, true)
	if err != nil {
		return "", false
	}
	deadline, cancel := context.WithTimeout(ctx.Context(), 3*time.Second)
	defer cancel()
	committed, err := pythonTargetCommitSource(deadline, root, commit, target)
	if err != nil {
		return "", false
	}
	current, err := readPythonTargetSource(root, target)
	if err != nil || !bytes.Equal(current, committed) {
		return "", false
	}
	return pythonTargetSHA(current), true
}

// Reuse the execution inventory and exact-file selector. Selection is shared
// with run_tests so a pytest sibling cannot silently replace this protocol.
func nativeRegistrationCandidate(surface types.TestSurface, target string) *types.TestSurfaceCandidate {
	var best *types.TestSurfaceCandidate
	for _, candidate := range surface.Candidates {
		suite, ok := projectTestObservationCandidateSuite(candidate, target)
		if !ok || !types.ExistingTestExactFileSelector(candidate.Runner, candidate.Framework, candidate.WorkingDir, suite, target) {
			continue
		}
		if best == nil || impactWorkingDirDepth(candidate.WorkingDir) > impactWorkingDirDepth(best.WorkingDir) {
			copy := candidate
			best = &copy
		}
	}
	return best
}
