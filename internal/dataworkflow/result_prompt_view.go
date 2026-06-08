package dataworkflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ResultPromptView struct {
	Answer                   string                             `json:"answer,omitempty"`
	AnswerItemCount          int                                `json:"answer_item_count,omitempty"`
	OutputContract           dataquery.OutputContract           `json:"output_contract,omitempty"`
	AuditSummary             string                             `json:"audit_summary,omitempty"`
	LedgerProjection         []LedgerProjection                 `json:"ledger_projection,omitempty"`
	DecisionRecords          int                                `json:"decision_records,omitempty"`
	DecisionSamples          []dataquery.RowDecision            `json:"decision_samples,omitempty"`
	RuleCoverageRecords      int                                `json:"rule_coverage_records,omitempty"`
	RuleCoverageSamples      []dataquery.RuleCoverageRecord     `json:"rule_coverage_samples,omitempty"`
	ContributionRecords      int                                `json:"contribution_records,omitempty"`
	ContributionSamples      []dataquery.ContributionRecord     `json:"contribution_samples,omitempty"`
	EntityResolutionRecords  int                                `json:"entity_resolution_records,omitempty"`
	EntityResolutionSamples  []dataquery.EntityResolutionRecord `json:"entity_resolution_samples,omitempty"`
	Reconcile                *dataquery.ReconcileReport         `json:"reconcile,omitempty"`
	ReconcileGroupCount      int                                `json:"reconcile_group_count,omitempty"`
	ReconcileGroupKeySample  []string                           `json:"reconcile_group_key_sample,omitempty"`
	ReconcileGroupsTruncated bool                               `json:"reconcile_groups_truncated,omitempty"`
	Metrics                  []dataquery.Metric                 `json:"metrics,omitempty"`
	Artifacts                []dataquery.DataArtifact           `json:"artifacts,omitempty"`
	ArtifactAccess           []ArtifactAccessView               `json:"artifact_access,omitempty"`
	MaterialSetHandles       []MaterialCollectionView           `json:"material_set_handles,omitempty"`
	ContractWarnings         []string                           `json:"contract_warnings,omitempty"`
}

type LedgerProjection struct {
	Kind          string         `json:"kind"`
	Count         int            `json:"count"`
	StatusCounts  map[string]int `json:"status_counts,omitempty"`
	DecisionCount map[string]int `json:"decision_counts,omitempty"`
	GroupKeys     []string       `json:"group_key_sample,omitempty"`
	Metrics       []string       `json:"metric_sample,omitempty"`
	Roles         []string       `json:"role_sample,omitempty"`
	Truncated     bool           `json:"truncated,omitempty"`
}

type ResultPromptViewBudget struct {
	AnswerLimit       int
	AuditLimit        int
	DecisionLimit     int
	RuleLimit         int
	ContributionLimit int
	ArtifactLimit     int
	ArtifactAccess    int
	MaterialSetLimit  int
}

func BuildResultPromptView(result dataquery.Result, budget ResultPromptViewBudget) *ResultPromptView {
	contract := result.OutputContract.Normalize()
	clampedReconcile := ClampPromptReconcileReport(result.Reconcile)
	groupCount, groupKeys, _ := ReconcileGroupSummary(result.Reconcile, 20)
	groupsTruncated := false
	if result.Reconcile != nil && clampedReconcile != nil {
		groupsTruncated = len(result.Reconcile.Groups) > len(clampedReconcile.Groups)
	}
	return &ResultPromptView{
		Answer:                   ClampRecordViewText(result.Answer, budget.AnswerLimit),
		AnswerItemCount:          InferAnswerItemCount(result.Answer, contract),
		OutputContract:           contract,
		AuditSummary:             ClampRecordViewText(result.AuditSummary, budget.AuditLimit),
		LedgerProjection:         BuildLedgerProjections(result, 8),
		DecisionRecords:          len(result.Rows),
		DecisionSamples:          SampleRowDecisions(result.Rows, budget.DecisionLimit),
		RuleCoverageRecords:      len(result.RuleCoverage),
		RuleCoverageSamples:      SampleRuleCoverage(result.RuleCoverage, budget.RuleLimit),
		ContributionRecords:      len(result.Contributions),
		ContributionSamples:      SampleContributions(result.Contributions, budget.ContributionLimit),
		EntityResolutionRecords:  len(result.EntityResolutions),
		EntityResolutionSamples:  SampleEntityResolutions(result.EntityResolutions, 2),
		Reconcile:                clampedReconcile,
		ReconcileGroupCount:      groupCount,
		ReconcileGroupKeySample:  groupKeys,
		ReconcileGroupsTruncated: groupsTruncated,
		Metrics:                  result.Metrics,
		Artifacts:                SampleArtifactsForPrompt(result.Artifacts, budget.ArtifactLimit),
		ArtifactAccess:           BuildArtifactAccessViews(result.Artifacts, budget.ArtifactAccess),
		MaterialSetHandles:       BuildMaterialCollectionViews(result.Artifacts, budget.MaterialSetLimit),
		ContractWarnings:         append([]string(nil), result.ContractWarnings...),
	}
}

