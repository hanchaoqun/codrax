package dataquery

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ActionRunner executes typed, read-only data actions. It is deliberately
// narrower than Runner: action nodes produce reusable artifacts, while
// custom_transform is the explicit fallback for bounded Python transforms.
type ActionRunner struct {
	RepoRoot      string
	TempRoot      string
	Timeout       int64
	MaxFileBytes  int64
	MaxTotalBytes int64
}

type DataActionError struct {
	ActionID   string         `json:"action_id,omitempty"`
	ActionKind DataActionKind `json:"action_kind,omitempty"`
	Err        error          `json:"-"`
}

func (e DataActionError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("data action failed action_id=%q action_kind=%q", e.ActionID, e.ActionKind)
	}
	return fmt.Sprintf("data action failed action_id=%q action_kind=%q: %v", e.ActionID, e.ActionKind, e.Err)
}

func (e DataActionError) Unwrap() error {
	return e.Err
}

func (r ActionRunner) Run(ctx context.Context, plan TaskPlan) (Result, error) {
	if len(plan.Actions) == 0 {
		return Result{}, errors.New("data action plan has no actions")
	}
	var artifacts []DataArtifact
	var consumed []string
	var summaries []string
	var ruleCoverage []RuleCoverageRecord
	var contributions []ContributionRecord
	var entityResolutions []EntityResolutionRecord
	var reconcile *ReconcileReport
	var lastResult *Result
	for i, action := range plan.Actions {
		action.Kind = normalizeDataActionKind(action.Kind)
		if strings.TrimSpace(action.ID) == "" {
			action.ID = fmt.Sprintf("action_%d", i+1)
		}
		switch action.Kind {
		case DataActionMaterialInventory:
			artifact, err := r.runMaterialInventory(action)
			if err != nil {
				return Result{}, DataActionError{ActionID: action.ID, ActionKind: action.Kind, Err: err}
			}
			artifacts = append(artifacts, artifact)
			summaries = append(summaries, artifact.Summary)
		case DataActionInspectMaterial:
			artifact, err := r.runInspectMaterial(action)
			if err != nil {
				return Result{}, DataActionError{ActionID: action.ID, ActionKind: action.Kind, Err: err}
			}
			artifacts = append(artifacts, artifact)
			consumed = append(consumed, artifact.SourcePaths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionExtractRecords:
			artifact, err := r.runExtractRecords(action)
			if err != nil {
				return Result{}, DataActionError{ActionID: action.ID, ActionKind: action.Kind, Err: err}
			}
			artifacts = append(artifacts, artifact)
			consumed = append(consumed, artifact.SourcePaths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionDeriveRules:
			artifact, records, err := r.runDeriveRules(plan, action)
			if err != nil {
				return Result{}, DataActionError{ActionID: action.ID, ActionKind: action.Kind, Err: err}
			}
			artifacts = append(artifacts, artifact)
			ruleCoverage = append(ruleCoverage, records...)
			summaries = append(summaries, artifact.Summary)
		case DataActionNormalizeEntities:
			artifact, records, err := r.runNormalizeEntities(action)
			if err != nil {
				return Result{}, DataActionError{ActionID: action.ID, ActionKind: action.Kind, Err: err}
			}
			artifacts = append(artifacts, artifact)
			entityResolutions = append(entityResolutions, records...)
			summaries = append(summaries, artifact.Summary)
		case DataActionComputeContribs:
			artifact, records, paths, err := r.runComputeContributions(action)
			if err != nil {
				return Result{}, DataActionError{ActionID: action.ID, ActionKind: action.Kind, Err: err}
			}
			artifacts = append(artifacts, artifact)
			contributions = append(contributions, records...)
			consumed = append(consumed, paths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionReconcile:
			artifact, report, err := r.runReconcileArtifacts(action, contributions)
			if err != nil {
				return Result{}, DataActionError{ActionID: action.ID, ActionKind: action.Kind, Err: err}
			}
			artifacts = append(artifacts, artifact)
			reconcile = &report
			summaries = append(summaries, artifact.Summary)
		case DataActionCustomTransform:
			result, err := r.runCustomTransform(ctx, plan, action)
			if err != nil {
				return Result{}, DataActionError{ActionID: action.ID, ActionKind: action.Kind, Err: err}
			}
			lastResult = &result
			artifacts = append(artifacts, result.Artifacts...)
			consumed = append(consumed, result.ConsumedPaths...)
			if strings.TrimSpace(result.AuditSummary) != "" {
				summaries = append(summaries, result.AuditSummary)
			}
		default:
			return Result{}, DataActionError{ActionID: action.ID, ActionKind: action.Kind, Err: fmt.Errorf("unsupported data action kind %q", action.Kind)}
		}
	}
	if lastResult != nil {
		out := *lastResult
		out.Artifacts = append(out.Artifacts, artifacts...)
		out.ConsumedPaths = normalizeMaterialPaths(append(out.ConsumedPaths, consumed...))
		out.RuleCoverage = append(out.RuleCoverage, ruleCoverage...)
		out.Contributions = append(out.Contributions, contributions...)
		out.EntityResolutions = append(out.EntityResolutions, entityResolutions...)
		if out.Reconcile == nil && reconcile != nil {
			out.Reconcile = reconcile
		}
		if strings.TrimSpace(out.AuditSummary) == "" {
			out.AuditSummary = strings.Join(cleanArtifactSummaries(summaries), "; ")
		}
		return validateRunnerResult(plan, out)
	}
	answer := renderArtifactsAnswer(artifacts, plan.OutputContract, reconcile)
	out := Result{
		Answer:            answer,
		OutputContract:    plan.OutputContract.Normalize(),
		AuditSummary:      strings.Join(cleanArtifactSummaries(summaries), "; "),
		Artifacts:         artifacts,
		RuleCoverage:      ruleCoverage,
		Contributions:     contributions,
		EntityResolutions: entityResolutions,
		Reconcile:         reconcile,
		ConsumedPaths:     normalizeMaterialPaths(consumed),
	}
	if out.OutputContract.Format == "" {
		out.OutputContract = OutputContract{Format: OutputMarkdown, ExplanationAllowed: true}.Normalize()
	}
	return validateRunnerResult(plan, out)
}

func normalizeDataActionKind(kind DataActionKind) DataActionKind {
	switch DataActionKind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case DataActionMaterialInventory:
		return DataActionMaterialInventory
	case DataActionInspectMaterial:
		return DataActionInspectMaterial
	case DataActionExtractRecords:
		return DataActionExtractRecords
	case DataActionDeriveRules:
		return DataActionDeriveRules
	case DataActionNormalizeEntities:
		return DataActionNormalizeEntities
	case DataActionComputeContribs:
		return DataActionComputeContribs
	case DataActionReconcile:
		return DataActionReconcile
	case DataActionCustomTransform, "":
		return DataActionCustomTransform
	default:
		return kind
	}
}

func (r ActionRunner) runMaterialInventory(action DataAction) (DataArtifact, error) {
	root := firstNonEmptyString(strings.TrimSpace(r.RepoRoot), ".")
	limit := 240
	if raw := strings.TrimSpace(action.Params["limit"]); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}
	files, err := DiscoverCandidateFiles(root, limit)
	if err != nil {
		return DataArtifact{}, err
	}
	children := make([]DataArtifact, 0, len(files))
	for _, f := range files {
		children = append(children, candidateArtifact(f))
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "material_inventory")
	return DataArtifact{
		ID:       id,
		Kind:     string(DataActionMaterialInventory),
		Summary:  fmt.Sprintf("discovered %d candidate material(s)", len(files)),
		Fields:   map[string]string{"count": fmt.Sprintf("%d", len(files))},
		Children: children,
	}, nil
}

func (r ActionRunner) runInspectMaterial(action DataAction) (DataArtifact, error) {
	paths := cleanStringList(action.InputPaths)
	if len(paths) == 0 {
		return DataArtifact{}, errors.New("inspect_material action has no input_paths")
	}
	root := firstNonEmptyString(strings.TrimSpace(r.RepoRoot), ".")
	all, err := DiscoverCandidateFiles(root, 1000)
	if err != nil {
		return DataArtifact{}, err
	}
	byPath := map[string]CandidateFile{}
	for _, f := range all {
		byPath[normalizeMaterialPath(f.Path)] = f
	}
	children := make([]DataArtifact, 0, len(paths))
	for _, p := range paths {
		key := normalizeMaterialPath(p)
		if f, ok := byPath[key]; ok {
			children = append(children, candidateArtifact(f))
			continue
		}
		children = append(children, DataArtifact{
			ID:          key,
			Kind:        "unknown",
			SourcePaths: []string{key},
			Summary:     "material was requested but not found in candidate inventory",
		})
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "material_inspection")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionInspectMaterial),
		SourcePaths: normalizeMaterialPaths(paths),
		Summary:     fmt.Sprintf("inspected %d material(s)", len(paths)),
		Fields:      map[string]string{"count": fmt.Sprintf("%d", len(paths))},
		Children:    children,
	}, nil
}

