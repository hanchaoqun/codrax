package dataquery

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// EVALFIX-2D 类4 (ledger lineage) pins. The class predicate: every key that
// sits in a VALUE position of a ledger entry must land on exactly one typed
// lineage lane (field values / declared literal / plain constant / synthetic
// "all"). A field name silently demoted to a constant group label is not a
// lane — it mints a phantom ledger group that self-consistently passes every
// shape check and reconcile (eval gap F11).

func lineageComputePlan(inputs []string, params map[string]string) TaskPlan {
	return TaskPlan{
		Status:         "ready",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		Actions: []DataAction{
			{
				ID:         "contribs",
				Kind:       DataActionComputeContribs,
				InputPaths: inputs,
				Params:     params,
			},
		},
	}
}

// Pin 1 (specimen, red-first): a group_key naming a ledger schema field
// ("canonical_label") that NO input carries must hard-reject with the typed
// lineage violation instead of silently minting the literal group. Before
// EVALFIX-2D the run passed with every row in group "canonical_label"
// (count=N) and reconcile agreed with its own phantom — zero discrimination.
func TestComputeContributionsGroupKeySchemaNameWithoutFieldHardRejects(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.csv"), []byte("id,amount\n1,10\n2,5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.csv"), []byte("id,amount\n3,7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), lineageComputePlan(
		[]string{"a.csv", "b.csv"},
		map[string]string{
			"group_key":     "canonical_label",
			"metric":        "record_count",
			"operation":     "count",
			"item_id_field": "id",
			"role":          "target",
		},
	))
	if err == nil {
		t.Fatalf("want typed lineage violation, got nil error (phantom literal group minted)")
	}
	violation := ClassifyExecutionFailure(err)
	if violation.Role != ContributionGroupKeyLineageRole {
		t.Fatalf("violation=%+v, want Role=%q", violation, ContributionGroupKeyLineageRole)
	}
	if violation.Code != "field_contract_violation" {
		t.Fatalf("violation code=%q, want field_contract_violation (existing typed classification chain)", violation.Code)
	}
	if got := strings.Join(violation.MissingFields, ","); got != "canonical_label" {
		t.Fatalf("MissingFields=%q, want canonical_label", got)
	}
	// Repair guidance must carry BOTH arms: materialize the field
	// (enrich/join lane) or declare the constant deliberately
	// (group_key_literal escape).
	text := err.Error() + " " + violation.RepairHint
	for _, arm := range []string{"enrich_records", "join_records", "group_key_literal"} {
		if !strings.Contains(text, arm) {
			t.Fatalf("error/repair hint %q missing repair arm %q", text, arm)
		}
	}
}

// Pin 2 (bifurcation): when input A carries the declared group field and
// input B does not, the interpretation is decided ONCE globally — A groups by
// observed field values, B takes the existing typed contribution_source_skipped
// lane. Before EVALFIX-2D, B's rows all landed in the literal constant group
// "canonical_label" while A grouped by values: one action, two group-key
// semantics, phantom groups in the shared ledger.
func TestComputeContributionsGroupKeyInterpretationDecidedGlobally(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.csv"), []byte("id,canonical_label\n1,North\n2,South\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.csv"), []byte("id,region\n3,East\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), lineageComputePlan(
		[]string{"a.csv", "b.csv"},
		map[string]string{
			"group_key":     "canonical_label",
			"metric":        "record_count",
			"operation":     "count",
			"item_id_field": "id",
			"role":          "target",
		},
	))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	groups := map[string]bool{}
	for _, rec := range res.Contributions {
		groups[rec.GroupKey.String()] = true
	}
	if groups["canonical_label"] {
		t.Fatalf("contributions=%+v, literal group %q must not exist (per-input demotion regressed)", res.Contributions, "canonical_label")
	}
	if !groups["North"] || !groups["South"] || len(res.Contributions) != 2 {
		t.Fatalf("contributions=%+v, want exactly A's observed field-value groups North/South", res.Contributions)
	}
	skipped := false
	for _, artifact := range res.Artifacts {
		for _, child := range artifact.Children {
			if child.Kind == "contribution_source_skipped" && strings.Contains(child.ID, "b.csv") {
				skipped = true
			}
		}
	}
	if !skipped {
		t.Fatalf("artifacts=%+v, want b.csv on the typed contribution_source_skipped lane", res.Artifacts)
	}
}

