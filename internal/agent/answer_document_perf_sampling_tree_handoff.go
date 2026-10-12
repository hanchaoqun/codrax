package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Sampling prefixes are supporting observations only. This display does not
// enter the runtime causal projection, source-call graph or root-rank pool.
func renderAnswerDocPerfSamplingTrees(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	type key struct{ source, scope, payload string }
	keyOf := func(r types.ObservationRecord) key {
		return key{r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef}
	}
	contents := map[key]string{}
	conflict := map[key]bool{}
	for _, r := range ledger.Records {
		if p, ok := tool.DecodeTracePerfSamplingTree(r); ok {
			k := keyOf(r)
			b, _ := json.Marshal(p)
			if prev, exists := contents[k]; exists && prev != string(b) {
				conflict[k] = true
			}
			contents[k] = string(b)
		}
	}
	var b strings.Builder
	seen := map[key]bool{}
	// Query receipts stay independent. Only byte-identical, validated native
	// tree publications share this display slot: source generations, target,
	// window, full denominators and all expansion/omission details remain in
	// the key. A different view or outer payload ID is not a different tree.
	seenFacts := map[string]bool{}
	accepted, shown := 0, 0
	for _, r := range ledger.Records {
		p, ok := tool.DecodeTracePerfSamplingTree(r)
		if !ok {
			continue
		}
		profile, qualified := answerDocRuntimeDisplayScope(ctx, ledger, r)
		if !qualified || profile.HasExplicitTimeWindows() && !resourceStackInsideRequestedWindow(profile, p.Window) {
			continue
		}
		k := keyOf(r)
		if seen[k] || conflict[k] {
			continue
		}
		seen[k] = true
		fact := contents[k]
		if p.Status == "available" && seenFacts[fact] {
			continue
		}
		if p.Status == "available" {
			seenFacts[fact] = true
		}
		accepted++
		if shown >= 4 {
			continue
		}
		var query strings.Builder
		if shown == 0 {
			query.WriteString("### 采样前缀树 / Observed sampling prefix trees\n\n")
		}
		fmt.Fprintf(&query, "observation_id=%q source=%q query_scope=%q payload=%q\n", r.ID, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef)
		query.WriteString(tool.TracePerfSamplingTreeText(p))
		if b.Len()+query.Len() > 63<<10 {
			continue
		}
		b.WriteString(query.String())
		shown++
	}
	if accepted > 0 {
		fmt.Fprintf(&b, "另省略采样树查询=%d；原始载荷保留完整查询展示。折叠不改变原cohort分母，采样权重不是毫秒；前缀父子不是实际调用、唤醒、等待依赖或根因证据。\n\n", accepted-shown)
	}
	return b.String()
}
