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
	Round                   int                    `json:"round,omitempty"`
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

type ProgressWindow struct {
	Latest                 ProgressFrame   `json:"latest,omitempty"`
	Recent                 []ProgressFrame `json:"recent,omitempty"`
	LatestDelta            ProgressDelta   `json:"latest_delta,omitempty"`
	RepeatedSignatureCount int             `json:"repeated_signature_count,omitempty"`
	LatestSignature        string          `json:"latest_signature,omitempty"`
}

type ProgressFrame struct {
	Round                   int      `json:"round,omitempty"`
	ActionKinds             []string `json:"action_kinds,omitempty"`
	Stage                   string   `json:"stage,omitempty"`
	ArtifactCount           int      `json:"artifact_count,omitempty"`
	ArtifactRows            int      `json:"artifact_rows,omitempty"`
	ArtifactFields          []string `json:"artifact_fields,omitempty"`
	DecisionRecords         int      `json:"decision_records,omitempty"`
	RuleCoverageRecords     int      `json:"rule_coverage_records,omitempty"`
	EntityResolutionRecords int      `json:"entity_resolution_records,omitempty"`
	ContributionRecords     int      `json:"contribution_records,omitempty"`
	HasReconcile            bool     `json:"has_reconcile,omitempty"`
	AnswerPresent           bool     `json:"answer_present,omitempty"`
	Signature               string   `json:"signature,omitempty"`
}

type ProgressDelta struct {
	StageChanged          bool     `json:"stage_changed,omitempty"`
	SignatureChanged      bool     `json:"signature_changed,omitempty"`
	ArtifactCountDelta    int      `json:"artifact_count_delta,omitempty"`
	ArtifactRowsDelta     int      `json:"artifact_rows_delta,omitempty"`
	AddedFields           []string `json:"added_fields,omitempty"`
	RemovedFields         []string `json:"removed_fields,omitempty"`
	DecisionRecordsDelta  int      `json:"decision_records_delta,omitempty"`
	RuleCoverageDelta     int      `json:"rule_coverage_delta,omitempty"`
	EntityResolutionDelta int      `json:"entity_resolution_delta,omitempty"`
	ContributionDelta     int      `json:"contribution_delta,omitempty"`
	ReconcileChanged      bool     `json:"reconcile_changed,omitempty"`
	AnswerPresenceChanged bool     `json:"answer_presence_changed,omitempty"`
}

func BuildProgressWindow(events []ProgressEvent, limit int) ProgressWindow {
	if len(events) == 0 {
		return ProgressWindow{}
	}
	if limit <= 0 {
		limit = 4
	}
	frames := make([]ProgressFrame, 0, len(events))
	for _, event := range events {
		if !event.ResultPresent || strings.TrimSpace(event.Error) != "" {
			continue
		}
		frames = append(frames, ProgressFrameFromEvent(event))
	}
	if len(frames) == 0 {
		return ProgressWindow{}
	}
	latest := frames[len(frames)-1]
	start := len(frames) - limit
	if start < 0 {
		start = 0
	}
	recent := append([]ProgressFrame(nil), frames[start:]...)
	repeated := 1
	for i := len(frames) - 2; i >= 0; i-- {
		if frames[i].Signature != latest.Signature {
			break
		}
		repeated++
	}
	var delta ProgressDelta
	if len(frames) >= 2 {
		delta = ProgressDeltaBetween(frames[len(frames)-2], latest)
	}
	return ProgressWindow{
		Latest:                 latest,
		Recent:                 recent,
		LatestDelta:            delta,
		RepeatedSignatureCount: repeated,
		LatestSignature:        latest.Signature,
	}
}

func ProgressFrameFromEvent(event ProgressEvent) ProgressFrame {
	fields := cleanStrings(event.ArtifactFields)
	sort.Strings(fields)
	return ProgressFrame{
		Round:                   event.Round,
		ActionKinds:             ProgressActionKinds(event.Actions),
		Stage:                   strings.TrimSpace(event.Stage),
		ArtifactCount:           event.ArtifactCount,
		ArtifactRows:            event.ArtifactRows,
		ArtifactFields:          fields,
		DecisionRecords:         event.DecisionRecords,
		RuleCoverageRecords:     event.RuleCoverageRecords,
		EntityResolutionRecords: event.EntityResolutionRecords,
		ContributionRecords:     event.ContributionRecords,
		HasReconcile:            event.HasReconcile,
		AnswerPresent:           event.AnswerPresent,
		Signature:               ProgressSignature(event),
	}
}

func ProgressDeltaBetween(previous, current ProgressFrame) ProgressDelta {
	added, removed := fieldSetDelta(previous.ArtifactFields, current.ArtifactFields)
	return ProgressDelta{
		StageChanged:          strings.TrimSpace(previous.Stage) != strings.TrimSpace(current.Stage),
		SignatureChanged:      strings.TrimSpace(previous.Signature) != strings.TrimSpace(current.Signature),
		ArtifactCountDelta:    current.ArtifactCount - previous.ArtifactCount,
		ArtifactRowsDelta:     current.ArtifactRows - previous.ArtifactRows,
		AddedFields:           added,
		RemovedFields:         removed,
		DecisionRecordsDelta:  current.DecisionRecords - previous.DecisionRecords,
		RuleCoverageDelta:     current.RuleCoverageRecords - previous.RuleCoverageRecords,
		EntityResolutionDelta: current.EntityResolutionRecords - previous.EntityResolutionRecords,
		ContributionDelta:     current.ContributionRecords - previous.ContributionRecords,
		ReconcileChanged:      previous.HasReconcile != current.HasReconcile,
		AnswerPresenceChanged: previous.AnswerPresent != current.AnswerPresent,
	}
}

func ProgressActionKinds(actions []dataquery.DataAction) []string {
	var out []string
	seen := map[string]bool{}
	for _, action := range actions {
		kind := strings.TrimSpace(string(NormalizeActionKind(action.Kind)))
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
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

func fieldSetDelta(previous, current []string) ([]string, []string) {
	prev := map[string]bool{}
	cur := map[string]bool{}
	for _, field := range cleanStrings(previous) {
		prev[field] = true
	}
	for _, field := range cleanStrings(current) {
		cur[field] = true
	}
	var added []string
	var removed []string
	for field := range cur {
		if !prev[field] {
			added = append(added, field)
		}
	}
	for field := range prev {
		if !cur[field] {
			removed = append(removed, field)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
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
