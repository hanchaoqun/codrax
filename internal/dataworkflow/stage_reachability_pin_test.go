package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// This file pins two structural invariants of the typed data workflow:
//
//  1. Producer reachability: every required validation ledger must have at
//     least one legal producer action inside the allowed set of some stage
//     reachable from the current facts. A gate table that withholds every
//     producer of a required ledger is a typed deadlock — the completion
//     gate demands a ledger that no admissible action can produce.
//
//  2. Advisory legality: the published workflow decision advisory
//     (decision_next_actions) must be a subset of the enforced allowed
//     action set. Publishing an action the admission gate must reject
//     misdirects the planner into guaranteed guard failures.
//
// Both invariants are checked over an enumerated StageFacts slice: all
// combinations of the five required flags × ledger presence (zero vs
// non-zero record counts, reconcile presence, answer presence) — 2^11 = 2048
// states. Deliberately NOT enumerated: MaterialCoverageSufficient is pinned
// true (material collection is upstream of every ledger gate here) and
// EntityStageMaterialized is pinned false (the entity-stage escape hatch has
// its own tests). Producer application honors the prerequisite edges
// published by BuildLedgerGraph — the single prerequisite authority; there
// is no second prerequisite table in this file — so a walk can never
// "succeed" through a production the runtime hard-rejects. Within that
// universe, any future edit to NextStage,
// AllowedNextActionContractsForFacts, BuildLedgerGraph, or
// BuildWorkflowDecision that reopens a deadlock, a gate/advisory
// contradiction, or a published recommendation of a guaranteed-failure
// action fails these tests immediately.
// Origin: eval data_basic_sum_with_rules 3/3 blocked terminal, 2026-07-05.

// pinLedgerUniverse maps MissingValidationStages entries to ledger kinds.
// final_answer is intentionally absent: final answers are produced by
// assemble_answer or by any batch result carrying an answer envelope, which
// is an assembly-layer concern, not a required validation ledger.
var pinLedgerUniverse = map[string]LedgerKind{
	"rule_coverage":       LedgerRuleCoverage,
	"decision_records":    LedgerDecisions,
	"entity_resolution":   LedgerEntityResolutions,
	"contribution_ledger": LedgerContributions,
	"reconcile":           LedgerReconcile,
}

