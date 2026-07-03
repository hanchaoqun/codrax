package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// violation_registry_legacy_test.go — F2 migration completion
// (2026-07-03). The pre-v3 legacy literals (severity switch, soft-kind
// map, fallback-policy map, layer switch, evaluator repair-phase
// switch) are DELETED; the registry is the sole source. The golden
// snapshot below replaces the legacy-vs-registry comparison: every
// registry edit is a visible one-line diff here, never a silent
// routing/severity change. Regenerate a row ONLY as a deliberate,
// reviewed behavior change.

// violGoldenRow is one pinned routing tuple:
// {DefaultSeverity, StrictSeverity(effective), SoftByDefault,
// Promotable, FallbackLocus, Layer, RepairPhase(effective)}.
type violGoldenRow struct {
	Default    types.Severity
	Strict     types.Severity
	Soft       bool
	Promotable bool
	Locus      types.RepairLocus
	Layer      string
	Phase      types.RepairPhase
}

var violRegistryGolden = map[types.ViolationKind]violGoldenRow{
	"shape":                                  {"medium", "high", true, true, "finalizer", "contract_check", "structure"},
	"citation":                               {"medium", "high", true, true, "explore", "contract_check", "consistency"},
	"must_include":                           {"medium", "high", true, true, "finalizer", "contract_check", "consistency"},
	"must_exclude":                           {"medium", "high", true, true, "finalizer", "contract_check", "consistency"},
	"acceptance":                             {"medium", "high", true, true, "explore", "contract_check", "consistency"},
	"success_criterion":                      {"medium", "high", true, true, "explore", "contract_check", "consistency"},
	"ghost_anchor":                           {"medium", "high", true, true, "explore", "cgec", "consistency"},
	"chain_demoted":                          {"medium", "high", true, true, "explore", "cgec", "consistency"},
	"self_ref_literal":                       {"medium", "high", true, true, "finalizer", "cgec", "consistency"},
	"pre_complete_downgrade":                 {"medium", "high", true, true, "finalizer", "cgec", "consistency"},
	"literal_form_failed":                    {"medium", "high", true, true, "finalizer", "cgec", "consistency"},
	"shape_swap":                             {"medium", "high", true, true, "finalizer", "cgec", "structure"},
	"view_intent_mismatch":                   {"medium", "high", true, true, "finalizer", "answer_oracle", "structure"},
	"sub_topic_count_mismatch":               {"medium", "high", true, true, "finalizer", "answer_oracle", "structure"},
	"diagram_identifier_unverified":          {"medium", "high", true, true, "finalizer", "v2_oracle", "consistency"},
	"declared_count_drift":                   {"medium", "high", true, true, "extract", "v2_oracle", "structure"},
	"self_contradiction":                     {"medium", "high", true, true, "finalizer", "self_consistency", "consistency"},
	"external_artifact_underdecoded":         {"medium", "high", true, true, "finalizer", "external_artifact", "consistency"},
	"authority_overreach":                    {"critical", "critical", true, true, "finalizer", "authority", "consistency"},
	"plan_critic_risk":                       {"soft", "soft", true, true, "terminal", "reviewer", "consistency"},
	"reflector_observation":                  {"soft", "soft", true, true, "terminal", "reviewer", "consistency"},
	"answer_reviewer_distilled":              {"soft", "soft", true, true, "terminal", "reviewer", "consistency"},
	"intent_trace_shallow":                   {"medium", "high", true, true, "explore", "answer_oracle", "coverage"},
	"intent_enumerate_not_list":              {"medium", "high", true, true, "finalizer", "answer_oracle", "coverage"},
	"intent_root_cause_no_cause":             {"medium", "high", true, true, "explore", "answer_oracle", "coverage"},
	"intent_config_no_trail":                 {"medium", "high", true, true, "explore", "answer_oracle", "coverage"},
	"subject_anchor_missing":                 {"medium", "high", true, true, "extract", "answer_oracle", "coverage"},
	"predicate_axis_missing":                 {"medium", "high", true, true, "explore", "answer_oracle", "coverage"},
	"facet_uncovered":                        {"medium", "high", true, true, "explore", "v2_oracle", "coverage"},
	"claim_form_unsupported":                 {"medium", "high", true, true, "finalizer", "v2_oracle", "consistency"},
	"absence_scope_exceeded":                 {"medium", "high", true, true, "extract", "v2_oracle", "consistency"},
	"missing_requested_role_undisclosed":     {"medium", "medium", true, true, "finalizer", "v2_oracle", "structure"},
	"exact_resolution_ungrounded":            {"medium", "high", true, true, "finalizer", "v2_oracle", "structure"},
	"scalar_value_ungrounded":                {"medium", "high", true, true, "finalizer", "v2_oracle", "structure"},
	"step_identifier_unverified":             {"medium", "high", true, true, "explore", "answer_oracle", "consistency"},
	"richness_regression":                    {"soft", "soft", true, false, "terminal", "v2_oracle", "richness"},
	"value_secondary_citation_off_focus":     {"medium", "high", true, true, "extract", "answer_oracle", "consistency"},
	"symbol_anchor_mismatch":                 {"soft", "high", true, true, "explore", "v2_oracle", "consistency"},
	"enumeration_label_ungrounded":           {"medium", "high", true, true, "extract", "contract_check", "consistency"},
	"enumeration_label_hallucinated":         {"medium", "high", true, true, "finalizer", "contract_check", "consistency"},
	"inline_identifier_hallucinated":         {"medium", "high", true, true, "finalizer", "contract_check", "consistency"},
	"diagram_edge_endpoint_hallucinated":     {"soft", "soft", true, false, "terminal", "contract_check", "consistency"},
	"enumeration_item_label_extractor_drift": {"medium", "high", true, true, "finalizer", "contract_check", "consistency"},
	"lane_block_kind_mismatch":               {"medium", "medium", true, true, "finalizer", "contract_check", "structure"},
	"principal_support_member_omitted":       {"medium", "medium", true, true, "finalizer", "contract_check", "consistency"},
	"structural_enumeration_divergence":      {"soft", "high", true, true, "extract", "v2_oracle", "structure"},
	"cross_citation_conflict":                {"medium", "high", true, true, "extract", "v2_oracle", "consistency"},
	"demotion_storm":                         {"soft", "soft", false, false, "terminal", "contract_check", "consistency"},
	"forced_read_storm":                      {"soft", "soft", false, false, "terminal", "contract_check", "consistency"},
	"block_coverage_missing":                 {"critical", "critical", true, true, "extract", "v2_oracle", "structure"},
	"principal_claim_use_missing":            {"critical", "critical", true, true, "finalizer", "v2_oracle", "structure"},
	"diagram_edge_unsupported":               {"medium", "high", true, true, "finalizer", "v2_oracle", "consistency"},
	"diagram_edge_label_mismatch":            {"soft", "soft", true, false, "finalizer", "v2_oracle", "consistency"},
	"uncertainty_block_missing":              {"soft", "high", true, true, "finalizer", "v2_oracle", "structure"},
	"answer_semantic_underfilled":            {"soft", "medium", true, true, "finalizer", "semantic_quality", "coverage"},
	"answer_topic_mismatch":                  {"high", "high", true, true, "finalizer", "semantic_quality", "coverage"},
	"current_status_verdict_missing":         {"medium", "high", true, true, "finalizer", "v2_oracle", "structure"},
	"exhaustive_member_set_coverage_drift":   {"medium", "medium", true, true, "finalizer", "contract_check", "consistency"},
	"inactive_scope_disclosure_missing":      {"soft", "soft", true, true, "terminal", "contract_check", "consistency"},
	"enumeration_evidence_underspecified":    {"soft", "medium", true, true, "explore", "evidence_pool", "richness"},
	"diagram_relation_label_only":            {"soft", "soft", true, false, "terminal", "v2_oracle", "richness"},
	"richness_glaring_gap":                   {"soft", "high", true, true, "finalizer", "v2_oracle", "richness"},
	"principal_prose_underfilled":            {"soft", "high", true, true, "finalizer", "v2_oracle", "richness"},
	"write_cross_sub_repo_forbidden":         {"soft", "soft", true, false, "terminal", "write_contract", "consistency"},
	"denied_token_undeclared":                {"soft", "medium", true, true, "finalizer", "answer_validator", "consistency"},
	"scalar_count_unsourced":                 {"medium", "medium", true, true, "explore", "tier2_completeness", "structure"},
	"path_depth_insufficient":                {"medium", "medium", true, true, "explore", "tier2_completeness", "structure"},
	"cardinality_short":                      {"medium", "medium", true, true, "extract", "tier2_completeness", "structure"},
	"entity_parity_imbalanced":               {"medium", "medium", true, true, "explore", "tier2_completeness", "structure"},
}

