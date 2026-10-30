package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Uses the existing target/window-filtered ledger, before the generic Top-N.
// This does not create a runtime-work relation, accepted focus, or causal seat.
func renderAnswerDocBusinessTreeFacts(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	const limit = 16
	var b strings.Builder
	seen := map[string]bool{}
	accepted, shown := 0, 0
	for _, r := range ledger.Records {
		fact, ok := tool.DecodeTraceBusinessTreeFact(r)
		if !ok {
			continue
		}
		profile, qualified := answerDocRuntimeDisplayScope(ctx, ledger, r)
		if !qualified || profile.HasExplicitTimeWindows() && (fact.WindowUnavailableReason != "" || !profile.ContainsExplicitTimeWindow(fact.Window.StartTs, fact.Window.EndTs)) {
			continue
		}
		key, _ := json.Marshal(r)
		if seen[string(key)] {
			continue
		}
		seen[string(key)] = true
		accepted++
		if shown == limit {
			continue
		}
		if shown == 0 {
			b.WriteString("### 已观测同步业务层级 / Observed synchronous work nesting\n\n")
			b.WriteString("- 以下为真实同源同线程同步打点的父子关系，不是唤醒因果链。原生为全线程观察库存按起始行展示，不是目标过滤或耗时排名；按用户所问线程解释，其它线程只作背景。总耗时包含子层，自身耗时扣除完整直接子区间并集，按各条记录的查询范围与已配对区间计量；不要把父子总耗时相加，也不要从展示省略后的子项重算自身。实例身份不能用名称替代；跨线程或邻近业务不能猜父子。\n")
			b.WriteString("- 状态按每个业务总区间/自身离散区间分别求交；未知不是零，睡眠中IO等待为睡眠包含项。业务耗时不是CPU执行时间、可消除量或根因贡献。父子图须区分缺失父/未闭合/展示省略，链上根因仍需独立唤醒与等待证据。Use the requested answer language and reader-facing business labels, not internal IDs/status codes; retain IDs only for unambiguous evidence references.\n")
			b.WriteString("- 同源、同线程、同一查询范围及单位下，已证互斥自身分段及其同状态交集可合计；小计只覆盖实际参与的分段，省略或未知不能补零或外推全集。此规则针对业务时间，不把采样权重换成毫秒，也不增加因果权限。\n")
		}
		fmt.Fprintf(&b, "- %s；%s；该查询观察实例=%d、原生未展示=%d；observation_id=%q；physical_source=%q；query_index=%q；query_scope=%q；payload=%q；support=%q\n",
			tool.TraceBusinessTreeNodeText(fact.Node), tool.TraceBusinessTreeWindowText(fact), fact.NodeCount, fact.OmittedNodes,
			r.ID, fact.Node.SourcePath, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef, strings.Join(r.SupportRefs, "; "))
		shown++
	}
	if shown > 0 {
		fmt.Fprintf(&b, "- 最终上下文独立展示 %d 条查询发布记录、另省略 %d 条已发布记录；这是保留各查询身份的记录数，不是跨查询去重后的物理实例总数。原生与上下文省略不可混为无打点或完整覆盖。未闭合行仅保存已观测身份，不证明跨窗仍活动。\n\n", shown, accepted-shown)
	}
	return b.String()
}

// The principal lane keeps its existing scope and budget. A second, bounded
// field handoff only completes an exact business interval whose entire native
// summary was actually published in this dispatch; it creates no new subject,
// target, causal relation, or query-completion claim.
func renderAnswerDocScopedBusinessTreeFacts(ctx *types.AgentContext, ledger, principal types.ObservationLedger, publication *answerDocNativePresentationReceipt) string {
	return renderAnswerDocBusinessTreeFacts(ctx, principal) + renderAnswerDocPublishedBusinessAccounts(ctx, ledger, principal, publication)
}