func BuildCompactResultPromptView(result dataquery.Result, answerLimit, auditLimit, decisionLimit, ruleLimit, contributionLimit int) *ResultPromptView {
	return BuildResultPromptView(result, ResultPromptViewBudget{
		AnswerLimit:       answerLimit,
		AuditLimit:        auditLimit,
		DecisionLimit:     decisionLimit,
		RuleLimit:         ruleLimit,
		ContributionLimit: contributionLimit,
		ArtifactLimit:     6,
		ArtifactAccess:    10,
		MaterialSetLimit:  8,
	})
}

func SampleArtifactsForPrompt(artifacts []dataquery.DataArtifact, limit int) []dataquery.DataArtifact {
	if limit <= 0 || len(artifacts) == 0 {
		return nil
	}
	if len(artifacts) > limit {
		artifacts = artifacts[:limit]
	}
	out := make([]dataquery.DataArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, compactArtifactForPrompt(artifact, 0))
	}
	return out
}

func compactArtifactForPrompt(artifact dataquery.DataArtifact, depth int) dataquery.DataArtifact {
	artifact.Summary = ClampRecordViewText(artifact.Summary, 300)
	artifact.SourcePaths = ClampRecordViewStringSlice(artifact.SourcePaths, 8)
	artifact.Headers = ClampRecordViewStringSlice(artifact.Headers, 12)
	artifact.Sample = ClampRecordViewStringSlice(artifact.Sample, artifactPromptSampleLimit(depth))
	artifact.Fields = compactArtifactFieldsForPrompt(artifact.Fields)
	childLimit := artifactPromptChildLimit(depth)
	if childLimit <= 0 {
		artifact.Children = nil
		return artifact
	}
	if len(artifact.Children) > childLimit {
		artifact.Children = append([]dataquery.DataArtifact(nil), artifact.Children[:childLimit]...)
	}
	for i := range artifact.Children {
		artifact.Children[i] = compactArtifactForPrompt(artifact.Children[i], depth+1)
	}
	return artifact
}

func artifactPromptSampleLimit(depth int) int {
	if depth <= 0 {
		return 4
	}
	return 2
}

func artifactPromptChildLimit(depth int) int {
	switch {
	case depth <= 0:
		return 4
	case depth == 1:
		return 2
	default:
		return 0
	}
}

func compactArtifactFieldsForPrompt(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = ClampRecordViewText(fields[key], 240)
	}
	return out
}

func InferAnswerItemCount(answer string, contract dataquery.OutputContract) int {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return 0
	}
	var arr []any
	if err := json.Unmarshal([]byte(answer), &arr); err == nil {
		return len(arr)
	}
	if contract.Normalize().Format == dataquery.OutputCSVLine || strings.Contains(answer, ",") {
		parts := strings.Split(answer, ",")
		count := 0
		for _, part := range parts {
			if strings.TrimSpace(part) != "" {
				count++
			}
		}
		if count > 1 {
			return count
		}
	}
	if contract.Normalize().Format == dataquery.OutputMarkdownTable {
		lines := strings.Split(answer, "\n")
		count := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "|") && strings.Contains(line, "|") && !strings.Contains(line, "---") {
				count++
			}
		}
		if count > 1 {
			return count - 1
		}
	}
	return 0
}