// TestRegistryGoldenSnapshot pins the full derived routing surface for
// every registered kind. It is the replacement for the deleted
// legacy-table comparison (F2-4): a registry edit must show up as an
// explicit diff of this table.
func TestRegistryGoldenSnapshot(t *testing.T) {
	if len(violRegistryGolden) != len(types.AllViolationKinds()) {
		t.Fatalf("golden has %d rows, AllViolationKinds has %d — regenerate the table when adding kinds",
			len(violRegistryGolden), len(types.AllViolationKinds()))
	}
	for _, kind := range types.AllViolationKinds() {
		want, ok := violRegistryGolden[kind]
		if !ok {
			t.Errorf("kind=%q missing from golden table", kind)
			continue
		}
		spec, ok := types.ViolKindSpecFor(kind)
		if !ok {
			t.Errorf("kind=%q: missing registry spec", kind)
			continue
		}
		got := violGoldenRow{
			Default:    spec.DefaultSeverity,
			Strict:     spec.EffectiveStrictSeverity(),
			Soft:       spec.SoftByDefault,
			Promotable: spec.Promotable,
			Locus:      spec.FallbackLocus,
			Layer:      spec.Layer,
			Phase:      spec.EffectiveRepairPhase(),
		}
		if got != want {
			t.Errorf("kind=%q routing drift:\n  got  %+v\n  want %+v", kind, got, want)
		}
		// The public derivation APIs must agree with the spec they
		// wrap (DeriveSeverity strict + default paths).
		if s := types.DeriveSeverity(kind, false); s != want.Default {
			t.Errorf("kind=%q: DeriveSeverity(false)=%q, golden=%q", kind, s, want.Default)
		}
		if s := types.DeriveSeverity(kind, true); s != want.Strict {
			t.Errorf("kind=%q: DeriveSeverity(true)=%q, golden=%q", kind, s, want.Strict)
		}
		if target := targetForLocus(spec.FallbackLocus); target != targetForLocus(want.Locus) {
			t.Errorf("kind=%q: fallback target drift", kind)
		}
	}
}

