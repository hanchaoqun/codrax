package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

const (
	LedgerStatusOptional              = "optional"
	LedgerStatusSatisfied             = "satisfied"
	LedgerStatusMissing               = "missing"
	LedgerStatusBlockedByPrerequisite = "blocked_by_prerequisite"

	LedgerPrerequisiteMaterials = "materials"
)

// BuildLedgerGraph converts structural stage facts into a durable ledger
// dependency contract. Business meaning stays with the model; this graph only
// describes which generic validation ledgers are required, present, blocked,
// and which typed actions can produce them.
func BuildLedgerGraph(facts StageFacts) LedgerGraph {
	contributionRequired := facts.ContributionLedgerNeeded()
	reconcileRequired := facts.ReconcileNeeded()
	ruleLinkageClosed := !facts.RuleLedgerLinkageRequired || facts.RuleLedgerLinkageSatisfied
	decisionPresent := facts.DecisionRecords > 0 && ruleLinkageClosed
	entityPresent := facts.EntityLedgerSatisfied() && ruleLinkageClosed
	contributionPresent := facts.ContributionRecords > 0 && ruleLinkageClosed
	reconcilePresent := facts.HasReconcile && ruleLinkageClosed
	answerPresent := facts.HasAnswer && ruleLinkageClosed
	var finalProjectionProducers []dataquery.DataActionKind
	if reconcilePresent {
		finalProjectionProducers = append(finalProjectionProducers, dataquery.DataActionAssembleAnswer)
	}
	if !facts.CustomTransformDisabled {
		finalProjectionProducers = append(finalProjectionProducers, dataquery.DataActionCustomTransform)
	}
	deps := []LedgerDependency{
		buildLedgerDependency(ledgerDependencyInput{
			Ledger:          LedgerRuleCoverage,
			Required:        facts.RuleCoverageRequired,
			Present:         facts.RuleCoverageRecords > 0,
			Count:           facts.RuleCoverageRecords,
			Stage:           StageDeriveRules,
			ProducesActions: []dataquery.DataActionKind{dataquery.DataActionDeriveRules},
			MissingPrerequisites: ledgerPrerequisites(
				ledgerPrerequisite{Enabled: !facts.MaterialCoverageSufficient, Value: LedgerPrerequisiteMaterials},
			),
		}),
		buildLedgerDependency(ledgerDependencyInput{
			Ledger:          LedgerDecisions,
			Required:        facts.DecisionRecordsRequired,
			Present:         decisionPresent,
			Count:           facts.DecisionRecords,
			Stage:           StagePrepareContributionInputs,
			ProducesActions: []dataquery.DataActionKind{dataquery.DataActionFilterRecords, dataquery.DataActionQualifyRecords, dataquery.DataActionComputeContribs},
			DependsOn:       ledgerDependsOn(facts.RuleCoverageRequired, LedgerRuleCoverage),
			MissingPrerequisites: ledgerPrerequisites(
				ledgerPrerequisite{Enabled: !facts.MaterialCoverageSufficient, Value: LedgerPrerequisiteMaterials},
				ledgerPrerequisite{Enabled: facts.RuleCoverageRequired && facts.RuleCoverageRecords == 0, Value: string(LedgerRuleCoverage)},
			),
		}),
		buildLedgerDependency(ledgerDependencyInput{
			Ledger:   LedgerEntityResolutions,
			Required: facts.EntityResolutionRequired,
			// Present MUST be the single shared obligation predicate: this
			// published face and the dataquery validator once re-derived
			// satisfaction differently and deadlocked the workflow
			// (GAP-3/G10, §29.142).
			Present:         entityPresent,
			Count:           facts.EntityResolutionRecords,
			Stage:           StageNormalizeOrEnrichEntities,
			ProducesActions: []dataquery.DataActionKind{dataquery.DataActionNormalizeEntities, dataquery.DataActionEnrichRecords},
			MissingPrerequisites: ledgerPrerequisites(
				ledgerPrerequisite{Enabled: !facts.MaterialCoverageSufficient, Value: LedgerPrerequisiteMaterials},
			),
		}),
		buildLedgerDependency(ledgerDependencyInput{
			Ledger:          LedgerContributions,
			Required:        contributionRequired,
			Present:         contributionPresent,
			Count:           facts.ContributionRecords,
			Stage:           StageComputeContributions,
			ProducesActions: []dataquery.DataActionKind{dataquery.DataActionComputeContribs},
			DependsOn: cleanStrings([]string{
				conditionalLedgerDependency(facts.RuleCoverageRequired, LedgerRuleCoverage),
				// Mirror of DecisionsGateExcludesComputeContribs: when the
				// admission gate withholds compute_contributions until
				// decision records exist, the published graph must state the
				// same decisions→contributions dependency. Omitting it made
				// the graph deny the very prerequisite the gate enforced,
				// so planners read "contributions unblocked, sole producer
				// compute_contributions" while admission rejected that
				// producer — a self-contradictory projection that caused a
				// blocked terminal despite a one-action legal path.
				conditionalLedgerDependency(facts.RuleCoverageRequired && facts.DecisionRecordsRequired, LedgerDecisions),
				conditionalLedgerDependency(facts.EntityResolutionRequired, LedgerEntityResolutions),
			}),
			MissingPrerequisites: ledgerPrerequisites(
				ledgerPrerequisite{Enabled: !facts.MaterialCoverageSufficient, Value: LedgerPrerequisiteMaterials},
				ledgerPrerequisite{Enabled: facts.RuleCoverageRequired && facts.RuleCoverageRecords == 0 && !facts.HasPostRuleProgress(), Value: string(LedgerRuleCoverage)},
				ledgerPrerequisite{Enabled: DecisionsGateExcludesComputeContribs(facts), Value: string(LedgerDecisions)},
				ledgerPrerequisite{Enabled: facts.EntityResolutionRequired && !facts.EntityLedgerSatisfied(), Value: string(LedgerEntityResolutions)},
			),
		}),
		buildLedgerDependency(ledgerDependencyInput{
			Ledger:          LedgerReconcile,
			Required:        reconcileRequired,
			Present:         reconcilePresent,
			Count:           boolCount(reconcilePresent),
			Stage:           StageReconcileArtifacts,
			ProducesActions: []dataquery.DataActionKind{dataquery.DataActionReconcile},
			DependsOn:       ledgerDependsOn(true, LedgerContributions),
			MissingPrerequisites: ledgerPrerequisites(
				ledgerPrerequisite{Enabled: !facts.MaterialCoverageSufficient, Value: LedgerPrerequisiteMaterials},
				ledgerPrerequisite{Enabled: !contributionPresent, Value: string(LedgerContributions)},
			),
		}),
		buildLedgerDependency(ledgerDependencyInput{
			Ledger:          LedgerFinalProjection,
			Required:        true,
			Present:         answerPresent,
			Count:           boolCount(answerPresent),
			Stage:           StageEmitOutputContractAnswer,
			ProducesActions: finalProjectionProducers,
			DependsOn: cleanStrings([]string{
				conditionalLedgerDependency(facts.RuleCoverageRequired, LedgerRuleCoverage),
				conditionalLedgerDependency(facts.DecisionRecordsRequired, LedgerDecisions),
				conditionalLedgerDependency(facts.EntityResolutionRequired, LedgerEntityResolutions),
				conditionalLedgerDependency(contributionRequired, LedgerContributions),
				conditionalLedgerDependency(reconcileRequired, LedgerReconcile),
			}),
			MissingPrerequisites: ledgerPrerequisites(
				ledgerPrerequisite{Enabled: !facts.MaterialCoverageSufficient, Value: LedgerPrerequisiteMaterials},
				ledgerPrerequisite{Enabled: facts.RuleCoverageRequired && facts.RuleCoverageRecords == 0, Value: string(LedgerRuleCoverage)},
				ledgerPrerequisite{Enabled: facts.DecisionRecordsRequired && !decisionPresent, Value: string(LedgerDecisions)},
				ledgerPrerequisite{Enabled: facts.EntityResolutionRequired && !entityPresent, Value: string(LedgerEntityResolutions)},
				ledgerPrerequisite{Enabled: contributionRequired && !contributionPresent, Value: string(LedgerContributions)},
				ledgerPrerequisite{Enabled: reconcileRequired && !reconcilePresent, Value: string(LedgerReconcile)},
			),
		}),
	}
	graph := LedgerGraph{
		RuleCoverage:      statusForLedger(deps, LedgerRuleCoverage),
		Decisions:         statusForLedger(deps, LedgerDecisions),
		EntityResolutions: statusForLedger(deps, LedgerEntityResolutions),
		Contributions:     statusForLedger(deps, LedgerContributions),
		Reconcile:         statusForLedger(deps, LedgerReconcile),
		FinalProjection:   statusForLedger(deps, LedgerFinalProjection),
		Dependencies:      deps,
		NextStage:         NextStage(facts),
	}
	graph.FirstMissing = FirstMissingLedger(graph)
	return graph
}