func ReconcileGroupSummary(report *dataquery.ReconcileReport, limit int) (int, []string, bool) {
	if report == nil || len(report.Groups) == 0 || limit <= 0 {
		if report == nil {
			return 0, nil, false
		}
		return len(report.Groups), nil, len(report.Groups) > 0
	}
	total := len(report.Groups)
	if total < limit {
		limit = total
	}
	keys := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		keys = append(keys, reconcileGroupKey(report.Groups[i]))
	}
	return total, keys, total > limit
}

func BuildLedgerProjections(result dataquery.Result, sampleLimit int) []LedgerProjection {
	if sampleLimit <= 0 {
		sampleLimit = 4
	}
	var out []LedgerProjection
	if len(result.Rows) > 0 {
		proj := LedgerProjection{
			Kind:          "decision_records",
			Count:         len(result.Rows),
			DecisionCount: map[string]int{},
		}
		for _, row := range result.Rows {
			key := strings.TrimSpace(row.Decision)
			if key == "" {
				key = "(blank)"
			}
			proj.DecisionCount[key]++
		}
		out = append(out, compactLedgerProjection(proj, sampleLimit))
	}
	if len(result.RuleCoverage) > 0 {
		proj := LedgerProjection{
			Kind:         "rule_coverage",
			Count:        len(result.RuleCoverage),
			StatusCounts: map[string]int{},
		}
		for _, rec := range result.RuleCoverage {
			key := strings.TrimSpace(rec.Status.String())
			if key == "" {
				key = "(blank)"
			}
			proj.StatusCounts[key]++
		}
		out = append(out, compactLedgerProjection(proj, sampleLimit))
	}
	if len(result.EntityResolutions) > 0 {
		proj := LedgerProjection{
			Kind:         "entity_resolutions",
			Count:        len(result.EntityResolutions),
			StatusCounts: map[string]int{},
		}
		for _, rec := range result.EntityResolutions {
			key := strings.TrimSpace(rec.Status.String())
			if key == "" {
				key = "(blank)"
			}
			proj.StatusCounts[key]++
		}
		out = append(out, compactLedgerProjection(proj, sampleLimit))
	}
	if len(result.Contributions) > 0 {
		proj := LedgerProjection{
			Kind:  "contributions",
			Count: len(result.Contributions),
		}
		seenGroup := map[string]bool{}
		seenMetric := map[string]bool{}
		seenRole := map[string]bool{}
		for _, rec := range result.Contributions {
			addLedgerProjectionSample(&proj.GroupKeys, seenGroup, contributionGroupKey(rec), sampleLimit)
			addLedgerProjectionSample(&proj.Metrics, seenMetric, rec.Metric.String(), sampleLimit)
			addLedgerProjectionSample(&proj.Roles, seenRole, rec.Role.String(), sampleLimit)
		}
		proj.Truncated = len(result.Contributions) > sampleLimit
		out = append(out, proj)
	}
	if result.Reconcile != nil {
		proj := LedgerProjection{
			Kind:  "reconcile",
			Count: len(result.Reconcile.Groups),
		}
		if status := strings.TrimSpace(result.Reconcile.Status.String()); status != "" {
			proj.StatusCounts = map[string]int{status: 1}
		}
		seenGroup := map[string]bool{}
		seenMetric := map[string]bool{}
		for _, group := range result.Reconcile.Groups {
			addLedgerProjectionSample(&proj.GroupKeys, seenGroup, reconcileGroupKey(group), sampleLimit)
			addLedgerProjectionSample(&proj.Metrics, seenMetric, group.Metric.String(), sampleLimit)
		}
		proj.Truncated = len(result.Reconcile.Groups) > sampleLimit
		out = append(out, proj)
	}
	return out
}

func SampleRuleCoverage(records []dataquery.RuleCoverageRecord, limit int) []dataquery.RuleCoverageRecord {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if len(records) > limit {
		records = records[:limit]
	}
	out := make([]dataquery.RuleCoverageRecord, 0, len(records))
	for _, rec := range records {
		rec.RuleID = dataquery.LooseText(ClampRecordViewText(rec.RuleID.String(), 120))
		rec.RuleText = dataquery.LooseText(ClampRecordViewText(rec.RuleText.String(), 300))
		rec.Status = dataquery.LooseText(ClampRecordViewText(rec.Status.String(), 80))
		rec.Notes = dataquery.LooseText(ClampRecordViewText(rec.Notes.String(), 300))
		if len(rec.EvidenceRefs) > 6 {
			rec.EvidenceRefs = append([]string(nil), rec.EvidenceRefs[:6]...)
		}
		out = append(out, rec)
	}
	return out
}

