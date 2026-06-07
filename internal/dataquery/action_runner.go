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

// ReferenceKeyCandidate describes a record field that can define the complete
// output key universe for a final projection. It is structural metadata only:
// callers must still decide whether the user's output contract requires using
// the candidate.
type ReferenceKeyCandidate struct {
	Path               string   `json:"path,omitempty"`
	Field              string   `json:"field,omitempty"`
	KeyCount           int      `json:"key_count,omitempty"`
	ExistingMatchCount int      `json:"existing_match_count,omitempty"`
	MissingCount       int      `json:"missing_count,omitempty"`
	Keys               []string `json:"keys,omitempty"`
	MissingKeys        []string `json:"missing_keys,omitempty"`
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
	seedArtifacts := stripWorkflowLedgerArtifacts(r.Seed.Artifacts)
	r.artifactFiles = dataActionArtifactFilesFromSeed(seedArtifacts)
	artifacts := append([]DataArtifact(nil), seedArtifacts...)
	consumed := append([]string(nil), r.Seed.ConsumedPaths...)
	var summaries []string
	rows := append([]RowDecision(nil), r.Seed.Rows...)
	ruleCoverage := DedupeRuleCoverageRecords(append([]RuleCoverageRecord(nil), r.Seed.RuleCoverage...))
	contributions := DedupeContributionRecords(append([]ContributionRecord(nil), r.Seed.Contributions...))
	if len(rows) == 0 && len(contributions) > 0 {
		rows = append(rows, rowDecisionsFromContributions(contributions)...)
	}
	entityResolutions := DedupeEntityResolutionRecords(append([]EntityResolutionRecord(nil), r.Seed.EntityResolutions...))
	if seedLedgerArtifacts, err := r.materializeWorkflowLedgerArtifacts(artifactDir, rows, ruleCoverage, contributions, entityResolutions); err != nil {
		return Result{}, err
	} else {
		artifacts = append(artifacts, seedLedgerArtifacts...)
	}
	var reconcile *ReconcileReport
	if r.Seed.Reconcile != nil {
		seedReconcile := *r.Seed.Reconcile
		reconcile = &seedReconcile
	}
	var lastResult *Result
	var projectedAnswer string
	partialResult := func() Result {
		outputContract := plan.OutputContract
		if outputContract.Format == "" {
			outputContract = OutputContract{Format: OutputFreeform, ExplanationAllowed: true}
		}
		return Result{
			OutputContract:    outputContract.Normalize(),
			AuditSummary:      strings.Join(cleanArtifactSummaries(summaries), "; "),
			Rows:              rows,
			Artifacts:         artifacts,
			RuleCoverage:      DedupeRuleCoverageRecords(ruleCoverage),
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
			ruleCoverage = DedupeRuleCoverageRecords(append(ruleCoverage, records...))
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
		case DataActionExpandRecords:
			artifact, records, paths, err := r.runExpandRecords(action)
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
		case DataActionFilterRecords:
			artifact, records, paths, err := r.runFilterRecords(action)
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
		case DataActionQualifyRecords:
			defaultRuleRefs := []string(nil)
			if plan.CoverageContract.RuleCoverageRequired {
				defaultRuleRefs = ruleCoverageIDs(ruleCoverage)
			}
			artifact, records, decisions, paths, err := r.runQualifyRecords(action, defaultRuleRefs)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, records)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			rows = append(rows, decisions...)
			consumed = append(consumed, paths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionNormalizeEntities:
			requireNonEmpty := plan.CoverageContract.EntityResolutionRequired && len(entityResolutions) == 0
			artifact, records, err := r.runNormalizeEntities(action, requireNonEmpty)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, records)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			entityResolutions = DedupeEntityResolutionRecords(append(entityResolutions, records...))
			consumed = append(consumed, artifact.SourcePaths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionApplyResolutions:
			artifact, records, paths, err := r.runApplyEntityResolutions(action)
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
		case DataActionEnrichRecords:
			artifact, records, resolutions, paths, err := r.runEnrichRecords(action)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, records)
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			entityResolutions = DedupeEntityResolutionRecords(append(entityResolutions, resolutions...))
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
			requireNonEmpty := plan.CoverageContract.ContributionLedgerRequired
			artifact, records, paths, err := r.runComputeContributions(action, defaultRuleRefs, requireNonEmpty)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, contributionActionArtifactPayload(artifact, records))
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			contributions = DedupeContributionRecords(append(contributions, records...))
			rows = append(rows, rowDecisionsFromContributions(records)...)
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
		case DataActionAssembleAnswer:
			artifact, answer, report, err := r.runAssembleAnswer(action, reconcile, plan.OutputContract, artifacts)
			if err != nil {
				return failAction(action, err)
			}
			artifact, err = r.materializeActionArtifact(artifactDir, action, artifact, map[string]any{
				"answer":    answer,
				"reconcile": report,
			})
			if err != nil {
				return failAction(action, err)
			}
			artifacts = append(artifacts, artifact)
			reconcile = report
			projectedAnswer = answer
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
		out.Rows = append(out.Rows, rows...)
		out.RuleCoverage = DedupeRuleCoverageRecords(append(out.RuleCoverage, ruleCoverage...))
		out.Contributions = append(out.Contributions, contributions...)
		out.EntityResolutions = DedupeEntityResolutionRecords(append(out.EntityResolutions, entityResolutions...))
		if out.Reconcile == nil && reconcile != nil {
			out.Reconcile = reconcile
		}
		if strings.TrimSpace(out.AuditSummary) == "" {
			out.AuditSummary = strings.Join(cleanArtifactSummaries(summaries), "; ")
		}
		return validateRunnerResult(actionRunnerValidationPlan(plan, r.Seed), out)
	}
	answer := projectedAnswer
	if strings.TrimSpace(answer) == "" {
		answer = renderArtifactsAnswer(artifacts, plan.OutputContract, reconcile)
	}
	outputContract := plan.OutputContract
	if strings.TrimSpace(r.Seed.Answer) != "" && lastResult == nil && strings.TrimSpace(projectedAnswer) == "" && !planHasActionKind(plan, DataActionAssembleAnswer) {
		// A seed answer is a carry-over fallback, not a stronger signal than
		// the current batch. In multi-batch data workflows an early material
		// inventory/custom-transform summary may live in the seed while a later
		// reconcile batch can deterministically render a stricter answer.
		if strings.TrimSpace(answer) == "" || AnswerLooksLikeArtifactSummary(answer) {
			answer = r.Seed.Answer
			if r.Seed.OutputContract.Format != "" {
				outputContract = r.Seed.OutputContract
			}
		}
	}
	out := Result{
		Answer:            answer,
		OutputContract:    outputContract.Normalize(),
		AuditSummary:      strings.Join(cleanArtifactSummaries(summaries), "; "),
		Rows:              rows,
		Artifacts:         artifacts,
		RuleCoverage:      DedupeRuleCoverageRecords(ruleCoverage),
		Contributions:     contributions,
		EntityResolutions: entityResolutions,
		Reconcile:         reconcile,
		ConsumedPaths:     normalizeMaterialPaths(consumed),
	}
	if out.OutputContract.Format == "" {
		out.OutputContract = OutputContract{Format: OutputMarkdown, ExplanationAllowed: true}.Normalize()
	}
	return validateRunnerResult(actionRunnerValidationPlan(plan, r.Seed), out)
}

func actionRunnerValidationPlan(plan TaskPlan, seed Result) TaskPlan {
	if !plan.ContinueAfter && !actionPlanCannotSatisfyFullCoverageInOneBatch(plan, seed) {
		return plan
	}
	out := plan
	out.OutputContract = OutputContract{Format: OutputFreeform, ExplanationAllowed: true}
	out.CoverageContract = actionRunnerIntermediateCoverageContract(plan)
	return out
}

func actionPlanCannotSatisfyFullCoverageInOneBatch(plan TaskPlan, seed Result) bool {
	if len(plan.Actions) == 0 {
		return false
	}
	contract := plan.CoverageContract
	if !actionPlanRequiredMaterialsCovered(plan, seed) {
		return false
	}
	if contract.RuleCoverageRequired && !planHasAnyActionKind(plan, DataActionDeriveRules, DataActionCustomTransform) {
		return true
	}
	if contract.DecisionRecordsRequired && !planHasAnyActionKind(plan, DataActionQualifyRecords, DataActionComputeContribs, DataActionCustomTransform) {
		return true
	}
	if contract.ContributionLedgerRequired && !planHasAnyActionKind(plan, DataActionComputeContribs, DataActionCustomTransform) {
		return true
	}
	if contract.EntityResolutionRequired && !planHasAnyActionKind(plan, DataActionNormalizeEntities, DataActionEnrichRecords, DataActionCustomTransform) {
		return true
	}
	if contract.ReconcileRequired && !planHasAnyActionKind(plan, DataActionReconcile, DataActionCustomTransform) {
		return true
	}
	return false
}

func actionPlanRequiredMaterialsCovered(plan TaskPlan, seed Result) bool {
	covered := append([]string(nil), seed.ConsumedPaths...)
	for _, action := range plan.Actions {
		if dataActionInputsCanSatisfyCoverage(action.Kind) {
			covered = append(covered, action.InputPaths...)
		}
	}
	covered = normalizeMaterialPaths(covered)
	for _, material := range plan.CoverageContract.RequiredMaterials {
		if !coverageMaterialRunnerPathInInputs(material, covered) {
			return false
		}
	}
	return true
}

func actionRunnerIntermediateCoverageContract(plan TaskPlan) CoverageContract {
	var inputs []string
	for _, action := range plan.Actions {
		if dataActionInputsCanSatisfyCoverage(action.Kind) {
			inputs = append(inputs, action.InputPaths...)
		}
	}
	out := coverageContractForActionInputs(plan.CoverageContract, inputs)
	for _, action := range plan.Actions {
		switch normalizeDataActionKind(action.Kind) {
		case DataActionDeriveRules:
			out.RuleCoverageRequired = out.RuleCoverageRequired || plan.CoverageContract.RuleCoverageRequired
		case DataActionNormalizeEntities:
			out.EntityResolutionRequired = out.EntityResolutionRequired || plan.CoverageContract.EntityResolutionRequired
		case DataActionQualifyRecords:
			out.DecisionRecordsRequired = out.DecisionRecordsRequired || plan.CoverageContract.DecisionRecordsRequired
		case DataActionComputeContribs:
			out.ContributionLedgerRequired = out.ContributionLedgerRequired || plan.CoverageContract.ContributionLedgerRequired
		case DataActionReconcile:
			out.ReconcileRequired = out.ReconcileRequired || plan.CoverageContract.ReconcileRequired
		case DataActionAssembleAnswer:
			out.ReconcileRequired = out.ReconcileRequired || plan.CoverageContract.ReconcileRequired
		}
	}
	return out
}

func dataActionInputsCanSatisfyCoverage(kind DataActionKind) bool {
	switch normalizeDataActionKind(kind) {
	case DataActionMaterialInventory:
		return false
	default:
		return true
	}
}

func planHasActionKind(plan TaskPlan, kind DataActionKind) bool {
	return planHasAnyActionKind(plan, kind)
}