func (r ActionRunner) runExtractRecords(action DataAction) (DataArtifact, error) {
	paths := cleanStringList(action.InputPaths)
	if len(paths) == 0 {
		return DataArtifact{}, errors.New("extract_records action has no input_paths")
	}
	limit := actionIntParam(action, "limit", 20, 1, 200)
	children := make([]DataArtifact, 0, len(paths))
	for _, p := range paths {
		child, err := r.extractRecordsFromPath(p, limit)
		if err != nil {
			return DataArtifact{}, err
		}
		children = append(children, child)
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "record_extract")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionExtractRecords),
		SourcePaths: normalizeMaterialPaths(paths),
		Summary:     fmt.Sprintf("extracted record samples from %d material(s)", len(paths)),
		Fields: map[string]string{
			"count": fmt.Sprintf("%d", len(paths)),
			"limit": fmt.Sprintf("%d", limit),
		},
		Children: children,
	}, nil
}

func (r ActionRunner) runDeriveRules(plan TaskPlan, action DataAction) (DataArtifact, []RuleCoverageRecord, error) {
	rules := parseRuleTexts(plan, action)
	if len(rules) == 0 {
		return DataArtifact{}, nil, errors.New("derive_rules action has no rules; provide params.rules_json, params.rules, or coverage_contract.validation_rules")
	}
	records := make([]RuleCoverageRecord, 0, len(rules))
	children := make([]DataArtifact, 0, len(rules))
	for i, rule := range rules {
		id := firstNonEmptyString(rule.ID, fmt.Sprintf("rule_%d", i+1))
		text := firstNonEmptyString(rule.Text, rule.Notes)
		rec := RuleCoverageRecord{
			RuleID:       LooseText(id),
			RuleText:     LooseText(text),
			Status:       LooseText(firstNonEmptyString(rule.Status, "derived")),
			EvidenceRefs: cleanStringList(rule.EvidenceRefs),
			Notes:        LooseText(firstNonEmptyString(rule.Notes, "derived into typed rule artifact")),
		}
		records = append(records, rec)
		children = append(children, DataArtifact{
			ID:      id,
			Kind:    "rule",
			Summary: text,
			Fields: map[string]string{
				"status": rec.Status.String(),
				"notes":  rec.Notes.String(),
			},
			SourcePaths: cleanStringList(rule.EvidenceRefs),
		})
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "derived_rules")
	return DataArtifact{
		ID:       id,
		Kind:     string(DataActionDeriveRules),
		Summary:  fmt.Sprintf("derived %d generic rule(s)", len(records)),
		Fields:   map[string]string{"count": fmt.Sprintf("%d", len(records))},
		Children: children,
	}, records, nil
}

