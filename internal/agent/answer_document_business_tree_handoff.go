package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Uses the existing target/window-filtered ledger, before the generic Top-N.
// This does not create a runtime-work relation, accepted focus, or causal seat.
func renderAnswerDocBusinessTreeFacts(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	return renderAnswerDocBusinessAccountPlan(planAnswerDocBusinessTreeFacts(ctx, ledger))
}

const answerDocBusinessAccountLimit = 16

// One source/query/instance account is the indivisible display unit. In
// particular, full-instance self and query-clipped self cannot borrow each
// other's inclusive duration, states, or segment completeness.
type answerDocBusinessAccount struct {
	record types.ObservationRecord
	fact   tool.TraceBusinessTreeFact
	key    string
	group  string
	spanID string // only the independently validated complete-table lane
}

type answerDocBusinessAccountPlan struct {
	accounts    []answerDocBusinessAccount
	accepted    int
	groups      int
	published   map[string]bool // exact records actually rendered, not candidate IDs
	shownGroups int
}

func planAnswerDocBusinessTreeFacts(ctx *types.AgentContext, ledger types.ObservationLedger) answerDocBusinessAccountPlan {
	var accounts []answerDocBusinessAccount
	for _, r := range ledger.Records {
		fact, ok := tool.DecodeTraceBusinessTreeFact(r)
		if !ok {
			continue
		}
		profile, qualified := answerDocRuntimeDisplayScope(ctx, ledger, r)
		if !qualified || profile.HasExplicitTimeWindows() && (fact.WindowUnavailableReason != "" || !profile.ContainsExplicitTimeWindow(fact.Window.StartTs, fact.Window.EndTs)) {
			continue
		}
		accounts = append(accounts, answerDocBusinessAccount{record: r, fact: fact})
	}
	return selectAnswerDocBusinessAccounts(accounts)
}

// Give every exact source/query population one account before spending its
// next slot. The grouping retains payload generation, target, clock and actual
// measuring window; equal business names or physical instance IDs never merge
// queries. A unique whole-instance match to this query's exact thread/window
// is display navigation, not a user focus or a causal root. Otherwise only a
// producer-declared observed root receives nesting priority; missing parents
// are not inferred from names, chronology or depth heuristics.
func selectAnswerDocBusinessAccounts(accounts []answerDocBusinessAccount) answerDocBusinessAccountPlan {
	plan := answerDocBusinessAccountPlan{published: map[string]bool{}}
	groups := map[string][]answerDocBusinessAccount{}
	seen := map[string]bool{}
	for _, a := range accounts {
		a.key = traceEventPublicationRecordKey(a.record)
		query := nativePresentationQueryKey(a.record)
		if a.key == "" || query == "" || seen[a.key] {
			continue
		}
		seen[a.key] = true
		group, _ := json.Marshal([]any{query, a.fact.Node.SourcePath, a.fact.Window, a.fact.WindowUnavailableReason})
		a.group = string(group)
		groups[a.group] = append(groups[a.group], a)
		plan.accepted++
	}
	keys := make([]string, 0, len(groups))
	for key, rows := range groups {
		keys = append(keys, key)
		instance := answerDocBusinessQueryInstanceKey(rows)
		sort.Slice(rows, func(i, j int) bool {
			a, b := rows[i], rows[j]
			if selectedA, selectedB := a.key == instance, b.key == instance; selectedA != selectedB {
				return selectedA
			}
			if instance != "" {
				// A uniquely bound complete query instance supplies an exact
				// owner. Share the producer's measured-account display order
				// for its remaining slots, without accepting any causal focus.
				pid := a.record.SourceRef.QueryTargetPID
				if less, reverse := tracequery.TraceMarkerTreeDisplayLess(a.fact.Node, b.fact.Node, pid), tracequery.TraceMarkerTreeDisplayLess(b.fact.Node, a.fact.Node, pid); less != reverse {
					return less
				}
			}
			aroot, broot := a.fact.Node.ParentStatus == "observed_root", b.fact.Node.ParentStatus == "observed_root"
			if aroot != broot {
				return aroot
			}
			if a.fact.Node.StartLine != b.fact.Node.StartLine {
				return a.fact.Node.StartLine < b.fact.Node.StartLine
			}
			return a.key < b.key
		})
	}
	sort.Strings(keys)
	plan.groups = len(keys)
	shownGroups := map[string]bool{}
	for round := 0; len(plan.accounts) < answerDocBusinessAccountLimit; round++ {
		progress := false
		for _, key := range keys {
			if round >= len(groups[key]) || len(plan.accounts) == answerDocBusinessAccountLimit {
				continue
			}
			a := groups[key][round]
			plan.accounts = append(plan.accounts, a)
			plan.published[a.key] = true
			shownGroups[key] = true
			progress = true
		}
		if !progress {
			break
		}
	}
	plan.shownGroups = len(shownGroups)
	return plan
}

