package dataworkflow

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type MissingJoinFieldFallbackInput struct {
	Current           dataquery.TaskPlan
	Records           []WorkflowRecord
	SchemaProjections []ArtifactSchemaProjection
	Violation         dataquery.DataTaskViolation
}

type InvalidRecordEnrichFallbackInput struct {
	Current           dataquery.TaskPlan
	Records           []WorkflowRecord
	SchemaProjections []ArtifactSchemaProjection
	Action            dataquery.DataAction
	InputPaths        []string
}

type MissingFieldRelationCandidate struct {
	Artifact     ArtifactSchemaProjection
	ValueField   string
	BaseFields   []string
	LookupFields []string
	MatchMode    string
	Evidence     []string
}

func HistoricalMissingJoinFieldFallbackPlan(input MissingJoinFieldFallbackInput) (dataquery.TaskPlan, bool) {
	if len(input.Records) == 0 {
		return dataquery.TaskPlan{}, false
	}
	for i := len(input.Records) - 1; i >= 0; i-- {
		violation, ok := JoinMissingFieldViolation(input.Records[i])
		if !ok {
			continue
		}
		input.Violation = violation
		fallback, ok := MissingJoinFieldFallbackPlan(input)
		if !ok {
			continue
		}
		if PlanActionSignatureAlreadySeenInRecords(input.Records, fallback) {
			continue
		}
		if strings.TrimSpace(fallback.WhyThisBatch) == "" {
			fallback.WhyThisBatch = "a previous join_records action found that a base artifact was missing a field; use an existing mapping/reference artifact to materialize that field before retrying the join"
		}
		if strings.TrimSpace(fallback.NextBatch) == "" {
			fallback.NextBatch = "retry the join_records stage using the newly enriched artifact, then continue to compute_contributions"
		}
		return fallback, true
	}
	return dataquery.TaskPlan{}, false
}

func MissingJoinFieldFallbackPlan(input MissingJoinFieldFallbackInput) (dataquery.TaskPlan, bool) {
	violation := input.Violation
	if strings.TrimSpace(violation.Code) != "field_contract_violation" ||
		strings.TrimSpace(violation.ActionKind) != string(dataquery.DataActionJoinRecords) {
		return dataquery.TaskPlan{}, false
	}
	missingField := firstNonEmpty(violation.MissingFields...)
	baseAlias := strings.Trim(strings.TrimSpace(violation.InputAlias), `"'`)
	if missingField == "" || baseAlias == "" {
		return dataquery.TaskPlan{}, false
	}
	projections := input.SchemaProjections
	if len(projections) == 0 {
		projections = ProjectArtifactSchemasNewestFirst(ArtifactsNewestFirst(input.Records))
	}
	base, ok := ArtifactSchemaByAlias(projections, baseAlias)
	if !ok || len(base.Fields) == 0 || ExistingField(base.Fields, missingField) != "" {
		return dataquery.TaskPlan{}, false
	}
	relation, ok := StructuralRelationForMissingTarget(projections, base, missingField, 8)
	if !ok {
		return dataquery.TaskPlan{}, false
	}
	mapping := relation.Artifact
	mappingAlias := firstArtifactAlias(mapping)
	if mappingAlias == "" {
		return dataquery.TaskPlan{}, false
	}
	outputArtifact := MissingFieldOutputArtifact(baseAlias, missingField)
	if ArtifactAliasExists(projections, outputArtifact) {
		return dataquery.TaskPlan{}, false
	}
	if normalizeAccessPath(outputArtifact) == normalizeAccessPath(mappingAlias) ||
		normalizeAccessPath(outputArtifact) == normalizeAccessPath(baseAlias) {
		return dataquery.TaskPlan{}, false
	}
	specs, err := compactJSON([]map[string]any{{
		"lookup_path":        mappingAlias,
		"base_fields":        relation.BaseFields,
		"lookup_fields":      relation.LookupFields,
		"lookup_value_field": relation.ValueField,
		"target_field":       missingField,
		"match_mode":         relation.MatchMode,
	}})
	if err != nil {
		return dataquery.TaskPlan{}, false
	}
	out := input.Current
	out.Status = "ready"
	out.Script = ""
	out.ContinueAfter = true
	out.Actions = []dataquery.DataAction{{
		ID:             "materialize_" + SafeActionName(missingField),
		Kind:           dataquery.DataActionEnrichRecords,
		Purpose:        fmt.Sprintf("materialize missing field %s on %s using an existing mapping/reference artifact", missingField, baseAlias),
		InputPaths:     []string{baseAlias, mappingAlias},
		OutputArtifact: outputArtifact,
		Params: map[string]string{
			"base_path":         baseAlias,
			"lookup_specs_json": specs,
		},
	}}
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = fmt.Sprintf("join_records failed because %s did not contain field %s; materialize that field first from existing mapping/reference rows", baseAlias, missingField)
	}
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = "retry the join_records stage using the newly enriched artifact, then continue to compute_contributions"
	}
	if PlanActionSignatureAlreadySeenInRecords(input.Records, out) {
		return dataquery.TaskPlan{}, false
	}
	return out, true
}