func planHasAnyActionKind(plan TaskPlan, kinds ...DataActionKind) bool {
	for _, action := range plan.Actions {
		got := normalizeDataActionKind(action.Kind)
		for _, kind := range kinds {
			if got == kind {
				return true
			}
		}
	}
	return false
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
	case DataActionExpandRecords:
		return DataActionExpandRecords
	case DataActionFilterRecords:
		return DataActionFilterRecords
	case DataActionQualifyRecords:
		return DataActionQualifyRecords
	case DataActionNormalizeEntities:
		return DataActionNormalizeEntities
	case DataActionApplyResolutions:
		return DataActionApplyResolutions
	case DataActionEnrichRecords:
		return DataActionEnrichRecords
	case DataActionJoinRecords:
		return DataActionJoinRecords
	case DataActionComputeContribs:
		return DataActionComputeContribs
	case DataActionReconcile:
		return DataActionReconcile
	case DataActionAssembleAnswer:
		return DataActionAssembleAnswer
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
	paths := cleanStringListPreserveOrder(action.InputPaths)
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
	paths := cleanStringListPreserveOrder(action.InputPaths)
	if len(paths) == 0 {
		return DataArtifact{}, errors.New("extract_records action has no input_paths")
	}
	limit := actionIntParamAny(action, []string{"limit", "max_records"}, 20, 1, 1000000)
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
	paths := cleanStringListPreserveOrder(action.InputPaths)
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
	if raw := strings.TrimSpace(firstNonEmptyString(action.Params["rules_json"], structuredActionParam(action.Params, "rules"))); raw != "" {
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

func (r ActionRunner) runNormalizeEntities(action DataAction, requireNonEmpty bool) (DataArtifact, []EntityResolutionRecord, error) {
	raw := strings.TrimSpace(firstNonEmptyString(action.Params["resolutions_json"], structuredActionParam(action.Params, "resolutions")))
	if raw == "" {
		raw = strings.TrimSpace(firstNonEmptyString(action.Params["mappings_json"], structuredActionParam(action.Params, "mappings")))
	}
	if raw == "" {
		raw = strings.TrimSpace(firstNonEmptyString(action.Params["entity_resolutions_json"], structuredActionParam(action.Params, "entity_resolutions")))
	}
	var records []EntityResolutionRecord
	var consumed []string
	var children []DataArtifact
	if raw != "" {
		var err error
		records, err = parseExplicitEntityResolutionRecords(raw)
		if err != nil {
			return DataArtifact{}, nil, fmt.Errorf("parse normalize_entities resolutions_json: %w", err)
		}
	}
	if raw == "" || (len(records) == 0 && len(cleanStringList(action.InputPaths)) > 0) {
		var err error
		records, consumed, children, err = r.deriveEntityResolutionsFromInputs(action)
		if err != nil {
			return DataArtifact{}, nil, err
		}
	}
	applyEntityResolutionDefaults(records, action)
	if raw != "" {
		if err := r.validateExplicitEntityResolutionCanonicalUniverse(action, records); err != nil {
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
		artifact := DataArtifact{
			ID:          id,
			Kind:        string(DataActionNormalizeEntities),
			SourcePaths: normalizeMaterialPaths(consumed),
			Summary:     "normalized 0 entity value(s)",
			Fields:      map[string]string{"count": "0"},
			Children:    children,
		}
		if requireNonEmpty {
			return artifact, nil, fmt.Errorf("normalize_entities produced zero entity resolution records while entity resolution ledger is required; diagnostics=%s. Check source/reference input paths, source_fields, reference_fields, canonical_id_field, match_mode, and role-specific source_filters/reference_filters against available fields and samples",
				entityNormalizationDiagnosticsJSON(artifact))
		}
		return artifact, nil, nil
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

func parseExplicitEntityResolutionRecords(raw string) ([]EntityResolutionRecord, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var items []json.RawMessage
	if strings.HasPrefix(raw, "{") {
		items = []json.RawMessage{json.RawMessage(raw)}
	} else {
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			return nil, err
		}
	}
	out := make([]EntityResolutionRecord, 0, len(items))
	for _, item := range items {
		var rec EntityResolutionRecord
		if err := json.Unmarshal(item, &rec); err != nil {
			return nil, err
		}
		var rawMap map[string]json.RawMessage
		if err := json.Unmarshal(item, &rawMap); err == nil && strings.TrimSpace(rec.SourceValue.String()) == "" {
			sources := rawAliasStringSlice(rawMap, "source_values", "source_value_list", "sources")
			if len(sources) > 0 {
				for _, source := range sources {
					source = strings.TrimSpace(source)
					if source == "" {
						continue
					}
					expanded := rec
					expanded.SourceValue = LooseText(source)
					out = append(out, expanded)
				}
				continue
			}
		}
		out = append(out, rec)
	}
	return out, nil
}

func entityNormalizationDiagnosticsJSON(artifact DataArtifact) string {
	type childDiag struct {
		ID      string            `json:"id,omitempty"`
		Kind    string            `json:"kind,omitempty"`
		Headers []string          `json:"headers,omitempty"`
		Fields  map[string]string `json:"fields,omitempty"`
		Summary string            `json:"summary,omitempty"`
	}
	out := struct {
		ID       string            `json:"id,omitempty"`
		Fields   map[string]string `json:"fields,omitempty"`
		Children []childDiag       `json:"children,omitempty"`
	}{
		ID:     artifact.ID,
		Fields: artifact.Fields,
	}
	for _, child := range artifact.Children {
		out.Children = append(out.Children, childDiag{
			ID:      child.ID,
			Kind:    child.Kind,
			Headers: clampStringSlice(child.Headers, 32),
			Fields:  child.Fields,
			Summary: child.Summary,
		})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return clampViolationText(string(raw), 2400)
}

func (r ActionRunner) validateExplicitEntityResolutionCanonicalUniverse(action DataAction, records []EntityResolutionRecord) error {
	if len(records) == 0 {
		return nil
	}
	referencePath := firstNonEmptyString(
		strings.TrimSpace(action.Params["reference_path"]),
		strings.TrimSpace(action.Params["lookup_path"]),
		strings.TrimSpace(action.Params["mapping_path"]),
	)
	if referencePath == "" {
		return nil
	}
	candidateFields := cleanStringSlice(append(
		[]string{
			strings.TrimSpace(action.Params["canonical_id_field"]),
			strings.TrimSpace(action.Params["id_field"]),
			strings.TrimSpace(action.Params["lookup_value_field"]),
			strings.TrimSpace(action.Params["mapping_value_field"]),
		},
		append(parseActionStringListParam(action.Params["reference_fields"]), parseActionStringListParam(action.Params["reference_field"])...)...,
	))
	if len(candidateFields) == 0 {
		return nil
	}
	maxRecords := actionIntParam(action, "max_reference_records", 100000, 1, 1000000)
	refRecords, refHeaders, _, refRel, err := r.readActionRecords(referencePath, maxRecords)
	if err != nil {
		return fmt.Errorf("normalize_entities reference_path %q could not be read for canonical universe validation: %w", referencePath, err)
	}
	allowed := map[string]bool{}
	var usedFields []string
	for _, field := range candidateFields {
		if !actionRecordFieldExistsInRecords(refHeaders, refRecords, field) {
			continue
		}
		usedFields = append(usedFields, field)
		for _, rec := range refRecords {
			value := strings.TrimSpace(recordField(rec.Fields, field))
			if value != "" {
				allowed[value] = true
			}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	var invalid []string
	seenInvalid := map[string]bool{}
	for _, rec := range records {
		status := strings.ToLower(strings.TrimSpace(rec.Status.String()))
		if status == "unresolved" || status == "missing" || status == "not_applicable" {
			continue
		}
		id := strings.TrimSpace(rec.CanonicalID.String())
		if id == "" || allowed[id] {
			continue
		}
		if !seenInvalid[id] {
			seenInvalid[id] = true
			invalid = append(invalid, id)
		}
	}
	if len(invalid) == 0 {
		return nil
	}
	allowedValues := make([]string, 0, len(allowed))
	for value := range allowed {
		allowedValues = append(allowedValues, value)
	}
	sort.Strings(invalid)
	sort.Strings(allowedValues)
	return fmt.Errorf("normalize_entities explicit canonical_id value(s) [%s] are outside reference universe %s field(s) [%s]; use canonical ids from the reference table, or omit explicit resolutions and let structured normalization derive them. allowed_sample=[%s]",
		strings.Join(clampStringSliceForError(invalid, 12), ", "),
		refRel,
		strings.Join(usedFields, ", "),
		strings.Join(clampStringSliceForError(allowedValues, 24), ", "),
	)
}

func applyEntityResolutionDefaults(records []EntityResolutionRecord, action DataAction) {
	defaultStatus := firstNonEmptyString(strings.TrimSpace(action.Params["default_status"]), "resolved")
	reason := firstNonEmptyString(strings.TrimSpace(action.Params["reason"]), strings.TrimSpace(action.Purpose))
	for i := range records {
		if strings.TrimSpace(records[i].Status.String()) == "" {
			records[i].Status = LooseText(defaultStatus)
		}
		if strings.TrimSpace(records[i].Reason.String()) == "" && reason != "" {
			records[i].Reason = LooseText(reason)
		}
	}
}

func (r ActionRunner) deriveEntityResolutionsFromInputs(action DataAction) ([]EntityResolutionRecord, []string, []DataArtifact, error) {
	paths := cleanStringListPreserveOrder(action.InputPaths)
	if len(paths) == 0 {
		return nil, nil, nil, errors.New("normalize_entities action requires either params.resolutions_json or structured input_paths")
	}
	if records, consumed, children, ok, err := r.deriveEntityResolutionsFromReferenceInputs(action, paths); ok || err != nil {
		return records, consumed, children, err
	}
	if actionDeclaresReferenceEntityMode(action, paths) {
		return nil, nil, nil, fmt.Errorf("normalize_entities declared source/reference mapping inputs but reference mode did not activate; provide source_path/reference_path or input_paths=[source_records, reference_records], source_field/source_fields, reference_name_fields/reference_fields, and canonical_id_field")
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
		effectiveFilters, ignoredFilters := effectiveActionFiltersForRecords(filters, headers, records)
		matched := 0
		for _, record := range records {
			if !recordPassesFilters(record.Fields, effectiveFilters) {
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
		fields := map[string]string{
			"canonical_id_field":    pathCanonicalIDField,
			"canonical_label_field": pathCanonicalLabelField,
			"inferred_fields":       strings.Join(sourceFields, ","),
			"inferred_schema":       fmt.Sprintf("%t", inferred),
			"matched":               fmt.Sprintf("%d", matched),
			"total":                 fmt.Sprintf("%d", total),
		}
		if len(ignoredFilters) > 0 {
			fields["ignored_filter_fields"] = strings.Join(ignoredFilters, ",")
		}
		children = append(children, DataArtifact{
			ID:          rel + "#entity_resolutions",
			Kind:        "entity_resolution_source",
			SourcePaths: []string{rel},
			Headers:     headers,
			RowCount:    total,
			Summary:     fmt.Sprintf("%s produced %d entity resolution(s) from %d record(s)", rel, matched, total),
			Fields:      fields,
		})
	}
	return out, normalizeMaterialPaths(consumed), children, nil
}

func actionDeclaresReferenceEntityMode(action DataAction, paths []string) bool {
	hasTwoInputs := len(cleanStringList(paths)) >= 2
	for _, key := range []string{
		"source_path", "base_path", "record_path", "source_field", "source_fields", "name_field", "name_fields",
		"canonical_id_field", "canonical_label_field", "id_field", "label_field",
		"reference_path", "lookup_path", "mapping_path",
		"reference_field", "reference_fields", "reference_name_field", "reference_name_fields",
		"mapping_source_field", "mapping_source_fields", "lookup_field", "lookup_fields",
		"reference_fields_json", "reference_name_fields_json",
	} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return hasTwoInputs || strings.Contains(key, "reference") || strings.Contains(key, "lookup") || strings.Contains(key, "mapping")
		}
	}
	return false
}

type entityReferenceCandidate struct {
	Source   string
	ID       string
	Label    string
	Evidence string
	Weight   int
}

func (r ActionRunner) deriveEntityResolutionsFromReferenceInputs(action DataAction, paths []string) ([]EntityResolutionRecord, []string, []DataArtifact, bool, error) {
	if len(paths) < 2 {
		return nil, nil, nil, false, nil
	}
	sourceFields := normalizeEntitySourceFields(action)
	if len(sourceFields) == 0 {
		return nil, nil, nil, false, nil
	}
	explicitSourcePath := firstNonEmptyString(
		strings.TrimSpace(action.Params["source_path"]),
		strings.TrimSpace(action.Params["base_path"]),
		strings.TrimSpace(action.Params["record_path"]),
	)
	sourcePath := firstNonEmptyString(
		explicitSourcePath,
		paths[0],
	)
	explicitReferencePath := firstNonEmptyString(
		strings.TrimSpace(action.Params["reference_path"]),
		strings.TrimSpace(action.Params["mapping_path"]),
		strings.TrimSpace(action.Params["lookup_path"]),
	)
	defaultReferencePath := ""
	if explicitSourcePath != "" {
		defaultReferencePath = firstNonBaseActionPath(paths, explicitSourcePath)
	}
	if defaultReferencePath == "" && len(paths) > 1 {
		defaultReferencePath = paths[1]
	}
	referencePath := firstNonEmptyString(
		explicitReferencePath,
		defaultReferencePath,
	)
	if normalizeMaterialPath(sourcePath) == "" || normalizeMaterialPath(referencePath) == "" || normalizeMaterialPath(sourcePath) == normalizeMaterialPath(referencePath) {
		return nil, nil, nil, false, nil
	}
	canonicalIDField := firstNonEmptyString(
		strings.TrimSpace(action.Params["canonical_id_field"]),
		strings.TrimSpace(action.Params["id_field"]),
		strings.TrimSpace(action.Params["reference_value_field"]),
		strings.TrimSpace(action.Params["lookup_value_field"]),
	)
	canonicalLabelField := firstNonEmptyString(
		strings.TrimSpace(action.Params["canonical_label_field"]),
		strings.TrimSpace(action.Params["label_field"]),
	)
	referenceFields := normalizeEntityReferenceFields(action)
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxResolutions := actionIntParam(action, "max_resolutions", 200000, 1, 500000)

	sourceRecords, sourceHeaders, sourceTotal, sourceRel, err := r.readActionRecords(sourcePath, maxRecords)
	if err != nil {
		return nil, nil, nil, true, err
	}
	referenceRecords, referenceHeaders, referenceTotal, referenceRel, err := r.readActionRecords(referencePath, maxRecords)
	if err != nil {
		return nil, nil, nil, true, err
	}
	sourceFieldNames := actionRecordFieldNames(sourceHeaders, sourceRecords)
	referenceFieldNames := actionRecordFieldNames(referenceHeaders, referenceRecords)
	roleInference := ""
	if explicitSourcePath == "" && explicitReferencePath == "" {
		if swap, reason := shouldSwapEntityReferenceInputs(sourceFieldNames, referenceFieldNames, sourceFields, referenceFields, canonicalIDField); swap {
			roleInference = reason
			sourceRecords, referenceRecords = referenceRecords, sourceRecords
			sourceHeaders, referenceHeaders = referenceHeaders, sourceHeaders
			sourceTotal, referenceTotal = referenceTotal, sourceTotal
			sourceRel, referenceRel = referenceRel, sourceRel
			sourceFieldNames, referenceFieldNames = referenceFieldNames, sourceFieldNames
		}
	}
	if canonicalIDField == "" {
		canonicalIDField = inferEntityCanonicalIDField(referenceHeaders)
	}
	if canonicalLabelField == "" {
		canonicalLabelField = inferEntityCanonicalLabelField(referenceHeaders, canonicalIDField)
	}
	if len(referenceFields) == 0 {
		referenceFields = inferEnrichMappingSourceFields(referenceHeaders, canonicalIDField, canonicalLabelField)
	}
	if len(referenceFields) == 0 {
		return nil, nil, nil, true, fmt.Errorf("normalize_entities reference mode could not infer reference fields for %s; provide params.reference_fields or reference_name_fields", referenceRel)
	}
	if !actionRecordAnyFieldExists(sourceFieldNames, sourceFields) {
		return nil, nil, nil, true, fmt.Errorf("normalize_entities source field(s) [%s] were not found in source input %s fields [%s]", strings.Join(sourceFields, ", "), sourceRel, strings.Join(clampStringSliceForError(sourceFieldNames, 24), ", "))
	}
	if !actionRecordFieldExists(referenceFieldNames, canonicalIDField) {
		return nil, nil, nil, true, fmt.Errorf("normalize_entities canonical_id_field %q was not found in reference input %s fields [%s]", canonicalIDField, referenceRel, strings.Join(clampStringSliceForError(referenceFieldNames, 24), ", "))
	}

	matchMode := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action.Params["match_mode"], action.Params["mode"], "exact")))
	genericFilters, err := parseContributionFilters(action)
	if err != nil {
		return nil, nil, nil, true, err
	}
	explicitSourceFilters, err := parseActionFilterListParams(action, "source_filters", "base_filters", "record_filters")
	if err != nil {
		return nil, nil, nil, true, err
	}
	explicitReferenceFilters, err := parseActionFilterListParams(action, "reference_filters", "lookup_filters", "mapping_filters")
	if err != nil {
		return nil, nil, nil, true, err
	}
	sourceFilters, referenceFilters, filterNotes := splitEntityReferenceFilters(genericFilters, explicitSourceFilters, explicitReferenceFilters, sourceHeaders, sourceRecords, referenceHeaders, referenceRecords)
	effectiveReferenceFilters, ignoredReferenceFilters := effectiveActionFiltersForRecords(referenceFilters, referenceHeaders, referenceRecords)
	referenceRecords = filterActionRecords(referenceRecords, effectiveReferenceFilters)
	referenceLookup := buildEntityReferenceLookup(referenceRecords, referenceRel, referenceFields, canonicalIDField, canonicalLabelField)
	defaultStatus := firstNonEmptyString(strings.TrimSpace(action.Params["default_status"]), "resolved")
	reason := firstNonEmptyString(strings.TrimSpace(action.Params["reason"]), strings.TrimSpace(action.Purpose), "normalized by reference records")
	ruleRefs := parseActionStringListParam(action.Params["rule_refs"])
	effectiveFilters, ignoredFilters := effectiveActionFiltersForRecords(sourceFilters, sourceHeaders, sourceRecords)

	var out []EntityResolutionRecord
	seen := map[string]bool{}
	for _, record := range sourceRecords {
		if !recordPassesFilters(record.Fields, effectiveFilters) {
			continue
		}
		for _, field := range sourceFields {
			sourceValue := recordField(record.Fields, field)
			if sourceValue == "" {
				continue
			}
			if len(out) >= maxResolutions {
				return nil, nil, nil, true, fmt.Errorf("normalize_entities exceeded max_resolutions=%d; split the action into smaller filters or input groups", maxResolutions)
			}
			candidate, status, evidence := selectEntityReferenceCandidate(sourceValue, referenceLookup, matchMode)
			canonicalID := candidate.ID
			canonicalLabel := firstNonEmptyString(candidate.Label, candidate.ID)
			if status == "" {
				status = defaultStatus
			}
			itemID := fmt.Sprintf("%s#%d:%s", sourceRel, record.Index, field)
			key := sourceRel + "\x00" + field + "\x00" + sourceValue + "\x00" + canonicalID + "\x00" + canonicalLabel + "\x00" + status
			if seen[key] {
				continue
			}
			seen[key] = true
			evidenceRefs := []string{fmt.Sprintf("%s:%d", sourceRel, record.Line)}
			if strings.TrimSpace(evidence) != "" {
				evidenceRefs = append(evidenceRefs, evidence)
			}
			out = append(out, EntityResolutionRecord{
				ItemID:         LooseText(itemID),
				SourceValue:    LooseText(sourceValue),
				CanonicalID:    LooseText(canonicalID),
				CanonicalLabel: LooseText(canonicalLabel),
				Status:         LooseText(status),
				EvidenceRefs:   cleanStringList(evidenceRefs),
				RuleRefs:       append([]string(nil), ruleRefs...),
				Reason:         LooseText(reason),
			})
		}
	}
	children := []DataArtifact{
		{
			ID:          sourceRel + "#entity_source",
			Kind:        "entity_resolution/source",
			SourcePaths: []string{sourceRel},
			Headers:     sourceHeaders,
			RowCount:    sourceTotal,
			Summary:     fmt.Sprintf("%s supplied %d source record(s) for entity normalization", sourceRel, sourceTotal),
			Fields: map[string]string{
				"source_fields":      strings.Join(sourceFields, ","),
				"matched":            fmt.Sprintf("%d", len(out)),
				"source_filters":     filterDiagnosticsJSON(sourceRecords, effectiveFilters, 8, 8),
				"filter_role_notes":  strings.Join(filterNotes, "; "),
				"filtered_source":    fmt.Sprintf("%d", countRecordsPassingFilters(sourceRecords, effectiveFilters)),
				"source_total":       fmt.Sprintf("%d", sourceTotal),
				"reference_filtered": fmt.Sprintf("%d", len(referenceRecords)),
			},
		},
		{
			ID:          referenceRel + "#entity_reference",
			Kind:        "entity_resolution/reference",
			SourcePaths: []string{referenceRel},
			Headers:     referenceHeaders,
			RowCount:    referenceTotal,
			Summary:     fmt.Sprintf("%s supplied %d reference record(s) for entity normalization", referenceRel, referenceTotal),
			Fields: map[string]string{
				"reference_fields":      strings.Join(referenceFields, ","),
				"canonical_id_field":    canonicalIDField,
				"canonical_label_field": canonicalLabelField,
				"match_mode":            matchMode,
				"reference_filters":     filterDiagnosticsJSON(referenceRecords, effectiveReferenceFilters, 8, 8),
				"reference_total":       fmt.Sprintf("%d", referenceTotal),
			},
		},
	}
	if len(ignoredFilters) > 0 {
		children[0].Fields["ignored_filter_fields"] = strings.Join(ignoredFilters, ",")
	}
	if len(ignoredReferenceFilters) > 0 {
		children[1].Fields["ignored_reference_filter_fields"] = strings.Join(ignoredReferenceFilters, ",")
	}
	if roleInference != "" {
		children[0].Fields["role_inference"] = roleInference
		children[1].Fields["role_inference"] = roleInference
	}
	return out, normalizeMaterialPaths([]string{sourceRel, referenceRel}), children, true, nil
}

func shouldSwapEntityReferenceInputs(sourceFieldsActual, referenceFieldsActual, sourceFields, referenceFields []string, canonicalIDField string) (bool, string) {
	current := entityReferenceOrientationScore(sourceFieldsActual, referenceFieldsActual, sourceFields, referenceFields, canonicalIDField)
	swapped := entityReferenceOrientationScore(referenceFieldsActual, sourceFieldsActual, sourceFields, referenceFields, canonicalIDField)
	if swapped <= current {
		return false, ""
	}
	candidateSourceHasSourceField := actionRecordAnyFieldExists(referenceFieldsActual, sourceFields)
	if !candidateSourceHasSourceField {
		return false, ""
	}
	candidateReferenceHasCanonical := canonicalIDField != "" && actionRecordFieldExists(sourceFieldsActual, canonicalIDField)
	candidateReferenceHasReferenceField := actionRecordAnyFieldExists(sourceFieldsActual, referenceFields)
	currentSourceMissingSourceField := !actionRecordAnyFieldExists(sourceFieldsActual, sourceFields)
	if !candidateReferenceHasCanonical && !candidateReferenceHasReferenceField && !currentSourceMissingSourceField {
		return false, ""
	}
	return true, fmt.Sprintf("swapped_source_reference_by_field_contract current_score=%d swapped_score=%d", current, swapped)
}

func entityReferenceOrientationScore(sourceFieldsActual, referenceFieldsActual, sourceFields, referenceFields []string, canonicalIDField string) int {
	score := 0
	sourceMatches := actionRecordFieldMatchCount(sourceFieldsActual, sourceFields)
	referenceSourceMatches := actionRecordFieldMatchCount(referenceFieldsActual, sourceFields)
	referenceMatches := actionRecordFieldMatchCount(referenceFieldsActual, referenceFields)
	sourceReferenceMatches := actionRecordFieldMatchCount(sourceFieldsActual, referenceFields)
	if sourceMatches > 0 {
		score += 40 + sourceMatches*4
	} else {
		score -= 40
	}
	if canonicalIDField != "" {
		if actionRecordFieldExists(referenceFieldsActual, canonicalIDField) {
			score += 50
		}
		if actionRecordFieldExists(sourceFieldsActual, canonicalIDField) {
			score -= 20
		}
	}
	score += referenceMatches * 6
	score -= sourceReferenceMatches * 3
	score -= referenceSourceMatches * 2
	return score
}

func normalizeEntityReferenceFields(action DataAction) []string {
	var fields []string
	for _, key := range []string{"reference_field", "reference_fields", "reference_name_field", "reference_name_fields", "mapping_source_field", "mapping_source_fields", "lookup_field", "lookup_fields", "reference_fields_json", "reference_name_fields_json"} {
		fields = append(fields, parseActionStringListParam(action.Params[key])...)
	}
	return cleanStringSlice(fields)
}

func splitEntityReferenceFilters(genericFilters, explicitSourceFilters, explicitReferenceFilters []contributionFilter, sourceHeaders []string, sourceRecords []actionRecord, referenceHeaders []string, referenceRecords []actionRecord) ([]contributionFilter, []contributionFilter, []string) {
	sourceFilters := append([]contributionFilter(nil), explicitSourceFilters...)
	referenceFilters := append([]contributionFilter(nil), explicitReferenceFilters...)
	if len(genericFilters) == 0 {
		return sourceFilters, referenceFilters, nil
	}
	sourceFields := actionRecordFilterFieldNames(sourceHeaders, sourceRecords)
	referenceFields := actionRecordFilterFieldNames(referenceHeaders, referenceRecords)
	var notes []string
	for _, filter := range genericFilters {
		field := strings.TrimSpace(filter.Field)
		sourceHasField := field == "" || actionRecordFieldExists(sourceFields, field)
		referenceHasField := field == "" || actionRecordFieldExists(referenceFields, field)
		sourceMatches := countRecordsPassingFilter(sourceRecords, filter)
		referenceMatches := countRecordsPassingFilter(referenceRecords, filter)
		switch {
		case !sourceHasField && referenceHasField:
			referenceFilters = append(referenceFilters, filter)
			notes = append(notes, fmt.Sprintf("%s:reference_field_only", field))
		case sourceHasField && !referenceHasField:
			sourceFilters = append(sourceFilters, filter)
			notes = append(notes, fmt.Sprintf("%s:source_field_only", field))
		case sourceHasField && referenceHasField && sourceMatches == 0 && referenceMatches > 0:
			referenceFilters = append(referenceFilters, filter)
			notes = append(notes, fmt.Sprintf("%s:reference_by_match_count", field))
		case sourceHasField && referenceHasField && referenceMatches == 0 && sourceMatches > 0:
			sourceFilters = append(sourceFilters, filter)
			notes = append(notes, fmt.Sprintf("%s:source_by_match_count", field))
		default:
			sourceFilters = append(sourceFilters, filter)
			notes = append(notes, fmt.Sprintf("%s:source_default", field))
		}
	}
	return sourceFilters, referenceFilters, notes
}

func filterActionRecords(records []actionRecord, filters []contributionFilter) []actionRecord {
	if len(filters) == 0 {
		return records
	}
	out := make([]actionRecord, 0, len(records))
	for _, record := range records {
		if recordPassesFilters(record.Fields, filters) {
			out = append(out, record)
		}
	}
	return out
}

func buildEntityReferenceLookup(records []actionRecord, rel string, referenceFields []string, canonicalIDField, canonicalLabelField string) map[string][]entityReferenceCandidate {
	out := map[string][]entityReferenceCandidate{}
	for _, record := range records {
		id := recordField(record.Fields, canonicalIDField)
		if id == "" {
			continue
		}
		label := recordField(record.Fields, canonicalLabelField)
		for _, field := range referenceFields {
			source := recordField(record.Fields, field)
			if source == "" {
				continue
			}
			for _, indexedSource := range enrichLookupSourceTerms(source) {
				candidate := entityReferenceCandidate{
					Source:   indexedSource,
					ID:       id,
					Label:    label,
					Evidence: fmt.Sprintf("%s:%d", rel, record.Line),
					Weight:   len([]rune(indexedSource)),
				}
				key := strings.ToLower(strings.TrimSpace(indexedSource))
				out[key] = append(out[key], candidate)
			}
		}
	}
	for key := range out {
		sort.SliceStable(out[key], func(i, j int) bool {
			return out[key][i].Weight > out[key][j].Weight
		})
	}
	return out
}

func selectEntityReferenceCandidate(sourceValue string, lookup map[string][]entityReferenceCandidate, matchMode string) (entityReferenceCandidate, string, string) {
	sourceValue = strings.TrimSpace(sourceValue)
	if sourceValue == "" {
		return entityReferenceCandidate{}, "missing_source", ""
	}
	key := strings.ToLower(strings.TrimSpace(sourceValue))
	var candidates []entityReferenceCandidate
	switch matchMode {
	case "contains", "mapping_contains_source", "reference_contains_source":
		for _, values := range lookup {
			for _, candidate := range values {
				haystack := strings.ToLower(strings.TrimSpace(candidate.Source))
				if (matchMode == "contains" && (strings.Contains(haystack, key) || strings.Contains(key, haystack))) ||
					(matchMode != "contains" && strings.Contains(haystack, key)) {
					candidates = append(candidates, candidate)
				}
			}
		}
	case "source_contains_mapping":
		for _, values := range lookup {
			for _, candidate := range values {
				needle := strings.ToLower(strings.TrimSpace(candidate.Source))
				if needle != "" && strings.Contains(key, needle) {
					candidates = append(candidates, candidate)
				}
			}
		}
	default:
		candidates = append(candidates, lookup[key]...)
	}
	if len(candidates) == 0 && matchMode != "exact" {
		candidates = append(candidates, lookup[key]...)
	}
	if len(candidates) == 0 {
		return entityReferenceCandidate{}, "unresolved", ""
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Weight > candidates[j].Weight
	})
	best := candidates[0]
	ambiguous := false
	for _, candidate := range candidates[1:] {
		if candidate.ID != best.ID {
			ambiguous = true
			break
		}
	}
	status := "resolved"
	if ambiguous {
		status = "matched_ambiguous"
	}
	return best, status, best.Evidence
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
	SourceFields        []string
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
	SourceField  string
	SourceFields []string
	TargetField  string
	Operation    string
	Pattern      string
	Replacement  string
	Value        string
	Default      string
	Separator    string
	Mapping      map[string]string
	Multiplier   string
	Divisor      string
	Group        int
	Start        int
	Length       int
}

func (r ActionRunner) runDeriveFields(action DataAction) (DataArtifact, []map[string]any, []string, error) {
	paths := cleanStringListPreserveOrder(action.InputPaths)
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
	markKnownActionVirtualFields(knownFields)
	for _, spec := range specs {
		if err := validateDeriveFieldSpec(spec, knownFields); err != nil {
			return DataArtifact{}, nil, nil, err
		}
		if target := strings.ToLower(strings.TrimSpace(spec.TargetField)); target != "" {
			knownFields[target] = true
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
		currentFields := map[string]string{}
		for key, value := range record.Fields {
			row[key] = value
			currentFields[key] = value
		}
		sourceFields := actionRecordVirtualFields(record, rel)
		for key, value := range sourceFields {
			if strings.TrimSpace(recordField(currentFields, key)) == "" {
				row[key] = value
				currentFields[key] = value
			}
		}
		for _, spec := range specs {
			value := applyDeriveFieldSpec(currentFields, spec)
			row[spec.TargetField] = value
			currentFields[spec.TargetField] = value
			if strings.TrimSpace(value) != "" {
				derivedNonEmpty[spec.TargetField]++
			}
		}
		for key, value := range sourceFields {
			if actionReservedSourceField(key) {
				row[key] = value
			}
		}
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

func (r ActionRunner) runExpandRecords(action DataAction) (DataArtifact, []map[string]any, []string, error) {
	paths := cleanStringListPreserveOrder(action.InputPaths)
	inputPath := firstNonEmptyString(strings.TrimSpace(action.Params["input_path"]), strings.TrimSpace(action.Params["record_path"]), strings.TrimSpace(action.Params["base_path"]))
	if inputPath == "" && len(paths) > 0 {
		inputPath = paths[0]
	}
	if inputPath == "" {
		return DataArtifact{}, nil, nil, errors.New("expand_records requires input_path or at least one input_path")
	}
	sourceField := firstNonEmptyString(action.Params["source_field"], action.Params["input_field"], action.Params["field"], action.Params["value_field"])
	if strings.TrimSpace(sourceField) == "" {
		return DataArtifact{}, nil, nil, errors.New("expand_records requires source_field")
	}
	targetField := firstNonEmptyString(action.Params["target_field"], action.Params["output_field"], action.Params["expanded_field"], sourceField)
	originalField := strings.TrimSpace(firstNonEmptyString(action.Params["original_field"], action.Params["keep_original_field"]))
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxOutput := actionIntParam(action, "max_output_records", 200000, 1, 2000000)
	records, headers, total, rel, err := r.readActionRecords(inputPath, maxRecords)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	knownFields := map[string]bool{}
	markKnownActionFields(knownFields, headers, records)
	if !knownFields[strings.ToLower(strings.TrimSpace(sourceField))] {
		return DataArtifact{}, nil, nil, fmt.Errorf("expand_records source_field %q was not found in input %s", sourceField, rel)
	}
	var rows []map[string]any
	expandedRows := 0
	emptyRows := 0
	for _, record := range records {
		sourceValue := recordField(record.Fields, sourceField)
		values := splitExpandRecordValues(sourceValue, action)
		if len(values) == 0 {
			emptyRows++
			if !parseBoolActionParam(firstNonEmptyString(action.Params["drop_empty"], "true")) {
				values = []string{""}
			}
		}
		for _, value := range values {
			if len(rows) >= maxOutput {
				return DataArtifact{}, nil, nil, fmt.Errorf("expand_records exceeded max_output_records=%d; split the action or lower max_records", maxOutput)
			}
			row := map[string]any{}
			for key, original := range record.Fields {
				row[key] = original
			}
			if originalField != "" {
				row[originalField] = sourceValue
			}
			row[targetField] = value
			row["_source"] = rel
			row["_source_line"] = record.Line
			row["_source_index"] = record.Index
			rows = append(rows, row)
			expandedRows++
		}
	}
	outputHeaders := collectJoinedRecordHeaders(rows)
	if len(outputHeaders) == 0 {
		outputHeaders = append([]string(nil), headers...)
		if originalField != "" && !stringSliceContainsFold(outputHeaders, originalField) {
			outputHeaders = append(outputHeaders, originalField)
		}
		if !stringSliceContainsFold(outputHeaders, targetField) {
			outputHeaders = append(outputHeaders, targetField)
		}
		sort.Strings(outputHeaders)
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "expanded_records")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionExpandRecords),
		SourcePaths: []string{rel},
		Headers:     outputHeaders,
		RowCount:    len(rows),
		Summary:     fmt.Sprintf("expanded field %s into %d record(s) from %d source record(s)", sourceField, len(rows), total),
		Sample:      sampleJoinedActionRows(rows, 3),
		Fields: map[string]string{
			"input_path":     rel,
			"input_rows":     fmt.Sprintf("%d", total),
			"output_rows":    fmt.Sprintf("%d", len(rows)),
			"source_field":   sourceField,
			"target_field":   targetField,
			"empty_rows":     fmt.Sprintf("%d", emptyRows),
			"expanded_rows":  fmt.Sprintf("%d", expandedRows),
			"original_field": originalField,
		},
		Children: []DataArtifact{{
			ID:          rel + "#source",
			Kind:        "expand_records/source",
			SourcePaths: []string{rel},
			Headers:     headers,
			RowCount:    total,
			Summary:     fmt.Sprintf("%s supplied %d source record(s)", rel, total),
		}},
	}, rows, []string{rel}, nil
}

func (r ActionRunner) runFilterRecords(action DataAction) (DataArtifact, []map[string]any, []string, error) {
	paths := cleanStringListPreserveOrder(action.InputPaths)
	inputPath := firstNonEmptyString(strings.TrimSpace(action.Params["input_path"]), strings.TrimSpace(action.Params["record_path"]), strings.TrimSpace(action.Params["base_path"]))
	if inputPath == "" && len(paths) > 0 {
		inputPath = paths[0]
	}
	if inputPath == "" {
		return DataArtifact{}, nil, nil, errors.New("filter_records requires input_path or at least one input_path")
	}
	filters, err := parseContributionFilters(action)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	if len(filters) == 0 {
		return DataArtifact{}, nil, nil, errors.New("filter_records requires filters_json or filter_field/filter_value")
	}
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxOutput := actionIntParam(action, "max_output_records", 100000, 1, 1000000)
	records, headers, total, rel, err := r.readActionRecords(inputPath, maxRecords)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	effectiveFilters, ignoredFilterFields := effectiveActionFiltersForRecords(filters, headers, records)
	filterDiagnostics := filterDiagnosticsJSON(records, effectiveFilters, 8, 4)
	if len(ignoredFilterFields) > 0 {
		fields := actionRecordFilterFieldNames(headers, records)
		return DataArtifact{}, nil, nil, fmt.Errorf("filter_records field(s) [%s] were not found in input %s fields [%s]; field_samples=%s; filter_diagnostics=%s",
			strings.Join(ignoredFilterFields, ", "),
			rel,
			strings.Join(clampStringSliceForError(fields, 32), ", "),
			compactFieldSampleJSON(records, filterFieldNames(filters), 8, 3),
			filterDiagnostics,
		)
	}
	rows := make([]map[string]any, 0, minInt(maxOutput, len(records)))
	for _, record := range records {
		if !recordPassesFilters(record.Fields, effectiveFilters) {
			continue
		}
		if len(rows) >= maxOutput {
			return DataArtifact{}, nil, nil, fmt.Errorf("filter_records exceeded max_output_records=%d; split the action or lower max_records", maxOutput)
		}
		row := map[string]any{}
		for key, value := range record.Fields {
			row[key] = value
		}
		row["_source"] = rel
		row["_source_line"] = record.Line
		row["_source_index"] = record.Index
		rows = append(rows, row)
	}
	outputHeaders := collectJoinedRecordHeaders(rows)
	if len(outputHeaders) == 0 {
		outputHeaders = append([]string(nil), headers...)
		for _, extra := range []string{"_source", "_source_line", "_source_index"} {
			if !stringSliceContainsFold(outputHeaders, extra) {
				outputHeaders = append(outputHeaders, extra)
			}
		}
		sort.Strings(outputHeaders)
	}
	filterFields := filterFieldNames(effectiveFilters)
	filterMatched := 0
	for _, record := range records {
		if recordPassesFilters(record.Fields, effectiveFilters) {
			filterMatched++
		}
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "filtered_records")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionFilterRecords),
		SourcePaths: []string{rel},
		Headers:     outputHeaders,
		RowCount:    len(rows),
		Summary:     fmt.Sprintf("filtered %d of %d record(s) from %s", len(rows), total, rel),
		Sample:      sampleJoinedActionRows(rows, 3),
		Fields: map[string]string{
			"input_path":         rel,
			"input_rows":         fmt.Sprintf("%d", total),
			"output_rows":        fmt.Sprintf("%d", len(rows)),
			"filter_matched":     fmt.Sprintf("%d", filterMatched),
			"filter_fields":      strings.Join(filterFields, ","),
			"filter_diagnostics": filterDiagnostics,
		},
		Children: []DataArtifact{{
			ID:          rel + "#source",
			Kind:        "filter_records/source",
			SourcePaths: []string{rel},
			Headers:     headers,
			RowCount:    total,
			Summary:     fmt.Sprintf("%s supplied %d source record(s)", rel, total),
		}},
	}, rows, []string{rel}, nil
}

func (r ActionRunner) runQualifyRecords(action DataAction, defaultRuleRefs []string) (DataArtifact, []map[string]any, []RowDecision, []string, error) {
	paths := cleanStringListPreserveOrder(action.InputPaths)
	inputPath := firstNonEmptyString(strings.TrimSpace(action.Params["input_path"]), strings.TrimSpace(action.Params["record_path"]), strings.TrimSpace(action.Params["base_path"]))
	if inputPath == "" && len(paths) > 0 {
		inputPath = paths[0]
	}
	if inputPath == "" {
		return DataArtifact{}, nil, nil, nil, errors.New("qualify_records requires input_path or at least one input_path")
	}
	includeFilters, err := parseContributionFilters(action)
	if err != nil {
		return DataArtifact{}, nil, nil, nil, err
	}
	rejectFilters, err := parseActionFilterListParams(action, "reject_filters", "exclude_filters", "block_filters")
	if err != nil {
		return DataArtifact{}, nil, nil, nil, err
	}
	requiredFields := parseActionStringListParam(firstNonEmptyString(action.Params["required_fields"], action.Params["non_empty_fields"]))
	evidenceFields := parseActionStringListParam(firstNonEmptyString(action.Params["evidence_fields"], action.Params["required_evidence_fields"]))
	statusFields := parseActionStringListParam(firstNonEmptyString(action.Params["status_fields"], action.Params["generated_status_fields"]))
	acceptedStatuses := lowerStringSet(parseActionStringListParam(action.Params["accepted_statuses"]))
	blockedStatuses := lowerStringSet(parseActionStringListParam(action.Params["blocked_statuses"]))
	if len(blockedStatuses) == 0 {
		blockedStatuses = defaultGeneratedBlockingStatusValues()
	}
	autoStatusFields := parseBoolActionParam(firstNonEmptyString(action.Params["auto_status_fields"], "true"))
	passField := firstNonEmptyString(strings.TrimSpace(action.Params["pass_field"]), strings.TrimSpace(action.Params["eligible_field"]), "eligible")
	reasonField := firstNonEmptyString(strings.TrimSpace(action.Params["reason_field"]), "eligibility_reason")
	itemIDField := strings.TrimSpace(action.Params["item_id_field"])
	outputMode := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action.Params["output_mode"], action.Params["mode"], "filter")))
	if outputMode == "" {
		outputMode = "filter"
	}
	if outputMode != "filter" && outputMode != "annotate" && outputMode != "all" {
		return DataArtifact{}, nil, nil, nil, fmt.Errorf("qualify_records unsupported output_mode %q; use filter or annotate", outputMode)
	}
	ruleRefs := parseActionStringListParam(action.Params["rule_refs"])
	if len(ruleRefs) == 0 {
		ruleRefs = append([]string(nil), defaultRuleRefs...)
	}
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxOutput := actionIntParam(action, "max_output_records", 100000, 1, 1000000)
	records, headers, total, rel, err := r.readActionRecords(inputPath, maxRecords)
	if err != nil {
		return DataArtifact{}, nil, nil, nil, err
	}
	knownFields := map[string]bool{}
	markKnownActionFields(knownFields, headers, records)
	allFilterFields := append(filterFieldNames(includeFilters), filterFieldNames(rejectFilters)...)
	for _, field := range append(append(append([]string{}, requiredFields...), evidenceFields...), allFilterFields...) {
		if strings.TrimSpace(field) == "" {
			continue
		}
		if !knownFields[strings.ToLower(strings.TrimSpace(field))] {
			return DataArtifact{}, nil, nil, nil, fmt.Errorf("qualify_records field %q was not found in input %s fields [%s]", field, rel, strings.Join(clampStringSliceForError(actionRecordFieldNames(headers, records), 32), ", "))
		}
	}
	if len(statusFields) == 0 && autoStatusFields {
		statusFields = generatedStatusFields(headers, records)
	}
	statusFields = cleanStringSlice(statusFields)
	for _, field := range statusFields {
		if !knownFields[strings.ToLower(strings.TrimSpace(field))] {
			return DataArtifact{}, nil, nil, nil, fmt.Errorf("qualify_records status_field %q was not found in input %s fields [%s]", field, rel, strings.Join(clampStringSliceForError(actionRecordFieldNames(headers, records), 32), ", "))
		}
	}
	rows := make([]map[string]any, 0, minInt(maxOutput, len(records)))
	decisions := make([]RowDecision, 0, len(records))
	eligibleCount := 0
	excludedCount := 0
	reasonCounts := map[string]int{}
	for _, record := range records {
		eligible, reason := recordQualification(record, includeFilters, rejectFilters, requiredFields, evidenceFields, statusFields, acceptedStatuses, blockedStatuses)
		if reason == "" {
			reason = "qualified"
		}
		reasonCounts[reason]++
		if eligible {
			eligibleCount++
		} else {
			excludedCount++
		}
		decision := "exclude"
		if eligible {
			decision = "include"
		}
		itemID := recordField(record.Fields, itemIDField)
		if itemID == "" {
			itemID = fmt.Sprintf("%s#%d", rel, record.Index)
		}
		decisions = append(decisions, RowDecision{
			RowID:         itemID,
			Source:        rel,
			SourceLocator: fmt.Sprintf("line:%d", record.Line),
			Decision:      decision,
			Reason:        reason,
			EvidenceRef:   qualificationEvidenceRefs(record, rel, statusFields, evidenceFields),
			RuleRefs:      cleanStringList(ruleRefs),
			NormalizedFields: map[string]string{
				passField: eligibleString(eligible),
			},
		})
		if outputMode == "filter" && !eligible {
			continue
		}
		if len(rows) >= maxOutput {
			return DataArtifact{}, nil, nil, nil, fmt.Errorf("qualify_records exceeded max_output_records=%d; split the action or lower max_records", maxOutput)
		}
		row := map[string]any{}
		for key, value := range record.Fields {
			row[key] = value
		}
		row[passField] = eligibleString(eligible)
		row[reasonField] = reason
		row["_source"] = rel
		row["_source_line"] = record.Line
		row["_source_index"] = record.Index
		rows = append(rows, row)
	}
	outputHeaders := collectJoinedRecordHeaders(rows)
	if len(outputHeaders) == 0 {
		outputHeaders = append([]string(nil), headers...)
		for _, extra := range []string{passField, reasonField, "_source", "_source_line", "_source_index"} {
			if !stringSliceContainsFold(outputHeaders, extra) {
				outputHeaders = append(outputHeaders, extra)
			}
		}
		sort.Strings(outputHeaders)
	}
	reasonJSON, _ := json.Marshal(reasonCounts)
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "qualified_records")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionQualifyRecords),
		SourcePaths: []string{rel},
		Headers:     outputHeaders,
		RowCount:    len(rows),
		Summary:     fmt.Sprintf("qualified %d of %d record(s) from %s", eligibleCount, total, rel),
		Sample:      sampleJoinedActionRows(rows, 3),
		Fields: map[string]string{
			"input_path":            rel,
			"input_rows":            fmt.Sprintf("%d", total),
			"output_rows":           fmt.Sprintf("%d", len(rows)),
			"eligible_rows":         fmt.Sprintf("%d", eligibleCount),
			"excluded_rows":         fmt.Sprintf("%d", excludedCount),
			"output_mode":           outputMode,
			"pass_field":            passField,
			"reason_field":          reasonField,
			"include_filters":       filterDiagnosticsJSON(records, includeFilters, 8, 4),
			"reject_filters":        filterDiagnosticsJSON(records, rejectFilters, 8, 4),
			"required_fields":       strings.Join(requiredFields, ","),
			"evidence_fields":       strings.Join(evidenceFields, ","),
			"status_fields":         strings.Join(statusFields, ","),
			"qualification_reasons": string(reasonJSON),
		},
		Children: []DataArtifact{{
			ID:          rel + "#source",
			Kind:        "qualify_records/source",
			SourcePaths: []string{rel},
			Headers:     headers,
			RowCount:    total,
			Summary:     fmt.Sprintf("%s supplied %d source record(s)", rel, total),
		}},
	}, rows, decisions, []string{rel}, nil
}