func FirstMissingLedger(graph LedgerGraph) string {
	for _, dep := range graph.Dependencies {
		if !dep.Required || dep.Present {
			continue
		}
		if dep.Status == LedgerStatusMissing || dep.Status == LedgerStatusBlockedByPrerequisite {
			return dep.Ledger
		}
	}
	return ""
}

func LedgerGraphCompletionGuardResult(graph LedgerGraph) GuardResult {
	dep, ok := FirstIncompleteRequiredLedger(graph)
	if !ok {
		return GuardResult{}
	}
	code := "missing_workflow_ledger"
	if dep.Status == LedgerStatusBlockedByPrerequisite {
		code = "blocked_workflow_ledger"
	}
	message := ledgerCompletionMessage(dep)
	action := dataquery.DataAction{
		ID:   "complete_" + strings.ReplaceAll(strings.TrimSpace(dep.Ledger), "/", "_"),
		Kind: firstLedgerProducerAction(dep.ProducesActions),
	}
	violation := NewActionInputViolation(
		code,
		"error",
		RepairNeedsTypedAction,
		action,
		"",
		nil,
		message,
		dep.ProducesActions,
	)
	return NewGuardResult(code, "error", RepairNeedsTypedAction, message, violation)
}

func FirstIncompleteRequiredLedger(graph LedgerGraph) (LedgerDependency, bool) {
	deps := IncompleteRequiredLedgers(graph)
	if len(deps) == 0 {
		return LedgerDependency{}, false
	}
	return deps[0], true
}

