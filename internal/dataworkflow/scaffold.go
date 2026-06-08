package dataworkflow

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ActionScaffold struct {
	Kind           string            `json:"kind"`
	Executable     bool              `json:"executable,omitempty"`
	UseWhen        string            `json:"use_when,omitempty"`
	InputPath      string            `json:"input_path,omitempty"`
	InputPaths     []string          `json:"input_paths,omitempty"`
	Fields         []string          `json:"fields,omitempty"`
	CommonFields   []string          `json:"common_fields,omitempty"`
	ParamsTemplate map[string]string `json:"params_template,omitempty"`
	Note           string            `json:"note,omitempty"`
}

type ActionScaffoldBuildInput struct {
	State     WorkflowStateView
	Artifacts []ArtifactSchemaProjection
	Limit     int
}

func BuildActionScaffolds(input ActionScaffoldBuildInput) []ActionScaffold {
	stage := strings.TrimSpace(input.State.NextStage)
	if stage == "" {
		stage = input.State.ComputedNextStage()
	}
	if !workflowStateNeedsActionScaffold(input.State) {
		return nil
	}
	if stage != StagePrepareContributionInputs && stage != StageComputeContributions && stage != StageNormalizeOrEnrichEntities {
		return nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	records := recordActionProjections(input.Artifacts)
	if len(records) == 0 {
		return nil
	}
	allowed := allowedActionSet(input.State.AllowedNextActions)
	var out []ActionScaffold
	appendAllowed := func(kind dataquery.DataActionKind, scaffolds []ActionScaffold) {
		if len(out) >= limit || !allowed[string(kind)] {
			return
		}
		for _, scaffold := range scaffolds {
			out = append(out, scaffold)
			if len(out) >= limit {
				return
			}
		}
	}
	appendAllowed(dataquery.DataActionDeriveFields, DeriveFieldScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionGroupRecords, GroupRecordScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionExtractFields, ExtractFieldScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionExpandRecords, ExpandRecordScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionEnrichRecords, EnrichRecordScaffolds(input.Artifacts, limit-len(out)))
	appendAllowed(dataquery.DataActionMappingCandidate, MappingCandidateScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionJoinRecords, JoinRecordScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionNormalizeEntities, NormalizeEntityScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionApplyResolutions, ApplyResolutionScaffolds(input.Artifacts, limit-len(out)))
	appendAllowed(dataquery.DataActionFilterRecords, FilterRecordScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionValueDistribution, ValueDistributionScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionQualifyRecords, QualifyRecordScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionComputeContribs, ComputeContributionScaffolds(records, limit-len(out)))
	return out
}

func workflowStateNeedsActionScaffold(state WorkflowStateView) bool {
	facts := state.Facts()
	if facts.RuleCoverageRequired ||
		facts.DecisionRecordsRequired ||
		facts.EntityResolutionRequired ||
		facts.ContributionLedgerRequired ||
		facts.ReconcileRequired {
		return true
	}
	if strings.TrimSpace(string(state.OutputContract.Format)) != "" && !facts.HasAnswer {
		return true
	}
	return len(state.FieldContractViolations) > 0 ||
		len(state.ZeroMatchFilterViolations) > 0 ||
		len(state.UnmatchedResolutionViolations) > 0 ||
		len(state.ZeroEligibleViolations) > 0
}

func RelationActionScaffolds(projections []ArtifactSchemaProjection, allowedActions []string, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	allowed := map[string]bool{}
	for _, action := range allowedActions {
		action = strings.TrimSpace(action)
		if action != "" {
			allowed[action] = true
		}
	}
	records := recordActionProjections(projections)
	if len(records) == 0 {
		return nil
	}
	var out []ActionScaffold
	appendAllowed := func(kind dataquery.DataActionKind, scaffolds []ActionScaffold) {
		if len(out) >= limit || !allowed[string(kind)] {
			return
		}
		for _, scaffold := range scaffolds {
			out = append(out, scaffold)
			if len(out) >= limit {
				return
			}
		}
	}
	appendAllowed(dataquery.DataActionNormalizeEntities, NormalizeEntityScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionMappingCandidate, MappingCandidateScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionApplyResolutions, ApplyResolutionScaffolds(projections, limit-len(out)))
	appendAllowed(dataquery.DataActionEnrichRecords, EnrichRecordScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionJoinRecords, JoinRecordScaffolds(records, limit-len(out)))
	return out
}

func PrioritizeConcreteScaffolds(scaffolds []ActionScaffold, facts StageFacts) []ActionScaffold {
	if len(scaffolds) == 0 {
		return nil
	}
	out := append([]ActionScaffold(nil), scaffolds...)
	stage := facts.NextStage()
	if !facts.EntityStageMaterialized || (stage != StagePrepareContributionInputs && stage != StageComputeContributions) {
		return out
	}
	priority := map[dataquery.DataActionKind]int{
		dataquery.DataActionApplyResolutions:  1,
		dataquery.DataActionMappingCandidate:  2,
		dataquery.DataActionEnrichRecords:     3,
		dataquery.DataActionJoinRecords:       4,
		dataquery.DataActionValueDistribution: 5,
		dataquery.DataActionFilterRecords:     6,
		dataquery.DataActionQualifyRecords:    7,
		dataquery.DataActionComputeContribs:   8,
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := priority[NormalizeActionKind(dataquery.DataActionKind(out[i].Kind))]
		right := priority[NormalizeActionKind(dataquery.DataActionKind(out[j].Kind))]
		if left == 0 {
			left = 100
		}
		if right == 0 {
			right = 100
		}
		return left < right
	})
	return out
}