func splitExpandRecordValues(sourceValue string, action DataAction) []string {
	sourceValue = strings.TrimSpace(sourceValue)
	if sourceValue == "" {
		return nil
	}
	var raw []string
	if pattern := strings.TrimSpace(firstNonEmptyString(action.Params["split_pattern"], action.Params["separator_pattern"], action.Params["delimiter_pattern"])); pattern != "" {
		re, err := regexp.Compile(pattern)
		if err == nil {
			raw = re.Split(sourceValue, -1)
		} else {
			raw = []string{sourceValue}
		}
	} else {
		delimiter := firstNonEmptyString(action.Params["delimiter"], action.Params["separator"])
		if strings.TrimSpace(delimiter) == "" || strings.EqualFold(strings.TrimSpace(delimiter), "auto") {
			raw = regexp.MustCompile(`[;；,，|\n\r]+`).Split(sourceValue, -1)
		} else {
			raw = strings.Split(sourceValue, delimiter)
		}
	}
	trim := parseBoolActionParam(firstNonEmptyString(action.Params["trim"], "true"))
	dedupe := parseBoolActionParam(firstNonEmptyString(action.Params["dedupe"], "true"))
	seen := map[string]bool{}
	var out []string
	for _, value := range raw {
		if trim {
			value = strings.TrimSpace(value)
		}
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if dedupe && seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func stringSliceContainsFold(values []string, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == needle {
			return true
		}
	}
	return false
}

func parseDeriveFieldSpecs(action DataAction) ([]deriveFieldSpec, error) {
	raw := strings.TrimSpace(firstNonEmptyString(
		structuredActionParam(action.Params, "field_specs"),
		action.Params["field_specs_json"],
		structuredActionParam(action.Params, "derive_specs"),
		action.Params["derive_specs_json"],
		structuredActionParam(action.Params, "transforms"),
		action.Params["transforms_json"],
	))
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
		SourceField:  firstNonEmptyString(action.Params["source_field"], action.Params["input_field"], action.Params["field"]),
		SourceFields: parseActionStringListParam(firstNonEmptyString(action.Params["source_fields"], action.Params["input_fields"], action.Params["fields"])),
		TargetField:  firstNonEmptyString(action.Params["target_field"], action.Params["output_field"], action.Params["derived_field"]),
		Operation:    firstNonEmptyString(action.Params["operation"], action.Params["op"], action.Params["transform"]),
		Pattern:      action.Params["pattern"],
		Replacement:  action.Params["replacement"],
		Value:        action.Params["value"],
		Default:      action.Params["default"],
		Separator:    firstNonEmptyString(action.Params["separator"], action.Params["delimiter"], action.Params["joiner"]),
		Multiplier:   firstNonEmptyString(action.Params["multiplier"], action.Params["scale"]),
		Divisor:      action.Params["divisor"],
		Group:        actionIntParam(action, "group", 1, 0, 64),
		Start:        actionIntParam(action, "start", 0, 0, 1000000),
		Length:       actionIntParam(action, "length", 0, 0, 1000000),
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
	getList := func(keys ...string) []string {
		for _, key := range keys {
			if v, ok := value[key]; ok {
				return parseActionStringListParamFromAny(v)
			}
		}
		return nil
	}
	spec := deriveFieldSpec{
		SourceField:  getString("source_field", "input_field", "field"),
		SourceFields: getList("source_fields", "input_fields", "fields"),
		TargetField:  getString("target_field", "output_field", "derived_field"),
		Operation:    getString("operation", "op", "transform"),
		Pattern:      getString("pattern", "regex"),
		Replacement:  getString("replacement"),
		Value:        getString("value"),
		Default:      getString("default", "default_value"),
		Separator:    getString("separator", "delimiter", "joiner"),
		Multiplier:   getString("multiplier", "scale"),
		Divisor:      getString("divisor"),
		Group:        getInt(1, "group", "capture_group"),
		Start:        getInt(0, "start"),
		Length:       getInt(0, "length"),
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
	case string:
		raw := strings.TrimSpace(typed)
		if raw == "" {
			return nil, errors.New("empty derive_fields mapping string; use an object of source-to-target scalar strings")
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			return nil, fmt.Errorf("parse derive_fields mapping string as JSON object: %w", err)
		}
		return parseDeriveFieldMappingFromAny(decoded)
	default:
		return nil, fmt.Errorf("unsupported derive_fields mapping shape %T; use an object of source-to-target scalar strings", value)
	}
}

func normalizeDeriveFieldSpec(spec deriveFieldSpec) deriveFieldSpec {
	spec.SourceField = strings.TrimSpace(spec.SourceField)
	spec.SourceFields = cleanStringSlice(spec.SourceFields)
	spec.TargetField = strings.TrimSpace(spec.TargetField)
	spec.Operation = normalizeDeriveFieldOperation(firstNonEmptyString(spec.Operation, "copy"))
	spec.Pattern = strings.TrimSpace(spec.Pattern)
	spec.Default = strings.TrimSpace(spec.Default)
	spec.Separator = strings.TrimSpace(spec.Separator)
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
		case "concat", "join", "join_fields", "coalesce", "first_non_empty":
			spec.TargetField = "derived_field"
		default:
			spec.TargetField = spec.SourceField + "_derived"
		}
	}
	return spec
}

func normalizeDeriveFieldOperation(operation string) string {
	operation = strings.ToLower(strings.TrimSpace(operation))
	operation = strings.ReplaceAll(operation, " ", "")
	switch operation {
	case "year/extract_year", "extract_year/year":
		return "extract_year"
	case "concat/join_fields", "join_fields/concat", "concat/join", "join/concat":
		return "concat"
	case "coalesce/first_non_empty", "first_non_empty/coalesce":
		return "coalesce"
	default:
		return operation
	}
}

