package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ApplyResolutionContractGuardInput struct {
	Action            dataquery.DataAction
	ActionIndex       int
	SchemaProjections []ArtifactSchemaProjection
}

func ApplyResolutionContractGuardResult(input ApplyResolutionContractGuardInput) GuardResult {
	action := input.Action
	inputs := cleanStrings(action.InputPaths)
	basePath := firstNonEmpty(
		strings.TrimSpace(action.Params["base_path"]),
		strings.TrimSpace(action.Params["record_path"]),
		strings.TrimSpace(action.Params["input_path"]),
		strings.TrimSpace(action.Params["source_path"]),
	)
	specs := actionObjectListParam(action.Params, "resolution_specs_json", "apply_specs_json", "resolution_specs", "apply_specs")
	if len(specs) == 0 {
		specs = []map[string]any{{}}
	}
	for i, spec := range specs {
		specBasePath := firstNonEmpty(
			mapStringValue(spec, "base_path"),
			mapStringValue(spec, "record_path"),
			mapStringValue(spec, "input_path"),
			mapStringValue(spec, "source_path"),
		)
		if basePath == "" && specBasePath != "" {
			basePath = specBasePath
		}
		if basePath != "" && specBasePath != "" && normalizeAccessPath(basePath) != normalizeAccessPath(specBasePath) {
			message := fmt.Sprintf("data planning incomplete: action %d (%s) has conflicting apply_entity_resolutions base paths [%s] and [%s]. Apply resolutions to one base record artifact per action, or split separate base artifacts into separate actions.",
				input.ActionIndex+1,
				applyResolutionActionLabel(action),
				basePath,
				specBasePath)
			return ActionInputContractGuardResult(ActionInputContractGuardInput{
				Code:          "apply_resolution_contract",
				Action:        action,
				InputAlias:    basePath,
				MissingFields: []string{"base_path"},
				Message:       message,
			})
		}
		if i == len(specs)-1 && basePath == "" {
			basePath = firstActionInput(inputs, 0)
		}
	}
	if basePath == "" {
		return GuardResult{}
	}
	inferredBasePath, inferredResolutionPath, roleErr := inferApplyResolutionRolePaths(input.SchemaProjections, action, input.ActionIndex, inputs, basePath)
	if roleErr != "" {
		return ActionInputContractGuardResult(ActionInputContractGuardInput{
			Code:       "apply_resolution_role_contract",
			Action:     action,
			InputAlias: basePath,
			Message:    roleErr,
		})
	}
	if inferredBasePath != "" {
		basePath = inferredBasePath
	}
	for i, spec := range specs {
		specBasePath := firstNonEmpty(
			mapStringValue(spec, "base_path"),
			mapStringValue(spec, "record_path"),
			mapStringValue(spec, "input_path"),
			mapStringValue(spec, "source_path"),
			basePath,
		)
		if inferredBasePath != "" && applyResolutionPathLooksLikeResolutionLedger(input.SchemaProjections, specBasePath) {
			specBasePath = inferredBasePath
		}
		baseFields := cleanStrings(append(
			fieldRefsFromSpec(spec, "base_key_fields", "source_key_fields"),
			fieldRefsFromSpec(spec, "base_key_field", "source_key_field")...,
		))
		baseFields = cleanStrings(append(baseFields,
			fieldRefsFromSpec(spec, "source_fields", "source_field", "base_fields", "base_field", "record_fields", "record_field")...,
		))
		if len(baseFields) == 0 {
			baseFields = parseActionStringList(firstNonEmpty(action.Params["base_key_fields"], action.Params["base_key_field"], action.Params["source_key_fields"], action.Params["source_key_field"]))
		}
		if missing := missingFieldsOnArtifactWithVirtual(input.SchemaProjections, specBasePath, baseFields, "_source_index", "source_index", "_source_line", "source_line", "line", "row_index", "row"); len(missing) > 0 {
			if guard := missingFieldReferenceGuard(ActionFieldReferenceGuardInput{
				Action:            action,
				ActionIndex:       input.ActionIndex,
				SchemaProjections: input.SchemaProjections,
			}, specBasePath, missing); !guard.Empty() {
				return guard
			}
		}
		existingIDField := firstNonEmpty(
			mapStringValue(spec, "existing_id_field"),
			mapStringValue(spec, "base_id_field"),
			mapStringValue(spec, "current_id_field"),
			mapStringValue(spec, "source_id_field"),
			mapStringValue(spec, "existing_canonical_id_field"),
			strings.TrimSpace(action.Params["existing_id_field"]),
			strings.TrimSpace(action.Params["base_id_field"]),
			strings.TrimSpace(action.Params["current_id_field"]),
			strings.TrimSpace(action.Params["source_id_field"]),
			strings.TrimSpace(action.Params["existing_canonical_id_field"]),
		)
		if existingIDField != "" {
			if guard := missingFieldReferenceGuard(ActionFieldReferenceGuardInput{
				Action:            action,
				ActionIndex:       input.ActionIndex,
				SchemaProjections: input.SchemaProjections,
			}, specBasePath, MissingFieldsOnArtifactSchema(input.SchemaProjections, specBasePath, []string{existingIDField})); !guard.Empty() {
				return guard
			}
			referencePath := firstNonEmpty(
				mapStringValue(spec, "reference_path"),
				mapStringValue(spec, "verification_path"),
				mapStringValue(spec, "canonical_reference_path"),
				strings.TrimSpace(action.Params["reference_path"]),
				strings.TrimSpace(action.Params["verification_path"]),
				strings.TrimSpace(action.Params["canonical_reference_path"]),
			)
			if referencePath == "" {
				message := fmt.Sprintf("data planning incomplete: action %d (%s) apply_entity_resolutions spec %d declares existing_id_field %q but no reference_path. Provide a reference/lookup artifact plus reference_id_field so the existing canonical value can be verified structurally, or omit existing_id_field.",
					input.ActionIndex+1,
					applyResolutionActionLabel(action),
					i+1,
					existingIDField)
				return ActionInputContractGuardResult(ActionInputContractGuardInput{
					Code:          "apply_resolution_contract",
					Action:        action,
					InputAlias:    specBasePath,
					MissingFields: []string{"reference_path"},
					Message:       message,
				})
			}
			referenceIDField := firstNonEmpty(
				mapStringValue(spec, "reference_id_field"),
				mapStringValue(spec, "reference_key_field"),
				mapStringValue(spec, "lookup_id_field"),
				mapStringValue(spec, "canonical_reference_id_field"),
				strings.TrimSpace(action.Params["reference_id_field"]),
				strings.TrimSpace(action.Params["reference_key_field"]),
				strings.TrimSpace(action.Params["lookup_id_field"]),
				strings.TrimSpace(action.Params["canonical_reference_id_field"]),
			)
			referenceFields := cleanStrings([]string{
				referenceIDField,
				firstNonEmpty(mapStringValue(spec, "reference_label_field"), mapStringValue(spec, "reference_name_field"), mapStringValue(spec, "lookup_label_field"), mapStringValue(spec, "canonical_reference_label_field"), strings.TrimSpace(action.Params["reference_label_field"]), strings.TrimSpace(action.Params["reference_name_field"]), strings.TrimSpace(action.Params["lookup_label_field"]), strings.TrimSpace(action.Params["canonical_reference_label_field"])),
				firstNonEmpty(mapStringValue(spec, "reference_status_field"), mapStringValue(spec, "lookup_status_field"), mapStringValue(spec, "canonical_reference_status_field"), strings.TrimSpace(action.Params["reference_status_field"]), strings.TrimSpace(action.Params["lookup_status_field"]), strings.TrimSpace(action.Params["canonical_reference_status_field"])),
			})
			if referenceIDField == "" {
				message := fmt.Sprintf("data planning incomplete: action %d (%s) apply_entity_resolutions spec %d declares existing_id_field %q but no reference_id_field. Provide the reference key/code/id field used to verify existing values.",
					input.ActionIndex+1,
					applyResolutionActionLabel(action),
					i+1,
					existingIDField)
				return ActionInputContractGuardResult(ActionInputContractGuardInput{
					Code:          "apply_resolution_contract",
					Action:        action,
					InputAlias:    referencePath,
					MissingFields: []string{"reference_id_field"},
					Message:       message,
				})
			}
			if guard := missingFieldReferenceGuard(ActionFieldReferenceGuardInput{
				Action:            action,
				ActionIndex:       input.ActionIndex,
				SchemaProjections: input.SchemaProjections,
			}, referencePath, MissingFieldsOnArtifactSchema(input.SchemaProjections, referencePath, referenceFields)); !guard.Empty() {
				return guard
			}
		}
		resolutionPath := firstNonEmpty(
			mapStringValue(spec, "resolution_path"),
			mapStringValue(spec, "mapping_path"),
			mapStringValue(spec, "lookup_path"),
			mapStringValue(spec, "path"),
			strings.TrimSpace(action.Params["resolution_path"]),
			strings.TrimSpace(action.Params["mapping_path"]),
			strings.TrimSpace(action.Params["lookup_path"]),
			firstNonBaseActionInput(inputs, specBasePath),
		)
		if inferredResolutionPath != "" && (resolutionPath == "" || normalizeAccessPath(resolutionPath) == normalizeAccessPath(inferredBasePath)) {
			resolutionPath = inferredResolutionPath
		}
		if resolutionPath == "" {
			continue
		}
		if resolutionArtifact, ok := ArtifactSchemaByAlias(input.SchemaProjections, resolutionPath); ok && artifactIsDiagnosticChild(resolutionArtifact) {
			return ApplyResolutionDiagnosticInputGuardResult(ApplyResolutionDiagnosticInputGuardInput{
				Action:         action,
				ActionIndex:    input.ActionIndex,
				ResolutionPath: resolutionPath,
			})
		}
		baseArtifact, baseOK := ArtifactSchemaByAlias(input.SchemaProjections, basePath)
		if mapping, ok := ArtifactSchemaByAlias(input.SchemaProjections, resolutionPath); ok && baseOK && !ArtifactResolutionLineageCompatible(baseArtifact, mapping) {
			return ApplyResolutionLineageGuardResult(ApplyResolutionLineageGuardInput{
				Action:                  action,
				ActionIndex:             input.ActionIndex,
				ResolutionPath:          resolutionPath,
				BasePath:                basePath,
				ResolutionSourceLineage: ArtifactSourceLineageSummary(mapping),
				BaseLineage:             ArtifactLineageSummary(basePath, baseArtifact),
			})
		}
		resolutionFields := cleanStrings(append(
			fieldRefsFromSpec(spec, "resolution_key_fields", "mapping_key_fields"),
			fieldRefsFromSpec(spec, "resolution_key_field", "mapping_key_field")...,
		))
		if len(resolutionFields) == 0 {
			resolutionFields = parseActionStringList(firstNonEmpty(action.Params["resolution_key_fields"], action.Params["resolution_key_field"], action.Params["mapping_key_fields"], action.Params["mapping_key_field"]))
		}
		if guard := missingFieldReferenceGuard(ActionFieldReferenceGuardInput{
			Action:            action,
			ActionIndex:       input.ActionIndex,
			SchemaProjections: input.SchemaProjections,
		}, resolutionPath, missingApplyResolutionFieldsOnArtifact(input.SchemaProjections, resolutionPath, resolutionFields)); !guard.Empty() {
			return guard
		}
		canonicalIDField := firstNonEmpty(mapStringValue(spec, "canonical_id_field"), strings.TrimSpace(action.Params["canonical_id_field"]), "canonical_id")
		canonicalLabelField := firstNonEmpty(mapStringValue(spec, "canonical_label_field"), strings.TrimSpace(action.Params["canonical_label_field"]), "canonical_label")
		if !artifactHasAnyField(input.SchemaProjections, resolutionPath, canonicalIDField, canonicalLabelField) {
			artifact, ok := ArtifactSchemaByAlias(input.SchemaProjections, resolutionPath)
			if !ok || len(artifact.Fields) == 0 {
				continue
			}
			canonicalFields := cleanStrings([]string{canonicalIDField, canonicalLabelField})
			if guard := missingFieldReferenceGuard(ActionFieldReferenceGuardInput{
				Action:            action,
				ActionIndex:       input.ActionIndex,
				SchemaProjections: input.SchemaProjections,
			}, resolutionPath, canonicalFields); !guard.Empty() {
				return guard
			}
			return ApplyResolutionCanonicalFieldGuardResult(ApplyResolutionCanonicalFieldGuardInput{
				Action:          action,
				ActionIndex:     input.ActionIndex,
				SpecIndex:       i,
				ResolutionPath:  resolutionPath,
				CanonicalFields: canonicalFields,
				AvailableFields: artifact.Fields,
			})
		}
		if guard := applyResolutionNoProgressGuardResult(input.SchemaProjections, action, input.ActionIndex, specBasePath, resolutionPath, spec, len(specs)); !guard.Empty() {
			return guard
		}
	}
	return GuardResult{}
}