type actionRuleDraft struct {
	ID           string   `json:"id"`
	Text         string   `json:"text"`
	Status       string   `json:"status"`
	Notes        string   `json:"notes"`
	EvidenceRefs []string `json:"evidence_refs"`
}

func parseRuleTexts(plan TaskPlan, action DataAction) []actionRuleDraft {
	if raw := strings.TrimSpace(action.Params["rules_json"]); raw != "" {
		var parsed []actionRuleDraft
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			return parsed
		}
		var stringsOnly []string
		if err := json.Unmarshal([]byte(raw), &stringsOnly); err == nil {
			out := make([]actionRuleDraft, 0, len(stringsOnly))
			for i, text := range stringsOnly {
				if text = strings.TrimSpace(text); text != "" {
					out = append(out, actionRuleDraft{ID: fmt.Sprintf("rule_%d", i+1), Text: text})
				}
			}
			return out
		}
	}
	if raw := strings.TrimSpace(action.Params["rules"]); raw != "" {
		lines := strings.Split(raw, "\n")
		out := make([]actionRuleDraft, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if line != "" {
				out = append(out, actionRuleDraft{ID: fmt.Sprintf("rule_%d", len(out)+1), Text: line})
			}
		}
		return out
	}
	out := make([]actionRuleDraft, 0, len(plan.CoverageContract.ValidationRules))
	for i, text := range plan.CoverageContract.ValidationRules {
		if text = strings.TrimSpace(text); text != "" {
			out = append(out, actionRuleDraft{ID: fmt.Sprintf("rule_%d", i+1), Text: text})
		}
	}
	return out
}

func (r ActionRunner) runNormalizeEntities(action DataAction) (DataArtifact, []EntityResolutionRecord, error) {
	raw := strings.TrimSpace(action.Params["resolutions_json"])
	if raw == "" {
		raw = strings.TrimSpace(action.Params["mappings_json"])
	}
	if raw == "" {
		return DataArtifact{}, nil, errors.New("normalize_entities action requires params.resolutions_json")
	}
	var records []EntityResolutionRecord
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return DataArtifact{}, nil, fmt.Errorf("parse normalize_entities resolutions_json: %w", err)
	}
	if len(records) == 0 {
		return DataArtifact{}, nil, errors.New("normalize_entities action produced no entity resolutions")
	}
	if err := validateEntityResolutionRecords(records); err != nil {
		return DataArtifact{}, nil, err
	}
	children := make([]DataArtifact, 0, len(records))
	for i, rec := range records {
		id := firstNonEmptyString(rec.ItemID.String(), fmt.Sprintf("entity_%d", i+1))
		children = append(children, DataArtifact{
			ID:      id,
			Kind:    "entity_resolution",
			Summary: fmt.Sprintf("%s -> %s", rec.SourceValue.String(), firstNonEmptyString(rec.CanonicalLabel.String(), rec.CanonicalID.String(), rec.Status.String())),
			Fields: map[string]string{
				"source_value":    rec.SourceValue.String(),
				"canonical_id":    rec.CanonicalID.String(),
				"canonical_label": rec.CanonicalLabel.String(),
				"status":          rec.Status.String(),
			},
			SourcePaths: cleanStringList(rec.EvidenceRefs),
		})
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "entity_resolutions")
	return DataArtifact{
		ID:       id,
		Kind:     string(DataActionNormalizeEntities),
		Summary:  fmt.Sprintf("normalized %d entity value(s)", len(records)),
		Fields:   map[string]string{"count": fmt.Sprintf("%d", len(records))},
		Children: children,
	}, records, nil
}