func validateDeriveFieldSpec(spec deriveFieldSpec, knownFields map[string]bool) error {
	if actionReservedSourceField(spec.TargetField) {
		return fmt.Errorf("derive_fields target_field %q is a reserved source locator field; read it as a source_field instead of overwriting it", spec.TargetField)
	}
	switch spec.Operation {
	case "constant":
		if spec.TargetField == "" {
			return errors.New("derive_fields constant spec requires target_field")
		}
		if actionIndexLikeField(spec.TargetField) {
			return fmt.Errorf("derive_fields constant spec cannot create locator-like target_field %q; copy an existing source/index field or derive it from a real record field", spec.TargetField)
		}
		return nil
	case "concat", "join", "join_fields", "coalesce", "first_non_empty":
		if spec.TargetField == "" {
			return fmt.Errorf("derive_fields %s spec requires target_field", spec.Operation)
		}
		if len(spec.SourceFields) == 0 {
			return fmt.Errorf("derive_fields %s spec requires source_fields", spec.Operation)
		}
		found := 0
		var missing []string
		for _, field := range spec.SourceFields {
			if knownFields[strings.ToLower(strings.TrimSpace(field))] {
				found++
			} else {
				missing = append(missing, field)
			}
		}
		if found == 0 {
			return fmt.Errorf("derive_fields %s source_fields [%s] were not found in input record fields", spec.Operation, strings.Join(missing, ", "))
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
	case "concat", "join", "join_fields":
		out = deriveConcatFields(fields, spec)
	case "coalesce", "first_non_empty":
		out = deriveCoalesceFields(fields, spec)
	default:
		out = source
	}
	if strings.TrimSpace(out) == "" && spec.Default != "" {
		return spec.Default
	}
	return strings.TrimSpace(out)
}

func deriveConcatFields(fields map[string]string, spec deriveFieldSpec) string {
	separator := spec.Separator
	var parts []string
	for _, field := range spec.SourceFields {
		value := strings.TrimSpace(recordField(fields, field))
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, separator)
}

func deriveCoalesceFields(fields map[string]string, spec deriveFieldSpec) string {
	for _, field := range spec.SourceFields {
		value := strings.TrimSpace(recordField(fields, field))
		if value != "" {
			return value
		}
	}
	return ""
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

type applyResolutionSpec struct {
	BasePath             string
	ResolutionPath       string
	BaseKeyFields        []string
	ResolutionKeyFields  []string
	GenericFilters       []contributionFilter
	BaseFilters          []contributionFilter
	ResolutionFilters    []contributionFilter
	TargetIDField        string
	TargetLabelField     string
	TargetStatusField    string
	CanonicalIDField     string
	CanonicalLabelField  string
	StatusField          string
	AcceptedStatuses     []string
	UnmatchedStatusValue string
}

type resolutionChoice struct {
	rec    actionRecord
	status string
}

func (r ActionRunner) runApplyEntityResolutions(action DataAction) (DataArtifact, []map[string]any, []string, error) {
	paths := cleanStringListPreserveOrder(action.InputPaths)
	basePathParam := firstNonEmptyString(strings.TrimSpace(action.Params["base_path"]), strings.TrimSpace(action.Params["record_path"]), strings.TrimSpace(action.Params["input_path"]))
	preliminaryBasePath := basePathParam
	if preliminaryBasePath == "" && len(paths) > 0 {
		preliminaryBasePath = paths[0]
	}
	specs, err := parseApplyResolutionSpecs(action, paths, preliminaryBasePath)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	if len(specs) == 0 {
		return DataArtifact{}, nil, nil, errors.New("apply_entity_resolutions requires at least one resolution spec or resolution_path")
	}
	basePath, err := resolveApplyResolutionBasePath(basePathParam, paths, specs)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	if basePath == "" {
		return DataArtifact{}, nil, nil, errors.New("apply_entity_resolutions requires base_path or at least one input_path")
	}
	specs, err = finalizeApplyResolutionSpecs(specs, paths, basePath)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxOutput := actionIntParam(action, "max_output_records", 100000, 1, 1000000)
	baseRecords, baseHeaders, baseTotal, baseRel, err := r.readActionRecords(basePath, maxRecords)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	preserveBaseRows := applyResolutionPreserveBaseRows(action)
	baseFields := actionRecordFieldNames(baseHeaders, baseRecords)
	genericFilters, err := parseContributionFilters(action)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	explicitBaseFilters, err := parseActionFilterListParams(action, "base_filters", "record_filters", "source_filters")
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	explicitResolutionFilters, err := parseActionFilterListParams(action, "resolution_filters", "mapping_filters", "reference_filters", "lookup_filters")
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	for _, spec := range specs {
		if len(spec.BaseKeyFields) > 0 && !actionRecordAnyFieldOrVirtualExists(baseFields, spec.BaseKeyFields, applyResolutionVirtualBaseKeyFields()...) {
			return DataArtifact{}, nil, nil, fmt.Errorf("apply_entity_resolutions base_key_fields [%s] were not found in base input %s fields [%s]", strings.Join(spec.BaseKeyFields, ", "), baseRel, strings.Join(clampStringSliceForError(baseFields, 32), ", "))
		}
	}
	consumed := []string{baseRel}
	indexes := make([]map[string][]resolutionChoice, 0, len(specs))
	children := make([]DataArtifact, 0, len(specs)+1)
	for i, spec := range specs {
		records, headers, total, rel, err := r.readActionRecords(spec.ResolutionPath, maxRecords)
		if err != nil {
			return DataArtifact{}, nil, nil, err
		}
		spec = normalizeApplyResolutionSpecForAvailableFields(spec, headers, records)
		specs[i].ResolutionPath = rel
		combinedGenericFilters := append(append([]contributionFilter(nil), genericFilters...), spec.GenericFilters...)
		combinedBaseFilters := append(append([]contributionFilter(nil), explicitBaseFilters...), spec.BaseFilters...)
		combinedResolutionFilters := append(append([]contributionFilter(nil), explicitResolutionFilters...), spec.ResolutionFilters...)
		baseSideFilters, resolutionSideFilters, filterNotes := splitEntityReferenceFilters(
			combinedGenericFilters,
			combinedBaseFilters,
			combinedResolutionFilters,
			baseHeaders, baseRecords, headers, records,
		)
		effectiveBaseFilters, ignoredBaseFilters := effectiveActionFiltersForRecords(baseSideFilters, baseHeaders, baseRecords)
		effectiveResolutionFilters, ignoredResolutionFilters := effectiveActionFiltersForRecords(resolutionSideFilters, headers, records)
		specs[i].BaseFilters = effectiveBaseFilters
		specs[i].ResolutionFilters = effectiveResolutionFilters
		if len(effectiveResolutionFilters) > 0 {
			records = filterActionRecords(records, effectiveResolutionFilters)
		}
		consumed = append(consumed, rel)
		index := map[string][]resolutionChoice{}
		for _, rec := range records {
			key := resolutionRecordApplyKey(rec, spec)
			if key == "" {
				continue
			}
			status := firstNonEmptyString(recordField(rec.Fields, spec.StatusField), "resolved")
			index[key] = append(index[key], resolutionChoice{rec: rec, status: status})
		}
		indexes = append(indexes, index)
		children = append(children, DataArtifact{
			ID:          rel + "#entity_resolution_source",
			Kind:        "apply_entity_resolutions/resolution",
			SourcePaths: []string{rel},
			Headers:     headers,
			RowCount:    total,
			Summary:     fmt.Sprintf("%s supplied %d entity resolution record(s)", rel, total),
			Fields: map[string]string{
				"target_id_field":     spec.TargetIDField,
				"target_label_field":  spec.TargetLabelField,
				"target_status_field": spec.TargetStatusField,
				"base_key_fields":     strings.Join(spec.BaseKeyFields, ","),
				"resolution_key_fields": strings.Join(
					spec.ResolutionKeyFields, ","),
				"resolution_filters": filterDiagnosticsJSON(records, effectiveResolutionFilters, 8, 8),
				"base_filters":       filterDiagnosticsJSON(baseRecords, effectiveBaseFilters, 8, 8),
				"filter_role_notes":  strings.Join(filterNotes, "; "),
			},
		})
		if len(ignoredBaseFilters) > 0 {
			children[len(children)-1].Fields["ignored_base_filter_fields"] = strings.Join(ignoredBaseFilters, ",")
		}
		if len(ignoredResolutionFilters) > 0 {
			children[len(children)-1].Fields["ignored_resolution_filter_fields"] = strings.Join(ignoredResolutionFilters, ",")
		}
	}
	children = append([]DataArtifact{{
		ID:          baseRel + "#base",
		Kind:        "apply_entity_resolutions/base",
		SourcePaths: []string{baseRel},
		Headers:     baseHeaders,
		RowCount:    baseTotal,
		Summary:     fmt.Sprintf("%s supplied %d base record(s)", baseRel, baseTotal),
	}}, children...)

	if len(specs) > 0 && !preserveBaseRows {
		baseRecords = applyResolutionBaseRecordsFiltered(baseRecords, specs)
	}
	rows := make([]map[string]any, 0, minInt(maxOutput, len(baseRecords)))
	matchedByTarget := map[string]int{}
	unmatchedByTarget := map[string]int{}
	ambiguousByTarget := map[string]int{}
	skippedByTarget := map[string]int{}
	for _, base := range baseRecords {
		if len(rows) >= maxOutput {
			break
		}
		row := map[string]any{}
		for key, value := range base.Fields {
			row[key] = value
		}
		row["_source"] = baseRel
		row["_source_line"] = base.Line
		row["_source_index"] = base.Index
		for i, spec := range specs {
			if spec.TargetIDField != "" {
				row[spec.TargetIDField] = ""
			}
			if spec.TargetLabelField != "" {
				row[spec.TargetLabelField] = ""
			}
			if preserveBaseRows && len(spec.BaseFilters) > 0 && !recordPassesFilters(base.Fields, spec.BaseFilters) {
				skippedByTarget[spec.TargetIDField]++
				if spec.TargetStatusField != "" {
					row[spec.TargetStatusField] = "not_applicable"
				}
				continue
			}
			key := baseRecordApplyResolutionKey(base, spec)
			choices := filterAcceptedResolutionChoices(indexes[i][key], spec)
			status := spec.UnmatchedStatusValue
			if status == "" {
				status = "unmatched"
			}
			if len(choices) == 1 {
				choice := choices[0].rec
				row[spec.TargetIDField] = recordField(choice.Fields, spec.CanonicalIDField)
				if spec.TargetLabelField != "" {
					row[spec.TargetLabelField] = recordField(choice.Fields, spec.CanonicalLabelField)
				}
				status = firstNonEmptyString(choices[0].status, "resolved")
				matchedByTarget[spec.TargetIDField]++
			} else if len(choices) > 1 {
				status = "ambiguous"
				ambiguousByTarget[spec.TargetIDField]++
			} else {
				unmatchedByTarget[spec.TargetIDField]++
			}
			if spec.TargetStatusField != "" {
				row[spec.TargetStatusField] = status
			}
		}
		rows = append(rows, row)
	}
	outputHeaders := collectJoinedRecordHeaders(rows)
	if len(outputHeaders) == 0 {
		outputHeaders = append([]string(nil), baseHeaders...)
		for _, spec := range specs {
			for _, field := range []string{spec.TargetIDField, spec.TargetLabelField, spec.TargetStatusField} {
				if field != "" && !stringSliceContainsFold(outputHeaders, field) {
					outputHeaders = append(outputHeaders, field)
				}
			}
		}
		sort.Strings(outputHeaders)
	}
	targetFields := make([]string, 0, len(specs))
	fields := map[string]string{
		"base_path":        baseRel,
		"base_rows":        fmt.Sprintf("%d", baseTotal),
		"output_rows":      fmt.Sprintf("%d", len(rows)),
		"base_filter_mode": applyResolutionBaseFilterModeLabel(preserveBaseRows),
	}
	for _, spec := range specs {
		targetFields = append(targetFields, spec.TargetIDField)
		fields["matched_"+spec.TargetIDField] = fmt.Sprintf("%d", matchedByTarget[spec.TargetIDField])
		fields["unmatched_"+spec.TargetIDField] = fmt.Sprintf("%d", unmatchedByTarget[spec.TargetIDField])
		fields["ambiguous_"+spec.TargetIDField] = fmt.Sprintf("%d", ambiguousByTarget[spec.TargetIDField])
		fields["not_applicable_"+spec.TargetIDField] = fmt.Sprintf("%d", skippedByTarget[spec.TargetIDField])
	}
	fields["target_fields"] = strings.Join(targetFields, ",")
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "resolved_records")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionApplyResolutions),
		SourcePaths: normalizeMaterialPaths(consumed),
		Headers:     outputHeaders,
		RowCount:    len(rows),
		Summary:     fmt.Sprintf("applied %d entity resolution set(s) to %d base record(s)", len(specs), len(rows)),
		Sample:      sampleJoinedActionRows(rows, 3),
		Fields:      fields,
		Children:    children,
	}, rows, normalizeMaterialPaths(consumed), nil
}

func parseApplyResolutionSpecs(action DataAction, paths []string, basePath string) ([]applyResolutionSpec, error) {
	topLevelResolutionPath := firstNonEmptyString(action.Params["resolution_path"], action.Params["mapping_path"], action.Params["lookup_path"])
	raw := strings.TrimSpace(firstNonEmptyString(
		structuredActionParam(action.Params, "resolution_specs"),
		action.Params["resolution_specs_json"],
		structuredActionParam(action.Params, "apply_specs"),
		action.Params["apply_specs_json"],
	))
	var specs []applyResolutionSpec
	if raw != "" {
		var values []map[string]any
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, fmt.Errorf("parse apply_entity_resolutions resolution_specs_json: %w", err)
		}
		for _, value := range values {
			spec, err := applyResolutionSpecFromMap(value)
			if err != nil {
				return nil, err
			}
			if spec.ResolutionPath == "" {
				spec.ResolutionPath = topLevelResolutionPath
			}
			specs = append(specs, spec)
		}
	} else {
		resolutionPath := topLevelResolutionPath
		if resolutionPath == "" {
			for _, p := range paths {
				if normalizeMaterialPath(p) != normalizeMaterialPath(basePath) {
					resolutionPath = p
					break
				}
			}
		}
		specs = append(specs, applyResolutionSpec{
			BasePath:            firstNonEmptyString(action.Params["base_path"], action.Params["record_path"], action.Params["input_path"], action.Params["source_path"]),
			ResolutionPath:      resolutionPath,
			BaseKeyFields:       parseActionStringListParam(firstNonEmptyString(action.Params["base_key_fields"], action.Params["base_key_field"], action.Params["source_key_fields"], action.Params["source_key_field"])),
			ResolutionKeyFields: parseActionStringListParam(firstNonEmptyString(action.Params["resolution_key_fields"], action.Params["resolution_key_field"], action.Params["mapping_key_fields"], action.Params["mapping_key_field"])),
			TargetIDField:       firstNonEmptyString(action.Params["target_id_field"], action.Params["target_field"], action.Params["canonical_target_field"]),
			TargetLabelField:    firstNonEmptyString(action.Params["target_label_field"], action.Params["label_target_field"]),
			TargetStatusField:   firstNonEmptyString(action.Params["target_status_field"], action.Params["status_target_field"]),
			CanonicalIDField:    firstNonEmptyString(action.Params["canonical_id_field"], "canonical_id"),
			CanonicalLabelField: firstNonEmptyString(action.Params["canonical_label_field"], "canonical_label"),
			StatusField:         firstNonEmptyString(action.Params["status_field"], "status"),
			AcceptedStatuses:    parseActionStringListParam(action.Params["accepted_statuses"]),
		})
	}
	return specs, nil
}

func firstNonBaseActionPath(paths []string, basePath string) string {
	base := normalizeMaterialPath(basePath)
	for _, path := range paths {
		rel := normalizeMaterialPath(path)
		if rel == "" || rel == base {
			continue
		}
		return rel
	}
	return ""
}

func applyResolutionSpecFromMap(value map[string]any) (applyResolutionSpec, error) {
	genericFilters, err := applyResolutionSpecFilters(value, "filters")
	if err != nil {
		return applyResolutionSpec{}, err
	}
	baseFilters, err := applyResolutionSpecFilters(value, "base_filters", "record_filters", "source_filters")
	if err != nil {
		return applyResolutionSpec{}, err
	}
	resolutionFilters, err := applyResolutionSpecFilters(value, "resolution_filters", "mapping_filters", "reference_filters", "lookup_filters")
	if err != nil {
		return applyResolutionSpec{}, err
	}
	return applyResolutionSpec{
		BasePath:             applyResolutionSpecString(value, "base_path", "record_path", "input_path", "source_path"),
		ResolutionPath:       applyResolutionSpecString(value, "resolution_path", "mapping_path", "lookup_path", "path"),
		BaseKeyFields:        parseActionStringListParamFromAny(firstApplyResolutionSpecAny(value, "base_key_fields", "base_key_field", "source_key_fields", "source_key_field")),
		ResolutionKeyFields:  parseActionStringListParamFromAny(firstApplyResolutionSpecAny(value, "resolution_key_fields", "resolution_key_field", "mapping_key_fields", "mapping_key_field")),
		GenericFilters:       genericFilters,
		BaseFilters:          baseFilters,
		ResolutionFilters:    resolutionFilters,
		TargetIDField:        applyResolutionSpecString(value, "target_id_field", "target_field", "canonical_target_field"),
		TargetLabelField:     applyResolutionSpecString(value, "target_label_field", "label_target_field"),
		TargetStatusField:    applyResolutionSpecString(value, "target_status_field", "status_target_field"),
		CanonicalIDField:     applyResolutionSpecString(value, "canonical_id_field"),
		CanonicalLabelField:  applyResolutionSpecString(value, "canonical_label_field"),
		StatusField:          applyResolutionSpecString(value, "status_field"),
		AcceptedStatuses:     parseActionStringListParamFromAny(firstApplyResolutionSpecAny(value, "accepted_statuses")),
		UnmatchedStatusValue: applyResolutionSpecString(value, "unmatched_status", "default_status"),
	}, nil
}

func applyResolutionSpecFilters(value map[string]any, keys ...string) ([]contributionFilter, error) {
	for _, key := range keys {
		raw, ok := value[key]
		if !ok || raw == nil {
			continue
		}
		filters, err := parseActionFiltersFromAny(raw)
		if err != nil {
			return nil, fmt.Errorf("parse apply_entity_resolutions %s: %w", key, err)
		}
		return filters, nil
	}
	return nil, nil
}

