package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	structuredAggregateDefaultPromptFacts = 16
	structuredAggregateMaxPromptFacts     = 48
)

func renderStructuredAggregateFactsForContext(ctx *types.AgentContext, facts []types.AnswerAggregateFact) string {
	return renderStructuredAggregateFactsWithPrincipalRefs(facts, structuredAggregatePromptFactLimit(ctx, facts), structuredAggregatePrincipalMemberSetRefs(ctx, facts))
}

func structuredAggregatePromptFactLimit(ctx *types.AgentContext, facts []types.AnswerAggregateFact) int {
	if len(facts) == 0 {
		return 0
	}
	limit := structuredAggregateDefaultPromptFacts
	principalCount := len(structuredAggregatePrincipalMemberSetRefs(ctx, facts))
	if principalCount > 0 {
		limit += aggregateFactMinInt(principalCount*2, 16)
	}
	if ctx != nil && ctx.AnalysisIR != nil {
		rm := ctx.AnalysisIR.RequestModel
		if len(rm.SubTopics) >= 2 {
			limit += aggregateFactMinInt(len(rm.SubTopics)*2, 8)
		}
		if buckets := rm.QuestionStructure().Buckets; len(buckets) >= 2 {
			limit += aggregateFactMinInt(len(buckets)*2, 8)
		}
		if rm.Complexity == types.ComplexityComplex {
			limit += 4
		}
		if rm.Predicates.IsCrossComponent || rm.Predicates.IsCategoryEnumeration || rm.Predicates.IsRelationalLookup {
			limit += 4
		}
	}
	if limit > structuredAggregateMaxPromptFacts {
		limit = structuredAggregateMaxPromptFacts
	}
	if limit < principalCount {
		// The max cap is for auxiliary aggregate context. Principal member_set
		// rows are the answer slate and must not disappear behind prompt-budget
		// projection after the model has emitted them structurally.
		limit = principalCount
	}
	if limit > len(facts) {
		limit = len(facts)
	}
	return limit
}

func renderStructuredAggregateFacts(facts []types.AnswerAggregateFact, maxFacts int) string {
	return renderStructuredAggregateFactsWithPrincipalRefs(facts, maxFacts, types.PrincipalAggregateMemberSetFactRefs(facts))
}

func renderStructuredAggregateFactsWithPrincipalRefs(facts []types.AnswerAggregateFact, maxFacts int, refs []types.AnswerAggregateFactRef) string {
	if len(facts) == 0 {
		return ""
	}
	principalMemberSets := map[int]bool{}
	for _, ref := range refs {
		principalMemberSets[ref.Index] = true
	}
	order := orderedAggregateFactIndexes(facts, principalMemberSets)
	if maxFacts <= 0 || maxFacts > len(order) {
		maxFacts = len(order)
	}
	var b strings.Builder
	for displayIdx := 0; displayIdx < maxFacts; displayIdx++ {
		i := order[displayIdx]
		fact := facts[i]
		fmt.Fprintf(&b, "- kind=`%s`, label=%s, value=`%s`",
			fact.Kind, fact.Label, fact.Value)
		if fact.Kind == types.AnswerAggregateMemberSet {
			if principalMemberSets[i] {
				b.WriteString(", principal_member_set=true")
			} else {
				b.WriteString(", coverage_axis_only=true")
			}
		}
		if fact.Unit != "" {
			fmt.Fprintf(&b, ", unit=%s", fact.Unit)
		}
		if dims := renderAggregateDimensions(fact.Dimensions); dims != "" {
			fmt.Fprintf(&b, ", dimensions=[%s]", dims)
		}
		memberLimit := 12
		if fact.Kind == types.AnswerAggregateMemberSet {
			memberLimit = 200
		}
		if members := renderAggregateStringList(fact.Members, memberLimit); members != "" {
			fmt.Fprintf(&b, ", members=[%s]", members)
		}
		if excluded := renderAggregateStringList(fact.Excluded, 8); excluded != "" {
			fmt.Fprintf(&b, ", excluded=[%s]", excluded)
		}
		refLimit := 8
		if fact.Kind == types.AnswerAggregateMemberSet {
			refLimit = 200
		}
		if refs := renderAggregateStringList(fact.SupportRefs, refLimit); refs != "" {
			fmt.Fprintf(&b, ", support_refs=[%s]", refs)
		}
		b.WriteString("\n")
	}
	if len(facts) > maxFacts {
		fmt.Fprintf(&b, "- ... %d more aggregate fact(s) omitted from prompt\n", len(facts)-maxFacts)
	}
	return b.String()
}

func structuredAggregatePrincipalMemberSetRefs(ctx *types.AgentContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFactRef {
	if ctx == nil || ctx.AnalysisIR == nil {
		return types.PrincipalAggregateMemberSetFactRefs(facts)
	}
	return types.PrincipalAggregateMemberSetFactRefsForRequest(facts, &ctx.AnalysisIR.RequestModel)
}

func orderedAggregateFactIndexes(facts []types.AnswerAggregateFact, principalMemberSets map[int]bool) []int {
	indexes := make([]int, len(facts))
	for i := range facts {
		indexes[i] = i
	}
	sort.SliceStable(indexes, func(a, b int) bool {
		ia, ib := indexes[a], indexes[b]
		pa := aggregateFactPromptPriority(facts[ia], principalMemberSets[ia])
		pb := aggregateFactPromptPriority(facts[ib], principalMemberSets[ib])
		if pa != pb {
			return pa < pb
		}
		return ia < ib
	})
	return indexes
}

func aggregateFactPromptPriority(fact types.AnswerAggregateFact, principal bool) int {
	if principal {
		return 0
	}
	switch fact.Kind {
	case types.AnswerAggregateScalar:
		return 1
	case types.AnswerAggregateMemberSet:
		return 2
	case types.AnswerAggregateTotalCount,
		types.AnswerAggregateUniqueCount,
		types.AnswerAggregateGroupedCount,
		types.AnswerAggregateBucketCount:
		return 3
	case types.AnswerAggregateExcluded:
		return 4
	default:
		return 5
	}
}

func aggregateFactMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func renderAggregateDimensions(dims []types.AnswerAggregateDimension) string {
	if len(dims) == 0 {
		return ""
	}
	parts := make([]string, 0, len(dims))
	for _, dim := range dims {
		if dim.Name == "" || dim.Value == "" {
			continue
		}
		parts = append(parts, dim.Name+"="+dim.Value)
	}
	return strings.Join(parts, ", ")
}

func renderAggregateStringList(items []string, limit int) string {
	if len(items) == 0 || limit <= 0 {
		return ""
	}
	if len(items) < limit {
		limit = len(items)
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		item := strings.TrimSpace(items[i])
		if item == "" {
			continue
		}
		parts = append(parts, "`"+item+"`")
	}
	if len(items) > limit {
		parts = append(parts, fmt.Sprintf("... +%d", len(items)-limit))
	}
	return strings.Join(parts, ", ")
}