// PlanDirectlyProducesFirstMissingLedger is the precise authority boundary for
// deterministic post-result dispatch. A scaffold can be executable without
// advancing the workflow's first missing ledger (for example another schema
// distribution or relation candidate while contributions are the sole open
// book). Such actions remain useful typed choices for the evaluator/planner,
// but the system must not select them automatically and starve the published
// producer indefinitely. Every action in an automatically dispatched plan
// therefore has to be one of the first incomplete ledger's declared producers.
func PlanDirectlyProducesFirstMissingLedger(plan dataquery.TaskPlan, graph LedgerGraph) bool {
	dep, ok := FirstIncompleteRequiredLedger(graph)
	if !ok || len(plan.Actions) == 0 || len(dep.ProducesActions) == 0 {
		return false
	}
	producers := map[dataquery.DataActionKind]bool{}
	for _, kind := range dep.ProducesActions {
		normalized := NormalizeActionKind(dataquery.DataActionKind(strings.TrimSpace(kind)))
		if normalized != "" {
			producers[normalized] = true
		}
	}
	if len(producers) == 0 {
		return false
	}
	for _, action := range plan.Actions {
		if !producers[NormalizeActionKind(action.Kind)] {
			return false
		}
	}
	return true
}

// IncompleteRequiredLedgers — GAP-EVAL-D1(b) (audit 2026-07-26): the FULL
// incomplete roster in graph order. The one-at-a-time first_missing read
// made every repair round discover exactly one more obligation (the
// EMITBURN shape — the eval specimen burned 18 batches unlocking four
// ledgers serially with the correct answer already in hand); disclosure
// faces now name the whole map at once so one round can plan the whole
// backfill. FirstIncompleteRequiredLedger stays the single-dependency
// authority for gates that genuinely dispatch on the first blocker.
func IncompleteRequiredLedgers(graph LedgerGraph) []LedgerDependency {
	var out []LedgerDependency
	for _, dep := range graph.Dependencies {
		if !dep.Required || dep.Present {
			continue
		}
		if dep.Status == LedgerStatusMissing || dep.Status == LedgerStatusBlockedByPrerequisite {
			out = append(out, dep)
		}
	}
	return out
}