func parseActionFiltersFromAny(value any) ([]contributionFilter, error) {
	var raw string
	switch typed := value.(type) {
	case string:
		raw = strings.TrimSpace(typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		raw = string(encoded)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	filters, err := parseContributionFiltersRaw(raw)
	if err != nil {
		return nil, err
	}
	for i := range filters {
		filters[i].Field = strings.TrimSpace(filters[i].Field)
		filters[i].Op = strings.ToLower(strings.TrimSpace(filters[i].Op))
		filters[i].Value = strings.TrimSpace(filters[i].Value)
		if filters[i].Field == "" {
			return nil, fmt.Errorf("filter %d has empty field", i)
		}
		if filters[i].Op == "" {
			filters[i].Op = "eq"
		}
	}
	return filters, nil
}

func resolveApplyResolutionBasePath(topLevelBasePath string, paths []string, specs []applyResolutionSpec) (string, error) {
	basePath := normalizeMaterialPath(topLevelBasePath)
	specBasePath := ""
	for i, spec := range specs {
		candidate := normalizeMaterialPath(spec.BasePath)
		if candidate == "" {
			continue
		}
		if specBasePath == "" {
			specBasePath = candidate
			continue
		}
		if specBasePath != candidate {
			return "", fmt.Errorf("apply_entity_resolutions specs use multiple base_path values: %s and %s", specBasePath, candidate)
		}
		if basePath != "" && basePath != candidate {
			return "", fmt.Errorf("apply_entity_resolutions spec %d base_path %s conflicts with top-level base_path %s", i+1, candidate, basePath)
		}
	}
	if basePath == "" {
		basePath = specBasePath
	}
	if basePath == "" && len(paths) > 0 {
		basePath = normalizeMaterialPath(paths[0])
	}
	return basePath, nil
}

func finalizeApplyResolutionSpecs(specs []applyResolutionSpec, paths []string, basePath string) ([]applyResolutionSpec, error) {
	defaultResolutionPaths := nonBaseEnrichPaths(paths, basePath)
	for i := range specs {
		specs[i] = normalizeApplyResolutionSpec(specs[i])
		if specs[i].BasePath == "" {
			specs[i].BasePath = normalizeMaterialPath(basePath)
		}
		if specs[i].ResolutionPath == "" && i < len(defaultResolutionPaths) {
			specs[i].ResolutionPath = defaultResolutionPaths[i]
		}
		if specs[i].ResolutionPath == "" {
			return nil, fmt.Errorf("apply_entity_resolutions spec %d requires resolution_path", i+1)
		}
		if specs[i].TargetIDField == "" {
			if len(specs) == 1 {
				specs[i].TargetIDField = "canonical_id"
			} else {
				return nil, fmt.Errorf("apply_entity_resolutions spec %d requires target_id_field when multiple resolution sets are applied", i+1)
			}
		}
		if specs[i].TargetStatusField == "" {
			specs[i].TargetStatusField = specs[i].TargetIDField + "_status"
		}
	}
	return specs, nil
}

func applyResolutionSpecString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := value[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		default:
			text := strings.TrimSpace(fmt.Sprint(typed))
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func firstApplyResolutionSpecAny(value map[string]any, keys ...string) any {
	for _, key := range keys {
		raw, ok := value[key]
		if ok && raw != nil {
			return raw
		}
	}
	return nil
}

func normalizeApplyResolutionSpec(spec applyResolutionSpec) applyResolutionSpec {
	spec.BasePath = normalizeMaterialPath(spec.BasePath)
	spec.ResolutionPath = normalizeMaterialPath(spec.ResolutionPath)
	spec.BaseKeyFields = cleanStringSlice(spec.BaseKeyFields)
	spec.ResolutionKeyFields = cleanStringSlice(spec.ResolutionKeyFields)
	spec.TargetIDField = strings.TrimSpace(spec.TargetIDField)
	spec.TargetLabelField = strings.TrimSpace(spec.TargetLabelField)
	spec.TargetStatusField = strings.TrimSpace(spec.TargetStatusField)
	spec.CanonicalIDField = firstNonEmptyString(spec.CanonicalIDField, "canonical_id")
	spec.CanonicalLabelField = firstNonEmptyString(spec.CanonicalLabelField, "canonical_label")
	spec.StatusField = firstNonEmptyString(spec.StatusField, "status")
	spec.AcceptedStatuses = cleanStringSlice(spec.AcceptedStatuses)
	if len(spec.AcceptedStatuses) == 0 {
		spec.AcceptedStatuses = []string{"resolved", "matched"}
	}
	spec.UnmatchedStatusValue = firstNonEmptyString(spec.UnmatchedStatusValue, "unmatched")
	return spec
}

func normalizeApplyResolutionSpecForAvailableFields(spec applyResolutionSpec, headers []string, records []actionRecord) applyResolutionSpec {
	if len(spec.ResolutionKeyFields) == 0 {
		return spec
	}
	fields := actionRecordFieldNames(headers, records)
	out := append([]string(nil), spec.ResolutionKeyFields...)
	for i, field := range out {
		if actionRecordFieldExists(fields, field) {
			continue
		}
		if replacement := equivalentResolutionLocatorField(fields, field); replacement != "" {
			out[i] = replacement
		}
	}
	spec.ResolutionKeyFields = cleanStringSlice(out)
	return spec
}

func equivalentResolutionLocatorField(fields []string, requested string) string {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "item_id", "source_locator", "_source_locator", "record_id", "row_id", "id":
	default:
		return ""
	}
	for _, candidate := range []string{"item_id", "source_locator", "_source_locator", "record_id", "row_id", "id"} {
		if actionRecordFieldExists(fields, candidate) {
			return candidate
		}
	}
	return ""
}

func applyResolutionBaseRecordsFiltered(records []actionRecord, specs []applyResolutionSpec) []actionRecord {
	var filters []contributionFilter
	for _, spec := range specs {
		filters = append(filters, spec.BaseFilters...)
	}
	if len(filters) == 0 {
		return records
	}
	return filterActionRecords(records, filters)
}

func applyResolutionPreserveBaseRows(action DataAction) bool {
	rawMode := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		action.Params["base_filter_mode"],
		action.Params["source_filter_mode"],
		action.Params["record_filter_mode"],
		action.Params["apply_filter_mode"],
		action.Params["output_mode"],
	)))
	switch rawMode {
	case "filter", "filtered", "filter_output", "output_filter", "matched_only", "eligible_only":
		return false
	case "preserve", "preserve_base", "preserve_base_rows", "annotate", "apply_only":
		return true
	}
	if strings.TrimSpace(action.Params["preserve_base_rows"]) != "" {
		return parseBoolActionParam(action.Params["preserve_base_rows"])
	}
	if strings.TrimSpace(action.Params["preserve_unmatched"]) != "" {
		return parseBoolActionParam(action.Params["preserve_unmatched"])
	}
	return true
}

func applyResolutionBaseFilterModeLabel(preserve bool) string {
	if preserve {
		return "preserve_base_rows"
	}
	return "filter_output_rows"
}

func baseRecordApplyResolutionKey(record actionRecord, spec applyResolutionSpec) string {
	if key := actionRecordCompositeKey(record, spec.BaseKeyFields); key != "" {
		return key
	}
	for _, field := range []string{"_source_index", "source_index", "_source_line", "source_line"} {
		if value := recordField(record.Fields, field); value != "" {
			return value
		}
	}
	if record.Index > 0 {
		return strconv.Itoa(record.Index)
	}
	if record.Line > 0 {
		return strconv.Itoa(record.Line)
	}
	return ""
}

func resolutionRecordApplyKey(record actionRecord, spec applyResolutionSpec) string {
	if key := actionRecordCompositeKey(record, spec.ResolutionKeyFields); key != "" {
		if locatorKey := resolutionLocatorKeyFromExplicitFields(record, spec.ResolutionKeyFields); locatorKey != "" {
			return locatorKey
		}
		return key
	}
	for _, field := range []string{"source_index", "_source_index", "source_line", "_source_line", "line", "row_index", "row"} {
		if value := recordField(record.Fields, field); value != "" {
			return value
		}
	}
	for _, field := range []string{"item_id", "source_locator", "record_id", "row_id", "id"} {
		if value := recordField(record.Fields, field); value != "" {
			if key := sourceIndexFromResolutionLocator(value); key != "" {
				return key
			}
		}
	}
	return ""
}

func resolutionLocatorKeyFromExplicitFields(record actionRecord, fields []string) string {
	fields = cleanStringSlice(fields)
	if len(fields) != 1 {
		return ""
	}
	field := strings.ToLower(strings.TrimSpace(fields[0]))
	switch field {
	case "item_id", "source_locator", "_source_locator", "record_id", "row_id", "id":
	default:
		return ""
	}
	return sourceIndexFromResolutionLocator(recordField(record.Fields, fields[0]))
}

func actionRecordCompositeKey(record actionRecord, fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		value := recordField(record.Fields, field)
		if value == "" {
			return ""
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, "\x1f")
}

var sourceIndexLocatorRE = regexp.MustCompile(`#([0-9]+)(?::|$)`)

func sourceIndexFromResolutionLocator(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	matches := sourceIndexLocatorRE.FindStringSubmatch(value)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func filterAcceptedResolutionChoices(choices []resolutionChoice, spec applyResolutionSpec) []resolutionChoice {
	if len(choices) == 0 {
		return nil
	}
	accepted := lowerStringSet(spec.AcceptedStatuses)
	var out []resolutionChoice
	seenCanonical := map[string]bool{}
	for _, choice := range choices {
		status := strings.ToLower(strings.TrimSpace(choice.status))
		if len(accepted) > 0 && !accepted[status] {
			continue
		}
		canonicalID := recordField(choice.rec.Fields, spec.CanonicalIDField)
		canonicalLabel := recordField(choice.rec.Fields, spec.CanonicalLabelField)
		if canonicalID == "" && canonicalLabel == "" {
			continue
		}
		key := canonicalID + "\x00" + canonicalLabel
		if seenCanonical[key] {
			continue
		}
		seenCanonical[key] = true
		out = append(out, choice)
	}
	return out
}

func (r ActionRunner) runEnrichRecords(action DataAction) (DataArtifact, []map[string]any, []EntityResolutionRecord, []string, error) {
	paths := cleanStringListPreserveOrder(action.InputPaths)
	basePath := firstNonEmptyString(strings.TrimSpace(action.Params["base_path"]), strings.TrimSpace(action.Params["record_path"]))
	if basePath == "" && len(paths) > 0 {
		basePath = paths[0]
	}
	if basePath == "" {
		return DataArtifact{}, nil, nil, nil, errors.New("enrich_records requires a base_path or at least one input_path")
	}
	specs, err := parseEnrichRecordSpecs(action, paths, basePath)
	if err != nil {
		return DataArtifact{}, nil, nil, nil, err
	}
	if len(specs) == 0 {
		return DataArtifact{}, nil, nil, nil, errors.New("enrich_records requires at least one mapping spec")
	}
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxOutput := actionIntParam(action, "max_output_records", 100000, 1, 1000000)
	baseRecords, baseHeaders, baseTotal, baseRel, err := r.readActionRecords(basePath, maxRecords)
	if err != nil {
		return DataArtifact{}, nil, nil, nil, err
	}
	baseFields := actionRecordFieldNames(baseHeaders, baseRecords)
	rows := make([]map[string]any, 0, minInt(maxOutput, len(baseRecords)))
	consumed := []string{baseRel}
	children := make([]DataArtifact, 0, len(specs)+1)
	lookups := make([]map[string][]enrichCandidate, 0, len(specs))
	for i, spec := range specs {
		records, headers, total, rel, err := r.readActionRecords(spec.MappingPath, maxRecords)
		if err != nil {
			return DataArtifact{}, nil, nil, nil, err
		}
		mappingFields := actionRecordFieldNames(headers, records)
		if len(baseFields) > 0 && !actionRecordAnyFieldExists(baseFields, spec.SourceFields) {
			if len(specs) == 1 && shouldSwapEnrichBaseAndMapping(spec, baseFields, mappingFields) {
				oldBaseRecords, oldBaseHeaders, oldBaseTotal, oldBaseRel, oldBaseFields := baseRecords, baseHeaders, baseTotal, baseRel, baseFields
				spec.MappingPath = oldBaseRel
				baseRecords, baseHeaders, baseTotal, baseRel, baseFields = records, headers, total, rel, mappingFields
				records, headers, total, rel, mappingFields = oldBaseRecords, oldBaseHeaders, oldBaseTotal, oldBaseRel, oldBaseFields
				if !actionRecordFieldExists(mappingFields, spec.MappingValueField) {
					if inferred := inferEnrichMappingValueField(mappingFields, spec.MappingSourceFields, spec.TargetField); inferred != "" {
						spec.MappingValueField = inferred
					}
				}
			}
			if !actionRecordAnyFieldExists(baseFields, spec.SourceFields) {
				return DataArtifact{}, nil, nil, nil, fmt.Errorf("enrich_records source_fields [%s] were not found in base input %s fields [%s]", strings.Join(spec.SourceFields, ", "), baseRel, strings.Join(clampStringSliceForError(baseFields, 24), ", "))
			}
		}
		if len(mappingFields) > 0 && !actionRecordFieldExists(mappingFields, spec.MappingValueField) {
			if inferred := inferEnrichMappingValueField(mappingFields, spec.MappingSourceFields, spec.TargetField); inferred != "" {
				spec.MappingValueField = inferred
			}
			if !actionRecordFieldExists(mappingFields, spec.MappingValueField) {
				return DataArtifact{}, nil, nil, nil, fmt.Errorf("enrich_records mapping_value_field %q was not found in mapping input %s fields [%s]", spec.MappingValueField, rel, strings.Join(clampStringSliceForError(mappingFields, 24), ", "))
			}
		}
		if len(mappingFields) > 0 && !actionRecordAnyFieldExists(mappingFields, spec.MappingSourceFields) {
			if inferred := inferEnrichMappingSourceFields(mappingFields, spec.MappingValueField, spec.TargetField); len(inferred) > 0 {
				spec.MappingSourceFields = inferred
			}
			if !actionRecordAnyFieldExists(mappingFields, spec.MappingSourceFields) {
				return DataArtifact{}, nil, nil, nil, fmt.Errorf("enrich_records mapping_source_fields [%s] were not found in mapping input %s fields [%s]", strings.Join(spec.MappingSourceFields, ", "), rel, strings.Join(clampStringSliceForError(mappingFields, 24), ", "))
			}
		}
		effectiveMappingFilters, ignoredMappingFilters := effectiveActionFiltersForRecords(spec.MappingFilters, headers, records)
		spec.MappingFilters = effectiveMappingFilters
		specs[i] = spec
		consumed = append(consumed, rel)
		lookup := buildEnrichLookup(records, rel, spec)
		fields := map[string]string{
			"source_fields":         strings.Join(spec.SourceFields, ","),
			"mapping_source_fields": strings.Join(spec.MappingSourceFields, ","),
			"mapping_value_field":   spec.MappingValueField,
			"target_field":          spec.TargetField,
			"match_mode":            spec.MatchMode,
		}
		if len(ignoredMappingFilters) > 0 {
			fields["ignored_filter_fields"] = strings.Join(ignoredMappingFilters, ",")
		}
		lookups = append(lookups, lookup)
		children = append(children, DataArtifact{
			ID:          rel + "#mapping",
			Kind:        "enrich_records/mapping",
			SourcePaths: []string{rel},
			Headers:     headers,
			RowCount:    total,
			Summary:     fmt.Sprintf("%s supplied %d mapping candidate(s) for target field %s", rel, len(flattenEnrichLookupCandidates(lookup)), spec.TargetField),
			Fields:      fields,
		})
	}
	children = append([]DataArtifact{{
		ID:          baseRel + "#base",
		Kind:        "enrich_records/base",
		SourcePaths: []string{baseRel},
		Headers:     baseHeaders,
		RowCount:    baseTotal,
		Summary:     fmt.Sprintf("%s supplied %d base record(s)", baseRel, baseTotal),
	}}, children...)
	matchesByTarget := map[string]int{}
	var resolutions []EntityResolutionRecord
	seenResolution := map[string]bool{}
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
			value, status, evidence := selectEnrichValueFromRecord(base, lookups[i], spec)
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
			sourceValue := firstSourceValueFromRecord(base, spec.SourceFields)
			if sourceValue != "" || value != "" || status != "" {
				itemID := fmt.Sprintf("%s#%d:%s", baseRel, base.Index, spec.TargetField)
				key := itemID + "\x00" + sourceValue + "\x00" + value + "\x00" + status
				if !seenResolution[key] {
					seenResolution[key] = true
					evidenceRefs := []string{fmt.Sprintf("%s:%d", baseRel, base.Line)}
					if strings.TrimSpace(evidence) != "" {
						evidenceRefs = append(evidenceRefs, evidence)
					}
					resolutions = append(resolutions, EntityResolutionRecord{
						ItemID:         LooseText(itemID),
						SourceValue:    LooseText(sourceValue),
						CanonicalID:    LooseText(value),
						CanonicalLabel: LooseText(value),
						Status:         LooseText(firstNonEmptyString(status, "unmatched")),
						EvidenceRefs:   cleanStringList(evidenceRefs),
						Reason:         LooseText(firstNonEmptyString(strings.TrimSpace(action.Purpose), "enriched by reference records")),
					})
				}
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
	}, rows, resolutions, normalizeMaterialPaths(consumed), nil
}

func firstSourceValueFromRecord(record actionRecord, fields []string) string {
	for _, field := range fields {
		if value := recordField(record.Fields, field); value != "" {
			return value
		}
	}
	return ""
}

func parseEnrichRecordSpecs(action DataAction, paths []string, basePath string) ([]enrichRecordSpec, error) {
	if raw := strings.TrimSpace(firstNonEmptyString(
		structuredActionParam(action.Params, "mapping_specs"),
		action.Params["mapping_specs_json"],
		structuredActionParam(action.Params, "map_specs"),
		action.Params["map_specs_json"],
		structuredActionParam(action.Params, "enrich_specs"),
		action.Params["enrich_specs_json"],
		structuredActionParam(action.Params, "lookup_specs"),
		action.Params["lookup_specs_json"],
	)); raw != "" {
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
		SourceFields:        parseActionStringListParam(firstNonEmptyString(action.Params["source_fields"], action.Params["base_fields"], action.Params["left_fields"], action.Params["key_fields"])),
		MappingSourceFields: parseActionStringListParam(firstNonEmptyString(action.Params["mapping_source_fields"], action.Params["mapping_source_field"], action.Params["reference_fields"], action.Params["reference_field"], action.Params["lookup_source_fields"], action.Params["lookup_source_field"], action.Params["lookup_fields"], action.Params["lookup_field"])),
		MappingValueField:   firstNonEmptyString(action.Params["mapping_value_field"], action.Params["reference_value_field"], action.Params["lookup_value_field"], action.Params["canonical_id_field"], action.Params["value_field"]),
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
	if raw := strings.TrimSpace(firstNonEmptyString(structuredActionParam(action.Params, "mapping_filters"), action.Params["mapping_filters_json"])); raw != "" {
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
		SourceFields:        getList("source_fields", "base_fields", "left_fields", "key_fields"),
		MappingSourceFields: getList("mapping_source_fields", "mapping_source_field", "reference_fields", "reference_field", "lookup_source_fields", "lookup_source_field", "lookup_fields", "lookup_field"),
		MappingValueField:   getString("mapping_value_field", "reference_value_field", "lookup_value_field", "canonical_id_field", "value_field"),
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
	if value == nil {
		return nil
	}
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
	spec.SourceFields = cleanStringSlice(spec.SourceFields)
	if spec.SourceField == "" && len(spec.SourceFields) > 0 {
		spec.SourceField = spec.SourceFields[0]
	}
	if len(spec.SourceFields) == 0 && spec.SourceField != "" {
		spec.SourceFields = []string{spec.SourceField}
	}
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
	if len(spec.SourceFields) == 0 {
		return errors.New("enrich_records mapping spec requires source_field/base_field or source_fields/base_fields")
	}
	if len(spec.MappingSourceFields) == 0 {
		return errors.New("enrich_records mapping spec requires mapping_source_field/reference_field")
	}
	if spec.MappingValueField == "" || spec.TargetField == "" {
		return errors.New("enrich_records mapping spec requires mapping_value_field and target_field")
	}
	return nil
}

func shouldSwapEnrichBaseAndMapping(spec enrichRecordSpec, baseFields, mappingFields []string) bool {
	if actionRecordAnyFieldExists(baseFields, spec.SourceFields) {
		return false
	}
	if !actionRecordAnyFieldExists(mappingFields, spec.SourceFields) {
		return false
	}
	if !actionRecordAnyFieldExists(baseFields, spec.MappingSourceFields) {
		return false
	}
	if actionRecordFieldExists(baseFields, spec.MappingValueField) {
		return true
	}
	return inferEnrichMappingValueField(baseFields, spec.MappingSourceFields, spec.TargetField) != ""
}

func inferEnrichMappingValueField(fields, sourceFields []string, targetField string) string {
	source := map[string]bool{}
	for _, field := range sourceFields {
		source[strings.ToLower(strings.TrimSpace(field))] = true
	}
	usable := func(field string) bool {
		field = strings.TrimSpace(field)
		if field == "" {
			return false
		}
		lower := strings.ToLower(field)
		return !strings.HasPrefix(lower, "_") && !source[lower]
	}
	findExact := func(names ...string) string {
		for _, name := range names {
			want := strings.ToLower(strings.TrimSpace(name))
			if want == "" {
				continue
			}
			for _, field := range fields {
				if usable(field) && strings.ToLower(strings.TrimSpace(field)) == want {
					return strings.TrimSpace(field)
				}
			}
		}
		return ""
	}
	if field := findExact(targetField, "canonical_id", "canonical_label", "target_value", "value", "code", "id", "label"); field != "" {
		return field
	}
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		lower := strings.ToLower(trimmed)
		if !usable(trimmed) {
			continue
		}
		if strings.Contains(lower, "canonical") && (strings.Contains(lower, "id") || strings.Contains(lower, "code") || strings.Contains(lower, "value") || strings.Contains(lower, "label")) {
			return trimmed
		}
	}
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		lower := strings.ToLower(trimmed)
		if !usable(trimmed) {
			continue
		}
		if strings.HasSuffix(lower, "_id") || strings.HasSuffix(lower, "_code") || strings.HasSuffix(lower, "_value") || strings.HasSuffix(lower, "_label") {
			return trimmed
		}
	}
	return ""
}

func inferEnrichMappingSourceFields(fields []string, valueField, targetField string) []string {
	valueLower := strings.ToLower(strings.TrimSpace(valueField))
	targetLower := strings.ToLower(strings.TrimSpace(targetField))
	var out []string
	seen := map[string]bool{}
	add := func(field string) {
		field = strings.TrimSpace(field)
		key := strings.ToLower(field)
		if field == "" || seen[key] || strings.HasPrefix(key, "_") || key == valueLower {
			return
		}
		seen[key] = true
		out = append(out, field)
	}
	score := func(field string) int {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || lower == valueLower {
			return -1
		}
		if strings.Contains(lower, "negative") || strings.Contains(lower, "exclude") || strings.Contains(lower, "except") {
			return -1
		}
		switch {
		case lower == "source_value" || lower == "source" || lower == "term" || lower == "terms":
			return 100
		case strings.Contains(lower, "source") || strings.Contains(lower, "term") || strings.Contains(lower, "keyword") || strings.Contains(lower, "alias") || strings.Contains(lower, "example"):
			return 90
		case strings.Contains(lower, "name") || strings.Contains(lower, "label") || strings.Contains(lower, "description") || strings.Contains(lower, "text") || strings.Contains(lower, "scope"):
			return 70
		case targetLower != "" && strings.Contains(lower, targetLower) && lower != targetLower:
			return 50
		default:
			return 10
		}
	}
	type candidate struct {
		field string
		score int
		index int
	}
	var candidates []candidate
	for i, field := range fields {
		s := score(field)
		if s < 0 {
			continue
		}
		candidates = append(candidates, candidate{field: field, score: s, index: i})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].index < candidates[j].index
	})
	for _, candidate := range candidates {
		add(candidate.field)
		if len(out) >= 8 {
			break
		}
	}
	return out
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
			for _, indexedSource := range enrichLookupSourceTerms(source) {
				candidate := enrichCandidate{
					Source:   indexedSource,
					Value:    value,
					Evidence: fmt.Sprintf("%s:%d", rel, record.Line),
					Weight:   len([]rune(indexedSource)),
				}
				key := normalizeEnrichKey(indexedSource, spec)
				out[key] = append(out[key], candidate)
			}
		}
	}
	for key := range out {
		sort.SliceStable(out[key], func(i, j int) bool {
			return out[key][i].Weight > out[key][j].Weight
		})
	}
	return out
}