type actionRecord struct {
	Fields map[string]string
	Path   string
	Line   int
	Index  int
}

type contributionFilter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

func (r ActionRunner) runComputeContributions(action DataAction) (DataArtifact, []ContributionRecord, []string, error) {
	paths := cleanStringList(action.InputPaths)
	if len(paths) == 0 {
		return DataArtifact{}, nil, nil, errors.New("compute_contributions action has no input_paths")
	}
	groupKeyField := strings.TrimSpace(action.Params["group_key_field"])
	groupKeyConst := strings.TrimSpace(action.Params["group_key"])
	valueField := strings.TrimSpace(action.Params["value_field"])
	metric := firstNonEmptyString(strings.TrimSpace(action.Params["metric"]), valueField, "count")
	operation := firstNonEmptyString(strings.TrimSpace(action.Params["operation"]), "add")
	if valueField == "" && strings.TrimSpace(action.Params["operation"]) == "" {
		operation = "count"
	}
	operation, ok := normalizeContributionOperation(operation)
	if !ok || operation == "" {
		return DataArtifact{}, nil, nil, fmt.Errorf("compute_contributions unsupported operation %q", action.Params["operation"])
	}
	itemIDField := strings.TrimSpace(action.Params["item_id_field"])
	reason := firstNonEmptyString(strings.TrimSpace(action.Params["reason"]), strings.TrimSpace(action.Purpose), "computed by typed data action")
	ruleRefs := parseActionStringListParam(action.Params["rule_refs"])
	filters, err := parseContributionFilters(action)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxContribs := actionIntParam(action, "max_contributions", 50000, 1, 200000)

	var contributions []ContributionRecord
	var consumed []string
	children := make([]DataArtifact, 0, len(paths))
	for _, path := range paths {
		records, headers, total, rel, err := r.readActionRecords(path, maxRecords)
		if err != nil {
			return DataArtifact{}, nil, nil, err
		}
		consumed = append(consumed, rel)
		matched := 0
		for _, record := range records {
			if !recordPassesFilters(record.Fields, filters) {
				continue
			}
			if len(contributions) >= maxContribs {
				return DataArtifact{}, nil, nil, fmt.Errorf("compute_contributions exceeded max_contributions=%d; split the action into smaller groups or filters", maxContribs)
			}
			matched++
			value := ""
			if valueField != "" {
				value = recordField(record.Fields, valueField)
				if value == "" && operation != "count" {
					return DataArtifact{}, nil, nil, fmt.Errorf("compute_contributions missing value_field %q at %s:%d", valueField, rel, record.Line)
				}
			}
			if value == "" && operation == "count" {
				value = "1"
			}
			itemID := recordField(record.Fields, itemIDField)
			if itemID == "" {
				itemID = fmt.Sprintf("%s#%d", rel, record.Index)
			}
			groupKey := firstNonEmptyString(recordField(record.Fields, groupKeyField), groupKeyConst, "all")
			sourceLocator := fmt.Sprintf("line:%d", record.Line)
			contributions = append(contributions, ContributionRecord{
				ItemID:        LooseText(itemID),
				Source:        LooseText(rel),
				SourceLocator: LooseText(sourceLocator),
				GroupKey:      LooseText(groupKey),
				Metric:        LooseText(metric),
				Value:         LooseText(value),
				Operation:     LooseText(operation),
				Reason:        LooseText(reason),
				EvidenceRefs:  []string{fmt.Sprintf("%s:%d", rel, record.Line)},
				RuleRefs:      append([]string(nil), ruleRefs...),
			})
		}
		children = append(children, DataArtifact{
			ID:          rel + "#contributions",
			Kind:        "contribution_source",
			SourcePaths: []string{rel},
			Headers:     headers,
			RowCount:    total,
			Summary:     fmt.Sprintf("%s produced %d contribution(s) from %d record(s)", rel, matched, total),
			Fields: map[string]string{
				"matched": fmt.Sprintf("%d", matched),
				"total":   fmt.Sprintf("%d", total),
			},
		})
	}
	if len(contributions) == 0 {
		return DataArtifact{}, nil, nil, errors.New("compute_contributions produced no contribution records")
	}
	if err := validateContributionRecords(contributions); err != nil {
		return DataArtifact{}, nil, nil, err
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "contributions")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionComputeContribs),
		SourcePaths: normalizeMaterialPaths(consumed),
		Summary:     fmt.Sprintf("computed %d generic contribution(s)", len(contributions)),
		Fields: map[string]string{
			"count":     fmt.Sprintf("%d", len(contributions)),
			"metric":    metric,
			"operation": operation,
		},
		Children: children,
	}, contributions, normalizeMaterialPaths(consumed), nil
}

