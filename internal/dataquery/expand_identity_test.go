package dataquery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// V9-1 (colleague_merge_audit §40.15): derived row identity follows the
// action's derivation topology. A 1:N action (expand_records/join_records)
// mints one `_row_identity` per derived row (`<parent identity>#<ordinal>`)
// while `_source_locator` keeps the parent lineage; 1:1 actions inherit the
// identity unchanged; N:1 (group_records) mints an artifact-local group
// identity that cannot collide with input rows. Ledger dedupe and the
// cross-ledger consistency gate key on that identity, so sibling rows that
// share a parent no longer fold into one contribution or reject each other.

func writeExpandIdentityFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,tags\n1,a;b;c\n2,d\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func expandTagsAction(input, output string) DataAction {
	return DataAction{
		ID:             "expand_" + output,
		Kind:           DataActionExpandRecords,
		InputPaths:     []string{input},
		OutputArtifact: output,
		Params: map[string]string{
			"source_field": "tags",
			"target_field": "tag",
			"delimiter":    ";",
		},
	}
}

func countAction(input string, extra map[string]string) DataAction {
	params := map[string]string{
		"group_key_literal": "all",
		"metric":            "tag_count",
		"operation":         "count",
		"reason":            "one contribution per expanded row",
	}
	for key, value := range extra {
		params[key] = value
	}
	return DataAction{
		ID:         "count_" + input,
		Kind:       DataActionComputeContribs,
		InputPaths: []string{input},
		Params:     params,
	}
}

func filterTagAction(input, output, tag string) DataAction {
	return DataAction{
		ID:             "filter_" + output,
		Kind:           DataActionFilterRecords,
		InputPaths:     []string{input},
		OutputArtifact: output,
		Params: map[string]string{
			"filters_json": `[{"field":"tag","op":"eq","value":"` + tag + `"}]`,
			"reason":       "keep one tag",
		},
	}
}

func contributionRowIdentities(records []ContributionRecord) []string {
	out := make([]string, 0, len(records))
	for _, rec := range records {
		out = append(out, rec.RowIdentity.String())
	}
	return out
}

// Pin 1 (red→green): expand→count counts every derived row. Before V9-1 the
// three tag siblings of item 1 shared identity items.csv#1 and the dedupe
// folded them into one contribution, publishing "2" for four rows.
func TestActionRunnerExpandThenCountCountsEveryDerivedRow(t *testing.T) {
	root := t.TempDir()
	writeExpandIdentityFixture(t, root)
	plan := decisionLineagePlan([]DataAction{
		expandTagsAction("items.csv", "item_tags"),
		countAction("item_tags", nil),
		{ID: "reconcile", Kind: DataActionReconcile},
	})
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "4" || len(res.Contributions) != 4 {
		t.Fatalf("Answer=%q Contributions=%d (%v), want 4 contributions over 4 expanded rows", res.Answer, len(res.Contributions), contributionRowIdentities(res.Contributions))
	}
	wantIdentity := []string{"items.csv#1#1", "items.csv#1#2", "items.csv#1#3", "items.csv#2#1"}
	wantLocator := []string{"items.csv#1", "items.csv#1", "items.csv#1", "items.csv#2"}
	for i, rec := range res.Contributions {
		if rec.RowIdentity.String() != wantIdentity[i] || rec.SourceLocator.String() != wantLocator[i] || rec.ItemID.String() != wantIdentity[i] {
			t.Fatalf("Contributions[%d]=%+v, want row_identity=%q (item_id follows it) and source_locator=%q (lineage)", i, rec, wantIdentity[i], wantLocator[i])
		}
	}
}

