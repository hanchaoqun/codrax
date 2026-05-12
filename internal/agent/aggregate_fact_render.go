package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func renderStructuredAggregateFacts(facts []types.AnswerAggregateFact, maxFacts int) string {
	if len(facts) == 0 {
		return ""
	}
	if maxFacts <= 0 || maxFacts > len(facts) {
		maxFacts = len(facts)
	}
	var b strings.Builder
	for i := 0; i < maxFacts; i++ {
		fact := facts[i]
		fmt.Fprintf(&b, "- kind=`%s`, label=%s, value=`%s`",
			fact.Kind, fact.Label, fact.Value)
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
		if refs := renderAggregateStringList(fact.SupportRefs, 8); refs != "" {
			fmt.Fprintf(&b, ", support_refs=[%s]", refs)
		}
		b.WriteString("\n")
	}
	if len(facts) > maxFacts {
		fmt.Fprintf(&b, "- ... %d more aggregate fact(s) omitted from prompt\n", len(facts)-maxFacts)
	}
	return b.String()
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
