package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

const PlanPersistenceNativeTestRegistration = "native_test_registration"

// NativeTestFileVersion is a producer-read, fully delivered test version.
// It is not an execution result, and never creates a user test requirement.
type NativeTestFileVersion struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// NativeTestRegistration binds one read-only declaration to one delivered
// source and the entire effective contract definition, not just contract IDs.
// The planner wire has no field for this controller/tool-owned carrier.
type NativeTestRegistration struct {
	Version         int                          `json:"version"`
	Digest          string                       `json:"digest"`
	PlanID          string                       `json:"plan_id"`
	AuthorizationID string                       `json:"authorization_id"`
	RunID           string                       `json:"run_id"`
	BatchID         string                       `json:"batch_id"`
	RepositoryRoot  string                       `json:"repository_root"`
	Delivery        VerificationDeliverySnapshot `json:"delivery"`
	ContractsDigest string                       `json:"contracts_digest"`
	AnalysisDigest  string                       `json:"analysis_digest"`
	Tests           []NativeTestFileVersion      `json:"tests"`
}

// NativeTestRegistrationRecord is a separate controller-run receipt. A plan
// JSON with a recomputed checksum cannot manufacture this registration record.
type NativeTestRegistrationRecord struct {
	PlanID string `json:"plan_id"`
	Digest string `json:"digest"`
}