// Pin 2 (red→green): sibling rows from one parent may carry different
// decisions. Before V9-1 the consistency gate saw include+exclude on the one
// shared identity and rejected the plan.
func TestActionRunnerExpandThenFilterSiblingDecisionsNotRejected(t *testing.T) {
	root := t.TempDir()
	writeExpandIdentityFixture(t, root)
	plan := decisionLineagePlan([]DataAction{
		expandTagsAction("items.csv", "item_tags"),
		filterTagAction("item_tags", "tag_a_rows", "a"),
		countAction("tag_a_rows", nil),
		{ID: "reconcile", Kind: DataActionReconcile},
	})
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v (sibling derived rows must hold independent decisions)", err)
	}
	if res.Answer != "1" {
		t.Fatalf("Answer=%q, want 1 (only tag a)", res.Answer)
	}
	// Four filter decisions (one per expanded row) plus the contribution-
	// derived audit decision for the included row: every identity is
	// distinct per derived row and no identity carries conflicting decisions.
	decisionByIdentity := map[string]string{}
	for _, row := range res.Rows {
		if row.RowIdentity == "" {
			t.Fatalf("row decision %+v lacks row_identity", row)
		}
		if prev, ok := decisionByIdentity[row.RowIdentity]; ok && prev != row.Decision {
			t.Fatalf("row_identity %q carries conflicting decisions %q/%q in %+v", row.RowIdentity, prev, row.Decision, res.Rows)
		}
		decisionByIdentity[row.RowIdentity] = row.Decision
	}
	if len(decisionByIdentity) != 4 || decisionByIdentity["items.csv#1#1"] != "include" || decisionByIdentity["items.csv#1#2"] != "exclude" {
		t.Fatalf("Rows=%+v, want 4 distinct row identities with sibling-independent decisions", res.Rows)
	}
}

// Pin 3 (red→green): an explicit item_id_field keeps ItemID authoritative
// (B461) but never collapses siblings — row identity stays distinct.
func TestActionRunnerExpandThenCountWithExplicitItemIDFieldKeepsSiblings(t *testing.T) {
	root := t.TempDir()
	writeExpandIdentityFixture(t, root)
	plan := decisionLineagePlan([]DataAction{
		expandTagsAction("items.csv", "item_tags"),
		countAction("item_tags", map[string]string{"item_id_field": "id"}),
		{ID: "reconcile", Kind: DataActionReconcile},
	})
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "4" || len(res.Contributions) != 4 {
		t.Fatalf("Answer=%q Contributions=%v, want 4", res.Answer, res.Contributions)
	}
	ids := map[string]int{}
	identities := map[string]bool{}
	for _, rec := range res.Contributions {
		ids[rec.ItemID.String()]++
		identities[rec.RowIdentity.String()] = true
	}
	if ids["1"] != 3 || ids["2"] != 1 || len(identities) != 4 {
		t.Fatalf("item_ids=%v row_identities=%v, want item_id 1×3 / 2×1 with four distinct row identities", ids, identities)
	}
}

// Pin 4 (red→green): nested 1:N derivations chain the identity through one
// formatting rule, and the ancestor walk peels exactly back to the locator.
func TestActionRunnerNestedExpandChainsRowIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "items.csv"), []byte("id,tags\n1,a;b|c\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := expandTagsAction("items.csv", "first")
	second := DataAction{
		ID:             "expand_second",
		Kind:           DataActionExpandRecords,
		InputPaths:     []string{"first"},
		OutputArtifact: "second",
		Params: map[string]string{
			"source_field": "tag",
			"target_field": "leaf",
			"delimiter":    "|",
		},
	}
	plan := decisionLineagePlan([]DataAction{
		first, second,
		countAction("second", nil),
		{ID: "reconcile", Kind: DataActionReconcile},
	})
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := strings.Join(contributionRowIdentities(res.Contributions), ",")
	if got != "items.csv#1#1#1,items.csv#1#2#1,items.csv#1#2#2" || res.Answer != "3" {
		t.Fatalf("Answer=%q identities=%s, want chained identities", res.Answer, got)
	}
	for _, rec := range res.Contributions {
		if rec.SourceLocator.String() != "items.csv#1" {
			t.Fatalf("contribution %+v, want lineage locator items.csv#1", rec)
		}
	}
	ancestors := rowIdentityAncestors("items.csv#1#2#1", "items.csv#1")
	if strings.Join(ancestors, ",") != "items.csv#1#2,items.csv#1" {
		t.Fatalf("rowIdentityAncestors=%v, want items.csv#1#2,items.csv#1", ancestors)
	}
	if got := rowIdentityAncestors("items.csv#1", "items.csv#1"); got != nil {
		t.Fatalf("a raw row has no ancestors, got %v", got)
	}
	if got := rowIdentityAncestors("other.csv#7#1", "items.csv#1"); got != nil {
		t.Fatalf("an identity not derived from the locator must fail open, got %v", got)
	}
	if got := rowIdentityAncestors("items.csv#1#x", "items.csv#1"); got != nil {
		t.Fatalf("a non-ordinal suffix is not a derivation step, got %v", got)
	}
}