// TestEveryViolKindHasSpec asserts every declared ViolationKind has a
// registry spec. Adding a new kind to violation.go without registering
// a spec fails the build via this test.
func TestEveryViolKindHasSpec(t *testing.T) {
	for _, kind := range types.AllViolationKinds() {
		if _, ok := types.ViolKindSpecFor(kind); !ok {
			t.Errorf("kind=%q has no ViolKindSpec — register in internal/types/violation_registry.go", kind)
		}
	}
}

// TestRegistrySpecCountMatchesAllViolationKinds is a defensive
// check: every registered spec must correspond to a kind in
// AllViolationKinds (no orphaned specs).
func TestRegistrySpecCountMatchesAllViolationKinds(t *testing.T) {
	declared := map[types.ViolationKind]bool{}
	for _, k := range types.AllViolationKinds() {
		declared[k] = true
	}
	for _, spec := range types.AllViolKindSpecs() {
		if !declared[spec.Kind] {
			t.Errorf("registry spec for kind=%q is not in AllViolationKinds — add to violation.go AllViolationKinds slice or remove the spec",
				spec.Kind)
		}
	}
}

// TestPermanentlySoftKindsAreStillSoftAfterPromote verifies that
// kinds with Promotable=false keep RetryEligible=false even when
// callers pass isStrict=true.
func TestPermanentlySoftKindsAreStillSoftAfterPromote(t *testing.T) {
	for _, spec := range types.AllViolKindSpecs() {
		if spec.Promotable {
			continue
		}
		profile := types.ViolationProfileFor(spec.Kind, true)
		if profile.RetryEligible {
			t.Errorf("kind=%q is Promotable=false but RetryEligible=true after isStrict — operator promote should be a no-op",
				spec.Kind)
		}
		if profile.Severity != spec.DefaultSeverity {
			t.Errorf("kind=%q is Promotable=false but Severity changed under isStrict — got %q want %q",
				spec.Kind, profile.Severity, spec.DefaultSeverity)
		}
	}
}
