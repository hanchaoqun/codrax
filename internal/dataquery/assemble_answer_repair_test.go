package dataquery

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This file pins the DL-A projection-lineage repairs and their
// adversarial-review corrections (ledger §7.12 as-built ruling;
// specimen eval data_json_strict_ids-20260705-050016):
//
//  1. Projection priority ladder under the answer-repair lane:
//     declared-mapping-first (fresh recompute from current reconcile
//     groups), then payload fallback, then advisory-traced lineage
//     fallback. Outside the lane, projection behavior is byte-identical
//     to before only when no typed output-field declaration exists —
//     the declared-field mapping (DL-A(b)) is an intentional
//     out-of-lane root fix and applies everywhere.
//  2. Payload admission: contract-format narrowing (JSON payloads only
//     under json_only), declared-schema guard on EVERY rank (anchor
//     included), truncation and kind boundaries.
//  3. Payload ranking: freshness primary (current invocation > newer
//     seeded round > older seeded round), then anchor > declared-key >
//     bare-format, then later production position.
//  4. Reconcile rewrite coupled to publication: the lane rewrite is
//     published only when the lane answer becomes result.Answer.
//  5. Rule-declared output field mapping (typed, no prose extraction).

func pollutedLineageAssemblePlan() TaskPlan {
	return TaskPlan{
		OutputContract: OutputContract{Format: OutputJSONOnly, ExplanationAllowed: false},
		CoverageContract: CoverageContract{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		Actions: []DataAction{
			{ID: "reconcile", Kind: DataActionReconcile},
			{
				ID:   "answer",
				Kind: DataActionAssembleAnswer,
				Params: map[string]string{
					"projection":  "json_object",
					"value_field": "actual",
				},
			},
		},
	}
}

// pollutedLineageSeed reproduces the specimen's contaminated
// contribution chain: the group metric carries the working field name
// active_user_id, so lineage-derived projection publishes
// active_user_ids instead of the contract field ids.
func pollutedLineageSeed() Result {
	return Result{Contributions: []ContributionRecord{
		{
			ItemID:        LooseText("row-1"),
			Source:        LooseText("users.json"),
			SourceLocator: LooseText("line:1"),
			GroupKey:      LooseText("all"),
			Metric:        LooseText("active_user_id"),
			Value:         LooseText("u1"),
			Operation:     LooseText("include"),
			Role:          LooseText("target"),
		},
		{
			ItemID:        LooseText("row-3"),
			Source:        LooseText("users.json"),
			SourceLocator: LooseText("line:3"),
			GroupKey:      LooseText("all"),
			Metric:        LooseText("active_user_id"),
			Value:         LooseText("u3"),
			Operation:     LooseText("include"),
			Role:          LooseText("target"),
		},
	}}
}

func emittedPayloadArtifact(id, payload, shape string) DataArtifact {
	return DataArtifact{
		ID:       id,
		Kind:     ArtifactKindCustomPayload,
		Summary:  "script emitted extra payload field(s)",
		Sample:   []string{payload},
		RowCount: 1,
		Fields: map[string]string{
			"payload_keys": "ids",
			"json_shape":   shape,
		},
	}
}

func TestAssembleAnswerRepairLaneSelectsAnswerBearingArtifactOverPollutedLineage(t *testing.T) {
	seed := pollutedLineageSeed()
	seed.Artifacts = []DataArtifact{emittedPayloadArtifact(ArtifactIDEmittedPayload, `{"ids":["u1","u3"]}`, "object(keys=ids)")}

	// Mutation witness: outside the answer-repair lane the projection
	// source stays bound to the reconcile-group lineage — the polluted
	// field name is published exactly as before this fix.
	res, err := (ActionRunner{RepoRoot: t.TempDir(), Seed: seed}).Run(context.Background(), pollutedLineageAssemblePlan())
	if err != nil {
		t.Fatalf("Run without repair lane: %v", err)
	}
	if res.Answer != `{"active_user_ids":["u1","u3"]}` {
		t.Fatalf("Answer=%q, want polluted lineage projection outside the repair lane", res.Answer)
	}

	// Inside the lane with no typed declaration (rung 2), the typed
	// answer-bearing artifact is selected as projection source and its
	// canonical payload re-encoding is published. The lane answer IS
	// result.Answer here, so the reconcile rewrite is published with it
	// (publication-coupling form A).
	res, err = (ActionRunner{
		RepoRoot:     t.TempDir(),
		Seed:         seed,
		AnswerRepair: AnswerRepairContext{Active: true},
	}).Run(context.Background(), pollutedLineageAssemblePlan())
	if err != nil {
		t.Fatalf("Run with repair lane: %v", err)
	}
	if res.Answer != `{"ids":["u1","u3"]}` {
		t.Fatalf("Answer=%q, want answer-bearing artifact payload under the repair lane", res.Answer)
	}
	var assembled *DataArtifact
	for i := range res.Artifacts {
		if res.Artifacts[i].Kind == string(DataActionAssembleAnswer) {
			assembled = &res.Artifacts[i]
		}
	}
	if assembled == nil {
		t.Fatalf("no assemble_answer artifact in result")
	}
	if assembled.Fields["repair_source_artifact"] != ArtifactIDEmittedPayload {
		t.Fatalf("repair_source_artifact=%q, want emitted_payload", assembled.Fields["repair_source_artifact"])
	}
	if assembled.Fields["projection"] != assembleProjectionAnswerArtifact {
		t.Fatalf("projection=%q, want %q lane marker", assembled.Fields["projection"], assembleProjectionAnswerArtifact)
	}
	if res.Reconcile == nil || res.Reconcile.ActualAnswer.String() != `{"ids":["u1","u3"]}` || res.Reconcile.ExpectedAnswer.String() != `{"ids":["u1","u3"]}` {
		t.Fatalf("reconcile answer faces not aligned with the published lane projection: %+v", res.Reconcile)
	}
}

// Rung-1 pin (mutation witness for the pre-ruling short-circuit): when
// a unique typed declaration exists AND the deterministic
// reconcile-group projection can compute the declared single-key object
// from CURRENT groups, that fresh recompute wins and the payload is
// never projected. Restoring the old payload-first short-circuit
// publishes the stale payload content ("stale") and turns this test
// red.
func TestAssembleAnswerRepairLaneDeclaredMappingOutranksPayload(t *testing.T) {
	seed := pollutedLineageSeed()
	seed.RuleCoverage = []RuleCoverageRecord{{
		RuleID:      LooseText("rule_2"),
		RuleText:    LooseText("The JSON object must have one field named ids"),
		Status:      LooseText("derived"),
		OutputField: LooseText("ids"),
	}}
	seed.Artifacts = []DataArtifact{emittedPayloadArtifact(ArtifactIDEmittedPayload, `{"ids":["stale"]}`, "object(keys=ids)")}
	res, err := (ActionRunner{
		RepoRoot:           t.TempDir(),
		Seed:               seed,
		AnswerRepair:       AnswerRepairContext{Active: true},
		SeedArtifactRounds: map[string]int{ArtifactIDEmittedPayload: 1},
	}).Run(context.Background(), pollutedLineageAssemblePlan())
	if err != nil {
		t.Fatalf("Run with declared mapping available: %v", err)
	}
	if res.Answer != `{"ids":["u1","u3"]}` {
		t.Fatalf("Answer=%q, want declared-mapping fresh recompute, never the stored payload", res.Answer)
	}
	for _, artifact := range res.Artifacts {
		if artifact.Kind != string(DataActionAssembleAnswer) {
			continue
		}
		if artifact.Fields["projection"] == assembleProjectionAnswerArtifact || artifact.Fields["repair_source_artifact"] != "" {
			t.Fatalf("assemble artifact %+v, want no payload projection on the declared-mapping rung", artifact.Fields)
		}
	}
}

func TestSelectAnswerBearingRepairSourceTypedPriority(t *testing.T) {
	runner := ActionRunner{}
	contract := OutputContract{Format: OutputJSONOnly, ExplanationAllowed: false}.Normalize()
	schemaMatch := emittedPayloadArtifact("schema_payload", `{"ids":["u1","u3"]}`, "object(keys=ids)")
	anchorNamed := emittedPayloadArtifact("anchor_action", `{"totals":[17]}`, "object(keys=totals)")

	// Declared-schema guard on the anchor rank (review fix): the
	// anchor-named payload contradicts the typed declaration, so it is
	// rejected and the declared-schema match wins — an anchor never
	// overrides the declared output schema.
	source, answer, ok := runner.selectAnswerBearingRepairSource([]DataArtifact{schemaMatch, anchorNamed}, contract, "anchor_action", "ids")
	if !ok || source.ID != "schema_payload" || answer != `{"ids":["u1","u3"]}` {
		t.Fatalf("source=%q answer=%q ok=%v, want declared-schema guard to reject the contradicting anchor payload", source.ID, answer, ok)
	}

	// Without a declaration and at equal freshness, the anchor rank
	// wins over a bare contract-format match regardless of position.
	bare := emittedPayloadArtifact("other_payload", `{"totals":[18]}`, "object(keys=totals)")
	source, answer, ok = runner.selectAnswerBearingRepairSource([]DataArtifact{anchorNamed, bare}, contract, "anchor_action", "")
	if !ok || source.ID != "anchor_action" || answer != `{"totals":[17]}` {
		t.Fatalf("source=%q answer=%q ok=%v, want anchor rank ahead of bare-format at equal freshness", source.ID, answer, ok)
	}

	// Without an anchor, the declared-schema match wins.
	source, answer, ok = runner.selectAnswerBearingRepairSource([]DataArtifact{anchorNamed, schemaMatch}, contract, "", "ids")
	if !ok || source.ID != "schema_payload" || answer != `{"ids":["u1","u3"]}` {
		t.Fatalf("source=%q answer=%q ok=%v, want declared-schema match to win", source.ID, answer, ok)
	}

	// A typed rule declaration with no matching payload selects nothing:
	// projecting a contradicting payload would invent an output schema.
	if source, answer, ok := runner.selectAnswerBearingRepairSource([]DataArtifact{anchorNamed}, contract, "", "ids"); ok {
		t.Fatalf("source=%q answer=%q, want no selection when payload contradicts typed declaration", source.ID, answer)
	}

	// Without any typed declaration, a bare contract-format match is
	// admissible.
	if _, _, ok := runner.selectAnswerBearingRepairSource([]DataArtifact{anchorNamed}, contract, "", ""); !ok {
		t.Fatalf("want bare contract-format admission without typed declaration")
	}

	// Equal freshness and equal rank: later production position wins.
	if source, _, ok := runner.selectAnswerBearingRepairSource([]DataArtifact{bare, anchorNamed}, contract, "", ""); !ok || source.ID != "anchor_action" {
		t.Fatalf("source=%q ok=%v, want later production position at full tie", source.ID, ok)
	}

	// Typed admission boundaries: truncated payloads and non-payload
	// kinds (e.g. the contested assemble_answer artifact itself) are
	// never candidates.
	truncated := emittedPayloadArtifact("truncated_payload", `{"ids":["u1"]}`+"\n...[truncated]", "object(keys=ids)")
	contested := DataArtifact{
		ID:     "final_answer.json",
		Kind:   string(DataActionAssembleAnswer),
		Sample: []string{`{"ids":["u1","u3"]}`},
		Fields: map[string]string{"json_shape": "object(keys=ids)"},
	}
	if _, _, ok := runner.selectAnswerBearingRepairSource([]DataArtifact{truncated, contested}, contract, "", ""); ok {
		t.Fatalf("want truncated payloads and non-custom_payload kinds rejected")
	}
}

// Contract-format narrowing pin (review redline): payload candidates
// are JSON by construction, so they are admissible ONLY under a
// json_only contract. A single-line JSON object satisfies the
// line-shape validator of plain_single_line/csv_line, so without this
// narrowing a JSON payload would be published against a non-JSON
// output contract — accepting one under plain/csv turns this test red.
func TestSelectAnswerBearingRepairSourceContractNarrowing(t *testing.T) {
	runner := ActionRunner{}
	payload := emittedPayloadArtifact(ArtifactIDEmittedPayload, `{"ids":["u1","u3"]}`, "object(keys=ids)")
	for _, contract := range []OutputContract{
		{Format: OutputPlainSingleLine, ExplanationAllowed: false},
		{Format: OutputCSVLine, ExplanationAllowed: false},
	} {
		if _, _, ok := runner.selectAnswerBearingRepairSource([]DataArtifact{payload}, contract.Normalize(), "", ""); ok {
			t.Fatalf("contract=%s: JSON payload admitted, want contract-format narrowing rejection", contract.Format)
		}
	}
	if _, _, ok := runner.selectAnswerBearingRepairSource([]DataArtifact{payload}, OutputContract{Format: OutputJSONOnly}.Normalize(), "", ""); !ok {
		t.Fatalf("json_only contract must keep admitting the JSON payload")
	}
}

// Freshness-primary ranking pin (mutation witness: dropping the
// freshness ordering turns this red): a stale anchor-named payload from
// an old round never suppresses fresher payloads — neither a newer
// seeded round nor the current invocation's output.
func TestSelectAnswerBearingRepairSourceFreshnessPrimary(t *testing.T) {
	contract := OutputContract{Format: OutputJSONOnly, ExplanationAllowed: false}.Normalize()
	staleAnchor := emittedPayloadArtifact("anchor_action", `{"ids":["stale"]}`, "object(keys=ids)")
	newerSeed := emittedPayloadArtifact(ArtifactIDEmittedPayload, `{"ids":["newer"]}`, "object(keys=ids)")
	current := emittedPayloadArtifact("current_payload", `{"ids":["current"]}`, "object(keys=ids)")

	// Newer seeded round beats the older anchor-named round.
	runner := ActionRunner{
		SeedArtifactRounds: map[string]int{"anchor_action": 1, ArtifactIDEmittedPayload: 3},
		seedArtifactCount:  2,
	}
	source, answer, ok := runner.selectAnswerBearingRepairSource([]DataArtifact{staleAnchor, newerSeed}, contract, "anchor_action", "")
	if !ok || source.ID != ArtifactIDEmittedPayload || answer != `{"ids":["newer"]}` {
		t.Fatalf("source=%q answer=%q ok=%v, want the newer seeded round over the stale anchor", source.ID, answer, ok)
	}

	// Current-invocation output beats every seeded round, anchor
	// included.
	source, answer, ok = runner.selectAnswerBearingRepairSource([]DataArtifact{staleAnchor, newerSeed, current}, contract, "anchor_action", "")
	if !ok || source.ID != "current_payload" || answer != `{"ids":["current"]}` {
		t.Fatalf("source=%q answer=%q ok=%v, want the current invocation's payload to win", source.ID, answer, ok)
	}
}

// Rung-3 pin (mutation witness: restoring the silent fallback turns
// this red): a typed declaration exists, the declared mapping is not
// deterministically computable from the current groups, and no
// admissible payload candidate matches — the contested lineage
// projection is still published, but with a typed advisory trace on
// result.ContractWarnings and the assemble artifact, never silently.
func TestAssembleAnswerRepairLaneNoCandidateLeavesTypedAdvisory(t *testing.T) {
	seed := Result{
		Contributions: []ContributionRecord{
			{
				ItemID:        LooseText("row-1"),
				Source:        LooseText("users.json"),
				SourceLocator: LooseText("line:1"),
				GroupKey:      LooseText("all"),
				Metric:        LooseText("metric_a"),
				Value:         LooseText("u1"),
				Operation:     LooseText("include"),
				Role:          LooseText("target"),
			},
			{
				ItemID:        LooseText("row-3"),
				Source:        LooseText("users.json"),
				SourceLocator: LooseText("line:3"),
				GroupKey:      LooseText("all"),
				Metric:        LooseText("metric_b"),
				Value:         LooseText("u3"),
				Operation:     LooseText("include"),
				Role:          LooseText("target"),
			},
		},
		RuleCoverage: []RuleCoverageRecord{{
			RuleID:      LooseText("rule_2"),
			RuleText:    LooseText("The JSON object must have one field named ids"),
			Status:      LooseText("derived"),
			OutputField: LooseText("ids"),
		}},
	}
	res, err := (ActionRunner{
		RepoRoot:     t.TempDir(),
		Seed:         seed,
		AnswerRepair: AnswerRepairContext{Active: true},
	}).Run(context.Background(), pollutedLineageAssemblePlan())
	if err != nil {
		t.Fatalf("Run with declaration but no candidate: %v", err)
	}
	if jsonObjectHasSingleKey(res.Answer, "ids") {
		t.Fatalf("Answer=%q, fixture must not be declared-mappable", res.Answer)
	}
	advisoryFound := false
	for _, warning := range res.ContractWarnings {
		if strings.Contains(warning, `declared output field "ids" not projected`) {
			advisoryFound = true
		}
	}
	if !advisoryFound {
		t.Fatalf("ContractWarnings=%q, want typed declared-field advisory", res.ContractWarnings)
	}
	assembleSeen := false
	for _, artifact := range res.Artifacts {
		if artifact.Kind != string(DataActionAssembleAnswer) {
			continue
		}
		assembleSeen = true
		if artifact.Fields[assembleDeclaredFieldUnprojectedKey] != "ids" {
			t.Fatalf("assemble artifact fields=%+v, want %s=ids trace", artifact.Fields, assembleDeclaredFieldUnprojectedKey)
		}
	}
	if !assembleSeen {
		t.Fatalf("no assemble_answer artifact in result")
	}
	// Hygiene pin: the accumulated rule-coverage ledger artifact
	// declares the typed output_field column.
	for _, artifact := range res.Artifacts {
		if artifact.ID != "workflow_rule_coverage" {
			continue
		}
		if !strings.Contains(strings.Join(artifact.Headers, ","), "output_field") {
			t.Fatalf("workflow_rule_coverage headers=%q, want output_field column", artifact.Headers)
		}
	}
}

// Publication-coupling form B (mutation witness: restoring the
// unconditional lane rewrite turns this red with "expected_answer ...
// does not match result.answer"): when a trailing custom_transform
// supplies its own answer, the lane projection did NOT become
// result.Answer, so the lane's reconcile rewrite must not be published
// — the pre-lane reconcile passes through instead.
func TestAssembleAnswerLaneReconcileRewriteCoupledToPublication(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	seed := Result{
		Contributions: []ContributionRecord{
			{
				ItemID:        LooseText("row-1"),
				Source:        LooseText("users.json"),
				SourceLocator: LooseText("line:1"),
				GroupKey:      LooseText("all"),
				Metric:        LooseText("metric_a"),
				Value:         LooseText("u1"),
				Operation:     LooseText("include"),
				Role:          LooseText("target"),
			},
			{
				ItemID:        LooseText("row-3"),
				Source:        LooseText("users.json"),
				SourceLocator: LooseText("line:3"),
				GroupKey:      LooseText("all"),
				Metric:        LooseText("metric_b"),
				Value:         LooseText("u3"),
				Operation:     LooseText("include"),
				Role:          LooseText("target"),
			},
		},
		Artifacts: []DataArtifact{emittedPayloadArtifact(ArtifactIDEmittedPayload, `{"ids":["u1","u3"]}`, "object(keys=ids)")},
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.txt"), []byte("x\n"), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	plan := pollutedLineageAssemblePlan()
	plan.Actions = append(plan.Actions, DataAction{
		ID:         "final_transform",
		Kind:       DataActionCustomTransform,
		InputPaths: []string{"data.txt"},
		Script:     `emit_result({"answer": '{"x": 1}', "output_contract": {"format": "json_only", "explanation_allowed": False}})`,
	})
	res, err := (ActionRunner{
		RepoRoot:           root,
		Seed:               seed,
		AnswerRepair:       AnswerRepairContext{Active: true},
		SeedArtifactRounds: map[string]int{ArtifactIDEmittedPayload: 1},
	}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run with trailing custom_transform: %v", err)
	}
	if res.Answer != `{"x": 1}` {
		t.Fatalf("Answer=%q, want the custom_transform answer published", res.Answer)
	}
	if res.Reconcile == nil {
		t.Fatalf("reconcile missing from published result")
	}
	if res.Reconcile.ExpectedAnswer.String() == `{"ids":["u1","u3"]}` || res.Reconcile.ActualAnswer.String() == `{"ids":["u1","u3"]}` {
		t.Fatalf("reconcile=%+v, want the unpublished lane rewrite withheld", res.Reconcile)
	}
	for _, group := range res.Reconcile.Groups {
		if group.Scope.String() == "final_answer" {
			t.Fatalf("reconcile groups=%+v, want no final_answer projection group from the unpublished lane rewrite", res.Reconcile.Groups)
		}
	}
}

func TestAssembleAnswerRuleDeclaredOutputFieldDeterministicMapping(t *testing.T) {
	// Typed declaration present: the single-key JSON object projection
	// maps onto the declared field name deterministically — no repair
	// lane involved, this is the root fix for the first wrong-name
	// projection.
	seed := pollutedLineageSeed()
	seed.RuleCoverage = []RuleCoverageRecord{{
		RuleID:      LooseText("rule_2"),
		RuleText:    LooseText("The JSON object must have one field named ids"),
		Status:      LooseText("derived"),
		OutputField: LooseText("ids"),
	}}
	res, err := (ActionRunner{RepoRoot: t.TempDir(), Seed: seed}).Run(context.Background(), pollutedLineageAssemblePlan())
	if err != nil {
		t.Fatalf("Run with typed declaration: %v", err)
	}
	if res.Answer != `{"ids":["u1","u3"]}` {
		t.Fatalf("Answer=%q, want rule-declared output field mapping", res.Answer)
	}

	// Prose-only rule: nothing is extracted from rule_text — the
	// lineage projection stands (no invention).
	seed = pollutedLineageSeed()
	seed.RuleCoverage = []RuleCoverageRecord{{
		RuleID:   LooseText("rule_2"),
		RuleText: LooseText("The JSON object must have one field named ids"),
		Status:   LooseText("derived"),
	}}
	res, err = (ActionRunner{RepoRoot: t.TempDir(), Seed: seed}).Run(context.Background(), pollutedLineageAssemblePlan())
	if err != nil {
		t.Fatalf("Run with prose-only rule: %v", err)
	}
	if res.Answer != `{"active_user_ids":["u1","u3"]}` {
		t.Fatalf("Answer=%q, want lineage projection kept for prose-only rules", res.Answer)
	}

	// Conflicting typed declarations: ambiguity maps nothing.
	seed = pollutedLineageSeed()
	seed.RuleCoverage = []RuleCoverageRecord{
		{RuleID: LooseText("rule_2"), RuleText: LooseText("field ids"), OutputField: LooseText("ids")},
		{RuleID: LooseText("rule_3"), RuleText: LooseText("field identifiers"), OutputField: LooseText("identifiers")},
	}
	res, err = (ActionRunner{RepoRoot: t.TempDir(), Seed: seed}).Run(context.Background(), pollutedLineageAssemblePlan())
	if err != nil {
		t.Fatalf("Run with conflicting declarations: %v", err)
	}
	if res.Answer != `{"active_user_ids":["u1","u3"]}` {
		t.Fatalf("Answer=%q, want lineage projection kept under conflicting declarations", res.Answer)
	}
}

// Wire-token pin: the payload envelope identifiers are load-bearing
// wire tokens — internal-summary detection
// (artifactSummaryObjectLooksInternal), repair-source admission, and
// recorded workflow results all match them verbatim. Changing either
// value silently orphans every stored artifact of the old shape.
func TestPayloadArtifactWireTokensPinned(t *testing.T) {
	if ArtifactKindCustomPayload != "custom_payload" {
		t.Fatalf("ArtifactKindCustomPayload=%q, wire token must stay custom_payload", ArtifactKindCustomPayload)
	}
	if ArtifactIDEmittedPayload != "emitted_payload" {
		t.Fatalf("ArtifactIDEmittedPayload=%q, wire token must stay emitted_payload", ArtifactIDEmittedPayload)
	}
}

func TestRuleDeclaredOutputField(t *testing.T) {
	if got := ruleDeclaredOutputField(nil); got != "" {
		t.Fatalf("got %q, want empty for no records", got)
	}
	single := []RuleCoverageRecord{
		{RuleID: LooseText("rule_1"), RuleText: LooseText("prose only")},
		{RuleID: LooseText("rule_2"), OutputField: LooseText(" ids ")},
	}
	if got := ruleDeclaredOutputField(single); got != "ids" {
		t.Fatalf("got %q, want trimmed single declaration", got)
	}
	conflict := append(single, RuleCoverageRecord{RuleID: LooseText("rule_3"), OutputField: LooseText("identifiers")})
	if got := ruleDeclaredOutputField(conflict); got != "" {
		t.Fatalf("got %q, want empty under conflicting declarations", got)
	}
}

func TestDeriveRulesPlumbsTypedOutputField(t *testing.T) {
	plan := TaskPlan{
		OutputContract: OutputContract{Format: OutputJSONOnly, ExplanationAllowed: false},
		Actions: []DataAction{{
			ID:   "derive",
			Kind: DataActionDeriveRules,
			Params: map[string]string{
				"rules_json": `[{"id":"rule_1","text":"only active users count"},{"id":"rule_2","text":"the JSON object must have one field named ids","output_field":"ids"}]`,
			},
		}},
		ContinueAfter: true,
	}
	res, err := (ActionRunner{RepoRoot: t.TempDir()}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run derive_rules: %v", err)
	}
	byID := map[string]RuleCoverageRecord{}
	for _, rec := range res.RuleCoverage {
		byID[rec.RuleID.String()] = rec
	}
	if got := byID["rule_2"].OutputField.String(); got != "ids" {
		t.Fatalf("rule_2 OutputField=%q, want typed plumb from rules_json", got)
	}
	if got := byID["rule_1"].OutputField.String(); got != "" {
		t.Fatalf("rule_1 OutputField=%q, want empty when emitter declared none", got)
	}
}