func SampleContributions(records []dataquery.ContributionRecord, limit int) []dataquery.ContributionRecord {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if len(records) > limit {
		records = records[:limit]
	}
	out := make([]dataquery.ContributionRecord, 0, len(records))
	for _, rec := range records {
		rec.ItemID = dataquery.LooseText(ClampRecordViewText(rec.ItemID.String(), 140))
		rec.Source = dataquery.LooseText(ClampRecordViewText(rec.Source.String(), 200))
		rec.SourceLocator = dataquery.LooseText(ClampRecordViewText(rec.SourceLocator.String(), 200))
		rec.GroupKey = dataquery.LooseText(ClampRecordViewText(rec.GroupKey.String(), 160))
		rec.Metric = dataquery.LooseText(ClampRecordViewText(rec.Metric.String(), 120))
		rec.Value = dataquery.LooseText(ClampRecordViewText(rec.Value.String(), 120))
		rec.Operation = dataquery.LooseText(ClampRecordViewText(rec.Operation.String(), 80))
		rec.Reason = dataquery.LooseText(ClampRecordViewText(rec.Reason.String(), 300))
		if len(rec.EvidenceRefs) > 6 {
			rec.EvidenceRefs = append([]string(nil), rec.EvidenceRefs[:6]...)
		}
		out = append(out, rec)
	}
	return out
}

func SampleEntityResolutions(records []dataquery.EntityResolutionRecord, limit int) []dataquery.EntityResolutionRecord {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if len(records) > limit {
		records = records[:limit]
	}
	out := make([]dataquery.EntityResolutionRecord, 0, len(records))
	for _, rec := range records {
		rec.ItemID = dataquery.LooseText(ClampRecordViewText(rec.ItemID.String(), 140))
		rec.SourceValue = dataquery.LooseText(ClampRecordViewText(rec.SourceValue.String(), 200))
		rec.CanonicalID = dataquery.LooseText(ClampRecordViewText(rec.CanonicalID.String(), 160))
		rec.CanonicalLabel = dataquery.LooseText(ClampRecordViewText(rec.CanonicalLabel.String(), 200))
		rec.Status = dataquery.LooseText(ClampRecordViewText(rec.Status.String(), 80))
		rec.Reason = dataquery.LooseText(ClampRecordViewText(rec.Reason.String(), 300))
		if len(rec.Candidates) > 4 {
			rec.Candidates = append([]dataquery.EntityCandidate(nil), rec.Candidates[:4]...)
		}
		if len(rec.EvidenceRefs) > 6 {
			rec.EvidenceRefs = append([]string(nil), rec.EvidenceRefs[:6]...)
		}
		out = append(out, rec)
	}
	return out
}

func ClampPromptReconcileReport(report *dataquery.ReconcileReport) *dataquery.ReconcileReport {
	if report == nil {
		return nil
	}
	out := *report
	out.Status = dataquery.LooseText(ClampRecordViewText(out.Status.String(), 80))
	out.ExpectedAnswer = dataquery.LooseText(ClampRecordViewText(out.ExpectedAnswer.String(), 400))
	out.ActualAnswer = dataquery.LooseText(ClampRecordViewText(out.ActualAnswer.String(), 400))
	if len(out.Differences) > 6 {
		out.Differences = append([]string(nil), out.Differences[:6]...)
	}
	if len(out.Groups) > 8 {
		out.Groups = append([]dataquery.ReconcileGroup(nil), out.Groups[:8]...)
	}
	for i := range out.Groups {
		out.Groups[i].GroupKey = dataquery.LooseText(ClampRecordViewText(out.Groups[i].GroupKey.String(), 160))
		out.Groups[i].Metric = dataquery.LooseText(ClampRecordViewText(out.Groups[i].Metric.String(), 120))
		out.Groups[i].Expected = dataquery.LooseText(ClampRecordViewText(out.Groups[i].Expected.String(), 120))
		out.Groups[i].Actual = dataquery.LooseText(ClampRecordViewText(out.Groups[i].Actual.String(), 120))
		out.Groups[i].Difference = dataquery.LooseText(ClampRecordViewText(out.Groups[i].Difference.String(), 120))
	}
	return &out
}

