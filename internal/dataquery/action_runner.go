package dataquery

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	Seed          Result

	artifactFiles map[string]string
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
	artifactDir, cleanup, err := r.prepareArtifactWorkspace()
	if err != nil {
		return Result{}, err
	}
	defer cleanup()
	r.artifactFiles = dataActionArtifactFilesFromSeed(r.Seed.Artifacts)
	artifacts := append([]DataArtifact(nil), r.Seed.Artifacts...)
	consumed := append([]string(nil), r.Seed.ConsumedPaths...)
	var summaries []string
	ruleCoverage := append([]RuleCoverageRecord(nil), r.Seed.RuleCoverage...)
	contributions := append([]ContributionRecord(nil), r.Seed.Contributions...)
	entityResolutions := append([]EntityResolutionRecord(nil), r.Seed.EntityResolutions...)
	var reconcile *ReconcileReport
	if r.Seed.Reconcile != nil {
		seedReconcile := *r.Seed.Reconcile
		reconcile = &seedReconcile
	}
	var lastResult *Result
	partialResult := func() Result {
		outputContract := plan.OutputContract
		if outputContract.Format == "" {
			outputContract = OutputContract{Format: OutputFreeform, ExplanationAllowed: true}
		}
		return Result{
			OutputContract:    outputContract.Normalize(),
			AuditSummary:      strings.Join(cleanArtifactSummaries(summaries), "; "),
			Artifacts:         artifacts,
			RuleCoverage:      ruleCoverage,
			Contributions:     contributions,
			EntityResolutions: entityResolutions,
			Reconcile:         reconcile,
			ConsumedPaths:     normalizeMaterialPaths(consumed),
		}
	}
	failAction := func(action DataAction, err error) (Result, error) {
		return partialResult(), DataActionError{ActionID: action.ID, ActionKind: action.Kind, Err: err}
	}
	for i, action := range plan.Actions {
		action.Kind = normalizeDataActionKind(action.Kind)
		if strings.TrimSpace(action.ID) == "" {
			action.ID = fmt.Sprintf("action_%d", i+1)
		}
		switch action.Kind {
		case DataActionMaterialInventory:
			artifact, err := r.runMaterialInventory(action)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, artifact)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			summaries = append(summaries, artifact.Summary)
		case DataActionInspectMaterial:
			artifact, err := r.runInspectMaterial(action)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, artifact)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			consumed = append(consumed, artifact.SourcePaths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionExtractRecords:
			artifact, err := r.runExtractRecords(action)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, materializedRecordPayloadFromArtifact(artifact))
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			consumed = append(consumed, artifact.SourcePaths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionDeriveRules:
			artifact, records, err := r.runDeriveRules(plan, action)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, records)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			ruleCoverage = append(ruleCoverage, records...)
			consumed = append(consumed, artifact.SourcePaths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionDeriveFields:
			artifact, records, paths, err := r.runDeriveFields(action)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, records)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			consumed = append(consumed, paths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionNormalizeEntities:
			artifact, records, err := r.runNormalizeEntities(action)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, records)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			entityResolutions = append(entityResolutions, records...)
			consumed = append(consumed, artifact.SourcePaths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionEnrichRecords:
			artifact, records, paths, err := r.runEnrichRecords(action)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, records)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			consumed = append(consumed, paths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionJoinRecords:
			artifact, records, paths, err := r.runJoinRecords(action)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, records)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			consumed = append(consumed, paths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionComputeContribs:
			defaultRuleRefs := []string(nil)
			if plan.CoverageContract.RuleCoverageRequired {
				defaultRuleRefs = ruleCoverageIDs(ruleCoverage)
			}
			artifact, records, paths, err := r.runComputeContributions(action, defaultRuleRefs)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, contributionActionArtifactPayload(artifact, records))
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			contributions = append(contributions, records...)
			consumed = append(consumed, paths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionReconcile:
			artifact, report, err := r.runReconcileArtifacts(action, contributions)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, report)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			reconcile = &report
			summaries = append(summaries, artifact.Summary)
		case DataActionCustomTransform:
			result, err := r.runCustomTransform(ctx, plan, action)
			if err != nil {
				return failAction(action, err)
			}
			lastResult = &result
			customArtifact, err := r.materializeActionArtifact(artifactDir, action, DataArtifact{
				ID:          firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "custom_transform_result"),
				Kind:        string(DataActionCustomTransform),
				SourcePaths: normalizeMaterialPaths(result.ConsumedPaths),
				Summary:     firstNonEmptyString(result.AuditSummary, fmt.Sprintf("custom transform produced %d artifact(s)", len(result.Artifacts))),
			}, result)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, customArtifact)
			artifacts = append(artifacts, result.Artifacts...)
			consumed = append(consumed, result.ConsumedPaths...)
			if strings.TrimSpace(result.AuditSummary) != "" {
				summaries = append(summaries, result.AuditSummary)
			}
		default:
			return failAction(action, fmt.Errorf("unsupported data action kind %q", action.Kind))
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
		return validateRunnerResult(actionRunnerValidationPlan(plan), out)
	}
	answer := renderArtifactsAnswer(artifacts, plan.OutputContract, reconcile)
	outputContract := plan.OutputContract
	if strings.TrimSpace(r.Seed.Answer) != "" && lastResult == nil {
		answer = r.Seed.Answer
		if r.Seed.OutputContract.Format != "" {
			outputContract = r.Seed.OutputContract
		}
	}
	out := Result{
		Answer:            answer,
		OutputContract:    outputContract.Normalize(),
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
	return validateRunnerResult(actionRunnerValidationPlan(plan), out)
}

func actionRunnerValidationPlan(plan TaskPlan) TaskPlan {
	if !plan.ContinueAfter {
		return plan
	}
	out := plan
	out.OutputContract = OutputContract{Format: OutputFreeform, ExplanationAllowed: true}
	out.CoverageContract = actionRunnerIntermediateCoverageContract(plan)
	return out
}