func InvalidRecordEnrichFallbackPlan(input InvalidRecordEnrichFallbackInput) (dataquery.TaskPlan, bool) {
	inputs := cleanStrings(input.InputPaths)
	if len(inputs) < 2 {
		return dataquery.TaskPlan{}, false
	}
	action := input.Action
	targetField := firstNonEmpty(
		HistoricalMissingJoinFieldName(input.Records),
		action.Params["target_field"],
		action.Params["output_field"],
		action.Params["derived_field"],
	)
	if targetField == "" {
		return dataquery.TaskPlan{}, false
	}
	basePath := firstNonEmpty(
		action.Params["base_path"],
		action.Params["record_path"],
		action.Params["input_path"],
		inputs[0],
	)
	sourceFields := cleanStrings(append(
		parseActionStringList(firstNonEmpty(action.Params["source_fields"], action.Params["base_fields"], action.Params["left_fields"])),
		parseActionStringList(firstNonEmpty(action.Params["source_field"], action.Params["base_field"], action.Params["left_field"], action.Params["field"]))...,
	))
	projections := input.SchemaProjections
	if len(projections) == 0 {
		projections = ProjectArtifactSchemasNewestFirst(ArtifactsNewestFirst(input.Records))
	}
	if len(sourceFields) == 0 {
		if base, ok := ArtifactSchemaByAlias(projections, basePath); ok {
			sourceFields = SourceFieldsForMissingTarget(base.Fields, targetField, 6)
		}
	}
	if len(sourceFields) == 0 {
		return dataquery.TaskPlan{}, false
	}
	mappingPath := firstNonEmpty(
		action.Params["mapping_path"],
		action.Params["reference_path"],
		action.Params["lookup_path"],
	)
	if mappingPath == "" {
		for _, candidate := range inputs {
			if normalizeAccessPath(candidate) == normalizeAccessPath(basePath) {
				continue
			}
			mappingPath = candidate
			break
		}
	}
	if mappingPath == "" {
		return dataquery.TaskPlan{}, false
	}
	valueField := firstNonEmpty(
		action.Params["mapping_value_field"],
		action.Params["reference_value_field"],
		action.Params["canonical_id_field"],
		targetField,
	)
	mappingFields := cleanStrings(append(
		parseActionStringList(firstNonEmpty(action.Params["mapping_source_fields"], action.Params["reference_fields"], action.Params["lookup_fields"])),
		parseActionStringList(firstNonEmpty(action.Params["mapping_source_field"], action.Params["reference_field"], action.Params["lookup_field"]))...,
	))
	if mapping, ok := ArtifactSchemaByAlias(projections, mappingPath); ok {
		if exact, _ := MappingValueFieldForMissingTarget(mapping.Fields, targetField); exact != "" {
			valueField = exact
		}
		if len(mappingFields) == 0 {
			mappingFields = ReferenceSourceFieldsForMissingTarget(mapping.Fields, valueField, targetField, 8)
		}
	}
	spec := map[string]any{
		"lookup_path":        mappingPath,
		"base_fields":        sourceFields,
		"lookup_value_field": valueField,
		"target_field":       targetField,
		"match_mode":         "contains",
	}
	if len(mappingFields) > 0 {
		spec["lookup_fields"] = mappingFields
	}
	specs, err := compactJSON([]map[string]any{spec})
	if err != nil {
		return dataquery.TaskPlan{}, false
	}
	out := input.Current
	out.Status = "ready"
	out.Script = ""
	out.ContinueAfter = true
	out.Actions = []dataquery.DataAction{{
		ID:             "materialize_" + SafeActionName(targetField),
		Kind:           dataquery.DataActionEnrichRecords,
		Purpose:        fmt.Sprintf("materialize field %s on %s using a mapping/reference input", targetField, basePath),
		InputPaths:     []string{basePath, mappingPath},
		OutputArtifact: MissingFieldOutputArtifact(basePath, targetField),
		Params: map[string]string{
			"base_path":         basePath,
			"lookup_specs_json": specs,
		},
	}}
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = fmt.Sprintf("derive_fields cannot consume multiple record sets; convert the lookup/reference mapping into enrich_records to materialize %s first", targetField)
	}
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = "continue with join_records or compute_contributions after the enriched artifact materializes"
	}
	return out, true
}

