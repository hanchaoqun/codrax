package dataworkflow

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestActionFieldRefsExtractDomainNeutralParams(t *testing.T) {
	filter := dataquery.DataAction{
		Kind: dataquery.DataActionFilterRecords,
		Params: map[string]string{
			"filters":      `[{"field":"status"},{"source_field":"currency"}]`,
			"filter_field": "region|country",
		},
	}
	if got := strings.Join(FilterActionFieldRefs(filter), ","); got != "status,currency,region,country" {
		t.Fatalf("filter refs=%q", got)
	}

	qualify := dataquery.DataAction{
		Kind: dataquery.DataActionQualifyRecords,
		Params: map[string]string{
			"filters_json":            `[{"field":"state"}]`,
			"exclude_filters":         `[{"input_field":"blocked_reason"}]`,
			"required_fields":         `["id","name"]`,
			"evidence_fields":         "evidence_a，evidence_b",
			"generated_status_fields": "eligible;reviewed",
		},
	}
	got := strings.Join(QualifyActionFieldRefs(qualify), ",")
	for _, want := range []string{"state", "blocked_reason", "id", "name", "evidence_a", "evidence_b", "eligible", "reviewed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("qualify refs=%q, want %q", got, want)
		}
	}
}

func TestSingleRecordSetActionFieldRefsUsesTypedActionContract(t *testing.T) {
	derive := dataquery.DataAction{
		Kind: dataquery.DataActionDeriveFields,
		Params: map[string]string{
			"field_specs": `[
				{"operation":"constant","target_field":"kind"},
				{"operation":"coalesce","left_field":"primary","right_field":"fallback"},
				{"source_fields":["raw_a","raw_b"]}
			]`,
		},
	}
	if got := strings.Join(SingleRecordSetActionFieldRefs(dataquery.DataActionDeriveFields, derive), ","); got != "primary,fallback,raw_a,raw_b" {
		t.Fatalf("derive refs=%q", got)
	}

	group := dataquery.DataAction{
		Kind: dataquery.DataActionGroupRecords,
		Params: map[string]string{
			"group_fields": "account_id",
			"text_fields":  `["description","note"]`,
			"first_fields": "currency;unit",
		},
	}
	if got := strings.Join(SingleRecordSetActionFieldRefs(dataquery.DataActionGroupRecords, group), ","); got != "account_id,description,note,currency,unit" {
		t.Fatalf("group refs=%q", got)
	}

	compute := dataquery.DataAction{
		Kind: dataquery.DataActionComputeContribs,
		Params: map[string]string{
			"value_field":     "duration_ms",
			"group_key_field": "request_id",
			"filters":         `[{"field":"status"}]`,
		},
	}
	if got := strings.Join(ComputeContributionActionFieldRefs(compute), ","); got != "duration_ms,request_id,status" {
		t.Fatalf("compute refs=%q", got)
	}
}

func TestJoinActionFieldRequirementsUseRolePathsAndSharedKeys(t *testing.T) {
	action := dataquery.DataAction{
		Kind:       dataquery.DataActionJoinRecords,
		InputPaths: []string{"left_rows", "right_rows"},
		Params: map[string]string{
			"join_fields": `["account_id","period"]`,
		},
	}
	requirements := JoinActionFieldRequirements(action)
	if len(requirements) != 2 {
		t.Fatalf("requirements len=%d, want 2: %#v", len(requirements), requirements)
	}
	if requirements[0].Role != "left" || requirements[0].Path != "left_rows" || !reflect.DeepEqual(requirements[0].Fields, []string{"account_id", "period"}) {
		t.Fatalf("left requirement=%#v", requirements[0])
	}
	if requirements[1].Role != "right" || requirements[1].Path != "right_rows" || !reflect.DeepEqual(requirements[1].Fields, []string{"account_id", "period"}) {
		t.Fatalf("right requirement=%#v", requirements[1])
	}

	explicit := dataquery.DataAction{
		Kind:       dataquery.DataActionJoinRecords,
		InputPaths: []string{"ignored_left", "ignored_right"},
		Params: map[string]string{
			"left_path":    "records",
			"right_path":   "reference",
			"left_fields":  "record_key",
			"right_fields": "reference_key",
		},
	}
	requirements = JoinActionFieldRequirements(explicit)
	if !requirements[0].Explicit || !requirements[1].Explicit {
		t.Fatalf("explicit paths not marked: %#v", requirements)
	}
	if requirements[0].Path != "records" || strings.Join(requirements[0].Fields, ",") != "record_key" {
		t.Fatalf("explicit left=%#v", requirements[0])
	}
	if requirements[1].Path != "reference" || strings.Join(requirements[1].Fields, ",") != "reference_key" {
		t.Fatalf("explicit right=%#v", requirements[1])
	}
}

func TestNormalizeEntityActionFieldRequirementsUseSourceReferenceRoles(t *testing.T) {
	action := dataquery.DataAction{
		Kind:       dataquery.DataActionNormalizeEntities,
		InputPaths: []string{"records", "reference"},
		Params: map[string]string{
			"source_fields":         `["raw_name"]`,
			"filters":               `[{"field":"eligible"}]`,
			"reference_name_field":  "candidate_name",
			"canonical_id_field":    "canonical_id",
			"canonical_label_field": "canonical_label",
		},
	}
	requirements, skip := NormalizeEntityActionFieldRequirements(action)
	if skip {
		t.Fatal("unexpected skip")
	}
	if len(requirements) != 2 {
		t.Fatalf("requirements len=%d, want 2: %#v", len(requirements), requirements)
	}
	if requirements[0].Role != "source" || requirements[0].Path != "records" || strings.Join(requirements[0].Fields, ",") != "raw_name,eligible" {
		t.Fatalf("source requirement=%#v", requirements[0])
	}
	if requirements[1].Role != "reference" || requirements[1].Path != "reference" || strings.Join(requirements[1].Fields, ",") != "candidate_name,canonical_id,canonical_label" {
		t.Fatalf("reference requirement=%#v", requirements[1])
	}

	explicitMappings := dataquery.DataAction{
		Kind: dataquery.DataActionNormalizeEntities,
		Params: map[string]string{
			"mappings_json": `[{"source":"a","canonical":"b"}]`,
		},
	}
	if _, skip := NormalizeEntityActionFieldRequirements(explicitMappings); !skip {
		t.Fatal("expected explicit mappings to skip structured source/reference requirements")
	}
}

func TestEnrichActionFieldRequirementsExtractLookupSpecs(t *testing.T) {
	action := dataquery.DataAction{
		Kind:       dataquery.DataActionEnrichRecords,
		InputPaths: []string{"base_records", "lookup_records"},
		Params: map[string]string{
			"lookup_specs_json": `[
				{
					"base_fields":["source_key"],
					"lookup_fields":["lookup_key"],
					"lookup_value_field":"canonical_value"
				}
			]`,
		},
	}
	requirements := EnrichActionFieldRequirements(action)
	if len(requirements) != 2 {
		t.Fatalf("requirements len=%d, want 2: %#v", len(requirements), requirements)
	}
	if requirements[0].Role != "base" || requirements[0].Path != "base_records" || strings.Join(requirements[0].Fields, ",") != "source_key" {
		t.Fatalf("base requirement=%#v", requirements[0])
	}
	if requirements[1].Role != "lookup" || requirements[1].Path != "lookup_records" || strings.Join(requirements[1].Fields, ",") != "lookup_key,canonical_value" {
		t.Fatalf("lookup requirement=%#v", requirements[1])
	}
}