func (r ActionRunner) runReconcileArtifacts(action DataAction, contributions []ContributionRecord) (DataArtifact, ReconcileReport, error) {
	if len(contributions) == 0 {
		return DataArtifact{}, ReconcileReport{}, errors.New("reconcile_artifacts requires prior contribution records")
	}
	sums := sumContributionGroups(contributions)
	if len(sums) == 0 {
		return DataArtifact{}, ReconcileReport{}, errors.New("reconcile_artifacts could not compute numeric groups from contribution records")
	}
	keys := make([]string, 0, len(sums))
	for key := range sums {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := make([]ReconcileGroup, 0, len(keys))
	children := make([]DataArtifact, 0, len(keys))
	answerParts := make([]string, 0, len(keys))
	for _, key := range keys {
		groupKey, metric := splitReconcileGroupKey(key)
		value := formatRat(sums[key])
		groups = append(groups, ReconcileGroup{
			GroupKey:   LooseText(groupKey),
			Metric:     LooseText(metric),
			Expected:   LooseText(value),
			Actual:     LooseText(value),
			Difference: LooseText("0"),
		})
		label := displayReconcileGroupKey(groupKey, metric)
		answerParts = append(answerParts, fmt.Sprintf("%s=%s", label, value))
		children = append(children, DataArtifact{
			ID:      "reconcile:" + label,
			Kind:    "reconcile_group",
			Summary: fmt.Sprintf("%s reconciled to %s", label, value),
			Fields: map[string]string{
				"group_key": groupKey,
				"metric":    metric,
				"value":     value,
			},
		})
	}
	answer := ""
	if len(groups) == 1 {
		answer = groups[0].Actual.String()
	} else {
		answer = strings.Join(answerParts, "; ")
	}
	report := ReconcileReport{
		Status:         LooseText("pass"),
		ExpectedAnswer: LooseText(answer),
		ActualAnswer:   LooseText(answer),
		Groups:         groups,
	}
	if err := validateReconcileReport(report, contributions, ""); err != nil {
		return DataArtifact{}, ReconcileReport{}, err
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "reconcile")
	return DataArtifact{
		ID:       id,
		Kind:     string(DataActionReconcile),
		Summary:  fmt.Sprintf("reconciled %d contribution group(s)", len(groups)),
		Fields:   map[string]string{"groups": fmt.Sprintf("%d", len(groups))},
		Children: children,
	}, report, nil
}

func (r ActionRunner) readActionRecords(path string, maxRecords int) ([]actionRecord, []string, int, string, error) {
	abs, rel, err := r.resolveActionInputPath(path)
	if err != nil {
		return nil, nil, 0, "", err
	}
	kind := dataKindForPath(abs)
	switch kind {
	case "csv", "tsv":
		records, headers, total, err := readDelimitedActionRecords(abs, rel, kind, maxRecords)
		return records, headers, total, rel, err
	case "json":
		records, headers, total, err := readJSONActionRecords(abs, rel, maxRecords)
		return records, headers, total, rel, err
	case "jsonl":
		records, headers, total, err := readJSONLActionRecords(abs, rel, maxRecords)
		return records, headers, total, rel, err
	default:
		records, total, err := readTextActionRecords(abs, rel, maxRecords)
		return records, []string{"text"}, total, rel, err
	}
}

func readDelimitedActionRecords(abs, rel, kind string, maxRecords int) ([]actionRecord, []string, int, error) {
	file, err := os.Open(abs)
	if err != nil {
		return nil, nil, 0, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	if kind == "tsv" {
		reader.Comma = '\t'
	}
	headers, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil, 0, nil
		}
		return nil, nil, 0, err
	}
	headers = cleanStringSlice(headers)
	var records []actionRecord
	total := 0
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return records, headers, total, err
		}
		total++
		if len(records) >= maxRecords {
			continue
		}
		fields := map[string]string{}
		for i, value := range row {
			key := fmt.Sprintf("col_%d", i+1)
			if i < len(headers) && headers[i] != "" {
				key = headers[i]
			}
			fields[key] = strings.TrimSpace(value)
		}
		records = append(records, actionRecord{Fields: fields, Path: rel, Line: total + 1, Index: total})
	}
	return records, headers, total, nil
}

func readJSONActionRecords(abs, rel string, maxRecords int) ([]actionRecord, []string, int, error) {
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, nil, 0, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, nil, 0, err
	}
	items := flattenJSONRecords(value)
	headers := collectRecordHeaders(items)
	records := make([]actionRecord, 0, minInt(maxRecords, len(items)))
	for i, item := range items {
		if i >= maxRecords {
			break
		}
		records = append(records, actionRecord{Fields: stringifyRecordMap(item), Path: rel, Line: i + 1, Index: i + 1})
	}
	return records, headers, len(items), nil
}

func readJSONLActionRecords(abs, rel string, maxRecords int) ([]actionRecord, []string, int, error) {
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, nil, 0, err
	}
	lines := strings.Split(string(raw), "\n")
	var records []actionRecord
	headersSeen := map[string]bool{}
	var headers []string
	total := 0
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		total++
		if len(records) >= maxRecords {
			continue
		}
		var obj map[string]any
		fields := map[string]string{}
		if err := json.Unmarshal([]byte(line), &obj); err == nil {
			fields = stringifyRecordMap(obj)
		} else {
			fields["text"] = line
		}
		for key := range fields {
			if !headersSeen[key] {
				headersSeen[key] = true
				headers = append(headers, key)
			}
		}
		records = append(records, actionRecord{Fields: fields, Path: rel, Line: i + 1, Index: total})
	}
	sort.Strings(headers)
	return records, headers, total, nil
}