func SampleRowDecisions(rows []dataquery.RowDecision, limit int) []dataquery.RowDecision {
	if limit <= 0 || len(rows) == 0 {
		return nil
	}
	out := make([]dataquery.RowDecision, 0, limit)
	used := map[int]bool{}
	for i, row := range rows {
		if len(out) >= limit {
			break
		}
		if !rowDecisionHasPromptSignal(row) {
			continue
		}
		out = append(out, clampPromptRowDecision(row))
		used[i] = true
	}
	for i, row := range rows {
		if len(out) >= limit {
			break
		}
		if used[i] {
			continue
		}
		out = append(out, clampPromptRowDecision(row))
	}
	return out
}

func compactLedgerProjection(proj LedgerProjection, sampleLimit int) LedgerProjection {
	proj.StatusCounts, proj.Truncated = compactLedgerCounts(proj.StatusCounts, sampleLimit, proj.Truncated)
	proj.DecisionCount, proj.Truncated = compactLedgerCounts(proj.DecisionCount, sampleLimit, proj.Truncated)
	return proj
}

func compactLedgerCounts(values map[string]int, limit int, truncated bool) (map[string]int, bool) {
	if len(values) == 0 || limit <= 0 || len(values) <= limit {
		return values, truncated
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]int, limit)
	for _, key := range keys[:limit] {
		out[ClampRecordViewText(key, 120)] = values[key]
	}
	return out, true
}

func addLedgerProjectionSample(out *[]string, seen map[string]bool, value string, limit int) {
	value = ClampRecordViewText(value, 160)
	if strings.TrimSpace(value) == "" || seen[value] || len(*out) >= limit {
		return
	}
	seen[value] = true
	*out = append(*out, value)
}

func contributionGroupKey(rec dataquery.ContributionRecord) string {
	groupKey := strings.TrimSpace(rec.GroupKey.String())
	metric := strings.TrimSpace(rec.Metric.String())
	switch {
	case groupKey != "" && metric != "":
		return groupKey + "/" + metric
	case groupKey != "":
		return groupKey
	case metric != "":
		return metric
	default:
		return "(default)"
	}
}

func reconcileGroupKey(group dataquery.ReconcileGroup) string {
	groupKey := strings.TrimSpace(group.GroupKey.String())
	metric := strings.TrimSpace(group.Metric.String())
	switch {
	case groupKey != "" && metric != "":
		return groupKey + "/" + metric
	case groupKey != "":
		return groupKey
	case metric != "":
		return metric
	default:
		return "(default)"
	}
}

func rowDecisionHasPromptSignal(row dataquery.RowDecision) bool {
	return strings.TrimSpace(row.RowID) != "" ||
		strings.TrimSpace(row.Source) != "" ||
		strings.TrimSpace(row.SourceLocator) != "" ||
		strings.TrimSpace(row.Decision) != "" ||
		strings.TrimSpace(row.Reason) != "" ||
		strings.TrimSpace(row.Value) != "" ||
		strings.TrimSpace(row.Contribution) != "" ||
		len(row.NormalizedFields) > 0 ||
		len(row.EvidenceRef) > 0
}

func clampPromptRowDecision(row dataquery.RowDecision) dataquery.RowDecision {
	row.RowID = ClampRecordViewText(row.RowID, 160)
	row.Source = ClampRecordViewText(row.Source, 240)
	row.SourceLocator = ClampRecordViewText(row.SourceLocator, 240)
	row.Decision = ClampRecordViewText(row.Decision, 160)
	row.Reason = ClampRecordViewText(row.Reason, 400)
	row.Value = ClampRecordViewText(row.Value, 200)
	row.Contribution = ClampRecordViewText(row.Contribution, 200)
	if len(row.NormalizedFields) > 0 {
		next := make(map[string]string, len(row.NormalizedFields))
		keys := make([]string, 0, len(row.NormalizedFields))
		for key := range row.NormalizedFields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i >= 24 {
				next["..."] = fmt.Sprintf("%d more field(s)", len(keys)-i)
				break
			}
			next[key] = ClampRecordViewText(row.NormalizedFields[key], 240)
		}
		row.NormalizedFields = next
	}
	if len(row.EvidenceRef) > 8 {
		row.EvidenceRef = append([]string(nil), row.EvidenceRef[:8]...)
	}
	return row
}