func ConcreteFallbackScaffolds(scaffolds []ActionScaffold, facts StageFacts) []ActionScaffold {
	out := PrioritizeConcreteScaffolds(scaffolds, facts)
	if len(out) == 0 {
		return nil
	}
	stage := facts.NextStage()
	if !facts.EntityStageMaterialized || (stage != StagePrepareContributionInputs && stage != StageComputeContributions) {
		return out
	}
	filtered := out[:0]
	for _, scaffold := range out {
		if NormalizeActionKind(dataquery.DataActionKind(scaffold.Kind)) == dataquery.DataActionNormalizeEntities {
			continue
		}
		filtered = append(filtered, scaffold)
	}
	return filtered
}

func ConcreteActionFromScaffold(scaffold ActionScaffold) (dataquery.DataAction, bool) {
	if !scaffold.Executable {
		return dataquery.DataAction{}, false
	}
	kind := NormalizeActionKind(dataquery.DataActionKind(scaffold.Kind))
	params := concreteScaffoldParams(scaffold.ParamsTemplate)
	switch kind {
	case dataquery.DataActionMappingCandidate:
		hasSingleSource := scaffoldParamsConcrete(params, "source_field", "reference_name_fields", "canonical_id_field")
		hasSourceList := scaffoldParamsConcrete(params, "source_fields", "reference_name_fields", "canonical_id_field")
		if len(scaffold.InputPaths) < 2 || (!hasSingleSource && !hasSourceList) {
			return dataquery.DataAction{}, false
		}
		if strings.TrimSpace(params["match_mode"]) == "" || strings.Contains(params["match_mode"], "|") {
			params["match_mode"] = "exact"
		}
		return dataquery.DataAction{
			ID:             concreteScaffoldActionID("continue_mapping_candidate", scaffold.InputPaths),
			Kind:           dataquery.DataActionMappingCandidate,
			Purpose:        "inspect source-to-reference mapping candidates from concrete artifact fields",
			InputPaths:     append([]string(nil), scaffold.InputPaths[:2]...),
			OutputArtifact: concreteScaffoldArtifactID("mapping_candidates", scaffold.InputPaths),
			Params:         params,
		}, true
	case dataquery.DataActionNormalizeEntities:
		hasSingleSource := scaffoldParamsConcrete(params, "source_field", "reference_name_fields", "canonical_id_field")
		hasSourceList := scaffoldParamsConcrete(params, "source_fields", "reference_name_fields", "canonical_id_field")
		if len(scaffold.InputPaths) < 2 || (!hasSingleSource && !hasSourceList) {
			return dataquery.DataAction{}, false
		}
		if strings.TrimSpace(params["match_mode"]) == "" || strings.Contains(params["match_mode"], "|") {
			params["match_mode"] = "exact"
		}
		return dataquery.DataAction{
			ID:             concreteScaffoldActionID("continue_normalize_entities", scaffold.InputPaths),
			Kind:           dataquery.DataActionNormalizeEntities,
			Purpose:        "materialize source-to-reference mappings from concrete artifact fields",
			InputPaths:     append([]string(nil), scaffold.InputPaths[:2]...),
			OutputArtifact: concreteScaffoldArtifactID("entity_mappings", scaffold.InputPaths),
			Params:         params,
		}, true
	case dataquery.DataActionJoinRecords:
		if len(scaffold.InputPaths) < 2 || len(scaffold.CommonFields) == 0 {
			return dataquery.DataAction{}, false
		}
		field := strings.TrimSpace(scaffold.CommonFields[0])
		if field == "" || strings.Contains(field, "<") {
			return dataquery.DataAction{}, false
		}
		if strings.TrimSpace(params["join_type"]) == "" || strings.Contains(params["join_type"], "|") {
			params["join_type"] = "inner"
		}
		params["left_fields"] = mustJSON([]string{field})
		params["right_fields"] = mustJSON([]string{field})
		return dataquery.DataAction{
			ID:             concreteScaffoldActionID("continue_join_records", scaffold.InputPaths),
			Kind:           dataquery.DataActionJoinRecords,
			Purpose:        "join two concrete record artifacts on an existing common field",
			InputPaths:     append([]string(nil), scaffold.InputPaths[:2]...),
			OutputArtifact: concreteScaffoldArtifactID("joined_records", scaffold.InputPaths),
			Params:         params,
		}, true
	case dataquery.DataActionApplyResolutions:
		if len(scaffold.InputPaths) < 2 || !scaffoldParamsConcrete(params, "base_path", "resolution_specs") {
			return dataquery.DataAction{}, false
		}
		return dataquery.DataAction{
			ID:             concreteScaffoldActionID("continue_apply_resolutions", scaffold.InputPaths),
			Kind:           dataquery.DataActionApplyResolutions,
			Purpose:        "apply concrete entity-resolution ledger fields onto base records",
			InputPaths:     append([]string(nil), scaffold.InputPaths[:2]...),
			OutputArtifact: concreteScaffoldArtifactID("resolved_records", scaffold.InputPaths),
			Params:         params,
		}, true
	case dataquery.DataActionValueDistribution:
		inputPath := strings.TrimSpace(scaffold.InputPath)
		if inputPath == "" && len(scaffold.InputPaths) > 0 {
			inputPath = strings.TrimSpace(scaffold.InputPaths[0])
		}
		fields := nonInternalScaffoldFields(scaffold.Fields)
		if inputPath == "" || len(fields) == 0 {
			return dataquery.DataAction{}, false
		}
		fieldsParam := strings.TrimSpace(params["fields"])
		if fieldsParam == "" {
			fieldsParam = strings.TrimSpace(params["fields_json"])
		}
		if fieldsParam == "" {
			fieldsParam = strings.TrimSpace(params["field"])
		}
		if fieldsParam == "" || strings.Contains(fieldsParam, "<") || scaffoldParamLooksTemplate(params["fields"]) || scaffoldParamLooksTemplate(params["fields_json"]) || scaffoldParamLooksTemplate(params["field"]) {
			params["fields"] = mustJSON(clampStrings(fields, 8))
			delete(params, "fields_json")
			delete(params, "field")
		}
		params["input_path"] = inputPath
		return dataquery.DataAction{
			ID:             concreteScaffoldActionID("continue_value_distribution", []string{inputPath}),
			Kind:           dataquery.DataActionValueDistribution,
			Purpose:        "inspect field value distribution before selecting filters or grouping parameters",
			InputPaths:     []string{inputPath},
			OutputArtifact: concreteScaffoldArtifactID("value_distribution", []string{inputPath}),
			Params:         params,
		}, true
	default:
		return dataquery.DataAction{}, false
	}
}