// Only already-admitted records from one exact source/query generation reach
// this matcher. Actual endpoints identify the physical pair; query-clipped
// inclusive/self segments can give many enclosing nodes identical endpoints.
// An absent/ambiguous owner or pair preserves the previous ordering. This
// neither mints an accepted business focus nor changes query/causal authority.
func answerDocBusinessQueryInstanceKey(rows []answerDocBusinessAccount) string {
	key, matches := "", 0
	for _, a := range rows {
		s, n := a.record.SourceRef, a.fact.Node
		if s.QueryTargetScope != "thread" || s.QueryTargetPID <= 0 || s.QueryTargetPID > types.RuntimeTargetMaxPID ||
			s.QueryTargetPID != n.Thread.PID || s.QueryTargetThread != "" && s.QueryTargetThread != n.Thread.Comm ||
			!s.QueryWindowKnown || a.fact.WindowUnavailableReason != "" || n.Closure != "closed" || n.ActualEndTs == nil ||
			n.ActualStartTs != s.QueryWindowStartTs || *n.ActualEndTs != s.QueryWindowEndTs {
			continue
		}
		key, matches = a.key, matches+1
	}
	if matches != 1 {
		return ""
	}
	return key
}

func renderAnswerDocBusinessAccountPlan(plan answerDocBusinessAccountPlan) string {
	var b strings.Builder
	for i, a := range plan.accounts {
		r, fact := a.record, a.fact
		if i == 0 {
			b.WriteString("### 已观测同步业务层级 / Observed synchronous work nesting\n\n")
			b.WriteString("- 以下为真实同源同线程同步打点的父子关系，不是唤醒因果链。原生为全线程观察库存，不是目标过滤结果；下列按各来源与查询公平分配完整账户，组内优先保留与查询线程及完整起止时刻精确对应的唯一实例，再优先展示该线程已测总区间较大的账户；没有唯一实例时保留已证最外层、原始起始行顺序。排序仅分配展示预算，不改变用户所问主体或因果身份。按用户所问线程解释，其它线程只作背景。总耗时包含子层，自身耗时扣除完整直接子区间并集，按各条记录的查询范围与已配对区间计量；不要把父子总耗时相加，也不要从展示省略后的子项重算自身。实例身份不能用名称替代；跨线程或邻近业务不能猜父子。\n")
			b.WriteString("- 状态按每个业务总区间/自身离散区间分别求交；未知不是零，睡眠中IO等待为睡眠包含项。业务耗时不是CPU执行时间、可消除量或根因贡献。父子图须区分缺失父/未闭合/展示省略，链上根因仍需独立唤醒与等待证据。Use the requested answer language and reader-facing business labels, not internal IDs/status codes; retain IDs only for unambiguous evidence references.\n")
			b.WriteString("- 同源、同线程、同一查询范围及单位下，已证互斥自身分段及其同状态交集可合计；小计只覆盖实际参与的分段，省略或未知不能补零或外推全集。此规则针对业务时间，不把采样权重换成毫秒，也不增加因果权限。\n")
			b.WriteString("- 每条总量、自身量、状态与窗口共同引用同一账户，不把另一窗口的自身量接到本条总量上。账户展示独立于测量表预览，不使被省略的表行或父子成员变为完整。\n")
		}
		fmt.Fprintf(&b, "- %s；%s；该查询观察实例=%d、原生未展示=%d；observation_id=%q；physical_source=%q；query_index=%q；query_scope=%q；payload=%q；support=%q\n",
			tool.TraceBusinessTreeNodeText(fact.Node), tool.TraceBusinessTreeWindowText(fact), fact.NodeCount, fact.OmittedNodes,
			r.ID, fact.Node.SourcePath, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef, strings.Join(r.SupportRefs, "; "))
	}
	if shown := len(plan.accounts); shown > 0 {
		fmt.Fprintf(&b, "- 最终上下文独立展示 %d 条查询发布记录、另省略 %d 条已发布记录；这是保留各查询身份的记录数，不是跨查询去重后的物理实例总数。原生与上下文省略不可混为无打点或完整覆盖。未闭合行仅保存已观测身份，不证明跨窗仍活动。\n", shown, plan.accepted-shown)
		fmt.Fprintf(&b, "- 独立账户预算仍为16条，覆盖来源查询组=%d/%d、未进入展示的组=%d；其余完整账户保留在各自原始查询载荷，不由已展示分段补算。\n\n", plan.shownGroups, plan.groups, plan.groups-plan.shownGroups)
	}
	return b.String()
}

