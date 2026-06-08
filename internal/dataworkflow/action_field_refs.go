package dataworkflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ActionFieldRequirement struct {
	Role     string   `json:"role,omitempty"`
	Path     string   `json:"path,omitempty"`
	Fields   []string `json:"fields,omitempty"`
	Explicit bool     `json:"explicit,omitempty"`
}

type ActionFieldReferenceGuardInput struct {
	Action            dataquery.DataAction
	ActionIndex       int
	SchemaProjections []ArtifactSchemaProjection
}

// SingleRecordSetActionFieldRefs returns the input-field contract for typed
// actions that consume exactly one record-shaped artifact. It is intentionally
// domain-neutral: it reads only action kind and structured params.
func SingleRecordSetActionFieldRefs(kind dataquery.DataActionKind, action dataquery.DataAction) []string {
	switch NormalizeActionKind(kind) {
	case dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields:
		return DeriveActionSourceFieldRefs(action)
	case dataquery.DataActionGroupRecords:
		return GroupRecordsActionFieldRefs(action)
	case dataquery.DataActionExpandRecords:
		return parseActionStringList(firstNonEmpty(
			action.Params["source_field"],
			action.Params["input_field"],
			action.Params["field"],
			action.Params["value_field"],
		))
	case dataquery.DataActionFilterRecords:
		return FilterActionFieldRefs(action)
	case dataquery.DataActionQualifyRecords:
		return QualifyActionFieldRefs(action)
	default:
		return nil
	}
}

func DeriveActionSourceFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	for _, spec := range actionObjectListParam(action.Params, "field_specs_json", "extract_specs_json", "derive_specs_json", "transforms_json", "field_specs", "extract_specs", "derive_specs", "transforms") {
		op := strings.ToLower(strings.TrimSpace(firstNonEmpty(
			mapStringValue(spec, "operation"),
			mapStringValue(spec, "op"),
			mapStringValue(spec, "transform"),
		)))
		sources := fieldRefsFromSpec(spec, "source_field", "input_field", "field", "value_field")
		sources = append(sources, fieldRefsFromSpec(spec, "source_fields", "input_fields", "fields")...)
		if len(sources) == 0 && op != "constant" {
			sources = append(sources, fieldRefsFromSpec(spec, "left_field", "right_field")...)
		}
		fields = append(fields, sources...)
	}
	fields = append(fields, parseActionStringList(firstNonEmpty(
		action.Params["source_field"],
		action.Params["input_field"],
		action.Params["field"],
		action.Params["value_field"],
	))...)
	return cleanStrings(fields)
}

func GroupRecordsActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	fields = append(fields, parseActionStringList(firstNonEmpty(
		action.Params["group_fields"],
		action.Params["group_by_fields"],
		action.Params["key_fields"],
		action.Params["group_field"],
		action.Params["group_by"],
		action.Params["key_field"],
	))...)
	fields = append(fields, parseActionStringList(firstNonEmpty(
		action.Params["text_fields"],
		action.Params["concat_fields"],
		action.Params["aggregate_fields"],
		action.Params["source_fields"],
		action.Params["text_field"],
		action.Params["source_field"],
		action.Params["field"],
	))...)
	fields = append(fields, parseActionStringList(firstNonEmpty(
		action.Params["first_fields"],
		action.Params["copy_fields"],
		action.Params["preserve_fields"],
		action.Params["keep_fields"],
	))...)
	return cleanStrings(fields)
}

func FilterActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	for _, spec := range actionObjectListParam(action.Params, "filters_json", "filters") {
		fields = append(fields, fieldRefsFromSpec(spec, "field", "source_field", "input_field")...)
	}
	fields = append(fields, parseActionStringList(action.Params["filter_field"])...)
	return cleanStrings(fields)
}

func QualifyActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	fields = append(fields, FilterActionFieldRefs(action)...)
	for _, spec := range actionObjectListParam(action.Params, "reject_filters_json", "exclude_filters_json", "block_filters_json", "reject_filters", "exclude_filters", "block_filters") {
		fields = append(fields, fieldRefsFromSpec(spec, "field", "source_field", "input_field")...)
	}
	fields = append(fields, parseActionStringList(firstNonEmpty(action.Params["required_fields"], action.Params["non_empty_fields"]))...)
	fields = append(fields, parseActionStringList(firstNonEmpty(action.Params["evidence_fields"], action.Params["required_evidence_fields"]))...)
	fields = append(fields, parseActionStringList(firstNonEmpty(action.Params["status_fields"], action.Params["generated_status_fields"]))...)
	return cleanStrings(fields)
}

func ComputeContributionActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	fields = append(fields, parseActionStringList(action.Params["value_field"])...)
	fields = append(fields, parseActionStringList(action.Params["group_key_field"])...)
	fields = append(fields, FilterActionFieldRefs(action)...)
	return cleanStrings(fields)
}

func ActionFieldReferenceGuardResult(input ActionFieldReferenceGuardInput) GuardResult {
	if len(input.SchemaProjections) == 0 {
		return GuardResult{}
	}
	action := input.Action
	switch NormalizeActionKind(action.Kind) {
	case dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields:
		inputs := cleanStrings(action.InputPaths)
		if len(inputs) != 1 {
			return GuardResult{}
		}
		return missingFieldReferenceGuard(input, inputs[0], MissingDeriveFieldInputs(input.SchemaProjections, inputs[0], action))
	case dataquery.DataActionGroupRecords:
		inputs := cleanStrings(action.InputPaths)
		if len(inputs) != 1 {
			return GuardResult{}
		}
		return missingFieldReferenceGuard(input, inputs[0], MissingFieldsOnArtifactSchema(input.SchemaProjections, inputs[0], GroupRecordsActionFieldRefs(action)))
	case dataquery.DataActionExpandRecords:
		inputs := cleanStrings(action.InputPaths)
		if len(inputs) != 1 {
			return GuardResult{}
		}
		fields := parseActionStringList(firstNonEmpty(
			action.Params["source_field"],
			action.Params["input_field"],
			action.Params["field"],
			action.Params["value_field"],
		))
		if len(fields) == 0 {
			return GuardResult{}
		}
		return missingFieldReferenceGuard(input, inputs[0], MissingFieldsOnArtifactSchema(input.SchemaProjections, inputs[0], fields))
	case dataquery.DataActionFilterRecords:
		inputs := cleanStrings(action.InputPaths)
		if len(inputs) != 1 {
			return GuardResult{}
		}
		fields := FilterActionFieldRefs(action)
		if len(fields) == 0 {
			return GuardResult{}
		}
		return missingFieldReferenceGuard(input, inputs[0], MissingFieldsOnArtifactSchema(input.SchemaProjections, inputs[0], fields))
	case dataquery.DataActionQualifyRecords:
		inputs := cleanStrings(action.InputPaths)
		if len(inputs) != 1 {
			return GuardResult{}
		}
		fields := QualifyActionFieldRefs(action)
		if len(fields) == 0 {
			return GuardResult{}
		}
		return missingFieldReferenceGuard(input, inputs[0], MissingFieldsOnArtifactSchema(input.SchemaProjections, inputs[0], fields))
	case dataquery.DataActionComputeContribs:
		inputs := cleanStrings(action.InputPaths)
		if len(inputs) != 1 {
			return GuardResult{}
		}
		fields := ComputeContributionActionFieldRefs(action)
		if len(fields) == 0 {
			return GuardResult{}
		}
		return missingFieldReferenceGuard(input, inputs[0], MissingFieldsOnArtifactSchema(input.SchemaProjections, inputs[0], fields))
	case dataquery.DataActionJoinRecords:
		for _, req := range JoinActionFieldRequirements(action) {
			if guard := missingFieldReferenceGuard(input, req.Path, MissingFieldsOnArtifactSchema(input.SchemaProjections, req.Path, req.Fields)); !guard.Empty() {
				return guard
			}
		}
	case dataquery.DataActionMappingCandidate, dataquery.DataActionNormalizeEntities:
		return normalizeEntityFieldReferenceGuard(input)
	case dataquery.DataActionEnrichRecords:
		for _, req := range EnrichActionFieldRequirements(action) {
			if guard := missingFieldReferenceGuard(input, req.Path, MissingFieldsOnArtifactSchema(input.SchemaProjections, req.Path, req.Fields)); !guard.Empty() {
				return guard
			}
		}
	}
	return GuardResult{}
}

