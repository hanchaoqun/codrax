package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderStructuredAggregateFactsPrioritizesPrincipalMemberSet(t *testing.T) {
	var facts []types.AnswerAggregateFact
	for i := 0; i < 18; i++ {
		facts = append(facts, types.AnswerAggregateFact{
			Kind:  types.AnswerAggregateTotalCount,
			Label: fmt.Sprintf("candidate bucket %02d", i),
			Value: fmt.Sprintf("%d", i),
		})
	}
	facts = append(facts, types.AnswerAggregateFact{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "verified qualifying members",
		Value:   "1",
		Members: []string{"explorer"},
	})

	got := renderStructuredAggregateFacts(facts, 16)
	if !strings.Contains(got, "label=verified qualifying members") ||
		!strings.Contains(got, "role=`principal_answer`") ||
		!strings.Contains(got, "members=[`explorer`]") {
		t.Fatalf("principal member_set should survive prompt projection:\n%s", got)
	}
	if strings.Index(got, "label=verified qualifying members") > strings.Index(got, "label=candidate bucket 00") {
		t.Fatalf("principal member_set should render before lower-priority count facts:\n%s", got)
	}
}

func TestRenderStructuredAggregateFactsDemotesConflictingParallelCounts(t *testing.T) {
	facts := []types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateScalar,
			Label: "func 数量",
			Value: "8",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
		},
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "公开函数",
			Value:   "5",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Eval", "EvalAll", "IsRegistered", "RegisteredKinds", "SetExternalArtifactFloor"},
		},
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &types.CompletenessObligation{
			Required:    true,
			SourceQuote: "all public symbols",
		},
	}}}

	got := renderStructuredAggregateFactsForContext(ctx, facts)
	if !strings.Contains(got, "label=func 数量") ||
		!strings.Contains(got, "role=`supporting_coverage`") ||
		!strings.Contains(got, "demoted:conflicts_with_principal_member_set_cardinality") {
		t.Fatalf("conflicting count should remain visible only as support context:\n%s", got)
	}
	if !strings.Contains(got, "label=公开函数") ||
		!strings.Contains(got, "role=`principal_answer`") {
		t.Fatalf("principal member_set should remain authoritative:\n%s", got)
	}
}

func TestRenderStructuredAggregateFactsShowsUnifiedEvidenceOrigin(t *testing.T) {
	facts := []types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateScalar,
		Label:      "direct history matches",
		Value:      "0",
		Provenance: "git_history_search",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "proof_source", Value: "git_history_search"},
			{Name: "measurement_kind", Value: "vcs_history_count"},
		},
	}}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Predicates: types.SemanticPredicates{
			IsHistoryLookup: true,
			IsCountQuestion: true,
			IsScalarAnswer:  true,
		},
	}}}

	got := renderStructuredAggregateFactsForContext(ctx, facts)
	for _, want := range []string{
		"evidence_origin=[`vcs_metadata`, `command_measurement`]",
		"provenance=git_history_search",
		"dimensions=[proof_source=git_history_search, measurement_kind=vcs_history_count]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("aggregate prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRenderStructuredAggregateFactsDoesNotTruncateCompleteCountMembers(t *testing.T) {
	var members []string
	var refs []string
	for i := 1; i <= 25; i++ {
		members = append(members, fmt.Sprintf("Kind%02d", i))
		refs = append(refs, fmt.Sprintf("Kind%02d @ internal/analysis/criterion/grammar.go:%d", i, 28+i))
	}
	facts := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateBucketCount,
		Label:       "Kind 常量",
		Value:       "25",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     members,
		SupportRefs: refs,
	}}

	got := renderStructuredAggregateFacts(facts, 16)
	if strings.Contains(got, "... +") {
		t.Fatalf("complete count-carried member set must not be prompt-truncated:\n%s", got)
	}
	if !strings.Contains(got, "`Kind25`") ||
		!strings.Contains(got, "`Kind25 @ internal/analysis/criterion/grammar.go:53`") {
		t.Fatalf("last complete member/support ref should survive prompt projection:\n%s", got)
	}
}

