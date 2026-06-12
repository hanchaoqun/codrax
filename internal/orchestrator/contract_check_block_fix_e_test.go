package orchestrator

import (
	"strings"
	"testing"
)

// Fix E coverage. Each test pins one cross-form scenario where the
// graph indexes the symbol under one form but the LLM renders it
// under another — without Fix E (case-sensitive exact match) the
// oracle would falsely report Tier 0 and Fix C / Fix D would fire
// "hallucination" on a real entity. With Fix E the flat index
// resolves the same logical identifier across surface forms.
//
// The stubOracleFixB.SymbolExistsFlat implementation flattens both
// the query and each tier-map key via contract.FlattenIdentifier,
// then matches case-insensitively across forms — same canonical
// rule as the production graphOracle.

// TestFixE_EnumLabelHallucination_YamlTagResolvesGoField pins the
// "config / yaml" scenario: graph indexes Go fields in PascalCase
// (`PipelineMaxSteps`); LLM emits the yaml-tag rendering
// (`pipeline_max_steps`). Fix E flattens both to
// "pipelinemaxsteps" → matches.
func TestFixE_EnumLabelHallucination_YamlTagResolvesGoField(t *testing.T) {
	doc := docWithEnumItems("config_list",
		"pipeline_max_steps",  // yaml-tag form, Go field = PipelineMaxSteps
		"write_enabled",       // yaml-tag form, Go field = WriteEnabled
		"auto_apply",          // yaml-tag form, Go field = AutoApply
	)
	// Oracle indexes the GO FIELD names (Pascal form).
	oracle := &stubOracleFixB{tiers: map[string]int{
		"PipelineMaxSteps": 1,
		"WriteEnabled":     1,
		"AutoApply":        1,
	}}
	if vs := validateEnumerationItemLabelHallucination(doc, oracle, nil); len(vs) != 0 {
		t.Errorf("yaml-tag labels must resolve Pascal-form Go fields via Fix E; got %+v", vs)
	}
}

// TestFixE_EnumLabelHallucination_GoFieldResolvesYamlTag — symmetric
// case: graph indexes yaml form, LLM emits Pascal form.
func TestFixE_EnumLabelHallucination_GoFieldResolvesYamlTag(t *testing.T) {
	doc := docWithEnumItems("config_list",
		"PipelineMaxSteps",
		"WriteEnabled",
		"AutoApply",
	)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"pipeline_max_steps": 1,
		"write_enabled":      1,
		"auto_apply":         1,
	}}
	if vs := validateEnumerationItemLabelHallucination(doc, oracle, nil); len(vs) != 0 {
		t.Errorf("Pascal labels must resolve snake-form indexed names via Fix E; got %+v", vs)
	}
}

// TestFixE_EnumLabelHallucination_ScreamingSnakeResolvesPascal pins
// the env-var rendering (`PIPELINE_MAX_STEPS`) resolving the same
// Go field.
func TestFixE_EnumLabelHallucination_ScreamingSnakeResolvesPascal(t *testing.T) {
	doc := docWithEnumItems("env_list",
		"PIPELINE_MAX_STEPS",
		"WRITE_ENABLED",
		"CODRAX_DEBUG_MODE",
	)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"PipelineMaxSteps": 1,
		"WriteEnabled":     1,
		"CodraxDebugMode":  1,
	}}
	if vs := validateEnumerationItemLabelHallucination(doc, oracle, nil); len(vs) != 0 {
		t.Errorf("SCREAMING_SNAKE env vars must resolve Pascal-form fields via Fix E; got %+v", vs)
	}
}

// TestFixE_EnumLabelHallucination_KebabResolvesCamel pins the
// CLI-flag rendering: `pipeline-max-steps` → `pipelineMaxSteps`.
// Note labels here drop the leading `--` because
// labelLeadingSymbolIdentifier walks identifier chars only (`_` /
// `-` / letters / digits), so the form the LLM emits in the answer
// list is typically the bare form.
func TestFixE_EnumLabelHallucination_KebabResolvesCamel(t *testing.T) {
	doc := docWithEnumItems("cli_list",
		"pipeline-max-steps",
		"auto-apply-mode",
		"write-enabled-flag",
	)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"pipelineMaxSteps": 1,
		"autoApplyMode":    1,
		"writeEnabledFlag": 1,
	}}
	if vs := validateEnumerationItemLabelHallucination(doc, oracle, nil); len(vs) != 0 {
		t.Errorf("kebab CLI flags must resolve camel-form indexed names via Fix E; got %+v", vs)
	}
}

// TestFixE_EnumLabelHallucination_GenuineFabricationStillFires
// confirms Fix E does NOT mask real hallucinations. A name that
// has NO equivalent under any form normalisation still fires.
func TestFixE_EnumLabelHallucination_GenuineFabricationStillFires(t *testing.T) {
	doc := docWithEnumItems("list1",
		"realCheckCoverage",
		"pipeline_imaginary_setting", // no flat-form equivalent in oracle
		"realCheckBudgetSanity",
	)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"realCheckCoverage":     1,
		"realCheckBudgetSanity": 1,
		// pipeline_imaginary_setting flat = "pipelineimaginarysetting"
		// — no entry in oracle under any form → Tier 0 → fire
	}}
	vs := validateEnumerationItemLabelHallucination(doc, oracle, nil)
	if len(vs) != 1 {
		t.Fatalf("genuine fabrication must still fire; got %d:\n  %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "pipeline_imaginary_setting") {
		t.Errorf("violation must list the genuine hallucination; got: %s", vs[0].Detail)
	}
}

