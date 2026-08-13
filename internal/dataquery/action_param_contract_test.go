package dataquery

import (
	"context"
	"errors"
	"slices"
	"strings"
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

	assemble, ok := DataActionAcceptedParamKeys(DataActionAssembleAnswer)
	if !ok {
		t.Fatal("assemble_answer missing runtime parameter contract")
	}
	for _, want := range []string{"projection", "output_field", "reference_key_field"} {
		if !slices.Contains(assemble, want) {
			t.Fatalf("assemble_answer accepted keys=%v, missing %q", assemble, want)
		}
	}
	if slices.Contains(assemble, "group_key") {
		t.Fatalf("assemble_answer accepted keys=%v, overloaded group_key must be closed", assemble)
	}
	if got := DataActionParamDescription(DataActionAssembleAnswer, "output_field"); got == "" {
		t.Fatal("assemble_answer output_field needs executor-owned schema teaching")
	}

	if keys, ok := DataActionAcceptedParamKeys(DataActionDeriveFields); ok || keys != nil {
		t.Fatalf("uncontracted derive_fields keys=%v ok=%v, planner must not invent a strict allowlist", keys, ok)
	}
}

func TestAssembleAnswerRejectsOutputNameInInternalGroupCarrier(t *testing.T) {
	seed := Result{Reconcile: &ReconcileReport{
		Status: LooseText("pass"),
		Groups: []ReconcileGroup{{
			GroupKey: LooseText("active_user_ids"),
			Metric:   LooseText("id"),
			Values:   []string{"u1", "u3"},
		}},
	}}
	_, err := (ActionRunner{RepoRoot: t.TempDir(), Seed: seed}).Run(context.Background(), TaskPlan{
		OutputContract: OutputContract{Format: OutputJSONOnly, ExplanationAllowed: false},
		Actions: []DataAction{{
			Kind: DataActionAssembleAnswer,
			Params: map[string]string{
				"projection": "json_object",
				"group_key":  "ids",
			},
		}},
	})
	if err == nil {
		t.Fatal("assemble_answer accepted group_key as an output rename")
	}
	var paramErr DataActionParamError
	if !errors.As(err, &paramErr) || paramErr.Param != "group_key" || !strings.Contains(paramErr.Error(), "use output_field") {
		t.Fatalf("err=%T %v paramErr=%+v, want typed output_field repair", err, err, paramErr)
	}
}

func TestAssembleAnswerRejectsOutputFieldOnNonObjectProjection(t *testing.T) {
	seed := Result{Reconcile: &ReconcileReport{
		Status: LooseText("pass"),
		Groups: []ReconcileGroup{{
			GroupKey: LooseText("active_user_ids"), Metric: LooseText("id"), Values: []string{"u1", "u3"},
		}},
	}}
	_, err := (ActionRunner{RepoRoot: t.TempDir(), Seed: seed}).Run(context.Background(), TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{Kind: DataActionAssembleAnswer, Params: map[string]string{
			"projection": "values", "output_field": "ids",
		}}},
	})
	var paramErr DataActionParamError
	if !errors.As(err, &paramErr) || paramErr.Param != "output_field/projection" {
		t.Fatalf("err=%T %v paramErr=%+v, want fail-loud unused external field", err, err, paramErr)
	}
}

func TestAssembleAnswerRejectsProjectionEncodingThatContradictsOutputFormat(t *testing.T) {
	seed := Result{Reconcile: &ReconcileReport{
		Status: LooseText("pass"),
		Groups: []ReconcileGroup{{
			GroupKey: LooseText("all"), Metric: LooseText("count"), Actual: LooseText("2"), Expected: LooseText("2"),
		}},
	}}
	_, err := (ActionRunner{RepoRoot: t.TempDir(), Seed: seed}).Run(context.Background(), TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{Kind: DataActionAssembleAnswer, Params: map[string]string{
			"projection": "json_object", "output_field": "count",
		}}},
	})
	var paramErr DataActionParamError
	if !errors.As(err, &paramErr) || paramErr.Param != "projection/output_contract.format" ||
		!strings.Contains(paramErr.Error(), "format=plain_single_line") {
		t.Fatalf("err=%T %v paramErr=%+v, want typed output-format/projection conflict", err, err, paramErr)
	}
}

func TestAssembleAnswerProjectionCompatibilityRegistryAndAlias(t *testing.T) {
	contracts := DataActionOutputProjectionContracts()
	if len(contracts) == 0 || !slices.Contains(DataActionOutputProjectionsForFormat(OutputJSONOnly), "object") ||
		slices.Contains(DataActionOutputProjectionsForFormat(OutputPlainSingleLine), "json_object") {
		t.Fatalf("projection contracts=%+v", contracts)
	}
	contracts[0].Formats[0] = OutputJSONOnly
	if DataActionOutputProjectionContracts()[0].Formats[0] == OutputJSONOnly {
		t.Fatal("projection registry leaked caller mutation")
	}
	action, err := NormalizeDataActionForOutputContract(DataAction{
		Kind: DataActionAssembleAnswer, Params: map[string]string{"projection": "object", "output_field": "count"},
	}, OutputContract{Format: OutputJSONOnly})
	if err != nil || action.Params["projection"] != "json_object" {
		t.Fatalf("normalized=%+v err=%v, want object alias canonicalized", action, err)
	}
	jsonScalar, err := NormalizeDataActionForOutputContract(DataAction{
		Kind: DataActionAssembleAnswer, Params: map[string]string{"projection": "values"},
	}, OutputContract{Format: OutputJSONOnly})
	if err != nil || jsonScalar.Params["projection"] != "values" {
		t.Fatalf("normalized=%+v err=%v, JSON scalar-capable values projection must remain admitted", jsonScalar, err)
	}
	implicitObject, err := NormalizeDataActionForOutputContract(DataAction{
		Kind: DataActionAssembleAnswer, Params: map[string]string{"output_field": "count"},
	}, OutputContract{Format: OutputJSONOnly})
	if err != nil || implicitObject.Params["projection"] != "json_object" {
		t.Fatalf("normalized=%+v err=%v, JSON output_field must make object projection executable", implicitObject, err)
	}
	_, err = NormalizeDataActionForOutputContract(DataAction{
		Kind: DataActionAssembleAnswer, Params: map[string]string{"output_field": "count"},
	}, OutputContract{Format: OutputPlainSingleLine})
	var paramErr DataActionParamError
	if !errors.As(err, &paramErr) || paramErr.Param != "output_field/projection" {
		t.Fatalf("err=%T %v, plain output_field omission must not be silently ignored", err, err)
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