// Pin 5 (red→green): N:1 group rows carry an artifact-local identity, so a
// parent exclusion of input row 1 cannot be misread as excluding group 1.
func TestActionRunnerGroupRowIdentityDoesNotCollideWithInputRows(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.csv"), []byte("id,bucket,active\n1,B,false\n2,A,true\n3,A,true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := decisionLineagePlan([]DataAction{
		{
			ID:             "filter_active",
			Kind:           DataActionFilterRecords,
			InputPaths:     []string{"x.csv"},
			OutputArtifact: "active_rows",
			Params: map[string]string{
				"filters_json": `[{"field":"active","op":"eq","value":"true"}]`,
				"reason":       "active rows only",
			},
		},
		{
			ID:             "group",
			Kind:           DataActionGroupRecords,
			InputPaths:     []string{"active_rows"},
			OutputArtifact: "buckets",
			Params: map[string]string{
				"group_fields": "bucket",
				"text_fields":  "id",
				"target_field": "members",
			},
		},
		countAction("buckets", nil),
		{ID: "reconcile", Kind: DataActionReconcile},
	})
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v (group row 1 is not input row 1)", err)
	}
	if res.Answer != "1" || len(res.Contributions) != 1 {
		t.Fatalf("Answer=%q Contributions=%+v, want one bucket", res.Answer, res.Contributions)
	}
	if got := res.Contributions[0].RowIdentity.String(); got != "active_rows#group#1" {
		t.Fatalf("group row identity=%q, want active_rows#group#1", got)
	}
}

// Pin 6 (baseline guard for the ancestor lane): a contribution over a derived
// row whose PARENT the decision ledger excludes is still a self-contradiction
// (B461 class) and must fail naming both the derived row and its ancestor.
func TestActionRunnerParentExclusionPropagatesToExpandedRows(t *testing.T) {
	root := t.TempDir()
	writeExpandIdentityFixture(t, root)
	plan := decisionLineagePlan([]DataAction{
		{
			ID:             "filter_item_one",
			Kind:           DataActionFilterRecords,
			InputPaths:     []string{"items.csv"},
			OutputArtifact: "item_one",
			Params: map[string]string{
				"filters_json": `[{"field":"id","op":"eq","value":"1"}]`,
				"reason":       "item 1 only",
			},
		},
		expandTagsAction("items.csv", "all_tags"),
		countAction("all_tags", nil),
		{ID: "reconcile", Kind: DataActionReconcile},
	})
	_, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err == nil {
		t.Fatal("Run err=nil, want the excluded parent to reject its expanded child contribution")
	}
	for _, want := range []string{"items.csv#2#1", "ancestor items.csv#2 excluded", "sums a row the decision ledger excludes"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Run err=%v, want %q", err, want)
		}
	}
}

// Pin 7: 1:1 actions inherit identity unchanged — for raw rows the identity,
// the lineage locator and the item id are the same string.
func TestActionRunnerOneToOneIdentityInheritsSourceLocator(t *testing.T) {
	root := t.TempDir()
	writeDecisionLineageFixture(t, root)
	plan := decisionLineagePlan([]DataAction{
		activeFilterAction("active_records"),
		contributionAction("active_records"),
		{ID: "reconcile", Kind: DataActionReconcile},
	})
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for i, rec := range res.Contributions {
		want := "observations.csv#" + string(rune('1'+i))
		if rec.RowIdentity.String() != want || rec.ItemID.String() != want || rec.SourceLocator.String() != want {
			t.Fatalf("Contributions[%d]=%+v, want identity == locator == item_id == %q", i, rec, want)
		}
	}
	for _, row := range res.Rows {
		if row.RowIdentity != row.SourceLocator || row.RowIdentity == "" {
			t.Fatalf("row %+v, want inherited identity equal to its locator", row)
		}
	}
}