func readTextActionRecords(abs, rel string, maxRecords int) ([]actionRecord, int, error) {
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, 0, err
	}
	lines := strings.Split(string(raw), "\n")
	var records []actionRecord
	total := 0
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		total++
		if len(records) >= maxRecords {
			continue
		}
		records = append(records, actionRecord{Fields: map[string]string{"text": line}, Path: rel, Line: i + 1, Index: total})
	}
	return records, total, nil
}

func flattenJSONRecords(value any) []map[string]any {
	switch v := value.(type) {
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			} else {
				out = append(out, map[string]any{"value": item})
			}
		}
		return out
	case map[string]any:
		for _, item := range v {
			if arr, ok := item.([]any); ok {
				return flattenJSONRecords(arr)
			}
		}
		return []map[string]any{v}
	default:
		return []map[string]any{{"value": v}}
	}
}

func stringifyRecordMap(in map[string]any) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		out[strings.TrimSpace(key)] = stringifyActionValue(value)
	}
	return out
}

func stringifyActionValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return strings.TrimSpace(rawJSONValueString(raw))
	}
}

func collectRecordHeaders(records []map[string]any) []string {
	seen := map[string]bool{}
	var headers []string
	for _, record := range records {
		for key := range record {
			key = strings.TrimSpace(key)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			headers = append(headers, key)
		}
	}
	sort.Strings(headers)
	return headers
}

func parseContributionFilters(action DataAction) ([]contributionFilter, error) {
	var filters []contributionFilter
	if raw := strings.TrimSpace(action.Params["filters_json"]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &filters); err != nil {
			return nil, fmt.Errorf("parse filters_json: %w", err)
		}
	}
	if field := strings.TrimSpace(action.Params["filter_field"]); field != "" {
		op := firstNonEmptyString(strings.TrimSpace(action.Params["filter_op"]), "eq")
		value := firstNonEmptyString(strings.TrimSpace(action.Params["filter_value"]), strings.TrimSpace(action.Params["filter_equals"]))
		if value != "" {
			filters = append(filters, contributionFilter{Field: field, Op: op, Value: value})
		}
		if ne := strings.TrimSpace(action.Params["filter_not_equals"]); ne != "" {
			filters = append(filters, contributionFilter{Field: field, Op: "ne", Value: ne})
		}
	}
	for i := range filters {
		filters[i].Field = strings.TrimSpace(filters[i].Field)
		filters[i].Op = strings.ToLower(strings.TrimSpace(filters[i].Op))
		filters[i].Value = strings.TrimSpace(filters[i].Value)
		if filters[i].Field == "" {
			return nil, fmt.Errorf("filters_json[%d] has empty field", i)
		}
		if filters[i].Op == "" {
			filters[i].Op = "eq"
		}
	}
	return filters, nil
}

func recordPassesFilters(fields map[string]string, filters []contributionFilter) bool {
	for _, filter := range filters {
		if !recordPassesFilter(fields, filter) {
			return false
		}
	}
	return true
}