// Pin 3 (typed escape, §15.12): group_key_literal stays the declared-constant
// lane even when the literal equals a ledger schema name — with or without a
// same-named input field. The hard gate must never fire on the escape lane.
func TestComputeContributionsGroupKeyLiteralEscapesLineageGate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "users.csv"), []byte("id,canonical_label\nu1,North\nu2,South\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plain.csv"), []byte("id,region\nu3,East\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, inputs := range [][]string{{"users.csv"}, {"plain.csv"}} {
		res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), lineageComputePlan(
			inputs,
			map[string]string{
				"group_key_literal": "canonical_label",
				"group_key":         "canonical_label",
				"metric":            "record_count",
				"operation":         "count",
				"item_id_field":     "id",
				"role":              "target",
			},
		))
		if err != nil {
			t.Fatalf("Run literal inputs=%v: %v (declared literal must never be gated)", inputs, err)
		}
		for _, rec := range res.Contributions {
			if rec.GroupKey.String() != "canonical_label" {
				t.Fatalf("inputs=%v contribution=%+v, want constant literal group", inputs, rec)
			}
		}
	}
}

// Pin 4 (zero-change): a plain constant label that is neither a ledger schema
// name nor an input field keeps its byte-identical constant-lane behavior.
func TestComputeContributionsPlainConstantGroupKeyUnchanged(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "users.csv"), []byte("id,region\nu1,East\nu2,West\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), lineageComputePlan(
		[]string{"users.csv"},
		map[string]string{
			"group_key":     "all_active",
			"metric":        "record_count",
			"operation":     "count",
			"item_id_field": "id",
			"role":          "target",
		},
	))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Contributions) != 2 {
		t.Fatalf("contributions=%+v, want 2", res.Contributions)
	}
	for _, rec := range res.Contributions {
		if rec.GroupKey.String() != "all_active" {
			t.Fatalf("contribution=%+v, want constant group all_active", rec)
		}
	}
}