// Pin 9: the dedupe keys carry row identity — two records equal in every
// other field but distinct in identity are two records.
func TestDedupeContributionRecordsKeepsDistinctRowIdentity(t *testing.T) {
	base := ContributionRecord{ItemID: LooseText("items.csv#1"), Source: LooseText("items.csv"), SourceLocator: LooseText("items.csv#1"), GroupKey: LooseText("all"), Metric: LooseText("n"), Value: LooseText("1"), Operation: LooseText("count"), Role: LooseText("target")}
	a, b := base, base
	a.RowIdentity = LooseText("items.csv#1#1")
	b.RowIdentity = LooseText("items.csv#1#2")
	if got := DedupeContributionRecords([]ContributionRecord{a, b}); len(got) != 2 {
		t.Fatalf("DedupeContributionRecords folded distinct row identities: %+v", got)
	}
	if got := DedupeContributionRecords([]ContributionRecord{a, a}); len(got) != 1 {
		t.Fatalf("identical records must still fold: %+v", got)
	}
}

func TestDedupeRowDecisionRecordsKeepsDistinctRowIdentity(t *testing.T) {
	base := RowDecision{RowID: "items.csv#1", Source: "items.csv", SourceLocator: "items.csv#1", Decision: "include", Reason: "r"}
	a, b := base, base
	a.RowIdentity = "items.csv#1#1"
	b.RowIdentity = "items.csv#1#2"
	if got := DedupeRowDecisionRecords([]RowDecision{a, b}); len(got) != 2 {
		t.Fatalf("DedupeRowDecisionRecords folded distinct row identities: %+v", got)
	}
	if got := DedupeRowDecisionRecords([]RowDecision{a, a}); len(got) != 1 {
		t.Fatalf("identical decisions must still fold: %+v", got)
	}
}

// Pin 12: `_row_identity` is runner-owned; derive_fields may not overwrite
// it, same as the other reserved origin fields.
func TestValidateDeriveFieldSpecRejectsRowIdentityTarget(t *testing.T) {
	err := validateDeriveFieldSpec(DataActionDeriveFields, deriveFieldSpec{SourceField: "id", TargetField: "_row_identity", Operation: "copy"}, map[string]bool{"id": true}, "items")
	if err == nil || !strings.Contains(err.Error(), "non-reserved output field name") {
		t.Fatalf("validateDeriveFieldSpec err=%v, want the reserved-target rejection for _row_identity", err)
	}
	if !actionReservedSourceField(" _Row_Identity ") {
		t.Fatal("_row_identity must be a reserved origin field (same predicate as _source_locator)")
	}
}

// Join is the second 1:N runner: each (left,right) match is its own derived
// row with an ordinal identity under the left (base) row's lineage.
func TestActionRunnerJoinRecordsStampDerivedRowIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("order_id,vendor\nO1,V1\nO2,V2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lines.csv"), []byte("vendor,amount\nV1,10\nV1,5\nV2,2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := decisionLineagePlan([]DataAction{
		{
			ID:             "join",
			Kind:           DataActionJoinRecords,
			InputPaths:     []string{"orders.csv", "lines.csv"},
			OutputArtifact: "joined",
			Params: map[string]string{
				"left_fields":  `["vendor"]`,
				"right_fields": `["vendor"]`,
			},
		},
		{
			ID:         "sum",
			Kind:       DataActionComputeContribs,
			InputPaths: []string{"joined"},
			Params: map[string]string{
				"group_key_literal": "all",
				"metric":            "amount",
				"value_field":       "amount",
				"operation":         "add",
				"reason":            "joined lines",
			},
		},
		{ID: "reconcile", Kind: DataActionReconcile},
	})
	res, err := (ActionRunner{RepoRoot: root}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := strings.Join(contributionRowIdentities(res.Contributions), ",")
	if res.Answer != "17" || got != "orders.csv#1#1,orders.csv#1#2,orders.csv#2#1" {
		t.Fatalf("Answer=%q identities=%s, want ordinal identities under the left row", res.Answer, got)
	}
	for _, rec := range res.Contributions {
		if !strings.HasPrefix(rec.SourceLocator.String(), "orders.csv#") || strings.Count(rec.SourceLocator.String(), "#") != 1 {
			t.Fatalf("contribution %+v, want left-row lineage locator", rec)
		}
	}
}
