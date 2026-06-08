package dataworkflow

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type GeneratedSchemaDiagnosticInput struct {
	Current   dataquery.TaskPlan
	Artifacts []ArtifactSchemaProjection
	Limit     int
}

type GeneratedSchemaDiagnosticFallbackInput struct {
	Current   dataquery.TaskPlan
	Coverage  dataquery.CoverageContract
	Output    dataquery.OutputContract
	Artifacts []ArtifactSchemaProjection
	Records   []WorkflowRecord
	Limit     int
}

func GeneratedSchemaDiagnosticInputs(input GeneratedSchemaDiagnosticInput) []string {
	limit := input.Limit
	if limit <= 0 {
		limit = 1
	}
	if len(input.Artifacts) == 0 {
		return nil
	}
	artifactByAlias := map[string]ArtifactSchemaProjection{}
	artifactOrder := map[string]int{}
	for i, artifact := range input.Artifacts {
		for _, alias := range artifactSchemaAliases(artifact) {
			key := normalizeAccessPath(alias)
			if key == "" {
				continue
			}
			if _, ok := artifactByAlias[key]; !ok {
				artifactByAlias[key] = artifact
				artifactOrder[key] = i
			}
		}
	}
	type diagnosticCandidate struct {
		alias string
		key   string
		score int
		order int
	}
	candidates := map[string]diagnosticCandidate{}
	add := func(value string, fromPlan bool, order int) {
		key := normalizeAccessPath(value)
		if key == "" {
			return
		}
		artifact, ok := artifactByAlias[key]
		if !ok || !ArtifactUsableForGeneratedSchemaDiagnostic(artifact) {
			return
		}
		alias := firstArtifactAlias(artifact)
		if alias == "" {
			alias = strings.TrimSpace(value)
		}
		score := 10
		if fromPlan {
			score += 5
		}
		if artifactOrderValue, ok := artifactOrder[key]; ok && artifactOrderValue < order {
			order = artifactOrderValue
		}
		score += min(len(artifact.Fields), 24)
		score += 45
		shape := strings.ToLower(strings.TrimSpace(artifact.JSONShape))
		if strings.Contains(shape, "array") || strings.Contains(shape, "records") || strings.Contains(shape, "object") {
			score += 8
		}
		if artifactKindHasPrefix(artifact.Kind,
			dataquery.DataActionApplyResolutions,
			dataquery.DataActionEnrichRecords,
			dataquery.DataActionJoinRecords,
			dataquery.DataActionDeriveFields,
			dataquery.DataActionExtractFields,
			dataquery.DataActionGroupRecords,
			dataquery.DataActionFilterRecords,
			dataquery.DataActionQualifyRecords,
			dataquery.DataActionExtractRecords,
			dataquery.DataActionExpandRecords,
			dataquery.DataActionComputeContribs,
		) {
			score += 16
		}
		next := diagnosticCandidate{alias: alias, key: key, score: score, order: order}
		if prev, ok := candidates[key]; ok {
			if prev.score > next.score || (prev.score == next.score && prev.order <= next.order) {
				return
			}
		}
		candidates[key] = next
	}
	order := 0
	collect := func(values []string) {
		for _, value := range cleanStrings(values) {
			add(value, true, order)
			order++
		}
	}
	collect(input.Current.InputPaths)
	for _, action := range input.Current.Actions {
		collect(action.InputPaths)
	}
	for i, artifact := range input.Artifacts {
		add(firstArtifactAlias(artifact), false, 1000+i)
	}
	ranked := make([]diagnosticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		ranked = append(ranked, candidate)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].order < ranked[j].order
	})
	out := make([]string, 0, min(limit, len(ranked)))
	for _, candidate := range ranked {
		out = append(out, candidate.alias)
		if len(out) >= limit {
			break
		}
	}
	return cleanStrings(out)
}

func ArtifactUsableForGeneratedSchemaDiagnostic(artifact ArtifactSchemaProjection) bool {
	if artifactKindHasPrefix(artifact.Kind,
		dataquery.DataActionMaterialInventory,
		dataquery.DataActionInspectMaterial,
		dataquery.DataActionDeriveRules,
		dataquery.DataActionNormalizeEntities,
		dataquery.DataActionReconcile,
		dataquery.DataActionAssembleAnswer,
	) {
		return false
	}
	if artifactKindHasPrefix(artifact.Kind,
		dataquery.DataActionExtractRecords,
		dataquery.DataActionDeriveFields,
		dataquery.DataActionExtractFields,
		dataquery.DataActionGroupRecords,
		dataquery.DataActionExpandRecords,
		dataquery.DataActionFilterRecords,
		dataquery.DataActionQualifyRecords,
		dataquery.DataActionMappingCandidate,
		dataquery.DataActionApplyResolutions,
		dataquery.DataActionEnrichRecords,
		dataquery.DataActionJoinRecords,
		dataquery.DataActionComputeContribs,
		dataquery.DataActionCustomTransform,
	) {
		return len(artifact.Fields) > 0 || artifactSchemaShapeLooksRecordLike(artifact)
	}
	if len(artifact.Fields) == 0 || artifactLooksLikeMappingOnly(artifact) {
		return false
	}
	return artifactSchemaShapeLooksRecordLike(artifact)
}

func BuildGeneratedSchemaDiagnosticFallbackPlan(input GeneratedSchemaDiagnosticFallbackInput) (dataquery.TaskPlan, bool) {
	inputs := GeneratedSchemaDiagnosticInputs(GeneratedSchemaDiagnosticInput{
		Current:   input.Current,
		Artifacts: input.Artifacts,
		Limit:     input.Limit,
	})
	if len(inputs) == 0 {
		return dataquery.TaskPlan{}, false
	}
	out := input.Current
	out.Status = "ready"
	out.Script = ""
	out.Actions = []dataquery.DataAction{{
		ID:             "inspect_generated_artifact_schema",
		Kind:           dataquery.DataActionInspectMaterial,
		Purpose:        "inspect generated artifact schema after a script-disabled repair attempted to use custom_transform",
		InputPaths:     inputs,
		OutputArtifact: "generated_artifact_schema_diagnostics.json",
	}}
	out.InputPaths = inputs
	out.OutputContract = dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true}
	out.CoverageContract = input.Coverage
	if out.CoverageContract.RequiredMaterials == nil {
		out.CoverageContract = input.Current.CoverageContract
	}
	out.ContinueAfter = true
	out.WhyThisBatch = "custom_transform is disabled for the current workflow stage; inspect generated artifact schemas before the next typed repair"
	out.NextBatch = "continue with allowed typed data actions using the inspected artifact fields"
	if strings.TrimSpace(out.Goal) == "" {
		out.Goal = "inspect generated data artifacts for the next typed workflow action"
	}
	if out.OutputContract.Format == "" {
		out.OutputContract = input.Output
	}
	if PlanActionSignatureAlreadySeenInRecords(input.Records, out) {
		return dataquery.TaskPlan{}, false
	}
	return out, true
}

func artifactLooksLikeMappingOnly(artifact ArtifactSchemaProjection) bool {
	if artifactKindHasPrefix(artifact.Kind, dataquery.DataActionMappingCandidate, dataquery.DataActionNormalizeEntities) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(artifact.NodeClass), ArtifactNodeClassWorkflowLedger)
}