func pinEnumerateStageFacts() []StageFacts {
	bools := []bool{false, true}
	var out []StageFacts
	for _, ruleReq := range bools {
		for _, decisionReq := range bools {
			for _, entityReq := range bools {
				for _, contribReq := range bools {
					for _, reconcileReq := range bools {
						for _, ruleCount := range []int{0, 2} {
							for _, decisionCount := range []int{0, 2} {
								for _, entityCount := range []int{0, 4} {
									for _, contribCount := range []int{0, 2} {
										for _, hasReconcile := range bools {
											for _, hasAnswer := range bools {
												out = append(out, StageFacts{
													MaterialCoverageSufficient: true,
													RuleCoverageRequired:       ruleReq,
													RuleCoverageRecords:        ruleCount,
													DecisionRecordsRequired:    decisionReq,
													DecisionRecords:            decisionCount,
													EntityResolutionRequired:   entityReq,
													EntityResolutionRecords:    entityCount,
													ContributionLedgerRequired: contribReq,
													ContributionRecords:        contribCount,
													ReconcileRequired:          reconcileReq,
													HasReconcile:               hasReconcile,
													HasAnswer:                  hasAnswer,
												})
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return out
}

func pinMissingRequiredLedgers(facts StageFacts) []LedgerKind {
	var out []LedgerKind
	for _, stage := range MissingValidationStages(facts) {
		if ledger, ok := pinLedgerUniverse[stage]; ok {
			out = append(out, ledger)
		}
	}
	return out
}

// pinAllowedTypedActions models the enforced admission surface in its worst
// legal case: the allowed contracts for the current facts with custom
// transforms excluded (custom_transform can be disabled at runtime, so
// reachability must never depend on it).
func pinAllowedTypedActions(facts StageFacts) []ActionContract {
	return FilterCustomTransformContracts(AllowedNextActionContractsForFacts(facts))
}

// pinMissingPrerequisites reads the missing prerequisites BuildLedgerGraph
// publishes for one ledger. The published graph is the single authority for
// prerequisite facts — this file deliberately keeps no second prerequisite
// table, so a graph edit and a pin edit can never diverge silently.
func pinMissingPrerequisites(graph LedgerGraph, ledger LedgerKind) []string {
	dep, ok := pinFindDependency(graph, ledger)
	if !ok {
		return nil
	}
	return dep.MissingPrerequisites
}

// pinProductionFrontier expands the missing required ledgers through the
// prerequisite edges the published ledger graph declares, returning only the
// ledgers that are legally producible right now (no missing prerequisites of
// their own). A ledger whose producer action would be hard-rejected at
// runtime (e.g. reconcile over an empty contribution ledger,
// runReconcileArtifacts) is NOT actionable — its unmet prerequisites are the
// actionable work. Any state whose frontier has no legal producer in the
// allowed set is a state where every published surface jointly recommends a
// guaranteed-failure action.
func pinProductionFrontier(graph LedgerGraph, missing []LedgerKind) []LedgerKind {
	var frontier []LedgerKind
	seen := map[LedgerKind]bool{}
	var visit func(ledger LedgerKind)
	visit = func(ledger LedgerKind) {
		if seen[ledger] {
			return
		}
		seen[ledger] = true
		prerequisites := pinMissingPrerequisites(graph, ledger)
		if len(prerequisites) == 0 {
			frontier = append(frontier, ledger)
			return
		}
		for _, prerequisite := range prerequisites {
			visit(LedgerKind(prerequisite))
		}
	}
	for _, ledger := range missing {
		visit(ledger)
	}
	return frontier
}

// pinApplyProducer executes one producer action symbolically: every ledger
// the action's capability declares is materialized on the facts, mirroring
// the runtime accumulation of result rows/records into ledger counts — but
// only for ledgers whose published missing prerequisites are empty. This
// mirrors the runtime hard-rejects (reconcile refuses an empty contribution
// ledger), so a pinned walk can never "succeed" through a production the
// runner is guaranteed to fail.
func pinApplyProducer(facts StageFacts, kind dataquery.DataActionKind) StageFacts {
	capability, ok := Capability(kind)
	if !ok {
		return facts
	}
	graph := BuildLedgerGraph(facts)
	for _, ledger := range capability.ProducesLedgers {
		if len(pinMissingPrerequisites(graph, ledger)) > 0 {
			continue
		}
		switch ledger {
		case LedgerRuleCoverage:
			if facts.RuleCoverageRecords == 0 {
				facts.RuleCoverageRecords = 1
			}
		case LedgerDecisions:
			if facts.DecisionRecords == 0 {
				facts.DecisionRecords = 1
			}
		case LedgerEntityResolutions:
			if facts.EntityResolutionRecords == 0 {
				facts.EntityResolutionRecords = 1
			}
		case LedgerContributions:
			if facts.ContributionRecords == 0 {
				facts.ContributionRecords = 1
			}
		case LedgerReconcile:
			facts.HasReconcile = true
		}
	}
	return facts
}

// TestEveryRequiredLedgerHasReachableProducer_NoTypedDeadlock walks every
// typed facts combination to completion using only admissible typed actions.
// If at any intermediate state a required-and-missing ledger has no legal
// producer in the allowed set, the gate table has a structural deadlock.
func TestEveryRequiredLedgerHasReachableProducer_NoTypedDeadlock(t *testing.T) {
	for _, start := range pinEnumerateStageFacts() {
		facts := start
		const maxSteps = 16
		for step := 0; step < maxSteps; step++ {
			missing := pinMissingRequiredLedgers(facts)
			if len(missing) == 0 {
				break
			}
			frontier := pinProductionFrontier(BuildLedgerGraph(facts), missing)
			contracts := pinAllowedTypedActions(facts)
			var producer dataquery.DataActionKind
			var produced LedgerKind
			for _, ledger := range frontier {
				for _, contract := range contracts {
					kind := dataquery.DataActionKind(contract.Kind)
					if ProducesLedger(kind, ledger) {
						producer = kind
						produced = ledger
						break
					}
				}
				if producer != "" {
					break
				}
			}
			if producer == "" {
				t.Fatalf("typed deadlock: required ledger(s) %v (production frontier %v) have no legal producer at next_stage=%s allowed=%v — every published surface recommends work the runtime is guaranteed to reject; start_facts=%+v current_facts=%+v",
					missing, frontier, NextStage(facts), ActionKindsFromContracts(contracts), start, facts)
			}
			next := pinApplyProducer(facts, producer)
			if next == facts {
				t.Fatalf("producer %s for ledger %s did not advance facts: %+v", producer, produced, facts)
			}
			facts = next
		}
		if missing := pinMissingRequiredLedgers(facts); len(missing) > 0 {
			t.Fatalf("required ledgers %v unreachable within %d steps from start_facts=%+v (stuck at %+v)", missing, maxSteps, start, facts)
		}
	}
}

// TestWorkflowDecisionNextActionsSubsetOfAllowed pins the advisory hard
// filter: for every facts combination, the published decision advisory is a
// subset of the enforced allowed set, including under mutation — a violation
// carrying repair hints outside the allowed set must not leak through.
func TestWorkflowDecisionNextActionsSubsetOfAllowed(t *testing.T) {
	for _, facts := range pinEnumerateStageFacts() {
		allowed := ActionKindsFromContracts(AllowedNextActionContractsForFacts(facts))
		allowedSet := map[string]bool{}
		for _, action := range allowed {
			allowedSet[action] = true
		}
		inputs := []WorkflowDecisionInput{
			{
				NextStage:          NextStage(facts),
				AllowedNextActions: allowed,
				LedgerGraph:        BuildLedgerGraph(facts),
			},
			// Mutation form: a violation whose repair hints advertise an
			// action the admission gate would reject for these facts.
			{
				NextStage:          NextStage(facts),
				AllowedNextActions: allowed,
				LedgerGraph:        BuildLedgerGraph(facts),
				Violations: []WorkflowViolation{{
					Code:              "pin_synthetic_violation",
					Severity:          "error",
					RepairActionHints: []string{string(dataquery.DataActionComputeContribs), "not_a_real_action"},
					Reason:            "synthetic mutation probe",
				}},
			},
		}
		for _, input := range inputs {
			decision := BuildWorkflowDecision(input)
			for _, action := range decision.NextActions {
				if _, known := Capability(dataquery.DataActionKind(action)); !known {
					t.Fatalf("published advisory entry %q is not a typed action kind for facts=%+v", action, facts)
				}
				if len(allowed) > 0 && !allowedSet[action] {
					t.Fatalf("published advisory action %q is outside the enforced allowed set %v for facts=%+v (violations=%v)",
						action, allowed, facts, decision.Violations)
				}
			}
		}
	}
}

// TestLedgerGraphContributionsMirrorsDecisionsGate pins graph/gate
// consistency: whenever the admission gate withholds compute_contributions
// pending decision records, the published contributions dependency must name
// decisions as a missing prerequisite — and vice versa.
func TestLedgerGraphContributionsMirrorsDecisionsGate(t *testing.T) {
	for _, facts := range pinEnumerateStageFacts() {
		graph := BuildLedgerGraph(facts)
		dep, ok := pinFindDependency(graph, LedgerContributions)
		if !ok {
			t.Fatalf("contributions dependency missing from ledger graph for facts=%+v", facts)
		}
		gateExcludes := DecisionsGateExcludesComputeContribs(facts)
		graphBlocksOnDecisions := pinContains(dep.MissingPrerequisites, string(LedgerDecisions))
		if gateExcludes != graphBlocksOnDecisions {
			t.Fatalf("gate/graph divergence: DecisionsGateExcludesComputeContribs=%v but contributions missing_prerequisites=%v for facts=%+v",
				gateExcludes, dep.MissingPrerequisites, facts)
		}
		if gateExcludes && !pinContains(dep.DependsOn, string(LedgerDecisions)) {
			t.Fatalf("contributions depends_on=%v omits decisions while the gate enforces that prerequisite for facts=%+v", dep.DependsOn, facts)
		}
	}
}

// TestDecisionAdvisoryRegression_DataBasicSumWithRules reproduces the exact
// stuck state of the 2026-07-05 blocked terminal and asserts every published
// surface now agrees on the one-action legal path.
func TestDecisionAdvisoryRegression_DataBasicSumWithRules(t *testing.T) {
	stuck := StageFacts{
		MaterialCoverageSufficient: true,
		RuleCoverageRequired:       true,
		RuleCoverageRecords:        2,
		DecisionRecordsRequired:    true,
		DecisionRecords:            0,
		EntityResolutionRequired:   true,
		EntityResolutionRecords:    4,
		ContributionLedgerRequired: true,
		ContributionRecords:        0,
		ReconcileRequired:          true,
	}
	allowed := ActionKindsFromContracts(AllowedNextActionContractsForFacts(stuck))
	if pinContains(allowed, string(dataquery.DataActionComputeContribs)) {
		t.Fatalf("compute_contributions must stay out of the allowed set while decision records are missing: %v", allowed)
	}
	for _, needed := range []string{string(dataquery.DataActionQualifyRecords), string(dataquery.DataActionFilterRecords)} {
		if !pinContains(allowed, needed) {
			t.Fatalf("decisions producer %s missing from allowed set %v", needed, allowed)
		}
	}
	decision := BuildWorkflowDecision(WorkflowDecisionInput{
		NextStage:          NextStage(stuck),
		AllowedNextActions: allowed,
		LedgerGraph:        BuildLedgerGraph(stuck),
	})
	if pinContains(decision.NextActions, string(dataquery.DataActionComputeContribs)) {
		t.Fatalf("published advisory still contains the gate-rejected compute_contributions: %v", decision.NextActions)
	}
	if !pinContains(decision.NextActions, string(dataquery.DataActionQualifyRecords)) &&
		!pinContains(decision.NextActions, string(dataquery.DataActionFilterRecords)) {
		t.Fatalf("published advisory %v names no admissible decisions producer", decision.NextActions)
	}
	graph := BuildLedgerGraph(stuck)
	dep, ok := pinFindDependency(graph, LedgerContributions)
	if !ok {
		t.Fatal("contributions dependency missing from ledger graph")
	}
	if dep.Status != LedgerStatusBlockedByPrerequisite || !pinContains(dep.MissingPrerequisites, string(LedgerDecisions)) {
		t.Fatalf("contributions dependency must be blocked on decisions: status=%s missing_prerequisites=%v", dep.Status, dep.MissingPrerequisites)
	}

	// One qualify_records/filter_records batch unlocks compute_contributions.
	unlocked := stuck
	unlocked.DecisionRecords = 2
	allowedAfter := ActionKindsFromContracts(AllowedNextActionContractsForFacts(unlocked))
	if !pinContains(allowedAfter, string(dataquery.DataActionComputeContribs)) {
		t.Fatalf("compute_contributions must return to the allowed set once decision records exist: %v", allowedAfter)
	}
	depAfter, _ := pinFindDependency(BuildLedgerGraph(unlocked), LedgerContributions)
	if pinContains(depAfter.MissingPrerequisites, string(LedgerDecisions)) {
		t.Fatalf("contributions must not stay blocked on satisfied decisions: %v", depAfter.MissingPrerequisites)
	}
}

// TestAnswerRepairLaneReachableOnCompletionShapedState pins the terminal
// completion gate vs repair lane consistency class (data_json_strict_ids /
// data_text_filter_count, 2026-07-05): whenever facts project stage=complete,
// the answer-repair action surface must be non-empty, must survive the
// custom-transform-disabled worst case, and must contain the recompute,
// reconcile, and projection repair actions so a typed evaluator repair_node
// always has a legal lane.
func TestAnswerRepairLaneReachableOnCompletionShapedState(t *testing.T) {
	completionShaped := 0
	for _, facts := range pinEnumerateStageFacts() {
		if NextStage(facts) != StageComplete {
			continue
		}
		completionShaped++
		lane := AnswerRepairActionContracts(facts)
		typedLane := FilterCustomTransformContracts(lane)
		if len(typedLane) == 0 {
			t.Fatalf("answer repair lane empty (after custom_transform exclusion) for completion-shaped facts=%+v", facts)
		}
		for _, needed := range []dataquery.DataActionKind{
			dataquery.DataActionComputeContribs,
			dataquery.DataActionReconcile,
			dataquery.DataActionAssembleAnswer,
		} {
			if !pinContains(ActionKindsFromContracts(typedLane), string(needed)) {
				t.Fatalf("answer repair lane %v lacks %s for facts=%+v", ActionKindsFromContracts(typedLane), needed, facts)
			}
		}
		for _, contract := range lane {
			if _, ok := Capability(dataquery.DataActionKind(contract.Kind)); !ok {
				t.Fatalf("answer repair lane advertises unknown action kind %q", contract.Kind)
			}
		}
	}
	// Anti-dead-code guard: without completion-shaped states in the
	// enumeration (the HasAnswer axis), every assertion above is skipped and
	// this test pins nothing. NextStage only projects "complete" when
	// HasAnswer is true and every required ledger is satisfied.
	if completionShaped == 0 {
		t.Fatal("enumeration produced no completion-shaped facts; the HasAnswer axis or completion routing regressed and all assertions were skipped")
	}
	t.Logf("completion-shaped facts covered: %d", completionShaped)
}

// TestAnswerRepairLaneWiredIntoWorkflowState pins the state-builder wiring: a
// completion-shaped state with an actionable evaluator repair_node must not
// publish allowed_next_actions=[], while the same state without a pending
// repair keeps the plain completion projection.
func TestAnswerRepairLaneWiredIntoWorkflowState(t *testing.T) {
	build := func(latest *dataquery.Evaluation) WorkflowStateView {
		return BuildWorkflowStateView(WorkflowStateViewBuildInput{
			HasMaterialProgress: true,
			LedgerCounts:        WorkflowStateLedgerCounts{HasAnswer: true},
			LatestEvaluation:    latest,
		})
	}
	actionable := &dataquery.Evaluation{
		Status:     dataquery.EvalRepairNode,
		Confidence: "high",
		ActionID:   "complete_output_contract_answer",
		ActionKind: string(dataquery.DataActionAssembleAnswer),
		Reason:     "output contract violation: wrong field name in published payload",
	}
	repairState := build(actionable)
	if repairState.NextStage != StageComplete {
		t.Fatalf("test setup: next_stage=%s, want complete", repairState.NextStage)
	}
	if !repairState.AnswerRepairLaneActive || len(repairState.AllowedNextActions) == 0 {
		t.Fatalf("actionable repair_node on completion-shaped state must open the repair lane: active=%v allowed=%v",
			repairState.AnswerRepairLaneActive, repairState.AllowedNextActions)
	}
	if !pinContains(repairState.AllowedNextActions, string(dataquery.DataActionAssembleAnswer)) {
		t.Fatalf("repair lane allowed set %v lacks assemble_answer", repairState.AllowedNextActions)
	}
	for _, action := range repairState.Decision.NextActions {
		if !pinContains(repairState.AllowedNextActions, action) {
			t.Fatalf("repair-lane decision advisory %q outside allowed set %v", action, repairState.AllowedNextActions)
		}
	}
	plainState := build(nil)
	if plainState.AnswerRepairLaneActive || len(plainState.AllowedNextActions) != 0 {
		t.Fatalf("completion-shaped state without pending repair must keep the plain completion projection: active=%v allowed=%v",
			plainState.AnswerRepairLaneActive, plainState.AllowedNextActions)
	}
	nonActionable := &dataquery.Evaluation{Status: dataquery.EvalRepairNode, Confidence: "low", Reason: "vague unease"}
	vagueState := build(nonActionable)
	if vagueState.AnswerRepairLaneActive {
		t.Fatal("unanchored low-confidence repair_node must not open the repair lane (typed escape stays precise)")
	}
}

func pinFindDependency(graph LedgerGraph, ledger LedgerKind) (LedgerDependency, bool) {
	for _, dep := range graph.Dependencies {
		if dep.Ledger == string(ledger) {
			return dep, true
		}
	}
	return LedgerDependency{}, false
}

func pinContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