func renderAnswerDocPublishedBusinessAccounts(ctx *types.AgentContext, ledger, principal types.ObservationLedger, publication *answerDocNativePresentationReceipt) string {
	if publication == nil {
		return ""
	}
	type population struct{ trees, spans []types.ObservationRecord }
	groups := map[string]*population{}
	spanQueries, conflictingIDs := map[string]string{}, map[string]bool{}
	for _, r := range ledger.Records {
		if r.Predicate != types.TraceBusinessTreePredicate && r.Predicate != types.TraceBusinessSpanPredicate {
			continue
		}
		key := nativePresentationQueryKey(r)
		if key == "" {
			continue
		}
		if groups[key] == nil {
			groups[key] = &population{}
		}
		if r.Predicate == types.TraceBusinessTreePredicate {
			groups[key].trees = append(groups[key].trees, r)
		} else {
			groups[key].spans = append(groups[key].spans, r)
			if previous, exists := spanQueries[r.ID]; exists && previous != key {
				conflictingIDs[r.ID] = true
			}
			spanQueries[r.ID] = key
		}
	}
	principalIDs := map[string]bool{}
	for _, r := range principal.Records {
		principalIDs[r.ID] = true
	}
	// Verify each complete predicate population before selecting a member.
	// A display match cannot lend another query its source/generation receipt.
	checked, current := map[string]bool{}, map[string]bool{}
	seen := map[string]bool{}
	var b strings.Builder
	shown, omitted := 0, 0
	for _, annotation := range publication.businessAnnotations {
		span := annotation.Record
		if span.Predicate != types.TraceBusinessSpanPredicate || seen[span.ID] || conflictingIDs[span.ID] {
			continue
		}
		p, ok := types.DecodeRuntimeMeasurementPublication(span)
		if !ok || len(p.Tables) != 1 || p.Tables[0].View != types.RuntimeMeasurementSummary {
			continue
		}
		published := publication.measurementViewPresentation(p.Tables[0])
		if !published.rowsComplete || !published.notesComplete {
			continue
		}
		// The ID only locates the compiled population. The private census
		// below binds its original source/query and every unmodified row;
		// compiler display aliases cannot forge or broaden that binding.
		key := spanQueries[span.ID]
		group := groups[key]
		if group == nil {
			continue
		}
		if !checked[key] {
			checked[key] = true
			current[key] = ledger.CurrentRuntimeQueryPopulation(span.SourceRef, types.TraceBusinessTreePredicate, group.trees) &&
				ledger.CurrentRuntimeQueryPopulation(span.SourceRef, types.TraceBusinessSpanPredicate, group.spans)
		}
		if !current[key] {
			continue
		}
		var matched types.ObservationRecord
		var account tool.TraceBusinessTreeFact
		ambiguous := false
		for _, r := range group.trees {
			fact, valid := tool.DecodeTraceBusinessTreeFact(r)
			if !valid || !businessAccountMatchesPublishedInterval(fact, annotation) {
				continue
			}
			if matched.ID != "" && matched.ID != r.ID {
				ambiguous = true
			}
			matched, account = r, fact
		}
		if matched.ID == "" || ambiguous || principalIDs[matched.ID] {
			continue
		}
		profile, qualified := answerDocRuntimeDisplayScope(ctx, ledger, matched)
		if !qualified || profile.HasExplicitTimeWindows() && (account.WindowUnavailableReason != "" || !profile.ContainsExplicitTimeWindow(account.Window.StartTs, account.Window.EndTs)) {
			continue
		}
		seen[span.ID] = true
		if shown == 16 {
			omitted++
			continue
		}
		if shown == 0 {
			b.WriteString("### 已展示业务区间的同源字段补全 / Same-instance account details\n\n")
			b.WriteString("- 以下仅补全本轮已完整展示的业务区间字段，仍是背景事实，不改变主分析目标、查询总体或因果权限。自身时长与各状态覆盖分别可用；状态未知不能抹去已知时长，也不能补零。不得把包含子层的总量重复相加。\n")
		}
		fmt.Fprintf(&b, "- 已展示区间 observation_id=%q；对应同源账户 observation_id=%q；query_scope=%q；payload=%q；%s；%s；support=%q\n",
			span.ID, matched.ID, matched.SourceRef.QueryScopeID, matched.SourceRef.PayloadRef, tool.TraceBusinessTreeNodeText(account.Node), tool.TraceBusinessTreeWindowText(account), strings.Join(matched.SupportRefs, "; "))
		shown++
	}
	if shown > 0 {
		fmt.Fprintf(&b, "- 字段补全受独立16条展示预算约束，另省略=%d；不计入上面的查询发布记录数，也不声明新增物理实例或完整覆盖。完整账户仍在各自原始查询载荷。\n\n", omitted)
	}
	return b.String()
}

func businessAccountMatchesPublishedInterval(f tool.TraceBusinessTreeFact, p types.NativeBusinessDiagramAnnotation) bool {
	n := f.Node
	return n.Closure == "closed" && n.ActualEndTs != nil && n.SourcePath == p.Record.SourceRef.Path &&
		types.RuntimeWakeupThread(n.Thread) == p.Owner && n.StartLine == p.StartLine && n.EndLine == p.EndLine &&
		n.ActualStartTs == p.StartTs && *n.ActualEndTs == p.EndTs && n.Name == p.Name
}