func recordPassesFilter(fields map[string]string, filter contributionFilter) bool {
	got := recordField(fields, filter.Field)
	want := strings.TrimSpace(filter.Value)
	switch filter.Op {
	case "eq", "equals", "==":
		return got == want
	case "ne", "not_equals", "!=":
		return got != want
	case "contains":
		return strings.Contains(got, want)
	case "not_contains":
		return !strings.Contains(got, want)
	case "nonempty", "present":
		return got != ""
	case "empty", "missing":
		return got == ""
	case "gt", "gte", "lt", "lte":
		return compareRecordDecimal(got, want, filter.Op)
	case "in":
		for _, item := range strings.Split(want, ",") {
			if got == strings.TrimSpace(item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func compareRecordDecimal(got, want, op string) bool {
	left, err := parseDecimalRat(got)
	if err != nil {
		return false
	}
	right, err := parseDecimalRat(want)
	if err != nil {
		return false
	}
	cmp := left.Cmp(right)
	switch op {
	case "gt":
		return cmp > 0
	case "gte":
		return cmp >= 0
	case "lt":
		return cmp < 0
	case "lte":
		return cmp <= 0
	default:
		return false
	}
}

func recordField(fields map[string]string, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if value, ok := fields[name]; ok {
		return strings.TrimSpace(value)
	}
	lower := strings.ToLower(name)
	for key, value := range fields {
		if strings.ToLower(strings.TrimSpace(key)) == lower {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseActionStringListParam(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var parsed []string
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return cleanStringSlice(parsed)
	}
	return cleanStringSlice(strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	}))
}

func (r ActionRunner) extractRecordsFromPath(path string, limit int) (DataArtifact, error) {
	abs, rel, err := r.resolveActionInputPath(path)
	if err != nil {
		return DataArtifact{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return DataArtifact{}, err
	}
	kind := dataKindForPath(abs)
	if kind == "" {
		kind = "text"
	}
	f := inspectCandidateFile(abs, CandidateFile{
		Path: rel,
		Size: info.Size(),
		Kind: kind,
	})
	artifact := candidateArtifact(f)
	artifact.ID = rel + "#records"
	artifact.Kind = string(DataActionExtractRecords) + "/" + kind
	artifact.Sample = nil
	switch kind {
	case "csv", "tsv":
		headers, samples, rowCount, err := extractDelimitedRecords(abs, f.Delimiter, limit)
		if err != nil {
			return DataArtifact{}, err
		}
		artifact.Headers = headers
		artifact.Sample = samples
		artifact.RowCount = rowCount
	case "json":
		samples, rowCount, err := extractJSONRecords(abs, limit)
		if err != nil {
			return DataArtifact{}, err
		}
		artifact.Sample = samples
		artifact.RowCount = rowCount
	case "jsonl":
		samples, rowCount, err := extractJSONLRecords(abs, limit)
		if err != nil {
			return DataArtifact{}, err
		}
		artifact.Sample = samples
		artifact.RowCount = rowCount
	default:
		samples, rowCount, err := extractTextRecords(abs, limit)
		if err != nil {
			return DataArtifact{}, err
		}
		artifact.Sample = samples
		artifact.RowCount = rowCount
	}
	if artifact.Fields == nil {
		artifact.Fields = map[string]string{}
	}
	artifact.Fields["sample_count"] = fmt.Sprintf("%d", len(artifact.Sample))
	artifact.Fields["limit"] = fmt.Sprintf("%d", limit)
	artifact.Summary = fmt.Sprintf("%s | extracted %d sample record(s) from %d total record(s)", rel, len(artifact.Sample), artifact.RowCount)
	return artifact, nil
}

func (r ActionRunner) resolveActionInputPath(path string) (abs string, rel string, err error) {
	path = normalizeMaterialPath(path)
	if path == "" {
		return "", "", errors.New("empty action input path")
	}
	root := firstNonEmptyString(strings.TrimSpace(r.RepoRoot), ".")
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	joined := path
	if !filepath.IsAbs(joined) {
		joined = filepath.Join(absRoot, filepath.FromSlash(path))
	}
	abs, err = filepath.Abs(joined)
	if err != nil {
		return "", "", err
	}
	relPath, err := filepath.Rel(absRoot, abs)
	if err != nil {
		return "", "", err
	}
	if relPath == "." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." || filepath.IsAbs(relPath) {
		return "", "", fmt.Errorf("action input path escapes data workspace: %s", path)
	}
	return abs, filepath.ToSlash(relPath), nil
}

func (r ActionRunner) runCustomTransform(ctx context.Context, plan TaskPlan, action DataAction) (Result, error) {
	script := strings.TrimSpace(action.Script)
	if script == "" {
		script = strings.TrimSpace(plan.Script)
	}
	if script == "" {
		return Result{}, errors.New("custom_transform action has empty script")
	}
	inputs := cleanStringList(action.InputPaths)
	if len(inputs) == 0 {
		inputs = cleanStringList(plan.InputPaths)
	}
	subPlan := plan
	subPlan.Actions = nil
	subPlan.Script = script
	subPlan.InputPaths = inputs
	runner := Runner{
		RepoRoot:      r.RepoRoot,
		TempRoot:      r.TempRoot,
		MaxFileBytes:  r.MaxFileBytes,
		MaxTotalBytes: r.MaxTotalBytes,
	}
	return runner.Run(ctx, subPlan)
}

func extractDelimitedRecords(path, delimiter string, limit int) ([]string, []string, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	if delimiter == "\t" {
		reader.Comma = '\t'
	}
	headers, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil, 0, nil
		}
		return nil, nil, 0, err
	}
	headers = cleanStringSlice(headers)
	var samples []string
	rowCount := 0
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return headers, samples, rowCount, err
		}
		rowCount++
		if len(samples) >= limit {
			continue
		}
		obj := map[string]string{}
		for i, value := range row {
			key := fmt.Sprintf("col_%d", i+1)
			if i < len(headers) && headers[i] != "" {
				key = headers[i]
			}
			obj[key] = strings.TrimSpace(value)
		}
		samples = append(samples, compactJSONLine(obj))
	}
	return headers, samples, rowCount, nil
}

func extractJSONRecords(path string, limit int) ([]string, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, 0, err
	}
	var records []any
	switch v := value.(type) {
	case []any:
		records = v
	case map[string]any:
		for _, item := range v {
			if arr, ok := item.([]any); ok {
				records = arr
				break
			}
		}
		if records == nil {
			records = []any{v}
		}
	default:
		records = []any{v}
	}
	samples := make([]string, 0, minInt(limit, len(records)))
	for i, record := range records {
		if i >= limit {
			break
		}
		samples = append(samples, compactJSONLine(record))
	}
	return samples, len(records), nil
}

