package types

import (
	"sort"
	"strings"
)

// write_behavior_contract_retirement.go — V5-3 (colleague_merge_audit §40.23):
// retiring a behavior contract after a failed verification is an irreversible
// action (a tombstone shadows the id in every later cumulative scope), so it
// may fire only on RELEVANCE EVIDENCE — a failed typed verification row
// (verification probe / exact project-test assertion) whose refs intersect the
// contract — or on an explicit planner supersession, and every tombstone
// carries the evidence ids that triggered it. Which failure kinds may open the
// relevance subset at all is a FailureKind→action table
// (FailureKindReplanActions in verification_worktree_drift.go); every other
// kind retains all contracts. Only typed fields are read here: never request,
// plan, test-output, or source prose.

// WriteBehaviorContractRetirementReason is the closed set of typed reasons a
// contract id may be tombstoned by the write controller.
type WriteBehaviorContractRetirementReason string

const (
	// WriteBehaviorContractRetiredFailedVerificationProbe: a failed
	// verification_probe/<lang> row names the contract in its contract_refs or
	// placement_refs.
	WriteBehaviorContractRetiredFailedVerificationProbe WriteBehaviorContractRetirementReason = "failed_verification_probe"
	// WriteBehaviorContractRetiredFailedProjectTestAssertion: a failed
	// assertion-scoped project-test row matches a declared
	// project_test_observation whose contract_refs name the contract.
	WriteBehaviorContractRetiredFailedProjectTestAssertion WriteBehaviorContractRetirementReason = "failed_project_test_assertion"
	// WriteBehaviorContractRetiredPlannerSupersession: the planner declared
	// the id in superseded_contract_refs[] on a verify-failure replan (typed
	// escape lane of the retention rule, §1.6).
	WriteBehaviorContractRetiredPlannerSupersession WriteBehaviorContractRetirementReason = "planner_supersession"
	// WriteBehaviorContractRetiredFallbackGenerationRebase: a model-authored
	// expected_outcome / plan_acceptance fallback row of the previous
	// generation was replaced by the current plan's acceptance_tests. It is
	// planning-only prose, never a proof obligation, so its replacement is a
	// generation change rather than an evidence-based retirement.
	WriteBehaviorContractRetiredFallbackGenerationRebase WriteBehaviorContractRetirementReason = "fallback_generation_rebase"
)

// AllWriteBehaviorContractRetirementReasons is the closed set in stable order.
func AllWriteBehaviorContractRetirementReasons() []WriteBehaviorContractRetirementReason {
	return []WriteBehaviorContractRetirementReason{
		WriteBehaviorContractRetiredFailedVerificationProbe,
		WriteBehaviorContractRetiredFailedProjectTestAssertion,
		WriteBehaviorContractRetiredPlannerSupersession,
		WriteBehaviorContractRetiredFallbackGenerationRebase,
	}
}

// WriteBehaviorContractTombstone is the controller-owned record of one retired
// contract id. EvidenceRefs use the closed forms `probe:<probe_id>`,
// `assertion:<assertion_suite>::<assertion_id>` and `plan:<plan_id>` so the
// retirement stays auditable (and reversible by a later generation) without
// parsing prose. PlanID/BatchID/Attempt/FailureKind identify the failed
// verification attempt that authorized it.
type WriteBehaviorContractTombstone struct {
	ID           string                                `json:"id"`
	Reason       WriteBehaviorContractRetirementReason `json:"reason,omitempty"`
	EvidenceRefs []string                              `json:"evidence_refs,omitempty"`
	PlanID       string                                `json:"plan_id,omitempty"`
	BatchID      string                                `json:"batch_id,omitempty"`
	Attempt      int                                   `json:"attempt,omitempty"`
	FailureKind  FailureKind                           `json:"failure_kind,omitempty"`
}