func JoinMissingFieldViolation(record WorkflowRecord) (dataquery.DataTaskViolation, bool) {
	for _, violation := range record.Violations {
		if strings.TrimSpace(violation.Code) != "field_contract_violation" {
			continue
		}
		if strings.TrimSpace(violation.ActionKind) != string(dataquery.DataActionJoinRecords) {
			continue
		}
		if strings.TrimSpace(violation.InputAlias) == "" || len(violation.MissingFields) == 0 {
			continue
		}
		return violation, true
	}
	return dataquery.DataTaskViolation{}, false
}

func HistoricalMissingJoinFieldName(records []WorkflowRecord) string {
	for i := len(records) - 1; i >= 0; i-- {
		violation, ok := JoinMissingFieldViolation(records[i])
		if ok && len(violation.MissingFields) > 0 {
			return strings.TrimSpace(violation.MissingFields[0])
		}
	}
	return ""
}

func StructuralRelationForMissingTarget(projections []ArtifactSchemaProjection, base ArtifactSchemaProjection, missingField string, limit int) (MissingFieldRelationCandidate, bool) {
	if limit <= 0 {
		limit = len(projections)
	}
	baseAlias := firstArtifactAlias(base)
	if baseAlias == "" || len(base.Fields) == 0 || ExistingField(base.Fields, missingField) != "" {
		return MissingFieldRelationCandidate{}, false
	}
	var best MissingFieldRelationCandidate
	bestScore := -1
	for _, artifact := range projections {
		alias := firstArtifactAlias(artifact)
		if alias == "" || normalizeAccessPath(alias) == normalizeAccessPath(baseAlias) || len(artifact.Fields) == 0 {
			continue
		}
		if artifact.NodeClass == ArtifactNodeClassDiagnosticChild || artifact.NodeClass == ArtifactNodeClassWorkflowLedger {
			continue
		}
		valueField := ExistingField(artifact.Fields, missingField)
		if valueField == "" {
			continue
		}
		baseFields, lookupFields := structuralRelationFields(base.Fields, artifact.Fields, valueField, missingField, 6)
		if len(baseFields) == 0 || len(lookupFields) == 0 {
			continue
		}
		score := len(baseFields)
		if ArtifactLineageContains(artifact, baseAlias) || ArtifactLineageContains(base, alias) {
			score += 4
		}
		if artifactKindHasPrefix(artifact.Kind, dataquery.DataActionNormalizeEntities, dataquery.DataActionEnrichRecords, dataquery.DataActionApplyResolutions) ||
			strings.Contains(strings.ToLower(strings.TrimSpace(artifact.Kind)), "entity_resolution") {
			score += 2
		}
		if score > bestScore {
			bestScore = score
			best = MissingFieldRelationCandidate{
				Artifact:     artifact,
				ValueField:   valueField,
				BaseFields:   baseFields,
				LookupFields: lookupFields,
				MatchMode:    "exact",
				Evidence: cleanStrings([]string{
					"target field exists exactly in lookup artifact",
					"structural common key fields exist between base and lookup",
				}),
			}
		}
	}
	if bestScore < 0 {
		return MissingFieldRelationCandidate{}, false
	}
	return best, true
}