func TestRenderStructuredAggregateFactsPrioritizesPrincipalNonFileAggregate(t *testing.T) {
	var facts []types.AnswerAggregateFact
	for i := 0; i < 18; i++ {
		facts = append(facts, types.AnswerAggregateFact{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   fmt.Sprintf("coverage bucket %02d", i),
			Value:   "1",
			Role:    types.AnswerAggregateRoleSupportingCoverage,
			Members: []string{fmt.Sprintf("helper-%02d", i)},
		})
	}
	facts = append(facts, types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateTotalCount,
		Label: "verified command count",
		Value: "42",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Unit:  "matches",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "tool", Value: "rg"},
			{Name: "scope", Value: "production"},
		},
	})

	got := renderStructuredAggregateFacts(facts, 16)
	if !strings.Contains(got, "label=verified command count") ||
		!strings.Contains(got, "kind=`total_count`") ||
		!strings.Contains(got, "role=`principal_answer`") ||
		!strings.Contains(got, "value=`42`") ||
		!strings.Contains(got, "tool=rg") {
		t.Fatalf("principal non-file aggregate should survive prompt projection:\n%s", got)
	}
	if strings.Index(got, "label=verified command count") > strings.Index(got, "label=coverage bucket 00") {
		t.Fatalf("principal non-file aggregate should render before coverage facts:\n%s", got)
	}
}

func TestRenderStructuredAggregateFactsMarksPathSetsAsCoverageForArchitecture(t *testing.T) {
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "files inspected",
		Value:   "2",
		Members: []string{"internal/agent/explorer.go", "internal/agent/agent.go"},
	}}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityComplex,
		Predicates: types.SemanticPredicates{
			IsCrossComponent: true,
		},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture},
	}}}

	got := renderStructuredAggregateFactsForContext(ctx, facts)
	if !strings.Contains(got, "role=`supporting_coverage`") {
		t.Fatalf("path-only member_set should render as coverage context for architecture answers:\n%s", got)
	}
	if strings.Contains(got, "role=`principal_answer`") {
		t.Fatalf("path-only coverage set must not be labeled principal for architecture answers:\n%s", got)
	}
}

func TestRenderStructuredAggregateFactsUsesExplicitRoleAndProvenance(t *testing.T) {
	facts := []types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "files inspected",
		Value:      "1",
		Role:       types.AnswerAggregateRoleAuditLedger,
		Provenance: "command:rg",
		Members:    []string{"internal/agent/explorer.go"},
	}}
	got := renderStructuredAggregateFacts(facts, 16)
	if !strings.Contains(got, "role=`audit_ledger`") || !strings.Contains(got, "provenance=command:rg") {
		t.Fatalf("explicit role/provenance should render as structured metadata:\n%s", got)
	}
	if strings.Contains(got, "role=`principal_answer`") {
		t.Fatalf("explicit audit role must not be re-labeled principal:\n%s", got)
	}
}

func TestRenderStructuredAggregateFactsIncludesNegativeSearchDimensions(t *testing.T) {
	facts, err := types.NormalizeAnswerAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateNegativeSearch,
		Label: "frameworks/base ressched interface search",
		Value: "0",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "repo", Value: "frameworks/base"},
			{Name: "pattern", Value: "ResSched|IRemoteBroker|SAMR"},
			{Name: "scope", Value: "frameworks/base"},
			{Name: "searched_at", Value: "explore iteration 20"},
		},
	}})
	if err != nil {
		t.Fatalf("negative_search aggregate should validate: %v", err)
	}
	got := renderStructuredAggregateFacts(facts, 16)
	for _, want := range []string{
		"kind=`negative_search`",
		"label=frameworks/base ressched interface search",
		"value=`0`",
		"unit=matches",
		"repo=frameworks/base",
		"pattern=ResSched|IRemoteBroker|SAMR",
		"scope=frameworks/base",
		"searched_at=explore iteration 20",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered aggregate facts missing %q:\n%s", want, got)
		}
	}
}

func TestRenderStructuredAggregateFactsOmitExcludedCandidatesUnderTypedPolicy(t *testing.T) {
	facts := []types.AnswerAggregateFact{{
		Kind:     types.AnswerAggregateExcluded,
		Label:    "excluded variables",
		Value:    "2",
		Excluded: []string{"registered", "defaultExternalArtifactFloor"},
	}}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
			IsExclusionRequested: true,
			ExcludedCandidateRoles: []types.AnswerCandidateRole{
				types.AnswerCandidateRoleVariable,
			},
		},
	}}}

	got := renderStructuredAggregateFactsForContext(ctx, facts)
	if !strings.Contains(got, "excluded_count=2") ||
		!strings.Contains(got, "excluded_candidates=omitted_by_typed_exclusion_policy") {
		t.Fatalf("typed exclusion policy should keep only excluded count/category metadata:\n%s", got)
	}
	if strings.Contains(got, "registered") || strings.Contains(got, "defaultExternalArtifactFloor") {
		t.Fatalf("concrete excluded candidates must not be projected to downstream prompts:\n%s", got)
	}

	reason := "变量（如 registered map、defaultExternalArtifactFloor）按要求未列入。"
	sanitized := sanitizeAggregateExcludedCandidatesForPrompt(ctx, reason, facts)
	if strings.Contains(sanitized, "registered") || strings.Contains(sanitized, "defaultExternalArtifactFloor") {
		t.Fatalf("closure prose should be redacted from concrete excluded candidates, got %q", sanitized)
	}
	if !strings.Contains(sanitized, "[excluded candidate omitted]") {
		t.Fatalf("closure prose should retain an omission marker, got %q", sanitized)
	}
}