// SupersededContractRefsTeaching is the ONE model-facing description of the
// planner supersession lane: the emit_change_plan / emit_plan_skeleton schema
// descriptions and the change-plan skill teaching sentence both render it, so
// the lane is taught from a single source (R2').
const SupersededContractRefsTeaching = "Optional, repair plans after a failed verification only: behavior_contract ids whose soft expectation this repair plan supersedes. " +
	"Use it when a contract with operator=satisfies and no evidence_ref describes the implementation shape that the failed verification disproved and no failed test or probe names that contract. " +
	"Only such soft or planning-only contracts may be listed; a hard, evidence-grounded, or observed contract cannot be superseded here and is rejected. " +
	"Do not reference a superseded or retired id in contract_refs, placement_refs, or project_test_observations."

// Evidence ref prefixes (closed forms).
const (
	WriteBehaviorContractEvidenceProbePrefix     = "probe:"
	WriteBehaviorContractEvidenceAssertionPrefix = "assertion:"
	WriteBehaviorContractEvidencePlanPrefix      = "plan:"
)

// VerifyFailureContractRelevance status values.
const (
	VerifyFailureContractRelevanceAvailable   = "available"
	VerifyFailureContractRelevanceUnavailable = "unavailable"
)

// VerifyFailureContractHit is one contract id named by at least one failed
// typed verification row.
type VerifyFailureContractHit struct {
	ContractID   string                                `json:"contract_id"`
	Reason       WriteBehaviorContractRetirementReason `json:"reason"`
	EvidenceRefs []string                              `json:"evidence_refs,omitempty"`
}

// VerifyFailureContractRelevance is the typed intersection between the failed
// rows of a ChangeReport and the contract refs declared by the SAME plan's
// verification probes and project-test observations. It is computed where the
// failed plan is still live (the scheduler, before the planning-state reset)
// and travels on the VerifyFailureHandoff; a report-less or plan-less carrier
// is `unavailable` and authorizes no retirement.
type VerifyFailureContractRelevance struct {
	Status     string                     `json:"status"`
	ReasonCode string                     `json:"reason_code,omitempty"`
	Hits       []VerifyFailureContractHit `json:"hits,omitempty"`
}

// Available reports whether the relevance was computed from a live plan and
// its failed report.
func (r *VerifyFailureContractRelevance) Available() bool {
	return r != nil && r.Status == VerifyFailureContractRelevanceAvailable
}

// HitByContractID indexes the hits by contract id (nil-safe).
func (r *VerifyFailureContractRelevance) HitByContractID() map[string]VerifyFailureContractHit {
	out := map[string]VerifyFailureContractHit{}
	if !r.Available() {
		return out
	}
	for _, hit := range r.Hits {
		if id := strings.TrimSpace(hit.ContractID); id != "" {
			out[id] = hit
		}
	}
	return out
}

