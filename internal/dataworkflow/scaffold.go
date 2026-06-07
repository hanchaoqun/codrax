package dataworkflow

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ActionScaffold struct {
	Kind           string            `json:"kind"`
	UseWhen        string            `json:"use_when,omitempty"`
	InputPath      string            `json:"input_path,omitempty"`
	InputPaths     []string          `json:"input_paths,omitempty"`
	Fields         []string          `json:"fields,omitempty"`
	CommonFields   []string          `json:"common_fields,omitempty"`
	ParamsTemplate map[string]string `json:"params_template,omitempty"`
	Note           string            `json:"note,omitempty"`
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
	appendAllowed(dataquery.DataActionApplyResolutions, ApplyResolutionScaffolds(projections, limit-len(out)))
	appendAllowed(dataquery.DataActionEnrichRecords, EnrichRecordScaffolds(records, limit-len(out)))
	appendAllowed(dataquery.DataActionJoinRecords, JoinRecordScaffolds(records, limit-len(out)))
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
	for _, base := range records {
		if !ArtifactUsableForRecordAction(base) {
			continue
		}
		baseAlias := firstProjectionAlias(base)
		if baseAlias == "" || len(base.Fields) == 0 {
			continue
		}
		for _, lookup := range records {
			lookupAlias := firstProjectionAlias(lookup)
			if lookupAlias == "" || lookupAlias == baseAlias || len(lookup.Fields) == 0 || lookup.NodeClass == ArtifactNodeClassDiagnosticChild {
				continue
			}
			common := commonProjectionFields(base.Fields, lookup.Fields, 4)
			baseFields := common
			lookupFields := common
			if len(baseFields) == 0 {
				baseFields = candidateMatchFields(base.Fields, 4)
				lookupFields = candidateMatchFields(lookup.Fields, 4)
			}
			if len(baseFields) == 0 || len(lookupFields) == 0 {
				continue
			}
			valueField := firstNonKeyField(lookup.Fields, lookupFields[:1])
			if valueField == "" {
				continue
			}
			spec := []map[string]any{{
				"lookup_path":        lookupAlias,
				"base_fields":        baseFields,
				"lookup_fields":      lookupFields,
				"lookup_value_field": valueField,
				"target_field":       "<new field to materialize on base records>",
				"match_mode":         "exact|contains|mapping_contains_source|source_contains_mapping",
			}}
			out = append(out, ActionScaffold{
				Kind:       string(dataquery.DataActionEnrichRecords),
				UseWhen:    "apply lookup/reference values onto base records while preserving base row cardinality",
				InputPaths: []string{baseAlias, lookupAlias},
				Fields:     clampStrings(base.Fields, 20),
				ParamsTemplate: map[string]string{
					"base_path":    baseAlias,
					"lookup_specs": mustJSON(spec),
				},
				Note: "Use enrich_records when reference values should be added to base rows. Do not use it to decide business semantics; choose fields from observed artifacts.",
			})
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func NormalizeEntityScaffolds(records []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []ActionScaffold
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
			out = append(out, ActionScaffold{
				Kind:       string(dataquery.DataActionNormalizeEntities),
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
			})
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func ApplyResolutionScaffolds(projections []ArtifactSchemaProjection, limit int) []ActionScaffold {
	if limit <= 0 {
		return nil
	}
	var ledgers []ArtifactSchemaProjection
	for _, projection := range projections {
		if projection.NodeClass == ArtifactNodeClassDiagnosticChild {
			continue
		}
		if projection.NodeClass == ArtifactNodeClassWorkflowLedger ||
			artifactKindHasPrefix(projection.Kind, dataquery.DataActionNormalizeEntities, dataquery.DataActionApplyResolutions) ||
			looksLikeResolutionProjection(projection) {
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
			spec := []map[string]any{{
				"resolution_path":       ledgerAlias,
				"resolution_key_fields": []string{"item_id"},
				"target_id_field":       "<new canonical id field>",
				"target_label_field":    "<new canonical label field>",
				"target_status_field":   "<new resolution status field>",
				"unmatched_status":      "unmatched",
			}}
			out = append(out, ActionScaffold{
				Kind:       string(dataquery.DataActionApplyResolutions),
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
	fields := projection.Fields
	hasLocator := hasAnyField(fields, "item_id", "source_locator", "_source_locator", "source_index", "_source_index", "row_index")
	hasResolutionValue := hasAnyField(fields, "canonical_id", "canonical_label", "resolution_status", "status")
	return hasLocator && hasResolutionValue
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(raw)
}