// The principal lane keeps its existing scope and budget. A second, bounded
// field handoff only completes an exact business interval whose entire native
// summary was actually published in this dispatch; it creates no new subject,
// target, causal relation, or query-completion claim.
func renderAnswerDocScopedBusinessTreeFacts(ctx *types.AgentContext, ledger, principal types.ObservationLedger, publication *answerDocNativePresentationReceipt) string {
	plan := planAnswerDocBusinessTreeFacts(ctx, principal)
	return renderAnswerDocBusinessAccountPlan(plan) + renderAnswerDocPublishedBusinessAccountsAfter(ctx, ledger, plan.published, publication)
}

func renderAnswerDocPublishedBusinessAccounts(ctx *types.AgentContext, ledger, principal types.ObservationLedger, publication *answerDocNativePresentationReceipt) string {
	return renderAnswerDocPublishedBusinessAccountsAfter(ctx, ledger, planAnswerDocBusinessTreeFacts(ctx, principal).published, publication)
}

func renderAnswerDocPublishedBusinessAccountsAfter(ctx *types.AgentContext, ledger types.ObservationLedger, publishedAccounts map[string]bool, publication *answerDocNativePresentationReceipt) string {
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
	// Verify each complete predicate population before selecting a member.
	// A display match cannot lend another query its source/generation receipt.
	checked, current := map[string]bool{}, map[string]bool{}
	seen := map[string]bool{}
	var accounts []answerDocBusinessAccount
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
		if matched.ID == "" || ambiguous || publishedAccounts[traceEventPublicationRecordKey(matched)] {
			continue
		}
		profile, qualified := answerDocRuntimeDisplayScope(ctx, ledger, matched)
		if !qualified || profile.HasExplicitTimeWindows() && (account.WindowUnavailableReason != "" || !profile.ContainsExplicitTimeWindow(account.Window.StartTs, account.Window.EndTs)) {
			continue
		}
		seen[span.ID] = true
		accounts = append(accounts, answerDocBusinessAccount{record: matched, fact: account, spanID: span.ID})
	}
	plan := selectAnswerDocBusinessAccounts(accounts)
	var b strings.Builder
	for i, a := range plan.accounts {
		matched, account := a.record, a.fact
		if i == 0 {
			b.WriteString("### 已展示业务区间的同源字段补全 / Same-instance account details\n\n")
			b.WriteString("- 以下仅补全本轮已完整展示的业务区间字段，仍是背景事实，不改变主分析目标、查询总体或因果权限。自身时长与各状态覆盖分别可用；状态未知不能抹去已知时长，也不能补零。不得把包含子层的总量重复相加。\n")
		}
		fmt.Fprintf(&b, "- 已展示区间 observation_id=%q；对应同源账户 observation_id=%q；query_scope=%q；payload=%q；%s；%s；support=%q\n",
			a.spanID, matched.ID, matched.SourceRef.QueryScopeID, matched.SourceRef.PayloadRef, tool.TraceBusinessTreeNodeText(account.Node), tool.TraceBusinessTreeWindowText(account), strings.Join(matched.SupportRefs, "; "))
	}
	if shown := len(plan.accounts); shown > 0 {
		fmt.Fprintf(&b, "- 字段补全受独立16条展示预算约束，另省略=%d；覆盖来源查询组=%d/%d；不计入上面的查询发布记录数，也不声明新增物理实例或完整覆盖。完整账户仍在各自原始查询载荷。\n\n", plan.accepted-shown, plan.shownGroups, plan.groups)
	}
	return b.String()
}

func businessAccountMatchesPublishedInterval(f tool.TraceBusinessTreeFact, p types.NativeBusinessDiagramAnnotation) bool {
	n := f.Node
	return n.Closure == "closed" && n.ActualEndTs != nil && n.SourcePath == p.Record.SourceRef.Path &&
		types.RuntimeWakeupThread(n.Thread) == p.Owner && n.StartLine == p.StartLine && n.EndLine == p.EndLine &&
		n.ActualStartTs == p.StartTs && *n.ActualEndTs == p.EndTs && n.Name == p.Name
}