func applyResolutionActionLabel(action dataquery.DataAction) string {
	return firstNonEmpty(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))
}

func inferApplyResolutionRolePaths(projections []ArtifactSchemaProjection, action dataquery.DataAction, actionIndex int, inputs []string, basePath string) (string, string, string) {
	if !applyResolutionPathLooksLikeMapping(projections, basePath) {
		return "", "", ""
	}
	var candidates []string
	for _, input := range cleanStrings(inputs) {
		if normalizeAccessPath(input) == "" || normalizeAccessPath(input) == normalizeAccessPath(basePath) {
			continue
		}
		artifact, ok := ArtifactSchemaByAlias(projections, input)
		if !ok || len(artifact.Fields) == 0 || artifactLooksLikeEntityResolutionLedger(artifact) {
			continue
		}
		candidates = append(candidates, input)
	}
	actionLabel := applyResolutionActionLabel(action)
	switch len(candidates) {
	case 0:
		return "", "", fmt.Sprintf("data planning incomplete: action %d (%s) apply_entity_resolutions selected base input %s, but that artifact looks like an entity_resolution ledger. Provide input_paths=[base_record_artifact, entity_resolution_artifact...] or set base_path to the base records.",
			actionIndex+1, actionLabel, basePath)
	case 1:
		return candidates[0], basePath, ""
	default:
		return "", "", fmt.Sprintf("data planning incomplete: action %d (%s) apply_entity_resolutions selected base input %s, but that artifact looks like an entity_resolution ledger and multiple possible base record artifacts were found [%s]. Set base_path explicitly or split the action.",
			actionIndex+1, actionLabel, basePath, strings.Join(candidates, ", "))
	}
}