func extractJSONLRecords(path string, limit int) ([]string, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	lines := strings.Split(string(raw), "\n")
	var samples []string
	rowCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rowCount++
		if len(samples) >= limit {
			continue
		}
		var obj any
		if err := json.Unmarshal([]byte(line), &obj); err == nil {
			samples = append(samples, compactJSONLine(obj))
		} else {
			samples = append(samples, line)
		}
	}
	return samples, rowCount, nil
}

func extractTextRecords(path string, limit int) ([]string, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	lines := strings.Split(string(raw), "\n")
	var samples []string
	rowCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rowCount++
		if len(samples) < limit {
			samples = append(samples, line)
		}
	}
	return samples, rowCount, nil
}

func compactJSONLine(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(raw)
}

func actionIntParam(action DataAction, key string, fallback, minValue, maxValue int) int {
	value := fallback
	if raw := strings.TrimSpace(action.Params[key]); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil {
			value = parsed
		}
	}
	if value < minValue {
		value = minValue
	}
	if value > maxValue {
		value = maxValue
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func candidateArtifact(f CandidateFile) DataArtifact {
	fields := map[string]string{
		"kind":              f.Kind,
		"size":              fmt.Sprintf("%d", f.Size),
		"extraction_status": f.ExtractionStatus,
	}
	if f.Delimiter != "" {
		fields["delimiter"] = f.Delimiter
	}
	if f.InspectError != "" {
		fields["inspect_error"] = f.InspectError
	}
	if len(f.TextEvidencePaths) > 0 {
		fields["text_evidence_paths"] = strings.Join(f.TextEvidencePaths, ", ")
	}
	return DataArtifact{
		ID:          normalizeMaterialPath(f.Path),
		Kind:        f.Kind,
		SourcePaths: []string{normalizeMaterialPath(f.Path)},
		Summary:     candidateSummary(f),
		Fields:      fields,
		Headers:     append([]string(nil), f.Headers...),
		Sample:      candidateSample(f),
		RowCount:    f.Lines,
	}
}

func candidateSummary(f CandidateFile) string {
	parts := []string{f.Path, f.Kind}
	if f.Lines > 0 {
		parts = append(parts, fmt.Sprintf("%d line(s)", f.Lines))
	}
	if len(f.Headers) > 0 {
		parts = append(parts, "headers="+strings.Join(f.Headers, ","))
	}
	if f.ExtractionStatus != "" {
		parts = append(parts, "status="+f.ExtractionStatus)
	}
	return strings.Join(parts, " | ")
}

func candidateSample(f CandidateFile) []string {
	if len(f.Sample) > 0 {
		return append([]string(nil), f.Sample...)
	}
	if len(f.SampleRows) == 0 {
		return nil
	}
	out := make([]string, 0, len(f.SampleRows))
	for _, row := range f.SampleRows {
		out = append(out, strings.Join(row, ","))
	}
	return out
}

func renderArtifactsAnswer(artifacts []DataArtifact, contract OutputContract, reconcile *ReconcileReport) string {
	if len(artifacts) == 0 {
		return ""
	}
	contract = contract.Normalize()
	reconcileAnswer := renderReconcileAnswer(reconcile)
	switch contract.Format {
	case OutputJSONOnly:
		payload := map[string]any{"artifacts": artifacts}
		if reconcile != nil {
			payload["reconcile"] = reconcile
		}
		raw, _ := json.Marshal(payload)
		return string(raw)
	case OutputCSVLine:
		if reconcileAnswer != "" {
			return reconcileAnswer
		}
		return fmt.Sprintf("artifacts,%d", len(artifacts))
	case OutputPlainSingleLine:
		if reconcileAnswer != "" {
			return reconcileAnswer
		}
		return fmt.Sprintf("%d artifact(s)", len(artifacts))
	default:
		var b strings.Builder
		for _, artifact := range artifacts {
			fmt.Fprintf(&b, "- %s: %s\n", firstNonEmptyString(artifact.ID, artifact.Kind), artifact.Summary)
		}
		return strings.TrimSpace(b.String())
	}
}

func renderReconcileAnswer(reconcile *ReconcileReport) string {
	if reconcile == nil {
		return ""
	}
	if strings.TrimSpace(reconcile.ActualAnswer.String()) != "" {
		return strings.TrimSpace(reconcile.ActualAnswer.String())
	}
	if strings.TrimSpace(reconcile.ExpectedAnswer.String()) != "" {
		return strings.TrimSpace(reconcile.ExpectedAnswer.String())
	}
	if len(reconcile.Groups) == 1 {
		return firstNonEmptyString(reconcile.Groups[0].Actual.String(), reconcile.Groups[0].Expected.String())
	}
	if len(reconcile.Groups) == 0 {
		return ""
	}
	parts := make([]string, 0, len(reconcile.Groups))
	for _, group := range reconcile.Groups {
		value := firstNonEmptyString(group.Actual.String(), group.Expected.String())
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", displayReconcileGroupKey(group.GroupKey.String(), group.Metric.String()), value))
	}
	return strings.Join(parts, "; ")
}

func cleanArtifactSummaries(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func cleanStringList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(filepath.ToSlash(s))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
