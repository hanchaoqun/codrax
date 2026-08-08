package dataquery

import (
	"slices"
	"testing"
)

func TestDataActionAcceptedParamKeysComeFromRuntimeContract(t *testing.T) {
	join, ok := DataActionAcceptedParamKeys(DataActionJoinRecords)
	if !ok {
		t.Fatal("join_records missing runtime parameter contract")
	}
	for _, want := range []string{"left_fields", "left_fields_json", "right_fields", "join_type", "type"} {
		if !slices.Contains(join, want) {
			t.Fatalf("join_records accepted keys=%v, missing %q", join, want)
		}
	}
	for _, blocked := range []string{"lookup_specs", "lookup_specs_json", "source_filter_field"} {
		if slices.Contains(join, blocked) {
			t.Fatalf("join_records accepted keys=%v, contains foreign key %q", join, blocked)
		}
	}

	filter, ok := DataActionAcceptedParamKeys(DataActionFilterRecords)
	if !ok || !slices.Contains(filter, "filters") || !slices.Contains(filter, "filters_json") {
		t.Fatalf("filter_records accepted keys=%v ok=%v, want canonical and compatibility filter carriers", filter, ok)
	}
	if slices.Contains(filter, "source_filter_field") {
		t.Fatalf("filter_records accepted keys=%v contains invented key source_filter_field", filter)
	}

	compute, ok := DataActionAcceptedParamKeys(DataActionComputeContribs)
	if !ok || !slices.Contains(compute, "value_field") || !slices.Contains(compute, "filters") {
		t.Fatalf("compute_contributions accepted keys=%v ok=%v", compute, ok)
	}
	if slices.Contains(compute, "include") {
		t.Fatalf("compute_contributions accepted keys=%v contains phantom include key", compute)
	}

	if keys, ok := DataActionAcceptedParamKeys(DataActionDeriveFields); ok || keys != nil {
		t.Fatalf("uncontracted derive_fields keys=%v ok=%v, planner must not invent a strict allowlist", keys, ok)
	}
}

func TestDataActionAcceptedParamKeysReturnsCopy(t *testing.T) {
	first, ok := DataActionAcceptedParamKeys(DataActionJoinRecords)
	if !ok || len(first) == 0 {
		t.Fatalf("first keys=%v ok=%v", first, ok)
	}
	first[0] = "corrupt"
	second, _ := DataActionAcceptedParamKeys(DataActionJoinRecords)
	if slices.Contains(second, "corrupt") {
		t.Fatalf("runtime contract leaked mutable caller storage: %v", second)
	}
}