func missingFieldReferenceGuard(input ActionFieldReferenceGuardInput, alias string, missing []string) GuardResult {
	return MissingFieldContractGuardResult(MissingFieldContractGuardInput{
		Action:            input.Action,
		ActionIndex:       input.ActionIndex,
		InputAlias:        alias,
		MissingFields:     missing,
		SchemaProjections: input.SchemaProjections,
	})
}

func normalizeEntityFieldReferenceGuard(input ActionFieldReferenceGuardInput) GuardResult {
	requirements, skip := NormalizeEntityActionFieldRequirements(input.Action)
	if skip {
		return GuardResult{}
	}
	sourceReq, sourceOK := actionFieldRequirementByRole(requirements, "source")
	referenceReq, referenceOK := actionFieldRequirementByRole(requirements, "reference")
	if !sourceOK {
		return GuardResult{}
	}
	if referenceOK && !sourceReq.Explicit && !referenceReq.Explicit && len(sourceReq.Fields) > 0 {
		sourceMissing := MissingFieldsOnArtifactSchema(input.SchemaProjections, sourceReq.Path, sourceReq.Fields)
		referenceCanBeSource := len(MissingFieldsOnArtifactSchema(input.SchemaProjections, referenceReq.Path, sourceReq.Fields)) == 0
		if len(sourceMissing) > 0 && referenceCanBeSource {
			sourceReq.Path, referenceReq.Path = referenceReq.Path, sourceReq.Path
		}
	}
	if guard := missingFieldReferenceGuard(input, sourceReq.Path, MissingFieldsOnArtifactSchema(input.SchemaProjections, sourceReq.Path, sourceReq.Fields)); !guard.Empty() {
		return guard
	}
	if referenceOK {
		if guard := missingFieldReferenceGuard(input, referenceReq.Path, MissingFieldsOnArtifactSchema(input.SchemaProjections, referenceReq.Path, referenceReq.Fields)); !guard.Empty() {
			return guard
		}
	}
	return GuardResult{}
}

func actionFieldRequirementByRole(requirements []ActionFieldRequirement, role string) (ActionFieldRequirement, bool) {
	role = strings.TrimSpace(role)
	for _, req := range requirements {
		if strings.TrimSpace(req.Role) == role {
			return req, true
		}
	}
	return ActionFieldRequirement{}, false
}