func enrichLookupSourceTerms(source string) []string {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, value)
	}
	add(source)
	for _, term := range regexp.MustCompile(`[;；,，|/\n\r]+`).Split(source, -1) {
		add(term)
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

func selectEnrichValueFromRecord(record actionRecord, lookup map[string][]enrichCandidate, spec enrichRecordSpec) (value, status, evidence string) {
	bestStatus := "unmatched"
	for _, field := range spec.SourceFields {
		sourceValue := recordField(record.Fields, field)
		value, status, evidence = selectEnrichValue(sourceValue, lookup, spec)
		switch status {
		case "matched":
			return value, status, evidence
		case "matched_ambiguous":
			if value != "" {
				return value, status, evidence
			}
			if bestStatus == "unmatched" {
				bestStatus = status
			}
		case "ambiguous", "unmatched_required", "missing_source":
			if bestStatus == "unmatched" {
				bestStatus = status
			}
		}
	}
	if spec.Required && bestStatus == "unmatched" {
		bestStatus = "unmatched_required"
	}
	return "", bestStatus, ""
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
	paths := cleanActionPathList(action.InputPaths)
	leftPath := firstNonEmptyString(strings.TrimSpace(action.Params["left_path"]), strings.TrimSpace(action.Params["left"]))
	rightPath := firstNonEmptyString(strings.TrimSpace(action.Params["right_path"]), strings.TrimSpace(action.Params["right"]))
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
	if leftPath == "" || rightPath == "" {
		inferredLeft, inferredRight, err := r.inferJoinRecordPaths(paths, leftPath, rightPath, leftFields, rightFields, maxRecords)
		if err != nil {
			return DataArtifact{}, nil, nil, err
		}
		leftPath, rightPath = inferredLeft, inferredRight
	}
	if leftPath == "" || rightPath == "" {
		return DataArtifact{}, nil, nil, errors.New("join_records requires left_path/right_path or at least two input_paths")
	}
	leftRecords, leftHeaders, leftTotal, leftRel, err := r.readActionRecords(leftPath, maxRecords)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	rightRecords, rightHeaders, rightTotal, rightRel, err := r.readActionRecords(rightPath, maxRecords)
	if err != nil {
		return DataArtifact{}, nil, nil, err
	}
	leftKnownFields := map[string]bool{}
	rightKnownFields := map[string]bool{}
	markKnownActionFields(leftKnownFields, leftHeaders, leftRecords)
	markKnownActionFields(rightKnownFields, rightHeaders, rightRecords)
	for _, field := range leftFields {
		if !leftKnownFields[strings.ToLower(strings.TrimSpace(field))] {
			return DataArtifact{}, nil, nil, fmt.Errorf("join_records left field %q was not found in left input %s", field, leftRel)
		}
	}
	for _, field := range rightFields {
		if !rightKnownFields[strings.ToLower(strings.TrimSpace(field))] {
			return DataArtifact{}, nil, nil, fmt.Errorf("join_records right field %q was not found in right input %s", field, rightRel)
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
	predictedHeaders := predictedJoinedRecordHeaders(leftHeaders, rightHeaders, leftPrefix, rightPrefix, collision)
	var rows []map[string]any
	matches := 0
	matchedLeftRows := 0
	processedLeftRows := 0
	for _, left := range leftRecords {
		processedLeftRows++
		key := joinActionRecordKey(left.Fields, leftFields)
		matched := rightIndex[key]
		if len(matched) == 0 {
			if joinType == "left" || joinType == "left_outer" {
				row := buildJoinedActionRecord(left, actionRecord{}, leftRel, rightRel, leftPrefix, rightPrefix, collision)
				fillMissingJoinedHeaders(row, predictedHeaders)
				rows = append(rows, row)
				if len(rows) >= maxOutput {
					break
				}
			}
			continue
		}
		matchedLeftRows++
		for _, right := range matched {
			row := buildJoinedActionRecord(left, right, leftRel, rightRel, leftPrefix, rightPrefix, collision)
			fillMissingJoinedHeaders(row, predictedHeaders)
			rows = append(rows, row)
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
	outputHeaders := mergeJoinedRecordHeaders(collectJoinedRecordHeaders(rows), predictedHeaders)
	unmatchedLeftRows := processedLeftRows - matchedLeftRows
	if unmatchedLeftRows < 0 {
		unmatchedLeftRows = 0
	}
	matchRate := "0"
	if processedLeftRows > 0 {
		matchRate = strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", float64(matchedLeftRows)/float64(processedLeftRows)), "0"), ".")
	}
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionJoinRecords),
		SourcePaths: normalizeMaterialPaths([]string{leftRel, rightRel}),
		Headers:     outputHeaders,
		RowCount:    len(rows),
		Summary:     summary,
		Sample:      sampleJoinedActionRows(rows, 3),
		Fields: map[string]string{
			"left_path":           leftRel,
			"right_path":          rightRel,
			"left_rows":           fmt.Sprintf("%d", leftTotal),
			"right_rows":          fmt.Sprintf("%d", rightTotal),
			"joined_rows":         fmt.Sprintf("%d", len(rows)),
			"matches":             fmt.Sprintf("%d", matches),
			"matched_left_rows":   fmt.Sprintf("%d", matchedLeftRows),
			"unmatched_left_rows": fmt.Sprintf("%d", unmatchedLeftRows),
			"match_rate":          matchRate,
			"zero_matches":        fmt.Sprintf("%t", matches == 0),
			"join_type":           joinType,
			"left_fields":         strings.Join(leftFields, ","),
			"right_fields":        strings.Join(rightFields, ","),
			"output_headers":      strings.Join(outputHeaders, ","),
		},
	}, rows, normalizeMaterialPaths([]string{leftRel, rightRel}), nil
}

func cleanActionPathList(in []string) []string {
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
	return out
}

type joinRecordPathProfile struct {
	Path   string
	Rel    string
	Fields map[string]bool
}

func (r ActionRunner) inferJoinRecordPaths(paths []string, explicitLeft, explicitRight string, leftFields, rightFields []string, maxRecords int) (string, string, error) {
	if explicitLeft != "" && explicitRight != "" {
		return explicitLeft, explicitRight, nil
	}
	if len(paths) < 2 && (explicitLeft == "" || explicitRight == "") {
		return "", "", errors.New("join_records requires left_path/right_path or at least two input_paths")
	}
	profiles := make([]joinRecordPathProfile, 0, len(paths))
	for _, p := range paths {
		records, headers, _, rel, err := r.readActionRecords(p, maxRecords)
		if err != nil {
			continue
		}
		fields := map[string]bool{}
		markKnownActionFields(fields, headers, records)
		profiles = append(profiles, joinRecordPathProfile{Path: p, Rel: rel, Fields: fields})
	}
	if len(profiles) == 0 {
		if explicitLeft == "" && len(paths) > 0 {
			explicitLeft = paths[0]
		}
		if explicitRight == "" && len(paths) > 1 {
			explicitRight = paths[1]
		}
		return explicitLeft, explicitRight, nil
	}
	matchesAll := func(profile joinRecordPathProfile, fields []string) bool {
		for _, field := range fields {
			if !profile.Fields[strings.ToLower(strings.TrimSpace(field))] {
				return false
			}
		}
		return true
	}
	left := explicitLeft
	right := explicitRight
	if left == "" {
		for _, profile := range profiles {
			if right != "" && normalizeMaterialPath(profile.Path) == normalizeMaterialPath(right) {
				continue
			}
			if matchesAll(profile, leftFields) {
				left = profile.Path
				break
			}
		}
	}
	if right == "" {
		for _, profile := range profiles {
			if left != "" && normalizeMaterialPath(profile.Path) == normalizeMaterialPath(left) {
				continue
			}
			if matchesAll(profile, rightFields) {
				right = profile.Path
				break
			}
		}
	}
	if left == "" && len(paths) > 0 {
		left = paths[0]
	}
	if right == "" {
		for _, p := range paths {
			if normalizeMaterialPath(p) == normalizeMaterialPath(left) {
				continue
			}
			right = p
			break
		}
	}
	if left == "" || right == "" {
		return "", "", errors.New("join_records requires left_path/right_path or at least two input_paths")
	}
	return left, right, nil
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

func mergeJoinedRecordHeaders(actual, predicted []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(values []string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			key := strings.ToLower(value)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, value)
		}
	}
	add(actual)
	add(predicted)
	sort.Strings(out)
	return out
}

func fillMissingJoinedHeaders(row map[string]any, headers []string) {
	for _, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		if _, ok := row[header]; ok {
			continue
		}
		row[header] = ""
	}
}

func predictedJoinedRecordHeaders(leftHeaders, rightHeaders []string, leftPrefix, rightPrefix, collision string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(prefix, key string, prefer bool) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		target := key
		if strings.TrimSpace(prefix) != "" {
			target = strings.TrimSpace(prefix) + "_" + key
		}
		if seen[target] {
			switch strings.ToLower(strings.TrimSpace(collision)) {
			case "right", "overwrite":
				return
			case "left", "keep_left":
				return
			default:
				collisionKey := strings.TrimSpace(prefix)
				if collisionKey == "" {
					collisionKey = "right"
				}
				target = collisionKey + "_" + key
			}
		}
		if target == "" || seen[target] {
			return
		}
		seen[target] = true
		out = append(out, target)
	}
	for _, header := range leftHeaders {
		add(leftPrefix, header, false)
	}
	for _, header := range rightHeaders {
		add(rightPrefix, header, true)
	}
	for _, header := range []string{"_left_source", "_left_line", "_left_index", "_right_source", "_right_line", "_right_index"} {
		add("", header, false)
	}
	sort.Strings(out)
	return out
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

func (r ActionRunner) runComputeContributions(action DataAction, defaultRuleRefs []string, requireNonEmpty bool) (DataArtifact, []ContributionRecord, []string, error) {
	paths := cleanStringListPreserveOrder(action.InputPaths)
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
	contributionStatusFields := parseActionStringListParam(firstNonEmptyString(action.Params["status_fields"], action.Params["generated_status_fields"]))
	allowUnqualifiedRecords := parseBoolActionParam(action.Params["allow_unqualified_records"])
	filterFields := make([]string, 0, len(filters))
	maxRecords := actionIntParam(action, "max_records", 100000, 1, 1000000)
	maxContribs := actionIntParam(action, "max_contributions", 50000, 1, 200000)

	var contributions []ContributionRecord
	var consumed []string
	children := make([]DataArtifact, 0, len(paths))
	knownFields := map[string]bool{}
	var diagnostics []string
	for _, path := range paths {
		records, headers, total, rel, err := r.readActionRecords(path, maxRecords)
		if err != nil {
			return DataArtifact{}, nil, nil, err
		}
		effectiveGroupKeyField := groupKeyField
		effectiveGroupKeyConst := groupKeyConst
		if effectiveGroupKeyField == "" && effectiveGroupKeyConst != "" && actionRecordFieldExistsInRecords(headers, records, effectiveGroupKeyConst) {
			effectiveGroupKeyField = effectiveGroupKeyConst
			effectiveGroupKeyConst = ""
			diagnostics = append(diagnostics, fmt.Sprintf("%s group_key %q matched an input field; treated as group_key_field", rel, effectiveGroupKeyField))
		}
		effectiveValueField := valueField
		if effectiveValueField == "" && rawMetric != "" && operation != "count" && actionRecordFieldExistsInRecords(headers, records, rawMetric) {
			effectiveValueField = rawMetric
			diagnostics = append(diagnostics, fmt.Sprintf("%s metric %q matched an input field; treated as value_field", rel, effectiveValueField))
		}
		if missing := missingComputeContributionFields(headers, records, effectiveValueField, effectiveGroupKeyField, effectiveGroupKeyConst, operation, filters); len(missing) > 0 && len(paths) > 1 {
			children = append(children, DataArtifact{
				ID:          rel + "#contributions_skipped",
				Kind:        "contribution_source_skipped",
				SourcePaths: []string{rel},
				Headers:     headers,
				RowCount:    total,
				Summary:     fmt.Sprintf("%s skipped for contribution calculation; missing fields [%s]", rel, strings.Join(missing, ", ")),
				Fields: map[string]string{
					"skipped":        "true",
					"missing_fields": strings.Join(missing, ","),
					"total":          fmt.Sprintf("%d", total),
				},
			})
			diagnostics = append(diagnostics, fmt.Sprintf("%s skipped as non-contribution/reference input; missing required contribution field(s) [%s]; fields=[%s]",
				rel,
				strings.Join(missing, ", "),
				strings.Join(clampStringSliceForError(actionRecordFieldNames(headers, records), 32), ", "),
			))
			continue
		}
		markKnownActionFields(knownFields, headers, records)
		effectiveFilters, ignoredFilterFields := effectiveActionFiltersForRecords(filters, headers, records)
		filterDiagnostics := filterDiagnosticsJSON(records, effectiveFilters, 8, 4)
		effectiveStatusFields := contributionStatusFields
		if len(effectiveStatusFields) == 0 {
			effectiveStatusFields = generatedStatusFields(headers, records)
		}
		statusFilterFields := lowerStringSet(filterFieldNames(effectiveFilters))
		blockedStatusValues := defaultGeneratedBlockingStatusValues()
		if len(ignoredFilterFields) > 0 {
			fields := actionRecordFilterFieldNames(headers, records)
			return DataArtifact{}, nil, nil, fmt.Errorf("compute_contributions filter field(s) [%s] were not found in input %s fields [%s]; field_samples=%s; filter_diagnostics=%s",
				strings.Join(ignoredFilterFields, ", "),
				rel,
				strings.Join(clampStringSliceForError(fields, 32), ", "),
				compactFieldSampleJSON(records, append(append(filterFieldNames(filters), valueField), effectiveGroupKeyField), 8, 3),
				filterDiagnostics,
			)
		}
		for _, filter := range effectiveFilters {
			filterFields = append(filterFields, filter.Field)
		}
		consumed = append(consumed, rel)
		matched := 0
		filterMatched := 0
		missingValue := 0
		var blockedStatusSamples []string
		for _, record := range records {
			if !recordPassesFilters(record.Fields, effectiveFilters) {
				continue
			}
			filterMatched++
			if len(contributions) >= maxContribs {
				return DataArtifact{}, nil, nil, fmt.Errorf("compute_contributions exceeded max_contributions=%d; split the action into smaller groups or filters", maxContribs)
			}
			value := ""
			if effectiveValueField != "" {
				value = recordField(record.Fields, effectiveValueField)
				if value == "" && operation != "count" {
					missingValue++
					continue
				}
			}
			if value == "" && operation == "count" {
				value = "1"
			}
			if !allowUnqualifiedRecords && requireNonEmpty && strings.EqualFold(role, "target") && len(ruleRefs) > 0 {
				if issues := contributionBlockedGeneratedStatusIssues(record, effectiveStatusFields, statusFilterFields, blockedStatusValues); len(issues) > 0 {
					if len(blockedStatusSamples) < 8 {
						blockedStatusSamples = append(blockedStatusSamples, strings.Join(issues, ","))
					}
					continue
				}
			}
			matched++
			itemID := recordField(record.Fields, itemIDField)
			if itemID == "" {
				itemID = fmt.Sprintf("%s#%d", rel, record.Index)
			}
			groupKey := firstNonEmptyString(recordField(record.Fields, effectiveGroupKeyField), effectiveGroupKeyConst, "all")
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
		fields := map[string]string{
			"matched":            fmt.Sprintf("%d", matched),
			"filter_matched":     fmt.Sprintf("%d", filterMatched),
			"missing_value":      fmt.Sprintf("%d", missingValue),
			"total":              fmt.Sprintf("%d", total),
			"filter_diagnostics": filterDiagnostics,
		}
		if len(effectiveStatusFields) > 0 {
			fields["status_fields"] = strings.Join(effectiveStatusFields, ",")
		}
		if len(blockedStatusSamples) > 0 {
			fields["blocked_status_rows"] = fmt.Sprintf("%d", len(blockedStatusSamples))
			fields["blocked_status_samples"] = strings.Join(blockedStatusSamples, "; ")
			return DataArtifact{}, nil, nil, fmt.Errorf("compute_contributions found rule-qualified target contribution rows with unresolved generated status fields in input %s: %s. Add a qualify_records or filter_records batch to produce explicit include/exclude decisions, or resolve/materialize the evidence fields before computing contributions",
				rel,
				strings.Join(blockedStatusSamples, "; "),
			)
		}
		if effectiveValueField != "" && valueField == "" && rawMetric != "" && effectiveValueField == rawMetric {
			fields["value_field_inferred_from"] = "metric"
		}
		if effectiveGroupKeyField != "" && groupKeyField == "" && groupKeyConst != "" {
			fields["group_key_field_inferred_from"] = "group_key"
		}
		if len(ignoredFilterFields) > 0 {
			fields["ignored_filter_fields"] = strings.Join(ignoredFilterFields, ",")
		}
		children = append(children, DataArtifact{
			ID:          rel + "#contributions",
			Kind:        "contribution_source",
			SourcePaths: []string{rel},
			Headers:     headers,
			RowCount:    total,
			Summary:     fmt.Sprintf("%s produced %d contribution(s) from %d record(s)", rel, matched, total),
			Fields:      fields,
		})
		diagnostics = append(diagnostics, fmt.Sprintf("%s total=%d filter_matched=%d contributions=%d missing_value=%d field_samples=%s filter_diagnostics=%s",
			rel,
			total,
			filterMatched,
			matched,
			missingValue,
			compactFieldSampleJSON(records, append(append(filterFieldNames(effectiveFilters), effectiveValueField), effectiveGroupKeyField), 8, 3),
			filterDiagnostics,
		))
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
		if requireNonEmpty {
			return DataArtifact{}, nil, nil, fmt.Errorf("compute_contributions produced zero contribution records while contribution ledger is required; diagnostics=[%s]; check filter field names, filter values, value_field, and group_key_field against the input artifact fields",
				strings.Join(diagnostics, "; "))
		}
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

func actionRecordFieldExistsInRecords(headers []string, records []actionRecord, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if len(records) > 0 {
		for _, record := range records {
			for key := range record.Fields {
				if strings.ToLower(strings.TrimSpace(key)) == name {
					return true
				}
			}
		}
		return false
	}
	for _, header := range headers {
		if strings.ToLower(strings.TrimSpace(header)) == name {
			return true
		}
	}
	return false
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

func missingComputeContributionFields(headers []string, records []actionRecord, valueField, groupKeyField, groupKeyConst, operation string, filters []contributionFilter) []string {
	var missing []string
	if strings.TrimSpace(valueField) != "" && operation != "count" && !actionRecordFieldExistsInRecords(headers, records, valueField) {
		missing = append(missing, valueField)
	}
	if strings.TrimSpace(groupKeyField) != "" && strings.TrimSpace(groupKeyConst) == "" && !actionRecordFieldExistsInRecords(headers, records, groupKeyField) {
		missing = append(missing, groupKeyField)
	}
	for _, filter := range filters {
		field := strings.TrimSpace(filter.Field)
		if field != "" && !actionRecordFieldExistsInRecords(headers, records, field) {
			missing = append(missing, field)
		}
	}
	return cleanStringList(missing)
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

func (r ActionRunner) runAssembleAnswer(action DataAction, reconcile *ReconcileReport, contract OutputContract, artifacts []DataArtifact) (DataArtifact, string, *ReconcileReport, error) {
	if reconcile == nil {
		return DataArtifact{}, "", nil, errors.New("assemble_answer requires prior reconcile report")
	}
	if len(reconcile.Groups) == 0 {
		return DataArtifact{}, "", nil, errors.New("assemble_answer requires reconcile groups")
	}
	contract = contract.Normalize()
	groups := append([]ReconcileGroup(nil), reconcile.Groups...)
	groups = r.completeAssembleAnswerGroups(action, contract, artifacts, groups)
	valueField := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action.Params["value_field"], "actual")))
	orderBy := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action.Params["order_by"], "group_key")))
	switch orderBy {
	case "input", "none", "stable":
	case "metric":
		sort.SliceStable(groups, func(i, j int) bool {
			return naturalLess(groups[i].Metric.String()+"\x00"+groups[i].GroupKey.String(), groups[j].Metric.String()+"\x00"+groups[j].GroupKey.String())
		})
	case "value":
		sort.SliceStable(groups, func(i, j int) bool {
			return naturalLess(reconcileGroupValueByField(groups[i], valueField), reconcileGroupValueByField(groups[j], valueField))
		})
	default:
		sort.SliceStable(groups, func(i, j int) bool {
			return naturalLess(groups[i].GroupKey.String()+"\x00"+groups[i].Metric.String(), groups[j].GroupKey.String()+"\x00"+groups[j].Metric.String())
		})
		orderBy = "group_key"
	}
	projection := strings.ToLower(strings.TrimSpace(action.Params["projection"]))
	includeKeys := parseBoolActionParam(action.Params["include_keys"])
	if projection == "" {
		switch contract.Format {
		case OutputCSVLine:
			projection = "values"
		case OutputPlainSingleLine:
			if !contract.ExplanationAllowed {
				projection = "values"
			} else {
				projection = "key_values"
			}
		case OutputJSONOnly:
			projection = "json_groups"
		case OutputMarkdownTable:
			projection = "markdown_table"
		default:
			projection = "key_values"
		}
	}
	delimiter := strings.TrimSpace(firstNonEmptyString(action.Params["delimiter"], contract.Delimiter))
	if delimiter == "" {
		delimiter = ","
	}
	var answer string
	switch projection {
	case "values", "value_list", "csv_values":
		values := make([]string, 0, len(groups))
		for _, group := range groups {
			value := reconcileGroupValueByField(group, valueField)
			if value == "" {
				continue
			}
			if includeKeys {
				value = displayReconcileGroupKey(group.GroupKey.String(), group.Metric.String()) + "=" + value
			}
			values = append(values, value)
		}
		answer = strings.Join(values, delimiter)
	case "json_groups", "json":
		items := make([]map[string]string, 0, len(groups))
		for _, group := range groups {
			items = append(items, map[string]string{
				"group_key": group.GroupKey.String(),
				"metric":    group.Metric.String(),
				"value":     reconcileGroupValueByField(group, valueField),
			})
		}
		raw, _ := json.Marshal(items)
		answer = string(raw)
	case "markdown_table":
		var b strings.Builder
		b.WriteString("| group | metric | value |\n|---|---|---|\n")
		for _, group := range groups {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", group.GroupKey.String(), group.Metric.String(), reconcileGroupValueByField(group, valueField))
		}
		answer = strings.TrimSpace(b.String())
	default:
		parts := make([]string, 0, len(groups))
		for _, group := range groups {
			value := reconcileGroupValueByField(group, valueField)
			if value == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s=%s", displayReconcileGroupKey(group.GroupKey.String(), group.Metric.String()), value))
		}
		answer = strings.Join(parts, "; ")
		projection = "key_values"
	}
	if err := ValidateAnswer(answer, contract); err != nil {
		return DataArtifact{}, "", nil, fmt.Errorf("assemble_answer output does not satisfy output contract: %w", err)
	}
	next := *reconcile
	next.ActualAnswer = LooseText(answer)
	next.ExpectedAnswer = LooseText(answer)
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "assembled_answer")
	return DataArtifact{
		ID:      id,
		Kind:    string(DataActionAssembleAnswer),
		Summary: fmt.Sprintf("assembled final answer from %d reconcile group(s)", len(groups)),
		Fields: map[string]string{
			"group_count": fmt.Sprintf("%d", len(groups)),
			"projection":  projection,
			"order_by":    orderBy,
			"value_field": valueField,
			"delimiter":   delimiter,
			"answer_len":  fmt.Sprintf("%d", len(answer)),
		},
	}, answer, &next, nil
}