func actionRunnerIntermediateCoverageContract(plan TaskPlan) CoverageContract {
	var inputs []string
	for _, action := range plan.Actions {
		inputs = append(inputs, action.InputPaths...)
	}
	out := coverageContractForActionInputs(plan.CoverageContract, inputs)
	for _, action := range plan.Actions {
		switch normalizeDataActionKind(action.Kind) {
		case DataActionDeriveRules:
			out.RuleCoverageRequired = out.RuleCoverageRequired || plan.CoverageContract.RuleCoverageRequired
		case DataActionNormalizeEntities:
			out.EntityResolutionRequired = out.EntityResolutionRequired || plan.CoverageContract.EntityResolutionRequired
		case DataActionComputeContribs:
			out.ContributionLedgerRequired = out.ContributionLedgerRequired || plan.CoverageContract.ContributionLedgerRequired
		case DataActionReconcile:
			out.ReconcileRequired = out.ReconcileRequired || plan.CoverageContract.ReconcileRequired
		}
	}
	return out
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
	case DataActionDeriveFields:
		return DataActionDeriveFields
	case DataActionNormalizeEntities:
		return DataActionNormalizeEntities
	case DataActionEnrichRecords:
		return DataActionEnrichRecords
	case DataActionJoinRecords:
		return DataActionJoinRecords
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
	sourcePaths := normalizeMaterialPaths(action.InputPaths)
	rules := parseActionRuleParamTexts(action)
	if len(action.InputPaths) > 0 {
		inputRules, inputSourcePaths, err := r.deriveRuleDraftsFromInputs(action)
		if err != nil {
			return DataArtifact{}, nil, err
		}
		if len(inputSourcePaths) > 0 {
			sourcePaths = inputSourcePaths
		}
		rules = append(rules, inputRules...)
	}
	if len(rules) == 0 {
		rules = parseValidationRuleTexts(plan)
	}
	if len(rules) == 0 {
		return DataArtifact{}, nil, errors.New("derive_rules action has no rules; provide params.rules_json, params.rules, or coverage_contract.validation_rules")
	}
	rules = normalizeActionRuleDraftIDs(rules)
	records := make([]RuleCoverageRecord, 0, len(rules))
	children := make([]DataArtifact, 0, len(rules))
	for i, rule := range rules {
		id := firstNonEmptyString(rule.ID, fmt.Sprintf("rule_%d", i+1))
		text := firstNonEmptyString(rule.Text, rule.Notes)
		evidenceRefs := cleanStringList(rule.EvidenceRefs)
		if len(evidenceRefs) == 0 && len(sourcePaths) > 0 {
			evidenceRefs = append([]string(nil), sourcePaths...)
		}
		rec := RuleCoverageRecord{
			RuleID:       LooseText(id),
			RuleText:     LooseText(text),
			Status:       LooseText(firstNonEmptyString(rule.Status, "derived")),
			EvidenceRefs: evidenceRefs,
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
		ID:          id,
		Kind:        string(DataActionDeriveRules),
		SourcePaths: sourcePaths,
		Summary:     fmt.Sprintf("derived %d generic rule(s)", len(records)),
		Fields:      map[string]string{"count": fmt.Sprintf("%d", len(records))},
		Children:    children,
	}, records, nil
}

func (r ActionRunner) deriveRuleDraftsFromInputs(action DataAction) ([]actionRuleDraft, []string, error) {
	paths := cleanStringList(action.InputPaths)
	if len(paths) == 0 {
		return nil, nil, nil
	}
	limit := actionIntParam(action, "limit", 80, 1, 500)
	var out []actionRuleDraft
	var consumed []string
	for _, path := range paths {
		records, _, _, rel, err := r.readActionRecords(path, limit)
		if err != nil {
			return nil, nil, err
		}
		consumed = append(consumed, rel)
		for _, record := range records {
			text := actionRecordRuleText(record)
			if strings.TrimSpace(text) == "" {
				continue
			}
			ref := fmt.Sprintf("%s:%d", rel, record.Line)
			out = append(out, actionRuleDraft{
				ID:           fmt.Sprintf("rule_%d", len(out)+1),
				Text:         text,
				Status:       "derived",
				Notes:        "derived from input material",
				EvidenceRefs: []string{ref},
			})
			if len(out) >= limit {
				break
			}
		}
		if len(out) >= limit {
			break
		}
	}
	return out, normalizeMaterialPaths(consumed), nil
}

func actionRecordRuleText(record actionRecord) string {
	if text := strings.TrimSpace(record.Fields["text"]); text != "" {
		return text
	}
	keys := make([]string, 0, len(record.Fields))
	for key := range record.Fields {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(record.Fields[key])
		if value == "" {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "; ")
}

type actionRuleDraft struct {
	ID           string   `json:"id"`
	Text         string   `json:"text"`
	Status       string   `json:"status"`
	Notes        string   `json:"notes"`
	EvidenceRefs []string `json:"evidence_refs"`
}

func parseActionRuleParamTexts(action DataAction) []actionRuleDraft {
	if raw := strings.TrimSpace(action.Params["rules_json"]); raw != "" {
		var parsed []actionRuleDraft
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			for i := range parsed {
				parsed[i] = normalizeActionRuleDraftTextID(parsed[i], fmt.Sprintf("rule_%d", i+1))
			}
			return parsed
		}
		var stringsOnly []string
		if err := json.Unmarshal([]byte(raw), &stringsOnly); err == nil {
			out := make([]actionRuleDraft, 0, len(stringsOnly))
			for i, text := range stringsOnly {
				if text = strings.TrimSpace(text); text != "" {
					out = append(out, actionRuleDraftFromText(fmt.Sprintf("rule_%d", i+1), text))
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
				out = append(out, actionRuleDraftFromText(fmt.Sprintf("rule_%d", len(out)+1), line))
			}
		}
		return out
	}
	return nil
}

var actionRuleLineIDPattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_.-]{0,127})[[:space:]]*(?::|：|=|--|—|–|-[[:space:]])[[:space:]]*(.+)$`)

func actionRuleDraftFromText(defaultID, raw string) actionRuleDraft {
	return normalizeActionRuleDraftTextID(actionRuleDraft{ID: defaultID, Text: raw}, defaultID)
}

func normalizeActionRuleDraftTextID(rule actionRuleDraft, defaultID string) actionRuleDraft {
	text := strings.TrimSpace(rule.Text)
	if text == "" || strings.TrimSpace(rule.ID) != "" && strings.TrimSpace(rule.ID) != strings.TrimSpace(defaultID) {
		return rule
	}
	if match := actionRuleLineIDPattern.FindStringSubmatch(text); len(match) == 3 {
		id := strings.TrimSpace(match[1])
		body := strings.TrimSpace(match[2])
		if id != "" && body != "" {
			rule.ID = id
			rule.Text = body
		}
	}
	return rule
}

func parseValidationRuleTexts(plan TaskPlan) []actionRuleDraft {
	out := make([]actionRuleDraft, 0, len(plan.CoverageContract.ValidationRules))
	for i, text := range plan.CoverageContract.ValidationRules {
		if text = strings.TrimSpace(text); text != "" {
			out = append(out, actionRuleDraft{ID: fmt.Sprintf("rule_%d", i+1), Text: text})
		}
	}
	return out
}

func normalizeActionRuleDraftIDs(in []actionRuleDraft) []actionRuleDraft {
	out := make([]actionRuleDraft, 0, len(in))
	seen := map[string]int{}
	for _, rule := range in {
		if strings.TrimSpace(rule.Text) == "" && strings.TrimSpace(rule.Notes) == "" {
			continue
		}
		id := strings.TrimSpace(rule.ID)
		if id == "" || seen[id] > 0 {
			id = fmt.Sprintf("rule_%d", len(out)+1)
		}
		seen[id]++
		rule.ID = id
		out = append(out, rule)
	}
	return out
}

func (r ActionRunner) runNormalizeEntities(action DataAction) (DataArtifact, []EntityResolutionRecord, error) {
	raw := strings.TrimSpace(action.Params["resolutions_json"])
	if raw == "" {
		raw = strings.TrimSpace(action.Params["mappings_json"])
	}
	if raw == "" {
		raw = strings.TrimSpace(action.Params["entity_resolutions_json"])
	}
	var records []EntityResolutionRecord
	var consumed []string
	var children []DataArtifact
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &records); err != nil {
			return DataArtifact{}, nil, fmt.Errorf("parse normalize_entities resolutions_json: %w", err)
		}
	} else {
		var err error
		records, consumed, children, err = r.deriveEntityResolutionsFromInputs(action)
		if err != nil {
			return DataArtifact{}, nil, err
		}
	}
	if len(records) > 0 {
		if err := validateEntityResolutionRecords(records); err != nil {
			return DataArtifact{}, nil, err
		}
	}
	if len(records) == 0 {
		id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "entity_resolutions")
		return DataArtifact{
			ID:          id,
			Kind:        string(DataActionNormalizeEntities),
			SourcePaths: normalizeMaterialPaths(consumed),
			Summary:     "normalized 0 entity value(s)",
			Fields:      map[string]string{"count": "0"},
			Children:    children,
		}, nil, nil
	}
	if len(children) == 0 {
		children = make([]DataArtifact, 0, len(records))
	}
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
		ID:          id,
		Kind:        string(DataActionNormalizeEntities),
		SourcePaths: normalizeMaterialPaths(consumed),
		Summary:     fmt.Sprintf("normalized %d entity value(s)", len(records)),
		Fields:      map[string]string{"count": fmt.Sprintf("%d", len(records))},
		Children:    children,
	}, records, nil
}

func (r ActionRunner) deriveEntityResolutionsFromInputs(action DataAction) ([]EntityResolutionRecord, []string, []DataArtifact, error) {
	paths := cleanStringList(action.InputPaths)
	if len(paths) == 0 {
		return nil, nil, nil, errors.New("normalize_entities action requires either params.resolutions_json or structured input_paths")
	}
	explicitSourceFields := normalizeEntitySourceFields(action)
	filters, err := parseContributionFilters(action)
	if err != nil {
		return nil, nil, nil, err
	}
	canonicalIDField := firstNonEmptyString(
		strings.TrimSpace(action.Params["canonical_id_field"]),
		strings.TrimSpace(action.Params["id_field"]),
	)
	canonicalLabelField := firstNonEmptyString(
		strings.TrimSpace(action.Params["canonical_label_field"]),
		strings.TrimSpace(action.Params["label_field"]),
	)
	itemIDField := strings.TrimSpace(action.Params["item_id_field"])
	resolutionStatusField := strings.TrimSpace(action.Params["resolution_status_field"])
	defaultStatus := firstNonEmptyString(strings.TrimSpace(action.Params["default_status"]), "resolved")
	reason := firstNonEmptyString(strings.TrimSpace(action.Params["reason"]), strings.TrimSpace(action.Purpose), "normalized by typed data action")
	ruleRefs := parseActionStringListParam(action.Params["rule_refs"])
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxResolutions := actionIntParam(action, "max_resolutions", 200000, 1, 500000)

	var out []EntityResolutionRecord
	var consumed []string
	var children []DataArtifact
	seen := map[string]bool{}
	for _, path := range paths {
		records, headers, total, rel, err := r.readActionRecords(path, maxRecords)
		if err != nil {
			return nil, nil, nil, err
		}
		consumed = append(consumed, rel)
		sourceFields := append([]string(nil), explicitSourceFields...)
		pathCanonicalIDField := canonicalIDField
		pathCanonicalLabelField := canonicalLabelField
		inferred := false
		if pathCanonicalIDField == "" {
			pathCanonicalIDField = inferEntityCanonicalIDField(headers)
			if pathCanonicalIDField != "" {
				inferred = true
			}
		}
		if pathCanonicalLabelField == "" {
			pathCanonicalLabelField = inferEntityCanonicalLabelField(headers, pathCanonicalIDField)
			if pathCanonicalLabelField != "" {
				inferred = true
			}
		}
		if len(sourceFields) == 0 {
			sourceFields = inferEntitySourceFields(headers, records, pathCanonicalIDField)
			if len(sourceFields) > 0 {
				inferred = true
			}
		}
		if len(sourceFields) == 0 {
			return nil, nil, nil, fmt.Errorf("normalize_entities structured mode could not infer source fields for %s; provide params.source_field, source_fields, or name_fields", rel)
		}
		matched := 0
		for _, record := range records {
			if !recordPassesFilters(record.Fields, filters) {
				continue
			}
			for _, field := range sourceFields {
				sourceValue := recordField(record.Fields, field)
				if sourceValue == "" {
					continue
				}
				if len(out) >= maxResolutions {
					return nil, nil, nil, fmt.Errorf("normalize_entities exceeded max_resolutions=%d; split the action into smaller filters or input groups", maxResolutions)
				}
				canonicalID := recordField(record.Fields, pathCanonicalIDField)
				canonicalLabel := recordField(record.Fields, pathCanonicalLabelField)
				if canonicalID == "" && canonicalLabel == "" {
					canonicalLabel = sourceValue
				}
				status := firstNonEmptyString(recordField(record.Fields, resolutionStatusField), defaultStatus)
				key := rel + "\x00" + field + "\x00" + sourceValue + "\x00" + canonicalID + "\x00" + canonicalLabel
				if seen[key] {
					continue
				}
				seen[key] = true
				itemID := recordField(record.Fields, itemIDField)
				if itemID == "" {
					itemID = fmt.Sprintf("%s#%d:%s", rel, record.Index, field)
				}
				out = append(out, EntityResolutionRecord{
					ItemID:         LooseText(itemID),
					SourceValue:    LooseText(sourceValue),
					CanonicalID:    LooseText(canonicalID),
					CanonicalLabel: LooseText(canonicalLabel),
					Status:         LooseText(status),
					EvidenceRefs:   []string{fmt.Sprintf("%s:%d", rel, record.Line)},
					RuleRefs:       append([]string(nil), ruleRefs...),
					Reason:         LooseText(reason),
				})
				matched++
			}
		}
		children = append(children, DataArtifact{
			ID:          rel + "#entity_resolutions",
			Kind:        "entity_resolution_source",
			SourcePaths: []string{rel},
			Headers:     headers,
			RowCount:    total,
			Summary:     fmt.Sprintf("%s produced %d entity resolution(s) from %d record(s)", rel, matched, total),
			Fields: map[string]string{
				"canonical_id_field":    pathCanonicalIDField,
				"canonical_label_field": pathCanonicalLabelField,
				"inferred_fields":       strings.Join(sourceFields, ","),
				"inferred_schema":       fmt.Sprintf("%t", inferred),
				"matched":               fmt.Sprintf("%d", matched),
				"total":                 fmt.Sprintf("%d", total),
			},
		})
	}
	return out, normalizeMaterialPaths(consumed), children, nil
}

func normalizeEntitySourceFields(action DataAction) []string {
	var fields []string
	for _, key := range []string{"source_field", "source_fields", "name_field", "name_fields", "value_field", "value_fields"} {
		fields = append(fields, parseActionStringListParam(action.Params[key])...)
	}
	return cleanStringSlice(fields)
}

func inferEntityCanonicalIDField(headers []string) string {
	if len(headers) == 0 {
		return ""
	}
	best := ""
	bestScore := -1
	for i, header := range headers {
		score := scoreEntityCanonicalIDHeader(header)
		if score > bestScore || (score == bestScore && best == "" && i == 0) {
			best = header
			bestScore = score
		}
	}
	if best != "" {
		return best
	}
	return headers[0]
}

func inferEntityCanonicalLabelField(headers []string, idField string) string {
	best := ""
	bestScore := -1
	for _, header := range headers {
		score := scoreEntityCanonicalLabelHeader(header)
		if strings.EqualFold(strings.TrimSpace(header), strings.TrimSpace(idField)) {
			score -= 2
		}
		if score > bestScore {
			best = header
			bestScore = score
		}
	}
	if best != "" && bestScore > 0 {
		return best
	}
	return idField
}

func inferEntitySourceFields(headers []string, records []actionRecord, canonicalIDField string) []string {
	var out []string
	for _, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" || strings.EqualFold(header, strings.TrimSpace(canonicalIDField)) {
			continue
		}
		if !fieldHasTextualEntityValues(header, records) {
			continue
		}
		out = append(out, header)
		if len(out) >= 12 {
			break
		}
	}
	if len(out) == 0 && strings.TrimSpace(canonicalIDField) != "" {
		out = append(out, strings.TrimSpace(canonicalIDField))
	}
	return cleanStringSlice(out)
}

func scoreEntityCanonicalIDHeader(header string) int {
	h := strings.ToLower(strings.TrimSpace(header))
	switch {
	case h == "id" || h == "key":
		return 100
	case strings.HasSuffix(h, "_id") || strings.HasSuffix(h, "-id") || strings.HasSuffix(h, " id"):
		return 90
	case strings.HasSuffix(h, "_code") || strings.HasSuffix(h, "-code") || strings.HasSuffix(h, " code"):
		return 80
	case strings.Contains(h, "canonical") && (strings.Contains(h, "id") || strings.Contains(h, "code")):
		return 75
	case strings.Contains(h, "id") || strings.Contains(h, "code") || strings.Contains(h, "key"):
		return 60
	default:
		return 0
	}
}

func scoreEntityCanonicalLabelHeader(header string) int {
	h := strings.ToLower(strings.TrimSpace(header))
	switch {
	case h == "label" || h == "name" || h == "title":
		return 100
	case strings.HasSuffix(h, "_label") || strings.HasSuffix(h, "-label") || strings.HasSuffix(h, " label"):
		return 90
	case strings.HasSuffix(h, "_name") || strings.HasSuffix(h, "-name") || strings.HasSuffix(h, " name"):
		return 90
	case strings.Contains(h, "label") || strings.Contains(h, "name") || strings.Contains(h, "title"):
		return 70
	case strings.Contains(h, "desc"):
		return 50
	default:
		return 0
	}
}

func fieldHasTextualEntityValues(field string, records []actionRecord) bool {
	checked := 0
	textual := 0
	for _, record := range records {
		value := recordField(record.Fields, field)
		if strings.TrimSpace(value) == "" {
			continue
		}
		checked++
		if looksLikeEntityText(value) {
			textual++
		}
		if checked >= 30 {
			break
		}
	}
	return checked > 0 && textual > 0
}

func looksLikeEntityText(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if _, ok := new(big.Rat).SetString(strings.ReplaceAll(value, ",", "")); ok {
		return false
	}
	lower := strings.ToLower(value)
	switch lower {
	case "true", "false", "yes", "no", "null", "none", "n/a":
		return false
	}
	hasLetter := false
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r > 127 {
			hasLetter = true
			break
		}
	}
	return hasLetter
}

type enrichRecordSpec struct {
	MappingPath         string
	SourceField         string
	MappingSourceFields []string
	MappingValueField   string
	TargetField         string
	MatchMode           string
	CaseInsensitive     bool
	Required            bool
	DefaultValue        string
	MappingFilters      []contributionFilter
}

type enrichCandidate struct {
	Source   string
	Value    string
	Evidence string
	Weight   int
}

type deriveFieldSpec struct {
	SourceField string
	TargetField string
	Operation   string
	Pattern     string
	Replacement string
	Value       string
	Default     string
	Mapping     map[string]string
	Multiplier  string
	Divisor     string
	Group       int
	Start       int
	Length      int
}

func (r ActionRunner) runDeriveFields(action DataAction) (DataArtifact, []map[string]any, []string, error) {
	paths := cleanStringList(action.InputPaths)
	inputPath := firstNonEmptyString(strings.TrimSpace(action.Params["input_path"]), strings.TrimSpace(action.Params["record_path"]), strings.TrimSpace(action.Params["base_path"]))
	if inputPath == "" && len(paths) > 0 {
		inputPath = paths[0]
	}
	if inputPath == "" {
		return DataArtifact{}, nil, nil, errors.New("derive_fields requires input_path or at least one input_path")
	}
	specs, err := parseDeriveFieldSpecs(action)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	if len(specs) == 0 {
		return DataArtifact{}, nil, nil, errors.New("derive_fields requires at least one field spec")
	}
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxOutput := actionIntParam(action, "max_output_records", 100000, 1, 1000000)
	records, headers, total, rel, err := r.readActionRecords(inputPath, maxRecords)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	knownFields := map[string]bool{}
	markKnownActionFields(knownFields, headers, records)
	for _, spec := range specs {
		if err := validateDeriveFieldSpec(spec, knownFields); err != nil {
			return DataArtifact{}, nil, nil, err
		}
	}
	rows := make([]map[string]any, 0, minInt(maxOutput, len(records)))
	derivedFields := make([]string, 0, len(specs))
	derivedNonEmpty := map[string]int{}
	for _, spec := range specs {
		derivedFields = append(derivedFields, spec.TargetField)
	}
	for _, record := range records {
		if len(rows) >= maxOutput {
			break
		}
		row := map[string]any{}
		for key, value := range record.Fields {
			row[key] = value
		}
		for _, spec := range specs {
			value := applyDeriveFieldSpec(record.Fields, spec)
			row[spec.TargetField] = value
			if strings.TrimSpace(value) != "" {
				derivedNonEmpty[spec.TargetField]++
			}
		}
		row["_source"] = rel
		row["_source_line"] = record.Line
		row["_source_index"] = record.Index
		rows = append(rows, row)
	}
	fields := map[string]string{
		"input_path":     rel,
		"input_rows":     fmt.Sprintf("%d", total),
		"output_rows":    fmt.Sprintf("%d", len(rows)),
		"derived_fields": strings.Join(derivedFields, ","),
	}
	for _, name := range derivedFields {
		fields["non_empty_"+name] = fmt.Sprintf("%d", derivedNonEmpty[name])
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "derived_fields")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionDeriveFields),
		SourcePaths: []string{rel},
		Headers:     collectJoinedRecordHeaders(rows),
		RowCount:    len(rows),
		Summary:     fmt.Sprintf("derived %d field(s) for %d record(s) from %s", len(specs), len(rows), rel),
		Sample:      sampleJoinedActionRows(rows, 3),
		Fields:      fields,
		Children: []DataArtifact{{
			ID:          rel + "#source",
			Kind:        "derive_fields/source",
			SourcePaths: []string{rel},
			Headers:     headers,
			RowCount:    total,
			Summary:     fmt.Sprintf("%s supplied %d source record(s)", rel, total),
		}},
	}, rows, []string{rel}, nil
}

func parseDeriveFieldSpecs(action DataAction) ([]deriveFieldSpec, error) {
	raw := strings.TrimSpace(firstNonEmptyString(action.Params["field_specs_json"], action.Params["derive_specs_json"], action.Params["transforms_json"]))
	if raw != "" {
		var values []map[string]any
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, fmt.Errorf("parse derive_fields field_specs_json: %w", err)
		}
		specs := make([]deriveFieldSpec, 0, len(values))
		for i, value := range values {
			spec, err := deriveFieldSpecFromMap(value)
			if err != nil {
				return nil, fmt.Errorf("derive_fields field_specs_json[%d]: %w", i, err)
			}
			spec = normalizeDeriveFieldSpec(spec)
			specs = append(specs, spec)
		}
		return specs, nil
	}
	spec := deriveFieldSpec{
		SourceField: firstNonEmptyString(action.Params["source_field"], action.Params["input_field"], action.Params["field"]),
		TargetField: firstNonEmptyString(action.Params["target_field"], action.Params["output_field"], action.Params["derived_field"]),
		Operation:   firstNonEmptyString(action.Params["operation"], action.Params["op"], action.Params["transform"]),
		Pattern:     action.Params["pattern"],
		Replacement: action.Params["replacement"],
		Value:       action.Params["value"],
		Default:     action.Params["default"],
		Multiplier:  firstNonEmptyString(action.Params["multiplier"], action.Params["scale"]),
		Divisor:     action.Params["divisor"],
		Group:       actionIntParam(action, "group", 1, 0, 64),
		Start:       actionIntParam(action, "start", 0, 0, 1000000),
		Length:      actionIntParam(action, "length", 0, 0, 1000000),
	}
	if rawMap := strings.TrimSpace(firstNonEmptyString(action.Params["mapping_json"], action.Params["map_json"])); rawMap != "" {
		mapping, err := parseDeriveFieldMapping(rawMap)
		if err != nil {
			return nil, err
		}
		spec.Mapping = mapping
	}
	spec = normalizeDeriveFieldSpec(spec)
	return []deriveFieldSpec{spec}, nil
}

func deriveFieldSpecFromMap(value map[string]any) (deriveFieldSpec, error) {
	getString := func(keys ...string) string {
		for _, key := range keys {
			if v, ok := value[key]; ok {
				return strings.TrimSpace(fmt.Sprint(v))
			}
		}
		return ""
	}
	getInt := func(def int, keys ...string) int {
		for _, key := range keys {
			v, ok := value[key]
			if !ok {
				continue
			}
			switch typed := v.(type) {
			case float64:
				return int(typed)
			case int:
				return typed
			default:
				if parsed, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v))); err == nil {
					return parsed
				}
			}
		}
		return def
	}
	spec := deriveFieldSpec{
		SourceField: getString("source_field", "input_field", "field"),
		TargetField: getString("target_field", "output_field", "derived_field"),
		Operation:   getString("operation", "op", "transform"),
		Pattern:     getString("pattern", "regex"),
		Replacement: getString("replacement"),
		Value:       getString("value"),
		Default:     getString("default", "default_value"),
		Multiplier:  getString("multiplier", "scale"),
		Divisor:     getString("divisor"),
		Group:       getInt(1, "group", "capture_group"),
		Start:       getInt(0, "start"),
		Length:      getInt(0, "length"),
	}
	if rawMapping, ok := value["mapping"]; ok {
		mapping, err := parseDeriveFieldMappingFromAny(rawMapping)
		if err != nil {
			return deriveFieldSpec{}, err
		}
		spec.Mapping = mapping
	}
	if rawMapping, ok := value["mapping_json"]; ok && len(spec.Mapping) == 0 {
		mapping, err := parseDeriveFieldMapping(fmt.Sprint(rawMapping))
		if err != nil {
			return deriveFieldSpec{}, err
		}
		spec.Mapping = mapping
	}
	return spec, nil
}

func parseDeriveFieldMapping(raw string) (map[string]string, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("parse derive_fields mapping_json: %w", err)
	}
	return parseDeriveFieldMappingFromAny(value)
}

func parseDeriveFieldMappingFromAny(value any) (map[string]string, error) {
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]string{}
		for k, v := range typed {
			out[strings.TrimSpace(k)] = strings.TrimSpace(fmt.Sprint(v))
		}
		return out, nil
	case map[string]string:
		out := map[string]string{}
		for k, v := range typed {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported derive_fields mapping shape %T; use an object of source-to-target scalar strings", value)
	}
}

func normalizeDeriveFieldSpec(spec deriveFieldSpec) deriveFieldSpec {
	spec.SourceField = strings.TrimSpace(spec.SourceField)
	spec.TargetField = strings.TrimSpace(spec.TargetField)
	spec.Operation = strings.ToLower(strings.TrimSpace(firstNonEmptyString(spec.Operation, "copy")))
	spec.Pattern = strings.TrimSpace(spec.Pattern)
	spec.Default = strings.TrimSpace(spec.Default)
	spec.Multiplier = strings.TrimSpace(spec.Multiplier)
	spec.Divisor = strings.TrimSpace(spec.Divisor)
	if spec.Group < 0 {
		spec.Group = 0
	}
	if spec.TargetField == "" {
		switch spec.Operation {
		case "year", "extract_year":
			spec.TargetField = spec.SourceField + "_year"
		case "parse_number", "number", "numeric":
			spec.TargetField = spec.SourceField + "_number"
		case "lower":
			spec.TargetField = spec.SourceField + "_lower"
		case "upper":
			spec.TargetField = spec.SourceField + "_upper"
		default:
			spec.TargetField = spec.SourceField + "_derived"
		}
	}
	return spec
}

func validateDeriveFieldSpec(spec deriveFieldSpec, knownFields map[string]bool) error {
	switch spec.Operation {
	case "constant":
		if spec.TargetField == "" {
			return errors.New("derive_fields constant spec requires target_field")
		}
		return nil
	}
	if spec.SourceField == "" {
		return fmt.Errorf("derive_fields %s spec requires source_field", spec.Operation)
	}
	if spec.TargetField == "" {
		return fmt.Errorf("derive_fields %s spec requires target_field", spec.Operation)
	}
	if !knownFields[strings.ToLower(strings.TrimSpace(spec.SourceField))] {
		return fmt.Errorf("derive_fields source_field %q was not found in input record fields", spec.SourceField)
	}
	switch spec.Operation {
	case "copy", "trim", "lower", "upper", "regex_extract", "regex_replace", "parse_number", "number", "numeric", "map", "lookup", "substring", "prefix", "suffix", "year", "extract_year":
	default:
		return fmt.Errorf("derive_fields unsupported operation %q", spec.Operation)
	}
	if (spec.Operation == "regex_extract" || spec.Operation == "regex_replace") && spec.Pattern == "" {
		return fmt.Errorf("derive_fields %s requires pattern", spec.Operation)
	}
	if (spec.Operation == "map" || spec.Operation == "lookup") && len(spec.Mapping) == 0 {
		return fmt.Errorf("derive_fields %s requires mapping/mapping_json", spec.Operation)
	}
	return nil
}

func applyDeriveFieldSpec(fields map[string]string, spec deriveFieldSpec) string {
	source := recordField(fields, spec.SourceField)
	var out string
	switch spec.Operation {
	case "constant":
		out = spec.Value
	case "copy":
		out = source
	case "trim":
		out = strings.TrimSpace(source)
	case "lower":
		out = strings.ToLower(strings.TrimSpace(source))
	case "upper":
		out = strings.ToUpper(strings.TrimSpace(source))
	case "regex_extract":
		out = deriveRegexExtract(source, spec)
	case "regex_replace":
		out = deriveRegexReplace(source, spec)
	case "parse_number", "number", "numeric":
		out = deriveParseNumber(source, spec)
	case "map", "lookup":
		out = deriveMapValue(source, spec)
	case "substring":
		out = deriveSubstring(source, spec.Start, spec.Length)
	case "prefix":
		out = deriveSubstring(source, 0, spec.Length)
	case "suffix":
		out = deriveSuffix(source, spec.Length)
	case "year", "extract_year":
		out = deriveExtractYear(source)
	default:
		out = source
	}
	if strings.TrimSpace(out) == "" && spec.Default != "" {
		return spec.Default
	}
	return strings.TrimSpace(out)
}

func deriveRegexExtract(source string, spec deriveFieldSpec) string {
	re, err := regexp.Compile(spec.Pattern)
	if err != nil {
		return ""
	}
	matches := re.FindStringSubmatch(source)
	if len(matches) == 0 {
		return ""
	}
	group := spec.Group
	if group <= 0 {
		group = 1
	}
	if group >= len(matches) {
		group = 0
	}
	return matches[group]
}

func deriveRegexReplace(source string, spec deriveFieldSpec) string {
	re, err := regexp.Compile(spec.Pattern)
	if err != nil {
		return ""
	}
	return re.ReplaceAllString(source, spec.Replacement)
}

func deriveParseNumber(source string, spec deriveFieldSpec) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	source = strings.ReplaceAll(source, ",", "")
	source = strings.ReplaceAll(source, "，", "")
	re := regexp.MustCompile(`[-+]?\d+(?:\.\d+)?`)
	raw := re.FindString(source)
	if raw == "" {
		return ""
	}
	rat, err := parseDecimalRat(raw)
	if err != nil {
		return raw
	}
	if spec.Multiplier != "" {
		multiplier, err := parseDecimalRat(spec.Multiplier)
		if err != nil {
			return ""
		}
		rat.Mul(rat, multiplier)
	}
	if spec.Divisor != "" {
		divisor, err := parseDecimalRat(spec.Divisor)
		if err != nil || divisor.Sign() == 0 {
			return ""
		}
		rat.Quo(rat, divisor)
	}
	return formatRat(rat)
}

func deriveMapValue(source string, spec deriveFieldSpec) string {
	key := strings.TrimSpace(source)
	if value, ok := spec.Mapping[key]; ok {
		return value
	}
	lower := strings.ToLower(key)
	for k, v := range spec.Mapping {
		if strings.ToLower(strings.TrimSpace(k)) == lower {
			return v
		}
	}
	return ""
}

func deriveSubstring(source string, start, length int) string {
	runes := []rune(source)
	if start < 0 {
		start = 0
	}
	if start >= len(runes) {
		return ""
	}
	end := len(runes)
	if length > 0 && start+length < end {
		end = start + length
	}
	return string(runes[start:end])
}

func deriveSuffix(source string, length int) string {
	runes := []rune(source)
	if length <= 0 || length >= len(runes) {
		return source
	}
	return string(runes[len(runes)-length:])
}

func deriveExtractYear(source string) string {
	re := regexp.MustCompile(`(?:^|[^\d])((?:19|20)\d{2})(?:[^\d]|$)`)
	matches := re.FindStringSubmatch(source)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func (r ActionRunner) runEnrichRecords(action DataAction) (DataArtifact, []map[string]any, []string, error) {
	paths := cleanStringList(action.InputPaths)
	basePath := firstNonEmptyString(strings.TrimSpace(action.Params["base_path"]), strings.TrimSpace(action.Params["record_path"]))
	if basePath == "" && len(paths) > 0 {
		basePath = paths[0]
	}
	if basePath == "" {
		return DataArtifact{}, nil, nil, errors.New("enrich_records requires a base_path or at least one input_path")
	}
	specs, err := parseEnrichRecordSpecs(action, paths, basePath)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	if len(specs) == 0 {
		return DataArtifact{}, nil, nil, errors.New("enrich_records requires at least one mapping spec")
	}
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxOutput := actionIntParam(action, "max_output_records", 100000, 1, 1000000)
	baseRecords, baseHeaders, baseTotal, baseRel, err := r.readActionRecords(basePath, maxRecords)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	rows := make([]map[string]any, 0, minInt(maxOutput, len(baseRecords)))
	consumed := []string{baseRel}
	children := make([]DataArtifact, 0, len(specs)+1)
	children = append(children, DataArtifact{
		ID:          baseRel + "#base",
		Kind:        "enrich_records/base",
		SourcePaths: []string{baseRel},
		Headers:     baseHeaders,
		RowCount:    baseTotal,
		Summary:     fmt.Sprintf("%s supplied %d base record(s)", baseRel, baseTotal),
	})
	lookups := make([]map[string][]enrichCandidate, 0, len(specs))
	for _, spec := range specs {
		records, headers, total, rel, err := r.readActionRecords(spec.MappingPath, maxRecords)
		if err != nil {
			return DataArtifact{}, nil, nil, err
		}
		consumed = append(consumed, rel)
		lookup := buildEnrichLookup(records, rel, spec)
		lookups = append(lookups, lookup)
		children = append(children, DataArtifact{
			ID:          rel + "#mapping",
			Kind:        "enrich_records/mapping",
			SourcePaths: []string{rel},
			Headers:     headers,
			RowCount:    total,
			Summary:     fmt.Sprintf("%s supplied %d mapping candidate(s) for target field %s", rel, len(flattenEnrichLookupCandidates(lookup)), spec.TargetField),
			Fields: map[string]string{
				"source_field":          spec.SourceField,
				"mapping_source_fields": strings.Join(spec.MappingSourceFields, ","),
				"mapping_value_field":   spec.MappingValueField,
				"target_field":          spec.TargetField,
				"match_mode":            spec.MatchMode,
			},
		})
	}
	matchesByTarget := map[string]int{}
	for _, base := range baseRecords {
		if len(rows) >= maxOutput {
			break
		}
		row := map[string]any{}
		for k, v := range base.Fields {
			row[k] = v
		}
		row["_source"] = baseRel
		row["_source_line"] = base.Line
		row["_source_index"] = base.Index
		for i, spec := range specs {
			sourceValue := recordField(base.Fields, spec.SourceField)
			value, status, evidence := selectEnrichValue(sourceValue, lookups[i], spec)
			if value == "" {
				value = spec.DefaultValue
			}
			row[spec.TargetField] = value
			row[spec.TargetField+"_status"] = status
			if evidence != "" {
				row[spec.TargetField+"_evidence"] = evidence
			}
			if status == "matched" {
				matchesByTarget[spec.TargetField]++
			}
		}
		rows = append(rows, row)
	}
	headers := collectJoinedRecordHeaders(rows)
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "enriched_records")
	fieldSummary := make([]string, 0, len(specs))
	fields := map[string]string{
		"base_path":     baseRel,
		"base_rows":     fmt.Sprintf("%d", baseTotal),
		"enriched_rows": fmt.Sprintf("%d", len(rows)),
	}
	for _, spec := range specs {
		fieldSummary = append(fieldSummary, spec.TargetField)
		fields["matches_"+spec.TargetField] = fmt.Sprintf("%d", matchesByTarget[spec.TargetField])
	}
	fields["target_fields"] = strings.Join(fieldSummary, ",")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionEnrichRecords),
		SourcePaths: normalizeMaterialPaths(consumed),
		Headers:     headers,
		RowCount:    len(rows),
		Summary:     fmt.Sprintf("enriched %d record(s) from %s with %d mapping spec(s)", len(rows), baseRel, len(specs)),
		Sample:      sampleJoinedActionRows(rows, 3),
		Fields:      fields,
		Children:    children,
	}, rows, normalizeMaterialPaths(consumed), nil
}

func parseEnrichRecordSpecs(action DataAction, paths []string, basePath string) ([]enrichRecordSpec, error) {
	if raw := strings.TrimSpace(firstNonEmptyString(action.Params["mapping_specs_json"], action.Params["map_specs_json"], action.Params["enrich_specs_json"])); raw != "" {
		var values []map[string]any
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, fmt.Errorf("parse enrich_records mapping_specs_json: %w", err)
		}
		specs := make([]enrichRecordSpec, 0, len(values))
		for i, value := range values {
			spec := enrichRecordSpecFromMap(value)
			if spec.MappingPath == "" {
				mappingPaths := nonBaseEnrichPaths(paths, basePath)
				if i < len(mappingPaths) {
					spec.MappingPath = mappingPaths[i]
				}
			}
			spec = normalizeEnrichRecordSpec(spec)
			if err := validateEnrichRecordSpec(spec); err != nil {
				return nil, err
			}
			specs = append(specs, spec)
		}
		return specs, nil
	}
	spec := enrichRecordSpec{
		MappingPath:         firstNonEmptyString(action.Params["mapping_path"], action.Params["reference_path"], action.Params["lookup_path"]),
		SourceField:         firstNonEmptyString(action.Params["source_field"], action.Params["base_field"], action.Params["left_field"], action.Params["key_field"]),
		MappingSourceFields: parseActionStringListParam(firstNonEmptyString(action.Params["mapping_source_fields"], action.Params["mapping_source_field"], action.Params["reference_fields"], action.Params["reference_field"], action.Params["lookup_fields"], action.Params["lookup_field"])),
		MappingValueField:   firstNonEmptyString(action.Params["mapping_value_field"], action.Params["reference_value_field"], action.Params["canonical_id_field"], action.Params["value_field"]),
		TargetField:         firstNonEmptyString(action.Params["target_field"], action.Params["output_field"], action.Params["enriched_field"]),
		MatchMode:           firstNonEmptyString(action.Params["match_mode"], action.Params["mode"]),
		DefaultValue:        action.Params["default_value"],
		Required:            strings.EqualFold(strings.TrimSpace(action.Params["required"]), "true"),
	}
	if spec.MappingPath == "" {
		for _, p := range nonBaseEnrichPaths(paths, basePath) {
			spec.MappingPath = p
			break
		}
	}
	if raw := strings.TrimSpace(action.Params["mapping_filters_json"]); raw != "" {
		filters, err := parseContributionFilters(DataAction{Params: map[string]string{"filters_json": raw}})
		if err != nil {
			return nil, err
		}
		spec.MappingFilters = filters
	}
	spec = normalizeEnrichRecordSpec(spec)
	if err := validateEnrichRecordSpec(spec); err != nil {
		return nil, err
	}
	return []enrichRecordSpec{spec}, nil
}

func enrichRecordSpecFromMap(value map[string]any) enrichRecordSpec {
	getString := func(keys ...string) string {
		for _, key := range keys {
			if v, ok := value[key]; ok {
				return strings.TrimSpace(fmt.Sprint(v))
			}
		}
		return ""
	}
	getList := func(keys ...string) []string {
		for _, key := range keys {
			if v, ok := value[key]; ok {
				return parseActionStringListParamFromAny(v)
			}
		}
		return nil
	}
	spec := enrichRecordSpec{
		MappingPath:         getString("mapping_path", "reference_path", "lookup_path"),
		SourceField:         getString("source_field", "base_field", "left_field", "key_field"),
		MappingSourceFields: getList("mapping_source_fields", "mapping_source_field", "reference_fields", "reference_field", "lookup_fields", "lookup_field"),
		MappingValueField:   getString("mapping_value_field", "reference_value_field", "canonical_id_field", "value_field"),
		TargetField:         getString("target_field", "output_field", "enriched_field"),
		MatchMode:           getString("match_mode", "mode"),
		DefaultValue:        getString("default_value"),
		Required:            strings.EqualFold(getString("required"), "true"),
	}
	if filters, ok := value["mapping_filters"]; ok {
		if raw, err := json.Marshal(filters); err == nil {
			parsed, err := parseContributionFilters(DataAction{Params: map[string]string{"filters_json": string(raw)}})
			if err == nil {
				spec.MappingFilters = parsed
			}
		}
	}
	if filters, ok := value["mapping_filters_json"]; ok {
		parsed, err := parseContributionFilters(DataAction{Params: map[string]string{"filters_json": fmt.Sprint(filters)}})
		if err == nil {
			spec.MappingFilters = parsed
		}
	}
	return spec
}

func parseActionStringListParamFromAny(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return cleanStringSlice(out)
	case []string:
		return cleanStringSlice(v)
	default:
		return parseActionStringListParam(fmt.Sprint(value))
	}
}

func nonBaseEnrichPaths(paths []string, basePath string) []string {
	var out []string
	base := normalizeMaterialPath(basePath)
	for _, p := range paths {
		if normalizeMaterialPath(p) == "" || normalizeMaterialPath(p) == base {
			continue
		}
		out = append(out, p)
	}
	return out
}

func normalizeEnrichRecordSpec(spec enrichRecordSpec) enrichRecordSpec {
	spec.MappingPath = normalizeMaterialPath(spec.MappingPath)
	spec.SourceField = strings.TrimSpace(spec.SourceField)
	spec.MappingSourceFields = cleanStringSlice(spec.MappingSourceFields)
	spec.MappingValueField = strings.TrimSpace(firstNonEmptyString(spec.MappingValueField, "canonical_id"))
	if len(spec.MappingSourceFields) == 0 {
		spec.MappingSourceFields = []string{"source_value"}
	}
	spec.TargetField = strings.TrimSpace(firstNonEmptyString(spec.TargetField, spec.MappingValueField, "enriched_"+spec.SourceField))
	spec.MatchMode = strings.ToLower(strings.TrimSpace(firstNonEmptyString(spec.MatchMode, "exact")))
	return spec
}

func validateEnrichRecordSpec(spec enrichRecordSpec) error {
	if spec.MappingPath == "" {
		return errors.New("enrich_records mapping spec requires mapping_path/reference_path or an additional input_path")
	}
	if spec.SourceField == "" {
		return errors.New("enrich_records mapping spec requires source_field/base_field")
	}
	if len(spec.MappingSourceFields) == 0 {
		return errors.New("enrich_records mapping spec requires mapping_source_field/reference_field")
	}
	if spec.MappingValueField == "" || spec.TargetField == "" {
		return errors.New("enrich_records mapping spec requires mapping_value_field and target_field")
	}
	return nil
}

func buildEnrichLookup(records []actionRecord, rel string, spec enrichRecordSpec) map[string][]enrichCandidate {
	out := map[string][]enrichCandidate{}
	for _, record := range records {
		if !recordPassesFilters(record.Fields, spec.MappingFilters) {
			continue
		}
		value := recordField(record.Fields, spec.MappingValueField)
		if value == "" {
			continue
		}
		for _, sourceField := range spec.MappingSourceFields {
			source := recordField(record.Fields, sourceField)
			if source == "" {
				continue
			}
			candidate := enrichCandidate{
				Source:   source,
				Value:    value,
				Evidence: fmt.Sprintf("%s:%d", rel, record.Line),
				Weight:   len([]rune(source)),
			}
			key := normalizeEnrichKey(source, spec)
			out[key] = append(out[key], candidate)
		}
	}
	for key := range out {
		sort.SliceStable(out[key], func(i, j int) bool {
			return out[key][i].Weight > out[key][j].Weight
		})
	}
	return out
}

func flattenEnrichLookupCandidates(in map[string][]enrichCandidate) []enrichCandidate {
	var out []enrichCandidate
	for _, values := range in {
		out = append(out, values...)
	}
	return out
}

func selectEnrichValue(sourceValue string, lookup map[string][]enrichCandidate, spec enrichRecordSpec) (value, status, evidence string) {
	sourceValue = strings.TrimSpace(sourceValue)
	if sourceValue == "" {
		if spec.Required {
			return "", "missing_source", ""
		}
		return "", "unmatched", ""
	}
	var candidates []enrichCandidate
	switch spec.MatchMode {
	case "contains", "mapping_contains_source", "reference_contains_source":
		needle := normalizeEnrichKey(sourceValue, spec)
		for _, values := range lookup {
			for _, candidate := range values {
				haystack := normalizeEnrichKey(candidate.Source, spec)
				if (spec.MatchMode == "contains" && (strings.Contains(haystack, needle) || strings.Contains(needle, haystack))) ||
					(spec.MatchMode != "contains" && strings.Contains(haystack, needle)) {
					candidates = append(candidates, candidate)
				}
			}
		}
	case "source_contains_mapping":
		needle := normalizeEnrichKey(sourceValue, spec)
		for _, values := range lookup {
			for _, candidate := range values {
				part := normalizeEnrichKey(candidate.Source, spec)
				if strings.Contains(needle, part) {
					candidates = append(candidates, candidate)
				}
			}
		}
	default:
		candidates = lookup[normalizeEnrichKey(sourceValue, spec)]
	}
	if len(candidates) == 0 {
		if spec.Required {
			return "", "unmatched_required", ""
		}
		return "", "unmatched", ""
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Weight > candidates[j].Weight
	})
	best := candidates[0]
	ambiguous := false
	for _, candidate := range candidates[1:] {
		if candidate.Value != best.Value {
			ambiguous = true
			break
		}
	}
	if ambiguous && spec.Required {
		return "", "ambiguous", best.Evidence
	}
	status = "matched"
	if ambiguous {
		status = "matched_ambiguous"
	}
	return best.Value, status, best.Evidence
}

func normalizeEnrichKey(value string, spec enrichRecordSpec) string {
	value = strings.TrimSpace(value)
	// Data enrichment is a matching step over user-provided materials, not a
	// canonical value rewrite. Match keys case-insensitively by default while
	// preserving the mapped value exactly as recorded in the reference row.
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "；", ";")
	value = strings.ReplaceAll(value, "，", ",")
	return value
}

func (r ActionRunner) runJoinRecords(action DataAction) (DataArtifact, []map[string]any, []string, error) {
	paths := cleanStringList(action.InputPaths)
	leftPath := firstNonEmptyString(strings.TrimSpace(action.Params["left_path"]), strings.TrimSpace(action.Params["left"]))
	rightPath := firstNonEmptyString(strings.TrimSpace(action.Params["right_path"]), strings.TrimSpace(action.Params["right"]))
	if leftPath == "" && len(paths) > 0 {
		leftPath = paths[0]
	}
	if rightPath == "" && len(paths) > 1 {
		rightPath = paths[1]
	}
	if leftPath == "" || rightPath == "" {
		return DataArtifact{}, nil, nil, errors.New("join_records requires left_path/right_path or at least two input_paths")
	}
	leftFields := parseActionStringListParam(firstNonEmptyString(
		action.Params["left_fields"],
		action.Params["left_keys"],
		action.Params["left_key"],
		action.Params["join_fields"],
		action.Params["join_key"],
	))
	rightFields := parseActionStringListParam(firstNonEmptyString(
		action.Params["right_fields"],
		action.Params["right_keys"],
		action.Params["right_key"],
	))
	if len(rightFields) == 0 {
		rightFields = append([]string(nil), leftFields...)
	}
	if len(leftFields) == 0 || len(rightFields) == 0 {
		return DataArtifact{}, nil, nil, errors.New("join_records requires left_fields/right_fields or join_fields params")
	}
	if len(leftFields) != len(rightFields) {
		return DataArtifact{}, nil, nil, fmt.Errorf("join_records field count mismatch: left_fields=%d right_fields=%d", len(leftFields), len(rightFields))
	}
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxOutput := actionIntParam(action, "max_output_records", 100000, 1, 1000000)
	leftRecords, leftHeaders, leftTotal, leftRel, err := r.readActionRecords(leftPath, maxRecords)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	rightRecords, rightHeaders, rightTotal, rightRel, err := r.readActionRecords(rightPath, maxRecords)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	knownFields := map[string]bool{}
	markKnownActionFields(knownFields, leftHeaders, leftRecords)
	markKnownActionFields(knownFields, rightHeaders, rightRecords)
	for _, field := range append(append([]string(nil), leftFields...), rightFields...) {
		if !knownFields[strings.ToLower(strings.TrimSpace(field))] {
			return DataArtifact{}, nil, nil, fmt.Errorf("join_records field %q was not found in any input record field", field)
		}
	}
	rightIndex := map[string][]actionRecord{}
	for _, rec := range rightRecords {
		key := joinActionRecordKey(rec.Fields, rightFields)
		if key == "" {
			continue
		}
		rightIndex[key] = append(rightIndex[key], rec)
	}
	joinType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action.Params["join_type"], action.Params["type"], "inner")))
	rightPrefix := strings.TrimSpace(action.Params["right_prefix"])
	leftPrefix := strings.TrimSpace(action.Params["left_prefix"])
	collision := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action.Params["collision"], "prefix")))
	var rows []map[string]any
	matches := 0
	for _, left := range leftRecords {
		key := joinActionRecordKey(left.Fields, leftFields)
		matched := rightIndex[key]
		if len(matched) == 0 {
			if joinType == "left" || joinType == "left_outer" {
				rows = append(rows, buildJoinedActionRecord(left, actionRecord{}, leftRel, "", leftPrefix, rightPrefix, collision))
				if len(rows) >= maxOutput {
					break
				}
			}
			continue
		}
		for _, right := range matched {
			rows = append(rows, buildJoinedActionRecord(left, right, leftRel, rightRel, leftPrefix, rightPrefix, collision))
			matches++
			if len(rows) >= maxOutput {
				break
			}
		}
		if len(rows) >= maxOutput {
			break
		}
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "joined_records")
	summary := fmt.Sprintf("joined %d record(s) from %s and %s", len(rows), leftRel, rightRel)
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionJoinRecords),
		SourcePaths: normalizeMaterialPaths([]string{leftRel, rightRel}),
		Headers:     collectJoinedRecordHeaders(rows),
		RowCount:    len(rows),
		Summary:     summary,
		Sample:      sampleJoinedActionRows(rows, 3),
		Fields: map[string]string{
			"left_path":    leftRel,
			"right_path":   rightRel,
			"left_rows":    fmt.Sprintf("%d", leftTotal),
			"right_rows":   fmt.Sprintf("%d", rightTotal),
			"joined_rows":  fmt.Sprintf("%d", len(rows)),
			"matches":      fmt.Sprintf("%d", matches),
			"join_type":    joinType,
			"left_fields":  strings.Join(leftFields, ","),
			"right_fields": strings.Join(rightFields, ","),
		},
	}, rows, normalizeMaterialPaths([]string{leftRel, rightRel}), nil
}

func joinActionRecordKey(fields map[string]string, keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := recordField(fields, key)
		if value == "" {
			return ""
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, "\x1f")
}

func buildJoinedActionRecord(left, right actionRecord, leftRel, rightRel, leftPrefix, rightPrefix, collision string) map[string]any {
	out := map[string]any{}
	addField := func(prefix string, key string, value string, prefer bool) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		target := key
		if strings.TrimSpace(prefix) != "" {
			target = strings.TrimSpace(prefix) + "_" + key
		}
		if existing, ok := out[target]; ok && fmt.Sprint(existing) != value {
			switch collision {
			case "right", "overwrite":
				if prefer {
					out[target] = value
				}
			case "left", "keep_left":
				return
			default:
				collisionKey := strings.TrimSpace(prefix)
				if collisionKey == "" {
					collisionKey = "right"
				}
				out[collisionKey+"_"+key] = value
			}
			return
		}
		out[target] = value
	}
	for key, value := range left.Fields {
		addField(leftPrefix, key, value, false)
	}
	if right.Fields != nil {
		for key, value := range right.Fields {
			addField(rightPrefix, key, value, true)
		}
	}
	out["_left_source"] = leftRel
	out["_left_line"] = left.Line
	out["_left_index"] = left.Index
	if rightRel != "" {
		out["_right_source"] = rightRel
		out["_right_line"] = right.Line
		out["_right_index"] = right.Index
	}
	return out
}

func collectJoinedRecordHeaders(rows []map[string]any) []string {
	seen := map[string]bool{}
	for _, row := range rows {
		for key := range row {
			key = strings.TrimSpace(key)
			if key != "" {
				seen[key] = true
			}
		}
	}
	headers := make([]string, 0, len(seen))
	for key := range seen {
		headers = append(headers, key)
	}
	sort.Strings(headers)
	return headers
}

func sampleJoinedActionRows(rows []map[string]any, limit int) []string {
	if limit <= 0 || len(rows) == 0 {
		return nil
	}
	out := make([]string, 0, minInt(limit, len(rows)))
	for i, row := range rows {
		if i >= limit {
			break
		}
		out = append(out, compactJSONLine(row))
	}
	return out
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

type contributionFilterDraft struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

func (r ActionRunner) runComputeContributions(action DataAction, defaultRuleRefs []string) (DataArtifact, []ContributionRecord, []string, error) {
	paths := cleanStringList(action.InputPaths)
	if len(paths) == 0 {
		return DataArtifact{}, nil, nil, errors.New("compute_contributions action has no input_paths")
	}
	groupKeyField := strings.TrimSpace(action.Params["group_key_field"])
	groupKeyConst := strings.TrimSpace(action.Params["group_key"])
	valueField := strings.TrimSpace(action.Params["value_field"])
	rawMetric := strings.TrimSpace(action.Params["metric"])
	rawOperation := strings.TrimSpace(action.Params["operation"])
	metric := firstNonEmptyString(rawMetric, valueField, "count")
	operation := firstNonEmptyString(rawOperation, "add")
	implicitMaterialCount := valueField == "" && rawOperation == "" && rawMetric == "" && groupKeyField == "" && groupKeyConst == ""
	if valueField == "" && rawOperation == "" {
		operation = "count"
	}
	operation, ok := normalizeContributionOperation(operation)
	if !ok || operation == "" {
		return DataArtifact{}, nil, nil, fmt.Errorf("compute_contributions unsupported operation %q", action.Params["operation"])
	}
	role := firstNonEmptyString(strings.TrimSpace(action.Params["role"]), strings.TrimSpace(action.Params["scope"]))
	if role == "" {
		if implicitMaterialCount {
			role = "audit"
		} else {
			role = "target"
		}
	}
	itemIDField := strings.TrimSpace(action.Params["item_id_field"])
	reason := firstNonEmptyString(strings.TrimSpace(action.Params["reason"]), strings.TrimSpace(action.Purpose), "computed by typed data action")
	ruleRefs := parseActionStringListParam(action.Params["rule_refs"])
	if len(ruleRefs) == 0 {
		ruleRefs = append([]string(nil), defaultRuleRefs...)
	}
	filters, err := parseContributionFilters(action)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	filterFields := make([]string, 0, len(filters))
	for _, filter := range filters {
		filterFields = append(filterFields, filter.Field)
	}
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxContribs := actionIntParam(action, "max_contributions", 50000, 1, 200000)

	var contributions []ContributionRecord
	var consumed []string
	children := make([]DataArtifact, 0, len(paths))
	knownFields := map[string]bool{}
	for _, path := range paths {
		records, headers, total, rel, err := r.readActionRecords(path, maxRecords)
		if err != nil {
			return DataArtifact{}, nil, nil, err
		}
		markKnownActionFields(knownFields, headers, records)
		consumed = append(consumed, rel)
		matched := 0
		for _, record := range records {
			if !recordPassesFilters(record.Fields, filters) {
				continue
			}
			if len(contributions) >= maxContribs {
				return DataArtifact{}, nil, nil, fmt.Errorf("compute_contributions exceeded max_contributions=%d; split the action into smaller groups or filters", maxContribs)
			}
			value := ""
			if valueField != "" {
				value = recordField(record.Fields, valueField)
				if value == "" && operation != "count" {
					continue
				}
			}
			if value == "" && operation == "count" {
				value = "1"
			}
			matched++
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
				Role:          LooseText(role),
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
	if err := validateComputeContributionFieldContract(action, knownFields, valueField, groupKeyField, groupKeyConst, filterFields); err != nil {
		return DataArtifact{}, nil, nil, err
	}
	if len(contributions) > 0 {
		if err := validateContributionRecords(contributions); err != nil {
			return DataArtifact{}, nil, nil, err
		}
	}
	if len(contributions) == 0 {
		id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "contributions")
		return DataArtifact{
			ID:          id,
			Kind:        string(DataActionComputeContribs),
			SourcePaths: normalizeMaterialPaths(consumed),
			Summary:     "computed 0 generic contribution(s)",
			Fields:      map[string]string{"count": "0"},
			Children:    children,
		}, nil, normalizeMaterialPaths(consumed), nil
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

func markKnownActionFields(out map[string]bool, headers []string, records []actionRecord) {
	mark := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = true
		}
	}
	for _, header := range headers {
		mark(header)
	}
	for _, record := range records {
		for key := range record.Fields {
			mark(key)
		}
	}
}

func validateComputeContributionFieldContract(action DataAction, knownFields map[string]bool, valueField, groupKeyField, groupKeyConst string, filterFields []string) error {
	hasField := func(name string) bool {
		name = strings.ToLower(strings.TrimSpace(name))
		return name == "" || knownFields[name]
	}
	if strings.TrimSpace(valueField) != "" && !hasField(valueField) {
		return fmt.Errorf("compute_contributions value_field %q was not found in any input record field; use extract_records/inspect_material first or set value_field to an existing field", valueField)
	}
	if strings.TrimSpace(groupKeyField) != "" && strings.TrimSpace(groupKeyConst) == "" && !hasField(groupKeyField) {
		return fmt.Errorf("compute_contributions group_key_field %q was not found in any input record field; use extract_records/inspect_material first or set group_key/group_key_field to an existing field", groupKeyField)
	}
	for _, field := range filterFields {
		if strings.TrimSpace(field) != "" && !hasField(field) {
			return fmt.Errorf("compute_contributions filter field %q was not found in any input record field; use extract_records/inspect_material first or set filters_json to existing fields", field)
		}
	}
	return nil
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

func (r ActionRunner) prepareArtifactWorkspace() (string, func(), error) {
	tempRoot := strings.TrimSpace(r.TempRoot)
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	tempRoot, err := filepath.Abs(tempRoot)
	if err != nil {
		return "", func() {}, err
	}
	if err := os.MkdirAll(tempRoot, 0700); err != nil {
		return "", func() {}, err
	}
	dir, err := os.MkdirTemp(tempRoot, "codrax-data-actions-*")
	if err != nil {
		return "", func() {}, err
	}
	if strings.TrimSpace(r.TempRoot) != "" {
		return dir, func() {}, nil
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

func (r ActionRunner) materializeActionArtifact(dir string, action DataAction, artifact DataArtifact, payload any) (DataArtifact, error) {
	if r.artifactFiles == nil || strings.TrimSpace(dir) == "" {
		return artifact, nil
	}
	aliases := dataActionArtifactAliases(action, artifact)
	if len(aliases) == 0 {
		return artifact, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return artifact, fmt.Errorf("materialize data action artifact %q: %w", firstNonEmptyString(action.ID, artifact.ID), err)
	}
	fileName := safeActionArtifactFileName(firstNonEmptyString(action.OutputArtifact, action.ID, artifact.ID, "artifact")) + ".json"
	abs := filepath.Join(dir, fileName)
	if err := os.WriteFile(abs, raw, 0600); err != nil {
		return artifact, fmt.Errorf("write data action artifact %q: %w", fileName, err)
	}
	if artifact.Fields == nil {
		artifact.Fields = map[string]string{}
	}
	if shape := dataActionArtifactJSONShape(raw); shape != "" {
		artifact.Fields["json_shape"] = shape
	}
	artifact.Fields["artifact_path"] = abs
	artifact.Fields["artifact_aliases"] = strings.Join(aliases, ",")
	for _, alias := range aliases {
		r.registerActionArtifactAlias(alias, abs)
	}
	return artifact, nil
}

func dataActionArtifactJSONShape(raw []byte) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return compactJSONShape(value, 0)
}

func compactJSONShape(value any, depth int) string {
	if depth > 2 {
		return "..."
	}
	switch v := value.(type) {
	case []any:
		if len(v) == 0 {
			return "array(len=0)"
		}
		return fmt.Sprintf("array(len=%d,item=%s)", len(v), compactJSONShape(firstNonNilJSONValue(v), depth+1))
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > 12 {
			keys = keys[:12]
			keys = append(keys, "...")
		}
		return "object(keys=" + strings.Join(keys, ",") + ")"
	case string:
		return "string"
	case float64, int, int64, uint64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return strings.ToLower(fmt.Sprintf("%T", value))
	}
}

func firstNonNilJSONValue(values []any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func dataActionArtifactAliases(action DataAction, artifact DataArtifact) []string {
	baseAliases := []string{action.ID, action.OutputArtifact, artifact.ID}
	var aliases []string
	for _, alias := range baseAliases {
		alias = normalizeMaterialPath(alias)
		if alias == "" {
			continue
		}
		aliases = append(aliases, alias)
		if !strings.HasSuffix(alias, ".json") {
			aliases = append(aliases, alias+".json")
		}
		base := strings.TrimSuffix(path.Base(alias), ".json")
		for _, namespaced := range []string{
			path.Join("artifacts", alias),
			path.Join("artifacts", alias+".json"),
			path.Join("artifacts", base),
			path.Join("artifacts", base+".json"),
		} {
			if strings.TrimSpace(namespaced) != "" {
				aliases = append(aliases, namespaced)
			}
		}
	}
	actionID := normalizeMaterialPath(action.ID)
	artifactID := normalizeMaterialPath(artifact.ID)
	if actionID != "" && artifactID != "" {
		artifactBase := strings.TrimSuffix(path.Base(artifactID), ".json")
		for _, namespaced := range []string{
			path.Join("artifacts", actionID, artifactID),
			path.Join("artifacts", actionID, artifactID+".json"),
			path.Join("artifacts", actionID, artifactBase),
			path.Join("artifacts", actionID, artifactBase+".json"),
		} {
			if strings.TrimSpace(namespaced) != "" {
				aliases = append(aliases, namespaced)
			}
		}
	}
	return cleanStringList(aliases)
}

func contributionActionArtifactPayload(artifact DataArtifact, records []ContributionRecord) map[string]any {
	return map[string]any{
		"artifact":      artifact,
		"kind":          artifact.Kind,
		"id":            artifact.ID,
		"source_paths":  artifact.SourcePaths,
		"summary":       artifact.Summary,
		"fields":        artifact.Fields,
		"contributions": records,
		"records":       records,
	}
}

func ruleCoverageIDs(records []RuleCoverageRecord) []string {
	var out []string
	seen := map[string]bool{}
	for _, rec := range records {
		id := strings.TrimSpace(rec.RuleID.String())
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func (r ActionRunner) registerActionArtifactAlias(alias, abs string) {
	alias = normalizeMaterialPath(alias)
	abs = strings.TrimSpace(abs)
	if alias == "" || abs == "" {
		return
	}
	r.artifactFiles[alias] = abs
}

func dataActionArtifactFilesFromSeed(artifacts []DataArtifact) map[string]string {
	out := map[string]string{}
	register := func(alias, abs string) {
		alias = normalizeMaterialPath(alias)
		abs = strings.TrimSpace(abs)
		if alias == "" || abs == "" {
			return
		}
		if info, err := os.Stat(abs); err != nil || info.IsDir() {
			return
		}
		out[alias] = abs
	}
	var walk func(DataArtifact)
	walk = func(artifact DataArtifact) {
		abs := ""
		if artifact.Fields != nil {
			abs = strings.TrimSpace(artifact.Fields["artifact_path"])
			for _, alias := range strings.Split(artifact.Fields["artifact_aliases"], ",") {
				register(alias, abs)
			}
		}
		for _, alias := range dataActionArtifactAliases(DataAction{ID: artifact.ID, OutputArtifact: artifact.ID}, artifact) {
			register(alias, abs)
		}
		for _, child := range artifact.Children {
			walk(child)
		}
	}
	for _, artifact := range artifacts {
		walk(artifact)
	}
	return out
}

func safeActionArtifactFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "artifact"
	}
	var b strings.Builder
	for _, ch := range name {
		switch {
		case ch >= 'a' && ch <= 'z':
			b.WriteRune(ch)
		case ch >= 'A' && ch <= 'Z':
			b.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		case ch == '-' || ch == '_' || ch == '.':
			b.WriteRune(ch)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= 120 {
			break
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "artifact"
	}
	return out
}

func materializedRecordPayloadFromArtifact(artifact DataArtifact) []map[string]any {
	var out []map[string]any
	for _, child := range artifact.Children {
		sourcePath := ""
		if len(child.SourcePaths) > 0 {
			sourcePath = normalizeMaterialPath(child.SourcePaths[0])
		}
		for i, sample := range child.Sample {
			sample = strings.TrimSpace(sample)
			if sample == "" {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(sample), &obj); err != nil {
				obj = map[string]any{"text": sample}
			}
			if sourcePath != "" {
				obj["_source_path"] = sourcePath
			}
			if _, ok := obj["_source_index"]; !ok {
				obj["_source_index"] = i + 1
			}
			if _, ok := obj["_source_locator"]; !ok && sourcePath != "" {
				obj["_source_locator"] = fmt.Sprintf("%s#%d", sourcePath, i+1)
			}
			out = append(out, obj)
		}
	}
	return out
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
		for _, key := range []string{"records", "rows", "contributions", "items", "data", "values"} {
			if item, ok := v[key]; ok {
				if arr, ok := item.([]any); ok {
					return flattenJSONRecords(arr)
				}
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
			var drafts []contributionFilterDraft
			if draftErr := json.Unmarshal([]byte(raw), &drafts); draftErr == nil {
				filters = make([]contributionFilter, 0, len(drafts))
				for i, draft := range drafts {
					value, err := normalizeContributionFilterValue(draft.Value)
					if err != nil {
						return nil, fmt.Errorf("parse filters_json[%d].value: %w", i, err)
					}
					filters = append(filters, contributionFilter{
						Field: draft.Field,
						Op:    draft.Op,
						Value: value,
					})
				}
			} else {
				return nil, fmt.Errorf("parse filters_json: %w", err)
			}
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

func normalizeContributionFilterValue(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return strings.TrimSpace(v), nil
	case float64, bool:
		return fmt.Sprint(v), nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			part, err := normalizeContributionFilterValue(item)
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(part) != "" {
				parts = append(parts, strings.TrimSpace(part))
			}
		}
		return strings.Join(parts, ","), nil
	default:
		return "", fmt.Errorf("unsupported filter value shape %T; use a scalar or array of scalars", value)
	}
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
	if r.artifactFiles != nil {
		if _, statErr := os.Stat(abs); statErr != nil {
			if artifactAbs, ok := r.artifactFiles[path]; ok {
				return artifactAbs, path, nil
			}
		}
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
	if err := r.validateCustomTransformContract(plan, script, inputs); err != nil {
		return Result{}, err
	}
	subPlan := plan
	subPlan.Actions = nil
	subPlan.Script = script
	subPlan.InputPaths = inputs
	subPlan.OutputContract = OutputContract{Format: OutputFreeform, ExplanationAllowed: true}
	subPlan.CoverageContract = coverageContractForActionInputs(plan.CoverageContract, inputs)
	runner := Runner{
		RepoRoot:        r.RepoRoot,
		TempRoot:        r.TempRoot,
		MaxFileBytes:    r.MaxFileBytes,
		MaxTotalBytes:   r.MaxTotalBytes,
		ExtraInputFiles: r.artifactFiles,
	}
	return runner.Run(ctx, subPlan)
}

func coverageContractForActionInputs(contract CoverageContract, inputs []string) CoverageContract {
	inputs = normalizeMaterialPaths(inputs)
	out := CoverageContract{}
	for _, material := range contract.RequiredMaterials {
		if coverageMaterialRunnerPathInInputs(material, inputs) {
			out.RequiredMaterials = append(out.RequiredMaterials, material)
		}
	}
	for _, material := range contract.OptionalMaterials {
		if coverageMaterialRunnerPathInInputs(material, inputs) {
			out.OptionalMaterials = append(out.OptionalMaterials, material)
		}
	}
	for _, material := range out.RequiredMaterials {
		if normalizeCoverageMaterialUseMode(material.UsageMode) == MaterialUsePlannerDistilled {
			out.ValidationRules = append(out.ValidationRules, cleanMaterialNotes(material.DistilledNotes)...)
		}
	}
	return out
}

func coverageMaterialRunnerPathInInputs(material CoverageMaterial, inputs []string) bool {
	mode := normalizeCoverageMaterialUseMode(material.UsageMode)
	var runnerPath string
	switch mode {
	case MaterialUseScriptConsumed:
		runnerPath = normalizeMaterialPath(material.Path)
	case MaterialUseTextEvidenceConsumed:
		runnerPath = normalizeMaterialPath(material.TextEvidencePath)
	case MaterialUsePlannerDistilled:
		return true
	default:
		return false
	}
	return runnerPath != "" && materialPathCovered(runnerPath, inputs)
}

func (r ActionRunner) validateCustomTransformContract(plan TaskPlan, script string, inputs []string) error {
	if err := r.validateCustomTransformRequiredDirectories(plan, inputs); err != nil {
		return err
	}
	if err := r.validateCustomTransformFieldReferences(script, inputs); err != nil {
		return err
	}
	return nil
}

func (r ActionRunner) validateCustomTransformRequiredDirectories(plan TaskPlan, inputs []string) error {
	if len(plan.CoverageContract.RequiredMaterials) == 0 {
		return nil
	}
	inputs = normalizeMaterialPaths(inputs)
	for _, material := range plan.CoverageContract.RequiredMaterials {
		if normalizeCoverageMaterialUseMode(material.UsageMode) != MaterialUseScriptConsumed {
			continue
		}
		req := normalizeMaterialPath(material.Path)
		if req == "" || !materialPathCovered(req, inputs) {
			continue
		}
		abs, _, err := r.resolveActionInputPath(req)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			continue
		}
		return fmt.Errorf("custom_transform material contract failed: required material %q is a directory with usage_mode=script_consumed; expand/profile concrete child files with typed actions, use text_evidence_consumed for extracted text, or use planner_distilled with distilled_notes before terminal computation", req)
	}
	return nil
}

type customTransformFieldRef struct {
	Path  string
	Field string
	Line  int
	Text  string
}

func (r ActionRunner) validateCustomTransformFieldReferences(script string, inputs []string) error {
	headersByPath := map[string][]string{}
	for _, input := range normalizeMaterialPaths(inputs) {
		abs, rel, err := r.resolveActionInputPath(input)
		if err != nil {
			continue
		}
		kind := dataKindForPath(abs)
		if kind != "csv" && kind != "tsv" {
			continue
		}
		headers, err := readDelimitedHeaders(abs, kind)
		if err != nil || len(headers) == 0 {
			continue
		}
		registerHeaderAliases(headersByPath, input, rel, headers)
	}
	if len(headersByPath) == 0 {
		return nil
	}
	refs := customTransformRowFieldRefs(script)
	if len(refs) == 0 {
		return nil
	}
	var missing []customTransformFieldRef
	for _, ref := range refs {
		headers := headersByPath[normalizeMaterialPath(ref.Path)]
		if len(headers) == 0 {
			continue
		}
		if !customTransformFieldExists(headers, ref.Field) {
			missing = append(missing, ref)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Slice(missing, func(i, j int) bool {
		if missing[i].Path != missing[j].Path {
			return missing[i].Path < missing[j].Path
		}
		if missing[i].Line != missing[j].Line {
			return missing[i].Line < missing[j].Line
		}
		return missing[i].Field < missing[j].Field
	})
	var parts []string
	seen := map[string]bool{}
	for _, ref := range missing {
		key := normalizeMaterialPath(ref.Path) + "\x00" + strings.ToLower(strings.TrimSpace(ref.Field)) + "\x00" + fmt.Sprint(ref.Line)
		if seen[key] {
			continue
		}
		seen[key] = true
		headers := headersByPath[normalizeMaterialPath(ref.Path)]
		parts = append(parts, fmt.Sprintf("%s line %d references missing field %q; available fields: %s",
			normalizeMaterialPath(ref.Path), ref.Line, ref.Field, strings.Join(headers, ", ")))
		if len(parts) >= 8 {
			break
		}
	}
	return fmt.Errorf("custom_transform field contract failed: %s", strings.Join(parts, " | "))
}

func customTransformFieldExists(headers []string, field string) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return true
	}
	lower := strings.ToLower(field)
	for _, h := range headers {
		if strings.ToLower(strings.TrimSpace(h)) == lower {
			return true
		}
	}
	return uniqueLooseFieldAlias(headers, field) != ""
}

func uniqueLooseFieldAlias(headers []string, field string) string {
	want := looseFieldAliasKey(field)
	if want == "" {
		return ""
	}
	var matched string
	for _, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" || looseFieldAliasKey(header) != want {
			continue
		}
		if matched != "" {
			return ""
		}
		matched = header
	}
	return matched
}

func looseFieldAliasKey(field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	if field == "" {
		return ""
	}
	field = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(field, "_")
	field = strings.Trim(field, "_")
	for _, suffix := range []string{"_raw", "_value", "_text"} {
		if strings.HasSuffix(field, suffix) {
			field = strings.TrimSuffix(field, suffix)
			break
		}
	}
	return strings.ReplaceAll(field, "_", "")
}

func registerHeaderAliases(out map[string][]string, input, rel string, headers []string) {
	for _, alias := range []string{
		input,
		rel,
		path.Base(input),
		path.Base(rel),
		"./" + input,
		"./" + rel,
	} {
		alias = normalizeMaterialPath(alias)
		if alias == "" {
			continue
		}
		if _, ok := out[alias]; !ok {
			out[alias] = headers
		}
	}
}

func readDelimitedHeaders(abs, kind string) ([]string, error) {
	file, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	if kind == "tsv" {
		reader.Comma = '\t'
	}
	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}
	return cleanStringSlice(headers), nil
}

var (
	customRowsAssignRE = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:list\s*\(\s*)?(csv_rows|tsv_rows)\s*\(\s*['"]([^'"]+)['"]\s*\)\s*\)?\s*$`)
	customForRowsRE    = regexp.MustCompile(`^\s*for\s+([A-Za-z_][A-Za-z0-9_]*)\s+in\s+(?:list\s*\(\s*)?(csv_rows|tsv_rows)\s*\(\s*['"]([^'"]+)['"]\s*\)\s*\)?\s*:`)
	customForVarRE     = regexp.MustCompile(`^\s*for\s+([A-Za-z_][A-Za-z0-9_]*)\s+in\s+([A-Za-z_][A-Za-z0-9_]*)\s*:`)
)