// Pin 5 (closed-set tripwire, bidirectional): the canonical ledger field-name
// closed set is the single-source predicate of the lineage gate. Adding a
// JSON field to any of the four ledger structs without registering it in
// canonicalLedgerFieldTagNames goes red, and a registered name matching no
// struct tag goes red. The alias side is pinned verbatim below so an alias
// registry edit is always a deliberate closed-set change.
func TestCanonicalLedgerFieldNameSetTripwire(t *testing.T) {
	tags := map[string]bool{}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(ContributionRecord{}),
		reflect.TypeOf(ReconcileGroup{}),
		reflect.TypeOf(EntityResolutionRecord{}),
		reflect.TypeOf(RowDecision{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			if tag == "" || tag == "-" {
				t.Fatalf("%s.%s has no JSON tag; ledger structs must stay fully tagged for the closed set", typ.Name(), typ.Field(i).Name)
			}
			tags[strings.ToLower(strings.TrimSpace(tag))] = true
		}
	}
	declared := map[string]bool{}
	for _, name := range canonicalLedgerFieldTagNames {
		declared[name] = true
	}
	for tag := range tags {
		if !declared[tag] {
			t.Errorf("ledger struct JSON tag %q is not registered in canonicalLedgerFieldTagNames (EVALFIX-2D closed set out of sync)", tag)
		}
	}
	for name := range declared {
		if !tags[name] {
			t.Errorf("canonicalLedgerFieldTagNames entry %q matches no ledger struct JSON tag; remove it", name)
		}
	}
	set := canonicalLedgerFieldNameSet()
	for tag := range tags {
		if !set[tag] {
			t.Errorf("closed set is missing ledger struct JSON tag %q", tag)
		}
	}
	for _, aliases := range ledgerFieldAliasRegistries() {
		for _, alias := range aliases {
			if !set[strings.ToLower(alias)] {
				t.Errorf("closed set is missing registered alias %q", alias)
			}
		}
	}
	// Verbatim membership golden: any growth or shrink of the closed set is a
	// deliberate change reviewed against the lineage gate's blast radius.
	want := []string{
		"action", "actual", "actual_total", "actual_value", "aggregation",
		"bucket", "candidates", "canonical", "canonical_id", "canonical_id_field",
		"canonical_label", "canonical_label_field", "cell", "contribution",
		"contribution_role", "decision", "delta", "details", "diff",
		"difference", "dimension_key", "dimensions", "evidence_refs",
		"expected", "expected_total", "expected_value", "field", "group",
		"group_key", "id", "id_field", "input_field", "item", "item_id", "kind",
		"label", "label_field", "ledger_role", "level", "location", "locator",
		"measure", "metric", "metric_name", "normalized_fields", "normalized_id",
		"normalized_label", "notes", "op", "operation", "outcome", "raw",
		"raw_value", "reason", "reconcile_role", "reconcile_scope",
		"record_id", "reference_id_field", "reference_label_field", "role",
		"row", "row_id", "rule", "rule_id", "rule_ref", "rule_refs", "scope",
		"source", "source_field", "source_locator", "source_name_field",
		"source_value", "span", "status", "summary", "target_id",
		"target_label", "value", "values",
	}
	got := make([]string, 0, len(set))
	for name := range set {
		got = append(got, name)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("canonicalLedgerFieldNameSet membership drifted.\n got: %s\nwant: %s", strings.Join(got, ","), strings.Join(want, ","))
	}
}

// Pin 6 (script lane, SOFT only): script-minted target contributions whose
// GroupKey equals a ledger schema name get a ContractWarnings entry — the
// result still validates and the answer is untouched. Headers/schema-name
// membership is a noisy signal for script records (value and schema
// namespaces may legally overlap), so it must never hard-gate there.
func TestScriptLaneGroupKeyLineageWarnsSoftly(t *testing.T) {
	plan := TaskPlan{
		Script:         "emit_result(...)",
		OutputContract: OutputContract{Format: OutputPlainSingleLine, ExplanationAllowed: false},
	}
	res := Result{
		Answer:        "1",
		ConsumedPaths: []string{"orders.csv"},
		Contributions: []ContributionRecord{{
			ItemID:        "r1",
			Source:        "orders.csv",
			SourceLocator: "line:2",
			GroupKey:      "metric",
			Metric:        "amount",
			Value:         "1",
			Operation:     "add",
			Role:          "target",
		}},
	}
	validated, err := validateRunnerResult(plan, res)
	if err != nil {
		t.Fatalf("validateRunnerResult: %v (script-lane lineage must stay soft)", err)
	}
	if validated.Answer != "1" {
		t.Fatalf("Answer=%q, want untouched %q", validated.Answer, "1")
	}
	found := false
	for _, warning := range validated.ContractWarnings {
		if strings.Contains(warning, `"metric"`) && strings.Contains(warning, "group_key_field") {
			found = true
		}
	}
	if !found {
		t.Fatalf("ContractWarnings=%q, want soft lineage warning naming the key and the field-grouping repair", validated.ContractWarnings)
	}
	// A benign constant label draws no warning.
	res.Contributions[0].GroupKey = "all_matching_rows"
	validated, err = validateRunnerResult(plan, res)
	if err != nil {
		t.Fatalf("validateRunnerResult benign: %v", err)
	}
	for _, warning := range validated.ContractWarnings {
		if strings.Contains(warning, "all_matching_rows") {
			t.Fatalf("ContractWarnings=%q, benign constant label must not warn", validated.ContractWarnings)
		}
	}
}