func nativeRegistrationHash(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func nativeRegistrationPath(path string) bool {
	return path != "" && path == strings.TrimSpace(path) && !filepath.IsAbs(path) &&
		filepath.ToSlash(filepath.Clean(path)) == path && path != "." && path != ".." &&
		!strings.HasPrefix(path, "../") && !strings.ContainsAny(path, "\\\x00")
}

func nativeRegistrationContractsDigest(plan *ChangePlan) string {
	return nativeRegistrationHash(struct {
		Contracts []WriteBehaviorContract
		Retired   []WriteBehaviorContractTombstone
		Legacy    []string
	}{ChangePlanVerificationBehaviorContracts(plan), plan.SupersededBehaviorContracts, plan.SupersededBehaviorContractIDs})
}

// NativeTestRegistrationDigest validates a durable shape and exact bindings.
// A valid checksum is NOT permission to execute; the current controller run
// and physical repository must independently authorize a fresh invocation.
func NativeTestRegistrationDigest(plan *ChangePlan) string {
	if plan == nil || plan.PersistenceKind != PlanPersistenceNativeTestRegistration || plan.NativeTestRegistration == nil {
		return ""
	}
	r := plan.NativeTestRegistration
	if r.Version != 1 || r.PlanID == "" || r.PlanID != plan.ID || r.AuthorizationID == "" ||
		r.RunID == "" || r.BatchID == "" || !filepath.IsAbs(r.RepositoryRoot) || filepath.Clean(r.RepositoryRoot) != r.RepositoryRoot ||
		!r.Delivery.Valid() || r.Delivery.SourcePlanID == plan.ID || len(plan.Changes) != 0 || len(plan.Slices) != 0 ||
		len(plan.VerificationProbes) != 0 || len(plan.ProjectTestObservations) == 0 || len(plan.TargetPaths) == 0 ||
		plan.PatchEffect != nil || plan.AppliedCommitSHA != "" || len(plan.AppliedPaths) != 0 || plan.ApplyCheckpoint != nil ||
		len(plan.SupersededBehaviorContracts) != 0 || len(plan.SupersededBehaviorContractIDs) != 0 {
		return ""
	}
	s := plan.CumulativeVerificationScope
	if s == nil || len(s.SourcePlanIDs) != 1 || s.SourcePlanIDs[0] != r.Delivery.SourcePlanID ||
		len(s.AppliedSources) != 1 || nativeRegistrationHash(s.AppliedSources[0]) != nativeRegistrationHash(r.Delivery) ||
		!reflect.DeepEqual(s.TargetPaths, plan.TargetPaths) || len(s.BehaviorContracts) != 0 ||
		len(s.VerificationProbes) != 0 || len(s.ProjectTestObservations) != 0 ||
		r.ContractsDigest != nativeRegistrationContractsDigest(plan) || r.AnalysisDigest != nativeRegistrationHash(plan.WriteAnalysisIR) {
		return ""
	}
	versions := map[string]string{}
	for _, test := range r.Tests {
		if !nativeRegistrationPath(test.Path) || !strings.HasSuffix(test.Path, ".py") ||
			!dispatchRepositoryReadSHA256(test.SHA256) || versions[test.Path] != "" {
			return ""
		}
		versions[test.Path] = test.SHA256
	}
	contracts := map[string]bool{}
	for _, contract := range plan.BehaviorContracts {
		if contract.ID == "" || contracts[contract.ID] {
			return ""
		}
		contracts[contract.ID] = true
	}
	used, observationIDs := map[string]bool{}, map[string]bool{}
	for _, row := range plan.ProjectTestObservations {
		if row.ID == "" || observationIDs[row.ID] || versions[row.TestPath] == "" || row.AssertionSuite == "" || row.AssertionID == "" || len(row.ContractRefs) == 0 {
			return ""
		}
		observationIDs[row.ID], used[row.TestPath] = true, true
		for _, ref := range row.ContractRefs {
			if !contracts[ref] {
				return ""
			}
		}
	}
	for _, path := range RequiredExistingTestPaths(plan) {
		if versions[path] == "" {
			return ""
		}
		used[path] = true
	}
	if len(used) != len(versions) {
		return ""
	}
	copy := *r
	copy.Digest = ""
	digest := nativeRegistrationHash(struct {
		Registration NativeTestRegistration
		Observations []ProjectTestObservation
		Targets      []string
	}{copy, plan.ProjectTestObservations, plan.TargetPaths})
	if r.Digest != digest {
		return ""
	}
	return digest
}

func sealNativeTestRegistration(plan *ChangePlan) {
	r := plan.NativeTestRegistration
	r.ContractsDigest = nativeRegistrationContractsDigest(plan)
	r.AnalysisDigest = nativeRegistrationHash(plan.WriteAnalysisIR)
	r.Digest = ""
	r.Digest = nativeRegistrationHash(struct {
		Registration NativeTestRegistration
		Observations []ProjectTestObservation
		Targets      []string
	}{*r, plan.ProjectTestObservations, plan.TargetPaths})
}

func IsPersistedNativeTestRegistrationPlan(plan *ChangePlan) bool {
	if NativeTestRegistrationDigest(plan) == "" {
		return false
	}
	switch plan.Status {
	case PlanStatusNoChangeRequired, PlanStatusApplied, PlanStatusUnverified, PlanStatusVerifyFailed, PlanStatusRejected:
		return true
	default:
		return false
	}
}

func RegisteredNativeTestPaths(plan *ChangePlan) []string {
	if NativeTestRegistrationDigest(plan) == "" {
		return nil
	}
	out := make([]string, 0, len(plan.NativeTestRegistration.Tests))
	for _, test := range plan.NativeTestRegistration.Tests {
		out = append(out, test.Path)
	}
	sort.Strings(out)
	return out
}

// NativeTestRegistrationMatchesRun requires the independent registration
// ledger and exact verify-only batch, never a historical prose progress event.
func NativeTestRegistrationMatchesRun(plan *ChangePlan, run *WriteWorkflowRun, root string) bool {
	digest := NativeTestRegistrationDigest(plan)
	if digest == "" || run == nil || !IsPersistedNativeTestRegistrationPlan(plan) {
		return false
	}
	r := plan.NativeTestRegistration
	if r.RunID != run.RunID || r.BatchID != run.ActiveBatchID || r.RepositoryRoot != root {
		return false
	}
	matched := 0
	for _, receipt := range run.NativeTestRegistrations {
		if receipt.PlanID == plan.ID {
			if receipt.Digest != digest {
				return false
			}
			matched++
		}
	}
	if matched != 1 {
		return false
	}
	for _, batch := range run.Batches {
		if batch.ID == run.ActiveBatchID {
			return batch.PlanID == plan.ID && batch.ExecutionMode == WriteWorkflowBatchExecutionVerifyOnly &&
				(batch.Purpose == "verification_proof_followup" || batch.Purpose == "impact_and_verification_proof_followup")
		}
	}
	return false
}