func JoinActionFieldRequirements(action dataquery.DataAction) []ActionFieldRequirement {
	inputs := cleanStrings(action.InputPaths)
	leftPath := firstNonEmpty(strings.TrimSpace(action.Params["left_path"]), strings.TrimSpace(action.Params["base_path"]))
	rightPath := firstNonEmpty(strings.TrimSpace(action.Params["right_path"]), strings.TrimSpace(action.Params["lookup_path"]), strings.TrimSpace(action.Params["reference_path"]))
	if leftPath == "" && len(inputs) >= 1 {
		leftPath = inputs[0]
	}
	if rightPath == "" && len(inputs) >= 2 {
		rightPath = inputs[1]
	}
	leftFields := cleanStrings(append(
		parseActionStringList(firstNonEmpty(
			action.Params["left_fields"],
			action.Params["left_fields_json"],
			action.Params["left_key_fields"],
			action.Params["left_key_fields_json"],
			action.Params["base_fields"],
			action.Params["base_fields_json"],
			action.Params["base_key_fields"],
			action.Params["base_key_fields_json"],
			action.Params["join_fields"],
			action.Params["join_fields_json"],
		)),
		parseActionStringList(firstNonEmpty(action.Params["left_field"], action.Params["base_field"], action.Params["join_field"]))...,
	))
	rightFields := cleanStrings(append(
		parseActionStringList(firstNonEmpty(
			action.Params["right_fields"],
			action.Params["right_fields_json"],
			action.Params["right_key_fields"],
			action.Params["right_key_fields_json"],
			action.Params["lookup_fields"],
			action.Params["lookup_fields_json"],
			action.Params["lookup_key_fields"],
			action.Params["lookup_key_fields_json"],
			action.Params["reference_fields"],
			action.Params["reference_fields_json"],
			action.Params["reference_key_fields"],
			action.Params["reference_key_fields_json"],
			action.Params["join_fields"],
			action.Params["join_fields_json"],
		)),
		parseActionStringList(firstNonEmpty(action.Params["right_field"], action.Params["lookup_field"], action.Params["reference_field"], action.Params["join_field"]))...,
	))
	if len(rightFields) == 0 && len(leftFields) > 0 {
		rightFields = append([]string(nil), leftFields...)
	}
	return cleanFieldRequirements([]ActionFieldRequirement{
		{Role: "left", Path: leftPath, Fields: leftFields, Explicit: strings.TrimSpace(action.Params["left_path"]) != "" || strings.TrimSpace(action.Params["base_path"]) != ""},
		{Role: "right", Path: rightPath, Fields: rightFields, Explicit: strings.TrimSpace(action.Params["right_path"]) != "" || strings.TrimSpace(action.Params["lookup_path"]) != "" || strings.TrimSpace(action.Params["reference_path"]) != ""},
	})
}

func NormalizeEntityActionFieldRequirements(action dataquery.DataAction) (requirements []ActionFieldRequirement, skip bool) {
	if len(actionObjectListParam(action.Params, "resolutions_json", "mappings_json", "entity_resolutions_json", "resolutions", "mappings", "entity_resolutions")) > 0 {
		return nil, true
	}
	sourceFields := cleanStrings(append(
		parseActionStringList(firstNonEmpty(action.Params["source_fields"], action.Params["source_fields_json"], action.Params["name_fields"], action.Params["name_fields_json"], action.Params["value_fields"], action.Params["value_fields_json"])),
		parseActionStringList(firstNonEmpty(action.Params["source_field"], action.Params["name_field"], action.Params["value_field"], action.Params["field"]))...,
	))
	referenceFields := cleanStrings(append(
		parseActionStringList(firstNonEmpty(action.Params["reference_fields"], action.Params["reference_fields_json"], action.Params["reference_name_fields"], action.Params["reference_name_fields_json"], action.Params["mapping_source_fields"], action.Params["mapping_source_fields_json"], action.Params["lookup_fields"], action.Params["lookup_fields_json"])),
		parseActionStringList(firstNonEmpty(action.Params["reference_field"], action.Params["reference_name_field"], action.Params["mapping_source_field"], action.Params["lookup_field"], action.Params["lookup_source_field"]))...,
	))
	filterFields := FilterActionFieldRefs(action)
	canonicalIDField := firstNonEmpty(
		strings.TrimSpace(action.Params["canonical_id_field"]),
		strings.TrimSpace(action.Params["id_field"]),
		strings.TrimSpace(action.Params["reference_value_field"]),
		strings.TrimSpace(action.Params["lookup_value_field"]),
	)
	canonicalLabelField := firstNonEmpty(
		strings.TrimSpace(action.Params["canonical_label_field"]),
		strings.TrimSpace(action.Params["label_field"]),
	)
	explicitSourcePath := firstNonEmpty(
		strings.TrimSpace(action.Params["source_path"]),
		strings.TrimSpace(action.Params["base_path"]),
		strings.TrimSpace(action.Params["record_path"]),
	)
	explicitReferencePath := firstNonEmpty(
		strings.TrimSpace(action.Params["reference_path"]),
		strings.TrimSpace(action.Params["mapping_path"]),
		strings.TrimSpace(action.Params["lookup_path"]),
	)
	inputs := cleanStrings(action.InputPaths)
	sourcePath := explicitSourcePath
	if sourcePath == "" && len(inputs) > 0 {
		sourcePath = inputs[0]
	}
	referencePath := explicitReferencePath
	if referencePath == "" && len(inputs) > 1 {
		referencePath = inputs[1]
	}
	sourceRequired := cleanStrings(append(sourceFields, filterFields...))
	referenceRequired := append([]string{}, referenceFields...)
	if canonicalIDField != "" {
		referenceRequired = append(referenceRequired, canonicalIDField)
	}
	if canonicalLabelField != "" {
		referenceRequired = append(referenceRequired, canonicalLabelField)
	}
	return cleanFieldRequirements([]ActionFieldRequirement{
		{Role: "source", Path: sourcePath, Fields: sourceRequired, Explicit: explicitSourcePath != ""},
		{Role: "reference", Path: referencePath, Fields: referenceRequired, Explicit: explicitReferencePath != ""},
	}), false
}