func nonInternalScaffoldFields(fields []string) []string {
	cleaned := cleanStrings(fields)
	out := make([]string, 0, len(cleaned))
	for _, field := range cleaned {
		if strings.HasPrefix(strings.TrimSpace(field), "_") {
			continue
		}
		out = append(out, field)
	}
	return out
}

func concreteScaffoldParams(template map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range template {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func scaffoldParamsConcrete(params map[string]string, required ...string) bool {
	for _, key := range required {
		value := strings.TrimSpace(params[key])
		if value == "" || strings.Contains(value, "<") || strings.Contains(value, "|") {
			return false
		}
	}
	for _, value := range params {
		if strings.Contains(value, "<") {
			return false
		}
	}
	return true
}

func scaffoldParamLooksTemplate(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || strings.Contains(value, "<") || strings.Contains(value, "|")
}

func concreteScaffoldActionID(prefix string, inputs []string) string {
	return cleanIdentifier(prefix + "_" + strings.Join(clampStrings(inputs, 2), "_"))
}

func concreteScaffoldArtifactID(prefix string, inputs []string) string {
	id := cleanIdentifier(prefix + "_" + strings.Join(clampStrings(inputs, 2), "_"))
	if id == "" {
		return prefix + ".json"
	}
	return id + ".json"
}

func cleanIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
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
	if len(out) > 96 {
		out = strings.TrimRight(out[:96], "_")
	}
	return out
}

func DeriveFieldScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []ActionScaffold
	for _, artifact := range records {
		alias := firstProjectionAlias(artifact)
		if alias == "" || len(artifact.Fields) == 0 || !artifactHasTextExtractionField(artifact.Fields) {
			continue
		}
		out = append(out, ActionScaffold{
			Kind:      string(dataquery.DataActionDeriveFields),
			UseWhen:   "materialize a derived field on one existing record artifact before filtering, joining, or aggregating",
			InputPath: alias,
			Fields:    clampStrings(artifact.Fields, 16),
			ParamsTemplate: map[string]string{
				"field_specs_json": `[{"source_field":"<existing field from fields>","source_fields":["<existing field A>","<existing field B>"],"target_field":"<new field>","operation":"parse_number|extract_year|lower|upper|trim|map|regex_extract|concat|coalesce|constant|case_when","separator":" ","cases":[{"filters":[{"field":"<existing field>","op":"eq|in|gt|gte|lt|lte|not_empty","value":"<expected value>"}],"value_field":"<existing field to copy>"}],"default_field":"<optional existing fallback field>","default":"<optional fallback value>"}]`,
			},
			Note: "Use only field names present in fields. concat/coalesce use source_fields; case_when uses existing-field filters plus value/value_field/default. The system executes conditions but does not choose business semantics.",
		})
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func GroupRecordScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []ActionScaffold
	for _, artifact := range records {
		alias := firstProjectionAlias(artifact)
		if alias == "" || len(artifact.Fields) == 0 || !artifactHasTextExtractionField(artifact.Fields) {
			continue
		}
		out = append(out, ActionScaffold{
			Kind:      string(dataquery.DataActionGroupRecords),
			UseWhen:   "one logical record is split across multiple rows/spans and later extraction needs neighboring text from the same group",
			InputPath: alias,
			Fields:    clampStrings(artifact.Fields, 16),
			ParamsTemplate: map[string]string{
				"group_field":  "<existing key field from fields, such as a document/page/message/block id>",
				"text_fields":  `["<existing text-like field from fields>"]`,
				"target_field": "<new grouped text field>",
				"separator":    `\n`,
			},
			Note: "Use only existing group/text fields. The system groups rows and concatenates text; it does not decide business semantics.",
		})
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func ExtractFieldScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []ActionScaffold
	for _, artifact := range records {
		alias := firstProjectionAlias(artifact)
		if alias == "" || len(artifact.Fields) == 0 {
			continue
		}
		out = append(out, ActionScaffold{
			Kind:      string(dataquery.DataActionExtractFields),
			UseWhen:   "materialize structured fields from text or mixed record fields before filtering, joining, or aggregating",
			InputPath: alias,
			Fields:    clampStrings(artifact.Fields, 16),
			ParamsTemplate: map[string]string{
				"field_specs":     `[{"source_field":"<existing text/source field from fields>","target_field":"<new field>","operation":"regex_extract|parse_number|copy|trim","pattern":"<regex when operation is regex_extract>","group":1}]`,
				"required_fields": `["<new field that must be non-empty for output rows>"]`,
			},
			Note: "Use only existing source_field names from fields. If the source text has multiple numeric tokens, use regex_extract with a context pattern instead of unanchored parse_number. The model supplies the regex or parse specs from observed material shape; the system only executes extraction and preserves source locators.",
		})
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func ExpandRecordScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []ActionScaffold
	for _, artifact := range records {
		alias := firstProjectionAlias(artifact)
		if alias == "" || len(artifact.Fields) == 0 {
			continue
		}
		out = append(out, ActionScaffold{
			Kind:      string(dataquery.DataActionExpandRecords),
			UseWhen:   "turn one multi-value field on one record artifact into multiple records before enrichment, join, or contribution calculation",
			InputPath: alias,
			Fields:    clampStrings(artifact.Fields, 16),
			ParamsTemplate: map[string]string{
				"source_field":   "<existing multi-value field from fields>",
				"target_field":   "<expanded value field>",
				"delimiter":      "auto",
				"original_field": "<optional field to preserve the unsplit source value>",
			},
			Note: "Use this for delimited values such as aliases, tags, labels, roles, categories, ids, terms, or other lists. It changes row cardinality; derive_fields does not.",
		})
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func JoinRecordScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []ActionScaffold
	for i := 0; i < len(records); i++ {
		left := records[i]
		leftAlias := firstProjectionAlias(left)
		if leftAlias == "" || len(left.Fields) == 0 {
			continue
		}
		for j := i + 1; j < len(records); j++ {
			right := records[j]
			rightAlias := firstProjectionAlias(right)
			if rightAlias == "" || len(right.Fields) == 0 {
				continue
			}
			common := commonProjectionFields(left.Fields, right.Fields, 8)
			if len(common) == 0 {
				continue
			}
			out = append(out, ActionScaffold{
				Kind:         string(dataquery.DataActionJoinRecords),
				Executable:   true,
				UseWhen:      "join two record artifacts when both sides already contain compatible key fields",
				InputPaths:   []string{leftAlias, rightAlias},
				CommonFields: common,
				ParamsTemplate: map[string]string{
					"left_fields":  `["<field from common_fields or another field present on the left artifact>"]`,
					"right_fields": `["<matching field present on the right artifact>"]`,
					"join_type":    "inner|left",
					"collision":    "prefix",
				},
				Note: "Join fields are side-specific. inner keeps only matched left/right rows; left preserves all left rows for diagnostics or later filtering.",
			})
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func EnrichRecordScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []ActionScaffold
	for _, relation := range ArtifactRelationsFromProjections(records, -1) {
		if len(relation.BaseFields) == 0 || len(relation.LookupFields) == 0 || len(relation.LookupValueFields) == 0 {
			continue
		}
		base, ok := ArtifactSchemaByAlias(records, relation.BaseAlias)
		if !ok {
			continue
		}
		valueField := relation.LookupValueFields[0]
		spec := []map[string]any{{
			"lookup_path":        relation.LookupAlias,
			"base_fields":        relation.BaseFields,
			"lookup_fields":      relation.LookupFields,
			"lookup_value_field": valueField,
			"target_field":       "<new field to materialize on base records>",
			"match_mode":         firstNonEmpty(relation.MatchMode, "exact"),
		}}
		out = append(out, ActionScaffold{
			Kind:       string(dataquery.DataActionEnrichRecords),
			Executable: false,
			UseWhen:    "apply lookup/reference values onto base records while preserving base row cardinality",
			InputPaths: []string{relation.BaseAlias, relation.LookupAlias},
			Fields:     clampStrings(base.Fields, 20),
			ParamsTemplate: map[string]string{
				"base_path":    relation.BaseAlias,
				"lookup_specs": mustJSON(spec),
			},
			Note: "Use enrich_records when a structural artifact_graph relation already pairs base and lookup key fields. Choose target_field from lookup value fields; do not invent key fields.",
		})
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func NormalizeEntityScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	type candidate struct {
		scaffold ActionScaffold
		score    int
		order    int
	}
	var candidates []candidate
	order := 0
	for _, source := range records {
		sourceAlias := firstProjectionAlias(source)
		if sourceAlias == "" || len(source.Fields) == 0 {
			continue
		}
		for _, reference := range records {
			referenceAlias := firstProjectionAlias(reference)
			if referenceAlias == "" || referenceAlias == sourceAlias || len(reference.Fields) == 0 {
				continue
			}
			sourceFields := candidateMatchFields(source.Fields, 2)
			referenceFields := candidateMatchFields(reference.Fields, 4)
			if len(sourceFields) == 0 || len(referenceFields) == 0 {
				continue
			}
			canonicalID := firstProjectionField(reference.Fields)
			canonicalLabel := firstNonKeyField(reference.Fields, []string{canonicalID})
			if canonicalID == "" {
				continue
			}
			scaffold := ActionScaffold{
				Kind:       string(dataquery.DataActionNormalizeEntities),
				Executable: true,
				UseWhen:    "produce a reusable mapping ledger between source records and reference records before applying canonical fields",
				InputPaths: []string{sourceAlias, referenceAlias},
				Fields:     clampStrings(source.Fields, 20),
				ParamsTemplate: map[string]string{
					"source_path":           sourceAlias,
					"reference_path":        referenceAlias,
					"source_fields":         mustJSON(sourceFields),
					"reference_name_fields": mustJSON(referenceFields),
					"canonical_id_field":    canonicalID,
					"canonical_label_field": canonicalLabel,
					"match_mode":            "exact|contains|token_set",
				},
				Note: "Use this to create mapping evidence. It does not modify base records; use apply_entity_resolutions or enrich_records afterward when base rows need canonical fields.",
			}
			candidates = append(candidates, candidate{
				scaffold: scaffold,
				score:    normalizeScaffoldDirectionScore(source.Fields, reference.Fields, sourceFields, referenceFields, canonicalID, canonicalLabel),
				order:    order,
			})
			order++
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].order < candidates[j].order
	})
	out := make([]ActionScaffold, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.scaffold)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func MappingCandidateScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	base := NormalizeEntityScaffolds(records, limit)
	out := make([]ActionScaffold, 0, len(base))
	for _, scaffold := range base {
		scaffold.Kind = string(dataquery.DataActionMappingCandidate)
		scaffold.UseWhen = "inspect candidate source/reference matches, ambiguity, and evidence before deciding or applying canonical mappings"
		scaffold.Note = "Use this when mapping direction or match coverage is uncertain. It produces candidate rows only; normalize_entities or apply_entity_resolutions performs the later typed decision/application step."
		out = append(out, scaffold)
	}
	return out
}

func ApplyResolutionScaffolds(projections []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var ledgers []ArtifactSchemaProjection
	for _, projection := range projections {
		if projection.NodeClass == ArtifactNodeClassDiagnosticChild || projection.NodeClass == ArtifactNodeClassWorkflowLedger {
			continue
		}
		if artifactKindHasPrefix(projection.Kind, dataquery.DataActionNormalizeEntities) || looksLikeResolutionProjection(projection) {
			ledgers = append(ledgers, projection)
		}
	}
	if len(ledgers) == 0 {
		return nil
	}
	records := recordActionProjections(projections)
	var out []ActionScaffold
	for _, base := range records {
		baseAlias := firstProjectionAlias(base)
		if baseAlias == "" {
			continue
		}
		for _, ledger := range ledgers {
			ledgerAlias := firstProjectionAlias(ledger)
			if ledgerAlias == "" || ledgerAlias == baseAlias {
				continue
			}
			if !applyResolutionBaseCompatibleWithLedger(base, ledger) {
				continue
			}
			targetPrefix := resolutionTargetFieldPrefix(ledgerAlias)
			targetFields := []string{
				targetPrefix + "_canonical_id",
				targetPrefix + "_canonical_label",
				targetPrefix + "_resolution_status",
			}
			if projectionHasAnyNamedField(base, targetFields...) && projectionLineageContains(base, ledgerAlias) {
				continue
			}
			spec := []map[string]any{{
				"resolution_path":       ledgerAlias,
				"resolution_key_fields": []string{"item_id"},
				"target_id_field":       targetPrefix + "_canonical_id",
				"target_label_field":    targetPrefix + "_canonical_label",
				"target_status_field":   targetPrefix + "_resolution_status",
				"unmatched_status":      "unmatched",
			}}
			if existingIDField := applyResolutionExistingIDField(base.Fields, targetPrefix); existingIDField != "" {
				if reference, ok := applyResolutionReferenceProjection(projections, baseAlias, ledgerAlias, existingIDField, ledger); ok {
					spec[0]["existing_id_field"] = existingIDField
					spec[0]["reference_path"] = firstProjectionAlias(reference)
					spec[0]["reference_id_field"] = existingIDField
					if labelField := applyResolutionReferenceLabelField(reference.Fields, existingIDField); labelField != "" {
						spec[0]["reference_label_field"] = labelField
					}
				}
			}
			out = append(out, ActionScaffold{
				Kind:       string(dataquery.DataActionApplyResolutions),
				Executable: true,
				UseWhen:    "apply an existing mapping/resolution ledger back onto base records before filtering, joining, or contribution calculation",
				InputPaths: []string{baseAlias, ledgerAlias},
				Fields:     clampStrings(base.Fields, 20),
				ParamsTemplate: map[string]string{
					"base_path":          baseAlias,
					"base_filter_mode":   "preserve",
					"preserve_base_rows": "true",
					"resolution_specs":   mustJSON(spec),
				},
				Note: "This applies existing mapping evidence to base rows and preserves base rows by default; it does not decide new mappings.",
			})
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func FilterRecordScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []ActionScaffold
	for _, artifact := range records {
		alias := firstProjectionAlias(artifact)
		if alias == "" || len(artifact.Fields) == 0 {
			continue
		}
		out = append(out, ActionScaffold{
			Kind:      string(dataquery.DataActionFilterRecords),
			UseWhen:   "select a smaller reusable record set using fields that already exist on one artifact",
			InputPath: alias,
			Fields:    clampStrings(artifact.Fields, 18),
			ParamsTemplate: map[string]string{
				"filters_json": `[{"field":"<existing field from fields>","op":"eq|ne|in|not_in|contains|not_contains|empty|not_empty|gt|gte|lt|lte","value":"<value>"}]`,
			},
			Note: "Use only filter field names present in fields. If the desired filter field is missing, first use derive_fields, enrich_records, or join_records to materialize it.",
		})
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func ValueDistributionScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []ActionScaffold
	for _, artifact := range records {
		alias := firstProjectionAlias(artifact)
		if alias == "" || len(artifact.Fields) == 0 {
			continue
		}
		out = append(out, ActionScaffold{
			Kind:       string(dataquery.DataActionValueDistribution),
			Executable: true,
			UseWhen:    "inspect objective field values before choosing filters, join keys, mapping params, grouping, or contribution fields",
			InputPath:  alias,
			Fields:     clampStrings(artifact.Fields, 20),
			ParamsTemplate: map[string]string{
				"fields":      `["<existing field from fields>"]`,
				"top_n":       "8",
				"max_records": "100000",
			},
			Note: "Use this when field names exist but compact samples are not enough to choose typed params. It returns top/distinct/empty counts and does not change rows.",
		})
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func QualifyRecordScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []ActionScaffold
	for _, artifact := range records {
		alias := firstProjectionAlias(artifact)
		if alias == "" || len(artifact.Fields) == 0 {
			continue
		}
		out = append(out, ActionScaffold{
			Kind:      string(dataquery.DataActionQualifyRecords),
			UseWhen:   "turn rule/evidence eligibility into auditable include/exclude rows before contribution calculation",
			InputPath: alias,
			Fields:    clampStrings(artifact.Fields, 20),
			ParamsTemplate: map[string]string{
				"filters":         `[{"field":"<existing field that must pass>","op":"eq|in|not_empty|exists","value":"<value>"}]`,
				"reject_filters":  `[{"field":"<existing field that excludes a row>","op":"eq|in|contains","value":"<value>"}]`,
				"required_fields": `["<existing field that must be non-empty>"]`,
				"evidence_fields": `["<existing evidence/provenance field if the current rules require it>"]`,
				"status_fields":   `["<generated *_status field if relevant>"]`,
				"output_mode":     "filter",
				"item_id_field":   "<existing stable row id field if available>",
			},
			Note: "Use this when records need explicit eligibility decisions before compute_contributions. The model chooses conditions from the current rules; the system only executes typed field/filter/status/evidence checks and emits decision rows.",
		})
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func ComputeContributionScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []ActionScaffold
	for _, artifact := range records {
		alias := firstProjectionAlias(artifact)
		if alias == "" || len(artifact.Fields) == 0 {
			continue
		}
		out = append(out, ActionScaffold{
			Kind:      string(dataquery.DataActionComputeContribs),
			UseWhen:   "compute a generic contribution ledger from one eligible artifact that already contains the value/group/filter fields",
			InputPath: alias,
			Fields:    clampStrings(artifact.Fields, 20),
			ParamsTemplate: map[string]string{
				"value_field":     "<existing numeric value field, or omit for count>",
				"group_key_field": "<existing grouping field, or use group_key for a constant group>",
				"filters_json":    `[{"field":"<existing field from fields>","op":"eq|in|not_in|contains|exists","value":"<value>"}]`,
				"operation":       "add|count",
				"metric":          "<metric name>",
				"item_id_field":   "<existing stable row id field if available>",
			},
			Note: "Do not invent value/group/filter fields; materialize missing fields with derive_fields, enrich_records, or join_records first. If rule/evidence eligibility is not already decided, run qualify_records before this action.",
		})
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func applyResolutionBaseCompatibleWithLedger(base, ledger ArtifactSchemaProjection) bool {
	return ArtifactResolutionLineageCompatible(base, ledger)
}

func applyResolutionSourceCandidates(ledger ArtifactSchemaProjection) ([]string, bool) {
	return ResolutionSourceCandidates(ledger)
}

func projectionLineageContains(projection ArtifactSchemaProjection, alias string) bool {
	return ArtifactLineageContains(projection, alias)
}

func projectionHasAnyNamedField(projection ArtifactSchemaProjection, fields ...string) bool {
	set := artifactFieldSet(projection.Fields)
	for _, field := range fields {
		if set[strings.ToLower(strings.TrimSpace(field))] != "" {
			return true
		}
	}
	return false
}

func applyResolutionExistingIDField(fields []string, targetPrefix string) string {
	prefix := strings.Trim(cleanIdentifier(targetPrefix), "_")
	if prefix == "" {
		return ""
	}
	var candidates []string
	add := func(value string) {
		value = strings.Trim(value, "_")
		if value != "" {
			candidates = append(candidates, value)
		}
	}
	add(prefix + "_id")
	add(prefix + "_code")
	for _, suffix := range []string{"_canonical", "_entity", "_dimension"} {
		if strings.HasSuffix(prefix, suffix) && len(prefix) > len(suffix) {
			base := strings.TrimSuffix(prefix, suffix)
			add(base + "_id")
			add(base + "_code")
		}
	}
	return firstExistingProjectionField(fields, candidates...)
}

func applyResolutionReferenceProjection(projections []ArtifactSchemaProjection, baseAlias, ledgerAlias, existingIDField string, ledger ArtifactSchemaProjection) (ArtifactSchemaProjection, bool) {
	sourceHints := map[string]int{}
	hints := cleanStrings(ledger.ReferencePaths)
	if len(hints) == 0 {
		sourceSet := map[string]bool{}
		for _, source := range cleanStrings(ledger.SourceRecordPaths) {
			if key := normalizeAccessPath(source); key != "" {
				sourceSet[key] = true
			}
		}
		for _, source := range cleanStrings(ledger.SourcePaths) {
			if key := normalizeAccessPath(source); key != "" && !sourceSet[key] {
				hints = append(hints, source)
			}
		}
	}
	for i, source := range hints {
		if key := normalizeAccessPath(source); key != "" {
			sourceHints[key] = 100 - i
		}
	}
	bestScore := -1
	var best ArtifactSchemaProjection
	for _, projection := range projections {
		alias := firstProjectionAlias(projection)
		if alias == "" || normalizeAccessPath(alias) == normalizeAccessPath(baseAlias) || normalizeAccessPath(alias) == normalizeAccessPath(ledgerAlias) {
			continue
		}
		if len(projection.Fields) == 0 || projection.NodeClass == ArtifactNodeClassDiagnosticChild || projection.NodeClass == ArtifactNodeClassWorkflowLedger || projectionLooksLikeResolutionLedger(projection) {
			continue
		}
		if firstExistingProjectionField(projection.Fields, existingIDField) == "" {
			continue
		}
		score := 1
		for _, candidate := range append([]string{projection.ID}, projection.Aliases...) {
			if sourceHints[normalizeAccessPath(candidate)] > score {
				score = sourceHints[normalizeAccessPath(candidate)]
			}
		}
		if score > bestScore {
			bestScore = score
			best = projection
		}
	}
	return best, bestScore >= 0
}

func applyResolutionReferenceLabelField(fields []string, existingIDField string) string {
	base := strings.TrimSpace(existingIDField)
	lower := strings.ToLower(base)
	var candidates []string
	if strings.HasSuffix(lower, "_id") && len(base) > 3 {
		candidates = append(candidates, base[:len(base)-3]+"_label", base[:len(base)-3]+"_name")
	}
	if strings.HasSuffix(lower, "_code") && len(base) > 5 {
		candidates = append(candidates, base[:len(base)-5]+"_label", base[:len(base)-5]+"_name")
	}
	candidates = append(candidates, "canonical_label", "label", "name", "display_name", "title")
	return firstExistingProjectionField(fields, candidates...)
}

func firstExistingProjectionField(fields []string, candidates ...string) string {
	set := artifactFieldSet(fields)
	for _, candidate := range candidates {
		if got := set[strings.ToLower(strings.TrimSpace(candidate))]; got != "" {
			return got
		}
	}
	return ""
}

func normalizeScaffoldDirectionScore(sourceFields, referenceFields, chosenSourceFields, chosenReferenceFields []string, canonicalID, canonicalLabel string) int {
	score := 0
	if hasAnyNamedField(sourceFields, canonicalID) {
		score += 40
	}
	if canonicalLabel != "" && hasAnyNamedField(referenceFields, canonicalLabel) {
		score += 20
	}
	for _, field := range chosenSourceFields {
		if fieldHasTokenCue(field, "raw", "source", "input", "original") {
			score += 20
		}
	}
	for _, field := range chosenReferenceFields {
		if fieldHasTokenCue(field, "name", "label", "title", "display", "canonical") {
			score += 10
		}
	}
	if fieldHasTokenCue(canonicalID, "id", "code", "key") {
		score += 10
	}
	return score
}

func hasAnyNamedField(fields []string, wants ...string) bool {
	set := map[string]bool{}
	for _, field := range cleanStrings(fields) {
		set[strings.ToLower(strings.TrimSpace(field))] = true
	}
	for _, want := range wants {
		if set[strings.ToLower(strings.TrimSpace(want))] {
			return true
		}
	}
	return false
}

func fieldHasTokenCue(field string, cues ...string) bool {
	field = strings.ToLower(strings.TrimSpace(field))
	if field == "" {
		return false
	}
	parts := strings.FieldsFunc(field, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/' || r == ' '
	})
	for _, part := range parts {
		for _, cue := range cues {
			if part == cue {
				return true
			}
		}
	}
	return false
}

func resolutionTargetFieldPrefix(alias string) string {
	value := cleanIdentifier(alias)
	for {
		next := value
		for _, suffix := range []string{
			"_entity_resolutions",
			"_entity_resolution",
			"_resolutions",
			"_resolution",
			"_entity_mappings",
			"_entity_mapping",
			"_normalizations",
			"_normalization",
			"_normalized",
			"_mappings",
			"_mapping",
			"_records",
			"_record",
		} {
			if strings.HasSuffix(next, suffix) && len(next) > len(suffix) {
				next = strings.TrimSuffix(next, suffix)
				break
			}
		}
		if next == value {
			break
		}
		value = next
	}
	value = strings.Trim(value, "_")
	if value == "" {
		return "resolved_entity"
	}
	return value
}

func artifactHasTextExtractionField(fields []string) bool {
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || strings.HasPrefix(lower, "_") {
			continue
		}
		switch {
		case lower == "text" || lower == "content" || lower == "body" || lower == "message" || lower == "line":
			return true
		case strings.Contains(lower, "text") || strings.Contains(lower, "content") || strings.Contains(lower, "message") || strings.Contains(lower, "description"):
			return true
		case strings.HasSuffix(lower, "_raw") || lower == "raw":
			return true
		}
	}
	return false
}

func recordActionProjections(projections []ArtifactSchemaProjection) []ArtifactSchemaProjection {
	var out []ArtifactSchemaProjection
	for _, projection := range projections {
		if ArtifactUsableForRecordAction(projection) {
			out = append(out, projection)
		}
	}
	return out
}

func firstProjectionAlias(projection ArtifactSchemaProjection) string {
	for _, alias := range projection.Aliases {
		if alias = strings.TrimSpace(alias); alias != "" {
			return alias
		}
	}
	return strings.TrimSpace(projection.ID)
}

func firstProjectionField(fields []string) string {
	for _, field := range cleanStrings(fields) {
		if !strings.HasPrefix(strings.TrimSpace(field), "_") {
			return field
		}
	}
	return ""
}

func firstNonKeyField(fields []string, keys []string) string {
	keySet := map[string]bool{}
	for _, key := range keys {
		keySet[strings.ToLower(strings.TrimSpace(key))] = true
	}
	for _, field := range cleanStrings(fields) {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || strings.HasPrefix(lower, "_") || keySet[lower] {
			continue
		}
		return field
	}
	return ""
}

func candidateMatchFields(fields []string, limit int) []string {
	fields = cleanStrings(fields)
	if limit <= 0 || len(fields) == 0 {
		return nil
	}
	var out []string
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || strings.HasPrefix(lower, "_") {
			continue
		}
		out = append(out, field)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func commonProjectionFields(left, right []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	rightSet := map[string]string{}
	for _, field := range cleanStrings(right) {
		if !FieldUsableForRecordJoin(field) {
			continue
		}
		rightSet[strings.ToLower(field)] = field
	}
	var out []string
	for _, field := range cleanStrings(left) {
		if !FieldUsableForRecordJoin(field) {
			continue
		}
		if _, ok := rightSet[strings.ToLower(field)]; ok {
			out = append(out, field)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func FieldUsableForRecordJoin(field string) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return false
	}
	// Fields beginning with "_" are system lineage/locator columns produced by
	// the data runtime. They are useful for diagnostics and resolution replay,
	// but relation joins should use explicit materialized record fields.
	return !strings.HasPrefix(field, "_")
}

func hasAnyField(fields []string, wants ...string) bool {
	set := map[string]bool{}
	for _, field := range cleanStrings(fields) {
		set[strings.ToLower(field)] = true
	}
	for _, want := range wants {
		if set[strings.ToLower(strings.TrimSpace(want))] {
			return true
		}
	}
	return false
}

func looksLikeResolutionProjection(projection ArtifactSchemaProjection) bool {
	return projectionLooksLikeResolutionLedger(projection)
}

func projectionLooksLikeResolutionLedger(projection ArtifactSchemaProjection) bool {
	fields := projection.Fields
	hasLocator := hasAnyField(fields, "item_id", "source_locator", "_source_locator", "source_index", "_source_index", "row_index", "record_id", "row_id")
	hasSource := hasAnyField(fields, "source_value", "source_field", "evidence_refs")
	hasCanonical := hasAnyField(fields, "canonical_id", "canonical_label", "canonical_value")
	if hasLocator && hasCanonical && (hasSource || artifactKindHasPrefix(projection.Kind, dataquery.DataActionNormalizeEntities)) {
		return true
	}
	return artifactKindHasPrefix(projection.Kind, dataquery.DataActionNormalizeEntities) && hasCanonical
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(raw)
}
