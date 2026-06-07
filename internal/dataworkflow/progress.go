package dataworkflow

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

const ViolationStageNoProgress = "stage_no_progress"

// ProgressEvent is the compact workflow-history view needed by convergence
// checks. It carries typed action and ledger facts only; UI records and model
// prose stay outside the hard policy layer.
type ProgressEvent struct {
	Actions                 []dataquery.DataAction `json:"actions,omitempty"`
	ResultPresent           bool                   `json:"result_present,omitempty"`
	Error                   string                 `json:"error,omitempty"`
	ArtifactCount           int                    `json:"artifact_count,omitempty"`
	ArtifactRows            int                    `json:"artifact_rows,omitempty"`
	ArtifactFields          []string               `json:"artifact_fields,omitempty"`
	DecisionRecords         int                    `json:"decision_records,omitempty"`
	RuleCoverageRecords     int                    `json:"rule_coverage_records,omitempty"`
	EntityResolutionRecords int                    `json:"entity_resolution_records,omitempty"`
	ContributionRecords     int                    `json:"contribution_records,omitempty"`
	HasReconcile            bool                   `json:"has_reconcile,omitempty"`
	AnswerPresent           bool                   `json:"answer_present,omitempty"`
	Stage                   string                 `json:"stage,omitempty"`
}

func IsRelationMaterialization(kind dataquery.DataActionKind) bool {
	switch NormalizeActionKind(kind) {
	case dataquery.DataActionMappingCandidate, dataquery.DataActionApplyResolutions, dataquery.DataActionEnrichRecords, dataquery.DataActionJoinRecords:
		return true
	default:
		return false
	}
}

func SingleRelationMaterializationKind(actions []dataquery.DataAction) (string, bool) {
	if len(actions) == 0 {
		return "", false
	}
	var found string
	for _, action := range actions {
		kind := NormalizeActionKind(action.Kind)
		if !IsRelationMaterialization(kind) {
			return "", false
		}
		kindText := string(kind)
		if found == "" {
			found = kindText
			continue
		}
		if found != kindText {
			found = "relation_materialization"
		}
	}
	return found, true
}

func RecentRelationNoProgressCount(events []ProgressEvent) (int, []string) {
	count := 0
	seen := make(map[string]bool)
	var kinds []string
	var latestSignature string
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if !event.ResultPresent || strings.TrimSpace(event.Error) != "" {
			break
		}
		if event.ContributionRecords > 0 || event.HasReconcile {
			break
		}
		kind, ok := SingleRelationMaterializationKind(event.Actions)
		if !ok {
			break
		}
		signature := ProgressSignature(event)
		if latestSignature == "" {
			latestSignature = signature
		} else if signature != latestSignature {
			break
		}
		count++
		if !seen[kind] {
			seen[kind] = true
			kinds = append(kinds, kind)
		}
	}
	sort.Strings(kinds)
	return count, kinds
}

func ProgressSignature(event ProgressEvent) string {
	fields := cleanStrings(event.ArtifactFields)
	sort.Strings(fields)
	return fmt.Sprintf("stage=%s artifacts=%d rows=%d fields=%s rule=%d decisions=%d entities=%d contrib=%d reconcile=%t answer=%t",
		strings.TrimSpace(event.Stage),
		event.ArtifactCount,
		event.ArtifactRows,
		strings.Join(fields, ","),
		event.RuleCoverageRecords,
		event.DecisionRecords,
		event.EntityResolutionRecords,
		event.ContributionRecords,
		event.HasReconcile,
		event.AnswerPresent)
}

func RelationNoProgressViolation(facts StageFacts, events []ProgressEvent, threshold int) (WorkflowViolation, bool) {
	if facts.NextStage() != StagePrepareContributionInputs && facts.NextStage() != StageComputeContributions {
		return WorkflowViolation{}, false
	}
	if (!facts.ContributionLedgerRequired && !facts.ReconcileRequired) || facts.ContributionRecords > 0 || facts.HasReconcile {
		return WorkflowViolation{}, false
	}
	if threshold <= 0 {
		threshold = 1
	}
	count, kinds := RecentRelationNoProgressCount(events)
	if count < threshold {
		return WorkflowViolation{}, false
	}
	actionKind := "relation_materialization"
	if len(kinds) == 1 {
		actionKind = kinds[0]
	}
	return WorkflowViolation{
		Code:          ViolationStageNoProgress,
		Severity:      "warning",
		Repairability: RepairNeedsTypedAction,
		ActionKind:    actionKind,
		RepairActionHints: []string{
			string(dataquery.DataActionDeriveFields),
			string(dataquery.DataActionExtractFields),
			string(dataquery.DataActionFilterRecords),
			string(dataquery.DataActionQualifyRecords),
			string(dataquery.DataActionComputeContribs),
			string(dataquery.DataActionReconcile),
		},
		Reason: fmt.Sprintf("the workflow is still at %s after %d recent relation materialization result(s) [%s] without contribution or reconcile progress; stop automatic apply/enrich/join scaffolds and repair from artifact fields, eligibility filters, contribution calculation, or reconciliation", facts.NextStage(), count, strings.Join(kinds, ", ")),
	}, true
}

func WouldRepeatRelationNoProgress(facts StageFacts, events []ProgressEvent, actionKind dataquery.DataActionKind, threshold int) bool {
	if !IsRelationMaterialization(actionKind) {
		return false
	}
	if facts.NextStage() != StagePrepareContributionInputs && facts.NextStage() != StageComputeContributions {
		return false
	}
	if (!facts.ContributionLedgerRequired && !facts.ReconcileRequired) || facts.ContributionRecords > 0 || facts.HasReconcile {
		return false
	}
	if threshold <= 0 {
		threshold = 1
	}
	count, _ := RecentRelationNoProgressCount(events)
	return count >= threshold
}

func (facts StageFacts) NextStage() string {
	return NextStage(facts)
}
