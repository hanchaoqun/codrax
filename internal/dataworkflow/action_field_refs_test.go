package dataworkflow

import (
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