func TestRenderStructuredAggregateFactsCompactsShadowedSourceInventoryMemberSets(t *testing.T) {
	facts := []types.AnswerAggregateFact{
		{
			Kind:       types.AnswerAggregateMemberSet,
			Label:      "foreign func 声明",
			Value:      "3",
			Role:       types.AnswerAggregateRoleSupportingCoverage,
			Provenance: "explorer;demoted:shadowed_by_source_inventory_principal_row_set",
			Members: []string{
				"native_add",
				"runOnMainThread",
			},
			SupportRefs: []string{
				"native_add: internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6",
				"runOnMainThread: internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:16",
			},
		},
		{
			Kind:       types.AnswerAggregateMemberSet,
			Label:      "source inventory principal rows",
			Value:      "1",
			Role:       types.AnswerAggregateRolePrincipalAnswer,
			Provenance: types.SourceInventoryPrincipalRowSetAggregateProvenance,
			Members:    []string{"native_add"},
			SupportRefs: []string{
				"native_add: internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6",
			},
		},
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		},
	}}}

	got := renderStructuredAggregateFactsForContext(ctx, facts)
	if strings.Contains(got, "`runOnMainThread`") ||
		strings.Contains(got, "`runOnMainThread: internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:16`") {
		t.Fatalf("shadowed source-inventory support rows must not leak as prompt members:\n%s", got)
	}
	if !strings.Contains(got, "members_compacted_due_to=shadowed_by_authoritative_principal_rows") ||
		!strings.Contains(got, "support_ref_count=2") {
		t.Fatalf("shadowed aggregate should stay auditable as compact metadata:\n%s", got)
	}
	if !strings.Contains(got, "members_rendered_in=authoritative_principal_member_rows") {
		t.Fatalf("system principal row-set should remain the visible member carrier:\n%s", got)
	}
}

func TestStructuredAggregatePromptFactLimitExpandsForComplexTypedQuestions(t *testing.T) {
	var facts []types.AnswerAggregateFact
	for i := 0; i < 24; i++ {
		facts = append(facts, types.AnswerAggregateFact{
			Kind:  types.AnswerAggregateTotalCount,
			Label: fmt.Sprintf("bucket %02d", i),
			Value: fmt.Sprintf("%d", i),
		})
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Complexity: types.ComplexityComplex,
		Predicates: types.SemanticPredicates{
			IsCrossComponent:      true,
			IsCategoryEnumeration: true,
			IsRelationalLookup:    true,
		},
		SubTopics: []types.SubTopic{{Summary: "left"}, {Summary: "right"}},
	}}}

	if got := structuredAggregatePromptFactLimit(ctx, facts); got <= structuredAggregateDefaultPromptFacts {
		t.Fatalf("complex typed questions should expand aggregate prompt budget, got %d", got)
	}
}

func TestStructuredAggregatePromptFactLimitDoesNotOmitPrincipalMemberSetRows(t *testing.T) {
	var facts []types.AnswerAggregateFact
	for i := 0; i < structuredAggregateMaxPromptFacts+3; i++ {
		facts = append(facts, types.AnswerAggregateFact{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   fmt.Sprintf("principal set %02d", i),
			Value:   "1",
			Members: []string{fmt.Sprintf("member-%02d", i)},
		})
	}

	got := structuredAggregatePromptFactLimit(nil, facts)
	if got != len(facts) {
		t.Fatalf("principal member_set fact rows should not be capped below principal count, got %d want %d", got, len(facts))
	}
}

func TestStructuredAggregatePromptFactLimitDoesNotOmitPrincipalNonFileRows(t *testing.T) {
	var facts []types.AnswerAggregateFact
	for i := 0; i < structuredAggregateMaxPromptFacts+3; i++ {
		facts = append(facts, types.AnswerAggregateFact{
			Kind:  types.AnswerAggregateTotalCount,
			Label: fmt.Sprintf("principal count %02d", i),
			Value: fmt.Sprintf("%d", i),
			Role:  types.AnswerAggregateRolePrincipalAnswer,
		})
	}

	got := structuredAggregatePromptFactLimit(nil, facts)
	if got != len(facts) {
		t.Fatalf("principal non-file aggregate facts should not be capped below principal count, got %d want %d", got, len(facts))
	}
}