func applyResolutionPathLooksLikeMapping(projections []ArtifactSchemaProjection, path string) bool {
	return applyResolutionPathLooksLikeResolutionLedger(projections, path)
}

func applyResolutionPathLooksLikeResolutionLedger(projections []ArtifactSchemaProjection, path string) bool {
	artifact, ok := ArtifactSchemaByAlias(projections, path)
	return ok && artifactLooksLikeEntityResolutionLedger(artifact)
}

func firstNonBaseActionInput(inputs []string, basePath string) string {
	base := normalizeAccessPath(basePath)
	for _, input := range cleanStrings(inputs) {
		if normalizeAccessPath(input) == "" || normalizeAccessPath(input) == base {
			continue
		}
		return input
	}
	return ""
}

func missingFieldsOnArtifactWithVirtual(projections []ArtifactSchemaProjection, alias string, fields []string, virtualFields ...string) []string {
	missing := MissingFieldsOnArtifactSchema(projections, alias, fields)
	if len(missing) == 0 {
		return nil
	}
	virtual := map[string]bool{}
	for _, field := range virtualFields {
		field = strings.ToLower(strings.TrimSpace(field))
		if field != "" {
			virtual[field] = true
		}
	}
	var out []string
	for _, field := range missing {
		if virtual[strings.ToLower(strings.TrimSpace(field))] {
			continue
		}
		out = append(out, field)
	}
	return out
}