func EnrichActionFieldRequirements(action dataquery.DataAction) []ActionFieldRequirement {
	inputs := cleanStrings(action.InputPaths)
	basePath := firstNonEmpty(strings.TrimSpace(action.Params["base_path"]), firstActionInput(inputs, 0))
	if basePath == "" {
		return nil
	}
	var requirements []ActionFieldRequirement
	for _, spec := range actionObjectListParam(action.Params, "lookup_specs_json", "enrich_specs_json", "mapping_specs_json", "map_specs_json", "lookup_specs", "enrich_specs", "mapping_specs", "map_specs") {
		specBase := firstNonEmpty(mapStringValue(spec, "base_path"), basePath)
		baseFields := cleanStrings(append(
			fieldRefsFromSpec(spec, "base_fields", "source_fields", "left_fields"),
			fieldRefsFromSpec(spec, "base_field", "source_field", "left_field")...,
		))
		requirements = append(requirements, ActionFieldRequirement{Role: "base", Path: specBase, Fields: baseFields})
		lookupPath := firstNonEmpty(
			mapStringValue(spec, "lookup_path"),
			mapStringValue(spec, "mapping_path"),
			mapStringValue(spec, "reference_path"),
			firstActionInput(inputs, 1),
		)
		lookupFields := cleanStrings(append(
			fieldRefsFromSpec(spec, "lookup_fields", "mapping_fields", "reference_fields"),
			fieldRefsFromSpec(spec, "lookup_field", "mapping_field", "reference_field", "lookup_value_field", "mapping_value_field", "reference_value_field")...,
		))
		requirements = append(requirements, ActionFieldRequirement{Role: "lookup", Path: lookupPath, Fields: lookupFields})
	}
	return cleanFieldRequirements(requirements)
}