func customTransformRowFieldRefs(script string) []customTransformFieldRef {
	lines := strings.Split(strings.ReplaceAll(script, "\r\n", "\n"), "\n")
	collections := map[string]string{}
	rowVars := map[string]string{}
	var refs []customTransformFieldRef
	for i, line := range lines {
		lineNo := i + 1
		if m := customRowsAssignRE.FindStringSubmatch(line); len(m) == 4 {
			collections[m[1]] = normalizeMaterialPath(m[3])
		}
		if m := customForRowsRE.FindStringSubmatch(line); len(m) == 4 {
			rowVars[m[1]] = normalizeMaterialPath(m[3])
		} else if m := customForVarRE.FindStringSubmatch(line); len(m) == 3 {
			if path := collections[m[2]]; path != "" {
				rowVars[m[1]] = path
			}
		}
		for rowVar, sourcePath := range rowVars {
			for _, field := range rowFieldRefsForVar(line, rowVar) {
				refs = append(refs, customTransformFieldRef{
					Path:  sourcePath,
					Field: field,
					Line:  lineNo,
					Text:  strings.TrimSpace(line),
				})
			}
		}
		for collectionVar, sourcePath := range collections {
			for _, field := range rowFieldRefsForCollection(line, collectionVar) {
				refs = append(refs, customTransformFieldRef{
					Path:  sourcePath,
					Field: field,
					Line:  lineNo,
					Text:  strings.TrimSpace(line),
				})
			}
		}
	}
	return refs
}