// BuildVerifyFailureContractRelevance joins the failed rows of report with the
// probes / project-test observations of plan (including its cumulative
// verification scope) using the same exact identities the satisfied lane uses:
//   - failed `verification_probe/<lang>` rows join a probe by
//     AssertionID == probe.ID → probe.ContractRefs ∪ probe.PlacementRefs;
//   - failed assertion-scoped, non-build rows join a project_test_observation
//     by AssertionID == observation.AssertionID and a boundary-qualified suite
//     match → observation.ContractRefs.
//
// A nil report/plan or a plan/report id mismatch yields `unavailable`.
func BuildVerifyFailureContractRelevance(report *ChangeReport, plan *ChangePlan) VerifyFailureContractRelevance {
	switch {
	case report == nil:
		return VerifyFailureContractRelevance{Status: VerifyFailureContractRelevanceUnavailable, ReasonCode: "report_unavailable"}
	case plan == nil:
		return VerifyFailureContractRelevance{Status: VerifyFailureContractRelevanceUnavailable, ReasonCode: "plan_unavailable"}
	case strings.TrimSpace(plan.ID) != strings.TrimSpace(report.PlanID):
		return VerifyFailureContractRelevance{Status: VerifyFailureContractRelevanceUnavailable, ReasonCode: "plan_report_mismatch"}
	}
	probes := ChangePlanVerificationProbes(plan)
	observations := ChangePlanVerificationProjectTestObservations(plan)
	hits := map[string]*VerifyFailureContractHit{}
	record := func(id string, reason WriteBehaviorContractRetirementReason, evidence string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		hit, ok := hits[id]
		if !ok {
			hit = &VerifyFailureContractHit{ContractID: id, Reason: reason}
			hits[id] = hit
		}
		hit.EvidenceRefs = append(hit.EvidenceRefs, evidence)
	}
	for _, row := range report.TestResults {
		if row.Passed || row.Kind == TestResultKindBuildError {
			continue
		}
		suite := strings.TrimSpace(row.Suite)
		assertionID := strings.TrimSpace(row.AssertionID)
		if assertionID == "" {
			continue
		}
		if strings.HasPrefix(suite, "verification_probe/") {
			for _, probe := range probes {
				if strings.TrimSpace(probe.ID) != assertionID {
					continue
				}
				evidence := WriteBehaviorContractEvidenceProbePrefix + assertionID
				for _, ref := range probe.ContractRefs {
					record(ref, WriteBehaviorContractRetiredFailedVerificationProbe, evidence)
				}
				for _, ref := range probe.PlacementRefs {
					record(ref, WriteBehaviorContractRetiredFailedVerificationProbe, evidence)
				}
			}
			continue
		}
		if row.ObservationScope != TestObservationScopeAssertion {
			continue
		}
		for _, observation := range observations {
			if strings.TrimSpace(observation.AssertionID) != assertionID ||
				!ProjectTestAssertionSuiteMatches(suite, observation.AssertionSuite) {
				continue
			}
			evidence := WriteBehaviorContractEvidenceAssertionPrefix + strings.TrimSpace(observation.AssertionSuite) + "::" + assertionID
			for _, ref := range observation.ContractRefs {
				record(ref, WriteBehaviorContractRetiredFailedProjectTestAssertion, evidence)
			}
		}
	}
	out := VerifyFailureContractRelevance{Status: VerifyFailureContractRelevanceAvailable, ReasonCode: "typed_failed_rows_joined"}
	ids := make([]string, 0, len(hits))
	for id := range hits {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		hit := hits[id]
		hit.EvidenceRefs = dedupSortedWriteBehaviorContractIDs(hit.EvidenceRefs)
		out.Hits = append(out.Hits, *hit)
	}
	return out
}

// ProjectTestAssertionSuiteMatches is the one authority for matching a
// runner-observed suite against a declared project-test assertion suite. The
// declaration names the assertion suite itself; structured runners may qualify
// that identity with a module, file, package, or class prefix. Only exact
// equality or a boundary-qualified suffix matches; an arbitrary substring
// never gains proof (or retirement) authority.
func ProjectTestAssertionSuiteMatches(observed, declared string) bool {
	observed = strings.TrimSpace(observed)
	declared = strings.TrimSpace(declared)
	if observed == "" || declared == "" {
		return false
	}
	if observed == declared {
		return true
	}
	for _, boundary := range []string{"::", ".", "/"} {
		if strings.HasSuffix(observed, boundary+declared) {
			return true
		}
	}
	return false
}

// FailureKindContractRetirementLane says what a replan-routed failure kind may
// do to the soft behavior-contract layer (§40.23 item 4: only the relevance
// subset of tests_failed may retire; everything else retains all).
type FailureKindContractRetirementLane string

const (
	// FailureKindContractRetainAll: no ungrounded soft contract is retired by
	// this failure kind (fallback rows are still regenerated from the current
	// plan's acceptance tests; the planner supersession lane stays open).
	FailureKindContractRetainAll FailureKindContractRetirementLane = "retain_all"
	// FailureKindContractRetireRelevanceSubset: ungrounded soft contracts named
	// by a failed typed row of the same plan may be retired.
	FailureKindContractRetireRelevanceSubset FailureKindContractRetirementLane = "relevance_subset"
)