func (r ActionRunner) completeAssembleAnswerGroups(action DataAction, contract OutputContract, artifacts []DataArtifact, groups []ReconcileGroup) []ReconcileGroup {
	if !parseBoolActionParam(firstNonEmptyString(action.Params["complete_reference"], strconv.FormatBool(contract.CompleteReference))) {
		return groups
	}
	keyField := firstNonEmptyString(
		strings.TrimSpace(action.Params["reference_key_field"]),
		strings.TrimSpace(contract.ReferenceKeyField),
		strings.TrimSpace(action.Params["key_field"]),
		strings.TrimSpace(action.Params["group_key_field"]),
		strings.TrimSpace(action.Params["group_key"]),
	)
	if strings.TrimSpace(keyField) == "" {
		return groups
	}
	metric := firstNonEmptyString(strings.TrimSpace(action.Params["metric"]), singleReconcileMetric(groups), "value")
	referencePaths := assembleReferencePaths(action, contract, artifacts)
	keys := r.referenceKeysForAssemble(referencePaths, keyField)
	if len(keys) == 0 {
		return groups
	}
	existing := make(map[string]bool, len(groups))
	for _, group := range groups {
		if strings.TrimSpace(group.Metric.String()) == metric || strings.TrimSpace(group.Metric.String()) == "" {
			if key := strings.TrimSpace(group.GroupKey.String()); key != "" {
				existing[key] = true
			}
		}
	}
	out := append([]ReconcileGroup(nil), groups...)
	for _, key := range keys {
		if key == "" || existing[key] {
			continue
		}
		existing[key] = true
		out = append(out, ReconcileGroup{
			GroupKey:   LooseText(key),
			Metric:     LooseText(metric),
			Expected:   LooseText("0"),
			Actual:     LooseText("0"),
			Difference: LooseText("0"),
		})
	}
	return out
}

func assembleReferencePaths(action DataAction, contract OutputContract, artifacts []DataArtifact) []string {
	var paths []string
	paths = append(paths, parseActionStringListParam(firstNonEmptyString(
		action.Params["reference_paths"],
		action.Params["reference_path"],
		contract.ReferencePath,
	))...)
	paths = append(paths, action.InputPaths...)
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ID) != "" {
			paths = append(paths, artifact.ID)
		}
		paths = append(paths, artifact.SourcePaths...)
	}
	return cleanStringList(paths)
}

func (r ActionRunner) referenceKeysForAssemble(paths []string, keyField string) []string {
	keyField = strings.TrimSpace(keyField)
	if keyField == "" {
		return nil
	}
	var best []string
	seenBest := map[string]bool{}
	for _, path := range cleanStringList(paths) {
		records, headers, _, _, err := r.readActionRecords(path, 100000)
		if err != nil || !actionRecordFieldExistsInRecords(headers, records, keyField) {
			continue
		}
		var keys []string
		seen := map[string]bool{}
		for _, record := range records {
			key := strings.TrimSpace(recordField(record.Fields, keyField))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			keys = append(keys, key)
		}
		if len(keys) > len(best) {
			best = keys
			seenBest = seen
		}
	}
	if len(best) == 0 || len(seenBest) == 0 {
		return nil
	}
	return best
}

// InferReferenceKeyCandidate finds a field whose values structurally cover the
// existing output groups and contain additional keys. It intentionally does not
// infer business semantics from names; it only uses record shape and value-set
// coverage so callers can decide whether a complete-reference projection is
// appropriate for the current output contract.
func (r ActionRunner) InferReferenceKeyCandidate(artifacts []DataArtifact, existingKeys []string, candidatePaths []string, candidateFields []string, maxRecords int) (ReferenceKeyCandidate, bool) {
	existing := cleanStringList(existingKeys)
	if len(existing) == 0 {
		return ReferenceKeyCandidate{}, false
	}
	existingSet := make(map[string]bool, len(existing))
	for _, key := range existing {
		if key != "" {
			existingSet[key] = true
		}
	}
	if len(existingSet) == 0 {
		return ReferenceKeyCandidate{}, false
	}
	if maxRecords <= 0 {
		maxRecords = 100000
	}
	if r.artifactFiles == nil {
		r.artifactFiles = dataActionArtifactFilesFromSeed(artifacts)
	}
	paths := referenceCandidatePaths(artifacts, candidatePaths)
	fieldsHint := cleanStringList(candidateFields)
	var best ReferenceKeyCandidate
	bestScoreSet := false
	for _, path := range paths {
		records, headers, _, _, err := r.readActionRecords(path, maxRecords)
		if err != nil || len(records) == 0 {
			continue
		}
		fields := fieldsHint
		if len(fields) == 0 {
			fields = headers
		}
		for _, field := range fields {
			field = strings.TrimSpace(field)
			if field == "" || !actionRecordFieldExistsInRecords(headers, records, field) {
				continue
			}
			candidate := referenceCandidateForField(path, field, records, existingSet)
			if candidate.KeyCount <= len(existingSet) || candidate.ExistingMatchCount < len(existingSet) {
				continue
			}
			if !bestScoreSet || referenceCandidateBetter(candidate, best) {
				best = candidate
				bestScoreSet = true
			}
		}
	}
	if !bestScoreSet {
		return ReferenceKeyCandidate{}, false
	}
	return best, true
}

func referenceCandidatePaths(artifacts []DataArtifact, seed []string) []string {
	var paths []string
	paths = append(paths, seed...)
	var walk func(DataArtifact)
	walk = func(artifact DataArtifact) {
		paths = append(paths, artifact.SourcePaths...)
		if artifact.Fields != nil {
			paths = append(paths, artifact.Fields["artifact_path"])
			for _, alias := range strings.Split(artifact.Fields["artifact_aliases"], ",") {
				paths = append(paths, alias)
			}
		}
		paths = append(paths, dataActionArtifactAliases(DataAction{ID: artifact.ID, OutputArtifact: artifact.ID}, artifact)...)
		for _, child := range artifact.Children {
			walk(child)
		}
	}
	for _, artifact := range artifacts {
		walk(artifact)
	}
	return cleanStringList(paths)
}

func referenceCandidateForField(path, field string, records []actionRecord, existing map[string]bool) ReferenceKeyCandidate {
	seen := map[string]bool{}
	var keys []string
	matchCount := 0
	for _, record := range records {
		value := strings.TrimSpace(recordField(record.Fields, field))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		keys = append(keys, value)
		if existing[value] {
			matchCount++
		}
	}
	var missing []string
	for _, key := range keys {
		if !existing[key] {
			missing = append(missing, key)
		}
	}
	return ReferenceKeyCandidate{
		Path:               path,
		Field:              field,
		KeyCount:           len(keys),
		ExistingMatchCount: matchCount,
		MissingCount:       len(missing),
		Keys:               keys,
		MissingKeys:        missing,
	}
}

func referenceCandidateBetter(candidate, current ReferenceKeyCandidate) bool {
	if candidate.MissingCount != current.MissingCount {
		return candidate.MissingCount < current.MissingCount
	}
	if candidate.KeyCount != current.KeyCount {
		return candidate.KeyCount < current.KeyCount
	}
	if candidate.Path != current.Path {
		return naturalLess(candidate.Path, current.Path)
	}
	return naturalLess(candidate.Field, current.Field)
}

func singleReconcileMetric(groups []ReconcileGroup) string {
	var metric string
	for _, group := range groups {
		value := strings.TrimSpace(group.Metric.String())
		if value == "" {
			continue
		}
		if metric == "" {
			metric = value
			continue
		}
		if metric != value {
			return ""
		}
	}
	return metric
}

func reconcileGroupValue(group ReconcileGroup) string {
	return reconcileGroupValueByField(group, "actual")
}

func reconcileGroupValueByField(group ReconcileGroup, field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "expected":
		return strings.TrimSpace(firstNonEmptyString(group.Expected.String(), group.Actual.String()))
	case "actual", "":
		return strings.TrimSpace(firstNonEmptyString(group.Actual.String(), group.Expected.String()))
	default:
		return strings.TrimSpace(firstNonEmptyString(group.Actual.String(), group.Expected.String()))
	}
}

func parseBoolActionParam(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on", "include", "key_values", "keys":
		return true
	default:
		return false
	}
}

func (r ActionRunner) readActionRecords(path string, maxRecords int) ([]actionRecord, []string, int, string, error) {
	originalPath := normalizeMaterialPath(path)
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
		records, headers, total = filterActionRecordsForSourceAlias(originalPath, records, headers, total)
		return records, headers, total, rel, err
	case "jsonl":
		records, headers, total, err := readJSONLActionRecords(abs, rel, maxRecords)
		records, headers, total = filterActionRecordsForSourceAlias(originalPath, records, headers, total)
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

func filterActionRecordsForSourceAlias(alias string, records []actionRecord, headers []string, total int) ([]actionRecord, []string, int) {
	alias = normalizeMaterialPath(alias)
	if alias == "" || !strings.HasSuffix(alias, "#records") {
		return records, headers, total
	}
	source := strings.TrimSuffix(alias, "#records")
	source = normalizeMaterialPath(source)
	if source == "" {
		return records, headers, total
	}
	sawSourceMarker := false
	var out []actionRecord
	for _, rec := range records {
		marker := normalizeMaterialPath(recordField(rec.Fields, "_source_path"))
		if marker == "" {
			continue
		}
		sawSourceMarker = true
		if marker != source {
			continue
		}
		out = append(out, rec)
	}
	if !sawSourceMarker {
		return records, headers, total
	}
	return out, collectActionRecordHeaders(out), len(out)
}

func collectActionRecordHeaders(records []actionRecord) []string {
	seen := map[string]bool{}
	var headers []string
	for _, rec := range records {
		for key := range rec.Fields {
			key = strings.TrimSpace(key)
			if key == "" || seen[strings.ToLower(key)] {
				continue
			}
			seen[strings.ToLower(key)] = true
			headers = append(headers, key)
		}
	}
	sort.Strings(headers)
	return headers
}

func (r ActionRunner) materializeActionArtifact(dir string, action DataAction, artifact DataArtifact, payload any) (DataArtifact, error) {
	if r.artifactFiles == nil || strings.TrimSpace(dir) == "" {
		return artifact, nil
	}
	aliases := dataActionArtifactAliases(action, artifact)
	if len(aliases) == 0 {
		return artifact, nil
	}
	payload = dataActionArtifactPayloadForWrite(artifact, payload)
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
	if len(artifact.Headers) == 0 {
		artifact.Headers = dataActionArtifactPayloadHeaders(raw)
	}
	if strings.TrimSpace(artifact.Fields["output_headers"]) == "" && len(artifact.Headers) > 0 {
		artifact.Fields["output_headers"] = strings.Join(artifact.Headers, ",")
	}
	artifact.Fields["artifact_path"] = abs
	artifact.Fields["artifact_aliases"] = strings.Join(aliases, ",")
	for _, alias := range aliases {
		r.registerActionArtifactAlias(alias, abs)
	}
	r.registerActionArtifactChildAliases(artifact, abs)
	return artifact, nil
}

func (r ActionRunner) materializeWorkflowLedgerArtifacts(dir string, rows []RowDecision, rules []RuleCoverageRecord, contributions []ContributionRecord, resolutions []EntityResolutionRecord) ([]DataArtifact, error) {
	var out []DataArtifact
	add := func(id, ledgerType, recordKey string, headers []string, records any, count int) error {
		if count == 0 {
			return nil
		}
		artifact := DataArtifact{
			ID:      id,
			Kind:    "workflow_ledger/" + ledgerType,
			Summary: fmt.Sprintf("workflow %s ledger with %d record(s)", ledgerType, count),
			Headers: cleanStringSlice(headers),
			Fields: map[string]string{
				"workflow_ledger": "true",
				"ledger_type":     ledgerType,
				"record_count":    fmt.Sprintf("%d", count),
			},
			RowCount: count,
		}
		action := DataAction{ID: id, OutputArtifact: id}
		payload := map[string]any{
			recordKey:     records,
			"records":     records,
			"headers":     artifact.Headers,
			"diagnostics": artifact.Fields,
		}
		materialized, err := r.materializeActionArtifact(dir, action, artifact, payload)
		if err != nil {
			return err
		}
		out = append(out, materialized)
		return nil
	}
	if err := add("workflow_decision_records", "decision_records", "rows",
		[]string{"item_id", "decision", "reason", "source", "source_locator"},
		rows, len(rows)); err != nil {
		return nil, err
	}
	if err := add("workflow_rule_coverage", "rule_coverage", "rule_coverage",
		[]string{"rule_id", "rule_text", "status", "notes"},
		rules, len(rules)); err != nil {
		return nil, err
	}
	if err := add("workflow_contributions", "contributions", "contributions",
		[]string{"item_id", "group_key", "metric", "value", "operation", "source", "source_locator"},
		contributions, len(contributions)); err != nil {
		return nil, err
	}
	if err := add("workflow_entity_resolutions", "entity_resolutions", "entity_resolutions",
		[]string{"item_id", "source", "source_locator", "source_value", "canonical_id", "canonical_label", "status", "reason"},
		resolutions, len(resolutions)); err != nil {
		return nil, err
	}
	return out, nil
}

func stripWorkflowLedgerArtifacts(artifacts []DataArtifact) []DataArtifact {
	if len(artifacts) == 0 {
		return nil
	}
	out := make([]DataArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if isWorkflowLedgerArtifact(artifact) {
			continue
		}
		artifact.Children = stripWorkflowLedgerArtifacts(artifact.Children)
		out = append(out, artifact)
	}
	return out
}

func isWorkflowLedgerArtifact(artifact DataArtifact) bool {
	if artifact.Fields != nil && strings.EqualFold(strings.TrimSpace(artifact.Fields["workflow_ledger"]), "true") {
		return true
	}
	id := strings.ToLower(strings.TrimSpace(artifact.ID))
	kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
	return strings.HasPrefix(id, "workflow_") && strings.HasPrefix(kind, "workflow_ledger/")
}

func dataActionArtifactPayloadForWrite(artifact DataArtifact, payload any) any {
	switch rows := payload.(type) {
	case []map[string]any:
		if len(rows) == 0 {
			headers := cleanStringSlice(artifact.Headers)
			if len(headers) > 0 {
				diagnostics := map[string]string{}
				for key, value := range artifact.Fields {
					key = strings.TrimSpace(key)
					value = strings.TrimSpace(value)
					if key != "" && value != "" {
						diagnostics[key] = value
					}
				}
				return map[string]any{
					"records":     []map[string]any{},
					"headers":     headers,
					"diagnostics": diagnostics,
				}
			}
			return []map[string]any{}
		}
	case []EntityResolutionRecord:
		if len(rows) == 0 {
			return emptyLedgerArtifactPayload(artifact, "entity_resolutions", []EntityResolutionRecord{}, []string{"item_id", "source_value", "canonical_id", "canonical_label", "status", "reason"})
		}
	case []RuleCoverageRecord:
		if len(rows) == 0 {
			return emptyLedgerArtifactPayload(artifact, "rule_coverage", []RuleCoverageRecord{}, []string{"rule_id", "rule_text", "status", "notes"})
		}
	case []ContributionRecord:
		if len(rows) == 0 {
			return emptyLedgerArtifactPayload(artifact, "contributions", []ContributionRecord{}, []string{"item_id", "group_key", "metric", "value", "operation", "source"})
		}
	case []RowDecision:
		if len(rows) == 0 {
			return emptyLedgerArtifactPayload(artifact, "rows", []RowDecision{}, []string{"item_id", "decision", "reason", "source"})
		}
	}
	return payload
}

func emptyLedgerArtifactPayload(artifact DataArtifact, key string, empty any, fallbackHeaders []string) map[string]any {
	headers := cleanStringSlice(append([]string(nil), artifact.Headers...))
	if len(headers) == 0 {
		headers = cleanStringSlice(fallbackHeaders)
	}
	diagnostics := map[string]string{}
	for k, v := range artifact.Fields {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			diagnostics[k] = v
		}
	}
	return map[string]any{
		key:           empty,
		"records":     []map[string]any{},
		"headers":     headers,
		"diagnostics": diagnostics,
	}
}

func dataActionArtifactJSONShape(raw []byte) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return compactJSONShape(value, 0)
}

func dataActionArtifactPayloadHeaders(raw []byte) []string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	headers := map[string]bool{}
	addHeader := func(name string) {
		name = strings.TrimSpace(name)
		if name != "" {
			headers[name] = true
		}
	}
	collectExplicitHeaders := func(v any) bool {
		items, ok := v.([]any)
		if !ok {
			return false
		}
		for _, item := range items {
			if s, ok := item.(string); ok {
				addHeader(s)
			}
		}
		return len(headers) > 0
	}
	const maxHeaderSampleRecords = 64
	var sampled int
	var collectObjectKeys func(any)
	collectObjectKeys = func(v any) {
		if sampled >= maxHeaderSampleRecords {
			return
		}
		switch typed := v.(type) {
		case []any:
			for _, item := range typed {
				if sampled >= maxHeaderSampleRecords {
					break
				}
				collectObjectKeys(item)
			}
		case map[string]any:
			containerKeys := []string{"records", "rows", "items", "data", "rules", "rule_coverage", "contributions", "entity_resolutions", "children", "groups"}
			hadContainer := false
			before := len(headers)
			for _, key := range containerKeys {
				if child, ok := typed[key]; ok {
					hadContainer = true
					collectObjectKeys(child)
				}
			}
			if len(headers) > before {
				return
			}
			if hadContainer && collectExplicitHeaders(typed["headers"]) {
				return
			}
			for key := range typed {
				if key == "headers" || key == "diagnostics" || key == "artifact" || key == "fields" {
					continue
				}
				addHeader(key)
			}
			sampled++
		}
	}
	collectObjectKeys(value)
	out := make([]string, 0, len(headers))
	for key := range headers {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
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
	if existing := strings.TrimSpace(r.artifactFiles[alias]); existing != "" && actionArtifactFilePreference(existing) > actionArtifactFilePreference(abs) {
		return
	}
	r.artifactFiles[alias] = abs
}

func (r ActionRunner) registerActionArtifactChildAliases(artifact DataArtifact, abs string) {
	if r.artifactFiles == nil || strings.TrimSpace(abs) == "" {
		return
	}
	for _, child := range artifact.Children {
		for _, alias := range dataActionArtifactAliases(DataAction{ID: child.ID, OutputArtifact: child.ID}, child) {
			r.registerActionArtifactAlias(alias, abs)
		}
		r.registerActionArtifactChildAliases(child, abs)
	}
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
		if existing := strings.TrimSpace(out[alias]); existing != "" && actionArtifactFilePreference(existing) > actionArtifactFilePreference(abs) {
			return
		}
		out[alias] = abs
	}
	var walk func(DataArtifact, string)
	walk = func(artifact DataArtifact, inheritedAbs string) {
		abs := inheritedAbs
		if artifact.Fields != nil {
			if artifactAbs := strings.TrimSpace(artifact.Fields["artifact_path"]); artifactAbs != "" {
				abs = artifactAbs
			}
			for _, alias := range strings.Split(artifact.Fields["artifact_aliases"], ",") {
				register(alias, abs)
			}
		}
		for _, alias := range dataActionArtifactAliases(DataAction{ID: artifact.ID, OutputArtifact: artifact.ID}, artifact) {
			register(alias, abs)
		}
		for _, child := range artifact.Children {
			walk(child, abs)
		}
	}
	for _, artifact := range artifacts {
		walk(artifact, "")
	}
	return out
}

