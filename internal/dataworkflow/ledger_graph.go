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
			Present:         facts.DecisionRecords > 0,
			Count:           facts.DecisionRecords,
			Stage:           StagePrepareContributionInputs,
			ProducesActions: []dataquery.DataActionKind{dataquery.DataActionQualifyRecords, dataquery.DataActionComputeContribs},
			DependsOn:       ledgerDependsOn(facts.RuleCoverageRequired, LedgerRuleCoverage),
			MissingPrerequisites: ledgerPrerequisites(
				ledgerPrerequisite{Enabled: !facts.MaterialCoverageSufficient, Value: LedgerPrerequisiteMaterials},
				ledgerPrerequisite{Enabled: facts.RuleCoverageRequired && facts.RuleCoverageRecords == 0, Value: string(LedgerRuleCoverage)},
			),
		}),
		buildLedgerDependency(ledgerDependencyInput{
			Ledger:          LedgerEntityResolutions,
			Required:        facts.EntityResolutionRequired,
			Present:         facts.EntityResolutionRecords > 0 || facts.EntityStageMaterialized,
			Count:           facts.EntityResolutionRecords,
			Stage:           StageNormalizeOrEnrichEntities,
			ProducesActions: []dataquery.DataActionKind{dataquery.DataActionNormalizeEntities, dataquery.DataActionEnrichRecords},
			MissingPrerequisites: ledgerPrerequisites(
				ledgerPrerequisite{Enabled: !facts.MaterialCoverageSufficient, Value: LedgerPrerequisiteMaterials},
			),
		}),
		buildLedgerDependency(ledgerDependencyInput{
			Ledger:          LedgerContributions,
			Required:        facts.ContributionLedgerRequired,
			Present:         facts.ContributionRecords > 0,
			Count:           facts.ContributionRecords,
			Stage:           StageComputeContributions,
			ProducesActions: []dataquery.DataActionKind{dataquery.DataActionComputeContribs},
			DependsOn: cleanStrings([]string{
				conditionalLedgerDependency(facts.RuleCoverageRequired, LedgerRuleCoverage),
				conditionalLedgerDependency(facts.EntityResolutionRequired, LedgerEntityResolutions),
			}),
			MissingPrerequisites: ledgerPrerequisites(
				ledgerPrerequisite{Enabled: !facts.MaterialCoverageSufficient, Value: LedgerPrerequisiteMaterials},
				ledgerPrerequisite{Enabled: facts.RuleCoverageRequired && facts.RuleCoverageRecords == 0 && !facts.HasPostRuleProgress(), Value: string(LedgerRuleCoverage)},
				ledgerPrerequisite{Enabled: facts.EntityResolutionRequired && facts.EntityResolutionRecords == 0 && !facts.EntityStageMaterialized, Value: string(LedgerEntityResolutions)},
			),
		}),
		buildLedgerDependency(ledgerDependencyInput{
			Ledger:          LedgerReconcile,
			Required:        facts.ReconcileRequired,
			Present:         facts.HasReconcile,
			Count:           boolCount(facts.HasReconcile),
			Stage:           StageReconcileArtifacts,
			ProducesActions: []dataquery.DataActionKind{dataquery.DataActionReconcile},
			DependsOn:       ledgerDependsOn(true, LedgerContributions),
			MissingPrerequisites: ledgerPrerequisites(
				ledgerPrerequisite{Enabled: !facts.MaterialCoverageSufficient, Value: LedgerPrerequisiteMaterials},
				ledgerPrerequisite{Enabled: facts.ContributionRecords == 0, Value: string(LedgerContributions)},
			),
		}),
		buildLedgerDependency(ledgerDependencyInput{
			Ledger:          LedgerFinalProjection,
			Required:        true,
			Present:         facts.HasAnswer,
			Count:           boolCount(facts.HasAnswer),
			Stage:           StageEmitOutputContractAnswer,
			ProducesActions: []dataquery.DataActionKind{dataquery.DataActionAssembleAnswer},
			DependsOn: cleanStrings([]string{
				conditionalLedgerDependency(facts.RuleCoverageRequired, LedgerRuleCoverage),
				conditionalLedgerDependency(facts.DecisionRecordsRequired, LedgerDecisions),
				conditionalLedgerDependency(facts.EntityResolutionRequired, LedgerEntityResolutions),
				conditionalLedgerDependency(facts.ContributionLedgerRequired, LedgerContributions),
				conditionalLedgerDependency(facts.ReconcileRequired, LedgerReconcile),
			}),
			MissingPrerequisites: ledgerPrerequisites(
				ledgerPrerequisite{Enabled: !facts.MaterialCoverageSufficient, Value: LedgerPrerequisiteMaterials},
				ledgerPrerequisite{Enabled: facts.RuleCoverageRequired && facts.RuleCoverageRecords == 0, Value: string(LedgerRuleCoverage)},
				ledgerPrerequisite{Enabled: facts.DecisionRecordsRequired && facts.DecisionRecords == 0, Value: string(LedgerDecisions)},
				ledgerPrerequisite{Enabled: facts.EntityResolutionRequired && facts.EntityResolutionRecords == 0 && !facts.EntityStageMaterialized, Value: string(LedgerEntityResolutions)},
				ledgerPrerequisite{Enabled: facts.ContributionLedgerRequired && facts.ContributionRecords == 0, Value: string(LedgerContributions)},
				ledgerPrerequisite{Enabled: facts.ReconcileRequired && !facts.HasReconcile, Value: string(LedgerReconcile)},
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
	for _, dep := range graph.Dependencies {
		if !dep.Required || dep.Present {
			continue
		}
		if dep.Status == LedgerStatusMissing || dep.Status == LedgerStatusBlockedByPrerequisite {
			return dep, true
		}
	}
	return LedgerDependency{}, false
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
