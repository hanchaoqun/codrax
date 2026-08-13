package dataquery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assembleReferenceAuthoritySeed() Result {
	contributions := []ContributionRecord{
		{ItemID: LooseText("a"), Source: LooseText("rows.csv"), SourceLocator: LooseText("row 1"), GroupKey: LooseText("GroupA"), Metric: LooseText("total_value"), Value: LooseText("17"), Operation: LooseText("add"), Role: LooseText("target")},
		{ItemID: LooseText("b"), Source: LooseText("rows.csv"), SourceLocator: LooseText("row 2"), GroupKey: LooseText("GroupB"), Metric: LooseText("total_value"), Value: LooseText("4"), Operation: LooseText("add"), Role: LooseText("target")},
		{ItemID: LooseText("c"), Source: LooseText("rows.csv"), SourceLocator: LooseText("row 3"), GroupKey: LooseText("GroupC"), Metric: LooseText("total_value"), Value: LooseText("5"), Operation: LooseText("add"), Role: LooseText("target")},
	}
	return Result{
		Answer:        "GroupA/total_value=17; GroupB/total_value=4; GroupC/total_value=5; GroupX/total_value=0",
		Contributions: contributions,
		Reconcile: &ReconcileReport{
			Status:                 LooseText("pass"),
			ExpectedAnswer:         LooseText("GroupA/total_value=17; GroupB/total_value=4; GroupC/total_value=5; GroupX/total_value=0"),
			ActualAnswer:           LooseText("GroupA/total_value=17; GroupB/total_value=4; GroupC/total_value=5; GroupX/total_value=0"),
			AnswerComparisonStatus: LooseText("pass"),
			Groups: []ReconcileGroup{
				{GroupKey: LooseText("GroupA"), Metric: LooseText("total_value"), Expected: LooseText("17"), Actual: LooseText("17")},
				{GroupKey: LooseText("GroupB"), Metric: LooseText("total_value"), Expected: LooseText("4"), Actual: LooseText("4")},
				{GroupKey: LooseText("GroupC"), Metric: LooseText("total_value"), Expected: LooseText("5"), Actual: LooseText("5")},
				{GroupKey: LooseText("final_answer"), Metric: LooseText("projection"), Scope: LooseText("final_answer"), Role: LooseText("output"), Expected: LooseText("GroupA/total_value=17; GroupB/total_value=4; GroupC/total_value=5; GroupX/total_value=0"), Actual: LooseText("GroupA/total_value=17; GroupB/total_value=4; GroupC/total_value=5; GroupX/total_value=0")},
			},
		},
	}
}

func writeAssembleReferenceAuthorityFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range map[string]string{
		"targets.csv":     "target_id,canonical_label\nT1,GroupA\nT2,GroupX\nT3,GroupC\n",
		"all_records.csv": "record_id,canonical_label\nR1,GroupA\nR2,GroupB\nR3,GroupC\nR4,GroupX\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// A typed action-local reference_path + key field is already sufficient to
// establish hard grounding authority. The executor must consume the same
// declaration even when the planner omits the redundant complete_reference
// boolean, and must replace a stale answer-scope reconcile group.
func TestAssembleAnswerExplicitReferencePairActivatesProjection(t *testing.T) {
	root := writeAssembleReferenceAuthorityFixture(t)
	res, err := (ActionRunner{RepoRoot: root, Seed: assembleReferenceAuthoritySeed()}).Run(context.Background(), TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","},
		Actions: []DataAction{{
			ID:         "repair_answer",
			Kind:       DataActionAssembleAnswer,
			InputPaths: []string{"reconcile_result", "targets.csv"},
			Params: map[string]string{
				"projection":          "values",
				"order_by":            "input",
				"delimiter":           ",",
				"reference_path":      "targets.csv",
				"reference_key_field": "canonical_label",
				"metric":              "total_value",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "17,0,5" {
		t.Fatalf("Answer=%q, want explicit reference pair to activate complete projection", res.Answer)
	}
	artifact := res.Artifacts[len(res.Artifacts)-1]
	if artifact.Fields["reference_path"] != "targets.csv" || artifact.Fields["reference_key_count"] != "3" || artifact.Fields["zero_filled_count"] != "1" || artifact.Fields["dropped_extra_count"] != "1" {
		t.Fatalf("artifact fields=%+v, want targets receipt 3/zero-fill 1/drop-extra 1", artifact.Fields)
	}
	projectionGroups := 0
	for _, group := range res.Reconcile.Groups {
		if reconcileGroupIsFinalAnswerProjection(group) {
			projectionGroups++
			if group.Expected.String() != "17,0,5" || group.Actual.String() != "17,0,5" {
				t.Fatalf("final projection group=%+v, want fresh answer", group)
			}
		}
	}
	if projectionGroups != 1 {
		t.Fatalf("reconcile groups=%+v, want exactly one fresh final projection group", res.Reconcile.Groups)
	}
}

func TestAssembleAnswerReferenceOrderAliasPreservesResolvedReferenceOrder(t *testing.T) {
	root := writeAssembleReferenceAuthorityFixture(t)
	res, err := (ActionRunner{RepoRoot: root, Seed: assembleReferenceAuthoritySeed()}).Run(context.Background(), TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","},
		Actions: []DataAction{{
			ID:         "repair_answer",
			Kind:       DataActionAssembleAnswer,
			InputPaths: []string{"reconcile_result", "targets.csv"},
			Params: map[string]string{
				"projection":          "values",
				"order_by":            "reference",
				"delimiter":           ",",
				"reference_path":      "targets.csv",
				"reference_key_field": "canonical_label",
				"metric":              "total_value",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "17,0,5" {
		t.Fatalf("Answer=%q, want reference-order alias to preserve GroupA,GroupX,GroupC", res.Answer)
	}
	artifact := res.Artifacts[len(res.Artifacts)-1]
	if artifact.Fields["order_by"] != "input" || artifact.Fields["order_preserved"] != "true" {
		t.Fatalf("artifact fields=%+v, want normalized reference-order receipt", artifact.Fields)
	}
}

func TestAssembleAnswerReferenceOrderRejectsUnresolvedReference(t *testing.T) {
	seed := Result{Reconcile: &ReconcileReport{Status: LooseText("pass"), Groups: []ReconcileGroup{{
		GroupKey: LooseText("A"), Metric: LooseText("amount"), Expected: LooseText("1"), Actual: LooseText("1"), Difference: LooseText("0"),
	}}}}
	_, err := (ActionRunner{Seed: seed}).Run(context.Background(), TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{{Kind: DataActionAssembleAnswer, Params: map[string]string{
			"projection": "values",
			"order_by":   "reference",
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "order_by=reference requires") {
		t.Fatalf("err=%v, want unresolved reference order to fail loud", err)
	}
}

func TestAssembleReferenceOrderDependencyRegistryMatchesRuntimeAuthority(t *testing.T) {
	contracts := DataActionParamDependencyContracts()
	if len(contracts) != 1 {
		t.Fatalf("dependency contracts=%+v, want one executor-owned contract", contracts)
	}
	contract := contracts[0]
	if contract.Kind != DataActionAssembleAnswer || contract.TriggerParam != "order_by" || contract.TriggerValue != "reference" {
		t.Fatalf("dependency contract=%+v", contract)
	}
	if len(contract.RequiredActionParamGroups) != 2 || !contract.OutputCompleteReferenceAlternative || !contract.ActionCompleteReferenceAlternative {
		t.Fatalf("dependency alternatives=%+v", contract)
	}
	for _, tc := range []struct {
		name   string
		params map[string]string
		want   bool
	}{
		{name: "canonical pair", params: map[string]string{"reference_path": "targets.csv", "reference_key_field": "id"}, want: true},
		{name: "accepted aliases", params: map[string]string{"reference_paths": `["targets.csv"]`, "group_key_field": "id"}, want: true},
		{name: "overloaded group key is not a reference credential", params: map[string]string{"reference_paths": `["targets.csv"]`, "group_key": "id"}},
		{name: "path only", params: map[string]string{"reference_path": "targets.csv"}},
		{name: "key only", params: map[string]string{"reference_key_field": "id"}},
		{name: "empty values", params: map[string]string{"reference_path": " ", "reference_key_field": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			action := DataAction{Kind: DataActionAssembleAnswer, Params: tc.params}
			if got := assembleActionDeclaresReferencePair(action); got != tc.want {
				t.Fatalf("declares pair=%t, want %t for %+v", got, tc.want, tc.params)
			}
		})
	}
	contracts[0].RequiredActionParamGroups[0][0] = "mutated"
	if got := DataActionParamDependencyContracts()[0].RequiredActionParamGroups[0][0]; got != "reference_paths" {
		t.Fatalf("registry leaked mutable contract slice: %q", got)
	}
}

func TestActionParamExclusionRegistryReturnsDefensiveCopies(t *testing.T) {
	contracts := DataActionParamExclusionContracts()
	if len(contracts) != 1 || contracts[0].Kind != DataActionComputeContribs ||
		contracts[0].TriggerParam != "operation" || contracts[0].TriggerValue != "count" ||
		len(contracts[0].ForbiddenParams) != 1 || contracts[0].ForbiddenParams[0] != "value_field" {
		t.Fatalf("contracts=%+v, want count/value_field runtime exclusion", contracts)
	}
	contracts[0].ForbiddenParams[0] = "corrupt"
	if got := DataActionParamExclusionContracts()[0].ForbiddenParams[0]; got != "value_field" {
		t.Fatalf("registry leaked caller mutation: %q", got)
	}
}

// Typed action inputs are the first reference scope. An unrelated historical
// artifact with more keys must not win merely because the old fallback ranked
// larger key sets higher.
func TestAssembleAnswerInputReferenceScopePrecedesHistoricalArtifacts(t *testing.T) {
	root := writeAssembleReferenceAuthorityFixture(t)
	seed := assembleReferenceAuthoritySeed()
	seed.Artifacts = []DataArtifact{{ID: "all_records", Kind: string(DataActionExtractRecords), SourcePaths: []string{"all_records.csv"}}}
	res, err := (ActionRunner{RepoRoot: root, Seed: seed}).Run(context.Background(), TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","},
		Actions: []DataAction{{
			ID:         "answer",
			Kind:       DataActionAssembleAnswer,
			InputPaths: []string{"targets.csv"},
			Params: map[string]string{
				"complete_reference":  "true",
				"projection":          "values",
				"order_by":            "input",
				"reference_key_field": "canonical_label",
				"metric":              "total_value",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "17,0,5" {
		t.Fatalf("Answer=%q, want typed input reference scope ahead of wider historical artifact", res.Answer)
	}
	fields := res.Artifacts[len(res.Artifacts)-1].Fields
	if fields["reference_path"] != "targets.csv" || fields["reference_key_count"] != "3" {
		t.Fatalf("fields=%+v, want targets.csv input-scope receipt", fields)
	}
}

func TestAssembleAnswerHistoricalReferenceFallbackRemainsAvailable(t *testing.T) {
	root := writeAssembleReferenceAuthorityFixture(t)
	seed := assembleReferenceAuthoritySeed()
	seed.Artifacts = []DataArtifact{{ID: "all_records", Kind: string(DataActionExtractRecords), SourcePaths: []string{"all_records.csv"}}}
	res, err := (ActionRunner{RepoRoot: root, Seed: seed}).Run(context.Background(), TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","},
		Actions: []DataAction{{
			ID:         "answer",
			Kind:       DataActionAssembleAnswer,
			InputPaths: []string{"reconcile_result"},
			Params: map[string]string{
				"complete_reference":  "true",
				"projection":          "values",
				"order_by":            "input",
				"reference_key_field": "canonical_label",
				"metric":              "total_value",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "17,4,5,0" {
		t.Fatalf("Answer=%q, want historical artifact fallback when input scope has no key candidate", res.Answer)
	}
	if res.Artifacts[len(res.Artifacts)-1].Fields["reference_key_count"] != "4" {
		t.Fatalf("fields=%+v, want four-key historical fallback receipt", res.Artifacts[len(res.Artifacts)-1].Fields)
	}
}

func TestAssembleAnswerAmbiguousInputReferenceScopeDoesNotFallThrough(t *testing.T) {
	root := writeAssembleReferenceAuthorityFixture(t)
	if err := os.WriteFile(filepath.Join(root, "other_targets.csv"), []byte("target_id,canonical_label\nO1,GroupA\nO2,GroupZ\nO3,GroupC\n"), 0600); err != nil {
		t.Fatal(err)
	}
	seed := assembleReferenceAuthoritySeed()
	seed.Answer = ""
	seed.Reconcile.ExpectedAnswer = ""
	seed.Reconcile.ActualAnswer = ""
	seed.Reconcile.Groups = seed.Reconcile.Groups[:3]
	seed.Artifacts = []DataArtifact{{ID: "all_records", Kind: string(DataActionExtractRecords), SourcePaths: []string{"all_records.csv"}}}
	res, err := (ActionRunner{RepoRoot: root, Seed: seed}).Run(context.Background(), TaskPlan{
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","},
		Actions: []DataAction{{
			ID:         "answer",
			Kind:       DataActionAssembleAnswer,
			InputPaths: []string{"targets.csv", "other_targets.csv"},
			Params: map[string]string{
				"complete_reference":  "true",
				"projection":          "values",
				"order_by":            "input",
				"reference_key_field": "canonical_label",
				"metric":              "total_value",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "17,4,5" {
		t.Fatalf("Answer=%q, want ambiguous typed input scope to remain unprojected instead of falling through to all_records", res.Answer)
	}
	if fields := res.Artifacts[len(res.Artifacts)-1].Fields; fields["reference_projected"] != "" {
		t.Fatalf("fields=%+v, ambiguous input universes must not mint a reference projection receipt", fields)
	}
}