func rowFieldRefsForVar(line, rowVar string) []string {
	if strings.TrimSpace(rowVar) == "" {
		return nil
	}
	quotedVar := regexp.QuoteMeta(rowVar)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\b` + quotedVar + `\s*\[\s*['"]([^'"]+)['"]\s*\]`),
		regexp.MustCompile(`\b` + quotedVar + `\s*\.get\s*\(\s*['"]([^'"]+)['"]`),
	}
	var out []string
	seen := map[string]bool{}
	for _, re := range patterns {
		for _, m := range re.FindAllStringSubmatch(line, -1) {
			if len(m) < 2 {
				continue
			}
			field := strings.TrimSpace(m[1])
			if field == "" || seen[field] {
				continue
			}
			seen[field] = true
			out = append(out, field)
		}
	}
	return out
}

func rowFieldRefsForCollection(line, collectionVar string) []string {
	if strings.TrimSpace(collectionVar) == "" {
		return nil
	}
	quotedVar := regexp.QuoteMeta(collectionVar)
	re := regexp.MustCompile(`\b` + quotedVar + `\s*\[[^\]]+\]\s*\[\s*['"]([^'"]+)['"]\s*\]`)
	var out []string
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(line, -1) {
		if len(m) < 2 {
			continue
		}
		field := strings.TrimSpace(m[1])
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
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