// LedgerCompletionMessageAll renders the full-roster incompleteness message
// (GAP-EVAL-D1(b)): every missing/blocked required ledger with its status
// and producers, so the repair loop never has to rediscover the map one
// round at a time.
func LedgerCompletionMessageAll(graph LedgerGraph) string {
	deps := IncompleteRequiredLedgers(graph)
	if len(deps) == 0 {
		return ""
	}
	if len(deps) == 1 {
		return ledgerCompletionMessage(deps[0])
	}
	parts := make([]string, 0, len(deps))
	for _, dep := range deps {
		ledger := strings.TrimSpace(dep.Ledger)
		if ledger == string(LedgerFinalProjection) {
			ledger = "final answer projection"
		}
		clause := fmt.Sprintf("%s is %s", ledger, dep.Status)
		var detail []string
		if len(dep.MissingPrerequisites) > 0 {
			detail = append(detail, fmt.Sprintf("missing_prerequisites=[%s]", strings.Join(dep.MissingPrerequisites, ", ")))
		}
		if len(dep.ProducesActions) > 0 {
			detail = append(detail, fmt.Sprintf("producer_actions=[%s]", strings.Join(dep.ProducesActions, ", ")))
		}
		if len(detail) > 0 {
			clause += " (" + strings.Join(detail, "; ") + ")"
		}
		parts = append(parts, clause)
	}
	return fmt.Sprintf("data validation incomplete: %d required ledgers unfinished — %s", len(deps), strings.Join(parts, "; "))
}

func ledgerCompletionMessage(dep LedgerDependency) string {
	ledger := strings.TrimSpace(dep.Ledger)
	if ledger == string(LedgerFinalProjection) {
		ledger = "final answer projection"
	}
	var parts []string
	if len(dep.MissingPrerequisites) > 0 {
		parts = append(parts, fmt.Sprintf("missing_prerequisites=[%s]", strings.Join(dep.MissingPrerequisites, ", ")))
	}
	if len(dep.ProducesActions) > 0 {
		parts = append(parts, fmt.Sprintf("producer_actions=[%s]", strings.Join(dep.ProducesActions, ", ")))
	}
	detail := ""
	if len(parts) > 0 {
		detail = " (" + strings.Join(parts, "; ") + ")"
	}
	return fmt.Sprintf("data validation incomplete: required ledger %s is %s%s", ledger, dep.Status, detail)
}

func firstLedgerProducerAction(actions []string) dataquery.DataActionKind {
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action != "" {
			return dataquery.DataActionKind(action)
		}
	}
	return ""
}

type ledgerDependencyInput struct {
	Ledger               LedgerKind
	Required             bool
	Present              bool
	Count                int
	Stage                string
	ProducesActions      []dataquery.DataActionKind
	DependsOn            []string
	MissingPrerequisites []string
}

func buildLedgerDependency(input ledgerDependencyInput) LedgerDependency {
	status := LedgerStatusOptional
	if input.Required {
		switch {
		case input.Present:
			status = LedgerStatusSatisfied
		case len(input.MissingPrerequisites) > 0:
			status = LedgerStatusBlockedByPrerequisite
		default:
			status = LedgerStatusMissing
		}
	}
	return LedgerDependency{
		Ledger:               string(input.Ledger),
		Required:             input.Required,
		Present:              input.Present,
		Count:                input.Count,
		Status:               status,
		Stage:                input.Stage,
		ProducesActions:      dataActionKindStrings(input.ProducesActions),
		DependsOn:            cleanStrings(input.DependsOn),
		MissingPrerequisites: cleanStrings(input.MissingPrerequisites),
	}
}

func statusForLedger(deps []LedgerDependency, ledger LedgerKind) LedgerStatus {
	for _, dep := range deps {
		if dep.Ledger != string(ledger) {
			continue
		}
		return LedgerStatus{
			Required:             dep.Required,
			Present:              dep.Present,
			Count:                dep.Count,
			Status:               dep.Status,
			Stage:                dep.Stage,
			ProducesActions:      append([]string(nil), dep.ProducesActions...),
			DependsOn:            append([]string(nil), dep.DependsOn...),
			MissingPrerequisites: append([]string(nil), dep.MissingPrerequisites...),
		}
	}
	return LedgerStatus{}
}

func ledgerDependsOn(include bool, ledger LedgerKind) []string {
	if !include {
		return nil
	}
	return []string{string(ledger)}
}

func conditionalLedgerDependency(include bool, ledger LedgerKind) string {
	if !include {
		return ""
	}
	return string(ledger)
}

type ledgerPrerequisite struct {
	Enabled bool
	Value   string
}

func ledgerPrerequisites(candidates ...ledgerPrerequisite) []string {
	var out []string
	for _, candidate := range candidates {
		if candidate.Enabled && candidate.Value != "" {
			out = append(out, candidate.Value)
		}
	}
	return cleanStrings(out)
}

func dataActionKindStrings(values []dataquery.DataActionKind) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, string(value))
		}
	}
	return cleanStrings(out)
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