func structuralRelationFields(baseFields, lookupFields []string, valueField, missingField string, limit int) ([]string, []string) {
	if limit <= 0 {
		return nil, nil
	}
	lookupSet := artifactFieldSet(lookupFields)
	valueLower := strings.ToLower(strings.TrimSpace(valueField))
	missingLower := strings.ToLower(strings.TrimSpace(missingField))
	var baseOut []string
	var lookupOut []string
	seen := map[string]bool{}
	for _, field := range cleanStrings(baseFields) {
		key := strings.ToLower(strings.TrimSpace(field))
		if key == "" || seen[key] || key == valueLower || key == missingLower || !FieldUsableForRecordJoin(field) {
			continue
		}
		lookupField := lookupSet[key]
		if lookupField == "" || !FieldUsableForRecordJoin(lookupField) {
			continue
		}
		seen[key] = true
		baseOut = append(baseOut, field)
		lookupOut = append(lookupOut, lookupField)
		if len(baseOut) >= limit {
			break
		}
	}
	return baseOut, lookupOut
}

func MappingForMissingTarget(projections []ArtifactSchemaProjection, base ArtifactSchemaProjection, missingField string, limit int) (ArtifactSchemaProjection, string, []string, string, bool) {
	baseAlias := firstArtifactAlias(base)
	bestScore := -1
	var best ArtifactSchemaProjection
	bestValue := ""
	var bestSources []string
	bestMode := "exact"
	for _, artifact := range projections {
		alias := firstArtifactAlias(artifact)
		if alias == "" || normalizeAccessPath(alias) == normalizeAccessPath(baseAlias) || len(artifact.Fields) == 0 {
			continue
		}
		valueField, exactValue := MappingValueFieldForMissingTarget(artifact.Fields, missingField)
		if valueField == "" {
			continue
		}
		sourceFields := ReferenceSourceFieldsForMissingTarget(artifact.Fields, valueField, missingField, limit)
		if len(sourceFields) == 0 {
			continue
		}
		score := 0
		if exactValue {
			score += 8
		}
		if ArtifactLooksLikeMappingProjection(artifact) {
			score += 4
		}
		score += FieldTokenOverlapScore(artifact.Fields, missingField)
		score += len(sourceFields)
		mode := "exact"
		if !artifactKindHasPrefix(artifact.Kind, dataquery.DataActionNormalizeEntities) &&
			!strings.Contains(strings.ToLower(strings.TrimSpace(artifact.Kind)), "entity_resolution") {
			mode = "contains"
		}
		if score > bestScore {
			bestScore = score
			best = artifact
			bestValue = valueField
			bestSources = sourceFields
			bestMode = mode
		}
	}
	if bestScore < 0 {
		return ArtifactSchemaProjection{}, "", nil, "", false
	}
	return best, bestValue, bestSources, bestMode, true
}

func MappingValueFieldForMissingTarget(fields []string, missingField string) (string, bool) {
	if exact := ExistingField(fields, missingField); exact != "" {
		return exact, true
	}
	value := ReferenceValueField(fields)
	if value == "" {
		return "", false
	}
	return value, false
}