func missingApplyResolutionFieldsOnArtifact(projections []ArtifactSchemaProjection, alias string, fields []string) []string {
	missing := MissingFieldsOnArtifactSchema(projections, alias, fields)
	if len(missing) == 0 {
		return nil
	}
	artifact, ok := ArtifactSchemaByAlias(projections, alias)
	if !ok || len(artifact.Fields) == 0 {
		return missing
	}
	set := artifactFieldSet(artifact.Fields)
	var out []string
	for _, field := range missing {
		if resolutionLocatorEquivalentField(set, field) != "" {
			continue
		}
		out = append(out, field)
	}
	return out
}

func resolutionLocatorEquivalentField(set map[string]string, requested string) string {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "item_id", "source_locator", "_source_locator", "record_id", "row_id", "id",
		"_source_index", "source_index", "_source_line", "source_line", "line", "row_index", "row":
	default:
		return ""
	}
	for _, candidate := range []string{"item_id", "source_locator", "_source_locator", "record_id", "row_id", "id"} {
		if actual := set[strings.ToLower(candidate)]; actual != "" {
			return actual
		}
	}
	return ""
}

func artifactHasAnyField(projections []ArtifactSchemaProjection, alias string, fields ...string) bool {
	artifact, ok := ArtifactSchemaByAlias(projections, alias)
	if !ok || len(artifact.Fields) == 0 {
		return true
	}
	return artifactHasAnyNamedField(artifact, fields...)
}