func MissingDeriveFieldInputs(projections []ArtifactSchemaProjection, alias string, action dataquery.DataAction) []string {
	projection, ok := ArtifactSchemaByAlias(projections, alias)
	if !ok || len(projection.Fields) == 0 {
		return nil
	}
	known := artifactFieldSet(projection.Fields)
	var missing []string
	seenMissing := map[string]bool{}
	specs := actionObjectListParam(action.Params, "field_specs_json", "derive_specs_json", "extract_specs_json", "transforms_json", "field_specs", "derive_specs", "extract_specs", "transforms")
	for _, spec := range specs {
		op := strings.ToLower(strings.TrimSpace(firstNonEmpty(
			mapStringValue(spec, "operation"),
			mapStringValue(spec, "op"),
			mapStringValue(spec, "transform"),
		)))
		sources := fieldRefsFromSpec(spec, "source_field", "input_field", "field", "value_field")
		sources = append(sources, fieldRefsFromSpec(spec, "source_fields", "input_fields", "fields")...)
		if len(sources) == 0 && op != "constant" {
			sources = append(sources, fieldRefsFromSpec(spec, "left_field", "right_field")...)
		}
		cleanSources := cleanStrings(sources)
		if deriveOperationAllowsPartialSources(op) && len(cleanSources) > 0 {
			knownCount := 0
			for _, source := range cleanSources {
				if known[strings.ToLower(strings.TrimSpace(source))] != "" {
					knownCount++
				}
			}
			if knownCount > 0 {
				addDeriveTargetFields(known, spec)
				continue
			}
		}
		for _, source := range cleanSources {
			key := strings.ToLower(strings.TrimSpace(source))
			if key == "" || known[key] != "" {
				continue
			}
			if !seenMissing[key] {
				missing = append(missing, source)
				seenMissing[key] = true
			}
		}
		addDeriveTargetFields(known, spec)
	}
	sort.Strings(missing)
	return missing
}

func addDeriveTargetFields(known map[string]string, spec map[string]any) {
	for _, target := range fieldRefsFromSpec(spec, "target_field", "output_field", "derived_field", "name") {
		target = strings.TrimSpace(target)
		if target != "" {
			known[strings.ToLower(target)] = target
		}
	}
}

func deriveOperationAllowsPartialSources(op string) bool {
	normalized := strings.ToLower(strings.TrimSpace(op))
	normalized = strings.ReplaceAll(normalized, " ", "")
	switch normalized {
	case "concat", "join", "join_fields", "coalesce", "first_non_empty",
		"concat/join_fields", "join_fields/concat", "concat/join", "join/concat",
		"coalesce/first_non_empty", "first_non_empty/coalesce":
		return true
	default:
		return false
	}
}

func fieldRefsFromSpec(spec map[string]any, keys ...string) []string {
	var out []string
	for _, key := range keys {
		value, ok := spec[key]
		if !ok {
			continue
		}
		out = append(out, anyStringList(value)...)
	}
	return cleanStrings(out)
}

func cleanFieldRequirements(in []ActionFieldRequirement) []ActionFieldRequirement {
	out := make([]ActionFieldRequirement, 0, len(in))
	for _, req := range in {
		req.Role = strings.TrimSpace(req.Role)
		req.Path = strings.TrimSpace(req.Path)
		req.Fields = cleanStrings(req.Fields)
		if req.Path == "" || len(req.Fields) == 0 {
			continue
		}
		out = append(out, req)
	}
	return out
}

func firstActionInput(inputs []string, index int) string {
	if index < 0 || index >= len(inputs) {
		return ""
	}
	return strings.TrimSpace(inputs[index])
}

func anyStringList(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return parseActionStringList(typed)
	case []string:
		return cleanStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, anyStringList(item)...)
		}
		return cleanStrings(out)
	default:
		return cleanStrings([]string{fmt.Sprint(typed)})
	}
}

func parseActionStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var stringsValue []string
		if err := json.Unmarshal([]byte(raw), &stringsValue); err == nil {
			return cleanStrings(stringsValue)
		}
		var anyValue []any
		if err := json.Unmarshal([]byte(raw), &anyValue); err == nil {
			out := make([]string, 0, len(anyValue))
			for _, item := range anyValue {
				if item == nil {
					continue
				}
				out = append(out, strings.TrimSpace(fmt.Sprint(item)))
			}
			return cleanStrings(out)
		}
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '|', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	return cleanStrings(fields)
}
