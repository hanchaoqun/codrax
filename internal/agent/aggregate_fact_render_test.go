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