func actionArtifactFilePreference(abs string) int {
	abs = strings.TrimSpace(abs)
	if abs == "" {
		return 0
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return 0
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return 2
	}
	return jsonActionRecordPayloadPreference(value)
}

func jsonActionRecordPayloadPreference(value any) int {
	switch typed := value.(type) {
	case []any:
		return 4
	case map[string]any:
		for _, key := range []string{"records", "rows", "contributions", "entity_resolutions", "rule_coverage", "groups", "items", "data", "values"} {
			if arr, ok := typed[key].([]any); ok {
				if len(arr) > 0 {
					return 4
				}
				return 3
			}
		}
		if _, hasChildren := typed["children"]; hasChildren {
			if _, hasSummary := typed["summary"]; hasSummary {
				return 1
			}
		}
		return 2
	default:
		return 2
	}
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
	headers := jsonRecordWrapperHeaders(value)
	if len(headers) == 0 {
		headers = collectRecordHeaders(items)
	}
	records := make([]actionRecord, 0, minInt(maxRecords, len(items)))
	for i, item := range items {
		if i >= maxRecords {
			break
		}
		records = append(records, actionRecord{Fields: stringifyRecordMap(item), Path: rel, Line: i + 1, Index: i + 1})
	}
	return records, headers, len(items), nil
}

func jsonRecordWrapperHeaders(value any) []string {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range []string{"headers", "fields", "output_headers"} {
		headers := parseJSONHeaderValue(obj[key])
		if len(headers) > 0 {
			return headers
		}
	}
	if meta, ok := obj["diagnostics"].(map[string]any); ok {
		for _, key := range []string{"output_headers", "headers", "fields"} {
			headers := parseJSONHeaderValue(meta[key])
			if len(headers) > 0 {
				return headers
			}
		}
	}
	return nil
}

func parseJSONHeaderValue(value any) []string {
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return cleanStringSlice(out)
	case []string:
		return cleanStringSlice(typed)
	case string:
		return cleanStringSlice(strings.FieldsFunc(typed, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n'
		}))
	default:
		return nil
	}
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
		for _, key := range []string{"records", "rows", "contributions", "entity_resolutions", "rule_coverage", "items", "data", "values"} {
			if item, ok := v[key]; ok {
				if arr, ok := item.([]any); ok {
					return flattenJSONRecords(arr)
				}
			}
		}
		if jsonObjectLooksLikeArtifactMetadata(v) {
			return nil
		}
		return []map[string]any{v}
	default:
		return []map[string]any{{"value": v}}
	}
}

func jsonObjectLooksLikeArtifactMetadata(v map[string]any) bool {
	if v == nil {
		return false
	}
	_, hasChildren := v["children"]
	_, hasSummary := v["summary"]
	_, hasKind := v["kind"]
	_, hasID := v["id"]
	return hasChildren && hasSummary && (hasKind || hasID)
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
	if raw := strings.TrimSpace(firstNonEmptyString(structuredActionParam(action.Params, "filters"), action.Params["filters_json"])); raw != "" {
		parsed, err := parseContributionFiltersRaw(raw)
		if err != nil {
			return nil, err
		}
		filters = append(filters, parsed...)
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

func parseActionFilterListParams(action DataAction, keys ...string) ([]contributionFilter, error) {
	for _, key := range keys {
		raw := strings.TrimSpace(firstNonEmptyString(structuredActionParam(action.Params, key), action.Params[key+"_json"]))
		if raw == "" {
			continue
		}
		filters, err := parseContributionFiltersRaw(raw)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", key, err)
		}
		for i := range filters {
			filters[i].Field = strings.TrimSpace(filters[i].Field)
			filters[i].Op = strings.ToLower(strings.TrimSpace(filters[i].Op))
			filters[i].Value = strings.TrimSpace(filters[i].Value)
			if filters[i].Field == "" {
				return nil, fmt.Errorf("%s[%d] has empty field", key, i)
			}
			if filters[i].Op == "" {
				filters[i].Op = "eq"
			}
		}
		return filters, nil
	}
	return nil, nil
}

func parseContributionFiltersRaw(raw string) ([]contributionFilter, error) {
	raw = strings.TrimSpace(raw)
	candidates := []string{raw}
	if repaired := repairJSONListShape(raw); repaired != raw {
		candidates = append(candidates, repaired)
	}
	var firstErr error
	for _, candidate := range candidates {
		var filters []contributionFilter
		if err := json.Unmarshal([]byte(candidate), &filters); err == nil {
			return filters, nil
		} else if firstErr == nil {
			firstErr = err
		}
		var drafts []contributionFilterDraft
		if err := json.Unmarshal([]byte(candidate), &drafts); err == nil {
			filters = make([]contributionFilter, 0, len(drafts))
			for i, draft := range drafts {
				value, valueErr := normalizeContributionFilterValue(draft.Value)
				if valueErr != nil {
					return nil, fmt.Errorf("parse filters_json[%d].value: %w", i, valueErr)
				}
				filters = append(filters, contributionFilter{
					Field: draft.Field,
					Op:    draft.Op,
					Value: value,
				})
			}
			return filters, nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	return nil, fmt.Errorf("parse filters_json: %w", firstErr)
}

func lowerStringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out[value] = true
	}
	return out
}

func defaultGeneratedBlockingStatusValues() map[string]bool {
	return map[string]bool{
		"ambiguous":          true,
		"matched_ambiguous":  true,
		"missing_source":     true,
		"unmatched_required": true,
		"unresolved":         true,
		"conflict":           true,
		"invalid":            true,
	}
}

func generatedStatusFields(headers []string, records []actionRecord) []string {
	fields := actionRecordFieldNames(headers, records)
	fieldSet := map[string]bool{}
	originalName := map[string]string{}
	for _, field := range fields {
		key := strings.ToLower(strings.TrimSpace(field))
		if key == "" {
			continue
		}
		fieldSet[key] = true
		originalName[key] = field
	}
	var out []string
	for _, field := range fields {
		key := strings.ToLower(strings.TrimSpace(field))
		if key == "status" || !strings.HasSuffix(key, "_status") {
			continue
		}
		base := strings.TrimSuffix(key, "_status")
		if base == "" {
			continue
		}
		if fieldSet[base] || fieldSet[base+"_evidence"] || fieldSet[base+"_reason"] {
			out = append(out, originalName[key])
		}
	}
	return cleanStringSlice(out)
}

func contributionBlockedGeneratedStatusIssues(record actionRecord, statusFields []string, statusFilterFields map[string]bool, blockedStatuses map[string]bool) []string {
	var issues []string
	for _, field := range statusFields {
		field = strings.TrimSpace(field)
		if field == "" || statusFilterFields[strings.ToLower(field)] {
			continue
		}
		value := strings.ToLower(strings.TrimSpace(recordField(record.Fields, field)))
		if value == "" || !blockedStatuses[value] {
			continue
		}
		item := firstNonEmptyString(recordField(record.Fields, "id"), recordField(record.Fields, "item_id"), recordField(record.Fields, "row_id"), fmt.Sprintf("line:%d", record.Line))
		issues = append(issues, fmt.Sprintf("%s=%s@%s", field, value, item))
	}
	return issues
}

func recordQualification(record actionRecord, includeFilters, rejectFilters []contributionFilter, requiredFields, evidenceFields, statusFields []string, acceptedStatuses, blockedStatuses map[string]bool) (bool, string) {
	if len(includeFilters) > 0 && !recordPassesFilters(record.Fields, includeFilters) {
		return false, "include_filter_failed"
	}
	for _, filter := range rejectFilters {
		if recordPassesFilter(record.Fields, filter) {
			return false, "reject_filter_matched:" + strings.TrimSpace(filter.Field)
		}
	}
	for _, field := range requiredFields {
		if strings.TrimSpace(recordField(record.Fields, field)) == "" {
			return false, "required_field_empty:" + strings.TrimSpace(field)
		}
	}
	for _, field := range evidenceFields {
		if strings.TrimSpace(recordField(record.Fields, field)) == "" {
			return false, "evidence_field_empty:" + strings.TrimSpace(field)
		}
	}
	for _, field := range statusFields {
		raw := strings.TrimSpace(recordField(record.Fields, field))
		if raw == "" {
			continue
		}
		value := strings.ToLower(raw)
		if len(acceptedStatuses) > 0 && !acceptedStatuses[value] {
			return false, "status_not_accepted:" + strings.TrimSpace(field)
		}
		if blockedStatuses[value] {
			return false, "status_blocked:" + strings.TrimSpace(field)
		}
	}
	return true, "qualified"
}

func qualificationEvidenceRefs(record actionRecord, rel string, statusFields, evidenceFields []string) []string {
	refs := []string{fmt.Sprintf("%s:%d", rel, record.Line)}
	for _, field := range append(statusFields, evidenceFields...) {
		value := strings.TrimSpace(recordField(record.Fields, strings.TrimSuffix(field, "_status")+"_evidence"))
		if value == "" {
			value = strings.TrimSpace(recordField(record.Fields, field))
		}
		if value != "" {
			refs = append(refs, value)
		}
	}
	return cleanStringList(refs)
}

func eligibleString(ok bool) string {
	if ok {
		return "true"
	}
	return "false"
}

func repairJSONListShape(raw string) string {
	repaired := strings.TrimSpace(raw)
	if repaired == "" {
		return raw
	}
	repaired = strings.ReplaceAll(repaired, "],{", ",{")
	repaired = strings.ReplaceAll(repaired, "], {", ", {")
	repaired = strings.ReplaceAll(repaired, "}],[{", "},{")
	repaired = strings.ReplaceAll(repaired, "}], [{", "}, {")
	if strings.HasPrefix(repaired, "{") && strings.HasSuffix(repaired, "}") {
		repaired = "[" + repaired + "]"
	}
	return repaired
}

func filterFieldNames(filters []contributionFilter) []string {
	out := make([]string, 0, len(filters))
	seen := map[string]bool{}
	for _, filter := range filters {
		field := strings.TrimSpace(filter.Field)
		key := strings.ToLower(field)
		if field == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, field)
	}
	return out
}

func compactFieldSampleJSON(records []actionRecord, fields []string, maxFields, maxValues int) string {
	if maxFields <= 0 || maxValues <= 0 || len(records) == 0 {
		return "{}"
	}
	cleanFields := cleanStringList(fields)
	if len(cleanFields) == 0 {
		return "{}"
	}
	out := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, field := range cleanFields {
		if len(out) >= maxFields {
			break
		}
		for _, record := range records {
			value := strings.TrimSpace(recordField(record.Fields, field))
			if value == "" {
				continue
			}
			if seen[field] == nil {
				seen[field] = map[string]bool{}
			}
			if seen[field][value] {
				continue
			}
			seen[field][value] = true
			out[field] = append(out[field], clampViolationText(value, 80))
			if len(out[field]) >= maxValues {
				break
			}
		}
	}
	if len(out) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return clampViolationText(string(raw), 1200)
}

func filterDiagnosticsJSON(records []actionRecord, filters []contributionFilter, maxFilters, maxValues int) string {
	if maxFilters <= 0 || len(filters) == 0 {
		return "{}"
	}
	type filterDiagnostic struct {
		Field        string   `json:"field"`
		Op           string   `json:"op"`
		Value        string   `json:"value"`
		Matched      int      `json:"matched"`
		SampleValues []string `json:"sample_values,omitempty"`
	}
	out := struct {
		Total         int                `json:"total"`
		CombinedMatch int                `json:"combined_match"`
		Filters       []filterDiagnostic `json:"filters"`
	}{
		Total: len(records),
	}
	for _, record := range records {
		if recordPassesFilters(record.Fields, filters) {
			out.CombinedMatch++
		}
	}
	for i, filter := range filters {
		if i >= maxFilters {
			break
		}
		diag := filterDiagnostic{
			Field: strings.TrimSpace(filter.Field),
			Op:    strings.TrimSpace(filter.Op),
			Value: strings.TrimSpace(filter.Value),
		}
		seen := map[string]bool{}
		for _, record := range records {
			if recordPassesFilter(record.Fields, filter) {
				diag.Matched++
			}
			if maxValues > 0 && len(diag.SampleValues) < maxValues {
				value := recordField(record.Fields, filter.Field)
				if value == "" || seen[value] {
					continue
				}
				seen[value] = true
				diag.SampleValues = append(diag.SampleValues, clampViolationText(value, 80))
			}
		}
		out.Filters = append(out.Filters, diag)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return clampViolationText(string(raw), 1800)
}

func countRecordsPassingFilters(records []actionRecord, filters []contributionFilter) int {
	if len(filters) == 0 {
		return len(records)
	}
	count := 0
	for _, record := range records {
		if recordPassesFilters(record.Fields, filters) {
			count++
		}
	}
	return count
}

func countRecordsPassingFilter(records []actionRecord, filter contributionFilter) int {
	count := 0
	for _, record := range records {
		if recordPassesFilter(record.Fields, filter) {
			count++
		}
	}
	return count
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

func effectiveActionFiltersForRecords(filters []contributionFilter, headers []string, records []actionRecord) ([]contributionFilter, []string) {
	if len(filters) == 0 {
		return nil, nil
	}
	fields := actionRecordFilterFieldNames(headers, records)
	var effective []contributionFilter
	var ignored []string
	seenIgnored := map[string]bool{}
	for _, filter := range filters {
		field := strings.TrimSpace(filter.Field)
		if field == "" || actionRecordFieldExists(fields, field) {
			effective = append(effective, filter)
			continue
		}
		key := strings.ToLower(field)
		if !seenIgnored[key] {
			seenIgnored[key] = true
			ignored = append(ignored, field)
		}
	}
	sort.Strings(ignored)
	return effective, ignored
}

func recordPassesFilter(fields map[string]string, filter contributionFilter) bool {
	got := recordField(fields, filter.Field)
	want := strings.TrimSpace(filter.Value)
	switch filter.Op {
	case "eq", "equals", "==":
		return recordFilterValueEquals(got, want)
	case "ne", "not_equals", "!=":
		return !recordFilterValueEquals(got, want)
	case "contains":
		return strings.Contains(got, want)
	case "not_contains":
		return !strings.Contains(got, want)
	case "nonempty", "present":
		return got != ""
	case "not_empty", "exists":
		return got != ""
	case "empty", "missing", "not_exists":
		return got == ""
	case "gt", "gte", "lt", "lte":
		return compareRecordDecimal(got, want, filter.Op)
	case "in":
		for _, item := range strings.Split(want, ",") {
			if recordFilterValueEquals(got, strings.TrimSpace(item)) {
				return true
			}
		}
		return false
	case "not_in", "nin":
		for _, item := range strings.Split(want, ",") {
			if recordFilterValueEquals(got, strings.TrimSpace(item)) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func recordFilterValueEquals(got, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if got == want {
		return true
	}
	gotBool, gotOK := parseBoolLikeValue(got)
	wantBool, wantOK := parseBoolLikeValue(want)
	if gotOK && wantOK {
		return gotBool == wantBool
	}
	return false
}

func parseBoolLikeValue(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y", "on", "ok", "valid", "include", "included", "pass", "passed", "match", "matched":
		return true, true
	case "0", "false", "f", "no", "n", "off", "invalid", "exclude", "excluded", "fail", "failed", "missing", "unmatched":
		return false, true
	default:
		return false, false
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

func actionRecordVirtualFields(record actionRecord, rel string) map[string]string {
	sourcePath := firstNonEmptyString(strings.TrimSpace(record.Path), strings.TrimSpace(rel))
	index := record.Index
	if index <= 0 {
		index = record.Line
	}
	line := record.Line
	if line <= 0 {
		line = index
	}
	sourceIndex := firstNonEmptyString(recordField(record.Fields, "_source_index"), recordField(record.Fields, "source_index"), recordField(record.Fields, "row_index"), fmt.Sprintf("%d", index))
	sourceLine := firstNonEmptyString(recordField(record.Fields, "_source_line"), recordField(record.Fields, "source_line"), recordField(record.Fields, "line"), fmt.Sprintf("%d", line))
	sourceLocator := firstNonEmptyString(recordField(record.Fields, "_source_locator"), recordField(record.Fields, "source_locator"))
	if sourceLocator == "" && sourcePath != "" {
		sourceLocator = fmt.Sprintf("%s#%s", sourcePath, sourceIndex)
	}
	out := map[string]string{
		"_source":         sourcePath,
		"_source_path":    sourcePath,
		"_source_index":   sourceIndex,
		"_source_line":    sourceLine,
		"_source_locator": sourceLocator,
		"source_path":     sourcePath,
		"source_index":    sourceIndex,
		"source_line":     sourceLine,
		"source_locator":  sourceLocator,
		"row_index":       sourceIndex,
		"line":            sourceLine,
	}
	return out
}

func markKnownActionVirtualFields(out map[string]bool) {
	for _, field := range []string{
		"_source", "_source_path", "_source_index", "_source_line", "_source_locator",
		"source_path", "source_index", "source_line", "source_locator",
		"row_index", "line",
	} {
		out[strings.ToLower(field)] = true
	}
}

func actionReservedSourceField(field string) bool {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "_source", "_source_path", "_source_index", "_source_line", "_source_locator":
		return true
	default:
		return false
	}
}

func actionIndexLikeField(field string) bool {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "_source_index", "_source_line", "_left_index", "_right_index", "_left_line", "_right_line",
		"source_index", "source_line", "row_index", "record_index", "item_index", "line_index",
		"index", "idx", "row", "line", "line_no", "line_number", "row_no", "row_number":
		return true
	default:
		return false
	}
}

func actionRecordFieldNames(headers []string, records []actionRecord) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, name)
	}
	for _, header := range headers {
		add(header)
	}
	for _, record := range records {
		for field := range record.Fields {
			add(field)
		}
	}
	sort.Strings(out)
	return out
}

func actionRecordFilterFieldNames(headers []string, records []actionRecord) []string {
	if len(records) == 0 {
		return actionRecordFieldNames(headers, records)
	}
	return actionRecordFieldNames(nil, records)
}

func actionRecordFieldExists(fields []string, field string) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return false
	}
	want := strings.ToLower(field)
	for _, got := range fields {
		if strings.ToLower(strings.TrimSpace(got)) == want {
			return true
		}
	}
	return false
}

func actionRecordAnyFieldExists(fields []string, candidates []string) bool {
	for _, candidate := range candidates {
		if actionRecordFieldExists(fields, candidate) {
			return true
		}
	}
	return false
}

func actionRecordFieldMatchCount(fields []string, candidates []string) int {
	count := 0
	seen := map[string]bool{}
	for _, candidate := range candidates {
		key := strings.ToLower(strings.TrimSpace(candidate))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if actionRecordFieldExists(fields, candidate) {
			count++
		}
	}
	return count
}

func actionRecordAnyFieldOrVirtualExists(fields []string, candidates []string, virtualFields ...string) bool {
	if actionRecordAnyFieldExists(fields, candidates) {
		return true
	}
	virtual := map[string]bool{}
	for _, field := range virtualFields {
		field = strings.ToLower(strings.TrimSpace(field))
		if field != "" {
			virtual[field] = true
		}
	}
	for _, candidate := range candidates {
		if virtual[strings.ToLower(strings.TrimSpace(candidate))] {
			return true
		}
	}
	return false
}

func applyResolutionVirtualBaseKeyFields() []string {
	return []string{"_source_index", "source_index", "_source_line", "source_line", "line", "row_index", "row"}
}

func clampStringSliceForError(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return append([]string(nil), values...)
	}
	out := append([]string(nil), values[:limit]...)
	out = append(out, fmt.Sprintf("...+%d", len(values)-limit))
	return out
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

func structuredActionParam(params map[string]string, key string) string {
	value := strings.TrimSpace(params[key])
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{") {
		return value
	}
	return ""
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
	inputs := cleanStringListPreserveOrder(action.InputPaths)
	if len(inputs) == 0 {
		inputs = cleanStringListPreserveOrder(plan.InputPaths)
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
	return actionIntParamAny(action, []string{key}, fallback, minValue, maxValue)
}

func actionIntParamAny(action DataAction, keys []string, fallback, minValue, maxValue int) int {
	value := fallback
	for _, key := range keys {
		if raw := strings.TrimSpace(action.Params[key]); raw != "" {
			var parsed int
			if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil {
				value = parsed
				break
			}
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

func naturalLess(a, b string) bool {
	ta := naturalTokens(a)
	tb := naturalTokens(b)
	n := len(ta)
	if len(tb) < n {
		n = len(tb)
	}
	for i := 0; i < n; i++ {
		if ta[i].isNum && tb[i].isNum {
			if ta[i].num != tb[i].num {
				return ta[i].num < tb[i].num
			}
			if ta[i].text != tb[i].text {
				return len(ta[i].text) < len(tb[i].text)
			}
			continue
		}
		if ta[i].text != tb[i].text {
			return ta[i].text < tb[i].text
		}
	}
	if len(ta) != len(tb) {
		return len(ta) < len(tb)
	}
	return a < b
}

type naturalToken struct {
	text  string
	num   int64
	isNum bool
}

func naturalTokens(value string) []naturalToken {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil
	}
	var out []naturalToken
	for i := 0; i < len(value); {
		start := i
		isDigit := value[i] >= '0' && value[i] <= '9'
		for i < len(value) && ((value[i] >= '0' && value[i] <= '9') == isDigit) {
			i++
		}
		text := value[start:i]
		token := naturalToken{text: text, isNum: isDigit}
		if isDigit {
			if n, err := strconv.ParseInt(text, 10, 64); err == nil {
				token.num = n
			}
		}
		out = append(out, token)
	}
	return out
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

func cleanStringListPreserveOrder(in []string) []string {
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
