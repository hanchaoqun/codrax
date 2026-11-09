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
func renderAnswerDocPerfSamplingTrees(ctx *types.AgentContext, ledger types.ObservationLedger, publications ...*answerDocNativePresentationReceipt) string {
	var publication *answerDocNativePresentationReceipt
	if len(publications) == 1 {
		publication = publications[0]
	}
	publication.prepareSamplingIdentities(ctx)
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
	// Query receipts stay independent. Only exact content or qualified
	// equivalent displays share a slot: source, target, window, denominators,
	// actual omissions and physical references remain in the key.
	seenFacts := map[string]bool{}
	publishedFacts := map[string]bool{}
	aggregateFacts := map[string]bool{}
	selectedFacts := map[string]bool{}
	type displayOwner struct {
		id    string
		depth int
	}
	factRefs := map[string]displayOwner{}
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
		if conflict[k] {
			publication.observe(r, false)
			publication.observeSamplingSelection(r, false)
			publication.observeSamplingAggregate(r, false)
			continue
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		fact := contents[k]
		if publication != nil && publication.samplingIdentities[nativePresentationQueryKey(r)] != "" {
			fact = publication.samplingIdentities[nativePresentationQueryKey(r)]
		}
		if p.Status == "available" && seenFacts[fact] {
			owner := factRefs[fact]
			depth := ""
			if p.DisplayDepth != owner.depth {
				depth = fmt.Sprintf(" query_display_depth=%d shared_display_depth=%d", p.DisplayDepth, owner.depth)
			}
			alias := fmt.Sprintf("native_tree_reuse observation_id=%q source=%q query_scope=%q payload=%q%s: shares displayed fields with observation_id=%q; native_summary=%q; query/cohort IDs and original references remain in their own native payload.\n", r.ID, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef, depth, owner.id, r.Summary)
			linked := publication.writeDetailNavigation(&b, alias, 63<<10)
			publication.observe(r, linked && publishedFacts[fact])
			publication.observeSamplingSelection(r, linked && selectedFacts[fact])
			publication.observeSamplingAggregate(r, linked && aggregateFacts[fact])
			continue
		}
		accepted++
		if shown >= 4 {
			publication.observe(r, false)
			publication.observeSamplingSelection(r, false)
			publication.observeSamplingAggregate(r, false)
			continue
		}
		var query strings.Builder
		if shown == 0 {
			query.WriteString("### 采样前缀树 / Observed sampling prefix trees\n\n")
		}
		fmt.Fprintf(&query, "observation_id=%q source=%q query_scope=%q payload=%q\n", r.ID, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef)
		fmt.Fprintf(&query, "native_summary=%q\n", r.Summary)
		query.WriteString(tool.TracePerfSamplingTreeText(p))
		if b.Len()+query.Len() > 63<<10 || !publication.takePresentationBytes(query.Len()) {
			publication.observe(r, false)
			publication.observeSamplingSelection(r, false)
			publication.observeSamplingAggregate(r, false)
			continue
		}
		b.WriteString(query.String())
		if p.Status == "available" {
			seenFacts[fact] = true
		}
		factRefs[fact] = displayOwner{r.ID, p.DisplayDepth}
		selected := p.Status == "available" && p.OmittedCohorts == 0
		selectedFacts[fact] = selected
		publication.observeSamplingSelection(r, selected)
		aggregateComplete := p.Status == "available" && p.OmittedCohorts == 0 && p.DisplayRoot == ""
		referencesComplete := true
		for _, cohort := range p.Cohorts {
			aggregateComplete = aggregateComplete && cohort.OmittedNodes == 0
			for _, node := range cohort.Nodes {
				aggregateComplete = aggregateComplete && node.OmittedChildren == 0
				referencesComplete = referencesComplete && node.OmittedReferences == 0
			}
		}
		complete := aggregateComplete && referencesComplete
		aggregateFacts[fact] = aggregateComplete
		publication.observeSamplingAggregate(r, aggregateComplete)
		publishedFacts[fact] = complete
		publication.observe(r, complete)
		shown++
	}
	if accepted > 0 {
		fmt.Fprintf(&b, "另省略采样树查询=%d；原始载荷保留完整查询展示。折叠不改变原cohort分母，采样权重不是毫秒；前缀父子不是实际调用、唤醒、等待依赖或根因证据。\n\n", accepted-shown)
	}
	return b.String()
}