// TestFixE_DiagramEndpoint_YamlTagResolvesGoField applies the same
// cross-form contract to Fix D's diagram surface.
func TestFixE_DiagramEndpoint_YamlTagResolvesGoField(t *testing.T) {
	body := "graph TD\n" +
		"    pipeline_max_steps --> write_enabled\n" +
		"    write_enabled --> auto_apply\n"
	doc := docWithDiagram("config_flow", body)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"PipelineMaxSteps": 1,
		"WriteEnabled":     1,
		"AutoApply":        1,
	}}
	if vs := validateDiagramEdgeEndpointHallucination(doc, oracle, nil); len(vs) != 0 {
		t.Errorf("yaml-tag mermaid endpoints must resolve Pascal Go fields via Fix E; got %+v", vs)
	}
}

// TestFixE_DiagramEndpoint_GenuineFabricationStillFires symmetric
// to the enum test — confirms Fix E preserves the hallucination
// detection guarantee on the diagram surface.
func TestFixE_DiagramEndpoint_GenuineFabricationStillFires(t *testing.T) {
	body := "graph TD\n" +
		"    realCheckCoverage --> pipeline_imaginary_setting\n"
	doc := docWithDiagram("call_dag", body)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"realCheckCoverage": 1,
	}}
	vs := validateDiagramEdgeEndpointHallucination(doc, oracle, nil)
	if len(vs) != 1 {
		t.Fatalf("genuine diagram-endpoint fabrication must still fire; got %d:\n  %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "pipeline_imaginary_setting") {
		t.Errorf("violation must list the genuine hallucination; got: %s", vs[0].Detail)
	}
}

// TestFixE_FormCrossingMatrix is the consolidated multi-language /
// multi-surface-form fixture table. Each row asserts that an LLM-
// emitted token in one form resolves an oracle-indexed entry under
// any of the other forms. This is the core promise of Fix E: the
// 5-form-equivalence class { snake / camel / Pascal /
// SCREAMING_SNAKE / kebab / flatcase } collapses to a single
// canonical lookup.
func TestFixE_FormCrossingMatrix(t *testing.T) {
	cases := []struct {
		name        string
		oracleForm  string // form the graph indexes
		emittedForm string // form the LLM renders in the answer
	}{
		// Pascal (Go field) ↔ snake (yaml-tag)
		{"PascalToSnake", "PipelineMaxSteps", "pipeline_max_steps"},
		{"SnakeToPascal", "pipeline_max_steps", "PipelineMaxSteps"},
		// Pascal ↔ camel
		{"PascalToCamel", "RequestHandler", "requestHandler"},
		{"CamelToPascal", "requestHandler", "RequestHandler"},
		// Pascal ↔ kebab (CLI)
		{"PascalToKebab", "PipelineMaxSteps", "pipeline-max-steps"},
		{"KebabToPascal", "pipeline-max-steps", "PipelineMaxSteps"},
		// Pascal ↔ SCREAMING_SNAKE (env var)
		{"PascalToScream", "PipelineMaxSteps", "PIPELINE_MAX_STEPS"},
		{"ScreamToPascal", "PIPELINE_MAX_STEPS", "PipelineMaxSteps"},
		// snake ↔ kebab
		{"SnakeToKebab", "pipeline_max_steps", "pipeline-max-steps"},
		{"KebabToSnake", "pipeline-max-steps", "pipeline_max_steps"},
		// snake ↔ SCREAMING_SNAKE
		{"SnakeToScream", "pipeline_max_steps", "PIPELINE_MAX_STEPS"},
		// camel ↔ flatcase
		{"CamelToFlat", "requestHandler", "requesthandler"},
		{"FlatToCamel", "requesthandler", "requestHandler"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oracle := &stubOracleFixB{tiers: map[string]int{tc.oracleForm: 1}}
			found, tier := oracle.SymbolExistsFlat(tc.emittedForm)
			if !found {
				t.Errorf("Fix E flat lookup: emitted=%q must resolve oracle=%q (same flat form)",
					tc.emittedForm, tc.oracleForm)
			}
			if found && tier != 1 {
				t.Errorf("Fix E flat tier: emitted=%q resolved oracle=%q got tier=%d, want 1",
					tc.emittedForm, tc.oracleForm, tier)
			}
		})
	}
}

// TestFixE_FormCrossingMatrix_DistinctNamesDoNotCollide pins the
// inverse: two genuinely different identifiers MUST NOT collapse
// to the same flat key. (`PipelineMaxSteps` vs
// `PipelineMinSteps` differ by mid-word letters → flat forms
// differ → no false match.)
func TestFixE_FormCrossingMatrix_DistinctNamesDoNotCollide(t *testing.T) {
	oracle := &stubOracleFixB{tiers: map[string]int{
		"PipelineMaxSteps": 1,
	}}
	// Distinct logical identifier — should NOT resolve.
	if found, _ := oracle.SymbolExistsFlat("pipeline_min_steps"); found {
		t.Errorf("distinct identifier MUST NOT collide via flat form; got match")
	}
	if found, _ := oracle.SymbolExistsFlat("PipelineMaxStepsAndExtra"); found {
		t.Errorf("longer prefix-extended identifier MUST NOT match shorter; got match")
	}
}