func artifactHasAnyNamedField(artifact ArtifactSchemaProjection, fields ...string) bool {
	set := artifactFieldSet(artifact.Fields)
	for _, field := range fields {
		if set[strings.ToLower(strings.TrimSpace(field))] != "" {
			return true
		}
	}
	return false
}

func applyResolutionNoProgressGuardResult(projections []ArtifactSchemaProjection, action dataquery.DataAction, actionIndex int, basePath, resolutionPath string, spec map[string]any, specCount int) GuardResult {
	base, ok := ArtifactSchemaByAlias(projections, basePath)
	if !ok || len(base.Fields) == 0 || strings.TrimSpace(resolutionPath) == "" {
		return GuardResult{}
	}
	targetFields := applyResolutionTargetFields(action, spec, specCount)
	if len(targetFields) == 0 {
		return GuardResult{}
	}
	if !artifactHasAnyNamedField(base, targetFields...) {
		return GuardResult{}
	}
	if !ArtifactLineageContains(base, resolutionPath) {
		return GuardResult{}
	}
	return ApplyResolutionNoProgressGuardResult(ApplyResolutionNoProgressGuardInput{
		Action:         action,
		ActionIndex:    actionIndex,
		ResolutionPath: resolutionPath,
		BasePath:       basePath,
		TargetFields:   targetFields,
	})
}

func applyResolutionTargetFields(action dataquery.DataAction, spec map[string]any, specCount int) []string {
	targetID := firstNonEmpty(
		mapStringValue(spec, "target_id_field"),
		mapStringValue(spec, "target_field"),
		mapStringValue(spec, "canonical_target_field"),
		strings.TrimSpace(action.Params["target_id_field"]),
		strings.TrimSpace(action.Params["target_field"]),
		strings.TrimSpace(action.Params["canonical_target_field"]),
	)
	if targetID == "" && specCount <= 1 {
		targetID = "canonical_id"
	}
	targetLabel := firstNonEmpty(
		mapStringValue(spec, "target_label_field"),
		mapStringValue(spec, "label_target_field"),
		strings.TrimSpace(action.Params["target_label_field"]),
		strings.TrimSpace(action.Params["label_target_field"]),
	)
	targetStatus := firstNonEmpty(
		mapStringValue(spec, "target_status_field"),
		mapStringValue(spec, "status_target_field"),
		strings.TrimSpace(action.Params["target_status_field"]),
		strings.TrimSpace(action.Params["status_target_field"]),
	)
	if targetStatus == "" && targetID != "" {
		targetStatus = targetID + "_status"
	}
	return cleanStrings([]string{targetID, targetLabel, targetStatus})
}

func artifactIsDiagnosticChild(artifact ArtifactSchemaProjection) bool {
	return strings.TrimSpace(artifact.NodeClass) == ArtifactNodeClassDiagnosticChild ||
		IsDiagnosticChildArtifact(artifact.ID, artifact.Kind)
}

func artifactLooksLikeEntityResolutionLedger(artifact ArtifactSchemaProjection) bool {
	fields := artifactFieldSet(artifact.Fields)
	hasLocator := fields["item_id"] != "" || fields["source_locator"] != "" || fields["_source_locator"] != "" || fields["record_id"] != "" || fields["row_id"] != ""
	if !hasLocator {
		return false
	}
	hasSource := fields["source_value"] != "" || fields["source_field"] != "" || fields["evidence_refs"] != ""
	hasCanonical := fields["canonical_id"] != "" || fields["canonical_label"] != "" || fields["canonical_value"] != ""
	return hasSource && hasCanonical
}