func SourceFieldsForMissingTarget(fields []string, missingField string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	tokens := FieldTokens(missingField)
	var out []string
	seen := map[string]bool{}
	add := func(field string) {
		field = strings.TrimSpace(field)
		key := strings.ToLower(field)
		if field == "" || seen[key] || strings.HasPrefix(key, "_") || key == strings.ToLower(strings.TrimSpace(missingField)) {
			return
		}
		seen[key] = true
		out = append(out, field)
	}
	for _, field := range fields {
		if FieldTokenOverlap(FieldTokens(field), tokens) > 0 {
			add(field)
			if len(out) >= limit {
				return out
			}
		}
	}
	for _, field := range CandidateSourceFields(fields, limit-len(out)) {
		add(field)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func ReferenceSourceFieldsForMissingTarget(fields []string, valueField, missingField string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(field string) {
		field = strings.TrimSpace(field)
		key := strings.ToLower(field)
		if field == "" || seen[key] || strings.HasPrefix(key, "_") || key == strings.ToLower(strings.TrimSpace(valueField)) {
			return
		}
		seen[key] = true
		out = append(out, field)
	}
	tokens := FieldTokens(missingField)
	for _, field := range fields {
		if FieldTokenOverlap(FieldTokens(field), tokens) > 0 {
			add(field)
			if len(out) >= limit {
				return out
			}
		}
	}
	for _, field := range CandidateReferenceSourceFields(fields, valueField, limit-len(out)) {
		add(field)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func FieldTokenOverlapScore(fields []string, target string) int {
	targetTokens := FieldTokens(target)
	score := 0
	for _, field := range fields {
		score += FieldTokenOverlap(FieldTokens(field), targetTokens)
	}
	return score
}

func FieldTokens(field string) map[string]bool {
	out := map[string]bool{}
	var b strings.Builder
	flush := func() {
		token := strings.ToLower(strings.TrimSpace(b.String()))
		b.Reset()
		if len(token) >= 3 {
			out[token] = true
		}
	}
	for _, r := range field {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func FieldTokenOverlap(left, right map[string]bool) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	score := 0
	for token := range left {
		if right[token] {
			score++
		}
	}
	return score
}

func MissingFieldOutputArtifact(baseAlias, missingField string) string {
	base := strings.TrimSpace(path.Base(baseAlias))
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = "enriched_records"
	}
	return stem + "_with_" + SafeActionName(missingField) + ".json"
}

func SafeActionName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "field"
	}
	return out
}

func ArtifactAliasExists(projections []ArtifactSchemaProjection, alias string) bool {
	_, ok := ArtifactSchemaByAlias(projections, alias)
	return ok
}

func ArtifactLooksLikeMappingProjection(artifact ArtifactSchemaProjection) bool {
	if artifactKindHasPrefix(artifact.Kind, dataquery.DataActionNormalizeEntities) ||
		strings.Contains(strings.ToLower(strings.TrimSpace(artifact.Kind)), "entity_resolution") {
		return true
	}
	fields := artifactFieldSet(artifact.Fields)
	if fields["source_value"] != "" && (fields["canonical_id"] != "" || fields["canonical_label"] != "") {
		return true
	}
	valueField := ReferenceValueField(artifact.Fields)
	return valueField != "" && len(CandidateReferenceSourceFields(artifact.Fields, valueField, 3)) > 0
}

func ReferenceValueField(fields []string) string {
	if field := ExistingField(fields, "canonical_id", "canonical_label", "value", "target_value", "code"); field != "" {
		return field
	}
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || strings.HasPrefix(lower, "_") {
			continue
		}
		if strings.Contains(lower, "canonical") && (strings.Contains(lower, "id") || strings.Contains(lower, "code") || strings.Contains(lower, "value")) {
			return strings.TrimSpace(field)
		}
	}
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || strings.HasPrefix(lower, "_") {
			continue
		}
		if strings.HasSuffix(lower, "_id") || strings.HasSuffix(lower, "_code") {
			return strings.TrimSpace(field)
		}
	}
	return ""
}

func CandidateSourceFields(fields []string, limit int) []string {
	var out []string
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || strings.HasPrefix(lower, "_") || lower == "amount" || lower == "year" {
			continue
		}
		if strings.Contains(lower, "name") || strings.Contains(lower, "raw") || strings.Contains(lower, "description") || strings.Contains(lower, "purpose") || strings.Contains(lower, "label") || strings.Contains(lower, "text") || strings.Contains(lower, "title") {
			out = append(out, trimmed)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func CandidateReferenceSourceFields(fields []string, valueField string, limit int) []string {
	var out []string
	valueLower := strings.ToLower(strings.TrimSpace(valueField))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || strings.HasPrefix(lower, "_") || lower == valueLower {
			continue
		}
		if strings.Contains(lower, "negative") || strings.Contains(lower, "exclude") || strings.Contains(lower, "except") {
			continue
		}
		if strings.Contains(lower, "source") || strings.Contains(lower, "term") || strings.Contains(lower, "name") || strings.Contains(lower, "label") || strings.Contains(lower, "keyword") || strings.Contains(lower, "example") || strings.Contains(lower, "description") || strings.Contains(lower, "text") {
			out = append(out, trimmed)
			if len(out) >= limit {
				return out
			}
		}
	}
	if len(out) == 0 {
		for _, field := range fields {
			trimmed := strings.TrimSpace(field)
			lower := strings.ToLower(trimmed)
			if trimmed == "" || strings.HasPrefix(lower, "_") || lower == valueLower {
				continue
			}
			if strings.Contains(lower, "negative") || strings.Contains(lower, "exclude") || strings.Contains(lower, "except") {
				continue
			}
			out = append(out, trimmed)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func ExistingField(fields []string, candidates ...string) string {
	set := artifactFieldSet(fields)
	for _, candidate := range candidates {
		if field := set[strings.ToLower(strings.TrimSpace(candidate))]; field != "" {
			return field
		}
	}
	return ""
}

func compactJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