// ContractRetirementLaneForFailureKind reads the FailureKind→action table.
// An empty or unregistered kind (for example the report-less resume carrier)
// fails closed toward retention.
func ContractRetirementLaneForFailureKind(kind FailureKind) FailureKindContractRetirementLane {
	if action, ok := FailureKindReplanActions[kind]; ok && action.ContractRetirement != "" {
		return action.ContractRetirement
	}
	return FailureKindContractRetainAll
}

// WriteBehaviorContractRetirementDecision is the typed input of a verify-failure
// contract rebase: which lane the failure kind opened, the relevance hits, the
// planner's explicit supersessions, and the attempt identity every tombstone
// records.
type WriteBehaviorContractRetirementDecision struct {
	Lane                 FailureKindContractRetirementLane
	Relevance            VerifyFailureContractRelevance
	PlannerSupersededIDs []string
	// Prior is the run's tombstone ledger as of this projection: every id it
	// names is retired again under its original row (§40.46 monotonicity).
	Prior       []WriteBehaviorContractTombstone
	PlanID      string
	BatchID     string
	Attempt     int
	FailureKind FailureKind
}

// WriteBehaviorContractRetirementDecisionFromHandoff projects the typed
// carrier of the latest failed attempt into a retirement decision over the
// run's prior tombstone ledger.
func WriteBehaviorContractRetirementDecisionFromHandoff(handoff *VerifyFailureHandoff, prior []WriteBehaviorContractTombstone, plannerSupersededIDs []string) WriteBehaviorContractRetirementDecision {
	decision := WriteBehaviorContractRetirementDecision{
		Lane:                 FailureKindContractRetainAll,
		Relevance:            VerifyFailureContractRelevance{Status: VerifyFailureContractRelevanceUnavailable, ReasonCode: "verify_failure_handoff_missing"},
		PlannerSupersededIDs: dedupSortedWriteBehaviorContractIDs(plannerSupersededIDs),
		Prior:                MergeWriteBehaviorContractTombstones(nil, prior...),
	}
	if handoff == nil {
		return decision
	}
	decision.Lane = ContractRetirementLaneForFailureKind(handoff.FailureKind)
	decision.PlanID = strings.TrimSpace(handoff.PlanID)
	decision.BatchID = strings.TrimSpace(handoff.BatchID)
	decision.Attempt = handoff.Attempt
	decision.FailureKind = handoff.FailureKind
	if handoff.ContractRelevance != nil {
		decision.Relevance = *handoff.ContractRelevance
	} else {
		decision.Relevance = VerifyFailureContractRelevance{Status: VerifyFailureContractRelevanceUnavailable, ReasonCode: "contract_relevance_not_evaluated"}
	}
	return decision
}

// IsUngroundedSoftExpectedWriteBehaviorContract reports whether the contract
// is a required, expected, non-hard row without any evidence_ref — the only
// class a verify-failure retirement or a planner supersession may touch.
func IsUngroundedSoftExpectedWriteBehaviorContract(contract WriteBehaviorContract) bool {
	return isUngroundedSoftExpectedWriteBehaviorContract(contract)
}

func (d WriteBehaviorContractRetirementDecision) tombstone(id string, reason WriteBehaviorContractRetirementReason, evidence []string) WriteBehaviorContractTombstone {
	return WriteBehaviorContractTombstone{
		ID:           strings.TrimSpace(id),
		Reason:       reason,
		EvidenceRefs: dedupSortedWriteBehaviorContractIDs(evidence),
		PlanID:       d.PlanID,
		BatchID:      d.BatchID,
		Attempt:      d.Attempt,
		FailureKind:  d.FailureKind,
	}
}

func (d WriteBehaviorContractRetirementDecision) planEvidence() []string {
	if d.PlanID == "" {
		return nil
	}
	return []string{WriteBehaviorContractEvidencePlanPrefix + d.PlanID}
}
